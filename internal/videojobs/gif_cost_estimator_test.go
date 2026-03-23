package videojobs

import "testing"

func TestEstimateGIFCandidateCost_Basic(t *testing.T) {
	meta := videoProbeMeta{
		Width:       1080,
		Height:      1920,
		DurationSec: 60,
		FPS:         30,
	}
	candidate := highlightCandidate{
		StartSec: 10,
		EndSec:   12.8,
		Score:    0.92,
	}
	settings := NormalizeQualitySettings(DefaultQualitySettings())
	estimate := estimateGIFCandidateCost(meta, candidate, jobOptions{}, settings)
	if estimate.PredictedSizeKB <= 0 {
		t.Fatalf("expected positive size estimate, got %.3f", estimate.PredictedSizeKB)
	}
	if estimate.PredictedRenderSec <= 0 {
		t.Fatalf("expected positive render estimate, got %.3f", estimate.PredictedRenderSec)
	}
	if estimate.CostUnits <= 0 {
		t.Fatalf("expected positive cost units, got %.3f", estimate.CostUnits)
	}
	if estimate.ModelVersion != gifCandidateCostModelVersion {
		t.Fatalf("unexpected model version: %s", estimate.ModelVersion)
	}
}

func TestEstimateGIFCandidateCost_LongWindowMoreExpensive(t *testing.T) {
	meta := videoProbeMeta{
		Width:       720,
		Height:      1280,
		DurationSec: 120,
		FPS:         25,
	}
	settings := NormalizeQualitySettings(DefaultQualitySettings())
	short := estimateGIFCandidateCost(meta, highlightCandidate{
		StartSec: 5,
		EndSec:   6.8,
		Score:    0.8,
	}, jobOptions{}, settings)
	long := estimateGIFCandidateCost(meta, highlightCandidate{
		StartSec: 5,
		EndSec:   9.8,
		Score:    0.8,
	}, jobOptions{}, settings)
	if long.CostUnits < short.CostUnits {
		t.Fatalf("expected long window cost >= short window cost, got long=%.3f short=%.3f", long.CostUnits, short.CostUnits)
	}
}
