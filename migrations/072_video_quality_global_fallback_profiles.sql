BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS gif_downshift_high_res_long_side_threshold INTEGER NOT NULL DEFAULT 1600,
    ADD COLUMN IF NOT EXISTS gif_downshift_early_duration_sec DOUBLE PRECISION NOT NULL DEFAULT 45,
    ADD COLUMN IF NOT EXISTS gif_downshift_early_long_side_threshold INTEGER NOT NULL DEFAULT 1800,
    ADD COLUMN IF NOT EXISTS gif_downshift_medium_fps_cap INTEGER NOT NULL DEFAULT 9,
    ADD COLUMN IF NOT EXISTS gif_downshift_medium_width_cap INTEGER NOT NULL DEFAULT 720,
    ADD COLUMN IF NOT EXISTS gif_downshift_medium_colors_cap INTEGER NOT NULL DEFAULT 128,
    ADD COLUMN IF NOT EXISTS gif_downshift_medium_duration_cap_sec DOUBLE PRECISION NOT NULL DEFAULT 2.2,
    ADD COLUMN IF NOT EXISTS gif_downshift_long_fps_cap INTEGER NOT NULL DEFAULT 8,
    ADD COLUMN IF NOT EXISTS gif_downshift_long_width_cap INTEGER NOT NULL DEFAULT 640,
    ADD COLUMN IF NOT EXISTS gif_downshift_long_colors_cap INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_downshift_long_duration_cap_sec DOUBLE PRECISION NOT NULL DEFAULT 2.0,
    ADD COLUMN IF NOT EXISTS gif_downshift_ultra_fps_cap INTEGER NOT NULL DEFAULT 8,
    ADD COLUMN IF NOT EXISTS gif_downshift_ultra_width_cap INTEGER NOT NULL DEFAULT 560,
    ADD COLUMN IF NOT EXISTS gif_downshift_ultra_colors_cap INTEGER NOT NULL DEFAULT 72,
    ADD COLUMN IF NOT EXISTS gif_downshift_ultra_duration_cap_sec DOUBLE PRECISION NOT NULL DEFAULT 1.8,
    ADD COLUMN IF NOT EXISTS gif_downshift_high_res_fps_cap INTEGER NOT NULL DEFAULT 9,
    ADD COLUMN IF NOT EXISTS gif_downshift_high_res_width_cap INTEGER NOT NULL DEFAULT 640,
    ADD COLUMN IF NOT EXISTS gif_downshift_high_res_colors_cap INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_downshift_high_res_duration_cap_sec DOUBLE PRECISION NOT NULL DEFAULT 2.1,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_fps_cap INTEGER NOT NULL DEFAULT 10,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_width_cap INTEGER NOT NULL DEFAULT 720,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_colors_cap INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_min_width INTEGER NOT NULL DEFAULT 360,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_ultra_fps_cap INTEGER NOT NULL DEFAULT 8,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_ultra_width_cap INTEGER NOT NULL DEFAULT 640,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_ultra_colors_cap INTEGER NOT NULL DEFAULT 64,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_fps_cap INTEGER NOT NULL DEFAULT 8,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_width_cap INTEGER NOT NULL DEFAULT 540,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_colors_cap INTEGER NOT NULL DEFAULT 64,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_min_width INTEGER NOT NULL DEFAULT 320,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_duration_trigger_sec DOUBLE PRECISION NOT NULL DEFAULT 2.0,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_duration_scale DOUBLE PRECISION NOT NULL DEFAULT 0.75,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_duration_min_sec DOUBLE PRECISION NOT NULL DEFAULT 1.4,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_fps_cap INTEGER NOT NULL DEFAULT 6,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_width_cap INTEGER NOT NULL DEFAULT 480,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_colors_cap INTEGER NOT NULL DEFAULT 48,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_min_width INTEGER NOT NULL DEFAULT 320,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_duration_min_sec DOUBLE PRECISION NOT NULL DEFAULT 1.2,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_duration_max_sec DOUBLE PRECISION NOT NULL DEFAULT 1.8;

COMMENT ON COLUMN ops.video_quality_settings.gif_downshift_high_res_long_side_threshold IS 'GIF 降档触发：高分辨率长边阈值';
COMMENT ON COLUMN ops.video_quality_settings.gif_downshift_early_duration_sec IS 'GIF 降档触发：提前降档时长阈值（秒）';
COMMENT ON COLUMN ops.video_quality_settings.gif_timeout_fallback_fps_cap IS 'GIF 超时回退第一层 FPS 上限';
COMMENT ON COLUMN ops.video_quality_settings.gif_timeout_emergency_fps_cap IS 'GIF 超时回退第二层 FPS 上限';
COMMENT ON COLUMN ops.video_quality_settings.gif_timeout_last_resort_fps_cap IS 'GIF 超时回退第三层 FPS 上限';

UPDATE ops.video_quality_settings
SET
    gif_downshift_high_res_long_side_threshold = LEAST(GREATEST(COALESCE(gif_downshift_high_res_long_side_threshold, 1600), 720), 4096),
    gif_downshift_early_duration_sec = LEAST(GREATEST(COALESCE(gif_downshift_early_duration_sec, 45), 10), 300),
    gif_downshift_early_long_side_threshold = LEAST(GREATEST(COALESCE(gif_downshift_early_long_side_threshold, 1800), COALESCE(gif_downshift_high_res_long_side_threshold, 1600)), 4096),

    gif_downshift_medium_fps_cap = LEAST(GREATEST(COALESCE(gif_downshift_medium_fps_cap, 9), 4), 30),
    gif_downshift_medium_width_cap = LEAST(GREATEST(COALESCE(gif_downshift_medium_width_cap, 720), 320), 1920),
    gif_downshift_medium_colors_cap = LEAST(GREATEST(COALESCE(gif_downshift_medium_colors_cap, 128), 16), 256),
    gif_downshift_medium_duration_cap_sec = LEAST(GREATEST(COALESCE(gif_downshift_medium_duration_cap_sec, 2.2), 1.0), 6.0),

    gif_downshift_long_fps_cap = LEAST(GREATEST(COALESCE(gif_downshift_long_fps_cap, 8), 4), COALESCE(gif_downshift_medium_fps_cap, 9)),
    gif_downshift_long_width_cap = LEAST(GREATEST(COALESCE(gif_downshift_long_width_cap, 640), 320), COALESCE(gif_downshift_medium_width_cap, 720)),
    gif_downshift_long_colors_cap = LEAST(GREATEST(COALESCE(gif_downshift_long_colors_cap, 96), 16), COALESCE(gif_downshift_medium_colors_cap, 128)),
    gif_downshift_long_duration_cap_sec = LEAST(GREATEST(COALESCE(gif_downshift_long_duration_cap_sec, 2.0), 0.8), COALESCE(gif_downshift_medium_duration_cap_sec, 2.2)),

    gif_downshift_ultra_fps_cap = LEAST(GREATEST(COALESCE(gif_downshift_ultra_fps_cap, 8), 4), COALESCE(gif_downshift_long_fps_cap, 8)),
    gif_downshift_ultra_width_cap = LEAST(GREATEST(COALESCE(gif_downshift_ultra_width_cap, 560), 320), COALESCE(gif_downshift_long_width_cap, 640)),
    gif_downshift_ultra_colors_cap = LEAST(GREATEST(COALESCE(gif_downshift_ultra_colors_cap, 72), 16), COALESCE(gif_downshift_long_colors_cap, 96)),
    gif_downshift_ultra_duration_cap_sec = LEAST(GREATEST(COALESCE(gif_downshift_ultra_duration_cap_sec, 1.8), 0.8), COALESCE(gif_downshift_long_duration_cap_sec, 2.0)),

    gif_downshift_high_res_fps_cap = LEAST(GREATEST(COALESCE(gif_downshift_high_res_fps_cap, 9), 4), COALESCE(gif_downshift_medium_fps_cap, 9)),
    gif_downshift_high_res_width_cap = LEAST(GREATEST(COALESCE(gif_downshift_high_res_width_cap, 640), 320), COALESCE(gif_downshift_medium_width_cap, 720)),
    gif_downshift_high_res_colors_cap = LEAST(GREATEST(COALESCE(gif_downshift_high_res_colors_cap, 96), 16), COALESCE(gif_downshift_medium_colors_cap, 128)),
    gif_downshift_high_res_duration_cap_sec = LEAST(GREATEST(COALESCE(gif_downshift_high_res_duration_cap_sec, 2.1), 0.8), COALESCE(gif_downshift_medium_duration_cap_sec, 2.2)),

    gif_timeout_fallback_fps_cap = LEAST(GREATEST(COALESCE(gif_timeout_fallback_fps_cap, 10), 4), 30),
    gif_timeout_fallback_width_cap = LEAST(GREATEST(COALESCE(gif_timeout_fallback_width_cap, 720), 320), 1920),
    gif_timeout_fallback_colors_cap = LEAST(GREATEST(COALESCE(gif_timeout_fallback_colors_cap, 96), 16), 256),
    gif_timeout_fallback_min_width = LEAST(GREATEST(COALESCE(gif_timeout_fallback_min_width, 360), 240), COALESCE(gif_timeout_fallback_width_cap, 720)),

    gif_timeout_fallback_ultra_fps_cap = LEAST(GREATEST(COALESCE(gif_timeout_fallback_ultra_fps_cap, 8), 4), COALESCE(gif_timeout_fallback_fps_cap, 10)),
    gif_timeout_fallback_ultra_width_cap = LEAST(GREATEST(COALESCE(gif_timeout_fallback_ultra_width_cap, 640), COALESCE(gif_timeout_fallback_min_width, 360)), COALESCE(gif_timeout_fallback_width_cap, 720)),
    gif_timeout_fallback_ultra_colors_cap = LEAST(GREATEST(COALESCE(gif_timeout_fallback_ultra_colors_cap, 64), 16), COALESCE(gif_timeout_fallback_colors_cap, 96)),

    gif_timeout_emergency_fps_cap = LEAST(GREATEST(COALESCE(gif_timeout_emergency_fps_cap, 8), 4), COALESCE(gif_timeout_fallback_fps_cap, 10)),
    gif_timeout_emergency_width_cap = LEAST(GREATEST(COALESCE(gif_timeout_emergency_width_cap, 540), 240), COALESCE(gif_timeout_fallback_width_cap, 720)),
    gif_timeout_emergency_colors_cap = LEAST(GREATEST(COALESCE(gif_timeout_emergency_colors_cap, 64), 16), COALESCE(gif_timeout_fallback_colors_cap, 96)),
    gif_timeout_emergency_min_width = LEAST(GREATEST(COALESCE(gif_timeout_emergency_min_width, 320), 240), COALESCE(gif_timeout_emergency_width_cap, 540)),
    gif_timeout_emergency_duration_trigger_sec = LEAST(GREATEST(COALESCE(gif_timeout_emergency_duration_trigger_sec, 2.0), 1.0), 6.0),
    gif_timeout_emergency_duration_scale = LEAST(GREATEST(COALESCE(gif_timeout_emergency_duration_scale, 0.75), 0.5), 1.0),
    gif_timeout_emergency_duration_min_sec = LEAST(GREATEST(COALESCE(gif_timeout_emergency_duration_min_sec, 1.4), 0.8), 4.0),

    gif_timeout_last_resort_fps_cap = LEAST(GREATEST(COALESCE(gif_timeout_last_resort_fps_cap, 6), 4), COALESCE(gif_timeout_emergency_fps_cap, 8)),
    gif_timeout_last_resort_width_cap = LEAST(GREATEST(COALESCE(gif_timeout_last_resort_width_cap, 480), 240), COALESCE(gif_timeout_emergency_width_cap, 540)),
    gif_timeout_last_resort_colors_cap = LEAST(GREATEST(COALESCE(gif_timeout_last_resort_colors_cap, 48), 16), COALESCE(gif_timeout_emergency_colors_cap, 64)),
    gif_timeout_last_resort_min_width = LEAST(GREATEST(COALESCE(gif_timeout_last_resort_min_width, 320), 240), COALESCE(gif_timeout_last_resort_width_cap, 480)),
    gif_timeout_last_resort_duration_min_sec = LEAST(GREATEST(COALESCE(gif_timeout_last_resort_duration_min_sec, 1.2), 0.6), 3.0),
    gif_timeout_last_resort_duration_max_sec = LEAST(GREATEST(COALESCE(gif_timeout_last_resort_duration_max_sec, 1.8), COALESCE(gif_timeout_last_resort_duration_min_sec, 1.2)), 4.0);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_downshift_relation'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_downshift_relation
            CHECK (
                gif_downshift_high_res_long_side_threshold <= gif_downshift_early_long_side_threshold
                AND gif_downshift_medium_fps_cap >= gif_downshift_long_fps_cap
                AND gif_downshift_long_fps_cap >= gif_downshift_ultra_fps_cap
                AND gif_downshift_medium_width_cap >= gif_downshift_long_width_cap
                AND gif_downshift_long_width_cap >= gif_downshift_ultra_width_cap
                AND gif_downshift_medium_colors_cap >= gif_downshift_long_colors_cap
                AND gif_downshift_long_colors_cap >= gif_downshift_ultra_colors_cap
                AND gif_downshift_medium_duration_cap_sec >= gif_downshift_long_duration_cap_sec
                AND gif_downshift_long_duration_cap_sec >= gif_downshift_ultra_duration_cap_sec
            );
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_timeout_profile_relation'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_timeout_profile_relation
            CHECK (
                gif_timeout_fallback_fps_cap >= gif_timeout_fallback_ultra_fps_cap
                AND gif_timeout_fallback_ultra_fps_cap >= gif_timeout_emergency_fps_cap
                AND gif_timeout_emergency_fps_cap >= gif_timeout_last_resort_fps_cap
                AND gif_timeout_fallback_width_cap >= gif_timeout_fallback_ultra_width_cap
                AND gif_timeout_fallback_ultra_width_cap >= gif_timeout_emergency_width_cap
                AND gif_timeout_emergency_width_cap >= gif_timeout_last_resort_width_cap
                AND gif_timeout_fallback_colors_cap >= gif_timeout_fallback_ultra_colors_cap
                AND gif_timeout_fallback_ultra_colors_cap >= gif_timeout_emergency_colors_cap
                AND gif_timeout_emergency_colors_cap >= gif_timeout_last_resort_colors_cap
                AND gif_timeout_last_resort_duration_max_sec >= gif_timeout_last_resort_duration_min_sec
            );
    END IF;
END
$$;

ALTER TABLE public.video_image_quality_settings
    ADD COLUMN IF NOT EXISTS gif_downshift_high_res_long_side_threshold INTEGER NOT NULL DEFAULT 1600,
    ADD COLUMN IF NOT EXISTS gif_downshift_early_duration_sec DOUBLE PRECISION NOT NULL DEFAULT 45,
    ADD COLUMN IF NOT EXISTS gif_downshift_early_long_side_threshold INTEGER NOT NULL DEFAULT 1800,
    ADD COLUMN IF NOT EXISTS gif_downshift_medium_fps_cap INTEGER NOT NULL DEFAULT 9,
    ADD COLUMN IF NOT EXISTS gif_downshift_medium_width_cap INTEGER NOT NULL DEFAULT 720,
    ADD COLUMN IF NOT EXISTS gif_downshift_medium_colors_cap INTEGER NOT NULL DEFAULT 128,
    ADD COLUMN IF NOT EXISTS gif_downshift_medium_duration_cap_sec DOUBLE PRECISION NOT NULL DEFAULT 2.2,
    ADD COLUMN IF NOT EXISTS gif_downshift_long_fps_cap INTEGER NOT NULL DEFAULT 8,
    ADD COLUMN IF NOT EXISTS gif_downshift_long_width_cap INTEGER NOT NULL DEFAULT 640,
    ADD COLUMN IF NOT EXISTS gif_downshift_long_colors_cap INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_downshift_long_duration_cap_sec DOUBLE PRECISION NOT NULL DEFAULT 2.0,
    ADD COLUMN IF NOT EXISTS gif_downshift_ultra_fps_cap INTEGER NOT NULL DEFAULT 8,
    ADD COLUMN IF NOT EXISTS gif_downshift_ultra_width_cap INTEGER NOT NULL DEFAULT 560,
    ADD COLUMN IF NOT EXISTS gif_downshift_ultra_colors_cap INTEGER NOT NULL DEFAULT 72,
    ADD COLUMN IF NOT EXISTS gif_downshift_ultra_duration_cap_sec DOUBLE PRECISION NOT NULL DEFAULT 1.8,
    ADD COLUMN IF NOT EXISTS gif_downshift_high_res_fps_cap INTEGER NOT NULL DEFAULT 9,
    ADD COLUMN IF NOT EXISTS gif_downshift_high_res_width_cap INTEGER NOT NULL DEFAULT 640,
    ADD COLUMN IF NOT EXISTS gif_downshift_high_res_colors_cap INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_downshift_high_res_duration_cap_sec DOUBLE PRECISION NOT NULL DEFAULT 2.1,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_fps_cap INTEGER NOT NULL DEFAULT 10,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_width_cap INTEGER NOT NULL DEFAULT 720,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_colors_cap INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_min_width INTEGER NOT NULL DEFAULT 360,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_ultra_fps_cap INTEGER NOT NULL DEFAULT 8,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_ultra_width_cap INTEGER NOT NULL DEFAULT 640,
    ADD COLUMN IF NOT EXISTS gif_timeout_fallback_ultra_colors_cap INTEGER NOT NULL DEFAULT 64,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_fps_cap INTEGER NOT NULL DEFAULT 8,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_width_cap INTEGER NOT NULL DEFAULT 540,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_colors_cap INTEGER NOT NULL DEFAULT 64,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_min_width INTEGER NOT NULL DEFAULT 320,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_duration_trigger_sec DOUBLE PRECISION NOT NULL DEFAULT 2.0,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_duration_scale DOUBLE PRECISION NOT NULL DEFAULT 0.75,
    ADD COLUMN IF NOT EXISTS gif_timeout_emergency_duration_min_sec DOUBLE PRECISION NOT NULL DEFAULT 1.4,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_fps_cap INTEGER NOT NULL DEFAULT 6,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_width_cap INTEGER NOT NULL DEFAULT 480,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_colors_cap INTEGER NOT NULL DEFAULT 48,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_min_width INTEGER NOT NULL DEFAULT 320,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_duration_min_sec DOUBLE PRECISION NOT NULL DEFAULT 1.2,
    ADD COLUMN IF NOT EXISTS gif_timeout_last_resort_duration_max_sec DOUBLE PRECISION NOT NULL DEFAULT 1.8;

COMMIT;
