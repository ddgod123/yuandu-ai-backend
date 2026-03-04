BEGIN;

ALTER TABLE archive.collections
  ADD COLUMN IF NOT EXISTS slug VARCHAR(128),
  ADD COLUMN IF NOT EXISTS category_id BIGINT,
  ADD COLUMN IF NOT EXISTS source VARCHAR(64) DEFAULT 'manual_zip',
  ADD COLUMN IF NOT EXISTS qiniu_prefix VARCHAR(512),
  ADD COLUMN IF NOT EXISTS file_count INTEGER DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS idx_archive_collections_slug ON archive.collections(slug);
CREATE INDEX IF NOT EXISTS idx_archive_collections_category ON archive.collections(category_id);

ALTER TABLE archive.collections
  ADD CONSTRAINT fk_archive_collections_category
  FOREIGN KEY (category_id) REFERENCES taxonomy.categories(id) ON DELETE SET NULL;

COMMENT ON COLUMN archive.collections.slug IS '合集标识';
COMMENT ON COLUMN archive.collections.category_id IS '分类ID';
COMMENT ON COLUMN archive.collections.source IS '来源';
COMMENT ON COLUMN archive.collections.qiniu_prefix IS '七牛对象前缀';
COMMENT ON COLUMN archive.collections.file_count IS '文件数量';

COMMIT;
