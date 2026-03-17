BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS feedback_integrity_output_coverage_rate_warn DOUBLE PRECISION NOT NULL DEFAULT 0.98,
    ADD COLUMN IF NOT EXISTS feedback_integrity_output_coverage_rate_critical DOUBLE PRECISION NOT NULL DEFAULT 0.95,
    ADD COLUMN IF NOT EXISTS feedback_integrity_output_resolved_rate_warn DOUBLE PRECISION NOT NULL DEFAULT 0.99,
    ADD COLUMN IF NOT EXISTS feedback_integrity_output_resolved_rate_critical DOUBLE PRECISION NOT NULL DEFAULT 0.97,
    ADD COLUMN IF NOT EXISTS feedback_integrity_output_job_consistency_rate_warn DOUBLE PRECISION NOT NULL DEFAULT 0.999,
    ADD COLUMN IF NOT EXISTS feedback_integrity_output_job_consistency_rate_critical DOUBLE PRECISION NOT NULL DEFAULT 0.995,
    ADD COLUMN IF NOT EXISTS feedback_integrity_top_pick_conflict_users_warn INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS feedback_integrity_top_pick_conflict_users_critical INTEGER NOT NULL DEFAULT 3;

COMMENT ON COLUMN ops.video_quality_settings.feedback_integrity_output_coverage_rate_warn IS '反馈完整性告警阈值：output_id覆盖率低于该值记为warn';
COMMENT ON COLUMN ops.video_quality_settings.feedback_integrity_output_coverage_rate_critical IS '反馈完整性告警阈值：output_id覆盖率低于该值记为critical';
COMMENT ON COLUMN ops.video_quality_settings.feedback_integrity_output_resolved_rate_warn IS '反馈完整性告警阈值：output_id可解析率低于该值记为warn';
COMMENT ON COLUMN ops.video_quality_settings.feedback_integrity_output_resolved_rate_critical IS '反馈完整性告警阈值：output_id可解析率低于该值记为critical';
COMMENT ON COLUMN ops.video_quality_settings.feedback_integrity_output_job_consistency_rate_warn IS '反馈完整性告警阈值：output_id与job对齐率低于该值记为warn';
COMMENT ON COLUMN ops.video_quality_settings.feedback_integrity_output_job_consistency_rate_critical IS '反馈完整性告警阈值：output_id与job对齐率低于该值记为critical';
COMMENT ON COLUMN ops.video_quality_settings.feedback_integrity_top_pick_conflict_users_warn IS '反馈完整性告警阈值：top_pick冲突用户数高于该值记为warn';
COMMENT ON COLUMN ops.video_quality_settings.feedback_integrity_top_pick_conflict_users_critical IS '反馈完整性告警阈值：top_pick冲突用户数高于该值记为critical';

UPDATE ops.video_quality_settings
SET feedback_integrity_output_coverage_rate_warn = LEAST(GREATEST(COALESCE(feedback_integrity_output_coverage_rate_warn, 0.98), 0.50), 1.00),
    feedback_integrity_output_coverage_rate_critical = LEAST(GREATEST(COALESCE(feedback_integrity_output_coverage_rate_critical, 0.95), 0.01), 0.999),
    feedback_integrity_output_resolved_rate_warn = LEAST(GREATEST(COALESCE(feedback_integrity_output_resolved_rate_warn, 0.99), 0.50), 1.00),
    feedback_integrity_output_resolved_rate_critical = LEAST(GREATEST(COALESCE(feedback_integrity_output_resolved_rate_critical, 0.97), 0.01), 0.999),
    feedback_integrity_output_job_consistency_rate_warn = LEAST(GREATEST(COALESCE(feedback_integrity_output_job_consistency_rate_warn, 0.999), 0.50), 1.00),
    feedback_integrity_output_job_consistency_rate_critical = LEAST(GREATEST(COALESCE(feedback_integrity_output_job_consistency_rate_critical, 0.995), 0.01), 0.999),
    feedback_integrity_top_pick_conflict_users_warn = GREATEST(COALESCE(feedback_integrity_top_pick_conflict_users_warn, 1), 1),
    feedback_integrity_top_pick_conflict_users_critical = GREATEST(COALESCE(feedback_integrity_top_pick_conflict_users_critical, 3), 1)
WHERE id = 1;

UPDATE ops.video_quality_settings
SET feedback_integrity_output_coverage_rate_critical = LEAST(
      feedback_integrity_output_coverage_rate_critical,
      GREATEST(feedback_integrity_output_coverage_rate_warn - 0.001, 0.01)
    ),
    feedback_integrity_output_resolved_rate_critical = LEAST(
      feedback_integrity_output_resolved_rate_critical,
      GREATEST(feedback_integrity_output_resolved_rate_warn - 0.001, 0.01)
    ),
    feedback_integrity_output_job_consistency_rate_critical = LEAST(
      feedback_integrity_output_job_consistency_rate_critical,
      GREATEST(feedback_integrity_output_job_consistency_rate_warn - 0.0001, 0.01)
    ),
    feedback_integrity_top_pick_conflict_users_critical = GREATEST(
      feedback_integrity_top_pick_conflict_users_critical,
      feedback_integrity_top_pick_conflict_users_warn + 1
    )
WHERE id = 1;

COMMIT;
