package videojobs

import (
	"testing"

	"emoji/internal/models"
)

func TestNormalizeAIGIFJudgeReviews_FilterAndCount(t *testing.T) {
	samples := []gifJudgeSample{
		{OutputID: 1},
		{OutputID: 2},
	}
	in := []gifAIJudgeReviewRow{
		{OutputID: 1, FinalRecommendation: "deliver"},
		{OutputID: 1, FinalRecommendation: "reject"}, // duplicate output, should be ignored
		{OutputID: 2, FinalRecommendation: "keep_internal"},
		{OutputID: 3, FinalRecommendation: "deliver"}, // unknown output, should be ignored
	}
	out, counts := normalizeAIGIFJudgeReviews(in, samples)
	if len(out) != 2 {
		t.Fatalf("expected 2 valid reviews, got %d", len(out))
	}
	if counts["deliver_count"] != 1 || counts["keep_internal_count"] != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

func TestSelectDeliverFallbackCandidate_PrefersReviewWeight(t *testing.T) {
	samples := []gifJudgeSample{
		{OutputID: 1, Score: 0.95, EvalOverall: 0.95, EvalClarity: 0.9, EvalLoop: 0.8},
		{OutputID: 2, Score: 0.6, EvalOverall: 0.6, EvalClarity: 0.5, EvalLoop: 0.5},
	}
	reviewByOutput := map[uint64]models.VideoJobGIFAIReview{
		1: {FinalRecommendation: "reject"},
		2: {FinalRecommendation: "keep_internal"},
	}
	picked, ok := selectDeliverFallbackCandidate(samples, reviewByOutput)
	if !ok {
		t.Fatalf("expected candidate selected")
	}
	if picked.Sample.OutputID != 2 {
		t.Fatalf("expected keep_internal candidate to be preferred, got output_id=%d", picked.Sample.OutputID)
	}
}
