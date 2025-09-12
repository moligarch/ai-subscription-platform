// File: .\cmd\seed\main.go
package main

import (
	"context"
	"log"
	"time"

	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/db/postgres"

	"github.com/google/uuid"
)

func main() {
	ctx := context.Background()

	// Load config (so we can reuse DB DSN)
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("config load: %v", err)
	}

	// Connect Postgres
	pool, err := postgres.NewPgxPool(ctx, cfg.Database.URL, 20)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()

	// Repos
	plans := postgres.NewPlanRepo(pool)
	prices := postgres.NewModelPricingRepo(pool)

	now := time.Now()

	// --- Seed Plans (micro-credits) ---
	// TIP: credits are MICRO-CREDITS; adjust to match your pricing.
	seedPlans := []model.SubscriptionPlan{
		{
			ID:           uuid.NewString(),
			Name:         "Starter",
			DurationDays: 30,
			Credits:      20_000_000, // 20.0 credits in micro units (example)
			PriceIRR:     3_900_000,
			CreatedAt:    now,
		},
		{
			ID:           uuid.NewString(),
			Name:         "Pro",
			DurationDays: 30,
			Credits:      100_000_000, // 100.0 credits
			PriceIRR:     15_900_000,
			CreatedAt:    now,
		},
	}

	for _, p := range seedPlans {
		if err := plans.Save(ctx, repository.NoTX, &p); err != nil {
			log.Printf("plan upsert %s: %v", p.ID, err)
		} else {
			log.Printf("plan upserted: %s", p.ID)
		}
	}

	// --- Seed Model Pricing (per-token prices in micro-credits) ---
	// You can tweak these to your actual pricing. The keys must match model names user selects.
	seedPrices := []model.ModelPricing{
		{
			ID:                     uuid.NewString(),
			ModelName:              "gpt-4o-mini",
			InputTokenPriceMicros:  30, // 0.000030 credits per input token
			OutputTokenPriceMicros: 60, // 0.000060 credits per output token
			Active:                 true,
			CreatedAt:              now, UpdatedAt: now,
		},
		{
			ID:                     uuid.NewString(),
			ModelName:              "gpt-4o",
			InputTokenPriceMicros:  150,
			OutputTokenPriceMicros: 300,
			Active:                 true,
			CreatedAt:              now, UpdatedAt: now,
		},
		{
			ID:                     uuid.NewString(),
			ModelName:              "gemini-1.5-flash",
			InputTokenPriceMicros:  40,
			OutputTokenPriceMicros: 80,
			Active:                 true,
			CreatedAt:              now, UpdatedAt: now,
		},
		{
			ID:                     uuid.NewString(),
			ModelName:              "gemini-1.5-pro",
			InputTokenPriceMicros:  90,
			OutputTokenPriceMicros: 180,
			Active:                 true,
			CreatedAt:              now, UpdatedAt: now,
		},
	}

	for _, pr := range seedPrices {
		if err := prices.Create(ctx, repository.NoTX, &pr); err != nil {
			log.Printf("pricing not create %s: %v", pr.ModelName, err)
		} else {
			log.Printf("pricing created: %s", pr.ModelName)
		}
	}

	log.Println("✅ seed complete")
}
