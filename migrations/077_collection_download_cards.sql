BEGIN;

CREATE TABLE IF NOT EXISTS ops.collection_download_codes (
    id BIGSERIAL PRIMARY KEY,
    code_hash VARCHAR(128) NOT NULL UNIQUE,
    code_plain VARCHAR(64) NOT NULL,
    code_mask VARCHAR(64) NOT NULL,
    batch_no VARCHAR(64) NOT NULL DEFAULT '',
    collection_id BIGINT NOT NULL REFERENCES archive.collections(id) ON DELETE CASCADE,
    granted_download_times INTEGER NOT NULL DEFAULT 1,
    max_redeem_users INTEGER NOT NULL DEFAULT 1,
    used_redeem_users INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    starts_at TIMESTAMPTZ NULL,
    ends_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES "user".users(id) ON DELETE SET NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_collection_download_codes_granted_download_times
        CHECK (granted_download_times >= 1 AND granted_download_times <= 10000),
    CONSTRAINT chk_collection_download_codes_max_redeem_users
        CHECK (max_redeem_users >= 1 AND max_redeem_users <= 10000),
    CONSTRAINT chk_collection_download_codes_used_redeem_users
        CHECK (used_redeem_users >= 0 AND used_redeem_users <= max_redeem_users)
);

CREATE INDEX IF NOT EXISTS idx_collection_download_codes_collection_id
    ON ops.collection_download_codes(collection_id);
CREATE INDEX IF NOT EXISTS idx_collection_download_codes_status
    ON ops.collection_download_codes(status);
CREATE INDEX IF NOT EXISTS idx_collection_download_codes_batch_no
    ON ops.collection_download_codes(batch_no);

COMMENT ON TABLE ops.collection_download_codes IS '合集下载次卡兑换码模板';
COMMENT ON COLUMN ops.collection_download_codes.collection_id IS '绑定的合集ID';
COMMENT ON COLUMN ops.collection_download_codes.granted_download_times IS '兑换后赠送下载次数';
COMMENT ON COLUMN ops.collection_download_codes.max_redeem_users IS '最多允许兑换用户数（默认1）';
COMMENT ON COLUMN ops.collection_download_codes.used_redeem_users IS '已兑换用户数';

CREATE TABLE IF NOT EXISTS ops.collection_download_entitlements (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    collection_id BIGINT NOT NULL REFERENCES archive.collections(id) ON DELETE CASCADE,
    code_id BIGINT NULL REFERENCES ops.collection_download_codes(id) ON DELETE SET NULL,
    granted_download_times INTEGER NOT NULL DEFAULT 0,
    used_download_times INTEGER NOT NULL DEFAULT 0,
    remaining_download_times INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ NULL,
    last_consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_collection_download_entitlements_user_collection UNIQUE (user_id, collection_id),
    CONSTRAINT chk_collection_download_entitlements_granted_download_times
        CHECK (granted_download_times >= 0),
    CONSTRAINT chk_collection_download_entitlements_used_download_times
        CHECK (used_download_times >= 0),
    CONSTRAINT chk_collection_download_entitlements_remaining_download_times
        CHECK (remaining_download_times >= 0)
);

CREATE INDEX IF NOT EXISTS idx_collection_download_entitlements_status
    ON ops.collection_download_entitlements(status);
CREATE INDEX IF NOT EXISTS idx_collection_download_entitlements_expires_at
    ON ops.collection_download_entitlements(expires_at);

COMMENT ON TABLE ops.collection_download_entitlements IS '用户在某个合集上的下载次数权益';
COMMENT ON COLUMN ops.collection_download_entitlements.remaining_download_times IS '剩余下载次数';

CREATE TABLE IF NOT EXISTS ops.collection_download_redemptions (
    id BIGSERIAL PRIMARY KEY,
    code_id BIGINT NOT NULL REFERENCES ops.collection_download_codes(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    collection_id BIGINT NOT NULL REFERENCES archive.collections(id) ON DELETE CASCADE,
    granted_download_times INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NULL,
    ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_collection_download_redemptions_code_user UNIQUE (code_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_collection_download_redemptions_code_id
    ON ops.collection_download_redemptions(code_id);
CREATE INDEX IF NOT EXISTS idx_collection_download_redemptions_user_id
    ON ops.collection_download_redemptions(user_id);

COMMENT ON TABLE ops.collection_download_redemptions IS '合集下载次卡兑换记录';

CREATE TABLE IF NOT EXISTS action.collection_download_consumptions (
    id BIGSERIAL PRIMARY KEY,
    entitlement_id BIGINT NOT NULL REFERENCES ops.collection_download_entitlements(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    collection_id BIGINT NOT NULL REFERENCES archive.collections(id) ON DELETE CASCADE,
    code_id BIGINT NULL REFERENCES ops.collection_download_codes(id) ON DELETE SET NULL,
    download_mode VARCHAR(32) NOT NULL DEFAULT 'zip',
    consumed_times INTEGER NOT NULL DEFAULT 1,
    ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent VARCHAR(255) NOT NULL DEFAULT '',
    request_id VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_collection_download_consumptions_consumed_times
        CHECK (consumed_times >= 1)
);

CREATE INDEX IF NOT EXISTS idx_collection_download_consumptions_user_id
    ON action.collection_download_consumptions(user_id);
CREATE INDEX IF NOT EXISTS idx_collection_download_consumptions_collection_id
    ON action.collection_download_consumptions(collection_id);
CREATE INDEX IF NOT EXISTS idx_collection_download_consumptions_request_id
    ON action.collection_download_consumptions(request_id);

COMMENT ON TABLE action.collection_download_consumptions IS '合集下载次卡消费流水';

COMMIT;
