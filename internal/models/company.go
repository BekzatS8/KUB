package models

import "time"

type Company struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	LegalName   *string   `json:"legal_name,omitempty"`
	BinIin      *string   `json:"bin_iin,omitempty"`
	CompanyType string    `json:"company_type"`
	Phone       *string   `json:"phone,omitempty"`
	Email       *string   `json:"email,omitempty"`
	Address     *string   `json:"address,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserCompany struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	CompanyID int       `json:"company_id"`
	IsPrimary bool      `json:"is_primary"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	Company   *Company  `json:"company,omitempty"`
}
