BEGIN;

CREATE TABLE IF NOT EXISTS archive.external_accounts (
  id BIGSERIAL PRIMARY KEY,
  provider VARCHAR(32) NOT NULL,
  tenant_key VARCHAR(128) NOT NULL,
  open_id VARCHAR(128) NOT NULL,
  union_id VARCHAR(128) NOT NULL DEFAULT '',
  user_id BIGINT NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_external_accounts_status CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_external_accounts_provider_tenant_open
  ON archive.external_accounts (provider, tenant_key, open_id);

CREATE INDEX IF NOT EXISTS idx_external_accounts_user_id
  ON archive.external_accounts (user_id);

CREATE INDEX IF NOT EXISTS idx_external_accounts_union_id
  ON archive.external_accounts (provider, union_id);

CREATE TABLE IF NOT EXISTS archive.feishu_event_logs (
  id BIGSERIAL PRIMARY KEY,
  event_id VARCHAR(128) NOT NULL,
  event_type VARCHAR(128) NOT NULL,
  tenant_key VARCHAR(128) NOT NULL DEFAULT '',
  message_id VARCHAR(128) NOT NULL DEFAULT '',
  status VARCHAR(32) NOT NULL DEFAULT 'received',
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_feishu_event_logs_status CHECK (status IN ('received', 'queued', 'ignored', 'failed'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_feishu_event_logs_event_id
  ON archive.feishu_event_logs (event_id);

CREATE INDEX IF NOT EXISTS idx_feishu_event_logs_message_id
  ON archive.feishu_event_logs (message_id);

CREATE INDEX IF NOT EXISTS idx_feishu_event_logs_created_at
  ON archive.feishu_event_logs (created_at DESC);

CREATE TABLE IF NOT EXISTS archive.feishu_message_jobs (
  id BIGSERIAL PRIMARY KEY,
  tenant_key VARCHAR(128) NOT NULL,
  chat_id VARCHAR(128) NOT NULL,
  message_id VARCHAR(128) NOT NULL,
  message_type VARCHAR(32) NOT NULL,
  file_key VARCHAR(256) NOT NULL,
  file_name VARCHAR(512) NOT NULL DEFAULT '',
  open_id VARCHAR(128) NOT NULL,
  union_id VARCHAR(128) NOT NULL DEFAULT '',
  bind_code VARCHAR(32) NOT NULL DEFAULT '',
  user_id BIGINT NULL,
  video_job_id BIGINT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'queued',
  error_message TEXT NOT NULL DEFAULT '',
  notify_attempts INTEGER NOT NULL DEFAULT 0,
  notified_at TIMESTAMPTZ NULL,
  request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  result_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  finished_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_feishu_message_jobs_status CHECK (status IN ('queued', 'processing', 'retrying', 'waiting_bind', 'job_queued', 'done', 'failed'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_feishu_message_jobs_resource
  ON archive.feishu_message_jobs (tenant_key, message_id, file_key);

CREATE INDEX IF NOT EXISTS idx_feishu_message_jobs_open_id
  ON archive.feishu_message_jobs (tenant_key, open_id);

CREATE INDEX IF NOT EXISTS idx_feishu_message_jobs_status
  ON archive.feishu_message_jobs (status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_feishu_message_jobs_video_job
  ON archive.feishu_message_jobs (video_job_id);

CREATE TABLE IF NOT EXISTS archive.feishu_bind_codes (
  id BIGSERIAL PRIMARY KEY,
  code VARCHAR(32) NOT NULL,
  tenant_key VARCHAR(128) NOT NULL,
  chat_id VARCHAR(128) NOT NULL DEFAULT '',
  open_id VARCHAR(128) NOT NULL,
  union_id VARCHAR(128) NOT NULL DEFAULT '',
  user_id BIGINT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_feishu_bind_codes_status CHECK (status IN ('active', 'used', 'expired'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_feishu_bind_codes_code
  ON archive.feishu_bind_codes (code);

CREATE INDEX IF NOT EXISTS idx_feishu_bind_codes_lookup
  ON archive.feishu_bind_codes (tenant_key, open_id, status, expires_at DESC);

CREATE INDEX IF NOT EXISTS idx_feishu_bind_codes_user
  ON archive.feishu_bind_codes (user_id);

COMMENT ON TABLE archive.external_accounts IS '第三方账号绑定（包含飞书账号）';
COMMENT ON TABLE archive.feishu_event_logs IS '飞书事件回调去重与追踪';
COMMENT ON TABLE archive.feishu_message_jobs IS '飞书视频消息入站任务';
COMMENT ON TABLE archive.feishu_bind_codes IS '飞书账号绑定码';

COMMIT;
