package models

import (
	"time"

	"gorm.io/datatypes"
)

type VideoImageJobPublic struct {
	ID              uint64         `gorm:"primaryKey;autoIncrement"`
	UserID          uint64         `gorm:"column:user_id;index"`
	Title           string         `gorm:"column:title;size:255"`
	SourceVideoKey  string         `gorm:"column:source_video_key;size:512"`
	SourceVideoName string         `gorm:"column:source_video_name;size:255"`
	SourceVideoExt  string         `gorm:"column:source_video_ext;size:16"`
	SourceSizeBytes int64          `gorm:"column:source_size_bytes"`
	SourceMD5       string         `gorm:"column:source_md5;size:64"`
	RequestedFormat string         `gorm:"column:requested_format;size:16;index"`
	Status          string         `gorm:"column:status;size:32;index"`
	Stage           string         `gorm:"column:stage;size:32;index"`
	Progress        int            `gorm:"column:progress"`
	Options         datatypes.JSON `gorm:"column:options;type:jsonb"`
	Metrics         datatypes.JSON `gorm:"column:metrics;type:jsonb"`
	ErrorCode       string         `gorm:"column:error_code;size:64"`
	ErrorMessage    string         `gorm:"column:error_message;type:text"`
	IdempotencyKey  string         `gorm:"column:idempotency_key;size:64;index"`
	StartedAt       *time.Time     `gorm:"column:started_at"`
	FinishedAt      *time.Time     `gorm:"column:finished_at"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoImageJobPublic) TableName() string {
	return "public.video_image_jobs"
}

type VideoImageOutputPublic struct {
	ID                          uint64         `gorm:"primaryKey;autoIncrement"`
	JobID                       uint64         `gorm:"column:job_id;index"`
	UserID                      uint64         `gorm:"column:user_id;index"`
	Format                      string         `gorm:"column:format;size:16;index"`
	FileRole                    string         `gorm:"column:file_role;size:32;index"`
	ObjectKey                   string         `gorm:"column:object_key;size:512;uniqueIndex"`
	Bucket                      string         `gorm:"column:bucket;size:64"`
	MimeType                    string         `gorm:"column:mime_type;size:128"`
	SizeBytes                   int64          `gorm:"column:size_bytes"`
	Width                       int            `gorm:"column:width"`
	Height                      int            `gorm:"column:height"`
	DurationMs                  int            `gorm:"column:duration_ms"`
	FrameIndex                  int            `gorm:"column:frame_index"`
	ProposalID                  *uint64        `gorm:"column:proposal_id;index"`
	Score                       float64        `gorm:"column:score"`
	GIFLoopTuneApplied          bool           `gorm:"column:gif_loop_tune_applied"`
	GIFLoopTuneEffectiveApplied bool           `gorm:"column:gif_loop_tune_effective_applied"`
	GIFLoopTuneFallbackToBase   bool           `gorm:"column:gif_loop_tune_fallback_to_base"`
	GIFLoopTuneScore            float64        `gorm:"column:gif_loop_tune_score"`
	GIFLoopTuneLoopClosure      float64        `gorm:"column:gif_loop_tune_loop_closure"`
	GIFLoopTuneMotionMean       float64        `gorm:"column:gif_loop_tune_motion_mean"`
	GIFLoopTuneEffectiveSec     float64        `gorm:"column:gif_loop_tune_effective_sec"`
	SHA256                      string         `gorm:"column:sha256;size:64"`
	IsPrimary                   bool           `gorm:"column:is_primary"`
	Metadata                    datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt                   time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (VideoImageOutputPublic) TableName() string {
	return "public.video_image_outputs"
}

type VideoImagePackagePublic struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement"`
	JobID        uint64         `gorm:"column:job_id;uniqueIndex"`
	UserID       uint64         `gorm:"column:user_id;index"`
	ZipObjectKey string         `gorm:"column:zip_object_key;size:512"`
	ZipName      string         `gorm:"column:zip_name;size:255"`
	ZipSizeBytes int64          `gorm:"column:zip_size_bytes"`
	FileCount    int            `gorm:"column:file_count"`
	Manifest     datatypes.JSON `gorm:"column:manifest;type:jsonb"`
	ExpiresAt    *time.Time     `gorm:"column:expires_at"`
	CreatedAt    time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (VideoImagePackagePublic) TableName() string {
	return "public.video_image_packages"
}

type VideoImageEventPublic struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement"`
	JobID     uint64         `gorm:"column:job_id;index"`
	Level     string         `gorm:"column:level;size:16;index"`
	Stage     string         `gorm:"column:stage;size:32;index"`
	Message   string         `gorm:"column:message;type:text"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (VideoImageEventPublic) TableName() string {
	return "public.video_image_events"
}

type VideoImageFeedbackPublic struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement"`
	JobID     uint64         `gorm:"column:job_id;index"`
	OutputID  *uint64        `gorm:"column:output_id;index"`
	UserID    uint64         `gorm:"column:user_id;index"`
	Action    string         `gorm:"column:action;size:32;index"`
	Weight    float64        `gorm:"column:weight"`
	SceneTag  string         `gorm:"column:scene_tag;size:64;index"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (VideoImageFeedbackPublic) TableName() string {
	return "public.video_image_feedback"
}

type VideoImageQualitySettingPublic struct {
	ID                              int16     `gorm:"primaryKey;default:1"`
	MinBrightness                   float64   `gorm:"column:min_brightness"`
	MaxBrightness                   float64   `gorm:"column:max_brightness"`
	BlurThresholdFactor             float64   `gorm:"column:blur_threshold_factor"`
	BlurThresholdMin                float64   `gorm:"column:blur_threshold_min"`
	BlurThresholdMax                float64   `gorm:"column:blur_threshold_max"`
	DuplicateHammingThreshold       int       `gorm:"column:duplicate_hamming_threshold"`
	DuplicateBacktrackFrames        int       `gorm:"column:duplicate_backtrack_frames"`
	FallbackBlurRelaxFactor         float64   `gorm:"column:fallback_blur_relax_factor"`
	FallbackHammingThreshold        int       `gorm:"column:fallback_hamming_threshold"`
	MinKeepBase                     int       `gorm:"column:min_keep_base"`
	MinKeepRatio                    float64   `gorm:"column:min_keep_ratio"`
	QualityAnalysisWorkers          int       `gorm:"column:quality_analysis_workers"`
	UploadConcurrency               int       `gorm:"column:upload_concurrency"`
	GIFDefaultFPS                   int       `gorm:"column:gif_default_fps"`
	GIFDefaultMaxColors             int       `gorm:"column:gif_default_max_colors"`
	GIFDitherMode                   string    `gorm:"column:gif_dither_mode;size:32"`
	GIFTargetSizeKB                 int       `gorm:"column:gif_target_size_kb"`
	GIFLoopTuneEnabled              bool      `gorm:"column:gif_loop_tune_enabled"`
	GIFLoopTuneMinEnableSec         float64   `gorm:"column:gif_loop_tune_min_enable_sec"`
	GIFLoopTuneMinImprovement       float64   `gorm:"column:gif_loop_tune_min_improvement"`
	GIFLoopTuneMotionTarget         float64   `gorm:"column:gif_loop_tune_motion_target"`
	GIFLoopTunePreferDuration       float64   `gorm:"column:gif_loop_tune_prefer_duration_sec"`
	GIFCandidateMaxOutputs          int       `gorm:"column:gif_candidate_max_outputs"`
	GIFCandidateConfidenceThreshold float64   `gorm:"column:gif_candidate_confidence_threshold"`
	GIFCandidateDedupIOUThreshold   float64   `gorm:"column:gif_candidate_dedup_iou_threshold"`
	WebPTargetSizeKB                int       `gorm:"column:webp_target_size_kb"`
	JPGTargetSizeKB                 int       `gorm:"column:jpg_target_size_kb"`
	PNGTargetSizeKB                 int       `gorm:"column:png_target_size_kb"`
	StillMinBlurScore               float64   `gorm:"column:still_min_blur_score"`
	StillMinExposureScore           float64   `gorm:"column:still_min_exposure_score"`
	StillMinWidth                   int       `gorm:"column:still_min_width"`
	StillMinHeight                  int       `gorm:"column:still_min_height"`
	LiveCoverPortraitWeight         float64   `gorm:"column:live_cover_portrait_weight"`
	LiveCoverSceneMinSamples        int       `gorm:"column:live_cover_scene_min_samples"`
	LiveCoverGuardMinTotal          int       `gorm:"column:live_cover_guard_min_total"`
	LiveCoverGuardScoreFloor        float64   `gorm:"column:live_cover_guard_score_floor"`
	CreatedAt                       time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoImageQualitySettingPublic) TableName() string {
	return "public.video_image_quality_settings"
}

type VideoImageRolloutAuditPublic struct {
	ID                   uint64         `gorm:"primaryKey;autoIncrement"`
	AdminID              uint64         `gorm:"column:admin_id;index"`
	FromRolloutPercent   int            `gorm:"column:from_rollout_percent"`
	ToRolloutPercent     int            `gorm:"column:to_rollout_percent"`
	WindowLabel          string         `gorm:"column:window_label;size:16"`
	RecommendationState  string         `gorm:"column:recommendation_state;size:32;index"`
	RecommendationReason string         `gorm:"column:recommendation_reason;type:text"`
	Metadata             datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt            time.Time      `gorm:"column:created_at;autoCreateTime;index"`
}

func (VideoImageRolloutAuditPublic) TableName() string {
	return "public.video_image_rollout_audits"
}
