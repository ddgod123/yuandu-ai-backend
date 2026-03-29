package videojobs

import "testing"

func TestNormalizeVideoJobAdvancedOptions(t *testing.T) {
	out := NormalizeVideoJobAdvancedOptions(VideoJobAdvancedOptions{
		Scene:         "XIAOHONGSHU",
		VisualFocus:   []string{"portrait", "action", "portrait", "unknown"},
		EnableMatting: true,
	})
	if out.Scene != AdvancedScenarioXiaohongshu {
		t.Fatalf("expected scene %s, got %s", AdvancedScenarioXiaohongshu, out.Scene)
	}
	if len(out.VisualFocus) != 2 {
		t.Fatalf("expected 2 valid visual focus options, got %+v", out.VisualFocus)
	}
	if !out.EnableMatting {
		t.Fatalf("expected enable_matting=true")
	}
}

func TestNormalizeVideoJobAdvancedOptions_CustomScene(t *testing.T) {
	out := NormalizeVideoJobAdvancedOptions(VideoJobAdvancedOptions{
		Scene: "Ecom Hero-Cover",
	})
	if out.Scene != "ecom_hero_cover" {
		t.Fatalf("expected custom scene key ecom_hero_cover, got %q", out.Scene)
	}
}

func TestNormalizeVideoJobAdvancedOptions_VisualFocusMaxTwo(t *testing.T) {
	out := NormalizeVideoJobAdvancedOptions(VideoJobAdvancedOptions{
		Scene:       "default",
		VisualFocus: []string{"portrait", "action", "vibe", "text"},
	})
	if len(out.VisualFocus) != 2 {
		t.Fatalf("expected visual_focus max 2, got %+v", out.VisualFocus)
	}
	if out.VisualFocus[0] != "portrait" || out.VisualFocus[1] != "action" {
		t.Fatalf("expected preserve first two valid values, got %+v", out.VisualFocus)
	}
}

func TestResolveVideoJobAI1StrategyProfile_Xiaohongshu(t *testing.T) {
	profile := ResolveVideoJobAI1StrategyProfile("png", VideoJobAdvancedOptions{
		Scene:         "xiaohongshu",
		VisualFocus:   []string{"portrait"},
		EnableMatting: true,
	})
	if profile.Scene != AdvancedScenarioXiaohongshu {
		t.Fatalf("unexpected scene: %s", profile.Scene)
	}
	if profile.BusinessGoal != "social_spread" {
		t.Fatalf("expected social_spread business goal, got %s", profile.BusinessGoal)
	}
	if len(profile.QualityWeights) == 0 {
		t.Fatalf("expected non-empty quality weights")
	}
	if profile.TechnicalReject == nil || stringFromAny(profile.TechnicalReject["max_blur_tolerance"]) != "low" {
		t.Fatalf("expected technical_reject.max_blur_tolerance=low, got %+v", profile.TechnicalReject)
	}
	if len(profile.RiskFlags) == 0 {
		t.Fatalf("expected non-empty risk flags")
	}
	weights := normalizeDirectiveQualityWeights(profile.QualityWeights)
	if roundTo(weights["clarity"], 2) < 0.5 {
		t.Fatalf("expected clarity weight >= 0.50 for xiaohongshu, got %.3f", weights["clarity"])
	}
	if roundTo(weights["semantic"], 2) > roundTo(weights["clarity"], 2) {
		t.Fatalf("expected clarity >= semantic for xiaohongshu, got semantic=%.3f clarity=%.3f", weights["semantic"], weights["clarity"])
	}
	if !containsString(profile.MustCaptureBias, "定格姿态") {
		t.Fatalf("expected must_capture_bias contains 定格姿态, got %+v", profile.MustCaptureBias)
	}
	if !containsString(profile.RiskFlags, "social_cover_priority") {
		t.Fatalf("expected risk_flags includes social_cover_priority, got %+v", profile.RiskFlags)
	}
}

func TestResolveVideoJobAI1StrategyProfile_DefaultPNG(t *testing.T) {
	profile := ResolveVideoJobAI1StrategyProfile("png", VideoJobAdvancedOptions{
		Scene:       "default",
		VisualFocus: []string{"portrait"},
	})
	if profile.Scene != AdvancedScenarioDefault {
		t.Fatalf("expected scene %s, got %s", AdvancedScenarioDefault, profile.Scene)
	}
	if profile.CandidateCountMin != 4 || profile.CandidateCountMax != 8 {
		t.Fatalf("expected candidate_count 4~8, got %d~%d", profile.CandidateCountMin, profile.CandidateCountMax)
	}
	weights := normalizeDirectiveQualityWeights(profile.QualityWeights)
	if roundTo(weights["clarity"], 2) < 0.40 {
		t.Fatalf("expected clarity weight >= 0.40 in default profile, got %.3f", weights["clarity"])
	}
	if roundTo(weights["semantic"], 2) < 0.35 {
		t.Fatalf("expected semantic weight >= 0.35 in default profile, got %.3f", weights["semantic"])
	}
	if !containsString(profile.MustCaptureBias, "构图稳定") {
		t.Fatalf("expected must_capture_bias contains 构图稳定, got %+v", profile.MustCaptureBias)
	}
	technical := mapFromAny(profile.TechnicalReject)
	if stringFromAny(technical["max_blur_tolerance"]) != "low" {
		t.Fatalf("expected max_blur_tolerance=low for png, got %+v", technical)
	}
}

func TestParseAI1SceneStrategyProfilesFromTemplateSchema(t *testing.T) {
	schema := map[string]interface{}{
		"scene_strategies_v1": map[string]interface{}{
			"version": "png_scene_strategy_v2",
			"default": map[string]interface{}{
				"scene_label":       "默认场景",
				"business_goal":     "extract_default",
				"must_capture_bias": []interface{}{"主体清晰", "主体清晰"},
				"quality_weights": map[string]interface{}{
					"semantic":   0.2,
					"clarity":    0.6,
					"loop":       0.0,
					"efficiency": 0.2,
				},
			},
			"xiaohongshu": map[string]interface{}{
				"enabled":       true,
				"business_goal": "social_spread",
				"audience":      "小红书用户",
			},
		},
	}
	profiles, version := parseAI1SceneStrategyProfilesFromTemplateSchema(schema)
	if version != "png_scene_strategy_v2" {
		t.Fatalf("expected version png_scene_strategy_v2, got %q", version)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 scene profiles, got %d", len(profiles))
	}
	defaultProfile, ok := profiles[AdvancedScenarioDefault]
	if !ok {
		t.Fatalf("expected default scene profile")
	}
	if defaultProfile.BusinessGoal != "extract_default" {
		t.Fatalf("unexpected default business_goal: %q", defaultProfile.BusinessGoal)
	}
	if len(defaultProfile.MustCaptureBias) != 1 {
		t.Fatalf("expected dedup must_capture_bias, got %+v", defaultProfile.MustCaptureBias)
	}
	if roundTo(defaultProfile.QualityWeights["clarity"], 2) != 0.6 {
		t.Fatalf("expected clarity weight 0.6, got %.3f", defaultProfile.QualityWeights["clarity"])
	}
}

func TestParseAI1SceneStrategyProfilesFromTemplateSchema_CustomScene(t *testing.T) {
	schema := map[string]interface{}{
		"scene_strategies_v1": map[string]interface{}{
			"version": "png_scene_strategy_v3",
			"scenes": map[string]interface{}{
				"ecom_cover": map[string]interface{}{
					"scene_label":       "电商封面",
					"operator_identity": "电商运营",
					"business_goal":     "ecom_cover",
					"candidate_count_bias": map[string]interface{}{
						"min": 5,
						"max": 12,
					},
				},
			},
		},
	}
	profiles, version := parseAI1SceneStrategyProfilesFromTemplateSchema(schema)
	if version != "png_scene_strategy_v3" {
		t.Fatalf("unexpected version: %q", version)
	}
	custom, ok := profiles["ecom_cover"]
	if !ok {
		t.Fatalf("expected custom scene ecom_cover in parsed profiles, got %+v", profiles)
	}
	if custom.OperatorIdentity != "电商运营" {
		t.Fatalf("expected operator identity 电商运营, got %q", custom.OperatorIdentity)
	}
	if custom.CandidateCountMin != 5 || custom.CandidateCountMax != 12 {
		t.Fatalf("expected candidate_count_bias 5~12, got min=%d max=%d", custom.CandidateCountMin, custom.CandidateCountMax)
	}
}

func TestMergeStrategyProfileOverlay(t *testing.T) {
	base := ResolveVideoJobAI1StrategyProfile("png", VideoJobAdvancedOptions{
		Scene:       AdvancedScenarioDefault,
		VisualFocus: []string{"portrait"},
	})
	overlay := VideoJobAI1StrategyProfile{
		Scene:           AdvancedScenarioXiaohongshu,
		SceneLabel:      "小红书网感",
		BusinessGoal:    "social_spread",
		MustCaptureBias: []string{"高颜值特写"},
		QualityWeights: map[string]float64{
			"semantic":   0.4,
			"clarity":    0.5,
			"loop":       0.0,
			"efficiency": 0.1,
		},
		TechnicalReject: map[string]interface{}{
			"max_blur_tolerance": "low",
		},
	}
	merged := mergeStrategyProfileOverlay(base, overlay)
	if merged.Scene != AdvancedScenarioXiaohongshu {
		t.Fatalf("expected scene %s, got %s", AdvancedScenarioXiaohongshu, merged.Scene)
	}
	if merged.BusinessGoal != "social_spread" {
		t.Fatalf("expected business_goal social_spread, got %s", merged.BusinessGoal)
	}
	if roundTo(merged.QualityWeights["clarity"], 2) != 0.5 {
		t.Fatalf("expected clarity weight 0.5, got %.3f", merged.QualityWeights["clarity"])
	}
	if stringFromAny(merged.TechnicalReject["max_blur_tolerance"]) != "low" {
		t.Fatalf("expected max_blur_tolerance low, got %+v", merged.TechnicalReject)
	}
}
