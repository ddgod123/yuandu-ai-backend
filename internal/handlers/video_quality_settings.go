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
	GIFHealthAlertThresholdSettings
	FeedbackIntegrityAlertThresholdSettings
}

type VideoQualitySettingResponse struct {
	videojobs.QualitySettings
	GIFHealthAlertThresholdSettings
	FeedbackIntegrityAlertThresholdSettings
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
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
	if req.GIFCandidateMaxOutputs < 1 || req.GIFCandidateMaxOutputs > 6 {
		return errors.New("invalid gif_candidate_max_outputs: expected 1..6")
	}
	if req.GIFCandidateConfidenceThreshold < 0 || req.GIFCandidateConfidenceThreshold > 0.95 {
		return errors.New("invalid gif_candidate_confidence_threshold: expected 0..0.95")
	}
	if req.GIFCandidateDedupIOUThreshold <= 0 || req.GIFCandidateDedupIOUThreshold >= 1 {
		return errors.New("invalid gif_candidate_dedup_iou_threshold: expected (0,1)")
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
	if len(strings.TrimSpace(req.AIDirectorOperatorInstructionVersion)) == 0 {
		return errors.New("invalid ai_director_operator_instruction_version: expected non-empty")
	}
	if len(strings.TrimSpace(req.AIDirectorOperatorInstructionVersion)) > 64 {
		return errors.New("invalid ai_director_operator_instruction_version: expected <= 64 chars")
	}
	if len(strings.TrimSpace(req.AIDirectorOperatorInstruction)) > 4000 {
		return errors.New("invalid ai_director_operator_instruction: expected <= 4000 chars")
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
		GIFLoopTuneEnabled:                   setting.GIFLoopTuneEnabled,
		GIFLoopTuneMinEnableSec:              setting.GIFLoopTuneMinEnableSec,
		GIFLoopTuneMinImprovement:            setting.GIFLoopTuneMinImprovement,
		GIFLoopTuneMotionTarget:              setting.GIFLoopTuneMotionTarget,
		GIFLoopTunePreferDuration:            setting.GIFLoopTunePreferDuration,
		GIFCandidateMaxOutputs:               setting.GIFCandidateMaxOutputs,
		GIFCandidateConfidenceThreshold:      setting.GIFCandidateConfidenceThreshold,
		GIFCandidateDedupIOUThreshold:        setting.GIFCandidateDedupIOUThreshold,
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
		AIDirectorOperatorInstruction:        setting.AIDirectorOperatorInstruction,
		AIDirectorOperatorInstructionVersion: setting.AIDirectorOperatorInstructionVersion,
		AIDirectorOperatorEnabled:            setting.AIDirectorOperatorEnabled,
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
	dst.GIFLoopTuneEnabled = settings.GIFLoopTuneEnabled
	dst.GIFLoopTuneMinEnableSec = settings.GIFLoopTuneMinEnableSec
	dst.GIFLoopTuneMinImprovement = settings.GIFLoopTuneMinImprovement
	dst.GIFLoopTuneMotionTarget = settings.GIFLoopTuneMotionTarget
	dst.GIFLoopTunePreferDuration = settings.GIFLoopTunePreferDuration
	dst.GIFCandidateMaxOutputs = settings.GIFCandidateMaxOutputs
	dst.GIFCandidateConfidenceThreshold = settings.GIFCandidateConfidenceThreshold
	dst.GIFCandidateDedupIOUThreshold = settings.GIFCandidateDedupIOUThreshold
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
	dst.AIDirectorOperatorInstruction = strings.TrimSpace(settings.AIDirectorOperatorInstruction)
	dst.AIDirectorOperatorInstructionVersion = strings.TrimSpace(settings.AIDirectorOperatorInstructionVersion)
	dst.AIDirectorOperatorEnabled = settings.AIDirectorOperatorEnabled
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
// @Success 200 {object} VideoQualitySettingResponse
// @Router /api/admin/video-jobs/quality-settings [get]
func (h *Handler) GetAdminVideoQualitySetting(c *gin.Context) {
	setting, err := h.loadVideoQualitySetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toVideoQualitySettingResponse(setting, true))
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
		GIFLoopTuneEnabled:                      quality.GIFLoopTuneEnabled,
		GIFLoopTuneMinEnableSec:                 quality.GIFLoopTuneMinEnableSec,
		GIFLoopTuneMinImprovement:               quality.GIFLoopTuneMinImprovement,
		GIFLoopTuneMotionTarget:                 quality.GIFLoopTuneMotionTarget,
		GIFLoopTunePreferDuration:               quality.GIFLoopTunePreferDuration,
		GIFCandidateMaxOutputs:                  quality.GIFCandidateMaxOutputs,
		GIFCandidateConfidenceThreshold:         quality.GIFCandidateConfidenceThreshold,
		GIFCandidateDedupIOUThreshold:           quality.GIFCandidateDedupIOUThreshold,
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
		AIDirectorOperatorInstruction:           quality.AIDirectorOperatorInstruction,
		AIDirectorOperatorInstructionVersion:    quality.AIDirectorOperatorInstructionVersion,
		AIDirectorOperatorEnabled:               quality.AIDirectorOperatorEnabled,
		GIFHealthAlertThresholdSettings:         gifThresholds,
		FeedbackIntegrityAlertThresholdSettings: feedbackThresholds,
	}
}

func (h *Handler) saveVideoQualitySetting(req VideoQualitySettingRequest) (models.VideoQualitySetting, error) {
	settings := videojobs.NormalizeQualitySettings(videojobs.QualitySettings{
		MinBrightness:                        req.MinBrightness,
		MaxBrightness:                        req.MaxBrightness,
		BlurThresholdFactor:                  req.BlurThresholdFactor,
		BlurThresholdMin:                     req.BlurThresholdMin,
		BlurThresholdMax:                     req.BlurThresholdMax,
		DuplicateHammingThreshold:            req.DuplicateHammingThreshold,
		DuplicateBacktrackFrames:             req.DuplicateBacktrackFrames,
		FallbackBlurRelaxFactor:              req.FallbackBlurRelaxFactor,
		FallbackHammingThreshold:             req.FallbackHammingThreshold,
		MinKeepBase:                          req.MinKeepBase,
		MinKeepRatio:                         req.MinKeepRatio,
		QualityAnalysisWorkers:               req.QualityAnalysisWorkers,
		UploadConcurrency:                    req.UploadConcurrency,
		GIFProfile:                           req.GIFProfile,
		WebPProfile:                          req.WebPProfile,
		LiveProfile:                          req.LiveProfile,
		JPGProfile:                           req.JPGProfile,
		PNGProfile:                           req.PNGProfile,
		GIFDefaultFPS:                        req.GIFDefaultFPS,
		GIFDefaultMaxColors:                  req.GIFDefaultMaxColors,
		GIFDitherMode:                        req.GIFDitherMode,
		GIFTargetSizeKB:                      req.GIFTargetSizeKB,
		GIFLoopTuneEnabled:                   req.GIFLoopTuneEnabled,
		GIFLoopTuneMinEnableSec:              req.GIFLoopTuneMinEnableSec,
		GIFLoopTuneMinImprovement:            req.GIFLoopTuneMinImprovement,
		GIFLoopTuneMotionTarget:              req.GIFLoopTuneMotionTarget,
		GIFLoopTunePreferDuration:            req.GIFLoopTunePreferDuration,
		GIFCandidateMaxOutputs:               req.GIFCandidateMaxOutputs,
		GIFCandidateConfidenceThreshold:      req.GIFCandidateConfidenceThreshold,
		GIFCandidateDedupIOUThreshold:        req.GIFCandidateDedupIOUThreshold,
		WebPTargetSizeKB:                     req.WebPTargetSizeKB,
		JPGTargetSizeKB:                      req.JPGTargetSizeKB,
		PNGTargetSizeKB:                      req.PNGTargetSizeKB,
		StillMinBlurScore:                    req.StillMinBlurScore,
		StillMinExposureScore:                req.StillMinExposureScore,
		StillMinWidth:                        req.StillMinWidth,
		StillMinHeight:                       req.StillMinHeight,
		LiveCoverPortraitWeight:              req.LiveCoverPortraitWeight,
		LiveCoverSceneMinSamples:             req.LiveCoverSceneMinSamples,
		LiveCoverGuardMinTotal:               req.LiveCoverGuardMinTotal,
		LiveCoverGuardScoreFloor:             req.LiveCoverGuardScoreFloor,
		HighlightFeedbackEnabled:             req.HighlightFeedbackEnabled,
		HighlightFeedbackRollout:             req.HighlightFeedbackRollout,
		HighlightFeedbackMinJobs:             req.HighlightFeedbackMinJobs,
		HighlightFeedbackMinScore:            req.HighlightFeedbackMinScore,
		HighlightFeedbackBoost:               req.HighlightFeedbackBoost,
		HighlightWeightPosition:              req.HighlightWeightPosition,
		HighlightWeightDuration:              req.HighlightWeightDuration,
		HighlightWeightReason:                req.HighlightWeightReason,
		HighlightNegativeGuardEnabled:        req.HighlightNegativeGuardEnabled,
		HighlightNegativeGuardThreshold:      req.HighlightNegativeGuardThreshold,
		HighlightNegativeGuardMinWeight:      req.HighlightNegativeGuardMinWeight,
		HighlightNegativePenaltyScale:        req.HighlightNegativePenaltyScale,
		HighlightNegativePenaltyWeight:       req.HighlightNegativePenaltyWeight,
		AIDirectorOperatorInstruction:        req.AIDirectorOperatorInstruction,
		AIDirectorOperatorInstructionVersion: req.AIDirectorOperatorInstructionVersion,
		AIDirectorOperatorEnabled:            req.AIDirectorOperatorEnabled,
	})
	alertThresholds := normalizeGIFHealthAlertThresholdSettings(req.GIFHealthAlertThresholdSettings)
	feedbackIntegrityThresholds := normalizeFeedbackIntegrityAlertThresholdSettings(req.FeedbackIntegrityAlertThresholdSettings)

	var saved models.VideoQualitySetting
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var current models.VideoQualitySetting
		err := tx.First(&current, 1).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			payload := models.VideoQualitySetting{}
			applyQualitySettingsToModel(&payload, settings)
			applyGIFHealthAlertThresholdSettingsToModel(&payload, alertThresholds)
			applyFeedbackIntegrityAlertThresholdSettingsToModel(&payload, feedbackIntegrityThresholds)
			if err := tx.Create(&payload).Error; err != nil {
				return err
			}
			saved = payload
			return nil
		}

		applyQualitySettingsToModel(&current, settings)
		applyGIFHealthAlertThresholdSettingsToModel(&current, alertThresholds)
		applyFeedbackIntegrityAlertThresholdSettingsToModel(&current, feedbackIntegrityThresholds)
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		saved = current
		return nil
	})
	return saved, err
}

func (h *Handler) bindVideoQualitySettingPatch(c *gin.Context) (VideoQualitySettingRequest, error) {
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

	current, err := h.loadVideoQualitySetting()
	if err != nil {
		return VideoQualitySettingRequest{}, err
	}
	req := videoQualitySettingRequestFromModel(current)
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
// @Param body body VideoQualitySettingRequest true "video quality setting"
// @Success 200 {object} VideoQualitySettingResponse
// @Router /api/admin/video-jobs/quality-settings [put]
func (h *Handler) UpdateAdminVideoQualitySetting(c *gin.Context) {
	var req VideoQualitySettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
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

	c.JSON(http.StatusOK, toVideoQualitySettingResponse(saved, true))
}

// PatchAdminVideoQualitySetting godoc
// @Summary Patch video quality tuning setting (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body object true "video quality setting patch"
// @Success 200 {object} VideoQualitySettingResponse
// @Router /api/admin/video-jobs/quality-settings [patch]
func (h *Handler) PatchAdminVideoQualitySetting(c *gin.Context) {
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

	c.JSON(http.StatusOK, toVideoQualitySettingResponse(saved, true))
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
	if err := h.db.Raw(`
SELECT
	COUNT(*) AS jobs_total,
	COUNT(*) FILTER (WHERE status = 'done') AS jobs_done,
	COUNT(*) FILTER (WHERE status = 'failed') AS jobs_failed
FROM public.video_image_jobs
WHERE requested_format = 'gif'
	AND created_at >= ?
	AND created_at < ?
`, start, end).Scan(&jobs).Error; err != nil {
		return VideoQualityRolloutEffectMetric{}, err
	}

	var outputs outputsRow
	if err := h.db.Raw(`
SELECT
	COUNT(*) AS output_samples,
	COALESCE(AVG(score), 0) AS avg_output_score,
	COALESCE(AVG(gif_loop_tune_loop_closure), 0) AS avg_loop_closure,
	COALESCE(AVG(CASE WHEN gif_loop_tune_effective_applied THEN 1 ELSE 0 END), 0) AS loop_effective_rate,
	COALESCE(AVG(CASE WHEN gif_loop_tune_fallback_to_base THEN 1 ELSE 0 END), 0) AS loop_fallback_rate
FROM public.video_image_outputs
WHERE format = 'gif'
	AND file_role = 'main'
	AND created_at >= ?
	AND created_at < ?
`, start, end).Scan(&outputs).Error; err != nil {
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
