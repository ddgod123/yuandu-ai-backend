BEGIN;

CREATE TABLE IF NOT EXISTS ops.video_job_ai_usage (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    stage VARCHAR(32) NOT NULL DEFAULT '',
    provider VARCHAR(32) NOT NULL DEFAULT '',
    model VARCHAR(128) NOT NULL DEFAULT '',
    endpoint VARCHAR(255) NOT NULL DEFAULT '',
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cached_input_tokens BIGINT NOT NULL DEFAULT 0,
    image_tokens BIGINT NOT NULL DEFAULT 0,
    video_tokens BIGINT NOT NULL DEFAULT 0,
    audio_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
    request_duration_ms BIGINT NOT NULL DEFAULT 0,
    request_status VARCHAR(16) NOT NULL DEFAULT 'ok',
    request_error TEXT NOT NULL DEFAULT '',
    unit_price_input NUMERIC(18,8) NOT NULL DEFAULT 0,
    unit_price_output NUMERIC(18,8) NOT NULL DEFAULT 0,
    unit_price_cached_input NUMERIC(18,8) NOT NULL DEFAULT 0,
    unit_price_audio_min NUMERIC(18,8) NOT NULL DEFAULT 0,
    cost_usd NUMERIC(18,8) NOT NULL DEFAULT 0,
    currency VARCHAR(16) NOT NULL DEFAULT 'USD',
    pricing_version VARCHAR(64) NOT NULL DEFAULT '',
    pricing_source_url TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_job_ai_usage_job_id ON ops.video_job_ai_usage(job_id);
CREATE INDEX IF NOT EXISTS idx_video_job_ai_usage_user_id ON ops.video_job_ai_usage(user_id);
CREATE INDEX IF NOT EXISTS idx_video_job_ai_usage_stage ON ops.video_job_ai_usage(stage);
CREATE INDEX IF NOT EXISTS idx_video_job_ai_usage_created_at_desc ON ops.video_job_ai_usage(created_at DESC);

COMMENT ON TABLE ops.video_job_ai_usage IS '视频任务AI调用消耗明细（token/时长/费用）';
COMMENT ON COLUMN ops.video_job_ai_usage.stage IS '调用阶段：planner/judge/asr/fallback等';
COMMENT ON COLUMN ops.video_job_ai_usage.request_status IS '调用结果：ok/error/timeout等';
COMMENT ON COLUMN ops.video_job_ai_usage.cost_usd IS '单次调用估算费用（USD）';
COMMENT ON COLUMN ops.video_job_ai_usage.metadata IS '原始usage、请求上下文、扩展字段';

COMMIT;

