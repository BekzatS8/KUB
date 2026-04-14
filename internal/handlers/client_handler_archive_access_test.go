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
func (s *stubClientArchiveService) ListForRole(userID, roleID, limit, offset int, clientType string, scope repositories.ArchiveScope) ([]*models.Client, error) {
	s.archiveScope = scope
	return []*models.Client{}, nil
}
func (s *stubClientArchiveService) ListMineWithArchiveScope(userID, limit, offset int, clientType string, scope repositories.ArchiveScope) ([]*models.Client, error) {
	s.archiveScope = scope
	return []*models.Client{}, nil
}
func (s *stubClientArchiveService) ListIndividualsForRole(userID, roleID, limit, offset int, q string, scope repositories.ArchiveScope) ([]*models.Client, error) {
	s.archiveScope = scope
	return []*models.Client{}, nil
}
func (s *stubClientArchiveService) ListCompaniesForRole(userID, roleID, limit, offset int, q string, scope repositories.ArchiveScope) ([]*models.Client, error) {
	s.archiveScope = scope
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
