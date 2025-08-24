// File: internal/config/config.go
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Bot       BotConfig       `yaml:"bot"`
	Log       LogConfig       `yaml:"log"`
	Admin     AdminConfig     `yaml:"admin"`
	Database  DatabaseConfig  `yaml:"database"`
	Redis     RedisConfig     `yaml:"redis"`
	AI        AIConfig        `yaml:"ai"`
	Payment   PaymentConfig   `yaml:"payment"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	Security  SecurityConfig  `yaml:"security"`

	Runtime RuntimeConfig `yaml:"-"` // populated from CLI flags (e.g., --dev)
}

type RuntimeConfig struct {
	Dev bool
}

type BotConfig struct {
	Token    string  `yaml:"token"`
	Mode     string  `yaml:"mode"` // polling | webhook
	Port     int     `yaml:"port"`
	URL      string  `yaml:"url"`
	AdminIDs []int64 `yaml:"admin_ids"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

type AdminConfig struct {
	Port int `yaml:"port"`
}

type DatabaseConfig struct {
	URL string `yaml:"url"`
}

type RedisConfig struct {
	URL      string        `yaml:"url"`
	Password string        `yaml:"password"`
	DB       int           `yaml:"db"`
	TTL      time.Duration `yaml:"ttl"` // "1h"
}

type AIConfig struct {
	OpenAIKey    string `yaml:"openai_key"`
	GeminiKey    string `yaml:"gemini_key"`
	GeminiURL    string `yaml:"gemini_url"`
	DefaultModel string `yaml:"default_model"`

	MetisKey     string `yaml:"metis_key"`
	MetisBaseURL string `yaml:"metis_base_url"`
}

type PaymentConfig struct {
	ZarinPal ZarinPalConfig `yaml:"zarinpal"`
}

type ZarinPalConfig struct {
	MerchantID   string `yaml:"merchant_id"`
	CallbackURL  string `yaml:"callback_url"`
	CallbackPort int    `yaml:"callback_port"`
	Sandbox      bool   `yaml:"sandbox"`
}

type SchedulerConfig struct {
	ExpiryCheckCron string `yaml:"expiry_check_cron"`
}

type SecurityConfig struct {
	EncryptionKey string `yaml:"encryption_key"`
}

// LoadConfig reads YAML config from path and applies runtime flags like devMode.
func LoadConfig(path string, devMode bool) (*Config, error) {
	if path == "" {
		path = "config.yaml"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	// Defaults
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Redis.TTL == 0 {
		cfg.Redis.TTL = time.Hour
	}
	if cfg.AI.DefaultModel == "" {
		cfg.AI.DefaultModel = "gpt-4o-mini"
	}
	if cfg.AI.MetisBaseURL == "" {
		cfg.AI.MetisBaseURL = "https://api.metisai.ir/openai/v1"
	}

	// Minimal validation
	if cfg.Bot.Token == "" {
		return nil, errors.New("bot.token is required")
	}
	if cfg.Database.URL == "" {
		return nil, errors.New("database.url is required")
	}
	if cfg.Redis.URL == "" {
		return nil, errors.New("redis.url is required")
	}

	cfg.Runtime.Dev = devMode
	return &cfg, nil
}
