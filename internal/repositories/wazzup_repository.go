package repositories

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"turcompany/internal/models"
)

type WazzupRepository interface {
	GetIntegrationByToken(ctx context.Context, token string) (*models.WazzupIntegration, error)
	ListCRMUsers(ctx context.Context) ([]CRMUserDTO, error)
	GetCRMUserByID(ctx context.Context, id int) (*CRMUserDTO, error)
	GetIntegrationByOwnerUserID(ctx context.Context, ownerUserID int) (*models.WazzupIntegration, error)
	GetAnyEnabledIntegration(ctx context.Context) (*models.WazzupIntegration, error)
	UpsertIntegrationByOwner(ctx context.Context, ownerUserID int, apiKeyEnc, crmKeyHash, webhooksURI string, enabled bool) (integrationID int, webhookToken string, err error)
	GetStatus(ctx context.Context) (*models.WazzupStatus, error)
	UpsertChannels(ctx context.Context, integrationID int, channels []models.WazzupChannel) error
	ListChannels(ctx context.Context, integrationID int) ([]models.WazzupChannel, error)
	RegisterDedup(ctx context.Context, integrationID int, externalID string) (isNew bool, err error)
	FindClientByPhone(ctx context.Context, phone string) (clientID int, err error)
	FindLeadByPhone(ctx context.Context, phone string) (leadID int, err error)
	FindLeadByExternalChatID(ctx context.Context, transport, externalChatID string) (leadID int, err error)
	CreateLeadFromInbound(ctx context.Context, ownerID int, phone, source, firstMessage string) (leadID int, err error)
	UpdateLeadDescriptionIfEmpty(ctx context.Context, leadID int, firstMessage string) error
	GetLeadPhoneByID(ctx context.Context, leadID int) (string, error)
	GetClientPhoneByID(ctx context.Context, clientID int) (string, error)
	UpsertExternalChat(ctx context.Context, in ExternalChatUpsert) (*models.WazzupDialog, error)
	CreateExternalMessage(ctx context.Context, in ExternalMessageCreate) (*models.WazzupDialogMessage, bool, error)
	ListExternalDialogs(ctx context.Context, userID int, transport string) ([]models.WazzupDialog, error)
	GetExternalDialog(ctx context.Context, userID, chatID int) (*models.WazzupDialog, error)
	ListExternalMessages(ctx context.Context, userID, chatID int, limit, offset int) ([]models.WazzupDialogMessage, error)
}

type CRMUserDTO struct {
	ID     int
	Name   string
	Email  string
	Phone  string
	Active bool
}

type ExternalChatUpsert struct {
	OwnerUserID       int
	BranchID          *int
	Transport         string
	ExternalChatID    string
	ExternalChannelID string
	DisplayName       string
	Username          string
	Phone             string
	ClientID          *int
	LeadID            *int
	RawPayload        json.RawMessage
	LastMessageAt     time.Time
	Direction         string
}

type ExternalMessageCreate struct {
	ChatID            int
	SenderID          *int
	Transport         string
	ExternalMessageID string
	ExternalChannelID string
	Direction         string
	Status            string
	Text              string
	RawPayload        json.RawMessage
	CreatedAt         time.Time
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

func (r *wazzupRepository) GetAnyEnabledIntegration(ctx context.Context) (*models.WazzupIntegration, error) {
	const q = `
		SELECT id, owner_user_id, api_key_enc, crm_key_hash, webhook_token, enabled, COALESCE(webhooks_uri, ''), created_at, updated_at
		FROM wazzup_integrations
		WHERE enabled = TRUE
		ORDER BY updated_at DESC, id DESC
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, q)
	return scanWazzupIntegration(row)
}

func (r *wazzupRepository) ListCRMUsers(ctx context.Context) ([]CRMUserDTO, error) {
	nameExpr, err := r.crmUserNameExpr(ctx)
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`
		SELECT id, email, %s AS name
		FROM public.users
		WHERE COALESCE(is_active, TRUE) = TRUE
		ORDER BY id
	`, nameExpr)
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list crm users: %w", err)
	}
	defer rows.Close()

	users := make([]CRMUserDTO, 0)
	for rows.Next() {
		var u CRMUserDTO
		if err := rows.Scan(&u.ID, &u.Email, &u.Name); err != nil {
			return nil, fmt.Errorf("scan crm users: %w", err)
		}
		u.Phone = ""
		u.Active = true
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate crm users: %w", err)
	}
	return users, nil
}

func (r *wazzupRepository) GetCRMUserByID(ctx context.Context, id int) (*CRMUserDTO, error) {
	nameExpr, err := r.crmUserNameExpr(ctx)
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`
		SELECT id, email, %s AS name
		FROM public.users
		WHERE id = $1
		  AND COALESCE(is_active, TRUE) = TRUE
	`, nameExpr)
	var u CRMUserDTO
	err = r.db.QueryRowContext(ctx, q, id).Scan(&u.ID, &u.Email, &u.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get crm user by id: %w", err)
	}
	u.Phone = ""
	u.Active = true
	return &u, nil
}

func (r *wazzupRepository) crmUserNameExpr(ctx context.Context) (string, error) {
	const q = `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'users'
		  AND column_name IN ('fullname', 'name')
		ORDER BY CASE column_name WHEN 'fullname' THEN 0 ELSE 1 END
		LIMIT 1
	`
	var col string
	err := r.db.QueryRowContext(ctx, q).Scan(&col)
	if errors.Is(err, sql.ErrNoRows) {
		return "email", nil
	}
	if err != nil {
		return "", fmt.Errorf("detect crm user name column: %w", err)
	}
	return fmt.Sprintf("COALESCE(NULLIF(BTRIM(%s), ''), email)", col), nil
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
			if !IsSQLState(err, SQLStateUniqueViolation) {
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

func (r *wazzupRepository) GetStatus(ctx context.Context) (*models.WazzupStatus, error) {
	status := &models.WazzupStatus{Provider: "wazzup"}
	const q = `
		SELECT
			EXISTS(SELECT 1 FROM wazzup_integrations) AS configured,
			EXISTS(SELECT 1 FROM wazzup_integrations WHERE enabled = TRUE) AS enabled,
			EXISTS(SELECT 1 FROM wazzup_integrations WHERE enabled = TRUE) AS iframe_available,
			(SELECT COUNT(*) FROM wazzup_channels)::INT AS channels_count,
			(SELECT MAX(received_at) FROM wazzup_dedup) AS last_webhook_at
	`
	var lastWebhook sql.NullTime
	if err := r.db.QueryRowContext(ctx, q).Scan(&status.Configured, &status.Enabled, &status.IframeAvailable, &status.ChannelsCount, &lastWebhook); err != nil {
		return nil, fmt.Errorf("get wazzup status: %w", err)
	}
	if lastWebhook.Valid {
		t := lastWebhook.Time
		status.LastWebhookAt = &t
	}
	return status, nil
}

func (r *wazzupRepository) UpsertChannels(ctx context.Context, integrationID int, channels []models.WazzupChannel) error {
	if len(channels) == 0 {
		return nil
	}
	const q = `
		INSERT INTO wazzup_channels (
			integration_id, external_channel_id, transport, name, username, phone, status, provider, raw_payload, updated_at
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), 'wazzup', COALESCE($8::jsonb, '{}'::jsonb), NOW())
		ON CONFLICT (integration_id, external_channel_id) DO UPDATE
		SET transport = EXCLUDED.transport,
		    name = EXCLUDED.name,
		    username = EXCLUDED.username,
		    phone = EXCLUDED.phone,
		    status = EXCLUDED.status,
		    provider = EXCLUDED.provider,
		    raw_payload = EXCLUDED.raw_payload,
		    updated_at = NOW()
	`
	for _, ch := range channels {
		raw := ch.RawPayload
		if len(raw) == 0 {
			raw = json.RawMessage(`{}`)
		}
		if _, err := r.db.ExecContext(ctx, q,
			integrationID,
			strings.TrimSpace(ch.ExternalChannelID),
			strings.ToLower(strings.TrimSpace(ch.Transport)),
			strings.TrimSpace(ch.Name),
			strings.TrimSpace(ch.Username),
			normalizePhone(ch.Phone),
			strings.ToLower(strings.TrimSpace(ch.Status)),
			string(raw),
		); err != nil {
			return fmt.Errorf("upsert wazzup channel: %w", err)
		}
	}
	return nil
}

func (r *wazzupRepository) ListChannels(ctx context.Context, integrationID int) ([]models.WazzupChannel, error) {
	args := []any{}
	where := ""
	if integrationID > 0 {
		where = "WHERE integration_id = $1"
		args = append(args, integrationID)
	}
	q := fmt.Sprintf(`
		SELECT id, integration_id, external_channel_id, transport, COALESCE(name, ''), COALESCE(username, ''),
		       COALESCE(phone, ''), COALESCE(status, ''), provider, raw_payload, created_at, updated_at
		FROM wazzup_channels
		%s
		ORDER BY transport, name, external_channel_id
	`, where)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list wazzup channels: %w", err)
	}
	defer rows.Close()
	var out []models.WazzupChannel
	for rows.Next() {
		var ch models.WazzupChannel
		var raw []byte
		if err := rows.Scan(
			&ch.ID,
			&ch.IntegrationID,
			&ch.ExternalChannelID,
			&ch.Transport,
			&ch.Name,
			&ch.Username,
			&ch.Phone,
			&ch.Status,
			&ch.Provider,
			&raw,
			&ch.CreatedAt,
			&ch.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan wazzup channel: %w", err)
		}
		ch.RawPayload = append(json.RawMessage(nil), raw...)
		out = append(out, ch)
	}
	return out, rows.Err()
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

func (r *wazzupRepository) FindClientByPhone(ctx context.Context, phone string) (int, error) {
	normalizedPhone := normalizePhone(phone)
	if normalizedPhone == "" {
		return 0, nil
	}
	const q = `
		SELECT id
		FROM clients
		WHERE regexp_replace(COALESCE(primary_phone, phone, ''), '\D', '', 'g') = $1
		ORDER BY id
		LIMIT 1
	`
	var clientID int
	err := r.db.QueryRowContext(ctx, q, normalizedPhone).Scan(&clientID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find client by phone: %w", err)
	}
	return clientID, nil
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

func (r *wazzupRepository) FindLeadByExternalChatID(ctx context.Context, transport, externalChatID string) (int, error) {
	const q = `
		SELECT c.lead_ref_id
		FROM chats c
		WHERE c.external_provider = 'wazzup'
		  AND c.external_transport = $1
		  AND c.external_chat_id = $2
		  AND c.lead_ref_id IS NOT NULL
		ORDER BY c.id
		LIMIT 1
	`
	var leadID int
	err := r.db.QueryRowContext(ctx, q, strings.ToLower(strings.TrimSpace(transport)), strings.TrimSpace(externalChatID)).Scan(&leadID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find lead by external chat id: %w", err)
	}
	return leadID, nil
}

func (r *wazzupRepository) CreateLeadFromInbound(ctx context.Context, ownerID int, phone, source, firstMessage string) (int, error) {
	description := strings.TrimSpace(firstMessage)
	normalizedPhone := normalizePhone(phone)
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "whatsapp"
	}

	var title string
	switch source {
	case "telegram":
		if phone != "" {
			title = fmt.Sprintf("Лид из Telegram +%s", normalizedPhone)
		} else {
			title = "Лид из Telegram"
		}
	case "instagram":
		if phone != "" {
			title = fmt.Sprintf("Лид из Instagram +%s", normalizedPhone)
		} else {
			title = "Лид из Instagram"
		}
	default:
		title = fmt.Sprintf("Лид из WhatsApp +%s", normalizedPhone)
	}

	resolvedOwner, err := resolveAutoLeadOwner(ctx, r.db, ownerID)
	if err != nil {
		return 0, fmt.Errorf("create lead from inbound: %w", err)
	}

	const q = `
		INSERT INTO leads (title, description, phone, source, owner_id, branch_id, department_id, status)
		VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, 0),
			(SELECT branch_id FROM users WHERE $5 > 0 AND id = $5),
			(SELECT department_id FROM users WHERE $5 > 0 AND id = $5),
			$6)
		RETURNING id
	`
	var leadID int
	if err := r.db.QueryRowContext(ctx, q, title, description, normalizedPhone, source, resolvedOwner, "new").Scan(&leadID); err != nil {
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
	const q = `SELECT COALESCE(primary_phone, phone, '') FROM clients WHERE id = $1`
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

func (r *wazzupRepository) UpsertExternalChat(ctx context.Context, in ExternalChatUpsert) (dialog *models.WazzupDialog, err error) {
	in.Transport = strings.ToLower(strings.TrimSpace(in.Transport))
	in.ExternalChatID = strings.TrimSpace(in.ExternalChatID)
	in.ExternalChannelID = strings.TrimSpace(in.ExternalChannelID)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.DisplayName == "" {
		in.DisplayName = firstNonEmptyString(in.Username, in.Phone, in.ExternalChatID)
	}
	if in.LastMessageAt.IsZero() {
		in.LastMessageAt = time.Now()
	}
	raw := in.RawPayload
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin external chat tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const findQ = `
		SELECT id
		FROM chats
		WHERE external_provider = 'wazzup'
		  AND external_transport = $1
		  AND external_chat_id = $2
		  AND COALESCE(external_channel_id, '') = $3
		ORDER BY id
		LIMIT 1
	`
	var chatID int
	findErr := tx.QueryRowContext(ctx, findQ, in.Transport, in.ExternalChatID, in.ExternalChannelID).Scan(&chatID)
	if findErr != nil && !errors.Is(findErr, sql.ErrNoRows) {
		return nil, fmt.Errorf("find external chat: %w", findErr)
	}

	var branchID sql.NullInt64
	if in.BranchID != nil {
		branchID = sql.NullInt64{Int64: int64(*in.BranchID), Valid: true}
	}
	var clientID sql.NullInt64
	if in.ClientID != nil {
		clientID = sql.NullInt64{Int64: int64(*in.ClientID), Valid: true}
	}
	var leadID sql.NullInt64
	if in.LeadID != nil {
		leadID = sql.NullInt64{Int64: int64(*in.LeadID), Valid: true}
	}

	if errors.Is(findErr, sql.ErrNoRows) {
		const insertQ = `
			INSERT INTO chats (
				name, creator_id, is_group, branch_id,
				external_provider, external_transport, external_chat_id, external_channel_id,
				external_display_name, external_username, external_phone, external_raw_payload,
				external_last_message_at, external_last_inbound_at, external_last_outbound_at,
				client_ref_id, lead_ref_id
			)
			VALUES (
				$1, $2, FALSE, COALESCE($3, (SELECT branch_id FROM users WHERE id = $2)),
				'wazzup', $4, $5, NULLIF($6, ''),
				NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), $10::jsonb,
				$11,
				CASE WHEN $12 = 'incoming' THEN $11 ELSE NULL END,
				CASE WHEN $12 = 'outgoing' THEN $11 ELSE NULL END,
				$13, $14
			)
			RETURNING id
		`
		if err = tx.QueryRowContext(ctx, insertQ,
			in.DisplayName,
			in.OwnerUserID,
			branchID,
			in.Transport,
			in.ExternalChatID,
			in.ExternalChannelID,
			in.DisplayName,
			strings.TrimSpace(in.Username),
			normalizePhone(in.Phone),
			string(raw),
			in.LastMessageAt,
			strings.ToLower(strings.TrimSpace(in.Direction)),
			clientID,
			leadID,
		).Scan(&chatID); err != nil {
			return nil, fmt.Errorf("insert external chat: %w", err)
		}
	} else {
		const updateQ = `
			UPDATE chats
			SET name = COALESCE(NULLIF($1, ''), name),
			    branch_id = COALESCE($2, branch_id, (SELECT branch_id FROM users WHERE id = $11)),
			    external_display_name = COALESCE(NULLIF($1, ''), external_display_name),
			    external_username = COALESCE(NULLIF($3, ''), external_username),
			    external_phone = COALESCE(NULLIF($4, ''), external_phone),
			    external_raw_payload = $5::jsonb,
			    external_last_message_at = GREATEST(COALESCE(external_last_message_at, $6), $6),
			    external_last_inbound_at = CASE WHEN $7 = 'incoming' THEN GREATEST(COALESCE(external_last_inbound_at, $6), $6) ELSE external_last_inbound_at END,
			    external_last_outbound_at = CASE WHEN $7 = 'outgoing' THEN GREATEST(COALESCE(external_last_outbound_at, $6), $6) ELSE external_last_outbound_at END,
			    client_ref_id = COALESCE($8, client_ref_id),
			    lead_ref_id = COALESCE($9, lead_ref_id)
			WHERE id = $10
		`
		if _, err = tx.ExecContext(ctx, updateQ,
			in.DisplayName,
			branchID,
			strings.TrimSpace(in.Username),
			normalizePhone(in.Phone),
			string(raw),
			in.LastMessageAt,
			strings.ToLower(strings.TrimSpace(in.Direction)),
			clientID,
			leadID,
			chatID,
			in.OwnerUserID,
		); err != nil {
			return nil, fmt.Errorf("update external chat: %w", err)
		}
	}

	const memberQ = `
		INSERT INTO chat_members (chat_id, user_id, role)
		VALUES ($1, $2, 'owner')
		ON CONFLICT (chat_id, user_id) DO NOTHING
	`
	if _, err = tx.ExecContext(ctx, memberQ, chatID, in.OwnerUserID); err != nil {
		return nil, fmt.Errorf("insert external chat member: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit external chat tx: %w", err)
	}

	return r.getExternalDialogByID(ctx, chatID)
}

func (r *wazzupRepository) CreateExternalMessage(ctx context.Context, in ExternalMessageCreate) (*models.WazzupDialogMessage, bool, error) {
	in.ExternalMessageID = strings.TrimSpace(in.ExternalMessageID)
	in.Transport = strings.ToLower(strings.TrimSpace(in.Transport))
	in.Direction = strings.ToLower(strings.TrimSpace(in.Direction))
	if in.Direction == "" {
		in.Direction = "incoming"
	}
	if in.Status == "" {
		if in.Direction == "outgoing" {
			in.Status = "sent"
		} else {
			in.Status = "received"
		}
	}
	if in.CreatedAt.IsZero() {
		in.CreatedAt = time.Now()
	}
	raw := in.RawPayload
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if in.ExternalMessageID != "" {
		const findQ = `
			SELECT id, chat_id, sender_id, text, external_direction, external_status, external_transport,
			       COALESCE(external_message_id, ''), COALESCE(external_channel_id, ''), external_raw_payload, created_at
			FROM messages
			WHERE external_provider = 'wazzup'
			  AND external_message_id = $1
			LIMIT 1
		`
		rows, err := r.db.QueryContext(ctx, findQ, in.ExternalMessageID)
		if err != nil {
			return nil, false, fmt.Errorf("find external message: %w", err)
		}
		msgs, scanErr := scanWazzupDialogMessages(rows)
		_ = rows.Close()
		if scanErr != nil {
			return nil, false, scanErr
		}
		if len(msgs) > 0 {
			return &msgs[0], false, nil
		}
	}

	var senderID sql.NullInt64
	if in.SenderID != nil {
		senderID = sql.NullInt64{Int64: int64(*in.SenderID), Valid: true}
	}
	const insertQ = `
		INSERT INTO messages (
			chat_id, sender_id, text, attachments, created_at,
			external_provider, external_transport, external_message_id, external_channel_id,
			external_direction, external_status, external_raw_payload
		)
		VALUES ($1, $2, $3, '[]'::jsonb, $4, 'wazzup', $5, NULLIF($6, ''), NULLIF($7, ''), $8, $9, $10::jsonb)
		RETURNING id, chat_id, sender_id, text, external_direction, external_status, external_transport,
		          COALESCE(external_message_id, ''), COALESCE(external_channel_id, ''), external_raw_payload, created_at
	`
	rows, err := r.db.QueryContext(ctx, insertQ,
		in.ChatID,
		senderID,
		strings.TrimSpace(in.Text),
		in.CreatedAt,
		in.Transport,
		in.ExternalMessageID,
		strings.TrimSpace(in.ExternalChannelID),
		in.Direction,
		strings.ToLower(strings.TrimSpace(in.Status)),
		string(raw),
	)
	if err != nil {
		if IsSQLState(err, SQLStateUniqueViolation) && in.ExternalMessageID != "" {
			return r.CreateExternalMessage(ctx, in)
		}
		return nil, false, fmt.Errorf("insert external message: %w", err)
	}
	msgs, scanErr := scanWazzupDialogMessages(rows)
	_ = rows.Close()
	if scanErr != nil {
		return nil, false, scanErr
	}
	if len(msgs) == 0 {
		return nil, false, sql.ErrNoRows
	}
	return &msgs[0], true, nil
}

func (r *wazzupRepository) ListExternalDialogs(ctx context.Context, userID int, transport string) ([]models.WazzupDialog, error) {
	transport = strings.ToLower(strings.TrimSpace(transport))
	args := []any{userID}
	where := "c.external_provider = 'wazzup' AND EXISTS (SELECT 1 FROM chat_members cm WHERE cm.chat_id = c.id AND cm.user_id = $1)"
	if transport != "" {
		args = append(args, transport)
		where += fmt.Sprintf(" AND c.external_transport = $%d", len(args))
	}
	q := fmt.Sprintf(`
		SELECT c.id, c.external_provider, c.external_transport, c.external_chat_id, COALESCE(c.external_channel_id, ''),
		       COALESCE(c.external_display_name, c.name), COALESCE(c.external_username, ''), COALESCE(c.external_phone, ''),
		       COALESCE(lm.text, ''), lm.created_at,
		       COALESCE(unread.unread_count, 0),
		       c.client_ref_id, c.lead_ref_id, c.branch_id
		FROM chats c
		LEFT JOIN LATERAL (
			SELECT text, created_at
			FROM messages m
			WHERE m.chat_id = c.id
			ORDER BY created_at DESC, id DESC
			LIMIT 1
		) lm ON TRUE
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS unread_count
			FROM messages m
			LEFT JOIN chat_read_state crs ON crs.chat_id = m.chat_id AND crs.user_id = $1
			WHERE m.chat_id = c.id
			  AND m.external_direction = 'incoming'
			  AND m.id > COALESCE(crs.last_read_message_id, 0)
		) unread ON TRUE
		WHERE %s
		ORDER BY COALESCE(lm.created_at, c.external_last_message_at, c.created_at) DESC, c.id DESC
	`, where)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list external dialogs: %w", err)
	}
	defer rows.Close()
	return scanWazzupDialogs(rows)
}

func (r *wazzupRepository) GetExternalDialog(ctx context.Context, userID, chatID int) (*models.WazzupDialog, error) {
	const q = `
		SELECT c.id, c.external_provider, c.external_transport, c.external_chat_id, COALESCE(c.external_channel_id, ''),
		       COALESCE(c.external_display_name, c.name), COALESCE(c.external_username, ''), COALESCE(c.external_phone, ''),
		       COALESCE(lm.text, ''), lm.created_at,
		       COALESCE(unread.unread_count, 0),
		       c.client_ref_id, c.lead_ref_id, c.branch_id
		FROM chats c
		LEFT JOIN LATERAL (
			SELECT text, created_at
			FROM messages m
			WHERE m.chat_id = c.id
			ORDER BY created_at DESC, id DESC
			LIMIT 1
		) lm ON TRUE
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS unread_count
			FROM messages m
			LEFT JOIN chat_read_state crs ON crs.chat_id = m.chat_id AND crs.user_id = $1
			WHERE m.chat_id = c.id
			  AND m.external_direction = 'incoming'
			  AND m.id > COALESCE(crs.last_read_message_id, 0)
		) unread ON TRUE
		WHERE c.id = $2
		  AND c.external_provider = 'wazzup'
		  AND EXISTS (SELECT 1 FROM chat_members cm WHERE cm.chat_id = c.id AND cm.user_id = $1)
	`
	rows, err := r.db.QueryContext(ctx, q, userID, chatID)
	if err != nil {
		return nil, fmt.Errorf("get external dialog: %w", err)
	}
	defer rows.Close()
	dialogs, err := scanWazzupDialogs(rows)
	if err != nil {
		return nil, err
	}
	if len(dialogs) == 0 {
		return nil, nil
	}
	return &dialogs[0], nil
}

func (r *wazzupRepository) ListExternalMessages(ctx context.Context, userID, chatID int, limit, offset int) ([]models.WazzupDialogMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	const q = `
		SELECT m.id, m.chat_id, m.sender_id, m.text, m.external_direction, m.external_status, m.external_transport,
		       COALESCE(m.external_message_id, ''), COALESCE(m.external_channel_id, ''), m.external_raw_payload, m.created_at
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		WHERE m.chat_id = $1
		  AND c.external_provider = 'wazzup'
		  AND EXISTS (SELECT 1 FROM chat_members cm WHERE cm.chat_id = m.chat_id AND cm.user_id = $2)
		ORDER BY m.created_at ASC, m.id ASC
		LIMIT $3 OFFSET $4
	`
	rows, err := r.db.QueryContext(ctx, q, chatID, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list external messages: %w", err)
	}
	msgs, scanErr := scanWazzupDialogMessages(rows)
	_ = rows.Close()
	if scanErr != nil {
		return nil, scanErr
	}
	if len(msgs) > 0 {
		lastID := msgs[len(msgs)-1].ID
		const readQ = `
			INSERT INTO chat_read_state (chat_id, user_id, last_read_message_id, read_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (chat_id, user_id) DO UPDATE
			SET last_read_message_id = GREATEST(COALESCE(chat_read_state.last_read_message_id, 0), EXCLUDED.last_read_message_id),
			    read_at = NOW()
		`
		if _, err := r.db.ExecContext(ctx, readQ, chatID, userID, lastID); err != nil {
			return nil, fmt.Errorf("mark external messages read: %w", err)
		}
	}
	return msgs, nil
}

func (r *wazzupRepository) getExternalDialogByID(ctx context.Context, chatID int) (*models.WazzupDialog, error) {
	const q = `
		SELECT c.id, c.external_provider, c.external_transport, c.external_chat_id, COALESCE(c.external_channel_id, ''),
		       COALESCE(c.external_display_name, c.name), COALESCE(c.external_username, ''), COALESCE(c.external_phone, ''),
		       COALESCE(lm.text, ''), lm.created_at,
		       0,
		       c.client_ref_id, c.lead_ref_id, c.branch_id
		FROM chats c
		LEFT JOIN LATERAL (
			SELECT text, created_at
			FROM messages m
			WHERE m.chat_id = c.id
			ORDER BY created_at DESC, id DESC
			LIMIT 1
		) lm ON TRUE
		WHERE c.id = $1
		  AND c.external_provider = 'wazzup'
	`
	rows, err := r.db.QueryContext(ctx, q, chatID)
	if err != nil {
		return nil, fmt.Errorf("get external dialog by id: %w", err)
	}
	defer rows.Close()
	dialogs, err := scanWazzupDialogs(rows)
	if err != nil {
		return nil, err
	}
	if len(dialogs) == 0 {
		return nil, nil
	}
	return &dialogs[0], nil
}

func scanWazzupDialogs(rows *sql.Rows) ([]models.WazzupDialog, error) {
	var out []models.WazzupDialog
	for rows.Next() {
		var d models.WazzupDialog
		var lastAt sql.NullTime
		var clientID, leadID, branchID sql.NullInt64
		if err := rows.Scan(
			&d.ID,
			&d.Provider,
			&d.Transport,
			&d.ExternalChatID,
			&d.ExternalChannelID,
			&d.DisplayName,
			&d.Username,
			&d.Phone,
			&d.LastMessageText,
			&lastAt,
			&d.UnreadCount,
			&clientID,
			&leadID,
			&branchID,
		); err != nil {
			return nil, fmt.Errorf("scan wazzup dialog: %w", err)
		}
		if lastAt.Valid {
			t := lastAt.Time
			d.LastMessageAt = &t
		}
		if clientID.Valid {
			v := int(clientID.Int64)
			d.ClientID = &v
		}
		if leadID.Valid {
			v := int(leadID.Int64)
			d.LeadID = &v
		}
		if branchID.Valid {
			v := int(branchID.Int64)
			d.BranchID = &v
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func scanWazzupDialogMessages(rows *sql.Rows) ([]models.WazzupDialogMessage, error) {
	var out []models.WazzupDialogMessage
	for rows.Next() {
		var m models.WazzupDialogMessage
		var senderID sql.NullInt64
		var raw []byte
		if err := rows.Scan(
			&m.ID,
			&m.ChatID,
			&senderID,
			&m.Text,
			&m.Direction,
			&m.Status,
			&m.Transport,
			&m.ExternalMessageID,
			&m.ExternalChannelID,
			&raw,
			&m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan wazzup dialog message: %w", err)
		}
		if senderID.Valid {
			v := int(senderID.Int64)
			m.SenderID = &v
		}
		if len(raw) > 0 {
			m.RawPayload = append(json.RawMessage(nil), raw...)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
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
