package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	Title            string                        `json:"title"`
	Prompt           string                        `json:"prompt,omitempty"`
	AIModel          string                        `json:"ai_model,omitempty"`
	FlowMode         string                        `json:"flow_mode,omitempty"`
	AdvancedOptions  *VideoJobAdvancedOptionsInput `json:"advanced_options,omitempty"`
	CategoryID       *uint64                       `json:"category_id"`
	SourceVideoKey   string                        `json:"source_video_key"`
	OutputFormats    []string                      `json:"output_formats"`
	GIFPipelineMode  string                        `json:"gif_pipeline_mode,omitempty"`
	MaxStatic        int                           `json:"max_static"`
	FrameIntervalSec float64                       `json:"frame_interval_sec"`
	AutoHighlight    *bool                         `json:"auto_highlight"`
	Priority         string                        `json:"priority"`
	EditOptions      *VideoJobEditOptionsInput     `json:"edit_options"`
}

type VideoJobAdvancedOptionsInput struct {
	Scene         string   `json:"scene,omitempty"`
	Scenario      string   `json:"scenario,omitempty"`
	VisualFocus   []string `json:"visual_focus,omitempty"`
	EnableMatting *bool    `json:"enable_matting,omitempty"`
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
	AssetDomain        string                 `json:"asset_domain,omitempty"`
	ErrorMessage       string                 `json:"error_message,omitempty"`
	ResultCollectionID *uint64                `json:"result_collection_id,omitempty"`
	Options            map[string]interface{} `json:"options,omitempty"`
	Metrics            map[string]interface{} `json:"metrics,omitempty"`
	QueuedAt           time.Time              `json:"queued_at"`
	StartedAt          *time.Time             `json:"started_at,omitempty"`
	FinishedAt         *time.Time             `json:"finished_at,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	Billing            *VideoJobBillingInfo   `json:"billing,omitempty"`
	ResultSummary      *VideoJobResultSummary `json:"result_summary,omitempty"`
}

type VideoJobBillingInfo struct {
	ActualCostCNY        float64 `json:"actual_cost_cny"`
	Currency             string  `json:"currency,omitempty"`
	PricingVersion       string  `json:"pricing_version,omitempty"`
	ChargedPoints        int64   `json:"charged_points"`
	ReservedPoints       int64   `json:"reserved_points"`
	HoldStatus           string  `json:"hold_status,omitempty"`
	PointPerCNY          float64 `json:"point_per_cny"`
	CostMarkupMultiplier float64 `json:"cost_markup_multiplier"`
}

type VideoJobResultSummary struct {
	CollectionID          uint64   `json:"collection_id,omitempty"`
	CollectionTitle       string   `json:"collection_title,omitempty"`
	FileCount             int      `json:"file_count"`
	PreviewImages         []string `json:"preview_images,omitempty"`
	FormatSummary         []string `json:"format_summary,omitempty"`
	PackageStatus         string   `json:"package_status,omitempty"`
	PackageSizeBytes      int64    `json:"package_size_bytes,omitempty"`
	OutputTotalSizeBytes  int64    `json:"output_total_size_bytes,omitempty"`
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
	Billing            *VideoJobBillingInfo       `json:"billing,omitempty"`
	Collection         map[string]interface{}     `json:"collection,omitempty"`
	Emojis             []VideoJobResultEmojiItem  `json:"emojis,omitempty"`
	Package            *VideoJobResultPackageItem `json:"package,omitempty"`
	Options            map[string]interface{}     `json:"options,omitempty"`
	Metrics            map[string]interface{}     `json:"metrics,omitempty"`
	Message            string                     `json:"message,omitempty"`
}

type VideoJobEventItemResponse struct {
	ID        uint64                 `json:"id"`
	Stage     string                 `json:"stage"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

type VideoJobEventListResponse struct {
	Items       []VideoJobEventItemResponse `json:"items"`
	NextSinceID uint64                      `json:"next_since_id"`
}

type VideoJobAI1PlanResponse struct {
	JobID           uint64                 `json:"job_id"`
	RequestedFormat string                 `json:"requested_format"`
	SchemaVersion   string                 `json:"schema_version"`
	PlanRevision    int                    `json:"plan_revision"`
	Status          string                 `json:"status"`
	SourcePrompt    string                 `json:"source_prompt,omitempty"`
	Plan            map[string]interface{} `json:"plan,omitempty"`
	ModelProvider   string                 `json:"model_provider,omitempty"`
	ModelName       string                 `json:"model_name,omitempty"`
	PromptVersion   string                 `json:"prompt_version,omitempty"`
	FallbackUsed    bool                   `json:"fallback_used"`
	ConfirmedByUser bool                   `json:"confirmed_by_user"`
	ConfirmedAt     *time.Time             `json:"confirmed_at,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

type PatchVideoJobAI1PlanRequest struct {
	PlanRevision        *int                `json:"plan_revision"`
	Summary             *string             `json:"summary"`
	IntentUnderstanding *string             `json:"intent_understanding"`
	StrategySummary     *string             `json:"strategy_summary"`
	InteractiveAction   *string             `json:"interactive_action"`
	ClarifyQuestions    *[]string           `json:"clarify_questions"`
	Objective           *string             `json:"objective"`
	MustCapture         *[]string           `json:"must_capture"`
	Avoid               *[]string           `json:"avoid"`
	StyleDirection      *string             `json:"style_direction"`
	QualityWeights      *map[string]float64 `json:"quality_weights"`
	RiskFlags           *[]string           `json:"risk_flags"`
	MaxBlurTolerance    *string             `json:"max_blur_tolerance"`
	AvoidWatermarks     *bool               `json:"avoid_watermarks"`
	AvoidExtremeDark    *bool               `json:"avoid_extreme_dark"`
	TargetCount         *int                `json:"target_count"`
	FrameIntervalSec    *float64            `json:"frame_interval_sec"`
	FocusStartSec       *float64            `json:"focus_start_sec"`
	FocusEndSec         *float64            `json:"focus_end_sec"`
}

type ConfirmVideoJobAI1Request struct {
	PlanRevision *int `json:"plan_revision"`
}

type VideoJobAI1DebugResponse struct {
	JobID           uint64                 `json:"job_id"`
	RequestedFormat string                 `json:"requested_format"`
	FlowMode        string                 `json:"flow_mode"`
	Stage           string                 `json:"stage"`
	Status          string                 `json:"status"`
	SourcePrompt    string                 `json:"source_prompt,omitempty"`
	Input           map[string]interface{} `json:"input,omitempty"`
	ModelRequest    map[string]interface{} `json:"model_request,omitempty"`
	ModelResponse   map[string]interface{} `json:"model_response,omitempty"`
	Output          map[string]interface{} `json:"output,omitempty"`
	Focus           map[string]interface{} `json:"focus,omitempty"`
	Trace           map[string]interface{} `json:"trace,omitempty"`
}

type VideoJobCapabilitiesResponse struct {
	FFmpegAvailable    bool                         `json:"ffmpeg_available"`
	FFprobeAvailable   bool                         `json:"ffprobe_available"`
	GifsicleAvailable  bool                         `json:"gifsicle_available"`
	GifsiclePath       string                       `json:"gifsicle_path,omitempty"`
	SupportedFormats   []string                     `json:"supported_formats"`
	UnsupportedFormats []string                     `json:"unsupported_formats"`
	Formats            []videojobs.FormatCapability `json:"formats"`
}

type VideoJobAdvancedSceneOptionItem struct {
	Scene             string `json:"scene"`
	Label             string `json:"label"`
	Description       string `json:"description,omitempty"`
	OperatorIdentity  string `json:"operator_identity,omitempty"`
	CandidateCountMin int    `json:"candidate_count_min,omitempty"`
	CandidateCountMax int    `json:"candidate_count_max,omitempty"`
}

type VideoJobAdvancedSceneOptionsResponse struct {
	Format       string                            `json:"format"`
	ResolvedFrom string                            `json:"resolved_from"`
	Version      string                            `json:"version,omitempty"`
	Items        []VideoJobAdvancedSceneOptionItem `json:"items"`
}

type ProbeSourceVideoRequest struct {
	SourceVideoKey string `json:"source_video_key"`
}

type ProbeSourceVideoURLRequest struct {
	SourceURL string `json:"source_url"`
}

type ProbeSourceVideoURLResponse struct {
	SourceURL      string `json:"source_url"`
	NormalizedURL  string `json:"normalized_url"`
	Provider       string `json:"provider"`
	ProviderLabel  string `json:"provider_label"`
	SourceType     string `json:"source_type"`
	MockOnly       bool   `json:"mock_only"`
	Supported      bool   `json:"supported"`
	NeedsIngestion bool   `json:"needs_ingestion"`
	Message        string `json:"message"`
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
	prompt := strings.TrimSpace(req.Prompt)
	if title == "" {
		if prompt != "" {
			title = prompt
		} else {
			title = fmt.Sprintf("视频表情包-%s", time.Now().Format("20060102150405"))
		}
	}

	options := map[string]interface{}{}
	if prompt != "" {
		options["user_prompt"] = prompt
	}
	if model := strings.TrimSpace(req.AIModel); model != "" {
		options["ai_model_preference"] = model
	}
	flowMode := strings.ToLower(strings.TrimSpace(req.FlowMode))
	switch flowMode {
	case "", "direct":
		options["flow_mode"] = "direct"
	case "ai1_confirm":
		options["flow_mode"] = "ai1_confirm"
		options["ai1_pending"] = false
		options["ai1_confirmed"] = false
		options["ai1_pause_consumed"] = false
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "flow_mode must be one of: direct, ai1_confirm"})
		return
	}
	initialQueue, initialTaskType, primaryFormat := videojobs.ResolveVideoJobExecutionTarget(strings.Join(formats, ","))
	options["execution_queue"] = initialQueue
	options["execution_task_type"] = initialTaskType
	if primaryFormat != "" {
		options["requested_format"] = primaryFormat
	}
	if req.AdvancedOptions != nil {
		enableMatting := false
		if req.AdvancedOptions.EnableMatting != nil {
			enableMatting = *req.AdvancedOptions.EnableMatting
		}
		advanced := videojobs.NormalizeVideoJobAdvancedOptions(videojobs.VideoJobAdvancedOptions{
			Scene: firstNonEmptyString(
				strings.TrimSpace(req.AdvancedOptions.Scene),
				strings.TrimSpace(req.AdvancedOptions.Scenario),
			),
			VisualFocus:   req.AdvancedOptions.VisualFocus,
			EnableMatting: enableMatting,
		})
		strategy := videojobs.ResolveVideoJobAI1StrategyProfile(primaryFormat, advanced)
		options["ai1_advanced_options_v1"] = videojobs.AdvancedOptionsToMap(advanced)
		options["ai1_strategy_profile_v1"] = videojobs.StrategyProfileToMap(strategy)
		options["ai1_strategy_profile_version"] = strings.TrimSpace(strategy.Version)
	}
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
		options["user_requested_max_static"] = req.MaxStatic
	}
	if req.FrameIntervalSec > 0 {
		options["frame_interval_sec"] = req.FrameIntervalSec
	}
	if mode := strings.ToLower(strings.TrimSpace(req.GIFPipelineMode)); mode != "" {
		switch mode {
		case "light", "standard", "hq":
			options["gif_pipeline_mode"] = mode
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "gif_pipeline_mode must be one of: light, standard, hq"})
			return
		}
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
		if h.cfg.VideoSourceProbeAllowDegraded {
			options["source_video_probe"] = map[string]interface{}{
				"duration_sec": 0,
				"width":        0,
				"height":       0,
				"fps":          0,
				"degraded":     true,
				"reason_code":  detail.Code,
				"message":      detail.Message,
				"hint":         detail.Hint,
			}
			options["source_video_probe_degraded_v1"] = map[string]interface{}{
				"enabled":     true,
				"reason_code": detail.Code,
				"message":     detail.Message,
				"hint":        detail.Hint,
				"error":       err.Error(),
			}
		} else {
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
	} else {
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
		AssetDomain:    models.VideoJobAssetDomainVideo,
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

	task, queueName, _, err := videojobs.NewProcessVideoJobTaskByFormat(job.ID, job.OutputFormats)
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
		asynq.Queue(queueName),
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
		GifsicleAvailable:  capabilities.GifsicleAvailable,
		GifsiclePath:       capabilities.GifsiclePath,
		SupportedFormats:   capabilities.SupportedFormats,
		UnsupportedFormats: capabilities.UnsupportedFormats,
		Formats:            capabilities.Formats,
	})
}

// GetVideoJobAdvancedSceneOptions godoc
// @Summary Get advanced scene options for video-to-image AI1
// @Tags user
// @Produce json
// @Param format query string false "target format, e.g. png/gif/jpg/webp/live"
// @Success 200 {object} VideoJobAdvancedSceneOptionsResponse
// @Router /api/video-jobs/advanced-scene-options [get]
func (h *Handler) GetVideoJobAdvancedSceneOptions(c *gin.Context) {
	format, err := normalizeVideoAIPromptTemplateFormat(c.Query("format"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if format == videoAIPromptTemplateFormatAll {
		format = videoAIPromptTemplateFormatPNG
	}

	profiles, resolvedFrom, version := h.resolveVideoJobAdvancedSceneProfiles(format)
	profiles = filterVideoJobAdvancedSceneProfilesByMainline(format, profiles)
	items := buildVideoJobAdvancedSceneOptionItems(profiles)
	c.JSON(http.StatusOK, VideoJobAdvancedSceneOptionsResponse{
		Format:       format,
		ResolvedFrom: resolvedFrom,
		Version:      version,
		Items:        items,
	})
}

func filterVideoJobAdvancedSceneProfilesByMainline(
	format string,
	profiles map[string]videojobs.VideoJobAI1StrategyProfile,
) map[string]videojobs.VideoJobAI1StrategyProfile {
	if strings.ToLower(strings.TrimSpace(format)) != videoAIPromptTemplateFormatPNG {
		return profiles
	}

	allowed := []string{
		videojobs.AdvancedScenarioDefault,
		videojobs.AdvancedScenarioXiaohongshu,
	}
	normalized := make(map[string]videojobs.VideoJobAI1StrategyProfile, len(allowed))
	for rawScene, profile := range profiles {
		scene := videojobs.NormalizeAdvancedScenario(firstNonEmptyString(profile.Scene, rawScene))
		if scene != videojobs.AdvancedScenarioDefault && scene != videojobs.AdvancedScenarioXiaohongshu {
			continue
		}
		profile.Scene = scene
		if strings.TrimSpace(profile.SceneLabel) == "" {
			fallback := videojobs.ResolveVideoJobAI1StrategyProfile(videoAIPromptTemplateFormatPNG, videojobs.VideoJobAdvancedOptions{Scene: scene})
			profile.SceneLabel = fallback.SceneLabel
		}
		normalized[scene] = profile
	}

	out := make(map[string]videojobs.VideoJobAI1StrategyProfile, len(allowed))
	for _, scene := range allowed {
		if profile, ok := normalized[scene]; ok {
			out[scene] = profile
			continue
		}
		out[scene] = videojobs.ResolveVideoJobAI1StrategyProfile(videoAIPromptTemplateFormatPNG, videojobs.VideoJobAdvancedOptions{Scene: scene})
	}
	return out
}

func (h *Handler) resolveVideoJobAdvancedSceneProfiles(format string) (map[string]videojobs.VideoJobAI1StrategyProfile, string, string) {
	candidates := make([]string, 0, 3)
	push := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return
		}
		for _, item := range candidates {
			if item == value {
				return
			}
		}
		candidates = append(candidates, value)
	}
	push(format)
	push(videoAIPromptTemplateFormatAll)
	push(videoAIPromptTemplateFormatPNG)

	for _, candidate := range candidates {
		var row models.VideoAIPromptTemplate
		err := h.db.
			Where("format = ? AND stage = ? AND layer = ? AND is_active = ? AND enabled = ?", candidate, videoAIPromptTemplateStageAI1, videoAIPromptTemplateLayerEdit, true, true).
			Order("id DESC").
			First(&row).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableError(err, "video_ai_prompt_templates") {
				continue
			}
			continue
		}
		profiles, version := videojobs.DecodeAI1SceneStrategyProfilesFromTemplateSchema([]byte(row.TemplateJSONSchema))
		if len(profiles) == 0 {
			continue
		}
		if strings.TrimSpace(version) == "" {
			version = strings.TrimSpace(row.Version)
		}
		return profiles, "ops.video_ai_prompt_templates:" + candidate, version
	}

	fallback := map[string]videojobs.VideoJobAI1StrategyProfile{}
	for _, scene := range []string{
		videojobs.AdvancedScenarioDefault,
		videojobs.AdvancedScenarioXiaohongshu,
		videojobs.AdvancedScenarioWallpaper,
		videojobs.AdvancedScenarioNews,
	} {
		profile := videojobs.ResolveVideoJobAI1StrategyProfile(format, videojobs.VideoJobAdvancedOptions{Scene: scene})
		fallback[profile.Scene] = profile
	}
	return fallback, "built_in_default", "ai1_strategy_profile_v1"
}

func buildVideoJobAdvancedSceneOptionItems(profiles map[string]videojobs.VideoJobAI1StrategyProfile) []VideoJobAdvancedSceneOptionItem {
	if len(profiles) == 0 {
		return []VideoJobAdvancedSceneOptionItem{}
	}

	keys := make([]string, 0, len(profiles))
	for key := range profiles {
		scene := videojobs.NormalizeAdvancedScenario(key)
		if scene == "" {
			continue
		}
		keys = append(keys, scene)
	}
	sort.Strings(keys)

	out := make([]VideoJobAdvancedSceneOptionItem, 0, len(keys))
	appendScene := func(scene string) {
		profile, ok := profiles[scene]
		if !ok {
			return
		}
		label := strings.TrimSpace(profile.SceneLabel)
		if label == "" {
			label = scene
		}
		minCount := profile.CandidateCountMin
		maxCount := profile.CandidateCountMax
		if minCount < 0 {
			minCount = 0
		}
		if maxCount < minCount {
			maxCount = minCount
		}
		out = append(out, VideoJobAdvancedSceneOptionItem{
			Scene:             scene,
			Label:             label,
			Description:       strings.TrimSpace(profile.DirectiveHint),
			OperatorIdentity:  strings.TrimSpace(profile.OperatorIdentity),
			CandidateCountMin: minCount,
			CandidateCountMax: maxCount,
		})
	}

	seen := map[string]struct{}{}
	for _, scene := range []string{
		videojobs.AdvancedScenarioDefault,
		videojobs.AdvancedScenarioXiaohongshu,
		videojobs.AdvancedScenarioWallpaper,
		videojobs.AdvancedScenarioNews,
	} {
		if _, ok := profiles[scene]; ok {
			appendScene(scene)
			seen[scene] = struct{}{}
		}
	}
	for _, scene := range keys {
		if _, ok := seen[scene]; ok {
			continue
		}
		appendScene(scene)
	}
	return out
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

// ProbeSourceVideoURLMock godoc
// @Summary Probe external video url (mock placeholder)
// @Tags user
// @Accept json
// @Produce json
// @Param body body ProbeSourceVideoURLRequest true "external source url"
// @Success 200 {object} ProbeSourceVideoURLResponse
// @Router /api/video-jobs/source-url-probe [post]
func (h *Handler) ProbeSourceVideoURLMock(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req ProbeSourceVideoURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	normalizedURL, provider, providerLabel, sourceType, err := normalizeExternalVideoSourceURL(req.SourceURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	message := "链接解析成功（Mock占位）。当前阶段暂不支持直接用外链创建任务，请先本地上传视频。"
	if sourceType == "direct_video_url" {
		message = "检测到可疑似直链视频地址（Mock占位）。后续将支持自动拉取并入库后创建任务。当前请先本地上传。"
	}

	c.JSON(http.StatusOK, ProbeSourceVideoURLResponse{
		SourceURL:      strings.TrimSpace(req.SourceURL),
		NormalizedURL:  normalizedURL,
		Provider:       provider,
		ProviderLabel:  providerLabel,
		SourceType:     sourceType,
		MockOnly:       true,
		Supported:      false,
		NeedsIngestion: true,
		Message:        message,
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
	costMap := h.loadVideoJobCostMap(jobs)
	pointHoldMap := h.loadVideoJobPointHoldMap(jobs)

	items := make([]VideoJobResponse, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, buildVideoJobResponseWithBilling(job, h.qiniu, lookupVideoJobCost(costMap, job.ID), lookupVideoJobPointHold(pointHoldMap, job.ID)))
	}
	if includeResultSummary && len(items) > 0 {
		summaryByCollectionKey, err := h.buildVideoJobResultSummary(jobs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for idx, job := range jobs {
			if idx >= len(items) || job.ResultCollectionID == nil || *job.ResultCollectionID == 0 {
				continue
			}
			collectionID := *job.ResultCollectionID
			collectionKey := videoJobCollectionMapKey(job.AssetDomain, collectionID)
			summary, ok := summaryByCollectionKey[collectionKey]
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

	costMap := h.loadVideoJobCostMap([]models.VideoJob{job})
	pointHoldMap := h.loadVideoJobPointHoldMap([]models.VideoJob{job})
	c.JSON(http.StatusOK, buildVideoJobResponseWithBilling(job, h.qiniu, lookupVideoJobCost(costMap, job.ID), lookupVideoJobPointHold(pointHoldMap, job.ID)))
}

// ListVideoJobEvents godoc
// @Summary List current user video job events
// @Tags user
// @Produce json
// @Param id path int true "job id"
// @Param since_id query int false "event id cursor (exclusive)"
// @Param limit query int false "limit (default 80, max 300)"
// @Success 200 {object} VideoJobEventListResponse
// @Router /api/video-jobs/{id}/events [get]
func (h *Handler) ListVideoJobEvents(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var job models.VideoJob
	if err := h.db.Select("id", "output_formats").Where("id = ? AND user_id = ?", jobID, userID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sinceID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("since_id")), 10, 64)
	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "80")))
	if limit <= 0 {
		limit = 80
	}
	if limit > 300 {
		limit = 300
	}

	requestedFormat := normalizeVideoImageFormatFilter(strings.Split(strings.ToLower(strings.TrimSpace(job.OutputFormats)), ",")[0])
	routedTables := resolveVideoImageReadTables(requestedFormat)
	baseEventsTable := models.VideoImageEventPublic{}.TableName()
	eventsTable := strings.TrimSpace(routedTables.Events)
	if eventsTable == "" {
		eventsTable = baseEventsTable
	}

	var rows []models.VideoImageEventPublic
	query := h.db.Table(eventsTable).Where("job_id = ?", jobID)
	if sinceID > 0 {
		query = query.Where("id > ?", sinceID)
	}
	err = query.Order("id ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		if eventsTable != baseEventsTable && isMissingTableError(err, eventsTable) {
			rows = nil
			fallbackQuery := h.db.Table(baseEventsTable).Where("job_id = ?", jobID)
			if sinceID > 0 {
				fallbackQuery = fallbackQuery.Where("id > ?", sinceID)
			}
			if fallbackErr := fallbackQuery.Order("id ASC").Limit(limit).Find(&rows).Error; fallbackErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fallbackErr.Error()})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	items := make([]VideoJobEventItemResponse, 0, len(rows))
	nextSinceID := sinceID
	for _, row := range rows {
		if row.ID > nextSinceID {
			nextSinceID = row.ID
		}
		items = append(items, VideoJobEventItemResponse{
			ID:        row.ID,
			Stage:     strings.TrimSpace(row.Stage),
			Level:     strings.TrimSpace(row.Level),
			Message:   strings.TrimSpace(row.Message),
			Metadata:  parseJSONMap(row.Metadata),
			CreatedAt: row.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, VideoJobEventListResponse{
		Items:       items,
		NextSinceID: nextSinceID,
	})
}

// GetVideoJobAI1Plan godoc
// @Summary Get current user video job ai1 executable plan
// @Tags user
// @Produce json
// @Param id path int true "job id"
// @Success 200 {object} VideoJobAI1PlanResponse
// @Router /api/video-jobs/{id}/ai1-plan [get]
func (h *Handler) GetVideoJobAI1Plan(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var job models.VideoJob
	if err := h.db.Select("id").Where("id = ? AND user_id = ?", jobID, userID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var row models.VideoJobAI1Plan
	if err := h.db.Where("job_id = ? AND user_id = ?", jobID, userID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableError(err, "video_job_ai1_plans") {
			c.JSON(http.StatusNotFound, gin.H{"error": "ai1 plan not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	plan := parseJSONMap(row.PlanJSON)

	c.JSON(http.StatusOK, VideoJobAI1PlanResponse{
		JobID:           row.JobID,
		RequestedFormat: strings.ToLower(strings.TrimSpace(row.RequestedFormat)),
		SchemaVersion:   strings.TrimSpace(row.SchemaVersion),
		PlanRevision:    extractAI1PlanRevision(plan),
		Status:          strings.TrimSpace(row.Status),
		SourcePrompt:    strings.TrimSpace(row.SourcePrompt),
		Plan:            plan,
		ModelProvider:   strings.TrimSpace(row.ModelProvider),
		ModelName:       strings.TrimSpace(row.ModelName),
		PromptVersion:   strings.TrimSpace(row.PromptVersion),
		FallbackUsed:    row.FallbackUsed,
		ConfirmedByUser: row.ConfirmedByUser,
		ConfirmedAt:     row.ConfirmedAt,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	})
}

// PatchVideoJobAI1Plan godoc
// @Summary Patch current user video job ai1 plan and regenerate ai2 consumable payload
// @Tags user
// @Accept json
// @Produce json
// @Param id path int true "job id"
// @Success 200 {object} VideoJobAI1PlanResponse
// @Router /api/video-jobs/{id}/ai1-plan [patch]
func (h *Handler) PatchVideoJobAI1Plan(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req PatchVideoJobAI1PlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	var job models.VideoJob
	if err := h.db.Where("id = ? AND user_id = ?", jobID, userID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	switch job.Status {
	case models.VideoJobStatusDone, models.VideoJobStatusFailed, models.VideoJobStatusCancelled:
		c.JSON(http.StatusBadRequest, gin.H{"error": "job cannot patch ai1 plan in current status"})
		return
	}

	options := parseJSONMap(job.Options)
	flowMode := strings.ToLower(strings.TrimSpace(stringFromAny(options["flow_mode"])))
	if flowMode != "ai1_confirm" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job is not in ai1_confirm flow mode"})
		return
	}
	ai1Pending := boolFromAny(options["ai1_pending"])
	if !ai1Pending && strings.ToLower(strings.TrimSpace(job.Stage)) != models.VideoJobStageAwaitingAI1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job is not waiting for ai1 confirmation"})
		return
	}

	var row models.VideoJobAI1Plan
	if err := h.db.Where("job_id = ? AND user_id = ?", jobID, userID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableError(err, "video_job_ai1_plans") {
			c.JSON(http.StatusNotFound, gin.H{"error": "ai1 plan not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	plan := parseJSONMap(row.PlanJSON)
	currentRevision := extractAI1PlanRevision(plan)
	if req.PlanRevision != nil && *req.PlanRevision != currentRevision {
		c.JSON(http.StatusConflict, gin.H{
			"error":                 "ai1_plan_revision_conflict",
			"current_plan_revision": currentRevision,
		})
		return
	}

	executablePlan := mapFromAnyValue(plan["executable_plan"])
	if len(executablePlan) == 0 {
		executablePlan = mapFromAnyValue(options["ai1_executable_plan_v1"])
	}
	if len(executablePlan) == 0 {
		executablePlan = map[string]interface{}{
			"target_format": normalizeRequestedFormatForDebug(stringFromAny(options["requested_format"])),
		}
	}
	eventMeta := mapFromAnyValue(plan["event_meta"])
	if len(eventMeta) == 0 {
		eventMeta = map[string]interface{}{}
	}
	ai1OutputV2 := mapFromAnyValue(plan["ai1_output_v2"])
	userFeedback := mapFromAnyValue(ai1OutputV2["user_feedback"])
	ai2Directive := mapFromAnyValue(ai1OutputV2["ai2_directive"])
	if len(userFeedback) == 0 {
		userFeedback = map[string]interface{}{
			"schema_version": videojobs.AI1UserFeedbackSchemaV2,
		}
	}
	if len(ai2Directive) == 0 {
		ai2Directive = map[string]interface{}{
			"schema_version": videojobs.AI2DirectiveSchemaV2,
			"source":         "ai1_executable_plan",
		}
	}

	editedFields := make([]string, 0, 16)
	editedFieldSet := map[string]struct{}{}
	markEdited := func(field string) {
		field = strings.TrimSpace(field)
		if field == "" {
			return
		}
		if _, exists := editedFieldSet[field]; exists {
			return
		}
		editedFieldSet[field] = struct{}{}
		editedFields = append(editedFields, field)
	}

	if req.Summary != nil {
		userFeedback["summary"] = strings.TrimSpace(*req.Summary)
		markEdited("summary")
	}
	if req.IntentUnderstanding != nil {
		userFeedback["intent_understanding"] = strings.TrimSpace(*req.IntentUnderstanding)
		markEdited("intent_understanding")
	}
	if req.StrategySummary != nil {
		userFeedback["strategy_summary"] = strings.TrimSpace(*req.StrategySummary)
		markEdited("strategy_summary")
	}
	if req.InteractiveAction != nil {
		action := strings.ToLower(strings.TrimSpace(*req.InteractiveAction))
		if action != "proceed" && action != "need_clarify" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "interactive_action must be proceed or need_clarify"})
			return
		}
		userFeedback["interactive_action"] = action
		markEdited("interactive_action")
	}
	if req.ClarifyQuestions != nil {
		questions := normalizeAI1EditableStringSlice(*req.ClarifyQuestions, 6)
		userFeedback["clarify_questions"] = questions
		markEdited("clarify_questions")
	}

	if req.Objective != nil {
		objective := strings.TrimSpace(*req.Objective)
		if objective == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "objective cannot be empty"})
			return
		}
		ai2Directive["objective"] = objective
		eventMeta["business_goal"] = objective
		markEdited("objective")
	}
	if req.MustCapture != nil {
		items := normalizeAI1EditableStringSlice(*req.MustCapture, 16)
		executablePlan["must_capture"] = items
		ai2Directive["must_capture"] = items
		eventMeta["must_capture"] = items
		markEdited("must_capture")
	}
	if req.Avoid != nil {
		items := normalizeAI1EditableStringSlice(*req.Avoid, 16)
		executablePlan["avoid"] = items
		ai2Directive["avoid"] = items
		eventMeta["avoid"] = items
		markEdited("avoid")
	}
	if req.StyleDirection != nil {
		style := strings.TrimSpace(*req.StyleDirection)
		if style == "" {
			delete(executablePlan, "style_direction")
			delete(ai2Directive, "style_direction")
			delete(eventMeta, "style_direction")
		} else {
			executablePlan["style_direction"] = style
			ai2Directive["style_direction"] = style
			eventMeta["style_direction"] = style
		}
		markEdited("style_direction")
	}
	targetFormatForPatch := normalizeRequestedFormatForDebug(stringFromAny(ai2Directive["target_format"]))
	if targetFormatForPatch == "" {
		targetFormatForPatch = normalizeRequestedFormatForDebug(stringFromAny(executablePlan["target_format"]))
	}
	if targetFormatForPatch == "" {
		targetFormatForPatch = normalizeRequestedFormatForDebug(stringFromAny(options["requested_format"]))
	}
	if targetFormatForPatch == "" {
		targetFormatForPatch = "png"
	}
	if req.QualityWeights != nil {
		normalizedWeights, err := normalizeAI1PatchQualityWeights(*req.QualityWeights)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ai2Directive["quality_weights"] = normalizedWeights
		eventMeta["quality_weights"] = normalizedWeights
		markEdited("quality_weights")
	}
	if req.RiskFlags != nil {
		riskFlags := normalizeAI1PatchRiskFlags(*req.RiskFlags, 16)
		ai2Directive["risk_flags"] = riskFlags
		eventMeta["risk_flags"] = riskFlags
		markEdited("risk_flags")
	}
	if req.MaxBlurTolerance != nil || req.AvoidWatermarks != nil || req.AvoidExtremeDark != nil {
		technicalReject := mapFromAnyValue(ai2Directive["technical_reject"])
		if len(technicalReject) == 0 {
			technicalReject = map[string]interface{}{
				"max_blur_tolerance": defaultAI1PatchMaxBlurTolerance(targetFormatForPatch),
				"avoid_watermarks":   true,
				"avoid_extreme_dark": true,
			}
		}
		if req.MaxBlurTolerance != nil {
			tolerance, err := normalizeAI1PatchMaxBlurTolerance(*req.MaxBlurTolerance, targetFormatForPatch)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			technicalReject["max_blur_tolerance"] = tolerance
			markEdited("max_blur_tolerance")
		}
		if req.AvoidWatermarks != nil {
			technicalReject["avoid_watermarks"] = *req.AvoidWatermarks
			markEdited("avoid_watermarks")
		}
		if req.AvoidExtremeDark != nil {
			technicalReject["avoid_extreme_dark"] = *req.AvoidExtremeDark
			markEdited("avoid_extreme_dark")
		}
		ai2Directive["technical_reject"] = technicalReject
		eventMeta["technical_reject"] = technicalReject
	}

	if req.TargetCount != nil {
		targetCount := *req.TargetCount
		if targetCount < 1 || targetCount > 80 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target_count must be between 1 and 80"})
			return
		}
		executablePlan["target_count"] = targetCount
		markEdited("target_count")
	}
	if req.FrameIntervalSec != nil {
		interval := *req.FrameIntervalSec
		if interval < 0.2 || interval > 8 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "frame_interval_sec must be between 0.2 and 8"})
			return
		}
		executablePlan["frame_interval_sec"] = interval
		markEdited("frame_interval_sec")
	}
	if req.FocusStartSec != nil || req.FocusEndSec != nil {
		if req.FocusStartSec == nil || req.FocusEndSec == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "focus_start_sec and focus_end_sec must be provided together"})
			return
		}
		startSec := *req.FocusStartSec
		endSec := *req.FocusEndSec
		if startSec < 0 || endSec <= startSec {
			c.JSON(http.StatusBadRequest, gin.H{"error": "focus window is invalid"})
			return
		}
		executablePlan["mode"] = "focus_window"
		executablePlan["focus_window"] = map[string]interface{}{
			"start_sec": roundTo1(startSec),
			"end_sec":   roundTo1(endSec),
			"source":    "user_override",
		}
		eventMeta["focus_window_source"] = "user_override"
		markEdited("focus_window")
	}

	if len(editedFields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no editable fields provided"})
		return
	}

	if _, exists := userFeedback["schema_version"]; !exists {
		userFeedback["schema_version"] = videojobs.AI1UserFeedbackSchemaV2
	}
	if _, exists := ai2Directive["schema_version"]; !exists {
		ai2Directive["schema_version"] = videojobs.AI2DirectiveSchemaV2
	}
	if _, exists := ai2Directive["target_format"]; !exists {
		ai2Directive["target_format"] = strings.ToLower(strings.TrimSpace(stringFromAny(executablePlan["target_format"])))
	}
	ai2Directive["sampling_plan"] = executablePlan
	ai2Directive["source"] = "ai1_executable_plan"

	ai1OutputV2["schema_version"] = videojobs.AI1OutputSchemaV2
	ai1OutputV2["user_feedback"] = userFeedback
	ai1OutputV2["ai2_directive"] = ai2Directive
	trace := mapFromAnyValue(ai1OutputV2["trace"])
	if len(trace) == 0 {
		trace = map[string]interface{}{}
	}

	now := time.Now()
	nowText := now.Format(time.RFC3339)
	trace["user_edited"] = true
	trace["user_edited_at"] = nowText
	trace["user_edited_fields"] = editedFields
	ai1OutputV2["trace"] = trace

	ai1OutputV1 := mapFromAnyValue(plan["ai1_output_v1"])
	if len(ai1OutputV1) == 0 {
		ai1OutputV1 = map[string]interface{}{
			"schema_version": strings.TrimSpace(stringFromAny(plan["schema_version"])),
		}
	}
	ai1OutputV1["user_reply"] = userFeedback
	ai1OutputV1["ai2_instruction"] = ai2Directive

	nextRevision := currentRevision + 1
	plan["plan_revision"] = nextRevision
	plan["last_user_edit_at"] = nowText
	plan["last_user_edit_fields"] = editedFields
	plan["event_meta"] = eventMeta
	plan["executable_plan"] = executablePlan
	plan["ai1_output_v2"] = ai1OutputV2
	plan["ai1_output_v1"] = ai1OutputV1

	options["ai1_executable_plan_v1"] = executablePlan
	options["ai2_instruction_v1"] = ai2Directive
	options["ai2_instruction_generated_at"] = nowText
	if qualityWeights := normalizeAI1PatchQualityWeightsFromAny(ai2Directive["quality_weights"]); len(qualityWeights) > 0 {
		options["ai2_quality_weights_v1"] = qualityWeights
	}
	if riskFlags := normalizeAI1PatchRiskFlags(parseStringSliceFromAny(ai2Directive["risk_flags"]), 16); len(riskFlags) > 0 {
		options["ai2_risk_flags_v1"] = riskFlags
	}
	if technicalReject := mapFromAnyValue(ai2Directive["technical_reject"]); len(technicalReject) > 0 {
		options["ai2_technical_reject_v1"] = map[string]interface{}{
			"max_blur_tolerance": strings.TrimSpace(stringFromAny(technicalReject["max_blur_tolerance"])),
			"avoid_watermarks":   boolFromAny(technicalReject["avoid_watermarks"]),
			"avoid_extreme_dark": boolFromAny(technicalReject["avoid_extreme_dark"]),
		}
	}
	options["ai1_event_meta_v1"] = eventMeta
	options["ai1_plan_user_edited"] = true
	options["ai1_plan_user_edited_at"] = nowText
	options["ai1_plan_user_edited_fields"] = editedFields

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		jobUpdates := map[string]interface{}{
			"options": toJSON(options),
		}
		if err := tx.Model(&models.VideoJob{}).Where("id = ? AND user_id = ?", jobID, userID).Updates(jobUpdates).Error; err != nil {
			return err
		}
		if err := videojobs.SyncPublicVideoImageJobUpdates(tx, jobID, jobUpdates); err != nil {
			return err
		}

		if err := tx.Model(&models.VideoJobAI1Plan{}).Where("job_id = ? AND user_id = ?", jobID, userID).Updates(map[string]interface{}{
			"status":            videojobs.VideoJobAI1PlanStatusAwaitingUser,
			"plan_json":         toJSON(plan),
			"confirmed_by_user": false,
			"confirmed_at":      nil,
			"updated_at":        now,
		}).Error; err != nil {
			return err
		}

		editEvent := models.VideoJobEvent{
			JobID:   jobID,
			Stage:   models.VideoJobStageAnalyzing,
			Level:   "info",
			Message: "user edited ai1 plan",
			Metadata: toJSON(map[string]interface{}{
				"flow_mode":      "ai1_confirm",
				"plan_revision":  nextRevision,
				"edited_fields":  editedFields,
				"user_edited_at": nowText,
			}),
		}
		if err := tx.Create(&editEvent).Error; err != nil {
			return err
		}
		return videojobs.CreatePublicVideoImageEvent(tx, editEvent)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, VideoJobAI1PlanResponse{
		JobID:           row.JobID,
		RequestedFormat: strings.ToLower(strings.TrimSpace(row.RequestedFormat)),
		SchemaVersion:   strings.TrimSpace(row.SchemaVersion),
		PlanRevision:    nextRevision,
		Status:          videojobs.VideoJobAI1PlanStatusAwaitingUser,
		SourcePrompt:    strings.TrimSpace(row.SourcePrompt),
		Plan:            plan,
		ModelProvider:   strings.TrimSpace(row.ModelProvider),
		ModelName:       strings.TrimSpace(row.ModelName),
		PromptVersion:   strings.TrimSpace(row.PromptVersion),
		FallbackUsed:    row.FallbackUsed,
		ConfirmedByUser: false,
		ConfirmedAt:     nil,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       now,
	})
}

// GetVideoJobAI1Debug godoc
// @Summary Get current user video job ai1 debug payloads
// @Tags user
// @Produce json
// @Param id path int true "job id"
// @Success 200 {object} VideoJobAI1DebugResponse
// @Router /api/video-jobs/{id}/ai1-debug [get]
func (h *Handler) GetVideoJobAI1Debug(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var job models.VideoJob
	if err := h.db.Where("id = ? AND user_id = ?", jobID, userID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var planRow models.VideoJobAI1Plan
	planFound := true
	if err := h.db.Where("job_id = ? AND user_id = ?", job.ID, userID).First(&planRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableError(err, "video_job_ai1_plans") {
			planFound = false
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	var usageRow models.VideoJobAIUsage
	usageFound := true
	if err := h.db.Where("job_id = ? AND user_id = ? AND stage = ?", job.ID, userID, "director").Order("id DESC").First(&usageRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableError(err, "video_job_ai_usage") {
			usageFound = false
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	var directiveRow models.VideoJobGIFAIDirective
	directiveFound := true
	if err := h.db.Where("job_id = ? AND user_id = ?", job.ID, userID).Order("id DESC").First(&directiveRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableError(err, "video_job_gif_ai_directives") {
			directiveFound = false
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
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
	planEventMeta := map[string]interface{}{}
	if planFound {
		planJSON = parseJSONMap(planRow.PlanJSON)
		planSchemaVersion = strings.TrimSpace(planRow.SchemaVersion)
		planStatus = strings.TrimSpace(planRow.Status)
		planEventMeta = mapFromAnyValue(planJSON["event_meta"])
	}
	ai1OutputV2 := mapFromAnyValue(planJSON["ai1_output_v2"])
	ai1OutputContract := buildAI1OutputContractReport(ai1OutputV2)

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

	userReply, ai2Instruction := buildAI1DebugOutput(planJSON)
	ai2Instruction = enrichAI2InstructionForDebug(ai2Instruction, requestedFormat, options, metrics, planEventMeta)

	modelRequest := map[string]interface{}{
		"provider":       strings.TrimSpace(planRow.ModelProvider),
		"model":          strings.TrimSpace(planRow.ModelName),
		"prompt_version": strings.TrimSpace(planRow.PromptVersion),
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
	advancedOptions := mapFromAnyValue(options["ai1_advanced_options_v1"])
	if len(advancedOptions) == 0 {
		advancedOptions = mapFromAnyValue(planEventMeta["advanced_options_v1"])
	}
	appliedStrategyProfile := mapFromAnyValue(options["ai1_strategy_profile_v1"])
	if len(appliedStrategyProfile) == 0 {
		appliedStrategyProfile = mapFromAnyValue(planEventMeta["strategy_profile_v1"])
	}
	appliedStrategyTrace := mapFromAnyValue(options["ai1_strategy_profile_trace_v1"])
	if len(appliedStrategyTrace) == 0 {
		appliedStrategyTrace = mapFromAnyValue(planEventMeta["strategy_profile_trace_v1"])
	}
	appliedSceneGuard := mapFromAnyValue(options["ai1_advanced_scene_guard_v1"])
	if len(appliedSceneGuard) == 0 {
		appliedSceneGuard = mapFromAnyValue(planEventMeta["advanced_scene_guard_v1"])
	}
	appliedStrategyOverrideReport := mapFromAnyValue(options["ai1_strategy_override_report_v1"])
	if len(appliedStrategyOverrideReport) == 0 {
		appliedStrategyOverrideReport = mapFromAnyValue(planEventMeta["strategy_override_report_v1"])
	}
	if len(advancedOptions) > 0 {
		input["advanced_options"] = advancedOptions
	}
	if len(appliedStrategyProfile) > 0 {
		input["applied_strategy_profile"] = appliedStrategyProfile
	}
	if len(appliedStrategyTrace) > 0 {
		input["applied_strategy_trace"] = appliedStrategyTrace
	}
	if len(appliedSceneGuard) > 0 {
		input["advanced_scene_guard_v1"] = appliedSceneGuard
	}
	if len(appliedStrategyOverrideReport) > 0 {
		input["strategy_override_report_v1"] = appliedStrategyOverrideReport
	}

	trace := map[string]interface{}{
		"options": map[string]interface{}{
			"execution_queue":             stringFromAny(options["execution_queue"]),
			"execution_task_type":         stringFromAny(options["execution_task_type"]),
			"ai1_plan_schema":             stringFromAny(options["ai1_plan_schema_version"]),
			"ai1_plan_mode":               stringFromAny(options["ai1_plan_mode"]),
			"ai1_plan_applied":            boolFromAny(options["ai1_plan_applied"]),
			"ai1_plan_generated":          boolFromAny(options["ai1_plan_generated"]),
			"ai1_pending":                 boolFromAny(options["ai1_pending"]),
			"ai1_confirmed":               boolFromAny(options["ai1_confirmed"]),
			"ai1_pause_consumed":          boolFromAny(options["ai1_pause_consumed"]),
			"requested_format":            stringFromAny(options["requested_format"]),
			"quality_overrides":           mapFromAnyValue(options["quality_profile_overrides"]),
			"source_video_probe_v1":       mapFromAnyValue(options["source_video_probe"]),
			"advanced_options_v1":         mapFromAnyValue(options["ai1_advanced_options_v1"]),
			"strategy_profile_v1":         mapFromAnyValue(options["ai1_strategy_profile_v1"]),
			"strategy_profile_trace_v1":   mapFromAnyValue(options["ai1_strategy_profile_trace_v1"]),
			"advanced_scene_guard_v1":     mapFromAnyValue(options["ai1_advanced_scene_guard_v1"]),
			"strategy_override_report_v1": mapFromAnyValue(options["ai1_strategy_override_report_v1"]),
		},
		"plan": map[string]interface{}{
			"schema_version": planSchemaVersion,
			"status":         planStatus,
			"plan_revision":  extractAI1PlanRevision(planJSON),
			"plan_json":      planJSON,
		},
		"rows": map[string]interface{}{
			"video_job_id":         job.ID,
			"video_job_stage":      strings.TrimSpace(job.Stage),
			"video_job_status":     strings.TrimSpace(job.Status),
			"ai1_plan_updated_at":  planRow.UpdatedAt,
			"ai_usage_created_at":  usageRow.CreatedAt,
			"directive_updated_at": directiveRow.UpdatedAt,
		},
		"ai1_output_contract_v2": ai1OutputContract,
	}

	output := map[string]interface{}{
		"user_reply":      userReply,
		"ai2_instruction": ai2Instruction,
		"contract_report": ai1OutputContract,
	}
	if len(advancedOptions) > 0 {
		output["advanced_options"] = advancedOptions
	}
	if len(appliedStrategyProfile) > 0 {
		output["applied_strategy_profile"] = appliedStrategyProfile
	}
	if len(appliedStrategyTrace) > 0 {
		output["applied_strategy_trace"] = appliedStrategyTrace
	}
	if len(appliedSceneGuard) > 0 {
		output["advanced_scene_guard_v1"] = appliedSceneGuard
	}
	if len(appliedStrategyOverrideReport) > 0 {
		output["strategy_override_report_v1"] = appliedStrategyOverrideReport
	}
	ai2ExecutionObservability := buildAI2ExecutionObservability(requestedFormat, options, metrics, ai2Instruction)
	if len(ai2ExecutionObservability) > 0 {
		output["ai2_execution_observability_v1"] = ai2ExecutionObservability
	}
	pipelineAlignmentReport := buildPipelineAlignmentReportV1(
		requestedFormat,
		sourcePrompt,
		advancedOptions,
		appliedStrategyProfile,
		appliedStrategyOverrideReport,
		appliedSceneGuard,
		userReply,
		ai2Instruction,
		ai2ExecutionObservability,
		options,
		metrics,
	)
	if len(pipelineAlignmentReport) > 0 {
		output["pipeline_alignment_report_v1"] = pipelineAlignmentReport
	}
	if len(ai1OutputV2) > 0 {
		output["ai1_output_v2"] = ai1OutputV2
	}
	if len(ai2ExecutionObservability) > 0 {
		trace["ai2_execution_observability_v1"] = ai2ExecutionObservability
	}
	if len(appliedSceneGuard) > 0 {
		trace["advanced_scene_guard_v1"] = appliedSceneGuard
	}
	if len(pipelineAlignmentReport) > 0 {
		trace["pipeline_alignment_report_v1"] = pipelineAlignmentReport
	}
	if len(appliedStrategyOverrideReport) > 0 {
		trace["strategy_override_report_v1"] = appliedStrategyOverrideReport
	}
	trace["timeline_v1"] = buildAI1DebugTimelineV1(
		sourcePrompt,
		requestedFormat,
		input,
		modelRequest,
		modelResponse,
		output,
		ai1OutputContract,
		ai2ExecutionObservability,
	)

	c.JSON(http.StatusOK, VideoJobAI1DebugResponse{
		JobID:           job.ID,
		RequestedFormat: strings.ToLower(strings.TrimSpace(requestedFormat)),
		FlowMode:        flowMode,
		Stage:           strings.TrimSpace(job.Stage),
		Status:          strings.TrimSpace(job.Status),
		SourcePrompt:    sourcePrompt,
		Input:           input,
		ModelRequest:    modelRequest,
		ModelResponse:   modelResponse,
		Output:          output,
		Focus: map[string]interface{}{
			"model_request":  modelRequest,
			"model_response": modelResponse,
			"contract_report": map[string]interface{}{
				"ai1_output_contract_v2": ai1OutputContract,
			},
			"ai2_execution_observability_v1": ai2ExecutionObservability,
			"pipeline_alignment_report_v1":   pipelineAlignmentReport,
		},
		Trace: trace,
	})
}

// ConfirmVideoJobAI1 godoc
// @Summary Confirm AI1 result and continue current user video job
// @Tags user
// @Produce json
// @Router /api/video-jobs/{id}/confirm-ai1 [post]
func (h *Handler) ConfirmVideoJobAI1(c *gin.Context) {
	if h.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "task queue not configured"})
		return
	}

	var req ConfirmVideoJobAI1Request
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "job cannot be confirmed in current status"})
		return
	}

	options := parseJSONMap(job.Options)
	flowMode := strings.ToLower(strings.TrimSpace(stringFromAny(options["flow_mode"])))
	if flowMode != "ai1_confirm" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job is not in ai1_confirm flow mode"})
		return
	}
	ai1Pending := boolFromAny(options["ai1_pending"])
	if !ai1Pending && strings.ToLower(strings.TrimSpace(job.Stage)) != models.VideoJobStageAwaitingAI1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job is not waiting for ai1 confirmation"})
		return
	}

	if req.PlanRevision != nil {
		var row models.VideoJobAI1Plan
		if err := h.db.Where("job_id = ? AND user_id = ?", job.ID, userID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableError(err, "video_job_ai1_plans") {
				// 兼容：AI1 计划表不存在/无记录时，跳过 revision 乐观锁校验，允许继续执行。
				req.PlanRevision = nil
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else {
			currentRevision := extractAI1PlanRevision(parseJSONMap(row.PlanJSON))
			if *req.PlanRevision != currentRevision {
				c.JSON(http.StatusConflict, gin.H{
					"error":                 "ai1_plan_revision_conflict",
					"current_plan_revision": currentRevision,
				})
				return
			}
		}
	}

	confirmedAt := time.Now()
	options["ai1_pending"] = false
	options["ai1_confirmed"] = true
	options["ai1_confirmed_at"] = confirmedAt.Format(time.RFC3339)
	resumeQueue, resumeTaskType, _ := videojobs.ResolveVideoJobExecutionTarget(job.OutputFormats)
	options["execution_queue"] = resumeQueue
	options["execution_task_type"] = resumeTaskType
	progress := job.Progress
	if progress < 36 {
		progress = 36
	}
	updates := map[string]interface{}{
		"status":        models.VideoJobStatusQueued,
		"stage":         models.VideoJobStageQueued,
		"progress":      progress,
		"options":       toJSON(options),
		"error_message": "",
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.VideoJob{}).Where("id = ? AND user_id = ?", job.ID, userID).Updates(updates).Error; err != nil {
			return err
		}
		if err := videojobs.SyncPublicVideoImageJobUpdates(tx, job.ID, updates); err != nil {
			return err
		}
		eventMeta := map[string]interface{}{
			"flow_mode": "ai1_confirm",
			"action":    "user_confirmed",
		}
		confirmEvent := models.VideoJobEvent{
			JobID:    job.ID,
			Stage:    models.VideoJobStageAnalyzing,
			Level:    "info",
			Message:  "user confirmed continue after ai1",
			Metadata: toJSON(eventMeta),
		}
		if err := tx.Create(&confirmEvent).Error; err != nil {
			return err
		}
		if err := videojobs.CreatePublicVideoImageEvent(tx, confirmEvent); err != nil {
			return err
		}
		if err := videojobs.ConfirmVideoJobAI1Plan(tx, job.ID, userID, confirmedAt); err != nil {
			return err
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	task, queueName, _, err := videojobs.NewProcessVideoJobTaskByFormat(job.ID, job.OutputFormats)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}
	if _, err := h.queue.Enqueue(
		task,
		asynq.Queue(queueName),
		asynq.MaxRetry(6),
		asynq.Timeout(2*time.Hour),
		asynq.Retention(7*24*time.Hour),
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue resume task"})
		return
	}

	var latest models.VideoJob
	if err := h.db.Where("id = ? AND user_id = ?", job.ID, userID).First(&latest).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, buildVideoJobResponse(latest, h.qiniu))
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

	collection, err := h.loadVideoJobResultCollection(job)
	if err != nil {
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

	emojis, err := h.listVideoJobResultEmojisByDomain(collection.ID, job.AssetDomain)
	if err != nil {
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
	jobHasGIFFormat := false
	for _, raw := range strings.Split(strings.ToLower(strings.TrimSpace(job.OutputFormats)), ",") {
		if strings.TrimSpace(raw) == "gif" {
			jobHasGIFFormat = true
			break
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
			if jobHasGIFFormat {
				reviewRecommendation = "need_manual_review"
			} else {
				reviewRecommendation = "deliver"
			}
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
	if latestZip, err := h.loadLatestVideoJobResultZipByDomain(collection.ID, job.AssetDomain); err == nil {
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
	costMap := h.loadVideoJobCostMap([]models.VideoJob{job})
	pointHoldMap := h.loadVideoJobPointHoldMap([]models.VideoJob{job})

	resp := VideoJobResultResponse{
		JobID:              job.ID,
		Status:             job.Status,
		DeliveryOnly:       deliveryOnly,
		ReviewStatusFilter: reviewStatusFilter,
		Billing:            buildVideoJobBillingInfo(lookupVideoJobCost(costMap, job.ID), lookupVideoJobPointHold(pointHoldMap, job.ID)),
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

func normalizeExternalVideoSourceURL(raw string) (normalizedURL string, provider string, providerLabel string, sourceType string, err error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", "", "", "", errors.New("source_url required")
	}
	parsed, parseErr := url.Parse(text)
	if parseErr != nil || parsed == nil {
		return "", "", "", "", errors.New("invalid source_url")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", "", "", "", errors.New("source_url must be http/https")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return "", "", "", "", errors.New("invalid source_url host")
	}

	parsed.Fragment = ""
	normalized := strings.TrimSpace(parsed.String())
	provider, providerLabel = detectExternalVideoProvider(host)

	sourceType = "platform_share_url"
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(parsed.Path)))
	if _, ok := allowedVideoFileExt[ext]; ok {
		sourceType = "direct_video_url"
	}
	return normalized, provider, providerLabel, sourceType, nil
}

func detectExternalVideoProvider(host string) (provider string, providerLabel string) {
	h := strings.ToLower(strings.TrimSpace(host))
	switch {
	case strings.Contains(h, "douyin.com"):
		return "douyin", "抖音"
	case strings.Contains(h, "kuaishou.com") || strings.Contains(h, "ksapisrv.com"):
		return "kuaishou", "快手"
	case strings.Contains(h, "xiaohongshu.com"):
		return "xiaohongshu", "小红书"
	case strings.Contains(h, "bilibili.com") || strings.Contains(h, "b23.tv"):
		return "bilibili", "哔哩哔哩"
	case strings.Contains(h, "weibo.com"):
		return "weibo", "微博"
	case strings.Contains(h, "youtube.com") || strings.Contains(h, "youtu.be"):
		return "youtube", "YouTube"
	default:
		return "generic", "通用链接"
	}
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
	return buildVideoJobResponseWithBilling(job, qiniu, nil, nil)
}

func buildVideoJobResponseWithBilling(job models.VideoJob, qiniu *storage.QiniuClient, cost *models.VideoJobCost, pointHold *models.ComputePointHold) VideoJobResponse {
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
		AssetDomain:        normalizeVideoJobAssetDomain(job.AssetDomain),
		ErrorMessage:       strings.TrimSpace(job.ErrorMessage),
		ResultCollectionID: job.ResultCollectionID,
		Options:            options,
		Metrics:            metrics,
		QueuedAt:           job.QueuedAt,
		StartedAt:          job.StartedAt,
		FinishedAt:         job.FinishedAt,
		CreatedAt:          job.CreatedAt,
		UpdatedAt:          job.UpdatedAt,
		Billing:            buildVideoJobBillingInfo(cost, pointHold),
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

func lookupVideoJobCost(costMap map[uint64]models.VideoJobCost, jobID uint64) *models.VideoJobCost {
	item, ok := costMap[jobID]
	if !ok {
		return nil
	}
	copied := item
	return &copied
}

func lookupVideoJobPointHold(pointHoldMap map[uint64]models.ComputePointHold, jobID uint64) *models.ComputePointHold {
	item, ok := pointHoldMap[jobID]
	if !ok {
		return nil
	}
	copied := item
	return &copied
}

func buildVideoJobBillingInfo(cost *models.VideoJobCost, pointHold *models.ComputePointHold) *VideoJobBillingInfo {
	if cost == nil && pointHold == nil {
		return nil
	}
	billing := &VideoJobBillingInfo{
		PointPerCNY:          videojobs.PointPerCNY(),
		CostMarkupMultiplier: videojobs.CostMarkupMultiplier(),
	}
	if cost != nil {
		details := parseJSONMap(cost.Details)
		billing.ActualCostCNY = parseFloat64FromAny(details["ai_cost_cny"])
		if billing.ActualCostCNY <= 0 {
			if strings.EqualFold(strings.TrimSpace(cost.Currency), "cny") && cost.EstimatedCost > 0 {
				billing.ActualCostCNY = cost.EstimatedCost
			}
		}
		billing.Currency = strings.TrimSpace(cost.Currency)
		billing.PricingVersion = strings.TrimSpace(cost.PricingVersion)
	}
	if pointHold != nil {
		billing.ChargedPoints = pointHold.SettledPoints
		billing.ReservedPoints = pointHold.ReservedPoints
		billing.HoldStatus = strings.TrimSpace(pointHold.Status)
	}
	return billing
}

type videoJobCollectionSummaryRow struct {
	ID            uint64 `gorm:"column:id"`
	Title         string `gorm:"column:title"`
	FileCount     int    `gorm:"column:file_count"`
	LatestZipKey  string `gorm:"column:latest_zip_key"`
	LatestZipSize int64  `gorm:"column:latest_zip_size"`
}

type videoJobEmojiSummaryRow struct {
	ID           uint64 `gorm:"column:id"`
	CollectionID uint64 `gorm:"column:collection_id"`
	Format       string `gorm:"column:format"`
	FileURL      string `gorm:"column:file_url"`
	ThumbURL     string `gorm:"column:thumb_url"`
	SizeBytes    int64  `gorm:"column:size_bytes"`
	DisplayOrder int    `gorm:"column:display_order"`
}

type videoJobOutputSummaryRow struct {
	ID                     uint64  `gorm:"column:id"`
	JobID                  uint64  `gorm:"column:job_id"`
	ObjectKey              string  `gorm:"column:object_key"`
	Format                 string  `gorm:"column:format"`
	Score                  float64 `gorm:"column:score"`
	GIFLoopTuneLoopClosure float64 `gorm:"column:gif_loop_tune_loop_closure"`
}

type videoJobReviewSummaryRow struct {
	ID                  uint64  `gorm:"column:id"`
	OutputID            *uint64 `gorm:"column:output_id"`
	FinalRecommendation string  `gorm:"column:final_recommendation"`
}

type videoJobResultSummaryAccumulator struct {
	Summary               VideoJobResultSummary
	ActiveCnt             int
	DeliverCnt            int
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

func videoJobHasOutputFormat(raw, target string) bool {
	needle := normalizeVideoJobResultFormat(target)
	if needle == "" {
		return false
	}
	for _, part := range strings.Split(raw, ",") {
		if normalizeVideoJobResultFormat(part) == needle {
			return true
		}
	}
	return false
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

func (h *Handler) buildVideoJobResultSummary(jobs []models.VideoJob) (map[string]VideoJobResultSummary, error) {
	videoCollectionIDs := make([]uint64, 0, len(jobs))
	archiveCollectionIDs := make([]uint64, 0, len(jobs))
	jobIDs := make([]uint64, 0, len(jobs))
	seenVideo := make(map[uint64]struct{}, len(jobs))
	seenArchive := make(map[uint64]struct{}, len(jobs))
	collectionToJobID := make(map[string]uint64, len(jobs))
	jobHasGIFFormat := make(map[uint64]bool, len(jobs))
	for _, job := range jobs {
		jobIDs = append(jobIDs, job.ID)
		jobHasGIFFormat[job.ID] = videoJobHasOutputFormat(job.OutputFormats, "gif")
		if job.ResultCollectionID == nil || *job.ResultCollectionID == 0 {
			continue
		}
		collectionID := *job.ResultCollectionID
		key := videoJobCollectionMapKey(job.AssetDomain, collectionID)
		if _, exists := collectionToJobID[key]; !exists {
			collectionToJobID[key] = job.ID
		}
		if normalizeVideoJobAssetDomain(job.AssetDomain) == models.VideoJobAssetDomainVideo {
			if _, ok := seenVideo[collectionID]; ok {
				continue
			}
			seenVideo[collectionID] = struct{}{}
			videoCollectionIDs = append(videoCollectionIDs, collectionID)
			continue
		}
		if _, ok := seenArchive[collectionID]; ok {
			continue
		}
		seenArchive[collectionID] = struct{}{}
		archiveCollectionIDs = append(archiveCollectionIDs, collectionID)
	}
	if len(videoCollectionIDs) == 0 && len(archiveCollectionIDs) == 0 {
		return map[string]VideoJobResultSummary{}, nil
	}

	acc := map[string]*videoJobResultSummaryAccumulator{}
	if len(archiveCollectionIDs) > 0 {
		var collections []videoJobCollectionSummaryRow
		if err := h.db.Model(&models.Collection{}).
			Select("id", "title", "file_count", "latest_zip_key", "latest_zip_size").
			Where("id IN ?", archiveCollectionIDs).
			Find(&collections).Error; err != nil {
			return nil, err
		}
		for _, row := range collections {
			key := videoJobCollectionMapKey(models.VideoJobAssetDomainArchive, row.ID)
			packageStatus := "processing"
			if strings.TrimSpace(row.LatestZipKey) != "" {
				packageStatus = "ready"
			}
			acc[key] = &videoJobResultSummaryAccumulator{
				Summary: VideoJobResultSummary{
					CollectionID:     row.ID,
					CollectionTitle:  strings.TrimSpace(row.Title),
					FileCount:        row.FileCount,
					PreviewImages:    make([]string, 0, 15),
					PackageStatus:    packageStatus,
					PackageSizeBytes: row.LatestZipSize,
				},
				FormatCnt:  map[string]int{},
				PreviewSet: map[string]struct{}{},
			}
		}
	}
	if len(videoCollectionIDs) > 0 {
		var collections []models.VideoAssetCollection
		if err := h.db.Select("id", "title", "file_count", "latest_zip_key", "latest_zip_size").
			Where("id IN ?", videoCollectionIDs).
			Find(&collections).Error; err != nil {
			return nil, err
		}
		for _, row := range collections {
			key := videoJobCollectionMapKey(models.VideoJobAssetDomainVideo, row.ID)
			packageStatus := "processing"
			if strings.TrimSpace(row.LatestZipKey) != "" {
				packageStatus = "ready"
			}
			acc[key] = &videoJobResultSummaryAccumulator{
				Summary: VideoJobResultSummary{
					CollectionID:     row.ID,
					CollectionTitle:  strings.TrimSpace(row.Title),
					FileCount:        row.FileCount,
					PreviewImages:    make([]string, 0, 15),
					PackageStatus:    packageStatus,
					PackageSizeBytes: row.LatestZipSize,
				},
				FormatCnt:  map[string]int{},
				PreviewSet: map[string]struct{}{},
			}
		}
	}
	if len(acc) == 0 {
		return map[string]VideoJobResultSummary{}, nil
	}

	emojiRowsByCollectionKey := map[string][]videoJobEmojiSummaryRow{}
	if len(archiveCollectionIDs) > 0 {
		var emojiRows []videoJobEmojiSummaryRow
		if err := h.db.Model(&models.Emoji{}).
			Select("id", "collection_id", "format", "file_url", "thumb_url", "size_bytes", "display_order").
			Where("collection_id IN ? AND status = ?", archiveCollectionIDs, "active").
			Order("collection_id ASC, display_order ASC, id ASC").
			Find(&emojiRows).Error; err != nil {
			return nil, err
		}
		for _, row := range emojiRows {
			key := videoJobCollectionMapKey(models.VideoJobAssetDomainArchive, row.CollectionID)
			if item, ok := acc[key]; ok {
				item.ActiveCnt++
				emojiRowsByCollectionKey[key] = append(emojiRowsByCollectionKey[key], row)
			}
		}
	}
	if len(videoCollectionIDs) > 0 {
		var emojiRows []videoJobEmojiSummaryRow
		if err := h.db.Model(&models.VideoAssetEmoji{}).
			Select("id", "collection_id", "format", "file_url", "thumb_url", "size_bytes", "display_order").
			Where("collection_id IN ? AND status = ?", videoCollectionIDs, "active").
			Order("collection_id ASC, display_order ASC, id ASC").
			Find(&emojiRows).Error; err != nil {
			return nil, err
		}
		for _, row := range emojiRows {
			key := videoJobCollectionMapKey(models.VideoJobAssetDomainVideo, row.CollectionID)
			if item, ok := acc[key]; ok {
				item.ActiveCnt++
				emojiRowsByCollectionKey[key] = append(emojiRowsByCollectionKey[key], row)
			}
		}
	}

	outputByJobAndObjectKey := make(map[uint64]map[string]videoJobOutputSummaryRow, len(jobIDs))
	if len(jobIDs) > 0 {
		var outputRows []videoJobOutputSummaryRow
		if err := h.db.Model(&models.VideoImageOutputPublic{}).
			Select("id", "job_id", "object_key", "format", "score", "gif_loop_tune_loop_closure").
			Where("job_id IN ? AND file_role = ?", jobIDs, "main").
			Find(&outputRows).Error; err != nil {
			return nil, err
		}
		for _, row := range outputRows {
			objectKey := strings.TrimSpace(row.ObjectKey)
			if objectKey == "" {
				continue
			}
			byObjectKey, exists := outputByJobAndObjectKey[row.JobID]
			if !exists {
				byObjectKey = map[string]videoJobOutputSummaryRow{}
				outputByJobAndObjectKey[row.JobID] = byObjectKey
			}
			if _, exists := byObjectKey[objectKey]; !exists {
				byObjectKey[objectKey] = row
			}
		}
	}

	reviewByOutputID := map[uint64]string{}
	if len(jobIDs) > 0 {
		var reviewRows []videoJobReviewSummaryRow
		if err := h.db.Model(&models.VideoJobGIFAIReview{}).
			Select("id", "output_id", "final_recommendation").
			Where("job_id IN ? AND output_id IS NOT NULL", jobIDs).
			Order("id DESC").
			Find(&reviewRows).Error; err != nil {
			msg := strings.ToLower(strings.TrimSpace(err.Error()))
			if !(strings.Contains(msg, "archive.video_job_gif_ai_reviews") &&
				(strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such table"))) {
				return nil, err
			}
			reviewRows = nil
		}
		for _, row := range reviewRows {
			if row.OutputID == nil || *row.OutputID == 0 {
				continue
			}
			if _, exists := reviewByOutputID[*row.OutputID]; exists {
				continue
			}
			status := normalizeVideoJobReviewStatus(row.FinalRecommendation)
			if status == "" {
				continue
			}
			reviewByOutputID[*row.OutputID] = status
		}
	}

	for collectionKey, item := range acc {
		jobID := collectionToJobID[collectionKey]
		jobOutputs := outputByJobAndObjectKey[jobID]
		jobIsGIF := jobHasGIFFormat[jobID]
		for _, row := range emojiRowsByCollectionKey[collectionKey] {
			outputID := uint64(0)
			outputFormat := ""
			outputScore := 0.0
			outputLoopClosure := 0.0
			if output, exists := jobOutputs[strings.TrimSpace(row.FileURL)]; exists {
				outputID = output.ID
				outputFormat = output.Format
				outputScore = output.Score
				outputLoopClosure = output.GIFLoopTuneLoopClosure
			}

			recommendation := ""
			if outputID > 0 {
				recommendation = normalizeVideoJobReviewStatus(reviewByOutputID[outputID])
			}
			if recommendation == "" {
				if jobIsGIF {
					recommendation = "need_manual_review"
				} else {
					recommendation = "deliver"
				}
			}
			if recommendation != "deliver" {
				continue
			}

			item.DeliverCnt++
			if row.SizeBytes > 0 {
				item.Summary.OutputTotalSizeBytes += row.SizeBytes
			}
			format := normalizeVideoJobResultFormat(row.Format)
			if format == "" {
				format = normalizeVideoJobResultFormat(outputFormat)
			}
			if format != "" {
				item.FormatCnt[format]++
			}

			if len(item.Summary.PreviewImages) < 15 {
				source := strings.TrimSpace(row.ThumbURL)
				if source == "" {
					source = strings.TrimSpace(row.FileURL)
				}
				previewURL := strings.TrimSpace(resolvePreviewURL(source, h.qiniu))
				if previewURL != "" {
					if _, exists := item.PreviewSet[previewURL]; !exists {
						item.PreviewSet[previewURL] = struct{}{}
						item.Summary.PreviewImages = append(item.Summary.PreviewImages, previewURL)
					}
				}
			}

			if outputID > 0 && normalizeVideoJobResultFormat(outputFormat) == "gif" {
				item.QualityCnt++
				item.QualityScoreSum += outputScore
				item.QualityLoopClosureSum += outputLoopClosure
				if item.QualityCnt == 1 || outputScore > item.Summary.QualityTopScore {
					item.Summary.QualityTopScore = outputScore
				}
			}
		}
	}

	out := make(map[string]VideoJobResultSummary, len(acc))
	for collectionKey, item := range acc {
		item.Summary.FileCount = item.DeliverCnt
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
		out[collectionKey] = item.Summary
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

func mapFromAnyValue(raw interface{}) map[string]interface{} {
	value, ok := raw.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func parseJSONStringArray(raw datatypes.JSON) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var arr []interface{}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		value := strings.TrimSpace(stringFromAny(item))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func resolveVideoJobDebugSourcePrompt(job models.VideoJob, options map[string]interface{}) string {
	if prompt := strings.TrimSpace(stringFromAny(options["user_prompt"])); prompt != "" {
		return prompt
	}
	return strings.TrimSpace(job.Title)
}

func buildAI1DebugOutput(plan map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
	if len(plan) == 0 {
		return map[string]interface{}{}, map[string]interface{}{}
	}
	ai1OutputV2 := mapFromAnyValue(plan["ai1_output_v2"])
	ai1Output := mapFromAnyValue(plan["ai1_output_v1"])
	userReply := map[string]interface{}{}
	ai2Instruction := map[string]interface{}{}
	if len(ai1OutputV2) > 0 {
		userReply = mapFromAnyValue(ai1OutputV2["user_feedback"])
		ai2Instruction = mapFromAnyValue(ai1OutputV2["ai2_directive"])
	}
	if len(ai1Output) > 0 {
		if len(userReply) == 0 {
			userReply = mapFromAnyValue(ai1Output["user_reply"])
		}
		if len(ai2Instruction) == 0 {
			ai2Instruction = mapFromAnyValue(ai1Output["ai2_instruction"])
		}
	}

	eventMeta := mapFromAnyValue(plan["event_meta"])
	executablePlan := mapFromAnyValue(plan["executable_plan"])
	directive := mapFromAnyValue(plan["directive"])

	if len(userReply) == 0 {
		userReply = map[string]interface{}{
			"summary": strings.TrimSpace(stringFromAny(plan["ai_reply"])),
			"understood_intent": map[string]interface{}{
				"business_goal": strings.TrimSpace(stringFromAny(directive["business_goal"])),
				"audience":      strings.TrimSpace(stringFromAny(directive["audience"])),
				"must_capture":  directive["must_capture"],
				"avoid":         directive["avoid"],
			},
			"source": "fallback_from_plan",
		}
		if text := strings.TrimSpace(stringFromAny(eventMeta["ai_reply"])); text != "" {
			userReply["summary"] = text
		}
	}
	if len(ai2Instruction) == 0 {
		ai2Instruction = map[string]interface{}{
			"instruction_version": strings.TrimSpace(stringFromAny(plan["schema_version"])),
			"source":              "fallback_from_executable_plan",
			"executable_plan":     executablePlan,
		}
	}
	return userReply, ai2Instruction
}

func enrichAI2InstructionForDebug(
	current map[string]interface{},
	requestedFormat string,
	options map[string]interface{},
	metrics map[string]interface{},
	planEventMeta map[string]interface{},
) map[string]interface{} {
	out := copyMapFromAnyValue(current)
	if len(out) == 0 {
		out = copyMapFromAnyValue(options["ai2_instruction_v1"])
	}

	metricPrefix := resolveImageMetricPrefixForDebug(requestedFormat)
	if metricPrefix != "" {
		out = overlayAI2InstructionForDebug(out, mapFromAnyValue(metrics[fmt.Sprintf("%s_ai2_instruction_v1", metricPrefix)]))
	}
	out = overlayAI2InstructionForDebug(out, mapFromAnyValue(planEventMeta["ai2_instruction_v1"]))

	if len(out) == 0 {
		if executable := mapFromAnyValue(options["ai1_executable_plan_v1"]); len(executable) > 0 {
			out = map[string]interface{}{
				"instruction_version": strings.TrimSpace(stringFromAny(options["ai1_plan_schema_version"])),
				"source":              "fallback_from_options_executable_plan",
				"sampling_plan":       executable,
			}
		}
	}

	if len(out) == 0 {
		return map[string]interface{}{}
	}

	if len(mapFromAnyValue(out["quality_weights"])) == 0 {
		switch {
		case len(mapFromAnyValue(options["ai2_quality_weights_v1"])) > 0:
			out["quality_weights"] = mapFromAnyValue(options["ai2_quality_weights_v1"])
		case len(mapFromAnyValue(mapFromAnyValue(options["ai2_guidance_v1"])["quality_weights"])) > 0:
			out["quality_weights"] = mapFromAnyValue(mapFromAnyValue(options["ai2_guidance_v1"])["quality_weights"])
		case len(mapFromAnyValue(planEventMeta["quality_weights"])) > 0:
			out["quality_weights"] = mapFromAnyValue(planEventMeta["quality_weights"])
		}
	}

	if len(mapFromAnyValue(out["technical_reject"])) == 0 {
		switch {
		case len(mapFromAnyValue(options["ai2_technical_reject_v1"])) > 0:
			out["technical_reject"] = mapFromAnyValue(options["ai2_technical_reject_v1"])
		case len(mapFromAnyValue(mapFromAnyValue(options["ai2_guidance_v1"])["technical_reject"])) > 0:
			out["technical_reject"] = mapFromAnyValue(mapFromAnyValue(options["ai2_guidance_v1"])["technical_reject"])
		case len(mapFromAnyValue(mapFromAnyValue(options["ai2_worker_strategy_v1"])["technical_reject"])) > 0:
			out["technical_reject"] = mapFromAnyValue(mapFromAnyValue(options["ai2_worker_strategy_v1"])["technical_reject"])
		case len(mapFromAnyValue(planEventMeta["technical_reject"])) > 0:
			out["technical_reject"] = mapFromAnyValue(planEventMeta["technical_reject"])
		}
	}

	if strings.TrimSpace(stringFromAny(out["style_direction"])) == "" {
		style := firstNonEmptyString(
			stringFromAny(planEventMeta["style_direction"]),
			stringFromAny(mapFromAnyValue(options["ai1_strategy_profile_v1"])["style_direction"]),
			stringFromAny(mapFromAnyValue(planEventMeta["strategy_profile_v1"])["style_direction"]),
		)
		if style != "" {
			out["style_direction"] = style
		}
	}

	if len(parseStringSliceFromAny(out["must_capture"])) == 0 {
		switch {
		case len(parseStringSliceFromAny(planEventMeta["must_capture"])) > 0:
			out["must_capture"] = parseStringSliceFromAny(planEventMeta["must_capture"])
		case len(parseStringSliceFromAny(mapFromAnyValue(options["ai1_strategy_profile_v1"])["must_capture_bias"])) > 0:
			out["must_capture"] = parseStringSliceFromAny(mapFromAnyValue(options["ai1_strategy_profile_v1"])["must_capture_bias"])
		}
	}
	if len(parseStringSliceFromAny(out["avoid"])) == 0 {
		switch {
		case len(parseStringSliceFromAny(planEventMeta["avoid"])) > 0:
			out["avoid"] = parseStringSliceFromAny(planEventMeta["avoid"])
		case len(parseStringSliceFromAny(mapFromAnyValue(options["ai1_strategy_profile_v1"])["avoid_bias"])) > 0:
			out["avoid"] = parseStringSliceFromAny(mapFromAnyValue(options["ai1_strategy_profile_v1"])["avoid_bias"])
		}
	}

	if len(mapFromAnyValue(out["advanced_options"])) == 0 {
		switch {
		case len(mapFromAnyValue(options["ai1_advanced_options_v1"])) > 0:
			out["advanced_options"] = mapFromAnyValue(options["ai1_advanced_options_v1"])
		case len(mapFromAnyValue(planEventMeta["advanced_options_v1"])) > 0:
			out["advanced_options"] = mapFromAnyValue(planEventMeta["advanced_options_v1"])
		}
	}

	if len(mapFromAnyValue(out["strategy_profile"])) == 0 {
		switch {
		case len(mapFromAnyValue(options["ai1_strategy_profile_v1"])) > 0:
			out["strategy_profile"] = mapFromAnyValue(options["ai1_strategy_profile_v1"])
		case len(mapFromAnyValue(planEventMeta["strategy_profile_v1"])) > 0:
			out["strategy_profile"] = mapFromAnyValue(planEventMeta["strategy_profile_v1"])
		}
	}

	if strings.TrimSpace(stringFromAny(out["objective"])) == "" {
		objective := firstNonEmptyString(
			stringFromAny(planEventMeta["business_goal"]),
			stringFromAny(mapFromAnyValue(options["ai1_strategy_profile_v1"])["business_goal"]),
		)
		if objective != "" {
			out["objective"] = objective
		}
	}

	if strings.TrimSpace(stringFromAny(out["source"])) == "" {
		out["source"] = "debug_enriched_from_options_metrics"
	}
	return out
}

func copyMapFromAnyValue(raw interface{}) map[string]interface{} {
	in := mapFromAnyValue(raw)
	if len(in) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func overlayAI2InstructionForDebug(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return dst
	}
	if len(dst) == 0 {
		dst = map[string]interface{}{}
	}
	for key, value := range src {
		if value == nil {
			continue
		}
		existing, exists := dst[key]
		switch {
		case !exists || debugValueEmpty(existing):
			dst[key] = value
		default:
			existingMap := mapFromAnyValue(existing)
			sourceMap := mapFromAnyValue(value)
			if len(existingMap) > 0 && len(sourceMap) > 0 {
				merged := copyMapFromAnyValue(existingMap)
				for mk, mv := range sourceMap {
					if debugValueEmpty(merged[mk]) {
						merged[mk] = mv
					}
				}
				dst[key] = merged
			}
		}
	}
	return dst
}

func debugValueEmpty(raw interface{}) bool {
	if raw == nil {
		return true
	}
	if text := strings.TrimSpace(stringFromAny(raw)); text != "" {
		return false
	}
	if len(mapFromAnyValue(raw)) > 0 {
		return false
	}
	if len(parseStringSliceFromAny(raw)) > 0 {
		return false
	}
	return true
}

func normalizeRequestedFormatForDebug(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "jpeg" {
		return "jpg"
	}
	return value
}

func resolveImageMetricPrefixForDebug(format string) string {
	switch normalizeRequestedFormatForDebug(format) {
	case "png":
		return "png"
	case "jpg":
		return "jpg"
	case "webp":
		return "webp"
	case "live":
		return "live"
	case "mp4":
		return "mp4"
	default:
		return "image"
	}
}

func floatFromAnyValue(raw interface{}) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case int32:
		return float64(value)
	case uint:
		return float64(value)
	case uint64:
		return float64(value)
	case uint32:
		return float64(value)
	case json.Number:
		if f, err := value.Float64(); err == nil {
			return f
		}
		return 0
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
			return f
		}
		return 0
	default:
		return 0
	}
}

func parseAI2QualityWeightsForDebug(raw interface{}) map[string]float64 {
	required := []string{"semantic", "clarity", "loop", "efficiency"}
	out := map[string]float64{}

	weights := mapFromAnyValue(raw)
	if len(weights) == 0 {
		return out
	}
	for _, key := range required {
		value, exists := weights[key]
		if !exists {
			continue
		}
		out[key] = floatFromAnyValue(value)
	}
	return out
}

func normalizeAI2QualityWeightsForDebug(raw map[string]float64) map[string]float64 {
	required := []string{"semantic", "clarity", "loop", "efficiency"}
	normalized := map[string]float64{}
	sum := 0.0
	for _, key := range required {
		value := raw[key]
		if value < 0 {
			value = 0
		}
		if value > 1 {
			value = 1
		}
		normalized[key] = value
		sum += value
	}
	if sum <= 0 {
		return map[string]float64{
			"semantic":   0.35,
			"clarity":    0.20,
			"loop":       0.25,
			"efficiency": 0.20,
		}
	}
	for _, key := range required {
		normalized[key] = normalized[key] / sum
	}
	return normalized
}

func normalizeAI2RiskFlagsForDebug(raw interface{}) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, item := range parseStringSliceFromAny(raw) {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func mergeFirstQualityWeightsForDebug(candidates ...interface{}) map[string]float64 {
	for _, candidate := range candidates {
		weights := parseAI2QualityWeightsForDebug(candidate)
		if len(weights) > 0 {
			return normalizeAI2QualityWeightsForDebug(weights)
		}
	}
	return map[string]float64{}
}

func mergeFirstRiskFlagsForDebug(candidates ...interface{}) []string {
	for _, candidate := range candidates {
		flags := normalizeAI2RiskFlagsForDebug(candidate)
		if len(flags) > 0 {
			return flags
		}
	}
	return nil
}

func firstNonEmptyStringValueForDebug(values ...interface{}) string {
	for _, value := range values {
		text := strings.TrimSpace(stringFromAny(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func buildAI2ExecutionObservability(
	requestedFormat string,
	options map[string]interface{},
	metrics map[string]interface{},
	ai2Instruction map[string]interface{},
) map[string]interface{} {
	prefix := resolveImageMetricPrefixForDebug(requestedFormat)
	metricAI2Guidance := mapFromAnyValue(metrics[prefix+"_ai2_guidance_v1"])
	metricWorkerStrategy := mapFromAnyValue(metrics[prefix+"_worker_strategy_v1"])
	metricExtraction := mapFromAnyValue(metrics[prefix+"_extraction_v1"])
	frameQuality := mapFromAnyValue(metrics["frame_quality"])
	optionAI2Guidance := mapFromAnyValue(options["ai2_guidance_v1"])
	optionWorkerStrategy := mapFromAnyValue(options["ai2_worker_strategy_v1"])

	effectiveWeights := mergeFirstQualityWeightsForDebug(
		frameQuality["scoring_weights"],
		metricAI2Guidance["quality_weights"],
		optionAI2Guidance["quality_weights"],
		options["ai2_quality_weights_v1"],
		ai2Instruction["quality_weights"],
	)
	appliedRiskFlags := mergeFirstRiskFlagsForDebug(
		frameQuality["risk_flags"],
		metricWorkerStrategy["risk_flags"],
		metricAI2Guidance["risk_flags"],
		optionWorkerStrategy["risk_flags"],
		optionAI2Guidance["risk_flags"],
		options["ai2_risk_flags_v1"],
		ai2Instruction["risk_flags"],
	)
	technicalReject := mapFromAnyValue(ai2Instruction["technical_reject"])
	if len(technicalReject) == 0 {
		technicalReject = mapFromAnyValue(options["ai2_technical_reject_v1"])
	}
	if len(technicalReject) == 0 {
		technicalReject = mapFromAnyValue(metricWorkerStrategy["technical_reject"])
	}
	maxBlurTolerance := firstNonEmptyStringValueForDebug(
		frameQuality["max_blur_tolerance"],
		technicalReject["max_blur_tolerance"],
		optionAI2Guidance["max_blur_tolerance"],
		metricAI2Guidance["max_blur_tolerance"],
	)
	selectorVersion := firstNonEmptyStringValueForDebug(
		frameQuality["selector_version"],
		metricExtraction["selector_version"],
		metricAI2Guidance["selector_version"],
	)
	scoringMode := firstNonEmptyStringValueForDebug(
		frameQuality["scoring_mode"],
		metricExtraction["scoring_mode"],
		optionAI2Guidance["scoring_mode"],
		metricAI2Guidance["scoring_mode"],
	)
	selectionPolicy := firstNonEmptyStringValueForDebug(
		frameQuality["selection_policy"],
		metricExtraction["selection_policy"],
		optionAI2Guidance["selection_policy"],
		metricAI2Guidance["selection_policy"],
	)
	scoringFormula := firstNonEmptyStringValueForDebug(
		frameQuality["scoring_formula"],
		metricExtraction["scoring_formula"],
		optionAI2Guidance["scoring_formula"],
		metricAI2Guidance["scoring_formula"],
	)
	candidateExplainability := buildAI2CandidateExplainabilitySummary(frameQuality["candidate_scores"])

	if len(effectiveWeights) == 0 && len(appliedRiskFlags) == 0 && len(technicalReject) == 0 &&
		maxBlurTolerance == "" && selectorVersion == "" && scoringMode == "" && selectionPolicy == "" && scoringFormula == "" &&
		len(metricWorkerStrategy) == 0 && len(candidateExplainability) == 0 {
		return map[string]interface{}{}
	}

	out := map[string]interface{}{
		"schema_version":     "ai2_execution_observability_v1",
		"requested_format":   normalizeRequestedFormatForDebug(requestedFormat),
		"selector_version":   selectorVersion,
		"scoring_mode":       scoringMode,
		"selection_policy":   selectionPolicy,
		"scoring_formula":    scoringFormula,
		"risk_flags_applied": appliedRiskFlags,
		"guidance_sources": map[string]interface{}{
			"from_ai2_instruction": len(ai2Instruction) > 0,
			"from_options":         len(optionAI2Guidance) > 0 || len(optionWorkerStrategy) > 0,
			"from_metrics":         len(metricAI2Guidance) > 0 || len(metricWorkerStrategy) > 0 || len(frameQuality) > 0,
		},
	}
	if len(effectiveWeights) > 0 {
		out["effective_quality_weights"] = effectiveWeights
	}
	if maxBlurTolerance != "" {
		out["max_blur_tolerance"] = strings.ToLower(strings.TrimSpace(maxBlurTolerance))
	}
	if len(technicalReject) > 0 {
		out["technical_reject"] = technicalReject
	}
	if len(metricWorkerStrategy) > 0 {
		out["worker_strategy"] = metricWorkerStrategy
	} else if len(optionWorkerStrategy) > 0 {
		out["worker_strategy"] = optionWorkerStrategy
	}
	if len(frameQuality) > 0 {
		summary := map[string]interface{}{
			"kept_frames":      intFromAnyValue(frameQuality["kept_frames"]),
			"total_frames":     intFromAnyValue(frameQuality["total_frames"]),
			"fallback_applied": boolFromAny(frameQuality["fallback_applied"]),
			"reject_counts": map[string]interface{}{
				"blur":         intFromAnyValue(frameQuality["rejected_blur"]),
				"brightness":   intFromAnyValue(frameQuality["rejected_brightness"]),
				"exposure":     intFromAnyValue(frameQuality["rejected_exposure"]),
				"resolution":   intFromAnyValue(frameQuality["rejected_resolution"]),
				"still_blur":   intFromAnyValue(frameQuality["rejected_still_blur_gate"]),
				"watermark":    intFromAnyValue(frameQuality["rejected_watermark"]),
				"near_dup":     intFromAnyValue(frameQuality["rejected_near_duplicate"]),
				"total_reject": intFromAnyValue(frameQuality["total_frames"]) - intFromAnyValue(frameQuality["kept_frames"]),
			},
		}
		if candidateCount := intFromAnyValue(frameQuality["candidate_count"]); candidateCount > 0 {
			summary["candidate_count"] = candidateCount
		} else if len(candidateExplainability) > 0 {
			summary["candidate_count"] = intFromAnyValue(candidateExplainability["total_candidates"])
		}
		out["frame_quality_summary"] = summary
	}
	if len(candidateExplainability) > 0 {
		out["candidate_explainability"] = candidateExplainability
	}
	return out
}

func buildPipelineAlignmentReportV1(
	requestedFormat string,
	sourcePrompt string,
	advancedOptions map[string]interface{},
	strategyProfile map[string]interface{},
	strategyOverrideReport map[string]interface{},
	advancedSceneGuard map[string]interface{},
	userReply map[string]interface{},
	ai2Instruction map[string]interface{},
	ai2ExecutionObservability map[string]interface{},
	options map[string]interface{},
	metrics map[string]interface{},
) map[string]interface{} {
	format := normalizeRequestedFormatForDebug(requestedFormat)
	if format == "" {
		format = "unknown"
	}
	metricPrefix := resolveImageMetricPrefixForDebug(format)
	pipelineMetric := mapFromAnyValue(metrics[fmt.Sprintf("%s_pipeline_v1", metricPrefix)])
	stageStatus := mapFromAnyValue(metrics[fmt.Sprintf("%s_pipeline_stage_status_v1", metricPrefix)])
	ai1Metric := mapFromAnyValue(metrics[fmt.Sprintf("%s_ai1_plan_v1", metricPrefix)])
	ai2Metric := mapFromAnyValue(metrics[fmt.Sprintf("%s_ai2_instruction_v1", metricPrefix)])
	workerMetric := mapFromAnyValue(metrics[fmt.Sprintf("%s_worker_strategy_v1", metricPrefix)])
	if len(workerMetric) == 0 {
		workerMetric = mapFromAnyValue(options["ai2_worker_strategy_v1"])
	}
	ai3Metric := mapFromAnyValue(metrics[fmt.Sprintf("%s_ai3_review_v1", metricPrefix)])

	ai2Advanced := mapFromAnyValue(ai2Instruction["advanced_options"])
	scene := videojobs.NormalizeAdvancedScenario(firstNonEmptyString(
		stringFromAny(advancedOptions["scene"]),
		stringFromAny(advancedOptions["scenario"]),
		stringFromAny(strategyProfile["scene"]),
		stringFromAny(strategyOverrideReport["scene"]),
		stringFromAny(ai2Advanced["scene"]),
	))
	if scene == "" {
		scene = videojobs.AdvancedScenarioDefault
	}
	sceneLabel := firstNonEmptyString(
		stringFromAny(strategyProfile["scene_label"]),
		stringFromAny(strategyOverrideReport["scene_label"]),
		stringFromAny(ai2Advanced["scene_label"]),
		resolvePipelineSceneLabelForDebug(scene),
	)
	visualFocus := parseStringSliceFromAny(advancedOptions["visual_focus"])
	if len(visualFocus) == 0 {
		visualFocus = parseStringSliceFromAny(ai2Advanced["visual_focus"])
	}
	enableMatting := boolFromAny(advancedOptions["enable_matting"])
	if !enableMatting {
		enableMatting = boolFromAny(ai2Advanced["enable_matting"])
	}

	requestedWeights := parseAI2QualityWeightsFlexible(ai2Instruction["quality_weights"])
	effectiveWeights := parseAI2QualityWeightsFlexible(ai2ExecutionObservability["effective_quality_weights"])
	requestedRiskFlags := normalizeAI2RiskFlagsForDebug(ai2Instruction["risk_flags"])
	appliedRiskFlags := normalizeAI2RiskFlagsForDebug(ai2ExecutionObservability["risk_flags_applied"])

	technicalRejectRequested := mapFromAnyValue(ai2Instruction["technical_reject"])
	technicalRejectEffective := mapFromAnyValue(ai2ExecutionObservability["technical_reject"])
	if len(technicalRejectEffective) == 0 {
		technicalRejectEffective = mapFromAnyValue(workerMetric["technical_reject"])
	}
	frameQualitySummary := mapFromAnyValue(ai2ExecutionObservability["frame_quality_summary"])
	rejectCounts := mapFromAnyValue(frameQualitySummary["reject_counts"])
	candidateExplainability := mapFromAnyValue(ai2ExecutionObservability["candidate_explainability"])

	ai1Node := map[string]interface{}{
		"source_prompt":             strings.TrimSpace(sourcePrompt),
		"intent_understanding":      strings.TrimSpace(stringFromAny(userReply["intent_understanding"])),
		"strategy_summary":          strings.TrimSpace(stringFromAny(userReply["strategy_summary"])),
		"interactive_action":        strings.TrimSpace(stringFromAny(userReply["interactive_action"])),
		"strategy_profile":          strategyProfile,
		"strategy_override_report":  strategyOverrideReport,
		"ai2_directive_seed_fields": buildPipelineAI2DirectiveSeedFields(ai2Instruction),
	}
	if len(ai1Metric) > 0 {
		ai1Node["metric"] = ai1Metric
	}

	ai2Node := map[string]interface{}{
		"objective":                 strings.TrimSpace(stringFromAny(ai2Instruction["objective"])),
		"style_direction":           strings.TrimSpace(stringFromAny(ai2Instruction["style_direction"])),
		"requested_quality_weights": requestedWeights,
		"effective_quality_weights": effectiveWeights,
		"requested_risk_flags":      requestedRiskFlags,
		"applied_risk_flags":        appliedRiskFlags,
		"technical_reject":          technicalRejectRequested,
		"selector_version":          strings.TrimSpace(stringFromAny(ai2ExecutionObservability["selector_version"])),
		"scoring_mode":              strings.TrimSpace(stringFromAny(ai2ExecutionObservability["scoring_mode"])),
		"scoring_formula":           strings.TrimSpace(stringFromAny(ai2ExecutionObservability["scoring_formula"])),
		"selection_policy":          strings.TrimSpace(stringFromAny(ai2ExecutionObservability["selection_policy"])),
	}
	if len(ai2Metric) > 0 {
		ai2Node["metric"] = ai2Metric
	}
	if len(candidateExplainability) > 0 {
		ai2Node["candidate_explainability"] = candidateExplainability
	}

	workerNode := map[string]interface{}{
		"strategy":              workerMetric,
		"technical_reject":      technicalRejectEffective,
		"frame_quality_summary": frameQualitySummary,
		"stage": map[string]interface{}{
			"worker":     strings.TrimSpace(stringFromAny(stageStatus["worker"])),
			"extraction": strings.TrimSpace(stringFromAny(stageStatus["extraction"])),
		},
	}

	ai3Node := map[string]interface{}{
		"review": ai3Metric,
		"stage":  strings.TrimSpace(stringFromAny(stageStatus["ai3"])),
	}

	report := map[string]interface{}{
		"schema_version":   "pipeline_alignment_report_v1",
		"requested_format": format,
		"scenario": map[string]interface{}{
			"scene":          scene,
			"scene_label":    sceneLabel,
			"visual_focus":   visualFocus,
			"enable_matting": enableMatting,
		},
		"ai1":          ai1Node,
		"ai2":          ai2Node,
		"worker":       workerNode,
		"ai3":          ai3Node,
		"stage_status": stageStatus,
	}
	if len(pipelineMetric) > 0 {
		report["pipeline_metric"] = pipelineMetric
	}
	if len(advancedSceneGuard) > 0 {
		report["advanced_scene_guard_v1"] = advancedSceneGuard
	}
	sceneBaselineDiff := buildPipelineSceneBaselineDiffV1(format, scene, strategyProfile, ai2Instruction, technicalRejectRequested)
	if len(sceneBaselineDiff) > 0 {
		report["scene_baseline_diff_v1"] = sceneBaselineDiff
	}

	checks := make([]map[string]interface{}, 0, 8)
	appendCheck := func(key, label, status, detail string, value interface{}) {
		entry := map[string]interface{}{
			"key":    strings.TrimSpace(key),
			"label":  strings.TrimSpace(label),
			"status": strings.TrimSpace(strings.ToLower(status)),
			"detail": strings.TrimSpace(detail),
		}
		if value != nil {
			entry["value"] = value
		}
		checks = append(checks, entry)
	}

	if scene == "" {
		appendCheck("scene_resolved", "场景解析", "warn", "未解析到场景，已按默认策略运行。", nil)
	} else {
		appendCheck("scene_resolved", "场景解析", "pass", "已解析场景并注入 AI1/AI2。", map[string]interface{}{
			"scene":       scene,
			"scene_label": sceneLabel,
		})
	}
	if len(advancedSceneGuard) > 0 {
		appendCheck("scene_guard_applied", "PNG主线场景收敛", "pass", "场景已按主线范围收敛。", advancedSceneGuard)
	}
	if len(sceneBaselineDiff) > 0 {
		sceneChanged := boolFromAny(sceneBaselineDiff["scene_changed"])
		weightsDiffKeys := parseStringSliceFromAny(sceneBaselineDiff["weight_diff_keys"])
		mustAdded := parseStringSliceFromAny(sceneBaselineDiff["must_capture_added"])
		avoidAdded := parseStringSliceFromAny(sceneBaselineDiff["avoid_added"])
		technicalChanged := parseStringSliceFromAny(sceneBaselineDiff["technical_reject_changed_keys"])
		if sceneChanged {
			detail := "当前场景已与 default 基线形成可视化差异。"
			if len(weightsDiffKeys) > 0 || len(mustAdded) > 0 || len(avoidAdded) > 0 || len(technicalChanged) > 0 {
				detail = "当前场景相对 default 基线存在已配置差异（权重/规则/门槛）。"
			}
			appendCheck("scene_baseline_diff", "场景基线差异", "pass", detail, map[string]interface{}{
				"weight_diff_keys":              weightsDiffKeys,
				"must_capture_added":            mustAdded,
				"avoid_added":                   avoidAdded,
				"technical_reject_changed_keys": technicalChanged,
			})
		} else {
			appendCheck("scene_baseline_diff", "场景基线差异", "warn", "当前即 default 场景，基线差异仅用于参考。", map[string]interface{}{
				"weight_diff_keys":              weightsDiffKeys,
				"technical_reject_changed_keys": technicalChanged,
			})
		}
	}

	strategyStyle := strings.ToLower(strings.TrimSpace(stringFromAny(strategyProfile["style_direction"])))
	directiveStyle := strings.ToLower(strings.TrimSpace(stringFromAny(ai2Instruction["style_direction"])))
	switch {
	case strategyStyle == "" && directiveStyle == "":
		appendCheck("style_direction_locked", "风格方向锁定", "warn", "未配置 style_direction，当前依赖模型自动输出。", nil)
	case strategyStyle == "":
		appendCheck("style_direction_locked", "风格方向锁定", "pass", "策略未强制 style_direction，AI2 指令已有风格方向。", map[string]interface{}{
			"directive_style_direction": directiveStyle,
		})
	case directiveStyle == "":
		appendCheck("style_direction_locked", "风格方向锁定", "warn", "策略已配置 style_direction，但 AI2 指令为空。", map[string]interface{}{
			"strategy_style_direction": strategyStyle,
		})
	case strategyStyle == directiveStyle:
		appendCheck("style_direction_locked", "风格方向锁定", "pass", "AI2 指令风格方向与策略一致。", map[string]interface{}{
			"style_direction": strategyStyle,
		})
	default:
		appendCheck("style_direction_locked", "风格方向锁定", "fail", "AI2 指令风格方向与策略不一致。", map[string]interface{}{
			"strategy_style_direction":  strategyStyle,
			"directive_style_direction": directiveStyle,
		})
	}

	if len(requestedWeights) == 0 && len(effectiveWeights) == 0 {
		appendCheck("quality_weights_applied", "质量权重生效", "warn", "未检测到请求权重与执行权重。", nil)
	} else if len(requestedWeights) == 0 {
		appendCheck("quality_weights_applied", "质量权重生效", "warn", "缺少 AI2 请求权重，无法完成严格比对。", map[string]interface{}{
			"effective_quality_weights": effectiveWeights,
		})
	} else if len(effectiveWeights) == 0 {
		appendCheck("quality_weights_applied", "质量权重生效", "fail", "未检测到执行生效权重。", map[string]interface{}{
			"requested_quality_weights": requestedWeights,
		})
	} else {
		mismatchKeys := diffAI2QualityWeightsForDebug(requestedWeights, effectiveWeights, 0.03)
		if len(mismatchKeys) == 0 {
			appendCheck("quality_weights_applied", "质量权重生效", "pass", "AI2 请求权重与执行权重一致。", map[string]interface{}{
				"requested_quality_weights": requestedWeights,
				"effective_quality_weights": effectiveWeights,
			})
		} else {
			appendCheck("quality_weights_applied", "质量权重生效", "fail", "检测到权重漂移。", map[string]interface{}{
				"mismatch_keys":             mismatchKeys,
				"requested_quality_weights": requestedWeights,
				"effective_quality_weights": effectiveWeights,
			})
		}
	}

	requestedAvoidWatermarks := boolFromAny(technicalRejectRequested["avoid_watermarks"])
	effectiveAvoidWatermarks := boolFromAny(technicalRejectEffective["avoid_watermarks"])
	watermarkRejectCount := intFromAnyValue(rejectCounts["watermark"])
	switch {
	case requestedAvoidWatermarks && effectiveAvoidWatermarks:
		detail := "水印硬门槛已生效。"
		if watermarkRejectCount > 0 {
			detail = fmt.Sprintf("水印硬门槛已生效，已拒绝 %d 帧。", watermarkRejectCount)
		}
		appendCheck("watermark_gate_active", "水印门槛生效", "pass", detail, map[string]interface{}{
			"requested_avoid_watermarks": requestedAvoidWatermarks,
			"effective_avoid_watermarks": effectiveAvoidWatermarks,
			"rejected_watermark_frames":  watermarkRejectCount,
		})
	case requestedAvoidWatermarks && !effectiveAvoidWatermarks:
		appendCheck("watermark_gate_active", "水印门槛生效", "fail", "AI2 指令要求避开水印，但 Worker 未确认生效。", map[string]interface{}{
			"requested_avoid_watermarks": requestedAvoidWatermarks,
			"effective_avoid_watermarks": effectiveAvoidWatermarks,
		})
	default:
		appendCheck("watermark_gate_active", "水印门槛生效", "warn", "当前场景未开启 avoid_watermarks 硬门槛。", map[string]interface{}{
			"requested_avoid_watermarks": requestedAvoidWatermarks,
		})
	}

	ai3Stage := strings.ToLower(strings.TrimSpace(stringFromAny(stageStatus["ai3"])))
	if len(ai3Metric) > 0 {
		appendCheck("ai3_review_available", "AI3 评审产物", "pass", "已生成 AI3 审核摘要。", map[string]interface{}{
			"stage": ai3Stage,
		})
	} else if ai3Stage == "running" || ai3Stage == "pending" {
		appendCheck("ai3_review_available", "AI3 评审产物", "warn", "AI3 阶段尚在执行中。", map[string]interface{}{
			"stage": ai3Stage,
		})
	} else {
		appendCheck("ai3_review_available", "AI3 评审产物", "warn", "未检测到 AI3 评审摘要。", map[string]interface{}{
			"stage": ai3Stage,
		})
	}

	ai1Stage := strings.ToLower(strings.TrimSpace(stringFromAny(stageStatus["ai1"])))
	ai2Stage := strings.ToLower(strings.TrimSpace(stringFromAny(stageStatus["ai2"])))
	workerStage := strings.ToLower(strings.TrimSpace(stringFromAny(stageStatus["worker"])))
	if isPipelineStageDoneForDebug(ai1Stage) && isPipelineStageDoneForDebug(ai2Stage) && isPipelineStageDoneForDebug(workerStage) {
		appendCheck("mainline_stage_status", "主线阶段状态", "pass", "AI1/AI2/Worker 阶段均已完成。", map[string]interface{}{
			"ai1":    ai1Stage,
			"ai2":    ai2Stage,
			"worker": workerStage,
		})
	} else {
		appendCheck("mainline_stage_status", "主线阶段状态", "warn", "主线阶段存在未完成状态，请结合 stage_status 排查。", map[string]interface{}{
			"ai1":    ai1Stage,
			"ai2":    ai2Stage,
			"worker": workerStage,
		})
	}

	passCount := 0
	warnCount := 0
	failCount := 0
	for _, item := range checks {
		switch strings.ToLower(strings.TrimSpace(stringFromAny(item["status"]))) {
		case "pass":
			passCount++
		case "fail":
			failCount++
		default:
			warnCount++
		}
	}

	finalStatus := "pass"
	if failCount > 0 {
		finalStatus = "fail"
	} else if warnCount > 0 {
		finalStatus = "warn"
	}
	report["consistency_checks"] = checks
	report["summary"] = map[string]interface{}{
		"status":       finalStatus,
		"pass_count":   passCount,
		"warn_count":   warnCount,
		"fail_count":   failCount,
		"total_checks": len(checks),
	}
	return report
}

func buildPipelineAI2DirectiveSeedFields(ai2Instruction map[string]interface{}) map[string]interface{} {
	if len(ai2Instruction) == 0 {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{
		"objective":        strings.TrimSpace(stringFromAny(ai2Instruction["objective"])),
		"must_capture":     parseStringSliceFromAny(ai2Instruction["must_capture"]),
		"avoid":            parseStringSliceFromAny(ai2Instruction["avoid"]),
		"style_direction":  strings.TrimSpace(stringFromAny(ai2Instruction["style_direction"])),
		"quality_weights":  parseAI2QualityWeightsFlexible(ai2Instruction["quality_weights"]),
		"technical_reject": mapFromAnyValue(ai2Instruction["technical_reject"]),
	}
	return out
}

func buildPipelineSceneBaselineDiffV1(
	requestedFormat string,
	scene string,
	strategyProfile map[string]interface{},
	ai2Instruction map[string]interface{},
	technicalRejectRequested map[string]interface{},
) map[string]interface{} {
	if normalizeRequestedFormatForDebug(requestedFormat) != "png" {
		return nil
	}

	currentScene := videojobs.NormalizeAdvancedScenario(scene)
	if currentScene == "" {
		currentScene = videojobs.AdvancedScenarioDefault
	}
	baselineScene := videojobs.AdvancedScenarioDefault
	baselineProfile := videojobs.ResolveVideoJobAI1StrategyProfile(
		"png",
		videojobs.VideoJobAdvancedOptions{Scene: baselineScene},
	)

	currentWeights := resolveFirstAI2QualityWeightsForBaselineDiff(
		ai2Instruction["quality_weights"],
		strategyProfile["quality_weights"],
	)
	if len(currentWeights) == 0 {
		currentWeights = normalizeAI2QualityWeightsForDebug(map[string]float64{
			"semantic":   baselineProfile.QualityWeights["semantic"],
			"clarity":    baselineProfile.QualityWeights["clarity"],
			"loop":       baselineProfile.QualityWeights["loop"],
			"efficiency": baselineProfile.QualityWeights["efficiency"],
		})
	}
	baselineWeights := resolveFirstAI2QualityWeightsForBaselineDiff(baselineProfile.QualityWeights)

	weightDelta := map[string]float64{}
	weightDiffKeys := make([]string, 0, 4)
	for _, key := range []string{"semantic", "clarity", "loop", "efficiency"} {
		delta := currentWeights[key] - baselineWeights[key]
		weightDelta[key] = delta
		if delta < -0.03 || delta > 0.03 {
			weightDiffKeys = append(weightDiffKeys, key)
		}
	}

	currentMustCapture := parseStringSliceFromAny(ai2Instruction["must_capture"])
	if len(currentMustCapture) == 0 {
		currentMustCapture = parseStringSliceFromAny(strategyProfile["must_capture_bias"])
	}
	currentAvoid := parseStringSliceFromAny(ai2Instruction["avoid"])
	if len(currentAvoid) == 0 {
		currentAvoid = parseStringSliceFromAny(strategyProfile["avoid_bias"])
	}
	baselineMustCapture := append([]string{}, baselineProfile.MustCaptureBias...)
	baselineAvoid := append([]string{}, baselineProfile.AvoidBias...)
	mustAdded, mustRemoved := diffStringSliceForBaseline(currentMustCapture, baselineMustCapture)
	avoidAdded, avoidRemoved := diffStringSliceForBaseline(currentAvoid, baselineAvoid)

	currentTechnical := mapFromAnyValue(ai2Instruction["technical_reject"])
	if len(currentTechnical) == 0 {
		currentTechnical = mapFromAnyValue(strategyProfile["technical_reject"])
	}
	if len(currentTechnical) == 0 {
		currentTechnical = baselineProfile.TechnicalReject
	}
	technicalChangedKeys := diffTechnicalRejectForBaseline(currentTechnical, baselineProfile.TechnicalReject)

	return map[string]interface{}{
		"schema_version":                "scene_baseline_diff_v1",
		"requested_format":              "png",
		"baseline_scene":                baselineScene,
		"current_scene":                 currentScene,
		"scene_changed":                 currentScene != baselineScene,
		"quality_weights_current":       currentWeights,
		"quality_weights_baseline":      baselineWeights,
		"quality_weight_delta":          weightDelta,
		"weight_diff_keys":              weightDiffKeys,
		"must_capture_current":          normalizeStringSliceForBaseline(currentMustCapture),
		"must_capture_baseline":         normalizeStringSliceForBaseline(baselineMustCapture),
		"must_capture_added":            mustAdded,
		"must_capture_removed":          mustRemoved,
		"avoid_current":                 normalizeStringSliceForBaseline(currentAvoid),
		"avoid_baseline":                normalizeStringSliceForBaseline(baselineAvoid),
		"avoid_added":                   avoidAdded,
		"avoid_removed":                 avoidRemoved,
		"technical_reject_current":      currentTechnical,
		"technical_reject_baseline":     baselineProfile.TechnicalReject,
		"technical_reject_changed_keys": technicalChangedKeys,
		"recommendation_summary":        buildSceneBaselineDiffSummary(currentScene, weightDiffKeys, mustAdded, avoidAdded, technicalChangedKeys),
	}
}

func resolveFirstAI2QualityWeightsForBaselineDiff(candidates ...interface{}) map[string]float64 {
	for _, candidate := range candidates {
		weights := parseAI2QualityWeightsFlexible(candidate)
		if len(weights) == 0 {
			continue
		}
		return normalizeAI2QualityWeightsForDebug(weights)
	}
	return map[string]float64{}
}

func normalizeStringSliceForBaseline(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func diffStringSliceForBaseline(current []string, baseline []string) (added []string, removed []string) {
	currentNorm := normalizeStringSliceForBaseline(current)
	baselineNorm := normalizeStringSliceForBaseline(baseline)
	currentSet := map[string]string{}
	for _, item := range currentNorm {
		currentSet[strings.ToLower(strings.TrimSpace(item))] = item
	}
	baselineSet := map[string]string{}
	for _, item := range baselineNorm {
		baselineSet[strings.ToLower(strings.TrimSpace(item))] = item
	}
	for key, item := range currentSet {
		if _, exists := baselineSet[key]; exists {
			continue
		}
		added = append(added, item)
	}
	for key, item := range baselineSet {
		if _, exists := currentSet[key]; exists {
			continue
		}
		removed = append(removed, item)
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func normalizeTechnicalRejectValueForDiff(raw interface{}) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case bool:
		if value {
			return "true"
		}
		return "false"
	case string:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.TrimSpace(strings.ToLower(stringFromAny(raw)))
	}
}

func diffTechnicalRejectForBaseline(current map[string]interface{}, baseline map[string]interface{}) []string {
	if len(current) == 0 && len(baseline) == 0 {
		return nil
	}
	keySet := map[string]struct{}{}
	for key := range current {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keySet[key] = struct{}{}
	}
	for key := range baseline {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keySet[key] = struct{}{}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	changed := make([]string, 0, len(keys))
	for _, key := range keys {
		cv := normalizeTechnicalRejectValueForDiff(current[key])
		bv := normalizeTechnicalRejectValueForDiff(baseline[key])
		if cv == bv {
			continue
		}
		changed = append(changed, key)
	}
	return changed
}

func buildSceneBaselineDiffSummary(
	currentScene string,
	weightDiffKeys []string,
	mustAdded []string,
	avoidAdded []string,
	technicalChangedKeys []string,
) string {
	if currentScene == videojobs.AdvancedScenarioDefault {
		return "当前场景为 default，作为主线基线参考。"
	}
	parts := make([]string, 0, 4)
	if len(weightDiffKeys) > 0 {
		parts = append(parts, fmt.Sprintf("权重差异 %d 项", len(weightDiffKeys)))
	}
	if len(mustAdded) > 0 {
		parts = append(parts, fmt.Sprintf("must_capture 新增 %d 项", len(mustAdded)))
	}
	if len(avoidAdded) > 0 {
		parts = append(parts, fmt.Sprintf("avoid 新增 %d 项", len(avoidAdded)))
	}
	if len(technicalChangedKeys) > 0 {
		parts = append(parts, fmt.Sprintf("技术门槛变化 %d 项", len(technicalChangedKeys)))
	}
	if len(parts) == 0 {
		return "当前场景与 default 基线差异较小。"
	}
	return "相对 default 基线：" + strings.Join(parts, "，") + "。"
}

func resolvePipelineSceneLabelForDebug(scene string) string {
	switch videojobs.NormalizeAdvancedScenario(scene) {
	case videojobs.AdvancedScenarioXiaohongshu:
		return "小红书网感"
	case videojobs.AdvancedScenarioWallpaper:
		return "手机壁纸"
	case videojobs.AdvancedScenarioNews:
		return "新闻配图"
	default:
		return "通用截图"
	}
}

func parseAI2QualityWeightsFlexible(raw interface{}) map[string]float64 {
	switch value := raw.(type) {
	case map[string]float64:
		if len(value) == 0 {
			return map[string]float64{}
		}
		return normalizeAI2QualityWeightsForDebug(value)
	case map[string]interface{}:
		return parseAI2QualityWeightsForDebug(value)
	default:
		return parseAI2QualityWeightsForDebug(raw)
	}
}

func diffAI2QualityWeightsForDebug(a, b map[string]float64, tolerance float64) []string {
	keys := []string{"semantic", "clarity", "loop", "efficiency"}
	out := make([]string, 0, len(keys))
	if tolerance < 0 {
		tolerance = 0
	}
	for _, key := range keys {
		diff := a[key] - b[key]
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			out = append(out, key)
		}
	}
	return out
}

func isPipelineStageDoneForDebug(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "reused", "completed", "success":
		return true
	default:
		return false
	}
}

func buildAI2CandidateExplainabilitySummary(raw interface{}) map[string]interface{} {
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}

	decisionCounts := map[string]int{}
	mustCaptureHitFrames := 0
	avoidHitFrames := 0
	topRows := make([]map[string]interface{}, 0, 8)
	totalRows := 0

	for _, item := range items {
		row := mapFromAnyValue(item)
		if len(row) == 0 {
			continue
		}
		totalRows++
		decision := strings.ToLower(strings.TrimSpace(stringFromAny(row["decision"])))
		if decision == "" {
			decision = "unknown"
		}
		decisionCounts[decision]++

		mustCaptureHits := parseStringSliceFromAny(row["must_capture_hits"])
		avoidHits := parseStringSliceFromAny(row["avoid_hits"])
		if len(mustCaptureHits) > 0 {
			mustCaptureHitFrames++
		}
		if len(avoidHits) > 0 {
			avoidHitFrames++
		}

		if len(topRows) >= 8 {
			continue
		}
		topRows = append(topRows, map[string]interface{}{
			"rank":              intFromAnyValue(row["rank"]),
			"frame_name":        firstNonEmptyStringValueForDebug(row["frame_name"], row["frame_path"]),
			"decision":          decision,
			"reject_reason":     strings.TrimSpace(stringFromAny(row["reject_reason"])),
			"must_capture_hits": mustCaptureHits,
			"avoid_hits":        avoidHits,
			"summary":           strings.TrimSpace(stringFromAny(row["explain_summary"])),
		})
	}

	if totalRows == 0 {
		return nil
	}
	return map[string]interface{}{
		"total_candidates":        totalRows,
		"decision_counts":         decisionCounts,
		"must_capture_hit_frames": mustCaptureHitFrames,
		"avoid_hit_frames":        avoidHitFrames,
		"top_rows":                topRows,
	}
}

func parseStringSliceFromAny(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok {
		if arr, ok2 := raw.([]string); ok2 {
			out := make([]string, 0, len(arr))
			for _, item := range arr {
				text := strings.TrimSpace(item)
				if text == "" {
					continue
				}
				out = append(out, text)
			}
			return out
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(stringFromAny(item))
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func normalizeAI1EditableStringSlice(raw []string, maxN int) []string {
	if maxN <= 0 {
		maxN = 16
	}
	out := make([]string, 0, minIntValue(len(raw), maxN))
	seen := map[string]struct{}{}
	for _, item := range raw {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
		if len(out) >= maxN {
			break
		}
	}
	if len(out) == 0 {
		return []string{}
	}
	return out
}

func normalizeAI1PatchRiskFlags(raw []string, maxN int) []string {
	if maxN <= 0 {
		maxN = 16
	}
	out := make([]string, 0, minIntValue(len(raw), maxN))
	seen := map[string]struct{}{}
	for _, item := range raw {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" {
			continue
		}
		value = strings.ReplaceAll(value, " ", "_")
		value = strings.ReplaceAll(value, "-", "_")
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= maxN {
			break
		}
	}
	return out
}

func normalizeAI1PatchQualityWeights(raw map[string]float64) (map[string]float64, error) {
	out := map[string]float64{
		"semantic":   0,
		"clarity":    0,
		"loop":       0,
		"efficiency": 0,
	}
	sum := 0.0
	for key, value := range raw {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if _, ok := out[normalizedKey]; !ok {
			continue
		}
		if value < 0 {
			value = 0
		}
		if value > 1 {
			value = 1
		}
		out[normalizedKey] = value
		sum += value
	}
	if sum <= 0 {
		return nil, fmt.Errorf("quality_weights must contain at least one positive value")
	}
	for key, value := range out {
		out[key] = value / sum
	}
	return out, nil
}

func normalizeAI1PatchQualityWeightsFromAny(raw interface{}) map[string]float64 {
	out := map[string]float64{}
	value := mapFromAnyValue(raw)
	for _, key := range []string{"semantic", "clarity", "loop", "efficiency"} {
		v := floatFromAnyValue(value[key])
		if v <= 0 {
			continue
		}
		out[key] = v
	}
	if len(out) == 0 {
		return nil
	}
	normalized, err := normalizeAI1PatchQualityWeights(out)
	if err != nil {
		return nil
	}
	return normalized
}

func defaultAI1PatchMaxBlurTolerance(targetFormat string) string {
	switch normalizeRequestedFormatForDebug(targetFormat) {
	case "png", "jpg", "webp":
		return "low"
	default:
		return "medium"
	}
}

func normalizeAI1PatchMaxBlurTolerance(raw string, targetFormat string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "low", "medium", "high":
		return value, nil
	case "":
		return defaultAI1PatchMaxBlurTolerance(targetFormat), nil
	default:
		return "", fmt.Errorf("max_blur_tolerance must be one of: low, medium, high")
	}
}

func extractAI1PlanRevision(plan map[string]interface{}) int {
	revision := intFromAnyValue(plan["plan_revision"])
	if revision <= 0 {
		return 1
	}
	return revision
}

func minIntValue(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func intFromAnyValue(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case int32:
		return int(value)
	case uint:
		return int(value)
	case uint64:
		return int(value)
	case uint32:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	case json.Number:
		i, err := value.Int64()
		if err == nil {
			return int(i)
		}
		f, err := value.Float64()
		if err == nil {
			return int(f)
		}
		return 0
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

func sortedMapKeys(raw map[string]interface{}) []string {
	if len(raw) == 0 {
		return nil
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parseContractFieldStatus(raw map[string]interface{}, field string, invalid []string) []string {
	switch strings.TrimSpace(strings.ToLower(stringFromAny(raw[field]))) {
	case "":
		return append(invalid, field)
	default:
		return invalid
	}
}

func buildAI1OutputContractReport(ai1OutputV2 map[string]interface{}) map[string]interface{} {
	report := map[string]interface{}{
		"schema":  videojobs.AI1OutputSchemaV2,
		"present": len(ai1OutputV2) > 0,
	}
	if len(ai1OutputV2) == 0 {
		report["valid"] = false
		report["missing_fields"] = []string{
			"schema_version",
			"user_feedback",
			"ai2_directive",
			"trace",
		}
		return report
	}

	missing := make([]string, 0, 8)
	invalid := make([]string, 0, 8)

	if strings.TrimSpace(stringFromAny(ai1OutputV2["schema_version"])) == "" {
		missing = append(missing, "schema_version")
	} else if strings.TrimSpace(stringFromAny(ai1OutputV2["schema_version"])) != videojobs.AI1OutputSchemaV2 {
		invalid = append(invalid, "schema_version")
	}

	userFeedback := mapFromAnyValue(ai1OutputV2["user_feedback"])
	if len(userFeedback) == 0 {
		missing = append(missing, "user_feedback")
	} else {
		if strings.TrimSpace(stringFromAny(userFeedback["schema_version"])) != videojobs.AI1UserFeedbackSchemaV2 {
			invalid = append(invalid, "user_feedback.schema_version")
		}
		for _, key := range []string{
			"summary",
			"intent_understanding",
			"strategy_summary",
			"interactive_action",
			"risk_warning",
			"confidence",
		} {
			if userFeedback[key] == nil || strings.TrimSpace(stringFromAny(userFeedback[key])) == "" {
				if key == "risk_warning" {
					if len(mapFromAnyValue(userFeedback["risk_warning"])) == 0 {
						missing = append(missing, "user_feedback."+key)
					}
					continue
				}
				if key == "confidence" {
					if _, exists := userFeedback["confidence"]; !exists {
						missing = append(missing, "user_feedback."+key)
					}
					continue
				}
				missing = append(missing, "user_feedback."+key)
			}
		}
		action := strings.TrimSpace(strings.ToLower(stringFromAny(userFeedback["interactive_action"])))
		if action != "" && action != "proceed" && action != "need_clarify" {
			invalid = append(invalid, "user_feedback.interactive_action")
		}
		if _, exists := userFeedback["confidence"]; !exists {
			missing = append(missing, "user_feedback.confidence")
		} else {
			confidence, _ := strconv.ParseFloat(strings.TrimSpace(stringFromAny(userFeedback["confidence"])), 64)
			if confidence < 0 || confidence > 1 {
				invalid = append(invalid, "user_feedback.confidence")
			}
		}
		clarifyQuestions, clarifyExists := userFeedback["clarify_questions"]
		if !clarifyExists {
			missing = append(missing, "user_feedback.clarify_questions")
		} else if action == "need_clarify" && len(parseStringSliceFromAny(clarifyQuestions)) == 0 {
			invalid = append(invalid, "user_feedback.clarify_questions")
		}
		riskWarning := mapFromAnyValue(userFeedback["risk_warning"])
		if len(riskWarning) == 0 {
			missing = append(missing, "user_feedback.risk_warning")
		} else if _, exists := riskWarning["has_risk"]; !exists {
			missing = append(missing, "user_feedback.risk_warning.has_risk")
		}
	}

	ai2Directive := mapFromAnyValue(ai1OutputV2["ai2_directive"])
	if len(ai2Directive) == 0 {
		missing = append(missing, "ai2_directive")
	} else {
		if strings.TrimSpace(stringFromAny(ai2Directive["schema_version"])) != videojobs.AI2DirectiveSchemaV2 {
			invalid = append(invalid, "ai2_directive.schema_version")
		}
		invalid = parseContractFieldStatus(ai2Directive, "target_format", invalid)
		invalid = parseContractFieldStatus(ai2Directive, "objective", invalid)
		if len(mapFromAnyValue(ai2Directive["sampling_plan"])) == 0 {
			missing = append(missing, "ai2_directive.sampling_plan")
		}
		technicalReject := mapFromAnyValue(ai2Directive["technical_reject"])
		if len(technicalReject) == 0 {
			missing = append(missing, "ai2_directive.technical_reject")
		} else {
			switch strings.TrimSpace(strings.ToLower(stringFromAny(technicalReject["max_blur_tolerance"]))) {
			case "low", "medium", "high":
			default:
				invalid = append(invalid, "ai2_directive.technical_reject.max_blur_tolerance")
			}
		}
		qualityWeights := parseAI2QualityWeightsForDebug(ai2Directive["quality_weights"])
		if len(qualityWeights) == 0 {
			missing = append(missing, "ai2_directive.quality_weights")
		} else {
			required := []string{"semantic", "clarity", "loop", "efficiency"}
			sum := 0.0
			for _, key := range required {
				weight, ok := qualityWeights[key]
				if !ok {
					missing = append(missing, "ai2_directive.quality_weights."+key)
					continue
				}
				if weight < 0 || weight > 1 {
					invalid = append(invalid, "ai2_directive.quality_weights."+key)
				}
				sum += weight
			}
			if sum < 0.98 || sum > 1.02 {
				invalid = append(invalid, "ai2_directive.quality_weights.sum")
			}
		}
		switch strings.TrimSpace(strings.ToLower(stringFromAny(ai2Directive["visual_focus_area"]))) {
		case "", "auto", "center", "lower_third", "upper_half", "full_frame":
		default:
			invalid = append(invalid, "ai2_directive.visual_focus_area")
		}
		switch strings.TrimSpace(strings.ToLower(stringFromAny(ai2Directive["rhythm_trajectory"]))) {
		case "", "loop", "sudden_impact", "start_peak_fade":
		default:
			invalid = append(invalid, "ai2_directive.rhythm_trajectory")
		}
	}

	trace := mapFromAnyValue(ai1OutputV2["trace"])
	if len(trace) == 0 {
		missing = append(missing, "trace")
	}

	report["missing_fields"] = missing
	report["invalid_fields"] = invalid
	report["valid"] = len(missing) == 0 && len(invalid) == 0
	if len(trace) > 0 {
		report["contract_repaired"] = boolFromAny(trace["contract_repaired"])
		if items := parseStringSliceFromAny(trace["repair_items"]); len(items) > 0 {
			report["repair_items"] = items
		}
		if reason := strings.TrimSpace(stringFromAny(trace["repair_reason"])); reason != "" {
			report["repair_reason"] = reason
		}
	}
	return report
}

func summarizeModelUserPartsShape(raw interface{}) map[string]interface{} {
	parts, ok := raw.([]interface{})
	if !ok || len(parts) == 0 {
		return map[string]interface{}{
			"total_parts": 0,
		}
	}
	textCount := 0
	imageCount := 0
	videoCount := 0
	for _, part := range parts {
		partMap := mapFromAnyValue(part)
		switch strings.TrimSpace(strings.ToLower(stringFromAny(partMap["type"]))) {
		case "text":
			textCount++
		case "image_url":
			imageCount++
		case "video_url":
			videoCount++
		}
	}
	return map[string]interface{}{
		"total_parts": len(parts),
		"text_parts":  textCount,
		"image_parts": imageCount,
		"video_parts": videoCount,
	}
}

func buildAI1ModelRequestSummary(modelRequest map[string]interface{}, usageMetadata map[string]interface{}) map[string]interface{} {
	summary := map[string]interface{}{
		"provider": strings.TrimSpace(stringFromAny(modelRequest["provider"])),
		"model":    strings.TrimSpace(stringFromAny(modelRequest["model"])),
	}
	payload := mapFromAnyValue(usageMetadata["director_model_payload_v2"])
	if len(payload) > 0 {
		summary["payload_schema_version"] = strings.TrimSpace(stringFromAny(payload["schema_version"]))
		summary["payload_top_level_keys"] = sortedMapKeys(payload)
	}
	if payloadBytes := intFromAnyValue(usageMetadata["director_model_payload_bytes"]); payloadBytes > 0 {
		summary["payload_bytes"] = payloadBytes
	}
	if debugBytes := intFromAnyValue(usageMetadata["director_debug_context_bytes"]); debugBytes > 0 {
		summary["debug_context_bytes"] = debugBytes
	}
	summary["user_parts_shape"] = summarizeModelUserPartsShape(usageMetadata["user_parts_shape_v1"])

	for _, key := range []string{
		"director_input_mode_requested",
		"director_input_mode_applied",
		"director_input_source",
		"director_payload_schema_version",
	} {
		if value := strings.TrimSpace(stringFromAny(usageMetadata[key])); value != "" {
			summary[key] = value
		}
	}
	if text := strings.TrimSpace(stringFromAny(usageMetadata["system_prompt_text"])); text != "" {
		summary["system_prompt_len"] = len(text)
	}
	if enabled, exists := usageMetadata["operator_instruction_enabled"]; exists {
		summary["operator_instruction_enabled"] = boolFromAny(enabled)
	}
	if value := strings.TrimSpace(stringFromAny(usageMetadata["operator_instruction_version"])); value != "" {
		summary["operator_instruction_version"] = value
	}
	if value := strings.TrimSpace(stringFromAny(usageMetadata["fixed_prompt_version"])); value != "" {
		summary["fixed_prompt_version"] = value
	}
	return summary
}

func buildAI1ModelResponseSummary(
	directiveFound bool,
	directiveRow models.VideoJobGIFAIDirective,
	directiveRawResponse map[string]interface{},
	usageFound bool,
	usageRow models.VideoJobAIUsage,
) map[string]interface{} {
	summary := map[string]interface{}{
		"usage_found":     usageFound,
		"directive_found": directiveFound,
	}
	if usageFound {
		summary["request_status"] = strings.TrimSpace(usageRow.RequestStatus)
		summary["request_duration_ms"] = usageRow.RequestDurationMs
		if errText := strings.TrimSpace(usageRow.RequestError); errText != "" {
			summary["request_error"] = errText
		}
	}
	if directiveFound {
		summary["directive_status"] = strings.TrimSpace(directiveRow.Status)
		summary["directive_fallback_used"] = directiveRow.FallbackUsed
		summary["directive_prompt_version"] = strings.TrimSpace(directiveRow.PromptVersion)
		summary["directive_model_version"] = strings.TrimSpace(directiveRow.ModelVersion)
	}
	if len(directiveRawResponse) > 0 {
		summary["raw_response_top_level_keys"] = sortedMapKeys(directiveRawResponse)
		if choices, ok := directiveRawResponse["choices"].([]interface{}); ok {
			summary["raw_choices_count"] = len(choices)
		}
		if errMap := mapFromAnyValue(directiveRawResponse["error"]); len(errMap) > 0 {
			summary["raw_error"] = errMap
		}
	}
	return summary
}

func buildAI1DebugTimelineV1(
	sourcePrompt string,
	requestedFormat string,
	input map[string]interface{},
	modelRequest map[string]interface{},
	modelResponse map[string]interface{},
	output map[string]interface{},
	contractReport map[string]interface{},
	ai2ExecutionObservability map[string]interface{},
) []map[string]interface{} {
	timeline := make([]map[string]interface{}, 0, 5)
	format := strings.ToUpper(strings.TrimSpace(requestedFormat))
	if format == "" {
		format = "UNKNOWN"
	}

	appendStep := func(key, title, status, summary string, details map[string]interface{}) {
		step := map[string]interface{}{
			"key":     strings.TrimSpace(key),
			"title":   strings.TrimSpace(title),
			"status":  strings.TrimSpace(strings.ToLower(status)),
			"summary": strings.TrimSpace(summary),
		}
		if len(details) > 0 {
			step["details"] = details
		}
		timeline = append(timeline, step)
	}

	appendStep(
		"input",
		"输入聚合（用户 + 视频 + 开发规则）",
		"done",
		fmt.Sprintf("已构建 %s 任务输入，用户提示词长度 %d。", format, len(strings.TrimSpace(sourcePrompt))),
		map[string]interface{}{
			"source_prompt": sourcePrompt,
			"input_keys":    sortedMapKeys(input),
		},
	)

	modelRequestSummary := mapFromAnyValue(modelRequest["payload_summary_v2"])
	requestStatus := strings.TrimSpace(strings.ToLower(stringFromAny(modelRequest["request_status"])))
	requestStepStatus := "done"
	if requestStatus == "error" {
		requestStepStatus = "error"
	} else if len(modelRequestSummary) == 0 {
		requestStepStatus = "pending"
	}
	appendStep(
		"model_request",
		"POST 模型请求",
		requestStepStatus,
		"已组装模型请求负载（含 payload 摘要与输入模式）。",
		map[string]interface{}{
			"provider":           stringFromAny(modelRequest["provider"]),
			"model":              stringFromAny(modelRequest["model"]),
			"request_status":     stringFromAny(modelRequest["request_status"]),
			"payload_summary_v2": modelRequestSummary,
		},
	)

	responseSummary := mapFromAnyValue(modelResponse["response_summary_v2"])
	responseStepStatus := "done"
	if strings.TrimSpace(strings.ToLower(stringFromAny(responseSummary["request_status"]))) == "error" {
		responseStepStatus = "error"
	} else if !boolFromAny(responseSummary["usage_found"]) && !boolFromAny(responseSummary["directive_found"]) {
		responseStepStatus = "pending"
	}
	appendStep(
		"model_response",
		"模型响应解析",
		responseStepStatus,
		"已提取模型返回、usage 与 directive 标准化结果。",
		map[string]interface{}{
			"response_summary_v2": responseSummary,
		},
	)

	contractStatus := "done"
	if !boolFromAny(contractReport["valid"]) {
		contractStatus = "warn"
	}
	if boolFromAny(contractReport["contract_repaired"]) {
		contractStatus = "repaired"
	}
	appendStep(
		"contract",
		"AI1 协议校验与修复",
		contractStatus,
		"已执行 AI1 Output v2 契约校验，并记录修复轨迹。",
		map[string]interface{}{
			"contract_report": contractReport,
		},
	)

	ai2ExecutionStatus := "pending"
	ai2ExecutionSummary := "尚未采集到 AI2/Worker 实际执行观测数据。"
	ai2ExecutionDetails := mapFromAnyValue(ai2ExecutionObservability)
	if len(ai2ExecutionDetails) > 0 {
		weights := mapFromAnyValue(ai2ExecutionDetails["effective_quality_weights"])
		riskFlags := parseStringSliceFromAny(ai2ExecutionDetails["risk_flags_applied"])
		selectorVersion := strings.TrimSpace(stringFromAny(ai2ExecutionDetails["selector_version"]))
		scoringMode := strings.TrimSpace(stringFromAny(ai2ExecutionDetails["scoring_mode"]))
		candidateExplainability := mapFromAnyValue(ai2ExecutionDetails["candidate_explainability"])
		if len(weights) > 0 || len(riskFlags) > 0 || selectorVersion != "" || scoringMode != "" || len(candidateExplainability) > 0 {
			ai2ExecutionStatus = "done"
			if len(candidateExplainability) > 0 {
				ai2ExecutionSummary = "已记录 AI2 权重生效、Worker 风险策略，并输出候选命中解释。"
			} else {
				ai2ExecutionSummary = "已记录 AI2 权重生效与 Worker 风险策略执行结果。"
			}
		} else {
			ai2ExecutionStatus = "warn"
			ai2ExecutionSummary = "已进入执行观测阶段，但关键字段仍不完整。"
		}
	}
	appendStep(
		"ai2_execution",
		"AI2/Worker 执行观测",
		ai2ExecutionStatus,
		ai2ExecutionSummary,
		ai2ExecutionDetails,
	)

	finalOutputStatus := "done"
	userReply := mapFromAnyValue(output["user_reply"])
	ai2Instruction := mapFromAnyValue(output["ai2_instruction"])
	if len(userReply) == 0 || len(ai2Instruction) == 0 {
		finalOutputStatus = "warn"
	}
	appendStep(
		"final_output",
		"最终输出（用户反馈 + AI2指令）",
		finalOutputStatus,
		"已生成用户可读反馈与 AI2 可执行指令。",
		map[string]interface{}{
			"user_reply_keys":      sortedMapKeys(userReply),
			"ai2_instruction_keys": sortedMapKeys(ai2Instruction),
		},
	)
	return timeline
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

func boolFromAny(raw interface{}) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		v := strings.ToLower(strings.TrimSpace(value))
		return v == "1" || v == "true" || v == "yes" || v == "y" || v == "on"
	case int:
		return value != 0
	case int64:
		return value != 0
	case int32:
		return value != 0
	case uint:
		return value != 0
	case uint64:
		return value != 0
	case uint32:
		return value != 0
	case float64:
		return value != 0
	case float32:
		return value != 0
	default:
		return false
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
