package config

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
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
	Port   int    `yaml:"port"`
	APIKey string `yaml:"api_key"`
}

type DatabaseConfig struct {
	URL          string `yaml:"url"`
	PoolMaxConns int    `yaml:"max_conn"`
}

type RedisConfig struct {
	URL      string        `yaml:"url"`
	Password string        `yaml:"password"`
	DB       int           `yaml:"db"`
	TTL      time.Duration `yaml:"ttl"`
}

type AIConfig struct {
	// model_provider_map maps model names to a provider key: "openai" or "gemini"
	ModelProviderMap map[string]string `yaml:"model_provider_map"`
	OpenAI           struct {
		APIKey       string `yaml:"api_key"`
		BaseURL      string `yaml:"base_url"` // supports OpenRouter/Metis style, leave empty for OpenAI
		DefaultModel string `yaml:"default_model"`
	} `yaml:"openai"`

	Gemini struct {
		APIKey       string `yaml:"api_key"`
		BaseURL      string `yaml:"base_url"`
		DefaultModel string `yaml:"default_model"`
	} `yaml:"gemini"`

	ConcurrentLimit int `yaml:"concurrent_limit"` // max in-flight AI calls across all providers
	MaxOutputTokens int `yaml:"max_output_tokens"`
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

type SafeAI struct {
	ModelProviderMap map[string]string `json:"model_provider_map"`
	OpenAI           struct {
		BaseURL      string `json:"base_url"`
		DefaultModel string `json:"default_model"`
		HasAPIKey    bool   `json:"has_api_key"`
	} `json:"openai"`
	Gemini struct {
		BaseURL      string `json:"base_url"`
		DefaultModel string `json:"default_model"`
		HasAPIKey    bool   `json:"has_api_key"`
	} `json:"gemini"`
	ConcurrentLimit int `json:"concurrent_limit"`
	MaxOutputTokens int `json:"max_output_tokens"`
}

func (a *AIConfig) Safe() SafeAI {
	s := SafeAI{
		ModelProviderMap: a.ModelProviderMap,
		ConcurrentLimit:  a.ConcurrentLimit,
		MaxOutputTokens:  a.MaxOutputTokens,
	}
	s.OpenAI.BaseURL = a.OpenAI.BaseURL
	s.OpenAI.DefaultModel = a.OpenAI.DefaultModel
	s.OpenAI.HasAPIKey = a.OpenAI.APIKey != ""

	s.Gemini.BaseURL = a.Gemini.BaseURL
	s.Gemini.DefaultModel = a.Gemini.DefaultModel
	s.Gemini.HasAPIKey = a.Gemini.APIKey != ""
	return s
}

// Full safe config for the “effective config” log
type SafeConfig struct {
	Runtime  RuntimeConfig `json:"runtime"`
	Log      LogConfig     `json:"log"`
	AI       SafeAI        `json:"ai"`
	Security struct {
		KeyLen int  `json:"key_len"`
		IsDev  bool `json:"is_dev"`
	} `json:"security"`
}

func (c *Config) Redacted() SafeConfig {
	out := SafeConfig{
		Runtime: c.Runtime,
		Log:     c.Log,
		AI:      c.AI.Safe(),
	}
	out.Security.KeyLen = len(c.Security.EncryptionKey)
	out.Security.IsDev = c.Runtime.Dev
	return out
}

func LoadConfig() (*Config, error) {
	var configPath string
	var dev bool
	flag.StringVar(&configPath, "config", "config.yaml", "path to config yaml")
	flag.BoolVar(&dev, "dev", false, "development mode")
	flag.Parse()

	// Step 1: Load base config from YAML file
	b, err := os.ReadFile(configPath)
	if err != nil {
		// It's okay for the file to not exist if all config is provided by env vars
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config: %w", err)
		}
		b = []byte{} // Use empty bytes if file doesn't exist
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Step 2: Override with environment variables for secrets and key settings
	// Bot
	if token := os.Getenv("BOT_TOKEN"); token != "" {
		cfg.Bot.Token = token
	}
	// Database
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		cfg.Database.URL = dbURL
	}
	// Redis
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		cfg.Redis.URL = redisURL
	}
	// Security
	if encKey := os.Getenv("SECURITY_ENCRYPTION_KEY"); encKey != "" {
		cfg.Security.EncryptionKey = encKey
	}
	// AI Providers
	if openAIKey := os.Getenv("AI_OPENAI_API_KEY"); openAIKey != "" {
		cfg.AI.OpenAI.APIKey = openAIKey
	}
	if geminiKey := os.Getenv("AI_GEMINI_API_KEY"); geminiKey != "" {
		cfg.AI.Gemini.APIKey = geminiKey
	}
	// Payment Gateway
	if merchantID := os.Getenv("PAYMENT_ZARINPAL_MERCHANT_ID"); merchantID != "" {
		cfg.Payment.ZarinPal.MerchantID = merchantID
	}
	if callbackURL := os.Getenv("PAYMENT_ZARINPAL_CALLBACK_URL"); callbackURL != "" {
		cfg.Payment.ZarinPal.CallbackURL = callbackURL
	}
	if apiKey := os.Getenv("ADMIN_API_KEY"); apiKey != "" {
		cfg.Admin.APIKey = apiKey
	}

	// Step 3: Apply defaults for non-sensitive values
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

	if cfg.AI.OpenAI.DefaultModel == "" {
		cfg.AI.OpenAI.DefaultModel = "gpt-4o-mini"
	}

	if cfg.AI.Gemini.DefaultModel == "" {
		cfg.AI.Gemini.DefaultModel = "gemini-1.5-flash"
	}

	// Step 4: Final validation (will now use the merged config)
	cfg.Runtime.Dev = dev
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return &cfg, nil
}

func (cfg *Config) Validate() error {
	// MaxOutputTokens
	if cfg.AI.MaxOutputTokens < 0 {
		return fmt.Errorf("ai.max_output_tokens cannot be negative")
	}
	// ModelProviderMap must reference configured providers
	for model, prov := range cfg.AI.ModelProviderMap {
		p := strings.ToLower(strings.TrimSpace(prov))
		switch p {
		case "openai":
			if cfg.AI.OpenAI.APIKey == "" {
				return fmt.Errorf("ai.model_provider_map[%q]=openai but ai.openai.api_key is empty", model)
			}
		case "gemini":
			if cfg.AI.Gemini.APIKey == "" {
				return fmt.Errorf("ai.model_provider_map[%q]=gemini but ai.gemini.api_key is empty", model)
			}
		case "":
			return fmt.Errorf("ai.model_provider_map[%q]: provider is empty", model)
		default:
			return fmt.Errorf("ai.model_provider_map[%q]: unknown provider %q", model, prov)
		}
	}
	// Security: enforce 32-byte key in non-dev
	if !cfg.Runtime.Dev {
		if len(cfg.Security.EncryptionKey) != 32 {
			return fmt.Errorf("security.encryption_key must be exactly 32 bytes in production")
		}
	}
	return nil
}

func LoadConfigWithLogger(boot *zerolog.Logger) (*Config, error) {
	cfg, err := LoadConfig() // call your existing LoadConfig
	if err != nil {
		if boot != nil {
			boot.Error().Err(err).Msg("config.load.error")
		}
		return nil, err
	}
	if err := cfg.Validate(); err != nil { // you already added Validate()
		if boot != nil {
			boot.Error().Err(err).Msg("config.validate.error")
		}
		return nil, fmt.Errorf("config validation: %w", err)
	}
	if boot != nil {
		boot.Info().
			Str("event", "config.loaded").
			Interface("log", cfg.Log).
			Interface("ai", cfg.AI.Safe()). // redacted keys (see below)
			Msg("")
	}
	return cfg, nil
}

func normalizeTTL(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Hour
	}
	return d
}
