package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"turcompany/internal/services"
)

type ChatHandler struct {
	service *services.ChatService
}

type sendMessageRequest struct {
	Text        string   `json:"text" binding:"required"`
	Attachments []string `json:"attachments"`
}

func NewChatHandler(service *services.ChatService) *ChatHandler {
	return &ChatHandler{service: service}
}

func (h *ChatHandler) ListChats(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chats, err := h.service.ListUserChats(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, chats)
}

func (h *ChatHandler) ListMessages(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat id"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	messages, err := h.service.GetMessages(chatID, userID, limit, offset)
	if err != nil {
		if err == services.ErrNotChatMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "not a chat member"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, messages)
}

func (h *ChatHandler) SendMessage(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat id"})
		return
	}
	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	msg, err := h.service.SendMessage(chatID, userID, req.Text, req.Attachments)
	if err != nil {
		if err == services.ErrNotChatMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "not a chat member"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, msg)
}
