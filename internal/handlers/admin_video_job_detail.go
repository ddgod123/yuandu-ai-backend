package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"emoji/internal/models"

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
	jobItem := h.buildAdminVideoJobListItem(job, userMap, collectionMap, costMap, pointHoldMap)

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
			Type:       item.FileRole,
			QiniuKey:   item.ObjectKey,
			URL:        resolvePreviewURL(item.ObjectKey, h.qiniu),
			MimeType:   item.MimeType,
			SizeBytes:  item.SizeBytes,
			Width:      item.Width,
			Height:     item.Height,
			DurationMs: item.DurationMs,
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

	c.JSON(http.StatusOK, AdminVideoJobDetailResponse{
		Job:                     jobItem,
		Events:                  respEvents,
		Artifacts:               respArtifacts,
		GIFCandidates:           respCandidates,
		AIUsages:                respAIUsages,
		AIGIFDirectives:         respAIDirectives,
		AIGIFProposals:          respAIProposals,
		AIGIFReviews:            respAIReviews,
		AIGIFReviewStatusCounts: reviewStatusCounts,
		AIGIFReviewStatusFilter: reviewStatusFilter,
	})
}
