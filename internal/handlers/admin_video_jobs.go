package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	defaultLiveCoverSceneGuardMaxRiskScenes = 3
)

type liveCoverSceneGuardConfig struct {
	MinSamples   int64
	MinTotal     int64
	ScoreFloor   float64
	MaxRiskScene int
}

type AdminVideoJobUser struct {
	ID                 uint64 `json:"id"`
	DisplayName        string `json:"display_name,omitempty"`
	Phone              string `json:"phone,omitempty"`
	UserLevel          string `json:"user_level,omitempty"`
	SubscriptionPlan   string `json:"subscription_plan,omitempty"`
	SubscriptionStatus string `json:"subscription_status,omitempty"`
}

type AdminVideoJobCollection struct {
	ID       uint64 `json:"id"`
	Title    string `json:"title"`
	CoverURL string `json:"cover_url,omitempty"`
	Status   string `json:"status,omitempty"`
	IsSample bool   `json:"is_sample"`
}

type AdminVideoJobCost struct {
	EstimatedCost      float64 `json:"estimated_cost"`
	Currency           string  `json:"currency,omitempty"`
	PricingVersion     string  `json:"pricing_version,omitempty"`
	CPUms              int64   `json:"cpu_ms"`
	GPUms              int64   `json:"gpu_ms"`
	StorageBytesRaw    int64   `json:"storage_bytes_raw"`
	StorageBytesOutput int64   `json:"storage_bytes_output"`
	OutputCount        int     `json:"output_count"`
	AICostUSD          float64 `json:"ai_cost_usd,omitempty"`
	AICostCNY          float64 `json:"ai_cost_cny,omitempty"`
	AICalls            int64   `json:"ai_calls,omitempty"`
	AIErrorCalls       int64   `json:"ai_error_calls,omitempty"`
	AIDurationMs       int64   `json:"ai_duration_ms,omitempty"`
	AIInputTokens      int64   `json:"ai_input_tokens,omitempty"`
	AIOutputTokens     int64   `json:"ai_output_tokens,omitempty"`
	AICachedInput      int64   `json:"ai_cached_input_tokens,omitempty"`
	AIImageTokens      int64   `json:"ai_image_tokens,omitempty"`
	AIVideoTokens      int64   `json:"ai_video_tokens,omitempty"`
	AIAudioSeconds     float64 `json:"ai_audio_seconds,omitempty"`
	USDtoCNYRate       float64 `json:"usd_to_cny_rate,omitempty"`
}

type AdminVideoJobPointHold struct {
	Status         string `json:"status"`
	ReservedPoints int64  `json:"reserved_points"`
	SettledPoints  int64  `json:"settled_points"`
}

type AdminVideoJobListItem struct {
	ID                 uint64                   `json:"id"`
	Title              string                   `json:"title"`
	SourceVideoKey     string                   `json:"source_video_key"`
	SourceVideoURL     string                   `json:"source_video_url,omitempty"`
	CategoryID         *uint64                  `json:"category_id,omitempty"`
	OutputFormats      []string                 `json:"output_formats"`
	Status             string                   `json:"status"`
	Stage              string                   `json:"stage"`
	Progress           int                      `json:"progress"`
	Priority           string                   `json:"priority"`
	ErrorMessage       string                   `json:"error_message,omitempty"`
	ResultCollectionID *uint64                  `json:"result_collection_id,omitempty"`
	Options            map[string]interface{}   `json:"options,omitempty"`
	Metrics            map[string]interface{}   `json:"metrics,omitempty"`
	Cost               *AdminVideoJobCost       `json:"cost,omitempty"`
	PointHold          *AdminVideoJobPointHold  `json:"point_hold,omitempty"`
	User               AdminVideoJobUser        `json:"user"`
	Collection         *AdminVideoJobCollection `json:"collection,omitempty"`
	QueuedAt           string                   `json:"queued_at,omitempty"`
	StartedAt          *string                  `json:"started_at,omitempty"`
	FinishedAt         *string                  `json:"finished_at,omitempty"`
	CreatedAt          string                   `json:"created_at"`
	UpdatedAt          string                   `json:"updated_at"`
}

type AdminVideoJobListResponse struct {
	Items    []AdminVideoJobListItem `json:"items"`
	Total    int64                   `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}

type AdminVideoJobEventItem struct {
	ID        uint64                 `json:"id"`
	Stage     string                 `json:"stage"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt string                 `json:"created_at"`
}

type AdminVideoJobArtifactItem struct {
	ID         uint64                 `json:"id"`
	Type       string                 `json:"type"`
	QiniuKey   string                 `json:"qiniu_key"`
	URL        string                 `json:"url,omitempty"`
	MimeType   string                 `json:"mime_type"`
	SizeBytes  int64                  `json:"size_bytes"`
	Width      int                    `json:"width"`
	Height     int                    `json:"height"`
	DurationMs int                    `json:"duration_ms"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  string                 `json:"created_at"`
}

type AdminVideoJobGIFCandidateItem struct {
	ID              uint64                 `json:"id"`
	StartMs         int                    `json:"start_ms"`
	EndMs           int                    `json:"end_ms"`
	DurationMs      int                    `json:"duration_ms"`
	BaseScore       float64                `json:"base_score"`
	ConfidenceScore float64                `json:"confidence_score"`
	FinalRank       int                    `json:"final_rank"`
	IsSelected      bool                   `json:"is_selected"`
	RejectReason    string                 `json:"reject_reason,omitempty"`
	FeatureJSON     map[string]interface{} `json:"feature_json,omitempty"`
	CreatedAt       string                 `json:"created_at"`
}

type AdminVideoJobDetailResponse struct {
	Job                     AdminVideoJobListItem             `json:"job"`
	Events                  []AdminVideoJobEventItem          `json:"events"`
	Artifacts               []AdminVideoJobArtifactItem       `json:"artifacts"`
	GIFCandidates           []AdminVideoJobGIFCandidateItem   `json:"gif_candidates,omitempty"`
	AIUsages                []AdminVideoJobAIUsageItem        `json:"ai_usages,omitempty"`
	AIGIFDirectives         []AdminVideoJobAIGIFDirectiveItem `json:"ai_gif_directives,omitempty"`
	AIGIFProposals          []AdminVideoJobAIGIFProposalItem  `json:"ai_gif_proposals,omitempty"`
	AIGIFReviews            []AdminVideoJobAIGIFReviewItem    `json:"ai_gif_reviews,omitempty"`
	AIGIFReviewStatusCounts map[string]int64                  `json:"ai_gif_review_status_counts,omitempty"`
	AIGIFReviewStatusFilter []string                          `json:"ai_gif_review_status_filter,omitempty"`
}

type AdminVideoJobAIUsageItem struct {
	ID                uint64                 `json:"id"`
	Stage             string                 `json:"stage"`
	Provider          string                 `json:"provider"`
	Model             string                 `json:"model"`
	Endpoint          string                 `json:"endpoint,omitempty"`
	RequestStatus     string                 `json:"request_status"`
	RequestError      string                 `json:"request_error,omitempty"`
	RequestDurationMs int64                  `json:"request_duration_ms"`
	InputTokens       int64                  `json:"input_tokens"`
	OutputTokens      int64                  `json:"output_tokens"`
	CachedInputTokens int64                  `json:"cached_input_tokens"`
	ImageTokens       int64                  `json:"image_tokens"`
	VideoTokens       int64                  `json:"video_tokens"`
	AudioSeconds      float64                `json:"audio_seconds"`
	CostUSD           float64                `json:"cost_usd"`
	Currency          string                 `json:"currency,omitempty"`
	PricingVersion    string                 `json:"pricing_version,omitempty"`
	PricingSourceURL  string                 `json:"pricing_source_url,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt         string                 `json:"created_at"`
}

type AdminVideoJobAIGIFProposalItem struct {
	ID                   uint64                 `json:"id"`
	ProposalRank         int                    `json:"proposal_rank"`
	StartSec             float64                `json:"start_sec"`
	EndSec               float64                `json:"end_sec"`
	DurationSec          float64                `json:"duration_sec"`
	BaseScore            float64                `json:"base_score"`
	ProposalReason       string                 `json:"proposal_reason,omitempty"`
	SemanticTags         []string               `json:"semantic_tags,omitempty"`
	ExpectedValueLevel   string                 `json:"expected_value_level,omitempty"`
	StandaloneConfidence float64                `json:"standalone_confidence"`
	LoopFriendlinessHint float64                `json:"loop_friendliness_hint"`
	Status               string                 `json:"status,omitempty"`
	Provider             string                 `json:"provider,omitempty"`
	Model                string                 `json:"model,omitempty"`
	PromptVersion        string                 `json:"prompt_version,omitempty"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt            string                 `json:"created_at"`
}

type AdminVideoJobAIGIFReviewItem struct {
	ID                  uint64                 `json:"id"`
	OutputID            *uint64                `json:"output_id,omitempty"`
	ProposalID          *uint64                `json:"proposal_id,omitempty"`
	FinalRecommendation string                 `json:"final_recommendation"`
	SemanticVerdict     float64                `json:"semantic_verdict"`
	DiagnosticReason    string                 `json:"diagnostic_reason,omitempty"`
	SuggestedAction     string                 `json:"suggested_action,omitempty"`
	Provider            string                 `json:"provider,omitempty"`
	Model               string                 `json:"model,omitempty"`
	PromptVersion       string                 `json:"prompt_version,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt           string                 `json:"created_at"`
}

type AdminVideoJobAIGIFDirectiveItem struct {
	ID                 uint64                 `json:"id"`
	BusinessGoal       string                 `json:"business_goal,omitempty"`
	Audience           string                 `json:"audience,omitempty"`
	MustCapture        []string               `json:"must_capture,omitempty"`
	Avoid              []string               `json:"avoid,omitempty"`
	ClipCountMin       int                    `json:"clip_count_min"`
	ClipCountMax       int                    `json:"clip_count_max"`
	DurationPrefMinSec float64                `json:"duration_pref_min_sec"`
	DurationPrefMaxSec float64                `json:"duration_pref_max_sec"`
	LoopPreference     float64                `json:"loop_preference"`
	StyleDirection     string                 `json:"style_direction,omitempty"`
	RiskFlags          []string               `json:"risk_flags,omitempty"`
	QualityWeights     map[string]interface{} `json:"quality_weights,omitempty"`
	BriefVersion       string                 `json:"brief_version,omitempty"`
	ModelVersion       string                 `json:"model_version,omitempty"`
	DirectiveText      string                 `json:"directive_text,omitempty"`
	Status             string                 `json:"status,omitempty"`
	FallbackUsed       bool                   `json:"fallback_used"`
	InputContext       map[string]interface{} `json:"input_context,omitempty"`
	Provider           string                 `json:"provider,omitempty"`
	Model              string                 `json:"model,omitempty"`
	PromptVersion      string                 `json:"prompt_version,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt          string                 `json:"created_at"`
}

type AdminVideoJobSimpleCount struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type AdminVideoJobFailureReason struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

type AdminVideoJobStageDuration struct {
	FromStage  string  `json:"from_stage"`
	ToStage    string  `json:"to_stage"`
	Transition string  `json:"transition"`
	Count      int64   `json:"count"`
	AvgSec     float64 `json:"avg_sec"`
	P95Sec     float64 `json:"p95_sec"`
}

type AdminVideoJobFormatStat struct {
	Format               string  `json:"format"`
	RequestedJobs        int64   `json:"requested_jobs"`
	GeneratedJobs        int64   `json:"generated_jobs"`
	SuccessRate          float64 `json:"success_rate"`
	ArtifactCount        int64   `json:"artifact_count"`
	AvgArtifactSizeBytes float64 `json:"avg_artifact_size_bytes"`
	SizeProfileJobs      int64   `json:"size_profile_jobs"`
	SizeProfileRate      float64 `json:"size_profile_rate"`
	SizeProfileAvgBytes  float64 `json:"size_profile_avg_artifact_size_bytes"`
	SizeBudgetSamples    int64   `json:"size_budget_samples"`
	SizeBudgetHits       int64   `json:"size_budget_hits"`
	SizeBudgetHitRate    float64 `json:"size_budget_hit_rate"`
	EngagedJobs          int64   `json:"engaged_jobs"`
	FeedbackSignals      int64   `json:"feedback_signals"`
	AvgEngagementScore   float64 `json:"avg_engagement_score"`
}

type AdminVideoJobFeedbackSceneStat struct {
	SceneTag string `json:"scene_tag"`
	Signals  int64  `json:"signals"`
}

type AdminVideoJobFeedbackGroupStat struct {
	Group              string  `json:"group"`
	Jobs               int64   `json:"jobs"`
	EngagedJobs        int64   `json:"engaged_jobs"`
	FeedbackSignals    int64   `json:"feedback_signals"`
	AvgEngagementScore float64 `json:"avg_engagement_score"`
	AppliedJobs        int64   `json:"applied_jobs"`
}

type AdminVideoJobFeedbackGroupFormatStat struct {
	Format             string  `json:"format"`
	Group              string  `json:"group"`
	Jobs               int64   `json:"jobs"`
	EngagedJobs        int64   `json:"engaged_jobs"`
	FeedbackSignals    int64   `json:"feedback_signals"`
	AvgEngagementScore float64 `json:"avg_engagement_score"`
	AppliedJobs        int64   `json:"applied_jobs"`
}

type AdminVideoJobFeedbackActionStat struct {
	Action    string  `json:"action"`
	Count     int64   `json:"count"`
	Ratio     float64 `json:"ratio"`
	WeightSum float64 `json:"weight_sum"`
}

type AdminVideoJobFeedbackTrendPoint struct {
	Bucket   string `json:"bucket"`
	Total    int64  `json:"total"`
	Positive int64  `json:"positive"`
	Neutral  int64  `json:"neutral"`
	Negative int64  `json:"negative"`
	TopPick  int64  `json:"top_pick"`
}

type AdminVideoJobFeedbackNegativeGuardOverview struct {
	Samples            int64   `json:"samples"`
	TreatmentJobs      int64   `json:"treatment_jobs"`
	GuardEnabledJobs   int64   `json:"guard_enabled_jobs"`
	GuardReasonHitJobs int64   `json:"guard_reason_hit_jobs"`
	SelectionShiftJobs int64   `json:"selection_shift_jobs"`
	BlockedReasonJobs  int64   `json:"blocked_reason_jobs"`
	GuardHitRate       float64 `json:"guard_hit_rate"`
	SelectionShiftRate float64 `json:"selection_shift_rate"`
	BlockedReasonRate  float64 `json:"blocked_reason_rate"`
	AvgNegativeSignals float64 `json:"avg_negative_signals"`
	AvgPositiveSignals float64 `json:"avg_positive_signals"`
}

type AdminVideoJobFeedbackNegativeGuardReasonStat struct {
	Reason      string  `json:"reason"`
	Jobs        int64   `json:"jobs"`
	BlockedJobs int64   `json:"blocked_jobs"`
	AvgWeight   float64 `json:"avg_weight"`
}

type AdminVideoJobFeedbackNegativeGuardJobRow struct {
	JobID           uint64     `json:"job_id"`
	UserID          uint64     `json:"user_id"`
	Title           string     `json:"title"`
	Group           string     `json:"group"`
	GuardHit        bool       `json:"guard_hit"`
	BlockedReason   bool       `json:"blocked_reason"`
	BeforeReason    string     `json:"before_reason"`
	AfterReason     string     `json:"after_reason"`
	BeforeStartSec  *float64   `json:"before_start_sec,omitempty"`
	BeforeEndSec    *float64   `json:"before_end_sec,omitempty"`
	AfterStartSec   *float64   `json:"after_start_sec,omitempty"`
	AfterEndSec     *float64   `json:"after_end_sec,omitempty"`
	GuardReasonList string     `json:"guard_reason_list,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type AdminVideoJobLiveCoverSceneStat struct {
	SceneTag         string  `json:"scene_tag"`
	Samples          int64   `json:"samples"`
	AvgCoverScore    float64 `json:"avg_cover_score"`
	AvgCoverPortrait float64 `json:"avg_cover_portrait"`
	AvgCoverExposure float64 `json:"avg_cover_exposure"`
	AvgCoverFace     float64 `json:"avg_cover_face"`
	LowSample        bool    `json:"low_sample"`
}

type AdminVideoJobGIFLoopTuneOverview struct {
	Samples              int64   `json:"samples"`
	Applied              int64   `json:"applied"`
	EffectiveApplied     int64   `json:"effective_applied"`
	FallbackToBase       int64   `json:"fallback_to_base"`
	AppliedRate          float64 `json:"applied_rate"`
	EffectiveAppliedRate float64 `json:"effective_applied_rate"`
	FallbackRate         float64 `json:"fallback_rate"`
	AvgScore             float64 `json:"avg_score"`
	AvgLoopClosure       float64 `json:"avg_loop_closure"`
	AvgMotionMean        float64 `json:"avg_motion_mean"`
	AvgEffectiveSec      float64 `json:"avg_effective_sec"`
}

type AdminVideoJobGIFEvaluationOverview struct {
	Samples            int64   `json:"samples"`
	AvgEmotionScore    float64 `json:"avg_emotion_score"`
	AvgClarityScore    float64 `json:"avg_clarity_score"`
	AvgMotionScore     float64 `json:"avg_motion_score"`
	AvgLoopScore       float64 `json:"avg_loop_score"`
	AvgEfficiencyScore float64 `json:"avg_efficiency_score"`
	AvgOverallScore    float64 `json:"avg_overall_score"`
}

type AdminVideoJobGIFBaselineSnapshot struct {
	BaselineDate       string  `json:"baseline_date"`
	WindowLabel        string  `json:"window_label"`
	Scope              string  `json:"scope"`
	SampleJobs         int64   `json:"sample_jobs"`
	DoneJobs           int64   `json:"done_jobs"`
	FailedJobs         int64   `json:"failed_jobs"`
	DoneRate           float64 `json:"done_rate"`
	FailedRate         float64 `json:"failed_rate"`
	SampleOutputs      int64   `json:"sample_outputs"`
	AvgEmotionScore    float64 `json:"avg_emotion_score"`
	AvgClarityScore    float64 `json:"avg_clarity_score"`
	AvgMotionScore     float64 `json:"avg_motion_score"`
	AvgLoopScore       float64 `json:"avg_loop_score"`
	AvgEfficiencyScore float64 `json:"avg_efficiency_score"`
	AvgOverallScore    float64 `json:"avg_overall_score"`
}

type AdminVideoJobGIFEvaluationSample struct {
	JobID           uint64  `json:"job_id"`
	OutputID        uint64  `json:"output_id"`
	PreviewURL      string  `json:"preview_url,omitempty"`
	ObjectKey       string  `json:"object_key,omitempty"`
	WindowStartMs   int     `json:"window_start_ms"`
	WindowEndMs     int     `json:"window_end_ms"`
	OverallScore    float64 `json:"overall_score"`
	EmotionScore    float64 `json:"emotion_score"`
	ClarityScore    float64 `json:"clarity_score"`
	MotionScore     float64 `json:"motion_score"`
	LoopScore       float64 `json:"loop_score"`
	EfficiencyScore float64 `json:"efficiency_score"`
	CandidateReason string  `json:"candidate_reason,omitempty"`
	SizeBytes       int64   `json:"size_bytes"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	DurationMs      int     `json:"duration_ms"`
	CreatedAt       string  `json:"created_at"`
}

type AdminVideoJobGIFManualScoreOverview struct {
	Samples             int64   `json:"samples"`
	WithOutputID        int64   `json:"with_output_id"`
	MatchedEvaluations  int64   `json:"matched_evaluations"`
	MatchedRate         float64 `json:"matched_rate"`
	TopPickRate         float64 `json:"top_pick_rate"`
	PassRate            float64 `json:"pass_rate"`
	AvgManualEmotion    float64 `json:"avg_manual_emotion"`
	AvgManualClarity    float64 `json:"avg_manual_clarity"`
	AvgManualMotion     float64 `json:"avg_manual_motion"`
	AvgManualLoop       float64 `json:"avg_manual_loop"`
	AvgManualEfficiency float64 `json:"avg_manual_efficiency"`
	AvgManualOverall    float64 `json:"avg_manual_overall"`
	AvgAutoEmotion      float64 `json:"avg_auto_emotion"`
	AvgAutoClarity      float64 `json:"avg_auto_clarity"`
	AvgAutoMotion       float64 `json:"avg_auto_motion"`
	AvgAutoLoop         float64 `json:"avg_auto_loop"`
	AvgAutoEfficiency   float64 `json:"avg_auto_efficiency"`
	AvgAutoOverall      float64 `json:"avg_auto_overall"`
	MAEEmotion          float64 `json:"mae_emotion"`
	MAEClarity          float64 `json:"mae_clarity"`
	MAEMotion           float64 `json:"mae_motion"`
	MAELoop             float64 `json:"mae_loop"`
	MAEEfficiency       float64 `json:"mae_efficiency"`
	MAEOverall          float64 `json:"mae_overall"`
	AvgOverallDelta     float64 `json:"avg_overall_delta"`
}

type AdminVideoJobGIFManualScoreDiffSample struct {
	SampleID            string  `json:"sample_id"`
	BaselineVersion     string  `json:"baseline_version"`
	ReviewRound         string  `json:"review_round"`
	Reviewer            string  `json:"reviewer"`
	JobID               uint64  `json:"job_id"`
	OutputID            uint64  `json:"output_id"`
	PreviewURL          string  `json:"preview_url,omitempty"`
	ObjectKey           string  `json:"object_key,omitempty"`
	ManualOverallScore  float64 `json:"manual_overall_score"`
	AutoOverallScore    float64 `json:"auto_overall_score"`
	OverallScoreDelta   float64 `json:"overall_score_delta"`
	AbsOverallScoreDiff float64 `json:"abs_overall_score_diff"`
	ManualLoopScore     float64 `json:"manual_loop_score"`
	AutoLoopScore       float64 `json:"auto_loop_score"`
	LoopScoreDelta      float64 `json:"loop_score_delta"`
	ManualClarityScore  float64 `json:"manual_clarity_score"`
	AutoClarityScore    float64 `json:"auto_clarity_score"`
	ClarityScoreDelta   float64 `json:"clarity_score_delta"`
	IsTopPick           bool    `json:"is_top_pick"`
	IsPass              bool    `json:"is_pass"`
	RejectReason        string  `json:"reject_reason,omitempty"`
	ReviewedAt          string  `json:"reviewed_at"`
}

type AdminVideoJobFeedbackRolloutRecommendation struct {
	State                   string   `json:"state"`
	Reason                  string   `json:"reason"`
	CurrentRolloutPercent   int      `json:"current_rollout_percent"`
	SuggestedRolloutPercent int      `json:"suggested_rollout_percent"`
	ConsecutiveRequired     int      `json:"consecutive_required"`
	ConsecutiveMatched      int      `json:"consecutive_matched"`
	ConsecutivePassed       bool     `json:"consecutive_passed"`
	RecentStates            []string `json:"recent_states,omitempty"`
	TreatmentJobs           int64    `json:"treatment_jobs"`
	ControlJobs             int64    `json:"control_jobs"`
	TreatmentSignalsPerJob  float64  `json:"treatment_signals_per_job"`
	ControlSignalsPerJob    float64  `json:"control_signals_per_job"`
	SignalsUplift           float64  `json:"signals_uplift"`
	TreatmentAvgScore       float64  `json:"treatment_avg_score"`
	ControlAvgScore         float64  `json:"control_avg_score"`
	ScoreUplift             float64  `json:"score_uplift"`
	LiveGuardTriggered      bool     `json:"live_guard_triggered"`
	LiveGuardMinSamples     int      `json:"live_guard_min_samples"`
	LiveGuardEligibleTotal  int64    `json:"live_guard_eligible_total"`
	LiveGuardScoreFloor     float64  `json:"live_guard_score_floor"`
	LiveGuardRiskScenes     []string `json:"live_guard_risk_scenes,omitempty"`
}

type AdminVideoJobFeedbackRolloutAudit struct {
	ID                   uint64 `json:"id"`
	AdminID              uint64 `json:"admin_id"`
	FromRolloutPercent   int    `json:"from_rollout_percent"`
	ToRolloutPercent     int    `json:"to_rollout_percent"`
	Window               string `json:"window"`
	ConfirmWindows       int    `json:"confirm_windows"`
	RecommendationState  string `json:"recommendation_state"`
	RecommendationReason string `json:"recommendation_reason"`
	CreatedAt            string `json:"created_at"`
}

type AdminVideoJobSourceProbeQualityStat struct {
	Bucket         string  `json:"bucket"`
	Jobs           int64   `json:"jobs"`
	DoneJobs       int64   `json:"done_jobs"`
	FailedJobs     int64   `json:"failed_jobs"`
	PendingJobs    int64   `json:"pending_jobs"`
	CancelledJobs  int64   `json:"cancelled_jobs"`
	TerminalJobs   int64   `json:"terminal_jobs"`
	SuccessRate    float64 `json:"success_rate"`
	FailureRate    float64 `json:"failure_rate"`
	DurationP50Sec float64 `json:"duration_p50_sec"`
	DurationP95Sec float64 `json:"duration_p95_sec"`
}

type videoImageFeedbackFilter struct {
	UserID      uint64
	Format      string
	GuardReason string
}

type AdminVideoJobFeedbackIntegrityOverview struct {
	Samples                  int64   `json:"samples"`
	WithOutputID             int64   `json:"with_output_id"`
	MissingOutputID          int64   `json:"missing_output_id"`
	ResolvedOutput           int64   `json:"resolved_output"`
	OrphanOutput             int64   `json:"orphan_output"`
	JobMismatch              int64   `json:"job_mismatch"`
	TopPickMultiHitUsers     int64   `json:"top_pick_multi_hit_users"`
	OutputCoverageRate       float64 `json:"output_coverage_rate"`
	OutputResolvedRate       float64 `json:"output_resolved_rate"`
	OutputJobConsistencyRate float64 `json:"output_job_consistency_rate"`
}

type AdminVideoJobFeedbackIntegrityAlert struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type AdminVideoJobFeedbackIntegrityDrilldownRow struct {
	JobID                  uint64 `json:"job_id"`
	UserID                 uint64 `json:"user_id"`
	Title                  string `json:"title"`
	Status                 string `json:"status"`
	Stage                  string `json:"stage"`
	AnomalyCount           int64  `json:"anomaly_count,omitempty"`
	TopPickConflictUsers   int64  `json:"top_pick_conflict_users,omitempty"`
	TopPickConflictActions int64  `json:"top_pick_conflict_actions,omitempty"`
	LatestFeedbackAt       string `json:"latest_feedback_at,omitempty"`
}

type AdminVideoJobFeedbackIntegrityDrilldownResponse struct {
	Window              string                                       `json:"window"`
	WindowStart         string                                       `json:"window_start,omitempty"`
	WindowEnd           string                                       `json:"window_end,omitempty"`
	FilterUserID        uint64                                       `json:"filter_user_id,omitempty"`
	FilterFormat        string                                       `json:"filter_format,omitempty"`
	FilterGuardReason   string                                       `json:"filter_guard_reason,omitempty"`
	AnomalyJobs         []AdminVideoJobFeedbackIntegrityDrilldownRow `json:"anomaly_jobs"`
	TopPickConflictJobs []AdminVideoJobFeedbackIntegrityDrilldownRow `json:"top_pick_conflict_jobs"`
}

type AdminVideoJobFeedbackIntegrityHealthTrendPoint struct {
	Bucket                   string   `json:"bucket"`
	Samples                  int64    `json:"samples"`
	Health                   string   `json:"health"`
	AlertCount               int      `json:"alert_count"`
	OutputCoverageRate       float64  `json:"output_coverage_rate"`
	OutputResolvedRate       float64  `json:"output_resolved_rate"`
	OutputJobConsistencyRate float64  `json:"output_job_consistency_rate"`
	TopPickMultiHitUsers     int64    `json:"top_pick_multi_hit_users"`
	AlertCodes               []string `json:"alert_codes,omitempty"`
}

type AdminVideoJobFeedbackIntegrityDelta struct {
	HasPreviousData               bool    `json:"has_previous_data"`
	PreviousWindowStart           string  `json:"previous_window_start,omitempty"`
	PreviousWindowEnd             string  `json:"previous_window_end,omitempty"`
	PreviousHealth                string  `json:"previous_health,omitempty"`
	CurrentHealth                 string  `json:"current_health,omitempty"`
	PreviousSamples               int64   `json:"previous_samples"`
	CurrentSamples                int64   `json:"current_samples"`
	SamplesDelta                  int64   `json:"samples_delta"`
	PreviousAlertCount            int     `json:"previous_alert_count"`
	CurrentAlertCount             int     `json:"current_alert_count"`
	AlertCountDelta               int     `json:"alert_count_delta"`
	OutputCoverageRateDelta       float64 `json:"output_coverage_rate_delta"`
	OutputResolvedRateDelta       float64 `json:"output_resolved_rate_delta"`
	OutputJobConsistencyRateDelta float64 `json:"output_job_consistency_rate_delta"`
	TopPickMultiHitUsersDelta     int64   `json:"top_pick_multi_hit_users_delta"`
}

type AdminVideoJobFeedbackIntegrityStreaks struct {
	ConsecutiveRedDays      int    `json:"consecutive_red_days"`
	ConsecutiveNonGreenDays int    `json:"consecutive_non_green_days"`
	ConsecutiveGreenDays    int    `json:"consecutive_green_days"`
	Recent7dRedDays         int    `json:"recent_7d_red_days"`
	Recent7dNonGreenDays    int    `json:"recent_7d_non_green_days"`
	LastNonGreenBucket      string `json:"last_non_green_bucket,omitempty"`
	LastRedBucket           string `json:"last_red_bucket,omitempty"`
}

type AdminVideoJobFeedbackIntegrityEscalation struct {
	Required       bool     `json:"required"`
	Level          string   `json:"level"`
	Reason         string   `json:"reason,omitempty"`
	TriggeredRules []string `json:"triggered_rules,omitempty"`
}

type AdminVideoJobFeedbackIntegrityEscalationTrendPoint struct {
	Bucket                    string   `json:"bucket"`
	Health                    string   `json:"health"`
	RecoveryStatus            string   `json:"recovery_status,omitempty"`
	AlertCount                int      `json:"alert_count"`
	AlertCountDelta           int      `json:"alert_count_delta"`
	TopPickMultiHitUsers      int64    `json:"top_pick_multi_hit_users"`
	TopPickMultiHitUsersDelta int64    `json:"top_pick_multi_hit_users_delta"`
	EscalationLevel           string   `json:"escalation_level"`
	EscalationRequired        bool     `json:"escalation_required"`
	EscalationReason          string   `json:"escalation_reason,omitempty"`
	TriggeredRules            []string `json:"triggered_rules,omitempty"`
}

type AdminVideoJobFeedbackIntegrityEscalationStats struct {
	TotalDays      int    `json:"total_days"`
	RequiredDays   int    `json:"required_days"`
	OncallDays     int    `json:"oncall_days"`
	WatchDays      int    `json:"watch_days"`
	NoticeDays     int    `json:"notice_days"`
	NoneDays       int    `json:"none_days"`
	LatestBucket   string `json:"latest_bucket,omitempty"`
	LatestLevel    string `json:"latest_level,omitempty"`
	LatestRequired bool   `json:"latest_required"`
	LatestReason   string `json:"latest_reason,omitempty"`
}

type AdminVideoJobFeedbackIntegrityEscalationIncident struct {
	Bucket                    string   `json:"bucket"`
	EscalationLevel           string   `json:"escalation_level"`
	EscalationRequired        bool     `json:"escalation_required"`
	EscalationReason          string   `json:"escalation_reason,omitempty"`
	TriggeredRules            []string `json:"triggered_rules,omitempty"`
	AlertCount                int      `json:"alert_count"`
	AlertCountDelta           int      `json:"alert_count_delta"`
	TopPickMultiHitUsers      int64    `json:"top_pick_multi_hit_users"`
	TopPickMultiHitUsersDelta int64    `json:"top_pick_multi_hit_users_delta"`
	RecoveryStatus            string   `json:"recovery_status,omitempty"`
}

type AdminVideoJobFeedbackIntegrityAlertCodeStat struct {
	Code         string `json:"code"`
	DaysHit      int    `json:"days_hit"`
	LatestLevel  string `json:"latest_level,omitempty"`
	LatestBucket string `json:"latest_bucket,omitempty"`
}

type AdminVideoJobFeedbackIntegrityRecommendation struct {
	Category        string   `json:"category"`
	Severity        string   `json:"severity"`
	Title           string   `json:"title"`
	Message         string   `json:"message"`
	SuggestedQuick  string   `json:"suggested_quick,omitempty"`
	SuggestedAction string   `json:"suggested_action,omitempty"`
	AlertCodes      []string `json:"alert_codes,omitempty"`
}

type AdminVideoJobFeedbackLearningChainStatus struct {
	LearningMode                  string `json:"learning_mode"`
	LegacyFeedbackFallbackEnabled bool   `json:"legacy_feedback_fallback_enabled"`
	LegacyEvalBackfillCandidates  int64  `json:"legacy_eval_backfill_candidates"`
}

type AdminVideoJobFeedbackIntegrityOverviewResponse struct {
	Window                               string                                               `json:"window"`
	WindowStart                          string                                               `json:"window_start,omitempty"`
	WindowEnd                            string                                               `json:"window_end,omitempty"`
	FilterUserID                         uint64                                               `json:"filter_user_id,omitempty"`
	FilterFormat                         string                                               `json:"filter_format,omitempty"`
	FilterGuardReason                    string                                               `json:"filter_guard_reason,omitempty"`
	FeedbackLearningChain                AdminVideoJobFeedbackLearningChainStatus             `json:"feedback_learning_chain"`
	FeedbackIntegrityOverview            AdminVideoJobFeedbackIntegrityOverview               `json:"feedback_integrity_overview"`
	FeedbackIntegrityAlertThresholds     FeedbackIntegrityAlertThresholdSettings              `json:"feedback_integrity_alert_thresholds"`
	FeedbackIntegrityHealth              string                                               `json:"feedback_integrity_health"`
	FeedbackIntegrityAlerts              []AdminVideoJobFeedbackIntegrityAlert                `json:"feedback_integrity_alerts"`
	FeedbackIntegrityHealthTrend         []AdminVideoJobFeedbackIntegrityHealthTrendPoint     `json:"feedback_integrity_health_trend"`
	FeedbackIntegrityAlertCodeStats      []AdminVideoJobFeedbackIntegrityAlertCodeStat        `json:"feedback_integrity_alert_code_stats"`
	FeedbackIntegrityDelta               *AdminVideoJobFeedbackIntegrityDelta                 `json:"feedback_integrity_delta,omitempty"`
	FeedbackIntegrityStreaks             AdminVideoJobFeedbackIntegrityStreaks                `json:"feedback_integrity_streaks"`
	FeedbackIntegrityEscalation          AdminVideoJobFeedbackIntegrityEscalation             `json:"feedback_integrity_escalation"`
	FeedbackIntegrityEscalationTrend     []AdminVideoJobFeedbackIntegrityEscalationTrendPoint `json:"feedback_integrity_escalation_trend"`
	FeedbackIntegrityEscalationStats     AdminVideoJobFeedbackIntegrityEscalationStats        `json:"feedback_integrity_escalation_stats"`
	FeedbackIntegrityEscalationIncidents []AdminVideoJobFeedbackIntegrityEscalationIncident   `json:"feedback_integrity_escalation_incidents"`
	FeedbackIntegrityRecoveryStatus      string                                               `json:"feedback_integrity_recovery_status"`
	FeedbackIntegrityRecovered           bool                                                 `json:"feedback_integrity_recovered"`
	FeedbackIntegrityPreviousHealth      string                                               `json:"feedback_integrity_previous_health,omitempty"`
	FeedbackIntegrityRecommendations     []AdminVideoJobFeedbackIntegrityRecommendation       `json:"feedback_integrity_recommendations"`
	FeedbackIntegrityAnomalyJobs         int64                                                `json:"feedback_integrity_anomaly_jobs_window"`
	FeedbackIntegrityTopPickJobs         int64                                                `json:"feedback_integrity_top_pick_conflict_jobs_window"`
	FeedbackActionStats                  []AdminVideoJobFeedbackActionStat                    `json:"feedback_action_stats"`
	FeedbackTopSceneTags                 []AdminVideoJobFeedbackSceneStat                     `json:"feedback_top_scene_tags"`
	FeedbackTrend                        []AdminVideoJobFeedbackTrendPoint                    `json:"feedback_trend"`
	FeedbackNegativeGuardOverview        AdminVideoJobFeedbackNegativeGuardOverview           `json:"feedback_negative_guard_overview"`
	FeedbackNegativeGuardReasons         []AdminVideoJobFeedbackNegativeGuardReasonStat       `json:"feedback_negative_guard_reasons"`
}

type AdminVideoJobOverviewResponse struct {
	Window                               string                                               `json:"window"`
	WindowStart                          string                                               `json:"window_start,omitempty"`
	WindowEnd                            string                                               `json:"window_end,omitempty"`
	Total                                int64                                                `json:"total"`
	Queued                               int64                                                `json:"queued"`
	Running                              int64                                                `json:"running"`
	Done                                 int64                                                `json:"done"`
	Failed                               int64                                                `json:"failed"`
	Cancelled                            int64                                                `json:"cancelled"`
	Retrying                             int64                                                `json:"retrying"`
	CreatedWindow                        int64                                                `json:"created_window"`
	DoneWindow                           int64                                                `json:"done_window"`
	FailedWindow                         int64                                                `json:"failed_window"`
	SourceProbeJobsWindow                int64                                                `json:"source_probe_jobs_window"`
	SourceProbeDurationBuckets           []AdminVideoJobSimpleCount                           `json:"source_probe_duration_buckets"`
	SourceProbeResolutionBuckets         []AdminVideoJobSimpleCount                           `json:"source_probe_resolution_buckets"`
	SourceProbeFpsBuckets                []AdminVideoJobSimpleCount                           `json:"source_probe_fps_buckets"`
	SourceProbeDurationQuality           []AdminVideoJobSourceProbeQualityStat                `json:"source_probe_duration_quality"`
	SourceProbeResolutionQuality         []AdminVideoJobSourceProbeQualityStat                `json:"source_probe_resolution_quality"`
	SourceProbeFpsQuality                []AdminVideoJobSourceProbeQualityStat                `json:"source_probe_fps_quality"`
	SampleJobsWindow                     int64                                                `json:"sample_jobs_window"`
	SampleDoneWindow                     int64                                                `json:"sample_done_window"`
	SampleFailedWindow                   int64                                                `json:"sample_failed_window"`
	SampleSuccessRateWindow              float64                                              `json:"sample_success_rate_window"`
	Created24h                           int64                                                `json:"created_24h"`
	Done24h                              int64                                                `json:"done_24h"`
	Failed24h                            int64                                                `json:"failed_24h"`
	DurationP50Sec                       float64                                              `json:"duration_p50_sec"`
	DurationP95Sec                       float64                                              `json:"duration_p95_sec"`
	SampleDurationP50Sec                 float64                                              `json:"sample_duration_p50_sec"`
	SampleDurationP95Sec                 float64                                              `json:"sample_duration_p95_sec"`
	CostWindow                           float64                                              `json:"cost_window"`
	CostAvgWindow                        float64                                              `json:"cost_avg_window"`
	CostTotal                            float64                                              `json:"cost_total"`
	Cost24h                              float64                                              `json:"cost_24h"`
	CostAvg24h                           float64                                              `json:"cost_avg_24h"`
	FeedbackSignalsWindow                int64                                                `json:"feedback_signals_window"`
	FeedbackDownloadsWindow              int64                                                `json:"feedback_downloads_window"`
	FeedbackFavoritesWindow              int64                                                `json:"feedback_favorites_window"`
	FeedbackEngagedJobsWindow            int64                                                `json:"feedback_engaged_jobs_window"`
	FeedbackAvgScoreWindow               float64                                              `json:"feedback_avg_score_window"`
	FeedbackIntegrityOverview            AdminVideoJobFeedbackIntegrityOverview               `json:"feedback_integrity_overview"`
	FeedbackIntegrityAlertThresholds     FeedbackIntegrityAlertThresholdSettings              `json:"feedback_integrity_alert_thresholds"`
	FeedbackIntegrityHealth              string                                               `json:"feedback_integrity_health"`
	FeedbackIntegrityAlerts              []AdminVideoJobFeedbackIntegrityAlert                `json:"feedback_integrity_alerts"`
	FeedbackIntegrityHealthTrend         []AdminVideoJobFeedbackIntegrityHealthTrendPoint     `json:"feedback_integrity_health_trend"`
	FeedbackIntegrityAlertCodeStats      []AdminVideoJobFeedbackIntegrityAlertCodeStat        `json:"feedback_integrity_alert_code_stats"`
	FeedbackIntegrityDelta               *AdminVideoJobFeedbackIntegrityDelta                 `json:"feedback_integrity_delta,omitempty"`
	FeedbackIntegrityStreaks             AdminVideoJobFeedbackIntegrityStreaks                `json:"feedback_integrity_streaks"`
	FeedbackIntegrityEscalation          AdminVideoJobFeedbackIntegrityEscalation             `json:"feedback_integrity_escalation"`
	FeedbackIntegrityEscalationTrend     []AdminVideoJobFeedbackIntegrityEscalationTrendPoint `json:"feedback_integrity_escalation_trend"`
	FeedbackIntegrityEscalationStats     AdminVideoJobFeedbackIntegrityEscalationStats        `json:"feedback_integrity_escalation_stats"`
	FeedbackIntegrityEscalationIncidents []AdminVideoJobFeedbackIntegrityEscalationIncident   `json:"feedback_integrity_escalation_incidents"`
	FeedbackIntegrityRecoveryStatus      string                                               `json:"feedback_integrity_recovery_status"`
	FeedbackIntegrityRecovered           bool                                                 `json:"feedback_integrity_recovered"`
	FeedbackIntegrityPreviousHealth      string                                               `json:"feedback_integrity_previous_health,omitempty"`
	FeedbackIntegrityRecommendations     []AdminVideoJobFeedbackIntegrityRecommendation       `json:"feedback_integrity_recommendations"`
	FeedbackIntegrityAnomalyJobs         int64                                                `json:"feedback_integrity_anomaly_jobs_window"`
	FeedbackIntegrityTopPickJobs         int64                                                `json:"feedback_integrity_top_pick_conflict_jobs_window"`
	LiveCoverSceneMinSamples             int64                                                `json:"live_cover_scene_min_samples"`
	LiveCoverSceneGuardMinTotal          int64                                                `json:"live_cover_scene_guard_min_total"`
	LiveCoverSceneGuardScoreFloor        float64                                              `json:"live_cover_scene_guard_score_floor"`
	FeedbackSceneStats                   []AdminVideoJobFeedbackSceneStat                     `json:"feedback_scene_stats"`
	FeedbackActionStats                  []AdminVideoJobFeedbackActionStat                    `json:"feedback_action_stats"`
	FeedbackTopSceneTags                 []AdminVideoJobFeedbackSceneStat                     `json:"feedback_top_scene_tags"`
	FeedbackTrend                        []AdminVideoJobFeedbackTrendPoint                    `json:"feedback_trend"`
	FeedbackNegativeGuardOverview        AdminVideoJobFeedbackNegativeGuardOverview           `json:"feedback_negative_guard_overview"`
	FeedbackNegativeGuardReasons         []AdminVideoJobFeedbackNegativeGuardReasonStat       `json:"feedback_negative_guard_reasons"`
	LiveCoverSceneStats                  []AdminVideoJobLiveCoverSceneStat                    `json:"live_cover_scene_stats"`
	GIFLoopTuneOverview                  AdminVideoJobGIFLoopTuneOverview                     `json:"gif_loop_tune_overview"`
	GIFEvaluationOverview                AdminVideoJobGIFEvaluationOverview                   `json:"gif_evaluation_overview"`
	GIFBaselineSnapshots                 []AdminVideoJobGIFBaselineSnapshot                   `json:"gif_baseline_snapshots"`
	GIFEvaluationTopSamples              []AdminVideoJobGIFEvaluationSample                   `json:"gif_evaluation_top_samples"`
	GIFEvaluationLowSamples              []AdminVideoJobGIFEvaluationSample                   `json:"gif_evaluation_low_samples"`
	GIFManualScoreOverview               AdminVideoJobGIFManualScoreOverview                  `json:"gif_manual_score_overview"`
	GIFManualScoreDiffSamples            []AdminVideoJobGIFManualScoreDiffSample              `json:"gif_manual_score_diff_samples"`
	FeedbackGroupStats                   []AdminVideoJobFeedbackGroupStat                     `json:"feedback_group_stats"`
	FeedbackGroupFormatStats             []AdminVideoJobFeedbackGroupFormatStat               `json:"feedback_group_format_stats"`
	FeedbackRolloutRecommendation        *AdminVideoJobFeedbackRolloutRecommendation          `json:"feedback_rollout_recommendation,omitempty"`
	FeedbackRolloutAuditLogs             []AdminVideoJobFeedbackRolloutAudit                  `json:"feedback_rollout_audit_logs"`
	StageCounts                          []AdminVideoJobSimpleCount                           `json:"stage_counts"`
	TopFailures                          []AdminVideoJobFailureReason                         `json:"top_failures"`
	StageDurations                       []AdminVideoJobStageDuration                         `json:"stage_durations"`
	FormatStats24h                       []AdminVideoJobFormatStat                            `json:"format_stats_24h"`
}

type adminSampleVideoJobsWindowSummary struct {
	JobsWindow   int64
	DoneWindow   int64
	FailedWindow int64
	SuccessRate  float64
	DurationP50  float64
	DurationP95  float64
}

type adminSampleVideoJobFormatBaselineRow struct {
	Format               string
	RequestedJobs        int64
	GeneratedJobs        int64
	SuccessRate          float64
	ArtifactCount        int64
	AvgArtifactSizeBytes float64
	DurationP50Sec       float64
	DurationP95Sec       float64
}

type adminSampleVideoJobFormatDiffRow struct {
	Format                     string
	BaseRequestedJobs          int64
	BaseGeneratedJobs          int64
	BaseSuccessRate            float64
	BaseAvgArtifactSizeBytes   float64
	BaseDurationP50Sec         float64
	BaseDurationP95Sec         float64
	TargetRequestedJobs        int64
	TargetGeneratedJobs        int64
	TargetSuccessRate          float64
	TargetAvgArtifactSizeBytes float64
	TargetDurationP50Sec       float64
	TargetDurationP95Sec       float64
	SuccessRateDelta           float64
	SuccessRateUplift          float64
	AvgArtifactSizeDelta       float64
	AvgArtifactSizeUplift      float64
	DurationP50Delta           float64
	DurationP50Uplift          float64
	DurationP95Delta           float64
	DurationP95Uplift          float64
}

type AdminSampleVideoJobsBaselineDiffSummary struct {
	BaseJobsWindow       int64   `json:"base_jobs_window"`
	TargetJobsWindow     int64   `json:"target_jobs_window"`
	JobsWindowDelta      float64 `json:"jobs_window_delta"`
	JobsWindowUplift     float64 `json:"jobs_window_uplift"`
	BaseDoneWindow       int64   `json:"base_done_window"`
	TargetDoneWindow     int64   `json:"target_done_window"`
	DoneWindowDelta      float64 `json:"done_window_delta"`
	DoneWindowUplift     float64 `json:"done_window_uplift"`
	BaseFailedWindow     int64   `json:"base_failed_window"`
	TargetFailedWindow   int64   `json:"target_failed_window"`
	FailedWindowDelta    float64 `json:"failed_window_delta"`
	FailedWindowUplift   float64 `json:"failed_window_uplift"`
	BaseSuccessRate      float64 `json:"base_success_rate"`
	TargetSuccessRate    float64 `json:"target_success_rate"`
	SuccessRateDelta     float64 `json:"success_rate_delta"`
	SuccessRateUplift    float64 `json:"success_rate_uplift"`
	BaseDurationP50Sec   float64 `json:"base_duration_p50_sec"`
	TargetDurationP50Sec float64 `json:"target_duration_p50_sec"`
	DurationP50Delta     float64 `json:"duration_p50_delta"`
	DurationP50Uplift    float64 `json:"duration_p50_uplift"`
	BaseDurationP95Sec   float64 `json:"base_duration_p95_sec"`
	TargetDurationP95Sec float64 `json:"target_duration_p95_sec"`
	DurationP95Delta     float64 `json:"duration_p95_delta"`
	DurationP95Uplift    float64 `json:"duration_p95_uplift"`
}

type AdminSampleVideoJobsBaselineDiffFormatStat struct {
	Format                     string  `json:"format"`
	BaseRequestedJobs          int64   `json:"base_requested_jobs"`
	TargetRequestedJobs        int64   `json:"target_requested_jobs"`
	BaseGeneratedJobs          int64   `json:"base_generated_jobs"`
	TargetGeneratedJobs        int64   `json:"target_generated_jobs"`
	BaseSuccessRate            float64 `json:"base_success_rate"`
	TargetSuccessRate          float64 `json:"target_success_rate"`
	SuccessRateDelta           float64 `json:"success_rate_delta"`
	SuccessRateUplift          float64 `json:"success_rate_uplift"`
	BaseAvgArtifactSizeBytes   float64 `json:"base_avg_artifact_size_bytes"`
	TargetAvgArtifactSizeBytes float64 `json:"target_avg_artifact_size_bytes"`
	AvgArtifactSizeDelta       float64 `json:"avg_artifact_size_delta"`
	AvgArtifactSizeUplift      float64 `json:"avg_artifact_size_uplift"`
	BaseDurationP50Sec         float64 `json:"base_duration_p50_sec"`
	TargetDurationP50Sec       float64 `json:"target_duration_p50_sec"`
	DurationP50Delta           float64 `json:"duration_p50_delta"`
	DurationP50Uplift          float64 `json:"duration_p50_uplift"`
	BaseDurationP95Sec         float64 `json:"base_duration_p95_sec"`
	TargetDurationP95Sec       float64 `json:"target_duration_p95_sec"`
	DurationP95Delta           float64 `json:"duration_p95_delta"`
	DurationP95Uplift          float64 `json:"duration_p95_uplift"`
}

type AdminSampleVideoJobsBaselineDiffResponse struct {
	BaseWindow   string                                       `json:"base_window"`
	TargetWindow string                                       `json:"target_window"`
	GeneratedAt  string                                       `json:"generated_at"`
	Summary      AdminSampleVideoJobsBaselineDiffSummary      `json:"summary"`
	Formats      []AdminSampleVideoJobsBaselineDiffFormatStat `json:"formats"`
}

// GetAdminVideoJobsOverview godoc
// @Summary Get video jobs overview (admin)
// @Tags admin
// @Produce json
// @Param window query string false "window: 24h | 7d | 30d"
// @Success 200 {object} AdminVideoJobOverviewResponse
// @Router /api/admin/video-jobs/overview [get]
func (h *Handler) GetAdminVideoJobsOverview(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	sinceWindow := now.Add(-windowDuration)
	since24h := now.Add(-24 * time.Hour)
	out := AdminVideoJobOverviewResponse{
		Window:                           windowLabel,
		WindowStart:                      sinceWindow.Format(time.RFC3339),
		WindowEnd:                        now.Format(time.RFC3339),
		FeedbackIntegrityAlertThresholds: defaultFeedbackIntegrityAlertThresholdSettings(),
		FeedbackIntegrityHealth:          "green",
		FeedbackIntegrityAlerts:          make([]AdminVideoJobFeedbackIntegrityAlert, 0, 8),
		FeedbackIntegrityHealthTrend:     make([]AdminVideoJobFeedbackIntegrityHealthTrendPoint, 0, 32),
		FeedbackIntegrityAlertCodeStats:  make([]AdminVideoJobFeedbackIntegrityAlertCodeStat, 0, 16),
		FeedbackIntegrityRecoveryStatus:  "no_data",
		FeedbackIntegrityRecommendations: make([]AdminVideoJobFeedbackIntegrityRecommendation, 0, 4),
		FeedbackIntegrityEscalation: AdminVideoJobFeedbackIntegrityEscalation{
			Required: false,
			Level:    "none",
		},
	}
	if setting, loadErr := h.loadVideoQualitySetting(); loadErr == nil {
		out.FeedbackIntegrityAlertThresholds = feedbackIntegrityAlertThresholdSettingsFromModel(setting)
	}

	if err := h.db.Model(&models.VideoJob{}).Count(&out.Total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type statusRow struct {
		Status string
		Count  int64
	}
	var statusRows []statusRow
	if err := h.db.Model(&models.VideoJob{}).
		Select("status, count(*) AS count").
		Group("status").
		Scan(&statusRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, row := range statusRows {
		switch strings.ToLower(strings.TrimSpace(row.Status)) {
		case models.VideoJobStatusQueued:
			out.Queued = row.Count
		case models.VideoJobStatusRunning:
			out.Running = row.Count
		case models.VideoJobStatusDone:
			out.Done = row.Count
		case models.VideoJobStatusFailed:
			out.Failed = row.Count
		case models.VideoJobStatusCancelled:
			out.Cancelled = row.Count
		}
	}

	if err := h.db.Model(&models.VideoJob{}).
		Where("stage = ?", models.VideoJobStageRetrying).
		Count(&out.Retrying).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Model(&models.VideoJob{}).
		Where("created_at >= ?", since24h).
		Count(&out.Created24h).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Model(&models.VideoJob{}).
		Where("status = ? AND finished_at >= ?", models.VideoJobStatusDone, since24h).
		Count(&out.Done24h).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Model(&models.VideoJob{}).
		Where("status = ? AND updated_at >= ?", models.VideoJobStatusFailed, since24h).
		Count(&out.Failed24h).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Model(&models.VideoJob{}).
		Where("created_at >= ?", sinceWindow).
		Count(&out.CreatedWindow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Model(&models.VideoJob{}).
		Where("status = ? AND finished_at >= ?", models.VideoJobStatusDone, sinceWindow).
		Count(&out.DoneWindow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Model(&models.VideoJob{}).
		Where("status = ? AND updated_at >= ?", models.VideoJobStatusFailed, sinceWindow).
		Count(&out.FailedWindow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sourceProbeJobsWindow, durationBuckets, resolutionBuckets, fpsBuckets, err := h.loadVideoJobSourceProbeBuckets(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.SourceProbeJobsWindow = sourceProbeJobsWindow
	out.SourceProbeDurationBuckets = durationBuckets
	out.SourceProbeResolutionBuckets = resolutionBuckets
	out.SourceProbeFpsBuckets = fpsBuckets
	durationQuality, resolutionQuality, fpsQuality, err := h.loadVideoJobSourceProbeQualityStats(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.SourceProbeDurationQuality = durationQuality
	out.SourceProbeResolutionQuality = resolutionQuality
	out.SourceProbeFpsQuality = fpsQuality

	type sampleWindowRow struct {
		SampleJobsWindow   int64 `gorm:"column:sample_jobs_window"`
		SampleDoneWindow   int64 `gorm:"column:sample_done_window"`
		SampleFailedWindow int64 `gorm:"column:sample_failed_window"`
	}
	var sampleWindow sampleWindowRow
	if err := h.db.Raw(`
SELECT
	COUNT(*) FILTER (WHERE v.created_at >= ?) AS sample_jobs_window,
	COUNT(*) FILTER (WHERE v.status = ? AND v.finished_at >= ?) AS sample_done_window,
	COUNT(*) FILTER (WHERE v.status = ? AND v.updated_at >= ?) AS sample_failed_window
FROM archive.video_jobs v
JOIN archive.collections c ON c.id = v.result_collection_id
WHERE c.is_sample = TRUE
`,
		sinceWindow,
		models.VideoJobStatusDone,
		sinceWindow,
		models.VideoJobStatusFailed,
		sinceWindow,
	).Scan(&sampleWindow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.SampleJobsWindow = sampleWindow.SampleJobsWindow
	out.SampleDoneWindow = sampleWindow.SampleDoneWindow
	out.SampleFailedWindow = sampleWindow.SampleFailedWindow
	if out.SampleJobsWindow > 0 {
		out.SampleSuccessRateWindow = float64(out.SampleDoneWindow) / float64(out.SampleJobsWindow)
	}

	type stageRow struct {
		Key   string
		Count int64
	}
	var stageRows []stageRow
	if err := h.db.Model(&models.VideoJob{}).
		Select("stage AS key, count(*) AS count").
		Group("stage").
		Order("count DESC").
		Scan(&stageRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.StageCounts = make([]AdminVideoJobSimpleCount, 0, len(stageRows))
	for _, row := range stageRows {
		out.StageCounts = append(out.StageCounts, AdminVideoJobSimpleCount{
			Key:   strings.TrimSpace(row.Key),
			Count: row.Count,
		})
	}

	type failureRow struct {
		Reason string
		Count  int64
	}
	var failureRows []failureRow
	if err := h.db.Model(&models.VideoJob{}).
		Select("error_message AS reason, count(*) AS count").
		Where("status = ? AND error_message <> ''", models.VideoJobStatusFailed).
		Group("error_message").
		Order("count DESC").
		Limit(6).
		Scan(&failureRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.TopFailures = make([]AdminVideoJobFailureReason, 0, len(failureRows))
	for _, row := range failureRows {
		out.TopFailures = append(out.TopFailures, AdminVideoJobFailureReason{
			Reason: strings.TrimSpace(row.Reason),
			Count:  row.Count,
		})
	}

	stageDurations, err := h.loadVideoJobStageDurations(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.StageDurations = stageDurations

	formatStats24h, err := h.loadVideoJobFormatStats24h(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FormatStats24h = formatStats24h

	feedbackSignals, feedbackDownloads, feedbackFavorites, feedbackEngagedJobs, feedbackAvgScore, err := h.loadVideoJobFeedbackOverview(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackSignalsWindow = feedbackSignals
	out.FeedbackDownloadsWindow = feedbackDownloads
	out.FeedbackFavoritesWindow = feedbackFavorites
	out.FeedbackEngagedJobsWindow = feedbackEngagedJobs
	out.FeedbackAvgScoreWindow = feedbackAvgScore
	feedbackIntegrity, err := h.loadVideoImageFeedbackIntegrityOverview(sinceWindow, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackIntegrityOverview = feedbackIntegrity
	out.FeedbackIntegrityAlerts, out.FeedbackIntegrityHealth = buildVideoJobFeedbackIntegrityAlerts(
		feedbackIntegrity,
		out.FeedbackIntegrityAlertThresholds,
	)
	out.FeedbackIntegrityRecommendations = buildFeedbackIntegrityRecommendations(
		out.FeedbackIntegrityAlerts,
		out.FeedbackIntegrityHealth,
	)
	previousWindowStart := sinceWindow.Add(-windowDuration)
	previousWindowEnd := sinceWindow
	previousFeedbackIntegrity, err := h.loadVideoImageFeedbackIntegrityOverviewRange(previousWindowStart, previousWindowEnd, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	previousAlerts, previousHealth := buildVideoJobFeedbackIntegrityAlerts(
		previousFeedbackIntegrity,
		out.FeedbackIntegrityAlertThresholds,
	)
	out.FeedbackIntegrityDelta = buildFeedbackIntegrityDelta(
		feedbackIntegrity,
		out.FeedbackIntegrityHealth,
		countEffectiveFeedbackIntegrityAlerts(out.FeedbackIntegrityAlerts),
		previousFeedbackIntegrity,
		previousHealth,
		countEffectiveFeedbackIntegrityAlerts(previousAlerts),
		previousWindowStart,
		previousWindowEnd,
	)
	feedbackHealthTrend, err := h.loadVideoImageFeedbackIntegrityHealthTrend(
		sinceWindow,
		now,
		out.FeedbackIntegrityAlertThresholds,
		nil,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackIntegrityHealthTrend = feedbackHealthTrend
	out.FeedbackIntegrityAlertCodeStats = buildFeedbackIntegrityAlertCodeStats(feedbackHealthTrend)
	out.FeedbackIntegrityStreaks = buildFeedbackIntegrityStreaks(feedbackHealthTrend)
	out.FeedbackIntegrityRecoveryStatus, out.FeedbackIntegrityRecovered, out.FeedbackIntegrityPreviousHealth = resolveFeedbackIntegrityRecovery(
		feedbackHealthTrend,
	)
	out.FeedbackIntegrityEscalation = buildFeedbackIntegrityEscalation(
		out.FeedbackIntegrityHealth,
		out.FeedbackIntegrityStreaks,
		out.FeedbackIntegrityDelta,
		out.FeedbackIntegrityRecoveryStatus,
	)
	out.FeedbackIntegrityEscalationTrend = buildFeedbackIntegrityEscalationTrend(feedbackHealthTrend)
	out.FeedbackIntegrityEscalationStats = buildFeedbackIntegrityEscalationStats(out.FeedbackIntegrityEscalationTrend)
	out.FeedbackIntegrityEscalationIncidents = buildFeedbackIntegrityEscalationIncidents(out.FeedbackIntegrityEscalationTrend, 7)
	feedbackAnomalyJobs, feedbackTopPickConflictJobs, err := h.loadVideoImageFeedbackIntegrityRiskJobCounts(sinceWindow, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackIntegrityAnomalyJobs = feedbackAnomalyJobs
	out.FeedbackIntegrityTopPickJobs = feedbackTopPickConflictJobs

	feedbackSceneStats, err := h.loadVideoJobFeedbackSceneStats(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackSceneStats = feedbackSceneStats

	feedbackActionStats, err := h.loadVideoImageFeedbackActionStats(sinceWindow, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackActionStats = feedbackActionStats

	feedbackTopSceneTags, err := h.loadVideoImageFeedbackTopSceneStats(sinceWindow, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackTopSceneTags = feedbackTopSceneTags

	feedbackTrend, err := h.loadVideoImageFeedbackTrend(sinceWindow, now, windowDuration, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackTrend = feedbackTrend

	feedbackNegativeGuardOverview, err := h.loadVideoJobFeedbackNegativeGuardOverview(sinceWindow, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackNegativeGuardOverview = feedbackNegativeGuardOverview
	feedbackNegativeGuardReasons, err := h.loadVideoJobFeedbackNegativeGuardReasonStats(sinceWindow, nil, 12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackNegativeGuardReasons = feedbackNegativeGuardReasons

	feedbackEnabled, currentRollout, liveGuardConfig, err := h.loadCurrentFeedbackRolloutConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.LiveCoverSceneMinSamples = liveGuardConfig.MinSamples
	out.LiveCoverSceneGuardMinTotal = liveGuardConfig.MinTotal
	out.LiveCoverSceneGuardScoreFloor = liveGuardConfig.ScoreFloor

	liveCoverSceneStats, err := h.loadVideoJobLiveCoverSceneStats(sinceWindow, liveGuardConfig.MinSamples)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.LiveCoverSceneStats = liveCoverSceneStats

	gifLoopOverview, err := h.loadVideoJobGIFLoopTuneOverview(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFLoopTuneOverview = gifLoopOverview
	gifEvaluationOverview, err := h.loadVideoJobGIFEvaluationOverview(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFEvaluationOverview = gifEvaluationOverview
	gifBaselineSnapshots, err := h.loadLatestVideoJobGIFBaselines(7)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFBaselineSnapshots = gifBaselineSnapshots
	gifTopSamples, err := h.loadVideoJobGIFEvaluationSamples(sinceWindow, 5, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFEvaluationTopSamples = gifTopSamples
	gifLowSamples, err := h.loadVideoJobGIFEvaluationSamples(sinceWindow, 5, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFEvaluationLowSamples = gifLowSamples
	gifManualScoreOverview, err := h.loadVideoJobGIFManualScoreOverview(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFManualScoreOverview = gifManualScoreOverview
	gifManualScoreDiffSamples, err := h.loadVideoJobGIFManualScoreDiffSamples(sinceWindow, 8)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFManualScoreDiffSamples = gifManualScoreDiffSamples

	feedbackGroupStats, err := h.loadVideoJobFeedbackGroupStats(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackGroupStats = feedbackGroupStats

	feedbackGroupFormatStats, err := h.loadVideoJobFeedbackGroupFormatStats(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackGroupFormatStats = feedbackGroupFormatStats

	feedbackHistory, err := h.loadVideoJobFeedbackGroupStatsHistory(windowDuration, 3, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	recommendation := buildFeedbackRolloutRecommendationWithHistory(
		feedbackEnabled,
		currentRollout,
		feedbackHistory,
		3,
		liveCoverSceneStats,
		liveGuardConfig,
	)
	out.FeedbackRolloutRecommendation = &recommendation
	rolloutAudits, err := h.loadVideoJobFeedbackRolloutAudits(12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackRolloutAuditLogs = rolloutAudits

	type durationRow struct {
		P50 *float64 `gorm:"column:p50"`
		P95 *float64 `gorm:"column:p95"`
	}
	var duration durationRow
	if err := h.db.Raw(`
SELECT
	percentile_cont(0.50) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at))) AS p50,
	percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at))) AS p95
FROM archive.video_jobs
WHERE status = ?
	AND started_at IS NOT NULL
	AND finished_at IS NOT NULL
	AND finished_at >= ?
`, models.VideoJobStatusDone, sinceWindow).Scan(&duration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if duration.P50 != nil {
		out.DurationP50Sec = *duration.P50
	}
	if duration.P95 != nil {
		out.DurationP95Sec = *duration.P95
	}

	var sampleDuration durationRow
	if err := h.db.Raw(`
SELECT
	percentile_cont(0.50) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (v.finished_at - v.started_at))) AS p50,
	percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (v.finished_at - v.started_at))) AS p95
FROM archive.video_jobs v
JOIN archive.collections c ON c.id = v.result_collection_id
WHERE c.is_sample = TRUE
	AND v.status = ?
	AND v.started_at IS NOT NULL
	AND v.finished_at IS NOT NULL
	AND v.finished_at >= ?
`, models.VideoJobStatusDone, sinceWindow).Scan(&sampleDuration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sampleDuration.P50 != nil {
		out.SampleDurationP50Sec = *sampleDuration.P50
	}
	if sampleDuration.P95 != nil {
		out.SampleDurationP95Sec = *sampleDuration.P95
	}
	_ = h.db.Model(&models.VideoJobCost{}).
		Select("COALESCE(sum(estimated_cost), 0)").
		Scan(&out.CostTotal).Error
	_ = h.db.Model(&models.VideoJobCost{}).
		Select("COALESCE(sum(estimated_cost), 0)").
		Where("created_at >= ?", since24h).
		Scan(&out.Cost24h).Error
	_ = h.db.Model(&models.VideoJobCost{}).
		Select("COALESCE(avg(estimated_cost), 0)").
		Where("created_at >= ?", since24h).
		Scan(&out.CostAvg24h).Error
	_ = h.db.Model(&models.VideoJobCost{}).
		Select("COALESCE(sum(estimated_cost), 0)").
		Where("created_at >= ?", sinceWindow).
		Scan(&out.CostWindow).Error
	_ = h.db.Model(&models.VideoJobCost{}).
		Select("COALESCE(avg(estimated_cost), 0)").
		Where("created_at >= ?", sinceWindow).
		Scan(&out.CostAvgWindow).Error

	c.JSON(http.StatusOK, out)
}

// GetAdminVideoJobsFeedbackIntegrityOverview godoc
// @Summary Get video-jobs feedback integrity overview (admin)
// @Tags admin
// @Produce json
// @Param window query string false "window: 24h | 7d | 30d"
// @Param user_id query int false "user id filter"
// @Param format query string false "requested format filter"
// @Param guard_reason query string false "negative guard reason filter"
// @Success 200 {object} AdminVideoJobFeedbackIntegrityOverviewResponse
// @Router /api/admin/video-jobs/feedback-integrity/overview [get]
func (h *Handler) GetAdminVideoJobsFeedbackIntegrityOverview(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter, err := parseVideoImageFeedbackFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	sinceWindow := now.Add(-windowDuration)
	out := AdminVideoJobFeedbackIntegrityOverviewResponse{
		Window:                           windowLabel,
		WindowStart:                      sinceWindow.Format(time.RFC3339),
		WindowEnd:                        now.Format(time.RFC3339),
		FeedbackLearningChain:            buildFeedbackLearningChainStatus(h.cfg.EnableLegacyFeedbackFallback),
		FeedbackIntegrityAlertThresholds: defaultFeedbackIntegrityAlertThresholdSettings(),
		FeedbackIntegrityHealth:          "green",
		FeedbackIntegrityAlerts:          make([]AdminVideoJobFeedbackIntegrityAlert, 0, 8),
		FeedbackIntegrityHealthTrend:     make([]AdminVideoJobFeedbackIntegrityHealthTrendPoint, 0, 32),
		FeedbackIntegrityAlertCodeStats:  make([]AdminVideoJobFeedbackIntegrityAlertCodeStat, 0, 16),
		FeedbackIntegrityRecoveryStatus:  "no_data",
		FeedbackIntegrityRecommendations: make([]AdminVideoJobFeedbackIntegrityRecommendation, 0, 4),
		FeedbackIntegrityEscalation: AdminVideoJobFeedbackIntegrityEscalation{
			Required: false,
			Level:    "none",
		},
	}
	if filter != nil {
		out.FilterUserID = filter.UserID
		out.FilterFormat = filter.Format
		out.FilterGuardReason = filter.GuardReason
	}
	if count, countErr := h.loadLegacyEvalBackfillCandidateCount(); countErr == nil {
		out.FeedbackLearningChain.LegacyEvalBackfillCandidates = count
	}
	if setting, loadErr := h.loadVideoQualitySetting(); loadErr == nil {
		out.FeedbackIntegrityAlertThresholds = feedbackIntegrityAlertThresholdSettingsFromModel(setting)
	}

	feedbackIntegrity, err := h.loadVideoImageFeedbackIntegrityOverview(sinceWindow, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackIntegrityOverview = feedbackIntegrity
	out.FeedbackIntegrityAlerts, out.FeedbackIntegrityHealth = buildVideoJobFeedbackIntegrityAlerts(
		feedbackIntegrity,
		out.FeedbackIntegrityAlertThresholds,
	)
	out.FeedbackIntegrityRecommendations = buildFeedbackIntegrityRecommendations(
		out.FeedbackIntegrityAlerts,
		out.FeedbackIntegrityHealth,
	)

	previousWindowStart := sinceWindow.Add(-windowDuration)
	previousWindowEnd := sinceWindow
	previousFeedbackIntegrity, err := h.loadVideoImageFeedbackIntegrityOverviewRange(previousWindowStart, previousWindowEnd, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	previousAlerts, previousHealth := buildVideoJobFeedbackIntegrityAlerts(
		previousFeedbackIntegrity,
		out.FeedbackIntegrityAlertThresholds,
	)
	out.FeedbackIntegrityDelta = buildFeedbackIntegrityDelta(
		feedbackIntegrity,
		out.FeedbackIntegrityHealth,
		countEffectiveFeedbackIntegrityAlerts(out.FeedbackIntegrityAlerts),
		previousFeedbackIntegrity,
		previousHealth,
		countEffectiveFeedbackIntegrityAlerts(previousAlerts),
		previousWindowStart,
		previousWindowEnd,
	)
	feedbackHealthTrend, err := h.loadVideoImageFeedbackIntegrityHealthTrend(
		sinceWindow,
		now,
		out.FeedbackIntegrityAlertThresholds,
		filter,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackIntegrityHealthTrend = feedbackHealthTrend
	out.FeedbackIntegrityAlertCodeStats = buildFeedbackIntegrityAlertCodeStats(feedbackHealthTrend)
	out.FeedbackIntegrityStreaks = buildFeedbackIntegrityStreaks(feedbackHealthTrend)
	out.FeedbackIntegrityRecoveryStatus, out.FeedbackIntegrityRecovered, out.FeedbackIntegrityPreviousHealth = resolveFeedbackIntegrityRecovery(
		feedbackHealthTrend,
	)
	out.FeedbackIntegrityEscalation = buildFeedbackIntegrityEscalation(
		out.FeedbackIntegrityHealth,
		out.FeedbackIntegrityStreaks,
		out.FeedbackIntegrityDelta,
		out.FeedbackIntegrityRecoveryStatus,
	)
	out.FeedbackIntegrityEscalationTrend = buildFeedbackIntegrityEscalationTrend(feedbackHealthTrend)
	out.FeedbackIntegrityEscalationStats = buildFeedbackIntegrityEscalationStats(out.FeedbackIntegrityEscalationTrend)
	out.FeedbackIntegrityEscalationIncidents = buildFeedbackIntegrityEscalationIncidents(out.FeedbackIntegrityEscalationTrend, 7)
	feedbackAnomalyJobs, feedbackTopPickConflictJobs, err := h.loadVideoImageFeedbackIntegrityRiskJobCounts(sinceWindow, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackIntegrityAnomalyJobs = feedbackAnomalyJobs
	out.FeedbackIntegrityTopPickJobs = feedbackTopPickConflictJobs

	feedbackActionStats, err := h.loadVideoImageFeedbackActionStats(sinceWindow, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackActionStats = feedbackActionStats

	feedbackTopSceneTags, err := h.loadVideoImageFeedbackTopSceneStats(sinceWindow, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackTopSceneTags = feedbackTopSceneTags

	feedbackTrend, err := h.loadVideoImageFeedbackTrend(sinceWindow, now, windowDuration, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackTrend = feedbackTrend

	feedbackNegativeGuardOverview, err := h.loadVideoJobFeedbackNegativeGuardOverview(sinceWindow, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackNegativeGuardOverview = feedbackNegativeGuardOverview
	feedbackNegativeGuardReasons, err := h.loadVideoJobFeedbackNegativeGuardReasonStats(sinceWindow, filter, 12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackNegativeGuardReasons = feedbackNegativeGuardReasons

	c.JSON(http.StatusOK, out)
}

// ExportAdminSampleVideoJobsBaselineCSV godoc
// @Summary Export sample video-jobs baseline CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/samples/baseline.csv [get]
func (h *Handler) ExportAdminSampleVideoJobsBaselineCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	since := time.Now().Add(-windowDuration)

	summary, err := h.loadSampleVideoJobsWindowSummary(since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	formatRows, err := h.loadSampleVideoJobFormatBaselineRows(since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"section",
		"window",
		"metric",
		"value",
		"format",
		"requested_jobs",
		"generated_jobs",
		"success_rate",
		"artifact_count",
		"avg_artifact_size_bytes",
		"duration_p50_sec",
		"duration_p95_sec",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	summaryRows := []struct {
		metric string
		value  string
	}{
		{metric: "sample_jobs_window", value: strconv.FormatInt(summary.JobsWindow, 10)},
		{metric: "sample_done_window", value: strconv.FormatInt(summary.DoneWindow, 10)},
		{metric: "sample_failed_window", value: strconv.FormatInt(summary.FailedWindow, 10)},
		{metric: "sample_success_rate_window", value: fmt.Sprintf("%.6f", summary.SuccessRate)},
		{metric: "sample_duration_p50_sec", value: fmt.Sprintf("%.6f", summary.DurationP50)},
		{metric: "sample_duration_p95_sec", value: fmt.Sprintf("%.6f", summary.DurationP95)},
	}
	for _, row := range summaryRows {
		if err := writer.Write([]string{
			"summary",
			windowLabel,
			row.metric,
			row.value,
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	for _, row := range formatRows {
		if err := writer.Write([]string{
			"format",
			windowLabel,
			"",
			"",
			row.Format,
			strconv.FormatInt(row.RequestedJobs, 10),
			strconv.FormatInt(row.GeneratedJobs, 10),
			fmt.Sprintf("%.6f", row.SuccessRate),
			strconv.FormatInt(row.ArtifactCount, 10),
			fmt.Sprintf("%.6f", row.AvgArtifactSizeBytes),
			fmt.Sprintf("%.6f", row.DurationP50Sec),
			fmt.Sprintf("%.6f", row.DurationP95Sec),
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("video_jobs_sample_baseline_%s_%s.csv", windowLabel, time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAdminSampleVideoJobsBaselineDiffCSV godoc
// @Summary Export sample video-jobs baseline diff CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param base_window query string false "reference window: 24h | 7d | 30d (default: 7d)"
// @Param target_window query string false "target window: 24h | 7d | 30d (default: 24h)"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/samples/baseline-diff.csv [get]
func (h *Handler) ExportAdminSampleVideoJobsBaselineDiffCSV(c *gin.Context) {
	baseLabel, baseDuration, err := parseVideoJobsOverviewWindow(c.DefaultQuery("base_window", "7d"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base_window"})
		return
	}
	targetLabel, targetDuration, err := parseVideoJobsOverviewWindow(c.DefaultQuery("target_window", "24h"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_window"})
		return
	}

	now := time.Now()
	baseSince := now.Add(-baseDuration)
	targetSince := now.Add(-targetDuration)

	baseSummary, err := h.loadSampleVideoJobsWindowSummary(baseSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetSummary, err := h.loadSampleVideoJobsWindowSummary(targetSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	baseRows, err := h.loadSampleVideoJobFormatBaselineRows(baseSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetRows, err := h.loadSampleVideoJobFormatBaselineRows(targetSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	diffRows := buildSampleVideoJobFormatDiffRows(baseRows, targetRows)

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"section",
		"base_window",
		"target_window",
		"metric",
		"base_value",
		"target_value",
		"delta",
		"uplift",
		"format",
		"base_requested_jobs",
		"target_requested_jobs",
		"base_generated_jobs",
		"target_generated_jobs",
		"base_success_rate",
		"target_success_rate",
		"success_rate_delta",
		"success_rate_uplift",
		"base_avg_artifact_size_bytes",
		"target_avg_artifact_size_bytes",
		"avg_artifact_size_delta",
		"avg_artifact_size_uplift",
		"base_duration_p50_sec",
		"target_duration_p50_sec",
		"duration_p50_delta",
		"duration_p50_uplift",
		"base_duration_p95_sec",
		"target_duration_p95_sec",
		"duration_p95_delta",
		"duration_p95_uplift",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	writeSummaryMetric := func(metric string, baseValue, targetValue float64) error {
		delta, uplift := computeDeltaAndUplift(baseValue, targetValue)
		return writer.Write([]string{
			"summary",
			baseLabel,
			targetLabel,
			metric,
			fmt.Sprintf("%.6f", baseValue),
			fmt.Sprintf("%.6f", targetValue),
			fmt.Sprintf("%.6f", delta),
			fmt.Sprintf("%.6f", uplift),
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
		})
	}

	if err := writeSummaryMetric("sample_jobs_window", float64(baseSummary.JobsWindow), float64(targetSummary.JobsWindow)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := writeSummaryMetric("sample_done_window", float64(baseSummary.DoneWindow), float64(targetSummary.DoneWindow)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := writeSummaryMetric("sample_failed_window", float64(baseSummary.FailedWindow), float64(targetSummary.FailedWindow)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := writeSummaryMetric("sample_success_rate_window", baseSummary.SuccessRate, targetSummary.SuccessRate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := writeSummaryMetric("sample_duration_p50_sec", baseSummary.DurationP50, targetSummary.DurationP50); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := writeSummaryMetric("sample_duration_p95_sec", baseSummary.DurationP95, targetSummary.DurationP95); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, row := range diffRows {
		if err := writer.Write([]string{
			"format",
			baseLabel,
			targetLabel,
			"",
			"",
			"",
			"",
			"",
			row.Format,
			strconv.FormatInt(row.BaseRequestedJobs, 10),
			strconv.FormatInt(row.TargetRequestedJobs, 10),
			strconv.FormatInt(row.BaseGeneratedJobs, 10),
			strconv.FormatInt(row.TargetGeneratedJobs, 10),
			fmt.Sprintf("%.6f", row.BaseSuccessRate),
			fmt.Sprintf("%.6f", row.TargetSuccessRate),
			fmt.Sprintf("%.6f", row.SuccessRateDelta),
			fmt.Sprintf("%.6f", row.SuccessRateUplift),
			fmt.Sprintf("%.6f", row.BaseAvgArtifactSizeBytes),
			fmt.Sprintf("%.6f", row.TargetAvgArtifactSizeBytes),
			fmt.Sprintf("%.6f", row.AvgArtifactSizeDelta),
			fmt.Sprintf("%.6f", row.AvgArtifactSizeUplift),
			fmt.Sprintf("%.6f", row.BaseDurationP50Sec),
			fmt.Sprintf("%.6f", row.TargetDurationP50Sec),
			fmt.Sprintf("%.6f", row.DurationP50Delta),
			fmt.Sprintf("%.6f", row.DurationP50Uplift),
			fmt.Sprintf("%.6f", row.BaseDurationP95Sec),
			fmt.Sprintf("%.6f", row.TargetDurationP95Sec),
			fmt.Sprintf("%.6f", row.DurationP95Delta),
			fmt.Sprintf("%.6f", row.DurationP95Uplift),
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf(
		"video_jobs_sample_baseline_diff_%s_vs_%s_%s.csv",
		targetLabel,
		baseLabel,
		time.Now().Format("20060102_150405"),
	)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAdminVideoJobsFeedbackCSV godoc
// @Summary Export video-jobs feedback report CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param user_id query int false "user id filter"
// @Param format query string false "requested format filter"
// @Param guard_reason query string false "negative guard reason filter"
// @Param blocked_only query bool false "only export blocked_reason rows"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/feedback-report.csv [get]
func (h *Handler) ExportAdminVideoJobsFeedbackCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter, err := parseVideoImageFeedbackFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	blockedOnlyParam, ok := parseOptionalBoolParam(c.Query("blocked_only"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid blocked_only"})
		return
	}
	blockedOnly := blockedOnlyParam != nil && *blockedOnlyParam

	now := time.Now()
	since := now.Add(-windowDuration)

	actionRows := make([]AdminVideoJobFeedbackActionStat, 0)
	sceneRows := make([]AdminVideoJobFeedbackSceneStat, 0)
	trendRows := make([]AdminVideoJobFeedbackTrendPoint, 0)
	negativeGuardOverview := AdminVideoJobFeedbackNegativeGuardOverview{}
	if !blockedOnly {
		actionRows, err = h.loadVideoImageFeedbackActionStats(since, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		sceneRows, err = h.loadVideoImageFeedbackTopSceneStats(since, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		trendRows, err = h.loadVideoImageFeedbackTrend(since, now, windowDuration, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		negativeGuardOverview, err = h.loadVideoJobFeedbackNegativeGuardOverview(since, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	negativeGuardJobRows, err := h.loadVideoJobFeedbackNegativeGuardJobRows(since, filter, 300, blockedOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{
		"section",
		"window",
		"window_start",
		"window_end",
		"bucket",
		"action",
		"scene_tag",
		"count",
		"ratio",
		"weight_sum",
		"metric",
		"value",
		"total",
		"positive",
		"neutral",
		"negative",
		"top_pick",
		"filter_user_id",
		"filter_format",
		"filter_guard_reason",
		"job_id",
		"job_user_id",
		"job_title",
		"job_group",
		"guard_hit",
		"blocked_reason",
		"before_reason",
		"after_reason",
		"before_start_sec",
		"before_end_sec",
		"after_start_sec",
		"after_end_sec",
		"guard_reason_list",
		"job_finished_at",
	}
	writeRow := func(row []string) error {
		if len(row) < len(header) {
			row = append(row, make([]string, len(header)-len(row))...)
		}
		return writer.Write(row)
	}
	if err := writer.Write(header); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	windowStartText := since.Format(time.RFC3339)
	windowEndText := now.Format(time.RFC3339)
	filterUserID := ""
	filterFormat := ""
	filterGuardReason := ""
	if filter != nil {
		if filter.UserID > 0 {
			filterUserID = strconv.FormatUint(filter.UserID, 10)
		}
		filterFormat = filter.Format
		filterGuardReason = filter.GuardReason
	}
	formatOptionalFloat := func(value *float64) string {
		if value == nil {
			return ""
		}
		return fmt.Sprintf("%.6f", *value)
	}
	formatOptionalTime := func(value *time.Time) string {
		if value == nil || value.IsZero() {
			return ""
		}
		return value.Format(time.RFC3339)
	}

	if err := writeRow([]string{
		"meta",
		windowLabel,
		windowStartText,
		windowEndText,
		"blocked_only",
		strconv.FormatBool(blockedOnly),
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		filterUserID,
		filterFormat,
		filterGuardReason,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !blockedOnly {
		for _, item := range actionRows {
			if err := writeRow([]string{
				"action",
				windowLabel,
				windowStartText,
				windowEndText,
				"",
				item.Action,
				"",
				strconv.FormatInt(item.Count, 10),
				fmt.Sprintf("%.6f", item.Ratio),
				fmt.Sprintf("%.6f", item.WeightSum),
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				filterUserID,
				filterFormat,
				filterGuardReason,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		for _, item := range sceneRows {
			if err := writeRow([]string{
				"scene",
				windowLabel,
				windowStartText,
				windowEndText,
				"",
				"",
				item.SceneTag,
				strconv.FormatInt(item.Signals, 10),
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				filterUserID,
				filterFormat,
				filterGuardReason,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		for _, item := range trendRows {
			if err := writeRow([]string{
				"trend",
				windowLabel,
				windowStartText,
				windowEndText,
				item.Bucket,
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				strconv.FormatInt(item.Total, 10),
				strconv.FormatInt(item.Positive, 10),
				strconv.FormatInt(item.Neutral, 10),
				strconv.FormatInt(item.Negative, 10),
				strconv.FormatInt(item.TopPick, 10),
				filterUserID,
				filterFormat,
				filterGuardReason,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		negativeGuardMetrics := []struct {
			metric string
			value  string
		}{
			{metric: "samples", value: strconv.FormatInt(negativeGuardOverview.Samples, 10)},
			{metric: "treatment_jobs", value: strconv.FormatInt(negativeGuardOverview.TreatmentJobs, 10)},
			{metric: "guard_enabled_jobs", value: strconv.FormatInt(negativeGuardOverview.GuardEnabledJobs, 10)},
			{metric: "guard_reason_hit_jobs", value: strconv.FormatInt(negativeGuardOverview.GuardReasonHitJobs, 10)},
			{metric: "selection_shift_jobs", value: strconv.FormatInt(negativeGuardOverview.SelectionShiftJobs, 10)},
			{metric: "blocked_reason_jobs", value: strconv.FormatInt(negativeGuardOverview.BlockedReasonJobs, 10)},
			{metric: "guard_hit_rate", value: fmt.Sprintf("%.6f", negativeGuardOverview.GuardHitRate)},
			{metric: "selection_shift_rate", value: fmt.Sprintf("%.6f", negativeGuardOverview.SelectionShiftRate)},
			{metric: "blocked_reason_rate", value: fmt.Sprintf("%.6f", negativeGuardOverview.BlockedReasonRate)},
			{metric: "avg_negative_signals", value: fmt.Sprintf("%.6f", negativeGuardOverview.AvgNegativeSignals)},
			{metric: "avg_positive_signals", value: fmt.Sprintf("%.6f", negativeGuardOverview.AvgPositiveSignals)},
		}
		for _, item := range negativeGuardMetrics {
			if err := writeRow([]string{
				"negative_guard_overview",
				windowLabel,
				windowStartText,
				windowEndText,
				"",
				"",
				"",
				"",
				"",
				"",
				item.metric,
				item.value,
				"",
				"",
				"",
				"",
				"",
				filterUserID,
				filterFormat,
				filterGuardReason,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	for _, item := range negativeGuardJobRows {
		if err := writeRow([]string{
			"negative_guard_job_detail",
			windowLabel,
			windowStartText,
			windowEndText,
			"",
			"",
			"",
			"",
			"",
			"",
			"blocked_reason",
			strconv.FormatBool(item.BlockedReason),
			"",
			"",
			"",
			"",
			"",
			filterUserID,
			filterFormat,
			filterGuardReason,
			strconv.FormatUint(item.JobID, 10),
			strconv.FormatUint(item.UserID, 10),
			item.Title,
			item.Group,
			strconv.FormatBool(item.GuardHit),
			strconv.FormatBool(item.BlockedReason),
			item.BeforeReason,
			item.AfterReason,
			formatOptionalFloat(item.BeforeStartSec),
			formatOptionalFloat(item.BeforeEndSec),
			formatOptionalFloat(item.AfterStartSec),
			formatOptionalFloat(item.AfterEndSec),
			item.GuardReasonList,
			formatOptionalTime(item.FinishedAt),
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filenamePrefix := "video_jobs_feedback_report"
	if blockedOnly {
		filenamePrefix = "video_jobs_feedback_blocked_report"
	}
	filename := fmt.Sprintf("%s_%s_%s.csv", filenamePrefix, windowLabel, now.Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAdminVideoJobsFeedbackIntegrityCSV godoc
// @Summary Export video-jobs feedback integrity CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param user_id query int false "user id filter"
// @Param format query string false "requested format filter"
// @Param guard_reason query string false "negative guard reason filter"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/feedback-integrity.csv [get]
func (h *Handler) ExportAdminVideoJobsFeedbackIntegrityCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter, err := parseVideoImageFeedbackFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	since := now.Add(-windowDuration)
	overview, err := h.loadVideoImageFeedbackIntegrityOverview(since, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	thresholds := defaultFeedbackIntegrityAlertThresholdSettings()
	if setting, loadErr := h.loadVideoQualitySetting(); loadErr == nil {
		thresholds = feedbackIntegrityAlertThresholdSettingsFromModel(setting)
	}
	alertRows, health := buildVideoJobFeedbackIntegrityAlerts(overview, thresholds)
	actionRows, err := h.loadVideoImageFeedbackIntegrityActionRows(since, filter, 24)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filterUserID := ""
	filterFormat := ""
	filterGuardReason := ""
	if filter != nil {
		if filter.UserID > 0 {
			filterUserID = strconv.FormatUint(filter.UserID, 10)
		}
		filterFormat = filter.Format
		filterGuardReason = filter.GuardReason
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{
		"section",
		"window",
		"window_start",
		"window_end",
		"action",
		"samples",
		"with_output_id",
		"missing_output_id",
		"resolved_output",
		"orphan_output",
		"job_mismatch",
		"top_pick_multi_hit_users",
		"output_coverage_rate",
		"output_resolved_rate",
		"output_job_consistency_rate",
		"health",
		"alert_code",
		"alert_message",
		"metric_value",
		"warn_threshold",
		"critical_threshold",
		"filter_user_id",
		"filter_format",
		"filter_guard_reason",
	}
	if err := writer.Write(header); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	writeRow := func(row []string) error {
		if len(row) < len(header) {
			row = append(row, make([]string, len(header)-len(row))...)
		}
		return writer.Write(row)
	}
	windowStartText := since.Format(time.RFC3339)
	windowEndText := now.Format(time.RFC3339)
	if err := writeRow([]string{
		"summary",
		windowLabel,
		windowStartText,
		windowEndText,
		"all",
		strconv.FormatInt(overview.Samples, 10),
		strconv.FormatInt(overview.WithOutputID, 10),
		strconv.FormatInt(overview.MissingOutputID, 10),
		strconv.FormatInt(overview.ResolvedOutput, 10),
		strconv.FormatInt(overview.OrphanOutput, 10),
		strconv.FormatInt(overview.JobMismatch, 10),
		strconv.FormatInt(overview.TopPickMultiHitUsers, 10),
		fmt.Sprintf("%.6f", overview.OutputCoverageRate),
		fmt.Sprintf("%.6f", overview.OutputResolvedRate),
		fmt.Sprintf("%.6f", overview.OutputJobConsistencyRate),
		health,
		"",
		"",
		"",
		"",
		"",
		filterUserID,
		filterFormat,
		filterGuardReason,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, item := range actionRows {
		if err := writeRow([]string{
			"action",
			windowLabel,
			windowStartText,
			windowEndText,
			item.Action,
			strconv.FormatInt(item.Samples, 10),
			strconv.FormatInt(item.WithOutputID, 10),
			strconv.FormatInt(item.MissingOutputID, 10),
			strconv.FormatInt(item.ResolvedOutput, 10),
			strconv.FormatInt(item.OrphanOutput, 10),
			strconv.FormatInt(item.JobMismatch, 10),
			"",
			fmt.Sprintf("%.6f", item.OutputCoverageRate),
			fmt.Sprintf("%.6f", item.OutputResolvedRate),
			fmt.Sprintf("%.6f", item.OutputJobConsistencyRate),
			"",
			"",
			"",
			"",
			"",
			"",
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	thresholdRows := []struct {
		key      string
		value    string
		warn     string
		critical string
	}{
		{
			key:      "output_coverage_rate",
			value:    fmt.Sprintf("%.6f", overview.OutputCoverageRate),
			warn:     fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputCoverageRateWarn),
			critical: fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputCoverageRateCritical),
		},
		{
			key:      "output_resolved_rate",
			value:    fmt.Sprintf("%.6f", overview.OutputResolvedRate),
			warn:     fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputResolvedRateWarn),
			critical: fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputResolvedRateCritical),
		},
		{
			key:      "output_job_consistency_rate",
			value:    fmt.Sprintf("%.6f", overview.OutputJobConsistencyRate),
			warn:     fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputJobConsistencyRateWarn),
			critical: fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputJobConsistencyRateCritical),
		},
		{
			key:      "top_pick_multi_hit_users",
			value:    strconv.FormatInt(overview.TopPickMultiHitUsers, 10),
			warn:     strconv.Itoa(thresholds.FeedbackIntegrityTopPickConflictUsersWarn),
			critical: strconv.Itoa(thresholds.FeedbackIntegrityTopPickConflictUsersCritical),
		},
	}
	for _, item := range thresholdRows {
		if err := writeRow([]string{
			"threshold",
			windowLabel,
			windowStartText,
			windowEndText,
			item.key,
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			item.value,
			item.warn,
			item.critical,
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	for _, item := range alertRows {
		if err := writeRow([]string{
			"alert",
			windowLabel,
			windowStartText,
			windowEndText,
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			item.Level,
			item.Code,
			item.Message,
			"",
			"",
			"",
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("video_jobs_feedback_integrity_%s_%s.csv", windowLabel, now.Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAdminVideoJobsFeedbackIntegrityTrendCSV godoc
// @Summary Export video-jobs feedback integrity trend CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param user_id query int false "user id filter"
// @Param format query string false "requested format filter"
// @Param guard_reason query string false "negative guard reason filter"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/feedback-integrity-trend.csv [get]
func (h *Handler) ExportAdminVideoJobsFeedbackIntegrityTrendCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter, err := parseVideoImageFeedbackFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	since := now.Add(-windowDuration)
	thresholds := defaultFeedbackIntegrityAlertThresholdSettings()
	if setting, loadErr := h.loadVideoQualitySetting(); loadErr == nil {
		thresholds = feedbackIntegrityAlertThresholdSettingsFromModel(setting)
	}
	trend, err := h.loadVideoImageFeedbackIntegrityHealthTrend(since, now, thresholds, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	alertCodeStats := buildFeedbackIntegrityAlertCodeStats(trend)
	escalationTrend := buildFeedbackIntegrityEscalationTrend(trend)
	escalationIncidents := buildFeedbackIntegrityEscalationIncidents(escalationTrend, len(escalationTrend))
	streaks := buildFeedbackIntegrityStreaks(trend)
	recoveryStatus, recovered, previousHealth := resolveFeedbackIntegrityRecovery(trend)
	previousWindowStart := since.Add(-windowDuration)
	previousWindowEnd := since
	currentOverview, currentOverviewErr := h.loadVideoImageFeedbackIntegrityOverviewRange(since, now, filter)
	if currentOverviewErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": currentOverviewErr.Error()})
		return
	}
	previousOverview, previousOverviewErr := h.loadVideoImageFeedbackIntegrityOverviewRange(previousWindowStart, previousWindowEnd, filter)
	if previousOverviewErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": previousOverviewErr.Error()})
		return
	}
	currentAlerts, currentHealth := buildVideoJobFeedbackIntegrityAlerts(currentOverview, thresholds)
	previousAlerts, previousHealthForDelta := buildVideoJobFeedbackIntegrityAlerts(previousOverview, thresholds)
	delta := buildFeedbackIntegrityDelta(
		currentOverview,
		currentHealth,
		countEffectiveFeedbackIntegrityAlerts(currentAlerts),
		previousOverview,
		previousHealthForDelta,
		countEffectiveFeedbackIntegrityAlerts(previousAlerts),
		previousWindowStart,
		previousWindowEnd,
	)
	escalation := buildFeedbackIntegrityEscalation(currentHealth, streaks, delta, recoveryStatus)

	filterUserID := ""
	filterFormat := ""
	filterGuardReason := ""
	if filter != nil {
		if filter.UserID > 0 {
			filterUserID = strconv.FormatUint(filter.UserID, 10)
		}
		filterFormat = filter.Format
		filterGuardReason = filter.GuardReason
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{
		"section",
		"window",
		"window_start",
		"window_end",
		"bucket",
		"samples",
		"health",
		"alert_count",
		"output_coverage_rate",
		"output_resolved_rate",
		"output_job_consistency_rate",
		"top_pick_multi_hit_users",
		"recovery_status",
		"recovered",
		"previous_health",
		"alert_code",
		"days_hit",
		"latest_bucket",
		"latest_level",
		"escalation_level",
		"escalation_required",
		"escalation_reason",
		"escalation_triggered_rules",
		"filter_user_id",
		"filter_format",
		"filter_guard_reason",
	}
	if err := writer.Write(header); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	writeRow := func(row []string) error {
		if len(row) < len(header) {
			row = append(row, make([]string, len(header)-len(row))...)
		}
		return writer.Write(row)
	}

	windowStartText := since.Format(time.RFC3339)
	windowEndText := now.Format(time.RFC3339)
	if err := writeRow([]string{
		"summary",
		windowLabel,
		windowStartText,
		windowEndText,
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		recoveryStatus,
		strconv.FormatBool(recovered),
		previousHealth,
		"",
		"",
		"",
		"",
		escalation.Level,
		strconv.FormatBool(escalation.Required),
		escalation.Reason,
		strings.Join(escalation.TriggeredRules, "|"),
		filterUserID,
		filterFormat,
		filterGuardReason,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	escalationTrendByBucket := make(map[string]AdminVideoJobFeedbackIntegrityEscalationTrendPoint, len(escalationTrend))
	for _, point := range escalationTrend {
		bucket := strings.TrimSpace(point.Bucket)
		if bucket == "" {
			continue
		}
		escalationTrendByBucket[bucket] = point
	}
	for _, item := range trend {
		escalationPoint := escalationTrendByBucket[strings.TrimSpace(item.Bucket)]
		if err := writeRow([]string{
			"trend_day",
			windowLabel,
			windowStartText,
			windowEndText,
			item.Bucket,
			strconv.FormatInt(item.Samples, 10),
			item.Health,
			strconv.Itoa(item.AlertCount),
			fmt.Sprintf("%.6f", item.OutputCoverageRate),
			fmt.Sprintf("%.6f", item.OutputResolvedRate),
			fmt.Sprintf("%.6f", item.OutputJobConsistencyRate),
			strconv.FormatInt(item.TopPickMultiHitUsers, 10),
			escalationPoint.RecoveryStatus,
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			escalationPoint.EscalationLevel,
			strconv.FormatBool(escalationPoint.EscalationRequired),
			escalationPoint.EscalationReason,
			strings.Join(escalationPoint.TriggeredRules, "|"),
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	for _, item := range escalationIncidents {
		if err := writeRow([]string{
			"escalation_incident",
			windowLabel,
			windowStartText,
			windowEndText,
			item.Bucket,
			"",
			"",
			strconv.Itoa(item.AlertCount),
			"",
			"",
			"",
			strconv.FormatInt(item.TopPickMultiHitUsers, 10),
			item.RecoveryStatus,
			"",
			"",
			"",
			"",
			"",
			"",
			item.EscalationLevel,
			strconv.FormatBool(item.EscalationRequired),
			item.EscalationReason,
			strings.Join(item.TriggeredRules, "|"),
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	for _, item := range alertCodeStats {
		if err := writeRow([]string{
			"alert_code_stat",
			windowLabel,
			windowStartText,
			windowEndText,
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			item.Code,
			strconv.Itoa(item.DaysHit),
			item.LatestBucket,
			item.LatestLevel,
			"",
			"",
			"",
			"",
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("video_jobs_feedback_integrity_trend_%s_%s.csv", windowLabel, now.Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// GetAdminVideoJobsFeedbackIntegrityDrilldown godoc
// @Summary Get feedback-integrity drilldown rows (admin)
// @Tags admin
// @Produce json
// @Param window query string false "window: 24h | 7d | 30d"
// @Param user_id query int false "user id filter"
// @Param format query string false "requested format filter"
// @Param guard_reason query string false "negative guard reason filter"
// @Param limit query int false "rows per section, default 20, max 100"
// @Success 200 {object} AdminVideoJobFeedbackIntegrityDrilldownResponse
// @Router /api/admin/video-jobs/feedback-integrity/drilldown [get]
func (h *Handler) GetAdminVideoJobsFeedbackIntegrityDrilldown(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter, err := parseVideoImageFeedbackFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		v, parseErr := strconv.Atoi(raw)
		if parseErr != nil || v <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		if v > 100 {
			v = 100
		}
		limit = v
	}

	now := time.Now()
	since := now.Add(-windowDuration)
	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQL(filter)

	type anomalyRow struct {
		JobID            uint64     `gorm:"column:job_id"`
		UserID           uint64     `gorm:"column:user_id"`
		Title            string     `gorm:"column:title"`
		Status           string     `gorm:"column:status"`
		Stage            string     `gorm:"column:stage"`
		AnomalyCount     int64      `gorm:"column:anomaly_count"`
		LatestFeedbackAt *time.Time `gorm:"column:latest_feedback_at"`
	}
	anomalyQuery := `
WITH base AS (
	SELECT
		f.id,
		f.job_id,
		f.user_id,
		f.output_id,
		f.created_at
	FROM public.video_image_feedback f
	WHERE f.created_at >= ?
` + filterSQL + `
),
joined AS (
	SELECT
		b.*,
		o.id AS output_exists_id,
		o.job_id AS output_job_id
	FROM base b
	LEFT JOIN public.video_image_outputs o ON o.id = b.output_id
)
SELECT
	j.id::bigint AS job_id,
	j.user_id::bigint AS user_id,
	COALESCE(j.title, '') AS title,
	COALESCE(j.status, '') AS status,
	COALESCE(j.stage, '') AS stage,
	COUNT(*)::bigint AS anomaly_count,
	MAX(joined.created_at) AS latest_feedback_at
FROM joined
JOIN public.video_image_jobs j ON j.id = joined.job_id
WHERE joined.output_id IS NULL
	OR (joined.output_id IS NOT NULL AND joined.output_exists_id IS NULL)
	OR (joined.output_id IS NOT NULL AND joined.output_exists_id IS NOT NULL AND joined.output_job_id <> joined.job_id)
GROUP BY j.id, j.user_id, j.title, j.status, j.stage
ORDER BY anomaly_count DESC, latest_feedback_at DESC, j.id DESC
LIMIT ?
`
	anomalyArgs := []interface{}{since}
	anomalyArgs = append(anomalyArgs, filterArgs...)
	anomalyArgs = append(anomalyArgs, limit)
	var anomalyRows []anomalyRow
	if err := h.db.Raw(anomalyQuery, anomalyArgs...).Scan(&anomalyRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type topPickConflictRow struct {
		JobID                  uint64     `gorm:"column:job_id"`
		UserID                 uint64     `gorm:"column:user_id"`
		Title                  string     `gorm:"column:title"`
		Status                 string     `gorm:"column:status"`
		Stage                  string     `gorm:"column:stage"`
		TopPickConflictUsers   int64      `gorm:"column:top_pick_conflict_users"`
		TopPickConflictActions int64      `gorm:"column:top_pick_conflict_actions"`
		LatestFeedbackAt       *time.Time `gorm:"column:latest_feedback_at"`
	}
	topPickQuery := `
WITH base AS (
	SELECT
		f.job_id,
		f.user_id,
		LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action,
		f.created_at
	FROM public.video_image_feedback f
	WHERE f.created_at >= ?
` + filterSQL + `
),
conflict AS (
	SELECT
		job_id,
		user_id,
		COUNT(*)::bigint AS conflict_count,
		MAX(created_at) AS latest_feedback_at
	FROM base
	WHERE action = 'top_pick'
	GROUP BY job_id, user_id
	HAVING COUNT(*) > 1
),
agg AS (
	SELECT
		job_id,
		COUNT(*)::bigint AS top_pick_conflict_users,
		COALESCE(SUM(conflict_count), 0)::bigint AS top_pick_conflict_actions,
		MAX(latest_feedback_at) AS latest_feedback_at
	FROM conflict
	GROUP BY job_id
)
SELECT
	j.id::bigint AS job_id,
	j.user_id::bigint AS user_id,
	COALESCE(j.title, '') AS title,
	COALESCE(j.status, '') AS status,
	COALESCE(j.stage, '') AS stage,
	agg.top_pick_conflict_users,
	agg.top_pick_conflict_actions,
	agg.latest_feedback_at
FROM agg
JOIN public.video_image_jobs j ON j.id = agg.job_id
ORDER BY agg.top_pick_conflict_users DESC, agg.top_pick_conflict_actions DESC, agg.latest_feedback_at DESC, agg.job_id DESC
LIMIT ?
`
	topPickArgs := []interface{}{since}
	topPickArgs = append(topPickArgs, filterArgs...)
	topPickArgs = append(topPickArgs, limit)
	var topPickRows []topPickConflictRow
	if err := h.db.Raw(topPickQuery, topPickArgs...).Scan(&topPickRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	formatOptionalTime := func(value *time.Time) string {
		if value == nil || value.IsZero() {
			return ""
		}
		return value.Format(time.RFC3339)
	}
	anomalyJobs := make([]AdminVideoJobFeedbackIntegrityDrilldownRow, 0, len(anomalyRows))
	for _, item := range anomalyRows {
		anomalyJobs = append(anomalyJobs, AdminVideoJobFeedbackIntegrityDrilldownRow{
			JobID:            item.JobID,
			UserID:           item.UserID,
			Title:            strings.TrimSpace(item.Title),
			Status:           strings.TrimSpace(item.Status),
			Stage:            strings.TrimSpace(item.Stage),
			AnomalyCount:     item.AnomalyCount,
			LatestFeedbackAt: formatOptionalTime(item.LatestFeedbackAt),
		})
	}
	topPickJobs := make([]AdminVideoJobFeedbackIntegrityDrilldownRow, 0, len(topPickRows))
	for _, item := range topPickRows {
		topPickJobs = append(topPickJobs, AdminVideoJobFeedbackIntegrityDrilldownRow{
			JobID:                  item.JobID,
			UserID:                 item.UserID,
			Title:                  strings.TrimSpace(item.Title),
			Status:                 strings.TrimSpace(item.Status),
			Stage:                  strings.TrimSpace(item.Stage),
			TopPickConflictUsers:   item.TopPickConflictUsers,
			TopPickConflictActions: item.TopPickConflictActions,
			LatestFeedbackAt:       formatOptionalTime(item.LatestFeedbackAt),
		})
	}

	response := AdminVideoJobFeedbackIntegrityDrilldownResponse{
		Window:              windowLabel,
		WindowStart:         since.Format(time.RFC3339),
		WindowEnd:           now.Format(time.RFC3339),
		AnomalyJobs:         anomalyJobs,
		TopPickConflictJobs: topPickJobs,
	}
	if filter != nil {
		response.FilterUserID = filter.UserID
		response.FilterFormat = filter.Format
		response.FilterGuardReason = filter.GuardReason
	}

	c.JSON(http.StatusOK, response)
}

// ExportAdminVideoJobsFeedbackIntegrityAnomaliesCSV godoc
// @Summary Export video-jobs feedback integrity anomalies CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param user_id query int false "user id filter"
// @Param format query string false "requested format filter"
// @Param guard_reason query string false "negative guard reason filter"
// @Param limit query int false "max rows per section, default 300, max 2000"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/feedback-integrity-anomalies.csv [get]
func (h *Handler) ExportAdminVideoJobsFeedbackIntegrityAnomaliesCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter, err := parseVideoImageFeedbackFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	limit := 300
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		v, parseErr := strconv.Atoi(raw)
		if parseErr != nil || v <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		if v > 2000 {
			v = 2000
		}
		limit = v
	}

	now := time.Now()
	since := now.Add(-windowDuration)
	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQL(filter)

	type anomalyRow struct {
		FeedbackID  uint64    `gorm:"column:feedback_id"`
		JobID       uint64    `gorm:"column:job_id"`
		UserID      uint64    `gorm:"column:user_id"`
		OutputID    *uint64   `gorm:"column:output_id"`
		Action      string    `gorm:"column:action"`
		Anomaly     string    `gorm:"column:anomaly"`
		OutputJobID *uint64   `gorm:"column:output_job_id"`
		ObjectKey   string    `gorm:"column:object_key"`
		CreatedAt   time.Time `gorm:"column:created_at"`
	}
	anomalyQuery := `
WITH base AS (
	SELECT
		f.id,
		f.job_id,
		f.user_id,
		f.output_id,
		LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action,
		f.created_at
	FROM public.video_image_feedback f
	WHERE f.created_at >= ?
` + filterSQL + `
),
joined AS (
	SELECT
		b.*,
		o.id AS output_exists_id,
		o.job_id AS output_job_id,
		o.object_key
	FROM base b
	LEFT JOIN public.video_image_outputs o ON o.id = b.output_id
)
SELECT
	id::bigint AS feedback_id,
	job_id::bigint AS job_id,
	user_id::bigint AS user_id,
	output_id::bigint AS output_id,
	action,
	CASE
		WHEN output_id IS NULL THEN 'missing_output_id'
		WHEN output_id IS NOT NULL AND output_exists_id IS NULL THEN 'orphan_output'
		WHEN output_id IS NOT NULL AND output_exists_id IS NOT NULL AND output_job_id <> job_id THEN 'job_mismatch'
		ELSE 'ok'
	END AS anomaly,
	output_job_id::bigint AS output_job_id,
	COALESCE(object_key, '') AS object_key,
	created_at
FROM joined
WHERE output_id IS NULL
	OR (output_id IS NOT NULL AND output_exists_id IS NULL)
	OR (output_id IS NOT NULL AND output_exists_id IS NOT NULL AND output_job_id <> job_id)
ORDER BY created_at DESC
LIMIT ?
`
	anomalyArgs := []interface{}{since}
	anomalyArgs = append(anomalyArgs, filterArgs...)
	anomalyArgs = append(anomalyArgs, limit)

	var anomalyRows []anomalyRow
	if err := h.db.Raw(anomalyQuery, anomalyArgs...).Scan(&anomalyRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type topPickConflictRow struct {
		JobID         uint64    `gorm:"column:job_id"`
		UserID        uint64    `gorm:"column:user_id"`
		ConflictCount int64     `gorm:"column:conflict_count"`
		CreatedAt     time.Time `gorm:"column:created_at"`
	}
	topPickQuery := `
WITH base AS (
	SELECT
		f.job_id,
		f.user_id,
		LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action,
		f.created_at
	FROM public.video_image_feedback f
	WHERE f.created_at >= ?
` + filterSQL + `
)
SELECT
	job_id::bigint AS job_id,
	user_id::bigint AS user_id,
	COUNT(*)::bigint AS conflict_count,
	MAX(created_at) AS created_at
FROM base
WHERE action = 'top_pick'
GROUP BY job_id, user_id
HAVING COUNT(*) > 1
ORDER BY conflict_count DESC, created_at DESC
LIMIT ?
`
	topPickArgs := []interface{}{since}
	topPickArgs = append(topPickArgs, filterArgs...)
	topPickArgs = append(topPickArgs, limit)

	var topPickRows []topPickConflictRow
	if err := h.db.Raw(topPickQuery, topPickArgs...).Scan(&topPickRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filterUserID := ""
	filterFormat := ""
	filterGuardReason := ""
	if filter != nil {
		if filter.UserID > 0 {
			filterUserID = strconv.FormatUint(filter.UserID, 10)
		}
		filterFormat = filter.Format
		filterGuardReason = filter.GuardReason
	}
	formatOptionalUint := func(value *uint64) string {
		if value == nil || *value == 0 {
			return ""
		}
		return strconv.FormatUint(*value, 10)
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{
		"section",
		"window",
		"window_start",
		"window_end",
		"feedback_id",
		"job_id",
		"user_id",
		"output_id",
		"action",
		"anomaly",
		"output_job_id",
		"object_key",
		"conflict_count",
		"created_at",
		"filter_user_id",
		"filter_format",
		"filter_guard_reason",
	}
	if err := writer.Write(header); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	writeRow := func(row []string) error {
		if len(row) < len(header) {
			row = append(row, make([]string, len(header)-len(row))...)
		}
		return writer.Write(row)
	}

	windowStartText := since.Format(time.RFC3339)
	windowEndText := now.Format(time.RFC3339)
	for _, item := range anomalyRows {
		if err := writeRow([]string{
			"feedback_anomaly",
			windowLabel,
			windowStartText,
			windowEndText,
			strconv.FormatUint(item.FeedbackID, 10),
			strconv.FormatUint(item.JobID, 10),
			strconv.FormatUint(item.UserID, 10),
			formatOptionalUint(item.OutputID),
			item.Action,
			item.Anomaly,
			formatOptionalUint(item.OutputJobID),
			item.ObjectKey,
			"",
			item.CreatedAt.Format(time.RFC3339),
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	for _, item := range topPickRows {
		if err := writeRow([]string{
			"top_pick_conflict",
			windowLabel,
			windowStartText,
			windowEndText,
			"",
			strconv.FormatUint(item.JobID, 10),
			strconv.FormatUint(item.UserID, 10),
			"",
			"top_pick",
			"top_pick_conflict",
			"",
			"",
			strconv.FormatInt(item.ConflictCount, 10),
			item.CreatedAt.Format(time.RFC3339),
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("video_jobs_feedback_integrity_anomalies_%s_%s.csv", windowLabel, now.Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// GetAdminSampleVideoJobsBaselineDiff godoc
// @Summary Get sample video-jobs baseline diff (admin)
// @Tags admin
// @Produce json
// @Param base_window query string false "reference window: 24h | 7d | 30d (default: 7d)"
// @Param target_window query string false "target window: 24h | 7d | 30d (default: 24h)"
// @Success 200 {object} AdminSampleVideoJobsBaselineDiffResponse
// @Router /api/admin/video-jobs/samples/baseline-diff [get]
func (h *Handler) GetAdminSampleVideoJobsBaselineDiff(c *gin.Context) {
	baseLabel, baseDuration, err := parseVideoJobsOverviewWindow(c.DefaultQuery("base_window", "7d"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base_window"})
		return
	}
	targetLabel, targetDuration, err := parseVideoJobsOverviewWindow(c.DefaultQuery("target_window", "24h"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_window"})
		return
	}

	now := time.Now()
	baseSince := now.Add(-baseDuration)
	targetSince := now.Add(-targetDuration)

	baseSummary, err := h.loadSampleVideoJobsWindowSummary(baseSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetSummary, err := h.loadSampleVideoJobsWindowSummary(targetSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	baseRows, err := h.loadSampleVideoJobFormatBaselineRows(baseSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetRows, err := h.loadSampleVideoJobFormatBaselineRows(targetSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	summary := buildSampleVideoJobsDiffSummary(baseSummary, targetSummary)
	formatDiffRows := buildSampleVideoJobFormatDiffRows(baseRows, targetRows)
	formats := make([]AdminSampleVideoJobsBaselineDiffFormatStat, 0, len(formatDiffRows))
	for _, row := range formatDiffRows {
		formats = append(formats, AdminSampleVideoJobsBaselineDiffFormatStat{
			Format:                     row.Format,
			BaseRequestedJobs:          row.BaseRequestedJobs,
			TargetRequestedJobs:        row.TargetRequestedJobs,
			BaseGeneratedJobs:          row.BaseGeneratedJobs,
			TargetGeneratedJobs:        row.TargetGeneratedJobs,
			BaseSuccessRate:            row.BaseSuccessRate,
			TargetSuccessRate:          row.TargetSuccessRate,
			SuccessRateDelta:           row.SuccessRateDelta,
			SuccessRateUplift:          row.SuccessRateUplift,
			BaseAvgArtifactSizeBytes:   row.BaseAvgArtifactSizeBytes,
			TargetAvgArtifactSizeBytes: row.TargetAvgArtifactSizeBytes,
			AvgArtifactSizeDelta:       row.AvgArtifactSizeDelta,
			AvgArtifactSizeUplift:      row.AvgArtifactSizeUplift,
			BaseDurationP50Sec:         row.BaseDurationP50Sec,
			TargetDurationP50Sec:       row.TargetDurationP50Sec,
			DurationP50Delta:           row.DurationP50Delta,
			DurationP50Uplift:          row.DurationP50Uplift,
			BaseDurationP95Sec:         row.BaseDurationP95Sec,
			TargetDurationP95Sec:       row.TargetDurationP95Sec,
			DurationP95Delta:           row.DurationP95Delta,
			DurationP95Uplift:          row.DurationP95Uplift,
		})
	}

	c.JSON(http.StatusOK, AdminSampleVideoJobsBaselineDiffResponse{
		BaseWindow:   baseLabel,
		TargetWindow: targetLabel,
		GeneratedAt:  now.Format(time.RFC3339),
		Summary:      summary,
		Formats:      formats,
	})
}

// ListAdminVideoJobs godoc
// @Summary List video jobs (admin)
// @Tags admin
// @Produce json
// @Param page query int false "page"
// @Param page_size query int false "page_size"
// @Param user_id query int false "user id"
// @Param status query string false "job status"
// @Param format query string false "requested format"
// @Param guard_reason query string false "negative guard reason filter"
// @Param stage query string false "job stage"
// @Param quick query string false "quick filter: retrying | failed_24h | guard_hit | guard_blocked | feedback_anomaly | top_pick_conflict"
// @Param is_sample query string false "sample filter: all | 1 | 0"
// @Param q query string false "title/source search"
// @Success 200 {object} AdminVideoJobListResponse
// @Router /api/admin/video-jobs [get]
func (h *Handler) ListAdminVideoJobs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := h.db.Model(&models.VideoImageJobPublic{})
	if quick := strings.ToLower(strings.TrimSpace(c.Query("quick"))); quick != "" {
		quickSince := time.Now().Add(-24 * time.Hour)
		switch quick {
		case "retrying":
			query = query.Where("stage = ?", models.VideoJobStageRetrying)
		case "failed_24h":
			query = query.Where("status = ? AND updated_at >= ?", models.VideoJobStatusFailed, time.Now().Add(-24*time.Hour))
		case "guard_hit":
			query = query.Where(`
status = ?
AND LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->>'group'), ''), '')) = 'treatment'
AND jsonb_typeof(metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
AND (metrics->'highlight_feedback_v1'->'reason_negative_guard') <> '{}'::jsonb
`, models.VideoJobStatusDone)
		case "guard_blocked":
			query = query.Where(`
status = ?
AND LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->>'group'), ''), '')) = 'treatment'
AND jsonb_typeof(metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
AND (metrics->'highlight_feedback_v1'->'reason_negative_guard') <> '{}'::jsonb
AND LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), '')) <> ''
AND LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->'selected_after'->>'reason'), ''), '')) <> ''
AND LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->'selected_after'->>'reason'), ''), '')) <>
	LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), ''))
AND (metrics->'highlight_feedback_v1'->'reason_negative_guard' ? LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), '')))
`, models.VideoJobStatusDone)
		case "feedback_anomaly":
			query = query.Where(`
EXISTS (
	SELECT 1
	FROM public.video_image_feedback f
	LEFT JOIN public.video_image_outputs o ON o.id = f.output_id
	WHERE f.job_id = public.video_image_jobs.id
		AND f.created_at >= ?
		AND (
			f.output_id IS NULL
			OR (f.output_id IS NOT NULL AND o.id IS NULL)
			OR (f.output_id IS NOT NULL AND o.id IS NOT NULL AND o.job_id <> f.job_id)
		)
)
`, quickSince)
		case "top_pick_conflict":
			query = query.Where(`
EXISTS (
	SELECT 1
	FROM public.video_image_feedback f
	WHERE f.job_id = public.video_image_jobs.id
		AND f.created_at >= ?
		AND LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) = 'top_pick'
	GROUP BY f.job_id, f.user_id
	HAVING COUNT(*) > 1
)
`, quickSince)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid quick"})
			return
		}
	}
	if userIDRaw := strings.TrimSpace(c.Query("user_id")); userIDRaw != "" {
		userID, err := strconv.ParseUint(userIDRaw, 10, 64)
		if err != nil || userID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
			return
		}
		query = query.Where("user_id = ?", userID)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if format := strings.ToLower(strings.TrimSpace(c.Query("format"))); format != "" {
		query = query.Where(buildVideoJobFormatFilterPredicate("video_image_jobs"), format, format)
	}
	if guardReason := strings.ToLower(strings.TrimSpace(c.Query("guard_reason"))); guardReason != "" {
		query = query.Where(`
EXISTS (
	SELECT 1
	FROM jsonb_each_text(
		CASE
			WHEN jsonb_typeof(metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
			THEN metrics->'highlight_feedback_v1'->'reason_negative_guard'
			ELSE '{}'::jsonb
		END
	) AS guard_reason(key, value)
	WHERE LOWER(TRIM(guard_reason.key)) = ?
)`, guardReason)
	}
	if stage := strings.TrimSpace(c.Query("stage")); stage != "" {
		query = query.Where("stage = ?", stage)
	}
	sampleRaw := strings.TrimSpace(c.Query("is_sample"))
	if sampleRaw == "" {
		sampleRaw = strings.TrimSpace(c.Query("sample"))
	}
	sampleFilter, ok := parseOptionalBoolParam(sampleRaw)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid is_sample"})
		return
	}
	if sampleFilter != nil {
		if *sampleFilter {
			query = query.Where(`
EXISTS (
	SELECT 1
	FROM archive.video_jobs v
	JOIN archive.collections c ON c.id = v.result_collection_id
	WHERE v.id = public.video_image_jobs.id
	  AND c.is_sample = TRUE
)`)
		} else {
			query = query.Where(`
NOT EXISTS (
	SELECT 1
	FROM archive.video_jobs v
	JOIN archive.collections c ON c.id = v.result_collection_id
	WHERE v.id = public.video_image_jobs.id
	  AND c.is_sample = TRUE
)`)
		}
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		like := "%" + q + "%"
		query = query.Where("title ILIKE ? OR source_video_key ILIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []models.VideoImageJobPublic
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	jobs := make([]models.VideoJob, 0, len(rows))
	for _, row := range rows {
		jobs = append(jobs, convertPublicVideoImageJobToLegacy(row))
	}

	userMap := h.loadVideoJobUserMap(jobs)
	collectionMap := h.loadVideoJobCollectionMap(jobs)
	costMap := h.loadVideoJobCostMap(jobs)
	pointHoldMap := h.loadVideoJobPointHoldMap(jobs)

	items := make([]AdminVideoJobListItem, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, h.buildAdminVideoJobListItem(job, userMap, collectionMap, costMap, pointHoldMap))
	}

	c.JSON(http.StatusOK, AdminVideoJobListResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// GetAdminVideoJob godoc
// @Summary Get video job detail (admin)
// @Tags admin
// @Produce json
// @Param id path int true "job id"
// @Param review_status query string false "comma-separated review statuses: deliver,keep_internal,reject,need_manual_review"
// @Success 200 {object} AdminVideoJobDetailResponse
// @Router /api/admin/video-jobs/{id} [get]
func (h *Handler) GetAdminVideoJob(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	reviewStatusFilter := parseVideoJobReviewStatusFilter(c.Query("review_status"))
	reviewStatusSet := buildVideoJobReviewStatusSet(reviewStatusFilter)

	var publicJob models.VideoImageJobPublic
	if err := h.db.First(&publicJob, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var legacy models.VideoJob
			if lerr := h.db.First(&legacy, id).Error; lerr != nil {
				if errors.Is(lerr, gorm.ErrRecordNotFound) {
					c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": lerr.Error()})
				return
			}
			publicJob = models.VideoImageJobPublic{
				ID:             legacy.ID,
				UserID:         legacy.UserID,
				Title:          legacy.Title,
				SourceVideoKey: legacy.SourceVideoKey,
				RequestedFormat: func() string {
					if f := strings.TrimSpace(legacy.OutputFormats); f != "" {
						parts := strings.Split(f, ",")
						if len(parts) > 0 {
							return strings.TrimSpace(parts[0])
						}
					}
					return "gif"
				}(),
				Status:       legacy.Status,
				Stage:        legacy.Stage,
				Progress:     legacy.Progress,
				Options:      legacy.Options,
				Metrics:      legacy.Metrics,
				ErrorMessage: legacy.ErrorMessage,
				StartedAt:    legacy.StartedAt,
				FinishedAt:   legacy.FinishedAt,
				CreatedAt:    legacy.CreatedAt,
				UpdatedAt:    legacy.UpdatedAt,
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	job := convertPublicVideoImageJobToLegacy(publicJob)

	userMap := h.loadVideoJobUserMap([]models.VideoJob{job})
	collectionMap := h.loadVideoJobCollectionMap([]models.VideoJob{job})
	costMap := h.loadVideoJobCostMap([]models.VideoJob{job})
	pointHoldMap := h.loadVideoJobPointHoldMap([]models.VideoJob{job})
	jobItem := h.buildAdminVideoJobListItem(job, userMap, collectionMap, costMap, pointHoldMap)

	var events []models.VideoImageEventPublic
	if err := h.db.Where("job_id = ?", job.ID).Order("id DESC").Limit(200).Find(&events).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	respEvents := make([]AdminVideoJobEventItem, 0, len(events))
	for _, item := range events {
		respEvents = append(respEvents, AdminVideoJobEventItem{
			ID:        item.ID,
			Stage:     item.Stage,
			Level:     item.Level,
			Message:   item.Message,
			Metadata:  parseJSONMap(item.Metadata),
			CreatedAt: item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var artifacts []models.VideoImageOutputPublic
	if err := h.db.Where("job_id = ?", job.ID).Order("id DESC").Limit(200).Find(&artifacts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	respArtifacts := make([]AdminVideoJobArtifactItem, 0, len(artifacts))
	for _, item := range artifacts {
		respArtifacts = append(respArtifacts, AdminVideoJobArtifactItem{
			ID:         item.ID,
			Type:       item.FileRole,
			QiniuKey:   item.ObjectKey,
			URL:        resolvePreviewURL(item.ObjectKey, h.qiniu),
			MimeType:   item.MimeType,
			SizeBytes:  item.SizeBytes,
			Width:      item.Width,
			Height:     item.Height,
			DurationMs: item.DurationMs,
			Metadata:   parseJSONMap(item.Metadata),
			CreatedAt:  item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var gifCandidates []models.VideoJobGIFCandidate
	if err := h.db.Where("job_id = ?", job.ID).
		Order("is_selected DESC, final_rank ASC, base_score DESC, id ASC").
		Limit(300).
		Find(&gifCandidates).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_gif_candidates") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	respCandidates := make([]AdminVideoJobGIFCandidateItem, 0, len(gifCandidates))
	for _, item := range gifCandidates {
		respCandidates = append(respCandidates, AdminVideoJobGIFCandidateItem{
			ID:              item.ID,
			StartMs:         item.StartMs,
			EndMs:           item.EndMs,
			DurationMs:      item.DurationMs,
			BaseScore:       item.BaseScore,
			ConfidenceScore: item.ConfidenceScore,
			FinalRank:       item.FinalRank,
			IsSelected:      item.IsSelected,
			RejectReason:    strings.TrimSpace(item.RejectReason),
			FeatureJSON:     parseJSONMap(item.FeatureJSON),
			CreatedAt:       item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var aiUsageRows []models.VideoJobAIUsage
	if err := h.db.Where("job_id = ?", job.ID).
		Order("id DESC").
		Limit(300).
		Find(&aiUsageRows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "ops.video_job_ai_usage") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	respAIUsages := make([]AdminVideoJobAIUsageItem, 0, len(aiUsageRows))
	for _, item := range aiUsageRows {
		respAIUsages = append(respAIUsages, AdminVideoJobAIUsageItem{
			ID:                item.ID,
			Stage:             strings.TrimSpace(item.Stage),
			Provider:          strings.TrimSpace(item.Provider),
			Model:             strings.TrimSpace(item.Model),
			Endpoint:          strings.TrimSpace(item.Endpoint),
			RequestStatus:     strings.TrimSpace(item.RequestStatus),
			RequestError:      strings.TrimSpace(item.RequestError),
			RequestDurationMs: item.RequestDurationMs,
			InputTokens:       item.InputTokens,
			OutputTokens:      item.OutputTokens,
			CachedInputTokens: item.CachedInputTokens,
			ImageTokens:       item.ImageTokens,
			VideoTokens:       item.VideoTokens,
			AudioSeconds:      item.AudioSeconds,
			CostUSD:           item.CostUSD,
			Currency:          strings.TrimSpace(item.Currency),
			PricingVersion:    strings.TrimSpace(item.PricingVersion),
			PricingSourceURL:  strings.TrimSpace(item.PricingSourceURL),
			Metadata:          parseJSONMap(item.Metadata),
			CreatedAt:         item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var aiDirectiveRows []models.VideoJobGIFAIDirective
	if err := h.db.Where("job_id = ?", job.ID).
		Order("id DESC").
		Limit(80).
		Find(&aiDirectiveRows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_gif_ai_directives") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	respAIDirectives := make([]AdminVideoJobAIGIFDirectiveItem, 0, len(aiDirectiveRows))
	for _, item := range aiDirectiveRows {
		respAIDirectives = append(respAIDirectives, AdminVideoJobAIGIFDirectiveItem{
			ID:                 item.ID,
			BusinessGoal:       strings.TrimSpace(item.BusinessGoal),
			Audience:           strings.TrimSpace(item.Audience),
			MustCapture:        parseStringSliceJSON(item.MustCapture),
			Avoid:              parseStringSliceJSON(item.Avoid),
			ClipCountMin:       item.ClipCountMin,
			ClipCountMax:       item.ClipCountMax,
			DurationPrefMinSec: item.DurationPrefMinSec,
			DurationPrefMaxSec: item.DurationPrefMaxSec,
			LoopPreference:     item.LoopPreference,
			StyleDirection:     strings.TrimSpace(item.StyleDirection),
			RiskFlags:          parseStringSliceJSON(item.RiskFlags),
			QualityWeights:     parseJSONMap(item.QualityWeights),
			BriefVersion:       strings.TrimSpace(item.BriefVersion),
			ModelVersion:       strings.TrimSpace(item.ModelVersion),
			DirectiveText:      strings.TrimSpace(item.DirectiveText),
			Status:             strings.TrimSpace(item.Status),
			FallbackUsed:       item.FallbackUsed,
			InputContext:       parseJSONMap(item.InputContextJSON),
			Provider:           strings.TrimSpace(item.Provider),
			Model:              strings.TrimSpace(item.Model),
			PromptVersion:      strings.TrimSpace(item.PromptVersion),
			Metadata:           parseJSONMap(item.Metadata),
			CreatedAt:          item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var aiProposalRows []models.VideoJobGIFAIProposal
	if err := h.db.Where("job_id = ?", job.ID).
		Order("proposal_rank ASC, id ASC").
		Limit(300).
		Find(&aiProposalRows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_gif_ai_proposals") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	respAIProposals := make([]AdminVideoJobAIGIFProposalItem, 0, len(aiProposalRows))
	for _, item := range aiProposalRows {
		respAIProposals = append(respAIProposals, AdminVideoJobAIGIFProposalItem{
			ID:                   item.ID,
			ProposalRank:         item.ProposalRank,
			StartSec:             item.StartSec,
			EndSec:               item.EndSec,
			DurationSec:          item.DurationSec,
			BaseScore:            item.BaseScore,
			ProposalReason:       strings.TrimSpace(item.ProposalReason),
			SemanticTags:         parseStringSliceJSON(item.SemanticTags),
			ExpectedValueLevel:   strings.TrimSpace(item.ExpectedValueLevel),
			StandaloneConfidence: item.StandaloneConfidence,
			LoopFriendlinessHint: item.LoopFriendlinessHint,
			Status:               strings.TrimSpace(item.Status),
			Provider:             strings.TrimSpace(item.Provider),
			Model:                strings.TrimSpace(item.Model),
			PromptVersion:        strings.TrimSpace(item.PromptVersion),
			Metadata:             parseJSONMap(item.Metadata),
			CreatedAt:            item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var aiReviewRows []models.VideoJobGIFAIReview
	if err := h.db.Where("job_id = ?", job.ID).
		Order("id DESC").
		Limit(300).
		Find(&aiReviewRows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_gif_ai_reviews") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	reviewStatusCounts := map[string]int64{
		"deliver":            0,
		"keep_internal":      0,
		"reject":             0,
		"need_manual_review": 0,
	}
	respAIReviews := make([]AdminVideoJobAIGIFReviewItem, 0, len(aiReviewRows))
	for _, item := range aiReviewRows {
		status := normalizeVideoJobReviewStatus(item.FinalRecommendation)
		statusLabel := status
		if statusLabel == "" {
			statusLabel = strings.TrimSpace(item.FinalRecommendation)
		}
		if status != "" {
			reviewStatusCounts[status]++
		}
		if reviewStatusSet != nil {
			if _, ok := reviewStatusSet[status]; !ok {
				continue
			}
		}
		respAIReviews = append(respAIReviews, AdminVideoJobAIGIFReviewItem{
			ID:                  item.ID,
			OutputID:            item.OutputID,
			ProposalID:          item.ProposalID,
			FinalRecommendation: statusLabel,
			SemanticVerdict:     item.SemanticVerdict,
			DiagnosticReason:    strings.TrimSpace(item.DiagnosticReason),
			SuggestedAction:     strings.TrimSpace(item.SuggestedAction),
			Provider:            strings.TrimSpace(item.Provider),
			Model:               strings.TrimSpace(item.Model),
			PromptVersion:       strings.TrimSpace(item.PromptVersion),
			Metadata:            parseJSONMap(item.Metadata),
			CreatedAt:           item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(http.StatusOK, AdminVideoJobDetailResponse{
		Job:                     jobItem,
		Events:                  respEvents,
		Artifacts:               respArtifacts,
		GIFCandidates:           respCandidates,
		AIUsages:                respAIUsages,
		AIGIFDirectives:         respAIDirectives,
		AIGIFProposals:          respAIProposals,
		AIGIFReviews:            respAIReviews,
		AIGIFReviewStatusCounts: reviewStatusCounts,
		AIGIFReviewStatusFilter: reviewStatusFilter,
	})
}

func convertPublicVideoImageJobToLegacy(row models.VideoImageJobPublic) models.VideoJob {
	outputFormat := strings.TrimSpace(row.RequestedFormat)
	if outputFormat == "" {
		outputFormat = "gif"
	}
	return models.VideoJob{
		ID:             row.ID,
		UserID:         row.UserID,
		Title:          strings.TrimSpace(row.Title),
		SourceVideoKey: strings.TrimSpace(row.SourceVideoKey),
		OutputFormats:  outputFormat,
		Status:         strings.TrimSpace(row.Status),
		Stage:          strings.TrimSpace(row.Stage),
		Progress:       row.Progress,
		Priority:       "normal",
		Options:        row.Options,
		Metrics:        row.Metrics,
		ErrorMessage:   strings.TrimSpace(row.ErrorMessage),
		StartedAt:      row.StartedAt,
		FinishedAt:     row.FinishedAt,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func (h *Handler) loadVideoJobUserMap(jobs []models.VideoJob) map[uint64]models.User {
	result := map[uint64]models.User{}
	if len(jobs) == 0 {
		return result
	}
	ids := make([]uint64, 0, len(jobs))
	seen := map[uint64]struct{}{}
	for _, item := range jobs {
		if item.UserID == 0 {
			continue
		}
		if _, ok := seen[item.UserID]; ok {
			continue
		}
		seen[item.UserID] = struct{}{}
		ids = append(ids, item.UserID)
	}
	if len(ids) == 0 {
		return result
	}

	var users []models.User
	_ = h.db.Select("id", "display_name", "phone", "subscription_plan", "subscription_status", "subscription_expires_at").
		Where("id IN ?", ids).
		Find(&users).Error
	for _, item := range users {
		result[item.ID] = item
	}
	return result
}

func (h *Handler) loadVideoJobCollectionMap(jobs []models.VideoJob) map[uint64]models.Collection {
	result := map[uint64]models.Collection{}
	if len(jobs) == 0 {
		return result
	}
	ids := make([]uint64, 0, len(jobs))
	seen := map[uint64]struct{}{}
	for _, item := range jobs {
		if item.ResultCollectionID == nil || *item.ResultCollectionID == 0 {
			continue
		}
		if _, ok := seen[*item.ResultCollectionID]; ok {
			continue
		}
		seen[*item.ResultCollectionID] = struct{}{}
		ids = append(ids, *item.ResultCollectionID)
	}
	if len(ids) == 0 {
		return result
	}

	var collections []models.Collection
	_ = h.db.Select("id", "title", "cover_url", "status").Where("id IN ?", ids).Find(&collections).Error
	for _, item := range collections {
		result[item.ID] = item
	}
	return result
}

func (h *Handler) loadVideoJobCostMap(jobs []models.VideoJob) map[uint64]models.VideoJobCost {
	result := map[uint64]models.VideoJobCost{}
	if len(jobs) == 0 {
		return result
	}
	ids := make([]uint64, 0, len(jobs))
	seen := map[uint64]struct{}{}
	for _, item := range jobs {
		if item.ID == 0 {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return result
	}

	var costs []models.VideoJobCost
	if err := h.db.Where("job_id IN ?", ids).Find(&costs).Error; err != nil {
		return result
	}
	for _, item := range costs {
		result[item.JobID] = item
	}
	return result
}

func (h *Handler) loadVideoJobPointHoldMap(jobs []models.VideoJob) map[uint64]models.ComputePointHold {
	result := map[uint64]models.ComputePointHold{}
	if len(jobs) == 0 {
		return result
	}
	ids := make([]uint64, 0, len(jobs))
	seen := map[uint64]struct{}{}
	for _, item := range jobs {
		if item.ID == 0 {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return result
	}

	var holds []models.ComputePointHold
	if err := h.db.Where("job_id IN ?", ids).Find(&holds).Error; err != nil {
		return result
	}
	for _, item := range holds {
		result[item.JobID] = item
	}
	return result
}

func (h *Handler) buildAdminVideoJobListItem(
	job models.VideoJob,
	userMap map[uint64]models.User,
	collectionMap map[uint64]models.Collection,
	costMap map[uint64]models.VideoJobCost,
	pointHoldMap map[uint64]models.ComputePointHold,
) AdminVideoJobListItem {
	user := userMap[job.UserID]
	userLevel := "free"
	if strings.EqualFold(strings.TrimSpace(user.SubscriptionStatus), "active") {
		userLevel = "subscriber"
	}

	var startedAt *string
	if job.StartedAt != nil {
		formatted := job.StartedAt.Format("2006-01-02 15:04:05")
		startedAt = &formatted
	}
	var finishedAt *string
	if job.FinishedAt != nil {
		formatted := job.FinishedAt.Format("2006-01-02 15:04:05")
		finishedAt = &formatted
	}

	var collection *AdminVideoJobCollection
	if job.ResultCollectionID != nil && *job.ResultCollectionID > 0 {
		if col, ok := collectionMap[*job.ResultCollectionID]; ok {
			collection = &AdminVideoJobCollection{
				ID:       col.ID,
				Title:    col.Title,
				CoverURL: resolvePreviewURL(col.CoverURL, h.qiniu),
				Status:   col.Status,
				IsSample: col.IsSample,
			}
		}
	}
	var cost *AdminVideoJobCost
	if item, ok := costMap[job.ID]; ok {
		details := parseJSONMap(item.Details)
		cost = &AdminVideoJobCost{
			EstimatedCost:      item.EstimatedCost,
			Currency:           strings.TrimSpace(item.Currency),
			PricingVersion:     strings.TrimSpace(item.PricingVersion),
			CPUms:              item.CPUms,
			GPUms:              item.GPUms,
			StorageBytesRaw:    item.StorageBytesRaw,
			StorageBytesOutput: item.StorageBytesOutput,
			OutputCount:        item.OutputCount,
			AICostUSD:          parseFloat64FromAny(details["ai_cost_usd"]),
			AICostCNY:          parseFloat64FromAny(details["ai_cost_cny"]),
			AICalls:            parseInt64FromAny(details["ai_usage_calls"]),
			AIErrorCalls:       parseInt64FromAny(details["ai_usage_error_calls"]),
			AIDurationMs:       parseInt64FromAny(details["ai_usage_duration_ms"]),
			AIInputTokens:      parseInt64FromAny(details["ai_input_tokens"]),
			AIOutputTokens:     parseInt64FromAny(details["ai_output_tokens"]),
			AICachedInput:      parseInt64FromAny(details["ai_cached_input_tokens"]),
			AIImageTokens:      parseInt64FromAny(details["ai_image_tokens"]),
			AIVideoTokens:      parseInt64FromAny(details["ai_video_tokens"]),
			AIAudioSeconds:     parseFloat64FromAny(details["ai_audio_seconds"]),
			USDtoCNYRate:       parseFloat64FromAny(details["usd_to_cny_rate"]),
		}
	}
	var pointHold *AdminVideoJobPointHold
	if item, ok := pointHoldMap[job.ID]; ok {
		pointHold = &AdminVideoJobPointHold{
			Status:         strings.TrimSpace(item.Status),
			ReservedPoints: item.ReservedPoints,
			SettledPoints:  item.SettledPoints,
		}
	}

	return AdminVideoJobListItem{
		ID:                 job.ID,
		Title:              job.Title,
		SourceVideoKey:     job.SourceVideoKey,
		SourceVideoURL:     resolvePreviewURL(job.SourceVideoKey, h.qiniu),
		CategoryID:         job.CategoryID,
		OutputFormats:      splitFormats(job.OutputFormats),
		Status:             job.Status,
		Stage:              job.Stage,
		Progress:           job.Progress,
		Priority:           job.Priority,
		ErrorMessage:       strings.TrimSpace(job.ErrorMessage),
		ResultCollectionID: job.ResultCollectionID,
		Options:            parseJSONMap(job.Options),
		Metrics:            parseJSONMap(job.Metrics),
		Cost:               cost,
		PointHold:          pointHold,
		User: AdminVideoJobUser{
			ID:                 user.ID,
			DisplayName:        strings.TrimSpace(user.DisplayName),
			Phone:              strings.TrimSpace(user.Phone),
			UserLevel:          userLevel,
			SubscriptionPlan:   strings.TrimSpace(user.SubscriptionPlan),
			SubscriptionStatus: strings.TrimSpace(user.SubscriptionStatus),
		},
		Collection: collection,
		QueuedAt:   job.QueuedAt.Format("2006-01-02 15:04:05"),
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		CreatedAt:  job.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:  job.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

func parseInt64FromAny(raw interface{}) int64 {
	switch value := raw.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case int32:
		return int64(value)
	case uint64:
		return int64(value)
	case uint32:
		return int64(value)
	case float64:
		return int64(value)
	case float32:
		return int64(value)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func parseFloat64FromAny(raw interface{}) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int64:
		return float64(value)
	case int:
		return float64(value)
	case uint64:
		return float64(value)
	case uint32:
		return float64(value)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func parseStringSliceJSON(raw datatypes.JSON) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func (h *Handler) loadVideoJobSourceProbeBuckets(since time.Time) (int64, []AdminVideoJobSimpleCount, []AdminVideoJobSimpleCount, []AdminVideoJobSimpleCount, error) {
	type countRow struct {
		Count int64 `gorm:"column:count"`
	}
	type bucketRow struct {
		Key       string `gorm:"column:key"`
		Count     int64  `gorm:"column:count"`
		SortOrder int    `gorm:"column:sort_order"`
	}

	baseCTE := `
WITH probe AS (
	SELECT
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'duration_sec') = 'number'
			THEN (v.options->'source_video_probe'->>'duration_sec')::double precision
			ELSE NULL
		END AS duration_sec,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'width') = 'number'
			THEN (v.options->'source_video_probe'->>'width')::double precision
			ELSE NULL
		END AS width,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'height') = 'number'
			THEN (v.options->'source_video_probe'->>'height')::double precision
			ELSE NULL
		END AS height,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'fps') = 'number'
			THEN (v.options->'source_video_probe'->>'fps')::double precision
			ELSE NULL
		END AS fps
	FROM archive.video_jobs v
	WHERE v.created_at >= ?
		AND jsonb_typeof(v.options->'source_video_probe') = 'object'
)
`

	var jobsWindowRow countRow
	if err := h.db.Raw(baseCTE+`
SELECT COUNT(*)::bigint AS count
FROM probe
`, since).Scan(&jobsWindowRow).Error; err != nil {
		return 0, nil, nil, nil, err
	}

	toCounts := func(rows []bucketRow) []AdminVideoJobSimpleCount {
		out := make([]AdminVideoJobSimpleCount, 0, len(rows))
		for _, row := range rows {
			key := strings.TrimSpace(row.Key)
			if key == "" {
				continue
			}
			out = append(out, AdminVideoJobSimpleCount{
				Key:   key,
				Count: row.Count,
			})
		}
		return out
	}

	var durationRows []bucketRow
	if err := h.db.Raw(baseCTE+`
SELECT
	bucket AS key,
	COUNT(*)::bigint AS count,
	MIN(sort_order)::int AS sort_order
FROM (
	SELECT
		CASE
			WHEN duration_sec < 5 THEN '<5s'
			WHEN duration_sec < 10 THEN '5-10s'
			WHEN duration_sec < 30 THEN '10-30s'
			WHEN duration_sec < 60 THEN '30-60s'
			WHEN duration_sec < 180 THEN '1-3m'
			WHEN duration_sec < 600 THEN '3-10m'
			ELSE '10m+'
		END AS bucket,
		CASE
			WHEN duration_sec < 5 THEN 1
			WHEN duration_sec < 10 THEN 2
			WHEN duration_sec < 30 THEN 3
			WHEN duration_sec < 60 THEN 4
			WHEN duration_sec < 180 THEN 5
			WHEN duration_sec < 600 THEN 6
			ELSE 7
		END AS sort_order
	FROM probe
	WHERE duration_sec IS NOT NULL
) t
GROUP BY bucket
ORDER BY sort_order ASC, bucket ASC
`, since).Scan(&durationRows).Error; err != nil {
		return 0, nil, nil, nil, err
	}

	var resolutionRows []bucketRow
	if err := h.db.Raw(baseCTE+`
SELECT
	bucket AS key,
	COUNT(*)::bigint AS count,
	MIN(sort_order)::int AS sort_order
FROM (
	SELECT
		CASE
			WHEN width >= 3840 OR height >= 2160 THEN '4k+'
			WHEN width >= 2560 OR height >= 1440 THEN '2k'
			WHEN width >= 1920 OR height >= 1080 THEN '1080p'
			WHEN width >= 1280 OR height >= 720 THEN '720p'
			WHEN width >= 854 OR height >= 480 THEN '480p'
			ELSE '<480p'
		END AS bucket,
		CASE
			WHEN width >= 3840 OR height >= 2160 THEN 1
			WHEN width >= 2560 OR height >= 1440 THEN 2
			WHEN width >= 1920 OR height >= 1080 THEN 3
			WHEN width >= 1280 OR height >= 720 THEN 4
			WHEN width >= 854 OR height >= 480 THEN 5
			ELSE 6
		END AS sort_order
	FROM probe
	WHERE width IS NOT NULL
		AND height IS NOT NULL
		AND width > 0
		AND height > 0
) t
GROUP BY bucket
ORDER BY sort_order ASC, bucket ASC
`, since).Scan(&resolutionRows).Error; err != nil {
		return 0, nil, nil, nil, err
	}

	var fpsRows []bucketRow
	if err := h.db.Raw(baseCTE+`
SELECT
	bucket AS key,
	COUNT(*)::bigint AS count,
	MIN(sort_order)::int AS sort_order
FROM (
	SELECT
		CASE
			WHEN fps < 15 THEN '<15fps'
			WHEN fps < 24 THEN '15-24fps'
			WHEN fps < 30 THEN '24-30fps'
			WHEN fps < 60 THEN '30-60fps'
			ELSE '60fps+'
		END AS bucket,
		CASE
			WHEN fps < 15 THEN 1
			WHEN fps < 24 THEN 2
			WHEN fps < 30 THEN 3
			WHEN fps < 60 THEN 4
			ELSE 5
		END AS sort_order
	FROM probe
	WHERE fps IS NOT NULL
) t
GROUP BY bucket
ORDER BY sort_order ASC, bucket ASC
`, since).Scan(&fpsRows).Error; err != nil {
		return 0, nil, nil, nil, err
	}

	return jobsWindowRow.Count, toCounts(durationRows), toCounts(resolutionRows), toCounts(fpsRows), nil
}

func sourceProbeBucketSQL(dimension string) (bucketExpr string, sortExpr string, filterExpr string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(dimension)) {
	case "duration":
		return `
CASE
	WHEN duration_sec < 5 THEN '<5s'
	WHEN duration_sec < 10 THEN '5-10s'
	WHEN duration_sec < 30 THEN '10-30s'
	WHEN duration_sec < 60 THEN '30-60s'
	WHEN duration_sec < 180 THEN '1-3m'
	WHEN duration_sec < 600 THEN '3-10m'
	ELSE '10m+'
END
`, `
CASE
	WHEN duration_sec < 5 THEN 1
	WHEN duration_sec < 10 THEN 2
	WHEN duration_sec < 30 THEN 3
	WHEN duration_sec < 60 THEN 4
	WHEN duration_sec < 180 THEN 5
	WHEN duration_sec < 600 THEN 6
	ELSE 7
END
`, "duration_sec IS NOT NULL", true
	case "resolution":
		return `
CASE
	WHEN width >= 3840 OR height >= 2160 THEN '4k+'
	WHEN width >= 2560 OR height >= 1440 THEN '2k'
	WHEN width >= 1920 OR height >= 1080 THEN '1080p'
	WHEN width >= 1280 OR height >= 720 THEN '720p'
	WHEN width >= 854 OR height >= 480 THEN '480p'
	ELSE '<480p'
END
`, `
CASE
	WHEN width >= 3840 OR height >= 2160 THEN 1
	WHEN width >= 2560 OR height >= 1440 THEN 2
	WHEN width >= 1920 OR height >= 1080 THEN 3
	WHEN width >= 1280 OR height >= 720 THEN 4
	WHEN width >= 854 OR height >= 480 THEN 5
	ELSE 6
END
`, "width IS NOT NULL AND height IS NOT NULL AND width > 0 AND height > 0", true
	case "fps":
		return `
CASE
	WHEN fps < 15 THEN '<15fps'
	WHEN fps < 24 THEN '15-24fps'
	WHEN fps < 30 THEN '24-30fps'
	WHEN fps < 60 THEN '30-60fps'
	ELSE '60fps+'
END
`, `
CASE
	WHEN fps < 15 THEN 1
	WHEN fps < 24 THEN 2
	WHEN fps < 30 THEN 3
	WHEN fps < 60 THEN 4
	ELSE 5
END
`, "fps IS NOT NULL", true
	default:
		return "", "", "", false
	}
}

func (h *Handler) loadVideoJobSourceProbeQualityStats(since time.Time) ([]AdminVideoJobSourceProbeQualityStat, []AdminVideoJobSourceProbeQualityStat, []AdminVideoJobSourceProbeQualityStat, error) {
	durationStats, err := h.loadVideoJobSourceProbeQualityStatsByDimension(since, "duration")
	if err != nil {
		return nil, nil, nil, err
	}
	resolutionStats, err := h.loadVideoJobSourceProbeQualityStatsByDimension(since, "resolution")
	if err != nil {
		return nil, nil, nil, err
	}
	fpsStats, err := h.loadVideoJobSourceProbeQualityStatsByDimension(since, "fps")
	if err != nil {
		return nil, nil, nil, err
	}
	return durationStats, resolutionStats, fpsStats, nil
}

func (h *Handler) loadVideoJobSourceProbeQualityStatsByDimension(since time.Time, dimension string) ([]AdminVideoJobSourceProbeQualityStat, error) {
	bucketExpr, sortExpr, filterExpr, ok := sourceProbeBucketSQL(dimension)
	if !ok {
		return nil, fmt.Errorf("unsupported source probe dimension: %s", dimension)
	}

	type row struct {
		Bucket         string   `gorm:"column:bucket"`
		Jobs           int64    `gorm:"column:jobs"`
		DoneJobs       int64    `gorm:"column:done_jobs"`
		FailedJobs     int64    `gorm:"column:failed_jobs"`
		CancelledJobs  int64    `gorm:"column:cancelled_jobs"`
		PendingJobs    int64    `gorm:"column:pending_jobs"`
		DurationP50Sec *float64 `gorm:"column:duration_p50_sec"`
		DurationP95Sec *float64 `gorm:"column:duration_p95_sec"`
	}

	query := fmt.Sprintf(`
WITH probe AS (
	SELECT
		COALESCE(LOWER(TRIM(v.status)), '') AS status,
		v.started_at,
		v.finished_at,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'duration_sec') = 'number'
			THEN (v.options->'source_video_probe'->>'duration_sec')::double precision
			ELSE NULL
		END AS duration_sec,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'width') = 'number'
			THEN (v.options->'source_video_probe'->>'width')::double precision
			ELSE NULL
		END AS width,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'height') = 'number'
			THEN (v.options->'source_video_probe'->>'height')::double precision
			ELSE NULL
		END AS height,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'fps') = 'number'
			THEN (v.options->'source_video_probe'->>'fps')::double precision
			ELSE NULL
		END AS fps
	FROM archive.video_jobs v
	WHERE v.created_at >= ?
		AND jsonb_typeof(v.options->'source_video_probe') = 'object'
)
SELECT
	bucket,
	COUNT(*)::bigint AS jobs,
	COUNT(*) FILTER (WHERE status = ?)::bigint AS done_jobs,
	COUNT(*) FILTER (WHERE status = ?)::bigint AS failed_jobs,
	COUNT(*) FILTER (WHERE status = ?)::bigint AS cancelled_jobs,
	COUNT(*) FILTER (WHERE status NOT IN (?, ?, ?))::bigint AS pending_jobs,
	percentile_cont(0.50) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at)))
		FILTER (WHERE status = ? AND started_at IS NOT NULL AND finished_at IS NOT NULL AND finished_at > started_at) AS duration_p50_sec,
	percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at)))
		FILTER (WHERE status = ? AND started_at IS NOT NULL AND finished_at IS NOT NULL AND finished_at > started_at) AS duration_p95_sec,
	MIN(sort_order)::int AS sort_order
FROM (
	SELECT
		%s AS bucket,
		%s AS sort_order,
		status,
		started_at,
		finished_at
	FROM probe
	WHERE %s
) b
GROUP BY bucket
ORDER BY sort_order ASC, bucket ASC
`, bucketExpr, sortExpr, filterExpr)

	var rows []row
	if err := h.db.Raw(
		query,
		since,
		models.VideoJobStatusDone,
		models.VideoJobStatusFailed,
		models.VideoJobStatusCancelled,
		models.VideoJobStatusDone,
		models.VideoJobStatusFailed,
		models.VideoJobStatusCancelled,
		models.VideoJobStatusDone,
		models.VideoJobStatusDone,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobSourceProbeQualityStat, 0, len(rows))
	for _, item := range rows {
		bucket := strings.TrimSpace(item.Bucket)
		if bucket == "" {
			continue
		}
		stat := AdminVideoJobSourceProbeQualityStat{
			Bucket:        bucket,
			Jobs:          item.Jobs,
			DoneJobs:      item.DoneJobs,
			FailedJobs:    item.FailedJobs,
			PendingJobs:   item.PendingJobs,
			CancelledJobs: item.CancelledJobs,
		}
		terminal := stat.DoneJobs + stat.FailedJobs
		stat.TerminalJobs = terminal
		if terminal > 0 {
			stat.SuccessRate = float64(stat.DoneJobs) / float64(terminal)
			stat.FailureRate = float64(stat.FailedJobs) / float64(terminal)
		}
		if item.DurationP50Sec != nil {
			stat.DurationP50Sec = *item.DurationP50Sec
		}
		if item.DurationP95Sec != nil {
			stat.DurationP95Sec = *item.DurationP95Sec
		}
		out = append(out, stat)
	}

	return out, nil
}

func (h *Handler) loadVideoJobStageDurations(since time.Time) ([]AdminVideoJobStageDuration, error) {
	type row struct {
		FromStage string   `gorm:"column:from_stage"`
		ToStage   string   `gorm:"column:to_stage"`
		Count     int64    `gorm:"column:count"`
		AvgSec    *float64 `gorm:"column:avg_sec"`
		P95Sec    *float64 `gorm:"column:p95_sec"`
	}

	var rows []row
	if err := h.db.Raw(`
WITH entries AS (
	SELECT
		job_id,
		stage,
		created_at,
		id
	FROM (
		SELECT
			job_id,
			COALESCE(NULLIF(TRIM(stage), ''), 'unknown') AS stage,
			created_at,
			id,
			LAG(COALESCE(NULLIF(TRIM(stage), ''), 'unknown')) OVER (
				PARTITION BY job_id
				ORDER BY created_at ASC, id ASC
			) AS prev_stage
		FROM archive.video_job_events
		WHERE created_at >= ?
	) t
	WHERE prev_stage IS NULL OR prev_stage <> stage
),
transitions AS (
	SELECT
		stage AS from_stage,
		LEAD(stage) OVER (
			PARTITION BY job_id
			ORDER BY created_at ASC, id ASC
		) AS to_stage,
		created_at AS from_at,
		LEAD(created_at) OVER (
			PARTITION BY job_id
			ORDER BY created_at ASC, id ASC
		) AS to_at
	FROM entries
),
durations AS (
	SELECT
		from_stage,
		to_stage,
		EXTRACT(EPOCH FROM (to_at - from_at)) AS duration_sec
	FROM transitions
	WHERE to_stage IS NOT NULL
		AND to_at IS NOT NULL
		AND to_at > from_at
)
SELECT
	from_stage,
	to_stage,
	COUNT(*)::bigint AS count,
	AVG(duration_sec) AS avg_sec,
	percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_sec) AS p95_sec
FROM durations
GROUP BY from_stage, to_stage
ORDER BY count DESC, from_stage ASC, to_stage ASC
LIMIT 24
`, since).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobStageDuration, 0, len(rows))
	for _, item := range rows {
		fromStage := strings.TrimSpace(item.FromStage)
		toStage := strings.TrimSpace(item.ToStage)
		if fromStage == "" || toStage == "" {
			continue
		}
		stat := AdminVideoJobStageDuration{
			FromStage:  fromStage,
			ToStage:    toStage,
			Transition: fromStage + " -> " + toStage,
			Count:      item.Count,
		}
		if item.AvgSec != nil {
			stat.AvgSec = *item.AvgSec
		}
		if item.P95Sec != nil {
			stat.P95Sec = *item.P95Sec
		}
		out = append(out, stat)
	}
	return out, nil
}

func (h *Handler) loadVideoJobFormatStats24h(since time.Time) ([]AdminVideoJobFormatStat, error) {
	type requestedRow struct {
		Format        string `gorm:"column:format"`
		RequestedJobs int64  `gorm:"column:requested_jobs"`
	}
	type generatedRow struct {
		Format        string `gorm:"column:format"`
		GeneratedJobs int64  `gorm:"column:generated_jobs"`
	}
	type artifactRow struct {
		Format               string   `gorm:"column:format"`
		ArtifactCount        int64    `gorm:"column:artifact_count"`
		AvgArtifactSizeBytes *float64 `gorm:"column:avg_artifact_size_bytes"`
	}
	type feedbackRow struct {
		Format             string   `gorm:"column:format"`
		EngagedJobs        int64    `gorm:"column:engaged_jobs"`
		FeedbackSignals    int64    `gorm:"column:feedback_signals"`
		AvgEngagementScore *float64 `gorm:"column:avg_engagement_score"`
	}
	type sizeProfileRow struct {
		Format          string `gorm:"column:format"`
		SizeProfileJobs int64  `gorm:"column:size_profile_jobs"`
	}
	type sizeProfileArtifactRow struct {
		Format               string   `gorm:"column:format"`
		AvgArtifactSizeBytes *float64 `gorm:"column:avg_artifact_size_bytes"`
	}
	type sizeBudgetHitRow struct {
		Format            string `gorm:"column:format"`
		SizeBudgetSamples int64  `gorm:"column:size_budget_samples"`
		SizeBudgetHits    int64  `gorm:"column:size_budget_hits"`
	}

	var requestedRows []requestedRow
	if err := h.db.Raw(`
WITH requested AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE fmt.value
		END AS format
	FROM archive.video_jobs j
	CROSS JOIN LATERAL unnest(
		string_to_array(replace(lower(COALESCE(j.output_formats, '')), ' ', ''), ',')
	) AS fmt(value)
	WHERE j.created_at >= ?
		AND trim(fmt.value) <> ''
)
SELECT
	format,
	COUNT(*)::bigint AS requested_jobs
FROM requested
GROUP BY format
`, since).Scan(&requestedRows).Error; err != nil {
		return nil, err
	}

	var generatedRows []generatedRow
	if err := h.db.Raw(`
WITH generated AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE lower(fmt.value)
		END AS format
	FROM archive.video_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'output_formats') = 'array' THEN j.metrics->'output_formats'
			ELSE '[]'::jsonb
		END
	) AS fmt(value)
	WHERE j.created_at >= ?
		AND trim(fmt.value) <> ''
)
SELECT
	format,
	COUNT(*)::bigint AS generated_jobs
FROM generated
GROUP BY format
`, since).Scan(&generatedRows).Error; err != nil {
		return nil, err
	}

	var artifactRows []artifactRow
	if err := h.db.Raw(`
WITH output_artifacts AS (
	SELECT
		CASE
			WHEN raw_format = 'jpeg' THEN 'jpg'
			ELSE raw_format
		END AS format,
		size_bytes
	FROM (
		SELECT
			lower(COALESCE(NULLIF(a.metadata->>'format', ''), '')) AS raw_format,
			a.size_bytes
		FROM archive.video_job_artifacts a
		JOIN archive.video_jobs j ON j.id = a.job_id
		WHERE j.created_at >= ?
			AND a.type IN ('frame', 'clip', 'live_package')
	) t
	WHERE raw_format <> ''
)
SELECT
	format,
	COUNT(*)::bigint AS artifact_count,
	AVG(size_bytes)::double precision AS avg_artifact_size_bytes
FROM output_artifacts
GROUP BY format
`, since).Scan(&artifactRows).Error; err != nil {
		return nil, err
	}

	var feedbackRows []feedbackRow
	if err := h.db.Raw(`
WITH generated AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE lower(fmt.value)
		END AS format,
		COALESCE((j.metrics->'feedback_v1'->>'total_signals')::bigint, 0) AS total_signals,
		COALESCE((j.metrics->'feedback_v1'->>'engagement_score')::double precision, 0) AS engagement_score
	FROM archive.video_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'output_formats') = 'array' THEN j.metrics->'output_formats'
			ELSE '[]'::jsonb
		END
	) AS fmt(value)
	WHERE j.created_at >= ?
		AND j.status = ?
		AND trim(fmt.value) <> ''
)
SELECT
	format,
	COUNT(*) FILTER (WHERE total_signals > 0)::bigint AS engaged_jobs,
	COALESCE(SUM(total_signals), 0)::bigint AS feedback_signals,
	AVG(engagement_score) FILTER (WHERE total_signals > 0) AS avg_engagement_score
FROM generated
GROUP BY format
`, since, models.VideoJobStatusDone).Scan(&feedbackRows).Error; err != nil {
		return nil, err
	}

	var sizeProfileRows []sizeProfileRow
	if err := h.db.Raw(`
WITH generated AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE lower(fmt.value)
		END AS format
	FROM archive.video_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'output_formats') = 'array' THEN j.metrics->'output_formats'
			ELSE '[]'::jsonb
		END
	) AS fmt(value)
	WHERE j.created_at >= ?
		AND j.status = ?
		AND trim(fmt.value) <> ''
)
SELECT
	g.format AS format,
	COUNT(*)::bigint AS size_profile_jobs
FROM generated g
JOIN archive.video_jobs j ON j.id = g.job_id
WHERE
	(g.format = 'jpg' AND lower(COALESCE(j.metrics->'quality_settings'->>'jpg_profile', '')) = 'size')
	OR
	(g.format = 'png' AND lower(COALESCE(j.metrics->'quality_settings'->>'png_profile', '')) = 'size')
GROUP BY g.format
`, since, models.VideoJobStatusDone).Scan(&sizeProfileRows).Error; err != nil {
		return nil, err
	}

	var sizeProfileArtifactRows []sizeProfileArtifactRow
	if err := h.db.Raw(`
WITH generated AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE lower(fmt.value)
		END AS format
	FROM archive.video_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'output_formats') = 'array' THEN j.metrics->'output_formats'
			ELSE '[]'::jsonb
		END
	) AS fmt(value)
	WHERE j.created_at >= ?
		AND j.status = ?
		AND trim(fmt.value) <> ''
),
size_profile_jobs AS (
	SELECT
		g.job_id,
		g.format
	FROM generated g
	JOIN archive.video_jobs j ON j.id = g.job_id
	WHERE
		(g.format = 'jpg' AND lower(COALESCE(j.metrics->'quality_settings'->>'jpg_profile', '')) = 'size')
		OR
		(g.format = 'png' AND lower(COALESCE(j.metrics->'quality_settings'->>'png_profile', '')) = 'size')
),
still_artifacts AS (
	SELECT
		t.job_id,
		CASE
			WHEN raw_format = 'jpeg' THEN 'jpg'
			ELSE raw_format
		END AS format,
		t.size_bytes
	FROM (
		SELECT
			a.job_id,
			lower(COALESCE(NULLIF(a.metadata->>'format', ''), '')) AS raw_format,
			a.size_bytes
		FROM archive.video_job_artifacts a
		JOIN archive.video_jobs j ON j.id = a.job_id
		WHERE j.created_at >= ?
			AND j.status = ?
			AND a.type = 'frame'
	) t
	WHERE raw_format <> ''
)
SELECT
	a.format,
	AVG(a.size_bytes)::double precision AS avg_artifact_size_bytes
FROM still_artifacts a
JOIN size_profile_jobs s ON s.job_id = a.job_id AND s.format = a.format
GROUP BY a.format
	`, since, models.VideoJobStatusDone, since, models.VideoJobStatusDone).Scan(&sizeProfileArtifactRows).Error; err != nil {
		return nil, err
	}

	var sizeBudgetHitRows []sizeBudgetHitRow
	if err := h.db.Raw(`
WITH still_artifacts AS (
	SELECT
		a.job_id,
		CASE
			WHEN raw_format = 'jpeg' THEN 'jpg'
			ELSE raw_format
		END AS format,
		a.size_bytes,
		lower(COALESCE(j.metrics->'quality_settings'->>'jpg_profile', '')) AS jpg_profile,
		lower(COALESCE(j.metrics->'quality_settings'->>'png_profile', '')) AS png_profile,
		COALESCE((j.metrics->'quality_settings'->>'jpg_target_size_kb')::bigint, 0) AS jpg_target_size_kb,
		COALESCE((j.metrics->'quality_settings'->>'png_target_size_kb')::bigint, 0) AS png_target_size_kb
	FROM (
		SELECT
			a.job_id,
			lower(COALESCE(NULLIF(a.metadata->>'format', ''), '')) AS raw_format,
			a.size_bytes
		FROM archive.video_job_artifacts a
		JOIN archive.video_jobs j ON j.id = a.job_id
		WHERE j.created_at >= ?
			AND j.status = ?
			AND a.type = 'frame'
	) a
	JOIN archive.video_jobs j ON j.id = a.job_id
	WHERE raw_format IN ('jpg', 'jpeg', 'png')
),
budget_targets AS (
	SELECT
		format,
		size_bytes,
		CASE
			WHEN format = 'jpg' AND jpg_profile = 'size' AND jpg_target_size_kb > 0
				THEN jpg_target_size_kb * 1024
			WHEN format = 'png' AND png_profile = 'size' AND png_target_size_kb > 0
				THEN png_target_size_kb * 1024
			ELSE 0
		END AS target_bytes
	FROM still_artifacts
)
SELECT
	format,
	COUNT(*) FILTER (WHERE target_bytes > 0)::bigint AS size_budget_samples,
	COUNT(*) FILTER (WHERE target_bytes > 0 AND size_bytes <= target_bytes)::bigint AS size_budget_hits
FROM budget_targets
GROUP BY format
`, since, models.VideoJobStatusDone).Scan(&sizeBudgetHitRows).Error; err != nil {
		return nil, err
	}

	statsMap := make(map[string]*AdminVideoJobFormatStat)
	for _, item := range requestedRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.RequestedJobs += item.RequestedJobs
	}
	for _, item := range generatedRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.GeneratedJobs += item.GeneratedJobs
	}
	for _, item := range artifactRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.ArtifactCount += item.ArtifactCount
		if item.AvgArtifactSizeBytes != nil {
			stat.AvgArtifactSizeBytes = *item.AvgArtifactSizeBytes
		}
	}
	for _, item := range feedbackRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.EngagedJobs += item.EngagedJobs
		stat.FeedbackSignals += item.FeedbackSignals
		if item.AvgEngagementScore != nil {
			stat.AvgEngagementScore = *item.AvgEngagementScore
		}
	}
	for _, item := range sizeProfileRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.SizeProfileJobs += item.SizeProfileJobs
	}
	for _, item := range sizeProfileArtifactRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		if item.AvgArtifactSizeBytes != nil {
			stat.SizeProfileAvgBytes = *item.AvgArtifactSizeBytes
		}
	}
	for _, item := range sizeBudgetHitRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.SizeBudgetSamples += item.SizeBudgetSamples
		stat.SizeBudgetHits += item.SizeBudgetHits
	}

	stats := make([]AdminVideoJobFormatStat, 0, len(statsMap))
	for _, item := range statsMap {
		if item.RequestedJobs > 0 {
			item.SuccessRate = float64(item.GeneratedJobs) / float64(item.RequestedJobs)
		}
		if item.GeneratedJobs > 0 {
			item.SizeProfileRate = float64(item.SizeProfileJobs) / float64(item.GeneratedJobs)
		}
		if item.SizeBudgetSamples > 0 {
			item.SizeBudgetHitRate = float64(item.SizeBudgetHits) / float64(item.SizeBudgetSamples)
		}
		stats = append(stats, *item)
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].RequestedJobs != stats[j].RequestedJobs {
			return stats[i].RequestedJobs > stats[j].RequestedJobs
		}
		if stats[i].GeneratedJobs != stats[j].GeneratedJobs {
			return stats[i].GeneratedJobs > stats[j].GeneratedJobs
		}
		return stats[i].Format < stats[j].Format
	})
	return stats, nil
}

func (h *Handler) loadSampleVideoJobsWindowSummary(since time.Time) (adminSampleVideoJobsWindowSummary, error) {
	type row struct {
		JobsWindow   int64    `gorm:"column:jobs_window"`
		DoneWindow   int64    `gorm:"column:done_window"`
		FailedWindow int64    `gorm:"column:failed_window"`
		P50          *float64 `gorm:"column:p50"`
		P95          *float64 `gorm:"column:p95"`
	}

	var result row
	if err := h.db.Raw(`
WITH sample_jobs AS (
	SELECT v.*
	FROM archive.video_jobs v
	JOIN archive.collections c ON c.id = v.result_collection_id
	WHERE c.is_sample = TRUE
		AND v.created_at >= ?
)
SELECT
	COUNT(*)::bigint AS jobs_window,
	COUNT(*) FILTER (WHERE status = ?)::bigint AS done_window,
	COUNT(*) FILTER (WHERE status = ?)::bigint AS failed_window,
	percentile_cont(0.50) WITHIN GROUP (
		ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at))
	) FILTER (
		WHERE status = ?
			AND started_at IS NOT NULL
			AND finished_at IS NOT NULL
	) AS p50,
	percentile_cont(0.95) WITHIN GROUP (
		ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at))
	) FILTER (
		WHERE status = ?
			AND started_at IS NOT NULL
			AND finished_at IS NOT NULL
	) AS p95
FROM sample_jobs
`,
		since,
		models.VideoJobStatusDone,
		models.VideoJobStatusFailed,
		models.VideoJobStatusDone,
		models.VideoJobStatusDone,
	).Scan(&result).Error; err != nil {
		return adminSampleVideoJobsWindowSummary{}, err
	}

	out := adminSampleVideoJobsWindowSummary{
		JobsWindow:   result.JobsWindow,
		DoneWindow:   result.DoneWindow,
		FailedWindow: result.FailedWindow,
	}
	if out.JobsWindow > 0 {
		out.SuccessRate = float64(out.DoneWindow) / float64(out.JobsWindow)
	}
	if result.P50 != nil {
		out.DurationP50 = *result.P50
	}
	if result.P95 != nil {
		out.DurationP95 = *result.P95
	}
	return out, nil
}

func (h *Handler) loadSampleVideoJobFormatBaselineRows(since time.Time) ([]adminSampleVideoJobFormatBaselineRow, error) {
	type requestedRow struct {
		Format        string `gorm:"column:format"`
		RequestedJobs int64  `gorm:"column:requested_jobs"`
	}
	type generatedRow struct {
		Format        string `gorm:"column:format"`
		GeneratedJobs int64  `gorm:"column:generated_jobs"`
	}
	type artifactRow struct {
		Format               string   `gorm:"column:format"`
		ArtifactCount        int64    `gorm:"column:artifact_count"`
		AvgArtifactSizeBytes *float64 `gorm:"column:avg_artifact_size_bytes"`
	}
	type durationRow struct {
		Format string   `gorm:"column:format"`
		P50Sec *float64 `gorm:"column:p50_sec"`
		P95Sec *float64 `gorm:"column:p95_sec"`
	}

	var requestedRows []requestedRow
	if err := h.db.Raw(`
WITH sample_jobs AS (
	SELECT v.id, v.output_formats
	FROM archive.video_jobs v
	JOIN archive.collections c ON c.id = v.result_collection_id
	WHERE c.is_sample = TRUE
		AND v.created_at >= ?
),
requested AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE fmt.value
		END AS format
	FROM sample_jobs j
	CROSS JOIN LATERAL unnest(
		string_to_array(replace(lower(COALESCE(j.output_formats, '')), ' ', ''), ',')
	) AS fmt(value)
	WHERE trim(fmt.value) <> ''
)
SELECT
	format,
	COUNT(*)::bigint AS requested_jobs
FROM requested
GROUP BY format
`, since).Scan(&requestedRows).Error; err != nil {
		return nil, err
	}

	var generatedRows []generatedRow
	if err := h.db.Raw(`
WITH sample_jobs AS (
	SELECT v.id, v.metrics
	FROM archive.video_jobs v
	JOIN archive.collections c ON c.id = v.result_collection_id
	WHERE c.is_sample = TRUE
		AND v.created_at >= ?
),
generated AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE lower(fmt.value)
		END AS format
	FROM sample_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'output_formats') = 'array' THEN j.metrics->'output_formats'
			ELSE '[]'::jsonb
		END
	) AS fmt(value)
	WHERE trim(fmt.value) <> ''
)
SELECT
	format,
	COUNT(*)::bigint AS generated_jobs
FROM generated
GROUP BY format
`, since).Scan(&generatedRows).Error; err != nil {
		return nil, err
	}

	var artifactRows []artifactRow
	if err := h.db.Raw(`
WITH sample_jobs AS (
	SELECT v.id
	FROM archive.video_jobs v
	JOIN archive.collections c ON c.id = v.result_collection_id
	WHERE c.is_sample = TRUE
		AND v.created_at >= ?
),
output_artifacts AS (
	SELECT
		CASE
			WHEN raw_format = 'jpeg' THEN 'jpg'
			ELSE raw_format
		END AS format,
		size_bytes
	FROM (
		SELECT
			lower(COALESCE(NULLIF(a.metadata->>'format', ''), '')) AS raw_format,
			a.size_bytes
		FROM archive.video_job_artifacts a
		JOIN sample_jobs s ON s.id = a.job_id
		WHERE a.type IN ('frame', 'clip', 'live_package')
	) t
	WHERE raw_format <> ''
)
SELECT
	format,
	COUNT(*)::bigint AS artifact_count,
	AVG(size_bytes)::double precision AS avg_artifact_size_bytes
FROM output_artifacts
GROUP BY format
`, since).Scan(&artifactRows).Error; err != nil {
		return nil, err
	}

	var durationRows []durationRow
	if err := h.db.Raw(`
WITH sample_jobs AS (
	SELECT v.id, v.output_formats, v.status, v.started_at, v.finished_at
	FROM archive.video_jobs v
	JOIN archive.collections c ON c.id = v.result_collection_id
	WHERE c.is_sample = TRUE
		AND v.created_at >= ?
),
requested AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE fmt.value
		END AS format
	FROM sample_jobs j
	CROSS JOIN LATERAL unnest(
		string_to_array(replace(lower(COALESCE(j.output_formats, '')), ' ', ''), ',')
	) AS fmt(value)
	WHERE trim(fmt.value) <> ''
),
durations AS (
	SELECT
		r.format,
		EXTRACT(EPOCH FROM (j.finished_at - j.started_at)) AS duration_sec
	FROM requested r
	JOIN sample_jobs j ON j.id = r.job_id
	WHERE j.status = ?
		AND j.started_at IS NOT NULL
		AND j.finished_at IS NOT NULL
)
SELECT
	format,
	percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_sec) AS p50_sec,
	percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_sec) AS p95_sec
FROM durations
GROUP BY format
`, since, models.VideoJobStatusDone).Scan(&durationRows).Error; err != nil {
		return nil, err
	}

	statsMap := make(map[string]*adminSampleVideoJobFormatBaselineRow)
	ensure := func(raw string) *adminSampleVideoJobFormatBaselineRow {
		format := normalizeVideoJobFormat(raw)
		if format == "" {
			return nil
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &adminSampleVideoJobFormatBaselineRow{Format: format}
			statsMap[format] = stat
		}
		return stat
	}
	for _, item := range requestedRows {
		stat := ensure(item.Format)
		if stat == nil {
			continue
		}
		stat.RequestedJobs += item.RequestedJobs
	}
	for _, item := range generatedRows {
		stat := ensure(item.Format)
		if stat == nil {
			continue
		}
		stat.GeneratedJobs += item.GeneratedJobs
	}
	for _, item := range artifactRows {
		stat := ensure(item.Format)
		if stat == nil {
			continue
		}
		stat.ArtifactCount += item.ArtifactCount
		if item.AvgArtifactSizeBytes != nil {
			stat.AvgArtifactSizeBytes = *item.AvgArtifactSizeBytes
		}
	}
	for _, item := range durationRows {
		stat := ensure(item.Format)
		if stat == nil {
			continue
		}
		if item.P50Sec != nil {
			stat.DurationP50Sec = *item.P50Sec
		}
		if item.P95Sec != nil {
			stat.DurationP95Sec = *item.P95Sec
		}
	}

	rows := make([]adminSampleVideoJobFormatBaselineRow, 0, len(statsMap))
	for _, item := range statsMap {
		if item.RequestedJobs > 0 {
			item.SuccessRate = float64(item.GeneratedJobs) / float64(item.RequestedJobs)
		}
		rows = append(rows, *item)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].RequestedJobs != rows[j].RequestedJobs {
			return rows[i].RequestedJobs > rows[j].RequestedJobs
		}
		if rows[i].GeneratedJobs != rows[j].GeneratedJobs {
			return rows[i].GeneratedJobs > rows[j].GeneratedJobs
		}
		return rows[i].Format < rows[j].Format
	})
	return rows, nil
}

func computeDeltaAndUplift(baseValue, targetValue float64) (delta float64, uplift float64) {
	delta = targetValue - baseValue
	if baseValue != 0 {
		uplift = delta / baseValue
	}
	return delta, uplift
}

func buildSampleVideoJobsDiffSummary(
	base adminSampleVideoJobsWindowSummary,
	target adminSampleVideoJobsWindowSummary,
) AdminSampleVideoJobsBaselineDiffSummary {
	jobsDelta, jobsUplift := computeDeltaAndUplift(float64(base.JobsWindow), float64(target.JobsWindow))
	doneDelta, doneUplift := computeDeltaAndUplift(float64(base.DoneWindow), float64(target.DoneWindow))
	failedDelta, failedUplift := computeDeltaAndUplift(float64(base.FailedWindow), float64(target.FailedWindow))
	successDelta, successUplift := computeDeltaAndUplift(base.SuccessRate, target.SuccessRate)
	p50Delta, p50Uplift := computeDeltaAndUplift(base.DurationP50, target.DurationP50)
	p95Delta, p95Uplift := computeDeltaAndUplift(base.DurationP95, target.DurationP95)

	return AdminSampleVideoJobsBaselineDiffSummary{
		BaseJobsWindow:       base.JobsWindow,
		TargetJobsWindow:     target.JobsWindow,
		JobsWindowDelta:      jobsDelta,
		JobsWindowUplift:     jobsUplift,
		BaseDoneWindow:       base.DoneWindow,
		TargetDoneWindow:     target.DoneWindow,
		DoneWindowDelta:      doneDelta,
		DoneWindowUplift:     doneUplift,
		BaseFailedWindow:     base.FailedWindow,
		TargetFailedWindow:   target.FailedWindow,
		FailedWindowDelta:    failedDelta,
		FailedWindowUplift:   failedUplift,
		BaseSuccessRate:      base.SuccessRate,
		TargetSuccessRate:    target.SuccessRate,
		SuccessRateDelta:     successDelta,
		SuccessRateUplift:    successUplift,
		BaseDurationP50Sec:   base.DurationP50,
		TargetDurationP50Sec: target.DurationP50,
		DurationP50Delta:     p50Delta,
		DurationP50Uplift:    p50Uplift,
		BaseDurationP95Sec:   base.DurationP95,
		TargetDurationP95Sec: target.DurationP95,
		DurationP95Delta:     p95Delta,
		DurationP95Uplift:    p95Uplift,
	}
}

func buildSampleVideoJobFormatDiffRows(
	baseRows []adminSampleVideoJobFormatBaselineRow,
	targetRows []adminSampleVideoJobFormatBaselineRow,
) []adminSampleVideoJobFormatDiffRow {
	baseMap := make(map[string]adminSampleVideoJobFormatBaselineRow, len(baseRows))
	targetMap := make(map[string]adminSampleVideoJobFormatBaselineRow, len(targetRows))
	formats := make(map[string]struct{}, len(baseRows)+len(targetRows))

	for _, row := range baseRows {
		format := normalizeVideoJobFormat(row.Format)
		if format == "" {
			continue
		}
		row.Format = format
		baseMap[format] = row
		formats[format] = struct{}{}
	}
	for _, row := range targetRows {
		format := normalizeVideoJobFormat(row.Format)
		if format == "" {
			continue
		}
		row.Format = format
		targetMap[format] = row
		formats[format] = struct{}{}
	}

	rows := make([]adminSampleVideoJobFormatDiffRow, 0, len(formats))
	for format := range formats {
		base := baseMap[format]
		target := targetMap[format]

		successDelta, successUplift := computeDeltaAndUplift(base.SuccessRate, target.SuccessRate)
		sizeDelta, sizeUplift := computeDeltaAndUplift(base.AvgArtifactSizeBytes, target.AvgArtifactSizeBytes)
		p50Delta, p50Uplift := computeDeltaAndUplift(base.DurationP50Sec, target.DurationP50Sec)
		p95Delta, p95Uplift := computeDeltaAndUplift(base.DurationP95Sec, target.DurationP95Sec)

		rows = append(rows, adminSampleVideoJobFormatDiffRow{
			Format:                     format,
			BaseRequestedJobs:          base.RequestedJobs,
			BaseGeneratedJobs:          base.GeneratedJobs,
			BaseSuccessRate:            base.SuccessRate,
			BaseAvgArtifactSizeBytes:   base.AvgArtifactSizeBytes,
			BaseDurationP50Sec:         base.DurationP50Sec,
			BaseDurationP95Sec:         base.DurationP95Sec,
			TargetRequestedJobs:        target.RequestedJobs,
			TargetGeneratedJobs:        target.GeneratedJobs,
			TargetSuccessRate:          target.SuccessRate,
			TargetAvgArtifactSizeBytes: target.AvgArtifactSizeBytes,
			TargetDurationP50Sec:       target.DurationP50Sec,
			TargetDurationP95Sec:       target.DurationP95Sec,
			SuccessRateDelta:           successDelta,
			SuccessRateUplift:          successUplift,
			AvgArtifactSizeDelta:       sizeDelta,
			AvgArtifactSizeUplift:      sizeUplift,
			DurationP50Delta:           p50Delta,
			DurationP50Uplift:          p50Uplift,
			DurationP95Delta:           p95Delta,
			DurationP95Uplift:          p95Uplift,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TargetRequestedJobs != rows[j].TargetRequestedJobs {
			return rows[i].TargetRequestedJobs > rows[j].TargetRequestedJobs
		}
		if rows[i].TargetGeneratedJobs != rows[j].TargetGeneratedJobs {
			return rows[i].TargetGeneratedJobs > rows[j].TargetGeneratedJobs
		}
		return rows[i].Format < rows[j].Format
	})
	return rows
}

func (h *Handler) loadVideoJobFeedbackOverview(since time.Time) (signals int64, downloads int64, favorites int64, engagedJobs int64, avgScore float64, err error) {
	type row struct {
		Signals     int64    `gorm:"column:signals"`
		Downloads   int64    `gorm:"column:downloads"`
		Favorites   int64    `gorm:"column:favorites"`
		EngagedJobs int64    `gorm:"column:engaged_jobs"`
		AvgScore    *float64 `gorm:"column:avg_score"`
	}

	var result row
	if err = h.db.Raw(`
SELECT
	COALESCE(SUM(COALESCE((metrics->'feedback_v1'->>'total_signals')::bigint, 0)), 0)::bigint AS signals,
	COALESCE(SUM(COALESCE((metrics->'feedback_v1'->>'download_count')::bigint, 0)), 0)::bigint AS downloads,
	COALESCE(SUM(COALESCE((metrics->'feedback_v1'->>'favorite_count')::bigint, 0)), 0)::bigint AS favorites,
	COUNT(*) FILTER (
		WHERE COALESCE((metrics->'feedback_v1'->>'total_signals')::bigint, 0) > 0
	)::bigint AS engaged_jobs,
	AVG(COALESCE((metrics->'feedback_v1'->>'engagement_score')::double precision, 0)) FILTER (
		WHERE COALESCE((metrics->'feedback_v1'->>'total_signals')::bigint, 0) > 0
	) AS avg_score
FROM archive.video_jobs
WHERE status = ?
	AND finished_at >= ?
`, models.VideoJobStatusDone, since).Scan(&result).Error; err != nil {
		return
	}

	signals = result.Signals
	downloads = result.Downloads
	favorites = result.Favorites
	engagedJobs = result.EngagedJobs
	if result.AvgScore != nil {
		avgScore = *result.AvgScore
	}
	return
}

func (h *Handler) loadVideoJobFeedbackSceneStats(since time.Time) ([]AdminVideoJobFeedbackSceneStat, error) {
	type row struct {
		SceneTag string `gorm:"column:scene_tag"`
		Signals  int64  `gorm:"column:signals"`
	}

	var rows []row
	if err := h.db.Raw(`
SELECT
	stats.key AS scene_tag,
	COALESCE(SUM(stats.value::bigint), 0)::bigint AS signals
FROM archive.video_jobs j
CROSS JOIN LATERAL jsonb_each_text(
	CASE
		WHEN jsonb_typeof(j.metrics->'feedback_v1'->'scene_signal_counts') = 'object'
		THEN j.metrics->'feedback_v1'->'scene_signal_counts'
		ELSE '{}'::jsonb
	END
) AS stats(key, value)
WHERE j.status = ?
	AND j.finished_at >= ?
GROUP BY stats.key
ORDER BY signals DESC, scene_tag ASC
LIMIT 16
`, models.VideoJobStatusDone, since).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobFeedbackSceneStat, 0, len(rows))
	for _, item := range rows {
		tag := strings.TrimSpace(item.SceneTag)
		if tag == "" {
			continue
		}
		out = append(out, AdminVideoJobFeedbackSceneStat{
			SceneTag: tag,
			Signals:  item.Signals,
		})
	}
	return out, nil
}

func buildVideoJobFeedbackIntegrityAlerts(
	overview AdminVideoJobFeedbackIntegrityOverview,
	thresholds FeedbackIntegrityAlertThresholdSettings,
) ([]AdminVideoJobFeedbackIntegrityAlert, string) {
	alerts := make([]AdminVideoJobFeedbackIntegrityAlert, 0, 8)
	health := "green"
	pushAlert := func(level, code, message string) {
		level = strings.ToLower(strings.TrimSpace(level))
		if level == "critical" {
			health = "red"
		} else if level == "warn" && health != "red" {
			health = "yellow"
		}
		alerts = append(alerts, AdminVideoJobFeedbackIntegrityAlert{
			Level:   level,
			Code:    strings.TrimSpace(code),
			Message: strings.TrimSpace(message),
		})
	}

	if overview.Samples <= 0 {
		pushAlert("info", "feedback_integrity_no_data", "当前窗口暂无反馈样本，暂不触发完整性告警")
		return alerts, health
	}

	if overview.OutputCoverageRate < thresholds.FeedbackIntegrityOutputCoverageRateCritical {
		pushAlert(
			"critical",
			"feedback_integrity_output_coverage_critical",
			fmt.Sprintf(
				"output覆盖率 %.2f%% 低于 critical %.2f%%",
				overview.OutputCoverageRate*100,
				thresholds.FeedbackIntegrityOutputCoverageRateCritical*100,
			),
		)
	} else if overview.OutputCoverageRate < thresholds.FeedbackIntegrityOutputCoverageRateWarn {
		pushAlert(
			"warn",
			"feedback_integrity_output_coverage_warn",
			fmt.Sprintf(
				"output覆盖率 %.2f%% 低于 warn %.2f%%",
				overview.OutputCoverageRate*100,
				thresholds.FeedbackIntegrityOutputCoverageRateWarn*100,
			),
		)
	}

	if overview.OutputResolvedRate < thresholds.FeedbackIntegrityOutputResolvedRateCritical {
		pushAlert(
			"critical",
			"feedback_integrity_output_resolved_critical",
			fmt.Sprintf(
				"output可解析率 %.2f%% 低于 critical %.2f%%",
				overview.OutputResolvedRate*100,
				thresholds.FeedbackIntegrityOutputResolvedRateCritical*100,
			),
		)
	} else if overview.OutputResolvedRate < thresholds.FeedbackIntegrityOutputResolvedRateWarn {
		pushAlert(
			"warn",
			"feedback_integrity_output_resolved_warn",
			fmt.Sprintf(
				"output可解析率 %.2f%% 低于 warn %.2f%%",
				overview.OutputResolvedRate*100,
				thresholds.FeedbackIntegrityOutputResolvedRateWarn*100,
			),
		)
	}

	if overview.OutputJobConsistencyRate < thresholds.FeedbackIntegrityOutputJobConsistencyRateCritical {
		pushAlert(
			"critical",
			"feedback_integrity_job_consistency_critical",
			fmt.Sprintf(
				"job对齐率 %.3f%% 低于 critical %.3f%%",
				overview.OutputJobConsistencyRate*100,
				thresholds.FeedbackIntegrityOutputJobConsistencyRateCritical*100,
			),
		)
	} else if overview.OutputJobConsistencyRate < thresholds.FeedbackIntegrityOutputJobConsistencyRateWarn {
		pushAlert(
			"warn",
			"feedback_integrity_job_consistency_warn",
			fmt.Sprintf(
				"job对齐率 %.3f%% 低于 warn %.3f%%",
				overview.OutputJobConsistencyRate*100,
				thresholds.FeedbackIntegrityOutputJobConsistencyRateWarn*100,
			),
		)
	}

	if overview.TopPickMultiHitUsers >= int64(thresholds.FeedbackIntegrityTopPickConflictUsersCritical) {
		pushAlert(
			"critical",
			"feedback_integrity_top_pick_conflict_critical",
			fmt.Sprintf(
				"top_pick 冲突用户数 %d 高于 critical %d",
				overview.TopPickMultiHitUsers,
				thresholds.FeedbackIntegrityTopPickConflictUsersCritical,
			),
		)
	} else if overview.TopPickMultiHitUsers >= int64(thresholds.FeedbackIntegrityTopPickConflictUsersWarn) {
		pushAlert(
			"warn",
			"feedback_integrity_top_pick_conflict_warn",
			fmt.Sprintf(
				"top_pick 冲突用户数 %d 高于 warn %d",
				overview.TopPickMultiHitUsers,
				thresholds.FeedbackIntegrityTopPickConflictUsersWarn,
			),
		)
	}

	return alerts, health
}

func buildFeedbackIntegrityRecommendations(
	alerts []AdminVideoJobFeedbackIntegrityAlert,
	health string,
) []AdminVideoJobFeedbackIntegrityRecommendation {
	type recommendationBuilder struct {
		Category        string
		Severity        string
		Title           string
		Message         string
		SuggestedQuick  string
		SuggestedAction string
		AlertCodes      map[string]struct{}
	}

	severityRank := func(value string) int {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "critical":
			return 3
		case "warn":
			return 2
		case "info":
			return 1
		default:
			return 0
		}
	}

	builders := make(map[string]*recommendationBuilder, 4)
	upsert := func(
		category string,
		severity string,
		title string,
		message string,
		suggestedQuick string,
		suggestedAction string,
		alertCode string,
	) {
		category = strings.TrimSpace(category)
		if category == "" {
			category = "general"
		}
		current, ok := builders[category]
		if !ok {
			current = &recommendationBuilder{
				Category:        category,
				Severity:        strings.ToLower(strings.TrimSpace(severity)),
				Title:           strings.TrimSpace(title),
				Message:         strings.TrimSpace(message),
				SuggestedQuick:  strings.TrimSpace(suggestedQuick),
				SuggestedAction: strings.TrimSpace(suggestedAction),
				AlertCodes:      make(map[string]struct{}, 4),
			}
			builders[category] = current
		}
		normalizedSeverity := strings.ToLower(strings.TrimSpace(severity))
		if severityRank(normalizedSeverity) > severityRank(current.Severity) {
			current.Severity = normalizedSeverity
		}
		if current.Title == "" {
			current.Title = strings.TrimSpace(title)
		}
		if current.Message == "" {
			current.Message = strings.TrimSpace(message)
		}
		if current.SuggestedQuick == "" {
			current.SuggestedQuick = strings.TrimSpace(suggestedQuick)
		}
		if current.SuggestedAction == "" {
			current.SuggestedAction = strings.TrimSpace(suggestedAction)
		}
		if code := strings.TrimSpace(alertCode); code != "" {
			current.AlertCodes[code] = struct{}{}
		}
	}

	for _, item := range alerts {
		code := strings.TrimSpace(item.Code)
		level := strings.ToLower(strings.TrimSpace(item.Level))
		switch {
		case strings.HasPrefix(code, "feedback_integrity_output_coverage_"),
			strings.HasPrefix(code, "feedback_integrity_output_resolved_"),
			strings.HasPrefix(code, "feedback_integrity_job_consistency_"):
			upsert(
				"mapping_chain",
				level,
				"检查 output 映射链路",
				"建议优先查看异常任务明细，排查 output_id 缺失、孤儿 output、job 对齐错误。",
				"feedback_anomaly",
				"open_drilldown_anomaly",
				code,
			)
		case strings.HasPrefix(code, "feedback_integrity_top_pick_conflict_"):
			upsert(
				"top_pick_conflict",
				level,
				"清理 top_pick 冲突反馈",
				"建议查看 top_pick 冲突任务，核对同一用户在同一任务的多次 top_pick 行为。",
				"top_pick_conflict",
				"open_drilldown_top_pick",
				code,
			)
		case code == "feedback_integrity_no_data":
			upsert(
				"no_data",
				"info",
				"当前窗口反馈样本不足",
				"建议放大观测窗口或积累更多用户反馈后再进行判级调优。",
				"",
				"collect_more_samples",
				code,
			)
		default:
			upsert(
				"general",
				level,
				"关注反馈完整性告警",
				"建议结合异常导出和任务明细进行排查。",
				"",
				"open_integrity_exports",
				code,
			)
		}
	}

	if len(builders) == 0 {
		normalizedHealth := strings.ToLower(strings.TrimSpace(health))
		if normalizedHealth == "green" {
			upsert(
				"healthy",
				"info",
				"反馈完整性状态健康",
				"继续保持日常巡检，关注趋势变化。",
				"",
				"none",
				"",
			)
		}
	}

	out := make([]AdminVideoJobFeedbackIntegrityRecommendation, 0, len(builders))
	for _, item := range builders {
		alertCodes := make([]string, 0, len(item.AlertCodes))
		for code := range item.AlertCodes {
			alertCodes = append(alertCodes, code)
		}
		sort.Strings(alertCodes)
		out = append(out, AdminVideoJobFeedbackIntegrityRecommendation{
			Category:        item.Category,
			Severity:        item.Severity,
			Title:           item.Title,
			Message:         item.Message,
			SuggestedQuick:  item.SuggestedQuick,
			SuggestedAction: item.SuggestedAction,
			AlertCodes:      alertCodes,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		leftRank := severityRank(out[i].Severity)
		rightRank := severityRank(out[j].Severity)
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		return out[i].Category < out[j].Category
	})
	return out
}

func buildFeedbackLearningChainStatus(enableLegacyFallback bool) AdminVideoJobFeedbackLearningChainStatus {
	status := AdminVideoJobFeedbackLearningChainStatus{
		LearningMode:                  "strict_output_candidate",
		LegacyFeedbackFallbackEnabled: enableLegacyFallback,
		LegacyEvalBackfillCandidates:  0,
	}
	if enableLegacyFallback {
		status.LearningMode = "legacy_fallback_enabled"
	}
	return status
}

func countEffectiveFeedbackIntegrityAlerts(alerts []AdminVideoJobFeedbackIntegrityAlert) int {
	if len(alerts) == 0 {
		return 0
	}
	count := 0
	for _, item := range alerts {
		code := strings.TrimSpace(item.Code)
		if code == "" || code == "feedback_integrity_no_data" {
			continue
		}
		count++
	}
	return count
}

func buildFeedbackIntegrityDelta(
	current AdminVideoJobFeedbackIntegrityOverview,
	currentHealth string,
	currentAlertCount int,
	previous AdminVideoJobFeedbackIntegrityOverview,
	previousHealth string,
	previousAlertCount int,
	previousWindowStart time.Time,
	previousWindowEnd time.Time,
) *AdminVideoJobFeedbackIntegrityDelta {
	delta := &AdminVideoJobFeedbackIntegrityDelta{
		HasPreviousData:               previous.Samples > 0,
		PreviousWindowStart:           previousWindowStart.Format(time.RFC3339),
		PreviousWindowEnd:             previousWindowEnd.Format(time.RFC3339),
		PreviousHealth:                strings.ToLower(strings.TrimSpace(previousHealth)),
		CurrentHealth:                 strings.ToLower(strings.TrimSpace(currentHealth)),
		PreviousSamples:               previous.Samples,
		CurrentSamples:                current.Samples,
		SamplesDelta:                  current.Samples - previous.Samples,
		PreviousAlertCount:            previousAlertCount,
		CurrentAlertCount:             currentAlertCount,
		AlertCountDelta:               currentAlertCount - previousAlertCount,
		OutputCoverageRateDelta:       current.OutputCoverageRate - previous.OutputCoverageRate,
		OutputResolvedRateDelta:       current.OutputResolvedRate - previous.OutputResolvedRate,
		OutputJobConsistencyRateDelta: current.OutputJobConsistencyRate - previous.OutputJobConsistencyRate,
		TopPickMultiHitUsersDelta:     current.TopPickMultiHitUsers - previous.TopPickMultiHitUsers,
	}
	return delta
}

func (h *Handler) loadVideoImageFeedbackIntegrityOverview(
	since time.Time,
	filter *videoImageFeedbackFilter,
) (AdminVideoJobFeedbackIntegrityOverview, error) {
	return h.loadVideoImageFeedbackIntegrityOverviewRange(since, time.Time{}, filter)
}

func (h *Handler) loadLegacyEvalBackfillCandidateCount() (int64, error) {
	if h == nil || h.db == nil {
		return 0, nil
	}
	var count int64
	if err := h.db.Model(&models.VideoJobGIFCandidate{}).
		Where("COALESCE(feature_json->>'version', '') = ?", "legacy_eval_backfill_v1").
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (h *Handler) loadVideoImageFeedbackIntegrityOverviewRange(
	since time.Time,
	until time.Time,
	filter *videoImageFeedbackFilter,
) (AdminVideoJobFeedbackIntegrityOverview, error) {
	type row struct {
		Samples              int64 `gorm:"column:samples"`
		WithOutputID         int64 `gorm:"column:with_output_id"`
		MissingOutputID      int64 `gorm:"column:missing_output_id"`
		ResolvedOutput       int64 `gorm:"column:resolved_output"`
		OrphanOutput         int64 `gorm:"column:orphan_output"`
		JobMismatch          int64 `gorm:"column:job_mismatch"`
		TopPickMultiHitUsers int64 `gorm:"column:top_pick_multi_hit_users"`
	}

	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQL(filter)
	untilSQL := ""
	args := make([]interface{}, 0, 2+len(filterArgs))
	args = append(args, since)
	if !until.IsZero() {
		untilSQL = " AND f.created_at < ?"
		args = append(args, until)
	}
	args = append(args, filterArgs...)
	query := `
WITH base AS (
	SELECT
		f.id,
		f.job_id,
		f.user_id,
		f.output_id,
		LOWER(COALESCE(f.action, '')) AS action
	FROM public.video_image_feedback f
	WHERE f.created_at >= ?
` + untilSQL + `
` + filterSQL + `
),
joined AS (
	SELECT
		b.*,
		o.id AS output_exists_id,
		o.job_id AS output_job_id
	FROM base b
	LEFT JOIN public.video_image_outputs o ON o.id = b.output_id
),
top_pick_conflict AS (
	SELECT COUNT(*)::bigint AS users
	FROM (
		SELECT job_id, user_id
		FROM base
		WHERE action = 'top_pick'
		GROUP BY job_id, user_id
		HAVING COUNT(*) > 1
	) t
)
SELECT
	COUNT(*)::bigint AS samples,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL)::bigint AS with_output_id,
	COUNT(*) FILTER (WHERE output_id IS NULL)::bigint AS missing_output_id,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::bigint AS resolved_output,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NULL)::bigint AS orphan_output,
	COUNT(*) FILTER (
		WHERE output_id IS NOT NULL
			AND output_exists_id IS NOT NULL
			AND output_job_id <> job_id
	)::bigint AS job_mismatch,
	COALESCE((SELECT users FROM top_pick_conflict), 0)::bigint AS top_pick_multi_hit_users
FROM joined
`

	var dbRow row
	if err := h.db.Raw(query, args...).Scan(&dbRow).Error; err != nil {
		return AdminVideoJobFeedbackIntegrityOverview{}, err
	}

	out := AdminVideoJobFeedbackIntegrityOverview{
		Samples:              dbRow.Samples,
		WithOutputID:         dbRow.WithOutputID,
		MissingOutputID:      dbRow.MissingOutputID,
		ResolvedOutput:       dbRow.ResolvedOutput,
		OrphanOutput:         dbRow.OrphanOutput,
		JobMismatch:          dbRow.JobMismatch,
		TopPickMultiHitUsers: dbRow.TopPickMultiHitUsers,
	}
	if out.Samples > 0 {
		out.OutputCoverageRate = float64(out.WithOutputID) / float64(out.Samples)
	}
	if out.WithOutputID > 0 {
		out.OutputResolvedRate = float64(out.ResolvedOutput) / float64(out.WithOutputID)
		consistent := out.ResolvedOutput - out.JobMismatch
		if consistent < 0 {
			consistent = 0
		}
		out.OutputJobConsistencyRate = float64(consistent) / float64(out.WithOutputID)
	}
	return out, nil
}

func (h *Handler) loadVideoImageFeedbackIntegrityHealthTrend(
	since time.Time,
	until time.Time,
	thresholds FeedbackIntegrityAlertThresholdSettings,
	filter *videoImageFeedbackFilter,
) ([]AdminVideoJobFeedbackIntegrityHealthTrendPoint, error) {
	type row struct {
		BucketDay            time.Time `gorm:"column:bucket_day"`
		Samples              int64     `gorm:"column:samples"`
		WithOutputID         int64     `gorm:"column:with_output_id"`
		ResolvedOutput       int64     `gorm:"column:resolved_output"`
		JobMismatch          int64     `gorm:"column:job_mismatch"`
		TopPickMultiHitUsers int64     `gorm:"column:top_pick_multi_hit_users"`
	}

	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQL(filter)
	query := `
WITH base AS (
	SELECT
		date_trunc('day', f.created_at) AS bucket_day,
		f.job_id,
		f.user_id,
		f.output_id,
		LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action
	FROM public.video_image_feedback f
	WHERE f.created_at >= ?
		AND f.created_at <= ?
` + filterSQL + `
),
joined AS (
	SELECT
		b.*,
		o.id AS output_exists_id,
		o.job_id AS output_job_id
	FROM base b
	LEFT JOIN public.video_image_outputs o ON o.id = b.output_id
),
daily_agg AS (
	SELECT
		bucket_day,
		COUNT(*)::bigint AS samples,
		COUNT(*) FILTER (WHERE output_id IS NOT NULL)::bigint AS with_output_id,
		COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::bigint AS resolved_output,
		COUNT(*) FILTER (
			WHERE output_id IS NOT NULL
				AND output_exists_id IS NOT NULL
				AND output_job_id <> job_id
		)::bigint AS job_mismatch
	FROM joined
	GROUP BY bucket_day
),
daily_top_pick_conflict AS (
	SELECT
		bucket_day,
		COUNT(*)::bigint AS users
	FROM (
		SELECT
			bucket_day,
			job_id,
			user_id
		FROM base
		WHERE action = 'top_pick'
		GROUP BY bucket_day, job_id, user_id
		HAVING COUNT(*) > 1
	) t
	GROUP BY bucket_day
),
days AS (
	SELECT generate_series(
		date_trunc('day', ?::timestamp),
		date_trunc('day', ?::timestamp),
		interval '1 day'
	) AS bucket_day
)
SELECT
	days.bucket_day,
	COALESCE(daily_agg.samples, 0)::bigint AS samples,
	COALESCE(daily_agg.with_output_id, 0)::bigint AS with_output_id,
	COALESCE(daily_agg.resolved_output, 0)::bigint AS resolved_output,
	COALESCE(daily_agg.job_mismatch, 0)::bigint AS job_mismatch,
	COALESCE(daily_top_pick_conflict.users, 0)::bigint AS top_pick_multi_hit_users
FROM days
LEFT JOIN daily_agg ON daily_agg.bucket_day = days.bucket_day
LEFT JOIN daily_top_pick_conflict ON daily_top_pick_conflict.bucket_day = days.bucket_day
ORDER BY days.bucket_day ASC
`
	args := []interface{}{since, until}
	args = append(args, filterArgs...)
	args = append(args, since, until)

	var rows []row
	if err := h.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobFeedbackIntegrityHealthTrendPoint, 0, len(rows))
	for _, item := range rows {
		overview := AdminVideoJobFeedbackIntegrityOverview{
			Samples:              item.Samples,
			WithOutputID:         item.WithOutputID,
			ResolvedOutput:       item.ResolvedOutput,
			JobMismatch:          item.JobMismatch,
			TopPickMultiHitUsers: item.TopPickMultiHitUsers,
		}
		if overview.Samples > 0 {
			overview.OutputCoverageRate = float64(overview.WithOutputID) / float64(overview.Samples)
		}
		if overview.WithOutputID > 0 {
			overview.OutputResolvedRate = float64(overview.ResolvedOutput) / float64(overview.WithOutputID)
			consistent := overview.ResolvedOutput - overview.JobMismatch
			if consistent < 0 {
				consistent = 0
			}
			overview.OutputJobConsistencyRate = float64(consistent) / float64(overview.WithOutputID)
		}
		alerts, health := buildVideoJobFeedbackIntegrityAlerts(overview, thresholds)
		alertCount := len(alerts)
		alertCodes := make([]string, 0, len(alerts))
		for _, alert := range alerts {
			code := strings.TrimSpace(alert.Code)
			if code == "" || code == "feedback_integrity_no_data" {
				continue
			}
			alertCodes = append(alertCodes, code)
		}
		if len(alertCodes) > 1 {
			sort.Strings(alertCodes)
			unique := alertCodes[:0]
			for _, code := range alertCodes {
				if len(unique) == 0 || unique[len(unique)-1] != code {
					unique = append(unique, code)
				}
			}
			alertCodes = unique
		}
		if overview.Samples <= 0 {
			health = "no_data"
			alertCount = 0
			alertCodes = nil
		}
		out = append(out, AdminVideoJobFeedbackIntegrityHealthTrendPoint{
			Bucket:                   item.BucketDay.Format("2006-01-02"),
			Samples:                  overview.Samples,
			Health:                   health,
			AlertCount:               alertCount,
			OutputCoverageRate:       overview.OutputCoverageRate,
			OutputResolvedRate:       overview.OutputResolvedRate,
			OutputJobConsistencyRate: overview.OutputJobConsistencyRate,
			TopPickMultiHitUsers:     overview.TopPickMultiHitUsers,
			AlertCodes:               alertCodes,
		})
	}
	return out, nil
}

func resolveFeedbackIntegrityRecovery(
	trend []AdminVideoJobFeedbackIntegrityHealthTrendPoint,
) (status string, recovered bool, previousHealth string) {
	if len(trend) == 0 {
		return "no_data", false, ""
	}

	lastIndex := -1
	for i := len(trend) - 1; i >= 0; i-- {
		health := strings.ToLower(strings.TrimSpace(trend[i].Health))
		if health == "" || health == "no_data" {
			continue
		}
		lastIndex = i
		break
	}
	if lastIndex < 0 {
		return "no_data", false, ""
	}

	currentHealth := strings.ToLower(strings.TrimSpace(trend[lastIndex].Health))
	previousHealth = ""
	for i := lastIndex - 1; i >= 0; i-- {
		health := strings.ToLower(strings.TrimSpace(trend[i].Health))
		if health == "" || health == "no_data" {
			continue
		}
		previousHealth = health
		break
	}
	if previousHealth == "" {
		return "insufficient_history", false, ""
	}

	if currentHealth == "green" && (previousHealth == "yellow" || previousHealth == "red") {
		return "recovered", true, previousHealth
	}
	if (currentHealth == "yellow" || currentHealth == "red") && previousHealth == "green" {
		return "regressed", false, previousHealth
	}
	return "stable", false, previousHealth
}

func buildFeedbackIntegrityAlertCodeStats(
	trend []AdminVideoJobFeedbackIntegrityHealthTrendPoint,
) []AdminVideoJobFeedbackIntegrityAlertCodeStat {
	type agg struct {
		DaysHit      int
		LatestLevel  string
		LatestBucket string
	}
	acc := make(map[string]agg, 16)

	for _, point := range trend {
		bucket := strings.TrimSpace(point.Bucket)
		level := strings.ToLower(strings.TrimSpace(point.Health))
		codes := point.AlertCodes
		if len(codes) == 0 {
			continue
		}
		seen := make(map[string]struct{}, len(codes))
		for _, raw := range codes {
			code := strings.TrimSpace(raw)
			if code == "" {
				continue
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			current := acc[code]
			current.DaysHit++
			if current.LatestBucket == "" || bucket > current.LatestBucket {
				current.LatestBucket = bucket
				current.LatestLevel = level
			}
			acc[code] = current
		}
	}

	out := make([]AdminVideoJobFeedbackIntegrityAlertCodeStat, 0, len(acc))
	for code, item := range acc {
		out = append(out, AdminVideoJobFeedbackIntegrityAlertCodeStat{
			Code:         code,
			DaysHit:      item.DaysHit,
			LatestLevel:  item.LatestLevel,
			LatestBucket: item.LatestBucket,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DaysHit != out[j].DaysHit {
			return out[i].DaysHit > out[j].DaysHit
		}
		if out[i].LatestBucket != out[j].LatestBucket {
			return out[i].LatestBucket > out[j].LatestBucket
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func buildFeedbackIntegrityStreaks(
	trend []AdminVideoJobFeedbackIntegrityHealthTrendPoint,
) AdminVideoJobFeedbackIntegrityStreaks {
	out := AdminVideoJobFeedbackIntegrityStreaks{}
	if len(trend) == 0 {
		return out
	}

	normalize := func(value string) string {
		return strings.ToLower(strings.TrimSpace(value))
	}

	for _, point := range trend {
		health := normalize(point.Health)
		if health == "red" || health == "yellow" {
			out.LastNonGreenBucket = strings.TrimSpace(point.Bucket)
		}
		if health == "red" {
			out.LastRedBucket = strings.TrimSpace(point.Bucket)
		}
	}

	recent := trend
	if len(recent) > 7 {
		recent = recent[len(recent)-7:]
	}
	for _, point := range recent {
		health := normalize(point.Health)
		if health == "red" {
			out.Recent7dRedDays++
			out.Recent7dNonGreenDays++
		} else if health == "yellow" {
			out.Recent7dNonGreenDays++
		}
	}

	for i := len(trend) - 1; i >= 0; i-- {
		health := normalize(trend[i].Health)
		if health == "red" {
			out.ConsecutiveRedDays++
		} else {
			break
		}
	}
	for i := len(trend) - 1; i >= 0; i-- {
		health := normalize(trend[i].Health)
		if health == "red" || health == "yellow" {
			out.ConsecutiveNonGreenDays++
		} else {
			break
		}
	}
	for i := len(trend) - 1; i >= 0; i-- {
		health := normalize(trend[i].Health)
		if health == "green" {
			out.ConsecutiveGreenDays++
		} else {
			break
		}
	}
	return out
}

func buildFeedbackIntegrityEscalation(
	health string,
	streaks AdminVideoJobFeedbackIntegrityStreaks,
	delta *AdminVideoJobFeedbackIntegrityDelta,
	recoveryStatus string,
) AdminVideoJobFeedbackIntegrityEscalation {
	normalizedHealth := strings.ToLower(strings.TrimSpace(health))
	normalizedRecovery := strings.ToLower(strings.TrimSpace(recoveryStatus))

	triggered := make([]string, 0, 6)
	reasonParts := make([]string, 0, 4)
	levelRank := func(value string) int {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "oncall":
			return 3
		case "watch":
			return 2
		case "notice":
			return 1
		default:
			return 0
		}
	}
	level := "none"
	setLevel := func(target string) {
		if levelRank(target) > levelRank(level) {
			level = target
		}
	}
	addRule := func(rule string, reason string) {
		rule = strings.TrimSpace(rule)
		if rule != "" {
			triggered = append(triggered, rule)
		}
		reason = strings.TrimSpace(reason)
		if reason != "" {
			reasonParts = append(reasonParts, reason)
		}
	}

	if normalizedHealth == "red" && streaks.ConsecutiveRedDays >= 2 {
		setLevel("oncall")
		addRule("consecutive_red_2d", "连续红色 >=2 天")
	}
	if normalizedHealth == "red" && streaks.Recent7dRedDays >= 3 {
		setLevel("oncall")
		addRule("recent_7d_red_3d", "近7天红色 >=3 天")
	}
	if delta != nil && delta.AlertCountDelta >= 2 && normalizedHealth != "green" {
		setLevel("watch")
		addRule("alert_count_growth", "告警数量较上一窗口明显上升")
	}
	if delta != nil && delta.TopPickMultiHitUsersDelta > 0 {
		setLevel("watch")
		addRule("top_pick_conflict_growth", "top_pick 冲突用户上升")
	}
	if streaks.ConsecutiveNonGreenDays >= 3 && normalizedHealth != "green" {
		setLevel("watch")
		addRule("consecutive_non_green_3d", "连续非绿 >=3 天")
	}
	if normalizedRecovery == "recovered" && levelRank(level) > levelRank("notice") {
		// 已恢复时降级一级，避免误报持续升级
		level = "notice"
		addRule("recovered_downgrade", "当前已恢复，降级为观察通知")
	}

	required := level == "watch" || level == "oncall"
	if len(triggered) == 0 {
		if normalizedHealth == "green" {
			return AdminVideoJobFeedbackIntegrityEscalation{
				Required: false,
				Level:    "none",
				Reason:   "当前完整性健康，无需升级处理",
			}
		}
		if normalizedHealth == "yellow" {
			return AdminVideoJobFeedbackIntegrityEscalation{
				Required: false,
				Level:    "notice",
				Reason:   "存在轻度告警，建议持续观察",
			}
		}
		if normalizedHealth == "red" {
			return AdminVideoJobFeedbackIntegrityEscalation{
				Required: true,
				Level:    "watch",
				Reason:   "当前为红色健康状态，建议立即排障",
			}
		}
		return AdminVideoJobFeedbackIntegrityEscalation{
			Required: false,
			Level:    "none",
			Reason:   "当前窗口反馈样本不足",
		}
	}

	// 去重 triggered rules，保留原始顺序
	dedup := make([]string, 0, len(triggered))
	seen := make(map[string]struct{}, len(triggered))
	for _, rule := range triggered {
		if _, ok := seen[rule]; ok {
			continue
		}
		seen[rule] = struct{}{}
		dedup = append(dedup, rule)
	}
	reasonText := strings.Join(reasonParts, "；")
	if strings.TrimSpace(reasonText) == "" {
		reasonText = "满足升级规则"
	}

	return AdminVideoJobFeedbackIntegrityEscalation{
		Required:       required,
		Level:          level,
		Reason:         reasonText,
		TriggeredRules: dedup,
	}
}

func deriveFeedbackIntegrityRecoveryStatusForPoint(currentHealth, previousHealth string) string {
	current := strings.ToLower(strings.TrimSpace(currentHealth))
	previous := strings.ToLower(strings.TrimSpace(previousHealth))
	if current == "" || current == "no_data" {
		return "no_data"
	}
	if previous == "" || previous == "no_data" {
		return "stable"
	}
	if (previous == "red" || previous == "yellow") && current == "green" {
		return "recovered"
	}
	if previous == "green" && (current == "red" || current == "yellow") {
		return "regressed"
	}
	return "stable"
}

func buildFeedbackIntegrityEscalationTrend(
	trend []AdminVideoJobFeedbackIntegrityHealthTrendPoint,
) []AdminVideoJobFeedbackIntegrityEscalationTrendPoint {
	if len(trend) == 0 {
		return nil
	}
	out := make([]AdminVideoJobFeedbackIntegrityEscalationTrendPoint, 0, len(trend))
	for idx, point := range trend {
		previousHealth := ""
		if idx > 0 {
			previousHealth = trend[idx-1].Health
		}
		recoveryStatus := deriveFeedbackIntegrityRecoveryStatusForPoint(point.Health, previousHealth)
		streaks := buildFeedbackIntegrityStreaks(trend[:idx+1])
		var delta *AdminVideoJobFeedbackIntegrityDelta
		alertCountDelta := 0
		topPickConflictDelta := int64(0)
		if idx > 0 {
			alertCountDelta = point.AlertCount - trend[idx-1].AlertCount
			topPickConflictDelta = point.TopPickMultiHitUsers - trend[idx-1].TopPickMultiHitUsers
			delta = &AdminVideoJobFeedbackIntegrityDelta{
				HasPreviousData:           true,
				AlertCountDelta:           alertCountDelta,
				TopPickMultiHitUsersDelta: topPickConflictDelta,
			}
		}
		escalation := buildFeedbackIntegrityEscalation(point.Health, streaks, delta, recoveryStatus)
		out = append(out, AdminVideoJobFeedbackIntegrityEscalationTrendPoint{
			Bucket:                    strings.TrimSpace(point.Bucket),
			Health:                    strings.ToLower(strings.TrimSpace(point.Health)),
			RecoveryStatus:            recoveryStatus,
			AlertCount:                point.AlertCount,
			AlertCountDelta:           alertCountDelta,
			TopPickMultiHitUsers:      point.TopPickMultiHitUsers,
			TopPickMultiHitUsersDelta: topPickConflictDelta,
			EscalationLevel:           escalation.Level,
			EscalationRequired:        escalation.Required,
			EscalationReason:          escalation.Reason,
			TriggeredRules:            escalation.TriggeredRules,
		})
	}
	return out
}

func buildFeedbackIntegrityEscalationStats(
	points []AdminVideoJobFeedbackIntegrityEscalationTrendPoint,
) AdminVideoJobFeedbackIntegrityEscalationStats {
	stats := AdminVideoJobFeedbackIntegrityEscalationStats{}
	if len(points) == 0 {
		return stats
	}
	stats.TotalDays = len(points)
	for _, item := range points {
		if item.EscalationRequired {
			stats.RequiredDays++
		}
		switch strings.ToLower(strings.TrimSpace(item.EscalationLevel)) {
		case "oncall":
			stats.OncallDays++
		case "watch":
			stats.WatchDays++
		case "notice":
			stats.NoticeDays++
		default:
			stats.NoneDays++
		}
	}
	last := points[len(points)-1]
	stats.LatestBucket = strings.TrimSpace(last.Bucket)
	stats.LatestLevel = strings.ToLower(strings.TrimSpace(last.EscalationLevel))
	stats.LatestRequired = last.EscalationRequired
	stats.LatestReason = strings.TrimSpace(last.EscalationReason)
	return stats
}

func buildFeedbackIntegrityEscalationIncidents(
	points []AdminVideoJobFeedbackIntegrityEscalationTrendPoint,
	limit int,
) []AdminVideoJobFeedbackIntegrityEscalationIncident {
	if len(points) == 0 || limit == 0 {
		return nil
	}
	if limit < 0 {
		limit = 0
	}
	capHint := len(points)
	if limit > 0 && limit < capHint {
		capHint = limit
	}
	out := make([]AdminVideoJobFeedbackIntegrityEscalationIncident, 0, capHint)
	for idx := len(points) - 1; idx >= 0; idx-- {
		item := points[idx]
		if !item.EscalationRequired {
			continue
		}
		out = append(out, AdminVideoJobFeedbackIntegrityEscalationIncident{
			Bucket:                    strings.TrimSpace(item.Bucket),
			EscalationLevel:           strings.ToLower(strings.TrimSpace(item.EscalationLevel)),
			EscalationRequired:        item.EscalationRequired,
			EscalationReason:          strings.TrimSpace(item.EscalationReason),
			TriggeredRules:            item.TriggeredRules,
			AlertCount:                item.AlertCount,
			AlertCountDelta:           item.AlertCountDelta,
			TopPickMultiHitUsers:      item.TopPickMultiHitUsers,
			TopPickMultiHitUsersDelta: item.TopPickMultiHitUsersDelta,
			RecoveryStatus:            strings.ToLower(strings.TrimSpace(item.RecoveryStatus)),
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (h *Handler) loadVideoImageFeedbackIntegrityRiskJobCounts(
	since time.Time,
	filter *videoImageFeedbackFilter,
) (anomalyJobs int64, topPickConflictJobs int64, err error) {
	type row struct {
		AnomalyJobs         int64 `gorm:"column:anomaly_jobs"`
		TopPickConflictJobs int64 `gorm:"column:top_pick_conflict_jobs"`
	}

	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQL(filter)
	query := `
WITH base AS (
	SELECT
		f.id,
		f.job_id,
		f.user_id,
		f.output_id,
		LOWER(COALESCE(f.action, '')) AS action
	FROM public.video_image_feedback f
	WHERE f.created_at >= ?
` + filterSQL + `
),
joined AS (
	SELECT
		b.*,
		o.id AS output_exists_id,
		o.job_id AS output_job_id
	FROM base b
	LEFT JOIN public.video_image_outputs o ON o.id = b.output_id
),
anomaly_jobs AS (
	SELECT COUNT(DISTINCT job_id)::bigint AS jobs
	FROM joined
	WHERE output_id IS NULL
		OR (output_id IS NOT NULL AND output_exists_id IS NULL)
		OR (output_id IS NOT NULL AND output_exists_id IS NOT NULL AND output_job_id <> job_id)
),
top_pick_conflict_jobs AS (
	SELECT COUNT(DISTINCT job_id)::bigint AS jobs
	FROM (
		SELECT job_id, user_id
		FROM base
		WHERE action = 'top_pick'
		GROUP BY job_id, user_id
		HAVING COUNT(*) > 1
	) t
)
SELECT
	COALESCE((SELECT jobs FROM anomaly_jobs), 0)::bigint AS anomaly_jobs,
	COALESCE((SELECT jobs FROM top_pick_conflict_jobs), 0)::bigint AS top_pick_conflict_jobs
`
	args := []interface{}{since}
	args = append(args, filterArgs...)

	var dbRow row
	if err = h.db.Raw(query, args...).Scan(&dbRow).Error; err != nil {
		return 0, 0, err
	}
	return dbRow.AnomalyJobs, dbRow.TopPickConflictJobs, nil
}

type adminVideoImageFeedbackIntegrityActionRow struct {
	Action                   string
	Samples                  int64
	WithOutputID             int64
	MissingOutputID          int64
	ResolvedOutput           int64
	OrphanOutput             int64
	JobMismatch              int64
	OutputCoverageRate       float64
	OutputResolvedRate       float64
	OutputJobConsistencyRate float64
}

func (h *Handler) loadVideoImageFeedbackIntegrityActionRows(
	since time.Time,
	filter *videoImageFeedbackFilter,
	limit int,
) ([]adminVideoImageFeedbackIntegrityActionRow, error) {
	type row struct {
		Action          string `gorm:"column:action"`
		Samples         int64  `gorm:"column:samples"`
		WithOutputID    int64  `gorm:"column:with_output_id"`
		MissingOutputID int64  `gorm:"column:missing_output_id"`
		ResolvedOutput  int64  `gorm:"column:resolved_output"`
		OrphanOutput    int64  `gorm:"column:orphan_output"`
		JobMismatch     int64  `gorm:"column:job_mismatch"`
	}

	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQL(filter)
	query := `
WITH base AS (
	SELECT
		f.id,
		f.job_id,
		f.output_id,
		LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action
	FROM public.video_image_feedback f
	WHERE f.created_at >= ?
` + filterSQL + `
),
joined AS (
	SELECT
		b.*,
		o.id AS output_exists_id,
		o.job_id AS output_job_id
	FROM base b
	LEFT JOIN public.video_image_outputs o ON o.id = b.output_id
)
SELECT
	action,
	COUNT(*)::bigint AS samples,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL)::bigint AS with_output_id,
	COUNT(*) FILTER (WHERE output_id IS NULL)::bigint AS missing_output_id,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::bigint AS resolved_output,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NULL)::bigint AS orphan_output,
	COUNT(*) FILTER (
		WHERE output_id IS NOT NULL
			AND output_exists_id IS NOT NULL
			AND output_job_id <> job_id
	)::bigint AS job_mismatch
FROM joined
GROUP BY action
ORDER BY samples DESC, action ASC
`
	args := []interface{}{since}
	args = append(args, filterArgs...)
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	var rows []row
	if err := h.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]adminVideoImageFeedbackIntegrityActionRow, 0, len(rows))
	for _, item := range rows {
		rowOut := adminVideoImageFeedbackIntegrityActionRow{
			Action:          strings.TrimSpace(item.Action),
			Samples:         item.Samples,
			WithOutputID:    item.WithOutputID,
			MissingOutputID: item.MissingOutputID,
			ResolvedOutput:  item.ResolvedOutput,
			OrphanOutput:    item.OrphanOutput,
			JobMismatch:     item.JobMismatch,
		}
		if rowOut.Samples > 0 {
			rowOut.OutputCoverageRate = float64(rowOut.WithOutputID) / float64(rowOut.Samples)
		}
		if rowOut.WithOutputID > 0 {
			rowOut.OutputResolvedRate = float64(rowOut.ResolvedOutput) / float64(rowOut.WithOutputID)
			consistent := rowOut.ResolvedOutput - rowOut.JobMismatch
			if consistent < 0 {
				consistent = 0
			}
			rowOut.OutputJobConsistencyRate = float64(consistent) / float64(rowOut.WithOutputID)
		}
		out = append(out, rowOut)
	}
	return out, nil
}

func (h *Handler) loadVideoImageFeedbackActionStats(
	since time.Time,
	filter *videoImageFeedbackFilter,
) ([]AdminVideoJobFeedbackActionStat, error) {
	type row struct {
		Action    string   `gorm:"column:action"`
		Count     int64    `gorm:"column:count"`
		WeightSum *float64 `gorm:"column:weight_sum"`
	}

	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQL(filter)
	query := `
SELECT
	LOWER(COALESCE(NULLIF(TRIM(action), ''), 'unknown')) AS action,
	COUNT(*)::bigint AS count,
	COALESCE(SUM(weight), 0)::double precision AS weight_sum
FROM public.video_image_feedback f
WHERE f.created_at >= ?
` + filterSQL + `
GROUP BY 1
ORDER BY count DESC, action ASC
LIMIT 16
`

	args := []interface{}{since}
	args = append(args, filterArgs...)

	var rows []row
	if err := h.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	total := int64(0)
	for _, item := range rows {
		total += item.Count
	}

	out := make([]AdminVideoJobFeedbackActionStat, 0, len(rows))
	for _, item := range rows {
		ratio := 0.0
		if total > 0 {
			ratio = float64(item.Count) / float64(total)
		}
		weightSum := 0.0
		if item.WeightSum != nil {
			weightSum = *item.WeightSum
		}
		out = append(out, AdminVideoJobFeedbackActionStat{
			Action:    strings.TrimSpace(item.Action),
			Count:     item.Count,
			Ratio:     ratio,
			WeightSum: weightSum,
		})
	}
	return out, nil
}

func (h *Handler) loadVideoImageFeedbackTopSceneStats(
	since time.Time,
	filter *videoImageFeedbackFilter,
) ([]AdminVideoJobFeedbackSceneStat, error) {
	type row struct {
		SceneTag string `gorm:"column:scene_tag"`
		Signals  int64  `gorm:"column:signals"`
	}

	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQL(filter)
	query := `
SELECT
	COALESCE(
		NULLIF(LOWER(TRIM(scene_tag)), ''),
		NULLIF(LOWER(TRIM(metadata->>'scene_tag')), ''),
		'uncategorized'
	) AS scene_tag,
	COUNT(*)::bigint AS signals
FROM public.video_image_feedback f
WHERE f.created_at >= ?
` + filterSQL + `
GROUP BY 1
ORDER BY signals DESC, scene_tag ASC
LIMIT 16
`
	args := []interface{}{since}
	args = append(args, filterArgs...)

	var rows []row
	if err := h.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobFeedbackSceneStat, 0, len(rows))
	for _, item := range rows {
		out = append(out, AdminVideoJobFeedbackSceneStat{
			SceneTag: strings.TrimSpace(item.SceneTag),
			Signals:  item.Signals,
		})
	}
	return out, nil
}

func (h *Handler) loadVideoImageFeedbackTrend(
	since,
	until time.Time,
	windowDuration time.Duration,
	filter *videoImageFeedbackFilter,
) ([]AdminVideoJobFeedbackTrendPoint, error) {
	precision := "day"
	step := 24 * time.Hour
	if windowDuration <= 24*time.Hour {
		precision = "hour"
		step = time.Hour
	}

	type row struct {
		BucketTS time.Time `gorm:"column:bucket_ts"`
		Total    int64     `gorm:"column:total"`
		Positive int64     `gorm:"column:positive"`
		Neutral  int64     `gorm:"column:neutral"`
		Negative int64     `gorm:"column:negative"`
		TopPick  int64     `gorm:"column:top_pick"`
	}

	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQL(filter)
	query := `
SELECT
	date_trunc(?, f.created_at) AS bucket_ts,
	COUNT(*)::bigint AS total,
	COUNT(*) FILTER (
		WHERE LOWER(COALESCE(action, '')) IN ('download', 'favorite', 'share', 'use', 'like', 'top_pick')
	)::bigint AS positive,
	COUNT(*) FILTER (WHERE LOWER(COALESCE(action, '')) = 'neutral')::bigint AS neutral,
	COUNT(*) FILTER (WHERE LOWER(COALESCE(action, '')) = 'dislike')::bigint AS negative,
	COUNT(*) FILTER (WHERE LOWER(COALESCE(action, '')) = 'top_pick')::bigint AS top_pick
FROM public.video_image_feedback f
WHERE f.created_at >= ?
	AND f.created_at <= ?
` + filterSQL + `
GROUP BY 1
ORDER BY 1 ASC
`
	args := []interface{}{precision, since, until}
	args = append(args, filterArgs...)

	var rows []row
	if err := h.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	align := func(t time.Time) time.Time {
		if precision == "hour" {
			return t.Truncate(time.Hour)
		}
		year, month, day := t.Date()
		return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
	}

	rowByBucket := make(map[int64]row, len(rows))
	for _, item := range rows {
		bucket := align(item.BucketTS).Unix()
		rowByBucket[bucket] = item
	}

	start := align(since)
	end := align(until)
	if end.Before(start) {
		end = start
	}

	out := make([]AdminVideoJobFeedbackTrendPoint, 0, len(rows))
	for cursor := start; !cursor.After(end); cursor = cursor.Add(step) {
		bucket := align(cursor)
		key := bucket.Unix()
		item, ok := rowByBucket[key]
		if !ok {
			out = append(out, AdminVideoJobFeedbackTrendPoint{
				Bucket: bucket.Format(time.RFC3339),
			})
			continue
		}
		out = append(out, AdminVideoJobFeedbackTrendPoint{
			Bucket:   bucket.Format(time.RFC3339),
			Total:    item.Total,
			Positive: item.Positive,
			Neutral:  item.Neutral,
			Negative: item.Negative,
			TopPick:  item.TopPick,
		})
	}
	return out, nil
}

func (h *Handler) loadVideoJobFeedbackNegativeGuardOverview(
	since time.Time,
	filter *videoImageFeedbackFilter,
) (AdminVideoJobFeedbackNegativeGuardOverview, error) {
	type row struct {
		Samples            int64    `gorm:"column:samples"`
		TreatmentJobs      int64    `gorm:"column:treatment_jobs"`
		GuardEnabledJobs   int64    `gorm:"column:guard_enabled_jobs"`
		GuardReasonHitJobs int64    `gorm:"column:guard_reason_hit_jobs"`
		SelectionShiftJobs int64    `gorm:"column:selection_shift_jobs"`
		BlockedReasonJobs  int64    `gorm:"column:blocked_reason_jobs"`
		AvgNegativeSignals *float64 `gorm:"column:avg_negative_signals"`
		AvgPositiveSignals *float64 `gorm:"column:avg_positive_signals"`
	}

	var dbRow row
	filterSQL, filterArgs := buildVideoJobFilterClause(filter, "j")
	query := `
WITH base AS (
	SELECT
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->>'group'), ''), 'unknown')) AS group_name,
		CASE
			WHEN LOWER(COALESCE(j.metrics->'highlight_feedback_v1'->>'negative_guard_enabled', 'false')) IN ('true', '1', 't', 'yes', 'y') THEN 1
			ELSE 0
		END AS guard_enabled,
		CASE
			WHEN jsonb_typeof(j.metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
				AND (j.metrics->'highlight_feedback_v1'->'reason_negative_guard') <> '{}'::jsonb
			THEN 1
			ELSE 0
		END AS guard_reason_hit,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->>'public_negative_signals', '')::double precision, 0) AS public_negative_signals,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->>'public_positive_signals', '')::double precision, 0) AS public_positive_signals,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), '')) AS before_reason,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_after'->>'reason'), ''), '')) AS after_reason,
		COALESCE(j.metrics->'highlight_feedback_v1'->'reason_negative_guard', '{}'::jsonb) AS reason_negative_guard,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->'selected_before'->>'start_sec', '')::double precision, -1) AS before_start_sec,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->'selected_before'->>'end_sec', '')::double precision, -1) AS before_end_sec,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->'selected_after'->>'start_sec', '')::double precision, -1) AS after_start_sec,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->'selected_after'->>'end_sec', '')::double precision, -1) AS after_end_sec
	FROM archive.video_jobs j
	WHERE j.status = ?
	  AND j.finished_at >= ?
` + filterSQL + `
)
SELECT
	COUNT(*) FILTER (WHERE group_name IN ('treatment', 'control'))::bigint AS samples,
	COUNT(*) FILTER (WHERE group_name = 'treatment')::bigint AS treatment_jobs,
	COUNT(*) FILTER (WHERE group_name = 'treatment' AND guard_enabled = 1)::bigint AS guard_enabled_jobs,
	COUNT(*) FILTER (WHERE group_name = 'treatment' AND guard_reason_hit = 1)::bigint AS guard_reason_hit_jobs,
	COUNT(*) FILTER (
		WHERE group_name = 'treatment'
			AND guard_reason_hit = 1
			AND (ABS(before_start_sec - after_start_sec) > 0.0001 OR ABS(before_end_sec - after_end_sec) > 0.0001)
	)::bigint AS selection_shift_jobs,
	COUNT(*) FILTER (
		WHERE group_name = 'treatment'
			AND guard_reason_hit = 1
			AND before_reason <> ''
			AND (reason_negative_guard ? before_reason)
			AND after_reason <> ''
			AND after_reason <> before_reason
	)::bigint AS blocked_reason_jobs,
	AVG(public_negative_signals) FILTER (WHERE group_name = 'treatment' AND guard_reason_hit = 1) AS avg_negative_signals,
	AVG(public_positive_signals) FILTER (WHERE group_name = 'treatment' AND guard_reason_hit = 1) AS avg_positive_signals
FROM base
`
	args := []interface{}{models.VideoJobStatusDone, since}
	args = append(args, filterArgs...)
	if err := h.db.Raw(query, args...).Scan(&dbRow).Error; err != nil {
		return AdminVideoJobFeedbackNegativeGuardOverview{}, err
	}

	out := AdminVideoJobFeedbackNegativeGuardOverview{
		Samples:            dbRow.Samples,
		TreatmentJobs:      dbRow.TreatmentJobs,
		GuardEnabledJobs:   dbRow.GuardEnabledJobs,
		GuardReasonHitJobs: dbRow.GuardReasonHitJobs,
		SelectionShiftJobs: dbRow.SelectionShiftJobs,
		BlockedReasonJobs:  dbRow.BlockedReasonJobs,
	}
	if dbRow.AvgNegativeSignals != nil {
		out.AvgNegativeSignals = *dbRow.AvgNegativeSignals
	}
	if dbRow.AvgPositiveSignals != nil {
		out.AvgPositiveSignals = *dbRow.AvgPositiveSignals
	}

	if out.GuardEnabledJobs > 0 {
		out.GuardHitRate = float64(out.GuardReasonHitJobs) / float64(out.GuardEnabledJobs)
	}
	if out.GuardReasonHitJobs > 0 {
		out.SelectionShiftRate = float64(out.SelectionShiftJobs) / float64(out.GuardReasonHitJobs)
		out.BlockedReasonRate = float64(out.BlockedReasonJobs) / float64(out.GuardReasonHitJobs)
	}
	return out, nil
}

func (h *Handler) loadVideoJobFeedbackNegativeGuardReasonStats(
	since time.Time,
	filter *videoImageFeedbackFilter,
	limit int,
) ([]AdminVideoJobFeedbackNegativeGuardReasonStat, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	type row struct {
		Reason      string   `gorm:"column:reason"`
		Jobs        int64    `gorm:"column:jobs"`
		BlockedJobs int64    `gorm:"column:blocked_jobs"`
		AvgWeight   *float64 `gorm:"column:avg_weight"`
	}
	filterSQL, filterArgs := buildVideoJobFilterClause(filter, "j")
	query := `
WITH base AS (
	SELECT
		j.id AS job_id,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->>'group'), ''), 'unknown')) AS group_name,
		LOWER(TRIM(reason_entry.key)) AS reason,
		CASE
			WHEN reason_entry.value ~ '^([0-9]+|-[0-9]+)(\.[0-9]+){0,1}$' THEN reason_entry.value::double precision
			ELSE NULL
		END AS reason_weight,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), '')) AS before_reason,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_after'->>'reason'), ''), '')) AS after_reason
	FROM public.video_image_jobs j
	CROSS JOIN LATERAL jsonb_each_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
			THEN j.metrics->'highlight_feedback_v1'->'reason_negative_guard'
			ELSE '{}'::jsonb
		END
	) AS reason_entry(key, value)
	WHERE j.status = ?
		AND j.finished_at >= ?
` + filterSQL + `
)
SELECT
	reason,
	COUNT(DISTINCT job_id)::bigint AS jobs,
	COUNT(DISTINCT job_id) FILTER (
		WHERE before_reason = reason
			AND after_reason <> ''
			AND after_reason <> before_reason
	)::bigint AS blocked_jobs,
	AVG(reason_weight) AS avg_weight
FROM base
WHERE group_name = 'treatment'
	AND reason <> ''
GROUP BY reason
ORDER BY blocked_jobs DESC, jobs DESC, reason ASC
LIMIT ?
`

	args := []interface{}{models.VideoJobStatusDone, since}
	args = append(args, filterArgs...)
	args = append(args, limit)

	var dbRows []row
	if err := h.db.Raw(query, args...).Scan(&dbRows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobFeedbackNegativeGuardReasonStat, 0, len(dbRows))
	for _, item := range dbRows {
		reason := strings.TrimSpace(item.Reason)
		if reason == "" {
			continue
		}
		avgWeight := 0.0
		if item.AvgWeight != nil {
			avgWeight = *item.AvgWeight
		}
		out = append(out, AdminVideoJobFeedbackNegativeGuardReasonStat{
			Reason:      reason,
			Jobs:        item.Jobs,
			BlockedJobs: item.BlockedJobs,
			AvgWeight:   avgWeight,
		})
	}
	return out, nil
}

func (h *Handler) loadVideoJobFeedbackNegativeGuardJobRows(
	since time.Time,
	filter *videoImageFeedbackFilter,
	limit int,
	blockedOnly bool,
) ([]AdminVideoJobFeedbackNegativeGuardJobRow, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}

	type row struct {
		JobID           uint64     `gorm:"column:job_id"`
		UserID          uint64     `gorm:"column:user_id"`
		Title           string     `gorm:"column:title"`
		GroupName       string     `gorm:"column:group_name"`
		GuardHit        bool       `gorm:"column:guard_hit"`
		BlockedReason   bool       `gorm:"column:blocked_reason"`
		BeforeReason    string     `gorm:"column:before_reason"`
		AfterReason     string     `gorm:"column:after_reason"`
		BeforeStartSec  *float64   `gorm:"column:before_start_sec"`
		BeforeEndSec    *float64   `gorm:"column:before_end_sec"`
		AfterStartSec   *float64   `gorm:"column:after_start_sec"`
		AfterEndSec     *float64   `gorm:"column:after_end_sec"`
		GuardReasonList string     `gorm:"column:guard_reason_list"`
		FinishedAt      *time.Time `gorm:"column:finished_at"`
	}

	filterSQL, filterArgs := buildVideoJobFilterClause(filter, "j")
	query := `
WITH base AS (
	SELECT
		j.id AS job_id,
		j.user_id AS user_id,
		COALESCE(j.title, '') AS title,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->>'group'), ''), 'unknown')) AS group_name,
		COALESCE(j.metrics->'highlight_feedback_v1'->'reason_negative_guard', '{}'::jsonb) AS reason_negative_guard,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), '')) AS before_reason,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_after'->>'reason'), ''), '')) AS after_reason,
		NULLIF(j.metrics->'highlight_feedback_v1'->'selected_before'->>'start_sec', '')::double precision AS before_start_sec,
		NULLIF(j.metrics->'highlight_feedback_v1'->'selected_before'->>'end_sec', '')::double precision AS before_end_sec,
		NULLIF(j.metrics->'highlight_feedback_v1'->'selected_after'->>'start_sec', '')::double precision AS after_start_sec,
		NULLIF(j.metrics->'highlight_feedback_v1'->'selected_after'->>'end_sec', '')::double precision AS after_end_sec,
		j.finished_at AS finished_at
	FROM public.video_image_jobs j
	WHERE j.status = ?
		AND j.finished_at >= ?
` + filterSQL + `
)
SELECT
	job_id,
	user_id,
	title,
	group_name,
	(jsonb_typeof(reason_negative_guard) = 'object' AND reason_negative_guard <> '{}'::jsonb) AS guard_hit,
	(
		(jsonb_typeof(reason_negative_guard) = 'object' AND reason_negative_guard <> '{}'::jsonb)
		AND before_reason <> ''
		AND (reason_negative_guard ? before_reason)
		AND after_reason <> ''
		AND after_reason <> before_reason
	) AS blocked_reason,
	before_reason,
	after_reason,
	before_start_sec,
	before_end_sec,
	after_start_sec,
	after_end_sec,
	COALESCE((
		SELECT string_agg(r.key || ':' || r.value, ' | ' ORDER BY r.key ASC)
		FROM jsonb_each_text(
			CASE
				WHEN jsonb_typeof(reason_negative_guard) = 'object'
				THEN reason_negative_guard
				ELSE '{}'::jsonb
			END
		) AS r(key, value)
	), '') AS guard_reason_list,
	finished_at
FROM base
WHERE group_name = 'treatment'
	AND jsonb_typeof(reason_negative_guard) = 'object'
	AND reason_negative_guard <> '{}'::jsonb
	AND (
		? = FALSE
		OR (
			before_reason <> ''
			AND after_reason <> ''
			AND after_reason <> before_reason
			AND (reason_negative_guard ? before_reason)
		)
	)
ORDER BY blocked_reason DESC, finished_at DESC NULLS LAST, job_id DESC
LIMIT ?
`

	args := []interface{}{models.VideoJobStatusDone, since}
	args = append(args, filterArgs...)
	args = append(args, blockedOnly)
	args = append(args, limit)

	var dbRows []row
	if err := h.db.Raw(query, args...).Scan(&dbRows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobFeedbackNegativeGuardJobRow, 0, len(dbRows))
	for _, item := range dbRows {
		out = append(out, AdminVideoJobFeedbackNegativeGuardJobRow{
			JobID:           item.JobID,
			UserID:          item.UserID,
			Title:           strings.TrimSpace(item.Title),
			Group:           strings.TrimSpace(item.GroupName),
			GuardHit:        item.GuardHit,
			BlockedReason:   item.BlockedReason,
			BeforeReason:    strings.TrimSpace(item.BeforeReason),
			AfterReason:     strings.TrimSpace(item.AfterReason),
			BeforeStartSec:  item.BeforeStartSec,
			BeforeEndSec:    item.BeforeEndSec,
			AfterStartSec:   item.AfterStartSec,
			AfterEndSec:     item.AfterEndSec,
			GuardReasonList: strings.TrimSpace(item.GuardReasonList),
			FinishedAt:      item.FinishedAt,
		})
	}
	return out, nil
}

func (h *Handler) loadVideoJobLiveCoverSceneStats(since time.Time, minSamples int64) ([]AdminVideoJobLiveCoverSceneStat, error) {
	if minSamples <= 0 {
		minSamples = 5
	}
	type row struct {
		SceneTag         string   `gorm:"column:scene_tag"`
		Samples          int64    `gorm:"column:samples"`
		AvgCoverScore    *float64 `gorm:"column:avg_cover_score"`
		AvgCoverPortrait *float64 `gorm:"column:avg_cover_portrait"`
		AvgCoverExposure *float64 `gorm:"column:avg_cover_exposure"`
		AvgCoverFace     *float64 `gorm:"column:avg_cover_face"`
	}

	var rows []row
	if err := h.db.Raw(`
WITH cover AS (
	SELECT
		a.job_id,
		COALESCE((a.metadata->>'cover_score')::double precision, 0) AS cover_score,
		COALESCE((a.metadata->>'cover_portrait')::double precision, 0) AS cover_portrait,
		COALESCE((a.metadata->>'cover_exposure')::double precision, 0) AS cover_exposure,
		COALESCE((a.metadata->>'cover_face')::double precision, 0) AS cover_face
	FROM archive.video_job_artifacts a
	JOIN archive.video_jobs j ON j.id = a.job_id
	WHERE a.type = 'live_cover'
		AND j.status = ?
		AND j.finished_at >= ?
),
scene AS (
	SELECT
		j.id AS job_id,
		LOWER(TRIM(tag.value)) AS scene_tag
	FROM archive.video_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'scene_tags_v1') = 'array'
			THEN j.metrics->'scene_tags_v1'
			ELSE '[]'::jsonb
		END
	) AS tag(value)
	WHERE j.status = ?
		AND j.finished_at >= ?
),
joined AS (
	SELECT
		COALESCE(NULLIF(scene.scene_tag, ''), 'uncategorized') AS scene_tag,
		cover.cover_score,
		cover.cover_portrait,
		cover.cover_exposure,
		cover.cover_face
	FROM cover
	LEFT JOIN scene ON scene.job_id = cover.job_id
)
SELECT
	scene_tag,
	COUNT(*)::bigint AS samples,
	AVG(cover_score) AS avg_cover_score,
	AVG(cover_portrait) AS avg_cover_portrait,
	AVG(cover_exposure) AS avg_cover_exposure,
	AVG(cover_face) AS avg_cover_face
FROM joined
GROUP BY scene_tag
ORDER BY samples DESC, scene_tag ASC
LIMIT 16
`, models.VideoJobStatusDone, since, models.VideoJobStatusDone, since).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobLiveCoverSceneStat, 0, len(rows))
	for _, item := range rows {
		tag := strings.TrimSpace(item.SceneTag)
		if tag == "" {
			tag = "uncategorized"
		}
		stat := AdminVideoJobLiveCoverSceneStat{
			SceneTag:  tag,
			Samples:   item.Samples,
			LowSample: item.Samples < minSamples,
		}
		if item.AvgCoverScore != nil {
			stat.AvgCoverScore = *item.AvgCoverScore
		}
		if item.AvgCoverPortrait != nil {
			stat.AvgCoverPortrait = *item.AvgCoverPortrait
		}
		if item.AvgCoverExposure != nil {
			stat.AvgCoverExposure = *item.AvgCoverExposure
		}
		if item.AvgCoverFace != nil {
			stat.AvgCoverFace = *item.AvgCoverFace
		}
		out = append(out, stat)
	}
	return out, nil
}

func (h *Handler) loadVideoJobGIFLoopTuneOverview(since time.Time) (AdminVideoJobGIFLoopTuneOverview, error) {
	type row struct {
		Samples          int64    `gorm:"column:samples"`
		Applied          int64    `gorm:"column:applied"`
		EffectiveApplied int64    `gorm:"column:effective_applied"`
		FallbackToBase   int64    `gorm:"column:fallback_to_base"`
		AvgScore         *float64 `gorm:"column:avg_score"`
		AvgLoopClosure   *float64 `gorm:"column:avg_loop_closure"`
		AvgMotionMean    *float64 `gorm:"column:avg_motion_mean"`
		AvgEffectiveSec  *float64 `gorm:"column:avg_effective_sec"`
	}

	var result row
	err := h.db.Raw(`
SELECT
	COUNT(*)::bigint AS samples,
	COUNT(*) FILTER (WHERE o.gif_loop_tune_applied = TRUE)::bigint AS applied,
	COUNT(*) FILTER (WHERE o.gif_loop_tune_effective_applied = TRUE)::bigint AS effective_applied,
	COUNT(*) FILTER (WHERE o.gif_loop_tune_fallback_to_base = TRUE)::bigint AS fallback_to_base,
	AVG(o.gif_loop_tune_score) FILTER (WHERE o.gif_loop_tune_applied = TRUE) AS avg_score,
	AVG(o.gif_loop_tune_loop_closure) FILTER (WHERE o.gif_loop_tune_applied = TRUE) AS avg_loop_closure,
	AVG(o.gif_loop_tune_motion_mean) FILTER (WHERE o.gif_loop_tune_applied = TRUE) AS avg_motion_mean,
	AVG(o.gif_loop_tune_effective_sec) FILTER (WHERE o.gif_loop_tune_applied = TRUE) AS avg_effective_sec
FROM public.video_image_outputs o
JOIN public.video_image_jobs j ON j.id = o.job_id
WHERE o.format = 'gif'
	AND o.file_role = 'main'
	AND j.status = ?
	AND j.finished_at >= ?
`, models.VideoJobStatusDone, since).Scan(&result).Error
	if err != nil {
		if isMissingPublicGIFLoopColumnsError(err) {
			return AdminVideoJobGIFLoopTuneOverview{}, nil
		}
		return AdminVideoJobGIFLoopTuneOverview{}, err
	}

	out := AdminVideoJobGIFLoopTuneOverview{
		Samples:          result.Samples,
		Applied:          result.Applied,
		EffectiveApplied: result.EffectiveApplied,
		FallbackToBase:   result.FallbackToBase,
	}
	if out.Samples > 0 {
		out.AppliedRate = float64(out.Applied) / float64(out.Samples)
		out.EffectiveAppliedRate = float64(out.EffectiveApplied) / float64(out.Samples)
		out.FallbackRate = float64(out.FallbackToBase) / float64(out.Samples)
	}
	if result.AvgScore != nil {
		out.AvgScore = *result.AvgScore
	}
	if result.AvgLoopClosure != nil {
		out.AvgLoopClosure = *result.AvgLoopClosure
	}
	if result.AvgMotionMean != nil {
		out.AvgMotionMean = *result.AvgMotionMean
	}
	if result.AvgEffectiveSec != nil {
		out.AvgEffectiveSec = *result.AvgEffectiveSec
	}
	return out, nil
}

func (h *Handler) loadVideoJobGIFEvaluationOverview(since time.Time) (AdminVideoJobGIFEvaluationOverview, error) {
	type row struct {
		Samples            int64    `gorm:"column:samples"`
		AvgEmotionScore    *float64 `gorm:"column:avg_emotion_score"`
		AvgClarityScore    *float64 `gorm:"column:avg_clarity_score"`
		AvgMotionScore     *float64 `gorm:"column:avg_motion_score"`
		AvgLoopScore       *float64 `gorm:"column:avg_loop_score"`
		AvgEfficiencyScore *float64 `gorm:"column:avg_efficiency_score"`
		AvgOverallScore    *float64 `gorm:"column:avg_overall_score"`
	}
	var result row
	if err := h.db.Raw(`
SELECT
	COUNT(*)::bigint AS samples,
	AVG(e.emotion_score) AS avg_emotion_score,
	AVG(e.clarity_score) AS avg_clarity_score,
	AVG(e.motion_score) AS avg_motion_score,
	AVG(e.loop_score) AS avg_loop_score,
	AVG(e.efficiency_score) AS avg_efficiency_score,
	AVG(e.overall_score) AS avg_overall_score
FROM archive.video_job_gif_evaluations e
JOIN public.video_image_jobs j ON j.id = e.job_id
WHERE j.requested_format = 'gif'
	AND e.created_at >= ?
`, since).Scan(&result).Error; err != nil {
		if isMissingGIFEvaluationTableError(err) {
			return AdminVideoJobGIFEvaluationOverview{}, nil
		}
		return AdminVideoJobGIFEvaluationOverview{}, err
	}
	out := AdminVideoJobGIFEvaluationOverview{
		Samples: result.Samples,
	}
	if result.AvgEmotionScore != nil {
		out.AvgEmotionScore = *result.AvgEmotionScore
	}
	if result.AvgClarityScore != nil {
		out.AvgClarityScore = *result.AvgClarityScore
	}
	if result.AvgMotionScore != nil {
		out.AvgMotionScore = *result.AvgMotionScore
	}
	if result.AvgLoopScore != nil {
		out.AvgLoopScore = *result.AvgLoopScore
	}
	if result.AvgEfficiencyScore != nil {
		out.AvgEfficiencyScore = *result.AvgEfficiencyScore
	}
	if result.AvgOverallScore != nil {
		out.AvgOverallScore = *result.AvgOverallScore
	}
	return out, nil
}

func (h *Handler) loadLatestVideoJobGIFBaselines(limit int) ([]AdminVideoJobGIFBaselineSnapshot, error) {
	if limit <= 0 {
		limit = 7
	}
	type row struct {
		BaselineDate       time.Time `gorm:"column:baseline_date"`
		WindowLabel        string    `gorm:"column:window_label"`
		Scope              string    `gorm:"column:scope"`
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
	}
	var rows []row
	if err := h.db.Raw(`
SELECT
	baseline_date,
	window_label,
	scope,
	sample_jobs,
	done_jobs,
	failed_jobs,
	done_rate,
	failed_rate,
	sample_outputs,
	avg_emotion_score,
	avg_clarity_score,
	avg_motion_score,
	avg_loop_score,
	avg_efficiency_score,
	avg_overall_score
FROM ops.video_job_gif_baselines
WHERE requested_format = 'gif'
ORDER BY baseline_date DESC, id DESC
LIMIT ?
`, limit).Scan(&rows).Error; err != nil {
		if isMissingGIFBaselineTableError(err) {
			return []AdminVideoJobGIFBaselineSnapshot{}, nil
		}
		return nil, err
	}
	out := make([]AdminVideoJobGIFBaselineSnapshot, 0, len(rows))
	for _, item := range rows {
		out = append(out, AdminVideoJobGIFBaselineSnapshot{
			BaselineDate:       item.BaselineDate.Format("2006-01-02"),
			WindowLabel:        strings.TrimSpace(item.WindowLabel),
			Scope:              strings.TrimSpace(item.Scope),
			SampleJobs:         item.SampleJobs,
			DoneJobs:           item.DoneJobs,
			FailedJobs:         item.FailedJobs,
			DoneRate:           item.DoneRate,
			FailedRate:         item.FailedRate,
			SampleOutputs:      item.SampleOutputs,
			AvgEmotionScore:    item.AvgEmotionScore,
			AvgClarityScore:    item.AvgClarityScore,
			AvgMotionScore:     item.AvgMotionScore,
			AvgLoopScore:       item.AvgLoopScore,
			AvgEfficiencyScore: item.AvgEfficiencyScore,
			AvgOverallScore:    item.AvgOverallScore,
		})
	}
	return out, nil
}

func (h *Handler) loadVideoJobGIFEvaluationSamples(
	since time.Time,
	limit int,
	ascending bool,
) ([]AdminVideoJobGIFEvaluationSample, error) {
	if limit <= 0 {
		limit = 5
	}
	type row struct {
		JobID           uint64         `gorm:"column:job_id"`
		OutputID        *uint64        `gorm:"column:output_id"`
		ObjectKey       string         `gorm:"column:object_key"`
		WindowStartMs   int            `gorm:"column:window_start_ms"`
		WindowEndMs     int            `gorm:"column:window_end_ms"`
		OverallScore    float64        `gorm:"column:overall_score"`
		EmotionScore    float64        `gorm:"column:emotion_score"`
		ClarityScore    float64        `gorm:"column:clarity_score"`
		MotionScore     float64        `gorm:"column:motion_score"`
		LoopScore       float64        `gorm:"column:loop_score"`
		EfficiencyScore float64        `gorm:"column:efficiency_score"`
		FeatureJSON     datatypes.JSON `gorm:"column:feature_json"`
		SizeBytes       int64          `gorm:"column:size_bytes"`
		Width           int            `gorm:"column:width"`
		Height          int            `gorm:"column:height"`
		DurationMs      int            `gorm:"column:duration_ms"`
		CreatedAt       time.Time      `gorm:"column:created_at"`
	}
	orderExpr := "e.overall_score DESC, e.id DESC"
	if ascending {
		orderExpr = "e.overall_score ASC, e.id ASC"
	}

	var rows []row
	if err := h.db.Raw(fmt.Sprintf(`
SELECT
	e.job_id,
	e.output_id,
	o.object_key,
	e.window_start_ms,
	e.window_end_ms,
	e.overall_score,
	e.emotion_score,
	e.clarity_score,
	e.motion_score,
	e.loop_score,
	e.efficiency_score,
	e.feature_json,
	o.size_bytes,
	o.width,
	o.height,
	o.duration_ms,
	e.created_at
FROM archive.video_job_gif_evaluations e
JOIN public.video_image_outputs o ON o.id = e.output_id
JOIN public.video_image_jobs j ON j.id = e.job_id
WHERE j.requested_format = 'gif'
	AND e.created_at >= ?
ORDER BY %s
LIMIT ?
`, orderExpr), since, limit).Scan(&rows).Error; err != nil {
		if isMissingGIFEvaluationTableError(err) {
			return []AdminVideoJobGIFEvaluationSample{}, nil
		}
		return nil, err
	}

	out := make([]AdminVideoJobGIFEvaluationSample, 0, len(rows))
	for _, item := range rows {
		outputID := uint64(0)
		if item.OutputID != nil {
			outputID = *item.OutputID
		}
		feature := parseJSONMap(item.FeatureJSON)
		reason := ""
		if raw, ok := feature["candidate_feature"].(map[string]interface{}); ok {
			reason = strings.TrimSpace(strings.ToLower(fmt.Sprint(raw["reason"])))
		}
		if reason == "" {
			reason = strings.TrimSpace(strings.ToLower(fmt.Sprint(feature["reason"])))
		}
		out = append(out, AdminVideoJobGIFEvaluationSample{
			JobID:           item.JobID,
			OutputID:        outputID,
			PreviewURL:      resolvePreviewURL(strings.TrimSpace(item.ObjectKey), h.qiniu),
			ObjectKey:       strings.TrimSpace(item.ObjectKey),
			WindowStartMs:   item.WindowStartMs,
			WindowEndMs:     item.WindowEndMs,
			OverallScore:    item.OverallScore,
			EmotionScore:    item.EmotionScore,
			ClarityScore:    item.ClarityScore,
			MotionScore:     item.MotionScore,
			LoopScore:       item.LoopScore,
			EfficiencyScore: item.EfficiencyScore,
			CandidateReason: reason,
			SizeBytes:       item.SizeBytes,
			Width:           item.Width,
			Height:          item.Height,
			DurationMs:      item.DurationMs,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func (h *Handler) loadVideoJobGIFManualScoreOverview(since time.Time) (AdminVideoJobGIFManualScoreOverview, error) {
	type row struct {
		Samples             int64    `gorm:"column:samples"`
		WithOutputID        int64    `gorm:"column:with_output_id"`
		MatchedEvaluations  int64    `gorm:"column:matched_evaluations"`
		TopPickRate         *float64 `gorm:"column:top_pick_rate"`
		PassRate            *float64 `gorm:"column:pass_rate"`
		AvgManualEmotion    *float64 `gorm:"column:avg_manual_emotion"`
		AvgManualClarity    *float64 `gorm:"column:avg_manual_clarity"`
		AvgManualMotion     *float64 `gorm:"column:avg_manual_motion"`
		AvgManualLoop       *float64 `gorm:"column:avg_manual_loop"`
		AvgManualEfficiency *float64 `gorm:"column:avg_manual_efficiency"`
		AvgManualOverall    *float64 `gorm:"column:avg_manual_overall"`
		AvgAutoEmotion      *float64 `gorm:"column:avg_auto_emotion"`
		AvgAutoClarity      *float64 `gorm:"column:avg_auto_clarity"`
		AvgAutoMotion       *float64 `gorm:"column:avg_auto_motion"`
		AvgAutoLoop         *float64 `gorm:"column:avg_auto_loop"`
		AvgAutoEfficiency   *float64 `gorm:"column:avg_auto_efficiency"`
		AvgAutoOverall      *float64 `gorm:"column:avg_auto_overall"`
		MAEEmotion          *float64 `gorm:"column:mae_emotion"`
		MAEClarity          *float64 `gorm:"column:mae_clarity"`
		MAEMotion           *float64 `gorm:"column:mae_motion"`
		MAELoop             *float64 `gorm:"column:mae_loop"`
		MAEEfficiency       *float64 `gorm:"column:mae_efficiency"`
		MAEOverall          *float64 `gorm:"column:mae_overall"`
		AvgOverallDelta     *float64 `gorm:"column:avg_overall_delta"`
	}
	var result row
	if err := h.db.Raw(`
SELECT
	COUNT(*)::bigint AS samples,
	COUNT(*) FILTER (WHERE m.output_id IS NOT NULL)::bigint AS with_output_id,
	COUNT(*) FILTER (WHERE e.id IS NOT NULL)::bigint AS matched_evaluations,
	AVG(CASE WHEN m.is_top_pick THEN 1.0 ELSE 0.0 END) AS top_pick_rate,
	AVG(CASE WHEN m.is_pass THEN 1.0 ELSE 0.0 END) AS pass_rate,
	AVG(m.emotion_score) AS avg_manual_emotion,
	AVG(m.clarity_score) AS avg_manual_clarity,
	AVG(m.motion_score) AS avg_manual_motion,
	AVG(m.loop_score) AS avg_manual_loop,
	AVG(m.efficiency_score) AS avg_manual_efficiency,
	AVG(m.overall_score) AS avg_manual_overall,
	AVG(e.emotion_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_emotion,
	AVG(e.clarity_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_clarity,
	AVG(e.motion_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_motion,
	AVG(e.loop_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_loop,
	AVG(e.efficiency_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_efficiency,
	AVG(e.overall_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_overall,
	AVG(ABS(m.emotion_score - e.emotion_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_emotion,
	AVG(ABS(m.clarity_score - e.clarity_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_clarity,
	AVG(ABS(m.motion_score - e.motion_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_motion,
	AVG(ABS(m.loop_score - e.loop_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_loop,
	AVG(ABS(m.efficiency_score - e.efficiency_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_efficiency,
	AVG(ABS(m.overall_score - e.overall_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_overall,
	AVG(m.overall_score - e.overall_score) FILTER (WHERE e.id IS NOT NULL) AS avg_overall_delta
FROM ops.video_job_gif_manual_scores m
LEFT JOIN archive.video_job_gif_evaluations e ON e.output_id = m.output_id
WHERE m.reviewed_at >= ?
`, since).Scan(&result).Error; err != nil {
		if isMissingGIFManualScoreTableError(err) {
			return AdminVideoJobGIFManualScoreOverview{}, nil
		}
		return AdminVideoJobGIFManualScoreOverview{}, err
	}

	out := AdminVideoJobGIFManualScoreOverview{
		Samples:            result.Samples,
		WithOutputID:       result.WithOutputID,
		MatchedEvaluations: result.MatchedEvaluations,
	}
	if out.Samples > 0 {
		out.MatchedRate = float64(out.MatchedEvaluations) / float64(out.Samples)
	}
	if result.TopPickRate != nil {
		out.TopPickRate = *result.TopPickRate
	}
	if result.PassRate != nil {
		out.PassRate = *result.PassRate
	}
	if result.AvgManualEmotion != nil {
		out.AvgManualEmotion = *result.AvgManualEmotion
	}
	if result.AvgManualClarity != nil {
		out.AvgManualClarity = *result.AvgManualClarity
	}
	if result.AvgManualMotion != nil {
		out.AvgManualMotion = *result.AvgManualMotion
	}
	if result.AvgManualLoop != nil {
		out.AvgManualLoop = *result.AvgManualLoop
	}
	if result.AvgManualEfficiency != nil {
		out.AvgManualEfficiency = *result.AvgManualEfficiency
	}
	if result.AvgManualOverall != nil {
		out.AvgManualOverall = *result.AvgManualOverall
	}
	if result.AvgAutoEmotion != nil {
		out.AvgAutoEmotion = *result.AvgAutoEmotion
	}
	if result.AvgAutoClarity != nil {
		out.AvgAutoClarity = *result.AvgAutoClarity
	}
	if result.AvgAutoMotion != nil {
		out.AvgAutoMotion = *result.AvgAutoMotion
	}
	if result.AvgAutoLoop != nil {
		out.AvgAutoLoop = *result.AvgAutoLoop
	}
	if result.AvgAutoEfficiency != nil {
		out.AvgAutoEfficiency = *result.AvgAutoEfficiency
	}
	if result.AvgAutoOverall != nil {
		out.AvgAutoOverall = *result.AvgAutoOverall
	}
	if result.MAEEmotion != nil {
		out.MAEEmotion = *result.MAEEmotion
	}
	if result.MAEClarity != nil {
		out.MAEClarity = *result.MAEClarity
	}
	if result.MAEMotion != nil {
		out.MAEMotion = *result.MAEMotion
	}
	if result.MAELoop != nil {
		out.MAELoop = *result.MAELoop
	}
	if result.MAEEfficiency != nil {
		out.MAEEfficiency = *result.MAEEfficiency
	}
	if result.MAEOverall != nil {
		out.MAEOverall = *result.MAEOverall
	}
	if result.AvgOverallDelta != nil {
		out.AvgOverallDelta = *result.AvgOverallDelta
	}
	return out, nil
}

func (h *Handler) loadVideoJobGIFManualScoreDiffSamples(
	since time.Time,
	limit int,
) ([]AdminVideoJobGIFManualScoreDiffSample, error) {
	if limit <= 0 {
		limit = 8
	}
	type row struct {
		SampleID            string    `gorm:"column:sample_id"`
		BaselineVersion     string    `gorm:"column:baseline_version"`
		ReviewRound         string    `gorm:"column:review_round"`
		Reviewer            string    `gorm:"column:reviewer"`
		JobID               uint64    `gorm:"column:job_id"`
		OutputID            uint64    `gorm:"column:output_id"`
		ObjectKey           string    `gorm:"column:object_key"`
		ManualOverallScore  float64   `gorm:"column:manual_overall_score"`
		AutoOverallScore    float64   `gorm:"column:auto_overall_score"`
		OverallScoreDelta   float64   `gorm:"column:overall_score_delta"`
		AbsOverallScoreDiff float64   `gorm:"column:abs_overall_score_diff"`
		ManualLoopScore     float64   `gorm:"column:manual_loop_score"`
		AutoLoopScore       float64   `gorm:"column:auto_loop_score"`
		LoopScoreDelta      float64   `gorm:"column:loop_score_delta"`
		ManualClarityScore  float64   `gorm:"column:manual_clarity_score"`
		AutoClarityScore    float64   `gorm:"column:auto_clarity_score"`
		ClarityScoreDelta   float64   `gorm:"column:clarity_score_delta"`
		IsTopPick           bool      `gorm:"column:is_top_pick"`
		IsPass              bool      `gorm:"column:is_pass"`
		RejectReason        string    `gorm:"column:reject_reason"`
		ReviewedAt          time.Time `gorm:"column:reviewed_at"`
	}
	rows := make([]row, 0, limit)
	if err := h.db.Raw(`
SELECT
	m.sample_id,
	m.baseline_version,
	m.review_round,
	m.reviewer,
	COALESCE(m.job_id, e.job_id, 0)::bigint AS job_id,
	COALESCE(m.output_id, 0)::bigint AS output_id,
	COALESCE(o.object_key, '') AS object_key,
	m.overall_score AS manual_overall_score,
	e.overall_score AS auto_overall_score,
	(m.overall_score - e.overall_score) AS overall_score_delta,
	ABS(m.overall_score - e.overall_score) AS abs_overall_score_diff,
	m.loop_score AS manual_loop_score,
	e.loop_score AS auto_loop_score,
	(m.loop_score - e.loop_score) AS loop_score_delta,
	m.clarity_score AS manual_clarity_score,
	e.clarity_score AS auto_clarity_score,
	(m.clarity_score - e.clarity_score) AS clarity_score_delta,
	m.is_top_pick,
	m.is_pass,
	m.reject_reason,
	m.reviewed_at
FROM ops.video_job_gif_manual_scores m
JOIN archive.video_job_gif_evaluations e ON e.output_id = m.output_id
LEFT JOIN public.video_image_outputs o ON o.id = m.output_id
WHERE m.reviewed_at >= ?
ORDER BY ABS(m.overall_score - e.overall_score) DESC, m.reviewed_at DESC
LIMIT ?
`, since, limit).Scan(&rows).Error; err != nil {
		if isMissingGIFManualScoreTableError(err) || isMissingGIFEvaluationTableError(err) {
			return []AdminVideoJobGIFManualScoreDiffSample{}, nil
		}
		return nil, err
	}

	out := make([]AdminVideoJobGIFManualScoreDiffSample, 0, len(rows))
	for _, item := range rows {
		out = append(out, AdminVideoJobGIFManualScoreDiffSample{
			SampleID:            strings.TrimSpace(item.SampleID),
			BaselineVersion:     strings.TrimSpace(item.BaselineVersion),
			ReviewRound:         strings.TrimSpace(item.ReviewRound),
			Reviewer:            strings.TrimSpace(item.Reviewer),
			JobID:               item.JobID,
			OutputID:            item.OutputID,
			PreviewURL:          resolvePreviewURL(strings.TrimSpace(item.ObjectKey), h.qiniu),
			ObjectKey:           strings.TrimSpace(item.ObjectKey),
			ManualOverallScore:  item.ManualOverallScore,
			AutoOverallScore:    item.AutoOverallScore,
			OverallScoreDelta:   item.OverallScoreDelta,
			AbsOverallScoreDiff: item.AbsOverallScoreDiff,
			ManualLoopScore:     item.ManualLoopScore,
			AutoLoopScore:       item.AutoLoopScore,
			LoopScoreDelta:      item.LoopScoreDelta,
			ManualClarityScore:  item.ManualClarityScore,
			AutoClarityScore:    item.AutoClarityScore,
			ClarityScoreDelta:   item.ClarityScoreDelta,
			IsTopPick:           item.IsTopPick,
			IsPass:              item.IsPass,
			RejectReason:        strings.TrimSpace(strings.ToLower(item.RejectReason)),
			ReviewedAt:          item.ReviewedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func isMissingGIFEvaluationTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "video_job_gif_evaluations") && strings.Contains(msg, "does not exist")
}

func isMissingGIFBaselineTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "video_job_gif_baselines") && strings.Contains(msg, "does not exist")
}

func isMissingGIFManualScoreTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "video_job_gif_manual_scores") && strings.Contains(msg, "does not exist")
}

func isMissingPublicGIFLoopColumnsError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "public.video_image_outputs") && strings.Contains(msg, "does not exist") {
		return true
	}
	if strings.Contains(msg, "gif_loop_tune_") && strings.Contains(msg, "does not exist") {
		return true
	}
	return false
}

func (h *Handler) loadVideoJobFeedbackGroupStats(since time.Time) ([]AdminVideoJobFeedbackGroupStat, error) {
	return h.loadVideoJobFeedbackGroupStatsRange(since, time.Now())
}

func (h *Handler) loadVideoJobFeedbackGroupStatsRange(start, end time.Time) ([]AdminVideoJobFeedbackGroupStat, error) {
	type row struct {
		Group              string   `gorm:"column:group_name"`
		Jobs               int64    `gorm:"column:jobs"`
		EngagedJobs        int64    `gorm:"column:engaged_jobs"`
		FeedbackSignals    int64    `gorm:"column:feedback_signals"`
		AvgEngagementScore *float64 `gorm:"column:avg_engagement_score"`
		AppliedJobs        int64    `gorm:"column:applied_jobs"`
	}
	if !end.IsZero() && end.Before(start) {
		end = start
	}

	var rows []row
	if err := h.db.Raw(`
WITH base AS (
	SELECT
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->>'group'), ''), 'unknown')) AS group_name,
		COALESCE((j.metrics->'feedback_v1'->>'total_signals')::bigint, 0) AS total_signals,
		COALESCE((j.metrics->'feedback_v1'->>'engagement_score')::double precision, 0) AS engagement_score,
		CASE
			WHEN LOWER(COALESCE(j.metrics->'highlight_feedback_v1'->>'applied', 'false')) IN ('true', '1', 't', 'yes', 'y') THEN 1
			ELSE 0
		END AS applied_flag
	FROM archive.video_jobs j
	WHERE j.status = ?
		AND j.finished_at >= ?
		AND j.finished_at < ?
)
SELECT
	group_name,
	COUNT(*)::bigint AS jobs,
	COUNT(*) FILTER (WHERE total_signals > 0)::bigint AS engaged_jobs,
	COALESCE(SUM(total_signals), 0)::bigint AS feedback_signals,
	AVG(engagement_score) FILTER (WHERE total_signals > 0) AS avg_engagement_score,
	COALESCE(SUM(applied_flag), 0)::bigint AS applied_jobs
FROM base
GROUP BY group_name
ORDER BY jobs DESC, group_name ASC
LIMIT 8
`, models.VideoJobStatusDone, start, end).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobFeedbackGroupStat, 0, len(rows))
	for _, item := range rows {
		group := strings.TrimSpace(strings.ToLower(item.Group))
		if group == "" {
			group = "unknown"
		}
		stat := AdminVideoJobFeedbackGroupStat{
			Group:           group,
			Jobs:            item.Jobs,
			EngagedJobs:     item.EngagedJobs,
			FeedbackSignals: item.FeedbackSignals,
			AppliedJobs:     item.AppliedJobs,
		}
		if item.AvgEngagementScore != nil {
			stat.AvgEngagementScore = *item.AvgEngagementScore
		}
		out = append(out, stat)
	}
	return out, nil
}

func (h *Handler) loadVideoJobFeedbackGroupStatsHistory(windowDuration time.Duration, windows int, endAt time.Time) ([][]AdminVideoJobFeedbackGroupStat, error) {
	if windowDuration <= 0 {
		windowDuration = 24 * time.Hour
	}
	if windows <= 0 {
		windows = 1
	}
	if windows > 12 {
		windows = 12
	}
	if endAt.IsZero() {
		endAt = time.Now()
	}

	out := make([][]AdminVideoJobFeedbackGroupStat, 0, windows)
	for idx := 0; idx < windows; idx++ {
		windowEnd := endAt.Add(-time.Duration(idx) * windowDuration)
		windowStart := windowEnd.Add(-windowDuration)
		stats, err := h.loadVideoJobFeedbackGroupStatsRange(windowStart, windowEnd)
		if err != nil {
			return nil, err
		}
		out = append(out, stats)
	}
	return out, nil
}

func (h *Handler) loadVideoJobFeedbackGroupFormatStats(since time.Time) ([]AdminVideoJobFeedbackGroupFormatStat, error) {
	type row struct {
		Format             string   `gorm:"column:format"`
		Group              string   `gorm:"column:group_name"`
		Jobs               int64    `gorm:"column:jobs"`
		EngagedJobs        int64    `gorm:"column:engaged_jobs"`
		FeedbackSignals    int64    `gorm:"column:feedback_signals"`
		AvgEngagementScore *float64 `gorm:"column:avg_engagement_score"`
		AppliedJobs        int64    `gorm:"column:applied_jobs"`
	}

	var rows []row
	if err := h.db.Raw(`
WITH base AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN LOWER(fmt.value) = 'jpeg' THEN 'jpg'
			ELSE LOWER(fmt.value)
		END AS format,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->>'group'), ''), 'unknown')) AS group_name,
		COALESCE((j.metrics->'feedback_v1'->>'total_signals')::bigint, 0) AS total_signals,
		COALESCE((j.metrics->'feedback_v1'->>'engagement_score')::double precision, 0) AS engagement_score,
		CASE
			WHEN LOWER(COALESCE(j.metrics->'highlight_feedback_v1'->>'applied', 'false')) IN ('true', '1', 't', 'yes', 'y') THEN 1
			ELSE 0
		END AS applied_flag
	FROM archive.video_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'output_formats') = 'array' THEN j.metrics->'output_formats'
			ELSE '[]'::jsonb
		END
	) AS fmt(value)
	WHERE j.status = ?
		AND j.finished_at >= ?
		AND TRIM(fmt.value) <> ''
)
SELECT
	format,
	group_name,
	COUNT(*)::bigint AS jobs,
	COUNT(*) FILTER (WHERE total_signals > 0)::bigint AS engaged_jobs,
	COALESCE(SUM(total_signals), 0)::bigint AS feedback_signals,
	AVG(engagement_score) FILTER (WHERE total_signals > 0) AS avg_engagement_score,
	COALESCE(SUM(applied_flag), 0)::bigint AS applied_jobs
FROM base
GROUP BY format, group_name
ORDER BY format ASC, group_name ASC
`, models.VideoJobStatusDone, since).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobFeedbackGroupFormatStat, 0, len(rows))
	for _, item := range rows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		group := strings.TrimSpace(strings.ToLower(item.Group))
		if group == "" {
			group = "unknown"
		}
		stat := AdminVideoJobFeedbackGroupFormatStat{
			Format:          format,
			Group:           group,
			Jobs:            item.Jobs,
			EngagedJobs:     item.EngagedJobs,
			FeedbackSignals: item.FeedbackSignals,
			AppliedJobs:     item.AppliedJobs,
		}
		if item.AvgEngagementScore != nil {
			stat.AvgEngagementScore = *item.AvgEngagementScore
		}
		out = append(out, stat)
	}
	return out, nil
}

func (h *Handler) loadVideoJobFeedbackRolloutAudits(limit int) ([]AdminVideoJobFeedbackRolloutAudit, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	var rows []models.VideoQualityRolloutAudit
	if err := h.db.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AdminVideoJobFeedbackRolloutAudit, 0, len(rows))
	for _, row := range rows {
		out = append(out, AdminVideoJobFeedbackRolloutAudit{
			ID:                   row.ID,
			AdminID:              row.AdminID,
			FromRolloutPercent:   row.FromRolloutPercent,
			ToRolloutPercent:     row.ToRolloutPercent,
			Window:               strings.TrimSpace(row.Window),
			ConfirmWindows:       row.ConfirmWindows,
			RecommendationState:  strings.TrimSpace(row.RecommendationState),
			RecommendationReason: strings.TrimSpace(row.RecommendationReason),
			CreatedAt:            row.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func defaultLiveCoverSceneGuardConfig() liveCoverSceneGuardConfig {
	defaults := videojobs.DefaultQualitySettings()
	return liveCoverSceneGuardConfig{
		MinSamples:   int64(defaults.LiveCoverSceneMinSamples),
		MinTotal:     int64(defaults.LiveCoverGuardMinTotal),
		ScoreFloor:   defaults.LiveCoverGuardScoreFloor,
		MaxRiskScene: defaultLiveCoverSceneGuardMaxRiskScenes,
	}
}

func buildLiveCoverSceneGuardConfigFromQualitySettings(settings videojobs.QualitySettings) liveCoverSceneGuardConfig {
	config := liveCoverSceneGuardConfig{
		MinSamples:   int64(settings.LiveCoverSceneMinSamples),
		MinTotal:     int64(settings.LiveCoverGuardMinTotal),
		ScoreFloor:   settings.LiveCoverGuardScoreFloor,
		MaxRiskScene: defaultLiveCoverSceneGuardMaxRiskScenes,
	}
	return normalizeLiveCoverSceneGuardConfig(config)
}

func normalizeLiveCoverSceneGuardConfig(config liveCoverSceneGuardConfig) liveCoverSceneGuardConfig {
	fallback := defaultLiveCoverSceneGuardConfig()
	if config.MinSamples <= 0 {
		config.MinSamples = fallback.MinSamples
	}
	if config.MinTotal <= 0 {
		config.MinTotal = fallback.MinTotal
	}
	if config.ScoreFloor <= 0 {
		config.ScoreFloor = fallback.ScoreFloor
	}
	if config.MaxRiskScene <= 0 {
		config.MaxRiskScene = fallback.MaxRiskScene
	}
	return config
}

func (h *Handler) loadCurrentFeedbackRolloutConfig() (enabled bool, rollout int, guardConfig liveCoverSceneGuardConfig, err error) {
	defaults := videojobs.DefaultQualitySettings()
	enabled = defaults.HighlightFeedbackEnabled
	rollout = defaults.HighlightFeedbackRollout
	guardConfig = defaultLiveCoverSceneGuardConfig()

	if h == nil || h.db == nil {
		return
	}
	setting, loadErr := h.loadVideoQualitySetting()
	if loadErr != nil {
		err = loadErr
		return
	}
	qualitySettings := qualitySettingsFromModel(setting)
	enabled = qualitySettings.HighlightFeedbackEnabled
	rollout = qualitySettings.HighlightFeedbackRollout
	if rollout < 0 {
		rollout = 0
	}
	if rollout > 100 {
		rollout = 100
	}
	guardConfig = buildLiveCoverSceneGuardConfigFromQualitySettings(qualitySettings)
	return
}

func buildFeedbackRolloutRecommendation(enabled bool, currentRollout int, groups []AdminVideoJobFeedbackGroupStat) AdminVideoJobFeedbackRolloutRecommendation {
	rec := AdminVideoJobFeedbackRolloutRecommendation{
		State:                   "hold",
		Reason:                  "当前策略维持不变",
		CurrentRolloutPercent:   currentRollout,
		SuggestedRolloutPercent: currentRollout,
		ConsecutiveRequired:     1,
		ConsecutiveMatched:      1,
		ConsecutivePassed:       true,
	}
	if !enabled {
		rec.State = "disabled"
		rec.Reason = "反馈重排未启用"
		return rec
	}

	byGroup := map[string]AdminVideoJobFeedbackGroupStat{}
	for _, item := range groups {
		key := strings.ToLower(strings.TrimSpace(item.Group))
		if key == "" {
			continue
		}
		byGroup[key] = item
	}
	treatment, okT := byGroup["treatment"]
	control, okC := byGroup["control"]
	if !okT || !okC {
		rec.State = "insufficient_data"
		rec.Reason = "缺少 control/treatment 分组样本"
		return rec
	}

	rec.TreatmentJobs = treatment.Jobs
	rec.ControlJobs = control.Jobs
	if treatment.Jobs > 0 {
		rec.TreatmentSignalsPerJob = float64(treatment.FeedbackSignals) / float64(treatment.Jobs)
	}
	if control.Jobs > 0 {
		rec.ControlSignalsPerJob = float64(control.FeedbackSignals) / float64(control.Jobs)
	}
	rec.TreatmentAvgScore = treatment.AvgEngagementScore
	rec.ControlAvgScore = control.AvgEngagementScore
	if rec.ControlSignalsPerJob > 0 {
		rec.SignalsUplift = (rec.TreatmentSignalsPerJob - rec.ControlSignalsPerJob) / rec.ControlSignalsPerJob
	}
	if rec.ControlAvgScore > 0 {
		rec.ScoreUplift = (rec.TreatmentAvgScore - rec.ControlAvgScore) / rec.ControlAvgScore
	}

	if treatment.Jobs < 30 || control.Jobs < 30 {
		rec.State = "insufficient_data"
		rec.Reason = "A/B 样本量不足（建议每组 >=30）"
		return rec
	}

	if rec.SignalsUplift >= 0.08 && rec.ScoreUplift >= 0 {
		rec.State = "scale_up"
		switch {
		case currentRollout < 50:
			rec.SuggestedRolloutPercent = 50
		case currentRollout < 100:
			rec.SuggestedRolloutPercent = 100
		default:
			rec.SuggestedRolloutPercent = 100
		}
		rec.Reason = "treatment 指标持续优于 control，建议放量"
		return rec
	}

	if rec.SignalsUplift <= -0.05 || rec.ScoreUplift <= -0.05 {
		rec.State = "scale_down"
		switch {
		case currentRollout > 50:
			rec.SuggestedRolloutPercent = 30
		case currentRollout > 30:
			rec.SuggestedRolloutPercent = 10
		default:
			rec.SuggestedRolloutPercent = currentRollout
		}
		rec.Reason = "treatment 指标劣于 control，建议降档观察"
		return rec
	}

	rec.State = "hold"
	rec.Reason = "A/B 差异不显著，建议维持当前流量"
	return rec
}

func buildFeedbackRolloutRecommendationWithHistory(
	enabled bool,
	currentRollout int,
	history [][]AdminVideoJobFeedbackGroupStat,
	requiredConsecutive int,
	liveCoverSceneStats []AdminVideoJobLiveCoverSceneStat,
	guardConfig liveCoverSceneGuardConfig,
) AdminVideoJobFeedbackRolloutRecommendation {
	if requiredConsecutive <= 0 {
		requiredConsecutive = 3
	}
	if len(history) == 0 {
		history = [][]AdminVideoJobFeedbackGroupStat{{}}
	}

	current := buildFeedbackRolloutRecommendation(enabled, currentRollout, history[0])
	current.ConsecutiveRequired = requiredConsecutive
	current.ConsecutiveMatched = 1
	current.ConsecutivePassed = true

	recentStates := make([]string, 0, len(history))
	recentStates = append(recentStates, current.State)
	for idx := 1; idx < len(history); idx++ {
		windowRec := buildFeedbackRolloutRecommendation(enabled, currentRollout, history[idx])
		recentStates = append(recentStates, windowRec.State)
	}
	current.RecentStates = recentStates

	shouldConfirm := current.State == "scale_up" || current.State == "scale_down"
	if !shouldConfirm {
		return applyLiveCoverSceneGuard(current, liveCoverSceneStats, guardConfig)
	}

	matched := 1
	for idx := 1; idx < len(history); idx++ {
		windowRec := buildFeedbackRolloutRecommendation(enabled, currentRollout, history[idx])
		if windowRec.State == current.State {
			matched++
			continue
		}
		break
	}

	current.ConsecutiveMatched = matched
	current.ConsecutivePassed = matched >= requiredConsecutive
	if current.ConsecutivePassed {
		current.Reason = fmt.Sprintf("%s（连续窗口确认 %d/%d）", current.Reason, matched, requiredConsecutive)
		return applyLiveCoverSceneGuard(current, liveCoverSceneStats, guardConfig)
	}

	current.State = "pending_confirmation"
	current.Reason = fmt.Sprintf("连续窗口确认未达标（%d/%d），暂不调整 rollout", matched, requiredConsecutive)
	current.SuggestedRolloutPercent = currentRollout
	return applyLiveCoverSceneGuard(current, liveCoverSceneStats, guardConfig)
}

func applyLiveCoverSceneGuard(
	rec AdminVideoJobFeedbackRolloutRecommendation,
	sceneStats []AdminVideoJobLiveCoverSceneStat,
	guardConfig liveCoverSceneGuardConfig,
) AdminVideoJobFeedbackRolloutRecommendation {
	guardConfig = normalizeLiveCoverSceneGuardConfig(guardConfig)
	rec.LiveGuardMinSamples = int(guardConfig.MinSamples)
	rec.LiveGuardScoreFloor = guardConfig.ScoreFloor
	shouldGuard := rec.State == "scale_up" || rec.State == "pending_confirmation"
	if !shouldGuard {
		return rec
	}

	triggered, eligibleTotal, riskScenes := evaluateLiveCoverSceneGuard(sceneStats, guardConfig)
	rec.LiveGuardEligibleTotal = eligibleTotal
	if !triggered {
		return rec
	}

	rec.LiveGuardTriggered = true
	rec.LiveGuardRiskScenes = riskScenes
	rec.State = "hold"
	rec.SuggestedRolloutPercent = rec.CurrentRolloutPercent
	rec.ConsecutivePassed = false
	rec.Reason = fmt.Sprintf(
		"Live 场景质量护栏触发（低于 %.2f：%s），暂不放量",
		guardConfig.ScoreFloor,
		strings.Join(riskScenes, ", "),
	)
	return rec
}

func evaluateLiveCoverSceneGuard(
	sceneStats []AdminVideoJobLiveCoverSceneStat,
	guardConfig liveCoverSceneGuardConfig,
) (triggered bool, eligibleTotal int64, riskScenes []string) {
	guardConfig = normalizeLiveCoverSceneGuardConfig(guardConfig)
	for _, item := range sceneStats {
		if item.Samples < guardConfig.MinSamples {
			continue
		}
		eligibleTotal += item.Samples
		if item.AvgCoverScore < guardConfig.ScoreFloor {
			riskScenes = append(riskScenes, item.SceneTag)
		}
	}
	if eligibleTotal < guardConfig.MinTotal || len(riskScenes) == 0 {
		return false, eligibleTotal, nil
	}
	sort.Strings(riskScenes)
	if len(riskScenes) > guardConfig.MaxRiskScene {
		riskScenes = riskScenes[:guardConfig.MaxRiskScene]
	}
	return true, eligibleTotal, riskScenes
}

func normalizeVideoJobFormat(raw string) string {
	format := strings.ToLower(strings.TrimSpace(raw))
	switch format {
	case "jpeg":
		return "jpg"
	default:
		return format
	}
}

func parseVideoImageFeedbackFilter(c *gin.Context) (*videoImageFeedbackFilter, error) {
	filter := videoImageFeedbackFilter{}

	if userIDRaw := strings.TrimSpace(c.Query("user_id")); userIDRaw != "" {
		userID, err := strconv.ParseUint(userIDRaw, 10, 64)
		if err != nil || userID == 0 {
			return nil, errors.New("invalid user_id")
		}
		filter.UserID = userID
	}

	if format := normalizeVideoJobFormat(c.Query("format")); format != "" && format != "all" {
		filter.Format = format
	}
	if guardReason := strings.ToLower(strings.TrimSpace(c.Query("guard_reason"))); guardReason != "" && guardReason != "all" {
		filter.GuardReason = guardReason
	}

	if filter.UserID == 0 && filter.Format == "" && filter.GuardReason == "" {
		return nil, nil
	}
	return &filter, nil
}

func buildVideoImageFeedbackFilterSQL(filter *videoImageFeedbackFilter) (string, []interface{}) {
	if filter == nil {
		return "", nil
	}

	clauses := make([]string, 0, 4)
	args := make([]interface{}, 0, 4)
	if filter.UserID > 0 {
		clauses = append(clauses, "j.user_id = ?")
		args = append(args, filter.UserID)
	}
	if filter.Format != "" {
		clauses = append(clauses, buildVideoJobFormatFilterPredicate("j"))
		args = append(args, filter.Format, filter.Format)
	}
	if filter.GuardReason != "" {
		clauses = append(clauses, `
EXISTS (
	SELECT 1
	FROM jsonb_each_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
			THEN j.metrics->'highlight_feedback_v1'->'reason_negative_guard'
			ELSE '{}'::jsonb
		END
	) AS guard_reason(key, value)
	WHERE LOWER(TRIM(guard_reason.key)) = ?
)`)
		args = append(args, filter.GuardReason)
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return " AND EXISTS (SELECT 1 FROM public.video_image_jobs j WHERE j.id = f.job_id AND " + strings.Join(clauses, " AND ") + ")", args
}

func buildVideoJobFormatFilterPredicate(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "j"
	}
	// 兼容历史表结构差异：
	// - 新结构：requested_format（单值）
	// - 过渡结构：requested_formats / output_formats（可能逗号分隔）
	// 使用 to_jsonb(alias)->>'column' 避免直接引用不存在列触发 SQLSTATE 42703。
	return fmt.Sprintf(`(
	LOWER(COALESCE(NULLIF(to_jsonb(%[1]s)->>'requested_format', ''), '')) = ?
	OR POSITION(
		',' || ? || ','
		IN ',' || REPLACE(
			LOWER(COALESCE(NULLIF(to_jsonb(%[1]s)->>'requested_formats', ''), to_jsonb(%[1]s)->>'output_formats', '')),
			' ',
			''
		) || ','
	) > 0
)`, alias)
}

func buildVideoJobFilterClause(filter *videoImageFeedbackFilter, alias string) (string, []interface{}) {
	if filter == nil {
		return "", nil
	}
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "j"
	}

	clauses := make([]string, 0, 4)
	args := make([]interface{}, 0, 4)
	if filter.UserID > 0 {
		clauses = append(clauses, alias+".user_id = ?")
		args = append(args, filter.UserID)
	}
	if filter.Format != "" {
		clauses = append(clauses, buildVideoJobFormatFilterPredicate(alias))
		args = append(args, filter.Format, filter.Format)
	}
	if filter.GuardReason != "" {
		clauses = append(clauses, `
EXISTS (
	SELECT 1
	FROM jsonb_each_text(
		CASE
			WHEN jsonb_typeof(`+alias+`.metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
			THEN `+alias+`.metrics->'highlight_feedback_v1'->'reason_negative_guard'
			ELSE '{}'::jsonb
		END
	) AS guard_reason(key, value)
	WHERE LOWER(TRIM(guard_reason.key)) = ?
)`)
		args = append(args, filter.GuardReason)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func parseVideoJobsOverviewWindow(raw string) (string, time.Duration, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "24h", "1d", "day":
		return "24h", 24 * time.Hour, nil
	case "7d", "7day", "7days", "week":
		return "7d", 7 * 24 * time.Hour, nil
	case "30d", "30day", "30days", "month":
		return "30d", 30 * 24 * time.Hour, nil
	default:
		return "", 0, errors.New("invalid window, expected one of: 24h, 7d, 30d")
	}
}
