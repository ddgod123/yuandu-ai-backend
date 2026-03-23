package videojobs

import "testing"

func TestBuildGIFRenderBundlePlan(t *testing.T) {
	tasks := []animatedTask{
		{Format: "gif", Window: highlightCandidate{StartSec: 1.0, EndSec: 3.0}},
		{Format: "gif", Window: highlightCandidate{StartSec: 3.4, EndSec: 5.1}},
		{Format: "gif", Window: highlightCandidate{StartSec: 12.0, EndSec: 14.0}},
		{Format: "gif", Window: highlightCandidate{StartSec: 14.3, EndSec: 16.2}},
		{Format: "jpg", Window: highlightCandidate{StartSec: 0, EndSec: 0}},
	}
	cfg := gifBundleRuntimeConfig{
		BundleEnabled:     true,
		BundleMergeGapSec: 0.8,
		BundleMaxSpanSec:  8,
	}
	plans := buildGIFRenderBundlePlan(tasks, cfg)
	if len(plans) != 2 {
		t.Fatalf("expected 2 bundles, got %d", len(plans))
	}
	if len(plans[0].TaskIndexes) != 2 || len(plans[1].TaskIndexes) != 2 {
		t.Fatalf("unexpected bundle window count: %#v", plans)
	}
}

func TestBundleWindowConvertRoundtrip(t *testing.T) {
	origin := highlightCandidate{
		StartSec: 7.5,
		EndSec:   9.9,
		Score:    0.8,
	}
	relative := toBundleRelativeWindow(origin, 5.0)
	restored := fromBundleRelativeWindow(relative, 5.0)
	if restored.StartSec != roundTo(origin.StartSec, 3) || restored.EndSec != roundTo(origin.EndSec, 3) {
		t.Fatalf("roundtrip mismatch: origin=%+v restored=%+v", origin, restored)
	}
}
