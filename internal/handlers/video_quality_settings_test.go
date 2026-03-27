package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"
)

func TestShouldApplyFeedbackRolloutRecommendation(t *testing.T) {
	tests := []struct {
		name string
		rec  AdminVideoJobFeedbackRolloutRecommendation
		want bool
	}{
		{
			name: "scale up and passed",
			rec: AdminVideoJobFeedbackRolloutRecommendation{
				State:                   "scale_up",
				ConsecutivePassed:       true,
				CurrentRolloutPercent:   30,
				SuggestedRolloutPercent: 50,
			},
			want: true,
		},
		{
			name: "scale down and passed",
			rec: AdminVideoJobFeedbackRolloutRecommendation{
				State:                   "scale_down",
				ConsecutivePassed:       true,
				CurrentRolloutPercent:   100,
				SuggestedRolloutPercent: 30,
			},
			want: true,
		},
		{
			name: "pending confirmation",
			rec: AdminVideoJobFeedbackRolloutRecommendation{
				State:                   "pending_confirmation",
				ConsecutivePassed:       false,
				CurrentRolloutPercent:   30,
				SuggestedRolloutPercent: 50,
			},
			want: false,
		},
		{
			name: "not passed",
			rec: AdminVideoJobFeedbackRolloutRecommendation{
				State:                   "scale_up",
				ConsecutivePassed:       false,
				CurrentRolloutPercent:   30,
				SuggestedRolloutPercent: 50,
			},
			want: false,
		},
		{
			name: "same rollout",
			rec: AdminVideoJobFeedbackRolloutRecommendation{
				State:                   "scale_up",
				ConsecutivePassed:       true,
				CurrentRolloutPercent:   50,
				SuggestedRolloutPercent: 50,
			},
			want: false,
		},
		{
			name: "hold state",
			rec: AdminVideoJobFeedbackRolloutRecommendation{
				State:                   "hold",
				ConsecutivePassed:       true,
				CurrentRolloutPercent:   50,
				SuggestedRolloutPercent: 100,
			},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := shouldApplyFeedbackRolloutRecommendation(tc.rec)
			if got != tc.want {
				t.Fatalf("unexpected result: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestRolloutCooldownState(t *testing.T) {
	now := time.Date(2026, 3, 11, 15, 4, 5, 0, time.UTC)

	t.Run("not in cooldown", func(t *testing.T) {
		remaining, nextAllowedAt, inCooldown := rolloutCooldownState(now, now.Add(-15*time.Minute), 10*time.Minute)
		if inCooldown {
			t.Fatalf("expected not in cooldown")
		}
		if remaining != 0 {
			t.Fatalf("expected zero remaining, got %d", remaining)
		}
		if nextAllowedAt.IsZero() {
			t.Fatalf("expected next allowed time")
		}
	})

	t.Run("in cooldown", func(t *testing.T) {
		remaining, nextAllowedAt, inCooldown := rolloutCooldownState(now, now.Add(-3*time.Minute), 10*time.Minute)
		if !inCooldown {
			t.Fatalf("expected in cooldown")
		}
		if remaining < 420 || remaining > 421 {
			t.Fatalf("unexpected remaining seconds: %d", remaining)
		}
		expected := now.Add(7 * time.Minute)
		if !nextAllowedAt.Equal(expected) {
			t.Fatalf("next allowed mismatch: got %s want %s", nextAllowedAt, expected)
		}
	})
}

func TestBuildRolloutAuditMetadata(t *testing.T) {
	rec := AdminVideoJobFeedbackRolloutRecommendation{
		State:                   "scale_up",
		Reason:                  "unit-test",
		CurrentRolloutPercent:   30,
		SuggestedRolloutPercent: 50,
		ConsecutiveRequired:     3,
		ConsecutiveMatched:      3,
		ConsecutivePassed:       true,
		RecentStates:            []string{"scale_up", "scale_up", "scale_up"},
	}

	raw, err := buildRolloutAuditMetadata(rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected non-empty metadata")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid json metadata: %v", err)
	}
	if parsed["state"] != "scale_up" {
		t.Fatalf("unexpected state in metadata: %+v", parsed["state"])
	}
}

func TestQualitySettingsModelRoundTrip_LivePortraitWeight(t *testing.T) {
	settings := qualitySettingsFromModel(models.VideoQualitySetting{})
	if settings.LiveCoverPortraitWeight != 0.04 {
		t.Fatalf("expected default portrait weight 0.04, got %.2f", settings.LiveCoverPortraitWeight)
	}
	if settings.GIFTargetSizeKB != 2048 || settings.WebPTargetSizeKB != 1536 {
		t.Fatalf("expected default animated targets 2048/1536, got %d/%d", settings.GIFTargetSizeKB, settings.WebPTargetSizeKB)
	}
	if !settings.GIFGifsicleEnabled || settings.GIFGifsicleLevel != 2 {
		t.Fatalf("expected default gifsicle enabled/level=true/2, got %v/%d", settings.GIFGifsicleEnabled, settings.GIFGifsicleLevel)
	}
	if settings.GIFGifsicleSkipBelowKB != 256 || settings.GIFGifsicleMinGainRatio != 0.03 {
		t.Fatalf("expected default gifsicle skip/min_gain=256/0.03, got %d/%.2f", settings.GIFGifsicleSkipBelowKB, settings.GIFGifsicleMinGainRatio)
	}
	if !settings.GIFLoopTuneEnabled {
		t.Fatalf("expected gif loop tuning enabled by default")
	}
	if settings.GIFLoopTuneMinEnableSec != 1.4 || settings.GIFLoopTuneMinImprovement != 0.04 {
		t.Fatalf("expected default gif loop tune min enable/improvement 1.4/0.04, got %.2f/%.2f", settings.GIFLoopTuneMinEnableSec, settings.GIFLoopTuneMinImprovement)
	}
	if settings.GIFLoopTuneMotionTarget != 0.22 || settings.GIFLoopTunePreferDuration != 2.4 {
		t.Fatalf("expected default gif loop motion/prefer duration 0.22/2.4, got %.2f/%.2f", settings.GIFLoopTuneMotionTarget, settings.GIFLoopTunePreferDuration)
	}
	if settings.JPGTargetSizeKB != 512 || settings.PNGTargetSizeKB != 1024 {
		t.Fatalf("expected default still budgets 512/1024, got %d/%d", settings.JPGTargetSizeKB, settings.PNGTargetSizeKB)
	}
	if settings.StillMinBlurScore != 12 || settings.StillMinExposureScore != 0.28 {
		t.Fatalf("expected default still gates 12/0.28, got %.2f/%.2f", settings.StillMinBlurScore, settings.StillMinExposureScore)
	}
	if settings.StillMinWidth != 0 || settings.StillMinHeight != 0 {
		t.Fatalf("expected default still size 0x0, got %dx%d", settings.StillMinWidth, settings.StillMinHeight)
	}
	if settings.LiveCoverSceneMinSamples != 5 {
		t.Fatalf("expected default scene min samples 5, got %d", settings.LiveCoverSceneMinSamples)
	}
	if settings.LiveCoverGuardMinTotal != 20 {
		t.Fatalf("expected default guard min total 20, got %d", settings.LiveCoverGuardMinTotal)
	}
	if settings.LiveCoverGuardScoreFloor != 0.58 {
		t.Fatalf("expected default guard floor 0.58, got %.2f", settings.LiveCoverGuardScoreFloor)
	}
	if !settings.HighlightNegativeGuardEnabled {
		t.Fatalf("expected default negative guard enabled")
	}
	if settings.HighlightNegativeGuardThreshold != 0.45 {
		t.Fatalf("expected default negative guard threshold 0.45, got %.2f", settings.HighlightNegativeGuardThreshold)
	}
	if settings.HighlightNegativeGuardMinWeight != 4 {
		t.Fatalf("expected default negative guard min weight 4, got %.2f", settings.HighlightNegativeGuardMinWeight)
	}
	if settings.GIFAIJudgeHardGateMinOverallScore != 0.2 ||
		settings.GIFAIJudgeHardGateMinClarityScore != 0.2 ||
		settings.GIFAIJudgeHardGateMinLoopScore != 0.2 ||
		settings.GIFAIJudgeHardGateMinOutputScore != 0.2 ||
		settings.GIFAIJudgeHardGateMinDurationMS != 200 ||
		settings.GIFAIJudgeHardGateSizeMultiplier != 4 {
		t.Fatalf("expected default hard-gate 0.2/0.2/0.2/0.2/200/4, got %.2f/%.2f/%.2f/%.2f/%d/%d",
			settings.GIFAIJudgeHardGateMinOverallScore,
			settings.GIFAIJudgeHardGateMinClarityScore,
			settings.GIFAIJudgeHardGateMinLoopScore,
			settings.GIFAIJudgeHardGateMinOutputScore,
			settings.GIFAIJudgeHardGateMinDurationMS,
			settings.GIFAIJudgeHardGateSizeMultiplier,
		)
	}

	settings.LiveCoverPortraitWeight = 0.12
	settings.GIFTargetSizeKB = 1024
	settings.GIFGifsicleEnabled = true
	settings.GIFGifsicleLevel = 3
	settings.GIFGifsicleSkipBelowKB = 512
	settings.GIFGifsicleMinGainRatio = 0.08
	settings.GIFLoopTuneEnabled = false
	settings.GIFLoopTuneMinEnableSec = 1.1
	settings.GIFLoopTuneMinImprovement = 0.02
	settings.GIFLoopTuneMotionTarget = 0.18
	settings.GIFLoopTunePreferDuration = 2.8
	settings.WebPTargetSizeKB = 896
	settings.JPGTargetSizeKB = 420
	settings.PNGTargetSizeKB = 990
	settings.StillMinBlurScore = 20
	settings.StillMinExposureScore = 0.4
	settings.StillMinWidth = 720
	settings.StillMinHeight = 400
	settings.LiveCoverSceneMinSamples = 9
	settings.LiveCoverGuardMinTotal = 40
	settings.LiveCoverGuardScoreFloor = 0.62
	settings.HighlightNegativeGuardEnabled = true
	settings.HighlightNegativeGuardThreshold = 0.52
	settings.HighlightNegativeGuardMinWeight = 6
	settings.HighlightNegativePenaltyScale = 0.6
	settings.HighlightNegativePenaltyWeight = 1.2
	settings.GIFAIJudgeHardGateMinOverallScore = 0.35
	settings.GIFAIJudgeHardGateMinClarityScore = 0.4
	settings.GIFAIJudgeHardGateMinLoopScore = 0.45
	settings.GIFAIJudgeHardGateMinOutputScore = 0.3
	settings.GIFAIJudgeHardGateMinDurationMS = 350
	settings.GIFAIJudgeHardGateSizeMultiplier = 6
	var row models.VideoQualitySetting
	applyQualitySettingsToModel(&row, settings)
	if row.LiveCoverPortraitWeight != 0.12 {
		t.Fatalf("expected model portrait weight 0.12, got %.2f", row.LiveCoverPortraitWeight)
	}
	if row.GIFTargetSizeKB != 1024 || row.WebPTargetSizeKB != 896 {
		t.Fatalf("expected model animated targets 1024/896, got %d/%d", row.GIFTargetSizeKB, row.WebPTargetSizeKB)
	}
	if !row.GIFGifsicleEnabled || row.GIFGifsicleLevel != 3 {
		t.Fatalf("expected model gifsicle enabled/level=true/3, got %v/%d", row.GIFGifsicleEnabled, row.GIFGifsicleLevel)
	}
	if row.GIFGifsicleSkipBelowKB != 512 || row.GIFGifsicleMinGainRatio != 0.08 {
		t.Fatalf("expected model gifsicle skip/min_gain=512/0.08, got %d/%.2f", row.GIFGifsicleSkipBelowKB, row.GIFGifsicleMinGainRatio)
	}
	if row.GIFLoopTuneEnabled {
		t.Fatalf("expected model gif loop tuning disabled")
	}
	if row.GIFLoopTuneMinEnableSec != 1.1 || row.GIFLoopTuneMinImprovement != 0.02 {
		t.Fatalf("expected model gif loop tune min enable/improvement 1.1/0.02, got %.2f/%.2f", row.GIFLoopTuneMinEnableSec, row.GIFLoopTuneMinImprovement)
	}
	if row.GIFLoopTuneMotionTarget != 0.18 || row.GIFLoopTunePreferDuration != 2.8 {
		t.Fatalf("expected model gif loop motion/prefer duration 0.18/2.8, got %.2f/%.2f", row.GIFLoopTuneMotionTarget, row.GIFLoopTunePreferDuration)
	}
	if row.JPGTargetSizeKB != 420 || row.PNGTargetSizeKB != 990 {
		t.Fatalf("expected model still budgets 420/990, got %d/%d", row.JPGTargetSizeKB, row.PNGTargetSizeKB)
	}
	if row.StillMinBlurScore != 20 || row.StillMinExposureScore != 0.4 {
		t.Fatalf("expected model still gates 20/0.4, got %.2f/%.2f", row.StillMinBlurScore, row.StillMinExposureScore)
	}
	if row.StillMinWidth != 720 || row.StillMinHeight != 400 {
		t.Fatalf("expected model still size 720x400, got %dx%d", row.StillMinWidth, row.StillMinHeight)
	}
	if row.LiveCoverSceneMinSamples != 9 {
		t.Fatalf("expected model scene min samples 9, got %d", row.LiveCoverSceneMinSamples)
	}
	if row.LiveCoverGuardMinTotal != 40 {
		t.Fatalf("expected model guard min total 40, got %d", row.LiveCoverGuardMinTotal)
	}
	if row.LiveCoverGuardScoreFloor != 0.62 {
		t.Fatalf("expected model guard floor 0.62, got %.2f", row.LiveCoverGuardScoreFloor)
	}
	if !row.HighlightNegativeGuardEnabled {
		t.Fatalf("expected model negative guard enabled")
	}
	if row.HighlightNegativeGuardThreshold != 0.52 {
		t.Fatalf("expected model negative guard threshold 0.52, got %.2f", row.HighlightNegativeGuardThreshold)
	}
	if row.HighlightNegativeGuardMinWeight != 6 {
		t.Fatalf("expected model negative guard min weight 6, got %.2f", row.HighlightNegativeGuardMinWeight)
	}
	if row.HighlightNegativePenaltyScale != 0.6 || row.HighlightNegativePenaltyWeight != 1.2 {
		t.Fatalf("expected model negative penalty 0.6/1.2, got %.2f/%.2f", row.HighlightNegativePenaltyScale, row.HighlightNegativePenaltyWeight)
	}
	if row.GIFAIJudgeHardGateMinOverallScore != 0.35 ||
		row.GIFAIJudgeHardGateMinClarityScore != 0.4 ||
		row.GIFAIJudgeHardGateMinLoopScore != 0.45 ||
		row.GIFAIJudgeHardGateMinOutputScore != 0.3 ||
		row.GIFAIJudgeHardGateMinDurationMS != 350 ||
		row.GIFAIJudgeHardGateSizeMultiplier != 6 {
		t.Fatalf("expected model hard-gate 0.35/0.4/0.45/0.3/350/6, got %.2f/%.2f/%.2f/%.2f/%d/%d",
			row.GIFAIJudgeHardGateMinOverallScore,
			row.GIFAIJudgeHardGateMinClarityScore,
			row.GIFAIJudgeHardGateMinLoopScore,
			row.GIFAIJudgeHardGateMinOutputScore,
			row.GIFAIJudgeHardGateMinDurationMS,
			row.GIFAIJudgeHardGateSizeMultiplier,
		)
	}
}

func TestToVideoQualitySettingResponse_IncludesGIFHealthAlertThresholds(t *testing.T) {
	row := models.VideoQualitySetting{
		GIFHealthDoneRateWarn:                             0.88,
		GIFHealthDoneRateCritical:                         0.66,
		GIFHealthFailedRateWarn:                           0.12,
		GIFHealthFailedRateCritical:                       0.28,
		GIFHealthPathStrictRateWarn:                       0.93,
		GIFHealthPathStrictRateCritical:                   0.52,
		GIFHealthLoopFallbackRateWarn:                     0.35,
		GIFHealthLoopFallbackRateCritical:                 0.63,
		FeedbackIntegrityOutputCoverageRateWarn:           0.98,
		FeedbackIntegrityOutputCoverageRateCritical:       0.95,
		FeedbackIntegrityOutputResolvedRateWarn:           0.99,
		FeedbackIntegrityOutputResolvedRateCritical:       0.97,
		FeedbackIntegrityOutputJobConsistencyRateWarn:     0.999,
		FeedbackIntegrityOutputJobConsistencyRateCritical: 0.995,
		FeedbackIntegrityTopPickConflictUsersWarn:         1,
		FeedbackIntegrityTopPickConflictUsersCritical:     3,
	}
	resp := toVideoQualitySettingResponse(row, false)
	if resp.GIFHealthDoneRateWarn != 0.88 || resp.GIFHealthDoneRateCritical != 0.66 {
		t.Fatalf("unexpected done thresholds in response: %+v", resp.GIFHealthAlertThresholdSettings)
	}
	if resp.GIFHealthPathStrictRateWarn != 0.93 || resp.GIFHealthLoopFallbackRateCritical != 0.63 {
		t.Fatalf("unexpected path/fallback thresholds in response: %+v", resp.GIFHealthAlertThresholdSettings)
	}
	if resp.FeedbackIntegrityOutputCoverageRateWarn != 0.98 || resp.FeedbackIntegrityOutputCoverageRateCritical != 0.95 {
		t.Fatalf("unexpected feedback integrity coverage thresholds: %+v", resp.FeedbackIntegrityAlertThresholdSettings)
	}
	if resp.FeedbackIntegrityTopPickConflictUsersWarn != 1 || resp.FeedbackIntegrityTopPickConflictUsersCritical != 3 {
		t.Fatalf("unexpected feedback integrity top_pick thresholds: %+v", resp.FeedbackIntegrityAlertThresholdSettings)
	}
}

func makeValidVideoQualitySettingRequest(t *testing.T) VideoQualitySettingRequest {
	t.Helper()
	var row models.VideoQualitySetting
	applyQualitySettingsToModel(&row, videojobs.DefaultQualitySettings())
	applyGIFHealthAlertThresholdSettingsToModel(&row, defaultGIFHealthAlertThresholdSettings())
	resp := toVideoQualitySettingResponse(row, false)
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal default response failed: %v", err)
	}
	var req VideoQualitySettingRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal request failed: %v", err)
	}
	return req
}

func TestValidateVideoQualitySettingRequest(t *testing.T) {
	t.Run("valid default request", func(t *testing.T) {
		req := makeValidVideoQualitySettingRequest(t)
		if err := validateVideoQualitySettingRequest(req); err != nil {
			t.Fatalf("expected default request valid, got error: %v", err)
		}
	})

	t.Run("invalid done rate threshold relation", func(t *testing.T) {
		req := makeValidVideoQualitySettingRequest(t)
		req.GIFHealthDoneRateWarn = 0.7
		req.GIFHealthDoneRateCritical = 0.72
		if err := validateVideoQualitySettingRequest(req); err == nil {
			t.Fatalf("expected threshold relation validation error")
		}
	})

	t.Run("invalid highlight weight sum", func(t *testing.T) {
		req := makeValidVideoQualitySettingRequest(t)
		req.HighlightWeightPosition = 0
		req.HighlightWeightDuration = 0
		req.HighlightWeightReason = 0
		if err := validateVideoQualitySettingRequest(req); err == nil {
			t.Fatalf("expected highlight weight sum validation error")
		}
	})

	t.Run("invalid gifsicle level", func(t *testing.T) {
		req := makeValidVideoQualitySettingRequest(t)
		req.GIFGifsicleLevel = 9
		if err := validateVideoQualitySettingRequest(req); err == nil {
			t.Fatalf("expected gifsicle level validation error")
		}
	})

	t.Run("invalid gifsicle min gain ratio", func(t *testing.T) {
		req := makeValidVideoQualitySettingRequest(t)
		req.GIFGifsicleMinGainRatio = 0.8
		if err := validateVideoQualitySettingRequest(req); err == nil {
			t.Fatalf("expected gifsicle min gain validation error")
		}
	})

	t.Run("invalid feedback integrity top_pick threshold relation", func(t *testing.T) {
		req := makeValidVideoQualitySettingRequest(t)
		req.FeedbackIntegrityTopPickConflictUsersWarn = 2
		req.FeedbackIntegrityTopPickConflictUsersCritical = 2
		if err := validateVideoQualitySettingRequest(req); err == nil {
			t.Fatalf("expected top_pick conflict threshold validation error")
		}
	})

	t.Run("invalid ai3 hard gate min output score", func(t *testing.T) {
		req := makeValidVideoQualitySettingRequest(t)
		req.GIFAIJudgeHardGateMinOutputScore = 0
		if err := validateVideoQualitySettingRequest(req); err == nil {
			t.Fatalf("expected ai3 hard gate score validation error")
		}
	})

	t.Run("invalid ai3 hard gate duration", func(t *testing.T) {
		req := makeValidVideoQualitySettingRequest(t)
		req.GIFAIJudgeHardGateMinDurationMS = 10
		if err := validateVideoQualitySettingRequest(req); err == nil {
			t.Fatalf("expected ai3 hard gate duration validation error")
		}
	})
}

func TestBuildVideoQualitySettingChangedFields(t *testing.T) {
	before := makeValidVideoQualitySettingRequest(t)
	after := before
	after.GIFAIJudgeHardGateMinClarityScore = 0.35
	after.GIFAIJudgeHardGateMinDurationMS = 320

	changes, err := buildVideoQualitySettingChangedFields(before, after)
	if err != nil {
		t.Fatalf("buildVideoQualitySettingChangedFields error: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changed fields, got %d", len(changes))
	}
	if changes[0].Field != "gif_ai_judge_hard_gate_min_clarity_score" {
		t.Fatalf("unexpected first changed field: %+v", changes[0])
	}
	if changes[1].Field != "gif_ai_judge_hard_gate_min_duration_ms" {
		t.Fatalf("unexpected second changed field: %+v", changes[1])
	}
}
