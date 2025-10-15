package models

import "time"

type User struct {
	ID           int    `json:"id"`
	CompanyName  string `json:"company_name"`
	BinIin       string `json:"bin_iin"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"` // не отдаём наружу
	RoleID       int    `json:"role_id"`

	// refresh-хранение в БД
	RefreshToken     *string    `json:"-"` // храним opaque строку
	RefreshExpiresAt *time.Time `json:"-"` // срок действия
	RefreshRevoked   bool       `json:"-"` // если понадобится отозвать
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
