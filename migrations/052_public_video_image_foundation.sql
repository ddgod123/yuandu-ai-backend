BEGIN;

CREATE TABLE IF NOT EXISTS public.video_image_jobs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    title VARCHAR(255) NOT NULL DEFAULT '',
    source_video_key VARCHAR(512) NOT NULL,
    source_video_name VARCHAR(255) NOT NULL DEFAULT '',
    source_video_ext VARCHAR(16) NOT NULL DEFAULT '',
    source_size_bytes BIGINT NOT NULL DEFAULT 0,
    source_md5 VARCHAR(64) NOT NULL DEFAULT '',
    requested_format VARCHAR(16) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'queued',
    stage VARCHAR(32) NOT NULL DEFAULT 'queued',
    progress SMALLINT NOT NULL DEFAULT 0,
    options JSONB NOT NULL DEFAULT '{}'::jsonb,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    idempotency_key VARCHAR(64) NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NULL,
    finished_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_video_image_jobs_requested_format CHECK (requested_format IN ('gif', 'webp', 'jpg', 'png', 'live', 'svg')),
    CONSTRAINT chk_video_image_jobs_status CHECK (status IN ('queued', 'running', 'done', 'failed', 'cancelled')),
    CONSTRAINT chk_video_image_jobs_stage CHECK (stage IN ('queued', 'preprocessing', 'analyzing', 'rendering', 'uploading', 'indexing', 'done', 'failed', 'cancelled', 'retrying')),
    CONSTRAINT chk_video_image_jobs_progress CHECK (progress >= 0 AND progress <= 100)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_image_jobs_idempotency_key
    ON public.video_image_jobs (idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_video_image_jobs_user_created
    ON public.video_image_jobs (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_video_image_jobs_status_created
    ON public.video_image_jobs (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_video_image_jobs_stage_updated
    ON public.video_image_jobs (stage, updated_at DESC);

COMMENT ON TABLE public.video_image_jobs IS '视频转图片任务主表（按用户隔离，一次任务单格式）';
COMMENT ON COLUMN public.video_image_jobs.user_id IS '创建任务的用户ID';
COMMENT ON COLUMN public.video_image_jobs.source_video_key IS '七牛源视频对象Key';
COMMENT ON COLUMN public.video_image_jobs.requested_format IS '本次任务目标格式（单选）';
COMMENT ON COLUMN public.video_image_jobs.options IS '任务参数（截取窗口、质量模板、探测信息等）';
COMMENT ON COLUMN public.video_image_jobs.metrics IS '任务运行指标（质量报告、耗时、反馈埋点等）';
COMMENT ON COLUMN public.video_image_jobs.idempotency_key IS '幂等键（防重复创建）';

CREATE TABLE IF NOT EXISTS public.video_image_outputs (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    format VARCHAR(16) NOT NULL,
    file_role VARCHAR(32) NOT NULL DEFAULT 'main',
    object_key VARCHAR(512) NOT NULL,
    bucket VARCHAR(64) NOT NULL DEFAULT '',
    mime_type VARCHAR(128) NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    frame_index INTEGER NOT NULL DEFAULT 0,
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    sha256 VARCHAR(64) NOT NULL DEFAULT '',
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_video_image_outputs_format CHECK (format IN ('gif', 'webp', 'jpg', 'png', 'live', 'svg', 'zip', 'mov', 'mp4'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_image_outputs_object_key
    ON public.video_image_outputs (object_key);

CREATE INDEX IF NOT EXISTS idx_video_image_outputs_job_format
    ON public.video_image_outputs (job_id, format, file_role);

CREATE INDEX IF NOT EXISTS idx_video_image_outputs_user_created
    ON public.video_image_outputs (user_id, created_at DESC);

COMMENT ON TABLE public.video_image_outputs IS '视频转图片产物文件表（每个输出文件一条）';
COMMENT ON COLUMN public.video_image_outputs.file_role IS '文件角色（main/cover/thumb/package/live_video等）';
COMMENT ON COLUMN public.video_image_outputs.object_key IS '七牛对象Key';
COMMENT ON COLUMN public.video_image_outputs.metadata IS '扩展信息（窗口评分、loop tuning等）';

CREATE TABLE IF NOT EXISTS public.video_image_packages (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    zip_object_key VARCHAR(512) NOT NULL,
    zip_name VARCHAR(255) NOT NULL,
    zip_size_bytes BIGINT NOT NULL DEFAULT 0,
    file_count INTEGER NOT NULL DEFAULT 0,
    manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_video_image_packages_job UNIQUE (job_id)
);

CREATE INDEX IF NOT EXISTS idx_video_image_packages_user_created
    ON public.video_image_packages (user_id, created_at DESC);

COMMENT ON TABLE public.video_image_packages IS '视频转图片ZIP打包记录（每任务最多一条主包）';
COMMENT ON COLUMN public.video_image_packages.manifest IS 'ZIP内文件清单与摘要信息';

CREATE TABLE IF NOT EXISTS public.video_image_events (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL,
    level VARCHAR(16) NOT NULL DEFAULT 'info',
    stage VARCHAR(32) NOT NULL DEFAULT 'queued',
    message TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_image_events_job_created
    ON public.video_image_events (job_id, id DESC);

COMMENT ON TABLE public.video_image_events IS '视频转图片任务事件日志（用于后台时间线排障）';

CREATE TABLE IF NOT EXISTS public.video_image_feedback (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL,
    output_id BIGINT NULL,
    user_id BIGINT NOT NULL,
    action VARCHAR(32) NOT NULL,
    weight DOUBLE PRECISION NOT NULL DEFAULT 1,
    scene_tag VARCHAR(64) NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_video_image_feedback_action CHECK (action IN ('download', 'favorite', 'share', 'use'))
);

CREATE INDEX IF NOT EXISTS idx_video_image_feedback_job_created
    ON public.video_image_feedback (job_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_video_image_feedback_user_created
    ON public.video_image_feedback (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_video_image_feedback_action_created
    ON public.video_image_feedback (action, created_at DESC);

COMMENT ON TABLE public.video_image_feedback IS '视频转图片反馈行为表（用于质量评估与后续训练）';

CREATE TABLE IF NOT EXISTS public.video_image_quality_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    min_brightness DOUBLE PRECISION NOT NULL DEFAULT 16,
    max_brightness DOUBLE PRECISION NOT NULL DEFAULT 244,
    blur_threshold_factor DOUBLE PRECISION NOT NULL DEFAULT 0.22,
    blur_threshold_min DOUBLE PRECISION NOT NULL DEFAULT 12,
    blur_threshold_max DOUBLE PRECISION NOT NULL DEFAULT 120,
    duplicate_hamming_threshold INTEGER NOT NULL DEFAULT 5,
    duplicate_backtrack_frames INTEGER NOT NULL DEFAULT 4,
    fallback_blur_relax_factor DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    fallback_hamming_threshold INTEGER NOT NULL DEFAULT 1,
    min_keep_base INTEGER NOT NULL DEFAULT 6,
    min_keep_ratio DOUBLE PRECISION NOT NULL DEFAULT 0.35,
    quality_analysis_workers INTEGER NOT NULL DEFAULT 4,
    upload_concurrency INTEGER NOT NULL DEFAULT 4,
    gif_default_fps INTEGER NOT NULL DEFAULT 12,
    gif_default_max_colors INTEGER NOT NULL DEFAULT 128,
    gif_dither_mode VARCHAR(32) NOT NULL DEFAULT 'sierra2_4a',
    gif_target_size_kb INTEGER NOT NULL DEFAULT 2048,
    gif_loop_tune_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    gif_loop_tune_min_enable_sec DOUBLE PRECISION NOT NULL DEFAULT 1.4,
    gif_loop_tune_min_improvement DOUBLE PRECISION NOT NULL DEFAULT 0.04,
    gif_loop_tune_motion_target DOUBLE PRECISION NOT NULL DEFAULT 0.22,
    gif_loop_tune_prefer_duration_sec DOUBLE PRECISION NOT NULL DEFAULT 2.4,
    webp_target_size_kb INTEGER NOT NULL DEFAULT 1536,
    jpg_target_size_kb INTEGER NOT NULL DEFAULT 512,
    png_target_size_kb INTEGER NOT NULL DEFAULT 1024,
    still_min_blur_score DOUBLE PRECISION NOT NULL DEFAULT 12,
    still_min_exposure_score DOUBLE PRECISION NOT NULL DEFAULT 0.28,
    still_min_width INTEGER NOT NULL DEFAULT 0,
    still_min_height INTEGER NOT NULL DEFAULT 0,
    live_cover_portrait_weight DOUBLE PRECISION NOT NULL DEFAULT 0.04,
    live_cover_scene_min_samples INTEGER NOT NULL DEFAULT 5,
    live_cover_guard_min_total INTEGER NOT NULL DEFAULT 20,
    live_cover_guard_score_floor DOUBLE PRECISION NOT NULL DEFAULT 0.58,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO public.video_image_quality_settings (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

COMMENT ON TABLE public.video_image_quality_settings IS '视频转图片质量设置（public 单例）';
COMMENT ON COLUMN public.video_image_quality_settings.gif_loop_tune_enabled IS '是否启用GIF循环闭合调优';

CREATE TABLE IF NOT EXISTS public.video_image_rollout_audits (
    id BIGSERIAL PRIMARY KEY,
    admin_id BIGINT NOT NULL,
    from_rollout_percent INTEGER NOT NULL DEFAULT 0,
    to_rollout_percent INTEGER NOT NULL DEFAULT 0,
    window_label VARCHAR(16) NOT NULL DEFAULT '24h',
    recommendation_state VARCHAR(32) NOT NULL DEFAULT 'hold',
    recommendation_reason TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_image_rollout_audits_created
    ON public.video_image_rollout_audits (created_at DESC);

COMMENT ON TABLE public.video_image_rollout_audits IS '视频转图片质量放量审计日志（public）';

COMMIT;
