BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS gif_render_retry_max_attempts INTEGER NOT NULL DEFAULT 6,
    ADD COLUMN IF NOT EXISTS gif_render_retry_primary_colors_floor INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_render_retry_primary_colors_step INTEGER NOT NULL DEFAULT 32,
    ADD COLUMN IF NOT EXISTS gif_render_retry_fps_floor INTEGER NOT NULL DEFAULT 8,
    ADD COLUMN IF NOT EXISTS gif_render_retry_fps_step INTEGER NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS gif_render_retry_width_trigger INTEGER NOT NULL DEFAULT 480,
    ADD COLUMN IF NOT EXISTS gif_render_retry_width_scale DOUBLE PRECISION NOT NULL DEFAULT 0.85,
    ADD COLUMN IF NOT EXISTS gif_render_retry_width_floor INTEGER NOT NULL DEFAULT 360,
    ADD COLUMN IF NOT EXISTS gif_render_retry_secondary_colors_floor INTEGER NOT NULL DEFAULT 48,
    ADD COLUMN IF NOT EXISTS gif_render_retry_secondary_colors_step INTEGER NOT NULL DEFAULT 16,
    ADD COLUMN IF NOT EXISTS gif_render_initial_size_fps_cap INTEGER NOT NULL DEFAULT 10,
    ADD COLUMN IF NOT EXISTS gif_render_initial_clarity_fps_floor INTEGER NOT NULL DEFAULT 12,
    ADD COLUMN IF NOT EXISTS gif_render_initial_size_colors_cap INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_render_initial_clarity_colors_floor INTEGER NOT NULL DEFAULT 160;

UPDATE ops.video_quality_settings
SET
    gif_render_retry_max_attempts = LEAST(GREATEST(COALESCE(gif_render_retry_max_attempts, 6), 1), 12),
    gif_render_retry_primary_colors_floor = LEAST(GREATEST(COALESCE(gif_render_retry_primary_colors_floor, 96), 16), 256),
    gif_render_retry_primary_colors_step = LEAST(GREATEST(COALESCE(gif_render_retry_primary_colors_step, 32), 1), 128),
    gif_render_retry_fps_floor = LEAST(GREATEST(COALESCE(gif_render_retry_fps_floor, 8), 2), 30),
    gif_render_retry_fps_step = LEAST(GREATEST(COALESCE(gif_render_retry_fps_step, 2), 1), 12),
    gif_render_retry_width_trigger = LEAST(GREATEST(COALESCE(gif_render_retry_width_trigger, 480), 240), 2048),
    gif_render_retry_width_scale = LEAST(GREATEST(COALESCE(gif_render_retry_width_scale, 0.85), 0.5), 0.98),
    gif_render_retry_width_floor = LEAST(GREATEST(COALESCE(gif_render_retry_width_floor, 360), 240), COALESCE(gif_render_retry_width_trigger, 480)),
    gif_render_retry_secondary_colors_floor = LEAST(GREATEST(COALESCE(gif_render_retry_secondary_colors_floor, 48), 16), COALESCE(gif_render_retry_primary_colors_floor, 96)),
    gif_render_retry_secondary_colors_step = LEAST(GREATEST(COALESCE(gif_render_retry_secondary_colors_step, 16), 1), 128),
    gif_render_initial_size_fps_cap = LEAST(GREATEST(COALESCE(gif_render_initial_size_fps_cap, 10), 2), 30),
    gif_render_initial_clarity_fps_floor = LEAST(GREATEST(COALESCE(gif_render_initial_clarity_fps_floor, 12), COALESCE(gif_render_initial_size_fps_cap, 10)), 30),
    gif_render_initial_size_colors_cap = LEAST(GREATEST(COALESCE(gif_render_initial_size_colors_cap, 96), 16), 256),
    gif_render_initial_clarity_colors_floor = LEAST(GREATEST(COALESCE(gif_render_initial_clarity_colors_floor, 160), COALESCE(gif_render_initial_size_colors_cap, 96)), 256);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_render_retry_relation'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_render_retry_relation
            CHECK (
                gif_render_retry_width_floor <= gif_render_retry_width_trigger
                AND gif_render_retry_secondary_colors_floor <= gif_render_retry_primary_colors_floor
                AND gif_render_initial_clarity_fps_floor >= gif_render_initial_size_fps_cap
                AND gif_render_initial_clarity_colors_floor >= gif_render_initial_size_colors_cap
            );
    END IF;
END
$$;

ALTER TABLE public.video_image_quality_settings
    ADD COLUMN IF NOT EXISTS gif_render_retry_max_attempts INTEGER NOT NULL DEFAULT 6,
    ADD COLUMN IF NOT EXISTS gif_render_retry_primary_colors_floor INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_render_retry_primary_colors_step INTEGER NOT NULL DEFAULT 32,
    ADD COLUMN IF NOT EXISTS gif_render_retry_fps_floor INTEGER NOT NULL DEFAULT 8,
    ADD COLUMN IF NOT EXISTS gif_render_retry_fps_step INTEGER NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS gif_render_retry_width_trigger INTEGER NOT NULL DEFAULT 480,
    ADD COLUMN IF NOT EXISTS gif_render_retry_width_scale DOUBLE PRECISION NOT NULL DEFAULT 0.85,
    ADD COLUMN IF NOT EXISTS gif_render_retry_width_floor INTEGER NOT NULL DEFAULT 360,
    ADD COLUMN IF NOT EXISTS gif_render_retry_secondary_colors_floor INTEGER NOT NULL DEFAULT 48,
    ADD COLUMN IF NOT EXISTS gif_render_retry_secondary_colors_step INTEGER NOT NULL DEFAULT 16,
    ADD COLUMN IF NOT EXISTS gif_render_initial_size_fps_cap INTEGER NOT NULL DEFAULT 10,
    ADD COLUMN IF NOT EXISTS gif_render_initial_clarity_fps_floor INTEGER NOT NULL DEFAULT 12,
    ADD COLUMN IF NOT EXISTS gif_render_initial_size_colors_cap INTEGER NOT NULL DEFAULT 96,
    ADD COLUMN IF NOT EXISTS gif_render_initial_clarity_colors_floor INTEGER NOT NULL DEFAULT 160;

COMMIT;
