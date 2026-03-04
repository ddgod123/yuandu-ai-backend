BEGIN;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'categories_slug_key'
      AND conrelid = 'taxonomy.categories'::regclass
  ) THEN
    ALTER TABLE taxonomy.categories DROP CONSTRAINT categories_slug_key;
  END IF;
END$$;

DROP INDEX IF EXISTS taxonomy.categories_slug_key;
DROP INDEX IF EXISTS categories_slug_key;
DROP INDEX IF EXISTS idx_taxonomy_categories_parent_slug;
DROP INDEX IF EXISTS taxonomy.idx_taxonomy_categories_parent_slug;

CREATE UNIQUE INDEX IF NOT EXISTS idx_taxonomy_categories_parent_slug
  ON taxonomy.categories (COALESCE(parent_id, 0), lower(slug));

COMMENT ON INDEX taxonomy.idx_taxonomy_categories_parent_slug IS '同一父级下 slug（忽略大小写）不可重复（顶级父级视为 0）';

COMMIT;
