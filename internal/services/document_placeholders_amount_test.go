package services

import (
	"strings"
	"testing"
)

func TestNormalizeMoney(t *testing.T) {
	tests := []struct {
		name      string
		in        any
		wantTenge int64
		wantTiyn  int64
		wantFmt   string
		wantWords string
	}{
		{
			name:      "db numeric string",
			in:        "100000.00",
			wantTenge: 100000,
			wantTiyn:  0,
			wantFmt:   "100 000.00",
			wantWords: "сто тысяч",
		},
		{
			name:      "spaced millions",
			in:        "10 000 000.00",
			wantTenge: 10000000,
			wantTiyn:  0,
			wantFmt:   "10 000 000.00",
			wantWords: "десять миллионов",
		},
		{
			name:      "en thousands separators",
			in:        "1,234,567.89",
			wantTenge: 1234567,
			wantTiyn:  89,
			wantFmt:   "1 234 567.89",
			wantWords: "один миллион двести тридцать четыре тысячи пятьсот шестьдесят семь",
		},
		{
			name:      "eu thousands separators",
			in:        "1.234.567,89",
			wantTenge: 1234567,
			wantTiyn:  89,
			wantFmt:   "1 234 567.89",
			wantWords: "один миллион двести тридцать четыре тысячи пятьсот шестьдесят семь",
		},
		{
			name:      "integer",
			in:        "100000",
			wantTenge: 100000,
			wantTiyn:  0,
			wantFmt:   "100 000.00",
			wantWords: "сто тысяч",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenge, tiyn, formatted, err := NormalizeMoney(tt.in)
			if err != nil {
				t.Fatalf("NormalizeMoney() error = %v", err)
			}
			if tenge != tt.wantTenge || tiyn != tt.wantTiyn {
				t.Fatalf("unexpected normalized value, got %d.%02d", tenge, tiyn)
			}
			if formatted != tt.wantFmt {
				t.Fatalf("formatted mismatch, got %q want %q", formatted, tt.wantFmt)
			}
			if got := amountToRuWords(tenge, tiyn); !strings.Contains(got, tt.wantWords) {
				t.Fatalf("words mismatch, got %q must contain %q", got, tt.wantWords)
			}
		})
	}
}

func TestNormalizeMoney_InvalidInput(t *testing.T) {
	if _, _, _, err := NormalizeMoney("12..34"); err == nil {
		t.Fatalf("expected error for invalid input")
	}
}
