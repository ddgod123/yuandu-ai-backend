package videojobs

import (
	"testing"
)

func TestParseAIGIFDirectiveResponse_DirectiveSchema(t *testing.T) {
	modelText := `{
		"directive": {
			"business_goal": "social_spread",
			"audience": "通用用户",
			"must_capture": ["人物表情爆发"],
			"avoid": ["转场空镜"],
			"clip_count_min": 2,
			"clip_count_max": 4,
			"duration_pref_min_sec": 1.4,
			"duration_pref_max_sec": 3.2,
			"loop_preference": 0.3,
			"style_direction": "balanced_reaction",
			"risk_flags": ["low_light"],
			"quality_weights": {"semantic": 0.35, "clarity": 0.2, "loop": 0.25, "efficiency": 0.2},
			"directive_text": "优先抓取情绪峰值"
		}
	}`
	got, shape, err := parseAIGIFDirectiveResponse(modelText)
	if err != nil {
		t.Fatalf("parseAIGIFDirectiveResponse failed: %v", err)
	}
	if shape != "directive" {
		t.Fatalf("unexpected response shape: %s", shape)
	}
	if got.BusinessGoal != "social_spread" || got.ClipCountMin != 2 || got.ClipCountMax != 4 {
		t.Fatalf("unexpected directive parsed: %+v", got)
	}
}

func TestParseAIGIFDirectiveResponse_BriefSchema(t *testing.T) {
	modelText := `{
		"business_goal":"social_spread",
		"audience":"通用用户",
		"must_capture":["笑点"],
		"avoid":["黑屏"],
		"quality_weights":{"semantic":0.35,"clarity":0.2,"loop":0.25,"efficiency":0.2},
		"planner_instruction_text":"优先抓取笑点镜头",
		"clip_count_range":[2,4],
		"duration_pref_sec":[1.2,2.8]
	}`
	got, shape, err := parseAIGIFDirectiveResponse(modelText)
	if err != nil {
		t.Fatalf("parseAIGIFDirectiveResponse failed: %v", err)
	}
	if shape != "brief_v2_flat" {
		t.Fatalf("unexpected response shape: %s", shape)
	}
	if got.ClipCountMin != 2 || got.ClipCountMax != 4 {
		t.Fatalf("unexpected clip range: %+v", got)
	}
	if got.DurationPrefMinSec <= 0 || got.DurationPrefMaxSec <= got.DurationPrefMinSec {
		t.Fatalf("unexpected duration range: %+v", got)
	}
}

func TestValidateAIGIFDirectiveContract_InvalidLoopPreference(t *testing.T) {
	in := gifAIDirectiveProfile{
		BusinessGoal:       "social_spread",
		Audience:           "通用用户",
		ClipCountMin:       2,
		ClipCountMax:       3,
		DurationPrefMinSec: 1.2,
		DurationPrefMaxSec: 2.2,
		LoopPreference:     1.4,
		QualityWeights: map[string]float64{
			"semantic": 1,
		},
	}
	if err := validateAIGIFDirectiveContract(in); err == nil {
		t.Fatalf("expected invalid contract due to loop_preference")
	}
}
