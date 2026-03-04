BEGIN;

CREATE TABLE IF NOT EXISTS taxonomy.themes (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(64) NOT NULL,
  slug VARCHAR(64) NOT NULL UNIQUE,
  description TEXT NULL,
  sort INTEGER NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_taxonomy_themes_status ON taxonomy.themes(status);
CREATE INDEX IF NOT EXISTS idx_taxonomy_themes_sort ON taxonomy.themes(sort);

COMMENT ON TABLE taxonomy.themes IS '资源主题分类（站点顶部栏目）';
COMMENT ON COLUMN taxonomy.themes.id IS '主键';
COMMENT ON COLUMN taxonomy.themes.name IS '主题名称';
COMMENT ON COLUMN taxonomy.themes.slug IS '主题标识';
COMMENT ON COLUMN taxonomy.themes.description IS '主题说明';
COMMENT ON COLUMN taxonomy.themes.sort IS '排序';
COMMENT ON COLUMN taxonomy.themes.status IS '状态';
COMMENT ON COLUMN taxonomy.themes.created_at IS '创建时间';
COMMENT ON COLUMN taxonomy.themes.updated_at IS '更新时间';

INSERT INTO taxonomy.themes (name, slug, description, sort, status)
VALUES
  ('表情包', 'emoji', '表情包与贴纸资源', 10, 'active'),
  ('头像', 'avatar', '头像与人物图像资源', 20, 'active'),
  ('壁纸', 'wallpaper', '手机/桌面壁纸资源', 30, 'active'),
  ('插画', 'illustration', '插画与视觉素材资源', 40, 'active')
ON CONFLICT DO NOTHING;

COMMIT;
