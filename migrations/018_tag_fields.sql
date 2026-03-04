BEGIN;

ALTER TABLE taxonomy.tags
  ADD COLUMN IF NOT EXISTS sort INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS status VARCHAR(32) NOT NULL DEFAULT 'active';

CREATE INDEX IF NOT EXISTS idx_taxonomy_tags_sort ON taxonomy.tags(sort);
CREATE INDEX IF NOT EXISTS idx_taxonomy_tags_status ON taxonomy.tags(status);

COMMENT ON COLUMN taxonomy.tags.sort IS '排序';
COMMENT ON COLUMN taxonomy.tags.status IS '状态';

COMMIT;
