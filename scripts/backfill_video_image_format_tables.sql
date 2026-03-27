-- 将历史公共镜像表数据按 requested_format 回填到分表。
-- 依赖：先执行 migrate_video_image_format_tables.sql 创建分表结构。
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/backfill_video_image_format_tables.sql

DO $$
DECLARE
  fmt text;
  formats text[] := ARRAY['gif', 'png', 'jpg', 'webp', 'live', 'mp4'];
BEGIN
  FOREACH fmt IN ARRAY formats LOOP
    EXECUTE format(
      'INSERT INTO public.video_image_jobs_%1$I
       SELECT j.*
       FROM public.video_image_jobs j
       WHERE COALESCE(NULLIF(LOWER(TRIM(j.requested_format)), ''''), ''gif'') = %1$L
       ON CONFLICT (id) DO NOTHING',
      fmt
    );

    EXECUTE format(
      'INSERT INTO public.video_image_outputs_%1$I
       SELECT o.*
       FROM public.video_image_outputs o
       JOIN public.video_image_jobs j ON j.id = o.job_id
       WHERE COALESCE(NULLIF(LOWER(TRIM(j.requested_format)), ''''), ''gif'') = %1$L
       ON CONFLICT (object_key) DO NOTHING',
      fmt
    );

    EXECUTE format(
      'INSERT INTO public.video_image_packages_%1$I
       SELECT p.*
       FROM public.video_image_packages p
       JOIN public.video_image_jobs j ON j.id = p.job_id
       WHERE COALESCE(NULLIF(LOWER(TRIM(j.requested_format)), ''''), ''gif'') = %1$L
       ON CONFLICT (job_id) DO NOTHING',
      fmt
    );

    EXECUTE format(
      'INSERT INTO public.video_image_events_%1$I
       SELECT e.*
       FROM public.video_image_events e
       JOIN public.video_image_jobs j ON j.id = e.job_id
       WHERE COALESCE(NULLIF(LOWER(TRIM(j.requested_format)), ''''), ''gif'') = %1$L
       ON CONFLICT (id) DO NOTHING',
      fmt
    );

    EXECUTE format(
      'INSERT INTO public.video_image_feedback_%1$I
       SELECT f.*
       FROM public.video_image_feedback f
       JOIN public.video_image_jobs j ON j.id = f.job_id
       WHERE COALESCE(NULLIF(LOWER(TRIM(j.requested_format)), ''''), ''gif'') = %1$L
       ON CONFLICT (id) DO NOTHING',
      fmt
    );
  END LOOP;
END $$;
