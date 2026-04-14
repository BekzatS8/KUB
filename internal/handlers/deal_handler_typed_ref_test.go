package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type stubDealService struct {
	createFn func(deal *models.Deals, userID, roleID int) (int64, error)
}

func (s *stubDealService) Create(deal *models.Deals, userID, roleID int) (int64, error) {
	return s.createFn(deal, userID, roleID)
}
func (s *stubDealService) Update(deal *models.Deals, userID, roleID int) error { return nil }
func (s *stubDealService) GetByID(id int, userID, roleID int) (*models.Deals, error) {
	return nil, nil
}
func (s *stubDealService) Delete(id, userID, roleID int) error { return nil }
func (s *stubDealService) ListForRole(userID, roleID, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, error) {
	return nil, nil
}
func (s *stubDealService) ListMyWithFilterAndArchiveScope(ownerID, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, error) {
	return nil, nil
}
func (s *stubDealService) UpdateStatus(id int, to string, userID, roleID int) error { return nil }
func (s *stubDealService) ArchiveDeal(id, userID, roleID int, reason string) error  { return nil }
func (s *stubDealService) UnarchiveDeal(id, userID, roleID int) error               { return nil }
func (s *stubDealService) GetByIDWithArchiveScope(id int, userID, roleID int, scope repositories.ArchiveScope) (*models.Deals, error) {
	return nil, nil
}

func performCreate(t *testing.T, h *DealHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/deals", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", 101)
	c.Set("role_id", authz.RoleSales)
	h.Create(c)
	return w
}

func TestDealCreate_RequiresClientType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DealHandler{}
	w := performCreate(t, h, `{
		"lead_id": 1,
		"client_id": 2,
		"amount": 1000,
		"currency": "USD"
	}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDealCreate_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DealHandler{Service: &stubDealService{
		createFn: func(deal *models.Deals, userID, roleID int) (int64, error) {
			return 55, nil
		},
	}}
	w := performCreate(t, h, `{
		"lead_id": 2,
		"client_id": 4,
		"client_type": "legal",
		"amount": 50000,
		"currency": "USD",
		"status": "new"
	}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDealCreate_ClientTypeMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DealHandler{Service: &stubDealService{
		createFn: func(deal *models.Deals, userID, roleID int) (int64, error) {
			return 0, services.ErrClientTypeMismatch
		},
	}}
	w := performCreate(t, h, `{"lead_id":2,"client_id":4,"client_type":"legal","amount":50000,"currency":"USD"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDealCreate_ClientNotFoundIsExplicit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DealHandler{Service: &stubDealService{
		createFn: func(deal *models.Deals, userID, roleID int) (int64, error) {
			return 0, services.ErrClientNotFound
		},
	}}
	w := performCreate(t, h, `{"lead_id":2,"client_id":404,"client_type":"legal","amount":50000,"currency":"USD"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("unexpected internal error body: %s", w.Body.String())
	}
}

func TestDealCreate_LeadNotFoundIsExplicit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DealHandler{Service: &stubDealService{
		createFn: func(deal *models.Deals, userID, roleID int) (int64, error) {
			return 0, services.ErrLeadNotFound
		},
	}}
	w := performCreate(t, h, `{"lead_id":404,"client_id":4,"client_type":"legal","amount":50000,"currency":"USD"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDealCreate_ClientRepoNotConfigured_IsMeaningful(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DealHandler{Service: &stubDealService{
		createFn: func(deal *models.Deals, userID, roleID int) (int64, error) {
			return 0, fmt.Errorf("wrapped: %w", services.ErrClientRepoNotConfigured)
		},
	}}
	w := performCreate(t, h, `{"lead_id":2,"client_id":4,"client_type":"legal","amount":50000,"currency":"USD"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDealCreate_LeadIDRequiredUsesErrorsIs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DealHandler{Service: &stubDealService{
		createFn: func(deal *models.Deals, userID, roleID int) (int64, error) {
			return 0, fmt.Errorf("validation: %w", services.ErrLeadIDRequired)
		},
	}}
	w := performCreate(t, h, `{"lead_id":2,"client_id":4,"client_type":"legal","amount":50000,"currency":"USD"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Lead ID is required") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestDealCreate_AmountValidationWrapped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DealHandler{Service: &stubDealService{
		createFn: func(deal *models.Deals, userID, roleID int) (int64, error) {
			return 0, errors.Join(errors.New("ctx"), services.ErrAmountInvalid)
		},
	}}
	w := performCreate(t, h, `{"lead_id":2,"client_id":4,"client_type":"legal","amount":50000,"currency":"USD"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDealCreate_ExistingLeadConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DealHandler{Service: &stubDealService{
		createFn: func(deal *models.Deals, userID, roleID int) (int64, error) {
			return 0, &services.DealAlreadyExistsError{LeadID: 2, ExistingDealID: 17}
		},
	}}
	w := performCreate(t, h, `{"lead_id":2,"client_id":4,"client_type":"legal","amount":50000,"currency":"USD"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, DealAlreadyExistsCode) || !strings.Contains(body, `"existing_deal_id":17`) {
		t.Fatalf("unexpected body: %s", body)
	}
}
