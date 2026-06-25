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

// TestReadOnlyGuard_AllowsDocumentActionsAndFeedForControl verifies ОКК can act
// on existing documents and submit approval requests to the feed, but cannot hit
// document-creation endpoints directly (those must go through admin approval).
func TestReadOnlyGuard_AllowsDocumentActionsAndFeedForControl(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role_id", authz.RoleControl)
		c.Next()
	})
	r.Use(ReadOnlyGuard())

	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	r.POST("/feed-events", ok)
	r.POST("/api/v1/feed-events", ok)
	r.POST("/documents/:id/archive", ok)
	r.POST("/documents/:id/send-for-signature", ok)
	r.POST("/documents", ok)
	r.POST("/documents/upload", ok)
	r.POST("/documents/create-from-client", ok)

	allowed := []string{"/feed-events", "/api/v1/feed-events", "/documents/12/archive", "/documents/12/send-for-signature"}
	for _, path := range allowed {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("path=%s expected 200 (allowed for ОКК), got %d", path, w.Code)
		}
	}

	// Direct document creation must stay blocked for ОКК.
	blocked := []string{"/documents", "/documents/upload", "/documents/create-from-client"}
	for _, path := range blocked {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("path=%s expected 403 (creation blocked for ОКК), got %d", path, w.Code)
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
