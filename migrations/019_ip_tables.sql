BEGIN;

CREATE TABLE IF NOT EXISTS taxonomy.ips (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  slug VARCHAR(128) NOT NULL UNIQUE,
  cover_url VARCHAR(512),
  category_id BIGINT,
  description TEXT,
  sort INTEGER NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_taxonomy_ips_category ON taxonomy.ips(category_id);
CREATE INDEX IF NOT EXISTS idx_taxonomy_ips_status ON taxonomy.ips(status);
CREATE INDEX IF NOT EXISTS idx_taxonomy_ips_sort ON taxonomy.ips(sort);

ALTER TABLE archive.collections
  ADD COLUMN IF NOT EXISTS ip_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_archive_collections_ip
  ON archive.collections(ip_id);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_archive_collections_ip'
  ) THEN
    ALTER TABLE archive.collections
      ADD CONSTRAINT fk_archive_collections_ip
      FOREIGN KEY (ip_id) REFERENCES taxonomy.ips(id) ON DELETE SET NULL;
  END IF;
END $$;

COMMIT;
