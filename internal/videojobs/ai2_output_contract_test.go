package videojobs

import "testing"

func TestAIGIFPlannerOutputContract_ValidPayload(t *testing.T) {
	modelText := `{
		"proposals": [
			{
				"proposal_rank": 1,
				"start_sec": 1.0,
				"end_sec": 2.8,
				"score": 0.82,
				"raw_scores": {"semantic": 0.9, "clarity": 0.8, "loop": 0.7, "efficiency": 0.6},
				"proposal_reason": "情绪峰值",
				"standalone_confidence": 0.88,
				"loop_friendliness_hint": 0.7
			},
			{
				"proposal_rank": 2,
				"start_sec": 3.2,
				"end_sec": 5.0,
				"score": 0.75,
				"proposal_reason": "动作完成点",
				"standalone_confidence": 0.73,
				"loop_friendliness_hint": 0.65
			}
		]
	}`
	var parsed gifAIPlannerResponse
	if err := unmarshalModelJSONWithRepair(modelText, &parsed); err != nil {
		t.Fatalf("unmarshalModelJSONWithRepair failed: %v", err)
	}
	proposals, _ := normalizeAIGIFPlannerProposals(parsed.Proposals, 12, nil)
	if err := validateAIGIFPlannerProposalsContract(proposals); err != nil {
		t.Fatalf("planner contract should be valid, got err=%v", err)
	}
}

func TestAIGIFPlannerOutputContract_RepairAndFilterInvalidPayload(t *testing.T) {
	modelText := `{
		"proposals": [
			{
				"proposal_rank": 0,
				"start_sec": -2,
				"end_sec": 0.2,
				"score": 1.6,
				"proposal_reason": ""
			},
			{
				"proposal_rank": 2,
				"start_sec": 1.0,
				"end_sec": 2.2,
				"score": 1.6,
				"raw_scores": {"semantic": 2},
				"proposal_reason": "",
				"standalone_confidence": 1.9,
				"loop_friendliness_hint": -0.4
			},
			{
				"proposal_rank": 2,
				"start_sec": 2.5,
				"end_sec": 3.7,
				"score": 0.6,
				"proposal_reason": "备用窗口"
			}
		]
	}`
	var parsed gifAIPlannerResponse
	if err := unmarshalModelJSONWithRepair(modelText, &parsed); err != nil {
		t.Fatalf("unmarshalModelJSONWithRepair failed: %v", err)
	}
	proposals, _ := normalizeAIGIFPlannerProposals(parsed.Proposals, 6, nil)
	if len(proposals) == 0 {
		t.Fatalf("expected at least one valid proposal after normalization")
	}
	if err := validateAIGIFPlannerProposalsContract(proposals); err != nil {
		t.Fatalf("planner contract should be valid after normalization, got err=%v", err)
	}
	for _, item := range proposals {
		if item.Score < 0 || item.Score > 1 {
			t.Fatalf("score out of range after normalize: %+v", item)
		}
		if item.StandaloneConfidence < 0 || item.StandaloneConfidence > 1 {
			t.Fatalf("standalone_confidence out of range after normalize: %+v", item)
		}
		if item.LoopFriendlinessHint < 0 || item.LoopFriendlinessHint > 1 {
			t.Fatalf("loop_friendliness_hint out of range after normalize: %+v", item)
		}
		if item.ProposalReason == "" {
			t.Fatalf("proposal_reason should be filled by normalize")
		}
	}
}

func TestAIGIFPlannerOutputContract_RepairTruncatedJSON(t *testing.T) {
	modelText := `{"proposals":[{"proposal_rank":1,"start_sec":0.8,"end_sec":1.9,"score":0.6,"proposal_reason":"笑点"}]`
	var parsed gifAIPlannerResponse
	if err := unmarshalModelJSONWithRepair(modelText, &parsed); err != nil {
		t.Fatalf("unmarshalModelJSONWithRepair failed: %v", err)
	}
	proposals, _ := normalizeAIGIFPlannerProposals(parsed.Proposals, 5, nil)
	if err := validateAIGIFPlannerProposalsContract(proposals); err != nil {
		t.Fatalf("planner contract should be valid after repair, got err=%v", err)
	}
}

func TestAIGIFPlannerOutputContract_EmptyRejected(t *testing.T) {
	if err := validateAIGIFPlannerProposalsContract(nil); err == nil {
		t.Fatalf("expected empty planner proposals to be rejected")
	}
}
