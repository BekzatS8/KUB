package models

import (
	"time"
)

type Deals struct {
	ID        int       `json:"id"`
	LeadID    int       `json:"lead_id"`
	Amount    string    `json:"amount"`
	Currency  string    `json:"currency"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
