CREATE SCHEMA IF NOT EXISTS action;

CREATE TABLE IF NOT EXISTS action.user_behavior_events (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NULL REFERENCES "user".users(id) ON DELETE SET NULL,
  device_id VARCHAR(128) NOT NULL DEFAULT '',
  session_id VARCHAR(128) NOT NULL DEFAULT '',
  event_name VARCHAR(64) NOT NULL,
  route VARCHAR(512) NOT NULL DEFAULT '',
  referrer VARCHAR(512) NOT NULL DEFAULT '',
  collection_id BIGINT NULL REFERENCES archive.collections(id) ON DELETE SET NULL,
  emoji_id BIGINT NULL REFERENCES archive.emojis(id) ON DELETE SET NULL,
  ip_id BIGINT NULL REFERENCES taxonomy.ips(id) ON DELETE SET NULL,
  subscription_status VARCHAR(32) NOT NULL DEFAULT '',
  success BOOLEAN NULL,
  error_code VARCHAR(64) NOT NULL DEFAULT '',
  request_id VARCHAR(128) NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_behavior_events_event_name_created_at
  ON action.user_behavior_events(event_name, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_behavior_events_user_id_created_at
  ON action.user_behavior_events(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_behavior_events_collection_id_created_at
  ON action.user_behavior_events(collection_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_behavior_events_emoji_id_created_at
  ON action.user_behavior_events(emoji_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_behavior_events_route_created_at
  ON action.user_behavior_events(route, created_at DESC);

