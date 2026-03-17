BEGIN;

CREATE TABLE IF NOT EXISTS ops.video_quality_rollout_audits (
    id BIGSERIAL PRIMARY KEY,
    admin_id BIGINT NOT NULL DEFAULT 0,
    from_rollout_percent INTEGER NOT NULL DEFAULT 0,
    to_rollout_percent INTEGER NOT NULL DEFAULT 0,
    window_label VARCHAR(16) NOT NULL DEFAULT '24h',
    confirm_windows INTEGER NOT NULL DEFAULT 3,
    recommendation_state VARCHAR(32) NOT NULL DEFAULT 'hold',
    recommendation_reason TEXT NOT NULL DEFAULT '',
    consecutive_required INTEGER NOT NULL DEFAULT 1,
    consecutive_matched INTEGER NOT NULL DEFAULT 1,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_quality_rollout_audits_created_at
    ON ops.video_quality_rollout_audits (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_video_quality_rollout_audits_admin_id
    ON ops.video_quality_rollout_audits (admin_id);
CREATE INDEX IF NOT EXISTS idx_video_quality_rollout_audits_state
    ON ops.video_quality_rollout_audits (recommendation_state);

COMMENT ON TABLE ops.video_quality_rollout_audits IS '视频高光反馈重排 rollout 变更审计日志';
COMMENT ON COLUMN ops.video_quality_rollout_audits.admin_id IS '操作管理员 ID';
COMMENT ON COLUMN ops.video_quality_rollout_audits.from_rollout_percent IS '应用前 rollout 百分比';
COMMENT ON COLUMN ops.video_quality_rollout_audits.to_rollout_percent IS '应用后 rollout 百分比';
COMMENT ON COLUMN ops.video_quality_rollout_audits.window_label IS '建议计算窗口（24h/7d/30d）';
COMMENT ON COLUMN ops.video_quality_rollout_audits.confirm_windows IS '连续确认窗口数';
COMMENT ON COLUMN ops.video_quality_rollout_audits.recommendation_state IS '建议状态（scale_up/scale_down 等）';
COMMENT ON COLUMN ops.video_quality_rollout_audits.recommendation_reason IS '建议解释文本';
COMMENT ON COLUMN ops.video_quality_rollout_audits.consecutive_required IS '连续确认阈值';
COMMENT ON COLUMN ops.video_quality_rollout_audits.consecutive_matched IS '已连续匹配窗口数';
COMMENT ON COLUMN ops.video_quality_rollout_audits.metadata IS '应用时的详细推荐快照';

COMMIT;
