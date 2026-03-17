package videojobs

import (
	"testing"

	"gorm.io/datatypes"
)

func TestParseSceneMetadataOutput(t *testing.T) {
	raw := []byte(`
frame:0    pts:25600   pts_time:2
lavfi.scene_score=0.400000
frame:1    pts:51200   pts_time:4
lavfi.scene_score=0.600000
`)
	points := parseSceneMetadataOutput(raw)
	if len(points) != 2 {
		t.Fatalf("expected 2 scene points, got %d", len(points))
	}
	if points[0].PtsSec != 4 || points[0].Score != 0.6 {
		t.Fatalf("unexpected top point: %+v", points[0])
	}
	if points[1].PtsSec != 2 || points[1].Score != 0.4 {
		t.Fatalf("unexpected second point: %+v", points[1])
	}
}

func TestPickNonOverlapCandidates(t *testing.T) {
	candidates := []highlightCandidate{
		{StartSec: 1, EndSec: 4, Score: 0.9},
		{StartSec: 1.2, EndSec: 4.2, Score: 0.8}, // overlap with #1
		{StartSec: 5, EndSec: 8, Score: 0.7},
	}
	selected := pickNonOverlapCandidates(candidates, 3, 0.45)
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected candidates, got %d", len(selected))
	}
	if selected[0].Score != 0.9 || selected[1].Score != 0.7 {
		t.Fatalf("unexpected selected list: %+v", selected)
	}
}

func TestParseJobOptions_AutoHighlightDefaultAndOverride(t *testing.T) {
	defaultOpts := parseJobOptions(datatypes.JSON([]byte(`{"max_static":12}`)))
	if !defaultOpts.AutoHighlight {
		t.Fatalf("expected auto highlight enabled by default")
	}

	disabled := parseJobOptions(datatypes.JSON([]byte(`{"auto_highlight":false}`)))
	if disabled.AutoHighlight {
		t.Fatalf("expected auto highlight disabled when explicitly set")
	}
}

func TestSelectHighlightCandidatesForExtraction(t *testing.T) {
	candidates := []highlightCandidate{
		{StartSec: 1, EndSec: 3, Score: 0.9},
		{StartSec: 4, EndSec: 6, Score: 0.8},
		{StartSec: 6, EndSec: 6, Score: 0.7}, // invalid, should skip
		{StartSec: 7, EndSec: 9, Score: 0.6},
	}
	selected := selectHighlightCandidatesForExtraction(candidates, 2, 3)
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected candidates, got %d", len(selected))
	}
	if selected[0].StartSec != 1 || selected[1].StartSec != 4 {
		t.Fatalf("unexpected candidate order: %+v", selected)
	}
}

func TestSelectHighlightCandidatesForExtraction_MaxOutputs(t *testing.T) {
	candidates := []highlightCandidate{
		{StartSec: 1, EndSec: 3, Score: 0.9},
		{StartSec: 4, EndSec: 6, Score: 0.8},
		{StartSec: 7, EndSec: 9, Score: 0.7},
	}
	selected := selectHighlightCandidatesForExtraction(candidates, 24, 1)
	if len(selected) != 1 {
		t.Fatalf("expected 1 selected candidate, got %d", len(selected))
	}
	if selected[0].StartSec != 1 {
		t.Fatalf("unexpected selected candidate: %+v", selected[0])
	}
}

func TestAllocateFrameBudgets(t *testing.T) {
	got := allocateFrameBudgets(25, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 budgets, got %d", len(got))
	}
	if got[0] != 9 || got[1] != 8 || got[2] != 8 {
		t.Fatalf("unexpected budget split: %+v", got)
	}
}

func TestApplyGIFCandidateConfidenceThreshold(t *testing.T) {
	selected := []highlightCandidate{
		{StartSec: 0, EndSec: 2, Score: 0.90},
		{StartSec: 2, EndSec: 4, Score: 0.20},
		{StartSec: 4, EndSec: 6, Score: 0.60},
	}
	filtered := applyGIFCandidateConfidenceThreshold(selected, nil, 0.45)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 candidates after confidence filter, got %d", len(filtered))
	}
	if filtered[0].Score != 0.90 || filtered[1].Score != 0.60 {
		t.Fatalf("unexpected filtered candidates: %+v", filtered)
	}
}
