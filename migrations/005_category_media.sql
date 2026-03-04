BEGIN;

ALTER TABLE taxonomy.categories
  ADD COLUMN IF NOT EXISTS cover_url VARCHAR(512),
  ADD COLUMN IF NOT EXISTS icon VARCHAR(128);

COMMENT ON COLUMN taxonomy.categories.cover_url IS '分类封面图地址';
COMMENT ON COLUMN taxonomy.categories.icon IS '分类图标（可为 emoji 或短文本）';

COMMIT;
