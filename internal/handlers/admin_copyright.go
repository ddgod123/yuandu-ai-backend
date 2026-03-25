package handlers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/copyrightjobs"
	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AdminCreateCopyrightTaskRequest struct {
	CollectionID          uint64   `json:"collectionId"`
	RunMode               string   `json:"runMode"`
	SampleStrategy        string   `json:"sampleStrategy"`
	EnableTagging         *bool    `json:"enableTagging"`
	OverwriteMachineTags  *bool    `json:"overwriteMachineTags"`
	Targets               []string `json:"targets"`
	AutoSubmitReview      *bool    `json:"autoSubmitReview"`
	SuggestUpgradeFullRun *bool    `json:"suggestUpgradeFullRun"`
}

type AdminCreateCopyrightTaskResponse struct {
	TaskID uint64 `json:"taskId"`
	TaskNo string `json:"taskNo"`
}

type AdminCopyrightTaskListResponse struct {
	Items    []models.CollectionCopyrightTask `json:"items"`
	Total    int64                            `json:"total"`
	Page     int                              `json:"page"`
	PageSize int                              `json:"page_size"`
}

type AdminCopyrightCollectionsListItem struct {
	CollectionID            uint64     `json:"collectionId"`
	CollectionName          string     `json:"collectionName"`
	CoverURL                string     `json:"coverUrl"`
	ImageCount              int64      `json:"imageCount"`
	SourceChannel           string     `json:"sourceChannel"`
	LatestTaskID            *uint64    `json:"latestTaskId,omitempty"`
	LatestRunMode           string     `json:"latestRunMode,omitempty"`
	LatestMachineConclusion string     `json:"latestMachineConclusion,omitempty"`
	LatestRiskLevel         string     `json:"latestRiskLevel,omitempty"`
	ReviewStatus            string     `json:"reviewStatus,omitempty"`
	UpdatedAt               *time.Time `json:"updatedAt,omitempty"`
}

type AdminCopyrightCollectionsListResponse struct {
	Items    []AdminCopyrightCollectionsListItem `json:"items"`
	Total    int64                               `json:"total"`
	Page     int                                 `json:"page"`
	PageSize int                                 `json:"page_size"`
}

type AdminTagDefinitionRequest struct {
	TagCode       string `json:"tagCode"`
	TagName       string `json:"tagName"`
	DimensionCode string `json:"dimensionCode"`
	TagLevel      string `json:"tagLevel"`
	IsSystem      *bool  `json:"isSystem"`
	SortNo        int    `json:"sortNo"`
	Status        *int16 `json:"status"`
	Remark        string `json:"remark"`
}

type AdminCopyrightReviewRequest struct {
	TaskID         *uint64  `json:"taskId"`
	CollectionID   uint64   `json:"collectionId"`
	EmojiID        *uint64  `json:"emojiId"`
	ReviewType     string   `json:"reviewType"`
	ReviewResult   string   `json:"reviewResult"`
	ReviewComment  string   `json:"reviewComment"`
	AttachmentURLs []string `json:"attachmentUrls"`
}

type AdminUpdateImageTagsRequest struct {
	AddTagIDs    []uint64 `json:"addTagIds"`
	RemoveTagIDs []uint64 `json:"removeTagIds"`
}

func (h *Handler) CreateAdminCopyrightTask(c *gin.Context) {
	var req AdminCreateCopyrightTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.CollectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collectionId required"})
		return
	}

	runMode := normalizeRunMode(req.RunMode)
	if runMode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "runMode must be first/five/all"})
		return
	}
	sampleStrategy := normalizeSampleStrategy(req.SampleStrategy)
	if sampleStrategy == "" {
		sampleStrategy = defaultSampleStrategy(runMode)
	}
	if !sampleStrategyAllowed(runMode, sampleStrategy) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sampleStrategy not allowed for runMode"})
		return
	}
	enableTagging := true
	if req.EnableTagging != nil {
		enableTagging = *req.EnableTagging
	}
	overwriteMachineTags := true
	if req.OverwriteMachineTags != nil {
		overwriteMachineTags = *req.OverwriteMachineTags
	}

	adminID, _ := currentUserIDFromContext(c)
	var collection models.Collection
	if err := h.db.Select("id").First(&collection, req.CollectionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	taskNo := buildCopyrightTaskNo()
	now := time.Now()
	var createdTaskID uint64

	err := h.db.Transaction(func(tx *gorm.DB) error {
		var emojis []models.Emoji
		if err := tx.Where("collection_id = ?", req.CollectionID).
			Order("display_order ASC, id ASC").
			Find(&emojis).Error; err != nil {
			return err
		}

		sampled := sampleEmojis(emojis, runMode, sampleStrategy)
		sampleCount := expectedSampleCount(len(emojis), runMode)

		task := models.CollectionCopyrightTask{
			TaskNo:               taskNo,
			CollectionID:         req.CollectionID,
			RunMode:              runMode,
			SampleStrategy:       sampleStrategy,
			SampleCount:          sampleCount,
			ActualSampleCount:    len(sampled),
			EnableTagging:        enableTagging,
			OverwriteMachineTags: overwriteMachineTags,
			Status:               "pending",
			Progress:             0,
			CreatedBy:            nullableUint64(adminID),
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		createdTaskID = task.ID

		rows := make([]models.CollectionCopyrightTaskImage, 0, len(sampled))
		for idx, e := range sampled {
			rows = append(rows, models.CollectionCopyrightTaskImage{
				TaskID:       task.ID,
				CollectionID: req.CollectionID,
				EmojiID:      e.ID,
				SampleOrder:  idx + 1,
				Status:       "pending",
				ErrorMsg:     "",
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}

		coverage := 0.0
		if len(emojis) > 0 {
			coverage = float64(len(sampled)) * 100 / float64(len(emojis))
		}

		summary := models.CollectionCopyrightResult{
			CollectionID:       req.CollectionID,
			LatestTaskID:       task.ID,
			RunMode:            runMode,
			SampleCoverage:     round2(coverage),
			MachineConclusion:  "",
			MachineConfidence:  0,
			RiskLevel:          "L1",
			SampledImageCount:  len(sampled),
			HighRiskCount:      0,
			UnknownSourceCount: 0,
			IPHitCount:         0,
			BrandHitCount:      0,
			RecommendedAction:  "pending_process",
			ReviewStatus:       "unreviewed",
			FinalDecision:      "",
			Summary:            "任务已创建，待识别执行",
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "collection_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"latest_task_id":      summary.LatestTaskID,
				"run_mode":            summary.RunMode,
				"sample_coverage":     summary.SampleCoverage,
				"sampled_image_count": summary.SampledImageCount,
				"recommended_action":  summary.RecommendedAction,
				"review_status":       summary.ReviewStatus,
				"summary":             summary.Summary,
				"updated_at":          now,
			}),
		}).Create(&summary).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, AdminCreateCopyrightTaskResponse{TaskID: createdTaskID, TaskNo: taskNo})

	if createdTaskID > 0 && h.queue != nil {
		if enqueueTask, err := copyrightjobs.NewProcessCollectionCopyrightTask(createdTaskID); err == nil {
			_, _ = h.queue.Enqueue(enqueueTask, asynq.Queue("default"))
		}
	}
}

func (h *Handler) ListAdminCopyrightTasks(c *gin.Context) {
	page := parseIntQuery(c, "page", 1)
	pageSize := parseIntQuery(c, "page_size", 20)
	if pageSize > 200 {
		pageSize = 200
	}

	query := h.db.Model(&models.CollectionCopyrightTask{})
	if collectionID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("collection_id")), 10, 64); collectionID > 0 {
		query = query.Where("collection_id = ?", collectionID)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if runMode := normalizeRunMode(c.Query("run_mode")); runMode != "" {
		query = query.Where("run_mode = ?", runMode)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var items []models.CollectionCopyrightTask
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, AdminCopyrightTaskListResponse{Items: items, Total: total, Page: page, PageSize: pageSize})
}

func (h *Handler) GetAdminCopyrightTask(c *gin.Context) {
	taskID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || taskID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var task models.CollectionCopyrightTask
	if err := h.db.First(&task, taskID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var sampleCount int64
	_ = h.db.Model(&models.CollectionCopyrightTaskImage{}).Where("task_id = ?", taskID).Count(&sampleCount).Error
	c.JSON(http.StatusOK, gin.H{
		"task":           task,
		"sampleCount":    sampleCount,
		"processedCount": 0,
	})
}

func (h *Handler) ListAdminCopyrightCollections(c *gin.Context) {
	page := parseIntQuery(c, "page", 1)
	pageSize := parseIntQuery(c, "page_size", 20)
	if pageSize > 200 {
		pageSize = 200
	}

	query := h.db.Model(&models.Collection{})
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		lower := strings.ToLower(keyword)
		query = query.Where("LOWER(title) LIKE ?", "%"+lower+"%")
	}
	if source := strings.TrimSpace(c.Query("source")); source != "" {
		query = query.Where("source = ?", source)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var collections []models.Collection
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&collections).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ids := make([]uint64, 0, len(collections))
	for _, col := range collections {
		ids = append(ids, col.ID)
	}

	summaryMap := map[uint64]models.CollectionCopyrightResult{}
	if len(ids) > 0 {
		var summaries []models.CollectionCopyrightResult
		if err := h.db.Where("collection_id IN ?", ids).Find(&summaries).Error; err == nil {
			for _, s := range summaries {
				summaryMap[s.CollectionID] = s
			}
		}
	}

	items := make([]AdminCopyrightCollectionsListItem, 0, len(collections))
	for _, col := range collections {
		entry := AdminCopyrightCollectionsListItem{
			CollectionID:   col.ID,
			CollectionName: col.Title,
			CoverURL:       col.CoverURL,
			ImageCount:     int64(col.FileCount),
			SourceChannel:  col.Source,
			UpdatedAt:      &col.UpdatedAt,
		}
		if s, ok := summaryMap[col.ID]; ok {
			entry.LatestTaskID = &s.LatestTaskID
			entry.LatestRunMode = s.RunMode
			entry.LatestMachineConclusion = s.MachineConclusion
			entry.LatestRiskLevel = s.RiskLevel
			entry.ReviewStatus = s.ReviewStatus
			entry.UpdatedAt = &s.UpdatedAt
		}
		items = append(items, entry)
	}

	c.JSON(http.StatusOK, AdminCopyrightCollectionsListResponse{Items: items, Total: total, Page: page, PageSize: pageSize})
}

func (h *Handler) GetAdminCopyrightCollection(c *gin.Context) {
	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var collection models.Collection
	if err := h.db.First(&collection, collectionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var summary models.CollectionCopyrightResult
	_ = h.db.Where("collection_id = ?", collectionID).First(&summary).Error

	var latestTask models.CollectionCopyrightTask
	if summary.LatestTaskID > 0 {
		_ = h.db.First(&latestTask, summary.LatestTaskID).Error
	}

	c.JSON(http.StatusOK, gin.H{
		"collection": collection,
		"summary":    summary,
		"latestTask": latestTask,
	})
}

func (h *Handler) ListAdminCopyrightCollectionImages(c *gin.Context) {
	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	page := parseIntQuery(c, "page", 1)
	pageSize := parseIntQuery(c, "page_size", 20)
	if pageSize > 200 {
		pageSize = 200
	}

	query := h.db.Model(&models.ImageCopyrightResult{}).Where("collection_id = ?", collectionID)
	if taskID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("task_id")), 10, 64); taskID > 0 {
		query = query.Where("task_id = ?", taskID)
	}
	if riskLevel := strings.TrimSpace(c.Query("risk_level")); riskLevel != "" {
		query = query.Where("risk_level = ?", riskLevel)
	}
	if rightsStatus := strings.TrimSpace(c.Query("rights_status")); rightsStatus != "" {
		query = query.Where("rights_status = ?", rightsStatus)
	}
	if isCommercialIP := strings.TrimSpace(c.Query("is_commercial_ip")); isCommercialIP != "" {
		if parsed, ok := parseOptionalBoolParam(isCommercialIP); ok && parsed != nil {
			query = query.Where("is_commercial_ip = ?", *parsed)
		}
	}
	if isBrandRelated := strings.TrimSpace(c.Query("is_brand_related")); isBrandRelated != "" {
		if parsed, ok := parseOptionalBoolParam(isBrandRelated); ok && parsed != nil {
			query = query.Where("is_brand_related = ?", *parsed)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var items []models.ImageCopyrightResult
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *Handler) GetAdminCopyrightImageDetail(c *gin.Context) {
	emojiID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || emojiID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var emoji models.Emoji
	if err := h.db.First(&emoji, emojiID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	query := h.db.Model(&models.ImageCopyrightResult{}).Where("emoji_id = ?", emojiID)
	if taskID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("task_id")), 10, 64); taskID > 0 {
		query = query.Where("task_id = ?", taskID)
	}
	var result models.ImageCopyrightResult
	_ = query.Order("id DESC").First(&result).Error

	type tagRow struct {
		ID            uint64  `json:"id"`
		TagID         uint64  `json:"tagId"`
		TagCode       string  `json:"tagCode"`
		TagName       string  `json:"tagName"`
		DimensionCode string  `json:"dimensionCode"`
		Source        string  `json:"source"`
		Confidence    float64 `json:"confidence"`
		ModelVersion  string  `json:"modelVersion"`
	}
	var tags []tagRow
	_ = h.db.Table("taxonomy.emoji_auto_tags eat").
		Select("eat.id, eat.tag_id, td.tag_code, td.tag_name, td.dimension_code, eat.source, eat.confidence, eat.model_version").
		Joins("LEFT JOIN taxonomy.tag_definitions td ON td.id = eat.tag_id").
		Where("eat.emoji_id = ? AND eat.status = 1", emojiID).
		Order("eat.id DESC").
		Scan(&tags).Error

	var evidences []models.CopyrightEvidence
	_ = h.db.Where("emoji_id = ?", emojiID).Order("id DESC").Find(&evidences).Error

	var reviews []models.CopyrightReviewRecord
	_ = h.db.Where("emoji_id = ?", emojiID).Order("id DESC").Find(&reviews).Error

	c.JSON(http.StatusOK, gin.H{
		"emoji":     emoji,
		"result":    result,
		"tags":      tags,
		"evidences": evidences,
		"reviews":   reviews,
	})
}

func (h *Handler) UpdateAdminCopyrightImageTags(c *gin.Context) {
	emojiID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || emojiID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req AdminUpdateImageTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var emoji models.Emoji
	if err := h.db.Select("id, collection_id").First(&emoji, emojiID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	adminID, _ := currentUserIDFromContext(c)
	now := time.Now()

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if len(req.RemoveTagIDs) > 0 {
			if err := tx.Where("emoji_id = ? AND tag_id IN ? AND source = ?", emojiID, req.RemoveTagIDs, "manual").
				Delete(&models.EmojiAutoTag{}).Error; err != nil {
				return err
			}
		}
		if len(req.AddTagIDs) > 0 {
			var exists []models.EmojiAutoTag
			if err := tx.Where("emoji_id = ? AND tag_id IN ? AND source = ?", emojiID, req.AddTagIDs, "manual").
				Find(&exists).Error; err != nil {
				return err
			}
			existing := make(map[uint64]struct{}, len(exists))
			for _, item := range exists {
				existing[item.TagID] = struct{}{}
			}
			rows := make([]models.EmojiAutoTag, 0, len(req.AddTagIDs))
			for _, tagID := range req.AddTagIDs {
				if tagID == 0 {
					continue
				}
				if _, ok := existing[tagID]; ok {
					continue
				}
				rows = append(rows, models.EmojiAutoTag{
					EmojiID:      emojiID,
					CollectionID: emoji.CollectionID,
					TagID:        tagID,
					Source:       "manual",
					Confidence:   1,
					ModelVersion: "",
					Status:       1,
					CreatedBy:    nullableUint64(adminID),
					CreatedAt:    now,
					UpdatedAt:    now,
				})
			}
			if len(rows) > 0 {
				if err := tx.Create(&rows).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "tags updated"})
}

func (h *Handler) ListAdminCopyrightTaskLogs(c *gin.Context) {
	taskID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || taskID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	page := parseIntQuery(c, "page", 1)
	pageSize := parseIntQuery(c, "page_size", 50)
	if pageSize > 500 {
		pageSize = 500
	}

	query := h.db.Model(&models.CopyrightTaskLog{}).Where("task_id = ?", taskID)
	if stage := strings.TrimSpace(c.Query("stage")); stage != "" {
		query = query.Where("stage = ?", stage)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var items []models.CopyrightTaskLog
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *Handler) ListAdminCopyrightPendingReviews(c *gin.Context) {
	page := parseIntQuery(c, "page", 1)
	pageSize := parseIntQuery(c, "page_size", 20)
	if pageSize > 200 {
		pageSize = 200
	}
	query := h.db.Model(&models.CopyrightReviewRecord{}).Where("review_status = ?", "pending")
	if reviewType := strings.TrimSpace(c.Query("review_type")); reviewType != "" {
		query = query.Where("review_type = ?", reviewType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var items []models.CopyrightReviewRecord
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *Handler) SubmitAdminCopyrightReview(c *gin.Context) {
	var req AdminCopyrightReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.CollectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collectionId required"})
		return
	}
	reviewType := strings.ToLower(strings.TrimSpace(req.ReviewType))
	if reviewType != "collection" && reviewType != "image" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reviewType must be collection/image"})
		return
	}
	reviewResult := strings.TrimSpace(req.ReviewResult)
	if reviewResult == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reviewResult required"})
		return
	}
	reviewComment := strings.TrimSpace(req.ReviewComment)
	if reviewComment == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reviewComment required"})
		return
	}
	reviewerID, ok := currentUserIDFromContext(c)
	if !ok || reviewerID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	now := time.Now()
	err := h.db.Transaction(func(tx *gorm.DB) error {
		record := models.CopyrightReviewRecord{
			CollectionID:  req.CollectionID,
			EmojiID:       req.EmojiID,
			TaskID:        req.TaskID,
			ReviewType:    reviewType,
			ReviewStatus:  "reviewed",
			ReviewResult:  reviewResult,
			ReviewComment: reviewComment,
			ReviewerID:    reviewerID,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}

		update := map[string]interface{}{
			"review_status":     "reviewed",
			"final_decision":    reviewResult,
			"final_reviewer_id": reviewerID,
			"final_reviewed_at": now,
			"updated_at":        now,
		}
		if err := tx.Model(&models.CollectionCopyrightResult{}).
			Where("collection_id = ?", req.CollectionID).
			Updates(update).Error; err != nil {
			return err
		}

		if len(req.AttachmentURLs) > 0 {
			for _, url := range req.AttachmentURLs {
				u := strings.TrimSpace(url)
				if u == "" {
					continue
				}
				e := models.CopyrightEvidence{
					CollectionID:  req.CollectionID,
					EmojiID:       req.EmojiID,
					TaskID:        req.TaskID,
					EvidenceType:  "manual_doc",
					EvidenceTitle: "review_attachment",
					EvidenceURL:   u,
					CreatedAt:     now,
					UpdatedAt:     now,
				}
				if err := tx.Create(&e).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "review submitted"})
}

func (h *Handler) ListAdminTagDimensions(c *gin.Context) {
	var dims []models.TagDimension
	if err := h.db.Order("sort_no ASC, id ASC").Find(&dims).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dims)
}

func (h *Handler) ListAdminTagDefinitions(c *gin.Context) {
	page := parseIntQuery(c, "page", 1)
	pageSize := parseIntQuery(c, "page_size", 50)
	if pageSize > 200 {
		pageSize = 200
	}
	query := h.db.Model(&models.TagDefinition{})
	if dimension := strings.TrimSpace(c.Query("dimension_code")); dimension != "" {
		query = query.Where("dimension_code = ?", dimension)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		if v, err := strconv.Atoi(status); err == nil {
			query = query.Where("status = ?", v)
		}
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		lower := strings.ToLower(keyword)
		query = query.Where("LOWER(tag_name) LIKE ? OR LOWER(tag_code) LIKE ?", "%"+lower+"%", "%"+lower+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var items []models.TagDefinition
	offset := (page - 1) * pageSize
	if err := query.Order("sort_no ASC, id DESC").Limit(pageSize).Offset(offset).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *Handler) CreateAdminTagDefinition(c *gin.Context) {
	var req AdminTagDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tagCode := strings.TrimSpace(req.TagCode)
	tagName := strings.TrimSpace(req.TagName)
	dimensionCode := strings.TrimSpace(req.DimensionCode)
	if tagCode == "" || tagName == "" || dimensionCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tagCode/tagName/dimensionCode required"})
		return
	}
	tagLevel := strings.TrimSpace(req.TagLevel)
	if tagLevel == "" {
		tagLevel = "image"
	}
	if tagLevel != "image" && tagLevel != "collection" && tagLevel != "both" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tagLevel must be image/collection/both"})
		return
	}
	isSystem := false
	if req.IsSystem != nil {
		isSystem = *req.IsSystem
	}
	status := int16(1)
	if req.Status != nil {
		status = *req.Status
	}
	row := models.TagDefinition{
		TagCode:       tagCode,
		TagName:       tagName,
		DimensionCode: dimensionCode,
		TagLevel:      tagLevel,
		IsSystem:      isSystem,
		SortNo:        req.SortNo,
		Status:        status,
		Remark:        strings.TrimSpace(req.Remark),
	}
	if err := h.db.Create(&row).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, row)
}

func (h *Handler) UpdateAdminTagDefinition(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req AdminTagDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var row models.TagDefinition
	if err := h.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if v := strings.TrimSpace(req.TagName); v != "" {
		row.TagName = v
	}
	if v := strings.TrimSpace(req.DimensionCode); v != "" {
		row.DimensionCode = v
	}
	if v := strings.TrimSpace(req.TagLevel); v != "" {
		if v != "image" && v != "collection" && v != "both" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tagLevel must be image/collection/both"})
			return
		}
		row.TagLevel = v
	}
	if req.IsSystem != nil {
		row.IsSystem = *req.IsSystem
	}
	if req.Status != nil {
		row.Status = *req.Status
	}
	row.SortNo = req.SortNo
	row.Remark = strings.TrimSpace(req.Remark)
	if err := h.db.Save(&row).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, row)
}

func normalizeRunMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "first":
		return "first"
	case "five":
		return "five"
	case "all":
		return "all"
	default:
		return ""
	}
}

func normalizeSampleStrategy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "first":
		return "first"
	case "even":
		return "even"
	case "random":
		return "random"
	case "all":
		return "all"
	default:
		return ""
	}
}

func defaultSampleStrategy(runMode string) string {
	switch runMode {
	case "first":
		return "first"
	case "five":
		return "even"
	default:
		return "all"
	}
}

func sampleStrategyAllowed(runMode, strategy string) bool {
	switch runMode {
	case "first":
		return strategy == "first"
	case "five":
		return strategy == "even" || strategy == "random" || strategy == "first"
	case "all":
		return strategy == "all"
	default:
		return false
	}
}

func expectedSampleCount(total int, runMode string) int {
	switch runMode {
	case "first":
		if total > 0 {
			return 1
		}
		return 0
	case "five":
		if total < 5 {
			return total
		}
		return 5
	default:
		return total
	}
}

func sampleEmojis(items []models.Emoji, runMode, strategy string) []models.Emoji {
	total := len(items)
	if total == 0 {
		return nil
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].DisplayOrder == items[j].DisplayOrder {
			return items[i].ID < items[j].ID
		}
		return items[i].DisplayOrder < items[j].DisplayOrder
	})

	switch runMode {
	case "first":
		return items[:1]
	case "five":
		target := 5
		if total <= target {
			return items
		}
		switch strategy {
		case "first":
			return items[:target]
		case "random":
			perm := []int{0, total / 4, total / 2, (total * 3) / 4, total - 1}
			res := make([]models.Emoji, 0, len(perm))
			seen := map[int]struct{}{}
			for _, idx := range perm {
				if idx < 0 || idx >= total {
					continue
				}
				if _, ok := seen[idx]; ok {
					continue
				}
				seen[idx] = struct{}{}
				res = append(res, items[idx])
			}
			for i := 0; len(res) < target && i < total; i++ {
				if _, ok := seen[i]; ok {
					continue
				}
				res = append(res, items[i])
			}
			return res
		default:
			fallthrough
		case "even":
			res := make([]models.Emoji, 0, target)
			for i := 0; i < target; i++ {
				idx := int(math.Round(float64(i) * float64(total-1) / float64(target-1)))
				res = append(res, items[idx])
			}
			return res
		}
	default:
		return items
	}
}

func buildCopyrightTaskNo() string {
	return fmt.Sprintf("CP%s%03d", time.Now().Format("20060102150405"), time.Now().UnixNano()%1000)
}

func nullableUint64(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
