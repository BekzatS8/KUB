package models

import (
	"time"
)

type Deals struct {
	ID            int        `json:"id"`
	LeadID        int        `json:"lead_id"`
	ClientID      int        `json:"client_id"`
	ClientType    string     `json:"client_type"`
	// ClientName — имя клиента для таблицы сделок (в выпадашке фронта есть не
	// все клиенты); заполняется только в списках.
	ClientName    string     `json:"client_name,omitempty"`
	OwnerID       int        `json:"owner_id"`
	// OwnerName — ответственный менеджер (кто оформил клиента), для колонки
	// «Ответственный» в таблице сделок; заполняется только в списках.
	OwnerName     string     `json:"owner_name,omitempty"`
	BranchID      *int       `json:"branch_id,omitempty"`
	BranchName    string     `json:"branch_name,omitempty"`
	DepartmentID  *int       `json:"department_id,omitempty"`
	FunnelID      *int       `json:"funnel_id,omitempty"`
	StageID       *int       `json:"stage_id,omitempty"`
	Amount        float64    `json:"amount"`
	// Prepayment — первоначальный взнос; остаток = Amount - Prepayment
	// (ТЗ 04.07.2026, п.2.5)
	Prepayment float64 `json:"prepayment"`
	Currency   string  `json:"currency"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	ExtraJSON     string     `json:"extra_json" db:"extra_json"`
	IsArchived    bool       `json:"is_archived"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	ArchivedBy    *int       `json:"archived_by,omitempty"`
	ArchiveReason string     `json:"archive_reason,omitempty"`
}
