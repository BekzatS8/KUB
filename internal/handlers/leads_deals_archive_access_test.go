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
	deleteCalled  bool
	archiveErr    error
	deleteErr     error
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
func (s *leadHandlerStubService) ListForRole(userID, roleID, limit, offset int, scope repositories.ArchiveScope) ([]*models.Leads, error) {
	s.listScope = scope
	return []*models.Leads{}, nil
}
func (s *leadHandlerStubService) ListMyWithArchiveScope(ownerID, limit, offset int, scope repositories.ArchiveScope) ([]*models.Leads, error) {
	s.listMyScope = scope
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
	listScope   repositories.ArchiveScope
	listMyScope repositories.ArchiveScope
	archived    bool
	deleteErr   error
}

func (s *dealHandlerStubService) ListForRole(userID, roleID, limit, offset int, scope repositories.ArchiveScope) ([]*models.Deals, error) {
	s.listScope = scope
	return []*models.Deals{}, nil
}
func (s *dealHandlerStubService) ListMyWithArchiveScope(ownerID, limit, offset int, scope repositories.ArchiveScope) ([]*models.Deals, error) {
	s.listMyScope = scope
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
