BEGIN;

ALTER TABLE archive.collections
  ADD COLUMN IF NOT EXISTS owner_deleted_at TIMESTAMPTZ;

COMMENT ON COLUMN archive.collections.owner_deleted_at IS '用户侧删除时间（软删除，仅对用户侧隐藏，运营后台可见）';

CREATE INDEX IF NOT EXISTS idx_collections_ugc_owner_deleted_at
  ON archive.collections(owner_id, owner_deleted_at DESC)
  WHERE source = 'ugc_upload';

COMMIT;

