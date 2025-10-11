package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"turcompany/internal/models"
)

// TaskRepository defines the interface for database operations on tasks.
type TaskRepository interface {
	Store(ctx context.Context, task *models.Task) error
	FindByID(ctx context.Context, id int64) (*models.Task, error)
	FindAll(ctx context.Context, filter models.TaskFilter) ([]models.Task, error)
	Update(ctx context.Context, task *models.Task) error
	Delete(ctx context.Context, id int64) error
}

type taskRepository struct {
	db *sql.DB
}

// NewTaskRepository creates a new instance of TaskRepository.
func NewTaskRepository(db *sql.DB) TaskRepository {
	return &taskRepository{db: db}
}

func (r *taskRepository) Store(ctx context.Context, task *models.Task) error {
	query := `
		INSERT INTO tasks (creator_id, assignee_id, entity_id, entity_type, title, description, due_date, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		task.CreatorID, task.AssigneeID, task.EntityID, task.EntityType,
		task.Title, task.Description, task.DueDate, task.Status,
		task.CreatedAt, task.UpdatedAt,
	).Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt)
}

func (r *taskRepository) FindByID(ctx context.Context, id int64) (*models.Task, error) {
	query := `SELECT id, creator_id, assignee_id, entity_id, entity_type, title, description, due_date, status, created_at, updated_at FROM tasks WHERE id = $1`

	task := &models.Task{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&task.ID, &task.CreatorID, &task.AssigneeID, &task.EntityID, &task.EntityType,
		&task.Title, &task.Description, &task.DueDate, &task.Status,
		&task.CreatedAt, &task.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found") // Or a custom error
		}
		return nil, err
	}
	return task, nil
}

func (r *taskRepository) FindAll(ctx context.Context, filter models.TaskFilter) ([]models.Task, error) {
	baseQuery := `SELECT id, creator_id, assignee_id, entity_id, entity_type, title, description, due_date, status, created_at, updated_at FROM tasks`

	conditions := []string{}
	args := []interface{}{}
	argID := 1

	if filter.AssigneeID != nil {
		conditions = append(conditions, fmt.Sprintf("assignee_id = $%d", argID))
		args = append(args, *filter.AssigneeID)
		argID++
	}
	if filter.CreatorID != nil {
		conditions = append(conditions, fmt.Sprintf("creator_id = $%d", argID))
		args = append(args, *filter.CreatorID)
		argID++
	}
	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argID))
		args = append(args, *filter.Status)
		argID++
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}
	baseQuery += " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []models.Task{}
	for rows.Next() {
		var task models.Task
		if err := rows.Scan(
			&task.ID, &task.CreatorID, &task.AssigneeID, &task.EntityID, &task.EntityType,
			&task.Title, &task.Description, &task.DueDate, &task.Status,
			&task.CreatedAt, &task.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (r *taskRepository) Update(ctx context.Context, task *models.Task) error {
	query := `
		UPDATE tasks SET
			assignee_id = $1, title = $2, description = $3, due_date = $4, status = $5, updated_at = $6
		WHERE id = $7`

	_, err := r.db.ExecContext(ctx, query,
		task.AssigneeID, task.Title, task.Description, task.DueDate, task.Status, task.UpdatedAt,
		task.ID,
	)
	return err
}

func (r *taskRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM tasks WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
