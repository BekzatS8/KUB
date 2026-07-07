package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ManagerReport — личный отчёт сотрудника: редактируемая таблица внутри CRM
// (ТЗ 04.07.2026, п.3). Content хранит JSON вида {"columns": [...], "rows": [[...], ...]}.
type ManagerReport struct {
	ID        int             `json:"id"`
	UserID    int             `json:"user_id"`
	UserName  string          `json:"user_name,omitempty"`
	Content   json.RawMessage `json:"content"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type ManagerReportRepository struct {
	db *sql.DB
}

func NewManagerReportRepository(db *sql.DB) *ManagerReportRepository {
	return &ManagerReportRepository{db: db}
}

// GetByUser возвращает отчёт сотрудника; nil, если отчёта ещё нет.
func (r *ManagerReportRepository) GetByUser(ctx context.Context, userID int) (*ManagerReport, error) {
	const q = `
		SELECT mr.id, mr.user_id,
		       COALESCE(NULLIF(BTRIM(CONCAT_WS(' ', u.last_name, u.first_name)), ''), u.email) AS user_name,
		       mr.content, mr.updated_at
		FROM manager_reports mr
		JOIN users u ON u.id = mr.user_id
		WHERE mr.user_id = $1
	`
	rep := &ManagerReport{}
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&rep.ID, &rep.UserID, &rep.UserName, &rep.Content, &rep.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get manager report: %w", err)
	}
	return rep, nil
}

// Upsert сохраняет содержимое отчёта сотрудника.
func (r *ManagerReportRepository) Upsert(ctx context.Context, userID int, content json.RawMessage) error {
	const q = `
		INSERT INTO manager_reports (user_id, content, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id) DO UPDATE SET content = EXCLUDED.content, updated_at = NOW()
	`
	if _, err := r.db.ExecContext(ctx, q, userID, content); err != nil {
		return fmt.Errorf("upsert manager report: %w", err)
	}
	return nil
}

// List возвращает все отчёты (без содержимого) для руководителя/КК:
// кто ведёт отчёт и когда последний раз обновлял.
func (r *ManagerReportRepository) List(ctx context.Context) ([]*ManagerReport, error) {
	const q = `
		SELECT mr.id, mr.user_id,
		       COALESCE(NULLIF(BTRIM(CONCAT_WS(' ', u.last_name, u.first_name)), ''), u.email) AS user_name,
		       '{}'::jsonb, mr.updated_at
		FROM manager_reports mr
		JOIN users u ON u.id = mr.user_id
		WHERE COALESCE(u.is_active, TRUE) = TRUE
		ORDER BY mr.updated_at DESC
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list manager reports: %w", err)
	}
	defer rows.Close()

	out := []*ManagerReport{}
	for rows.Next() {
		rep := &ManagerReport{}
		if err := rows.Scan(&rep.ID, &rep.UserID, &rep.UserName, &rep.Content, &rep.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}
