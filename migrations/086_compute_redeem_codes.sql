BEGIN;

CREATE TABLE IF NOT EXISTS ops.compute_redeem_codes (
    id BIGSERIAL PRIMARY KEY,
    code_hash VARCHAR(128) NOT NULL UNIQUE,
    code_plain VARCHAR(64) NOT NULL DEFAULT '',
    code_mask VARCHAR(64) NOT NULL DEFAULT '',
    batch_no VARCHAR(64) NOT NULL DEFAULT '',
    granted_points BIGINT NOT NULL DEFAULT 0,
    duration_days INTEGER NOT NULL DEFAULT 0,
    max_uses INTEGER NOT NULL DEFAULT 1,
    used_count INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    starts_at TIMESTAMPTZ NULL,
    ends_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES "user".users(id) ON DELETE SET NULL,
    note TEXT NOT NULL DEFAULT '',
    last_issued_at TIMESTAMPTZ NULL,
    last_issued_uid BIGINT NULL REFERENCES "user".users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_compute_redeem_codes_status ON ops.compute_redeem_codes(status);
CREATE INDEX IF NOT EXISTS idx_compute_redeem_codes_batch_no ON ops.compute_redeem_codes(batch_no);
CREATE INDEX IF NOT EXISTS idx_compute_redeem_codes_used_count ON ops.compute_redeem_codes(used_count);
CREATE INDEX IF NOT EXISTS idx_compute_redeem_codes_starts_at ON ops.compute_redeem_codes(starts_at);
CREATE INDEX IF NOT EXISTS idx_compute_redeem_codes_ends_at ON ops.compute_redeem_codes(ends_at);
CREATE INDEX IF NOT EXISTS idx_compute_redeem_codes_code_mask ON ops.compute_redeem_codes(code_mask);

CREATE TABLE IF NOT EXISTS ops.compute_redeem_redemptions (
    id BIGSERIAL PRIMARY KEY,
    code_id BIGINT NOT NULL REFERENCES ops.compute_redeem_codes(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    granted_points BIGINT NOT NULL DEFAULT 0,
    granted_starts_at TIMESTAMPTZ NOT NULL,
    granted_expires_at TIMESTAMPTZ NULL,
    ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uk_compute_redeem_code_user UNIQUE (code_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_compute_redeem_redemptions_code_id ON ops.compute_redeem_redemptions(code_id);
CREATE INDEX IF NOT EXISTS idx_compute_redeem_redemptions_user_id ON ops.compute_redeem_redemptions(user_id);
CREATE INDEX IF NOT EXISTS idx_compute_redeem_redemptions_created_at ON ops.compute_redeem_redemptions(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_compute_redeem_redemptions_expires_at ON ops.compute_redeem_redemptions(granted_expires_at);

COMMENT ON TABLE ops.compute_redeem_codes IS '算力点兑换码主表';
COMMENT ON COLUMN ops.compute_redeem_codes.code_hash IS '兑换码哈希值（SHA-256）';
COMMENT ON COLUMN ops.compute_redeem_codes.code_plain IS '兑换码明文（仅管理后台可见）';
COMMENT ON COLUMN ops.compute_redeem_codes.code_mask IS '兑换码掩码';
COMMENT ON COLUMN ops.compute_redeem_codes.batch_no IS '批次号';
COMMENT ON COLUMN ops.compute_redeem_codes.granted_points IS '核销后发放算力点';
COMMENT ON COLUMN ops.compute_redeem_codes.duration_days IS '有效天数（0 表示仅加点不设置到期）';
COMMENT ON COLUMN ops.compute_redeem_codes.max_uses IS '最大可核销次数';
COMMENT ON COLUMN ops.compute_redeem_codes.used_count IS '已核销次数';
COMMENT ON COLUMN ops.compute_redeem_codes.status IS '状态：active/disabled/expired';
COMMENT ON COLUMN ops.compute_redeem_codes.starts_at IS '可核销开始时间';
COMMENT ON COLUMN ops.compute_redeem_codes.ends_at IS '可核销结束时间';
COMMENT ON COLUMN ops.compute_redeem_codes.created_by IS '创建管理员用户ID';
COMMENT ON COLUMN ops.compute_redeem_codes.last_issued_at IS '最后一次核销时间';
COMMENT ON COLUMN ops.compute_redeem_codes.last_issued_uid IS '最后一次核销用户ID';

COMMENT ON TABLE ops.compute_redeem_redemptions IS '算力点兑换码核销记录';
COMMENT ON COLUMN ops.compute_redeem_redemptions.code_id IS '兑换码ID';
COMMENT ON COLUMN ops.compute_redeem_redemptions.user_id IS '核销用户ID';
COMMENT ON COLUMN ops.compute_redeem_redemptions.granted_points IS '本次核销发放算力点';
COMMENT ON COLUMN ops.compute_redeem_redemptions.granted_starts_at IS '本次核销生效时间';
COMMENT ON COLUMN ops.compute_redeem_redemptions.granted_expires_at IS '本次核销对应的展示到期时间';
COMMENT ON COLUMN ops.compute_redeem_redemptions.ip IS '核销来源IP';
COMMENT ON COLUMN ops.compute_redeem_redemptions.user_agent IS '核销来源UA';

COMMIT;
