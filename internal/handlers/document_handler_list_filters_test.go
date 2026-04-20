package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDocumentListFilterFromQuery_ParsesExpectedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/documents?q=contract&status=signed&doc_type=addendum_korea&deal_id=25&client_id=16&client_type=individual&branch_id=9&sort_by=doc_type&order=asc", nil)

	filter, err := documentListFilterFromQuery(c)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if filter.Query != "contract" || filter.Status != "signed" || filter.DocType != "addendum_korea" || filter.ClientType != "individual" || filter.SortBy != "doc_type" || filter.Order != "asc" {
		t.Fatalf("unexpected filter: %+v", filter)
	}
	if filter.DealID == nil || *filter.DealID != 25 {
		t.Fatalf("expected deal_id=25, got %+v", filter.DealID)
	}
	if filter.ClientID == nil || *filter.ClientID != 16 {
		t.Fatalf("expected client_id=16, got %+v", filter.ClientID)
	}
	if filter.BranchID == nil || *filter.BranchID != 9 {
		t.Fatalf("expected branch_id=9, got %+v", filter.BranchID)
	}
}

func TestDocumentListFilterFromQuery_InvalidParams(t *testing.T) {
	tests := []string{
		"/documents?deal_id=oops",
		"/documents?client_id=oops",
		"/documents?client_type=corp",
		"/documents?sort_by=amount",
		"/documents?order=up",
		"/documents?status=unknown",
		"/documents?branch_id=oops",
	}
	for _, url := range tests {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", url, nil)
		if _, err := documentListFilterFromQuery(c); err == nil {
			t.Fatalf("expected error for url %s", url)
		}
	}
}
