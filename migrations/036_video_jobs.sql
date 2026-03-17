BEGIN;

CREATE TABLE IF NOT EXISTS archive.video_jobs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL DEFAULT '',
    source_video_key VARCHAR(512) NOT NULL,
    category_id BIGINT REFERENCES taxonomy.categories(id) ON DELETE SET NULL,
    output_formats VARCHAR(128) NOT NULL DEFAULT 'jpg,gif',
    status VARCHAR(32) NOT NULL DEFAULT 'queued',
    stage VARCHAR(32) NOT NULL DEFAULT 'queued',
    progress INTEGER NOT NULL DEFAULT 0,
    priority VARCHAR(16) NOT NULL DEFAULT 'normal',
    options JSONB NOT NULL DEFAULT '{}'::jsonb,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT NOT NULL DEFAULT '',
    result_collection_id BIGINT REFERENCES archive.collections(id) ON DELETE SET NULL,
    queued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_jobs_user_id ON archive.video_jobs(user_id);
CREATE INDEX IF NOT EXISTS idx_video_jobs_status ON archive.video_jobs(status);
CREATE INDEX IF NOT EXISTS idx_video_jobs_stage ON archive.video_jobs(stage);
CREATE INDEX IF NOT EXISTS idx_video_jobs_category_id ON archive.video_jobs(category_id);
CREATE INDEX IF NOT EXISTS idx_video_jobs_priority ON archive.video_jobs(priority);
CREATE INDEX IF NOT EXISTS idx_video_jobs_result_collection_id ON archive.video_jobs(result_collection_id);
CREATE INDEX IF NOT EXISTS idx_video_jobs_created_at_desc ON archive.video_jobs(created_at DESC);

CREATE TABLE IF NOT EXISTS archive.video_job_artifacts (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    type VARCHAR(32) NOT NULL,
    qiniu_key VARCHAR(512) NOT NULL DEFAULT '',
    mime_type VARCHAR(128) NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_job_artifacts_job_id ON archive.video_job_artifacts(job_id);
CREATE INDEX IF NOT EXISTS idx_video_job_artifacts_type ON archive.video_job_artifacts(type);

CREATE TABLE IF NOT EXISTS archive.video_job_events (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    stage VARCHAR(32) NOT NULL DEFAULT '',
    level VARCHAR(16) NOT NULL DEFAULT 'info',
    message TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_job_events_job_id ON archive.video_job_events(job_id);
CREATE INDEX IF NOT EXISTS idx_video_job_events_stage ON archive.video_job_events(stage);
CREATE INDEX IF NOT EXISTS idx_video_job_events_created_at_desc ON archive.video_job_events(created_at DESC);

COMMENT ON TABLE archive.video_jobs IS '用户视频转表情包异步任务';
COMMENT ON COLUMN archive.video_jobs.source_video_key IS '源视频在对象存储中的 key';
COMMENT ON COLUMN archive.video_jobs.output_formats IS '输出格式列表，逗号分隔';
COMMENT ON COLUMN archive.video_jobs.options IS '任务可选参数（如 max_static、frame_interval）';
COMMENT ON COLUMN archive.video_jobs.metrics IS '处理过程指标与结果统计';

COMMENT ON TABLE archive.video_job_artifacts IS '视频任务产物索引（原视频/帧图/动图等）';
COMMENT ON TABLE archive.video_job_events IS '视频任务阶段事件日志';

COMMIT;
