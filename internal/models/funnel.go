package models

import "time"

type Funnel struct {
	ID           int         `json:"id"`
	Name         string      `json:"name"`
	Code         string      `json:"code"`
	DepartmentID int         `json:"department_id"`
	Department   *Department `json:"department,omitempty"`
	BranchID     *int        `json:"branch_id,omitempty"`
	Branch       *Branch     `json:"branch,omitempty"`
	IsActive     bool        `json:"is_active"`
	SortOrder    int         `json:"sort_order"`
	CreatedBy    *int        `json:"created_by,omitempty"`
	CreatedAt    time.Time   `json:"created_at,omitempty"`
	UpdatedAt    time.Time   `json:"updated_at,omitempty"`
}
