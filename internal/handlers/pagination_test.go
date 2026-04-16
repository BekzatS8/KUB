package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNormalizedPageAndSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/x?page=-10&size=200", nil)

	page, size := normalizedPageAndSize(c)
	if page != 1 || size != 100 {
		t.Fatalf("expected normalized (1,100), got (%d,%d)", page, size)
	}
}

func TestBuildPaginationMeta(t *testing.T) {
	meta := buildPaginationMeta(2, 15, 31)
	if meta.TotalPages != 3 || !meta.HasNext || !meta.HasPrev {
		t.Fatalf("unexpected meta: %+v", meta)
	}
}
