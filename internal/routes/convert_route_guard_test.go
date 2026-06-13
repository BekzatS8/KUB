package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/middleware"
)

// buildConvertGuardRouter wires the SAME RequirePermission("deals.create") guard used
// on the lead→deal convert routes in SetupRoutes, with stub 200 handlers, injecting
// the given role. It verifies the action gate (Defect 1) in isolation.
func buildConvertGuardRouter(roleID int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role_id", roleID)
		c.Set("user_id", 99)
		c.Next()
	})
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	g := r.Group("/leads")
	{
		g.PUT("/:id/convert", middleware.RequirePermission("deals.create", "deal"), ok)
		g.PUT("/:id/convert-with-client", middleware.RequirePermission("deals.create", "deal"), ok)
	}
	return r
}

func convertGuardStatus(r *gin.Engine, path string) int {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, path, nil))
	return w.Code
}

// TestConvertRouteGuard_DeniedRoles: visa/partner/qc/hr/legal have no deals.create → 403
// on both convert routes (they cannot create deals via conversion).
func TestConvertRouteGuard_DeniedRoles(t *testing.T) {
	denied := []struct {
		role  int
		label string
	}{
		{authz.RoleVisa, "visa"},
		{authz.RolePartner, "partner"},
		{authz.RoleControl, "quality_control"},
		{authz.RoleHR, "hr"},
		{authz.RoleLegal, "legal"},
	}
	for _, tc := range denied {
		r := buildConvertGuardRouter(tc.role)
		if code := convertGuardStatus(r, "/leads/1/convert"); code != http.StatusForbidden {
			t.Errorf("%s PUT /leads/:id/convert: want 403, got %d", tc.label, code)
		}
		if code := convertGuardStatus(r, "/leads/1/convert-with-client"); code != http.StatusForbidden {
			t.Errorf("%s PUT /leads/:id/convert-with-client: want 403, got %d", tc.label, code)
		}
	}
}

// TestConvertRouteGuard_AllowedRoles: sales/management/admin pass the action gate
// (lead scope is then still enforced inside the service).
func TestConvertRouteGuard_AllowedRoles(t *testing.T) {
	allowed := []struct {
		role  int
		label string
	}{
		{authz.RoleSales, "sales"},
		{authz.RoleManagement, "management"},
		{authz.RoleSystemAdmin, "admin"},
	}
	for _, tc := range allowed {
		r := buildConvertGuardRouter(tc.role)
		if code := convertGuardStatus(r, "/leads/1/convert"); code != http.StatusOK {
			t.Errorf("%s PUT /leads/:id/convert: want 200 (gate passes), got %d", tc.label, code)
		}
		if code := convertGuardStatus(r, "/leads/1/convert-with-client"); code != http.StatusOK {
			t.Errorf("%s PUT /leads/:id/convert-with-client: want 200 (gate passes), got %d", tc.label, code)
		}
	}
}
