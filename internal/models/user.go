package models

import "time"

type User struct {
	ID           int    `json:"id"`
	CompanyName  string `json:"company_name"`
	BinIin       string `json:"bin_iin"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"` // не отдаём
	RoleID       int    `json:"role_id"`
	// новое:
	Phone               string     `json:"phone"`
	IsVerified          bool       `json:"is_verified"`
	VerifiedAt          *time.Time `json:"verified_at,omitempty"`
	TelegramChatID      int64      `json:"telegram_chat_id"`
	NotifyTasksTelegram bool       `json:"notify_tasks_telegram"`

	// refresh:
	RefreshToken     *string    `json:"-"`
	RefreshExpiresAt *time.Time `json:"-"`
	RefreshRevoked   bool       `json:"-"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
