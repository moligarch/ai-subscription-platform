// File: internal/usecase/subscription_uc.go
package usecase

import (
	"context"
	"errors"
	"hash/fnv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"

	"github.com/jackc/pgx/v4"
)

// SubscriptionUseCase implements subscription business operations.
type SubscriptionUseCase struct {
	planRepo repository.SubscriptionPlanRepository
	subRepo  repository.SubscriptionRepository
	pool     *pgxpool.Pool
}

// NewSubscriptionUseCase constructs usecase.
// Backwards-compatible: callers can provide pool as third argument (variadic-like).
func NewSubscriptionUseCase(planRepo repository.SubscriptionPlanRepository, subRepo repository.SubscriptionRepository, poolOpt ...*pgxpool.Pool) *SubscriptionUseCase {
	var p *pgxpool.Pool
	if len(poolOpt) > 0 {
		p = poolOpt[0]
	}
	return &SubscriptionUseCase{planRepo: planRepo, subRepo: subRepo, pool: p}
}

func hashToInt64(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return int64(h.Sum64() & ((1 << 63) - 1))
}

// Subscribe implements Subscribe as in interface.
// Rules:
// - A user can have up to two subscriptions: one active and one reserved.
// - If no active subscription -> new subscription becomes active (StartAt = now).
// - If active exists and no reserved -> new subscription becomes reserved (ScheduledStartAt set to active.ExpiresAt).
// - If both active+reserved exist -> returns error.
func (uc *SubscriptionUseCase) Subscribe(ctx context.Context, userID, planID string) (*model.UserSubscription, error) {
	plan, err := uc.planRepo.FindByID(ctx, planID)
	if err != nil {
		return nil, err
	}

	// If we have a DB pool we prefer TX + advisory lock to avoid races.
	if uc.pool != nil {
		conn, err := uc.pool.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		defer conn.Release()

		tx, err := conn.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)

		// acquire advisory xact lock per user
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", hashToInt64(userID)); err != nil {
			return nil, err
		}

		// if there is an existing active subscription for same plan -> extend it
		if same, _ := uc.subRepo.FindActiveByUserAndPlanTx(ctx, tx, userID, planID); same != nil {
			// extend expiration and credits
			if same.StartAt == nil {
				now := time.Now()
				same.StartAt = &now
			}
			if same.ExpiresAt == nil {
				ex := time.Now().Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
				same.ExpiresAt = &ex
			} else {
				*same.ExpiresAt = same.ExpiresAt.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
			}
			same.RemainingCredits += plan.Credits
			same.Status = model.SubscriptionStatusActive
			if err := uc.subRepo.SaveTx(ctx, tx, same); err != nil {
				return nil, err
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return same, nil
		}

		active, _ := uc.subRepo.FindActiveByUserTx(ctx, tx, userID)
		reserved, _ := uc.subRepo.FindReservedByUserTx(ctx, tx, userID)

		now := time.Now()
		if active == nil {
			// create active
			start := now
			ex := start.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
			s := &model.UserSubscription{
				ID:               uuid.NewString(),
				UserID:           userID,
				PlanID:           planID,
				CreatedAt:        now,
				ScheduledStartAt: nil,
				StartAt:          &start,
				ExpiresAt:        &ex,
				RemainingCredits: plan.Credits,
				Status:           model.SubscriptionStatusActive,
			}
			if err := uc.subRepo.SaveTx(ctx, tx, s); err != nil {
				return nil, err
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return s, nil
		}

		if len(reserved) == 0 {
			// schedule after latest active expiry
			latest := time.Now()
			if active.ExpiresAt != nil {
				latest = *active.ExpiresAt
			}
			scheduled := latest
			ex := scheduled.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
			s := &model.UserSubscription{
				ID:               uuid.NewString(),
				UserID:           userID,
				PlanID:           planID,
				CreatedAt:        now,
				ScheduledStartAt: &scheduled,
				StartAt:          nil,
				ExpiresAt:        &ex,
				RemainingCredits: plan.Credits,
				Status:           model.SubscriptionStatusReserved,
			}
			if err := uc.subRepo.SaveTx(ctx, tx, s); err != nil {
				return nil, err
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return s, nil
		}

		return nil, errors.New("user already has active and reserved subscriptions")
	}

	// Non-DB-pool path (tests / in-memory repo)
	active, _ := uc.subRepo.FindActiveByUser(ctx, userID)
	reserved, _ := uc.subRepo.FindReservedByUser(ctx, userID)
	now := time.Now()
	if active == nil {
		start := now
		ex := start.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
		s := &model.UserSubscription{
			ID:               uuid.NewString(),
			UserID:           userID,
			PlanID:           planID,
			CreatedAt:        now,
			ScheduledStartAt: nil,
			StartAt:          &start,
			ExpiresAt:        &ex,
			RemainingCredits: plan.Credits,
			Status:           model.SubscriptionStatusActive,
		}
		if err := uc.subRepo.Save(ctx, s); err != nil {
			return nil, err
		}
		return s, nil
	}
	if len(reserved) == 0 {
		latest := time.Now()
		if active.ExpiresAt != nil {
			latest = *active.ExpiresAt
		}
		scheduled := latest
		ex := scheduled.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
		s := &model.UserSubscription{
			ID:               uuid.NewString(),
			UserID:           userID,
			PlanID:           planID,
			CreatedAt:        now,
			ScheduledStartAt: &scheduled,
			StartAt:          nil,
			ExpiresAt:        &ex,
			RemainingCredits: plan.Credits,
			Status:           model.SubscriptionStatusReserved,
		}
		if err := uc.subRepo.Save(ctx, s); err != nil {
			return nil, err
		}
		return s, nil
	}
	return nil, errors.New("user already has active and reserved subscriptions")
}

// GetActiveSubscription returns user's active subscription (nil + ErrNotFound if none)
func (uc *SubscriptionUseCase) GetActiveSubscription(ctx context.Context, userID string) (*model.UserSubscription, error) {
	return uc.subRepo.FindActiveByUser(ctx, userID)
}

// DeductCredits decreases credits by amount on the provided subscription object.
// If credits drop to <=0 or expiration passed, set status to Finished and promote reserved.
func (uc *SubscriptionUseCase) DeductCredits(ctx context.Context, sub *model.UserSubscription, amount int) (*model.UserSubscription, error) {
	if sub == nil {
		return nil, errors.New("subscription is nil")
	}

	// Fast path for in-memory repos (no pool)
	if uc.pool == nil {
		now := time.Now()
		if sub.StartAt == nil || sub.ExpiresAt == nil || now.After(*sub.ExpiresAt) {
			// expired -> finish and promote
			sub.Status = model.SubscriptionStatusFinished
			if err := uc.subRepo.Save(ctx, sub); err != nil {
				return nil, err
			}

			return uc.promoteNextReservedNonTx(ctx, sub.UserID)
		}

		if sub.Status != model.SubscriptionStatusActive {
			return nil, errors.New("subscription not active")
		}
		if sub.RemainingCredits-amount < 0 {
			return nil, errors.New("insufficient credits")
		}
		sub.RemainingCredits -= amount
		if sub.RemainingCredits <= 0 {
			sub.Status = model.SubscriptionStatusFinished
		}
		if err := uc.subRepo.Save(ctx, sub); err != nil {
			return nil, err
		}
		if sub.Status == model.SubscriptionStatusFinished {
			_, _ = uc.promoteNextReservedNonTx(ctx, sub.UserID)
		}
		return sub, nil
	}

	// DB-backed path with tx and advisory lock
	conn, err := uc.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// re-fetch inside tx to ensure up-to-date
	dbSub, err := uc.subRepo.FindByIDTx(ctx, tx, sub.ID)
	if err != nil {
		return nil, err
	}

	// lock per user
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", hashToInt64(dbSub.UserID)); err != nil {
		return nil, err
	}

	now := time.Now()
	if dbSub.StartAt == nil || dbSub.ExpiresAt == nil || now.After(*dbSub.ExpiresAt) {
		dbSub.Status = model.SubscriptionStatusFinished
		if err := uc.subRepo.SaveTx(ctx, tx, dbSub); err != nil {
			return nil, err
		}
		if _, err := uc.promoteNextReservedInTx(ctx, tx, dbSub.UserID); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return dbSub, nil
	}

	if dbSub.Status != model.SubscriptionStatusActive {
		return nil, errors.New("subscription not active")
	}

	dbSub.RemainingCredits -= amount
	if dbSub.RemainingCredits <= 0 {
		dbSub.Status = model.SubscriptionStatusFinished
	}
	if err := uc.subRepo.SaveTx(ctx, tx, dbSub); err != nil {
		return nil, err
	}
	if dbSub.Status == model.SubscriptionStatusFinished {
		if _, err := uc.promoteNextReservedInTx(ctx, tx, dbSub.UserID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return dbSub, nil
}

// CountActiveSubscriptionsByPlan delegates to repo.
func (uc *SubscriptionUseCase) CountActiveSubscriptionsByPlan(ctx context.Context) (map[string]int, error) {
	return uc.subRepo.CountActiveByPlan(ctx)
}

// TotalRemainingCredits delegates to repo.
func (uc *SubscriptionUseCase) TotalRemainingCredits(ctx context.Context) (int, error) {
	return uc.subRepo.TotalRemainingCredits(ctx)
}

// --- helpers for promotion (non-TX and TX variants) ---

func (uc *SubscriptionUseCase) promoteNextReservedNonTx(ctx context.Context, userID string) (*model.UserSubscription, error) {
	reserved, err := uc.subRepo.FindReservedByUser(ctx, userID)
	if err != nil || len(reserved) == 0 {
		return nil, nil
	}
	// pick earliest scheduled
	var candidate *model.UserSubscription
	for _, r := range reserved {
		if candidate == nil {
			candidate = r
			continue
		}
		if r.ScheduledStartAt != nil && candidate.ScheduledStartAt != nil {
			if r.ScheduledStartAt.Before(*candidate.ScheduledStartAt) {
				candidate = r
			}
		}
	}
	if candidate == nil {
		return nil, nil
	}
	now := time.Now()
	candidate.StartAt = &now
	plan, err := uc.planRepo.FindByID(ctx, candidate.PlanID)
	if err != nil {
		return nil, err
	}
	ex := now.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
	candidate.ExpiresAt = &ex
	candidate.Status = model.SubscriptionStatusActive
	if err := uc.subRepo.Save(ctx, candidate); err != nil {
		return nil, err
	}
	return candidate, nil
}

func (uc *SubscriptionUseCase) promoteNextReservedInTx(ctx context.Context, tx pgx.Tx, userID string) (*model.UserSubscription, error) {
	reserved, err := uc.subRepo.FindReservedByUserTx(ctx, tx, userID)
	if err != nil || len(reserved) == 0 {
		return nil, nil
	}
	var candidate *model.UserSubscription
	for _, r := range reserved {
		if candidate == nil {
			candidate = r
			continue
		}
		if r.ScheduledStartAt != nil && candidate.ScheduledStartAt != nil {
			if r.ScheduledStartAt.Before(*candidate.ScheduledStartAt) {
				candidate = r
			}
		}
	}
	if candidate == nil {
		return nil, nil
	}
	now := time.Now()
	candidate.StartAt = &now
	plan, err := uc.planRepo.FindByID(ctx, candidate.PlanID)
	if err != nil {
		return nil, err
	}
	ex := now.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
	candidate.ExpiresAt = &ex
	candidate.Status = model.SubscriptionStatusActive
	if err := uc.subRepo.SaveTx(ctx, tx, candidate); err != nil {
		return nil, err
	}
	return candidate, nil
}

// FinishExpiredSubscription marks a subscription as finished (if due) and promotes
// the next reserved subscription for the user. It is safe to call concurrently:
// when DB-backed, it uses an advisory xact lock per user.
func (uc *SubscriptionUseCase) FinishExpiredSubscription(ctx context.Context, subID string) error {
	if uc.pool == nil {
		// Non-DB (in-memory) flow:
		sub, err := uc.subRepo.FindByID(ctx, subID)
		if err != nil {
			return err
		}
		// If already finished - nothing to do
		if sub.Status == model.SubscriptionStatusFinished {
			return nil
		}

		// mark finished if now is past expires or explicit
		now := time.Now()
		if sub.ExpiresAt == nil || now.After(*sub.ExpiresAt) || sub.RemainingCredits <= 0 {
			sub.Status = model.SubscriptionStatusFinished
			// ensure ExpiresAt set
			if sub.ExpiresAt == nil {
				sub.ExpiresAt = &now
			}
			if err := uc.subRepo.Save(ctx, sub); err != nil {
				return err
			}
			// promote reserved (non-TX)
			_, err = uc.promoteNextReservedNonTx(ctx, sub.UserID)
			return err
		}

		// Not expired yet -> nothing to do
		return nil
	}

	// DB-backed path: use connection, tx, and advisory lock on user
	conn, err := uc.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// fetch subscription inside tx
	dbSub, err := uc.subRepo.FindByIDTx(ctx, tx, subID)
	if err != nil {
		return err
	}

	// if already finished, nothing to do
	if dbSub.Status == model.SubscriptionStatusFinished {
		return tx.Commit(ctx)
	}

	// lock by user id (advisory lock)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", hashToInt64(dbSub.UserID)); err != nil {
		return err
	}

	now := time.Now()
	// If expiresAt is nil or already passed OR remaining credits <= 0, mark finished
	shouldFinish := false
	if dbSub.ExpiresAt == nil {
		shouldFinish = true
	} else if now.After(*dbSub.ExpiresAt) {
		shouldFinish = true
	}
	if dbSub.RemainingCredits <= 0 {
		shouldFinish = true
	}

	if !shouldFinish {
		// nothing to do
		return tx.Commit(ctx)
	}

	// mark finished and set ExpiresAt if missing
	dbSub.Status = model.SubscriptionStatusFinished
	if dbSub.ExpiresAt == nil {
		dbSub.ExpiresAt = &now
	}
	if err := uc.subRepo.SaveTx(ctx, tx, dbSub); err != nil {
		return err
	}

	// promote next reserved (in-tx)
	if _, err := uc.promoteNextReservedInTx(ctx, tx, dbSub.UserID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
