package services

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"turcompany/internal/middleware"
)

func TestGenerateAccessToken_DefaultTTLIsAboutTwoHours(t *testing.T) {
	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	svc := NewAuthService([]byte("01234567890123456789012345678901"), nil, 0, 0, func() time.Time {
		return fixedNow
	})

	token, exp, err := svc.GenerateAccessToken(10, 20)
	if err != nil {
		t.Fatalf("GenerateAccessToken returned error: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateAccessToken returned empty token")
	}

	if got := exp.Sub(fixedNow.Add(2 * time.Hour)); got > time.Second || got < -time.Second {
		t.Fatalf("unexpected token exp: got=%s want about=%s", exp, fixedNow.Add(2*time.Hour))
	}

	claims := &middleware.Claims{}
	parsed, err := jwt.NewParser(jwt.WithoutClaimsValidation()).ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte("01234567890123456789012345678901"), nil
	})
	if err != nil {
		t.Fatalf("ParseWithClaims returned error: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("token should be valid")
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatal("expected iat and exp in access token")
	}

	ttl := claims.ExpiresAt.Sub(claims.IssuedAt.Time)
	if diff := ttl - 2*time.Hour; diff > time.Second || diff < -time.Second {
		t.Fatalf("unexpected ttl: got=%s want about=2h", ttl)
	}
}
