package videojobs

import "testing"

func TestNormalizeQualitySettings_DefaultProfiles(t *testing.T) {
	settings := NormalizeQualitySettings(QualitySettings{})
	if settings.GIFProfile != QualityProfileClarity {
		t.Fatalf("expected gif profile clarity, got %s", settings.GIFProfile)
	}
	if settings.WebPProfile != QualityProfileClarity {
		t.Fatalf("expected webp profile clarity, got %s", settings.WebPProfile)
	}
	if settings.LiveProfile != QualityProfileClarity {
		t.Fatalf("expected live profile clarity, got %s", settings.LiveProfile)
	}
	if settings.JPGProfile != QualityProfileClarity {
		t.Fatalf("expected jpg profile clarity, got %s", settings.JPGProfile)
	}
	if settings.PNGProfile != QualityProfileClarity {
		t.Fatalf("expected png profile clarity, got %s", settings.PNGProfile)
	}
	if !settings.HighlightFeedbackEnabled {
		t.Fatalf("expected highlight feedback rerank enabled by default")
	}
	if settings.HighlightFeedbackRollout != 100 {
		t.Fatalf("expected rollout 100, got %d", settings.HighlightFeedbackRollout)
	}
	if !settings.HighlightNegativeGuardEnabled {
		t.Fatalf("expected negative guard enabled by default")
	}
	if settings.HighlightNegativeGuardThreshold != 0.45 {
		t.Fatalf("expected negative guard threshold 0.45, got %.2f", settings.HighlightNegativeGuardThreshold)
	}
	if settings.HighlightNegativeGuardMinWeight != 4 {
		t.Fatalf("expected negative guard min weight 4, got %.2f", settings.HighlightNegativeGuardMinWeight)
	}
	if settings.HighlightNegativePenaltyScale != 0.55 {
		t.Fatalf("expected negative penalty scale 0.55, got %.2f", settings.HighlightNegativePenaltyScale)
	}
	if settings.HighlightNegativePenaltyWeight != 0.9 {
		t.Fatalf("expected negative penalty weight 0.9, got %.2f", settings.HighlightNegativePenaltyWeight)
	}
	if settings.LiveCoverPortraitWeight != 0.04 {
		t.Fatalf("expected default live portrait weight 0.04, got %.2f", settings.LiveCoverPortraitWeight)
	}
	if settings.GIFTargetSizeKB != 2048 {
		t.Fatalf("expected default gif target size 2048, got %d", settings.GIFTargetSizeKB)
	}
	if !settings.GIFGifsicleEnabled {
		t.Fatalf("expected gif gifsicle optimization enabled by default")
	}
	if settings.GIFGifsicleLevel != 2 {
		t.Fatalf("expected default gifsicle level 2, got %d", settings.GIFGifsicleLevel)
	}
	if settings.GIFGifsicleSkipBelowKB != 256 {
		t.Fatalf("expected default gifsicle skip_below 256kb, got %d", settings.GIFGifsicleSkipBelowKB)
	}
	if settings.GIFGifsicleMinGainRatio != 0.03 {
		t.Fatalf("expected default gifsicle min gain ratio 0.03, got %.2f", settings.GIFGifsicleMinGainRatio)
	}
	if !settings.GIFLoopTuneEnabled {
		t.Fatalf("expected gif loop tuning enabled by default")
	}
	if settings.GIFLoopTuneMinEnableSec != 1.4 {
		t.Fatalf("expected default gif loop min enable sec 1.4, got %.2f", settings.GIFLoopTuneMinEnableSec)
	}
	if settings.GIFLoopTuneMinImprovement != 0.04 {
		t.Fatalf("expected default gif loop min improvement 0.04, got %.2f", settings.GIFLoopTuneMinImprovement)
	}
	if settings.GIFLoopTuneMotionTarget != 0.22 {
		t.Fatalf("expected default gif loop motion target 0.22, got %.2f", settings.GIFLoopTuneMotionTarget)
	}
	if settings.GIFLoopTunePreferDuration != 2.4 {
		t.Fatalf("expected default gif loop prefer duration 2.4, got %.2f", settings.GIFLoopTunePreferDuration)
	}
	if settings.GIFCandidateMaxOutputs != 3 {
		t.Fatalf("expected default gif candidate max outputs 3, got %d", settings.GIFCandidateMaxOutputs)
	}
	if settings.GIFCandidateLongVideoMaxOutputs != 3 {
		t.Fatalf("expected default gif long video max outputs 3, got %d", settings.GIFCandidateLongVideoMaxOutputs)
	}
	if settings.GIFCandidateUltraVideoMaxOutputs != 2 {
		t.Fatalf("expected default gif ultra video max outputs 2, got %d", settings.GIFCandidateUltraVideoMaxOutputs)
	}
	if settings.GIFCandidateConfidenceThreshold != 0.35 {
		t.Fatalf("expected default gif candidate confidence threshold 0.35, got %.2f", settings.GIFCandidateConfidenceThreshold)
	}
	if settings.GIFCandidateDedupIOUThreshold != 0.45 {
		t.Fatalf("expected default gif candidate dedup iou threshold 0.45, got %.2f", settings.GIFCandidateDedupIOUThreshold)
	}
	if settings.WebPTargetSizeKB != 1536 {
		t.Fatalf("expected default webp target size 1536, got %d", settings.WebPTargetSizeKB)
	}
	if settings.JPGTargetSizeKB != 512 {
		t.Fatalf("expected default jpg target size 512, got %d", settings.JPGTargetSizeKB)
	}
	if settings.PNGTargetSizeKB != 1024 {
		t.Fatalf("expected default png target size 1024, got %d", settings.PNGTargetSizeKB)
	}
	if settings.StillMinBlurScore != 12 {
		t.Fatalf("expected default still min blur 12, got %.2f", settings.StillMinBlurScore)
	}
	if settings.StillMinExposureScore != 0.28 {
		t.Fatalf("expected default still min exposure 0.28, got %.2f", settings.StillMinExposureScore)
	}
	if settings.StillMinWidth != 0 || settings.StillMinHeight != 0 {
		t.Fatalf("expected default still min size 0x0, got %dx%d", settings.StillMinWidth, settings.StillMinHeight)
	}
	if settings.LiveCoverSceneMinSamples != 5 {
		t.Fatalf("expected default scene min samples 5, got %d", settings.LiveCoverSceneMinSamples)
	}
	if settings.LiveCoverGuardMinTotal != 20 {
		t.Fatalf("expected default guard min total 20, got %d", settings.LiveCoverGuardMinTotal)
	}
	if settings.LiveCoverGuardScoreFloor != 0.58 {
		t.Fatalf("expected default guard score floor 0.58, got %.2f", settings.LiveCoverGuardScoreFloor)
	}
}

func TestApplyAnimatedProfileDefaults_SizeMode(t *testing.T) {
	settings := DefaultQualitySettings()
	settings.GIFProfile = QualityProfileSize

	out := applyAnimatedProfileDefaults(jobOptions{}, []string{"gif"}, settings)
	if out.FPS != 10 {
		t.Fatalf("expected fps 10, got %d", out.FPS)
	}
	if out.MaxColors != 96 {
		t.Fatalf("expected max colors 96, got %d", out.MaxColors)
	}
	if out.Width != 720 {
		t.Fatalf("expected width 720, got %d", out.Width)
	}
}

func TestClampWindowDuration_EnforcesCap(t *testing.T) {
	window := highlightCandidate{
		StartSec: 1.2,
		EndSec:   8.8,
	}
	clamped := clampWindowDuration(window, 3.0, 12.0)
	if clamped.EndSec-clamped.StartSec > 3.001 {
		t.Fatalf("expected <=3s, got %.3f", clamped.EndSec-clamped.StartSec)
	}
}

func TestNormalizeQualitySettings_FeedbackBounds(t *testing.T) {
	settings := NormalizeQualitySettings(QualitySettings{
		HighlightFeedbackEnabled:        true,
		HighlightFeedbackRollout:        1000,
		HighlightFeedbackMinJobs:        -1,
		HighlightFeedbackMinScore:       -2,
		HighlightFeedbackBoost:          9,
		HighlightWeightPosition:         9,
		HighlightWeightDuration:         -2,
		HighlightWeightReason:           3,
		HighlightNegativeGuardEnabled:   true,
		HighlightNegativeGuardThreshold: 9,
		HighlightNegativeGuardMinWeight: -1,
		HighlightNegativePenaltyScale:   2,
		HighlightNegativePenaltyWeight:  9,
	})

	if settings.HighlightFeedbackRollout != 100 {
		t.Fatalf("expected rollout clamp to 100, got %d", settings.HighlightFeedbackRollout)
	}
	if settings.HighlightFeedbackMinJobs != 1 {
		t.Fatalf("expected min jobs clamp to 1, got %d", settings.HighlightFeedbackMinJobs)
	}
	if settings.HighlightFeedbackMinScore != 0 {
		t.Fatalf("expected min weighted signals clamp to 0, got %.2f", settings.HighlightFeedbackMinScore)
	}
	if settings.HighlightFeedbackBoost != 3 {
		t.Fatalf("expected boost clamp to 3, got %.2f", settings.HighlightFeedbackBoost)
	}
	if settings.HighlightWeightPosition != 1 {
		t.Fatalf("expected position weight clamp to 1, got %.2f", settings.HighlightWeightPosition)
	}
	if settings.HighlightWeightDuration != 0 {
		t.Fatalf("expected duration weight clamp to 0, got %.2f", settings.HighlightWeightDuration)
	}
	if settings.HighlightWeightReason != 1 {
		t.Fatalf("expected reason weight clamp to 1, got %.2f", settings.HighlightWeightReason)
	}
	if settings.HighlightNegativeGuardThreshold != 0.95 {
		t.Fatalf("expected negative guard threshold clamp to 0.95, got %.2f", settings.HighlightNegativeGuardThreshold)
	}
	if settings.HighlightNegativeGuardMinWeight != 0.5 {
		t.Fatalf("expected negative guard min weight clamp to 0.5, got %.2f", settings.HighlightNegativeGuardMinWeight)
	}
	if settings.HighlightNegativePenaltyScale != 1 {
		t.Fatalf("expected negative penalty scale clamp to 1, got %.2f", settings.HighlightNegativePenaltyScale)
	}
	if settings.HighlightNegativePenaltyWeight != 2 {
		t.Fatalf("expected negative penalty weight clamp to 2, got %.2f", settings.HighlightNegativePenaltyWeight)
	}
}

func TestNormalizeQualitySettings_LivePortraitWeightBounds(t *testing.T) {
	settings := NormalizeQualitySettings(QualitySettings{
		LiveCoverPortraitWeight: 9,
	})
	if settings.LiveCoverPortraitWeight != 0.25 {
		t.Fatalf("expected portrait weight clamp to 0.25, got %.2f", settings.LiveCoverPortraitWeight)
	}

	settings = NormalizeQualitySettings(QualitySettings{
		LiveCoverPortraitWeight: -1,
	})
	if settings.LiveCoverPortraitWeight != 0.04 {
		t.Fatalf("expected portrait weight fallback to default 0.04, got %.2f", settings.LiveCoverPortraitWeight)
	}
}

func TestNormalizeQualitySettings_LiveSceneGuardBounds(t *testing.T) {
	settings := NormalizeQualitySettings(QualitySettings{
		LiveCoverSceneMinSamples: 1000,
		LiveCoverGuardMinTotal:   -2,
		LiveCoverGuardScoreFloor: 9,
	})
	if settings.LiveCoverSceneMinSamples != 100 {
		t.Fatalf("expected scene min samples clamp to 100, got %d", settings.LiveCoverSceneMinSamples)
	}
	if settings.LiveCoverGuardMinTotal != 20 {
		t.Fatalf("expected guard min total fallback to 20, got %d", settings.LiveCoverGuardMinTotal)
	}
	if settings.LiveCoverGuardScoreFloor != 0.95 {
		t.Fatalf("expected guard score floor clamp to 0.95, got %.2f", settings.LiveCoverGuardScoreFloor)
	}

	settings = NormalizeQualitySettings(QualitySettings{
		LiveCoverSceneMinSamples: -1,
		LiveCoverGuardMinTotal:   2000,
		LiveCoverGuardScoreFloor: 0,
	})
	if settings.LiveCoverSceneMinSamples != 5 {
		t.Fatalf("expected scene min samples fallback to 5, got %d", settings.LiveCoverSceneMinSamples)
	}
	if settings.LiveCoverGuardMinTotal != 1000 {
		t.Fatalf("expected guard min total clamp to 1000, got %d", settings.LiveCoverGuardMinTotal)
	}
	if settings.LiveCoverGuardScoreFloor != 0.58 {
		t.Fatalf("expected guard score floor fallback to 0.58, got %.2f", settings.LiveCoverGuardScoreFloor)
	}
}

func TestNormalizeQualitySettings_StillAndAnimatedBudgetBounds(t *testing.T) {
	settings := NormalizeQualitySettings(QualitySettings{
		GIFTargetSizeKB:                  -1,
		GIFGifsicleEnabled:               false,
		GIFGifsicleLevel:                 99,
		GIFGifsicleSkipBelowKB:           -9,
		GIFGifsicleMinGainRatio:          9,
		GIFLoopTuneEnabled:               false,
		GIFLoopTuneMinEnableSec:          -1,
		GIFLoopTuneMinImprovement:        9,
		GIFLoopTuneMotionTarget:          -1,
		GIFLoopTunePreferDuration:        99,
		GIFCandidateMaxOutputs:           100,
		GIFCandidateLongVideoMaxOutputs:  100,
		GIFCandidateUltraVideoMaxOutputs: 100,
		GIFCandidateConfidenceThreshold:  9,
		GIFCandidateDedupIOUThreshold:    -1,
		WebPTargetSizeKB:                 200000,
		JPGTargetSizeKB:                  -2,
		PNGTargetSizeKB:                  200000,
		StillMinBlurScore:                999,
		StillMinExposureScore:            -1,
		StillMinWidth:                    -100,
		StillMinHeight:                   99999,
	})
	if settings.GIFTargetSizeKB != 2048 {
		t.Fatalf("expected gif target fallback 2048, got %d", settings.GIFTargetSizeKB)
	}
	if settings.GIFGifsicleEnabled {
		t.Fatalf("expected explicit gifsicle enabled false to remain false")
	}
	if settings.GIFGifsicleLevel != 3 {
		t.Fatalf("expected gifsicle level clamp 3, got %d", settings.GIFGifsicleLevel)
	}
	if settings.GIFGifsicleSkipBelowKB != 0 {
		t.Fatalf("expected gifsicle skip threshold clamp 0, got %d", settings.GIFGifsicleSkipBelowKB)
	}
	if settings.GIFGifsicleMinGainRatio != 0.5 {
		t.Fatalf("expected gifsicle min gain ratio clamp 0.5, got %.2f", settings.GIFGifsicleMinGainRatio)
	}
	if settings.WebPTargetSizeKB != 10240 {
		t.Fatalf("expected webp target clamp 10240, got %d", settings.WebPTargetSizeKB)
	}
	if settings.GIFLoopTuneEnabled {
		t.Fatalf("expected gif loop tuning to keep explicit false")
	}
	if settings.GIFLoopTuneMinEnableSec != 1.4 {
		t.Fatalf("expected gif loop min enable fallback 1.4, got %.2f", settings.GIFLoopTuneMinEnableSec)
	}
	if settings.GIFLoopTuneMinImprovement != 0.3 {
		t.Fatalf("expected gif loop min improvement clamp 0.3, got %.2f", settings.GIFLoopTuneMinImprovement)
	}
	if settings.GIFLoopTuneMotionTarget != 0.22 {
		t.Fatalf("expected gif loop motion target fallback 0.22, got %.2f", settings.GIFLoopTuneMotionTarget)
	}
	if settings.GIFLoopTunePreferDuration != 4.0 {
		t.Fatalf("expected gif loop prefer duration clamp 4.0, got %.2f", settings.GIFLoopTunePreferDuration)
	}
	if settings.GIFCandidateMaxOutputs != maxGIFCandidateOutputs {
		t.Fatalf("expected gif candidate max outputs clamp %d, got %d", maxGIFCandidateOutputs, settings.GIFCandidateMaxOutputs)
	}
	if settings.GIFCandidateLongVideoMaxOutputs != maxGIFCandidateOutputs {
		t.Fatalf("expected gif long video max outputs clamp %d, got %d", maxGIFCandidateOutputs, settings.GIFCandidateLongVideoMaxOutputs)
	}
	if settings.GIFCandidateUltraVideoMaxOutputs != maxGIFCandidateOutputs {
		t.Fatalf("expected gif ultra video max outputs clamp %d, got %d", maxGIFCandidateOutputs, settings.GIFCandidateUltraVideoMaxOutputs)
	}
	if settings.GIFCandidateConfidenceThreshold != 0.95 {
		t.Fatalf("expected gif candidate confidence threshold clamp 0.95, got %.2f", settings.GIFCandidateConfidenceThreshold)
	}
	if settings.GIFCandidateDedupIOUThreshold != 0.45 {
		t.Fatalf("expected gif candidate dedup iou threshold fallback 0.45, got %.2f", settings.GIFCandidateDedupIOUThreshold)
	}
	if settings.JPGTargetSizeKB != 512 {
		t.Fatalf("expected jpg target fallback 512, got %d", settings.JPGTargetSizeKB)
	}
	if settings.PNGTargetSizeKB != 10240 {
		t.Fatalf("expected png target clamp 10240, got %d", settings.PNGTargetSizeKB)
	}
	if settings.StillMinBlurScore != 300 {
		t.Fatalf("expected still min blur clamp 300, got %.2f", settings.StillMinBlurScore)
	}
	if settings.StillMinExposureScore != 0.28 {
		t.Fatalf("expected still min exposure fallback 0.28, got %.2f", settings.StillMinExposureScore)
	}
	if settings.StillMinWidth != 0 {
		t.Fatalf("expected still min width clamp 0, got %d", settings.StillMinWidth)
	}
	if settings.StillMinHeight != 4096 {
		t.Fatalf("expected still min height clamp 4096, got %d", settings.StillMinHeight)
	}
}

func TestNormalizeQualitySettings_GIFCandidateTierCaps(t *testing.T) {
	settings := NormalizeQualitySettings(QualitySettings{
		GIFCandidateMaxOutputs:           4,
		GIFCandidateLongVideoMaxOutputs:  6,
		GIFCandidateUltraVideoMaxOutputs: 5,
		GIFCandidateConfidenceThreshold:  0.35,
		GIFCandidateDedupIOUThreshold:    0.45,
	})
	if settings.GIFCandidateLongVideoMaxOutputs != 4 {
		t.Fatalf("expected long tier cap <= base cap (4), got %d", settings.GIFCandidateLongVideoMaxOutputs)
	}
	if settings.GIFCandidateUltraVideoMaxOutputs != 4 {
		t.Fatalf("expected ultra tier cap <= long cap (4), got %d", settings.GIFCandidateUltraVideoMaxOutputs)
	}

	settings = NormalizeQualitySettings(QualitySettings{
		GIFCandidateMaxOutputs:           2,
		GIFCandidateLongVideoMaxOutputs:  0,
		GIFCandidateUltraVideoMaxOutputs: 0,
		GIFCandidateConfidenceThreshold:  0.35,
		GIFCandidateDedupIOUThreshold:    0.45,
	})
	if settings.GIFCandidateLongVideoMaxOutputs != 2 {
		t.Fatalf("expected long tier fallback clamped to base(2), got %d", settings.GIFCandidateLongVideoMaxOutputs)
	}
	if settings.GIFCandidateUltraVideoMaxOutputs != 2 {
		t.Fatalf("expected ultra tier fallback clamped to long(2), got %d", settings.GIFCandidateUltraVideoMaxOutputs)
	}
}

func TestNormalizeQualitySettings_AIDirectorOperatorFields(t *testing.T) {
	settings := NormalizeQualitySettings(QualitySettings{
		AIDirectorOperatorInstruction:        "   keep only emotional peaks   ",
		AIDirectorOperatorInstructionVersion: "",
		AIDirectorOperatorEnabled:            false,
	})
	if settings.AIDirectorOperatorInstruction != "keep only emotional peaks" {
		t.Fatalf("expected trimmed operator instruction, got %q", settings.AIDirectorOperatorInstruction)
	}
	if settings.AIDirectorOperatorInstructionVersion != "v1" {
		t.Fatalf("expected default operator version v1, got %q", settings.AIDirectorOperatorInstructionVersion)
	}
	if settings.AIDirectorOperatorEnabled {
		t.Fatalf("expected explicit disabled operator instruction to remain false")
	}

	defaults := NormalizeQualitySettings(QualitySettings{})
	if !defaults.AIDirectorOperatorEnabled {
		t.Fatalf("expected zero-value settings fallback to enabled=true")
	}
	if defaults.AIDirectorOperatorInstructionVersion != "v1" {
		t.Fatalf("expected default version v1, got %q", defaults.AIDirectorOperatorInstructionVersion)
	}
}

func TestNormalizeQualitySettings_AIDirectorInputMode(t *testing.T) {
	settings := NormalizeQualitySettings(QualitySettings{
		AIDirectorInputMode: " full_video ",
	})
	if settings.AIDirectorInputMode != "full_video" {
		t.Fatalf("expected normalized input mode full_video, got %q", settings.AIDirectorInputMode)
	}

	settings = NormalizeQualitySettings(QualitySettings{
		AIDirectorInputMode: "invalid_mode",
	})
	if settings.AIDirectorInputMode != "hybrid" {
		t.Fatalf("expected invalid input mode fallback to hybrid, got %q", settings.AIDirectorInputMode)
	}

	defaults := NormalizeQualitySettings(QualitySettings{})
	if defaults.AIDirectorInputMode != "hybrid" {
		t.Fatalf("expected zero-value settings default ai_director_input_mode=hybrid, got %q", defaults.AIDirectorInputMode)
	}
}

func TestNormalizeQualitySettings_AIJudgeHardGateFields(t *testing.T) {
	defaults := NormalizeQualitySettings(QualitySettings{})
	if defaults.GIFAIJudgeHardGateMinOverallScore != 0.2 ||
		defaults.GIFAIJudgeHardGateMinClarityScore != 0.2 ||
		defaults.GIFAIJudgeHardGateMinLoopScore != 0.2 ||
		defaults.GIFAIJudgeHardGateMinOutputScore != 0.2 ||
		defaults.GIFAIJudgeHardGateMinDurationMS != 200 ||
		defaults.GIFAIJudgeHardGateSizeMultiplier != 4 {
		t.Fatalf("unexpected default ai judge hard-gate: %+v", defaults)
	}

	settings := NormalizeQualitySettings(QualitySettings{
		GIFAIJudgeHardGateMinOverallScore: 0.5,
		GIFAIJudgeHardGateMinClarityScore: 0.6,
		GIFAIJudgeHardGateMinLoopScore:    0.7,
		GIFAIJudgeHardGateMinOutputScore:  0.8,
		GIFAIJudgeHardGateMinDurationMS:   500,
		GIFAIJudgeHardGateSizeMultiplier:  8,
	})
	if settings.GIFAIJudgeHardGateMinOverallScore != 0.5 ||
		settings.GIFAIJudgeHardGateMinClarityScore != 0.6 ||
		settings.GIFAIJudgeHardGateMinLoopScore != 0.7 ||
		settings.GIFAIJudgeHardGateMinOutputScore != 0.8 ||
		settings.GIFAIJudgeHardGateMinDurationMS != 500 ||
		settings.GIFAIJudgeHardGateSizeMultiplier != 8 {
		t.Fatalf("expected hard-gate values preserved, got %+v", settings)
	}
}
