BEGIN;

ALTER TABLE archive.collections
    ADD COLUMN IF NOT EXISTS is_sample BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_archive_collections_is_sample ON archive.collections (is_sample);

COMMENT ON COLUMN archive.collections.is_sample IS '是否样本合集（用于样本筛选与训练样本池）';

COMMIT;
