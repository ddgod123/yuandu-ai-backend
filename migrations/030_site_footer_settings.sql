BEGIN;

CREATE TABLE IF NOT EXISTS ops.site_footer_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    site_name VARCHAR(128) NOT NULL DEFAULT '表情包档案馆',
    site_description TEXT NOT NULL DEFAULT '',
    contact_email VARCHAR(255) NOT NULL DEFAULT '',
    complaint_email VARCHAR(255) NOT NULL DEFAULT '',
    icp_number VARCHAR(128) NOT NULL DEFAULT '',
    icp_link VARCHAR(512) NOT NULL DEFAULT '',
    public_security_number VARCHAR(128) NOT NULL DEFAULT '',
    public_security_link VARCHAR(512) NOT NULL DEFAULT '',
    copyright_text VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO ops.site_footer_settings (
    id,
    site_name,
    site_description,
    contact_email,
    complaint_email,
    icp_number,
    icp_link,
    public_security_number,
    public_security_link,
    copyright_text
)
VALUES (
    1,
    '表情包档案馆',
    '致力于收集、整理和分享互联网表情包资源。本站提供合集浏览、下载与收藏功能，服务于个人非商业交流场景。',
    'contact@emoji-archive.com',
    'contact@emoji-archive.com',
    'ICP备案号：待补充',
    '',
    '公安备案号：待补充',
    '',
    '表情包档案馆. All rights reserved.'
)
ON CONFLICT (id) DO NOTHING;

COMMENT ON TABLE ops.site_footer_settings IS '官网底部展示信息配置（单例）';
COMMENT ON COLUMN ops.site_footer_settings.site_name IS '站点名称';
COMMENT ON COLUMN ops.site_footer_settings.site_description IS '站点描述';
COMMENT ON COLUMN ops.site_footer_settings.contact_email IS '通用联系邮箱';
COMMENT ON COLUMN ops.site_footer_settings.complaint_email IS '版权投诉邮箱';
COMMENT ON COLUMN ops.site_footer_settings.icp_number IS 'ICP备案号文案';
COMMENT ON COLUMN ops.site_footer_settings.icp_link IS 'ICP备案跳转链接';
COMMENT ON COLUMN ops.site_footer_settings.public_security_number IS '公安备案号文案';
COMMENT ON COLUMN ops.site_footer_settings.public_security_link IS '公安备案跳转链接';
COMMENT ON COLUMN ops.site_footer_settings.copyright_text IS '版权补充说明';

COMMIT;
