package services

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultVerificationTTL = 5 * time.Minute
	MaxConfirmAttempts     = 5
	UserResendCooldown     = time.Minute
	UserMaxResends         = 5
)

// GenerateVerificationCode returns a 6-digit numeric OTP.
func GenerateVerificationCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// HashVerificationCode returns bcrypt hash for a verification code.
func HashVerificationCode(code string) (string, error) {
	codeHashBytes, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt generate: %w", err)
	}
	return string(codeHashBytes), nil
}

// CompareVerificationCode compares a bcrypt hash with a plaintext code.
func CompareVerificationCode(codeHash, code string) error {
	return bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(code))
}
