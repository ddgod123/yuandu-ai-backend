BEGIN;

ALTER TABLE IF EXISTS public.users
  ADD COLUMN IF NOT EXISTS subscription_status VARCHAR(32) DEFAULT 'inactive',
  ADD COLUMN IF NOT EXISTS subscription_plan VARCHAR(32),
  ADD COLUMN IF NOT EXISTS subscription_expires_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS "user".users
  ADD COLUMN IF NOT EXISTS subscription_status VARCHAR(32) DEFAULT 'inactive',
  ADD COLUMN IF NOT EXISTS subscription_plan VARCHAR(32),
  ADD COLUMN IF NOT EXISTS subscription_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_user_users_subscription_status ON "user".users(subscription_status);
CREATE INDEX IF NOT EXISTS idx_user_users_subscription_expires ON "user".users(subscription_expires_at);

COMMIT;
