// internal/services/telegram_service.go
package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type TelegramService struct {
	token      string
	baseURL    string
	client     *http.Client
	linkRepo   repositories.TelegramLinkRepository
	usersRepo  repositories.UserRepository
	taskSvc    TaskService
	linkTTL    time.Duration
	linkPrefix string
}

type TelegramUpdate struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
		MessageID int    `json:"message_id"`
		Text      string `json:"text"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

type tgResp struct {
	Ok          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

// linkPrefix is used when building the /integrations/telegram/link?code=... URL.
func NewTelegramService(botToken string, linkRepo repositories.TelegramLinkRepository, usersRepo repositories.UserRepository, taskSvc TaskService, linkPrefix string) *TelegramService {
	if botToken == "" {
		return nil
	}
	return &TelegramService{
		token:      botToken,
		baseURL:    fmt.Sprintf("https://api.telegram.org/bot%s", botToken),
		client:     &http.Client{Timeout: 10 * time.Second},
		linkRepo:   linkRepo,
		usersRepo:  usersRepo,
		taskSvc:    taskSvc,
		linkTTL:    30 * time.Minute,
		linkPrefix: strings.TrimSuffix(linkPrefix, "/"),
	}
}

func (t *TelegramService) SetTaskService(s TaskService) {
	if t != nil {
		t.taskSvc = s
	}
}

func (t *TelegramService) SendMessage(chatID int64, text string) error {
	if t == nil || t.token == "" || chatID == 0 {
		log.Printf("[tg][skip] token or chatID empty (token? %v chatID=%d)", t != nil && t.token != "", chatID)
		return nil
	}

	return t.sendMessage(chatID, text, nil)
}

func (t *TelegramService) SendSigningConfirm(chatID int64, docInfo, approveToken, rejectToken string) error {
	if t == nil {
		return nil
	}
	docInfo = strings.TrimSpace(docInfo)
	if docInfo == "" {
		docInfo = "Документ"
	}
	message := fmt.Sprintf(
		"✍️ <b>%s</b>\n\nПодтвердите или отклоните подписание:",
		html.EscapeString(docInfo),
	)
	replyMarkup := map[string]any{
		"inline_keyboard": [][]map[string]string{
			{
				{"text": "✅ Подтвердить", "callback_data": fmt.Sprintf("sign:approve:%s", approveToken)},
				{"text": "❌ Отклонить", "callback_data": fmt.Sprintf("sign:reject:%s", rejectToken)},
			},
		},
	}
	return t.sendMessage(chatID, message, replyMarkup)
}

func (t *TelegramService) sendMessage(chatID int64, text string, replyMarkup any) error {
	if t == nil || t.token == "" || chatID == 0 {
		log.Printf("[tg][skip] token or chatID empty (token? %v chatID=%d)", t != nil && t.token != "", chatID)
		return nil
	}

	body := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if replyMarkup != nil {
		body["reply_markup"] = replyMarkup
	}
	b, _ := json.Marshal(body)
	url := t.baseURL + "/sendMessage"
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		log.Printf("[tg][send][err] http: %v", err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var api tgResp
	_ = json.Unmarshal(respBody, &api)
	if resp.StatusCode != 200 || !api.Ok {
		log.Printf("[tg][send] http_status=%d body=%s", resp.StatusCode, string(respBody))
		return fmt.Errorf("telegram sendMessage failed: status=%d ok=%v desc=%s", resp.StatusCode, api.Ok, api.Description)
	}
	return nil
}

func (t *TelegramService) SetWebhook(url string) error {
	if t == nil || t.token == "" || url == "" {
		return nil
	}
	full := t.baseURL + "/setWebhook?url=" + url
	log.Printf("[tg][setWebhook] %s", full)
	req, _ := http.NewRequest("GET", full, nil)
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	log.Printf("[tg][setWebhook] status=%d body=%s", resp.StatusCode, string(b))
	return nil
}

func (t *TelegramService) HandleUpdate(update *TelegramUpdate) error {
	if t == nil || update == nil || update.Message == nil {
		return nil
	}
	text := strings.TrimSpace(update.Message.Text)
	chatID := update.Message.Chat.ID

	switch {
	case strings.HasPrefix(text, "/start"):
		payload := ""
		parts := strings.Fields(text)
		if len(parts) >= 2 {
			payload = strings.TrimSpace(parts[1])
		}
		return t.handleStart(chatID, payload)

	case strings.HasPrefix(text, "/help"):
		return t.SendMessage(chatID, t.FormatHelpMessage())

	case strings.HasPrefix(text, "/tasks"):
		return t.handleTasks(chatID)

	default:
		return t.SendMessage(chatID, t.FormatHelpMessage())
	}
}

func (t *TelegramService) handleStart(chatID int64, payload string) error {
	if t.linkRepo == nil {
		return t.SendMessage(chatID, "⚠️ Интеграция временно недоступна. Попробуйте позже.")
	}

	payload = strings.ToUpper(strings.TrimSpace(payload))
	codeForLog := payload
	if len(codeForLog) > 8 {
		codeForLog = codeForLog[:8]
	}

	// ✅ CRM-flow: "/start CODE" -> attach chatID to that code
	if payload != "" {
		log.Printf("[tg][start][diag] code_prefix=%s chat_id=%d attach_attempt=true", codeForLog, chatID)
		err := t.linkRepo.AttachChatID(context.Background(), payload, chatID)
		if err == nil {
			log.Printf("[tg][start][diag] code_prefix=%s chat_id=%d attach_result=attached", codeForLog, chatID)
			return t.SendMessage(chatID, t.FormatStartAttachedMessage(payload))
		}
		log.Printf("[tg][start][diag] code_prefix=%s chat_id=%d attach_result=not_found_or_expired err=%v", codeForLog, chatID, err)
		// if code not found/expired -> fallback to normal start
	}

	// ✅ Bot-flow: generate code with chatID
	code, err := t.generateLinkCode()
	if err != nil {
		log.Printf("[tg][start] code generation failed: %v", err)
		return t.SendMessage(chatID, "⚠️ Не удалось сгенерировать код привязки, попробуйте позже.")
	}
	expiresAt := time.Now().Add(t.linkTTL)
	if _, err := t.linkRepo.CreateLink(context.Background(), 0, chatID, code, expiresAt); err != nil {
		log.Printf("[tg][start] CreateLink failed: %v", err)
	}
	return t.SendMessage(chatID, t.FormatStartMessage(code))
}

func (t *TelegramService) handleTasks(chatID int64) error {
	if t.usersRepo == nil || t.taskSvc == nil {
		return t.SendMessage(chatID, "⚠️ Интеграция недоступна. Попробуйте позже.")
	}

	user, err := t.usersRepo.GetByChatID(context.Background(), chatID)
	if err != nil {
		log.Printf("[tg][tasks] lookup failed chatID=%d: %v", chatID, err)
		return t.SendMessage(chatID, "⚠️ Не удалось определить пользователя. Выполните /start для привязки.")
	}
	if user == nil {
		return t.SendMessage(chatID, t.FormatNotLinkedMessage())
	}

	uid := int64(user.ID)
	tasks, err := t.taskSvc.GetAll(context.Background(), models.TaskFilter{AssigneeID: &uid})
	if err != nil {
		log.Printf("[tg][tasks] load failed for uid=%d: %v", uid, err)
		return t.SendMessage(chatID, "⚠️ Не удалось получить список задач.")
	}

	return t.SendMessage(chatID, t.FormatTasksList(tasks))
}

func (t *TelegramService) FormatHelpMessage() string {
	return "🧭 <b>Команды</b>\n" +
		"• <code>/start</code> — подключить Telegram\n" +
		"• <code>/tasks</code> — мои задачи\n" +
		"• <code>/help</code> — помощь"
}

func (t *TelegramService) FormatNotLinkedMessage() string {
	return "🔗 <b>Аккаунт не привязан</b>\n\n" +
		"1) Отправь <code>/start</code>\n" +
		"2) Скопируй код\n" +
		"3) Вставь его в CRM (Интеграции → Telegram)\n\n" +
		"Команды: /start /help"
}

func (t *TelegramService) FormatStartAttachedMessage(code string) string {
	link := ""
	if t.linkPrefix != "" {
		link = fmt.Sprintf("%s/integrations/telegram/link?code=%s", t.linkPrefix, code)
	}
	msg := "✅ <b>Код принят</b>\n\n" +
		"Теперь заверши привязку в CRM.\n" +
		"Код: <code>" + html.EscapeString(code) + "</code>\n"
	if link != "" {
		msg += "\n<a href=\"" + html.EscapeString(link) + "\">🔗 Подтвердить привязку</a>\n"
	}
	msg += "\nКоманды: /tasks /help"
	return msg
}

func (t *TelegramService) FormatStartMessage(code string) string {
	link := ""
	if t.linkPrefix != "" {
		link = fmt.Sprintf("%s/integrations/telegram/link?code=%s", t.linkPrefix, code)
	}

	msg := "👋 <b>KUB • Telegram</b>\n" +
		"Подключим уведомления из CRM.\n\n" +
		"Код: <code>" + html.EscapeString(code) + "</code>\n" +
		"Срок: <i>30 минут</i>\n\n" +
		"1) Открой CRM → Интеграции → Telegram\n" +
		"2) Вставь код выше и нажми «Подтвердить»\n"

	if link != "" {
		msg += "\n<a href=\"" + html.EscapeString(link) + "\">🔗 Открыть страницу привязки</a>\n"
	}

	msg += "\nКоманды: /tasks /help"
	return msg
}

func (t *TelegramService) FormatTasksList(tasks []models.Task) string {
	now := time.Now()
	var b strings.Builder

	// header
	b.WriteString("📋 <b>Ваши актуальные задачи</b> • <i>" + now.Format("02.01.2006 15:04") + "</i>\n\n")

	active := make([]models.Task, 0, len(tasks))
	for _, tsk := range tasks {
		if tsk.Status == models.StatusDone || tsk.Status == models.StatusCancelled {
			continue
		}
		active = append(active, tsk)
	}

	if len(active) == 0 {
		return "✅ <b>Задач нет</b>\nВсе актуальные задачи закрыты.\n\nКоманды: /tasks /help"
	}

	for i, tsk := range active {
		title := html.EscapeString(tsk.Title)

		statusStr := string(tsk.Status)
		priorityStr := string(tsk.Priority)

		statusEmoji := map[string]string{
			"new":         "🆕",
			"in_progress": "🟡",
			"confirmed":   "✅",
			"done":        "✅",
			"cancelled":   "⛔",
		}[statusStr]
		if statusEmoji == "" {
			statusEmoji = "📌"
		}

		priEmoji := map[string]string{
			"high":   "🔴",
			"medium": "🟠",
			"low":    "🟢",
		}[priorityStr]

		// due
		dueLine := "—"
		overdue := false
		if tsk.DueDate != nil {
			dueLine = tsk.DueDate.Format("02.01.2006 15:04")
			if tsk.DueDate.Before(now) {
				overdue = true
			}
		}

		// related entity (deal/lead/etc)
		related := ""
		if tsk.EntityType != "" && tsk.EntityID > 0 {
			et := strings.ToLower(tsk.EntityType)
			switch et {
			case "deal", "deals":
				if t.linkPrefix != "" {
					related = fmt.Sprintf("<a href=\"%s/deals/%d\">deal#%d</a>", html.EscapeString(t.linkPrefix), tsk.EntityID, tsk.EntityID)
				} else {
					related = fmt.Sprintf("deal#%d", tsk.EntityID)
				}
			case "lead", "leads":
				if t.linkPrefix != "" {
					related = fmt.Sprintf("<a href=\"%s/leads/%d\">lead#%d</a>", html.EscapeString(t.linkPrefix), tsk.EntityID, tsk.EntityID)
				} else {
					related = fmt.Sprintf("lead#%d", tsk.EntityID)
				}
			default:
				related = html.EscapeString(tsk.EntityType) + "#" + fmt.Sprintf("%d", tsk.EntityID)
			}
		}

		b.WriteString(fmt.Sprintf("%d) %s %s <b>%s</b>\n", i+1, statusEmoji, priEmoji, title))
		b.WriteString("   • Статус: <code>" + html.EscapeString(statusStr) + "</code>\n")
		if priorityStr != "" {
			b.WriteString("   • Приоритет: <code>" + html.EscapeString(priorityStr) + "</code>\n")
		}
		if overdue {
			b.WriteString("   • Срок: <b>" + html.EscapeString(dueLine) + "</b> ⚠️ <b>просрочено</b>\n")
		} else {
			b.WriteString("   • Срок: <b>" + html.EscapeString(dueLine) + "</b>\n")
		}
		if related != "" {
			b.WriteString("   • Связано: " + related + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("Команды: /tasks /help")
	return b.String()
}

func (t *TelegramService) FormatTaskNotification(task *models.Task) string {
	if task == nil {
		return ""
	}

	title := html.EscapeString(task.Title)

	statusStr := string(task.Status)
	priorityStr := string(task.Priority)

	due := "—"
	overdue := false
	if task.DueDate != nil {
		due = task.DueDate.Format("02.01.2006 15:04")
		if task.DueDate.Before(time.Now()) {
			overdue = true
		}
	}

	related := ""
	if task.EntityType != "" && task.EntityID > 0 {
		et := strings.ToLower(task.EntityType)
		switch et {
		case "deal", "deals":
			if t.linkPrefix != "" {
				related = fmt.Sprintf("<a href=\"%s/deals/%d\">deal#%d</a>", html.EscapeString(t.linkPrefix), task.EntityID, task.EntityID)
			} else {
				related = fmt.Sprintf("deal#%d", task.EntityID)
			}
		case "lead", "leads":
			if t.linkPrefix != "" {
				related = fmt.Sprintf("<a href=\"%s/leads/%d\">lead#%d</a>", html.EscapeString(t.linkPrefix), task.EntityID, task.EntityID)
			} else {
				related = fmt.Sprintf("lead#%d", task.EntityID)
			}
		default:
			related = html.EscapeString(task.EntityType) + "#" + fmt.Sprintf("%d", task.EntityID)
		}
	}

	msg := "📌 <b>Новая задача</b>\n<b>" + title + "</b>\n\n" +
		"• Статус: <code>" + html.EscapeString(statusStr) + "</code>\n" +
		"• Приоритет: <code>" + html.EscapeString(priorityStr) + "</code>\n"

	if overdue {
		msg += "• Срок: <b>" + html.EscapeString(due) + "</b> ⚠️ <b>просрочено</b>\n"
	} else {
		msg += "• Срок: <b>" + html.EscapeString(due) + "</b>\n"
	}

	if related != "" {
		msg += "• Связано: " + related + "\n"
	}

	msg += "\nКоманды: /tasks /help"
	return msg
}

func (t *TelegramService) generateLinkCode() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(buf)), nil
}
