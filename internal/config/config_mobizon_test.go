package config

import "testing"

func TestApplyEnvOverridesReadsMobizonAPIURL(t *testing.T) {
	t.Setenv("MOBIZON_API_URL", "https://api.mobizon.kz/service")
	t.Setenv("MOBIZON_API_KEY", "secret")
	t.Setenv("MOBIZON_FROM", "KUB")

	cfg := &Config{}
	applyEnvOverrides(cfg)

	if cfg.Mobizon.BaseURL != "https://api.mobizon.kz/service" {
		t.Fatalf("Mobizon.BaseURL = %q", cfg.Mobizon.BaseURL)
	}
	if cfg.Mobizon.APIKey != "secret" {
		t.Fatal("Mobizon.APIKey was not read from env")
	}
	if cfg.Mobizon.From != "KUB" {
		t.Fatalf("Mobizon.From = %q", cfg.Mobizon.From)
	}
	if !cfg.Mobizon.Enabled {
		t.Fatal("Mobizon.Enabled should be true when MOBIZON_API_KEY is set")
	}
	if cfg.Mobizon.DryRun {
		t.Fatal("Mobizon.DryRun should be false when MOBIZON_API_KEY is set")
	}
}

func TestApplyEnvOverridesAllowsExplicitMobizonDisable(t *testing.T) {
	t.Setenv("MOBIZON_API_KEY", "secret")
	t.Setenv("MOBIZON_ENABLED", "false")
	t.Setenv("MOBIZON_DRY_RUN", "true")

	cfg := &Config{}
	applyEnvOverrides(cfg)

	if cfg.Mobizon.Enabled {
		t.Fatal("Mobizon.Enabled should honor explicit MOBIZON_ENABLED=false")
	}
	if !cfg.Mobizon.DryRun {
		t.Fatal("Mobizon.DryRun should honor explicit MOBIZON_DRY_RUN=true")
	}
}
