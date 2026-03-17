-- 回填视频任务 started_at（archive + public）
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/backfill_video_job_started_at.sql

BEGIN;

-- 0) 回填前概览
SELECT
  'archive_before' AS section,
  COUNT(*) FILTER (WHERE started_at IS NULL) AS started_at_null_total,
  COUNT(*) FILTER (
    WHERE started_at IS NULL
      AND LOWER(COALESCE(status, '')) IN ('running','done','failed','cancelled')
  ) AS started_at_null_non_queued
FROM archive.video_jobs;

SELECT
  'public_before' AS section,
  COUNT(*) FILTER (WHERE started_at IS NULL) AS started_at_null_total,
  COUNT(*) FILTER (
    WHERE started_at IS NULL
      AND LOWER(COALESCE(status, '')) IN ('running','done','failed','cancelled')
  ) AS started_at_null_non_queued
FROM public.video_image_jobs;

-- 1) archive：优先使用事件链首个非 queued 时间
WITH event_start AS (
  SELECT
    job_id,
    MIN(created_at) FILTER (WHERE LOWER(COALESCE(stage, '')) <> 'queued') AS started_at
  FROM archive.video_job_events
  GROUP BY job_id
), updated AS (
  UPDATE archive.video_jobs j
  SET
    started_at = event_start.started_at,
    updated_at = NOW()
  FROM event_start
  WHERE j.id = event_start.job_id
    AND j.started_at IS NULL
    AND LOWER(COALESCE(j.status, '')) <> 'queued'
    AND event_start.started_at IS NOT NULL
  RETURNING j.id
)
SELECT 'archive_from_events' AS section, COUNT(*) AS updated_rows
FROM updated;

-- 2) archive：兜底（仍为空的非 queued 任务）
WITH updated AS (
  UPDATE archive.video_jobs j
  SET
    started_at = COALESCE(j.finished_at, j.queued_at, j.created_at),
    updated_at = NOW()
  WHERE j.started_at IS NULL
    AND LOWER(COALESCE(j.status, '')) IN ('running','done','failed','cancelled')
    AND COALESCE(j.finished_at, j.queued_at, j.created_at) IS NOT NULL
  RETURNING j.id
)
SELECT 'archive_fallback' AS section, COUNT(*) AS updated_rows
FROM updated;

-- 3) public：优先使用 public 事件链；其次回退 archive.started_at
WITH event_start AS (
  SELECT
    job_id,
    MIN(created_at) FILTER (WHERE LOWER(COALESCE(stage, '')) <> 'queued') AS started_at
  FROM public.video_image_events
  GROUP BY job_id
), updated AS (
  UPDATE public.video_image_jobs j
  SET
    started_at = COALESCE(event_start.started_at, aj.started_at, j.finished_at, j.created_at),
    updated_at = NOW()
  FROM event_start
  LEFT JOIN archive.video_jobs aj ON aj.id = event_start.job_id
  WHERE j.id = event_start.job_id
    AND j.started_at IS NULL
    AND LOWER(COALESCE(j.status, '')) <> 'queued'
    AND COALESCE(event_start.started_at, aj.started_at, j.finished_at, j.created_at) IS NOT NULL
  RETURNING j.id
)
SELECT 'public_from_events' AS section, COUNT(*) AS updated_rows
FROM updated;

-- 4) public：兜底（仍为空的非 queued 任务）
WITH updated AS (
  UPDATE public.video_image_jobs j
  SET
    started_at = COALESCE(aj.started_at, j.finished_at, j.created_at),
    updated_at = NOW()
  FROM archive.video_jobs aj
  WHERE j.id = aj.id
    AND j.started_at IS NULL
    AND LOWER(COALESCE(j.status, '')) IN ('running','done','failed','cancelled')
    AND COALESCE(aj.started_at, j.finished_at, j.created_at) IS NOT NULL
  RETURNING j.id
)
SELECT 'public_fallback' AS section, COUNT(*) AS updated_rows
FROM updated;

-- 5) 回填后概览
SELECT
  'archive_after' AS section,
  COUNT(*) FILTER (WHERE started_at IS NULL) AS started_at_null_total,
  COUNT(*) FILTER (
    WHERE started_at IS NULL
      AND LOWER(COALESCE(status, '')) IN ('running','done','failed','cancelled')
  ) AS started_at_null_non_queued
FROM archive.video_jobs;

SELECT
  'public_after' AS section,
  COUNT(*) FILTER (WHERE started_at IS NULL) AS started_at_null_total,
  COUNT(*) FILTER (
    WHERE started_at IS NULL
      AND LOWER(COALESCE(status, '')) IN ('running','done','failed','cancelled')
  ) AS started_at_null_non_queued
FROM public.video_image_jobs;

COMMIT;
