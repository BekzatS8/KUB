package config

import "testing"

func TestValidateWazzupDisabledAllowsMissingToken(t *testing.T) {
	cfg := &Config{}
	cfg.Server.Port = 4000
	cfg.Database.DSN = "postgres://u:p@localhost:5432/db?sslmode=disable"
	cfg.SignConfirmPolicy = "ANY"
	cfg.Wazzup.Enable = false
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateWazzupEnabledRequiresTokenAndBaseURL(t *testing.T) {
	cfg := &Config{}
	cfg.Server.Port = 4000
	cfg.Database.DSN = "postgres://u:p@localhost:5432/db?sslmode=disable"
	cfg.SignConfirmPolicy = "ANY"
	cfg.Wazzup.Enable = true
	cfg.Wazzup.APIBaseURL = "https://api.wazzup24.com"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when api token is missing")
	}

	cfg.Wazzup.APIToken = "token"
	cfg.Wazzup.APIBaseURL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when api base url is missing")
	}
}
