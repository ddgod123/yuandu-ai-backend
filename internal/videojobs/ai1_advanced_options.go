package videojobs

import (
	"encoding/json"
	"sort"
	"strings"
)

const (
	AdvancedScenarioDefault     = "default"
	AdvancedScenarioXiaohongshu = "xiaohongshu"
	AdvancedScenarioWallpaper   = "wallpaper"
	AdvancedScenarioNews        = "news"
	ai1SceneStrategiesSchemaKey = "scene_strategies_v1"
)

type VideoJobAdvancedOptions struct {
	Scene         string   `json:"scene"`
	VisualFocus   []string `json:"visual_focus,omitempty"`
	EnableMatting bool     `json:"enable_matting,omitempty"`
}

type VideoJobAI1StrategyProfile struct {
	Version           string                 `json:"version,omitempty"`
	Scene             string                 `json:"scene"`
	SceneLabel        string                 `json:"scene_label,omitempty"`
	BusinessGoal      string                 `json:"business_goal,omitempty"`
	Audience          string                 `json:"audience,omitempty"`
	OperatorIdentity  string                 `json:"operator_identity,omitempty"`
	StyleDirection    string                 `json:"style_direction,omitempty"`
	CandidateCountMin int                    `json:"candidate_count_min,omitempty"`
	CandidateCountMax int                    `json:"candidate_count_max,omitempty"`
	MustCaptureBias   []string               `json:"must_capture_bias,omitempty"`
	AvoidBias         []string               `json:"avoid_bias,omitempty"`
	RiskFlags         []string               `json:"risk_flags,omitempty"`
	QualityWeights    map[string]float64     `json:"quality_weights,omitempty"`
	TechnicalReject   map[string]interface{} `json:"technical_reject,omitempty"`
	DirectiveHint     string                 `json:"directive_hint,omitempty"`
}

func normalizeAdvancedSceneKey(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	lastUnderscore := false
	appendUnderscore := func() {
		if lastUnderscore {
			return
		}
		b.WriteByte('_')
		lastUnderscore = true
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastUnderscore = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		case r == '_' || r == '-' || r == ' ' || r == '.' || r == '/' || r == '\\':
			appendUnderscore()
		default:
			appendUnderscore()
		}
	}
	scene := strings.Trim(b.String(), "_")
	if scene == "" {
		return ""
	}
	if len(scene) > 48 {
		scene = strings.Trim(scene[:48], "_")
	}
	return scene
}

func NormalizeAdvancedScenario(raw string) string {
	scene := normalizeAdvancedSceneKey(raw)
	switch scene {
	case AdvancedScenarioXiaohongshu:
		return AdvancedScenarioXiaohongshu
	case AdvancedScenarioWallpaper:
		return AdvancedScenarioWallpaper
	case AdvancedScenarioNews:
		return AdvancedScenarioNews
	case AdvancedScenarioDefault:
		return AdvancedScenarioDefault
	default:
		if scene == "" {
			return AdvancedScenarioDefault
		}
		return scene
	}
}

func normalizeAdvancedVisualFocus(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, minInt(len(values), 2))
	for _, item := range values {
		value := strings.ToLower(strings.TrimSpace(item))
		switch value {
		case "portrait", "action", "vibe", "text":
		default:
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= 2 {
			break
		}
	}
	return out
}

func NormalizeVideoJobAdvancedOptions(in VideoJobAdvancedOptions) VideoJobAdvancedOptions {
	out := in
	out.Scene = NormalizeAdvancedScenario(firstNonEmptyString(in.Scene))
	out.VisualFocus = normalizeAdvancedVisualFocus(in.VisualFocus)
	return out
}

func ParseVideoJobAdvancedOptions(raw interface{}) VideoJobAdvancedOptions {
	switch value := raw.(type) {
	case VideoJobAdvancedOptions:
		return NormalizeVideoJobAdvancedOptions(value)
	case map[string]interface{}:
		out := VideoJobAdvancedOptions{
			Scene: firstNonEmptyString(
				stringFromAny(value["scene"]),
				stringFromAny(value["scenario"]),
			),
			VisualFocus:   stringSliceFromAny(value["visual_focus"]),
			EnableMatting: boolFromAny(value["enable_matting"]),
		}
		return NormalizeVideoJobAdvancedOptions(out)
	default:
		return NormalizeVideoJobAdvancedOptions(VideoJobAdvancedOptions{})
	}
}

func AdvancedOptionsToMap(options VideoJobAdvancedOptions) map[string]interface{} {
	options = NormalizeVideoJobAdvancedOptions(options)
	out := map[string]interface{}{
		"scene":          options.Scene,
		"visual_focus":   options.VisualFocus,
		"enable_matting": options.EnableMatting,
	}
	return out
}

func ResolveVideoJobAI1StrategyProfile(targetFormat string, options VideoJobAdvancedOptions) VideoJobAI1StrategyProfile {
	options = NormalizeVideoJobAdvancedOptions(options)
	targetFormat = NormalizeRequestedFormat(strings.TrimSpace(targetFormat))
	if targetFormat == "" {
		targetFormat = "png"
	}

	maxBlurTolerance := "medium"
	switch targetFormat {
	case "png", "jpg", "webp":
		maxBlurTolerance = "low"
	}

	profile := VideoJobAI1StrategyProfile{
		Version:           "ai1_strategy_profile_v1",
		Scene:             options.Scene,
		BusinessGoal:      "extract_high_quality_frames",
		Audience:          "通用用户",
		OperatorIdentity:  "视觉总监",
		StyleDirection:    "balanced_clarity",
		CandidateCountMin: 4,
		CandidateCountMax: 8,
		QualityWeights: normalizeDirectiveQualityWeights(map[string]float64{
			"semantic":   0.38,
			"clarity":    0.40,
			"loop":       0.02,
			"efficiency": 0.20,
		}),
		MustCaptureBias: []string{"主体清晰", "关键内容完整", "构图稳定"},
		AvoidBias:       []string{"严重模糊", "全黑或全白曝光异常", "主体残缺"},
		RiskFlags:       []string{},
		TechnicalReject: map[string]interface{}{
			"max_blur_tolerance": maxBlurTolerance,
			"avoid_watermarks":   true,
			"avoid_extreme_dark": true,
		},
		DirectiveHint: "优先输出清晰、稳定、主体完整的静态帧。",
		SceneLabel:    "通用截图",
	}

	switch options.Scene {
	case AdvancedScenarioXiaohongshu:
		profile.SceneLabel = "小红书网感"
		profile.BusinessGoal = "social_spread"
		profile.Audience = "小红书内容受众"
		profile.OperatorIdentity = "时尚视觉总监"
		profile.StyleDirection = "social_cover_high_clarity"
		profile.CandidateCountMin = 4
		profile.CandidateCountMax = 8
		profile.MustCaptureBias = []string{"高颜值特写", "情绪峰值", "定格姿态", "色彩明快"}
		profile.AvoidBias = []string{"背影遮挡", "低饱和灰雾", "杂乱背景", "运动拖影"}
		profile.RiskFlags = []string{"social_cover_priority"}
		profile.QualityWeights = normalizeDirectiveQualityWeights(map[string]float64{
			"semantic":   0.36,
			"clarity":    0.50,
			"loop":       0.02,
			"efficiency": 0.12,
		})
		profile.DirectiveHint = "按社交封面标准筛选，优先高吸引力特写与高画质。"
	case AdvancedScenarioWallpaper:
		profile.SceneLabel = "手机壁纸"
		profile.BusinessGoal = "mobile_wallpaper"
		profile.Audience = "手机壁纸使用者"
		profile.OperatorIdentity = "壁纸构图编辑"
		profile.StyleDirection = "clean_centered_wallpaper"
		profile.CandidateCountMin = 3
		profile.CandidateCountMax = 6
		profile.MustCaptureBias = []string{"主体居中", "画面干净", "竖屏友好"}
		profile.AvoidBias = []string{"杂乱背景", "大面积字幕", "边缘主体残缺"}
		profile.RiskFlags = []string{"wallpaper_composition_strict"}
		profile.QualityWeights = normalizeDirectiveQualityWeights(map[string]float64{
			"semantic":   0.30,
			"clarity":    0.50,
			"loop":       0.02,
			"efficiency": 0.18,
		})
		profile.DirectiveHint = "按壁纸可用性标准筛选，优先构图与纯净度。"
	case AdvancedScenarioNews:
		profile.SceneLabel = "新闻配图"
		profile.BusinessGoal = "news_illustration"
		profile.Audience = "新闻阅读受众"
		profile.OperatorIdentity = "纪实图片编辑"
		profile.StyleDirection = "documentary_objective"
		profile.CandidateCountMin = 3
		profile.CandidateCountMax = 8
		profile.MustCaptureBias = []string{"事件关键瞬间", "信息量充足", "主体明确"}
		profile.AvoidBias = []string{"夸张滤镜", "过度美化", "场景失真"}
		profile.RiskFlags = []string{"documentary_objective"}
		profile.QualityWeights = normalizeDirectiveQualityWeights(map[string]float64{
			"semantic":   0.45,
			"clarity":    0.35,
			"loop":       0.02,
			"efficiency": 0.18,
		})
		profile.DirectiveHint = "按纪实配图标准筛选，保证客观表达。"
	default:
		if options.Scene != "" && options.Scene != AdvancedScenarioDefault {
			profile.SceneLabel = options.Scene
		}
	}

	for _, focus := range options.VisualFocus {
		switch focus {
		case "portrait":
			profile.MustCaptureBias = append(profile.MustCaptureBias, "人物与面部清晰", "主角特写")
			profile.AvoidBias = append(profile.AvoidBias, "背影", "侧脸遮挡")
		case "action":
			profile.MustCaptureBias = append(profile.MustCaptureBias, "动作峰值瞬间", "关键动作完成帧")
			profile.AvoidBias = append(profile.AvoidBias, "动作拖影")
		case "vibe":
			profile.MustCaptureBias = append(profile.MustCaptureBias, "场景氛围感", "环境光影")
		case "text":
			profile.MustCaptureBias = append(profile.MustCaptureBias, "文字/字幕完整可读")
			profile.AvoidBias = append(profile.AvoidBias, "文字裁切")
		}
	}

	if options.EnableMatting {
		profile.MustCaptureBias = append(profile.MustCaptureBias, "主体边缘清晰", "人物主体完整")
		profile.AvoidBias = append(profile.AvoidBias, "主体与背景同色混叠")
		profile.RiskFlags = append(profile.RiskFlags, "matting_requested")
		profile.DirectiveHint += " 已开启人物主体抠图准备。"
	}

	profile.MustCaptureBias = normalizeStringSlice(profile.MustCaptureBias, 16)
	profile.AvoidBias = normalizeStringSlice(profile.AvoidBias, 16)
	profile.RiskFlags = normalizeStringSlice(profile.RiskFlags, 16)
	profile.CandidateCountMin, profile.CandidateCountMax = normalizeCandidateCountBias(
		profile.CandidateCountMin,
		profile.CandidateCountMax,
		targetFormat,
	)
	return profile
}

func StrategyProfileToMap(profile VideoJobAI1StrategyProfile) map[string]interface{} {
	profile.Scene = NormalizeAdvancedScenario(profile.Scene)
	out := map[string]interface{}{
		"version":           strings.TrimSpace(profile.Version),
		"scene":             profile.Scene,
		"scene_label":       strings.TrimSpace(profile.SceneLabel),
		"business_goal":     strings.TrimSpace(profile.BusinessGoal),
		"audience":          strings.TrimSpace(profile.Audience),
		"operator_identity": strings.TrimSpace(profile.OperatorIdentity),
		"style_direction":   strings.TrimSpace(profile.StyleDirection),
		"must_capture_bias": profile.MustCaptureBias,
		"avoid_bias":        profile.AvoidBias,
		"risk_flags":        profile.RiskFlags,
		"quality_weights":   normalizeDirectiveQualityWeights(profile.QualityWeights),
		"directive_hint":    strings.TrimSpace(profile.DirectiveHint),
	}
	minCount, maxCount := normalizeCandidateCountBias(profile.CandidateCountMin, profile.CandidateCountMax, "png")
	out["candidate_count_bias"] = map[string]interface{}{
		"min": minCount,
		"max": maxCount,
	}
	if len(profile.TechnicalReject) > 0 {
		out["technical_reject"] = profile.TechnicalReject
	}
	return out
}

func ParseStrategyProfileFromAny(raw interface{}) VideoJobAI1StrategyProfile {
	value := mapFromAny(raw)
	if len(value) == 0 {
		return VideoJobAI1StrategyProfile{}
	}
	return VideoJobAI1StrategyProfile{
		Version:           strings.TrimSpace(stringFromAny(value["version"])),
		Scene:             NormalizeAdvancedScenario(firstNonEmptyString(stringFromAny(value["scene"]), stringFromAny(value["scenario"]))),
		SceneLabel:        strings.TrimSpace(stringFromAny(value["scene_label"])),
		BusinessGoal:      strings.TrimSpace(stringFromAny(value["business_goal"])),
		Audience:          strings.TrimSpace(stringFromAny(value["audience"])),
		OperatorIdentity:  strings.TrimSpace(stringFromAny(value["operator_identity"])),
		StyleDirection:    strings.TrimSpace(stringFromAny(value["style_direction"])),
		CandidateCountMin: resolveStrategyProfileCandidateCountMin(value),
		CandidateCountMax: resolveStrategyProfileCandidateCountMax(value),
		MustCaptureBias:   normalizeStringSlice(stringSliceFromAny(value["must_capture_bias"]), 16),
		AvoidBias:         normalizeStringSlice(stringSliceFromAny(value["avoid_bias"]), 16),
		RiskFlags:         normalizeStringSlice(stringSliceFromAny(value["risk_flags"]), 16),
		QualityWeights:    normalizeAI2QualityWeightsAny(value["quality_weights"]),
		TechnicalReject:   cloneMapStringKey(mapFromAny(value["technical_reject"])),
		DirectiveHint:     strings.TrimSpace(stringFromAny(value["directive_hint"])),
	}
}

func mergeTextHints(base []string, appendList []string, maxN int) []string {
	if maxN <= 0 {
		maxN = 16
	}
	merged := append([]string{}, base...)
	merged = append(merged, appendList...)
	return normalizeStringSlice(merged, maxN)
}

func mergeRiskFlags(base []string, appendList []string) []string {
	merged := append([]string{}, base...)
	merged = append(merged, appendList...)
	return normalizeStringSlice(merged, 16)
}

func blendQualityWeights(primary map[string]float64, secondary map[string]float64, primaryRatio float64) map[string]float64 {
	primary = normalizeDirectiveQualityWeights(primary)
	secondary = normalizeDirectiveQualityWeights(secondary)
	if primaryRatio <= 0 {
		primaryRatio = 0
	}
	if primaryRatio >= 1 {
		primaryRatio = 1
	}
	secondaryRatio := 1 - primaryRatio
	out := map[string]float64{
		"semantic":   primary["semantic"]*primaryRatio + secondary["semantic"]*secondaryRatio,
		"clarity":    primary["clarity"]*primaryRatio + secondary["clarity"]*secondaryRatio,
		"loop":       primary["loop"]*primaryRatio + secondary["loop"]*secondaryRatio,
		"efficiency": primary["efficiency"]*primaryRatio + secondary["efficiency"]*secondaryRatio,
	}
	return normalizeDirectiveQualityWeights(out)
}

func firstNonEmptyString(values ...string) string {
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveAdvancedSceneLabel(scene string) string {
	switch normalized := NormalizeAdvancedScenario(scene); normalized {
	case AdvancedScenarioXiaohongshu:
		return "小红书网感"
	case AdvancedScenarioWallpaper:
		return "手机壁纸"
	case AdvancedScenarioNews:
		return "新闻配图"
	case AdvancedScenarioDefault:
		return "通用截图"
	default:
		if normalized == "" {
			return "通用截图"
		}
		return normalized
	}
}

func hasPositiveQualityWeights(weights map[string]float64) bool {
	if len(weights) == 0 {
		return false
	}
	return weights["semantic"] > 0 || weights["clarity"] > 0 || weights["loop"] > 0 || weights["efficiency"] > 0
}

func hasStrategyProfileContent(profile VideoJobAI1StrategyProfile) bool {
	return strings.TrimSpace(profile.Scene) != "" ||
		strings.TrimSpace(profile.SceneLabel) != "" ||
		strings.TrimSpace(profile.BusinessGoal) != "" ||
		strings.TrimSpace(profile.Audience) != "" ||
		strings.TrimSpace(profile.OperatorIdentity) != "" ||
		strings.TrimSpace(profile.StyleDirection) != "" ||
		profile.CandidateCountMin > 0 ||
		profile.CandidateCountMax > 0 ||
		strings.TrimSpace(profile.DirectiveHint) != "" ||
		len(profile.MustCaptureBias) > 0 ||
		len(profile.AvoidBias) > 0 ||
		len(profile.RiskFlags) > 0 ||
		len(profile.QualityWeights) > 0 ||
		len(profile.TechnicalReject) > 0
}

func mergeStrategyProfileOverlay(base, overlay VideoJobAI1StrategyProfile) VideoJobAI1StrategyProfile {
	out := base
	if scene := NormalizeAdvancedScenario(overlay.Scene); scene != "" {
		out.Scene = scene
	}
	if label := strings.TrimSpace(overlay.SceneLabel); label != "" {
		out.SceneLabel = label
	}
	if version := strings.TrimSpace(overlay.Version); version != "" {
		out.Version = version
	}
	if goal := strings.TrimSpace(overlay.BusinessGoal); goal != "" {
		out.BusinessGoal = goal
	}
	if audience := strings.TrimSpace(overlay.Audience); audience != "" {
		out.Audience = audience
	}
	if identity := strings.TrimSpace(overlay.OperatorIdentity); identity != "" {
		out.OperatorIdentity = identity
	}
	if style := strings.TrimSpace(overlay.StyleDirection); style != "" {
		out.StyleDirection = style
	}
	if overlay.CandidateCountMin > 0 {
		out.CandidateCountMin = overlay.CandidateCountMin
	}
	if overlay.CandidateCountMax > 0 {
		out.CandidateCountMax = overlay.CandidateCountMax
	}
	if directive := strings.TrimSpace(overlay.DirectiveHint); directive != "" {
		out.DirectiveHint = directive
	}
	if len(overlay.MustCaptureBias) > 0 {
		out.MustCaptureBias = mergeTextHints(out.MustCaptureBias, overlay.MustCaptureBias, 16)
	}
	if len(overlay.AvoidBias) > 0 {
		out.AvoidBias = mergeTextHints(out.AvoidBias, overlay.AvoidBias, 16)
	}
	if len(overlay.RiskFlags) > 0 {
		out.RiskFlags = mergeRiskFlags(out.RiskFlags, overlay.RiskFlags)
	}
	if hasPositiveQualityWeights(overlay.QualityWeights) {
		out.QualityWeights = normalizeDirectiveQualityWeights(overlay.QualityWeights)
	}
	if len(overlay.TechnicalReject) > 0 {
		merged := cloneMapStringKey(out.TechnicalReject)
		if len(merged) == 0 {
			merged = map[string]interface{}{}
		}
		for key, value := range overlay.TechnicalReject {
			merged[key] = value
		}
		out.TechnicalReject = merged
	}
	out.MustCaptureBias = normalizeStringSlice(out.MustCaptureBias, 16)
	out.AvoidBias = normalizeStringSlice(out.AvoidBias, 16)
	out.RiskFlags = normalizeStringSlice(out.RiskFlags, 16)
	if len(out.QualityWeights) > 0 {
		out.QualityWeights = normalizeDirectiveQualityWeights(out.QualityWeights)
	}
	out.CandidateCountMin, out.CandidateCountMax = normalizeCandidateCountBias(out.CandidateCountMin, out.CandidateCountMax, "png")
	return out
}

func parseAI1SceneStrategyProfilesFromTemplateSchema(templateSchema map[string]interface{}) (map[string]VideoJobAI1StrategyProfile, string) {
	if len(templateSchema) == 0 {
		return nil, ""
	}
	root := mapFromAny(templateSchema[ai1SceneStrategiesSchemaKey])
	if len(root) == 0 {
		root = mapFromAny(templateSchema["scene_strategies"])
	}
	if len(root) == 0 {
		return nil, ""
	}

	version := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(root["version"]),
		stringFromAny(templateSchema["scene_strategy_version"]),
	))
	sceneItems := mapFromAny(root["scenes"])
	if len(sceneItems) == 0 {
		sceneItems = root
	}

	out := map[string]VideoJobAI1StrategyProfile{}
	for rawScene, rawEntry := range sceneItems {
		item := mapFromAny(rawEntry)
		if len(item) == 0 {
			continue
		}
		if _, hasEnabled := item["enabled"]; hasEnabled && !boolFromAny(item["enabled"]) {
			continue
		}
		profile := ParseStrategyProfileFromAny(item)
		explicitScene := firstNonEmptyString(stringFromAny(item["scene"]), stringFromAny(item["scenario"]))
		if strings.TrimSpace(explicitScene) == "" {
			profile.Scene = NormalizeAdvancedScenario(rawScene)
		} else {
			profile.Scene = NormalizeAdvancedScenario(explicitScene)
		}
		if strings.TrimSpace(profile.SceneLabel) == "" {
			profile.SceneLabel = resolveAdvancedSceneLabel(profile.Scene)
		}
		if strings.TrimSpace(profile.Version) == "" {
			profile.Version = version
		}
		profile.MustCaptureBias = normalizeStringSlice(profile.MustCaptureBias, 16)
		profile.AvoidBias = normalizeStringSlice(profile.AvoidBias, 16)
		profile.RiskFlags = normalizeStringSlice(profile.RiskFlags, 16)
		if len(profile.QualityWeights) > 0 {
			profile.QualityWeights = normalizeDirectiveQualityWeights(profile.QualityWeights)
		}
		profile.CandidateCountMin, profile.CandidateCountMax = normalizeCandidateCountBias(
			profile.CandidateCountMin,
			profile.CandidateCountMax,
			"png",
		)
		out[profile.Scene] = profile
	}
	if len(out) == 0 {
		return nil, version
	}
	return out, version
}

func (p *Processor) loadAI1SceneStrategyProfilesWithFallback(targetFormat string) (map[string]VideoJobAI1StrategyProfile, map[string]interface{}) {
	if p == nil || p.db == nil {
		return nil, nil
	}
	normalizedFormat := normalizeAIPromptTemplateFormat(targetFormat)
	candidates := []string{normalizedFormat}
	if normalizedFormat != "all" {
		candidates = append(candidates, "all")
	}
	for _, candidate := range candidates {
		row, found, err := p.loadAIPromptTemplateExact(candidate, "ai1", "editable")
		if err != nil || !found || !row.Enabled {
			continue
		}
		templateSchema := parseJSONMap(row.TemplateJSONSchema)
		sceneProfiles, schemaVersion := parseAI1SceneStrategyProfilesFromTemplateSchema(templateSchema)
		if len(sceneProfiles) == 0 {
			continue
		}
		meta := map[string]interface{}{
			"source":           "ops.video_ai_prompt_templates:" + candidate,
			"template_version": strings.TrimSpace(row.Version),
			"schema_version":   strings.TrimSpace(schemaVersion),
			"format_scope":     candidate,
		}
		return sceneProfiles, meta
	}
	return nil, nil
}

func (p *Processor) resolveVideoJobAI1StrategyProfileWithOverrides(
	targetFormat string,
	options VideoJobAdvancedOptions,
	existing VideoJobAI1StrategyProfile,
) (VideoJobAI1StrategyProfile, map[string]interface{}) {
	options = NormalizeVideoJobAdvancedOptions(options)
	profile := ResolveVideoJobAI1StrategyProfile(targetFormat, options)
	trace := map[string]interface{}{
		"source":          "built_in_default",
		"requested_scene": options.Scene,
	}

	if hasStrategyProfileContent(existing) {
		profile = mergeStrategyProfileOverlay(profile, existing)
		trace["job_profile_applied"] = true
	}

	adminSceneProfiles, adminMeta := p.loadAI1SceneStrategyProfilesWithFallback(targetFormat)
	if len(adminSceneProfiles) > 0 {
		matchedScene := options.Scene
		override, ok := adminSceneProfiles[matchedScene]
		if !ok {
			matchedScene = AdvancedScenarioDefault
			override, ok = adminSceneProfiles[matchedScene]
		}
		if ok {
			profile = mergeStrategyProfileOverlay(profile, override)
			trace["source"] = firstNonEmptyString(
				strings.TrimSpace(stringFromAny(adminMeta["source"])),
				"ops.video_ai_prompt_templates",
			)
			trace["matched_scene"] = matchedScene
			if value := strings.TrimSpace(stringFromAny(adminMeta["template_version"])); value != "" {
				trace["template_version"] = value
			}
			if value := strings.TrimSpace(stringFromAny(adminMeta["schema_version"])); value != "" {
				trace["schema_version"] = value
			}
			if value := strings.TrimSpace(stringFromAny(adminMeta["format_scope"])); value != "" {
				trace["format_scope"] = value
			}
		}
	}

	if profile.Scene == "" {
		profile.Scene = options.Scene
	}
	if profile.Scene == "" {
		profile.Scene = AdvancedScenarioDefault
	}
	if strings.TrimSpace(profile.SceneLabel) == "" {
		profile.SceneLabel = resolveAdvancedSceneLabel(profile.Scene)
	}
	if strings.TrimSpace(profile.Version) == "" {
		profile.Version = "ai1_strategy_profile_v1"
	}
	if strings.TrimSpace(profile.OperatorIdentity) == "" {
		profile.OperatorIdentity = "视觉总监"
	}
	if len(profile.QualityWeights) > 0 {
		profile.QualityWeights = normalizeDirectiveQualityWeights(profile.QualityWeights)
	}
	profile.CandidateCountMin, profile.CandidateCountMax = normalizeCandidateCountBias(
		profile.CandidateCountMin,
		profile.CandidateCountMax,
		targetFormat,
	)
	if len(profile.TechnicalReject) > 0 {
		technicalReject := cloneMapStringKey(profile.TechnicalReject)
		technicalReject["max_blur_tolerance"] = normalizeAI1MaxBlurTolerance(
			stringFromAny(technicalReject["max_blur_tolerance"]),
			targetFormat,
		)
		profile.TechnicalReject = technicalReject
	}
	trace["resolved_scene"] = profile.Scene
	trace["resolved_scene_label"] = profile.SceneLabel
	trace["resolved_version"] = profile.Version
	return profile, trace
}

func EncodeAI1SceneStrategyProfilesForTemplateSchema(
	version string,
	profiles map[string]VideoJobAI1StrategyProfile,
) map[string]interface{} {
	root := map[string]interface{}{}
	if strings.TrimSpace(version) != "" {
		root["version"] = strings.TrimSpace(version)
	}
	if len(profiles) == 0 {
		return map[string]interface{}{ai1SceneStrategiesSchemaKey: root}
	}
	scenes := map[string]interface{}{}
	sceneKeys := make([]string, 0, len(profiles))
	for scene := range profiles {
		normalized := NormalizeAdvancedScenario(scene)
		if normalized == "" {
			continue
		}
		sceneKeys = append(sceneKeys, normalized)
	}
	sort.Strings(sceneKeys)
	for _, scene := range sceneKeys {
		profile := profiles[scene]
		payload := StrategyProfileToMap(profile)
		payload["scene"] = scene
		scenes[scene] = payload
	}
	if len(scenes) > 0 {
		root["scenes"] = scenes
	}
	return map[string]interface{}{ai1SceneStrategiesSchemaKey: root}
}

func DecodeAI1SceneStrategyProfilesFromTemplateSchema(rawJSON []byte) (map[string]VideoJobAI1StrategyProfile, string) {
	if len(rawJSON) == 0 {
		return nil, ""
	}
	root := map[string]interface{}{}
	if err := json.Unmarshal(rawJSON, &root); err != nil {
		return nil, ""
	}
	return parseAI1SceneStrategyProfilesFromTemplateSchema(root)
}

func resolveStrategyProfileCandidateCountMin(value map[string]interface{}) int {
	bias := mapFromAny(value["candidate_count_bias"])
	if min := intFromAny(bias["min"]); min > 0 {
		return min
	}
	return intFromAny(value["candidate_count_min"])
}

func resolveStrategyProfileCandidateCountMax(value map[string]interface{}) int {
	bias := mapFromAny(value["candidate_count_bias"])
	if max := intFromAny(bias["max"]); max > 0 {
		return max
	}
	return intFromAny(value["candidate_count_max"])
}

func normalizeCandidateCountBias(minCount, maxCount int, targetFormat string) (int, int) {
	targetFormat = NormalizeRequestedFormat(strings.TrimSpace(targetFormat))
	defaultMin := 4
	defaultMax := 8
	if targetFormat == "gif" {
		defaultMin = 3
		defaultMax = 6
	}
	if minCount <= 0 {
		minCount = defaultMin
	}
	if maxCount <= 0 {
		maxCount = defaultMax
	}
	if minCount < 1 {
		minCount = 1
	}
	if minCount > 80 {
		minCount = 80
	}
	if maxCount < minCount {
		maxCount = minCount
	}
	if maxCount > 80 {
		maxCount = 80
	}
	if maxCount < minCount {
		maxCount = minCount
	}
	return minCount, maxCount
}
