package utils

import "testing"

func TestMaskDSN(t *testing.T) {
	dsn := "postgres://user:secret@localhost:5432/app?sslmode=disable"
	masked := MaskDSN(dsn)

	if masked == dsn {
		t.Fatalf("expected masked DSN to differ")
	}
	if masked == "invalid-dsn" {
		t.Fatalf("expected valid masked DSN")
	}
	if masked == "" {
		t.Fatalf("expected non-empty masked DSN")
	}
	if masked == "postgres://user:secret@localhost:5432/app?sslmode=disable" {
		t.Fatalf("expected password to be masked")
	}
	if masked != "postgres://localhost:5432/app?sslmode=disable" {
		t.Fatalf("unexpected masked DSN: %s", masked)
	}
}
