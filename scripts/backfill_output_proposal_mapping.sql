-- 回填 output.proposal_id（可重复执行）
-- 用法：
--   psql "$DATABASE_URL" -f backend/scripts/backfill_output_proposal_mapping.sql

\echo '=== 1) from ai_reviews ==='
UPDATE public.video_image_outputs o
SET proposal_id = r.proposal_id
FROM archive.video_job_gif_ai_reviews r
WHERE o.id = r.output_id
  AND o.proposal_id IS NULL
  AND r.proposal_id IS NOT NULL;

\echo '=== 2) from output metadata.proposal_rank ==='
WITH output_rank AS (
    SELECT
        o.id AS output_id,
        o.job_id,
        CASE
            WHEN (o.metadata ->> 'proposal_rank') ~ '^[0-9]+$'
                THEN (o.metadata ->> 'proposal_rank')::INT
            ELSE NULL
        END AS proposal_rank
    FROM public.video_image_outputs o
    WHERE o.proposal_id IS NULL
      AND LOWER(COALESCE(o.format, '')) = 'gif'
      AND LOWER(COALESCE(o.file_role, '')) = 'main'
),
resolved AS (
    SELECT DISTINCT ON (x.output_id)
        x.output_id,
        p.id AS proposal_id
    FROM output_rank x
    JOIN archive.video_job_gif_ai_proposals p
      ON p.job_id = x.job_id
     AND p.proposal_rank = x.proposal_rank
    WHERE x.proposal_rank IS NOT NULL
    ORDER BY x.output_id, p.id ASC
)
UPDATE public.video_image_outputs o
SET proposal_id = r.proposal_id
FROM resolved r
WHERE o.id = r.output_id
  AND o.proposal_id IS NULL;

\echo '=== 3) coverage summary ==='
SELECT
  COUNT(*) AS gif_outputs_total,
  COUNT(*) FILTER (WHERE proposal_id IS NOT NULL) AS with_proposal_id,
  COUNT(*) FILTER (WHERE proposal_id IS NULL) AS missing_proposal_id,
  ROUND(
    COUNT(*) FILTER (WHERE proposal_id IS NOT NULL)::numeric / NULLIF(COUNT(*), 0),
    4
  ) AS proposal_coverage_rate
FROM public.video_image_outputs
WHERE LOWER(COALESCE(format, '')) = 'gif'
  AND LOWER(COALESCE(file_role, '')) = 'main';

\echo '=== 4) unresolved sample ==='
SELECT
  id AS output_id,
  job_id,
  object_key,
  metadata ->> 'proposal_rank' AS metadata_proposal_rank,
  created_at
FROM public.video_image_outputs
WHERE LOWER(COALESCE(format, '')) = 'gif'
  AND LOWER(COALESCE(file_role, '')) = 'main'
  AND proposal_id IS NULL
ORDER BY id DESC
LIMIT 50;
