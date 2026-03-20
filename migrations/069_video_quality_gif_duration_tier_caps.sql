BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS gif_candidate_long_video_max_outputs INTEGER NOT NULL DEFAULT 3,
    ADD COLUMN IF NOT EXISTS gif_candidate_ultra_video_max_outputs INTEGER NOT NULL DEFAULT 2;

COMMENT ON COLUMN ops.video_quality_settings.gif_candidate_long_video_max_outputs IS 'GIF候选长视频产出上限（duration>=60s）';
COMMENT ON COLUMN ops.video_quality_settings.gif_candidate_ultra_video_max_outputs IS 'GIF候选超长视频产出上限（duration>=150s）';

UPDATE ops.video_quality_settings
SET gif_candidate_max_outputs = LEAST(GREATEST(COALESCE(gif_candidate_max_outputs, 3), 1), 6),
    gif_candidate_long_video_max_outputs = LEAST(
        GREATEST(COALESCE(gif_candidate_long_video_max_outputs, 3), 1),
        LEAST(GREATEST(COALESCE(gif_candidate_max_outputs, 3), 1), 6)
    ),
    gif_candidate_ultra_video_max_outputs = LEAST(
        GREATEST(COALESCE(gif_candidate_ultra_video_max_outputs, 2), 1),
        LEAST(
            GREATEST(COALESCE(gif_candidate_long_video_max_outputs, 3), 1),
            LEAST(GREATEST(COALESCE(gif_candidate_max_outputs, 3), 1), 6)
        )
    )
WHERE id = 1;

ALTER TABLE public.video_image_quality_settings
    ADD COLUMN IF NOT EXISTS gif_candidate_long_video_max_outputs INTEGER NOT NULL DEFAULT 3,
    ADD COLUMN IF NOT EXISTS gif_candidate_ultra_video_max_outputs INTEGER NOT NULL DEFAULT 2;

COMMENT ON COLUMN public.video_image_quality_settings.gif_candidate_long_video_max_outputs IS 'GIF候选长视频产出上限';
COMMENT ON COLUMN public.video_image_quality_settings.gif_candidate_ultra_video_max_outputs IS 'GIF候选超长视频产出上限';

UPDATE public.video_image_quality_settings
SET gif_candidate_max_outputs = LEAST(GREATEST(COALESCE(gif_candidate_max_outputs, 3), 1), 6),
    gif_candidate_long_video_max_outputs = LEAST(
        GREATEST(COALESCE(gif_candidate_long_video_max_outputs, 3), 1),
        LEAST(GREATEST(COALESCE(gif_candidate_max_outputs, 3), 1), 6)
    ),
    gif_candidate_ultra_video_max_outputs = LEAST(
        GREATEST(COALESCE(gif_candidate_ultra_video_max_outputs, 2), 1),
        LEAST(
            GREATEST(COALESCE(gif_candidate_long_video_max_outputs, 3), 1),
            LEAST(GREATEST(COALESCE(gif_candidate_max_outputs, 3), 1), 6)
        )
    )
WHERE id = 1;

COMMIT;
