package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"turcompany/internal/models"
)

// TelephonyRepository defines data access for telephony_calls.
type TelephonyRepository interface {
	CreateCall(ctx context.Context, call *models.TelephonyCall) (int64, error)
	UpsertCall(ctx context.Context, call *models.TelephonyCall) (int64, bool, error)
	GetByID(ctx context.Context, id int64) (*models.TelephonyCallResponse, error)
	FindByExternalCallID(ctx context.Context, provider, externalCallID string) (*models.TelephonyCall, error)
	List(ctx context.Context, filter models.TelephonyCallListFilter) ([]*models.TelephonyCallResponse, int, error)
	ListByClient(ctx context.Context, clientID int64, limit, offset int) ([]*models.TelephonyCallResponse, int, error)
	ListByLead(ctx context.Context, leadID int64, limit, offset int) ([]*models.TelephonyCallResponse, int, error)
	LinkToClient(ctx context.Context, callID, clientID int64) error
	LinkToLead(ctx context.Context, callID, leadID int64) error
	FindClientByPhone(ctx context.Context, normalizedPhone string) (int64, error)
	FindLeadByPhone(ctx context.Context, normalizedPhone string) (int64, error)
	CreateLeadFromCall(ctx context.Context, phone, normalizedPhone string, ownerID *int, branchID *int) (int64, error)
	FindManagerByExtension(ctx context.Context, extension string) (userID int, branchID int, err error)
}

type telephonyRepository struct {
	db *sql.DB
}

// NewTelephonyRepository creates a new TelephonyRepository backed by PostgreSQL.
func NewTelephonyRepository(db *sql.DB) TelephonyRepository {
	return &telephonyRepository{db: db}
}

// NormalizePhoneForTelephony strips all non-digit characters and returns digits only.
// Exposed so the service layer can use the same normalization.
func NormalizePhoneForTelephony(phone string) string {
	if strings.TrimSpace(phone) == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range phone {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CreateCall inserts a new telephony_calls row and returns the new ID.
func (r *telephonyRepository) CreateCall(ctx context.Context, call *models.TelephonyCall) (int64, error) {
	raw := call.RawPayload
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	const q = `
		INSERT INTO telephony_calls
			(provider, external_call_id, direction, status, phone, normalized_phone,
			 client_id, lead_id, manager_id, branch_id,
			 started_at, answered_at, ended_at, duration_seconds, recording_url,
			 raw_payload, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6,
			 $7, $8, $9, $10,
			 $11, $12, $13, $14, $15,
			 $16::jsonb, NOW(), NOW())
		RETURNING id
	`
	var id int64
	err := r.db.QueryRowContext(ctx, q,
		call.Provider,
		call.ExternalCallID,
		call.Direction,
		call.Status,
		call.Phone,
		call.NormalizedPhone,
		call.ClientID,
		call.LeadID,
		call.ManagerID,
		call.BranchID,
		call.StartedAt,
		call.AnsweredAt,
		call.EndedAt,
		call.DurationSeconds,
		call.RecordingURL,
		string(raw),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("telephony: create call: %w", err)
	}
	return id, nil
}

// UpsertCall inserts or updates a call by (provider, external_call_id).
// Returns (id, isNew, error). If external_call_id is nil, always inserts.
func (r *telephonyRepository) UpsertCall(ctx context.Context, call *models.TelephonyCall) (int64, bool, error) {
	if call.ExternalCallID == nil || strings.TrimSpace(*call.ExternalCallID) == "" {
		id, err := r.CreateCall(ctx, call)
		return id, err == nil, err
	}

	raw := call.RawPayload
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}

	const q = `
		INSERT INTO telephony_calls
			(provider, external_call_id, direction, status, phone, normalized_phone,
			 client_id, lead_id, manager_id, branch_id,
			 started_at, answered_at, ended_at, duration_seconds, recording_url,
			 raw_payload, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6,
			 $7, $8, $9, $10,
			 $11, $12, $13, $14, $15,
			 $16::jsonb, NOW(), NOW())
		ON CONFLICT (provider, external_call_id)
		WHERE external_call_id IS NOT NULL
		DO UPDATE SET
			direction        = EXCLUDED.direction,
			status           = EXCLUDED.status,
			phone            = EXCLUDED.phone,
			normalized_phone = EXCLUDED.normalized_phone,
			client_id        = COALESCE(EXCLUDED.client_id, telephony_calls.client_id),
			lead_id          = COALESCE(EXCLUDED.lead_id,   telephony_calls.lead_id),
			manager_id       = COALESCE(EXCLUDED.manager_id, telephony_calls.manager_id),
			branch_id        = COALESCE(EXCLUDED.branch_id,  telephony_calls.branch_id),
			answered_at      = COALESCE(EXCLUDED.answered_at, telephony_calls.answered_at),
			ended_at         = COALESCE(EXCLUDED.ended_at,   telephony_calls.ended_at),
			duration_seconds = COALESCE(EXCLUDED.duration_seconds, telephony_calls.duration_seconds),
			recording_url    = COALESCE(EXCLUDED.recording_url, telephony_calls.recording_url),
			raw_payload      = EXCLUDED.raw_payload,
			updated_at       = NOW()
		RETURNING id, (xmax = 0) AS is_new
	`
	var id int64
	var isNew bool
	err := r.db.QueryRowContext(ctx, q,
		call.Provider,
		call.ExternalCallID,
		call.Direction,
		call.Status,
		call.Phone,
		call.NormalizedPhone,
		call.ClientID,
		call.LeadID,
		call.ManagerID,
		call.BranchID,
		call.StartedAt,
		call.AnsweredAt,
		call.EndedAt,
		call.DurationSeconds,
		call.RecordingURL,
		string(raw),
	).Scan(&id, &isNew)
	if err != nil {
		return 0, false, fmt.Errorf("telephony: upsert call: %w", err)
	}
	return id, isNew, nil
}

// GetByID fetches a single call with joined client/lead/manager names.
func (r *telephonyRepository) GetByID(ctx context.Context, id int64) (*models.TelephonyCallResponse, error) {
	const q = `
		SELECT
			tc.id, tc.provider, tc.external_call_id, tc.direction, tc.status,
			tc.phone, tc.normalized_phone,
			tc.client_id, tc.lead_id, tc.manager_id, tc.branch_id,
			tc.started_at, tc.answered_at, tc.ended_at, tc.duration_seconds,
			tc.recording_url, tc.raw_payload, tc.created_at, tc.updated_at,
			COALESCE(c.display_name, '')    AS client_name,
			COALESCE(l.title, '')           AS lead_title,
			COALESCE(u.full_name, u.email, '') AS manager_name
		FROM telephony_calls tc
		LEFT JOIN clients c ON c.id = tc.client_id
		LEFT JOIN leads l   ON l.id = tc.lead_id
		LEFT JOIN users u   ON u.id = tc.manager_id
		WHERE tc.id = $1
	`
	return scanTelephonyCallResponse(r.db.QueryRowContext(ctx, q, id))
}

// FindByExternalCallID finds a call by its provider+externalCallID.
func (r *telephonyRepository) FindByExternalCallID(ctx context.Context, provider, externalCallID string) (*models.TelephonyCall, error) {
	const q = `
		SELECT id, provider, external_call_id, direction, status, phone, normalized_phone,
		       client_id, lead_id, manager_id, branch_id,
		       started_at, answered_at, ended_at, duration_seconds, recording_url,
		       raw_payload, created_at, updated_at
		FROM telephony_calls
		WHERE provider = $1 AND external_call_id = $2
		LIMIT 1
	`
	return scanTelephonyCall(r.db.QueryRowContext(ctx, q, provider, externalCallID))
}

// List returns calls filtered by TelephonyCallListFilter with pagination.
func (r *telephonyRepository) List(ctx context.Context, filter models.TelephonyCallListFilter) ([]*models.TelephonyCallResponse, int, error) {
	where, args := buildWhereClause(filter)
	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM telephony_calls tc %s`, where)
	var total int
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("telephony: list count: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	q := fmt.Sprintf(`
		SELECT
			tc.id, tc.provider, tc.external_call_id, tc.direction, tc.status,
			tc.phone, tc.normalized_phone,
			tc.client_id, tc.lead_id, tc.manager_id, tc.branch_id,
			tc.started_at, tc.answered_at, tc.ended_at, tc.duration_seconds,
			tc.recording_url, tc.raw_payload, tc.created_at, tc.updated_at,
			COALESCE(c.display_name, '')       AS client_name,
			COALESCE(l.title, '')              AS lead_title,
			COALESCE(u.full_name, u.email, '') AS manager_name
		FROM telephony_calls tc
		LEFT JOIN clients c ON c.id = tc.client_id
		LEFT JOIN leads l   ON l.id = tc.lead_id
		LEFT JOIN users u   ON u.id = tc.manager_id
		%s
		ORDER BY tc.started_at DESC NULLS LAST, tc.id DESC
		LIMIT $%d OFFSET $%d
	`, where, len(args)-1, len(args))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("telephony: list: %w", err)
	}
	defer rows.Close()
	items, _, err := scanTelephonyCallRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListByClient returns calls for a given client.
func (r *telephonyRepository) ListByClient(ctx context.Context, clientID int64, limit, offset int) ([]*models.TelephonyCallResponse, int, error) {
	filter := models.TelephonyCallListFilter{ClientID: &clientID, Limit: limit, Offset: offset}
	return r.List(ctx, filter)
}

// ListByLead returns calls for a given lead.
func (r *telephonyRepository) ListByLead(ctx context.Context, leadID int64, limit, offset int) ([]*models.TelephonyCallResponse, int, error) {
	filter := models.TelephonyCallListFilter{LeadID: &leadID, Limit: limit, Offset: offset}
	return r.List(ctx, filter)
}

// LinkToClient sets client_id for a given call.
func (r *telephonyRepository) LinkToClient(ctx context.Context, callID, clientID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE telephony_calls SET client_id = $1, updated_at = NOW() WHERE id = $2`,
		clientID, callID)
	return err
}

// LinkToLead sets lead_id for a given call.
func (r *telephonyRepository) LinkToLead(ctx context.Context, callID, leadID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE telephony_calls SET lead_id = $1, updated_at = NOW() WHERE id = $2`,
		leadID, callID)
	return err
}

// FindClientByPhone searches clients by normalized phone (digits only).
func (r *telephonyRepository) FindClientByPhone(ctx context.Context, normalizedPhone string) (int64, error) {
	if strings.TrimSpace(normalizedPhone) == "" {
		return 0, nil
	}
	const q = `
		SELECT id
		FROM clients
		WHERE regexp_replace(COALESCE(primary_phone, phone, ''), '\D', '', 'g') = $1
		ORDER BY id
		LIMIT 1
	`
	var id int64
	err := r.db.QueryRowContext(ctx, q, normalizedPhone).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("telephony: find client by phone: %w", err)
	}
	return id, nil
}

// FindLeadByPhone searches leads by normalized phone.
func (r *telephonyRepository) FindLeadByPhone(ctx context.Context, normalizedPhone string) (int64, error) {
	if strings.TrimSpace(normalizedPhone) == "" {
		return 0, nil
	}
	const q = `
		SELECT id
		FROM leads
		WHERE phone = $1
		ORDER BY id
		LIMIT 1
	`
	var id int64
	err := r.db.QueryRowContext(ctx, q, normalizedPhone).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("telephony: find lead by phone: %w", err)
	}
	return id, nil
}

// CreateLeadFromCall creates a new lead for an inbound call from an unknown number.
// owner_id is always resolved to a real user (manager, else fallback admin) so the
// lead is never lost; department_id is inherited from that owner's profile (A2:
// avoids NULL-department leads leaking across departments).
func (r *telephonyRepository) CreateLeadFromCall(ctx context.Context, phone, normalizedPhone string, ownerID *int, branchID *int) (int64, error) {
	displayPhone := phone
	if normalizedPhone != "" {
		displayPhone = normalizedPhone
	}
	title := fmt.Sprintf("Входящий звонок +%s", displayPhone)

	preferredOwner := 0
	if ownerID != nil {
		preferredOwner = *ownerID
	}
	resolvedOwner, err := resolveAutoLeadOwner(ctx, r.db, preferredOwner)
	if err != nil {
		return 0, fmt.Errorf("telephony: create lead from call: %w", err)
	}

	const q = `
		INSERT INTO leads (title, phone, source, owner_id, branch_id, department_id, status)
		VALUES ($1, $2, 'binotel', $3,
			COALESCE($4, (SELECT branch_id FROM users WHERE id = $3)),
			(SELECT department_id FROM users WHERE id = $3),
			'new')
		RETURNING id
	`
	var leadID int64
	if err := r.db.QueryRowContext(ctx, q, title, normalizedPhone, resolvedOwner, branchID).Scan(&leadID); err != nil {
		return 0, fmt.Errorf("telephony: create lead from call: %w", err)
	}
	return leadID, nil
}

// FindManagerByExtension looks up a user whose extension/phone matches.
// Returns zero values if not found.
func (r *telephonyRepository) FindManagerByExtension(ctx context.Context, extension string) (int, int, error) {
	if strings.TrimSpace(extension) == "" {
		return 0, 0, nil
	}
	normalized := NormalizePhoneForTelephony(extension)
	const q = `
		SELECT u.id, COALESCE(u.branch_id, 0)
		FROM users u
		WHERE regexp_replace(COALESCE(u.phone, ''), '\D', '', 'g') = $1
		   OR regexp_replace(COALESCE(u.phone, ''), '\D', '', 'g') = $2
		LIMIT 1
	`
	var userID, branchID int
	err := r.db.QueryRowContext(ctx, q, extension, normalized).Scan(&userID, &branchID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("telephony: find manager by extension: %w", err)
	}
	return userID, branchID, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func buildWhereClause(f models.TelephonyCallListFilter) (string, []any) {
	var conditions []string
	var args []any
	idx := 1

	add := func(cond string, val any) {
		conditions = append(conditions, fmt.Sprintf(cond, idx))
		args = append(args, val)
		idx++
	}

	if f.ClientID != nil {
		add("tc.client_id = $%d", *f.ClientID)
	}
	if f.LeadID != nil {
		add("tc.lead_id = $%d", *f.LeadID)
	}
	if f.ManagerID != nil {
		add("tc.manager_id = $%d", *f.ManagerID)
	}
	if f.BranchID != nil {
		add("tc.branch_id = $%d", *f.BranchID)
	}
	if f.Status != "" {
		add("tc.status = $%d", f.Status)
	}
	if f.Phone != "" {
		add("tc.normalized_phone = $%d", NormalizePhoneForTelephony(f.Phone))
	}
	if f.DateFrom != nil {
		add("tc.started_at >= $%d", *f.DateFrom)
	}
	if f.DateTo != nil {
		add("tc.started_at <= $%d", *f.DateTo)
	}

	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func scanTelephonyCall(row *sql.Row) (*models.TelephonyCall, error) {
	var c models.TelephonyCall
	var raw []byte
	err := row.Scan(
		&c.ID, &c.Provider, &c.ExternalCallID, &c.Direction, &c.Status,
		&c.Phone, &c.NormalizedPhone,
		&c.ClientID, &c.LeadID, &c.ManagerID, &c.BranchID,
		&c.StartedAt, &c.AnsweredAt, &c.EndedAt, &c.DurationSeconds,
		&c.RecordingURL, &raw, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("telephony: scan call: %w", err)
	}
	c.RawPayload = append(json.RawMessage(nil), raw...)
	return &c, nil
}

func scanTelephonyCallResponse(row *sql.Row) (*models.TelephonyCallResponse, error) {
	var r models.TelephonyCallResponse
	var raw []byte
	var clientName, leadTitle, managerName string
	err := row.Scan(
		&r.ID, &r.Provider, &r.ExternalCallID, &r.Direction, &r.Status,
		&r.Phone, &r.NormalizedPhone,
		&r.ClientID, &r.LeadID, &r.ManagerID, &r.BranchID,
		&r.StartedAt, &r.AnsweredAt, &r.EndedAt, &r.DurationSeconds,
		&r.RecordingURL, &raw, &r.CreatedAt, &r.UpdatedAt,
		&clientName, &leadTitle, &managerName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("telephony: scan call response: %w", err)
	}
	r.RawPayload = append(json.RawMessage(nil), raw...)
	if clientName != "" {
		r.ClientName = &clientName
	}
	if leadTitle != "" {
		r.LeadTitle = &leadTitle
	}
	if managerName != "" {
		r.ManagerName = &managerName
	}
	return &r, nil
}

func scanTelephonyCallRows(rows *sql.Rows) ([]*models.TelephonyCallResponse, int, error) {
	var out []*models.TelephonyCallResponse
	for rows.Next() {
		var r models.TelephonyCallResponse
		var raw []byte
		var clientName, leadTitle, managerName string
		err := rows.Scan(
			&r.ID, &r.Provider, &r.ExternalCallID, &r.Direction, &r.Status,
			&r.Phone, &r.NormalizedPhone,
			&r.ClientID, &r.LeadID, &r.ManagerID, &r.BranchID,
			&r.StartedAt, &r.AnsweredAt, &r.EndedAt, &r.DurationSeconds,
			&r.RecordingURL, &raw, &r.CreatedAt, &r.UpdatedAt,
			&clientName, &leadTitle, &managerName,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("telephony: scan row: %w", err)
		}
		r.RawPayload = append(json.RawMessage(nil), raw...)
		if clientName != "" {
			r.ClientName = &clientName
		}
		if leadTitle != "" {
			r.LeadTitle = &leadTitle
		}
		if managerName != "" {
			r.ManagerName = &managerName
		}
		out = append(out, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	// Return slice length as total when called from within List (which already has the real total)
	return out, len(out), nil
}

// unused import guard
var _ = time.Now
