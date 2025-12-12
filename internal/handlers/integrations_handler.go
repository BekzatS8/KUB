package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models" // ← ДОБАВЬ ЭТО
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type IntegrationsHandler struct {
	TG        *services.TelegramService
	LinksRepo repositories.TelegramLinkRepository
	UsersRepo repositories.UserRepository
	TaskSvc   services.TaskService
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

func NewIntegrationsHandler(tg *services.TelegramService, links repositories.TelegramLinkRepository, users repositories.UserRepository, taskSvc services.TaskService) *IntegrationsHandler {
	return &IntegrationsHandler{TG: tg, LinksRepo: links, UsersRepo: users, TaskSvc: taskSvc}
}

// POST /integrations/telegram/webhook
func (h *IntegrationsHandler) Webhook(c *gin.Context) {
	defer c.Status(http.StatusOK)
	if h.TG == nil {
		log.Printf("[TG:WEBHOOK] TelegramService is nil")
		return
	}

	var up services.TelegramUpdate
	if err := c.ShouldBindJSON(&up); err != nil {
		log.Printf("[TG:WEBHOOK] failed to bind update: %v", err)
		return
	}
	if err := h.TG.HandleUpdate(&up); err != nil {
		log.Printf("[TG:WEBHOOK] handle error: %v", err)
	}
}

// GET /integrations/telegram/link?code=...
func (h *IntegrationsHandler) ConfirmLink(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		badRequest(c, "code is required")
		return
	}
	if h.LinksRepo == nil || h.UsersRepo == nil {
		internalError(c, "integration disabled")
		return
	}

	link, err := h.LinksRepo.GetByCode(c.Request.Context(), code)
	if err != nil || link == nil || link.Used || time.Now().After(link.ExpiresAt) {
		log.Printf("[TG:LINK] invalid code=%s err=%v", code, err)
		badRequest(c, "invalid or expired code")
		return
	}

	// ⚠️ ВАЖНО: используем ключ "user_id", как кладёт middleware
	userIDVal, ok := c.Get("user_id")
	if !ok {
		unauthorized(c, "unauthorized")
		return
	}
	userID := toInt(userIDVal)

	chatID, err := h.LinksRepo.ConfirmLink(c.Request.Context(), code, userID)
	if err != nil {
		log.Printf("[TG:LINK] confirm failed: %v", err)
		internalError(c, "cannot confirm link")
		return
	}
	if err := h.UsersRepo.UpdateTelegramLink(userID, chatID, true); err != nil {
		log.Printf("[TG:LINK] update user failed: %v", err)
		internalError(c, "cannot update user")
		return
	}

	// 🔔 Отправляем сообщение в Telegram: успешно привязано + задачи
	if h.TG != nil && h.TaskSvc != nil {
		ctx := c.Request.Context()

		uid := int64(userID)
		tasks, err := h.TaskSvc.GetAll(ctx, models.TaskFilter{
			AssigneeID: &uid,
		})
		if err != nil {
			log.Printf("[TG:LINK] load tasks failed userID=%d: %v", userID, err)
		}

		msg := "✅ Аккаунт успешно привязан к CRM.\n\n" +
			"Теперь вы будете получать уведомления о задачах.\n\n" +
			"Чтобы в любой момент посмотреть список задач, отправьте команду: /tasks.\n\n"

		// добавим текущие задачи пользователя
		msg += h.TG.FormatTasksList(tasks)

		if err := h.TG.SendMessage(chatID, msg); err != nil {
			log.Printf("[TG:LINK] send welcome msg failed: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *IntegrationsHandler) RequestTelegramLink(c *gin.Context) {

	userIDVal, ok := c.Get("user_id")
	if !ok {
		unauthorized(c, "unauthorized")
		return
	}
	userID := toInt(userIDVal)

	if h.LinksRepo == nil {
		internalError(c, "integration disabled")
		return
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		internalError(c, "cannot generate code")
		return
	}
	code := strings.ToUpper(hex.EncodeToString(buf))

	link, err := h.LinksRepo.CreateLink(c.Request.Context(), userID, 0, code, time.Now().Add(30*time.Minute))
	if err != nil {
		log.Printf("[TG:REQ-LINK] create failed: %v", err)
		internalError(c, "cannot create link")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":       link.Code,
		"expires_at": link.ExpiresAt,
	})
}
