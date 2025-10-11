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

// Task represents the structure of a task in the system.
type Task struct {
	ID          int64      `json:"id"`
	CreatorID   int64      `json:"creator_id"`
	AssigneeID  int64      `json:"assignee_id"`
	EntityID    int64      `json:"entity_id"`
	EntityType  string     `json:"entity_type"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueDate     *time.Time `json:"due_date,omitempty"` // Use pointer to allow null
	Status      TaskStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TaskFilter defines the available parameters for filtering tasks.
type TaskFilter struct {
	AssigneeID *int64
	CreatorID  *int64
	EntityID   *int64
	EntityType *string
	Status     *TaskStatus
}
