package videojobs

import "strings"

type imageAI2Guidance struct {
	TargetFormat     string
	Source           string
	ScoringMode      string
	SelectionPolicy  string
	Objective        string
	MustCapture      []string
	Avoid            []string
	QualityWeights   map[string]float64
	RiskFlags        []string
	MaxBlurTolerance string
	AvoidWatermarks  bool
	AvoidExtremeDark bool
	Scene            string
	SceneLabel       string
	VisualFocus      []string
	EnableMatting    bool
	StyleDirection   string
}

func resolveImageAI2Guidance(targetFormat string, ai2Instruction map[string]interface{}, eventMeta map[string]interface{}) imageAI2Guidance {
	targetFormat = NormalizeRequestedFormat(targetFormat)
	if targetFormat == "" {
		targetFormat = NormalizeRequestedFormat(strings.TrimSpace(stringFromAny(ai2Instruction["target_format"])))
	}
	if targetFormat == "" {
		targetFormat = "png"
	}

	qualityWeights := normalizeAI2QualityWeightsAny(ai2Instruction["quality_weights"])
	if len(qualityWeights) == 0 {
		qualityWeights = normalizeAI2QualityWeightsAny(eventMeta["quality_weights"])
	}
	if len(qualityWeights) == 0 {
		qualityWeights = normalizeDirectiveQualityWeights(map[string]float64{})
	}

	riskFlags := normalizeAI2RiskFlags(stringSliceFromAny(ai2Instruction["risk_flags"]))
	if len(riskFlags) == 0 {
		riskFlags = normalizeAI2RiskFlags(stringSliceFromAny(eventMeta["risk_flags"]))
	}

	technicalReject := mapFromAny(ai2Instruction["technical_reject"])
	maxBlurTolerance := normalizeAI1MaxBlurTolerance(stringFromAny(technicalReject["max_blur_tolerance"]), targetFormat)
	avoidWatermarks := true
	if _, exists := technicalReject["avoid_watermarks"]; exists {
		avoidWatermarks = boolFromAny(technicalReject["avoid_watermarks"])
	}
	avoidExtremeDark := true
	if _, exists := technicalReject["avoid_extreme_dark"]; exists {
		avoidExtremeDark = boolFromAny(technicalReject["avoid_extreme_dark"])
	}

	source := strings.TrimSpace(stringFromAny(ai2Instruction["source"]))
	if source == "" {
		source = "ai1_executable_plan"
	}
	advancedOptions := mapFromAny(ai2Instruction["advanced_options"])
	strategyProfile := mapFromAny(ai2Instruction["strategy_profile"])
	scene := NormalizeAdvancedScenario(firstNonEmptyString(
		stringFromAny(advancedOptions["scene"]),
		stringFromAny(strategyProfile["scene"]),
	))
	sceneLabel := strings.TrimSpace(stringFromAny(strategyProfile["scene_label"]))
	if sceneLabel == "" {
		sceneLabel = resolveAdvancedSceneLabel(scene)
	}
	visualFocus := normalizeAdvancedVisualFocus(stringSliceFromAny(advancedOptions["visual_focus"]))
	enableMatting := boolFromAny(advancedOptions["enable_matting"])
	styleDirection := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(ai2Instruction["style_direction"]),
		stringFromAny(strategyProfile["style_direction"]),
		stringFromAny(eventMeta["style_direction"]),
	))
	objective := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(ai2Instruction["objective"]),
		stringFromAny(eventMeta["business_goal"]),
	))
	mustCapture := normalizeStringSlice(stringSliceFromAny(ai2Instruction["must_capture"]), 16)
	if len(mustCapture) == 0 {
		mustCapture = normalizeStringSlice(stringSliceFromAny(eventMeta["must_capture"]), 16)
	}
	avoid := normalizeStringSlice(stringSliceFromAny(ai2Instruction["avoid"]), 16)
	if len(avoid) == 0 {
		avoid = normalizeStringSlice(stringSliceFromAny(eventMeta["avoid"]), 16)
	}
	selectionPolicy := resolveAI2FrameSelectionPolicy(qualityWeights, visualFocus, enableMatting)

	return imageAI2Guidance{
		TargetFormat:     targetFormat,
		Source:           source,
		ScoringMode:      "weighted_quality_v1",
		SelectionPolicy:  selectionPolicy,
		Objective:        objective,
		MustCapture:      mustCapture,
		Avoid:            avoid,
		QualityWeights:   qualityWeights,
		RiskFlags:        riskFlags,
		MaxBlurTolerance: maxBlurTolerance,
		AvoidWatermarks:  avoidWatermarks,
		AvoidExtremeDark: avoidExtremeDark,
		Scene:            scene,
		SceneLabel:       sceneLabel,
		VisualFocus:      visualFocus,
		EnableMatting:    enableMatting,
		StyleDirection:   styleDirection,
	}
}

func (g imageAI2Guidance) toMetricsMap() map[string]interface{} {
	out := map[string]interface{}{
		"target_format":       g.TargetFormat,
		"source":              g.Source,
		"scoring_mode":        g.ScoringMode,
		"selection_policy":    g.SelectionPolicy,
		"objective":           g.Objective,
		"must_capture":        g.MustCapture,
		"avoid":               g.Avoid,
		"quality_weights":     g.QualityWeights,
		"risk_flags":          g.RiskFlags,
		"max_blur_tolerance":  g.MaxBlurTolerance,
		"avoid_watermarks":    g.AvoidWatermarks,
		"avoid_extreme_dark":  g.AvoidExtremeDark,
		"scene":               g.Scene,
		"scene_label":         g.SceneLabel,
		"visual_focus":        g.VisualFocus,
		"enable_matting":      g.EnableMatting,
		"style_direction":     g.StyleDirection,
		"selector_version":    "v2_ai2_guided_ranker",
		"worker_strategy_tag": "ai2_risk_strategy_v1",
	}
	return out
}

func applyImageAI2WorkerStrategy(base QualitySettings, options jobOptions, guidance imageAI2Guidance) (QualitySettings, jobOptions, map[string]interface{}) {
	base = NormalizeQualitySettings(base)
	applied := map[string]interface{}{
		"max_blur_tolerance": guidance.MaxBlurTolerance,
		"risk_flags":         guidance.RiskFlags,
		"scoring_mode":       guidance.ScoringMode,
		"selection_policy":   guidance.SelectionPolicy,
		"scene":              guidance.Scene,
		"scene_label":        guidance.SceneLabel,
		"visual_focus":       guidance.VisualFocus,
		"enable_matting":     guidance.EnableMatting,
		"technical_reject": map[string]interface{}{
			"avoid_watermarks":   guidance.AvoidWatermarks,
			"avoid_extreme_dark": guidance.AvoidExtremeDark,
		},
	}
	tuning := map[string]interface{}{}

	switch guidance.MaxBlurTolerance {
	case "low":
		base.StillMinBlurScore = roundTo(clampFloat(base.StillMinBlurScore+3.0, 4.0, 240.0), 3)
		base.BlurThresholdFactor = roundTo(clampFloat(base.BlurThresholdFactor+0.06, 0.05, 3.0), 3)
		tuning["blur_gate"] = "strict"
	case "high":
		base.StillMinBlurScore = roundTo(clampFloat(base.StillMinBlurScore-2.0, 2.0, 240.0), 3)
		base.BlurThresholdFactor = roundTo(clampFloat(base.BlurThresholdFactor-0.04, 0.05, 3.0), 3)
		tuning["blur_gate"] = "relaxed"
	default:
		tuning["blur_gate"] = "balanced"
	}

	if hasRiskFlag(guidance.RiskFlags, "low_light") {
		options.RiskLowLight = true
		base.MinBrightness = roundTo(maxFloat(base.MinBrightness-12, 8), 3)
		base.StillMinExposureScore = roundTo(clampFloat(base.StillMinExposureScore*0.82, 0.08, 1.0), 3)
		tuning["low_light"] = "denoise_and_exposure_relax"
	}
	if hasRiskFlag(guidance.RiskFlags, "fast_motion") {
		options.RiskFastMotion = true
		base.StillMinBlurScore = roundTo(clampFloat(base.StillMinBlurScore+2.0, 4.0, 240.0), 3)
		base.BlurThresholdFactor = roundTo(clampFloat(base.BlurThresholdFactor+0.03, 0.05, 3.0), 3)
		interval := options.FrameIntervalSec
		if interval <= 0 {
			interval = 1.0
		}
		options.FrameIntervalSec = roundTo(clampFloat(interval*0.82, 0.2, 8.0), 3)
		tuning["fast_motion"] = "sharpen_and_dense_sampling"
	}
	if hasRiskFlag(guidance.RiskFlags, "watermark_risk") {
		options.RiskWatermark = true
		tuning["watermark_risk"] = "manual_review_hint"
	}
	if guidance.Scene == AdvancedScenarioXiaohongshu {
		base.StillMinBlurScore = roundTo(clampFloat(base.StillMinBlurScore+1.2, 4.0, 240.0), 3)
		base.BlurThresholdFactor = roundTo(clampFloat(base.BlurThresholdFactor+0.02, 0.05, 3.0), 3)
		tuning["scene_xiaohongshu"] = "clarity_and_visual_impact"
	}
	if guidance.Scene == AdvancedScenarioWallpaper {
		base.StillMinBlurScore = roundTo(clampFloat(base.StillMinBlurScore+1.6, 4.0, 240.0), 3)
		base.DuplicateHammingThreshold = int(clampFloat(float64(base.DuplicateHammingThreshold)+2, 1, 64))
		base.MinBrightness = roundTo(maxFloat(base.MinBrightness, 20), 3)
		tuning["scene_wallpaper"] = "clean_and_diversity_strict"
	}
	if guidance.Scene == AdvancedScenarioNews {
		base.StillMinExposureScore = roundTo(clampFloat(base.StillMinExposureScore+0.04, 0.08, 1.0), 3)
		tuning["scene_news"] = "documentary_objective"
	}
	if hasVisualFocus(guidance.VisualFocus, "portrait") {
		base.StillMinBlurScore = roundTo(clampFloat(base.StillMinBlurScore+1.0, 4.0, 240.0), 3)
		tuning["focus_portrait"] = "face_clarity_priority"
	}
	if hasVisualFocus(guidance.VisualFocus, "action") {
		interval := options.FrameIntervalSec
		if interval <= 0 {
			interval = 1.0
		}
		options.FrameIntervalSec = roundTo(clampFloat(interval*0.88, 0.2, 8.0), 3)
		options.RiskFastMotion = true
		tuning["focus_action"] = "dense_sampling"
	}
	if hasVisualFocus(guidance.VisualFocus, "text") {
		base.StillMinBlurScore = roundTo(clampFloat(base.StillMinBlurScore+0.8, 4.0, 240.0), 3)
		base.StillMinExposureScore = roundTo(clampFloat(base.StillMinExposureScore+0.05, 0.08, 1.0), 3)
		tuning["focus_text"] = "readability_priority"
	}
	if guidance.EnableMatting {
		base.StillMinBlurScore = roundTo(clampFloat(base.StillMinBlurScore+0.8, 4.0, 240.0), 3)
		base.StillMinExposureScore = roundTo(clampFloat(base.StillMinExposureScore+0.03, 0.08, 1.0), 3)
		tuning["matting"] = "subject_edge_strict"
	}

	options.AIAvoidWatermark = guidance.AvoidWatermarks
	options.AIAvoidDark = guidance.AvoidExtremeDark
	options.AIMaxBlur = guidance.MaxBlurTolerance
	if guidance.AvoidExtremeDark && !options.RiskLowLight {
		base.MinBrightness = roundTo(maxFloat(base.MinBrightness, 18), 3)
		tuning["avoid_extreme_dark"] = "bright_floor_enabled"
	}
	if guidance.AvoidWatermarks {
		tuning["avoid_watermarks"] = "enabled"
	}
	applied["applied_tuning"] = tuning

	base = NormalizeQualitySettings(base)
	return base, options, applied
}

func resolveAI2FrameSelectionPolicy(qualityWeights map[string]float64, visualFocus []string, enableMatting bool) string {
	semantic := clampZeroOne(qualityWeights["semantic"])
	clarity := clampZeroOne(qualityWeights["clarity"])
	loop := clampZeroOne(qualityWeights["loop"])
	efficiency := clampZeroOne(qualityWeights["efficiency"])
	if semantic+clarity+loop+efficiency <= 0 {
		return "scene_diversity_first"
	}

	qualityPriority := semantic + clarity
	coveragePriority := efficiency + loop
	if hasVisualFocus(visualFocus, "portrait") || enableMatting {
		qualityPriority += 0.12
	}
	if hasVisualFocus(visualFocus, "action") || hasVisualFocus(visualFocus, "vibe") {
		coveragePriority += 0.08
	}

	if qualityPriority >= coveragePriority+0.12 {
		return "ai2_global_quality_first"
	}
	return "ai2_scene_diversity_first"
}

func normalizeAI2QualityWeightsAny(raw interface{}) map[string]float64 {
	weights := map[string]float64{}
	switch value := raw.(type) {
	case map[string]float64:
		for key, item := range value {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if normalizedKey == "" {
				continue
			}
			weights[normalizedKey] = item
		}
	case map[string]interface{}:
		for key, item := range value {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if normalizedKey == "" {
				continue
			}
			weights[normalizedKey] = floatFromAny(item)
		}
	}
	if len(weights) == 0 {
		return nil
	}
	return normalizeDirectiveQualityWeights(weights)
}

func normalizeAI2RiskFlags(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func hasRiskFlag(flags []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	for _, item := range flags {
		if strings.ToLower(strings.TrimSpace(item)) == target {
			return true
		}
	}
	return false
}

func hasVisualFocus(focus []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	for _, item := range focus {
		if strings.ToLower(strings.TrimSpace(item)) == target {
			return true
		}
	}
	return false
}
