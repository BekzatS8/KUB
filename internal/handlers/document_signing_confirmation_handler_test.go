package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type verifyRepoStub struct {
	confirmation  *models.SignatureConfirmation
	updateMetaErr error
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
	return s.confirmation, nil
}
func (s *verifyRepoStub) UpdateMeta(_ context.Context, _ string, metaUpdate []byte) (*models.SignatureConfirmation, error) {
	if s.updateMetaErr != nil {
		return nil, s.updateMetaErr
	}
	if s.confirmation != nil {
		current := map[string]any{}
		if len(s.confirmation.Meta) > 0 {
			_ = json.Unmarshal(s.confirmation.Meta, &current)
		}
		incoming := map[string]any{}
		if len(metaUpdate) > 0 {
			_ = json.Unmarshal(metaUpdate, &incoming)
		}
		for key, value := range incoming {
			current[key] = value
		}
		merged, _ := json.Marshal(current)
		s.confirmation.Meta = json.RawMessage(merged)
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

type statusDocRepoStub struct {
	doc *models.Document
}

func (s *statusDocRepoStub) Create(*models.Document) (int64, error) { return 0, nil }
func (s *statusDocRepoStub) GetByID(id int64) (*models.Document, error) {
	if s.doc != nil && s.doc.ID == id {
		return s.doc, nil
	}
	return nil, nil
}
func (s *statusDocRepoStub) GetByIDWithArchiveScope(id int64, scope repositories.ArchiveScope) (*models.Document, error) {
	return s.GetByID(id)
}
func (s *statusDocRepoStub) ListDocuments(int, int) ([]*models.Document, error) { return nil, nil }
func (s *statusDocRepoStub) ListDocumentsWithArchiveScope(int, int, repositories.ArchiveScope) ([]*models.Document, error) {
	return nil, nil
}
func (s *statusDocRepoStub) ListDocumentsByDeal(int64) ([]*models.Document, error) {
	return nil, nil
}
func (s *statusDocRepoStub) ListDocumentsByDealWithArchiveScope(int64, repositories.ArchiveScope) ([]*models.Document, error) {
	return nil, nil
}
func (s *statusDocRepoStub) Delete(int64) error                        { return nil }
func (s *statusDocRepoStub) Archive(int64, int, string) error          { return nil }
func (s *statusDocRepoStub) Unarchive(int64) error                     { return nil }
func (s *statusDocRepoStub) UpdateStatus(int64, string) error          { return nil }
func (s *statusDocRepoStub) MarkSigned(int64, string, time.Time) error { return nil }
func (s *statusDocRepoStub) Update(*models.Document) error             { return nil }
func (s *statusDocRepoStub) UpdateSigningMeta(int64, string, string, string, string) error {
	return nil
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
	tmp := t.TempDir()
	docRel := "pdf/sample.pdf"
	docAbs := filepath.Join(tmp, docRel)
	if err := os.MkdirAll(filepath.Dir(docAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(docAbs, []byte("sample-pdf-content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
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
	docRepo := &verifyDocStub{doc: &models.Document{ID: 3, DocType: "contract_paid_50_50_ru", Status: "approved", FilePathPdf: "/" + docRel}}
	svc := services.NewDocumentSigningConfirmationService(
		repo,
		nil,
		docRepo,
		nil,
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute, FilesRoot: tmp},
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
	documentPayload, ok := payload["document"].(map[string]any)
	if !ok {
		t.Fatalf("expected document payload object, got=%T", payload["document"])
	}
	previewURL, ok := documentPayload["preview_url"].(string)
	if !ok || strings.TrimSpace(previewURL) == "" {
		t.Fatalf("expected preview_url in verify response")
	}
	hashPreview, ok := documentPayload["document_hash_preview"].(string)
	if !ok || strings.TrimSpace(hashPreview) == "" {
		t.Fatalf("expected document_hash_preview in verify response")
	}
	agreementPayload, ok := payload["agreement"].(map[string]any)
	if !ok {
		t.Fatalf("expected agreement payload object, got=%T", payload["agreement"])
	}
	if agreementPayload["required"] != true {
		t.Fatalf("expected agreement.required=true, got=%v", agreementPayload["required"])
	}
	if strings.TrimSpace(agreementPayload["version"].(string)) == "" {
		t.Fatalf("expected agreement.version")
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

func TestStatusIncludesEmailConfirmationAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	meta := json.RawMessage(`{
		"link_opened_at":"2026-04-13T10:00:00Z",
		"opened_ip":"1.1.1.1",
		"opened_user_agent":"verify-ua",
		"preview_opened_at":"2026-04-13T10:01:00Z",
		"preview_open_count":2,
		"preview_document_hash":"sha256:abc",
		"agreed_at":"2026-04-13T10:02:00Z",
		"agreement_text_version":"v1",
		"agreement_version_verified":true,
		"agreement_version_verified_at":"2026-04-13T10:02:01Z",
		"document_hash_verified":true
	}`)
	repo := &verifyRepoStub{confirmation: &models.SignatureConfirmation{
		ID:         "c-status",
		DocumentID: 42,
		UserID:     0,
		Channel:    "email",
		Status:     "approved",
		ExpiresAt:  time.Now().Add(10 * time.Minute),
		Meta:       meta,
	}}
	docRepo := &verifyDocStub{doc: &models.Document{ID: 42, DocType: "contract", Status: "signed", SignIP: "2.2.2.2", SignUserAgent: "sign-ua"}}
	confirmSvc := services.NewDocumentSigningConfirmationService(
		repo,
		nil,
		docRepo,
		nil,
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute},
		time.Now,
	)
	docSvc := services.NewDocumentService(
		&statusDocRepoStub{doc: &models.Document{ID: 42, DocType: "contract", Status: "signed", SignIP: "2.2.2.2", SignUserAgent: "sign-ua"}},
		nil,
		nil,
		nil,
		"",
		"",
		nil,
		nil,
		nil,
	)
	h := NewDocumentSigningConfirmationHandler(confirmSvc, docSvc, nil, "")
	r := gin.New()
	r.GET("/documents/:id/sign/status", h.Status)
	req := httptest.NewRequest(http.MethodGet, "/documents/42/sign/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	audit, ok := payload["email_confirmation_audit"].(map[string]any)
	if !ok {
		t.Fatalf("expected email_confirmation_audit object, got=%T", payload["email_confirmation_audit"])
	}
	if got := int(audit["preview_open_count"].(float64)); got != 2 {
		t.Fatalf("unexpected preview_open_count: %d", got)
	}
	if audit["preview_document_hash"] != "sha256:abc" {
		t.Fatalf("unexpected preview_document_hash: %v", audit["preview_document_hash"])
	}
	if audit["agreement_version_verified"] != true {
		t.Fatalf("expected agreement_version_verified=true, got=%v", audit["agreement_version_verified"])
	}
}

func TestStatusWithoutAuditKeepsResponseCompatible(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &verifyRepoStub{confirmation: &models.SignatureConfirmation{
		ID:         "c-status-empty",
		DocumentID: 43,
		UserID:     0,
		Channel:    "email",
		Status:     "pending",
		ExpiresAt:  time.Now().Add(10 * time.Minute),
	}}
	docRepo := &verifyDocStub{doc: &models.Document{ID: 43, DocType: "contract", Status: "approved"}}
	confirmSvc := services.NewDocumentSigningConfirmationService(
		repo,
		nil,
		docRepo,
		nil,
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute},
		time.Now,
	)
	docSvc := services.NewDocumentService(
		&statusDocRepoStub{doc: &models.Document{ID: 43, DocType: "contract", Status: "approved"}},
		nil,
		nil,
		nil,
		"",
		"",
		nil,
		nil,
		nil,
	)
	h := NewDocumentSigningConfirmationHandler(confirmSvc, docSvc, nil, "")
	r := gin.New()
	r.GET("/documents/:id/sign/status", h.Status)
	req := httptest.NewRequest(http.MethodGet, "/documents/43/sign/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := payload["status"]; !ok {
		t.Fatalf("expected legacy status field")
	}
	if payload["email_confirmation_audit"] != nil {
		t.Fatalf("expected nil email_confirmation_audit when meta is absent, got=%v", payload["email_confirmation_audit"])
	}
}

type confirmRepoStub struct {
	confirmation *models.SignatureConfirmation
}

func (s *confirmRepoStub) CreatePending(context.Context, int64, int64, string, *string, *string, time.Time, []byte) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *confirmRepoStub) FindPending(context.Context, int64, int64, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *confirmRepoStub) FindPendingByTokenHash(context.Context, string, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *confirmRepoStub) FindByTokenHash(_ context.Context, channel, tokenHash string) (*models.SignatureConfirmation, error) {
	if s.confirmation != nil && s.confirmation.Channel == channel && s.confirmation.TokenHash != nil && *s.confirmation.TokenHash == tokenHash {
		return s.confirmation, nil
	}
	return nil, nil
}
func (s *confirmRepoStub) Approve(_ context.Context, _ string, metaUpdate []byte) (*models.SignatureConfirmation, error) {
	if s.confirmation != nil {
		s.confirmation.Status = "approved"
		now := time.Now().UTC()
		s.confirmation.ApprovedAt = &now
		s.confirmation.Meta = json.RawMessage(metaUpdate)
	}
	return s.confirmation, nil
}
func (s *confirmRepoStub) Reject(context.Context, string, []byte) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *confirmRepoStub) CancelPrevious(context.Context, int64, int64, string) (int64, error) {
	return 0, nil
}
func (s *confirmRepoStub) IncrementAttempts(context.Context, string) (int, error) { return 0, nil }
func (s *confirmRepoStub) Expire(context.Context, string) error                   { return nil }
func (s *confirmRepoStub) HasApproved(context.Context, int64, int64, string) (bool, error) {
	return false, nil
}
func (s *confirmRepoStub) GetLatestByChannel(context.Context, int64, int64, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *confirmRepoStub) UpdateMeta(_ context.Context, _ string, metaUpdate []byte) (*models.SignatureConfirmation, error) {
	if s.confirmation != nil {
		s.confirmation.Meta = json.RawMessage(metaUpdate)
	}
	return s.confirmation, nil
}

type confirmDocRepoStub struct{ doc *models.Document }

func (s *confirmDocRepoStub) GetByID(id int64) (*models.Document, error) {
	if s.doc != nil && s.doc.ID == id {
		return s.doc, nil
	}
	return nil, nil
}

type confirmSignSessionRepoStub struct {
	nextID int64
}

func (r *confirmSignSessionRepoStub) Create(_ context.Context, s *models.SignSession) error {
	r.nextID++
	s.ID = r.nextID
	s.CreatedAt = time.Now().UTC()
	return nil
}
func (r *confirmSignSessionRepoStub) GetByTokenHash(context.Context, string) (*models.SignSession, error) {
	return nil, nil
}
func (r *confirmSignSessionRepoStub) GetByID(context.Context, int64) (*models.SignSession, error) {
	return nil, nil
}
func (r *confirmSignSessionRepoStub) FindSignedByDocumentEmail(context.Context, int64, string) (*models.SignSession, error) {
	return nil, nil
}
func (r *confirmSignSessionRepoStub) CountRecentByDocumentID(context.Context, int64, time.Time) (int, error) {
	return 0, nil
}
func (r *confirmSignSessionRepoStub) CountRecentByPhone(context.Context, string, time.Time) (int, error) {
	return 0, nil
}
func (r *confirmSignSessionRepoStub) Update(context.Context, *models.SignSession) error { return nil }
func (r *confirmSignSessionRepoStub) IncrementAttempts(context.Context, int64) (int, error) {
	return 0, nil
}

type confirmDocSignerStub struct{}

func (s *confirmDocSignerStub) FinalizeSigning(int64) error { return nil }

func TestConfirmByEmailCodeRequiresAgreementFlags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewDocumentSigningConfirmationHandler(
		services.NewDocumentSigningConfirmationService(nil, nil, nil, nil, nil, nil, services.DocumentSigningConfirmationConfig{}, time.Now),
		nil,
		nil,
		"",
	)
	r := gin.New()
	r.POST("/documents/:id/sign/confirm/email", h.ConfirmByEmailCode)

	body := `{"token":"token","code":"123456","agree_terms":false,"confirm_document_read":true}`
	req := httptest.NewRequest(http.MethodPost, "/documents/55/sign/confirm/email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Document review and agreement are required") {
		t.Fatalf("expected agreement validation message, got body=%s", w.Body.String())
	}
}

func TestConfirmByEmailCodeRequiresReadFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewDocumentSigningConfirmationHandler(
		services.NewDocumentSigningConfirmationService(nil, nil, nil, nil, nil, nil, services.DocumentSigningConfirmationConfig{}, time.Now),
		nil,
		nil,
		"",
	)
	r := gin.New()
	r.POST("/documents/:id/sign/confirm/email", h.ConfirmByEmailCode)

	body := `{"token":"token","code":"123456","agree_terms":true,"confirm_document_read":false}`
	req := httptest.NewRequest(http.MethodPost, "/documents/55/sign/confirm/email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Document review and agreement are required") {
		t.Fatalf("expected agreement validation message, got body=%s", w.Body.String())
	}
}

func TestConfirmByEmailCodeRequiresDocumentHash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewDocumentSigningConfirmationHandler(
		services.NewDocumentSigningConfirmationService(nil, nil, nil, nil, nil, nil, services.DocumentSigningConfirmationConfig{}, time.Now),
		nil,
		nil,
		"",
	)
	r := gin.New()
	r.POST("/documents/:id/sign/confirm/email", h.ConfirmByEmailCode)

	body := `{"token":"token","code":"123456","agree_terms":true,"confirm_document_read":true,"agreement_text_version":"v1"}`
	req := httptest.NewRequest(http.MethodPost, "/documents/55/sign/confirm/email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Document hash is required") {
		t.Fatalf("expected hash validation message, got body=%s", w.Body.String())
	}
}

func TestConfirmByEmailCodeRequiresAgreementVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewDocumentSigningConfirmationHandler(
		services.NewDocumentSigningConfirmationService(nil, nil, nil, nil, nil, nil, services.DocumentSigningConfirmationConfig{}, time.Now),
		nil,
		nil,
		"",
	)
	r := gin.New()
	r.POST("/documents/:id/sign/confirm/email", h.ConfirmByEmailCode)

	body := `{"token":"token","code":"123456","agree_terms":true,"confirm_document_read":true,"document_hash_from_client":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
	req := httptest.NewRequest(http.MethodPost, "/documents/55/sign/confirm/email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Agreement text version is required") {
		t.Fatalf("expected agreement version validation message, got body=%s", w.Body.String())
	}
}

func TestConfirmByEmailCodeSuccessWithAgreementFlags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	docRel := "pdf/sample.pdf"
	docAbs := filepath.Join(tmp, docRel)
	if err := os.MkdirAll(filepath.Dir(docAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(docAbs, []byte("sample-pdf-content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	token := "confirm-token"
	code := "123456"
	otpHash, err := services.HashVerificationCode(code)
	if err != nil {
		t.Fatalf("hash verification code: %v", err)
	}
	tokenHash := services.HashEmailConfirmTokenForLog(token, "")
	confirmRepo := &confirmRepoStub{confirmation: &models.SignatureConfirmation{
		ID:         "c2",
		DocumentID: 55,
		UserID:     1,
		Channel:    "email",
		Status:     "pending",
		TokenHash:  &tokenHash,
		OTPHash:    &otpHash,
		ExpiresAt:  time.Now().Add(10 * time.Minute),
		Meta:       json.RawMessage(`{"signer_email":"client@example.com"}`),
	}}
	docRepo := &confirmDocRepoStub{doc: &models.Document{ID: 55, DocType: "contract", Status: "approved", FilePathPdf: "/" + docRel}}
	confirmSvc := services.NewDocumentSigningConfirmationService(
		confirmRepo,
		nil,
		docRepo,
		&confirmDocSignerStub{},
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute, FilesRoot: tmp},
		time.Now,
	)
	sessionSvc := services.NewSignSessionService(
		&confirmSignSessionRepoStub{},
		nil,
		nil,
		services.SignSessionConfig{SessionTTL: 30 * time.Minute},
		time.Now,
	)

	h := NewDocumentSigningConfirmationHandler(confirmSvc, nil, sessionSvc, "")
	r := gin.New()
	r.POST("/documents/:id/sign/confirm/email", h.ConfirmByEmailCode)

	fileHash := sha256.Sum256([]byte("sample-pdf-content"))
	body := `{"token":"confirm-token","code":"123456","agree_terms":true,"confirm_document_read":true,"agreement_text_version":"v1","document_hash_from_client":"sha256:` + hex.EncodeToString(fileHash[:]) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/documents/55/sign/confirm/email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-agent")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["status"] != "approved" {
		t.Fatalf("expected status=approved, got=%v", payload["status"])
	}
	if payload["session_id"] == nil {
		t.Fatalf("expected session_id in response")
	}
}

func TestConfirmByEmailCodeHashMismatchReturnsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	docRel := "pdf/sample.pdf"
	docAbs := filepath.Join(tmp, docRel)
	if err := os.MkdirAll(filepath.Dir(docAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(docAbs, []byte("sample-pdf-content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	token := "confirm-token"
	code := "123456"
	otpHash, err := services.HashVerificationCode(code)
	if err != nil {
		t.Fatalf("hash verification code: %v", err)
	}
	tokenHash := services.HashEmailConfirmTokenForLog(token, "")
	confirmRepo := &confirmRepoStub{confirmation: &models.SignatureConfirmation{
		ID:         "c3",
		DocumentID: 56,
		UserID:     1,
		Channel:    "email",
		Status:     "pending",
		TokenHash:  &tokenHash,
		OTPHash:    &otpHash,
		ExpiresAt:  time.Now().Add(10 * time.Minute),
		Meta:       json.RawMessage(`{"signer_email":"client@example.com"}`),
	}}
	docRepo := &confirmDocRepoStub{doc: &models.Document{ID: 56, DocType: "contract", Status: "approved", FilePathPdf: "/" + docRel}}
	confirmSvc := services.NewDocumentSigningConfirmationService(
		confirmRepo,
		nil,
		docRepo,
		&confirmDocSignerStub{},
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute, FilesRoot: tmp},
		time.Now,
	)
	sessionSvc := services.NewSignSessionService(
		&confirmSignSessionRepoStub{},
		nil,
		nil,
		services.SignSessionConfig{SessionTTL: 30 * time.Minute},
		time.Now,
	)
	h := NewDocumentSigningConfirmationHandler(confirmSvc, nil, sessionSvc, "")
	r := gin.New()
	r.POST("/documents/:id/sign/confirm/email", h.ConfirmByEmailCode)

	body := `{"token":"confirm-token","code":"123456","agree_terms":true,"confirm_document_read":true,"agreement_text_version":"v1","document_hash_from_client":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
	req := httptest.NewRequest(http.MethodPost, "/documents/56/sign/confirm/email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Document version mismatch") {
		t.Fatalf("expected mismatch message, got body=%s", w.Body.String())
	}
}

func TestConfirmByEmailCodeAgreementVersionMismatchReturnsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	docRel := "pdf/sample.pdf"
	docAbs := filepath.Join(tmp, docRel)
	if err := os.MkdirAll(filepath.Dir(docAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(docAbs, []byte("sample-pdf-content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	token := "confirm-token"
	code := "123456"
	otpHash, err := services.HashVerificationCode(code)
	if err != nil {
		t.Fatalf("hash verification code: %v", err)
	}
	tokenHash := services.HashEmailConfirmTokenForLog(token, "")
	confirmRepo := &confirmRepoStub{confirmation: &models.SignatureConfirmation{
		ID:         "c4",
		DocumentID: 57,
		UserID:     1,
		Channel:    "email",
		Status:     "pending",
		TokenHash:  &tokenHash,
		OTPHash:    &otpHash,
		ExpiresAt:  time.Now().Add(10 * time.Minute),
		Meta:       json.RawMessage(`{"signer_email":"client@example.com"}`),
	}}
	docRepo := &confirmDocRepoStub{doc: &models.Document{ID: 57, DocType: "contract", Status: "approved", FilePathPdf: "/" + docRel}}
	confirmSvc := services.NewDocumentSigningConfirmationService(
		confirmRepo,
		nil,
		docRepo,
		&confirmDocSignerStub{},
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute, FilesRoot: tmp},
		time.Now,
	)
	sessionSvc := services.NewSignSessionService(
		&confirmSignSessionRepoStub{},
		nil,
		nil,
		services.SignSessionConfig{SessionTTL: 30 * time.Minute},
		time.Now,
	)
	h := NewDocumentSigningConfirmationHandler(confirmSvc, nil, sessionSvc, "")
	r := gin.New()
	r.POST("/documents/:id/sign/confirm/email", h.ConfirmByEmailCode)

	fileHash := sha256.Sum256([]byte("sample-pdf-content"))
	body := `{"token":"confirm-token","code":"123456","agree_terms":true,"confirm_document_read":true,"agreement_text_version":"v0","document_hash_from_client":"sha256:` + hex.EncodeToString(fileHash[:]) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/documents/57/sign/confirm/email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Agreement text version mismatch") {
		t.Fatalf("expected mismatch message, got body=%s", w.Body.String())
	}
}

func TestPreviewByEmailTokenReturnsInlineFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	docRel := "pdf/sample.pdf"
	docAbs := filepath.Join(tmp, docRel)
	if err := os.MkdirAll(filepath.Dir(docAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(docAbs, []byte("sample-pdf-content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	token := "preview-token"
	hash := sha256.Sum256([]byte(token))
	hashHex := hex.EncodeToString(hash[:])
	repo := &verifyRepoStub{confirmation: &models.SignatureConfirmation{
		ID:         "c-preview",
		DocumentID: 7,
		UserID:     1,
		Channel:    "email",
		Status:     "pending",
		TokenHash:  &hashHex,
		ExpiresAt:  time.Now().Add(10 * time.Minute),
	}}
	docRepo := &verifyDocStub{doc: &models.Document{ID: 7, DocType: "contract", Status: "approved", FilePathPdf: "/" + docRel}}
	svc := services.NewDocumentSigningConfirmationService(
		repo,
		nil,
		docRepo,
		nil,
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute, FilesRoot: tmp},
		time.Now,
	)
	h := NewDocumentSigningConfirmationHandler(svc, nil, nil, "")

	r := gin.New()
	r.GET("/api/v1/sign/email/preview", h.PreviewByEmailToken)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sign/email/preview?token=preview-token", nil)
	req.Header.Set("User-Agent", "preview-ua")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "inline;") {
		t.Fatalf("expected inline content disposition, got=%q", w.Header().Get("Content-Disposition"))
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/pdf") {
		t.Fatalf("expected pdf content type, got=%q", got)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/sign/email/preview?token=preview-token", nil)
	req2.Header.Set("User-Agent", "preview-ua")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("unexpected status on second preview: got=%d want=%d body=%s", w2.Code, http.StatusOK, w2.Body.String())
	}
	var metaPayload map[string]any
	if err := json.Unmarshal(repo.confirmation.Meta, &metaPayload); err != nil {
		t.Fatalf("unmarshal preview meta: %v", err)
	}
	if got := int(metaPayload["preview_open_count"].(float64)); got != 2 {
		t.Fatalf("expected preview_open_count=2, got=%d", got)
	}
	if strings.TrimSpace(metaPayload["preview_opened_at"].(string)) == "" {
		t.Fatalf("expected preview_opened_at to be set")
	}
	if !strings.HasPrefix(metaPayload["preview_document_hash"].(string), "sha256:") {
		t.Fatalf("expected preview_document_hash with sha256 prefix, got=%v", metaPayload["preview_document_hash"])
	}
}

func TestPreviewByEmailTokenMetaUpdateFailSoft(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	docRel := "pdf/sample.pdf"
	docAbs := filepath.Join(tmp, docRel)
	if err := os.MkdirAll(filepath.Dir(docAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(docAbs, []byte("sample-pdf-content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	token := "preview-token-failsoft"
	hash := sha256.Sum256([]byte(token))
	hashHex := hex.EncodeToString(hash[:])
	repo := &verifyRepoStub{
		confirmation: &models.SignatureConfirmation{
			ID:         "c-preview-failsoft",
			DocumentID: 8,
			UserID:     1,
			Channel:    "email",
			Status:     "pending",
			TokenHash:  &hashHex,
			ExpiresAt:  time.Now().Add(10 * time.Minute),
		},
		updateMetaErr: errors.New("db unavailable"),
	}
	docRepo := &verifyDocStub{doc: &models.Document{ID: 8, DocType: "contract", Status: "approved", FilePathPdf: "/" + docRel}}
	svc := services.NewDocumentSigningConfirmationService(
		repo,
		nil,
		docRepo,
		nil,
		nil,
		nil,
		services.DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute, FilesRoot: tmp},
		time.Now,
	)
	h := NewDocumentSigningConfirmationHandler(svc, nil, nil, "")
	r := gin.New()
	r.GET("/api/v1/sign/email/preview", h.PreviewByEmailToken)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sign/email/preview?token=preview-token-failsoft", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("preview should remain available on meta update error: got=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPreviewByEmailTokenInvalidToken(t *testing.T) {
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
	h := NewDocumentSigningConfirmationHandler(svc, nil, nil, "")
	r := gin.New()
	r.GET("/api/v1/sign/email/preview", h.PreviewByEmailToken)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sign/email/preview?token=bad-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestPreviewByEmailTokenExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := "expired-preview-token"
	hash := sha256.Sum256([]byte(token))
	hashHex := hex.EncodeToString(hash[:])
	repo := &verifyRepoStub{confirmation: &models.SignatureConfirmation{
		ID:         "c-preview-exp",
		DocumentID: 9,
		UserID:     1,
		Channel:    "email",
		Status:     "pending",
		TokenHash:  &hashHex,
		ExpiresAt:  time.Now().Add(-1 * time.Minute),
	}}
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
	h := NewDocumentSigningConfirmationHandler(svc, nil, nil, "")
	r := gin.New()
	r.GET("/api/v1/sign/email/preview", h.PreviewByEmailToken)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sign/email/preview?token=expired-preview-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", w.Code, http.StatusGone, w.Body.String())
	}
}
