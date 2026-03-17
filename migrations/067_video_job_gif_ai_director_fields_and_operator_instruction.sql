BEGIN;

ALTER TABLE archive.video_job_gif_ai_directives
    ADD COLUMN IF NOT EXISTS style_direction TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS risk_flags JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS brief_version VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS model_version VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS input_context_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS status VARCHAR(16) NOT NULL DEFAULT 'ok',
    ADD COLUMN IF NOT EXISTS fallback_used BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN archive.video_job_gif_ai_directives.style_direction IS 'AI1 风格倾向（结构化标签/说明）';
COMMENT ON COLUMN archive.video_job_gif_ai_directives.risk_flags IS 'AI1 风险标签（如内容风险/质量风险）';
COMMENT ON COLUMN archive.video_job_gif_ai_directives.brief_version IS 'AI1 任务简报版本（运营可控版本）';
COMMENT ON COLUMN archive.video_job_gif_ai_directives.model_version IS 'AI1 实际模型版本快照';
COMMENT ON COLUMN archive.video_job_gif_ai_directives.input_context_json IS 'AI1 输入上下文快照（便于复盘）';
COMMENT ON COLUMN archive.video_job_gif_ai_directives.status IS 'AI1 执行状态（ok/fallback/error）';
COMMENT ON COLUMN archive.video_job_gif_ai_directives.fallback_used IS 'AI1 是否触发回退';

UPDATE archive.video_job_gif_ai_directives
SET brief_version = COALESCE(NULLIF(brief_version, ''), NULLIF(prompt_version, ''), 'v1'),
    model_version = COALESCE(NULLIF(model_version, ''), NULLIF(model, ''), ''),
    status = CASE
      WHEN COALESCE(NULLIF(status, ''), '') = '' THEN 'ok'
      ELSE status
    END
WHERE TRUE;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS ai_director_operator_instruction TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS ai_director_operator_instruction_version VARCHAR(64) NOT NULL DEFAULT 'v1',
    ADD COLUMN IF NOT EXISTS ai_director_operator_enabled BOOLEAN NOT NULL DEFAULT TRUE;

COMMENT ON COLUMN ops.video_quality_settings.ai_director_operator_instruction IS 'AI1 运营指令模板（可编辑）';
COMMENT ON COLUMN ops.video_quality_settings.ai_director_operator_instruction_version IS 'AI1 运营指令版本';
COMMENT ON COLUMN ops.video_quality_settings.ai_director_operator_enabled IS '是否启用 AI1 运营指令模板';

UPDATE ops.video_quality_settings
SET ai_director_operator_instruction = COALESCE(ai_director_operator_instruction, ''),
    ai_director_operator_instruction_version = COALESCE(NULLIF(ai_director_operator_instruction_version, ''), 'v1'),
    ai_director_operator_enabled = COALESCE(ai_director_operator_enabled, TRUE)
WHERE TRUE;

COMMIT;
