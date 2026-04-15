package videojobs

import (
	"fmt"
	"strings"
)

func validateAIGIFPlannerProposalsContract(proposals []gifAIPlannerProposal) error {
	if len(proposals) == 0 {
		return fmt.Errorf("planner proposals empty")
	}
	seenRank := make(map[int]struct{}, len(proposals))
	for idx, item := range proposals {
		if err := validateAIGIFPlannerProposalContract(item); err != nil {
			return fmt.Errorf("proposal[%d] invalid: %w", idx, err)
		}
		if _, exists := seenRank[item.ProposalRank]; exists {
			return fmt.Errorf("proposal[%d] duplicate proposal_rank=%d", idx, item.ProposalRank)
		}
		seenRank[item.ProposalRank] = struct{}{}
	}
	return nil
}

func validateAIGIFPlannerProposalContract(item gifAIPlannerProposal) error {
	if item.ProposalRank < 1 {
		return fmt.Errorf("proposal_rank must be >=1")
	}
	if item.StartSec < 0 {
		return fmt.Errorf("start_sec must be >=0")
	}
	if item.EndSec <= item.StartSec {
		return fmt.Errorf("end_sec must be > start_sec")
	}
	if item.Score < 0 || item.Score > 1 {
		return fmt.Errorf("score must be in [0,1]")
	}
	if item.StandaloneConfidence < 0 || item.StandaloneConfidence > 1 {
		return fmt.Errorf("standalone_confidence must be in [0,1]")
	}
	if item.LoopFriendlinessHint < 0 || item.LoopFriendlinessHint > 1 {
		return fmt.Errorf("loop_friendliness_hint must be in [0,1]")
	}
	if strings.TrimSpace(item.ProposalReason) == "" {
		return fmt.Errorf("proposal_reason required")
	}
	if len(item.RawScores) > 0 {
		for _, key := range []string{"semantic", "clarity", "loop", "efficiency"} {
			value, ok := item.RawScores[key]
			if !ok {
				return fmt.Errorf("raw_scores.%s required", key)
			}
			if value < 0 || value > 1 {
				return fmt.Errorf("raw_scores.%s must be in [0,1]", key)
			}
		}
	}
	return nil
}

func validateAIGIFJudgeReviewsContract(reviews []gifAIJudgeReviewRow, samples []gifJudgeSample) error {
	if len(reviews) == 0 {
		return fmt.Errorf("judge reviews empty")
	}
	allowedOutput := make(map[uint64]struct{}, len(samples))
	for _, sample := range samples {
		if sample.OutputID > 0 {
			allowedOutput[sample.OutputID] = struct{}{}
		}
	}
	seen := make(map[uint64]struct{}, len(reviews))
	for idx, item := range reviews {
		if err := validateAIGIFJudgeReviewContract(item, allowedOutput); err != nil {
			return fmt.Errorf("review[%d] invalid: %w", idx, err)
		}
		if _, exists := seen[item.OutputID]; exists {
			return fmt.Errorf("review[%d] duplicate output_id=%d", idx, item.OutputID)
		}
		seen[item.OutputID] = struct{}{}
	}
	return nil
}

func validateAIGIFJudgeReviewContract(item gifAIJudgeReviewRow, allowedOutput map[uint64]struct{}) error {
	if item.OutputID == 0 {
		return fmt.Errorf("output_id required")
	}
	if len(allowedOutput) > 0 {
		if _, ok := allowedOutput[item.OutputID]; !ok {
			return fmt.Errorf("output_id not in input set")
		}
	}
	if normalizeGIFAIReviewRecommendation(item.FinalRecommendation) == "" {
		return fmt.Errorf("final_recommendation invalid")
	}
	if item.SemanticVerdict < 0 || item.SemanticVerdict > 1 {
		return fmt.Errorf("semantic_verdict must be in [0,1]")
	}
	return nil
}
