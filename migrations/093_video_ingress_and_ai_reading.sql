-- 093_video_ingress_and_ai_reading.sql
-- 补齐 AI1 plan 表缺失，并新增统一入站任务表 + 视频语义阅读结果表。

CREATE TABLE IF NOT EXISTS archive.video_job_ai1_plans (
  id BIGSERIAL PRIMARY KEY,
  job_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  requested_format VARCHAR(16) NOT NULL DEFAULT 'gif',
  schema_version VARCHAR(64) NOT NULL DEFAULT '',
  status VARCHAR(32) NOT NULL DEFAULT 'draft',
  source_prompt TEXT NOT NULL DEFAULT '',
  plan_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  model_provider VARCHAR(32) NOT NULL DEFAULT '',
  model_name VARCHAR(128) NOT NULL DEFAULT '',
  prompt_version VARCHAR(64) NOT NULL DEFAULT '',
  fallback_used BOOLEAN NOT NULL DEFAULT FALSE,
  confirmed_by_user BOOLEAN NOT NULL DEFAULT FALSE,
  confirmed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT uk_video_job_ai1_plans_job UNIQUE (job_id)
);

CREATE INDEX IF NOT EXISTS idx_video_job_ai1_plans_user ON archive.video_job_ai1_plans (user_id);
CREATE INDEX IF NOT EXISTS idx_video_job_ai1_plans_format_status ON archive.video_job_ai1_plans (requested_format, status);
CREATE INDEX IF NOT EXISTS idx_video_job_ai1_plans_confirmed_at ON archive.video_job_ai1_plans (confirmed_at);

CREATE TABLE IF NOT EXISTS archive.video_ingress_jobs (
  id BIGSERIAL PRIMARY KEY,
  provider VARCHAR(32) NOT NULL,
  tenant_key VARCHAR(128) NOT NULL DEFAULT '',
  channel VARCHAR(64) NOT NULL DEFAULT '',
  chat_id VARCHAR(128) NOT NULL DEFAULT '',
  session_id VARCHAR(128) NOT NULL DEFAULT '',
  external_user_id VARCHAR(128) NOT NULL DEFAULT '',
  external_open_id VARCHAR(128) NOT NULL DEFAULT '',
  external_union_id VARCHAR(128) NOT NULL DEFAULT '',
  bound_user_id BIGINT NULL,
  source_message_id VARCHAR(128) NOT NULL DEFAULT '',
  source_resource_key VARCHAR(256) NOT NULL DEFAULT '',
  source_video_key VARCHAR(512) NOT NULL DEFAULT '',
  source_video_url TEXT NOT NULL DEFAULT '',
  source_file_name VARCHAR(512) NOT NULL DEFAULT '',
  source_size_bytes BIGINT NOT NULL DEFAULT 0,
  video_job_id BIGINT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'queued',
  error_message TEXT NOT NULL DEFAULT '',
  request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  result_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  finished_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_video_ingress_provider CHECK (provider IN ('web', 'feishu', 'qq', 'wecom')),
  CONSTRAINT chk_video_ingress_status CHECK (status IN ('queued', 'processing', 'waiting_bind', 'job_queued', 'done', 'failed'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_video_ingress_provider_tenant_message_resource
  ON archive.video_ingress_jobs (provider, tenant_key, source_message_id, source_resource_key);
CREATE INDEX IF NOT EXISTS idx_video_ingress_bound_user ON archive.video_ingress_jobs (bound_user_id);
CREATE INDEX IF NOT EXISTS idx_video_ingress_video_job ON archive.video_ingress_jobs (video_job_id);
CREATE INDEX IF NOT EXISTS idx_video_ingress_status_updated ON archive.video_ingress_jobs (status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_video_ingress_provider_created ON archive.video_ingress_jobs (provider, created_at DESC);

CREATE TABLE IF NOT EXISTS archive.video_job_ai_readings (
  id BIGSERIAL PRIMARY KEY,
  job_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'queued',
  summary_text TEXT NOT NULL DEFAULT '',
  highlights_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  model_provider VARCHAR(32) NOT NULL DEFAULT '',
  model_name VARCHAR(128) NOT NULL DEFAULT '',
  prompt_version VARCHAR(64) NOT NULL DEFAULT '',
  request_duration_ms BIGINT NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  finished_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT uk_video_job_ai_readings_job UNIQUE (job_id),
  CONSTRAINT chk_video_job_ai_readings_status CHECK (status IN ('queued', 'processing', 'done', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_video_job_ai_readings_user ON archive.video_job_ai_readings (user_id);
CREATE INDEX IF NOT EXISTS idx_video_job_ai_readings_status_updated ON archive.video_job_ai_readings (status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_video_job_ai_readings_finished_at ON archive.video_job_ai_readings (finished_at DESC);

COMMENT ON TABLE archive.video_ingress_jobs IS '多入口视频入站任务（web/飞书/QQ/企微统一）';
COMMENT ON TABLE archive.video_job_ai_readings IS '视频异步语义阅读结果（图文链路中的文）';
