-- Extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================
-- ENUM TYPES
-- =============================================================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'subscription_status') THEN
    CREATE TYPE subscription_status AS ENUM ('reserved','active','finished','cancelled');
  END IF;
  
  -- Add a new type for user registration status
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_registration_status') THEN
    CREATE TYPE user_registration_status AS ENUM ('pending', 'completed');
  END IF;
END$$;


-- =============================================================
-- USERS
-- =============================================================
-- Privacy and admin flags are included to support v1 features and future panel.
CREATE TABLE IF NOT EXISTS users (
  id                      UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
  telegram_id             BIGINT       NOT NULL UNIQUE,
  username                TEXT,
  full_name               TEXT,
  phone_number            TEXT,
  registration_status     user_registration_status NOT NULL DEFAULT 'pending', -- NEW
  registered_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  last_active_at          TIMESTAMPTZ  NULL,
  -- Privacy
  allow_message_storage   BOOLEAN      NOT NULL DEFAULT TRUE,
  auto_delete_messages    BOOLEAN      NOT NULL DEFAULT FALSE,
  message_retention_days  INTEGER      NOT NULL DEFAULT 0,
  data_encrypted          BOOLEAN      NOT NULL DEFAULT TRUE,
  -- Admin flag (optional convenience in addition to config-based list)
  is_admin                BOOLEAN      NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_users_last_active ON users(last_active_at);

-- =============================================================
-- SUBSCRIPTION PLANS
-- =============================================================
-- price_irr is defaulted to 0 initially to avoid breaking older code paths
-- until repositories are updated to set a proper price.

CREATE TABLE IF NOT EXISTS subscription_plans (
  id             UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
  name           TEXT         NOT NULL UNIQUE,
  duration_days  INTEGER      NOT NULL CHECK (duration_days > 0),
  credits        BIGINT       NOT NULL DEFAULT 0 CHECK (credits >= 0),
  price_irr      BIGINT       NOT NULL DEFAULT 0 CHECK (price_irr >= 0),
  created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- =============================================================
-- USER SUBSCRIPTIONS
-- =============================================================
CREATE TABLE IF NOT EXISTS user_subscriptions (
  id                   UUID                 PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id              UUID                 NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plan_id              UUID                 NOT NULL REFERENCES subscription_plans(id) ON DELETE RESTRICT,
  created_at           TIMESTAMPTZ          NOT NULL DEFAULT NOW(),
  scheduled_start_at   TIMESTAMPTZ          NULL,
  start_at             TIMESTAMPTZ          NULL,
  expires_at           TIMESTAMPTZ          NULL,
  remaining_credits    BIGINT               NOT NULL DEFAULT 0 CHECK (remaining_credits >= 0),
  status               subscription_status  NOT NULL DEFAULT 'reserved'
);

-- Only one ACTIVE subscription per (user, plan)
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_plan_active
  ON user_subscriptions(user_id, plan_id)
  WHERE status = 'active';

  -- At most one RESERVED subscription per user
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_reserved_once
  ON user_subscriptions(user_id)
  WHERE status = 'reserved';


-- Fast lookups
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_user_status
  ON user_subscriptions(user_id, status);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_expires_at
  ON user_subscriptions(expires_at);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_scheduled_start
  ON user_subscriptions(scheduled_start_at);

-- =============================================================
-- MODEL PRICING
-- =============================================================
CREATE TABLE IF NOT EXISTS model_pricing (
  id                         UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
  model_name                 TEXT         UNIQUE NOT NULL,    -- e.g., 'gpt-4o-mini'
  input_token_price_micros   BIGINT       NOT NULL,           -- price per input token (micro-credits)
  output_token_price_micros  BIGINT       NOT NULL,           -- price per output token (micro-credits)
  active                     BOOLEAN      NOT NULL DEFAULT TRUE,
  created_at                 TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at                 TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- =============================================================
-- PAYMENTS
-- =============================================================
CREATE TABLE IF NOT EXISTS payments (
  id                       UUID         PRIMARY KEY,
  user_id                  UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plan_id                  UUID         NOT NULL REFERENCES subscription_plans(id) ON DELETE RESTRICT,
  provider                 TEXT         NOT NULL,
  amount                   BIGINT       NOT NULL,
  currency                 TEXT         NOT NULL,
  authority                TEXT,
  ref_id                   TEXT,
  status                   TEXT         NOT NULL,
  created_at               TIMESTAMPTZ  NOT NULL,
  updated_at               TIMESTAMPTZ  NOT NULL,
  paid_at                  TIMESTAMPTZ,
  callback                 TEXT,
  description              TEXT,
  meta                     JSONB,
  subscription_id          UUID,
  -- activation-code flow support for post-payment manual activation
  activation_code          TEXT,
  activation_expires_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_payments_user      ON payments(user_id);
CREATE INDEX IF NOT EXISTS idx_payments_plan      ON payments(plan_id);
CREATE INDEX IF NOT EXISTS idx_payments_authority ON payments(authority);
CREATE INDEX IF NOT EXISTS idx_payments_status    ON payments(status);

-- =============================================================
-- PURCHASE HISTORY (append-only)
-- =============================================================
CREATE TABLE IF NOT EXISTS purchases (
  id               UUID         PRIMARY KEY,
  user_id          UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plan_id          UUID         NOT NULL REFERENCES subscription_plans(id) ON DELETE RESTRICT,
  payment_id       UUID         NOT NULL REFERENCES payments(id) ON DELETE RESTRICT,
  subscription_id  UUID         NOT NULL,
  created_at       TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_purchases_user    ON purchases(user_id);
CREATE INDEX IF NOT EXISTS idx_purchases_plan    ON purchases(plan_id);
CREATE INDEX IF NOT EXISTS idx_purchases_payment ON purchases(payment_id);

-- Adds a strict uniqueness constraint so each payment can result in at most one purchase row.
-- Safe to run multiple times: uses IF NOT EXISTS semantics via DO block.


DO $$
  BEGIN
    IF NOT EXISTS (
      SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_type = 'UNIQUE'
          AND table_name = 'purchases'
          AND constraint_name = 'uq_purchases_payment') THEN
      ALTER TABLE purchases ADD CONSTRAINT uq_purchases_payment UNIQUE (payment_id);
  END IF;
END$$;


-- =============================================================
-- CHAT SESSIONS + MESSAGES
-- =============================================================
CREATE TABLE IF NOT EXISTS chat_sessions (
  id          UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  model       TEXT,
  status      TEXT         NOT NULL DEFAULT 'active' CHECK (status IN ('active','finished')),
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- At most one active chat per user (business rule)
CREATE UNIQUE INDEX IF NOT EXISTS uq_active_chat_by_user
  ON chat_sessions(user_id)
  WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_chat_sessions_user   ON chat_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_chat_sessions_status ON chat_sessions(status);

CREATE TABLE IF NOT EXISTS chat_messages (
  id          UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
  session_id  UUID         NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  role        VARCHAR(20)  NOT NULL CHECK (role IN ('user','assistant','system')),
  content     TEXT         NOT NULL,
  tokens      INTEGER      NOT NULL DEFAULT 0,
  encrypted   BOOLEAN      NOT NULL DEFAULT FALSE,
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_session_id ON chat_messages(session_id);
CREATE INDEX IF NOT EXISTS idx_chat_messages_created_at ON chat_messages(created_at);

-- =============================================================
-- AI PROCESSING JOBS (OUTBOX PATTERN)
-- =============================================================
CREATE TABLE IF NOT EXISTS ai_jobs (
  id                   UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
  status               TEXT         NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
  session_id           UUID         NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  user_message_id      UUID         NULL REFERENCES chat_messages(id) ON DELETE CASCADE, -- This column is now NULLABLE
  retries              INTEGER      NOT NULL DEFAULT 0,
  last_error           TEXT,
  created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_jobs_status_created ON ai_jobs(status, created_at);

-- =============================================================
-- VIEWS (STATS)
-- =============================================================
-- Active = active + reserved (for business reporting)
CREATE MATERIALIZED VIEW IF NOT EXISTS v_active_subscriptions_by_plan AS
SELECT sp.id AS plan_id,
       sp.name AS plan_name,
       COUNT(us.*) AS active_count
  FROM subscription_plans sp
  LEFT JOIN user_subscriptions us
    ON us.plan_id = sp.id
   AND us.status IN ('active','reserved')
 GROUP BY sp.id, sp.name;

-- Optional helper to refresh
-- REFRESH MATERIALIZED VIEW CONCURRENTLY v_active_subscriptions_by_plan;


-- =============================================================
-- VIEWS (STATS)
-- =============================================================
-- notification log: further analysis
CREATE TABLE IF NOT EXISTS subscription_notifications (
  id               UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
  subscription_id  UUID         NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
  user_id          UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind             TEXT         NOT NULL CHECK (kind IN ('expiry')),
  threshold_days   INTEGER      NOT NULL CHECK (threshold_days >= 0),
  sent_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE (subscription_id, kind, threshold_days)
);

CREATE INDEX IF NOT EXISTS idx_subnotif_user ON subscription_notifications(user_id);