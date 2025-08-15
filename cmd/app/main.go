// File: cmd/app/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/application"
	"telegram-ai-subscription/internal/config"
	pg "telegram-ai-subscription/internal/infra/db/postgres"
	"telegram-ai-subscription/internal/infra/telegram"
	"telegram-ai-subscription/internal/usecase"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		dir, oerr := os.Getwd()
		if oerr != nil {
			log.Println("Error:", oerr)
			return
		}
		log.Println("Current Directory:", dir)
		log.Fatalf("failed to load config: %v", err)
	}

	// Connect to Postgres
	pool, err := pgxpool.Connect(context.Background(), cfg.Database.URL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	// ensure pool closed at exit
	defer pool.Close()

	// Repositories
	userRepo := pg.NewPostgresUserRepository(pool)
	if userRepo == nil {
		log.Fatalf("failed to init user repo")
	}

	planRepo := pg.NewPostgresPlanRepo(pool)
	if planRepo == nil {
		log.Fatalf("failed to init plan repo")
	}

	subRepo := pg.NewPostgresSubscriptionRepo(pool)
	if subRepo == nil {
		log.Fatalf("failed to init subscription repo")
	}

	payRepo := pg.NewPostgresPaymentRepo(pool)
	if payRepo == nil {
		log.Fatalf("failed to init payment repo")
	}

	// Usecases
	userUC := usecase.NewUserUseCase(userRepo)
	planUC := usecase.NewPlanUseCase(planRepo)
	subUC := usecase.NewSubscriptionUseCase(planRepo, subRepo)
	payUC := usecase.NewPaymentUseCase(payRepo, subUC)
	statsUC := usecase.NewStatsUseCase(userRepo, subRepo, payRepo)

	// Notification usecase: pass subRepo, bot may be set later
	notifUC := usecase.NewNotificationUseCase(subRepo, nil)

	// create facade (pass notifUC even though bot is not set yet)
	botFacade := application.NewBotFacade(userUC, planUC, subUC, payUC, statsUC, notifUC)

	// pass facade to telegram adapter constructor
	botAdapter, err := telegram.NewRealTelegramBotAdapter(&cfg.Bot, userRepo, botFacade, 5)
	if err != nil {
		log.Fatalf("failed to init telegram bot adapter: %v", err)
	}

	// Now that we have the real bot adapter, set it into the notification usecase so it can send messages
	notifUC.SetBot(botAdapter)

	// Start polling updates in background
	go func() {
		if err := botAdapter.StartPolling(ctx); err != nil {
			log.Printf("telegram polling stopped with error: %v", err)
		}
	}()

	// Graceful shutdown handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	<-sigCh
	log.Println("Shutting down gracefully...")
	// stop bot polling
	botAdapter.StopPolling()

	// give components a moment to finish
	time.Sleep(2 * time.Second)
}
