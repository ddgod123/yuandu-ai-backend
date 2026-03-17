-- 任务级 ZIP 巡检脚本（按 job_id）
-- 用法：
--   psql "$DATABASE_URL" -v job_id=22 -f backend/scripts/check_video_job_package.sql

\if :{?job_id}
\else
\echo '[ERROR] missing required variable: -v job_id=<id>'
\quit 1
\endif

\echo ''
\echo '==[0) 参数确认]==============================================='
SELECT :job_id::bigint AS job_id;

\echo ''
\echo '==[1) 任务状态快照（public + archive）]========================='
WITH pub AS (
  SELECT
    id,
    user_id,
    title,
    requested_format,
    status,
    stage,
    progress,
    error_message,
    started_at,
    finished_at,
    created_at,
    updated_at,
    COALESCE(metrics, '{}'::jsonb) AS metrics
  FROM public.video_image_jobs
  WHERE id = :job_id::bigint
), arc AS (
  SELECT
    id,
    user_id,
    title,
    output_formats,
    status,
    stage,
    progress,
    error_message,
    result_collection_id,
    started_at,
    finished_at,
    created_at,
    updated_at,
    COALESCE(metrics, '{}'::jsonb) AS metrics
  FROM archive.video_jobs
  WHERE id = :job_id::bigint
)
SELECT
  COALESCE(pub.id, arc.id) AS job_id,
  COALESCE(pub.user_id, arc.user_id) AS user_id,
  COALESCE(NULLIF(pub.title, ''), arc.title) AS title,
  pub.requested_format,
  arc.output_formats,
  pub.status AS public_status,
  arc.status AS archive_status,
  pub.stage AS public_stage,
  arc.stage AS archive_stage,
  pub.progress AS public_progress,
  arc.progress AS archive_progress,
  arc.result_collection_id,
  pub.metrics ->> 'package_zip_status' AS public_package_zip_status,
  pub.metrics ->> 'package_zip_attempts' AS public_package_zip_attempts,
  pub.metrics ->> 'package_zip_retry_count' AS public_package_zip_retry_count,
  pub.metrics ->> 'package_zip_error' AS public_package_zip_error,
  pub.metrics ->> 'package_zip_key' AS public_package_zip_key,
  arc.metrics ->> 'package_zip_status' AS archive_package_zip_status,
  arc.metrics ->> 'package_zip_attempts' AS archive_package_zip_attempts,
  arc.metrics ->> 'package_zip_retry_count' AS archive_package_zip_retry_count,
  arc.metrics ->> 'package_zip_error' AS archive_package_zip_error,
  arc.metrics ->> 'package_zip_key' AS archive_package_zip_key,
  COALESCE(pub.error_message, arc.error_message) AS error_message,
  COALESCE(pub.started_at, arc.started_at) AS started_at,
  COALESCE(pub.finished_at, arc.finished_at) AS finished_at,
  COALESCE(pub.created_at, arc.created_at) AS created_at,
  COALESCE(pub.updated_at, arc.updated_at) AS updated_at
FROM pub
FULL OUTER JOIN arc ON arc.id = pub.id;

\echo ''
\echo '==[2) 输出文件概览（public.video_image_outputs）]================='
SELECT
  COUNT(*) AS outputs_total,
  COUNT(*) FILTER (WHERE file_role = 'main') AS main_outputs,
  COUNT(*) FILTER (WHERE file_role = 'package' OR format = 'zip') AS package_outputs,
  COUNT(*) FILTER (WHERE COALESCE(object_key, '') = '') AS missing_object_key,
  MIN(created_at) AS first_output_at,
  MAX(created_at) AS last_output_at
FROM public.video_image_outputs
WHERE job_id = :job_id::bigint;

SELECT
  format,
  file_role,
  COUNT(*) AS cnt,
  ROUND(AVG(size_bytes)) AS avg_size_bytes,
  MAX(size_bytes) AS max_size_bytes
FROM public.video_image_outputs
WHERE job_id = :job_id::bigint
GROUP BY 1, 2
ORDER BY 1, 2;

\echo ''
\echo '==[3) ZIP落库链路（多表对照）]=================================='
WITH job AS (
  SELECT result_collection_id
  FROM archive.video_jobs
  WHERE id = :job_id::bigint
)
SELECT
  'public.video_image_packages'::text AS source,
  p.id::text AS ref_id,
  p.zip_object_key AS object_key,
  p.zip_name AS file_name,
  p.zip_size_bytes AS size_bytes,
  p.created_at AS created_at,
  NULL::text AS extra
FROM public.video_image_packages p
WHERE p.job_id = :job_id::bigint

UNION ALL

SELECT
  'archive.video_job_artifacts(type=package)'::text AS source,
  a.id::text AS ref_id,
  a.qiniu_key AS object_key,
  a.mime_type AS file_name,
  a.size_bytes AS size_bytes,
  a.created_at AS created_at,
  ('type=' || COALESCE(a.type, ''))::text AS extra
FROM archive.video_job_artifacts a
WHERE a.job_id = :job_id::bigint
  AND (a.type = 'package' OR a.qiniu_key LIKE '%.zip')

UNION ALL

SELECT
  'archive.collections.latest_zip'::text AS source,
  c.id::text AS ref_id,
  c.latest_zip_key AS object_key,
  c.latest_zip_name AS file_name,
  c.latest_zip_size AS size_bytes,
  c.latest_zip_at AS created_at,
  ('collection_id=' || c.id::text)::text AS extra
FROM archive.collections c
JOIN job j ON j.result_collection_id = c.id

UNION ALL

SELECT
  'archive.collection_zips'::text AS source,
  z.id::text AS ref_id,
  z.zip_key AS object_key,
  z.zip_name AS file_name,
  z.size_bytes AS size_bytes,
  z.uploaded_at AS created_at,
  ('collection_id=' || z.collection_id::text)::text AS extra
FROM archive.collection_zips z
JOIN job j ON j.result_collection_id = z.collection_id
ORDER BY source, created_at DESC NULLS LAST;

\echo ''
\echo '==[4) 一致性判断 + 健康结论]===================================='
WITH j AS (
  SELECT
    id,
    LOWER(COALESCE(status, '')) AS status,
    LOWER(COALESCE(stage, '')) AS stage,
    progress,
    COALESCE(metrics, '{}'::jsonb) AS metrics
  FROM public.video_image_jobs
  WHERE id = :job_id::bigint
),
out_main AS (
  SELECT COUNT(*) AS cnt
  FROM public.video_image_outputs
  WHERE job_id = :job_id::bigint
    AND file_role = 'main'
),
pkg AS (
  SELECT COALESCE(MAX(NULLIF(BTRIM(zip_object_key), '')), '') AS key
  FROM public.video_image_packages
  WHERE job_id = :job_id::bigint
),
art AS (
  SELECT COALESCE(MAX(NULLIF(BTRIM(qiniu_key), '')), '') AS key
  FROM archive.video_job_artifacts
  WHERE job_id = :job_id::bigint
    AND (type = 'package' OR qiniu_key LIKE '%.zip')
),
col AS (
  SELECT COALESCE(MAX(NULLIF(BTRIM(c.latest_zip_key), '')), '') AS key
  FROM archive.video_jobs j2
  LEFT JOIN archive.collections c ON c.id = j2.result_collection_id
  WHERE j2.id = :job_id::bigint
),
verdict AS (
  SELECT
    COALESCE((SELECT status FROM j), '') AS job_status,
    COALESCE((SELECT stage FROM j), '') AS job_stage,
    COALESCE((SELECT (metrics ->> 'package_zip_status') FROM j), '') AS package_zip_status,
    COALESCE((SELECT (metrics ->> 'package_zip_attempts') FROM j), '') AS package_zip_attempts,
    COALESCE((SELECT (metrics ->> 'package_zip_retry_count') FROM j), '') AS package_zip_retry_count,
    COALESCE((SELECT (metrics ->> 'package_zip_error') FROM j), '') AS package_zip_error,
    (SELECT cnt FROM out_main) AS main_output_count,
    (SELECT key FROM pkg) AS public_pkg_key,
    (SELECT key FROM art) AS artifact_pkg_key,
    (SELECT key FROM col) AS collection_latest_zip_key
)
SELECT
  job_status,
  job_stage,
  main_output_count,
  package_zip_status,
  package_zip_attempts,
  package_zip_retry_count,
  package_zip_error,
  public_pkg_key,
  artifact_pkg_key,
  collection_latest_zip_key,
  CASE
    WHEN job_status = '' THEN 'RED'
    WHEN job_status = 'failed' THEN 'RED'
    WHEN job_status IN ('queued', 'running') THEN 'YELLOW'
    WHEN main_output_count <= 0 THEN 'RED'
    WHEN package_zip_status = 'failed' THEN 'YELLOW'
    WHEN public_pkg_key = '' THEN 'YELLOW'
    ELSE 'GREEN'
  END AS health,
  CASE
    WHEN job_status = '' THEN 'job_not_found_in_public'
    WHEN job_status = 'failed' THEN 'job_failed'
    WHEN job_status IN ('queued', 'running') THEN 'job_processing'
    WHEN main_output_count <= 0 THEN 'done_without_main_output'
    WHEN package_zip_status = 'failed' THEN 'zip_failed_but_job_done'
    WHEN public_pkg_key = '' THEN 'zip_missing_or_not_synced'
    ELSE 'all_good'
  END AS reason,
  CASE
    WHEN public_pkg_key = '' OR artifact_pkg_key = '' OR public_pkg_key = artifact_pkg_key THEN 'ok'
    ELSE 'mismatch'
  END AS public_vs_artifact,
  CASE
    WHEN public_pkg_key = '' OR collection_latest_zip_key = '' OR public_pkg_key = collection_latest_zip_key THEN 'ok'
    ELSE 'mismatch'
  END AS public_vs_collection
FROM verdict;

\echo ''
\echo '==[5) ZIP相关事件（最近20条）]=================================='
SELECT
  created_at,
  level,
  stage,
  message,
  COALESCE(metadata ->> 'attempt', metadata ->> 'attempts', '') AS attempt,
  COALESCE(metadata ->> 'error', '') AS error
FROM public.video_image_events
WHERE job_id = :job_id::bigint
  AND (
    message ILIKE 'zip package%'
    OR metadata ? 'attempt'
    OR metadata ? 'attempts'
    OR metadata ? 'package_zip_status'
  )
ORDER BY created_at DESC
LIMIT 20;

\echo ''
\echo '==[6) 前端预期展示（我的作品）]=================================='
WITH j AS (
  SELECT
    LOWER(COALESCE(status, '')) AS status,
    LOWER(COALESCE(metrics ->> 'package_zip_status', '')) AS package_zip_status
  FROM public.video_image_jobs
  WHERE id = :job_id::bigint
),
p AS (
  SELECT COALESCE(MAX(NULLIF(BTRIM(zip_object_key), '')), '') AS zip_key
  FROM public.video_image_packages
  WHERE job_id = :job_id::bigint
)
SELECT
  CASE
    WHEN (SELECT zip_key FROM p) <> '' THEN 'ZIP 已就绪'
    WHEN (SELECT package_zip_status FROM j) = 'failed' THEN 'ZIP 生成失败'
    WHEN (SELECT status FROM j) = 'done' THEN 'ZIP 生成失败'
    ELSE 'ZIP 处理中'
  END AS expected_badge,
  CASE
    WHEN (SELECT zip_key FROM p) <> '' THEN '显示下载 ZIP 按钮'
    WHEN (SELECT package_zip_status FROM j) = 'failed' THEN '显示失败提示 + 重试次数/错误信息'
    WHEN (SELECT status FROM j) = 'done' THEN '显示失败提示（兜底）'
    ELSE '显示处理中提示并建议刷新'
  END AS expected_detail_hint;
