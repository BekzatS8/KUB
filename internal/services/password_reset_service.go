package services

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"turcompany/internal/repositories"
	"turcompany/internal/utils"
)

type PasswordResetService interface {
	RequestReset(email string) error
	ResetPassword(token, newPassword string) error
}

type passwordResetService struct {
	userRepo repositories.UserRepository
	repo     repositories.PasswordResetRepository
	emails   EmailService
	auth     AuthService
}

func NewPasswordResetService(userRepo repositories.UserRepository, repo repositories.PasswordResetRepository, emails EmailService, auth AuthService) PasswordResetService {
	return &passwordResetService{
		userRepo: userRepo,
		repo:     repo,
		emails:   emails,
		auth:     auth,
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
	if _, err := s.repo.Create(user.ID, token, expires); err != nil {
		return err
	}

	if s.emails != nil {
		if err := s.emails.SendPasswordResetEmail(user.Email, token); err != nil {
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
	if err != nil || pr == nil {
		return errors.New("invalid or expired token")
	}
	if pr.UsedAt != nil {
		return errors.New("token already used")
	}
	if time.Now().After(pr.ExpiresAt) {
		return errors.New("token expired")
	}

	hash, err := s.auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.userRepo.UpdatePassword(pr.UserID, hash); err != nil {
		return err
	}
	return s.repo.MarkUsed(pr.ID)
}
