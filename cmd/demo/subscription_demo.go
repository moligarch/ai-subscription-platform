package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/infrastructure/db"
	"telegram-ai-subscription/internal/usecase"
)

func main() {
	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	// Connect
	pool, err := pgxpool.Connect(context.Background(), cfg.Database.URL)
	if err != nil {
		log.Fatalf("db connect error: %v", err)
	}
	defer pool.Close()

	// Repos & UCs
	userRepo := db.NewPostgresUserRepository(pool)
	planRepo := db.NewPostgresPlanRepo(pool)
	subRepo := db.NewPostgresSubscriptionRepo(pool)

	userUC := usecase.NewUserUseCase(userRepo)
	planUC := usecase.NewPlanUseCase(planRepo)
	subUC := usecase.NewSubscriptionUseCase(planRepo, subRepo)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1) Ensure user exists
	tgID := int64(42424242)
	user, err := userUC.RegisterOrFetch(ctx, tgID, "Demo User")
	if err != nil {
		log.Fatalf("RegisterOrFetch: %v", err)
	}
	fmt.Printf("User: %+v\n", user)

	// 2) Create demo plan
	planID := uuid.NewString()
	plan := &domain.SubscriptionPlan{
		ID:           planID,
		Name:         "Demo Plan",
		DurationDays: 7,
		Credits:      5,
		CreatedAt:    time.Now(),
	}
	if err := planUC.Create(ctx, plan); err != nil {
		log.Fatalf("plan create: %v", err)
	}
	fmt.Printf("Plan created: %+v\n", plan)

	// 3) Subscribe
	sub, err := subUC.Subscribe(ctx, user.ID, plan.ID)
	if err != nil {
		log.Fatalf("subscribe: %v", err)
	}
	fmt.Printf("Subscribed: %+v\n", sub)

	// 4) Deduct a credit
	updated, err := subUC.DeductCredit(ctx, sub)
	if err != nil {
		log.Fatalf("deduct: %v", err)
	}
	fmt.Printf("After deduct: %+v\n", updated)
}
