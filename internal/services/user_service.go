package services

import (
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type UserService interface {
	CreateUser(user *models.User) error
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

func (s *userService) CreateUser(user *models.User) error {
	hashedPassword, err := s.authService.HashPassword(user.PasswordHash)
	if err != nil {
		return err
	}
	user.PasswordHash = hashedPassword

	if err := s.repo.Create(user); err != nil {
		return err
	}

	if err := s.emailService.SendWelcomeEmail(user.Email, user.CompanyName); err != nil {
		return nil
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
