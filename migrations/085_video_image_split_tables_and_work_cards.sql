BEGIN;

-- 扩展 requested_format 约束，纳入 mp4（兼容新格式任务）
ALTER TABLE IF EXISTS public.video_image_jobs
  DROP CONSTRAINT IF EXISTS chk_video_image_jobs_requested_format;

ALTER TABLE IF EXISTS public.video_image_jobs
  ADD CONSTRAINT chk_video_image_jobs_requested_format
  CHECK (requested_format IN ('gif', 'webp', 'jpg', 'png', 'live', 'svg', 'mp4'));

DO $$
DECLARE
  fmt TEXT;
BEGIN
  FOREACH fmt IN ARRAY ARRAY['gif', 'png', 'jpg', 'webp', 'live', 'mp4']
  LOOP
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS public.video_image_jobs_%I (LIKE public.video_image_jobs INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED INCLUDING IDENTITY INCLUDING STORAGE INCLUDING COMMENTS)',
      fmt
    );
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS public.video_image_outputs_%I (LIKE public.video_image_outputs INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED INCLUDING IDENTITY INCLUDING STORAGE INCLUDING COMMENTS)',
      fmt
    );
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS public.video_image_packages_%I (LIKE public.video_image_packages INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED INCLUDING IDENTITY INCLUDING STORAGE INCLUDING COMMENTS)',
      fmt
    );
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS public.video_image_events_%I (LIKE public.video_image_events INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED INCLUDING IDENTITY INCLUDING STORAGE INCLUDING COMMENTS)',
      fmt
    );
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS public.video_image_feedback_%I (LIKE public.video_image_feedback INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED INCLUDING IDENTITY INCLUDING STORAGE INCLUDING COMMENTS)',
      fmt
    );

    EXECUTE format(
      'ALTER TABLE public.video_image_jobs_%I DROP CONSTRAINT IF EXISTS chk_video_image_jobs_%I_requested_format',
      fmt, fmt
    );
    EXECUTE format(
      'ALTER TABLE public.video_image_jobs_%I ADD CONSTRAINT chk_video_image_jobs_%I_requested_format CHECK (LOWER(COALESCE(requested_format, '''')) = %L)',
      fmt, fmt, fmt
    );

    EXECUTE format(
      'CREATE UNIQUE INDEX IF NOT EXISTS uq_video_image_jobs_%I_idempotency_key ON public.video_image_jobs_%I (idempotency_key) WHERE idempotency_key <> ''''',
      fmt, fmt
    );
    EXECUTE format(
      'CREATE INDEX IF NOT EXISTS idx_video_image_jobs_%I_user_updated ON public.video_image_jobs_%I (user_id, updated_at DESC, id DESC)',
      fmt, fmt
    );
    EXECUTE format(
      'CREATE INDEX IF NOT EXISTS idx_video_image_jobs_%I_status_updated ON public.video_image_jobs_%I (status, updated_at DESC, id DESC)',
      fmt, fmt
    );

    EXECUTE format(
      'CREATE UNIQUE INDEX IF NOT EXISTS uq_video_image_outputs_%I_object_key ON public.video_image_outputs_%I (object_key)',
      fmt, fmt
    );
    EXECUTE format(
      'CREATE INDEX IF NOT EXISTS idx_video_image_outputs_%I_job_role_created ON public.video_image_outputs_%I (job_id, file_role, created_at DESC, id DESC)',
      fmt, fmt
    );
    EXECUTE format(
      'CREATE INDEX IF NOT EXISTS idx_video_image_outputs_%I_job_format ON public.video_image_outputs_%I (job_id, format, id DESC)',
      fmt, fmt
    );

    EXECUTE format(
      'CREATE UNIQUE INDEX IF NOT EXISTS uq_video_image_packages_%I_job ON public.video_image_packages_%I (job_id)',
      fmt, fmt
    );
    EXECUTE format(
      'CREATE INDEX IF NOT EXISTS idx_video_image_packages_%I_user_created ON public.video_image_packages_%I (user_id, created_at DESC, id DESC)',
      fmt, fmt
    );

    EXECUTE format(
      'CREATE INDEX IF NOT EXISTS idx_video_image_events_%I_job_id ON public.video_image_events_%I (job_id, id DESC)',
      fmt, fmt
    );

    EXECUTE format(
      'CREATE INDEX IF NOT EXISTS idx_video_image_feedback_%I_job_created ON public.video_image_feedback_%I (job_id, created_at DESC, id DESC)',
      fmt, fmt
    );
    EXECUTE format(
      'CREATE INDEX IF NOT EXISTS idx_video_image_feedback_%I_user_created ON public.video_image_feedback_%I (user_id, created_at DESC, id DESC)',
      fmt, fmt
    );
  END LOOP;
END $$;

CREATE TABLE IF NOT EXISTS public.video_work_cards (
  job_id BIGINT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  requested_format VARCHAR(16) NOT NULL DEFAULT '',
  title VARCHAR(255) NOT NULL DEFAULT '',
  status VARCHAR(32) NOT NULL DEFAULT 'queued',
  stage VARCHAR(32) NOT NULL DEFAULT 'queued',
  progress SMALLINT NOT NULL DEFAULT 0,
  result_collection_id BIGINT NULL,
  file_count INTEGER NOT NULL DEFAULT 0,
  preview_images JSONB NOT NULL DEFAULT '[]'::jsonb,
  format_summary JSONB NOT NULL DEFAULT '[]'::jsonb,
  package_status VARCHAR(32) NOT NULL DEFAULT 'processing',
  package_name VARCHAR(255) NOT NULL DEFAULT '',
  package_size_bytes BIGINT NOT NULL DEFAULT 0,
  quality_sample_count INTEGER NOT NULL DEFAULT 0,
  quality_top_score DOUBLE PRECISION NOT NULL DEFAULT 0,
  quality_avg_score DOUBLE PRECISION NOT NULL DEFAULT 0,
  quality_avg_loop_closure DOUBLE PRECISION NOT NULL DEFAULT 0,
  options JSONB NOT NULL DEFAULT '{}'::jsonb,
  metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
  source_updated_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_work_cards_user_updated
  ON public.video_work_cards (user_id, updated_at DESC, job_id DESC);

CREATE INDEX IF NOT EXISTS idx_video_work_cards_user_format_updated
  ON public.video_work_cards (user_id, requested_format, updated_at DESC, job_id DESC);

CREATE INDEX IF NOT EXISTS idx_video_work_cards_user_status_updated
  ON public.video_work_cards (user_id, status, updated_at DESC, job_id DESC);

COMMENT ON TABLE public.video_work_cards IS '我的作品卡片读模型（按任务聚合产物、ZIP与质量摘要）';
COMMENT ON COLUMN public.video_work_cards.preview_images IS '卡片预览图URL数组（最多15张）';
COMMENT ON COLUMN public.video_work_cards.format_summary IS '格式汇总数组，如 GIF × 2';

COMMIT;
