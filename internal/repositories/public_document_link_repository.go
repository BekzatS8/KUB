package repositories

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrPublicLinkNotFound = errors.New("public document link not found")
	ErrPublicLinkExpired  = errors.New("public document link expired")
	ErrPublicLinkUsed     = errors.New("public document link already used")
)

type PublicDocumentLink struct {
	ID              int64
	DocumentID      int64
	TokenHash       string
	ExpiresAt       time.Time
	UsedAt          *time.Time
	CreatedByUserID *int64
	CreatedAt       time.Time
}

type PublicDocumentSignatureInsert struct {
	DocumentID  int64
	LinkID      int64
	SignerName  string
	SignerEmail string
	SignerPhone string
	Signature   string
	IP          string
	UserAgent   string
	EventID     string
	MetaJSON    string
}

type PublicDocumentLinkRepository struct {
	db *sql.DB
}

func NewPublicDocumentLinkRepository(db *sql.DB) *PublicDocumentLinkRepository {
	return &PublicDocumentLinkRepository{db: db}
}

func (r *PublicDocumentLinkRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

func (r *PublicDocumentLinkRepository) CreateLink(ctx context.Context, documentID int64, createdByUserID *int64, tokenHash string, expiresAt time.Time) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO public_document_links (document_id, token_hash, expires_at, created_by_user_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, documentID, tokenHash, expiresAt, createdByUserID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create public document link: %w", err)
	}
	return id, nil
}

func (r *PublicDocumentLinkRepository) FindActiveByTokenHash(ctx context.Context, tokenHash string) (*PublicDocumentLink, error) {
	link, err := r.findByTokenHash(ctx, nil, tokenHash, false)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, ErrPublicLinkNotFound
	}
	if link.UsedAt != nil {
		return nil, ErrPublicLinkUsed
	}
	if !link.ExpiresAt.After(time.Now()) {
		return nil, ErrPublicLinkExpired
	}
	if subtle.ConstantTimeCompare([]byte(link.TokenHash), []byte(tokenHash)) != 1 {
		return nil, ErrPublicLinkNotFound
	}
	return link, nil
}

func (r *PublicDocumentLinkRepository) FindActiveByTokenHashForUpdate(ctx context.Context, tx *sql.Tx, tokenHash string) (*PublicDocumentLink, error) {
	link, err := r.findByTokenHash(ctx, tx, tokenHash, true)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, ErrPublicLinkNotFound
	}
	if link.UsedAt != nil {
		return nil, ErrPublicLinkUsed
	}
	if !link.ExpiresAt.After(time.Now()) {
		return nil, ErrPublicLinkExpired
	}
	if subtle.ConstantTimeCompare([]byte(link.TokenHash), []byte(tokenHash)) != 1 {
		return nil, ErrPublicLinkNotFound
	}
	return link, nil
}

func (r *PublicDocumentLinkRepository) MarkUsed(ctx context.Context, linkID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE public_document_links SET used_at = NOW() WHERE id = $1`, linkID)
	if err != nil {
		return fmt.Errorf("mark public document link used: %w", err)
	}
	return nil
}

func (r *PublicDocumentLinkRepository) MarkUsedTx(ctx context.Context, tx *sql.Tx, linkID int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE public_document_links SET used_at = NOW() WHERE id = $1`, linkID)
	if err != nil {
		return fmt.Errorf("mark public document link used tx: %w", err)
	}
	return nil
}

func (r *PublicDocumentLinkRepository) InsertSignature(ctx context.Context, input PublicDocumentSignatureInsert) (time.Time, string, error) {
	var signedAt time.Time
	var eventID string
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO public_document_signatures (
			document_id, link_id, signer_name, signer_email, signer_phone, signature, ip, user_agent, event_id, meta
		)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,NULLIF($7,'')::inet,NULLIF($8,''),$9::uuid,COALESCE(NULLIF($10,''),'{}')::jsonb)
		RETURNING signed_at, event_id::text
	`, input.DocumentID, input.LinkID, input.SignerName, input.SignerEmail, input.SignerPhone, input.Signature, input.IP, input.UserAgent, input.EventID, input.MetaJSON).Scan(&signedAt, &eventID)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("insert public document signature: %w", err)
	}
	return signedAt, eventID, nil
}

func (r *PublicDocumentLinkRepository) InsertSignatureTx(ctx context.Context, tx *sql.Tx, input PublicDocumentSignatureInsert) (time.Time, string, error) {
	var signedAt time.Time
	var eventID string
	err := tx.QueryRowContext(ctx, `
		INSERT INTO public_document_signatures (
			document_id, link_id, signer_name, signer_email, signer_phone, signature, ip, user_agent, event_id, meta
		)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,NULLIF($7,'')::inet,NULLIF($8,''),$9::uuid,COALESCE(NULLIF($10,''),'{}')::jsonb)
		RETURNING signed_at, event_id::text
	`, input.DocumentID, input.LinkID, input.SignerName, input.SignerEmail, input.SignerPhone, input.Signature, input.IP, input.UserAgent, input.EventID, input.MetaJSON).Scan(&signedAt, &eventID)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("insert public document signature tx: %w", err)
	}
	return signedAt, eventID, nil
}

func (r *PublicDocumentLinkRepository) findByTokenHash(ctx context.Context, tx *sql.Tx, tokenHash string, forUpdate bool) (*PublicDocumentLink, error) {
	query := `
		SELECT id, document_id, token_hash, expires_at, used_at, created_by_user_id, created_at
		FROM public_document_links
		WHERE token_hash = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var link PublicDocumentLink
	var usedAt sql.NullTime
	var createdBy sql.NullInt64
	row := queryRow(ctx, r.db, tx, query, tokenHash)
	err := row.Scan(&link.ID, &link.DocumentID, &link.TokenHash, &link.ExpiresAt, &usedAt, &createdBy, &link.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find public document link: %w", err)
	}
	if usedAt.Valid {
		link.UsedAt = &usedAt.Time
	}
	if createdBy.Valid {
		v := createdBy.Int64
		link.CreatedByUserID = &v
	}
	return &link, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func queryRow(ctx context.Context, db *sql.DB, tx *sql.Tx, query string, args ...any) scanner {
	if tx != nil {
		return tx.QueryRowContext(ctx, query, args...)
	}
	return db.QueryRowContext(ctx, query, args...)
}
