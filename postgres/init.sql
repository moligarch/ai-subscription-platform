-- extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- users table
CREATE TABLE IF NOT EXISTS users (
  id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
  telegram_id     BIGINT      NOT NULL UNIQUE,
  username        TEXT,
  registered_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_active_at  TIMESTAMPTZ NULL
);

-- subscription_plans table
CREATE TABLE IF NOT EXISTS subscription_plans (
  id            UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
  name          TEXT        NOT NULL UNIQUE,
  duration_days INTEGER     NOT NULL CHECK (duration_days > 0),
  credits       INTEGER     NOT NULL CHECK (credits >= 0),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- user_subscriptions table
-- status: 'reserved' | 'active' | 'finished' | 'cancelled'
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'subscription_status') THEN
    CREATE TYPE subscription_status AS ENUM ('reserved', 'active', 'finished', 'cancelled');
  END IF;
END$$;

CREATE TABLE IF NOT EXISTS user_subscriptions (
  id                  UUID                 PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id             UUID                 NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plan_id             UUID                 NOT NULL REFERENCES subscription_plans(id) ON DELETE RESTRICT,
  -- when subscription was created (recording time)
  created_at          TIMESTAMPTZ          NOT NULL DEFAULT now(),
  -- scheduled_start_at: when a reserved subscription is scheduled to start (NULL for immediate/active)
  scheduled_start_at  TIMESTAMPTZ          NULL,
  -- start_at: when subscription actually started (NULL until active)
  start_at            TIMESTAMPTZ          NULL,
  -- expires_at: calculated base on start_at + plan.duration_days when the subscription is active or reserved
  expires_at          TIMESTAMPTZ          NULL,
  -- remaining credits (>=0)
  remaining_credits   INTEGER              NOT NULL DEFAULT 0 CHECK (remaining_credits >= 0),
  -- status enum
  status              subscription_status  NOT NULL DEFAULT 'reserved'
);

-- payments
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    plan_id UUID NOT NULL,
    provider TEXT NOT NULL,
    amount BIGINT NOT NULL,
    currency TEXT NOT NULL,
    authority TEXT,
    ref_id TEXT,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ,
    callback TEXT,
    description TEXT,
    meta JSONB,
    subscription_id UUID
);

-- purchase history (append-only)
CREATE TABLE IF NOT EXISTS purchases (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    plan_id UUID NOT NULL,
    payment_id UUID NOT NULL REFERENCES payments(id) ON DELETE RESTRICT,
    subscription_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

-- Indexes and constraints
-- Ensure only one active subscription per (user, plan) — permitted multiple active plans across different plan_id
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_plan_active ON user_subscriptions(user_id, plan_id)
  WHERE status = 'active';

-- To quickly find a user's active subscription(s) and reserved
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_user_status ON user_subscriptions(user_id, status);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_expires_at ON user_subscriptions(expires_at);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_scheduled_start ON user_subscriptions(scheduled_start_at);

CREATE INDEX IF NOT EXISTS idx_payments_user ON payments (user_id);
CREATE INDEX IF NOT EXISTS idx_payments_plan ON payments (plan_id);
CREATE INDEX IF NOT EXISTS idx_payments_authority ON payments (authority);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments (status);

CREATE INDEX IF NOT EXISTS idx_purchases_user ON purchases (user_id);
CREATE INDEX IF NOT EXISTS idx_purchases_plan ON purchases (plan_id);
CREATE INDEX IF NOT EXISTS idx_purchases_payment ON purchases (payment_id);

-- Convenience view for stats
CREATE MATERIALIZED VIEW IF NOT EXISTS v_active_subscriptions_by_plan AS
SELECT sp.id AS plan_id, sp.name AS plan_name, COUNT(us.*) AS active_count
FROM subscription_plans sp
LEFT JOIN user_subscriptions us
  ON us.plan_id = sp.id
 AND us.status IN ('active','reserved') -- reserved counts as active for stats per requirement
GROUP BY sp.id, sp.name;

