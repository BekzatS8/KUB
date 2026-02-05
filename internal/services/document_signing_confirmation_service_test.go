package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

func (r *fakeSignatureConfirmRepo) Approve(_ context.Context, id string, _ []byte) (*models.SignatureConfirmation, error) {
	for _, rec := range r.records {
		if rec.ID == id {
			rec.Status = "approved"
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

func (r *fakeSignatureConfirmRepo) CancelPrevious(_ context.Context, documentID, userID int64) (int64, error) {
	var count int64
	for _, rec := range r.records {
		if rec.DocumentID == documentID && rec.UserID == userID && rec.Status == "pending" {
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

type fakeDocumentRepo struct {
	doc *models.Document
}

func (r *fakeDocumentRepo) GetByID(id int64) (*models.Document, error) {
	if r.doc == nil || r.doc.ID != id {
		return nil, nil
	}
	return r.doc, nil
}
func (r *fakeDocumentRepo) UpdateStatus(id int64, status string) error {
	if r.doc == nil || r.doc.ID != id {
		return errors.New("not found")
	}
	r.doc.Status = status
	return nil
}

type fakeDocumentSigner struct {
	docRepo *fakeDocumentRepo
	calls   int
}

func (s *fakeDocumentSigner) SignBySMS(docID int64) error {
	s.calls++
	if s.docRepo != nil {
		return s.docRepo.UpdateStatus(docID, "signed")
	}
	return nil
}

type fakeEmailSender struct {
	sent int
}

func (s *fakeEmailSender) SendSigningConfirm(string, string, string) error {
	s.sent++
	return nil
}

type fakeTelegramSender struct {
	sent int
}

func (s *fakeTelegramSender) SendSigningConfirm(int64, string, string, string) error {
	s.sent++
	return nil
}

func TestStartSigningCreatesPendingAndCancelsPrevious(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	docRepo := &fakeDocumentRepo{doc: &models.Document{ID: 10, DocType: "contract", Status: "approved"}}
	userRepo := &fakeUserRepo{user: &models.User{ID: 7, Email: "user@example.com", TelegramChatID: 123}}
	emailSender := &fakeEmailSender{}
	tgSender := &fakeTelegramSender{}
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
		tgSender,
		DocumentSigningConfirmationConfig{ConfirmPolicy: SignConfirmPolicyAny},
		now,
	)
	if _, err := service.StartSigning(context.Background(), 10, 7); err != nil {
		t.Fatalf("start signing: %v", err)
	}
	if old.Status != "cancelled" {
		t.Fatalf("expected previous pending cancelled")
	}
	if len(repo.records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(repo.records))
	}
	if emailSender.sent != 1 {
		t.Fatalf("expected email sent once, got %d", emailSender.sent)
	}
	if tgSender.sent != 1 {
		t.Fatalf("expected telegram sent once, got %d", tgSender.sent)
	}
}

func TestConfirmByEmailCodeHappyPath(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	docRepo := &fakeDocumentRepo{doc: &models.Document{ID: 22, DocType: "contract", Status: "approved"}}
	userRepo := &fakeUserRepo{user: &models.User{ID: 9, Email: "user@example.com", TelegramChatID: 321}}
	docSigner := &fakeDocumentSigner{docRepo: docRepo}

	code := "123456"
	codeHash, err := HashVerificationCode(code)
	if err != nil {
		t.Fatalf("hash code: %v", err)
	}
	_, _ = repo.CreatePending(context.Background(), 22, 9, "email", &codeHash, nil, now().Add(10*time.Minute), nil)

	service := &DocumentSigningConfirmationService{
		repo:      repo,
		userRepo:  userRepo,
		docRepo:   docRepo,
		docSigner: docSigner,
		policy:    SignConfirmPolicyAny,
		now:       now,
	}

	confirmation, err := service.ConfirmByEmailCode(context.Background(), 22, 9, code, "127.0.0.1", "UA")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if confirmation.Status != "approved" {
		t.Fatalf("expected approved, got %s", confirmation.Status)
	}
	if docRepo.doc.Status != "signed" {
		t.Fatalf("expected document signed, got %s", docRepo.doc.Status)
	}
}

func TestConfirmByEmailCodeExpired(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	codeHash, _ := HashVerificationCode("123456")
	_, _ = repo.CreatePending(context.Background(), 1, 2, "email", &codeHash, nil, now().Add(-time.Minute), nil)

	service := &DocumentSigningConfirmationService{
		repo:   repo,
		policy: SignConfirmPolicyAny,
		now:    now,
	}
	_, err := service.ConfirmByEmailCode(context.Background(), 1, 2, "123456", "ip", "ua")
	if !errors.Is(err, ErrSignConfirmExpired) {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestConfirmByEmailCodeAttemptsLimit(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 4, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	codeHash, _ := HashVerificationCode("123456")
	rec, _ := repo.CreatePending(context.Background(), 1, 2, "email", &codeHash, nil, now().Add(10*time.Minute), nil)
	rec.Attempts = 4

	service := &DocumentSigningConfirmationService{
		repo:   repo,
		policy: SignConfirmPolicyAny,
		now:    now,
	}
	_, err := service.ConfirmByEmailCode(context.Background(), 1, 2, "000000", "ip", "ua")
	if !errors.Is(err, ErrSignConfirmTooManyTries) {
		t.Fatalf("expected too many attempts error, got %v", err)
	}
	if rec.Status != "expired" {
		t.Fatalf("expected expired status, got %s", rec.Status)
	}
}

func TestConfirmByTelegramCallbackApproveReject(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 5, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	docRepo := &fakeDocumentRepo{doc: &models.Document{ID: 7, DocType: "contract", Status: "approved"}}
	docSigner := &fakeDocumentSigner{docRepo: docRepo}

	token := "token123"
	tokenHash := hashTokenForTest(token)
	_, _ = repo.CreatePending(context.Background(), 7, 11, "telegram", nil, &tokenHash, now().Add(10*time.Minute), nil)

	service := &DocumentSigningConfirmationService{
		repo:      repo,
		docSigner: docSigner,
		policy:    SignConfirmPolicyAny,
		now:       now,
	}
	confirmation, err := service.ConfirmByTelegramCallback(context.Background(), token, "approve", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if confirmation.Status != "approved" {
		t.Fatalf("expected approved, got %s", confirmation.Status)
	}

	tokenReject := "token456"
	tokenRejectHash := hashTokenForTest(tokenReject)
	_, _ = repo.CreatePending(context.Background(), 7, 11, "telegram", nil, &tokenRejectHash, now().Add(10*time.Minute), nil)
	confirmation, err = service.ConfirmByTelegramCallback(context.Background(), tokenReject, "reject", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if confirmation.Status != "rejected" {
		t.Fatalf("expected rejected, got %s", confirmation.Status)
	}
}

func TestPolicyBothRequiresBothChannels(t *testing.T) {
	now := func() time.Time { return time.Date(2025, 1, 6, 12, 0, 0, 0, time.UTC) }
	repo := newFakeSignatureConfirmRepo(now)
	docRepo := &fakeDocumentRepo{doc: &models.Document{ID: 33, DocType: "contract", Status: "approved"}}
	docSigner := &fakeDocumentSigner{docRepo: docRepo}

	codeHash, _ := HashVerificationCode("123456")
	_, _ = repo.CreatePending(context.Background(), 33, 44, "email", &codeHash, nil, now().Add(10*time.Minute), nil)
	token := "token789"
	tokenHash := hashTokenForTest(token)
	_, _ = repo.CreatePending(context.Background(), 33, 44, "telegram", nil, &tokenHash, now().Add(10*time.Minute), nil)

	service := &DocumentSigningConfirmationService{
		repo:      repo,
		docSigner: docSigner,
		policy:    SignConfirmPolicyBoth,
		now:       now,
	}

	if _, err := service.ConfirmByEmailCode(context.Background(), 33, 44, "123456", "ip", "ua"); err != nil {
		t.Fatalf("email confirm: %v", err)
	}
	if docRepo.doc.Status == "signed" {
		t.Fatalf("expected not signed after email only")
	}

	if _, err := service.ConfirmByTelegramCallback(context.Background(), token, "approve", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("telegram confirm: %v", err)
	}
	if docRepo.doc.Status != "signed" {
		t.Fatalf("expected signed after both approvals")
	}
}

func hashTokenForTest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
