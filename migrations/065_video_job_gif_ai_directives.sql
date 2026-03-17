BEGIN;

CREATE TABLE IF NOT EXISTS archive.video_job_gif_ai_directives (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL DEFAULT '',
    model VARCHAR(128) NOT NULL DEFAULT '',
    endpoint VARCHAR(255) NOT NULL DEFAULT '',
    prompt_version VARCHAR(64) NOT NULL DEFAULT '',
    business_goal VARCHAR(64) NOT NULL DEFAULT '',
    audience VARCHAR(128) NOT NULL DEFAULT '',
    must_capture JSONB NOT NULL DEFAULT '[]'::jsonb,
    avoid JSONB NOT NULL DEFAULT '[]'::jsonb,
    clip_count_min INTEGER NOT NULL DEFAULT 0,
    clip_count_max INTEGER NOT NULL DEFAULT 0,
    duration_pref_min_sec DOUBLE PRECISION NOT NULL DEFAULT 0,
    duration_pref_max_sec DOUBLE PRECISION NOT NULL DEFAULT 0,
    loop_preference DOUBLE PRECISION NOT NULL DEFAULT 0,
    quality_weights JSONB NOT NULL DEFAULT '{}'::jsonb,
    directive_text TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    raw_response JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_ai_directives_job_id
    ON archive.video_job_gif_ai_directives (job_id, id DESC);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_ai_directives_user_id
    ON archive.video_job_gif_ai_directives (user_id, created_at DESC);

COMMENT ON TABLE archive.video_job_gif_ai_directives IS 'GIF AI阶段0 指令层（Prompt Director）';
COMMENT ON COLUMN archive.video_job_gif_ai_directives.quality_weights IS '语义/清晰度/loop/效率等质量权重';
COMMENT ON COLUMN archive.video_job_gif_ai_directives.raw_response IS '模型原始响应快照（JSON）';

COMMIT;

