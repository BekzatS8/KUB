package models

import "time"

// CalendarEvent — встреча с клиентом в календаре (ТЗ «доработки CRM КУБ», п.6).
type CalendarEvent struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	LeadID      *int       `json:"lead_id,omitempty"`
	LeadTitle   string     `json:"lead_title,omitempty"`
	ClientID    *int       `json:"client_id,omitempty"`
	ClientName  string     `json:"client_name,omitempty"`
	OwnerID     *int       `json:"owner_id,omitempty"`
	OwnerName   string     `json:"owner_name,omitempty"`
	CreatedBy   *int       `json:"created_by,omitempty"`
	CreatorName string     `json:"creator_name,omitempty"`
	BranchID    *int       `json:"branch_id,omitempty"`
	BranchName  string     `json:"branch_name,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// CanEdit — текущий пользователь может изменить/удалить встречу.
	CanEdit bool `json:"can_edit"`
}

// CalendarEventFilter — выборка встреч за период.
type CalendarEventFilter struct {
	From     time.Time
	To       time.Time
	OwnerID  *int
	BranchID *int
	LeadID   *int
	// VisibleTo — рядовой сотрудник: свои встречи (ответственный или
	// назначил сам) и встречи по его лидам.
	VisibleTo *int
}
