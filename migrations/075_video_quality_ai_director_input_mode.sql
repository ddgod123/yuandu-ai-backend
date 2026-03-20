BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS ai_director_input_mode VARCHAR(16) NOT NULL DEFAULT 'hybrid';

COMMENT ON COLUMN ops.video_quality_settings.ai_director_input_mode IS 'AI1 输入模式（frames|full_video|hybrid）';

UPDATE ops.video_quality_settings
SET ai_director_input_mode = LOWER(TRIM(COALESCE(NULLIF(ai_director_input_mode, ''), 'hybrid')));

UPDATE ops.video_quality_settings
SET ai_director_input_mode = CASE
    WHEN ai_director_input_mode IN ('frames', 'full_video', 'hybrid') THEN ai_director_input_mode
    ELSE 'hybrid'
END;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_ai_director_input_mode'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_ai_director_input_mode
            CHECK (ai_director_input_mode IN ('frames', 'full_video', 'hybrid'));
    END IF;
END
$$;

ALTER TABLE public.video_image_quality_settings
    ADD COLUMN IF NOT EXISTS ai_director_input_mode VARCHAR(16) NOT NULL DEFAULT 'hybrid';

COMMENT ON COLUMN public.video_image_quality_settings.ai_director_input_mode IS 'AI1 输入模式（frames|full_video|hybrid）';

UPDATE public.video_image_quality_settings
SET ai_director_input_mode = LOWER(TRIM(COALESCE(NULLIF(ai_director_input_mode, ''), 'hybrid')));

UPDATE public.video_image_quality_settings
SET ai_director_input_mode = CASE
    WHEN ai_director_input_mode IN ('frames', 'full_video', 'hybrid') THEN ai_director_input_mode
    ELSE 'hybrid'
END;

COMMIT;

