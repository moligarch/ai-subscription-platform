-- File: sql/000_init_schema.sql

-- 1) Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 2) users table
CREATE TABLE IF NOT EXISTS users (
  id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
  telegram_id     BIGINT      NOT NULL UNIQUE,
  username        TEXT,
  registered_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_active_at  TIMESTAMPTZ NULL
);

-- 3) subscription_plans table
CREATE TABLE IF NOT EXISTS subscription_plans (
  id            UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
  name          TEXT        NOT NULL UNIQUE,
  duration_days INTEGER     NOT NULL CHECK (duration_days > 0),
  credits       INTEGER     NOT NULL CHECK (credits >= 0),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 4) user_subscriptions table
CREATE TABLE IF NOT EXISTS user_subscriptions (
  id                 UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id            UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plan_id            UUID        NOT NULL REFERENCES subscription_plans(id) ON DELETE RESTRICT,
  start_at           TIMESTAMPTZ NOT NULL,
  expires_at         TIMESTAMPTZ NOT NULL,
  remaining_credits  INTEGER     NOT NULL CHECK (remaining_credits >= 0),
  is_active          BOOLEAN     NOT NULL DEFAULT true,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS payments (
  id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  amount     DOUBLE PRECISION NOT NULL,          -- amount in Toman
  method     TEXT        NOT NULL,               -- e.g. "mellat", "zarinpal", or a planID alias in demo mode
  status     TEXT        NOT NULL,               -- store your domain.PaymentStatus as TEXT
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- helpful index for period totals
CREATE INDEX IF NOT EXISTS idx_payments_created_at ON payments (created_at);

-- 5) Partial unique index: one active subscription per user+plan
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_plan_active
  ON user_subscriptions(user_id, plan_id)
  WHERE is_active;

-- 6) Indexes for lookup performance
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_user_active
  ON user_subscriptions(user_id)
  WHERE is_active;

CREATE INDEX IF NOT EXISTS idx_user_subscriptions_expires_at
  ON user_subscriptions(expires_at);
