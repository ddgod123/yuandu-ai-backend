BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS quality_analysis_workers INTEGER NOT NULL DEFAULT 4,
    ADD COLUMN IF NOT EXISTS upload_concurrency INTEGER NOT NULL DEFAULT 4;

COMMENT ON COLUMN ops.video_quality_settings.quality_analysis_workers IS '质量分析并发 worker 数';
COMMENT ON COLUMN ops.video_quality_settings.upload_concurrency IS '静态结果上传并发数';

UPDATE ops.video_quality_settings
SET quality_analysis_workers = COALESCE(NULLIF(quality_analysis_workers, 0), 4),
    upload_concurrency = COALESCE(NULLIF(upload_concurrency, 0), 4)
WHERE id = 1;

COMMIT;
