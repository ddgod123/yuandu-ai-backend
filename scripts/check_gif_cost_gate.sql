-- GIF 成本门禁巡检（默认窗口 24h）
-- 说明：用于发布前确认 GIF done 任务平均成本未超过阈值。

WITH scoped AS (
  SELECT
    c.job_id,
    c.estimated_cost,
    c.currency,
    c.created_at
  FROM ops.video_job_costs c
  JOIN public.video_image_jobs j ON j.id = c.job_id
  WHERE LOWER(COALESCE(NULLIF(TRIM(j.requested_format), ''), 'gif')) = 'gif'
    AND LOWER(COALESCE(NULLIF(TRIM(j.status), ''), 'unknown')) = 'done'
    AND j.created_at >= NOW() - INTERVAL '24 hours'
),
summary AS (
  SELECT
    COUNT(*)::bigint AS samples,
    ROUND(COALESCE(AVG(estimated_cost), 0)::numeric, 6) AS avg_cost,
    ROUND(COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY estimated_cost), 0)::numeric, 6) AS p50_cost,
    ROUND(COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY estimated_cost), 0)::numeric, 6) AS p95_cost,
    ROUND(COALESCE(MAX(estimated_cost), 0)::numeric, 6) AS max_cost
  FROM scoped
)
SELECT
  samples,
  avg_cost,
  p50_cost,
  p95_cost,
  max_cost
FROM summary;
