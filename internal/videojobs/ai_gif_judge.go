package videojobs

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const defaultAIGIFJudgeSystemPrompt = `你是GIF语义复审评委。请根据每个GIF样本的技术评分与上下文，输出可执行的最终建议。
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

func (p *Processor) runAIGIFJudgeReview(ctx context.Context, job models.VideoJob, qualitySettings QualitySettings) (map[string]interface{}, error) {
	cfg := p.loadGIFAIJudgeConfig()
	cfg, modelPreference := p.applyVideoJobAIModelPreference(cfg, job)
	result := aiGIFJudgeRunSnapshot{
		Enabled:       cfg.Enabled,
		Provider:      cfg.Provider,
		Model:         cfg.Model,
		PromptVersion: cfg.PromptVersion,
		Applied:       false,
	}
	result.ModelPreference = modelPreference
	if !cfg.Enabled {
		result.Error = "judge disabled"
		return normalizeVideoJobAIUsageMetadata(result), fmt.Errorf("judge disabled")
	}

	samples, err := p.loadGIFJudgeSamples(job.ID)
	if err != nil {
		result.Error = err.Error()
		return normalizeVideoJobAIUsageMetadata(result), err
	}
	if len(samples) == 0 {
		result.Error = "no gif outputs for judge"
		return normalizeVideoJobAIUsageMetadata(result), fmt.Errorf("no gif outputs for judge")
	}

	input := aiGIFJudgeInputPayload{
		JobID:      job.ID,
		SampleSize: len(samples),
		Outputs:    samples,
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
	usageMetadata := aiGIFJudgeUsageMetadata{
		PromptVersion:           cfg.PromptVersion,
		PromptTemplateVersion:   promptVersion,
		PromptTemplateSource:    promptSource,
		SampleSize:              len(samples),
		JudgeInputSchemaVersion: "v2_snake_case",
		JudgeInputPayloadV1:     input,
		JudgeInputPayloadBytes:  len(userPayload),
	}
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
		Metadata:          usageMetadata,
	})
	if callErr != nil {
		result.Error = callErr.Error()
		return normalizeVideoJobAIUsageMetadata(result), callErr
	}

	var parsed gifAIJudgeResponse
	if err := unmarshalModelJSONWithRepair(modelText, &parsed); err != nil {
		result.Error = "parse judge response: " + err.Error()
		return normalizeVideoJobAIUsageMetadata(result), err
	}
	validReviews, _ := normalizeAIGIFJudgeReviews(parsed.Reviews, samples)
	if len(validReviews) == 0 {
		result.Error = "judge produced empty valid reviews"
		return normalizeVideoJobAIUsageMetadata(result), fmt.Errorf("judge produced empty valid reviews")
	}
	if contractErr := validateAIGIFJudgeReviewsContract(validReviews, samples); contractErr != nil {
		result.Error = "judge contract invalid: " + contractErr.Error()
		return normalizeVideoJobAIUsageMetadata(result), contractErr
	}

	gatedReviews, hardGateByOutput, hardGateStats := applyAIGIFTechnicalHardGates(validReviews, samples, qualitySettings)
	if err := p.persistAIGIFReviews(job, cfg, samples, gatedReviews, hardGateByOutput, rawResp); err != nil {
		result.Error = err.Error()
		return normalizeVideoJobAIUsageMetadata(result), err
	}

	counts := countAIGIFJudgeRecommendations(gatedReviews)
	result.Applied = true
	result.ReviewedOutputs = len(gatedReviews)
	result.DeliverCount = counts["deliver_count"]
	result.KeepInternalCount = counts["keep_internal_count"]
	result.RejectCount = counts["reject_count"]
	result.ManualReviewCount = counts["manual_review_count"]
	result.HardGateApplied = hardGateStats.Applied
	result.HardGateRejectCount = hardGateStats.RejectCount
	result.HardGateManualReviewCount = hardGateStats.ManualReviewCount
	result.Summary = parsed.Summary
	return normalizeVideoJobAIUsageMetadata(result), nil
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
	result := aiGIFDeliverFallbackResult{
		Attempted:     false,
		Applied:       false,
		TriggerReason: strings.TrimSpace(triggerReason),
	}
	if p == nil || p.db == nil || job.ID == 0 {
		result.Reason = "invalid_processor"
		return normalizeVideoJobAIUsageMetadata(result), nil
	}
	samples, err := p.loadGIFJudgeSamples(job.ID)
	if err != nil {
		result.Reason = "load_samples_error"
		result.Error = err.Error()
		return normalizeVideoJobAIUsageMetadata(result), err
	}
	result.SampleCount = len(samples)
	if len(samples) == 0 {
		result.Reason = "no_gif_outputs"
		return normalizeVideoJobAIUsageMetadata(result), nil
	}
	result.Attempted = true

	outputIDs := make([]uint64, 0, len(samples))
	for _, sample := range samples {
		outputIDs = append(outputIDs, sample.OutputID)
	}
	var reviewRows []models.VideoJobGIFAIReview
	if err := p.db.Where("job_id = ? AND output_id IN ?", job.ID, outputIDs).
		Order("id DESC").
		Find(&reviewRows).Error; err != nil {
		if isMissingTableError(err, "video_job_gif_ai_reviews") {
			result.Attempted = false
			result.Reason = "review_table_missing"
			return normalizeVideoJobAIUsageMetadata(result), nil
		}
		result.Reason = "load_reviews_error"
		result.Error = err.Error()
		return normalizeVideoJobAIUsageMetadata(result), err
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
	result.ReviewCount = len(latestReviewByOutput)
	result.DeliverCountBefore = deliverCount
	if deliverCount > 0 {
		result.Reason = "deliver_exists"
		return normalizeVideoJobAIUsageMetadata(result), nil
	}

	selected, ok := selectDeliverFallbackCandidate(samples, latestReviewByOutput)
	if !ok || selected.Sample.OutputID == 0 {
		result.Reason = "no_selectable_sample"
		return normalizeVideoJobAIUsageMetadata(result), nil
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
	fallbackMeta := aiGIFDeliverFallbackMetadata{
		Applied:                true,
		Reason:                 strings.TrimSpace(triggerReason),
		TriggerReason:          strings.TrimSpace(triggerReason),
		PreviousRecommendation: prevRecommendation,
		SelectedReviewStatus:   selected.ReviewStatus,
		SelectedOutputScore:    roundTo(selected.Sample.Score, 4),
		SelectedEvalOverall:    roundTo(selected.Sample.EvalOverall, 4),
		SelectedEvalClarity:    roundTo(selected.Sample.EvalClarity, 4),
		SelectedEvalLoop:       roundTo(selected.Sample.EvalLoop, 4),
		SelectedIsPrimary:      selected.Sample.IsPrimary,
		AppliedAt:              time.Now().UTC().Format(time.RFC3339Nano),
	}
	if len(contextMeta) > 0 {
		fallbackMeta.Context = contextMeta
	}
	metadata := parseJSONMap(existingReview.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["deliver_fallback"] = fallbackMeta
	rawResponse := aiGIFDeliverFallbackRawResponse{
		Type:                   "deliver_fallback",
		TriggerReason:          strings.TrimSpace(triggerReason),
		OutputID:               outputID,
		PreviousRecommendation: prevRecommendation,
		SelectedReviewStatus:   selected.ReviewStatus,
	}
	if len(contextMeta) > 0 {
		rawResponse.Context = contextMeta
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
			result.Attempted = false
			result.Reason = "review_table_missing"
			return normalizeVideoJobAIUsageMetadata(result), nil
		}
		result.Reason = "persist_failed"
		result.Error = err.Error()
		return normalizeVideoJobAIUsageMetadata(result), err
	}

	result.Applied = true
	result.Reason = "deliver_promoted"
	result.DeliverCountAfter = 1
	result.SelectedOutputID = outputID
	result.PreviousRecommendation = prevRecommendation
	return normalizeVideoJobAIUsageMetadata(result), nil
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
	qualitySettings = NormalizeQualitySettings(qualitySettings)

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
	if sample.DurationMs > 0 && sample.DurationMs < qualitySettings.GIFAIJudgeHardGateMinDurationMS {
		addReason("duration_too_short")
	}
	if sample.Score > 0 && sample.Score < qualitySettings.GIFAIJudgeHardGateMinOutputScore {
		addReason("output_score_low")
	}

	evalMissing := sample.EvalOverall <= 0 && sample.EvalClarity <= 0 && sample.EvalLoop <= 0 && sample.EvalMotion <= 0
	if evalMissing {
		addReason("evaluation_missing")
	}
	if sample.EvalOverall > 0 && sample.EvalOverall < qualitySettings.GIFAIJudgeHardGateMinOverallScore {
		addReason("overall_low")
	}
	if sample.EvalClarity > 0 && sample.EvalClarity < qualitySettings.GIFAIJudgeHardGateMinClarityScore {
		addReason("clarity_low")
	}
	if sample.EvalLoop > 0 && sample.EvalLoop < qualitySettings.GIFAIJudgeHardGateMinLoopScore {
		addReason("loop_low")
	}

	targetKB := qualitySettings.GIFTargetSizeKB
	if targetKB <= 0 {
		targetKB = DefaultQualitySettings().GIFTargetSizeKB
	}
	hardMaxBytes := int64(targetKB) * 1024 * int64(qualitySettings.GIFAIJudgeHardGateSizeMultiplier)
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
			Metadata: mustJSON(aiGIFJudgeReviewRowMetadata{
				ProposalRank: item.ProposalRank,
				OutputScore:  sample.Score,
				EvalOverall:  sample.EvalOverall,
				EvalLoop:     sample.EvalLoop,
				EvalClarity:  sample.EvalClarity,
				WindowStart:  sample.StartSec,
				WindowEnd:    sample.EndSec,
				HardGate: aiGIFJudgeHardGateMetadata{
					Applied:       hardGate.Applied,
					Blocked:       hardGate.Blocked,
					From:          hardGate.FromRecommendation,
					To:            hardGate.ToRecommendation,
					ReasonCodes:   hardGate.ReasonCodes,
					ReasonSummary: strings.Join(hardGate.ReasonCodes, ","),
				},
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
