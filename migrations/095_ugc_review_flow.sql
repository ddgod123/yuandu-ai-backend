BEGIN;

CREATE TABLE IF NOT EXISTS ops.ugc_collection_review_states (
  collection_id BIGINT PRIMARY KEY REFERENCES archive.collections(id) ON DELETE CASCADE,
  owner_id BIGINT NOT NULL REFERENCES "user".users(id),
  review_status VARCHAR(16) NOT NULL DEFAULT 'draft'
    CHECK (review_status IN ('draft', 'reviewing', 'approved', 'rejected')),
  publish_status VARCHAR(16) NOT NULL DEFAULT 'offline'
    CHECK (publish_status IN ('offline', 'online')),
  submit_count INTEGER NOT NULL DEFAULT 0,
  last_submitted_at TIMESTAMPTZ,
  last_reviewed_at TIMESTAMPTZ,
  last_reviewer_id BIGINT REFERENCES "user".users(id),
  reject_reason TEXT NOT NULL DEFAULT '',
  offline_reason TEXT NOT NULL DEFAULT '',
  last_content_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_approved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE ops.ugc_collection_review_states IS 'UGC合集审核/上下架状态（每合集单行）';
COMMENT ON COLUMN ops.ugc_collection_review_states.review_status IS '审核状态：draft/reviewing/approved/rejected';
COMMENT ON COLUMN ops.ugc_collection_review_states.publish_status IS '上架状态：offline/online';

CREATE INDEX IF NOT EXISTS idx_ugc_review_states_owner_updated
  ON ops.ugc_collection_review_states(owner_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_ugc_review_states_review
  ON ops.ugc_collection_review_states(review_status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_ugc_review_states_public
  ON ops.ugc_collection_review_states(review_status, publish_status, updated_at DESC);

CREATE TABLE IF NOT EXISTS audit.ugc_collection_review_logs (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL REFERENCES archive.collections(id) ON DELETE CASCADE,
  owner_id BIGINT NOT NULL REFERENCES "user".users(id),
  action VARCHAR(32) NOT NULL
    CHECK (action IN (
      'submit_review',
      'withdraw_review',
      'approve',
      'reject',
      'publish',
      'unpublish',
      'admin_offline',
      'content_changed'
    )),
  from_review_status VARCHAR(16),
  to_review_status VARCHAR(16),
  from_publish_status VARCHAR(16),
  to_publish_status VARCHAR(16),
  operator_role VARCHAR(16) NOT NULL
    CHECK (operator_role IN ('user', 'admin', 'system')),
  operator_id BIGINT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  snapshot_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE audit.ugc_collection_review_logs IS 'UGC合集审核与上下架操作日志';

CREATE INDEX IF NOT EXISTS idx_ugc_review_logs_collection_time
  ON audit.ugc_collection_review_logs(collection_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ugc_review_logs_owner_time
  ON audit.ugc_collection_review_logs(owner_id, created_at DESC);

INSERT INTO ops.ugc_collection_review_states (
  collection_id,
  owner_id,
  review_status,
  publish_status,
  submit_count,
  last_submitted_at,
  created_at,
  updated_at,
  last_content_changed_at
)
SELECT
  c.id,
  c.owner_id,
  CASE
    WHEN LOWER(COALESCE(c.status, '')) = 'pending' THEN 'reviewing'
    WHEN LOWER(COALESCE(c.status, '')) = 'active'
         AND LOWER(COALESCE(c.visibility, '')) = 'public' THEN 'approved'
    ELSE 'draft'
  END AS review_status,
  CASE
    WHEN LOWER(COALESCE(c.status, '')) = 'active'
         AND LOWER(COALESCE(c.visibility, '')) = 'public' THEN 'online'
    ELSE 'offline'
  END AS publish_status,
  CASE
    WHEN LOWER(COALESCE(c.status, '')) = 'pending' THEN 1
    ELSE 0
  END AS submit_count,
  CASE
    WHEN LOWER(COALESCE(c.status, '')) = 'pending' THEN c.updated_at
    ELSE NULL
  END AS last_submitted_at,
  NOW(),
  NOW(),
  COALESCE(c.updated_at, NOW())
FROM archive.collections c
WHERE LOWER(COALESCE(c.source, '')) = 'ugc_upload'
ON CONFLICT (collection_id) DO NOTHING;

COMMIT;
