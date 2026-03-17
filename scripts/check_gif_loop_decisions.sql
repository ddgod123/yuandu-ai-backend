-- GIF Loop Tuning 决策巡检（最近 24h）
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/check_gif_loop_decisions.sql

\echo ''
\echo '==[1) 最近 24h GIF Loop 决策分布]================================='
WITH rows AS (
  SELECT
    COALESCE(NULLIF(metadata -> 'gif_loop_tune' ->> 'decision_reason', ''), '[empty]') AS decision_reason,
    gif_loop_tune_applied,
    gif_loop_tune_effective_applied,
    gif_loop_tune_fallback_to_base
  FROM public.video_image_outputs
  WHERE format = 'gif'
    AND file_role = 'main'
    AND created_at >= NOW() - INTERVAL '24 hours'
)
SELECT
  decision_reason,
  COUNT(*) AS samples,
  COUNT(*) FILTER (WHERE gif_loop_tune_applied) AS applied,
  COUNT(*) FILTER (WHERE gif_loop_tune_effective_applied) AS effective_applied,
  COUNT(*) FILTER (WHERE gif_loop_tune_fallback_to_base) AS fallback_to_base
FROM rows
GROUP BY decision_reason
ORDER BY samples DESC, decision_reason ASC;

\echo ''
\echo '==[2) 最近 24h 明细（最新 50 条）]================================='
SELECT
  id,
  job_id,
  created_at,
  object_key,
  gif_loop_tune_applied,
  gif_loop_tune_effective_applied,
  gif_loop_tune_fallback_to_base,
  gif_loop_tune_score,
  gif_loop_tune_loop_closure,
  gif_loop_tune_motion_mean,
  gif_loop_tune_effective_sec,
  COALESCE(NULLIF(metadata -> 'gif_loop_tune' ->> 'decision_reason', ''), '[empty]') AS decision_reason,
  NULLIF(metadata -> 'gif_loop_tune' ->> 'base_score', '')::double precision AS base_score,
  NULLIF(metadata -> 'gif_loop_tune' ->> 'best_score', '')::double precision AS best_score,
  NULLIF(metadata -> 'gif_loop_tune' ->> 'score_improvement', '')::double precision AS score_improvement,
  NULLIF(metadata -> 'gif_loop_tune' ->> 'min_improvement', '')::double precision AS min_improvement,
  NULLIF(metadata -> 'gif_loop_tune' ->> 'base_loop', '')::double precision AS base_loop,
  NULLIF(metadata -> 'gif_loop_tune' ->> 'best_loop', '')::double precision AS best_loop
FROM public.video_image_outputs
WHERE format = 'gif'
  AND file_role = 'main'
  AND created_at >= NOW() - INTERVAL '24 hours'
ORDER BY created_at DESC
LIMIT 50;
