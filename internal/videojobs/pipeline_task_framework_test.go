package videojobs

import (
	"errors"
	"testing"
)

func TestPipelineTaskFrameworkMarkStageStatus(t *testing.T) {
	metrics := map[string]interface{}{}
	stageStatus := map[string]string{
		"ai1": "pending",
	}
	fw := newPipelineTaskFramework(nil, 1, metrics, "image_pipeline_stage_status_v1", stageStatus)
	fw.markStageStatus("ai2", "running")

	if got := stageStatus["ai2"]; got != "running" {
		t.Fatalf("expected ai2 stage running, got %s", got)
	}
	if got, ok := metrics["image_pipeline_stage_status_v1"]; !ok || got == nil {
		t.Fatalf("expected stage status metric in metrics")
	}
}

func TestPipelineTaskFrameworkRunStageFailureMarksFailed(t *testing.T) {
	fw := newPipelineTaskFramework(nil, 1, map[string]interface{}{}, "status_key", map[string]string{
		"worker": "pending",
	})
	err := fw.runStage(
		pipelineTaskStageOptions{
			StageNames: []string{"worker"},
		},
		func() error {
			return errors.New("boom")
		},
	)
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := fw.stageStatus["worker"]; got != "failed" {
		t.Fatalf("expected worker failed, got %s", got)
	}
}

func TestPipelineSubStageTrackerLifecycle(t *testing.T) {
	metrics := map[string]interface{}{}
	tracker := newPipelineSubStageTracker(metrics, "gif_pipeline_sub_stages_v1", map[string]map[string]interface{}{
		gifSubStageBriefing: {"status": "pending"},
	})
	if tracker.HasFinalStatus(gifSubStageBriefing) {
		t.Fatalf("pending stage should not be final")
	}

	started := tracker.Start(gifSubStageBriefing, map[string]interface{}{"entry_stage": "analyzing"})
	if started.IsZero() {
		t.Fatalf("started should not be zero")
	}
	if tracker.HasFinalStatus(gifSubStageBriefing) {
		t.Fatalf("running stage should not be final")
	}

	tracker.Done(gifSubStageBriefing, started, "done", map[string]interface{}{"applied": true})
	if !tracker.HasFinalStatus(gifSubStageBriefing) {
		t.Fatalf("done stage should be final")
	}
	status := tracker.StatusSnapshot()
	if status[gifSubStageBriefing] != "done" {
		t.Fatalf("expected done status, got %s", status[gifSubStageBriefing])
	}
	if _, ok := metrics["gif_pipeline_sub_stages_v1"]; !ok {
		t.Fatalf("expected sub stage metric synced")
	}
}
