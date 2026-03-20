-- 用途：
-- 1) 补齐 AI1 editable 可编辑模板（v2）
-- 2) 激活 all/ai1/editable 的 v2
-- 3) 同步回 ops.video_quality_settings（兼容旧链路）
--
-- 执行：
--   psql "$DATABASE_URL" -f backend/scripts/seed_ai1_editable_template_v2.sql

BEGIN;

-- 1) 创建或更新 v2 模板（先不激活）
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
你的职责是：基于输入视频信息，产出“给 AI2 的结构化任务简报（task brief）”，用于指导 AI2 提出 GIF 候选窗口方案。

重要约束：
1) 你不是剪辑执行者，不要直接输出最终窗口 start/end 秒点；
2) 你只负责定义目标、策略、约束、偏好；
3) 输出必须是 JSON，不能有 markdown、不能有额外解释文本；
4) 若视频信息不足，必须给出保守策略，不得编造不存在内容。

请只返回以下 JSON 结构：
{
  "brief_version": "gif_director_v2",
  "business_goal": "reaction_meme|emotion_peak|action_moment|news_highlight",
  "audience": "社媒用户/设计师/运营",
  "style_direction": "简洁、直给、可传播、脱离上下文也能成立",
  "clip_count_range": [2, 5],
  "duration_pref_sec": [1.8, 3.2],
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
  "planner_instruction_text": "请优先提名情绪峰值与动作完成点，保证候选多样性与可传播性；每个候选需给理由与置信度。",
  "fallback_policy": {
    "min_deliver_count": 1,
    "when_no_confident_window": "放宽语义阈值但保持清晰度底线"
  }
}$$,
  '{}'::jsonb,
  TRUE,
  'v2',
  FALSE,
  0,
  0,
  jsonb_build_object(
    'source', 'seed_ai1_editable_template_v2',
    'notes', 'ai1 operator editable template v2'
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
    || jsonb_build_object('source', 'seed_ai1_editable_template_v2', 'updated_at', NOW()),
  updated_by = 0,
  updated_at = NOW();

-- 2) 先下线当前 active，避免唯一约束冲突
UPDATE ops.video_ai_prompt_templates
SET
  is_active = FALSE,
  updated_by = 0,
  updated_at = NOW()
WHERE format = 'all'
  AND stage = 'ai1'
  AND layer = 'editable'
  AND is_active = TRUE;

-- 3) 激活 v2
UPDATE ops.video_ai_prompt_templates
SET
  is_active = TRUE,
  updated_by = 0,
  updated_at = NOW()
WHERE format = 'all'
  AND stage = 'ai1'
  AND layer = 'editable'
  AND version = 'v2';

-- 4) 同步旧链路配置（兼容）
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
  AND t.version = 'v2';

-- 5) 写审计
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
  'seed_ai1_editable_v2_activate' AS reason,
  0 AS operator_admin_id,
  jsonb_build_object('source', 'seed_ai1_editable_template_v2.sql') AS metadata,
  NOW() AS created_at
FROM ops.video_ai_prompt_templates t
WHERE t.format = 'all'
  AND t.stage = 'ai1'
  AND t.layer = 'editable'
  AND t.version = 'v2';

COMMIT;
