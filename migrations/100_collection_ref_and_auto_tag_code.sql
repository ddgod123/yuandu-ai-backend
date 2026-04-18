-- 100_collection_ref_and_auto_tag_code.sql
-- 目标：
-- 1) 为运营补充「自定义编号」字段（人工对照本地 ZIP 源文件）；
-- 2) 为系统补充「唯一传输编码」字段（自动生成，用作后续自动回传/传输 tag）。

BEGIN;

ALTER TABLE archive.collections
  ADD COLUMN IF NOT EXISTS manual_ref_code VARCHAR(128) NOT NULL DEFAULT '';

ALTER TABLE archive.collections
  ADD COLUMN IF NOT EXISTS auto_tag_code VARCHAR(40) NOT NULL DEFAULT '';

-- 回填历史数据：使用基于 collection id 的稳定编码，保证无空值并避免重复。
UPDATE archive.collections
SET auto_tag_code = 'c' || lpad(lower(to_hex(id)), 16, '0')
WHERE btrim(COALESCE(auto_tag_code, '')) = '';

CREATE INDEX IF NOT EXISTS idx_collections_manual_ref_code
  ON archive.collections(manual_ref_code);

CREATE UNIQUE INDEX IF NOT EXISTS uq_collections_auto_tag_code
  ON archive.collections(auto_tag_code)
  WHERE btrim(COALESCE(auto_tag_code, '')) <> '';

COMMENT ON COLUMN archive.collections.manual_ref_code IS '运营自定义编号，用于线下/本地 ZIP 源文件检索';
COMMENT ON COLUMN archive.collections.auto_tag_code IS '系统自动生成的唯一传输编码，用于自动回传/传输链路打 tag';

COMMIT;

