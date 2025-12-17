package repositories

import (
	"context"
	"database/sql"
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
		INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, ip, user_agent, meta)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, $6, COALESCE(NULLIF($7,''),'{}')::jsonb)
	`, actorUserID, action, entityType, entityID, ipVal, uaVal, metaJSON)

	return err
}
