package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Обратная связь заказчика 17.09.2026: сотрудник со статусом «Не подтверждён»
// (или заблокированный) не должен работать в системе даже с ещё живым
// access-токеном — access живёт 2 часа, refresh 30 дней.
func TestAuthMiddleware_AccountStatusGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := []byte("01234567890123456789012345678901")

	cases := []struct {
		name     string
		checker  AccountStatusChecker
		wantCode int
	}{
		{
			name:     "активный и подтверждённый — пропускаем",
			checker:  func(int) (bool, bool, error) { return true, true, nil },
			wantCode: http.StatusOK,
		},
		{
			name:     "не подтверждён — 401",
			checker:  func(int) (bool, bool, error) { return true, false, nil },
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "заблокирован — 401",
			checker:  func(int) (bool, bool, error) { return false, true, nil },
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "учётка не найдена — 401",
			checker:  func(int) (bool, bool, error) { return false, false, errors.New("no rows") },
			wantCode: http.StatusUnauthorized,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(NewAuthMiddleware(secret, tc.checker))
			r.GET("/protected", func(c *gin.Context) { c.Status(http.StatusOK) })

			token := signToken(t, secret, time.Now().UTC().Add(-time.Minute), time.Now().UTC().Add(10*time.Minute))
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)
			if w.Code != tc.wantCode {
				t.Fatalf("unexpected status: got=%d want=%d", w.Code, tc.wantCode)
			}
		})
	}
}

// Публичные маршруты не должны дергать проверку учётки вообще.
func TestAuthMiddleware_PublicPathSkipsAccountStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := []byte("01234567890123456789012345678901")

	called := false
	r := gin.New()
	r.Use(NewAuthMiddleware(secret, func(int) (bool, bool, error) {
		called = true
		return false, false, nil
	}))
	r.POST("/auth/login", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/auth/login", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d", w.Code, http.StatusOK)
	}
	if called {
		t.Fatal("account status checker must not run on public paths")
	}
}
