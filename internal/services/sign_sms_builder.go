package services

import (
	"fmt"
	"strings"
	"time"
)

func BuildSigningSMS(docRef, otpCode, signURL string, expiresAt time.Time) string {
	docRef = strings.TrimSpace(docRef)
	if docRef == "" {
		docRef = "документ"
	}
	exp := expiresAt.UTC().Format("15:04 MST")
	text := fmt.Sprintf("Код подписи: %s. %s. Ссылка: %s. Действует до %s.", strings.TrimSpace(otpCode), docRef, strings.TrimSpace(signURL), exp)
	if len(text) <= 320 {
		return text
	}
	trimmedURL := strings.TrimSpace(signURL)
	if len(trimmedURL) > 80 {
		trimmedURL = trimmedURL[:80]
	}
	text = fmt.Sprintf("Код: %s. %s. %s", strings.TrimSpace(otpCode), docRef, trimmedURL)
	if len(text) > 320 {
		return text[:320]
	}
	return text
}
