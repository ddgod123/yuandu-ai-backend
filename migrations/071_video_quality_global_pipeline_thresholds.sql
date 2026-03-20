BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS gif_render_budget_normal_mult DOUBLE PRECISION NOT NULL DEFAULT 1.8,
    ADD COLUMN IF NOT EXISTS gif_render_budget_long_mult DOUBLE PRECISION NOT NULL DEFAULT 1.45,
    ADD COLUMN IF NOT EXISTS gif_render_budget_ultra_mult DOUBLE PRECISION NOT NULL DEFAULT 1.2,
    ADD COLUMN IF NOT EXISTS gif_pipeline_short_video_max_sec DOUBLE PRECISION NOT NULL DEFAULT 18,
    ADD COLUMN IF NOT EXISTS gif_pipeline_long_video_min_sec DOUBLE PRECISION NOT NULL DEFAULT 180,
    ADD COLUMN IF NOT EXISTS gif_pipeline_short_video_mode VARCHAR(16) NOT NULL DEFAULT 'light',
    ADD COLUMN IF NOT EXISTS gif_pipeline_default_mode VARCHAR(16) NOT NULL DEFAULT 'standard',
    ADD COLUMN IF NOT EXISTS gif_pipeline_long_video_mode VARCHAR(16) NOT NULL DEFAULT 'light',
    ADD COLUMN IF NOT EXISTS gif_pipeline_high_priority_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS gif_pipeline_high_priority_mode VARCHAR(16) NOT NULL DEFAULT 'hq',
    ADD COLUMN IF NOT EXISTS gif_duration_tier_medium_sec DOUBLE PRECISION NOT NULL DEFAULT 60,
    ADD COLUMN IF NOT EXISTS gif_duration_tier_long_sec DOUBLE PRECISION NOT NULL DEFAULT 120,
    ADD COLUMN IF NOT EXISTS gif_duration_tier_ultra_sec DOUBLE PRECISION NOT NULL DEFAULT 240,
    ADD COLUMN IF NOT EXISTS gif_segment_timeout_min_sec INTEGER NOT NULL DEFAULT 30,
    ADD COLUMN IF NOT EXISTS gif_segment_timeout_max_sec INTEGER NOT NULL DEFAULT 120,
    ADD COLUMN IF NOT EXISTS gif_segment_timeout_fallback_cap_sec INTEGER NOT NULL DEFAULT 60,
    ADD COLUMN IF NOT EXISTS gif_segment_timeout_emergency_cap_sec INTEGER NOT NULL DEFAULT 40,
    ADD COLUMN IF NOT EXISTS gif_segment_timeout_last_resort_cap_sec INTEGER NOT NULL DEFAULT 30;

COMMENT ON COLUMN ops.video_quality_settings.gif_pipeline_short_video_max_sec IS 'GIF pipeline 自动短视频阈值（秒，<=该值命中短视频分流）';
COMMENT ON COLUMN ops.video_quality_settings.gif_render_budget_normal_mult IS 'GIF 预算倍率（normal 档）';
COMMENT ON COLUMN ops.video_quality_settings.gif_render_budget_long_mult IS 'GIF 预算倍率（long 档）';
COMMENT ON COLUMN ops.video_quality_settings.gif_render_budget_ultra_mult IS 'GIF 预算倍率（ultra 档）';
COMMENT ON COLUMN ops.video_quality_settings.gif_pipeline_long_video_min_sec IS 'GIF pipeline 自动长视频阈值（秒，>=该值命中长视频分流）';
COMMENT ON COLUMN ops.video_quality_settings.gif_pipeline_short_video_mode IS 'GIF pipeline 短视频模式（light|standard|hq）';
COMMENT ON COLUMN ops.video_quality_settings.gif_pipeline_default_mode IS 'GIF pipeline 默认模式（light|standard|hq）';
COMMENT ON COLUMN ops.video_quality_settings.gif_pipeline_long_video_mode IS 'GIF pipeline 长视频模式（light|standard|hq）';
COMMENT ON COLUMN ops.video_quality_settings.gif_pipeline_high_priority_enabled IS '是否启用高优先级任务强制模式';
COMMENT ON COLUMN ops.video_quality_settings.gif_pipeline_high_priority_mode IS '高优先级任务模式（light|standard|hq）';
COMMENT ON COLUMN ops.video_quality_settings.gif_duration_tier_medium_sec IS 'GIF 时长档位阈值：中档（秒）';
COMMENT ON COLUMN ops.video_quality_settings.gif_duration_tier_long_sec IS 'GIF 时长档位阈值：长档（秒）';
COMMENT ON COLUMN ops.video_quality_settings.gif_duration_tier_ultra_sec IS 'GIF 时长档位阈值：超长档（秒）';
COMMENT ON COLUMN ops.video_quality_settings.gif_segment_timeout_min_sec IS 'GIF 单片段渲染最小时限（秒）';
COMMENT ON COLUMN ops.video_quality_settings.gif_segment_timeout_max_sec IS 'GIF 单片段渲染最大时限（秒）';
COMMENT ON COLUMN ops.video_quality_settings.gif_segment_timeout_fallback_cap_sec IS 'GIF 单片段超时后第一层回退时限上限（秒）';
COMMENT ON COLUMN ops.video_quality_settings.gif_segment_timeout_emergency_cap_sec IS 'GIF 单片段超时后第二层回退时限上限（秒）';
COMMENT ON COLUMN ops.video_quality_settings.gif_segment_timeout_last_resort_cap_sec IS 'GIF 单片段超时后第三层回退时限上限（秒）';

UPDATE ops.video_quality_settings
SET gif_render_budget_normal_mult = LEAST(GREATEST(COALESCE(gif_render_budget_normal_mult, 1.8), 1.0), 4.0),
    gif_render_budget_long_mult = LEAST(GREATEST(COALESCE(gif_render_budget_long_mult, 1.45), 0.8), 4.0),
    gif_render_budget_ultra_mult = LEAST(GREATEST(COALESCE(gif_render_budget_ultra_mult, 1.2), 0.5), 4.0),
    gif_pipeline_short_video_max_sec = LEAST(GREATEST(COALESCE(gif_pipeline_short_video_max_sec, 18), 3), 300),
    gif_pipeline_long_video_min_sec = LEAST(GREATEST(COALESCE(gif_pipeline_long_video_min_sec, 180), 4), 3600),
    gif_pipeline_short_video_mode = LOWER(TRIM(COALESCE(NULLIF(gif_pipeline_short_video_mode, ''), 'light'))),
    gif_pipeline_default_mode = LOWER(TRIM(COALESCE(NULLIF(gif_pipeline_default_mode, ''), 'standard'))),
    gif_pipeline_long_video_mode = LOWER(TRIM(COALESCE(NULLIF(gif_pipeline_long_video_mode, ''), 'light'))),
    gif_pipeline_high_priority_mode = LOWER(TRIM(COALESCE(NULLIF(gif_pipeline_high_priority_mode, ''), 'hq'))),
    gif_duration_tier_medium_sec = LEAST(GREATEST(COALESCE(gif_duration_tier_medium_sec, 60), 10), 600),
    gif_duration_tier_long_sec = LEAST(GREATEST(COALESCE(gif_duration_tier_long_sec, 120), 11), 1800),
    gif_duration_tier_ultra_sec = LEAST(GREATEST(COALESCE(gif_duration_tier_ultra_sec, 240), 12), 7200),
    gif_segment_timeout_min_sec = LEAST(GREATEST(COALESCE(gif_segment_timeout_min_sec, 30), 10), 300),
    gif_segment_timeout_max_sec = LEAST(GREATEST(COALESCE(gif_segment_timeout_max_sec, 120), 10), 600),
    gif_segment_timeout_fallback_cap_sec = LEAST(GREATEST(COALESCE(gif_segment_timeout_fallback_cap_sec, 60), 10), 600),
    gif_segment_timeout_emergency_cap_sec = LEAST(GREATEST(COALESCE(gif_segment_timeout_emergency_cap_sec, 40), 10), 600),
    gif_segment_timeout_last_resort_cap_sec = LEAST(GREATEST(COALESCE(gif_segment_timeout_last_resort_cap_sec, 30), 10), 600);

UPDATE ops.video_quality_settings
SET gif_render_budget_long_mult = LEAST(gif_render_budget_long_mult, gif_render_budget_normal_mult),
    gif_render_budget_ultra_mult = LEAST(gif_render_budget_ultra_mult, gif_render_budget_long_mult),
    gif_pipeline_long_video_min_sec = GREATEST(gif_pipeline_long_video_min_sec, gif_pipeline_short_video_max_sec + 1),
    gif_duration_tier_long_sec = GREATEST(gif_duration_tier_long_sec, gif_duration_tier_medium_sec + 1),
    gif_duration_tier_ultra_sec = GREATEST(gif_duration_tier_ultra_sec, gif_duration_tier_long_sec + 1),
    gif_segment_timeout_max_sec = GREATEST(gif_segment_timeout_max_sec, gif_segment_timeout_min_sec),
    gif_segment_timeout_fallback_cap_sec = LEAST(GREATEST(gif_segment_timeout_fallback_cap_sec, gif_segment_timeout_min_sec), gif_segment_timeout_max_sec),
    gif_segment_timeout_emergency_cap_sec = LEAST(GREATEST(gif_segment_timeout_emergency_cap_sec, gif_segment_timeout_min_sec), gif_segment_timeout_fallback_cap_sec),
    gif_segment_timeout_last_resort_cap_sec = LEAST(GREATEST(gif_segment_timeout_last_resort_cap_sec, gif_segment_timeout_min_sec), gif_segment_timeout_emergency_cap_sec);

UPDATE ops.video_quality_settings
SET gif_pipeline_short_video_mode = CASE WHEN gif_pipeline_short_video_mode IN ('light', 'standard', 'hq') THEN gif_pipeline_short_video_mode ELSE 'light' END,
    gif_pipeline_default_mode = CASE WHEN gif_pipeline_default_mode IN ('light', 'standard', 'hq') THEN gif_pipeline_default_mode ELSE 'standard' END,
    gif_pipeline_long_video_mode = CASE WHEN gif_pipeline_long_video_mode IN ('light', 'standard', 'hq') THEN gif_pipeline_long_video_mode ELSE 'light' END,
    gif_pipeline_high_priority_mode = CASE WHEN gif_pipeline_high_priority_mode IN ('light', 'standard', 'hq') THEN gif_pipeline_high_priority_mode ELSE 'hq' END;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_budget_multiplier_relation'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_budget_multiplier_relation
            CHECK (
                gif_render_budget_normal_mult >= gif_render_budget_long_mult
                AND gif_render_budget_long_mult >= gif_render_budget_ultra_mult
            );
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_pipeline_mode_values'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_pipeline_mode_values
            CHECK (
                gif_pipeline_short_video_mode IN ('light', 'standard', 'hq')
                AND gif_pipeline_default_mode IN ('light', 'standard', 'hq')
                AND gif_pipeline_long_video_mode IN ('light', 'standard', 'hq')
                AND gif_pipeline_high_priority_mode IN ('light', 'standard', 'hq')
            );
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_pipeline_duration_relation'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_pipeline_duration_relation
            CHECK (gif_pipeline_long_video_min_sec > gif_pipeline_short_video_max_sec);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_duration_tier_relation'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_duration_tier_relation
            CHECK (gif_duration_tier_medium_sec < gif_duration_tier_long_sec AND gif_duration_tier_long_sec < gif_duration_tier_ultra_sec);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_video_quality_settings_gif_timeout_relation'
    ) THEN
        ALTER TABLE ops.video_quality_settings
            ADD CONSTRAINT chk_video_quality_settings_gif_timeout_relation
            CHECK (
                gif_segment_timeout_min_sec <= gif_segment_timeout_last_resort_cap_sec
                AND gif_segment_timeout_last_resort_cap_sec <= gif_segment_timeout_emergency_cap_sec
                AND gif_segment_timeout_emergency_cap_sec <= gif_segment_timeout_fallback_cap_sec
                AND gif_segment_timeout_fallback_cap_sec <= gif_segment_timeout_max_sec
            );
    END IF;
END
$$;

ALTER TABLE public.video_image_quality_settings
    ADD COLUMN IF NOT EXISTS gif_render_budget_normal_mult DOUBLE PRECISION NOT NULL DEFAULT 1.8,
    ADD COLUMN IF NOT EXISTS gif_render_budget_long_mult DOUBLE PRECISION NOT NULL DEFAULT 1.45,
    ADD COLUMN IF NOT EXISTS gif_render_budget_ultra_mult DOUBLE PRECISION NOT NULL DEFAULT 1.2,
    ADD COLUMN IF NOT EXISTS gif_pipeline_short_video_max_sec DOUBLE PRECISION NOT NULL DEFAULT 18,
    ADD COLUMN IF NOT EXISTS gif_pipeline_long_video_min_sec DOUBLE PRECISION NOT NULL DEFAULT 180,
    ADD COLUMN IF NOT EXISTS gif_pipeline_short_video_mode VARCHAR(16) NOT NULL DEFAULT 'light',
    ADD COLUMN IF NOT EXISTS gif_pipeline_default_mode VARCHAR(16) NOT NULL DEFAULT 'standard',
    ADD COLUMN IF NOT EXISTS gif_pipeline_long_video_mode VARCHAR(16) NOT NULL DEFAULT 'light',
    ADD COLUMN IF NOT EXISTS gif_pipeline_high_priority_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS gif_pipeline_high_priority_mode VARCHAR(16) NOT NULL DEFAULT 'hq',
    ADD COLUMN IF NOT EXISTS gif_duration_tier_medium_sec DOUBLE PRECISION NOT NULL DEFAULT 60,
    ADD COLUMN IF NOT EXISTS gif_duration_tier_long_sec DOUBLE PRECISION NOT NULL DEFAULT 120,
    ADD COLUMN IF NOT EXISTS gif_duration_tier_ultra_sec DOUBLE PRECISION NOT NULL DEFAULT 240,
    ADD COLUMN IF NOT EXISTS gif_segment_timeout_min_sec INTEGER NOT NULL DEFAULT 30,
    ADD COLUMN IF NOT EXISTS gif_segment_timeout_max_sec INTEGER NOT NULL DEFAULT 120,
    ADD COLUMN IF NOT EXISTS gif_segment_timeout_fallback_cap_sec INTEGER NOT NULL DEFAULT 60,
    ADD COLUMN IF NOT EXISTS gif_segment_timeout_emergency_cap_sec INTEGER NOT NULL DEFAULT 40,
    ADD COLUMN IF NOT EXISTS gif_segment_timeout_last_resort_cap_sec INTEGER NOT NULL DEFAULT 30;

COMMENT ON COLUMN public.video_image_quality_settings.gif_pipeline_short_video_max_sec IS 'GIF pipeline 自动短视频阈值（秒）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_render_budget_normal_mult IS 'GIF 预算倍率（normal 档）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_render_budget_long_mult IS 'GIF 预算倍率（long 档）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_render_budget_ultra_mult IS 'GIF 预算倍率（ultra 档）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_pipeline_long_video_min_sec IS 'GIF pipeline 自动长视频阈值（秒）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_pipeline_short_video_mode IS 'GIF pipeline 短视频模式（light|standard|hq）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_pipeline_default_mode IS 'GIF pipeline 默认模式（light|standard|hq）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_pipeline_long_video_mode IS 'GIF pipeline 长视频模式（light|standard|hq）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_pipeline_high_priority_enabled IS '是否启用高优先级任务强制模式';
COMMENT ON COLUMN public.video_image_quality_settings.gif_pipeline_high_priority_mode IS '高优先级任务模式（light|standard|hq）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_duration_tier_medium_sec IS 'GIF 时长档位阈值：中档（秒）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_duration_tier_long_sec IS 'GIF 时长档位阈值：长档（秒）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_duration_tier_ultra_sec IS 'GIF 时长档位阈值：超长档（秒）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_segment_timeout_min_sec IS 'GIF 单片段渲染最小时限（秒）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_segment_timeout_max_sec IS 'GIF 单片段渲染最大时限（秒）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_segment_timeout_fallback_cap_sec IS 'GIF 单片段超时后第一层回退时限上限（秒）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_segment_timeout_emergency_cap_sec IS 'GIF 单片段超时后第二层回退时限上限（秒）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_segment_timeout_last_resort_cap_sec IS 'GIF 单片段超时后第三层回退时限上限（秒）';

COMMIT;
