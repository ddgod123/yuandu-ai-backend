BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS gif_profile VARCHAR(16) NOT NULL DEFAULT 'clarity',
    ADD COLUMN IF NOT EXISTS webp_profile VARCHAR(16) NOT NULL DEFAULT 'clarity',
    ADD COLUMN IF NOT EXISTS live_profile VARCHAR(16) NOT NULL DEFAULT 'clarity',
    ADD COLUMN IF NOT EXISTS jpg_profile VARCHAR(16) NOT NULL DEFAULT 'clarity',
    ADD COLUMN IF NOT EXISTS png_profile VARCHAR(16) NOT NULL DEFAULT 'clarity';

COMMENT ON COLUMN ops.video_quality_settings.gif_profile IS 'GIF 模板（clarity|size）';
COMMENT ON COLUMN ops.video_quality_settings.webp_profile IS 'WebP 模板（clarity|size）';
COMMENT ON COLUMN ops.video_quality_settings.live_profile IS 'Live 模板（clarity|size）';
COMMENT ON COLUMN ops.video_quality_settings.jpg_profile IS 'JPG 模板（clarity|size）';
COMMENT ON COLUMN ops.video_quality_settings.png_profile IS 'PNG 模板（clarity|size）';

UPDATE ops.video_quality_settings
SET gif_profile = COALESCE(NULLIF(TRIM(gif_profile), ''), 'clarity'),
    webp_profile = COALESCE(NULLIF(TRIM(webp_profile), ''), 'clarity'),
    live_profile = COALESCE(NULLIF(TRIM(live_profile), ''), 'clarity'),
    jpg_profile = COALESCE(NULLIF(TRIM(jpg_profile), ''), 'clarity'),
    png_profile = COALESCE(NULLIF(TRIM(png_profile), ''), 'clarity')
WHERE id = 1;

COMMIT;
