package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	gpuTestCallbackTokenHeader = "X-GPU-CALLBACK-TOKEN"
	gpuTestDefaultModel        = "realesrgan-x4plus"
	gpuTestDefaultScale        = 4
	gpuTestMaxDispatchBodySize = 1 << 20 // 1 MiB
)

type createGPUImageEnhanceJobRequest struct {
	Title           string `json:"title"`
	SourceObjectKey string `json:"source_object_key" binding:"required"`
	SourceMimeType  string `json:"source_mime_type,omitempty"`
	SourceSizeBytes int64  `json:"source_size_bytes,omitempty"`
	Model           string `json:"model,omitempty"`
	Scale           int    `json:"scale,omitempty"`
}

type gpuImageEnhanceCallbackRequest struct {
	Status          string                 `json:"status"`
	Stage           string                 `json:"stage,omitempty"`
	Progress        *int                   `json:"progress,omitempty"`
	ResultObjectKey string                 `json:"result_object_key,omitempty"`
	ResultKey       string                 `json:"result_key,omitempty"`
	ResultMimeType  string                 `json:"result_mime_type,omitempty"`
	ResultSizeBytes int64                  `json:"result_size_bytes,omitempty"`
	Width           int                    `json:"width,omitempty"`
	Height          int                    `json:"height,omitempty"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	ElapsedMS       int64                  `json:"elapsed_ms,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type gpuImageEnhanceDispatchRequest struct {
	JobID           uint64                 `json:"job_id"`
	UserID          uint64                 `json:"user_id"`
	Provider        string                 `json:"provider"`
	Model           string                 `json:"model"`
	Scale           int                    `json:"scale"`
	SourceObjectKey string                 `json:"source_object_key"`
	SourceURL       string                 `json:"source_url"`
	ResultObjectKey string                 `json:"result_object_key"`
	CallbackURL     string                 `json:"callback_url"`
	CallbackToken   string                 `json:"callback_token,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type gpuImageEnhanceAssetResponse struct {
	ID        uint64                 `json:"id"`
	AssetRole string                 `json:"asset_role"`
	ObjectKey string                 `json:"object_key"`
	ObjectURL string                 `json:"object_url,omitempty"`
	MimeType  string                 `json:"mime_type,omitempty"`
	SizeBytes int64                  `json:"size_bytes"`
	Width     int                    `json:"width,omitempty"`
	Height    int                    `json:"height,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type gpuImageEnhanceJobResponse struct {
	ID              uint64                         `json:"id"`
	Title           string                         `json:"title"`
	Provider        string                         `json:"provider"`
	Model           string                         `json:"model"`
	Scale           int                            `json:"scale"`
	Status          string                         `json:"status"`
	Stage           string                         `json:"stage"`
	Progress        int                            `json:"progress"`
	ErrorMessage    string                         `json:"error_message,omitempty"`
	SourceObjectKey string                         `json:"source_object_key"`
	SourceObjectURL string                         `json:"source_object_url,omitempty"`
	ResultObjectKey string                         `json:"result_object_key,omitempty"`
	ResultObjectURL string                         `json:"result_object_url,omitempty"`
	QueuedAt        time.Time                      `json:"queued_at"`
	StartedAt       *time.Time                     `json:"started_at,omitempty"`
	FinishedAt      *time.Time                     `json:"finished_at,omitempty"`
	CreatedAt       time.Time                      `json:"created_at"`
	UpdatedAt       time.Time                      `json:"updated_at"`
	Assets          []gpuImageEnhanceAssetResponse `json:"assets,omitempty"`
}

type listGPUImageEnhanceJobsResponse struct {
	Items    []gpuImageEnhanceJobResponse `json:"items"`
	Page     int                          `json:"page"`
	PageSize int                          `json:"page_size"`
	Total    int64                        `json:"total"`
	HasMore  bool                         `json:"has_more"`
}

type batchDeleteGPUImageEnhanceJobsRequest struct {
	IDs []uint64 `json:"ids" binding:"required"`
}

func (h *Handler) GetGPUTestUploadToken(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req UploadTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userSourcePrefix := h.qiniuUserGPUTestSourcePrefix(userID)
	legacySourcePrefix := h.qiniuLegacyUserGPUTestSourcePrefix(userID)
	key := strings.TrimLeft(strings.TrimSpace(req.Key), "/")
	prefix := strings.TrimLeft(strings.TrimSpace(req.Prefix), "/")

	if key == "" && prefix == "" {
		prefix = userSourcePrefix
	}
	if key != "" && !hasAnyPrefix(key, userSourcePrefix, legacySourcePrefix) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden key"})
		return
	}
	if prefix != "" && !hasAnyPrefix(prefix, userSourcePrefix, legacySourcePrefix) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden key"})
		return
	}

	resp := issueQiniuUploadToken(h, key, prefix, normalizeUploadTokenExpire(req.Expires, h.qiniu.SignTTL), true)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) CreateGPUImageEnhanceJob(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}

	var req createGPUImageEnhanceJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sourceKey := normalizeStorageObjectKey(req.SourceObjectKey)
	if sourceKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_object_key is required"})
		return
	}
	if !hasAnyPrefix(sourceKey, h.qiniuUserGPUTestSourcePrefix(userID), h.qiniuLegacyUserGPUTestSourcePrefix(userID)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden source_object_key"})
		return
	}
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(sourceKey)))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_object_key must be an image (.png/.jpg/.jpeg/.webp)"})
		return
	}
	sourceMimeType := strings.TrimSpace(strings.ToLower(req.SourceMimeType))
	if sourceMimeType != "" && !strings.HasPrefix(sourceMimeType, "image/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_mime_type must start with image/"})
		return
	}

	scale := req.Scale
	if scale == 0 {
		scale = gpuTestDefaultScale
	}
	if scale < 1 || scale > 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scale must be within 1..8"})
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = defaultGPUJobTitle(sourceKey)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = gpuTestDefaultModel
	}

	sourceURL, err := h.resolveGPUObjectURLByKey(sourceKey, 1800)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to build source_object_url"})
		return
	}

	resultKey := buildGPUTestResultObjectKey(h.qiniuUserGPUTestResultPrefix(userID), sourceKey)

	job := models.GPUImageEnhanceJob{
		UserID:          userID,
		Title:           title,
		Provider:        "realesrgan",
		Model:           model,
		Scale:           scale,
		SourceObjectKey: sourceKey,
		SourceMimeType:  sourceMimeType,
		SourceSizeBytes: clampInt64Min(req.SourceSizeBytes, 0),
		ResultObjectKey: resultKey,
		Status:          models.GPUImageEnhanceStatusQueued,
		Stage:           models.GPUImageEnhanceStatusQueued,
		Progress:        0,
		RequestPayload:  gpuEmptyJSON(),
		CallbackPayload: gpuEmptyJSON(),
		Metadata:        gpuEmptyJSON(),
		QueuedAt:        time.Now(),
	}
	sourceAsset := models.GPUImageEnhanceAsset{
		UserID:    userID,
		AssetRole: models.GPUImageEnhanceAssetRoleSource,
		ObjectKey: sourceKey,
		MimeType:  sourceMimeType,
		SizeBytes: clampInt64Min(req.SourceSizeBytes, 0),
		Metadata:  gpuEmptyJSON(),
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		sourceAsset.JobID = job.ID
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "job_id"}, {Name: "asset_role"}},
			DoUpdates: clause.AssignmentColumns([]string{"object_key", "mime_type", "size_bytes", "metadata", "updated_at"}),
		}).Create(&sourceAsset).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	callbackURL, cbErr := h.buildGPUCallbackURL(c, job.ID)
	if cbErr != nil {
		finishedAt := time.Now()
		_ = h.db.Model(&models.GPUImageEnhanceJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
			"status":        models.GPUImageEnhanceStatusFailed,
			"stage":         models.GPUImageEnhanceStatusFailed,
			"progress":      100,
			"error_message": "callback url unresolved: " + cbErr.Error(),
			"finished_at":   &finishedAt,
			"updated_at":    finishedAt,
		}).Error
		updated, _ := h.getGPUImageEnhanceJobWithAssets(job.ID, userID)
		c.JSON(http.StatusOK, updated)
		return
	}

	dispatchReq := gpuImageEnhanceDispatchRequest{
		JobID:           job.ID,
		UserID:          userID,
		Provider:        job.Provider,
		Model:           model,
		Scale:           scale,
		SourceObjectKey: sourceKey,
		SourceURL:       sourceURL,
		ResultObjectKey: resultKey,
		CallbackURL:     callbackURL,
		CallbackToken:   strings.TrimSpace(h.cfg.GPUCallbackToken),
		Metadata: map[string]interface{}{
			"title": title,
		},
	}
	requestPayload := gpuMustMarshalJSON(dispatchReq)

	dispatchTimeout := h.cfg.GPURequestTimeoutSec
	if dispatchTimeout <= 0 {
		dispatchTimeout = 20
	}
	dispatchCtx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(dispatchTimeout)*time.Second)
	defer cancel()

	statusCode, responseText, dispatchErr := h.dispatchGPUImageEnhanceJob(dispatchCtx, dispatchReq)
	now := time.Now()
	dispatchMeta := map[string]interface{}{
		"dispatch_http_status": statusCode,
		"dispatch_response":    responseText,
	}

	if dispatchErr != nil {
		errText := strings.TrimSpace(dispatchErr.Error())
		if errText == "" {
			errText = "dispatch failed"
		}
		if len(responseText) > 0 {
			errText = errText + " · " + strings.TrimSpace(responseText)
		}
		_ = h.db.Model(&models.GPUImageEnhanceJob{}).
			Where("id = ? AND status IN ?", job.ID, []string{
				models.GPUImageEnhanceStatusQueued,
				models.GPUImageEnhanceStatusRunning,
			}).
			Updates(map[string]interface{}{
				"status":          models.GPUImageEnhanceStatusFailed,
				"stage":           models.GPUImageEnhanceStatusFailed,
				"progress":        100,
				"error_message":   truncateGPUString(errText, 1500),
				"request_payload": datatypes.JSON(requestPayload),
				"metadata":        gpuToJSON(dispatchMeta),
				"finished_at":     &now,
				"updated_at":      now,
			}).Error
		updated, _ := h.getGPUImageEnhanceJobWithAssets(job.ID, userID)
		c.JSON(http.StatusOK, updated)
		return
	}

	_ = h.db.Model(&models.GPUImageEnhanceJob{}).
		Where("id = ? AND status = ?", job.ID, models.GPUImageEnhanceStatusQueued).
		Updates(map[string]interface{}{
			"status":          models.GPUImageEnhanceStatusRunning,
			"stage":           models.GPUImageEnhanceStatusRunning,
			"progress":        10,
			"error_message":   "",
			"request_payload": datatypes.JSON(requestPayload),
			"metadata":        gpuToJSON(dispatchMeta),
			"started_at":      &now,
			"updated_at":      now,
		}).Error

	updated, err := h.getGPUImageEnhanceJobWithAssets(job.ID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *Handler) ListGPUImageEnhanceJobs(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	page, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("page", "1")))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("page_size", "12")))
	if pageSize <= 0 {
		pageSize = 12
	}
	if pageSize > 50 {
		pageSize = 50
	}
	statusFilter := strings.TrimSpace(strings.ToLower(c.Query("status")))
	if statusFilter == "all" {
		statusFilter = ""
	}

	query := h.db.Model(&models.GPUImageEnhanceJob{}).Where("user_id = ?", userID)
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	offset := (page - 1) * pageSize
	var jobs []models.GPUImageEnhanceJob
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	assetsMap, err := h.listGPUImageEnhanceAssetsByJobIDs(extractGPUJobIDs(jobs))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]gpuImageEnhanceJobResponse, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, h.buildGPUImageEnhanceJobResponse(job, assetsMap[job.ID]))
	}

	hasMore := int64(offset+len(jobs)) < total
	c.JSON(http.StatusOK, listGPUImageEnhanceJobsResponse{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		HasMore:  hasMore,
	})
}

func (h *Handler) GetGPUImageEnhanceJob(c *gin.Context) {
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

	resp, err := h.getGPUImageEnhanceJobWithAssets(jobID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) DeleteGPUImageEnhanceJob(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}

	jobID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	deletedKeys, missingKeys, failedKeys, err := h.deleteGPUImageEnhanceJobByUser(jobID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(failedKeys) > 0 {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":        "failed to delete some qiniu objects",
			"failed_keys":  failedKeys,
			"deleted_keys": deletedKeys,
			"missing_keys": missingKeys,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":           true,
		"job_id":       jobID,
		"deleted_keys": deletedKeys,
		"missing_keys": missingKeys,
	})
}

func (h *Handler) BatchDeleteGPUImageEnhanceJobs(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}

	var req batchDeleteGPUImageEnhanceJobsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	seen := map[uint64]struct{}{}
	ids := make([]uint64, 0, len(req.IDs))
	for _, raw := range req.IDs {
		if raw == 0 {
			continue
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		ids = append(ids, raw)
		if len(ids) >= 100 {
			break
		}
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids is required"})
		return
	}

	deletedIDs := make([]uint64, 0, len(ids))
	notFoundIDs := make([]uint64, 0)
	failedIDs := make(map[string]string)
	details := make(map[string]gin.H)
	totalDeletedKeys := make([]string, 0)
	totalMissingKeys := make([]string, 0)

	for _, jobID := range ids {
		deletedKeys, missingKeys, failedKeys, err := h.deleteGPUImageEnhanceJobByUser(jobID, userID)
		key := strconv.FormatUint(jobID, 10)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				notFoundIDs = append(notFoundIDs, jobID)
				details[key] = gin.H{"status": "not_found"}
				continue
			}
			msg := truncateGPUString(strings.TrimSpace(err.Error()), 500)
			failedIDs[key] = msg
			details[key] = gin.H{
				"status":       "failed",
				"error":        msg,
				"deleted_keys": deletedKeys,
				"missing_keys": missingKeys,
			}
			continue
		}
		if len(failedKeys) > 0 {
			failedIDs[key] = "failed to delete some qiniu objects"
			details[key] = gin.H{
				"status":       "failed",
				"error":        "failed to delete some qiniu objects",
				"failed_keys":  failedKeys,
				"deleted_keys": deletedKeys,
				"missing_keys": missingKeys,
			}
			continue
		}
		deletedIDs = append(deletedIDs, jobID)
		totalDeletedKeys = append(totalDeletedKeys, deletedKeys...)
		totalMissingKeys = append(totalMissingKeys, missingKeys...)
		details[key] = gin.H{
			"status":       "deleted",
			"deleted_keys": deletedKeys,
			"missing_keys": missingKeys,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":            len(failedIDs) == 0,
		"requested_ids": ids,
		"deleted_ids":   deletedIDs,
		"not_found_ids": notFoundIDs,
		"failed_ids":    failedIDs,
		"details":       details,
		"deleted_keys":  totalDeletedKeys,
		"missing_keys":  totalMissingKeys,
	})
}

func (h *Handler) CallbackGPUImageEnhanceJob(c *gin.Context) {
	jobID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	expectedToken := strings.TrimSpace(h.cfg.GPUCallbackToken)
	if expectedToken != "" {
		gotToken := strings.TrimSpace(c.GetHeader(gpuTestCallbackTokenHeader))
		if gotToken == "" || gotToken != expectedToken {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid callback token"})
			return
		}
	}

	var req gpuImageEnhanceCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status, ok := normalizeGPUImageEnhanceStatus(req.Status)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}
	stage := normalizeGPUImageEnhanceStage(req.Stage, status)
	progress := resolveGPUImageEnhanceProgress(status, req.Progress)

	resultKey := normalizeStorageObjectKey(req.ResultObjectKey)
	if resultKey == "" {
		resultKey = normalizeStorageObjectKey(req.ResultKey)
	}

	var responseJob gpuImageEnhanceJobResponse
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var job models.GPUImageEnhanceJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", jobID).First(&job).Error; err != nil {
			return err
		}

		if status == models.GPUImageEnhanceStatusSucceeded && resultKey == "" {
			return fmt.Errorf("result_object_key is required when status is succeeded")
		}
		if resultKey != "" && !hasAnyPrefix(resultKey, h.qiniuUserGPUTestResultPrefix(job.UserID), h.qiniuLegacyUserGPUTestResultPrefix(job.UserID)) {
			return fmt.Errorf("forbidden result_object_key")
		}

		now := time.Now()
		updates := map[string]interface{}{
			"status":           status,
			"stage":            stage,
			"progress":         progress,
			"error_message":    truncateGPUString(strings.TrimSpace(req.ErrorMessage), 1500),
			"callback_payload": gpuToJSON(req),
			"updated_at":       now,
		}
		if status == models.GPUImageEnhanceStatusRunning && job.StartedAt == nil {
			updates["started_at"] = &now
		}
		if isGPUImageEnhanceFinalStatus(status) {
			updates["finished_at"] = &now
		}
		if status == models.GPUImageEnhanceStatusSucceeded {
			updates["error_message"] = ""
		}
		if resultKey != "" {
			updates["result_object_key"] = resultKey
		}
		if strings.TrimSpace(req.ResultMimeType) != "" {
			updates["result_mime_type"] = strings.TrimSpace(req.ResultMimeType)
		}
		if req.ResultSizeBytes > 0 {
			updates["result_size_bytes"] = req.ResultSizeBytes
		}

		if err := tx.Model(&models.GPUImageEnhanceJob{}).Where("id = ?", job.ID).Updates(updates).Error; err != nil {
			return err
		}

		if status == models.GPUImageEnhanceStatusSucceeded && resultKey != "" {
			resultAsset := models.GPUImageEnhanceAsset{
				JobID:     job.ID,
				UserID:    job.UserID,
				AssetRole: models.GPUImageEnhanceAssetRoleResult,
				ObjectKey: resultKey,
				MimeType:  strings.TrimSpace(req.ResultMimeType),
				SizeBytes: clampInt64Min(req.ResultSizeBytes, 0),
				Width:     clampIntMin(req.Width, 0),
				Height:    clampIntMin(req.Height, 0),
				Metadata:  gpuToJSON(req.Metadata),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "job_id"}, {Name: "asset_role"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"user_id":    resultAsset.UserID,
					"object_key": resultAsset.ObjectKey,
					"mime_type":  resultAsset.MimeType,
					"size_bytes": resultAsset.SizeBytes,
					"width":      resultAsset.Width,
					"height":     resultAsset.Height,
					"metadata":   resultAsset.Metadata,
					"updated_at": now,
				}),
			}).Create(&resultAsset).Error; err != nil {
				return err
			}
		}

		var updated models.GPUImageEnhanceJob
		if err := tx.Where("id = ?", job.ID).First(&updated).Error; err != nil {
			return err
		}
		assetsMap, err := h.listGPUImageEnhanceAssetsByJobIDs([]uint64{job.ID})
		if err != nil {
			return err
		}
		responseJob = h.buildGPUImageEnhanceJobResponse(updated, assetsMap[job.ID])
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		msg := strings.TrimSpace(err.Error())
		if strings.Contains(strings.ToLower(msg), "forbidden") || strings.Contains(strings.ToLower(msg), "required") {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":  true,
		"job": responseJob,
	})
}

func (h *Handler) getGPUImageEnhanceJobWithAssets(jobID, userID uint64) (gpuImageEnhanceJobResponse, error) {
	var job models.GPUImageEnhanceJob
	if err := h.db.Where("id = ? AND user_id = ?", jobID, userID).First(&job).Error; err != nil {
		return gpuImageEnhanceJobResponse{}, err
	}
	assetsMap, err := h.listGPUImageEnhanceAssetsByJobIDs([]uint64{job.ID})
	if err != nil {
		return gpuImageEnhanceJobResponse{}, err
	}
	return h.buildGPUImageEnhanceJobResponse(job, assetsMap[job.ID]), nil
}

func (h *Handler) listGPUImageEnhanceAssetsByJobIDs(jobIDs []uint64) (map[uint64][]models.GPUImageEnhanceAsset, error) {
	result := make(map[uint64][]models.GPUImageEnhanceAsset, len(jobIDs))
	if len(jobIDs) == 0 {
		return result, nil
	}
	var assets []models.GPUImageEnhanceAsset
	if err := h.db.Where("job_id IN ?", jobIDs).Order("id ASC").Find(&assets).Error; err != nil {
		return nil, err
	}
	for _, asset := range assets {
		result[asset.JobID] = append(result[asset.JobID], asset)
	}
	return result, nil
}

func extractGPUJobIDs(jobs []models.GPUImageEnhanceJob) []uint64 {
	ids := make([]uint64, 0, len(jobs))
	for _, job := range jobs {
		if job.ID == 0 {
			continue
		}
		ids = append(ids, job.ID)
	}
	return ids
}

func (h *Handler) buildGPUImageEnhanceJobResponse(job models.GPUImageEnhanceJob, assets []models.GPUImageEnhanceAsset) gpuImageEnhanceJobResponse {
	respAssets := make([]gpuImageEnhanceAssetResponse, 0, len(assets))
	for _, asset := range assets {
		respAssets = append(respAssets, gpuImageEnhanceAssetResponse{
			ID:        asset.ID,
			AssetRole: asset.AssetRole,
			ObjectKey: asset.ObjectKey,
			ObjectURL: h.resolveGPUObjectURLForDisplay(asset.ObjectKey),
			MimeType:  asset.MimeType,
			SizeBytes: asset.SizeBytes,
			Width:     asset.Width,
			Height:    asset.Height,
			Metadata:  parseJSONMap(asset.Metadata),
			CreatedAt: asset.CreatedAt,
			UpdatedAt: asset.UpdatedAt,
		})
	}

	sourceURL := h.resolveGPUObjectURLForDisplay(job.SourceObjectKey)
	resultURL := h.resolveGPUObjectURLForDisplay(job.ResultObjectKey)

	if sourceURL == "" {
		for _, asset := range respAssets {
			if asset.AssetRole == models.GPUImageEnhanceAssetRoleSource && asset.ObjectURL != "" {
				sourceURL = asset.ObjectURL
				break
			}
		}
	}
	if resultURL == "" {
		for _, asset := range respAssets {
			if asset.AssetRole == models.GPUImageEnhanceAssetRoleResult && asset.ObjectURL != "" {
				resultURL = asset.ObjectURL
				break
			}
		}
	}

	return gpuImageEnhanceJobResponse{
		ID:              job.ID,
		Title:           strings.TrimSpace(job.Title),
		Provider:        strings.TrimSpace(job.Provider),
		Model:           strings.TrimSpace(job.Model),
		Scale:           job.Scale,
		Status:          strings.TrimSpace(job.Status),
		Stage:           strings.TrimSpace(job.Stage),
		Progress:        job.Progress,
		ErrorMessage:    strings.TrimSpace(job.ErrorMessage),
		SourceObjectKey: strings.TrimSpace(job.SourceObjectKey),
		SourceObjectURL: sourceURL,
		ResultObjectKey: strings.TrimSpace(job.ResultObjectKey),
		ResultObjectURL: resultURL,
		QueuedAt:        job.QueuedAt,
		StartedAt:       job.StartedAt,
		FinishedAt:      job.FinishedAt,
		CreatedAt:       job.CreatedAt,
		UpdatedAt:       job.UpdatedAt,
		Assets:          respAssets,
	}
}

func normalizeGPUImageEnhanceStatus(raw string) (string, bool) {
	v := strings.TrimSpace(strings.ToLower(raw))
	switch v {
	case models.GPUImageEnhanceStatusQueued,
		models.GPUImageEnhanceStatusRunning,
		models.GPUImageEnhanceStatusSucceeded,
		models.GPUImageEnhanceStatusFailed,
		models.GPUImageEnhanceStatusCancelled:
		return v, true
	case "success", "done":
		return models.GPUImageEnhanceStatusSucceeded, true
	case "error":
		return models.GPUImageEnhanceStatusFailed, true
	default:
		return "", false
	}
}

func normalizeGPUImageEnhanceStage(raw string, fallbackStatus string) string {
	stage := strings.TrimSpace(strings.ToLower(raw))
	switch stage {
	case "queued", "running", "uploading", "callback", "succeeded", "failed", "cancelled":
		return stage
	}
	switch fallbackStatus {
	case models.GPUImageEnhanceStatusQueued,
		models.GPUImageEnhanceStatusRunning,
		models.GPUImageEnhanceStatusSucceeded,
		models.GPUImageEnhanceStatusFailed,
		models.GPUImageEnhanceStatusCancelled:
		return fallbackStatus
	default:
		return models.GPUImageEnhanceStatusRunning
	}
}

func resolveGPUImageEnhanceProgress(status string, raw *int) int {
	if raw != nil {
		value := *raw
		if value < 0 {
			value = 0
		}
		if value > 100 {
			value = 100
		}
		return value
	}
	switch status {
	case models.GPUImageEnhanceStatusQueued:
		return 0
	case models.GPUImageEnhanceStatusRunning:
		return 50
	default:
		return 100
	}
}

func isGPUImageEnhanceFinalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case models.GPUImageEnhanceStatusSucceeded, models.GPUImageEnhanceStatusFailed, models.GPUImageEnhanceStatusCancelled:
		return true
	default:
		return false
	}
}

func (h *Handler) resolveGPUObjectURLByKey(key string, ttl int64) (string, error) {
	if h == nil || h.qiniu == nil {
		return "", errors.New("qiniu not configured")
	}
	cleanKey := normalizeStorageObjectKey(key)
	if cleanKey == "" {
		return "", errors.New("empty key")
	}
	if signed, err := h.qiniu.SignedURL(cleanKey, ttl); err == nil {
		signed = strings.TrimSpace(signed)
		if strings.HasPrefix(signed, "http://") || strings.HasPrefix(signed, "https://") {
			return signed, nil
		}
	}
	publicURL := strings.TrimSpace(h.qiniu.PublicURL(cleanKey))
	if strings.HasPrefix(publicURL, "http://") || strings.HasPrefix(publicURL, "https://") {
		return publicURL, nil
	}
	return "", fmt.Errorf("unable to build object url for key=%s", cleanKey)
}

func (h *Handler) resolveGPUObjectURLForDisplay(key string) string {
	cleanKey := normalizeStorageObjectKey(key)
	if cleanKey == "" || h == nil || h.qiniu == nil {
		return ""
	}
	if signed, err := h.qiniu.SignedURL(cleanKey, 1800); err == nil {
		signed = strings.TrimSpace(signed)
		if strings.HasPrefix(signed, "http://") || strings.HasPrefix(signed, "https://") {
			return signed
		}
	}
	publicURL := strings.TrimSpace(h.qiniu.PublicURL(cleanKey))
	if strings.HasPrefix(publicURL, "http://") || strings.HasPrefix(publicURL, "https://") {
		return publicURL
	}
	return ""
}

func defaultGPUJobTitle(sourceKey string) string {
	base := strings.TrimSpace(filepath.Base(sourceKey))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.TrimSpace(base)
	if base == "" {
		return "图片增强任务"
	}
	return base
}

func buildGPUTestResultObjectKey(resultPrefix, sourceKey string) string {
	prefix := strings.TrimLeft(strings.TrimSpace(resultPrefix), "/")
	prefix = strings.TrimSuffix(prefix, "/")
	sourceName := filepath.Base(strings.TrimSpace(sourceKey))
	stem := sanitizeGPUObjectStem(strings.TrimSuffix(sourceName, filepath.Ext(sourceName)))
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(sourceName)))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
	default:
		ext = ".png"
	}
	if ext == "" {
		ext = ".png"
	}
	name := fmt.Sprintf("%s-%d%s", stem, time.Now().UnixNano(), ext)
	if prefix == "" {
		return name
	}
	return path.Join(prefix, name)
}

func sanitizeGPUObjectStem(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "enhanced"
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-_")
	s = strings.ReplaceAll(s, "--", "-")
	if s == "" {
		s = "enhanced"
	}
	if len(s) > 48 {
		s = s[:48]
		s = strings.TrimRight(s, "-_")
		if s == "" {
			s = "enhanced"
		}
	}
	return s
}

func (h *Handler) buildGPUCallbackURL(c *gin.Context, jobID uint64) (string, error) {
	base := strings.TrimSpace(h.cfg.GPUCallbackBaseURL)
	if base == "" && c != nil && c.Request != nil {
		proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
		if proto == "" {
			if c.Request.TLS != nil {
				proto = "https"
			} else {
				proto = "http"
			}
		}
		if idx := strings.Index(proto, ","); idx >= 0 {
			proto = strings.TrimSpace(proto[:idx])
		}
		host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
		if host == "" {
			host = strings.TrimSpace(c.Request.Host)
		}
		if host != "" {
			base = proto + "://" + host
		}
	}
	base = strings.TrimSuffix(strings.TrimSpace(base), "/")
	if base == "" {
		return "", errors.New("GPU_CALLBACK_BASE_URL empty and request host unavailable")
	}
	return fmt.Sprintf("%s/api/gpu-tests/jobs/%d/callback", base, jobID), nil
}

func (h *Handler) gpuServiceEnhanceEndpoint() string {
	base := strings.TrimSuffix(strings.TrimSpace(h.cfg.GPUServiceURL), "/")
	if base == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(base), "/enhance") {
		return base
	}
	return base + "/enhance"
}

func (h *Handler) dispatchGPUImageEnhanceJob(ctx context.Context, req gpuImageEnhanceDispatchRequest) (int, string, error) {
	endpoint := h.gpuServiceEnhanceEndpoint()
	if endpoint == "" {
		return 0, "", errors.New("GPU_SERVICE_URL not configured")
	}

	body := gpuMustMarshalJSON(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	token := strings.TrimSpace(h.cfg.GPUServiceToken)
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
		httpReq.Header.Set("X-GPU-SERVICE-TOKEN", token)
	}

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, gpuTestMaxDispatchBodySize))
	bodyText := strings.TrimSpace(string(bodyBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if bodyText == "" {
			bodyText = fmt.Sprintf("gpu service http %d", resp.StatusCode)
		}
		return resp.StatusCode, bodyText, fmt.Errorf("gpu service rejected request")
	}
	return resp.StatusCode, bodyText, nil
}

func gpuEmptyJSON() datatypes.JSON {
	return datatypes.JSON([]byte("{}"))
}

func gpuToJSON(v interface{}) datatypes.JSON {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) == 0 {
		return gpuEmptyJSON()
	}
	return datatypes.JSON(raw)
}

func gpuMustMarshalJSON(v interface{}) []byte {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

func truncateGPUString(raw string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(raw) <= maxLen {
		return raw
	}
	return raw[:maxLen]
}

func clampIntMin(value int, min int) int {
	if value < min {
		return min
	}
	return value
}

func clampInt64Min(value int64, min int64) int64 {
	if value < min {
		return min
	}
	return value
}

func (h *Handler) deleteGPUImageEnhanceJobByUser(jobID, userID uint64) ([]string, []string, map[string]string, error) {
	var job models.GPUImageEnhanceJob
	if err := h.db.Where("id = ? AND user_id = ?", jobID, userID).First(&job).Error; err != nil {
		return nil, nil, nil, err
	}

	var assets []models.GPUImageEnhanceAsset
	if err := h.db.Where("job_id = ? AND user_id = ?", jobID, userID).Find(&assets).Error; err != nil {
		return nil, nil, nil, err
	}

	keys := collectGPUImageEnhanceObjectKeys(job, assets)
	deletedKeys := make([]string, 0, len(keys))
	missingKeys := make([]string, 0, len(keys))
	failedKeys := make(map[string]string)

	bm := h.qiniu.BucketManager()
	for _, key := range keys {
		if !h.isGPUObjectKeyOwnedByUser(key, userID) {
			failedKeys[key] = "forbidden object key"
			continue
		}
		if err := bm.Delete(h.qiniu.Bucket, key); err != nil {
			if isQiniuObjectNotFoundErr(err) {
				missingKeys = append(missingKeys, key)
				continue
			}
			failedKeys[key] = truncateGPUString(strings.TrimSpace(err.Error()), 400)
			continue
		}
		deletedKeys = append(deletedKeys, key)
	}

	if len(failedKeys) > 0 {
		return deletedKeys, missingKeys, failedKeys, nil
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("job_id = ? AND user_id = ?", jobID, userID).Delete(&models.GPUImageEnhanceAsset{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ? AND user_id = ?", jobID, userID).Delete(&models.GPUImageEnhanceJob{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	}); err != nil {
		return deletedKeys, missingKeys, failedKeys, err
	}

	return deletedKeys, missingKeys, failedKeys, nil
}

func collectGPUImageEnhanceObjectKeys(job models.GPUImageEnhanceJob, assets []models.GPUImageEnhanceAsset) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(assets)+2)
	appendKey := func(raw string) {
		key := normalizeStorageObjectKey(raw)
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	appendKey(job.SourceObjectKey)
	appendKey(job.ResultObjectKey)
	for _, asset := range assets {
		appendKey(asset.ObjectKey)
	}
	return out
}

func (h *Handler) isGPUObjectKeyOwnedByUser(rawKey string, userID uint64) bool {
	key := normalizeStorageObjectKey(rawKey)
	if key == "" {
		return false
	}
	return hasAnyPrefix(
		key,
		h.qiniuUserGPUTestSourcePrefix(userID),
		h.qiniuUserGPUTestResultPrefix(userID),
		h.qiniuLegacyUserGPUTestSourcePrefix(userID),
		h.qiniuLegacyUserGPUTestResultPrefix(userID),
	)
}

func isQiniuObjectNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "612") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "no such key") ||
		strings.Contains(msg, "file not found")
}
