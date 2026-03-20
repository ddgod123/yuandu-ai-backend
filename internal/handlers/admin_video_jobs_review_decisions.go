package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AdminVideoJobGIFReviewDecisionItemRequest struct {
	OutputID   uint64  `json:"output_id"`
	ProposalID *uint64 `json:"proposal_id,omitempty"`
	Decision   string  `json:"decision"`
	Reason     string  `json:"reason,omitempty"`
	Notes      string  `json:"notes,omitempty"`
}

type AdminVideoJobGIFReviewDecisionsRequest struct {
	RequestID string                                      `json:"request_id"`
	Items     []AdminVideoJobGIFReviewDecisionItemRequest `json:"items"`
}

type AdminVideoJobGIFReviewDecisionItemResult struct {
	OutputID     uint64  `json:"output_id"`
	ProposalID   *uint64 `json:"proposal_id,omitempty"`
	Decision     string  `json:"decision"`
	RequestID    string  `json:"request_id"`
	Applied      bool    `json:"applied"`
	Skipped      bool    `json:"skipped"`
	SkipReason   string  `json:"skip_reason,omitempty"`
	ReviewRowID  uint64  `json:"review_row_id,omitempty"`
	OutputStatus string  `json:"output_status,omitempty"`
}

type AdminVideoJobGIFReviewDecisionsResponse struct {
	JobID     uint64                                     `json:"job_id"`
	RequestID string                                     `json:"request_id"`
	Applied   int                                        `json:"applied"`
	Skipped   int                                        `json:"skipped"`
	Items     []AdminVideoJobGIFReviewDecisionItemResult `json:"items"`
}

// SubmitAdminVideoJobGIFReviewDecisions godoc
// @Summary Submit manual review decisions for gif outputs (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "job id"
// @Param body body AdminVideoJobGIFReviewDecisionsRequest true "manual review decisions"
// @Success 200 {object} AdminVideoJobGIFReviewDecisionsResponse
// @Router /api/admin/video-jobs/{id}/gif-review-decisions [post]
func (h *Handler) SubmitAdminVideoJobGIFReviewDecisions(c *gin.Context) {
	jobID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req AdminVideoJobGIFReviewDecisionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "items required"})
		return
	}
	if len(req.Items) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "items too many"})
		return
	}

	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		requestID = strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	seenOutput := make(map[uint64]struct{}, len(req.Items))
	normalized := make([]AdminVideoJobGIFReviewDecisionItemRequest, 0, len(req.Items))
	for _, item := range req.Items {
		if item.OutputID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "output_id required"})
			return
		}
		decision := normalizeVideoJobReviewStatus(item.Decision)
		if decision == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid decision"})
			return
		}
		if _, exists := seenOutput[item.OutputID]; exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "duplicated output_id in items"})
			return
		}
		seenOutput[item.OutputID] = struct{}{}
		item.Decision = decision
		item.Reason = strings.TrimSpace(item.Reason)
		item.Notes = strings.TrimSpace(item.Notes)
		normalized = append(normalized, item)
	}

	job, err := h.loadAdminVideoJobByID(jobID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	outputIDs := make([]uint64, 0, len(normalized))
	for _, item := range normalized {
		outputIDs = append(outputIDs, item.OutputID)
	}
	var outputs []models.VideoImageOutputPublic
	if err := h.db.Where("job_id = ? AND id IN ?", jobID, outputIDs).Find(&outputs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(outputs) != len(outputIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "some output_id does not belong to this job"})
		return
	}
	outputByID := make(map[uint64]models.VideoImageOutputPublic, len(outputs))
	for _, item := range outputs {
		outputByID[item.ID] = item
	}

	actorID, _ := currentUserIDFromContext(c)
	now := time.Now()
	results := make([]AdminVideoJobGIFReviewDecisionItemResult, 0, len(normalized))
	appliedCount := 0
	skippedCount := 0

	err = h.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range normalized {
			output := outputByID[item.OutputID]

			var existing models.VideoJobGIFAIReview
			existingFound := true
			if err := tx.Where("job_id = ? AND output_id = ?", jobID, item.OutputID).First(&existing).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					existingFound = false
				} else {
					return err
				}
			}

			existingMeta := map[string]interface{}{}
			if existingFound {
				existingMeta = parseJSONMap(existing.Metadata)
				if strings.TrimSpace(stringFromAny(existingMeta["manual_review_request_id"])) == requestID {
					if normalizeVideoJobReviewStatus(existing.FinalRecommendation) == item.Decision {
						results = append(results, AdminVideoJobGIFReviewDecisionItemResult{
							OutputID:     item.OutputID,
							ProposalID:   item.ProposalID,
							Decision:     item.Decision,
							RequestID:    requestID,
							Applied:      false,
							Skipped:      true,
							SkipReason:   "idempotent_duplicate",
							ReviewRowID:  existing.ID,
							OutputStatus: strings.TrimSpace(output.FileRole),
						})
						skippedCount++
						continue
					}
				}
			}

			proposalID := item.ProposalID
			if proposalID == nil && output.ProposalID != nil && *output.ProposalID > 0 {
				id := *output.ProposalID
				proposalID = &id
			}

			metadata := existingMeta
			if len(metadata) == 0 {
				metadata = map[string]interface{}{}
			}
			if existingFound {
				metadata["manual_review_previous_recommendation"] = strings.TrimSpace(existing.FinalRecommendation)
				metadata["manual_review_previous_provider"] = strings.TrimSpace(existing.Provider)
				metadata["manual_review_previous_model"] = strings.TrimSpace(existing.Model)
				metadata["manual_review_previous_prompt_version"] = strings.TrimSpace(existing.PromptVersion)
			}
			metadata["manual_review"] = true
			metadata["manual_review_request_id"] = requestID
			metadata["manual_review_actor_id"] = actorID
			metadata["manual_review_actor_role"] = "admin"
			metadata["manual_review_at"] = now.Format(time.RFC3339)
			metadata["manual_review_reason"] = item.Reason
			metadata["manual_review_notes"] = item.Notes

			review := models.VideoJobGIFAIReview{
				JobID:               jobID,
				UserID:              job.UserID,
				OutputID:            &item.OutputID,
				ProposalID:          proposalID,
				Provider:            "admin_manual",
				Model:               "manual_review_v1",
				Endpoint:            "admin/video-jobs/gif-review-decisions",
				PromptVersion:       "manual_v1",
				FinalRecommendation: item.Decision,
				SemanticVerdict:     existing.SemanticVerdict,
				DiagnosticReason:    item.Reason,
				SuggestedAction:     item.Notes,
				Metadata:            mustJSON(metadata),
				RawResponse: mustJSON(map[string]interface{}{
					"source":     "admin_manual_review",
					"request_id": requestID,
					"decision":   item.Decision,
					"reason":     item.Reason,
					"notes":      item.Notes,
				}),
			}
			if err := tx.Clauses(clause.OnConflict{
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
					"updated_at":           now,
				}),
			}).Create(&review).Error; err != nil {
				return err
			}

			var persisted models.VideoJobGIFAIReview
			if err := tx.Where("job_id = ? AND output_id = ?", jobID, item.OutputID).First(&persisted).Error; err != nil {
				return err
			}

			outputMeta := parseJSONMap(output.Metadata)
			outputMeta["manual_review_decision_v1"] = map[string]interface{}{
				"decision":   item.Decision,
				"reason":     item.Reason,
				"notes":      item.Notes,
				"request_id": requestID,
				"actor_id":   actorID,
				"actor_role": "admin",
				"at":         now.Format(time.RFC3339),
			}
			if err := tx.Model(&models.VideoImageOutputPublic{}).
				Where("id = ? AND job_id = ?", item.OutputID, jobID).
				Update("metadata", mustJSON(outputMeta)).Error; err != nil {
				return err
			}

			results = append(results, AdminVideoJobGIFReviewDecisionItemResult{
				OutputID:     item.OutputID,
				ProposalID:   proposalID,
				Decision:     item.Decision,
				RequestID:    requestID,
				Applied:      true,
				Skipped:      false,
				ReviewRowID:  persisted.ID,
				OutputStatus: strings.TrimSpace(output.FileRole),
			})
			appliedCount++
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.recordAuditLog(actorID, "video_job", jobID, "admin_gif_review_decisions", map[string]interface{}{
		"request_id": requestID,
		"applied":    appliedCount,
		"skipped":    skippedCount,
		"items":      results,
	})

	c.JSON(http.StatusOK, AdminVideoJobGIFReviewDecisionsResponse{
		JobID:     jobID,
		RequestID: requestID,
		Applied:   appliedCount,
		Skipped:   skippedCount,
		Items:     results,
	})
}

func mustJSON(raw interface{}) datatypes.JSON {
	if raw == nil {
		return datatypes.JSON([]byte("{}"))
	}
	encoded, err := json.Marshal(raw)
	if err != nil || len(encoded) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(encoded)
}
