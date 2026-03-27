-- AI 图像复审（PNG/JPG/WebP/LIVE/MP4）落库表
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/migrate_video_job_image_ai_reviews.sql

CREATE SCHEMA IF NOT EXISTS archive;

CREATE TABLE IF NOT EXISTS archive.video_job_image_ai_reviews (
  id BIGSERIAL PRIMARY KEY,
  job_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL DEFAULT 0,
  target_format VARCHAR(16) NOT NULL,
  stage VARCHAR(32) NOT NULL DEFAULT 'ai3',
  recommendation VARCHAR(32) NOT NULL DEFAULT 'deliver',
  reviewed_outputs INTEGER NOT NULL DEFAULT 0,
  deliver_count INTEGER NOT NULL DEFAULT 0,
  reject_count INTEGER NOT NULL DEFAULT 0,
  manual_review_count INTEGER NOT NULL DEFAULT 0,
  hard_gate_reject_count INTEGER NOT NULL DEFAULT 0,
  hard_gate_manual_review_count INTEGER NOT NULL DEFAULT 0,
  candidate_budget INTEGER NOT NULL DEFAULT 0,
  effective_duration_sec DOUBLE PRECISION NOT NULL DEFAULT 0,
  quality_fallback BOOLEAN NOT NULL DEFAULT FALSE,
  quality_selector_version VARCHAR(64) NOT NULL DEFAULT '',
  summary_note TEXT NOT NULL DEFAULT '',
  summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_video_job_image_ai_reviews_job_format
  ON archive.video_job_image_ai_reviews (job_id, target_format);

CREATE INDEX IF NOT EXISTS idx_video_job_image_ai_reviews_user_id
  ON archive.video_job_image_ai_reviews (user_id);

CREATE INDEX IF NOT EXISTS idx_video_job_image_ai_reviews_target_format
  ON archive.video_job_image_ai_reviews (target_format);

CREATE INDEX IF NOT EXISTS idx_video_job_image_ai_reviews_recommendation
  ON archive.video_job_image_ai_reviews (recommendation);

CREATE INDEX IF NOT EXISTS idx_video_job_image_ai_reviews_stage
  ON archive.video_job_image_ai_reviews (stage);

