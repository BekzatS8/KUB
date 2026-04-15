package handlers

import (
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

type leadHandlerStubService struct {
	archiveCalled bool
	listScope     repositories.ArchiveScope
	listMyScope   repositories.ArchiveScope
	listFilter    repositories.LeadListFilter
	listMyFilter  repositories.LeadListFilter
	deleteCalled  bool
	archiveErr    error
	deleteErr     error
	listResp      []*models.Leads
	listMyResp    []*models.Leads
}

func (s *leadHandlerStubService) Create(lead *models.Leads, userID, roleID int) (int64, error) {
	return 1, nil
}
func (s *leadHandlerStubService) Update(lead *models.Leads, userID, roleID int) error { return nil }
func (s *leadHandlerStubService) GetByID(id int, userID, roleID int) (*models.Leads, error) {
	return &models.Leads{ID: id, OwnerID: userID}, nil
}
func (s *leadHandlerStubService) GetByIDWithArchiveScope(id int, userID, roleID int, scope repositories.ArchiveScope) (*models.Leads, error) {
	return &models.Leads{ID: id, OwnerID: userID, IsArchived: scope == repositories.ArchiveScopeArchivedOnly}, nil
}
func (s *leadHandlerStubService) Delete(id int, userID, roleID int) error {
	s.deleteCalled = true
	return s.deleteErr
}
func (s *leadHandlerStubService) ListForRole(userID, roleID, limit, offset int, scope repositories.ArchiveScope, filter repositories.LeadListFilter) ([]*models.Leads, error) {
	s.listScope = scope
	s.listFilter = filter
	if s.listResp != nil {
		return s.listResp, nil
	}
	return []*models.Leads{}, nil
}
func (s *leadHandlerStubService) ListMyWithArchiveScope(ownerID, limit, offset int, scope repositories.ArchiveScope) ([]*models.Leads, error) {
	s.listMyScope = scope
	return []*models.Leads{}, nil
}
func (s *leadHandlerStubService) ListMyWithFilterAndArchiveScope(ownerID, limit, offset int, scope repositories.ArchiveScope, filter repositories.LeadListFilter) ([]*models.Leads, error) {
	s.listMyScope = scope
	s.listMyFilter = filter
	if s.listMyResp != nil {
		return s.listMyResp, nil
	}
	return []*models.Leads{}, nil
}
func (s *leadHandlerStubService) AssignOwner(id, assigneeID, userID, roleID int) error { return nil }
func (s *leadHandlerStubService) UpdateStatus(id int, to string, userID, roleID int) error {
	return nil
}
func (s *leadHandlerStubService) ArchiveLead(id, userID, roleID int, reason string) error {
	s.archiveCalled = true
	return s.archiveErr
}
func (s *leadHandlerStubService) UnarchiveLead(id, userID, roleID int) error { return nil }
func (s *leadHandlerStubService) ConvertLeadToDeal(leadID int, amount float64, currency string, ownerID, userID, roleID int, clientID int, clientType string) (*models.Deals, error) {
	return &models.Deals{ID: 1}, nil
}
func (s *leadHandlerStubService) ConvertLeadToDealWithClientData(leadID int, amount float64, currency string, ownerID, userID, roleID int, clientData *models.Client) (*models.Deals, error) {
	return &models.Deals{ID: 1}, nil
}

type dealHandlerStubService struct {
	stubDealService
	listScope    repositories.ArchiveScope
	listMyScope  repositories.ArchiveScope
	listFilter   repositories.DealListFilter
	listMyFilter repositories.DealListFilter
	archived     bool
	deleteErr    error
	listResp     []*models.Deals
	listMyResp   []*models.Deals
}

func (s *dealHandlerStubService) ListForRole(userID, roleID, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, error) {
	s.listScope = scope
	s.listFilter = filter
	if s.listResp != nil {
		return s.listResp, nil
	}
	return []*models.Deals{}, nil
}
func (s *dealHandlerStubService) ListMyWithFilterAndArchiveScope(ownerID, limit, offset int, scope repositories.ArchiveScope, filter repositories.DealListFilter) ([]*models.Deals, error) {
	s.listMyScope = scope
	s.listMyFilter = filter
	if s.listMyResp != nil {
		return s.listMyResp, nil
	}
	return []*models.Deals{}, nil
}
func (s *dealHandlerStubService) Delete(id, userID, roleID int) error { return s.deleteErr }
func (s *dealHandlerStubService) GetByID(id int, userID, roleID int) (*models.Deals, error) {
	return &models.Deals{ID: id, OwnerID: userID}, nil
}
func (s *dealHandlerStubService) GetByIDWithArchiveScope(id int, userID, roleID int, scope repositories.ArchiveScope) (*models.Deals, error) {
	return &models.Deals{ID: id, OwnerID: userID, IsArchived: scope == repositories.ArchiveScopeArchivedOnly}, nil
}
func (s *dealHandlerStubService) ArchiveDeal(id, userID, roleID int, reason string) error {
	s.archived = true
	return nil
}
func (s *dealHandlerStubService) UnarchiveDeal(id, userID, roleID int) error { return nil }

func ctx(method, path, body string, role int) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", 100)
	c.Set("role_id", role)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	return c, w
}

func TestLeadDelete_NonAdminForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &LeadHandler{Service: &leadHandlerStubService{}}
	c, w := ctx(http.MethodDelete, "/leads/1", "", authz.RoleManagement)
	h.Delete(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestLeadDelete_SystemAdminAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &leadHandlerStubService{}
	h := &LeadHandler{Service: s}
	c, w := ctx(http.MethodDelete, "/leads/1", "", authz.RoleSystemAdmin)
	h.Delete(c)
	if (w.Code != http.StatusNoContent && w.Code != http.StatusOK) || !s.deleteCalled {
		t.Fatalf("expected successful delete and delete called, got code=%d called=%v", w.Code, s.deleteCalled)
	}
}

func TestLeadArchive_ReadOnlyForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &LeadHandler{Service: &leadHandlerStubService{archiveErr: services.ErrForbidden}}
	c, w := ctx(http.MethodPost, "/leads/1/archive", `{"reason":"x"}`, authz.RoleControl)
	h.Archive(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestLeadList_SupportsArchivedFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &leadHandlerStubService{}
	h := &LeadHandler{Service: s}
	c, w := ctx(http.MethodGet, "/leads?archive=archived", "", authz.RoleManagement)
	c.Request = httptest.NewRequest(http.MethodGet, "/leads?archive=archived", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleManagement)
	h.List(c)
	if w.Code != http.StatusOK || s.listScope != repositories.ArchiveScopeArchivedOnly {
		t.Fatalf("expected archived scope, code=%d scope=%s", w.Code, s.listScope)
	}
}

func TestLeadList_AppliesExtendedFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &leadHandlerStubService{}
	h := &LeadHandler{Service: s}
	c, w := ctx(http.MethodGet, "/leads?q=Smoke&status_group=active&sort_by=title&order=asc", "", authz.RoleManagement)
	c.Request = httptest.NewRequest(http.MethodGet, "/leads?q=Smoke&status_group=active&sort_by=title&order=asc", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleManagement)
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if s.listFilter.Query != "Smoke" || s.listFilter.StatusGroup != "active" || s.listFilter.SortBy != "title" || s.listFilter.Order != "asc" {
		t.Fatalf("unexpected lead filter: %+v", s.listFilter)
	}
}

func TestLeadListMy_ForwardsFiltersAndKeepsOwnerScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &leadHandlerStubService{}
	h := &LeadHandler{Service: s}
	c, w := ctx(http.MethodGet, "/leads/my?q=7701&status=converted&archive=all", "", authz.RoleSales)
	c.Request = httptest.NewRequest(http.MethodGet, "/leads/my?q=7701&status=converted&archive=all", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleSales)
	h.ListMy(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if s.listMyScope != repositories.ArchiveScopeAll {
		t.Fatalf("expected all scope, got %s", s.listMyScope)
	}
	if s.listMyFilter.Query != "7701" || s.listMyFilter.Status != "converted" {
		t.Fatalf("unexpected my filter: %+v", s.listMyFilter)
	}
}

func TestLeadList_InvalidFilterParams(t *testing.T) {
	testCases := []struct {
		name string
		url  string
	}{
		{name: "invalid sort", url: "/leads?sort_by=amount"},
		{name: "invalid order", url: "/leads?order=up"},
		{name: "invalid status", url: "/leads?status=won"},
		{name: "invalid status group", url: "/leads?status_group=completed"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			s := &leadHandlerStubService{}
			h := &LeadHandler{Service: s}
			c, w := ctx(http.MethodGet, tc.url, "", authz.RoleManagement)
			c.Request = httptest.NewRequest(http.MethodGet, tc.url, nil)
			c.Set("user_id", 100)
			c.Set("role_id", authz.RoleManagement)
			h.List(c)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestDealDelete_NonAdminForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DealHandler{Service: &dealHandlerStubService{}}
	c, w := ctx(http.MethodDelete, "/deals/1", "", authz.RoleOperations)
	h.Delete(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestDealList_DefaultActiveScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &dealHandlerStubService{}
	h := &DealHandler{Service: s}
	c, w := ctx(http.MethodGet, "/deals", "", authz.RoleManagement)
	c.Request = httptest.NewRequest(http.MethodGet, "/deals", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleManagement)
	h.List(c)
	if w.Code != http.StatusOK || s.listScope != repositories.ArchiveScopeActiveOnly {
		t.Fatalf("expected active scope, code=%d scope=%s", w.Code, s.listScope)
	}
}

func TestDealArchive_SystemAdminAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &dealHandlerStubService{}
	h := &DealHandler{Service: s}
	c, w := ctx(http.MethodPost, "/deals/1/archive", `{"reason":"cleanup"}`, authz.RoleSystemAdmin)
	h.Archive(c)
	if w.Code != http.StatusOK || !s.archived {
		t.Fatalf("expected 200 archived=true got code=%d archived=%v", w.Code, s.archived)
	}
}

func TestDealList_AppliesClientFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &dealHandlerStubService{}
	h := &DealHandler{Service: s}
	c, w := ctx(http.MethodGet, "/deals?client_id=42&client_type=individual", "", authz.RoleManagement)
	c.Request = httptest.NewRequest(http.MethodGet, "/deals?client_id=42&client_type=individual", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleManagement)
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if s.listFilter.ClientID != 42 || s.listFilter.ClientType != "individual" {
		t.Fatalf("expected filter {42 individual}, got %+v", s.listFilter)
	}
}

func TestLeadList_FiltersByActiveCompany(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &leadHandlerStubService{
		listResp: []*models.Leads{
			{ID: 1, CompanyID: 10},
			{ID: 2, CompanyID: 20},
		},
	}
	h := &LeadHandler{Service: s}
	c, w := ctx(http.MethodGet, "/leads", "", authz.RoleManagement)
	c.Request = httptest.NewRequest(http.MethodGet, "/leads", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleManagement)
	c.Set("active_company_id", 10)

	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"id":1`) || strings.Contains(body, `"id":2`) {
		t.Fatalf("expected only company-scoped leads, got %s", body)
	}
}

func TestDealList_FiltersByActiveCompany(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &dealHandlerStubService{
		listResp: []*models.Deals{
			{ID: 1, CompanyID: 10},
			{ID: 2, CompanyID: 20},
		},
	}
	h := &DealHandler{Service: s}
	c, w := ctx(http.MethodGet, "/deals", "", authz.RoleManagement)
	c.Request = httptest.NewRequest(http.MethodGet, "/deals", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleManagement)
	c.Set("active_company_id", 10)

	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"id":1`) || strings.Contains(body, `"id":2`) {
		t.Fatalf("expected only company-scoped deals, got %s", body)
	}
}

func TestDealListMy_AppliesClientIDWithoutType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &dealHandlerStubService{}
	h := &DealHandler{Service: s}
	c, w := ctx(http.MethodGet, "/deals/my?client_id=42", "", authz.RoleSales)
	c.Request = httptest.NewRequest(http.MethodGet, "/deals/my?client_id=42", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleSales)
	h.ListMy(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if s.listMyFilter.ClientID != 42 || s.listMyFilter.ClientType != "" {
		t.Fatalf("expected filter {42 \"\"}, got %+v", s.listMyFilter)
	}
}

func TestDealListMy_ArchiveAndClientFiltersWorkTogether(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &dealHandlerStubService{}
	h := &DealHandler{Service: s}
	c, w := ctx(http.MethodGet, "/deals/my?archive=archived&client_id=42&client_type=individual", "", authz.RoleSales)
	c.Request = httptest.NewRequest(http.MethodGet, "/deals/my?archive=archived&client_id=42&client_type=individual", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleSales)
	h.ListMy(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if s.listMyScope != repositories.ArchiveScopeArchivedOnly {
		t.Fatalf("expected archived scope, got %s", s.listMyScope)
	}
	if s.listMyFilter.ClientID != 42 || s.listMyFilter.ClientType != "individual" {
		t.Fatalf("expected filter {42 individual}, got %+v", s.listMyFilter)
	}
}

func TestDealList_InvalidClientID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &dealHandlerStubService{}
	h := &DealHandler{Service: s}
	c, w := ctx(http.MethodGet, "/deals?client_id=abc", "", authz.RoleManagement)
	c.Request = httptest.NewRequest(http.MethodGet, "/deals?client_id=abc", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleManagement)
	h.List(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDealList_AppliesExtendedFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &dealHandlerStubService{}
	h := &DealHandler{Service: s}
	c, w := ctx(http.MethodGet, "/deals?q=Марина&status_group=active&amount_min=50000&amount_max=300000&currency=kzt&sort_by=amount&order=desc", "", authz.RoleManagement)
	c.Request = httptest.NewRequest(http.MethodGet, "/deals?q=Марина&status_group=active&amount_min=50000&amount_max=300000&currency=kzt&sort_by=amount&order=desc", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleManagement)
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if s.listFilter.Query != "Марина" || s.listFilter.StatusGroup != "active" || s.listFilter.Currency != "KZT" || s.listFilter.SortBy != "amount" || s.listFilter.Order != "desc" {
		t.Fatalf("unexpected filter fields: %+v", s.listFilter)
	}
	if s.listFilter.AmountMin == nil || *s.listFilter.AmountMin != 50000 {
		t.Fatalf("expected amount_min=50000, got %+v", s.listFilter.AmountMin)
	}
	if s.listFilter.AmountMax == nil || *s.listFilter.AmountMax != 300000 {
		t.Fatalf("expected amount_max=300000, got %+v", s.listFilter.AmountMax)
	}
}

func TestDealList_StatusHasPriorityOverStatusGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &dealHandlerStubService{}
	h := &DealHandler{Service: s}
	c, w := ctx(http.MethodGet, "/deals?status=won&status_group=active", "", authz.RoleManagement)
	c.Request = httptest.NewRequest(http.MethodGet, "/deals?status=won&status_group=active", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleManagement)
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if s.listFilter.Status != "won" || s.listFilter.StatusGroup != "active" {
		t.Fatalf("unexpected filters: %+v", s.listFilter)
	}
}

func TestDealListMy_ForwardsExtendedFiltersInOwnerScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &dealHandlerStubService{}
	h := &DealHandler{Service: s}
	c, w := ctx(http.MethodGet, "/deals/my?q=7701&status=won&currency=usd", "", authz.RoleSales)
	c.Request = httptest.NewRequest(http.MethodGet, "/deals/my?q=7701&status=won&currency=usd", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleSales)
	h.ListMy(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if s.listMyFilter.Query != "7701" || s.listMyFilter.Status != "won" || s.listMyFilter.Currency != "USD" {
		t.Fatalf("unexpected listMy filter: %+v", s.listMyFilter)
	}
}

func TestDealList_InvalidAmountAndSortFilters(t *testing.T) {
	testCases := []struct {
		name string
		url  string
	}{
		{name: "invalid amount min", url: "/deals?amount_min=oops"},
		{name: "invalid amount max", url: "/deals?amount_max=oops"},
		{name: "invalid sort", url: "/deals?sort_by=random_field"},
		{name: "invalid order", url: "/deals?order=up"},
		{name: "invalid status", url: "/deals?status=converted"},
		{name: "invalid status group", url: "/deals?status_group=unknown"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			s := &dealHandlerStubService{}
			h := &DealHandler{Service: s}
			c, w := ctx(http.MethodGet, tc.url, "", authz.RoleManagement)
			c.Request = httptest.NewRequest(http.MethodGet, tc.url, nil)
			c.Set("user_id", 100)
			c.Set("role_id", authz.RoleManagement)
			h.List(c)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
