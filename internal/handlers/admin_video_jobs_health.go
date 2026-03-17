package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AdminVideoJobHealthCheckItem struct {
	Code    string                 `json:"code"`
	Status  string                 `json:"status"`
	Message string                 `json:"message"`
	Detail  map[string]interface{} `json:"detail,omitempty"`
}

type AdminVideoJobHealthSummary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Warned int `json:"warned"`
	Failed int `json:"failed"`
}

type AdminVideoJobHealthStats struct {
	EventsSource         string         `json:"events_source"`
	OutputsSource        string         `json:"outputs_source"`
	PublicEventCount     int            `json:"public_event_count"`
	LegacyEventCount     int            `json:"legacy_event_count"`
	PublicOutputCount    int            `json:"public_output_count"`
	LegacyArtifactCount  int            `json:"legacy_artifact_count"`
	EffectiveEventCount  int            `json:"effective_event_count"`
	EffectiveOutputCount int            `json:"effective_output_count"`
	PrimaryOutputCount   int            `json:"primary_output_count"`
	FormatCounts         map[string]int `json:"format_counts,omitempty"`
}

type AdminVideoJobHealthResponse struct {
	JobID           uint64                         `json:"job_id"`
	Health          string                         `json:"health"`
	CheckedAt       string                         `json:"checked_at"`
	JobStatus       string                         `json:"job_status"`
	JobStage        string                         `json:"job_stage"`
	RequestedFormat string                         `json:"requested_format"`
	SourceVideoKey  string                         `json:"source_video_key"`
	StorageChecked  bool                           `json:"storage_checked"`
	Summary         AdminVideoJobHealthSummary     `json:"summary"`
	Stats           AdminVideoJobHealthStats       `json:"stats"`
	Checks          []AdminVideoJobHealthCheckItem `json:"checks"`
}

type adminVideoJobHealthSnapshot struct {
	Job             models.VideoJob
	PublicFound     bool
	LegacyFound     bool
	LegacyJob       *models.VideoJob
	PublicEvents    []models.VideoImageEventPublic
	LegacyEvents    []models.VideoJobEvent
	PublicOutputs   []models.VideoImageOutputPublic
	LegacyArtifacts []models.VideoJobArtifact
	Package         *models.VideoImagePackagePublic
}

type adminVideoJobHealthOutput struct {
	Key                         string
	Format                      string
	Role                        string
	SizeBytes                   int64
	Width                       int
	Height                      int
	IsPrimary                   bool
	UserID                      uint64
	GIFLoopTuneApplied          bool
	GIFLoopTuneEffectiveApplied bool
	GIFLoopTuneFallbackToBase   bool
	GIFLoopTuneScore            float64
	GIFLoopTuneLoopClosure      float64
	GIFLoopTuneMotionMean       float64
	GIFLoopTuneEffectiveSec     float64
}

type adminHealthCheckCollector struct {
	items  []AdminVideoJobHealthCheckItem
	passed int
	warned int
	failed int
}

func (c *adminHealthCheckCollector) add(status string, code string, message string, detail map[string]interface{}) {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "pass":
		c.passed++
	case "warn":
		c.warned++
	case "fail":
		c.failed++
	default:
		status = "warn"
		c.warned++
	}
	c.items = append(c.items, AdminVideoJobHealthCheckItem{
		Code:    code,
		Status:  status,
		Message: message,
		Detail:  detail,
	})
}

func (c *adminHealthCheckCollector) summary() AdminVideoJobHealthSummary {
	return AdminVideoJobHealthSummary{
		Total:  len(c.items),
		Passed: c.passed,
		Warned: c.warned,
		Failed: c.failed,
	}
}

func (c *adminHealthCheckCollector) health() string {
	if c.failed > 0 {
		return "red"
	}
	if c.warned > 0 {
		return "yellow"
	}
	return "green"
}

// GetAdminVideoJobHealth godoc
// @Summary Inspect video job health (admin)
// @Tags admin
// @Produce json
// @Param id path int true "job id"
// @Param check_storage query bool false "whether verify qiniu object existence" default(true)
// @Success 200 {object} AdminVideoJobHealthResponse
// @Router /api/admin/video-jobs/{id}/health [get]
func (h *Handler) GetAdminVideoJobHealth(c *gin.Context) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	checkStorage := true
	if ptr, ok := parseOptionalBoolParam(c.Query("check_storage")); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid check_storage"})
		return
	} else if ptr != nil {
		checkStorage = *ptr
	}

	snapshot, err := h.loadAdminVideoJobHealthSnapshot(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := h.buildAdminVideoJobHealthResponse(snapshot, checkStorage)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) loadAdminVideoJobHealthSnapshot(id uint64) (*adminVideoJobHealthSnapshot, error) {
	snapshot := &adminVideoJobHealthSnapshot{}

	var publicJob models.VideoImageJobPublic
	publicErr := h.db.Where("id = ?", id).First(&publicJob).Error
	if publicErr == nil {
		snapshot.PublicFound = true
		snapshot.Job = convertPublicVideoImageJobToLegacy(publicJob)
	}
	if publicErr != nil && !errors.Is(publicErr, gorm.ErrRecordNotFound) {
		return nil, publicErr
	}

	var legacyJob models.VideoJob
	legacyErr := h.db.Where("id = ?", id).First(&legacyJob).Error
	if legacyErr == nil {
		snapshot.LegacyFound = true
		snapshot.LegacyJob = &legacyJob
		if !snapshot.PublicFound {
			snapshot.Job = legacyJob
		}
	}
	if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
		return nil, legacyErr
	}

	if !snapshot.PublicFound && !snapshot.LegacyFound {
		return nil, gorm.ErrRecordNotFound
	}
	if snapshot.PublicFound && snapshot.LegacyFound && snapshot.LegacyJob != nil {
		if snapshot.Job.StartedAt == nil && snapshot.LegacyJob.StartedAt != nil {
			snapshot.Job.StartedAt = snapshot.LegacyJob.StartedAt
		}
		if snapshot.Job.FinishedAt == nil && snapshot.LegacyJob.FinishedAt != nil {
			snapshot.Job.FinishedAt = snapshot.LegacyJob.FinishedAt
		}
	}

	if err := h.db.Where("job_id = ?", id).Order("id ASC").Find(&snapshot.PublicEvents).Error; err != nil {
		return nil, err
	}
	if err := h.db.Where("job_id = ?", id).Order("id ASC").Find(&snapshot.LegacyEvents).Error; err != nil {
		return nil, err
	}
	if err := h.db.Where("job_id = ?", id).Order("id ASC").Find(&snapshot.PublicOutputs).Error; err != nil {
		return nil, err
	}
	if err := h.db.Where("job_id = ?", id).Order("id ASC").Find(&snapshot.LegacyArtifacts).Error; err != nil {
		return nil, err
	}

	var pkg models.VideoImagePackagePublic
	if err := h.db.Where("job_id = ?", id).First(&pkg).Error; err == nil {
		snapshot.Package = &pkg
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	return snapshot, nil
}

func (h *Handler) buildAdminVideoJobHealthResponse(snapshot *adminVideoJobHealthSnapshot, checkStorage bool) AdminVideoJobHealthResponse {
	job := snapshot.Job
	requestedFormat := parseRequestedFormatFromVideoJob(job)

	eventsSource := "public"
	effectiveEventCount := len(snapshot.PublicEvents)
	if effectiveEventCount == 0 {
		eventsSource = "archive"
		effectiveEventCount = len(snapshot.LegacyEvents)
	}
	outputsSource := "public"
	effectiveOutputs := normalizeEffectiveHealthOutputs(snapshot, requestedFormat)
	if len(snapshot.PublicOutputs) == 0 {
		outputsSource = "archive"
	}

	collector := &adminHealthCheckCollector{}

	if snapshot.PublicFound {
		if snapshot.LegacyFound {
			collector.add("pass", "job_record", "public/archive 双写任务都存在", map[string]interface{}{
				"job_id": job.ID,
			})
		} else {
			collector.add("warn", "job_record", "仅 public 任务存在，archive 旧表缺失（可继续观察）", map[string]interface{}{
				"job_id": job.ID,
			})
		}
	} else {
		collector.add("warn", "job_record", "仅 archive 旧任务存在，public 镜像缺失", map[string]interface{}{
			"job_id": job.ID,
		})
	}

	if snapshot.PublicFound && snapshot.LegacyFound && snapshot.LegacyJob != nil {
		legacy := snapshot.LegacyJob
		if strings.TrimSpace(legacy.Status) != strings.TrimSpace(job.Status) || strings.TrimSpace(legacy.Stage) != strings.TrimSpace(job.Stage) {
			collector.add("warn", "job_sync", "public/archive 状态不一致", map[string]interface{}{
				"public_status": job.Status,
				"public_stage":  job.Stage,
				"legacy_status": legacy.Status,
				"legacy_stage":  legacy.Stage,
			})
		} else {
			collector.add("pass", "job_sync", "public/archive 状态一致", nil)
		}
	}

	status := strings.ToLower(strings.TrimSpace(job.Status))
	stage := strings.ToLower(strings.TrimSpace(job.Stage))
	progress := job.Progress

	statusDetail := map[string]interface{}{
		"status":   status,
		"stage":    stage,
		"progress": progress,
	}
	switch status {
	case models.VideoJobStatusDone:
		if stage != models.VideoJobStageDone || progress < 100 {
			collector.add("warn", "status_stage", "任务已完成但 stage/progress 未完全收敛", statusDetail)
		} else {
			collector.add("pass", "status_stage", "任务状态流转正常", statusDetail)
		}
	case models.VideoJobStatusFailed:
		if stage != models.VideoJobStageFailed && stage != models.VideoJobStageRetrying {
			collector.add("warn", "status_stage", "任务失败但 stage 异常", statusDetail)
		} else {
			collector.add("pass", "status_stage", "失败状态与 stage 匹配", statusDetail)
		}
	case models.VideoJobStatusCancelled:
		if stage != models.VideoJobStageCancelled {
			collector.add("warn", "status_stage", "任务取消但 stage 未落到 cancelled", statusDetail)
		} else {
			collector.add("pass", "status_stage", "取消状态与 stage 匹配", statusDetail)
		}
	case models.VideoJobStatusRunning:
		if stage == models.VideoJobStageDone || stage == models.VideoJobStageFailed || stage == models.VideoJobStageCancelled {
			collector.add("fail", "status_stage", "任务运行中但处于终态 stage", statusDetail)
		} else {
			collector.add("pass", "status_stage", "任务处于处理中", statusDetail)
		}
	default:
		collector.add("warn", "status_stage", "任务状态无法识别，建议人工复核", statusDetail)
	}

	if err := checkVideoJobLifecycleTimes(job); err != nil {
		collector.add("warn", "timestamps", err.Error(), map[string]interface{}{
			"started_at":  formatOptionalTime(job.StartedAt),
			"finished_at": formatOptionalTime(job.FinishedAt),
		})
	} else {
		collector.add("pass", "timestamps", "任务时间轴正常", map[string]interface{}{
			"started_at":  formatOptionalTime(job.StartedAt),
			"finished_at": formatOptionalTime(job.FinishedAt),
		})
	}

	if sourceIssue, detail := checkSourceVideoKeyHealth(job.SourceVideoKey); sourceIssue == "fail" {
		collector.add("fail", "source_key", "源视频 key 不合法", detail)
	} else if sourceIssue == "warn" {
		collector.add("warn", "source_key", "源视频 key 存在兼容性风险", detail)
	} else {
		collector.add("pass", "source_key", "源视频 key 检查通过", detail)
	}

	h.checkEventStreamHealth(collector, status, stage, snapshot, effectiveEventCount)

	formatCounts := map[string]int{}
	primaryOutputs := 0
	for _, item := range effectiveOutputs {
		if item.Format != "" {
			formatCounts[item.Format]++
		}
		if item.IsPrimary {
			primaryOutputs++
		}
	}
	h.checkOutputHealth(collector, snapshot, status, requestedFormat, effectiveOutputs, primaryOutputs, formatCounts)
	h.checkPackageHealth(collector, snapshot, status, requestedFormat, effectiveOutputs)
	h.checkGIFQualityHealth(collector, status, requestedFormat, effectiveOutputs)

	if checkStorage {
		h.checkStorageObjectHealth(collector, snapshot, effectiveOutputs)
	}

	stats := AdminVideoJobHealthStats{
		EventsSource:         eventsSource,
		OutputsSource:        outputsSource,
		PublicEventCount:     len(snapshot.PublicEvents),
		LegacyEventCount:     len(snapshot.LegacyEvents),
		PublicOutputCount:    len(snapshot.PublicOutputs),
		LegacyArtifactCount:  len(snapshot.LegacyArtifacts),
		EffectiveEventCount:  effectiveEventCount,
		EffectiveOutputCount: len(effectiveOutputs),
		PrimaryOutputCount:   primaryOutputs,
		FormatCounts:         formatCounts,
	}

	return AdminVideoJobHealthResponse{
		JobID:           job.ID,
		Health:          collector.health(),
		CheckedAt:       time.Now().Format(time.RFC3339),
		JobStatus:       job.Status,
		JobStage:        job.Stage,
		RequestedFormat: requestedFormat,
		SourceVideoKey:  strings.TrimSpace(job.SourceVideoKey),
		StorageChecked:  checkStorage,
		Summary:         collector.summary(),
		Stats:           stats,
		Checks:          collector.items,
	}
}

func parseRequestedFormatFromVideoJob(job models.VideoJob) string {
	parts := strings.Split(strings.TrimSpace(job.OutputFormats), ",")
	for _, item := range parts {
		format := strings.ToLower(strings.TrimSpace(item))
		if format == "" {
			continue
		}
		if format == "jpeg" {
			return "jpg"
		}
		return format
	}
	return ""
}

func normalizeEffectiveHealthOutputs(snapshot *adminVideoJobHealthSnapshot, fallbackFormat string) []adminVideoJobHealthOutput {
	if len(snapshot.PublicOutputs) > 0 {
		out := make([]adminVideoJobHealthOutput, 0, len(snapshot.PublicOutputs))
		for _, item := range snapshot.PublicOutputs {
			out = append(out, adminVideoJobHealthOutput{
				Key:                         strings.TrimSpace(item.ObjectKey),
				Format:                      strings.ToLower(strings.TrimSpace(item.Format)),
				Role:                        strings.ToLower(strings.TrimSpace(item.FileRole)),
				SizeBytes:                   item.SizeBytes,
				Width:                       item.Width,
				Height:                      item.Height,
				IsPrimary:                   item.IsPrimary,
				UserID:                      item.UserID,
				GIFLoopTuneApplied:          item.GIFLoopTuneApplied,
				GIFLoopTuneEffectiveApplied: item.GIFLoopTuneEffectiveApplied,
				GIFLoopTuneFallbackToBase:   item.GIFLoopTuneFallbackToBase,
				GIFLoopTuneScore:            item.GIFLoopTuneScore,
				GIFLoopTuneLoopClosure:      item.GIFLoopTuneLoopClosure,
				GIFLoopTuneMotionMean:       item.GIFLoopTuneMotionMean,
				GIFLoopTuneEffectiveSec:     item.GIFLoopTuneEffectiveSec,
			})
		}
		return out
	}

	out := make([]adminVideoJobHealthOutput, 0, len(snapshot.LegacyArtifacts))
	for _, item := range snapshot.LegacyArtifacts {
		key := strings.TrimSpace(item.QiniuKey)
		format := strings.ToLower(strings.TrimPrefix(filepath.Ext(key), "."))
		if format == "jpeg" {
			format = "jpg"
		}
		if format == "" {
			format = strings.ToLower(strings.TrimSpace(fallbackFormat))
		}
		role := strings.ToLower(strings.TrimSpace(item.Type))
		switch role {
		case "live_cover", "poster":
			role = "cover"
		case "live_package":
			role = "package"
		}
		if role == "" {
			role = "main"
		}
		if strings.HasSuffix(strings.ToLower(key), ".zip") {
			role = "package"
			if format == "" {
				format = "zip"
			}
		}
		if format == "" {
			format = "gif"
		}
		out = append(out, adminVideoJobHealthOutput{
			Key:       key,
			Format:    format,
			Role:      role,
			SizeBytes: item.SizeBytes,
			Width:     item.Width,
			Height:    item.Height,
			IsPrimary: role == "main",
		})
	}
	return out
}

func checkVideoJobLifecycleTimes(job models.VideoJob) error {
	status := strings.ToLower(strings.TrimSpace(job.Status))
	if status != models.VideoJobStatusQueued && job.StartedAt == nil {
		return errors.New("任务状态已进入处理中/终态，但 started_at 为空")
	}
	if (status == models.VideoJobStatusDone || status == models.VideoJobStatusFailed || status == models.VideoJobStatusCancelled) && job.FinishedAt == nil {
		return errors.New("任务处于终态，但 finished_at 为空")
	}
	if job.StartedAt != nil && job.FinishedAt != nil && job.FinishedAt.Before(*job.StartedAt) {
		return errors.New("finished_at 早于 started_at")
	}
	return nil
}

func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func checkSourceVideoKeyHealth(key string) (string, map[string]interface{}) {
	key = strings.TrimLeft(strings.TrimSpace(strings.SplitN(key, "?", 2)[0]), "/")
	detail := map[string]interface{}{"key": key}
	if key == "" {
		return "fail", detail
	}
	if !strings.HasPrefix(key, "emoji/") {
		return "warn", detail
	}
	ext := strings.ToLower(filepath.Ext(key))
	detail["ext"] = ext
	if _, ok := allowedVideoFileExt[ext]; !ok {
		return "fail", detail
	}
	return "pass", detail
}

func (h *Handler) checkEventStreamHealth(collector *adminHealthCheckCollector, status string, stage string, snapshot *adminVideoJobHealthSnapshot, effectiveCount int) {
	hasPublic := len(snapshot.PublicEvents) > 0
	events := make([]models.VideoJobEvent, 0)
	if hasPublic {
		for _, item := range snapshot.PublicEvents {
			events = append(events, models.VideoJobEvent{Stage: item.Stage, Level: item.Level, Message: item.Message})
		}
	} else {
		events = append(events, snapshot.LegacyEvents...)
	}
	if effectiveCount == 0 {
		collector.add("warn", "events", "任务无阶段事件，建议检查 worker 上报链路", nil)
		return
	}

	hasErrorLevel := false
	hasFinalStageEvent := false
	for _, item := range events {
		lvl := strings.ToLower(strings.TrimSpace(item.Level))
		stg := strings.ToLower(strings.TrimSpace(item.Stage))
		if lvl == "error" {
			hasErrorLevel = true
		}
		if stg == stage || (status == models.VideoJobStatusDone && stg == models.VideoJobStageDone) ||
			(status == models.VideoJobStatusFailed && stg == models.VideoJobStageFailed) ||
			(status == models.VideoJobStatusCancelled && stg == models.VideoJobStageCancelled) {
			hasFinalStageEvent = true
		}
	}

	detail := map[string]interface{}{
		"source": func() string {
			if hasPublic {
				return "public"
			}
			return "archive"
		}(),
		"event_count":     effectiveCount,
		"has_error_level": hasErrorLevel,
		"has_final_stage": hasFinalStageEvent,
		"expected_stage":  stage,
		"expected_status": status,
	}

	if (status == models.VideoJobStatusFailed || status == models.VideoJobStatusCancelled) && !hasErrorLevel {
		collector.add("warn", "events", "终态任务缺少 error 级事件", detail)
		return
	}
	if !hasFinalStageEvent {
		collector.add("warn", "events", "事件链路未记录到当前终态 stage", detail)
		return
	}
	collector.add("pass", "events", "事件链路完整", detail)
}

func (h *Handler) checkOutputHealth(
	collector *adminHealthCheckCollector,
	snapshot *adminVideoJobHealthSnapshot,
	status string,
	requestedFormat string,
	effectiveOutputs []adminVideoJobHealthOutput,
	primaryCount int,
	formatCounts map[string]int,
) {
	effectiveCount := len(effectiveOutputs)
	if status == models.VideoJobStatusDone {
		if effectiveCount == 0 {
			collector.add("fail", "outputs", "任务已完成但无任何产物记录", nil)
			return
		}
		collector.add("pass", "outputs", "任务已完成且存在产物", map[string]interface{}{
			"count": effectiveCount,
		})
	} else if effectiveCount > 0 {
		collector.add("warn", "outputs", "任务未完成但已有产物写入（可能重试中）", map[string]interface{}{
			"status": status,
			"count":  effectiveCount,
		})
	} else {
		collector.add("pass", "outputs", "当前任务尚未产出文件", map[string]interface{}{
			"status": status,
		})
	}

	if status == models.VideoJobStatusDone && primaryCount <= 0 {
		collector.add("warn", "outputs_primary", "完成任务缺少主产物标记", map[string]interface{}{
			"count": effectiveCount,
		})
	} else if primaryCount > 0 {
		collector.add("pass", "outputs_primary", "主产物标记正常", map[string]interface{}{"primary_count": primaryCount})
	}

	if status == models.VideoJobStatusDone && requestedFormat != "" {
		if formatCounts[requestedFormat] <= 0 {
			collector.add("fail", "outputs_format", "产物缺少请求格式文件", map[string]interface{}{
				"requested_format": requestedFormat,
				"format_counts":    formatCounts,
			})
		} else {
			collector.add("pass", "outputs_format", "请求格式产物存在", map[string]interface{}{
				"requested_format": requestedFormat,
				"count":            formatCounts[requestedFormat],
			})
		}
	}

	if len(snapshot.PublicOutputs) > 0 && len(snapshot.LegacyArtifacts) > 0 && len(snapshot.PublicOutputs) != len(snapshot.LegacyArtifacts) {
		collector.add("warn", "outputs_sync", "public 与 archive 产物数量不一致", map[string]interface{}{
			"public_count": len(snapshot.PublicOutputs),
			"legacy_count": len(snapshot.LegacyArtifacts),
		})
	}

	if jobUserID := snapshot.Job.UserID; jobUserID > 0 && len(snapshot.PublicOutputs) > 0 {
		invalid := 0
		for _, item := range effectiveOutputs {
			if item.UserID > 0 && item.UserID != jobUserID {
				invalid++
			}
		}
		if invalid > 0 {
			collector.add("fail", "outputs_user", "产物 user_id 与任务 user_id 不一致", map[string]interface{}{
				"invalid_count": invalid,
				"job_user_id":   jobUserID,
			})
		} else {
			collector.add("pass", "outputs_user", "产物 user_id 与任务一致", map[string]interface{}{"job_user_id": jobUserID})
		}
	}
}

func (h *Handler) checkPackageHealth(
	collector *adminHealthCheckCollector,
	snapshot *adminVideoJobHealthSnapshot,
	status string,
	requestedFormat string,
	outputs []adminVideoJobHealthOutput,
) {
	packageExpected := shouldExpectPackageForHealthJob(requestedFormat, outputs)
	if snapshot.Package == nil {
		if status == models.VideoJobStatusDone {
			if packageExpected {
				collector.add("warn", "package", "完成任务缺少 ZIP 打包记录", map[string]interface{}{
					"requested_format": requestedFormat,
				})
			} else {
				collector.add("pass", "package", "当前任务类型不要求 ZIP 打包", map[string]interface{}{
					"requested_format": requestedFormat,
				})
			}
		} else {
			collector.add("pass", "package", "当前任务尚无 ZIP 打包记录", nil)
		}
		return
	}

	if strings.TrimSpace(snapshot.Package.ZipObjectKey) == "" || snapshot.Package.ZipSizeBytes <= 0 {
		collector.add("warn", "package", "ZIP 打包记录不完整", map[string]interface{}{
			"zip_key":    snapshot.Package.ZipObjectKey,
			"zip_size":   snapshot.Package.ZipSizeBytes,
			"file_count": snapshot.Package.FileCount,
		})
		return
	}

	collector.add("pass", "package", "ZIP 打包记录存在", map[string]interface{}{
		"zip_key":    snapshot.Package.ZipObjectKey,
		"zip_size":   snapshot.Package.ZipSizeBytes,
		"file_count": snapshot.Package.FileCount,
	})
}

func shouldExpectPackageForHealthJob(requestedFormat string, outputs []adminVideoJobHealthOutput) bool {
	format := strings.ToLower(strings.TrimSpace(requestedFormat))
	if format == "live" || format == "zip" {
		return true
	}
	for _, item := range outputs {
		role := strings.ToLower(strings.TrimSpace(item.Role))
		outputFormat := strings.ToLower(strings.TrimSpace(item.Format))
		key := strings.ToLower(strings.TrimSpace(item.Key))
		if role == "package" || outputFormat == "zip" || strings.HasSuffix(key, ".zip") {
			return true
		}
	}
	return false
}

func (h *Handler) checkGIFQualityHealth(collector *adminHealthCheckCollector, status string, requestedFormat string, outputs []adminVideoJobHealthOutput) {
	if requestedFormat != "gif" {
		return
	}
	if status != models.VideoJobStatusDone {
		collector.add("pass", "gif_quality", "任务尚未完成，暂不检查 GIF 质量", nil)
		return
	}

	var candidate *adminVideoJobHealthOutput
	for idx := range outputs {
		item := &outputs[idx]
		if item.Format != "gif" {
			continue
		}
		if item.IsPrimary || item.Role == "main" {
			candidate = item
			break
		}
		if candidate == nil {
			candidate = item
		}
	}
	if candidate == nil {
		collector.add("fail", "gif_quality", "完成任务缺少主 GIF 产物", nil)
		return
	}

	if candidate.SizeBytes <= 0 {
		collector.add("fail", "gif_quality", "GIF 文件大小异常", map[string]interface{}{
			"size_bytes": candidate.SizeBytes,
		})
		return
	}
	if candidate.Width <= 0 || candidate.Height <= 0 {
		collector.add("warn", "gif_quality", "GIF 尺寸信息缺失", map[string]interface{}{
			"width":  candidate.Width,
			"height": candidate.Height,
		})
		return
	}

	detail := map[string]interface{}{
		"size_bytes":                     candidate.SizeBytes,
		"width":                          candidate.Width,
		"height":                         candidate.Height,
		"loop_tune_applied":              candidate.GIFLoopTuneApplied,
		"loop_tune_effective_applied":    candidate.GIFLoopTuneEffectiveApplied,
		"loop_tune_fallback_to_base":     candidate.GIFLoopTuneFallbackToBase,
		"loop_tune_score":                candidate.GIFLoopTuneScore,
		"loop_tune_loop_closure":         candidate.GIFLoopTuneLoopClosure,
		"loop_tune_motion_mean":          candidate.GIFLoopTuneMotionMean,
		"loop_tune_effective_duration_s": candidate.GIFLoopTuneEffectiveSec,
	}
	if candidate.GIFLoopTuneApplied && candidate.GIFLoopTuneScore <= 0 {
		collector.add("warn", "gif_quality", "GIF loop tune 已应用但评分缺失", detail)
		return
	}
	collector.add("pass", "gif_quality", "GIF 主产物质量指标正常", detail)
}

func (h *Handler) checkStorageObjectHealth(collector *adminHealthCheckCollector, snapshot *adminVideoJobHealthSnapshot, outputs []adminVideoJobHealthOutput) {
	if h == nil || h.qiniu == nil {
		collector.add("warn", "storage", "七牛配置缺失，跳过对象存在性检查", nil)
		return
	}
	metrics := parseJSONMap(snapshot.Job.Metrics)
	sourceVideoDeleted := metricBool(metrics, "source_video_deleted")

	sourceKeys := make([]string, 0, 1)
	outputKeys := make([]string, 0, len(outputs)+1)
	if sourceKey := strings.TrimLeft(strings.TrimSpace(snapshot.Job.SourceVideoKey), "/"); sourceKey != "" {
		sourceKeys = append(sourceKeys, sourceKey)
	}
	for _, item := range outputs {
		if key := strings.TrimLeft(strings.TrimSpace(item.Key), "/"); key != "" {
			outputKeys = append(outputKeys, key)
		}
	}
	if snapshot.Package != nil {
		if key := strings.TrimLeft(strings.TrimSpace(snapshot.Package.ZipObjectKey), "/"); key != "" {
			outputKeys = append(outputKeys, key)
		}
	}

	keys := append([]string{}, sourceKeys...)
	keys = append(keys, outputKeys...)
	keys = uniqueSortedKeys(keys)
	sourceKeys = uniqueSortedKeys(sourceKeys)
	outputKeys = uniqueSortedKeys(outputKeys)
	if len(keys) == 0 {
		collector.add("warn", "storage", "无可检查的对象 key", nil)
		return
	}

	missing := make([]string, 0)
	missingSource := make([]string, 0)
	missingOutput := make([]string, 0)
	errorsList := make([]string, 0)
	legacyPrefix := make([]string, 0)
	sourceSet := make(map[string]struct{}, len(sourceKeys))
	for _, key := range sourceKeys {
		sourceSet[key] = struct{}{}
	}

	bm := h.qiniu.BucketManager()
	for _, key := range keys {
		if !strings.HasPrefix(key, "emoji/video-image/") {
			if _, isSource := sourceSet[key]; isSource {
				continue
			}
			legacyPrefix = append(legacyPrefix, key)
		}
		if _, err := bm.Stat(h.qiniu.Bucket, key); err != nil {
			if isQiniuObjectNotFoundError(err) {
				missing = append(missing, key)
				if _, isSource := sourceSet[key]; isSource {
					missingSource = append(missingSource, key)
				} else {
					missingOutput = append(missingOutput, key)
				}
			} else {
				errorsList = append(errorsList, fmt.Sprintf("%s: %v", key, err))
			}
		}
	}

	detail := map[string]interface{}{
		"checked":              len(keys),
		"checked_source":       len(sourceKeys),
		"checked_outputs":      len(outputKeys),
		"source_video_deleted": sourceVideoDeleted,
		"missing_count":        len(missing),
		"missing_source_count": len(missingSource),
		"missing_output_count": len(missingOutput),
		"error_count":          len(errorsList),
	}
	if len(missing) > 0 {
		detail["missing_keys"] = truncateStringList(missing, 8)
	}
	if len(missingSource) > 0 {
		detail["missing_source_keys"] = truncateStringList(missingSource, 4)
	}
	if len(missingOutput) > 0 {
		detail["missing_output_keys"] = truncateStringList(missingOutput, 8)
	}
	if len(errorsList) > 0 {
		detail["errors"] = truncateStringList(errorsList, 4)
	}
	if len(legacyPrefix) > 0 {
		detail["legacy_prefix_keys"] = truncateStringList(legacyPrefix, 8)
	}

	if len(missingOutput) > 0 {
		collector.add("fail", "storage", "七牛产物对象存在缺失", detail)
		return
	}
	if len(missingSource) > 0 {
		if sourceVideoDeleted {
			detail["ignored_missing_source_keys"] = truncateStringList(missingSource, 4)
			collector.add("pass", "storage", "源视频已清理，产物对象检查通过", detail)
			return
		}
		collector.add("warn", "storage", "源视频对象缺失（可能已被清理）", detail)
		return
	}
	if len(errorsList) > 0 {
		collector.add("warn", "storage", "七牛对象检查出现异常", detail)
		return
	}
	if len(legacyPrefix) > 0 {
		collector.add("warn", "storage", "对象 key 前缀存在历史路径（非新规范）", detail)
		return
	}
	collector.add("pass", "storage", "七牛对象检查通过", detail)
}

func uniqueSortedKeys(keys []string) []string {
	set := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, item := range keys {
		key := strings.TrimLeft(strings.TrimSpace(item), "/")
		if key == "" {
			continue
		}
		if _, ok := set[key]; ok {
			continue
		}
		set[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func truncateStringList(in []string, limit int) []string {
	if limit <= 0 || len(in) <= limit {
		return in
	}
	out := append([]string{}, in[:limit]...)
	out = append(out, fmt.Sprintf("...(+%d)", len(in)-limit))
	return out
}

func isQiniuObjectNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "no such file") || strings.Contains(msg, "not found") || strings.Contains(msg, "612") {
		return true
	}
	return false
}
