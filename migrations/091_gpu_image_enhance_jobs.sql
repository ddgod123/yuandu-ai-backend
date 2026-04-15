BEGIN;

CREATE TABLE IF NOT EXISTS archive.gpu_image_enhance_jobs (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  title VARCHAR(255) NOT NULL DEFAULT '',
  provider VARCHAR(64) NOT NULL DEFAULT '',
  model VARCHAR(128) NOT NULL DEFAULT '',
  scale INTEGER NOT NULL DEFAULT 4,
  source_object_key VARCHAR(768) NOT NULL,
  source_mime_type VARCHAR(128) NOT NULL DEFAULT '',
  source_size_bytes BIGINT NOT NULL DEFAULT 0,
  result_object_key VARCHAR(768) NOT NULL DEFAULT '',
  result_mime_type VARCHAR(128) NOT NULL DEFAULT '',
  result_size_bytes BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL DEFAULT 'queued',
  stage VARCHAR(32) NOT NULL DEFAULT 'queued',
  progress INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  callback_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  queued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  started_at TIMESTAMPTZ NULL,
  finished_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_gpu_image_enhance_jobs_status
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
  CONSTRAINT chk_gpu_image_enhance_jobs_stage
    CHECK (stage IN ('queued', 'running', 'uploading', 'callback', 'succeeded', 'failed', 'cancelled')),
  CONSTRAINT chk_gpu_image_enhance_jobs_scale
    CHECK (scale >= 1 AND scale <= 8),
  CONSTRAINT chk_gpu_image_enhance_jobs_progress
    CHECK (progress >= 0 AND progress <= 100)
);

CREATE INDEX IF NOT EXISTS idx_gpu_image_enhance_jobs_user_id
  ON archive.gpu_image_enhance_jobs (user_id);

CREATE INDEX IF NOT EXISTS idx_gpu_image_enhance_jobs_status
  ON archive.gpu_image_enhance_jobs (status);

CREATE INDEX IF NOT EXISTS idx_gpu_image_enhance_jobs_stage
  ON archive.gpu_image_enhance_jobs (stage);

CREATE INDEX IF NOT EXISTS idx_gpu_image_enhance_jobs_created_at_desc
  ON archive.gpu_image_enhance_jobs (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_gpu_image_enhance_jobs_source_object_key
  ON archive.gpu_image_enhance_jobs (source_object_key);

CREATE TABLE IF NOT EXISTS archive.gpu_image_enhance_assets (
  id BIGSERIAL PRIMARY KEY,
  job_id BIGINT NOT NULL REFERENCES archive.gpu_image_enhance_jobs (id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL,
  asset_role VARCHAR(32) NOT NULL,
  object_key VARCHAR(768) NOT NULL,
  mime_type VARCHAR(128) NOT NULL DEFAULT '',
  size_bytes BIGINT NOT NULL DEFAULT 0,
  width INTEGER NOT NULL DEFAULT 0,
  height INTEGER NOT NULL DEFAULT 0,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_gpu_image_enhance_assets_role
    CHECK (asset_role IN ('source', 'result'))
);

CREATE INDEX IF NOT EXISTS idx_gpu_image_enhance_assets_job_id
  ON archive.gpu_image_enhance_assets (job_id);

CREATE INDEX IF NOT EXISTS idx_gpu_image_enhance_assets_user_id
  ON archive.gpu_image_enhance_assets (user_id);

CREATE INDEX IF NOT EXISTS idx_gpu_image_enhance_assets_object_key
  ON archive.gpu_image_enhance_assets (object_key);

CREATE UNIQUE INDEX IF NOT EXISTS uk_gpu_image_enhance_assets_job_role
  ON archive.gpu_image_enhance_assets (job_id, asset_role);

COMMENT ON TABLE archive.gpu_image_enhance_jobs IS 'GPU 单图增强任务表';
COMMENT ON TABLE archive.gpu_image_enhance_assets IS 'GPU 单图增强任务产物表';
COMMENT ON COLUMN archive.gpu_image_enhance_jobs.source_object_key IS '源图对象 key（七牛云）';
COMMENT ON COLUMN archive.gpu_image_enhance_jobs.result_object_key IS '增强后对象 key（七牛云）';

COMMIT;
