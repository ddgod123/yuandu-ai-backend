-- 为 ops.video_quality_settings 增加 AI3 技术硬闸门参数（可视化配置用）
BEGIN;

ALTER TABLE IF EXISTS ops.video_quality_settings
  ADD COLUMN IF NOT EXISTS gif_ai_judge_hard_gate_min_overall_score DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS gif_ai_judge_hard_gate_min_clarity_score DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS gif_ai_judge_hard_gate_min_loop_score DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS gif_ai_judge_hard_gate_min_output_score DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS gif_ai_judge_hard_gate_min_duration_ms INTEGER,
  ADD COLUMN IF NOT EXISTS gif_ai_judge_hard_gate_size_multiplier INTEGER;

UPDATE ops.video_quality_settings
SET
  gif_ai_judge_hard_gate_min_overall_score = COALESCE(gif_ai_judge_hard_gate_min_overall_score, 0.20),
  gif_ai_judge_hard_gate_min_clarity_score = COALESCE(gif_ai_judge_hard_gate_min_clarity_score, 0.20),
  gif_ai_judge_hard_gate_min_loop_score = COALESCE(gif_ai_judge_hard_gate_min_loop_score, 0.20),
  gif_ai_judge_hard_gate_min_output_score = COALESCE(gif_ai_judge_hard_gate_min_output_score, 0.20),
  gif_ai_judge_hard_gate_min_duration_ms = COALESCE(gif_ai_judge_hard_gate_min_duration_ms, 200),
  gif_ai_judge_hard_gate_size_multiplier = COALESCE(gif_ai_judge_hard_gate_size_multiplier, 4);

COMMIT;

