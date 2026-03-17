BEGIN;

CREATE TABLE IF NOT EXISTS archive.video_job_gif_ai_proposals (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL DEFAULT '',
    model VARCHAR(128) NOT NULL DEFAULT '',
    endpoint VARCHAR(255) NOT NULL DEFAULT '',
    prompt_version VARCHAR(64) NOT NULL DEFAULT '',
    proposal_rank INTEGER NOT NULL DEFAULT 0,
    start_sec DOUBLE PRECISION NOT NULL DEFAULT 0,
    end_sec DOUBLE PRECISION NOT NULL DEFAULT 0,
    duration_sec DOUBLE PRECISION NOT NULL DEFAULT 0,
    base_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    proposal_reason TEXT NOT NULL DEFAULT '',
    semantic_tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    expected_value_level VARCHAR(32) NOT NULL DEFAULT '',
    standalone_confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    loop_friendliness_hint DOUBLE PRECISION NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'proposed',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    raw_response JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_ai_proposals_job_id
    ON archive.video_job_gif_ai_proposals (job_id, proposal_rank ASC, id ASC);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_ai_proposals_user_id
    ON archive.video_job_gif_ai_proposals (user_id, created_at DESC);

COMMENT ON TABLE archive.video_job_gif_ai_proposals IS 'GIF AI阶段1提名结果（窗口方案+理由）';
COMMENT ON COLUMN archive.video_job_gif_ai_proposals.raw_response IS '模型原始响应快照（JSON）';

CREATE TABLE IF NOT EXISTS archive.video_job_gif_ai_reviews (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    output_id BIGINT NULL REFERENCES public.video_image_outputs(id) ON DELETE SET NULL,
    proposal_id BIGINT NULL REFERENCES archive.video_job_gif_ai_proposals(id) ON DELETE SET NULL,
    provider VARCHAR(32) NOT NULL DEFAULT '',
    model VARCHAR(128) NOT NULL DEFAULT '',
    endpoint VARCHAR(255) NOT NULL DEFAULT '',
    prompt_version VARCHAR(64) NOT NULL DEFAULT '',
    final_recommendation VARCHAR(32) NOT NULL DEFAULT '',
    semantic_verdict DOUBLE PRECISION NOT NULL DEFAULT 0,
    diagnostic_reason TEXT NOT NULL DEFAULT '',
    suggested_action TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    raw_response JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uk_video_job_gif_ai_reviews_job_output UNIQUE (job_id, output_id)
);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_ai_reviews_job_id
    ON archive.video_job_gif_ai_reviews (job_id, id DESC);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_ai_reviews_recommendation
    ON archive.video_job_gif_ai_reviews (final_recommendation, created_at DESC);

COMMENT ON TABLE archive.video_job_gif_ai_reviews IS 'GIF AI阶段3复审结果（逐output推荐与诊断）';
COMMENT ON COLUMN archive.video_job_gif_ai_reviews.raw_response IS '模型原始响应快照（JSON）';

COMMIT;

