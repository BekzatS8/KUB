package config

import "testing"

func TestValidateRejectsLocalhostSigningURLsInRelease(t *testing.T) {
	t.Setenv("GIN_MODE", "release")
	cfg := &Config{}
	cfg.Server.Port = 4000
	cfg.Database.DSN = "postgres://u:p@db.example.com:5432/db?sslmode=require"
	cfg.SignConfirmPolicy = "ANY"
	cfg.Security.JWTSecret = "secret"
	cfg.Email.SMTPHost = "smtp.example.com"
	cfg.Email.SMTPUser = "user"
	cfg.Email.SMTPPassword = "pass"
	cfg.Email.FromEmail = "noreply@example.com"
	cfg.SignEmailTokenPepper = "pepper"
	cfg.Frontend.Host = "https://kubcrm.kz"
	cfg.SignBaseURL = "https://kubcrm.kz/sign"
	cfg.PublicBaseURL = "http://localhost:3000"
	cfg.SignEmailVerifyBaseURL = "https://kubcrm.kz"

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for localhost public_base_url")
	}
}

func TestApplyEnvOverridesSupportsFrontendAppURL(t *testing.T) {
	cfg := &Config{}
	t.Setenv("FRONTEND_APP_URL", "https://app.example.com")
	applyEnvOverrides(cfg)
	if cfg.Frontend.Host != "https://app.example.com" {
		t.Fatalf("unexpected frontend host: got=%q", cfg.Frontend.Host)
	}
}
