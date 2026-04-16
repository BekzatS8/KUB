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

type WazzupConfig struct {
	Enable             bool   `yaml:"enable"`
	APIBaseURL         string `yaml:"api_base_url"`
	APIToken           string `yaml:"api_token"`
	ChannelID          string `yaml:"channel_id"`
	WebhookVerifyToken string `yaml:"webhook_verify_token"`
	WebhookBaseURL     string `yaml:"webhook_base_url"`
	RequestTimeoutSec  int    `yaml:"request_timeout_sec"`
	RetryCount         int    `yaml:"retry_count"`
	RetryDelayMS       int    `yaml:"retry_delay_ms"`
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

type DocumentsConfig struct {
	StrictPlaceholders bool `yaml:"strict_placeholders"`
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

	Telegram  TelegramConfig  `yaml:"telegram"`
	Wazzup    WazzupConfig    `yaml:"wazzup"`
	Frontend  FrontendConfig  `yaml:"frontend"`
	Documents DocumentsConfig `yaml:"documents"`
	CORS      CORSConfig      `yaml:"cors"`
	Security  SecurityConfig  `yaml:"security"`

	SignBaseURL            string `yaml:"sign_base_url"`
	PublicBaseURL          string `yaml:"public_base_url"`
	SignConfirmPolicy      string `yaml:"sign_confirm_policy"`
	SignEmailVerifyBaseURL string `yaml:"sign_email_verify_base_url"`
	SignSMSVerifyBaseURL   string `yaml:"sign_sms_verify_base_url"`
	SignEmailTTLMinutes    int    `yaml:"sign_email_ttl_minutes"`
	SignSMSTTLMinutes      int    `yaml:"sign_sms_ttl_minutes"`
	SignSessionTTLMinutes  int    `yaml:"sign_session_ttl_minutes"`
	SignEmailTokenPepper   string `yaml:"sign_email_token_pepper"`
	SignPublicTokenPepper  string `yaml:"sign_public_token_pepper"`
	Mobizon                struct {
		Enabled        bool   `yaml:"enabled"`
		APIKey         string `yaml:"api_key"`
		BaseURL        string `yaml:"base_url"`
		From           string `yaml:"from"`
		TimeoutSeconds int    `yaml:"timeout_seconds"`
		Retries        int    `yaml:"retries"`
		DryRun         bool   `yaml:"dry_run"`
	} `yaml:"mobizon"`
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
	log.Printf("[BOOT] config source=%s path=%s", source, maskConfigPath(configPath))
	log.Printf("[BOOT] loaded config from %s, mode=%s", maskConfigPath(configPath), appMode)

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
		if err := validatePublicURL("frontend.host", cfg.Frontend.Host); err != nil {
			return err
		}
		if err := validatePublicURL("public_base_url", cfg.PublicBaseURL); err != nil {
			return err
		}
		if err := validatePublicURL("sign_base_url", cfg.SignBaseURL); err != nil {
			return err
		}
		if err := validatePublicURL("sign_email_verify_base_url", cfg.SignEmailVerifyBaseURL); err != nil {
			return err
		}
		if err := validatePublicURL("sign_sms_verify_base_url", cfg.SignSMSVerifyBaseURL); err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Security.JWTSecret) == "" {
			return fmt.Errorf("security.jwt_secret is required in release mode")
		}
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
	if cfg.Wazzup.Enable {
		if strings.TrimSpace(cfg.Wazzup.APIToken) == "" {
			return fmt.Errorf("wazzup.api_token is required when wazzup.enable=true")
		}
		if strings.TrimSpace(cfg.Wazzup.APIBaseURL) == "" {
			return fmt.Errorf("wazzup.api_base_url is required when wazzup.enable=true")
		}
	}

	return nil
}

func maskConfigPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "<empty>"
	}
	return filepath.Base(path)
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
	if cfg.Frontend.Host == "" && configMode() != "release" {
		cfg.Frontend.Host = "http://localhost:3000"
	}
	if strings.TrimSpace(cfg.Wazzup.APIBaseURL) == "" {
		cfg.Wazzup.APIBaseURL = "https://api.wazzup24.com"
	}
	if cfg.Wazzup.RequestTimeoutSec <= 0 {
		cfg.Wazzup.RequestTimeoutSec = 10
	}
	if cfg.Wazzup.RetryCount < 0 {
		cfg.Wazzup.RetryCount = 0
	}
	if cfg.Wazzup.RetryDelayMS <= 0 {
		cfg.Wazzup.RetryDelayMS = 300
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
		cfg.Database.DSN = dbURL
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
	if strings.TrimSpace(cfg.PublicBaseURL) == "" && cfg.Frontend.Host != "" {
		cfg.PublicBaseURL = strings.TrimRight(cfg.Frontend.Host, "/")
	}
	if strings.TrimSpace(cfg.SignEmailVerifyBaseURL) == "" && cfg.Frontend.Host != "" {
		cfg.SignEmailVerifyBaseURL = strings.TrimRight(cfg.Frontend.Host, "/")
	}
	if strings.TrimSpace(cfg.SignSMSVerifyBaseURL) == "" && cfg.Frontend.Host != "" {
		cfg.SignSMSVerifyBaseURL = strings.TrimRight(cfg.Frontend.Host, "/")
	}
	if strings.TrimSpace(cfg.SignPublicTokenPepper) == "" {
		cfg.SignPublicTokenPepper = cfg.SignEmailTokenPepper
	}
	if cfg.SignEmailTTLMinutes <= 0 {
		cfg.SignEmailTTLMinutes = 30
	}
	if cfg.SignSMSTTLMinutes <= 0 {
		cfg.SignSMSTTLMinutes = cfg.SignEmailTTLMinutes
	}
	if cfg.SignSessionTTLMinutes <= 0 {
		cfg.SignSessionTTLMinutes = cfg.SignEmailTTLMinutes
	}
	if strings.TrimSpace(cfg.Mobizon.BaseURL) == "" {
		cfg.Mobizon.BaseURL = "https://api.mobizon.kz"
	}
	if cfg.Mobizon.TimeoutSeconds <= 0 {
		cfg.Mobizon.TimeoutSeconds = 10
	}
	if cfg.Mobizon.Retries < 0 {
		cfg.Mobizon.Retries = 0
	}
	if !cfg.Documents.StrictPlaceholders && configMode() != "release" {
		cfg.Documents.StrictPlaceholders = true
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
	setString(os.Getenv("PUBLIC_BASE_URL"), &cfg.PublicBaseURL)
	setString(os.Getenv("SIGN_PUBLIC_BASE_URL"), &cfg.PublicBaseURL)
	setString(os.Getenv("FRONTEND_HOST"), &cfg.Frontend.Host)
	setString(os.Getenv("FRONTEND_APP_URL"), &cfg.Frontend.Host)
	setString(os.Getenv("PUBLIC_APP_URL"), &cfg.Frontend.Host)
	setString(os.Getenv("NEXT_PUBLIC_APP_URL"), &cfg.Frontend.Host)
	setString(os.Getenv("VITE_APP_URL"), &cfg.Frontend.Host)
	setString(os.Getenv("SIGN_CONFIRM_POLICY"), &cfg.SignConfirmPolicy)
	setString(os.Getenv("EMAIL_FROM"), &cfg.Email.FromEmail)
	setString(os.Getenv("SMTP_FROM"), &cfg.Email.FromEmail)
	setString(os.Getenv("SMTP_FROM_NAME"), &cfg.Email.FromName)
	setString(os.Getenv("SMTP_HOST"), &cfg.Email.SMTPHost)
	setString(os.Getenv("SMTP_USER"), &cfg.Email.SMTPUser)
	setString(os.Getenv("SMTP_PASSWORD"), &cfg.Email.SMTPPassword)
	setString(os.Getenv("SMTP_PASS"), &cfg.Email.SMTPPassword)
	setInt(os.Getenv("SMTP_PORT"), &cfg.Email.SMTPPort)
	setString(os.Getenv("SIGN_EMAIL_TOKEN_PEPPER"), &cfg.SignEmailTokenPepper)
	setString(os.Getenv("SIGN_PUBLIC_TOKEN_PEPPER"), &cfg.SignPublicTokenPepper)
	setString(os.Getenv("SIGN_EMAIL_VERIFY_BASE_URL"), &cfg.SignEmailVerifyBaseURL)
	setString(os.Getenv("SIGN_SMS_VERIFY_BASE_URL"), &cfg.SignSMSVerifyBaseURL)
	setString(os.Getenv("MOBIZON_API_KEY"), &cfg.Mobizon.APIKey)
	setString(os.Getenv("MOBIZON_BASE_URL"), &cfg.Mobizon.BaseURL)
	setString(os.Getenv("MOBIZON_FROM"), &cfg.Mobizon.From)
	setInt(os.Getenv("MOBIZON_TIMEOUT_SECONDS"), &cfg.Mobizon.TimeoutSeconds)
	setInt(os.Getenv("MOBIZON_RETRIES"), &cfg.Mobizon.Retries)
	setString(os.Getenv("TELEGRAM_APITOKEN"), &cfg.Telegram.BotToken)
	setString(os.Getenv("TELEGRAM_WEBHOOK_URL"), &cfg.Telegram.WebhookURL)
	setString(os.Getenv("WAZZUP_API_BASE_URL"), &cfg.Wazzup.APIBaseURL)
	setString(os.Getenv("WAZZUP_API_TOKEN"), &cfg.Wazzup.APIToken)
	setString(os.Getenv("WAZZUP_CHANNEL_ID"), &cfg.Wazzup.ChannelID)
	setString(os.Getenv("WAZZUP_WEBHOOK_VERIFY_TOKEN"), &cfg.Wazzup.WebhookVerifyToken)
	setString(os.Getenv("WAZZUP_WEBHOOK_BASE_URL"), &cfg.Wazzup.WebhookBaseURL)
	setInt(os.Getenv("WAZZUP_REQUEST_TIMEOUT_SEC"), &cfg.Wazzup.RequestTimeoutSec)
	setInt(os.Getenv("WAZZUP_RETRY_COUNT"), &cfg.Wazzup.RetryCount)
	setInt(os.Getenv("WAZZUP_RETRY_DELAY_MS"), &cfg.Wazzup.RetryDelayMS)
	if val := strings.TrimSpace(os.Getenv("WAZZUP_ENABLE")); val != "" {
		cfg.Wazzup.Enable = parseBoolEnvValue(val)
	}
	if val := strings.TrimSpace(os.Getenv("DOCUMENTS_STRICT_PLACEHOLDERS")); val != "" {
		cfg.Documents.StrictPlaceholders = parseBoolEnvValue(val)
	}
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
	if ttl := strings.TrimSpace(os.Getenv("SIGN_SMS_TTL")); ttl != "" {
		if duration, err := time.ParseDuration(ttl); err == nil {
			minutes := int(duration.Minutes())
			if minutes > 0 {
				cfg.SignSMSTTLMinutes = minutes
			}
		} else if minutes, err := strconv.Atoi(ttl); err == nil && minutes > 0 {
			cfg.SignSMSTTLMinutes = minutes
		}
	}
	if val := strings.TrimSpace(os.Getenv("MOBIZON_ENABLED")); val != "" {
		cfg.Mobizon.Enabled = parseBoolEnvValue(val)
	}
	if val := strings.TrimSpace(os.Getenv("MOBIZON_DRY_RUN")); val != "" {
		cfg.Mobizon.DryRun = parseBoolEnvValue(val)
	}
	setInt(os.Getenv("SIGN_SESSION_TTL_MINUTES"), &cfg.SignSessionTTLMinutes)
}

func validatePublicURL(fieldName, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("%s is required in release mode", fieldName)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute URL", fieldName)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return fmt.Errorf("%s cannot point to localhost in release mode", fieldName)
	}
	return nil
}

func parseBoolEnvValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
