package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type stubLeadPaginationService struct{}

func (s *stubLeadPaginationService) Create(*models.Leads, int, int) (int64, error) { return 0, nil }
func (s *stubLeadPaginationService) Update(*models.Leads, int, int) error          { return nil }
func (s *stubLeadPaginationService) GetByID(int, int, int) (*models.Leads, error)  { return nil, nil }
func (s *stubLeadPaginationService) GetByIDWithArchiveScope(int, int, int, repositories.ArchiveScope) (*models.Leads, error) {
	return nil, nil
}
func (s *stubLeadPaginationService) Delete(int, int, int) error { return nil }
func (s *stubLeadPaginationService) ListForRole(int, int, int, int, repositories.ArchiveScope, repositories.LeadListFilter) ([]*models.Leads, error) {
	return []*models.Leads{}, nil
}
func (s *stubLeadPaginationService) ListMyWithArchiveScope(int, int, int, repositories.ArchiveScope) ([]*models.Leads, error) {
	return []*models.Leads{}, nil
}
func (s *stubLeadPaginationService) ListMyWithFilterAndArchiveScope(int, int, int, repositories.ArchiveScope, repositories.LeadListFilter) ([]*models.Leads, error) {
	return []*models.Leads{}, nil
}
func (s *stubLeadPaginationService) AssignOwner(int, int, int, int) error { return nil }
func (s *stubLeadPaginationService) UpdateStatus(int, string, int, int) error {
	return nil
}
func (s *stubLeadPaginationService) ArchiveLead(int, int, int, string) error { return nil }
func (s *stubLeadPaginationService) UnarchiveLead(int, int, int) error       { return nil }
func (s *stubLeadPaginationService) ConvertLeadToDeal(int, float64, string, int, int, int, int, string) (*models.Deals, error) {
	return nil, nil
}
func (s *stubLeadPaginationService) ConvertLeadToDealWithClientData(int, float64, string, int, int, int, *models.Client) (*models.Deals, error) {
	return nil, nil
}
func (s *stubLeadPaginationService) ListForRoleWithTotal(int, int, int, int, repositories.ArchiveScope, repositories.LeadListFilter) ([]*models.Leads, int, error) {
	return []*models.Leads{}, 20, nil
}
func (s *stubLeadPaginationService) ListMyWithFilterAndArchiveScopeAndTotal(int, int, int, repositories.ArchiveScope, repositories.LeadListFilter) ([]*models.Leads, int, error) {
	return []*models.Leads{}, 20, nil
}

func TestLeadHandler_List_PaginatedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &LeadHandler{Service: &stubLeadPaginationService{}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/leads?paginate=true&page=1&size=15", nil)
	c.Set("user_id", 1)
	c.Set("role_id", 1)
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "\"pagination\"") {
		t.Fatalf("expected pagination envelope, got %s", w.Body.String())
	}
}

type stubDealPaginationService struct{}

func (s *stubDealPaginationService) Create(*models.Deals, int, int) (int64, error) { return 0, nil }
func (s *stubDealPaginationService) Update(*models.Deals, int, int) error          { return nil }
func (s *stubDealPaginationService) GetByID(int, int, int) (*models.Deals, error)  { return nil, nil }
func (s *stubDealPaginationService) Delete(int, int, int) error                    { return nil }
func (s *stubDealPaginationService) ListForRole(int, int, int, int, repositories.ArchiveScope, repositories.DealListFilter) ([]*models.Deals, error) {
	return []*models.Deals{}, nil
}
func (s *stubDealPaginationService) ListMyWithFilterAndArchiveScope(int, int, int, repositories.ArchiveScope, repositories.DealListFilter) ([]*models.Deals, error) {
	return []*models.Deals{}, nil
}
func (s *stubDealPaginationService) UpdateStatus(int, string, int, int) error { return nil }
func (s *stubDealPaginationService) ArchiveDeal(int, int, int, string) error  { return nil }
func (s *stubDealPaginationService) UnarchiveDeal(int, int, int) error        { return nil }
func (s *stubDealPaginationService) GetByIDWithArchiveScope(int, int, int, repositories.ArchiveScope) (*models.Deals, error) {
	return nil, nil
}
func (s *stubDealPaginationService) ListForRoleWithTotal(int, int, int, int, repositories.ArchiveScope, repositories.DealListFilter) ([]*models.Deals, int, error) {
	return []*models.Deals{}, 11, nil
}
func (s *stubDealPaginationService) ListMyWithFilterAndArchiveScopeAndTotal(int, int, int, repositories.ArchiveScope, repositories.DealListFilter) ([]*models.Deals, int, error) {
	return []*models.Deals{}, 11, nil
}

func TestDealHandler_List_LegacyArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DealHandler{Service: &stubDealPaginationService{}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/deals?page=1&size=10", nil)
	c.Set("user_id", 1)
	c.Set("role_id", 1)
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "\"pagination\"") {
		t.Fatalf("expected legacy array response, got %s", w.Body.String())
	}
}
