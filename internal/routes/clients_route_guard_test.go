package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/middleware"
)

// buildClientsGuardRouter wires the SAME RequirePermission guards used on the
// /clients group in SetupRoutes, with stub 200 handlers, and injects the given
// role into the gin context. It verifies the action gate (RequirePermission)
// in isolation — service-layer scope is tested separately in the services package.
func buildClientsGuardRouter(roleID int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role_id", roleID)
		c.Set("user_id", 99)
		c.Next()
	})
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	g := r.Group("/clients")
	{
		g.POST("", middleware.RequirePermission("clients.create", "client"), ok)
		g.GET("", middleware.RequirePermission("clients.view", "client"), ok)
		g.PUT("/:id", middleware.RequirePermission("clients.update", "client"), ok)
		g.PATCH("/:id", middleware.RequirePermission("clients.update", "client"), ok)
	}
	return r
}

func clientsGuardStatus(r *gin.Engine, method, path string) int {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w.Code
}

// TestClientsRouteGuard_HRNoAccess: hr has no clients.* → 403 on create and view.
func TestClientsRouteGuard_HRNoAccess(t *testing.T) {
	r := buildClientsGuardRouter(authz.RoleHR)
	if code := clientsGuardStatus(r, http.MethodPost, "/clients"); code != http.StatusForbidden {
		t.Errorf("hr POST /clients: want 403, got %d", code)
	}
	if code := clientsGuardStatus(r, http.MethodGet, "/clients"); code != http.StatusForbidden {
		t.Errorf("hr GET /clients: want 403, got %d", code)
	}
}

// TestClientsRouteGuard_LegalViewOnly: legal reads (sees all) but cannot create/update the record.
func TestClientsRouteGuard_LegalViewOnly(t *testing.T) {
	r := buildClientsGuardRouter(authz.RoleLegal)
	if code := clientsGuardStatus(r, http.MethodGet, "/clients"); code != http.StatusOK {
		t.Errorf("legal GET /clients: want 200, got %d", code)
	}
	if code := clientsGuardStatus(r, http.MethodPut, "/clients/1"); code != http.StatusForbidden {
		t.Errorf("legal PUT /clients/:id: want 403, got %d", code)
	}
	if code := clientsGuardStatus(r, http.MethodPatch, "/clients/1"); code != http.StatusForbidden {
		t.Errorf("legal PATCH /clients/:id: want 403, got %d", code)
	}
	if code := clientsGuardStatus(r, http.MethodPost, "/clients"); code != http.StatusForbidden {
		t.Errorf("legal POST /clients: want 403, got %d", code)
	}
}

// TestClientsRouteGuard_QCReadOnly: quality_control may view but not create/update (no grant).
func TestClientsRouteGuard_QCReadOnly(t *testing.T) {
	r := buildClientsGuardRouter(authz.RoleControl)
	if code := clientsGuardStatus(r, http.MethodGet, "/clients"); code != http.StatusOK {
		t.Errorf("qc GET /clients: want 200, got %d", code)
	}
	if code := clientsGuardStatus(r, http.MethodPost, "/clients"); code != http.StatusForbidden {
		t.Errorf("qc POST /clients: want 403, got %d", code)
	}
	if code := clientsGuardStatus(r, http.MethodPut, "/clients/1"); code != http.StatusForbidden {
		t.Errorf("qc PUT /clients/:id: want 403, got %d", code)
	}
}

// TestClientsRouteGuard_ScopedWritersPass: sales/partner create+update and visa update
// still pass the action gate (scope is then enforced by the service).
func TestClientsRouteGuard_ScopedWritersPass(t *testing.T) {
	checks := []struct {
		role   int
		label  string
		method string
		path   string
	}{
		{authz.RoleSales, "sales create", http.MethodPost, "/clients"},
		{authz.RoleSales, "sales update", http.MethodPut, "/clients/1"},
		{authz.RoleVisa, "visa update", http.MethodPut, "/clients/1"},
		{authz.RolePartner, "partner create", http.MethodPost, "/clients"},
		{authz.RolePartner, "partner update", http.MethodPut, "/clients/1"},
		{authz.RoleManagement, "management create", http.MethodPost, "/clients"},
		{authz.RoleSystemAdmin, "admin create", http.MethodPost, "/clients"},
	}
	for _, c := range checks {
		r := buildClientsGuardRouter(c.role)
		if code := clientsGuardStatus(r, c.method, c.path); code != http.StatusOK {
			t.Errorf("%s: want 200 (action gate passes), got %d", c.label, code)
		}
	}
}
