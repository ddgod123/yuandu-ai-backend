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
	ID                                   int16     `gorm:"primaryKey;default:1"`
	MinBrightness                        float64   `gorm:"column:min_brightness"`
	MaxBrightness                        float64   `gorm:"column:max_brightness"`
	BlurThresholdFactor                  float64   `gorm:"column:blur_threshold_factor"`
	BlurThresholdMin                     float64   `gorm:"column:blur_threshold_min"`
	BlurThresholdMax                     float64   `gorm:"column:blur_threshold_max"`
	DuplicateHammingThreshold            int       `gorm:"column:duplicate_hamming_threshold"`
	DuplicateBacktrackFrames             int       `gorm:"column:duplicate_backtrack_frames"`
	FallbackBlurRelaxFactor              float64   `gorm:"column:fallback_blur_relax_factor"`
	FallbackHammingThreshold             int       `gorm:"column:fallback_hamming_threshold"`
	MinKeepBase                          int       `gorm:"column:min_keep_base"`
	MinKeepRatio                         float64   `gorm:"column:min_keep_ratio"`
	QualityAnalysisWorkers               int       `gorm:"column:quality_analysis_workers"`
	UploadConcurrency                    int       `gorm:"column:upload_concurrency"`
	GIFDefaultFPS                        int       `gorm:"column:gif_default_fps"`
	GIFDefaultMaxColors                  int       `gorm:"column:gif_default_max_colors"`
	GIFDitherMode                        string    `gorm:"column:gif_dither_mode;size:32"`
	GIFTargetSizeKB                      int       `gorm:"column:gif_target_size_kb"`
	GIFGifsicleEnabled                   bool      `gorm:"column:gif_gifsicle_enabled"`
	GIFGifsicleLevel                     int       `gorm:"column:gif_gifsicle_level"`
	GIFGifsicleSkipBelowKB               int       `gorm:"column:gif_gifsicle_skip_below_kb"`
	GIFGifsicleMinGainRatio              float64   `gorm:"column:gif_gifsicle_min_gain_ratio"`
	GIFLoopTuneEnabled                   bool      `gorm:"column:gif_loop_tune_enabled"`
	GIFLoopTuneMinEnableSec              float64   `gorm:"column:gif_loop_tune_min_enable_sec"`
	GIFLoopTuneMinImprovement            float64   `gorm:"column:gif_loop_tune_min_improvement"`
	GIFLoopTuneMotionTarget              float64   `gorm:"column:gif_loop_tune_motion_target"`
	GIFLoopTunePreferDuration            float64   `gorm:"column:gif_loop_tune_prefer_duration_sec"`
	GIFCandidateMaxOutputs               int       `gorm:"column:gif_candidate_max_outputs"`
	GIFCandidateLongVideoMaxOutputs      int       `gorm:"column:gif_candidate_long_video_max_outputs"`
	GIFCandidateUltraVideoMaxOutputs     int       `gorm:"column:gif_candidate_ultra_video_max_outputs"`
	GIFCandidateConfidenceThreshold      float64   `gorm:"column:gif_candidate_confidence_threshold"`
	GIFCandidateDedupIOUThreshold        float64   `gorm:"column:gif_candidate_dedup_iou_threshold"`
	GIFRenderBudgetNormalMultiplier      float64   `gorm:"column:gif_render_budget_normal_mult"`
	GIFRenderBudgetLongMultiplier        float64   `gorm:"column:gif_render_budget_long_mult"`
	GIFRenderBudgetUltraMultiplier       float64   `gorm:"column:gif_render_budget_ultra_mult"`
	GIFPipelineShortVideoMaxSec          float64   `gorm:"column:gif_pipeline_short_video_max_sec"`
	GIFPipelineLongVideoMinSec           float64   `gorm:"column:gif_pipeline_long_video_min_sec"`
	GIFPipelineShortVideoMode            string    `gorm:"column:gif_pipeline_short_video_mode;size:16"`
	GIFPipelineDefaultMode               string    `gorm:"column:gif_pipeline_default_mode;size:16"`
	GIFPipelineLongVideoMode             string    `gorm:"column:gif_pipeline_long_video_mode;size:16"`
	GIFPipelineHighPriorityEnabled       bool      `gorm:"column:gif_pipeline_high_priority_enabled"`
	GIFPipelineHighPriorityMode          string    `gorm:"column:gif_pipeline_high_priority_mode;size:16"`
	GIFDurationTierMediumSec             float64   `gorm:"column:gif_duration_tier_medium_sec"`
	GIFDurationTierLongSec               float64   `gorm:"column:gif_duration_tier_long_sec"`
	GIFDurationTierUltraSec              float64   `gorm:"column:gif_duration_tier_ultra_sec"`
	GIFSegmentTimeoutMinSec              int       `gorm:"column:gif_segment_timeout_min_sec"`
	GIFSegmentTimeoutMaxSec              int       `gorm:"column:gif_segment_timeout_max_sec"`
	GIFSegmentTimeoutFallbackCapSec      int       `gorm:"column:gif_segment_timeout_fallback_cap_sec"`
	GIFSegmentTimeoutEmergencyCapSec     int       `gorm:"column:gif_segment_timeout_emergency_cap_sec"`
	GIFSegmentTimeoutLastResortCapSec    int       `gorm:"column:gif_segment_timeout_last_resort_cap_sec"`
	GIFRenderRetryMaxAttempts            int       `gorm:"column:gif_render_retry_max_attempts"`
	GIFRenderRetryPrimaryColorsFloor     int       `gorm:"column:gif_render_retry_primary_colors_floor"`
	GIFRenderRetryPrimaryColorsStep      int       `gorm:"column:gif_render_retry_primary_colors_step"`
	GIFRenderRetryFPSFloor               int       `gorm:"column:gif_render_retry_fps_floor"`
	GIFRenderRetryFPSStep                int       `gorm:"column:gif_render_retry_fps_step"`
	GIFRenderRetryWidthTrigger           int       `gorm:"column:gif_render_retry_width_trigger"`
	GIFRenderRetryWidthScale             float64   `gorm:"column:gif_render_retry_width_scale"`
	GIFRenderRetryWidthFloor             int       `gorm:"column:gif_render_retry_width_floor"`
	GIFRenderRetrySecondaryColorsFloor   int       `gorm:"column:gif_render_retry_secondary_colors_floor"`
	GIFRenderRetrySecondaryColorsStep    int       `gorm:"column:gif_render_retry_secondary_colors_step"`
	GIFRenderInitialSizeFPSCap           int       `gorm:"column:gif_render_initial_size_fps_cap"`
	GIFRenderInitialClarityFPSFloor      int       `gorm:"column:gif_render_initial_clarity_fps_floor"`
	GIFRenderInitialSizeColorsCap        int       `gorm:"column:gif_render_initial_size_colors_cap"`
	GIFRenderInitialClarityColorsFloor   int       `gorm:"column:gif_render_initial_clarity_colors_floor"`
	GIFMotionLowScoreThreshold           float64   `gorm:"column:gif_motion_low_score_threshold"`
	GIFMotionHighScoreThreshold          float64   `gorm:"column:gif_motion_high_score_threshold"`
	GIFMotionLowFPSDelta                 int       `gorm:"column:gif_motion_low_fps_delta"`
	GIFMotionHighFPSDelta                int       `gorm:"column:gif_motion_high_fps_delta"`
	GIFAdaptiveFPSMin                    int       `gorm:"column:gif_adaptive_fps_min"`
	GIFAdaptiveFPSMax                    int       `gorm:"column:gif_adaptive_fps_max"`
	GIFWidthSizeLow                      int       `gorm:"column:gif_width_size_low"`
	GIFWidthSizeMedium                   int       `gorm:"column:gif_width_size_medium"`
	GIFWidthSizeHigh                     int       `gorm:"column:gif_width_size_high"`
	GIFWidthClarityLow                   int       `gorm:"column:gif_width_clarity_low"`
	GIFWidthClarityMedium                int       `gorm:"column:gif_width_clarity_medium"`
	GIFWidthClarityHigh                  int       `gorm:"column:gif_width_clarity_high"`
	GIFColorsSizeLow                     int       `gorm:"column:gif_colors_size_low"`
	GIFColorsSizeMedium                  int       `gorm:"column:gif_colors_size_medium"`
	GIFColorsSizeHigh                    int       `gorm:"column:gif_colors_size_high"`
	GIFColorsClarityLow                  int       `gorm:"column:gif_colors_clarity_low"`
	GIFColorsClarityMedium               int       `gorm:"column:gif_colors_clarity_medium"`
	GIFColorsClarityHigh                 int       `gorm:"column:gif_colors_clarity_high"`
	GIFDurationLowSec                    float64   `gorm:"column:gif_duration_low_sec"`
	GIFDurationMediumSec                 float64   `gorm:"column:gif_duration_medium_sec"`
	GIFDurationHighSec                   float64   `gorm:"column:gif_duration_high_sec"`
	GIFDurationSizeProfileMaxSec         float64   `gorm:"column:gif_duration_size_profile_max_sec"`
	GIFDownshiftHighResLongSideThreshold int       `gorm:"column:gif_downshift_high_res_long_side_threshold"`
	GIFDownshiftEarlyDurationSec         float64   `gorm:"column:gif_downshift_early_duration_sec"`
	GIFDownshiftEarlyLongSideThreshold   int       `gorm:"column:gif_downshift_early_long_side_threshold"`
	GIFDownshiftMediumFPSCap             int       `gorm:"column:gif_downshift_medium_fps_cap"`
	GIFDownshiftMediumWidthCap           int       `gorm:"column:gif_downshift_medium_width_cap"`
	GIFDownshiftMediumColorsCap          int       `gorm:"column:gif_downshift_medium_colors_cap"`
	GIFDownshiftMediumDurationCapSec     float64   `gorm:"column:gif_downshift_medium_duration_cap_sec"`
	GIFDownshiftLongFPSCap               int       `gorm:"column:gif_downshift_long_fps_cap"`
	GIFDownshiftLongWidthCap             int       `gorm:"column:gif_downshift_long_width_cap"`
	GIFDownshiftLongColorsCap            int       `gorm:"column:gif_downshift_long_colors_cap"`
	GIFDownshiftLongDurationCapSec       float64   `gorm:"column:gif_downshift_long_duration_cap_sec"`
	GIFDownshiftUltraFPSCap              int       `gorm:"column:gif_downshift_ultra_fps_cap"`
	GIFDownshiftUltraWidthCap            int       `gorm:"column:gif_downshift_ultra_width_cap"`
	GIFDownshiftUltraColorsCap           int       `gorm:"column:gif_downshift_ultra_colors_cap"`
	GIFDownshiftUltraDurationCapSec      float64   `gorm:"column:gif_downshift_ultra_duration_cap_sec"`
	GIFDownshiftHighResFPSCap            int       `gorm:"column:gif_downshift_high_res_fps_cap"`
	GIFDownshiftHighResWidthCap          int       `gorm:"column:gif_downshift_high_res_width_cap"`
	GIFDownshiftHighResColorsCap         int       `gorm:"column:gif_downshift_high_res_colors_cap"`
	GIFDownshiftHighResDurationCapSec    float64   `gorm:"column:gif_downshift_high_res_duration_cap_sec"`
	GIFTimeoutFallbackFPSCap             int       `gorm:"column:gif_timeout_fallback_fps_cap"`
	GIFTimeoutFallbackWidthCap           int       `gorm:"column:gif_timeout_fallback_width_cap"`
	GIFTimeoutFallbackColorsCap          int       `gorm:"column:gif_timeout_fallback_colors_cap"`
	GIFTimeoutFallbackMinWidth           int       `gorm:"column:gif_timeout_fallback_min_width"`
	GIFTimeoutFallbackUltraFPSCap        int       `gorm:"column:gif_timeout_fallback_ultra_fps_cap"`
	GIFTimeoutFallbackUltraWidthCap      int       `gorm:"column:gif_timeout_fallback_ultra_width_cap"`
	GIFTimeoutFallbackUltraColorsCap     int       `gorm:"column:gif_timeout_fallback_ultra_colors_cap"`
	GIFTimeoutEmergencyFPSCap            int       `gorm:"column:gif_timeout_emergency_fps_cap"`
	GIFTimeoutEmergencyWidthCap          int       `gorm:"column:gif_timeout_emergency_width_cap"`
	GIFTimeoutEmergencyColorsCap         int       `gorm:"column:gif_timeout_emergency_colors_cap"`
	GIFTimeoutEmergencyMinWidth          int       `gorm:"column:gif_timeout_emergency_min_width"`
	GIFTimeoutEmergencyDurationTrigger   float64   `gorm:"column:gif_timeout_emergency_duration_trigger_sec"`
	GIFTimeoutEmergencyDurationScale     float64   `gorm:"column:gif_timeout_emergency_duration_scale"`
	GIFTimeoutEmergencyDurationMinSec    float64   `gorm:"column:gif_timeout_emergency_duration_min_sec"`
	GIFTimeoutLastResortFPSCap           int       `gorm:"column:gif_timeout_last_resort_fps_cap"`
	GIFTimeoutLastResortWidthCap         int       `gorm:"column:gif_timeout_last_resort_width_cap"`
	GIFTimeoutLastResortColorsCap        int       `gorm:"column:gif_timeout_last_resort_colors_cap"`
	GIFTimeoutLastResortMinWidth         int       `gorm:"column:gif_timeout_last_resort_min_width"`
	GIFTimeoutLastResortDurationMinSec   float64   `gorm:"column:gif_timeout_last_resort_duration_min_sec"`
	GIFTimeoutLastResortDurationMaxSec   float64   `gorm:"column:gif_timeout_last_resort_duration_max_sec"`
	WebPTargetSizeKB                     int       `gorm:"column:webp_target_size_kb"`
	JPGTargetSizeKB                      int       `gorm:"column:jpg_target_size_kb"`
	PNGTargetSizeKB                      int       `gorm:"column:png_target_size_kb"`
	StillMinBlurScore                    float64   `gorm:"column:still_min_blur_score"`
	StillMinExposureScore                float64   `gorm:"column:still_min_exposure_score"`
	StillMinWidth                        int       `gorm:"column:still_min_width"`
	StillMinHeight                       int       `gorm:"column:still_min_height"`
	LiveCoverPortraitWeight              float64   `gorm:"column:live_cover_portrait_weight"`
	LiveCoverSceneMinSamples             int       `gorm:"column:live_cover_scene_min_samples"`
	LiveCoverGuardMinTotal               int       `gorm:"column:live_cover_guard_min_total"`
	LiveCoverGuardScoreFloor             float64   `gorm:"column:live_cover_guard_score_floor"`
	CreatedAt                            time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                            time.Time `gorm:"column:updated_at;autoUpdateTime"`
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
