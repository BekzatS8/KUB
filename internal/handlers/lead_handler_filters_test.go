package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLeadListFilterFromQuery_UsesQParamNotQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/leads?q=smoke&query=ignored", nil)

	filter, err := leadListFilterFromQuery(c)
	if err != nil {
		t.Fatalf("leadListFilterFromQuery returned error: %v", err)
	}
	if filter.Query != "smoke" {
		t.Fatalf("expected q param to be used, got %q", filter.Query)
	}
}
