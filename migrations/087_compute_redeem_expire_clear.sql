BEGIN;

ALTER TABLE ops.compute_redeem_redemptions
    ADD COLUMN IF NOT EXISTS clear_status VARCHAR(32) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS cleared_points BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cleared_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_compute_redeem_redemptions_clear_status
    ON ops.compute_redeem_redemptions(clear_status);

CREATE INDEX IF NOT EXISTS idx_compute_redeem_redemptions_expire_pending
    ON ops.compute_redeem_redemptions(granted_expires_at, clear_status);

UPDATE ops.compute_redeem_redemptions
SET clear_status = 'not_applicable'
WHERE granted_expires_at IS NULL
  AND (clear_status IS NULL OR clear_status = '' OR clear_status = 'pending');

UPDATE ops.compute_redeem_redemptions
SET clear_status = 'pending'
WHERE granted_expires_at IS NOT NULL
  AND (clear_status IS NULL OR clear_status = '');

COMMENT ON COLUMN ops.compute_redeem_redemptions.clear_status IS '清零状态：pending/cleared/not_applicable';
COMMENT ON COLUMN ops.compute_redeem_redemptions.cleared_points IS '到期清零实际扣减点数';
COMMENT ON COLUMN ops.compute_redeem_redemptions.cleared_at IS '到期清零处理时间';

COMMIT;
