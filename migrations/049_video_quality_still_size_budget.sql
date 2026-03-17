BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS jpg_target_size_kb INTEGER NOT NULL DEFAULT 512,
    ADD COLUMN IF NOT EXISTS png_target_size_kb INTEGER NOT NULL DEFAULT 1024;

COMMENT ON COLUMN ops.video_quality_settings.jpg_target_size_kb IS 'JPG 静态图目标体积（KB），用于体积优先模板自适应压缩';
COMMENT ON COLUMN ops.video_quality_settings.png_target_size_kb IS 'PNG 静态图目标体积（KB），用于体积优先模板自适应压缩';

UPDATE ops.video_quality_settings
SET jpg_target_size_kb = LEAST(GREATEST(COALESCE(jpg_target_size_kb, 512), 64), 10240),
    png_target_size_kb = LEAST(GREATEST(COALESCE(png_target_size_kb, 1024), 64), 10240)
WHERE id = 1;

COMMIT;
