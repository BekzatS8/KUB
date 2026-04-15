package services

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"turcompany/internal/middleware"
	"turcompany/internal/utils"
)

type AuthService interface {
	VerifyPassword(hash, password string) bool
	HashPassword(password string) (string, error)
	GenerateAccessToken(userID, roleID int, activeCompanyID *int) (string, time.Time, error)
	GenerateRefreshToken() (string, time.Time, error)
}

type authService struct {
	AccessSecret  []byte
	RefreshSecret []byte
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	now           func() time.Time
}

func NewAuthService(accessSecret, refreshSecret []byte, accessTTL, refreshTTL time.Duration, now func() time.Time) AuthService {
	if len(accessSecret) == 0 {
		panic("access secret is required")
	}
	if accessTTL <= 0 {
		accessTTL = 2 * time.Hour
	}
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	if now == nil {
		now = time.Now
	}
	return &authService{
		AccessSecret:  accessSecret,
		RefreshSecret: refreshSecret,
		AccessTTL:     accessTTL,
		RefreshTTL:    refreshTTL,
		now:           now,
	}
}

func (s *authService) VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *authService) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func (s *authService) GenerateAccessToken(userID, roleID int, activeCompanyID *int) (string, time.Time, error) {
	nowUTC := s.now().UTC()
	expiresAt := nowUTC.Add(s.AccessTTL)
	accessClaims := &middleware.Claims{
		UserID:          userID,
		RoleID:          roleID,
		ActiveCompanyID: activeCompanyID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(nowUTC),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	signed, err := token.SignedString(s.AccessSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

func (s *authService) GenerateRefreshToken() (string, time.Time, error) {
	token, err := utils.NewRefreshToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, s.now().Add(s.RefreshTTL), nil
}
