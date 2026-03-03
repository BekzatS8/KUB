package services

import (
	"testing"
	"time"

	"turcompany/internal/models"
)

func TestBuildClientPlaceholders_HasBaseClientFields(t *testing.T) {
	now := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	client := &models.Client{
		Name:        "Иванов Иван Иванович",
		IIN:         "990101300123",
		Address:     "г. Алматы, ул. Абая, 1",
		Phone:       "8 (701) 123-45-67",
		Email:       "ivanov@example.com",
		ContactInfo: "+77011234567, ivanov@example.com",
	}

	ph := buildClientPlaceholders(client, nil, nil, now)

	mustNonEmpty := []string{"CLIENT_PHONE", "CLIENT_EMAIL", "CLIENT_ADDRESS", "CLIENT_IIN"}
	for _, key := range mustNonEmpty {
		if ph[key] == "" {
			t.Fatalf("%s must be non-empty", key)
		}
	}

	if ph["CLIENT_PHONE"] != "+77011234567" {
		t.Fatalf("CLIENT_PHONE normalization mismatch: %q", ph["CLIENT_PHONE"])
	}

	if ph["CLIENT_FULL_NAME_SHORT"] == "" {
		t.Fatalf("CLIENT_FULL_NAME_SHORT must be filled")
	}

	if ph["CLIENT_FIO"] != ph["CLIENT_FULL_NAME"] {
		t.Fatalf("CLIENT_FIO alias mismatch: %q vs %q", ph["CLIENT_FIO"], ph["CLIENT_FULL_NAME"])
	}
	if ph["CLIENT_FIO_SHORT"] != ph["CLIENT_FULL_NAME_SHORT"] {
		t.Fatalf("CLIENT_FIO_SHORT alias mismatch: %q vs %q", ph["CLIENT_FIO_SHORT"], ph["CLIENT_FULL_NAME_SHORT"])
	}
}

func TestBuildClientPlaceholders_ExtraOverridesOnlyNonEmpty(t *testing.T) {
	now := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	client := &models.Client{
		Name:    "Иванов Иван Иванович",
		Phone:   "87011234567",
		Email:   "ivanov@example.com",
		Address: "г. Алматы, ул. Абая, 1",
		IIN:     "990101300123",
	}
	extra := map[string]string{
		"CLIENT_PHONE": "   ",
		"CLIENT_EMAIL": "new@example.com",
	}

	ph := buildClientPlaceholders(client, nil, extra, now)

	if ph["CLIENT_PHONE"] != "+77011234567" {
		t.Fatalf("empty extra must not override CLIENT_PHONE, got %q", ph["CLIENT_PHONE"])
	}
	if ph["CLIENT_EMAIL"] != "new@example.com" {
		t.Fatalf("non-empty extra must override CLIENT_EMAIL, got %q", ph["CLIENT_EMAIL"])
	}
}
