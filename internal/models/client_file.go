package models

import "time"

type ClientFile struct {
	ID         int64     `json:"id"`
	ClientID   int64     `json:"client_id"`
	Category   string    `json:"category"`
	FilePath   string    `json:"file_path"`
	Mime       *string   `json:"mime,omitempty"`
	SizeBytes  *int64    `json:"size_bytes,omitempty"`
	UploadedBy *int64    `json:"uploaded_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	IsPrimary  bool      `json:"is_primary"`
}
