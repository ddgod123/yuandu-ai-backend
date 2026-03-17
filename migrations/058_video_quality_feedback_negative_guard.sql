BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS highlight_feedback_negative_guard_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS highlight_feedback_negative_guard_dominance_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.45,
    ADD COLUMN IF NOT EXISTS highlight_feedback_negative_guard_min_weight DOUBLE PRECISION NOT NULL DEFAULT 4,
    ADD COLUMN IF NOT EXISTS highlight_feedback_negative_guard_penalty_scale DOUBLE PRECISION NOT NULL DEFAULT 0.55,
    ADD COLUMN IF NOT EXISTS highlight_feedback_negative_guard_penalty_weight DOUBLE PRECISION NOT NULL DEFAULT 0.9;

COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_negative_guard_enabled
    IS '是否启用负反馈保护（dislike 等）';
COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_negative_guard_dominance_threshold
    IS '负反馈占比阈值（超过后触发原因级惩罚）';
COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_negative_guard_min_weight
    IS '触发负反馈保护的最小负向权重（用于置信度饱和）';
COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_negative_guard_penalty_scale
    IS '负反馈乘性降权强度（0-1）';
COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_negative_guard_penalty_weight
    IS '负反馈加性惩罚强度（0-2）';

UPDATE ops.video_quality_settings
SET highlight_feedback_negative_guard_enabled = COALESCE(highlight_feedback_negative_guard_enabled, TRUE),
    highlight_feedback_negative_guard_dominance_threshold = LEAST(
        GREATEST(COALESCE(NULLIF(highlight_feedback_negative_guard_dominance_threshold, 0), 0.45), 0.2),
        0.95
    ),
    highlight_feedback_negative_guard_min_weight = LEAST(
        GREATEST(COALESCE(NULLIF(highlight_feedback_negative_guard_min_weight, 0), 4), 0.5),
        20
    ),
    highlight_feedback_negative_guard_penalty_scale = LEAST(
        GREATEST(COALESCE(NULLIF(highlight_feedback_negative_guard_penalty_scale, 0), 0.55), 0),
        1
    ),
    highlight_feedback_negative_guard_penalty_weight = LEAST(
        GREATEST(COALESCE(NULLIF(highlight_feedback_negative_guard_penalty_weight, 0), 0.9), 0),
        2
    )
WHERE id = 1;

ALTER TABLE public.video_image_quality_settings
    ADD COLUMN IF NOT EXISTS highlight_feedback_negative_guard_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS highlight_feedback_negative_guard_dominance_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.45,
    ADD COLUMN IF NOT EXISTS highlight_feedback_negative_guard_min_weight DOUBLE PRECISION NOT NULL DEFAULT 4,
    ADD COLUMN IF NOT EXISTS highlight_feedback_negative_guard_penalty_scale DOUBLE PRECISION NOT NULL DEFAULT 0.55,
    ADD COLUMN IF NOT EXISTS highlight_feedback_negative_guard_penalty_weight DOUBLE PRECISION NOT NULL DEFAULT 0.9;

COMMENT ON COLUMN public.video_image_quality_settings.highlight_feedback_negative_guard_enabled
    IS '是否启用负反馈保护（dislike 等）';
COMMENT ON COLUMN public.video_image_quality_settings.highlight_feedback_negative_guard_dominance_threshold
    IS '负反馈占比阈值（超过后触发原因级惩罚）';
COMMENT ON COLUMN public.video_image_quality_settings.highlight_feedback_negative_guard_min_weight
    IS '触发负反馈保护的最小负向权重（用于置信度饱和）';
COMMENT ON COLUMN public.video_image_quality_settings.highlight_feedback_negative_guard_penalty_scale
    IS '负反馈乘性降权强度（0-1）';
COMMENT ON COLUMN public.video_image_quality_settings.highlight_feedback_negative_guard_penalty_weight
    IS '负反馈加性惩罚强度（0-2）';

UPDATE public.video_image_quality_settings
SET highlight_feedback_negative_guard_enabled = COALESCE(highlight_feedback_negative_guard_enabled, TRUE),
    highlight_feedback_negative_guard_dominance_threshold = LEAST(
        GREATEST(COALESCE(NULLIF(highlight_feedback_negative_guard_dominance_threshold, 0), 0.45), 0.2),
        0.95
    ),
    highlight_feedback_negative_guard_min_weight = LEAST(
        GREATEST(COALESCE(NULLIF(highlight_feedback_negative_guard_min_weight, 0), 4), 0.5),
        20
    ),
    highlight_feedback_negative_guard_penalty_scale = LEAST(
        GREATEST(COALESCE(NULLIF(highlight_feedback_negative_guard_penalty_scale, 0), 0.55), 0),
        1
    ),
    highlight_feedback_negative_guard_penalty_weight = LEAST(
        GREATEST(COALESCE(NULLIF(highlight_feedback_negative_guard_penalty_weight, 0), 0.9), 0),
        2
    )
WHERE id = 1;

COMMIT;
