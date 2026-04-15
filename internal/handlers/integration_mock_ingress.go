package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CreateMockIngressVideoJobRequest struct {
	Title          string `json:"title" example:"QQ 测试视频任务"`
	SourceVideoKey string `json:"source_video_key" example:"video/jobs/demo/source.mp4"`
	OutputFormat   string `json:"output_format,omitempty" example:"png" enums:"png,gif,jpg"`
	PNGMode        string `json:"png_mode,omitempty" example:"smart_llm" enums:"smart_llm,fast_extract"`
	FastExtractFPS int    `json:"fast_extract_fps,omitempty" example:"1" enums:"1,2"`
	TenantKey      string `json:"tenant_key,omitempty" example:"corp_demo"`
	ChatID         string `json:"chat_id,omitempty" example:"chat_123456"`
	SessionID      string `json:"session_id,omitempty" example:"session_123456"`
	ExternalUserID string `json:"external_user_id,omitempty" example:"openid_123456"`
}

type CreateMockIngressVideoJobResponse struct {
	Provider  string           `json:"provider" example:"qq"`
	Channel   string           `json:"channel" example:"qq_bot"`
	IngressID uint64           `json:"ingress_id" example:"1001"`
	Job       VideoJobResponse `json:"job"`
}

// CreateMockQQVideoJob godoc
// @Summary Create mock QQ ingress video job
// @Tags user
// @Accept json
// @Produce json
// @Param body body CreateMockIngressVideoJobRequest true "create mock ingress request"
// @Success 200 {object} CreateMockIngressVideoJobResponse
// @Router /api/integrations/mock/qq/video-jobs [post]
func (h *Handler) CreateMockQQVideoJob(c *gin.Context) {
	h.createMockIngressVideoJob(c, models.VideoIngressProviderQQ, "qq_bot")
}

// CreateMockWeComVideoJob godoc
// @Summary Create mock WeCom ingress video job
// @Tags user
// @Accept json
// @Produce json
// @Param body body CreateMockIngressVideoJobRequest true "create mock ingress request"
// @Success 200 {object} CreateMockIngressVideoJobResponse
// @Router /api/integrations/mock/wecom/video-jobs [post]
func (h *Handler) CreateMockWeComVideoJob(c *gin.Context) {
	h.createMockIngressVideoJob(c, models.VideoIngressProviderWeCom, "wecom_bot")
}

func (h *Handler) createMockIngressVideoJob(c *gin.Context, provider, channel string) {
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

	var req CreateMockIngressVideoJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sourceKey, err := h.normalizeSourceVideoKey(req.SourceVideoKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	format := strings.ToLower(strings.TrimSpace(req.OutputFormat))
	if format == "" {
		format = "png"
	}
	if format == "jpeg" {
		format = "jpg"
	}
	queueName, taskType, primaryFormat := videojobs.ResolveVideoJobExecutionTarget(format)
	if primaryFormat == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported output format"})
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = fmt.Sprintf("%s-视频任务-%s", strings.ToUpper(provider), time.Now().Format("20060102150405"))
	}
	options := map[string]interface{}{
		"flow_mode":               "direct",
		"auto_highlight":          true,
		"entry_channel":           channel,
		"entry_provider":          provider,
		"execution_queue":         queueName,
		"execution_task_type":     taskType,
		"requested_format":        primaryFormat,
		"source_video_size_bytes": h.lookupSourceVideoSizeBytes(sourceKey),
	}
	if primaryFormat == "png" {
		pngMode := videojobs.NormalizePNGMode(req.PNGMode)
		options["png_mode"] = pngMode
		if pngMode == videojobs.PNGModeFastExtract {
			fastFPS := videojobs.NormalizePNGFastExtractFPS(req.FastExtractFPS)
			options["fast_extract_fps"] = fastFPS
			options["frame_interval_sec"] = 1.0 / float64(fastFPS)
		}
	}
	estimatedPoints := videojobs.EstimateReservationPoints(h.lookupSourceVideoSizeBytes(sourceKey), []string{primaryFormat}, options)
	options["estimate_points"] = estimatedPoints

	job := models.VideoJob{
		UserID:         userID,
		Title:          title,
		SourceVideoKey: sourceKey,
		OutputFormats:  primaryFormat,
		Status:         models.VideoJobStatusQueued,
		Stage:          models.VideoJobStageQueued,
		Progress:       0,
		Priority:       "normal",
		Options:        toJSON(options),
		Metrics:        datatypes.JSON([]byte("{}")),
		AssetDomain:    models.VideoJobAssetDomainVideo,
	}

	var ingress models.VideoIngressJob
	sourceSize := h.lookupSourceVideoSizeBytes(sourceKey)
	err = h.db.Transaction(func(tx *gorm.DB) error {
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
		if err := tx.Create(&queuedEvent).Error; err != nil {
			return err
		}
		if err := videojobs.CreatePublicVideoImageEvent(tx, queuedEvent); err != nil {
			return err
		}

		boundUserID := userID
		videoJobID := job.ID
		ingress = models.VideoIngressJob{
			Provider:          provider,
			TenantKey:         strings.TrimSpace(req.TenantKey),
			Channel:           channel,
			ChatID:            strings.TrimSpace(req.ChatID),
			SessionID:         strings.TrimSpace(req.SessionID),
			ExternalUserID:    strings.TrimSpace(req.ExternalUserID),
			BoundUserID:       &boundUserID,
			SourceMessageID:   fmt.Sprintf("video_job_%d", job.ID),
			SourceResourceKey: sourceKey,
			SourceVideoKey:    sourceKey,
			SourceSizeBytes:   sourceSize,
			VideoJobID:        &videoJobID,
			Status:            models.VideoIngressStatusJobQueued,
			RequestPayload: toJSON(map[string]interface{}{
				"provider":         provider,
				"channel":          channel,
				"output_format":    primaryFormat,
				"png_mode":         options["png_mode"],
				"fast_extract_fps": options["fast_extract_fps"],
			}),
			ResultPayload: toJSON(map[string]interface{}{
				"video_job_id": job.ID,
			}),
		}
		if err := tx.Create(&ingress).Error; err != nil {
			return err
		}
		if extUserID := strings.TrimSpace(req.ExternalUserID); extUserID != "" {
			account := models.ExternalAccount{
				Provider:  provider,
				TenantKey: strings.TrimSpace(req.TenantKey),
				OpenID:    extUserID,
				UserID:    userID,
				Status:    models.ExternalAccountStatusActive,
				Metadata: toJSON(map[string]interface{}{
					"source":  "mock_ingress",
					"channel": channel,
				}),
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "provider"}, {Name: "tenant_key"}, {Name: "open_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"user_id":    userID,
					"status":     models.ExternalAccountStatusActive,
					"updated_at": time.Now(),
				}),
			}).Create(&account).Error; err != nil {
				return err
			}
		}
		return videojobs.ReservePointsForJob(tx, userID, job.ID, estimatedPoints, provider+" mock ingress reserve", map[string]interface{}{
			"source_video_key": sourceKey,
			"output_format":    primaryFormat,
			"entry_channel":    channel,
		})
	})
	if err != nil {
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
		_ = h.db.Model(&models.VideoJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
			"status":        models.VideoJobStatusFailed,
			"stage":         models.VideoJobStageFailed,
			"error_message": err.Error(),
			"finished_at":   time.Now(),
		}).Error
		_ = videojobs.ReleaseReservedPointsForJob(h.db, job.ID, "mock_task_create_failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}
	if _, err := h.queue.Enqueue(
		task,
		asynq.Queue(queueName),
		asynq.MaxRetry(6),
		asynq.Timeout(2*time.Hour),
		asynq.Retention(7*24*time.Hour),
		asynq.TaskID(fmt.Sprintf("video-job-%d", job.ID)),
	); err != nil {
		_ = h.db.Model(&models.VideoJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
			"status":        models.VideoJobStatusFailed,
			"stage":         models.VideoJobStageFailed,
			"error_message": err.Error(),
			"finished_at":   time.Now(),
		}).Error
		_ = videojobs.ReleaseReservedPointsForJob(h.db, job.ID, "mock_enqueue_failed")
		_ = h.db.Model(&models.VideoIngressJob{}).Where("id = ?", ingress.ID).Updates(map[string]interface{}{
			"status":        models.VideoIngressStatusFailed,
			"error_message": err.Error(),
			"updated_at":    time.Now(),
		}).Error
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue job"})
		return
	}

	h.enqueueVideoAIReadingBestEffort(job.ID, userID)
	c.JSON(http.StatusOK, CreateMockIngressVideoJobResponse{
		Provider:  provider,
		Channel:   channel,
		IngressID: ingress.ID,
		Job:       buildVideoJobResponse(job, h.qiniu),
	})
}
