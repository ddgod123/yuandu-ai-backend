package videojobs

import "testing"

func TestKnownGIFCandidateRejectReasonsIncludesFineReasons(t *testing.T) {
	cases := []string{
		GIFCandidateRejectReasonLowEmotion,
		GIFCandidateRejectReasonLowConfidence,
		GIFCandidateRejectReasonDuplicate,
		GIFCandidateRejectReasonBlurLow,
		GIFCandidateRejectReasonSizeBudgetExceeded,
		GIFCandidateRejectReasonLoopPoor,
	}
	for _, item := range cases {
		if !IsKnownGIFCandidateRejectReason(item) {
			t.Fatalf("expected known reject reason: %s", item)
		}
	}
}

func TestInferGIFCandidateRejectReasonFineReasons(t *testing.T) {
	settings := DefaultQualitySettings()

	t.Run("duplicate first", func(t *testing.T) {
		reason := inferGIFCandidateRejectReason(
			highlightCandidate{StartSec: 2.0, EndSec: 4.0, Score: 0.4},
			[]highlightCandidate{{StartSec: 2.1, EndSec: 3.9, Score: 0.9}},
			0.3,
			0,
			1.0,
			0.5,
			map[string]interface{}{},
			settings,
		)
		if reason != GIFCandidateRejectReasonDuplicate {
			t.Fatalf("expected duplicate reason, got %s", reason)
		}
	})

	t.Run("low confidence before fine reasons", func(t *testing.T) {
		reason := inferGIFCandidateRejectReason(
			highlightCandidate{StartSec: 1.0, EndSec: 3.0, Score: 0.2},
			nil,
			0.45,
			0.8,
			0.95,
			0.7,
			map[string]interface{}{
				"blur_mean":      5.0,
				"blur_threshold": 12.0,
			},
			settings,
		)
		if reason != GIFCandidateRejectReasonLowConfidence {
			t.Fatalf("expected low_confidence reason, got %s", reason)
		}
	})

	t.Run("blur low", func(t *testing.T) {
		reason := inferGIFCandidateRejectReason(
			highlightCandidate{StartSec: 1.0, EndSec: 3.0, Score: 0.5},
			nil,
			0.45,
			0,
			0.9,
			0.4,
			map[string]interface{}{
				"blur_mean":      6.0,
				"blur_threshold": 10.0,
			},
			settings,
		)
		if reason != GIFCandidateRejectReasonBlurLow {
			t.Fatalf("expected blur_low reason, got %s", reason)
		}
	})

	t.Run("size budget exceeded", func(t *testing.T) {
		reason := inferGIFCandidateRejectReason(
			highlightCandidate{StartSec: 1.0, EndSec: 3.0, Score: 0.5},
			nil,
			0.45,
			0,
			0.9,
			0.4,
			map[string]interface{}{
				"blur_mean":         30.0,
				"estimated_size_kb": float64(settings.GIFTargetSizeKB) * 1.3,
			},
			settings,
		)
		if reason != GIFCandidateRejectReasonSizeBudgetExceeded {
			t.Fatalf("expected size_budget_exceeded reason, got %s", reason)
		}
	})

	t.Run("loop poor", func(t *testing.T) {
		reason := inferGIFCandidateRejectReason(
			highlightCandidate{StartSec: 1.0, EndSec: 3.0, Score: 0.5},
			nil,
			0.45,
			0,
			0.9,
			0.4,
			map[string]interface{}{
				"blur_mean":   30.0,
				"motion_mean": 0.7,
				"scene_count": 3.0,
			},
			settings,
		)
		if reason != GIFCandidateRejectReasonLoopPoor {
			t.Fatalf("expected loop_poor reason, got %s", reason)
		}
	})

	t.Run("fallback low emotion", func(t *testing.T) {
		reason := inferGIFCandidateRejectReason(
			highlightCandidate{StartSec: 1.0, EndSec: 3.0, Score: 0.5},
			nil,
			0.45,
			0,
			0.9,
			0.4,
			map[string]interface{}{
				"blur_mean":         30.0,
				"estimated_size_kb": float64(settings.GIFTargetSizeKB) * 0.6,
				"motion_mean":       0.2,
				"scene_count":       1.0,
			},
			settings,
		)
		if reason != GIFCandidateRejectReasonLowEmotion {
			t.Fatalf("expected low_emotion reason, got %s", reason)
		}
	})
}
