package models

import (
	"encoding/json"
	"time"
)

const (
	FeedEventTypePendingCreateLead   = "pending_create_lead"
	FeedEventTypePendingEditLead     = "pending_edit_lead"
	FeedEventTypePendingCreateDeal   = "pending_create_deal"
	FeedEventTypePendingEditDeal     = "pending_edit_deal"
	FeedEventTypePendingCreateClient = "pending_create_client"
	FeedEventTypePendingEditClient   = "pending_edit_client"

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
