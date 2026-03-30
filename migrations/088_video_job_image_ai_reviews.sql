BEGIN;

CREATE TABLE IF NOT EXISTS archive.video_job_image_ai_reviews (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    target_format VARCHAR(16) NOT NULL DEFAULT 'png',
    stage VARCHAR(32) NOT NULL DEFAULT 'ai3',
    recommendation VARCHAR(32) NOT NULL DEFAULT 'deliver',
    reviewed_outputs INTEGER NOT NULL DEFAULT 0,
    deliver_count INTEGER NOT NULL DEFAULT 0,
    reject_count INTEGER NOT NULL DEFAULT 0,
    manual_review_count INTEGER NOT NULL DEFAULT 0,
    hard_gate_reject_count INTEGER NOT NULL DEFAULT 0,
    hard_gate_manual_review_count INTEGER NOT NULL DEFAULT 0,
    candidate_budget INTEGER NOT NULL DEFAULT 0,
    effective_duration_sec DOUBLE PRECISION NOT NULL DEFAULT 0,
    quality_fallback BOOLEAN NOT NULL DEFAULT FALSE,
    quality_selector_version VARCHAR(64) NOT NULL DEFAULT '',
    summary_note TEXT NOT NULL DEFAULT '',
    summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uk_video_job_image_ai_reviews_job_format UNIQUE (job_id, target_format)
);

CREATE INDEX IF NOT EXISTS idx_video_job_image_ai_reviews_user_format_created
    ON archive.video_job_image_ai_reviews(user_id, target_format, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_video_job_image_ai_reviews_stage_recommendation
    ON archive.video_job_image_ai_reviews(stage, recommendation, created_at DESC);

COMMENT ON TABLE archive.video_job_image_ai_reviews IS '视频转图 AI3 质量复审摘要（按任务+格式）';
COMMENT ON COLUMN archive.video_job_image_ai_reviews.summary_json IS 'AI3复审摘要结构化JSON';
COMMENT ON COLUMN archive.video_job_image_ai_reviews.metadata IS '质量筛选统计与扩展元数据';

COMMIT;
