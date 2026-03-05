package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestAuthMiddleware_ValidFutureTokenPasses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := []byte("01234567890123456789012345678901")

	r := gin.New()
	r.Use(NewAuthMiddleware(secret))
	r.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	token := signToken(t, secret, time.Now().UTC().Add(-1*time.Minute), time.Now().UTC().Add(10*time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d", w.Code, http.StatusOK)
	}
}

func TestAuthMiddleware_ExpiredTokenReturns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := []byte("01234567890123456789012345678901")

	r := gin.New()
	r.Use(NewAuthMiddleware(secret))
	r.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	token := signToken(t, secret, time.Now().UTC().Add(-20*time.Minute), time.Now().UTC().Add(-10*time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: got=%d want=%d", w.Code, http.StatusUnauthorized)
	}
}

func signToken(t *testing.T, secret []byte, iat, exp time.Time) string {
	t.Helper()
	claims := &Claims{
		UserID: 1,
		RoleID: 2,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(iat),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString returned error: %v", err)
	}
	return s
}
