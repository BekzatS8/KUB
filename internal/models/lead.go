package models

import (
	"time"
)

type Leads struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Phone       string    `json:"phone"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"created_at"`
	OwnerID     int       `json:"owner_id"`
	Status      string    `json:"status"`
}
