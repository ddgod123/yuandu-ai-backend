BEGIN;

CREATE TABLE IF NOT EXISTS ops.compute_accounts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    available_points BIGINT NOT NULL DEFAULT 0,
    frozen_points BIGINT NOT NULL DEFAULT 0,
    debt_points BIGINT NOT NULL DEFAULT 0,
    total_consumed_points BIGINT NOT NULL DEFAULT 0,
    total_recharged_points BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_compute_accounts_user_id UNIQUE (user_id)
);

CREATE INDEX IF NOT EXISTS idx_compute_accounts_status ON ops.compute_accounts(status);
CREATE INDEX IF NOT EXISTS idx_compute_accounts_updated_at_desc ON ops.compute_accounts(updated_at DESC);

CREATE TABLE IF NOT EXISTS ops.compute_ledgers (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES ops.compute_accounts(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    job_id BIGINT REFERENCES archive.video_jobs(id) ON DELETE SET NULL,
    type VARCHAR(32) NOT NULL DEFAULT '',
    points BIGINT NOT NULL DEFAULT 0,
    available_before BIGINT NOT NULL DEFAULT 0,
    available_after BIGINT NOT NULL DEFAULT 0,
    frozen_before BIGINT NOT NULL DEFAULT 0,
    frozen_after BIGINT NOT NULL DEFAULT 0,
    debt_before BIGINT NOT NULL DEFAULT 0,
    debt_after BIGINT NOT NULL DEFAULT 0,
    remark TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_compute_ledgers_account_id ON ops.compute_ledgers(account_id);
CREATE INDEX IF NOT EXISTS idx_compute_ledgers_user_id ON ops.compute_ledgers(user_id);
CREATE INDEX IF NOT EXISTS idx_compute_ledgers_job_id ON ops.compute_ledgers(job_id);
CREATE INDEX IF NOT EXISTS idx_compute_ledgers_type ON ops.compute_ledgers(type);
CREATE INDEX IF NOT EXISTS idx_compute_ledgers_created_at_desc ON ops.compute_ledgers(created_at DESC);

CREATE TABLE IF NOT EXISTS ops.compute_point_holds (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES archive.video_jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL REFERENCES ops.compute_accounts(id) ON DELETE CASCADE,
    reserved_points BIGINT NOT NULL DEFAULT 0,
    settled_points BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'held',
    remark TEXT NOT NULL DEFAULT '',
    settled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_compute_point_holds_job_id UNIQUE (job_id)
);

CREATE INDEX IF NOT EXISTS idx_compute_point_holds_user_id ON ops.compute_point_holds(user_id);
CREATE INDEX IF NOT EXISTS idx_compute_point_holds_account_id ON ops.compute_point_holds(account_id);
CREATE INDEX IF NOT EXISTS idx_compute_point_holds_status ON ops.compute_point_holds(status);
CREATE INDEX IF NOT EXISTS idx_compute_point_holds_created_at_desc ON ops.compute_point_holds(created_at DESC);

COMMENT ON TABLE ops.compute_accounts IS '用户算力账户余额（可用/冻结/欠费）';
COMMENT ON TABLE ops.compute_ledgers IS '算力点流水明细';
COMMENT ON TABLE ops.compute_point_holds IS '视频任务预扣冻结记录';

COMMIT;
