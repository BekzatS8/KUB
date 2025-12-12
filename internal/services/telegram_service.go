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

// NewTelegramService constructs the Telegram bot helper.
// linkPrefix is used when building the /integrations/telegram/link?code=... URL.
func NewTelegramService(botToken string, linkRepo repositories.TelegramLinkRepository, usersRepo repositories.UserRepository, taskSvc TaskService, linkPrefix string) *TelegramService {
	if botToken == "" {
		return nil
	}
	return &TelegramService{
		token:      botToken,
		baseURL:    fmt.Sprintf("https://api.telegram.org/bot%s", botToken),
		client:     &http.Client{},
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

	body := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
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

// SetWebhook configures Telegram webhook.
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

// HandleUpdate processes incoming webhook updates. Errors are logged and returned to the caller,
// but the HTTP layer must still return 200 to Telegram.
func (t *TelegramService) HandleUpdate(update *TelegramUpdate) error {
	if t == nil || update == nil || update.Message == nil {
		return nil
	}
	text := strings.TrimSpace(update.Message.Text)
	chatID := update.Message.Chat.ID
	switch {
	case strings.HasPrefix(text, "/start"):
		return t.handleStart(chatID)
	case strings.HasPrefix(text, "/help"):
		return t.SendMessage(chatID, t.FormatHelpMessage())
	case strings.HasPrefix(text, "/tasks"):
		return t.handleTasks(chatID)
	default:
		return t.SendMessage(chatID, t.FormatHelpMessage())
	}
}

func (t *TelegramService) handleStart(chatID int64) error {
	if t.linkRepo == nil {
		return t.SendMessage(chatID, "Интеграция временно недоступна. Попробуйте позже.")
	}
	code, err := t.generateLinkCode()
	if err != nil {
		log.Printf("[tg][start] code generation failed: %v", err)
		return t.SendMessage(chatID, "Не удалось сгенерировать код привязки, попробуйте позже.")
	}
	expiresAt := time.Now().Add(t.linkTTL)
	if _, err := t.linkRepo.CreateLink(context.Background(), 0, chatID, code, expiresAt); err != nil {
		log.Printf("[tg][start] CreateLink failed: %v", err)
	}
	return t.SendMessage(chatID, t.FormatStartMessage(code))
}

func (t *TelegramService) handleTasks(chatID int64) error {
	if t.usersRepo == nil || t.taskSvc == nil {
		return t.SendMessage(chatID, "Интеграция недоступна. Попробуйте позже.")
	}
	user, err := t.usersRepo.GetByChatID(context.Background(), chatID)
	if err != nil {
		log.Printf("[tg][tasks] lookup failed chatID=%d: %v", chatID, err)
		return t.SendMessage(chatID, "Не удалось определить пользователя. Выполните /start для привязки.")
	}
	if user == nil {
		return t.SendMessage(chatID, "Аккаунт не привязан, сначала выполните /start и привяжите код в личном кабинете.")
	}
	uid := int64(user.ID)
	tasks, err := t.taskSvc.GetAll(context.Background(), models.TaskFilter{AssigneeID: &uid})
	if err != nil {
		log.Printf("[tg][tasks] load failed for uid=%d: %v", uid, err)
		return t.SendMessage(chatID, "Не удалось получить список задач.")
	}
	return t.SendMessage(chatID, t.FormatTasksList(tasks))
}

// FormatStartMessage builds welcome + link instructions.
func (t *TelegramService) FormatStartMessage(code string) string {
	link := code
	if t.linkPrefix != "" {
		link = fmt.Sprintf("%s/integrations/telegram/link?code=%s", t.linkPrefix, code)
	}
	return fmt.Sprintf("Привет! Для привязки аккаунта перейдите по ссылке:\n%s\n\nили введите код в личном кабинете: <code>%s</code>\nДоступные команды: /start, /help, /tasks", html.EscapeString(link), code)
}

func (t *TelegramService) FormatHelpMessage() string {
	return "Доступные команды:\n/start — привязать аккаунт\n/help — помощь\n/tasks — задачи на сегодня"
}

func (t *TelegramService) FormatTasksList(tasks []models.Task) string {
	now := time.Now()
	var b strings.Builder

	activeCount := 0

	for _, tsk := range tasks {
		// ❌ Пропускаем завершённые и отменённые
		if tsk.Status == models.StatusDone || tsk.Status == models.StatusCancelled {
			continue
		}

		if activeCount == 0 {
			b.WriteString("Ваши актуальные задачи:\n")
		}
		activeCount++

		due := "—"
		if tsk.DueDate != nil {
			due = tsk.DueDate.Format("2006-01-02 15:04")
		}

		status := string(tsk.Status)
		if tsk.DueDate != nil && tsk.DueDate.Before(now) {
			status += " (просрочено)"
		}

		b.WriteString("• " + html.EscapeString(tsk.Title) + " — " + status + " (до " + due + ")\n")
	}

	// Если нет активных задач
	if activeCount == 0 {
		return "У вас нет невыполненных задач. Все актуальные задачи закрыты ✅"
	}

	return b.String()
}

func (t *TelegramService) FormatTaskNotification(task *models.Task) string {
	if task == nil {
		return ""
	}
	due := "—"
	if task.DueDate != nil {
		due = task.DueDate.Format("2006-01-02 15:04")
	}
	title := html.EscapeString(task.Title)
	entity := task.EntityType + "#" + fmt.Sprintf("%d", task.EntityID)
	return "• <b>" + title + "</b>\n" +
		"• Статус: <code>" + string(task.Status) + "</code>\n" +
		"• Приоритет: <code>" + string(task.Priority) + "</code>\n" +
		"• Срок: <code>" + due + "</code>\n" +
		"• Связано: <code>" + entity + "</code>"
}

func (t *TelegramService) generateLinkCode() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(buf)), nil
}
