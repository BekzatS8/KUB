package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignatureConfirmationsChannelMigrationIncludesSMS(t *testing.T) {
	path := filepath.Join("029_signature_confirmations_add_sms_channel.up.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	s := string(b)

	checks := []string{
		"DROP CONSTRAINT IF EXISTS signature_confirmations_channel_chk",
		"CHECK (channel IN ('email', 'telegram', 'sms'))",
	}
	for _, check := range checks {
		if !strings.Contains(s, check) {
			t.Fatalf("migration missing fragment %q", check)
		}
	}
}
