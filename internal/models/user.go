package models

import "time"

type User struct {
	ID           int    `json:"id"`
	CompanyName  string `json:"company_name"`
	BinIin       string `json:"bin_iin"`
	IIN          string `json:"iin,omitempty"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	MiddleName   string `json:"middle_name,omitempty"`
	Position     string `json:"position,omitempty"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"` // не отдаём
	RoleID       int    `json:"role_id"`
	BranchID     *int   `json:"branch_id,omitempty"`
	DepartmentID *int   `json:"department_id,omitempty"`
	IsActive     bool   `json:"is_active"`
	IsActiveSet  bool   `json:"-"`

	Phone               string     `json:"phone"`
	Address             string     `json:"address,omitempty"`
	ExtraInfo           string     `json:"extra_info,omitempty"`
	AvatarURL           string     `json:"avatar_url,omitempty"`
	AvatarPath          string     `json:"-"`
	AvatarOriginalPath  string     `json:"-"`
	AvatarCropX         *float64   `json:"avatar_crop_x,omitempty"`
	AvatarCropY         *float64   `json:"avatar_crop_y,omitempty"`
	AvatarCropScale     *float64   `json:"avatar_crop_scale,omitempty"`
	AvatarCropSize      *float64   `json:"avatar_crop_size,omitempty"`
	IsVerified          bool       `json:"is_verified"`
	VerifiedAt          *time.Time `json:"verified_at,omitempty"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
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
