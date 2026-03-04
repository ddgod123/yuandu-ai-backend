BEGIN;

CREATE TABLE IF NOT EXISTS taxonomy.tag_groups (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(64) NOT NULL,
  slug VARCHAR(64) NOT NULL UNIQUE,
  description TEXT NULL,
  sort INTEGER NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE taxonomy.tags
  ADD COLUMN IF NOT EXISTS tag_group_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_taxonomy_tags_group_id ON taxonomy.tags(tag_group_id);
CREATE INDEX IF NOT EXISTS idx_taxonomy_tag_groups_sort ON taxonomy.tag_groups(sort);
CREATE INDEX IF NOT EXISTS idx_taxonomy_tag_groups_status ON taxonomy.tag_groups(status);

ALTER TABLE taxonomy.tags
  ADD CONSTRAINT fk_taxonomy_tags_group
  FOREIGN KEY (tag_group_id) REFERENCES taxonomy.tag_groups(id) ON DELETE SET NULL;

COMMENT ON TABLE taxonomy.tag_groups IS '标签分类（后台快速检索用）';
COMMENT ON COLUMN taxonomy.tag_groups.id IS '主键';
COMMENT ON COLUMN taxonomy.tag_groups.name IS '分类名称';
COMMENT ON COLUMN taxonomy.tag_groups.slug IS '分类标识';
COMMENT ON COLUMN taxonomy.tag_groups.description IS '分类说明';
COMMENT ON COLUMN taxonomy.tag_groups.sort IS '排序';
COMMENT ON COLUMN taxonomy.tag_groups.status IS '状态';
COMMENT ON COLUMN taxonomy.tag_groups.created_at IS '创建时间';
COMMENT ON COLUMN taxonomy.tag_groups.updated_at IS '更新时间';

COMMENT ON COLUMN taxonomy.tags.tag_group_id IS '标签分类ID';

INSERT INTO taxonomy.tag_groups (name, slug, description, sort, status)
VALUES
  ('动物', 'animals', '动物相关标签', 10, 'active'),
  ('明星', 'celebrity', '明星相关标签', 20, 'active'),
  ('动漫', 'anime', '动漫/二次元相关标签', 30, 'active'),
  ('鬼畜', 'meme', '鬼畜/梗相关标签', 40, 'active'),
  ('卡通', 'cartoon', '卡通/手绘相关标签', 50, 'active'),
  ('数字', 'numbers', '数字类标签', 60, 'active')
ON CONFLICT DO NOTHING;

UPDATE taxonomy.tags
SET tag_group_id = (SELECT id FROM taxonomy.tag_groups WHERE slug = 'animals')
WHERE name IN (
  '狗','狗狗','柴犬','哈士奇','柯基',
  '猫','猫咪','橘猫','黑猫','奶牛猫','喵星人',
  '猴儿','猴子','猩猩','考拉','熊猫','兔子','小鸡','小鸭','小鸟',
  '萌宠','狗狗','猫咪'
);

UPDATE taxonomy.tags
SET tag_group_id = (SELECT id FROM taxonomy.tag_groups WHERE slug = 'celebrity')
WHERE name IN ('明星');

UPDATE taxonomy.tags
SET tag_group_id = (SELECT id FROM taxonomy.tag_groups WHERE slug = 'anime')
WHERE name IN ('动漫','二次元','动画','卡通');

UPDATE taxonomy.tags
SET tag_group_id = (SELECT id FROM taxonomy.tag_groups WHERE slug = 'meme')
WHERE name IN ('鬼畜','搞怪','搞笑');

UPDATE taxonomy.tags
SET tag_group_id = (SELECT id FROM taxonomy.tag_groups WHERE slug = 'cartoon')
WHERE name IN ('卡通','手绘');

COMMIT;
