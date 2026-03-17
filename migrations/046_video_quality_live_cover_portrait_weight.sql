BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS live_cover_portrait_weight DOUBLE PRECISION NOT NULL DEFAULT 0.04;

COMMENT ON COLUMN ops.video_quality_settings.live_cover_portrait_weight IS 'Live 封面评分中人像提示分权重（0-0.25）';

UPDATE ops.video_quality_settings
SET live_cover_portrait_weight = LEAST(GREATEST(COALESCE(live_cover_portrait_weight, 0.04), 0.01), 0.25)
WHERE id = 1;

COMMIT;
