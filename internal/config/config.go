// File: internal/config/config.go
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// BotConfig holds Telegram bot settings.
type BotConfig struct {
	Token    string  `mapstructure:"token"`
	Mode     string  `mapstructure:"mode"` // webhook or polling
	Port     int     `mapstructure:"port"` // webhook listen port
	URL      string  `mapstructure:"url"`  // webhook URL base
	AdminIDs []int64 `mapstructure:"admin_ids"`
}

// AdminConfig holds HTTP admin server settings.
type AdminConfig struct {
	Port int `mapstructure:"port"`
}

// DatabaseConfig holds PostgreSQL connection string.
type DatabaseConfig struct {
	URL string `mapstructure:"url"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	URL      string        `mapstructure:"url"`
	Password string        `mapstructure:"password"`
	DB       int           `mapstructure:"db"`
	TTL      time.Duration `mapstructure:"ttl"` // for caching, notifications
}

// AIConfig holds API keys/URLs for OpenAI and Gemini.
type AIConfig struct {
	OpenAIKey    string `mapstructure:"openai_key"`
	GeminiKey    string `mapstructure:"gemini_key"`
	GeminiURL    string `mapstructure:"gemini_url"`
	DefaultModel string `mapstructure:"default_model"`
}

type ZarinPalConfig struct {
	MerchantID   string `mapstructure:"merchant_id"`
	CallbackURL  string `mapstructure:"callback_url"`
	CallbackPort int    `mapstructure:"callback_port"`
	Sandbox      bool   `mapstructure:"sandbox"`
}

type PaymentConfig struct {
	ZarinPal ZarinPalConfig `mapstructure:"zarinpal"`
}

// SchedulerConfig holds cron schedules.
type SchedulerConfig struct {
	ExpiryCheckCron string `mapstructure:"expiry_check_cron"`
}

// Config is the complete application configuration.
type Config struct {
	Bot       BotConfig       `mapstructure:"bot"`
	Admin     AdminConfig     `mapstructure:"admin"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	AI        AIConfig        `mapstructure:"ai"`
	Payment   PaymentConfig   `mapstructure:"payment"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
}

// LoadConfig reads config.yaml (if exists), environment variables, and flags.
// This is the application-level loader and performs stricter validation:
// - bot.token is required
// - database.url is required
// - at least one AI key is required (openai_key or gemini_key)
func LoadConfig() (*Config, error) {
	// Command-line flag to override config file path
	cfgFile := flag.String("config", "./config.yaml", "path to config file")
	flag.Parse()

	v := viper.New()
	v.SetConfigFile(*cfgFile)
	v.SetConfigType("yaml")

	// Set defaults
	v.SetDefault("bot.mode", "webhook")
	v.SetDefault("bot.port", 8443)
	v.SetDefault("admin.port", 8080)
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.ttl", "24h")
	v.SetDefault("ai.default_model", "gpt-3.5-turbo")
	v.SetDefault("scheduler.expiry_check_cron", "@daily")

	// Environment variable support
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		// If the file is missing, continue with env/flags
		if _, notFound := err.(viper.ConfigFileNotFoundError); !notFound {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal into struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}

	// Parse TTL (string → time.Duration)
	ttlStr := v.GetString("redis.ttl")
	ttlDur, err := time.ParseDuration(ttlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid redis.ttl: %w", err)
	}
	cfg.Redis.TTL = ttlDur

	// Validate required fields (application strict mode)
	if cfg.Bot.Token == "" {
		return nil, errors.New("bot.token is required")
	}
	if cfg.Database.URL == "" {
		return nil, errors.New("database.url is required")
	}
	if cfg.AI.OpenAIKey == "" && cfg.AI.GeminiKey == "" {
		return nil, errors.New("at least one AI key (openai_key or gemini_key) is required")
	}
	// Inside LoadConfig(), after existing validations:
	if len(cfg.Bot.AdminIDs) == 0 {
		return nil, errors.New("at least one admin ID must be specified in bot.admin_ids")
	}

	return &cfg, nil
}

// LoadConfigFrom loads configuration from the provided YAML path (e.g. "config.test.yml").
// Behavior:
//   - If the file exists, it is parsed (viper) and values are used.
//   - If the file does not exist, env vars are used (prefer TEST_DATABASE_URL then DATABASE_URL).
//   - It parses redis.ttl into time.Duration.
//   - It is lenient: it **only requires database.url** and does not enforce bot.token or AI keys.
//
// This function is intended for tests/integration where only DB connectivity is needed.
func LoadConfigFrom(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Defaults mirrored from LoadConfig
	v.SetDefault("bot.mode", "webhook")
	v.SetDefault("bot.port", 8443)
	v.SetDefault("admin.port", 8080)
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.ttl", "24h")
	v.SetDefault("ai.default_model", "gpt-3.5-turbo")
	v.SetDefault("scheduler.expiry_check_cron", "@daily")

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Try read the provided config file. If missing, we continue and rely on env vars.
	if err := v.ReadInConfig(); err != nil {
		if _, notFound := err.(viper.ConfigFileNotFoundError); notFound {
			// file not found → continue to env fallback
		} else {
			return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode config from %s: %w", path, err)
	}

	// Parse TTL (string -> time.Duration). If parse fails, use default 24h.
	ttlStr := v.GetString("redis.ttl")
	if ttlStr == "" {
		ttlStr = "24h"
	}
	ttlDur, err := time.ParseDuration(ttlStr)
	if err != nil {
		ttlDur = 24 * time.Hour
	}
	cfg.Redis.TTL = ttlDur

	// Environment override priority for DB URL:
	// 1. TEST_DATABASE_URL env var
	// 2. DATABASE_URL env var
	// 3. config file value (already in cfg if present)
	if env := os.Getenv("TEST_DATABASE_URL"); env != "" {
		cfg.Database.URL = env
	} else if env := os.Getenv("DATABASE_URL"); env != "" && cfg.Database.URL == "" {
		cfg.Database.URL = env
	}

	// For tests we require database.url to be present; other fields are optional.
	if cfg.Database.URL == "" {
		return nil, errors.New("database.url is required (set TEST_DATABASE_URL, DATABASE_URL, or provide it in the YAML)")
	}

	return &cfg, nil
}
