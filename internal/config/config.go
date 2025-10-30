package config

import (
	"gopkg.in/yaml.v3"
	"os"
)

type FilesConfig struct {
	RootDir string `yaml:"root_dir"`
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

type Config struct {
	Server struct {
		Port int `yaml:"port"`
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
	Files    FilesConfig    `yaml:"files"`
	Mobizon  MobizonConfig  `yaml:"mobizon"`
	Telegram TelegramConfig `yaml:"telegram"`
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
	return &cfg
}
