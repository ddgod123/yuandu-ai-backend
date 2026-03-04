BEGIN;

CREATE TABLE IF NOT EXISTS audit.home_daily_stats (
  stat_date DATE PRIMARY KEY,
  total_collections BIGINT NOT NULL DEFAULT 0,
  total_emojis BIGINT NOT NULL DEFAULT 0,
  today_new_emojis BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_home_daily_stats_updated_at
  ON audit.home_daily_stats (updated_at DESC);

COMMIT;
