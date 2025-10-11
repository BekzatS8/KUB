package services

import (
	"context"
	"time"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// MessageService defines the interface for message-related business logic.
type MessageService interface {
	Send(ctx context.Context, msg *models.Message) (*models.Message, error)
	GetConversationHistory(ctx context.Context, userID, partnerID int64) ([]models.Message, error)
	GetConversations(ctx context.Context, userID int64) ([]models.Conversation, error)
}

type messageService struct {
	repo repositories.MessageRepository
}

// NewMessageService creates a new instance of MessageService.
func NewMessageService(repo repositories.MessageRepository) MessageService {
	return &messageService{repo: repo}
}

func (s *messageService) Send(ctx context.Context, msg *models.Message) (*models.Message, error) {
	msg.SentAt = time.Now()
	if err := s.repo.Store(ctx, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (s *messageService) GetConversationHistory(ctx context.Context, userID, partnerID int64) ([]models.Message, error) {
	// Future logic: mark messages as read
	return s.repo.FindConversationHistory(ctx, userID, partnerID)
}

func (s *messageService) GetConversations(ctx context.Context, userID int64) ([]models.Conversation, error) {
	return s.repo.FindConversations(ctx, userID)
}
