-- GIFSICLE 优化巡检（最近 24h）
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/check_gif_optimizer_gain.sql

-- 1) 主产物样本数 + 优化动作统计
WITH base AS (
  SELECT
    id,
    metadata -> 'gif_optimization_v1' AS opt
  FROM public.video_image_outputs
  WHERE format = 'gif'
    AND file_role = 'main'
    AND created_at >= NOW() - INTERVAL '24 hours'
)
SELECT
  COUNT(*) AS samples,
  COUNT(*) FILTER (WHERE opt IS NOT NULL) AS opt_recorded,
  COUNT(*) FILTER (WHERE COALESCE((opt ->> 'attempted')::boolean, false)) AS attempted,
  COUNT(*) FILTER (WHERE COALESCE((opt ->> 'applied')::boolean, false)) AS applied,
  ROUND(
    COUNT(*) FILTER (WHERE COALESCE((opt ->> 'applied')::boolean, false))::numeric
      / NULLIF(COUNT(*) FILTER (WHERE COALESCE((opt ->> 'attempted')::boolean, false)), 0),
    4
  ) AS applied_rate_when_attempted
FROM base;

-- 2) 体积收益（仅 applied 样本）
WITH applied AS (
  SELECT
    COALESCE((metadata -> 'gif_optimization_v1' ->> 'before_size_bytes')::bigint, 0) AS before_size_bytes,
    COALESCE((metadata -> 'gif_optimization_v1' ->> 'after_size_bytes')::bigint, 0) AS after_size_bytes,
    COALESCE((metadata -> 'gif_optimization_v1' ->> 'saved_bytes')::bigint, 0) AS saved_bytes,
    COALESCE((metadata -> 'gif_optimization_v1' ->> 'saved_ratio')::numeric, 0) AS saved_ratio
  FROM public.video_image_outputs
  WHERE format = 'gif'
    AND file_role = 'main'
    AND created_at >= NOW() - INTERVAL '24 hours'
    AND COALESCE((metadata -> 'gif_optimization_v1' ->> 'applied')::boolean, false)
)
SELECT
  COUNT(*) AS applied_samples,
  ROUND(AVG(before_size_bytes)) AS avg_before_size_bytes,
  ROUND(AVG(after_size_bytes)) AS avg_after_size_bytes,
  ROUND(AVG(saved_bytes)) AS avg_saved_bytes,
  ROUND(AVG(saved_ratio), 4) AS avg_saved_ratio,
  ROUND(PERCENTILE_DISC(0.5) WITHIN GROUP (ORDER BY saved_ratio), 4) AS p50_saved_ratio,
  ROUND(PERCENTILE_DISC(0.9) WITHIN GROUP (ORDER BY saved_ratio), 4) AS p90_saved_ratio
FROM applied;

-- 3) 未应用原因分布（最近 24h）
SELECT
  COALESCE(NULLIF(metadata -> 'gif_optimization_v1' ->> 'reason', ''), '[empty]') AS reason,
  COUNT(*) AS samples
FROM public.video_image_outputs
WHERE format = 'gif'
  AND file_role = 'main'
  AND created_at >= NOW() - INTERVAL '24 hours'
  AND metadata ? 'gif_optimization_v1'
GROUP BY 1
ORDER BY samples DESC, reason ASC;

