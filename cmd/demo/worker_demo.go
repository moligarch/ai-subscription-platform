package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain/model"
	pg "telegram-ai-subscription/internal/infra/db/postgres"
	"telegram-ai-subscription/internal/infra/scheduler"
	infraTelegram "telegram-ai-subscription/internal/infra/telegram"
	"telegram-ai-subscription/internal/infra/worker"
	"telegram-ai-subscription/internal/usecase"
)

func main() {
	// load config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("config load: %v", err)
	}

	// connect
	pool, err := pgxpool.Connect(context.Background(), cfg.Database.URL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	// repos and usecases/executor
	userRepo := pg.NewPostgresUserRepository(pool)
	planRepo := pg.NewPostgresPlanRepo(pool)
	subRepo := pg.NewPostgresSubscriptionRepo(pool)

	userUC := usecase.NewUserUseCase(userRepo)
	// executor used by worker pool
	subExecutor := usecase.NewSubscriptionExecutor(planRepo, subRepo)
	// notification usecase (uses Telegram adapter)
	telegramAdapter := infraTelegram.NewNoopBotAdapter()
	notificationUC := usecase.NewNotificationUseCase(subRepo, telegramAdapter)

	// create demo user
	ctx := context.Background()
	tgID := int64(555000111)
	user, err := userUC.RegisterOrFetch(ctx, tgID, "WorkerDemoUser")
	if err != nil {
		log.Fatalf("user create: %v", err)
	}
	fmt.Printf("Demo user id: %s\n", user.ID)

	// create demo plan if not exists
	planID := uuid.NewString()
	plan := &model.SubscriptionPlan{
		ID:           planID,
		Name:         "Worker-Demo-Plan",
		DurationDays: 7,
		Credits:      3,
		CreatedAt:    time.Now(),
	}
	// Save plan via repo directly (or you can create a PlanUseCase if you want)
	if err := planRepo.Save(ctx, plan); err != nil {
		log.Fatalf("plan create: %v", err)
	}
	fmt.Printf("Plan created: %s\n", planID)

	// start scheduler: runs CheckAndNotify every 30s (demo)
	sched := scheduler.NewScheduler(30*time.Second, notificationUC)
	sched.Start(context.Background())
	defer sched.Stop()

	// start worker pool
	poolSize := 4
	queueSize := 16
	workerPool := worker.NewWorkerPool(poolSize, queueSize, subExecutor)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := workerPool.Shutdown(shutdownCtx); err != nil {
			log.Printf("worker pool shutdown err: %v", err)
		}
	}()

	// Submit a few subscription tasks concurrently
	numTasks := 6
	results := make([]chan worker.SubscribeResult, numTasks)
	for i := 0; i < numTasks; i++ {
		resultCh := make(chan worker.SubscribeResult, 1)
		results[i] = resultCh
		task := &worker.SubscribeTask{
			UserID:   user.ID,
			PlanID:   planID,
			ResultCh: resultCh,
			Ctx:      context.Background(),
		}
		if err := workerPool.Submit(task); err != nil {
			log.Printf("submit error: %v", err)
			close(resultCh)
			continue
		}
	}

	// Wait for results
	for i, ch := range results {
		res := <-ch
		if res.Err != nil {
			log.Printf("task %d error: %v", i, res.Err)
			continue
		}
		log.Printf("task %d subscription: %+v", i, res.Subscription)
	}

	// Wait for signal to gracefully exit
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	log.Println("Demo running. Press Ctrl+C to exit.")
	<-sig
	log.Println("Shutting down demo")
}
