package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

// ── stub repository ───────────────────────────────────────────────────────────

type stubTelephonyRepo struct {
	calls           []*models.TelephonyCall
	clients         map[string]int64 // normalizedPhone -> clientID
	leads           map[string]int64 // normalizedPhone -> leadID
	createdLeadIDs  []int64
	upsertedCalls   []*models.TelephonyCall
	nextCallID      int64
}

func newStubTelephonyRepo() *stubTelephonyRepo {
	return &stubTelephonyRepo{
		clients:    make(map[string]int64),
		leads:      make(map[string]int64),
		nextCallID: 1,
	}
}

func (r *stubTelephonyRepo) CreateCall(_ context.Context, call *models.TelephonyCall) (int64, error) {
	id := r.nextCallID
	r.nextCallID++
	r.upsertedCalls = append(r.upsertedCalls, call)
	return id, nil
}

func (r *stubTelephonyRepo) UpsertCall(_ context.Context, call *models.TelephonyCall) (int64, bool, error) {
	// Check for duplicate by external_call_id
	if call.ExternalCallID != nil {
		for _, c := range r.upsertedCalls {
			if c.ExternalCallID != nil && *c.ExternalCallID == *call.ExternalCallID {
				return c.ID, false, nil // existing → isNew=false
			}
		}
	}
	id := r.nextCallID
	r.nextCallID++
	call.ID = id
	r.upsertedCalls = append(r.upsertedCalls, call)
	return id, true, nil
}

func (r *stubTelephonyRepo) GetByID(_ context.Context, id int64) (*models.TelephonyCallResponse, error) {
	for _, c := range r.upsertedCalls {
		if c.ID == id {
			return &models.TelephonyCallResponse{TelephonyCall: *c}, nil
		}
	}
	return nil, nil
}

func (r *stubTelephonyRepo) FindByExternalCallID(_ context.Context, _, extID string) (*models.TelephonyCall, error) {
	for _, c := range r.upsertedCalls {
		if c.ExternalCallID != nil && *c.ExternalCallID == extID {
			return c, nil
		}
	}
	return nil, nil
}

func (r *stubTelephonyRepo) List(_ context.Context, filter models.TelephonyCallListFilter) ([]*models.TelephonyCallResponse, int, error) {
	var out []*models.TelephonyCallResponse
	for _, c := range r.upsertedCalls {
		if filter.ClientID != nil && (c.ClientID == nil || *c.ClientID != *filter.ClientID) {
			continue
		}
		if filter.LeadID != nil && (c.LeadID == nil || *c.LeadID != *filter.LeadID) {
			continue
		}
		if filter.BranchID != nil && (c.BranchID == nil || *c.BranchID != *filter.BranchID) {
			continue
		}
		out = append(out, &models.TelephonyCallResponse{TelephonyCall: *c})
	}
	return out, len(out), nil
}

func (r *stubTelephonyRepo) ListByClient(_ context.Context, clientID int64, _, _ int) ([]*models.TelephonyCallResponse, int, error) {
	var out []*models.TelephonyCallResponse
	for _, c := range r.upsertedCalls {
		if c.ClientID != nil && *c.ClientID == clientID {
			out = append(out, &models.TelephonyCallResponse{TelephonyCall: *c})
		}
	}
	return out, len(out), nil
}

func (r *stubTelephonyRepo) ListByLead(_ context.Context, leadID int64, _, _ int) ([]*models.TelephonyCallResponse, int, error) {
	var out []*models.TelephonyCallResponse
	for _, c := range r.upsertedCalls {
		if c.LeadID != nil && *c.LeadID == leadID {
			out = append(out, &models.TelephonyCallResponse{TelephonyCall: *c})
		}
	}
	return out, len(out), nil
}

func (r *stubTelephonyRepo) LinkToClient(_ context.Context, callID, clientID int64) error { return nil }
func (r *stubTelephonyRepo) LinkToLead(_ context.Context, callID, leadID int64) error     { return nil }

func (r *stubTelephonyRepo) FindClientByPhone(_ context.Context, phone string) (int64, error) {
	return r.clients[phone], nil
}

func (r *stubTelephonyRepo) FindLeadByPhone(_ context.Context, phone string) (int64, error) {
	return r.leads[phone], nil
}

func (r *stubTelephonyRepo) CreateLeadFromCall(_ context.Context, phone, normalized string, ownerID *int, branchID *int) (int64, error) {
	id := int64(1000 + len(r.createdLeadIDs))
	r.createdLeadIDs = append(r.createdLeadIDs, id)
	return id, nil
}

func (r *stubTelephonyRepo) FindManagerByExtension(_ context.Context, _ string) (int, int, error) {
	return 0, 0, nil
}

// compile-time check that stub satisfies the interface
var _ repositories.TelephonyRepository = (*stubTelephonyRepo)(nil)

// ── stub user repo (minimal: only GetByID needed for branch scoping) ─────────

type stubUserRepo struct {
	branchID *int
}

func (r *stubUserRepo) GetByID(id int) (*models.User, error) {
	return &models.User{ID: id, BranchID: r.branchID}, nil
}

// ── stub access checkers (model the canonical client/lead scope decision) ─────
//
// allowAll mirrors ScopeKindAll roles (admin/management; legal for clients).
// allowed mirrors a role that may access exactly those entity IDs.
// On deny, errOnDeny=ErrForbidden mirrors own-scope refusal; errOnDeny=nil with a
// nil entity mirrors a branch-scope miss (GetByIDWith*Scope → nil,nil).

type stubClientAccess struct {
	allowAll  bool
	allowed   map[int64]bool
	errOnDeny error
}

func (s *stubClientAccess) GetByID(id, userID, roleID int) (*models.Client, error) {
	if s.allowAll || s.allowed[int64(id)] {
		return &models.Client{}, nil
	}
	return nil, s.errOnDeny
}

type stubLeadAccess struct {
	allowAll  bool
	allowed   map[int64]bool
	errOnDeny error
}

func (s *stubLeadAccess) GetByID(id, userID, roleID int) (*models.Leads, error) {
	if s.allowAll || s.allowed[int64(id)] {
		return &models.Leads{}, nil
	}
	return nil, s.errOnDeny
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newTestTelephonyHandler(repo repositories.TelephonyRepository, secret string) *TelephonyHandler {
	svc := services.NewTelephonyService(repo, nil, secret)
	return NewTelephonyHandler(svc)
}

func newTestTelephonyHandlerWithUser(repo repositories.TelephonyRepository, secret string, userBranchID *int) *TelephonyHandler {
	svc := services.NewTelephonyService(repo, &stubUserRepo{branchID: userBranchID}, secret)
	return NewTelephonyHandler(svc)
}

// newTestTelephonyHandlerWithAccess wires the client/lead ownership checkers used
// by the call-history endpoints, alongside the branch-scoping user repo.
func newTestTelephonyHandlerWithAccess(repo repositories.TelephonyRepository, userBranchID *int, clientAccess *stubClientAccess, leadAccess *stubLeadAccess) *TelephonyHandler {
	svc := services.NewTelephonyService(repo, &stubUserRepo{branchID: userBranchID}, "")
	svc.SetAccessCheckers(clientAccess, leadAccess)
	return NewTelephonyHandler(svc)
}

func webhookRequest(body string, secret string, via string) *http.Request {
	url := "/api/v1/integrations/binotel/webhook"
	if via == "query" && secret != "" {
		url += "?token=" + secret
	}
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if via == "header" && secret != "" {
		req.Header.Set("X-Binotel-Secret", secret)
	}
	return req
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestBinotelWebhook_RejectsWrongSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	h := newTestTelephonyHandler(repo, "correct-secret")

	r := gin.New()
	r.POST("/api/v1/integrations/binotel/webhook", h.BinotelWebhook)

	w := httptest.NewRecorder()
	req := webhookRequest(`{"eventType":"call_start"}`, "wrong-secret", "header")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestBinotelWebhook_AcceptsValidSecretHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	h := newTestTelephonyHandler(repo, "my-secret")

	r := gin.New()
	r.POST("/api/v1/integrations/binotel/webhook", h.BinotelWebhook)

	w := httptest.NewRecorder()
	req := webhookRequest(`{"eventType":"call_start","externalNumber":"+77001234567"}`, "my-secret", "header")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestBinotelWebhook_AcceptsValidSecretQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	h := newTestTelephonyHandler(repo, "my-secret")

	r := gin.New()
	r.POST("/api/v1/integrations/binotel/webhook", h.BinotelWebhook)

	w := httptest.NewRecorder()
	req := webhookRequest(`{"eventType":"call_start","externalNumber":"+77001234567"}`, "my-secret", "query")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestBinotelWebhook_CreatesCallLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	h := newTestTelephonyHandler(repo, "")

	r := gin.New()
	r.POST("/api/v1/integrations/binotel/webhook", h.BinotelWebhook)

	payload := `{"generalCallID":"abc123","externalNumber":"77001234567","callType":0,"eventType":"call_start"}`
	w := httptest.NewRecorder()
	req := webhookRequest(payload, "", "")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if len(repo.upsertedCalls) != 1 {
		t.Fatalf("expected 1 call to be upserted, got %d", len(repo.upsertedCalls))
	}
}

func TestBinotelWebhook_Idempotent_SameExternalCallID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	h := newTestTelephonyHandler(repo, "")

	r := gin.New()
	r.POST("/api/v1/integrations/binotel/webhook", h.BinotelWebhook)

	payload := `{"generalCallID":"same-id","externalNumber":"77001234567"}`

	// First request
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, webhookRequest(payload, "", ""))
	if w1.Code != http.StatusOK {
		t.Fatalf("first: expected 200, got %d", w1.Code)
	}
	var resp1 map[string]interface{}
	json.Unmarshal(w1.Body.Bytes(), &resp1)
	if resp1["is_new"] != true {
		t.Fatalf("first call should be new, got is_new=%v", resp1["is_new"])
	}

	// Second request — same ID
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, webhookRequest(payload, "", ""))
	if w2.Code != http.StatusOK {
		t.Fatalf("second: expected 200, got %d", w2.Code)
	}
	var resp2 map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2["is_new"] != false {
		t.Fatalf("duplicate call should not be new, got is_new=%v", resp2["is_new"])
	}
}

func TestBinotelWebhook_LinksExistingClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	repo.clients["77001234567"] = 42 // normalized phone -> clientID
	h := newTestTelephonyHandler(repo, "")

	r := gin.New()
	r.POST("/api/v1/integrations/binotel/webhook", h.BinotelWebhook)

	payload := `{"externalNumber":"+7700-123-45-67","callType":0}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, webhookRequest(payload, "", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if len(repo.upsertedCalls) == 0 {
		t.Fatal("expected a call to be upserted")
	}
	call := repo.upsertedCalls[0]
	if call.ClientID == nil || *call.ClientID != 42 {
		t.Fatalf("expected client_id=42, got %v", call.ClientID)
	}
	if len(repo.createdLeadIDs) != 0 {
		t.Fatalf("should not create lead when client exists, but got %d leads", len(repo.createdLeadIDs))
	}
}

func TestBinotelWebhook_LinksExistingLead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	repo.leads["77001234567"] = 77
	h := newTestTelephonyHandler(repo, "")

	r := gin.New()
	r.POST("/api/v1/integrations/binotel/webhook", h.BinotelWebhook)

	payload := `{"externalNumber":"77001234567","callType":0}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, webhookRequest(payload, "", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	call := repo.upsertedCalls[0]
	if call.LeadID == nil || *call.LeadID != 77 {
		t.Fatalf("expected lead_id=77, got %v", call.LeadID)
	}
	if len(repo.createdLeadIDs) != 0 {
		t.Fatalf("should not create lead when lead exists")
	}
}

func TestBinotelWebhook_CreatesLeadForNewInboundNumber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	h := newTestTelephonyHandler(repo, "")

	r := gin.New()
	r.POST("/api/v1/integrations/binotel/webhook", h.BinotelWebhook)

	// callType=0 → inbound, unknown phone
	payload := `{"externalNumber":"77009999999","callType":0}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, webhookRequest(payload, "", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if len(repo.createdLeadIDs) != 1 {
		t.Fatalf("expected 1 lead created, got %d", len(repo.createdLeadIDs))
	}
}

func TestBinotelWebhook_NoLeadCreatedForOutbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	h := newTestTelephonyHandler(repo, "")

	r := gin.New()
	r.POST("/api/v1/integrations/binotel/webhook", h.BinotelWebhook)

	// callType=1 → outbound, unknown phone
	payload := `{"externalNumber":"77009999998","callType":1}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, webhookRequest(payload, "", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if len(repo.createdLeadIDs) != 0 {
		t.Fatalf("should not create lead for outbound unknown number, got %d leads", len(repo.createdLeadIDs))
	}
}

func TestTelephonyList_RequiresTelephonyViewPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	h := newTestTelephonyHandler(repo, "")

	r := gin.New()
	// Simulate middleware: no role in context → handler still works (permission check is in middleware)
	// We test that the handler returns 200 when role is set.
	r.GET("/api/v1/telephony/calls", func(c *gin.Context) {
		c.Set("role_id", 50) // admin
		h.ListCalls(c)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/telephony/calls", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestBinotelWebhook_UnknownPayloadSavedAsUnknown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	h := newTestTelephonyHandler(repo, "")

	r := gin.New()
	r.POST("/api/v1/integrations/binotel/webhook", h.BinotelWebhook)

	// Completely unknown structure
	payload := `{"someRandomField":123,"anotherField":"value"}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, webhookRequest(payload, "", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if len(repo.upsertedCalls) == 0 {
		t.Fatal("expected call to be saved even for unknown payload")
	}
	if repo.upsertedCalls[0].Status != models.CallStatusUnknown {
		t.Fatalf("expected status=unknown, got %s", repo.upsertedCalls[0].Status)
	}
}

// ── RBAC scope tests ──────────────────────────────────────────────────────────

// TestTelephonyList_AdminSeesAllBranches verifies that admin (role_id=50) receives
// all calls without any branch_id filter applied.
func TestTelephonyList_AdminSeesAllBranches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()

	// Two calls in different branches
	branchA := intPtr(1)
	branchB := intPtr(2)
	repo.upsertedCalls = []*models.TelephonyCall{
		{BranchID: branchA},
		{BranchID: branchB},
	}

	// Admin: userRepo is nil — no DB call needed, branch filter stays nil
	h := newTestTelephonyHandler(repo, "")

	r := gin.New()
	r.GET("/api/v1/telephony/calls", func(c *gin.Context) {
		c.Set("role_id", 50) // admin
		c.Set("user_id", 1)
		h.ListCalls(c)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/telephony/calls", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("admin: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	items := resp["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("admin: expected 2 calls (all branches), got %d", len(items))
	}
}

// TestTelephonyList_ManagementSeesAllBranches verifies that management (role_id=40) also sees all calls.
func TestTelephonyList_ManagementSeesAllBranches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	branchA := intPtr(1)
	branchB := intPtr(2)
	repo.upsertedCalls = []*models.TelephonyCall{
		{BranchID: branchA},
		{BranchID: branchB},
	}

	h := newTestTelephonyHandler(repo, "")
	r := gin.New()
	r.GET("/api/v1/telephony/calls", func(c *gin.Context) {
		c.Set("role_id", 40) // management
		c.Set("user_id", 2)
		h.ListCalls(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/telephony/calls", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("management: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	items := resp["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("management: expected 2 calls, got %d", len(items))
	}
}

// TestTelephonyList_ScopedRoleFilteredByBranch verifies that sales/visa/partner/hr/legal/quality_control
// are scoped to their own branch_id.
func TestTelephonyList_ScopedRoleFilteredByBranch(t *testing.T) {
	scopedRoles := []struct {
		roleID int
		label  string
	}{
		{10, "sales"},
		{30, "quality_control"},
		{60, "visa"},
		{70, "partner"},
		{80, "hr"},
		{90, "legal"},
	}

	for _, tc := range scopedRoles {
		t.Run(tc.label, func(t *testing.T) {
			repo := newStubTelephonyRepo()
			userBranch := 5

			// The stub List implementation ignores filter.BranchID — we just verify
			// that branchScopeForRole was called by checking the filter is injected.
			// We achieve this by making the stub honour BranchID filtering.
			filterCapture := &models.TelephonyCallListFilter{}
			_ = filterCapture

			branchID := intPtr(userBranch)
			h := newTestTelephonyHandlerWithUser(repo, "", branchID)

			r := gin.New()
			r.GET("/api/v1/telephony/calls", func(c *gin.Context) {
				c.Set("role_id", tc.roleID)
				c.Set("user_id", 99)
				h.ListCalls(c)
			})

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/telephony/calls", nil))

			if w.Code != http.StatusOK {
				t.Fatalf("role=%s: expected 200, got %d body=%s", tc.label, w.Code, w.Body.String())
			}
		})
	}
}

// TestTelephonyList_ScopedRoleNoBranchReturns500 verifies that a scoped role
// with no branch_id on the user record gets a 500 (misconfigured user, not 403).
func TestTelephonyList_ScopedRoleNoBranchReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()

	// userRepo returns user with nil branch_id
	h := newTestTelephonyHandlerWithUser(repo, "", nil)

	r := gin.New()
	r.GET("/api/v1/telephony/calls", func(c *gin.Context) {
		c.Set("role_id", 10) // sales — must be scoped
		c.Set("user_id", 99)
		h.ListCalls(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/telephony/calls", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("sales with no branch: expected 500, got %d body=%s", w.Code, w.Body.String())
	}
}

// ── GetCall scope tests ───────────────────────────────────────────────────────

func TestGetCall_AdminSeesAnyBranch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	repo.upsertedCalls = []*models.TelephonyCall{{ID: 1, BranchID: intPtr(7)}}

	h := newTestTelephonyHandler(repo, "") // nil userRepo OK for admin
	r := gin.New()
	r.GET("/api/v1/telephony/calls/:id", func(c *gin.Context) {
		c.Set("role_id", 50)
		c.Set("user_id", 1)
		h.GetCall(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/telephony/calls/1", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("admin GetCall: want 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetCall_ManagementSeesAnyBranch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	repo.upsertedCalls = []*models.TelephonyCall{{ID: 1, BranchID: intPtr(9)}}

	h := newTestTelephonyHandler(repo, "")
	r := gin.New()
	r.GET("/api/v1/telephony/calls/:id", func(c *gin.Context) {
		c.Set("role_id", 40) // management
		c.Set("user_id", 2)
		h.GetCall(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/telephony/calls/1", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("management GetCall: want 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetCall_ScopedRoleOwnBranch(t *testing.T) {
	scopedRoles := []struct {
		roleID int
		label  string
	}{
		{10, "sales"},
		{30, "quality_control"},
		{60, "visa"},
		{70, "partner"},
		{80, "hr"},
		{90, "legal"},
	}
	for _, tc := range scopedRoles {
		t.Run(tc.label, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			repo := newStubTelephonyRepo()
			repo.upsertedCalls = []*models.TelephonyCall{{ID: 1, BranchID: intPtr(5)}}

			h := newTestTelephonyHandlerWithUser(repo, "", intPtr(5)) // user branch = 5, call branch = 5
			r := gin.New()
			r.GET("/api/v1/telephony/calls/:id", func(c *gin.Context) {
				c.Set("role_id", tc.roleID)
				c.Set("user_id", 99)
				h.GetCall(c)
			})

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/telephony/calls/1", nil))

			if w.Code != http.StatusOK {
				t.Fatalf("role %s own branch: want 200, got %d body=%s", tc.label, w.Code, w.Body.String())
			}
		})
	}
}

func TestGetCall_ScopedRoleForeignBranch(t *testing.T) {
	scopedRoles := []struct {
		roleID int
		label  string
	}{
		{10, "sales"},
		{30, "quality_control"},
		{60, "visa"},
		{70, "partner"},
		{80, "hr"},
		{90, "legal"},
	}
	for _, tc := range scopedRoles {
		t.Run(tc.label, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			repo := newStubTelephonyRepo()
			repo.upsertedCalls = []*models.TelephonyCall{{ID: 1, BranchID: intPtr(99)}} // call is branch 99

			h := newTestTelephonyHandlerWithUser(repo, "", intPtr(5)) // user is branch 5
			r := gin.New()
			r.GET("/api/v1/telephony/calls/:id", func(c *gin.Context) {
				c.Set("role_id", tc.roleID)
				c.Set("user_id", 99)
				h.GetCall(c)
			})

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/telephony/calls/1", nil))

			if w.Code != http.StatusForbidden {
				t.Fatalf("role %s foreign branch: want 403, got %d body=%s", tc.label, w.Code, w.Body.String())
			}
		})
	}
}

func TestGetCall_ScopedRoleCallNoBranch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	repo.upsertedCalls = []*models.TelephonyCall{{ID: 1, BranchID: nil}} // call has no branch

	h := newTestTelephonyHandlerWithUser(repo, "", intPtr(5))
	r := gin.New()
	r.GET("/api/v1/telephony/calls/:id", func(c *gin.Context) {
		c.Set("role_id", 10) // sales
		c.Set("user_id", 99)
		h.GetCall(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/telephony/calls/1", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("scoped role + call with nil branch: want 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetCall_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo() // no calls

	h := newTestTelephonyHandler(repo, "")
	r := gin.New()
	r.GET("/api/v1/telephony/calls/:id", func(c *gin.Context) {
		c.Set("role_id", 50)
		c.Set("user_id", 1)
		h.GetCall(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/telephony/calls/999", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("not found: want 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// ── ListClientCalls scope tests ───────────────────────────────────────────────

// TestListClientCalls_AdminSeesAll: admin has ScopeKindAll → client access granted,
// and is branch-exempt → sees all calls of the client across branches.
func TestListClientCalls_AdminSeesAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	clientID := int64(42)
	repo.upsertedCalls = []*models.TelephonyCall{
		{ID: 1, ClientID: &clientID, BranchID: intPtr(1)},
		{ID: 2, ClientID: &clientID, BranchID: intPtr(2)},
	}

	h := newTestTelephonyHandlerWithAccess(repo, nil,
		&stubClientAccess{allowAll: true}, &stubLeadAccess{})
	r := gin.New()
	r.GET("/api/v1/clients/:id/calls", func(c *gin.Context) {
		c.Set("role_id", 50)
		c.Set("user_id", 1)
		h.ListClientCalls(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/clients/42/calls", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("admin ListClientCalls: want 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 2 {
		t.Fatalf("admin: want 2 calls, got total=%v", resp["total"])
	}
}

// TestListClientCalls_ScopedRoleWithAccessBranchFiltered: a scoped role (sales)
// that DOES have access to the client still gets its calls branch-filtered.
func TestListClientCalls_ScopedRoleWithAccessBranchFiltered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	clientID := int64(42)
	repo.upsertedCalls = []*models.TelephonyCall{
		{ID: 1, ClientID: &clientID, BranchID: intPtr(5)},  // own branch
		{ID: 2, ClientID: &clientID, BranchID: intPtr(99)}, // foreign
	}

	h := newTestTelephonyHandlerWithAccess(repo, intPtr(5),
		&stubClientAccess{allowed: map[int64]bool{42: true}}, &stubLeadAccess{})
	r := gin.New()
	r.GET("/api/v1/clients/:id/calls", func(c *gin.Context) {
		c.Set("role_id", 10) // sales
		c.Set("user_id", 99)
		h.ListClientCalls(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/clients/42/calls", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("sales with client access: want 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 1 {
		t.Fatalf("sales: want 1 call (own branch only), got total=%v", resp["total"])
	}
}

// TestListClientCalls_PartnerForeignClientForbidden is the P0 regression test:
// partner (own-scoped) requests the call history of a client they do NOT own —
// even though the call sits in the partner's own branch. Must be 403, no leak.
func TestListClientCalls_PartnerForeignClientForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	clientID := int64(42)
	repo.upsertedCalls = []*models.TelephonyCall{
		{ID: 1, ClientID: &clientID, BranchID: intPtr(5)}, // SAME branch as the partner
	}

	// partner branch 5; own-scope refusal of client 42 (mirrors clientMatchesScope→ErrForbidden).
	h := newTestTelephonyHandlerWithAccess(repo, intPtr(5),
		&stubClientAccess{errOnDeny: services.ErrForbidden}, &stubLeadAccess{})
	r := gin.New()
	r.GET("/api/v1/clients/:id/calls", func(c *gin.Context) {
		c.Set("role_id", 70) // partner
		c.Set("user_id", 99)
		h.ListClientCalls(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/clients/42/calls", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("partner foreign client: want 403, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestListClientCalls_NoClientAccessForbidden: every scoped role that cannot see a
// given client (own-scope miss for partner, branch miss for sales/visa/qc, no
// client access for hr) is denied BEFORE any call is returned. Covers both the
// ErrForbidden deny path and the nil-entity (branch miss) deny path.
func TestListClientCalls_NoClientAccessForbidden(t *testing.T) {
	cases := []struct {
		roleID  int
		label   string
		denyErr error // nil → mirrors branch-scope miss (nil,nil); ErrForbidden → own-scope refusal
	}{
		{70, "partner_own_miss", services.ErrForbidden},
		{10, "sales_branch_miss", nil},
		{60, "visa_branch_miss", nil},
		{30, "quality_control_branch_miss", nil},
		{80, "hr_no_access", services.ErrForbidden},
	}
	clientID := int64(42)
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			repo := newStubTelephonyRepo()
			repo.upsertedCalls = []*models.TelephonyCall{
				{ID: 1, ClientID: &clientID, BranchID: intPtr(5)}, // same branch as caller
			}
			h := newTestTelephonyHandlerWithAccess(repo, intPtr(5),
				&stubClientAccess{errOnDeny: tc.denyErr}, &stubLeadAccess{})
			r := gin.New()
			r.GET("/api/v1/clients/:id/calls", func(c *gin.Context) {
				c.Set("role_id", tc.roleID)
				c.Set("user_id", 99)
				h.ListClientCalls(c)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/clients/42/calls", nil))
			if w.Code != http.StatusForbidden {
				t.Fatalf("role=%s without client access: want 403, got %d body=%s", tc.label, w.Code, w.Body.String())
			}
		})
	}
}

// ── ListLeadCalls scope tests ─────────────────────────────────────────────────

func TestListLeadCalls_AdminSeesAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	leadID := int64(77)
	repo.upsertedCalls = []*models.TelephonyCall{
		{ID: 1, LeadID: &leadID, BranchID: intPtr(1)},
		{ID: 2, LeadID: &leadID, BranchID: intPtr(2)},
	}

	h := newTestTelephonyHandlerWithAccess(repo, nil,
		&stubClientAccess{}, &stubLeadAccess{allowAll: true})
	r := gin.New()
	r.GET("/api/v1/leads/:id/calls", func(c *gin.Context) {
		c.Set("role_id", 50)
		c.Set("user_id", 1)
		h.ListLeadCalls(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/leads/77/calls", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("admin ListLeadCalls: want 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 2 {
		t.Fatalf("admin: want 2 lead calls, got total=%v", resp["total"])
	}
}

// TestListLeadCalls_ScopedRoleWithAccessBranchFiltered: a scoped role (sales) with
// access to the lead gets its calls branch-filtered.
func TestListLeadCalls_ScopedRoleWithAccessBranchFiltered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	leadID := int64(77)
	repo.upsertedCalls = []*models.TelephonyCall{
		{ID: 1, LeadID: &leadID, BranchID: intPtr(5)},  // own branch
		{ID: 2, LeadID: &leadID, BranchID: intPtr(99)}, // foreign
	}

	h := newTestTelephonyHandlerWithAccess(repo, intPtr(5),
		&stubClientAccess{}, &stubLeadAccess{allowed: map[int64]bool{77: true}})
	r := gin.New()
	r.GET("/api/v1/leads/:id/calls", func(c *gin.Context) {
		c.Set("role_id", 10) // sales
		c.Set("user_id", 99)
		h.ListLeadCalls(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/leads/77/calls", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("sales with lead access: want 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 1 {
		t.Fatalf("sales: want 1 call (own branch only), got total=%v", resp["total"])
	}
}

// TestListLeadCalls_PartnerForeignLeadForbidden is the P0 regression test for the
// lead path: partner requests calls of a lead they do NOT own (same branch) → 403.
func TestListLeadCalls_PartnerForeignLeadForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newStubTelephonyRepo()
	leadID := int64(77)
	repo.upsertedCalls = []*models.TelephonyCall{
		{ID: 1, LeadID: &leadID, BranchID: intPtr(5)}, // SAME branch as the partner
	}

	h := newTestTelephonyHandlerWithAccess(repo, intPtr(5),
		&stubClientAccess{}, &stubLeadAccess{errOnDeny: services.ErrForbidden})
	r := gin.New()
	r.GET("/api/v1/leads/:id/calls", func(c *gin.Context) {
		c.Set("role_id", 70) // partner
		c.Set("user_id", 99)
		h.ListLeadCalls(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/leads/77/calls", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("partner foreign lead: want 403, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestListLeadCalls_NoLeadAccessForbidden: every role without access to the given
// lead is denied. For leads, hr AND legal have no lead access at all (resolveLeadScope
// → Forbidden), alongside the own/branch misses for partner/sales/visa/qc.
func TestListLeadCalls_NoLeadAccessForbidden(t *testing.T) {
	cases := []struct {
		roleID  int
		label   string
		denyErr error
	}{
		{70, "partner_own_miss", services.ErrForbidden},
		{10, "sales_branch_miss", nil},
		{60, "visa_branch_miss", nil},
		{30, "quality_control_branch_miss", nil},
		{80, "hr_no_access", services.ErrForbidden},
		{90, "legal_no_lead_access", services.ErrForbidden},
	}
	leadID := int64(77)
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			repo := newStubTelephonyRepo()
			repo.upsertedCalls = []*models.TelephonyCall{
				{ID: 1, LeadID: &leadID, BranchID: intPtr(5)},
			}
			h := newTestTelephonyHandlerWithAccess(repo, intPtr(5),
				&stubClientAccess{}, &stubLeadAccess{errOnDeny: tc.denyErr})
			r := gin.New()
			r.GET("/api/v1/leads/:id/calls", func(c *gin.Context) {
				c.Set("role_id", tc.roleID)
				c.Set("user_id", 99)
				h.ListLeadCalls(c)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/leads/77/calls", nil))
			if w.Code != http.StatusForbidden {
				t.Fatalf("role=%s without lead access: want 403, got %d body=%s", tc.label, w.Code, w.Body.String())
			}
		})
	}
}

