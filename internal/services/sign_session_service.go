package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"turcompany/internal/models"
	"turcompany/internal/utils"
)

var (
	ErrSignSessionNotFound      = errors.New("sign session not found")
	ErrSignSessionExpired       = errors.New("sign session expired")
	ErrSignSessionInvalidCode   = errors.New("invalid sign code")
	ErrSignSessionTooManyTries  = errors.New("too many attempts")
	ErrSignSessionNotVerified   = errors.New("sign session not verified")
	ErrSignSessionAlreadySigned = errors.New("sign session already signed")
	ErrSignSessionRateLimited   = errors.New("sign session rate limited")
)

const (
	signSessionTTL           = 10 * time.Minute
	signSessionMaxAttempts   = 5
	signSessionRateLimitMax  = 3
	signSessionRateLimitSpan = 10 * time.Minute
)

type SignSessionRepo interface {
	Create(session *models.SignSession) error
	GetByTokenHash(tokenHash string) (*models.SignSession, error)
	CountRecentByDocumentID(documentID int64, since time.Time) (int, error)
	CountRecentByPhone(phoneE164 string, since time.Time) (int, error)
	Update(session *models.SignSession) error
}

type SignSessionConfig struct {
	SignBaseURL  string
	DryRun       bool
	TokenVisible bool
}

type SignDocumentService interface {
	EnsureSigningAllowed(docID int64, userID, roleID int) error
	FinalizeSigning(docID int64) error
}

type SignSessionService struct {
	repo         SignSessionRepo
	docService   SignDocumentService
	delivery     SignDelivery
	signBaseURL  string
	dryRun       bool
	tokenVisible bool
	now          func() time.Time
}

func NewSignSessionService(
	repo SignSessionRepo,
	docService SignDocumentService,
	delivery SignDelivery,
	cfg SignSessionConfig,
	now func() time.Time,
) *SignSessionService {
	if now == nil {
		now = time.Now
	}
	return &SignSessionService{
		repo:         repo,
		docService:   docService,
		delivery:     delivery,
		signBaseURL:  strings.TrimSpace(cfg.SignBaseURL),
		dryRun:       cfg.DryRun,
		tokenVisible: cfg.TokenVisible,
		now:          now,
	}
}

func (s *SignSessionService) Create(ctx context.Context, documentID int64, phone string, userID, roleID int) (string, string, *models.SignSession, error) {
	if s.docService == nil {
		return "", "", nil, errors.New("document service unavailable")
	}
	if err := s.docService.EnsureSigningAllowed(documentID, userID, roleID); err != nil {
		return "", "", nil, err
	}

	recent, err := s.repo.CountRecentByDocumentID(documentID, s.now().Add(-signSessionRateLimitSpan))
	if err != nil {
		return "", "", nil, err
	}
	if recent >= signSessionRateLimitMax {
		return "", "", nil, ErrSignSessionRateLimited
	}

	code := GenerateVerificationCode()
	codeHash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return "", "", nil, fmt.Errorf("hash code: %w", err)
	}
	token, tokenHash, err := generateToken()
	if err != nil {
		return "", "", nil, err
	}

	phoneE164, err := utils.SanitizeE164Digits(phone)
	if err != nil {
		return "", "", nil, err
	}

	recentByPhone, err := s.repo.CountRecentByPhone(phoneE164, s.now().Add(-signSessionRateLimitSpan))
	if err != nil {
		return "", "", nil, err
	}
	if recentByPhone >= signSessionRateLimitMax {
		return "", "", nil, ErrSignSessionRateLimited
	}

	session := &models.SignSession{
		DocumentID: documentID,
		PhoneE164:  phoneE164,
		CodeHash:   string(codeHash),
		TokenHash:  tokenHash,
		ExpiresAt:  s.now().Add(signSessionTTL),
		Attempts:   0,
		Status:     "pending",
	}
	if err := s.repo.Create(session); err != nil {
		return "", "", nil, err
	}

	signURL, err := s.buildSignURL(token)
	if err != nil {
		return "", "", nil, err
	}

	if s.delivery == nil {
		return "", "", nil, errors.New("sign delivery unavailable")
	}
	if err := s.delivery.SendSignCode(ctx, phoneE164, code); err != nil {
		log.Printf("[sign][send][code] doc=%d err=%v", documentID, err)
		return "", "", nil, err
	}
	if err := s.delivery.SendSignLink(ctx, phoneE164, signURL); err != nil {
		log.Printf("[sign][send][link] doc=%d err=%v", documentID, err)
		return "", "", nil, err
	}

	log.Printf("[sign][session][created] doc=%d session=%d expires=%s",
		documentID, session.ID, session.ExpiresAt.Format(time.RFC3339))

	if !s.tokenVisible {
		token = ""
		signURL = ""
	}
	return token, signURL, session, nil
}

func (s *SignSessionService) Verify(ctx context.Context, token, code, ip, userAgent string) (*models.SignSession, error) {
	session, err := s.getByToken(token)
	if err != nil {
		return nil, err
	}
	if session.Status == "signed" {
		return session, ErrSignSessionAlreadySigned
	}
	if s.isExpired(session) {
		session.Status = "expired"
		_ = s.repo.Update(session)
		return nil, ErrSignSessionExpired
	}
	if session.Attempts >= signSessionMaxAttempts {
		session.Status = "expired"
		_ = s.repo.Update(session)
		return nil, ErrSignSessionTooManyTries
	}

	if err := bcrypt.CompareHashAndPassword([]byte(session.CodeHash), []byte(code)); err != nil {
		session.Attempts++
		if session.Attempts >= signSessionMaxAttempts {
			session.Status = "expired"
		}
		_ = s.repo.Update(session)
		return nil, ErrSignSessionInvalidCode
	}

	now := s.now()
	session.Status = "verified"
	session.VerifiedAt = &now
	if err := s.repo.Update(session); err != nil {
		return nil, err
	}

	log.Printf("[sign][session][verified] session=%d ip=%s", session.ID, ip)
	return session, nil
}

func (s *SignSessionService) Sign(ctx context.Context, token, ip, userAgent string) (*models.SignSession, error) {
	session, err := s.getByToken(token)
	if err != nil {
		return nil, err
	}
	if s.isExpired(session) {
		session.Status = "expired"
		_ = s.repo.Update(session)
		return nil, ErrSignSessionExpired
	}
	if session.Status == "signed" {
		return session, ErrSignSessionAlreadySigned
	}
	if session.Status != "verified" {
		return nil, ErrSignSessionNotVerified
	}

	now := s.now()
	session.Status = "signed"
	session.SignedAt = &now
	session.SignedIP = ip
	session.SignedUserAgent = userAgent

	if err := s.repo.Update(session); err != nil {
		return nil, err
	}

	if err := s.docService.FinalizeSigning(session.DocumentID); err != nil {
		return nil, err
	}

	log.Printf("[sign][session][signed] session=%d doc=%d ip=%s", session.ID, session.DocumentID, ip)
	return session, nil
}

func (s *SignSessionService) buildSignURL(token string) (string, error) {
	base := strings.TrimSpace(s.signBaseURL)
	if base == "" {
		return "", errors.New("SIGN_BASE_URL is required")
	}
	base = strings.TrimRight(base, "/")
	return fmt.Sprintf("%s/%s", base, token), nil
}

func (s *SignSessionService) isExpired(session *models.SignSession) bool {
	if session.Status == "expired" {
		return true
	}
	return s.now().After(session.ExpiresAt)
}

func (s *SignSessionService) getByToken(token string) (*models.SignSession, error) {
	hash := hashToken(token)
	session, err := s.repo.GetByTokenHash(hash)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrSignSessionNotFound
	}
	if subtle.ConstantTimeCompare([]byte(session.TokenHash), []byte(hash)) != 1 {
		return nil, ErrSignSessionNotFound
	}
	return session, nil
}

func generateToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("token rand: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashToken(token)
	return token, hash, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
