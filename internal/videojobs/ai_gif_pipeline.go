package videojobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

const (
	defaultAIGIFDirectorFixedCorePrompt = `你是视频转GIF任务的“需求甲方（Prompt Director）”。
你的职责是：在正式剪辑方案生成前，给出结构化任务指令，指导后续Planner更稳定地产出高价值GIF。
你不是剪辑执行者，不要直接输出最终窗口 start/end 秒点。
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
}`

	defaultAIGIFDirectorFixedContractTailPrompt = `最终输出契约（必须严格遵守）：
1) 只允许输出 JSON，不允许 markdown，不允许额外解释文字；
2) 最外层必须是 {"directive":{...}}；
3) directive 必须包含字段：
business_goal, audience, must_capture, avoid, clip_count_min, clip_count_max,
duration_pref_min_sec, duration_pref_max_sec, loop_preference, style_direction,
risk_flags, quality_weights, directive_text；
4) loop_preference 与 quality_weights 所有值都必须在 [0,1]；
5) quality_weights 必须包含 semantic, clarity, loop, efficiency 四个键，且总和接近 1；
6) clip_count_min >= 1，clip_count_max >= clip_count_min；
7) duration_pref_max_sec > duration_pref_min_sec；
8) 信息不足时也必须返回合法 JSON，不允许返回空文本或非结构化描述；
9) 运营模板只能影响偏好策略，不能改变输出结构与字段合法性。`

	defaultAIGIFDirectorSystemPrompt = defaultAIGIFDirectorFixedCorePrompt + "\n\n" + defaultAIGIFDirectorFixedContractTailPrompt

	defaultAIGIFPlannerSystemPrompt = `你是视频GIF剪辑提名助手。请基于输入的视频元数据、关键帧与director指令，给出“高光、可脱离上下文、可传播、可循环”的提名方案。
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
4) 若输入包含 director，请优先遵循 director 的目标和偏好；
5) 不要依赖“预先给定候选窗口”，你需要直接从关键帧推断提名窗口。`

	defaultAIGIFJudgeSystemPrompt = `你是GIF语义复审评委。请根据每个GIF样本的技术评分与上下文，输出可执行的最终建议。
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

type openAICompatImageURL struct {
	URL string `json:"url"`
}

type openAICompatVideoURL struct {
	URL string `json:"url"`
}

type openAICompatContentPart struct {
	Type     string                `json:"type"`
	Text     string                `json:"text,omitempty"`
	ImageURL *openAICompatImageURL `json:"image_url,omitempty"`
	VideoURL *openAICompatVideoURL `json:"video_url,omitempty"`
}

type openAICompatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
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

type gifAIDirectorBriefV2Response struct {
	BusinessGoal         string                 `json:"business_goal"`
	Audience             string                 `json:"audience"`
	StyleDirection       string                 `json:"style_direction"`
	MustCapture          []string               `json:"must_capture"`
	Avoid                []string               `json:"avoid"`
	RiskFlags            []string               `json:"risk_flags"`
	QualityWeights       map[string]float64     `json:"quality_weights"`
	PlannerInstruction   string                 `json:"planner_instruction_text"`
	DirectiveText        string                 `json:"directive_text"`
	ClipCountRange       []float64              `json:"clip_count_range"`
	DurationPrefSecRange []float64              `json:"duration_pref_sec"`
	FallbackPolicy       map[string]interface{} `json:"fallback_policy"`
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
	OutputID        uint64  `json:"output_id"`
	IsPrimary       bool    `json:"is_primary"`
	Score           float64 `json:"score"`
	SizeBytes       int64   `json:"size_bytes"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	DurationMs      int     `json:"duration_ms"`
	StartSec        float64 `json:"start_sec"`
	EndSec          float64 `json:"end_sec"`
	Reason          string  `json:"reason"`
	EvalOverall     float64 `json:"eval_overall"`
	EvalEmotion     float64 `json:"eval_emotion"`
	EvalClarity     float64 `json:"eval_clarity"`
	EvalMotion      float64 `json:"eval_motion"`
	EvalLoop        float64 `json:"eval_loop"`
	EvalEfficiency  float64 `json:"eval_efficiency"`
	ProposalIDByWin *uint64 `json:"proposal_id_by_win"`
	ProposalRank    int     `json:"proposal_rank"`
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

type aiPromptTemplateSnapshot struct {
	Found   bool
	Format  string
	Stage   string
	Layer   string
	Text    string
	Version string
	Enabled bool
	Source  string
}

func normalizeAIPromptTemplateFormat(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "gif", "webp", "jpg", "png", "live":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "all"
	}
}

func (p *Processor) loadAIPromptTemplateExact(format, stage, layer string) (models.VideoAIPromptTemplate, bool, error) {
	if p == nil || p.db == nil {
		return models.VideoAIPromptTemplate{}, false, nil
	}
	var row models.VideoAIPromptTemplate
	err := p.db.Where("format = ? AND stage = ? AND layer = ? AND is_active = ?", format, stage, layer, true).
		Order("id DESC").
		Limit(1).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.VideoAIPromptTemplate{}, false, nil
		}
		if isMissingTableError(err, "video_ai_prompt_templates") {
			return models.VideoAIPromptTemplate{}, false, nil
		}
		return models.VideoAIPromptTemplate{}, false, err
	}
	return row, true, nil
}

func (p *Processor) loadAIPromptTemplateWithFallback(format, stage, layer string) (aiPromptTemplateSnapshot, error) {
	normalizedFormat := normalizeAIPromptTemplateFormat(format)
	row, found, err := p.loadAIPromptTemplateExact(normalizedFormat, stage, layer)
	if err != nil {
		return aiPromptTemplateSnapshot{}, err
	}
	if found {
		return aiPromptTemplateSnapshot{
			Found:   true,
			Format:  normalizedFormat,
			Stage:   stage,
			Layer:   layer,
			Text:    strings.TrimSpace(row.TemplateText),
			Version: strings.TrimSpace(row.Version),
			Enabled: row.Enabled,
			Source:  "ops.video_ai_prompt_templates:" + normalizedFormat,
		}, nil
	}
	if normalizedFormat != "all" {
		row, found, err = p.loadAIPromptTemplateExact("all", stage, layer)
		if err != nil {
			return aiPromptTemplateSnapshot{}, err
		}
		if found {
			return aiPromptTemplateSnapshot{
				Found:   true,
				Format:  "all",
				Stage:   stage,
				Layer:   layer,
				Text:    strings.TrimSpace(row.TemplateText),
				Version: strings.TrimSpace(row.Version),
				Enabled: row.Enabled,
				Source:  "ops.video_ai_prompt_templates:all",
			}, nil
		}
	}
	return aiPromptTemplateSnapshot{
		Found:  false,
		Format: normalizedFormat,
		Stage:  stage,
		Layer:  layer,
	}, nil
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
	userParts := []openAICompatContentPart{
		{Type: "text", Text: userPrompt},
	}
	return p.callOpenAICompatJSONChatWithUserParts(ctx, cfg, systemPrompt, userParts)
}

func (p *Processor) callOpenAICompatJSONChatWithUserParts(
	ctx context.Context,
	cfg aiModelCallConfig,
	systemPrompt string,
	userParts []openAICompatContentPart,
) (string, cloudHighlightUsage, map[string]interface{}, int64, error) {
	userContent := interface{}("")
	switch len(userParts) {
	case 0:
		userContent = ""
	case 1:
		if strings.EqualFold(strings.TrimSpace(userParts[0].Type), "text") {
			userContent = userParts[0].Text
		} else {
			userContent = userParts
		}
	default:
		userContent = userParts
	}
	return p.callOpenAICompatJSONChatWithMessages(ctx, cfg, []openAICompatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	})
}

func (p *Processor) callOpenAICompatJSONChatWithMessages(
	ctx context.Context,
	cfg aiModelCallConfig,
	messages []openAICompatMessage,
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
		Messages:    messages,
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

func renderAIDirectorOperatorInstruction(raw string) (string, map[string]interface{}, string) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", nil, "empty"
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(text), &payload); err != nil || len(payload) == 0 {
		return text, nil, "text_passthrough"
	}

	knownKeys := []string{
		"operator_identity",
		"business_scene",
		"asset_goal",
		"delivery_goal",
		"success_definition",
		"must_capture_bias",
		"avoid_bias",
		"cost_mode",
		"candidate_strategy",
		"candidate_count_bias",
		"diversity_preference",
		"clarity_priority",
		"loop_priority",
		"standalone_priority",
		"emotion_priority",
		"risk_tolerance",
		"subtitle_dependency_tolerance",
		"quality_floor_note",
		"user_request_passthrough",
		"extra_business_note",
	}
	hasKnown := false
	for _, key := range knownKeys {
		if _, ok := payload[key]; ok {
			hasKnown = true
			break
		}
	}
	if !hasKnown {
		return text, payload, "json_passthrough"
	}

	strValue := func(key string) string {
		return strings.TrimSpace(stringFromAny(payload[key]))
	}
	listValue := func(key string) []string {
		values := stringSliceFromAny(payload[key])
		out := make([]string, 0, len(values))
		seen := map[string]struct{}{}
		for _, item := range values {
			v := strings.TrimSpace(item)
			if v == "" {
				continue
			}
			k := strings.ToLower(v)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, v)
			if len(out) >= 8 {
				break
			}
		}
		return out
	}
	appendSection := func(buf *bytes.Buffer, title string, lines []string) {
		buf.WriteString(title)
		buf.WriteString("\n")
		if len(lines) == 0 {
			buf.WriteString("- （未设置）\n\n")
			return
		}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			buf.WriteString("- ")
			buf.WriteString(line)
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}

	var buf bytes.Buffer
	appendSection(&buf, "【运营身份】", []string{
		fmt.Sprintf("你当前站在“%s”的业务视角做判断。", firstNonEmpty(strValue("operator_identity"), "运营")),
	})
	appendSection(&buf, "【业务场景与交付目标】", []string{
		"本次业务场景：" + firstNonEmpty(strValue("business_scene"), "social_spread"),
		"本次资产目标：" + firstNonEmpty(strValue("asset_goal"), "gif_highlight"),
		"本次交付目标：" + firstNonEmpty(strValue("delivery_goal"), "standalone_shareable"),
	})
	appendSection(&buf, "【成功结果定义】", listValue("success_definition"))
	appendSection(&buf, "【优先抓取倾向】", listValue("must_capture_bias"))
	appendSection(&buf, "【尽量规避倾向】", listValue("avoid_bias"))
	appendSection(&buf, "【成本与候选策略】", []string{
		"当前成本策略：" + firstNonEmpty(strValue("cost_mode"), "balanced"),
		"当前候选策略：" + firstNonEmpty(strValue("candidate_strategy"), "balanced"),
		"候选数量偏好：" + firstNonEmpty(strValue("candidate_count_bias"), "medium"),
		"多样性偏好：" + firstNonEmpty(strValue("diversity_preference"), "medium"),
	})
	appendSection(&buf, "【质量偏好】", []string{
		"清晰度优先级：" + firstNonEmpty(strValue("clarity_priority"), "high"),
		"循环优先级：" + firstNonEmpty(strValue("loop_priority"), "medium"),
		"独立成立优先级：" + firstNonEmpty(strValue("standalone_priority"), "high"),
		"情绪/高光优先级：" + firstNonEmpty(strValue("emotion_priority"), "high"),
	})
	appendSection(&buf, "【风险与限制】", []string{
		"风险容忍度：" + firstNonEmpty(strValue("risk_tolerance"), "low"),
		"字幕/对白依赖容忍度：" + firstNonEmpty(strValue("subtitle_dependency_tolerance"), "low"),
		"质量底线说明：" + firstNonEmpty(strValue("quality_floor_note"), "不要为了凑数量而接受明显模糊或抖动严重的片段"),
	})
	appendSection(&buf, "【用户需求（如有）】", []string{
		firstNonEmpty(strValue("user_request_passthrough"), "无"),
	})
	appendSection(&buf, "【补充业务说明】", []string{
		firstNonEmpty(strValue("extra_business_note"), "无"),
	})
	rendered := strings.TrimSpace(buf.String())
	if rendered == "" {
		return text, payload, "json_passthrough"
	}
	return rendered, payload, "json_schema_rendered"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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

func resolveAIDirectorTaskConstraints(meta videoProbeMeta, qualitySettings QualitySettings) map[string]interface{} {
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
	return map[string]interface{}{
		"target_count_min": targetMin,
		"target_count_max": targetMax,
		"duration_sec_min": durationMin,
		"duration_sec_max": durationMax,
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
	operatorInstruction := strings.TrimSpace(qualitySettings.AIDirectorOperatorInstruction)
	operatorVersion := strings.TrimSpace(qualitySettings.AIDirectorOperatorInstructionVersion)
	if operatorVersion == "" {
		operatorVersion = "v1"
	}
	operatorEnabled := qualitySettings.AIDirectorOperatorEnabled
	operatorSource := "ops.video_quality_settings"

	fixedPromptCore := defaultAIGIFDirectorFixedCorePrompt
	fixedPromptContractTail := defaultAIGIFDirectorFixedContractTailPrompt
	fixedPromptVersion := "built_in_v1"
	fixedPromptSource := "built_in_default"
	contractTailVersion := "built_in_contract_tail_v1"

	if fixedTemplate, templateErr := p.loadAIPromptTemplateWithFallback(targetFormat, "ai1", "fixed"); templateErr == nil {
		if fixedTemplate.Found {
			if strings.TrimSpace(fixedTemplate.Text) != "" && fixedTemplate.Enabled {
				fixedPromptCore = strings.TrimSpace(fixedTemplate.Text)
			}
			if strings.TrimSpace(fixedTemplate.Version) != "" {
				fixedPromptVersion = strings.TrimSpace(fixedTemplate.Version)
			}
			if strings.TrimSpace(fixedTemplate.Source) != "" {
				fixedPromptSource = strings.TrimSpace(fixedTemplate.Source)
			}
		}
	}
	if editableTemplate, templateErr := p.loadAIPromptTemplateWithFallback(targetFormat, "ai1", "editable"); templateErr == nil {
		if editableTemplate.Found {
			operatorInstruction = strings.TrimSpace(editableTemplate.Text)
			operatorEnabled = editableTemplate.Enabled
			if strings.TrimSpace(editableTemplate.Version) != "" {
				operatorVersion = strings.TrimSpace(editableTemplate.Version)
			}
			if strings.TrimSpace(editableTemplate.Source) != "" {
				operatorSource = strings.TrimSpace(editableTemplate.Source)
			}
		}
	}

	operatorInstructionRaw := operatorInstruction
	operatorInstructionRendered, operatorInstructionSchema, operatorInstructionRenderMode := renderAIDirectorOperatorInstruction(operatorInstructionRaw)
	if strings.TrimSpace(operatorInstructionRendered) == "" {
		operatorInstructionRendered = operatorInstructionRaw
	}

	info := map[string]interface{}{
		"enabled":                            cfg.Enabled,
		"provider":                           cfg.Provider,
		"model":                              cfg.Model,
		"prompt_version":                     cfg.PromptVersion,
		"fixed_prompt_version":               fixedPromptVersion,
		"fixed_prompt_source":                fixedPromptSource,
		"fixed_prompt_contract_tail_version": contractTailVersion,
		"operator_instruction_enabled":       operatorEnabled,
		"operator_instruction_version":       operatorVersion,
		"operator_instruction_source":        operatorSource,
		"operator_instruction_raw_len":       len(operatorInstructionRaw),
		"operator_instruction_rendered_len":  len(operatorInstructionRendered),
		"operator_instruction_render_mode":   operatorInstructionRenderMode,
		"target_format":                      targetFormat,
	}
	if len(operatorInstructionSchema) > 0 {
		info["operator_instruction_schema"] = operatorInstructionSchema
	}
	if !cfg.Enabled {
		info["applied"] = false
		return nil, info, fmt.Errorf("director disabled")
	}
	directorInputModeRequested := normalizeAIDirectorInputModeSetting(qualitySettings.AIDirectorInputMode, "hybrid")
	directorInputModeApplied := "frames"
	directorInputSource := "frame_manifest"
	sourceVideoURL := ""
	sourceVideoURLError := ""
	frameSamplingError := ""
	frameSamples := make([]aiDirectorFrameSample, 0)
	frameManifest := make([]map[string]interface{}, 0)

	loadFrameSamples := func() {
		if len(frameSamples) > 0 || frameSamplingError != "" {
			return
		}
		samples, err := sampleAIDirectorFrames(ctx, sourcePath, meta, 6)
		if err != nil {
			frameSamplingError = err.Error()
			frameSamples = nil
			frameManifest = nil
			return
		}
		frameSamples = samples
		frameManifest = make([]map[string]interface{}, 0, len(frameSamples))
		for _, item := range frameSamples {
			frameManifest = append(frameManifest, map[string]interface{}{
				"index":         item.Index,
				"timestamp_sec": roundTo(item.TimestampSec, 3),
				"bytes":         item.Bytes,
			})
		}
	}

	resolveVideoURL := func() bool {
		sourceKey := strings.TrimSpace(job.SourceVideoKey)
		if sourceKey == "" {
			sourceVideoURLError = "source_video_key missing"
			return false
		}
		videoURL, err := p.buildObjectReadURL(sourceKey)
		if err != nil {
			sourceVideoURLError = err.Error()
			return false
		}
		videoURL = strings.TrimSpace(videoURL)
		if videoURL == "" {
			sourceVideoURLError = "empty source video url"
			return false
		}
		sourceVideoURL = videoURL
		return true
	}

	switch directorInputModeRequested {
	case "frames":
		loadFrameSamples()
		directorInputModeApplied = "frames"
		directorInputSource = "frame_manifest"
	case "full_video":
		if resolveVideoURL() {
			directorInputModeApplied = "full_video"
			directorInputSource = "full_video_url"
		} else {
			loadFrameSamples()
			directorInputModeApplied = "frames"
			directorInputSource = "full_video_fallback_frame_manifest"
		}
	default: // hybrid
		if resolveVideoURL() {
			directorInputModeApplied = "full_video"
			directorInputSource = "full_video_url"
		} else {
			loadFrameSamples()
			directorInputModeApplied = "frames"
			directorInputSource = "hybrid_fallback_frame_manifest"
		}
	}

	buildDirectorPayloads := func() (map[string]interface{}, map[string]interface{}) {
		frameRefs := make([]map[string]interface{}, 0, len(frameManifest))
		for _, item := range frameManifest {
			row := map[string]interface{}{
				"index":         item["index"],
				"timestamp_sec": item["timestamp_sec"],
			}
			frameRefs = append(frameRefs, row)
		}
		task := map[string]interface{}{
			"asset_goal":           resolveAIDirectorAssetGoal(targetFormat),
			"business_scene":       "social_spread",
			"delivery_goal":        resolveAIDirectorDeliveryGoal(targetFormat),
			"optimization_target":  resolveAIDirectorOptimizationTarget(qualitySettings.GIFProfile),
			"cost_sensitivity":     resolveAIDirectorCostSensitivity(qualitySettings.GIFTargetSizeKB),
			"hard_constraints":     resolveAIDirectorTaskConstraints(meta, qualitySettings),
			"operator_instruction": map[string]interface{}{"enabled": operatorEnabled, "version": operatorVersion},
			"requested_format":     targetFormat,
		}
		source := map[string]interface{}{
			"title":        strings.TrimSpace(job.Title),
			"duration_sec": roundTo(meta.DurationSec, 3),
			"width":        meta.Width,
			"height":       meta.Height,
			"fps":          roundTo(meta.FPS, 3),
			"aspect_ratio": resolveAIDirectorSourceAspectRatio(meta.Width, meta.Height),
			"orientation":  resolveAIDirectorSourceOrientation(meta.Width, meta.Height),
			"input_mode":   directorInputModeApplied,
		}
		if len(frameRefs) > 0 {
			source["frame_refs"] = frameRefs
		}
		modelPayload := map[string]interface{}{
			"schema_version": "ai1_input_v2",
			"task":           task,
			"source":         source,
			"risk_hints":     resolveAIDirectorRiskHints(meta),
		}
		if sourceVideoURL != "" && strings.EqualFold(directorInputModeApplied, "full_video") {
			source["video_source_kind"] = "full_video_url_attached_in_content_part"
		}

		debugPayload := map[string]interface{}{
			"job_id":                      job.ID,
			"title":                       strings.TrimSpace(job.Title),
			"duration_sec":                roundTo(meta.DurationSec, 3),
			"width":                       meta.Width,
			"height":                      meta.Height,
			"fps":                         roundTo(meta.FPS, 3),
			"gif_profile":                 strings.ToLower(strings.TrimSpace(qualitySettings.GIFProfile)),
			"gif_target_size_kb":          qualitySettings.GIFTargetSizeKB,
			"source_input_mode_requested": directorInputModeRequested,
			"source_input_mode_applied":   directorInputModeApplied,
			"source_input_type":           directorInputSource,
			"frame_count":                 len(frameManifest),
			"frame_manifest":              frameManifest,
			"source_video_url_available":  sourceVideoURL != "",
			"source_video_url_error":      sourceVideoURLError,
			"operator_instruction": map[string]interface{}{
				"enabled":       operatorEnabled,
				"version":       operatorVersion,
				"render_mode":   operatorInstructionRenderMode,
				"text":          operatorInstructionRendered,
				"raw_text":      operatorInstructionRaw,
				"schema_fields": operatorInstructionSchema,
			},
		}
		if sourceVideoURL != "" {
			debugPayload["source_video_url"] = sourceVideoURL
		}
		return modelPayload, debugPayload
	}

	buildDirectorUserParts := func(modelPayload map[string]interface{}) ([]openAICompatContentPart, []byte) {
		userBytes, _ := json.Marshal(modelPayload)
		parts := make([]openAICompatContentPart, 0, len(frameSamples)+2)
		parts = append(parts, openAICompatContentPart{
			Type: "text",
			Text: string(userBytes),
		})
		if sourceVideoURL != "" && strings.EqualFold(directorInputModeApplied, "full_video") {
			parts = append(parts, openAICompatContentPart{
				Type: "video_url",
				VideoURL: &openAICompatVideoURL{
					URL: sourceVideoURL,
				},
			})
			return parts, userBytes
		}
		for _, item := range frameSamples {
			if strings.TrimSpace(item.DataURL) == "" {
				continue
			}
			parts = append(parts, openAICompatContentPart{
				Type: "image_url",
				ImageURL: &openAICompatImageURL{
					URL: item.DataURL,
				},
			})
		}
		return parts, userBytes
	}

	modelPayload, debugPayload := buildDirectorPayloads()
	userParts, userBytes := buildDirectorUserParts(modelPayload)

	systemPrompt := strings.TrimSpace(fixedPromptCore)
	if operatorEnabled && strings.TrimSpace(operatorInstructionRendered) != "" {
		systemPrompt += "\n\n额外的运营指令模板（必须严格遵守，冲突时优先级高于默认偏好）：" +
			"\n版本：" + operatorVersion +
			"\n" + operatorInstructionRendered
	}
	if strings.TrimSpace(fixedPromptContractTail) != "" {
		systemPrompt += "\n\n" + strings.TrimSpace(fixedPromptContractTail)
	}
	if frameSamplingError != "" {
		info["frame_sampling_error"] = frameSamplingError
	}
	info["frame_count"] = len(frameSamples)
	info["director_input_mode_requested"] = directorInputModeRequested
	info["director_input_mode_applied"] = directorInputModeApplied
	info["director_input_source"] = directorInputSource
	info["director_model_payload_schema_version"] = stringFromAny(modelPayload["schema_version"])
	info["source_video_url_available"] = sourceVideoURL != ""
	info["source_video_url_error"] = sourceVideoURLError

	buildPersistContext := func(status string, fallbackUsed bool, raw map[string]interface{}, fallbackReason string) aiGIFDirectivePersistContext {
		return aiGIFDirectivePersistContext{
			Status:              status,
			FallbackUsed:        fallbackUsed,
			BriefVersion:        resolveAIDirectiveBriefVersion(operatorVersion, cfg.PromptVersion),
			ModelVersion:        resolveAIDirectiveModelVersion(cfg.Model, raw),
			InputContext:        modelPayload,
			OperatorEnabled:     operatorEnabled,
			OperatorInstruction: operatorInstructionRendered,
			OperatorVersion:     operatorVersion,
			FallbackReason:      fallbackReason,
		}
	}

	callAndRecordDirector := func(
		attempt int,
		modelPayload map[string]interface{},
		debugPayload map[string]interface{},
		modelPayloadBytes []byte,
		parts []openAICompatContentPart,
	) (string, cloudHighlightUsage, map[string]interface{}, int64, error) {
		modelText, usage, rawResp, durationMs, err := p.callOpenAICompatJSONChatWithUserParts(ctx, cfg, systemPrompt, parts)
		debugPayloadBytes, _ := json.Marshal(debugPayload)
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
				"attempt":                           attempt,
				"prompt_version":                    cfg.PromptVersion,
				"fixed_prompt_version":              fixedPromptVersion,
				"fixed_prompt_source":               fixedPromptSource,
				"fixed_prompt_contract_version":     contractTailVersion,
				"candidate_source":                  directorInputSource,
				"director_input_mode_requested":     directorInputModeRequested,
				"director_input_mode_applied":       directorInputModeApplied,
				"frame_count":                       len(frameSamples),
				"frame_sampling_error":              frameSamplingError,
				"source_video_url_available":        sourceVideoURL != "",
				"source_video_url_error":            sourceVideoURLError,
				"operator_instruction_enabled":      operatorEnabled,
				"operator_instruction_version":      operatorVersion,
				"operator_instruction_raw_len":      len(operatorInstructionRaw),
				"operator_instruction_len":          len(operatorInstructionRendered),
				"operator_instruction_rendered_len": len(operatorInstructionRendered),
				"operator_instruction_render_mode":  operatorInstructionRenderMode,
				"operator_instruction_source":       operatorSource,
				"operator_instruction_schema":       operatorInstructionSchema,
				"director_payload_schema_version":   stringFromAny(modelPayload["schema_version"]),
				"director_model_payload_v2":         modelPayload,
				"director_model_payload_bytes":      len(modelPayloadBytes),
				"director_debug_context_v1":         debugPayload,
				"director_debug_context_bytes":      len(debugPayloadBytes),
				"director_input_payload_v1":         debugPayload,
				"director_input_payload_bytes":      len(debugPayloadBytes),
			},
		})
		return modelText, usage, rawResp, durationMs, err
	}

	modelText, _, rawResp, _, err := callAndRecordDirector(1, modelPayload, debugPayload, userBytes, userParts)
	if err != nil && directorInputModeRequested == "hybrid" && sourceVideoURL != "" && strings.EqualFold(directorInputModeApplied, "full_video") {
		loadFrameSamples()
		if len(frameSamples) > 0 {
			directorInputModeApplied = "frames"
			directorInputSource = "hybrid_retry_frame_manifest"
			sourceVideoURL = ""
			modelPayload, debugPayload = buildDirectorPayloads()
			userParts, userBytes = buildDirectorUserParts(modelPayload)
			info["hybrid_retry"] = "video_input_error_fallback_to_frames"
			info["hybrid_first_error"] = err.Error()
			modelText, _, rawResp, _, err = callAndRecordDirector(2, modelPayload, debugPayload, userBytes, userParts)
		}
	}
	info["frame_count"] = len(frameSamples)
	info["director_input_mode_applied"] = directorInputModeApplied
	info["director_input_source"] = directorInputSource
	info["source_video_url_available"] = sourceVideoURL != ""
	info["source_video_url_error"] = sourceVideoURLError
	if err != nil {
		fallbackDirective := buildFallbackAIGIFDirective(local, qualitySettings, "director_call_error")
		_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *fallbackDirective, rawResp, buildPersistContext("fallback", true, rawResp, "director_call_error"))
		info["applied"] = false
		info["status"] = "fallback"
		info["fallback_used"] = true
		info["error"] = err.Error()
		return nil, info, err
	}

	parsedDirective, responseShape, parseErr := parseAIGIFDirectiveResponse(modelText)
	if parseErr != nil {
		fallbackDirective := buildFallbackAIGIFDirective(local, qualitySettings, "director_parse_error")
		_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *fallbackDirective, rawResp, buildPersistContext("fallback", true, rawResp, "director_parse_error"))
		info["applied"] = false
		info["status"] = "fallback"
		info["fallback_used"] = true
		info["error"] = "parse director response: " + parseErr.Error()
		return nil, info, parseErr
	}
	directive := normalizeAIGIFDirective(parsedDirective, qualitySettings.GIFCandidateMaxOutputs)
	if directive == nil {
		fallbackDirective := buildFallbackAIGIFDirective(local, qualitySettings, "director_output_invalid")
		_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *fallbackDirective, rawResp, buildPersistContext("fallback", true, rawResp, "director_output_invalid"))
		info["applied"] = false
		info["status"] = "fallback"
		info["fallback_used"] = true
		info["error"] = "director output invalid"
		return nil, info, fmt.Errorf("director output invalid")
	}
	if contractErr := validateAIGIFDirectiveContract(*directive); contractErr != nil {
		fallbackDirective := buildFallbackAIGIFDirective(local, qualitySettings, "director_contract_invalid")
		_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *fallbackDirective, rawResp, buildPersistContext("fallback", true, rawResp, "director_contract_invalid"))
		info["applied"] = false
		info["status"] = "fallback"
		info["fallback_used"] = true
		info["error"] = "director contract invalid: " + contractErr.Error()
		return nil, info, contractErr
	}
	_ = p.persistAIGIFDirective(job.ID, job.UserID, cfg, *directive, rawResp, buildPersistContext("ok", false, rawResp, ""))

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
	sourcePath string,
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

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	topNDecision := resolveAIGIFPlannerTargetTopNDecision(meta, directive, qualitySettings)
	targetTopN := topNDecision.AppliedTopN
	frameSamples, frameErr := sampleAIDirectorFrames(ctx, sourcePath, meta, 8)
	frameManifest := make([]map[string]interface{}, 0, len(frameSamples))
	for _, item := range frameSamples {
		frameManifest = append(frameManifest, map[string]interface{}{
			"index":         item.Index,
			"timestamp_sec": roundTo(item.TimestampSec, 3),
			"bytes":         item.Bytes,
		})
	}

	payload := map[string]interface{}{
		"job_id":            job.ID,
		"title":             strings.TrimSpace(job.Title),
		"duration_sec":      roundTo(meta.DurationSec, 3),
		"width":             meta.Width,
		"height":            meta.Height,
		"fps":               roundTo(meta.FPS, 3),
		"target_top_n":      targetTopN,
		"target_window_sec": roundTo(chooseHighlightDuration(meta.DurationSec), 3),
		"frame_count":       len(frameManifest),
		"frame_manifest":    frameManifest,
		"hard_constraint_policy": map[string]interface{}{
			"base_top_n":         topNDecision.BaseTopN,
			"ai_suggested_top_n": topNDecision.AISuggestedTopN,
			"allowed_top_n":      topNDecision.AllowedTopN,
			"applied_top_n":      topNDecision.AppliedTopN,
			"override_enabled":   topNDecision.OverrideEnabled,
			"expand_ratio":       roundTo(topNDecision.ExpandRatio, 4),
			"absolute_cap":       topNDecision.AbsoluteCap,
			"clamp_reason":       topNDecision.ClampReason,
			"duration_tier":      topNDecision.DurationTier,
		},
	}
	if directive != nil {
		payload["director"] = directive
	}
	userBytes, _ := json.Marshal(payload)
	userParts := make([]openAICompatContentPart, 0, len(frameSamples)+1)
	userParts = append(userParts, openAICompatContentPart{
		Type: "text",
		Text: string(userBytes),
	})
	for _, item := range frameSamples {
		if strings.TrimSpace(item.DataURL) == "" {
			continue
		}
		userParts = append(userParts, openAICompatContentPart{
			Type: "image_url",
			ImageURL: &openAICompatImageURL{
				URL: item.DataURL,
			},
		})
	}

	systemPrompt := defaultAIGIFPlannerSystemPrompt
	promptVersion := "built_in_v1"
	promptSource := "built_in_default"
	if template, templateErr := p.loadAIPromptTemplateWithFallback("gif", "ai2", "fixed"); templateErr == nil {
		if template.Found {
			if strings.TrimSpace(template.Text) != "" && template.Enabled {
				systemPrompt = strings.TrimSpace(template.Text)
			}
			if strings.TrimSpace(template.Version) != "" {
				promptVersion = strings.TrimSpace(template.Version)
			}
			if strings.TrimSpace(template.Source) != "" {
				promptSource = strings.TrimSpace(template.Source)
			}
		}
	}

	modelText, usage, rawResp, durationMs, err := p.callOpenAICompatJSONChatWithUserParts(ctx, cfg, systemPrompt, userParts)
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
			"prompt_version":                cfg.PromptVersion,
			"prompt_template_version":       promptVersion,
			"prompt_template_source":        promptSource,
			"target_top_n":                  targetTopN,
			"target_top_n_base":             topNDecision.BaseTopN,
			"target_top_n_ai_suggested":     topNDecision.AISuggestedTopN,
			"target_top_n_allowed":          topNDecision.AllowedTopN,
			"target_top_n_applied":          topNDecision.AppliedTopN,
			"target_top_n_override_enabled": topNDecision.OverrideEnabled,
			"target_top_n_expand_ratio":     roundTo(topNDecision.ExpandRatio, 4),
			"target_top_n_absolute_cap":     topNDecision.AbsoluteCap,
			"target_top_n_clamp_reason":     topNDecision.ClampReason,
			"target_top_n_duration_tier":    topNDecision.DurationTier,
			"candidate_source":              "frame_manifest",
			"frame_count":                   len(frameSamples),
			"frame_sampling_error": func() string {
				if frameErr != nil {
					return frameErr.Error()
				}
				return ""
			}(),
			"director_applied":            directive != nil,
			"planner_input_payload_v1":    payload,
			"planner_input_payload_bytes": len(userBytes),
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
	selected := selectAIGIFPlannerExecutionCandidates(candidates, targetTopN)
	if len(selected) == 0 {
		selected = candidates
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
	info["target_top_n_base"] = topNDecision.BaseTopN
	info["target_top_n_ai_suggested"] = topNDecision.AISuggestedTopN
	info["target_top_n_allowed"] = topNDecision.AllowedTopN
	info["target_top_n_applied"] = topNDecision.AppliedTopN
	info["selection_policy"] = "ai2_rank_order_primary"
	info["target_top_n_override_enabled"] = topNDecision.OverrideEnabled
	info["target_top_n_expand_ratio"] = roundTo(topNDecision.ExpandRatio, 4)
	info["target_top_n_absolute_cap"] = topNDecision.AbsoluteCap
	info["target_top_n_clamp_reason"] = topNDecision.ClampReason
	info["target_top_n_duration_tier"] = topNDecision.DurationTier
	info["frame_count"] = len(frameSamples)
	if frameErr != nil {
		info["frame_sampling_error"] = frameErr.Error()
	}
	info["director_applied"] = directive != nil
	return suggestion, info, nil
}

func resolveAIGIFPlannerTargetTopN(
	meta videoProbeMeta,
	directive *gifAIDirectiveProfile,
	qualitySettings QualitySettings,
) int {
	return resolveAIGIFPlannerTargetTopNDecision(meta, directive, qualitySettings).AppliedTopN
}

type aiGIFPlannerTopNDecision struct {
	BaseTopN        int
	AISuggestedTopN int
	AllowedTopN     int
	AppliedTopN     int
	OverrideEnabled bool
	ExpandRatio     float64
	AbsoluteCap     int
	ClampReason     string
	DurationTier    string
}

func resolveAIGIFPlannerTargetTopNDecision(
	meta videoProbeMeta,
	directive *gifAIDirectiveProfile,
	qualitySettings QualitySettings,
) aiGIFPlannerTopNDecision {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	_, longVideoThresholdSec, ultraVideoThresholdSec := resolveGIFDurationTierThresholds(qualitySettings)
	baseTarget := qualitySettings.GIFCandidateMaxOutputs
	if baseTarget <= 0 {
		baseTarget = defaultHighlightTopN
	}
	if baseTarget < 1 {
		baseTarget = 1
	}
	if baseTarget > maxGIFCandidateOutputs {
		baseTarget = maxGIFCandidateOutputs
	}
	durationTier := "normal"
	if meta.DurationSec >= ultraVideoThresholdSec {
		durationTier = "ultra"
		ultraCap := qualitySettings.GIFCandidateUltraVideoMaxOutputs
		if ultraCap <= 0 {
			ultraCap = qualitySettings.GIFCandidateLongVideoMaxOutputs
		}
		if ultraCap > 0 && baseTarget > ultraCap {
			baseTarget = ultraCap
		}
	} else if meta.DurationSec >= longVideoThresholdSec {
		durationTier = "long"
		longCap := qualitySettings.GIFCandidateLongVideoMaxOutputs
		if longCap > 0 && baseTarget > longCap {
			baseTarget = longCap
		}
	}
	if baseTarget < 1 {
		baseTarget = 1
	}
	if baseTarget > maxGIFCandidateOutputs {
		baseTarget = maxGIFCandidateOutputs
	}

	aiSuggested := baseTarget
	if directive != nil {
		if directive.ClipCountMax > 0 {
			aiSuggested = directive.ClipCountMax
		}
		if directive.ClipCountMin > 0 && aiSuggested < directive.ClipCountMin {
			aiSuggested = directive.ClipCountMin
		}
	}
	if aiSuggested < 1 {
		aiSuggested = 1
	}
	if aiSuggested > maxGIFCandidateOutputs {
		aiSuggested = maxGIFCandidateOutputs
	}

	decision := aiGIFPlannerTopNDecision{
		BaseTopN:        baseTarget,
		AISuggestedTopN: aiSuggested,
		AllowedTopN:     baseTarget,
		AppliedTopN:     baseTarget,
		OverrideEnabled: qualitySettings.AIDirectorConstraintOverrideEnabled,
		ExpandRatio:     qualitySettings.AIDirectorCountExpandRatio,
		AbsoluteCap:     qualitySettings.AIDirectorCountAbsoluteCap,
		DurationTier:    durationTier,
	}
	if decision.AbsoluteCap <= 0 {
		decision.AbsoluteCap = baseTarget
	}
	if decision.AbsoluteCap > maxGIFCandidateOutputs {
		decision.AbsoluteCap = maxGIFCandidateOutputs
	}
	if decision.AbsoluteCap < baseTarget {
		decision.AbsoluteCap = baseTarget
	}

	if decision.OverrideEnabled {
		expanded := int(float64(baseTarget)*(1.0+decision.ExpandRatio) + 0.5)
		if expanded < baseTarget {
			expanded = baseTarget
		}
		if expanded > decision.AbsoluteCap {
			expanded = decision.AbsoluteCap
		}
		if expanded > maxGIFCandidateOutputs {
			expanded = maxGIFCandidateOutputs
		}
		if expanded < 1 {
			expanded = 1
		}
		decision.AllowedTopN = expanded
	} else {
		decision.AllowedTopN = baseTarget
	}

	decision.AppliedTopN = aiSuggested
	if decision.AppliedTopN > decision.AllowedTopN {
		decision.AppliedTopN = decision.AllowedTopN
		if decision.OverrideEnabled {
			decision.ClampReason = "exceeds_policy_allowed_top_n"
		} else {
			decision.ClampReason = "override_disabled"
		}
	}
	if decision.AppliedTopN < 1 {
		decision.AppliedTopN = 1
	}
	if decision.AppliedTopN > maxGIFCandidateOutputs {
		decision.AppliedTopN = maxGIFCandidateOutputs
	}

	return decision
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

func selectAIGIFPlannerExecutionCandidates(candidates []highlightCandidate, targetTopN int) []highlightCandidate {
	if len(candidates) == 0 {
		return nil
	}
	if targetTopN <= 0 || targetTopN >= len(candidates) {
		return append([]highlightCandidate{}, candidates...)
	}
	selected := make([]highlightCandidate, 0, targetTopN)
	for idx := 0; idx < len(candidates) && len(selected) < targetTopN; idx++ {
		selected = append(selected, candidates[idx])
	}
	return selected
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
	systemPrompt := defaultAIGIFJudgeSystemPrompt
	promptVersion := "built_in_v1"
	promptSource := "built_in_default"
	if template, templateErr := p.loadAIPromptTemplateWithFallback("gif", "ai3", "fixed"); templateErr == nil {
		if template.Found {
			if strings.TrimSpace(template.Text) != "" && template.Enabled {
				systemPrompt = strings.TrimSpace(template.Text)
			}
			if strings.TrimSpace(template.Version) != "" {
				promptVersion = strings.TrimSpace(template.Version)
			}
			if strings.TrimSpace(template.Source) != "" {
				promptSource = strings.TrimSpace(template.Source)
			}
		}
	}

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
			"prompt_version":             cfg.PromptVersion,
			"prompt_template_version":    promptVersion,
			"prompt_template_source":     promptSource,
			"sample_size":                len(samples),
			"judge_input_schema_version": "v2_snake_case",
			"judge_input_payload_v1":     input,
			"judge_input_payload_bytes":  len(userPayload),
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

type gifDeliverFallbackCandidate struct {
	Sample       gifJudgeSample
	ReviewStatus string
}

func fallbackReviewStatusWeight(status string) int {
	switch normalizeGIFAIReviewRecommendation(status) {
	case "keep_internal":
		return 4
	case "need_manual_review":
		return 3
	case "":
		return 2
	case "reject":
		return 1
	default:
		return 2
	}
}

func selectDeliverFallbackCandidate(
	samples []gifJudgeSample,
	reviewByOutput map[uint64]models.VideoJobGIFAIReview,
) (gifDeliverFallbackCandidate, bool) {
	if len(samples) == 0 {
		return gifDeliverFallbackCandidate{}, false
	}
	best := gifDeliverFallbackCandidate{}
	bestSet := false
	scoreOf := func(item gifJudgeSample) float64 {
		if item.Score > 0 {
			return item.Score
		}
		if item.EvalOverall > 0 {
			return item.EvalOverall
		}
		return 0
	}
	for _, sample := range samples {
		current := gifDeliverFallbackCandidate{
			Sample: sample,
		}
		if row, ok := reviewByOutput[sample.OutputID]; ok {
			current.ReviewStatus = normalizeGIFAIReviewRecommendation(row.FinalRecommendation)
		}
		if !bestSet {
			best = current
			bestSet = true
			continue
		}
		currentWeight := fallbackReviewStatusWeight(current.ReviewStatus)
		bestWeight := fallbackReviewStatusWeight(best.ReviewStatus)
		if currentWeight != bestWeight {
			if currentWeight > bestWeight {
				best = current
			}
			continue
		}
		currentScore := scoreOf(current.Sample)
		bestScore := scoreOf(best.Sample)
		if currentScore != bestScore {
			if currentScore > bestScore {
				best = current
			}
			continue
		}
		if current.Sample.EvalClarity != best.Sample.EvalClarity {
			if current.Sample.EvalClarity > best.Sample.EvalClarity {
				best = current
			}
			continue
		}
		if current.Sample.EvalLoop != best.Sample.EvalLoop {
			if current.Sample.EvalLoop > best.Sample.EvalLoop {
				best = current
			}
			continue
		}
		if current.Sample.IsPrimary != best.Sample.IsPrimary {
			if current.Sample.IsPrimary {
				best = current
			}
			continue
		}
		if current.Sample.OutputID < best.Sample.OutputID {
			best = current
		}
	}
	return best, bestSet
}

func (p *Processor) ensureAIGIFDeliverFallback(
	job models.VideoJob,
	triggerReason string,
	contextMeta map[string]interface{},
) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"attempted":      false,
		"applied":        false,
		"trigger_reason": strings.TrimSpace(triggerReason),
	}
	if p == nil || p.db == nil || job.ID == 0 {
		result["reason"] = "invalid_processor"
		return result, nil
	}
	samples, err := p.loadGIFJudgeSamples(job.ID)
	if err != nil {
		result["reason"] = "load_samples_error"
		result["error"] = err.Error()
		return result, err
	}
	result["sample_count"] = len(samples)
	if len(samples) == 0 {
		result["reason"] = "no_gif_outputs"
		return result, nil
	}
	result["attempted"] = true

	outputIDs := make([]uint64, 0, len(samples))
	for _, sample := range samples {
		outputIDs = append(outputIDs, sample.OutputID)
	}
	var reviewRows []models.VideoJobGIFAIReview
	if err := p.db.Where("job_id = ? AND output_id IN ?", job.ID, outputIDs).
		Order("id DESC").
		Find(&reviewRows).Error; err != nil {
		if isMissingTableError(err, "video_job_gif_ai_reviews") {
			result["attempted"] = false
			result["reason"] = "review_table_missing"
			return result, nil
		}
		result["reason"] = "load_reviews_error"
		result["error"] = err.Error()
		return result, err
	}

	latestReviewByOutput := make(map[uint64]models.VideoJobGIFAIReview, len(reviewRows))
	deliverCount := 0
	for _, row := range reviewRows {
		if row.OutputID == nil || *row.OutputID == 0 {
			continue
		}
		outputID := *row.OutputID
		if _, exists := latestReviewByOutput[outputID]; exists {
			continue
		}
		latestReviewByOutput[outputID] = row
		if normalizeGIFAIReviewRecommendation(row.FinalRecommendation) == "deliver" {
			deliverCount++
		}
	}
	result["review_count"] = len(latestReviewByOutput)
	result["deliver_count_before"] = deliverCount
	if deliverCount > 0 {
		result["reason"] = "deliver_exists"
		return result, nil
	}

	selected, ok := selectDeliverFallbackCandidate(samples, latestReviewByOutput)
	if !ok || selected.Sample.OutputID == 0 {
		result["reason"] = "no_selectable_sample"
		return result, nil
	}
	outputID := selected.Sample.OutputID
	existingReview := latestReviewByOutput[outputID]
	prevRecommendation := normalizeGIFAIReviewRecommendation(existingReview.FinalRecommendation)
	if prevRecommendation == "" {
		prevRecommendation = "none"
	}
	proposalID := selected.Sample.ProposalIDByWin
	if (proposalID == nil || *proposalID == 0) && existingReview.ProposalID != nil && *existingReview.ProposalID > 0 {
		proposalID = existingReview.ProposalID
	}
	if (proposalID == nil || *proposalID == 0) && selected.Sample.ProposalRank > 0 {
		if proposal, loadErr := p.loadAIGIFProposalByRank(job.ID, selected.Sample.ProposalRank); loadErr == nil && proposal != nil {
			id := proposal.ID
			proposalID = &id
		}
	}
	semanticVerdict := clampZeroOne(selected.Sample.EvalOverall)
	if semanticVerdict <= 0 {
		semanticVerdict = clampZeroOne(selected.Sample.Score)
	}
	if semanticVerdict <= 0 {
		semanticVerdict = 0.5
	}
	metadata := map[string]interface{}{}
	if existing := parseJSONMap(existingReview.Metadata); len(existing) > 0 {
		for key, value := range existing {
			metadata[key] = value
		}
	}
	metadata["deliver_fallback_applied"] = true
	metadata["deliver_fallback_reason"] = strings.TrimSpace(triggerReason)
	metadata["deliver_fallback_trigger_reason"] = strings.TrimSpace(triggerReason)
	metadata["deliver_fallback_previous_recommendation"] = prevRecommendation
	metadata["deliver_fallback_selected_review_status"] = selected.ReviewStatus
	metadata["deliver_fallback_selected_output_score"] = roundTo(selected.Sample.Score, 4)
	metadata["deliver_fallback_selected_eval_overall"] = roundTo(selected.Sample.EvalOverall, 4)
	metadata["deliver_fallback_selected_eval_clarity"] = roundTo(selected.Sample.EvalClarity, 4)
	metadata["deliver_fallback_selected_eval_loop"] = roundTo(selected.Sample.EvalLoop, 4)
	metadata["deliver_fallback_selected_is_primary"] = selected.Sample.IsPrimary
	metadata["deliver_fallback_applied_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	if len(contextMeta) > 0 {
		metadata["deliver_fallback_context"] = contextMeta
	}
	rawResponse := map[string]interface{}{
		"type":                    "deliver_fallback",
		"trigger_reason":          strings.TrimSpace(triggerReason),
		"output_id":               outputID,
		"previous_recommendation": prevRecommendation,
		"selected_review_status":  selected.ReviewStatus,
	}
	if len(contextMeta) > 0 {
		rawResponse["context"] = contextMeta
	}
	diagnosticReason := fmt.Sprintf("系统兜底：任务已完成但无 deliver，自动提升级最佳产物（trigger=%s）", strings.TrimSpace(triggerReason))
	row := models.VideoJobGIFAIReview{
		JobID:               job.ID,
		UserID:              job.UserID,
		OutputID:            &outputID,
		ProposalID:          proposalID,
		Provider:            "system",
		Model:               "deliver_fallback_v1",
		Endpoint:            "",
		PromptVersion:       "deliver_fallback_v1",
		FinalRecommendation: "deliver",
		SemanticVerdict:     roundTo(semanticVerdict, 4),
		DiagnosticReason:    diagnosticReason,
		SuggestedAction:     "auto_deliver_fallback",
		Metadata:            mustJSON(metadata),
		RawResponse:         mustJSON(rawResponse),
	}
	if err := p.db.Clauses(clause.OnConflict{
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
	}).Create(&row).Error; err != nil {
		if isMissingTableError(err, "video_job_gif_ai_reviews") {
			result["attempted"] = false
			result["reason"] = "review_table_missing"
			return result, nil
		}
		result["reason"] = "persist_failed"
		result["error"] = err.Error()
		return result, err
	}

	result["applied"] = true
	result["reason"] = "deliver_promoted"
	result["deliver_count_after"] = 1
	result["selected_output_id"] = outputID
	result["previous_recommendation"] = prevRecommendation
	return result, nil
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
			IsPrimary:       output.IsPrimary,
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
