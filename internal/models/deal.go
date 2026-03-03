package models

import (
	"time"
)

type Deals struct {
	ID        int       `json:"id"`
	LeadID    int       `json:"lead_id"`
	ClientID  int       `json:"client_id"`
	OwnerID   int       `json:"owner_id"`
	Amount    float64   `json:"amount"`
	Currency  string    `json:"currency"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ExtraJSON string    `json:"extra_json" db:"extra_json"`
}
