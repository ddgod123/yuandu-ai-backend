BEGIN;

CREATE TABLE IF NOT EXISTS ops.video_job_gif_manual_scores (
    id BIGSERIAL PRIMARY KEY,
    sample_id VARCHAR(64) NOT NULL,
    baseline_version VARCHAR(64) NOT NULL DEFAULT '',
    review_round VARCHAR(32) NOT NULL DEFAULT 'R1',
    reviewer VARCHAR(64) NOT NULL DEFAULT '',
    reviewed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    job_id BIGINT NULL REFERENCES archive.video_jobs(id) ON DELETE SET NULL,
    output_id BIGINT NULL REFERENCES public.video_image_outputs(id) ON DELETE SET NULL,
    emotion_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    clarity_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    motion_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    loop_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    efficiency_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    overall_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    is_top_pick BOOLEAN NOT NULL DEFAULT FALSE,
    is_pass BOOLEAN NOT NULL DEFAULT TRUE,
    reject_reason VARCHAR(64) NOT NULL DEFAULT '',
    review_notes TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_video_job_gif_manual_scores_range CHECK (
        emotion_score >= 0 AND emotion_score <= 1 AND
        clarity_score >= 0 AND clarity_score <= 1 AND
        motion_score >= 0 AND motion_score <= 1 AND
        loop_score >= 0 AND loop_score <= 1 AND
        efficiency_score >= 0 AND efficiency_score <= 1 AND
        overall_score >= 0 AND overall_score <= 1
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_video_job_gif_manual_scores_sample_reviewer
    ON ops.video_job_gif_manual_scores (sample_id, baseline_version, review_round, reviewer);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_manual_scores_output_id
    ON ops.video_job_gif_manual_scores (output_id)
    WHERE output_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_video_job_gif_manual_scores_reviewed_at
    ON ops.video_job_gif_manual_scores (reviewed_at DESC);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_manual_scores_sample_version
    ON ops.video_job_gif_manual_scores (sample_id, baseline_version, review_round);

COMMENT ON TABLE ops.video_job_gif_manual_scores IS 'GIF人工评分记录（与自动评测对比的基线样本评分）';
COMMENT ON COLUMN ops.video_job_gif_manual_scores.metadata IS '扩展上下文（场景标签、导入批次、来源等）';

COMMIT;
