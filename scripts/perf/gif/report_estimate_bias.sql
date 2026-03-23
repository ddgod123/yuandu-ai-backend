-- GIF 估算偏差报告（predicted vs actual）
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/perf/gif/report_estimate_bias.sql
--   psql "$DATABASE_URL" -v window_hours=72 -f backend/scripts/perf/gif/report_estimate_bias.sql

\if :{?window_hours}
\else
\set window_hours 24
\endif

WITH scoped AS (
  SELECT
    a.id,
    a.job_id,
    a.created_at,
    NULLIF(a.metadata->>'predicted_render_sec', '')::numeric AS predicted_render_sec,
    NULLIF(a.metadata->>'render_actual_sec', '')::numeric AS actual_render_sec,
    NULLIF(a.metadata->>'predicted_size_kb', '')::numeric AS predicted_size_kb,
    a.size_bytes
  FROM archive.video_job_artifacts a
  WHERE a.type = 'clip'
    AND a.metadata->>'format' = 'gif'
    AND a.created_at >= NOW() - (:'window_hours'::int || ' hours')::interval
), normalized AS (
  SELECT
    *,
    (size_bytes::numeric / 1024.0) AS actual_size_kb,
    CASE WHEN predicted_render_sec > 0 THEN actual_render_sec / predicted_render_sec ELSE NULL END AS render_bias_ratio,
    CASE WHEN predicted_size_kb > 0 THEN (size_bytes::numeric / 1024.0) / predicted_size_kb ELSE NULL END AS size_bias_ratio
  FROM scoped
)
SELECT
  :'window_hours'::int AS window_hours,
  COUNT(*) AS sample_count,
  ROUND(AVG(predicted_render_sec), 3) AS avg_predicted_render_sec,
  ROUND(AVG(actual_render_sec), 3) AS avg_actual_render_sec,
  ROUND(AVG(render_bias_ratio), 3) AS avg_render_bias_ratio,
  ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY render_bias_ratio), 3) AS p50_render_bias_ratio,
  ROUND(PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY render_bias_ratio), 3) AS p90_render_bias_ratio,
  ROUND(AVG(predicted_size_kb), 2) AS avg_predicted_size_kb,
  ROUND(AVG(actual_size_kb), 2) AS avg_actual_size_kb,
  ROUND(AVG(size_bias_ratio), 3) AS avg_size_bias_ratio,
  ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY size_bias_ratio), 3) AS p50_size_bias_ratio,
  ROUND(PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY size_bias_ratio), 3) AS p90_size_bias_ratio
FROM normalized;

SELECT
  CASE
    WHEN render_bias_ratio IS NULL THEN 'unknown'
    WHEN render_bias_ratio < 0.7 THEN 'under'
    WHEN render_bias_ratio > 1.3 THEN 'over'
    ELSE 'ok'
  END AS render_bias_bucket,
  COUNT(*) AS samples
FROM normalized
GROUP BY 1
ORDER BY 2 DESC;
