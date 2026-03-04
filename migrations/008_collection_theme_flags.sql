BEGIN;

ALTER TABLE archive.collections
  ADD COLUMN IF NOT EXISTS theme_id BIGINT,
  ADD COLUMN IF NOT EXISTS is_featured BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS is_pinned BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS pinned_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_archive_collections_theme ON archive.collections(theme_id);
CREATE INDEX IF NOT EXISTS idx_archive_collections_featured ON archive.collections(is_featured);
CREATE INDEX IF NOT EXISTS idx_archive_collections_pinned ON archive.collections(is_pinned);

ALTER TABLE archive.collections
  ADD CONSTRAINT fk_archive_collections_theme
  FOREIGN KEY (theme_id) REFERENCES taxonomy.themes(id) ON DELETE SET NULL;

COMMENT ON COLUMN archive.collections.theme_id IS '主题ID';
COMMENT ON COLUMN archive.collections.is_featured IS '是否推荐';
COMMENT ON COLUMN archive.collections.is_pinned IS '是否置顶';
COMMENT ON COLUMN archive.collections.pinned_at IS '置顶时间';

COMMIT;
