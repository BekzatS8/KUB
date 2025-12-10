package repositories

import (
	"context"
	"database/sql"
	"time"
)

type TelegramLink struct {
	ID        int
	UserID    int
	ChatID    int64
	Code      string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

type TelegramLinkRepository interface {
	CreateLink(ctx context.Context, userID int, chatID int64, code string, expiresAt time.Time) (*TelegramLink, error)
	GetByCode(ctx context.Context, code string) (*TelegramLink, error)
	ConfirmLink(ctx context.Context, code string, userID int) (int64, error)
}

type telegramLinkRepository struct{ db *sql.DB }

func NewTelegramLinkRepository(db *sql.DB) TelegramLinkRepository {
	return &telegramLinkRepository{db: db}
}

func (r *telegramLinkRepository) CreateLink(ctx context.Context, userID int, chatID int64, code string, expiresAt time.Time) (*TelegramLink, error) {
	row := r.db.QueryRowContext(ctx, `
                INSERT INTO telegram_links (user_id, telegram_chat_id, code, expires_at)
                VALUES ($1, $2, $3, $4)
                RETURNING id, user_id, telegram_chat_id, code, expires_at, used, created_at
        `, userID, chatID, code, expiresAt)

	var l TelegramLink
	if err := row.Scan(&l.ID, &l.UserID, &l.ChatID, &l.Code, &l.ExpiresAt, &l.Used, &l.CreatedAt); err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *telegramLinkRepository) GetByCode(ctx context.Context, code string) (*TelegramLink, error) {
	row := r.db.QueryRowContext(ctx, `
                SELECT id, user_id, telegram_chat_id, code, expires_at, used, created_at
                FROM telegram_links
                WHERE code=$1
        `, code)

	var l TelegramLink
	if err := row.Scan(&l.ID, &l.UserID, &l.ChatID, &l.Code, &l.ExpiresAt, &l.Used, &l.CreatedAt); err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *telegramLinkRepository) ConfirmLink(ctx context.Context, code string, userID int) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var l TelegramLink
	err = tx.QueryRowContext(ctx, `
                SELECT id, user_id, telegram_chat_id, code, expires_at, used, created_at
                FROM telegram_links
                WHERE code=$1
                FOR UPDATE
        `, code).Scan(&l.ID, &l.UserID, &l.ChatID, &l.Code, &l.ExpiresAt, &l.Used, &l.CreatedAt)
	if err != nil {
		return 0, err
	}

	if l.Used || time.Now().After(l.ExpiresAt) {
		return 0, sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `UPDATE telegram_links SET used=true, user_id=$1 WHERE id=$2`, userID, l.ID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return l.ChatID, nil
}
