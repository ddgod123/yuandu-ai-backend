BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS gif_motion_low_score_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.30,
    ADD COLUMN IF NOT EXISTS gif_motion_high_score_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.64,
    ADD COLUMN IF NOT EXISTS gif_motion_low_fps_delta INTEGER NOT NULL DEFAULT -2,
    ADD COLUMN IF NOT EXISTS gif_motion_high_fps_delta INTEGER NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS gif_adaptive_fps_min INTEGER NOT NULL DEFAULT 6,
    ADD COLUMN IF NOT EXISTS gif_adaptive_fps_max INTEGER NOT NULL DEFAULT 18,
    ADD COLUMN IF NOT EXISTS gif_width_size_low INTEGER NOT NULL DEFAULT 640,
    ADD COLUMN IF NOT EXISTS gif_width_size_medium INTEGER NOT NULL DEFAULT 720,
    ADD COLUMN IF NOT EXISTS gif_width_size_high INTEGER NOT NULL DEFAULT 768,
    ADD COLUMN IF NOT EXISTS gif_width_clarity_low INTEGER NOT NULL DEFAULT 720,
    ADD COLUMN IF NOT EXISTS gif_width_clarity_medium INTEGER NOT NULL DEFAULT 960,
    ADD COLUMN IF NOT EXISTS gif_width_clarity_high INTEGER NOT NULL DEFAULT 1080,
    ADD COLUMN IF NOT EXISTS gif_colors_size_low INTEGER NOT NULL DEFAULT 72,
    ADD COLUMN IF NOT EXISTS gif_colors_size_medium INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_colors_size_high INTEGER NOT NULL DEFAULT 128,
    ADD COLUMN IF NOT EXISTS gif_colors_clarity_low INTEGER NOT NULL DEFAULT 128,
    ADD COLUMN IF NOT EXISTS gif_colors_clarity_medium INTEGER NOT NULL DEFAULT 176,
    ADD COLUMN IF NOT EXISTS gif_colors_clarity_high INTEGER NOT NULL DEFAULT 224,
    ADD COLUMN IF NOT EXISTS gif_duration_low_sec DOUBLE PRECISION NOT NULL DEFAULT 2.0,
    ADD COLUMN IF NOT EXISTS gif_duration_medium_sec DOUBLE PRECISION NOT NULL DEFAULT 2.4,
    ADD COLUMN IF NOT EXISTS gif_duration_high_sec DOUBLE PRECISION NOT NULL DEFAULT 2.8,
    ADD COLUMN IF NOT EXISTS gif_duration_size_profile_max_sec DOUBLE PRECISION NOT NULL DEFAULT 2.4;

UPDATE ops.video_quality_settings
SET
    gif_motion_low_score_threshold = LEAST(GREATEST(COALESCE(gif_motion_low_score_threshold, 0.30), 0.0), 0.95),
    gif_motion_high_score_threshold = LEAST(GREATEST(COALESCE(gif_motion_high_score_threshold, 0.64), COALESCE(gif_motion_low_score_threshold, 0.30) + 0.01), 1.0),
    gif_motion_low_fps_delta = LEAST(GREATEST(COALESCE(gif_motion_low_fps_delta, -2), -12), 0),
    gif_motion_high_fps_delta = LEAST(GREATEST(COALESCE(gif_motion_high_fps_delta, 2), 0), 12),
    gif_adaptive_fps_min = LEAST(GREATEST(COALESCE(gif_adaptive_fps_min, 6), 2), 30),
    gif_adaptive_fps_max = LEAST(GREATEST(COALESCE(gif_adaptive_fps_max, 18), COALESCE(gif_adaptive_fps_min, 6)), 60),

    gif_width_size_low = LEAST(GREATEST(COALESCE(gif_width_size_low, 640), 320), 1920),
    gif_width_size_medium = LEAST(GREATEST(COALESCE(gif_width_size_medium, 720), COALESCE(gif_width_size_low, 640)), 1920),
    gif_width_size_high = LEAST(GREATEST(COALESCE(gif_width_size_high, 768), COALESCE(gif_width_size_medium, 720)), 1920),

    gif_width_clarity_low = LEAST(GREATEST(COALESCE(gif_width_clarity_low, 720), 320), 1920),
    gif_width_clarity_medium = LEAST(GREATEST(COALESCE(gif_width_clarity_medium, 960), COALESCE(gif_width_clarity_low, 720)), 1920),
    gif_width_clarity_high = LEAST(GREATEST(COALESCE(gif_width_clarity_high, 1080), COALESCE(gif_width_clarity_medium, 960)), 1920),

    gif_colors_size_low = LEAST(GREATEST(COALESCE(gif_colors_size_low, 72), 16), 256),
    gif_colors_size_medium = LEAST(GREATEST(COALESCE(gif_colors_size_medium, 96), COALESCE(gif_colors_size_low, 72)), 256),
    gif_colors_size_high = LEAST(GREATEST(COALESCE(gif_colors_size_high, 128), COALESCE(gif_colors_size_medium, 96)), 256),

    gif_colors_clarity_low = LEAST(GREATEST(COALESCE(gif_colors_clarity_low, 128), 16), 256),
    gif_colors_clarity_medium = LEAST(GREATEST(COALESCE(gif_colors_clarity_medium, 176), COALESCE(gif_colors_clarity_low, 128)), 256),
    gif_colors_clarity_high = LEAST(GREATEST(COALESCE(gif_colors_clarity_high, 224), COALESCE(gif_colors_clarity_medium, 176)), 256),

    gif_duration_low_sec = LEAST(GREATEST(COALESCE(gif_duration_low_sec, 2.0), 0.8), 6.0),
    gif_duration_medium_sec = LEAST(GREATEST(COALESCE(gif_duration_medium_sec, 2.4), COALESCE(gif_duration_low_sec, 2.0)), 6.0),
    gif_duration_high_sec = LEAST(GREATEST(COALESCE(gif_duration_high_sec, 2.8), COALESCE(gif_duration_medium_sec, 2.4)), 6.0),
    gif_duration_size_profile_max_sec = LEAST(GREATEST(COALESCE(gif_duration_size_profile_max_sec, 2.4), COALESCE(gif_duration_low_sec, 2.0)), COALESCE(gif_duration_high_sec, 2.8));

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_adaptive_profile_relation'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_adaptive_profile_relation
            CHECK (
                gif_motion_high_score_threshold > gif_motion_low_score_threshold
                AND gif_adaptive_fps_max >= gif_adaptive_fps_min
                AND gif_width_size_high >= gif_width_size_medium
                AND gif_width_size_medium >= gif_width_size_low
                AND gif_width_clarity_high >= gif_width_clarity_medium
                AND gif_width_clarity_medium >= gif_width_clarity_low
                AND gif_colors_size_high >= gif_colors_size_medium
                AND gif_colors_size_medium >= gif_colors_size_low
                AND gif_colors_clarity_high >= gif_colors_clarity_medium
                AND gif_colors_clarity_medium >= gif_colors_clarity_low
                AND gif_duration_high_sec >= gif_duration_medium_sec
                AND gif_duration_medium_sec >= gif_duration_low_sec
                AND gif_duration_size_profile_max_sec >= gif_duration_low_sec
                AND gif_duration_size_profile_max_sec <= gif_duration_high_sec
            );
    END IF;
END
$$;

ALTER TABLE public.video_image_quality_settings
    ADD COLUMN IF NOT EXISTS gif_motion_low_score_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.30,
    ADD COLUMN IF NOT EXISTS gif_motion_high_score_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.64,
    ADD COLUMN IF NOT EXISTS gif_motion_low_fps_delta INTEGER NOT NULL DEFAULT -2,
    ADD COLUMN IF NOT EXISTS gif_motion_high_fps_delta INTEGER NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS gif_adaptive_fps_min INTEGER NOT NULL DEFAULT 6,
    ADD COLUMN IF NOT EXISTS gif_adaptive_fps_max INTEGER NOT NULL DEFAULT 18,
    ADD COLUMN IF NOT EXISTS gif_width_size_low INTEGER NOT NULL DEFAULT 640,
    ADD COLUMN IF NOT EXISTS gif_width_size_medium INTEGER NOT NULL DEFAULT 720,
    ADD COLUMN IF NOT EXISTS gif_width_size_high INTEGER NOT NULL DEFAULT 768,
    ADD COLUMN IF NOT EXISTS gif_width_clarity_low INTEGER NOT NULL DEFAULT 720,
    ADD COLUMN IF NOT EXISTS gif_width_clarity_medium INTEGER NOT NULL DEFAULT 960,
    ADD COLUMN IF NOT EXISTS gif_width_clarity_high INTEGER NOT NULL DEFAULT 1080,
    ADD COLUMN IF NOT EXISTS gif_colors_size_low INTEGER NOT NULL DEFAULT 72,
    ADD COLUMN IF NOT EXISTS gif_colors_size_medium INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_colors_size_high INTEGER NOT NULL DEFAULT 128,
    ADD COLUMN IF NOT EXISTS gif_colors_clarity_low INTEGER NOT NULL DEFAULT 128,
    ADD COLUMN IF NOT EXISTS gif_colors_clarity_medium INTEGER NOT NULL DEFAULT 176,
    ADD COLUMN IF NOT EXISTS gif_colors_clarity_high INTEGER NOT NULL DEFAULT 224,
    ADD COLUMN IF NOT EXISTS gif_duration_low_sec DOUBLE PRECISION NOT NULL DEFAULT 2.0,
    ADD COLUMN IF NOT EXISTS gif_duration_medium_sec DOUBLE PRECISION NOT NULL DEFAULT 2.4,
    ADD COLUMN IF NOT EXISTS gif_duration_high_sec DOUBLE PRECISION NOT NULL DEFAULT 2.8,
    ADD COLUMN IF NOT EXISTS gif_duration_size_profile_max_sec DOUBLE PRECISION NOT NULL DEFAULT 2.4;

COMMIT;
