package repositories

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"turcompany/internal/models"
)

type WazzupRepository interface {
	GetIntegrationByToken(ctx context.Context, token string) (*models.WazzupIntegration, error)
	GetIntegrationByOwnerUserID(ctx context.Context, ownerUserID int) (*models.WazzupIntegration, error)
	UpsertIntegrationByOwner(ctx context.Context, ownerUserID int, apiKeyEnc, crmKeyHash, webhooksURI string, enabled bool) (integrationID int, webhookToken string, err error)
	RegisterDedup(ctx context.Context, integrationID int, externalID string) (isNew bool, err error)
	FindLeadByPhone(ctx context.Context, phone string) (leadID int, err error)
	CreateLeadFromInbound(ctx context.Context, ownerID int, phone, firstMessage string) (leadID int, err error)
	UpdateLeadDescriptionIfEmpty(ctx context.Context, leadID int, firstMessage string) error
	GetLeadPhoneByID(ctx context.Context, leadID int) (string, error)
	GetClientPhoneByID(ctx context.Context, clientID int) (string, error)
}

type wazzupRepository struct {
	db *sql.DB
}

func NewWazzupRepository(db *sql.DB) WazzupRepository {
	return &wazzupRepository{db: db}
}

func (r *wazzupRepository) GetIntegrationByToken(ctx context.Context, token string) (*models.WazzupIntegration, error) {
	const q = `
		SELECT id, owner_user_id, api_key_enc, crm_key_hash, webhook_token, enabled, COALESCE(webhooks_uri, ''), created_at, updated_at
		FROM wazzup_integrations
		WHERE webhook_token = $1
	`
	row := r.db.QueryRowContext(ctx, q, strings.TrimSpace(token))
	return scanWazzupIntegration(row)
}

func (r *wazzupRepository) GetIntegrationByOwnerUserID(ctx context.Context, ownerUserID int) (*models.WazzupIntegration, error) {
	const q = `
		SELECT id, owner_user_id, api_key_enc, crm_key_hash, webhook_token, enabled, COALESCE(webhooks_uri, ''), created_at, updated_at
		FROM wazzup_integrations
		WHERE owner_user_id = $1
		ORDER BY id
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, q, ownerUserID)
	return scanWazzupIntegration(row)
}

func (r *wazzupRepository) UpsertIntegrationByOwner(ctx context.Context, ownerUserID int, apiKeyEnc, crmKeyHash, webhooksURI string, enabled bool) (integrationID int, webhookToken string, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const findQ = `SELECT id, webhook_token FROM wazzup_integrations WHERE owner_user_id = $1 ORDER BY id LIMIT 1`
	var id int
	var token string
	findErr := tx.QueryRowContext(ctx, findQ, ownerUserID).Scan(&id, &token)
	switch {
	case findErr == nil:
		const updQ = `
			UPDATE wazzup_integrations
			SET api_key_enc = $1,
			    crm_key_hash = $2,
			    webhooks_uri = NULLIF($3, ''),
			    enabled = $4,
			    updated_at = NOW()
			WHERE id = $5
		`
		if _, err = tx.ExecContext(ctx, updQ, apiKeyEnc, crmKeyHash, strings.TrimSpace(webhooksURI), enabled, id); err != nil {
			return 0, "", fmt.Errorf("update integration: %w", err)
		}
		if err = tx.Commit(); err != nil {
			return 0, "", fmt.Errorf("commit tx: %w", err)
		}
		return id, token, nil
	case errors.Is(findErr, sql.ErrNoRows):
		newToken, tokenErr := newWebhookToken()
		if tokenErr != nil {
			return 0, "", tokenErr
		}
		const insQ = `
			INSERT INTO wazzup_integrations (owner_user_id, api_key_enc, crm_key_hash, webhook_token, enabled, webhooks_uri)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))
			RETURNING id, webhook_token
		`
		if err = tx.QueryRowContext(ctx, insQ, ownerUserID, apiKeyEnc, crmKeyHash, newToken, enabled, strings.TrimSpace(webhooksURI)).Scan(&id, &token); err != nil {
			if !isUniqueViolation(err) {
				return 0, "", fmt.Errorf("insert integration: %w", err)
			}
			if scanErr := tx.QueryRowContext(ctx, findQ, ownerUserID).Scan(&id, &token); scanErr != nil {
				return 0, "", fmt.Errorf("re-select integration after unique violation: %w", scanErr)
			}
			const updQ = `
				UPDATE wazzup_integrations
				SET api_key_enc = $1,
				    crm_key_hash = $2,
				    webhooks_uri = NULLIF($3, ''),
				    enabled = $4,
				    updated_at = NOW()
				WHERE id = $5
			`
			if _, execErr := tx.ExecContext(ctx, updQ, apiKeyEnc, crmKeyHash, strings.TrimSpace(webhooksURI), enabled, id); execErr != nil {
				return 0, "", fmt.Errorf("update integration after unique violation: %w", execErr)
			}
		}
		if err = tx.Commit(); err != nil {
			return 0, "", fmt.Errorf("commit tx: %w", err)
		}
		return id, token, nil
	default:
		return 0, "", fmt.Errorf("find integration by owner: %w", findErr)
	}
}

func (r *wazzupRepository) RegisterDedup(ctx context.Context, integrationID int, externalID string) (bool, error) {
	const q = `
		INSERT INTO wazzup_dedup (integration_id, external_id)
		VALUES ($1, $2)
		ON CONFLICT (integration_id, external_id) DO NOTHING
	`
	res, err := r.db.ExecContext(ctx, q, integrationID, strings.TrimSpace(externalID))
	if err != nil {
		return false, fmt.Errorf("register dedup: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("register dedup rows affected: %w", err)
	}
	return affected > 0, nil
}

func (r *wazzupRepository) FindLeadByPhone(ctx context.Context, phone string) (int, error) {
	normalizedPhone := normalizePhone(phone)
	// leads.phone should be stored normalized (digits only).
	const q = `
		SELECT id
		FROM leads
		WHERE phone = $1
		ORDER BY id
		LIMIT 1
	`
	var leadID int
	err := r.db.QueryRowContext(ctx, q, normalizedPhone).Scan(&leadID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find lead by phone: %w", err)
	}
	return leadID, nil
}

func (r *wazzupRepository) CreateLeadFromInbound(ctx context.Context, ownerID int, phone, firstMessage string) (int, error) {
	description := strings.TrimSpace(firstMessage)
	normalizedPhone := normalizePhone(phone)
	title := fmt.Sprintf("Лид из WhatsApp +%s", normalizedPhone)
	const q = `
		INSERT INTO leads (title, description, phone, source, owner_id, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`
	var leadID int
	err := r.db.QueryRowContext(ctx, q, title, description, normalizedPhone, "whatsapp", ownerID, "new").Scan(&leadID)
	if err != nil {
		return 0, fmt.Errorf("create lead from inbound: %w", err)
	}
	return leadID, nil
}

func (r *wazzupRepository) UpdateLeadDescriptionIfEmpty(ctx context.Context, leadID int, firstMessage string) error {
	const q = `
		UPDATE leads
		SET description = $1
		WHERE id = $2
		  AND (description IS NULL OR BTRIM(description) = '')
	`
	_, err := r.db.ExecContext(ctx, q, strings.TrimSpace(firstMessage), leadID)
	if err != nil {
		return fmt.Errorf("update lead description if empty: %w", err)
	}
	return nil
}

func (r *wazzupRepository) GetLeadPhoneByID(ctx context.Context, leadID int) (string, error) {
	const q = `SELECT COALESCE(phone, '') FROM leads WHERE id = $1`
	var phone string
	err := r.db.QueryRowContext(ctx, q, leadID).Scan(&phone)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get lead phone by id: %w", err)
	}
	return phone, nil
}

func (r *wazzupRepository) GetClientPhoneByID(ctx context.Context, clientID int) (string, error) {
	const q = `SELECT COALESCE(phone, '') FROM clients WHERE id = $1`
	var phone string
	err := r.db.QueryRowContext(ctx, q, clientID).Scan(&phone)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get client phone by id: %w", err)
	}
	return phone, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}

func normalizePhone(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	b := strings.Builder{}
	b.Grow(len(trimmed))
	for _, r := range trimmed {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func newWebhookToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate webhook token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func scanWazzupIntegration(scanner interface{ Scan(dest ...any) error }) (*models.WazzupIntegration, error) {
	integration := &models.WazzupIntegration{}
	if err := scanner.Scan(
		&integration.ID,
		&integration.OwnerUserID,
		&integration.APIKeyEnc,
		&integration.CRMKeyHash,
		&integration.WebhookToken,
		&integration.Enabled,
		&integration.WebhooksURI,
		&integration.CreatedAt,
		&integration.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return integration, nil
}
