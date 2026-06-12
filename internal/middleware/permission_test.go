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
		{999, "", http.StatusForbidden}, // unknown role_id — no code, no permission
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
		{999, http.StatusForbidden}, // unknown role_id
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

func TestRequirePermission_TelephonyView(t *testing.T) {
	// All active roles have telephony.view — all should get 200.
	allowed := []struct {
		roleID int
		label  string
	}{
		{authz.RoleSystemAdmin, "admin"},
		{authz.RoleManagement, "management"},
		{authz.RoleControl, "quality_control"},
		{authz.RoleSales, "sales"},
		{authz.RoleVisa, "visa"},
		{authz.RolePartner, "partner"},
		{authz.RoleHR, "hr"},
		{authz.RoleLegal, "legal"},
	}
	for _, tc := range allowed {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("role_id", tc.roleID); c.Set("user_id", 1); c.Next() })
		r.GET("/telephony/calls", RequirePermission("telephony.view", "telephony"), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/telephony/calls", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("role %s (id=%d): GET /telephony/calls: want 200 got %d", tc.label, tc.roleID, w.Code)
		}
	}

	// Unknown role has no telephony.view — should get 403.
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("role_id", 999); c.Set("user_id", 1); c.Next() })
	r.GET("/telephony/calls", RequirePermission("telephony.view", "telephony"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/telephony/calls", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("unknown role (id=999): GET /telephony/calls: want 403 got %d", w.Code)
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
