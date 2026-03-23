-- GIF 成本调度使用报告（基于 render_cost_units）
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/perf/gif/report_scheduler_token_usage.sql
--   psql "$DATABASE_URL" -v window_hours=72 -f backend/scripts/perf/gif/report_scheduler_token_usage.sql

\if :{?window_hours}
\else
\set window_hours 24
\endif

WITH scoped AS (
  SELECT
    a.job_id,
    a.created_at,
    NULLIF(a.metadata->>'render_cost_units', '')::numeric AS render_cost_units,
    NULLIF(a.metadata->>'predicted_render_sec', '')::numeric AS predicted_render_sec,
    NULLIF(a.metadata->>'render_actual_sec', '')::numeric AS actual_render_sec
  FROM archive.video_job_artifacts a
  WHERE a.type = 'clip'
    AND a.metadata->>'format' = 'gif'
    AND a.created_at >= NOW() - (:'window_hours'::int || ' hours')::interval
)
SELECT
  :'window_hours'::int AS window_hours,
  COUNT(*) AS sample_count,
  ROUND(AVG(render_cost_units), 3) AS avg_cost_units,
  ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY render_cost_units), 3) AS p50_cost_units,
  ROUND(PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY render_cost_units), 3) AS p90_cost_units,
  ROUND(SUM(render_cost_units), 2) AS total_cost_units,
  ROUND(SUM(predicted_render_sec), 2) AS total_predicted_render_sec,
  ROUND(SUM(actual_render_sec), 2) AS total_actual_render_sec
FROM scoped;

SELECT
  job_id,
  COUNT(*) AS gif_outputs,
  ROUND(SUM(render_cost_units), 2) AS job_cost_units,
  ROUND(SUM(actual_render_sec), 2) AS job_actual_render_sec,
  ROUND(PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY actual_render_sec), 2) AS job_p90_output_render_sec
FROM scoped
GROUP BY job_id
ORDER BY job_cost_units DESC, job_actual_render_sec DESC
LIMIT 30;
