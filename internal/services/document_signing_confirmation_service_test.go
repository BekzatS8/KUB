package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"turcompany/internal/models"
)

type fakeSignatureConfirmRepo struct {
	now     func() time.Time
	records []*models.SignatureConfirmation
}

func newFakeSignatureConfirmRepo(now func() time.Time) *fakeSignatureConfirmRepo {
	return &fakeSignatureConfirmRepo{now: now, records: []*models.SignatureConfirmation{}}
}

func (r *fakeSignatureConfirmRepo) CreatePending(
	_ context.Context,
	documentID int64,
	userID int64,
	channel string,
	otpHash *string,
	tokenHash *string,
	expiresAt time.Time,
	meta []byte,
) (*models.SignatureConfirmation, error) {
	id := fmt.Sprintf("id-%d", len(r.records)+1)
	record := &models.SignatureConfirmation{
		ID:         id,
		DocumentID: documentID,
		UserID:     userID,
		Channel:    channel,
		Status:     "pending",
		OTPHash:    otpHash,
		TokenHash:  tokenHash,
		Attempts:   0,
		ExpiresAt:  expiresAt,
		Meta:       meta,
	}
	r.records = append(r.records, record)
	return record, nil
}

func (r *fakeSignatureConfirmRepo) FindPending(_ context.Context, documentID, userID int64, channel string) (*models.SignatureConfirmation, error) {
	var latest *models.SignatureConfirmation
	for _, rec := range r.records {
		if rec.DocumentID == documentID && rec.UserID == userID && rec.Channel == channel && rec.Status == "pending" {
			if latest == nil || rec.ExpiresAt.After(latest.ExpiresAt) {
				latest = rec
			}
		}
	}
	return latest, nil
}

func (r *fakeSignatureConfirmRepo) FindPendingByTokenHash(_ context.Context, channel string, tokenHash string) (*models.SignatureConfirmation, error) {
	now := r.now()
	for _, rec := range r.records {
		if rec.Channel == channel && rec.Status == "pending" && rec.TokenHash != nil && *rec.TokenHash == tokenHash {
			if now.Before(rec.ExpiresAt) {
				return rec, nil
			}
		}
	}
	return nil, nil
}

func (r *fakeSignatureConfirmRepo) FindByTokenHash(_ context.Context, channel string, tokenHash string) (*models.SignatureConfirmation, error) {
	for _, rec := range r.records {
		if rec.Channel == channel && rec.TokenHash != nil && *rec.TokenHash == tokenHash {
			return rec, nil
		}
	}
	return nil, nil
}

func (r *fakeSignatureConfirmRepo) Approve(_ context.Context, id string, _ []byte) (*models.SignatureConfirmation, error) {
	for _, rec := range r.records {
		if rec.ID == id {
			rec.Status = "approved"
			now := r.now()
			rec.ApprovedAt = &now
			return rec, nil
		}
	}
	return nil, nil
}

func (r *fakeSignatureConfirmRepo) Reject(_ context.Context, id string, _ []byte) (*models.SignatureConfirmation, error) {
	for _, rec := range r.records {
		if rec.ID == id {
			rec.Status = "rejected"
			return rec, nil
		}
	}
	return nil, nil
}

func (r *fakeSignatureConfirmRepo) CancelPrevious(_ context.Context, documentID, userID int64, channel string) (int64, error) {
	var count int64
	for _, rec := range r.records {
		if rec.DocumentID == documentID && rec.UserID == userID && rec.Status == "pending" && rec.Channel == channel {
			rec.Status = "cancelled"
			count++
		}
	}
	return count, nil
}

func (r *fakeSignatureConfirmRepo) IncrementAttempts(_ context.Context, id string) (int, error) {
	for _, rec := range r.records {
		if rec.ID == id {
			rec.Attempts++
			return rec.Attempts, nil
		}
	}
	return 0, errors.New("not found")
}

func (r *fakeSignatureConfirmRepo) Expire(_ context.Context, id string) error {
	for _, rec := range r.records {
		if rec.ID == id {
			rec.Status = "expired"
			return nil
		}
	}
	return errors.New("not found")
}

func (r *fakeSignatureConfirmRepo) UpdateMeta(_ context.Context, id string, metaUpdate []byte) (*models.SignatureConfirmation, error) {
	for _, rec := range r.records {
		if rec.ID == id {
			merged := map[string]any{}
			if len(rec.Meta) > 0 {
				_ = json.Unmarshal(rec.Meta, &merged)
			}
			if len(metaUpdate) > 0 {
				update := map[string]any{}
				_ = json.Unmarshal(metaUpdate, &update)
				for key, value := range update {
					merged[key] = value
				}
			}
			if len(merged) > 0 {
				if bytes, err := json.Marshal(merged); err == nil {
					rec.Meta = bytes
				}
			}
			return rec, nil
		}
	}
	return nil, nil
}

func (r *fakeSignatureConfirmRepo) HasApproved(_ context.Context, documentID, userID int64, channel string) (bool, error) {
	for _, rec := range r.records {
		if rec.DocumentID == documentID && rec.UserID == userID && rec.Channel == channel && rec.Status == "approved" {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeSignatureConfirmRepo) GetLatestByChannel(_ context.Context, documentID, userID int64, channel string) (*models.SignatureConfirmation, error) {
	var latest *models.SignatureConfirmation
	for _, rec := range r.records {
		if rec.DocumentID == documentID && rec.UserID == userID && rec.Channel == channel {
			if latest == nil || rec.ExpiresAt.After(latest.ExpiresAt) {
				latest = rec
			}
		}
	}
	return latest, nil
}

type fakeUserRepo struct {
	user *models.User
}

func (r *fakeUserRepo) GetByID(id int) (*models.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, nil
	}
	return r.user, nil
}

func (r *fakeUserRepo) Create(*models.User) error                  { return nil }
func (r *fakeUserRepo) Update(*models.User) error                  { return nil }
func (r *fakeUserRepo) Delete(int) error                           { return nil }
func (r *fakeUserRepo) List(int, int) ([]*models.User, error)      { return nil, nil }
func (r *fakeUserRepo) GetByEmail(string) (*models.User, error)    { return nil, nil }
func (r *fakeUserRepo) GetCount() (int, error)                     { return 0, nil }
func (r *fakeUserRepo) GetCountByRole(int) (int, error)            { return 0, nil }
func (r *fakeUserRepo) UpdatePassword(int, string) error           { return nil }
func (r *fakeUserRepo) UpdateRefresh(int, string, time.Time) error { return nil }
func (r *fakeUserRepo) RotateRefresh(string, string, time.Time) (*models.User, error) {
	return nil, nil
}
func (r *fakeUserRepo) ClearRefresh(int) error                         { return nil }
func (r *fakeUserRepo) GetByRefreshToken(string) (*models.User, error) { return nil, nil }
func (r *fakeUserRepo) VerifyUser(int) error                           { return nil }
func (r *fakeUserRepo) UpdateTelegramLink(int, int64, bool) error      { return nil }
func (r *fakeUserRepo) GetByIDSimple(int) (*models.User, error)        { return nil, nil }
func (r *fakeUserRepo) GetTelegramSettings(context.Context, int64) (int64, bool, error) {
	return 0, false, nil
}
func (r *fakeUserRepo) GetByChatID(context.Context, int64) (*models.User, error) {
	return nil, nil
}

type fakeSignDocumentRepo struct {
	doc *models.Document
}

func (r *fakeSignDocumentRepo) GetByID(id int64) (*models.Document, error) {
	if r.doc == nil || r.doc.ID != id {
		return nil, nil
	}
	return r.doc, nil
}
func (r *fakeSignDocumentRepo) UpdateStatus(id int64, status string) error {
	if r.doc == nil || r.doc.ID != id {
		return errors.New("not found")
	}
	r.doc.Status = status
	return nil
}

type fakeDocumentSigner struct {
	docRepo *fakeSignDocumentRepo
	calls   int
}

func (s *fakeDocumentSigner) FinalizeSigning(docID int64) error {
	s.calls++
	if s.docRepo != nil {
		return s.docRepo.UpdateStatus(docID, "signed")
	}
	return nil
}

type fakeEmailSender struct {
	sent int
}

func (s *fakeEmailSender) SendSigningConfirm(string, SigningEmailData) error {
	s.sent++
	return nil
}

func TestStartSigningCreatesPendingAndCancelsPrevious(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	docRepo := &fakeSignDocumentRepo{doc: &models.Document{ID: 10, DocType: "contract", Status: "approved"}}
	userRepo := &fakeUserRepo{user: &models.User{ID: 7, Email: "user@example.com", TelegramChatID: 123}}
	emailSender := &fakeEmailSender{}
	docSigner := &fakeDocumentSigner{docRepo: docRepo}

	old := &models.SignatureConfirmation{
		ID:         "old",
		DocumentID: 10,
		UserID:     7,
		Channel:    "email",
		Status:     "pending",
		ExpiresAt:  now().Add(5 * time.Minute),
	}
	repo.records = append(repo.records, old)

	service := NewDocumentSigningConfirmationService(
		repo,
		userRepo,
		docRepo,
		docSigner,
		emailSender,
		nil,
		DocumentSigningConfirmationConfig{
			ConfirmPolicy:      SignConfirmPolicyAny,
			EmailTTL:           30 * time.Minute,
			EmailVerifyBaseURL: "http://example.com",
		},
		now,
	)
	if _, err := service.StartSigning(context.Background(), 10, 7, "client@example.com"); err != nil {
		t.Fatalf("start signing: %v", err)
	}
	if old.Status != "cancelled" {
		t.Fatalf("expected previous pending cancelled")
	}
	if len(repo.records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(repo.records))
	}
	if emailSender.sent != 1 {
		t.Fatalf("expected email sent once, got %d", emailSender.sent)
	}
}

func TestConfirmByEmailTokenHappyPath(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	tempDir := t.TempDir()
	fileName := "doc.pdf"
	if err := os.WriteFile(filepath.Join(tempDir, fileName), []byte("signed-content"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	docRepo := &fakeSignDocumentRepo{doc: &models.Document{ID: 22, DocType: "contract", Status: "approved", FilePathPdf: fileName}}
	userRepo := &fakeUserRepo{user: &models.User{ID: 9, Email: "user@example.com", TelegramChatID: 321}}
	docSigner := &fakeDocumentSigner{docRepo: docRepo}

	token := "token123"
	tokenHash := hashTokenForTest(token)
	code := "123456"
	codeHash, err := HashVerificationCode(code)
	if err != nil {
		t.Fatalf("hash code: %v", err)
	}
	meta := []byte(`{"signer_email":"client@example.com"}`)
	_, _ = repo.CreatePending(context.Background(), 22, 9, "email", &codeHash, &tokenHash, now().Add(10*time.Minute), meta)

	service := &DocumentSigningConfirmationService{
		repo:      repo,
		userRepo:  userRepo,
		docRepo:   docRepo,
		docSigner: docSigner,
		policy:    SignConfirmPolicyAny,
		now:       now,
		filesRoot: tempDir,
	}

	status, signerEmail, docHash, _, err := service.ConfirmByEmailToken(context.Background(), 22, token, code, "127.0.0.1", "UA")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if status != "approved" {
		t.Fatalf("expected approved, got %s", status)
	}
	if signerEmail != "client@example.com" {
		t.Fatalf("expected signer email, got %s", signerEmail)
	}
	if docHash == "" {
		t.Fatalf("expected doc hash")
	}
	if docRepo.doc.Status != "approved" {
		t.Fatalf("expected document still approved, got %s", docRepo.doc.Status)
	}
}

func TestConfirmByEmailTokenRepeatReturnsConflict(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 2, 13, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	tempDir := t.TempDir()
	fileName := "repeat.pdf"
	if err := os.WriteFile(filepath.Join(tempDir, fileName), []byte("repeat"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	docRepo := &fakeSignDocumentRepo{doc: &models.Document{ID: 23, DocType: "contract", Status: "approved", FilePathPdf: fileName}}
	docSigner := &fakeDocumentSigner{docRepo: docRepo}
	token := "repeat-token"
	tokenHash := hashTokenForTest(token)
	code := "654321"
	codeHash, err := HashVerificationCode(code)
	if err != nil {
		t.Fatalf("hash code: %v", err)
	}
	_, _ = repo.CreatePending(context.Background(), 23, 9, "email", &codeHash, &tokenHash, now().Add(10*time.Minute), nil)

	service := &DocumentSigningConfirmationService{
		repo:      repo,
		docRepo:   docRepo,
		docSigner: docSigner,
		policy:    SignConfirmPolicyAny,
		now:       now,
		filesRoot: tempDir,
	}
	if _, _, _, _, err := service.ConfirmByEmailToken(context.Background(), 23, token, code, "ip", "ua"); err != nil {
		t.Fatalf("confirm first: %v", err)
	}
	if _, _, _, _, err := service.ConfirmByEmailToken(context.Background(), 23, token, code, "ip", "ua"); !errors.Is(err, ErrSignConfirmAlreadyUsed) {
		t.Fatalf("expected already used error, got %v", err)
	}
}

func TestConfirmByEmailTokenExpired(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	tokenHash := hashTokenForTest("token-expired")
	codeHash, err := HashVerificationCode("123456")
	if err != nil {
		t.Fatalf("hash code: %v", err)
	}
	_, _ = repo.CreatePending(context.Background(), 1, 2, "email", &codeHash, &tokenHash, now().Add(-time.Minute), nil)

	service := &DocumentSigningConfirmationService{
		repo:      repo,
		policy:    SignConfirmPolicyAny,
		now:       now,
		filesRoot: t.TempDir(),
		docRepo:   &fakeSignDocumentRepo{doc: &models.Document{ID: 1, Status: "approved", FilePath: "missing.pdf"}},
	}
	_, _, _, _, err = service.ConfirmByEmailToken(context.Background(), 1, "token-expired", "123456", "ip", "ua")
	if !errors.Is(err, ErrSignConfirmExpired) {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestConfirmByEmailTokenAttemptsLimit(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 4, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	tokenHash := hashTokenForTest("token")
	codeHash, err := HashVerificationCode("123456")
	if err != nil {
		t.Fatalf("hash code: %v", err)
	}
	rec, _ := repo.CreatePending(context.Background(), 1, 2, "email", &codeHash, &tokenHash, now().Add(10*time.Minute), nil)
	rec.Attempts = 5

	service := &DocumentSigningConfirmationService{
		repo:      repo,
		policy:    SignConfirmPolicyAny,
		now:       now,
		filesRoot: t.TempDir(),
		docRepo:   &fakeSignDocumentRepo{doc: &models.Document{ID: 1, Status: "approved", FilePath: "missing.pdf"}},
	}
	_, _, _, _, err = service.ConfirmByEmailToken(context.Background(), 1, "token", "123456", "ip", "ua")
	if !errors.Is(err, ErrSignConfirmTooManyTries) {
		t.Fatalf("expected too many attempts error, got %v", err)
	}
	if rec.Status != "expired" {
		t.Fatalf("expected expired status, got %s", rec.Status)
	}
}

func TestConfirmByEmailTokenInvalidCodeIncrementsAttempts(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 4, 13, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	tokenHash := hashTokenForTest("token-invalid")
	codeHash, err := HashVerificationCode("123456")
	if err != nil {
		t.Fatalf("hash code: %v", err)
	}
	rec, _ := repo.CreatePending(context.Background(), 1, 2, "email", &codeHash, &tokenHash, now().Add(10*time.Minute), nil)

	service := &DocumentSigningConfirmationService{
		repo:      repo,
		policy:    SignConfirmPolicyAny,
		now:       now,
		filesRoot: t.TempDir(),
		docRepo:   &fakeSignDocumentRepo{doc: &models.Document{ID: 1, Status: "approved", FilePath: "missing.pdf"}},
	}
	_, _, _, _, err = service.ConfirmByEmailToken(context.Background(), 1, "token-invalid", "654321", "ip", "ua")
	if !errors.Is(err, ErrSignConfirmInvalidCode) {
		t.Fatalf("expected invalid code error, got %v", err)
	}
	if rec.Attempts != 1 {
		t.Fatalf("expected attempts to be 1, got %d", rec.Attempts)
	}
}

func TestValidateEmailTokenDoesNotSign(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 7, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	docRepo := &fakeSignDocumentRepo{doc: &models.Document{ID: 55, DocType: "contract", Status: "approved"}}
	token := "token-verify"
	tokenHash := hashTokenForTest(token)
	codeHash, err := HashVerificationCode("123456")
	if err != nil {
		t.Fatalf("hash code: %v", err)
	}
	_, _ = repo.CreatePending(context.Background(), 55, 9, "email", &codeHash, &tokenHash, now().Add(10*time.Minute), nil)

	service := &DocumentSigningConfirmationService{
		repo:    repo,
		docRepo: docRepo,
		now:     now,
	}

	payload, err := service.ValidateEmailToken(context.Background(), token, "127.0.0.1", "UA")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if payload.RequirePostConfirm != true {
		t.Fatalf("expected require_post_confirm true")
	}
	if docRepo.doc.Status != "approved" {
		t.Fatalf("expected document not signed on verify")
	}
}

func TestEmailSignFlowStartVerifyConfirmStatus(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 8, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	tempDir := t.TempDir()
	fileName := "flow.pdf"
	if err := os.WriteFile(filepath.Join(tempDir, fileName), []byte("content"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	docRepo := &fakeSignDocumentRepo{doc: &models.Document{ID: 77, DocType: "contract", Status: "approved", FilePathPdf: fileName}}
	userRepo := &fakeUserRepo{user: &models.User{ID: 5, Email: "sender@example.com"}}
	emailSender := &fakeEmailSender{}
	docSigner := &fakeDocumentSigner{docRepo: docRepo}

	service := NewDocumentSigningConfirmationService(
		repo,
		userRepo,
		docRepo,
		docSigner,
		emailSender,
		nil,
		DocumentSigningConfirmationConfig{
			ConfirmPolicy:      SignConfirmPolicyAny,
			EmailTTL:           30 * time.Minute,
			EmailVerifyBaseURL: "http://example.com",
			FilesRoot:          tempDir,
		},
		now,
	)
	service.EnableDebug("")

	if _, err := service.StartSigning(context.Background(), 77, 5, "client@example.com"); err != nil {
		t.Fatalf("start: %v", err)
	}
	debug, ok := service.DebugLatest(77, 5)
	if !ok || debug.EmailToken == "" {
		t.Fatalf("expected debug token")
	}
	if _, err := service.ValidateEmailToken(context.Background(), debug.EmailToken, "127.0.0.1", "UA"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	status, _, _, _, err := service.ConfirmByEmailToken(context.Background(), 77, debug.EmailToken, debug.EmailCode, "127.0.0.1", "UA")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if status != "approved" {
		t.Fatalf("expected approved status, got %s", status)
	}
}

func TestGenerateConfirmTokenUsesSha256(t *testing.T) {
	token, tokenHash, err := generateConfirmToken("")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if len(token) < 43 {
		t.Fatalf("expected base64url token length >= 43, got %d", len(token))
	}
	expected := hashConfirmToken(token)
	if tokenHash != expected {
		t.Fatalf("expected hash %s, got %s", expected, tokenHash)
	}
}

func TestGenerateConfirmTokenUsesPepper(t *testing.T) {
	token, tokenHash, err := generateConfirmToken("pepper-1")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if tokenHash != hashConfirmTokenWithPepper(token, "pepper-1") {
		t.Fatalf("unexpected token hash for peppered token")
	}
}

func TestValidateEmailTokenAcceptsEscapedAndTrimmedToken(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 9, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	docRepo := &fakeSignDocumentRepo{doc: &models.Document{ID: 90, DocType: "contract", Status: "approved"}}

	token := "abc+def"
	tokenHash := hashConfirmTokenWithPepper(token, "pep")
	codeHash, err := HashVerificationCode("123456")
	if err != nil {
		t.Fatalf("hash code: %v", err)
	}
	_, _ = repo.CreatePending(context.Background(), 90, 9, "email", &codeHash, &tokenHash, now().Add(10*time.Minute), nil)

	service := &DocumentSigningConfirmationService{
		repo:        repo,
		docRepo:     docRepo,
		tokenPepper: "pep",
		now:         now,
	}

	if _, err := service.ValidateEmailToken(context.Background(), "  abc%2Bdef  ", "127.0.0.1", "UA"); err != nil {
		t.Fatalf("validate token: %v", err)
	}
}

func hashTokenForTest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
