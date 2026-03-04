package services

import (
	"strings"
	"testing"
	"time"

	"turcompany/internal/models"
)

func TestBuildClientPlaceholders_NormalizesAmountWordsWithoutCurrencyAndTiyn(t *testing.T) {
	deal := &models.Deals{
		ID:        1,
		Amount:    1000568.00,
		CreatedAt: time.Now(),
	}

	ph := buildClientPlaceholders(nil, deal, nil, time.Now())
	for _, key := range []string{"DEAL_AMOUNT_TEXT", "TOTAL_AMOUNT_TEXT", "DEAL_TOTAL_KZT_TEXT", "AMOUNT_KZT_TEXT"} {
		got := strings.ToLower(ph[key])
		if strings.Contains(got, "тенге") {
			t.Fatalf("placeholder %s still contains тенге: %q", key, ph[key])
		}
		if strings.Contains(got, "тиын") {
			t.Fatalf("placeholder %s still contains тиын: %q", key, ph[key])
		}
	}
}

func TestNormalizeKZTWords(t *testing.T) {
	got := normalizeKZTWords("один миллион пятьсот шестьдесят восемь тенге 00 тиын .")
	if strings.Contains(strings.ToLower(got), "тенге") || strings.Contains(strings.ToLower(got), "тиын") {
		t.Fatalf("normalizeKZTWords did not clean currency words: %q", got)
	}
	if got != "один миллион пятьсот шестьдесят восемь." {
		t.Fatalf("unexpected normalized value: %q", got)
	}
}
