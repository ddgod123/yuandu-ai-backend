package videojobs

import (
	"errors"
	"testing"
	"time"
)

func TestPipelineObservability_RunStageTracksDurationAndFailureProfile(t *testing.T) {
	metrics := map[string]interface{}{}
	fw := newPipelineTaskFramework(nil, 99, metrics, "status_key", map[string]string{
		"worker": "pending",
	})

	if err := fw.runStage(
		pipelineTaskStageOptions{StageNames: []string{"worker"}},
		func() error { return nil },
	); err != nil {
		t.Fatalf("runStage success should not fail: %v", err)
	}

	err := fw.runStage(
		pipelineTaskStageOptions{StageNames: []string{"worker"}},
		func() error { return errors.New("planner contract invalid") },
	)
	if err == nil {
		t.Fatalf("runStage failure should return error")
	}

	obs := mapFromAny(metrics[pipelineObservabilityMetricKey])
	if obs == nil {
		t.Fatalf("expected observability metric")
	}
	stageRuns := mapFromAny(obs["stage_runs"])
	if got := intFromAny(stageRuns["worker"]); got != 2 {
		t.Fatalf("expected worker stage_runs=2, got %d", got)
	}
	stageFailures := mapFromAny(obs["stage_failures"])
	if got := intFromAny(stageFailures["worker"]); got != 1 {
		t.Fatalf("expected worker stage_failures=1, got %d", got)
	}
	failureProfile := mapFromAny(obs["failure_profile"])
	byReason := mapFromAny(failureProfile["by_reason"])
	if got := intFromAny(byReason["contract_invalid"]); got != 1 {
		t.Fatalf("expected contract_invalid=1, got %d", got)
	}
}

func TestPipelineObservability_FallbackHitRate(t *testing.T) {
	metrics := map[string]interface{}{}
	fw := newPipelineTaskFramework(nil, 100, metrics, "", nil)
	fw.recordFallback("image_ai1_director", false, "")
	fw.recordFallback("image_ai1_director", true, "director_timeout")

	obs := mapFromAny(metrics[pipelineObservabilityMetricKey])
	fallback := mapFromAny(obs["fallback"])
	if got := intFromAny(fallback["total"]); got != 2 {
		t.Fatalf("expected fallback total=2, got %d", got)
	}
	if got := intFromAny(fallback["hit"]); got != 1 {
		t.Fatalf("expected fallback hit=1, got %d", got)
	}
	rate := floatFromAny(fallback["rate"])
	if rate < 0.49 || rate > 0.51 {
		t.Fatalf("expected fallback rate≈0.5, got %f", rate)
	}
	byKey := mapFromAny(mapFromAny(fallback["by_key"])["image_ai1_director"])
	if got := intFromAny(byKey["total"]); got != 2 {
		t.Fatalf("expected by_key total=2, got %d", got)
	}
	if got := intFromAny(byKey["hit"]); got != 1 {
		t.Fatalf("expected by_key hit=1, got %d", got)
	}
	if got := stringFromAny(byKey["last_reason"]); got != "director_timeout" {
		t.Fatalf("expected last_reason director_timeout, got %q", got)
	}
}

func TestPipelineObservability_SubStageDegradedCountsFallback(t *testing.T) {
	metrics := map[string]interface{}{}
	tracker := newPipelineSubStageTracker(metrics, "gif_pipeline_sub_stages_v1", map[string]map[string]interface{}{
		gifSubStagePlanning: {"status": "pending"},
	})

	started := time.Now().Add(-120 * time.Millisecond)
	tracker.Done(gifSubStagePlanning, started, "degraded", map[string]interface{}{
		"reason": "ai_planner_unavailable",
	})

	obs := mapFromAny(metrics[pipelineObservabilityMetricKey])
	fallback := mapFromAny(obs["fallback"])
	if got := intFromAny(fallback["total"]); got != 1 {
		t.Fatalf("expected fallback total=1, got %d", got)
	}
	if got := intFromAny(fallback["hit"]); got != 1 {
		t.Fatalf("expected fallback hit=1, got %d", got)
	}
	entry := mapFromAny(mapFromAny(fallback["by_key"])["gif_sub_stage_"+gifSubStagePlanning])
	if got := intFromAny(entry["hit"]); got != 1 {
		t.Fatalf("expected planning fallback hit=1, got %d", got)
	}
}
