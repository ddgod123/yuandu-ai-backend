-- 用途：
-- 1) 发布 PNG 专属 AI1 editable 模板（含 scene_strategies_v1）
-- 2) 激活 png/ai1/editable 的版本 png_scene_strategy_v1
-- 3) 保留 all/ai1/editable 作为兜底
--
-- 执行：
--   psql "$DATABASE_URL" -f backend/scripts/seed_png_ai1_editable_strategy_v1.sql

BEGIN;

WITH all_active AS (
  SELECT
    template_text,
    enabled
  FROM ops.video_ai_prompt_templates
  WHERE format = 'all'
    AND stage = 'ai1'
    AND layer = 'editable'
    AND is_active = TRUE
  ORDER BY id DESC
  LIMIT 1
)
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
VALUES (
  'png',
  'ai1',
  'editable',
  COALESCE((SELECT template_text FROM all_active), ''),
  jsonb_build_object(
    'scene_strategies_v1',
    jsonb_build_object(
      'version', 'png_scene_strategy_v1',
      'scenes', jsonb_build_object(
        'default', jsonb_build_object(
          'scene', 'default',
          'enabled', TRUE,
          'scene_label', '通用截图',
          'business_goal', 'extract_high_quality_frames',
          'audience', '通用用户',
          'operator_identity', '视觉总监',
          'style_direction', 'balanced_clarity',
          'candidate_count_bias', jsonb_build_object('min', 4, 'max', 8),
          'directive_hint', '优先输出清晰、稳定、主体完整的静态帧。',
          'must_capture_bias', jsonb_build_array('主体清晰', '关键内容完整', '构图稳定'),
          'avoid_bias', jsonb_build_array('严重模糊', '全黑或全白曝光异常', '主体残缺'),
          'risk_flags', jsonb_build_array(),
          'quality_weights', jsonb_build_object(
            'semantic', 0.38,
            'clarity', 0.40,
            'loop', 0.02,
            'efficiency', 0.20
          ),
          'technical_reject', jsonb_build_object(
            'max_blur_tolerance', 'low',
            'avoid_watermarks', TRUE,
            'avoid_extreme_dark', TRUE
          )
        ),
        'xiaohongshu', jsonb_build_object(
          'scene', 'xiaohongshu',
          'enabled', TRUE,
          'scene_label', '小红书网感',
          'business_goal', 'social_spread',
          'audience', '小红书内容受众',
          'operator_identity', '时尚视觉总监',
          'style_direction', 'social_cover_high_clarity',
          'candidate_count_bias', jsonb_build_object('min', 4, 'max', 8),
          'directive_hint', '按社交封面标准筛选，优先高吸引力特写与高画质。',
          'must_capture_bias', jsonb_build_array('高颜值特写', '情绪峰值', '定格姿态', '色彩明快'),
          'avoid_bias', jsonb_build_array('背影遮挡', '低饱和灰雾', '杂乱背景', '运动拖影'),
          'risk_flags', jsonb_build_array('social_cover_priority'),
          'quality_weights', jsonb_build_object(
            'semantic', 0.36,
            'clarity', 0.50,
            'loop', 0.02,
            'efficiency', 0.12
          ),
          'technical_reject', jsonb_build_object(
            'max_blur_tolerance', 'low',
            'avoid_watermarks', TRUE,
            'avoid_extreme_dark', TRUE
          )
        )
      )
    )
  ),
  COALESCE((SELECT enabled FROM all_active), TRUE),
  'png_scene_strategy_v1',
  FALSE,
  0,
  0,
  jsonb_build_object(
    'source', 'seed_png_ai1_editable_strategy_v1',
    'notes', 'png ai1 editable with scene_strategies_v1; all kept as fallback'
  ),
  NOW(),
  NOW()
)
ON CONFLICT (format, stage, layer, version)
DO UPDATE
SET
  template_text = EXCLUDED.template_text,
  template_json_schema = EXCLUDED.template_json_schema,
  enabled = EXCLUDED.enabled,
  metadata = COALESCE(ops.video_ai_prompt_templates.metadata, '{}'::jsonb)
    || jsonb_build_object('source', 'seed_png_ai1_editable_strategy_v1', 'updated_at', NOW()),
  updated_by = 0,
  updated_at = NOW();

UPDATE ops.video_ai_prompt_templates
SET
  is_active = FALSE,
  updated_by = 0,
  updated_at = NOW()
WHERE format = 'png'
  AND stage = 'ai1'
  AND layer = 'editable'
  AND is_active = TRUE
  AND version <> 'png_scene_strategy_v1';

UPDATE ops.video_ai_prompt_templates
SET
  is_active = TRUE,
  updated_by = 0,
  updated_at = NOW()
WHERE format = 'png'
  AND stage = 'ai1'
  AND layer = 'editable'
  AND version = 'png_scene_strategy_v1';

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
  t.id AS template_id,
  t.format,
  t.stage,
  t.layer,
  'activate' AS action,
  '{}'::jsonb AS old_value,
  jsonb_build_object(
    'id', t.id,
    'format', t.format,
    'stage', t.stage,
    'layer', t.layer,
    'version', t.version,
    'enabled', t.enabled,
    'is_active', t.is_active
  ) AS new_value,
  'seed_png_ai1_editable_strategy_v1_activate' AS reason,
  0 AS operator_admin_id,
  jsonb_build_object('source', 'seed_png_ai1_editable_strategy_v1.sql') AS metadata,
  NOW() AS created_at
FROM ops.video_ai_prompt_templates t
WHERE t.format = 'png'
  AND t.stage = 'ai1'
  AND t.layer = 'editable'
  AND t.version = 'png_scene_strategy_v1'
  AND t.is_active = TRUE;

COMMIT;

