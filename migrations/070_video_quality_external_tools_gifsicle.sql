BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS gif_gifsicle_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS gif_gifsicle_level SMALLINT NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS gif_gifsicle_skip_below_kb INTEGER NOT NULL DEFAULT 256,
    ADD COLUMN IF NOT EXISTS gif_gifsicle_min_gain_ratio DOUBLE PRECISION NOT NULL DEFAULT 0.03;

COMMENT ON COLUMN ops.video_quality_settings.gif_gifsicle_enabled IS '是否启用 GIFSICLE 二次优化';
COMMENT ON COLUMN ops.video_quality_settings.gif_gifsicle_level IS 'GIFSICLE 优化等级（1..3）';
COMMENT ON COLUMN ops.video_quality_settings.gif_gifsicle_skip_below_kb IS '低于该体积（KB）的 GIF 跳过 GIFSICLE';
COMMENT ON COLUMN ops.video_quality_settings.gif_gifsicle_min_gain_ratio IS '最小收益比例，低于阈值保留原文件';

UPDATE ops.video_quality_settings
SET gif_gifsicle_enabled = COALESCE(gif_gifsicle_enabled, TRUE),
    gif_gifsicle_level = LEAST(GREATEST(COALESCE(gif_gifsicle_level, 2), 1), 3),
    gif_gifsicle_skip_below_kb = LEAST(GREATEST(COALESCE(gif_gifsicle_skip_below_kb, 256), 0), 4096),
    gif_gifsicle_min_gain_ratio = LEAST(GREATEST(COALESCE(gif_gifsicle_min_gain_ratio, 0.03), 0), 0.50);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_gifsicle_level'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_gifsicle_level
            CHECK (gif_gifsicle_level BETWEEN 1 AND 3);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_gifsicle_skip_below_kb'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_gifsicle_skip_below_kb
            CHECK (gif_gifsicle_skip_below_kb BETWEEN 0 AND 4096);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_gifsicle_min_gain_ratio'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_gifsicle_min_gain_ratio
            CHECK (gif_gifsicle_min_gain_ratio >= 0 AND gif_gifsicle_min_gain_ratio <= 0.50);
    END IF;
END
$$;

ALTER TABLE public.video_image_quality_settings
    ADD COLUMN IF NOT EXISTS gif_gifsicle_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS gif_gifsicle_level SMALLINT NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS gif_gifsicle_skip_below_kb INTEGER NOT NULL DEFAULT 256,
    ADD COLUMN IF NOT EXISTS gif_gifsicle_min_gain_ratio DOUBLE PRECISION NOT NULL DEFAULT 0.03;

COMMENT ON COLUMN public.video_image_quality_settings.gif_gifsicle_enabled IS '是否启用 GIFSICLE 二次优化';
COMMENT ON COLUMN public.video_image_quality_settings.gif_gifsicle_level IS 'GIFSICLE 优化等级（1..3）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_gifsicle_skip_below_kb IS '低于该体积（KB）的 GIF 跳过 GIFSICLE';
COMMENT ON COLUMN public.video_image_quality_settings.gif_gifsicle_min_gain_ratio IS '最小收益比例，低于阈值保留原文件';

UPDATE public.video_image_quality_settings
SET gif_gifsicle_enabled = COALESCE(gif_gifsicle_enabled, TRUE),
    gif_gifsicle_level = LEAST(GREATEST(COALESCE(gif_gifsicle_level, 2), 1), 3),
    gif_gifsicle_skip_below_kb = LEAST(GREATEST(COALESCE(gif_gifsicle_skip_below_kb, 256), 0), 4096),
    gif_gifsicle_min_gain_ratio = LEAST(GREATEST(COALESCE(gif_gifsicle_min_gain_ratio, 0.03), 0), 0.50);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_image_quality_settings_gif_gifsicle_level'
    ) THEN
        ALTER TABLE public.video_image_quality_settings
            ADD CONSTRAINT chk_video_image_quality_settings_gif_gifsicle_level
            CHECK (gif_gifsicle_level BETWEEN 1 AND 3);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_image_quality_settings_gif_gifsicle_skip_below_kb'
    ) THEN
        ALTER TABLE public.video_image_quality_settings
            ADD CONSTRAINT chk_video_image_quality_settings_gif_gifsicle_skip_below_kb
            CHECK (gif_gifsicle_skip_below_kb BETWEEN 0 AND 4096);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_image_quality_settings_gif_gifsicle_min_gain_ratio'
    ) THEN
        ALTER TABLE public.video_image_quality_settings
            ADD CONSTRAINT chk_video_image_quality_settings_gif_gifsicle_min_gain_ratio
            CHECK (gif_gifsicle_min_gain_ratio >= 0 AND gif_gifsicle_min_gain_ratio <= 0.50);
    END IF;
END
$$;

COMMIT;

