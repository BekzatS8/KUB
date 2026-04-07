package handlers

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/realtime"
	"turcompany/internal/services"
)

type ChatHandler struct {
	service *services.ChatService
	hub     *realtime.ChatHub
}

// ✅ text больше НЕ required — можно отправлять только attachments
type sendMessageRequest struct {
	Text          string   `json:"text"`
	Attachments   []string `json:"attachments"`
	AttachmentIDs []string `json:"attachment_ids"`
}

type personalChatRequest struct {
	UserID int `json:"user_id" binding:"required"`
}

type groupChatRequest struct {
	Name      string `json:"name" binding:"required"`
	Members   []int  `json:"members"`
	MemberIDs []int  `json:"member_ids"` // ✅ алиас, чтобы не ломаться если фронт/постман шлёт member_ids
}

type addMembersRequest struct {
	Members   []int `json:"members"`
	MemberIDs []int `json:"member_ids"` // ✅ алиас
}

type markReadRequest struct {
	MessageID *int `json:"message_id"`
}

type editMessageRequest struct {
	Text string `json:"text"`
}

func NewChatHandler(service *services.ChatService, hub *realtime.ChatHub) *ChatHandler {
	return &ChatHandler{service: service, hub: hub}
}

func ensureCanUseChat(c *gin.Context, roleID int) bool {
	if !authz.CanUseChat(roleID) {
		forbidden(c, "Chat is not allowed for this role")
		return false
	}
	return true
}

func (h *ChatHandler) ListChats(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	chats, err := h.service.ListUserChats(userID)
	if err != nil {
		internalError(c, "Failed to load chats")
		return
	}
	c.JSON(http.StatusOK, chats)
}

func (h *ChatHandler) SearchChats(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}

	q := strings.TrimSpace(c.Query("query"))
	if q == "" {
		q = strings.TrimSpace(c.Query("q")) // ✅ алиас
	}
	if q == "" {
		badRequest(c, "Query is required")
		return
	}

	chats, err := h.service.SearchChats(userID, q)
	if err != nil {
		internalError(c, "Failed to search chats")
		return
	}
	c.JSON(http.StatusOK, listWithCount(chats))
}

func (h *ChatHandler) ListChatDirectoryUsers(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}

	query := strings.TrimSpace(c.Query("query"))
	if query == "" {
		query = strings.TrimSpace(c.Query("q"))
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	items, total, err := h.service.ListDirectoryUsers(userID, query, limit, offset)
	if err != nil {
		internalError(c, "Failed to load chat directory users")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"value": ensureNonNilSlice(items),
		"count": total,
	})
}

func (h *ChatHandler) CreatePersonalChat(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	if !authz.CanStartPersonalChat(roleID) {
		forbidden(c, "Personal chat creation is not allowed")
		return
	}

	var req personalChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}

	chat, err := h.service.CreatePersonalChat(userID, req.UserID)
	if err != nil {
		writeChatError(c, err, "Failed to create personal chat")
		return
	}
	c.JSON(http.StatusCreated, chat)
}

func (h *ChatHandler) CreateGroupChat(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	if !authz.CanCreateChatGroup(roleID) {
		forbidden(c, "Group chat creation is not allowed")
		return
	}

	var req groupChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}

	// ✅ поддержка member_ids
	if len(req.Members) == 0 && len(req.MemberIDs) > 0 {
		req.Members = req.MemberIDs
	}

	chat, err := h.service.CreateGroupChat(req.Name, userID, req.Members)
	if err != nil {
		writeChatError(c, err, "Failed to create group chat")
		return
	}
	c.JSON(http.StatusCreated, chat)
}

func (h *ChatHandler) GetChatInfo(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}

	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}

	info, err := h.service.GetChatInfo(chatID, userID)
	if err != nil {
		writeChatError(c, err, "Failed to load chat info")
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *ChatHandler) ListMessages(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}

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

	includeAttachments := c.Query("include_attachments") == "1"
	if includeAttachments {
		messages, attached, err := h.service.GetMessagesWithAttachments(chatID, userID, limit, offset)
		if err != nil {
			writeChatError(c, err, "Failed to load chat messages")
			return
		}
		resp := buildMessagesWithAttachmentsResponse(messages, attached)
		c.JSON(http.StatusOK, resp)
		return
	}

	messages, err := h.service.GetMessages(chatID, userID, limit, offset)
	if err != nil {
		writeChatError(c, err, "Failed to load chat messages")
		return
	}
	c.JSON(http.StatusOK, listWithCount(messages))
}

func (h *ChatHandler) SearchMessages(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}

	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}

	q := strings.TrimSpace(c.Query("query"))
	if q == "" {
		q = strings.TrimSpace(c.Query("q")) // ✅ алиас
	}
	if q == "" {
		badRequest(c, "Query is required")
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	mode := strings.TrimSpace(c.DefaultQuery("mode", "fts"))

	messages, err := h.service.SearchMessages(chatID, userID, q, mode, limit, offset)
	if err != nil {
		writeChatError(c, err, "Failed to search messages")
		return
	}
	c.JSON(http.StatusOK, listWithCount(messages))
}

func (h *ChatHandler) SendMessage(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	if !authz.CanSendChatMessage(roleID) {
		forbidden(c, "Message sending is not allowed")
		return
	}
	if !authz.CanWriteChat(roleID) {
		forbidden(c, "Chat write is not allowed")
		return
	}

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

	if err := validateSendMessagePayload(&req); err != nil {
		writeChatError(c, err, "Invalid message payload")
		return
	}

	msg, unreadByUser, err := h.service.SendMessage(chatID, userID, req.Text, req.Attachments, req.AttachmentIDs)
	if err != nil {
		writeChatError(c, err, "Failed to send message")
		return
	}

	h.hub.Broadcast(msg)
	for uid, unread := range unreadByUser {
		h.hub.NotifyUnread(chatID, uid, unread)
	}
	c.JSON(http.StatusCreated, msg)
}

func (h *ChatHandler) UploadAttachment(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}

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

	uploaded, err := h.service.UploadAttachment(chatID, userID, file)
	if err != nil {
		writeChatError(c, err, "Failed to upload attachment")
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": uploaded.ID, "url": uploaded.URL, "file_name": uploaded.FileName, "mime_type": uploaded.MimeType, "size_bytes": uploaded.SizeBytes})
}

func (h *ChatHandler) UploadAttachmentAlias(c *gin.Context) {
	h.UploadAttachment(c)
}

func (h *ChatHandler) DownloadAttachment(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	attachmentID := strings.TrimSpace(c.Param("id"))
	if attachmentID == "" {
		badRequest(c, "Invalid attachment id")
		return
	}
	att, reader, _, err := h.service.DownloadAttachment(attachmentID, userID)
	if err != nil {
		switch err {
		case services.ErrNotChatMember:
			forbidden(c, "Not a chat member")
			return
		case services.ErrChatNotFound:
			notFound(c, NotFoundCode, "Attachment not found")
			return
		default:
			internalError(c, "Failed to download attachment")
			return
		}
	}
	defer reader.Close()
	c.Header("Content-Type", att.MimeType)
	c.Header("Content-Disposition", "attachment; filename=\""+att.FileName+"\"")
	http.ServeContent(c.Writer, c.Request, att.FileName, att.CreatedAt, reader)
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

	// ✅ поддержка member_ids
	members := req.Members
	if len(members) == 0 && len(req.MemberIDs) > 0 {
		members = req.MemberIDs
	}
	if len(members) == 0 {
		badRequest(c, "Members are required")
		return
	}

	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	if err := h.service.AddMembers(chatID, userID, members); err != nil {
		writeChatError(c, err, "Failed to add members")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ChatHandler) LeaveChat(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}

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
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}

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
			forbidden(c, "Only owner/admin can delete group chat")
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
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}

	chats, err := h.service.ListUnreadChats(userID)
	if err != nil {
		internalError(c, "Failed to load unread chats")
		return
	}
	c.JSON(http.StatusOK, chats)
}

func (h *ChatHandler) MarkRead(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	if !authz.CanMarkReadChat(roleID) {
		forbidden(c, "Mark read is not allowed")
		return
	}

	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}

	var req markReadRequest
	if len(c.Request.Header.Get("Content-Type")) > 0 && c.ContentType() == "application/json" {
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			badRequest(c, "Invalid payload")
			return
		}
	}
	if req.MessageID != nil && *req.MessageID <= 0 {
		badRequest(c, "message_id must be greater than 0")
		return
	}

	unread, evt, err := h.service.MarkChatRead(chatID, userID, req.MessageID)
	if err != nil {
		writeChatError(c, err, "Failed to mark chat as read")
		return
	}

	h.hub.NotifyUnread(chatID, userID, unread)
	if evt != nil {
		h.hub.NotifyRead(*evt)
	}
	c.Status(http.StatusNoContent)
}

func (h *ChatHandler) EditMessage(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	messageID, err := strconv.Atoi(c.Param("message_id"))
	if err != nil {
		badRequest(c, "Invalid message id")
		return
	}
	var req editMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	msg, err := h.service.EditMessage(chatID, messageID, userID, req.Text)
	if err != nil {
		switch err {
		case services.ErrForbidden:
			forbidden(c, "Forbidden")
			return
		default:
			internalError(c, "Failed to edit message")
			return
		}
	}
	h.hub.NotifyMessageUpdated(chatID, msg)
	c.JSON(http.StatusOK, msg)
}

func (h *ChatHandler) DeleteMessage(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	messageID, err := strconv.Atoi(c.Param("message_id"))
	if err != nil {
		badRequest(c, "Invalid message id")
		return
	}
	msg, err := h.service.DeleteMessage(chatID, messageID, userID)
	if err != nil {
		switch err {
		case services.ErrForbidden:
			forbidden(c, "Forbidden")
			return
		default:
			internalError(c, "Failed to delete message")
			return
		}
	}
	deletedBy := 0
	if msg.DeletedBy != nil {
		deletedBy = *msg.DeletedBy
	}
	deletedAt := time.Now()
	if msg.DeletedAt != nil {
		deletedAt = *msg.DeletedAt
	}
	h.hub.NotifyMessageDeleted(chatID, messageID, deletedBy, deletedAt)
	c.Status(http.StatusNoContent)
}

func (h *ChatHandler) PinMessage(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	messageID, err := strconv.Atoi(c.Param("message_id"))
	if err != nil {
		badRequest(c, "Invalid message id")
		return
	}
	pin, err := h.service.PinMessage(chatID, messageID, userID)
	if err != nil {
		if err == services.ErrForbidden {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to pin message")
		return
	}
	h.hub.NotifyMessagePinned(chatID, pin)
	c.JSON(http.StatusOK, pin)
}

func (h *ChatHandler) UnpinMessage(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	messageID, err := strconv.Atoi(c.Param("message_id"))
	if err != nil {
		badRequest(c, "Invalid message id")
		return
	}
	if err := h.service.UnpinMessage(chatID, messageID, userID); err != nil {
		if err == services.ErrForbidden {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to unpin message")
		return
	}
	h.hub.NotifyMessageUnpinned(chatID, messageID)
	c.Status(http.StatusNoContent)
}

func (h *ChatHandler) FavoriteMessage(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	messageID, err := strconv.Atoi(c.Param("message_id"))
	if err != nil {
		badRequest(c, "Invalid message id")
		return
	}
	fav, err := h.service.FavoriteMessage(chatID, messageID, userID)
	if err != nil {
		internalError(c, "Failed to favorite message")
		return
	}
	c.JSON(http.StatusOK, fav)
}

func (h *ChatHandler) UnfavoriteMessage(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	messageID, err := strconv.Atoi(c.Param("message_id"))
	if err != nil {
		badRequest(c, "Invalid message id")
		return
	}
	if err := h.service.UnfavoriteMessage(chatID, messageID, userID); err != nil {
		internalError(c, "Failed to unfavorite message")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ChatHandler) ListPins(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	pins, err := h.service.ListPins(chatID, userID, limit, offset)
	if err != nil {
		internalError(c, "Failed to list pins")
		return
	}
	c.JSON(http.StatusOK, listWithCount(pins))
}

func (h *ChatHandler) ListFavorites(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if !ensureCanUseChat(c, roleID) {
		return
	}
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "Invalid chat id")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	favs, err := h.service.ListFavorites(chatID, userID, limit, offset)
	if err != nil {
		internalError(c, "Failed to list favorites")
		return
	}
	c.JSON(http.StatusOK, listWithCount(favs))
}

func ensureNonNilSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func listWithCount[T any](items []T) gin.H {
	items = ensureNonNilSlice(items)
	return gin.H{"value": items, "Count": len(items)}
}

func validateSendMessagePayload(req *sendMessageRequest) error {
	if req == nil {
		return services.ErrInvalidChatPayload
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" && len(req.Attachments) == 0 {
		return services.ErrInvalidChatPayload
	}
	return nil
}

func mapChatError(err error, fallbackMsg string) (int, string, string) {
	switch {
	case errors.Is(err, services.ErrNotChatMember):
		return http.StatusForbidden, ChatNotMemberCode, "Not a chat member"
	case errors.Is(err, services.ErrChatForbidden), errors.Is(err, services.ErrForbidden):
		return http.StatusForbidden, ChatForbiddenCode, "Forbidden"
	case errors.Is(err, services.ErrChatNotFound):
		return http.StatusNotFound, ChatNotFoundCode, "Chat not found"
	case errors.Is(err, services.ErrChatUserNotFound):
		return http.StatusNotFound, ChatUserNotFoundCode, "Target user not found"
	case errors.Is(err, services.ErrChatUserInactive):
		return http.StatusBadRequest, ChatUserInactiveCode, "Target user is inactive or not verified"
	case errors.Is(err, services.ErrDirectChatWithSelf):
		return http.StatusBadRequest, DirectChatWithSelfCode, "Cannot create direct chat with yourself"
	case errors.Is(err, services.ErrInvalidChatPayload), errors.Is(err, services.ErrGroupChatNameRequired):
		return http.StatusBadRequest, ChatInvalidPayloadCode, "Invalid chat payload"
	case errors.Is(err, services.ErrPersonalChatAlreadyExists):
		return http.StatusConflict, ChatConflictCode, "Personal chat already exists"
	default:
		return http.StatusInternalServerError, InternalErrorCode, fallbackMsg
	}
}

func writeChatError(c *gin.Context, err error, fallbackMsg string) {
	status, code, message := mapChatError(err, fallbackMsg)
	writeError(c, status, code, message)
}

func buildMessagesWithAttachmentsResponse(messages []*models.ChatMessage, attached map[int][]models.AttachmentResponse) []gin.H {
	messages = ensureNonNilSlice(messages)
	resp := make([]gin.H, 0, len(messages))
	for _, m := range messages {
		attachments := ensureNonNilSlice(attached[m.ID])
		if m.IsDeleted {
			attachments = []models.AttachmentResponse{}
		}
		resp = append(resp, gin.H{
			"id":          m.ID,
			"chat_id":     m.ChatID,
			"sender_id":   m.SenderID,
			"text":        m.Text,
			"attachments": attachments,
			"created_at":  m.CreatedAt,
		})
	}
	return resp
}

func (h *ChatHandler) Stream(c *gin.Context) {
	userID, roleID := getUserAndRole(c)

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

		if !authz.CanSendChatMessage(roleID) {
			_ = conn.WriteJSON(APIError{ErrorCode: ChatForbiddenCode, Message: "Message sending is not allowed"})
			continue
		}
		if !authz.CanWriteChat(roleID) {
			_ = conn.WriteJSON(APIError{ErrorCode: ChatForbiddenCode, Message: "Chat write is not allowed"})
			continue
		}
		if err := validateSendMessagePayload(&incoming); err != nil {
			_ = conn.WriteJSON(APIError{ErrorCode: ChatInvalidPayloadCode, Message: "Message text or attachments are required"})
			continue
		}

		msg, unreadByUser, err := h.service.SendMessage(chatID, userID, incoming.Text, incoming.Attachments, incoming.AttachmentIDs)
		if err != nil {
			log.Printf("[chat_stream] failed to persist message for chat %d user %d: %v", chatID, userID, err)
			status, code, message := mapChatError(err, "Failed to send message")
			if status >= 500 {
				message = "Failed to send message"
			}
			_ = conn.WriteJSON(APIError{ErrorCode: code, Message: message})
			continue
		}

		h.hub.Broadcast(msg)
		for uid, unread := range unreadByUser {
			h.hub.NotifyUnread(chatID, uid, unread)
		}
	}
}
