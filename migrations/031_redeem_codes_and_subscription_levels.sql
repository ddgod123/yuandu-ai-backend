BEGIN;

ALTER TABLE "user".users
  ADD COLUMN IF NOT EXISTS subscription_started_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_user_users_subscription_started
  ON "user".users(subscription_started_at);

CREATE TABLE IF NOT EXISTS ops.redeem_codes (
    id BIGSERIAL PRIMARY KEY,
    code_hash VARCHAR(128) NOT NULL UNIQUE,
    code_mask VARCHAR(64) NOT NULL,
    batch_no VARCHAR(64) NOT NULL DEFAULT '',
    plan VARCHAR(32) NOT NULL DEFAULT 'subscriber',
    duration_days INTEGER NOT NULL DEFAULT 30,
    max_uses INTEGER NOT NULL DEFAULT 1,
    used_count INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    starts_at TIMESTAMPTZ NULL,
    ends_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL,
    note TEXT NOT NULL DEFAULT '',
    last_issued_at TIMESTAMPTZ NULL,
    last_issued_uid BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_redeem_codes_status ON ops.redeem_codes(status);
CREATE INDEX IF NOT EXISTS idx_redeem_codes_plan ON ops.redeem_codes(plan);
CREATE INDEX IF NOT EXISTS idx_redeem_codes_batch ON ops.redeem_codes(batch_no);
CREATE INDEX IF NOT EXISTS idx_redeem_codes_starts ON ops.redeem_codes(starts_at);
CREATE INDEX IF NOT EXISTS idx_redeem_codes_ends ON ops.redeem_codes(ends_at);
CREATE INDEX IF NOT EXISTS idx_redeem_codes_used_count ON ops.redeem_codes(used_count);

CREATE TABLE IF NOT EXISTS ops.redeem_code_redemptions (
    id BIGSERIAL PRIMARY KEY,
    code_id BIGINT NOT NULL REFERENCES ops.redeem_codes(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    granted_plan VARCHAR(32) NOT NULL DEFAULT 'subscriber',
    granted_status VARCHAR(32) NOT NULL DEFAULT 'active',
    granted_starts_at TIMESTAMPTZ NOT NULL,
    granted_expires_at TIMESTAMPTZ NOT NULL,
    ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uk_redeem_code_user UNIQUE (code_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_user_id ON ops.redeem_code_redemptions(user_id);
CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_code_id ON ops.redeem_code_redemptions(code_id);
CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_created ON ops.redeem_code_redemptions(created_at DESC);

COMMENT ON TABLE ops.redeem_codes IS '兑换码主表（仅存哈希和掩码，不存明文）';
COMMENT ON COLUMN ops.redeem_codes.code_hash IS '兑换码哈希值（SHA-256）';
COMMENT ON COLUMN ops.redeem_codes.code_mask IS '兑换码掩码，用于后台展示';
COMMENT ON COLUMN ops.redeem_codes.batch_no IS '批次号';
COMMENT ON COLUMN ops.redeem_codes.plan IS '订阅计划';
COMMENT ON COLUMN ops.redeem_codes.duration_days IS '兑换后增加的订阅天数';
COMMENT ON COLUMN ops.redeem_codes.max_uses IS '最大可使用次数';
COMMENT ON COLUMN ops.redeem_codes.used_count IS '已使用次数';
COMMENT ON COLUMN ops.redeem_codes.status IS '状态：active/disabled/expired';
COMMENT ON COLUMN ops.redeem_codes.starts_at IS '可兑换开始时间';
COMMENT ON COLUMN ops.redeem_codes.ends_at IS '可兑换结束时间';
COMMENT ON COLUMN ops.redeem_codes.created_by IS '创建人管理员ID';
COMMENT ON COLUMN ops.redeem_codes.last_issued_at IS '最后一次核销时间';
COMMENT ON COLUMN ops.redeem_codes.last_issued_uid IS '最后一次核销用户ID';

COMMENT ON TABLE ops.redeem_code_redemptions IS '兑换码核销记录';
COMMENT ON COLUMN ops.redeem_code_redemptions.code_id IS '兑换码ID';
COMMENT ON COLUMN ops.redeem_code_redemptions.user_id IS '核销用户ID';
COMMENT ON COLUMN ops.redeem_code_redemptions.granted_plan IS '核销赋予的订阅计划';
COMMENT ON COLUMN ops.redeem_code_redemptions.granted_status IS '核销赋予的订阅状态';
COMMENT ON COLUMN ops.redeem_code_redemptions.granted_starts_at IS '本次核销生效时间';
COMMENT ON COLUMN ops.redeem_code_redemptions.granted_expires_at IS '本次核销到期时间';
COMMENT ON COLUMN ops.redeem_code_redemptions.ip IS '核销来源IP';
COMMENT ON COLUMN ops.redeem_code_redemptions.user_agent IS '核销来源UA';

COMMIT;
