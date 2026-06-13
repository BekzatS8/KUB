package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/middleware"
)

// buildBranchesGuardRouter wires the SAME guards used on the /branches group:
// view = branches.view (admin + management), create/update/delete = admin-only grants.
func buildBranchesGuardRouter(roleID int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role_id", roleID)
		c.Set("user_id", 99)
		c.Next()
	})
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	g := r.Group("/branches")
	{
		g.GET("", middleware.RequirePermission("branches.view", "branch"), ok)
		g.POST("", middleware.RequirePermission("branches.create", "branch"), ok)
		g.PUT("/:id", middleware.RequirePermission("branches.update", "branch"), ok)
	}
	return r
}

func branchesGuardStatus(r *gin.Engine, method, path string) int {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w.Code
}

// B1: branch create/update are admin-only — management is denied, admin passes.
func TestBranchesRouteGuard_CreateUpdateAdminOnly(t *testing.T) {
	mgmt := buildBranchesGuardRouter(authz.RoleManagement)
	if code := branchesGuardStatus(mgmt, http.MethodPost, "/branches"); code != http.StatusForbidden {
		t.Errorf("management POST /branches: want 403, got %d", code)
	}
	if code := branchesGuardStatus(mgmt, http.MethodPut, "/branches/1"); code != http.StatusForbidden {
		t.Errorf("management PUT /branches/:id: want 403, got %d", code)
	}

	admin := buildBranchesGuardRouter(authz.RoleSystemAdmin)
	if code := branchesGuardStatus(admin, http.MethodPost, "/branches"); code != http.StatusOK {
		t.Errorf("admin POST /branches: want 200, got %d", code)
	}
	if code := branchesGuardStatus(admin, http.MethodPut, "/branches/1"); code != http.StatusOK {
		t.Errorf("admin PUT /branches/:id: want 200, got %d", code)
	}
}

// B2: GET /branches requires branches.view — admin/management pass, others 403.
func TestBranchesRouteGuard_ViewRequiresPermission(t *testing.T) {
	for _, role := range []struct {
		id    int
		label string
	}{
		{authz.RoleSystemAdmin, "admin"},
		{authz.RoleManagement, "management"},
	} {
		r := buildBranchesGuardRouter(role.id)
		if code := branchesGuardStatus(r, http.MethodGet, "/branches"); code != http.StatusOK {
			t.Errorf("%s GET /branches: want 200, got %d", role.label, code)
		}
	}
	for _, role := range []struct {
		id    int
		label string
	}{
		{authz.RoleSales, "sales"},
		{authz.RoleVisa, "visa"},
		{authz.RolePartner, "partner"},
		{authz.RoleControl, "quality_control"},
		{authz.RoleHR, "hr"},
		{authz.RoleLegal, "legal"},
	} {
		r := buildBranchesGuardRouter(role.id)
		if code := branchesGuardStatus(r, http.MethodGet, "/branches"); code != http.StatusForbidden {
			t.Errorf("%s GET /branches without branches.view: want 403, got %d", role.label, code)
		}
	}
}
