package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

const btnMyTasks = "📋 Мои задачи"

type IntegrationsHandler struct {
	TG        *services.TelegramService
	LinksRepo repositories.TelegramLinkRepository
	UsersRepo repositories.UserRepository
	TaskSvc   services.TaskService
}

func NewIntegrationsHandler(
	tg *services.TelegramService,
	links repositories.TelegramLinkRepository,
	users repositories.UserRepository,
	taskSvc services.TaskService,
) *IntegrationsHandler {
	return &IntegrationsHandler{TG: tg, LinksRepo: links, UsersRepo: users, TaskSvc: taskSvc}
}

type tgUpdate struct {
	Message *struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

func ctxUserID(c *gin.Context) (int, bool) {
	keysToTry := []string{"userID", "user_id", "uid"}
	for _, k := range keysToTry {
		if v, ok := c.Get(k); ok {
			switch vv := v.(type) {
			case int:
				return vv, true
			case int64:
				return int(vv), true
			case float64:
				return int(vv), true
			case string:
				if n, err := strconv.Atoi(vv); err == nil {
					return n, true
				}
			default:
			}
		}
	}
	return 0, false
}

func normalizeLinkCode(s string) (string, bool) {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'`“”«»<>.,;:()[]{}\\")
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)

	var b strings.Builder
	for _, r := range s {
		if unicode.Is(unicode.Hex_Digit, r) {
			b.WriteRune(r)
		}
	}
	code := b.String()
	if len(code) != 32 {
		return "", false
	}
	return code, true
}

func (h *IntegrationsHandler) Webhook(c *gin.Context) {
	if h.TG == nil {
		log.Printf("[TG:WEBHOOK] TelegramService == nil (webhook disabled or no token). Return 200.")
		c.Status(http.StatusOK)
		return
	}

	var up tgUpdate
	if err := c.ShouldBindJSON(&up); err != nil || up.Message == nil {
		if err != nil {
			log.Printf("[TG:WEBHOOK] bind json error: %v", err)
		} else {
			log.Printf("[TG:WEBHOOK] empty message in update")
		}
		c.Status(http.StatusOK)
		return
	}

	text := strings.TrimSpace(up.Message.Text)
	chatID := up.Message.Chat.ID
	log.Printf("[TG:WEBHOOK] incoming: chatID=%d, text=%q", chatID, text)

	switch {
	case strings.HasPrefix(text, "/start"):
		log.Printf("[TG:WEBHOOK] /start from chatID=%d", chatID)
		_ = h.TG.SendReplyKeyboard(chatID,
			"Привет! Чтобы связать аккаунт, отправьте:\n<code>/link &lt;код&gt;</code>\n\nИли нажмите кнопку ниже, когда привяжете:",
			[][]string{{btnMyTasks}},
		)

	case strings.HasPrefix(text, "/link"):
		raw := strings.TrimSpace(strings.TrimPrefix(text, "/link"))
		log.Printf("[TG:WEBHOOK] /link from chatID=%d, code_raw=%q", chatID, raw)

		code, ok := normalizeLinkCode(raw)
		if !ok {
			log.Printf("[TG:WEBHOOK] code normalize failed: raw=%q", raw)
			_ = h.TG.SendMessage(chatID, "Неверный формат кода. Скопируйте и отправьте ровно 32 символа HEX:\n<code>/link 0123456789ABCDEF0123456789ABCDEF</code>")
			break
		}

		link, err := h.LinksRepo.UseByCode(c.Request.Context(), code)
		if err != nil {
			log.Printf("[TG:WEBHOOK] UseByCode failed (code=%q): %v", code, err)
			_ = h.TG.SendMessage(chatID, "Код недействителен или истёк. Сгенерируйте новый в личном кабинете.")
			break
		}

		if err := h.UsersRepo.UpdateTelegramLink(link.UserID, chatID, true); err != nil {
			log.Printf("[TG:WEBHOOK] UpdateTelegramLink failed: userID=%d chatID=%d err=%v", link.UserID, chatID, err)
			_ = h.TG.SendMessage(chatID, "Не удалось привязать аккаунт, попробуйте позже.")
			break
		}
		_ = h.TG.SendMessage(chatID, "Готово! Аккаунт привязан. Вы начнёте получать уведомления о задачах.")

		// Дайджест активных задач (если есть)
		if h.TaskSvc != nil {
			assigneeID := int64(link.UserID)
			filter := models.TaskFilter{AssigneeID: &assigneeID}
			tasks, err := h.TaskSvc.GetAll(c.Request.Context(), filter)
			if err == nil && len(tasks) > 0 {
				var active []models.Task
				for _, t := range tasks {
					if t.Status != models.StatusDone && t.Status != models.StatusCancelled {
						active = append(active, t)
					}
				}
				if len(active) > 0 {
					var b strings.Builder
					max := len(active)
					if max > 10 {
						max = 10
					}
					b.WriteString("📝 Ваши активные задачи:\n")
					for i := 0; i < max; i++ {
						t := active[i]
						due := "—"
						if t.DueDate != nil {
							due = t.DueDate.Format("2006-01-02 15:04")
						}
						b.WriteString("• " + t.Title + " (" + string(t.Status) + ", " + string(t.Priority) + ") [due: " + due + "]\n")
					}
					if len(active) > max {
						b.WriteString("…и ещё " + strconv.Itoa(len(active)-max) + " шт.\n")
					}
					_ = h.TG.SendMessage(chatID, b.String())
				} else {
					_ = h.TG.SendMessage(chatID, "У вас нет активных задач. 👍")
				}
			}
		}

		_ = h.TG.SendReplyKeyboard(chatID,
			"Нажмите кнопку ниже, чтобы посмотреть ваши задачи:",
			[][]string{{btnMyTasks}},
		)

	default:
		// Обработка кнопок
		if text == btnMyTasks {
			h.sendMyTasksDigest(c, chatID)
			break
		}
		_ = h.TG.SendMessage(chatID, "Не понял команду. Используйте <code>/link &lt;код&gt;</code> или кнопку меню.")
	}

	c.Status(http.StatusOK)
}

func (h *IntegrationsHandler) RequestTelegramLink(c *gin.Context) {
	authz := c.GetHeader("Authorization")
	log.Printf("[TG:REQ-LINK] Authorization header: %q", authz)

	userID, ok := ctxUserID(c)
	if !ok {
		log.Printf("[TG:REQ-LINK] userID not in context, keys=%v -> 401", c.Keys)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		log.Printf("[TG:REQ-LINK] rand.Read failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rng failed"})
		return
	}
	code := strings.ToUpper(hex.EncodeToString(buf)) // 32 HEX

	link, err := h.LinksRepo.Create(c.Request.Context(), userID, code, 30*time.Minute)
	if err != nil {
		log.Printf("[TG:REQ-LINK] LinksRepo.Create failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot create link"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":       link.Code,
		"expires_at": link.ExpiresAt,
		"hint":       "Откройте чат с ботом и отправьте: /link " + link.Code,
	})
}

// ===== Кнопка "Мои задачи" =====

func daysLeftStr(now time.Time, due *time.Time) (bucket string, sortKey int) {
	if due == nil {
		return "Без срока", 1_000_000
	}
	days := int(due.Sub(now).Hours() / 24) // floor
	switch {
	case days < 0:
		bucket = fmt.Sprintf("Просрочено (%d дн.)", -days)
	case days == 0:
		bucket = "Сегодня (0 дн.)"
	case days == 1:
		bucket = "Через 1 день"
	default:
		bucket = fmt.Sprintf("Через %d дней", days)
	}
	return bucket, days
}

func (h *IntegrationsHandler) sendMyTasksDigest(c *gin.Context, chatID int64) {
	u, err := h.UsersRepo.GetByChatID(c.Request.Context(), chatID)
	if err != nil || u == nil {
		_ = h.TG.SendMessage(chatID, "Не удалось определить пользователя по Telegram. Привяжите аккаунт командой /link.")
		return
	}
	uid := int64(u.ID)

	tasks, err := h.TaskSvc.GetAll(c.Request.Context(), models.TaskFilter{AssigneeID: &uid})
	if err != nil {
		log.Printf("[TG:MYTASKS] tasks fetch failed for uid=%d: %v", uid, err)
		_ = h.TG.SendMessage(chatID, "Не удалось загрузить задачи.")
		return
	}

	var active []models.Task
	for _, t := range tasks {
		if t.Status != models.StatusDone && t.Status != models.StatusCancelled {
			active = append(active, t)
		}
	}
	if len(active) == 0 {
		_ = h.TG.SendMessage(chatID, "У вас нет активных задач. 👍")
		return
	}

	now := time.Now()
	type grp struct {
		key   int
		items []models.Task
	}
	buckets := map[string]*grp{}

	for _, t := range active {
		bName, key := daysLeftStr(now, t.DueDate)
		g := buckets[bName]
		if g == nil {
			g = &grp{key: key}
			buckets[bName] = g
		}
		g.items = append(g.items, t)
	}

	type kv struct {
		name string
		grp  *grp
	}
	arr := make([]kv, 0, len(buckets))
	for name, g := range buckets {
		arr = append(arr, kv{name, g})
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].grp.key < arr[j].grp.key })

	var b strings.Builder
	b.WriteString("📋 <b>Мои задачи по срокам</b>\n")
	for _, it := range arr {
		b.WriteString("\n— <b>" + html.EscapeString(it.name) + "</b>\n")
		// сортируем внутри группы по due
		sort.Slice(it.grp.items, func(i, j int) bool {
			di, dj := it.grp.items[i].DueDate, it.grp.items[j].DueDate
			switch {
			case di == nil && dj == nil:
				return it.grp.items[i].ID < it.grp.items[j].ID
			case di == nil:
				return false
			case dj == nil:
				return true
			default:
				return di.Before(*dj)
			}
		})
		for _, t := range it.grp.items {
			due := "—"
			if t.DueDate != nil {
				due = t.DueDate.Format("2006-01-02")
			}
			b.WriteString("• " + html.EscapeString(t.Title) + " [до: " + due + "]\n")
		}
	}

	_ = h.TG.SendReplyKeyboard(chatID, b.String(), [][]string{{btnMyTasks}})
}
