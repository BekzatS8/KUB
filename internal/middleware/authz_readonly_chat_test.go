package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
)

func TestReadOnlyGuard_AllowsNarrowChatWritesForControl(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role_id", authz.RoleControl)
		c.Next()
	})
	r.Use(ReadOnlyGuard())

	r.POST("/chats/personal", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/chats/:id/messages", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/chats/:id/read", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, path := range []string{"/chats/personal", "/chats/12/messages", "/chats/12/read"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("path=%s expected 200, got %d", path, w.Code)
		}
	}
}

func TestReadOnlyGuard_StillBlocksBusinessWritesForControl(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role_id", authz.RoleControl)
		c.Next()
	})
	r.Use(ReadOnlyGuard())

	r.POST("/clients", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/clients", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for business write, got %d", w.Code)
	}
}
