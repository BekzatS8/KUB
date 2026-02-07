package services

import (
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"turcompany/internal/models"
)

// UserVerificationService handles registration verification via email.
type UserVerificationService struct {
	Repo     UserVerificationRepo
	UserSvc  UserService
	EmailSvc EmailService
	CodeTTL  time.Duration
	now      func() time.Time
}

func NewUserVerificationService(
	repo UserVerificationRepo,
	userSvc UserService,
	emailSvc EmailService,
	now func() time.Time,
) *UserVerificationService {
	if now == nil {
		now = time.Now
	}
	return &UserVerificationService{
		Repo:     repo,
		UserSvc:  userSvc,
		EmailSvc: emailSvc,
		CodeTTL:  DefaultVerificationTTL,
		now:      now,
	}
}

// Send creates a verification record and sends an email with the OTP.
func (s *UserVerificationService) Send(userID int, email string) error {
	if s.Repo == nil {
		return fmt.Errorf("verification repo is nil")
	}
	if s.EmailSvc == nil {
		return fmt.Errorf("email service is nil")
	}
	if strings.TrimSpace(email) == "" {
		return fmt.Errorf("email required")
	}

	code := NormalizeVerificationCode(GenerateVerificationCode())
	codeHash, err := HashVerificationCode(code)
	if err != nil {
		return err
	}

	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = DefaultVerificationTTL
	}
	sentAt := s.now()
	expiresAt := sentAt.Add(ttl)

	if _, err := s.Repo.Create(userID, codeHash, sentAt, expiresAt); err != nil {
		return err
	}

	if err := s.EmailSvc.SendVerificationCode(email, code, ttlMinutes(ttl)); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}

	log.Printf("[email][user][send] user_id=%d email=%s", userID, email)
	return nil
}

// Resend generates a new code, updates the record, and sends email.
func (s *UserVerificationService) Resend(userID int) error {
	if s.UserSvc == nil {
		return fmt.Errorf("user service is nil")
	}
	if s.Repo == nil {
		return fmt.Errorf("verification repo is nil")
	}
	if s.EmailSvc == nil {
		return fmt.Errorf("email service is nil")
	}

	if userID <= 0 {
		return ErrNoPendingVerification
	}

	user, err := s.UserSvc.GetUserByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		log.Printf("[verify][resend] user_id=%d record=false reason=user_not_found", userID)
		return ErrNoPendingVerification
	}
	if user.IsVerified {
		log.Printf("[verify][resend] user_id=%d record=false reason=already_verified", userID)
		return ErrAlreadyVerified
	}
	if strings.TrimSpace(user.Email) == "" {
		log.Printf("[verify][resend] user_id=%d record=false reason=missing_email", userID)
		return ErrNoPendingVerification
	}

	existing, err := s.Repo.GetLatestByUserID(user.ID)
	if err != nil {
		return err
	}

	now := s.now()
	if existing == nil {
		log.Printf("[verify][resend] user_id=%d record=false reason=not_found", userID)
	}
	if existing != nil {
		log.Printf(
			"[verify][resend] user_id=%d record=true expires_at=%s attempts=%d resend_count=%d reason=check_active",
			userID,
			existing.ExpiresAt.Format(time.RFC3339),
			existing.Attempts,
			existing.ResendCount,
		)
	}

	if existing == nil || existing.Confirmed || now.After(existing.ExpiresAt) {
		return s.Send(user.ID, user.Email)
	}
	if existing.LastResendAt != nil && now.Sub(*existing.LastResendAt) < UserResendCooldown {
		log.Printf("[verify][resend] user_id=%d record=true reason=cooldown", userID)
		return ErrResendThrottled
	}
	if existing.ResendCount >= UserMaxResends {
		log.Printf("[verify][resend] user_id=%d record=true reason=max_resends", userID)
		return ErrResendThrottled
	}

	code := NormalizeVerificationCode(GenerateVerificationCode())
	codeHash, err := HashVerificationCode(code)
	if err != nil {
		return err
	}

	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = DefaultVerificationTTL
	}

	existing.CodeHash = codeHash
	existing.SentAt = now
	existing.ExpiresAt = now.Add(ttl)
	existing.Attempts = 0
	existing.Confirmed = false
	existing.ConfirmedAt = nil
	existing.LastResendAt = &now
	existing.ResendCount++

	if err := s.Repo.Update(existing); err != nil {
		return err
	}

	if err := s.EmailSvc.SendVerificationCode(user.Email, code, ttlMinutes(ttl)); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}

	log.Printf("[email][user][resend] user_id=%d email=%s", user.ID, user.Email)
	return nil
}

// Confirm checks the verification code and marks the user verified.
func (s *UserVerificationService) Confirm(userID int, code string) (bool, error) {
	if s.UserSvc == nil {
		return false, fmt.Errorf("user service is nil")
	}
	if s.Repo == nil {
		return false, fmt.Errorf("verification repo is nil")
	}

	code = NormalizeVerificationCode(code)
	if userID <= 0 || code == "" {
		return false, ErrCodeInvalid
	}

	user, err := s.UserSvc.GetUserByID(userID)
	if err != nil {
		return false, err
	}
	if user == nil {
		log.Printf("[verify][confirm] user_id=%d record=false reason=user_not_found", userID)
		return false, ErrNoPendingVerification
	}
	if user.IsVerified {
		log.Printf("[verify][confirm] user_id=%d record=false reason=already_verified", userID)
		return false, ErrAlreadyVerified
	}

	v, err := s.Repo.GetLatestByUserID(user.ID)
	if err != nil {
		return false, err
	}
	if v == nil || v.Confirmed {
		if shouldLogVerificationDebug() {
			log.Printf("[verify][confirm][debug] user_id=%d email=%s record=%t reason=NOT_FOUND", userID, user.Email, v != nil)
		}
		log.Printf("[verify][confirm] user_id=%d record=%t reason=no_pending", userID, v != nil)
		return false, ErrNoPendingVerification
	}
	if s.now().After(v.ExpiresAt) {
		logVerifyConfirmDebug(userID, user.Email, v, code, "EXPIRED")
		log.Printf(
			"[verify][confirm] user_id=%d record=true expires_at=%s attempts=%d reason=expired",
			userID,
			v.ExpiresAt.Format(time.RFC3339),
			v.Attempts,
		)
		return false, ErrCodeExpired
	}

	if err := CompareVerificationCode(v.CodeHash, code); err != nil {
		logVerifyConfirmDebug(userID, user.Email, v, code, "HASH_MISMATCH")
		attempts, incErr := s.Repo.IncrementAttempts(v.ID)
		if incErr != nil {
			return false, incErr
		}
		if attempts >= MaxConfirmAttempts {
			_ = s.Repo.ExpireNow(v.ID)
			log.Printf(
				"[verify][confirm] user_id=%d record=true expires_at=%s attempts=%d reason=too_many_attempts",
				userID,
				v.ExpiresAt.Format(time.RFC3339),
				attempts,
			)
			return false, ErrTooManyAttempts
		}
		log.Printf(
			"[verify][confirm] user_id=%d record=true expires_at=%s attempts=%d reason=invalid_code",
			userID,
			v.ExpiresAt.Format(time.RFC3339),
			attempts,
		)
		return false, ErrCodeInvalid
	}

	if err := s.Repo.MarkConfirmed(v.ID); err != nil {
		return false, err
	}
	if err := s.UserSvc.VerifyUser(user.ID); err != nil {
		return false, err
	}

	log.Printf("[email][user][confirm] OK user_id=%d", user.ID)
	return true, nil
}

// Latest returns the most recent verification record for debugging.
func (s *UserVerificationService) Latest(userID int) (*models.UserVerification, *models.User, error) {
	if s.UserSvc == nil {
		return nil, nil, fmt.Errorf("user service is nil")
	}
	if s.Repo == nil {
		return nil, nil, fmt.Errorf("verification repo is nil")
	}
	if userID <= 0 {
		return nil, nil, ErrNoPendingVerification
	}
	user, err := s.UserSvc.GetUserByID(userID)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, ErrNoPendingVerification
	}
	v, err := s.Repo.GetLatestByUserID(user.ID)
	if err != nil {
		return nil, user, err
	}
	return v, user, nil
}

func ttlMinutes(ttl time.Duration) int {
	if ttl <= 0 {
		ttl = DefaultVerificationTTL
	}
	return int(math.Ceil(ttl.Minutes()))
}

func logVerifyConfirmDebug(userID int, email string, v *models.UserVerification, code, reason string) {
	if v == nil || !shouldLogVerificationDebug() {
		return
	}
	now := time.Now()
	codeHashPrefix := ""
	if len(v.CodeHash) >= 8 {
		codeHashPrefix = v.CodeHash[:8]
	} else {
		codeHashPrefix = v.CodeHash
	}
	debugHash := VerificationCodeDebugHash(code)
	debugHashPrefix := debugHash
	if len(debugHashPrefix) >= 8 {
		debugHashPrefix = debugHashPrefix[:8]
	}
	log.Printf(
		"[verify][confirm][debug] user_id=%d email=%s record=true expires_at=%s now=%s expired=%t attempts=%d code_hash_len=%d code_hash_prefix=%s code_debug_hash_prefix=%s reason=%s",
		userID,
		email,
		v.ExpiresAt.Format(time.RFC3339),
		now.Format(time.RFC3339),
		now.After(v.ExpiresAt),
		v.Attempts,
		len(v.CodeHash),
		codeHashPrefix,
		debugHashPrefix,
		reason,
	)
}

func shouldLogVerificationDebug() bool {
	return strings.ToLower(os.Getenv("GIN_MODE")) != "release"
}
