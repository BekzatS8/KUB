package models

import (
	"time"
)

type Leads struct {
	ID            int        `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Phone         string     `json:"phone"`
	Source        string     `json:"source"`
	CreatedAt     time.Time  `json:"created_at"`
	OwnerID       int        `json:"owner_id"`
	CompanyID     int        `json:"company_id"`
	Status        string     `json:"status"`
	IsArchived    bool       `json:"is_archived"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	ArchivedBy    *int       `json:"archived_by,omitempty"`
	ArchiveReason string     `json:"archive_reason,omitempty"`
}
