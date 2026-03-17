-- GIF AI Planner + Judge 巡检（最近24小时）

-- 1) Planner 提名样本
SELECT
  COUNT(*) AS proposal_rows,
  COUNT(DISTINCT job_id) AS proposal_jobs,
  COUNT(*) FILTER (WHERE status = 'selected') AS selected_rows
FROM archive.video_job_gif_ai_proposals
WHERE created_at >= NOW() - INTERVAL '24 hours';

-- 2) Judge 复审样本
SELECT
  COUNT(*) AS review_rows,
  COUNT(DISTINCT job_id) AS review_jobs,
  COUNT(*) FILTER (WHERE final_recommendation = 'deliver') AS deliver_rows,
  COUNT(*) FILTER (WHERE final_recommendation = 'keep_internal') AS keep_internal_rows,
  COUNT(*) FILTER (WHERE final_recommendation = 'reject') AS reject_rows,
  COUNT(*) FILTER (WHERE final_recommendation = 'need_manual_review') AS manual_rows
FROM archive.video_job_gif_ai_reviews
WHERE created_at >= NOW() - INTERVAL '24 hours';

-- 3) AI 调用消耗（planner/judge）
SELECT
  stage,
  provider,
  model,
  COUNT(*) AS calls,
  ROUND(COALESCE(SUM(cost_usd),0)::numeric, 6) AS cost_usd,
  ROUND(COALESCE(SUM(input_tokens),0)::numeric, 0) AS input_tokens,
  ROUND(COALESCE(SUM(output_tokens),0)::numeric, 0) AS output_tokens,
  COUNT(*) FILTER (WHERE request_status <> 'ok') AS error_calls
FROM ops.video_job_ai_usage
WHERE stage IN ('planner','judge')
  AND created_at >= NOW() - INTERVAL '24 hours'
GROUP BY 1,2,3
ORDER BY cost_usd DESC, calls DESC;

-- 4) 强映射覆盖率（job->proposal->output->review）
SELECT
  COUNT(*) AS gif_outputs,
  COUNT(*) FILTER (WHERE o.proposal_id IS NOT NULL) AS outputs_with_proposal_id,
  ROUND(
    COUNT(*) FILTER (WHERE o.proposal_id IS NOT NULL)::numeric / NULLIF(COUNT(*), 0),
    4
  ) AS output_proposal_coverage,
  COUNT(*) FILTER (WHERE r.id IS NOT NULL) AS outputs_with_review,
  ROUND(
    COUNT(*) FILTER (WHERE r.id IS NOT NULL)::numeric / NULLIF(COUNT(*), 0),
    4
  ) AS output_review_coverage,
  COUNT(*) FILTER (WHERE r.id IS NOT NULL AND r.proposal_id IS NOT NULL) AS reviews_with_proposal_id
FROM public.video_image_outputs o
LEFT JOIN archive.video_job_gif_ai_reviews r
  ON r.job_id = o.job_id
 AND r.output_id = o.id
WHERE o.format = 'gif'
  AND o.file_role = 'main'
  AND o.created_at >= NOW() - INTERVAL '24 hours';
