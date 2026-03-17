BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS gif_loop_tune_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS gif_loop_tune_min_enable_sec DOUBLE PRECISION NOT NULL DEFAULT 1.4,
    ADD COLUMN IF NOT EXISTS gif_loop_tune_min_improvement DOUBLE PRECISION NOT NULL DEFAULT 0.04,
    ADD COLUMN IF NOT EXISTS gif_loop_tune_motion_target DOUBLE PRECISION NOT NULL DEFAULT 0.22,
    ADD COLUMN IF NOT EXISTS gif_loop_tune_prefer_duration_sec DOUBLE PRECISION NOT NULL DEFAULT 2.4;

COMMENT ON COLUMN ops.video_quality_settings.gif_loop_tune_enabled IS '是否启用 GIF 循环闭合窗口调优';
COMMENT ON COLUMN ops.video_quality_settings.gif_loop_tune_min_enable_sec IS '启用 GIF 循环调优的最小窗口时长（秒）';
COMMENT ON COLUMN ops.video_quality_settings.gif_loop_tune_min_improvement IS 'GIF 循环调优应用所需的最小评分提升阈值';
COMMENT ON COLUMN ops.video_quality_settings.gif_loop_tune_motion_target IS 'GIF 循环调优的目标运动强度（哈希差分均值）';
COMMENT ON COLUMN ops.video_quality_settings.gif_loop_tune_prefer_duration_sec IS 'GIF 循环调优偏好时长（秒）';

UPDATE ops.video_quality_settings
SET gif_loop_tune_enabled = COALESCE(gif_loop_tune_enabled, TRUE),
    gif_loop_tune_min_enable_sec = LEAST(GREATEST(COALESCE(gif_loop_tune_min_enable_sec, 1.4), 0.8), 4.0),
    gif_loop_tune_min_improvement = LEAST(GREATEST(COALESCE(gif_loop_tune_min_improvement, 0.04), 0.005), 0.3),
    gif_loop_tune_motion_target = LEAST(GREATEST(COALESCE(gif_loop_tune_motion_target, 0.22), 0.05), 0.8),
    gif_loop_tune_prefer_duration_sec = LEAST(GREATEST(COALESCE(gif_loop_tune_prefer_duration_sec, 2.4), 1.0), 4.0)
WHERE id = 1;

COMMIT;
