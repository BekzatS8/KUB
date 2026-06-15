package models

import "time"

// DocumentVersion represents a historical version of a document
type DocumentVersion struct {
	ID           int64      `json:"id"`
	DocumentID   int64      `json:"document_id"`
	Version      int        `json:"version"`
	FilePath     string     `json:"file_path"`
	FilePathPdf  string     `json:"file_path_pdf"`
	FilePathDocx string     `json:"file_path_docx"`
	FileSize     *int64     `json:"file_size,omitempty"`
	MimeType     *string    `json:"mime_type,omitempty"`
	UploadedBy   *int       `json:"uploaded_by,omitempty"`
	Comment      *string    `json:"comment,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}
