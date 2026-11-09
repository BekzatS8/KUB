package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"turcompany/internal/models"
)

// CalendarRepository — встречи календаря (ТЗ «доработки CRM КУБ», п.6).
type CalendarRepository struct {
	db *sql.DB
}

func NewCalendarRepository(db *sql.DB) *CalendarRepository {
	return &CalendarRepository{db: db}
}

const calendarSelect = `
	SELECT e.id, e.title, COALESCE(e.description, ''), e.starts_at, e.ends_at,
	       e.lead_id, COALESCE(l.title, ''), e.client_id, COALESCE(cl.name, ''),
	       e.owner_id, COALESCE(NULLIF(TRIM(CONCAT_WS(' ', ou.last_name, ou.first_name)), ''), ou.email, ''),
	       e.created_by, COALESCE(NULLIF(TRIM(CONCAT_WS(' ', cu.last_name, cu.first_name)), ''), cu.email, ''),
	       e.branch_id, COALESCE(b.name, ''), e.created_at, e.updated_at
	FROM calendar_events e
	LEFT JOIN leads l ON l.id = e.lead_id
	LEFT JOIN clients cl ON cl.id = e.client_id
	LEFT JOIN users ou ON ou.id = e.owner_id
	LEFT JOIN users cu ON cu.id = e.created_by
	LEFT JOIN branches b ON b.id = e.branch_id
`

func scanCalendarEvent(row interface{ Scan(...any) error }) (*models.CalendarEvent, error) {
	var e models.CalendarEvent
	var endsAt sql.NullTime
	var leadID, clientID, ownerID, createdBy, branchID sql.NullInt64
	if err := row.Scan(&e.ID, &e.Title, &e.Description, &e.StartsAt, &endsAt,
		&leadID, &e.LeadTitle, &clientID, &e.ClientName,
		&ownerID, &e.OwnerName, &createdBy, &e.CreatorName,
		&branchID, &e.BranchName, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	if endsAt.Valid {
		t := endsAt.Time
		e.EndsAt = &t
	}
	e.LeadID = nullIntPtr(leadID)
	e.ClientID = nullIntPtr(clientID)
	e.OwnerID = nullIntPtr(ownerID)
	e.CreatedBy = nullIntPtr(createdBy)
	e.BranchID = nullIntPtr(branchID)
	return &e, nil
}

func nullIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

func (r *CalendarRepository) List(ctx context.Context, f models.CalendarEventFilter) ([]*models.CalendarEvent, error) {
	where := []string{"e.deleted_at IS NULL", "e.starts_at >= $1", "e.starts_at < $2"}
	args := []any{f.From, f.To}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.OwnerID != nil {
		add("e.owner_id = $%d", *f.OwnerID)
	}
	if f.BranchID != nil {
		add("e.branch_id = $%d", *f.BranchID)
	}
	if f.LeadID != nil {
		add("e.lead_id = $%d", *f.LeadID)
	}
	if f.VisibleTo != nil {
		args = append(args, *f.VisibleTo)
		n := len(args)
		where = append(where, fmt.Sprintf(
			"(e.owner_id = $%d OR e.created_by = $%d OR e.lead_id IN (SELECT id FROM leads WHERE owner_id = $%d))", n, n, n))
	}
	rows, err := r.db.QueryContext(ctx, calendarSelect+" WHERE "+strings.Join(where, " AND ")+" ORDER BY e.starts_at, e.id LIMIT 2000", args...)
	if err != nil {
		return nil, fmt.Errorf("list calendar events: %w", err)
	}
	defer rows.Close()
	out := []*models.CalendarEvent{}
	for rows.Next() {
		e, err := scanCalendarEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *CalendarRepository) GetByID(ctx context.Context, id int64) (*models.CalendarEvent, error) {
	e, err := scanCalendarEvent(r.db.QueryRowContext(ctx, calendarSelect+" WHERE e.id = $1 AND e.deleted_at IS NULL", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

func (r *CalendarRepository) Create(ctx context.Context, e *models.CalendarEvent) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO calendar_events (title, description, starts_at, ends_at, lead_id, client_id, owner_id, created_by, branch_id)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, e.Title, e.Description, e.StartsAt, e.EndsAt, e.LeadID, e.ClientID, e.OwnerID, e.CreatedBy, e.BranchID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create calendar event: %w", err)
	}
	return id, nil
}

func (r *CalendarRepository) Update(ctx context.Context, e *models.CalendarEvent) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE calendar_events
		SET title = $2, description = NULLIF($3, ''), starts_at = $4, ends_at = $5,
		    lead_id = $6, client_id = $7, owner_id = $8, branch_id = $9, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, e.ID, e.Title, e.Description, e.StartsAt, e.EndsAt, e.LeadID, e.ClientID, e.OwnerID, e.BranchID)
	if err != nil {
		return fmt.Errorf("update calendar event: %w", err)
	}
	return nil
}

func (r *CalendarRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE calendar_events SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
	return err
}

// UserBranch — филиал сотрудника (встреча получает филиал ответственного).
func (r *CalendarRepository) UserBranch(ctx context.Context, userID int) (*int, error) {
	var b sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT branch_id FROM users WHERE id = $1`, userID).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return nullIntPtr(b), nil
}
