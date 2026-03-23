-- 078_asset_domain_split_scaffold.sql
-- 目标：为三类资产域建立独立分表骨架（运营上传 / 视频生成 / 用户上传）
-- 说明：本迁移先做“结构隔离”与“存储策略配置”落地，不改变既有读写路径。

CREATE SCHEMA IF NOT EXISTS admin_asset;
CREATE SCHEMA IF NOT EXISTS video_asset;
CREATE SCHEMA IF NOT EXISTS ugc_asset;

-- 统一存储策略表：可用于后台做“按域管理 bucket/prefix”
CREATE TABLE IF NOT EXISTS ops.asset_domain_storage_policies (
  id BIGSERIAL PRIMARY KEY,
  domain VARCHAR(32) NOT NULL UNIQUE,
  provider VARCHAR(32) NOT NULL DEFAULT 'qiniu',
  bucket VARCHAR(128) NOT NULL DEFAULT '',
  key_prefix VARCHAR(256) NOT NULL DEFAULT '',
  is_private BOOLEAN NOT NULL DEFAULT FALSE,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  notes TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE ops.asset_domain_storage_policies IS '资产域存储策略（admin/video/ugc）';
COMMENT ON COLUMN ops.asset_domain_storage_policies.domain IS '资产域：admin|video|ugc';
COMMENT ON COLUMN ops.asset_domain_storage_policies.provider IS '存储提供方（qiniu/minio/oss）';
COMMENT ON COLUMN ops.asset_domain_storage_policies.bucket IS '对象存储 bucket';
COMMENT ON COLUMN ops.asset_domain_storage_policies.key_prefix IS '对象 key 前缀';
COMMENT ON COLUMN ops.asset_domain_storage_policies.is_private IS '是否私有存储';
COMMENT ON COLUMN ops.asset_domain_storage_policies.enabled IS '是否启用';

INSERT INTO ops.asset_domain_storage_policies(domain, provider, bucket, key_prefix, is_private, enabled, notes)
VALUES
  ('admin', 'qiniu', 'rentpro-floor-plans', 'emoji/admin/', FALSE, TRUE, '运营上传素材域'),
  ('video', 'qiniu', 'rentpro-floor-plans', 'emoji/video/', FALSE, TRUE, '视频转图产物域'),
  ('ugc',   'qiniu', 'rentpro-floor-plans', 'emoji/ugc/',   FALSE, TRUE, '用户上传二创域')
ON CONFLICT (domain) DO UPDATE
SET
  provider = EXCLUDED.provider,
  bucket = EXCLUDED.bucket,
  key_prefix = EXCLUDED.key_prefix,
  is_private = EXCLUDED.is_private,
  enabled = EXCLUDED.enabled,
  notes = EXCLUDED.notes,
  updated_at = NOW();

-- admin_asset（运营上传）
CREATE TABLE IF NOT EXISTS admin_asset.collections (
  id BIGSERIAL PRIMARY KEY,
  title VARCHAR(255) NOT NULL DEFAULT '',
  slug VARCHAR(128) NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  cover_url VARCHAR(512) NOT NULL DEFAULT '',
  owner_id BIGINT NOT NULL DEFAULT 0,
  creator_profile_id BIGINT,
  category_id BIGINT,
  ip_id BIGINT,
  theme_id BIGINT,
  source VARCHAR(64) NOT NULL DEFAULT 'manual_zip',
  storage_bucket VARCHAR(128) NOT NULL DEFAULT '',
  qiniu_prefix VARCHAR(512) NOT NULL DEFAULT '',
  file_count INTEGER NOT NULL DEFAULT 0,
  is_featured BOOLEAN NOT NULL DEFAULT FALSE,
  is_pinned BOOLEAN NOT NULL DEFAULT FALSE,
  is_sample BOOLEAN NOT NULL DEFAULT FALSE,
  pinned_at TIMESTAMPTZ,
  latest_zip_key VARCHAR(512) NOT NULL DEFAULT '',
  latest_zip_name VARCHAR(255) NOT NULL DEFAULT '',
  latest_zip_size BIGINT NOT NULL DEFAULT 0,
  latest_zip_at TIMESTAMPTZ,
  download_code VARCHAR(16) NOT NULL DEFAULT '',
  visibility VARCHAR(32) NOT NULL DEFAULT 'public',
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_admin_asset_collections_status_visibility
  ON admin_asset.collections (status, visibility, id DESC);
CREATE INDEX IF NOT EXISTS idx_admin_asset_collections_owner
  ON admin_asset.collections (owner_id);
CREATE INDEX IF NOT EXISTS idx_admin_asset_collections_source
  ON admin_asset.collections (source);
CREATE INDEX IF NOT EXISTS idx_admin_asset_collections_prefix
  ON admin_asset.collections (qiniu_prefix);
CREATE INDEX IF NOT EXISTS idx_admin_asset_collections_deleted_at
  ON admin_asset.collections (deleted_at);

CREATE TABLE IF NOT EXISTS admin_asset.emojis (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL REFERENCES admin_asset.collections(id) ON DELETE CASCADE,
  title VARCHAR(255) NOT NULL DEFAULT '',
  file_url VARCHAR(512) NOT NULL DEFAULT '',
  thumb_url VARCHAR(512) NOT NULL DEFAULT '',
  format VARCHAR(32) NOT NULL DEFAULT '',
  width INTEGER NOT NULL DEFAULT 0,
  height INTEGER NOT NULL DEFAULT 0,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  display_order INTEGER NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_admin_asset_emojis_collection_status_order
  ON admin_asset.emojis (collection_id, status, display_order, id);
CREATE INDEX IF NOT EXISTS idx_admin_asset_emojis_deleted_at
  ON admin_asset.emojis (deleted_at);

CREATE TABLE IF NOT EXISTS admin_asset.collection_zips (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL REFERENCES admin_asset.collections(id) ON DELETE CASCADE,
  zip_key VARCHAR(512) NOT NULL,
  zip_hash VARCHAR(128) NOT NULL DEFAULT '',
  zip_name VARCHAR(255) NOT NULL DEFAULT '',
  size_bytes BIGINT NOT NULL DEFAULT 0,
  uploaded_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_asset_collection_zips_unique
  ON admin_asset.collection_zips (collection_id, zip_key);
CREATE INDEX IF NOT EXISTS idx_admin_asset_collection_zips_uploaded
  ON admin_asset.collection_zips (uploaded_at);

-- video_asset（视频转图片产物）
CREATE TABLE IF NOT EXISTS video_asset.collections (
  id BIGSERIAL PRIMARY KEY,
  title VARCHAR(255) NOT NULL DEFAULT '',
  slug VARCHAR(128) NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  cover_url VARCHAR(512) NOT NULL DEFAULT '',
  owner_id BIGINT NOT NULL DEFAULT 0,
  source VARCHAR(64) NOT NULL DEFAULT 'video_generated',
  storage_bucket VARCHAR(128) NOT NULL DEFAULT '',
  qiniu_prefix VARCHAR(512) NOT NULL DEFAULT '',
  file_count INTEGER NOT NULL DEFAULT 0,
  latest_zip_key VARCHAR(512) NOT NULL DEFAULT '',
  latest_zip_name VARCHAR(255) NOT NULL DEFAULT '',
  latest_zip_size BIGINT NOT NULL DEFAULT 0,
  latest_zip_at TIMESTAMPTZ,
  visibility VARCHAR(32) NOT NULL DEFAULT 'private',
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_video_asset_collections_owner
  ON video_asset.collections (owner_id);
CREATE INDEX IF NOT EXISTS idx_video_asset_collections_status_visibility
  ON video_asset.collections (status, visibility, id DESC);
CREATE INDEX IF NOT EXISTS idx_video_asset_collections_source
  ON video_asset.collections (source);
CREATE INDEX IF NOT EXISTS idx_video_asset_collections_prefix
  ON video_asset.collections (qiniu_prefix);
CREATE INDEX IF NOT EXISTS idx_video_asset_collections_deleted_at
  ON video_asset.collections (deleted_at);

CREATE TABLE IF NOT EXISTS video_asset.emojis (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL REFERENCES video_asset.collections(id) ON DELETE CASCADE,
  title VARCHAR(255) NOT NULL DEFAULT '',
  file_url VARCHAR(512) NOT NULL DEFAULT '',
  thumb_url VARCHAR(512) NOT NULL DEFAULT '',
  format VARCHAR(32) NOT NULL DEFAULT '',
  width INTEGER NOT NULL DEFAULT 0,
  height INTEGER NOT NULL DEFAULT 0,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  display_order INTEGER NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_video_asset_emojis_collection_status_order
  ON video_asset.emojis (collection_id, status, display_order, id);
CREATE INDEX IF NOT EXISTS idx_video_asset_emojis_deleted_at
  ON video_asset.emojis (deleted_at);

CREATE TABLE IF NOT EXISTS video_asset.collection_zips (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL REFERENCES video_asset.collections(id) ON DELETE CASCADE,
  zip_key VARCHAR(512) NOT NULL,
  zip_hash VARCHAR(128) NOT NULL DEFAULT '',
  zip_name VARCHAR(255) NOT NULL DEFAULT '',
  size_bytes BIGINT NOT NULL DEFAULT 0,
  uploaded_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_video_asset_collection_zips_unique
  ON video_asset.collection_zips (collection_id, zip_key);
CREATE INDEX IF NOT EXISTS idx_video_asset_collection_zips_uploaded
  ON video_asset.collection_zips (uploaded_at);

-- ugc_asset（用户二创上传）
CREATE TABLE IF NOT EXISTS ugc_asset.collections (
  id BIGSERIAL PRIMARY KEY,
  title VARCHAR(255) NOT NULL DEFAULT '',
  slug VARCHAR(128) NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  cover_url VARCHAR(512) NOT NULL DEFAULT '',
  owner_id BIGINT NOT NULL DEFAULT 0,
  source VARCHAR(64) NOT NULL DEFAULT 'ugc_upload',
  storage_bucket VARCHAR(128) NOT NULL DEFAULT '',
  qiniu_prefix VARCHAR(512) NOT NULL DEFAULT '',
  file_count INTEGER NOT NULL DEFAULT 0,
  latest_zip_key VARCHAR(512) NOT NULL DEFAULT '',
  latest_zip_name VARCHAR(255) NOT NULL DEFAULT '',
  latest_zip_size BIGINT NOT NULL DEFAULT 0,
  latest_zip_at TIMESTAMPTZ,
  visibility VARCHAR(32) NOT NULL DEFAULT 'private',
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_ugc_asset_collections_owner
  ON ugc_asset.collections (owner_id);
CREATE INDEX IF NOT EXISTS idx_ugc_asset_collections_status_visibility
  ON ugc_asset.collections (status, visibility, id DESC);
CREATE INDEX IF NOT EXISTS idx_ugc_asset_collections_source
  ON ugc_asset.collections (source);
CREATE INDEX IF NOT EXISTS idx_ugc_asset_collections_prefix
  ON ugc_asset.collections (qiniu_prefix);
CREATE INDEX IF NOT EXISTS idx_ugc_asset_collections_deleted_at
  ON ugc_asset.collections (deleted_at);

CREATE TABLE IF NOT EXISTS ugc_asset.emojis (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL REFERENCES ugc_asset.collections(id) ON DELETE CASCADE,
  title VARCHAR(255) NOT NULL DEFAULT '',
  file_url VARCHAR(512) NOT NULL DEFAULT '',
  thumb_url VARCHAR(512) NOT NULL DEFAULT '',
  format VARCHAR(32) NOT NULL DEFAULT '',
  width INTEGER NOT NULL DEFAULT 0,
  height INTEGER NOT NULL DEFAULT 0,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  display_order INTEGER NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_ugc_asset_emojis_collection_status_order
  ON ugc_asset.emojis (collection_id, status, display_order, id);
CREATE INDEX IF NOT EXISTS idx_ugc_asset_emojis_deleted_at
  ON ugc_asset.emojis (deleted_at);

CREATE TABLE IF NOT EXISTS ugc_asset.collection_zips (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL REFERENCES ugc_asset.collections(id) ON DELETE CASCADE,
  zip_key VARCHAR(512) NOT NULL,
  zip_hash VARCHAR(128) NOT NULL DEFAULT '',
  zip_name VARCHAR(255) NOT NULL DEFAULT '',
  size_bytes BIGINT NOT NULL DEFAULT 0,
  uploaded_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ugc_asset_collection_zips_unique
  ON ugc_asset.collection_zips (collection_id, zip_key);
CREATE INDEX IF NOT EXISTS idx_ugc_asset_collection_zips_uploaded
  ON ugc_asset.collection_zips (uploaded_at);
