-- 用途：
-- 批量激活 all/ai2+ai3/fixed 的 v2 版本（并写 activate 审计）
--
-- 执行：
--   psql "$DATABASE_URL" -f backend/scripts/activate_ai_fixed_templates_v2.sql

BEGIN;

-- 先全量下线目标 scope 的 active，避免触发唯一约束冲突
UPDATE ops.video_ai_prompt_templates
SET is_active = FALSE,
    updated_by = 0,
    updated_at = NOW()
WHERE format = 'all'
  AND layer = 'fixed'
  AND stage IN ('ai2', 'ai3')
  AND is_active = TRUE;

WITH activated AS (
  UPDATE ops.video_ai_prompt_templates
  SET is_active = TRUE,
      updated_by = 0,
      updated_at = NOW()
  WHERE format = 'all'
    AND layer = 'fixed'
    AND stage IN ('ai2', 'ai3')
    AND version = 'v2'
  RETURNING *
)
INSERT INTO audit.video_ai_prompt_template_audits (
  template_id,
  format,
  stage,
  layer,
  action,
  old_value,
  new_value,
  reason,
  operator_admin_id,
  metadata,
  created_at
)
SELECT
  a.id AS template_id,
  a.format,
  a.stage,
  a.layer,
  'activate' AS action,
  '{}'::jsonb AS old_value,
  jsonb_build_object(
    'id', a.id,
    'format', a.format,
    'stage', a.stage,
    'layer', a.layer,
    'version', a.version,
    'enabled', a.enabled,
    'is_active', a.is_active
  ) AS new_value,
  'activate_v2_batch' AS reason,
  0 AS operator_admin_id,
  jsonb_build_object('source', 'activate_ai_fixed_templates_v2.sql') AS metadata,
  NOW() AS created_at
FROM activated a;

COMMIT;
