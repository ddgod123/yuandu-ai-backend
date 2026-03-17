-- GIF 人工评分对齐巡检（最近 7d）
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/check_gif_manual_compare.sql

-- 1) 人工评分样本规模
SELECT
  COUNT(*) AS samples,
  COUNT(*) FILTER (WHERE output_id IS NOT NULL) AS with_output_id,
  COUNT(*) FILTER (WHERE is_top_pick) AS top_pick_samples,
  ROUND(AVG(CASE WHEN is_pass THEN 1.0 ELSE 0.0 END)::numeric, 4) AS pass_rate
FROM ops.video_job_gif_manual_scores
WHERE reviewed_at >= NOW() - INTERVAL '7 days';

-- 2) 与自动评测的匹配覆盖与误差
WITH joined AS (
  SELECT
    m.*,
    e.emotion_score AS auto_emotion_score,
    e.clarity_score AS auto_clarity_score,
    e.motion_score AS auto_motion_score,
    e.loop_score AS auto_loop_score,
    e.efficiency_score AS auto_efficiency_score,
    e.overall_score AS auto_overall_score
  FROM ops.video_job_gif_manual_scores m
  LEFT JOIN archive.video_job_gif_evaluations e
    ON e.output_id = m.output_id
  WHERE m.reviewed_at >= NOW() - INTERVAL '7 days'
)
SELECT
  COUNT(*) AS samples,
  COUNT(*) FILTER (WHERE auto_overall_score IS NOT NULL) AS matched_evaluations,
  ROUND(
    COUNT(*) FILTER (WHERE auto_overall_score IS NOT NULL)::numeric / NULLIF(COUNT(*), 0),
    4
  ) AS matched_rate,
  ROUND(AVG(ABS(overall_score - auto_overall_score)) FILTER (WHERE auto_overall_score IS NOT NULL)::numeric, 4) AS mae_overall,
  ROUND(AVG(ABS(emotion_score - auto_emotion_score)) FILTER (WHERE auto_overall_score IS NOT NULL)::numeric, 4) AS mae_emotion,
  ROUND(AVG(ABS(clarity_score - auto_clarity_score)) FILTER (WHERE auto_overall_score IS NOT NULL)::numeric, 4) AS mae_clarity,
  ROUND(AVG(ABS(motion_score - auto_motion_score)) FILTER (WHERE auto_overall_score IS NOT NULL)::numeric, 4) AS mae_motion,
  ROUND(AVG(ABS(loop_score - auto_loop_score)) FILTER (WHERE auto_overall_score IS NOT NULL)::numeric, 4) AS mae_loop,
  ROUND(AVG(ABS(efficiency_score - auto_efficiency_score)) FILTER (WHERE auto_overall_score IS NOT NULL)::numeric, 4) AS mae_efficiency
FROM joined;

-- 3) 偏差最大的样本 Top 20
SELECT
  m.sample_id,
  m.baseline_version,
  m.review_round,
  m.reviewer,
  m.job_id,
  m.output_id,
  ROUND(m.overall_score::numeric, 4) AS manual_overall,
  ROUND(e.overall_score::numeric, 4) AS auto_overall,
  ROUND((m.overall_score - e.overall_score)::numeric, 4) AS delta_overall,
  ROUND(ABS(m.overall_score - e.overall_score)::numeric, 4) AS abs_delta_overall,
  m.is_top_pick,
  m.is_pass,
  COALESCE(NULLIF(m.reject_reason, ''), '[empty]') AS reject_reason,
  m.reviewed_at
FROM ops.video_job_gif_manual_scores m
JOIN archive.video_job_gif_evaluations e ON e.output_id = m.output_id
WHERE m.reviewed_at >= NOW() - INTERVAL '7 days'
ORDER BY ABS(m.overall_score - e.overall_score) DESC, m.reviewed_at DESC
LIMIT 20;
