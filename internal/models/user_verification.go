package models

import "time"

// UserVerification — отдельная запись на каждую отправку кода.
// Мы храним только bcrypt-хэш кода (CodeHash), TTL и счётчик попыток.
type UserVerification struct {
	ID        int64     `json:"id"`
	UserID    int       `json:"user_id"`
	CodeHash  string    `json:"-"`
	SentAt    time.Time `json:"sent_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Confirmed bool      `json:"confirmed"`
	Attempts  int       `json:"attempts"`
}
