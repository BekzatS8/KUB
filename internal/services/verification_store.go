package services

import (
	"errors"
	"time"

	"turcompany/internal/models"
)

var (
	ErrResendThrottled = errors.New("resend throttled")
	ErrTooManyAttempts = errors.New("too many attempts")
	ErrCodeExpired     = errors.New("code expired")
	ErrCodeInvalid     = errors.New("code invalid")
)

type UserVerificationRepo interface {
	CountRecentSends(userID int, since time.Time) (int, error)
	Create(userID int, codeHash string, sentAt, expiresAt time.Time) (int64, error)
	GetLatestByUserID(userID int) (*models.UserVerification, error)
	IncrementAttempts(id int64) (int, error)
	ExpireNow(id int64) error
	MarkConfirmed(id int64) error
	Update(v *models.UserVerification) error
}
