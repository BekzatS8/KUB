// internal/models/task.go
package models

import "time"

// TaskStatus defines the possible statuses for a task.
type TaskStatus string

const (
	StatusNew        TaskStatus = "new"
	StatusInProgress TaskStatus = "in_progress"
	StatusDone       TaskStatus = "done"
	StatusCancelled  TaskStatus = "cancelled"
)

type TaskPriority string

const (
	PriorityLow    TaskPriority = "low"
	PriorityNormal TaskPriority = "normal"
	PriorityHigh   TaskPriority = "high"
	PriorityUrgent TaskPriority = "urgent"
)

// Task represents the structure of a task in the system.
type Task struct {
	ID             int64        `json:"id"`
	CreatorID      int64        `json:"creator_id"`
	AssigneeID     int64        `json:"assignee_id"`
	EntityID       int64        `json:"entity_id"`
	EntityType     string       `json:"entity_type"`
	Title          string       `json:"title"`
	Description    string       `json:"description"`
	DueDate        *time.Time   `json:"due_date,omitempty"`
	ReminderAt     *time.Time   `json:"reminder_at,omitempty"`
	LastRemindedAt *time.Time   `json:"last_reminded_at,omitempty"`
	Priority       TaskPriority `json:"priority"`
	Status         TaskStatus   `json:"status"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// TaskFilter defines the available parameters for filtering tasks.
type TaskFilter struct {
	AssigneeID *int64
	CreatorID  *int64
	EntityID   *int64
	EntityType *string
	Status     *TaskStatus
}
