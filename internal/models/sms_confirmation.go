// models/user_verification.go
package models

import "time"

// models/sms_confirmation.go
type SMSConfirmation struct {
	ID          int64     `json:"id"`
	DocumentID  int64     `json:"document_id"`
	Phone       string    `json:"phone"`
	SMSCode     string    `json:"sms_code"` // можно оставить как есть, или тоже сделать CodeHash
	SentAt      time.Time `json:"sent_at"`
	Confirmed   bool      `json:"confirmed"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}
