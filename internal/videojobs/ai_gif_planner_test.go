package videojobs

import "testing"

func TestNormalizeAIGIFPlannerProposals_RecomputeByRawScores(t *testing.T) {
	in := []gifAIPlannerProposal{
		{
			ProposalRank: 2,
			StartSec:     3.2,
			EndSec:       5.0,
			Score:        0.95,
			RawScores: map[string]float64{
				"semantic":   0.4,
				"clarity":    0.9,
				"loop":       0.2,
				"efficiency": 0.5,
			},
			ProposalReason: "候选A",
		},
		{
			ProposalRank: 1,
			StartSec:     1.0,
			EndSec:       2.8,
			Score:        0.20,
			RawScores: map[string]float64{
				"semantic":   0.9,
				"clarity":    0.5,
				"loop":       0.4,
				"efficiency": 0.5,
			},
			ProposalReason: "候选B",
		},
	}
	directive := &gifAIDirectiveProfile{
		QualityWeights: map[string]float64{
			"semantic":   0.6,
			"clarity":    0.2,
			"loop":       0.1,
			"efficiency": 0.1,
		},
	}
	out, summary := normalizeAIGIFPlannerProposals(in, 20, directive)
	if len(out) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(out))
	}
	if !boolFromAny(summary["score_recomputed"]) {
		t.Fatalf("expected score_recomputed=true, got %+v", summary)
	}
	if intFromAny(summary["raw_scores_applied_count"]) != 2 {
		t.Fatalf("expected raw_scores_applied_count=2, got %+v", summary["raw_scores_applied_count"])
	}
	// With semantic-heavy weights, candidate B should rank first after recompute.
	if out[0].StartSec != 1.0 || out[0].ProposalRank != 1 {
		t.Fatalf("expected candidate B to be rank1 after recompute, got %+v", out[0])
	}
	if out[1].ProposalRank != 2 {
		t.Fatalf("expected second proposal rank=2, got %+v", out[1])
	}
	if out[0].Score <= out[1].Score {
		t.Fatalf("expected first score > second score, got %.4f vs %.4f", out[0].Score, out[1].Score)
	}
}

func TestNormalizeAIGIFPlannerProposals_BackwardCompatibleWithoutRawScores(t *testing.T) {
	in := []gifAIPlannerProposal{
		{
			ProposalRank:   2,
			StartSec:       4.0,
			EndSec:         5.8,
			Score:          0.8,
			ProposalReason: "候选A",
		},
		{
			ProposalRank:   1,
			StartSec:       1.0,
			EndSec:         2.2,
			Score:          0.6,
			ProposalReason: "候选B",
		},
	}
	out, summary := normalizeAIGIFPlannerProposals(in, 20, nil)
	if len(out) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(out))
	}
	if boolFromAny(summary["score_recomputed"]) {
		t.Fatalf("expected score_recomputed=false, got %+v", summary)
	}
	// Keep legacy ordering by proposal_rank when raw_scores are absent.
	if out[0].ProposalRank != 1 || out[0].StartSec != 1.0 {
		t.Fatalf("expected proposal_rank=1 first, got %+v", out[0])
	}
}
