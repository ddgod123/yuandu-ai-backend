BEGIN;

-- Deduplicate artifacts produced by the same job/key pair, keep the newest row.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY job_id, qiniu_key ORDER BY id DESC) AS rn
    FROM archive.video_job_artifacts
    WHERE qiniu_key <> ''
)
DELETE FROM archive.video_job_artifacts a
USING ranked r
WHERE a.id = r.id
  AND r.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS uidx_video_job_artifacts_job_key
    ON archive.video_job_artifacts(job_id, qiniu_key)
    WHERE qiniu_key <> '';

-- Deduplicate active emojis in the same collection/file pair, keep newest active row.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY collection_id, file_url ORDER BY id DESC) AS rn
    FROM archive.emojis
    WHERE deleted_at IS NULL
      AND file_url <> ''
)
UPDATE archive.emojis e
SET deleted_at = NOW(),
    updated_at = NOW()
FROM ranked r
WHERE e.id = r.id
  AND r.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS uidx_archive_emojis_collection_file_active
    ON archive.emojis(collection_id, file_url)
    WHERE deleted_at IS NULL
      AND file_url <> '';

COMMIT;
