BEGIN;

-- 060 初版使用了 partial unique index，gorm OnConflict(job_id,output_id)
-- 无法稳定命中推断，导致 upsert 报 42P10。这里统一改为常规唯一索引。
DROP INDEX IF EXISTS archive.uk_video_job_gif_evaluations_job_output;

CREATE UNIQUE INDEX IF NOT EXISTS uk_video_job_gif_evaluations_job_output
    ON archive.video_job_gif_evaluations (job_id, output_id);

COMMIT;
