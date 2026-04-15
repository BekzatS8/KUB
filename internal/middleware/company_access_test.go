package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type companyResolverStub struct {
	hasAccessFn        func(userID, companyID int) (bool, error)
	getActiveFn        func(userID int) (*int, error)
	getActiveCallCount int
}

func (s *companyResolverStub) HasUserAccess(userID, companyID int) (bool, error) {
	if s.hasAccessFn != nil {
		return s.hasAccessFn(userID, companyID)
	}
	return true, nil
}

func (s *companyResolverStub) GetUserActiveCompanyID(userID int) (*int, error) {
	s.getActiveCallCount++
	if s.getActiveFn != nil {
		return s.getActiveFn(userID)
	}
	return nil, nil
}

func TestRequireCompanyAccess_UsesClaimsActiveCompanyFirst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &companyResolverStub{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 42)
		c.Set("active_company_id", 7)
	})
	r.Use(RequireCompanyAccess(resolver))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if resolver.getActiveCallCount != 0 {
		t.Fatalf("expected DB fallback not called, got %d calls", resolver.getActiveCallCount)
	}
}

func TestRequireCompanyAccess_RejectsUncontrolledOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &companyResolverStub{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 42)
		c.Set("active_company_id", 7)
	})
	r.Use(RequireCompanyAccess(resolver))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Company-ID", "9")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRequireCompanyAccess_AllowsControlledOverrideWithAccessCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	resolver := &companyResolverStub{
		hasAccessFn: func(userID, companyID int) (bool, error) {
			called = true
			if userID != 42 || companyID != 9 {
				t.Fatalf("unexpected access check args: user=%d company=%d", userID, companyID)
			}
			return true, nil
		},
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 42)
		c.Set("active_company_id", 7)
	})
	r.Use(RequireCompanyAccess(resolver))
	r.GET("/", func(c *gin.Context) {
		if got, ok := GetActiveCompanyID(c); !ok || got != 9 {
			t.Fatalf("expected active company in context to be 9, got=%d ok=%v", got, ok)
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Company-ID", "9")
	req.Header.Set("X-Company-Override", "true")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Fatal("expected HasUserAccess check to be called")
	}
}

func TestRequireCompanyAccess_FallsBackToDBActiveCompanyWhenClaimsMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &companyResolverStub{
		getActiveFn: func(userID int) (*int, error) {
			v := 11
			return &v, nil
		},
	}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", 42) })
	r.Use(RequireCompanyAccess(resolver))
	r.GET("/", func(c *gin.Context) {
		if got, ok := GetActiveCompanyID(c); !ok || got != 11 {
			t.Fatalf("expected active company in context to be 11, got=%d ok=%v", got, ok)
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if resolver.getActiveCallCount != 1 {
		t.Fatalf("expected one DB fallback call, got %d", resolver.getActiveCallCount)
	}
}
