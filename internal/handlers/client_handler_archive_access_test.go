package handlers

import (
	"context"
	"errors"
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

type stubClientArchiveService struct {
	archiveScope repositories.ArchiveScope
	lastFilter   repositories.ClientListFilter
	deleted      bool
}

func (s *stubClientArchiveService) Create(*models.Client, int, int) (int64, error) { return 1, nil }
func (s *stubClientArchiveService) Update(*models.Client, int, int) error          { return nil }
func (s *stubClientArchiveService) Delete(int, int, int) error                     { s.deleted = true; return nil }
func (s *stubClientArchiveService) ArchiveClient(int, int, int, string) error      { return nil }
func (s *stubClientArchiveService) UnarchiveClient(int, int, int) error            { return nil }
func (s *stubClientArchiveService) Patch(int, map[string]any, int, int) (*models.Client, error) {
	return nil, nil
}
func (s *stubClientArchiveService) GetByID(id int, userID, roleID int) (*models.Client, error) {
	return &models.Client{ID: id, OwnerID: userID}, nil
}
func (s *stubClientArchiveService) GetByIDWithArchiveScope(id int, userID, roleID int, scope repositories.ArchiveScope) (*models.Client, error) {
	return &models.Client{ID: id, OwnerID: userID, IsArchived: scope == repositories.ArchiveScopeArchivedOnly}, nil
}
func (s *stubClientArchiveService) ListForRole(userID, roleID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	s.archiveScope = scope
	s.lastFilter = filter
	return []*models.Client{}, nil
}
func (s *stubClientArchiveService) ListMineWithArchiveScope(userID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	s.archiveScope = scope
	s.lastFilter = filter
	return []*models.Client{}, nil
}
func (s *stubClientArchiveService) ListIndividualsForRole(userID, roleID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	s.archiveScope = scope
	s.lastFilter = filter
	return []*models.Client{}, nil
}
func (s *stubClientArchiveService) ListCompaniesForRole(userID, roleID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	s.archiveScope = scope
	s.lastFilter = filter
	return []*models.Client{}, nil
}
func (s *stubClientArchiveService) GetMissingYellow(context.Context, int, int, int) ([]string, error) {
	return nil, errors.New("no")
}
func (s *stubClientArchiveService) GetProfile(context.Context, int, int, int) (*services.ClientProfilePayload, error) {
	return nil, errors.New("no")
}

func clientCtx(method, path, body string, role int) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", 100)
	c.Set("role_id", role)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	return c, w
}

func TestClientDelete_NonAdminForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &ClientHandler{Service: &stubClientArchiveService{}}
	c, w := clientCtx(http.MethodDelete, "/clients/1", "", authz.RoleManagement)
	h.Delete(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 got %d", w.Code)
	}
}

func TestClientDelete_SystemAdminAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &stubClientArchiveService{}
	h := &ClientHandler{Service: s}
	c, w := clientCtx(http.MethodDelete, "/clients/1", "", authz.RoleSystemAdmin)
	h.Delete(c)
	if (w.Code != http.StatusNoContent && w.Code != http.StatusOK) || !s.deleted {
		t.Fatalf("expected success delete")
	}
}

func TestClientList_ArchiveFilterArchived(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &stubClientArchiveService{}
	h := &ClientHandler{Service: s}
	c, w := clientCtx(http.MethodGet, "/clients?archive=archived", "", authz.RoleManagement)
	c.Request = httptest.NewRequest(http.MethodGet, "/clients?archive=archived", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleManagement)
	h.List(c)
	if w.Code != http.StatusOK || s.archiveScope != repositories.ArchiveScopeArchivedOnly {
		t.Fatalf("expected archived scope")
	}
}

func TestClientList_AppliesExtendedFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &stubClientArchiveService{}
	h := &ClientHandler{Service: s}
	c, w := clientCtx(http.MethodGet, "/clients?q=Silk&client_type=legal&has_deals=true&deal_status_group=active&sort_by=display_name&order=asc", "", authz.RoleManagement)
	c.Request = httptest.NewRequest(http.MethodGet, "/clients?q=Silk&client_type=legal&has_deals=true&deal_status_group=active&sort_by=display_name&order=asc", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleManagement)
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", w.Code, w.Body.String())
	}
	if s.lastFilter.Query != "Silk" || s.lastFilter.ClientType != "legal" || s.lastFilter.DealStatusGroup != "active" || s.lastFilter.SortBy != "display_name" || s.lastFilter.Order != "asc" {
		t.Fatalf("unexpected filter %+v", s.lastFilter)
	}
	if s.lastFilter.HasDeals == nil || !*s.lastFilter.HasDeals {
		t.Fatalf("expected has_deals=true, got %+v", s.lastFilter.HasDeals)
	}
}

func TestClientListMy_KeepsOwnerScopeAndFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &stubClientArchiveService{}
	h := &ClientHandler{Service: s}
	c, w := clientCtx(http.MethodGet, "/clients/my?q=Марина&has_deals=false&archive=all", "", authz.RoleSales)
	c.Request = httptest.NewRequest(http.MethodGet, "/clients/my?q=Марина&has_deals=false&archive=all", nil)
	c.Set("user_id", 100)
	c.Set("role_id", authz.RoleSales)
	h.ListMy(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", w.Code, w.Body.String())
	}
	if s.archiveScope != repositories.ArchiveScopeAll {
		t.Fatalf("expected scope all, got %s", s.archiveScope)
	}
	if s.lastFilter.Query != "Марина" || s.lastFilter.HasDeals == nil || *s.lastFilter.HasDeals {
		t.Fatalf("unexpected my filter %+v", s.lastFilter)
	}
}

func TestClientList_InvalidFilterParams(t *testing.T) {
	cases := []string{
		"/clients?client_type=corp",
		"/clients?has_deals=maybe",
		"/clients?deal_status_group=unknown",
		"/clients?sort_by=amount",
		"/clients?order=up",
	}
	for _, url := range cases {
		gin.SetMode(gin.TestMode)
		s := &stubClientArchiveService{}
		h := &ClientHandler{Service: s}
		c, w := clientCtx(http.MethodGet, url, "", authz.RoleManagement)
		c.Request = httptest.NewRequest(http.MethodGet, url, nil)
		c.Set("user_id", 100)
		c.Set("role_id", authz.RoleManagement)
		h.List(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("url=%s expected 400 got %d body=%s", url, w.Code, w.Body.String())
		}
	}
}

func TestClientListByPresetType_KeepsEndpointSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &stubClientArchiveService{}
	h := &ClientHandler{Service: s}

	c1, w1 := clientCtx(http.MethodGet, "/clients/individual?client_type=legal&q=fio", "", authz.RoleManagement)
	c1.Request = httptest.NewRequest(http.MethodGet, "/clients/individual?client_type=legal&q=fio", nil)
	c1.Set("user_id", 100)
	c1.Set("role_id", authz.RoleManagement)
	h.ListIndividuals(c1)
	if w1.Code != http.StatusOK {
		t.Fatalf("individual expected 200 got %d", w1.Code)
	}

	c2, w2 := clientCtx(http.MethodGet, "/clients/company?client_type=individual&q=Silk", "", authz.RoleManagement)
	c2.Request = httptest.NewRequest(http.MethodGet, "/clients/company?client_type=individual&q=Silk", nil)
	c2.Set("user_id", 100)
	c2.Set("role_id", authz.RoleManagement)
	h.ListCompanies(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("company expected 200 got %d", w2.Code)
	}
}
