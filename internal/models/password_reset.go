package models

import "time"

type PasswordReset struct {
	ID        int        `json:"id"`
	UserID    int        `json:"user_id"`
	Token     string     `json:"token"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
