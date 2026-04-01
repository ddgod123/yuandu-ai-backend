package videojobs

import (
	"errors"
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/gorm"
)

const (
	gifAIDirectorStage = "director"
	gifAIPlannerStage  = "planner"
	gifAIJudgeStage    = "judge"
	// DeepSeek reasoner may return long reasoning_content payloads.
	// Keep the response cap high to avoid truncation-induced JSON decode errors.
	openAICompatMaxRespBytes = 8 << 20 // 8 MiB
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

	defaultAIImageDirectorFixedCorePrompt = `你是视频转图片任务（PNG/JPG/WEBP/LIVE）的“需求甲方（Prompt Director）”。
你的职责是：在正式抽帧执行前，输出结构化任务指令，指导后续Planner/Worker更稳定地产出高质量静态图片。
你不是最终渲染执行器，不要输出 ffmpeg 参数或最终文件路径。
仅返回JSON（不要markdown）：
{
  "directive": {
    "business_goal": "entertainment|news|design_asset|social_spread",
    "audience": "简短描述",
    "must_capture": ["必须抓取的瞬间/主体特征"],
    "avoid": ["应避免片段/质量风险"],
    "clip_count_min": 3,
    "clip_count_max": 12,
    "duration_pref_min_sec": 0.8,
    "duration_pref_max_sec": 2.4,
    "loop_preference": 0.0,
    "style_direction": "画面风格方向（简短）",
    "risk_flags": ["low_light","fast_motion","low_resolution","watermark_risk"],
    "quality_weights": {"semantic":0.40,"clarity":0.35,"loop":0.05,"efficiency":0.20},
    "directive_text": "给Planner/Worker的自然语言摘要，50~120字"
  }
}
补充要求：
1) 面向静态图时，优先“清晰度、主体完整、语义命中”；
2) 尽量减少无效过渡帧、模糊帧、黑场帧；
3) 若用户需求不清晰，仍需给出保守可执行指令，并在 risk_flags 中标记。`

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
      "raw_scores":{"semantic":0.92,"clarity":0.78,"loop":0.74,"efficiency":0.80},
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
3) raw_scores 必须包含 semantic/clarity/loop/efficiency 且都在 [0,1]；
4) score/standalone_confidence/loop_friendliness_hint 均在 [0,1]；
5) score 建议按 raw_scores 综合估算；系统会按 director.quality_weights 二次计算最终排序；
6) proposal_reason 必须明确说明命中的 must_capture 与规避的 avoid；
7) 若输入包含 director，请优先遵循 director 的目标和偏好；
8) 不要依赖“预先给定候选窗口”，你需要直接从关键帧推断提名窗口。`
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
	ProposalRank         int                `json:"proposal_rank"`
	StartSec             float64            `json:"start_sec"`
	EndSec               float64            `json:"end_sec"`
	Score                float64            `json:"score"`
	RawScores            map[string]float64 `json:"raw_scores,omitempty"`
	ProposalReason       string             `json:"proposal_reason"`
	SemanticTags         []string           `json:"semantic_tags"`
	ExpectedValueLevel   string             `json:"expected_value_level"`
	StandaloneConfidence float64            `json:"standalone_confidence"`
	LoopFriendlinessHint float64            `json:"loop_friendliness_hint"`
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

type aiGIFDirectivePersistContext struct {
	Status              string
	FallbackUsed        bool
	BriefVersion        string
	ModelVersion        string
	InputContext        interface{}
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

func parseVideoJobAIModelPreference(raw string) (provider string, model string) {
	text := strings.TrimSpace(strings.ToLower(raw))
	if text == "" || text == "auto" {
		return "", ""
	}

	switch text {
	case "speed":
		return "qwen", "qwen-turbo"
	case "quality":
		return "qwen", "qwen-max"
	}

	if idx := strings.Index(text, ":"); idx > 0 && idx < len(text)-1 {
		return strings.TrimSpace(text[:idx]), strings.TrimSpace(text[idx+1:])
	}
	if idx := strings.Index(text, "/"); idx > 0 && idx < len(text)-1 {
		return strings.TrimSpace(text[:idx]), strings.TrimSpace(text[idx+1:])
	}
	return "", text
}

func (p *Processor) applyVideoJobAIModelPreference(cfg aiModelCallConfig, job models.VideoJob) (aiModelCallConfig, map[string]interface{}) {
	options := parseJSONMap(job.Options)
	raw := strings.TrimSpace(stringFromAny(options["ai_model_preference"]))
	provider, model := parseVideoJobAIModelPreference(raw)
	meta := map[string]interface{}{
		"requested": raw,
		"applied":   false,
	}
	if model == "" {
		meta["reason"] = "auto_or_empty"
		return cfg, meta
	}

	if provider != "" {
		currentProvider := strings.TrimSpace(strings.ToLower(cfg.Provider))
		requestProvider := strings.TrimSpace(strings.ToLower(provider))
		if currentProvider == "" || currentProvider == requestProvider {
			cfg.Provider = requestProvider
		} else {
			meta["provider_override_ignored"] = requestProvider
		}
	}

	cfg.Model = strings.TrimSpace(model)
	meta["applied"] = true
	meta["provider"] = cfg.Provider
	meta["model"] = cfg.Model
	return cfg, meta
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
			Metadata:             mustJSON(buildAIGIFProposalMetadata(item, status == "selected")),
			RawResponse:          mustJSON(raw),
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

func buildAIGIFProposalMetadata(item gifAIPlannerProposal, selected bool) map[string]interface{} {
	meta := map[string]interface{}{
		"selected":    selected,
		"final_score": roundTo(item.Score, 4),
	}
	if rawScores := normalizeAIGIFPlannerRawScores(item.RawScores); len(rawScores) > 0 {
		meta["raw_scores"] = rawScores
		meta["score_recomputed"] = true
		meta["score_formula"] = "final = semantic×w_semantic + clarity×w_clarity + loop×w_loop + efficiency×w_efficiency"
	}
	return meta
}

func isMissingTableError(err error, table string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, strings.ToLower(strings.TrimSpace(table))) &&
		(strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such table"))
}
