package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"turcompany/internal/models"
)

type validateRepoStub struct {
	confirmation *models.SignatureConfirmation
	approveCalls int
}

func (s *validateRepoStub) CreatePending(context.Context, int64, int64, string, *string, *string, time.Time, []byte) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *validateRepoStub) FindPending(context.Context, int64, int64, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *validateRepoStub) FindPendingByTokenHash(context.Context, string, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *validateRepoStub) FindByTokenHash(_ context.Context, channel, tokenHash string) (*models.SignatureConfirmation, error) {
	if s.confirmation != nil && s.confirmation.Channel == channel && s.confirmation.TokenHash != nil && *s.confirmation.TokenHash == tokenHash {
		return s.confirmation, nil
	}
	return nil, nil
}
func (s *validateRepoStub) Approve(context.Context, string, []byte) (*models.SignatureConfirmation, error) {
	s.approveCalls++
	if s.confirmation != nil {
		s.confirmation.Status = "approved"
		now := time.Now().UTC()
		s.confirmation.ApprovedAt = &now
		return s.confirmation, nil
	}
	return nil, nil
}
func (s *validateRepoStub) Reject(context.Context, string, []byte) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *validateRepoStub) CancelPrevious(context.Context, int64, int64, string) (int64, error) {
	return 0, nil
}
func (s *validateRepoStub) IncrementAttempts(context.Context, string) (int, error) { return 0, nil }
func (s *validateRepoStub) Expire(context.Context, string) error                   { return nil }
func (s *validateRepoStub) HasApproved(context.Context, int64, int64, string) (bool, error) {
	return false, nil
}
func (s *validateRepoStub) GetLatestByChannel(context.Context, int64, int64, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (s *validateRepoStub) UpdateMeta(_ context.Context, _ string, metaUpdate []byte) (*models.SignatureConfirmation, error) {
	if s.confirmation != nil {
		s.confirmation.Meta = json.RawMessage(metaUpdate)
	}
	return s.confirmation, nil
}

type validateDocRepoStub struct{ doc *models.Document }

func (s *validateDocRepoStub) GetByID(id int64) (*models.Document, error) {
	if s.doc != nil && s.doc.ID == id {
		return s.doc, nil
	}
	return nil, nil
}

type validateSignerSpy struct{ calls int }

func (s *validateSignerSpy) FinalizeSigning(int64) error {
	s.calls++
	return nil
}

func TestValidateEmailTokenDoesNotFinalizeDocument(t *testing.T) {
	token := "verify-token"
	hash := hashConfirmTokenWithPepper(token, "")
	repo := &validateRepoStub{confirmation: &models.SignatureConfirmation{
		ID:         "c1",
		DocumentID: 44,
		UserID:     1,
		Channel:    "email",
		Status:     "pending",
		TokenHash:  &hash,
		ExpiresAt:  time.Now().Add(5 * time.Minute),
	}}
	docRepo := &validateDocRepoStub{doc: &models.Document{ID: 44, DocType: "contract", Status: "approved"}}
	signer := &validateSignerSpy{}

	svc := NewDocumentSigningConfirmationService(
		repo,
		nil,
		docRepo,
		signer,
		nil,
		nil,
		DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute},
		time.Now,
	)

	result, err := svc.ValidateEmailToken(context.Background(), token, "", "")
	if err != nil {
		t.Fatalf("ValidateEmailToken returned error: %v", err)
	}
	if !result.TokenValid {
		t.Fatalf("expected token_valid=true")
	}
	if signer.calls != 0 {
		t.Fatalf("expected no finalize calls during verify, got %d", signer.calls)
	}
}

func TestConfirmByEmailTokenDoesNotFinalizeDocument(t *testing.T) {
	tmp := t.TempDir()
	docRel := "pdf/sample.pdf"
	docAbs := tmp + "/" + docRel
	if err := os.MkdirAll(filepath.Dir(docAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(docAbs, []byte("sample-pdf-content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	token := "confirm-token"
	code := "123456"
	otpHash, err := HashVerificationCode(code)
	if err != nil {
		t.Fatalf("hash verification code: %v", err)
	}
	tokenHash := hashConfirmTokenWithPepper(token, "")
	repo := &validateRepoStub{confirmation: &models.SignatureConfirmation{
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
	docRepo := &validateDocRepoStub{doc: &models.Document{ID: 55, DocType: "contract", Status: "approved", FilePathPdf: "/" + docRel}}
	signer := &validateSignerSpy{}
	svc := NewDocumentSigningConfirmationService(
		repo,
		nil,
		docRepo,
		signer,
		nil,
		nil,
		DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute, FilesRoot: tmp},
		time.Now,
	)

	sum := sha256.Sum256([]byte("sample-pdf-content"))
	clientHash := "sha256:" + hex.EncodeToString(sum[:])
	status, signerEmail, docHash, _, err := svc.ConfirmByEmailToken(context.Background(), 55, token, code, clientHash, "v1", "127.0.0.1", "ua")
	if err != nil {
		t.Fatalf("ConfirmByEmailToken returned error: %v", err)
	}
	if status != "approved" {
		t.Fatalf("unexpected status: %s", status)
	}
	if signerEmail != "client@example.com" {
		t.Fatalf("unexpected signer email: %s", signerEmail)
	}
	if strings.TrimSpace(docHash) == "" {
		t.Fatal("expected non-empty document hash")
	}
	if repo.approveCalls != 1 {
		t.Fatalf("expected approve to be called once, got %d", repo.approveCalls)
	}
	if signer.calls != 0 {
		t.Fatalf("expected no finalize call on confirm step, got %d", signer.calls)
	}
}
