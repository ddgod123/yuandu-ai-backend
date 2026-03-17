BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS gif_target_size_kb INTEGER NOT NULL DEFAULT 2048,
    ADD COLUMN IF NOT EXISTS webp_target_size_kb INTEGER NOT NULL DEFAULT 1536,
    ADD COLUMN IF NOT EXISTS still_min_blur_score DOUBLE PRECISION NOT NULL DEFAULT 12,
    ADD COLUMN IF NOT EXISTS still_min_exposure_score DOUBLE PRECISION NOT NULL DEFAULT 0.28,
    ADD COLUMN IF NOT EXISTS still_min_width INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS still_min_height INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN ops.video_quality_settings.gif_target_size_kb IS 'GIF 动图目标体积（KB），用于自适应降档控制';
COMMENT ON COLUMN ops.video_quality_settings.webp_target_size_kb IS 'WebP 动图目标体积（KB），用于自适应降档控制';
COMMENT ON COLUMN ops.video_quality_settings.still_min_blur_score IS '静态图最小清晰度阈值（拉普拉斯方差）';
COMMENT ON COLUMN ops.video_quality_settings.still_min_exposure_score IS '静态图最小曝光评分阈值（0-1）';
COMMENT ON COLUMN ops.video_quality_settings.still_min_width IS '静态图最小宽度阈值（像素）';
COMMENT ON COLUMN ops.video_quality_settings.still_min_height IS '静态图最小高度阈值（像素）';

UPDATE ops.video_quality_settings
SET gif_target_size_kb = LEAST(GREATEST(COALESCE(gif_target_size_kb, 2048), 128), 10240),
    webp_target_size_kb = LEAST(GREATEST(COALESCE(webp_target_size_kb, 1536), 128), 10240),
    still_min_blur_score = LEAST(GREATEST(COALESCE(still_min_blur_score, 12), 0), 300),
    still_min_exposure_score = LEAST(GREATEST(COALESCE(still_min_exposure_score, 0.28), 0), 1),
    still_min_width = LEAST(GREATEST(COALESCE(still_min_width, 0), 0), 4096),
    still_min_height = LEAST(GREATEST(COALESCE(still_min_height, 0), 0), 4096)
WHERE id = 1;

COMMIT;
