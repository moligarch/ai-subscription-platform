package application

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"telegram-ai-subscription/internal/domain/model"

	"github.com/google/uuid"
)

// BotFacade provides a high-level interface for bot commands, using various use cases.
type BotFacade struct {
	UserUC  UserUseCaseIface
	PlanUC  PlanUseCaseIface
	SubUC   SubscriptionUseCaseIface
	PayUC   PaymentUseCaseIface
	StatsUC StatsUseCaseIface
	NotifUC NotificationUseCaseIface
}

// NewBotFacade creates a new BotFacade with the provided use case interfaces.
func NewBotFacade(
	userUC UserUseCaseIface,
	planUC PlanUseCaseIface,
	subUC SubscriptionUseCaseIface,
	payUC PaymentUseCaseIface,
	statsUC StatsUseCaseIface,
	notifUC NotificationUseCaseIface,
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

// HandleSubscribe initiates a payment for the plan and returns the payment URL.
func (b *BotFacade) HandleSubscribe(ctx context.Context, tgID int64, username string, planID string) (string, error) {
	if b.UserUC == nil || b.PayUC == nil || b.PlanUC == nil {
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

	// Determine amount based on plan - for now, use a hardcoded value (100000 Rials = 10,000 Tomans)
	amountIRR := int64(100000)
	description := fmt.Sprintf("Subscription to %s", plan.Name)
	meta := map[string]interface{}{
		"telegram_id": tgID,
		"username":    username,
	}

	// Initiate payment
	payment, payURL, err := b.PayUC.Initiate(ctx, user.ID, plan.ID, amountIRR, description, meta)
	if err != nil {
		return "", fmt.Errorf("payment initiation failed: %w", err)
	}

	// Store payment ID in meta for future verification if needed
	// You might want to store this in a database or cache for later reference
	log.Printf("Initiated payment %s for user %s", payment.ID, user.ID)

	return fmt.Sprintf("Please complete your payment at: %s\nAfter payment, your subscription will be activated automatically.", payURL), nil
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

// HandleStats assembles and returns admin-facing stats text.
func (b *BotFacade) HandleStats(ctx context.Context) (string, error) {
	numUsers, err := b.UserUC.CountUsers(ctx)
	if err != nil {
		return "", err
	}
	numInactive, err := b.UserUC.CountInactiveUsers(ctx, time.Now().AddDate(0, -1, 0))
	if err != nil {
		return "", err
	}
	byPlan, err := b.SubUC.CountActiveSubscriptionsByPlan(ctx)
	if err != nil {
		return "", err
	}
	totalCredits, err := b.SubUC.TotalRemainingCredits(ctx)
	if err != nil {
		totalCredits = 0
	}
	totalWeek, _ := b.PayUC.TotalPayments(ctx, "week")
	totalMonth, _ := b.PayUC.TotalPayments(ctx, "month")
	totalYear, _ := b.PayUC.TotalPayments(ctx, "year")

	var sb strings.Builder
	sb.WriteString("Statistics:\n\n")
	sb.WriteString(fmt.Sprintf("Total users: %d\n", numUsers))
	sb.WriteString(fmt.Sprintf("Inactive users: %d\n\n", numInactive))
	sb.WriteString("Active subscriptions by plan:\n")
	for name, cnt := range byPlan {
		sb.WriteString(fmt.Sprintf("- %s: %d\n", name, cnt))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Total credits (remaining): %d\n", totalCredits))
	sb.WriteString(fmt.Sprintf("Payments - week: %d, month: %d, year: %d\n", totalWeek, totalMonth, totalYear))
	return sb.String(), nil
}

// HandleCreatePlan creates a new subscription plan.
func (b *BotFacade) HandleCreatePlan(ctx context.Context, name string, durationDays, credits int) (string, error) {
	if b.PlanUC == nil {
		return "", fmt.Errorf("plan usecase not available")
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("plan name is required")
	}
	if durationDays <= 0 {
		return "", fmt.Errorf("durationDays must be > 0")
	}
	if credits < 0 {
		return "", fmt.Errorf("credits must be >= 0")
	}

	p := &model.SubscriptionPlan{
		ID:           uuid.NewString(),
		Name:         strings.TrimSpace(name),
		DurationDays: durationDays,
		Credits:      credits,
		CreatedAt:    time.Now(),
	}

	if err := b.PlanUC.Create(ctx, p); err != nil {
		return "", fmt.Errorf("create plan: %w", err)
	}

	return fmt.Sprintf("Plan created: %s (id: %s) — %d days, %d credits", p.Name, p.ID, p.DurationDays, p.Credits), nil
}

// HandleUpdatePlan updates an existing plan.
func (b *BotFacade) HandleUpdatePlan(ctx context.Context, planID, name string, durationDays, credits int) (string, error) {
	if b.PlanUC == nil {
		return "", fmt.Errorf("plan usecase not available")
	}
	if strings.TrimSpace(planID) == "" {
		return "", fmt.Errorf("plan id is required")
	}
	existing, err := b.PlanUC.Get(ctx, planID)
	if err != nil {
		return "", fmt.Errorf("plan not found: %w", err)
	}

	existing.Name = strings.TrimSpace(name)
	existing.DurationDays = durationDays
	existing.Credits = credits

	if err := b.PlanUC.Update(ctx, existing); err != nil {
		return "", fmt.Errorf("update plan: %w", err)
	}

	return fmt.Sprintf("Plan updated: %s (id: %s) — %d days, %d credits", existing.Name, existing.ID, existing.DurationDays, existing.Credits), nil
}

// HandleDeletePlan deletes the given plan by id.
func (b *BotFacade) HandleDeletePlan(ctx context.Context, planID string) (string, error) {
	if b.PlanUC == nil {
		return "", fmt.Errorf("plan usecase not available")
	}
	if strings.TrimSpace(planID) == "" {
		return "", fmt.Errorf("plan id is required")
	}
	if err := b.PlanUC.Delete(ctx, planID); err != nil {
		return "", fmt.Errorf("delete plan: %w", err)
	}
	return fmt.Sprintf("Plan %s deleted.", planID), nil
}
