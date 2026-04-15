BEGIN;

CREATE TABLE IF NOT EXISTS ops.upload_rule_settings (
  id SMALLINT PRIMARY KEY DEFAULT 1,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  auto_audit_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  auto_activate_on_pass BOOLEAN NOT NULL DEFAULT FALSE,
  allowed_extensions VARCHAR(255) NOT NULL DEFAULT 'jpg,jpeg,png,gif,webp',
  max_file_size_bytes BIGINT NOT NULL DEFAULT 10485760,
  max_files_per_collection INTEGER NOT NULL DEFAULT 50,
  max_files_per_request INTEGER NOT NULL DEFAULT 20,
  blocked_keywords TEXT NOT NULL DEFAULT '',
  content_rules TEXT NOT NULL DEFAULT '',
  reference_url VARCHAR(512) NOT NULL DEFAULT '',
  updated_by BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE ops.upload_rule_settings IS '用户上传规则配置（单例）';
COMMENT ON COLUMN ops.upload_rule_settings.enabled IS '是否允许用户上传';
COMMENT ON COLUMN ops.upload_rule_settings.auto_audit_enabled IS '是否启用自动审核流程';
COMMENT ON COLUMN ops.upload_rule_settings.auto_activate_on_pass IS '自动审核通过时是否直接转 active';
COMMENT ON COLUMN ops.upload_rule_settings.allowed_extensions IS '允许上传扩展名，逗号分隔';
COMMENT ON COLUMN ops.upload_rule_settings.max_file_size_bytes IS '单文件最大字节数';
COMMENT ON COLUMN ops.upload_rule_settings.max_files_per_collection IS '单合集最大文件数';
COMMENT ON COLUMN ops.upload_rule_settings.max_files_per_request IS '单次请求最大文件数';
COMMENT ON COLUMN ops.upload_rule_settings.blocked_keywords IS '命中即 pending 的关键词，逗号或换行分隔';
COMMENT ON COLUMN ops.upload_rule_settings.content_rules IS '上传内容规则文案（多行）';
COMMENT ON COLUMN ops.upload_rule_settings.reference_url IS '规则参考链接';

INSERT INTO ops.upload_rule_settings (
  id,
  enabled,
  auto_audit_enabled,
  auto_activate_on_pass,
  allowed_extensions,
  max_file_size_bytes,
  max_files_per_collection,
  max_files_per_request,
  blocked_keywords,
  content_rules,
  reference_url
) VALUES (
  1,
  TRUE,
  TRUE,
  FALSE,
  'jpg,jpeg,png,gif,webp',
  10485760,
  50,
  20,
  '涉政,色情,赌博,暴力,恐怖,诈骗,侵权,违法',
  E'不得上传违法违规、涉政极端、色情暴力、恐怖、诈骗等内容。\\n不得侵犯他人著作权、商标权、肖像权、隐私权等合法权益。\\n上传即默认你对素材拥有合法使用与传播授权。',
  'https://mos.m.taobao.com/iconfont/upload_rule?spm=a313x.icons_upload.i1.5.176b3a813scH6m'
)
ON CONFLICT (id) DO NOTHING;

COMMIT;
