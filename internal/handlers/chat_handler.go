package handlers

import (
	"log"
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

type personalChatRequest struct {
	UserID int `json:"user_id" binding:"required"`
}

type groupChatRequest struct {
	Name    string `json:"name" binding:"required"`
	Members []int  `json:"members"`
}

type addMembersRequest struct {
	Members []int `json:"members" binding:"required"`
}

func NewChatHandler(service *services.ChatService, hub *realtime.ChatHub) *ChatHandler {
	return &ChatHandler{service: service, hub: hub}
}

func (h *ChatHandler) ListChats(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chats, err := h.service.ListUserChats(userID)
	if err != nil {
		internalError(c, "Failed to load chats")
		return
	}
	c.JSON(http.StatusOK, chats)
}

func (h *ChatHandler) SearchChats(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	query := c.Query("query")
	if query == "" {
		badRequest(c, "Query is required")
		return
	}
	chats, err := h.service.SearchChats(userID, query)
	if err != nil {
		internalError(c, "Failed to search chats")
		return
	}
	c.JSON(http.StatusOK, chats)
}

func (h *ChatHandler) CreatePersonalChat(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	var req personalChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	chat, err := h.service.CreatePersonalChat(userID, req.UserID)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusCreated, chat)
}

func (h *ChatHandler) CreateGroupChat(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	var req groupChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	chat, err := h.service.CreateGroupChat(req.Name, userID, req.Members)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusCreated, chat)
}

func (h *ChatHandler) ListMessages(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
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
		switch err {
		case services.ErrNotChatMember:
			forbidden(c, "Not a chat member")
			return
		case services.ErrChatNotFound:
			notFound(c, NotFoundCode, "Chat not found")
			return
		default:
			internalError(c, "Failed to load chat messages")
			return
		}
	}
	c.JSON(http.StatusOK, messages)
}

func (h *ChatHandler) SearchMessages(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	query := c.Query("query")
	if query == "" {
		badRequest(c, "Query is required")
		return
	}
	messages, err := h.service.SearchMessages(chatID, userID, query)
	if err != nil {
		switch err {
		case services.ErrNotChatMember:
			forbidden(c, "Not a chat member")
			return
		case services.ErrChatNotFound:
			notFound(c, NotFoundCode, "Chat not found")
			return
		default:
			internalError(c, "Failed to search messages")
			return
		}
	}
	c.JSON(http.StatusOK, messages)
}

func (h *ChatHandler) SendMessage(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	msg, unreadByUser, err := h.service.SendMessage(chatID, userID, req.Text, req.Attachments)
	if err != nil {
		switch err {
		case services.ErrNotChatMember:
			forbidden(c, "Not a chat member")
			return
		case services.ErrChatNotFound:
			notFound(c, NotFoundCode, "Chat not found")
			return
		default:
			internalError(c, "Failed to send message")
			return
		}
	}
	h.hub.Broadcast(msg)
	for uid, unread := range unreadByUser {
		h.hub.NotifyUnread(chatID, uid, unread)
	}
	c.JSON(http.StatusCreated, msg)
}

func (h *ChatHandler) UploadAttachment(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
	file, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "File is required")
		return
	}

	url, err := h.service.UploadAttachment(chatID, userID, file)
	if err != nil {
		switch err {
		case services.ErrNotChatMember:
			forbidden(c, "Not a chat member")
			return
		case services.ErrChatNotFound:
			notFound(c, NotFoundCode, "Chat not found")
			return
		default:
			badRequest(c, err.Error())
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

func (h *ChatHandler) AddMembers(c *gin.Context) {
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	var req addMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	if err := h.service.AddMembers(chatID, req.Members); err != nil {
		switch err {
		case services.ErrChatNotFound:
			notFound(c, NotFoundCode, "Chat not found")
			return
		default:
			internalError(c, "Failed to add members")
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (h *ChatHandler) LeaveChat(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	if err := h.service.LeaveChat(chatID, userID); err != nil {
		switch err {
		case services.ErrNotChatMember:
			forbidden(c, "Not a chat member")
			return
		case services.ErrChatNotFound:
			notFound(c, NotFoundCode, "Chat not found")
			return
		default:
			internalError(c, "Failed to leave chat")
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (h *ChatHandler) DeleteChat(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	if err := h.service.DeleteChat(chatID, userID); err != nil {
		switch err {
		case services.ErrChatNotFound:
			notFound(c, NotFoundCode, "Chat not found")
			return
		case services.ErrForbidden:
			forbidden(c, "Only chat creator can delete chat")
			return
		default:
			internalError(c, "Failed to delete chat")
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (h *ChatHandler) GetUserStatus(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid user id")
		return
	}
	online, lastSeen, err := h.service.GetUserStatus(userID)
	if err != nil {
		internalError(c, "Failed to load status")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"online":    online,
		"last_seen": lastSeen,
	})
}

func (h *ChatHandler) ListUnread(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chats, err := h.service.ListUnreadChats(userID)
	if err != nil {
		internalError(c, "Failed to load unread chats")
		return
	}
	c.JSON(http.StatusOK, chats)
}

func (h *ChatHandler) MarkRead(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	unread, err := h.service.MarkChatRead(chatID, userID)
	if err != nil {
		switch err {
		case services.ErrNotChatMember:
			forbidden(c, "Not a chat member")
			return
		case services.ErrChatNotFound:
			notFound(c, NotFoundCode, "Chat not found")
			return
		default:
			internalError(c, "Failed to mark chat as read")
			return
		}
	}
	h.hub.NotifyUnread(chatID, userID, unread)
	c.Status(http.StatusNoContent)
}

func (h *ChatHandler) Stream(c *gin.Context) {
	userID, _ := getUserAndRole(c)
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	if err := h.service.EnsureMember(chatID, userID); err != nil {
		switch err {
		case services.ErrNotChatMember:
			forbidden(c, "Not a chat member")
			return
		case services.ErrChatNotFound:
			notFound(c, NotFoundCode, "Chat not found")
			return
		default:
			internalError(c, "Failed to process chat request")
			return
		}
	}

	conn, err := realtime.Upgrade(c.Writer, c.Request)
	if err != nil {
		log.Printf("[chat_stream] websocket upgrade failed for chat %d user %d: %v", chatID, userID, err)
		writeError(c, http.StatusInternalServerError, InternalErrorCode, "Failed to upgrade connection")
		return
	}
	h.hub.Register(chatID, userID, conn)
	defer h.hub.Unregister(chatID, userID, conn)

	for {
		var incoming sendMessageRequest
		if err := conn.ReadJSON(&incoming); err != nil {
			log.Printf("[chat_stream] read failed for chat %d user %d: %v", chatID, userID, err)
			break
		}
		msg, unreadByUser, err := h.service.SendMessage(chatID, userID, incoming.Text, incoming.Attachments)
		if err != nil {
			log.Printf("[chat_stream] failed to persist message for chat %d user %d: %v", chatID, userID, err)
			_ = conn.WriteJSON(APIError{ErrorCode: InternalErrorCode, Message: "Failed to send message"})
			continue
		}
		h.hub.Broadcast(msg)
		for uid, unread := range unreadByUser {
			h.hub.NotifyUnread(chatID, uid, unread)
		}
	}
}
