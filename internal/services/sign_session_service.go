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
	"net/url"
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
	ErrSignSessionInvalidPhone  = errors.New("invalid sign session phone")
	ErrSignSessionDocNotFound   = errors.New("sign session document not found")
	ErrSignSessionForbidden     = errors.New("sign session forbidden")
	ErrSignSessionInvalidStatus = errors.New("sign session invalid status")
	ErrSignSessionBaseURL       = errors.New("sign session base url required")
	ErrSignSessionDelivery      = errors.New("sign session delivery unavailable")
	ErrSignSessionInvalidToken  = errors.New("sign session invalid token")
	ErrSignSessionInvalidEmail  = errors.New("sign session invalid email")
)

const (
	signSessionMaxAttempts   = 5
	signSessionRateLimitMax  = 3
	signSessionRateLimitSpan = 10 * time.Minute
)

type SignSessionRepo interface {
	Create(ctx context.Context, session *models.SignSession) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*models.SignSession, error)
	GetByID(ctx context.Context, id int64) (*models.SignSession, error)
	FindSignedByDocumentEmail(ctx context.Context, documentID int64, signerEmail string) (*models.SignSession, error)
	CountRecentByDocumentID(ctx context.Context, documentID int64, since time.Time) (int, error)
	CountRecentByPhone(ctx context.Context, phoneE164 string, since time.Time) (int, error)
	Update(ctx context.Context, session *models.SignSession) error
	IncrementAttempts(ctx context.Context, id int64) (int, error)
}

type SignSessionConfig struct {
	SignBaseURL  string
	DryRun       bool
	TokenVisible bool
	SessionTTL   time.Duration
	ServerTZ     *time.Location
}

type SignDocumentService interface {
	EnsureSigningAllowed(docID int64, userID, roleID int) error
	FinalizeSigning(docID int64) error
	FinalizeSignedArtifact(session *models.SignSession) error
}

type SignSessionService struct {
	repo         SignSessionRepo
	docService   SignDocumentService
	delivery     SignDelivery
	signBaseURL  string
	dryRun       bool
	tokenVisible bool
	sessionTTL   time.Duration
	serverTZ     *time.Location
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
	sessionTTL := cfg.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = 30 * time.Minute
	}
	serverTZ := cfg.ServerTZ
	if serverTZ == nil {
		serverTZ = time.UTC
	}
	return &SignSessionService{
		repo:         repo,
		docService:   docService,
		delivery:     delivery,
		signBaseURL:  strings.TrimSpace(cfg.SignBaseURL),
		dryRun:       cfg.DryRun,
		tokenVisible: cfg.TokenVisible,
		sessionTTL:   sessionTTL,
		serverTZ:     serverTZ,
		now:          now,
	}
}

func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, 5*time.Second)
}

func (s *SignSessionService) Create(ctx context.Context, documentID int64, phone string, userID, roleID int) (string, string, *models.SignSession, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	if s.docService == nil {
		return "", "", nil, errors.New("document service unavailable")
	}
	if err := s.docService.EnsureSigningAllowed(documentID, userID, roleID); err != nil {
		switch err.Error() {
		case "not found":
			return "", "", nil, ErrSignSessionDocNotFound
		case "forbidden":
			return "", "", nil, ErrSignSessionForbidden
		case "invalid status":
			return "", "", nil, ErrSignSessionInvalidStatus
		default:
			return "", "", nil, err
		}
	}

	recent, err := s.repo.CountRecentByDocumentID(ctx, documentID, s.now().Add(-signSessionRateLimitSpan))
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
		return "", "", nil, ErrSignSessionInvalidPhone
	}

	recentByPhone, err := s.repo.CountRecentByPhone(ctx, phoneE164, s.now().Add(-signSessionRateLimitSpan))
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
		ExpiresAt:  s.now().Add(s.sessionTTL),
		Attempts:   0,
		Status:     "pending",
	}
	if err := s.repo.Create(ctx, session); err != nil {
		return "", "", nil, err
	}

	signURL, err := s.buildSignURL(token)
	if err != nil {
		if errors.Is(err, ErrSignSessionBaseURL) {
			return "", "", nil, err
		}
		return "", "", nil, err
	}

	if s.delivery == nil {
		return "", "", nil, ErrSignSessionDelivery
	}
	if err := s.delivery.SendSignCode(ctx, phoneE164, code); err != nil {
		log.Printf("[sign][send][code] doc=%d err=%v", documentID, err)
		return "", "", nil, fmt.Errorf("send sign code: %w", err)
	}
	if err := s.delivery.SendSignLink(ctx, phoneE164, signURL); err != nil {
		log.Printf("[sign][send][link] doc=%d err=%v", documentID, err)
		return "", "", nil, fmt.Errorf("send sign link: %w", err)
	}

	s.logSessionState("created", session, "init")

	if !s.tokenVisible {
		token = ""
		signURL = ""
	}
	return token, signURL, session, nil
}

func (s *SignSessionService) CreateEmailSession(ctx context.Context, documentID int64, signerEmail, docHash string) (string, *models.SignSession, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	signerEmail = strings.TrimSpace(signerEmail)
	if signerEmail == "" {
		return "", nil, ErrSignSessionInvalidEmail
	}
	if signed, err := s.repo.FindSignedByDocumentEmail(ctx, documentID, signerEmail); err != nil {
		return "", nil, err
	} else if signed != nil {
		return "", nil, ErrSignSessionAlreadySigned
	}
	token, tokenHash, err := generateToken()
	if err != nil {
		return "", nil, err
	}
	session := &models.SignSession{
		DocumentID:  documentID,
		SignerEmail: signerEmail,
		DocHash:     strings.TrimSpace(docHash),
		TokenHash:   tokenHash,
		ExpiresAt:   s.now().Add(s.sessionTTL),
		Attempts:    0,
		Status:      "pending",
	}
	if err := s.repo.Create(ctx, session); err != nil {
		return "", nil, err
	}
	s.logSessionState("created", session, "email_confirmed")
	return token, session, nil
}

func (s *SignSessionService) Verify(ctx context.Context, token, code, ip, userAgent string) (*models.SignSession, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	session, err := s.getByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if session.Status == "signed" {
		return session, ErrSignSessionAlreadySigned
	}
	if session.Status == "expired" {
		if session.Attempts >= signSessionMaxAttempts {
			return nil, ErrSignSessionTooManyTries
		}
		return nil, ErrSignSessionExpired
	}
	if s.isExpired(session) {
		s.expireSession(ctx, session, "ttl_elapsed")
		return nil, ErrSignSessionExpired
	}
	if session.Attempts >= signSessionMaxAttempts {
		s.expireSession(ctx, session, "too_many_attempts")
		return nil, ErrSignSessionTooManyTries
	}

	if err := bcrypt.CompareHashAndPassword([]byte(session.CodeHash), []byte(code)); err != nil {
		session.Attempts++
		if session.Attempts >= signSessionMaxAttempts {
			session.Status = "expired"
			s.logSessionState("transition", session, "too_many_attempts_after_invalid_code")
		}
		_ = s.repo.Update(ctx, session)
		return nil, ErrSignSessionInvalidCode
	}

	now := s.now()
	session.Status = "verified"
	session.VerifiedAt = &now
	if err := s.repo.Update(ctx, session); err != nil {
		return nil, err
	}

	s.logSessionState("verified", session, "otp_valid")
	return session, nil
}

func (s *SignSessionService) Sign(ctx context.Context, token, ip, userAgent string) (*models.SignSession, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	session, err := s.getByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if s.isExpired(session) {
		s.expireSession(ctx, session, "ttl_elapsed_before_sign")
		return nil, ErrSignSessionExpired
	}
	if session.Status == "signed" {
		return session, ErrSignSessionAlreadySigned
	}
	if session.Status != "verified" {
		return nil, ErrSignSessionNotVerified
	}

	now := s.now()
	session.SignedAt = &now
	session.SignedIP = ip
	session.SignedUserAgent = userAgent

	if err := s.docService.FinalizeSignedArtifact(session); err != nil {
		if errors.Is(err, ErrDocumentChangedAfterOTP) {
			s.expireSession(ctx, session, "document_changed_after_otp")
		}
		return nil, err
	}

	if err := s.docService.FinalizeSigning(session.DocumentID); err != nil {
		return nil, err
	}
	session.Status = "signed"
	if err := s.repo.Update(ctx, session); err != nil {
		return nil, err
	}

	s.logSessionState("signed", session, "signed_success")
	return session, nil
}

func (s *SignSessionService) ValidateSessionForPage(ctx context.Context, sessionID int64, token string) (*models.SignSession, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return s.validateByIDToken(ctx, sessionID, token)
}

func (s *SignSessionService) SignByID(ctx context.Context, sessionID int64, token, ip, userAgent string) (*models.SignSession, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	session, err := s.validateByIDToken(ctx, sessionID, token)
	if err != nil {
		return nil, err
	}
	if session.Status == "signed" {
		return session, ErrSignSessionAlreadySigned
	}

	now := s.now()
	session.SignedAt = &now
	session.SignedIP = ip
	session.SignedUserAgent = userAgent

	if s.docService == nil {
		return nil, errors.New("document service unavailable")
	}
	if err := s.docService.FinalizeSignedArtifact(session); err != nil {
		if errors.Is(err, ErrDocumentChangedAfterOTP) {
			s.expireSession(ctx, session, "document_changed_after_otp")
		}
		return nil, err
	}
	if err := s.docService.FinalizeSigning(session.DocumentID); err != nil {
		switch err.Error() {
		case "not found":
			return nil, ErrSignSessionDocNotFound
		case "invalid status":
			return nil, ErrSignSessionInvalidStatus
		default:
			return nil, err
		}
	}
	session.Status = "signed"
	if err := s.repo.Update(ctx, session); err != nil {
		return nil, err
	}

	s.logSessionState("signed", session, "signed_success")
	return session, nil
}

func (s *SignSessionService) buildSignURL(token string) (string, error) {
	base := strings.TrimSpace(s.signBaseURL)
	if base == "" {
		return "", ErrSignSessionBaseURL
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

func (s *SignSessionService) validateByIDToken(ctx context.Context, sessionID int64, token string) (*models.SignSession, error) {
	token = normalizeSessionToken(token)
	if token == "" {
		return nil, ErrSignSessionInvalidToken
	}
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrSignSessionNotFound
	}
	if session.Status == "signed" {
		return session, ErrSignSessionAlreadySigned
	}
	if s.isExpired(session) {
		s.expireSession(ctx, session, "ttl_elapsed_page_validation")
		return nil, ErrSignSessionExpired
	}
	if session.Attempts >= signSessionMaxAttempts {
		s.expireSession(ctx, session, "too_many_attempts_page_validation")
		return nil, ErrSignSessionTooManyTries
	}
	hash := hashToken(token)
	if subtle.ConstantTimeCompare([]byte(session.TokenHash), []byte(hash)) != 1 {
		attempts, err := s.repo.IncrementAttempts(ctx, session.ID)
		if err != nil {
			return nil, err
		}
		if attempts >= signSessionMaxAttempts {
			s.expireSession(ctx, session, "too_many_attempts_invalid_token")
			return nil, ErrSignSessionTooManyTries
		}
		return nil, ErrSignSessionInvalidToken
	}
	return session, nil
}

func (s *SignSessionService) expireSession(ctx context.Context, session *models.SignSession, reason string) {
	if session == nil {
		return
	}
	session.Status = "expired"
	_ = s.repo.Update(ctx, session)
	s.logSessionState("expired", session, reason)
}

func (s *SignSessionService) logSessionState(transition string, session *models.SignSession, reason string) {
	if session == nil {
		return
	}
	nowUTC := s.now().UTC()
	nowLocal := nowUTC.In(s.serverTZ)
	createdAt := session.CreatedAt.UTC().Format(time.RFC3339Nano)
	expiresAt := session.ExpiresAt.UTC().Format(time.RFC3339Nano)
	log.Printf("[sign][session][%s] document_id=%d session_id=%d server_tz=%s now_utc=%s now_local=%s created_at=%s expires_at=%s ttl_minutes=%d status=%s reason=%s",
		transition,
		session.DocumentID,
		session.ID,
		s.serverTZ.String(),
		nowUTC.Format(time.RFC3339Nano),
		nowLocal.Format(time.RFC3339Nano),
		createdAt,
		expiresAt,
		int(s.sessionTTL/time.Minute),
		session.Status,
		strings.TrimSpace(reason),
	)
}

func (s *SignSessionService) getByToken(ctx context.Context, token string) (*models.SignSession, error) {
	token = normalizeSessionToken(token)
	if token == "" {
		return nil, ErrSignSessionNotFound
	}
	hash := hashToken(token)
	session, err := s.repo.GetByTokenHash(ctx, hash)
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

func normalizeSessionToken(raw string) string {
	token := strings.TrimSpace(raw)
	if token == "" {
		return ""
	}
	if decoded, err := url.QueryUnescape(token); err == nil {
		token = strings.TrimSpace(decoded)
	}
	return token
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
