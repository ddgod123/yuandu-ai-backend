package videojobs

import "testing"

func testAIGIFJudgeContractSamples() []gifJudgeSample {
	return []gifJudgeSample{
		{OutputID: 11},
		{OutputID: 22},
	}
}

func TestAIGIFJudgeOutputContract_ValidPayload(t *testing.T) {
	modelText := `{
		"reviews": [
			{
				"output_id": 11,
				"proposal_rank": 1,
				"final_recommendation": "deliver",
				"semantic_verdict": 0.91,
				"diagnostic_reason": "语义和清晰度均高",
				"suggested_action": "直接发版"
			},
			{
				"output_id": 22,
				"proposal_rank": 2,
				"final_recommendation": "keep_internal",
				"semantic_verdict": 0.63,
				"diagnostic_reason": "语义一般但可留档",
				"suggested_action": "内部候选"
			}
		]
	}`
	var parsed gifAIJudgeResponse
	if err := unmarshalModelJSONWithRepair(modelText, &parsed); err != nil {
		t.Fatalf("unmarshalModelJSONWithRepair failed: %v", err)
	}
	samples := testAIGIFJudgeContractSamples()
	reviews, _ := normalizeAIGIFJudgeReviews(parsed.Reviews, samples)
	if err := validateAIGIFJudgeReviewsContract(reviews, samples); err != nil {
		t.Fatalf("judge contract should be valid, got err=%v", err)
	}
}

func TestAIGIFJudgeOutputContract_NormalizeInvalidEnumAndRange(t *testing.T) {
	modelText := `{
		"reviews": [
			{
				"output_id": 11,
				"proposal_rank": 1,
				"final_recommendation": "deliver",
				"semantic_verdict": 1.4,
				"diagnostic_reason": "第一条"
			},
			{
				"output_id": 11,
				"proposal_rank": 2,
				"final_recommendation": "reject",
				"semantic_verdict": 0.2,
				"diagnostic_reason": "重复 output_id"
			},
			{
				"output_id": 22,
				"proposal_rank": 3,
				"final_recommendation": "ship",
				"semantic_verdict": 0.8,
				"diagnostic_reason": "非法枚举"
			},
			{
				"output_id": 33,
				"proposal_rank": 4,
				"final_recommendation": "deliver",
				"semantic_verdict": 0.9,
				"diagnostic_reason": "未知输出"
			}
		]
	}`
	var parsed gifAIJudgeResponse
	if err := unmarshalModelJSONWithRepair(modelText, &parsed); err != nil {
		t.Fatalf("unmarshalModelJSONWithRepair failed: %v", err)
	}
	samples := testAIGIFJudgeContractSamples()
	reviews, _ := normalizeAIGIFJudgeReviews(parsed.Reviews, samples)
	if len(reviews) != 1 {
		t.Fatalf("expected only one valid review after normalize, got %d", len(reviews))
	}
	if reviews[0].OutputID != 11 || reviews[0].FinalRecommendation != "deliver" {
		t.Fatalf("unexpected normalized review: %+v", reviews[0])
	}
	if reviews[0].SemanticVerdict < 0 || reviews[0].SemanticVerdict > 1 {
		t.Fatalf("semantic_verdict should be clamped to [0,1], got %+v", reviews[0])
	}
	if err := validateAIGIFJudgeReviewsContract(reviews, samples); err != nil {
		t.Fatalf("judge contract should be valid after normalization, got err=%v", err)
	}
}

func TestAIGIFJudgeOutputContract_RepairTruncatedJSON(t *testing.T) {
	modelText := `{"reviews":[{"output_id":22,"final_recommendation":"need_manual_review","semantic_verdict":0.2,"diagnostic_reason":"边界样本"}]`
	var parsed gifAIJudgeResponse
	if err := unmarshalModelJSONWithRepair(modelText, &parsed); err != nil {
		t.Fatalf("unmarshalModelJSONWithRepair failed: %v", err)
	}
	samples := testAIGIFJudgeContractSamples()
	reviews, _ := normalizeAIGIFJudgeReviews(parsed.Reviews, samples)
	if err := validateAIGIFJudgeReviewsContract(reviews, samples); err != nil {
		t.Fatalf("judge contract should be valid after repair, got err=%v", err)
	}
}

func TestAIGIFJudgeOutputContract_EmptyRejected(t *testing.T) {
	samples := testAIGIFJudgeContractSamples()
	if err := validateAIGIFJudgeReviewsContract(nil, samples); err == nil {
		t.Fatalf("expected empty judge reviews to be rejected")
	}
}
