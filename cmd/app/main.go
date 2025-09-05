package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"telegram-ai-subscription/internal/application"
	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/infra/adapters/ai"
	payAdapters "telegram-ai-subscription/internal/infra/adapters/payment"
	tele "telegram-ai-subscription/internal/infra/adapters/telegram"
	"telegram-ai-subscription/internal/infra/api"
	pg "telegram-ai-subscription/internal/infra/db/postgres"
	"telegram-ai-subscription/internal/infra/logging"
	appmetrics "telegram-ai-subscription/internal/infra/metrics"
	red "telegram-ai-subscription/internal/infra/redis"
	"telegram-ai-subscription/internal/infra/sched"
	"telegram-ai-subscription/internal/infra/security"
	"telegram-ai-subscription/internal/usecase"

	"github.com/rs/zerolog"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ---- Boot logger (pre-config) ----
	boot := zerolog.New(os.Stderr).With().
		Timestamp().
		Str("cmp", "boot").
		Logger()

	// ---- Config ----
	cfg, err := config.LoadConfigWithLogger(&boot) // new helper; see below
	if err != nil {
		boot.Error().Err(err).Msg("config load failed")
		os.Exit(1)
	}

	// ---- Logging ----
	logger := logging.New(cfg.Log, cfg.Runtime.Dev) // your existing function

	// log effective (redacted) config once final logger is ready
	logger.Info().
		Str("event", "config.effective").
		Interface("config", cfg.Redacted()).
		Msg("")

	if cfg.Runtime.Dev {
		logger.Info().Msg("[DEV MODE] Enabled")
	}

	// ---- Metrics ----
	appmetrics.MustRegister()

	// ---- Redis ----
	redisClient, err := red.NewClient(ctx, &cfg.Redis)
	if err != nil {
		logger.Fatal().Err(err).Msg("redis")
	}
	defer redisClient.Close()

	// Redis-based services
	rateLimiter := red.NewRateLimiter(redisClient)
	chatCache := red.NewChatCache(redisClient, cfg.Redis.TTL)
	locker := red.NewLocker(redisClient)

	// ---- Postgres ----
	pool, err := pg.TryConnect(ctx, cfg.Database.URL, int32(cfg.Database.PoolMaxConns), 30*time.Second)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres")
	}
	defer pg.ClosePgxPool(pool)

	// ---- Encryption ----
	encKey := cfg.Security.EncryptionKey
	if len(encKey) != 32 {
		logger.Warn().Msg("security.encryption_key not 32 bytes; using insecure dev key")
		encKey = "0123456789abcdef0123456789abcdef"
	}
	enc, err := security.NewEncryptionService(encKey)
	if err != nil {
		logger.Fatal().Err(err).Msg("encryption init failed")
	}

	// ---- Repositories ----
	userRepo := pg.NewUserRepo(pool)
	planRepo := pg.NewPlanRepo(pool)
	subRepo := pg.NewSubscriptionRepo(pool)
	payRepo := pg.NewPaymentRepo(pool)
	purchaseRepo := pg.NewPurchaseRepo(pool)
	chatRepo := pg.NewChatSessionRepo(pool, chatCache, enc)

	// ---- Use Cases ----
	userUC := usecase.NewUserUseCase(userRepo, logger)
	planUC := usecase.NewPlanUseCase(planRepo, logger)
	subUC := usecase.NewSubscriptionUseCase(subRepo, planRepo, logger)

	providers := map[string]adapter.AIServiceAdapter{}

	if cfg.AI.OpenAI.APIKey != "" {
		oa, err := ai.NewOpenAIAdapter(
			cfg.AI.OpenAI.APIKey,
			cfg.AI.OpenAI.BaseURL,
			cfg.AI.OpenAI.DefaultModel,
			cfg.AI.MaxOutputTokens, // NEW
		)
		if err != nil {
			logger.Warn().Err(err).Msg("[OpenAI Adapter]")
		} else {
			providers["openai"] = ai.NewLimitedAI(oa, cfg.AI.ConcurrentLimit)
			logger.Info().Str("default", cfg.AI.OpenAI.DefaultModel).Msg("[OpenAI Adapter]")
		}
	}

	if cfg.AI.Gemini.APIKey != "" {
		ga, err := ai.NewGeminiAdapter(
			ctx,
			cfg.AI.Gemini.APIKey,
			cfg.AI.Gemini.BaseURL,
			cfg.AI.Gemini.DefaultModel,
			cfg.AI.MaxOutputTokens, // NEW
		)
		if err != nil {
			logger.Warn().Err(err).Msg("[Gemini Adapter]")
		} else {
			providers["gemini"] = ai.NewLimitedAI(ga, cfg.AI.ConcurrentLimit)
			logger.Info().Str("default", cfg.AI.Gemini.DefaultModel).Msg("[Gemini Adapter]")
		}
	}

	// composite used across the app
	aiRouter := ai.NewMultiAIAdapter("openai", providers, cfg.AI.ModelProviderMap)

	priceRepo := pg.NewModelPricingRepo(pool)
	chatUC := usecase.NewChatUseCase(chatRepo, aiRouter, subUC, locker, logger, cfg.Runtime.Dev, priceRepo)

	// Payment gateway + use case
	zp, err := payAdapters.NewZarinPalGateway(cfg.Payment.ZarinPal.MerchantID, cfg.Payment.ZarinPal.CallbackURL, cfg.Payment.ZarinPal.Sandbox)
	if err != nil {
		logger.Fatal().Err(err).Msg("zarinpal gateway")
	}
	paymentUC := usecase.NewPaymentUseCase(payRepo, planRepo, subUC, purchaseRepo, zp, logger)

	_ = usecase.NewStatsUseCase(userRepo, subRepo, payRepo, logger)
	notifUC := usecase.NewNotificationUseCase(subRepo, logger)
	_ = notifUC // wired by schedulers/workers later

	// Compute callback path from full URL in config (fallback to default)
	cbPath := "/api/payment/callback"
	if u := strings.TrimSpace(cfg.Payment.ZarinPal.CallbackURL); u != "" {
		if parsed, err := url.Parse(u); err == nil && parsed.Path != "" {
			cbPath = parsed.Path
		}
	}

	// Bot facade (used by telegram adapter)
	facade := application.NewBotFacade(userUC, planUC, subUC, paymentUC, chatUC, cfg.Payment.ZarinPal.CallbackURL)

	// ---- Telegram ----
	botAdapter, err := tele.NewRealTelegramBotAdapter(&cfg.Bot, userRepo, facade, rateLimiter, cfg.Bot.Workers, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("telegram adapter")
	}
	defer botAdapter.StopPolling() // ensure we stop cleanly on shutdown
	if strings.ToLower(cfg.Bot.Mode) != "polling" {
		logger.Warn().Str("mode", cfg.Bot.Mode).Msg("bot.mode not implemented; using polling")
	}
	go func() {
		if err := botAdapter.StartPolling(ctx); err != nil {
			logger.Error().Err(err).Msg("telegram polling stopped")
		}
	}()

	// ---- HTTP callback server with guards ----
	srv := api.NewServer(paymentUC, userRepo, botAdapter, cbPath, cfg.Bot.Username)
	mux := http.NewServeMux()
	srv.Register(mux)
	handler := api.Chain(mux,
		api.TraceID(logger),
		api.RequestLog(logger),
		api.Recover(logger),
		api.Timeout(2*time.Second),
	)
	httpPort := cfg.Payment.ZarinPal.CallbackPort
	if httpPort == 0 {
		httpPort = cfg.Admin.Port
	}
	server := &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", httpPort), Handler: handler}
	// graceful shutdown for the HTTP server
	defer func() {
		shCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shCancel()
		_ = server.Shutdown(shCtx)
	}()

	go func() {
		logger.Info().Str("addr", server.Addr).Str("path", cbPath).Msg("http listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().Err(err).Msg("http server error")
		}
	}()

	// ---- Background workers ----
	// Expiry worker: hourly sweep
	expiryWorker := sched.NewExpiryWorker(1*time.Hour, subRepo, planRepo, subUC)
	go func() { _ = expiryWorker.Run(ctx) }()

	// Payment reconciler: periodically reconcile stuck/pending payments
	reconciler := sched.NewPaymentReconciler(paymentUC, payRepo, 10*time.Second, 1*time.Minute)
	go func() { reconciler.Start(ctx) }()

	// ---- Graceful shutdown ----
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	<-sigc
	logger.Info().Msg("shutdown requested")
	cancel()
}
