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
		SELECT r.id, r.requester_id,
		       COALESCE(NULLIF(TRIM(COALESCE(u.last_name,'') || ' ' || COALESCE(u.first_name,'') || ' ' || COALESCE(u.middle_name,'')), ''), COALESCE(u.email,''), '') AS requester_name,
		       r.action, r.target_user_id,
		       COALESCE(NULLIF(TRIM(COALESCE(tu.last_name,'') || ' ' || COALESCE(tu.first_name,'') || ' ' || COALESCE(tu.middle_name,'')), ''), COALESCE(tu.email,''), '') AS target_user_name,
		       r.request_data, r.status,
		       r.reviewer_id,
		       COALESCE(NULLIF(TRIM(COALESCE(rv.last_name,'') || ' ' || COALESCE(rv.first_name,'') || ' ' || COALESCE(rv.middle_name,'')), ''), COALESCE(rv.email,''), '') AS reviewer_name,
		       r.reject_reason, r.reviewed_at, r.created_at
		FROM user_approval_requests r
		LEFT JOIN users u  ON u.id = r.requester_id
		LEFT JOIN users tu ON tu.id = r.target_user_id
		LEFT JOIN users rv ON rv.id = r.reviewer_id
		WHERE r.id = $1`, id)
	return scanApprovalRow(row.Scan)
}

func (r *UserApprovalRepository) ListPending(ctx context.Context, limit, offset int) ([]*models.UserApprovalRequest, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.requester_id,
		       COALESCE(NULLIF(TRIM(COALESCE(u.last_name,'') || ' ' || COALESCE(u.first_name,'') || ' ' || COALESCE(u.middle_name,'')), ''), COALESCE(u.email,''), '') AS requester_name,
		       r.action, r.target_user_id,
		       COALESCE(NULLIF(TRIM(COALESCE(tu.last_name,'') || ' ' || COALESCE(tu.first_name,'') || ' ' || COALESCE(tu.middle_name,'')), ''), COALESCE(tu.email,''), '') AS target_user_name,
		       r.request_data, r.status,
		       r.reviewer_id,
		       COALESCE(NULLIF(TRIM(COALESCE(rv.last_name,'') || ' ' || COALESCE(rv.first_name,'') || ' ' || COALESCE(rv.middle_name,'')), ''), COALESCE(rv.email,''), '') AS reviewer_name,
		       r.reject_reason, r.reviewed_at, r.created_at
		FROM user_approval_requests r
		LEFT JOIN users u  ON u.id = r.requester_id
		LEFT JOIN users tu ON tu.id = r.target_user_id
		LEFT JOIN users rv ON rv.id = r.reviewer_id
		WHERE r.status = 'pending'
		ORDER BY r.created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApprovalRows(rows)
}

func (r *UserApprovalRepository) ListAll(ctx context.Context, limit, offset int) ([]*models.UserApprovalRequest, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.requester_id,
		       COALESCE(NULLIF(TRIM(COALESCE(u.last_name,'') || ' ' || COALESCE(u.first_name,'') || ' ' || COALESCE(u.middle_name,'')), ''), COALESCE(u.email,''), '') AS requester_name,
		       r.action, r.target_user_id,
		       COALESCE(NULLIF(TRIM(COALESCE(tu.last_name,'') || ' ' || COALESCE(tu.first_name,'') || ' ' || COALESCE(tu.middle_name,'')), ''), COALESCE(tu.email,''), '') AS target_user_name,
		       r.request_data, r.status,
		       r.reviewer_id,
		       COALESCE(NULLIF(TRIM(COALESCE(rv.last_name,'') || ' ' || COALESCE(rv.first_name,'') || ' ' || COALESCE(rv.middle_name,'')), ''), COALESCE(rv.email,''), '') AS reviewer_name,
		       r.reject_reason, r.reviewed_at, r.created_at
		FROM user_approval_requests r
		LEFT JOIN users u  ON u.id = r.requester_id
		LEFT JOIN users tu ON tu.id = r.target_user_id
		LEFT JOIN users rv ON rv.id = r.reviewer_id
		ORDER BY r.created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApprovalRows(rows)
}

func (r *UserApprovalRepository) ListByRequester(ctx context.Context, requesterID, limit, offset int) ([]*models.UserApprovalRequest, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.requester_id,
		       COALESCE(NULLIF(TRIM(COALESCE(u.last_name,'') || ' ' || COALESCE(u.first_name,'') || ' ' || COALESCE(u.middle_name,'')), ''), COALESCE(u.email,''), '') AS requester_name,
		       r.action, r.target_user_id,
		       COALESCE(NULLIF(TRIM(COALESCE(tu.last_name,'') || ' ' || COALESCE(tu.first_name,'') || ' ' || COALESCE(tu.middle_name,'')), ''), COALESCE(tu.email,''), '') AS target_user_name,
		       r.request_data, r.status,
		       r.reviewer_id,
		       COALESCE(NULLIF(TRIM(COALESCE(rv.last_name,'') || ' ' || COALESCE(rv.first_name,'') || ' ' || COALESCE(rv.middle_name,'')), ''), COALESCE(rv.email,''), '') AS reviewer_name,
		       r.reject_reason, r.reviewed_at, r.created_at
		FROM user_approval_requests r
		LEFT JOIN users u  ON u.id = r.requester_id
		LEFT JOIN users tu ON tu.id = r.target_user_id
		LEFT JOIN users rv ON rv.id = r.reviewer_id
		WHERE r.requester_id = $1
		ORDER BY r.created_at DESC
		LIMIT $2 OFFSET $3`, requesterID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApprovalRows(rows)
}

func (r *UserApprovalRepository) UpdateStatus(ctx context.Context, id int, status string, reviewerID int, rejectReason *string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE user_approval_requests
		SET status = $1, reviewer_id = $2, reviewed_at = $3, reject_reason = $4
		WHERE id = $5 AND status = 'pending'`,
		status, reviewerID, now, rejectReason, id)
	return err
}

type scanFunc func(dest ...any) error

func scanApprovalRow(scan scanFunc) (*models.UserApprovalRequest, error) {
	var r models.UserApprovalRequest
	var dataBytes []byte
	var reviewedAt sql.NullTime
	err := scan(
		&r.ID, &r.RequesterID, &r.RequesterName,
		&r.Action, &r.TargetUserID, &r.TargetUserName,
		&dataBytes, &r.Status,
		&r.ReviewerID, &r.ReviewerName, &r.RejectReason, &reviewedAt, &r.CreatedAt,
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
			&r.ID, &r.RequesterID, &r.RequesterName,
			&r.Action, &r.TargetUserID, &r.TargetUserName,
			&dataBytes, &r.Status,
			&r.ReviewerID, &r.ReviewerName, &r.RejectReason, &reviewedAt, &r.CreatedAt,
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
