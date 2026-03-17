BEGIN;

CREATE TABLE IF NOT EXISTS ops.video_job_costs (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    status VARCHAR(32) NOT NULL DEFAULT '',
    cpu_ms BIGINT NOT NULL DEFAULT 0,
    gpu_ms BIGINT NOT NULL DEFAULT 0,
    asr_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
    ocr_frames INTEGER NOT NULL DEFAULT 0,
    storage_bytes_raw BIGINT NOT NULL DEFAULT 0,
    storage_bytes_output BIGINT NOT NULL DEFAULT 0,
    output_count INTEGER NOT NULL DEFAULT 0,
    estimated_cost NUMERIC(14,6) NOT NULL DEFAULT 0,
    currency VARCHAR(16) NOT NULL DEFAULT 'CNY',
    pricing_version VARCHAR(32) NOT NULL DEFAULT 'v1_lite',
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_video_job_costs_job_id UNIQUE (job_id)
);

CREATE INDEX IF NOT EXISTS idx_video_job_costs_user_id ON ops.video_job_costs(user_id);
CREATE INDEX IF NOT EXISTS idx_video_job_costs_status ON ops.video_job_costs(status);
CREATE INDEX IF NOT EXISTS idx_video_job_costs_created_at_desc ON ops.video_job_costs(created_at DESC);

COMMENT ON TABLE ops.video_job_costs IS '视频任务算力与成本核算快照';
COMMENT ON COLUMN ops.video_job_costs.estimated_cost IS '估算成本（货币单位由 currency 指定）';
COMMENT ON COLUMN ops.video_job_costs.details IS '成本拆分细节与上下文指标';

COMMIT;
