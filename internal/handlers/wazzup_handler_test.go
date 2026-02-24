package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	wz "turcompany/internal/integrations/wazzup"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

func TestWazzupWebhook_Unauthorized(t *testing.T) {
	h := NewWazzupHandler(wazzupServiceStub{handleWebhookFn: func(ctx context.Context, token string, authHeader string, payload []byte) (int, bool, error) {
		return 0, false, wz.ErrUnauthorized
	}})

	r := gin.New()
	r.POST("/integrations/wazzup/webhook/:token", h.Webhook)
	req := httptest.NewRequest(http.MethodPost, "/integrations/wazzup/webhook/tkn", bytes.NewBufferString(`{"messages":[]}`))
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", w.Code)
	}
}

func TestWazzupWebhook_DedupCreatesOnlyOneLead(t *testing.T) {
	repo := &wazzupRepoFake{integration: &models.WazzupIntegration{ID: 1, OwnerUserID: 7, Enabled: true}}
	crm := "secret-crm"
	sum := sha256.Sum256([]byte(crm))
	repo.integration.CRMKeyHash = hex.EncodeToString(sum[:])

	svc := wz.NewService(repo, &wazzupClientFake{})
	h := NewWazzupHandler(svc)
	r := gin.New()
	r.POST("/integrations/wazzup/webhook/:token", h.Webhook)

	payload := map[string]any{"messages": []map[string]any{{"id": "m1", "chatId": "+7 (700) 111-22-33", "chatType": "whatsapp", "text": "hello", "isIncoming": true}}}
	b, _ := json.Marshal(payload)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/integrations/wazzup/webhook/tkn", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+crm)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("unexpected status iteration %d: %d body=%s", i, w.Code, w.Body.String())
		}
	}
	if repo.createLeadCalls != 1 {
		t.Fatalf("expected one lead creation, got %d", repo.createLeadCalls)
	}
}

func TestWazzupWebhook_TestHandshakeWithoutAuthorization(t *testing.T) {
	repo := &wazzupRepoFake{integration: &models.WazzupIntegration{ID: 1, OwnerUserID: 7, Enabled: true, CRMKeyHash: "expected-hash"}}
	svc := wz.NewService(repo, &wazzupClientFake{})
	h := NewWazzupHandler(svc)
	r := gin.New()
	r.POST("/integrations/wazzup/webhook/:token", h.Webhook)

	req := httptest.NewRequest(http.MethodPost, "/integrations/wazzup/webhook/tkn", bytes.NewBufferString(`{"test":true}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestWazzupWebhook_MessagesWithoutAuthorizationReturnsUnauthorized(t *testing.T) {
	repo := &wazzupRepoFake{integration: &models.WazzupIntegration{ID: 1, OwnerUserID: 7, Enabled: true, CRMKeyHash: "expected-hash"}}
	svc := wz.NewService(repo, &wazzupClientFake{})
	h := NewWazzupHandler(svc)
	r := gin.New()
	r.POST("/integrations/wazzup/webhook/:token", h.Webhook)

	req := httptest.NewRequest(http.MethodPost, "/integrations/wazzup/webhook/tkn", bytes.NewBufferString(`{"messages":[{"id":"m1"}]}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestWazzupSetup_ReturnsCRMAndWebhookURL(t *testing.T) {
	repo := &wazzupRepoFake{integration: &models.WazzupIntegration{ID: 1, OwnerUserID: 5, Enabled: true, WebhookToken: "tok-old", APIKeyEnc: "key"}}
	cli := &wazzupClientFake{}
	svc := wz.NewService(repo, cli)
	h := NewWazzupHandler(svc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 5)
		c.Set("role_id", 40)
		c.Next()
	})
	r.POST("/integrations/wazzup/setup", h.Setup)

	req := httptest.NewRequest(http.MethodPost, "/integrations/wazzup/setup", bytes.NewBufferString(`{"webhooks_base_url":"https://crm.example.com","api_key":"key","enabled":true}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["crm_key"] == "" {
		t.Fatalf("crm_key must be returned once")
	}
	if out["webhook_url"] == "" {
		t.Fatalf("webhook_url must be returned")
	}
	if cli.patchCalls == 0 {
		t.Fatalf("expected PatchWebhooks call")
	}
}

func TestWazzupSetup_EmptyWebhooksBaseURL(t *testing.T) {
	h := NewWazzupHandler(wazzupServiceStub{})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 5)
		c.Set("role_id", 40)
		c.Next()
	})
	r.POST("/integrations/wazzup/setup", h.Setup)

	req := httptest.NewRequest(http.MethodPost, "/integrations/wazzup/setup", bytes.NewBufferString(`{"webhooks_base_url":"","api_key":"key","enabled":true}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestWazzupIframe_UsesLeadOrClientPhoneOverRequestPhone(t *testing.T) {
	var gotPhone string
	h := NewWazzupHandler(wazzupServiceStub{getIframeURLFn: func(ctx context.Context, ownerUserID int, companyID int, userName string, phone string, leadID int, clientID int) (string, error) {
		gotPhone = phone
		return "https://iframe.local", nil
	}})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 5)
		c.Set("role_id", 40)
		c.Next()
	})
	r.POST("/integrations/wazzup/iframe", h.Iframe)

	req := httptest.NewRequest(http.MethodPost, "/integrations/wazzup/iframe", bytes.NewBufferString(`{"phone":"77472013916","lead_id":42}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	if gotPhone != "" {
		t.Fatalf("expected empty phone when lead/client ids are provided, got %q", gotPhone)
	}
}

func TestWazzupIframe_InternalErrorReturns500(t *testing.T) {
	h := NewWazzupHandler(wazzupServiceStub{getIframeURLFn: func(ctx context.Context, ownerUserID int, companyID int, userName string, phone string, leadID int, clientID int) (string, error) {
		return "", errors.New("db down")
	}})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 5)
		c.Set("role_id", 40)
		c.Next()
	})
	r.POST("/integrations/wazzup/iframe", h.Iframe)

	req := httptest.NewRequest(http.MethodPost, "/integrations/wazzup/iframe", bytes.NewBufferString(`{"phone":"77001112233"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestWazzupCRMUsers_OK(t *testing.T) {
	repo := &wazzupRepoFake{
		integrationByToken: map[string]*models.WazzupIntegration{"tok": {ID: 1, Enabled: true}},
		crmUsers: []repoCRMUserDTO{{
			ID: 2, Name: "admin@kubcrm.kz", Email: "admin@kubcrm.kz", Phone: "", Active: true,
		}},
	}
	h := NewWazzupHandlerWithRepo(wazzupServiceStub{}, repo)
	r := gin.New()
	r.GET("/integrations/wazzup/crm/:token/users", h.CRMUsers)

	req := httptest.NewRequest(http.MethodGet, "/integrations/wazzup/crm/tok/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"id":"2"`) || !strings.Contains(body, `"users"`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestWazzupCRMUsers_NotFoundByToken(t *testing.T) {
	repo := &wazzupRepoFake{integrationByToken: map[string]*models.WazzupIntegration{}}
	h := NewWazzupHandlerWithRepo(wazzupServiceStub{}, repo)
	r := gin.New()
	r.GET("/integrations/wazzup/crm/:token/users", h.CRMUsers)

	req := httptest.NewRequest(http.MethodGet, "/integrations/wazzup/crm/bad/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestWazzupCRMUserByID_OK(t *testing.T) {
	repo := &wazzupRepoFake{
		integrationByToken: map[string]*models.WazzupIntegration{"tok": {ID: 1, Enabled: true}},
		crmUserByID: map[int]repoCRMUserDTO{
			2: {ID: 2, Name: "admin@kubcrm.kz", Email: "admin@kubcrm.kz", Phone: "", Active: true},
		},
	}
	h := NewWazzupHandlerWithRepo(wazzupServiceStub{}, repo)
	r := gin.New()
	r.GET("/integrations/wazzup/crm/:token/users/:id", h.CRMUserByID)

	req := httptest.NewRequest(http.MethodGet, "/integrations/wazzup/crm/tok/users/2", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"id":"2"`) {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestWazzupCRMUserByID_NotFoundByToken(t *testing.T) {
	repo := &wazzupRepoFake{integrationByToken: map[string]*models.WazzupIntegration{}}
	h := NewWazzupHandlerWithRepo(wazzupServiceStub{}, repo)
	r := gin.New()
	r.GET("/integrations/wazzup/crm/:token/users/:id", h.CRMUserByID)

	req := httptest.NewRequest(http.MethodGet, "/integrations/wazzup/crm/bad/users/2", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}

type wazzupServiceStub struct {
	handleWebhookFn func(ctx context.Context, token string, authHeader string, payload []byte) (int, bool, error)
	setupFn         func(ctx context.Context, ownerUserID int, webhooksBaseURL string, apiKey string, enabled bool) (*wz.SetupResponse, error)
	getIframeURLFn  func(ctx context.Context, ownerUserID int, companyID int, userName string, phone string, leadID int, clientID int) (string, error)
}

func (s wazzupServiceStub) Setup(ctx context.Context, ownerUserID int, webhooksBaseURL string, apiKey string, enabled bool) (*wz.SetupResponse, error) {
	if s.setupFn != nil {
		return s.setupFn(ctx, ownerUserID, webhooksBaseURL, apiKey, enabled)
	}
	return nil, nil
}
func (s wazzupServiceStub) GetIframeURL(ctx context.Context, ownerUserID int, companyID int, userName string, phone string, leadID int, clientID int) (string, error) {
	if s.getIframeURLFn != nil {
		return s.getIframeURLFn(ctx, ownerUserID, companyID, userName, phone, leadID, clientID)
	}
	return "", nil
}
func (s wazzupServiceStub) HandleWebhook(ctx context.Context, token string, authHeader string, payload []byte) (int, bool, error) {
	return s.handleWebhookFn(ctx, token, authHeader, payload)
}

type wazzupClientFake struct{ patchCalls int }

func (f *wazzupClientFake) PatchWebhooks(ctx context.Context, apiKey, webhooksURI, crmKey string) error {
	f.patchCalls++
	return nil
}
func (f *wazzupClientFake) UpsertUsers(ctx context.Context, apiKey string, users []wz.UserUpsert) error {
	return nil
}
func (f *wazzupClientFake) CreateIframe(ctx context.Context, apiKey string, req wz.CreateIframeRequest) (string, error) {
	return "https://iframe.local", nil
}

type wazzupRepoFake struct {
	integration        *models.WazzupIntegration
	integrationByToken map[string]*models.WazzupIntegration
	crmUsers           []repoCRMUserDTO
	crmUserByID        map[int]repoCRMUserDTO
	dedup              map[string]struct{}
	leadByPhone        map[string]int
	nextLeadID         int
	createLeadCalls    int
}

type repoCRMUserDTO struct {
	ID     int
	Name   string
	Email  string
	Phone  string
	Active bool
}

func (f *wazzupRepoFake) GetIntegrationByToken(ctx context.Context, token string) (*models.WazzupIntegration, error) {
	if f.integrationByToken != nil {
		return f.integrationByToken[token], nil
	}
	return f.integration, nil
}
func (f *wazzupRepoFake) ListCRMUsers(ctx context.Context) ([]repositories.CRMUserDTO, error) {
	out := make([]repositories.CRMUserDTO, 0, len(f.crmUsers))
	for _, u := range f.crmUsers {
		out = append(out, repositories.CRMUserDTO{ID: u.ID, Name: u.Name, Email: u.Email, Phone: u.Phone, Active: u.Active})
	}
	return out, nil
}
func (f *wazzupRepoFake) GetCRMUserByID(ctx context.Context, id int) (*repositories.CRMUserDTO, error) {
	if f.crmUserByID == nil {
		return nil, nil
	}
	u, ok := f.crmUserByID[id]
	if !ok {
		return nil, nil
	}
	out := &repositories.CRMUserDTO{ID: u.ID, Name: u.Name, Email: u.Email, Phone: u.Phone, Active: u.Active}
	return out, nil
}
func (f *wazzupRepoFake) GetIntegrationByOwnerUserID(ctx context.Context, ownerUserID int) (*models.WazzupIntegration, error) {
	return f.integration, nil
}
func (f *wazzupRepoFake) UpsertIntegrationByOwner(ctx context.Context, ownerUserID int, apiKeyEnc, crmKeyHash, webhooksURI string, enabled bool) (int, string, error) {
	if f.integration == nil {
		f.integration = &models.WazzupIntegration{ID: 1, OwnerUserID: ownerUserID}
	}
	f.integration.APIKeyEnc = apiKeyEnc
	f.integration.CRMKeyHash = crmKeyHash
	f.integration.Enabled = enabled
	if f.integration.WebhookToken == "" {
		f.integration.WebhookToken = "tok-new"
	}
	f.integration.WebhooksURI = webhooksURI
	return f.integration.ID, f.integration.WebhookToken, nil
}
func (f *wazzupRepoFake) RegisterDedup(ctx context.Context, integrationID int, externalID string) (bool, error) {
	if f.dedup == nil {
		f.dedup = map[string]struct{}{}
	}
	if _, ok := f.dedup[externalID]; ok {
		return false, nil
	}
	f.dedup[externalID] = struct{}{}
	return true, nil
}
func (f *wazzupRepoFake) FindLeadByPhone(ctx context.Context, phone string) (int, error) {
	if f.leadByPhone == nil {
		f.leadByPhone = map[string]int{}
	}
	return f.leadByPhone[phone], nil
}
func (f *wazzupRepoFake) CreateLeadFromInbound(ctx context.Context, ownerID int, phone, firstMessage string) (int, error) {
	if f.leadByPhone == nil {
		f.leadByPhone = map[string]int{}
	}
	f.createLeadCalls++
	f.nextLeadID++
	if f.nextLeadID == 0 {
		f.nextLeadID = 1
	}
	f.leadByPhone[phone] = f.nextLeadID
	return f.nextLeadID, nil
}
func (f *wazzupRepoFake) UpdateLeadDescriptionIfEmpty(ctx context.Context, leadID int, firstMessage string) error {
	return nil
}
func (f *wazzupRepoFake) GetLeadPhoneByID(ctx context.Context, leadID int) (string, error) {
	return "", nil
}
func (f *wazzupRepoFake) GetClientPhoneByID(ctx context.Context, clientID int) (string, error) {
	return "", nil
}
