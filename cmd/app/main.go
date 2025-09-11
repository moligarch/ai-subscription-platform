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
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/adapters/ai"
	payAdapters "telegram-ai-subscription/internal/infra/adapters/payment"
	tele "telegram-ai-subscription/internal/infra/adapters/telegram"
	"telegram-ai-subscription/internal/infra/api"
	pg "telegram-ai-subscription/internal/infra/db/postgres"
	"telegram-ai-subscription/internal/infra/i18n"
	"telegram-ai-subscription/internal/infra/logging"
	appmetrics "telegram-ai-subscription/internal/infra/metrics"
	red "telegram-ai-subscription/internal/infra/redis"
	"telegram-ai-subscription/internal/infra/sched"
	"telegram-ai-subscription/internal/infra/security"
	"telegram-ai-subscription/internal/infra/worker"
	"telegram-ai-subscription/internal/usecase"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
)

var (
	version = "dev"
	commit  = "none"
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

	// ---- Localization ----
	translator, err := i18n.NewTranslator(i18n.LocalesFS, "fa")
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load translations")
	}

	// ---- Metrics ----
	appmetrics.MustRegister()
	appmetrics.SetBuildInfo(version, commit)

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
	regStateRepo := red.NewRegistrationStateRepo(redisClient)

	// ---- Postgres ----
	pool, err := pg.TryConnect(ctx, cfg.Database.URL, int32(cfg.Database.PoolMaxConns), 30*time.Second)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres")
	}
	defer pg.ClosePgxPool(pool)

	// ---- Transaction Manager ----
	txManager := pg.NewTxManager(pool)

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
	dbUserRepo := pg.NewUserRepo(pool)
	userRepo := pg.NewUserRepoCacheDecorator(dbUserRepo, redisClient)

	dbPlanRepo := pg.NewPlanRepo(pool)
	planRepo := pg.NewPlanRepoCacheDecorator(dbPlanRepo, redisClient)

	subRepo := pg.NewSubscriptionRepo(pool)
	payRepo := pg.NewPaymentRepo(pool)
	purchaseRepo := pg.NewPurchaseRepo(pool)

	dbPriceRepo := pg.NewModelPricingRepo(pool)
	priceRepo := pg.NewModelPricingRepoCacheDecorator(dbPriceRepo, redisClient)

	aiJobRepo := pg.NewAIJobRepo(pool, txManager)
	chatRepo := pg.NewChatSessionRepo(pool, chatCache, enc)

	notifLogRepo := pg.NewNotificationLogRepo(pool)

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

	// ---- Use Cases ----
	userUC := usecase.NewUserUseCase(userRepo, chatRepo, regStateRepo, translator, txManager, logger)
	planUC := usecase.NewPlanUseCase(planRepo, priceRepo, logger)
	subUC := usecase.NewSubscriptionUseCase(subRepo, planRepo, txManager, logger)
	chatUC := usecase.NewChatUseCase(chatRepo, aiJobRepo, aiRouter, subUC, locker, txManager, logger, cfg.Runtime.Dev, priceRepo)

	// Payment gateway + use case
	zp, err := payAdapters.NewZarinPalGateway(cfg.Payment.ZarinPal.MerchantID, cfg.Payment.ZarinPal.CallbackURL, cfg.Payment.ZarinPal.Sandbox)
	if err != nil {
		logger.Fatal().Err(err).Msg("zarinpal gateway")
	}
	paymentUC := usecase.NewPaymentUseCase(payRepo, planRepo, subUC, purchaseRepo, zp, txManager, logger)

	_ = usecase.NewStatsUseCase(userRepo, subRepo, payRepo, logger)

	// Bot facade (used by telegram adapter)
	facade := application.NewBotFacade(userUC, planUC, subUC, paymentUC, chatUC, cfg.Payment.ZarinPal.CallbackURL)

	// ---- Telegram ----
	botAdapter, err := tele.NewRealTelegramBotAdapter(&cfg.Bot, userRepo, facade, translator, rateLimiter, cfg.Bot.Workers, logger)
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

	notifUC := usecase.NewNotificationUseCase(subRepo, notifLogRepo, userRepo, botAdapter, logger)

	// Compute callback path from full URL in config (fallback to default)
	cbPath := "/api/payment/callback"
	if u := strings.TrimSpace(cfg.Payment.ZarinPal.CallbackURL); u != "" {
		if parsed, err := url.Parse(u); err == nil && parsed.Path != "" {
			cbPath = parsed.Path
		}
	}

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
	go startMetricsCollector(ctx, pool, subRepo, logger)
	appWorkerPool := worker.NewPool(cfg.Bot.Workers) // Use the same worker count as the bot
	appWorkerPool.Start(ctx)
	defer appWorkerPool.Stop()

	// Notification worker: check for expiring subs every 6 hours
	notificationWorker := sched.NewNotificationWorker(6*time.Hour, notifUC, logger)
	go func() { _ = notificationWorker.Run(ctx) }()

	aiProcessor := worker.NewAIJobProcessor(
		aiJobRepo,
		chatRepo,
		priceRepo,
		subUC,
		aiRouter,
		// botAdapter needs to be an interface that can be passed here
		botAdapter,
		txManager,
		logger,
	)
	go aiProcessor.Start(ctx, appWorkerPool)

	// Expiry worker: hourly sweep
	expiryWorker := sched.NewExpiryWorker(1*time.Hour, subRepo, planRepo, subUC, logger)
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

func startMetricsCollector(ctx context.Context, pool *pgxpool.Pool, subRepo repository.SubscriptionRepository, log *zerolog.Logger) {
	cpLog := log.With().Str("component", "MetricsCollector").Logger()
	log = &cpLog
	log.Info().Msg("Starting metrics collector")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping metrics collector")
			return
		case <-ticker.C:
			// Collect DB Pool Stats
			stats := pool.Stat()
			appmetrics.SetDBPoolStats(stats.TotalConns(), stats.IdleConns(), stats.AcquiredConns())

			// Collect Subscription Stats
			subCounts, err := subRepo.CountByStatus(ctx, nil)
			if err != nil {
				log.Error().Err(err).Msg("failed to collect subscription stats")
			} else {
				appmetrics.SetSubscriptionsTotal(subCounts)
			}
		}
	}
}
