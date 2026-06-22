package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"turcompany/internal/models"
)

type UserApprovalRepository struct {
	db *sql.DB
}

func NewUserApprovalRepository(db *sql.DB) *UserApprovalRepository {
	return &UserApprovalRepository{db: db}
}

func (r *UserApprovalRepository) Create(ctx context.Context, req *models.UserApprovalRequest) error {
	var dataJSON *[]byte
	if req.RequestData != nil {
		b := []byte(*req.RequestData)
		dataJSON = &b
	}
	return r.db.QueryRowContext(ctx, `
		INSERT INTO user_approval_requests (requester_id, action, target_user_id, request_data, status)
		VALUES ($1, $2, $3, $4, 'pending')
		RETURNING id, created_at`,
		req.RequesterID, req.Action, req.TargetUserID, dataJSON,
	).Scan(&req.ID, &req.CreatedAt)
}

func (r *UserApprovalRepository) GetByID(ctx context.Context, id int) (*models.UserApprovalRequest, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, requester_id, action, target_user_id, request_data, status,
		       reviewer_id, reviewed_at, created_at
		FROM user_approval_requests WHERE id = $1`, id)
	return scanApprovalRequest(row)
}

func (r *UserApprovalRepository) ListPending(ctx context.Context, limit, offset int) ([]*models.UserApprovalRequest, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, requester_id, action, target_user_id, request_data, status,
		       reviewer_id, reviewed_at, created_at
		FROM user_approval_requests
		WHERE status = 'pending'
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApprovalRows(rows)
}

func (r *UserApprovalRepository) ListAll(ctx context.Context, limit, offset int) ([]*models.UserApprovalRequest, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, requester_id, action, target_user_id, request_data, status,
		       reviewer_id, reviewed_at, created_at
		FROM user_approval_requests
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApprovalRows(rows)
}

func (r *UserApprovalRepository) UpdateStatus(ctx context.Context, id int, status string, reviewerID int) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE user_approval_requests
		SET status = $1, reviewer_id = $2, reviewed_at = $3
		WHERE id = $4 AND status = 'pending'`,
		status, reviewerID, now, id)
	return err
}

func scanApprovalRequest(row *sql.Row) (*models.UserApprovalRequest, error) {
	var r models.UserApprovalRequest
	var dataBytes []byte
	var reviewedAt sql.NullTime
	err := row.Scan(
		&r.ID, &r.RequesterID, &r.Action, &r.TargetUserID, &dataBytes,
		&r.Status, &r.ReviewerID, &reviewedAt, &r.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(dataBytes) > 0 {
		raw := json.RawMessage(dataBytes)
		r.RequestData = &raw
	}
	if reviewedAt.Valid {
		r.ReviewedAt = &reviewedAt.Time
	}
	return &r, nil
}

func scanApprovalRows(rows *sql.Rows) ([]*models.UserApprovalRequest, error) {
	var out []*models.UserApprovalRequest
	for rows.Next() {
		var r models.UserApprovalRequest
		var dataBytes []byte
		var reviewedAt sql.NullTime
		if err := rows.Scan(
			&r.ID, &r.RequesterID, &r.Action, &r.TargetUserID, &dataBytes,
			&r.Status, &r.ReviewerID, &reviewedAt, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		if len(dataBytes) > 0 {
			raw := json.RawMessage(dataBytes)
			r.RequestData = &raw
		}
		if reviewedAt.Valid {
			r.ReviewedAt = &reviewedAt.Time
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}
