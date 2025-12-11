package services

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// ChatService handles read/send operations for chats without realtime transport.
type ChatService struct {
	repo      repositories.ChatRepository
	filesRoot string
}

func NewChatService(repo repositories.ChatRepository, filesRoot string) *ChatService {
	return &ChatService{repo: repo, filesRoot: filesRoot}
}

func (s *ChatService) ListUserChats(userID int) ([]*models.Chat, error) {
	chats, err := s.repo.ListUserChats(userID)
	if err != nil {
		return nil, err
	}
	if err := s.attachMemberStatuses(chats, userID); err != nil {
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
	return chats, nil
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
	return messages, nil
}

func (s *ChatService) SearchMessages(chatID, userID int, query string) ([]*models.ChatMessage, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []*models.ChatMessage{}, nil
	}
	if err := s.ensureMember(chatID, userID); err != nil {
		return nil, err
	}
	return s.repo.SearchMessages(chatID, userID, query)
}

func (s *ChatService) SendMessage(chatID, senderID int, text string, attachments []string) (*models.ChatMessage, map[int]int, error) {
	if text == "" {
		return nil, nil, fmt.Errorf("message text is required")
	}
	if err := s.ensureMember(chatID, senderID); err != nil {
		return nil, nil, err
	}
	msg, err := s.repo.CreateMessage(chatID, senderID, text, attachments)
	if err != nil {
		return nil, nil, err
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
	return msg, unreadByUser, nil
}

func (s *ChatService) UploadAttachment(chatID, userID int, file *multipart.FileHeader) (string, error) {
	if err := s.ensureMember(chatID, userID); err != nil {
		return "", err
	}
	if file == nil {
		return "", fmt.Errorf("file is required")
	}
	if file.Size > 10*1024*1024 {
		return "", fmt.Errorf("file is too large")
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowed := map[string]struct{}{
		".pdf":  {},
		".png":  {},
		".jpg":  {},
		".docx": {},
		".xlsx": {},
	}
	if _, ok := allowed[ext]; !ok {
		return "", fmt.Errorf("file type not allowed")
	}
	safeName := filepath.Base(file.Filename)
	if safeName == "" || safeName == "." {
		return "", fmt.Errorf("invalid filename")
	}

	destDir := filepath.Join(s.filesRoot, "messages")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	finalName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), safeName)
	destPath := filepath.Join(destDir, finalName)

	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	return path.Join("/files/messages", finalName), nil
}

func (s *ChatService) CreatePersonalChat(user1, user2 int) (*models.Chat, error) {
	if user1 == user2 {
		return nil, fmt.Errorf("cannot create personal chat with the same user")
	}
	members := uniqueInts([]int{user1, user2})
	return s.repo.CreateChat("", false, members)
}

func (s *ChatService) CreateGroupChat(name string, creatorID int, members []int) (*models.Chat, error) {
	if name == "" {
		return nil, fmt.Errorf("chat name is required")
	}
	members = append(members, creatorID)
	members = uniqueInts(members)
	return s.repo.CreateChat(name, true, members)
}

func (s *ChatService) LeaveChat(chatID, userID int) error {
	if err := s.ensureMember(chatID, userID); err != nil {
		return err
	}
	return s.repo.RemoveMember(chatID, userID)
}

func (s *ChatService) AddMembers(chatID int, memberIDs []int) error {
	if len(memberIDs) == 0 {
		return fmt.Errorf("no members to add")
	}
	if _, err := s.repo.GetChatByID(chatID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChatNotFound
		}
		return err
	}
	return s.repo.AddMembers(chatID, uniqueInts(memberIDs))
}

func (s *ChatService) DeleteChat(chatID, userID int) error {
	chat, err := s.repo.GetChatByID(chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChatNotFound
		}
		return err
	}
	if len(chat.Members) == 0 || chat.Members[0] != userID {
		return ErrForbidden
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

func (s *ChatService) GetUserStatus(userID int) (bool, time.Time, error) {
	return s.repo.GetOnlineStatus(userID)
}

func (s *ChatService) EnsureMember(chatID, userID int) error {
	return s.ensureMember(chatID, userID)
}

func (s *ChatService) MarkChatRead(chatID, userID int) (int, error) {
	if err := s.ensureMember(chatID, userID); err != nil {
		return 0, err
	}
	lastMsg, err := s.repo.LastMessage(chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if err := s.repo.UpdateLastRead(chatID, userID, 0); err != nil {
				return 0, err
			}
			return 0, nil
		}
		return 0, err
	}
	if err := s.repo.UpdateLastRead(chatID, userID, lastMsg.ID); err != nil {
		return 0, err
	}
	count, err := s.repo.CountUnread(chatID, userID)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *ChatService) ListUnreadChats(userID int) ([]*models.Chat, error) {
	chats, err := s.repo.ListUserChats(userID)
	if err != nil {
		return nil, err
	}
	if err := s.attachMemberStatuses(chats, userID); err != nil {
		return nil, err
	}
	return chats, nil
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
