package repositories

import (
	"database/sql"
	"fmt"
	"turcompany/internal/models"
)

type SMSConfirmationRepository struct {
	DB *sql.DB
}

func NewSMSConfirmationRepository(db *sql.DB) *SMSConfirmationRepository {
	return &SMSConfirmationRepository{DB: db}
}

// Create — сохранить запись о коде для документа
func (r *SMSConfirmationRepository) Create(sms *models.SMSConfirmation) (int64, error) {
	const q = `
		INSERT INTO sms_confirmations (
			document_id,
			phone,
			code_hash,
			sent_at,
			expires_at,
			attempts,
			confirmed,
			confirmed_at,
			last_resend_at,
			resend_count
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`
	if err := r.DB.QueryRow(q,
		sms.DocumentID,
		sms.Phone,
		sms.CodeHash,
		sms.SentAt,
		sms.ExpiresAt,
		sms.Attempts,
		sms.Confirmed,
		sms.ConfirmedAt,
		sms.LastResendAt,
		sms.ResendCount,
	).Scan(&sms.ID); err != nil {
		return 0, fmt.Errorf("create sms confirmation: %w", err)
	}
	return sms.ID, nil
}

func (r *SMSConfirmationRepository) GetByID(id int64) (*models.SMSConfirmation, error) {
	const q = `
		SELECT id, document_id, phone, code_hash, sent_at, expires_at, attempts, confirmed, confirmed_at, last_resend_at, resend_count
		FROM sms_confirmations
		WHERE id = $1
	`
	row := r.DB.QueryRow(q, id)

	var sms models.SMSConfirmation
	var confirmedAt sql.NullTime
	var lastResendAt sql.NullTime
	if err := row.Scan(
		&sms.ID,
		&sms.DocumentID,
		&sms.Phone,
		&sms.CodeHash,
		&sms.SentAt,
		&sms.ExpiresAt,
		&sms.Attempts,
		&sms.Confirmed,
		&confirmedAt,
		&lastResendAt,
		&sms.ResendCount,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get sms confirmation: %w", err)
	}

	if confirmedAt.Valid {
		sms.ConfirmedAt = confirmedAt.Time
	}
	if lastResendAt.Valid {
		sms.LastResendAt = &lastResendAt.Time
	}
	return &sms, nil
}

func (r *SMSConfirmationRepository) GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error) {
	const q = `
		SELECT id, document_id, phone, code_hash, sent_at, expires_at, attempts, confirmed, confirmed_at, last_resend_at, resend_count
		FROM sms_confirmations
		WHERE document_id = $1
		ORDER BY sent_at DESC
		LIMIT 1
	`
	row := r.DB.QueryRow(q, documentID)

	var sms models.SMSConfirmation
	var confirmedAt sql.NullTime
	var lastResendAt sql.NullTime
	if err := row.Scan(
		&sms.ID,
		&sms.DocumentID,
		&sms.Phone,
		&sms.CodeHash,
		&sms.SentAt,
		&sms.ExpiresAt,
		&sms.Attempts,
		&sms.Confirmed,
		&confirmedAt,
		&lastResendAt,
		&sms.ResendCount,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest sms confirmation: %w", err)
	}

	if confirmedAt.Valid {
		sms.ConfirmedAt = confirmedAt.Time
	}
	if lastResendAt.Valid {
		sms.LastResendAt = &lastResendAt.Time
	}
	return &sms, nil
}

func (r *SMSConfirmationRepository) Update(sms *models.SMSConfirmation) error {
	const q = `
		UPDATE sms_confirmations
		SET document_id = $1,
			phone = $2,
			code_hash = $3,
			sent_at = $4,
			expires_at = $5,
			attempts = $6,
			confirmed = $7,
			confirmed_at = $8,
			last_resend_at = $9,
			resend_count = $10
		WHERE id = $11
	`
	if _, err := r.DB.Exec(q,
		sms.DocumentID,
		sms.Phone,
		sms.CodeHash,
		sms.SentAt,
		sms.ExpiresAt,
		sms.Attempts,
		sms.Confirmed,
		sms.ConfirmedAt,
		sms.LastResendAt,
		sms.ResendCount,
		sms.ID,
	); err != nil {
		return fmt.Errorf("update sms confirmation: %w", err)
	}
	return nil
}

func (r *SMSConfirmationRepository) Delete(id int64) error {
	if _, err := r.DB.Exec(`DELETE FROM sms_confirmations WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete sms confirmation: %w", err)
	}
	return nil
}

func (r *SMSConfirmationRepository) DeleteByDocumentID(documentID int64) error {
	if _, err := r.DB.Exec(`DELETE FROM sms_confirmations WHERE document_id = $1`, documentID); err != nil {
		return fmt.Errorf("delete sms by docID: %w", err)
	}
	return nil
}
