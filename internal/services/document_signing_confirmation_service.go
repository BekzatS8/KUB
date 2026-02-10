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
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

const (
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
	ErrSignConfirmAlreadyUsed  = errors.New("sign confirmation already used")
	emailOTPPattern            = regexp.MustCompile(`^\d{6}$`)
)

type SigningEmailSender interface {
	SendSigningConfirm(email string, data SigningEmailData) error
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
	FindByTokenHash(ctx context.Context, channel, tokenHash string) (*models.SignatureConfirmation, error)
	Approve(ctx context.Context, id string, metaUpdate []byte) (*models.SignatureConfirmation, error)
	Reject(ctx context.Context, id string, metaUpdate []byte) (*models.SignatureConfirmation, error)
	CancelPrevious(ctx context.Context, documentID, userID int64, channel string) (int64, error)
	IncrementAttempts(ctx context.Context, id string) (int, error)
	Expire(ctx context.Context, id string) error
	HasApproved(ctx context.Context, documentID, userID int64, channel string) (bool, error)
	GetLatestByChannel(ctx context.Context, documentID, userID int64, channel string) (*models.SignatureConfirmation, error)
	UpdateMeta(ctx context.Context, id string, metaUpdate []byte) (*models.SignatureConfirmation, error)
}

type DocumentLookup interface {
	GetByID(id int64) (*models.Document, error)
}

type DocumentSigningConfirmationConfig struct {
	ConfirmPolicy      string
	EmailVerifyBaseURL string
	EmailTokenPepper   string
	EmailTTL           time.Duration
	FilesRoot          string
}

type SigningChannelStatus struct {
	Channel    string     `json:"channel"`
	Status     string     `json:"status"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
}

type SigningStartResult struct {
	DocumentID int64                  `json:"document_id"`
	UserID     int64                  `json:"user_id"`
	Policy     string                 `json:"policy"`
	Channels   []SigningChannelStatus `json:"channels"`
}

type DocumentSigningConfirmationService struct {
	repo          SignatureConfirmationStore
	userRepo      repositories.UserRepository
	docRepo       DocumentLookup
	docSigner     DocumentSigner
	email         SigningEmailSender
	telegram      SigningTelegramSender
	policy        string
	verifyBaseURL string
	tokenPepper   string
	emailTTL      time.Duration
	filesRoot     string
	now           func() time.Time
	debug         *signConfirmDebugStore
}

func withSignConfirmTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, 5*time.Second)
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
	emailTTL := cfg.EmailTTL
	if emailTTL == 0 {
		emailTTL = 30 * time.Minute
	}
	return &DocumentSigningConfirmationService{
		repo:          repo,
		userRepo:      userRepo,
		docRepo:       docRepo,
		docSigner:     docSigner,
		email:         email,
		telegram:      telegram,
		policy:        policy,
		verifyBaseURL: strings.TrimSpace(cfg.EmailVerifyBaseURL),
		tokenPepper:   strings.TrimSpace(cfg.EmailTokenPepper),
		emailTTL:      emailTTL,
		filesRoot:     strings.TrimSpace(cfg.FilesRoot),
		now:           now,
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

func (s *DocumentSigningConfirmationService) TokenPepperForLog() string {
	if s == nil {
		return ""
	}
	return s.tokenPepper
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

func (s *DocumentSigningConfirmationService) StartSigning(ctx context.Context, documentID, userID int64, signerEmail string) (*SigningStartResult, error) {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
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

	user, err := s.userRepo.GetByID(int(userID))
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	doc, err := s.docRepo.GetByID(documentID)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return nil, errors.New("document not found")
	}

	if _, err := s.repo.CancelPrevious(ctx, documentID, userID, "email"); err != nil {
		return nil, err
	}

	now := s.now()
	expiresAt := now.Add(s.emailTTL)

	emailToken, emailTokenHash, err := generateConfirmToken(s.tokenPepper)
	if err != nil {
		return nil, err
	}
	otp := GenerateVerificationCode()
	otpHash, err := HashVerificationCode(otp)
	if err != nil {
		return nil, err
	}
	signerEmail = strings.TrimSpace(signerEmail)
	if signerEmail == "" {
		return nil, errors.New("signer email is required")
	}
	meta := map[string]any{
		"sent_at":      now.UTC().Format(time.RFC3339Nano),
		"signer_email": signerEmail,
	}
	metaBytes, _ := json.Marshal(meta)
	_, err = s.repo.CreatePending(
		ctx,
		documentID,
		userID,
		"email",
		&otpHash,
		&emailTokenHash,
		expiresAt,
		metaBytes,
	)
	if err != nil {
		return nil, err
	}

	magicLink, err := s.buildEmailVerifyURL(emailToken)
	if err != nil {
		return nil, err
	}
	signingEmail := SigningEmailData{
		DocumentID:   doc.ID,
		DocumentType: doc.DocType,
		Sender:       user.CompanyName,
		MagicLink:    magicLink,
		ExpiresAt:    expiresAt,
		Code:         otp,
	}
	if signingEmail.Sender == "" {
		signingEmail.Sender = user.Email
	}
	if err := s.email.SendSigningConfirm(signerEmail, signingEmail); err != nil {
		return nil, fmt.Errorf("send signing email: %w", err)
	}
	s.storeDebug(documentID, userID, otp, emailToken, "", expiresAt)

	channels := []SigningChannelStatus{
		{Channel: "email", Status: "pending", ExpiresAt: expiresAt},
	}
	return &SigningStartResult{
		DocumentID: documentID,
		UserID:     userID,
		Policy:     s.policy,
		Channels:   channels,
	}, nil
}

func (s *DocumentSigningConfirmationService) ConfirmByEmailToken(
	ctx context.Context,
	documentID int64,
	token, code, ip, userAgent string,
) (string, string, string, *models.SignatureConfirmation, error) {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	if s.repo == nil {
		return "", "", "", nil, errors.New("signature confirmation repo is nil")
	}
	token = normalizeEmailConfirmToken(token)
	if token == "" {
		return "", "", "", nil, ErrSignConfirmNotFound
	}
	tokenHash := hashConfirmTokenWithPepper(token, s.tokenPepper)
	pending, err := s.repo.FindByTokenHash(ctx, "email", tokenHash)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("find confirmation by token hash: %w", err)
	}
	if pending == nil {
		return "", "", "", nil, ErrSignConfirmNotFound
	}
	if pending.TokenHash == nil || subtle.ConstantTimeCompare([]byte(*pending.TokenHash), []byte(tokenHash)) != 1 {
		return "", "", "", nil, ErrSignConfirmInvalidToken
	}
	if pending.DocumentID != documentID {
		return "", "", "", nil, ErrSignConfirmInvalidToken
	}
	code = normalizeEmailOTP(code)
	if code == "" {
		return "", "", "", nil, ErrSignConfirmInvalidCode
	}
	if pending.Status != "pending" {
		if pending.Status == "approved" {
			return "", "", "", pending, ErrSignConfirmAlreadyUsed
		}
		return "", "", "", pending, ErrSignConfirmExpired
	}
	if s.now().After(pending.ExpiresAt) {
		_ = s.repo.Expire(ctx, pending.ID)
		return "", "", "", pending, ErrSignConfirmExpired
	}
	if pending.Attempts >= signConfirmMaxAttempts {
		_ = s.repo.Expire(ctx, pending.ID)
		return "", "", "", pending, ErrSignConfirmTooManyTries
	}
	if pending.OTPHash == nil || *pending.OTPHash == "" {
		return "", "", "", nil, ErrSignConfirmInvalidCode
	}
	if err := CompareVerificationCode(*pending.OTPHash, code); err != nil {
		attempts, incErr := s.repo.IncrementAttempts(ctx, pending.ID)
		if incErr != nil {
			return "", "", "", pending, fmt.Errorf("increment confirmation attempts: %w", incErr)
		}
		if attempts >= signConfirmMaxAttempts {
			_ = s.repo.Expire(ctx, pending.ID)
			return "", "", "", pending, ErrSignConfirmTooManyTries
		}
		return "", "", "", nil, ErrSignConfirmInvalidCode
	}

	documentHash, err := s.hashDocumentContent(pending.DocumentID)
	if err != nil {
		return "", "", "", pending, fmt.Errorf("hash document content: %w", err)
	}
	signerEmail := extractSignerEmail(pending.Meta)
	metaUpdate := map[string]any{
		"ip":            ip,
		"user_agent":    userAgent,
		"method":        "email_magic_link",
		"document_hash": documentHash,
	}
	if signerEmail != "" {
		metaUpdate["signer_email"] = signerEmail
	}
	metaBytes, _ := json.Marshal(metaUpdate)
	approved, err := s.repo.Approve(ctx, pending.ID, metaBytes)
	if err != nil {
		return "", "", "", pending, fmt.Errorf("approve confirmation: %w", err)
	}
	return "approved", signerEmail, documentHash, approved, nil
}

type EmailTokenVerification struct {
	Document struct {
		ID     int64  `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
	} `json:"document"`
	Confirmation struct {
		ExpiresAt time.Time `json:"expires_at"`
	} `json:"confirmation"`
	RequirePostConfirm bool `json:"require_post_confirm"`
}

func (s *DocumentSigningConfirmationService) ValidateEmailToken(
	ctx context.Context,
	token, ip, userAgent string,
) (*EmailTokenVerification, error) {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	if s.repo == nil {
		return nil, errors.New("signature confirmation repo is nil")
	}
	token = normalizeEmailConfirmToken(token)
	if token == "" {
		return nil, ErrSignConfirmNotFound
	}
	tokenHash := hashConfirmTokenWithPepper(token, s.tokenPepper)
	pending, err := s.repo.FindByTokenHash(ctx, "email", tokenHash)
	if err != nil {
		return nil, fmt.Errorf("find confirmation by token hash: %w", err)
	}
	if pending == nil {
		return nil, ErrSignConfirmNotFound
	}
	if pending.TokenHash == nil || subtle.ConstantTimeCompare([]byte(*pending.TokenHash), []byte(tokenHash)) != 1 {
		return nil, ErrSignConfirmInvalidToken
	}
	if pending.Status != "pending" {
		if pending.Status == "approved" {
			return nil, ErrSignConfirmAlreadyUsed
		}
		return nil, ErrSignConfirmExpired
	}
	if s.now().After(pending.ExpiresAt) {
		_ = s.repo.Expire(ctx, pending.ID)
		return nil, ErrSignConfirmExpired
	}
	if pending.Attempts >= signConfirmMaxAttempts {
		_ = s.repo.Expire(ctx, pending.ID)
		return nil, ErrSignConfirmTooManyTries
	}

	metaUpdate := buildOpenMetaUpdate(pending.Meta, ip, userAgent, s.now())
	if len(metaUpdate) > 0 {
		metaBytes, _ := json.Marshal(metaUpdate)
		if _, err := s.repo.UpdateMeta(ctx, pending.ID, metaBytes); err != nil {
			return nil, fmt.Errorf("update confirmation meta: %w", err)
		}
	}

	doc, err := s.docRepo.GetByID(pending.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("get document by id: %w", err)
	}
	if doc == nil {
		return nil, ErrSignConfirmNotFound
	}
	response := &EmailTokenVerification{
		RequirePostConfirm: true,
	}
	response.Document.ID = doc.ID
	response.Document.Title = doc.DocType
	response.Document.Status = doc.Status
	response.Confirmation.ExpiresAt = pending.ExpiresAt
	return response, nil
}

func (s *DocumentSigningConfirmationService) LookupEmailConfirmationByToken(
	ctx context.Context,
	token string,
) (*models.SignatureConfirmation, error) {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	if s.repo == nil {
		return nil, errors.New("signature confirmation repo is nil")
	}
	token = normalizeEmailConfirmToken(token)
	if token == "" {
		return nil, nil
	}
	tokenHash := hashConfirmTokenWithPepper(token, s.tokenPepper)
	confirmation, err := s.repo.FindByTokenHash(ctx, "email", tokenHash)
	if err != nil {
		return nil, fmt.Errorf("lookup confirmation by token hash: %w", err)
	}
	return confirmation, nil
}

func (s *DocumentSigningConfirmationService) ConfirmByTelegramCallback(
	ctx context.Context,
	token string,
	action string,
	meta json.RawMessage,
) (*models.SignatureConfirmation, error) {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	if s.repo == nil {
		return nil, errors.New("signature confirmation repo is nil")
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "approve" && action != "reject" {
		return nil, errors.New("invalid action")
	}

	token = normalizeEmailConfirmToken(token)
	tokenHash := hashConfirmTokenWithPepper(token, s.tokenPepper)
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
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	if s.repo == nil {
		return nil, errors.New("signature confirmation repo is nil")
	}
	now := s.now()
	channels := []string{"email"}
	result := make([]SigningChannelStatus, 0, len(channels))
	for _, channel := range channels {
		latest, err := s.repo.GetLatestByChannel(ctx, documentID, userID, channel)
		if err != nil {
			return nil, err
		}
		status := "pending"
		expiresAt := now
		var approvedAt *time.Time
		if latest != nil {
			expiresAt = latest.ExpiresAt
			status = latest.Status
			approvedAt = latest.ApprovedAt
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
			Channel:    channel,
			Status:     status,
			ExpiresAt:  expiresAt,
			ApprovedAt: approvedAt,
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
		return s.docSigner.FinalizeSigning(documentID)
	default:
		return fmt.Errorf("invalid sign confirm policy: %s", s.policy)
	}
}

func (s *DocumentSigningConfirmationService) buildEmailVerifyURL(token string) (string, error) {
	base := strings.TrimRight(s.verifyBaseURL, "/")
	if base == "" {
		return "", errors.New("sign email verify base URL is required")
	}
	target, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid sign email verify base URL: %w", err)
	}
	target.Path = strings.TrimRight(target.Path, "/") + "/sign/email/verify"
	values := target.Query()
	values.Set("token", token)
	target.RawQuery = values.Encode()
	return target.String(), nil
}

func generateConfirmToken(pepper string) (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("token rand: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashConfirmTokenWithPepper(token, pepper)
	return token, hash, nil
}

func hashConfirmToken(token string) string {
	return hashConfirmTokenWithPepper(token, "")
}

func hashConfirmTokenWithPepper(token, pepper string) string {
	token = strings.TrimSpace(token)
	if pepper = strings.TrimSpace(pepper); pepper != "" {
		token = token + ":" + pepper
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func NormalizeEmailConfirmTokenForLog(raw string) string {
	return normalizeEmailConfirmToken(raw)
}

func HashEmailConfirmTokenForLog(token, pepper string) string {
	return hashConfirmTokenWithPepper(token, pepper)
}

func normalizeEmailConfirmToken(raw string) string {
	token := strings.TrimSpace(raw)
	if token == "" {
		return ""
	}
	if decoded, err := url.QueryUnescape(token); err == nil {
		token = strings.TrimSpace(decoded)
	}
	return token
}

func normalizeEmailOTP(code string) string {
	code = strings.TrimSpace(code)
	if !emailOTPPattern.MatchString(code) {
		return ""
	}
	return code
}

func buildOpenMetaUpdate(meta json.RawMessage, ip, userAgent string, now time.Time) map[string]any {
	update := map[string]any{}
	if ip = strings.TrimSpace(ip); ip != "" {
		update["opened_ip"] = ip
	}
	if userAgent = strings.TrimSpace(userAgent); userAgent != "" {
		update["opened_user_agent"] = userAgent
	}
	existing := map[string]any{}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &existing)
	}
	if existing["opened_at"] == nil {
		update["opened_at"] = now.UTC().Format(time.RFC3339Nano)
	}
	if len(update) == 0 {
		return nil
	}
	return update
}

func extractSignerEmail(meta json.RawMessage) string {
	if len(meta) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(meta, &payload); err != nil {
		return ""
	}
	if val, ok := payload["signer_email"].(string); ok {
		return strings.TrimSpace(val)
	}
	return ""
}

func (s *DocumentSigningConfirmationService) hashDocumentContent(documentID int64) (string, error) {
	if s.docRepo == nil {
		return "", errors.New("document repo is nil")
	}
	doc, err := s.docRepo.GetByID(documentID)
	if err != nil || doc == nil {
		return "", errors.New("document not found")
	}
	rel := strings.TrimSpace(doc.FilePathPdf)
	if rel == "" {
		rel = strings.TrimSpace(doc.FilePath)
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	if strings.HasPrefix(rel, "files/") {
		rel = strings.TrimPrefix(rel, "files/")
	}
	if strings.Contains(rel, "..") || rel == "" || rel == "." {
		return "", errors.New("bad filepath")
	}
	root := strings.TrimSpace(s.filesRoot)
	if root == "" {
		return "", errors.New("files root is required")
	}
	path := filepath.Join(root, rel)
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open document: %w", err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hash document: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
