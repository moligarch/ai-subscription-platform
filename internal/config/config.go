// File: internal/config/config.go
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type RuntimeConfig struct {
	Dev bool
}

type BotConfig struct {
	Token    string  `yaml:"token"`
	Mode     string  `yaml:"mode"` // polling | webhook (future)
	Port     int     `yaml:"port"`
	Username string  `yaml:"username"`
	Workers  int     `yaml:"workers"` // polling workers
	AdminIDs []int64 `yaml:"admin_ids"`
}

type LogConfig struct {
	Level    string `yaml:"level"`    // trace|debug|info|warn|error
	Format   string `yaml:"format"`   // json|console
	Sampling bool   `yaml:"sampling"` // enable sampling in prod
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
	TTL      time.Duration `yaml:"ttl"`
}

type AIConfig struct {
	OpenAIKey       string `yaml:"openai_key"`
	GeminiKey       string `yaml:"gemini_key"`
	GeminiURL       string `yaml:"gemini_url"`
	DefaultModel    string `yaml:"default_model"`
	MetisKey        string `yaml:"metis_key"`
	MetisBaseURL    string `yaml:"metis_base_url"`
	ConcurrentLimit int    `yaml:"concurrent_limit"` // max concurrent AI calls
}

type PaymentConfig struct {
	ZarinPal struct {
		MerchantID   string `yaml:"merchant_id"`
		CallbackURL  string `yaml:"callback_url"`
		CallbackPort int    `yaml:"callback_port"`
		Sandbox      bool   `yaml:"sandbox"`
		AccessToken  string `yaml:"access_token"`
	} `yaml:"zarinpal"`
}

type SchedulerConfig struct {
	ExpiryCheckCron string `yaml:"expiry_check_cron"`
}

type SecurityConfig struct {
	EncryptionKey string `yaml:"encryption_key"`
}

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

	Runtime RuntimeConfig `yaml:"-"`
}

func LoadConfig() (*Config, error) {
	var configPath string = ""
	var dev bool
	flag.StringVar(&configPath, "config", "config.yaml", "path to config yaml")
	flag.BoolVar(&dev, "dev", false, "development mode")
	flag.Parse()

	b, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// defaults
	if cfg.Bot.Workers <= 0 {
		cfg.Bot.Workers = 8
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}
	if cfg.AI.ConcurrentLimit <= 0 {
		cfg.AI.ConcurrentLimit = 16
	}
	cfg.Redis.TTL = normalizeTTL(cfg.Redis.TTL)

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

	cfg.Runtime.Dev = dev
	return &cfg, nil
}

func normalizeTTL(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Hour
	}
	return d
}
