-- AI3 judge_input_payload_v1 键名兼容巡检（兼容 snake_case / CamelCase）
-- 用法示例：
--   psql "$DATABASE_URL" -f backend/scripts/check_ai3_judge_input_schema_compat.sql
--   psql "$DATABASE_URL" -v job_id=166 -f backend/scripts/check_ai3_judge_input_schema_compat.sql
--   psql "$DATABASE_URL" -v hours=72 -f backend/scripts/check_ai3_judge_input_schema_compat.sql

\if :{?hours}
\else
\set hours 168
\endif

\if :{?job_id}
\else
SELECT COALESCE((
  SELECT u.job_id
  FROM ops.video_job_ai_usage u
  WHERE LOWER(COALESCE(u.stage, '')) = 'judge'
    AND u.metadata ? 'judge_input_payload_v1'
  ORDER BY u.id DESC
  LIMIT 1
), 0) AS default_job_id
\gset
\set job_id :default_job_id
\endif

\echo '=== params ==='
SELECT
  :'job_id'::bigint AS job_id,
  :'hours'::int AS hours;

\echo '=== 1) latest judge usage for job ==='
WITH latest AS (
  SELECT
    u.id,
    u.job_id,
    u.user_id,
    u.created_at,
    u.provider,
    u.model,
    u.request_status,
    u.request_duration_ms,
    u.input_tokens,
    u.output_tokens,
    u.cost_usd,
    u.metadata
  FROM ops.video_job_ai_usage u
  WHERE LOWER(COALESCE(u.stage, '')) = 'judge'
    AND u.job_id = :'job_id'::bigint
  ORDER BY u.id DESC
  LIMIT 1
)
SELECT
  id,
  job_id,
  user_id,
  created_at,
  provider,
  model,
  request_status,
  request_duration_ms,
  input_tokens,
  output_tokens,
  cost_usd,
  COALESCE(metadata->>'judge_input_schema_version', 'legacy_or_unknown') AS judge_input_schema_version,
  CASE
    WHEN jsonb_typeof(COALESCE(metadata->'judge_input_payload_v1'->'outputs', '[]'::jsonb)) = 'array'
      THEN jsonb_array_length(COALESCE(metadata->'judge_input_payload_v1'->'outputs', '[]'::jsonb))
    ELSE 0
  END AS payload_outputs_count
FROM latest;

\echo '=== 2) key style mix (recent window) ==='
WITH src AS (
  SELECT
    u.id,
    u.job_id,
    COALESCE(u.metadata->>'judge_input_schema_version', 'legacy_or_unknown') AS schema_version,
    e.elem
  FROM ops.video_job_ai_usage u
  CROSS JOIN LATERAL jsonb_array_elements(
    CASE
      WHEN jsonb_typeof(COALESCE(u.metadata->'judge_input_payload_v1'->'outputs', '[]'::jsonb)) = 'array'
        THEN COALESCE(u.metadata->'judge_input_payload_v1'->'outputs', '[]'::jsonb)
      ELSE '[]'::jsonb
    END
  ) AS e(elem)
  WHERE LOWER(COALESCE(u.stage, '')) = 'judge'
    AND u.created_at >= NOW() - (:'hours'::text || ' hours')::interval
)
SELECT
  schema_version,
  CASE
    WHEN elem ? 'output_id' THEN 'snake_case'
    WHEN elem ? 'OutputID' THEN 'camel_case'
    ELSE 'unknown'
  END AS output_key_style,
  COUNT(*) AS rows
FROM src
GROUP BY 1,2
ORDER BY rows DESC, schema_version, output_key_style;

\echo '=== 3) normalized outputs for latest judge usage (this job) ==='
WITH latest AS (
  SELECT u.id, u.job_id, u.metadata
  FROM ops.video_job_ai_usage u
  WHERE LOWER(COALESCE(u.stage, '')) = 'judge'
    AND u.job_id = :'job_id'::bigint
  ORDER BY u.id DESC
  LIMIT 1
), expanded AS (
  SELECT
    l.id AS usage_id,
    l.job_id,
    t.ordinality AS idx,
    t.elem
  FROM latest l
  CROSS JOIN LATERAL jsonb_array_elements(
    CASE
      WHEN jsonb_typeof(COALESCE(l.metadata->'judge_input_payload_v1'->'outputs', '[]'::jsonb)) = 'array'
        THEN COALESCE(l.metadata->'judge_input_payload_v1'->'outputs', '[]'::jsonb)
      ELSE '[]'::jsonb
    END
  ) WITH ORDINALITY AS t(elem, ordinality)
), normalized AS (
  SELECT
    usage_id,
    job_id,
    idx,
    elem,
    COALESCE(NULLIF(elem->>'output_id', ''), NULLIF(elem->>'OutputID', '')) AS output_id_text,
    COALESCE(NULLIF(elem->>'proposal_rank', ''), NULLIF(elem->>'ProposalRank', '')) AS proposal_rank_text,
    COALESCE(NULLIF(elem->>'proposal_id_by_win', ''), NULLIF(elem->>'ProposalIDByWin', '')) AS proposal_id_by_win_text,
    COALESCE(NULLIF(elem->>'eval_overall', ''), NULLIF(elem->>'EvalOverall', '')) AS eval_overall_text,
    COALESCE(NULLIF(elem->>'score', ''), NULLIF(elem->>'Score', '')) AS score_text,
    COALESCE(NULLIF(elem->>'duration_ms', ''), NULLIF(elem->>'DurationMs', '')) AS duration_ms_text,
    CASE
      WHEN elem ? 'output_id' THEN 'snake_case'
      WHEN elem ? 'OutputID' THEN 'camel_case'
      ELSE 'unknown'
    END AS key_style
  FROM expanded
)
SELECT
  n.idx,
  n.key_style,
  CASE WHEN n.output_id_text ~ '^[0-9]+$' THEN n.output_id_text::bigint END AS output_id,
  CASE WHEN n.proposal_rank_text ~ '^-?[0-9]+$' THEN n.proposal_rank_text::int END AS proposal_rank,
  CASE WHEN n.proposal_id_by_win_text ~ '^[0-9]+$' THEN n.proposal_id_by_win_text::bigint END AS proposal_id_by_win,
  CASE WHEN n.eval_overall_text ~ '^-?[0-9]+(\.[0-9]+)?$' THEN n.eval_overall_text::numeric END AS eval_overall,
  CASE WHEN n.score_text ~ '^-?[0-9]+(\.[0-9]+)?$' THEN n.score_text::numeric END AS score,
  CASE WHEN n.duration_ms_text ~ '^-?[0-9]+$' THEN n.duration_ms_text::int END AS duration_ms,
  o.id AS output_exists_id,
  o.job_id AS output_job_id,
  o.format,
  o.file_role
FROM normalized n
LEFT JOIN public.video_image_outputs o
  ON o.id = CASE WHEN n.output_id_text ~ '^[0-9]+$' THEN n.output_id_text::bigint END
ORDER BY n.idx;

\echo '=== 4) mismatch / missing check (normalized) ==='
WITH latest AS (
  SELECT u.id, u.job_id, u.metadata
  FROM ops.video_job_ai_usage u
  WHERE LOWER(COALESCE(u.stage, '')) = 'judge'
    AND u.job_id = :'job_id'::bigint
  ORDER BY u.id DESC
  LIMIT 1
), expanded AS (
  SELECT
    l.job_id,
    t.elem
  FROM latest l
  CROSS JOIN LATERAL jsonb_array_elements(
    CASE
      WHEN jsonb_typeof(COALESCE(l.metadata->'judge_input_payload_v1'->'outputs', '[]'::jsonb)) = 'array'
        THEN COALESCE(l.metadata->'judge_input_payload_v1'->'outputs', '[]'::jsonb)
      ELSE '[]'::jsonb
    END
  ) AS t(elem)
), normalized AS (
  SELECT
    job_id,
    CASE
      WHEN COALESCE(NULLIF(elem->>'output_id', ''), NULLIF(elem->>'OutputID', '')) ~ '^[0-9]+$'
        THEN COALESCE(NULLIF(elem->>'output_id', ''), NULLIF(elem->>'OutputID', ''))::bigint
      ELSE NULL
    END AS output_id
  FROM expanded
)
SELECT
  COUNT(*) AS rows_total,
  COUNT(*) FILTER (WHERE output_id IS NULL) AS output_id_missing,
  COUNT(*) FILTER (WHERE output_id IS NOT NULL AND o.id IS NULL) AS output_not_found,
  COUNT(*) FILTER (WHERE output_id IS NOT NULL AND o.id IS NOT NULL AND o.job_id <> n.job_id) AS output_job_mismatch,
  COUNT(*) FILTER (WHERE output_id IS NOT NULL AND o.id IS NOT NULL AND o.job_id = n.job_id) AS output_ok
FROM normalized n
LEFT JOIN public.video_image_outputs o ON o.id = n.output_id;
