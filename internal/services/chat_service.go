package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/storage"
)

// ChatService handles read/send operations for chats without realtime transport.
type ChatService struct {
	repo      repositories.ChatRepository
	userRepo  repositories.UserRepository
	filesRoot string
	storage   storage.Storage
}

func NewChatService(repo repositories.ChatRepository, filesRoot string, userRepo repositories.UserRepository) *ChatService {
	return &ChatService{repo: repo, userRepo: userRepo, filesRoot: filesRoot, storage: storage.NewLocalStorage(filesRoot)}
}

func (s *ChatService) ListUserChats(userID int) ([]*models.Chat, error) {
	chats, err := s.repo.ListUserChats(userID)
	if err != nil {
		return nil, err
	}
	if err := s.attachMemberStatuses(chats, userID); err != nil {
		return nil, err
	}
	if err := s.attachChatProfiles(chats, userID); err != nil {
		return nil, err
	}
	return chats, nil
}

func (s *ChatService) SearchChats(userID int, query string) ([]*models.Chat, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []*models.Chat{}, nil
	}
	chats, err := s.repo.SearchChats(userID, query)
	if err != nil {
		return nil, err
	}
	if err := s.attachMemberStatuses(chats, userID); err != nil {
		return nil, err
	}
	if err := s.attachChatProfiles(chats, userID); err != nil {
		return nil, err
	}
	return chats, nil
}

func (s *ChatService) ListDirectoryUsers(viewerUserID int, query string, limit, offset int) ([]*models.ChatUserDirectoryItem, int, error) {
	query = strings.TrimSpace(query)
	items, total, err := s.repo.ListChatDirectoryUsers(viewerUserID, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	// defense-in-depth: even if repository filter changes, never return current user in picker.
	filtered := make([]*models.ChatUserDirectoryItem, 0, len(items))
	for _, item := range items {
		if item == nil || item.UserID == viewerUserID {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, total, nil
}

func (s *ChatService) GetMessages(chatID, userID, limit, offset int) ([]*models.ChatMessage, error) {
	if err := s.ensureMember(chatID, userID); err != nil {
		return nil, err
	}
	messages, err := s.repo.ListMessages(chatID, limit, offset)
	if err != nil {
		return nil, err
	}
	if len(messages) > 0 {
		lastID := messages[len(messages)-1].ID
		if err := s.repo.UpdateLastRead(chatID, userID, lastID); err != nil {
			return nil, err
		}
	}
	if err := s.attachSenderProfiles(messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *ChatService) GetMessagesWithAttachments(chatID, userID, limit, offset int) ([]*models.ChatMessage, map[int][]models.AttachmentResponse, error) {
	messages, err := s.GetMessages(chatID, userID, limit, offset)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int, 0, len(messages))
	for _, m := range messages {
		ids = append(ids, m.ID)
	}
	attached, err := s.repo.GetAttachmentsByMessageIDs(ids)
	if err != nil {
		return nil, nil, err
	}
	return messages, attached, nil
}

func (s *ChatService) SearchMessages(chatID, userID int, query, mode string, limit, offset int) ([]*models.ChatMessage, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []*models.ChatMessage{}, nil
	}
	if err := s.ensureMember(chatID, userID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "ilike" {
		messages, err := s.repo.SearchMessagesILIKE(chatID, userID, query, limit, offset)
		if err != nil {
			return nil, err
		}
		if err := s.attachSenderProfiles(messages); err != nil {
			return nil, err
		}
		return messages, nil
	}
	messages, err := s.repo.SearchMessagesFTS(chatID, userID, query, limit, offset)
	if err != nil {
		return nil, err
	}
	if err := s.attachSenderProfiles(messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *ChatService) SendMessage(chatID, senderID int, text string, attachments []string, attachmentIDs []string) (*models.ChatMessage, map[int]int, error) {
	text = strings.TrimSpace(text)
	attachments = normalizeAttachments(attachments)

	if text == "" && len(attachments) == 0 && len(attachmentIDs) == 0 {
		return nil, nil, ErrInvalidChatPayload
	}

	if err := s.ensureMember(chatID, senderID); err != nil {
		return nil, nil, err
	}
	if err := s.ensureActiveUser(senderID); err != nil {
		return nil, nil, err
	}

	msg, err := s.repo.CreateMessage(chatID, senderID, text, attachments)
	if err != nil {
		return nil, nil, err
	}
	if len(attachmentIDs) > 0 {
		if err := s.repo.AttachToMessage(uniqueStrings(attachmentIDs), msg.ID, chatID, senderID); err != nil {
			return nil, nil, fmt.Errorf("attach to message: %w", err)
		}
	}

	chat, err := s.repo.GetChatByID(chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrChatNotFound
		}
		return nil, nil, err
	}

	unreadByUser := make(map[int]int)
	if err := s.repo.UpdateLastRead(chatID, senderID, msg.ID); err != nil {
		return nil, nil, err
	}
	for _, member := range chat.Members {
		if member == senderID {
			continue
		}
		count, err := s.repo.CountUnread(chatID, member)
		if err != nil {
			return nil, nil, err
		}
		unreadByUser[member] = count
	}
	if err := s.attachSenderProfiles([]*models.ChatMessage{msg}); err != nil {
		return nil, nil, err
	}

	return msg, unreadByUser, nil
}

func (s *ChatService) UploadAttachment(chatID, userID int, file *multipart.FileHeader) (*models.AttachmentResponse, error) {
	if err := s.ensureMember(chatID, userID); err != nil {
		return nil, err
	}
	if file == nil {
		return nil, fmt.Errorf("file is required")
	}
	if file.Size > 10*1024*1024 {
		return nil, fmt.Errorf("file is too large")
	}

	safeName, ext, err := sanitizeAttachmentName(file.Filename)
	if err != nil {
		return nil, err
	}
	mimeType, ok := allowedAttachmentTypes()[ext]
	if !ok {
		return nil, fmt.Errorf("file type not allowed")
	}

	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	key := fmt.Sprintf("chat/%d/%d_%s", chatID, time.Now().UnixNano(), safeName)
	if err := s.storage.Save(context.Background(), src, key); err != nil {
		return nil, err
	}
	att, err := s.repo.CreateAttachment(chatID, userID, safeName, mimeType, file.Size, key)
	if err != nil {
		_ = s.storage.Delete(context.Background(), key)
		return nil, err
	}
	return &models.AttachmentResponse{ID: att.ID, URL: "/attachments/" + att.ID + "/download", FileName: att.FileName, MimeType: att.MimeType, SizeBytes: att.SizeBytes}, nil
}

func (s *ChatService) DownloadAttachment(attachmentID string, userID int) (*models.Attachment, io.ReadSeekCloser, int64, error) {
	att, err := s.repo.GetAttachmentForDownload(attachmentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, 0, ErrChatNotFound
		}
		return nil, nil, 0, err
	}
	if err := s.ensureMember(att.ChatID, userID); err != nil {
		return nil, nil, 0, err
	}
	r, size, err := s.storage.Open(context.Background(), att.StorageKey)
	if err != nil {
		return nil, nil, 0, err
	}
	return att, r, size, nil
}

func (s *ChatService) CreatePersonalChat(user1, user2 int) (*models.Chat, error) {
	if user1 == user2 {
		return nil, ErrDirectChatWithSelf
	}
	if err := s.ensureActiveUser(user1); err != nil {
		return nil, err
	}
	if err := s.ensureTargetUserForPersonalChat(user2); err != nil {
		return nil, err
	}
	existing, err := s.repo.FindPersonalChat(user1, user2)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	members := uniqueInts([]int{user1, user2})
	return s.repo.CreateChat("", false, user1, members)
}

func (s *ChatService) CreateGroupChat(name string, creatorID int, members []int) (*models.Chat, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrGroupChatNameRequired
	}
	members = append(members, creatorID)
	members = uniqueInts(members)
	for _, memberID := range members {
		if err := s.ensureActiveUser(memberID); err != nil {
			return nil, err
		}
	}
	return s.repo.CreateChat(name, true, creatorID, members)
}

func (s *ChatService) LeaveChat(chatID, userID int) error {
	if err := s.ensureMember(chatID, userID); err != nil {
		return err
	}
	return s.repo.RemoveMember(chatID, userID)
}

func contains(slice []int, v int) bool {
	for _, x := range slice {
		if x == v {
			return true
		}
	}
	return false
}

func (s *ChatService) AddMembers(chatID, userID int, memberIDs []int) error {
	if len(memberIDs) == 0 {
		return ErrInvalidChatPayload
	}

	chat, err := s.repo.GetChatByID(chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChatNotFound
		}
		return err
	}

	// проверяем, что userID – участник чата
	if !contains(chat.Members, userID) {
		return ErrChatForbidden
	}

	uniq := uniqueInts(memberIDs)
	for _, memberID := range uniq {
		if err := s.ensureActiveUser(memberID); err != nil {
			return err
		}
	}
	return s.repo.AddMembers(chatID, uniq)
}

func (s *ChatService) DeleteChat(chatID, userID int) error {
	chat, err := s.repo.GetChatByID(chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChatNotFound
		}
		return err
	}

	if !chat.IsGroup {
		if err := s.ensureMember(chatID, userID); err != nil {
			return err
		}
		return s.repo.DeleteChat(chatID)
	}

	role, err := s.repo.GetMemberRole(chatID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotChatMember
		}
		return err
	}
	if role != models.ChatMemberRoleOwner && role != models.ChatMemberRoleAdmin {
		return ErrChatForbidden
	}
	return s.repo.DeleteChat(chatID)
}

func (s *ChatService) ensureMember(chatID, userID int) error {
	ok, err := s.repo.IsMember(chatID, userID)
	if err != nil {
		return err
	}
	if !ok {
		if _, err := s.repo.GetChatByID(chatID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatNotFound
			}
			return err
		}
		return ErrNotChatMember
	}
	return nil
}

func (s *ChatService) GetChatInfo(chatID, userID int) (*models.ChatInfoResponse, error) {
	info, err := s.repo.GetChatInfo(chatID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if _, chatErr := s.repo.GetChatByID(chatID); chatErr != nil {
				if errors.Is(chatErr, sql.ErrNoRows) {
					return nil, ErrChatNotFound
				}
				return nil, chatErr
			}
			return nil, ErrNotChatMember
		}
		return nil, err
	}
	return info, nil
}

func (s *ChatService) GetUserStatus(userID int) (bool, time.Time, error) {
	return s.repo.GetOnlineStatus(userID)
}

func (s *ChatService) EnsureMember(chatID, userID int) error {
	return s.ensureMember(chatID, userID)
}

func (s *ChatService) MarkChatRead(chatID, userID int, messageID *int) (int, *models.ChatReadEvent, error) {
	if err := s.ensureMember(chatID, userID); err != nil {
		return 0, nil, err
	}
	lastReadID, readAt, err := s.repo.MarkChatRead(chatID, userID, messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil, ErrChatNotFound
		}
		return 0, nil, err
	}
	count, err := s.repo.CountUnread(chatID, userID)
	if err != nil {
		return 0, nil, err
	}
	evt := &models.ChatReadEvent{Type: "chat:read", ChatID: chatID, UserID: userID, LastReadMessageID: lastReadID, ReadAt: readAt}
	return count, evt, nil
}

func (s *ChatService) ListUnreadChats(userID int) ([]*models.Chat, error) {
	chats, err := s.repo.ListUserChats(userID)
	if err != nil {
		return nil, err
	}

	var res []*models.Chat
	for _, ch := range chats {
		if ch.UnreadCount > 0 {
			res = append(res, ch)
		}
	}

	if err := s.attachMemberStatuses(res, userID); err != nil {
		return nil, err
	}
	if err := s.attachChatProfiles(res, userID); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *ChatService) attachMemberStatuses(chats []*models.Chat, currentUserID int) error {
	for _, chat := range chats {
		var statuses []models.UserStatus
		var latest time.Time
		online := false
		for _, member := range chat.Members {
			isOnline, lastSeen, err := s.repo.GetOnlineStatus(member)
			if err != nil {
				return err
			}
			statuses = append(statuses, models.UserStatus{UserID: member, IsOnline: isOnline, LastSeen: lastSeen})
			if member != currentUserID && isOnline {
				online = true
			}
			if lastSeen.After(latest) {
				latest = lastSeen
			}
		}
		chat.MemberStatuses = statuses
		chat.Online = online
		chat.LastSeen = latest
	}
	return nil
}

func (s *ChatService) attachChatProfiles(chats []*models.Chat, currentUserID int) error {
	if len(chats) == 0 {
		return nil
	}
	seen := map[int]struct{}{}
	userIDs := make([]int, 0)
	for _, chat := range chats {
		for _, memberID := range chat.Members {
			if _, ok := seen[memberID]; ok {
				continue
			}
			seen[memberID] = struct{}{}
			userIDs = append(userIDs, memberID)
		}
	}
	profiles, err := s.repo.GetChatVisibleProfiles(userIDs)
	if err != nil {
		return err
	}

	for _, chat := range chats {
		statusByUser := make(map[int]models.UserStatus, len(chat.MemberStatuses))
		for _, st := range chat.MemberStatuses {
			statusByUser[st.UserID] = st
		}

		memberProfiles := make([]models.ChatParticipantLite, 0, len(chat.Members))
		var counterparty *models.ChatParticipantLite
		for _, memberID := range chat.Members {
			p, ok := profiles[memberID]
			if !ok || p == nil {
				continue
			}
			item := models.ChatParticipantLite{
				UserID:      p.UserID,
				DisplayName: p.DisplayName,
				RoleCode:    p.RoleCode,
				RoleName:    p.RoleName,
				Email:       p.Email,
			}
			if st, ok := statusByUser[memberID]; ok {
				item.Online = st.IsOnline
				lastSeen := st.LastSeen
				item.LastSeen = &lastSeen
			}
			memberProfiles = append(memberProfiles, item)
			if !chat.IsGroup && memberID != currentUserID {
				cp := item
				counterparty = &cp
			}
		}

		chat.MemberProfiles = memberProfiles
		if chat.IsGroup {
			preview := make([]models.ChatParticipantLite, 0, 3)
			for _, item := range memberProfiles {
				if item.UserID == currentUserID {
					continue
				}
				preview = append(preview, item)
				if len(preview) == 3 {
					break
				}
			}
			chat.ParticipantsPreview = preview
			chat.Counterparty = nil
		} else {
			chat.Counterparty = counterparty
			chat.ParticipantsPreview = nil
		}
	}
	return nil
}

func uniqueInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	var result []int
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

func normalizeAttachments(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *ChatService) EditMessage(chatID, messageID, userID int, text string) (*models.ChatMessage, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrInvalidChatPayload
	}
	msg, err := s.repo.EditMessage(chatID, messageID, userID, text)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrChatForbidden
		}
		return nil, err
	}
	return msg, nil
}

func (s *ChatService) DeleteMessage(chatID, messageID, userID int) (*models.ChatMessage, error) {
	msg, err := s.repo.DeleteMessage(chatID, messageID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrChatForbidden
		}
		return nil, err
	}
	return msg, nil
}

func (s *ChatService) PinMessage(chatID, messageID, userID int) (*models.PinResponse, error) {
	p, err := s.repo.PinMessage(chatID, messageID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrChatForbidden
		}
		return nil, err
	}
	return p, nil
}

func (s *ChatService) UnpinMessage(chatID, messageID, userID int) error {
	if err := s.repo.UnpinMessage(chatID, messageID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChatForbidden
		}
		return err
	}
	return nil
}

func (s *ChatService) FavoriteMessage(chatID, messageID, userID int) (*models.FavoriteResponse, error) {
	return s.repo.FavoriteMessage(chatID, messageID, userID)
}

func (s *ChatService) UnfavoriteMessage(chatID, messageID, userID int) error {
	return s.repo.UnfavoriteMessage(chatID, messageID, userID)
}

func (s *ChatService) ListPins(chatID, userID, limit, offset int) ([]*models.PinResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListPins(chatID, userID, limit, offset)
}

func (s *ChatService) ListFavorites(chatID, userID, limit, offset int) ([]*models.FavoriteResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListFavorites(chatID, userID, limit, offset)
}

func allowedAttachmentTypes() map[string]string {
	return map[string]string{
		".pdf":  "application/pdf",
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}
}

var badNameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeAttachmentName(name string) (string, string, error) {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "" || base == "." {
		return "", "", fmt.Errorf("invalid filename")
	}
	if strings.Count(base, ".") > 1 {
		return "", "", fmt.Errorf("invalid filename")
	}
	ext := strings.ToLower(filepath.Ext(base))
	if ext == "" {
		return "", "", fmt.Errorf("invalid filename")
	}
	clean := badNameChars.ReplaceAllString(base, "_")
	return clean, ext, nil
}

func uniqueStrings(v []string) []string {
	seen := make(map[string]struct{}, len(v))
	res := make([]string, 0, len(v))
	for _, s := range v {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		res = append(res, s)
	}
	return res
}

func (s *ChatService) ensureActiveUser(userID int) error {
	if s.userRepo == nil {
		return nil
	}
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil || !user.IsVerified {
		if user == nil {
			return ErrChatUserNotFound
		}
		return ErrChatUserInactive
	}
	return nil
}

func (s *ChatService) ensureTargetUserForPersonalChat(userID int) error {
	if s.userRepo == nil {
		return nil
	}
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTargetUserNotFound
		}
		return err
	}
	if user == nil {
		return ErrTargetUserNotFound
	}
	if !user.IsVerified {
		return ErrTargetUserNotVerified
	}
	return nil
}

func (s *ChatService) attachSenderProfiles(messages []*models.ChatMessage) error {
	if len(messages) == 0 {
		return nil
	}
	userIDs := make([]int, 0, len(messages))
	seen := map[int]struct{}{}
	for _, msg := range messages {
		if _, ok := seen[msg.SenderID]; ok {
			continue
		}
		seen[msg.SenderID] = struct{}{}
		userIDs = append(userIDs, msg.SenderID)
	}
	profiles, err := s.repo.GetChatVisibleProfiles(userIDs)
	if err != nil {
		return err
	}
	for _, msg := range messages {
		msg.SenderProfile = profiles[msg.SenderID]
	}
	return nil
}
