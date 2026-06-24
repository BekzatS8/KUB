package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"turcompany/internal/authz"
	binotelclient "turcompany/internal/integrations/binotel"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// userBranchLookup is a minimal interface for resolving a user's branch_id.
// Satisfied by repositories.UserRepository.
type userBranchLookup interface {
	GetByID(id int) (*models.User, error)
}

// clientAccessChecker (declared in client_files_service.go) resolves whether the
// caller may access a specific client via the canonical client-scope rules.
// Satisfied by *ClientService.GetByID (resolveClientScope + clientMatchesScope).

// leadAccessChecker resolves whether the caller may access a specific lead,
// using the canonical lead-scope rules. Satisfied by *LeadService.GetByID,
// which applies resolveLeadScope + leadMatchesScope and returns ErrForbidden
// (or a nil lead) when the caller has no access.
type leadAccessChecker interface {
	GetByID(id, userID, roleID int) (*models.Leads, error)
}

// TelephonyService handles business logic for telephony / Binotel integration.
type TelephonyService struct {
	repo          repositories.TelephonyRepository
	userRepo      userBranchLookup
	clientAccess  clientAccessChecker
	leadAccess    leadAccessChecker
	secret        string // BINOTEL_WEBHOOK_SECRET — empty means no check in dev
	binotelClient *binotelclient.Client
	audit         *AuditService
}

// NewTelephonyService constructs a TelephonyService.
func NewTelephonyService(repo repositories.TelephonyRepository, userRepo userBranchLookup, webhookSecret string) *TelephonyService {
	return &TelephonyService{repo: repo, userRepo: userRepo, secret: webhookSecret}
}

// SetAccessCheckers wires the per-entity access checkers used to enforce client/
// lead ownership on the call-history endpoints. Kept separate from the constructor
// so wiring stays additive (ClientService/LeadService are built after telephony).
func (s *TelephonyService) SetAccessCheckers(clientAccess clientAccessChecker, leadAccess leadAccessChecker) {
	s.clientAccess = clientAccess
	s.leadAccess = leadAccess
}

// SetBinotelClient wires the Binotel REST API client used for outgoing calls.
func (s *TelephonyService) SetBinotelClient(c *binotelclient.Client) { s.binotelClient = c }

// SetAuditService wires the audit logger for feed events (telephony.call.*).
func (s *TelephonyService) SetAuditService(audit *AuditService) { s.audit = audit }

// WebhookSecret returns the configured secret (used by the handler for validation).
func (s *TelephonyService) WebhookSecret() string { return s.secret }

// InitiateCall asks Binotel to ring internalNumber (manager's extension) and connect
// it to externalPhone (client/lead number). The manager's extension is read from
// users.phone in CRM — make sure managers have their Binotel extension set there.
// managerID overrides the caller's own user; only admin/management may use it.
func (s *TelephonyService) InitiateCall(ctx context.Context, callerUserID, callerRoleID int, externalPhone string, managerID *int) (string, error) {
	if s.binotelClient == nil || !s.binotelClient.IsConfigured() {
		return "", fmt.Errorf("telephony: binotel client not configured (set BINOTEL_API_KEY + BINOTEL_API_SECRET)")
	}

	// Resolve which manager's extension to dial from.
	resolvedManagerID := callerUserID
	if managerID != nil && *managerID > 0 && *managerID != callerUserID {
		if callerRoleID != authz.RoleSystemAdmin && callerRoleID != authz.RoleManagement {
			return "", ErrForbidden
		}
		resolvedManagerID = *managerID
	}

	if s.userRepo == nil {
		return "", fmt.Errorf("telephony: user repo not configured")
	}
	user, err := s.userRepo.GetByID(resolvedManagerID)
	if err != nil {
		return "", fmt.Errorf("telephony: lookup manager: %w", err)
	}
	if user == nil {
		return "", fmt.Errorf("telephony: manager id=%d not found", resolvedManagerID)
	}
	internalNumber := strings.TrimSpace(user.Phone)
	if internalNumber == "" {
		return "", fmt.Errorf("telephony: manager id=%d has no phone/extension configured in CRM", resolvedManagerID)
	}

	normalizedPhone := repositories.NormalizePhoneForTelephony(externalPhone)
	if normalizedPhone == "" {
		return "", fmt.Errorf("telephony: invalid external phone %q", externalPhone)
	}

	result, err := s.binotelClient.MakeCall(ctx, internalNumber, normalizedPhone)
	if err != nil {
		return "", fmt.Errorf("telephony: initiate call: %w", err)
	}
	log.Printf(
		"integration=binotel operation=make_call manager_id=%d internal=%s external=%s general_call_id=%s",
		resolvedManagerID, maskStr(internalNumber), maskPhone(normalizedPhone), maskStr(result.GeneralCallID),
	)
	return result.GeneralCallID, nil
}

// HandleBinotelWebhook processes an inbound Binotel webhook payload.
// It is idempotent: repeated delivery of the same external_call_id is a no-op.
func (s *TelephonyService) HandleBinotelWebhook(ctx context.Context, rawBody []byte) (callID int64, isNew bool, err error) {
	// 1. Parse payload best-effort — never reject on unknown structure.
	var p models.BinotelWebhookPayload
	if jsonErr := json.Unmarshal(rawBody, &p); jsonErr != nil {
		log.Printf("integration=binotel operation=webhook status=warn parse_error=%v", jsonErr)
		// Still save with status=unknown
		p = models.BinotelWebhookPayload{}
	}

	// 2. Extract fields.
	externalCallID := strings.TrimSpace(coalesceStr(p.GeneralCallID, p.CallID))
	phone := strings.TrimSpace(p.ExternalNumber)
	internalExt := strings.TrimSpace(coalesceStr(p.EmployeePhone, p.InternalNumber, p.EmployeeID))

	direction := resolveDirection(p)
	status := resolveStatus(p)

	normalizedPhone := repositories.NormalizePhoneForTelephony(phone)

	startedAt := parseUnixTime(p.StartTime)
	answeredAt := parseUnixTime(p.AnswTime)
	endedAt := parseUnixTime(p.EndTime)
	duration := parseDuration(p.Duration)
	recordingURL := strings.TrimSpace(p.RecordURL)

	log.Printf(
		"integration=binotel operation=webhook external_call_id=%s direction=%s status=%s phone=%s",
		maskStr(externalCallID), direction, status, maskPhone(normalizedPhone),
	)

	// 3. Find manager by internal extension.
	var managerID *int
	var managerBranchID *int
	if internalExt != "" {
		uid, bid, mErr := s.repo.FindManagerByExtension(ctx, internalExt)
		if mErr != nil {
			log.Printf("integration=binotel operation=webhook status=warn find_manager error=%v", mErr)
		} else if uid > 0 {
			managerID = &uid
			if bid > 0 {
				managerBranchID = &bid
			}
		}
	}

	// 4. Build call record.
	var extCallIDPtr *string
	if externalCallID != "" {
		extCallIDPtr = &externalCallID
	}
	var normPhonePtr *string
	if normalizedPhone != "" {
		normPhonePtr = &normalizedPhone
	}
	var recURLPtr *string
	if recordingURL != "" {
		recURLPtr = &recordingURL
	}

	call := &models.TelephonyCall{
		Provider:        "binotel",
		ExternalCallID:  extCallIDPtr,
		Direction:       direction,
		Status:          status,
		Phone:           phone,
		NormalizedPhone: normPhonePtr,
		ManagerID:       managerID,
		BranchID:        managerBranchID,
		StartedAt:       startedAt,
		AnsweredAt:      answeredAt,
		EndedAt:         endedAt,
		DurationSeconds: duration,
		RecordingURL:    recURLPtr,
		RawPayload:      rawBody,
	}

	// 5. Link to client or lead.
	if normalizedPhone != "" {
		clientID, cErr := s.repo.FindClientByPhone(ctx, normalizedPhone)
		if cErr != nil {
			log.Printf("integration=binotel operation=webhook status=warn find_client error=%v", cErr)
		}
		if clientID > 0 {
			call.ClientID = &clientID
			log.Printf("integration=binotel operation=webhook linked_client_id=%d", clientID)
		} else {
			// Search by lead.
			leadID, lErr := s.repo.FindLeadByPhone(ctx, normalizedPhone)
			if lErr != nil {
				log.Printf("integration=binotel operation=webhook status=warn find_lead error=%v", lErr)
			}
			if leadID > 0 {
				call.LeadID = &leadID
				log.Printf("integration=binotel operation=webhook linked_lead_id=%d", leadID)
			} else if direction == models.CallDirectionInbound {
				// 6. Auto-create lead for unknown inbound numbers.
				newLeadID, lcErr := s.repo.CreateLeadFromCall(ctx, phone, normalizedPhone, managerID, managerBranchID)
				if lcErr != nil {
					log.Printf("integration=binotel operation=webhook status=warn create_lead error=%v", lcErr)
				} else if newLeadID > 0 {
					call.LeadID = &newLeadID
					log.Printf("integration=binotel operation=webhook created_lead_id=%d", newLeadID)
				}
			}
		}
	}

	// 7. Upsert — idempotent by (provider, external_call_id).
	callID, isNew, err = s.repo.UpsertCall(ctx, call)
	if err != nil {
		return 0, false, fmt.Errorf("telephony: handle webhook: %w", err)
	}
	log.Printf("integration=binotel operation=webhook status=ok call_id=%d is_new=%v", callID, isNew)

	// 8. Feed event — write to audit log so the call appears in the activity feed.
	if s.audit != nil {
		action := "telephony.call.updated"
		if isNew {
			action = "telephony.call.received"
		}
		meta := map[string]any{
			"direction": call.Direction,
			"status":    call.Status,
			"phone":     phone,
		}
		if call.LeadID != nil {
			meta["lead_id"] = *call.LeadID
		}
		if call.ClientID != nil {
			meta["client_id"] = *call.ClientID
		}
		if call.ManagerID != nil {
			meta["manager_id"] = *call.ManagerID
		}
		s.audit.Log(ctx, AuditEvent{
			ActorUserID: call.ManagerID,
			Action:      action,
			EntityType:  "telephony_call",
			EntityID:    fmt.Sprintf("%d", callID),
			Meta:        meta,
		})
	}

	return callID, isNew, nil
}

// ListCalls returns a paginated list of calls with branch scope enforcement.
// admin/management see all calls; all other roles are scoped to their own branch.
func (s *TelephonyService) ListCalls(ctx context.Context, userID, roleID int, filter models.TelephonyCallListFilter) ([]*models.TelephonyCallResponse, int, error) {
	branchID, err := s.branchScopeForRole(userID, roleID)
	if err != nil {
		return nil, 0, err
	}
	if branchID != nil {
		filter.BranchID = branchID
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	return s.repo.List(ctx, filter)
}

// branchScopeForRole returns the branch_id to filter by, or nil for full access.
func (s *TelephonyService) branchScopeForRole(userID, roleID int) (*int, error) {
	switch roleID {
	case authz.RoleManagement, authz.RoleSystemAdmin:
		return nil, nil // full access — all branches
	default:
		if s.userRepo == nil {
			return nil, fmt.Errorf("telephony: user repo not configured for branch scoping")
		}
		u, err := s.userRepo.GetByID(userID)
		if err != nil {
			return nil, fmt.Errorf("telephony: resolve branch scope: %w", err)
		}
		if u == nil || u.BranchID == nil {
			// Fail-closed: a scoped role without a resolvable branch must NOT fall back
			// to all-branch access (that would leak every branch's calls). Surface the
			// misconfiguration as an error (→ 500) instead of silently widening scope.
			return nil, fmt.Errorf("telephony: user %d has no branch_id (scoped role cannot resolve branch)", userID)
		}
		return u.BranchID, nil
	}
}

// GetCall returns a single call by ID with branch scope enforcement.
// Scoped roles (non-admin/management) get ErrForbidden if the call belongs to a different branch.
func (s *TelephonyService) GetCall(ctx context.Context, userID, roleID int, id int64) (*models.TelephonyCallResponse, error) {
	call, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if call == nil {
		return nil, nil
	}
	branchID, err := s.branchScopeForRole(userID, roleID)
	if err != nil {
		return nil, err
	}
	if branchID != nil {
		if call.BranchID == nil || *call.BranchID != *branchID {
			return nil, ErrForbidden
		}
	}
	return call, nil
}

// ListClientCalls returns calls linked to a client, scoped to the caller's branch.
//
// Ownership is enforced FIRST: the caller must be able to access this specific
// client under the canonical client-scope rules (same check as GET /clients/:id).
// This blocks an own-scoped role (partner) — or any role — from reading the call
// history of a client they don't own/can't see by guessing the client_id.
func (s *TelephonyService) ListClientCalls(ctx context.Context, userID, roleID int, clientID int64, limit, offset int) ([]*models.TelephonyCallResponse, int, error) {
	if err := s.ensureClientAccess(userID, roleID, clientID); err != nil {
		return nil, 0, err
	}

	branchID, err := s.branchScopeForRole(userID, roleID)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	filter := models.TelephonyCallListFilter{
		ClientID: &clientID,
		Limit:    limit,
		Offset:   offset,
	}
	if branchID != nil {
		filter.BranchID = branchID
	}
	return s.repo.List(ctx, filter)
}

// ListLeadCalls returns calls linked to a lead, scoped to the caller's branch.
//
// Ownership is enforced FIRST via the canonical lead-scope rules (same check as
// GET /leads/:id) — see ListClientCalls.
func (s *TelephonyService) ListLeadCalls(ctx context.Context, userID, roleID int, leadID int64, limit, offset int) ([]*models.TelephonyCallResponse, int, error) {
	if err := s.ensureLeadAccess(userID, roleID, leadID); err != nil {
		return nil, 0, err
	}

	branchID, err := s.branchScopeForRole(userID, roleID)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	filter := models.TelephonyCallListFilter{
		LeadID: &leadID,
		Limit:  limit,
		Offset: offset,
	}
	if branchID != nil {
		filter.BranchID = branchID
	}
	return s.repo.List(ctx, filter)
}

// ensureClientAccess returns nil only when the caller may access this client
// under the canonical client-scope rules. Fails closed: a missing checker, an
// out-of-scope client (nil), or ErrForbidden all deny access. A non-scope error
// (e.g. DB failure) is propagated as-is.
func (s *TelephonyService) ensureClientAccess(userID, roleID int, clientID int64) error {
	if s.clientAccess == nil {
		return ErrForbidden
	}
	client, err := s.clientAccess.GetByID(int(clientID), userID, roleID)
	if err != nil {
		return err
	}
	if client == nil {
		return ErrForbidden
	}
	return nil
}

// ensureLeadAccess is the lead counterpart of ensureClientAccess.
func (s *TelephonyService) ensureLeadAccess(userID, roleID int, leadID int64) error {
	if s.leadAccess == nil {
		return ErrForbidden
	}
	lead, err := s.leadAccess.GetByID(int(leadID), userID, roleID)
	if err != nil {
		return err
	}
	if lead == nil {
		return ErrForbidden
	}
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func resolveDirection(p models.BinotelWebhookPayload) string {
	if p.CallType != nil {
		switch v := p.CallType.(type) {
		case float64:
			if int(v) == 0 {
				return models.CallDirectionInbound
			}
			return models.CallDirectionOutbound
		case string:
			lower := strings.ToLower(v)
			if lower == "0" || lower == "inbound" || lower == "in" {
				return models.CallDirectionInbound
			}
			return models.CallDirectionOutbound
		}
	}
	// event_type hint
	switch strings.ToLower(p.EventType) {
	case "incoming_call", "missed_call", "inbound":
		return models.CallDirectionInbound
	case "outgoing_call", "outbound":
		return models.CallDirectionOutbound
	}
	return models.CallDirectionInbound // safe default
}

func resolveStatus(p models.BinotelWebhookPayload) string {
	disp := strings.ToLower(strings.TrimSpace(p.Disposition))
	switch disp {
	case "answered":
		return models.CallStatusAnswered
	case "noanswer", "no answer", "no_answer":
		return models.CallStatusMissed
	case "busy":
		return models.CallStatusMissed
	case "failed":
		return models.CallStatusFailed
	case "completed":
		return models.CallStatusCompleted
	}
	evt := strings.ToLower(strings.TrimSpace(p.EventType))
	switch evt {
	case "call_start", "incoming_call", "outgoing_call":
		return models.CallStatusIncoming
	case "call_answer":
		return models.CallStatusAnswered
	case "call_end":
		return models.CallStatusCompleted
	case "missed_call":
		return models.CallStatusMissed
	}
	return models.CallStatusUnknown
}

func parseUnixTime(v interface{}) *time.Time {
	if v == nil {
		return nil
	}
	var sec int64
	switch val := v.(type) {
	case float64:
		sec = int64(val)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			return nil
		}
		sec = parsed
	default:
		return nil
	}
	if sec <= 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}

func parseDuration(v interface{}) *int {
	if v == nil {
		return nil
	}
	var d int
	switch val := v.(type) {
	case float64:
		d = int(val)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return nil
		}
		d = parsed
	default:
		return nil
	}
	if d < 0 {
		return nil
	}
	return &d
}

func coalesceStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func maskStr(s string) string {
	if len(s) <= 4 {
		return "***"
	}
	return s[:4] + "***"
}

func maskPhone(s string) string {
	if len(s) <= 4 {
		return "***"
	}
	return s[:len(s)-4] + "****"
}

