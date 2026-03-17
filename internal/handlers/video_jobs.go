package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/queue"
	"emoji/internal/storage"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var allowedVideoFileExt = map[string]struct{}{
	".mp4":  {},
	".mov":  {},
	".mkv":  {},
	".webm": {},
	".avi":  {},
	".m4v":  {},
	".mpeg": {},
	".mpg":  {},
	".wmv":  {},
	".flv":  {},
	".3gp":  {},
	".ts":   {},
	".mts":  {},
	".m2ts": {},
}

type CreateVideoJobRequest struct {
	Title            string                    `json:"title"`
	CategoryID       *uint64                   `json:"category_id"`
	SourceVideoKey   string                    `json:"source_video_key"`
	OutputFormats    []string                  `json:"output_formats"`
	MaxStatic        int                       `json:"max_static"`
	FrameIntervalSec float64                   `json:"frame_interval_sec"`
	AutoHighlight    *bool                     `json:"auto_highlight"`
	Priority         string                    `json:"priority"`
	EditOptions      *VideoJobEditOptionsInput `json:"edit_options"`
}

type VideoJobCropInput struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type VideoJobEditOptionsInput struct {
	StartSec  float64            `json:"start_sec"`
	EndSec    float64            `json:"end_sec"`
	Speed     float64            `json:"speed"`
	FPS       int                `json:"fps"`
	Width     int                `json:"width"`
	MaxColors int                `json:"max_colors"`
	Crop      *VideoJobCropInput `json:"crop"`
}

type VideoJobResponse struct {
	ID                 uint64                 `json:"id"`
	Title              string                 `json:"title"`
	SourceVideoKey     string                 `json:"source_video_key"`
	SourceVideoURL     string                 `json:"source_video_url,omitempty"`
	CategoryID         *uint64                `json:"category_id,omitempty"`
	OutputFormats      []string               `json:"output_formats"`
	Status             string                 `json:"status"`
	Stage              string                 `json:"stage"`
	Progress           int                    `json:"progress"`
	Priority           string                 `json:"priority"`
	ErrorMessage       string                 `json:"error_message,omitempty"`
	ResultCollectionID *uint64                `json:"result_collection_id,omitempty"`
	Options            map[string]interface{} `json:"options,omitempty"`
	Metrics            map[string]interface{} `json:"metrics,omitempty"`
	QueuedAt           time.Time              `json:"queued_at"`
	StartedAt          *time.Time             `json:"started_at,omitempty"`
	FinishedAt         *time.Time             `json:"finished_at,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	ResultSummary      *VideoJobResultSummary `json:"result_summary,omitempty"`
}

type VideoJobResultSummary struct {
	CollectionID          uint64   `json:"collection_id,omitempty"`
	CollectionTitle       string   `json:"collection_title,omitempty"`
	FileCount             int      `json:"file_count"`
	PreviewImages         []string `json:"preview_images,omitempty"`
	FormatSummary         []string `json:"format_summary,omitempty"`
	PackageStatus         string   `json:"package_status,omitempty"`
	QualitySampleCount    int      `json:"quality_sample_count,omitempty"`
	QualityTopScore       float64  `json:"quality_top_score,omitempty"`
	QualityAvgScore       float64  `json:"quality_avg_score,omitempty"`
	QualityAvgLoopClosure float64  `json:"quality_avg_loop_closure,omitempty"`
}

type VideoJobResultEmojiItem struct {
	ID                          uint64     `json:"id"`
	OutputID                    uint64     `json:"output_id,omitempty"`
	ReviewRecommendation        string     `json:"review_recommendation,omitempty"`
	Title                       string     `json:"title"`
	Format                      string     `json:"format"`
	FileKey                     string     `json:"file_key"`
	FileURL                     string     `json:"file_url"`
	ThumbKey                    string     `json:"thumb_key,omitempty"`
	ThumbURL                    string     `json:"thumb_url,omitempty"`
	Width                       int        `json:"width"`
	Height                      int        `json:"height"`
	SizeBytes                   int64      `json:"size_bytes"`
	DisplayOrder                int        `json:"display_order"`
	FeedbackAction              string     `json:"feedback_action,omitempty"`
	FeedbackAt                  *time.Time `json:"feedback_at,omitempty"`
	FeedbackOutputID            uint64     `json:"feedback_output_id,omitempty"`
	OutputScore                 float64    `json:"output_score,omitempty"`
	GIFLoopTuneApplied          bool       `json:"gif_loop_tune_applied,omitempty"`
	GIFLoopTuneEffectiveApplied bool       `json:"gif_loop_tune_effective_applied,omitempty"`
	GIFLoopTuneFallbackToBase   bool       `json:"gif_loop_tune_fallback_to_base,omitempty"`
	GIFLoopTuneScore            float64    `json:"gif_loop_tune_score,omitempty"`
	GIFLoopTuneLoopClosure      float64    `json:"gif_loop_tune_loop_closure,omitempty"`
	GIFLoopTuneMotionMean       float64    `json:"gif_loop_tune_motion_mean,omitempty"`
	GIFLoopTuneEffectiveSec     float64    `json:"gif_loop_tune_effective_sec,omitempty"`
}

type VideoJobResultPackageItem struct {
	ID         uint64     `json:"id,omitempty"`
	FileKey    string     `json:"file_key"`
	FileURL    string     `json:"file_url"`
	FileName   string     `json:"file_name"`
	SizeBytes  int64      `json:"size_bytes"`
	UploadedAt *time.Time `json:"uploaded_at,omitempty"`
}

type VideoJobResultResponse struct {
	JobID              uint64                     `json:"job_id"`
	Status             string                     `json:"status"`
	DeliveryOnly       bool                       `json:"delivery_only,omitempty"`
	ReviewStatusFilter []string                   `json:"review_status_filter,omitempty"`
	Collection         map[string]interface{}     `json:"collection,omitempty"`
	Emojis             []VideoJobResultEmojiItem  `json:"emojis,omitempty"`
	Package            *VideoJobResultPackageItem `json:"package,omitempty"`
	Options            map[string]interface{}     `json:"options,omitempty"`
	Metrics            map[string]interface{}     `json:"metrics,omitempty"`
	Message            string                     `json:"message,omitempty"`
}

type VideoJobCapabilitiesResponse struct {
	FFmpegAvailable    bool                         `json:"ffmpeg_available"`
	FFprobeAvailable   bool                         `json:"ffprobe_available"`
	SupportedFormats   []string                     `json:"supported_formats"`
	UnsupportedFormats []string                     `json:"unsupported_formats"`
	Formats            []videojobs.FormatCapability `json:"formats"`
}

type ProbeSourceVideoRequest struct {
	SourceVideoKey string `json:"source_video_key"`
}

type ProbeSourceVideoResponse struct {
	SourceVideoKey string  `json:"source_video_key"`
	Format         string  `json:"format"`
	MimeType       string  `json:"mime_type"`
	SizeBytes      int64   `json:"size_bytes"`
	DurationSec    float64 `json:"duration_sec"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	FPS            float64 `json:"fps"`
	AspectRatio    string  `json:"aspect_ratio"`
	Orientation    string  `json:"orientation"`
}

// CreateVideoJob godoc
// @Summary Create video emoji generation job
// @Tags user
// @Accept json
// @Produce json
// @Param body body CreateVideoJobRequest true "create video job request"
// @Success 200 {object} VideoJobResponse
// @Router /api/video-jobs [post]
func (h *Handler) CreateVideoJob(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	if h.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "task queue not configured"})
		return
	}

	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreateVideoJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sourceKey, err := h.normalizeSourceVideoKey(req.SourceVideoKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var categoryID *uint64
	if req.CategoryID != nil && *req.CategoryID > 0 {
		if err := h.requireLeafCategory(*req.CategoryID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "category not found"})
				return
			}
			if errors.Is(err, errCategoryHasChildren) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "category has children"})
				return
			}
			if errors.Is(err, errInvalidCategory) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		categoryID = req.CategoryID
	}

	formats := normalizeVideoOutputFormats(req.OutputFormats)
	if len(formats) != 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "exactly_one_output_format_required",
			"message": "一次任务仅支持选择 1 种输出格式",
		})
		return
	}
	capabilities := videojobs.DetectRuntimeCapabilities()
	if !capabilities.FFmpegAvailable || !capabilities.FFprobeAvailable {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "video runtime unavailable",
			"message": "视频处理运行时不可用，请联系管理员检查 ffmpeg/ffprobe",
			"details": capabilities,
		})
		return
	}
	unsupportedReasons := map[string]string{}
	for _, item := range capabilities.Formats {
		if item.Supported {
			continue
		}
		unsupportedReasons[strings.ToLower(strings.TrimSpace(item.Format))] = strings.TrimSpace(item.Reason)
	}
	rejectedFormats := make([]gin.H, 0, len(formats))
	for _, format := range formats {
		if reason, ok := unsupportedReasons[format]; ok {
			rejectedFormats = append(rejectedFormats, gin.H{
				"format": format,
				"reason": reason,
			})
		}
	}
	if len(rejectedFormats) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":               "unsupported_output_format",
			"message":             "请求包含当前服务器不支持的输出格式",
			"rejected_formats":    rejectedFormats,
			"supported_formats":   capabilities.SupportedFormats,
			"unsupported_formats": capabilities.UnsupportedFormats,
		})
		return
	}
	priority := normalizeVideoJobPriority(req.Priority)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = fmt.Sprintf("视频表情包-%s", time.Now().Format("20060102150405"))
	}

	options := map[string]interface{}{}
	autoHighlight := true
	if req.AutoHighlight != nil {
		autoHighlight = *req.AutoHighlight
	}
	options["auto_highlight"] = autoHighlight
	if req.MaxStatic > 0 {
		if req.MaxStatic > 80 {
			req.MaxStatic = 80
		}
		options["max_static"] = req.MaxStatic
	}
	if req.FrameIntervalSec > 0 {
		options["frame_interval_sec"] = req.FrameIntervalSec
	}
	editOptions, err := normalizeVideoEditOptions(req.EditOptions)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for key, value := range editOptions {
		options[key] = value
	}
	sourceSizeBytes := h.lookupSourceVideoSizeBytes(sourceKey)
	probeMeta, err := h.preflightProbeSourceVideo(c.Request.Context(), sourceKey)
	if err != nil {
		detail := describeSourceVideoProbeFailure(err)
		resp := gin.H{
			"error":       "invalid_source_video",
			"message":     detail.Message,
			"reason_code": detail.Code,
		}
		if detail.Hint != "" {
			resp["hint"] = detail.Hint
		}
		if detail.MaxDurationSec > 0 {
			resp["max_duration_sec"] = detail.MaxDurationSec
		}
		c.JSON(http.StatusBadRequest, resp)
		return
	}
	options["source_video_probe"] = map[string]interface{}{
		"duration_sec": roundTo1(probeMeta.DurationSec),
		"width":        probeMeta.Width,
		"height":       probeMeta.Height,
		"fps":          roundTo1(probeMeta.FPS),
	}
	recommendation := h.buildQualityTemplateRecommendation(probeMeta, formats)
	if len(recommendation) > 0 {
		options["quality_template_recommendation"] = recommendation
		recommended := extractRecommendedProfilesFromRecommendation(recommendation)
		if len(recommended) > 0 {
			options["quality_profile_overrides"] = recommended
			options["quality_template_applied"] = "auto_recommendation"
		}
	}
	if sourceSizeBytes > 0 {
		options["source_video_size_bytes"] = sourceSizeBytes
	}
	estimatedPoints := videojobs.EstimateReservationPoints(sourceSizeBytes, formats, options)
	options["estimate_points"] = estimatedPoints

	job := models.VideoJob{
		UserID:         userID,
		Title:          title,
		SourceVideoKey: sourceKey,
		CategoryID:     categoryID,
		OutputFormats:  strings.Join(formats, ","),
		Status:         models.VideoJobStatusQueued,
		Stage:          models.VideoJobStageQueued,
		Progress:       0,
		Priority:       priority,
		Options:        toJSON(options),
		Metrics:        datatypes.JSON([]byte("{}")),
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := videojobs.UpsertPublicVideoImageJob(tx, job); err != nil {
			return err
		}
		queuedEvent := models.VideoJobEvent{
			JobID:    job.ID,
			Stage:    models.VideoJobStageQueued,
			Level:    "info",
			Message:  "job queued",
			Metadata: datatypes.JSON([]byte("{}")),
		}
		if err := tx.Create(&models.VideoJobEvent{
			JobID:    job.ID,
			Stage:    models.VideoJobStageQueued,
			Level:    "info",
			Message:  "job queued",
			Metadata: datatypes.JSON([]byte("{}")),
		}).Error; err != nil {
			return err
		}
		if err := videojobs.CreatePublicVideoImageEvent(tx, queuedEvent); err != nil {
			return err
		}
		return videojobs.ReservePointsForJob(tx, userID, job.ID, estimatedPoints, "video job reserve", map[string]interface{}{
			"source_video_key":  sourceKey,
			"output_formats":    formats,
			"source_size_bytes": sourceSizeBytes,
		})
	}); err != nil {
		var insufficient videojobs.InsufficientPointsError
		if errors.As(err, &insufficient) {
			c.JSON(http.StatusPaymentRequired, gin.H{
				"error":            "insufficient_compute_points",
				"message":          insufficient.Error(),
				"required_points":  insufficient.Required,
				"available_points": insufficient.Available,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	task, err := videojobs.NewProcessVideoJobTask(job.ID)
	if err != nil {
		failUpdates := map[string]interface{}{
			"status":        models.VideoJobStatusFailed,
			"stage":         models.VideoJobStageFailed,
			"error_message": err.Error(),
			"finished_at":   time.Now(),
		}
		_ = h.db.Model(&models.VideoJob{}).Where("id = ?", job.ID).Updates(failUpdates).Error
		_ = videojobs.SyncPublicVideoImageJobUpdates(h.db, job.ID, failUpdates)
		_ = videojobs.ReleaseReservedPointsForJob(h.db, job.ID, "task_create_failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	_, err = h.queue.Enqueue(
		task,
		asynq.Queue("media"),
		asynq.MaxRetry(6),
		asynq.Timeout(2*time.Hour),
		asynq.Retention(7*24*time.Hour),
		asynq.TaskID(fmt.Sprintf("video-job-%d", job.ID)),
	)
	if err != nil {
		failUpdates := map[string]interface{}{
			"status":        models.VideoJobStatusFailed,
			"stage":         models.VideoJobStageFailed,
			"error_message": err.Error(),
			"finished_at":   time.Now(),
		}
		_ = h.db.Model(&models.VideoJob{}).Where("id = ?", job.ID).Updates(failUpdates).Error
		_ = videojobs.SyncPublicVideoImageJobUpdates(h.db, job.ID, failUpdates)
		failEvent := models.VideoJobEvent{
			JobID:    job.ID,
			Stage:    models.VideoJobStageFailed,
			Level:    "error",
			Message:  "enqueue failed: " + err.Error(),
			Metadata: datatypes.JSON([]byte("{}")),
		}
		_ = h.db.Create(&failEvent).Error
		_ = videojobs.CreatePublicVideoImageEvent(h.db, failEvent)
		_ = videojobs.ReleaseReservedPointsForJob(h.db, job.ID, "enqueue_failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue job"})
		return
	}

	c.JSON(http.StatusOK, buildVideoJobResponse(job, h.qiniu))
}

// GetVideoJobCapabilities godoc
// @Summary Get video generation runtime capabilities
// @Tags user
// @Produce json
// @Success 200 {object} VideoJobCapabilitiesResponse
// @Router /api/video-jobs/capabilities [get]
func (h *Handler) GetVideoJobCapabilities(c *gin.Context) {
	capabilities := videojobs.DetectRuntimeCapabilities()
	c.JSON(http.StatusOK, VideoJobCapabilitiesResponse{
		FFmpegAvailable:    capabilities.FFmpegAvailable,
		FFprobeAvailable:   capabilities.FFprobeAvailable,
		SupportedFormats:   capabilities.SupportedFormats,
		UnsupportedFormats: capabilities.UnsupportedFormats,
		Formats:            capabilities.Formats,
	})
}

// ProbeVideoSource godoc
// @Summary Probe uploaded source video metadata
// @Tags user
// @Accept json
// @Produce json
// @Param body body ProbeSourceVideoRequest true "source video key"
// @Success 200 {object} ProbeSourceVideoResponse
// @Router /api/video-jobs/source-probe [post]
func (h *Handler) ProbeVideoSource(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req ProbeSourceVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sourceKey, err := h.normalizeSourceVideoKey(req.SourceVideoKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	capabilities := videojobs.DetectRuntimeCapabilities()
	if !capabilities.FFmpegAvailable || !capabilities.FFprobeAvailable {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "video runtime unavailable",
			"message": "视频处理运行时不可用，请联系管理员检查 ffmpeg/ffprobe",
			"details": capabilities,
		})
		return
	}

	sizeBytes := h.lookupSourceVideoSizeBytes(sourceKey)
	meta, err := h.preflightProbeSourceVideo(c.Request.Context(), sourceKey)
	if err != nil {
		detail := describeSourceVideoProbeFailure(err)
		resp := gin.H{
			"error":       "invalid_source_video",
			"message":     detail.Message,
			"reason_code": detail.Code,
		}
		if detail.Hint != "" {
			resp["hint"] = detail.Hint
		}
		if detail.MaxDurationSec > 0 {
			resp["max_duration_sec"] = detail.MaxDurationSec
		}
		c.JSON(http.StatusBadRequest, resp)
		return
	}

	width := meta.Width
	height := meta.Height
	aspectRatio := buildAspectRatio(width, height)
	orientation := "unknown"
	if width > 0 && height > 0 {
		switch {
		case width > height:
			orientation = "landscape"
		case width < height:
			orientation = "portrait"
		default:
			orientation = "square"
		}
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(sourceKey)), ".")
	mimeType := mimeTypeByVideoExt(format)
	c.JSON(http.StatusOK, ProbeSourceVideoResponse{
		SourceVideoKey: sourceKey,
		Format:         format,
		MimeType:       mimeType,
		SizeBytes:      sizeBytes,
		DurationSec:    roundTo1(meta.DurationSec),
		Width:          width,
		Height:         height,
		FPS:            roundTo1(meta.FPS),
		AspectRatio:    aspectRatio,
		Orientation:    orientation,
	})
}

// ListMyVideoJobs godoc
// @Summary List current user video jobs
// @Tags user
// @Produce json
// @Router /api/video-jobs [get]
func (h *Handler) ListMyVideoJobs(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	status := strings.TrimSpace(c.Query("status"))
	includeResultSummary := shouldIncludeResultSummary(c.Query("include_result_summary"))

	query := h.db.Model(&models.VideoJob{}).Where("user_id = ?", userID)
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var jobs []models.VideoJob
	if err := query.Order("id DESC").Limit(limit).Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]VideoJobResponse, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, buildVideoJobResponse(job, h.qiniu))
	}
	if includeResultSummary && len(items) > 0 {
		summaryByCollectionID, err := h.buildVideoJobResultSummary(jobs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for idx, job := range jobs {
			if idx >= len(items) || job.ResultCollectionID == nil || *job.ResultCollectionID == 0 {
				continue
			}
			collectionID := *job.ResultCollectionID
			summary, ok := summaryByCollectionID[collectionID]
			if !ok {
				summary = VideoJobResultSummary{
					CollectionID:  collectionID,
					PackageStatus: "processing",
				}
			}
			summary.PackageStatus = resolveVideoJobPackageStatus(job, summary.PackageStatus)
			filled := summary
			items[idx].ResultSummary = &filled
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GetVideoJob godoc
// @Summary Get current user video job detail
// @Tags user
// @Produce json
// @Router /api/video-jobs/{id} [get]
func (h *Handler) GetVideoJob(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var job models.VideoJob
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, buildVideoJobResponse(job, h.qiniu))
}

// CancelVideoJob godoc
// @Summary Cancel current user video job
// @Tags user
// @Produce json
// @Router /api/video-jobs/{id}/cancel [post]
func (h *Handler) CancelVideoJob(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var job models.VideoJob
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	switch job.Status {
	case models.VideoJobStatusDone, models.VideoJobStatusFailed, models.VideoJobStatusCancelled:
		c.JSON(http.StatusBadRequest, gin.H{"error": "job cannot be cancelled in current status"})
		return
	}

	finishedAt := time.Now()
	updates := map[string]interface{}{
		"status":      models.VideoJobStatusCancelled,
		"stage":       models.VideoJobStageCancelled,
		"progress":    job.Progress,
		"finished_at": finishedAt,
	}
	metrics := parseJSONMap(job.Metrics)
	if cleanupVideoSourceObject(h.qiniu, job.SourceVideoKey) {
		metrics["source_video_deleted"] = true
		metrics["source_video_deleted_at"] = time.Now().Format(time.RFC3339)
		metrics["source_video_cleanup_reason"] = "cancelled"
		updates["metrics"] = toJSON(metrics)
	}

	updateResult := h.db.Model(&models.VideoJob{}).
		Where("id = ? AND user_id = ? AND status IN ?", job.ID, userID, []string{
			models.VideoJobStatusQueued,
			models.VideoJobStatusRunning,
		}).
		Updates(updates)
	if updateResult.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": updateResult.Error.Error()})
		return
	}
	if updateResult.RowsAffected == 0 {
		var latest models.VideoJob
		if err := h.db.Where("id = ? AND user_id = ?", job.ID, userID).First(&latest).Error; err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "job status changed"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{
			"error":  "job status changed",
			"status": latest.Status,
		})
		return
	}
	_ = videojobs.SyncPublicVideoImageJobUpdates(h.db, job.ID, updates)
	h.bestEffortCancelVideoJobTask(job.ID)
	_ = videojobs.UpsertJobCost(h.db, job.ID)
	_ = videojobs.SettleReservedPointsForJob(h.db, job.ID, models.VideoJobStatusCancelled)
	cancelEvent := models.VideoJobEvent{
		JobID:    job.ID,
		Stage:    models.VideoJobStageCancelled,
		Level:    "info",
		Message:  "job cancelled by user",
		Metadata: datatypes.JSON([]byte("{}")),
	}
	_ = h.db.Create(&cancelEvent).Error
	_ = videojobs.CreatePublicVideoImageEvent(h.db, cancelEvent)

	var latest models.VideoJob
	if err := h.db.First(&latest, job.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, buildVideoJobResponse(latest, h.qiniu))
}

func (h *Handler) bestEffortCancelVideoJobTask(jobID uint64) {
	if h == nil || jobID == 0 {
		return
	}
	inspector := queue.NewInspector(h.cfg)
	if inspector == nil {
		return
	}
	defer inspector.Close()

	taskID := fmt.Sprintf("video-job-%d", jobID)
	_ = inspector.DeleteTask("media", taskID)
	_ = inspector.ArchiveTask("media", taskID)
	_ = inspector.CancelProcessing(taskID)
}

// GetVideoJobResult godoc
// @Summary Get current user video job result collection
// @Tags user
// @Produce json
// @Param delivery_only query bool false "whether to only return deliver outputs (default true)"
// @Param review_status query string false "comma-separated review statuses: deliver,keep_internal,reject,need_manual_review"
// @Router /api/video-jobs/{id}/result [get]
func (h *Handler) GetVideoJobResult(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var job models.VideoJob
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if job.Status != models.VideoJobStatusDone || job.ResultCollectionID == nil || *job.ResultCollectionID == 0 {
		c.JSON(http.StatusConflict, VideoJobResultResponse{
			JobID:   job.ID,
			Status:  job.Status,
			Message: "job is not completed",
		})
		return
	}

	var collection models.Collection
	if err := h.db.Where("id = ?", *job.ResultCollectionID).First(&collection).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "result collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if collection.OwnerID != userID && !isAdminRole(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	deliveryOnly := parseBoolQueryWithDefault(c.Query("delivery_only"), true)
	reviewStatusFilter := parseVideoJobReviewStatusFilter(c.Query("review_status"))
	if deliveryOnly && len(reviewStatusFilter) == 0 {
		reviewStatusFilter = []string{"deliver"}
	}
	reviewStatusSet := buildVideoJobReviewStatusSet(reviewStatusFilter)

	var emojis []models.Emoji
	if err := h.db.Where("collection_id = ? AND status = ?", collection.ID, "active").Order("display_order ASC, id ASC").Find(&emojis).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type outputSnapshot struct {
		ID                          uint64
		Score                       float64
		GIFLoopTuneApplied          bool
		GIFLoopTuneEffectiveApplied bool
		GIFLoopTuneFallbackToBase   bool
		GIFLoopTuneScore            float64
		GIFLoopTuneLoopClosure      float64
		GIFLoopTuneMotionMean       float64
		GIFLoopTuneEffectiveSec     float64
	}
	outputByObjectKey := map[string]outputSnapshot{}
	var outputs []models.VideoImageOutputPublic
	if err := h.db.
		Select(
			"id",
			"object_key",
			"score",
			"gif_loop_tune_applied",
			"gif_loop_tune_effective_applied",
			"gif_loop_tune_fallback_to_base",
			"gif_loop_tune_score",
			"gif_loop_tune_loop_closure",
			"gif_loop_tune_motion_mean",
			"gif_loop_tune_effective_sec",
		).
		Where("job_id = ? AND file_role = ?", job.ID, "main").
		Find(&outputs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, out := range outputs {
		key := strings.TrimSpace(out.ObjectKey)
		if key == "" {
			continue
		}
		if _, exists := outputByObjectKey[key]; !exists {
			outputByObjectKey[key] = outputSnapshot{
				ID:                          out.ID,
				Score:                       out.Score,
				GIFLoopTuneApplied:          out.GIFLoopTuneApplied,
				GIFLoopTuneEffectiveApplied: out.GIFLoopTuneEffectiveApplied,
				GIFLoopTuneFallbackToBase:   out.GIFLoopTuneFallbackToBase,
				GIFLoopTuneScore:            out.GIFLoopTuneScore,
				GIFLoopTuneLoopClosure:      out.GIFLoopTuneLoopClosure,
				GIFLoopTuneMotionMean:       out.GIFLoopTuneMotionMean,
				GIFLoopTuneEffectiveSec:     out.GIFLoopTuneEffectiveSec,
			}
		}
	}

	reviewByOutputID := map[uint64]string{}
	var aiReviewRows []models.VideoJobGIFAIReview
	if err := h.db.
		Select("id", "output_id", "final_recommendation").
		Where("job_id = ?", job.ID).
		Order("id DESC").
		Find(&aiReviewRows).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_gif_ai_reviews") &&
			(strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such table"))) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		aiReviewRows = nil
	}
	for _, row := range aiReviewRows {
		if row.OutputID == nil || *row.OutputID == 0 {
			continue
		}
		if _, exists := reviewByOutputID[*row.OutputID]; exists {
			continue
		}
		if status := normalizeVideoJobReviewStatus(row.FinalRecommendation); status != "" {
			reviewByOutputID[*row.OutputID] = status
		}
	}

	type feedbackSnapshot struct {
		Action           string
		CreatedAt        time.Time
		FeedbackOutputID uint64
	}
	feedbackByOutputID := map[uint64]feedbackSnapshot{}
	feedbackByEmojiID := map[uint64]feedbackSnapshot{}
	type feedbackRow struct {
		ID        uint64         `gorm:"column:id"`
		OutputID  *uint64        `gorm:"column:output_id"`
		Action    string         `gorm:"column:action"`
		Metadata  datatypes.JSON `gorm:"column:metadata"`
		CreatedAt time.Time      `gorm:"column:created_at"`
	}
	var feedbackRows []feedbackRow
	if err := h.db.Model(&models.VideoImageFeedbackPublic{}).
		Select("id", "output_id", "action", "metadata", "created_at").
		Where("job_id = ? AND user_id = ?", job.ID, userID).
		Order("created_at DESC, id DESC").
		Find(&feedbackRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, row := range feedbackRows {
		action := strings.TrimSpace(strings.ToLower(row.Action))
		if action == "" {
			continue
		}
		if row.OutputID != nil && *row.OutputID > 0 {
			if _, exists := feedbackByOutputID[*row.OutputID]; !exists {
				feedbackByOutputID[*row.OutputID] = feedbackSnapshot{
					Action:           action,
					CreatedAt:        row.CreatedAt,
					FeedbackOutputID: *row.OutputID,
				}
			}
			continue
		}
		meta := parseJSONMap(row.Metadata)
		emojiID := parseUint64FromAny(meta["emoji_id"])
		if emojiID == 0 {
			continue
		}
		if _, exists := feedbackByEmojiID[emojiID]; !exists {
			feedbackByEmojiID[emojiID] = feedbackSnapshot{
				Action:    action,
				CreatedAt: row.CreatedAt,
			}
		}
	}

	items := make([]VideoJobResultEmojiItem, 0, len(emojis))
	filteredOutCount := 0
	for _, item := range emojis {
		output := outputByObjectKey[strings.TrimSpace(item.FileURL)]
		outputID := output.ID
		reviewRecommendation := ""
		if outputID > 0 {
			reviewRecommendation = normalizeVideoJobReviewStatus(reviewByOutputID[outputID])
		}
		if reviewRecommendation == "" {
			reviewRecommendation = "deliver"
		}
		if reviewStatusSet != nil {
			if _, ok := reviewStatusSet[reviewRecommendation]; !ok {
				filteredOutCount++
				continue
			}
		}
		feedback := feedbackSnapshot{}
		if outputID > 0 {
			feedback = feedbackByOutputID[outputID]
		} else {
			feedback = feedbackByEmojiID[item.ID]
		}
		var feedbackAt *time.Time
		if !feedback.CreatedAt.IsZero() {
			at := feedback.CreatedAt
			feedbackAt = &at
		}
		items = append(items, VideoJobResultEmojiItem{
			ID:                          item.ID,
			OutputID:                    outputID,
			ReviewRecommendation:        reviewRecommendation,
			Title:                       item.Title,
			Format:                      item.Format,
			FileKey:                     item.FileURL,
			FileURL:                     resolvePreviewURL(item.FileURL, h.qiniu),
			ThumbKey:                    item.ThumbURL,
			ThumbURL:                    resolvePreviewURL(item.ThumbURL, h.qiniu),
			Width:                       item.Width,
			Height:                      item.Height,
			SizeBytes:                   item.SizeBytes,
			DisplayOrder:                item.DisplayOrder,
			FeedbackAction:              feedback.Action,
			FeedbackAt:                  feedbackAt,
			FeedbackOutputID:            feedback.FeedbackOutputID,
			OutputScore:                 output.Score,
			GIFLoopTuneApplied:          output.GIFLoopTuneApplied,
			GIFLoopTuneEffectiveApplied: output.GIFLoopTuneEffectiveApplied,
			GIFLoopTuneFallbackToBase:   output.GIFLoopTuneFallbackToBase,
			GIFLoopTuneScore:            output.GIFLoopTuneScore,
			GIFLoopTuneLoopClosure:      output.GIFLoopTuneLoopClosure,
			GIFLoopTuneMotionMean:       output.GIFLoopTuneMotionMean,
			GIFLoopTuneEffectiveSec:     output.GIFLoopTuneEffectiveSec,
		})
	}

	var packageItem *VideoJobResultPackageItem
	var latestZip models.CollectionZip
	if err := h.db.Where("collection_id = ?", collection.ID).
		Order("uploaded_at desc nulls last, id desc").
		First(&latestZip).Error; err == nil {
		packageItem = &VideoJobResultPackageItem{
			ID:         latestZip.ID,
			FileKey:    strings.TrimSpace(latestZip.ZipKey),
			FileURL:    resolvePreviewURL(latestZip.ZipKey, h.qiniu),
			FileName:   strings.TrimSpace(latestZip.ZipName),
			SizeBytes:  latestZip.SizeBytes,
			UploadedAt: latestZip.UploadedAt,
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if packageItem == nil && strings.TrimSpace(collection.LatestZipKey) != "" {
		packageItem = &VideoJobResultPackageItem{
			FileKey:    strings.TrimSpace(collection.LatestZipKey),
			FileURL:    resolvePreviewURL(collection.LatestZipKey, h.qiniu),
			FileName:   strings.TrimSpace(collection.LatestZipName),
			SizeBytes:  collection.LatestZipSize,
			UploadedAt: collection.LatestZipAt,
		}
	}
	if packageItem == nil {
		var pkg models.VideoImagePackagePublic
		if err := h.db.Where("job_id = ?", job.ID).First(&pkg).Error; err == nil {
			packageItem = &VideoJobResultPackageItem{
				ID:         pkg.ID,
				FileKey:    strings.TrimSpace(pkg.ZipObjectKey),
				FileURL:    resolvePreviewURL(pkg.ZipObjectKey, h.qiniu),
				FileName:   strings.TrimSpace(pkg.ZipName),
				SizeBytes:  pkg.ZipSizeBytes,
				UploadedAt: &pkg.CreatedAt,
			}
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	resp := VideoJobResultResponse{
		JobID:              job.ID,
		Status:             job.Status,
		DeliveryOnly:       deliveryOnly,
		ReviewStatusFilter: reviewStatusFilter,
		Collection: map[string]interface{}{
			"id":          collection.ID,
			"title":       collection.Title,
			"description": collection.Description,
			"cover_key":   resolveCollectionCoverKey(collection.CoverURL, h.qiniu),
			"cover_url":   resolvePreviewURL(collection.CoverURL, h.qiniu),
			"file_count":  collection.FileCount,
			"created_at":  collection.CreatedAt,
			"updated_at":  collection.UpdatedAt,
		},
		Emojis:  items,
		Package: packageItem,
		Options: parseJSONMap(job.Options),
		Metrics: parseJSONMap(job.Metrics),
	}
	if len(items) == 0 && len(emojis) > 0 && deliveryOnly {
		resp.Message = "当前任务暂无可交付结果（deliver）。请在后台查看 keep_internal/reject/need_manual_review。"
	} else if filteredOutCount > 0 {
		resp.Message = fmt.Sprintf("已按状态过滤，隐藏 %d 条结果。", filteredOutCount)
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) normalizeSourceVideoKey(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("source_video_key required")
	}
	key := raw
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		parsed, ok := extractQiniuObjectKey(raw, h.qiniu)
		if !ok {
			return "", errors.New("invalid source video url")
		}
		key = parsed
	}
	key = strings.TrimLeft(strings.SplitN(key, "?", 2)[0], "/")
	if key == "" {
		return "", errors.New("invalid source video key")
	}
	if !strings.HasPrefix(key, "emoji/") {
		return "", errors.New("source_video_key must start with emoji/")
	}
	ext := strings.ToLower(filepath.Ext(key))
	if _, ok := allowedVideoFileExt[ext]; !ok {
		return "", errors.New("unsupported video format")
	}
	return key, nil
}

func buildAspectRatio(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	a := width
	b := height
	for b != 0 {
		a, b = b, a%b
	}
	if a <= 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d", width/a, height/a)
}

func mimeTypeByVideoExt(ext string) string {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".") {
	case "mp4", "m4v":
		return "video/mp4"
	case "mov":
		return "video/quicktime"
	case "mkv":
		return "video/x-matroska"
	case "webm":
		return "video/webm"
	case "avi":
		return "video/x-msvideo"
	case "mpeg", "mpg":
		return "video/mpeg"
	case "wmv":
		return "video/x-ms-wmv"
	case "flv":
		return "video/x-flv"
	case "3gp":
		return "video/3gpp"
	case "ts", "mts", "m2ts":
		return "video/mp2t"
	default:
		return "video/*"
	}
}

func (h *Handler) lookupSourceVideoSizeBytes(sourceKey string) int64 {
	if h == nil || h.qiniu == nil {
		return 0
	}
	key := strings.TrimLeft(strings.TrimSpace(sourceKey), "/")
	if key == "" {
		return 0
	}
	info, err := h.qiniu.BucketManager().Stat(h.qiniu.Bucket, key)
	if err != nil {
		return 0
	}
	if info.Fsize <= 0 {
		return 0
	}
	return info.Fsize
}

func (h *Handler) preflightProbeSourceVideo(ctx context.Context, sourceKey string) (videojobs.ProbeMeta, error) {
	if h == nil || h.qiniu == nil {
		return videojobs.ProbeMeta{}, errors.New("video storage not configured")
	}
	cleanKey := strings.TrimLeft(strings.TrimSpace(sourceKey), "/")
	if cleanKey == "" {
		return videojobs.ProbeMeta{}, errors.New("source video key is empty")
	}

	meta, sdkErr := h.probeSourceVideoByObject(ctx, cleanKey)
	if sdkErr == nil {
		return meta, nil
	}

	probeURL, urlErr := h.resolveSourceVideoProbeURL(cleanKey)
	if urlErr != nil {
		return videojobs.ProbeMeta{}, fmt.Errorf("source video probe failed: sdk=%v; url=%w", sdkErr, urlErr)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	meta, err := videojobs.ProbeVideoSource(probeCtx, probeURL)
	if err != nil {
		return videojobs.ProbeMeta{}, fmt.Errorf("source video probe failed: sdk=%v; url_probe=%w", sdkErr, err)
	}
	return meta, nil
}

func (h *Handler) resolveSourceVideoProbeURL(sourceKey string) (string, error) {
	if h == nil || h.qiniu == nil {
		return "", errors.New("video storage not configured")
	}
	key := strings.TrimLeft(strings.TrimSpace(sourceKey), "/")
	if key == "" {
		return "", errors.New("source video key is empty")
	}
	if signed, err := h.qiniu.SignedURL(key, 900); err == nil {
		signed = strings.TrimSpace(signed)
		if strings.HasPrefix(signed, "http://") || strings.HasPrefix(signed, "https://") {
			return signed, nil
		}
	}
	publicURL := strings.TrimSpace(h.qiniu.PublicURL(key))
	if strings.HasPrefix(publicURL, "http://") || strings.HasPrefix(publicURL, "https://") {
		return publicURL, nil
	}
	return "", errors.New("unable to build source video probe url")
}

func (h *Handler) probeSourceVideoByObject(ctx context.Context, sourceKey string) (videojobs.ProbeMeta, error) {
	if h == nil || h.qiniu == nil {
		return videojobs.ProbeMeta{}, errors.New("video storage not configured")
	}
	cleanKey := strings.TrimLeft(strings.TrimSpace(sourceKey), "/")
	if cleanKey == "" {
		return videojobs.ProbeMeta{}, errors.New("source video key is empty")
	}

	output, err := h.qiniu.BucketManager().Get(h.qiniu.Bucket, cleanKey, nil)
	if err != nil {
		return videojobs.ProbeMeta{}, fmt.Errorf("qiniu get object failed: %w", err)
	}
	defer output.Close()

	tmpFile, err := os.CreateTemp("", "video-source-probe-*"+filepath.Ext(cleanKey))
	if err != nil {
		return videojobs.ProbeMeta{}, err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, output.Body); err != nil {
		return videojobs.ProbeMeta{}, fmt.Errorf("qiniu object stream copy failed: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		return videojobs.ProbeMeta{}, err
	}

	probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	meta, err := videojobs.ProbeVideoSource(probeCtx, tmpPath)
	if err != nil {
		return videojobs.ProbeMeta{}, fmt.Errorf("source video probe failed: %w", err)
	}
	return meta, nil
}

type sourceVideoProbeFailure struct {
	Code           string
	Message        string
	Hint           string
	MaxDurationSec int
}

func describeSourceVideoProbeFailure(err error) sourceVideoProbeFailure {
	detail := sourceVideoProbeFailure{
		Code:    "video_probe_failed",
		Message: "源视频无法识别，请更换文件后重试",
		Hint:    "建议上传常见编码格式的视频（如 MP4/MOV），并确认文件可正常播放。",
	}
	if err == nil {
		return detail
	}
	switch {
	case errors.Is(err, videojobs.ErrVideoStreamNotFound):
		return sourceVideoProbeFailure{
			Code:    "video_stream_missing",
			Message: "未检测到有效视频画面流",
			Hint:    "请确认上传的是视频文件（不是音频或损坏文件）。",
		}
	case errors.Is(err, videojobs.ErrVideoDurationMissing):
		return sourceVideoProbeFailure{
			Code:    "video_duration_invalid",
			Message: "视频时长信息读取失败",
			Hint:    "请先在本地导出为标准 MP4 后再上传。",
		}
	case errors.Is(err, videojobs.ErrVideoDurationTooLong):
		return sourceVideoProbeFailure{
			Code:           "video_duration_too_long",
			Message:        "视频时长超过当前处理上限",
			Hint:           "建议先裁剪为较短片段后再上传。",
			MaxDurationSec: videojobs.MaxAllowedProbeDurationSec,
		}
	case errors.Is(err, context.DeadlineExceeded):
		return sourceVideoProbeFailure{
			Code:    "video_probe_timeout",
			Message: "视频探测超时，请稍后重试",
			Hint:    "可先压缩文件体积或更换网络后再尝试。",
		}
	}

	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(lower, "qiniu get object failed"),
		strings.Contains(lower, "source video key is empty"),
		strings.Contains(lower, "unable to build source video probe url"),
		strings.Contains(lower, "qiniu signed url unavailable"),
		strings.Contains(lower, "qiniu public url unavailable"):
		return sourceVideoProbeFailure{
			Code:    "video_storage_unavailable",
			Message: "视频存储暂时不可用",
			Hint:    "请联系管理员检查七牛存储配置（下载域名/权限）后再试。",
		}
	case strings.Contains(lower, "moov atom not found"),
		strings.Contains(lower, "invalid data found"),
		strings.Contains(lower, "ffprobe failed"):
		return sourceVideoProbeFailure{
			Code:    "video_corrupted",
			Message: "视频文件可能损坏或编码异常",
			Hint:    "请重新导出视频后再上传。",
		}
	case strings.Contains(lower, "deadline exceeded"),
		strings.Contains(lower, "i/o timeout"),
		strings.Contains(lower, "timed out"):
		return sourceVideoProbeFailure{
			Code:    "video_probe_timeout",
			Message: "视频探测超时，请稍后重试",
			Hint:    "可先压缩文件体积或更换网络后再尝试。",
		}
	}
	return detail
}

func roundTo1(v float64) float64 {
	if v <= 0 {
		return 0
	}
	return float64(int(v*10+0.5)) / 10
}

func buildVideoJobResponse(job models.VideoJob, qiniu *storage.QiniuClient) VideoJobResponse {
	options := parseJSONMap(job.Options)
	metrics := parseJSONMap(job.Metrics)
	resp := VideoJobResponse{
		ID:                 job.ID,
		Title:              job.Title,
		SourceVideoKey:     job.SourceVideoKey,
		CategoryID:         job.CategoryID,
		OutputFormats:      splitFormats(job.OutputFormats),
		Status:             job.Status,
		Stage:              job.Stage,
		Progress:           job.Progress,
		Priority:           job.Priority,
		ErrorMessage:       strings.TrimSpace(job.ErrorMessage),
		ResultCollectionID: job.ResultCollectionID,
		Options:            options,
		Metrics:            metrics,
		QueuedAt:           job.QueuedAt,
		StartedAt:          job.StartedAt,
		FinishedAt:         job.FinishedAt,
		CreatedAt:          job.CreatedAt,
		UpdatedAt:          job.UpdatedAt,
	}
	if qiniu != nil {
		if !metricBool(metrics, "source_video_deleted") && strings.TrimSpace(job.SourceVideoKey) != "" {
			if signed, err := qiniu.SignedURL(job.SourceVideoKey, 0); err == nil && strings.TrimSpace(signed) != "" {
				resp.SourceVideoURL = signed
			} else {
				resp.SourceVideoURL = qiniu.PublicURL(job.SourceVideoKey)
			}
		}
	}
	return resp
}

type videoJobCollectionSummaryRow struct {
	ID           uint64 `gorm:"column:id"`
	Title        string `gorm:"column:title"`
	FileCount    int    `gorm:"column:file_count"`
	LatestZipKey string `gorm:"column:latest_zip_key"`
}

type videoJobEmojiSummaryRow struct {
	ID           uint64 `gorm:"column:id"`
	CollectionID uint64 `gorm:"column:collection_id"`
	Format       string `gorm:"column:format"`
	FileURL      string `gorm:"column:file_url"`
	ThumbURL     string `gorm:"column:thumb_url"`
	DisplayOrder int    `gorm:"column:display_order"`
}

type videoJobOutputQualityRow struct {
	JobID                  uint64  `gorm:"column:job_id"`
	Score                  float64 `gorm:"column:score"`
	GIFLoopTuneLoopClosure float64 `gorm:"column:gif_loop_tune_loop_closure"`
}

type videoJobResultSummaryAccumulator struct {
	Summary               VideoJobResultSummary
	ActiveCnt             int
	FormatCnt             map[string]int
	PreviewSet            map[string]struct{}
	QualityScoreSum       float64
	QualityLoopClosureSum float64
	QualityCnt            int
}

func shouldIncludeResultSummary(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func normalizeVideoJobResultFormat(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "jpeg" {
		return "jpg"
	}
	return value
}

func resolveVideoJobPackageStatus(job models.VideoJob, fallback string) string {
	current := strings.ToLower(strings.TrimSpace(fallback))
	if current == "ready" {
		return "ready"
	}
	metrics := parseJSONMap(job.Metrics)
	rawStatus := strings.ToLower(strings.TrimSpace(fmt.Sprint(metrics["package_zip_status"])))
	switch rawStatus {
	case "ready":
		return "ready"
	case "pending", "processing":
		return "processing"
	case "failed":
		return "failed"
	}
	if strings.EqualFold(strings.TrimSpace(job.Status), models.VideoJobStatusDone) {
		return "failed"
	}
	if current == "failed" {
		return "failed"
	}
	return "processing"
}

func (h *Handler) buildVideoJobResultSummary(jobs []models.VideoJob) (map[uint64]VideoJobResultSummary, error) {
	collectionIDs := make([]uint64, 0, len(jobs))
	jobIDs := make([]uint64, 0, len(jobs))
	seen := make(map[uint64]struct{}, len(jobs))
	jobToCollectionID := make(map[uint64]uint64, len(jobs))
	for _, job := range jobs {
		jobIDs = append(jobIDs, job.ID)
		if job.ResultCollectionID == nil || *job.ResultCollectionID == 0 {
			continue
		}
		id := *job.ResultCollectionID
		jobToCollectionID[job.ID] = id
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		collectionIDs = append(collectionIDs, id)
	}
	if len(collectionIDs) == 0 {
		return map[uint64]VideoJobResultSummary{}, nil
	}

	var collections []videoJobCollectionSummaryRow
	if err := h.db.Model(&models.Collection{}).
		Select("id", "title", "file_count", "latest_zip_key").
		Where("id IN ?", collectionIDs).
		Find(&collections).Error; err != nil {
		return nil, err
	}

	acc := make(map[uint64]*videoJobResultSummaryAccumulator, len(collections))
	for _, row := range collections {
		packageStatus := "processing"
		if strings.TrimSpace(row.LatestZipKey) != "" {
			packageStatus = "ready"
		}
		acc[row.ID] = &videoJobResultSummaryAccumulator{
			Summary: VideoJobResultSummary{
				CollectionID:    row.ID,
				CollectionTitle: strings.TrimSpace(row.Title),
				FileCount:       row.FileCount,
				PreviewImages:   make([]string, 0, 15),
				PackageStatus:   packageStatus,
			},
			FormatCnt:  map[string]int{},
			PreviewSet: map[string]struct{}{},
		}
	}
	if len(acc) == 0 {
		return map[uint64]VideoJobResultSummary{}, nil
	}

	var emojiRows []videoJobEmojiSummaryRow
	if err := h.db.Model(&models.Emoji{}).
		Select("id", "collection_id", "format", "file_url", "thumb_url", "display_order").
		Where("collection_id IN ? AND status = ?", collectionIDs, "active").
		Order("collection_id ASC, display_order ASC, id ASC").
		Find(&emojiRows).Error; err != nil {
		return nil, err
	}
	for _, row := range emojiRows {
		item, ok := acc[row.CollectionID]
		if !ok {
			continue
		}
		item.ActiveCnt++
		if format := normalizeVideoJobResultFormat(row.Format); format != "" {
			item.FormatCnt[format]++
		}
		if len(item.Summary.PreviewImages) >= 15 {
			continue
		}
		source := strings.TrimSpace(row.ThumbURL)
		if source == "" {
			source = strings.TrimSpace(row.FileURL)
		}
		previewURL := strings.TrimSpace(resolvePreviewURL(source, h.qiniu))
		if previewURL == "" {
			continue
		}
		if _, exists := item.PreviewSet[previewURL]; exists {
			continue
		}
		item.PreviewSet[previewURL] = struct{}{}
		item.Summary.PreviewImages = append(item.Summary.PreviewImages, previewURL)
	}

	if len(jobIDs) > 0 && len(jobToCollectionID) > 0 {
		var qualityRows []videoJobOutputQualityRow
		if err := h.db.Model(&models.VideoImageOutputPublic{}).
			Select("job_id", "score", "gif_loop_tune_loop_closure").
			Where("job_id IN ? AND file_role = ? AND format = ?", jobIDs, "main", "gif").
			Find(&qualityRows).Error; err != nil {
			return nil, err
		}
		for _, row := range qualityRows {
			collectionID, ok := jobToCollectionID[row.JobID]
			if !ok || collectionID == 0 {
				continue
			}
			item, exists := acc[collectionID]
			if !exists {
				continue
			}
			item.QualityCnt++
			item.QualityScoreSum += row.Score
			item.QualityLoopClosureSum += row.GIFLoopTuneLoopClosure
			if item.QualityCnt == 1 || row.Score > item.Summary.QualityTopScore {
				item.Summary.QualityTopScore = row.Score
			}
		}
	}

	out := make(map[uint64]VideoJobResultSummary, len(acc))
	for collectionID, item := range acc {
		if item.Summary.FileCount <= 0 {
			item.Summary.FileCount = item.ActiveCnt
		}
		if len(item.FormatCnt) > 0 {
			formats := make([]string, 0, len(item.FormatCnt))
			for format := range item.FormatCnt {
				formats = append(formats, format)
			}
			sort.Strings(formats)
			item.Summary.FormatSummary = make([]string, 0, len(formats))
			for _, format := range formats {
				item.Summary.FormatSummary = append(item.Summary.FormatSummary, fmt.Sprintf("%s × %d", strings.ToUpper(format), item.FormatCnt[format]))
			}
		}
		if item.QualityCnt > 0 {
			item.Summary.QualitySampleCount = item.QualityCnt
			item.Summary.QualityAvgScore = item.QualityScoreSum / float64(item.QualityCnt)
			item.Summary.QualityAvgLoopClosure = item.QualityLoopClosureSum / float64(item.QualityCnt)
		}
		out[collectionID] = item.Summary
	}

	return out, nil
}

func metricBool(metrics map[string]interface{}, key string) bool {
	if len(metrics) == 0 {
		return false
	}
	raw, ok := metrics[key]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func cleanupVideoSourceObject(qiniu *storage.QiniuClient, rawKey string) bool {
	if qiniu == nil {
		return false
	}
	key := strings.TrimLeft(strings.TrimSpace(rawKey), "/")
	if key == "" {
		return false
	}
	err := qiniu.BucketManager().Delete(qiniu.Bucket, key)
	if err == nil {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "612")
}

func parseJSONMap(raw datatypes.JSON) map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]interface{}{}
	}
	m := map[string]interface{}{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]interface{}{}
	}
	return m
}

func parseUint64FromAny(raw interface{}) uint64 {
	switch value := raw.(type) {
	case uint64:
		return value
	case uint32:
		return uint64(value)
	case uint:
		return uint64(value)
	case int64:
		if value <= 0 {
			return 0
		}
		return uint64(value)
	case int:
		if value <= 0 {
			return 0
		}
		return uint64(value)
	case float64:
		if value <= 0 {
			return 0
		}
		return uint64(value)
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func splitFormats(raw string) []string {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(strings.ToLower(part))
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{"jpg", "gif"}
	}
	return out
}

func normalizeVideoOutputFormats(in []string) []string {
	if len(in) == 0 {
		return []string{"gif"}
	}
	allow := map[string]struct{}{"jpg": {}, "jpeg": {}, "png": {}, "gif": {}, "webp": {}, "mp4": {}, "live": {}}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		f := strings.ToLower(strings.TrimSpace(item))
		if f == "jpeg" {
			f = "jpg"
		}
		if _, ok := allow[f]; !ok {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	if len(out) == 0 {
		return []string{"gif"}
	}
	return out
}

func normalizeVideoJobPriority(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "high" || raw == "vip" {
		return "high"
	}
	return "normal"
}

func normalizeVideoEditOptions(in *VideoJobEditOptionsInput) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	if in == nil {
		return out, nil
	}

	if in.StartSec < 0 {
		return nil, errors.New("edit_options.start_sec must be >= 0")
	}
	if in.EndSec < 0 {
		return nil, errors.New("edit_options.end_sec must be >= 0")
	}
	if in.StartSec > 0 {
		out["start_sec"] = in.StartSec
	}
	if in.EndSec > 0 {
		out["end_sec"] = in.EndSec
	}
	if in.StartSec > 0 && in.EndSec > 0 && in.EndSec <= in.StartSec {
		return nil, errors.New("edit_options.end_sec must be greater than start_sec")
	}

	if in.Speed > 0 {
		if in.Speed < 0.5 || in.Speed > 2.0 {
			return nil, errors.New("edit_options.speed must be between 0.5 and 2.0")
		}
		out["speed"] = in.Speed
	}

	if in.FPS > 0 {
		if in.FPS < 4 || in.FPS > 30 {
			return nil, errors.New("edit_options.fps must be between 4 and 30")
		}
		out["fps"] = in.FPS
	}

	if in.Width > 0 {
		if in.Width < 120 || in.Width > 1280 {
			return nil, errors.New("edit_options.width must be between 120 and 1280")
		}
		out["width"] = in.Width
	}

	if in.MaxColors > 0 {
		if in.MaxColors < 16 || in.MaxColors > 256 {
			return nil, errors.New("edit_options.max_colors must be between 16 and 256")
		}
		out["max_colors"] = in.MaxColors
	}

	if in.Crop != nil {
		if in.Crop.X < 0 || in.Crop.Y < 0 {
			return nil, errors.New("edit_options.crop x/y must be >= 0")
		}
		if in.Crop.W <= 0 || in.Crop.H <= 0 {
			return nil, errors.New("edit_options.crop w/h must be > 0")
		}
		out["crop_x"] = in.Crop.X
		out["crop_y"] = in.Crop.Y
		out["crop_w"] = in.Crop.W
		out["crop_h"] = in.Crop.H
	}

	return out, nil
}

func classifyVideoDurationBucket(durationSec float64) string {
	switch {
	case durationSec <= 0:
		return ""
	case durationSec < 5:
		return "<5s"
	case durationSec < 10:
		return "5-10s"
	case durationSec < 30:
		return "10-30s"
	case durationSec < 60:
		return "30-60s"
	case durationSec < 180:
		return "1-3m"
	case durationSec < 600:
		return "3-10m"
	default:
		return "10m+"
	}
}

func classifyVideoResolutionBucket(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	switch {
	case width >= 3840 || height >= 2160:
		return "4k+"
	case width >= 2560 || height >= 1440:
		return "2k"
	case width >= 1920 || height >= 1080:
		return "1080p"
	case width >= 1280 || height >= 720:
		return "720p"
	case width >= 854 || height >= 480:
		return "480p"
	default:
		return "<480p"
	}
}

func classifyVideoFPSBucket(fps float64) string {
	switch {
	case fps <= 0:
		return ""
	case fps < 15:
		return "<15fps"
	case fps < 24:
		return "15-24fps"
	case fps < 30:
		return "24-30fps"
	case fps < 60:
		return "30-60fps"
	default:
		return "60fps+"
	}
}

func supportsQualityTemplateProfile(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "gif", "webp", "live", "jpg", "png", "mp4":
		return true
	default:
		return false
	}
}

func normalizeQualityProfileValue(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case videojobs.QualityProfileClarity:
		return videojobs.QualityProfileClarity, true
	case videojobs.QualityProfileSize:
		return videojobs.QualityProfileSize, true
	default:
		return "", false
	}
}

func stringFromAny(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return fmt.Sprintf("%v", raw)
	}
}

func extractRecommendedProfilesFromRecommendation(recommendation map[string]interface{}) map[string]string {
	out := map[string]string{}
	if len(recommendation) == 0 {
		return out
	}
	raw, ok := recommendation["recommended_profiles"]
	if !ok || raw == nil {
		return out
	}
	profiles, ok := raw.(map[string]interface{})
	if !ok {
		return out
	}
	for key, value := range profiles {
		format := strings.ToLower(strings.TrimSpace(key))
		if !supportsQualityTemplateProfile(format) {
			continue
		}
		profile, ok := normalizeQualityProfileValue(stringFromAny(value))
		if !ok {
			continue
		}
		out[format] = profile
	}
	return out
}

func (h *Handler) loadSourceProbeBucketQualitySnapshot(since time.Time, dimension, bucket string) (*AdminVideoJobSourceProbeQualityStat, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, nil
	}
	bucketExpr, _, filterExpr, ok := sourceProbeBucketSQL(dimension)
	if !ok {
		return nil, nil
	}

	type row struct {
		Jobs           int64    `gorm:"column:jobs"`
		DoneJobs       int64    `gorm:"column:done_jobs"`
		FailedJobs     int64    `gorm:"column:failed_jobs"`
		PendingJobs    int64    `gorm:"column:pending_jobs"`
		CancelledJobs  int64    `gorm:"column:cancelled_jobs"`
		DurationP50Sec *float64 `gorm:"column:duration_p50_sec"`
		DurationP95Sec *float64 `gorm:"column:duration_p95_sec"`
	}

	query := fmt.Sprintf(`
WITH probe AS (
	SELECT
		COALESCE(LOWER(TRIM(v.status)), '') AS status,
		v.started_at,
		v.finished_at,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'duration_sec') = 'number'
			THEN (v.options->'source_video_probe'->>'duration_sec')::double precision
			ELSE NULL
		END AS duration_sec,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'width') = 'number'
			THEN (v.options->'source_video_probe'->>'width')::double precision
			ELSE NULL
		END AS width,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'height') = 'number'
			THEN (v.options->'source_video_probe'->>'height')::double precision
			ELSE NULL
		END AS height,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'fps') = 'number'
			THEN (v.options->'source_video_probe'->>'fps')::double precision
			ELSE NULL
		END AS fps
	FROM archive.video_jobs v
	WHERE v.created_at >= ?
		AND jsonb_typeof(v.options->'source_video_probe') = 'object'
)
SELECT
	COUNT(*)::bigint AS jobs,
	COUNT(*) FILTER (WHERE status = ?)::bigint AS done_jobs,
	COUNT(*) FILTER (WHERE status = ?)::bigint AS failed_jobs,
	COUNT(*) FILTER (WHERE status = ?)::bigint AS cancelled_jobs,
	COUNT(*) FILTER (WHERE status NOT IN (?, ?, ?))::bigint AS pending_jobs,
	percentile_cont(0.50) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at)))
		FILTER (WHERE status = ? AND started_at IS NOT NULL AND finished_at IS NOT NULL AND finished_at > started_at) AS duration_p50_sec,
	percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at)))
		FILTER (WHERE status = ? AND started_at IS NOT NULL AND finished_at IS NOT NULL AND finished_at > started_at) AS duration_p95_sec
FROM (
	SELECT
		%s AS bucket,
		status,
		started_at,
		finished_at
	FROM probe
	WHERE %s
) b
WHERE bucket = ?
`, bucketExpr, filterExpr)

	var snapshot row
	if err := h.db.Raw(
		query,
		since,
		models.VideoJobStatusDone,
		models.VideoJobStatusFailed,
		models.VideoJobStatusCancelled,
		models.VideoJobStatusDone,
		models.VideoJobStatusFailed,
		models.VideoJobStatusCancelled,
		models.VideoJobStatusDone,
		models.VideoJobStatusDone,
		bucket,
	).Scan(&snapshot).Error; err != nil {
		return nil, err
	}
	if snapshot.Jobs <= 0 {
		return nil, nil
	}

	stat := &AdminVideoJobSourceProbeQualityStat{
		Bucket:        bucket,
		Jobs:          snapshot.Jobs,
		DoneJobs:      snapshot.DoneJobs,
		FailedJobs:    snapshot.FailedJobs,
		PendingJobs:   snapshot.PendingJobs,
		CancelledJobs: snapshot.CancelledJobs,
	}
	stat.TerminalJobs = stat.DoneJobs + stat.FailedJobs
	if stat.TerminalJobs > 0 {
		stat.SuccessRate = float64(stat.DoneJobs) / float64(stat.TerminalJobs)
		stat.FailureRate = float64(stat.FailedJobs) / float64(stat.TerminalJobs)
	}
	if snapshot.DurationP50Sec != nil {
		stat.DurationP50Sec = *snapshot.DurationP50Sec
	}
	if snapshot.DurationP95Sec != nil {
		stat.DurationP95Sec = *snapshot.DurationP95Sec
	}
	return stat, nil
}

func recommendFormatProfilesForVideo(
	formats []string,
	durationBucket string,
	resolutionBucket string,
	fpsBucket string,
	durationStat *AdminVideoJobSourceProbeQualityStat,
	resolutionStat *AdminVideoJobSourceProbeQualityStat,
	fpsStat *AdminVideoJobSourceProbeQualityStat,
) (map[string]string, []string) {
	recommended := map[string]string{}
	for _, raw := range formats {
		format := strings.ToLower(strings.TrimSpace(raw))
		if !supportsQualityTemplateProfile(format) {
			continue
		}
		recommended[format] = videojobs.QualityProfileClarity
	}

	reasons := make([]string, 0, 6)
	longDuration := durationBucket == "3-10m" || durationBucket == "10m+"
	veryLongDuration := durationBucket == "10m+"
	highFPS := fpsBucket == "60fps+"
	lowResolution := resolutionBucket == "<480p" || resolutionBucket == "480p"
	highResolution := resolutionBucket == "4k+" || resolutionBucket == "2k"

	if longDuration {
		reasons = append(reasons, "检测到输入时长偏长，动图默认建议体积优先以提升稳定性。")
	}
	if highFPS {
		reasons = append(reasons, "检测到高帧率输入，动图默认建议体积优先以降低产物体积和渲染压力。")
	}
	if lowResolution {
		reasons = append(reasons, "检测到低分辨率输入，静态图默认优先保清晰。")
	}
	if highResolution {
		reasons = append(reasons, "检测到高分辨率输入，PNG 默认建议体积优先以控制大图输出成本。")
	}

	isRisky := func(stat *AdminVideoJobSourceProbeQualityStat) bool {
		if stat == nil || stat.TerminalJobs < 8 {
			return false
		}
		return stat.FailureRate >= 0.18 || stat.DurationP95Sec >= 180
	}
	hasRiskBucket := isRisky(durationStat) || isRisky(resolutionStat) || isRisky(fpsStat)
	if hasRiskBucket {
		reasons = append(reasons, "历史同类输入桶存在失败率/长尾耗时风险，建议动图先走体积优先。")
	}

	useAnimatedSize := longDuration || highFPS || hasRiskBucket
	if useAnimatedSize {
		for _, format := range []string{"gif", "webp", "live", "mp4"} {
			if _, ok := recommended[format]; ok {
				recommended[format] = videojobs.QualityProfileSize
			}
		}
	}

	if highResolution || veryLongDuration {
		if _, ok := recommended["png"]; ok {
			recommended["png"] = videojobs.QualityProfileSize
		}
	}
	if lowResolution && resolutionStat != nil && resolutionStat.TerminalJobs >= 8 && resolutionStat.FailureRate >= 0.22 {
		if _, ok := recommended["png"]; ok {
			recommended["png"] = videojobs.QualityProfileSize
			reasons = append(reasons, "低清输入在历史样本中失败率偏高，PNG 建议体积优先保成功率。")
		}
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "默认建议：静态图与动图都走清晰优先。")
	}
	return recommended, reasons
}

func (h *Handler) buildQualityTemplateRecommendation(probe videojobs.ProbeMeta, formats []string) map[string]interface{} {
	if len(formats) == 0 {
		return nil
	}
	durationBucket := classifyVideoDurationBucket(probe.DurationSec)
	resolutionBucket := classifyVideoResolutionBucket(probe.Width, probe.Height)
	fpsBucket := classifyVideoFPSBucket(probe.FPS)
	if durationBucket == "" && resolutionBucket == "" && fpsBucket == "" {
		return nil
	}

	since := time.Now().Add(-7 * 24 * time.Hour)
	var durationStat *AdminVideoJobSourceProbeQualityStat
	var resolutionStat *AdminVideoJobSourceProbeQualityStat
	var fpsStat *AdminVideoJobSourceProbeQualityStat
	if h != nil && h.db != nil {
		if stat, err := h.loadSourceProbeBucketQualitySnapshot(since, "duration", durationBucket); err == nil {
			durationStat = stat
		}
		if stat, err := h.loadSourceProbeBucketQualitySnapshot(since, "resolution", resolutionBucket); err == nil {
			resolutionStat = stat
		}
		if stat, err := h.loadSourceProbeBucketQualitySnapshot(since, "fps", fpsBucket); err == nil {
			fpsStat = stat
		}
	}

	recommendedProfiles, reasons := recommendFormatProfilesForVideo(
		formats,
		durationBucket,
		resolutionBucket,
		fpsBucket,
		durationStat,
		resolutionStat,
		fpsStat,
	)
	if len(recommendedProfiles) == 0 {
		return nil
	}

	diagnosis := map[string]interface{}{}
	if durationStat != nil {
		diagnosis["duration"] = map[string]interface{}{
			"bucket":           durationStat.Bucket,
			"jobs":             durationStat.Jobs,
			"terminal_jobs":    durationStat.TerminalJobs,
			"failure_rate":     roundTo1(durationStat.FailureRate * 100),
			"duration_p95_sec": roundTo1(durationStat.DurationP95Sec),
		}
	}
	if resolutionStat != nil {
		diagnosis["resolution"] = map[string]interface{}{
			"bucket":           resolutionStat.Bucket,
			"jobs":             resolutionStat.Jobs,
			"terminal_jobs":    resolutionStat.TerminalJobs,
			"failure_rate":     roundTo1(resolutionStat.FailureRate * 100),
			"duration_p95_sec": roundTo1(resolutionStat.DurationP95Sec),
		}
	}
	if fpsStat != nil {
		diagnosis["fps"] = map[string]interface{}{
			"bucket":           fpsStat.Bucket,
			"jobs":             fpsStat.Jobs,
			"terminal_jobs":    fpsStat.TerminalJobs,
			"failure_rate":     roundTo1(fpsStat.FailureRate * 100),
			"duration_p95_sec": roundTo1(fpsStat.DurationP95Sec),
		}
	}

	return map[string]interface{}{
		"version":    "v1",
		"window":     "7d",
		"auto_apply": false,
		"source_buckets": map[string]string{
			"duration":   durationBucket,
			"resolution": resolutionBucket,
			"fps":        fpsBucket,
		},
		"recommended_profiles": recommendedProfiles,
		"reasons":              reasons,
		"diagnosis":            diagnosis,
	}
}

func toJSON(v interface{}) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte("{}"))
	}
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(b)
}
