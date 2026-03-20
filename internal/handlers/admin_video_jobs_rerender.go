package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
)

type AdminRerenderVideoJobGIFRequest struct {
	ProposalID   uint64 `json:"proposal_id"`
	ProposalRank int    `json:"proposal_rank"`
	Force        bool   `json:"force"`
}

func (h *Handler) AdminRerenderVideoJobGIF(c *gin.Context) {
	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req AdminRerenderVideoJobGIFRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ProposalID == 0 && req.ProposalRank <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "proposal_id or proposal_rank required"})
		return
	}

	actorID, _ := currentUserIDFromContext(c)
	processor := videojobs.NewProcessor(h.db, h.qiniu, h.cfg)
	result, rerenderErr := processor.RerenderGIFByProposal(c.Request.Context(), videojobs.GIFRerenderRequest{
		JobID:        jobID,
		ProposalID:   req.ProposalID,
		ProposalRank: req.ProposalRank,
		Force:        req.Force,
		Trigger:      "admin_api",
		ActorID:      actorID,
		ActorRole:    "admin",
	})
	if rerenderErr != nil {
		statusCode := mapGIFRerenderHTTPStatus(rerenderErr)
		c.JSON(statusCode, gin.H{"error": rerenderErr.Error()})
		return
	}

	h.recordAuditLog(actorID, "video_job", jobID, "admin_rerender_gif", map[string]interface{}{
		"proposal_id":      result.ProposalID,
		"proposal_rank":    result.ProposalRank,
		"candidate_id":     result.CandidateID,
		"output_id":        result.OutputID,
		"emoji_id":         result.EmojiID,
		"artifact_id":      result.ArtifactID,
		"output_object":    result.OutputObjectKey,
		"display_order":    result.DisplayOrder,
		"window_start_sec": result.WindowStartSec,
		"window_end_sec":   result.WindowEndSec,
		"size_bytes":       result.SizeBytes,
		"zip_invalidated":  result.ZipInvalidated,
		"cost_before_cny":  result.CostBeforeCNY,
		"cost_after_cny":   result.CostAfterCNY,
		"cost_delta_cny":   result.CostDeltaCNY,
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "ok",
		"result":  result,
	})
}

func mapGIFRerenderHTTPStatus(err error) int {
	var target *videojobs.GIFRerenderError
	if !errors.As(err, &target) || target == nil {
		return http.StatusInternalServerError
	}
	switch strings.TrimSpace(target.Code) {
	case videojobs.GIFRerenderErrorInvalidInput:
		return http.StatusBadRequest
	case videojobs.GIFRerenderErrorJobNotFound, videojobs.GIFRerenderErrorCollectionMiss, videojobs.GIFRerenderErrorProposalNotFound:
		return http.StatusNotFound
	case videojobs.GIFRerenderErrorJobNotDone, videojobs.GIFRerenderErrorSourceDeleted, videojobs.GIFRerenderErrorSourceMissing, videojobs.GIFRerenderErrorAlreadyRendered:
		return http.StatusConflict
	case videojobs.GIFRerenderErrorRenderFailed:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}
