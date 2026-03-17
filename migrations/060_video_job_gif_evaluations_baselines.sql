BEGIN;

CREATE TABLE IF NOT EXISTS archive.video_job_gif_evaluations (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    output_id BIGINT NULL,
    candidate_id BIGINT NULL REFERENCES archive.video_job_gif_candidates(id) ON DELETE SET NULL,
    window_start_ms INTEGER NOT NULL DEFAULT 0,
    window_end_ms INTEGER NOT NULL DEFAULT 0,
    emotion_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    clarity_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    motion_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    loop_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    efficiency_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    overall_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    feature_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_video_job_gif_evaluations_window CHECK (window_start_ms >= 0 AND window_end_ms >= window_start_ms),
    CONSTRAINT chk_video_job_gif_evaluations_dimension_range CHECK (
        emotion_score >= 0 AND emotion_score <= 1 AND
        clarity_score >= 0 AND clarity_score <= 1 AND
        motion_score >= 0 AND motion_score <= 1 AND
        loop_score >= 0 AND loop_score <= 1 AND
        efficiency_score >= 0 AND efficiency_score <= 1 AND
        overall_score >= 0 AND overall_score <= 1
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_video_job_gif_evaluations_job_output
    ON archive.video_job_gif_evaluations (job_id, output_id);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_evaluations_job_id
    ON archive.video_job_gif_evaluations (job_id, id DESC);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_evaluations_candidate_id
    ON archive.video_job_gif_evaluations (candidate_id)
    WHERE candidate_id IS NOT NULL;

COMMENT ON TABLE archive.video_job_gif_evaluations IS 'GIF结果级评测明细（五维评分+总分，支持质量回归）';
COMMENT ON COLUMN archive.video_job_gif_evaluations.feature_json IS '评测特征快照（候选特征、loop指标、尺寸效率、上下文来源）';

CREATE TABLE IF NOT EXISTS ops.video_job_gif_baselines (
    id BIGSERIAL PRIMARY KEY,
    baseline_date DATE NOT NULL,
    window_label VARCHAR(16) NOT NULL DEFAULT '1d',
    scope VARCHAR(32) NOT NULL DEFAULT 'all',
    requested_format VARCHAR(16) NOT NULL DEFAULT 'gif',
    sample_jobs BIGINT NOT NULL DEFAULT 0,
    done_jobs BIGINT NOT NULL DEFAULT 0,
    failed_jobs BIGINT NOT NULL DEFAULT 0,
    done_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
    failed_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
    sample_outputs BIGINT NOT NULL DEFAULT 0,
    avg_emotion_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_clarity_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_motion_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_loop_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_efficiency_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_overall_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_output_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_loop_closure DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_size_bytes DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uk_video_job_gif_baselines UNIQUE (baseline_date, window_label, scope, requested_format)
);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_baselines_date
    ON ops.video_job_gif_baselines (baseline_date DESC, requested_format, scope);

COMMENT ON TABLE ops.video_job_gif_baselines IS 'GIF基线聚合快照（按日窗口），用于回归对比与调参评估';

CREATE TABLE IF NOT EXISTS ops.video_job_gif_rerank_logs (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    candidate_id BIGINT NULL REFERENCES archive.video_job_gif_candidates(id) ON DELETE SET NULL,
    start_ms INTEGER NOT NULL DEFAULT 0,
    end_ms INTEGER NOT NULL DEFAULT 0,
    before_rank INTEGER NOT NULL DEFAULT 0,
    after_rank INTEGER NOT NULL DEFAULT 0,
    before_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    after_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    score_delta DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason VARCHAR(64) NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_video_job_gif_rerank_window CHECK (start_ms >= 0 AND end_ms >= start_ms)
);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_rerank_logs_job_id
    ON ops.video_job_gif_rerank_logs (job_id, id DESC);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_rerank_logs_user_id
    ON ops.video_job_gif_rerank_logs (user_id, created_at DESC);

COMMENT ON TABLE ops.video_job_gif_rerank_logs IS 'GIF候选重排日志（记录反馈重排前后排序变化，便于可解释与回溯）';

COMMIT;
