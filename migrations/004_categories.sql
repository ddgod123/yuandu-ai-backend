BEGIN;

CREATE TABLE IF NOT EXISTS taxonomy.categories (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  slug VARCHAR(128) NOT NULL UNIQUE,
  parent_id BIGINT NULL,
  prefix VARCHAR(256) NOT NULL UNIQUE,
  description TEXT NULL,
  sort INTEGER NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_taxonomy_categories_parent_id ON taxonomy.categories(parent_id);
CREATE INDEX IF NOT EXISTS idx_taxonomy_categories_status ON taxonomy.categories(status);
CREATE INDEX IF NOT EXISTS idx_taxonomy_categories_sort ON taxonomy.categories(sort);

COMMENT ON TABLE taxonomy.categories IS '表情包分类目录';
COMMENT ON COLUMN taxonomy.categories.id IS '主键';
COMMENT ON COLUMN taxonomy.categories.name IS '分类名称';
COMMENT ON COLUMN taxonomy.categories.slug IS '分类标识';
COMMENT ON COLUMN taxonomy.categories.parent_id IS '父级分类ID';
COMMENT ON COLUMN taxonomy.categories.prefix IS '七牛对象前缀（以 emoji/ 开头）';
COMMENT ON COLUMN taxonomy.categories.description IS '分类说明';
COMMENT ON COLUMN taxonomy.categories.sort IS '排序';
COMMENT ON COLUMN taxonomy.categories.status IS '状态';
COMMENT ON COLUMN taxonomy.categories.created_at IS '创建时间';
COMMENT ON COLUMN taxonomy.categories.updated_at IS '更新时间';
COMMENT ON COLUMN taxonomy.categories.deleted_at IS '删除时间（软删除）';

COMMIT;
