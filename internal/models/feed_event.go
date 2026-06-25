package models

import (
	"encoding/json"
	"time"
)

const (
	FeedEventTypePendingCreateLead   = "pending_create_lead"
	FeedEventTypePendingEditLead     = "pending_edit_lead"
	FeedEventTypePendingDeleteLead   = "pending_delete_lead"
	FeedEventTypePendingCreateDeal   = "pending_create_deal"
	FeedEventTypePendingEditDeal     = "pending_edit_deal"
	FeedEventTypePendingDeleteDeal   = "pending_delete_deal"
	FeedEventTypePendingCreateClient = "pending_create_client"
	FeedEventTypePendingEditClient   = "pending_edit_client"
	FeedEventTypePendingDeleteClient = "pending_delete_client"
	// Документы ОКК/HR: создание/редактирование/удаление документа уходит на
	// одобрение администратора. На approve администратор применяет действие
	// своими правами (RoleSystemAdmin).
	FeedEventTypePendingCreateDocument = "pending_create_document"
	FeedEventTypePendingEditDocument   = "pending_edit_document"
	FeedEventTypePendingDeleteDocument = "pending_delete_document"

	FeedEventStatusPending  = "pending"
	FeedEventStatusApproved = "approved"
	FeedEventStatusRejected = "rejected"
)

type FeedEvent struct {
	ID            int              `json:"id"`
	EventType     string           `json:"type"`
	Status        string           `json:"status"`
	RequesterID   int              `json:"requestor_id"`
	RequesterName string           `json:"requestor_name"`
	Payload       json.RawMessage  `json:"payload"`
	ResourceID    *int             `json:"resource_id,omitempty"`
	RejectReason  *string          `json:"reject_reason,omitempty"`
	ReviewerID    *int             `json:"reviewer_id,omitempty"`
	AdminName     *string          `json:"admin_name,omitempty"`
	ReviewedAt    *time.Time       `json:"updated_at,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
}
