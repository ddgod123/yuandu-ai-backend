package handlers

import "testing"

func TestBuildFeedbackRolloutRecommendationScaleUp(t *testing.T) {
	rec := buildFeedbackRolloutRecommendation(true, 30, []AdminVideoJobFeedbackGroupStat{
		{Group: "control", Jobs: 60, FeedbackSignals: 120, AvgEngagementScore: 2.0},
		{Group: "treatment", Jobs: 58, FeedbackSignals: 150, AvgEngagementScore: 2.2},
	})

	if rec.State != "scale_up" {
		t.Fatalf("expected scale_up, got %s", rec.State)
	}
	if rec.SuggestedRolloutPercent != 50 {
		t.Fatalf("expected suggested rollout 50, got %d", rec.SuggestedRolloutPercent)
	}
}

func TestBuildFeedbackRolloutRecommendationScaleDown(t *testing.T) {
	rec := buildFeedbackRolloutRecommendation(true, 50, []AdminVideoJobFeedbackGroupStat{
		{Group: "control", Jobs: 80, FeedbackSignals: 220, AvgEngagementScore: 2.5},
		{Group: "treatment", Jobs: 82, FeedbackSignals: 160, AvgEngagementScore: 2.1},
	})

	if rec.State != "scale_down" {
		t.Fatalf("expected scale_down, got %s", rec.State)
	}
	if rec.SuggestedRolloutPercent != 10 {
		t.Fatalf("expected suggested rollout 10, got %d", rec.SuggestedRolloutPercent)
	}
}

func TestBuildFeedbackRolloutRecommendationInsufficientData(t *testing.T) {
	rec := buildFeedbackRolloutRecommendation(true, 30, []AdminVideoJobFeedbackGroupStat{
		{Group: "control", Jobs: 10, FeedbackSignals: 10, AvgEngagementScore: 1.2},
		{Group: "treatment", Jobs: 11, FeedbackSignals: 15, AvgEngagementScore: 1.3},
	})

	if rec.State != "insufficient_data" {
		t.Fatalf("expected insufficient_data, got %s", rec.State)
	}
}

func TestBuildFeedbackRolloutRecommendationDisabled(t *testing.T) {
	rec := buildFeedbackRolloutRecommendation(false, 30, nil)
	if rec.State != "disabled" {
		t.Fatalf("expected disabled, got %s", rec.State)
	}
}

func TestBuildFeedbackRolloutRecommendationWithHistoryPending(t *testing.T) {
	history := [][]AdminVideoJobFeedbackGroupStat{
		{
			{Group: "control", Jobs: 60, FeedbackSignals: 120, AvgEngagementScore: 2.0},
			{Group: "treatment", Jobs: 58, FeedbackSignals: 150, AvgEngagementScore: 2.2},
		},
		{
			{Group: "control", Jobs: 61, FeedbackSignals: 122, AvgEngagementScore: 2.0},
			{Group: "treatment", Jobs: 59, FeedbackSignals: 145, AvgEngagementScore: 2.1},
		},
		{
			{Group: "control", Jobs: 62, FeedbackSignals: 124, AvgEngagementScore: 2.0},
			{Group: "treatment", Jobs: 57, FeedbackSignals: 110, AvgEngagementScore: 1.9},
		},
	}
	rec := buildFeedbackRolloutRecommendationWithHistory(true, 30, history, 3, nil, defaultLiveCoverSceneGuardConfig())
	if rec.State != "pending_confirmation" {
		t.Fatalf("expected pending_confirmation, got %s", rec.State)
	}
	if rec.ConsecutiveMatched != 2 || rec.ConsecutiveRequired != 3 {
		t.Fatalf("unexpected consecutive values: %+v", rec)
	}
}

func TestBuildFeedbackRolloutRecommendationWithHistoryPassed(t *testing.T) {
	history := [][]AdminVideoJobFeedbackGroupStat{
		{
			{Group: "control", Jobs: 60, FeedbackSignals: 120, AvgEngagementScore: 2.0},
			{Group: "treatment", Jobs: 58, FeedbackSignals: 150, AvgEngagementScore: 2.2},
		},
		{
			{Group: "control", Jobs: 61, FeedbackSignals: 122, AvgEngagementScore: 2.0},
			{Group: "treatment", Jobs: 59, FeedbackSignals: 152, AvgEngagementScore: 2.2},
		},
		{
			{Group: "control", Jobs: 62, FeedbackSignals: 124, AvgEngagementScore: 2.0},
			{Group: "treatment", Jobs: 57, FeedbackSignals: 148, AvgEngagementScore: 2.3},
		},
	}
	rec := buildFeedbackRolloutRecommendationWithHistory(true, 30, history, 3, nil, defaultLiveCoverSceneGuardConfig())
	if rec.State != "scale_up" {
		t.Fatalf("expected scale_up, got %s", rec.State)
	}
	if !rec.ConsecutivePassed {
		t.Fatalf("expected consecutive passed")
	}
	if rec.SuggestedRolloutPercent != 50 {
		t.Fatalf("expected suggested rollout 50, got %d", rec.SuggestedRolloutPercent)
	}
}

func TestBuildFeedbackRolloutRecommendationWithHistoryScaleUpGuardedByLiveScenes(t *testing.T) {
	history := [][]AdminVideoJobFeedbackGroupStat{
		{
			{Group: "control", Jobs: 60, FeedbackSignals: 120, AvgEngagementScore: 2.0},
			{Group: "treatment", Jobs: 58, FeedbackSignals: 152, AvgEngagementScore: 2.3},
		},
		{
			{Group: "control", Jobs: 62, FeedbackSignals: 124, AvgEngagementScore: 2.0},
			{Group: "treatment", Jobs: 59, FeedbackSignals: 154, AvgEngagementScore: 2.3},
		},
		{
			{Group: "control", Jobs: 61, FeedbackSignals: 121, AvgEngagementScore: 2.0},
			{Group: "treatment", Jobs: 60, FeedbackSignals: 156, AvgEngagementScore: 2.3},
		},
	}
	sceneStats := []AdminVideoJobLiveCoverSceneStat{
		{SceneTag: "explore", Samples: 15, AvgCoverScore: 0.52},
		{SceneTag: "food", Samples: 8, AvgCoverScore: 0.57},
	}
	rec := buildFeedbackRolloutRecommendationWithHistory(true, 30, history, 3, sceneStats, defaultLiveCoverSceneGuardConfig())
	if rec.State != "hold" {
		t.Fatalf("expected hold after guard, got %s", rec.State)
	}
	if rec.SuggestedRolloutPercent != 30 {
		t.Fatalf("expected rollout unchanged to 30, got %d", rec.SuggestedRolloutPercent)
	}
	if !rec.LiveGuardTriggered {
		t.Fatalf("expected live guard triggered")
	}
	if rec.LiveGuardEligibleTotal < 20 {
		t.Fatalf("expected eligible total >= 20, got %d", rec.LiveGuardEligibleTotal)
	}
	if len(rec.LiveGuardRiskScenes) == 0 {
		t.Fatalf("expected risk scenes")
	}
}

func TestEvaluateLiveCoverSceneGuardInsufficientSamples(t *testing.T) {
	triggered, eligible, risks := evaluateLiveCoverSceneGuard([]AdminVideoJobLiveCoverSceneStat{
		{SceneTag: "explore", Samples: 3, AvgCoverScore: 0.20},
		{SceneTag: "food", Samples: 4, AvgCoverScore: 0.30},
	}, defaultLiveCoverSceneGuardConfig())
	if triggered {
		t.Fatalf("expected guard not triggered for low samples")
	}
	if eligible != 0 {
		t.Fatalf("expected eligible total 0, got %d", eligible)
	}
	if len(risks) != 0 {
		t.Fatalf("expected no risks, got %v", risks)
	}
}
