package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"turcompany/internal/models"
)

type fakeEmailSignDocService struct {
	docRepo *fakeSignDocumentRepo
}

func (s *fakeEmailSignDocService) EnsureSigningAllowed(docID int64, userID, roleID int) error {
	return nil
}

func (s *fakeEmailSignDocService) FinalizeSigning(docID int64) error {
	if s.docRepo == nil {
		return nil
	}
	return s.docRepo.UpdateStatus(docID, "signed")
}

func TestEmailSignSessionFlowEndToEnd(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	signRepo := newFakeSignRepo()
	tempDir := t.TempDir()
	fileName := "doc.pdf"
	if err := os.WriteFile(filepath.Join(tempDir, fileName), []byte("content"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	docRepo := &fakeSignDocumentRepo{doc: &models.Document{ID: 101, DocType: "contract", Status: "approved", FilePathPdf: fileName}}
	userRepo := &fakeUserRepo{user: &models.User{ID: 9, Email: "sender@example.com"}}
	emailSender := &fakeEmailSender{}

	signConfirmService := NewDocumentSigningConfirmationService(
		repo,
		userRepo,
		docRepo,
		&fakeDocumentSigner{docRepo: docRepo},
		emailSender,
		nil,
		DocumentSigningConfirmationConfig{
			ConfirmPolicy:      SignConfirmPolicyAny,
			EmailTTL:           15 * time.Minute,
			EmailVerifyBaseURL: "http://example.com",
			FilesRoot:          tempDir,
		},
		now,
	)
	signConfirmService.EnableDebug("")

	signSessionService := NewSignSessionService(
		signRepo,
		&fakeEmailSignDocService{docRepo: docRepo},
		&fakeSignDelivery{},
		SignSessionConfig{},
		now,
	)

	if _, err := signConfirmService.StartSigning(context.Background(), 101, 9, "client@example.com"); err != nil {
		t.Fatalf("start signing: %v", err)
	}
	debug, ok := signConfirmService.DebugLatest(101, 9)
	if !ok {
		t.Fatalf("expected debug entry")
	}
	if _, err := signConfirmService.ValidateEmailToken(context.Background(), debug.EmailToken, "127.0.0.1", "UA"); err != nil {
		t.Fatalf("verify token: %v", err)
	}
	status, signerEmail, docHash, _, err := signConfirmService.ConfirmByEmailToken(
		context.Background(),
		101,
		debug.EmailToken,
		debug.EmailCode,
		"127.0.0.1",
		"UA",
	)
	if err != nil {
		t.Fatalf("confirm email: %v", err)
	}
	if status != "approved" {
		t.Fatalf("expected approved status, got %s", status)
	}
	if signerEmail != "client@example.com" {
		t.Fatalf("expected signer email, got %s", signerEmail)
	}
	if docHash == "" {
		t.Fatalf("expected document hash")
	}

	token, session, err := signSessionService.CreateEmailSession(context.Background(), 101, signerEmail, docHash)
	if err != nil {
		t.Fatalf("create sign session: %v", err)
	}
	if session.DocHash != docHash {
		t.Fatalf("expected doc hash saved")
	}
	if _, err := signSessionService.ValidateSessionForPage(context.Background(), session.ID, token); err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if _, err := signSessionService.SignByID(context.Background(), session.ID, token, "127.0.0.1", "UA"); err != nil {
		t.Fatalf("sign session: %v", err)
	}
	if docRepo.doc.Status != "signed" {
		t.Fatalf("expected document signed, got %s", docRepo.doc.Status)
	}
}
