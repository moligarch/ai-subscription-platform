// File: internal/config/config.go
package config

import (
    "errors"
    "flag"
    "fmt"
    "strings"
    "time"

    "github.com/spf13/viper"
)

// BotConfig holds Telegram bot settings.
type BotConfig struct {
    Token string `mapstructure:"token"`
    Mode  string `mapstructure:"mode"`  // webhook or polling
    Port  int    `mapstructure:"port"`  // webhook listen port
    URL   string `mapstructure:"url"`   // webhook URL base
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
    OpenAIKey   string `mapstructure:"openai_key"`
    GeminiKey   string `mapstructure:"gemini_key"`
    GeminiURL   string `mapstructure:"gemini_url"`
    DefaultModel string `mapstructure:"default_model"`
}

// PaymentConfig holds payment gateway credentials.
type PaymentConfig struct {
    MellatTerminal string `mapstructure:"mellat_terminal"`
    ZarinpalKey    string `mapstructure:"zarinpal_key"`
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
func LoadConfig() (*Config, error) {
    // Command-line flag to override config file path
    cfgFile := flag.String("config", "config.yaml", "path to config file")
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

    // Validate required fields
    if cfg.Bot.Token == "" {
        return nil, errors.New("bot.token is required")
    }
    if cfg.Database.URL == "" {
        return nil, errors.New("database.url is required")
    }
    if cfg.AI.OpenAIKey == "" && cfg.AI.GeminiKey == "" {
        return nil, errors.New("at least one AI key (openai_key or gemini_key) is required")
    }

    return &cfg, nil
}
