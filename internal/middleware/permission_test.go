package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestRouter(action, resource string, roleID int, roleCode string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role_id", roleID)
		c.Set("role_code", roleCode)
		c.Set("user_id", 1)
		c.Next()
	})
	r.POST("/funnels", RequirePermission(action, resource), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.PATCH("/leads/:id/funnel", RequirePermission(authz.ActionLeadsMoveBetweenFunnels, "lead"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestRequirePermission_FunnelsCreate(t *testing.T) {
	cases := []struct {
		roleID   int
		roleCode string
		want     int
	}{
		{authz.RoleSystemAdmin, "admin", http.StatusOK},
		{authz.RoleManagement, "management", http.StatusForbidden},
		{authz.RoleControl, "quality_control", http.StatusForbidden},
		{authz.RoleSales, "sales", http.StatusForbidden},
		{authz.RoleVisa, "visa", http.StatusForbidden},
		{authz.RoleHR, "hr", http.StatusForbidden},
		{authz.RoleLegal, "legal", http.StatusForbidden},
		{authz.RoleOperations, "", http.StatusForbidden}, // legacy role_id=20 — no code, no permission
	}

	for _, tc := range cases {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("role_id", tc.roleID)
			c.Set("user_id", 1)
			c.Next()
		})
		r.POST("/funnels", RequirePermission(authz.ActionFunnelsCreate, "funnel"), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/funnels", nil)
		r.ServeHTTP(w, req)

		if w.Code != tc.want {
			t.Errorf("role_id=%d: POST /funnels: want=%d got=%d", tc.roleID, tc.want, w.Code)
		}
	}
}

func TestRequirePermission_LeadsMoveBetweenFunnels(t *testing.T) {
	cases := []struct {
		roleID int
		want   int
	}{
		{authz.RoleSystemAdmin, http.StatusOK},
		{authz.RoleManagement, http.StatusOK},
		{authz.RoleSales, http.StatusForbidden},
		{authz.RoleVisa, http.StatusForbidden},
		{authz.RolePartner, http.StatusForbidden},
		{authz.RoleControl, http.StatusForbidden},
		{authz.RoleHR, http.StatusForbidden},
		{authz.RoleLegal, http.StatusForbidden},
		{authz.RoleOperations, http.StatusForbidden},
	}

	for _, tc := range cases {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("role_id", tc.roleID)
			c.Set("user_id", 1)
			c.Next()
		})
		r.PATCH("/leads/:id/funnel", RequirePermission(authz.ActionLeadsMoveBetweenFunnels, "lead"), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/leads/42/funnel", nil)
		r.ServeHTTP(w, req)

		if w.Code != tc.want {
			t.Errorf("role_id=%d: PATCH /leads/:id/funnel: want=%d got=%d", tc.roleID, tc.want, w.Code)
		}
	}
}

func TestRequirePermission_NoRoleInContext(t *testing.T) {
	r := gin.New()
	// No role set in context
	r.POST("/funnels", RequirePermission(authz.ActionFunnelsCreate, "funnel"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/funnels", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("no role in context: want 401 got %d", w.Code)
	}
}
