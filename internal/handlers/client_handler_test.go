package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/services"
)

func TestClientCreateMissingRedFieldsReturns400WithMissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewClientHandler(services.NewClientService(nil))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 11)
		c.Set("role_id", authz.RoleManagement)
		c.Next()
	})
	r.POST("/clients", h.Create)

	body := `{"last_name":"Иванов","first_name":"Иван","phone":"+7 777 000 00 00"}`
	req := httptest.NewRequest(http.MethodPost, "/clients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", w.Code, w.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	missingRaw, ok := payload["missing_fields"].([]any)
	if !ok {
		t.Fatalf("missing_fields is not array: %#v", payload["missing_fields"])
	}
	missing := map[string]bool{}
	for _, item := range missingRaw {
		if s, ok := item.(string); ok {
			missing[s] = true
		}
	}
	for _, field := range []string{"country", "trip_purpose", "birth_date"} {
		if !missing[field] {
			t.Fatalf("expected missing_fields to contain %q, got %#v", field, missing)
		}
	}
}
