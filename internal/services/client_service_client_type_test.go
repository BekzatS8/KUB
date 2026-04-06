package services

import (
	"errors"
	"testing"

	"turcompany/internal/models"
)

func TestEnsureClientTypeImmutable_IndividualToLegalRejected(t *testing.T) {
	_, err := ensureClientTypeImmutable(models.ClientTypeIndividual, models.ClientTypeLegal)
	if !errors.Is(err, ErrClientTypeImmutable) {
		t.Fatalf("expected ErrClientTypeImmutable, got %v", err)
	}
}

func TestEnsureClientTypeImmutable_LegalToIndividualRejected(t *testing.T) {
	_, err := ensureClientTypeImmutable(models.ClientTypeLegal, models.ClientTypeIndividual)
	if !errors.Is(err, ErrClientTypeImmutable) {
		t.Fatalf("expected ErrClientTypeImmutable, got %v", err)
	}
}

func TestEnsureClientTypeImmutable_SameTypeAllowed(t *testing.T) {
	got, err := ensureClientTypeImmutable(models.ClientTypeLegal, models.ClientTypeLegal)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != models.ClientTypeLegal {
		t.Fatalf("expected legal, got %q", got)
	}
}

func TestEnsureClientTypeImmutable_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		stored    string
		requested string
		want      string
		wantErr   bool
	}{
		{name: "keep legal", stored: models.ClientTypeLegal, requested: models.ClientTypeLegal, want: models.ClientTypeLegal},
		{name: "keep individual by empty requested", stored: models.ClientTypeIndividual, requested: "", want: models.ClientTypeIndividual},
		{name: "reject legal to individual", stored: models.ClientTypeLegal, requested: models.ClientTypeIndividual, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ensureClientTypeImmutable(tt.stored, tt.requested)
			if tt.wantErr {
				if !errors.Is(err, ErrClientTypeImmutable) {
					t.Fatalf("expected ErrClientTypeImmutable, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected type: got=%q want=%q", got, tt.want)
			}
		})
	}
}
