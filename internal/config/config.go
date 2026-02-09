package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type FilesConfig struct {
	RootDir string `yaml:"root_dir"`
}

type TemplatesConfig struct {
	DocxDir string `yaml:"docx_dir"`
	XlsxDir string `yaml:"xlsx_dir"`
	TxtDir  string `yaml:"txt_dir"`
}

type LibreOfficeConfig struct {
	Enable bool   `yaml:"enable"`
	Binary string `yaml:"binary"`
}

type TelegramConfig struct {
	Enable     bool   `yaml:"enable"`
	BotToken   string `yaml:"bot_token"`
	WebhookURL string `yaml:"webhook_url"`
}

type SecurityConfig struct {
	JWTSecret string `yaml:"jwt_secret"`
}

type CORSConfig struct {
	AllowOrigins  []string `yaml:"allow_origins"`
	AllowMethods  string   `yaml:"allow_methods"`
	AllowHeaders  string   `yaml:"allow_headers"`
	ExposeHeaders string   `yaml:"expose_headers"`
}

type FrontendConfig struct {
	Host string `yaml:"host"`
}
type Config struct {
	Server struct {
		Port int    `yaml:"port"`
		TZ   string `yaml:"tz"`
	} `yaml:"server"`

	Database struct {
		DSN string `yaml:"dsn"`
		URL string `yaml:"url"`
	} `yaml:"database"`

	DB struct {
		DSN string `yaml:"dsn"`
	} `yaml:"db"`

	Email struct {
		SMTPHost     string `yaml:"smtp_host"`
		SMTPPort     int    `yaml:"smtp_port"`
		SMTPUser     string `yaml:"smtp_user"`
		SMTPPassword string `yaml:"smtp_password"`
		FromEmail    string `yaml:"from_email"`
		FromName     string `yaml:"from_name"`
	} `yaml:"email"`

	Files       FilesConfig       `yaml:"files"`
	Templates   TemplatesConfig   `yaml:"templates"`
	LibreOffice LibreOfficeConfig `yaml:"libreoffice"`

	Telegram TelegramConfig `yaml:"telegram"`
	Frontend FrontendConfig `yaml:"frontend"`
	CORS     CORSConfig     `yaml:"cors"`
	Security SecurityConfig `yaml:"security"`

	SignBaseURL            string `yaml:"sign_base_url"`
	SignConfirmPolicy      string `yaml:"sign_confirm_policy"`
	SignEmailVerifyBaseURL string `yaml:"sign_email_verify_base_url"`
	SignEmailTTLMinutes    int    `yaml:"sign_email_ttl_minutes"`
	SignEmailTokenPepper   string `yaml:"sign_email_token_pepper"`
}

func LoadConfig() (*Config, error) {
	configPath, source, err := resolveConfigPath()
	if err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(configPath)
	if err == nil {
		configPath = absPath
	}
	appMode := configMode()
	log.Printf("[BOOT] config source=%s path=%s", source, configPath)
	log.Printf("[BOOT] loaded config from %s, mode=%s", configPath, appMode)

	f, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %s: %w", configPath, err)
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %s: %w", configPath, err)
	}

	applyEnvOverrides(&cfg)
	normalizeConfig(&cfg)
	applyDefaults(&cfg)
	return &cfg, nil
}

func configMode() string {
	if mode := os.Getenv("GIN_MODE"); mode == "release" {
		return "release"
	}
	return "dev"
}

func resolveConfigPath() (string, string, error) {
	if configPath := os.Getenv("CONFIG_PATH"); configPath != "" {
		return configPath, "env", nil
	}

	paths := []string{"config/config.yaml", "config.yaml"}
	for _, candidate := range paths {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, "file", nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", "", fmt.Errorf("failed to stat config file: %s: %w", candidate, err)
		}
	}

	return "", "", fmt.Errorf("config file not found in %v", paths)
}

func normalizeConfig(cfg *Config) {
	if cfg.DB.DSN != "" {
		cfg.Database.DSN = cfg.DB.DSN
	}
	if cfg.Database.DSN == "" && cfg.Database.URL != "" {
		cfg.Database.DSN = cfg.Database.URL
	}
}

func (cfg *Config) Validate() error {
	if cfg.Server.Port <= 0 {
		return fmt.Errorf("invalid server.port: %d", cfg.Server.Port)
	}
	if cfg.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}

	mode := configMode()
	normalizedDSN, err := normalizeDSN(cfg.Database.DSN, mode)
	if err != nil {
		return err
	}
	cfg.Database.DSN = normalizedDSN

	if len(cfg.CORS.AllowOrigins) == 0 {
		if mode == "release" {
			return fmt.Errorf("cors.allow_origins is required in release mode")
		}
		cfg.CORS.AllowOrigins = []string{
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		}
	}
	if mode == "release" {
		for _, origin := range cfg.CORS.AllowOrigins {
			if origin == "*" {
				return fmt.Errorf("cors.allow_origins cannot include '*' in release mode")
			}
		}
	}
	switch cfg.SignConfirmPolicy {
	case "ANY", "BOTH":
	default:
		return fmt.Errorf("invalid sign_confirm_policy: %s", cfg.SignConfirmPolicy)
	}
	if mode == "release" {
		missing := []string{}
		if strings.TrimSpace(cfg.Email.SMTPHost) == "" {
			missing = append(missing, "email.smtp_host")
		}
		if strings.TrimSpace(cfg.Email.SMTPUser) == "" {
			missing = append(missing, "email.smtp_user")
		}
		if strings.TrimSpace(cfg.Email.SMTPPassword) == "" {
			missing = append(missing, "email.smtp_password")
		}
		if strings.TrimSpace(cfg.Email.FromEmail) == "" {
			missing = append(missing, "email.from_email")
		}
		if strings.TrimSpace(cfg.SignEmailTokenPepper) == "" {
			missing = append(missing, "sign_email_token_pepper")
		}
		if len(missing) > 0 {
			return fmt.Errorf("email settings required in release mode: %s", strings.Join(missing, ", "))
		}
	}

	return nil
}

func normalizeDSN(dsn string, mode string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("invalid database.dsn: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid database.dsn: missing scheme or host")
	}

	values := parsed.Query()
	sslMode := values.Get("sslmode")
	if sslMode == "" {
		if mode == "release" {
			return "", fmt.Errorf("database.dsn must include sslmode in release mode")
		}
		values.Set("sslmode", "disable")
		parsed.RawQuery = values.Encode()
		return parsed.String(), nil
	}

	validSSLModes := map[string]struct{}{
		"disable":     {},
		"allow":       {},
		"prefer":      {},
		"require":     {},
		"verify-ca":   {},
		"verify-full": {},
	}
	if _, ok := validSSLModes[strings.ToLower(sslMode)]; !ok {
		return "", fmt.Errorf("invalid sslmode in database.dsn: %s", sslMode)
	}

	return parsed.String(), nil
}

func applyDefaults(cfg *Config) {
	// defaults
	if cfg.Files.RootDir == "" {
		cfg.Files.RootDir = "./files"
	}
	if cfg.Templates.DocxDir == "" {
		cfg.Templates.DocxDir = "assets/templates/docx"
	}
	if cfg.Templates.XlsxDir == "" {
		cfg.Templates.XlsxDir = "assets/templates/xlsx"
	}
	if cfg.Templates.TxtDir == "" {
		cfg.Templates.TxtDir = "assets/templates/txt"
	}
	if cfg.LibreOffice.Binary == "" {
		cfg.LibreOffice.Binary = "libreoffice"
	}
	if cfg.Frontend.Host == "" {
		cfg.Frontend.Host = "http://localhost:3000"
	}
	if cfg.CORS.AllowMethods == "" {
		cfg.CORS.AllowMethods = "GET, POST, PUT, DELETE, OPTIONS"
	}
	if cfg.CORS.AllowHeaders == "" {
		cfg.CORS.AllowHeaders = "Origin, Content-Type, Authorization"
	}
	if cfg.CORS.ExposeHeaders == "" {
		cfg.CORS.ExposeHeaders = "Content-Disposition, Content-Type, Content-Length"
	}
	if envSecret := os.Getenv("JWT_SECRET"); envSecret != "" {
		cfg.Security.JWTSecret = envSecret
	}
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		if cfg.Database.DSN == "" {
			cfg.Database.DSN = dbURL
		}
	}

	if strings.TrimSpace(cfg.SignConfirmPolicy) == "" {
		cfg.SignConfirmPolicy = "ANY"
	}
	cfg.SignConfirmPolicy = strings.ToUpper(strings.TrimSpace(cfg.SignConfirmPolicy))
	if strings.TrimSpace(cfg.SignEmailTokenPepper) == "" && configMode() != "release" {
		if strings.TrimSpace(cfg.Security.JWTSecret) != "" {
			cfg.SignEmailTokenPepper = cfg.Security.JWTSecret
		} else {
			cfg.SignEmailTokenPepper = "dev-insecure-sign-email-pepper"
		}
	}
	if strings.TrimSpace(cfg.SignBaseURL) == "" && cfg.Frontend.Host != "" {
		cfg.SignBaseURL = strings.TrimRight(cfg.Frontend.Host, "/") + "/sign"
	}
	if strings.TrimSpace(cfg.SignEmailVerifyBaseURL) == "" && cfg.Frontend.Host != "" {
		cfg.SignEmailVerifyBaseURL = strings.TrimRight(cfg.Frontend.Host, "/")
	}
	if cfg.SignEmailTTLMinutes <= 0 {
		cfg.SignEmailTTLMinutes = 30
	}
}

func applyEnvOverrides(cfg *Config) {
	setString := func(value string, target *string) {
		if strings.TrimSpace(value) != "" {
			*target = strings.TrimSpace(value)
		}
	}
	setInt := func(value string, target *int) {
		if strings.TrimSpace(value) == "" {
			return
		}
		if intVal, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			*target = intVal
		}
	}
	setString(os.Getenv("SIGN_BASE_URL"), &cfg.SignBaseURL)
	setString(os.Getenv("SIGN_CONFIRM_POLICY"), &cfg.SignConfirmPolicy)
	setString(os.Getenv("EMAIL_FROM"), &cfg.Email.FromEmail)
	setString(os.Getenv("SMTP_FROM"), &cfg.Email.FromEmail)
	setString(os.Getenv("SMTP_FROM_NAME"), &cfg.Email.FromName)
	setString(os.Getenv("SMTP_HOST"), &cfg.Email.SMTPHost)
	setString(os.Getenv("SMTP_USER"), &cfg.Email.SMTPUser)
	setString(os.Getenv("SMTP_PASS"), &cfg.Email.SMTPPassword)
	setInt(os.Getenv("SMTP_PORT"), &cfg.Email.SMTPPort)
	setString(os.Getenv("SIGN_EMAIL_TOKEN_PEPPER"), &cfg.SignEmailTokenPepper)
	setString(os.Getenv("SIGN_EMAIL_VERIFY_BASE_URL"), &cfg.SignEmailVerifyBaseURL)
	if ttl := strings.TrimSpace(os.Getenv("SIGN_EMAIL_TTL")); ttl != "" {
		if duration, err := time.ParseDuration(ttl); err == nil {
			minutes := int(duration.Minutes())
			if minutes > 0 {
				cfg.SignEmailTTLMinutes = minutes
			}
		} else if minutes, err := strconv.Atoi(ttl); err == nil && minutes > 0 {
			cfg.SignEmailTTLMinutes = minutes
		}
	}
}

func parseBoolEnvValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
