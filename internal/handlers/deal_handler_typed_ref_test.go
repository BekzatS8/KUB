package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDealCreate_RequiresClientType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/deals", strings.NewReader(`{
		"lead_id": 1,
		"client_id": 2,
		"amount": 1000,
		"currency": "USD"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h := &DealHandler{}
	h.Create(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", w.Code, w.Body.String())
	}
}
