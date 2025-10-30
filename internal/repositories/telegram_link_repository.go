package repositories

import (
	"context"
	"database/sql"
	"time"
)

type TelegramLink struct {
	ID        int
	UserID    int
	Code      string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

type TelegramLinkRepository interface {
	Create(ctx context.Context, userID int, code string, ttl time.Duration) (*TelegramLink, error)
	UseByCode(ctx context.Context, code string) (*TelegramLink, error)
}

type telegramLinkRepository struct{ db *sql.DB }

func NewTelegramLinkRepository(db *sql.DB) TelegramLinkRepository {
	return &telegramLinkRepository{db: db}
}

func (r *telegramLinkRepository) Create(ctx context.Context, userID int, code string, ttl time.Duration) (*TelegramLink, error) {
	expiresAt := time.Now().Add(ttl)

	row := r.db.QueryRowContext(ctx, `
		INSERT INTO telegram_links (user_id, code, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, code, expires_at, used, created_at
	`, userID, code, expiresAt)

	var l TelegramLink
	if err := row.Scan(&l.ID, &l.UserID, &l.Code, &l.ExpiresAt, &l.Used, &l.CreatedAt); err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *telegramLinkRepository) UseByCode(ctx context.Context, code string) (*TelegramLink, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var l TelegramLink
	err = tx.QueryRowContext(ctx, `
		SELECT id, user_id, code, expires_at, used, created_at
		FROM telegram_links
		WHERE code=$1
		FOR UPDATE
	`, code).Scan(&l.ID, &l.UserID, &l.Code, &l.ExpiresAt, &l.Used, &l.CreatedAt)
	if err != nil {
		return nil, err
	}

	if l.Used || time.Now().After(l.ExpiresAt) {
		return nil, sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `UPDATE telegram_links SET used=true WHERE id=$1`, l.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &l, nil
}
