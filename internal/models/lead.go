package models

import (
	"time"
)

type Leads struct {
	ID            int        `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Phone         string     `json:"phone"`
	Source        string     `json:"source"`
	CreatedAt     time.Time  `json:"created_at"`
	OwnerID       int        `json:"owner_id"`
	BranchID      *int       `json:"branch_id,omitempty"`
	BranchName    string     `json:"branch_name,omitempty"`
	DepartmentID  *int       `json:"department_id,omitempty"`
	FunnelID      *int       `json:"funnel_id,omitempty"`
	StageID       *int       `json:"stage_id,omitempty"`
	Status        string     `json:"status"`
	// Мессенджер-переписка (Wazzup): для лидов из Telegram/Instagram переписка
	// открывается не по телефону, а по external_chat_id. Заполняется только в
	// карточке лида (GetByID), чтобы кнопка «Переписка» вела в нужный диалог.
	MessengerTransport string `json:"messenger_transport,omitempty"`
	MessengerChatID    string `json:"messenger_chat_id,omitempty"`
	IsArchived    bool       `json:"is_archived"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	ArchivedBy    *int       `json:"archived_by,omitempty"`
	ArchiveReason string     `json:"archive_reason,omitempty"`
}
