BEGIN;

CREATE TABLE IF NOT EXISTS ops.data_audit_runs (
    id BIGSERIAL PRIMARY KEY,
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status VARCHAR(32) NOT NULL DEFAULT 'healthy',
    apply BOOLEAN NOT NULL DEFAULT FALSE,
    fix_orphans BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    report_path TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    db_emoji_total INTEGER NOT NULL DEFAULT 0,
    db_zip_total INTEGER NOT NULL DEFAULT 0,
    qiniu_object_total INTEGER NOT NULL DEFAULT 0,
    missing_emoji_object_count INTEGER NOT NULL DEFAULT 0,
    missing_zip_object_count INTEGER NOT NULL DEFAULT 0,
    qiniu_orphan_raw_count INTEGER NOT NULL DEFAULT 0,
    qiniu_orphan_zip_count INTEGER NOT NULL DEFAULT 0,
    file_count_mismatch_count INTEGER NOT NULL DEFAULT 0,
    report_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_data_audit_runs_run_at_desc
    ON ops.data_audit_runs(run_at DESC);

CREATE INDEX IF NOT EXISTS idx_data_audit_runs_status_run_at_desc
    ON ops.data_audit_runs(status, run_at DESC);

COMMENT ON TABLE ops.data_audit_runs IS '数据审计任务运行记录（DB 与对象存储一致性巡检）';
COMMENT ON COLUMN ops.data_audit_runs.status IS '运行状态：healthy|warn|failed';
COMMENT ON COLUMN ops.data_audit_runs.report_path IS '本地 JSON 报告路径';
COMMENT ON COLUMN ops.data_audit_runs.report_json IS '完整审计结果快照';

COMMIT;
