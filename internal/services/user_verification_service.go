package services

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"
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

	code := GenerateVerificationCode()
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
func (s *UserVerificationService) Resend(email string) error {
	if s.UserSvc == nil {
		return fmt.Errorf("user service is nil")
	}
	if s.Repo == nil {
		return fmt.Errorf("verification repo is nil")
	}
	if s.EmailSvc == nil {
		return fmt.Errorf("email service is nil")
	}

	user, err := s.UserSvc.GetUserByEmail(email)
	if err != nil {
		return err
	}
	if user == nil || strings.TrimSpace(user.Email) == "" {
		return fmt.Errorf("email required")
	}

	existing, err := s.Repo.GetLatestByUserID(user.ID)
	if err != nil {
		return err
	}

	now := s.now()
	if existing == nil || existing.Confirmed || now.After(existing.ExpiresAt) {
		return s.Send(user.ID, user.Email)
	}
	if existing.LastResendAt != nil && now.Sub(*existing.LastResendAt) < UserResendCooldown {
		return ErrResendThrottled
	}
	if existing.ResendCount >= UserMaxResends {
		return ErrResendThrottled
	}

	code := GenerateVerificationCode()
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
func (s *UserVerificationService) Confirm(email, code string) (bool, error) {
	if s.UserSvc == nil {
		return false, fmt.Errorf("user service is nil")
	}
	if s.Repo == nil {
		return false, fmt.Errorf("verification repo is nil")
	}

	user, err := s.UserSvc.GetUserByEmail(email)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, ErrCodeInvalid
	}

	v, err := s.Repo.GetLatestByUserID(user.ID)
	if err != nil {
		return false, err
	}
	if v == nil || v.Confirmed {
		return false, ErrCodeInvalid
	}
	if s.now().After(v.ExpiresAt) {
		return false, ErrCodeExpired
	}

	if err := CompareVerificationCode(v.CodeHash, code); err != nil {
		attempts, incErr := s.Repo.IncrementAttempts(v.ID)
		if incErr != nil {
			return false, incErr
		}
		if attempts >= MaxConfirmAttempts {
			_ = s.Repo.ExpireNow(v.ID)
			return false, ErrTooManyAttempts
		}
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

func ttlMinutes(ttl time.Duration) int {
	if ttl <= 0 {
		ttl = DefaultVerificationTTL
	}
	return int(math.Ceil(ttl.Minutes()))
}
