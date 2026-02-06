package utils

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var reNonDigits = regexp.MustCompile(`\D+`)

// SanitizeE164Digits приводит номер к формату E.164 без плюса
func SanitizeE164Digits(phone string) (string, error) {
	p := strings.TrimSpace(phone)
	p = reNonDigits.ReplaceAllString(p, "")
	if p == "" {
		return "", errors.New("empty phone")
	}

	// Для Казахстана: заменяем 8 на 7
	if len(p) == 11 && strings.HasPrefix(p, "8") {
		p = "7" + p[1:]
	}

	// Удаляем префикс + если есть
	if strings.HasPrefix(p, "+") {
		p = p[1:]
	}

	// Проверяем длину
	if len(p) < 11 || len(p) > 15 {
		return "", fmt.Errorf("invalid phone length: %d", len(p))
	}

	return p, nil
}
