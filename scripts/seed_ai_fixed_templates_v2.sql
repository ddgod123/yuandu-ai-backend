-- 用途：
-- 1) 为 AI2 / AI3 fixed 层创建 v2 版本（默认不激活）
-- 2) 写入模板审计日志（action=create）
--
-- 执行：
--   psql "$DATABASE_URL" -f backend/scripts/seed_ai_fixed_templates_v2.sql

BEGIN;

WITH source_rows AS (
  SELECT *
  FROM ops.video_ai_prompt_templates s
  WHERE s.format = 'all'
    AND s.layer = 'fixed'
    AND s.stage IN ('ai2', 'ai3')
    AND s.is_active = TRUE
),
inserted AS (
  INSERT INTO ops.video_ai_prompt_templates (
    format,
    stage,
    layer,
    template_text,
    template_json_schema,
    enabled,
    version,
    is_active,
    created_by,
    updated_by,
    metadata,
    created_at,
    updated_at
  )
  SELECT
    s.format,
    s.stage,
    s.layer,
    s.template_text,
    s.template_json_schema,
    s.enabled,
    'v2' AS version,
    FALSE AS is_active,
    0 AS created_by,
    0 AS updated_by,
    COALESCE(s.metadata, '{}'::jsonb) || jsonb_build_object(
      'seed', 'seed_ai_fixed_templates_v2',
      'source_template_id', s.id
    ) AS metadata,
    NOW(),
    NOW()
  FROM source_rows s
  WHERE NOT EXISTS (
    SELECT 1
    FROM ops.video_ai_prompt_templates t
    WHERE t.format = s.format
      AND t.stage = s.stage
      AND t.layer = s.layer
      AND t.version = 'v2'
  )
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
  i.id AS template_id,
  i.format,
  i.stage,
  i.layer,
  'create' AS action,
  '{}'::jsonb AS old_value,
  jsonb_build_object(
    'id', i.id,
    'format', i.format,
    'stage', i.stage,
    'layer', i.layer,
    'version', i.version,
    'enabled', i.enabled,
    'is_active', i.is_active
  ) AS new_value,
  'seed_v2_template' AS reason,
  0 AS operator_admin_id,
  jsonb_build_object('source', 'seed_ai_fixed_templates_v2.sql') AS metadata,
  NOW() AS created_at
FROM inserted i;

COMMIT;
