package repositories

import (
	"context"
	"database/sql"
	"fmt"
)

// ChannelUserRole — роль сотрудника на номере Wazzup, настроенная вручную.
// Role: seller | manager | auditor (см. миграцию 085).
type ChannelUserRole struct {
	UserID             int
	Role               string
	AllowGetNewClients bool
}

// RoleCandidateDTO — активный сотрудник для окна «Доступ к чатам».
type RoleCandidateDTO struct {
	ID         int
	Name       string
	RoleID     int
	BranchName string
}

func (r *wazzupRepository) ListChannelRoles(ctx context.Context, channelID int64) ([]ChannelUserRole, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id, role, allow_get_new_clients
		FROM wazzup_channel_user_roles
		WHERE channel_id = $1
		ORDER BY user_id
	`, channelID)
	if err != nil {
		return nil, fmt.Errorf("list wazzup channel roles: %w", err)
	}
	defer rows.Close()
	out := []ChannelUserRole{}
	for rows.Next() {
		var role ChannelUserRole
		if err := rows.Scan(&role.UserID, &role.Role, &role.AllowGetNewClients); err != nil {
			return nil, fmt.Errorf("scan wazzup channel role: %w", err)
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

// ReplaceChannelRoles заменяет роли номера целиком и помечает, что доступ к
// нему настроен вручную.
func (r *wazzupRepository) ReplaceChannelRoles(ctx context.Context, channelID int64, roles []ChannelUserRole) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace wazzup channel roles: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `UPDATE wazzup_channels SET roles_configured = TRUE WHERE id = $1`, channelID)
	if err != nil {
		return fmt.Errorf("replace wazzup channel roles: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wazzup_channel_user_roles WHERE channel_id = $1`, channelID); err != nil {
		return fmt.Errorf("replace wazzup channel roles: %w", err)
	}
	for _, role := range roles {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO wazzup_channel_user_roles (channel_id, user_id, role, allow_get_new_clients)
			VALUES ($1, $2, $3, $4)
		`, channelID, role.UserID, role.Role, role.AllowGetNewClients); err != nil {
			return fmt.Errorf("replace wazzup channel roles: %w", err)
		}
	}
	return tx.Commit()
}

// ResetChannelRoles возвращает номер к автоматическим ролям.
func (r *wazzupRepository) ResetChannelRoles(ctx context.Context, channelID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("reset wazzup channel roles: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `UPDATE wazzup_channels SET roles_configured = FALSE WHERE id = $1`, channelID)
	if err != nil {
		return fmt.Errorf("reset wazzup channel roles: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wazzup_channel_user_roles WHERE channel_id = $1`, channelID); err != nil {
		return fmt.Errorf("reset wazzup channel roles: %w", err)
	}
	return tx.Commit()
}

// ListRoleCandidates — активные сотрудники с филиалом, по алфавиту. Филиал —
// подзапросом, а не JOIN: выражение имени (crmUserNameExpr) ссылается на
// колонки users без префикса и столкнулось бы с branches.name.
func (r *wazzupRepository) ListRoleCandidates(ctx context.Context) ([]RoleCandidateDTO, error) {
	nameExpr, err := r.crmUserNameExpr(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, %s AS display_name, COALESCE(role_id, 0),
		       COALESCE((SELECT b.name FROM branches b WHERE b.id = users.branch_id), '')
		FROM public.users
		WHERE COALESCE(is_active, TRUE) = TRUE
		ORDER BY 2, 1
	`, nameExpr))
	if err != nil {
		return nil, fmt.Errorf("list wazzup role candidates: %w", err)
	}
	defer rows.Close()
	out := []RoleCandidateDTO{}
	for rows.Next() {
		var c RoleCandidateDTO
		if err := rows.Scan(&c.ID, &c.Name, &c.RoleID, &c.BranchName); err != nil {
			return nil, fmt.Errorf("scan wazzup role candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
