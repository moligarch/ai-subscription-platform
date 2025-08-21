// File: cmd/app/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"telegram-ai-subscription/internal/application"
	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/infra/db/postgres"
	"telegram-ai-subscription/internal/infra/http"
	"telegram-ai-subscription/internal/infra/payment"
	"telegram-ai-subscription/internal/infra/redis"
	"telegram-ai-subscription/internal/infra/telegram"
	"telegram-ai-subscription/internal/usecase"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create PostgreSQL pool
	pool := postgres.MustConnectPostgres(&cfg.Database)
	defer pool.Close()

	// Create Redis client
	redisClient := redis.NewRedisClient(&cfg.Redis)
	defer redisClient.Close()

	// Ping Redis to check connection
	ctx := context.Background()
	if err := redisClient.Ping(ctx); err != nil {
		log.Fatalf("Redis ping failed: %v", err)
	}

	// Create rate limiter
	rateLimiter := redis.NewRateLimiter(redisClient)

	// Initialize repositories
	userRepo := postgres.NewPostgresUserRepository(pool)
	planRepo := postgres.NewPostgresPlanRepo(pool)
	subRepo := postgres.NewPostgresSubscriptionRepo(pool)
	paymentRepo := postgres.NewPostgresPaymentRepo(pool)
	purchaseRepo := postgres.NewPostgresPurchaseRepo(pool)

	// Initialize use cases
	userUC := usecase.NewUserUseCase(userRepo)
	planUC := usecase.NewPlanUseCase(planRepo)
	subUC := usecase.NewSubscriptionUseCase(planRepo, subRepo, pool)
	statsUC := usecase.NewStatsUseCase(userRepo, subRepo, paymentRepo)
	notifUC := usecase.NewNotificationUseCase(subRepo, nil) // bot adapter not set yet

	// Create payment gateway
	zarinpalGateway := payment.NewZarinPalDirectGateway(cfg.Payment.ZarinPal.MerchantID, cfg.Payment.ZarinPal.Sandbox)

	// Create payment use case
	paymentUC := usecase.NewPaymentUseCase(paymentRepo, purchaseRepo, *subUC, zarinpalGateway, cfg.Payment.ZarinPal.CallbackURL)

	// Create bot facade
	botFacade := application.NewBotFacade(userUC, planUC, subUC, paymentUC, statsUC, notifUC)

	// Create Telegram bot adapter
	botAdapter, err := telegram.NewRealTelegramBotAdapter(&cfg.Bot, userRepo, botFacade, rateLimiter, 5)
	if err != nil {
		log.Fatalf("Failed to create bot adapter: %v", err)
	}

	// Set the bot adapter for notifications
	notifUC.SetBot(botAdapter)

	// Create HTTP server for webhooks
	httpServer := http.NewServer(cfg, *paymentUC, userRepo)

	// Start components
	go func() {
		if err := httpServer.Start(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start bot in polling mode
	if cfg.Bot.Mode == "polling" {
		go func() {
			if err := botAdapter.StartPolling(ctx); err != nil {
				log.Printf("Bot polling error: %v", err)
			}
		}()
	} else {
		log.Printf("Webhook mode not implemented yet")
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Shutdown gracefully
	log.Printf("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	httpServer.Shutdown(shutdownCtx)
	botAdapter.StopPolling()
}
