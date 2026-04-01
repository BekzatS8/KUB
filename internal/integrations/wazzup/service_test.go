package wazzup

import (
	"context"
	"errors"
	"testing"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type stubRepo struct {
	integration *models.WazzupIntegration
}

func (s stubRepo) GetIntegrationByToken(context.Context, string) (*models.WazzupIntegration, error) {
	return s.integration, nil
}
func (s stubRepo) ListCRMUsers(context.Context) ([]repositories.CRMUserDTO, error) { return nil, nil }
func (s stubRepo) GetCRMUserByID(context.Context, int) (*repositories.CRMUserDTO, error) {
	return nil, nil
}
func (s stubRepo) GetIntegrationByOwnerUserID(context.Context, int) (*models.WazzupIntegration, error) {
	return s.integration, nil
}
func (s stubRepo) UpsertIntegrationByOwner(context.Context, int, string, string, string, bool) (int, string, error) {
	return 1, "tok", nil
}
func (s stubRepo) RegisterDedup(context.Context, int, string) (bool, error) { return true, nil }
func (s stubRepo) FindLeadByPhone(context.Context, string) (int, error)     { return 0, nil }
func (s stubRepo) CreateLeadFromInbound(context.Context, int, string, string) (int, error) {
	return 123, nil
}
func (s stubRepo) UpdateLeadDescriptionIfEmpty(context.Context, int, string) error { return nil }
func (s stubRepo) GetLeadPhoneByID(context.Context, int) (string, error)           { return "", nil }
func (s stubRepo) GetClientPhoneByID(context.Context, int) (string, error)         { return "", nil }

type noopClient struct{}

func (noopClient) PatchWebhooks(context.Context, string, string, string) error { return nil }
func (noopClient) UpsertUsers(context.Context, string, []UserUpsert) error     { return nil }
func (noopClient) CreateIframe(context.Context, string, CreateIframeRequest) (string, error) {
	return "", nil
}
func (noopClient) SendMessage(context.Context, string, SendMessageRequest) (*SendMessageResponse, error) {
	return &SendMessageResponse{MessageID: "ok"}, nil
}

func TestHandleWebhookRejectsBadPayload(t *testing.T) {
	svc := NewService(stubRepo{integration: &models.WazzupIntegration{ID: 1, Enabled: true}}, noopClient{}, "tok", "", "", "")
	_, _, err := svc.HandleWebhook(context.Background(), "abc", "", []byte("{"))
	if !errors.Is(err, ErrBadPayload) {
		t.Fatalf("expected ErrBadPayload, got %v", err)
	}
}

func TestHandleWebhookVerifyToken(t *testing.T) {
	svc := NewService(stubRepo{integration: &models.WazzupIntegration{ID: 1, Enabled: true}}, noopClient{}, "tok", "", "verify", "")
	_, _, err := svc.HandleWebhook(context.Background(), "abc", "Bearer wrong", []byte(`{"messages":[]}`))
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}
