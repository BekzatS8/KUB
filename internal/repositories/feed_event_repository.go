package repositories

import (
	"context"
	"database/sql"
	"time"

	"turcompany/internal/models"
)

type FeedEventRepository struct {
	db *sql.DB
}

func NewFeedEventRepository(db *sql.DB) *FeedEventRepository {
	return &FeedEventRepository{db: db}
}

func (r *FeedEventRepository) Create(ctx context.Context, e *models.FeedEvent) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO feed_events (event_type, status, requester_id, requester_name, payload, resource_id)
		VALUES ($1, 'pending', $2, $3, $4, $5)
		RETURNING id, created_at`,
		e.EventType, e.RequesterID, e.RequesterName, []byte(e.Payload), e.ResourceID,
	).Scan(&e.ID, &e.CreatedAt)
}

func (r *FeedEventRepository) GetByID(ctx context.Context, id int) (*models.FeedEvent, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT fe.id, fe.event_type, fe.status, fe.requester_id, fe.requester_name,
		       fe.payload, fe.resource_id, fe.reject_reason, fe.reviewer_id,
		       TRIM(COALESCE(ru.first_name,'') || ' ' || COALESCE(ru.last_name,'')),
		       fe.reviewed_at, fe.created_at
		FROM feed_events fe
		LEFT JOIN users ru ON ru.id = fe.reviewer_id
		WHERE fe.id = $1`, id)
	return scanFeedEvent(row)
}

func (r *FeedEventRepository) List(ctx context.Context, requesterID *int, status string, limit, offset int) ([]*models.FeedEvent, error) {
	query := `
		SELECT fe.id, fe.event_type, fe.status, fe.requester_id, fe.requester_name,
		       fe.payload, fe.resource_id, fe.reject_reason, fe.reviewer_id,
		       TRIM(COALESCE(ru.first_name,'') || ' ' || COALESCE(ru.last_name,'')),
		       fe.reviewed_at, fe.created_at
		FROM feed_events fe
		LEFT JOIN users ru ON ru.id = fe.reviewer_id
		WHERE ($1::INT IS NULL OR fe.requester_id = $1)
		  AND ($2 = '' OR fe.status = $2)
		ORDER BY fe.created_at DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.db.QueryContext(ctx, query, requesterID, status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeedEventRows(rows)
}

func (r *FeedEventRepository) UpdateStatus(ctx context.Context, id int, status string, reviewerID int, rejectReason *string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE feed_events
		SET status = $1, reviewer_id = $2, reviewed_at = $3, reject_reason = $4, updated_at = $3
		WHERE id = $5 AND status = 'pending'`,
		status, reviewerID, now, rejectReason, id)
	return err
}

func scanFeedEvent(row *sql.Row) (*models.FeedEvent, error) {
	var e models.FeedEvent
	var payloadBytes []byte
	var resourceID sql.NullInt64
	var rejectReason sql.NullString
	var reviewerID sql.NullInt64
	var adminName sql.NullString
	var reviewedAt sql.NullTime

	err := row.Scan(
		&e.ID, &e.EventType, &e.Status, &e.RequesterID, &e.RequesterName,
		&payloadBytes, &resourceID, &rejectReason, &reviewerID,
		&adminName, &reviewedAt, &e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	e.Payload = payloadBytes
	if resourceID.Valid {
		v := int(resourceID.Int64)
		e.ResourceID = &v
	}
	if rejectReason.Valid {
		e.RejectReason = &rejectReason.String
	}
	if reviewerID.Valid {
		v := int(reviewerID.Int64)
		e.ReviewerID = &v
	}
	if adminName.Valid {
		e.AdminName = &adminName.String
	}
	if reviewedAt.Valid {
		e.ReviewedAt = &reviewedAt.Time
	}
	return &e, nil
}

func scanFeedEventRows(rows *sql.Rows) ([]*models.FeedEvent, error) {
	var out []*models.FeedEvent
	for rows.Next() {
		var e models.FeedEvent
		var payloadBytes []byte
		var resourceID sql.NullInt64
		var rejectReason sql.NullString
		var reviewerID sql.NullInt64
		var adminName sql.NullString
		var reviewedAt sql.NullTime

		if err := rows.Scan(
			&e.ID, &e.EventType, &e.Status, &e.RequesterID, &e.RequesterName,
			&payloadBytes, &resourceID, &rejectReason, &reviewerID,
			&adminName, &reviewedAt, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		e.Payload = payloadBytes
		if resourceID.Valid {
			v := int(resourceID.Int64)
			e.ResourceID = &v
		}
		if rejectReason.Valid {
			e.RejectReason = &rejectReason.String
		}
		if reviewerID.Valid {
			v := int(reviewerID.Int64)
			e.ReviewerID = &v
		}
		if adminName.Valid {
			e.AdminName = &adminName.String
		}
		if reviewedAt.Valid {
			e.ReviewedAt = &reviewedAt.Time
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
