package models

import "time"

type Document struct {
	ID           int64      `json:"id"`
	DealID       int64      `json:"deal_id"`
	DocType      string     `json:"doc_type"`
	Status       string     `json:"status"` // draft, under_review, approved, sent_for_signature, signed, cancelled
	FilePath     string     `json:"file_path"`
	FilePathDocx string     `json:"file_path_docx"`
	FilePathPdf  string     `json:"file_path_pdf"`
	CreatedAt    time.Time  `json:"created_at"`
	SignedAt     *time.Time `json:"signed_at,omitempty"`
	// Добавляем поля для юридической значимости
	SignMethod    string     `json:"sign_method,omitempty"`     // otp, manual, e-sign
	SignIP        string     `json:"sign_ip,omitempty"`         // IP адрес подписавшего
	SignUserAgent string     `json:"sign_user_agent,omitempty"` // User-Agent браузера
	SignMetadata  string     `json:"sign_metadata,omitempty"`   // JSON с метаданными подписи
	SignedBy      string     `json:"signed_by,omitempty"`
	IsArchived    bool       `json:"is_archived"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	ArchivedBy    *int       `json:"archived_by,omitempty"`
	ArchiveReason string     `json:"archive_reason,omitempty"`
}
