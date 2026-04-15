BEGIN;

-- 同步 public.video_image_jobs 及其分表 stage 约束：支持 AI1 确认暂停态。
ALTER TABLE IF EXISTS public.video_image_jobs
  DROP CONSTRAINT IF EXISTS chk_video_image_jobs_stage;

ALTER TABLE IF EXISTS public.video_image_jobs
  ADD CONSTRAINT chk_video_image_jobs_stage
  CHECK (stage IN (
    'queued',
    'preprocessing',
    'analyzing',
    'awaiting_ai1_confirm',
    'rendering',
    'uploading',
    'indexing',
    'done',
    'failed',
    'cancelled',
    'retrying'
  ));

DO $$
DECLARE
  fmt TEXT;
  jobs_table TEXT;
  outputs_table TEXT;
BEGIN
  FOREACH fmt IN ARRAY ARRAY['gif', 'png', 'jpg', 'webp', 'live', 'mp4']
  LOOP
    jobs_table := format('video_image_jobs_%s', fmt);
    outputs_table := format('video_image_outputs_%s', fmt);

    IF to_regclass(format('public.%I', jobs_table)) IS NOT NULL THEN
      EXECUTE format(
        'ALTER TABLE public.%I DROP CONSTRAINT IF EXISTS chk_video_image_jobs_stage',
        jobs_table
      );
      EXECUTE format(
        'ALTER TABLE public.%I ADD CONSTRAINT chk_video_image_jobs_stage CHECK (stage IN (''queued'',''preprocessing'',''analyzing'',''awaiting_ai1_confirm'',''rendering'',''uploading'',''indexing'',''done'',''failed'',''cancelled'',''retrying''))',
        jobs_table
      );
    END IF;

    -- 085 分表后，outputs 分表缺失后续迁移新增列，这里补齐。
    IF to_regclass(format('public.%I', outputs_table)) IS NOT NULL THEN
      EXECUTE format(
        'ALTER TABLE public.%I ADD COLUMN IF NOT EXISTS proposal_id BIGINT NULL',
        outputs_table
      );
      EXECUTE format(
        'ALTER TABLE public.%I ADD COLUMN IF NOT EXISTS gif_loop_tune_applied BOOLEAN NOT NULL DEFAULT FALSE',
        outputs_table
      );
      EXECUTE format(
        'ALTER TABLE public.%I ADD COLUMN IF NOT EXISTS gif_loop_tune_effective_applied BOOLEAN NOT NULL DEFAULT FALSE',
        outputs_table
      );
      EXECUTE format(
        'ALTER TABLE public.%I ADD COLUMN IF NOT EXISTS gif_loop_tune_fallback_to_base BOOLEAN NOT NULL DEFAULT FALSE',
        outputs_table
      );
      EXECUTE format(
        'ALTER TABLE public.%I ADD COLUMN IF NOT EXISTS gif_loop_tune_score DOUBLE PRECISION NOT NULL DEFAULT 0',
        outputs_table
      );
      EXECUTE format(
        'ALTER TABLE public.%I ADD COLUMN IF NOT EXISTS gif_loop_tune_loop_closure DOUBLE PRECISION NOT NULL DEFAULT 0',
        outputs_table
      );
      EXECUTE format(
        'ALTER TABLE public.%I ADD COLUMN IF NOT EXISTS gif_loop_tune_motion_mean DOUBLE PRECISION NOT NULL DEFAULT 0',
        outputs_table
      );
      EXECUTE format(
        'ALTER TABLE public.%I ADD COLUMN IF NOT EXISTS gif_loop_tune_effective_sec DOUBLE PRECISION NOT NULL DEFAULT 0',
        outputs_table
      );

      EXECUTE format(
        'CREATE INDEX IF NOT EXISTS idx_%I_proposal_id ON public.%I (proposal_id)',
        outputs_table,
        outputs_table
      );
    END IF;
  END LOOP;
END $$;

COMMIT;
