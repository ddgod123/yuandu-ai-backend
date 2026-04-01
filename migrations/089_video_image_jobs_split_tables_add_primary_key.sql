BEGIN;

-- 085 分表使用 LIKE ... INCLUDING CONSTRAINTS，未复制父表主键索引。
-- 这里为所有 public.video_image_*_格式 分表补齐 PRIMARY KEY(id)，避免 ON CONFLICT(id) 报 42P10。
DO $$
DECLARE
  base_name TEXT;
  fmt TEXT;
  table_name TEXT;
  has_pk BOOLEAN;
BEGIN
  FOREACH base_name IN ARRAY ARRAY[
    'video_image_jobs',
    'video_image_outputs',
    'video_image_packages',
    'video_image_events',
    'video_image_feedback'
  ]
  LOOP
    FOREACH fmt IN ARRAY ARRAY['gif', 'png', 'jpg', 'webp', 'live', 'mp4']
    LOOP
      table_name := format('%s_%s', base_name, fmt);

      IF to_regclass(format('public.%I', table_name)) IS NULL THEN
        CONTINUE;
      END IF;

      SELECT EXISTS (
        SELECT 1
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = t.relnamespace
        WHERE n.nspname = 'public'
          AND t.relname = table_name
          AND c.contype = 'p'
      ) INTO has_pk;

      IF NOT has_pk THEN
        EXECUTE format(
          'ALTER TABLE public.%I ADD CONSTRAINT %I PRIMARY KEY (id)',
          table_name,
          table_name || '_pkey'
        );
      END IF;
    END LOOP;
  END LOOP;
END $$;

COMMIT;
