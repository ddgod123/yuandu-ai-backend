BEGIN;

ALTER TABLE IF EXISTS archive.collections
  ADD COLUMN IF NOT EXISTS latest_zip_key VARCHAR(512),
  ADD COLUMN IF NOT EXISTS latest_zip_name VARCHAR(255),
  ADD COLUMN IF NOT EXISTS latest_zip_size BIGINT,
  ADD COLUMN IF NOT EXISTS latest_zip_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS archive.emojis
  ADD COLUMN IF NOT EXISTS display_order INT;

CREATE INDEX IF NOT EXISTS idx_archive_emojis_collection_order ON archive.emojis(collection_id, display_order);

COMMIT;
