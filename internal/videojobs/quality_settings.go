package videojobs

import "strings"

const (
	QualityProfileClarity  = "clarity"
	QualityProfileSize     = "size"
	maxGIFCandidateOutputs = 20
)

type QualitySettings struct {
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
}

func DefaultQualitySettings() QualitySettings {
	return QualitySettings{
		MinBrightness:                        16,
		MaxBrightness:                        244,
		BlurThresholdFactor:                  0.22,
		BlurThresholdMin:                     12,
		BlurThresholdMax:                     120,
		DuplicateHammingThreshold:            5,
		DuplicateBacktrackFrames:             4,
		FallbackBlurRelaxFactor:              0.5,
		FallbackHammingThreshold:             1,
		MinKeepBase:                          6,
		MinKeepRatio:                         0.35,
		QualityAnalysisWorkers:               4,
		UploadConcurrency:                    4,
		GIFProfile:                           QualityProfileClarity,
		WebPProfile:                          QualityProfileClarity,
		LiveProfile:                          QualityProfileClarity,
		JPGProfile:                           QualityProfileClarity,
		PNGProfile:                           QualityProfileClarity,
		GIFDefaultFPS:                        12,
		GIFDefaultMaxColors:                  128,
		GIFDitherMode:                        "sierra2_4a",
		GIFTargetSizeKB:                      2048,
		GIFGifsicleEnabled:                   true,
		GIFGifsicleLevel:                     2,
		GIFGifsicleSkipBelowKB:               256,
		GIFGifsicleMinGainRatio:              0.03,
		GIFLoopTuneEnabled:                   true,
		GIFLoopTuneMinEnableSec:              1.4,
		GIFLoopTuneMinImprovement:            0.04,
		GIFLoopTuneMotionTarget:              0.22,
		GIFLoopTunePreferDuration:            2.4,
		GIFCandidateMaxOutputs:               3,
		GIFCandidateLongVideoMaxOutputs:      3,
		GIFCandidateUltraVideoMaxOutputs:     2,
		GIFCandidateConfidenceThreshold:      0.35,
		GIFCandidateDedupIOUThreshold:        0.45,
		GIFRenderBudgetNormalMultiplier:      1.8,
		GIFRenderBudgetLongMultiplier:        1.45,
		GIFRenderBudgetUltraMultiplier:       1.2,
		GIFPipelineShortVideoMaxSec:          18,
		GIFPipelineLongVideoMinSec:           180,
		GIFPipelineShortVideoMode:            "light",
		GIFPipelineDefaultMode:               "standard",
		GIFPipelineLongVideoMode:             "light",
		GIFPipelineHighPriorityEnabled:       true,
		GIFPipelineHighPriorityMode:          "hq",
		GIFDurationTierMediumSec:             60,
		GIFDurationTierLongSec:               120,
		GIFDurationTierUltraSec:              240,
		GIFSegmentTimeoutMinSec:              30,
		GIFSegmentTimeoutMaxSec:              120,
		GIFSegmentTimeoutFallbackCapSec:      60,
		GIFSegmentTimeoutEmergencyCapSec:     40,
		GIFSegmentTimeoutLastResortCapSec:    30,
		GIFRenderRetryMaxAttempts:            6,
		GIFRenderRetryPrimaryColorsFloor:     96,
		GIFRenderRetryPrimaryColorsStep:      32,
		GIFRenderRetryFPSFloor:               8,
		GIFRenderRetryFPSStep:                2,
		GIFRenderRetryWidthTrigger:           480,
		GIFRenderRetryWidthScale:             0.85,
		GIFRenderRetryWidthFloor:             360,
		GIFRenderRetrySecondaryColorsFloor:   48,
		GIFRenderRetrySecondaryColorsStep:    16,
		GIFRenderInitialSizeFPSCap:           10,
		GIFRenderInitialClarityFPSFloor:      12,
		GIFRenderInitialSizeColorsCap:        96,
		GIFRenderInitialClarityColorsFloor:   160,
		GIFMotionLowScoreThreshold:           0.30,
		GIFMotionHighScoreThreshold:          0.64,
		GIFMotionLowFPSDelta:                 -2,
		GIFMotionHighFPSDelta:                2,
		GIFAdaptiveFPSMin:                    6,
		GIFAdaptiveFPSMax:                    18,
		GIFWidthSizeLow:                      640,
		GIFWidthSizeMedium:                   720,
		GIFWidthSizeHigh:                     768,
		GIFWidthClarityLow:                   720,
		GIFWidthClarityMedium:                960,
		GIFWidthClarityHigh:                  1080,
		GIFColorsSizeLow:                     72,
		GIFColorsSizeMedium:                  96,
		GIFColorsSizeHigh:                    128,
		GIFColorsClarityLow:                  128,
		GIFColorsClarityMedium:               176,
		GIFColorsClarityHigh:                 224,
		GIFDurationLowSec:                    2.0,
		GIFDurationMediumSec:                 2.4,
		GIFDurationHighSec:                   2.8,
		GIFDurationSizeProfileMaxSec:         2.4,
		GIFDownshiftHighResLongSideThreshold: 1600,
		GIFDownshiftEarlyDurationSec:         45,
		GIFDownshiftEarlyLongSideThreshold:   1800,
		GIFDownshiftMediumFPSCap:             9,
		GIFDownshiftMediumWidthCap:           720,
		GIFDownshiftMediumColorsCap:          128,
		GIFDownshiftMediumDurationCapSec:     2.2,
		GIFDownshiftLongFPSCap:               8,
		GIFDownshiftLongWidthCap:             640,
		GIFDownshiftLongColorsCap:            96,
		GIFDownshiftLongDurationCapSec:       2.0,
		GIFDownshiftUltraFPSCap:              8,
		GIFDownshiftUltraWidthCap:            560,
		GIFDownshiftUltraColorsCap:           72,
		GIFDownshiftUltraDurationCapSec:      1.8,
		GIFDownshiftHighResFPSCap:            9,
		GIFDownshiftHighResWidthCap:          640,
		GIFDownshiftHighResColorsCap:         96,
		GIFDownshiftHighResDurationCapSec:    2.1,
		GIFTimeoutFallbackFPSCap:             10,
		GIFTimeoutFallbackWidthCap:           720,
		GIFTimeoutFallbackColorsCap:          96,
		GIFTimeoutFallbackMinWidth:           360,
		GIFTimeoutFallbackUltraFPSCap:        8,
		GIFTimeoutFallbackUltraWidthCap:      640,
		GIFTimeoutFallbackUltraColorsCap:     64,
		GIFTimeoutEmergencyFPSCap:            8,
		GIFTimeoutEmergencyWidthCap:          540,
		GIFTimeoutEmergencyColorsCap:         64,
		GIFTimeoutEmergencyMinWidth:          320,
		GIFTimeoutEmergencyDurationTrigger:   2.0,
		GIFTimeoutEmergencyDurationScale:     0.75,
		GIFTimeoutEmergencyDurationMinSec:    1.4,
		GIFTimeoutLastResortFPSCap:           6,
		GIFTimeoutLastResortWidthCap:         480,
		GIFTimeoutLastResortColorsCap:        48,
		GIFTimeoutLastResortMinWidth:         320,
		GIFTimeoutLastResortDurationMinSec:   1.2,
		GIFTimeoutLastResortDurationMaxSec:   1.8,
		WebPTargetSizeKB:                     1536,
		JPGTargetSizeKB:                      512,
		PNGTargetSizeKB:                      1024,
		StillMinBlurScore:                    12,
		StillMinExposureScore:                0.28,
		StillMinWidth:                        0,
		StillMinHeight:                       0,
		LiveCoverPortraitWeight:              0.04,
		LiveCoverSceneMinSamples:             5,
		LiveCoverGuardMinTotal:               20,
		LiveCoverGuardScoreFloor:             0.58,
		HighlightFeedbackEnabled:             true,
		HighlightFeedbackRollout:             100,
		HighlightFeedbackMinJobs:             2,
		HighlightFeedbackMinScore:            6,
		HighlightFeedbackBoost:               1,
		HighlightWeightPosition:              0.14,
		HighlightWeightDuration:              0.08,
		HighlightWeightReason:                0.08,
		HighlightNegativeGuardEnabled:        true,
		HighlightNegativeGuardThreshold:      0.45,
		HighlightNegativeGuardMinWeight:      4,
		HighlightNegativePenaltyScale:        0.55,
		HighlightNegativePenaltyWeight:       0.9,
		AIDirectorInputMode:                  "hybrid",
		AIDirectorOperatorInstruction:        "",
		AIDirectorOperatorInstructionVersion: "v1",
		AIDirectorOperatorEnabled:            true,
		AIDirectorConstraintOverrideEnabled:  false,
		AIDirectorCountExpandRatio:           0.20,
		AIDirectorDurationExpandRatio:        0.20,
		AIDirectorCountAbsoluteCap:           10,
		AIDirectorDurationAbsoluteCapSec:     6.0,
		GIFAIJudgeHardGateMinOverallScore:    0.20,
		GIFAIJudgeHardGateMinClarityScore:    0.20,
		GIFAIJudgeHardGateMinLoopScore:       0.20,
		GIFAIJudgeHardGateMinOutputScore:     0.20,
		GIFAIJudgeHardGateMinDurationMS:      200,
		GIFAIJudgeHardGateSizeMultiplier:     4,
	}
}

func NormalizeQualitySettings(in QualitySettings) QualitySettings {
	out := in
	def := DefaultQualitySettings()

	out.MinBrightness = clampFloat(out.MinBrightness, 0, 255)
	out.MaxBrightness = clampFloat(out.MaxBrightness, 0, 255)
	if out.MaxBrightness <= out.MinBrightness {
		out.MinBrightness = def.MinBrightness
		out.MaxBrightness = def.MaxBrightness
	}

	out.BlurThresholdFactor = clampFloat(out.BlurThresholdFactor, 0.05, 1.0)
	out.BlurThresholdMin = clampFloat(out.BlurThresholdMin, 0, 500)
	out.BlurThresholdMax = clampFloat(out.BlurThresholdMax, out.BlurThresholdMin, 1000)

	if out.DuplicateHammingThreshold < 0 {
		out.DuplicateHammingThreshold = 0
	}
	if out.DuplicateHammingThreshold > 32 {
		out.DuplicateHammingThreshold = 32
	}
	if out.DuplicateBacktrackFrames < 1 {
		out.DuplicateBacktrackFrames = 1
	}
	if out.DuplicateBacktrackFrames > 24 {
		out.DuplicateBacktrackFrames = 24
	}

	out.FallbackBlurRelaxFactor = clampFloat(out.FallbackBlurRelaxFactor, 0.1, 1.0)
	if out.FallbackHammingThreshold < 0 {
		out.FallbackHammingThreshold = 0
	}
	if out.FallbackHammingThreshold > 16 {
		out.FallbackHammingThreshold = 16
	}

	if out.MinKeepBase < 1 {
		out.MinKeepBase = 1
	}
	if out.MinKeepBase > 120 {
		out.MinKeepBase = 120
	}
	out.MinKeepRatio = clampFloat(out.MinKeepRatio, 0.05, 1.0)
	if out.QualityAnalysisWorkers < 1 {
		out.QualityAnalysisWorkers = def.QualityAnalysisWorkers
	}
	if out.QualityAnalysisWorkers > 32 {
		out.QualityAnalysisWorkers = 32
	}
	if out.UploadConcurrency < 1 {
		out.UploadConcurrency = def.UploadConcurrency
	}
	if out.UploadConcurrency > 32 {
		out.UploadConcurrency = 32
	}
	out.GIFProfile = normalizeQualityProfile(out.GIFProfile, def.GIFProfile)
	out.WebPProfile = normalizeQualityProfile(out.WebPProfile, def.WebPProfile)
	out.LiveProfile = normalizeQualityProfile(out.LiveProfile, def.LiveProfile)
	out.JPGProfile = normalizeQualityProfile(out.JPGProfile, def.JPGProfile)
	out.PNGProfile = normalizeQualityProfile(out.PNGProfile, def.PNGProfile)

	if out.GIFDefaultFPS < 4 {
		out.GIFDefaultFPS = 4
	}
	if out.GIFDefaultFPS > 30 {
		out.GIFDefaultFPS = 30
	}
	if out.GIFDefaultMaxColors < 16 {
		out.GIFDefaultMaxColors = 16
	}
	if out.GIFDefaultMaxColors > 256 {
		out.GIFDefaultMaxColors = 256
	}
	if out.GIFTargetSizeKB <= 0 {
		out.GIFTargetSizeKB = def.GIFTargetSizeKB
	}
	if out.GIFTargetSizeKB > 10240 {
		out.GIFTargetSizeKB = 10240
	}
	legacyZeroGIFGifsicle := !out.GIFGifsicleEnabled &&
		out.GIFGifsicleLevel == 0 &&
		out.GIFGifsicleSkipBelowKB == 0 &&
		out.GIFGifsicleMinGainRatio == 0
	if legacyZeroGIFGifsicle {
		out.GIFGifsicleEnabled = def.GIFGifsicleEnabled
		out.GIFGifsicleLevel = def.GIFGifsicleLevel
		out.GIFGifsicleSkipBelowKB = def.GIFGifsicleSkipBelowKB
		out.GIFGifsicleMinGainRatio = def.GIFGifsicleMinGainRatio
	} else {
		if out.GIFGifsicleLevel <= 0 {
			out.GIFGifsicleLevel = def.GIFGifsicleLevel
		}
		if out.GIFGifsicleLevel > 3 {
			out.GIFGifsicleLevel = 3
		}
		if out.GIFGifsicleSkipBelowKB < 0 {
			out.GIFGifsicleSkipBelowKB = 0
		}
		if out.GIFGifsicleSkipBelowKB > 4096 {
			out.GIFGifsicleSkipBelowKB = 4096
		}
		if out.GIFGifsicleMinGainRatio < 0 {
			out.GIFGifsicleMinGainRatio = 0
		}
		out.GIFGifsicleMinGainRatio = clampFloat(out.GIFGifsicleMinGainRatio, 0, 0.5)
	}
	legacyZeroGIFFallback := out.GIFLoopTuneMinEnableSec == 0 &&
		out.GIFLoopTuneMinImprovement == 0 &&
		out.GIFLoopTuneMotionTarget == 0 &&
		out.GIFLoopTunePreferDuration == 0
	if legacyZeroGIFFallback {
		// Backward compatibility: old rows (or zero-value structs) do not carry GIF loop tuning fields.
		out.GIFLoopTuneEnabled = def.GIFLoopTuneEnabled
		out.GIFLoopTuneMinEnableSec = def.GIFLoopTuneMinEnableSec
		out.GIFLoopTuneMinImprovement = def.GIFLoopTuneMinImprovement
		out.GIFLoopTuneMotionTarget = def.GIFLoopTuneMotionTarget
		out.GIFLoopTunePreferDuration = def.GIFLoopTunePreferDuration
	} else {
		if out.GIFLoopTuneMinEnableSec <= 0 {
			out.GIFLoopTuneMinEnableSec = def.GIFLoopTuneMinEnableSec
		}
		out.GIFLoopTuneMinEnableSec = clampFloat(out.GIFLoopTuneMinEnableSec, 0.8, 4.0)
		if out.GIFLoopTuneMinImprovement <= 0 {
			out.GIFLoopTuneMinImprovement = def.GIFLoopTuneMinImprovement
		}
		out.GIFLoopTuneMinImprovement = clampFloat(out.GIFLoopTuneMinImprovement, 0.005, 0.3)
		if out.GIFLoopTuneMotionTarget <= 0 {
			out.GIFLoopTuneMotionTarget = def.GIFLoopTuneMotionTarget
		}
		out.GIFLoopTuneMotionTarget = clampFloat(out.GIFLoopTuneMotionTarget, 0.05, 0.8)
		if out.GIFLoopTunePreferDuration <= 0 {
			out.GIFLoopTunePreferDuration = def.GIFLoopTunePreferDuration
		}
		out.GIFLoopTunePreferDuration = clampFloat(out.GIFLoopTunePreferDuration, 1.0, 4.0)
	}

	legacyZeroGIFCandidate := out.GIFCandidateMaxOutputs == 0 &&
		out.GIFCandidateLongVideoMaxOutputs == 0 &&
		out.GIFCandidateUltraVideoMaxOutputs == 0 &&
		out.GIFCandidateConfidenceThreshold == 0 &&
		out.GIFCandidateDedupIOUThreshold == 0 &&
		out.GIFRenderBudgetNormalMultiplier == 0 &&
		out.GIFRenderBudgetLongMultiplier == 0 &&
		out.GIFRenderBudgetUltraMultiplier == 0
	if legacyZeroGIFCandidate {
		// Backward compatibility: old rows (or zero-value structs) do not carry GIF candidate fields.
		out.GIFCandidateMaxOutputs = def.GIFCandidateMaxOutputs
		out.GIFCandidateLongVideoMaxOutputs = def.GIFCandidateLongVideoMaxOutputs
		out.GIFCandidateUltraVideoMaxOutputs = def.GIFCandidateUltraVideoMaxOutputs
		out.GIFCandidateConfidenceThreshold = def.GIFCandidateConfidenceThreshold
		out.GIFCandidateDedupIOUThreshold = def.GIFCandidateDedupIOUThreshold
		out.GIFRenderBudgetNormalMultiplier = def.GIFRenderBudgetNormalMultiplier
		out.GIFRenderBudgetLongMultiplier = def.GIFRenderBudgetLongMultiplier
		out.GIFRenderBudgetUltraMultiplier = def.GIFRenderBudgetUltraMultiplier
	} else {
		if out.GIFCandidateMaxOutputs <= 0 {
			out.GIFCandidateMaxOutputs = def.GIFCandidateMaxOutputs
		}
		if out.GIFCandidateMaxOutputs > maxGIFCandidateOutputs {
			out.GIFCandidateMaxOutputs = maxGIFCandidateOutputs
		}
		if out.GIFCandidateLongVideoMaxOutputs <= 0 {
			out.GIFCandidateLongVideoMaxOutputs = def.GIFCandidateLongVideoMaxOutputs
		}
		if out.GIFCandidateLongVideoMaxOutputs > maxGIFCandidateOutputs {
			out.GIFCandidateLongVideoMaxOutputs = maxGIFCandidateOutputs
		}
		if out.GIFCandidateUltraVideoMaxOutputs <= 0 {
			out.GIFCandidateUltraVideoMaxOutputs = def.GIFCandidateUltraVideoMaxOutputs
		}
		if out.GIFCandidateUltraVideoMaxOutputs > maxGIFCandidateOutputs {
			out.GIFCandidateUltraVideoMaxOutputs = maxGIFCandidateOutputs
		}
		if out.GIFCandidateLongVideoMaxOutputs > out.GIFCandidateMaxOutputs {
			out.GIFCandidateLongVideoMaxOutputs = out.GIFCandidateMaxOutputs
		}
		if out.GIFCandidateUltraVideoMaxOutputs > out.GIFCandidateLongVideoMaxOutputs {
			out.GIFCandidateUltraVideoMaxOutputs = out.GIFCandidateLongVideoMaxOutputs
		}
		if out.GIFCandidateConfidenceThreshold < 0 {
			out.GIFCandidateConfidenceThreshold = 0
		}
		out.GIFCandidateConfidenceThreshold = clampFloat(out.GIFCandidateConfidenceThreshold, 0, 0.95)
		if out.GIFCandidateDedupIOUThreshold <= 0 {
			out.GIFCandidateDedupIOUThreshold = def.GIFCandidateDedupIOUThreshold
		}
		out.GIFCandidateDedupIOUThreshold = clampFloat(out.GIFCandidateDedupIOUThreshold, 0.1, 0.95)
		if out.GIFRenderBudgetNormalMultiplier <= 0 {
			out.GIFRenderBudgetNormalMultiplier = def.GIFRenderBudgetNormalMultiplier
		}
		out.GIFRenderBudgetNormalMultiplier = clampFloat(out.GIFRenderBudgetNormalMultiplier, 1.0, 4.0)
		if out.GIFRenderBudgetLongMultiplier <= 0 {
			out.GIFRenderBudgetLongMultiplier = def.GIFRenderBudgetLongMultiplier
		}
		out.GIFRenderBudgetLongMultiplier = clampFloat(out.GIFRenderBudgetLongMultiplier, 0.8, 4.0)
		if out.GIFRenderBudgetUltraMultiplier <= 0 {
			out.GIFRenderBudgetUltraMultiplier = def.GIFRenderBudgetUltraMultiplier
		}
		out.GIFRenderBudgetUltraMultiplier = clampFloat(out.GIFRenderBudgetUltraMultiplier, 0.5, 4.0)
		if out.GIFRenderBudgetLongMultiplier > out.GIFRenderBudgetNormalMultiplier {
			out.GIFRenderBudgetLongMultiplier = out.GIFRenderBudgetNormalMultiplier
		}
		if out.GIFRenderBudgetUltraMultiplier > out.GIFRenderBudgetLongMultiplier {
			out.GIFRenderBudgetUltraMultiplier = out.GIFRenderBudgetLongMultiplier
		}
	}
	legacyZeroPipeline := out.GIFPipelineShortVideoMaxSec == 0 &&
		out.GIFPipelineLongVideoMinSec == 0 &&
		strings.TrimSpace(out.GIFPipelineShortVideoMode) == "" &&
		strings.TrimSpace(out.GIFPipelineDefaultMode) == "" &&
		strings.TrimSpace(out.GIFPipelineLongVideoMode) == "" &&
		strings.TrimSpace(out.GIFPipelineHighPriorityMode) == ""
	if legacyZeroPipeline {
		out.GIFPipelineShortVideoMaxSec = def.GIFPipelineShortVideoMaxSec
		out.GIFPipelineLongVideoMinSec = def.GIFPipelineLongVideoMinSec
		out.GIFPipelineShortVideoMode = def.GIFPipelineShortVideoMode
		out.GIFPipelineDefaultMode = def.GIFPipelineDefaultMode
		out.GIFPipelineLongVideoMode = def.GIFPipelineLongVideoMode
		out.GIFPipelineHighPriorityEnabled = def.GIFPipelineHighPriorityEnabled
		out.GIFPipelineHighPriorityMode = def.GIFPipelineHighPriorityMode
	} else {
		out.GIFPipelineShortVideoMaxSec = clampFloat(out.GIFPipelineShortVideoMaxSec, 3, 300)
		if out.GIFPipelineLongVideoMinSec <= out.GIFPipelineShortVideoMaxSec {
			out.GIFPipelineLongVideoMinSec = maxFloat(out.GIFPipelineShortVideoMaxSec+1, def.GIFPipelineLongVideoMinSec)
		}
		out.GIFPipelineLongVideoMinSec = clampFloat(out.GIFPipelineLongVideoMinSec, out.GIFPipelineShortVideoMaxSec+1, 3600)

		out.GIFPipelineShortVideoMode = normalizeGIFPipelineModeSetting(out.GIFPipelineShortVideoMode, def.GIFPipelineShortVideoMode)
		out.GIFPipelineDefaultMode = normalizeGIFPipelineModeSetting(out.GIFPipelineDefaultMode, def.GIFPipelineDefaultMode)
		out.GIFPipelineLongVideoMode = normalizeGIFPipelineModeSetting(out.GIFPipelineLongVideoMode, def.GIFPipelineLongVideoMode)
		out.GIFPipelineHighPriorityMode = normalizeGIFPipelineModeSetting(out.GIFPipelineHighPriorityMode, def.GIFPipelineHighPriorityMode)
	}

	legacyZeroDurationTier := out.GIFDurationTierMediumSec == 0 &&
		out.GIFDurationTierLongSec == 0 &&
		out.GIFDurationTierUltraSec == 0
	if legacyZeroDurationTier {
		out.GIFDurationTierMediumSec = def.GIFDurationTierMediumSec
		out.GIFDurationTierLongSec = def.GIFDurationTierLongSec
		out.GIFDurationTierUltraSec = def.GIFDurationTierUltraSec
	} else {
		out.GIFDurationTierMediumSec = clampFloat(out.GIFDurationTierMediumSec, 10, 600)
		if out.GIFDurationTierLongSec <= out.GIFDurationTierMediumSec {
			out.GIFDurationTierLongSec = maxFloat(out.GIFDurationTierMediumSec+1, def.GIFDurationTierLongSec)
		}
		out.GIFDurationTierLongSec = clampFloat(out.GIFDurationTierLongSec, out.GIFDurationTierMediumSec+1, 1800)
		if out.GIFDurationTierUltraSec <= out.GIFDurationTierLongSec {
			out.GIFDurationTierUltraSec = maxFloat(out.GIFDurationTierLongSec+1, def.GIFDurationTierUltraSec)
		}
		out.GIFDurationTierUltraSec = clampFloat(out.GIFDurationTierUltraSec, out.GIFDurationTierLongSec+1, 7200)
	}

	legacyZeroRenderTimeout := out.GIFSegmentTimeoutMinSec == 0 &&
		out.GIFSegmentTimeoutMaxSec == 0 &&
		out.GIFSegmentTimeoutFallbackCapSec == 0 &&
		out.GIFSegmentTimeoutEmergencyCapSec == 0 &&
		out.GIFSegmentTimeoutLastResortCapSec == 0
	if legacyZeroRenderTimeout {
		out.GIFSegmentTimeoutMinSec = def.GIFSegmentTimeoutMinSec
		out.GIFSegmentTimeoutMaxSec = def.GIFSegmentTimeoutMaxSec
		out.GIFSegmentTimeoutFallbackCapSec = def.GIFSegmentTimeoutFallbackCapSec
		out.GIFSegmentTimeoutEmergencyCapSec = def.GIFSegmentTimeoutEmergencyCapSec
		out.GIFSegmentTimeoutLastResortCapSec = def.GIFSegmentTimeoutLastResortCapSec
	} else {
		if out.GIFSegmentTimeoutMinSec < 10 {
			out.GIFSegmentTimeoutMinSec = 10
		}
		if out.GIFSegmentTimeoutMinSec > 300 {
			out.GIFSegmentTimeoutMinSec = 300
		}
		if out.GIFSegmentTimeoutMaxSec < out.GIFSegmentTimeoutMinSec {
			out.GIFSegmentTimeoutMaxSec = out.GIFSegmentTimeoutMinSec
		}
		if out.GIFSegmentTimeoutMaxSec > 600 {
			out.GIFSegmentTimeoutMaxSec = 600
		}
		if out.GIFSegmentTimeoutFallbackCapSec < out.GIFSegmentTimeoutMinSec {
			out.GIFSegmentTimeoutFallbackCapSec = out.GIFSegmentTimeoutMinSec
		}
		if out.GIFSegmentTimeoutFallbackCapSec > out.GIFSegmentTimeoutMaxSec {
			out.GIFSegmentTimeoutFallbackCapSec = out.GIFSegmentTimeoutMaxSec
		}
		if out.GIFSegmentTimeoutEmergencyCapSec < out.GIFSegmentTimeoutMinSec {
			out.GIFSegmentTimeoutEmergencyCapSec = out.GIFSegmentTimeoutMinSec
		}
		if out.GIFSegmentTimeoutEmergencyCapSec > out.GIFSegmentTimeoutFallbackCapSec {
			out.GIFSegmentTimeoutEmergencyCapSec = out.GIFSegmentTimeoutFallbackCapSec
		}
		if out.GIFSegmentTimeoutLastResortCapSec < out.GIFSegmentTimeoutMinSec {
			out.GIFSegmentTimeoutLastResortCapSec = out.GIFSegmentTimeoutMinSec
		}
		if out.GIFSegmentTimeoutLastResortCapSec > out.GIFSegmentTimeoutEmergencyCapSec {
			out.GIFSegmentTimeoutLastResortCapSec = out.GIFSegmentTimeoutEmergencyCapSec
		}
	}
	legacyZeroRenderRetry := out.GIFRenderRetryMaxAttempts == 0 &&
		out.GIFRenderRetryPrimaryColorsFloor == 0 &&
		out.GIFRenderRetryFPSFloor == 0 &&
		out.GIFRenderRetryWidthTrigger == 0
	if legacyZeroRenderRetry {
		out.GIFRenderRetryMaxAttempts = def.GIFRenderRetryMaxAttempts
		out.GIFRenderRetryPrimaryColorsFloor = def.GIFRenderRetryPrimaryColorsFloor
		out.GIFRenderRetryPrimaryColorsStep = def.GIFRenderRetryPrimaryColorsStep
		out.GIFRenderRetryFPSFloor = def.GIFRenderRetryFPSFloor
		out.GIFRenderRetryFPSStep = def.GIFRenderRetryFPSStep
		out.GIFRenderRetryWidthTrigger = def.GIFRenderRetryWidthTrigger
		out.GIFRenderRetryWidthScale = def.GIFRenderRetryWidthScale
		out.GIFRenderRetryWidthFloor = def.GIFRenderRetryWidthFloor
		out.GIFRenderRetrySecondaryColorsFloor = def.GIFRenderRetrySecondaryColorsFloor
		out.GIFRenderRetrySecondaryColorsStep = def.GIFRenderRetrySecondaryColorsStep
		out.GIFRenderInitialSizeFPSCap = def.GIFRenderInitialSizeFPSCap
		out.GIFRenderInitialClarityFPSFloor = def.GIFRenderInitialClarityFPSFloor
		out.GIFRenderInitialSizeColorsCap = def.GIFRenderInitialSizeColorsCap
		out.GIFRenderInitialClarityColorsFloor = def.GIFRenderInitialClarityColorsFloor
	} else {
		out.GIFRenderRetryMaxAttempts = int(clampFloat(float64(out.GIFRenderRetryMaxAttempts), 1, 12))
		out.GIFRenderRetryPrimaryColorsFloor = int(clampFloat(float64(out.GIFRenderRetryPrimaryColorsFloor), 16, 256))
		out.GIFRenderRetryPrimaryColorsStep = int(clampFloat(float64(out.GIFRenderRetryPrimaryColorsStep), 1, 128))
		out.GIFRenderRetryFPSFloor = int(clampFloat(float64(out.GIFRenderRetryFPSFloor), 2, 30))
		out.GIFRenderRetryFPSStep = int(clampFloat(float64(out.GIFRenderRetryFPSStep), 1, 12))
		out.GIFRenderRetryWidthTrigger = int(clampFloat(float64(out.GIFRenderRetryWidthTrigger), 240, 2048))
		out.GIFRenderRetryWidthScale = clampFloat(out.GIFRenderRetryWidthScale, 0.5, 0.98)
		out.GIFRenderRetryWidthFloor = int(clampFloat(float64(out.GIFRenderRetryWidthFloor), 240, 1920))
		out.GIFRenderRetrySecondaryColorsFloor = int(clampFloat(float64(out.GIFRenderRetrySecondaryColorsFloor), 16, float64(out.GIFRenderRetryPrimaryColorsFloor)))
		out.GIFRenderRetrySecondaryColorsStep = int(clampFloat(float64(out.GIFRenderRetrySecondaryColorsStep), 1, 128))
		out.GIFRenderInitialSizeFPSCap = int(clampFloat(float64(out.GIFRenderInitialSizeFPSCap), 2, 30))
		out.GIFRenderInitialClarityFPSFloor = int(clampFloat(float64(out.GIFRenderInitialClarityFPSFloor), float64(out.GIFRenderInitialSizeFPSCap), 30))
		out.GIFRenderInitialSizeColorsCap = int(clampFloat(float64(out.GIFRenderInitialSizeColorsCap), 16, 256))
		out.GIFRenderInitialClarityColorsFloor = int(clampFloat(float64(out.GIFRenderInitialClarityColorsFloor), float64(out.GIFRenderInitialSizeColorsCap), 256))
		if out.GIFRenderRetryWidthFloor > out.GIFRenderRetryWidthTrigger {
			out.GIFRenderRetryWidthFloor = out.GIFRenderRetryWidthTrigger
		}
	}
	legacyZeroAdaptiveProfile := out.GIFMotionLowScoreThreshold == 0 &&
		out.GIFMotionHighScoreThreshold == 0 &&
		out.GIFAdaptiveFPSMin == 0 &&
		out.GIFAdaptiveFPSMax == 0 &&
		out.GIFWidthClarityMedium == 0 &&
		out.GIFColorsClarityMedium == 0 &&
		out.GIFDurationMediumSec == 0
	if legacyZeroAdaptiveProfile {
		out.GIFMotionLowScoreThreshold = def.GIFMotionLowScoreThreshold
		out.GIFMotionHighScoreThreshold = def.GIFMotionHighScoreThreshold
		out.GIFMotionLowFPSDelta = def.GIFMotionLowFPSDelta
		out.GIFMotionHighFPSDelta = def.GIFMotionHighFPSDelta
		out.GIFAdaptiveFPSMin = def.GIFAdaptiveFPSMin
		out.GIFAdaptiveFPSMax = def.GIFAdaptiveFPSMax
		out.GIFWidthSizeLow = def.GIFWidthSizeLow
		out.GIFWidthSizeMedium = def.GIFWidthSizeMedium
		out.GIFWidthSizeHigh = def.GIFWidthSizeHigh
		out.GIFWidthClarityLow = def.GIFWidthClarityLow
		out.GIFWidthClarityMedium = def.GIFWidthClarityMedium
		out.GIFWidthClarityHigh = def.GIFWidthClarityHigh
		out.GIFColorsSizeLow = def.GIFColorsSizeLow
		out.GIFColorsSizeMedium = def.GIFColorsSizeMedium
		out.GIFColorsSizeHigh = def.GIFColorsSizeHigh
		out.GIFColorsClarityLow = def.GIFColorsClarityLow
		out.GIFColorsClarityMedium = def.GIFColorsClarityMedium
		out.GIFColorsClarityHigh = def.GIFColorsClarityHigh
		out.GIFDurationLowSec = def.GIFDurationLowSec
		out.GIFDurationMediumSec = def.GIFDurationMediumSec
		out.GIFDurationHighSec = def.GIFDurationHighSec
		out.GIFDurationSizeProfileMaxSec = def.GIFDurationSizeProfileMaxSec
	} else {
		out.GIFMotionLowScoreThreshold = clampFloat(out.GIFMotionLowScoreThreshold, 0, 0.95)
		out.GIFMotionHighScoreThreshold = clampFloat(out.GIFMotionHighScoreThreshold, out.GIFMotionLowScoreThreshold+0.01, 1.0)
		out.GIFMotionLowFPSDelta = int(clampFloat(float64(out.GIFMotionLowFPSDelta), -12, 0))
		out.GIFMotionHighFPSDelta = int(clampFloat(float64(out.GIFMotionHighFPSDelta), 0, 12))
		out.GIFAdaptiveFPSMin = int(clampFloat(float64(out.GIFAdaptiveFPSMin), 2, 30))
		out.GIFAdaptiveFPSMax = int(clampFloat(float64(out.GIFAdaptiveFPSMax), float64(out.GIFAdaptiveFPSMin), 60))

		out.GIFWidthSizeLow = int(clampFloat(float64(out.GIFWidthSizeLow), 320, 1920))
		out.GIFWidthSizeMedium = int(clampFloat(float64(out.GIFWidthSizeMedium), float64(out.GIFWidthSizeLow), 1920))
		out.GIFWidthSizeHigh = int(clampFloat(float64(out.GIFWidthSizeHigh), float64(out.GIFWidthSizeMedium), 1920))
		out.GIFWidthClarityLow = int(clampFloat(float64(out.GIFWidthClarityLow), 320, 1920))
		out.GIFWidthClarityMedium = int(clampFloat(float64(out.GIFWidthClarityMedium), float64(out.GIFWidthClarityLow), 1920))
		out.GIFWidthClarityHigh = int(clampFloat(float64(out.GIFWidthClarityHigh), float64(out.GIFWidthClarityMedium), 1920))

		out.GIFColorsSizeLow = int(clampFloat(float64(out.GIFColorsSizeLow), 16, 256))
		out.GIFColorsSizeMedium = int(clampFloat(float64(out.GIFColorsSizeMedium), float64(out.GIFColorsSizeLow), 256))
		out.GIFColorsSizeHigh = int(clampFloat(float64(out.GIFColorsSizeHigh), float64(out.GIFColorsSizeMedium), 256))
		out.GIFColorsClarityLow = int(clampFloat(float64(out.GIFColorsClarityLow), 16, 256))
		out.GIFColorsClarityMedium = int(clampFloat(float64(out.GIFColorsClarityMedium), float64(out.GIFColorsClarityLow), 256))
		out.GIFColorsClarityHigh = int(clampFloat(float64(out.GIFColorsClarityHigh), float64(out.GIFColorsClarityMedium), 256))

		out.GIFDurationLowSec = clampFloat(out.GIFDurationLowSec, 0.8, 6.0)
		out.GIFDurationMediumSec = clampFloat(out.GIFDurationMediumSec, out.GIFDurationLowSec, 6.0)
		out.GIFDurationHighSec = clampFloat(out.GIFDurationHighSec, out.GIFDurationMediumSec, 6.0)
		out.GIFDurationSizeProfileMaxSec = clampFloat(out.GIFDurationSizeProfileMaxSec, out.GIFDurationLowSec, out.GIFDurationHighSec)
	}
	legacyZeroDownshift := out.GIFDownshiftHighResLongSideThreshold == 0 &&
		out.GIFDownshiftEarlyDurationSec == 0 &&
		out.GIFDownshiftEarlyLongSideThreshold == 0 &&
		out.GIFDownshiftMediumFPSCap == 0 &&
		out.GIFDownshiftMediumWidthCap == 0 &&
		out.GIFDownshiftMediumColorsCap == 0 &&
		out.GIFDownshiftMediumDurationCapSec == 0
	if legacyZeroDownshift {
		out.GIFDownshiftHighResLongSideThreshold = def.GIFDownshiftHighResLongSideThreshold
		out.GIFDownshiftEarlyDurationSec = def.GIFDownshiftEarlyDurationSec
		out.GIFDownshiftEarlyLongSideThreshold = def.GIFDownshiftEarlyLongSideThreshold
		out.GIFDownshiftMediumFPSCap = def.GIFDownshiftMediumFPSCap
		out.GIFDownshiftMediumWidthCap = def.GIFDownshiftMediumWidthCap
		out.GIFDownshiftMediumColorsCap = def.GIFDownshiftMediumColorsCap
		out.GIFDownshiftMediumDurationCapSec = def.GIFDownshiftMediumDurationCapSec
		out.GIFDownshiftLongFPSCap = def.GIFDownshiftLongFPSCap
		out.GIFDownshiftLongWidthCap = def.GIFDownshiftLongWidthCap
		out.GIFDownshiftLongColorsCap = def.GIFDownshiftLongColorsCap
		out.GIFDownshiftLongDurationCapSec = def.GIFDownshiftLongDurationCapSec
		out.GIFDownshiftUltraFPSCap = def.GIFDownshiftUltraFPSCap
		out.GIFDownshiftUltraWidthCap = def.GIFDownshiftUltraWidthCap
		out.GIFDownshiftUltraColorsCap = def.GIFDownshiftUltraColorsCap
		out.GIFDownshiftUltraDurationCapSec = def.GIFDownshiftUltraDurationCapSec
		out.GIFDownshiftHighResFPSCap = def.GIFDownshiftHighResFPSCap
		out.GIFDownshiftHighResWidthCap = def.GIFDownshiftHighResWidthCap
		out.GIFDownshiftHighResColorsCap = def.GIFDownshiftHighResColorsCap
		out.GIFDownshiftHighResDurationCapSec = def.GIFDownshiftHighResDurationCapSec
	} else {
		if out.GIFDownshiftHighResLongSideThreshold < 720 {
			out.GIFDownshiftHighResLongSideThreshold = 720
		}
		if out.GIFDownshiftHighResLongSideThreshold > 4096 {
			out.GIFDownshiftHighResLongSideThreshold = 4096
		}
		out.GIFDownshiftEarlyDurationSec = clampFloat(out.GIFDownshiftEarlyDurationSec, 10, 300)
		if out.GIFDownshiftEarlyLongSideThreshold < out.GIFDownshiftHighResLongSideThreshold {
			out.GIFDownshiftEarlyLongSideThreshold = out.GIFDownshiftHighResLongSideThreshold
		}
		if out.GIFDownshiftEarlyLongSideThreshold > 4096 {
			out.GIFDownshiftEarlyLongSideThreshold = 4096
		}
		out.GIFDownshiftMediumFPSCap = int(clampFloat(float64(out.GIFDownshiftMediumFPSCap), 4, 30))
		out.GIFDownshiftMediumWidthCap = int(clampFloat(float64(out.GIFDownshiftMediumWidthCap), 320, 1920))
		out.GIFDownshiftMediumColorsCap = int(clampFloat(float64(out.GIFDownshiftMediumColorsCap), 16, 256))
		out.GIFDownshiftMediumDurationCapSec = clampFloat(out.GIFDownshiftMediumDurationCapSec, 1.0, 6.0)
		out.GIFDownshiftLongFPSCap = int(clampFloat(float64(out.GIFDownshiftLongFPSCap), 4, float64(out.GIFDownshiftMediumFPSCap)))
		out.GIFDownshiftLongWidthCap = int(clampFloat(float64(out.GIFDownshiftLongWidthCap), 320, float64(out.GIFDownshiftMediumWidthCap)))
		out.GIFDownshiftLongColorsCap = int(clampFloat(float64(out.GIFDownshiftLongColorsCap), 16, float64(out.GIFDownshiftMediumColorsCap)))
		out.GIFDownshiftLongDurationCapSec = clampFloat(out.GIFDownshiftLongDurationCapSec, 0.8, out.GIFDownshiftMediumDurationCapSec)
		out.GIFDownshiftUltraFPSCap = int(clampFloat(float64(out.GIFDownshiftUltraFPSCap), 4, float64(out.GIFDownshiftLongFPSCap)))
		out.GIFDownshiftUltraWidthCap = int(clampFloat(float64(out.GIFDownshiftUltraWidthCap), 320, float64(out.GIFDownshiftLongWidthCap)))
		out.GIFDownshiftUltraColorsCap = int(clampFloat(float64(out.GIFDownshiftUltraColorsCap), 16, float64(out.GIFDownshiftLongColorsCap)))
		out.GIFDownshiftUltraDurationCapSec = clampFloat(out.GIFDownshiftUltraDurationCapSec, 0.8, out.GIFDownshiftLongDurationCapSec)
		out.GIFDownshiftHighResFPSCap = int(clampFloat(float64(out.GIFDownshiftHighResFPSCap), 4, float64(out.GIFDownshiftMediumFPSCap)))
		out.GIFDownshiftHighResWidthCap = int(clampFloat(float64(out.GIFDownshiftHighResWidthCap), 320, float64(out.GIFDownshiftMediumWidthCap)))
		out.GIFDownshiftHighResColorsCap = int(clampFloat(float64(out.GIFDownshiftHighResColorsCap), 16, float64(out.GIFDownshiftMediumColorsCap)))
		out.GIFDownshiftHighResDurationCapSec = clampFloat(out.GIFDownshiftHighResDurationCapSec, 0.8, out.GIFDownshiftMediumDurationCapSec)
	}

	legacyZeroTimeoutProfiles := out.GIFTimeoutFallbackFPSCap == 0 &&
		out.GIFTimeoutEmergencyFPSCap == 0 &&
		out.GIFTimeoutLastResortFPSCap == 0
	if legacyZeroTimeoutProfiles {
		out.GIFTimeoutFallbackFPSCap = def.GIFTimeoutFallbackFPSCap
		out.GIFTimeoutFallbackWidthCap = def.GIFTimeoutFallbackWidthCap
		out.GIFTimeoutFallbackColorsCap = def.GIFTimeoutFallbackColorsCap
		out.GIFTimeoutFallbackMinWidth = def.GIFTimeoutFallbackMinWidth
		out.GIFTimeoutFallbackUltraFPSCap = def.GIFTimeoutFallbackUltraFPSCap
		out.GIFTimeoutFallbackUltraWidthCap = def.GIFTimeoutFallbackUltraWidthCap
		out.GIFTimeoutFallbackUltraColorsCap = def.GIFTimeoutFallbackUltraColorsCap
		out.GIFTimeoutEmergencyFPSCap = def.GIFTimeoutEmergencyFPSCap
		out.GIFTimeoutEmergencyWidthCap = def.GIFTimeoutEmergencyWidthCap
		out.GIFTimeoutEmergencyColorsCap = def.GIFTimeoutEmergencyColorsCap
		out.GIFTimeoutEmergencyMinWidth = def.GIFTimeoutEmergencyMinWidth
		out.GIFTimeoutEmergencyDurationTrigger = def.GIFTimeoutEmergencyDurationTrigger
		out.GIFTimeoutEmergencyDurationScale = def.GIFTimeoutEmergencyDurationScale
		out.GIFTimeoutEmergencyDurationMinSec = def.GIFTimeoutEmergencyDurationMinSec
		out.GIFTimeoutLastResortFPSCap = def.GIFTimeoutLastResortFPSCap
		out.GIFTimeoutLastResortWidthCap = def.GIFTimeoutLastResortWidthCap
		out.GIFTimeoutLastResortColorsCap = def.GIFTimeoutLastResortColorsCap
		out.GIFTimeoutLastResortMinWidth = def.GIFTimeoutLastResortMinWidth
		out.GIFTimeoutLastResortDurationMinSec = def.GIFTimeoutLastResortDurationMinSec
		out.GIFTimeoutLastResortDurationMaxSec = def.GIFTimeoutLastResortDurationMaxSec
	} else {
		out.GIFTimeoutFallbackFPSCap = int(clampFloat(float64(out.GIFTimeoutFallbackFPSCap), 4, 30))
		out.GIFTimeoutFallbackWidthCap = int(clampFloat(float64(out.GIFTimeoutFallbackWidthCap), 320, 1920))
		out.GIFTimeoutFallbackColorsCap = int(clampFloat(float64(out.GIFTimeoutFallbackColorsCap), 16, 256))
		out.GIFTimeoutFallbackMinWidth = int(clampFloat(float64(out.GIFTimeoutFallbackMinWidth), 240, float64(out.GIFTimeoutFallbackWidthCap)))
		out.GIFTimeoutFallbackUltraFPSCap = int(clampFloat(float64(out.GIFTimeoutFallbackUltraFPSCap), 4, float64(out.GIFTimeoutFallbackFPSCap)))
		out.GIFTimeoutFallbackUltraWidthCap = int(clampFloat(float64(out.GIFTimeoutFallbackUltraWidthCap), float64(out.GIFTimeoutFallbackMinWidth), float64(out.GIFTimeoutFallbackWidthCap)))
		out.GIFTimeoutFallbackUltraColorsCap = int(clampFloat(float64(out.GIFTimeoutFallbackUltraColorsCap), 16, float64(out.GIFTimeoutFallbackColorsCap)))
		out.GIFTimeoutEmergencyFPSCap = int(clampFloat(float64(out.GIFTimeoutEmergencyFPSCap), 4, float64(out.GIFTimeoutFallbackFPSCap)))
		out.GIFTimeoutEmergencyWidthCap = int(clampFloat(float64(out.GIFTimeoutEmergencyWidthCap), 240, float64(out.GIFTimeoutFallbackWidthCap)))
		out.GIFTimeoutEmergencyColorsCap = int(clampFloat(float64(out.GIFTimeoutEmergencyColorsCap), 16, float64(out.GIFTimeoutFallbackColorsCap)))
		out.GIFTimeoutEmergencyMinWidth = int(clampFloat(float64(out.GIFTimeoutEmergencyMinWidth), 240, float64(out.GIFTimeoutEmergencyWidthCap)))
		out.GIFTimeoutEmergencyDurationTrigger = clampFloat(out.GIFTimeoutEmergencyDurationTrigger, 1.0, 6.0)
		out.GIFTimeoutEmergencyDurationScale = clampFloat(out.GIFTimeoutEmergencyDurationScale, 0.5, 1.0)
		out.GIFTimeoutEmergencyDurationMinSec = clampFloat(out.GIFTimeoutEmergencyDurationMinSec, 0.8, 4.0)
		out.GIFTimeoutLastResortFPSCap = int(clampFloat(float64(out.GIFTimeoutLastResortFPSCap), 4, float64(out.GIFTimeoutEmergencyFPSCap)))
		out.GIFTimeoutLastResortWidthCap = int(clampFloat(float64(out.GIFTimeoutLastResortWidthCap), 240, float64(out.GIFTimeoutEmergencyWidthCap)))
		out.GIFTimeoutLastResortColorsCap = int(clampFloat(float64(out.GIFTimeoutLastResortColorsCap), 16, float64(out.GIFTimeoutEmergencyColorsCap)))
		out.GIFTimeoutLastResortMinWidth = int(clampFloat(float64(out.GIFTimeoutLastResortMinWidth), 240, float64(out.GIFTimeoutLastResortWidthCap)))
		out.GIFTimeoutLastResortDurationMinSec = clampFloat(out.GIFTimeoutLastResortDurationMinSec, 0.6, 3.0)
		out.GIFTimeoutLastResortDurationMaxSec = clampFloat(out.GIFTimeoutLastResortDurationMaxSec, out.GIFTimeoutLastResortDurationMinSec, 4.0)
	}

	if out.WebPTargetSizeKB <= 0 {
		out.WebPTargetSizeKB = def.WebPTargetSizeKB
	}
	if out.WebPTargetSizeKB > 10240 {
		out.WebPTargetSizeKB = 10240
	}
	if out.JPGTargetSizeKB <= 0 {
		out.JPGTargetSizeKB = def.JPGTargetSizeKB
	}
	if out.JPGTargetSizeKB > 10240 {
		out.JPGTargetSizeKB = 10240
	}
	if out.PNGTargetSizeKB <= 0 {
		out.PNGTargetSizeKB = def.PNGTargetSizeKB
	}
	if out.PNGTargetSizeKB > 10240 {
		out.PNGTargetSizeKB = 10240
	}
	if out.StillMinBlurScore <= 0 {
		out.StillMinBlurScore = def.StillMinBlurScore
	}
	out.StillMinBlurScore = clampFloat(out.StillMinBlurScore, 0, 300)
	if out.StillMinExposureScore <= 0 {
		out.StillMinExposureScore = def.StillMinExposureScore
	}
	out.StillMinExposureScore = clampFloat(out.StillMinExposureScore, 0, 1)
	if out.StillMinWidth < 0 {
		out.StillMinWidth = 0
	}
	if out.StillMinWidth > 4096 {
		out.StillMinWidth = 4096
	}
	if out.StillMinHeight < 0 {
		out.StillMinHeight = 0
	}
	if out.StillMinHeight > 4096 {
		out.StillMinHeight = 4096
	}

	mode := strings.ToLower(strings.TrimSpace(out.GIFDitherMode))
	switch mode {
	case "sierra2_4a", "bayer", "floyd_steinberg", "none":
		out.GIFDitherMode = mode
	default:
		out.GIFDitherMode = def.GIFDitherMode
	}
	if out.LiveCoverPortraitWeight <= 0 {
		out.LiveCoverPortraitWeight = def.LiveCoverPortraitWeight
	}
	out.LiveCoverPortraitWeight = clampFloat(out.LiveCoverPortraitWeight, 0.01, 0.25)
	if out.LiveCoverSceneMinSamples <= 0 {
		out.LiveCoverSceneMinSamples = def.LiveCoverSceneMinSamples
	}
	if out.LiveCoverSceneMinSamples > 100 {
		out.LiveCoverSceneMinSamples = 100
	}
	if out.LiveCoverGuardMinTotal <= 0 {
		out.LiveCoverGuardMinTotal = def.LiveCoverGuardMinTotal
	}
	if out.LiveCoverGuardMinTotal > 1000 {
		out.LiveCoverGuardMinTotal = 1000
	}
	if out.LiveCoverGuardScoreFloor <= 0 {
		out.LiveCoverGuardScoreFloor = def.LiveCoverGuardScoreFloor
	}
	out.LiveCoverGuardScoreFloor = clampFloat(out.LiveCoverGuardScoreFloor, 0.3, 0.95)

	legacyZeroFeedback := out.HighlightFeedbackRollout == 0 &&
		out.HighlightFeedbackMinJobs == 0 &&
		out.HighlightFeedbackMinScore == 0 &&
		out.HighlightFeedbackBoost == 0 &&
		out.HighlightWeightPosition == 0 &&
		out.HighlightWeightDuration == 0 &&
		out.HighlightWeightReason == 0
	if legacyZeroFeedback {
		// Backward compatibility: old rows (or zero-value structs) do not carry the
		// new feedback-rerank fields and decode to all-zero values.
		out.HighlightFeedbackEnabled = def.HighlightFeedbackEnabled
		out.HighlightFeedbackRollout = def.HighlightFeedbackRollout
		out.HighlightFeedbackMinJobs = def.HighlightFeedbackMinJobs
		out.HighlightFeedbackMinScore = def.HighlightFeedbackMinScore
		out.HighlightFeedbackBoost = def.HighlightFeedbackBoost
		out.HighlightWeightPosition = def.HighlightWeightPosition
		out.HighlightWeightDuration = def.HighlightWeightDuration
		out.HighlightWeightReason = def.HighlightWeightReason
	}

	legacyZeroNegativeGuard := out.HighlightNegativeGuardThreshold == 0 &&
		out.HighlightNegativeGuardMinWeight == 0 &&
		out.HighlightNegativePenaltyScale == 0 &&
		out.HighlightNegativePenaltyWeight == 0
	if legacyZeroNegativeGuard {
		// Backward compatibility: old rows (or zero-value structs) do not carry
		// negative-feedback guard fields and decode to all-zero values.
		out.HighlightNegativeGuardEnabled = def.HighlightNegativeGuardEnabled
		out.HighlightNegativeGuardThreshold = def.HighlightNegativeGuardThreshold
		out.HighlightNegativeGuardMinWeight = def.HighlightNegativeGuardMinWeight
		out.HighlightNegativePenaltyScale = def.HighlightNegativePenaltyScale
		out.HighlightNegativePenaltyWeight = def.HighlightNegativePenaltyWeight
	}

	if out.HighlightFeedbackRollout < 0 {
		out.HighlightFeedbackRollout = 0
	}
	if out.HighlightFeedbackRollout > 100 {
		out.HighlightFeedbackRollout = 100
	}
	if out.HighlightFeedbackMinJobs < 1 {
		out.HighlightFeedbackMinJobs = 1
	}
	if out.HighlightFeedbackMinJobs > 200 {
		out.HighlightFeedbackMinJobs = 200
	}
	out.HighlightFeedbackMinScore = clampFloat(out.HighlightFeedbackMinScore, 0, 200)
	out.HighlightFeedbackBoost = clampFloat(out.HighlightFeedbackBoost, 0, 3)
	out.HighlightWeightPosition = clampFloat(out.HighlightWeightPosition, 0, 1)
	out.HighlightWeightDuration = clampFloat(out.HighlightWeightDuration, 0, 1)
	out.HighlightWeightReason = clampFloat(out.HighlightWeightReason, 0, 1)
	out.HighlightNegativeGuardThreshold = clampFloat(out.HighlightNegativeGuardThreshold, 0.2, 0.95)
	out.HighlightNegativeGuardMinWeight = clampFloat(out.HighlightNegativeGuardMinWeight, 0.5, 20)
	out.HighlightNegativePenaltyScale = clampFloat(out.HighlightNegativePenaltyScale, 0, 1)
	out.HighlightNegativePenaltyWeight = clampFloat(out.HighlightNegativePenaltyWeight, 0, 2)

	out.AIDirectorInputMode = normalizeAIDirectorInputModeSetting(out.AIDirectorInputMode, def.AIDirectorInputMode)
	out.AIDirectorOperatorInstruction = strings.TrimSpace(out.AIDirectorOperatorInstruction)
	if len(out.AIDirectorOperatorInstruction) > 4000 {
		out.AIDirectorOperatorInstruction = out.AIDirectorOperatorInstruction[:4000]
	}
	out.AIDirectorOperatorInstructionVersion = strings.TrimSpace(out.AIDirectorOperatorInstructionVersion)
	if out.AIDirectorOperatorInstructionVersion == "" {
		out.AIDirectorOperatorInstructionVersion = def.AIDirectorOperatorInstructionVersion
	}
	if len(out.AIDirectorOperatorInstructionVersion) > 64 {
		out.AIDirectorOperatorInstructionVersion = out.AIDirectorOperatorInstructionVersion[:64]
	}
	legacyZeroAIDirectorOperator := !out.AIDirectorOperatorEnabled &&
		out.AIDirectorOperatorInstruction == "" &&
		out.AIDirectorOperatorInstructionVersion == def.AIDirectorOperatorInstructionVersion
	if legacyZeroAIDirectorOperator {
		out.AIDirectorOperatorEnabled = def.AIDirectorOperatorEnabled
	}
	if out.AIDirectorCountExpandRatio == 0 && out.AIDirectorDurationExpandRatio == 0 && out.AIDirectorCountAbsoluteCap == 0 && out.AIDirectorDurationAbsoluteCapSec == 0 {
		out.AIDirectorCountExpandRatio = def.AIDirectorCountExpandRatio
		out.AIDirectorDurationExpandRatio = def.AIDirectorDurationExpandRatio
		out.AIDirectorCountAbsoluteCap = def.AIDirectorCountAbsoluteCap
		out.AIDirectorDurationAbsoluteCapSec = def.AIDirectorDurationAbsoluteCapSec
	}
	out.AIDirectorCountExpandRatio = clampFloat(out.AIDirectorCountExpandRatio, 0, 3)
	out.AIDirectorDurationExpandRatio = clampFloat(out.AIDirectorDurationExpandRatio, 0, 3)
	if out.AIDirectorCountAbsoluteCap <= 0 {
		out.AIDirectorCountAbsoluteCap = def.AIDirectorCountAbsoluteCap
	}
	if out.AIDirectorCountAbsoluteCap > maxGIFCandidateOutputs {
		out.AIDirectorCountAbsoluteCap = maxGIFCandidateOutputs
	}
	if out.AIDirectorCountAbsoluteCap < out.GIFCandidateMaxOutputs {
		out.AIDirectorCountAbsoluteCap = out.GIFCandidateMaxOutputs
	}
	if out.AIDirectorDurationAbsoluteCapSec <= 0 {
		out.AIDirectorDurationAbsoluteCapSec = def.AIDirectorDurationAbsoluteCapSec
	}
	out.AIDirectorDurationAbsoluteCapSec = clampFloat(out.AIDirectorDurationAbsoluteCapSec, 2.0, 12.0)

	legacyZeroAIJudgeHardGate := out.GIFAIJudgeHardGateMinOverallScore == 0 &&
		out.GIFAIJudgeHardGateMinClarityScore == 0 &&
		out.GIFAIJudgeHardGateMinLoopScore == 0 &&
		out.GIFAIJudgeHardGateMinOutputScore == 0 &&
		out.GIFAIJudgeHardGateMinDurationMS == 0 &&
		out.GIFAIJudgeHardGateSizeMultiplier == 0
	if legacyZeroAIJudgeHardGate {
		out.GIFAIJudgeHardGateMinOverallScore = def.GIFAIJudgeHardGateMinOverallScore
		out.GIFAIJudgeHardGateMinClarityScore = def.GIFAIJudgeHardGateMinClarityScore
		out.GIFAIJudgeHardGateMinLoopScore = def.GIFAIJudgeHardGateMinLoopScore
		out.GIFAIJudgeHardGateMinOutputScore = def.GIFAIJudgeHardGateMinOutputScore
		out.GIFAIJudgeHardGateMinDurationMS = def.GIFAIJudgeHardGateMinDurationMS
		out.GIFAIJudgeHardGateSizeMultiplier = def.GIFAIJudgeHardGateSizeMultiplier
	} else {
		if out.GIFAIJudgeHardGateMinOverallScore <= 0 {
			out.GIFAIJudgeHardGateMinOverallScore = def.GIFAIJudgeHardGateMinOverallScore
		}
		if out.GIFAIJudgeHardGateMinClarityScore <= 0 {
			out.GIFAIJudgeHardGateMinClarityScore = def.GIFAIJudgeHardGateMinClarityScore
		}
		if out.GIFAIJudgeHardGateMinLoopScore <= 0 {
			out.GIFAIJudgeHardGateMinLoopScore = def.GIFAIJudgeHardGateMinLoopScore
		}
		if out.GIFAIJudgeHardGateMinOutputScore <= 0 {
			out.GIFAIJudgeHardGateMinOutputScore = def.GIFAIJudgeHardGateMinOutputScore
		}
		if out.GIFAIJudgeHardGateMinDurationMS <= 0 {
			out.GIFAIJudgeHardGateMinDurationMS = def.GIFAIJudgeHardGateMinDurationMS
		}
		if out.GIFAIJudgeHardGateSizeMultiplier <= 0 {
			out.GIFAIJudgeHardGateSizeMultiplier = def.GIFAIJudgeHardGateSizeMultiplier
		}
	}
	out.GIFAIJudgeHardGateMinOverallScore = clampFloat(out.GIFAIJudgeHardGateMinOverallScore, 0.01, 1.0)
	out.GIFAIJudgeHardGateMinClarityScore = clampFloat(out.GIFAIJudgeHardGateMinClarityScore, 0.01, 1.0)
	out.GIFAIJudgeHardGateMinLoopScore = clampFloat(out.GIFAIJudgeHardGateMinLoopScore, 0.01, 1.0)
	out.GIFAIJudgeHardGateMinOutputScore = clampFloat(out.GIFAIJudgeHardGateMinOutputScore, 0.01, 1.0)
	out.GIFAIJudgeHardGateMinDurationMS = int(clampFloat(float64(out.GIFAIJudgeHardGateMinDurationMS), 50, 10000))
	out.GIFAIJudgeHardGateSizeMultiplier = int(clampFloat(float64(out.GIFAIJudgeHardGateSizeMultiplier), 1, 20))

	return out
}

func normalizeQualityProfile(raw, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case QualityProfileClarity:
		return QualityProfileClarity
	case QualityProfileSize:
		return QualityProfileSize
	default:
		return strings.ToLower(strings.TrimSpace(fallback))
	}
}

func normalizeGIFPipelineModeSetting(raw, fallback string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "light", "standard", "hq":
		return mode
	default:
		fallbackMode := strings.ToLower(strings.TrimSpace(fallback))
		switch fallbackMode {
		case "light", "standard", "hq":
			return fallbackMode
		default:
			return "standard"
		}
	}
}

func normalizeAIDirectorInputModeSetting(raw, fallback string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "frames", "full_video", "hybrid":
		return mode
	default:
		fallbackMode := strings.ToLower(strings.TrimSpace(fallback))
		switch fallbackMode {
		case "frames", "full_video", "hybrid":
			return fallbackMode
		default:
			return "hybrid"
		}
	}
}

func resolveGIFDurationTierThresholds(settings QualitySettings) (mediumSec, longSec, ultraSec float64) {
	out := NormalizeQualitySettings(settings)
	return out.GIFDurationTierMediumSec, out.GIFDurationTierLongSec, out.GIFDurationTierUltraSec
}
