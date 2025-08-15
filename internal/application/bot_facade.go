package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"telegram-ai-subscription/internal/usecase"
)

// BotFacade composes usecases into high-level bot commands.
// Keep the facade methods returning strings so the Telegram adapter just forwards them to the chat.
type BotFacade struct {
	UserUC  *usecase.UserUseCase
	PlanUC  *usecase.PlanUseCase
	SubUC   *usecase.SubscriptionUseCase
	PayUC   *usecase.PaymentUseCase
	StatsUC *usecase.StatsUseCase
	NotifUC *usecase.NotificationUseCase
}

// NewBotFacade constructs a facade from provided usecases. Any of the usecases can be nil
// if not needed for some demo flows (but methods that use them will return errors).
func NewBotFacade(
	userUC *usecase.UserUseCase,
	planUC *usecase.PlanUseCase,
	subUC *usecase.SubscriptionUseCase,
	payUC *usecase.PaymentUseCase,
	statsUC *usecase.StatsUseCase,
	notifUC *usecase.NotificationUseCase,
) *BotFacade {
	return &BotFacade{
		UserUC:  userUC,
		PlanUC:  planUC,
		SubUC:   subUC,
		PayUC:   payUC,
		StatsUC: statsUC,
		NotifUC: notifUC,
	}
}

// HandleStart registers or fetches the user and returns a welcome string.
func (b *BotFacade) HandleStart(ctx context.Context, tgID int64, username string) (string, error) {
	if b.UserUC == nil {
		return "", fmt.Errorf("user usecase not available")
	}
	u, err := b.UserUC.RegisterOrFetch(ctx, tgID, username)
	if err != nil {
		return "", fmt.Errorf("register/fetch user: %w", err)
	}
	return fmt.Sprintf("Hello %s!\nYour account id: %s\nUse /plans to see subscription plans.", username, u.ID), nil
}

// HandlePlans returns a formatted list of plans.
func (b *BotFacade) HandlePlans(ctx context.Context) (string, error) {
	if b.PlanUC == nil {
		return "", fmt.Errorf("plan usecase not available")
	}
	plans, err := b.PlanUC.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list plans: %w", err)
	}
	if len(plans) == 0 {
		return "No plans available right now.", nil
	}
	sb := strings.Builder{}
	sb.WriteString("Available plans:\n")
	for _, p := range plans {
		sb.WriteString(fmt.Sprintf("- %s (id: %s): %d days, %d credits\n", p.Name, p.ID, p.DurationDays, p.Credits))
	}
	sb.WriteString("\nSubscribe with: /subscribe <plan_id>")
	return sb.String(), nil
}

// HandleSubscribe subscribes telegram user (demo-mode: immediately create subscription).
// It will ensure the user exists via RegisterOrFetch, then subscribe the user's domain ID.
func (b *BotFacade) HandleSubscribe(ctx context.Context, tgID int64, username string, planID string) (string, error) {
	if b.UserUC == nil || b.SubUC == nil || b.PlanUC == nil {
		return "", fmt.Errorf("some usecases are not available")
	}

	// Ensure plan exists
	plan, err := b.PlanUC.Get(ctx, planID)
	if err != nil {
		return "", fmt.Errorf("plan not found: %w", err)
	}

	// Ensure or create user
	user, err := b.UserUC.RegisterOrFetch(ctx, tgID, username)
	if err != nil {
		return "", fmt.Errorf("register/fetch user: %w", err)
	}

	// Subscribe (demo: immediate subscribe)
	sub, err := b.SubUC.Subscribe(ctx, user.ID, plan.ID)
	if err != nil {
		return "", fmt.Errorf("subscribe: %w", err)
	}

	return fmt.Sprintf("✅ Subscribed to *%s*.\nExpires: %s\nRemaining credits: %d",
		plan.Name, sub.ExpiresAt.Format(time.RFC1123), sub.RemainingCredits), nil
}

// HandleMyPlan shows the active subscription for telegram user.
func (b *BotFacade) HandleMyPlan(ctx context.Context, tgID int64) (string, error) {
	if b.UserUC == nil || b.SubUC == nil || b.PlanUC == nil {
		return "", fmt.Errorf("some usecases are not available")
	}
	user, err := b.UserUC.GetByTelegramID(ctx, tgID)
	if err != nil {
		return "", fmt.Errorf("user not found: %w", err)
	}
	sub, err := b.SubUC.GetActiveSubscription(ctx, user.ID)
	if err != nil {
		return "You currently have no active subscription.", nil
	}
	plan, err := b.PlanUC.Get(ctx, sub.PlanID)
	if err != nil {
		// Plan missing in DB -> still show subscription basic info
		return fmt.Sprintf("Active subscription:\nPlan ID: %s\nExpires: %s\nRemaining credits: %d",
			sub.PlanID, sub.ExpiresAt.Format(time.RFC1123), sub.RemainingCredits), nil
	}
	return fmt.Sprintf("Active subscription:\nPlan: %s\nExpires: %s\nRemaining credits: %d",
		plan.Name, sub.ExpiresAt.Format(time.RFC1123), sub.RemainingCredits), nil
}

// HandleBalance returns the remaining credits for the user's active subscription.
func (b *BotFacade) HandleBalance(ctx context.Context, tgID int64) (string, error) {
	if b.UserUC == nil || b.SubUC == nil {
		return "", fmt.Errorf("usecases not available")
	}
	user, err := b.UserUC.GetByTelegramID(ctx, tgID)
	if err != nil {
		return "", fmt.Errorf("user not found: %w", err)
	}
	sub, err := b.SubUC.GetActiveSubscription(ctx, user.ID)
	if err != nil {
		return "You have no active subscription.", nil
	}
	return fmt.Sprintf("Remaining credits: %d\nExpires: %s", sub.RemainingCredits, sub.ExpiresAt.Format(time.RFC1123)), nil
}

// HandleStats builds admin-facing formatted stats string.
// Uses StatsUseCase to fetch counts/payments and formats a message.
func (b *BotFacade) HandleStats(ctx context.Context) (string, error) {
	if b.StatsUC == nil {
		return "", fmt.Errorf("stats usecase not available")
	}
	// default inactive window 30 days
	totalUsers, inactiveUsers, byPlan, totalCredits, err := b.StatsUC.GetCounts(ctx, 30*24*time.Hour)
	if err != nil {
		return "", fmt.Errorf("get counts: %w", err)
	}
	week, month, year, err := b.StatsUC.GetPaymentsForPeriods(ctx)
	if err != nil {
		return "", fmt.Errorf("get payments: %w", err)
	}

	formatMoney := func(c float64) string { return fmt.Sprintf("%.2f", c/100.0) }

	var sb strings.Builder
	sb.WriteString("📊 System Statistics:\n\n")
	sb.WriteString(fmt.Sprintf("👥 Users: %d\n", totalUsers))
	sb.WriteString(fmt.Sprintf("🚫 Deactivated (30d): %d\n\n", inactiveUsers))
	sb.WriteString("📦 Active subscriptions by plan:\n")
	for name, cnt := range byPlan {
		sb.WriteString(fmt.Sprintf("  - %s: %d\n", name, cnt))
	}
	sb.WriteString("\n💰 Payments:\n")
	sb.WriteString(fmt.Sprintf("  - This Week: %s\n", formatMoney(week)))
	sb.WriteString(fmt.Sprintf("  - This Month: %s\n", formatMoney(month)))
	sb.WriteString(fmt.Sprintf("  - This Year: %s\n\n", formatMoney(year)))
	sb.WriteString(fmt.Sprintf("🎫 Total Active Credits: %d\n", totalCredits))
	return sb.String(), nil
}
