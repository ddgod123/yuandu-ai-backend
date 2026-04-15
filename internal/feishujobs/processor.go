package feishujobs

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/feishu"
	"emoji/internal/models"
	"emoji/internal/queue"
	"emoji/internal/storage"
	"emoji/internal/videojobs"

	"github.com/hibiken/asynq"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultFeishuMessageMaxBytes       int64 = 100 * 1024 * 1024
	defaultFeishuBindCodeTTLMinutes          = 15
	defaultFeishuNotifyPollIntervalSec       = 20
	defaultFeishuNotifyPollMaxAttempts       = 180
)

var feishuBindCodeAlphabet = []byte("ABCDEFGHJKMNPQRSTUVWXYZ23456789")

var allowedVideoExt = map[string]struct{}{
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

type Processor struct {
	db     *gorm.DB
	qiniu  *storage.QiniuClient
	cfg    config.Config
	queue  *asynq.Client
	feishu *feishu.Client
}

func NewProcessor(db *gorm.DB, qiniu *storage.QiniuClient, cfg config.Config) *Processor {
	return &Processor{
		db:     db,
		qiniu:  qiniu,
		cfg:    cfg,
		queue:  queue.NewClient(cfg),
		feishu: feishu.NewClient(cfg),
	}
}

func (p *Processor) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskTypeIngestFeishuMessage, p.HandleIngestFeishuMessage)
	mux.HandleFunc(TaskTypeNotifyFeishuResult, p.HandleNotifyFeishuResult)
}

func (p *Processor) HandleIngestFeishuMessage(ctx context.Context, task *asynq.Task) error {
	var payload IngestFeishuMessagePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if payload.MessageJobID == 0 {
		return fmt.Errorf("invalid message_job_id")
	}

	var messageJob models.FeishuMessageJob
	if err := p.db.Where("id = ?", payload.MessageJobID).First(&messageJob).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	p.upsertVideoIngressFromFeishu(messageJob, map[string]interface{}{
		"status": mapFeishuMessageStatusToIngress(messageJob.Status),
	})

	if messageJob.VideoJobID != nil && strings.EqualFold(strings.TrimSpace(messageJob.Status), models.FeishuMessageJobStatusJobQueued) {
		_ = p.enqueueNotifyTask(messageJob.ID, 0, 0)
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(messageJob.Status), models.FeishuMessageJobStatusDone) {
		return nil
	}

	p.updateMessageJob(messageJob.ID, map[string]interface{}{
		"status":        models.FeishuMessageJobStatusProcessing,
		"error_message": "",
	})
	p.upsertVideoIngressFromFeishu(messageJob, map[string]interface{}{
		"status": models.VideoIngressStatusProcessing,
	})

	userID, err := p.resolveBoundUser(ctx, &messageJob)
	if err != nil {
		return p.handleIngestError(ctx, messageJob, err)
	}
	if userID == 0 {
		if err := p.markWaitingBind(ctx, messageJob); err != nil {
			return p.handleIngestError(ctx, messageJob, err)
		}
		return nil
	}

	sourceKey, sourceSize, fileName, err := p.fetchAndUploadSourceVideo(ctx, messageJob, userID)
	if err != nil {
		return p.handleIngestError(ctx, messageJob, err)
	}

	videoJob, err := p.createVideoJobForMessage(messageJob, userID, sourceKey, sourceSize, fileName)
	if err != nil {
		var insufficient videojobs.InsufficientPointsError
		if errors.As(err, &insufficient) {
			p.updateMessageJob(messageJob.ID, map[string]interface{}{
				"status":        models.FeishuMessageJobStatusFailed,
				"error_message": err.Error(),
				"finished_at":   time.Now(),
			})
			p.upsertVideoIngressFromFeishu(messageJob, map[string]interface{}{
				"status":        models.VideoIngressStatusFailed,
				"error_message": err.Error(),
				"finished_at":   time.Now(),
			})
			p.sendTextBestEffort(ctx, messageJob.ChatID,
				fmt.Sprintf("余额不足，无法创建任务。需要 %d 点，当前可用 %d 点。", insufficient.Required, insufficient.Available),
			)
			return nil
		}
		return p.handleIngestError(ctx, messageJob, err)
	}

	result := map[string]interface{}{
		"video_job_id":        videoJob.ID,
		"source_video_key":    sourceKey,
		"source_video_size":   sourceSize,
		"source_message_id":   messageJob.MessageID,
		"source_message_type": messageJob.MessageType,
	}

	p.updateMessageJob(messageJob.ID, map[string]interface{}{
		"user_id":         userID,
		"video_job_id":    videoJob.ID,
		"status":          models.FeishuMessageJobStatusJobQueued,
		"error_message":   "",
		"bind_code":       "",
		"result_payload":  mustJSON(result),
		"notify_attempts": 0,
	})

	p.sendTextBestEffort(ctx, messageJob.ChatID, fmt.Sprintf("已收到视频，任务 #%d 已创建，开始处理。", videoJob.ID))
	_ = p.enqueueNotifyTask(messageJob.ID, 0, 0)
	return nil
}

func (p *Processor) HandleNotifyFeishuResult(ctx context.Context, task *asynq.Task) error {
	var payload NotifyFeishuResultPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if payload.MessageJobID == 0 {
		return nil
	}

	var messageJob models.FeishuMessageJob
	if err := p.db.Where("id = ?", payload.MessageJobID).First(&messageJob).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if messageJob.VideoJobID == nil || *messageJob.VideoJobID == 0 {
		return nil
	}
	if messageJob.NotifiedAt != nil && (messageJob.Status == models.FeishuMessageJobStatusDone || messageJob.Status == models.FeishuMessageJobStatusFailed) {
		return nil
	}

	var job models.VideoJob
	if err := p.db.Select("id", "title", "status", "stage", "error_message", "result_collection_id", "updated_at").Where("id = ?", *messageJob.VideoJobID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	status := strings.ToLower(strings.TrimSpace(job.Status))
	switch status {
	case models.VideoJobStatusDone:
		msg := p.buildDoneMessage(job)
		if err := p.feishu.SendTextMessageToChat(ctx, messageJob.ChatID, msg); err != nil {
			return err
		}
		now := time.Now()
		p.updateMessageJob(messageJob.ID, map[string]interface{}{
			"status":          models.FeishuMessageJobStatusDone,
			"error_message":   "",
			"finished_at":     now,
			"notified_at":     now,
			"notify_attempts": payload.Attempt,
		})
		p.upsertVideoIngressFromFeishu(messageJob, map[string]interface{}{
			"video_job_id":  job.ID,
			"status":        models.VideoIngressStatusDone,
			"error_message": "",
			"finished_at":   now,
		})
		return nil
	case models.VideoJobStatusFailed, models.VideoJobStatusCancelled:
		errMsg := strings.TrimSpace(job.ErrorMessage)
		if errMsg == "" {
			errMsg = "任务处理失败"
		}
		if err := p.feishu.SendTextMessageToChat(ctx, messageJob.ChatID, "任务处理失败："+errMsg); err != nil {
			return err
		}
		now := time.Now()
		p.updateMessageJob(messageJob.ID, map[string]interface{}{
			"status":          models.FeishuMessageJobStatusFailed,
			"error_message":   errMsg,
			"finished_at":     now,
			"notified_at":     now,
			"notify_attempts": payload.Attempt,
		})
		p.upsertVideoIngressFromFeishu(messageJob, map[string]interface{}{
			"video_job_id":  job.ID,
			"status":        models.VideoIngressStatusFailed,
			"error_message": errMsg,
			"finished_at":   now,
		})
		return nil
	default:
		nextAttempt := payload.Attempt + 1
		maxAttempts := p.cfg.FeishuNotifyPollingMaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = defaultFeishuNotifyPollMaxAttempts
		}
		if nextAttempt > maxAttempts {
			if err := p.feishu.SendTextMessageToChat(ctx, messageJob.ChatID, fmt.Sprintf("任务 #%d 仍在处理中，请稍后在 Web 端查看进度。", job.ID)); err != nil {
				return err
			}
			now := time.Now()
			p.updateMessageJob(messageJob.ID, map[string]interface{}{
				"status":          models.FeishuMessageJobStatusFailed,
				"error_message":   "notify timeout",
				"finished_at":     now,
				"notified_at":     now,
				"notify_attempts": nextAttempt,
			})
			return nil
		}

		p.updateMessageJob(messageJob.ID, map[string]interface{}{
			"status":          models.FeishuMessageJobStatusJobQueued,
			"notify_attempts": nextAttempt,
		})
		interval := p.cfg.FeishuNotifyPollingIntervalSec
		if interval <= 0 {
			interval = defaultFeishuNotifyPollIntervalSec
		}
		if interval < 5 {
			interval = 5
		}
		return p.enqueueNotifyTask(messageJob.ID, nextAttempt, time.Duration(interval)*time.Second)
	}
}

func (p *Processor) resolveBoundUser(ctx context.Context, messageJob *models.FeishuMessageJob) (uint64, error) {
	if messageJob == nil {
		return 0, fmt.Errorf("message job is nil")
	}
	if messageJob.UserID != nil && *messageJob.UserID > 0 {
		return *messageJob.UserID, nil
	}

	var account models.ExternalAccount
	err := p.db.Where("provider = ? AND tenant_key = ? AND open_id = ? AND status = ?",
		models.ExternalAccountProviderFeishu,
		messageJob.TenantKey,
		messageJob.OpenID,
		models.ExternalAccountStatusActive,
	).First(&account).Error
	if err == nil {
		return account.UserID, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	if strings.TrimSpace(messageJob.UnionID) == "" {
		return 0, nil
	}

	err = p.db.Where("provider = ? AND union_id = ? AND status = ?",
		models.ExternalAccountProviderFeishu,
		messageJob.UnionID,
		models.ExternalAccountStatusActive,
	).Order("id DESC").First(&account).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}

	// 自动补齐 tenant+open 映射，便于后续查找。
	_ = p.db.Create(&models.ExternalAccount{
		Provider:  models.ExternalAccountProviderFeishu,
		TenantKey: strings.TrimSpace(messageJob.TenantKey),
		OpenID:    strings.TrimSpace(messageJob.OpenID),
		UnionID:   strings.TrimSpace(messageJob.UnionID),
		UserID:    account.UserID,
		Status:    models.ExternalAccountStatusActive,
		Metadata:  mustJSON(map[string]interface{}{"source": "union_id_backfill"}),
	}).Error

	return account.UserID, nil
}

func (p *Processor) markWaitingBind(ctx context.Context, messageJob models.FeishuMessageJob) error {
	codeRow, err := p.ensureActiveBindCode(messageJob)
	if err != nil {
		return err
	}

	p.updateMessageJob(messageJob.ID, map[string]interface{}{
		"status":        models.FeishuMessageJobStatusWaitingBind,
		"bind_code":     codeRow.Code,
		"error_message": "feishu account not bound",
	})
	p.upsertVideoIngressFromFeishu(messageJob, map[string]interface{}{
		"status":        models.VideoIngressStatusWaitingBind,
		"error_message": "feishu account not bound",
	})

	text := "当前飞书账号尚未绑定平台账号。\n请前往网站完成绑定，并输入绑定码：" + codeRow.Code
	if portal := strings.TrimSpace(p.cfg.FeishuBindPortalURL); portal != "" {
		text += "\n绑定入口：" + portal
	}
	text += "\n绑定后你可直接重发视频，或等待系统自动重试。"
	p.sendTextBestEffort(ctx, messageJob.ChatID, text)
	return nil
}

func (p *Processor) ensureActiveBindCode(messageJob models.FeishuMessageJob) (models.FeishuBindCode, error) {
	now := time.Now()
	_ = p.db.Model(&models.FeishuBindCode{}).
		Where("status = ? AND expires_at < ?", models.FeishuBindCodeStatusActive, now).
		Updates(map[string]interface{}{
			"status":     models.FeishuBindCodeStatusExpired,
			"updated_at": now,
		}).Error

	var existing models.FeishuBindCode
	err := p.db.Where("tenant_key = ? AND open_id = ? AND status = ? AND expires_at > ?",
		messageJob.TenantKey,
		messageJob.OpenID,
		models.FeishuBindCodeStatusActive,
		now,
	).Order("id DESC").First(&existing).Error
	if err == nil {
		return existing, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.FeishuBindCode{}, err
	}

	ttlMin := p.cfg.FeishuBindCodeTTLMinutes
	if ttlMin <= 0 {
		ttlMin = defaultFeishuBindCodeTTLMinutes
	}
	expiresAt := now.Add(time.Duration(ttlMin) * time.Minute)

	var lastErr error
	for i := 0; i < 8; i++ {
		code, genErr := generateBindCode(8)
		if genErr != nil {
			return models.FeishuBindCode{}, genErr
		}
		row := models.FeishuBindCode{
			Code:      code,
			TenantKey: strings.TrimSpace(messageJob.TenantKey),
			ChatID:    strings.TrimSpace(messageJob.ChatID),
			OpenID:    strings.TrimSpace(messageJob.OpenID),
			UnionID:   strings.TrimSpace(messageJob.UnionID),
			Status:    models.FeishuBindCodeStatusActive,
			ExpiresAt: expiresAt,
		}
		if createErr := p.db.Create(&row).Error; createErr != nil {
			lastErr = createErr
			if isDuplicateError(createErr) {
				continue
			}
			return models.FeishuBindCode{}, createErr
		}
		return row, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("failed to allocate bind code")
	}
	return models.FeishuBindCode{}, lastErr
}

func (p *Processor) fetchAndUploadSourceVideo(ctx context.Context, messageJob models.FeishuMessageJob, userID uint64) (string, int64, string, error) {
	if p.qiniu == nil {
		return "", 0, "", fmt.Errorf("qiniu not configured")
	}
	if p.feishu == nil || !p.feishu.Enabled() {
		return "", 0, "", fmt.Errorf("feishu app credentials not configured")
	}

	resp, _, err := p.feishu.DownloadMessageResource(ctx, messageJob.MessageID, messageJob.FileKey, []string{messageJob.MessageType})
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	fileName := resolveMessageFileName(messageJob.FileName, resp.Header.Get("Content-Disposition"), contentType)
	if fileName == "" {
		fileName = "source.mp4"
	}
	ext := normalizeVideoExt(fileName, contentType)

	maxBytes := p.cfg.FeishuBotMessageMaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultFeishuMessageMaxBytes
	}

	tmpFile, err := os.CreateTemp("", "feishu-video-*.tmp")
	if err != nil {
		return "", 0, "", err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	defer tmpFile.Close()

	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", 0, "", err
	}
	if written > maxBytes {
		return "", 0, "", fmt.Errorf("source video exceeds max bytes (%d)", maxBytes)
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return "", 0, "", err
	}

	sourceKey := p.buildSourceVideoKey(userID, messageJob.MessageID, ext)
	if err := p.uploadFileToQiniu(ctx, sourceKey, tmpFile, written, contentType); err != nil {
		return "", 0, "", err
	}
	return sourceKey, written, fileName, nil
}

func (p *Processor) createVideoJobForMessage(
	messageJob models.FeishuMessageJob,
	userID uint64,
	sourceKey string,
	sourceSize int64,
	sourceName string,
) (models.VideoJob, error) {
	if p.queue == nil {
		return models.VideoJob{}, fmt.Errorf("queue not configured")
	}
	format := normalizeOutputFormat(p.cfg.FeishuBotDefaultOutputFormat)
	formats := []string{format}

	title := buildJobTitle(sourceName, messageJob.MessageID)
	priority := "normal"
	options := map[string]interface{}{
		"flow_mode":               "direct",
		"auto_highlight":          true,
		"entry_channel":           "feishu_bot",
		"entry_provider":          "feishu",
		"feishu_message_job_id":   messageJob.ID,
		"feishu_message_id":       messageJob.MessageID,
		"feishu_chat_id":          messageJob.ChatID,
		"feishu_open_id":          messageJob.OpenID,
		"source_video_size_bytes": sourceSize,
	}
	queueName, taskType, primaryFormat := videojobs.ResolveVideoJobExecutionTarget(strings.Join(formats, ","))
	options["execution_queue"] = queueName
	options["execution_task_type"] = taskType
	if primaryFormat != "" {
		options["requested_format"] = primaryFormat
		if primaryFormat == "png" {
			options["png_mode"] = videojobs.PNGModeSmartLLM
		}
	}
	estimatedPoints := videojobs.EstimateReservationPoints(sourceSize, formats, options)
	options["estimate_points"] = estimatedPoints

	job := models.VideoJob{
		UserID:         userID,
		Title:          title,
		SourceVideoKey: sourceKey,
		OutputFormats:  strings.Join(formats, ","),
		Status:         models.VideoJobStatusQueued,
		Stage:          models.VideoJobStageQueued,
		Progress:       0,
		Priority:       priority,
		Options:        mustJSON(options),
		Metrics:        datatypes.JSON([]byte("{}")),
		AssetDomain:    models.VideoJobAssetDomainVideo,
	}

	err := p.db.Transaction(func(tx *gorm.DB) error {
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
		return videojobs.ReservePointsForJob(tx, userID, job.ID, estimatedPoints, "feishu video job reserve", map[string]interface{}{
			"source_video_key":  sourceKey,
			"output_formats":    formats,
			"source_size_bytes": sourceSize,
			"entry_channel":     "feishu_bot",
		})
	})
	if err != nil {
		return models.VideoJob{}, err
	}

	task, queueName, _, err := videojobs.NewProcessVideoJobTaskByFormat(job.ID, job.OutputFormats)
	if err != nil {
		failUpdates := map[string]interface{}{
			"status":        models.VideoJobStatusFailed,
			"stage":         models.VideoJobStageFailed,
			"error_message": err.Error(),
			"finished_at":   time.Now(),
		}
		_ = p.db.Model(&models.VideoJob{}).Where("id = ?", job.ID).Updates(failUpdates).Error
		_ = videojobs.SyncPublicVideoImageJobUpdates(p.db, job.ID, failUpdates)
		_ = videojobs.ReleaseReservedPointsForJob(p.db, job.ID, "task_create_failed")
		return models.VideoJob{}, err
	}

	_, err = p.queue.Enqueue(
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
		_ = p.db.Model(&models.VideoJob{}).Where("id = ?", job.ID).Updates(failUpdates).Error
		_ = videojobs.SyncPublicVideoImageJobUpdates(p.db, job.ID, failUpdates)
		_ = p.db.Create(&models.VideoJobEvent{
			JobID:    job.ID,
			Stage:    models.VideoJobStageFailed,
			Level:    "error",
			Message:  "enqueue failed: " + err.Error(),
			Metadata: datatypes.JSON([]byte("{}")),
		}).Error
		_ = videojobs.ReleaseReservedPointsForJob(p.db, job.ID, "enqueue_failed")
		return models.VideoJob{}, err
	}
	_ = videojobs.EnsureVideoJobAIReadingQueued(p.db, job.ID, userID)
	if analysisTask, analysisErr := videojobs.NewAnalyzeVideoTextTask(job.ID); analysisErr == nil {
		_, _ = p.queue.Enqueue(
			analysisTask,
			asynq.Queue(videojobs.QueueVideoJobAI),
			asynq.MaxRetry(3),
			asynq.Timeout(20*time.Minute),
			asynq.Retention(7*24*time.Hour),
			asynq.TaskID(fmt.Sprintf("video-ai-reading-%d", job.ID)),
		)
	}
	p.upsertVideoIngressFromFeishu(messageJob, map[string]interface{}{
		"bound_user_id":     userID,
		"source_video_key":  sourceKey,
		"source_size_bytes": sourceSize,
		"source_file_name":  sourceName,
		"video_job_id":      job.ID,
		"status":            models.VideoIngressStatusJobQueued,
		"result_payload": mustJSON(map[string]interface{}{
			"video_job_id":     job.ID,
			"source_video_key": sourceKey,
		}),
	})

	return job, nil
}

func (p *Processor) uploadFileToQiniu(ctx context.Context, key string, file *os.File, size int64, contentType string) error {
	if p.qiniu == nil {
		return fmt.Errorf("qiniu not configured")
	}
	if file == nil {
		return fmt.Errorf("file is nil")
	}
	if size < 0 {
		size = 0
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	uploader := qiniustorage.NewFormUploader(p.qiniu.Cfg)
	putPolicy := qiniustorage.PutPolicy{Scope: p.qiniu.Bucket + ":" + key}
	upToken := putPolicy.UploadToken(p.qiniu.Mac)
	putExtra := qiniustorage.PutExtra{}
	if strings.TrimSpace(contentType) != "" {
		putExtra.MimeType = strings.TrimSpace(contentType)
	}
	var putRet qiniustorage.PutRet
	return uploader.Put(ctx, &putRet, upToken, key, file, size, &putExtra)
}

func (p *Processor) buildDoneMessage(job models.VideoJob) string {
	lines := []string{fmt.Sprintf("任务 #%d 已完成。", job.ID)}
	if strings.TrimSpace(job.Title) != "" {
		lines = append(lines, "标题："+strings.TrimSpace(job.Title))
	}
	if summary := p.resolveAIReadingSummary(job.ID); summary != "" {
		lines = append(lines, "视频解析："+summary)
	}
	if pageURL := buildResultPageURL(p.cfg.FeishuBotResultPageBaseURL, job.ID); pageURL != "" {
		lines = append(lines, "结果页："+pageURL)
	}
	if zipURL := p.resolveZipURL(job); zipURL != "" {
		lines = append(lines, "下载链接："+zipURL)
	}
	return strings.Join(lines, "\n")
}

func (p *Processor) resolveAIReadingSummary(jobID uint64) string {
	if p == nil || p.db == nil || jobID == 0 {
		return ""
	}
	var row models.VideoJobAIReading
	if err := p.db.Select("summary_text", "status").Where("job_id = ?", jobID).First(&row).Error; err != nil {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(row.Status), models.VideoJobAIReadingStatusDone) {
		return ""
	}
	return strings.TrimSpace(truncateText(row.SummaryText, 120))
}

func (p *Processor) resolveZipURL(job models.VideoJob) string {
	if p.qiniu == nil || job.ResultCollectionID == nil || *job.ResultCollectionID == 0 {
		return ""
	}
	var collection models.Collection
	if err := p.db.Select("id", "latest_zip_key").Where("id = ?", *job.ResultCollectionID).First(&collection).Error; err != nil {
		return ""
	}
	zipKey := strings.TrimSpace(collection.LatestZipKey)
	if zipKey == "" {
		return ""
	}
	if p.qiniu.Private {
		url, err := p.qiniu.SignedURL(zipKey, int64(p.qiniu.SignTTL))
		if err != nil {
			return ""
		}
		return url
	}
	return p.qiniu.PublicURL(zipKey)
}

func (p *Processor) enqueueNotifyTask(messageJobID uint64, attempt int, delay time.Duration) error {
	if p.queue == nil || messageJobID == 0 {
		return nil
	}
	task, err := NewNotifyFeishuResultTask(messageJobID, attempt)
	if err != nil {
		return err
	}
	options := []asynq.Option{
		asynq.Queue("default"),
		asynq.MaxRetry(6),
		asynq.Timeout(2 * time.Minute),
		asynq.TaskID(fmt.Sprintf("feishu-notify-%d-%d", messageJobID, attempt)),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = p.queue.Enqueue(task, options...)
	if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	return nil
}

func (p *Processor) handleIngestError(ctx context.Context, messageJob models.FeishuMessageJob, ingestErr error) error {
	retryCount, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	if retryCount >= maxRetry {
		now := time.Now()
		p.updateMessageJob(messageJob.ID, map[string]interface{}{
			"status":        models.FeishuMessageJobStatusFailed,
			"error_message": trimErr(ingestErr),
			"finished_at":   now,
		})
		p.upsertVideoIngressFromFeishu(messageJob, map[string]interface{}{
			"status":        models.VideoIngressStatusFailed,
			"error_message": trimErr(ingestErr),
			"finished_at":   now,
		})
		p.sendTextBestEffort(ctx, messageJob.ChatID, "视频处理请求失败，请稍后重试。")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, ingestErr)
	}

	p.updateMessageJob(messageJob.ID, map[string]interface{}{
		"status":        models.FeishuMessageJobStatusRetrying,
		"error_message": trimErr(ingestErr),
	})
	p.upsertVideoIngressFromFeishu(messageJob, map[string]interface{}{
		"status":        models.VideoIngressStatusProcessing,
		"error_message": trimErr(ingestErr),
	})
	return ingestErr
}

func (p *Processor) sendTextBestEffort(ctx context.Context, chatID, text string) {
	if p.feishu == nil || !p.feishu.Enabled() {
		return
	}
	if err := p.feishu.SendTextMessageToChat(ctx, chatID, text); err != nil {
		log.Printf("feishu send message failed (chat=%s): %v", strings.TrimSpace(chatID), err)
	}
}

func (p *Processor) updateMessageJob(messageJobID uint64, updates map[string]interface{}) {
	if messageJobID == 0 || len(updates) == 0 {
		return
	}
	updates["updated_at"] = time.Now()
	_ = p.db.Model(&models.FeishuMessageJob{}).Where("id = ?", messageJobID).Updates(updates).Error
}

func (p *Processor) upsertVideoIngressFromFeishu(messageJob models.FeishuMessageJob, updates map[string]interface{}) {
	if p == nil || p.db == nil || messageJob.ID == 0 {
		return
	}
	now := time.Now()
	status := mapFeishuMessageStatusToIngress(messageJob.Status)
	if text := strings.TrimSpace(fmt.Sprint(updates["status"])); text != "" && text != "<nil>" {
		status = strings.ToLower(strings.TrimSpace(text))
	}

	row := models.VideoIngressJob{
		Provider:          models.VideoIngressProviderFeishu,
		TenantKey:         strings.TrimSpace(messageJob.TenantKey),
		Channel:           "feishu_bot",
		ChatID:            strings.TrimSpace(messageJob.ChatID),
		ExternalUserID:    strings.TrimSpace(messageJob.OpenID),
		ExternalOpenID:    strings.TrimSpace(messageJob.OpenID),
		ExternalUnionID:   strings.TrimSpace(messageJob.UnionID),
		SourceMessageID:   strings.TrimSpace(messageJob.MessageID),
		SourceResourceKey: strings.TrimSpace(messageJob.FileKey),
		SourceFileName:    strings.TrimSpace(messageJob.FileName),
		Status:            status,
		RequestPayload:    messageJob.RequestPayload,
	}
	if messageJob.UserID != nil && *messageJob.UserID > 0 {
		row.BoundUserID = messageJob.UserID
	}
	if messageJob.VideoJobID != nil && *messageJob.VideoJobID > 0 {
		row.VideoJobID = messageJob.VideoJobID
	}

	assignments := map[string]interface{}{
		"channel":           row.Channel,
		"chat_id":           row.ChatID,
		"external_user_id":  row.ExternalUserID,
		"external_open_id":  row.ExternalOpenID,
		"external_union_id": row.ExternalUnionID,
		"source_file_name":  row.SourceFileName,
		"request_payload":   row.RequestPayload,
		"status":            row.Status,
		"updated_at":        now,
	}
	if row.BoundUserID != nil && *row.BoundUserID > 0 {
		assignments["bound_user_id"] = *row.BoundUserID
	}
	if row.VideoJobID != nil && *row.VideoJobID > 0 {
		assignments["video_job_id"] = *row.VideoJobID
	}
	for key, value := range updates {
		k := strings.ToLower(strings.TrimSpace(key))
		if k == "" {
			continue
		}
		assignments[k] = value
	}

	err := p.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "provider"},
			{Name: "tenant_key"},
			{Name: "source_message_id"},
			{Name: "source_resource_key"},
		},
		DoUpdates: clause.Assignments(assignments),
	}).Create(&row).Error
	if err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "video_ingress_jobs") && strings.Contains(msg, "does not exist") {
			return
		}
	}
}

func mapFeishuMessageStatusToIngress(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case models.FeishuMessageJobStatusWaitingBind:
		return models.VideoIngressStatusWaitingBind
	case models.FeishuMessageJobStatusDone:
		return models.VideoIngressStatusDone
	case models.FeishuMessageJobStatusFailed:
		return models.VideoIngressStatusFailed
	case models.FeishuMessageJobStatusJobQueued:
		return models.VideoIngressStatusJobQueued
	case models.FeishuMessageJobStatusProcessing, models.FeishuMessageJobStatusRetrying:
		return models.VideoIngressStatusProcessing
	default:
		return models.VideoIngressStatusQueued
	}
}

func (p *Processor) buildSourceVideoKey(userID uint64, messageID string, ext string) string {
	root := strings.TrimSuffix(storage.NormalizeRootPrefix(p.cfg.QiniuRootPrefix), "/")
	datePart := time.Now().UTC().Format("20060102")
	name := sanitizeObjectName(messageID)
	if name == "" {
		name = strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	fileName := fmt.Sprintf("%s_%d%s", name, time.Now().Unix(), ext)
	return path.Join(root, "user-video", strconv.FormatUint(userID, 10), "feishu", datePart, fileName)
}

func normalizeOutputFormat(raw string) string {
	format := strings.ToLower(strings.TrimSpace(raw))
	switch format {
	case "gif", "png", "jpg", "webp", "mp4", "live":
		return format
	case "jpeg":
		return "jpg"
	default:
		return "gif"
	}
}

func buildJobTitle(sourceName, messageID string) string {
	title := strings.TrimSpace(sourceName)
	if title != "" {
		ext := filepath.Ext(title)
		title = strings.TrimSpace(strings.TrimSuffix(title, ext))
	}
	if title == "" {
		title = "飞书视频任务-" + sanitizeObjectName(messageID)
	}
	if title == "" {
		title = "飞书视频任务"
	}
	if len(title) > 100 {
		title = title[:100]
	}
	return title
}

func resolveMessageFileName(preferred, contentDisposition, contentType string) string {
	candidate := strings.TrimSpace(preferred)
	if candidate != "" {
		return candidate
	}

	if strings.TrimSpace(contentDisposition) != "" {
		_, params, err := mime.ParseMediaType(contentDisposition)
		if err == nil {
			if name := strings.TrimSpace(params["filename"]); name != "" {
				return name
			}
		}
	}

	if strings.TrimSpace(contentType) != "" {
		extList, err := mime.ExtensionsByType(contentType)
		if err == nil {
			for _, ext := range extList {
				ext = strings.ToLower(strings.TrimSpace(ext))
				if _, ok := allowedVideoExt[ext]; ok {
					return "source" + ext
				}
			}
		}
	}

	return "source.mp4"
}

func normalizeVideoExt(fileName, contentType string) string {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(fileName)))
	if _, ok := allowedVideoExt[ext]; ok {
		return ext
	}
	if strings.TrimSpace(contentType) != "" {
		extList, err := mime.ExtensionsByType(contentType)
		if err == nil {
			for _, item := range extList {
				item = strings.ToLower(strings.TrimSpace(item))
				if _, ok := allowedVideoExt[item]; ok {
					return item
				}
			}
		}
	}
	return ".mp4"
}

func sanitizeObjectName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	value := strings.Trim(b.String(), "_")
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func generateBindCode(length int) (string, error) {
	if length <= 0 {
		length = 8
	}
	buf := make([]byte, length)
	randBytes := make([]byte, length)
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}
	for i := 0; i < length; i++ {
		buf[i] = feishuBindCodeAlphabet[int(randBytes[i])%len(feishuBindCodeAlphabet)]
	}
	return string(buf), nil
}

func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")
}

func trimErr(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 1200 {
		return msg[:1200]
	}
	return msg
}

func mustJSON(v interface{}) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte("{}"))
	}
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(b)
}

func truncateText(raw string, maxLen int) string {
	text := strings.TrimSpace(raw)
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return strings.TrimSpace(string(runes[:maxLen])) + "…"
}

func buildResultPageURL(base string, jobID uint64) string {
	base = strings.TrimSpace(base)
	if base == "" || jobID == 0 {
		return ""
	}
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	return base + separator + "job_id=" + strconv.FormatUint(jobID, 10)
}
