package models

import (
	"encoding/json"
	"time"
)

type SignatureConfirmation struct {
	ID         string          `json:"id"`
	DocumentID int64           `json:"document_id"`
	UserID     int64           `json:"user_id"`
	Channel    string          `json:"channel"`
	Status     string          `json:"status"`
	OTPHash    *string         `json:"-"`
	TokenHash  *string         `json:"-"`
	Attempts   int             `json:"attempts"`
	ExpiresAt  time.Time       `json:"expires_at"`
	ApprovedAt *time.Time      `json:"approved_at,omitempty"`
	RejectedAt *time.Time      `json:"rejected_at,omitempty"`
	Meta       json.RawMessage `json:"meta,omitempty"`
}
