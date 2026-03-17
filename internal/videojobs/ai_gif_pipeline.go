package videojobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	gifAIDirectorStage = "director"
	gifAIPlannerStage  = "planner"
	gifAIJudgeStage    = "judge"
	// DeepSeek reasoner may return long reasoning_content payloads.
	// Keep the response cap high to avoid truncation-induced JSON decode errors.
	openAICompatMaxRespBytes = 8 << 20 // 8 MiB
	// AI3 hard gate thresholds: AI review cannot promote hard-failed outputs to deliver.
	gifAIJudgeHardGateMinOverallScore = 0.20
	gifAIJudgeHardGateMinClarityScore = 0.20
	gifAIJudgeHardGateMinLoopScore    = 0.20
	gifAIJudgeHardGateMinOutputScore  = 0.20
	gifAIJudgeHardGateMinDurationMS   = 200
	gifAIJudgeHardGateSizeMultiplier  = 4 // hard exceed budget => severe oversize
)

type aiModelCallConfig struct {
	Enabled       bool
	Provider      string
	Model         string
	Endpoint      string
	APIKey        string
	PromptVersion string
	Timeout       time.Duration
	MaxTokens     int
}

type openAICompatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAICompatChatRequest struct {
	Model       string                `json:"model"`
	MaxTokens   int                   `json:"max_tokens,omitempty"`
	Temperature float64               `json:"temperature,omitempty"`
	Messages    []openAICompatMessage `json:"messages"`
}

type gifAIPlannerProposal struct {
	ProposalRank         int      `json:"proposal_rank"`
	StartSec             float64  `json:"start_sec"`
	EndSec               float64  `json:"end_sec"`
	Score                float64  `json:"score"`
	ProposalReason       string   `json:"proposal_reason"`
	SemanticTags         []string `json:"semantic_tags"`
	ExpectedValueLevel   string   `json:"expected_value_level"`
	StandaloneConfidence float64  `json:"standalone_confidence"`
	LoopFriendlinessHint float64  `json:"loop_friendliness_hint"`
}

type gifAIDirectiveProfile struct {
	BusinessGoal       string             `json:"business_goal"`
	Audience           string             `json:"audience"`
	MustCapture        []string           `json:"must_capture"`
	Avoid              []string           `json:"avoid"`
	ClipCountMin       int                `json:"clip_count_min"`
	ClipCountMax       int                `json:"clip_count_max"`
	DurationPrefMinSec float64            `json:"duration_pref_min_sec"`
	DurationPrefMaxSec float64            `json:"duration_pref_max_sec"`
	LoopPreference     float64            `json:"loop_preference"`
	StyleDirection     string             `json:"style_direction"`
	RiskFlags          []string           `json:"risk_flags"`
	QualityWeights     map[string]float64 `json:"quality_weights"`
	DirectiveText      string             `json:"directive_text"`
}

type gifAIDirectorResponse struct {
	Directive gifAIDirectiveProfile `json:"directive"`
}

type gifAIPlannerResponse struct {
	Proposals    []gifAIPlannerProposal `json:"proposals"`
	SelectedRank int                    `json:"selected_rank"`
	Notes        string                 `json:"notes"`
}

type gifAIJudgeReviewRow struct {
	OutputID            uint64  `json:"output_id"`
	ProposalRank        int     `json:"proposal_rank"`
	FinalRecommendation string  `json:"final_recommendation"`
	SemanticVerdict     float64 `json:"semantic_verdict"`
	DiagnosticReason    string  `json:"diagnostic_reason"`
	SuggestedAction     string  `json:"suggested_action"`
}

type gifAIJudgeResponse struct {
	Reviews []gifAIJudgeReviewRow  `json:"reviews"`
	Summary map[string]interface{} `json:"summary"`
}

type gifJudgeSample struct {
	OutputID        uint64
	Score           float64
	SizeBytes       int64
	Width           int
	Height          int
	DurationMs      int
	StartSec        float64
	EndSec          float64
	Reason          string
	EvalOverall     float64
	EvalEmotion     float64
	EvalClarity     float64
	EvalMotion      float64
	EvalLoop        float64
	EvalEfficiency  float64
	ProposalIDByWin *uint64
	ProposalRank    int
}

type aiGIFDirectivePersistContext struct {
	Status              string
	FallbackUsed        bool
	BriefVersion        string
	ModelVersion        string
	InputContext        map[string]interface{}
	OperatorEnabled     bool
	OperatorInstruction string
	OperatorVersion     string
	FallbackReason      string
}

func (p *Processor) loadGIFAIPlannerConfig() aiModelCallConfig {
	timeoutSec := p.cfg.AIPlannerTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	maxTokens := p.cfg.AIPlannerMaxTokens
	if maxTokens <= 0 {
		maxTokens = 1200
	}
	return aiModelCallConfig{
		Enabled:       p.cfg.AIPlannerEnabled,
		Provider:      strings.ToLower(strings.TrimSpace(p.cfg.AIPlannerProvider)),
		Model:         strings.TrimSpace(p.cfg.AIPlannerModel),
		Endpoint:      strings.TrimSpace(p.cfg.AIPlannerEndpoint),
		APIKey:        strings.TrimSpace(p.cfg.AIPlannerAPIKey),
		PromptVersion: strings.TrimSpace(p.cfg.AIPlannerPromptVersion),
		Timeout:       time.Duration(timeoutSec) * time.Second,
		MaxTokens:     maxTokens,
	}
}

func (p *Processor) loadGIFAIDirectorConfig() aiModelCallConfig {
	timeoutSec := p.cfg.AIDirectorTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 16
	}
	maxTokens := p.cfg.AIDirectorMaxTokens
	if maxTokens <= 0 {
		maxTokens = 1000
	}
	return aiModelCallConfig{
		Enabled:       p.cfg.AIDirectorEnabled,
		Provider:      strings.ToLower(strings.TrimSpace(p.cfg.AIDirectorProvider)),
		Model:         strings.TrimSpace(p.cfg.AIDirectorModel),
		Endpoint:      strings.TrimSpace(p.cfg.AIDirectorEndpoint),
		APIKey:        strings.TrimSpace(p.cfg.AIDirectorAPIKey),
		PromptVersion: strings.TrimSpace(p.cfg.AIDirectorPromptVersion),
		Timeout:       time.Duration(timeoutSec) * time.Second,
		MaxTokens:     maxTokens,
	}
}

func (p *Processor) loadGIFAIJudgeConfig() aiModelCallConfig {
	timeoutSec := p.cfg.AIJudgeTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	maxTokens := p.cfg.AIJudgeMaxTokens
	if maxTokens <= 0 {
		maxTokens = 1400
	}
	return aiModelCallConfig{
		Enabled:       p.cfg.AIJudgeEnabled,
		Provider:      strings.ToLower(strings.TrimSpace(p.cfg.AIJudgeProvider)),
		Model:         strings.TrimSpace(p.cfg.AIJudgeModel),
		Endpoint:      strings.TrimSpace(p.cfg.AIJudgeEndpoint),
		APIKey:        strings.TrimSpace(p.cfg.AIJudgeAPIKey),
		PromptVersion: strings.TrimSpace(p.cfg.AIJudgePromptVersion),
		Timeout:       time.Duration(timeoutSec) * time.Second,
		MaxTokens:     maxTokens,
	}
}

func (p *Processor) callOpenAICompatJSONChat(
	ctx context.Context,
	cfg aiModelCallConfig,
	systemPrompt string,
	userPrompt string,
) (string, cloudHighlightUsage, map[string]interface{}, int64, error) {
	if !cfg.Enabled {
		return "", cloudHighlightUsage{}, nil, 0, fmt.Errorf("ai call disabled")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", cloudHighlightUsage{}, nil, 0, fmt.Errorf("missing api key")
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return "", cloudHighlightUsage{}, nil, 0, fmt.Errorf("missing endpoint")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return "", cloudHighlightUsage{}, nil, 0, fmt.Errorf("missing model")
	}

	reqPayload := openAICompatChatRequest{
		Model:       cfg.Model,
		MaxTokens:   cfg.MaxTokens,
		Temperature: 0.1,
		Messages: []openAICompatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
	body, _ := json.Marshal(reqPayload)

	endpoint := strings.TrimRight(cfg.Endpoint, "/") + "/v1/chat/completions"
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", cloudHighlightUsage{}, nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := p.httpClient
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	} else {
		client = &http.Client{
			Transport: client.Transport,
			Timeout:   cfg.Timeout,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", cloudHighlightUsage{}, nil, clampDurationMillis(started), err
	}
	defer resp.Body.Close()
	rawResp, readErr := io.ReadAll(io.LimitReader(resp.Body, openAICompatMaxRespBytes))
	durationMs := clampDurationMillis(started)
	if readErr != nil {
		return "", cloudHighlightUsage{}, nil, durationMs, fmt.Errorf("read response: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", cloudHighlightUsage{}, nil, durationMs, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(rawResp)))
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(rawResp, &payload); err != nil {
		return "", cloudHighlightUsage{}, nil, durationMs, fmt.Errorf("decode response: %w", err)
	}
	content := extractOpenAICompatMessageContent(payload)
	if strings.TrimSpace(content) == "" {
		return "", cloudHighlightUsage{}, payload, durationMs, fmt.Errorf("empty content in model response")
	}

	usage := extractUsageFromOpenAICompat(payload)
	return content, usage, payload, durationMs, nil
}

func extractOpenAICompatMessageContent(raw map[string]interface{}) string {
	choices, ok := raw["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return ""
	}
	choice := mapFromAny(choices[0])
	message := mapFromAny(choice["message"])
	contentRaw, ok := message["content"]
	if !ok {
		return ""
	}
	switch value := contentRaw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			part := mapFromAny(item)
			text := strings.TrimSpace(stringFromAny(part["text"]))
			if text == "" {
				continue
			}
			parts = append(parts, text)
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func extractUsageFromOpenAICompat(raw map[string]interface{}) cloudHighlightUsage {
	usageMap := mapFromAny(raw["usage"])
	usage := cloudHighlightUsage{
		InputTokens:       int64(floatFromAny(usageMap["prompt_tokens"])),
		OutputTokens:      int64(floatFromAny(usageMap["completion_tokens"])),
		CachedInputTokens: int64(floatFromAny(usageMap["cached_tokens"])),
		ImageTokens:       int64(floatFromAny(usageMap["image_tokens"])),
		VideoTokens:       int64(floatFromAny(usageMap["video_tokens"])),
		AudioSeconds:      floatFromAny(usageMap["audio_seconds"]),
	}
	if usage.InputTokens <= 0 {
		usage.InputTokens = int64(floatFromAny(usageMap["input_tokens"]))
	}
	if usage.OutputTokens <= 0 {
		usage.OutputTokens = int64(floatFromAny(usageMap["output_tokens"]))
	}
	if usage.CachedInputTokens <= 0 {
		usage.CachedInputTokens = int64(floatFromAny(usageMap["prompt_cache_hit_tokens"]))
	}
	if usage.CachedInputTokens <= 0 {
		details := mapFromAny(usageMap["prompt_tokens_details"])
		usage.CachedInputTokens = int64(floatFromAny(details["cached_tokens"]))
	}
	return normalizeCloudHighlightUsage(usage)
}

func sanitizeModelJSON(raw string) string {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSpace(text)
	}
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSpace(text)
	}
	if strings.HasSuffix(text, "```") {
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	return text
}

func (p *Processor) requestAIGIFPromptDirective(
	ctx context.Context,
	job models.VideoJob,
	meta videoProbeMeta,
	local highlightSuggestion,
	qualitySettings QualitySettings,
) (*gifAIDirectiveProfile, map[string]interface{}, error) {
	cfg := p.loadGIFAIDirectorConfig()
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	operatorInstruction := strings.TrimSpace(qualitySettings.AIDirectorOperatorInstruction)
	operatorVersion := strings.TrimSpace(qualitySettings.AIDirectorOperatorInstructionVersion)
	if operatorVersion == "" {
		operatorVersion = "v1"
	}
	operatorEnabled := qualitySettings.AIDirectorOperatorEnabled

	info := map[string]interface{}{
		"enabled":                      cfg.Enabled,
		"provider":                     cfg.Provider,
		"model":                        cfg.Model,
		"prompt_version":               cfg.PromptVersion,
		"operator_instruction_enabled": operatorEnabled,
		"operator_instruction_version": operatorVersion,
		"operator_instruction_len":     len(operatorInstruction),
	}
	if !cfg.Enabled {
		info["applied"] = false
		return nil, info, fmt.Errorf("director disabled")
	}
	if !containsString(normalizeOutputFormats(job.OutputFormats), "gif") {
		info["applied"] = false
		return nil, info, fmt.Errorf("non-gif job")
	}
	localTop := local.Candidates
	if len(localTop) > 8 {
		localTop = localTop[:8]
	}
	payload := map[string]interface{}{
		"job_id":                job.ID,
		"title":                 strings.TrimSpace(job.Title),
		"duration_sec":          roundTo(meta.DurationSec, 3),
		"width":                 meta.Width,
		"height":                meta.Height,
		"fps":                   roundTo(meta.FPS, 3),
		"gif_profile":           strings.ToLower(strings.TrimSpace(qualitySettings.GIFProfile)),
		"gif_target_size_kb":    qualitySettings.GIFTargetSizeKB,
		"candidate_top_windows": localTop,
		"operator_instruction": map[string]interface{}{
			"enabled": operatorEnabled,
			"version": operatorVersion,
			"text":    operatorInstruction,
		},
	}
	userBytes, _ := json.Marshal(payload)

	systemPrompt := `你是视频转GIF任务的“需求甲方（Prompt Director）”。
你的职责是：在正式剪辑方案生成前，给出结构化任务指令，指导后续Planner更稳定地产出高价值GIF。
仅返回JSON（不要markdown）：
{
  "directive": {
    "business_goal": "entertainment|news|design_asset|social_spread",
    "audience": "简短描述",
    "must_capture": ["必须抓取的瞬间/特征"],
    "avoid": ["应避免片段/质量风险"],
    "clip_count_min": 3,
    "clip_count_max": 8,
    "duration_pref_min_sec": 1.4,
    "duration_pref_max_sec": 3.2,
    "loop_preference": 0.0,
    "style_direction": "画风/节奏方向（简短）",
    "risk_flags": ["low_light","fast_motion","noise_audio"],
    "quality_weights": {"semantic":0.35,"clarity":0.20,"loop":0.25,"efficiency":0.20},
    "directive_text": "给Planner的自然语言摘要，50~120字"
  }
}
约束：
1) loop_preference、quality_weights 各值在 [0,1]；
2) quality_weights 的和应接近 1；
3) clip_count_min>=1 且 clip_count_max>=clip_count_min；
4) duration_pref_max_sec > duration_pref_min_sec。`
	if operatorEnabled && operatorInstruction != "" {
		systemPrompt += "\n\n额外的运营指令模板（必须严格遵守，冲突时优先级高于默认偏好）：" +
			"\n版本：" + operatorVersion +
			"\n" + operatorInstruction
	}

	buildPersistContext := func(status string, fallbackUsed bool, raw map[string]interface{}, fallbackReason string) aiGIFDirectivePersistContext {
		return aiGIFDirectivePersistContext{
			Status:              status,
			FallbackUsed:        fallbackUsed,
			BriefVersion:        resolveAIDirectiveBriefVersion(operatorVersion, cfg.PromptVersion),
			ModelVersion:        resolveAIDirectiveModelVersion(cfg.Model, raw),
			InputContext:        payload,
			OperatorEnabled:     operatorEnabled,
			OperatorInstruction: operatorInstruction,
			OperatorVersion:     operatorVersion,
			FallbackReason:      fallbackReason,
		}
	}

	modelText, usage, rawResp, durationMs, err := p.callOpenAICompatJSONChat(ctx, cfg, systemPrompt, string(userBytes))
	status := "ok"
	errText := ""
	if err != nil {
		status = "error"
		errText = err.Error()
	}
	p.recordVideoJobAIUsage(videoJobAIUsageInput{
		JobID:             job.ID,
		UserID:            job.UserID,
		Stage:             gifAIDirectorStage,
		Provider:          cfg.Provider,
		Model:             cfg.Model,
		Endpoint:          cfg.Endpoint,
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CachedInputTokens: usage.CachedInputTokens,
		ImageTokens:       usage.ImageTokens,
		VideoTokens:       usage.VideoTokens,
		AudioSeconds:      usage.AudioSeconds,
		RequestDurationMs: durationMs,
		RequestStatus:     status,
		RequestError:      errText,
		Metadata: map[string]interface{}{
			"prompt_version":               cfg.PromptVersion,
			"local_candidate_count":        len(local.Candidates),
			"operator_instruction_enabled": operatorEnabled,
			"operator_instruction_version": operatorVersion,
			"operator_instruction_len":     len(operatorInstruction),
		},
	})
	if err != nil {
		fallbackDirective := buildFallbackAIGIFDirective(local, qualitySettings, "director_call_error")
		_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *fallbackDirective, rawResp, buildPersistContext("fallback", true, rawResp, "director_call_error"))
		info["applied"] = false
		info["status"] = "fallback"
		info["fallback_used"] = true
		info["error"] = err.Error()
		return nil, info, err
	}

	var parsed gifAIDirectorResponse
	if err := json.Unmarshal([]byte(sanitizeModelJSON(modelText)), &parsed); err != nil {
		fallbackDirective := buildFallbackAIGIFDirective(local, qualitySettings, "director_parse_error")
		_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *fallbackDirective, rawResp, buildPersistContext("fallback", true, rawResp, "director_parse_error"))
		info["applied"] = false
		info["status"] = "fallback"
		info["fallback_used"] = true
		info["error"] = "parse director response: " + err.Error()
		return nil, info, err
	}
	directive := normalizeAIGIFDirective(parsed.Directive, qualitySettings.GIFCandidateMaxOutputs)
	if directive == nil {
		fallbackDirective := buildFallbackAIGIFDirective(local, qualitySettings, "director_output_invalid")
		_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *fallbackDirective, rawResp, buildPersistContext("fallback", true, rawResp, "director_output_invalid"))
		info["applied"] = false
		info["status"] = "fallback"
		info["fallback_used"] = true
		info["error"] = "director output invalid"
		return nil, info, fmt.Errorf("director output invalid")
	}
	_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *directive, rawResp, buildPersistContext("ok", false, rawResp, ""))

	info["applied"] = true
	info["status"] = "ok"
	info["fallback_used"] = false
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
		Metadata: mustJSON(map[string]interface{}{
			"must_capture_count":           len(directive.MustCapture),
			"avoid_count":                  len(directive.Avoid),
			"risk_flags_count":             len(directive.RiskFlags),
			"operator_instruction_enabled": persist.OperatorEnabled,
			"operator_instruction_version": strings.TrimSpace(persist.OperatorVersion),
			"operator_instruction_len":     len(strings.TrimSpace(persist.OperatorInstruction)),
			"fallback_reason":              strings.TrimSpace(persist.FallbackReason),
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

func (p *Processor) requestAIGIFPlannerSuggestion(
	ctx context.Context,
	job models.VideoJob,
	meta videoProbeMeta,
	local highlightSuggestion,
	directive *gifAIDirectiveProfile,
	qualitySettings QualitySettings,
) (highlightSuggestion, map[string]interface{}, error) {
	cfg := p.loadGIFAIPlannerConfig()
	info := map[string]interface{}{
		"enabled":        cfg.Enabled,
		"provider":       cfg.Provider,
		"model":          cfg.Model,
		"prompt_version": cfg.PromptVersion,
	}
	if !cfg.Enabled {
		info["applied"] = false
		return local, info, fmt.Errorf("planner disabled")
	}
	if !containsString(normalizeOutputFormats(job.OutputFormats), "gif") {
		info["applied"] = false
		return local, info, fmt.Errorf("non-gif job")
	}
	if local.Selected == nil || len(local.Candidates) == 0 {
		info["applied"] = false
		return local, info, fmt.Errorf("local suggestion unavailable")
	}

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	targetTopN := qualitySettings.GIFCandidateMaxOutputs
	if targetTopN <= 0 {
		targetTopN = defaultHighlightTopN
	}

	payload := map[string]interface{}{
		"job_id":               job.ID,
		"title":                strings.TrimSpace(job.Title),
		"duration_sec":         roundTo(meta.DurationSec, 3),
		"width":                meta.Width,
		"height":               meta.Height,
		"fps":                  roundTo(meta.FPS, 3),
		"target_top_n":         targetTopN,
		"target_window_sec":    roundTo(chooseHighlightDuration(meta.DurationSec), 3),
		"local_candidates":     local.Candidates,
		"local_all_candidates": local.All,
	}
	if directive != nil {
		payload["director"] = directive
	}
	userBytes, _ := json.Marshal(payload)

	systemPrompt := `你是视频GIF剪辑提名助手。请基于输入候选窗口，给出更贴近“高光、可脱离上下文、可传播、可循环”的提名方案。
必须返回JSON（不要markdown）：
{
  "proposals":[
    {
      "proposal_rank":1,
      "start_sec":12.3,
      "end_sec":14.8,
      "score":0.86,
      "proposal_reason":"表情爆发点+动作完成点",
      "semantic_tags":["emotion_peak","reaction"],
      "expected_value_level":"high",
      "standalone_confidence":0.88,
      "loop_friendliness_hint":0.72
    }
  ],
  "selected_rank":1,
  "notes":"可选，简短"
}
约束：
1) 时间窗口必须在视频时长范围内，end_sec > start_sec，单窗口建议 1.0~5.0 秒；
2) proposals 按质量从高到低，最多 20 条；
3) score/standalone_confidence/loop_friendliness_hint 均在 [0,1]；
4) 若输入包含 director，请优先遵循 director 的目标和偏好。`

	modelText, usage, rawResp, durationMs, err := p.callOpenAICompatJSONChat(ctx, cfg, systemPrompt, string(userBytes))
	status := "ok"
	errText := ""
	if err != nil {
		status = "error"
		errText = err.Error()
	}
	p.recordVideoJobAIUsage(videoJobAIUsageInput{
		JobID:             job.ID,
		UserID:            job.UserID,
		Stage:             gifAIPlannerStage,
		Provider:          cfg.Provider,
		Model:             cfg.Model,
		Endpoint:          cfg.Endpoint,
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CachedInputTokens: usage.CachedInputTokens,
		ImageTokens:       usage.ImageTokens,
		VideoTokens:       usage.VideoTokens,
		AudioSeconds:      usage.AudioSeconds,
		RequestDurationMs: durationMs,
		RequestStatus:     status,
		RequestError:      errText,
		Metadata: map[string]interface{}{
			"prompt_version":        cfg.PromptVersion,
			"target_top_n":          targetTopN,
			"local_candidate_count": len(local.Candidates),
			"director_applied":      directive != nil,
		},
	})
	if err != nil {
		info["applied"] = false
		info["error"] = err.Error()
		return local, info, err
	}

	var parsed gifAIPlannerResponse
	if err := json.Unmarshal([]byte(sanitizeModelJSON(modelText)), &parsed); err != nil {
		info["applied"] = false
		info["error"] = "parse planner response: " + err.Error()
		return local, info, err
	}

	proposals := normalizeAIGIFPlannerProposals(parsed.Proposals, meta.DurationSec)
	if len(proposals) == 0 {
		info["applied"] = false
		info["error"] = "planner produced empty proposals"
		return local, info, fmt.Errorf("planner produced empty proposals")
	}
	candidates := make([]highlightCandidate, 0, len(proposals))
	for _, item := range proposals {
		candidates = append(candidates, highlightCandidate{
			StartSec:     roundTo(item.StartSec, 3),
			EndSec:       roundTo(item.EndSec, 3),
			Score:        roundTo(item.Score, 4),
			SceneScore:   roundTo(item.StandaloneConfidence, 4),
			Reason:       strings.TrimSpace(item.ProposalReason),
			ProposalRank: item.ProposalRank,
		})
	}
	selected := pickNonOverlapCandidates(candidates, targetTopN, qualitySettings.GIFCandidateDedupIOUThreshold)
	selected = applyGIFCandidateConfidenceThreshold(selected, candidates, qualitySettings.GIFCandidateConfidenceThreshold)
	if len(selected) == 0 {
		selected = candidates
	}
	if len(selected) > targetTopN {
		selected = selected[:targetTopN]
	}
	suggestion := highlightSuggestion{
		Version:    "ai_planner_v1",
		Strategy:   "ai_semantic_planner",
		Selected:   &selected[0],
		Candidates: selected,
		All:        candidates,
	}
	proposalIDByRank, _ := p.persistAIGIFProposals(job.ID, job.UserID, cfg, proposals, suggestion, rawResp)
	if len(proposalIDByRank) > 0 {
		for idx := range suggestion.Candidates {
			rank := suggestion.Candidates[idx].ProposalRank
			if rank <= 0 {
				continue
			}
			if proposalID, ok := proposalIDByRank[rank]; ok && proposalID > 0 {
				proposalID := proposalID
				suggestion.Candidates[idx].ProposalID = &proposalID
			}
		}
		for idx := range suggestion.All {
			rank := suggestion.All[idx].ProposalRank
			if rank <= 0 {
				continue
			}
			if proposalID, ok := proposalIDByRank[rank]; ok && proposalID > 0 {
				proposalID := proposalID
				suggestion.All[idx].ProposalID = &proposalID
			}
		}
		if len(suggestion.Candidates) > 0 {
			suggestion.Selected = &suggestion.Candidates[0]
		}
	}

	info["applied"] = true
	info["candidate_count"] = len(proposals)
	info["selected_count"] = len(selected)
	info["selected_start_sec"] = suggestion.Selected.StartSec
	info["selected_end_sec"] = suggestion.Selected.EndSec
	info["selected_score"] = suggestion.Selected.Score
	info["director_applied"] = directive != nil
	return suggestion, info, nil
}

func normalizeAIGIFPlannerProposals(in []gifAIPlannerProposal, durationSec float64) []gifAIPlannerProposal {
	if len(in) == 0 {
		return nil
	}
	out := make([]gifAIPlannerProposal, 0, len(in))
	for idx, item := range in {
		start, end := clampHighlightWindow(item.StartSec, item.EndSec, durationSec)
		if end-start < 0.8 {
			continue
		}
		row := item
		row.StartSec = start
		row.EndSec = end
		if row.ProposalRank <= 0 {
			row.ProposalRank = idx + 1
		}
		row.Score = clampZeroOne(row.Score)
		row.StandaloneConfidence = clampZeroOne(row.StandaloneConfidence)
		row.LoopFriendlinessHint = clampZeroOne(row.LoopFriendlinessHint)
		if strings.TrimSpace(row.ProposalReason) == "" {
			row.ProposalReason = "ai_proposal"
		}
		row.ExpectedValueLevel = strings.ToLower(strings.TrimSpace(row.ExpectedValueLevel))
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProposalRank == out[j].ProposalRank {
			if out[i].Score == out[j].Score {
				return out[i].StartSec < out[j].StartSec
			}
			return out[i].Score > out[j].Score
		}
		return out[i].ProposalRank < out[j].ProposalRank
	})
	dedup := make([]gifAIPlannerProposal, 0, len(out))
	seenRank := map[int]struct{}{}
	for _, item := range out {
		if item.ProposalRank <= 0 {
			continue
		}
		if _, exists := seenRank[item.ProposalRank]; exists {
			continue
		}
		seenRank[item.ProposalRank] = struct{}{}
		dedup = append(dedup, item)
	}
	return dedup
}

func (p *Processor) persistAIGIFProposals(
	jobID uint64,
	userID uint64,
	cfg aiModelCallConfig,
	proposals []gifAIPlannerProposal,
	selected highlightSuggestion,
	raw map[string]interface{},
) (map[int]uint64, error) {
	proposalIDByRank := map[int]uint64{}
	if p == nil || p.db == nil || jobID == 0 || userID == 0 {
		return proposalIDByRank, nil
	}
	if len(proposals) == 0 {
		return proposalIDByRank, nil
	}
	selectedMap := map[string]struct{}{}
	for _, candidate := range selected.Candidates {
		key := highlightCandidateWindowKey(candidate)
		if key == "" {
			continue
		}
		selectedMap[key] = struct{}{}
	}
	rows := make([]models.VideoJobGIFAIProposal, 0, len(proposals))
	for _, item := range proposals {
		candidate := highlightCandidate{
			StartSec: item.StartSec,
			EndSec:   item.EndSec,
			Score:    item.Score,
		}
		status := "proposed"
		if _, ok := selectedMap[highlightCandidateWindowKey(candidate)]; ok {
			status = "selected"
		}
		durationSec := item.EndSec - item.StartSec
		if durationSec < 0 {
			durationSec = 0
		}
		rows = append(rows, models.VideoJobGIFAIProposal{
			JobID:                jobID,
			UserID:               userID,
			Provider:             cfg.Provider,
			Model:                cfg.Model,
			Endpoint:             cfg.Endpoint,
			PromptVersion:        cfg.PromptVersion,
			ProposalRank:         item.ProposalRank,
			StartSec:             roundTo(item.StartSec, 3),
			EndSec:               roundTo(item.EndSec, 3),
			DurationSec:          roundTo(durationSec, 3),
			BaseScore:            roundTo(item.Score, 4),
			ProposalReason:       strings.TrimSpace(item.ProposalReason),
			SemanticTags:         mustJSON(item.SemanticTags),
			ExpectedValueLevel:   strings.TrimSpace(item.ExpectedValueLevel),
			StandaloneConfidence: roundTo(item.StandaloneConfidence, 4),
			LoopFriendlinessHint: roundTo(item.LoopFriendlinessHint, 4),
			Status:               status,
			Metadata: mustJSON(map[string]interface{}{
				"selected": status == "selected",
			}),
			RawResponse: mustJSON(raw),
		})
	}
	err := p.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("job_id = ?", jobID).Delete(&models.VideoJobGIFAIProposal{}).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(rows, 100).Error; err != nil {
			return err
		}
		type createdRow struct {
			ID           uint64 `gorm:"column:id"`
			ProposalRank int    `gorm:"column:proposal_rank"`
		}
		var created []createdRow
		if err := tx.Model(&models.VideoJobGIFAIProposal{}).
			Select("id", "proposal_rank").
			Where("job_id = ? AND proposal_rank > 0", jobID).
			Order("proposal_rank ASC, id ASC").
			Find(&created).Error; err != nil {
			return err
		}
		for _, row := range created {
			if row.ProposalRank <= 0 || row.ID == 0 {
				continue
			}
			if _, exists := proposalIDByRank[row.ProposalRank]; exists {
				continue
			}
			proposalIDByRank[row.ProposalRank] = row.ID
		}
		return nil
	})
	if err != nil && isMissingTableError(err, "video_job_gif_ai_proposals") {
		return proposalIDByRank, nil
	}
	return proposalIDByRank, err
}

func (p *Processor) runAIGIFJudgeReview(ctx context.Context, job models.VideoJob, qualitySettings QualitySettings) (map[string]interface{}, error) {
	cfg := p.loadGIFAIJudgeConfig()
	result := map[string]interface{}{
		"enabled":        cfg.Enabled,
		"provider":       cfg.Provider,
		"model":          cfg.Model,
		"prompt_version": cfg.PromptVersion,
	}
	if !cfg.Enabled {
		result["applied"] = false
		return result, fmt.Errorf("judge disabled")
	}

	samples, err := p.loadGIFJudgeSamples(job.ID)
	if err != nil {
		result["applied"] = false
		return result, err
	}
	if len(samples) == 0 {
		result["applied"] = false
		return result, fmt.Errorf("no gif outputs for judge")
	}

	input := map[string]interface{}{
		"job_id":      job.ID,
		"sample_size": len(samples),
		"outputs":     samples,
	}
	userPayload, _ := json.Marshal(input)
	systemPrompt := `你是GIF语义复审评委。请根据每个GIF样本的技术评分与上下文，输出可执行的最终建议。
仅返回JSON（不要markdown）：
{
  "reviews":[
    {
      "output_id":123,
      "proposal_rank":1,
      "final_recommendation":"deliver|keep_internal|reject|need_manual_review",
      "semantic_verdict":0.82,
      "diagnostic_reason":"简短原因",
      "suggested_action":"简短建议"
    }
  ],
  "summary":{"note":"可选"}
}
要求：
1) reviews 里的 output_id 必须来自输入；
2) final_recommendation 仅允许四个枚举值；
3) semantic_verdict 在 [0,1]。`

	modelText, usage, rawResp, durationMs, callErr := p.callOpenAICompatJSONChat(ctx, cfg, systemPrompt, string(userPayload))
	status := "ok"
	errText := ""
	if callErr != nil {
		status = "error"
		errText = callErr.Error()
	}
	p.recordVideoJobAIUsage(videoJobAIUsageInput{
		JobID:             job.ID,
		UserID:            job.UserID,
		Stage:             gifAIJudgeStage,
		Provider:          cfg.Provider,
		Model:             cfg.Model,
		Endpoint:          cfg.Endpoint,
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CachedInputTokens: usage.CachedInputTokens,
		ImageTokens:       usage.ImageTokens,
		VideoTokens:       usage.VideoTokens,
		AudioSeconds:      usage.AudioSeconds,
		RequestDurationMs: durationMs,
		RequestStatus:     status,
		RequestError:      errText,
		Metadata: map[string]interface{}{
			"prompt_version": cfg.PromptVersion,
			"sample_size":    len(samples),
		},
	})
	if callErr != nil {
		result["applied"] = false
		result["error"] = callErr.Error()
		return result, callErr
	}

	var parsed gifAIJudgeResponse
	if err := json.Unmarshal([]byte(sanitizeModelJSON(modelText)), &parsed); err != nil {
		result["applied"] = false
		result["error"] = "parse judge response: " + err.Error()
		return result, err
	}
	validReviews, _ := normalizeAIGIFJudgeReviews(parsed.Reviews, samples)
	if len(validReviews) == 0 {
		result["applied"] = false
		result["error"] = "judge produced empty valid reviews"
		return result, fmt.Errorf("judge produced empty valid reviews")
	}

	gatedReviews, hardGateByOutput, hardGateStats := applyAIGIFTechnicalHardGates(validReviews, samples, qualitySettings)
	if err := p.persistAIGIFReviews(job, cfg, samples, gatedReviews, hardGateByOutput, rawResp); err != nil {
		result["applied"] = false
		result["error"] = err.Error()
		return result, err
	}

	counts := countAIGIFJudgeRecommendations(gatedReviews)
	result["applied"] = true
	result["reviewed_outputs"] = len(gatedReviews)
	for key, value := range counts {
		result[key] = value
	}
	result["hard_gate_applied"] = hardGateStats.Applied
	result["hard_gate_reject_count"] = hardGateStats.RejectCount
	result["hard_gate_manual_review_count"] = hardGateStats.ManualReviewCount
	result["summary"] = parsed.Summary
	return result, nil
}

func normalizeAIGIFJudgeReviews(reviews []gifAIJudgeReviewRow, samples []gifJudgeSample) ([]gifAIJudgeReviewRow, map[string]int) {
	allowedOutput := map[uint64]struct{}{}
	for _, item := range samples {
		allowedOutput[item.OutputID] = struct{}{}
	}
	out := make([]gifAIJudgeReviewRow, 0, len(reviews))
	seen := map[uint64]struct{}{}
	counts := map[string]int{
		"deliver_count":       0,
		"keep_internal_count": 0,
		"reject_count":        0,
		"manual_review_count": 0,
	}
	for _, item := range reviews {
		if item.OutputID == 0 {
			continue
		}
		if _, ok := allowedOutput[item.OutputID]; !ok {
			continue
		}
		if _, exists := seen[item.OutputID]; exists {
			continue
		}
		recommendation := normalizeGIFAIReviewRecommendation(item.FinalRecommendation)
		if recommendation == "" {
			continue
		}
		item.FinalRecommendation = recommendation
		item.SemanticVerdict = clampZeroOne(item.SemanticVerdict)
		item.DiagnosticReason = strings.TrimSpace(item.DiagnosticReason)
		item.SuggestedAction = strings.TrimSpace(item.SuggestedAction)
		out = append(out, item)
		seen[item.OutputID] = struct{}{}
		switch recommendation {
		case "deliver":
			counts["deliver_count"]++
		case "keep_internal":
			counts["keep_internal_count"]++
		case "reject":
			counts["reject_count"]++
		case "need_manual_review":
			counts["manual_review_count"]++
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].OutputID < out[j].OutputID
	})
	return out, counts
}

func countAIGIFJudgeRecommendations(reviews []gifAIJudgeReviewRow) map[string]int {
	counts := map[string]int{
		"deliver_count":       0,
		"keep_internal_count": 0,
		"reject_count":        0,
		"manual_review_count": 0,
	}
	for _, item := range reviews {
		switch normalizeGIFAIReviewRecommendation(item.FinalRecommendation) {
		case "deliver":
			counts["deliver_count"]++
		case "keep_internal":
			counts["keep_internal_count"]++
		case "reject":
			counts["reject_count"]++
		case "need_manual_review":
			counts["manual_review_count"]++
		}
	}
	return counts
}

type gifAIJudgeHardGateVerdict struct {
	Applied            bool
	Blocked            bool
	FromRecommendation string
	ToRecommendation   string
	ReasonCodes        []string
}

type gifAIJudgeHardGateStats struct {
	Applied           int
	RejectCount       int
	ManualReviewCount int
}

func applyAIGIFTechnicalHardGates(
	reviews []gifAIJudgeReviewRow,
	samples []gifJudgeSample,
	qualitySettings QualitySettings,
) ([]gifAIJudgeReviewRow, map[uint64]gifAIJudgeHardGateVerdict, gifAIJudgeHardGateStats) {
	if len(reviews) == 0 {
		return reviews, nil, gifAIJudgeHardGateStats{}
	}
	sampleMap := make(map[uint64]gifJudgeSample, len(samples))
	for _, sample := range samples {
		sampleMap[sample.OutputID] = sample
	}
	out := make([]gifAIJudgeReviewRow, 0, len(reviews))
	verdicts := make(map[uint64]gifAIJudgeHardGateVerdict, len(reviews))
	stats := gifAIJudgeHardGateStats{}

	for _, item := range reviews {
		sample, ok := sampleMap[item.OutputID]
		if !ok {
			out = append(out, item)
			continue
		}
		v := evaluateAIGIFTechnicalHardGate(sample, normalizeGIFAIReviewRecommendation(item.FinalRecommendation), qualitySettings)
		if v.Applied {
			item.FinalRecommendation = v.ToRecommendation
			if strings.TrimSpace(item.DiagnosticReason) == "" {
				item.DiagnosticReason = "技术硬规则闸门生效：" + strings.Join(v.ReasonCodes, ",")
			} else {
				item.DiagnosticReason = strings.TrimSpace(item.DiagnosticReason) + "；技术硬规则闸门：" + strings.Join(v.ReasonCodes, ",")
			}
			stats.Applied++
			switch v.ToRecommendation {
			case "reject":
				stats.RejectCount++
			case "need_manual_review":
				stats.ManualReviewCount++
			}
		}
		verdicts[item.OutputID] = v
		out = append(out, item)
	}
	return out, verdicts, stats
}

func evaluateAIGIFTechnicalHardGate(sample gifJudgeSample, recommendation string, qualitySettings QualitySettings) gifAIJudgeHardGateVerdict {
	v := gifAIJudgeHardGateVerdict{
		Applied:            false,
		Blocked:            false,
		FromRecommendation: normalizeGIFAIReviewRecommendation(recommendation),
		ToRecommendation:   normalizeGIFAIReviewRecommendation(recommendation),
		ReasonCodes:        nil,
	}
	if v.FromRecommendation == "" {
		v.FromRecommendation = "need_manual_review"
		v.ToRecommendation = "need_manual_review"
	}
	if v.FromRecommendation != "deliver" {
		return v
	}

	reasonSet := map[string]struct{}{}
	addReason := func(code string) {
		code = strings.TrimSpace(code)
		if code == "" {
			return
		}
		reasonSet[code] = struct{}{}
	}

	if sample.SizeBytes <= 0 || sample.Width <= 0 || sample.Height <= 0 || sample.DurationMs <= 0 {
		addReason("invalid_output")
	}
	if sample.DurationMs > 0 && sample.DurationMs < gifAIJudgeHardGateMinDurationMS {
		addReason("duration_too_short")
	}
	if sample.Score > 0 && sample.Score < gifAIJudgeHardGateMinOutputScore {
		addReason("output_score_low")
	}

	evalMissing := sample.EvalOverall <= 0 && sample.EvalClarity <= 0 && sample.EvalLoop <= 0 && sample.EvalMotion <= 0
	if evalMissing {
		addReason("evaluation_missing")
	}
	if sample.EvalOverall > 0 && sample.EvalOverall < gifAIJudgeHardGateMinOverallScore {
		addReason("overall_low")
	}
	if sample.EvalClarity > 0 && sample.EvalClarity < gifAIJudgeHardGateMinClarityScore {
		addReason("clarity_low")
	}
	if sample.EvalLoop > 0 && sample.EvalLoop < gifAIJudgeHardGateMinLoopScore {
		addReason("loop_low")
	}

	targetKB := qualitySettings.GIFTargetSizeKB
	if targetKB <= 0 {
		targetKB = DefaultQualitySettings().GIFTargetSizeKB
	}
	hardMaxBytes := int64(targetKB) * 1024 * gifAIJudgeHardGateSizeMultiplier
	if hardMaxBytes > 0 && sample.SizeBytes > hardMaxBytes {
		addReason("size_hard_exceeded")
	}

	if len(reasonSet) == 0 {
		return v
	}

	reasons := make([]string, 0, len(reasonSet))
	for code := range reasonSet {
		reasons = append(reasons, code)
	}
	sort.Strings(reasons)

	v.Applied = true
	v.Blocked = true
	v.ReasonCodes = reasons
	if containsString(reasons, "evaluation_missing") {
		v.ToRecommendation = "need_manual_review"
	} else {
		v.ToRecommendation = "reject"
	}
	return v
}

func normalizeGIFAIReviewRecommendation(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "deliver":
		return "deliver"
	case "keep_internal":
		return "keep_internal"
	case "reject":
		return "reject"
	case "need_manual_review":
		return "need_manual_review"
	default:
		return ""
	}
}

func (p *Processor) loadGIFJudgeSamples(jobID uint64) ([]gifJudgeSample, error) {
	if p == nil || p.db == nil || jobID == 0 {
		return nil, nil
	}
	var outputs []models.VideoImageOutputPublic
	if err := p.db.Where("job_id = ? AND format = ? AND file_role = ?", jobID, "gif", "main").
		Order("is_primary DESC, score DESC, id ASC").
		Limit(24).
		Find(&outputs).Error; err != nil {
		return nil, err
	}
	if len(outputs) == 0 {
		return nil, nil
	}
	outputIDs := make([]uint64, 0, len(outputs))
	for _, item := range outputs {
		outputIDs = append(outputIDs, item.ID)
	}

	var evalRows []models.VideoJobGIFEvaluation
	if err := p.db.Where("job_id = ? AND output_id IN ?", jobID, outputIDs).Find(&evalRows).Error; err != nil {
		if !isMissingTableError(err, "video_job_gif_evaluations") {
			return nil, err
		}
		evalRows = nil
	}
	evalMap := map[uint64]models.VideoJobGIFEvaluation{}
	for _, item := range evalRows {
		if item.OutputID == nil || *item.OutputID == 0 {
			continue
		}
		evalMap[*item.OutputID] = item
	}

	samples := make([]gifJudgeSample, 0, len(outputs))
	for _, output := range outputs {
		outputMeta := parseJSONMap(output.Metadata)
		startSec, endSec, reason := parseFeedbackOutputWindowContext(output.Metadata)
		eval := evalMap[output.ID]
		proposalID := output.ProposalID
		if (proposalID == nil || *proposalID == 0) && outputMeta != nil {
			if resolved := uint64(toFloatFromAny(outputMeta["proposal_id"])); resolved > 0 {
				resolved := resolved
				proposalID = &resolved
			}
		}
		proposalRank := int(toFloatFromAny(outputMeta["proposal_rank"]))
		samples = append(samples, gifJudgeSample{
			OutputID:        output.ID,
			Score:           roundTo(output.Score, 4),
			SizeBytes:       output.SizeBytes,
			Width:           output.Width,
			Height:          output.Height,
			DurationMs:      output.DurationMs,
			StartSec:        roundTo(startSec, 3),
			EndSec:          roundTo(endSec, 3),
			Reason:          reason,
			EvalOverall:     roundTo(eval.OverallScore, 4),
			EvalEmotion:     roundTo(eval.EmotionScore, 4),
			EvalClarity:     roundTo(eval.ClarityScore, 4),
			EvalMotion:      roundTo(eval.MotionScore, 4),
			EvalLoop:        roundTo(eval.LoopScore, 4),
			EvalEfficiency:  roundTo(eval.EfficiencyScore, 4),
			ProposalIDByWin: proposalID,
			ProposalRank:    proposalRank,
		})
	}
	return samples, nil
}

func (p *Processor) persistAIGIFReviews(
	job models.VideoJob,
	cfg aiModelCallConfig,
	samples []gifJudgeSample,
	reviews []gifAIJudgeReviewRow,
	hardGateByOutput map[uint64]gifAIJudgeHardGateVerdict,
	raw map[string]interface{},
) error {
	if p == nil || p.db == nil || job.ID == 0 || job.UserID == 0 || len(reviews) == 0 {
		return nil
	}
	sampleMap := map[uint64]gifJudgeSample{}
	for _, item := range samples {
		sampleMap[item.OutputID] = item
	}
	rows := make([]models.VideoJobGIFAIReview, 0, len(reviews))
	for _, item := range reviews {
		outputID := item.OutputID
		sample := sampleMap[outputID]
		proposalID := sample.ProposalIDByWin
		if (proposalID == nil || *proposalID == 0) && sample.ProposalRank > 0 {
			if proposal, err := p.loadAIGIFProposalByRank(job.ID, sample.ProposalRank); err == nil && proposal != nil {
				id := proposal.ID
				proposalID = &id
			}
		}
		if item.ProposalRank > 0 {
			if proposal, err := p.loadAIGIFProposalByRank(job.ID, item.ProposalRank); err == nil && proposal != nil {
				id := proposal.ID
				proposalID = &id
			}
		}
		hardGate := hardGateByOutput[outputID]
		row := models.VideoJobGIFAIReview{
			JobID:               job.ID,
			UserID:              job.UserID,
			OutputID:            &outputID,
			ProposalID:          proposalID,
			Provider:            cfg.Provider,
			Model:               cfg.Model,
			Endpoint:            cfg.Endpoint,
			PromptVersion:       cfg.PromptVersion,
			FinalRecommendation: item.FinalRecommendation,
			SemanticVerdict:     roundTo(item.SemanticVerdict, 4),
			DiagnosticReason:    strings.TrimSpace(item.DiagnosticReason),
			SuggestedAction:     strings.TrimSpace(item.SuggestedAction),
			Metadata: mustJSON(map[string]interface{}{
				"proposal_rank":            item.ProposalRank,
				"output_score":             sample.Score,
				"eval_overall":             sample.EvalOverall,
				"eval_loop":                sample.EvalLoop,
				"eval_clarity":             sample.EvalClarity,
				"window_start":             sample.StartSec,
				"window_end":               sample.EndSec,
				"hard_gate_applied":        hardGate.Applied,
				"hard_gate_blocked":        hardGate.Blocked,
				"hard_gate_from":           hardGate.FromRecommendation,
				"hard_gate_to":             hardGate.ToRecommendation,
				"hard_gate_reason_codes":   hardGate.ReasonCodes,
				"hard_gate_reason_summary": strings.Join(hardGate.ReasonCodes, ","),
			}),
			RawResponse: mustJSON(raw),
		}
		rows = append(rows, row)
	}
	err := p.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "job_id"}, {Name: "output_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"proposal_id":          gorm.Expr("EXCLUDED.proposal_id"),
			"provider":             gorm.Expr("EXCLUDED.provider"),
			"model":                gorm.Expr("EXCLUDED.model"),
			"endpoint":             gorm.Expr("EXCLUDED.endpoint"),
			"prompt_version":       gorm.Expr("EXCLUDED.prompt_version"),
			"final_recommendation": gorm.Expr("EXCLUDED.final_recommendation"),
			"semantic_verdict":     gorm.Expr("EXCLUDED.semantic_verdict"),
			"diagnostic_reason":    gorm.Expr("EXCLUDED.diagnostic_reason"),
			"suggested_action":     gorm.Expr("EXCLUDED.suggested_action"),
			"metadata":             gorm.Expr("EXCLUDED.metadata"),
			"raw_response":         gorm.Expr("EXCLUDED.raw_response"),
			"updated_at":           time.Now(),
		}),
	}).CreateInBatches(rows, 100).Error
	if err != nil && isMissingTableError(err, "video_job_gif_ai_reviews") {
		return nil
	}
	return err
}

func (p *Processor) loadAIGIFProposalByRank(jobID uint64, rank int) (*models.VideoJobGIFAIProposal, error) {
	if p == nil || p.db == nil || jobID == 0 || rank <= 0 {
		return nil, nil
	}
	var row models.VideoJobGIFAIProposal
	err := p.db.Where("job_id = ? AND proposal_rank = ?", jobID, rank).
		Order("id ASC").
		First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound || isMissingTableError(err, "video_job_gif_ai_proposals") {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func isMissingTableError(err error, table string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, strings.ToLower(strings.TrimSpace(table))) &&
		(strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such table"))
}
