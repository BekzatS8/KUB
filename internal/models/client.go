package models

import "time"

// Client represents a counterparty.
type Client struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	BinIin      string    `json:"bin_iin"`
	Address     string    `json:"address"`
	ContactInfo string    `json:"contact_info"`
	CreatedAt   time.Time `json:"created_at"`
}
