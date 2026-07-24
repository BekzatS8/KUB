package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ManagerReport — именованный отчёт сотрудника: редактируемая таблица внутри CRM
// (ТЗ 04.07.2026, п.3). У сотрудника может быть несколько отчётов (миграция 070).
// Content хранит JSON вида {"columns": [...], "rows": [[...], ...]}.
type ManagerReport struct {
	ID        int             `json:"id"`
	UserID    int             `json:"user_id"`
	UserName  string          `json:"user_name,omitempty"`
	Title     string          `json:"title"`
	Content   json.RawMessage `json:"content,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ManagerReportOwner — строка списка «Отчёты сотрудников»: сотрудник, сколько у
// него отчётов и когда он последний раз что-то в них правил.
type ManagerReportOwner struct {
	UserID      int       `json:"user_id"`
	UserName    string    `json:"user_name"`
	ReportCount int       `json:"report_count"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ManagerReportRepository struct {
	db *sql.DB
}

func NewManagerReportRepository(db *sql.DB) *ManagerReportRepository {
	return &ManagerReportRepository{db: db}
}

// ListByUser возвращает отчёты сотрудника для списка/выбора. Content не
// выбираем вовсе: таблицы бывают на мегабайты, а списку нужны только имена.
func (r *ManagerReportRepository) ListByUser(ctx context.Context, userID int) ([]*ManagerReport, error) {
	const q = `
		SELECT mr.id, mr.user_id,
		       COALESCE(NULLIF(BTRIM(CONCAT_WS(' ', u.last_name, u.first_name)), ''), u.email) AS user_name,
		       mr.title, mr.created_at, mr.updated_at
		FROM manager_reports mr
		JOIN users u ON u.id = mr.user_id
		WHERE mr.user_id = $1 AND mr.deleted_at IS NULL
		ORDER BY mr.updated_at DESC, mr.id DESC
	`
	rows, err := r.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list manager reports by user: %w", err)
	}
	defer rows.Close()

	out := []*ManagerReport{}
	for rows.Next() {
		rep := &ManagerReport{}
		if err := rows.Scan(&rep.ID, &rep.UserID, &rep.UserName, &rep.Title, &rep.CreatedAt, &rep.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

// GetByID возвращает отчёт с содержимым; nil, если отчёта нет.
func (r *ManagerReportRepository) GetByID(ctx context.Context, id int) (*ManagerReport, error) {
	const q = `
		SELECT mr.id, mr.user_id,
		       COALESCE(NULLIF(BTRIM(CONCAT_WS(' ', u.last_name, u.first_name)), ''), u.email) AS user_name,
		       mr.title, mr.content, mr.created_at, mr.updated_at
		FROM manager_reports mr
		JOIN users u ON u.id = mr.user_id
		WHERE mr.id = $1 AND mr.deleted_at IS NULL
	`
	rep := &ManagerReport{}
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&rep.ID, &rep.UserID, &rep.UserName, &rep.Title, &rep.Content, &rep.CreatedAt, &rep.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get manager report: %w", err)
	}
	return rep, nil
}

// Create заводит новый отчёт сотрудника и возвращает его.
func (r *ManagerReportRepository) Create(ctx context.Context, userID int, title string, content json.RawMessage) (*ManagerReport, error) {
	const q = `
		INSERT INTO manager_reports (user_id, title, content, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id
	`
	var id int
	if err := r.db.QueryRowContext(ctx, q, userID, title, content).Scan(&id); err != nil {
		return nil, fmt.Errorf("create manager report: %w", err)
	}
	return r.GetByID(ctx, id)
}

// Update сохраняет содержимое и название отчёта. Возвращает false, если отчёт
// не найден или принадлежит другому сотруднику — владение проверяем прямо в
// UPDATE, чтобы нельзя было записать в чужой отчёт по его id.
func (r *ManagerReportRepository) Update(ctx context.Context, id, userID int, title string, content json.RawMessage) (bool, error) {
	const q = `
		UPDATE manager_reports
		SET title = $3, content = $4, updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`
	res, err := r.db.ExecContext(ctx, q, id, userID, title, content)
	if err != nil {
		return false, fmt.Errorf("update manager report: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// Delete мягко удаляет отчёт сотрудника в корзину. false — отчёта нет или он чужой.
func (r *ManagerReportRepository) Delete(ctx context.Context, id, userID int) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE manager_reports SET deleted_at = NOW(), deleted_by = $2 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		id, userID)
	if err != nil {
		return false, fmt.Errorf("delete manager report: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ListDeletedByUser возвращает корзину сотрудника (его мягко удалённые отчёты).
func (r *ManagerReportRepository) ListDeletedByUser(ctx context.Context, userID int) ([]*ManagerReport, error) {
	const q = `
		SELECT mr.id, mr.user_id,
		       COALESCE(NULLIF(BTRIM(CONCAT_WS(' ', u.last_name, u.first_name)), ''), u.email) AS user_name,
		       mr.title, mr.created_at, mr.updated_at
		FROM manager_reports mr
		JOIN users u ON u.id = mr.user_id
		WHERE mr.user_id = $1 AND mr.deleted_at IS NOT NULL
		ORDER BY mr.deleted_at DESC, mr.id DESC
	`
	rows, err := r.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list deleted manager reports by user: %w", err)
	}
	defer rows.Close()
	out := []*ManagerReport{}
	for rows.Next() {
		rep := &ManagerReport{}
		if err := rows.Scan(&rep.ID, &rep.UserID, &rep.UserName, &rep.Title, &rep.CreatedAt, &rep.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

// ListAllDeleted возвращает корзину для админа — все мягко удалённые отчёты.
func (r *ManagerReportRepository) ListAllDeleted(ctx context.Context) ([]*ManagerReport, error) {
	const q = `
		SELECT mr.id, mr.user_id,
		       COALESCE(NULLIF(BTRIM(CONCAT_WS(' ', u.last_name, u.first_name)), ''), u.email) AS user_name,
		       mr.title, mr.created_at, mr.updated_at
		FROM manager_reports mr
		JOIN users u ON u.id = mr.user_id
		WHERE mr.deleted_at IS NOT NULL
		ORDER BY mr.deleted_at DESC, mr.id DESC
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list all deleted manager reports: %w", err)
	}
	defer rows.Close()
	out := []*ManagerReport{}
	for rows.Next() {
		rep := &ManagerReport{}
		if err := rows.Scan(&rep.ID, &rep.UserID, &rep.UserName, &rep.Title, &rep.CreatedAt, &rep.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

// Restore возвращает отчёт сотрудника из корзины (проверка владельца в UPDATE).
func (r *ManagerReportRepository) Restore(ctx context.Context, id, userID int) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE manager_reports SET deleted_at = NULL, deleted_by = NULL WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL`,
		id, userID)
	if err != nil {
		return false, fmt.Errorf("restore manager report: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// RestoreByID возвращает любой отчёт из корзины — для админа.
func (r *ManagerReportRepository) RestoreByID(ctx context.Context, id int) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE manager_reports SET deleted_at = NULL, deleted_by = NULL WHERE id = $1 AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return false, fmt.Errorf("restore manager report by id: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// Purge окончательно удаляет отчёт сотрудника из корзины (проверка владельца).
func (r *ManagerReportRepository) Purge(ctx context.Context, id, userID int) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM manager_reports WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL`, id, userID)
	if err != nil {
		return false, fmt.Errorf("purge manager report: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// PurgeByID окончательно удаляет любой отчёт из корзины — для админа.
func (r *ManagerReportRepository) PurgeByID(ctx context.Context, id int) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM manager_reports WHERE id = $1 AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return false, fmt.Errorf("purge manager report by id: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// UpdateByID редактирует любой отчёт без проверки владельца — для админа.
func (r *ManagerReportRepository) UpdateByID(ctx context.Context, id int, title string, content json.RawMessage) (bool, error) {
	const q = `UPDATE manager_reports SET title = $2, content = $3, updated_at = NOW() WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id, title, content)
	if err != nil {
		return false, fmt.Errorf("update manager report by id: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// DeleteByID мягко удаляет любой отчёт в корзину без проверки владельца — для админа.
func (r *ManagerReportRepository) DeleteByID(ctx context.Context, id int, deletedBy int) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE manager_reports SET deleted_at = NOW(), deleted_by = NULLIF($2, 0) WHERE id = $1 AND deleted_at IS NULL`,
		id, deletedBy)
	if err != nil {
		return false, fmt.Errorf("delete manager report by id: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ListOwners возвращает сотрудников, у которых есть отчёты, — список для
// руководителя/КК: кто ведёт отчёты, сколько их и когда обновлялись.
func (r *ManagerReportRepository) ListOwners(ctx context.Context) ([]*ManagerReportOwner, error) {
	const q = `
		SELECT mr.user_id,
		       COALESCE(NULLIF(BTRIM(CONCAT_WS(' ', u.last_name, u.first_name)), ''), u.email) AS user_name,
		       COUNT(*) AS report_count,
		       MAX(mr.updated_at) AS updated_at
		FROM manager_reports mr
		JOIN users u ON u.id = mr.user_id
		WHERE COALESCE(u.is_active, TRUE) = TRUE AND mr.deleted_at IS NULL
		  -- отчёты системного администратора (role_id=50) приватны: видны только
		  -- ему самому в «Мои отчёты», в списке «Отчёты сотрудников» их нет
		  AND u.role_id <> 50
		GROUP BY mr.user_id, user_name
		ORDER BY updated_at DESC
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list manager report owners: %w", err)
	}
	defer rows.Close()

	out := []*ManagerReportOwner{}
	for rows.Next() {
		o := &ManagerReportOwner{}
		if err := rows.Scan(&o.UserID, &o.UserName, &o.ReportCount, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
