-- 视频任务 AI 消耗巡检（最近 24h）

-- 1) AI 调用总体
SELECT
  COUNT(*) AS calls_total,
  COUNT(*) FILTER (WHERE request_status = 'ok') AS calls_ok,
  COUNT(*) FILTER (WHERE request_status <> 'ok') AS calls_error,
  ROUND(COALESCE(SUM(cost_usd), 0)::numeric, 6) AS cost_usd_total,
  ROUND(COALESCE(SUM(input_tokens), 0)::numeric, 0) AS input_tokens_total,
  ROUND(COALESCE(SUM(output_tokens), 0)::numeric, 0) AS output_tokens_total,
  ROUND(COALESCE(SUM(cached_input_tokens), 0)::numeric, 0) AS cached_input_tokens_total,
  ROUND(COALESCE(SUM(request_duration_ms), 0)::numeric, 0) AS duration_ms_total
FROM ops.video_job_ai_usage
WHERE created_at >= NOW() - INTERVAL '24 hours';

-- 2) 按 provider/model 看成本与失败率
SELECT
  provider,
  model,
  COUNT(*) AS calls,
  COUNT(*) FILTER (WHERE request_status <> 'ok') AS errors,
  ROUND(
    COUNT(*) FILTER (WHERE request_status <> 'ok')::numeric / NULLIF(COUNT(*),0),
    4
  ) AS error_rate,
  ROUND(COALESCE(SUM(cost_usd),0)::numeric, 6) AS cost_usd,
  ROUND(COALESCE(SUM(input_tokens),0)::numeric, 0) AS input_tokens,
  ROUND(COALESCE(SUM(output_tokens),0)::numeric, 0) AS output_tokens
FROM ops.video_job_ai_usage
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY 1,2
ORDER BY cost_usd DESC, calls DESC
LIMIT 20;

-- 3) 任务级 AI 成本 Top
SELECT
  job_id,
  user_id,
  COUNT(*) AS calls,
  ROUND(COALESCE(SUM(cost_usd),0)::numeric, 6) AS ai_cost_usd,
  ROUND(COALESCE(SUM(request_duration_ms),0)::numeric, 0) AS ai_duration_ms,
  ROUND(COALESCE(SUM(input_tokens),0)::numeric, 0) AS input_tokens,
  ROUND(COALESCE(SUM(output_tokens),0)::numeric, 0) AS output_tokens
FROM ops.video_job_ai_usage
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY 1,2
ORDER BY ai_cost_usd DESC
LIMIT 20;

-- 4) 任务成本快照中 AI 维度覆盖率（details JSON）
SELECT
  COUNT(*) AS jobs_total,
  COUNT(*) FILTER (
    WHERE details ? 'ai_usage_calls'
  ) AS ai_dimension_jobs,
  ROUND(
    COUNT(*) FILTER (WHERE details ? 'ai_usage_calls')::numeric / NULLIF(COUNT(*),0),
    4
  ) AS ai_dimension_rate
FROM ops.video_job_costs
WHERE updated_at >= NOW() - INTERVAL '24 hours';
