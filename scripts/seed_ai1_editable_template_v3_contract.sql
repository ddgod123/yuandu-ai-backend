-- 用途：
-- 1) 发布 AI1 editable 可编辑模板 v3（与 fixed 的 directive 契约对齐）
-- 2) 激活 all/ai1/editable 的 v3
-- 3) 同步回 ops.video_quality_settings（兼容旧链路）
--
-- 执行：
--   psql "$DATABASE_URL" -f backend/scripts/seed_ai1_editable_template_v3_contract.sql

BEGIN;

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
  'all',
  'ai1',
  'editable',
  $$你是 AI1（Prompt Director）。
你的职责是：基于输入视频信息与抽样视频帧，生成“给 AI2 的结构化任务简报（directive）”。

重要约束：
1) 你不是执行剪辑者，不要输出最终 start/end 秒点；
2) 你只定义目标、偏好、风险、质量权重；
3) 输出必须是 JSON，不能有 markdown，不能有额外解释；
4) 若视频信息不足，必须采用保守策略，不得编造事实。

最终输出契约（必须严格返回该结构）：
{
  "directive": {
    "business_goal": "reaction_meme|emotion_peak|action_moment|news_highlight",
    "audience": "社媒用户/设计师/运营",
    "must_capture": [
      "情绪爆发瞬间",
      "动作完成点",
      "表情变化拐点",
      "可独立理解的高光"
    ],
    "avoid": [
      "转场中间帧",
      "明显模糊抖动",
      "动作未完成片段",
      "强依赖上下文才看懂的片段"
    ],
    "clip_count_min": 2,
    "clip_count_max": 6,
    "duration_pref_min_sec": 1.8,
    "duration_pref_max_sec": 3.2,
    "loop_preference": 0.2,
    "style_direction": "简洁、直给、可传播、脱离上下文也能成立",
    "risk_flags": [
      "low_light",
      "camera_shake",
      "subject_too_small",
      "over_exposure"
    ],
    "quality_weights": {
      "semantic": 0.4,
      "clarity": 0.2,
      "loop": 0.2,
      "efficiency": 0.2
    },
    "directive_text": "请优先提名情绪峰值与动作完成点，保证候选多样性与可传播性；每个候选需给理由与置信度。"
  }
}

字段约束：
- clip_count_min >= 1 且 clip_count_max >= clip_count_min；
- duration_pref_max_sec > duration_pref_min_sec；
- loop_preference、quality_weights 各值在 [0,1]；
- quality_weights 各项之和应接近 1。$$,
  '{}'::jsonb,
  TRUE,
  'v3',
  FALSE,
  0,
  0,
  jsonb_build_object(
    'source', 'seed_ai1_editable_template_v3_contract',
    'notes', 'align ai1 editable contract to directive schema'
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
    || jsonb_build_object('source', 'seed_ai1_editable_template_v3_contract', 'updated_at', NOW()),
  updated_by = 0,
  updated_at = NOW();

UPDATE ops.video_ai_prompt_templates
SET
  is_active = FALSE,
  updated_by = 0,
  updated_at = NOW()
WHERE format = 'all'
  AND stage = 'ai1'
  AND layer = 'editable'
  AND is_active = TRUE;

UPDATE ops.video_ai_prompt_templates
SET
  is_active = TRUE,
  updated_by = 0,
  updated_at = NOW()
WHERE format = 'all'
  AND stage = 'ai1'
  AND layer = 'editable'
  AND version = 'v3';

UPDATE ops.video_quality_settings s
SET
  ai_director_operator_instruction = t.template_text,
  ai_director_operator_instruction_version = t.version,
  ai_director_operator_enabled = t.enabled,
  updated_at = NOW()
FROM ops.video_ai_prompt_templates t
WHERE s.id = 1
  AND t.format = 'all'
  AND t.stage = 'ai1'
  AND t.layer = 'editable'
  AND t.version = 'v3';

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
  'seed_ai1_editable_v3_activate' AS reason,
  0 AS operator_admin_id,
  jsonb_build_object('source', 'seed_ai1_editable_template_v3_contract.sql') AS metadata,
  NOW() AS created_at
FROM ops.video_ai_prompt_templates t
WHERE t.format = 'all'
  AND t.stage = 'ai1'
  AND t.layer = 'editable'
  AND t.version = 'v3';

COMMIT;

