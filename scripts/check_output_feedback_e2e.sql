-- output_id 级反馈闭环巡检（支持默认参数）
-- 传参示例：
--   psql "$DATABASE_URL" \
--     -v job_id=26 \
--     -v user_id=5 \
--     -v output_id=17 \
--     -v action='top_pick' \
--     -f backend/scripts/check_output_feedback_e2e.sql
--
-- 不传参时默认行为：
--   job_id   = 最近一条 done 的 GIF 任务
--   user_id  = 该任务所属 user_id
--   output_id= 该任务第一条 GIF 主产物
--   action   = top_pick

\if :{?action}
\else
\set action top_pick
\endif

\if :{?job_id}
\else
SELECT COALESCE((
  SELECT j.id
  FROM public.video_image_jobs j
  WHERE LOWER(COALESCE(NULLIF(TRIM(j.requested_format), ''), 'gif')) = 'gif'
    AND LOWER(COALESCE(NULLIF(TRIM(j.status), ''), 'unknown')) = 'done'
  ORDER BY j.id DESC
  LIMIT 1
), 0) AS default_job_id
\gset
\set job_id :default_job_id
\endif

\if :{?user_id}
\else
SELECT COALESCE((
  SELECT j.user_id
  FROM public.video_image_jobs j
  WHERE j.id = :'job_id'::bigint
  LIMIT 1
), 0) AS default_user_id
\gset
\set user_id :default_user_id
\endif

\if :{?output_id}
\else
SELECT COALESCE((
  SELECT o.id
  FROM public.video_image_outputs o
  WHERE o.job_id = :'job_id'::bigint
    AND o.format = 'gif'
    AND o.file_role = 'main'
  ORDER BY o.id ASC
  LIMIT 1
), 0) AS default_output_id
\gset
\set output_id :default_output_id
\endif

\echo '=== params ==='
SELECT
  :'job_id'::bigint AS job_id,
  :'user_id'::bigint AS user_id,
  :'output_id'::bigint AS output_id,
  :'action' AS action;

\echo '=== 1) feedback row (latest) ==='
SELECT
  f.id,
  f.job_id,
  f.user_id,
  f.output_id,
  f.action,
  f.weight,
  f.scene_tag,
  f.created_at
FROM public.video_image_feedback f
WHERE f.job_id = :'job_id'::bigint
  AND f.user_id = :'user_id'::bigint
  AND f.output_id = :'output_id'::bigint
  AND LOWER(COALESCE(f.action, '')) = LOWER(:'action')
ORDER BY f.id DESC
LIMIT 5;

\echo '=== 2) output ownership check ==='
SELECT
  o.id AS output_id,
  o.job_id,
  o.format,
  o.file_role,
  o.object_key,
  o.created_at
FROM public.video_image_outputs o
WHERE o.id = :'output_id'::bigint
  AND o.job_id = :'job_id'::bigint;

\echo '=== 3) output -> proposal -> evaluation -> review trace ==='
SELECT
  o.id AS output_id,
  o.proposal_id AS output_proposal_id,
  p.proposal_rank,
  p.start_sec AS proposal_start_sec,
  p.end_sec AS proposal_end_sec,
  e.id AS evaluation_id,
  e.job_id AS eval_job_id,
  e.candidate_id,
  e.window_start_ms,
  e.window_end_ms,
  e.overall_score,
  c.id AS candidate_id_resolved,
  c.start_ms AS candidate_start_ms,
  c.end_ms AS candidate_end_ms,
  c.base_score AS candidate_base_score,
  c.confidence_score AS candidate_confidence_score,
  r.id AS review_id,
  r.proposal_id AS review_proposal_id,
  r.final_recommendation
FROM public.video_image_outputs o
LEFT JOIN archive.video_job_gif_ai_proposals p
  ON p.id = o.proposal_id
LEFT JOIN archive.video_job_gif_evaluations e
  ON e.output_id = o.id
LEFT JOIN archive.video_job_gif_candidates c
  ON c.id = e.candidate_id
LEFT JOIN archive.video_job_gif_ai_reviews r
  ON r.job_id = o.job_id
 AND r.output_id = o.id
WHERE o.id = :'output_id'::bigint;

\echo '=== 4) this job rerank logs summary ==='
SELECT
  COUNT(*) AS rerank_rows,
  ROUND(AVG(score_delta)::numeric, 4) AS avg_score_delta,
  COUNT(*) FILTER (WHERE score_delta > 0) AS positive_delta_rows,
  COUNT(*) FILTER (WHERE score_delta < 0) AS negative_delta_rows
FROM ops.video_job_gif_rerank_logs
WHERE job_id = :'job_id'::bigint;
