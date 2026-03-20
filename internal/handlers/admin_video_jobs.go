package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

const (
	defaultLiveCoverSceneGuardMaxRiskScenes = 3
	videoJobGIFSubStageBriefing             = "briefing"
	videoJobGIFSubStagePlanning             = "planning"
	videoJobGIFSubStageScoring              = "scoring"
	videoJobGIFSubStageReviewing            = "reviewing"
)

var videoJobGIFSubStageOrder = []string{
	videoJobGIFSubStageBriefing,
	videoJobGIFSubStagePlanning,
	videoJobGIFSubStageScoring,
	videoJobGIFSubStageReviewing,
}

var videoJobGIFSubStageLabelMap = map[string]string{
	videoJobGIFSubStageBriefing:  "Briefing（AI1）",
	videoJobGIFSubStagePlanning:  "Planning（AI2）",
	videoJobGIFSubStageScoring:   "Scoring（评分）",
	videoJobGIFSubStageReviewing: "Reviewing（AI3）",
}

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

type AdminVideoJobGIFSubStageAnomalyStat struct {
	SubStage      string  `json:"sub_stage"`
	SubStageLabel string  `json:"sub_stage_label"`
	Samples       int64   `json:"samples"`
	DoneJobs      int64   `json:"done_jobs"`
	RunningJobs   int64   `json:"running_jobs"`
	PendingJobs   int64   `json:"pending_jobs"`
	DegradedJobs  int64   `json:"degraded_jobs"`
	FailedJobs    int64   `json:"failed_jobs"`
	AnomalyJobs   int64   `json:"anomaly_jobs"`
	AnomalyRate   float64 `json:"anomaly_rate"`
	TopReason     string  `json:"top_reason,omitempty"`
	TopReasonCnt  int64   `json:"top_reason_count,omitempty"`
}

type AdminVideoJobGIFSubStageAnomalyReasonStat struct {
	SubStage      string `json:"sub_stage"`
	SubStageLabel string `json:"sub_stage_label"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
	Count         int64  `json:"count"`
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
	GIFSubStageAnomalyJobsWindow         int64                                                `json:"gif_sub_stage_anomaly_jobs_window"`
	GIFSubStageAnomalyOverview           []AdminVideoJobGIFSubStageAnomalyStat                `json:"gif_sub_stage_anomaly_overview"`
	GIFSubStageAnomalyReasons            []AdminVideoJobGIFSubStageAnomalyReasonStat          `json:"gif_sub_stage_anomaly_reasons"`
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

type adminVideoJobGIFSubStageAnomalyExportRow struct {
	JobID              uint64    `gorm:"column:job_id"`
	UserID             uint64    `gorm:"column:user_id"`
	Title              string    `gorm:"column:title"`
	SourceVideoKey     string    `gorm:"column:source_video_key"`
	JobStatus          string    `gorm:"column:job_status"`
	JobStage           string    `gorm:"column:job_stage"`
	SubStage           string    `gorm:"column:sub_stage"`
	SubStageStatus     string    `gorm:"column:sub_stage_status"`
	SubStageReason     string    `gorm:"column:sub_stage_reason"`
	SubStageDurationMs int64     `gorm:"column:sub_stage_duration_ms"`
	SubStageStartedAt  string    `gorm:"column:sub_stage_started_at"`
	SubStageFinishedAt string    `gorm:"column:sub_stage_finished_at"`
	JobCreatedAt       time.Time `gorm:"column:job_created_at"`
	JobUpdatedAt       time.Time `gorm:"column:job_updated_at"`
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
func convertPublicVideoImageJobToLegacy(row models.VideoImageJobPublic) models.VideoJob {
	outputFormat := strings.TrimSpace(row.RequestedFormat)
	if outputFormat == "" {
		outputFormat = "gif"
	}
	options := parseJSONMap(row.Options)
	metrics := parseJSONMap(row.Metrics)
	priority := strings.TrimSpace(stringFromAny(options["priority"]))
	if priority == "" {
		priority = strings.TrimSpace(stringFromAny(metrics["priority"]))
	}
	if priority == "" {
		priority = "unknown"
	}
	var categoryID *uint64
	if raw := parseUint64FromAny(options["category_id"]); raw > 0 {
		v := raw
		categoryID = &v
	}
	var resultCollectionID *uint64
	if raw := parseUint64FromAny(metrics["result_collection_id"]); raw > 0 {
		v := raw
		resultCollectionID = &v
	}
	return models.VideoJob{
		ID:                 row.ID,
		UserID:             row.UserID,
		Title:              strings.TrimSpace(row.Title),
		SourceVideoKey:     strings.TrimSpace(row.SourceVideoKey),
		CategoryID:         categoryID,
		OutputFormats:      outputFormat,
		Status:             strings.TrimSpace(row.Status),
		Stage:              strings.TrimSpace(row.Stage),
		Progress:           row.Progress,
		Priority:           priority,
		Options:            row.Options,
		Metrics:            row.Metrics,
		ErrorMessage:       strings.TrimSpace(row.ErrorMessage),
		ResultCollectionID: resultCollectionID,
		QueuedAt:           row.CreatedAt,
		StartedAt:          row.StartedAt,
		FinishedAt:         row.FinishedAt,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
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

func buildVideoJobGIFSubStageRowsBaseCTE() string {
	return fmt.Sprintf(`
WITH stage_rows AS (
	SELECT
		j.id AS job_id,
		j.user_id AS user_id,
		COALESCE(NULLIF(TRIM(j.title), ''), '') AS title,
		COALESCE(NULLIF(TRIM(j.source_video_key), ''), '') AS source_video_key,
		LOWER(COALESCE(NULLIF(TRIM(j.status), ''), 'unknown')) AS job_status,
		LOWER(COALESCE(NULLIF(TRIM(j.stage), ''), 'unknown')) AS job_stage,
		LOWER(TRIM(stage_entry.key)) AS sub_stage,
		LOWER(COALESCE(NULLIF(TRIM(stage_entry.value->>'status'), ''), 'pending')) AS sub_stage_status,
		COALESCE(
			NULLIF(TRIM(stage_entry.value->>'error'), ''),
			NULLIF(TRIM(stage_entry.value->>'reason'), ''),
			''
		) AS sub_stage_reason,
		CASE
			WHEN jsonb_typeof(stage_entry.value->'duration_ms') = 'number'
			THEN ROUND((stage_entry.value->>'duration_ms')::double precision)::bigint
			ELSE 0
		END AS sub_stage_duration_ms,
		COALESCE(NULLIF(TRIM(stage_entry.value->>'started_at'), ''), '') AS sub_stage_started_at,
		COALESCE(NULLIF(TRIM(stage_entry.value->>'finished_at'), ''), '') AS sub_stage_finished_at,
		j.created_at AS job_created_at,
		j.updated_at AS job_updated_at
	FROM public.video_image_jobs j
	CROSS JOIN LATERAL jsonb_each(
		CASE
			WHEN jsonb_typeof(j.metrics->'gif_pipeline_sub_stages_v1') = 'object'
			THEN j.metrics->'gif_pipeline_sub_stages_v1'
			ELSE '{}'::jsonb
		END
	) AS stage_entry(key, value)
	WHERE j.created_at >= ?
		AND %s
		AND LOWER(TRIM(stage_entry.key)) IN ('briefing', 'planning', 'scoring', 'reviewing')
)
`, buildVideoJobFormatFilterPredicate("j"))
}

func (h *Handler) loadVideoJobGIFSubStageAnomalyOverview(
	since time.Time,
) ([]AdminVideoJobGIFSubStageAnomalyStat, int64, error) {
	type row struct {
		SubStage     string `gorm:"column:sub_stage"`
		Samples      int64  `gorm:"column:samples"`
		DoneJobs     int64  `gorm:"column:done_jobs"`
		RunningJobs  int64  `gorm:"column:running_jobs"`
		PendingJobs  int64  `gorm:"column:pending_jobs"`
		DegradedJobs int64  `gorm:"column:degraded_jobs"`
		FailedJobs   int64  `gorm:"column:failed_jobs"`
		AnomalyJobs  int64  `gorm:"column:anomaly_jobs"`
	}
	type reasonRow struct {
		SubStage string `gorm:"column:sub_stage"`
		Reason   string `gorm:"column:reason"`
		Count    int64  `gorm:"column:count"`
	}
	type countRow struct {
		Count int64 `gorm:"column:count"`
	}

	baseCTE := buildVideoJobGIFSubStageRowsBaseCTE()
	baseArgs := []interface{}{since, "gif", "gif"}

	var rows []row
	if err := h.db.Raw(baseCTE+`
SELECT
	sub_stage,
	COUNT(*)::bigint AS samples,
	COUNT(*) FILTER (WHERE sub_stage_status = 'done')::bigint AS done_jobs,
	COUNT(*) FILTER (WHERE sub_stage_status = 'running')::bigint AS running_jobs,
	COUNT(*) FILTER (WHERE sub_stage_status = 'pending')::bigint AS pending_jobs,
	COUNT(*) FILTER (WHERE sub_stage_status = 'degraded')::bigint AS degraded_jobs,
	COUNT(*) FILTER (WHERE sub_stage_status = 'failed')::bigint AS failed_jobs,
	COUNT(*) FILTER (WHERE sub_stage_status IN ('failed', 'degraded'))::bigint AS anomaly_jobs
FROM stage_rows
GROUP BY sub_stage
ORDER BY sub_stage ASC
`, baseArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	statsByStage := make(map[string]AdminVideoJobGIFSubStageAnomalyStat, len(videoJobGIFSubStageOrder))
	for _, stage := range videoJobGIFSubStageOrder {
		statsByStage[stage] = AdminVideoJobGIFSubStageAnomalyStat{
			SubStage:      stage,
			SubStageLabel: videoJobGIFSubStageLabel(stage),
		}
	}
	for _, item := range rows {
		stage := normalizeVideoJobGIFSubStage(item.SubStage)
		if stage == "" {
			continue
		}
		stat := statsByStage[stage]
		stat.Samples = item.Samples
		stat.DoneJobs = item.DoneJobs
		stat.RunningJobs = item.RunningJobs
		stat.PendingJobs = item.PendingJobs
		stat.DegradedJobs = item.DegradedJobs
		stat.FailedJobs = item.FailedJobs
		stat.AnomalyJobs = item.AnomalyJobs
		if stat.Samples > 0 {
			stat.AnomalyRate = float64(stat.AnomalyJobs) / float64(stat.Samples)
		}
		statsByStage[stage] = stat
	}

	var reasonRows []reasonRow
	if err := h.db.Raw(baseCTE+`
, ranked AS (
	SELECT
		sub_stage,
		COALESCE(NULLIF(TRIM(sub_stage_reason), ''), '[empty]') AS reason,
		COUNT(*)::bigint AS count,
		ROW_NUMBER() OVER (
			PARTITION BY sub_stage
			ORDER BY COUNT(*) DESC, COALESCE(NULLIF(TRIM(sub_stage_reason), ''), '[empty]') ASC
		) AS rn
	FROM stage_rows
	WHERE sub_stage_status IN ('failed', 'degraded')
	GROUP BY sub_stage, COALESCE(NULLIF(TRIM(sub_stage_reason), ''), '[empty]')
)
SELECT sub_stage, reason, count
FROM ranked
WHERE rn = 1
`, baseArgs...).Scan(&reasonRows).Error; err != nil {
		return nil, 0, err
	}
	for _, item := range reasonRows {
		stage := normalizeVideoJobGIFSubStage(item.SubStage)
		if stage == "" {
			continue
		}
		stat := statsByStage[stage]
		stat.TopReason = strings.TrimSpace(item.Reason)
		stat.TopReasonCnt = item.Count
		statsByStage[stage] = stat
	}

	var anomalyJobsWindow countRow
	if err := h.db.Raw(baseCTE+`
SELECT COUNT(DISTINCT job_id)::bigint AS count
FROM stage_rows
WHERE sub_stage_status IN ('failed', 'degraded')
`, baseArgs...).Scan(&anomalyJobsWindow).Error; err != nil {
		return nil, 0, err
	}

	out := make([]AdminVideoJobGIFSubStageAnomalyStat, 0, len(videoJobGIFSubStageOrder))
	for _, stage := range videoJobGIFSubStageOrder {
		out = append(out, statsByStage[stage])
	}
	return out, anomalyJobsWindow.Count, nil
}

func (h *Handler) loadVideoJobGIFSubStageAnomalyReasons(
	since time.Time,
	limit int,
) ([]AdminVideoJobGIFSubStageAnomalyReasonStat, error) {
	type row struct {
		SubStage string `gorm:"column:sub_stage"`
		Status   string `gorm:"column:status"`
		Reason   string `gorm:"column:reason"`
		Count    int64  `gorm:"column:count"`
	}

	if limit <= 0 {
		limit = 12
	}
	if limit > 100 {
		limit = 100
	}

	baseCTE := buildVideoJobGIFSubStageRowsBaseCTE()
	baseArgs := []interface{}{since, "gif", "gif", limit}

	var rows []row
	if err := h.db.Raw(baseCTE+`
SELECT
	sub_stage,
	sub_stage_status AS status,
	COALESCE(NULLIF(TRIM(sub_stage_reason), ''), '[empty]') AS reason,
	COUNT(*)::bigint AS count
FROM stage_rows
WHERE sub_stage_status IN ('failed', 'degraded')
GROUP BY sub_stage, sub_stage_status, COALESCE(NULLIF(TRIM(sub_stage_reason), ''), '[empty]')
ORDER BY count DESC, sub_stage ASC, sub_stage_status ASC, reason ASC
LIMIT ?
`, baseArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobGIFSubStageAnomalyReasonStat, 0, len(rows))
	for _, item := range rows {
		stage := normalizeVideoJobGIFSubStage(item.SubStage)
		if stage == "" {
			continue
		}
		out = append(out, AdminVideoJobGIFSubStageAnomalyReasonStat{
			SubStage:      stage,
			SubStageLabel: videoJobGIFSubStageLabel(stage),
			Status:        normalizeVideoJobGIFSubStageStatus(item.Status),
			Reason:        strings.TrimSpace(item.Reason),
			Count:         item.Count,
		})
	}
	return out, nil
}

func (h *Handler) loadVideoJobGIFSubStageAnomalyExportRows(
	since time.Time,
	subStage string,
	subStatus string,
	limit int,
) ([]adminVideoJobGIFSubStageAnomalyExportRow, int64, int64, error) {
	type countRow struct {
		TotalRows int64 `gorm:"column:total_rows"`
		TotalJobs int64 `gorm:"column:total_jobs"`
	}

	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	subStage = normalizeVideoJobGIFSubStage(subStage)
	subStatus = normalizeVideoJobGIFSubStageStatus(subStatus)

	clauses := []string{"sub_stage_status IN ('failed', 'degraded')"}
	filterArgs := make([]interface{}, 0, 2)
	if subStage != "" {
		clauses = append(clauses, "sub_stage = ?")
		filterArgs = append(filterArgs, subStage)
	}
	if subStatus != "" {
		clauses = append(clauses, "sub_stage_status = ?")
		filterArgs = append(filterArgs, subStatus)
	}
	whereSQL := strings.Join(clauses, " AND ")
	baseCTE := buildVideoJobGIFSubStageRowsBaseCTE()
	baseArgs := []interface{}{since, "gif", "gif"}

	countArgs := make([]interface{}, 0, len(baseArgs)+len(filterArgs))
	countArgs = append(countArgs, baseArgs...)
	countArgs = append(countArgs, filterArgs...)
	var totals countRow
	if err := h.db.Raw(
		baseCTE+`
SELECT
	COUNT(*)::bigint AS total_rows,
	COUNT(DISTINCT job_id)::bigint AS total_jobs
FROM stage_rows
WHERE `+whereSQL,
		countArgs...,
	).Scan(&totals).Error; err != nil {
		return nil, 0, 0, err
	}

	queryArgs := make([]interface{}, 0, len(baseArgs)+len(filterArgs)+1)
	queryArgs = append(queryArgs, baseArgs...)
	queryArgs = append(queryArgs, filterArgs...)
	queryArgs = append(queryArgs, limit)
	var rows []adminVideoJobGIFSubStageAnomalyExportRow
	if err := h.db.Raw(
		baseCTE+`
SELECT
	job_id,
	user_id,
	title,
	source_video_key,
	job_status,
	job_stage,
	sub_stage,
	sub_stage_status,
	COALESCE(NULLIF(TRIM(sub_stage_reason), ''), '[empty]') AS sub_stage_reason,
	sub_stage_duration_ms,
	sub_stage_started_at,
	sub_stage_finished_at,
	job_created_at,
	job_updated_at
FROM stage_rows
WHERE `+whereSQL+`
ORDER BY job_created_at DESC, job_id DESC, sub_stage ASC
LIMIT ?
`,
		queryArgs...,
	).Scan(&rows).Error; err != nil {
		return nil, 0, 0, err
	}
	return rows, totals.TotalRows, totals.TotalJobs, nil
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

func normalizeVideoJobGIFSubStage(raw string) string {
	stage := strings.ToLower(strings.TrimSpace(raw))
	for _, item := range videoJobGIFSubStageOrder {
		if stage == item {
			return stage
		}
	}
	return ""
}

func normalizeVideoJobGIFSubStageStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "failed":
		return "failed"
	case "degraded":
		return "degraded"
	default:
		return ""
	}
}

func videoJobGIFSubStageLabel(stage string) string {
	normalized := normalizeVideoJobGIFSubStage(stage)
	if normalized == "" {
		return strings.TrimSpace(stage)
	}
	return videoJobGIFSubStageLabelMap[normalized]
}

func parseVideoJobGIFSubStageFilter(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || value == "all" {
		return "", nil
	}
	stage := normalizeVideoJobGIFSubStage(value)
	if stage == "" {
		return "", errors.New("invalid sub_stage")
	}
	return stage, nil
}

func parseVideoJobGIFSubStageStatusFilter(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || value == "all" {
		return "", nil
	}
	status := normalizeVideoJobGIFSubStageStatus(value)
	if status == "" {
		return "", errors.New("invalid sub_status")
	}
	return status, nil
}

func buildVideoJobAnyGIFSubStageAnomalyPredicate() string {
	return `
EXISTS (
	SELECT 1
	FROM jsonb_each(
		CASE
			WHEN jsonb_typeof(metrics->'gif_pipeline_sub_stages_v1') = 'object'
			THEN metrics->'gif_pipeline_sub_stages_v1'
			ELSE '{}'::jsonb
		END
	) AS stage_row(key, value)
	WHERE LOWER(TRIM(stage_row.key)) IN ('briefing', 'planning', 'scoring', 'reviewing')
		AND LOWER(COALESCE(NULLIF(TRIM(stage_row.value->>'status'), ''), 'pending')) IN ('failed', 'degraded')
)`
}

func buildVideoJobGIFSubStageAnomalyPredicate() string {
	return `
LOWER(COALESCE(
	NULLIF(
		TRIM(
			jsonb_extract_path_text(
				CASE
					WHEN jsonb_typeof(metrics->'gif_pipeline_sub_stages_v1') = 'object'
					THEN metrics->'gif_pipeline_sub_stages_v1'
					ELSE '{}'::jsonb
				END,
				?,
				'status'
			)
		),
		''
	),
	'pending'
)) IN ('failed', 'degraded')`
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
