package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseOptionalBranchID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("empty branch", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/reports/funnel?from=2026-01-01&to=2026-01-02", nil)
		branchID, ok := parseOptionalBranchID(c)
		if !ok {
			t.Fatalf("expected ok for empty branch_id")
		}
		if branchID != nil {
			t.Fatalf("expected nil branch for empty query, got %v", *branchID)
		}
	})

	t.Run("valid branch", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/reports/funnel?branch_id=4", nil)
		branchID, ok := parseOptionalBranchID(c)
		if !ok {
			t.Fatalf("expected ok for valid branch_id")
		}
		if branchID == nil || *branchID != 4 {
			t.Fatalf("expected branch_id=4, got %v", branchID)
		}
	})

	t.Run("invalid branch", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/reports/funnel?branch_id=abc", nil)
		branchID, ok := parseOptionalBranchID(c)
		if ok {
			t.Fatalf("expected invalid branch_id to fail")
		}
		if branchID != nil {
			t.Fatalf("expected nil branch on invalid value")
		}
	})
}
