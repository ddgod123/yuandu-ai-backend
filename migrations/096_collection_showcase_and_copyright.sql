-- 096_collection_showcase_and_copyright.sql
-- 目标：
-- 1) 支持「表情包赏析」展示位（只展示不下载）；
-- 2) 支持合集级版权信息维护（作者、原作、来源链接）。

BEGIN;

ALTER TABLE archive.collections
  ADD COLUMN IF NOT EXISTS is_showcase BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE archive.collections
  ADD COLUMN IF NOT EXISTS copyright_author VARCHAR(128) NOT NULL DEFAULT '';

ALTER TABLE archive.collections
  ADD COLUMN IF NOT EXISTS copyright_work VARCHAR(255) NOT NULL DEFAULT '';

ALTER TABLE archive.collections
  ADD COLUMN IF NOT EXISTS copyright_link VARCHAR(512) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_collections_is_showcase
  ON archive.collections(is_showcase);

COMMENT ON COLUMN archive.collections.is_showcase IS '是否仅在表情包赏析页展示（true=赏析，默认禁用下载）';
COMMENT ON COLUMN archive.collections.copyright_author IS '版权署名：作者/画师ID';
COMMENT ON COLUMN archive.collections.copyright_work IS '版权署名：原作（如作品名）';
COMMENT ON COLUMN archive.collections.copyright_link IS '版权署名：来源链接或作者主页';

COMMIT;
