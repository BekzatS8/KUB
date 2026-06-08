package services

import "testing"

func TestClientPhotoAllowsAnyExtension(t *testing.T) {
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".heic", ".heif", ".bmp", ".tiff", ".avif", ".unknown", ""} {
		if !isAllowedClientFileExtension("photo35x45", ext) {
			t.Fatalf("photo35x45 must allow extension %q", ext)
		}
	}
}

func TestNonPhotoClientFilesKeepExtensionWhitelist(t *testing.T) {
	if !isAllowedClientFileExtension("charter", ".pdf") {
		t.Fatal("non-photo category must allow pdf")
	}
	if isAllowedClientFileExtension("charter", ".heic") {
		t.Fatal("non-photo category must keep extension whitelist")
	}
}
