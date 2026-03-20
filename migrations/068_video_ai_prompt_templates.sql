BEGIN;

CREATE TABLE IF NOT EXISTS ops.video_ai_prompt_templates (
  id BIGSERIAL PRIMARY KEY,
  format VARCHAR(16) NOT NULL,
  stage VARCHAR(16) NOT NULL,
  layer VARCHAR(16) NOT NULL,
  template_text TEXT NOT NULL DEFAULT '',
  template_json_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  version VARCHAR(64) NOT NULL DEFAULT 'v1',
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by BIGINT NOT NULL DEFAULT 0,
  updated_by BIGINT NOT NULL DEFAULT 0,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_video_ai_prompt_templates_format CHECK (format IN ('all', 'gif', 'webp', 'jpg', 'png', 'live')),
  CONSTRAINT chk_video_ai_prompt_templates_stage CHECK (stage IN ('ai1', 'ai2', 'scoring', 'ai3')),
  CONSTRAINT chk_video_ai_prompt_templates_layer CHECK (
    (stage = 'ai1' AND layer IN ('editable', 'fixed'))
    OR (stage IN ('ai2', 'scoring', 'ai3') AND layer = 'fixed')
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_video_ai_prompt_templates_unique_version
  ON ops.video_ai_prompt_templates (format, stage, layer, version);
CREATE UNIQUE INDEX IF NOT EXISTS idx_video_ai_prompt_templates_active
  ON ops.video_ai_prompt_templates (format, stage, layer)
  WHERE is_active = TRUE;
CREATE INDEX IF NOT EXISTS idx_video_ai_prompt_templates_lookup
  ON ops.video_ai_prompt_templates (stage, layer, format, is_active, enabled);
CREATE INDEX IF NOT EXISTS idx_video_ai_prompt_templates_updated_at
  ON ops.video_ai_prompt_templates (updated_at DESC);

COMMENT ON TABLE ops.video_ai_prompt_templates IS '视频转图 AI 各阶段模板（AI1/AI2/评分/AI3）';
COMMENT ON COLUMN ops.video_ai_prompt_templates.format IS '格式作用域：all/gif/webp/jpg/png/live';
COMMENT ON COLUMN ops.video_ai_prompt_templates.stage IS '阶段：ai1/ai2/scoring/ai3';
COMMENT ON COLUMN ops.video_ai_prompt_templates.layer IS '层级：editable/fixed（仅 ai1 有 editable）';
COMMENT ON COLUMN ops.video_ai_prompt_templates.template_text IS '模板正文';
COMMENT ON COLUMN ops.video_ai_prompt_templates.template_json_schema IS '模板结构约束（可选）';
COMMENT ON COLUMN ops.video_ai_prompt_templates.enabled IS '模板开关';
COMMENT ON COLUMN ops.video_ai_prompt_templates.version IS '模板版本';
COMMENT ON COLUMN ops.video_ai_prompt_templates.is_active IS '是否为当前生效版本';
COMMENT ON COLUMN ops.video_ai_prompt_templates.created_by IS '创建管理员ID';
COMMENT ON COLUMN ops.video_ai_prompt_templates.updated_by IS '更新管理员ID';
COMMENT ON COLUMN ops.video_ai_prompt_templates.metadata IS '补充信息（来源、注释等）';

CREATE TABLE IF NOT EXISTS audit.video_ai_prompt_template_audits (
  id BIGSERIAL PRIMARY KEY,
  template_id BIGINT NULL,
  format VARCHAR(16) NOT NULL,
  stage VARCHAR(16) NOT NULL,
  layer VARCHAR(16) NOT NULL,
  action VARCHAR(16) NOT NULL,
  old_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  new_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  reason TEXT NOT NULL DEFAULT '',
  operator_admin_id BIGINT NOT NULL DEFAULT 0,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_video_ai_prompt_template_audits_action CHECK (action IN ('create', 'update', 'upsert', 'activate', 'deactivate'))
);

CREATE INDEX IF NOT EXISTS idx_video_ai_prompt_template_audits_created
  ON audit.video_ai_prompt_template_audits (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_video_ai_prompt_template_audits_stage
  ON audit.video_ai_prompt_template_audits (stage, format, layer, created_at DESC);

COMMENT ON TABLE audit.video_ai_prompt_template_audits IS '视频转图 AI 模板变更审计日志';
COMMENT ON COLUMN audit.video_ai_prompt_template_audits.template_id IS '模板ID（可为空）';
COMMENT ON COLUMN audit.video_ai_prompt_template_audits.old_value IS '变更前快照';
COMMENT ON COLUMN audit.video_ai_prompt_template_audits.new_value IS '变更后快照';
COMMENT ON COLUMN audit.video_ai_prompt_template_audits.reason IS '变更原因';

INSERT INTO ops.video_ai_prompt_templates (
  format, stage, layer, template_text, template_json_schema,
  enabled, version, is_active, created_by, updated_by, metadata
)
SELECT
  'all',
  'ai1',
  'editable',
  COALESCE(vqs.ai_director_operator_instruction, ''),
  '{}'::jsonb,
  COALESCE(vqs.ai_director_operator_enabled, TRUE),
  COALESCE(NULLIF(vqs.ai_director_operator_instruction_version, ''), 'v1'),
  TRUE,
  0,
  0,
  jsonb_build_object('source', 'ops.video_quality_settings')
FROM ops.video_quality_settings vqs
WHERE vqs.id = 1
  AND NOT EXISTS (
    SELECT 1
    FROM ops.video_ai_prompt_templates t
    WHERE t.format = 'all' AND t.stage = 'ai1' AND t.layer = 'editable' AND t.is_active = TRUE
  );

INSERT INTO ops.video_ai_prompt_templates (
  format, stage, layer, template_text, template_json_schema,
  enabled, version, is_active, created_by, updated_by, metadata
)
SELECT
  'all', 'ai1', 'editable', '', '{}'::jsonb, TRUE, 'v1', TRUE, 0, 0,
  jsonb_build_object('source', 'migration_default')
WHERE NOT EXISTS (
  SELECT 1 FROM ops.video_ai_prompt_templates t
  WHERE t.format = 'all' AND t.stage = 'ai1' AND t.layer = 'editable' AND t.is_active = TRUE
);

INSERT INTO ops.video_ai_prompt_templates (
  format, stage, layer, template_text, template_json_schema,
  enabled, version, is_active, created_by, updated_by, metadata
)
SELECT
  'all', s.stage, 'fixed', s.template_text, '{}'::jsonb, TRUE, 'v1', TRUE, 0, 0,
  jsonb_build_object('source', 'migration_default')
FROM (
  VALUES
  (
    'ai1',
    $$你是视频转GIF任务的“需求甲方（Prompt Director）”。
你的职责是：在正式剪辑方案生成前，给出结构化任务指令，指导后续Planner更稳定地产出高价值GIF。
仅返回JSON（不要markdown）：
{
  "directive": {
    "business_goal": "entertainment|news|design_asset|social_spread",
    "audience": "简短描述",
    "must_capture": ["必须抓取的瞬间/特征"],
    "avoid": ["应避免片段/质量风险"],
    "clip_count_min": 3,
    "clip_count_max": 8,
    "duration_pref_min_sec": 1.4,
    "duration_pref_max_sec": 3.2,
    "loop_preference": 0.0,
    "style_direction": "画风/节奏方向（简短）",
    "risk_flags": ["low_light","fast_motion","noise_audio"],
    "quality_weights": {"semantic":0.35,"clarity":0.20,"loop":0.25,"efficiency":0.20},
    "directive_text": "给Planner的自然语言摘要，50~120字"
  }
}
约束：
1) loop_preference、quality_weights 各值在 [0,1]；
2) quality_weights 的和应接近 1；
3) clip_count_min>=1 且 clip_count_max>=clip_count_min；
4) duration_pref_max_sec > duration_pref_min_sec。$$
  ),
  (
    'ai2',
    $$你是视频GIF剪辑提名助手。请基于输入候选窗口，给出更贴近“高光、可脱离上下文、可传播、可循环”的提名方案。
必须返回JSON（不要markdown）：
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
3) score/standalone_confidence/loop_friendliness_hint 均在 [0,1]；
4) 若输入包含 director，请优先遵循 director 的目标和偏好。$$
  ),
  (
    'scoring',
    $$评分系统（固定层）说明：
1) 技术硬门槛优先：清晰度、循环闭合、时长、体积预算；
2) 五维质量分：overall/emotion/clarity/motion/loop/efficiency；
3) 结果保留语义：deliver/keep_internal/reject/need_manual_review；
4) 前台默认仅展示 deliver，后台可见全量状态。$$
  ),
  (
    'ai3',
    $$你是GIF语义复审评委。请根据每个GIF样本的技术评分与上下文，输出可执行的最终建议。
仅返回JSON（不要markdown）：
{
  "reviews":[
    {
      "output_id":123,
      "proposal_rank":1,
      "final_recommendation":"deliver|keep_internal|reject|need_manual_review",
      "semantic_verdict":0.82,
      "diagnostic_reason":"简短原因",
      "suggested_action":"简短建议"
    }
  ],
  "summary":{"note":"可选"}
}
要求：
1) reviews 里的 output_id 必须来自输入；
2) final_recommendation 仅允许四个枚举值；
3) semantic_verdict 在 [0,1]。$$
  )
) AS s(stage, template_text)
WHERE NOT EXISTS (
  SELECT 1
  FROM ops.video_ai_prompt_templates t
  WHERE t.format = 'all' AND t.stage = s.stage AND t.layer = 'fixed' AND t.is_active = TRUE
);

COMMIT;
