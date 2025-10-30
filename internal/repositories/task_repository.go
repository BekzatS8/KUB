package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"turcompany/internal/models"
)

type TaskRepository interface {
	Store(ctx context.Context, task *models.Task) error
	FindByID(ctx context.Context, id int64) (*models.Task, error)
	FindAll(ctx context.Context, filter models.TaskFilter) ([]models.Task, error)
	Update(ctx context.Context, task *models.Task) error
	Delete(ctx context.Context, id int64) error

	// NEW:
	UpdateStatus(ctx context.Context, id int64, to models.TaskStatus) error
	UpdateAssignee(ctx context.Context, id int64, assigneeID int64) error
	ListDueForReminder(ctx context.Context, limit int) ([]models.Task, error)
	SetReminderFired(ctx context.Context, id int64) error
}

type taskRepository struct {
	db *sql.DB
}

func NewTaskRepository(db *sql.DB) TaskRepository {
	return &taskRepository{db: db}
}

func (r *taskRepository) Store(ctx context.Context, task *models.Task) error {
	query := `
		INSERT INTO tasks (
			creator_id, assignee_id, entity_id, entity_type, title, description,
			due_date, reminder_at, priority, status, created_at, updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		task.CreatorID, task.AssigneeID, task.EntityID, task.EntityType,
		task.Title, task.Description, task.DueDate, task.ReminderAt, task.Priority, task.Status,
		task.CreatedAt, task.UpdatedAt,
	).Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt)
}

func (r *taskRepository) FindByID(ctx context.Context, id int64) (*models.Task, error) {
	query := `SELECT id, creator_id, assignee_id, entity_id, entity_type, title, description,
       due_date, reminder_at, last_reminded_at, priority, status, created_at, updated_at
       FROM tasks WHERE id = $1`
	task := &models.Task{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&task.ID, &task.CreatorID, &task.AssigneeID, &task.EntityID, &task.EntityType,
		&task.Title, &task.Description, &task.DueDate, &task.ReminderAt, &task.LastRemindedAt,
		&task.Priority, &task.Status, &task.CreatedAt, &task.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found")
		}
		return nil, err
	}
	return task, nil
}

func (r *taskRepository) FindAll(ctx context.Context, filter models.TaskFilter) ([]models.Task, error) {
	baseQuery := `SELECT id, creator_id, assignee_id, entity_id, entity_type, title, description,
       due_date, reminder_at, last_reminded_at, priority, status, created_at, updated_at FROM tasks`

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

	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		if err := rows.Scan(
			&t.ID, &t.CreatorID, &t.AssigneeID, &t.EntityID, &t.EntityType,
			&t.Title, &t.Description, &t.DueDate, &t.ReminderAt, &t.LastRemindedAt,
			&t.Priority, &t.Status, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (r *taskRepository) Update(ctx context.Context, task *models.Task) error {
	query := `
		UPDATE tasks SET
			assignee_id=$1, title=$2, description=$3, due_date=$4,
			reminder_at=$5, priority=$6, status=$7, updated_at=$8
		WHERE id=$9`
	_, err := r.db.ExecContext(ctx, query,
		task.AssigneeID, task.Title, task.Description, task.DueDate,
		task.ReminderAt, task.Priority, task.Status, task.UpdatedAt, task.ID,
	)
	return err
}

func (r *taskRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	return err
}

func (r *taskRepository) UpdateStatus(ctx context.Context, id int64, to models.TaskStatus) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET status=$1, updated_at=NOW() WHERE id=$2`, to, id)
	return err
}

func (r *taskRepository) UpdateAssignee(ctx context.Context, id int64, assigneeID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET assignee_id=$1, updated_at=NOW() WHERE id=$2`, assigneeID, id)
	return err
}

func (r *taskRepository) ListDueForReminder(ctx context.Context, limit int) ([]models.Task, error) {
	q := `
SELECT id, creator_id, assignee_id, entity_id, entity_type, title, description,
       due_date, reminder_at, last_reminded_at, priority, status, created_at, updated_at
FROM tasks
WHERE reminder_at IS NOT NULL
  AND reminder_at <= NOW()
  AND (last_reminded_at IS NULL OR last_reminded_at < reminder_at)
  AND status NOT IN ('done','cancelled')
ORDER BY reminder_at ASC
LIMIT $1`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Task
	for rows.Next() {
		var t models.Task
		if err := rows.Scan(
			&t.ID, &t.CreatorID, &t.AssigneeID, &t.EntityID, &t.EntityType, &t.Title, &t.Description,
			&t.DueDate, &t.ReminderAt, &t.LastRemindedAt, &t.Priority, &t.Status, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *taskRepository) SetReminderFired(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET last_reminded_at = NOW(), updated_at=NOW() WHERE id=$1`, id)
	return err
}
