BEGIN;

ALTER TABLE ops.site_footer_settings
    ADD COLUMN IF NOT EXISTS self_media_items JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Migrate legacy single-media config into the new list-based structure.
UPDATE ops.site_footer_settings
SET self_media_items = jsonb_build_array(
    jsonb_build_object(
        'key', 'qq',
        'name', 'QQ',
        'logo', COALESCE(NULLIF(TRIM(self_media_logo), ''), ''),
        'qr_code', COALESCE(NULLIF(TRIM(self_media_qr_code), ''), ''),
        'profile_link', '',
        'enabled', true,
        'sort', 1
    ),
    jsonb_build_object(
        'key', 'wechat',
        'name', '微信',
        'logo', '',
        'qr_code', '',
        'profile_link', '',
        'enabled', false,
        'sort', 2
    ),
    jsonb_build_object(
        'key', 'xiaohongshu',
        'name', '小红书',
        'logo', '',
        'qr_code', '',
        'profile_link', '',
        'enabled', false,
        'sort', 3
    )
)
WHERE (
    self_media_items IS NULL
    OR self_media_items = '[]'::jsonb
)
AND (
    COALESCE(TRIM(self_media_logo), '') <> ''
    OR COALESCE(TRIM(self_media_qr_code), '') <> ''
);

COMMENT ON COLUMN ops.site_footer_settings.self_media_items IS '底部自媒体配置列表(JSON): key/name/logo/qr_code/profile_link/enabled/sort';

COMMIT;
