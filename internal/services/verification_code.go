package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
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

// NormalizeVerificationCode removes non-digit characters from input.
func NormalizeVerificationCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(code))
	for _, r := range code {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// VerificationCodeDebugHash returns a stable debug hash for logs (sha256).
func VerificationCodeDebugHash(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// HashVerificationCode returns bcrypt hash for a verification code.
func HashVerificationCode(code string) (string, error) {
	code = strings.TrimSpace(code)
	codeHashBytes, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt generate: %w", err)
	}
	return string(codeHashBytes), nil
}

// CompareVerificationCode compares a bcrypt hash with a plaintext code.
func CompareVerificationCode(codeHash, code string) error {
	code = strings.TrimSpace(code)
	return bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(code))
}
