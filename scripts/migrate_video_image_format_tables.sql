-- 每格式一套 public 镜像表（jobs / outputs / packages / events / feedback）
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/migrate_video_image_format_tables.sql

DO $$
DECLARE
  fmt text;
  formats text[] := ARRAY['gif', 'png', 'jpg', 'webp', 'live', 'mp4'];
BEGIN
  FOREACH fmt IN ARRAY formats LOOP
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS public.video_image_jobs_%I (LIKE public.video_image_jobs INCLUDING ALL)',
      fmt
    );
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS public.video_image_outputs_%I (LIKE public.video_image_outputs INCLUDING ALL)',
      fmt
    );
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS public.video_image_packages_%I (LIKE public.video_image_packages INCLUDING ALL)',
      fmt
    );
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS public.video_image_events_%I (LIKE public.video_image_events INCLUDING ALL)',
      fmt
    );
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS public.video_image_feedback_%I (LIKE public.video_image_feedback INCLUDING ALL)',
      fmt
    );
  END LOOP;
END $$;
