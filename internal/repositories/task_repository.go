package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"turcompany/internal/models"

	"github.com/lib/pq"
)

type TaskRepository interface {
	Store(ctx context.Context, task *models.Task) error
	FindByID(ctx context.Context, id int64) (*models.Task, error)
	FindByIDWithArchiveScope(ctx context.Context, id int64, scope ArchiveScope) (*models.Task, error)
	FindAll(ctx context.Context, filter models.TaskFilter) ([]models.Task, error)
	FindAllPaginated(ctx context.Context, filter models.TaskFilter, limit, offset int) ([]models.Task, error)
	CountAll(ctx context.Context, filter models.TaskFilter) (int, error)
	Update(ctx context.Context, task *models.Task) error
	Delete(ctx context.Context, id int64) error
	Archive(ctx context.Context, id int64, archivedBy int64, reason string) error
	Unarchive(ctx context.Context, id int64) error

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

func taskArchiveWhere(scope ArchiveScope) string {
	switch scope {
	case ArchiveScopeArchivedOnly:
		return "is_archived = TRUE"
	case ArchiveScopeAll:
		return "1=1"
	default:
		return "is_archived = FALSE"
	}
}

func (r *taskRepository) Store(ctx context.Context, task *models.Task) error {
	task.AssigneeIDs = dedupeAssignees(task.AssigneeID, task.AssigneeIDs)
	if len(task.AssigneeIDs) > 0 {
		task.AssigneeID = task.AssigneeIDs[0]
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO tasks (
			creator_id, assignee_id, branch_id, entity_id, entity_type, title, description,
			due_date, reminder_at, priority, status, created_at, updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, created_at, updated_at`
	if err := tx.QueryRowContext(ctx, query,
		task.CreatorID, task.AssigneeID, task.BranchID, task.EntityID, task.EntityType,
		task.Title, task.Description, task.DueDate, task.ReminderAt, task.Priority, task.Status,
		task.CreatedAt, task.UpdatedAt,
	).Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt); err != nil {
		return err
	}
	if err := replaceTaskAssignees(ctx, tx, task.ID, task.AssigneeIDs); err != nil {
		return err
	}
	return tx.Commit()
}

// dedupeAssignees builds the ordered, de-duplicated assignee list with the
// primary assignee first, ignoring zero ids.
func dedupeAssignees(primary int64, ids []int64) []int64 {
	seen := map[int64]bool{}
	out := []int64{}
	add := func(id int64) {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	add(primary)
	for _, id := range ids {
		add(id)
	}
	return out
}

// replaceTaskAssignees rewrites the task_assignees rows for a task inside a tx.
func replaceTaskAssignees(ctx context.Context, tx *sql.Tx, taskID int64, ids []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_assignees WHERE task_id = $1`, taskID); err != nil {
		return err
	}
	for _, uid := range ids {
		if uid == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO task_assignees (task_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			taskID, uid,
		); err != nil {
			return err
		}
	}
	return nil
}

// loadAssignees fills AssigneeIDs for the given tasks from the join table,
// keeping the primary assignee first and falling back to the legacy
// assignee_id column when a task has no join rows yet.
func (r *taskRepository) loadAssignees(ctx context.Context, tasks []models.Task) error {
	if len(tasks) == 0 {
		return nil
	}
	ids := make([]int64, len(tasks))
	for i := range tasks {
		ids[i] = tasks[i].ID
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT task_id, user_id FROM task_assignees WHERE task_id = ANY($1) ORDER BY task_id, user_id`,
		pq.Array(ids),
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	byTask := map[int64][]int64{}
	for rows.Next() {
		var taskID, userID int64
		if err := rows.Scan(&taskID, &userID); err != nil {
			return err
		}
		byTask[taskID] = append(byTask[taskID], userID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range tasks {
		tasks[i].AssigneeIDs = dedupeAssignees(tasks[i].AssigneeID, byTask[tasks[i].ID])
	}
	return nil
}

func (r *taskRepository) FindByID(ctx context.Context, id int64) (*models.Task, error) {
	return r.FindByIDWithArchiveScope(ctx, id, ArchiveScopeActiveOnly)
}

func (r *taskRepository) FindByIDWithArchiveScope(ctx context.Context, id int64, scope ArchiveScope) (*models.Task, error) {
	query := `SELECT id, COALESCE(creator_id, 0), COALESCE(assignee_id, 0), branch_id, entity_id, entity_type, title, description,
       due_date, reminder_at, last_reminded_at, priority, status, created_at, updated_at, is_archived, archived_at, archived_by, COALESCE(archive_reason,'')
       FROM tasks WHERE id = $1 AND ` + taskArchiveWhere(scope)
	task := &models.Task{}
	var branchID sql.NullInt64
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&task.ID, &task.CreatorID, &task.AssigneeID, &branchID, &task.EntityID, &task.EntityType,
		&task.Title, &task.Description, &task.DueDate, &task.ReminderAt, &task.LastRemindedAt,
		&task.Priority, &task.Status, &task.CreatedAt, &task.UpdatedAt, &task.IsArchived, &task.ArchivedAt, &task.ArchivedBy, &task.ArchiveReason,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if branchID.Valid {
		v := branchID.Int64
		task.BranchID = &v
	}
	single := []models.Task{*task}
	if err := r.loadAssignees(ctx, single); err != nil {
		return nil, err
	}
	task.AssigneeIDs = single[0].AssigneeIDs
	return task, nil
}

func (r *taskRepository) FindAll(ctx context.Context, filter models.TaskFilter) ([]models.Task, error) {
	baseQuery := `SELECT id, COALESCE(creator_id, 0), COALESCE(assignee_id, 0), branch_id, entity_id, entity_type, title, description,
       due_date, reminder_at, last_reminded_at, priority, status, created_at, updated_at, is_archived, archived_at, archived_by, COALESCE(archive_reason,'') FROM tasks`
	whereClause, args := buildTaskFilterWhere(filter, 1)
	baseQuery += " WHERE " + whereClause
	sortExpr, sortOrder := taskSortExpression(filter.SortBy, filter.Order)
	baseQuery += fmt.Sprintf(" ORDER BY %s %s", sortExpr, sortOrder)

	rows, err := r.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		var branchID sql.NullInt64
		if err := rows.Scan(
			&t.ID, &t.CreatorID, &t.AssigneeID, &branchID, &t.EntityID, &t.EntityType,
			&t.Title, &t.Description, &t.DueDate, &t.ReminderAt, &t.LastRemindedAt,
			&t.Priority, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.IsArchived, &t.ArchivedAt, &t.ArchivedBy, &t.ArchiveReason,
		); err != nil {
			return nil, err
		}
		if branchID.Valid {
			v := branchID.Int64
			t.BranchID = &v
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.loadAssignees(ctx, tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *taskRepository) FindAllPaginated(ctx context.Context, filter models.TaskFilter, limit, offset int) ([]models.Task, error) {
	baseQuery := `SELECT id, COALESCE(creator_id, 0), COALESCE(assignee_id, 0), branch_id, entity_id, entity_type, title, description,
       due_date, reminder_at, last_reminded_at, priority, status, created_at, updated_at, is_archived, archived_at, archived_by, COALESCE(archive_reason,'') FROM tasks`
	whereClause, args := buildTaskFilterWhere(filter, 1)
	baseQuery += " WHERE " + whereClause
	sortExpr, sortOrder := taskSortExpression(filter.SortBy, filter.Order)
	args = append(args, limit, offset)
	baseQuery += fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d", sortExpr, sortOrder, len(args)-1, len(args))

	rows, err := r.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		var branchID sql.NullInt64
		if err := rows.Scan(
			&t.ID, &t.CreatorID, &t.AssigneeID, &branchID, &t.EntityID, &t.EntityType,
			&t.Title, &t.Description, &t.DueDate, &t.ReminderAt, &t.LastRemindedAt,
			&t.Priority, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.IsArchived, &t.ArchivedAt, &t.ArchivedBy, &t.ArchiveReason,
		); err != nil {
			return nil, err
		}
		if branchID.Valid {
			v := branchID.Int64
			t.BranchID = &v
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.loadAssignees(ctx, tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *taskRepository) CountAll(ctx context.Context, filter models.TaskFilter) (int, error) {
	whereClause, args := buildTaskFilterWhere(filter, 1)
	query := fmt.Sprintf("SELECT COUNT(1) FROM tasks WHERE %s", whereClause)
	var total int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func buildTaskFilterWhere(filter models.TaskFilter, startAt int) (string, []interface{}) {
	conditions := []string{}
	args := []interface{}{}
	argID := startAt

	if filter.AssigneeID != nil {
		conditions = append(conditions, fmt.Sprintf("EXISTS (SELECT 1 FROM task_assignees ta WHERE ta.task_id = tasks.id AND ta.user_id = $%d)", argID))
		args = append(args, *filter.AssigneeID)
		argID++
	}
	if filter.CreatorID != nil {
		conditions = append(conditions, fmt.Sprintf("creator_id = $%d", argID))
		args = append(args, *filter.CreatorID)
		argID++
	}
	if filter.BranchID != nil {
		conditions = append(conditions, fmt.Sprintf("branch_id = $%d", argID))
		args = append(args, *filter.BranchID)
		argID++
	}
	if filter.EntityID != nil {
		conditions = append(conditions, fmt.Sprintf("entity_id = $%d", argID))
		args = append(args, *filter.EntityID)
		argID++
	}
	if filter.EntityType != nil {
		conditions = append(conditions, fmt.Sprintf("entity_type = $%d", argID))
		args = append(args, *filter.EntityType)
		argID++
	}
	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argID))
		args = append(args, *filter.Status)
		argID++
	} else {
		statuses := taskStatusesFromGroup(filter.StatusGroup)
		if len(statuses) > 0 {
			conditions = append(conditions, fmt.Sprintf("status = ANY($%d)", argID))
			args = append(args, pq.Array(statuses))
			argID++
		}
	}
	if strings.TrimSpace(filter.Query) != "" {
		conditions = append(conditions, fmt.Sprintf("(LOWER(COALESCE(title,'')) LIKE $%d OR LOWER(COALESCE(description,'')) LIKE $%d)", argID, argID))
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(filter.Query))+"%")
		argID++
	}
	scope := ArchiveScopeActiveOnly
	switch strings.ToLower(strings.TrimSpace(filter.Archive)) {
	case "archived":
		scope = ArchiveScopeArchivedOnly
	case "all":
		scope = ArchiveScopeAll
	}
	conditions = append(conditions, taskArchiveWhere(scope))

	return strings.Join(conditions, " AND "), args
}

func taskStatusesFromGroup(group string) []string {
	switch strings.ToLower(strings.TrimSpace(group)) {
	case "active":
		return []string{string(models.StatusNew), string(models.StatusInProgress)}
	case "closed":
		return []string{string(models.StatusDone), string(models.StatusCancelled)}
	default:
		return nil
	}
}

func taskSortExpression(sortBy, order string) (string, string) {
	sortOrder := "DESC"
	if strings.EqualFold(order, "asc") {
		sortOrder = "ASC"
	}
	switch sortBy {
	case "due_date":
		return "due_date", sortOrder
	case "priority":
		return "priority", sortOrder
	case "status":
		return "status", sortOrder
	case "title":
		return "LOWER(COALESCE(title,''))", sortOrder
	default:
		return "created_at", sortOrder
	}
}

func (r *taskRepository) Update(ctx context.Context, task *models.Task) error {
	task.AssigneeIDs = dedupeAssignees(task.AssigneeID, task.AssigneeIDs)
	if len(task.AssigneeIDs) > 0 {
		task.AssigneeID = task.AssigneeIDs[0]
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		UPDATE tasks SET
			assignee_id=$1, branch_id=$2, title=$3, description=$4, due_date=$5,
			reminder_at=$6, priority=$7, status=$8, updated_at=$9
		WHERE id=$10`
	if _, err := tx.ExecContext(ctx, query,
		task.AssigneeID, task.BranchID, task.Title, task.Description, task.DueDate,
		task.ReminderAt, task.Priority, task.Status, task.UpdatedAt, task.ID,
	); err != nil {
		return err
	}
	if err := replaceTaskAssignees(ctx, tx, task.ID, task.AssigneeIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *taskRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	return err
}

func (r *taskRepository) Archive(ctx context.Context, id int64, archivedBy int64, reason string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET is_archived = TRUE,
		    archived_at = NOW(),
		    archived_by = $2,
		    archive_reason = $3,
		    updated_at = NOW()
		WHERE id = $1
	`, id, archivedBy, reason)
	return err
}

func (r *taskRepository) Unarchive(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET is_archived = FALSE,
		    archived_at = NULL,
		    archived_by = NULL,
		    archive_reason = NULL,
		    updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

func (r *taskRepository) UpdateStatus(ctx context.Context, id int64, to models.TaskStatus) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET status=$1, updated_at=NOW() WHERE id=$2`, to, id)
	return err
}

func (r *taskRepository) UpdateAssignee(ctx context.Context, id int64, assigneeID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE tasks SET assignee_id=$1, updated_at=NOW() WHERE id=$2`, assigneeID, id); err != nil {
		return err
	}
	if err := replaceTaskAssignees(ctx, tx, id, []int64{assigneeID}); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *taskRepository) ListDueForReminder(ctx context.Context, limit int) ([]models.Task, error) {
	q := `
SELECT id, COALESCE(creator_id, 0), COALESCE(assignee_id, 0), branch_id, entity_id, entity_type, title, description,
       due_date, reminder_at, last_reminded_at, priority, status, created_at, updated_at, is_archived, archived_at, archived_by, COALESCE(archive_reason,'')
FROM tasks
WHERE reminder_at IS NOT NULL
  AND is_archived = FALSE
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
		var branchID sql.NullInt64
		if err := rows.Scan(
			&t.ID, &t.CreatorID, &t.AssigneeID, &branchID, &t.EntityID, &t.EntityType, &t.Title, &t.Description,
			&t.DueDate, &t.ReminderAt, &t.LastRemindedAt, &t.Priority, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.IsArchived, &t.ArchivedAt, &t.ArchivedBy, &t.ArchiveReason,
		); err != nil {
			return nil, err
		}
		if branchID.Valid {
			v := branchID.Int64
			t.BranchID = &v
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.loadAssignees(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *taskRepository) SetReminderFired(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET last_reminded_at = NOW(), updated_at=NOW() WHERE id=$1`, id)
	return err
}
