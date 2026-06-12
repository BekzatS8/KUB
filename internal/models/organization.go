package models

import "time"

type Organization struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	LegalName string    `json:"legal_name,omitempty"`
	BIN       string    `json:"bin,omitempty"`
	Phone     string    `json:"phone,omitempty"`
	Email     string    `json:"email,omitempty"`
	Address   string    `json:"address,omitempty"`
	Website   string    `json:"website,omitempty"`
	WhatsApp  string    `json:"whatsapp,omitempty"`
	Telegram  string    `json:"telegram,omitempty"`
	Instagram string    `json:"instagram,omitempty"`
	TikTok    string    `json:"tiktok,omitempty"`
	LogoURL   string    `json:"logo_url,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type UpdateOrganizationRequest struct {
	Name      *string `json:"name"`
	LegalName *string `json:"legal_name"`
	BIN       *string `json:"bin"`
	Phone     *string `json:"phone"`
	Email     *string `json:"email"`
	Address   *string `json:"address"`
	Website   *string `json:"website"`
	WhatsApp  *string `json:"whatsapp"`
	Telegram  *string `json:"telegram"`
	Instagram *string `json:"instagram"`
	TikTok    *string `json:"tiktok"`
	LogoURL   *string `json:"logo_url"`
}
