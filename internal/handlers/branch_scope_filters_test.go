package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLeadListFilterFromQuery_BranchID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/leads?branch_id=3", nil)
	f, err := leadListFilterFromQuery(c)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if f.BranchID == nil || *f.BranchID != 3 {
		t.Fatalf("expected branch_id=3, got %+v", f.BranchID)
	}
}

func TestDealListFilterFromQuery_BranchID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/deals?branch_id=5", nil)
	f, err := dealListFilterFromQuery(c)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if f.BranchID == nil || *f.BranchID != 5 {
		t.Fatalf("expected branch_id=5, got %+v", f.BranchID)
	}
}

func TestTaskFilterFromQuery_BranchID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/tasks?branch_id=2", nil)
	f, err := taskFilterFromQuery(c)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if f.BranchID == nil || *f.BranchID != 2 {
		t.Fatalf("expected branch_id=2, got %+v", f.BranchID)
	}
}

func TestClientFilterFromQuery_BranchID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/clients?branch_id=4", nil)
	f, err := clientListFilterFromQuery(c)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if f.BranchID == nil || *f.BranchID != 4 {
		t.Fatalf("expected branch_id=4, got %+v", f.BranchID)
	}
}
