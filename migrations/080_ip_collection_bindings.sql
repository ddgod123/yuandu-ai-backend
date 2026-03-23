-- 080_ip_collection_bindings.sql
-- 目标：支持 IP 与合集的手工绑定（IP 页面维护绑定合集 ID）
-- 说明：
-- 1) 新增 taxonomy.ip_collection_bindings 作为主绑定表；
-- 2) 保留 archive.collections.ip_id 兼容旧链路，新链路优先读绑定表；
-- 3) 支持排序、状态、备注字段，便于运营管理与审计。

BEGIN;

CREATE TABLE IF NOT EXISTS taxonomy.ip_collection_bindings (
  id BIGSERIAL PRIMARY KEY,
  ip_id BIGINT NOT NULL,
  collection_id BIGINT NOT NULL,
  sort INTEGER NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  note TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ip_collection_bindings_unique
  ON taxonomy.ip_collection_bindings(ip_id, collection_id);

CREATE INDEX IF NOT EXISTS idx_ip_collection_bindings_ip_sort
  ON taxonomy.ip_collection_bindings(ip_id, sort, id);

CREATE INDEX IF NOT EXISTS idx_ip_collection_bindings_collection
  ON taxonomy.ip_collection_bindings(collection_id);

CREATE INDEX IF NOT EXISTS idx_ip_collection_bindings_status
  ON taxonomy.ip_collection_bindings(status);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'fk_ip_collection_bindings_ip'
  ) THEN
    ALTER TABLE taxonomy.ip_collection_bindings
      ADD CONSTRAINT fk_ip_collection_bindings_ip
      FOREIGN KEY (ip_id) REFERENCES taxonomy.ips(id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'fk_ip_collection_bindings_collection'
  ) THEN
    ALTER TABLE taxonomy.ip_collection_bindings
      ADD CONSTRAINT fk_ip_collection_bindings_collection
      FOREIGN KEY (collection_id) REFERENCES archive.collections(id) ON DELETE CASCADE;
  END IF;
END $$;

COMMENT ON TABLE taxonomy.ip_collection_bindings IS 'IP 与表情包合集绑定关系（运营可手工维护）';
COMMENT ON COLUMN taxonomy.ip_collection_bindings.ip_id IS 'IP ID（taxonomy.ips.id）';
COMMENT ON COLUMN taxonomy.ip_collection_bindings.collection_id IS '合集 ID（archive.collections.id）';
COMMENT ON COLUMN taxonomy.ip_collection_bindings.sort IS '同一 IP 下的排序（越小越靠前）';
COMMENT ON COLUMN taxonomy.ip_collection_bindings.status IS '绑定状态：active|inactive';
COMMENT ON COLUMN taxonomy.ip_collection_bindings.note IS '运营备注';

COMMIT;
