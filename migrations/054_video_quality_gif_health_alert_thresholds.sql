BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS gif_health_done_rate_warn DOUBLE PRECISION NOT NULL DEFAULT 0.85,
    ADD COLUMN IF NOT EXISTS gif_health_done_rate_critical DOUBLE PRECISION NOT NULL DEFAULT 0.60,
    ADD COLUMN IF NOT EXISTS gif_health_failed_rate_warn DOUBLE PRECISION NOT NULL DEFAULT 0.15,
    ADD COLUMN IF NOT EXISTS gif_health_failed_rate_critical DOUBLE PRECISION NOT NULL DEFAULT 0.30,
    ADD COLUMN IF NOT EXISTS gif_health_path_strict_rate_warn DOUBLE PRECISION NOT NULL DEFAULT 0.90,
    ADD COLUMN IF NOT EXISTS gif_health_path_strict_rate_critical DOUBLE PRECISION NOT NULL DEFAULT 0.50,
    ADD COLUMN IF NOT EXISTS gif_health_loop_fallback_rate_warn DOUBLE PRECISION NOT NULL DEFAULT 0.40,
    ADD COLUMN IF NOT EXISTS gif_health_loop_fallback_rate_critical DOUBLE PRECISION NOT NULL DEFAULT 0.70;

COMMENT ON COLUMN ops.video_quality_settings.gif_health_done_rate_warn IS 'GIF巡检告警阈值：任务完成率低于该值记为warn';
COMMENT ON COLUMN ops.video_quality_settings.gif_health_done_rate_critical IS 'GIF巡检告警阈值：任务完成率低于该值记为critical';
COMMENT ON COLUMN ops.video_quality_settings.gif_health_failed_rate_warn IS 'GIF巡检告警阈值：任务失败率高于该值记为warn';
COMMENT ON COLUMN ops.video_quality_settings.gif_health_failed_rate_critical IS 'GIF巡检告警阈值：任务失败率高于该值记为critical';
COMMENT ON COLUMN ops.video_quality_settings.gif_health_path_strict_rate_warn IS 'GIF巡检告警阈值：七牛新路径严格命中率低于该值记为warn';
COMMENT ON COLUMN ops.video_quality_settings.gif_health_path_strict_rate_critical IS 'GIF巡检告警阈值：七牛新路径严格命中率低于该值记为critical';
COMMENT ON COLUMN ops.video_quality_settings.gif_health_loop_fallback_rate_warn IS 'GIF巡检告警阈值：loop调优回退率高于该值记为warn';
COMMENT ON COLUMN ops.video_quality_settings.gif_health_loop_fallback_rate_critical IS 'GIF巡检告警阈值：loop调优回退率高于该值记为critical';

UPDATE ops.video_quality_settings
SET gif_health_done_rate_warn = LEAST(GREATEST(COALESCE(gif_health_done_rate_warn, 0.85), 0.50), 0.99),
    gif_health_done_rate_critical = LEAST(GREATEST(COALESCE(gif_health_done_rate_critical, 0.60), 0.01), 0.98),
    gif_health_failed_rate_warn = LEAST(GREATEST(COALESCE(gif_health_failed_rate_warn, 0.15), 0.01), 0.95),
    gif_health_failed_rate_critical = LEAST(GREATEST(COALESCE(gif_health_failed_rate_critical, 0.30), 0.02), 0.99),
    gif_health_path_strict_rate_warn = LEAST(GREATEST(COALESCE(gif_health_path_strict_rate_warn, 0.90), 0.50), 0.99),
    gif_health_path_strict_rate_critical = LEAST(GREATEST(COALESCE(gif_health_path_strict_rate_critical, 0.50), 0.01), 0.98),
    gif_health_loop_fallback_rate_warn = LEAST(GREATEST(COALESCE(gif_health_loop_fallback_rate_warn, 0.40), 0.01), 0.95),
    gif_health_loop_fallback_rate_critical = LEAST(GREATEST(COALESCE(gif_health_loop_fallback_rate_critical, 0.70), 0.02), 0.99)
WHERE id = 1;

UPDATE ops.video_quality_settings
SET gif_health_done_rate_critical = LEAST(gif_health_done_rate_critical, GREATEST(gif_health_done_rate_warn - 0.01, 0.01)),
    gif_health_failed_rate_critical = GREATEST(gif_health_failed_rate_critical, LEAST(gif_health_failed_rate_warn + 0.01, 0.99)),
    gif_health_path_strict_rate_critical = LEAST(gif_health_path_strict_rate_critical, GREATEST(gif_health_path_strict_rate_warn - 0.01, 0.01)),
    gif_health_loop_fallback_rate_critical = GREATEST(gif_health_loop_fallback_rate_critical, LEAST(gif_health_loop_fallback_rate_warn + 0.01, 0.99))
WHERE id = 1;

COMMIT;
