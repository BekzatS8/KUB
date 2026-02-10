// internal/handlers/integrations_handler.go
package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type IntegrationsHandler struct {
	TG        *services.TelegramService
	LinksRepo repositories.TelegramLinkRepository
	UsersRepo repositories.UserRepository
	TaskSvc   services.TaskService

	Env          string
	ConfigSource string
	DBDSNMasked  string
	FrontendHost string
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
	return &IntegrationsHandler{TG: tg, LinksRepo: links, UsersRepo: users, TaskSvc: taskSvc, Env: "unknown", ConfigSource: "unknown"}
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
		var payload struct {
			Code string `json:"code" form:"code"`
		}
		_ = c.ShouldBind(&payload)
		code = strings.TrimSpace(payload.Code)
	}
	if code == "" {
		badRequest(c, "code is required")
		return
	}
	originalCode := code
	code = strings.ToUpper(code)
	if h.LinksRepo == nil || h.UsersRepo == nil {
		internalError(c, "integration disabled")
		return
	}

	nowUTC := time.Now().UTC()
	codeForLog := code
	if len(codeForLog) > 8 {
		codeForLog = codeForLog[:8]
	}
	requestHost := c.Request.Host
	log.Printf("[TG:LINK][diag] env=%s config_source=%s frontend_host=%s request_host=%s db=%s code_prefix=%s code_changed=%v query_code_present=%v",
		h.Env,
		h.ConfigSource,
		h.FrontendHost,
		requestHost,
		h.DBDSNMasked,
		codeForLog,
		originalCode != code,
		strings.TrimSpace(c.Query("code")) != "",
	)

	link, err := h.LinksRepo.GetByCode(c.Request.Context(), code)
	if err != nil || link == nil || link.Used || nowUTC.After(link.ExpiresAt.UTC()) {
		if link == nil {
			log.Printf("[TG:LINK][diag] lookup result: not_found code_prefix=%s err=%v", codeForLog, err)
		} else {
			log.Printf("[TG:LINK][diag] lookup result: found=false used=%v expires_at_utc=%s now_utc=%s diff=%s err=%v",
				link.Used,
				link.ExpiresAt.UTC().Format(time.RFC3339),
				nowUTC.Format(time.RFC3339),
				time.Until(link.ExpiresAt.UTC()).String(),
				err,
			)
		}
		log.Printf("[TG:LINK] invalid code_prefix=%s err=%v", codeForLog, err)
		badRequest(c, "invalid or expired code")
		return
	}
	log.Printf("[TG:LINK][diag] lookup result: found=true used=%v expires_at_utc=%s now_utc=%s diff=%s chat_attached=%v",
		link.Used,
		link.ExpiresAt.UTC().Format(time.RFC3339),
		nowUTC.Format(time.RFC3339),
		time.Until(link.ExpiresAt.UTC()).String(),
		link.ChatID.Valid,
	)

	userIDVal, ok := c.Get("user_id")
	if !ok {
		unauthorized(c, "unauthorized")
		return
	}
	userID := toInt(userIDVal)

	chatID, err := h.LinksRepo.ConfirmLink(c.Request.Context(), code, userID)
	if err != nil {
		if errors.Is(err, repositories.ErrTelegramChatNotAttached) {
			log.Printf("[TG:LINK][diag] confirm blocked: code_prefix=%s chat is not attached yet", codeForLog)
			c.JSON(http.StatusConflict, gin.H{
				"error": "telegram chat not attached",
				"hint":  "Open Telegram bot and send /start <code> first",
			})
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[TG:LINK][diag] confirm lookup no rows: code_prefix=%s err=%v", codeForLog, err)
			badRequest(c, "invalid or expired code")
			return
		}
		log.Printf("[TG:LINK] confirm failed: %v", err)
		internalError(c, "cannot confirm link")
		return
	}
	if err := h.UsersRepo.UpdateTelegramLink(userID, chatID, true); err != nil {
		log.Printf("[TG:LINK] update user failed: %v", err)
		internalError(c, "cannot update user")
		return
	}

	// message in TG
	if h.TG != nil && h.TaskSvc != nil {
		ctx := c.Request.Context()

		uid := int64(userID)
		tasks, err := h.TaskSvc.GetAll(ctx, models.TaskFilter{AssigneeID: &uid})
		if err != nil {
			log.Printf("[TG:LINK] load tasks failed userID=%d: %v", userID, err)
		}

		msg := "✅ <b>Аккаунт успешно привязан к CRM</b>\n\n" +
			"Теперь вы будете получать уведомления о задачах.\n\n" +
			h.TG.FormatTasksList(tasks)

		if err := h.TG.SendMessage(chatID, msg); err != nil {
			log.Printf("[TG:LINK] send welcome msg failed: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// POST /integrations/telegram/request-link
// Returns code and a command for bot: "/start CODE"
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
	nowUTC := time.Now().UTC()
	expiresAt := nowUTC.Add(30 * time.Minute)

	link, err := h.LinksRepo.CreateLink(c.Request.Context(), userID, 0, code, expiresAt)
	if err != nil {
		log.Printf("[TG:REQ-LINK] create failed: %v", err)
		internalError(c, "cannot create link")
		return
	}
	codeForLog := code
	if len(codeForLog) > 8 {
		codeForLog = codeForLog[:8]
	}
	log.Printf("[TG:REQ-LINK][diag] code_prefix=%s user_id=%d created_at_utc=%s expires_at_utc=%s db=%s env=%s",
		codeForLog,
		userID,
		nowUTC.Format(time.RFC3339),
		expiresAt.Format(time.RFC3339),
		h.DBDSNMasked,
		h.Env,
	)

	c.JSON(http.StatusOK, gin.H{
		"code":          link.Code,
		"expires_at":    link.ExpiresAt,
		"start_command": "/start " + link.Code,
	})
}
