package models

import (
	"encoding/json"
	"time"
)

const (
	ApprovalActionCreate = "create"
	ApprovalActionDelete = "delete"
	ApprovalActionUpdate = "update"

	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
)

// UserApprovalRequest — запрос юриста на создание или удаление пользователя.
// Хранит все необходимые данные для выполнения действия после одобрения админом.
type UserApprovalRequest struct {
	ID              int              `json:"id"`
	RequesterID     int              `json:"requester_id"`
	RequesterName   string           `json:"requester_name,omitempty"`
	Action          string           `json:"action"` // "create" | "delete"
	TargetUserID    *int             `json:"target_user_id,omitempty"`
	TargetUserName  string           `json:"target_user_name,omitempty"`
	RequestData     *json.RawMessage `json:"request_data,omitempty"`
	Status          string           `json:"status"` // "pending" | "approved" | "rejected"
	ReviewerID      *int             `json:"reviewer_id,omitempty"`
	ReviewerName    string           `json:"reviewer_name,omitempty"`
	RejectReason    *string          `json:"reject_reason,omitempty"`
	ReviewedAt      *time.Time       `json:"reviewed_at,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
}

// UserApprovalUpdatePayload хранится в request_data для action=update.
type UserApprovalUpdatePayload struct {
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	MiddleName string `json:"middle_name,omitempty"`
	Phone      string `json:"phone,omitempty"`
	Address    string `json:"address,omitempty"`
	ExtraInfo  string `json:"extra_info,omitempty"`
	BinIin     string `json:"bin_iin,omitempty"`

	// Before содержит снимок исходных значений до запроса на редактирование.
	Before *UserApprovalFieldSnapshot `json:"before,omitempty"`
}

// UserApprovalFieldSnapshot хранит оригинальные значения полей пользователя до изменения.
type UserApprovalFieldSnapshot struct {
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	MiddleName string `json:"middle_name,omitempty"`
	Phone      string `json:"phone,omitempty"`
	Address    string `json:"address,omitempty"`
	ExtraInfo  string `json:"extra_info,omitempty"`
	BinIin     string `json:"bin_iin,omitempty"`
}

// UserApprovalCreatePayload хранится в request_data для action=create.
type UserApprovalCreatePayload struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	MiddleName   string `json:"middle_name,omitempty"`
	Position     string `json:"position"`
	Email        string `json:"email"`
	Phone        string `json:"phone,omitempty"`
	Address      string `json:"address,omitempty"`
	ExtraInfo    string `json:"extra_info,omitempty"`
	CompanyName  string `json:"company_name,omitempty"`
	BinIin       string `json:"bin_iin,omitempty"`
	RoleID       int    `json:"role_id"`
	BranchID     *int   `json:"branch_id,omitempty"`
	PasswordHash string `json:"password_hash"` // уже захеширован при создании запроса
}
