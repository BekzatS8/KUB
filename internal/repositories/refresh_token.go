package repositories

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const refreshTokenHashPrefix = "sha256:"

func hashRefreshToken(token string) string {
	normalized := strings.TrimSpace(token)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return refreshTokenHashPrefix + hex.EncodeToString(sum[:])
}

func isHashedRefreshToken(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), refreshTokenHashPrefix)
}
