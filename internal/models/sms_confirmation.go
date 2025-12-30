// models/user_verification.go
package models

import "time"

// models/sms_confirmation.go
type SMSConfirmation struct {
	ID           int64      `json:"id"`
	DocumentID   int64      `json:"document_id"`
	Phone        string     `json:"phone"`
	CodeHash     string     `json:"-"`
	SentAt       time.Time  `json:"sent_at"`
	ExpiresAt    time.Time  `json:"-"`
	Attempts     int        `json:"-"`
	Confirmed    bool       `json:"confirmed"`
	ConfirmedAt  time.Time  `json:"confirmed_at"`
	LastResendAt *time.Time `json:"-"`
	ResendCount  int        `json:"-"`
}
