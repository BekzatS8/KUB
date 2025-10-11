package models

import "time"

type Document struct {
    ID       int64     `json:"id"`
    DealID   int64     `json:"deal_id"`
    DocType  string    `json:"doc_type"`
    FilePath string    `json:"file_path"`
    Status   string    `json:"status"`
    SignedAt time.Time `json:"signed_at"`
}
