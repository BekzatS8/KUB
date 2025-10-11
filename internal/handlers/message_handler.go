package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

// MessageHandler handles HTTP requests for messages.
type MessageHandler struct {
	service services.MessageService
}

// NewMessageHandler creates a new MessageHandler.
func NewMessageHandler(service services.MessageService) *MessageHandler {
	return &MessageHandler{service: service}
}

// Send handles POST /messages
func (h *MessageHandler) Send(c *gin.Context) {
	var req struct {
		ReceiverID int64  `json:"receiver_id" binding:"required"`
		Content    string `json:"content" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// senderID := c.MustGet("userID").(int64)
	senderID := int64(1) // Placeholder

	msg := &models.Message{
		SenderID:   senderID,
		ReceiverID: req.ReceiverID,
		Content:    req.Content,
	}

	sentMsg, err := h.service.Send(c.Request.Context(), msg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send message"})
		return
	}
	c.JSON(http.StatusCreated, sentMsg)
}

// GetConversationHistory handles GET /messages/history/:partner_id
func (h *MessageHandler) GetConversationHistory(c *gin.Context) {
	partnerID, err := strconv.ParseInt(c.Param("partner_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid partner ID"})
		return
	}

	// userID := c.MustGet("userID").(int64)
	userID := int64(1) // Placeholder

	history, err := h.service.GetConversationHistory(c.Request.Context(), userID, partnerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve history"})
		return
	}
	c.JSON(http.StatusOK, history)
}

// GetConversations handles GET /messages/conversations
func (h *MessageHandler) GetConversations(c *gin.Context) {
	// userID := c.MustGet("userID").(int64)
	userID := int64(1) // Placeholder

	conversations, err := h.service.GetConversations(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve conversations"})
		return
	}
	c.JSON(http.StatusOK, conversations)
}
