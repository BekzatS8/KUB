package services

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func IsBase64Signature(s string) bool {
	return strings.HasPrefix(s, "data:image/")
}

func SaveSignatureImage(base64Data string, eventID string, uploadsDir string) (string, error) {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(base64Data, prefix) {
		return "", fmt.Errorf("signature_image: unsupported format")
	}
	raw := strings.TrimPrefix(base64Data, prefix)
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("signature_image: base64 decode: %w", err)
	}
	dir := filepath.Join(uploadsDir, "signatures")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("signature_image: mkdir: %w", err)
	}
	filename := fmt.Sprintf("sig_%s.png", eventID)
	fullPath := filepath.Join(dir, filename)
	if err := os.WriteFile(fullPath, decoded, 0644); err != nil {
		return "", fmt.Errorf("signature_image: write file: %w", err)
	}
	return filepath.Join("signatures", filename), nil
}
