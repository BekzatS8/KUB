package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
)

func requestIDFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if val := strings.TrimSpace(c.GetHeader("X-Request-Id")); val != "" {
		return val
	}
	if val := strings.TrimSpace(c.GetHeader("X-Request-ID")); val != "" {
		return val
	}
	if val := strings.TrimSpace(c.GetHeader("X-Correlation-Id")); val != "" {
		return val
	}
	if val := strings.TrimSpace(c.GetHeader("X-Correlation-ID")); val != "" {
		return val
	}
	return ""
}

func redactPrefix(value string, size int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if size <= 0 {
		size = 6
	}
	if len(value) <= size {
		return value
	}
	return value[:size]
}

func hashPrefix(value string, size int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if size <= 0 {
		size = 8
	}
	sum := sha256.Sum256([]byte(value))
	encoded := hex.EncodeToString(sum[:])
	if len(encoded) <= size {
		return encoded
	}
	return encoded[:size]
}
