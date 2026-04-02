package repositories

import "testing"

func TestHashRefreshTokenDeterministic(t *testing.T) {
	token := "  sample-refresh-token  "
	h1 := hashRefreshToken(token)
	h2 := hashRefreshToken("sample-refresh-token")
	if h1 == "" || h2 == "" {
		t.Fatal("expected non-empty hash")
	}
	if h1 != h2 {
		t.Fatalf("expected deterministic hash, got %q and %q", h1, h2)
	}
	if !isHashedRefreshToken(h1) {
		t.Fatalf("expected %q to have hash prefix", h1)
	}
}

func TestHashRefreshTokenRejectsEmpty(t *testing.T) {
	if got := hashRefreshToken("  "); got != "" {
		t.Fatalf("expected empty hash for empty token, got %q", got)
	}
	if isHashedRefreshToken("plain-token") {
		t.Fatal("plain token must not be detected as hashed")
	}
}
