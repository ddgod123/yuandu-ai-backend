BEGIN;

ALTER TABLE ops.redeem_codes
  ADD COLUMN IF NOT EXISTS code_plain VARCHAR(64);

COMMENT ON COLUMN ops.redeem_codes.code_plain IS '兑换码明文（后台可见，便于运营复制发放）';

COMMIT;
