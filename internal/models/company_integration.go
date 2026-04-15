package models

import "time"

type CompanyIntegration struct {
	ID                int64     `json:"id"`
	CompanyID         int       `json:"company_id"`
	IntegrationType   string    `json:"integration_type"`
	Provider          *string   `json:"provider,omitempty"`
	Title             string    `json:"title"`
	ExternalAccountID *string   `json:"external_account_id,omitempty"`
	Phone             *string   `json:"phone,omitempty"`
	Username          *string   `json:"username,omitempty"`
	MetaJSON          *string   `json:"meta_json,omitempty"`
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
