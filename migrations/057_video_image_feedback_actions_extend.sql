BEGIN;

ALTER TABLE public.video_image_feedback
    DROP CONSTRAINT IF EXISTS chk_video_image_feedback_action;

ALTER TABLE public.video_image_feedback
    ADD CONSTRAINT chk_video_image_feedback_action
    CHECK (
        action IN (
            'download',
            'favorite',
            'share',
            'use',
            'like',
            'neutral',
            'dislike',
            'top_pick'
        )
    );

COMMENT ON CONSTRAINT chk_video_image_feedback_action ON public.video_image_feedback
    IS '反馈动作枚举：download/favorite/share/use/like/neutral/dislike/top_pick';

COMMIT;
