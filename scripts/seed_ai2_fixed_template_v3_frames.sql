-- 用途：
-- 1) 发布 AI2 fixed 模板 v3（关键帧直推窗口，不依赖本地候选窗口）
-- 2) 激活 all/ai2/fixed 的 v3
--
-- 执行：
--   psql "$DATABASE_URL" -f backend/scripts/seed_ai2_fixed_template_v3_frames.sql

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
  'ai2',
  'fixed',
  $$你是视频GIF剪辑提名助手（AI2）。
请基于输入的视频元数据、关键帧抽样（frame_manifest）和 AI1 director 指令，直接生成 GIF 候选窗口方案。

必须返回 JSON（不要 markdown）：
{
  "proposals":[
    {
      "proposal_rank":1,
      "start_sec":12.3,
      "end_sec":14.8,
      "score":0.86,
      "proposal_reason":"表情爆发点+动作完成点",
      "semantic_tags":["emotion_peak","reaction"],
      "expected_value_level":"high",
      "standalone_confidence":0.88,
      "loop_friendliness_hint":0.72
    }
  ],
  "selected_rank":1,
  "notes":"可选，简短"
}

约束：
1) 时间窗口必须在视频时长范围内，end_sec > start_sec，单窗口建议 1.0~5.0 秒；
2) proposals 按质量从高到低，最多 20 条；
3) score / standalone_confidence / loop_friendliness_hint 均在 [0,1]；
4) 若输入包含 director，请优先遵循 director 的目标和偏好；
5) 不要依赖“预先给定候选窗口”，你需要直接从关键帧推断提名窗口。$$,
  '{}'::jsonb,
  TRUE,
  'v3',
  FALSE,
  0,
  0,
  jsonb_build_object(
    'source', 'seed_ai2_fixed_template_v3_frames',
    'notes', 'frame-first planner template'
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
    || jsonb_build_object('source', 'seed_ai2_fixed_template_v3_frames', 'updated_at', NOW()),
  updated_by = 0,
  updated_at = NOW();

UPDATE ops.video_ai_prompt_templates
SET
  is_active = FALSE,
  updated_by = 0,
  updated_at = NOW()
WHERE format = 'all'
  AND stage = 'ai2'
  AND layer = 'fixed'
  AND is_active = TRUE;

UPDATE ops.video_ai_prompt_templates
SET
  is_active = TRUE,
  updated_by = 0,
  updated_at = NOW()
WHERE format = 'all'
  AND stage = 'ai2'
  AND layer = 'fixed'
  AND version = 'v3';

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
  'seed_ai2_fixed_v3_activate' AS reason,
  0 AS operator_admin_id,
  jsonb_build_object('source', 'seed_ai2_fixed_template_v3_frames.sql') AS metadata,
  NOW() AS created_at
FROM ops.video_ai_prompt_templates t
WHERE t.format = 'all'
  AND t.stage = 'ai2'
  AND t.layer = 'fixed'
  AND t.version = 'v3';

COMMIT;

