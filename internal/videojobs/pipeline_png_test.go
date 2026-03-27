package videojobs

import "testing"

func TestBuildImageAI3ReviewSummary_Deliver(t *testing.T) {
	report := frameQualityReport{
		TotalFrames:     18,
		KeptFrames:      12,
		SelectorVersion: "v1_scene_ranker",
		FallbackApplied: false,
	}
	summary := buildImageAI3ReviewSummary("png", report, 12, 24, 8.2)
	if got := stringFromAny(summary["recommendation"]); got != "deliver" {
		t.Fatalf("expected recommendation deliver, got %q", got)
	}
	if got := intFromAny(summary["deliver_count"]); got != 12 {
		t.Fatalf("expected deliver_count 12, got %d", got)
	}
	if got := intFromAny(summary["reject_count"]); got != 6 {
		t.Fatalf("expected reject_count 6, got %d", got)
	}
}

func TestBuildImageAI3ReviewSummary_DeliverWithFallback(t *testing.T) {
	report := frameQualityReport{
		TotalFrames:     10,
		KeptFrames:      5,
		SelectorVersion: "v1_scene_ranker",
		FallbackApplied: true,
	}
	summary := buildImageAI3ReviewSummary("png", report, 5, 20, 6.0)
	if got := stringFromAny(summary["recommendation"]); got != "deliver_with_fallback" {
		t.Fatalf("expected recommendation deliver_with_fallback, got %q", got)
	}
	summaryMap := mapFromAny(summary["summary"])
	if note := stringFromAny(summaryMap["note"]); note == "" {
		t.Fatalf("expected non-empty summary note when fallback applied")
	}
}

func TestBuildImageAI3ReviewSummary_NeedManualReview(t *testing.T) {
	report := frameQualityReport{
		TotalFrames:     9,
		KeptFrames:      0,
		SelectorVersion: "v1_scene_ranker",
	}
	summary := buildImageAI3ReviewSummary("png", report, 0, 24, 12.3)
	if got := stringFromAny(summary["recommendation"]); got != "need_manual_review" {
		t.Fatalf("expected recommendation need_manual_review, got %q", got)
	}
	if got := intFromAny(summary["manual_review_count"]); got != 0 {
		t.Fatalf("expected manual_review_count 0, got %d", got)
	}
}
