package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/rs/zerolog"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/logging"
)

const (
	StepAwaitingActivationCode = "awaiting_activation_code"
)

// Compile-time check
var _ SubscriptionUseCase = (*subscriptionUC)(nil)

type SubscriptionUseCase interface {
	Subscribe(ctx context.Context, userID, planID string) (*model.UserSubscription, error)
	GetActive(ctx context.Context, userID string) (*model.UserSubscription, error)
	GetReserved(ctx context.Context, userID string) ([]*model.UserSubscription, error)
	ListByUserID(ctx context.Context, userID string) ([]*model.UserSubscription, error)
	DeductCredits(ctx context.Context, userID string, amount int64) (*model.UserSubscription, error)
	FinishExpired(ctx context.Context) (int, error)
	RedeemActivationCode(ctx context.Context, userID, code string) (*model.UserSubscription, error)
}

type subscriptionUC struct {
	subs  repository.SubscriptionRepository
	plans repository.SubscriptionPlanRepository
	codes repository.ActivationCodeRepository
	tm    repository.TransactionManager
	log   *zerolog.Logger
}

func NewSubscriptionUseCase(
	subs repository.SubscriptionRepository,
	plans repository.SubscriptionPlanRepository,
	codes repository.ActivationCodeRepository,
	tm repository.TransactionManager,
	logger *zerolog.Logger,
) *subscriptionUC {
	return &subscriptionUC{
		subs:  subs,
		plans: plans,
		codes: codes,
		tm:    tm,
		log:   logger,
	}
}

func (u *subscriptionUC) Subscribe(ctx context.Context, userID, planID string) (*model.UserSubscription, error) {
	defer logging.TraceDuration(u.log, "SubscriptionUC.Subscribe")()
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(planID) == "" {
		return nil, errors.New("missing user or plan")
	}

	var sub *model.UserSubscription
	// Use a serializable transaction to prevent race conditions
	txOpts := pgx.TxOptions{IsoLevel: pgx.Serializable}
	err := u.tm.WithTx(ctx, txOpts, func(ctx context.Context, tx repository.Tx) error {
		plan, err := u.plans.FindByID(ctx, tx, planID)
		if err != nil {
			return domain.ErrNotFound
		}

		now := time.Now()
		active, _ := u.subs.FindActiveByUser(ctx, tx, userID)

		newSub := &model.UserSubscription{
			ID:               uuid.NewString(),
			UserID:           userID,
			PlanID:           planID,
			CreatedAt:        now,
			RemainingCredits: plan.Credits,
			Status:           model.SubscriptionStatusReserved,
		}

		if active == nil {
			newSub.Status = model.SubscriptionStatusActive
			newSub.StartAt = &now
			exp := now.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
			newSub.ExpiresAt = &exp
		} else if active.ExpiresAt != nil {
			sched := *active.ExpiresAt
			newSub.ScheduledStartAt = &sched
			exp := sched.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
			newSub.ExpiresAt = &exp
		}

		if err := u.subs.Save(ctx, tx, newSub); err != nil {
			return err
		}
		sub = newSub // Assign to the outer scope variable
		return nil
	})

	return sub, err
}

func (u *subscriptionUC) GetActive(ctx context.Context, userID string) (*model.UserSubscription, error) {
	defer logging.TraceDuration(u.log, "SubscriptionUC.GetActive")()
	return u.subs.FindActiveByUser(ctx, repository.NoTX, userID)
}

func (u *subscriptionUC) GetReserved(ctx context.Context, userID string) ([]*model.UserSubscription, error) {
	defer logging.TraceDuration(u.log, "SubscriptionUC.GetReserved")()
	return u.subs.FindReservedByUser(ctx, repository.NoTX, userID)
}

func (u *subscriptionUC) ListByUserID(ctx context.Context, userID string) ([]*model.UserSubscription, error) {
	defer logging.TraceDuration(u.log, "SubscriptionUC.ListByUserID")()
	return u.subs.ListByUserID(ctx, repository.NoTX, userID)
}

func (u *subscriptionUC) DeductCredits(ctx context.Context, userID string, amount int64) (*model.UserSubscription, error) {
	defer logging.TraceDuration(u.log, "SubscriptionUC.DeductCredits")()
	s, err := u.subs.FindActiveByUser(ctx, repository.NoTX, userID)
	if err != nil {
		// map repo not-found to a typed UC error
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNoActiveSubscription
		}
		return nil, err
	}
	if amount <= 0 {
		return s, nil
	}
	if s.RemainingCredits > 0 {
		s.RemainingCredits -= amount
		if s.RemainingCredits < 0 {
			s.RemainingCredits = 0
		}
	}
	// If credits exhausted, finish subscription now
	if s.RemainingCredits == 0 {
		now := time.Now()
		s.Status = model.SubscriptionStatusFinished
		s.ExpiresAt = &now
	}
	if err := u.subs.Save(ctx, repository.NoTX, s); err != nil {
		return nil, err
	}
	return s, nil
}

// FinishExpired transitions any active subscription whose expires_at <= now to finished.
// Returns number of subscriptions updated.
func (u *subscriptionUC) FinishExpired(ctx context.Context) (int, error) {
	defer logging.TraceDuration(u.log, "SubscriptionUC.FinishExpired")()
	expiring, err := u.subs.FindExpiring(ctx, repository.NoTX, 0)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, s := range expiring {
		if s.Status != model.SubscriptionStatusActive || s.ExpiresAt == nil || s.ExpiresAt.After(time.Now()) {
			continue
		}
		s.Status = model.SubscriptionStatusFinished
		if err := u.subs.Save(ctx, repository.NoTX, s); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (u *subscriptionUC) RedeemActivationCode(ctx context.Context, userID, code string) (*model.UserSubscription, error) {
	defer logging.TraceDuration(u.log, "SubscriptionUC.RedeemActivationCode")()
	var grantedSub *model.UserSubscription

	// The entire redemption process must be atomic
	err := u.tm.WithTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable}, func(ctx context.Context, tx repository.Tx) error {
		// 1. Find the code. The repository method already ensures it's unredeemed.
		ac, err := u.codes.FindByCode(ctx, tx, code)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return domain.ErrCodeNotFound // Use a more specific domain error
			}
			return err
		}

		// 2. Grant the subscription by calling our existing, trusted Subscribe method.
		// This correctly handles the logic for active vs. reserved plans.
		sub, err := u.Subscribe(ctx, userID, ac.PlanID)
		if err != nil {
			return err
		}

		// 3. Mark the code as redeemed to prevent reuse.
		now := time.Now()
		ac.IsRedeemed = true
		ac.RedeemedByUserID = &userID
		ac.RedeemedAt = &now
		if err := u.codes.Save(ctx, tx, ac); err != nil {
			return err
		}

		grantedSub = sub
		return nil
	})

	return grantedSub, err
}
