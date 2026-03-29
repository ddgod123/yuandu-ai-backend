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

func TestClampPNGMainlineAdvancedOptions_FallbackToDefault(t *testing.T) {
	in := VideoJobAdvancedOptions{
		Scene:       AdvancedScenarioWallpaper,
		VisualFocus: []string{"portrait"},
	}
	out, guard := clampPNGMainlineAdvancedOptions("png", in)
	if out.Scene != AdvancedScenarioDefault {
		t.Fatalf("expected scene to fallback default, got %q", out.Scene)
	}
	if len(guard) == 0 {
		t.Fatalf("expected guard report when scene out of mainline allowlist")
	}
	if got := stringFromAny(guard["requested_scene"]); got != AdvancedScenarioWallpaper {
		t.Fatalf("expected requested_scene=%s, got %q", AdvancedScenarioWallpaper, got)
	}
	if got := stringFromAny(guard["applied_scene"]); got != AdvancedScenarioDefault {
		t.Fatalf("expected applied_scene=%s, got %q", AdvancedScenarioDefault, got)
	}
}

func TestClampPNGMainlineAdvancedOptions_AllowXiaohongshu(t *testing.T) {
	in := VideoJobAdvancedOptions{
		Scene:       AdvancedScenarioXiaohongshu,
		VisualFocus: []string{"portrait"},
	}
	out, guard := clampPNGMainlineAdvancedOptions("png", in)
	if out.Scene != AdvancedScenarioXiaohongshu {
		t.Fatalf("expected scene xiaohongshu keep, got %q", out.Scene)
	}
	if len(guard) > 0 {
		t.Fatalf("expected empty guard for allowed scene, got %+v", guard)
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

func TestBuildImageAI2Instruction_IncludesGuidance(t *testing.T) {
	executablePlan := map[string]interface{}{
		"target_format": "png",
		"mode":          "focus_window",
		"must_capture":  []interface{}{"人物笑脸"},
		"avoid":         []interface{}{"模糊画面"},
		"focus_window": map[string]interface{}{
			"start_sec": 1.2,
			"end_sec":   3.8,
		},
	}
	eventMeta := map[string]interface{}{
		"business_goal":     "cover_story",
		"operator_identity": "时尚视觉总监",
		"candidate_count_bias": map[string]interface{}{
			"min": 4,
			"max": 8,
		},
		"quality_weights": map[string]float64{
			"semantic":   0.5,
			"clarity":    0.2,
			"loop":       0.1,
			"efficiency": 0.2,
		},
		"risk_flags": []interface{}{"low_light", "fast_motion"},
		"advanced_options_v1": map[string]interface{}{
			"scene":          "xiaohongshu",
			"visual_focus":   []interface{}{"portrait"},
			"enable_matting": true,
		},
		"strategy_profile_v1": map[string]interface{}{
			"scene":          "xiaohongshu",
			"scene_label":    "小红书网感",
			"directive_hint": "社交封面优先",
		},
		"postprocess": map[string]interface{}{
			"enable_matting": true,
			"type":           "portrait_cutout",
		},
	}

	out := buildImageAI2Instruction("png_v1", executablePlan, eventMeta)
	if got := stringFromAny(out["schema_version"]); got != AI2DirectiveSchemaV2 {
		t.Fatalf("expected ai2 schema %s, got %q", AI2DirectiveSchemaV2, got)
	}
	if got := stringFromAny(out["target_format"]); got != "png" {
		t.Fatalf("expected target_format png, got %q", got)
	}
	technical := mapFromAny(out["technical_reject"])
	if got := stringFromAny(technical["max_blur_tolerance"]); got != "low" {
		t.Fatalf("expected max_blur_tolerance low, got %q", got)
	}
	weights := normalizeAI2QualityWeightsAny(out["quality_weights"])
	if len(weights) == 0 {
		t.Fatalf("expected quality_weights in instruction")
	}
	if roundTo(weights["semantic"], 2) != 0.5 {
		t.Fatalf("expected semantic weight 0.5, got %.3f", weights["semantic"])
	}
	flags := normalizeAI2RiskFlags(stringSliceFromAny(out["risk_flags"]))
	if len(flags) != 2 {
		t.Fatalf("expected two risk flags, got %+v", flags)
	}
	if len(mapFromAny(out["advanced_options"])) == 0 {
		t.Fatalf("expected advanced_options in ai2 instruction")
	}
	if len(mapFromAny(out["strategy_profile"])) == 0 {
		t.Fatalf("expected strategy_profile in ai2 instruction")
	}
	if len(mapFromAny(out["postprocess"])) == 0 {
		t.Fatalf("expected postprocess in ai2 instruction")
	}
	if len(stringSliceFromAny(out["must_capture"])) == 0 {
		t.Fatalf("expected must_capture in ai2 instruction")
	}
	if len(stringSliceFromAny(out["avoid"])) == 0 {
		t.Fatalf("expected avoid in ai2 instruction")
	}
	if got := stringFromAny(out["operator_identity"]); got != "时尚视觉总监" {
		t.Fatalf("expected operator_identity 时尚视觉总监, got %q", got)
	}
	candidateBias := mapFromAny(out["candidate_count_bias"])
	if intFromAny(candidateBias["min"]) != 4 || intFromAny(candidateBias["max"]) != 8 {
		t.Fatalf("expected candidate_count_bias 4~8, got %+v", candidateBias)
	}
}

func TestBuildImageAI2Instruction_MergesTechnicalRejectFromEventMeta(t *testing.T) {
	executablePlan := map[string]interface{}{
		"target_format": "png",
		"mode":          "focus_window",
	}
	eventMeta := map[string]interface{}{
		"technical_reject": map[string]interface{}{
			"max_blur_tolerance": "high",
			"avoid_watermarks":   false,
			"avoid_extreme_dark": false,
		},
	}

	out := buildImageAI2Instruction("png_v1", executablePlan, eventMeta)
	technical := mapFromAny(out["technical_reject"])
	if got := stringFromAny(technical["max_blur_tolerance"]); got != "high" {
		t.Fatalf("expected max_blur_tolerance=high, got %q", got)
	}
	if got := boolFromAny(technical["avoid_watermarks"]); got {
		t.Fatalf("expected avoid_watermarks=false")
	}
	if got := boolFromAny(technical["avoid_extreme_dark"]); got {
		t.Fatalf("expected avoid_extreme_dark=false")
	}
}

func TestPNGMainline_AI1EditedDirectiveFlowsToWorkerStrategy(t *testing.T) {
	executablePlan := map[string]interface{}{
		"target_format":      "png",
		"mode":               "focus_window",
		"target_count":       8,
		"frame_interval_sec": 1.2,
		"must_capture":       []interface{}{"人物特写", "动作峰值"},
		"avoid":              []interface{}{"极暗画面"},
		"focus_window": map[string]interface{}{
			"start_sec": 1.4,
			"end_sec":   4.2,
		},
	}
	eventMeta := map[string]interface{}{
		"business_goal": "social_spread",
		"quality_weights": map[string]interface{}{
			"semantic":   0.2,
			"clarity":    0.55,
			"loop":       0.05,
			"efficiency": 0.2,
		},
		"risk_flags": []interface{}{"low_light", "fast_motion"},
		"technical_reject": map[string]interface{}{
			"max_blur_tolerance": "low",
			"avoid_watermarks":   true,
			"avoid_extreme_dark": true,
		},
		"advanced_options_v1": map[string]interface{}{
			"scene":          "xiaohongshu",
			"visual_focus":   []interface{}{"portrait", "action"},
			"enable_matting": true,
		},
		"strategy_profile_v1": map[string]interface{}{
			"scene":       "xiaohongshu",
			"scene_label": "小红书网感",
		},
	}

	ai2Instruction := buildImageAI2Instruction("png_v1", executablePlan, eventMeta)
	weights := normalizeAI2QualityWeightsAny(ai2Instruction["quality_weights"])
	if roundTo(weights["clarity"], 2) != 0.55 {
		t.Fatalf("expected clarity weight 0.55, got %.3f", weights["clarity"])
	}
	technical := mapFromAny(ai2Instruction["technical_reject"])
	if stringFromAny(technical["max_blur_tolerance"]) != "low" {
		t.Fatalf("expected max_blur_tolerance=low, got %+v", technical["max_blur_tolerance"])
	}
	if !boolFromAny(technical["avoid_watermarks"]) || !boolFromAny(technical["avoid_extreme_dark"]) {
		t.Fatalf("expected technical reject boolean flags true, got %+v", technical)
	}

	guidance := resolveImageAI2Guidance("png", ai2Instruction, eventMeta)
	if guidance.Scene != AdvancedScenarioXiaohongshu {
		t.Fatalf("expected scene=%s, got %q", AdvancedScenarioXiaohongshu, guidance.Scene)
	}
	if !guidance.EnableMatting {
		t.Fatalf("expected enable_matting=true in guidance")
	}
	if !hasVisualFocus(guidance.VisualFocus, "portrait") || !hasVisualFocus(guidance.VisualFocus, "action") {
		t.Fatalf("expected portrait/action focus in guidance, got %+v", guidance.VisualFocus)
	}

	baseSettings := DefaultQualitySettings()
	baseOptions := jobOptions{FrameIntervalSec: 1.4}
	nextSettings, nextOptions, strategy := applyImageAI2WorkerStrategy(baseSettings, baseOptions, guidance)

	if !nextOptions.RiskLowLight || !nextOptions.RiskFastMotion {
		t.Fatalf("expected low_light + fast_motion risk flags to be applied, got %+v", nextOptions)
	}
	if nextOptions.FrameIntervalSec >= baseOptions.FrameIntervalSec {
		t.Fatalf("expected denser sampling interval after guidance, base=%.3f now=%.3f", baseOptions.FrameIntervalSec, nextOptions.FrameIntervalSec)
	}
	if nextSettings.StillMinBlurScore <= baseSettings.StillMinBlurScore {
		t.Fatalf("expected stricter still blur score, base=%.3f now=%.3f", baseSettings.StillMinBlurScore, nextSettings.StillMinBlurScore)
	}
	appliedTuning := mapFromAny(strategy["applied_tuning"])
	if _, ok := appliedTuning["scene_xiaohongshu"]; !ok {
		t.Fatalf("expected scene_xiaohongshu tuning, got %+v", appliedTuning)
	}
	if _, ok := appliedTuning["focus_portrait"]; !ok {
		t.Fatalf("expected focus_portrait tuning, got %+v", appliedTuning)
	}
	if _, ok := appliedTuning["focus_action"]; !ok {
		t.Fatalf("expected focus_action tuning, got %+v", appliedTuning)
	}
	if _, ok := appliedTuning["matting"]; !ok {
		t.Fatalf("expected matting tuning, got %+v", appliedTuning)
	}
}

func TestApplyImageAI2WorkerStrategy_UpdatesSettings(t *testing.T) {
	base := DefaultQualitySettings()
	opts := jobOptions{FrameIntervalSec: 1.2}
	guidance := imageAI2Guidance{
		TargetFormat:     "png",
		Source:           "ai1_executable_plan",
		ScoringMode:      "weighted_quality_v1",
		QualityWeights:   normalizeDirectiveQualityWeights(map[string]float64{}),
		RiskFlags:        []string{"low_light", "fast_motion"},
		MaxBlurTolerance: "low",
		AvoidWatermarks:  true,
		AvoidExtremeDark: true,
	}

	nextSettings, nextOptions, strategy := applyImageAI2WorkerStrategy(base, opts, guidance)
	if !nextOptions.RiskLowLight || !nextOptions.RiskFastMotion {
		t.Fatalf("expected risk flags propagated to worker options, got %+v", nextOptions)
	}
	if nextOptions.FrameIntervalSec >= opts.FrameIntervalSec {
		t.Fatalf("expected denser sampling interval, got %.3f (base %.3f)", nextOptions.FrameIntervalSec, opts.FrameIntervalSec)
	}
	if nextSettings.StillMinBlurScore <= base.StillMinBlurScore {
		t.Fatalf("expected stricter blur gate after strategy, base=%.2f now=%.2f", base.StillMinBlurScore, nextSettings.StillMinBlurScore)
	}
	appliedTuning := mapFromAny(strategy["applied_tuning"])
	if len(appliedTuning) == 0 {
		t.Fatalf("expected non-empty applied_tuning, got %+v", strategy)
	}
}

func TestResolveImageAI2Guidance_WithSceneFocusAndMatting(t *testing.T) {
	ai2Instruction := map[string]interface{}{
		"target_format": "png",
		"source":        "ai1_executable_plan",
		"quality_weights": map[string]interface{}{
			"semantic":   0.4,
			"clarity":    0.4,
			"loop":       0.05,
			"efficiency": 0.15,
		},
		"risk_flags": []interface{}{"low_light"},
		"technical_reject": map[string]interface{}{
			"max_blur_tolerance": "low",
			"avoid_watermarks":   true,
			"avoid_extreme_dark": true,
		},
		"advanced_options": map[string]interface{}{
			"scene":          "xiaohongshu",
			"visual_focus":   []interface{}{"portrait", "action"},
			"enable_matting": true,
		},
		"strategy_profile": map[string]interface{}{
			"scene_label": "小红书网感",
		},
		"style_direction": "vivid_contrast_portrait",
		"must_capture":    []interface{}{"人物特写", "动作瞬间"},
		"avoid":           []interface{}{"模糊画面"},
	}
	guidance := resolveImageAI2Guidance("png", ai2Instruction, map[string]interface{}{})
	if guidance.Scene != AdvancedScenarioXiaohongshu {
		t.Fatalf("expected scene xiaohongshu, got %q", guidance.Scene)
	}
	if guidance.SceneLabel != "小红书网感" {
		t.Fatalf("expected scene label, got %q", guidance.SceneLabel)
	}
	if !guidance.EnableMatting {
		t.Fatalf("expected enable_matting=true")
	}
	if !hasVisualFocus(guidance.VisualFocus, "portrait") || !hasVisualFocus(guidance.VisualFocus, "action") {
		t.Fatalf("expected portrait/action focus, got %+v", guidance.VisualFocus)
	}
	if guidance.StyleDirection != "vivid_contrast_portrait" {
		t.Fatalf("unexpected style direction: %q", guidance.StyleDirection)
	}
	if guidance.SelectionPolicy != "ai2_global_quality_first" {
		t.Fatalf("expected selection policy ai2_global_quality_first, got %q", guidance.SelectionPolicy)
	}
	if len(guidance.MustCapture) == 0 || len(guidance.Avoid) == 0 {
		t.Fatalf("expected must/avoid in guidance, got must=%+v avoid=%+v", guidance.MustCapture, guidance.Avoid)
	}
}

func TestResolveAI2FrameSelectionPolicy_QualityVsCoverage(t *testing.T) {
	qualityFirst := resolveAI2FrameSelectionPolicy(map[string]float64{
		"semantic":   0.45,
		"clarity":    0.4,
		"loop":       0.05,
		"efficiency": 0.1,
	}, []string{"portrait"}, true)
	if qualityFirst != "ai2_global_quality_first" {
		t.Fatalf("expected ai2_global_quality_first, got %q", qualityFirst)
	}

	coverageFirst := resolveAI2FrameSelectionPolicy(map[string]float64{
		"semantic":   0.2,
		"clarity":    0.18,
		"loop":       0.28,
		"efficiency": 0.34,
	}, []string{"action"}, false)
	if coverageFirst != "ai2_scene_diversity_first" {
		t.Fatalf("expected ai2_scene_diversity_first, got %q", coverageFirst)
	}
}

func TestApplyImageAI2WorkerStrategy_SceneAndFocusHardEffects(t *testing.T) {
	base := DefaultQualitySettings()
	opts := jobOptions{FrameIntervalSec: 1.0}
	guidance := imageAI2Guidance{
		TargetFormat:     "png",
		Source:           "ai1_executable_plan",
		ScoringMode:      "weighted_quality_v1",
		QualityWeights:   normalizeDirectiveQualityWeights(map[string]float64{}),
		RiskFlags:        []string{"fast_motion"},
		MaxBlurTolerance: "low",
		AvoidWatermarks:  true,
		AvoidExtremeDark: true,
		Scene:            AdvancedScenarioWallpaper,
		SceneLabel:       "手机壁纸",
		VisualFocus:      []string{"action", "text"},
		EnableMatting:    true,
	}
	nextSettings, nextOptions, strategy := applyImageAI2WorkerStrategy(base, opts, guidance)
	if nextSettings.StillMinBlurScore <= base.StillMinBlurScore {
		t.Fatalf("expected stricter still blur score, base=%.2f now=%.2f", base.StillMinBlurScore, nextSettings.StillMinBlurScore)
	}
	if nextSettings.DuplicateHammingThreshold <= base.DuplicateHammingThreshold {
		t.Fatalf("expected stronger dedup threshold, base=%d now=%d", base.DuplicateHammingThreshold, nextSettings.DuplicateHammingThreshold)
	}
	if nextOptions.FrameIntervalSec >= opts.FrameIntervalSec {
		t.Fatalf("expected denser sampling from action focus, base=%.2f now=%.2f", opts.FrameIntervalSec, nextOptions.FrameIntervalSec)
	}
	if !nextOptions.RiskFastMotion {
		t.Fatalf("expected RiskFastMotion=true")
	}
	appliedTuning := mapFromAny(strategy["applied_tuning"])
	if _, ok := appliedTuning["scene_wallpaper"]; !ok {
		t.Fatalf("expected scene_wallpaper tuning, got %+v", appliedTuning)
	}
	if _, ok := appliedTuning["focus_action"]; !ok {
		t.Fatalf("expected focus_action tuning, got %+v", appliedTuning)
	}
	if _, ok := appliedTuning["matting"]; !ok {
		t.Fatalf("expected matting tuning, got %+v", appliedTuning)
	}
}

func TestShouldPauseForAI1NeedClarify(t *testing.T) {
	if !shouldPauseForAI1NeedClarify(map[string]interface{}{"interactive_action": "need_clarify"}, false, false) {
		t.Fatalf("expected need_clarify to trigger ai1 pause")
	}
	if shouldPauseForAI1NeedClarify(map[string]interface{}{"interactive_action": "proceed"}, false, false) {
		t.Fatalf("proceed should not trigger ai1 pause")
	}
	if shouldPauseForAI1NeedClarify(map[string]interface{}{"interactive_action": "need_clarify"}, true, false) {
		t.Fatalf("ai1_confirmed=true should skip ai1 pause")
	}
	if shouldPauseForAI1NeedClarify(map[string]interface{}{"interactive_action": "need_clarify"}, false, true) {
		t.Fatalf("ai1_pause_consumed=true should skip ai1 pause")
	}
}

func TestApplyImageAI1StrategyHardOverrides(t *testing.T) {
	eventMeta := map[string]interface{}{
		"style_direction":   "model_drift_style",
		"business_goal":     "entertainment",
		"audience":          "通用用户",
		"operator_identity": "",
	}
	strategy := VideoJobAI1StrategyProfile{
		Scene:            AdvancedScenarioXiaohongshu,
		SceneLabel:       "小红书网感",
		BusinessGoal:     "social_spread",
		Audience:         "小红书内容受众",
		OperatorIdentity: "时尚视觉总监",
		StyleDirection:   "social_cover_high_clarity",
	}
	report := applyImageAI1StrategyHardOverrides(eventMeta, strategy, "png")
	if len(report) == 0 {
		t.Fatalf("expected non-empty override report")
	}
	if !boolFromAny(report["has_override"]) {
		t.Fatalf("expected has_override=true, got %+v", report)
	}
	if got := stringFromAny(eventMeta["style_direction"]); got != "social_cover_high_clarity" {
		t.Fatalf("expected style_direction overridden, got %q", got)
	}
	if got := stringFromAny(eventMeta["business_goal"]); got != "social_spread" {
		t.Fatalf("expected business_goal overridden, got %q", got)
	}
	if got := stringFromAny(eventMeta["operator_identity"]); got != "时尚视觉总监" {
		t.Fatalf("expected operator_identity filled, got %q", got)
	}
	rawOverrides, _ := report["overrides"].([]map[string]interface{})
	if len(rawOverrides) == 0 {
		if list, ok := report["overrides"].([]interface{}); ok {
			rawOverrides = make([]map[string]interface{}, 0, len(list))
			for _, item := range list {
				rawOverrides = append(rawOverrides, mapFromAny(item))
			}
		}
	}
	if len(rawOverrides) < 3 {
		t.Fatalf("expected >=3 override entries, got %+v", report["overrides"])
	}
}

func TestApplyImageAI1StrategyHardOverrides_LockMainlineQualityWeights(t *testing.T) {
	eventMeta := map[string]interface{}{
		"quality_weights": map[string]float64{
			"semantic":   0.35,
			"clarity":    0.26,
			"loop":       0.19,
			"efficiency": 0.20,
		},
	}
	strategy := VideoJobAI1StrategyProfile{
		Scene:      AdvancedScenarioDefault,
		SceneLabel: "通用截图",
		QualityWeights: map[string]float64{
			"semantic":   0.38,
			"clarity":    0.40,
			"loop":       0.02,
			"efficiency": 0.20,
		},
	}

	report := applyImageAI1StrategyHardOverrides(eventMeta, strategy, "png")
	if len(report) == 0 {
		t.Fatalf("expected non-empty report")
	}
	weights := normalizeAI2QualityWeightsAny(eventMeta["quality_weights"])
	if !isSameQualityWeights(weights, strategy.QualityWeights) {
		t.Fatalf("expected mainline default quality_weights to be locked, got %+v", weights)
	}
	if !boolFromAny(report["has_override"]) {
		t.Fatalf("expected has_override=true")
	}
}
