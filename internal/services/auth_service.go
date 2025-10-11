package services

import (
	"golang.org/x/crypto/bcrypt"
)

type AuthService interface {
	VerifyPassword(hash, password string) bool
	HashPassword(password string) (string, error)
}

type authService struct{}

func NewAuthService() AuthService {
	return &authService{}
}

func (s *authService) VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *authService) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}
