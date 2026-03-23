-- GIF 渲染阶段性能报告（窗口级）
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/perf/gif/report_render_stage.sql
--   psql "$DATABASE_URL" -v window_hours=72 -f backend/scripts/perf/gif/report_render_stage.sql

\if :{?window_hours}
\else
\set window_hours 24
\endif

WITH scoped AS (
  SELECT
    a.id,
    a.job_id,
    a.created_at,
    a.metadata,
    NULLIF(a.metadata->>'render_elapsed_ms', '')::numeric AS render_elapsed_ms,
    NULLIF(a.metadata->>'upload_elapsed_ms', '')::numeric AS upload_elapsed_ms,
    NULLIF(a.metadata->>'predicted_render_sec', '')::numeric AS predicted_render_sec,
    NULLIF(a.metadata->>'render_actual_sec', '')::numeric AS actual_render_sec,
    NULLIF(a.metadata->>'render_cost_units', '')::numeric AS render_cost_units,
    NULLIF(a.metadata->>'predicted_size_kb', '')::numeric AS predicted_size_kb,
    NULLIF(a.metadata->>'gifsicle_stage_ms', '')::numeric AS gifsicle_stage_ms,
    NULLIF(a.metadata->>'loop_tune_stage_ms', '')::numeric AS loop_tune_stage_ms
  FROM archive.video_job_artifacts a
  WHERE a.type = 'clip'
    AND a.metadata->>'format' = 'gif'
    AND a.created_at >= NOW() - (:'window_hours'::int || ' hours')::interval
)
SELECT
  :'window_hours'::int AS window_hours,
  COUNT(*) AS sample_count,
  ROUND(AVG(render_elapsed_ms), 2) AS avg_render_ms,
  ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY render_elapsed_ms), 2) AS p50_render_ms,
  ROUND(PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY render_elapsed_ms), 2) AS p90_render_ms,
  ROUND(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY render_elapsed_ms), 2) AS p99_render_ms,
  ROUND(AVG(upload_elapsed_ms), 2) AS avg_upload_ms,
  ROUND(AVG(predicted_render_sec), 3) AS avg_predicted_render_sec,
  ROUND(AVG(actual_render_sec), 3) AS avg_actual_render_sec,
  ROUND(AVG(render_cost_units), 3) AS avg_render_cost_units,
  ROUND(AVG(predicted_size_kb), 2) AS avg_predicted_size_kb,
  ROUND(AVG(gifsicle_stage_ms), 2) AS avg_gifsicle_stage_ms,
  ROUND(AVG(loop_tune_stage_ms), 2) AS avg_loop_tune_stage_ms
FROM scoped;

SELECT
  DATE_TRUNC('hour', created_at) AS hour_bucket,
  COUNT(*) AS sample_count,
  ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY render_elapsed_ms), 2) AS p50_render_ms,
  ROUND(PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY render_elapsed_ms), 2) AS p90_render_ms,
  ROUND(AVG(render_cost_units), 3) AS avg_render_cost_units
FROM scoped
GROUP BY 1
ORDER BY 1 DESC
LIMIT 48;
