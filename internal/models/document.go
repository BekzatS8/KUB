package models

import "time"

type Document struct {
	ID           int64     `json:"id"`
	DealID       int64     `json:"deal_id"`
	DocType      string    `json:"doc_type"`
	Status       string    `json:"status"`
	FilePath     string    `json:"file_path"`      // старый PDF или upload
	FilePathDocx string    `json:"file_path_docx"` // DOCX путь
	FilePathPdf  string    `json:"file_path_pdf"`  // PDF путь
	CreatedAt    time.Time `json:"created_at"`
}
