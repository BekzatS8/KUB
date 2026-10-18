package repositories

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// MetaAppSettings — ключи приложения Meta, введённые в CRM.
type MetaAppSettings struct {
	AppID     string
	AppSecret string
}

// MetaSettingsRepository хранит ключи приложения Meta (одна строка на CRM).
type MetaSettingsRepository interface {
	Get(ctx context.Context) (*MetaAppSettings, error)
	// Save сохраняет ключи; пустой appSecret оставляет прежний секрет.
	Save(ctx context.Context, appID, appSecret string, userID int) error
}

type metaSettingsRepository struct {
	db *sql.DB
}

func NewMetaSettingsRepository(db *sql.DB) MetaSettingsRepository {
	return &metaSettingsRepository{db: db}
}

func (r *metaSettingsRepository) Get(ctx context.Context) (*MetaAppSettings, error) {
	var s MetaAppSettings
	err := r.db.QueryRowContext(ctx, `SELECT app_id, app_secret FROM meta_app_settings WHERE id = 1`).Scan(&s.AppID, &s.AppSecret)
	if errors.Is(err, sql.ErrNoRows) {
		return &MetaAppSettings{}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *metaSettingsRepository) Save(ctx context.Context, appID, appSecret string, userID int) error {
	var updatedBy any
	if userID > 0 {
		updatedBy = userID
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO meta_app_settings (id, app_id, app_secret, updated_by, updated_at)
		VALUES (1, $1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE
		SET app_id     = EXCLUDED.app_id,
		    app_secret = CASE WHEN EXCLUDED.app_secret = '' THEN meta_app_settings.app_secret ELSE EXCLUDED.app_secret END,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = NOW()`,
		strings.TrimSpace(appID), strings.TrimSpace(appSecret), updatedBy)
	return err
}
