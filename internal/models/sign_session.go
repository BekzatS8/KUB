package models

import "time"

type SignSession struct {
	ID              int64      `json:"id"`
	DocumentID      int64      `json:"document_id"`
	PhoneE164       string     `json:"phone_e164"`
	CodeHash        string     `json:"-"`
	TokenHash       string     `json:"-"`
	ExpiresAt       time.Time  `json:"expires_at"`
	Attempts        int        `json:"attempts"`
	Status          string     `json:"status"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	SignedAt        *time.Time `json:"signed_at,omitempty"`
	SignedIP        string     `json:"signed_ip,omitempty"`
	SignedUserAgent string     `json:"signed_user_agent,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
