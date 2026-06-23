package services

import (
	"fmt"
	"log"
	"strings"
	"time"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type UserService interface {
	CreateUser(user *models.User) error
	CreateUserWithPassword(user *models.User, plainPassword string) error
	GetUserByID(id int) (*models.User, error)
	UpdateUser(user *models.User) error
	ApplyUpdatePatch(userID int, patch *models.UserApprovalUpdatePayload) error
	DeleteUser(id int) error
	ListUsers(limit, offset int) ([]*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetAuthUserByEmail(email string) (*models.User, error)
	GetUserCount() (int, error)
	GetUserCountByRole(roleID int) (int, error)
	UpdateProfile(userID int, profile *models.User) error
	UpdateAvatar(userID int, avatarURL, avatarPath, originalPath string) error
	UpdateAvatarCrop(userID int, cropX, cropY, cropScale, cropSize *float64) error
	DeleteAvatar(userID int) error

	// refresh helpers
	UpdateRefresh(userID int, token string, expiresAt time.Time) error
	GetByRefreshToken(token string) (*models.User, error)
	RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error)

	// verification
	VerifyUser(userID int) error

	AdminChangePassword(userID int, newPassword string) error
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
	normalizeUserVerificationForCreate(user)

	if err := s.repo.Create(user); err != nil {
		return normalizeUserCreateError(err)
	}

	if s.emailService != nil {
		welcomeName := strings.TrimSpace(user.FirstName + " " + user.LastName)
		if welcomeName == "" {
			welcomeName = user.CompanyName
		}
		if err := s.emailService.SendWelcomeEmail(user.Email, welcomeName); err != nil {
			log.Printf("CreateUserWithPassword: warning: failed to send welcome email to %s: %v", user.Email, err)
		}
	}
	return nil
}

// CreateUser - backward compatible: if PasswordHash looks like plain pwd, hash; if bcrypt — keep
func (s *userService) CreateUser(user *models.User) error {
	if user.PasswordHash == "" {
		return fmt.Errorf("password not provided")
	}
	ph := strings.TrimSpace(user.PasswordHash)
	if !(strings.HasPrefix(ph, "$2a$") || strings.HasPrefix(ph, "$2b$") || strings.HasPrefix(ph, "$2y$")) {
		h, err := s.authService.HashPassword(ph)
		if err != nil {
			return err
		}
		user.PasswordHash = h
	} else {
		user.PasswordHash = ph
	}
	normalizeUserVerificationForCreate(user)

	if err := s.repo.Create(user); err != nil {
		return normalizeUserCreateError(err)
	}

	if s.emailService != nil {
		welcomeName := strings.TrimSpace(user.FirstName + " " + user.LastName)
		if welcomeName == "" {
			welcomeName = user.CompanyName
		}
		if err := s.emailService.SendWelcomeEmail(user.Email, welcomeName); err != nil {
			log.Printf("CreateUser: warning: failed to send welcome email to %s: %v", user.Email, err)
		}
	}
	return nil
}

func (s *userService) AdminChangePassword(userID int, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return fmt.Errorf("password is required")
	}
	hashed, err := s.authService.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(userID, hashed)
}

func (s *userService) GetUserByID(id int) (*models.User, error) {
	return s.repo.GetByID(id)
}

func (s *userService) UpdateUser(user *models.User) error {
	return s.repo.Update(user)
}

func (s *userService) ApplyUpdatePatch(userID int, patch *models.UserApprovalUpdatePayload) error {
	return s.repo.ApplyUserPatch(userID, patch)
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

func (s *userService) GetAuthUserByEmail(email string) (*models.User, error) {
	return s.repo.GetAuthByEmail(email)
}

func (s *userService) GetUserCount() (int, error) {
	return s.repo.GetCount()
}

func (s *userService) GetUserCountByRole(roleID int) (int, error) {
	return s.repo.GetCountByRole(roleID)
}

func (s *userService) UpdateProfile(userID int, profile *models.User) error {
	return s.repo.UpdateProfile(userID, profile)
}

func (s *userService) UpdateAvatar(userID int, avatarURL, avatarPath, originalPath string) error {
	return s.repo.UpdateAvatar(userID, avatarURL, avatarPath, originalPath)
}

func (s *userService) UpdateAvatarCrop(userID int, cropX, cropY, cropScale, cropSize *float64) error {
	return s.repo.UpdateAvatarCrop(userID, cropX, cropY, cropScale, cropSize)
}

func (s *userService) DeleteAvatar(userID int) error {
	return s.repo.DeleteAvatar(userID)
}

func (s *userService) UpdateRefresh(userID int, token string, expiresAt time.Time) error {
	return s.repo.UpdateRefresh(userID, token, expiresAt)
}

func (s *userService) GetByRefreshToken(token string) (*models.User, error) {
	return s.repo.GetByRefreshToken(token)
}

func (s *userService) RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error) {
	return s.repo.RotateRefresh(oldToken, newToken, newExpiresAt)
}

// === verification ===
func (s *userService) VerifyUser(userID int) error {
	return s.repo.VerifyUser(userID)
}

func normalizeUserCreateError(err error) error {
	if repositories.IsSQLState(err, repositories.SQLStateUniqueViolation) {
		if repositories.ConstraintName(err) == "users_email_key" {
			return ErrEmailAlreadyUsed
		}
	}
	return err
}

func normalizeUserVerificationForCreate(user *models.User) {
	if user == nil {
		return
	}
	if user.IsVerified {
		if user.VerifiedAt == nil {
			now := time.Now().UTC()
			user.VerifiedAt = &now
		}
		return
	}
	user.VerifiedAt = nil
}
