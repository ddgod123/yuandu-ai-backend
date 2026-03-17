BEGIN;

ALTER TABLE public.video_image_outputs
    ADD COLUMN IF NOT EXISTS gif_loop_tune_applied BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS gif_loop_tune_effective_applied BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS gif_loop_tune_fallback_to_base BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS gif_loop_tune_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gif_loop_tune_loop_closure DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gif_loop_tune_motion_mean DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gif_loop_tune_effective_sec DOUBLE PRECISION NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_video_image_outputs_gif_loop_window
    ON public.video_image_outputs (created_at DESC)
    WHERE format = 'gif' AND file_role = 'main';

COMMENT ON COLUMN public.video_image_outputs.gif_loop_tune_applied IS 'GIF 循环调优是否被触发';
COMMENT ON COLUMN public.video_image_outputs.gif_loop_tune_effective_applied IS 'GIF 循环调优是否最终生效（未回退）';
COMMENT ON COLUMN public.video_image_outputs.gif_loop_tune_fallback_to_base IS 'GIF 调优后是否回退到基线窗口';
COMMENT ON COLUMN public.video_image_outputs.gif_loop_tune_score IS 'GIF 循环调优评分';
COMMENT ON COLUMN public.video_image_outputs.gif_loop_tune_loop_closure IS 'GIF 首尾闭合度评分（0-1）';
COMMENT ON COLUMN public.video_image_outputs.gif_loop_tune_motion_mean IS 'GIF 片段平均运动强度';
COMMENT ON COLUMN public.video_image_outputs.gif_loop_tune_effective_sec IS 'GIF 最终生效时长（秒）';

UPDATE public.video_image_outputs
SET gif_loop_tune_applied = CASE
        WHEN COALESCE((metadata->'gif_loop_tune'->>'applied')::boolean, FALSE) THEN TRUE
        ELSE FALSE
    END,
    gif_loop_tune_effective_applied = CASE
        WHEN COALESCE((metadata->'gif_loop_tune'->>'effective_applied')::boolean, FALSE) THEN TRUE
        ELSE FALSE
    END,
    gif_loop_tune_fallback_to_base = CASE
        WHEN COALESCE((metadata->'gif_loop_tune'->>'fallback_to_base')::boolean, FALSE) THEN TRUE
        ELSE FALSE
    END,
    gif_loop_tune_score = COALESCE((metadata->'gif_loop_tune'->>'score')::double precision, 0),
    gif_loop_tune_loop_closure = COALESCE((metadata->'gif_loop_tune'->>'loop_closure')::double precision, 0),
    gif_loop_tune_motion_mean = COALESCE((metadata->'gif_loop_tune'->>'motion_mean')::double precision, 0),
    gif_loop_tune_effective_sec = COALESCE((metadata->'gif_loop_tune'->>'effective_sec')::double precision, 0)
WHERE format = 'gif';

COMMIT;
