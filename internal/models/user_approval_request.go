package models

import (
	"encoding/json"
	"time"
)

const (
	ApprovalActionCreate = "create"
	ApprovalActionDelete = "delete"

	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
)

// UserApprovalRequest — запрос юриста на создание или удаление пользователя.
// Хранит все необходимые данные для выполнения действия после одобрения админом.
type UserApprovalRequest struct {
	ID           int              `json:"id"`
	RequesterID  int              `json:"requester_id"`
	Action       string           `json:"action"` // "create" | "delete"
	TargetUserID *int             `json:"target_user_id,omitempty"`
	RequestData  *json.RawMessage `json:"request_data,omitempty"`
	Status       string           `json:"status"` // "pending" | "approved" | "rejected"
	ReviewerID   *int             `json:"reviewer_id,omitempty"`
	ReviewedAt   *time.Time       `json:"reviewed_at,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
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
