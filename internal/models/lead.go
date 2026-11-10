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
	// OwnerName и Unassigned заполняются только в карточке лида (GetByID).
	// Unassigned — у лида нет рабочего ответственного (никого, заблокированный
	// или неподтверждённый сотрудник, либо лид «припаркован» на админе): его
	// может взять себе любой сотрудник филиала.
	OwnerName     string     `json:"owner_name,omitempty"`
	Unassigned    *bool      `json:"unassigned,omitempty"`
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
	// «Краткая информация» (ТЗ «доработки CRM КУБ», п.3.1): заполняет оператор
	// колл-центра, МОП и руководство дополняют. Заполняется в карточке (GetByID).
	BriefFullName string `json:"brief_full_name,omitempty"`
	BriefCountry  string `json:"brief_country,omitempty"`
	BriefVisaType string `json:"brief_visa_type,omitempty"`
	BriefCity     string `json:"brief_city,omitempty"`
	BriefAge      string `json:"brief_age,omitempty"`
	BriefComment  string `json:"brief_comment,omitempty"`
	// Передан в отдел продаж оператором (п.3.1); для оператора лид после
	// передачи — только просмотр.
	HandedToSalesAt     *time.Time `json:"handed_to_sales_at,omitempty"`
	HandedToSalesBy     *int       `json:"handed_to_sales_by,omitempty"`
	HandedToSalesByName string     `json:"handed_to_sales_by_name,omitempty"`
	// Текущая воронка и этап (блок «Воронка», п.3.4) — только в карточке.
	FunnelName string `json:"funnel_name,omitempty"`
	StageName  string `json:"stage_name,omitempty"`
	// SharedBranchIDs — филиалы каналов мессенджера лида (основные и
	// дополнительные): лид виден и им. Только для проверки доступа.
	SharedBranchIDs []int `json:"-"`
	// ReadOnly — текущему пользователю лид доступен только на просмотр.
	ReadOnly bool `json:"read_only,omitempty"`
	IsArchived    bool       `json:"is_archived"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	ArchivedBy    *int       `json:"archived_by,omitempty"`
	ArchiveReason string     `json:"archive_reason,omitempty"`
}

// LeadBrief — поля «Краткой информации» лида.
type LeadBrief struct {
	FullName string `json:"brief_full_name"`
	Country  string `json:"brief_country"`
	VisaType string `json:"brief_visa_type"`
	City     string `json:"brief_city"`
	Age      string `json:"brief_age"`
	Comment  string `json:"brief_comment"`
}

// LeadHistoryEntry — запись истории лида: изменение поля, передача, возврат,
// объединение (ТЗ «доработки CRM КУБ», п.3).
type LeadHistoryEntry struct {
	ID        int64     `json:"id"`
	LeadID    int       `json:"lead_id"`
	UserID    *int      `json:"user_id,omitempty"`
	UserName  string    `json:"user_name,omitempty"`
	Action    string    `json:"action"`
	Field     string    `json:"field,omitempty"`
	OldValue  string    `json:"old_value,omitempty"`
	NewValue  string    `json:"new_value,omitempty"`
	Comment   string    `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
