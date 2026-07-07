package models

import "time"

const (
	FunnelStageTypeRegular = "regular"
	FunnelStageTypeWon     = "won"
	FunnelStageTypeLost    = "lost"
)

type FunnelStage struct {
	ID          int       `json:"id"`
	FunnelID    int       `json:"funnel_id"`
	Name        string    `json:"name"`
	Code        string    `json:"code"`
	Color       string    `json:"color"`
	Type        string    `json:"type"`
	Position    int       `json:"position"`
	Probability int       `json:"probability"`
	Description string    `json:"description,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type DealStageHistory struct {
	ID            int          `json:"id"`
	DealID        int          `json:"deal_id"`
	FromStageID   *int         `json:"from_stage_id,omitempty"`
	ToStageID     *int         `json:"to_stage_id,omitempty"`
	ChangedBy     *int         `json:"changed_by,omitempty"`
	ChangedByName string       `json:"changed_by_name,omitempty"`
	Comment       string       `json:"comment,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	FromStage     *FunnelStage `json:"from_stage,omitempty"`
	ToStage       *FunnelStage `json:"to_stage,omitempty"`
}

const (
	FunnelBoardCardDeal = "deal"
	FunnelBoardCardLead = "lead"
)

// FunnelBoardDeal is a lightweight card projection used by the kanban board
// endpoint. Kind distinguishes deal cards from lead cards: inbound leads sit
// on the board (first stage = "Новая заявка") until they are converted to a
// deal (ТЗ 04.07.2026, п.1.1).
type FunnelBoardDeal struct {
	ID         int       `json:"id"`
	Kind       string    `json:"kind"`
	LeadID     int       `json:"lead_id"`
	FunnelID   *int      `json:"funnel_id,omitempty"`
	StageID    *int      `json:"stage_id,omitempty"`
	ClientID   int       `json:"client_id"`
	ClientType string    `json:"client_type"`
	ClientName string    `json:"client_name"`
	OwnerID    int       `json:"owner_id"`
	OwnerName  string    `json:"owner_name"`
	BranchID   *int      `json:"branch_id,omitempty"`
	Amount     float64   `json:"amount"`
	Currency   string    `json:"currency"`
	Status     string    `json:"status"`
	Phone      string    `json:"phone,omitempty"`
	Source     string    `json:"source,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// FunnelBoardColumn groups deals by stage for the kanban board response.
type FunnelBoardColumn struct {
	Stage       *FunnelStage       `json:"stage"`
	Deals       []*FunnelBoardDeal `json:"deals"`
	Count       int                `json:"count"`
	TotalAmount float64            `json:"total_amount"`
}

type FunnelBoard struct {
	Funnel  *Funnel              `json:"funnel"`
	Columns []*FunnelBoardColumn `json:"columns"`
}
