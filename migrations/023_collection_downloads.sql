BEGIN;

CREATE TABLE IF NOT EXISTS action.collection_downloads (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL REFERENCES archive.collections(id) ON DELETE CASCADE,
  user_id BIGINT,
  ip VARCHAR(64),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_collection_downloads_collection
  ON action.collection_downloads (collection_id);

CREATE INDEX IF NOT EXISTS idx_collection_downloads_user
  ON action.collection_downloads (user_id);

CREATE INDEX IF NOT EXISTS idx_collection_downloads_created
  ON action.collection_downloads (created_at);

COMMIT;
