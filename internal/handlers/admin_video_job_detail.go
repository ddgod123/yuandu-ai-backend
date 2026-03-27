package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetAdminVideoJob godoc
// @Summary Get video job detail (admin)
// @Tags admin
// @Produce json
// @Param id path int true "job id"
// @Param review_status query string false "comma-separated review statuses: deliver,keep_internal,reject,need_manual_review"
// @Success 200 {object} AdminVideoJobDetailResponse
// @Router /api/admin/video-jobs/{id} [get]
func (h *Handler) GetAdminVideoJob(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	reviewStatusFilter := parseVideoJobReviewStatusFilter(c.Query("review_status"))
	reviewStatusSet := buildVideoJobReviewStatusSet(reviewStatusFilter)

	job, err := h.loadAdminVideoJobByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	userMap := h.loadVideoJobUserMap([]models.VideoJob{job})
	collectionMap := h.loadVideoJobCollectionMap([]models.VideoJob{job})
	costMap := h.loadVideoJobCostMap([]models.VideoJob{job})
	pointHoldMap := h.loadVideoJobPointHoldMap([]models.VideoJob{job})
	auditSummaryMap := h.loadVideoJobAuditSummaryMap([]models.VideoJob{job})
	jobItem := h.buildAdminVideoJobListItem(job, userMap, collectionMap, costMap, pointHoldMap, auditSummaryMap)

	var events []models.VideoImageEventPublic
	if err := h.db.Where("job_id = ?", job.ID).Order("id DESC").Limit(200).Find(&events).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	respEvents := make([]AdminVideoJobEventItem, 0, len(events))
	for _, item := range events {
		respEvents = append(respEvents, AdminVideoJobEventItem{
			ID:        item.ID,
			Stage:     item.Stage,
			Level:     item.Level,
			Message:   item.Message,
			Metadata:  parseJSONMap(item.Metadata),
			CreatedAt: item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var artifacts []models.VideoImageOutputPublic
	if err := h.db.Where("job_id = ?", job.ID).Order("id DESC").Limit(200).Find(&artifacts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	respArtifacts := make([]AdminVideoJobArtifactItem, 0, len(artifacts))
	for _, item := range artifacts {
		respArtifacts = append(respArtifacts, AdminVideoJobArtifactItem{
			ID:         item.ID,
			Format:     strings.TrimSpace(item.Format),
			Type:       item.FileRole,
			QiniuKey:   item.ObjectKey,
			URL:        resolvePreviewURL(item.ObjectKey, h.qiniu),
			MimeType:   item.MimeType,
			SizeBytes:  item.SizeBytes,
			Width:      item.Width,
			Height:     item.Height,
			DurationMs: item.DurationMs,
			ProposalID: item.ProposalID,
			IsPrimary:  item.IsPrimary,
			Metadata:   parseJSONMap(item.Metadata),
			CreatedAt:  item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var gifCandidates []models.VideoJobGIFCandidate
	if err := h.db.Where("job_id = ?", job.ID).
		Order("is_selected DESC, final_rank ASC, base_score DESC, id ASC").
		Limit(300).
		Find(&gifCandidates).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_gif_candidates") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	respCandidates := make([]AdminVideoJobGIFCandidateItem, 0, len(gifCandidates))
	for _, item := range gifCandidates {
		respCandidates = append(respCandidates, AdminVideoJobGIFCandidateItem{
			ID:              item.ID,
			StartMs:         item.StartMs,
			EndMs:           item.EndMs,
			DurationMs:      item.DurationMs,
			BaseScore:       item.BaseScore,
			ConfidenceScore: item.ConfidenceScore,
			FinalRank:       item.FinalRank,
			IsSelected:      item.IsSelected,
			RejectReason:    strings.TrimSpace(item.RejectReason),
			FeatureJSON:     parseJSONMap(item.FeatureJSON),
			CreatedAt:       item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var aiUsageRows []models.VideoJobAIUsage
	if err := h.db.Where("job_id = ?", job.ID).
		Order("id DESC").
		Limit(300).
		Find(&aiUsageRows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "ops.video_job_ai_usage") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	respAIUsages := make([]AdminVideoJobAIUsageItem, 0, len(aiUsageRows))
	for _, item := range aiUsageRows {
		respAIUsages = append(respAIUsages, AdminVideoJobAIUsageItem{
			ID:                item.ID,
			Stage:             strings.TrimSpace(item.Stage),
			Provider:          strings.TrimSpace(item.Provider),
			Model:             strings.TrimSpace(item.Model),
			Endpoint:          strings.TrimSpace(item.Endpoint),
			RequestStatus:     strings.TrimSpace(item.RequestStatus),
			RequestError:      strings.TrimSpace(item.RequestError),
			RequestDurationMs: item.RequestDurationMs,
			InputTokens:       item.InputTokens,
			OutputTokens:      item.OutputTokens,
			CachedInputTokens: item.CachedInputTokens,
			ImageTokens:       item.ImageTokens,
			VideoTokens:       item.VideoTokens,
			AudioSeconds:      item.AudioSeconds,
			CostUSD:           item.CostUSD,
			Currency:          strings.TrimSpace(item.Currency),
			PricingVersion:    strings.TrimSpace(item.PricingVersion),
			PricingSourceURL:  strings.TrimSpace(item.PricingSourceURL),
			Metadata:          parseJSONMap(item.Metadata),
			CreatedAt:         item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var aiDirectiveRows []models.VideoJobGIFAIDirective
	if err := h.db.Where("job_id = ?", job.ID).
		Order("id DESC").
		Limit(80).
		Find(&aiDirectiveRows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_gif_ai_directives") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	respAIDirectives := make([]AdminVideoJobAIGIFDirectiveItem, 0, len(aiDirectiveRows))
	for _, item := range aiDirectiveRows {
		respAIDirectives = append(respAIDirectives, AdminVideoJobAIGIFDirectiveItem{
			ID:                 item.ID,
			BusinessGoal:       strings.TrimSpace(item.BusinessGoal),
			Audience:           strings.TrimSpace(item.Audience),
			MustCapture:        parseStringSliceJSON(item.MustCapture),
			Avoid:              parseStringSliceJSON(item.Avoid),
			ClipCountMin:       item.ClipCountMin,
			ClipCountMax:       item.ClipCountMax,
			DurationPrefMinSec: item.DurationPrefMinSec,
			DurationPrefMaxSec: item.DurationPrefMaxSec,
			LoopPreference:     item.LoopPreference,
			StyleDirection:     strings.TrimSpace(item.StyleDirection),
			RiskFlags:          parseStringSliceJSON(item.RiskFlags),
			QualityWeights:     parseJSONMap(item.QualityWeights),
			BriefVersion:       strings.TrimSpace(item.BriefVersion),
			ModelVersion:       strings.TrimSpace(item.ModelVersion),
			DirectiveText:      strings.TrimSpace(item.DirectiveText),
			Status:             strings.TrimSpace(item.Status),
			FallbackUsed:       item.FallbackUsed,
			InputContext:       parseJSONMap(item.InputContextJSON),
			Provider:           strings.TrimSpace(item.Provider),
			Model:              strings.TrimSpace(item.Model),
			PromptVersion:      strings.TrimSpace(item.PromptVersion),
			Metadata:           parseJSONMap(item.Metadata),
			CreatedAt:          item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var aiProposalRows []models.VideoJobGIFAIProposal
	if err := h.db.Where("job_id = ?", job.ID).
		Order("proposal_rank ASC, id ASC").
		Limit(300).
		Find(&aiProposalRows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_gif_ai_proposals") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	respAIProposals := make([]AdminVideoJobAIGIFProposalItem, 0, len(aiProposalRows))
	for _, item := range aiProposalRows {
		respAIProposals = append(respAIProposals, AdminVideoJobAIGIFProposalItem{
			ID:                   item.ID,
			ProposalRank:         item.ProposalRank,
			StartSec:             item.StartSec,
			EndSec:               item.EndSec,
			DurationSec:          item.DurationSec,
			BaseScore:            item.BaseScore,
			ProposalReason:       strings.TrimSpace(item.ProposalReason),
			SemanticTags:         parseStringSliceJSON(item.SemanticTags),
			ExpectedValueLevel:   strings.TrimSpace(item.ExpectedValueLevel),
			StandaloneConfidence: item.StandaloneConfidence,
			LoopFriendlinessHint: item.LoopFriendlinessHint,
			Status:               strings.TrimSpace(item.Status),
			Provider:             strings.TrimSpace(item.Provider),
			Model:                strings.TrimSpace(item.Model),
			PromptVersion:        strings.TrimSpace(item.PromptVersion),
			Metadata:             parseJSONMap(item.Metadata),
			CreatedAt:            item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var aiReviewRows []models.VideoJobGIFAIReview
	if err := h.db.Where("job_id = ?", job.ID).
		Order("id DESC").
		Limit(300).
		Find(&aiReviewRows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_gif_ai_reviews") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	reviewStatusCounts := map[string]int64{
		"deliver":            0,
		"keep_internal":      0,
		"reject":             0,
		"need_manual_review": 0,
	}
	respAIReviews := make([]AdminVideoJobAIGIFReviewItem, 0, len(aiReviewRows))
	for _, item := range aiReviewRows {
		status := normalizeVideoJobReviewStatus(item.FinalRecommendation)
		statusLabel := status
		if statusLabel == "" {
			statusLabel = strings.TrimSpace(item.FinalRecommendation)
		}
		if status != "" {
			reviewStatusCounts[status]++
		}
		if reviewStatusSet != nil {
			if _, ok := reviewStatusSet[status]; !ok {
				continue
			}
		}
		respAIReviews = append(respAIReviews, AdminVideoJobAIGIFReviewItem{
			ID:                  item.ID,
			OutputID:            item.OutputID,
			ProposalID:          item.ProposalID,
			FinalRecommendation: statusLabel,
			SemanticVerdict:     item.SemanticVerdict,
			DiagnosticReason:    strings.TrimSpace(item.DiagnosticReason),
			SuggestedAction:     strings.TrimSpace(item.SuggestedAction),
			Provider:            strings.TrimSpace(item.Provider),
			Model:               strings.TrimSpace(item.Model),
			PromptVersion:       strings.TrimSpace(item.PromptVersion),
			Metadata:            parseJSONMap(item.Metadata),
			CreatedAt:           item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var aiImageReviewRows []models.VideoJobImageAIReview
	if err := h.db.Where("job_id = ?", job.ID).
		Order("id DESC").
		Limit(120).
		Find(&aiImageReviewRows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_image_ai_reviews") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	respAIImageReviews := make([]AdminVideoJobAIImageReviewItem, 0, len(aiImageReviewRows))
	for _, item := range aiImageReviewRows {
		respAIImageReviews = append(respAIImageReviews, AdminVideoJobAIImageReviewItem{
			ID:                        item.ID,
			TargetFormat:              strings.TrimSpace(item.TargetFormat),
			Stage:                     strings.TrimSpace(item.Stage),
			Recommendation:            strings.TrimSpace(item.Recommendation),
			ReviewedOutputs:           item.ReviewedOutputs,
			DeliverCount:              item.DeliverCount,
			RejectCount:               item.RejectCount,
			ManualReviewCount:         item.ManualReviewCount,
			HardGateRejectCount:       item.HardGateRejectCount,
			HardGateManualReviewCount: item.HardGateManualReviewCount,
			CandidateBudget:           item.CandidateBudget,
			EffectiveDurationSec:      item.EffectiveDurationSec,
			QualityFallback:           item.QualityFallback,
			QualitySelectorVersion:    strings.TrimSpace(item.QualitySelectorVersion),
			SummaryNote:               strings.TrimSpace(item.SummaryNote),
			Summary:                   parseJSONMap(item.SummaryJSON),
			Metadata:                  parseJSONMap(item.Metadata),
			CreatedAt:                 item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	ai1Debug, err := h.buildAdminVideoJobAI1Debug(job, aiUsageRows, aiDirectiveRows)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, AdminVideoJobDetailResponse{
		Job:                     jobItem,
		Events:                  respEvents,
		Artifacts:               respArtifacts,
		GIFCandidates:           respCandidates,
		AIUsages:                respAIUsages,
		AIGIFDirectives:         respAIDirectives,
		AIGIFProposals:          respAIProposals,
		AIGIFReviews:            respAIReviews,
		AIImageReviews:          respAIImageReviews,
		AI1Debug:                ai1Debug,
		AIGIFReviewStatusCounts: reviewStatusCounts,
		AIGIFReviewStatusFilter: reviewStatusFilter,
	})
}

func (h *Handler) buildAdminVideoJobAI1Debug(
	job models.VideoJob,
	aiUsageRows []models.VideoJobAIUsage,
	aiDirectiveRows []models.VideoJobGIFAIDirective,
) (*AdminVideoJobAI1DebugItem, error) {
	if h == nil || h.db == nil || job.ID == 0 {
		return nil, nil
	}

	var planRow models.VideoJobAI1Plan
	planFound := true
	if err := h.db.Where("job_id = ?", job.ID).Order("id DESC").First(&planRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableError(err, "video_job_ai1_plans") {
			planFound = false
		} else {
			return nil, err
		}
	}

	usageFound := false
	usageRow := models.VideoJobAIUsage{}
	for _, item := range aiUsageRows {
		if strings.EqualFold(strings.TrimSpace(item.Stage), "director") {
			usageRow = item
			usageFound = true
			break
		}
	}
	if !usageFound {
		for _, item := range aiUsageRows {
			if strings.EqualFold(strings.TrimSpace(item.Stage), "ai1") {
				usageRow = item
				usageFound = true
				break
			}
		}
	}

	directiveFound := len(aiDirectiveRows) > 0
	directiveRow := models.VideoJobGIFAIDirective{}
	if directiveFound {
		directiveRow = aiDirectiveRows[0]
	}

	options := parseJSONMap(job.Options)
	metrics := parseJSONMap(job.Metrics)
	requestedFormat := videojobs.PrimaryRequestedFormat(job.OutputFormats)
	if requestedFormat == "" {
		requestedFormat = strings.ToLower(strings.TrimSpace(stringFromAny(options["requested_format"])))
	}
	requestedFormat = normalizeRequestedFormatForDebug(requestedFormat)
	sourcePrompt := resolveVideoJobDebugSourcePrompt(job, options)
	flowMode := strings.ToLower(strings.TrimSpace(stringFromAny(options["flow_mode"])))
	if flowMode == "" {
		flowMode = "direct"
	}

	planJSON := map[string]interface{}{}
	planSchemaVersion := ""
	planStatus := ""
	planModelProvider := ""
	planModelName := ""
	planPromptVersion := ""
	if planFound {
		planJSON = parseJSONMap(planRow.PlanJSON)
		planSchemaVersion = strings.TrimSpace(planRow.SchemaVersion)
		planStatus = strings.TrimSpace(planRow.Status)
		planModelProvider = strings.TrimSpace(planRow.ModelProvider)
		planModelName = strings.TrimSpace(planRow.ModelName)
		planPromptVersion = strings.TrimSpace(planRow.PromptVersion)
	}

	ai1OutputV2 := mapFromAnyValue(planJSON["ai1_output_v2"])
	ai1OutputContract := buildAI1OutputContractReport(ai1OutputV2)
	userReply, ai2Instruction := buildAI1DebugOutput(planJSON)

	usageMetadata := map[string]interface{}{}
	if usageFound {
		usageMetadata = parseJSONMap(usageRow.Metadata)
	}

	directiveRawResponse := map[string]interface{}{}
	directiveInputContext := map[string]interface{}{}
	if directiveFound {
		directiveRawResponse = parseJSONMap(directiveRow.RawResponse)
		directiveInputContext = parseJSONMap(directiveRow.InputContextJSON)
	}

	modelRequest := map[string]interface{}{
		"provider":       planModelProvider,
		"model":          planModelName,
		"prompt_version": planPromptVersion,
		"flow_mode":      flowMode,
	}
	if usageFound {
		modelRequest["provider"] = firstNonEmptyString(stringFromAny(modelRequest["provider"]), strings.TrimSpace(usageRow.Provider))
		modelRequest["model"] = firstNonEmptyString(stringFromAny(modelRequest["model"]), strings.TrimSpace(usageRow.Model))
		modelRequest["endpoint"] = strings.TrimSpace(usageRow.Endpoint)
		modelRequest["request_status"] = strings.TrimSpace(usageRow.RequestStatus)
		modelRequest["request_error"] = strings.TrimSpace(usageRow.RequestError)
		modelRequest["request_duration_ms"] = usageRow.RequestDurationMs
		modelRequest["usage"] = map[string]interface{}{
			"input_tokens":        usageRow.InputTokens,
			"output_tokens":       usageRow.OutputTokens,
			"cached_input_tokens": usageRow.CachedInputTokens,
			"image_tokens":        usageRow.ImageTokens,
			"video_tokens":        usageRow.VideoTokens,
			"audio_seconds":       usageRow.AudioSeconds,
			"cost_usd":            usageRow.CostUSD,
		}
		if payload := mapFromAnyValue(usageMetadata["director_model_payload_v2"]); len(payload) > 0 {
			modelRequest["director_model_payload_v2"] = payload
		}
		if debugCtx := mapFromAnyValue(usageMetadata["director_debug_context_v1"]); len(debugCtx) > 0 {
			modelRequest["director_debug_context_v1"] = debugCtx
		}
		if systemPrompt := strings.TrimSpace(stringFromAny(usageMetadata["system_prompt_text"])); systemPrompt != "" {
			modelRequest["system_prompt_text"] = systemPrompt
		}
	}
	modelRequest["payload_summary_v2"] = buildAI1ModelRequestSummary(modelRequest, usageMetadata)

	developerRules := map[string]interface{}{}
	for _, key := range []string{
		"fixed_prompt_version",
		"fixed_prompt_source",
		"fixed_prompt_contract_version",
		"operator_instruction_enabled",
		"operator_instruction_version",
		"operator_instruction_source",
		"operator_instruction_render_mode",
		"operator_instruction_schema",
		"director_payload_schema_version",
		"director_input_mode_requested",
		"director_input_mode_applied",
		"director_input_source",
	} {
		if value, exists := usageMetadata[key]; exists && value != nil {
			developerRules[key] = value
		}
	}

	modelResponse := map[string]interface{}{
		"plan_found":      planFound,
		"usage_found":     usageFound,
		"directive_found": directiveFound,
	}
	if directiveFound {
		modelResponse["raw_response"] = directiveRawResponse
		modelResponse["normalized_directive"] = map[string]interface{}{
			"business_goal":         strings.TrimSpace(directiveRow.BusinessGoal),
			"audience":              strings.TrimSpace(directiveRow.Audience),
			"must_capture":          parseJSONStringArray(directiveRow.MustCapture),
			"avoid":                 parseJSONStringArray(directiveRow.Avoid),
			"clip_count_min":        directiveRow.ClipCountMin,
			"clip_count_max":        directiveRow.ClipCountMax,
			"duration_pref_min_sec": directiveRow.DurationPrefMinSec,
			"duration_pref_max_sec": directiveRow.DurationPrefMaxSec,
			"loop_preference":       directiveRow.LoopPreference,
			"style_direction":       strings.TrimSpace(directiveRow.StyleDirection),
			"risk_flags":            parseJSONStringArray(directiveRow.RiskFlags),
			"quality_weights":       parseJSONMap(directiveRow.QualityWeights),
			"directive_text":        strings.TrimSpace(directiveRow.DirectiveText),
			"status":                strings.TrimSpace(directiveRow.Status),
			"fallback_used":         directiveRow.FallbackUsed,
			"prompt_version":        strings.TrimSpace(directiveRow.PromptVersion),
			"model_version":         strings.TrimSpace(directiveRow.ModelVersion),
		}
		if len(directiveInputContext) > 0 {
			modelResponse["input_context_v2"] = directiveInputContext
		}
	}
	modelResponse["response_summary_v2"] = buildAI1ModelResponseSummary(
		directiveFound,
		directiveRow,
		directiveRawResponse,
		usageFound,
		usageRow,
	)

	input := map[string]interface{}{
		"user": map[string]interface{}{
			"prompt":              sourcePrompt,
			"title":               strings.TrimSpace(job.Title),
			"ai_model_preference": strings.TrimSpace(stringFromAny(options["ai_model_preference"])),
		},
		"video": map[string]interface{}{
			"source_video_key": strings.TrimSpace(job.SourceVideoKey),
			"output_formats":   strings.TrimSpace(job.OutputFormats),
			"probe":            mapFromAnyValue(options["source_video_probe"]),
			"metrics_meta": map[string]interface{}{
				"duration_sec": metrics["duration_sec"],
				"width":        metrics["width"],
				"height":       metrics["height"],
				"fps":          metrics["fps"],
			},
		},
		"developer_rules": developerRules,
	}

	rowTrace := map[string]interface{}{
		"video_job_id":     job.ID,
		"video_job_stage":  strings.TrimSpace(job.Stage),
		"video_job_status": strings.TrimSpace(job.Status),
	}
	if planFound {
		rowTrace["ai1_plan_updated_at"] = planRow.UpdatedAt
	}
	if usageFound {
		rowTrace["ai_usage_created_at"] = usageRow.CreatedAt
	}
	if directiveFound {
		rowTrace["directive_updated_at"] = directiveRow.UpdatedAt
	}

	trace := map[string]interface{}{
		"options": map[string]interface{}{
			"execution_queue":       stringFromAny(options["execution_queue"]),
			"execution_task_type":   stringFromAny(options["execution_task_type"]),
			"ai1_plan_schema":       stringFromAny(options["ai1_plan_schema_version"]),
			"ai1_plan_mode":         stringFromAny(options["ai1_plan_mode"]),
			"ai1_plan_applied":      boolFromAny(options["ai1_plan_applied"]),
			"ai1_plan_generated":    boolFromAny(options["ai1_plan_generated"]),
			"ai1_pending":           boolFromAny(options["ai1_pending"]),
			"ai1_confirmed":         boolFromAny(options["ai1_confirmed"]),
			"ai1_pause_consumed":    boolFromAny(options["ai1_pause_consumed"]),
			"requested_format":      stringFromAny(options["requested_format"]),
			"quality_overrides":     mapFromAnyValue(options["quality_profile_overrides"]),
			"source_video_probe_v1": mapFromAnyValue(options["source_video_probe"]),
		},
		"plan": map[string]interface{}{
			"schema_version": planSchemaVersion,
			"status":         planStatus,
			"plan_json":      planJSON,
		},
		"rows":                   rowTrace,
		"ai1_output_contract_v2": ai1OutputContract,
	}

	output := map[string]interface{}{
		"user_reply":      userReply,
		"ai2_instruction": ai2Instruction,
		"contract_report": ai1OutputContract,
	}
	if len(ai1OutputV2) > 0 {
		output["ai1_output_v2"] = ai1OutputV2
	}
	trace["timeline_v1"] = buildAI1DebugTimelineV1(
		sourcePrompt,
		requestedFormat,
		input,
		modelRequest,
		modelResponse,
		output,
		ai1OutputContract,
	)

	fieldAudit := make([]AdminVideoJobAI1FieldAuditItem, 0, 48)
	appendAudit := func(stage, fieldPath, label, source string, value interface{}, detail string) {
		row := AdminVideoJobAI1FieldAuditItem{
			Stage:     strings.TrimSpace(strings.ToLower(stage)),
			FieldPath: strings.TrimSpace(fieldPath),
			Label:     strings.TrimSpace(label),
			Source:    strings.TrimSpace(strings.ToLower(source)),
			Detail:    strings.TrimSpace(detail),
		}
		if value != nil {
			row.Value = value
		}
		fieldAudit = append(fieldAudit, row)
	}

	userPromptRaw := strings.TrimSpace(stringFromAny(options["user_prompt"]))
	sourcePromptSource := "user_input"
	sourcePromptDetail := "from options.user_prompt"
	if userPromptRaw == "" {
		sourcePromptSource = "fallback"
		sourcePromptDetail = "fallback to job.title because options.user_prompt is empty"
	}
	appendAudit("input", "user.prompt", "用户提示词", sourcePromptSource, sourcePrompt, sourcePromptDetail)
	appendAudit("input", "user.ai_model_preference", "用户模型偏好", "user_input", strings.TrimSpace(stringFromAny(options["ai_model_preference"])), "from options.ai_model_preference")
	appendAudit("input", "video.source_video_key", "视频源 key", "video_asset", strings.TrimSpace(job.SourceVideoKey), "from video_jobs.source_video_key")
	appendAudit("input", "video.output_formats", "请求输出格式", "user_input", strings.TrimSpace(job.OutputFormats), "from video_jobs.output_formats")
	probe := mapFromAnyValue(options["source_video_probe"])
	appendAudit("input", "video.probe.duration_sec", "探针时长(s)", "video_probe", probe["duration_sec"], "from options.source_video_probe.duration_sec")
	appendAudit("input", "video.probe.width", "探针宽度(px)", "video_probe", probe["width"], "from options.source_video_probe.width")
	appendAudit("input", "video.probe.height", "探针高度(px)", "video_probe", probe["height"], "from options.source_video_probe.height")
	appendAudit("input", "video.probe.fps", "探针帧率", "video_probe", probe["fps"], "from options.source_video_probe.fps")
	for _, key := range sortedMapKeys(developerRules) {
		appendAudit(
			"input",
			"developer_rules."+key,
			"开发规则 "+key,
			"developer_rule",
			developerRules[key],
			"from director usage metadata",
		)
	}

	appendAudit("model_request", "provider", "模型供应商", "system_route", modelRequest["provider"], "resolved from ai usage row / ai1 plan fallback")
	appendAudit("model_request", "model", "模型名称", "system_route", modelRequest["model"], "resolved from ai usage row / ai1 plan fallback")
	appendAudit("model_request", "prompt_version", "提示词版本", "developer_rule", modelRequest["prompt_version"], "from ai1 plan or template metadata")
	appendAudit("model_request", "request_status", "调用状态", "system_runtime", modelRequest["request_status"], "from ops.video_job_ai_usage.request_status")
	appendAudit("model_request", "request_duration_ms", "调用耗时(ms)", "system_runtime", modelRequest["request_duration_ms"], "from ops.video_job_ai_usage.request_duration_ms")
	appendAudit("model_request", "payload_summary_v2", "请求负载摘要", "system_runtime", modelRequest["payload_summary_v2"], "generated by buildAI1ModelRequestSummary")

	outputUserReply := mapFromAnyValue(output["user_reply"])
	outputAI2Instruction := mapFromAnyValue(output["ai2_instruction"])
	appendAudit("output", "user_reply", "用户可读理解", "model_output", outputUserReply, "from ai1_output_v2.user_feedback (fallback ai1_output_v1)")
	appendAudit("output", "ai2_instruction", "AI2执行指令", "model_output", outputAI2Instruction, "from ai1_output_v2.ai2_directive (fallback ai1_output_v1)")
	appendAudit("output", "contract_report", "AI1输出协议校验", "system_validation", output["contract_report"], "from buildAI1OutputContractReport")
	appendAudit("output", "model_response.response_summary_v2", "模型响应摘要", "system_runtime", modelResponse["response_summary_v2"], "from buildAI1ModelResponseSummary")
	appendAudit("output", "trace.options.ai1_plan_schema", "AI1计划 schema", "system_runtime", stringFromAny(options["ai1_plan_schema_version"]), "from options.ai1_plan_schema_version")
	appendAudit("output", "trace.options.ai1_plan_mode", "AI1计划模式", "system_runtime", stringFromAny(options["ai1_plan_mode"]), "from options.ai1_plan_mode")
	appendAudit("output", "trace.options.ai1_confirmed", "AI1是否已确认", "user_action", boolFromAny(options["ai1_confirmed"]), "from options.ai1_confirmed")

	return &AdminVideoJobAI1DebugItem{
		RequestedFormat: strings.ToLower(strings.TrimSpace(requestedFormat)),
		FlowMode:        flowMode,
		SourcePrompt:    sourcePrompt,
		FieldAudit:      fieldAudit,
		Input:           input,
		ModelRequest:    modelRequest,
		ModelResponse:   modelResponse,
		Output:          output,
		Trace:           trace,
	}, nil
}
