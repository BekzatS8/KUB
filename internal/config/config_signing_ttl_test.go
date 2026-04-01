package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSignSessionTTLDefaultsToSignEmailTTL(t *testing.T) {
	t.Setenv("GIN_MODE", "debug")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := []byte(`server:
  port: 4000
database:
  dsn: "postgres://u:p@localhost:5432/db?sslmode=disable"
sign_email_ttl_minutes: 45
`)
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CONFIG_PATH", cfgPath)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SignSessionTTLMinutes != 45 {
		t.Fatalf("unexpected sign session ttl: got=%d want=45", cfg.SignSessionTTLMinutes)
	}
}
