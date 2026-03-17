BEGIN;

CREATE TABLE IF NOT EXISTS ops.video_quality_settings (
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
    gif_default_fps INTEGER NOT NULL DEFAULT 12,
    gif_default_max_colors INTEGER NOT NULL DEFAULT 128,
    gif_dither_mode VARCHAR(32) NOT NULL DEFAULT 'sierra2_4a',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO ops.video_quality_settings (
    id,
    min_brightness,
    max_brightness,
    blur_threshold_factor,
    blur_threshold_min,
    blur_threshold_max,
    duplicate_hamming_threshold,
    duplicate_backtrack_frames,
    fallback_blur_relax_factor,
    fallback_hamming_threshold,
    min_keep_base,
    min_keep_ratio,
    gif_default_fps,
    gif_default_max_colors,
    gif_dither_mode
)
VALUES (
    1,
    16,
    244,
    0.22,
    12,
    120,
    5,
    4,
    0.5,
    1,
    6,
    0.35,
    12,
    128,
    'sierra2_4a'
)
ON CONFLICT (id) DO NOTHING;

COMMENT ON TABLE ops.video_quality_settings IS '视频转图片质量调优参数（单例）';
COMMENT ON COLUMN ops.video_quality_settings.min_brightness IS '允许的最小亮度阈值（0-255）';
COMMENT ON COLUMN ops.video_quality_settings.max_brightness IS '允许的最大亮度阈值（0-255）';
COMMENT ON COLUMN ops.video_quality_settings.blur_threshold_factor IS '模糊阈值系数（基于拉普拉斯中位数）';
COMMENT ON COLUMN ops.video_quality_settings.blur_threshold_min IS '模糊阈值下限';
COMMENT ON COLUMN ops.video_quality_settings.blur_threshold_max IS '模糊阈值上限';
COMMENT ON COLUMN ops.video_quality_settings.duplicate_hamming_threshold IS '近重复帧哈希距离阈值';
COMMENT ON COLUMN ops.video_quality_settings.duplicate_backtrack_frames IS '近重复比较回看帧数';
COMMENT ON COLUMN ops.video_quality_settings.fallback_blur_relax_factor IS '回退阶段模糊阈值放宽系数';
COMMENT ON COLUMN ops.video_quality_settings.fallback_hamming_threshold IS '回退阶段近重复哈希阈值';
COMMENT ON COLUMN ops.video_quality_settings.min_keep_base IS '最少保留帧基础值';
COMMENT ON COLUMN ops.video_quality_settings.min_keep_ratio IS '最少保留帧比例';
COMMENT ON COLUMN ops.video_quality_settings.gif_default_fps IS 'GIF 默认帧率';
COMMENT ON COLUMN ops.video_quality_settings.gif_default_max_colors IS 'GIF 默认颜色数';
COMMENT ON COLUMN ops.video_quality_settings.gif_dither_mode IS 'GIF 默认抖动模式';

COMMIT;
