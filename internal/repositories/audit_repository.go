package repositories

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type AuditRepository struct {
	db *sql.DB
}

func NewAuditRepository(db *sql.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Insert(
	ctx context.Context,
	actorUserID *int,
	action, entityType, entityID string,
	ip, userAgent *string,
	metaJSON string,
	isHidden bool,
) error {
	var ipVal any
	if ip != nil && *ip != "" {
		ipVal = *ip
	} else {
		ipVal = nil
	}

	var uaVal any
	if userAgent != nil && *userAgent != "" {
		uaVal = *userAgent
	} else {
		uaVal = nil
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, ip, user_agent, meta, is_hidden)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, $6, COALESCE(NULLIF($7,''),'{}')::jsonb, $8)
	`, actorUserID, action, entityType, entityID, ipVal, uaVal, metaJSON, isHidden)
	if IsSQLState(err, SQLStateUndefinedTable) {
		return errors.Join(ErrAuditSchemaMissing, err)
	}
	return err
}

func (r *AuditRepository) AuditTableExists(ctx context.Context) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'audit_logs'
		)
	`).Scan(&exists)
	return exists, err
}

type AuditLogEntry struct {
	ID          int64
	ActorUserID *int
	Action      string
	EntityType  *string
	EntityID    *string
	IP          *string
	UserAgent   *string
	Meta        string
	IsHidden    bool
	CreatedAt   time.Time
}

type AuditListFilter struct {
	// когда true — скрытые записи включаются в выдачу
	IncludeHidden bool
	// когда задан — возвращает только записи данного актора
	ActorUserID *int
}

func (r *AuditRepository) List(ctx context.Context, limit, offset int, f AuditListFilter) ([]*AuditLogEntry, error) {
	q := `
		SELECT id, actor_user_id, action, entity_type, entity_id, ip, user_agent, meta, is_hidden, created_at
		FROM audit_logs
		WHERE ($1 OR is_hidden = FALSE)
		  AND ($2::int IS NULL OR actor_user_id = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := r.db.QueryContext(ctx, q, f.IncludeHidden, f.ActorUserID, limit, offset)
	if err != nil {
		if IsSQLState(err, SQLStateUndefinedTable) {
			return nil, errors.Join(ErrAuditSchemaMissing, err)
		}
		return nil, err
	}
	defer rows.Close()

	var out []*AuditLogEntry
	for rows.Next() {
		e := &AuditLogEntry{}
		if err := rows.Scan(
			&e.ID, &e.ActorUserID, &e.Action, &e.EntityType, &e.EntityID,
			&e.IP, &e.UserAgent, &e.Meta, &e.IsHidden, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
