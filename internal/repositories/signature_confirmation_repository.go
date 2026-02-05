package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"turcompany/internal/models"
)

type SignatureConfirmationRepository struct {
	DB *sql.DB
}

func NewSignatureConfirmationRepository(db *sql.DB) *SignatureConfirmationRepository {
	return &SignatureConfirmationRepository{DB: db}
}

func (r *SignatureConfirmationRepository) CreatePending(
	ctx context.Context,
	documentID int64,
	userID int64,
	channel string,
	otpHash *string,
	tokenHash *string,
	expiresAt time.Time,
	meta []byte,
) (*models.SignatureConfirmation, error) {
	const q = `
		INSERT INTO signature_confirmations (
			document_id,
			user_id,
			channel,
			status,
			otp_hash,
			token_hash,
			attempts,
			expires_at,
			meta
		)
		VALUES ($1,$2,$3,'pending',$4,$5,0,$6,$7)
		RETURNING id, document_id, user_id, channel, status, otp_hash, token_hash,
		          attempts, expires_at, approved_at, rejected_at, meta`

	var metaVal any
	if len(meta) > 0 {
		metaVal = meta
	} else {
		metaVal = nil
	}

	row := r.DB.QueryRowContext(
		ctx,
		q,
		documentID,
		userID,
		channel,
		otpHash,
		tokenHash,
		expiresAt,
		metaVal,
	)
	return scanSignatureConfirmation(row)
}

func (r *SignatureConfirmationRepository) FindActivePending(
	ctx context.Context,
	documentID int64,
	userID int64,
	channel string,
) (*models.SignatureConfirmation, error) {
	const q = `
		SELECT id, document_id, user_id, channel, status, otp_hash, token_hash,
		       attempts, expires_at, approved_at, rejected_at, meta
		FROM signature_confirmations
		WHERE document_id = $1
		  AND user_id = $2
		  AND channel = $3
		  AND status = 'pending'
		  AND expires_at > NOW()
		ORDER BY expires_at DESC
		LIMIT 1`
	row := r.DB.QueryRowContext(ctx, q, documentID, userID, channel)
	confirmation, err := scanSignatureConfirmation(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find active pending confirmation: %w", err)
	}
	return confirmation, nil
}

func (r *SignatureConfirmationRepository) FindPending(
	ctx context.Context,
	documentID int64,
	userID int64,
	channel string,
) (*models.SignatureConfirmation, error) {
	const q = `
		SELECT id, document_id, user_id, channel, status, otp_hash, token_hash,
		       attempts, expires_at, approved_at, rejected_at, meta
		FROM signature_confirmations
		WHERE document_id = $1
		  AND user_id = $2
		  AND channel = $3
		  AND status = 'pending'
		ORDER BY expires_at DESC
		LIMIT 1`
	row := r.DB.QueryRowContext(ctx, q, documentID, userID, channel)
	confirmation, err := scanSignatureConfirmation(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find pending confirmation: %w", err)
	}
	return confirmation, nil
}

func (r *SignatureConfirmationRepository) GetLatestByChannel(
	ctx context.Context,
	documentID int64,
	userID int64,
	channel string,
) (*models.SignatureConfirmation, error) {
	const q = `
		SELECT id, document_id, user_id, channel, status, otp_hash, token_hash,
		       attempts, expires_at, approved_at, rejected_at, meta
		FROM signature_confirmations
		WHERE document_id = $1
		  AND user_id = $2
		  AND channel = $3
		ORDER BY expires_at DESC
		LIMIT 1`
	row := r.DB.QueryRowContext(ctx, q, documentID, userID, channel)
	confirmation, err := scanSignatureConfirmation(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest confirmation by channel: %w", err)
	}
	return confirmation, nil
}

func (r *SignatureConfirmationRepository) FindPendingByTokenHash(
	ctx context.Context,
	channel string,
	tokenHash string,
) (*models.SignatureConfirmation, error) {
	const q = `
		SELECT id, document_id, user_id, channel, status, otp_hash, token_hash,
		       attempts, expires_at, approved_at, rejected_at, meta
		FROM signature_confirmations
		WHERE channel = $1
		  AND token_hash = $2
		  AND status = 'pending'
		  AND expires_at > NOW()
		LIMIT 1`
	row := r.DB.QueryRowContext(ctx, q, channel, tokenHash)
	confirmation, err := scanSignatureConfirmation(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find pending confirmation by token: %w", err)
	}
	return confirmation, nil
}

func (r *SignatureConfirmationRepository) HasApproved(
	ctx context.Context,
	documentID int64,
	userID int64,
	channel string,
) (bool, error) {
	const q = `
		SELECT EXISTS (
			SELECT 1
			FROM signature_confirmations
			WHERE document_id = $1
			  AND user_id = $2
			  AND channel = $3
			  AND status = 'approved'
		)`
	var exists bool
	if err := r.DB.QueryRowContext(ctx, q, documentID, userID, channel).Scan(&exists); err != nil {
		return false, fmt.Errorf("check approved confirmation: %w", err)
	}
	return exists, nil
}

func (r *SignatureConfirmationRepository) Approve(
	ctx context.Context,
	id string,
	metaUpdate []byte,
) (*models.SignatureConfirmation, error) {
	const q = `
		UPDATE signature_confirmations
		SET status = 'approved',
		    approved_at = NOW(),
		    meta = CASE
		        WHEN $2 IS NULL THEN meta
		        ELSE COALESCE(meta, '{}'::jsonb) || $2::jsonb
		    END
		WHERE id = $1
		RETURNING id, document_id, user_id, channel, status, otp_hash, token_hash,
		          attempts, expires_at, approved_at, rejected_at, meta`
	var metaVal any
	if len(metaUpdate) > 0 {
		metaVal = metaUpdate
	} else {
		metaVal = nil
	}
	row := r.DB.QueryRowContext(ctx, q, id, metaVal)
	confirmation, err := scanSignatureConfirmation(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("approve confirmation: %w", err)
	}
	return confirmation, nil
}

func (r *SignatureConfirmationRepository) Reject(
	ctx context.Context,
	id string,
	metaUpdate []byte,
) (*models.SignatureConfirmation, error) {
	const q = `
		UPDATE signature_confirmations
		SET status = 'rejected',
		    rejected_at = NOW(),
		    meta = CASE
		        WHEN $2 IS NULL THEN meta
		        ELSE COALESCE(meta, '{}'::jsonb) || $2::jsonb
		    END
		WHERE id = $1
		RETURNING id, document_id, user_id, channel, status, otp_hash, token_hash,
		          attempts, expires_at, approved_at, rejected_at, meta`
	var metaVal any
	if len(metaUpdate) > 0 {
		metaVal = metaUpdate
	} else {
		metaVal = nil
	}
	row := r.DB.QueryRowContext(ctx, q, id, metaVal)
	confirmation, err := scanSignatureConfirmation(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reject confirmation: %w", err)
	}
	return confirmation, nil
}

func (r *SignatureConfirmationRepository) Expire(ctx context.Context, id string) error {
	const q = `
		UPDATE signature_confirmations
		SET status = 'expired'
		WHERE id = $1`
	if _, err := r.DB.ExecContext(ctx, q, id); err != nil {
		return fmt.Errorf("expire confirmation: %w", err)
	}
	return nil
}

func (r *SignatureConfirmationRepository) CancelPrevious(
	ctx context.Context,
	documentID int64,
	userID int64,
) (int64, error) {
	const q = `
		UPDATE signature_confirmations
		SET status = 'cancelled'
		WHERE document_id = $1
		  AND user_id = $2
		  AND status = 'pending'`
	res, err := r.DB.ExecContext(ctx, q, documentID, userID)
	if err != nil {
		return 0, fmt.Errorf("cancel previous confirmations: %w", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (r *SignatureConfirmationRepository) IncrementAttempts(ctx context.Context, id string) (int, error) {
	const q = `
		UPDATE signature_confirmations
		SET attempts = attempts + 1
		WHERE id = $1
		RETURNING attempts`
	var attempts int
	if err := r.DB.QueryRowContext(ctx, q, id).Scan(&attempts); err != nil {
		return 0, fmt.Errorf("increment confirmation attempts: %w", err)
	}
	return attempts, nil
}

func scanSignatureConfirmation(row *sql.Row) (*models.SignatureConfirmation, error) {
	var confirmation models.SignatureConfirmation
	var otpHash sql.NullString
	var tokenHash sql.NullString
	var approvedAt sql.NullTime
	var rejectedAt sql.NullTime
	var metaBytes []byte
	if err := row.Scan(
		&confirmation.ID,
		&confirmation.DocumentID,
		&confirmation.UserID,
		&confirmation.Channel,
		&confirmation.Status,
		&otpHash,
		&tokenHash,
		&confirmation.Attempts,
		&confirmation.ExpiresAt,
		&approvedAt,
		&rejectedAt,
		&metaBytes,
	); err != nil {
		return nil, err
	}
	if otpHash.Valid {
		confirmation.OTPHash = &otpHash.String
	}
	if tokenHash.Valid {
		confirmation.TokenHash = &tokenHash.String
	}
	if approvedAt.Valid {
		confirmation.ApprovedAt = &approvedAt.Time
	}
	if rejectedAt.Valid {
		confirmation.RejectedAt = &rejectedAt.Time
	}
	if len(metaBytes) > 0 {
		confirmation.Meta = metaBytes
	}
	return &confirmation, nil
}
