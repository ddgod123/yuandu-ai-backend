package videojobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/hibiken/asynq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	videoAIReadingStage         = "video_reading"
	videoAIReadingPromptVersion = "video_reading_v1"
)

type videoAIReadingResponse struct {
	Summary    string   `json:"summary"`
	Highlights []string `json:"highlights"`
	Tone       string   `json:"tone,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

func EnsureVideoJobAIReadingQueued(db *gorm.DB, jobID, userID uint64) error {
	if db == nil || jobID == 0 || userID == 0 {
		return nil
	}
	now := time.Now()
	row := models.VideoJobAIReading{
		JobID:          jobID,
		UserID:         userID,
		Status:         models.VideoJobAIReadingStatusQueued,
		HighlightsJSON: datatypes.JSON([]byte("[]")),
		Metadata:       datatypes.JSON([]byte("{}")),
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "job_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"user_id":       userID,
			"status":        models.VideoJobAIReadingStatusQueued,
			"error_message": "",
			"updated_at":    now,
		}),
	}).Create(&row).Error
}

func (p *Processor) HandleAnalyzeVideoText(ctx context.Context, task *asynq.Task) error {
	var payload ProcessVideoJobPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if payload.JobID == 0 {
		return fmt.Errorf("%w: invalid job id", asynq.SkipRetry)
	}
	return p.processVideoAIReading(ctx, payload.JobID)
}

func (p *Processor) processVideoAIReading(ctx context.Context, jobID uint64) error {
	if p == nil || p.db == nil {
		return fmt.Errorf("%w: db not initialized", asynq.SkipRetry)
	}
	if p.qiniu == nil {
		return fmt.Errorf("%w: qiniu not configured", asynq.SkipRetry)
	}

	var job models.VideoJob
	if err := p.db.Select("id", "user_id", "title", "status", "source_video_key", "options", "metrics").First(&job, jobID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: video job not found", asynq.SkipRetry)
		}
		return err
	}
	if strings.EqualFold(strings.TrimSpace(job.Status), models.VideoJobStatusCancelled) {
		return nil
	}

	_ = EnsureVideoJobAIReadingQueued(p.db, job.ID, job.UserID)
	if err := p.updateVideoAIReadingRow(job.ID, map[string]interface{}{
		"user_id":       job.UserID,
		"status":        models.VideoJobAIReadingStatusProcessing,
		"error_message": "",
		"finished_at":   nil,
	}); err != nil {
		return err
	}

	started := time.Now()
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("video-reading-%d-*", job.ID))
	if err != nil {
		return p.failVideoAIReading(job, 0, err, nil)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.mp4")
	optionsPayload := parseJSONMap(job.Options)
	expectedSourceSizeBytes := sourceInt64FromAny(optionsPayload["source_video_size_bytes"])
	if _, err := p.downloadObjectByKeyWithReadability(ctx, job.SourceVideoKey, sourcePath, expectedSourceSizeBytes); err != nil {
		return p.failVideoAIReading(job, clampDurationMillis(started), fmt.Errorf("download source video: %w", err), nil)
	}

	meta, err := probeVideo(ctx, sourcePath)
	if err != nil {
		return p.failVideoAIReading(job, clampDurationMillis(started), fmt.Errorf("probe video: %w", err), nil)
	}

	sampleFrames, intervalSec, sampleErr := p.extractVideoReadingFrames(ctx, tmpDir, sourcePath, meta, optionsPayload)
	if sampleErr != nil {
		return p.failVideoAIReading(job, clampDurationMillis(started), sampleErr, map[string]interface{}{
			"duration_sec": roundTo(meta.DurationSec, 3),
			"fps":          roundTo(meta.FPS, 3),
			"width":        meta.Width,
			"height":       meta.Height,
		})
	}
	defer cleanupFiles(sampleFrames)

	cfg := p.loadVideoAIReadingConfig(optionsPayload)
	systemPrompt := buildVideoAIReadingSystemPrompt()
	userParts := p.buildVideoAIReadingUserParts(job, meta, sampleFrames, intervalSec)

	var (
		modelText   string
		usage       cloudHighlightUsage
		rawResp     map[string]interface{}
		durationMs  int64
		callErr     error
		readingResp videoAIReadingResponse
	)
	if cfg.Enabled && strings.TrimSpace(cfg.APIKey) != "" && strings.TrimSpace(cfg.Endpoint) != "" && strings.TrimSpace(cfg.Model) != "" {
		modelText, usage, rawResp, durationMs, callErr = p.callOpenAICompatJSONChatWithUserParts(ctx, cfg, systemPrompt, userParts)
		if callErr == nil {
			parseErr := unmarshalModelJSONWithRepair(modelText, &readingResp)
			if parseErr != nil || strings.TrimSpace(readingResp.Summary) == "" {
				callErr = fmt.Errorf("parse model response failed: %w", parseErr)
			}
		}
		p.recordVideoJobAIUsage(videoJobAIUsageInput{
			JobID:             job.ID,
			UserID:            job.UserID,
			Stage:             videoAIReadingStage,
			Provider:          cfg.Provider,
			Model:             cfg.Model,
			Endpoint:          cfg.Endpoint,
			InputTokens:       usage.InputTokens,
			OutputTokens:      usage.OutputTokens,
			CachedInputTokens: usage.CachedInputTokens,
			ImageTokens:       usage.ImageTokens,
			VideoTokens:       usage.VideoTokens,
			AudioSeconds:      usage.AudioSeconds,
			RequestDurationMs: durationMs,
			RequestStatus: func() string {
				if callErr != nil {
					return "error"
				}
				return "ok"
			}(),
			RequestError: errorText(callErr),
			Metadata: map[string]interface{}{
				"prompt_version":      cfg.PromptVersion,
				"sample_frame_count":  len(sampleFrames),
				"sample_interval_sec": roundTo(intervalSec, 3),
				"raw_model":           stringFromAny(rawResp["model"]),
			},
		})
	}

	if callErr != nil || strings.TrimSpace(readingResp.Summary) == "" {
		readingResp = buildFallbackVideoAIReading(job, meta, len(sampleFrames), intervalSec)
		if durationMs <= 0 {
			durationMs = clampDurationMillis(started)
		}
	}
	readingResp.Summary = truncateString(strings.TrimSpace(readingResp.Summary), 1200)
	readingResp.Highlights = sanitizeTextList(readingResp.Highlights, 12)
	readingResp.Tags = sanitizeTextList(readingResp.Tags, 12)

	metadata := map[string]interface{}{
		"sample_frame_count":  len(sampleFrames),
		"sample_interval_sec": roundTo(intervalSec, 3),
		"duration_sec":        roundTo(meta.DurationSec, 3),
		"width":               meta.Width,
		"height":              meta.Height,
		"fps":                 roundTo(meta.FPS, 3),
		"tone":                strings.TrimSpace(readingResp.Tone),
		"tags":                readingResp.Tags,
	}
	if callErr != nil {
		metadata["fallback_reason"] = callErr.Error()
	}
	finishedAt := time.Now()
	if err := p.updateVideoAIReadingRow(job.ID, map[string]interface{}{
		"user_id":             job.UserID,
		"status":              models.VideoJobAIReadingStatusDone,
		"summary_text":        readingResp.Summary,
		"highlights_json":     mustJSON(readingResp.Highlights),
		"model_provider":      strings.TrimSpace(cfg.Provider),
		"model_name":          strings.TrimSpace(cfg.Model),
		"prompt_version":      strings.TrimSpace(firstNonEmptyString(cfg.PromptVersion, videoAIReadingPromptVersion)),
		"request_duration_ms": durationMs,
		"error_message":       "",
		"metadata":            mustJSON(metadata),
		"finished_at":         &finishedAt,
	}); err != nil {
		return err
	}

	p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "video semantic reading completed", map[string]interface{}{
		"summary_preview":    truncateString(readingResp.Summary, 120),
		"highlight_count":    len(readingResp.Highlights),
		"sample_frame_count": len(sampleFrames),
	})
	p.markVideoAIReadingMetrics(job, models.VideoJobAIReadingStatusDone, readingResp, metadata)
	return nil
}

func (p *Processor) extractVideoReadingFrames(
	ctx context.Context,
	tmpDir string,
	sourcePath string,
	meta videoProbeMeta,
	optionsPayload map[string]interface{},
) ([]string, float64, error) {
	frameDir := filepath.Join(tmpDir, "reading_frames")
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return nil, 0, fmt.Errorf("create frame dir: %w", err)
	}

	options := parseJobOptions(mustJSON(optionsPayload))
	requestedFormats := normalizeOutputFormats(stringFromAny(optionsPayload["output_formats"]))
	if len(requestedFormats) == 0 {
		requestedFormats = []string{NormalizeRequestedFormat(stringFromAny(optionsPayload["requested_format"]))}
	}
	qualitySettings := DefaultQualitySettings()
	options = applyStillProfileDefaults(options, requestedFormats, qualitySettings)
	options.MaxStatic = clampInt(options.MaxStatic, 4, 12)
	effectiveDuration := effectiveSampleDuration(meta, options)
	interval := chooseFrameInterval(effectiveDuration, options.FrameIntervalSec, options.MaxStatic)
	interval = clampFloat(interval, 0.5, 4.0)

	if err := extractFrames(ctx, sourcePath, frameDir, meta, options, interval, qualitySettings); err != nil {
		return nil, 0, fmt.Errorf("extract reading frames: %w", err)
	}
	paths, err := collectFramePaths(frameDir, options.MaxStatic)
	if err != nil {
		return nil, 0, fmt.Errorf("collect reading frames: %w", err)
	}
	if len(paths) == 0 {
		return nil, 0, fmt.Errorf("no reading frames extracted")
	}
	return paths, interval, nil
}

func (p *Processor) buildVideoAIReadingUserParts(
	job models.VideoJob,
	meta videoProbeMeta,
	framePaths []string,
	intervalSec float64,
) []openAICompatContentPart {
	parts := make([]openAICompatContentPart, 0, 1+len(framePaths)*2)
	parts = append(parts, openAICompatContentPart{
		Type: "text",
		Text: fmt.Sprintf(
			"任务信息：title=%q, duration_sec=%.3f, width=%d, height=%d, fps=%.3f, frame_interval_sec=%.3f。请结合样本帧输出 JSON：{\"summary\":\"...\",\"highlights\":[\"...\"],\"tone\":\"...\",\"tags\":[\"...\"]}。",
			strings.TrimSpace(job.Title),
			roundTo(meta.DurationSec, 3),
			meta.Width,
			meta.Height,
			roundTo(meta.FPS, 3),
			roundTo(intervalSec, 3),
		),
	})
	for idx, framePath := range framePaths {
		dataURL, _, err := buildPNGAI2LLMRerankImageDataURL(framePath, 768, 72, 900*1024)
		if err != nil || strings.TrimSpace(dataURL) == "" {
			continue
		}
		parts = append(parts, openAICompatContentPart{
			Type: "text",
			Text: fmt.Sprintf("sample_frame=%d, approx_second=%.3f, file_name=%s", idx+1, roundTo(float64(idx)*intervalSec, 3), filepath.Base(framePath)),
		})
		parts = append(parts, openAICompatContentPart{
			Type: "image_url",
			ImageURL: &openAICompatImageURL{
				URL: dataURL,
			},
		})
	}
	return parts
}

func (p *Processor) loadVideoAIReadingConfig(options map[string]interface{}) aiModelCallConfig {
	cfg := p.loadGIFAIPlannerConfig()
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = strings.ToLower(strings.TrimSpace(p.cfg.LLMProvider))
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = strings.TrimSpace(p.cfg.LLMModel)
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		cfg.Endpoint = strings.TrimSpace(p.cfg.LLMEndpoint)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		cfg.APIKey = strings.TrimSpace(p.cfg.LLMAPIKey)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 900
	}
	cfg.PromptVersion = firstNonEmptyString(strings.TrimSpace(cfg.PromptVersion), videoAIReadingPromptVersion)
	if pref := strings.TrimSpace(stringFromAny(options["ai_model_preference"])); pref != "" {
		if provider, model := parseVideoJobAIModelPreference(pref); model != "" {
			if provider != "" {
				cfg.Provider = provider
			}
			cfg.Model = model
		}
	}
	cfg.Enabled = strings.TrimSpace(cfg.APIKey) != "" && strings.TrimSpace(cfg.Endpoint) != "" && strings.TrimSpace(cfg.Model) != ""
	return cfg
}

func buildVideoAIReadingSystemPrompt() string {
	return `你是视频内容分析助手。请根据给定的视频信息与抽样帧，返回精简 JSON（不要 markdown）：
{
  "summary":"80~180字，描述视频主题、叙事与可传播价值",
  "highlights":["3~6条，每条不超过30字，强调关键画面或动作"],
  "tone":"视频整体风格，如活泼/叙事/科普/新闻",
  "tags":["可选，最多8个关键词"]
}
要求：
1) 仅输出 JSON；
2) summary 与 highlights 必须可读、可直接给运营使用；
3) 信息不足时给出保守结论，不要编造不存在的事实。`
}

func buildFallbackVideoAIReading(job models.VideoJob, meta videoProbeMeta, sampleCount int, intervalSec float64) videoAIReadingResponse {
	title := strings.TrimSpace(job.Title)
	if title == "" {
		title = "未命名视频任务"
	}
	highlights := []string{
		fmt.Sprintf("视频时长约 %.1f 秒", roundTo(meta.DurationSec, 1)),
		fmt.Sprintf("分辨率 %dx%d", meta.Width, meta.Height),
		fmt.Sprintf("已抽样 %d 帧（间隔 %.1f 秒）", sampleCount, roundTo(intervalSec, 1)),
	}
	return videoAIReadingResponse{
		Summary:    fmt.Sprintf("任务「%s」已完成视频语义分析降级摘要：当前根据元数据与抽样帧生成概览，可用于后续图文生成流程。", title),
		Highlights: highlights,
		Tone:       "中性",
		Tags:       []string{"视频解析", "自动摘要"},
	}
}

func (p *Processor) updateVideoAIReadingRow(jobID uint64, updates map[string]interface{}) error {
	if p == nil || p.db == nil || jobID == 0 || len(updates) == 0 {
		return nil
	}
	row := models.VideoJobAIReading{
		JobID:          jobID,
		Status:         models.VideoJobAIReadingStatusQueued,
		HighlightsJSON: datatypes.JSON([]byte("[]")),
		Metadata:       datatypes.JSON([]byte("{}")),
	}
	return p.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "job_id"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&row).Error
}

func (p *Processor) failVideoAIReading(
	job models.VideoJob,
	durationMs int64,
	err error,
	extra map[string]interface{},
) error {
	errText := strings.TrimSpace(errorText(err))
	if durationMs <= 0 {
		durationMs = 1
	}
	updates := map[string]interface{}{
		"user_id":             job.UserID,
		"status":              models.VideoJobAIReadingStatusFailed,
		"request_duration_ms": durationMs,
		"error_message":       errText,
		"metadata":            mustJSON(extra),
		"finished_at":         time.Now(),
	}
	_ = p.updateVideoAIReadingRow(job.ID, updates)
	p.markVideoAIReadingMetrics(job, models.VideoJobAIReadingStatusFailed, videoAIReadingResponse{}, map[string]interface{}{
		"error": errText,
	})
	p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "video semantic reading failed", map[string]interface{}{
		"error": errText,
	})
	if err == nil {
		return nil
	}
	return err
}

func (p *Processor) markVideoAIReadingMetrics(
	job models.VideoJob,
	status string,
	resp videoAIReadingResponse,
	extra map[string]interface{},
) {
	if p == nil || job.ID == 0 {
		return
	}
	metrics := parseJSONMap(job.Metrics)
	metrics["video_ai_reading_v1"] = map[string]interface{}{
		"status":          strings.ToLower(strings.TrimSpace(status)),
		"summary_preview": truncateString(strings.TrimSpace(resp.Summary), 120),
		"highlight_count": len(resp.Highlights),
		"updated_at":      time.Now().Format(time.RFC3339),
		"extra":           extra,
	}
	p.updateVideoJob(job.ID, map[string]interface{}{
		"metrics": mustJSON(metrics),
	})
}

func cleanupFiles(paths []string) {
	for _, item := range paths {
		path := strings.TrimSpace(item)
		if path == "" {
			continue
		}
		_ = os.Remove(path)
	}
}

func truncateString(raw string, maxLen int) string {
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
