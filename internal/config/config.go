package config

import (
	"gopkg.in/yaml.v3"
	"os"
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
	WebhookURL string `yaml:"webhook_url"` // публичный URL вида https://domain/tg/webhook
}
type MobizonConfig struct {
	APIKey   string `yaml:"api_key"`
	SenderID string `yaml:"sender_id"`
	DryRun   bool   `yaml:"dry_run"`
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
		DSN string `yaml:"url"`
	} `yaml:"database"`
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
	Mobizon     MobizonConfig     `yaml:"mobizon"`
	Telegram    TelegramConfig    `yaml:"telegram"`
	Frontend    FrontendConfig    `yaml:"frontend"`
}

func LoadConfig() *Config {
	f, err := os.Open("config/config.yaml")
	if err != nil {
		panic("Failed to open config.yaml: " + err.Error())
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		panic("Failed to parse config.yaml: " + err.Error())
	}

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
	return &cfg
}
