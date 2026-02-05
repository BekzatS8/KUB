package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"turcompany/internal/services"
)

type TelegramSignWebhookHandler struct {
	Telegram *services.TelegramService
	Service  *services.DocumentSigningConfirmationService
}

func NewTelegramSignWebhookHandler(
	telegram *services.TelegramService,
	service *services.DocumentSigningConfirmationService,
) *TelegramSignWebhookHandler {
	return &TelegramSignWebhookHandler{
		Telegram: telegram,
		Service:  service,
	}
}

type telegramCallbackUpdate struct {
	CallbackQuery *struct {
		ID      string `json:"id"`
		Data    string `json:"data"`
		Message *struct {
			MessageID int `json:"message_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
		From *struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"from"`
	} `json:"callback_query"`
}

func (h *TelegramSignWebhookHandler) Handle(c *gin.Context) {
	if h.Service == nil {
		internalError(c, "Service unavailable")
		return
	}
	var update telegramCallbackUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if update.CallbackQuery == nil {
		c.Status(http.StatusOK)
		return
	}
	action, token := parseTelegramCallbackData(update.CallbackQuery.Data)
	if action == "" || token == "" {
		c.Status(http.StatusOK)
		return
	}

	meta := map[string]any{
		"callback_id": update.CallbackQuery.ID,
	}
	if update.CallbackQuery.From != nil {
		meta["from_id"] = update.CallbackQuery.From.ID
		meta["from_username"] = update.CallbackQuery.From.Username
	}
	if update.CallbackQuery.Message != nil {
		meta["message_id"] = update.CallbackQuery.Message.MessageID
	}
	metaBytes, _ := json.Marshal(meta)

	_, err := h.Service.ConfirmByTelegramCallback(c.Request.Context(), token, action, metaBytes)
	responseText := "Ссылка недействительна"
	switch {
	case err == nil && action == "approve":
		responseText = "Подтверждено"
	case err == nil && action == "reject":
		responseText = "Отклонено"
	case err == nil:
		responseText = "Готово"
	case err != nil && errors.Is(err, services.ErrSignConfirmExpired):
		responseText = "Ссылка истекла"
	}
	if h.Telegram != nil && update.CallbackQuery.Message != nil {
		_ = h.Telegram.SendMessage(update.CallbackQuery.Message.Chat.ID, responseText)
	}
	c.Status(http.StatusOK)
}

func parseTelegramCallbackData(data string) (string, string) {
	data = strings.TrimSpace(data)
	if !strings.HasPrefix(data, "sign:") {
		return "", ""
	}
	parts := strings.SplitN(data, ":", 3)
	if len(parts) != 3 {
		return "", ""
	}
	action := strings.TrimSpace(parts[1])
	token := strings.TrimSpace(parts[2])
	if action != "approve" && action != "reject" {
		return "", ""
	}
	return action, token
}
