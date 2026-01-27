package utils

import "strings"

func ParseBoolEnv(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	switch strings.ToLower(trimmed) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
