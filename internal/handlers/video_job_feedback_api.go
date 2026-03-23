package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type SubmitVideoJobFeedbackRequest struct {
	Action   string                 `json:"action" binding:"required"`
	EmojiID  uint64                 `json:"emoji_id"`
	OutputID uint64                 `json:"output_id"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type SubmitVideoJobFeedbackResponse struct {
	JobID     uint64                 `json:"job_id"`
	Action    string                 `json:"action"`
	Weight    float64                `json:"weight"`
	EmojiID   uint64                 `json:"emoji_id,omitempty"`
	OutputID  uint64                 `json:"output_id,omitempty"`
	SceneTag  string                 `json:"scene_tag,omitempty"`
	CreatedAt string                 `json:"created_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// SubmitVideoJobFeedback godoc
// @Summary Submit user feedback for one generated output
// @Tags user
// @Accept json
// @Produce json
// @Param id path int true "job id"
// @Param body body SubmitVideoJobFeedbackRequest true "feedback request"
// @Success 200 {object} SubmitVideoJobFeedbackResponse
// @Router /api/video-jobs/{id}/feedback [post]
func (h *Handler) SubmitVideoJobFeedback(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	jobID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req SubmitVideoJobFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.EmojiID == 0 && req.OutputID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "emoji_id or output_id required"})
		return
	}

	action, weight, ok := parseVideoImageFeedbackAction(req.Action)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action"})
		return
	}

	var job models.VideoJob
	if err := h.db.
		Select("id", "user_id", "status", "asset_domain", "result_collection_id", "metrics").
		Where("id = ? AND user_id = ?", jobID, userID).
		First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if strings.ToLower(strings.TrimSpace(job.Status)) != models.VideoJobStatusDone {
		c.JSON(http.StatusConflict, gin.H{"error": "job is not completed"})
		return
	}
	if job.ResultCollectionID == nil || *job.ResultCollectionID == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "result collection unavailable"})
		return
	}

	var (
		emoji    *models.Emoji
		output   *models.VideoImageOutputPublic
		sceneTag string
		meta     = map[string]interface{}{}
	)

	if req.EmojiID > 0 {
		resolvedEmoji, err := h.resolveFeedbackEmojiInCollection(req.EmojiID, *job.ResultCollectionID)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "emoji not found in this job"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		emoji = resolvedEmoji
		meta["emoji_id"] = emoji.ID
		meta["emoji_format"] = strings.TrimSpace(strings.ToLower(emoji.Format))
		meta["emoji_title"] = strings.TrimSpace(emoji.Title)
		meta["object_key"] = strings.TrimSpace(emoji.FileURL)
	}

	if req.OutputID > 0 {
		resolvedOutput, err := h.resolveFeedbackOutputByJob(job.ID, req.OutputID)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "output not found in this job"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		output = resolvedOutput
	}

	if output == nil && emoji != nil {
		if resolvedOutput, err := h.resolveFeedbackOutputByJobAndObjectKey(job.ID, emoji.FileURL); err == nil {
			output = resolvedOutput
		}
	}
	if err := validateVideoJobFeedbackTarget(emoji, output); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	outputMetadata := datatypes.JSON([]byte("{}"))
	if len(output.Metadata) > 0 {
		outputMetadata = output.Metadata
	}
	outputContext := parseJSONMap(outputMetadata)
	sceneTag = resolveVideoImageFeedbackSceneTag(job.Metrics, outputMetadata)
	if sceneTag != "" {
		meta["scene_tag"] = sceneTag
	}
	if startSec := feedbackFloatFromAny(outputContext["start_sec"]); startSec > 0 {
		meta["output_start_sec"] = startSec
	}
	if endSec := feedbackFloatFromAny(outputContext["end_sec"]); endSec > 0 {
		meta["output_end_sec"] = endSec
	}
	if score := feedbackFloatFromAny(outputContext["score"]); score != 0 {
		meta["output_score"] = score
	}
	if windowIndex := int(feedbackFloatFromAny(outputContext["window_index"])); windowIndex > 0 {
		meta["output_window_index"] = windowIndex
	}
	if reason := strings.ToLower(strings.TrimSpace(feedbackStringFromAny(outputContext["reason"]))); reason != "" {
		meta["output_reason"] = reason
	}
	meta["output_id"] = output.ID
	meta["output_key"] = strings.TrimSpace(output.ObjectKey)
	meta["output_role"] = strings.TrimSpace(strings.ToLower(output.FileRole))
	if output.ProposalID != nil && *output.ProposalID > 0 {
		meta["proposal_id"] = *output.ProposalID
	}
	for key, value := range h.buildFeedbackTraceContext(job.ID, output.ID) {
		meta[key] = value
	}
	for key, value := range req.Metadata {
		meta[key] = value
	}
	meta["source"] = "manual_feedback_v1"
	meta["submitted_at"] = time.Now().Format(time.RFC3339)

	entry := models.VideoImageFeedbackPublic{
		JobID:     job.ID,
		UserID:    userID,
		Action:    action,
		Weight:    weight,
		SceneTag:  sceneTag,
		Metadata:  toJSON(meta),
		CreatedAt: time.Now(),
	}
	outID := output.ID
	entry.OutputID = &outID

	if action == videoImageFeedbackActionTopPick {
		if err := h.db.
			Where("job_id = ? AND user_id = ? AND LOWER(COALESCE(action, '')) = ?", job.ID, userID, videoImageFeedbackActionTopPick).
			Delete(&models.VideoImageFeedbackPublic{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := h.db.Create(&entry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.legacyFeedbackFallbackEnabled() {
		if signal := mapVideoImageActionToLegacySignal(action); signal != "" {
			h.bumpVideoJobFeedbackByCollectionID(*job.ResultCollectionID, signal, userID)
		}
	}

	resp := SubmitVideoJobFeedbackResponse{
		JobID:     job.ID,
		Action:    action,
		Weight:    weight,
		SceneTag:  sceneTag,
		CreatedAt: entry.CreatedAt.Format(time.RFC3339),
		Metadata:  meta,
	}
	if emoji != nil {
		resp.EmojiID = emoji.ID
	}
	resp.OutputID = *entry.OutputID
	c.JSON(http.StatusOK, resp)
}

func validateVideoJobFeedbackTarget(emoji *models.Emoji, output *models.VideoImageOutputPublic) error {
	if output == nil {
		return errors.New("output not found for feedback target")
	}
	if emoji == nil {
		return nil
	}
	emojiKey := strings.TrimSpace(emoji.FileURL)
	outputKey := strings.TrimSpace(output.ObjectKey)
	if emojiKey != "" && outputKey != "" && emojiKey != outputKey {
		return errors.New("emoji_id and output_id mismatch")
	}
	return nil
}

func (h *Handler) buildFeedbackTraceContext(jobID uint64, outputID uint64) map[string]interface{} {
	out := map[string]interface{}{}
	if h == nil || h.db == nil || jobID == 0 || outputID == 0 {
		return out
	}
	var evalRow struct {
		ID          uint64  `gorm:"column:id"`
		CandidateID *uint64 `gorm:"column:candidate_id"`
	}
	if err := h.db.Table("archive.video_job_gif_evaluations").
		Select("id", "candidate_id").
		Where("job_id = ? AND output_id = ?", jobID, outputID).
		Order("id DESC").
		Limit(1).
		Scan(&evalRow).Error; err == nil {
		if evalRow.ID > 0 {
			out["evaluation_id"] = evalRow.ID
		}
		if evalRow.CandidateID != nil && *evalRow.CandidateID > 0 {
			out["candidate_id"] = *evalRow.CandidateID
		}
	}
	var reviewRow struct {
		ID                  uint64  `gorm:"column:id"`
		ProposalID          *uint64 `gorm:"column:proposal_id"`
		FinalRecommendation string  `gorm:"column:final_recommendation"`
	}
	if err := h.db.Table("archive.video_job_gif_ai_reviews").
		Select("id", "proposal_id", "final_recommendation").
		Where("job_id = ? AND output_id = ?", jobID, outputID).
		Order("id DESC").
		Limit(1).
		Scan(&reviewRow).Error; err == nil {
		if reviewRow.ID > 0 {
			out["review_id"] = reviewRow.ID
		}
		if reviewRow.ProposalID != nil && *reviewRow.ProposalID > 0 {
			out["proposal_id"] = *reviewRow.ProposalID
		}
		recommendation := strings.TrimSpace(strings.ToLower(reviewRow.FinalRecommendation))
		if recommendation != "" {
			out["review_recommendation"] = recommendation
		}
	}
	return out
}
