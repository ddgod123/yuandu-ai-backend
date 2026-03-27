package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AdminVideoJobGIFEvaluationItem struct {
	ID              uint64                 `json:"id"`
	OutputID        *uint64                `json:"output_id,omitempty"`
	ProposalID      *uint64                `json:"proposal_id,omitempty"`
	CandidateID     *uint64                `json:"candidate_id,omitempty"`
	ObjectKey       string                 `json:"object_key,omitempty"`
	PreviewURL      string                 `json:"preview_url,omitempty"`
	WindowStartMs   int                    `json:"window_start_ms"`
	WindowEndMs     int                    `json:"window_end_ms"`
	EmotionScore    float64                `json:"emotion_score"`
	ClarityScore    float64                `json:"clarity_score"`
	MotionScore     float64                `json:"motion_score"`
	LoopScore       float64                `json:"loop_score"`
	EfficiencyScore float64                `json:"efficiency_score"`
	OverallScore    float64                `json:"overall_score"`
	FeatureJSON     map[string]interface{} `json:"feature_json,omitempty"`
	CreatedAt       string                 `json:"created_at"`
}

type AdminVideoJobGIFFeedbackItem struct {
	ID         uint64                 `json:"id"`
	OutputID   *uint64                `json:"output_id,omitempty"`
	ProposalID *uint64                `json:"proposal_id,omitempty"`
	UserID     uint64                 `json:"user_id"`
	Action     string                 `json:"action"`
	Weight     float64                `json:"weight"`
	SceneTag   string                 `json:"scene_tag,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  string                 `json:"created_at"`
}

type AdminVideoJobGIFRerenderRecord struct {
	ReviewID        uint64                 `json:"review_id"`
	OutputID        *uint64                `json:"output_id,omitempty"`
	ProposalID      *uint64                `json:"proposal_id,omitempty"`
	ProposalRank    int                    `json:"proposal_rank"`
	Recommendation  string                 `json:"recommendation"`
	Diagnostic      string                 `json:"diagnostic,omitempty"`
	SuggestedAction string                 `json:"suggested_action,omitempty"`
	Trigger         string                 `json:"trigger,omitempty"`
	ActorID         uint64                 `json:"actor_id,omitempty"`
	ActorRole       string                 `json:"actor_role,omitempty"`
	OutputObjectKey string                 `json:"output_object_key,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt       string                 `json:"created_at"`
}

type AdminVideoJobGIFAuditChainSummary struct {
	CandidateCount         int              `json:"candidate_count"`
	DirectiveCount         int              `json:"directive_count"`
	ProposalCount          int              `json:"proposal_count"`
	OutputCount            int              `json:"output_count"`
	EvaluationCount        int              `json:"evaluation_count"`
	ReviewCount            int              `json:"review_count"`
	FeedbackCount          int              `json:"feedback_count"`
	RerenderCount          int              `json:"rerender_count"`
	AIUsageCount           int              `json:"ai_usage_count"`
	EventCount             int              `json:"event_count"`
	HardGateBlockedCount   int              `json:"hard_gate_blocked_count"`
	LatestRecommendation   string           `json:"latest_recommendation,omitempty"`
	LatestRecommendationAt string           `json:"latest_recommendation_at,omitempty"`
	ReviewStatusCounts     map[string]int64 `json:"review_status_counts,omitempty"`
	PolicyVersion          string           `json:"policy_version,omitempty"`
	ExperimentBucket       string           `json:"experiment_bucket,omitempty"`
	PipelineMode           string           `json:"pipeline_mode,omitempty"`
}

type AdminVideoJobGIFAuditChainResponse struct {
	Job             AdminVideoJobListItem               `json:"job"`
	Summary         AdminVideoJobGIFAuditChainSummary   `json:"summary"`
	Events          []AdminVideoJobEventItem            `json:"events,omitempty"`
	Outputs         []AdminVideoJobArtifactItem         `json:"outputs,omitempty"`
	GIFCandidates   []AdminVideoJobGIFCandidateItem     `json:"gif_candidates,omitempty"`
	AIUsages        []AdminVideoJobAIUsageItem          `json:"ai_usages,omitempty"`
	AIGIFDirectives []AdminVideoJobAIGIFDirectiveItem   `json:"ai_gif_directives,omitempty"`
	AIGIFProposals  []AdminVideoJobAIGIFProposalItem    `json:"ai_gif_proposals,omitempty"`
	AIGIFReviews    []AdminVideoJobAIGIFReviewItem      `json:"ai_gif_reviews,omitempty"`
	Evaluations     []AdminVideoJobGIFEvaluationItem    `json:"gif_evaluations,omitempty"`
	ProposalChains  []AdminVideoJobGIFProposalChainItem `json:"proposal_chains,omitempty"`
	Feedbacks       []AdminVideoJobGIFFeedbackItem      `json:"feedbacks,omitempty"`
	Rerenders       []AdminVideoJobGIFRerenderRecord    `json:"rerenders,omitempty"`
	ReviewFilter    []string                            `json:"review_status_filter,omitempty"`
}

// GetAdminVideoJobGIFAuditChain godoc
// @Summary Get gif audit chain by job (admin)
// @Tags admin
// @Produce json
// @Param id path int true "job id"
// @Param review_status query string false "comma-separated review statuses: deliver,keep_internal,reject,need_manual_review"
// @Success 200 {object} AdminVideoJobGIFAuditChainResponse
// @Router /api/admin/video-jobs/{id}/gif-audit-chain [get]
func (h *Handler) GetAdminVideoJobGIFAuditChain(c *gin.Context) {
	jobID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	reviewStatusFilter := parseVideoJobReviewStatusFilter(c.Query("review_status"))
	reviewStatusSet := buildVideoJobReviewStatusSet(reviewStatusFilter)

	job, err := h.loadAdminVideoJobByID(jobID)
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

	events, err := h.loadAdminVideoJobAuditEvents(job.ID, 600)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	outputs, outputByID, err := h.loadAdminVideoJobAuditOutputs(job.ID, 600)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	candidates, err := h.loadAdminVideoJobAuditCandidates(job.ID, 600)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	aiUsages, err := h.loadAdminVideoJobAuditAIUsages(job.ID, 400)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	directives, err := h.loadAdminVideoJobAuditDirectives(job.ID, 120)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	proposals, proposalByID, err := h.loadAdminVideoJobAuditProposals(job.ID, 600)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	reviews, reviewStatusCounts, hardGateBlockedCount, latestRecommendation, latestRecommendationAt, err := h.loadAdminVideoJobAuditReviews(job.ID, 600, reviewStatusSet)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	evaluations, err := h.loadAdminVideoJobAuditEvaluations(job.ID, outputByID, proposalByID, 600)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	feedbacks, err := h.loadAdminVideoJobAuditFeedbacks(job.ID, outputByID, proposalByID, 600)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rerenders := h.buildAdminVideoJobAuditRerenderRecords(reviews, outputByID)
	proposalChains := h.buildAdminVideoJobAuditProposalChains(proposals, outputs, evaluations, reviews, feedbacks, rerenders)

	metrics := parseJSONMap(job.Metrics)
	policyVersion := strings.TrimSpace(stringFromAny(metrics["pipeline_policy_version"]))
	experimentBucket := strings.TrimSpace(stringFromAny(metrics["experiment_bucket"]))
	pipelineMode := ""
	if mode := mapFromAny(metrics["gif_pipeline_mode_v1"]); len(mode) > 0 {
		pipelineMode = strings.TrimSpace(stringFromAny(mode["resolved_mode"]))
	}
	if pipelineMode == "" {
		pipelineMode = strings.TrimSpace(stringFromAny(metrics["gif_pipeline_mode"]))
	}

	resp := AdminVideoJobGIFAuditChainResponse{
		Job:             jobItem,
		Events:          events,
		Outputs:         outputs,
		GIFCandidates:   candidates,
		AIUsages:        aiUsages,
		AIGIFDirectives: directives,
		AIGIFProposals:  proposals,
		AIGIFReviews:    reviews,
		Evaluations:     evaluations,
		ProposalChains:  proposalChains,
		Feedbacks:       feedbacks,
		Rerenders:       rerenders,
		ReviewFilter:    reviewStatusFilter,
		Summary: AdminVideoJobGIFAuditChainSummary{
			CandidateCount:         len(candidates),
			DirectiveCount:         len(directives),
			ProposalCount:          len(proposals),
			OutputCount:            len(outputs),
			EvaluationCount:        len(evaluations),
			ReviewCount:            len(reviews),
			FeedbackCount:          len(feedbacks),
			RerenderCount:          len(rerenders),
			AIUsageCount:           len(aiUsages),
			EventCount:             len(events),
			HardGateBlockedCount:   hardGateBlockedCount,
			LatestRecommendation:   latestRecommendation,
			LatestRecommendationAt: latestRecommendationAt,
			ReviewStatusCounts:     reviewStatusCounts,
			PolicyVersion:          policyVersion,
			ExperimentBucket:       experimentBucket,
			PipelineMode:           pipelineMode,
		},
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) loadAdminVideoJobByID(jobID uint64) (models.VideoJob, error) {
	var legacy models.VideoJob
	if err := h.db.Where("id = ?", jobID).First(&legacy).Error; err != nil {
		return models.VideoJob{}, err
	}
	return legacy, nil
}

func (h *Handler) loadAdminVideoJobAuditEvents(jobID uint64, limit int) ([]AdminVideoJobEventItem, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}
	gifTables := resolveVideoImageReadTables("gif")
	var rows []models.VideoImageEventPublic
	if err := h.db.Table(gifTables.Events).Where("job_id = ?", jobID).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AdminVideoJobEventItem, 0, len(rows))
	for _, item := range rows {
		out = append(out, AdminVideoJobEventItem{
			ID:        item.ID,
			Stage:     strings.TrimSpace(item.Stage),
			Level:     strings.TrimSpace(item.Level),
			Message:   strings.TrimSpace(item.Message),
			Metadata:  parseJSONMap(item.Metadata),
			CreatedAt: item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return out, nil
}

func (h *Handler) loadAdminVideoJobAuditOutputs(jobID uint64, limit int) ([]AdminVideoJobArtifactItem, map[uint64]models.VideoImageOutputPublic, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 4000 {
		limit = 4000
	}
	gifTables := resolveVideoImageReadTables("gif")
	var rows []models.VideoImageOutputPublic
	if err := h.db.Table(gifTables.Outputs).Where("job_id = ?", jobID).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	out := make([]AdminVideoJobArtifactItem, 0, len(rows))
	byID := make(map[uint64]models.VideoImageOutputPublic, len(rows))
	for _, item := range rows {
		byID[item.ID] = item
		out = append(out, AdminVideoJobArtifactItem{
			ID:         item.ID,
			Format:     strings.TrimSpace(item.Format),
			Type:       strings.TrimSpace(item.FileRole),
			QiniuKey:   strings.TrimSpace(item.ObjectKey),
			URL:        resolvePreviewURL(item.ObjectKey, h.qiniu),
			MimeType:   strings.TrimSpace(item.MimeType),
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
	return out, byID, nil
}

func (h *Handler) loadAdminVideoJobAuditCandidates(jobID uint64, limit int) ([]AdminVideoJobGIFCandidateItem, error) {
	if limit <= 0 {
		limit = 300
	}
	if limit > 5000 {
		limit = 5000
	}
	var rows []models.VideoJobGIFCandidate
	if err := h.db.Where("job_id = ?", jobID).
		Order("is_selected DESC, final_rank ASC, base_score DESC, id ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "archive.video_job_gif_candidates") && strings.Contains(msg, "does not exist") {
			return nil, nil
		}
		return nil, err
	}
	out := make([]AdminVideoJobGIFCandidateItem, 0, len(rows))
	for _, item := range rows {
		out = append(out, AdminVideoJobGIFCandidateItem{
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
	return out, nil
}

func (h *Handler) loadAdminVideoJobAuditAIUsages(jobID uint64, limit int) ([]AdminVideoJobAIUsageItem, error) {
	if limit <= 0 {
		limit = 300
	}
	if limit > 3000 {
		limit = 3000
	}
	var rows []models.VideoJobAIUsage
	if err := h.db.Where("job_id = ?", jobID).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "ops.video_job_ai_usage") && strings.Contains(msg, "does not exist") {
			return nil, nil
		}
		return nil, err
	}
	out := make([]AdminVideoJobAIUsageItem, 0, len(rows))
	for _, item := range rows {
		out = append(out, AdminVideoJobAIUsageItem{
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
	return out, nil
}

func (h *Handler) loadAdminVideoJobAuditDirectives(jobID uint64, limit int) ([]AdminVideoJobAIGIFDirectiveItem, error) {
	if limit <= 0 {
		limit = 80
	}
	if limit > 2000 {
		limit = 2000
	}
	var rows []models.VideoJobGIFAIDirective
	if err := h.db.Where("job_id = ?", jobID).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "archive.video_job_gif_ai_directives") && strings.Contains(msg, "does not exist") {
			return nil, nil
		}
		return nil, err
	}
	out := make([]AdminVideoJobAIGIFDirectiveItem, 0, len(rows))
	for _, item := range rows {
		out = append(out, AdminVideoJobAIGIFDirectiveItem{
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
	return out, nil
}

func (h *Handler) loadAdminVideoJobAuditProposals(jobID uint64, limit int) ([]AdminVideoJobAIGIFProposalItem, map[uint64]models.VideoJobGIFAIProposal, error) {
	if limit <= 0 {
		limit = 300
	}
	if limit > 5000 {
		limit = 5000
	}
	var rows []models.VideoJobGIFAIProposal
	if err := h.db.Where("job_id = ?", jobID).Order("proposal_rank ASC, id ASC").Limit(limit).Find(&rows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "archive.video_job_gif_ai_proposals") && strings.Contains(msg, "does not exist") {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	out := make([]AdminVideoJobAIGIFProposalItem, 0, len(rows))
	byID := make(map[uint64]models.VideoJobGIFAIProposal, len(rows))
	for _, item := range rows {
		byID[item.ID] = item
		out = append(out, AdminVideoJobAIGIFProposalItem{
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
	return out, byID, nil
}

func (h *Handler) loadAdminVideoJobAuditReviews(
	jobID uint64,
	limit int,
	reviewStatusSet map[string]struct{},
) (items []AdminVideoJobAIGIFReviewItem, statusCounts map[string]int64, hardGateBlockedCount int, latestRecommendation string, latestRecommendationAt string, err error) {
	if limit <= 0 {
		limit = 300
	}
	if limit > 5000 {
		limit = 5000
	}
	var rows []models.VideoJobGIFAIReview
	if err := h.db.Where("job_id = ?", jobID).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "archive.video_job_gif_ai_reviews") && strings.Contains(msg, "does not exist") {
			return nil, map[string]int64{
				"deliver":            0,
				"keep_internal":      0,
				"reject":             0,
				"need_manual_review": 0,
			}, 0, "", "", nil
		}
		return nil, nil, 0, "", "", err
	}

	statusCounts = map[string]int64{
		"deliver":            0,
		"keep_internal":      0,
		"reject":             0,
		"need_manual_review": 0,
	}
	items = make([]AdminVideoJobAIGIFReviewItem, 0, len(rows))
	for _, item := range rows {
		status := normalizeVideoJobReviewStatus(item.FinalRecommendation)
		label := status
		if label == "" {
			label = strings.TrimSpace(item.FinalRecommendation)
		}
		if status != "" {
			statusCounts[status]++
		}

		meta := parseJSONMap(item.Metadata)
		if parseBoolFromAny(meta["hard_gate_blocked"]) {
			hardGateBlockedCount++
		}
		if latestRecommendation == "" {
			latestRecommendation = label
			latestRecommendationAt = item.CreatedAt.Format("2006-01-02 15:04:05")
		}
		if reviewStatusSet != nil {
			if _, ok := reviewStatusSet[status]; !ok {
				continue
			}
		}
		items = append(items, AdminVideoJobAIGIFReviewItem{
			ID:                  item.ID,
			OutputID:            item.OutputID,
			ProposalID:          item.ProposalID,
			FinalRecommendation: label,
			SemanticVerdict:     item.SemanticVerdict,
			DiagnosticReason:    strings.TrimSpace(item.DiagnosticReason),
			SuggestedAction:     strings.TrimSpace(item.SuggestedAction),
			Provider:            strings.TrimSpace(item.Provider),
			Model:               strings.TrimSpace(item.Model),
			PromptVersion:       strings.TrimSpace(item.PromptVersion),
			Metadata:            meta,
			CreatedAt:           item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return items, statusCounts, hardGateBlockedCount, latestRecommendation, latestRecommendationAt, nil
}

func (h *Handler) loadAdminVideoJobAuditEvaluations(
	jobID uint64,
	outputByID map[uint64]models.VideoImageOutputPublic,
	proposalByID map[uint64]models.VideoJobGIFAIProposal,
	limit int,
) ([]AdminVideoJobGIFEvaluationItem, error) {
	if limit <= 0 {
		limit = 300
	}
	if limit > 5000 {
		limit = 5000
	}
	var rows []models.VideoJobGIFEvaluation
	if err := h.db.Where("job_id = ?", jobID).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "archive.video_job_gif_evaluations") && strings.Contains(msg, "does not exist") {
			return nil, nil
		}
		return nil, err
	}
	out := make([]AdminVideoJobGIFEvaluationItem, 0, len(rows))
	for _, item := range rows {
		var objectKey string
		if item.OutputID != nil && *item.OutputID > 0 {
			if output, ok := outputByID[*item.OutputID]; ok {
				objectKey = strings.TrimSpace(output.ObjectKey)
			}
		}
		var proposalID *uint64
		if item.OutputID != nil && *item.OutputID > 0 {
			if output, ok := outputByID[*item.OutputID]; ok && output.ProposalID != nil && *output.ProposalID > 0 {
				id := *output.ProposalID
				proposalID = &id
			}
		}
		if proposalID == nil && item.CandidateID != nil && *item.CandidateID > 0 {
			feature := parseJSONMap(item.FeatureJSON)
			if raw := parseUint64FromAny(feature["proposal_id"]); raw > 0 {
				id := raw
				proposalID = &id
			}
		}
		if proposalID == nil && len(proposalByID) > 0 {
			feature := parseJSONMap(item.FeatureJSON)
			if rank := intFromAny(feature["proposal_rank"]); rank > 0 {
				for _, proposal := range proposalByID {
					if proposal.ProposalRank == rank {
						id := proposal.ID
						proposalID = &id
						break
					}
				}
			}
		}
		out = append(out, AdminVideoJobGIFEvaluationItem{
			ID:              item.ID,
			OutputID:        item.OutputID,
			ProposalID:      proposalID,
			CandidateID:     item.CandidateID,
			ObjectKey:       objectKey,
			PreviewURL:      resolvePreviewURL(objectKey, h.qiniu),
			WindowStartMs:   item.WindowStartMs,
			WindowEndMs:     item.WindowEndMs,
			EmotionScore:    item.EmotionScore,
			ClarityScore:    item.ClarityScore,
			MotionScore:     item.MotionScore,
			LoopScore:       item.LoopScore,
			EfficiencyScore: item.EfficiencyScore,
			OverallScore:    item.OverallScore,
			FeatureJSON:     parseJSONMap(item.FeatureJSON),
			CreatedAt:       item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return out, nil
}

func (h *Handler) loadAdminVideoJobAuditFeedbacks(
	jobID uint64,
	outputByID map[uint64]models.VideoImageOutputPublic,
	proposalByID map[uint64]models.VideoJobGIFAIProposal,
	limit int,
) ([]AdminVideoJobGIFFeedbackItem, error) {
	if limit <= 0 {
		limit = 300
	}
	if limit > 5000 {
		limit = 5000
	}
	gifTables := resolveVideoImageReadTables("gif")
	var rows []models.VideoImageFeedbackPublic
	if err := h.db.Table(gifTables.Feedback).Where("job_id = ?", jobID).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "public.video_image_feedback") && strings.Contains(msg, "does not exist") {
			return nil, nil
		}
		return nil, err
	}
	out := make([]AdminVideoJobGIFFeedbackItem, 0, len(rows))
	for _, item := range rows {
		var proposalID *uint64
		if item.OutputID != nil && *item.OutputID > 0 {
			if output, ok := outputByID[*item.OutputID]; ok && output.ProposalID != nil && *output.ProposalID > 0 {
				id := *output.ProposalID
				proposalID = &id
			}
		}
		meta := parseJSONMap(item.Metadata)
		if proposalID == nil {
			if raw := parseUint64FromAny(meta["proposal_id"]); raw > 0 {
				id := raw
				proposalID = &id
			}
		}
		if proposalID == nil && len(proposalByID) > 0 {
			if rank := intFromAny(meta["proposal_rank"]); rank > 0 {
				for _, proposal := range proposalByID {
					if proposal.ProposalRank == rank {
						id := proposal.ID
						proposalID = &id
						break
					}
				}
			}
		}
		out = append(out, AdminVideoJobGIFFeedbackItem{
			ID:         item.ID,
			OutputID:   item.OutputID,
			ProposalID: proposalID,
			UserID:     item.UserID,
			Action:     strings.TrimSpace(strings.ToLower(item.Action)),
			Weight:     item.Weight,
			SceneTag:   strings.TrimSpace(strings.ToLower(item.SceneTag)),
			Metadata:   meta,
			CreatedAt:  item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return out, nil
}

func (h *Handler) buildAdminVideoJobAuditRerenderRecords(
	reviews []AdminVideoJobAIGIFReviewItem,
	outputByID map[uint64]models.VideoImageOutputPublic,
) []AdminVideoJobGIFRerenderRecord {
	if len(reviews) == 0 {
		return nil
	}
	out := make([]AdminVideoJobGIFRerenderRecord, 0, len(reviews))
	for _, item := range reviews {
		meta := item.Metadata
		if len(meta) == 0 {
			continue
		}
		if !parseBoolFromAny(meta["rerender"]) && strings.TrimSpace(strings.ToLower(item.Model)) != "admin_rerender_v1" {
			continue
		}
		record := AdminVideoJobGIFRerenderRecord{
			ReviewID:        item.ID,
			OutputID:        item.OutputID,
			ProposalID:      item.ProposalID,
			ProposalRank:    intFromAny(meta["proposal_rank"]),
			Recommendation:  strings.TrimSpace(item.FinalRecommendation),
			Diagnostic:      strings.TrimSpace(item.DiagnosticReason),
			SuggestedAction: strings.TrimSpace(item.SuggestedAction),
			Trigger:         strings.TrimSpace(stringFromAny(meta["trigger"])),
			ActorID:         parseUint64FromAny(meta["actor_id"]),
			ActorRole:       strings.TrimSpace(stringFromAny(meta["actor_role"])),
			Metadata:        meta,
			CreatedAt:       item.CreatedAt,
		}
		if record.OutputID != nil && *record.OutputID > 0 {
			if output, ok := outputByID[*record.OutputID]; ok {
				record.OutputObjectKey = strings.TrimSpace(output.ObjectKey)
			}
		}
		out = append(out, record)
	}
	return out
}

func (h *Handler) buildAdminVideoJobAuditProposalChains(
	proposals []AdminVideoJobAIGIFProposalItem,
	outputs []AdminVideoJobArtifactItem,
	evaluations []AdminVideoJobGIFEvaluationItem,
	reviews []AdminVideoJobAIGIFReviewItem,
	feedbacks []AdminVideoJobGIFFeedbackItem,
	rerenders []AdminVideoJobGIFRerenderRecord,
) []AdminVideoJobGIFProposalChainItem {
	type chainBucket struct {
		item         AdminVideoJobGIFProposalChainItem
		sortRank     int
		sortID       uint64
		outputSeen   map[uint64]struct{}
		evalSeen     map[uint64]struct{}
		reviewSeen   map[uint64]struct{}
		feedbackSeen map[uint64]struct{}
		rerenderSeen map[uint64]struct{}
	}

	ensureBucket := func(
		buckets map[string]*chainBucket,
		order *[]string,
		key string,
		chainType string,
		sortRank int,
		sortID uint64,
	) *chainBucket {
		if existing, ok := buckets[key]; ok {
			if existing.item.ChainType == "" && chainType != "" {
				existing.item.ChainType = chainType
			}
			if sortRank > 0 && (existing.sortRank <= 0 || sortRank < existing.sortRank) {
				existing.sortRank = sortRank
			}
			if sortID > 0 && (existing.sortID == 0 || sortID < existing.sortID) {
				existing.sortID = sortID
			}
			return existing
		}
		bucket := &chainBucket{
			item: AdminVideoJobGIFProposalChainItem{
				ChainKey:  key,
				ChainType: chainType,
			},
			sortRank:     sortRank,
			sortID:       sortID,
			outputSeen:   make(map[uint64]struct{}),
			evalSeen:     make(map[uint64]struct{}),
			reviewSeen:   make(map[uint64]struct{}),
			feedbackSeen: make(map[uint64]struct{}),
			rerenderSeen: make(map[uint64]struct{}),
		}
		buckets[key] = bucket
		*order = append(*order, key)
		return bucket
	}

	buckets := make(map[string]*chainBucket, len(proposals)+len(outputs))
	order := make([]string, 0, len(proposals)+len(outputs))
	outputChainKeyByID := make(map[uint64]string, len(outputs))

	for _, proposal := range proposals {
		key := fmt.Sprintf("proposal:%d", proposal.ID)
		bucket := ensureBucket(buckets, &order, key, "proposal", proposal.ProposalRank, proposal.ID)
		proposalCopy := proposal
		bucket.item.Proposal = &proposalCopy
	}

	for _, output := range outputs {
		if !isAdminVideoJobGIFMainOutput(output) {
			continue
		}
		proposalID := resolveAdminVideoJobArtifactProposalID(output)
		var key string
		var sortRank int
		if proposalID > 0 {
			key = fmt.Sprintf("proposal:%d", proposalID)
			if existing := findAdminVideoJobProposalRank(proposals, proposalID); existing > 0 {
				sortRank = existing
			}
		} else {
			key = fmt.Sprintf("output:%d", output.ID)
		}
		bucket := ensureBucket(buckets, &order, key, "output_only", sortRank, output.ID)
		if _, ok := bucket.outputSeen[output.ID]; !ok {
			bucket.item.Outputs = append(bucket.item.Outputs, output)
			bucket.outputSeen[output.ID] = struct{}{}
		}
		outputChainKeyByID[output.ID] = key
	}

	for _, evaluation := range evaluations {
		key, sortRank, sortID := resolveAdminVideoJobProposalChainIdentity(
			evaluation.OutputID,
			evaluation.ProposalID,
			evaluation.ID,
			outputChainKeyByID,
			proposals,
			"evaluation",
		)
		bucket := ensureBucket(buckets, &order, key, "evaluation_only", sortRank, sortID)
		if _, ok := bucket.evalSeen[evaluation.ID]; !ok {
			bucket.item.Evaluations = append(bucket.item.Evaluations, evaluation)
			bucket.evalSeen[evaluation.ID] = struct{}{}
		}
	}

	for _, review := range reviews {
		key, sortRank, sortID := resolveAdminVideoJobProposalChainIdentity(
			review.OutputID,
			review.ProposalID,
			review.ID,
			outputChainKeyByID,
			proposals,
			"review",
		)
		bucket := ensureBucket(buckets, &order, key, "review_only", sortRank, sortID)
		if _, ok := bucket.reviewSeen[review.ID]; !ok {
			bucket.item.Reviews = append(bucket.item.Reviews, review)
			bucket.reviewSeen[review.ID] = struct{}{}
		}
	}

	for _, feedback := range feedbacks {
		key, sortRank, sortID := resolveAdminVideoJobProposalChainIdentity(
			feedback.OutputID,
			feedback.ProposalID,
			feedback.ID,
			outputChainKeyByID,
			proposals,
			"feedback",
		)
		bucket := ensureBucket(buckets, &order, key, "feedback_only", sortRank, sortID)
		if _, ok := bucket.feedbackSeen[feedback.ID]; !ok {
			bucket.item.Feedbacks = append(bucket.item.Feedbacks, feedback)
			bucket.feedbackSeen[feedback.ID] = struct{}{}
		}
	}

	for _, rerender := range rerenders {
		key, sortRank, sortID := resolveAdminVideoJobProposalChainIdentity(
			rerender.OutputID,
			rerender.ProposalID,
			rerender.ReviewID,
			outputChainKeyByID,
			proposals,
			"rerender",
		)
		bucket := ensureBucket(buckets, &order, key, "rerender_only", sortRank, sortID)
		if _, ok := bucket.rerenderSeen[rerender.ReviewID]; !ok {
			bucket.item.Rerenders = append(bucket.item.Rerenders, rerender)
			bucket.rerenderSeen[rerender.ReviewID] = struct{}{}
		}
	}

	sort.SliceStable(order, func(i, j int) bool {
		left := buckets[order[i]]
		right := buckets[order[j]]
		switch {
		case left.sortRank > 0 && right.sortRank > 0 && left.sortRank != right.sortRank:
			return left.sortRank < right.sortRank
		case left.sortRank > 0 && right.sortRank <= 0:
			return true
		case left.sortRank <= 0 && right.sortRank > 0:
			return false
		case left.sortID > 0 && right.sortID > 0 && left.sortID != right.sortID:
			return left.sortID < right.sortID
		default:
			return left.item.ChainKey < right.item.ChainKey
		}
	})

	out := make([]AdminVideoJobGIFProposalChainItem, 0, len(order))
	for _, key := range order {
		bucket := buckets[key]
		sort.SliceStable(bucket.item.Outputs, func(i, j int) bool {
			if bucket.item.Outputs[i].ProposalID != nil && bucket.item.Outputs[j].ProposalID != nil &&
				*bucket.item.Outputs[i].ProposalID != *bucket.item.Outputs[j].ProposalID {
				return *bucket.item.Outputs[i].ProposalID < *bucket.item.Outputs[j].ProposalID
			}
			return bucket.item.Outputs[i].ID < bucket.item.Outputs[j].ID
		})
		sort.SliceStable(bucket.item.Evaluations, func(i, j int) bool {
			if bucket.item.Evaluations[i].OutputID != nil && bucket.item.Evaluations[j].OutputID != nil &&
				*bucket.item.Evaluations[i].OutputID != *bucket.item.Evaluations[j].OutputID {
				return *bucket.item.Evaluations[i].OutputID < *bucket.item.Evaluations[j].OutputID
			}
			return bucket.item.Evaluations[i].ID < bucket.item.Evaluations[j].ID
		})
		sort.SliceStable(bucket.item.Reviews, func(i, j int) bool {
			if bucket.item.Reviews[i].OutputID != nil && bucket.item.Reviews[j].OutputID != nil &&
				*bucket.item.Reviews[i].OutputID != *bucket.item.Reviews[j].OutputID {
				return *bucket.item.Reviews[i].OutputID < *bucket.item.Reviews[j].OutputID
			}
			return bucket.item.Reviews[i].ID < bucket.item.Reviews[j].ID
		})
		sort.SliceStable(bucket.item.Feedbacks, func(i, j int) bool {
			if bucket.item.Feedbacks[i].OutputID != nil && bucket.item.Feedbacks[j].OutputID != nil &&
				*bucket.item.Feedbacks[i].OutputID != *bucket.item.Feedbacks[j].OutputID {
				return *bucket.item.Feedbacks[i].OutputID < *bucket.item.Feedbacks[j].OutputID
			}
			return bucket.item.Feedbacks[i].ID < bucket.item.Feedbacks[j].ID
		})
		sort.SliceStable(bucket.item.Rerenders, func(i, j int) bool {
			if bucket.item.Rerenders[i].OutputID != nil && bucket.item.Rerenders[j].OutputID != nil &&
				*bucket.item.Rerenders[i].OutputID != *bucket.item.Rerenders[j].OutputID {
				return *bucket.item.Rerenders[i].OutputID < *bucket.item.Rerenders[j].OutputID
			}
			return bucket.item.Rerenders[i].ReviewID < bucket.item.Rerenders[j].ReviewID
		})

		summary := AdminVideoJobGIFProposalChainSummary{
			OutputCount:          len(bucket.item.Outputs),
			EvaluationCount:      len(bucket.item.Evaluations),
			ReviewCount:          len(bucket.item.Reviews),
			FeedbackCount:        len(bucket.item.Feedbacks),
			RerenderCount:        len(bucket.item.Rerenders),
			FeedbackActionCounts: make(map[string]int),
		}
		for _, review := range bucket.item.Reviews {
			switch normalizeVideoJobReviewStatus(review.FinalRecommendation) {
			case "deliver":
				summary.DeliverCount++
			case "keep_internal":
				summary.KeepInternalCount++
			case "reject":
				summary.RejectCount++
			case "need_manual_review":
				summary.NeedManualReviewCount++
			}
		}
		if len(bucket.item.Reviews) > 0 {
			summary.LatestRecommendation = strings.TrimSpace(bucket.item.Reviews[len(bucket.item.Reviews)-1].FinalRecommendation)
		}
		for _, feedback := range bucket.item.Feedbacks {
			action := strings.TrimSpace(strings.ToLower(feedback.Action))
			if action == "" {
				action = "unknown"
			}
			summary.FeedbackActionCounts[action]++
		}
		if len(summary.FeedbackActionCounts) == 0 {
			summary.FeedbackActionCounts = nil
		}
		bucket.item.Summary = summary
		out = append(out, bucket.item)
	}
	return out
}

func isAdminVideoJobGIFMainOutput(item AdminVideoJobArtifactItem) bool {
	if strings.TrimSpace(strings.ToLower(item.Type)) != "main" {
		return false
	}
	if strings.TrimSpace(strings.ToLower(item.Format)) == "gif" {
		return true
	}
	if strings.Contains(strings.TrimSpace(strings.ToLower(item.MimeType)), "gif") {
		return true
	}
	if strings.Contains(strings.TrimSpace(strings.ToLower(item.QiniuKey)), "/outputs/gif/") {
		return true
	}
	if format := strings.TrimSpace(strings.ToLower(stringFromAny(item.Metadata["format"]))); format == "gif" {
		return true
	}
	return false
}

func resolveAdminVideoJobArtifactProposalID(item AdminVideoJobArtifactItem) uint64 {
	if item.ProposalID != nil && *item.ProposalID > 0 {
		return *item.ProposalID
	}
	if raw := parseUint64FromAny(item.Metadata["proposal_id"]); raw > 0 {
		return raw
	}
	return 0
}

func findAdminVideoJobProposalRank(items []AdminVideoJobAIGIFProposalItem, proposalID uint64) int {
	if proposalID == 0 {
		return 0
	}
	for _, item := range items {
		if item.ID == proposalID {
			return item.ProposalRank
		}
	}
	return 0
}

func resolveAdminVideoJobProposalChainIdentity(
	outputID *uint64,
	proposalID *uint64,
	fallbackID uint64,
	outputChainKeyByID map[uint64]string,
	proposals []AdminVideoJobAIGIFProposalItem,
	fallbackPrefix string,
) (key string, sortRank int, sortID uint64) {
	if outputID != nil && *outputID > 0 {
		if existing, ok := outputChainKeyByID[*outputID]; ok && existing != "" {
			return existing, 0, *outputID
		}
	}
	if proposalID != nil && *proposalID > 0 {
		return fmt.Sprintf("proposal:%d", *proposalID), findAdminVideoJobProposalRank(proposals, *proposalID), *proposalID
	}
	return fmt.Sprintf("%s:%d", fallbackPrefix, fallbackID), 0, fallbackID
}

func parseBoolFromAny(raw interface{}) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case int:
		return value > 0
	case int64:
		return value > 0
	case float64:
		return value > 0
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "y":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func mapFromAny(raw interface{}) map[string]interface{} {
	value, ok := raw.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return value
}

func intFromAny(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case int32:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}
