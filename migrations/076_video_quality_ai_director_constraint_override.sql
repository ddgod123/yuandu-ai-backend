BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS ai_director_constraint_override_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS ai_director_count_expand_ratio DOUBLE PRECISION NOT NULL DEFAULT 0.20,
    ADD COLUMN IF NOT EXISTS ai_director_duration_expand_ratio DOUBLE PRECISION NOT NULL DEFAULT 0.20,
    ADD COLUMN IF NOT EXISTS ai_director_count_absolute_cap INTEGER NOT NULL DEFAULT 10,
    ADD COLUMN IF NOT EXISTS ai_director_duration_absolute_cap_sec DOUBLE PRECISION NOT NULL DEFAULT 6.0;

COMMENT ON COLUMN ops.video_quality_settings.ai_director_constraint_override_enabled IS '是否启用 AI1 建议扩张（在硬约束范围内放宽）';
COMMENT ON COLUMN ops.video_quality_settings.ai_director_count_expand_ratio IS 'AI1 候选数量扩张比例（0~3；1 表示 +100%）';
COMMENT ON COLUMN ops.video_quality_settings.ai_director_duration_expand_ratio IS 'AI1 建议窗口时长扩张比例（0~3；1 表示 +100%）';
COMMENT ON COLUMN ops.video_quality_settings.ai_director_count_absolute_cap IS 'AI1 候选数量绝对上限（用于防失控）';
COMMENT ON COLUMN ops.video_quality_settings.ai_director_duration_absolute_cap_sec IS 'AI1 建议窗口时长绝对上限（秒）';

UPDATE ops.video_quality_settings
SET ai_director_count_expand_ratio = LEAST(GREATEST(COALESCE(ai_director_count_expand_ratio, 0.2), 0), 3),
    ai_director_duration_expand_ratio = LEAST(GREATEST(COALESCE(ai_director_duration_expand_ratio, 0.2), 0), 3),
    ai_director_count_absolute_cap = LEAST(GREATEST(COALESCE(ai_director_count_absolute_cap, 10), 1), 20),
    ai_director_duration_absolute_cap_sec = LEAST(GREATEST(COALESCE(ai_director_duration_absolute_cap_sec, 6.0), 2.0), 12.0);

UPDATE ops.video_quality_settings
SET ai_director_count_absolute_cap = GREATEST(ai_director_count_absolute_cap, COALESCE(gif_candidate_max_outputs, 1));

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_ai_director_count_expand_ratio'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_ai_director_count_expand_ratio
            CHECK (ai_director_count_expand_ratio >= 0 AND ai_director_count_expand_ratio <= 3);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_ai_director_duration_expand_ratio'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_ai_director_duration_expand_ratio
            CHECK (ai_director_duration_expand_ratio >= 0 AND ai_director_duration_expand_ratio <= 3);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_ai_director_count_absolute_cap'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_ai_director_count_absolute_cap
            CHECK (ai_director_count_absolute_cap >= 1 AND ai_director_count_absolute_cap <= 20);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname IN (
            'chk_video_quality_settings_ai_director_duration_absolute_cap_sec',
            'chk_video_quality_settings_ai_director_duration_absolute_cap_se',
            'chk_vqs_ai_director_duration_abs_cap_sec'
        )
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_vqs_ai_director_duration_abs_cap_sec
            CHECK (ai_director_duration_absolute_cap_sec >= 2.0 AND ai_director_duration_absolute_cap_sec <= 12.0);
    END IF;
END
$$;

ALTER TABLE public.video_image_quality_settings
    ADD COLUMN IF NOT EXISTS ai_director_constraint_override_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS ai_director_count_expand_ratio DOUBLE PRECISION NOT NULL DEFAULT 0.20,
    ADD COLUMN IF NOT EXISTS ai_director_duration_expand_ratio DOUBLE PRECISION NOT NULL DEFAULT 0.20,
    ADD COLUMN IF NOT EXISTS ai_director_count_absolute_cap INTEGER NOT NULL DEFAULT 10,
    ADD COLUMN IF NOT EXISTS ai_director_duration_absolute_cap_sec DOUBLE PRECISION NOT NULL DEFAULT 6.0;

COMMENT ON COLUMN public.video_image_quality_settings.ai_director_constraint_override_enabled IS '是否启用 AI1 建议扩张';
COMMENT ON COLUMN public.video_image_quality_settings.ai_director_count_expand_ratio IS 'AI1 候选数量扩张比例（0~3）';
COMMENT ON COLUMN public.video_image_quality_settings.ai_director_duration_expand_ratio IS 'AI1 建议窗口时长扩张比例（0~3）';
COMMENT ON COLUMN public.video_image_quality_settings.ai_director_count_absolute_cap IS 'AI1 候选数量绝对上限';
COMMENT ON COLUMN public.video_image_quality_settings.ai_director_duration_absolute_cap_sec IS 'AI1 建议窗口时长绝对上限（秒）';

UPDATE public.video_image_quality_settings
SET ai_director_count_expand_ratio = LEAST(GREATEST(COALESCE(ai_director_count_expand_ratio, 0.2), 0), 3),
    ai_director_duration_expand_ratio = LEAST(GREATEST(COALESCE(ai_director_duration_expand_ratio, 0.2), 0), 3),
    ai_director_count_absolute_cap = LEAST(GREATEST(COALESCE(ai_director_count_absolute_cap, 10), 1), 20),
    ai_director_duration_absolute_cap_sec = LEAST(GREATEST(COALESCE(ai_director_duration_absolute_cap_sec, 6.0), 2.0), 12.0);

UPDATE public.video_image_quality_settings
SET ai_director_count_absolute_cap = GREATEST(ai_director_count_absolute_cap, COALESCE(gif_candidate_max_outputs, 1));

COMMIT;
