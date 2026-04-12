package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type verifyRepoStub struct {
	confirmation *models.SignatureConfirmation
}

func (s *verifyRepoStub) CreatePending(context.Context, int64, int64, string, *string, *string, time.Time, []byte) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *verifyRepoStub) FindPending(context.Context, int64, int64, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *verifyRepoStub) FindPendingByTokenHash(context.Context, string, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *verifyRepoStub) FindByTokenHash(_ context.Context, channel, tokenHash string) (*models.SignatureConfirmation, error) {
	if s.confirmation != nil && s.confirmation.Channel == channel && s.confirmation.TokenHash != nil && *s.confirmation.TokenHash == tokenHash {
		return s.confirmation, nil
	}
	return nil, nil
}
func (s *verifyRepoStub) Approve(context.Context, string, []byte) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *verifyRepoStub) Reject(context.Context, string, []byte) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *verifyRepoStub) CancelPrevious(context.Context, int64, int64, string) (int64, error) {
	return 0, nil
}
func (s *verifyRepoStub) IncrementAttempts(context.Context, string) (int, error) { return 0, nil }
func (s *verifyRepoStub) Expire(context.Context, string) error                   { return nil }
func (s *verifyRepoStub) HasApproved(context.Context, int64, int64, string) (bool, error) {
	return false, nil
}
func (s *verifyRepoStub) GetLatestByChannel(context.Context, int64, int64, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *verifyRepoStub) UpdateMeta(_ context.Context, _ string, metaUpdate []byte) (*models.SignatureConfirmation, error) {
	if s.confirmation != nil {
		s.confirmation.Meta = json.RawMessage(metaUpdate)
	}
	return s.confirmation, nil
}

type verifyDocStub struct{ doc *models.Document }

func (s *verifyDocStub) GetByID(id int64) (*models.Document, error) {
	if s.doc != nil && s.doc.ID == id {
		return s.doc, nil
	}
	return nil, nil
}

func TestVerifyEmailTokenBrowserRedirectsToFrontend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewDocumentSigningConfirmationHandler(&services.DocumentSigningConfirmationService{}, nil, nil, "https://frontend.example.com")
	r := gin.New()
	r.GET("/sign/email/verify", h.VerifyEmailToken)

	req := httptest.NewRequest(http.MethodGet, "/sign/email/verify?token=test-token", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("unexpected status: got=%d want=%d", w.Code, http.StatusFound)
	}
	if got := w.Header().Get("Location"); got != "https://frontend.example.com/sign/email/verify?token=test-token" {
		t.Fatalf("unexpected location: %q", got)
	}
}

func TestVerifyEmailTokenAPIReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := "test-token"
	hash := sha256.Sum256([]byte(token))
	hashHex := hex.EncodeToString(hash[:])
	repo := &verifyRepoStub{confirmation: &models.SignatureConfirmation{
		ID:         "c1",
		DocumentID: 3,
		UserID:     1,
		Channel:    "email",
		Status:     "pending",
		TokenHash:  &hashHex,
		ExpiresAt:  time.Now().Add(10 * time.Minute),
	}}
	docRepo := &verifyDocStub{doc: &models.Document{ID: 3, DocType: "contract_paid_50_50_ru", Status: "approved"}}
	svc := services.NewDocumentSigningConfirmationService(
		repo,
		nil,
		docRepo,
		nil,
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute},
		time.Now,
	)
	h := NewDocumentSigningConfirmationHandler(svc, nil, nil, "https://frontend.example.com")

	r := gin.New()
	r.GET("/api/v1/sign/email/verify", h.VerifyEmailToken)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sign/email/verify?token=test-token&format=json", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["token_valid"] != true {
		t.Fatalf("expected token_valid=true, got=%v", payload["token_valid"])
	}
	if payload["require_post_confirm"] != true {
		t.Fatalf("expected require_post_confirm=true, got=%v", payload["require_post_confirm"])
	}
}

func TestVerifyEmailTokenAPIInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &verifyRepoStub{}
	docRepo := &verifyDocStub{}
	svc := services.NewDocumentSigningConfirmationService(
		repo,
		nil,
		docRepo,
		nil,
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute},
		time.Now,
	)
	h := NewDocumentSigningConfirmationHandler(svc, nil, nil, "https://frontend.example.com")

	r := gin.New()
	r.GET("/api/v1/sign/email/verify", h.VerifyEmailToken)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sign/email/verify?token=bad-token&format=json", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Fatalf("expected NOT_FOUND error code, got body=%s", w.Body.String())
	}
}

func TestVerifyEmailTokenAPIExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := "expired-token"
	hash := sha256.Sum256([]byte(token))
	hashHex := hex.EncodeToString(hash[:])
	repo := &verifyRepoStub{confirmation: &models.SignatureConfirmation{
		ID:         "c-exp",
		DocumentID: 9,
		UserID:     1,
		Channel:    "email",
		Status:     "pending",
		TokenHash:  &hashHex,
		ExpiresAt:  time.Now().Add(-1 * time.Minute),
	}}
	docRepo := &verifyDocStub{doc: &models.Document{ID: 9, DocType: "contract", Status: "approved"}}
	svc := services.NewDocumentSigningConfirmationService(
		repo,
		nil,
		docRepo,
		nil,
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute},
		time.Now,
	)
	h := NewDocumentSigningConfirmationHandler(svc, nil, nil, "https://frontend.example.com")

	r := gin.New()
	r.GET("/api/v1/sign/email/verify", h.VerifyEmailToken)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sign/email/verify?token=expired-token&format=json", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusGone, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "EXPIRED") {
		t.Fatalf("expected EXPIRED code, got body=%s", w.Body.String())
	}
}
