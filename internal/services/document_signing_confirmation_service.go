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
	"log"
	"mime"
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
	ErrSignConfirmNotFound                 = errors.New("sign confirmation not found")
	ErrSignConfirmExpired                  = errors.New("sign confirmation expired")
	ErrSignConfirmInvalidCode              = errors.New("invalid sign confirmation code")
	ErrSignConfirmTooManyTries             = errors.New("too many sign confirmation attempts")
	ErrSignConfirmInvalidToken             = errors.New("invalid sign confirmation token")
	ErrSignConfirmAlreadyUsed              = errors.New("sign confirmation already used")
	ErrSignConfirmHashRequired             = errors.New("sign confirmation document hash required")
	ErrSignConfirmDocMismatch              = errors.New("sign confirmation document hash mismatch")
	ErrSignConfirmAgreementVersionRequired = errors.New("sign confirmation agreement version required")
	ErrSignConfirmAgreementVersionMismatch = errors.New("sign confirmation agreement version mismatch")
	emailOTPPattern                        = regexp.MustCompile(`^\d{6}$`)
)

type EmailSigningAgreement struct {
	Required               bool   `json:"required"`
	Version                string `json:"version"`
	Title                  string `json:"title"`
	CheckboxLabel          string `json:"checkbox_label"`
	ConfirmButtonLabel     string `json:"confirm_button_label"`
	VersionMismatchMessage string `json:"version_mismatch_message"`
}

func CurrentEmailSigningAgreement() EmailSigningAgreement {
	return EmailSigningAgreement{
		Required:               true,
		Version:                "v1",
		Title:                  "Подтверждение ознакомления",
		CheckboxLabel:          "Я ознакомился с документом, проверил данные и согласен с его условиями.",
		ConfirmButtonLabel:     "Перейти к подписанию",
		VersionMismatchMessage: "Текст согласия изменился. Пожалуйста, откройте документ заново перед подписанием.",
	}
}

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
	SMSVerifyBaseURL   string
	EmailTokenPepper   string
	EmailTTL           time.Duration
	SMSTTL             time.Duration
	FilesRoot          string
	ServerTZ           *time.Location
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
	sms           SMSSender
	telegram      SigningTelegramSender
	policy        string
	verifyBaseURL string
	smsVerifyBase string
	tokenPepper   string
	emailTTL      time.Duration
	smsTTL        time.Duration
	filesRoot     string
	serverTZ      *time.Location
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
	smsTTL := cfg.SMSTTL
	if smsTTL == 0 {
		smsTTL = emailTTL
	}
	serverTZ := cfg.ServerTZ
	if serverTZ == nil {
		serverTZ = time.UTC
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
		smsVerifyBase: strings.TrimSpace(cfg.SMSVerifyBaseURL),
		tokenPepper:   strings.TrimSpace(cfg.EmailTokenPepper),
		emailTTL:      emailTTL,
		smsTTL:        smsTTL,
		filesRoot:     strings.TrimSpace(cfg.FilesRoot),
		serverTZ:      serverTZ,
		now:           now,
	}
}

func (s *DocumentSigningConfirmationService) SetSMSSender(sender SMSSender) {
	if s == nil {
		return
	}
	s.sms = sender
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
	s.logConfirmState("created", documentID, "", userID, expiresAt, int(s.emailTTL/time.Minute), "pending", "start_signing")

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

func (s *DocumentSigningConfirmationService) StartSigningBySMS(ctx context.Context, documentID, userID int64, signerPhone, signerEmail string) (*SigningStartResult, error) {
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
	if s.sms == nil {
		return nil, errors.New("sms sender is nil")
	}
	signerPhone = strings.TrimSpace(signerPhone)
	if signerPhone == "" {
		return nil, errors.New("signer phone is required")
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
	if _, err := s.repo.CancelPrevious(ctx, documentID, userID, "sms"); err != nil {
		return nil, err
	}
	now := s.now()
	expiresAt := now.Add(s.smsTTL)
	smsToken, smsTokenHash, err := generateConfirmToken(s.tokenPepper)
	if err != nil {
		return nil, err
	}
	otp := GenerateVerificationCode()
	otpHash, err := HashVerificationCode(otp)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{
		"sent_at":       now.UTC().Format(time.RFC3339Nano),
		"signer_phone":  signerPhone,
		"provider":      "mobizon",
		"signer_email":  strings.TrimSpace(signerEmail),
		"delivery_type": "sms",
	}
	metaBytes, _ := json.Marshal(meta)
	confirmation, err := s.repo.CreatePending(ctx, documentID, userID, "sms", &otpHash, &smsTokenHash, expiresAt, metaBytes)
	if err != nil {
		return nil, err
	}
	verifyURL, err := s.buildSMSVerifyURL(smsToken)
	if err != nil {
		return nil, err
	}
	message := BuildSigningSMS(doc.DocType, otp, verifyURL, expiresAt)
	sendResult, err := s.sms.Send(ctx, SMSMessage{To: signerPhone, Text: message})
	if err != nil {
		return nil, fmt.Errorf("send signing sms: %w", err)
	}
	if confirmation != nil && sendResult != nil {
		_ = s.UpdateConfirmationMeta(ctx, confirmation.ID, map[string]any{
			"provider":            sendResult.Provider,
			"provider_message_id": strings.TrimSpace(sendResult.ProviderMessageID),
		})
	}
	channels := []SigningChannelStatus{{Channel: "sms", Status: "pending", ExpiresAt: expiresAt}}
	return &SigningStartResult{DocumentID: documentID, UserID: userID, Policy: s.policy, Channels: channels}, nil
}

func (s *DocumentSigningConfirmationService) StartSigningByChannel(ctx context.Context, channel string, documentID, userID int64, signerPhone, signerEmail string) (*SigningStartResult, error) {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "sms":
		return s.StartSigningBySMS(ctx, documentID, userID, signerPhone, signerEmail)
	case "email":
		return s.StartSigning(ctx, documentID, userID, signerEmail)
	default:
		return nil, errors.New("unsupported signing channel")
	}
}

func (s *DocumentSigningConfirmationService) ConfirmByEmailToken(
	ctx context.Context,
	documentID int64,
	token, code, documentHashFromClient, agreementTextVersion, ip, userAgent string,
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
		s.logConfirmState("expired", pending.DocumentID, pending.ID, pending.UserID, pending.ExpiresAt, int(s.emailTTL/time.Minute), "expired", "ttl_elapsed")
		return "", "", "", pending, ErrSignConfirmExpired
	}
	if pending.Attempts >= signConfirmMaxAttempts {
		_ = s.repo.Expire(ctx, pending.ID)
		s.logConfirmState("expired", pending.DocumentID, pending.ID, pending.UserID, pending.ExpiresAt, int(s.emailTTL/time.Minute), "expired", "too_many_attempts")
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
			s.logConfirmState("expired", pending.DocumentID, pending.ID, pending.UserID, pending.ExpiresAt, int(s.emailTTL/time.Minute), "expired", "too_many_attempts_after_invalid_code")
			return "", "", "", pending, ErrSignConfirmTooManyTries
		}
		return "", "", "", nil, ErrSignConfirmInvalidCode
	}

	documentHash, err := s.hashDocumentContent(pending.DocumentID)
	if err != nil {
		return "", "", "", pending, fmt.Errorf("hash document content: %w", err)
	}
	clientHash := normalizeDocumentHash(documentHashFromClient)
	currentHash := normalizeDocumentHash("sha256:" + documentHash)
	if clientHash == "" {
		return "", "", "", pending, ErrSignConfirmHashRequired
	}
	agreement := CurrentEmailSigningAgreement()
	clientAgreementVersion := strings.TrimSpace(strings.ToLower(agreementTextVersion))
	expectedAgreementVersion := strings.TrimSpace(strings.ToLower(agreement.Version))
	if clientAgreementVersion == "" {
		return "", "", "", pending, ErrSignConfirmAgreementVersionRequired
	}
	if clientAgreementVersion != expectedAgreementVersion {
		agreementMeta := map[string]any{
			"agreement_text_version":        strings.TrimSpace(agreementTextVersion),
			"agreement_version_verified":    false,
			"agreement_version_mismatch_at": s.now().UTC().Format(time.RFC3339Nano),
		}
		metaBytes, _ := json.Marshal(agreementMeta)
		if _, updateErr := s.repo.UpdateMeta(ctx, pending.ID, metaBytes); updateErr != nil {
			return "", "", "", pending, fmt.Errorf("update agreement mismatch meta: %w", updateErr)
		}
		return "", "", "", pending, ErrSignConfirmAgreementVersionMismatch
	}
	hashMeta := map[string]any{
		"document_hash_from_client":     clientHash,
		"document_hash_current":         currentHash,
		"agreement_text_version":        agreement.Version,
		"agreement_version_verified":    true,
		"agreement_version_verified_at": s.now().UTC().Format(time.RFC3339Nano),
	}
	if clientHash != currentHash {
		hashMeta["document_hash_verified"] = false
		hashMeta["document_hash_mismatch_at"] = s.now().UTC().Format(time.RFC3339Nano)
		metaBytes, _ := json.Marshal(hashMeta)
		if _, updateErr := s.repo.UpdateMeta(ctx, pending.ID, metaBytes); updateErr != nil {
			return "", "", "", pending, fmt.Errorf("update mismatch confirmation meta: %w", updateErr)
		}
		return "", "", "", pending, ErrSignConfirmDocMismatch
	}
	hashMeta["document_hash_verified"] = true
	hashMeta["document_hash_verified_at"] = s.now().UTC().Format(time.RFC3339Nano)
	signerEmail := extractSignerEmail(pending.Meta)
	metaUpdate := map[string]any{
		"ip":            ip,
		"user_agent":    userAgent,
		"method":        "email_magic_link",
		"document_hash": currentHash,
	}
	for key, value := range hashMeta {
		metaUpdate[key] = value
	}
	if signerEmail != "" {
		metaUpdate["signer_email"] = signerEmail
	}
	metaBytes, _ := json.Marshal(metaUpdate)
	approved, err := s.repo.Approve(ctx, pending.ID, metaBytes)
	if err != nil {
		return "", "", "", pending, fmt.Errorf("approve confirmation: %w", err)
	}
	s.logConfirmState("approved", approved.DocumentID, approved.ID, approved.UserID, approved.ExpiresAt, int(s.emailTTL/time.Minute), approved.Status, "otp_valid")
	return "approved", signerEmail, documentHash, approved, nil
}

func (s *DocumentSigningConfirmationService) confirmByTokenAndChannel(
	ctx context.Context,
	channel string,
	documentID int64,
	token, code, documentHashFromClient, agreementTextVersion, ip, userAgent string,
) (string, string, string, *models.SignatureConfirmation, error) {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	token = normalizeEmailConfirmToken(token)
	if token == "" {
		return "", "", "", nil, ErrSignConfirmNotFound
	}
	tokenHash := hashConfirmTokenWithPepper(token, s.tokenPepper)
	pending, err := s.repo.FindByTokenHash(ctx, channel, tokenHash)
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
	clientHash := normalizeDocumentHash(documentHashFromClient)
	currentHash := normalizeDocumentHash("sha256:" + documentHash)
	if clientHash == "" {
		return "", "", "", pending, ErrSignConfirmHashRequired
	}
	agreement := CurrentEmailSigningAgreement()
	clientAgreementVersion := strings.TrimSpace(strings.ToLower(agreementTextVersion))
	expectedAgreementVersion := strings.TrimSpace(strings.ToLower(agreement.Version))
	if clientAgreementVersion == "" {
		return "", "", "", pending, ErrSignConfirmAgreementVersionRequired
	}
	if clientAgreementVersion != expectedAgreementVersion {
		return "", "", "", pending, ErrSignConfirmAgreementVersionMismatch
	}
	if clientHash != currentHash {
		return "", "", "", pending, ErrSignConfirmDocMismatch
	}
	signerIdentity := extractSignerEmail(pending.Meta)
	if signerIdentity == "" {
		signerIdentity = extractSignerPhone(pending.Meta)
	}
	metaUpdate := map[string]any{
		"ip":                         ip,
		"user_agent":                 userAgent,
		"method":                     channel + "_otp",
		"document_hash":              currentHash,
		"document_hash_from_client":  clientHash,
		"document_hash_current":      currentHash,
		"agreement_text_version":     agreement.Version,
		"agreement_version_verified": true,
		"document_hash_verified":     true,
	}
	metaBytes, _ := json.Marshal(metaUpdate)
	approved, err := s.repo.Approve(ctx, pending.ID, metaBytes)
	if err != nil {
		return "", "", "", pending, fmt.Errorf("approve confirmation: %w", err)
	}
	return "approved", signerIdentity, documentHash, approved, nil
}

func normalizeDocumentHash(raw string) string {
	hash := strings.TrimSpace(strings.ToLower(raw))
	if hash == "" {
		return ""
	}
	if strings.HasPrefix(hash, "sha256:") {
		hash = strings.TrimPrefix(hash, "sha256:")
	}
	if len(hash) != 64 {
		return ""
	}
	for _, ch := range hash {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return ""
		}
	}
	return "sha256:" + hash
}

func (s *DocumentSigningConfirmationService) UpdateConfirmationMeta(ctx context.Context, confirmationID string, meta map[string]any) error {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	if s.repo == nil {
		return errors.New("signature confirmation repo is nil")
	}
	if strings.TrimSpace(confirmationID) == "" || len(meta) == 0 {
		return nil
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal confirmation meta update: %w", err)
	}
	if _, err := s.repo.UpdateMeta(ctx, confirmationID, metaBytes); err != nil {
		return fmt.Errorf("update confirmation meta: %w", err)
	}
	return nil
}

type EmailTokenVerification struct {
	TokenValid bool `json:"token_valid"`
	Document   struct {
		ID                  int64  `json:"id"`
		Title               string `json:"title"`
		Status              string `json:"status"`
		FileName            string `json:"file_name,omitempty"`
		ContentType         string `json:"content_type,omitempty"`
		PreviewURL          string `json:"preview_url,omitempty"`
		DocumentHashPreview string `json:"document_hash_preview,omitempty"`
	} `json:"document"`
	Confirmation struct {
		ExpiresAt time.Time `json:"expires_at"`
	} `json:"confirmation"`
	Agreement          EmailSigningAgreement `json:"agreement"`
	RequirePostConfirm bool                  `json:"require_post_confirm"`
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
		s.logConfirmState("expired", pending.DocumentID, pending.ID, pending.UserID, pending.ExpiresAt, int(s.emailTTL/time.Minute), "expired", "ttl_elapsed_validate")
		return nil, ErrSignConfirmExpired
	}
	if pending.Attempts >= signConfirmMaxAttempts {
		_ = s.repo.Expire(ctx, pending.ID)
		s.logConfirmState("expired", pending.DocumentID, pending.ID, pending.UserID, pending.ExpiresAt, int(s.emailTTL/time.Minute), "expired", "too_many_attempts_validate")
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
		TokenValid:         true,
		RequirePostConfirm: true,
	}
	response.Document.ID = doc.ID
	response.Document.Title = doc.DocType
	response.Document.Status = doc.Status
	response.Document.FileName = documentFileName(doc)
	response.Document.ContentType = documentContentType(response.Document.FileName)
	if hash, hashErr := s.hashDocumentContent(pending.DocumentID); hashErr == nil && strings.TrimSpace(hash) != "" {
		response.Document.DocumentHashPreview = "sha256:" + hash
	}
	response.Confirmation.ExpiresAt = pending.ExpiresAt
	response.Agreement = CurrentEmailSigningAgreement()
	return response, nil
}

func (s *DocumentSigningConfirmationService) ValidateSMSToken(
	ctx context.Context,
	token, ip, userAgent string,
) (*EmailTokenVerification, error) {
	response, err := s.validateTokenByChannel(ctx, "sms", token, ip, userAgent)
	if err != nil {
		return nil, err
	}
	return response, nil
}

type EmailDocumentPreview struct {
	DocumentID       int64
	ConfirmationID   string
	FileName         string
	ContentType      string
	DocumentHash     string
	AbsPath          string
	ConfirmationMeta json.RawMessage
}

func (s *DocumentSigningConfirmationService) PrepareEmailDocumentPreview(
	ctx context.Context,
	token string,
) (*EmailDocumentPreview, error) {
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

	doc, relPath, err := s.getDocumentAndPreviewPath(pending.DocumentID)
	if err != nil {
		return nil, err
	}
	root := strings.TrimSpace(s.filesRoot)
	if root == "" {
		return nil, errors.New("files root is required")
	}
	absPath := filepath.Join(root, relPath)
	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("open document: %w", err)
	}
	fileName := documentFileName(doc)
	documentHash := ""
	if hash, hashErr := s.hashDocumentContent(doc.ID); hashErr == nil && strings.TrimSpace(hash) != "" {
		documentHash = "sha256:" + hash
	}
	return &EmailDocumentPreview{
		DocumentID:       doc.ID,
		ConfirmationID:   pending.ID,
		FileName:         fileName,
		ContentType:      documentContentType(fileName),
		DocumentHash:     documentHash,
		AbsPath:          absPath,
		ConfirmationMeta: pending.Meta,
	}, nil
}

func (s *DocumentSigningConfirmationService) PrepareSMSDocumentPreview(
	ctx context.Context,
	token string,
) (*EmailDocumentPreview, error) {
	return s.prepareDocumentPreviewByChannel(ctx, "sms", token)
}

func (s *DocumentSigningConfirmationService) RecordEmailPreviewOpened(
	ctx context.Context,
	preview *EmailDocumentPreview,
	ip, userAgent string,
) error {
	// Preview audit is intentionally independent from verify-open audit:
	// verify tracks link opening, preview tracks actual file opening.
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	if s.repo == nil || preview == nil || strings.TrimSpace(preview.ConfirmationID) == "" {
		return nil
	}
	existing := map[string]any{}
	if len(preview.ConfirmationMeta) > 0 {
		_ = json.Unmarshal(preview.ConfirmationMeta, &existing)
	}
	openCount := intFromAny(existing["preview_open_count"]) + 1
	metaUpdate := map[string]any{
		"preview_opened_at":         s.now().UTC().Format(time.RFC3339Nano),
		"preview_open_count":        openCount,
		"preview_opened_ip":         strings.TrimSpace(ip),
		"preview_opened_user_agent": strings.TrimSpace(userAgent),
	}
	if val := strings.TrimSpace(preview.DocumentHash); val != "" {
		metaUpdate["preview_document_hash"] = val
	}
	if val := strings.TrimSpace(preview.FileName); val != "" {
		metaUpdate["preview_file_name"] = val
	}
	if val := strings.TrimSpace(preview.ContentType); val != "" {
		metaUpdate["preview_content_type"] = val
	}
	metaBytes, _ := json.Marshal(metaUpdate)
	if _, err := s.repo.UpdateMeta(ctx, preview.ConfirmationID, metaBytes); err != nil {
		return fmt.Errorf("update preview meta: %w", err)
	}
	return nil
}

func (s *DocumentSigningConfirmationService) RecordSMSPreviewOpened(
	ctx context.Context,
	preview *EmailDocumentPreview,
	ip, userAgent string,
) error {
	return s.RecordEmailPreviewOpened(ctx, preview, ip, userAgent)
}

func (s *DocumentSigningConfirmationService) ConfirmBySMSToken(
	ctx context.Context,
	documentID int64,
	token, code, documentHashFromClient, agreementTextVersion, ip, userAgent string,
) (string, string, string, *models.SignatureConfirmation, error) {
	return s.confirmByTokenAndChannel(ctx, "sms", documentID, token, code, documentHashFromClient, agreementTextVersion, ip, userAgent)
}

func (s *DocumentSigningConfirmationService) GetEmailConfirmationAudit(ctx context.Context, documentID, userID int64) (map[string]any, error) {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	if s.repo == nil {
		return nil, errors.New("signature confirmation repo is nil")
	}
	confirmation, err := s.repo.GetLatestByChannel(ctx, documentID, userID, "email")
	if err != nil {
		return nil, err
	}
	if confirmation == nil || len(confirmation.Meta) == 0 {
		return nil, nil
	}
	var meta map[string]any
	if err := json.Unmarshal(confirmation.Meta, &meta); err != nil {
		return nil, nil
	}
	audit := map[string]any{
		"link_opened_at":                firstNonEmpty(meta["link_opened_at"], meta["opened_at"]),
		"link_opened_ip":                firstNonEmpty(meta["opened_ip"]),
		"link_opened_user_agent":        firstNonEmpty(meta["opened_user_agent"]),
		"preview_opened_at":             firstNonEmpty(meta["preview_opened_at"]),
		"preview_opened_ip":             firstNonEmpty(meta["preview_opened_ip"]),
		"preview_opened_user_agent":     firstNonEmpty(meta["preview_opened_user_agent"]),
		"preview_open_count":            intFromAny(meta["preview_open_count"]),
		"preview_document_hash":         firstNonEmpty(meta["preview_document_hash"]),
		"preview_file_name":             firstNonEmpty(meta["preview_file_name"]),
		"preview_content_type":          firstNonEmpty(meta["preview_content_type"]),
		"agreed_at":                     firstNonEmpty(meta["agreed_at"]),
		"agreed_ip":                     firstNonEmpty(meta["agreed_ip"]),
		"agreed_user_agent":             firstNonEmpty(meta["agreed_user_agent"]),
		"agreement_text_version":        firstNonEmpty(meta["agreement_text_version"]),
		"agreement_version_verified":    meta["agreement_version_verified"],
		"agreement_version_verified_at": firstNonEmpty(meta["agreement_version_verified_at"]),
		"agreement_version_mismatch_at": firstNonEmpty(meta["agreement_version_mismatch_at"]),
		"document_hash_from_client":     firstNonEmpty(meta["document_hash_from_client"]),
		"document_hash_current":         firstNonEmpty(meta["document_hash_current"]),
		"document_hash_verified":        meta["document_hash_verified"],
		"document_hash_verified_at":     firstNonEmpty(meta["document_hash_verified_at"]),
		"document_hash_mismatch_at":     firstNonEmpty(meta["document_hash_mismatch_at"]),
	}
	return audit, nil
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

func (s *DocumentSigningConfirmationService) LookupSMSConfirmationByToken(
	ctx context.Context,
	token string,
) (*models.SignatureConfirmation, error) {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	token = normalizeEmailConfirmToken(token)
	if token == "" {
		return nil, nil
	}
	tokenHash := hashConfirmTokenWithPepper(token, s.tokenPepper)
	confirmation, err := s.repo.FindByTokenHash(ctx, "sms", tokenHash)
	if err != nil {
		return nil, fmt.Errorf("lookup sms confirmation by token hash: %w", err)
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
	channels := []string{"email", "sms"}
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
				s.logConfirmState("expired", latest.DocumentID, latest.ID, latest.UserID, latest.ExpiresAt, int(s.emailTTL/time.Minute), "expired", "status_poll_ttl_elapsed")
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

func (s *DocumentSigningConfirmationService) GetSMSConfirmationAudit(ctx context.Context, documentID, userID int64) (map[string]any, error) {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	confirmation, err := s.repo.GetLatestByChannel(ctx, documentID, userID, "sms")
	if err != nil || confirmation == nil || len(confirmation.Meta) == 0 {
		return nil, err
	}
	var meta map[string]any
	if err := json.Unmarshal(confirmation.Meta, &meta); err != nil {
		return nil, nil
	}
	approvedAt := ""
	if confirmation.ApprovedAt != nil {
		approvedAt = confirmation.ApprovedAt.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{
		"signer_phone":               firstNonEmpty(meta["signer_phone"]),
		"sent_at":                    firstNonEmpty(meta["sent_at"]),
		"opened_at":                  firstNonEmpty(meta["opened_at"]),
		"opened_ip":                  firstNonEmpty(meta["opened_ip"]),
		"opened_user_agent":          firstNonEmpty(meta["opened_user_agent"]),
		"preview_opened_at":          firstNonEmpty(meta["preview_opened_at"]),
		"preview_open_count":         intFromAny(meta["preview_open_count"]),
		"preview_document_hash":      firstNonEmpty(meta["preview_document_hash"]),
		"agreement_text_version":     firstNonEmpty(meta["agreement_text_version"]),
		"agreement_version_verified": meta["agreement_version_verified"],
		"document_hash_verified":     meta["document_hash_verified"],
		"approved_at":                firstNonEmpty(meta["approved_at"], approvedAt),
		"provider":                   firstNonEmpty(meta["provider"]),
		"provider_message_id":        firstNonEmpty(meta["provider_message_id"]),
	}, nil
}

func (s *DocumentSigningConfirmationService) logConfirmState(transition string, documentID int64, confirmationID string, userID int64, expiresAt time.Time, ttlMinutes int, status string, reason string) {
	nowUTC := s.now().UTC()
	nowLocal := nowUTC.In(s.serverTZ)
	log.Printf("[sign][confirm][%s] document_id=%d confirmation_id=%s user_id=%d server_tz=%s now_utc=%s now_local=%s expires_at=%s ttl_minutes=%d status=%s reason=%s",
		transition,
		documentID,
		strings.TrimSpace(confirmationID),
		userID,
		s.serverTZ.String(),
		nowUTC.Format(time.RFC3339Nano),
		nowLocal.Format(time.RFC3339Nano),
		expiresAt.UTC().Format(time.RFC3339Nano),
		ttlMinutes,
		status,
		reason,
	)
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

func (s *DocumentSigningConfirmationService) buildSMSVerifyURL(token string) (string, error) {
	base := strings.TrimRight(s.smsVerifyBase, "/")
	if base == "" {
		return "", errors.New("sign sms verify base URL is required")
	}
	target, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid sign sms verify base URL: %w", err)
	}
	target.Path = strings.TrimRight(target.Path, "/") + "/sign/sms/verify"
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
		openedAt := now.UTC().Format(time.RFC3339Nano)
		update["opened_at"] = openedAt
		if existing["link_opened_at"] == nil {
			update["link_opened_at"] = openedAt
		}
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

func extractSignerPhone(meta json.RawMessage) string {
	if len(meta) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(meta, &payload); err != nil {
		return ""
	}
	if val, ok := payload["signer_phone"].(string); ok {
		return strings.TrimSpace(val)
	}
	return ""
}

func (s *DocumentSigningConfirmationService) validateTokenByChannel(
	ctx context.Context,
	channel, token, ip, userAgent string,
) (*EmailTokenVerification, error) {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	token = normalizeEmailConfirmToken(token)
	if token == "" {
		return nil, ErrSignConfirmNotFound
	}
	tokenHash := hashConfirmTokenWithPepper(token, s.tokenPepper)
	pending, err := s.repo.FindByTokenHash(ctx, channel, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("find confirmation by token hash: %w", err)
	}
	if pending == nil {
		return nil, ErrSignConfirmNotFound
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
	metaUpdate := buildOpenMetaUpdate(pending.Meta, ip, userAgent, s.now())
	if len(metaUpdate) > 0 {
		metaBytes, _ := json.Marshal(metaUpdate)
		if _, err := s.repo.UpdateMeta(ctx, pending.ID, metaBytes); err != nil {
			return nil, fmt.Errorf("update confirmation meta: %w", err)
		}
	}
	doc, err := s.docRepo.GetByID(pending.DocumentID)
	if err != nil || doc == nil {
		return nil, ErrSignConfirmNotFound
	}
	response := &EmailTokenVerification{TokenValid: true, RequirePostConfirm: true}
	response.Document.ID = doc.ID
	response.Document.Title = doc.DocType
	response.Document.Status = doc.Status
	response.Document.FileName = documentFileName(doc)
	response.Document.ContentType = documentContentType(response.Document.FileName)
	if hash, hashErr := s.hashDocumentContent(pending.DocumentID); hashErr == nil && strings.TrimSpace(hash) != "" {
		response.Document.DocumentHashPreview = "sha256:" + hash
	}
	response.Confirmation.ExpiresAt = pending.ExpiresAt
	response.Agreement = CurrentEmailSigningAgreement()
	return response, nil
}

func (s *DocumentSigningConfirmationService) prepareDocumentPreviewByChannel(
	ctx context.Context,
	channel, token string,
) (*EmailDocumentPreview, error) {
	ctx, cancel := withSignConfirmTimeout(ctx)
	defer cancel()
	token = normalizeEmailConfirmToken(token)
	if token == "" {
		return nil, ErrSignConfirmNotFound
	}
	tokenHash := hashConfirmTokenWithPepper(token, s.tokenPepper)
	pending, err := s.repo.FindByTokenHash(ctx, channel, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("find confirmation by token hash: %w", err)
	}
	if pending == nil {
		return nil, ErrSignConfirmNotFound
	}
	if s.now().After(pending.ExpiresAt) {
		_ = s.repo.Expire(ctx, pending.ID)
		return nil, ErrSignConfirmExpired
	}
	doc, relPath, err := s.getDocumentAndPreviewPath(pending.DocumentID)
	if err != nil {
		return nil, err
	}
	absPath := filepath.Join(strings.TrimSpace(s.filesRoot), relPath)
	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("open document: %w", err)
	}
	documentHash := ""
	if hash, hashErr := s.hashDocumentContent(doc.ID); hashErr == nil && strings.TrimSpace(hash) != "" {
		documentHash = "sha256:" + hash
	}
	return &EmailDocumentPreview{
		DocumentID:       doc.ID,
		ConfirmationID:   pending.ID,
		FileName:         documentFileName(doc),
		ContentType:      documentContentType(documentFileName(doc)),
		DocumentHash:     documentHash,
		AbsPath:          absPath,
		ConfirmationMeta: pending.Meta,
	}, nil
}

func (s *DocumentSigningConfirmationService) hashDocumentContent(documentID int64) (string, error) {
	_, rel, err := s.getDocumentAndPreviewPath(documentID)
	if err != nil {
		return "", err
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

func (s *DocumentSigningConfirmationService) getDocumentAndPreviewPath(documentID int64) (*models.Document, string, error) {
	if s.docRepo == nil {
		return nil, "", errors.New("document repo is nil")
	}
	doc, err := s.docRepo.GetByID(documentID)
	if err != nil || doc == nil {
		return nil, "", errors.New("document not found")
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
		return nil, "", errors.New("bad filepath")
	}
	return doc, rel, nil
}

func documentFileName(doc *models.Document) string {
	if doc == nil {
		return ""
	}
	pathVal := strings.TrimSpace(doc.FilePathPdf)
	if pathVal == "" {
		pathVal = strings.TrimSpace(doc.FilePath)
	}
	name := strings.TrimSpace(filepath.Base(pathVal))
	if name == "." || name == "/" {
		return ""
	}
	return name
}

func documentContentType(fileName string) string {
	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName)))
	if strings.TrimSpace(ct) == "" {
		return "application/octet-stream"
	}
	return ct
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}

func firstNonEmpty(values ...any) any {
	for _, value := range values {
		if str, ok := value.(string); ok && strings.TrimSpace(str) != "" {
			return strings.TrimSpace(str)
		}
	}
	return nil
}
