BEGIN;

ALTER TABLE ops.site_footer_settings
    ADD COLUMN IF NOT EXISTS self_media_logo VARCHAR(512) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS self_media_qr_code VARCHAR(512) NOT NULL DEFAULT '';

COMMENT ON COLUMN ops.site_footer_settings.self_media_logo IS '底部自媒体 logo（可存对象 key 或完整 URL）';
COMMENT ON COLUMN ops.site_footer_settings.self_media_qr_code IS '底部自媒体二维码（可存对象 key 或完整 URL）';

COMMIT;
