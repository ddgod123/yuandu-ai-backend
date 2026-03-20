package handlers

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AdminBatchRerenderVideoJobGIFRequest struct {
	RequestID     string   `json:"request_id"`
	ProposalIDs   []uint64 `json:"proposal_ids"`
	ProposalRanks []int    `json:"proposal_ranks"`
	Strategy      string   `json:"strategy"`
	Force         bool     `json:"force"`
}

type AdminBatchRerenderVideoJobGIFItemResult struct {
	ProposalID   uint64                       `json:"proposal_id"`
	ProposalRank int                          `json:"proposal_rank"`
	Status       string                       `json:"status"`
	ErrorCode    string                       `json:"error_code,omitempty"`
	Error        string                       `json:"error,omitempty"`
	Result       *videojobs.GIFRerenderResult `json:"result,omitempty"`
}

type AdminBatchRerenderVideoJobGIFResponse struct {
	JobID      uint64                                    `json:"job_id"`
	RequestID  string                                    `json:"request_id"`
	Strategy   string                                    `json:"strategy"`
	Force      bool                                      `json:"force"`
	Total      int                                       `json:"total"`
	Succeeded  int                                       `json:"succeeded"`
	Failed     int                                       `json:"failed"`
	Idempotent bool                                      `json:"idempotent,omitempty"`
	Message    string                                    `json:"message,omitempty"`
	Items      []AdminBatchRerenderVideoJobGIFItemResult `json:"items"`
}

// AdminBatchRerenderVideoJobGIF godoc
// @Summary Batch rerender gif by proposals (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "job id"
// @Param body body AdminBatchRerenderVideoJobGIFRequest true "batch rerender request"
// @Success 200 {object} AdminBatchRerenderVideoJobGIFResponse
// @Router /api/admin/video-jobs/{id}/rerender-gif/batch [post]
func (h *Handler) AdminBatchRerenderVideoJobGIF(c *gin.Context) {
	jobID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req AdminBatchRerenderVideoJobGIFRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.ProposalIDs) == 0 && len(req.ProposalRanks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "proposal_ids or proposal_ranks required"})
		return
	}
	if len(req.ProposalIDs)+len(req.ProposalRanks) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many proposals"})
		return
	}

	strategy := normalizeBatchRerenderStrategy(req.Strategy)
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		requestID = strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	if requestID != "" {
		var duplicate int64
		if err := h.db.Model(&models.AuditLog{}).
			Where("target_type = ? AND target_id = ? AND action = ? AND meta ->> 'request_id' = ?", "video_job", jobID, "admin_rerender_gif_batch", requestID).
			Count(&duplicate).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if duplicate > 0 {
			c.JSON(http.StatusOK, AdminBatchRerenderVideoJobGIFResponse{
				JobID:      jobID,
				RequestID:  requestID,
				Strategy:   strategy,
				Force:      req.Force,
				Total:      0,
				Succeeded:  0,
				Failed:     0,
				Idempotent: true,
				Message:    "duplicate request_id ignored",
				Items:      []AdminBatchRerenderVideoJobGIFItemResult{},
			})
			return
		}
	}

	proposals, err := h.loadBatchRerenderProposals(jobID, req.ProposalIDs, req.ProposalRanks)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "proposal not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(proposals) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "proposal not found"})
		return
	}
	sortBatchRerenderProposals(proposals, strategy, req.ProposalIDs, req.ProposalRanks)

	actorID, _ := currentUserIDFromContext(c)
	processor := videojobs.NewProcessor(h.db, h.qiniu, h.cfg)
	results := make([]AdminBatchRerenderVideoJobGIFItemResult, 0, len(proposals))
	succeeded := 0
	failed := 0

	for _, proposal := range proposals {
		result, rerenderErr := processor.RerenderGIFByProposal(c.Request.Context(), videojobs.GIFRerenderRequest{
			JobID:      jobID,
			ProposalID: proposal.ID,
			Force:      req.Force,
			Trigger:    "admin_batch_api",
			ActorID:    actorID,
			ActorRole:  "admin",
		})
		itemResult := AdminBatchRerenderVideoJobGIFItemResult{
			ProposalID:   proposal.ID,
			ProposalRank: proposal.ProposalRank,
			Status:       "succeeded",
			Result:       result,
		}
		if rerenderErr != nil {
			itemResult.Status = "failed"
			itemResult.Error = rerenderErr.Error()
			var target *videojobs.GIFRerenderError
			if errors.As(rerenderErr, &target) && target != nil {
				itemResult.ErrorCode = strings.TrimSpace(target.Code)
			}
			failed++
		} else {
			succeeded++
		}
		results = append(results, itemResult)
	}

	h.recordAuditLog(actorID, "video_job", jobID, "admin_rerender_gif_batch", map[string]interface{}{
		"request_id":     requestID,
		"strategy":       strategy,
		"force":          req.Force,
		"proposal_ids":   req.ProposalIDs,
		"proposal_ranks": req.ProposalRanks,
		"total":          len(results),
		"succeeded":      succeeded,
		"failed":         failed,
		"results":        results,
	})

	c.JSON(http.StatusOK, AdminBatchRerenderVideoJobGIFResponse{
		JobID:      jobID,
		RequestID:  requestID,
		Strategy:   strategy,
		Force:      req.Force,
		Total:      len(results),
		Succeeded:  succeeded,
		Failed:     failed,
		Idempotent: false,
		Items:      results,
	})
}

func normalizeBatchRerenderStrategy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "loop_first":
		return "loop_first"
	case "size_first":
		return "size_first"
	case "clarity_first":
		return "clarity_first"
	case "viral_first":
		return "viral_first"
	default:
		return "default"
	}
}

func (h *Handler) loadBatchRerenderProposals(jobID uint64, proposalIDs []uint64, proposalRanks []int) ([]models.VideoJobGIFAIProposal, error) {
	idSet := make(map[uint64]struct{}, len(proposalIDs))
	ids := make([]uint64, 0, len(proposalIDs))
	for _, id := range proposalIDs {
		if id == 0 {
			continue
		}
		if _, exists := idSet[id]; exists {
			continue
		}
		idSet[id] = struct{}{}
		ids = append(ids, id)
	}
	rankSet := make(map[int]struct{}, len(proposalRanks))
	ranks := make([]int, 0, len(proposalRanks))
	for _, rank := range proposalRanks {
		if rank <= 0 {
			continue
		}
		if _, exists := rankSet[rank]; exists {
			continue
		}
		rankSet[rank] = struct{}{}
		ranks = append(ranks, rank)
	}
	if len(ids) == 0 && len(ranks) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	query := h.db.Where("job_id = ?", jobID)
	if len(ids) > 0 && len(ranks) > 0 {
		query = query.Where("id IN ? OR proposal_rank IN ?", ids, ranks)
	} else if len(ids) > 0 {
		query = query.Where("id IN ?", ids)
	} else {
		query = query.Where("proposal_rank IN ?", ranks)
	}

	var rows []models.VideoJobGIFAIProposal
	if err := query.Order("proposal_rank ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return rows, nil
}

func sortBatchRerenderProposals(
	rows []models.VideoJobGIFAIProposal,
	strategy string,
	proposalIDs []uint64,
	proposalRanks []int,
) {
	idOrder := map[uint64]int{}
	for idx, id := range proposalIDs {
		if id > 0 {
			idOrder[id] = idx
		}
	}
	rankOrder := map[int]int{}
	for idx, rank := range proposalRanks {
		if rank > 0 {
			rankOrder[rank] = idx
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		switch strategy {
		case "loop_first":
			if left.LoopFriendlinessHint != right.LoopFriendlinessHint {
				return left.LoopFriendlinessHint > right.LoopFriendlinessHint
			}
			if left.StandaloneConfidence != right.StandaloneConfidence {
				return left.StandaloneConfidence > right.StandaloneConfidence
			}
		case "size_first":
			if left.DurationSec != right.DurationSec {
				return left.DurationSec < right.DurationSec
			}
			if left.StandaloneConfidence != right.StandaloneConfidence {
				return left.StandaloneConfidence > right.StandaloneConfidence
			}
		case "clarity_first":
			if left.BaseScore != right.BaseScore {
				return left.BaseScore > right.BaseScore
			}
			if left.DurationSec != right.DurationSec {
				return left.DurationSec < right.DurationSec
			}
		case "viral_first":
			if left.StandaloneConfidence != right.StandaloneConfidence {
				return left.StandaloneConfidence > right.StandaloneConfidence
			}
			leftLevelWeight := expectedValueLevelSortWeight(left.ExpectedValueLevel)
			rightLevelWeight := expectedValueLevelSortWeight(right.ExpectedValueLevel)
			if leftLevelWeight != rightLevelWeight {
				return leftLevelWeight > rightLevelWeight
			}
		default:
			if order, ok := idOrder[left.ID]; ok {
				if rightOrder, rok := idOrder[right.ID]; rok && order != rightOrder {
					return order < rightOrder
				}
			}
			if order, ok := rankOrder[left.ProposalRank]; ok {
				if rightOrder, rok := rankOrder[right.ProposalRank]; rok && order != rightOrder {
					return order < rightOrder
				}
			}
		}
		if left.ProposalRank != right.ProposalRank {
			return left.ProposalRank < right.ProposalRank
		}
		return left.ID < right.ID
	})
}

func expectedValueLevelSortWeight(raw string) int {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}
