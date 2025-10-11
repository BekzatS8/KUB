package config

import (
	"gopkg.in/yaml.v3"
	"os"
)

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
}

func LoadConfig() *Config {
	f, err := os.Open("config/config.yaml")
	if err != nil {
		panic("Failed to open config.yaml: " + err.Error())
	}
	defer f.Close()

	var cfg Config
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		panic("Failed to parse config.yaml: " + err.Error())
	}

	return &cfg
}
