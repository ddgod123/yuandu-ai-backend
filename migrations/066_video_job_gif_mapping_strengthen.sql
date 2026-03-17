BEGIN;

ALTER TABLE public.video_image_outputs
    ADD COLUMN IF NOT EXISTS proposal_id BIGINT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'fk_video_image_outputs_proposal_id'
    ) THEN
        ALTER TABLE public.video_image_outputs
            ADD CONSTRAINT fk_video_image_outputs_proposal_id
            FOREIGN KEY (proposal_id)
            REFERENCES archive.video_job_gif_ai_proposals(id)
            ON DELETE SET NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_video_image_outputs_proposal_id
    ON public.video_image_outputs (proposal_id);

-- 同一 job 的 proposal_rank 应唯一，先清理历史重复数据再加唯一索引。
WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (PARTITION BY job_id, proposal_rank ORDER BY id ASC) AS rn
    FROM archive.video_job_gif_ai_proposals
    WHERE proposal_rank > 0
)
DELETE FROM archive.video_job_gif_ai_proposals p
USING ranked r
WHERE p.id = r.id
  AND r.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_job_gif_ai_proposals_job_rank
    ON archive.video_job_gif_ai_proposals (job_id, proposal_rank)
    WHERE proposal_rank > 0;

-- 先用 review 回填 output.proposal_id（最可靠）。
UPDATE public.video_image_outputs o
SET proposal_id = r.proposal_id
FROM archive.video_job_gif_ai_reviews r
WHERE o.id = r.output_id
  AND o.proposal_id IS NULL
  AND r.proposal_id IS NOT NULL;

-- 再按 output metadata.proposal_rank 回填剩余空值。
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

COMMENT ON COLUMN public.video_image_outputs.proposal_id IS '强映射到 AI Planner proposal（job->proposal->output 链路）';

COMMIT;
