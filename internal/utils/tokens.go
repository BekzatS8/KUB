package utils

import (
	"crypto/rand"
	"encoding/hex"
)

func NewRefreshToken(nBytes int) (string, error) {
	if nBytes <= 0 {
		nBytes = 32 // 256 бит по умолчанию
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
