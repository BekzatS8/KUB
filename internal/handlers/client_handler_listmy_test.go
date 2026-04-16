package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type stubClientListMyService struct {
	clients    []*models.Client
	err        error
	lastFilter repositories.ClientListFilter
	total      int
}

func (s *stubClientListMyService) Create(*models.Client, int, int) (int64, error) {
	return 0, errors.New("not implemented")
}
func (s *stubClientListMyService) Update(*models.Client, int, int) error {
	return errors.New("not implemented")
}
func (s *stubClientListMyService) Delete(int, int, int) error { return errors.New("not implemented") }
func (s *stubClientListMyService) ArchiveClient(int, int, int, string) error {
	return errors.New("not implemented")
}
func (s *stubClientListMyService) UnarchiveClient(int, int, int) error {
	return errors.New("not implemented")
}
func (s *stubClientListMyService) Patch(int, map[string]any, int, int) (*models.Client, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) GetByID(int, int, int) (*models.Client, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) GetByIDWithArchiveScope(int, int, int, repositories.ArchiveScope) (*models.Client, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) ListForRole(int, int, int, int, repositories.ClientListFilter, repositories.ArchiveScope) ([]*models.Client, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) ListMineWithArchiveScope(_ int, _ int, _ int, filter repositories.ClientListFilter, _ repositories.ArchiveScope) ([]*models.Client, error) {
	s.lastFilter = filter
	return s.clients, s.err
}
func (s *stubClientListMyService) ListIndividualsForRole(int, int, int, int, repositories.ClientListFilter, repositories.ArchiveScope) ([]*models.Client, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) ListCompaniesForRole(int, int, int, int, repositories.ClientListFilter, repositories.ArchiveScope) ([]*models.Client, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) GetMissingYellow(_ context.Context, _, _, _ int) ([]string, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) GetProfile(_ context.Context, _, _, _ int) (*services.ClientProfilePayload, error) {
	return nil, errors.New("not implemented")
}
func (s *stubClientListMyService) ListMineWithArchiveScopeAndTotal(_ int, _ int, _ int, filter repositories.ClientListFilter, _ repositories.ArchiveScope) ([]*models.Client, int, error) {
	s.lastFilter = filter
	return s.clients, s.total, s.err
}
func (s *stubClientListMyService) ListForRoleWithTotal(int, int, int, int, repositories.ClientListFilter, repositories.ArchiveScope) ([]*models.Client, int, error) {
	return nil, 0, errors.New("not implemented")
}
func (s *stubClientListMyService) ListIndividualsForRoleWithTotal(int, int, int, int, repositories.ClientListFilter, repositories.ArchiveScope) ([]*models.Client, int, error) {
	return nil, 0, errors.New("not implemented")
}
func (s *stubClientListMyService) ListCompaniesForRoleWithTotal(int, int, int, int, repositories.ClientListFilter, repositories.ArchiveScope) ([]*models.Client, int, error) {
	return nil, 0, errors.New("not implemented")
}

func TestClientHandler_ListMy_SuccessScenarios(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		clients []*models.Client
	}{
		{
			name: "individual clients",
			clients: []*models.Client{{
				ID:         1,
				ClientType: models.ClientTypeIndividual,
				FirstName:  "A",
				LastName:   "B",
			}},
		},
		{
			name: "legal clients",
			clients: []*models.Client{{
				ID:         2,
				ClientType: models.ClientTypeLegal,
				Name:       "LLP Test",
				LegalProfile: &models.ClientLegalProfile{
					CompanyName: "LLP Test",
					BIN:         "123",
				},
			}},
		},
		{
			name:    "empty list",
			clients: []*models.Client{},
		},
		{
			name: "mixed legacy and profile",
			clients: []*models.Client{{
				ID:         3,
				ClientType: models.ClientTypeIndividual,
				Name:       "Legacy Person",
				FirstName:  "Legacy",
				LastName:   "Person",
			}, {
				ID:         4,
				ClientType: models.ClientTypeLegal,
				Name:       "Profile LLC",
				LegalProfile: &models.ClientLegalProfile{
					CompanyName: "Profile LLC",
					BIN:         "987",
				},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &ClientHandler{Service: &stubClientListMyService{clients: tc.clients}}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/clients/my", nil)
			c.Set("user_id", 101)
			c.Set("role_id", 10)

			h.ListMy(c)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			var got []map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("invalid json: %v", err)
			}
			if len(got) != len(tc.clients) {
				t.Fatalf("expected %d clients, got %d", len(tc.clients), len(got))
			}
		})
	}
}

func TestClientHandler_ListMy_ForwardsOptionalClientType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubClientListMyService{clients: []*models.Client{}}
	h := &ClientHandler{Service: svc}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/clients/my?client_type=legal", nil)
	c.Set("user_id", 101)
	c.Set("role_id", 10)

	h.ListMy(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if svc.lastFilter.ClientType != "legal" {
		t.Fatalf("expected client_type legal passed to service, got %q", svc.lastFilter.ClientType)
	}
}

func TestClientHandler_ListMy_ForwardsDealsAndSortFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubClientListMyService{clients: []*models.Client{}}
	h := &ClientHandler{Service: svc}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/clients/my?has_deals=true&deal_status_group=active&sort_by=client_type&order=desc", nil)
	c.Set("user_id", 101)
	c.Set("role_id", 10)

	h.ListMy(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if svc.lastFilter.HasDeals == nil || !*svc.lastFilter.HasDeals {
		t.Fatalf("expected has_deals=true, got %+v", svc.lastFilter.HasDeals)
	}
	if svc.lastFilter.DealStatusGroup != "active" || svc.lastFilter.SortBy != "client_type" || svc.lastFilter.Order != "desc" {
		t.Fatalf("unexpected filter: %+v", svc.lastFilter)
	}
}

func TestClientHandler_ListMy_PaginatedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubClientListMyService{clients: []*models.Client{}, total: 45}
	h := &ClientHandler{Service: svc}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/clients/my?paginate=true&page=0&size=-1", nil)
	c.Set("user_id", 101)
	c.Set("role_id", 10)

	h.ListMy(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "\"items\":") || !strings.Contains(body, "\"pagination\":") {
		t.Fatalf("expected paginated body, got %s", body)
	}
	if !strings.Contains(body, "\"size\":15") || !strings.Contains(body, "\"page\":1") {
		t.Fatalf("expected normalized defaults in pagination, got %s", body)
	}
}
