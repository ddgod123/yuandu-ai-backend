package videojobs

import (
	"testing"

	"emoji/internal/models"
)

func TestResolveGIFPipelineMode_RequestOverride(t *testing.T) {
	job := models.VideoJob{Priority: "normal"}
	meta := videoProbeMeta{DurationSec: 66}
	decision := resolveGIFPipelineMode(job, meta, map[string]interface{}{
		"gif_pipeline_mode": "hq",
	}, DefaultQualitySettings())
	if decision.Mode != gifPipelineModeHQ {
		t.Fatalf("expected hq, got %s", decision.Mode)
	}
	if !decision.EnableAI1 || !decision.EnableAI2 || !decision.EnableAI3 {
		t.Fatalf("hq mode should enable ai1/ai2/ai3")
	}
}

func TestResolveGIFPipelineMode_AutoByPriority(t *testing.T) {
	job := models.VideoJob{Priority: "high"}
	meta := videoProbeMeta{DurationSec: 66}
	decision := resolveGIFPipelineMode(job, meta, map[string]interface{}{}, DefaultQualitySettings())
	if decision.Mode != gifPipelineModeHQ {
		t.Fatalf("expected hq by high priority, got %s", decision.Mode)
	}
	if decision.Reason == "" {
		t.Fatalf("expected reason")
	}
}

func TestResolveGIFPipelineMode_AutoLightByDuration(t *testing.T) {
	job := models.VideoJob{Priority: "normal"}
	shortDecision := resolveGIFPipelineMode(job, videoProbeMeta{DurationSec: 12.5}, map[string]interface{}{}, DefaultQualitySettings())
	if shortDecision.Mode != gifPipelineModeLight {
		t.Fatalf("short video should pick light, got %s", shortDecision.Mode)
	}
	if shortDecision.EnableAI1 || shortDecision.EnableAI2 || shortDecision.EnableAI3 {
		t.Fatalf("light mode should disable ai1/ai2/ai3")
	}

	longDecision := resolveGIFPipelineMode(job, videoProbeMeta{DurationSec: 240}, map[string]interface{}{}, DefaultQualitySettings())
	if longDecision.Mode != gifPipelineModeLight {
		t.Fatalf("long video should pick light, got %s", longDecision.Mode)
	}
}

func TestResolveGIFPipelineMode_AutoStandardDefault(t *testing.T) {
	job := models.VideoJob{Priority: "normal"}
	decision := resolveGIFPipelineMode(job, videoProbeMeta{DurationSec: 90}, map[string]interface{}{}, DefaultQualitySettings())
	if decision.Mode != gifPipelineModeStandard {
		t.Fatalf("expected standard, got %s", decision.Mode)
	}
	if decision.EnableAI1 {
		t.Fatalf("standard mode should not enable ai1")
	}
	if !decision.EnableAI2 || !decision.EnableAI3 {
		t.Fatalf("standard mode should enable ai2/ai3")
	}
}

func TestResolveGIFPipelineMode_ConfigurableThresholdsAndModes(t *testing.T) {
	job := models.VideoJob{Priority: "normal"}
	settings := DefaultQualitySettings()
	settings.GIFPipelineShortVideoMaxSec = 8
	settings.GIFPipelineLongVideoMinSec = 45
	settings.GIFPipelineShortVideoMode = gifPipelineModeStandard
	settings.GIFPipelineDefaultMode = gifPipelineModeHQ
	settings.GIFPipelineLongVideoMode = gifPipelineModeStandard
	settings.GIFPipelineHighPriorityEnabled = false

	shortDecision := resolveGIFPipelineMode(job, videoProbeMeta{DurationSec: 7.5}, map[string]interface{}{}, settings)
	if shortDecision.Mode != gifPipelineModeStandard {
		t.Fatalf("expected configurable short mode standard, got %s", shortDecision.Mode)
	}

	defaultDecision := resolveGIFPipelineMode(job, videoProbeMeta{DurationSec: 30}, map[string]interface{}{}, settings)
	if defaultDecision.Mode != gifPipelineModeHQ {
		t.Fatalf("expected configurable default mode hq, got %s", defaultDecision.Mode)
	}

	longDecision := resolveGIFPipelineMode(job, videoProbeMeta{DurationSec: 75}, map[string]interface{}{}, settings)
	if longDecision.Mode != gifPipelineModeStandard {
		t.Fatalf("expected configurable long mode standard, got %s", longDecision.Mode)
	}
}
