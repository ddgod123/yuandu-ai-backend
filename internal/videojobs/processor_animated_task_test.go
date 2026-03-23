package videojobs

import "testing"

func TestBuildExpandedInvalidGIFWindow(t *testing.T) {
	t.Run("expand short window", func(t *testing.T) {
		window := highlightCandidate{StartSec: 10, EndSec: 12}
		expanded, ok := buildExpandedInvalidGIFWindow(window, 180)
		if !ok {
			t.Fatalf("expected expandable window")
		}
		if expanded.EndSec-expanded.StartSec < 2.79 {
			t.Fatalf("expected expanded duration >= 2.8, got %.3f", expanded.EndSec-expanded.StartSec)
		}
	})

	t.Run("no expand for already long window", func(t *testing.T) {
		window := highlightCandidate{StartSec: 10, EndSec: 13.2}
		_, ok := buildExpandedInvalidGIFWindow(window, 180)
		if ok {
			t.Fatalf("expected no expansion for long window")
		}
	})

	t.Run("clamp at start boundary", func(t *testing.T) {
		window := highlightCandidate{StartSec: 0.1, EndSec: 2.1}
		expanded, ok := buildExpandedInvalidGIFWindow(window, 60)
		if !ok {
			t.Fatalf("expected expandable window")
		}
		if expanded.StartSec < 0 {
			t.Fatalf("start should be clamped >= 0, got %.3f", expanded.StartSec)
		}
	})

	t.Run("clamp at end boundary", func(t *testing.T) {
		window := highlightCandidate{StartSec: 57.9, EndSec: 59.9}
		expanded, ok := buildExpandedInvalidGIFWindow(window, 60)
		if !ok {
			t.Fatalf("expected expandable window")
		}
		if expanded.EndSec > 60 {
			t.Fatalf("end should be clamped <= source duration, got %.3f", expanded.EndSec)
		}
		if expanded.EndSec-expanded.StartSec < 2.79 {
			t.Fatalf("expected expanded duration >= 2.8, got %.3f", expanded.EndSec-expanded.StartSec)
		}
	})
}
