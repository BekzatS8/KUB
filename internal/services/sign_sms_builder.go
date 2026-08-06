package services

import (
	"fmt"
	"strings"
	"time"
)

func BuildSigningSMS(docRef, otpCode, signURL string, expiresAt time.Time, tz *time.Location) string {
	docRef = strings.TrimSpace(docRef)
	if docRef == "" {
		docRef = "документ"
	}
	if tz == nil {
		tz = time.UTC
	}
	// Время истечения — в часовом поясе сервиса (UTC+5, Asia/Almaty), а не в UTC:
	// иначе клиент видит время на 5 часов раньше и думает, что ссылка уже
	// недействительна (обратная связь 31.07.2026).
	exp := expiresAt.In(tz).Format("15:04 02.01")
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
