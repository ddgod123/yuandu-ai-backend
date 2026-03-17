-- GIF 巡检脚本（最近 24h）
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/check_gif_health.sql

-- 0) 基础表与关键字段是否存在
WITH required_columns AS (
  SELECT *
  FROM (VALUES
    ('video_image_jobs', 'id'),
    ('video_image_jobs', 'requested_format'),
    ('video_image_jobs', 'status'),
    ('video_image_jobs', 'stage'),
    ('video_image_jobs', 'error_message'),
    ('video_image_jobs', 'created_at'),
    ('video_image_outputs', 'job_id'),
    ('video_image_outputs', 'format'),
    ('video_image_outputs', 'file_role'),
    ('video_image_outputs', 'object_key'),
    ('video_image_outputs', 'size_bytes'),
    ('video_image_outputs', 'width'),
    ('video_image_outputs', 'height'),
    ('video_image_outputs', 'gif_loop_tune_applied'),
    ('video_image_outputs', 'gif_loop_tune_effective_applied'),
    ('video_image_outputs', 'gif_loop_tune_fallback_to_base'),
    ('video_image_outputs', 'gif_loop_tune_score'),
    ('video_image_outputs', 'gif_loop_tune_loop_closure'),
    ('video_image_outputs', 'gif_loop_tune_motion_mean'),
    ('video_image_outputs', 'gif_loop_tune_effective_sec'),
    ('video_image_outputs', 'created_at')
  ) AS t(table_name, column_name)
), existing_columns AS (
  SELECT table_name, column_name
  FROM information_schema.columns
  WHERE table_schema = 'public'
    AND table_name IN ('video_image_jobs', 'video_image_outputs')
)
SELECT
  rc.table_name,
  rc.column_name,
  CASE WHEN ec.column_name IS NULL THEN 'missing' ELSE 'ok' END AS status
FROM required_columns rc
LEFT JOIN existing_columns ec
  ON ec.table_name = rc.table_name
 AND ec.column_name = rc.column_name
ORDER BY rc.table_name, rc.column_name;

-- 1) 最近 24h GIF 任务概况
SELECT
  COUNT(*) AS jobs_total,
  COUNT(*) FILTER (WHERE status = 'done') AS jobs_done,
  COUNT(*) FILTER (WHERE status = 'failed') AS jobs_failed,
  COUNT(*) FILTER (WHERE status = 'running') AS jobs_running,
  COUNT(*) FILTER (WHERE status = 'queued') AS jobs_queued,
  ROUND(
    COUNT(*) FILTER (WHERE status = 'done')::numeric / NULLIF(COUNT(*), 0),
    4
  ) AS done_rate,
  ROUND(
    COUNT(*) FILTER (WHERE status = 'failed')::numeric / NULLIF(COUNT(*), 0),
    4
  ) AS failed_rate
FROM public.video_image_jobs
WHERE requested_format = 'gif'
  AND created_at >= NOW() - INTERVAL '24 hours';

-- 2) 最近 24h GIF 产物概况（主产物）
SELECT
  COUNT(*) AS outputs_total,
  ROUND(AVG(size_bytes)) AS avg_size_bytes,
  ROUND(PERCENTILE_DISC(0.5) WITHIN GROUP (ORDER BY size_bytes)) AS p50_size_bytes,
  ROUND(PERCENTILE_DISC(0.95) WITHIN GROUP (ORDER BY size_bytes)) AS p95_size_bytes,
  ROUND(AVG(width)) AS avg_width,
  ROUND(AVG(height)) AS avg_height
FROM public.video_image_outputs
WHERE format = 'gif'
  AND file_role = 'main'
  AND created_at >= NOW() - INTERVAL '24 hours';

-- 3) GIF loop tuning 质量指标（最近 24h）
WITH loop_stats AS (
  SELECT
    COUNT(*) AS samples,
    COUNT(*) FILTER (WHERE gif_loop_tune_applied) AS applied,
    COUNT(*) FILTER (WHERE gif_loop_tune_effective_applied) AS effective_applied,
    COUNT(*) FILTER (WHERE gif_loop_tune_fallback_to_base) AS fallback_to_base,
    ROUND((AVG(gif_loop_tune_score) FILTER (WHERE gif_loop_tune_applied))::numeric, 4) AS avg_score,
    ROUND((AVG(gif_loop_tune_loop_closure) FILTER (WHERE gif_loop_tune_applied))::numeric, 4) AS avg_loop_closure,
    ROUND((AVG(gif_loop_tune_motion_mean) FILTER (WHERE gif_loop_tune_applied))::numeric, 4) AS avg_motion_mean,
    ROUND((AVG(gif_loop_tune_effective_sec) FILTER (WHERE gif_loop_tune_applied))::numeric, 4) AS avg_effective_sec
  FROM public.video_image_outputs
  WHERE format = 'gif'
    AND file_role = 'main'
    AND created_at >= NOW() - INTERVAL '24 hours'
)
SELECT
  samples,
  applied,
  effective_applied,
  fallback_to_base,
  ROUND(applied::numeric / NULLIF(samples, 0), 4) AS applied_rate,
  ROUND(effective_applied::numeric / NULLIF(samples, 0), 4) AS effective_applied_rate,
  ROUND(fallback_to_base::numeric / NULLIF(samples, 0), 4) AS fallback_rate,
  avg_score,
  avg_loop_closure,
  avg_motion_mean,
  avg_effective_sec
FROM loop_stats;

-- 3b) GIF loop tuning 决策原因（最近 24h）
SELECT
  COALESCE(NULLIF(metadata -> 'gif_loop_tune' ->> 'decision_reason', ''), '[empty]') AS decision_reason,
  COUNT(*) AS samples,
  COUNT(*) FILTER (WHERE gif_loop_tune_applied) AS applied,
  COUNT(*) FILTER (WHERE gif_loop_tune_effective_applied) AS effective_applied,
  COUNT(*) FILTER (WHERE gif_loop_tune_fallback_to_base) AS fallback_to_base
FROM public.video_image_outputs
WHERE format = 'gif'
  AND file_role = 'main'
  AND created_at >= NOW() - INTERVAL '24 hours'
GROUP BY 1
ORDER BY samples DESC, decision_reason ASC;

-- 4) 七牛新路径命中率（最近 24h）
-- 新规范示例：emoji/video-image/{env}/u/{uid_mod100}/{uid}/j/{job_id}/outputs/gif/...
SELECT
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE object_key LIKE 'emoji/video-image/%') AS new_path_prefix_count,
  COUNT(*) FILTER (
    WHERE object_key ~ '^emoji/video-image/[^/]+/u/[0-9]{1,2}/[0-9]+/j/[0-9]+/outputs/gif/.+'
  ) AS new_path_strict_count,
  ROUND(
    COUNT(*) FILTER (WHERE object_key LIKE 'emoji/video-image/%')::numeric / NULLIF(COUNT(*), 0),
    4
  ) AS new_path_prefix_rate,
  ROUND(
    COUNT(*) FILTER (
      WHERE object_key ~ '^emoji/video-image/[^/]+/u/[0-9]{1,2}/[0-9]+/j/[0-9]+/outputs/gif/.+'
    )::numeric / NULLIF(COUNT(*), 0),
    4
  ) AS new_path_strict_rate
FROM public.video_image_outputs
WHERE format = 'gif'
  AND file_role = 'main'
  AND created_at >= NOW() - INTERVAL '24 hours';

-- 5) 最近失败任务 Top 原因（GIF）
SELECT
  COALESCE(NULLIF(error_code, ''), '[empty]') AS error_code,
  COALESCE(NULLIF(error_message, ''), '[empty]') AS error_message,
  COUNT(*) AS cnt
FROM public.video_image_jobs
WHERE requested_format = 'gif'
  AND status = 'failed'
  AND created_at >= NOW() - INTERVAL '24 hours'
GROUP BY 1, 2
ORDER BY cnt DESC
LIMIT 10;

-- 6) 主链路一致性（最近 24h，GIF）
WITH gif_jobs AS (
  SELECT id, status
  FROM public.video_image_jobs
  WHERE requested_format = 'gif'
    AND created_at >= NOW() - INTERVAL '24 hours'
), gif_main_outputs AS (
  SELECT DISTINCT job_id
  FROM public.video_image_outputs
  WHERE format = 'gif'
    AND file_role = 'main'
    AND created_at >= NOW() - INTERVAL '24 hours'
)
SELECT
  COUNT(*) FILTER (WHERE j.status = 'done' AND o.job_id IS NULL) AS done_without_main_output,
  COUNT(*) FILTER (WHERE j.status = 'failed' AND o.job_id IS NOT NULL) AS failed_but_has_main_output,
  COUNT(*) FILTER (WHERE j.status = 'running' AND o.job_id IS NOT NULL) AS running_but_has_main_output
FROM gif_jobs j
LEFT JOIN gif_main_outputs o ON o.job_id = j.id;

-- 7) 字段完整性异常（最近 24h，GIF 主产物）
SELECT
  COUNT(*) AS samples,
  COUNT(*) FILTER (WHERE COALESCE(object_key, '') = '') AS missing_object_key,
  COUNT(*) FILTER (WHERE size_bytes <= 0) AS non_positive_size,
  COUNT(*) FILTER (WHERE width <= 0 OR height <= 0) AS invalid_dimension,
  COUNT(*) FILTER (WHERE gif_loop_tune_applied AND gif_loop_tune_score = 0) AS tune_applied_but_zero_score
FROM public.video_image_outputs
WHERE format = 'gif'
  AND file_role = 'main'
  AND created_at >= NOW() - INTERVAL '24 hours';

-- 8) 候选淘汰细归因（最近 24h）
WITH base AS (
  SELECT LOWER(TRIM(COALESCE(c.reject_reason, ''))) AS reject_reason
  FROM archive.video_job_gif_candidates c
  JOIN public.video_image_jobs j ON j.id = c.job_id
  WHERE j.requested_format = 'gif'
    AND c.created_at >= NOW() - INTERVAL '24 hours'
)
SELECT
  COUNT(*) AS samples,
  COUNT(*) FILTER (WHERE reject_reason <> '') AS rejected,
  ROUND(COUNT(*) FILTER (WHERE reject_reason <> '')::numeric / NULLIF(COUNT(*), 0), 4) AS reject_rate,
  COUNT(*) FILTER (WHERE reject_reason = 'low_emotion') AS low_emotion,
  COUNT(*) FILTER (WHERE reject_reason = 'low_confidence') AS low_confidence,
  COUNT(*) FILTER (WHERE reject_reason = 'duplicate_candidate') AS duplicate_candidate,
  COUNT(*) FILTER (WHERE reject_reason = 'blur_low') AS blur_low,
  COUNT(*) FILTER (WHERE reject_reason = 'size_budget_exceeded') AS size_budget_exceeded,
  COUNT(*) FILTER (WHERE reject_reason = 'loop_poor') AS loop_poor,
  COUNT(*) FILTER (
    WHERE reject_reason <> ''
      AND reject_reason NOT IN (
        'low_emotion',
        'low_confidence',
        'duplicate_candidate',
        'blur_low',
        'size_budget_exceeded',
        'loop_poor'
      )
  ) AS unknown_reason
FROM base;
