package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"turcompany/internal/models"
)

type fakeSignRepo struct {
	sessions map[string]*models.SignSession
}

func newFakeSignRepo() *fakeSignRepo {
	return &fakeSignRepo{sessions: make(map[string]*models.SignSession)}
}

func (r *fakeSignRepo) Create(session *models.SignSession) error {
	if session.TokenHash == "" {
		return errors.New("token hash required")
	}
	if session.CreatedAt.IsZero() {
		now := time.Now()
		session.CreatedAt = now
		session.UpdatedAt = now
	}
	r.sessions[session.TokenHash] = session
	return nil
}

func (r *fakeSignRepo) GetByTokenHash(tokenHash string) (*models.SignSession, error) {
	if session, ok := r.sessions[tokenHash]; ok {
		return session, nil
	}
	return nil, nil
}

func (r *fakeSignRepo) CountRecentByDocumentID(documentID int64, since time.Time) (int, error) {
	count := 0
	for _, session := range r.sessions {
		if session.DocumentID == documentID && !session.CreatedAt.Before(since) {
			count++
		}
	}
	return count, nil
}

func (r *fakeSignRepo) CountRecentByPhone(phoneE164 string, since time.Time) (int, error) {
	count := 0
	for _, session := range r.sessions {
		if session.PhoneE164 == phoneE164 && !session.CreatedAt.Before(since) {
			count++
		}
	}
	return count, nil
}

func (r *fakeSignRepo) Update(session *models.SignSession) error {
	if session.TokenHash == "" {
		return errors.New("token hash required")
	}
	session.UpdatedAt = time.Now()
	r.sessions[session.TokenHash] = session
	return nil
}

type fakeSignDelivery struct{}

func (d *fakeSignDelivery) SendSignCode(ctx context.Context, phoneE164, code string) error {
	return nil
}

func (d *fakeSignDelivery) SendSignLink(ctx context.Context, phoneE164, url string) error {
	return nil
}

type fakeSignDocService struct {
	allowed    bool
	signedDoc  int64
	allowError error
}

func (s *fakeSignDocService) EnsureSMSAllowed(docID int64, userID, roleID int) error {
	if s.allowError != nil {
		return s.allowError
	}
	if !s.allowed {
		return errors.New("forbidden")
	}
	return nil
}

func (s *fakeSignDocService) SignBySMS(docID int64) error {
	s.signedDoc = docID
	return nil
}

func TestSignSessionVerifyAttempts(t *testing.T) {
	repo := newFakeSignRepo()
	codeHash, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	token := "token-123"
	tokenHash := hashToken(token)
	session := &models.SignSession{
		DocumentID: 1,
		PhoneE164:  "77001234567",
		CodeHash:   string(codeHash),
		TokenHash:  tokenHash,
		ExpiresAt:  time.Now().Add(10 * time.Minute),
		Status:     "pending",
	}
	repo.Create(session)

	svc := NewSignSessionService(repo, &fakeSignDocService{allowed: true}, &fakeSignDelivery{}, SignSessionConfig{}, time.Now)

	for i := 0; i < signSessionMaxAttempts-1; i++ {
		if _, err := svc.Verify(context.Background(), token, "000000", "ip", "ua"); !errors.Is(err, ErrSignSessionInvalidCode) {
			t.Fatalf("expected invalid code, got %v", err)
		}
	}
	if _, err := svc.Verify(context.Background(), token, "000000", "ip", "ua"); !errors.Is(err, ErrSignSessionInvalidCode) {
		t.Fatalf("expected invalid code on last attempt, got %v", err)
	}
	if _, err := svc.Verify(context.Background(), token, "000000", "ip", "ua"); !errors.Is(err, ErrSignSessionTooManyTries) {
		t.Fatalf("expected too many attempts, got %v", err)
	}
}

func TestSignSessionVerifyExpired(t *testing.T) {
	repo := newFakeSignRepo()
	codeHash, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	token := "token-expired"
	tokenHash := hashToken(token)
	session := &models.SignSession{
		DocumentID: 2,
		PhoneE164:  "77001230000",
		CodeHash:   string(codeHash),
		TokenHash:  tokenHash,
		ExpiresAt:  time.Now().Add(-time.Minute),
		Status:     "pending",
	}
	repo.Create(session)

	svc := NewSignSessionService(repo, &fakeSignDocService{allowed: true}, &fakeSignDelivery{}, SignSessionConfig{}, time.Now)

	if _, err := svc.Verify(context.Background(), token, "123456", "ip", "ua"); !errors.Is(err, ErrSignSessionExpired) {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestSignSessionSignFlow(t *testing.T) {
	repo := newFakeSignRepo()
	codeHash, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	token := "token-ok"
	tokenHash := hashToken(token)
	session := &models.SignSession{
		DocumentID: 3,
		PhoneE164:  "77001230001",
		CodeHash:   string(codeHash),
		TokenHash:  tokenHash,
		ExpiresAt:  time.Now().Add(10 * time.Minute),
		Status:     "verified",
	}
	repo.Create(session)

	docSvc := &fakeSignDocService{allowed: true}
	svc := NewSignSessionService(repo, docSvc, &fakeSignDelivery{}, SignSessionConfig{}, time.Now)

	if _, err := svc.Sign(context.Background(), token, "ip", "ua"); err != nil {
		t.Fatalf("expected sign to succeed, got %v", err)
	}
	if docSvc.signedDoc != session.DocumentID {
		t.Fatalf("expected signed doc %d, got %d", session.DocumentID, docSvc.signedDoc)
	}
}

func TestSignSessionCreateRateLimit(t *testing.T) {
	repo := newFakeSignRepo()
	now := time.Now()
	for i := 0; i < signSessionRateLimitMax; i++ {
		repo.Create(&models.SignSession{
			DocumentID: 5,
			PhoneE164:  "77009990000",
			TokenHash:  hashToken(time.Now().Add(time.Duration(i)).String()),
			CreatedAt:  now,
			ExpiresAt:  now.Add(10 * time.Minute),
			Status:     "pending",
		})
	}
	docSvc := &fakeSignDocService{allowed: true}
	svc := NewSignSessionService(repo, docSvc, &fakeSignDelivery{}, SignSessionConfig{SignBaseURL: "https://example.com/sign"}, func() time.Time { return now })

	if _, _, _, err := svc.Create(context.Background(), 5, "+77009990000", 1, 1); !errors.Is(err, ErrSignSessionRateLimited) {
		t.Fatalf("expected rate limited error, got %v", err)
	}
}
