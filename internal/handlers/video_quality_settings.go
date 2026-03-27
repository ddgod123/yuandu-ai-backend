package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const videoQualityRolloutApplyCooldown = 10 * time.Minute

type VideoQualitySettingRequest struct {
	MinBrightness                        float64 `json:"min_brightness"`
	MaxBrightness                        float64 `json:"max_brightness"`
	BlurThresholdFactor                  float64 `json:"blur_threshold_factor"`
	BlurThresholdMin                     float64 `json:"blur_threshold_min"`
	BlurThresholdMax                     float64 `json:"blur_threshold_max"`
	DuplicateHammingThreshold            int     `json:"duplicate_hamming_threshold"`
	DuplicateBacktrackFrames             int     `json:"duplicate_backtrack_frames"`
	FallbackBlurRelaxFactor              float64 `json:"fallback_blur_relax_factor"`
	FallbackHammingThreshold             int     `json:"fallback_hamming_threshold"`
	MinKeepBase                          int     `json:"min_keep_base"`
	MinKeepRatio                         float64 `json:"min_keep_ratio"`
	QualityAnalysisWorkers               int     `json:"quality_analysis_workers"`
	UploadConcurrency                    int     `json:"upload_concurrency"`
	GIFProfile                           string  `json:"gif_profile"`
	WebPProfile                          string  `json:"webp_profile"`
	LiveProfile                          string  `json:"live_profile"`
	JPGProfile                           string  `json:"jpg_profile"`
	PNGProfile                           string  `json:"png_profile"`
	GIFDefaultFPS                        int     `json:"gif_default_fps"`
	GIFDefaultMaxColors                  int     `json:"gif_default_max_colors"`
	GIFDitherMode                        string  `json:"gif_dither_mode"`
	GIFTargetSizeKB                      int     `json:"gif_target_size_kb"`
	GIFGifsicleEnabled                   bool    `json:"gif_gifsicle_enabled"`
	GIFGifsicleLevel                     int     `json:"gif_gifsicle_level"`
	GIFGifsicleSkipBelowKB               int     `json:"gif_gifsicle_skip_below_kb"`
	GIFGifsicleMinGainRatio              float64 `json:"gif_gifsicle_min_gain_ratio"`
	GIFLoopTuneEnabled                   bool    `json:"gif_loop_tune_enabled"`
	GIFLoopTuneMinEnableSec              float64 `json:"gif_loop_tune_min_enable_sec"`
	GIFLoopTuneMinImprovement            float64 `json:"gif_loop_tune_min_improvement"`
	GIFLoopTuneMotionTarget              float64 `json:"gif_loop_tune_motion_target"`
	GIFLoopTunePreferDuration            float64 `json:"gif_loop_tune_prefer_duration_sec"`
	GIFCandidateMaxOutputs               int     `json:"gif_candidate_max_outputs"`
	GIFCandidateLongVideoMaxOutputs      int     `json:"gif_candidate_long_video_max_outputs"`
	GIFCandidateUltraVideoMaxOutputs     int     `json:"gif_candidate_ultra_video_max_outputs"`
	GIFCandidateConfidenceThreshold      float64 `json:"gif_candidate_confidence_threshold"`
	GIFCandidateDedupIOUThreshold        float64 `json:"gif_candidate_dedup_iou_threshold"`
	GIFRenderBudgetNormalMultiplier      float64 `json:"gif_render_budget_normal_mult"`
	GIFRenderBudgetLongMultiplier        float64 `json:"gif_render_budget_long_mult"`
	GIFRenderBudgetUltraMultiplier       float64 `json:"gif_render_budget_ultra_mult"`
	GIFPipelineShortVideoMaxSec          float64 `json:"gif_pipeline_short_video_max_sec"`
	GIFPipelineLongVideoMinSec           float64 `json:"gif_pipeline_long_video_min_sec"`
	GIFPipelineShortVideoMode            string  `json:"gif_pipeline_short_video_mode"`
	GIFPipelineDefaultMode               string  `json:"gif_pipeline_default_mode"`
	GIFPipelineLongVideoMode             string  `json:"gif_pipeline_long_video_mode"`
	GIFPipelineHighPriorityEnabled       bool    `json:"gif_pipeline_high_priority_enabled"`
	GIFPipelineHighPriorityMode          string  `json:"gif_pipeline_high_priority_mode"`
	GIFDurationTierMediumSec             float64 `json:"gif_duration_tier_medium_sec"`
	GIFDurationTierLongSec               float64 `json:"gif_duration_tier_long_sec"`
	GIFDurationTierUltraSec              float64 `json:"gif_duration_tier_ultra_sec"`
	GIFSegmentTimeoutMinSec              int     `json:"gif_segment_timeout_min_sec"`
	GIFSegmentTimeoutMaxSec              int     `json:"gif_segment_timeout_max_sec"`
	GIFSegmentTimeoutFallbackCapSec      int     `json:"gif_segment_timeout_fallback_cap_sec"`
	GIFSegmentTimeoutEmergencyCapSec     int     `json:"gif_segment_timeout_emergency_cap_sec"`
	GIFSegmentTimeoutLastResortCapSec    int     `json:"gif_segment_timeout_last_resort_cap_sec"`
	GIFRenderRetryMaxAttempts            int     `json:"gif_render_retry_max_attempts"`
	GIFRenderRetryPrimaryColorsFloor     int     `json:"gif_render_retry_primary_colors_floor"`
	GIFRenderRetryPrimaryColorsStep      int     `json:"gif_render_retry_primary_colors_step"`
	GIFRenderRetryFPSFloor               int     `json:"gif_render_retry_fps_floor"`
	GIFRenderRetryFPSStep                int     `json:"gif_render_retry_fps_step"`
	GIFRenderRetryWidthTrigger           int     `json:"gif_render_retry_width_trigger"`
	GIFRenderRetryWidthScale             float64 `json:"gif_render_retry_width_scale"`
	GIFRenderRetryWidthFloor             int     `json:"gif_render_retry_width_floor"`
	GIFRenderRetrySecondaryColorsFloor   int     `json:"gif_render_retry_secondary_colors_floor"`
	GIFRenderRetrySecondaryColorsStep    int     `json:"gif_render_retry_secondary_colors_step"`
	GIFRenderInitialSizeFPSCap           int     `json:"gif_render_initial_size_fps_cap"`
	GIFRenderInitialClarityFPSFloor      int     `json:"gif_render_initial_clarity_fps_floor"`
	GIFRenderInitialSizeColorsCap        int     `json:"gif_render_initial_size_colors_cap"`
	GIFRenderInitialClarityColorsFloor   int     `json:"gif_render_initial_clarity_colors_floor"`
	GIFMotionLowScoreThreshold           float64 `json:"gif_motion_low_score_threshold"`
	GIFMotionHighScoreThreshold          float64 `json:"gif_motion_high_score_threshold"`
	GIFMotionLowFPSDelta                 int     `json:"gif_motion_low_fps_delta"`
	GIFMotionHighFPSDelta                int     `json:"gif_motion_high_fps_delta"`
	GIFAdaptiveFPSMin                    int     `json:"gif_adaptive_fps_min"`
	GIFAdaptiveFPSMax                    int     `json:"gif_adaptive_fps_max"`
	GIFWidthSizeLow                      int     `json:"gif_width_size_low"`
	GIFWidthSizeMedium                   int     `json:"gif_width_size_medium"`
	GIFWidthSizeHigh                     int     `json:"gif_width_size_high"`
	GIFWidthClarityLow                   int     `json:"gif_width_clarity_low"`
	GIFWidthClarityMedium                int     `json:"gif_width_clarity_medium"`
	GIFWidthClarityHigh                  int     `json:"gif_width_clarity_high"`
	GIFColorsSizeLow                     int     `json:"gif_colors_size_low"`
	GIFColorsSizeMedium                  int     `json:"gif_colors_size_medium"`
	GIFColorsSizeHigh                    int     `json:"gif_colors_size_high"`
	GIFColorsClarityLow                  int     `json:"gif_colors_clarity_low"`
	GIFColorsClarityMedium               int     `json:"gif_colors_clarity_medium"`
	GIFColorsClarityHigh                 int     `json:"gif_colors_clarity_high"`
	GIFDurationLowSec                    float64 `json:"gif_duration_low_sec"`
	GIFDurationMediumSec                 float64 `json:"gif_duration_medium_sec"`
	GIFDurationHighSec                   float64 `json:"gif_duration_high_sec"`
	GIFDurationSizeProfileMaxSec         float64 `json:"gif_duration_size_profile_max_sec"`
	GIFDownshiftHighResLongSideThreshold int     `json:"gif_downshift_high_res_long_side_threshold"`
	GIFDownshiftEarlyDurationSec         float64 `json:"gif_downshift_early_duration_sec"`
	GIFDownshiftEarlyLongSideThreshold   int     `json:"gif_downshift_early_long_side_threshold"`
	GIFDownshiftMediumFPSCap             int     `json:"gif_downshift_medium_fps_cap"`
	GIFDownshiftMediumWidthCap           int     `json:"gif_downshift_medium_width_cap"`
	GIFDownshiftMediumColorsCap          int     `json:"gif_downshift_medium_colors_cap"`
	GIFDownshiftMediumDurationCapSec     float64 `json:"gif_downshift_medium_duration_cap_sec"`
	GIFDownshiftLongFPSCap               int     `json:"gif_downshift_long_fps_cap"`
	GIFDownshiftLongWidthCap             int     `json:"gif_downshift_long_width_cap"`
	GIFDownshiftLongColorsCap            int     `json:"gif_downshift_long_colors_cap"`
	GIFDownshiftLongDurationCapSec       float64 `json:"gif_downshift_long_duration_cap_sec"`
	GIFDownshiftUltraFPSCap              int     `json:"gif_downshift_ultra_fps_cap"`
	GIFDownshiftUltraWidthCap            int     `json:"gif_downshift_ultra_width_cap"`
	GIFDownshiftUltraColorsCap           int     `json:"gif_downshift_ultra_colors_cap"`
	GIFDownshiftUltraDurationCapSec      float64 `json:"gif_downshift_ultra_duration_cap_sec"`
	GIFDownshiftHighResFPSCap            int     `json:"gif_downshift_high_res_fps_cap"`
	GIFDownshiftHighResWidthCap          int     `json:"gif_downshift_high_res_width_cap"`
	GIFDownshiftHighResColorsCap         int     `json:"gif_downshift_high_res_colors_cap"`
	GIFDownshiftHighResDurationCapSec    float64 `json:"gif_downshift_high_res_duration_cap_sec"`
	GIFTimeoutFallbackFPSCap             int     `json:"gif_timeout_fallback_fps_cap"`
	GIFTimeoutFallbackWidthCap           int     `json:"gif_timeout_fallback_width_cap"`
	GIFTimeoutFallbackColorsCap          int     `json:"gif_timeout_fallback_colors_cap"`
	GIFTimeoutFallbackMinWidth           int     `json:"gif_timeout_fallback_min_width"`
	GIFTimeoutFallbackUltraFPSCap        int     `json:"gif_timeout_fallback_ultra_fps_cap"`
	GIFTimeoutFallbackUltraWidthCap      int     `json:"gif_timeout_fallback_ultra_width_cap"`
	GIFTimeoutFallbackUltraColorsCap     int     `json:"gif_timeout_fallback_ultra_colors_cap"`
	GIFTimeoutEmergencyFPSCap            int     `json:"gif_timeout_emergency_fps_cap"`
	GIFTimeoutEmergencyWidthCap          int     `json:"gif_timeout_emergency_width_cap"`
	GIFTimeoutEmergencyColorsCap         int     `json:"gif_timeout_emergency_colors_cap"`
	GIFTimeoutEmergencyMinWidth          int     `json:"gif_timeout_emergency_min_width"`
	GIFTimeoutEmergencyDurationTrigger   float64 `json:"gif_timeout_emergency_duration_trigger_sec"`
	GIFTimeoutEmergencyDurationScale     float64 `json:"gif_timeout_emergency_duration_scale"`
	GIFTimeoutEmergencyDurationMinSec    float64 `json:"gif_timeout_emergency_duration_min_sec"`
	GIFTimeoutLastResortFPSCap           int     `json:"gif_timeout_last_resort_fps_cap"`
	GIFTimeoutLastResortWidthCap         int     `json:"gif_timeout_last_resort_width_cap"`
	GIFTimeoutLastResortColorsCap        int     `json:"gif_timeout_last_resort_colors_cap"`
	GIFTimeoutLastResortMinWidth         int     `json:"gif_timeout_last_resort_min_width"`
	GIFTimeoutLastResortDurationMinSec   float64 `json:"gif_timeout_last_resort_duration_min_sec"`
	GIFTimeoutLastResortDurationMaxSec   float64 `json:"gif_timeout_last_resort_duration_max_sec"`
	WebPTargetSizeKB                     int     `json:"webp_target_size_kb"`
	JPGTargetSizeKB                      int     `json:"jpg_target_size_kb"`
	PNGTargetSizeKB                      int     `json:"png_target_size_kb"`
	StillMinBlurScore                    float64 `json:"still_min_blur_score"`
	StillMinExposureScore                float64 `json:"still_min_exposure_score"`
	StillMinWidth                        int     `json:"still_min_width"`
	StillMinHeight                       int     `json:"still_min_height"`
	LiveCoverPortraitWeight              float64 `json:"live_cover_portrait_weight"`
	LiveCoverSceneMinSamples             int     `json:"live_cover_scene_min_samples"`
	LiveCoverGuardMinTotal               int     `json:"live_cover_guard_min_total"`
	LiveCoverGuardScoreFloor             float64 `json:"live_cover_guard_score_floor"`
	HighlightFeedbackEnabled             bool    `json:"highlight_feedback_enabled"`
	HighlightFeedbackRollout             int     `json:"highlight_feedback_rollout_percent"`
	HighlightFeedbackMinJobs             int     `json:"highlight_feedback_min_engaged_jobs"`
	HighlightFeedbackMinScore            float64 `json:"highlight_feedback_min_weighted_signals"`
	HighlightFeedbackBoost               float64 `json:"highlight_feedback_boost_scale"`
	HighlightWeightPosition              float64 `json:"highlight_feedback_position_weight"`
	HighlightWeightDuration              float64 `json:"highlight_feedback_duration_weight"`
	HighlightWeightReason                float64 `json:"highlight_feedback_reason_weight"`
	HighlightNegativeGuardEnabled        bool    `json:"highlight_feedback_negative_guard_enabled"`
	HighlightNegativeGuardThreshold      float64 `json:"highlight_feedback_negative_guard_dominance_threshold"`
	HighlightNegativeGuardMinWeight      float64 `json:"highlight_feedback_negative_guard_min_weight"`
	HighlightNegativePenaltyScale        float64 `json:"highlight_feedback_negative_guard_penalty_scale"`
	HighlightNegativePenaltyWeight       float64 `json:"highlight_feedback_negative_guard_penalty_weight"`
	AIDirectorInputMode                  string  `json:"ai_director_input_mode"`
	AIDirectorOperatorInstruction        string  `json:"ai_director_operator_instruction"`
	AIDirectorOperatorInstructionVersion string  `json:"ai_director_operator_instruction_version"`
	AIDirectorOperatorEnabled            bool    `json:"ai_director_operator_enabled"`
	AIDirectorConstraintOverrideEnabled  bool    `json:"ai_director_constraint_override_enabled"`
	AIDirectorCountExpandRatio           float64 `json:"ai_director_count_expand_ratio"`
	AIDirectorDurationExpandRatio        float64 `json:"ai_director_duration_expand_ratio"`
	AIDirectorCountAbsoluteCap           int     `json:"ai_director_count_absolute_cap"`
	AIDirectorDurationAbsoluteCapSec     float64 `json:"ai_director_duration_absolute_cap_sec"`
	GIFAIJudgeHardGateMinOverallScore    float64 `json:"gif_ai_judge_hard_gate_min_overall_score"`
	GIFAIJudgeHardGateMinClarityScore    float64 `json:"gif_ai_judge_hard_gate_min_clarity_score"`
	GIFAIJudgeHardGateMinLoopScore       float64 `json:"gif_ai_judge_hard_gate_min_loop_score"`
	GIFAIJudgeHardGateMinOutputScore     float64 `json:"gif_ai_judge_hard_gate_min_output_score"`
	GIFAIJudgeHardGateMinDurationMS      int     `json:"gif_ai_judge_hard_gate_min_duration_ms"`
	GIFAIJudgeHardGateSizeMultiplier     int     `json:"gif_ai_judge_hard_gate_size_multiplier"`
	GIFHealthAlertThresholdSettings
	FeedbackIntegrityAlertThresholdSettings
}

type VideoQualitySettingResponse struct {
	videojobs.QualitySettings
	GIFHealthAlertThresholdSettings
	FeedbackIntegrityAlertThresholdSettings
	CreatedAt       string   `json:"created_at,omitempty"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
	FormatScope     string   `json:"format_scope,omitempty"`
	ResolvedFrom    []string `json:"resolved_from,omitempty"`
	OverrideVersion string   `json:"override_version,omitempty"`
}

type ApplyVideoQualityRolloutSuggestionResponse struct {
	Applied         bool                                       `json:"applied"`
	AppliedAt       string                                     `json:"applied_at,omitempty"`
	CooldownSeconds int                                        `json:"cooldown_seconds,omitempty"`
	NextAllowedAt   string                                     `json:"next_allowed_at,omitempty"`
	Message         string                                     `json:"message"`
	Window          string                                     `json:"window"`
	ConfirmWindows  int                                        `json:"confirm_windows"`
	Recommendation  AdminVideoJobFeedbackRolloutRecommendation `json:"recommendation"`
	Setting         VideoQualitySettingResponse                `json:"setting"`
}

type VideoQualityRolloutEffectMetric struct {
	JobsTotal         int64   `json:"jobs_total"`
	JobsDone          int64   `json:"jobs_done"`
	JobsFailed        int64   `json:"jobs_failed"`
	DoneRate          float64 `json:"done_rate"`
	FailedRate        float64 `json:"failed_rate"`
	OutputSamples     int64   `json:"output_samples"`
	AvgOutputScore    float64 `json:"avg_output_score"`
	AvgLoopClosure    float64 `json:"avg_loop_closure"`
	LoopEffectiveRate float64 `json:"loop_effective_rate"`
	LoopFallbackRate  float64 `json:"loop_fallback_rate"`
}

type VideoQualityRolloutEffectDelta struct {
	DoneRateDelta          float64 `json:"done_rate_delta"`
	FailedRateDelta        float64 `json:"failed_rate_delta"`
	AvgOutputScoreDelta    float64 `json:"avg_output_score_delta"`
	AvgLoopClosureDelta    float64 `json:"avg_loop_closure_delta"`
	LoopEffectiveRateDelta float64 `json:"loop_effective_rate_delta"`
	LoopFallbackRateDelta  float64 `json:"loop_fallback_rate_delta"`
}

type VideoQualityRolloutEffectCard struct {
	AuditID             uint64                          `json:"audit_id"`
	AdminID             uint64                          `json:"admin_id"`
	AppliedAt           string                          `json:"applied_at"`
	Window              string                          `json:"window"`
	FromRolloutPercent  int                             `json:"from_rollout_percent"`
	ToRolloutPercent    int                             `json:"to_rollout_percent"`
	BaseWindowStart     string                          `json:"base_window_start"`
	BaseWindowEnd       string                          `json:"base_window_end"`
	TargetWindowStart   string                          `json:"target_window_start"`
	TargetWindowEnd     string                          `json:"target_window_end"`
	BaseMetrics         VideoQualityRolloutEffectMetric `json:"base_metrics"`
	TargetMetrics       VideoQualityRolloutEffectMetric `json:"target_metrics"`
	Delta               VideoQualityRolloutEffectDelta  `json:"delta"`
	Verdict             string                          `json:"verdict"`
	VerdictReason       string                          `json:"verdict_reason,omitempty"`
	RecommendationState string                          `json:"recommendation_state,omitempty"`
}

type ListVideoQualityRolloutEffectsResponse struct {
	Items []VideoQualityRolloutEffectCard `json:"items"`
}

func validateVideoQualitySettingRequest(req VideoQualitySettingRequest) error {
	if req.MinBrightness <= 0 || req.MaxBrightness <= 0 || req.MinBrightness >= req.MaxBrightness {
		return errors.New("invalid brightness range: require min_brightness > 0 and min_brightness < max_brightness")
	}
	if req.BlurThresholdMin <= 0 || req.BlurThresholdMax <= 0 || req.BlurThresholdMin > req.BlurThresholdMax {
		return errors.New("invalid blur threshold range: require blur_threshold_min > 0 and blur_threshold_min <= blur_threshold_max")
	}
	if req.MinKeepBase < 1 {
		return errors.New("invalid min_keep_base: expected >= 1")
	}
	if req.MinKeepRatio <= 0 || req.MinKeepRatio > 1 {
		return errors.New("invalid min_keep_ratio: expected (0,1]")
	}
	if req.QualityAnalysisWorkers < 1 || req.UploadConcurrency < 1 {
		return errors.New("invalid workers/concurrency: expected >= 1")
	}
	if req.GIFDefaultFPS < 1 || req.GIFDefaultFPS > 60 {
		return errors.New("invalid gif_default_fps: expected 1..60")
	}
	if req.GIFDefaultMaxColors < 2 || req.GIFDefaultMaxColors > 256 {
		return errors.New("invalid gif_default_max_colors: expected 2..256")
	}
	if req.GIFTargetSizeKB < 64 || req.WebPTargetSizeKB < 64 || req.JPGTargetSizeKB < 32 || req.PNGTargetSizeKB < 32 {
		return errors.New("invalid target size kb: expected gif/webp>=64 and jpg/png>=32")
	}
	if req.GIFGifsicleLevel < 1 || req.GIFGifsicleLevel > 3 {
		return errors.New("invalid gif_gifsicle_level: expected 1..3")
	}
	if req.GIFGifsicleSkipBelowKB < 0 || req.GIFGifsicleSkipBelowKB > 4096 {
		return errors.New("invalid gif_gifsicle_skip_below_kb: expected 0..4096")
	}
	if req.GIFGifsicleMinGainRatio < 0 || req.GIFGifsicleMinGainRatio > 0.5 {
		return errors.New("invalid gif_gifsicle_min_gain_ratio: expected 0..0.5")
	}
	if req.GIFCandidateMaxOutputs < 1 || req.GIFCandidateMaxOutputs > videojobs.DefaultQualitySettings().AIDirectorCountAbsoluteCap*2 {
		return errors.New("invalid gif_candidate_max_outputs: expected 1..20")
	}
	if req.GIFCandidateLongVideoMaxOutputs < 1 || req.GIFCandidateLongVideoMaxOutputs > videojobs.DefaultQualitySettings().AIDirectorCountAbsoluteCap*2 {
		return errors.New("invalid gif_candidate_long_video_max_outputs: expected 1..20")
	}
	if req.GIFCandidateUltraVideoMaxOutputs < 1 || req.GIFCandidateUltraVideoMaxOutputs > videojobs.DefaultQualitySettings().AIDirectorCountAbsoluteCap*2 {
		return errors.New("invalid gif_candidate_ultra_video_max_outputs: expected 1..20")
	}
	if req.GIFCandidateLongVideoMaxOutputs > req.GIFCandidateMaxOutputs {
		return errors.New("invalid gif candidate cap: long_video_max_outputs must be <= gif_candidate_max_outputs")
	}
	if req.GIFCandidateUltraVideoMaxOutputs > req.GIFCandidateLongVideoMaxOutputs {
		return errors.New("invalid gif candidate cap: ultra_video_max_outputs must be <= long_video_max_outputs")
	}
	if req.GIFCandidateConfidenceThreshold < 0 || req.GIFCandidateConfidenceThreshold > 0.95 {
		return errors.New("invalid gif_candidate_confidence_threshold: expected 0..0.95")
	}
	if req.GIFCandidateDedupIOUThreshold <= 0 || req.GIFCandidateDedupIOUThreshold >= 1 {
		return errors.New("invalid gif_candidate_dedup_iou_threshold: expected (0,1)")
	}
	if req.GIFRenderBudgetNormalMultiplier < 1 || req.GIFRenderBudgetNormalMultiplier > 4 {
		return errors.New("invalid gif_render_budget_normal_mult: expected 1..4")
	}
	if req.GIFRenderBudgetLongMultiplier < 0.8 || req.GIFRenderBudgetLongMultiplier > req.GIFRenderBudgetNormalMultiplier {
		return errors.New("invalid gif_render_budget_long_mult: expected 0.8..normal_mult")
	}
	if req.GIFRenderBudgetUltraMultiplier < 0.5 || req.GIFRenderBudgetUltraMultiplier > req.GIFRenderBudgetLongMultiplier {
		return errors.New("invalid gif_render_budget_ultra_mult: expected 0.5..long_mult")
	}
	if req.GIFPipelineShortVideoMaxSec < 3 || req.GIFPipelineShortVideoMaxSec > 300 {
		return errors.New("invalid gif_pipeline_short_video_max_sec: expected 3..300")
	}
	if req.GIFPipelineLongVideoMinSec <= req.GIFPipelineShortVideoMaxSec || req.GIFPipelineLongVideoMinSec > 3600 {
		return errors.New("invalid gif_pipeline_long_video_min_sec: expected > short_video_max_sec and <= 3600")
	}
	if !isValidGIFPipelineMode(req.GIFPipelineShortVideoMode) ||
		!isValidGIFPipelineMode(req.GIFPipelineDefaultMode) ||
		!isValidGIFPipelineMode(req.GIFPipelineLongVideoMode) ||
		!isValidGIFPipelineMode(req.GIFPipelineHighPriorityMode) {
		return errors.New("invalid gif pipeline mode: expected one of light|standard|hq")
	}
	if req.GIFDurationTierMediumSec < 10 || req.GIFDurationTierMediumSec > 600 {
		return errors.New("invalid gif_duration_tier_medium_sec: expected 10..600")
	}
	if req.GIFDurationTierLongSec <= req.GIFDurationTierMediumSec || req.GIFDurationTierLongSec > 1800 {
		return errors.New("invalid gif_duration_tier_long_sec: expected > medium and <= 1800")
	}
	if req.GIFDurationTierUltraSec <= req.GIFDurationTierLongSec || req.GIFDurationTierUltraSec > 7200 {
		return errors.New("invalid gif_duration_tier_ultra_sec: expected > long and <= 7200")
	}
	if req.GIFSegmentTimeoutMinSec < 10 || req.GIFSegmentTimeoutMinSec > 300 {
		return errors.New("invalid gif_segment_timeout_min_sec: expected 10..300")
	}
	if req.GIFSegmentTimeoutMaxSec < req.GIFSegmentTimeoutMinSec || req.GIFSegmentTimeoutMaxSec > 600 {
		return errors.New("invalid gif_segment_timeout_max_sec: expected >= min and <= 600")
	}
	if req.GIFSegmentTimeoutFallbackCapSec < req.GIFSegmentTimeoutMinSec || req.GIFSegmentTimeoutFallbackCapSec > req.GIFSegmentTimeoutMaxSec {
		return errors.New("invalid gif_segment_timeout_fallback_cap_sec: expected in [min,max]")
	}
	if req.GIFSegmentTimeoutEmergencyCapSec < req.GIFSegmentTimeoutMinSec || req.GIFSegmentTimeoutEmergencyCapSec > req.GIFSegmentTimeoutFallbackCapSec {
		return errors.New("invalid gif_segment_timeout_emergency_cap_sec: expected in [min,fallback_cap]")
	}
	if req.GIFSegmentTimeoutLastResortCapSec < req.GIFSegmentTimeoutMinSec || req.GIFSegmentTimeoutLastResortCapSec > req.GIFSegmentTimeoutEmergencyCapSec {
		return errors.New("invalid gif_segment_timeout_last_resort_cap_sec: expected in [min,emergency_cap]")
	}
	if req.GIFRenderRetryMaxAttempts < 1 || req.GIFRenderRetryMaxAttempts > 12 {
		return errors.New("invalid gif_render_retry_max_attempts: expected 1..12")
	}
	if req.GIFRenderRetryPrimaryColorsFloor < 16 || req.GIFRenderRetryPrimaryColorsFloor > 256 {
		return errors.New("invalid gif_render_retry_primary_colors_floor: expected 16..256")
	}
	if req.GIFRenderRetryPrimaryColorsStep < 1 || req.GIFRenderRetryPrimaryColorsStep > 128 {
		return errors.New("invalid gif_render_retry_primary_colors_step: expected 1..128")
	}
	if req.GIFRenderRetryFPSFloor < 2 || req.GIFRenderRetryFPSFloor > 30 {
		return errors.New("invalid gif_render_retry_fps_floor: expected 2..30")
	}
	if req.GIFRenderRetryFPSStep < 1 || req.GIFRenderRetryFPSStep > 12 {
		return errors.New("invalid gif_render_retry_fps_step: expected 1..12")
	}
	if req.GIFRenderRetryWidthTrigger < 240 || req.GIFRenderRetryWidthTrigger > 2048 {
		return errors.New("invalid gif_render_retry_width_trigger: expected 240..2048")
	}
	if req.GIFRenderRetryWidthScale < 0.5 || req.GIFRenderRetryWidthScale > 0.98 {
		return errors.New("invalid gif_render_retry_width_scale: expected 0.5..0.98")
	}
	if req.GIFRenderRetryWidthFloor < 240 || req.GIFRenderRetryWidthFloor > req.GIFRenderRetryWidthTrigger {
		return errors.New("invalid gif_render_retry_width_floor: expected 240..width_trigger")
	}
	if req.GIFRenderRetrySecondaryColorsFloor < 16 || req.GIFRenderRetrySecondaryColorsFloor > req.GIFRenderRetryPrimaryColorsFloor {
		return errors.New("invalid gif_render_retry_secondary_colors_floor: expected 16..primary_colors_floor")
	}
	if req.GIFRenderRetrySecondaryColorsStep < 1 || req.GIFRenderRetrySecondaryColorsStep > 128 {
		return errors.New("invalid gif_render_retry_secondary_colors_step: expected 1..128")
	}
	if req.GIFRenderInitialSizeFPSCap < 2 || req.GIFRenderInitialSizeFPSCap > 30 {
		return errors.New("invalid gif_render_initial_size_fps_cap: expected 2..30")
	}
	if req.GIFRenderInitialClarityFPSFloor < req.GIFRenderInitialSizeFPSCap || req.GIFRenderInitialClarityFPSFloor > 30 {
		return errors.New("invalid gif_render_initial_clarity_fps_floor: expected >= size_fps_cap and <= 30")
	}
	if req.GIFRenderInitialSizeColorsCap < 16 || req.GIFRenderInitialSizeColorsCap > 256 {
		return errors.New("invalid gif_render_initial_size_colors_cap: expected 16..256")
	}
	if req.GIFRenderInitialClarityColorsFloor < req.GIFRenderInitialSizeColorsCap || req.GIFRenderInitialClarityColorsFloor > 256 {
		return errors.New("invalid gif_render_initial_clarity_colors_floor: expected >= size_colors_cap and <= 256")
	}
	if req.GIFMotionLowScoreThreshold < 0 || req.GIFMotionLowScoreThreshold > 0.95 {
		return errors.New("invalid gif_motion_low_score_threshold: expected 0..0.95")
	}
	if req.GIFMotionHighScoreThreshold <= req.GIFMotionLowScoreThreshold || req.GIFMotionHighScoreThreshold > 1 {
		return errors.New("invalid gif_motion_high_score_threshold: expected > low_score_threshold and <= 1")
	}
	if req.GIFMotionLowFPSDelta < -12 || req.GIFMotionLowFPSDelta > 0 {
		return errors.New("invalid gif_motion_low_fps_delta: expected -12..0")
	}
	if req.GIFMotionHighFPSDelta < 0 || req.GIFMotionHighFPSDelta > 12 {
		return errors.New("invalid gif_motion_high_fps_delta: expected 0..12")
	}
	if req.GIFAdaptiveFPSMin < 2 || req.GIFAdaptiveFPSMin > 30 {
		return errors.New("invalid gif_adaptive_fps_min: expected 2..30")
	}
	if req.GIFAdaptiveFPSMax < req.GIFAdaptiveFPSMin || req.GIFAdaptiveFPSMax > 60 {
		return errors.New("invalid gif_adaptive_fps_max: expected >= min and <= 60")
	}
	if req.GIFWidthSizeLow < 320 || req.GIFWidthSizeLow > 1920 {
		return errors.New("invalid gif_width_size_low: expected 320..1920")
	}
	if req.GIFWidthSizeMedium < req.GIFWidthSizeLow || req.GIFWidthSizeMedium > 1920 {
		return errors.New("invalid gif_width_size_medium: expected >= width_size_low and <= 1920")
	}
	if req.GIFWidthSizeHigh < req.GIFWidthSizeMedium || req.GIFWidthSizeHigh > 1920 {
		return errors.New("invalid gif_width_size_high: expected >= width_size_medium and <= 1920")
	}
	if req.GIFWidthClarityLow < 320 || req.GIFWidthClarityLow > 1920 {
		return errors.New("invalid gif_width_clarity_low: expected 320..1920")
	}
	if req.GIFWidthClarityMedium < req.GIFWidthClarityLow || req.GIFWidthClarityMedium > 1920 {
		return errors.New("invalid gif_width_clarity_medium: expected >= width_clarity_low and <= 1920")
	}
	if req.GIFWidthClarityHigh < req.GIFWidthClarityMedium || req.GIFWidthClarityHigh > 1920 {
		return errors.New("invalid gif_width_clarity_high: expected >= width_clarity_medium and <= 1920")
	}
	if req.GIFColorsSizeLow < 16 || req.GIFColorsSizeLow > 256 {
		return errors.New("invalid gif_colors_size_low: expected 16..256")
	}
	if req.GIFColorsSizeMedium < req.GIFColorsSizeLow || req.GIFColorsSizeMedium > 256 {
		return errors.New("invalid gif_colors_size_medium: expected >= colors_size_low and <= 256")
	}
	if req.GIFColorsSizeHigh < req.GIFColorsSizeMedium || req.GIFColorsSizeHigh > 256 {
		return errors.New("invalid gif_colors_size_high: expected >= colors_size_medium and <= 256")
	}
	if req.GIFColorsClarityLow < 16 || req.GIFColorsClarityLow > 256 {
		return errors.New("invalid gif_colors_clarity_low: expected 16..256")
	}
	if req.GIFColorsClarityMedium < req.GIFColorsClarityLow || req.GIFColorsClarityMedium > 256 {
		return errors.New("invalid gif_colors_clarity_medium: expected >= colors_clarity_low and <= 256")
	}
	if req.GIFColorsClarityHigh < req.GIFColorsClarityMedium || req.GIFColorsClarityHigh > 256 {
		return errors.New("invalid gif_colors_clarity_high: expected >= colors_clarity_medium and <= 256")
	}
	if req.GIFDurationLowSec < 0.8 || req.GIFDurationLowSec > 6.0 {
		return errors.New("invalid gif_duration_low_sec: expected 0.8..6.0")
	}
	if req.GIFDurationMediumSec < req.GIFDurationLowSec || req.GIFDurationMediumSec > 6.0 {
		return errors.New("invalid gif_duration_medium_sec: expected >= duration_low_sec and <= 6.0")
	}
	if req.GIFDurationHighSec < req.GIFDurationMediumSec || req.GIFDurationHighSec > 6.0 {
		return errors.New("invalid gif_duration_high_sec: expected >= duration_medium_sec and <= 6.0")
	}
	if req.GIFDurationSizeProfileMaxSec < req.GIFDurationLowSec || req.GIFDurationSizeProfileMaxSec > req.GIFDurationHighSec {
		return errors.New("invalid gif_duration_size_profile_max_sec: expected in [duration_low_sec,duration_high_sec]")
	}
	if req.GIFDownshiftHighResLongSideThreshold < 720 || req.GIFDownshiftHighResLongSideThreshold > 4096 {
		return errors.New("invalid gif_downshift_high_res_long_side_threshold: expected 720..4096")
	}
	if req.GIFDownshiftEarlyDurationSec < 10 || req.GIFDownshiftEarlyDurationSec > 300 {
		return errors.New("invalid gif_downshift_early_duration_sec: expected 10..300")
	}
	if req.GIFDownshiftEarlyLongSideThreshold < req.GIFDownshiftHighResLongSideThreshold || req.GIFDownshiftEarlyLongSideThreshold > 4096 {
		return errors.New("invalid gif_downshift_early_long_side_threshold: expected >= high_res_long_side_threshold and <= 4096")
	}
	if req.GIFDownshiftMediumFPSCap < 4 || req.GIFDownshiftMediumFPSCap > 30 {
		return errors.New("invalid gif_downshift_medium_fps_cap: expected 4..30")
	}
	if req.GIFDownshiftMediumWidthCap < 320 || req.GIFDownshiftMediumWidthCap > 1920 {
		return errors.New("invalid gif_downshift_medium_width_cap: expected 320..1920")
	}
	if req.GIFDownshiftMediumColorsCap < 16 || req.GIFDownshiftMediumColorsCap > 256 {
		return errors.New("invalid gif_downshift_medium_colors_cap: expected 16..256")
	}
	if req.GIFDownshiftMediumDurationCapSec < 1.0 || req.GIFDownshiftMediumDurationCapSec > 6.0 {
		return errors.New("invalid gif_downshift_medium_duration_cap_sec: expected 1.0..6.0")
	}
	if req.GIFDownshiftLongFPSCap < 4 || req.GIFDownshiftLongFPSCap > req.GIFDownshiftMediumFPSCap {
		return errors.New("invalid gif_downshift_long_fps_cap: expected 4..medium_fps_cap")
	}
	if req.GIFDownshiftLongWidthCap < 320 || req.GIFDownshiftLongWidthCap > req.GIFDownshiftMediumWidthCap {
		return errors.New("invalid gif_downshift_long_width_cap: expected 320..medium_width_cap")
	}
	if req.GIFDownshiftLongColorsCap < 16 || req.GIFDownshiftLongColorsCap > req.GIFDownshiftMediumColorsCap {
		return errors.New("invalid gif_downshift_long_colors_cap: expected 16..medium_colors_cap")
	}
	if req.GIFDownshiftLongDurationCapSec < 0.8 || req.GIFDownshiftLongDurationCapSec > req.GIFDownshiftMediumDurationCapSec {
		return errors.New("invalid gif_downshift_long_duration_cap_sec: expected 0.8..medium_duration_cap_sec")
	}
	if req.GIFDownshiftUltraFPSCap < 4 || req.GIFDownshiftUltraFPSCap > req.GIFDownshiftLongFPSCap {
		return errors.New("invalid gif_downshift_ultra_fps_cap: expected 4..long_fps_cap")
	}
	if req.GIFDownshiftUltraWidthCap < 320 || req.GIFDownshiftUltraWidthCap > req.GIFDownshiftLongWidthCap {
		return errors.New("invalid gif_downshift_ultra_width_cap: expected 320..long_width_cap")
	}
	if req.GIFDownshiftUltraColorsCap < 16 || req.GIFDownshiftUltraColorsCap > req.GIFDownshiftLongColorsCap {
		return errors.New("invalid gif_downshift_ultra_colors_cap: expected 16..long_colors_cap")
	}
	if req.GIFDownshiftUltraDurationCapSec < 0.8 || req.GIFDownshiftUltraDurationCapSec > req.GIFDownshiftLongDurationCapSec {
		return errors.New("invalid gif_downshift_ultra_duration_cap_sec: expected 0.8..long_duration_cap_sec")
	}
	if req.GIFDownshiftHighResFPSCap < 4 || req.GIFDownshiftHighResFPSCap > req.GIFDownshiftMediumFPSCap {
		return errors.New("invalid gif_downshift_high_res_fps_cap: expected 4..medium_fps_cap")
	}
	if req.GIFDownshiftHighResWidthCap < 320 || req.GIFDownshiftHighResWidthCap > req.GIFDownshiftMediumWidthCap {
		return errors.New("invalid gif_downshift_high_res_width_cap: expected 320..medium_width_cap")
	}
	if req.GIFDownshiftHighResColorsCap < 16 || req.GIFDownshiftHighResColorsCap > req.GIFDownshiftMediumColorsCap {
		return errors.New("invalid gif_downshift_high_res_colors_cap: expected 16..medium_colors_cap")
	}
	if req.GIFDownshiftHighResDurationCapSec < 0.8 || req.GIFDownshiftHighResDurationCapSec > req.GIFDownshiftMediumDurationCapSec {
		return errors.New("invalid gif_downshift_high_res_duration_cap_sec: expected 0.8..medium_duration_cap_sec")
	}
	if req.GIFTimeoutFallbackFPSCap < 4 || req.GIFTimeoutFallbackFPSCap > 30 {
		return errors.New("invalid gif_timeout_fallback_fps_cap: expected 4..30")
	}
	if req.GIFTimeoutFallbackWidthCap < 320 || req.GIFTimeoutFallbackWidthCap > 1920 {
		return errors.New("invalid gif_timeout_fallback_width_cap: expected 320..1920")
	}
	if req.GIFTimeoutFallbackColorsCap < 16 || req.GIFTimeoutFallbackColorsCap > 256 {
		return errors.New("invalid gif_timeout_fallback_colors_cap: expected 16..256")
	}
	if req.GIFTimeoutFallbackMinWidth < 240 || req.GIFTimeoutFallbackMinWidth > req.GIFTimeoutFallbackWidthCap {
		return errors.New("invalid gif_timeout_fallback_min_width: expected 240..fallback_width_cap")
	}
	if req.GIFTimeoutFallbackUltraFPSCap < 4 || req.GIFTimeoutFallbackUltraFPSCap > req.GIFTimeoutFallbackFPSCap {
		return errors.New("invalid gif_timeout_fallback_ultra_fps_cap: expected 4..fallback_fps_cap")
	}
	if req.GIFTimeoutFallbackUltraWidthCap < req.GIFTimeoutFallbackMinWidth || req.GIFTimeoutFallbackUltraWidthCap > req.GIFTimeoutFallbackWidthCap {
		return errors.New("invalid gif_timeout_fallback_ultra_width_cap: expected [fallback_min_width,fallback_width_cap]")
	}
	if req.GIFTimeoutFallbackUltraColorsCap < 16 || req.GIFTimeoutFallbackUltraColorsCap > req.GIFTimeoutFallbackColorsCap {
		return errors.New("invalid gif_timeout_fallback_ultra_colors_cap: expected 16..fallback_colors_cap")
	}
	if req.GIFTimeoutEmergencyFPSCap < 4 || req.GIFTimeoutEmergencyFPSCap > req.GIFTimeoutFallbackFPSCap {
		return errors.New("invalid gif_timeout_emergency_fps_cap: expected 4..fallback_fps_cap")
	}
	if req.GIFTimeoutEmergencyWidthCap < 240 || req.GIFTimeoutEmergencyWidthCap > req.GIFTimeoutFallbackWidthCap {
		return errors.New("invalid gif_timeout_emergency_width_cap: expected 240..fallback_width_cap")
	}
	if req.GIFTimeoutEmergencyColorsCap < 16 || req.GIFTimeoutEmergencyColorsCap > req.GIFTimeoutFallbackColorsCap {
		return errors.New("invalid gif_timeout_emergency_colors_cap: expected 16..fallback_colors_cap")
	}
	if req.GIFTimeoutEmergencyMinWidth < 240 || req.GIFTimeoutEmergencyMinWidth > req.GIFTimeoutEmergencyWidthCap {
		return errors.New("invalid gif_timeout_emergency_min_width: expected 240..emergency_width_cap")
	}
	if req.GIFTimeoutEmergencyDurationTrigger < 1.0 || req.GIFTimeoutEmergencyDurationTrigger > 6.0 {
		return errors.New("invalid gif_timeout_emergency_duration_trigger_sec: expected 1.0..6.0")
	}
	if req.GIFTimeoutEmergencyDurationScale < 0.5 || req.GIFTimeoutEmergencyDurationScale > 1.0 {
		return errors.New("invalid gif_timeout_emergency_duration_scale: expected 0.5..1.0")
	}
	if req.GIFTimeoutEmergencyDurationMinSec < 0.8 || req.GIFTimeoutEmergencyDurationMinSec > 4.0 {
		return errors.New("invalid gif_timeout_emergency_duration_min_sec: expected 0.8..4.0")
	}
	if req.GIFTimeoutLastResortFPSCap < 4 || req.GIFTimeoutLastResortFPSCap > req.GIFTimeoutEmergencyFPSCap {
		return errors.New("invalid gif_timeout_last_resort_fps_cap: expected 4..emergency_fps_cap")
	}
	if req.GIFTimeoutLastResortWidthCap < 240 || req.GIFTimeoutLastResortWidthCap > req.GIFTimeoutEmergencyWidthCap {
		return errors.New("invalid gif_timeout_last_resort_width_cap: expected 240..emergency_width_cap")
	}
	if req.GIFTimeoutLastResortColorsCap < 16 || req.GIFTimeoutLastResortColorsCap > req.GIFTimeoutEmergencyColorsCap {
		return errors.New("invalid gif_timeout_last_resort_colors_cap: expected 16..emergency_colors_cap")
	}
	if req.GIFTimeoutLastResortMinWidth < 240 || req.GIFTimeoutLastResortMinWidth > req.GIFTimeoutLastResortWidthCap {
		return errors.New("invalid gif_timeout_last_resort_min_width: expected 240..last_resort_width_cap")
	}
	if req.GIFTimeoutLastResortDurationMinSec < 0.6 || req.GIFTimeoutLastResortDurationMinSec > 3.0 {
		return errors.New("invalid gif_timeout_last_resort_duration_min_sec: expected 0.6..3.0")
	}
	if req.GIFTimeoutLastResortDurationMaxSec < req.GIFTimeoutLastResortDurationMinSec || req.GIFTimeoutLastResortDurationMaxSec > 4.0 {
		return errors.New("invalid gif_timeout_last_resort_duration_max_sec: expected >= min and <= 4.0")
	}
	if req.GIFLoopTuneMinEnableSec <= 0 || req.GIFLoopTuneMinImprovement < 0 {
		return errors.New("invalid gif loop tune settings: min_enable_sec must be >0 and min_improvement must be >=0")
	}
	if req.GIFLoopTuneMotionTarget <= 0 || req.GIFLoopTuneMotionTarget > 1 {
		return errors.New("invalid gif_loop_tune_motion_target: expected (0,1]")
	}
	if req.GIFLoopTunePreferDuration <= 0 {
		return errors.New("invalid gif_loop_tune_prefer_duration_sec: expected > 0")
	}
	if req.GIFLoopTuneMinEnableSec > req.GIFLoopTunePreferDuration {
		return errors.New("invalid gif loop tune duration: gif_loop_tune_min_enable_sec must be <= gif_loop_tune_prefer_duration_sec")
	}
	if req.StillMinExposureScore < 0 || req.StillMinExposureScore > 1 {
		return errors.New("invalid still_min_exposure_score: expected 0..1")
	}
	if req.StillMinWidth < 0 || req.StillMinHeight < 0 {
		return errors.New("invalid still size threshold: expected still_min_width/height >= 0")
	}
	if req.LiveCoverPortraitWeight < 0 || req.LiveCoverPortraitWeight > 1 {
		return errors.New("invalid live_cover_portrait_weight: expected 0..1")
	}
	if req.LiveCoverSceneMinSamples < 1 || req.LiveCoverGuardMinTotal < 1 {
		return errors.New("invalid live cover guard threshold: expected >= 1")
	}
	if req.LiveCoverGuardMinTotal < req.LiveCoverSceneMinSamples {
		return errors.New("invalid live cover guard threshold: live_cover_guard_min_total must be >= live_cover_scene_min_samples")
	}
	if req.LiveCoverGuardScoreFloor <= 0 || req.LiveCoverGuardScoreFloor >= 1 {
		return errors.New("invalid live_cover_guard_score_floor: expected (0,1)")
	}

	allowedProfile := map[string]struct{}{
		"clarity": {},
		"size":    {},
	}
	for _, profile := range []string{req.GIFProfile, req.WebPProfile, req.LiveProfile, req.JPGProfile, req.PNGProfile} {
		clean := strings.ToLower(strings.TrimSpace(profile))
		if _, ok := allowedProfile[clean]; !ok {
			return fmt.Errorf("invalid profile value: %q (expected clarity/size)", profile)
		}
	}
	allowedDither := map[string]struct{}{
		"sierra2_4a":      {},
		"floyd_steinberg": {},
		"bayer":           {},
		"none":            {},
	}
	if _, ok := allowedDither[strings.ToLower(strings.TrimSpace(req.GIFDitherMode))]; !ok {
		return fmt.Errorf("invalid gif_dither_mode: %q", req.GIFDitherMode)
	}

	if req.HighlightFeedbackRollout < 0 || req.HighlightFeedbackRollout > 100 {
		return errors.New("invalid highlight_feedback_rollout_percent: expected 0..100")
	}
	if req.HighlightFeedbackMinJobs < 1 {
		return errors.New("invalid highlight_feedback_min_engaged_jobs: expected >= 1")
	}
	if req.HighlightFeedbackMinScore <= 0 {
		return errors.New("invalid highlight_feedback_min_weighted_signals: expected > 0")
	}
	if req.HighlightFeedbackBoost < 0 {
		return errors.New("invalid highlight_feedback_boost_scale: expected >= 0")
	}
	if req.HighlightWeightPosition < 0 || req.HighlightWeightDuration < 0 || req.HighlightWeightReason < 0 {
		return errors.New("invalid highlight feedback weights: each weight must be >= 0")
	}
	if req.HighlightWeightPosition+req.HighlightWeightDuration+req.HighlightWeightReason <= 0 {
		return errors.New("invalid highlight feedback weights: total weight must be > 0")
	}
	if req.HighlightNegativeGuardThreshold <= 0 || req.HighlightNegativeGuardThreshold >= 1 {
		return errors.New("invalid highlight_feedback_negative_guard_dominance_threshold: expected (0,1)")
	}
	if req.HighlightNegativeGuardMinWeight <= 0 {
		return errors.New("invalid highlight_feedback_negative_guard_min_weight: expected > 0")
	}
	if req.HighlightNegativePenaltyScale < 0 || req.HighlightNegativePenaltyScale > 1 {
		return errors.New("invalid highlight_feedback_negative_guard_penalty_scale: expected 0..1")
	}
	if req.HighlightNegativePenaltyWeight < 0 || req.HighlightNegativePenaltyWeight > 2 {
		return errors.New("invalid highlight_feedback_negative_guard_penalty_weight: expected 0..2")
	}
	switch strings.ToLower(strings.TrimSpace(req.AIDirectorInputMode)) {
	case "", "frames", "full_video", "hybrid":
	default:
		return errors.New("invalid ai_director_input_mode: expected frames|full_video|hybrid")
	}
	if len(strings.TrimSpace(req.AIDirectorOperatorInstructionVersion)) == 0 {
		return errors.New("invalid ai_director_operator_instruction_version: expected non-empty")
	}
	if len(strings.TrimSpace(req.AIDirectorOperatorInstructionVersion)) > 64 {
		return errors.New("invalid ai_director_operator_instruction_version: expected <= 64 chars")
	}
	if len(strings.TrimSpace(req.AIDirectorOperatorInstruction)) > 4000 {
		return errors.New("invalid ai_director_operator_instruction: expected <= 4000 chars")
	}
	if req.AIDirectorCountExpandRatio < 0 || req.AIDirectorCountExpandRatio > 3 {
		return errors.New("invalid ai_director_count_expand_ratio: expected 0..3")
	}
	if req.AIDirectorDurationExpandRatio < 0 || req.AIDirectorDurationExpandRatio > 3 {
		return errors.New("invalid ai_director_duration_expand_ratio: expected 0..3")
	}
	if req.AIDirectorCountAbsoluteCap < 1 || req.AIDirectorCountAbsoluteCap > videojobs.DefaultQualitySettings().AIDirectorCountAbsoluteCap*2 {
		return errors.New("invalid ai_director_count_absolute_cap: expected 1..20")
	}
	if req.AIDirectorDurationAbsoluteCapSec < 2 || req.AIDirectorDurationAbsoluteCapSec > 12 {
		return errors.New("invalid ai_director_duration_absolute_cap_sec: expected 2..12")
	}
	if req.AIDirectorCountAbsoluteCap < req.GIFCandidateMaxOutputs {
		return errors.New("invalid ai_director_count_absolute_cap: expected >= gif_candidate_max_outputs")
	}
	if req.GIFAIJudgeHardGateMinOverallScore <= 0 || req.GIFAIJudgeHardGateMinOverallScore > 1 {
		return errors.New("invalid gif_ai_judge_hard_gate_min_overall_score: expected (0,1]")
	}
	if req.GIFAIJudgeHardGateMinClarityScore <= 0 || req.GIFAIJudgeHardGateMinClarityScore > 1 {
		return errors.New("invalid gif_ai_judge_hard_gate_min_clarity_score: expected (0,1]")
	}
	if req.GIFAIJudgeHardGateMinLoopScore <= 0 || req.GIFAIJudgeHardGateMinLoopScore > 1 {
		return errors.New("invalid gif_ai_judge_hard_gate_min_loop_score: expected (0,1]")
	}
	if req.GIFAIJudgeHardGateMinOutputScore <= 0 || req.GIFAIJudgeHardGateMinOutputScore > 1 {
		return errors.New("invalid gif_ai_judge_hard_gate_min_output_score: expected (0,1]")
	}
	if req.GIFAIJudgeHardGateMinDurationMS < 50 || req.GIFAIJudgeHardGateMinDurationMS > 10000 {
		return errors.New("invalid gif_ai_judge_hard_gate_min_duration_ms: expected 50..10000")
	}
	if req.GIFAIJudgeHardGateSizeMultiplier < 1 || req.GIFAIJudgeHardGateSizeMultiplier > 20 {
		return errors.New("invalid gif_ai_judge_hard_gate_size_multiplier: expected 1..20")
	}

	if err := validateLowRateThreshold("gif_health_done_rate", req.GIFHealthDoneRateWarn, req.GIFHealthDoneRateCritical); err != nil {
		return err
	}
	if err := validateHighRateThreshold("gif_health_failed_rate", req.GIFHealthFailedRateWarn, req.GIFHealthFailedRateCritical); err != nil {
		return err
	}
	if err := validateLowRateThreshold("gif_health_path_strict_rate", req.GIFHealthPathStrictRateWarn, req.GIFHealthPathStrictRateCritical); err != nil {
		return err
	}
	if err := validateHighRateThreshold("gif_health_loop_fallback_rate", req.GIFHealthLoopFallbackRateWarn, req.GIFHealthLoopFallbackRateCritical); err != nil {
		return err
	}
	if err := validateLowRateThreshold(
		"feedback_integrity_output_coverage_rate",
		req.FeedbackIntegrityOutputCoverageRateWarn,
		req.FeedbackIntegrityOutputCoverageRateCritical,
	); err != nil {
		return err
	}
	if err := validateLowRateThreshold(
		"feedback_integrity_output_resolved_rate",
		req.FeedbackIntegrityOutputResolvedRateWarn,
		req.FeedbackIntegrityOutputResolvedRateCritical,
	); err != nil {
		return err
	}
	if err := validateLowRateThreshold(
		"feedback_integrity_output_job_consistency_rate",
		req.FeedbackIntegrityOutputJobConsistencyRateWarn,
		req.FeedbackIntegrityOutputJobConsistencyRateCritical,
	); err != nil {
		return err
	}
	if err := validateHighCountThreshold(
		"feedback_integrity_top_pick_conflict_users",
		req.FeedbackIntegrityTopPickConflictUsersWarn,
		req.FeedbackIntegrityTopPickConflictUsersCritical,
	); err != nil {
		return err
	}

	return nil
}

func validateLowRateThreshold(name string, warn, critical float64) error {
	if warn <= 0 || warn >= 1 || critical <= 0 || critical >= 1 {
		return fmt.Errorf("invalid %s thresholds: expected both warn/critical in (0,1)", name)
	}
	if critical >= warn {
		return fmt.Errorf("invalid %s thresholds: critical must be lower than warn", name)
	}
	return nil
}

func isValidGIFPipelineMode(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "light", "standard", "hq":
		return true
	default:
		return false
	}
}

func validateHighRateThreshold(name string, warn, critical float64) error {
	if warn <= 0 || warn >= 1 || critical <= 0 || critical >= 1 {
		return fmt.Errorf("invalid %s thresholds: expected both warn/critical in (0,1)", name)
	}
	if critical <= warn {
		return fmt.Errorf("invalid %s thresholds: critical must be higher than warn", name)
	}
	return nil
}

func validateHighCountThreshold(name string, warn, critical int) error {
	if warn < 0 || critical < 0 {
		return fmt.Errorf("invalid %s thresholds: expected warn/critical >= 0", name)
	}
	if critical <= warn {
		return fmt.Errorf("invalid %s thresholds: critical must be higher than warn", name)
	}
	return nil
}

func qualitySettingsFromModel(setting models.VideoQualitySetting) videojobs.QualitySettings {
	return videojobs.NormalizeQualitySettings(videojobs.QualitySettings{
		MinBrightness:                        setting.MinBrightness,
		MaxBrightness:                        setting.MaxBrightness,
		BlurThresholdFactor:                  setting.BlurThresholdFactor,
		BlurThresholdMin:                     setting.BlurThresholdMin,
		BlurThresholdMax:                     setting.BlurThresholdMax,
		DuplicateHammingThreshold:            setting.DuplicateHammingThreshold,
		DuplicateBacktrackFrames:             setting.DuplicateBacktrackFrames,
		FallbackBlurRelaxFactor:              setting.FallbackBlurRelaxFactor,
		FallbackHammingThreshold:             setting.FallbackHammingThreshold,
		MinKeepBase:                          setting.MinKeepBase,
		MinKeepRatio:                         setting.MinKeepRatio,
		QualityAnalysisWorkers:               setting.QualityAnalysisWorkers,
		UploadConcurrency:                    setting.UploadConcurrency,
		GIFProfile:                           setting.GIFProfile,
		WebPProfile:                          setting.WebPProfile,
		LiveProfile:                          setting.LiveProfile,
		JPGProfile:                           setting.JPGProfile,
		PNGProfile:                           setting.PNGProfile,
		GIFDefaultFPS:                        setting.GIFDefaultFPS,
		GIFDefaultMaxColors:                  setting.GIFDefaultMaxColors,
		GIFDitherMode:                        setting.GIFDitherMode,
		GIFTargetSizeKB:                      setting.GIFTargetSizeKB,
		GIFGifsicleEnabled:                   setting.GIFGifsicleEnabled,
		GIFGifsicleLevel:                     setting.GIFGifsicleLevel,
		GIFGifsicleSkipBelowKB:               setting.GIFGifsicleSkipBelowKB,
		GIFGifsicleMinGainRatio:              setting.GIFGifsicleMinGainRatio,
		GIFLoopTuneEnabled:                   setting.GIFLoopTuneEnabled,
		GIFLoopTuneMinEnableSec:              setting.GIFLoopTuneMinEnableSec,
		GIFLoopTuneMinImprovement:            setting.GIFLoopTuneMinImprovement,
		GIFLoopTuneMotionTarget:              setting.GIFLoopTuneMotionTarget,
		GIFLoopTunePreferDuration:            setting.GIFLoopTunePreferDuration,
		GIFCandidateMaxOutputs:               setting.GIFCandidateMaxOutputs,
		GIFCandidateLongVideoMaxOutputs:      setting.GIFCandidateLongVideoMaxOutputs,
		GIFCandidateUltraVideoMaxOutputs:     setting.GIFCandidateUltraVideoMaxOutputs,
		GIFCandidateConfidenceThreshold:      setting.GIFCandidateConfidenceThreshold,
		GIFCandidateDedupIOUThreshold:        setting.GIFCandidateDedupIOUThreshold,
		GIFRenderBudgetNormalMultiplier:      setting.GIFRenderBudgetNormalMultiplier,
		GIFRenderBudgetLongMultiplier:        setting.GIFRenderBudgetLongMultiplier,
		GIFRenderBudgetUltraMultiplier:       setting.GIFRenderBudgetUltraMultiplier,
		GIFPipelineShortVideoMaxSec:          setting.GIFPipelineShortVideoMaxSec,
		GIFPipelineLongVideoMinSec:           setting.GIFPipelineLongVideoMinSec,
		GIFPipelineShortVideoMode:            setting.GIFPipelineShortVideoMode,
		GIFPipelineDefaultMode:               setting.GIFPipelineDefaultMode,
		GIFPipelineLongVideoMode:             setting.GIFPipelineLongVideoMode,
		GIFPipelineHighPriorityEnabled:       setting.GIFPipelineHighPriorityEnabled,
		GIFPipelineHighPriorityMode:          setting.GIFPipelineHighPriorityMode,
		GIFDurationTierMediumSec:             setting.GIFDurationTierMediumSec,
		GIFDurationTierLongSec:               setting.GIFDurationTierLongSec,
		GIFDurationTierUltraSec:              setting.GIFDurationTierUltraSec,
		GIFSegmentTimeoutMinSec:              setting.GIFSegmentTimeoutMinSec,
		GIFSegmentTimeoutMaxSec:              setting.GIFSegmentTimeoutMaxSec,
		GIFSegmentTimeoutFallbackCapSec:      setting.GIFSegmentTimeoutFallbackCapSec,
		GIFSegmentTimeoutEmergencyCapSec:     setting.GIFSegmentTimeoutEmergencyCapSec,
		GIFSegmentTimeoutLastResortCapSec:    setting.GIFSegmentTimeoutLastResortCapSec,
		GIFRenderRetryMaxAttempts:            setting.GIFRenderRetryMaxAttempts,
		GIFRenderRetryPrimaryColorsFloor:     setting.GIFRenderRetryPrimaryColorsFloor,
		GIFRenderRetryPrimaryColorsStep:      setting.GIFRenderRetryPrimaryColorsStep,
		GIFRenderRetryFPSFloor:               setting.GIFRenderRetryFPSFloor,
		GIFRenderRetryFPSStep:                setting.GIFRenderRetryFPSStep,
		GIFRenderRetryWidthTrigger:           setting.GIFRenderRetryWidthTrigger,
		GIFRenderRetryWidthScale:             setting.GIFRenderRetryWidthScale,
		GIFRenderRetryWidthFloor:             setting.GIFRenderRetryWidthFloor,
		GIFRenderRetrySecondaryColorsFloor:   setting.GIFRenderRetrySecondaryColorsFloor,
		GIFRenderRetrySecondaryColorsStep:    setting.GIFRenderRetrySecondaryColorsStep,
		GIFRenderInitialSizeFPSCap:           setting.GIFRenderInitialSizeFPSCap,
		GIFRenderInitialClarityFPSFloor:      setting.GIFRenderInitialClarityFPSFloor,
		GIFRenderInitialSizeColorsCap:        setting.GIFRenderInitialSizeColorsCap,
		GIFRenderInitialClarityColorsFloor:   setting.GIFRenderInitialClarityColorsFloor,
		GIFMotionLowScoreThreshold:           setting.GIFMotionLowScoreThreshold,
		GIFMotionHighScoreThreshold:          setting.GIFMotionHighScoreThreshold,
		GIFMotionLowFPSDelta:                 setting.GIFMotionLowFPSDelta,
		GIFMotionHighFPSDelta:                setting.GIFMotionHighFPSDelta,
		GIFAdaptiveFPSMin:                    setting.GIFAdaptiveFPSMin,
		GIFAdaptiveFPSMax:                    setting.GIFAdaptiveFPSMax,
		GIFWidthSizeLow:                      setting.GIFWidthSizeLow,
		GIFWidthSizeMedium:                   setting.GIFWidthSizeMedium,
		GIFWidthSizeHigh:                     setting.GIFWidthSizeHigh,
		GIFWidthClarityLow:                   setting.GIFWidthClarityLow,
		GIFWidthClarityMedium:                setting.GIFWidthClarityMedium,
		GIFWidthClarityHigh:                  setting.GIFWidthClarityHigh,
		GIFColorsSizeLow:                     setting.GIFColorsSizeLow,
		GIFColorsSizeMedium:                  setting.GIFColorsSizeMedium,
		GIFColorsSizeHigh:                    setting.GIFColorsSizeHigh,
		GIFColorsClarityLow:                  setting.GIFColorsClarityLow,
		GIFColorsClarityMedium:               setting.GIFColorsClarityMedium,
		GIFColorsClarityHigh:                 setting.GIFColorsClarityHigh,
		GIFDurationLowSec:                    setting.GIFDurationLowSec,
		GIFDurationMediumSec:                 setting.GIFDurationMediumSec,
		GIFDurationHighSec:                   setting.GIFDurationHighSec,
		GIFDurationSizeProfileMaxSec:         setting.GIFDurationSizeProfileMaxSec,
		GIFDownshiftHighResLongSideThreshold: setting.GIFDownshiftHighResLongSideThreshold,
		GIFDownshiftEarlyDurationSec:         setting.GIFDownshiftEarlyDurationSec,
		GIFDownshiftEarlyLongSideThreshold:   setting.GIFDownshiftEarlyLongSideThreshold,
		GIFDownshiftMediumFPSCap:             setting.GIFDownshiftMediumFPSCap,
		GIFDownshiftMediumWidthCap:           setting.GIFDownshiftMediumWidthCap,
		GIFDownshiftMediumColorsCap:          setting.GIFDownshiftMediumColorsCap,
		GIFDownshiftMediumDurationCapSec:     setting.GIFDownshiftMediumDurationCapSec,
		GIFDownshiftLongFPSCap:               setting.GIFDownshiftLongFPSCap,
		GIFDownshiftLongWidthCap:             setting.GIFDownshiftLongWidthCap,
		GIFDownshiftLongColorsCap:            setting.GIFDownshiftLongColorsCap,
		GIFDownshiftLongDurationCapSec:       setting.GIFDownshiftLongDurationCapSec,
		GIFDownshiftUltraFPSCap:              setting.GIFDownshiftUltraFPSCap,
		GIFDownshiftUltraWidthCap:            setting.GIFDownshiftUltraWidthCap,
		GIFDownshiftUltraColorsCap:           setting.GIFDownshiftUltraColorsCap,
		GIFDownshiftUltraDurationCapSec:      setting.GIFDownshiftUltraDurationCapSec,
		GIFDownshiftHighResFPSCap:            setting.GIFDownshiftHighResFPSCap,
		GIFDownshiftHighResWidthCap:          setting.GIFDownshiftHighResWidthCap,
		GIFDownshiftHighResColorsCap:         setting.GIFDownshiftHighResColorsCap,
		GIFDownshiftHighResDurationCapSec:    setting.GIFDownshiftHighResDurationCapSec,
		GIFTimeoutFallbackFPSCap:             setting.GIFTimeoutFallbackFPSCap,
		GIFTimeoutFallbackWidthCap:           setting.GIFTimeoutFallbackWidthCap,
		GIFTimeoutFallbackColorsCap:          setting.GIFTimeoutFallbackColorsCap,
		GIFTimeoutFallbackMinWidth:           setting.GIFTimeoutFallbackMinWidth,
		GIFTimeoutFallbackUltraFPSCap:        setting.GIFTimeoutFallbackUltraFPSCap,
		GIFTimeoutFallbackUltraWidthCap:      setting.GIFTimeoutFallbackUltraWidthCap,
		GIFTimeoutFallbackUltraColorsCap:     setting.GIFTimeoutFallbackUltraColorsCap,
		GIFTimeoutEmergencyFPSCap:            setting.GIFTimeoutEmergencyFPSCap,
		GIFTimeoutEmergencyWidthCap:          setting.GIFTimeoutEmergencyWidthCap,
		GIFTimeoutEmergencyColorsCap:         setting.GIFTimeoutEmergencyColorsCap,
		GIFTimeoutEmergencyMinWidth:          setting.GIFTimeoutEmergencyMinWidth,
		GIFTimeoutEmergencyDurationTrigger:   setting.GIFTimeoutEmergencyDurationTrigger,
		GIFTimeoutEmergencyDurationScale:     setting.GIFTimeoutEmergencyDurationScale,
		GIFTimeoutEmergencyDurationMinSec:    setting.GIFTimeoutEmergencyDurationMinSec,
		GIFTimeoutLastResortFPSCap:           setting.GIFTimeoutLastResortFPSCap,
		GIFTimeoutLastResortWidthCap:         setting.GIFTimeoutLastResortWidthCap,
		GIFTimeoutLastResortColorsCap:        setting.GIFTimeoutLastResortColorsCap,
		GIFTimeoutLastResortMinWidth:         setting.GIFTimeoutLastResortMinWidth,
		GIFTimeoutLastResortDurationMinSec:   setting.GIFTimeoutLastResortDurationMinSec,
		GIFTimeoutLastResortDurationMaxSec:   setting.GIFTimeoutLastResortDurationMaxSec,
		WebPTargetSizeKB:                     setting.WebPTargetSizeKB,
		JPGTargetSizeKB:                      setting.JPGTargetSizeKB,
		PNGTargetSizeKB:                      setting.PNGTargetSizeKB,
		StillMinBlurScore:                    setting.StillMinBlurScore,
		StillMinExposureScore:                setting.StillMinExposureScore,
		StillMinWidth:                        setting.StillMinWidth,
		StillMinHeight:                       setting.StillMinHeight,
		LiveCoverPortraitWeight:              setting.LiveCoverPortraitWeight,
		LiveCoverSceneMinSamples:             setting.LiveCoverSceneMinSamples,
		LiveCoverGuardMinTotal:               setting.LiveCoverGuardMinTotal,
		LiveCoverGuardScoreFloor:             setting.LiveCoverGuardScoreFloor,
		HighlightFeedbackEnabled:             setting.HighlightFeedbackEnabled,
		HighlightFeedbackRollout:             setting.HighlightFeedbackRollout,
		HighlightFeedbackMinJobs:             setting.HighlightFeedbackMinJobs,
		HighlightFeedbackMinScore:            setting.HighlightFeedbackMinScore,
		HighlightFeedbackBoost:               setting.HighlightFeedbackBoost,
		HighlightWeightPosition:              setting.HighlightWeightPosition,
		HighlightWeightDuration:              setting.HighlightWeightDuration,
		HighlightWeightReason:                setting.HighlightWeightReason,
		HighlightNegativeGuardEnabled:        setting.HighlightNegativeGuardEnabled,
		HighlightNegativeGuardThreshold:      setting.HighlightNegativeGuardThreshold,
		HighlightNegativeGuardMinWeight:      setting.HighlightNegativeGuardMinWeight,
		HighlightNegativePenaltyScale:        setting.HighlightNegativePenaltyScale,
		HighlightNegativePenaltyWeight:       setting.HighlightNegativePenaltyWeight,
		AIDirectorInputMode:                  setting.AIDirectorInputMode,
		AIDirectorOperatorInstruction:        setting.AIDirectorOperatorInstruction,
		AIDirectorOperatorInstructionVersion: setting.AIDirectorOperatorInstructionVersion,
		AIDirectorOperatorEnabled:            setting.AIDirectorOperatorEnabled,
		AIDirectorConstraintOverrideEnabled:  setting.AIDirectorConstraintOverrideEnabled,
		AIDirectorCountExpandRatio:           setting.AIDirectorCountExpandRatio,
		AIDirectorDurationExpandRatio:        setting.AIDirectorDurationExpandRatio,
		AIDirectorCountAbsoluteCap:           setting.AIDirectorCountAbsoluteCap,
		AIDirectorDurationAbsoluteCapSec:     setting.AIDirectorDurationAbsoluteCapSec,
		GIFAIJudgeHardGateMinOverallScore:    setting.GIFAIJudgeHardGateMinOverallScore,
		GIFAIJudgeHardGateMinClarityScore:    setting.GIFAIJudgeHardGateMinClarityScore,
		GIFAIJudgeHardGateMinLoopScore:       setting.GIFAIJudgeHardGateMinLoopScore,
		GIFAIJudgeHardGateMinOutputScore:     setting.GIFAIJudgeHardGateMinOutputScore,
		GIFAIJudgeHardGateMinDurationMS:      setting.GIFAIJudgeHardGateMinDurationMS,
		GIFAIJudgeHardGateSizeMultiplier:     setting.GIFAIJudgeHardGateSizeMultiplier,
	})
}

func applyQualitySettingsToModel(dst *models.VideoQualitySetting, settings videojobs.QualitySettings) {
	if dst == nil {
		return
	}
	dst.ID = 1
	dst.MinBrightness = settings.MinBrightness
	dst.MaxBrightness = settings.MaxBrightness
	dst.BlurThresholdFactor = settings.BlurThresholdFactor
	dst.BlurThresholdMin = settings.BlurThresholdMin
	dst.BlurThresholdMax = settings.BlurThresholdMax
	dst.DuplicateHammingThreshold = settings.DuplicateHammingThreshold
	dst.DuplicateBacktrackFrames = settings.DuplicateBacktrackFrames
	dst.FallbackBlurRelaxFactor = settings.FallbackBlurRelaxFactor
	dst.FallbackHammingThreshold = settings.FallbackHammingThreshold
	dst.MinKeepBase = settings.MinKeepBase
	dst.MinKeepRatio = settings.MinKeepRatio
	dst.QualityAnalysisWorkers = settings.QualityAnalysisWorkers
	dst.UploadConcurrency = settings.UploadConcurrency
	dst.GIFProfile = strings.TrimSpace(settings.GIFProfile)
	dst.WebPProfile = strings.TrimSpace(settings.WebPProfile)
	dst.LiveProfile = strings.TrimSpace(settings.LiveProfile)
	dst.JPGProfile = strings.TrimSpace(settings.JPGProfile)
	dst.PNGProfile = strings.TrimSpace(settings.PNGProfile)
	dst.GIFDefaultFPS = settings.GIFDefaultFPS
	dst.GIFDefaultMaxColors = settings.GIFDefaultMaxColors
	dst.GIFDitherMode = strings.TrimSpace(settings.GIFDitherMode)
	dst.GIFTargetSizeKB = settings.GIFTargetSizeKB
	dst.GIFGifsicleEnabled = settings.GIFGifsicleEnabled
	dst.GIFGifsicleLevel = settings.GIFGifsicleLevel
	dst.GIFGifsicleSkipBelowKB = settings.GIFGifsicleSkipBelowKB
	dst.GIFGifsicleMinGainRatio = settings.GIFGifsicleMinGainRatio
	dst.GIFLoopTuneEnabled = settings.GIFLoopTuneEnabled
	dst.GIFLoopTuneMinEnableSec = settings.GIFLoopTuneMinEnableSec
	dst.GIFLoopTuneMinImprovement = settings.GIFLoopTuneMinImprovement
	dst.GIFLoopTuneMotionTarget = settings.GIFLoopTuneMotionTarget
	dst.GIFLoopTunePreferDuration = settings.GIFLoopTunePreferDuration
	dst.GIFCandidateMaxOutputs = settings.GIFCandidateMaxOutputs
	dst.GIFCandidateLongVideoMaxOutputs = settings.GIFCandidateLongVideoMaxOutputs
	dst.GIFCandidateUltraVideoMaxOutputs = settings.GIFCandidateUltraVideoMaxOutputs
	dst.GIFCandidateConfidenceThreshold = settings.GIFCandidateConfidenceThreshold
	dst.GIFCandidateDedupIOUThreshold = settings.GIFCandidateDedupIOUThreshold
	dst.GIFRenderBudgetNormalMultiplier = settings.GIFRenderBudgetNormalMultiplier
	dst.GIFRenderBudgetLongMultiplier = settings.GIFRenderBudgetLongMultiplier
	dst.GIFRenderBudgetUltraMultiplier = settings.GIFRenderBudgetUltraMultiplier
	dst.GIFPipelineShortVideoMaxSec = settings.GIFPipelineShortVideoMaxSec
	dst.GIFPipelineLongVideoMinSec = settings.GIFPipelineLongVideoMinSec
	dst.GIFPipelineShortVideoMode = strings.TrimSpace(settings.GIFPipelineShortVideoMode)
	dst.GIFPipelineDefaultMode = strings.TrimSpace(settings.GIFPipelineDefaultMode)
	dst.GIFPipelineLongVideoMode = strings.TrimSpace(settings.GIFPipelineLongVideoMode)
	dst.GIFPipelineHighPriorityEnabled = settings.GIFPipelineHighPriorityEnabled
	dst.GIFPipelineHighPriorityMode = strings.TrimSpace(settings.GIFPipelineHighPriorityMode)
	dst.GIFDurationTierMediumSec = settings.GIFDurationTierMediumSec
	dst.GIFDurationTierLongSec = settings.GIFDurationTierLongSec
	dst.GIFDurationTierUltraSec = settings.GIFDurationTierUltraSec
	dst.GIFSegmentTimeoutMinSec = settings.GIFSegmentTimeoutMinSec
	dst.GIFSegmentTimeoutMaxSec = settings.GIFSegmentTimeoutMaxSec
	dst.GIFSegmentTimeoutFallbackCapSec = settings.GIFSegmentTimeoutFallbackCapSec
	dst.GIFSegmentTimeoutEmergencyCapSec = settings.GIFSegmentTimeoutEmergencyCapSec
	dst.GIFSegmentTimeoutLastResortCapSec = settings.GIFSegmentTimeoutLastResortCapSec
	dst.GIFRenderRetryMaxAttempts = settings.GIFRenderRetryMaxAttempts
	dst.GIFRenderRetryPrimaryColorsFloor = settings.GIFRenderRetryPrimaryColorsFloor
	dst.GIFRenderRetryPrimaryColorsStep = settings.GIFRenderRetryPrimaryColorsStep
	dst.GIFRenderRetryFPSFloor = settings.GIFRenderRetryFPSFloor
	dst.GIFRenderRetryFPSStep = settings.GIFRenderRetryFPSStep
	dst.GIFRenderRetryWidthTrigger = settings.GIFRenderRetryWidthTrigger
	dst.GIFRenderRetryWidthScale = settings.GIFRenderRetryWidthScale
	dst.GIFRenderRetryWidthFloor = settings.GIFRenderRetryWidthFloor
	dst.GIFRenderRetrySecondaryColorsFloor = settings.GIFRenderRetrySecondaryColorsFloor
	dst.GIFRenderRetrySecondaryColorsStep = settings.GIFRenderRetrySecondaryColorsStep
	dst.GIFRenderInitialSizeFPSCap = settings.GIFRenderInitialSizeFPSCap
	dst.GIFRenderInitialClarityFPSFloor = settings.GIFRenderInitialClarityFPSFloor
	dst.GIFRenderInitialSizeColorsCap = settings.GIFRenderInitialSizeColorsCap
	dst.GIFRenderInitialClarityColorsFloor = settings.GIFRenderInitialClarityColorsFloor
	dst.GIFMotionLowScoreThreshold = settings.GIFMotionLowScoreThreshold
	dst.GIFMotionHighScoreThreshold = settings.GIFMotionHighScoreThreshold
	dst.GIFMotionLowFPSDelta = settings.GIFMotionLowFPSDelta
	dst.GIFMotionHighFPSDelta = settings.GIFMotionHighFPSDelta
	dst.GIFAdaptiveFPSMin = settings.GIFAdaptiveFPSMin
	dst.GIFAdaptiveFPSMax = settings.GIFAdaptiveFPSMax
	dst.GIFWidthSizeLow = settings.GIFWidthSizeLow
	dst.GIFWidthSizeMedium = settings.GIFWidthSizeMedium
	dst.GIFWidthSizeHigh = settings.GIFWidthSizeHigh
	dst.GIFWidthClarityLow = settings.GIFWidthClarityLow
	dst.GIFWidthClarityMedium = settings.GIFWidthClarityMedium
	dst.GIFWidthClarityHigh = settings.GIFWidthClarityHigh
	dst.GIFColorsSizeLow = settings.GIFColorsSizeLow
	dst.GIFColorsSizeMedium = settings.GIFColorsSizeMedium
	dst.GIFColorsSizeHigh = settings.GIFColorsSizeHigh
	dst.GIFColorsClarityLow = settings.GIFColorsClarityLow
	dst.GIFColorsClarityMedium = settings.GIFColorsClarityMedium
	dst.GIFColorsClarityHigh = settings.GIFColorsClarityHigh
	dst.GIFDurationLowSec = settings.GIFDurationLowSec
	dst.GIFDurationMediumSec = settings.GIFDurationMediumSec
	dst.GIFDurationHighSec = settings.GIFDurationHighSec
	dst.GIFDurationSizeProfileMaxSec = settings.GIFDurationSizeProfileMaxSec
	dst.GIFDownshiftHighResLongSideThreshold = settings.GIFDownshiftHighResLongSideThreshold
	dst.GIFDownshiftEarlyDurationSec = settings.GIFDownshiftEarlyDurationSec
	dst.GIFDownshiftEarlyLongSideThreshold = settings.GIFDownshiftEarlyLongSideThreshold
	dst.GIFDownshiftMediumFPSCap = settings.GIFDownshiftMediumFPSCap
	dst.GIFDownshiftMediumWidthCap = settings.GIFDownshiftMediumWidthCap
	dst.GIFDownshiftMediumColorsCap = settings.GIFDownshiftMediumColorsCap
	dst.GIFDownshiftMediumDurationCapSec = settings.GIFDownshiftMediumDurationCapSec
	dst.GIFDownshiftLongFPSCap = settings.GIFDownshiftLongFPSCap
	dst.GIFDownshiftLongWidthCap = settings.GIFDownshiftLongWidthCap
	dst.GIFDownshiftLongColorsCap = settings.GIFDownshiftLongColorsCap
	dst.GIFDownshiftLongDurationCapSec = settings.GIFDownshiftLongDurationCapSec
	dst.GIFDownshiftUltraFPSCap = settings.GIFDownshiftUltraFPSCap
	dst.GIFDownshiftUltraWidthCap = settings.GIFDownshiftUltraWidthCap
	dst.GIFDownshiftUltraColorsCap = settings.GIFDownshiftUltraColorsCap
	dst.GIFDownshiftUltraDurationCapSec = settings.GIFDownshiftUltraDurationCapSec
	dst.GIFDownshiftHighResFPSCap = settings.GIFDownshiftHighResFPSCap
	dst.GIFDownshiftHighResWidthCap = settings.GIFDownshiftHighResWidthCap
	dst.GIFDownshiftHighResColorsCap = settings.GIFDownshiftHighResColorsCap
	dst.GIFDownshiftHighResDurationCapSec = settings.GIFDownshiftHighResDurationCapSec
	dst.GIFTimeoutFallbackFPSCap = settings.GIFTimeoutFallbackFPSCap
	dst.GIFTimeoutFallbackWidthCap = settings.GIFTimeoutFallbackWidthCap
	dst.GIFTimeoutFallbackColorsCap = settings.GIFTimeoutFallbackColorsCap
	dst.GIFTimeoutFallbackMinWidth = settings.GIFTimeoutFallbackMinWidth
	dst.GIFTimeoutFallbackUltraFPSCap = settings.GIFTimeoutFallbackUltraFPSCap
	dst.GIFTimeoutFallbackUltraWidthCap = settings.GIFTimeoutFallbackUltraWidthCap
	dst.GIFTimeoutFallbackUltraColorsCap = settings.GIFTimeoutFallbackUltraColorsCap
	dst.GIFTimeoutEmergencyFPSCap = settings.GIFTimeoutEmergencyFPSCap
	dst.GIFTimeoutEmergencyWidthCap = settings.GIFTimeoutEmergencyWidthCap
	dst.GIFTimeoutEmergencyColorsCap = settings.GIFTimeoutEmergencyColorsCap
	dst.GIFTimeoutEmergencyMinWidth = settings.GIFTimeoutEmergencyMinWidth
	dst.GIFTimeoutEmergencyDurationTrigger = settings.GIFTimeoutEmergencyDurationTrigger
	dst.GIFTimeoutEmergencyDurationScale = settings.GIFTimeoutEmergencyDurationScale
	dst.GIFTimeoutEmergencyDurationMinSec = settings.GIFTimeoutEmergencyDurationMinSec
	dst.GIFTimeoutLastResortFPSCap = settings.GIFTimeoutLastResortFPSCap
	dst.GIFTimeoutLastResortWidthCap = settings.GIFTimeoutLastResortWidthCap
	dst.GIFTimeoutLastResortColorsCap = settings.GIFTimeoutLastResortColorsCap
	dst.GIFTimeoutLastResortMinWidth = settings.GIFTimeoutLastResortMinWidth
	dst.GIFTimeoutLastResortDurationMinSec = settings.GIFTimeoutLastResortDurationMinSec
	dst.GIFTimeoutLastResortDurationMaxSec = settings.GIFTimeoutLastResortDurationMaxSec
	dst.WebPTargetSizeKB = settings.WebPTargetSizeKB
	dst.JPGTargetSizeKB = settings.JPGTargetSizeKB
	dst.PNGTargetSizeKB = settings.PNGTargetSizeKB
	dst.StillMinBlurScore = settings.StillMinBlurScore
	dst.StillMinExposureScore = settings.StillMinExposureScore
	dst.StillMinWidth = settings.StillMinWidth
	dst.StillMinHeight = settings.StillMinHeight
	dst.LiveCoverPortraitWeight = settings.LiveCoverPortraitWeight
	dst.LiveCoverSceneMinSamples = settings.LiveCoverSceneMinSamples
	dst.LiveCoverGuardMinTotal = settings.LiveCoverGuardMinTotal
	dst.LiveCoverGuardScoreFloor = settings.LiveCoverGuardScoreFloor
	dst.HighlightFeedbackEnabled = settings.HighlightFeedbackEnabled
	dst.HighlightFeedbackRollout = settings.HighlightFeedbackRollout
	dst.HighlightFeedbackMinJobs = settings.HighlightFeedbackMinJobs
	dst.HighlightFeedbackMinScore = settings.HighlightFeedbackMinScore
	dst.HighlightFeedbackBoost = settings.HighlightFeedbackBoost
	dst.HighlightWeightPosition = settings.HighlightWeightPosition
	dst.HighlightWeightDuration = settings.HighlightWeightDuration
	dst.HighlightWeightReason = settings.HighlightWeightReason
	dst.HighlightNegativeGuardEnabled = settings.HighlightNegativeGuardEnabled
	dst.HighlightNegativeGuardThreshold = settings.HighlightNegativeGuardThreshold
	dst.HighlightNegativeGuardMinWeight = settings.HighlightNegativeGuardMinWeight
	dst.HighlightNegativePenaltyScale = settings.HighlightNegativePenaltyScale
	dst.HighlightNegativePenaltyWeight = settings.HighlightNegativePenaltyWeight
	dst.AIDirectorInputMode = strings.TrimSpace(settings.AIDirectorInputMode)
	dst.AIDirectorOperatorInstruction = strings.TrimSpace(settings.AIDirectorOperatorInstruction)
	dst.AIDirectorOperatorInstructionVersion = strings.TrimSpace(settings.AIDirectorOperatorInstructionVersion)
	dst.AIDirectorOperatorEnabled = settings.AIDirectorOperatorEnabled
	dst.AIDirectorConstraintOverrideEnabled = settings.AIDirectorConstraintOverrideEnabled
	dst.AIDirectorCountExpandRatio = settings.AIDirectorCountExpandRatio
	dst.AIDirectorDurationExpandRatio = settings.AIDirectorDurationExpandRatio
	dst.AIDirectorCountAbsoluteCap = settings.AIDirectorCountAbsoluteCap
	dst.AIDirectorDurationAbsoluteCapSec = settings.AIDirectorDurationAbsoluteCapSec
	dst.GIFAIJudgeHardGateMinOverallScore = settings.GIFAIJudgeHardGateMinOverallScore
	dst.GIFAIJudgeHardGateMinClarityScore = settings.GIFAIJudgeHardGateMinClarityScore
	dst.GIFAIJudgeHardGateMinLoopScore = settings.GIFAIJudgeHardGateMinLoopScore
	dst.GIFAIJudgeHardGateMinOutputScore = settings.GIFAIJudgeHardGateMinOutputScore
	dst.GIFAIJudgeHardGateMinDurationMS = settings.GIFAIJudgeHardGateMinDurationMS
	dst.GIFAIJudgeHardGateSizeMultiplier = settings.GIFAIJudgeHardGateSizeMultiplier
}

func toVideoQualitySettingResponse(setting models.VideoQualitySetting, withMeta bool) VideoQualitySettingResponse {
	resp := VideoQualitySettingResponse{
		QualitySettings:                         qualitySettingsFromModel(setting),
		GIFHealthAlertThresholdSettings:         gifHealthAlertThresholdSettingsFromModel(setting),
		FeedbackIntegrityAlertThresholdSettings: feedbackIntegrityAlertThresholdSettingsFromModel(setting),
	}
	if withMeta {
		if !setting.CreatedAt.IsZero() {
			resp.CreatedAt = setting.CreatedAt.Format("2006-01-02 15:04:05")
		}
		if !setting.UpdatedAt.IsZero() {
			resp.UpdatedAt = setting.UpdatedAt.Format("2006-01-02 15:04:05")
		}
	}
	return resp
}

func attachVideoQualitySettingScopeMeta(resp *VideoQualitySettingResponse, format string, resolvedFrom []string, override *models.VideoQualitySettingScoped) {
	if resp == nil {
		return
	}
	if format == "" {
		format = videoQualitySettingFormatAll
	}
	resp.FormatScope = format
	if len(resolvedFrom) > 0 {
		resp.ResolvedFrom = resolvedFrom
	}
	if override != nil {
		resp.OverrideVersion = strings.TrimSpace(override.Version)
	}
}

func (h *Handler) loadVideoQualitySetting() (models.VideoQualitySetting, error) {
	var setting models.VideoQualitySetting
	if err := h.db.First(&setting, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			defaults := videojobs.DefaultQualitySettings()
			applyQualitySettingsToModel(&setting, defaults)
			applyGIFHealthAlertThresholdSettingsToModel(&setting, defaultGIFHealthAlertThresholdSettings())
			applyFeedbackIntegrityAlertThresholdSettingsToModel(&setting, defaultFeedbackIntegrityAlertThresholdSettings())
			return setting, nil
		}
		return models.VideoQualitySetting{}, err
	}

	normalized := qualitySettingsFromModel(setting)
	applyQualitySettingsToModel(&setting, normalized)
	applyGIFHealthAlertThresholdSettingsToModel(&setting, gifHealthAlertThresholdSettingsFromModel(setting))
	applyFeedbackIntegrityAlertThresholdSettingsToModel(&setting, feedbackIntegrityAlertThresholdSettingsFromModel(setting))
	return setting, nil
}

func (h *Handler) loadLatestVideoQualityRolloutAudit() (*models.VideoQualityRolloutAudit, error) {
	if h == nil || h.db == nil {
		return nil, nil
	}
	var row models.VideoQualityRolloutAudit
	if err := h.db.Order("id DESC").Limit(1).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// GetAdminVideoQualitySetting godoc
// @Summary Get video quality tuning setting (admin)
// @Tags admin
// @Produce json
// @Param format query string false "format scope: all|gif|png|jpg|webp|live|mp4"
// @Success 200 {object} VideoQualitySettingResponse
// @Router /api/admin/video-jobs/quality-settings [get]
func (h *Handler) GetAdminVideoQualitySetting(c *gin.Context) {
	formatScope, err := normalizeVideoQualitySettingFormatScope(c.Query("format"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	setting, resolvedFrom, override, err := h.resolveVideoQualitySettingByFormat(formatScope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := toVideoQualitySettingResponse(setting, true)
	attachVideoQualitySettingScopeMeta(&resp, formatScope, resolvedFrom, override)
	c.JSON(http.StatusOK, resp)
}

func videoQualitySettingRequestFromModel(setting models.VideoQualitySetting) VideoQualitySettingRequest {
	quality := qualitySettingsFromModel(setting)
	gifThresholds := gifHealthAlertThresholdSettingsFromModel(setting)
	feedbackThresholds := feedbackIntegrityAlertThresholdSettingsFromModel(setting)
	return VideoQualitySettingRequest{
		MinBrightness:                           quality.MinBrightness,
		MaxBrightness:                           quality.MaxBrightness,
		BlurThresholdFactor:                     quality.BlurThresholdFactor,
		BlurThresholdMin:                        quality.BlurThresholdMin,
		BlurThresholdMax:                        quality.BlurThresholdMax,
		DuplicateHammingThreshold:               quality.DuplicateHammingThreshold,
		DuplicateBacktrackFrames:                quality.DuplicateBacktrackFrames,
		FallbackBlurRelaxFactor:                 quality.FallbackBlurRelaxFactor,
		FallbackHammingThreshold:                quality.FallbackHammingThreshold,
		MinKeepBase:                             quality.MinKeepBase,
		MinKeepRatio:                            quality.MinKeepRatio,
		QualityAnalysisWorkers:                  quality.QualityAnalysisWorkers,
		UploadConcurrency:                       quality.UploadConcurrency,
		GIFProfile:                              quality.GIFProfile,
		WebPProfile:                             quality.WebPProfile,
		LiveProfile:                             quality.LiveProfile,
		JPGProfile:                              quality.JPGProfile,
		PNGProfile:                              quality.PNGProfile,
		GIFDefaultFPS:                           quality.GIFDefaultFPS,
		GIFDefaultMaxColors:                     quality.GIFDefaultMaxColors,
		GIFDitherMode:                           quality.GIFDitherMode,
		GIFTargetSizeKB:                         quality.GIFTargetSizeKB,
		GIFGifsicleEnabled:                      quality.GIFGifsicleEnabled,
		GIFGifsicleLevel:                        quality.GIFGifsicleLevel,
		GIFGifsicleSkipBelowKB:                  quality.GIFGifsicleSkipBelowKB,
		GIFGifsicleMinGainRatio:                 quality.GIFGifsicleMinGainRatio,
		GIFLoopTuneEnabled:                      quality.GIFLoopTuneEnabled,
		GIFLoopTuneMinEnableSec:                 quality.GIFLoopTuneMinEnableSec,
		GIFLoopTuneMinImprovement:               quality.GIFLoopTuneMinImprovement,
		GIFLoopTuneMotionTarget:                 quality.GIFLoopTuneMotionTarget,
		GIFLoopTunePreferDuration:               quality.GIFLoopTunePreferDuration,
		GIFCandidateMaxOutputs:                  quality.GIFCandidateMaxOutputs,
		GIFCandidateLongVideoMaxOutputs:         quality.GIFCandidateLongVideoMaxOutputs,
		GIFCandidateUltraVideoMaxOutputs:        quality.GIFCandidateUltraVideoMaxOutputs,
		GIFCandidateConfidenceThreshold:         quality.GIFCandidateConfidenceThreshold,
		GIFCandidateDedupIOUThreshold:           quality.GIFCandidateDedupIOUThreshold,
		GIFRenderBudgetNormalMultiplier:         quality.GIFRenderBudgetNormalMultiplier,
		GIFRenderBudgetLongMultiplier:           quality.GIFRenderBudgetLongMultiplier,
		GIFRenderBudgetUltraMultiplier:          quality.GIFRenderBudgetUltraMultiplier,
		GIFPipelineShortVideoMaxSec:             quality.GIFPipelineShortVideoMaxSec,
		GIFPipelineLongVideoMinSec:              quality.GIFPipelineLongVideoMinSec,
		GIFPipelineShortVideoMode:               quality.GIFPipelineShortVideoMode,
		GIFPipelineDefaultMode:                  quality.GIFPipelineDefaultMode,
		GIFPipelineLongVideoMode:                quality.GIFPipelineLongVideoMode,
		GIFPipelineHighPriorityEnabled:          quality.GIFPipelineHighPriorityEnabled,
		GIFPipelineHighPriorityMode:             quality.GIFPipelineHighPriorityMode,
		GIFDurationTierMediumSec:                quality.GIFDurationTierMediumSec,
		GIFDurationTierLongSec:                  quality.GIFDurationTierLongSec,
		GIFDurationTierUltraSec:                 quality.GIFDurationTierUltraSec,
		GIFSegmentTimeoutMinSec:                 quality.GIFSegmentTimeoutMinSec,
		GIFSegmentTimeoutMaxSec:                 quality.GIFSegmentTimeoutMaxSec,
		GIFSegmentTimeoutFallbackCapSec:         quality.GIFSegmentTimeoutFallbackCapSec,
		GIFSegmentTimeoutEmergencyCapSec:        quality.GIFSegmentTimeoutEmergencyCapSec,
		GIFSegmentTimeoutLastResortCapSec:       quality.GIFSegmentTimeoutLastResortCapSec,
		GIFRenderRetryMaxAttempts:               quality.GIFRenderRetryMaxAttempts,
		GIFRenderRetryPrimaryColorsFloor:        quality.GIFRenderRetryPrimaryColorsFloor,
		GIFRenderRetryPrimaryColorsStep:         quality.GIFRenderRetryPrimaryColorsStep,
		GIFRenderRetryFPSFloor:                  quality.GIFRenderRetryFPSFloor,
		GIFRenderRetryFPSStep:                   quality.GIFRenderRetryFPSStep,
		GIFRenderRetryWidthTrigger:              quality.GIFRenderRetryWidthTrigger,
		GIFRenderRetryWidthScale:                quality.GIFRenderRetryWidthScale,
		GIFRenderRetryWidthFloor:                quality.GIFRenderRetryWidthFloor,
		GIFRenderRetrySecondaryColorsFloor:      quality.GIFRenderRetrySecondaryColorsFloor,
		GIFRenderRetrySecondaryColorsStep:       quality.GIFRenderRetrySecondaryColorsStep,
		GIFRenderInitialSizeFPSCap:              quality.GIFRenderInitialSizeFPSCap,
		GIFRenderInitialClarityFPSFloor:         quality.GIFRenderInitialClarityFPSFloor,
		GIFRenderInitialSizeColorsCap:           quality.GIFRenderInitialSizeColorsCap,
		GIFRenderInitialClarityColorsFloor:      quality.GIFRenderInitialClarityColorsFloor,
		GIFMotionLowScoreThreshold:              quality.GIFMotionLowScoreThreshold,
		GIFMotionHighScoreThreshold:             quality.GIFMotionHighScoreThreshold,
		GIFMotionLowFPSDelta:                    quality.GIFMotionLowFPSDelta,
		GIFMotionHighFPSDelta:                   quality.GIFMotionHighFPSDelta,
		GIFAdaptiveFPSMin:                       quality.GIFAdaptiveFPSMin,
		GIFAdaptiveFPSMax:                       quality.GIFAdaptiveFPSMax,
		GIFWidthSizeLow:                         quality.GIFWidthSizeLow,
		GIFWidthSizeMedium:                      quality.GIFWidthSizeMedium,
		GIFWidthSizeHigh:                        quality.GIFWidthSizeHigh,
		GIFWidthClarityLow:                      quality.GIFWidthClarityLow,
		GIFWidthClarityMedium:                   quality.GIFWidthClarityMedium,
		GIFWidthClarityHigh:                     quality.GIFWidthClarityHigh,
		GIFColorsSizeLow:                        quality.GIFColorsSizeLow,
		GIFColorsSizeMedium:                     quality.GIFColorsSizeMedium,
		GIFColorsSizeHigh:                       quality.GIFColorsSizeHigh,
		GIFColorsClarityLow:                     quality.GIFColorsClarityLow,
		GIFColorsClarityMedium:                  quality.GIFColorsClarityMedium,
		GIFColorsClarityHigh:                    quality.GIFColorsClarityHigh,
		GIFDurationLowSec:                       quality.GIFDurationLowSec,
		GIFDurationMediumSec:                    quality.GIFDurationMediumSec,
		GIFDurationHighSec:                      quality.GIFDurationHighSec,
		GIFDurationSizeProfileMaxSec:            quality.GIFDurationSizeProfileMaxSec,
		GIFDownshiftHighResLongSideThreshold:    quality.GIFDownshiftHighResLongSideThreshold,
		GIFDownshiftEarlyDurationSec:            quality.GIFDownshiftEarlyDurationSec,
		GIFDownshiftEarlyLongSideThreshold:      quality.GIFDownshiftEarlyLongSideThreshold,
		GIFDownshiftMediumFPSCap:                quality.GIFDownshiftMediumFPSCap,
		GIFDownshiftMediumWidthCap:              quality.GIFDownshiftMediumWidthCap,
		GIFDownshiftMediumColorsCap:             quality.GIFDownshiftMediumColorsCap,
		GIFDownshiftMediumDurationCapSec:        quality.GIFDownshiftMediumDurationCapSec,
		GIFDownshiftLongFPSCap:                  quality.GIFDownshiftLongFPSCap,
		GIFDownshiftLongWidthCap:                quality.GIFDownshiftLongWidthCap,
		GIFDownshiftLongColorsCap:               quality.GIFDownshiftLongColorsCap,
		GIFDownshiftLongDurationCapSec:          quality.GIFDownshiftLongDurationCapSec,
		GIFDownshiftUltraFPSCap:                 quality.GIFDownshiftUltraFPSCap,
		GIFDownshiftUltraWidthCap:               quality.GIFDownshiftUltraWidthCap,
		GIFDownshiftUltraColorsCap:              quality.GIFDownshiftUltraColorsCap,
		GIFDownshiftUltraDurationCapSec:         quality.GIFDownshiftUltraDurationCapSec,
		GIFDownshiftHighResFPSCap:               quality.GIFDownshiftHighResFPSCap,
		GIFDownshiftHighResWidthCap:             quality.GIFDownshiftHighResWidthCap,
		GIFDownshiftHighResColorsCap:            quality.GIFDownshiftHighResColorsCap,
		GIFDownshiftHighResDurationCapSec:       quality.GIFDownshiftHighResDurationCapSec,
		GIFTimeoutFallbackFPSCap:                quality.GIFTimeoutFallbackFPSCap,
		GIFTimeoutFallbackWidthCap:              quality.GIFTimeoutFallbackWidthCap,
		GIFTimeoutFallbackColorsCap:             quality.GIFTimeoutFallbackColorsCap,
		GIFTimeoutFallbackMinWidth:              quality.GIFTimeoutFallbackMinWidth,
		GIFTimeoutFallbackUltraFPSCap:           quality.GIFTimeoutFallbackUltraFPSCap,
		GIFTimeoutFallbackUltraWidthCap:         quality.GIFTimeoutFallbackUltraWidthCap,
		GIFTimeoutFallbackUltraColorsCap:        quality.GIFTimeoutFallbackUltraColorsCap,
		GIFTimeoutEmergencyFPSCap:               quality.GIFTimeoutEmergencyFPSCap,
		GIFTimeoutEmergencyWidthCap:             quality.GIFTimeoutEmergencyWidthCap,
		GIFTimeoutEmergencyColorsCap:            quality.GIFTimeoutEmergencyColorsCap,
		GIFTimeoutEmergencyMinWidth:             quality.GIFTimeoutEmergencyMinWidth,
		GIFTimeoutEmergencyDurationTrigger:      quality.GIFTimeoutEmergencyDurationTrigger,
		GIFTimeoutEmergencyDurationScale:        quality.GIFTimeoutEmergencyDurationScale,
		GIFTimeoutEmergencyDurationMinSec:       quality.GIFTimeoutEmergencyDurationMinSec,
		GIFTimeoutLastResortFPSCap:              quality.GIFTimeoutLastResortFPSCap,
		GIFTimeoutLastResortWidthCap:            quality.GIFTimeoutLastResortWidthCap,
		GIFTimeoutLastResortColorsCap:           quality.GIFTimeoutLastResortColorsCap,
		GIFTimeoutLastResortMinWidth:            quality.GIFTimeoutLastResortMinWidth,
		GIFTimeoutLastResortDurationMinSec:      quality.GIFTimeoutLastResortDurationMinSec,
		GIFTimeoutLastResortDurationMaxSec:      quality.GIFTimeoutLastResortDurationMaxSec,
		WebPTargetSizeKB:                        quality.WebPTargetSizeKB,
		JPGTargetSizeKB:                         quality.JPGTargetSizeKB,
		PNGTargetSizeKB:                         quality.PNGTargetSizeKB,
		StillMinBlurScore:                       quality.StillMinBlurScore,
		StillMinExposureScore:                   quality.StillMinExposureScore,
		StillMinWidth:                           quality.StillMinWidth,
		StillMinHeight:                          quality.StillMinHeight,
		LiveCoverPortraitWeight:                 quality.LiveCoverPortraitWeight,
		LiveCoverSceneMinSamples:                quality.LiveCoverSceneMinSamples,
		LiveCoverGuardMinTotal:                  quality.LiveCoverGuardMinTotal,
		LiveCoverGuardScoreFloor:                quality.LiveCoverGuardScoreFloor,
		HighlightFeedbackEnabled:                quality.HighlightFeedbackEnabled,
		HighlightFeedbackRollout:                quality.HighlightFeedbackRollout,
		HighlightFeedbackMinJobs:                quality.HighlightFeedbackMinJobs,
		HighlightFeedbackMinScore:               quality.HighlightFeedbackMinScore,
		HighlightFeedbackBoost:                  quality.HighlightFeedbackBoost,
		HighlightWeightPosition:                 quality.HighlightWeightPosition,
		HighlightWeightDuration:                 quality.HighlightWeightDuration,
		HighlightWeightReason:                   quality.HighlightWeightReason,
		HighlightNegativeGuardEnabled:           quality.HighlightNegativeGuardEnabled,
		HighlightNegativeGuardThreshold:         quality.HighlightNegativeGuardThreshold,
		HighlightNegativeGuardMinWeight:         quality.HighlightNegativeGuardMinWeight,
		HighlightNegativePenaltyScale:           quality.HighlightNegativePenaltyScale,
		HighlightNegativePenaltyWeight:          quality.HighlightNegativePenaltyWeight,
		AIDirectorInputMode:                     quality.AIDirectorInputMode,
		AIDirectorOperatorInstruction:           quality.AIDirectorOperatorInstruction,
		AIDirectorOperatorInstructionVersion:    quality.AIDirectorOperatorInstructionVersion,
		AIDirectorOperatorEnabled:               quality.AIDirectorOperatorEnabled,
		AIDirectorConstraintOverrideEnabled:     quality.AIDirectorConstraintOverrideEnabled,
		AIDirectorCountExpandRatio:              quality.AIDirectorCountExpandRatio,
		AIDirectorDurationExpandRatio:           quality.AIDirectorDurationExpandRatio,
		AIDirectorCountAbsoluteCap:              quality.AIDirectorCountAbsoluteCap,
		AIDirectorDurationAbsoluteCapSec:        quality.AIDirectorDurationAbsoluteCapSec,
		GIFAIJudgeHardGateMinOverallScore:       quality.GIFAIJudgeHardGateMinOverallScore,
		GIFAIJudgeHardGateMinClarityScore:       quality.GIFAIJudgeHardGateMinClarityScore,
		GIFAIJudgeHardGateMinLoopScore:          quality.GIFAIJudgeHardGateMinLoopScore,
		GIFAIJudgeHardGateMinOutputScore:        quality.GIFAIJudgeHardGateMinOutputScore,
		GIFAIJudgeHardGateMinDurationMS:         quality.GIFAIJudgeHardGateMinDurationMS,
		GIFAIJudgeHardGateSizeMultiplier:        quality.GIFAIJudgeHardGateSizeMultiplier,
		GIFHealthAlertThresholdSettings:         gifThresholds,
		FeedbackIntegrityAlertThresholdSettings: feedbackThresholds,
	}
}

func (h *Handler) saveVideoQualitySetting(req VideoQualitySettingRequest) (models.VideoQualitySetting, error) {
	var saved models.VideoQualitySetting
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var current models.VideoQualitySetting
		err := tx.First(&current, 1).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			payload, buildErr := buildVideoQualitySettingModelFromRequest(req, models.VideoQualitySetting{})
			if buildErr != nil {
				return buildErr
			}
			payload.ID = 1
			if err := tx.Create(&payload).Error; err != nil {
				return err
			}
			saved = payload
			return nil
		}

		payload, buildErr := buildVideoQualitySettingModelFromRequest(req, current)
		if buildErr != nil {
			return buildErr
		}
		payload.ID = current.ID
		if err := tx.Save(&payload).Error; err != nil {
			return err
		}
		saved = payload
		return nil
	})
	return saved, err
}

func (h *Handler) bindVideoQualitySettingPatch(c *gin.Context) (VideoQualitySettingRequest, error) {
	if h == nil || c == nil {
		return VideoQualitySettingRequest{}, errors.New("invalid request")
	}
	current, err := h.loadVideoQualitySetting()
	if err != nil {
		return VideoQualitySettingRequest{}, err
	}
	return h.bindVideoQualitySettingPatchWithBase(c, videoQualitySettingRequestFromModel(current))
}

func (h *Handler) bindVideoQualitySettingPatchWithBase(c *gin.Context, base VideoQualitySettingRequest) (VideoQualitySettingRequest, error) {
	if h == nil || c == nil {
		return VideoQualitySettingRequest{}, errors.New("invalid request")
	}
	raw, err := c.GetRawData()
	if err != nil {
		return VideoQualitySettingRequest{}, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return VideoQualitySettingRequest{}, errors.New("empty request body")
	}
	var incoming map[string]interface{}
	if err := json.Unmarshal(raw, &incoming); err != nil {
		return VideoQualitySettingRequest{}, err
	}
	delete(incoming, "created_at")
	delete(incoming, "updated_at")
	if len(incoming) == 0 {
		return VideoQualitySettingRequest{}, errors.New("no patch fields provided")
	}

	req := base
	if err := json.Unmarshal(raw, &req); err != nil {
		return VideoQualitySettingRequest{}, err
	}
	return req, nil
}

// UpdateAdminVideoQualitySetting godoc
// @Summary Update video quality tuning setting (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param format query string false "format scope: all|gif|png|jpg|webp|live|mp4"
// @Param body body VideoQualitySettingRequest true "video quality setting"
// @Success 200 {object} VideoQualitySettingResponse
// @Router /api/admin/video-jobs/quality-settings [put]
func (h *Handler) UpdateAdminVideoQualitySetting(c *gin.Context) {
	formatScope, err := normalizeVideoQualitySettingFormatScope(c.Query("format"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req VideoQualitySettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateVideoQualitySettingRequest(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if formatScope == videoQualitySettingFormatAll {
		beforeSetting, err := h.loadVideoQualitySetting()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		beforeReq := videoQualitySettingRequestFromModel(beforeSetting)
		saved, err := h.saveVideoQualitySetting(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		h.recordVideoQualitySettingAudit(c, formatScope, []string{videoQualitySettingFormatAll}, "put", beforeReq, videoQualitySettingRequestFromModel(saved))
		resp := toVideoQualitySettingResponse(saved, true)
		attachVideoQualitySettingScopeMeta(&resp, formatScope, []string{videoQualitySettingFormatAll}, nil)
		c.JSON(http.StatusOK, resp)
		return
	}

	beforeScopedSetting, _, _, err := h.resolveVideoQualitySettingByFormat(formatScope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	beforeScopedReq := videoQualitySettingRequestFromModel(beforeScopedSetting)

	base, err := h.loadVideoQualitySetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	baseReq := videoQualitySettingRequestFromModel(base)
	baseMap, err := videoQualitySettingRequestToMap(baseReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetMap, err := videoQualitySettingRequestToMap(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	override := diffVideoQualitySettingMaps(baseMap, targetMap)
	adminID, _ := currentUserIDFromContext(c)
	if _, err := h.saveVideoQualitySettingScope(formatScope, override, adminID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	effective, resolvedFrom, scopeRow, err := h.resolveVideoQualitySettingByFormat(formatScope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.recordVideoQualitySettingAudit(c, formatScope, resolvedFrom, "put", beforeScopedReq, videoQualitySettingRequestFromModel(effective))
	resp := toVideoQualitySettingResponse(effective, true)
	attachVideoQualitySettingScopeMeta(&resp, formatScope, resolvedFrom, scopeRow)
	c.JSON(http.StatusOK, resp)
}

// PatchAdminVideoQualitySetting godoc
// @Summary Patch video quality tuning setting (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param format query string false "format scope: all|gif|png|jpg|webp|live|mp4"
// @Param body body object true "video quality setting patch"
// @Success 200 {object} VideoQualitySettingResponse
// @Router /api/admin/video-jobs/quality-settings [patch]
func (h *Handler) PatchAdminVideoQualitySetting(c *gin.Context) {
	formatScope, err := normalizeVideoQualitySettingFormatScope(c.Query("format"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if formatScope == videoQualitySettingFormatAll {
		beforeSetting, err := h.loadVideoQualitySetting()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		beforeReq := videoQualitySettingRequestFromModel(beforeSetting)
		req, err := h.bindVideoQualitySettingPatch(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := validateVideoQualitySettingRequest(req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		saved, err := h.saveVideoQualitySetting(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		h.recordVideoQualitySettingAudit(c, formatScope, []string{videoQualitySettingFormatAll}, "patch", beforeReq, videoQualitySettingRequestFromModel(saved))
		resp := toVideoQualitySettingResponse(saved, true)
		attachVideoQualitySettingScopeMeta(&resp, formatScope, []string{videoQualitySettingFormatAll}, nil)
		c.JSON(http.StatusOK, resp)
		return
	}

	effective, _, _, err := h.resolveVideoQualitySettingByFormat(formatScope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	beforeScopedReq := videoQualitySettingRequestFromModel(effective)
	req, err := h.bindVideoQualitySettingPatchWithBase(c, videoQualitySettingRequestFromModel(effective))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateVideoQualitySettingRequest(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	base, err := h.loadVideoQualitySetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	baseMap, err := videoQualitySettingRequestToMap(videoQualitySettingRequestFromModel(base))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetMap, err := videoQualitySettingRequestToMap(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	override := diffVideoQualitySettingMaps(baseMap, targetMap)
	adminID, _ := currentUserIDFromContext(c)
	if _, err := h.saveVideoQualitySettingScope(formatScope, override, adminID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resolvedSetting, resolvedFrom, scopeRow, err := h.resolveVideoQualitySettingByFormat(formatScope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.recordVideoQualitySettingAudit(c, formatScope, resolvedFrom, "patch", beforeScopedReq, videoQualitySettingRequestFromModel(resolvedSetting))
	resp := toVideoQualitySettingResponse(resolvedSetting, true)
	attachVideoQualitySettingScopeMeta(&resp, formatScope, resolvedFrom, scopeRow)
	c.JSON(http.StatusOK, resp)
}

// ApplyAdminVideoQualityRolloutSuggestion godoc
// @Summary Apply rollout suggestion to quality setting (admin)
// @Tags admin
// @Produce json
// @Param window query string false "window: 24h | 7d | 30d"
// @Param confirm_windows query int false "required consecutive windows, default 3"
// @Success 200 {object} ApplyVideoQualityRolloutSuggestionResponse
// @Router /api/admin/video-jobs/quality-settings/apply-rollout-suggestion [post]
func (h *Handler) ApplyAdminVideoQualityRolloutSuggestion(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	confirmWindows := 3
	if raw := strings.TrimSpace(c.Query("confirm_windows")); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 1 || value > 12 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid confirm_windows, expected 1..12"})
			return
		}
		confirmWindows = value
	}

	currentSetting, err := h.loadVideoQualitySetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	currentQualitySettings := qualitySettingsFromModel(currentSetting)
	liveGuardConfig := buildLiveCoverSceneGuardConfigFromQualitySettings(currentQualitySettings)
	now := time.Now()
	history, err := h.loadVideoJobFeedbackGroupStatsHistory(windowDuration, confirmWindows, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	sinceWindow := now.Add(-windowDuration)
	liveCoverSceneStats, err := h.loadVideoJobLiveCoverSceneStats(sinceWindow, int64(currentQualitySettings.LiveCoverSceneMinSamples))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	recommendation := buildFeedbackRolloutRecommendationWithHistory(
		currentQualitySettings.HighlightFeedbackEnabled,
		currentQualitySettings.HighlightFeedbackRollout,
		history,
		confirmWindows,
		liveCoverSceneStats,
		liveGuardConfig,
	)

	response := ApplyVideoQualityRolloutSuggestionResponse{
		Applied:        false,
		Window:         windowLabel,
		ConfirmWindows: confirmWindows,
		Recommendation: recommendation,
		Setting:        toVideoQualitySettingResponse(currentSetting, true),
		Message:        recommendation.Reason,
	}
	shouldApply := shouldApplyFeedbackRolloutRecommendation(recommendation)
	if !shouldApply {
		c.JSON(http.StatusOK, response)
		return
	}

	latestAudit, err := h.loadLatestVideoQualityRolloutAudit()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if latestAudit != nil {
		if remainingSeconds, nextAllowedAt, inCooldown := rolloutCooldownState(now, latestAudit.CreatedAt, videoQualityRolloutApplyCooldown); inCooldown {
			response.Message = fmt.Sprintf("rollout 调整冷却中，请在 %s 后重试（剩余 %ds）", nextAllowedAt.Format("2006-01-02 15:04:05"), remainingSeconds)
			response.CooldownSeconds = remainingSeconds
			response.NextAllowedAt = nextAllowedAt.Format(time.RFC3339)
			c.JSON(http.StatusOK, response)
			return
		}
	}

	adminID, _ := currentUserIDFromContext(c)
	var appliedAt time.Time
	err = h.db.Transaction(func(tx *gorm.DB) error {
		normalized := qualitySettingsFromModel(currentSetting)
		normalized.HighlightFeedbackRollout = recommendation.SuggestedRolloutPercent
		normalized = videojobs.NormalizeQualitySettings(normalized)
		applyQualitySettingsToModel(&currentSetting, normalized)
		if err := tx.Save(&currentSetting).Error; err != nil {
			return err
		}

		metadata, err := buildRolloutAuditMetadata(recommendation)
		if err != nil {
			return err
		}
		audit := models.VideoQualityRolloutAudit{
			AdminID:              adminID,
			FromRolloutPercent:   recommendation.CurrentRolloutPercent,
			ToRolloutPercent:     recommendation.SuggestedRolloutPercent,
			Window:               windowLabel,
			ConfirmWindows:       confirmWindows,
			RecommendationState:  strings.ToLower(strings.TrimSpace(recommendation.State)),
			RecommendationReason: recommendation.Reason,
			ConsecutiveRequired:  recommendation.ConsecutiveRequired,
			ConsecutiveMatched:   recommendation.ConsecutiveMatched,
			Metadata:             metadata,
		}
		if err := tx.Create(&audit).Error; err != nil {
			return err
		}
		appliedAt = audit.CreatedAt
		if appliedAt.IsZero() {
			appliedAt = time.Now()
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response.Applied = true
	response.Message = fmt.Sprintf("已应用 rollout 建议：%d%% -> %d%%", recommendation.CurrentRolloutPercent, recommendation.SuggestedRolloutPercent)
	response.Setting = toVideoQualitySettingResponse(currentSetting, true)
	response.Recommendation.CurrentRolloutPercent = recommendation.SuggestedRolloutPercent
	if appliedAt.IsZero() {
		appliedAt = currentSetting.UpdatedAt
	}
	if appliedAt.IsZero() {
		appliedAt = now
	}
	response.AppliedAt = appliedAt.Format(time.RFC3339)
	c.JSON(http.StatusOK, response)
}

// ListAdminVideoQualityRolloutEffects godoc
// @Summary List recent rollout effects (admin)
// @Tags admin
// @Produce json
// @Param limit query int false "max cards, default 6, max 20"
// @Success 200 {object} ListVideoQualityRolloutEffectsResponse
// @Router /api/admin/video-jobs/quality-settings/rollout-effects [get]
func (h *Handler) ListAdminVideoQualityRolloutEffects(c *gin.Context) {
	limit := 6
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 20 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit, expected 1..20"})
			return
		}
		limit = value
	}

	audits, err := h.loadVideoQualityRolloutAudits(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(audits) == 0 {
		c.JSON(http.StatusOK, ListVideoQualityRolloutEffectsResponse{Items: []VideoQualityRolloutEffectCard{}})
		return
	}

	items := make([]VideoQualityRolloutEffectCard, 0, len(audits))
	for _, audit := range audits {
		windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(audit.Window)
		if err != nil {
			windowLabel = "24h"
			windowDuration = 24 * time.Hour
		}
		targetEnd := audit.CreatedAt
		if targetEnd.IsZero() {
			targetEnd = time.Now()
		}
		targetStart := targetEnd.Add(-windowDuration)
		baseEnd := targetStart
		baseStart := baseEnd.Add(-windowDuration)

		baseMetrics, err := h.loadVideoQualityRolloutEffectMetrics(baseStart, baseEnd)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		targetMetrics, err := h.loadVideoQualityRolloutEffectMetrics(targetStart, targetEnd)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		delta := VideoQualityRolloutEffectDelta{
			DoneRateDelta:          targetMetrics.DoneRate - baseMetrics.DoneRate,
			FailedRateDelta:        targetMetrics.FailedRate - baseMetrics.FailedRate,
			AvgOutputScoreDelta:    targetMetrics.AvgOutputScore - baseMetrics.AvgOutputScore,
			AvgLoopClosureDelta:    targetMetrics.AvgLoopClosure - baseMetrics.AvgLoopClosure,
			LoopEffectiveRateDelta: targetMetrics.LoopEffectiveRate - baseMetrics.LoopEffectiveRate,
			LoopFallbackRateDelta:  targetMetrics.LoopFallbackRate - baseMetrics.LoopFallbackRate,
		}
		verdict, reason := evaluateVideoQualityRolloutEffect(baseMetrics, targetMetrics, delta)

		items = append(items, VideoQualityRolloutEffectCard{
			AuditID:             audit.ID,
			AdminID:             audit.AdminID,
			AppliedAt:           targetEnd.Format(time.RFC3339),
			Window:              windowLabel,
			FromRolloutPercent:  audit.FromRolloutPercent,
			ToRolloutPercent:    audit.ToRolloutPercent,
			BaseWindowStart:     baseStart.Format(time.RFC3339),
			BaseWindowEnd:       baseEnd.Format(time.RFC3339),
			TargetWindowStart:   targetStart.Format(time.RFC3339),
			TargetWindowEnd:     targetEnd.Format(time.RFC3339),
			BaseMetrics:         baseMetrics,
			TargetMetrics:       targetMetrics,
			Delta:               delta,
			Verdict:             verdict,
			VerdictReason:       reason,
			RecommendationState: strings.TrimSpace(strings.ToLower(audit.RecommendationState)),
		})
	}
	c.JSON(http.StatusOK, ListVideoQualityRolloutEffectsResponse{Items: items})
}

func (h *Handler) loadVideoQualityRolloutAudits(limit int) ([]models.VideoQualityRolloutAudit, error) {
	if h == nil || h.db == nil || limit <= 0 {
		return nil, nil
	}
	var rows []models.VideoQualityRolloutAudit
	err := h.db.Order("id DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (h *Handler) loadVideoQualityRolloutEffectMetrics(start, end time.Time) (VideoQualityRolloutEffectMetric, error) {
	type jobsRow struct {
		JobsTotal  int64 `gorm:"column:jobs_total"`
		JobsDone   int64 `gorm:"column:jobs_done"`
		JobsFailed int64 `gorm:"column:jobs_failed"`
	}
	type outputsRow struct {
		OutputSamples     int64   `gorm:"column:output_samples"`
		AvgOutputScore    float64 `gorm:"column:avg_output_score"`
		AvgLoopClosure    float64 `gorm:"column:avg_loop_closure"`
		LoopEffectiveRate float64 `gorm:"column:loop_effective_rate"`
		LoopFallbackRate  float64 `gorm:"column:loop_fallback_rate"`
	}

	var jobs jobsRow
	gifTables := resolveVideoImageReadTables("gif")
	if err := h.db.Raw(fmt.Sprintf(`
SELECT
	COUNT(*) AS jobs_total,
	COUNT(*) FILTER (WHERE status = 'done') AS jobs_done,
	COUNT(*) FILTER (WHERE status = 'failed') AS jobs_failed
FROM %s
WHERE requested_format = 'gif'
	AND created_at >= ?
	AND created_at < ?
`, gifTables.Jobs), start, end).Scan(&jobs).Error; err != nil {
		return VideoQualityRolloutEffectMetric{}, err
	}

	var outputs outputsRow
	if err := h.db.Raw(fmt.Sprintf(`
SELECT
	COUNT(*) AS output_samples,
	COALESCE(AVG(score), 0) AS avg_output_score,
	COALESCE(AVG(gif_loop_tune_loop_closure), 0) AS avg_loop_closure,
	COALESCE(AVG(CASE WHEN gif_loop_tune_effective_applied THEN 1 ELSE 0 END), 0) AS loop_effective_rate,
	COALESCE(AVG(CASE WHEN gif_loop_tune_fallback_to_base THEN 1 ELSE 0 END), 0) AS loop_fallback_rate
FROM %s
WHERE format = 'gif'
	AND file_role = 'main'
	AND created_at >= ?
	AND created_at < ?
`, gifTables.Outputs), start, end).Scan(&outputs).Error; err != nil {
		return VideoQualityRolloutEffectMetric{}, err
	}

	out := VideoQualityRolloutEffectMetric{
		JobsTotal:         jobs.JobsTotal,
		JobsDone:          jobs.JobsDone,
		JobsFailed:        jobs.JobsFailed,
		OutputSamples:     outputs.OutputSamples,
		AvgOutputScore:    outputs.AvgOutputScore,
		AvgLoopClosure:    outputs.AvgLoopClosure,
		LoopEffectiveRate: outputs.LoopEffectiveRate,
		LoopFallbackRate:  outputs.LoopFallbackRate,
	}
	if out.JobsTotal > 0 {
		out.DoneRate = float64(out.JobsDone) / float64(out.JobsTotal)
		out.FailedRate = float64(out.JobsFailed) / float64(out.JobsTotal)
	}
	return out, nil
}

func evaluateVideoQualityRolloutEffect(base, target VideoQualityRolloutEffectMetric, delta VideoQualityRolloutEffectDelta) (string, string) {
	if base.JobsTotal < 8 || target.JobsTotal < 8 {
		return "insufficient_data", fmt.Sprintf("样本不足（base=%d, target=%d）", base.JobsTotal, target.JobsTotal)
	}

	score := 0
	positives := make([]string, 0, 4)
	negatives := make([]string, 0, 4)

	if delta.DoneRateDelta >= 0.03 {
		score++
		positives = append(positives, "完成率提升")
	} else if delta.DoneRateDelta <= -0.03 {
		score--
		negatives = append(negatives, "完成率下降")
	}

	if delta.FailedRateDelta <= -0.02 {
		score++
		positives = append(positives, "失败率下降")
	} else if delta.FailedRateDelta >= 0.02 {
		score--
		negatives = append(negatives, "失败率上升")
	}

	if base.OutputSamples >= 8 && target.OutputSamples >= 8 {
		if delta.AvgOutputScoreDelta >= 0.02 {
			score++
			positives = append(positives, "质量分提升")
		} else if delta.AvgOutputScoreDelta <= -0.02 {
			score--
			negatives = append(negatives, "质量分下降")
		}
		if delta.AvgLoopClosureDelta >= 0.02 {
			score++
			positives = append(positives, "Loop 闭环提升")
		} else if delta.AvgLoopClosureDelta <= -0.02 {
			score--
			negatives = append(negatives, "Loop 闭环下降")
		}
	}

	if score >= 2 {
		reason := "核心指标改善"
		if len(positives) > 0 {
			reason = strings.Join(positives, "、")
		}
		return "improved", reason
	}
	if score <= -2 {
		reason := "核心指标回退"
		if len(negatives) > 0 {
			reason = strings.Join(negatives, "、")
		}
		return "regressed", reason
	}
	if len(positives) > 0 && len(negatives) > 0 {
		return "neutral", fmt.Sprintf("有升有降：%s；%s", strings.Join(positives, "、"), strings.Join(negatives, "、"))
	}
	if len(positives) > 0 {
		return "neutral", fmt.Sprintf("轻微改善：%s", strings.Join(positives, "、"))
	}
	if len(negatives) > 0 {
		return "neutral", fmt.Sprintf("轻微回退：%s", strings.Join(negatives, "、"))
	}
	return "neutral", "指标波动不显著"
}

func shouldApplyFeedbackRolloutRecommendation(rec AdminVideoJobFeedbackRolloutRecommendation) bool {
	state := strings.ToLower(strings.TrimSpace(rec.State))
	if state != "scale_up" && state != "scale_down" {
		return false
	}
	if !rec.ConsecutivePassed {
		return false
	}
	return rec.SuggestedRolloutPercent != rec.CurrentRolloutPercent
}

func buildRolloutAuditMetadata(rec AdminVideoJobFeedbackRolloutRecommendation) (datatypes.JSON, error) {
	payload := map[string]interface{}{
		"state":                     rec.State,
		"reason":                    rec.Reason,
		"current_rollout_percent":   rec.CurrentRolloutPercent,
		"suggested_rollout_percent": rec.SuggestedRolloutPercent,
		"consecutive_required":      rec.ConsecutiveRequired,
		"consecutive_matched":       rec.ConsecutiveMatched,
		"consecutive_passed":        rec.ConsecutivePassed,
		"recent_states":             rec.RecentStates,
		"treatment_jobs":            rec.TreatmentJobs,
		"control_jobs":              rec.ControlJobs,
		"treatment_signals_per_job": rec.TreatmentSignalsPerJob,
		"control_signals_per_job":   rec.ControlSignalsPerJob,
		"signals_uplift":            rec.SignalsUplift,
		"treatment_avg_score":       rec.TreatmentAvgScore,
		"control_avg_score":         rec.ControlAvgScore,
		"score_uplift":              rec.ScoreUplift,
		"live_guard_triggered":      rec.LiveGuardTriggered,
		"live_guard_min_samples":    rec.LiveGuardMinSamples,
		"live_guard_eligible_total": rec.LiveGuardEligibleTotal,
		"live_guard_score_floor":    rec.LiveGuardScoreFloor,
		"live_guard_risk_scenes":    rec.LiveGuardRiskScenes,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(raw), nil
}

func rolloutCooldownState(now, lastAppliedAt time.Time, cooldown time.Duration) (remainingSeconds int, nextAllowedAt time.Time, inCooldown bool) {
	if cooldown <= 0 || lastAppliedAt.IsZero() {
		return 0, time.Time{}, false
	}
	nextAllowedAt = lastAppliedAt.Add(cooldown)
	if !now.Before(nextAllowedAt) {
		return 0, nextAllowedAt, false
	}
	remaining := nextAllowedAt.Sub(now)
	remainingSeconds = int(remaining.Seconds())
	if remainingSeconds < 1 {
		remainingSeconds = 1
	}
	if remaining > time.Duration(remainingSeconds)*time.Second {
		remainingSeconds++
	}
	return remainingSeconds, nextAllowedAt, true
}
