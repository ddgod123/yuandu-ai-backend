-- 反馈完整性巡检脚本（最近 24h）
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/check_feedback_integrity.sql

-- 0) 关键表字段巡检
WITH required_columns AS (
  SELECT *
  FROM (VALUES
    ('video_image_feedback', 'id'),
    ('video_image_feedback', 'job_id'),
    ('video_image_feedback', 'output_id'),
    ('video_image_feedback', 'user_id'),
    ('video_image_feedback', 'action'),
    ('video_image_feedback', 'created_at'),
    ('video_image_outputs', 'id'),
    ('video_image_outputs', 'job_id'),
    ('video_image_outputs', 'object_key'),
    ('video_image_jobs', 'id'),
    ('video_image_jobs', 'requested_format')
  ) AS t(table_name, column_name)
), existing_columns AS (
  SELECT table_name, column_name
  FROM information_schema.columns
  WHERE table_schema = 'public'
    AND table_name IN ('video_image_feedback', 'video_image_outputs', 'video_image_jobs')
)
SELECT
  rc.table_name,
  rc.column_name,
  CASE WHEN ec.column_name IS NULL THEN 'missing' ELSE 'ok' END AS status
FROM required_columns rc
LEFT JOIN existing_columns ec
  ON ec.table_name = rc.table_name
 AND ec.column_name = rc.column_name
ORDER BY rc.table_name, rc.column_name;

-- 1) 最近 24h 反馈完整性总览
WITH base AS (
  SELECT
    f.id,
    f.job_id,
    f.user_id,
    f.output_id,
    LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action
  FROM public.video_image_feedback f
  WHERE f.created_at >= NOW() - INTERVAL '24 hours'
), joined AS (
  SELECT
    b.*,
    o.id AS output_exists_id,
    o.job_id AS output_job_id
  FROM base b
  LEFT JOIN public.video_image_outputs o ON o.id = b.output_id
), top_pick_conflict AS (
  SELECT COUNT(*)::bigint AS users
  FROM (
    SELECT job_id, user_id
    FROM base
    WHERE action = 'top_pick'
    GROUP BY job_id, user_id
    HAVING COUNT(*) > 1
  ) t
)
SELECT
  COUNT(*)::bigint AS samples,
  COUNT(*) FILTER (WHERE output_id IS NOT NULL)::bigint AS with_output_id,
  COUNT(*) FILTER (WHERE output_id IS NULL)::bigint AS missing_output_id,
  COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::bigint AS resolved_output,
  COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NULL)::bigint AS orphan_output,
  COUNT(*) FILTER (
    WHERE output_id IS NOT NULL
      AND output_exists_id IS NOT NULL
      AND output_job_id <> job_id
  )::bigint AS job_mismatch,
  COALESCE((SELECT users FROM top_pick_conflict), 0)::bigint AS top_pick_multi_hit_users,
  ROUND(
    COUNT(*) FILTER (WHERE output_id IS NOT NULL)::numeric / NULLIF(COUNT(*), 0),
    4
  ) AS output_coverage_rate,
  ROUND(
    COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::numeric
      / NULLIF(COUNT(*) FILTER (WHERE output_id IS NOT NULL), 0),
    4
  ) AS output_resolved_rate,
  ROUND(
    (COUNT(*) FILTER (
      WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL AND output_job_id = job_id
    ))::numeric
      / NULLIF(COUNT(*) FILTER (WHERE output_id IS NOT NULL), 0),
    4
  ) AS output_job_consistency_rate
FROM joined;

-- 2) 按 action 的完整性分布
WITH base AS (
  SELECT
    f.id,
    f.job_id,
    f.output_id,
    LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action
  FROM public.video_image_feedback f
  WHERE f.created_at >= NOW() - INTERVAL '24 hours'
), joined AS (
  SELECT
    b.*,
    o.id AS output_exists_id,
    o.job_id AS output_job_id
  FROM base b
  LEFT JOIN public.video_image_outputs o ON o.id = b.output_id
)
SELECT
  action,
  COUNT(*)::bigint AS samples,
  COUNT(*) FILTER (WHERE output_id IS NOT NULL)::bigint AS with_output_id,
  COUNT(*) FILTER (WHERE output_id IS NULL)::bigint AS missing_output_id,
  COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::bigint AS resolved_output,
  COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NULL)::bigint AS orphan_output,
  COUNT(*) FILTER (
    WHERE output_id IS NOT NULL
      AND output_exists_id IS NOT NULL
      AND output_job_id <> job_id
  )::bigint AS job_mismatch,
  ROUND(
    COUNT(*) FILTER (WHERE output_id IS NOT NULL)::numeric / NULLIF(COUNT(*), 0),
    4
  ) AS output_coverage_rate,
  ROUND(
    COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::numeric
      / NULLIF(COUNT(*) FILTER (WHERE output_id IS NOT NULL), 0),
    4
  ) AS output_resolved_rate,
  ROUND(
    (COUNT(*) FILTER (
      WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL AND output_job_id = job_id
    ))::numeric
      / NULLIF(COUNT(*) FILTER (WHERE output_id IS NOT NULL), 0),
    4
  ) AS output_job_consistency_rate
FROM joined
GROUP BY action
ORDER BY samples DESC, action ASC;

-- 3) top_pick 冲突明细（理论应为 0）
SELECT
  f.job_id,
  f.user_id,
  COUNT(*) AS top_pick_count
FROM public.video_image_feedback f
WHERE f.created_at >= NOW() - INTERVAL '24 hours'
  AND LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) = 'top_pick'
GROUP BY f.job_id, f.user_id
HAVING COUNT(*) > 1
ORDER BY top_pick_count DESC, f.job_id DESC
LIMIT 50;

-- 4) orphan / mismatch 样本（用于快速排障）
WITH base AS (
  SELECT
    f.id,
    f.job_id,
    f.user_id,
    f.output_id,
    LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action,
    f.created_at
  FROM public.video_image_feedback f
  WHERE f.created_at >= NOW() - INTERVAL '24 hours'
), joined AS (
  SELECT
    b.*,
    o.id AS output_exists_id,
    o.job_id AS output_job_id,
    o.object_key
  FROM base b
  LEFT JOIN public.video_image_outputs o ON o.id = b.output_id
)
SELECT
  id AS feedback_id,
  job_id,
  user_id,
  output_id,
  action,
  CASE
    WHEN output_id IS NOT NULL AND output_exists_id IS NULL THEN 'orphan_output'
    WHEN output_id IS NOT NULL AND output_exists_id IS NOT NULL AND output_job_id <> job_id THEN 'job_mismatch'
    ELSE 'ok'
  END AS anomaly,
  output_job_id,
  object_key,
  created_at
FROM joined
WHERE (output_id IS NOT NULL AND output_exists_id IS NULL)
   OR (output_id IS NOT NULL AND output_exists_id IS NOT NULL AND output_job_id <> job_id)
ORDER BY created_at DESC
LIMIT 100;

-- 5) 按后台阈值判级（ops.video_quality_settings.id=1）
-- 兼容点：阈值字段通过 to_jsonb 读取；若尚未执行 059 迁移，则自动回退到默认阈值。
WITH setting_row AS (
  SELECT COALESCE(
    (SELECT to_jsonb(s) FROM ops.video_quality_settings s WHERE s.id = 1 LIMIT 1),
    '{}'::jsonb
  ) AS data
), setting AS (
  SELECT
    COALESCE(NULLIF(TRIM(data->>'feedback_integrity_output_coverage_rate_warn'), '')::numeric, 0.98) AS feedback_integrity_output_coverage_rate_warn,
    COALESCE(NULLIF(TRIM(data->>'feedback_integrity_output_coverage_rate_critical'), '')::numeric, 0.95) AS feedback_integrity_output_coverage_rate_critical,
    COALESCE(NULLIF(TRIM(data->>'feedback_integrity_output_resolved_rate_warn'), '')::numeric, 0.99) AS feedback_integrity_output_resolved_rate_warn,
    COALESCE(NULLIF(TRIM(data->>'feedback_integrity_output_resolved_rate_critical'), '')::numeric, 0.97) AS feedback_integrity_output_resolved_rate_critical,
    COALESCE(NULLIF(TRIM(data->>'feedback_integrity_output_job_consistency_rate_warn'), '')::numeric, 0.999) AS feedback_integrity_output_job_consistency_rate_warn,
    COALESCE(NULLIF(TRIM(data->>'feedback_integrity_output_job_consistency_rate_critical'), '')::numeric, 0.995) AS feedback_integrity_output_job_consistency_rate_critical,
    COALESCE(NULLIF(TRIM(data->>'feedback_integrity_top_pick_conflict_users_warn'), '')::numeric, 1) AS feedback_integrity_top_pick_conflict_users_warn,
    COALESCE(NULLIF(TRIM(data->>'feedback_integrity_top_pick_conflict_users_critical'), '')::numeric, 3) AS feedback_integrity_top_pick_conflict_users_critical
  FROM setting_row
), stats AS (
  WITH base AS (
    SELECT
      f.id,
      f.job_id,
      f.user_id,
      f.output_id,
      LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action
    FROM public.video_image_feedback f
    WHERE f.created_at >= NOW() - INTERVAL '24 hours'
  ), joined AS (
    SELECT
      b.*,
      o.id AS output_exists_id,
      o.job_id AS output_job_id
    FROM base b
    LEFT JOIN public.video_image_outputs o ON o.id = b.output_id
  ), top_pick_conflict AS (
    SELECT COUNT(*)::bigint AS users
    FROM (
      SELECT job_id, user_id
      FROM base
      WHERE action = 'top_pick'
      GROUP BY job_id, user_id
      HAVING COUNT(*) > 1
    ) t
  )
  SELECT
    COUNT(*)::bigint AS samples,
    COUNT(*) FILTER (WHERE output_id IS NOT NULL)::bigint AS with_output_id,
    COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::bigint AS resolved_output,
    COUNT(*) FILTER (
      WHERE output_id IS NOT NULL
        AND output_exists_id IS NOT NULL
        AND output_job_id = job_id
    )::bigint AS consistent_output,
    COALESCE((SELECT users FROM top_pick_conflict), 0)::bigint AS top_pick_multi_hit_users,
    COALESCE(
      COUNT(*) FILTER (WHERE output_id IS NOT NULL)::numeric / NULLIF(COUNT(*), 0),
      0
    ) AS output_coverage_rate,
    COALESCE(
      COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::numeric
        / NULLIF(COUNT(*) FILTER (WHERE output_id IS NOT NULL), 0),
      0
    ) AS output_resolved_rate,
    COALESCE(
      COUNT(*) FILTER (
        WHERE output_id IS NOT NULL
          AND output_exists_id IS NOT NULL
          AND output_job_id = job_id
      )::numeric
        / NULLIF(COUNT(*) FILTER (WHERE output_id IS NOT NULL), 0),
      0
    ) AS output_job_consistency_rate
  FROM joined
)
SELECT
  metric,
  samples,
  value,
  warn_threshold,
  critical_threshold,
  CASE
    WHEN samples = 0 THEN 'no_data'
    WHEN mode = 'low_is_bad' AND value < critical_threshold THEN 'critical'
    WHEN mode = 'low_is_bad' AND value < warn_threshold THEN 'warn'
    WHEN mode = 'high_is_bad' AND value >= critical_threshold THEN 'critical'
    WHEN mode = 'high_is_bad' AND value >= warn_threshold THEN 'warn'
    ELSE 'ok'
  END AS level
FROM (
  SELECT
    'output_coverage_rate'::text AS metric,
    stats.samples::numeric AS samples,
    stats.output_coverage_rate::numeric AS value,
    setting.feedback_integrity_output_coverage_rate_warn::numeric AS warn_threshold,
    setting.feedback_integrity_output_coverage_rate_critical::numeric AS critical_threshold,
    'low_is_bad'::text AS mode
  FROM stats CROSS JOIN setting
  UNION ALL
  SELECT
    'output_resolved_rate',
    stats.samples::numeric,
    stats.output_resolved_rate::numeric,
    setting.feedback_integrity_output_resolved_rate_warn::numeric,
    setting.feedback_integrity_output_resolved_rate_critical::numeric,
    'low_is_bad'
  FROM stats CROSS JOIN setting
  UNION ALL
  SELECT
    'output_job_consistency_rate',
    stats.samples::numeric,
    stats.output_job_consistency_rate::numeric,
    setting.feedback_integrity_output_job_consistency_rate_warn::numeric,
    setting.feedback_integrity_output_job_consistency_rate_critical::numeric,
    'low_is_bad'
  FROM stats CROSS JOIN setting
  UNION ALL
  SELECT
    'top_pick_multi_hit_users',
    stats.samples::numeric,
    stats.top_pick_multi_hit_users::numeric,
    setting.feedback_integrity_top_pick_conflict_users_warn::numeric,
    setting.feedback_integrity_top_pick_conflict_users_critical::numeric,
    'high_is_bad'
  FROM stats CROSS JOIN setting
) rows
ORDER BY
  CASE metric
    WHEN 'output_coverage_rate' THEN 1
    WHEN 'output_resolved_rate' THEN 2
    WHEN 'output_job_consistency_rate' THEN 3
    WHEN 'top_pick_multi_hit_users' THEN 4
    ELSE 99
  END;
