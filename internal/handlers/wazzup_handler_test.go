package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	wz "turcompany/internal/integrations/wazzup"
)

type stubWazzupService struct{ called bool }

func (s *stubWazzupService) Setup(ctx context.Context, ownerUserID int, webhooksBaseURL string, enabled bool) (*wz.SetupResponse, error) {
	s.called = true
	return &wz.SetupResponse{WebhookURL: webhooksBaseURL}, nil
}
func (s *stubWazzupService) GetIframeURL(context.Context, int, int, string, string, int, int) (string, error) {
	return "", nil
}
func (s *stubWazzupService) HandleWebhook(context.Context, string, string, []byte) (int, bool, error) {
	return 0, false, nil
}
func (s *stubWazzupService) SendMessage(context.Context, int, string, string) (*wz.SendMessageResponse, error) {
	return &wz.SendMessageResponse{}, nil
}

func TestWazzupSetupAllowsEmptyWebhookBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubWazzupService{}
	h := NewWazzupHandler(svc)

	r := gin.New()
	r.POST("/setup", func(c *gin.Context) {
		c.Set("user_id", 1)
		c.Set("role_id", 50) // system admin
		h.Setup(c)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !svc.called {
		t.Fatal("expected service setup call")
	}
}

func TestWazzupSetupForbiddenForLeadership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubWazzupService{}
	h := NewWazzupHandler(svc)

	r := gin.New()
	r.POST("/setup", func(c *gin.Context) {
		c.Set("user_id", 1)
		c.Set("role_id", 40) // leadership
		h.Setup(c)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.called {
		t.Fatal("service setup must not be called for leadership role")
	}
}

func TestWazzupSetupForbiddenForOrdinaryRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubWazzupService{}
	h := NewWazzupHandler(svc)

	r := gin.New()
	r.POST("/setup", func(c *gin.Context) {
		c.Set("user_id", 1)
		c.Set("role_id", 10) // sales
		h.Setup(c)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
	if svc.called {
		t.Fatal("service setup must not be called for ordinary role")
	}
}
