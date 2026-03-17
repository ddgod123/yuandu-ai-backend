BEGIN;

ALTER TABLE ops.video_quality_settings
    ADD COLUMN IF NOT EXISTS gif_candidate_max_outputs INTEGER NOT NULL DEFAULT 3,
    ADD COLUMN IF NOT EXISTS gif_candidate_confidence_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.35,
    ADD COLUMN IF NOT EXISTS gif_candidate_dedup_iou_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.45;

COMMENT ON COLUMN ops.video_quality_settings.gif_candidate_max_outputs IS 'GIF候选最大产出数（最终输出窗口上限）';
COMMENT ON COLUMN ops.video_quality_settings.gif_candidate_confidence_threshold IS 'GIF候选置信阈值（低于该值优先淘汰，0表示关闭）';
COMMENT ON COLUMN ops.video_quality_settings.gif_candidate_dedup_iou_threshold IS 'GIF候选去重IoU阈值（重叠超过阈值视为重复）';

UPDATE ops.video_quality_settings
SET gif_candidate_max_outputs = LEAST(GREATEST(COALESCE(gif_candidate_max_outputs, 3), 1), 6),
    gif_candidate_confidence_threshold = LEAST(GREATEST(COALESCE(gif_candidate_confidence_threshold, 0.35), 0), 0.95),
    gif_candidate_dedup_iou_threshold = LEAST(GREATEST(COALESCE(NULLIF(gif_candidate_dedup_iou_threshold, 0), 0.45), 0.1), 0.95)
WHERE id = 1;

ALTER TABLE public.video_image_quality_settings
    ADD COLUMN IF NOT EXISTS gif_candidate_max_outputs INTEGER NOT NULL DEFAULT 3,
    ADD COLUMN IF NOT EXISTS gif_candidate_confidence_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.35,
    ADD COLUMN IF NOT EXISTS gif_candidate_dedup_iou_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.45;

COMMENT ON COLUMN public.video_image_quality_settings.gif_candidate_max_outputs IS 'GIF候选最大产出数';
COMMENT ON COLUMN public.video_image_quality_settings.gif_candidate_confidence_threshold IS 'GIF候选置信阈值';
COMMENT ON COLUMN public.video_image_quality_settings.gif_candidate_dedup_iou_threshold IS 'GIF候选去重IoU阈值';

UPDATE public.video_image_quality_settings
SET gif_candidate_max_outputs = LEAST(GREATEST(COALESCE(gif_candidate_max_outputs, 3), 1), 6),
    gif_candidate_confidence_threshold = LEAST(GREATEST(COALESCE(gif_candidate_confidence_threshold, 0.35), 0), 0.95),
    gif_candidate_dedup_iou_threshold = LEAST(GREATEST(COALESCE(NULLIF(gif_candidate_dedup_iou_threshold, 0), 0.45), 0.1), 0.95)
WHERE id = 1;

COMMIT;
