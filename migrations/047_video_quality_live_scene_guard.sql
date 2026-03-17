BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS live_cover_scene_min_samples INTEGER NOT NULL DEFAULT 5,
    ADD COLUMN IF NOT EXISTS live_cover_guard_min_total INTEGER NOT NULL DEFAULT 20,
    ADD COLUMN IF NOT EXISTS live_cover_guard_score_floor DOUBLE PRECISION NOT NULL DEFAULT 0.58;

COMMENT ON COLUMN ops.video_quality_settings.live_cover_scene_min_samples IS 'Live 场景质量统计最小样本阈值（小于该值标记为低样本）';
COMMENT ON COLUMN ops.video_quality_settings.live_cover_guard_min_total IS 'Live 放量护栏启用所需的最小有效场景样本总数';
COMMENT ON COLUMN ops.video_quality_settings.live_cover_guard_score_floor IS 'Live 放量护栏封面均分底线（低于该值阻断放量）';

UPDATE ops.video_quality_settings
SET live_cover_scene_min_samples = LEAST(GREATEST(COALESCE(live_cover_scene_min_samples, 5), 1), 100),
    live_cover_guard_min_total = LEAST(GREATEST(COALESCE(live_cover_guard_min_total, 20), 1), 1000),
    live_cover_guard_score_floor = LEAST(GREATEST(COALESCE(live_cover_guard_score_floor, 0.58), 0.30), 0.95)
WHERE id = 1;

COMMIT;
