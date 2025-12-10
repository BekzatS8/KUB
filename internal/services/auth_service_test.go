package services

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"turcompany/internal/middleware"
)

func TestAuthService_GenerateAndValidateAccessToken(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	svc := &authService{
		AccessSecret:  []byte("test-access"),
		RefreshSecret: []byte("test-refresh"),
		AccessTTL:     time.Minute,
		RefreshTTL:    5 * time.Minute,
		now:           func() time.Time { return fixedNow },
	}

	tokenStr, exp, err := svc.GenerateAccessToken(42, 7)
	if err != nil {
		t.Fatalf("GenerateAccessToken returned error: %v", err)
	}

	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		return svc.AccessSecret, nil
	}, jwt.WithTimeFunc(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatalf("ParseWithClaims returned error: %v", err)
	}
	if !token.Valid {
		t.Fatalf("token should be valid")
	}

	if claims.UserID != 42 {
		t.Errorf("userID mismatch: got %d want %d", claims.UserID, 42)
	}
	if claims.RoleID != 7 {
		t.Errorf("roleID mismatch: got %d want %d", claims.RoleID, 7)
	}

	expectedExp := fixedNow.Add(svc.AccessTTL)
	if !claims.ExpiresAt.Time.Equal(expectedExp) {
		t.Errorf("exp mismatch: got %s want %s", claims.ExpiresAt.Time, expectedExp)
	}
	if !exp.Equal(expectedExp) {
		t.Errorf("returned expiration mismatch: got %s want %s", exp, expectedExp)
	}
}

func TestAuthService_ExpiredToken(t *testing.T) {
	current := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	svc := &authService{
		AccessSecret:  []byte("test-access"),
		RefreshSecret: []byte("test-refresh"),
		AccessTTL:     time.Minute,
		RefreshTTL:    5 * time.Minute,
		now:           func() time.Time { return current },
	}

	tokenStr, _, err := svc.GenerateAccessToken(1, 2)
	if err != nil {
		t.Fatalf("GenerateAccessToken returned error: %v", err)
	}

	current = current.Add(2 * time.Minute)

	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		return svc.AccessSecret, nil
	}, jwt.WithTimeFunc(func() time.Time { return current }))
	if err == nil && token.Valid {
		t.Fatalf("expected token to be invalid after expiration")
	}
}

func TestAuthService_RefreshTokenGeneration(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	svc := &authService{
		AccessSecret:  []byte("test-access"),
		RefreshSecret: []byte("test-refresh"),
		AccessTTL:     time.Minute,
		RefreshTTL:    5 * time.Minute,
		now:           func() time.Time { return fixedNow },
	}

	rt1, exp1, err := svc.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken returned error: %v", err)
	}
	if rt1 == "" {
		t.Fatalf("refresh token should not be empty")
	}
	expectedExp := fixedNow.Add(svc.RefreshTTL)
	if !exp1.Equal(expectedExp) {
		t.Errorf("refresh expiration mismatch: got %s want %s", exp1, expectedExp)
	}

	rt2, _, err := svc.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("second GenerateRefreshToken returned error: %v", err)
	}
	if rt1 == rt2 {
		t.Errorf("refresh tokens should be unique between generations")
	}
}
