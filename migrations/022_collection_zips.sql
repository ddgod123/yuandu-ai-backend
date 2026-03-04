BEGIN;

CREATE TABLE IF NOT EXISTS archive.collection_zips (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL REFERENCES archive.collections(id) ON DELETE CASCADE,
  zip_key VARCHAR(512) NOT NULL,
  zip_name VARCHAR(255) NOT NULL,
  size_bytes BIGINT DEFAULT 0,
  uploaded_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_collection_zips_unique
  ON archive.collection_zips (collection_id, zip_key);

CREATE INDEX IF NOT EXISTS idx_collection_zips_collection
  ON archive.collection_zips (collection_id);

CREATE INDEX IF NOT EXISTS idx_collection_zips_uploaded
  ON archive.collection_zips (uploaded_at);

COMMIT;
