package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

const (
	signConfirmTTL         = 15 * time.Minute
	signConfirmMaxAttempts = 5
)

const (
	SignConfirmPolicyAny  = "ANY"
	SignConfirmPolicyBoth = "BOTH"
)

var (
	ErrSignConfirmNotFound     = errors.New("sign confirmation not found")
	ErrSignConfirmExpired      = errors.New("sign confirmation expired")
	ErrSignConfirmInvalidCode  = errors.New("invalid sign confirmation code")
	ErrSignConfirmTooManyTries = errors.New("too many sign confirmation attempts")
	ErrSignConfirmInvalidToken = errors.New("invalid sign confirmation token")
)

type SigningEmailSender interface {
	SendSigningConfirm(email, code, magicLink string) error
}

type SigningTelegramSender interface {
	SendSigningConfirm(chatID int64, docInfo, approveToken, rejectToken string) error
}

type DocumentSigner interface {
	FinalizeSigning(docID int64) error
}

type SignatureConfirmationStore interface {
	CreatePending(ctx context.Context, documentID, userID int64, channel string, otpHash *string, tokenHash *string, expiresAt time.Time, meta []byte) (*models.SignatureConfirmation, error)
	FindPending(ctx context.Context, documentID, userID int64, channel string) (*models.SignatureConfirmation, error)
	FindPendingByTokenHash(ctx context.Context, channel, tokenHash string) (*models.SignatureConfirmation, error)
	Approve(ctx context.Context, id string, metaUpdate []byte) (*models.SignatureConfirmation, error)
	Reject(ctx context.Context, id string, metaUpdate []byte) (*models.SignatureConfirmation, error)
	CancelPrevious(ctx context.Context, documentID, userID int64) (int64, error)
	IncrementAttempts(ctx context.Context, id string) (int, error)
	Expire(ctx context.Context, id string) error
	HasApproved(ctx context.Context, documentID, userID int64, channel string) (bool, error)
	GetLatestByChannel(ctx context.Context, documentID, userID int64, channel string) (*models.SignatureConfirmation, error)
}

type DocumentLookup interface {
	GetByID(id int64) (*models.Document, error)
}

type DocumentSigningConfirmationConfig struct {
	ConfirmPolicy string
	BaseURL       string
}

type SigningChannelStatus struct {
	Channel   string    `json:"channel"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SigningStartResult struct {
	DocumentID int64                  `json:"document_id"`
	UserID     int64                  `json:"user_id"`
	Policy     string                 `json:"policy"`
	Channels   []SigningChannelStatus `json:"channels"`
}

type DocumentSigningConfirmationService struct {
	repo      SignatureConfirmationStore
	userRepo  repositories.UserRepository
	docRepo   DocumentLookup
	docSigner DocumentSigner
	email     SigningEmailSender
	telegram  SigningTelegramSender
	policy    string
	baseURL   string
	now       func() time.Time
	debug     *signConfirmDebugStore
}

func NewDocumentSigningConfirmationService(
	repo SignatureConfirmationStore,
	userRepo repositories.UserRepository,
	docRepo DocumentLookup,
	docSigner DocumentSigner,
	email SigningEmailSender,
	telegram SigningTelegramSender,
	cfg DocumentSigningConfirmationConfig,
	now func() time.Time,
) *DocumentSigningConfirmationService {
	if now == nil {
		now = time.Now
	}
	policy := strings.ToUpper(strings.TrimSpace(cfg.ConfirmPolicy))
	if policy == "" {
		policy = SignConfirmPolicyAny
	}
	return &DocumentSigningConfirmationService{
		repo:      repo,
		userRepo:  userRepo,
		docRepo:   docRepo,
		docSigner: docSigner,
		email:     email,
		telegram:  telegram,
		policy:    policy,
		baseURL:   strings.TrimSpace(cfg.BaseURL),
		now:       now,
	}
}

type DebugSigningInfo struct {
	DocumentID    int64     `json:"document_id"`
	UserID        int64     `json:"user_id"`
	EmailCode     string    `json:"email_code"`
	EmailToken    string    `json:"email_token"`
	TelegramToken string    `json:"telegram_token"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type signConfirmDebugStore struct {
	key     string
	entries map[string]*DebugSigningInfo
	mu      sync.RWMutex
}

func (s *DocumentSigningConfirmationService) EnableDebug(key string) {
	if s == nil {
		return
	}
	if s.debug == nil {
		s.debug = &signConfirmDebugStore{
			key:     strings.TrimSpace(key),
			entries: make(map[string]*DebugSigningInfo),
		}
		return
	}
	s.debug.mu.Lock()
	defer s.debug.mu.Unlock()
	s.debug.key = strings.TrimSpace(key)
}

func (s *DocumentSigningConfirmationService) DebugEnabled() bool {
	return s != nil && s.debug != nil
}

func (s *DocumentSigningConfirmationService) DebugKey() string {
	if s == nil || s.debug == nil {
		return ""
	}
	s.debug.mu.RLock()
	defer s.debug.mu.RUnlock()
	return s.debug.key
}

func (s *DocumentSigningConfirmationService) DebugLatest(documentID, userID int64) (*DebugSigningInfo, bool) {
	if s == nil || s.debug == nil {
		return nil, false
	}
	key := debugKey(documentID, userID)
	s.debug.mu.RLock()
	defer s.debug.mu.RUnlock()
	entry := s.debug.entries[key]
	if entry == nil {
		return nil, false
	}
	out := *entry
	return &out, true
}

func (s *DocumentSigningConfirmationService) storeDebug(documentID, userID int64, emailCode, emailToken, telegramToken string, expiresAt time.Time) {
	if s == nil || s.debug == nil {
		return
	}
	key := debugKey(documentID, userID)
	s.debug.mu.Lock()
	defer s.debug.mu.Unlock()
	s.debug.entries[key] = &DebugSigningInfo{
		DocumentID:    documentID,
		UserID:        userID,
		EmailCode:     emailCode,
		EmailToken:    emailToken,
		TelegramToken: telegramToken,
		ExpiresAt:     expiresAt,
	}
}

func debugKey(documentID, userID int64) string {
	return fmt.Sprintf("%d:%d", documentID, userID)
}

func (s *DocumentSigningConfirmationService) StartSigning(ctx context.Context, documentID, userID int64) (*SigningStartResult, error) {
	if s.repo == nil {
		return nil, errors.New("signature confirmation repo is nil")
	}
	if s.userRepo == nil {
		return nil, errors.New("user repo is nil")
	}
	if s.docRepo == nil {
		return nil, errors.New("document repo is nil")
	}
	if s.email == nil {
		return nil, errors.New("email sender is nil")
	}
	if s.telegram == nil {
		return nil, errors.New("telegram sender is nil")
	}

	user, err := s.userRepo.GetByID(int(userID))
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user == nil || strings.TrimSpace(user.Email) == "" {
		return nil, errors.New("user email is required")
	}
	if user.TelegramChatID == 0 {
		return nil, errors.New("user telegram chat id is required")
	}

	doc, err := s.docRepo.GetByID(documentID)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return nil, errors.New("document not found")
	}

	if _, err := s.repo.CancelPrevious(ctx, documentID, userID); err != nil {
		return nil, err
	}

	now := s.now()
	expiresAt := now.Add(signConfirmTTL)

	code := GenerateVerificationCode()
	codeHash, err := HashVerificationCode(code)
	if err != nil {
		return nil, err
	}
	emailToken, emailTokenHash, err := generateConfirmToken()
	if err != nil {
		return nil, err
	}
	_, err = s.repo.CreatePending(
		ctx,
		documentID,
		userID,
		"email",
		&codeHash,
		&emailTokenHash,
		expiresAt,
		nil,
	)
	if err != nil {
		return nil, err
	}

	telegramToken, telegramTokenHash, err := generateConfirmToken()
	if err != nil {
		return nil, err
	}
	_, err = s.repo.CreatePending(
		ctx,
		documentID,
		userID,
		"telegram",
		nil,
		&telegramTokenHash,
		expiresAt,
		nil,
	)
	if err != nil {
		return nil, err
	}

	magicLink, err := s.buildConfirmURL(emailToken, "")
	if err != nil {
		return nil, err
	}
	if err := s.email.SendSigningConfirm(user.Email, code, magicLink); err != nil {
		return nil, fmt.Errorf("send signing email: %w", err)
	}

	docInfo := fmt.Sprintf("Документ #%d (%s)", doc.ID, doc.DocType)
	if err := s.telegram.SendSigningConfirm(user.TelegramChatID, docInfo, telegramToken, telegramToken); err != nil {
		return nil, fmt.Errorf("send telegram signing confirm: %w", err)
	}
	s.storeDebug(documentID, userID, code, emailToken, telegramToken, expiresAt)

	return &SigningStartResult{
		DocumentID: documentID,
		UserID:     userID,
		Policy:     s.policy,
		Channels: []SigningChannelStatus{
			{Channel: "email", Status: "pending", ExpiresAt: expiresAt},
			{Channel: "telegram", Status: "pending", ExpiresAt: expiresAt},
		},
	}, nil
}

func (s *DocumentSigningConfirmationService) ConfirmByEmailCode(
	ctx context.Context,
	documentID, userID int64,
	code, ip, userAgent string,
) (*models.SignatureConfirmation, error) {
	if s.repo == nil {
		return nil, errors.New("signature confirmation repo is nil")
	}
	pending, err := s.repo.FindPending(ctx, documentID, userID, "email")
	if err != nil {
		return nil, err
	}
	if pending == nil {
		return nil, ErrSignConfirmNotFound
	}
	if s.now().After(pending.ExpiresAt) {
		_ = s.repo.Expire(ctx, pending.ID)
		return nil, ErrSignConfirmExpired
	}
	if pending.Attempts >= signConfirmMaxAttempts {
		_ = s.repo.Expire(ctx, pending.ID)
		return nil, ErrSignConfirmTooManyTries
	}
	if pending.OTPHash == nil {
		return nil, ErrSignConfirmInvalidCode
	}
	if err := CompareVerificationCode(*pending.OTPHash, code); err != nil {
		attempts, incErr := s.repo.IncrementAttempts(ctx, pending.ID)
		if incErr != nil {
			return nil, incErr
		}
		if attempts >= signConfirmMaxAttempts {
			_ = s.repo.Expire(ctx, pending.ID)
			return nil, ErrSignConfirmTooManyTries
		}
		return nil, ErrSignConfirmInvalidCode
	}

	metaUpdate := map[string]any{
		"ip":         ip,
		"user_agent": userAgent,
		"method":     "email_code",
	}
	metaBytes, _ := json.Marshal(metaUpdate)
	confirmation, err := s.repo.Approve(ctx, pending.ID, metaBytes)
	if err != nil {
		return nil, err
	}
	if err := s.evaluatePolicy(ctx, documentID, userID); err != nil {
		return nil, err
	}
	return confirmation, nil
}

func (s *DocumentSigningConfirmationService) ConfirmByEmailToken(ctx context.Context, token string) (*models.SignatureConfirmation, error) {
	if s.repo == nil {
		return nil, errors.New("signature confirmation repo is nil")
	}
	tokenHash := hashConfirmToken(token)
	pending, err := s.repo.FindPendingByTokenHash(ctx, "email", tokenHash)
	if err != nil {
		return nil, err
	}
	if pending == nil {
		return nil, ErrSignConfirmNotFound
	}
	if pending.TokenHash == nil || subtle.ConstantTimeCompare([]byte(*pending.TokenHash), []byte(tokenHash)) != 1 {
		return nil, ErrSignConfirmInvalidToken
	}
	metaUpdate := map[string]any{
		"method": "email_token",
	}
	metaBytes, _ := json.Marshal(metaUpdate)
	confirmation, err := s.repo.Approve(ctx, pending.ID, metaBytes)
	if err != nil {
		return nil, err
	}
	if err := s.evaluatePolicy(ctx, pending.DocumentID, pending.UserID); err != nil {
		return nil, err
	}
	return confirmation, nil
}

func (s *DocumentSigningConfirmationService) ConfirmByTelegramCallback(
	ctx context.Context,
	token string,
	action string,
	meta json.RawMessage,
) (*models.SignatureConfirmation, error) {
	if s.repo == nil {
		return nil, errors.New("signature confirmation repo is nil")
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "approve" && action != "reject" {
		return nil, errors.New("invalid action")
	}

	tokenHash := hashConfirmToken(token)
	pending, err := s.repo.FindPendingByTokenHash(ctx, "telegram", tokenHash)
	if err != nil {
		return nil, err
	}
	if pending == nil {
		return nil, ErrSignConfirmNotFound
	}
	if pending.TokenHash == nil || subtle.ConstantTimeCompare([]byte(*pending.TokenHash), []byte(tokenHash)) != 1 {
		return nil, ErrSignConfirmInvalidToken
	}

	metaBytes := meta
	if len(metaBytes) == 0 {
		metaBytes, _ = json.Marshal(map[string]any{"method": "telegram"})
	}

	var confirmation *models.SignatureConfirmation
	switch action {
	case "approve":
		confirmation, err = s.repo.Approve(ctx, pending.ID, metaBytes)
	case "reject":
		confirmation, err = s.repo.Reject(ctx, pending.ID, metaBytes)
	}
	if err != nil {
		return nil, err
	}
	if action == "approve" {
		if err := s.evaluatePolicy(ctx, pending.DocumentID, pending.UserID); err != nil {
			return nil, err
		}
	}
	return confirmation, nil
}

func (s *DocumentSigningConfirmationService) GetStatus(ctx context.Context, documentID, userID int64) ([]SigningChannelStatus, error) {
	if s.repo == nil {
		return nil, errors.New("signature confirmation repo is nil")
	}
	now := s.now()
	channels := []string{"email", "telegram"}
	result := make([]SigningChannelStatus, 0, len(channels))
	for _, channel := range channels {
		latest, err := s.repo.GetLatestByChannel(ctx, documentID, userID, channel)
		if err != nil {
			return nil, err
		}
		status := "pending"
		expiresAt := now
		if latest != nil {
			expiresAt = latest.ExpiresAt
			status = latest.Status
			if status == "pending" && now.After(latest.ExpiresAt) {
				_ = s.repo.Expire(ctx, latest.ID)
				status = "expired"
			}
		}
		switch status {
		case "approved":
			status = "approved"
		case "pending":
			status = "pending"
		default:
			status = "expired"
		}
		result = append(result, SigningChannelStatus{
			Channel:   channel,
			Status:    status,
			ExpiresAt: expiresAt,
		})
	}
	return result, nil
}

func (s *DocumentSigningConfirmationService) evaluatePolicy(ctx context.Context, documentID, userID int64) error {
	if s.docSigner == nil {
		return errors.New("document signer is nil")
	}
	switch s.policy {
	case SignConfirmPolicyAny:
		return s.docSigner.FinalizeSigning(documentID)
	case SignConfirmPolicyBoth:
		telegramApproved, err := s.repo.HasApproved(ctx, documentID, userID, "telegram")
		if err != nil {
			return err
		}
		emailApproved, err := s.repo.HasApproved(ctx, documentID, userID, "email")
		if err != nil {
			return err
		}
		if telegramApproved && emailApproved {
			return s.docSigner.FinalizeSigning(documentID)
		}
		return nil
	default:
		return fmt.Errorf("invalid sign confirm policy: %s", s.policy)
	}
}

func (s *DocumentSigningConfirmationService) buildConfirmURL(token, action string) (string, error) {
	base := strings.TrimRight(s.baseURL, "/")
	if base == "" {
		return "", errors.New("sign confirmation base URL is required")
	}
	target, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid sign confirm base URL: %w", err)
	}
	target.Path = strings.TrimRight(target.Path, "/") + "/sign/confirm"
	values := target.Query()
	values.Set("token", token)
	if action != "" {
		values.Set("action", action)
	}
	target.RawQuery = values.Encode()
	return target.String(), nil
}

func generateConfirmToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("token rand: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashConfirmToken(token)
	return token, hash, nil
}

func hashConfirmToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
