package repositories

import (
	"context"
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

func (r *SignSessionRepository) Create(ctx context.Context, session *models.SignSession) error {
	const q = `
		INSERT INTO sign_sessions (
			document_id,
			phone_e164,
			code_hash,
			token_hash,
			signer_email,
			doc_hash,
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
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NOW(),NOW())
		RETURNING id, created_at, updated_at`
	var phoneVal any
	if session.PhoneE164 != "" {
		phoneVal = session.PhoneE164
	}
	var codeHashVal any
	if session.CodeHash != "" {
		codeHashVal = session.CodeHash
	}
	var signerEmailVal any
	if session.SignerEmail != "" {
		signerEmailVal = session.SignerEmail
	}
	var docHashVal any
	if session.DocHash != "" {
		docHashVal = session.DocHash
	}
	return r.DB.QueryRowContext(
		ctx,
		q,
		session.DocumentID,
		phoneVal,
		codeHashVal,
		session.TokenHash,
		signerEmailVal,
		docHashVal,
		session.ExpiresAt,
		session.Attempts,
		session.Status,
		session.VerifiedAt,
		session.SignedAt,
		session.SignedIP,
		session.SignedUserAgent,
	).Scan(&session.ID, &session.CreatedAt, &session.UpdatedAt)
}

func (r *SignSessionRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*models.SignSession, error) {
	const q = `
		SELECT id, document_id, phone_e164, code_hash, token_hash, signer_email, doc_hash, expires_at,
		       attempts, status, verified_at, signed_at, signed_ip, signed_user_agent,
		       created_at, updated_at
		FROM sign_sessions
		WHERE token_hash = $1`
	row := r.DB.QueryRowContext(ctx, q, tokenHash)

	var session models.SignSession
	var phoneE164 sql.NullString
	var codeHash sql.NullString
	var verifiedAt sql.NullTime
	var signedAt sql.NullTime
	var signedIP sql.NullString
	var signedUserAgent sql.NullString
	var signerEmail sql.NullString
	var docHash sql.NullString
	if err := row.Scan(
		&session.ID,
		&session.DocumentID,
		&phoneE164,
		&codeHash,
		&session.TokenHash,
		&signerEmail,
		&docHash,
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
	if phoneE164.Valid {
		session.PhoneE164 = phoneE164.String
	}
	if codeHash.Valid {
		session.CodeHash = codeHash.String
	}
	if signerEmail.Valid {
		session.SignerEmail = signerEmail.String
	}
	if docHash.Valid {
		session.DocHash = docHash.String
	}
	return &session, nil
}

func (r *SignSessionRepository) GetByID(ctx context.Context, id int64) (*models.SignSession, error) {
	const q = `
		SELECT id, document_id, phone_e164, code_hash, token_hash, signer_email, doc_hash, expires_at,
		       attempts, status, verified_at, signed_at, signed_ip, signed_user_agent,
		       created_at, updated_at
		FROM sign_sessions
		WHERE id = $1`
	row := r.DB.QueryRowContext(ctx, q, id)

	var session models.SignSession
	var phoneE164 sql.NullString
	var codeHash sql.NullString
	var verifiedAt sql.NullTime
	var signedAt sql.NullTime
	var signedIP sql.NullString
	var signedUserAgent sql.NullString
	var signerEmail sql.NullString
	var docHash sql.NullString
	if err := row.Scan(
		&session.ID,
		&session.DocumentID,
		&phoneE164,
		&codeHash,
		&session.TokenHash,
		&signerEmail,
		&docHash,
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
		return nil, fmt.Errorf("get sign session by id: %w", err)
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
	if phoneE164.Valid {
		session.PhoneE164 = phoneE164.String
	}
	if codeHash.Valid {
		session.CodeHash = codeHash.String
	}
	if signerEmail.Valid {
		session.SignerEmail = signerEmail.String
	}
	if docHash.Valid {
		session.DocHash = docHash.String
	}
	return &session, nil
}

func (r *SignSessionRepository) FindSignedByDocumentEmail(ctx context.Context, documentID int64, signerEmail string) (*models.SignSession, error) {
	const q = `
		SELECT id, document_id, phone_e164, code_hash, token_hash, signer_email, doc_hash, expires_at,
		       attempts, status, verified_at, signed_at, signed_ip, signed_user_agent,
		       created_at, updated_at
		FROM sign_sessions
		WHERE document_id = $1
		  AND signer_email = $2
		  AND status = 'signed'
		ORDER BY signed_at DESC
		LIMIT 1`
	row := r.DB.QueryRowContext(ctx, q, documentID, signerEmail)

	var session models.SignSession
	var phoneE164 sql.NullString
	var codeHash sql.NullString
	var verifiedAt sql.NullTime
	var signedAt sql.NullTime
	var signedIP sql.NullString
	var signedUserAgent sql.NullString
	var signerEmailVal sql.NullString
	var docHash sql.NullString
	if err := row.Scan(
		&session.ID,
		&session.DocumentID,
		&phoneE164,
		&codeHash,
		&session.TokenHash,
		&signerEmailVal,
		&docHash,
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
		return nil, fmt.Errorf("find signed sign session: %w", err)
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
	if phoneE164.Valid {
		session.PhoneE164 = phoneE164.String
	}
	if codeHash.Valid {
		session.CodeHash = codeHash.String
	}
	if signerEmailVal.Valid {
		session.SignerEmail = signerEmailVal.String
	}
	if docHash.Valid {
		session.DocHash = docHash.String
	}
	return &session, nil
}

func (r *SignSessionRepository) CountRecentByDocumentID(ctx context.Context, documentID int64, since time.Time) (int, error) {
	const q = `
		SELECT COUNT(*)
		FROM sign_sessions
		WHERE document_id = $1 AND created_at >= $2`
	var count int
	if err := r.DB.QueryRowContext(ctx, q, documentID, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("count sign sessions: %w", err)
	}
	return count, nil
}

func (r *SignSessionRepository) CountRecentByPhone(ctx context.Context, phoneE164 string, since time.Time) (int, error) {
	const q = `
		SELECT COUNT(*)
		FROM sign_sessions
		WHERE phone_e164 = $1 AND created_at >= $2`
	var count int
	if err := r.DB.QueryRowContext(ctx, q, phoneE164, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("count sign sessions by phone: %w", err)
	}
	return count, nil
}

func (r *SignSessionRepository) Update(ctx context.Context, session *models.SignSession) error {
	const q = `
		UPDATE sign_sessions
		SET phone_e164 = $1,
		    code_hash = $2,
		    token_hash = $3,
		    signer_email = $4,
		    doc_hash = $5,
		    expires_at = $6,
		    attempts = $7,
		    status = $8,
		    verified_at = $9,
		    signed_at = $10,
		    signed_ip = $11,
		    signed_user_agent = $12,
		    updated_at = NOW()
		WHERE id = $13`
	var phoneVal any
	if session.PhoneE164 != "" {
		phoneVal = session.PhoneE164
	}
	var codeHashVal any
	if session.CodeHash != "" {
		codeHashVal = session.CodeHash
	}
	var signerEmailVal any
	if session.SignerEmail != "" {
		signerEmailVal = session.SignerEmail
	}
	var docHashVal any
	if session.DocHash != "" {
		docHashVal = session.DocHash
	}
	if _, err := r.DB.ExecContext(
		ctx,
		q,
		phoneVal,
		codeHashVal,
		session.TokenHash,
		signerEmailVal,
		docHashVal,
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

func (r *SignSessionRepository) IncrementAttempts(ctx context.Context, id int64) (int, error) {
	const q = `
		UPDATE sign_sessions
		SET attempts = attempts + 1,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING attempts`
	var attempts int
	if err := r.DB.QueryRowContext(ctx, q, id).Scan(&attempts); err != nil {
		return 0, fmt.Errorf("increment sign session attempts: %w", err)
	}
	return attempts, nil
}
