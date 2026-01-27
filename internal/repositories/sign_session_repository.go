package repositories

import (
	"database/sql"
	"fmt"
	"time"

	"turcompany/internal/models"
)

type SignSessionRepository struct {
	DB *sql.DB
}

func NewSignSessionRepository(db *sql.DB) *SignSessionRepository {
	return &SignSessionRepository{DB: db}
}

func (r *SignSessionRepository) Create(session *models.SignSession) error {
	const q = `
		INSERT INTO sign_sessions (
			document_id,
			phone_e164,
			code_hash,
			token_hash,
			expires_at,
			attempts,
			status,
			verified_at,
			signed_at,
			signed_ip,
			signed_user_agent,
			created_at,
			updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW(),NOW())
		RETURNING id, created_at, updated_at`
	return r.DB.QueryRow(
		q,
		session.DocumentID,
		session.PhoneE164,
		session.CodeHash,
		session.TokenHash,
		session.ExpiresAt,
		session.Attempts,
		session.Status,
		session.VerifiedAt,
		session.SignedAt,
		session.SignedIP,
		session.SignedUserAgent,
	).Scan(&session.ID, &session.CreatedAt, &session.UpdatedAt)
}

func (r *SignSessionRepository) GetByTokenHash(tokenHash string) (*models.SignSession, error) {
	const q = `
		SELECT id, document_id, phone_e164, code_hash, token_hash, expires_at,
		       attempts, status, verified_at, signed_at, signed_ip, signed_user_agent,
		       created_at, updated_at
		FROM sign_sessions
		WHERE token_hash = $1`
	row := r.DB.QueryRow(q, tokenHash)

	var session models.SignSession
	var verifiedAt sql.NullTime
	var signedAt sql.NullTime
	var signedIP sql.NullString
	var signedUserAgent sql.NullString
	if err := row.Scan(
		&session.ID,
		&session.DocumentID,
		&session.PhoneE164,
		&session.CodeHash,
		&session.TokenHash,
		&session.ExpiresAt,
		&session.Attempts,
		&session.Status,
		&verifiedAt,
		&signedAt,
		&signedIP,
		&signedUserAgent,
		&session.CreatedAt,
		&session.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get sign session: %w", err)
	}
	if verifiedAt.Valid {
		session.VerifiedAt = &verifiedAt.Time
	}
	if signedAt.Valid {
		session.SignedAt = &signedAt.Time
	}
	if signedIP.Valid {
		session.SignedIP = signedIP.String
	}
	if signedUserAgent.Valid {
		session.SignedUserAgent = signedUserAgent.String
	}
	return &session, nil
}

func (r *SignSessionRepository) CountRecentByDocumentID(documentID int64, since time.Time) (int, error) {
	const q = `
		SELECT COUNT(*)
		FROM sign_sessions
		WHERE document_id = $1 AND created_at >= $2`
	var count int
	if err := r.DB.QueryRow(q, documentID, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("count sign sessions: %w", err)
	}
	return count, nil
}

func (r *SignSessionRepository) CountRecentByPhone(phoneE164 string, since time.Time) (int, error) {
	const q = `
		SELECT COUNT(*)
		FROM sign_sessions
		WHERE phone_e164 = $1 AND created_at >= $2`
	var count int
	if err := r.DB.QueryRow(q, phoneE164, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("count sign sessions by phone: %w", err)
	}
	return count, nil
}

func (r *SignSessionRepository) Update(session *models.SignSession) error {
	const q = `
		UPDATE sign_sessions
		SET phone_e164 = $1,
		    code_hash = $2,
		    token_hash = $3,
		    expires_at = $4,
		    attempts = $5,
		    status = $6,
		    verified_at = $7,
		    signed_at = $8,
		    signed_ip = $9,
		    signed_user_agent = $10,
		    updated_at = NOW()
		WHERE id = $11`
	if _, err := r.DB.Exec(
		q,
		session.PhoneE164,
		session.CodeHash,
		session.TokenHash,
		session.ExpiresAt,
		session.Attempts,
		session.Status,
		session.VerifiedAt,
		session.SignedAt,
		session.SignedIP,
		session.SignedUserAgent,
		session.ID,
	); err != nil {
		return fmt.Errorf("update sign session: %w", err)
	}
	return nil
}
