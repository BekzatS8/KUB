package services

import (
	"errors"
	"fmt"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

var ErrNotChatMember = errors.New("user is not a member of this chat")

// ChatService handles read/send operations for chats without realtime transport.
type ChatService struct {
	repo repositories.ChatRepository
}

func NewChatService(repo repositories.ChatRepository) *ChatService {
	return &ChatService{repo: repo}
}

func (s *ChatService) ListUserChats(userID int) ([]*models.Chat, error) {
	return s.repo.ListUserChats(userID)
}

func (s *ChatService) GetMessages(chatID, userID, limit, offset int) ([]*models.ChatMessage, error) {
	if err := s.ensureMember(chatID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListMessages(chatID, limit, offset)
}

func (s *ChatService) SendMessage(chatID, senderID int, text string, attachments []string) (*models.ChatMessage, error) {
	if text == "" {
		return nil, fmt.Errorf("message text is required")
	}
	if err := s.ensureMember(chatID, senderID); err != nil {
		return nil, err
	}
	return s.repo.CreateMessage(chatID, senderID, text, attachments)
}

func (s *ChatService) ensureMember(chatID, userID int) error {
	ok, err := s.repo.IsMember(chatID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotChatMember
	}
	return nil
}

func (s *ChatService) EnsureMember(chatID, userID int) error {
	return s.ensureMember(chatID, userID)
}
