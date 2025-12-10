package services

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"turcompany/internal/repositories"
	"turcompany/internal/utils"
)

var (
	ErrResetTokenNotFound = errors.New("reset token not found")
	ErrResetTokenExpired  = errors.New("reset token expired")
	ErrResetTokenUsed     = errors.New("reset token already used")
)

type PasswordResetService interface {
	RequestReset(email string) error
	ResetPassword(token, newPassword string) error
}

type passwordResetService struct {
	userRepo     repositories.UserRepository
	repo         repositories.PasswordResetRepository
	emails       EmailService
	auth         AuthService
	frontendHost string
}

func NewPasswordResetService(userRepo repositories.UserRepository, repo repositories.PasswordResetRepository, emails EmailService, auth AuthService, frontendHost string) PasswordResetService {
	return &passwordResetService{
		userRepo:     userRepo,
		repo:         repo,
		emails:       emails,
		auth:         auth,
		frontendHost: strings.TrimSpace(frontendHost),
	}
}

func (s *passwordResetService) RequestReset(email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return fmt.Errorf("email is required")
	}
	user, err := s.userRepo.GetByEmail(email)
	if err != nil || user == nil {
		// don't leak existence
		log.Printf("[password-reset] request for %q: user not found or error: %v", email, err)
		return nil
	}

	token, err := utils.NewRefreshToken(32)
	if err != nil {
		return err
	}
	expires := time.Now().Add(1 * time.Hour)
	if err := s.repo.Create(user.ID, token, expires); err != nil {
		return err
	}

	if s.emails != nil {
		resetURL := s.buildResetURL(token)
		if err := s.emails.SendPasswordResetEmail(user.Email, resetURL); err != nil {
			log.Printf("[password-reset] failed to send email to %s: %v", user.Email, err)
		}
	}
	return nil
}

func (s *passwordResetService) ResetPassword(token, newPassword string) error {
	token = strings.TrimSpace(token)
	newPassword = strings.TrimSpace(newPassword)
	if token == "" || newPassword == "" {
		return fmt.Errorf("token and password are required")
	}
	if len(newPassword) < 6 {
		return fmt.Errorf("password must be at least 6 characters")
	}

	pr, err := s.repo.GetByToken(token)
	if err != nil {
		return err
	}
	if pr == nil {
		return ErrResetTokenNotFound
	}
	if pr.Used {
		return ErrResetTokenUsed
	}
	if time.Now().After(pr.ExpiresAt) {
		return ErrResetTokenExpired
	}

	hash, err := s.auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.userRepo.UpdatePassword(pr.UserID, hash); err != nil {
		return err
	}
	return s.repo.MarkUsed(pr.Token)
}

func (s *passwordResetService) buildResetURL(token string) string {
	base := strings.TrimSpace(s.frontendHost)
	if base == "" {
		return ""
	}

	base = strings.TrimRight(base, "/")
	escapedToken := url.QueryEscape(token)
	return fmt.Sprintf("%s/reset-password?token=%s", base, escapedToken)
}
