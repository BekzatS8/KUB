// internal/repositories/telegram_link_repository.go
package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

var ErrTelegramChatNotAttached = errors.New("telegram chat is not attached to link code")

type TelegramLink struct {
	ID        int
	UserID    sql.NullInt64
	ChatID    sql.NullInt64
	Code      string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

type TelegramLinkRepository interface {
	CreateLink(ctx context.Context, userID int, chatID int64, code string, expiresAt time.Time) (*TelegramLink, error)
	GetByCode(ctx context.Context, code string) (*TelegramLink, error)

	// ✅ NEW: attach chat_id to existing code (for CRM-generated codes)
	AttachChatID(ctx context.Context, code string, chatID int64) error

	// ✅ FIXED: don't burn code if chat_id is NULL
	ConfirmLink(ctx context.Context, code string, userID int) (int64, error)
}

type telegramLinkRepository struct{ db *sql.DB }

type dbIdentity struct {
	Database   string
	User       string
	ServerAddr sql.NullString
	ServerPort sql.NullInt64
}

func NewTelegramLinkRepository(db *sql.DB) TelegramLinkRepository {
	return &telegramLinkRepository{db: db}
}

func codePrefix(code string) string {
	if len(code) <= 8 {
		return code
	}
	return code[:8]
}

func (r *telegramLinkRepository) logDBIdentity(ctx context.Context, op string) {
	var ident dbIdentity
	err := r.db.QueryRowContext(ctx, `
		SELECT current_database(), current_user, inet_server_addr(), inet_server_port()
	`).Scan(&ident.Database, &ident.User, &ident.ServerAddr, &ident.ServerPort)
	if err != nil {
		log.Printf("[TG:LINK-REPO][diag] op=%s db_identity_err=%v", op, err)
		return
	}

	addr := "unknown"
	if ident.ServerAddr.Valid {
		addr = ident.ServerAddr.String
	}
	port := int64(0)
	if ident.ServerPort.Valid {
		port = ident.ServerPort.Int64
	}
	log.Printf("[TG:LINK-REPO][diag] op=%s database=%s user=%s server_addr=%s server_port=%d",
		op,
		ident.Database,
		ident.User,
		addr,
		port,
	)
}

// 1) INSERT: 0 -> NULL
func (r *telegramLinkRepository) CreateLink(ctx context.Context, userID int, chatID int64, code string, expiresAt time.Time) (*TelegramLink, error) {
	r.logDBIdentity(ctx, "CreateLink")
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}

	var userIDVal any
	if userID > 0 {
		userIDVal = userID
	} else {
		userIDVal = nil
	}

	var chatIDVal any
	if chatID != 0 {
		chatIDVal = chatID
	} else {
		chatIDVal = nil
	}

	row := r.db.QueryRowContext(ctx, `
		INSERT INTO telegram_links (user_id, telegram_chat_id, code, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, telegram_chat_id, code, expires_at, used, created_at
	`, userIDVal, chatIDVal, code, expiresAt)

	var l TelegramLink
	if err := row.Scan(
		&l.ID, &l.UserID, &l.ChatID, &l.Code, &l.ExpiresAt, &l.Used, &l.CreatedAt,
	); err != nil {
		return nil, err
	}
	log.Printf("[TG:LINK-REPO][diag] op=CreateLink code_prefix=%s user_id=%d chat_id=%d expires_at=%s", codePrefix(l.Code), userID, chatID, l.ExpiresAt.UTC().Format(time.RFC3339))
	return &l, nil
}

func (r *telegramLinkRepository) GetByCode(ctx context.Context, code string) (*TelegramLink, error) {
	r.logDBIdentity(ctx, "GetByCode")
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, nil
	}

	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, telegram_chat_id, code, expires_at, used, created_at
		FROM telegram_links
		WHERE code=$1
	`, code)

	var l TelegramLink
	if err := row.Scan(
		&l.ID, &l.UserID, &l.ChatID, &l.Code, &l.ExpiresAt, &l.Used, &l.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[TG:LINK-REPO][diag] op=GetByCode code_prefix=%s found=false", codePrefix(code))
			return nil, nil
		}
		return nil, err
	}
	log.Printf("[TG:LINK-REPO][diag] op=GetByCode code_prefix=%s found=true used=%v chat_attached=%v expires_at=%s", codePrefix(code), l.Used, l.ChatID.Valid, l.ExpiresAt.UTC().Format(time.RFC3339))
	return &l, nil
}

// ✅ NEW: user got code in CRM, then opens bot and sends "/start CODE".
// This stores chat_id on that code row.
func (r *telegramLinkRepository) AttachChatID(ctx context.Context, code string, chatID int64) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" || chatID == 0 {
		return fmt.Errorf("code/chatID required")
	}

	res, err := r.db.ExecContext(ctx, `
		UPDATE telegram_links
		SET telegram_chat_id = $1
		WHERE code = $2
		  AND used = FALSE
		  AND expires_at > NOW()
	`, chatID, code)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	log.Printf("[TG:LINK-REPO][diag] op=AttachChatID code_prefix=%s chat_id=%d rows_affected=%d", codePrefix(code), chatID, aff)
	if aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ✅ FIXED: no "burn" if chat_id NULL
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
	`, code).Scan(
		&l.ID, &l.UserID, &l.ChatID, &l.Code, &l.ExpiresAt, &l.Used, &l.CreatedAt,
	)
	if err != nil {
		return 0, err
	}

	if l.Used || time.Now().After(l.ExpiresAt) {
		return 0, sql.ErrNoRows
	}

	// ✅ MUST be before update/commit
	if !l.ChatID.Valid {
		return 0, fmt.Errorf("%w for code=%s", ErrTelegramChatNotAttached, code)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE telegram_links SET used=true, user_id=$1 WHERE id=$2`,
		userID, l.ID,
	); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return l.ChatID.Int64, nil
}
