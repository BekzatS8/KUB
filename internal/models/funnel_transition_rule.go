package models

import "time"

// FunnelTransitionRule describes an automatic cross-funnel transition:
// when a deal arrives at FromStage inside FromFunnel, it is automatically
// moved to ToStage inside ToFunnel.
type FunnelTransitionRule struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	FromFunnelID int       `json:"from_funnel_id"`
	FromStageID  int       `json:"from_stage_id"`
	ToFunnelID   int       `json:"to_funnel_id"`
	ToStageID    int       `json:"to_stage_id"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Enriched for list/get responses — not persisted.
	FromFunnel *Funnel      `json:"from_funnel,omitempty"`
	FromStage  *FunnelStage `json:"from_stage,omitempty"`
	ToFunnel   *Funnel      `json:"to_funnel,omitempty"`
	ToStage    *FunnelStage `json:"to_stage,omitempty"`
}
