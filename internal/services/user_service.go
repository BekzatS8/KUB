package services

import (
	"fmt"
	"log"
	"strings"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type UserService interface {
	CreateUser(user *models.User) error
	CreateUserWithPassword(user *models.User, plainPassword string) error
	GetUserByID(id int) (*models.User, error)
	UpdateUser(user *models.User) error
	DeleteUser(id int) error
	ListUsers(limit, offset int) ([]*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserCount() (int, error)
	GetUserCountByRole(roleID int) (int, error)
}

type userService struct {
	repo         repositories.UserRepository
	emailService EmailService
	authService  AuthService
}

func NewUserService(repo repositories.UserRepository, emailService EmailService, authService AuthService) UserService {
	return &userService{
		repo:         repo,
		emailService: emailService,
		authService:  authService,
	}
}

// CreateUserWithPassword - preferred: pass plain password here
func (s *userService) CreateUserWithPassword(user *models.User, plainPassword string) error {
	if strings.TrimSpace(plainPassword) == "" {
		return fmt.Errorf("password is required")
	}

	hashedPassword, err := s.authService.HashPassword(plainPassword)
	if err != nil {
		return err
	}
	user.PasswordHash = hashedPassword

	if err := s.repo.Create(user); err != nil {
		return err
	}

	if s.emailService != nil {
		if err := s.emailService.SendWelcomeEmail(user.Email, user.CompanyName); err != nil {
			// warn but do not fail creation
			log.Printf("CreateUserWithPassword: warning: failed to send welcome email to %s: %v", user.Email, err)
		}
	}

	return nil
}

// CreateUser - backward compatible: if PasswordHash looks like a plain password, hash it; if it's already a bcrypt hash, keep as is
func (s *userService) CreateUser(user *models.User) error {
	if user.PasswordHash == "" {
		return fmt.Errorf("password not provided")
	}
	ph := strings.TrimSpace(user.PasswordHash)
	// detect bcrypt hash prefix
	if !(strings.HasPrefix(ph, "$2a$") || strings.HasPrefix(ph, "$2b$") || strings.HasPrefix(ph, "$2y$")) {
		// looks like plain password -> hash it
		h, err := s.authService.HashPassword(ph)
		if err != nil {
			return err
		}
		user.PasswordHash = h
	} else {
		// already a hash -> keep
		user.PasswordHash = ph
	}

	if err := s.repo.Create(user); err != nil {
		return err
	}

	if s.emailService != nil {
		if err := s.emailService.SendWelcomeEmail(user.Email, user.CompanyName); err != nil {
			log.Printf("CreateUser: warning: failed to send welcome email to %s: %v", user.Email, err)
		}
	}

	return nil
}

func (s *userService) GetUserByID(id int) (*models.User, error) {
	return s.repo.GetByID(id)
}

func (s *userService) UpdateUser(user *models.User) error {
	return s.repo.Update(user)
}

func (s *userService) DeleteUser(id int) error {
	return s.repo.Delete(id)
}

func (s *userService) ListUsers(limit, offset int) ([]*models.User, error) {
	return s.repo.List(limit, offset)
}

func (s *userService) GetUserByEmail(email string) (*models.User, error) {
	return s.repo.GetByEmail(email)
}

func (s *userService) GetUserCount() (int, error) {
	return s.repo.GetCount()
}

func (s *userService) GetUserCountByRole(roleID int) (int, error) {
	return s.repo.GetCountByRole(roleID)
}
