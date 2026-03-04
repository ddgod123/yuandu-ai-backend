BEGIN;

CREATE TABLE IF NOT EXISTS ops.upload_tasks (
  id BIGSERIAL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  stage VARCHAR(32) NOT NULL,
  user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
  collection_id BIGINT REFERENCES archive.collections(id) ON DELETE SET NULL,
  category_id BIGINT REFERENCES taxonomy.categories(id) ON DELETE SET NULL,
  file_name VARCHAR(255) NOT NULL DEFAULT '',
  file_size BIGINT NOT NULL DEFAULT 0,
  input JSONB NOT NULL DEFAULT '{}'::jsonb,
  result JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_upload_tasks_kind ON ops.upload_tasks (kind);
CREATE INDEX IF NOT EXISTS idx_upload_tasks_status ON ops.upload_tasks (status);
CREATE INDEX IF NOT EXISTS idx_upload_tasks_stage ON ops.upload_tasks (stage);
CREATE INDEX IF NOT EXISTS idx_upload_tasks_user_id ON ops.upload_tasks (user_id);
CREATE INDEX IF NOT EXISTS idx_upload_tasks_collection_id ON ops.upload_tasks (collection_id);
CREATE INDEX IF NOT EXISTS idx_upload_tasks_started_at ON ops.upload_tasks (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_upload_tasks_finished_at ON ops.upload_tasks (finished_at DESC);

COMMENT ON TABLE ops.upload_tasks IS '上传任务中心任务记录';
COMMENT ON COLUMN ops.upload_tasks.kind IS '任务类型：import/append';
COMMENT ON COLUMN ops.upload_tasks.status IS '任务状态：running/success/failed';
COMMENT ON COLUMN ops.upload_tasks.stage IS '任务阶段：uploading/processing/done';
COMMENT ON COLUMN ops.upload_tasks.input IS '任务输入参数快照';
COMMENT ON COLUMN ops.upload_tasks.result IS '任务结果摘要';

COMMIT;
