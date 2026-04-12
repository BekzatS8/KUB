package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type fakeSignSessionRepo struct {
	sessionByHash map[string]*models.SignSession
	byID          map[int64]*models.SignSession
	nextID        int64
}

func (r *fakeSignSessionRepo) Create(_ context.Context, s *models.SignSession) error {
	r.nextID++
	s.ID = r.nextID
	s.CreatedAt = time.Now().UTC()
	cp := *s
	if r.sessionByHash == nil {
		r.sessionByHash = map[string]*models.SignSession{}
		r.byID = map[int64]*models.SignSession{}
	}
	r.sessionByHash[s.TokenHash] = &cp
	r.byID[s.ID] = &cp
	return nil
}
func (r *fakeSignSessionRepo) GetByTokenHash(_ context.Context, h string) (*models.SignSession, error) {
	if s := r.sessionByHash[h]; s != nil {
		cp := *s
		return &cp, nil
	}
	return nil, nil
}
func (r *fakeSignSessionRepo) GetByID(_ context.Context, id int64) (*models.SignSession, error) {
	if s := r.byID[id]; s != nil {
		cp := *s
		return &cp, nil
	}
	return nil, nil
}
func (r *fakeSignSessionRepo) FindSignedByDocumentEmail(context.Context, int64, string) (*models.SignSession, error) {
	return nil, nil
}
func (r *fakeSignSessionRepo) CountRecentByDocumentID(context.Context, int64, time.Time) (int, error) {
	return 0, nil
}
func (r *fakeSignSessionRepo) CountRecentByPhone(context.Context, string, time.Time) (int, error) {
	return 0, nil
}
func (r *fakeSignSessionRepo) Update(_ context.Context, s *models.SignSession) error {
	cp := *s
	r.sessionByHash[s.TokenHash] = &cp
	r.byID[s.ID] = &cp
	return nil
}
func (r *fakeSignSessionRepo) IncrementAttempts(context.Context, int64) (int, error) { return 1, nil }

type fakeDocService struct {
	finalizeCalls         int
	finalizeArtifactCalls int
}

func (f *fakeDocService) EnsureSigningAllowed(int64, int, int) error { return nil }
func (f *fakeDocService) FinalizeSigning(int64) error {
	f.finalizeCalls++
	return nil
}
func (f *fakeDocService) FinalizeSignedArtifact(*models.SignSession) error {
	f.finalizeArtifactCalls++
	return nil
}

type fakeConfirmRepo struct {
	createdExpiresAt time.Time
	createdID        string
	item             *models.SignatureConfirmation
}

func (r *fakeConfirmRepo) CreatePending(_ context.Context, documentID, userID int64, channel string, otpHash *string, tokenHash *string, expiresAt time.Time, meta []byte) (*models.SignatureConfirmation, error) {
	r.createdExpiresAt = expiresAt
	r.createdID = "c-1"
	r.item = &models.SignatureConfirmation{ID: "c-1", DocumentID: documentID, UserID: userID, Channel: channel, Status: "pending", OTPHash: otpHash, TokenHash: tokenHash, ExpiresAt: expiresAt, Meta: meta}
	return r.item, nil
}
func (r *fakeConfirmRepo) FindPending(context.Context, int64, int64, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (r *fakeConfirmRepo) FindPendingByTokenHash(context.Context, string, string) (*models.SignatureConfirmation, error) {
	return nil, nil
}
func (r *fakeConfirmRepo) FindByTokenHash(_ context.Context, _ string, _ string) (*models.SignatureConfirmation, error) {
	return r.item, nil
}
func (r *fakeConfirmRepo) Approve(_ context.Context, _ string, _ []byte) (*models.SignatureConfirmation, error) {
	now := time.Now().UTC()
	r.item.Status = "approved"
	r.item.ApprovedAt = &now
	return r.item, nil
}
func (r *fakeConfirmRepo) Reject(context.Context, string, []byte) (*models.SignatureConfirmation, error) {
	return nil, errors.New("not implemented")
}
func (r *fakeConfirmRepo) CancelPrevious(context.Context, int64, int64, string) (int64, error) {
	return 0, nil
}
func (r *fakeConfirmRepo) IncrementAttempts(context.Context, string) (int, error) { return 1, nil }
func (r *fakeConfirmRepo) Expire(context.Context, string) error                   { return nil }
func (r *fakeConfirmRepo) HasApproved(context.Context, int64, int64, string) (bool, error) {
	return false, nil
}
func (r *fakeConfirmRepo) GetLatestByChannel(context.Context, int64, int64, string) (*models.SignatureConfirmation, error) {
	return r.item, nil
}
func (r *fakeConfirmRepo) UpdateMeta(context.Context, string, []byte) (*models.SignatureConfirmation, error) {
	return r.item, nil
}

type fakeUserRepo struct{}

func (f *fakeUserRepo) Create(*models.User) error { return nil }
func (f *fakeUserRepo) GetByID(id int) (*models.User, error) {
	return &models.User{ID: id, CompanyName: "Acme", Email: "u@test"}, nil
}
func (f *fakeUserRepo) Update(*models.User) error                      { return nil }
func (f *fakeUserRepo) Delete(int) error                               { return nil }
func (f *fakeUserRepo) List(limit, offset int) ([]*models.User, error) { return nil, nil }
func (f *fakeUserRepo) GetByEmail(string) (*models.User, error)        { return nil, nil }
func (f *fakeUserRepo) GetCount() (int, error)                         { return 0, nil }
func (f *fakeUserRepo) GetCountByRole(int) (int, error)                { return 0, nil }
func (f *fakeUserRepo) UpdatePassword(int, string) error               { return nil }
func (f *fakeUserRepo) UpdateRefresh(int, string, time.Time) error     { return nil }
func (f *fakeUserRepo) RotateRefresh(string, string, time.Time) (*models.User, error) {
	return nil, nil
}
func (f *fakeUserRepo) ClearRefresh(int) error                         { return nil }
func (f *fakeUserRepo) GetByRefreshToken(string) (*models.User, error) { return nil, nil }
func (f *fakeUserRepo) VerifyUser(int) error                           { return nil }
func (f *fakeUserRepo) UpdateTelegramLink(int, int64, bool) error      { return nil }
func (f *fakeUserRepo) GetByIDSimple(int) (*models.User, error)        { return nil, nil }
func (f *fakeUserRepo) GetTelegramSettings(context.Context, int64) (int64, bool, error) {
	return 0, false, nil
}
func (f *fakeUserRepo) GetByChatID(context.Context, int64) (*models.User, error) { return nil, nil }

type fakeDocLookup struct{}

func (f *fakeDocLookup) GetByID(id int64) (*models.Document, error) {
	return &models.Document{ID: id, DocType: "contract"}, nil
}

type fakeEmailSender struct{}

func (f *fakeEmailSender) SendSigningConfirm(string, SigningEmailData) error { return nil }

func TestStartSigningCreatesExpectedExpiresAt(t *testing.T) {
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	repo := &fakeConfirmRepo{}
	svc := NewDocumentSigningConfirmationService(
		repo,
		&fakeUserRepo{},
		&fakeDocLookup{},
		nil,
		&fakeEmailSender{},
		nil,
		DocumentSigningConfirmationConfig{EmailTTL: 30 * time.Minute, EmailVerifyBaseURL: "http://localhost:4000", ServerTZ: time.UTC},
		func() time.Time { return now },
	)
	_, err := svc.StartSigning(context.Background(), 10, 20, "signer@example.com")
	if err != nil {
		t.Fatalf("StartSigning error: %v", err)
	}
	want := now.Add(30 * time.Minute)
	if !repo.createdExpiresAt.Equal(want) {
		t.Fatalf("unexpected expires_at: got=%s want=%s", repo.createdExpiresAt, want)
	}
}

func TestCreateEmailSessionUsesConfiguredTTL(t *testing.T) {
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	repo := &fakeSignSessionRepo{}
	svc := NewSignSessionService(repo, &fakeDocService{}, nil, SignSessionConfig{SessionTTL: 30 * time.Minute, ServerTZ: time.UTC}, func() time.Time { return now })
	_, session, err := svc.CreateEmailSession(context.Background(), 15, "s@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateEmailSession error: %v", err)
	}
	if !session.ExpiresAt.Equal(now.Add(30 * time.Minute)) {
		t.Fatalf("session ttl mismatch: got=%s want=%s", session.ExpiresAt, now.Add(30*time.Minute))
	}
}

func TestSessionExpiryBoundary(t *testing.T) {
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	svc := NewSignSessionService(&fakeSignSessionRepo{}, &fakeDocService{}, nil, SignSessionConfig{SessionTTL: 30 * time.Minute, ServerTZ: time.UTC}, func() time.Time { return now })
	session := &models.SignSession{ExpiresAt: now.Add(30 * time.Minute), Status: "pending"}
	if svc.isExpired(session) {
		t.Fatalf("session should be valid before ttl")
	}
	svc.now = func() time.Time { return now.Add(31 * time.Minute) }
	if !svc.isExpired(session) {
		t.Fatalf("session should be expired after ttl")
	}
}

func TestPublicSigningDefaultTTL(t *testing.T) {
	svc := NewPublicDocumentSigningService(nil, nil, &repositories.DocumentRepository{}, PublicDocumentSigningConfig{TTLMinutes: 0, ServerTZ: time.UTC}, time.Now)
	if svc.ttlMinutes != 60 {
		t.Fatalf("unexpected default public ttl: %d", svc.ttlMinutes)
	}
}

func TestSigningSheetUsesConfiguredTZ(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Almaty")
	ts := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	got := formatSignedAtForSigningSheet(&ts, loc)
	want := "01.04.2026 15:00 (Asia/Almaty)"
	if got != want {
		t.Fatalf("unexpected signedAt format: got=%q want=%q", got, want)
	}
}

func TestSignSessionFallbackTTL(t *testing.T) {
	svc := NewSignSessionService(&fakeSignSessionRepo{}, &fakeDocService{}, nil, SignSessionConfig{SessionTTL: 0}, time.Now)
	if svc.sessionTTL != 30*time.Minute {
		t.Fatalf("unexpected fallback ttl: %s", svc.sessionTTL)
	}
}

func TestConfirmMetaKeepsDocHash(t *testing.T) {
	meta := map[string]any{"document_hash": "abc"}
	raw, _ := json.Marshal(meta)
	if extractSignerEmail(raw) != "" {
		t.Fatalf("unexpected signer email")
	}
}

func TestSignByIDRepeatedFinalizeIsStable(t *testing.T) {
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	repo := &fakeSignSessionRepo{}
	docSvc := &fakeDocService{}
	svc := NewSignSessionService(repo, docSvc, nil, SignSessionConfig{SessionTTL: 30 * time.Minute, ServerTZ: time.UTC}, func() time.Time { return now })

	token, session, err := svc.CreateEmailSession(context.Background(), 25, "client@example.com", "doc-hash")
	if err != nil {
		t.Fatalf("CreateEmailSession error: %v", err)
	}
	if _, err := svc.SignByID(context.Background(), session.ID, token, "127.0.0.1", "ua"); err != nil {
		t.Fatalf("first SignByID failed: %v", err)
	}
	if docSvc.finalizeCalls != 1 || docSvc.finalizeArtifactCalls != 1 {
		t.Fatalf("unexpected finalize calls after first sign: finalize=%d artifact=%d", docSvc.finalizeCalls, docSvc.finalizeArtifactCalls)
	}

	_, err = svc.SignByID(context.Background(), session.ID, token, "127.0.0.1", "ua")
	if !errors.Is(err, ErrSignSessionAlreadySigned) {
		t.Fatalf("expected ErrSignSessionAlreadySigned, got %v", err)
	}
	if docSvc.finalizeCalls != 1 || docSvc.finalizeArtifactCalls != 1 {
		t.Fatalf("finalize should not repeat: finalize=%d artifact=%d", docSvc.finalizeCalls, docSvc.finalizeArtifactCalls)
	}
}
