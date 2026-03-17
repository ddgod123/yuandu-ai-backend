package videojobs

import "strings"

const (
	QualityProfileClarity = "clarity"
	QualityProfileSize    = "size"
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
	GIFLoopTuneEnabled                   bool    `json:"gif_loop_tune_enabled"`
	GIFLoopTuneMinEnableSec              float64 `json:"gif_loop_tune_min_enable_sec"`
	GIFLoopTuneMinImprovement            float64 `json:"gif_loop_tune_min_improvement"`
	GIFLoopTuneMotionTarget              float64 `json:"gif_loop_tune_motion_target"`
	GIFLoopTunePreferDuration            float64 `json:"gif_loop_tune_prefer_duration_sec"`
	GIFCandidateMaxOutputs               int     `json:"gif_candidate_max_outputs"`
	GIFCandidateConfidenceThreshold      float64 `json:"gif_candidate_confidence_threshold"`
	GIFCandidateDedupIOUThreshold        float64 `json:"gif_candidate_dedup_iou_threshold"`
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
	AIDirectorOperatorInstruction        string  `json:"ai_director_operator_instruction"`
	AIDirectorOperatorInstructionVersion string  `json:"ai_director_operator_instruction_version"`
	AIDirectorOperatorEnabled            bool    `json:"ai_director_operator_enabled"`
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
		GIFLoopTuneEnabled:                   true,
		GIFLoopTuneMinEnableSec:              1.4,
		GIFLoopTuneMinImprovement:            0.04,
		GIFLoopTuneMotionTarget:              0.22,
		GIFLoopTunePreferDuration:            2.4,
		GIFCandidateMaxOutputs:               3,
		GIFCandidateConfidenceThreshold:      0.35,
		GIFCandidateDedupIOUThreshold:        0.45,
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
		AIDirectorOperatorInstruction:        "",
		AIDirectorOperatorInstructionVersion: "v1",
		AIDirectorOperatorEnabled:            true,
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
		out.GIFCandidateConfidenceThreshold == 0 &&
		out.GIFCandidateDedupIOUThreshold == 0
	if legacyZeroGIFCandidate {
		// Backward compatibility: old rows (or zero-value structs) do not carry GIF candidate fields.
		out.GIFCandidateMaxOutputs = def.GIFCandidateMaxOutputs
		out.GIFCandidateConfidenceThreshold = def.GIFCandidateConfidenceThreshold
		out.GIFCandidateDedupIOUThreshold = def.GIFCandidateDedupIOUThreshold
	} else {
		if out.GIFCandidateMaxOutputs <= 0 {
			out.GIFCandidateMaxOutputs = def.GIFCandidateMaxOutputs
		}
		if out.GIFCandidateMaxOutputs > 6 {
			out.GIFCandidateMaxOutputs = 6
		}
		if out.GIFCandidateConfidenceThreshold < 0 {
			out.GIFCandidateConfidenceThreshold = 0
		}
		out.GIFCandidateConfidenceThreshold = clampFloat(out.GIFCandidateConfidenceThreshold, 0, 0.95)
		if out.GIFCandidateDedupIOUThreshold <= 0 {
			out.GIFCandidateDedupIOUThreshold = def.GIFCandidateDedupIOUThreshold
		}
		out.GIFCandidateDedupIOUThreshold = clampFloat(out.GIFCandidateDedupIOUThreshold, 0.1, 0.95)
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
