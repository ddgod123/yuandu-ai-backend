package videojobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"emoji/internal/models"

	"gorm.io/gorm"
)

func parseAIGIFDirectiveResponse(modelText string) (gifAIDirectiveProfile, string, error) {
	rawText := sanitizeModelJSON(modelText)
	var canonical gifAIDirectorResponse
	if err := json.Unmarshal([]byte(rawText), &canonical); err == nil {
		if strings.TrimSpace(canonical.Directive.BusinessGoal) != "" ||
			strings.TrimSpace(canonical.Directive.Audience) != "" ||
			len(canonical.Directive.MustCapture) > 0 ||
			len(canonical.Directive.Avoid) > 0 ||
			canonical.Directive.ClipCountMin > 0 ||
			canonical.Directive.ClipCountMax > 0 ||
			canonical.Directive.DurationPrefMaxSec > 0 ||
			len(canonical.Directive.QualityWeights) > 0 ||
			strings.TrimSpace(canonical.Directive.DirectiveText) != "" {
			return canonical.Directive, "directive", nil
		}
	}

	var brief gifAIDirectorBriefV2Response
	if err := json.Unmarshal([]byte(rawText), &brief); err == nil {
		directive := gifAIDirectiveProfile{
			BusinessGoal:   strings.TrimSpace(brief.BusinessGoal),
			Audience:       strings.TrimSpace(brief.Audience),
			MustCapture:    normalizeStringSlice(brief.MustCapture, 8),
			Avoid:          normalizeStringSlice(brief.Avoid, 8),
			StyleDirection: strings.TrimSpace(brief.StyleDirection),
			RiskFlags:      normalizeStringSlice(brief.RiskFlags, 8),
			QualityWeights: brief.QualityWeights,
			DirectiveText:  strings.TrimSpace(brief.DirectiveText),
		}
		if directive.DirectiveText == "" {
			directive.DirectiveText = strings.TrimSpace(brief.PlannerInstruction)
		}
		if len(brief.ClipCountRange) >= 1 {
			directive.ClipCountMin = int(brief.ClipCountRange[0])
		}
		if len(brief.ClipCountRange) >= 2 {
			directive.ClipCountMax = int(brief.ClipCountRange[1])
		}
		if len(brief.DurationPrefSecRange) >= 1 {
			directive.DurationPrefMinSec = brief.DurationPrefSecRange[0]
		}
		if len(brief.DurationPrefSecRange) >= 2 {
			directive.DurationPrefMaxSec = brief.DurationPrefSecRange[1]
		}
		if directive.BusinessGoal != "" ||
			len(directive.MustCapture) > 0 ||
			len(directive.Avoid) > 0 ||
			directive.ClipCountMin > 0 ||
			directive.ClipCountMax > 0 ||
			directive.DurationPrefMaxSec > 0 {
			return directive, "brief_v2_flat", nil
		}
	}

	return gifAIDirectiveProfile{}, "", fmt.Errorf("director response does not match supported schema")
}

func resolveAIDirectorAssetGoal(format string) string {
	switch normalizeAIPromptTemplateFormat(format) {
	case "gif":
		return "gif_highlight"
	case "png", "jpg":
		return "keyframe_image_set"
	case "webp":
		return "webp_reaction_set"
	case "live":
		return "live_cover_set"
	default:
		return "visual_asset_set"
	}
}

func resolveAIDirectorDeliveryGoal(format string) string {
	switch normalizeAIPromptTemplateFormat(format) {
	case "gif", "webp":
		return "standalone_shareable"
	case "png", "jpg":
		return "high_quality_images"
	case "live":
		return "cover_ready"
	default:
		return "general_delivery"
	}
}

func validateAIGIFDirectiveContract(directive gifAIDirectiveProfile) error {
	if strings.TrimSpace(directive.BusinessGoal) == "" {
		return fmt.Errorf("business_goal required")
	}
	if directive.ClipCountMin < 1 {
		return fmt.Errorf("clip_count_min must be >=1")
	}
	if directive.ClipCountMax < directive.ClipCountMin {
		return fmt.Errorf("clip_count_max must be >= clip_count_min")
	}
	if directive.DurationPrefMaxSec <= directive.DurationPrefMinSec {
		return fmt.Errorf("duration_pref_max_sec must be > duration_pref_min_sec")
	}
	if directive.LoopPreference < 0 || directive.LoopPreference > 1 {
		return fmt.Errorf("loop_preference must be in [0,1]")
	}
	weights := normalizeDirectiveQualityWeights(directive.QualityWeights)
	required := []string{"semantic", "clarity", "loop", "efficiency"}
	sum := 0.0
	for _, key := range required {
		v, ok := weights[key]
		if !ok {
			return fmt.Errorf("quality_weights.%s required", key)
		}
		if v < 0 || v > 1 {
			return fmt.Errorf("quality_weights.%s must be in [0,1]", key)
		}
		sum += v
	}
	if sum < 0.98 || sum > 1.02 {
		return fmt.Errorf("quality_weights sum out of range")
	}
	return nil
}

func resolveAIDirectorSourceAspectRatio(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	a, b := width, height
	for b != 0 {
		a, b = b, a%b
	}
	if a <= 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d", width/a, height/a)
}

func resolveAIDirectorSourceOrientation(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if width == height {
		return "square"
	}
	if width > height {
		return "landscape"
	}
	return "portrait"
}

func resolveAIDirectorOptimizationTarget(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "clarity":
		return "clarity_first"
	case "size":
		return "size_first"
	case "balanced":
		return "balanced"
	default:
		return "balanced"
	}
}

func resolveAIDirectorCostSensitivity(targetSizeKB int) string {
	switch {
	case targetSizeKB <= 0:
		return "medium"
	case targetSizeKB <= 1024:
		return "high"
	case targetSizeKB <= 2048:
		return "medium"
	default:
		return "low"
	}
}

func resolveAIDirectorTaskConstraints(meta videoProbeMeta, qualitySettings QualitySettings) aiGIFDirectorTaskConstraints {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	decision := resolveAIGIFPlannerTargetTopNDecision(meta, nil, qualitySettings)
	targetMax := decision.AllowedTopN
	if targetMax <= 0 {
		targetMax = 3
	}
	targetMin := decision.BaseTopN - 2
	if targetMin < 1 {
		targetMin = 1
	}
	if targetMin > targetMax {
		targetMin = targetMax
	}
	targetDuration := chooseHighlightDuration(meta.DurationSec)
	durationMin := roundTo(clampFloat(targetDuration*0.7, 0.8, 4.0), 3)
	durationMax := roundTo(clampFloat(targetDuration*1.35, durationMin+0.4, 6.0), 3)
	if qualitySettings.AIDirectorConstraintOverrideEnabled && qualitySettings.AIDirectorDurationExpandRatio > 0 {
		durationMax = roundTo(durationMax*(1.0+qualitySettings.AIDirectorDurationExpandRatio), 3)
	}
	durationCap := qualitySettings.AIDirectorDurationAbsoluteCapSec
	if durationCap <= 0 {
		durationCap = NormalizeQualitySettings(qualitySettings).AIDirectorDurationAbsoluteCapSec
	}
	durationMax = roundTo(clampFloat(durationMax, durationMin+0.4, clampFloat(durationCap, durationMin+0.4, 12.0)), 3)
	if durationMax <= durationMin {
		durationMax = roundTo(durationMin+0.8, 3)
	}
	return aiGIFDirectorTaskConstraints{
		TargetCountMin:          targetMin,
		TargetCountMax:          targetMax,
		PreferredDurationSecMin: durationMin,
		PreferredDurationSecMax: durationMax,
	}
}

func resolveAIDirectorRiskHints(meta videoProbeMeta) []string {
	hints := make([]string, 0, 4)
	appendHint := func(value string) {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			return
		}
		if !containsString(hints, value) {
			hints = append(hints, value)
		}
	}
	if meta.FPS > 0 && meta.FPS < 15 {
		appendHint("low_fps")
	}
	if meta.DurationSec >= 180 {
		appendHint("long_video")
	}
	if meta.Width > 0 && meta.Height > 0 {
		longSide := meta.Width
		if meta.Height > longSide {
			longSide = meta.Height
		}
		if longSide > 0 && longSide < 480 {
			appendHint("low_resolution")
		}
	}
	return hints
}

func (p *Processor) requestAIGIFPromptDirective(
	ctx context.Context,
	job models.VideoJob,
	sourcePath string,
	meta videoProbeMeta,
	local highlightSuggestion,
	qualitySettings QualitySettings,
) (*gifAIDirectiveProfile, map[string]interface{}, error) {
	requestedFormats := normalizeOutputFormats(job.OutputFormats)
	targetFormat := "gif"
	if len(requestedFormats) > 0 {
		targetFormat = strings.ToLower(strings.TrimSpace(requestedFormats[0]))
	}
	targetFormat = normalizeAIPromptTemplateFormat(targetFormat)

	cfg := p.loadGIFAIDirectorConfig()
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	promptPack := p.resolveAIGIFDirectorPromptPack(targetFormat, qualitySettings)

	info := map[string]interface{}{
		"enabled":                            cfg.Enabled,
		"provider":                           cfg.Provider,
		"model":                              cfg.Model,
		"prompt_version":                     cfg.PromptVersion,
		"fixed_prompt_version":               promptPack.FixedPromptVersion,
		"fixed_prompt_source":                promptPack.FixedPromptSource,
		"fixed_prompt_contract_tail_version": promptPack.ContractTailVersion,
		"operator_instruction_enabled":       promptPack.OperatorEnabled,
		"operator_instruction_version":       promptPack.OperatorVersion,
		"operator_instruction_source":        promptPack.OperatorSource,
		"operator_instruction_raw_len":       len(promptPack.OperatorInstructionRaw),
		"operator_instruction_rendered_len":  len(promptPack.OperatorInstructionRendered),
		"operator_instruction_render_mode":   promptPack.OperatorInstructionRenderMode,
		"target_format":                      targetFormat,
	}
	if len(promptPack.OperatorInstructionSchema) > 0 {
		info["operator_instruction_schema"] = promptPack.OperatorInstructionSchema
	}
	if !cfg.Enabled {
		info["applied"] = false
		return nil, info, fmt.Errorf("director disabled")
	}
	directorRuntime := &aiGIFDirectorRuntime{
		ctx:                           ctx,
		job:                           job,
		sourcePath:                    sourcePath,
		meta:                          meta,
		qualitySettings:               qualitySettings,
		targetFormat:                  targetFormat,
		operatorInstructionRaw:        promptPack.OperatorInstructionRaw,
		operatorInstructionRendered:   promptPack.OperatorInstructionRendered,
		operatorInstructionSchema:     promptPack.OperatorInstructionSchema,
		operatorInstructionRenderMode: promptPack.OperatorInstructionRenderMode,
		operatorVersion:               promptPack.OperatorVersion,
		operatorSource:                promptPack.OperatorSource,
		operatorEnabled:               promptPack.OperatorEnabled,
		directorInputModeRequested:    normalizeAIDirectorInputModeSetting(qualitySettings.AIDirectorInputMode, "hybrid"),
		directorInputModeApplied:      "frames",
		directorInputSource:           "frame_manifest",
		sourceVideoURL:                "",
		sourceVideoURLError:           "",
		frameSamplingError:            "",
		frameSamples:                  make([]aiDirectorFrameSample, 0),
		frameManifest:                 make([]aiGIFFrameManifestEntry, 0),
	}
	p.resolveAIGIFDirectorInputMode(directorRuntime)

	modelPayload, debugPayload := p.buildAIGIFDirectorPayloads(directorRuntime)
	userParts, userBytes := buildAIGIFDirectorUserParts(directorRuntime, modelPayload)

	systemPrompt := strings.TrimSpace(promptPack.FixedPromptCore)
	if promptPack.OperatorEnabled && strings.TrimSpace(promptPack.OperatorInstructionRendered) != "" {
		systemPrompt += "\n\n额外的运营指令模板（必须严格遵守，冲突时优先级高于默认偏好）：" +
			"\n版本：" + promptPack.OperatorVersion +
			"\n" + promptPack.OperatorInstructionRendered
	}
	if strings.TrimSpace(promptPack.FixedPromptContractTail) != "" {
		systemPrompt += "\n\n" + strings.TrimSpace(promptPack.FixedPromptContractTail)
	}
	if directorRuntime.frameSamplingError != "" {
		info["frame_sampling_error"] = directorRuntime.frameSamplingError
	}
	info["frame_count"] = len(directorRuntime.frameSamples)
	info["director_input_mode_requested"] = directorRuntime.directorInputModeRequested
	info["director_input_mode_applied"] = directorRuntime.directorInputModeApplied
	info["director_input_source"] = directorRuntime.directorInputSource
	info["director_model_payload_schema_version"] = modelPayload.SchemaVersion
	info["source_video_url_available"] = directorRuntime.sourceVideoURL != ""
	info["source_video_url_error"] = directorRuntime.sourceVideoURLError

	modelText, _, rawResp, _, err := p.callAndRecordAIGIFDirector(
		directorRuntime,
		systemPrompt,
		1,
		cfg,
		modelPayload,
		debugPayload,
		userBytes,
		userParts,
		promptPack.FixedPromptVersion,
		promptPack.FixedPromptSource,
		promptPack.ContractTailVersion,
	)
	if err != nil && directorRuntime.directorInputModeRequested == "hybrid" && directorRuntime.sourceVideoURL != "" && strings.EqualFold(directorRuntime.directorInputModeApplied, "full_video") {
		p.loadAIGIFDirectorFrameSamples(directorRuntime)
		if len(directorRuntime.frameSamples) > 0 {
			directorRuntime.directorInputModeApplied = "frames"
			directorRuntime.directorInputSource = "hybrid_retry_frame_manifest"
			directorRuntime.sourceVideoURL = ""
			modelPayload, debugPayload = p.buildAIGIFDirectorPayloads(directorRuntime)
			userParts, userBytes = buildAIGIFDirectorUserParts(directorRuntime, modelPayload)
			info["hybrid_retry"] = "video_input_error_fallback_to_frames"
			info["hybrid_first_error"] = err.Error()
			modelText, _, rawResp, _, err = p.callAndRecordAIGIFDirector(
				directorRuntime,
				systemPrompt,
				2,
				cfg,
				modelPayload,
				debugPayload,
				userBytes,
				userParts,
				promptPack.FixedPromptVersion,
				promptPack.FixedPromptSource,
				promptPack.ContractTailVersion,
			)
		}
	}
	info["frame_count"] = len(directorRuntime.frameSamples)
	info["director_input_mode_applied"] = directorRuntime.directorInputModeApplied
	info["director_input_source"] = directorRuntime.directorInputSource
	info["source_video_url_available"] = directorRuntime.sourceVideoURL != ""
	info["source_video_url_error"] = directorRuntime.sourceVideoURLError
	if err != nil {
		fallbackDirective := buildFallbackAIGIFDirective(local, qualitySettings, "director_call_error")
		_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *fallbackDirective, rawResp, buildAIGIFDirectivePersistContext(directorRuntime, cfg, modelPayload, "fallback", true, rawResp, "director_call_error"))
		info["applied"] = false
		info["status"] = "fallback"
		info["fallback_used"] = true
		info["error"] = err.Error()
		return nil, info, err
	}

	parsedDirective, responseShape, parseErr := parseAIGIFDirectiveResponse(modelText)
	if parseErr != nil {
		fallbackDirective := buildFallbackAIGIFDirective(local, qualitySettings, "director_parse_error")
		_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *fallbackDirective, rawResp, buildAIGIFDirectivePersistContext(directorRuntime, cfg, modelPayload, "fallback", true, rawResp, "director_parse_error"))
		info["applied"] = false
		info["status"] = "fallback"
		info["fallback_used"] = true
		info["error"] = "parse director response: " + parseErr.Error()
		return nil, info, parseErr
	}
	directive := normalizeAIGIFDirective(parsedDirective, qualitySettings.GIFCandidateMaxOutputs)
	if directive == nil {
		fallbackDirective := buildFallbackAIGIFDirective(local, qualitySettings, "director_output_invalid")
		_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *fallbackDirective, rawResp, buildAIGIFDirectivePersistContext(directorRuntime, cfg, modelPayload, "fallback", true, rawResp, "director_output_invalid"))
		info["applied"] = false
		info["status"] = "fallback"
		info["fallback_used"] = true
		info["error"] = "director output invalid"
		return nil, info, fmt.Errorf("director output invalid")
	}
	if contractErr := validateAIGIFDirectiveContract(*directive); contractErr != nil {
		fallbackDirective := buildFallbackAIGIFDirective(local, qualitySettings, "director_contract_invalid")
		_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *fallbackDirective, rawResp, buildAIGIFDirectivePersistContext(directorRuntime, cfg, modelPayload, "fallback", true, rawResp, "director_contract_invalid"))
		info["applied"] = false
		info["status"] = "fallback"
		info["fallback_used"] = true
		info["error"] = "director contract invalid: " + contractErr.Error()
		return nil, info, contractErr
	}
	_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *directive, rawResp, buildAIGIFDirectivePersistContext(directorRuntime, cfg, modelPayload, "ok", false, rawResp, ""))

	info["applied"] = true
	info["status"] = "ok"
	info["fallback_used"] = false
	info["response_shape"] = responseShape
	info["business_goal"] = directive.BusinessGoal
	info["clip_count_min"] = directive.ClipCountMin
	info["clip_count_max"] = directive.ClipCountMax
	info["duration_pref_min_sec"] = directive.DurationPrefMinSec
	info["duration_pref_max_sec"] = directive.DurationPrefMaxSec
	return directive, info, nil
}

func normalizeAIGIFDirective(in gifAIDirectiveProfile, fallbackMaxOutputs int) *gifAIDirectiveProfile {
	out := in
	out.BusinessGoal = strings.ToLower(strings.TrimSpace(out.BusinessGoal))
	if out.BusinessGoal == "" {
		out.BusinessGoal = "social_spread"
	}
	out.Audience = strings.TrimSpace(out.Audience)
	out.MustCapture = normalizeStringSlice(out.MustCapture, 8)
	out.Avoid = normalizeStringSlice(out.Avoid, 8)
	out.StyleDirection = strings.TrimSpace(out.StyleDirection)
	if out.StyleDirection == "" {
		out.StyleDirection = "balanced_reaction"
	}
	out.RiskFlags = normalizeStringSlice(out.RiskFlags, 8)
	if out.ClipCountMin <= 0 {
		out.ClipCountMin = 3
	}
	if out.ClipCountMax <= 0 {
		out.ClipCountMax = out.ClipCountMin + 2
	}
	if fallbackMaxOutputs > 0 && out.ClipCountMax > fallbackMaxOutputs*3 {
		out.ClipCountMax = fallbackMaxOutputs * 3
	}
	if out.ClipCountMax < out.ClipCountMin {
		out.ClipCountMax = out.ClipCountMin
	}
	out.DurationPrefMinSec = roundTo(clampFloat(out.DurationPrefMinSec, 0.8, 4.0), 3)
	out.DurationPrefMaxSec = roundTo(clampFloat(out.DurationPrefMaxSec, 1.0, 6.0), 3)
	if out.DurationPrefMaxSec <= out.DurationPrefMinSec {
		out.DurationPrefMaxSec = roundTo(out.DurationPrefMinSec+0.8, 3)
	}
	out.LoopPreference = roundTo(clampZeroOne(out.LoopPreference), 4)
	if out.QualityWeights == nil {
		out.QualityWeights = map[string]float64{}
	}
	normalizedWeights := normalizeDirectiveQualityWeights(out.QualityWeights)
	out.QualityWeights = normalizedWeights
	out.DirectiveText = strings.TrimSpace(out.DirectiveText)
	if out.DirectiveText == "" {
		out.DirectiveText = fmt.Sprintf("优先抓取%s场景，建议窗口 %.1f~%.1f 秒，避免低价值过渡镜头。", out.BusinessGoal, out.DurationPrefMinSec, out.DurationPrefMaxSec)
	}
	return &out
}

func buildFallbackAIGIFDirective(local highlightSuggestion, qualitySettings QualitySettings, reason string) *gifAIDirectiveProfile {
	def := NormalizeQualitySettings(qualitySettings)
	clipMin := 2
	clipMax := def.GIFCandidateMaxOutputs
	if clipMax <= 0 {
		clipMax = 3
	}
	if clipMax < clipMin {
		clipMax = clipMin
	}
	durationMin := 1.4
	durationMax := 3.2
	if local.Selected != nil {
		selectedDuration := local.Selected.EndSec - local.Selected.StartSec
		if selectedDuration > 0 {
			durationMin = clampFloat(selectedDuration*0.7, 0.8, 3.2)
			durationMax = clampFloat(selectedDuration*1.35, durationMin+0.5, 5.0)
		}
	}
	directive := gifAIDirectiveProfile{
		BusinessGoal:       "social_spread",
		Audience:           "通用用户",
		MustCapture:        []string{"情绪峰值", "动作完成点"},
		Avoid:              []string{"转场过渡", "低清晰度片段"},
		ClipCountMin:       clipMin,
		ClipCountMax:       clipMax,
		DurationPrefMinSec: roundTo(durationMin, 3),
		DurationPrefMaxSec: roundTo(durationMax, 3),
		LoopPreference:     0.35,
		StyleDirection:     "balanced_reaction",
		RiskFlags:          []string{strings.TrimSpace(reason)},
		QualityWeights: map[string]float64{
			"semantic":   0.35,
			"clarity":    0.20,
			"loop":       0.25,
			"efficiency": 0.20,
		},
		DirectiveText: "AI1 回退策略：优先情绪/反应峰值片段，控制时长，避免低价值过渡镜头。",
	}
	return normalizeAIGIFDirective(directive, def.GIFCandidateMaxOutputs)
}

func resolveAIDirectiveBriefVersion(operatorVersion, promptVersion string) string {
	if value := strings.TrimSpace(operatorVersion); value != "" {
		return value
	}
	if value := strings.TrimSpace(promptVersion); value != "" {
		return value
	}
	return "v1"
}

func resolveAIDirectiveModelVersion(cfgModel string, raw map[string]interface{}) string {
	if value := strings.TrimSpace(stringFromAny(raw["model"])); value != "" {
		return value
	}
	if value := strings.TrimSpace(cfgModel); value != "" {
		return value
	}
	return ""
}

func normalizeDirectiveQualityWeights(raw map[string]float64) map[string]float64 {
	keys := []string{"semantic", "clarity", "loop", "efficiency"}
	out := map[string]float64{}
	sum := 0.0
	for _, key := range keys {
		value := clampZeroOne(raw[key])
		out[key] = value
		sum += value
	}
	if sum <= 0 {
		return map[string]float64{
			"semantic":   0.35,
			"clarity":    0.20,
			"loop":       0.25,
			"efficiency": 0.20,
		}
	}
	for _, key := range keys {
		out[key] = roundTo(out[key]/sum, 4)
	}
	return out
}

func normalizeStringSlice(in []string, maxN int) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
		if maxN > 0 && len(out) >= maxN {
			break
		}
	}
	return out
}

func (p *Processor) persistAIGIFDirective(
	jobID uint64,
	userID uint64,
	cfg aiModelCallConfig,
	directive gifAIDirectiveProfile,
	raw map[string]interface{},
	persist aiGIFDirectivePersistContext,
) error {
	if p == nil || p.db == nil || jobID == 0 || userID == 0 {
		return nil
	}
	status := strings.ToLower(strings.TrimSpace(persist.Status))
	if status == "" {
		status = "ok"
	}
	briefVersion := strings.TrimSpace(persist.BriefVersion)
	if briefVersion == "" {
		briefVersion = resolveAIDirectiveBriefVersion("", cfg.PromptVersion)
	}
	modelVersion := strings.TrimSpace(persist.ModelVersion)
	if modelVersion == "" {
		modelVersion = resolveAIDirectiveModelVersion(cfg.Model, raw)
	}
	row := models.VideoJobGIFAIDirective{
		JobID:              jobID,
		UserID:             userID,
		Provider:           cfg.Provider,
		Model:              cfg.Model,
		Endpoint:           cfg.Endpoint,
		PromptVersion:      cfg.PromptVersion,
		BusinessGoal:       directive.BusinessGoal,
		Audience:           directive.Audience,
		MustCapture:        mustJSON(directive.MustCapture),
		Avoid:              mustJSON(directive.Avoid),
		ClipCountMin:       directive.ClipCountMin,
		ClipCountMax:       directive.ClipCountMax,
		DurationPrefMinSec: directive.DurationPrefMinSec,
		DurationPrefMaxSec: directive.DurationPrefMaxSec,
		LoopPreference:     directive.LoopPreference,
		StyleDirection:     directive.StyleDirection,
		RiskFlags:          mustJSON(directive.RiskFlags),
		QualityWeights:     mustJSON(directive.QualityWeights),
		BriefVersion:       briefVersion,
		ModelVersion:       modelVersion,
		DirectiveText:      directive.DirectiveText,
		InputContextJSON:   mustJSON(persist.InputContext),
		Status:             status,
		FallbackUsed:       persist.FallbackUsed,
		Metadata: mustJSON(aiGIFDirectiveRowMetadata{
			MustCaptureCount:           len(directive.MustCapture),
			AvoidCount:                 len(directive.Avoid),
			RiskFlagsCount:             len(directive.RiskFlags),
			OperatorInstructionEnabled: persist.OperatorEnabled,
			OperatorInstructionVersion: strings.TrimSpace(persist.OperatorVersion),
			OperatorInstructionLen:     len(strings.TrimSpace(persist.OperatorInstruction)),
			FallbackReason:             strings.TrimSpace(persist.FallbackReason),
		}),
		RawResponse: mustJSON(raw),
	}
	err := p.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("job_id = ?", jobID).Delete(&models.VideoJobGIFAIDirective{}).Error; err != nil {
			return err
		}
		return tx.Create(&row).Error
	})
	if err != nil && isMissingTableError(err, "video_job_gif_ai_directives") {
		return nil
	}
	return err
}
