package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"turcompany/internal/realtime"
	"turcompany/internal/services"
)

type ChatHandler struct {
	service *services.ChatService
	hub     *realtime.ChatHub
}

type sendMessageRequest struct {
	Text        string   `json:"text" binding:"required"`
	Attachments []string `json:"attachments"`
}

func NewChatHandler(service *services.ChatService, hub *realtime.ChatHub) *ChatHandler {
	return &ChatHandler{service: service, hub: hub}
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
	h.hub.Broadcast(msg)
	c.JSON(http.StatusCreated, msg)
}

func (h *ChatHandler) Stream(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat id"})
		return
	}
	if err := h.service.EnsureMember(chatID, userID); err != nil {
		status := http.StatusInternalServerError
		if err == services.ErrNotChatMember {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	conn, err := realtime.Upgrade(c.Writer, c.Request)
	if err != nil {
		return
	}
	h.hub.Register(chatID, conn)
	defer h.hub.Unregister(chatID, conn)

	for {
		var incoming sendMessageRequest
		if err := conn.ReadJSON(&incoming); err != nil {
			break
		}
		msg, err := h.service.SendMessage(chatID, userID, incoming.Text, incoming.Attachments)
		if err != nil {
			_ = conn.WriteJSON(gin.H{"error": err.Error()})
			continue
		}
		h.hub.Broadcast(msg)
	}
}
