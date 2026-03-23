package models

import (
	"time"

	"gorm.io/datatypes"
)

const (
	VideoJobStatusQueued    = "queued"
	VideoJobStatusRunning   = "running"
	VideoJobStatusDone      = "done"
	VideoJobStatusFailed    = "failed"
	VideoJobStatusCancelled = "cancelled"
)

const (
	VideoJobAssetDomainArchive = "archive"
	VideoJobAssetDomainVideo   = "video"
	VideoJobAssetDomainUGC     = "ugc"
	VideoJobAssetDomainAdmin   = "admin"
)

const (
	VideoJobStageQueued        = "queued"
	VideoJobStagePreprocessing = "preprocessing"
	VideoJobStageAnalyzing     = "analyzing"
	VideoJobStageRendering     = "rendering"
	VideoJobStageUploading     = "uploading"
	VideoJobStageIndexing      = "indexing"
	VideoJobStageDone          = "done"
	VideoJobStageFailed        = "failed"
	VideoJobStageCancelled     = "cancelled"
	VideoJobStageRetrying      = "retrying"
)

type VideoJob struct {
	ID                 uint64         `gorm:"primaryKey;autoIncrement"`
	UserID             uint64         `gorm:"column:user_id;index"`
	Title              string         `gorm:"column:title;size:255"`
	SourceVideoKey     string         `gorm:"column:source_video_key;size:512;index"`
	CategoryID         *uint64        `gorm:"column:category_id;index"`
	OutputFormats      string         `gorm:"column:output_formats;size:128"`
	Status             string         `gorm:"column:status;size:32;index"`
	Stage              string         `gorm:"column:stage;size:32;index"`
	Progress           int            `gorm:"column:progress"`
	Priority           string         `gorm:"column:priority;size:16;index"`
	Options            datatypes.JSON `gorm:"column:options;type:jsonb"`
	Metrics            datatypes.JSON `gorm:"column:metrics;type:jsonb"`
	ErrorMessage       string         `gorm:"column:error_message;type:text"`
	AssetDomain        string         `gorm:"column:asset_domain;size:32;index"`
	ResultCollectionID *uint64        `gorm:"column:result_collection_id;index"`
	QueuedAt           time.Time      `gorm:"column:queued_at;autoCreateTime"`
	StartedAt          *time.Time     `gorm:"column:started_at;index"`
	FinishedAt         *time.Time     `gorm:"column:finished_at;index"`
	CreatedAt          time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoJob) TableName() string {
	return "archive.video_jobs"
}

type VideoJobArtifact struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement"`
	JobID      uint64         `gorm:"column:job_id;index"`
	Type       string         `gorm:"column:type;size:32;index"`
	QiniuKey   string         `gorm:"column:qiniu_key;size:512"`
	MimeType   string         `gorm:"column:mime_type;size:128"`
	SizeBytes  int64          `gorm:"column:size_bytes"`
	Width      int            `gorm:"column:width"`
	Height     int            `gorm:"column:height"`
	DurationMs int            `gorm:"column:duration_ms"`
	Metadata   datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt  time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (VideoJobArtifact) TableName() string {
	return "archive.video_job_artifacts"
}

type VideoJobEvent struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement"`
	JobID     uint64         `gorm:"column:job_id;index"`
	Stage     string         `gorm:"column:stage;size:32;index"`
	Level     string         `gorm:"column:level;size:16;index"`
	Message   string         `gorm:"column:message;type:text"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (VideoJobEvent) TableName() string {
	return "archive.video_job_events"
}

type VideoJobGIFCandidate struct {
	ID              uint64         `gorm:"primaryKey;autoIncrement"`
	JobID           uint64         `gorm:"column:job_id;index"`
	StartMs         int            `gorm:"column:start_ms"`
	EndMs           int            `gorm:"column:end_ms"`
	DurationMs      int            `gorm:"column:duration_ms"`
	BaseScore       float64        `gorm:"column:base_score"`
	ConfidenceScore float64        `gorm:"column:confidence_score"`
	FinalRank       int            `gorm:"column:final_rank"`
	IsSelected      bool           `gorm:"column:is_selected;index"`
	RejectReason    string         `gorm:"column:reject_reason;size:64;index"`
	FeatureJSON     datatypes.JSON `gorm:"column:feature_json;type:jsonb"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (VideoJobGIFCandidate) TableName() string {
	return "archive.video_job_gif_candidates"
}

type VideoJobGIFEvaluation struct {
	ID              uint64         `gorm:"primaryKey;autoIncrement"`
	JobID           uint64         `gorm:"column:job_id;index"`
	OutputID        *uint64        `gorm:"column:output_id;index"`
	CandidateID     *uint64        `gorm:"column:candidate_id;index"`
	WindowStartMs   int            `gorm:"column:window_start_ms"`
	WindowEndMs     int            `gorm:"column:window_end_ms"`
	EmotionScore    float64        `gorm:"column:emotion_score"`
	ClarityScore    float64        `gorm:"column:clarity_score"`
	MotionScore     float64        `gorm:"column:motion_score"`
	LoopScore       float64        `gorm:"column:loop_score"`
	EfficiencyScore float64        `gorm:"column:efficiency_score"`
	OverallScore    float64        `gorm:"column:overall_score"`
	FeatureJSON     datatypes.JSON `gorm:"column:feature_json;type:jsonb"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoJobGIFEvaluation) TableName() string {
	return "archive.video_job_gif_evaluations"
}

type VideoJobGIFBaseline struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement"`
	BaselineDate       time.Time `gorm:"column:baseline_date;type:date;index"`
	WindowLabel        string    `gorm:"column:window_label;size:16"`
	Scope              string    `gorm:"column:scope;size:32"`
	RequestedFormat    string    `gorm:"column:requested_format;size:16"`
	SampleJobs         int64     `gorm:"column:sample_jobs"`
	DoneJobs           int64     `gorm:"column:done_jobs"`
	FailedJobs         int64     `gorm:"column:failed_jobs"`
	DoneRate           float64   `gorm:"column:done_rate"`
	FailedRate         float64   `gorm:"column:failed_rate"`
	SampleOutputs      int64     `gorm:"column:sample_outputs"`
	AvgEmotionScore    float64   `gorm:"column:avg_emotion_score"`
	AvgClarityScore    float64   `gorm:"column:avg_clarity_score"`
	AvgMotionScore     float64   `gorm:"column:avg_motion_score"`
	AvgLoopScore       float64   `gorm:"column:avg_loop_score"`
	AvgEfficiencyScore float64   `gorm:"column:avg_efficiency_score"`
	AvgOverallScore    float64   `gorm:"column:avg_overall_score"`
	AvgOutputScore     float64   `gorm:"column:avg_output_score"`
	AvgLoopClosure     float64   `gorm:"column:avg_loop_closure"`
	AvgSizeBytes       float64   `gorm:"column:avg_size_bytes"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoJobGIFBaseline) TableName() string {
	return "ops.video_job_gif_baselines"
}

type VideoJobGIFRerankLog struct {
	ID          uint64         `gorm:"primaryKey;autoIncrement"`
	JobID       uint64         `gorm:"column:job_id;index"`
	UserID      uint64         `gorm:"column:user_id;index"`
	CandidateID *uint64        `gorm:"column:candidate_id;index"`
	StartMs     int            `gorm:"column:start_ms"`
	EndMs       int            `gorm:"column:end_ms"`
	BeforeRank  int            `gorm:"column:before_rank"`
	AfterRank   int            `gorm:"column:after_rank"`
	BeforeScore float64        `gorm:"column:before_score"`
	AfterScore  float64        `gorm:"column:after_score"`
	ScoreDelta  float64        `gorm:"column:score_delta"`
	Reason      string         `gorm:"column:reason;size:64"`
	Metadata    datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (VideoJobGIFRerankLog) TableName() string {
	return "ops.video_job_gif_rerank_logs"
}

type VideoJobGIFManualScore struct {
	ID              uint64         `gorm:"primaryKey;autoIncrement"`
	SampleID        string         `gorm:"column:sample_id;size:64;index"`
	BaselineVersion string         `gorm:"column:baseline_version;size:64;index"`
	ReviewRound     string         `gorm:"column:review_round;size:32;index"`
	Reviewer        string         `gorm:"column:reviewer;size:64;index"`
	ReviewedAt      time.Time      `gorm:"column:reviewed_at;index"`
	JobID           *uint64        `gorm:"column:job_id;index"`
	OutputID        *uint64        `gorm:"column:output_id;index"`
	EmotionScore    float64        `gorm:"column:emotion_score"`
	ClarityScore    float64        `gorm:"column:clarity_score"`
	MotionScore     float64        `gorm:"column:motion_score"`
	LoopScore       float64        `gorm:"column:loop_score"`
	EfficiencyScore float64        `gorm:"column:efficiency_score"`
	OverallScore    float64        `gorm:"column:overall_score"`
	IsTopPick       bool           `gorm:"column:is_top_pick;index"`
	IsPass          bool           `gorm:"column:is_pass;index"`
	RejectReason    string         `gorm:"column:reject_reason;size:64;index"`
	ReviewNotes     string         `gorm:"column:review_notes;type:text"`
	Metadata        datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoJobGIFManualScore) TableName() string {
	return "ops.video_job_gif_manual_scores"
}

type VideoJobCost struct {
	ID                 uint64         `gorm:"primaryKey;autoIncrement"`
	JobID              uint64         `gorm:"column:job_id;uniqueIndex"`
	UserID             uint64         `gorm:"column:user_id;index"`
	Status             string         `gorm:"column:status;size:32;index"`
	CPUms              int64          `gorm:"column:cpu_ms"`
	GPUms              int64          `gorm:"column:gpu_ms"`
	ASRSeconds         float64        `gorm:"column:asr_seconds"`
	OCRFrames          int            `gorm:"column:ocr_frames"`
	StorageBytesRaw    int64          `gorm:"column:storage_bytes_raw"`
	StorageBytesOutput int64          `gorm:"column:storage_bytes_output"`
	OutputCount        int            `gorm:"column:output_count"`
	EstimatedCost      float64        `gorm:"column:estimated_cost"`
	Currency           string         `gorm:"column:currency;size:16"`
	PricingVersion     string         `gorm:"column:pricing_version;size:32"`
	Details            datatypes.JSON `gorm:"column:details;type:jsonb"`
	CreatedAt          time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoJobCost) TableName() string {
	return "ops.video_job_costs"
}

type VideoJobAIUsage struct {
	ID                uint64         `gorm:"primaryKey;autoIncrement"`
	JobID             uint64         `gorm:"column:job_id;index"`
	UserID            uint64         `gorm:"column:user_id;index"`
	Stage             string         `gorm:"column:stage;size:32;index"`
	Provider          string         `gorm:"column:provider;size:32"`
	Model             string         `gorm:"column:model;size:128"`
	Endpoint          string         `gorm:"column:endpoint;size:255"`
	InputTokens       int64          `gorm:"column:input_tokens"`
	OutputTokens      int64          `gorm:"column:output_tokens"`
	CachedInputTokens int64          `gorm:"column:cached_input_tokens"`
	ImageTokens       int64          `gorm:"column:image_tokens"`
	VideoTokens       int64          `gorm:"column:video_tokens"`
	AudioSeconds      float64        `gorm:"column:audio_seconds"`
	RequestDurationMs int64          `gorm:"column:request_duration_ms"`
	RequestStatus     string         `gorm:"column:request_status;size:16;index"`
	RequestError      string         `gorm:"column:request_error;type:text"`
	UnitPriceInput    float64        `gorm:"column:unit_price_input"`
	UnitPriceOutput   float64        `gorm:"column:unit_price_output"`
	UnitPriceCachedIn float64        `gorm:"column:unit_price_cached_input"`
	UnitPriceAudioMin float64        `gorm:"column:unit_price_audio_min"`
	CostUSD           float64        `gorm:"column:cost_usd"`
	Currency          string         `gorm:"column:currency;size:16"`
	PricingVersion    string         `gorm:"column:pricing_version;size:64"`
	PricingSourceURL  string         `gorm:"column:pricing_source_url;type:text"`
	Metadata          datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt         time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (VideoJobAIUsage) TableName() string {
	return "ops.video_job_ai_usage"
}

type VideoJobGIFAIProposal struct {
	ID                   uint64         `gorm:"primaryKey;autoIncrement"`
	JobID                uint64         `gorm:"column:job_id;index"`
	UserID               uint64         `gorm:"column:user_id;index"`
	Provider             string         `gorm:"column:provider;size:32"`
	Model                string         `gorm:"column:model;size:128"`
	Endpoint             string         `gorm:"column:endpoint;size:255"`
	PromptVersion        string         `gorm:"column:prompt_version;size:64"`
	ProposalRank         int            `gorm:"column:proposal_rank"`
	StartSec             float64        `gorm:"column:start_sec"`
	EndSec               float64        `gorm:"column:end_sec"`
	DurationSec          float64        `gorm:"column:duration_sec"`
	BaseScore            float64        `gorm:"column:base_score"`
	ProposalReason       string         `gorm:"column:proposal_reason;type:text"`
	SemanticTags         datatypes.JSON `gorm:"column:semantic_tags;type:jsonb"`
	ExpectedValueLevel   string         `gorm:"column:expected_value_level;size:32"`
	StandaloneConfidence float64        `gorm:"column:standalone_confidence"`
	LoopFriendlinessHint float64        `gorm:"column:loop_friendliness_hint"`
	Status               string         `gorm:"column:status;size:32;index"`
	Metadata             datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	RawResponse          datatypes.JSON `gorm:"column:raw_response;type:jsonb"`
	CreatedAt            time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt            time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoJobGIFAIProposal) TableName() string {
	return "archive.video_job_gif_ai_proposals"
}

type VideoJobGIFAIReview struct {
	ID                  uint64         `gorm:"primaryKey;autoIncrement"`
	JobID               uint64         `gorm:"column:job_id;index"`
	UserID              uint64         `gorm:"column:user_id;index"`
	OutputID            *uint64        `gorm:"column:output_id;index"`
	ProposalID          *uint64        `gorm:"column:proposal_id;index"`
	Provider            string         `gorm:"column:provider;size:32"`
	Model               string         `gorm:"column:model;size:128"`
	Endpoint            string         `gorm:"column:endpoint;size:255"`
	PromptVersion       string         `gorm:"column:prompt_version;size:64"`
	FinalRecommendation string         `gorm:"column:final_recommendation;size:32;index"`
	SemanticVerdict     float64        `gorm:"column:semantic_verdict"`
	DiagnosticReason    string         `gorm:"column:diagnostic_reason;type:text"`
	SuggestedAction     string         `gorm:"column:suggested_action;type:text"`
	Metadata            datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	RawResponse         datatypes.JSON `gorm:"column:raw_response;type:jsonb"`
	CreatedAt           time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt           time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoJobGIFAIReview) TableName() string {
	return "archive.video_job_gif_ai_reviews"
}

type VideoJobGIFAIDirective struct {
	ID                 uint64         `gorm:"primaryKey;autoIncrement"`
	JobID              uint64         `gorm:"column:job_id;index"`
	UserID             uint64         `gorm:"column:user_id;index"`
	Provider           string         `gorm:"column:provider;size:32"`
	Model              string         `gorm:"column:model;size:128"`
	Endpoint           string         `gorm:"column:endpoint;size:255"`
	PromptVersion      string         `gorm:"column:prompt_version;size:64"`
	BusinessGoal       string         `gorm:"column:business_goal;size:64"`
	Audience           string         `gorm:"column:audience;size:128"`
	MustCapture        datatypes.JSON `gorm:"column:must_capture;type:jsonb"`
	Avoid              datatypes.JSON `gorm:"column:avoid;type:jsonb"`
	ClipCountMin       int            `gorm:"column:clip_count_min"`
	ClipCountMax       int            `gorm:"column:clip_count_max"`
	DurationPrefMinSec float64        `gorm:"column:duration_pref_min_sec"`
	DurationPrefMaxSec float64        `gorm:"column:duration_pref_max_sec"`
	LoopPreference     float64        `gorm:"column:loop_preference"`
	StyleDirection     string         `gorm:"column:style_direction;type:text"`
	RiskFlags          datatypes.JSON `gorm:"column:risk_flags;type:jsonb"`
	QualityWeights     datatypes.JSON `gorm:"column:quality_weights;type:jsonb"`
	BriefVersion       string         `gorm:"column:brief_version;size:64"`
	ModelVersion       string         `gorm:"column:model_version;size:128"`
	DirectiveText      string         `gorm:"column:directive_text;type:text"`
	InputContextJSON   datatypes.JSON `gorm:"column:input_context_json;type:jsonb"`
	Status             string         `gorm:"column:status;size:16;index"`
	FallbackUsed       bool           `gorm:"column:fallback_used;index"`
	Metadata           datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	RawResponse        datatypes.JSON `gorm:"column:raw_response;type:jsonb"`
	CreatedAt          time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoJobGIFAIDirective) TableName() string {
	return "archive.video_job_gif_ai_directives"
}

type ComputeAccount struct {
	ID                   uint64    `gorm:"primaryKey;autoIncrement"`
	UserID               uint64    `gorm:"column:user_id;uniqueIndex"`
	AvailablePoints      int64     `gorm:"column:available_points"`
	FrozenPoints         int64     `gorm:"column:frozen_points"`
	DebtPoints           int64     `gorm:"column:debt_points"`
	TotalConsumedPoints  int64     `gorm:"column:total_consumed_points"`
	TotalRechargedPoints int64     `gorm:"column:total_recharged_points"`
	Status               string    `gorm:"column:status;size:32;index"`
	CreatedAt            time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt            time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (ComputeAccount) TableName() string {
	return "ops.compute_accounts"
}

type ComputeLedger struct {
	ID              uint64         `gorm:"primaryKey;autoIncrement"`
	AccountID       uint64         `gorm:"column:account_id;index"`
	UserID          uint64         `gorm:"column:user_id;index"`
	JobID           *uint64        `gorm:"column:job_id;index"`
	Type            string         `gorm:"column:type;size:32;index"`
	Points          int64          `gorm:"column:points"`
	AvailableBefore int64          `gorm:"column:available_before"`
	AvailableAfter  int64          `gorm:"column:available_after"`
	FrozenBefore    int64          `gorm:"column:frozen_before"`
	FrozenAfter     int64          `gorm:"column:frozen_after"`
	DebtBefore      int64          `gorm:"column:debt_before"`
	DebtAfter       int64          `gorm:"column:debt_after"`
	Remark          string         `gorm:"column:remark;type:text"`
	Metadata        datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (ComputeLedger) TableName() string {
	return "ops.compute_ledgers"
}

type ComputePointHold struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	JobID          uint64     `gorm:"column:job_id;uniqueIndex"`
	UserID         uint64     `gorm:"column:user_id;index"`
	AccountID      uint64     `gorm:"column:account_id;index"`
	ReservedPoints int64      `gorm:"column:reserved_points"`
	SettledPoints  int64      `gorm:"column:settled_points"`
	Status         string     `gorm:"column:status;size:32;index"`
	Remark         string     `gorm:"column:remark;type:text"`
	SettledAt      *time.Time `gorm:"column:settled_at;index"`
	CreatedAt      time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (ComputePointHold) TableName() string {
	return "ops.compute_point_holds"
}

type VideoQualitySetting struct {
	ID                                                int16     `gorm:"primaryKey;default:1"`
	MinBrightness                                     float64   `gorm:"column:min_brightness"`
	MaxBrightness                                     float64   `gorm:"column:max_brightness"`
	BlurThresholdFactor                               float64   `gorm:"column:blur_threshold_factor"`
	BlurThresholdMin                                  float64   `gorm:"column:blur_threshold_min"`
	BlurThresholdMax                                  float64   `gorm:"column:blur_threshold_max"`
	DuplicateHammingThreshold                         int       `gorm:"column:duplicate_hamming_threshold"`
	DuplicateBacktrackFrames                          int       `gorm:"column:duplicate_backtrack_frames"`
	FallbackBlurRelaxFactor                           float64   `gorm:"column:fallback_blur_relax_factor"`
	FallbackHammingThreshold                          int       `gorm:"column:fallback_hamming_threshold"`
	MinKeepBase                                       int       `gorm:"column:min_keep_base"`
	MinKeepRatio                                      float64   `gorm:"column:min_keep_ratio"`
	QualityAnalysisWorkers                            int       `gorm:"column:quality_analysis_workers"`
	UploadConcurrency                                 int       `gorm:"column:upload_concurrency"`
	GIFProfile                                        string    `gorm:"column:gif_profile;size:16"`
	WebPProfile                                       string    `gorm:"column:webp_profile;size:16"`
	LiveProfile                                       string    `gorm:"column:live_profile;size:16"`
	JPGProfile                                        string    `gorm:"column:jpg_profile;size:16"`
	PNGProfile                                        string    `gorm:"column:png_profile;size:16"`
	GIFDefaultFPS                                     int       `gorm:"column:gif_default_fps"`
	GIFDefaultMaxColors                               int       `gorm:"column:gif_default_max_colors"`
	GIFDitherMode                                     string    `gorm:"column:gif_dither_mode;size:32"`
	GIFTargetSizeKB                                   int       `gorm:"column:gif_target_size_kb"`
	GIFGifsicleEnabled                                bool      `gorm:"column:gif_gifsicle_enabled"`
	GIFGifsicleLevel                                  int       `gorm:"column:gif_gifsicle_level"`
	GIFGifsicleSkipBelowKB                            int       `gorm:"column:gif_gifsicle_skip_below_kb"`
	GIFGifsicleMinGainRatio                           float64   `gorm:"column:gif_gifsicle_min_gain_ratio"`
	GIFLoopTuneEnabled                                bool      `gorm:"column:gif_loop_tune_enabled"`
	GIFLoopTuneMinEnableSec                           float64   `gorm:"column:gif_loop_tune_min_enable_sec"`
	GIFLoopTuneMinImprovement                         float64   `gorm:"column:gif_loop_tune_min_improvement"`
	GIFLoopTuneMotionTarget                           float64   `gorm:"column:gif_loop_tune_motion_target"`
	GIFLoopTunePreferDuration                         float64   `gorm:"column:gif_loop_tune_prefer_duration_sec"`
	GIFCandidateMaxOutputs                            int       `gorm:"column:gif_candidate_max_outputs"`
	GIFCandidateLongVideoMaxOutputs                   int       `gorm:"column:gif_candidate_long_video_max_outputs"`
	GIFCandidateUltraVideoMaxOutputs                  int       `gorm:"column:gif_candidate_ultra_video_max_outputs"`
	GIFCandidateConfidenceThreshold                   float64   `gorm:"column:gif_candidate_confidence_threshold"`
	GIFCandidateDedupIOUThreshold                     float64   `gorm:"column:gif_candidate_dedup_iou_threshold"`
	GIFRenderBudgetNormalMultiplier                   float64   `gorm:"column:gif_render_budget_normal_mult"`
	GIFRenderBudgetLongMultiplier                     float64   `gorm:"column:gif_render_budget_long_mult"`
	GIFRenderBudgetUltraMultiplier                    float64   `gorm:"column:gif_render_budget_ultra_mult"`
	GIFPipelineShortVideoMaxSec                       float64   `gorm:"column:gif_pipeline_short_video_max_sec"`
	GIFPipelineLongVideoMinSec                        float64   `gorm:"column:gif_pipeline_long_video_min_sec"`
	GIFPipelineShortVideoMode                         string    `gorm:"column:gif_pipeline_short_video_mode;size:16"`
	GIFPipelineDefaultMode                            string    `gorm:"column:gif_pipeline_default_mode;size:16"`
	GIFPipelineLongVideoMode                          string    `gorm:"column:gif_pipeline_long_video_mode;size:16"`
	GIFPipelineHighPriorityEnabled                    bool      `gorm:"column:gif_pipeline_high_priority_enabled"`
	GIFPipelineHighPriorityMode                       string    `gorm:"column:gif_pipeline_high_priority_mode;size:16"`
	GIFDurationTierMediumSec                          float64   `gorm:"column:gif_duration_tier_medium_sec"`
	GIFDurationTierLongSec                            float64   `gorm:"column:gif_duration_tier_long_sec"`
	GIFDurationTierUltraSec                           float64   `gorm:"column:gif_duration_tier_ultra_sec"`
	GIFSegmentTimeoutMinSec                           int       `gorm:"column:gif_segment_timeout_min_sec"`
	GIFSegmentTimeoutMaxSec                           int       `gorm:"column:gif_segment_timeout_max_sec"`
	GIFSegmentTimeoutFallbackCapSec                   int       `gorm:"column:gif_segment_timeout_fallback_cap_sec"`
	GIFSegmentTimeoutEmergencyCapSec                  int       `gorm:"column:gif_segment_timeout_emergency_cap_sec"`
	GIFSegmentTimeoutLastResortCapSec                 int       `gorm:"column:gif_segment_timeout_last_resort_cap_sec"`
	GIFRenderRetryMaxAttempts                         int       `gorm:"column:gif_render_retry_max_attempts"`
	GIFRenderRetryPrimaryColorsFloor                  int       `gorm:"column:gif_render_retry_primary_colors_floor"`
	GIFRenderRetryPrimaryColorsStep                   int       `gorm:"column:gif_render_retry_primary_colors_step"`
	GIFRenderRetryFPSFloor                            int       `gorm:"column:gif_render_retry_fps_floor"`
	GIFRenderRetryFPSStep                             int       `gorm:"column:gif_render_retry_fps_step"`
	GIFRenderRetryWidthTrigger                        int       `gorm:"column:gif_render_retry_width_trigger"`
	GIFRenderRetryWidthScale                          float64   `gorm:"column:gif_render_retry_width_scale"`
	GIFRenderRetryWidthFloor                          int       `gorm:"column:gif_render_retry_width_floor"`
	GIFRenderRetrySecondaryColorsFloor                int       `gorm:"column:gif_render_retry_secondary_colors_floor"`
	GIFRenderRetrySecondaryColorsStep                 int       `gorm:"column:gif_render_retry_secondary_colors_step"`
	GIFRenderInitialSizeFPSCap                        int       `gorm:"column:gif_render_initial_size_fps_cap"`
	GIFRenderInitialClarityFPSFloor                   int       `gorm:"column:gif_render_initial_clarity_fps_floor"`
	GIFRenderInitialSizeColorsCap                     int       `gorm:"column:gif_render_initial_size_colors_cap"`
	GIFRenderInitialClarityColorsFloor                int       `gorm:"column:gif_render_initial_clarity_colors_floor"`
	GIFMotionLowScoreThreshold                        float64   `gorm:"column:gif_motion_low_score_threshold"`
	GIFMotionHighScoreThreshold                       float64   `gorm:"column:gif_motion_high_score_threshold"`
	GIFMotionLowFPSDelta                              int       `gorm:"column:gif_motion_low_fps_delta"`
	GIFMotionHighFPSDelta                             int       `gorm:"column:gif_motion_high_fps_delta"`
	GIFAdaptiveFPSMin                                 int       `gorm:"column:gif_adaptive_fps_min"`
	GIFAdaptiveFPSMax                                 int       `gorm:"column:gif_adaptive_fps_max"`
	GIFWidthSizeLow                                   int       `gorm:"column:gif_width_size_low"`
	GIFWidthSizeMedium                                int       `gorm:"column:gif_width_size_medium"`
	GIFWidthSizeHigh                                  int       `gorm:"column:gif_width_size_high"`
	GIFWidthClarityLow                                int       `gorm:"column:gif_width_clarity_low"`
	GIFWidthClarityMedium                             int       `gorm:"column:gif_width_clarity_medium"`
	GIFWidthClarityHigh                               int       `gorm:"column:gif_width_clarity_high"`
	GIFColorsSizeLow                                  int       `gorm:"column:gif_colors_size_low"`
	GIFColorsSizeMedium                               int       `gorm:"column:gif_colors_size_medium"`
	GIFColorsSizeHigh                                 int       `gorm:"column:gif_colors_size_high"`
	GIFColorsClarityLow                               int       `gorm:"column:gif_colors_clarity_low"`
	GIFColorsClarityMedium                            int       `gorm:"column:gif_colors_clarity_medium"`
	GIFColorsClarityHigh                              int       `gorm:"column:gif_colors_clarity_high"`
	GIFDurationLowSec                                 float64   `gorm:"column:gif_duration_low_sec"`
	GIFDurationMediumSec                              float64   `gorm:"column:gif_duration_medium_sec"`
	GIFDurationHighSec                                float64   `gorm:"column:gif_duration_high_sec"`
	GIFDurationSizeProfileMaxSec                      float64   `gorm:"column:gif_duration_size_profile_max_sec"`
	GIFDownshiftHighResLongSideThreshold              int       `gorm:"column:gif_downshift_high_res_long_side_threshold"`
	GIFDownshiftEarlyDurationSec                      float64   `gorm:"column:gif_downshift_early_duration_sec"`
	GIFDownshiftEarlyLongSideThreshold                int       `gorm:"column:gif_downshift_early_long_side_threshold"`
	GIFDownshiftMediumFPSCap                          int       `gorm:"column:gif_downshift_medium_fps_cap"`
	GIFDownshiftMediumWidthCap                        int       `gorm:"column:gif_downshift_medium_width_cap"`
	GIFDownshiftMediumColorsCap                       int       `gorm:"column:gif_downshift_medium_colors_cap"`
	GIFDownshiftMediumDurationCapSec                  float64   `gorm:"column:gif_downshift_medium_duration_cap_sec"`
	GIFDownshiftLongFPSCap                            int       `gorm:"column:gif_downshift_long_fps_cap"`
	GIFDownshiftLongWidthCap                          int       `gorm:"column:gif_downshift_long_width_cap"`
	GIFDownshiftLongColorsCap                         int       `gorm:"column:gif_downshift_long_colors_cap"`
	GIFDownshiftLongDurationCapSec                    float64   `gorm:"column:gif_downshift_long_duration_cap_sec"`
	GIFDownshiftUltraFPSCap                           int       `gorm:"column:gif_downshift_ultra_fps_cap"`
	GIFDownshiftUltraWidthCap                         int       `gorm:"column:gif_downshift_ultra_width_cap"`
	GIFDownshiftUltraColorsCap                        int       `gorm:"column:gif_downshift_ultra_colors_cap"`
	GIFDownshiftUltraDurationCapSec                   float64   `gorm:"column:gif_downshift_ultra_duration_cap_sec"`
	GIFDownshiftHighResFPSCap                         int       `gorm:"column:gif_downshift_high_res_fps_cap"`
	GIFDownshiftHighResWidthCap                       int       `gorm:"column:gif_downshift_high_res_width_cap"`
	GIFDownshiftHighResColorsCap                      int       `gorm:"column:gif_downshift_high_res_colors_cap"`
	GIFDownshiftHighResDurationCapSec                 float64   `gorm:"column:gif_downshift_high_res_duration_cap_sec"`
	GIFTimeoutFallbackFPSCap                          int       `gorm:"column:gif_timeout_fallback_fps_cap"`
	GIFTimeoutFallbackWidthCap                        int       `gorm:"column:gif_timeout_fallback_width_cap"`
	GIFTimeoutFallbackColorsCap                       int       `gorm:"column:gif_timeout_fallback_colors_cap"`
	GIFTimeoutFallbackMinWidth                        int       `gorm:"column:gif_timeout_fallback_min_width"`
	GIFTimeoutFallbackUltraFPSCap                     int       `gorm:"column:gif_timeout_fallback_ultra_fps_cap"`
	GIFTimeoutFallbackUltraWidthCap                   int       `gorm:"column:gif_timeout_fallback_ultra_width_cap"`
	GIFTimeoutFallbackUltraColorsCap                  int       `gorm:"column:gif_timeout_fallback_ultra_colors_cap"`
	GIFTimeoutEmergencyFPSCap                         int       `gorm:"column:gif_timeout_emergency_fps_cap"`
	GIFTimeoutEmergencyWidthCap                       int       `gorm:"column:gif_timeout_emergency_width_cap"`
	GIFTimeoutEmergencyColorsCap                      int       `gorm:"column:gif_timeout_emergency_colors_cap"`
	GIFTimeoutEmergencyMinWidth                       int       `gorm:"column:gif_timeout_emergency_min_width"`
	GIFTimeoutEmergencyDurationTrigger                float64   `gorm:"column:gif_timeout_emergency_duration_trigger_sec"`
	GIFTimeoutEmergencyDurationScale                  float64   `gorm:"column:gif_timeout_emergency_duration_scale"`
	GIFTimeoutEmergencyDurationMinSec                 float64   `gorm:"column:gif_timeout_emergency_duration_min_sec"`
	GIFTimeoutLastResortFPSCap                        int       `gorm:"column:gif_timeout_last_resort_fps_cap"`
	GIFTimeoutLastResortWidthCap                      int       `gorm:"column:gif_timeout_last_resort_width_cap"`
	GIFTimeoutLastResortColorsCap                     int       `gorm:"column:gif_timeout_last_resort_colors_cap"`
	GIFTimeoutLastResortMinWidth                      int       `gorm:"column:gif_timeout_last_resort_min_width"`
	GIFTimeoutLastResortDurationMinSec                float64   `gorm:"column:gif_timeout_last_resort_duration_min_sec"`
	GIFTimeoutLastResortDurationMaxSec                float64   `gorm:"column:gif_timeout_last_resort_duration_max_sec"`
	WebPTargetSizeKB                                  int       `gorm:"column:webp_target_size_kb"`
	JPGTargetSizeKB                                   int       `gorm:"column:jpg_target_size_kb"`
	PNGTargetSizeKB                                   int       `gorm:"column:png_target_size_kb"`
	StillMinBlurScore                                 float64   `gorm:"column:still_min_blur_score"`
	StillMinExposureScore                             float64   `gorm:"column:still_min_exposure_score"`
	StillMinWidth                                     int       `gorm:"column:still_min_width"`
	StillMinHeight                                    int       `gorm:"column:still_min_height"`
	LiveCoverPortraitWeight                           float64   `gorm:"column:live_cover_portrait_weight"`
	LiveCoverSceneMinSamples                          int       `gorm:"column:live_cover_scene_min_samples"`
	LiveCoverGuardMinTotal                            int       `gorm:"column:live_cover_guard_min_total"`
	LiveCoverGuardScoreFloor                          float64   `gorm:"column:live_cover_guard_score_floor"`
	HighlightFeedbackEnabled                          bool      `gorm:"column:highlight_feedback_enabled"`
	HighlightFeedbackRollout                          int       `gorm:"column:highlight_feedback_rollout_percent"`
	HighlightFeedbackMinJobs                          int       `gorm:"column:highlight_feedback_min_engaged_jobs"`
	HighlightFeedbackMinScore                         float64   `gorm:"column:highlight_feedback_min_weighted_signals"`
	HighlightFeedbackBoost                            float64   `gorm:"column:highlight_feedback_boost_scale"`
	HighlightWeightPosition                           float64   `gorm:"column:highlight_feedback_position_weight"`
	HighlightWeightDuration                           float64   `gorm:"column:highlight_feedback_duration_weight"`
	HighlightWeightReason                             float64   `gorm:"column:highlight_feedback_reason_weight"`
	HighlightNegativeGuardEnabled                     bool      `gorm:"column:highlight_feedback_negative_guard_enabled"`
	HighlightNegativeGuardThreshold                   float64   `gorm:"column:highlight_feedback_negative_guard_dominance_threshold"`
	HighlightNegativeGuardMinWeight                   float64   `gorm:"column:highlight_feedback_negative_guard_min_weight"`
	HighlightNegativePenaltyScale                     float64   `gorm:"column:highlight_feedback_negative_guard_penalty_scale"`
	HighlightNegativePenaltyWeight                    float64   `gorm:"column:highlight_feedback_negative_guard_penalty_weight"`
	GIFHealthDoneRateWarn                             float64   `gorm:"column:gif_health_done_rate_warn"`
	GIFHealthDoneRateCritical                         float64   `gorm:"column:gif_health_done_rate_critical"`
	GIFHealthFailedRateWarn                           float64   `gorm:"column:gif_health_failed_rate_warn"`
	GIFHealthFailedRateCritical                       float64   `gorm:"column:gif_health_failed_rate_critical"`
	GIFHealthPathStrictRateWarn                       float64   `gorm:"column:gif_health_path_strict_rate_warn"`
	GIFHealthPathStrictRateCritical                   float64   `gorm:"column:gif_health_path_strict_rate_critical"`
	GIFHealthLoopFallbackRateWarn                     float64   `gorm:"column:gif_health_loop_fallback_rate_warn"`
	GIFHealthLoopFallbackRateCritical                 float64   `gorm:"column:gif_health_loop_fallback_rate_critical"`
	FeedbackIntegrityOutputCoverageRateWarn           float64   `gorm:"column:feedback_integrity_output_coverage_rate_warn"`
	FeedbackIntegrityOutputCoverageRateCritical       float64   `gorm:"column:feedback_integrity_output_coverage_rate_critical"`
	FeedbackIntegrityOutputResolvedRateWarn           float64   `gorm:"column:feedback_integrity_output_resolved_rate_warn"`
	FeedbackIntegrityOutputResolvedRateCritical       float64   `gorm:"column:feedback_integrity_output_resolved_rate_critical"`
	FeedbackIntegrityOutputJobConsistencyRateWarn     float64   `gorm:"column:feedback_integrity_output_job_consistency_rate_warn"`
	FeedbackIntegrityOutputJobConsistencyRateCritical float64   `gorm:"column:feedback_integrity_output_job_consistency_rate_critical"`
	FeedbackIntegrityTopPickConflictUsersWarn         int       `gorm:"column:feedback_integrity_top_pick_conflict_users_warn"`
	FeedbackIntegrityTopPickConflictUsersCritical     int       `gorm:"column:feedback_integrity_top_pick_conflict_users_critical"`
	AIDirectorInputMode                               string    `gorm:"column:ai_director_input_mode;size:16"`
	AIDirectorOperatorInstruction                     string    `gorm:"column:ai_director_operator_instruction;type:text"`
	AIDirectorOperatorInstructionVersion              string    `gorm:"column:ai_director_operator_instruction_version;size:64"`
	AIDirectorOperatorEnabled                         bool      `gorm:"column:ai_director_operator_enabled"`
	AIDirectorConstraintOverrideEnabled               bool      `gorm:"column:ai_director_constraint_override_enabled"`
	AIDirectorCountExpandRatio                        float64   `gorm:"column:ai_director_count_expand_ratio"`
	AIDirectorDurationExpandRatio                     float64   `gorm:"column:ai_director_duration_expand_ratio"`
	AIDirectorCountAbsoluteCap                        int       `gorm:"column:ai_director_count_absolute_cap"`
	AIDirectorDurationAbsoluteCapSec                  float64   `gorm:"column:ai_director_duration_absolute_cap_sec"`
	CreatedAt                                         time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                                         time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoQualitySetting) TableName() string {
	return "ops.video_quality_settings"
}

type VideoAIPromptTemplate struct {
	ID                 uint64         `gorm:"primaryKey;autoIncrement"`
	Format             string         `gorm:"column:format;size:16;index"`
	Stage              string         `gorm:"column:stage;size:16;index"`
	Layer              string         `gorm:"column:layer;size:16;index"`
	TemplateText       string         `gorm:"column:template_text;type:text"`
	TemplateJSONSchema datatypes.JSON `gorm:"column:template_json_schema;type:jsonb"`
	Enabled            bool           `gorm:"column:enabled;index"`
	Version            string         `gorm:"column:version;size:64"`
	IsActive           bool           `gorm:"column:is_active;index"`
	CreatedBy          uint64         `gorm:"column:created_by"`
	UpdatedBy          uint64         `gorm:"column:updated_by"`
	Metadata           datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt          time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoAIPromptTemplate) TableName() string {
	return "ops.video_ai_prompt_templates"
}

type VideoAIPromptTemplateAudit struct {
	ID              uint64         `gorm:"primaryKey;autoIncrement"`
	TemplateID      *uint64        `gorm:"column:template_id;index"`
	Format          string         `gorm:"column:format;size:16;index"`
	Stage           string         `gorm:"column:stage;size:16;index"`
	Layer           string         `gorm:"column:layer;size:16;index"`
	Action          string         `gorm:"column:action;size:16;index"`
	OldValue        datatypes.JSON `gorm:"column:old_value;type:jsonb"`
	NewValue        datatypes.JSON `gorm:"column:new_value;type:jsonb"`
	Reason          string         `gorm:"column:reason;type:text"`
	OperatorAdminID uint64         `gorm:"column:operator_admin_id;index"`
	Metadata        datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime;index"`
}

func (VideoAIPromptTemplateAudit) TableName() string {
	return "audit.video_ai_prompt_template_audits"
}

type VideoQualityRolloutAudit struct {
	ID                   uint64         `gorm:"primaryKey;autoIncrement"`
	AdminID              uint64         `gorm:"column:admin_id;index"`
	FromRolloutPercent   int            `gorm:"column:from_rollout_percent"`
	ToRolloutPercent     int            `gorm:"column:to_rollout_percent"`
	Window               string         `gorm:"column:window_label;size:16"`
	ConfirmWindows       int            `gorm:"column:confirm_windows"`
	RecommendationState  string         `gorm:"column:recommendation_state;size:32;index"`
	RecommendationReason string         `gorm:"column:recommendation_reason;type:text"`
	ConsecutiveRequired  int            `gorm:"column:consecutive_required"`
	ConsecutiveMatched   int            `gorm:"column:consecutive_matched"`
	Metadata             datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt            time.Time      `gorm:"column:created_at;autoCreateTime;index"`
}

func (VideoQualityRolloutAudit) TableName() string {
	return "ops.video_quality_rollout_audits"
}
