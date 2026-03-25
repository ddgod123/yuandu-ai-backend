BEGIN;

CREATE TABLE IF NOT EXISTS ops.collection_copyright_tasks (
  id BIGSERIAL PRIMARY KEY,
  task_no VARCHAR(64) NOT NULL UNIQUE,
  collection_id BIGINT NOT NULL,
  run_mode VARCHAR(16) NOT NULL,
  sample_strategy VARCHAR(16) NOT NULL,
  sample_count INTEGER NOT NULL DEFAULT 0,
  actual_sample_count INTEGER NOT NULL DEFAULT 0,
  enable_tagging BOOLEAN NOT NULL DEFAULT TRUE,
  overwrite_machine_tags BOOLEAN NOT NULL DEFAULT TRUE,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  progress INTEGER NOT NULL DEFAULT 0,
  high_risk_count INTEGER NOT NULL DEFAULT 0,
  unknown_source_count INTEGER NOT NULL DEFAULT 0,
  ip_hit_count INTEGER NOT NULL DEFAULT 0,
  machine_conclusion VARCHAR(64) NOT NULL DEFAULT '',
  result_summary TEXT NOT NULL DEFAULT '',
  created_by BIGINT,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_collection_copyright_tasks_collection_id
  ON ops.collection_copyright_tasks(collection_id);

CREATE INDEX IF NOT EXISTS idx_collection_copyright_tasks_status
  ON ops.collection_copyright_tasks(status);

CREATE INDEX IF NOT EXISTS idx_collection_copyright_tasks_created_by
  ON ops.collection_copyright_tasks(created_by);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_collection_copyright_tasks_collection'
  ) THEN
    ALTER TABLE ops.collection_copyright_tasks
      ADD CONSTRAINT fk_collection_copyright_tasks_collection
      FOREIGN KEY (collection_id) REFERENCES archive.collections(id) ON DELETE CASCADE;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS ops.collection_copyright_task_images (
  id BIGSERIAL PRIMARY KEY,
  task_id BIGINT NOT NULL,
  collection_id BIGINT NOT NULL,
  emoji_id BIGINT NOT NULL,
  sample_order INTEGER NOT NULL DEFAULT 0,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  error_msg TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(task_id, emoji_id)
);

CREATE INDEX IF NOT EXISTS idx_collection_copyright_task_images_collection_id
  ON ops.collection_copyright_task_images(collection_id);

CREATE INDEX IF NOT EXISTS idx_collection_copyright_task_images_status
  ON ops.collection_copyright_task_images(status);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_collection_copyright_task_images_task'
  ) THEN
    ALTER TABLE ops.collection_copyright_task_images
      ADD CONSTRAINT fk_collection_copyright_task_images_task
      FOREIGN KEY (task_id) REFERENCES ops.collection_copyright_tasks(id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_collection_copyright_task_images_collection'
  ) THEN
    ALTER TABLE ops.collection_copyright_task_images
      ADD CONSTRAINT fk_collection_copyright_task_images_collection
      FOREIGN KEY (collection_id) REFERENCES archive.collections(id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_collection_copyright_task_images_emoji'
  ) THEN
    ALTER TABLE ops.collection_copyright_task_images
      ADD CONSTRAINT fk_collection_copyright_task_images_emoji
      FOREIGN KEY (emoji_id) REFERENCES archive.emojis(id) ON DELETE CASCADE;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS ops.image_copyright_results (
  id BIGSERIAL PRIMARY KEY,
  task_id BIGINT NOT NULL,
  collection_id BIGINT NOT NULL,
  emoji_id BIGINT NOT NULL,
  ocr_text TEXT NOT NULL DEFAULT '',
  content_type VARCHAR(64) NOT NULL DEFAULT '',
  copyright_owner_guess VARCHAR(255) NOT NULL DEFAULT '',
  owner_type VARCHAR(64) NOT NULL DEFAULT 'unknown',
  is_commercial_ip BOOLEAN NOT NULL DEFAULT FALSE,
  ip_name VARCHAR(255) NOT NULL DEFAULT '',
  is_brand_related BOOLEAN NOT NULL DEFAULT FALSE,
  brand_name VARCHAR(255) NOT NULL DEFAULT '',
  is_celebrity_related BOOLEAN NOT NULL DEFAULT FALSE,
  celebrity_name VARCHAR(255) NOT NULL DEFAULT '',
  is_screenshot BOOLEAN NOT NULL DEFAULT FALSE,
  is_source_unknown BOOLEAN NOT NULL DEFAULT FALSE,
  rights_status VARCHAR(64) NOT NULL DEFAULT 'unknown',
  commercial_use_advice VARCHAR(64) NOT NULL DEFAULT 'not_recommended',
  risk_level VARCHAR(16) NOT NULL DEFAULT 'L1',
  risk_score NUMERIC(5,2) NOT NULL DEFAULT 0,
  model_confidence NUMERIC(5,2) NOT NULL DEFAULT 0,
  evidence_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  machine_summary TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(task_id, emoji_id)
);

CREATE INDEX IF NOT EXISTS idx_image_copyright_results_collection_id
  ON ops.image_copyright_results(collection_id);

CREATE INDEX IF NOT EXISTS idx_image_copyright_results_risk_level
  ON ops.image_copyright_results(risk_level);

CREATE INDEX IF NOT EXISTS idx_image_copyright_results_ip_name
  ON ops.image_copyright_results(ip_name);

CREATE INDEX IF NOT EXISTS idx_image_copyright_results_brand_name
  ON ops.image_copyright_results(brand_name);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_image_copyright_results_task'
  ) THEN
    ALTER TABLE ops.image_copyright_results
      ADD CONSTRAINT fk_image_copyright_results_task
      FOREIGN KEY (task_id) REFERENCES ops.collection_copyright_tasks(id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_image_copyright_results_collection'
  ) THEN
    ALTER TABLE ops.image_copyright_results
      ADD CONSTRAINT fk_image_copyright_results_collection
      FOREIGN KEY (collection_id) REFERENCES archive.collections(id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_image_copyright_results_emoji'
  ) THEN
    ALTER TABLE ops.image_copyright_results
      ADD CONSTRAINT fk_image_copyright_results_emoji
      FOREIGN KEY (emoji_id) REFERENCES archive.emojis(id) ON DELETE CASCADE;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS ops.collection_copyright_results (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL UNIQUE,
  latest_task_id BIGINT NOT NULL,
  run_mode VARCHAR(16) NOT NULL,
  sample_coverage NUMERIC(5,2) NOT NULL DEFAULT 0,
  machine_conclusion VARCHAR(64) NOT NULL DEFAULT '',
  machine_confidence NUMERIC(5,2) NOT NULL DEFAULT 0,
  risk_level VARCHAR(16) NOT NULL DEFAULT 'L1',
  sampled_image_count INTEGER NOT NULL DEFAULT 0,
  high_risk_count INTEGER NOT NULL DEFAULT 0,
  unknown_source_count INTEGER NOT NULL DEFAULT 0,
  ip_hit_count INTEGER NOT NULL DEFAULT 0,
  brand_hit_count INTEGER NOT NULL DEFAULT 0,
  recommended_action VARCHAR(64) NOT NULL DEFAULT '',
  review_status VARCHAR(32) NOT NULL DEFAULT 'unreviewed',
  final_decision VARCHAR(64) NOT NULL DEFAULT '',
  final_reviewer_id BIGINT,
  final_reviewed_at TIMESTAMPTZ,
  summary TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_collection_copyright_results_latest_task_id
  ON ops.collection_copyright_results(latest_task_id);

CREATE INDEX IF NOT EXISTS idx_collection_copyright_results_risk_level
  ON ops.collection_copyright_results(risk_level);

CREATE INDEX IF NOT EXISTS idx_collection_copyright_results_review_status
  ON ops.collection_copyright_results(review_status);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_collection_copyright_results_collection'
  ) THEN
    ALTER TABLE ops.collection_copyright_results
      ADD CONSTRAINT fk_collection_copyright_results_collection
      FOREIGN KEY (collection_id) REFERENCES archive.collections(id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_collection_copyright_results_latest_task'
  ) THEN
    ALTER TABLE ops.collection_copyright_results
      ADD CONSTRAINT fk_collection_copyright_results_latest_task
      FOREIGN KEY (latest_task_id) REFERENCES ops.collection_copyright_tasks(id) ON DELETE CASCADE;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS ops.copyright_review_records (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL,
  emoji_id BIGINT,
  task_id BIGINT,
  review_type VARCHAR(16) NOT NULL,
  review_status VARCHAR(32) NOT NULL,
  review_result VARCHAR(64) NOT NULL DEFAULT '',
  review_comment TEXT NOT NULL DEFAULT '',
  reviewer_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_copyright_review_records_collection_id
  ON ops.copyright_review_records(collection_id);

CREATE INDEX IF NOT EXISTS idx_copyright_review_records_emoji_id
  ON ops.copyright_review_records(emoji_id);

CREATE INDEX IF NOT EXISTS idx_copyright_review_records_task_id
  ON ops.copyright_review_records(task_id);

CREATE TABLE IF NOT EXISTS taxonomy.tag_dimensions (
  id BIGSERIAL PRIMARY KEY,
  dimension_code VARCHAR(64) NOT NULL UNIQUE,
  dimension_name VARCHAR(64) NOT NULL,
  sort_no INTEGER NOT NULL DEFAULT 0,
  status SMALLINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS taxonomy.tag_definitions (
  id BIGSERIAL PRIMARY KEY,
  tag_code VARCHAR(128) NOT NULL UNIQUE,
  tag_name VARCHAR(128) NOT NULL,
  dimension_code VARCHAR(64) NOT NULL,
  tag_level VARCHAR(16) NOT NULL DEFAULT 'image',
  is_system BOOLEAN NOT NULL DEFAULT FALSE,
  sort_no INTEGER NOT NULL DEFAULT 0,
  status SMALLINT NOT NULL DEFAULT 1,
  remark VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS taxonomy.emoji_auto_tags (
  id BIGSERIAL PRIMARY KEY,
  emoji_id BIGINT NOT NULL,
  collection_id BIGINT NOT NULL,
  task_id BIGINT,
  tag_id BIGINT NOT NULL,
  source VARCHAR(16) NOT NULL,
  confidence NUMERIC(5,2) NOT NULL DEFAULT 0,
  model_version VARCHAR(64) NOT NULL DEFAULT '',
  status SMALLINT NOT NULL DEFAULT 1,
  created_by BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_emoji_auto_tags_emoji_id
  ON taxonomy.emoji_auto_tags(emoji_id);

CREATE INDEX IF NOT EXISTS idx_emoji_auto_tags_collection_id
  ON taxonomy.emoji_auto_tags(collection_id);

CREATE INDEX IF NOT EXISTS idx_emoji_auto_tags_tag_id
  ON taxonomy.emoji_auto_tags(tag_id);

CREATE TABLE IF NOT EXISTS taxonomy.collection_auto_tags (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL,
  task_id BIGINT,
  tag_id BIGINT NOT NULL,
  source VARCHAR(16) NOT NULL,
  confidence NUMERIC(5,2) NOT NULL DEFAULT 0,
  model_version VARCHAR(64) NOT NULL DEFAULT '',
  status SMALLINT NOT NULL DEFAULT 1,
  created_by BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_collection_auto_tags_collection_id
  ON taxonomy.collection_auto_tags(collection_id);

CREATE INDEX IF NOT EXISTS idx_collection_auto_tags_tag_id
  ON taxonomy.collection_auto_tags(tag_id);

CREATE TABLE IF NOT EXISTS ops.copyright_evidences (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL,
  emoji_id BIGINT,
  task_id BIGINT,
  evidence_type VARCHAR(32) NOT NULL,
  evidence_title VARCHAR(255) NOT NULL DEFAULT '',
  evidence_value TEXT NOT NULL DEFAULT '',
  evidence_url VARCHAR(1024) NOT NULL DEFAULT '',
  extra_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_copyright_evidences_collection_id
  ON ops.copyright_evidences(collection_id);

CREATE INDEX IF NOT EXISTS idx_copyright_evidences_emoji_id
  ON ops.copyright_evidences(emoji_id);

CREATE INDEX IF NOT EXISTS idx_copyright_evidences_task_id
  ON ops.copyright_evidences(task_id);

CREATE TABLE IF NOT EXISTS ops.copyright_task_logs (
  id BIGSERIAL PRIMARY KEY,
  task_id BIGINT NOT NULL,
  emoji_id BIGINT,
  stage VARCHAR(32) NOT NULL,
  status VARCHAR(16) NOT NULL,
  message VARCHAR(1024) NOT NULL DEFAULT '',
  detail_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_copyright_task_logs_task_id
  ON ops.copyright_task_logs(task_id);

CREATE INDEX IF NOT EXISTS idx_copyright_task_logs_emoji_id
  ON ops.copyright_task_logs(emoji_id);

INSERT INTO taxonomy.tag_dimensions (dimension_code, dimension_name, sort_no, status)
VALUES
  ('subject', '主体', 10, 1),
  ('emotion', '情绪', 20, 1),
  ('action', '动作', 30, 1),
  ('style', '风格', 40, 1),
  ('text_semantic', '文本语义', 50, 1),
  ('scene', '适用场景', 60, 1),
  ('copyright_attr', '版权属性', 70, 1),
  ('collection_theme', '合集主题', 80, 1)
ON CONFLICT (dimension_code) DO UPDATE
SET
  dimension_name = EXCLUDED.dimension_name,
  sort_no = EXCLUDED.sort_no,
  status = EXCLUDED.status,
  updated_at = NOW();

COMMIT;
