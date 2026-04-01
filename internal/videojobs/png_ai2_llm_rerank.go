package videojobs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"emoji/internal/models"

	xdraw "golang.org/x/image/draw"

	_ "image/gif"
	_ "image/png"
)

const (
	pngAI2LLMRerankModeOff    = "off"
	pngAI2LLMRerankModeShadow = "shadow"
	pngAI2LLMRerankModeOn     = "on"
	pngAI2LLMRerankStage      = "png_ai2_rerank"
)

const defaultPNGAI2LLMRerankSystemPrompt = `你是“视频转图片”PNG链路的终排裁判。输入是一批已经过技术质量门槛的候选帧，请只做重排，不要删减。

目标：
1) 尽量覆盖视频中的不同精彩瞬间，避免连续重复构图；
2) 优先满足 must_capture，尽量规避 avoid；
3) 在覆盖与质量之间平衡，优先保留清晰、主体明确、更有传播价值的画面。

硬性要求：
- 只能使用输入中提供的 candidate_id；
- 输出必须包含全部 candidate_id，且每个只出现一次；
- 如果输入包含 visual_candidate_sequence 与多张图片，图片与 candidate_id 一一对应，请综合视觉内容判断；
- 必须返回严格 JSON（不要 markdown）。

返回格式：
{
  "ordered_candidate_ids": ["c01","c02","..."],
  "reason": "一句话说明重排策略",
  "coverage_score": 0.0
}`

type pngAI2LLMRerankConfig struct {
	Mode               string
	MaxCandidates      int
	MinCandidates      int
	EnablePostEnhance  bool
	IncludeImages      bool
	MaxImageCandidates int
	ImageMaxSide       int
	ImageJPEGQuality   int
	ImageMaxBytes      int
	ModelCfg           aiModelCallConfig
}

type pngAI2LLMRerankResponse struct {
	OrderedCandidateIDs []string `json:"ordered_candidate_ids"`
	OrderedFramePaths   []string `json:"ordered_frame_paths,omitempty"`
	Reason              string   `json:"reason,omitempty"`
	CoverageScore       float64  `json:"coverage_score,omitempty"`
}

type pngAI2LLMRerankCandidate struct {
	CandidateID          string   `json:"candidate_id"`
	FramePath            string   `json:"frame_path,omitempty"`
	FrameName            string   `json:"frame_name,omitempty"`
	LocalRank            int      `json:"local_rank"`
	SceneID              int      `json:"scene_id,omitempty"`
	FinalScore           float64  `json:"final_score"`
	SemanticScore        float64  `json:"semantic_score,omitempty"`
	ClarityScore         float64  `json:"clarity_score,omitempty"`
	LoopScore            float64  `json:"loop_score,omitempty"`
	EfficiencyScore      float64  `json:"efficiency_score,omitempty"`
	MustCaptureHits      []string `json:"must_capture_hits,omitempty"`
	AvoidHits            []string `json:"avoid_hits,omitempty"`
	PositiveSignals      []string `json:"positive_signals,omitempty"`
	NegativeSignals      []string `json:"negative_signals,omitempty"`
	CurrentDecision      string   `json:"current_decision,omitempty"`
	CurrentRejectReason  string   `json:"current_reject_reason,omitempty"`
	CurrentExplainReview string   `json:"current_explain_review,omitempty"`
}

func (p *Processor) maybeApplyPNGAI2LLMRerank(
	ctx context.Context,
	job models.VideoJob,
	primaryFormat string,
	framePaths []string,
	qualityReport frameQualityReport,
	guidance imageAI2Guidance,
	phase string,
) ([]string, map[string]interface{}) {
	report := map[string]interface{}{
		"schema_version": "png_ai2_llm_rerank_v1",
	}
	phase = strings.ToLower(strings.TrimSpace(phase))
	if phase == "" {
		phase = "pre_enhance"
	}
	report["phase"] = phase
	if NormalizeRequestedFormat(primaryFormat) != "png" {
		report["status"] = "skipped_not_png"
		return framePaths, report
	}
	cfg := p.loadPNGAI2LLMRerankConfig()
	modelCfg, modelPreference := p.applyVideoJobAIModelPreference(cfg.ModelCfg, job)
	cfg.ModelCfg = modelCfg
	report["mode"] = cfg.Mode
	report["max_candidates"] = cfg.MaxCandidates
	report["min_candidates"] = cfg.MinCandidates
	report["include_images"] = cfg.IncludeImages
	report["max_image_candidates"] = cfg.MaxImageCandidates
	report["image_max_side"] = cfg.ImageMaxSide
	report["image_jpeg_quality"] = cfg.ImageJPEGQuality
	report["image_max_bytes"] = cfg.ImageMaxBytes
	report["provider"] = cfg.ModelCfg.Provider
	report["model"] = cfg.ModelCfg.Model
	report["model_preference"] = modelPreference
	report["prompt_version"] = cfg.ModelCfg.PromptVersion
	if cfg.Mode == pngAI2LLMRerankModeOff {
		report["status"] = "disabled"
		return framePaths, report
	}
	if phase == "post_enhance" && !cfg.EnablePostEnhance {
		report["status"] = "disabled_post_enhance"
		return framePaths, report
	}
	if len(framePaths) < cfg.MinCandidates {
		report["status"] = "skipped_too_few_candidates"
		report["candidate_count"] = len(framePaths)
		return framePaths, report
	}
	if strings.TrimSpace(cfg.ModelCfg.APIKey) == "" || strings.TrimSpace(cfg.ModelCfg.Endpoint) == "" || strings.TrimSpace(cfg.ModelCfg.Model) == "" {
		report["status"] = "disabled_missing_model_config"
		return framePaths, report
	}

	candidates, idToPath, frameNameToPath := buildPNGAI2LLMRerankCandidates(framePaths, qualityReport, cfg.MaxCandidates)
	if len(candidates) < cfg.MinCandidates {
		report["status"] = "skipped_candidates_not_enough_after_build"
		report["candidate_count"] = len(candidates)
		return framePaths, report
	}

	systemPrompt := defaultPNGAI2LLMRerankSystemPrompt
	promptVersion := "built_in_v1"
	promptSource := "built_in_default"
	if template, err := p.loadAIPromptTemplateWithFallback("png", "ai2", "rerank"); err == nil && template.Found {
		if template.Enabled && strings.TrimSpace(template.Text) != "" {
			systemPrompt = strings.TrimSpace(template.Text)
		}
		if strings.TrimSpace(template.Version) != "" {
			promptVersion = strings.TrimSpace(template.Version)
		}
		if strings.TrimSpace(template.Source) != "" {
			promptSource = strings.TrimSpace(template.Source)
		}
	}

	payload := map[string]interface{}{
		"task":             "reorder_candidates_only",
		"selection_policy": qualityReport.SelectionPolicy,
		"scoring_mode":     qualityReport.ScoringMode,
		"scene":            guidance.Scene,
		"visual_focus":     guidance.VisualFocus,
		"must_capture":     normalizeStringSlice(guidance.MustCapture, 16),
		"avoid":            normalizeStringSlice(guidance.Avoid, 16),
		"quality_weights":  normalizeDirectiveQualityWeights(guidance.QualityWeights),
		"risk_flags":       normalizeStringSlice(guidance.RiskFlags, 16),
		"candidates":       candidates,
	}
	visualCandidates := selectPNGAI2LLMRerankVisualCandidates(candidates, cfg.MaxImageCandidates)
	if cfg.IncludeImages && len(visualCandidates) > 0 {
		payload["visual_candidate_sequence"] = extractPNGAI2CandidateIDs(visualCandidates)
	}
	userBytes, _ := json.Marshal(payload)
	userParts := []openAICompatContentPart{
		{
			Type: "text",
			Text: string(userBytes),
		},
	}
	if cfg.IncludeImages && len(visualCandidates) > 0 {
		visualParts, visualReport := buildPNGAI2LLMRerankVisualParts(visualCandidates, cfg)
		if len(visualParts) > 0 {
			userParts = append(userParts, visualParts...)
		}
		report["vision_input"] = visualReport
	}

	modelText, usage, rawResp, durationMs, callErr := p.callOpenAICompatJSONChatWithUserParts(ctx, cfg.ModelCfg, systemPrompt, userParts)
	reqStatus := "ok"
	reqErr := ""
	if callErr != nil {
		reqStatus = "error"
		reqErr = callErr.Error()
	}
	p.recordVideoJobAIUsage(videoJobAIUsageInput{
		JobID:             job.ID,
		UserID:            job.UserID,
		Stage:             pngAI2LLMRerankStage,
		Provider:          cfg.ModelCfg.Provider,
		Model:             cfg.ModelCfg.Model,
		Endpoint:          cfg.ModelCfg.Endpoint,
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CachedInputTokens: usage.CachedInputTokens,
		ImageTokens:       usage.ImageTokens,
		VideoTokens:       usage.VideoTokens,
		AudioSeconds:      usage.AudioSeconds,
		RequestDurationMs: durationMs,
		RequestStatus:     reqStatus,
		RequestError:      reqErr,
		Metadata: map[string]interface{}{
			"mode":             cfg.Mode,
			"phase":            phase,
			"candidate_count":  len(candidates),
			"vision_enabled":   cfg.IncludeImages,
			"vision_count":     countPNGAI2LLMRerankVisionAttachments(userParts),
			"prompt_version":   promptVersion,
			"prompt_source":    promptSource,
			"payload_bytes":    len(userBytes),
			"selection_policy": qualityReport.SelectionPolicy,
		},
	})

	report["candidate_count"] = len(candidates)
	report["request_duration_ms"] = durationMs
	report["prompt_version"] = promptVersion
	report["prompt_source"] = promptSource
	if callErr != nil {
		report["status"] = "llm_error"
		report["error"] = callErr.Error()
		return framePaths, report
	}

	var parsed pngAI2LLMRerankResponse
	if err := unmarshalModelJSONWithRepair(modelText, &parsed); err != nil {
		report["status"] = "parse_error"
		report["error"] = err.Error()
		if len(rawResp) > 0 {
			report["raw_response_model"] = strings.TrimSpace(stringFromAny(rawResp["model"]))
		}
		return framePaths, report
	}

	ordered := normalizePNGAI2LLMOrderedPaths(parsed, framePaths, idToPath, frameNameToPath)
	if len(ordered) == 0 {
		report["status"] = "invalid_order"
		return framePaths, report
	}

	changed := !isSameStringOrder(framePaths, ordered)
	report["reason"] = strings.TrimSpace(parsed.Reason)
	report["coverage_score"] = roundTo(clampZeroOne(parsed.CoverageScore), 3)
	report["reordered"] = changed
	report["ordered_sample"] = pickFramePathSample(ordered, 6)
	report["ordered_candidate_ids"] = normalizeStringSlice(parsed.OrderedCandidateIDs, len(parsed.OrderedCandidateIDs))

	if cfg.Mode == pngAI2LLMRerankModeOn {
		report["status"] = "applied"
		report["applied"] = changed
		if changed {
			return ordered, report
		}
		return framePaths, report
	}

	report["status"] = "shadow_done"
	report["applied"] = false
	return framePaths, report
}

func (p *Processor) loadPNGAI2LLMRerankConfig() pngAI2LLMRerankConfig {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("PNG_AI2_LLM_RERANK_MODE")))
	switch mode {
	case pngAI2LLMRerankModeOn, pngAI2LLMRerankModeShadow:
	default:
		mode = pngAI2LLMRerankModeOff
	}

	modelCfg := p.loadGIFAIPlannerConfig()
	modelCfg.Enabled = mode != pngAI2LLMRerankModeOff
	if strings.TrimSpace(modelCfg.Provider) == "" {
		modelCfg.Provider = strings.ToLower(strings.TrimSpace(p.cfg.LLMProvider))
	}
	if strings.TrimSpace(modelCfg.Model) == "" {
		modelCfg.Model = strings.TrimSpace(p.cfg.LLMModel)
	}
	if strings.TrimSpace(modelCfg.Endpoint) == "" {
		modelCfg.Endpoint = strings.TrimSpace(p.cfg.LLMEndpoint)
	}
	if strings.TrimSpace(modelCfg.APIKey) == "" {
		modelCfg.APIKey = strings.TrimSpace(p.cfg.LLMAPIKey)
	}
	modelCfg.Provider = strings.ToLower(strings.TrimSpace(firstNonEmptyString(os.Getenv("PNG_AI2_LLM_RERANK_PROVIDER"), modelCfg.Provider)))
	modelCfg.Model = strings.TrimSpace(firstNonEmptyString(os.Getenv("PNG_AI2_LLM_RERANK_MODEL"), modelCfg.Model))
	modelCfg.Endpoint = strings.TrimSpace(firstNonEmptyString(os.Getenv("PNG_AI2_LLM_RERANK_ENDPOINT"), modelCfg.Endpoint))
	modelCfg.APIKey = strings.TrimSpace(firstNonEmptyString(os.Getenv("PNG_AI2_LLM_RERANK_API_KEY"), modelCfg.APIKey))

	timeoutSec := envIntOrDefault("PNG_AI2_LLM_RERANK_TIMEOUT_SECONDS", int(modelCfg.Timeout/time.Second))
	if timeoutSec <= 0 {
		timeoutSec = 25
	}
	modelCfg.Timeout = time.Duration(timeoutSec) * time.Second
	modelCfg.MaxTokens = envIntOrDefault("PNG_AI2_LLM_RERANK_MAX_TOKENS", modelCfg.MaxTokens)
	if modelCfg.MaxTokens <= 0 {
		modelCfg.MaxTokens = 900
	}
	modelCfg.PromptVersion = strings.TrimSpace(firstNonEmptyString(os.Getenv("PNG_AI2_LLM_RERANK_PROMPT_VERSION"), modelCfg.PromptVersion, "png_ai2_rerank_v1"))

	maxCandidates := envIntOrDefault("PNG_AI2_LLM_RERANK_MAX_CANDIDATES", 18)
	maxCandidates = clampInt(maxCandidates, 6, 32)
	minCandidates := envIntOrDefault("PNG_AI2_LLM_RERANK_MIN_CANDIDATES", 4)
	minCandidates = clampInt(minCandidates, 2, maxCandidates)
	enablePostEnhance := parseEnvBool("PNG_AI2_LLM_RERANK_POST_ENHANCE", false)
	includeImages := parseEnvBool("PNG_AI2_LLM_RERANK_INCLUDE_IMAGES", true)
	maxImageCandidates := envIntOrDefault("PNG_AI2_LLM_RERANK_IMAGE_MAX_CANDIDATES", 10)
	maxImageCandidates = clampInt(maxImageCandidates, 0, maxCandidates)
	imageMaxSide := envIntOrDefault("PNG_AI2_LLM_RERANK_IMAGE_MAX_SIDE", 640)
	imageMaxSide = clampInt(imageMaxSide, 256, 1280)
	imageJPEGQuality := envIntOrDefault("PNG_AI2_LLM_RERANK_IMAGE_JPEG_QUALITY", 72)
	imageJPEGQuality = clampInt(imageJPEGQuality, 40, 95)
	imageMaxBytes := envIntOrDefault("PNG_AI2_LLM_RERANK_IMAGE_MAX_BYTES", 900*1024)
	imageMaxBytes = clampInt(imageMaxBytes, 80*1024, 4*1024*1024)

	return pngAI2LLMRerankConfig{
		Mode:               mode,
		MaxCandidates:      maxCandidates,
		MinCandidates:      minCandidates,
		EnablePostEnhance:  enablePostEnhance,
		IncludeImages:      includeImages && maxImageCandidates > 0,
		MaxImageCandidates: maxImageCandidates,
		ImageMaxSide:       imageMaxSide,
		ImageJPEGQuality:   imageJPEGQuality,
		ImageMaxBytes:      imageMaxBytes,
		ModelCfg:           modelCfg,
	}
}

func buildPNGAI2LLMRerankCandidates(
	framePaths []string,
	qualityReport frameQualityReport,
	maxCandidates int,
) ([]pngAI2LLMRerankCandidate, map[string]string, map[string]string) {
	if len(framePaths) == 0 {
		return nil, map[string]string{}, map[string]string{}
	}
	if maxCandidates <= 0 || maxCandidates > len(framePaths) {
		maxCandidates = len(framePaths)
	}
	scoreByPath := map[string]frameQualityCandidateScore{}
	for _, row := range qualityReport.CandidateScores {
		key := strings.TrimSpace(row.FramePath)
		if key == "" {
			continue
		}
		if _, exists := scoreByPath[key]; exists {
			continue
		}
		scoreByPath[key] = row
	}

	out := make([]pngAI2LLMRerankCandidate, 0, maxCandidates)
	idToPath := make(map[string]string, maxCandidates)
	frameNameToPath := make(map[string]string, maxCandidates)
	for idx, path := range framePaths {
		if len(out) >= maxCandidates {
			break
		}
		candidateID := fmt.Sprintf("c%02d", len(out)+1)
		score := scoreByPath[path]
		frameName := filepath.Base(path)
		out = append(out, pngAI2LLMRerankCandidate{
			CandidateID:          candidateID,
			FramePath:            path,
			FrameName:            frameName,
			LocalRank:            idx + 1,
			SceneID:              score.SceneID,
			FinalScore:           roundTo(maxFloat(score.FinalScore, 0), 4),
			SemanticScore:        roundTo(maxFloat(score.SemanticScore, 0), 4),
			ClarityScore:         roundTo(maxFloat(score.ClarityScore, 0), 4),
			LoopScore:            roundTo(maxFloat(score.LoopScore, 0), 4),
			EfficiencyScore:      roundTo(maxFloat(score.EfficiencyScore, 0), 4),
			MustCaptureHits:      normalizeStringSlice(score.MustCaptureHits, 12),
			AvoidHits:            normalizeStringSlice(score.AvoidHits, 12),
			PositiveSignals:      normalizeStringSlice(score.PositiveSignals, 8),
			NegativeSignals:      normalizeStringSlice(score.NegativeSignals, 8),
			CurrentDecision:      strings.TrimSpace(score.Decision),
			CurrentRejectReason:  strings.TrimSpace(score.RejectReason),
			CurrentExplainReview: strings.TrimSpace(score.ExplainSummary),
		})
		idToPath[candidateID] = path
		if frameName != "" {
			frameNameToPath[frameName] = path
		}
	}
	return out, idToPath, frameNameToPath
}

func normalizePNGAI2LLMOrderedPaths(
	resp pngAI2LLMRerankResponse,
	original []string,
	idToPath map[string]string,
	frameNameToPath map[string]string,
) []string {
	if len(original) == 0 {
		return nil
	}
	out := make([]string, 0, len(original))
	seen := map[string]struct{}{}
	appendPath := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}

	for _, id := range resp.OrderedCandidateIDs {
		if path := strings.TrimSpace(idToPath[strings.TrimSpace(id)]); path != "" {
			appendPath(path)
		}
	}

	// 兼容模型输出 ordered_frame_paths（可能是全路径，也可能只给文件名）。
	if len(resp.OrderedFramePaths) > 0 {
		pathSet := map[string]struct{}{}
		for _, p := range original {
			key := strings.TrimSpace(p)
			if key != "" {
				pathSet[key] = struct{}{}
			}
		}
		for _, item := range resp.OrderedFramePaths {
			value := strings.TrimSpace(item)
			if value == "" {
				continue
			}
			if _, ok := pathSet[value]; ok {
				appendPath(value)
				continue
			}
			base := filepath.Base(value)
			if mapped := strings.TrimSpace(frameNameToPath[base]); mapped != "" {
				appendPath(mapped)
			}
		}
	}

	for _, path := range original {
		appendPath(path)
	}
	return out
}

func selectPNGAI2LLMRerankVisualCandidates(candidates []pngAI2LLMRerankCandidate, maxCount int) []pngAI2LLMRerankCandidate {
	if len(candidates) == 0 || maxCount <= 0 {
		return nil
	}
	if maxCount > len(candidates) {
		maxCount = len(candidates)
	}
	out := make([]pngAI2LLMRerankCandidate, 0, maxCount)
	for _, row := range candidates {
		if strings.TrimSpace(row.FramePath) == "" || strings.TrimSpace(row.CandidateID) == "" {
			continue
		}
		out = append(out, row)
		if len(out) >= maxCount {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func extractPNGAI2CandidateIDs(candidates []pngAI2LLMRerankCandidate) []string {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]string, 0, len(candidates))
	for _, item := range candidates {
		id := strings.TrimSpace(item.CandidateID)
		if id == "" {
			continue
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildPNGAI2LLMRerankVisualParts(candidates []pngAI2LLMRerankCandidate, cfg pngAI2LLMRerankConfig) ([]openAICompatContentPart, map[string]interface{}) {
	report := map[string]interface{}{
		"enabled":    cfg.IncludeImages,
		"attempted":  0,
		"succeeded":  0,
		"failed":     0,
		"candidates": len(candidates),
	}
	if !cfg.IncludeImages || len(candidates) == 0 {
		report["status"] = "disabled_or_empty"
		return nil, report
	}

	maxCount := cfg.MaxImageCandidates
	if maxCount <= 0 {
		maxCount = len(candidates)
	}
	if maxCount > len(candidates) {
		maxCount = len(candidates)
	}

	parts := make([]openAICompatContentPart, 0, maxCount*2)
	attachedIDs := make([]string, 0, maxCount)
	failed := make([]map[string]interface{}, 0)
	for idx := 0; idx < maxCount; idx++ {
		row := candidates[idx]
		candidateID := strings.TrimSpace(row.CandidateID)
		framePath := strings.TrimSpace(row.FramePath)
		if candidateID == "" || framePath == "" {
			continue
		}
		report["attempted"] = intFromAny(report["attempted"]) + 1
		dataURL, imageBytes, imageErr := buildPNGAI2LLMRerankImageDataURL(framePath, cfg.ImageMaxSide, cfg.ImageJPEGQuality, cfg.ImageMaxBytes)
		if imageErr != nil || strings.TrimSpace(dataURL) == "" {
			report["failed"] = intFromAny(report["failed"]) + 1
			failed = append(failed, map[string]interface{}{
				"candidate_id": candidateID,
				"frame_name":   filepath.Base(framePath),
				"error":        errorText(imageErr),
			})
			continue
		}
		parts = append(parts, openAICompatContentPart{
			Type: "text",
			Text: fmt.Sprintf("candidate_id=%s frame_name=%s local_rank=%d", candidateID, filepath.Base(framePath), row.LocalRank),
		})
		parts = append(parts, openAICompatContentPart{
			Type: "image_url",
			ImageURL: &openAICompatImageURL{
				URL: dataURL,
			},
		})
		report["succeeded"] = intFromAny(report["succeeded"]) + 1
		attachedIDs = append(attachedIDs, candidateID)
		report["last_image_bytes"] = imageBytes
	}
	if len(attachedIDs) > 0 {
		report["attached_candidate_ids"] = attachedIDs
	}
	if len(failed) > 0 {
		report["failed_sample"] = truncateMapSlice(failed, 8)
	}
	if len(parts) == 0 {
		report["status"] = "no_images_attached"
		return nil, report
	}
	report["status"] = "attached"
	return parts, report
}

func truncateMapSlice(items []map[string]interface{}, maxItems int) []map[string]interface{} {
	if len(items) == 0 || maxItems <= 0 {
		return nil
	}
	if len(items) <= maxItems {
		return items
	}
	out := make([]map[string]interface{}, 0, maxItems)
	for idx := 0; idx < maxItems; idx++ {
		out = append(out, items[idx])
	}
	return out
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func buildPNGAI2LLMRerankImageDataURL(framePath string, maxSide int, jpegQuality int, maxBytes int) (string, int, error) {
	framePath = strings.TrimSpace(framePath)
	if framePath == "" {
		return "", 0, fmt.Errorf("empty frame path")
	}
	if maxSide <= 0 {
		maxSide = 640
	}
	if jpegQuality <= 0 {
		jpegQuality = 72
	}
	if maxBytes <= 0 {
		maxBytes = 900 * 1024
	}

	file, err := os.Open(framePath)
	if err != nil {
		return "", 0, fmt.Errorf("open image: %w", err)
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return "", 0, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return "", 0, fmt.Errorf("invalid image size")
	}
	targetW := width
	targetH := height
	if width > maxSide || height > maxSide {
		if width >= height {
			targetW = maxSide
			targetH = int(roundTo(float64(height)*float64(maxSide)/float64(width), 0))
		} else {
			targetH = maxSide
			targetW = int(roundTo(float64(width)*float64(maxSide)/float64(height), 0))
		}
		if targetW <= 0 {
			targetW = 1
		}
		if targetH <= 0 {
			targetH = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
		img = dst
	}

	quality := clampInt(jpegQuality, 35, 95)
	var encoded []byte
	for {
		var buf bytes.Buffer
		encodeErr := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
		if encodeErr != nil {
			return "", 0, fmt.Errorf("encode jpeg: %w", encodeErr)
		}
		encoded = buf.Bytes()
		if len(encoded) <= maxBytes || quality <= 40 {
			break
		}
		quality -= 8
	}
	if len(encoded) == 0 {
		return "", 0, fmt.Errorf("empty jpeg bytes")
	}
	if len(encoded) > maxBytes {
		return "", len(encoded), fmt.Errorf("jpeg bytes exceed max (%d>%d)", len(encoded), maxBytes)
	}
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encoded)
	return dataURL, len(encoded), nil
}

func countPNGAI2LLMRerankVisionAttachments(parts []openAICompatContentPart) int {
	if len(parts) == 0 {
		return 0
	}
	total := 0
	for _, part := range parts {
		if strings.EqualFold(strings.TrimSpace(part.Type), "image_url") && part.ImageURL != nil && strings.TrimSpace(part.ImageURL.URL) != "" {
			total++
		}
	}
	return total
}

func isSameStringOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func parseEnvBool(key string, def bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return def
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func (c *pngAI2LLMRerankCandidate) stableSortKey() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.CandidateID)
}

func sortPNGAI2LLMRerankCandidates(candidates []pngAI2LLMRerankCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].LocalRank == candidates[j].LocalRank {
			return candidates[i].stableSortKey() < candidates[j].stableSortKey()
		}
		return candidates[i].LocalRank < candidates[j].LocalRank
	})
}
