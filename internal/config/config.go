package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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

type MobizonConfig struct {
	APIKey   string `yaml:"api_key"`
	SenderID string `yaml:"sender_id"`
	DryRun   bool   `yaml:"dry_run"`
}

type WhatsAppConfig struct {
	Enable        bool   `yaml:"enable"`
	AccessToken   string `yaml:"access_token"`
	PhoneNumberID string `yaml:"phone_number_id"`
	APIVersion    string `yaml:"api_version"`
	TemplateName  string `yaml:"template_name"`
	LangCode      string `yaml:"lang_code"`
	DryRun        bool   `yaml:"dry_run"`
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
	} `yaml:"email"`

	Files       FilesConfig       `yaml:"files"`
	Templates   TemplatesConfig   `yaml:"templates"`
	LibreOffice LibreOfficeConfig `yaml:"libreoffice"`

	Mobizon  MobizonConfig  `yaml:"mobizon"`
	WhatsApp WhatsAppConfig `yaml:"whatsapp"`

	Telegram TelegramConfig `yaml:"telegram"`
	Frontend FrontendConfig `yaml:"frontend"`
	CORS     CORSConfig     `yaml:"cors"`
	Security SecurityConfig `yaml:"security"`
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

	// WhatsApp defaults
	if cfg.WhatsApp.APIVersion == "" {
		cfg.WhatsApp.APIVersion = "v21.0"
	}
	if cfg.WhatsApp.LangCode == "" {
		cfg.WhatsApp.LangCode = "ru"
	}
}
