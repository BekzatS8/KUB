package services

import (
	"errors"
	"testing"
)

func TestNormalizeRequiredDealClientType_RequiresValue(t *testing.T) {
	_, err := normalizeRequiredDealClientType("")
	if !errors.Is(err, ErrClientTypeRequired) {
		t.Fatalf("expected ErrClientTypeRequired, got %v", err)
	}
}

func TestNormalizeRequiredDealClientType_InvalidValue(t *testing.T) {
	_, err := normalizeRequiredDealClientType("partner")
	if err == nil {
		t.Fatal("expected error for invalid client_type")
	}
}

func TestNormalizeRequiredDealClientType_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "individual", input: "individual", want: "individual"},
		{name: "legal", input: "legal", want: "legal"},
		{name: "trim and lower", input: "  LeGaL  ", want: "legal"},
		{name: "missing", input: " ", wantErr: true},
		{name: "invalid", input: "partner", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeRequiredDealClientType(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("unexpected normalized value: got=%q want=%q", got, tt.want)
			}
		})
	}
}
