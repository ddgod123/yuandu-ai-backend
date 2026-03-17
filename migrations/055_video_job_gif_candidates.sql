BEGIN;

CREATE TABLE IF NOT EXISTS archive.video_job_gif_candidates (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    start_ms INTEGER NOT NULL DEFAULT 0,
    end_ms INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    base_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    confidence_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    final_rank INTEGER NOT NULL DEFAULT 0,
    is_selected BOOLEAN NOT NULL DEFAULT FALSE,
    reject_reason VARCHAR(64) NOT NULL DEFAULT '',
    feature_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_video_job_gif_candidates_window CHECK (start_ms >= 0 AND end_ms >= 0 AND end_ms >= start_ms),
    CONSTRAINT chk_video_job_gif_candidates_duration CHECK (duration_ms >= 0),
    CONSTRAINT chk_video_job_gif_candidates_scores CHECK (base_score >= 0 AND confidence_score >= 0)
);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_candidates_job_id
    ON archive.video_job_gif_candidates (job_id, id DESC);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_candidates_selected_rank
    ON archive.video_job_gif_candidates (job_id, is_selected, final_rank);

CREATE INDEX IF NOT EXISTS idx_video_job_gif_candidates_reject_reason
    ON archive.video_job_gif_candidates (reject_reason);

COMMENT ON TABLE archive.video_job_gif_candidates IS 'GIF候选片段明细（用于任务复盘、排序解释与失败归因）';
COMMENT ON COLUMN archive.video_job_gif_candidates.base_score IS '候选基础得分（规则/轻模型）';
COMMENT ON COLUMN archive.video_job_gif_candidates.confidence_score IS '候选置信度（用于后续AI低置信度重排触发）';
COMMENT ON COLUMN archive.video_job_gif_candidates.reject_reason IS '淘汰原因（如 duplicate_candidate/low_emotion 等）';
COMMENT ON COLUMN archive.video_job_gif_candidates.feature_json IS '候选特征快照（scene_score/reason/window_sec/source 等）';

COMMIT;
