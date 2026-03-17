BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS highlight_feedback_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS highlight_feedback_rollout_percent INTEGER NOT NULL DEFAULT 100,
    ADD COLUMN IF NOT EXISTS highlight_feedback_min_engaged_jobs INTEGER NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS highlight_feedback_min_weighted_signals DOUBLE PRECISION NOT NULL DEFAULT 6,
    ADD COLUMN IF NOT EXISTS highlight_feedback_boost_scale DOUBLE PRECISION NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS highlight_feedback_position_weight DOUBLE PRECISION NOT NULL DEFAULT 0.14,
    ADD COLUMN IF NOT EXISTS highlight_feedback_duration_weight DOUBLE PRECISION NOT NULL DEFAULT 0.08,
    ADD COLUMN IF NOT EXISTS highlight_feedback_reason_weight DOUBLE PRECISION NOT NULL DEFAULT 0.08;

COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_enabled IS '是否启用高光反馈重排';
COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_rollout_percent IS '高光反馈重排流量百分比（A/B）';
COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_min_engaged_jobs IS '触发反馈重排的最小有反馈任务数';
COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_min_weighted_signals IS '触发反馈重排的最小加权反馈信号';
COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_boost_scale IS '反馈重排总增强系数';
COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_position_weight IS '反馈重排位置偏好权重';
COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_duration_weight IS '反馈重排时长偏好权重';
COMMENT ON COLUMN ops.video_quality_settings.highlight_feedback_reason_weight IS '反馈重排片段来源权重';

UPDATE ops.video_quality_settings
SET highlight_feedback_enabled = COALESCE(highlight_feedback_enabled, TRUE),
    highlight_feedback_rollout_percent = LEAST(GREATEST(COALESCE(highlight_feedback_rollout_percent, 100), 0), 100),
    highlight_feedback_min_engaged_jobs = LEAST(GREATEST(COALESCE(highlight_feedback_min_engaged_jobs, 2), 1), 200),
    highlight_feedback_min_weighted_signals = LEAST(GREATEST(COALESCE(highlight_feedback_min_weighted_signals, 6), 0), 200),
    highlight_feedback_boost_scale = LEAST(GREATEST(COALESCE(highlight_feedback_boost_scale, 1), 0), 3),
    highlight_feedback_position_weight = LEAST(GREATEST(COALESCE(highlight_feedback_position_weight, 0.14), 0), 1),
    highlight_feedback_duration_weight = LEAST(GREATEST(COALESCE(highlight_feedback_duration_weight, 0.08), 0), 1),
    highlight_feedback_reason_weight = LEAST(GREATEST(COALESCE(highlight_feedback_reason_weight, 0.08), 0), 1)
WHERE id = 1;

COMMIT;
