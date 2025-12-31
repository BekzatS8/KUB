package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMissingFile(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))

	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected error for missing config file")
	}
}

func TestLoadConfigExample(t *testing.T) {
	examplePath := filepath.Join("..", "..", "config", "config.example.yaml")
	absPath, err := filepath.Abs(examplePath)
	if err != nil {
		t.Fatalf("failed to resolve example path: %v", err)
	}

	t.Setenv("CONFIG_PATH", absPath)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load example config: %v", err)
	}

	if cfg.Server.Port != 4000 {
		t.Fatalf("expected port 4000, got %d", cfg.Server.Port)
	}
	if cfg.Database.DSN == "" {
		t.Fatalf("expected non-empty database.dsn")
	}
	if !cfg.Telegram.Enable {
		t.Fatalf("expected telegram.enable=true")
	}
}

func TestValidateConfigAddsSSLModesInDev(t *testing.T) {
	t.Setenv("GIN_MODE", "")
	cfg := &Config{}
	cfg.Server.Port = 4000
	cfg.Database.DSN = "postgres://user:pass@localhost:5432/dbname"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if cfg.Database.DSN != "postgres://user:pass@localhost:5432/dbname?sslmode=disable" {
		t.Fatalf("expected sslmode=disable appended, got %q", cfg.Database.DSN)
	}
}

func TestValidateConfigKeepsSSLModes(t *testing.T) {
	t.Setenv("GIN_MODE", "")
	cfg := &Config{}
	cfg.Server.Port = 4000
	cfg.Database.DSN = "postgres://user:pass@localhost:5432/dbname?sslmode=require"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if cfg.Database.DSN != "postgres://user:pass@localhost:5432/dbname?sslmode=require" {
		t.Fatalf("unexpected DSN after validation: %q", cfg.Database.DSN)
	}
}

func TestValidateConfigRejectsInvalidSSLModes(t *testing.T) {
	t.Setenv("GIN_MODE", "")
	cfg := &Config{}
	cfg.Server.Port = 4000
	cfg.Database.DSN = "postgres://user:pass@localhost:5432/dbname?sslmode=утфиду"

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error for invalid sslmode")
	}
}

func TestLoadConfigDatabaseURLFallback(t *testing.T) {
	configContent := []byte(`server:
  port: 4000
telegram:
  enable: true
database:
  url: "postgres://user:pass@localhost:5432/dbname?sslmode=disable"
`)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, configContent, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	t.Setenv("CONFIG_PATH", configPath)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Database.DSN != "postgres://user:pass@localhost:5432/dbname?sslmode=disable" {
		t.Fatalf("expected dsn from database.url, got %q", cfg.Database.DSN)
	}
}

func TestLoadConfigDbDsnPriority(t *testing.T) {
	configContent := []byte(`server:
  port: 4000
telegram:
  enable: true
database:
  url: "postgres://user:pass@localhost:5432/url_db?sslmode=disable"
db:
  dsn: "postgres://user:pass@localhost:5432/db_db?sslmode=disable"
`)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, configContent, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	t.Setenv("CONFIG_PATH", configPath)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Database.DSN != "postgres://user:pass@localhost:5432/db_db?sslmode=disable" {
		t.Fatalf("expected dsn from db.dsn, got %q", cfg.Database.DSN)
	}
}
