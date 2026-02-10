package services

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"turcompany/internal/repositories"
)

type telegramLinkRepoStub struct {
	links           map[string]*repositories.TelegramLink
	attachCalls     int
	lastAttachCode  string
	lastAttachChat  int64
	createLinkCalls int
}

func (s *telegramLinkRepoStub) CreateLink(ctx context.Context, userID int, chatID int64, code string, expiresAt time.Time) (*repositories.TelegramLink, error) {
	s.createLinkCalls++
	return nil, nil
}

func (s *telegramLinkRepoStub) GetByCode(ctx context.Context, code string) (*repositories.TelegramLink, error) {
	if s.links == nil {
		return nil, nil
	}
	return s.links[code], nil
}

func (s *telegramLinkRepoStub) AttachChatID(ctx context.Context, code string, chatID int64) error {
	s.attachCalls++
	s.lastAttachCode = code
	s.lastAttachChat = chatID
	return nil
}

func (s *telegramLinkRepoStub) ConfirmLink(ctx context.Context, code string, userID int) (int64, error) {
	return 0, nil
}

func TestHandleUpdate_StartWithCode_AttachesChatID(t *testing.T) {
	repo := &telegramLinkRepoStub{links: map[string]*repositories.TelegramLink{
		"ABC123": {
			Code:      "ABC123",
			Used:      false,
			ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
			ChatID:    sql.NullInt64{Valid: false},
		},
	}}
	svc := &TelegramService{linkRepo: repo}

	err := svc.HandleUpdate(&TelegramUpdate{Message: &struct {
		MessageID int    "json:\"message_id\""
		Text      string "json:\"text\""
		Chat      struct {
			ID int64 "json:\"id\""
		} "json:\"chat\""
	}{Text: "/start abc123", Chat: struct {
		ID int64 "json:\"id\""
	}{ID: 12345}}})
	if err != nil {
		t.Fatalf("HandleUpdate error: %v", err)
	}
	if repo.attachCalls != 1 {
		t.Fatalf("expected attach call once, got %d", repo.attachCalls)
	}
	if repo.lastAttachCode != "ABC123" {
		t.Fatalf("expected normalized code ABC123, got %s", repo.lastAttachCode)
	}
	if repo.lastAttachChat != 12345 {
		t.Fatalf("expected chatID 12345, got %d", repo.lastAttachChat)
	}
}

func TestHandleUpdate_StartWithoutCode_DoesNotAttach(t *testing.T) {
	repo := &telegramLinkRepoStub{}
	svc := &TelegramService{linkRepo: repo}

	err := svc.HandleUpdate(&TelegramUpdate{Message: &struct {
		MessageID int    "json:\"message_id\""
		Text      string "json:\"text\""
		Chat      struct {
			ID int64 "json:\"id\""
		} "json:\"chat\""
	}{Text: "/start", Chat: struct {
		ID int64 "json:\"id\""
	}{ID: 12345}}})
	if err != nil {
		t.Fatalf("HandleUpdate error: %v", err)
	}
	if repo.attachCalls != 0 {
		t.Fatalf("expected no attach calls, got %d", repo.attachCalls)
	}
	if repo.createLinkCalls != 0 {
		t.Fatalf("expected no create link calls, got %d", repo.createLinkCalls)
	}
}
