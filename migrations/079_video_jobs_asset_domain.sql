BEGIN;

ALTER TABLE archive.video_jobs
  ADD COLUMN IF NOT EXISTS asset_domain VARCHAR(32) NOT NULL DEFAULT 'video';

UPDATE archive.video_jobs
SET asset_domain = 'video'
WHERE COALESCE(NULLIF(TRIM(asset_domain), ''), '') = '';

CREATE INDEX IF NOT EXISTS idx_video_jobs_asset_domain
  ON archive.video_jobs(asset_domain);

COMMENT ON COLUMN archive.video_jobs.asset_domain IS '任务产物域：video/admin/ugc/archive';

DO $$
DECLARE
  con_name text;
BEGIN
  SELECT conname INTO con_name
  FROM pg_constraint
  WHERE conrelid = 'archive.video_jobs'::regclass
    AND confrelid = 'archive.collections'::regclass
    AND contype = 'f'
    AND array_position(conkey, (
      SELECT attnum
      FROM pg_attribute
      WHERE attrelid = 'archive.video_jobs'::regclass
        AND attname = 'result_collection_id'
      LIMIT 1
    )) IS NOT NULL
  LIMIT 1;

  IF con_name IS NOT NULL THEN
    EXECUTE format('ALTER TABLE archive.video_jobs DROP CONSTRAINT %I', con_name);
  END IF;
END
$$;

COMMIT;
