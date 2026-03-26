package videojobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type imageAI1ExecutablePlan struct {
	SchemaVersion    string
	Schema           map[string]interface{}
	Executable       map[string]interface{}
	EventMeta        map[string]interface{}
	DirectorSnapshot map[string]interface{}
}

// processImagePipeline is the dedicated execution lane for still/image-first jobs
// (png/jpg/webp/live/mp4). It is intentionally decoupled from GIF-specific
// AI2/AI3 stages and metrics.
func (p *Processor) processImagePipeline(ctx context.Context, jobID uint64) error {
	executionLane := "image"

	var job models.VideoJob
	if err := p.db.First(&job, jobID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: video job not found", asynq.SkipRetry)
		}
		return err
	}
	if job.Status == models.VideoJobStatusDone || job.Status == models.VideoJobStatusCancelled {
		return nil
	}
	if recovered, err := p.recoverCompletedJobFromExistingResult(&job); err != nil {
		return err
	} else if recovered {
		return nil
	}
	if strings.TrimSpace(job.SourceVideoKey) == "" {
		return permanentError{err: errors.New("source video key is empty")}
	}

	preOptions := parseJSONMap(job.Options)
	if isFlowAwaitingAI1Confirm(preOptions) {
		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "waiting for user confirmation before continuing", map[string]interface{}{
			"flow_mode": "ai1_confirm",
		})
		return nil
	}

	now := time.Now()
	acquired, err := p.acquireVideoJobRun(job.ID, now)
	if err != nil {
		return fmt.Errorf("acquire video job run: %w", err)
	}
	if !acquired {
		p.appendJobEvent(job.ID, models.VideoJobStageQueued, "info", "skip duplicated processing trigger", nil)
		return nil
	}

	requestedFormats := normalizeOutputFormats(job.OutputFormats)
	primaryFormat := firstRequestedFormat(requestedFormats)
	metricPrefix := resolveImagePipelineMetricPrefix(primaryFormat)
	pipelineMetricKey := fmt.Sprintf("%s_pipeline_v1", metricPrefix)
	stageStatusKey := fmt.Sprintf("%s_pipeline_stage_status_v1", metricPrefix)
	ai1MetricKey := fmt.Sprintf("%s_ai1_plan_v1", metricPrefix)
	extractionMetricKey := fmt.Sprintf("%s_extraction_v1", metricPrefix)

	p.appendJobEvent(job.ID, models.VideoJobStagePreprocessing, "info", "execution lane resolved", map[string]interface{}{
		"execution_lane":   executionLane,
		"requested_format": primaryFormat,
	})
	p.appendJobEvent(job.ID, models.VideoJobStagePreprocessing, "info", "video job started", nil)

	if p.isJobCancelled(job.ID) {
		p.appendJobEvent(job.ID, models.VideoJobStageCancelled, "info", "job cancelled before processing", nil)
		p.syncJobCost(job.ID)
		p.syncJobPointSettlement(job.ID, models.VideoJobStatusCancelled)
		p.cleanupSourceVideo(job.ID, "cancelled")
		return nil
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return permanentError{err: errors.New("ffmpeg not found in PATH")}
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return permanentError{err: errors.New("ffprobe not found in PATH")}
	}

	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("video-job-%d-*", job.ID))
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.mp4")
	optionsPayload := parseJSONMap(job.Options)
	expectedSourceSizeBytes := sourceInt64FromAny(optionsPayload["source_video_size_bytes"])
	sourceReadability, err := p.downloadObjectByKeyWithReadability(ctx, job.SourceVideoKey, sourcePath, expectedSourceSizeBytes)
	if err != nil {
		p.persistSourceReadability(job.ID, job.Metrics, sourceReadability)
		p.appendJobEvent(job.ID, models.VideoJobStagePreprocessing, "warn", "source video readability failed", sourceReadability)
		var readErr *sourceReadError
		if errors.As(err, &readErr) && readErr != nil && readErr.Permanent {
			return permanentError{err: fmt.Errorf("download source video: %w", readErr)}
		}
		return fmt.Errorf("download source video: %w", err)
	}
	if used, ok := sourceReadability["used_fallback"].(bool); ok && used {
		p.appendJobEvent(job.ID, models.VideoJobStagePreprocessing, "info", "source video readability fallback used", sourceReadability)
	}

	meta, err := probeVideo(ctx, sourcePath)
	if err != nil {
		return fmt.Errorf("probe video: %w", err)
	}
	sourceInfo, _ := os.Stat(sourcePath)

	pipelineStageStatus := map[string]string{
		"ai1":        "pending",
		"extraction": "pending",
		"delivery":   "pending",
	}
	metrics := map[string]interface{}{
		"duration_sec": meta.DurationSec,
		"width":        meta.Width,
		"height":       meta.Height,
		"fps":          meta.FPS,
		pipelineMetricKey: map[string]interface{}{
			"execution_lane":   executionLane,
			"requested_format": primaryFormat,
			"version":          "v1",
		},
		stageStatusKey: pipelineStageStatus,
	}
	if len(sourceReadability) > 0 {
		metrics["source_video_readability_v1"] = sourceReadability
	}
	if sourceInfo != nil && sourceInfo.Size() > 0 {
		metrics["source_video_size_bytes"] = sourceInfo.Size()
	}

	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageAnalyzing,
		"progress": 30,
		"metrics":  mustJSON(metrics),
	})
	p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "video metadata analyzed", metrics)

	if p.isJobCancelled(job.ID) {
		p.appendJobEvent(job.ID, models.VideoJobStageCancelled, "info", "job cancelled during analyzing", nil)
		p.syncJobCost(job.ID)
		p.syncJobPointSettlement(job.ID, models.VideoJobStatusCancelled)
		p.cleanupSourceVideo(job.ID, "cancelled")
		return nil
	}

	options := parseJobOptions(job.Options)
	qualitySettings := p.loadQualitySettings()
	flowMode := normalizeVideoFlowMode(stringFromAny(optionsPayload["flow_mode"]))
	ai1Confirmed := boolFromAny(optionsPayload["ai1_confirmed"])
	ai1PauseConsumed := boolFromAny(optionsPayload["ai1_pause_consumed"])

	qualitySettings, qualityOverrides := applyQualityProfileOverridesFromOptions(qualitySettings, optionsPayload, requestedFormats)
	if len(qualityOverrides) > 0 {
		metrics["quality_profile_overrides_applied"] = qualityOverrides
		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "quality profile overrides applied", map[string]interface{}{
			"overrides": qualityOverrides,
		})
	}
	if sceneTags := inferSceneTags(job.Title, job.SourceVideoKey, requestedFormats); len(sceneTags) > 0 {
		metrics["scene_tags_v1"] = sceneTags
	}

	options = applyStillProfileDefaults(options, requestedFormats, qualitySettings)
	if options.CropW <= 0 || options.CropH <= 0 {
		autoCrop, applied, cropErr := detectAutoLetterboxCrop(ctx, sourcePath, meta)
		switch {
		case cropErr != nil:
			metrics["auto_crop_v1"] = map[string]interface{}{
				"enabled": true,
				"applied": false,
				"error":   cropErr.Error(),
			}
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "auto letterbox crop detection failed", map[string]interface{}{
				"error": cropErr.Error(),
			})
		case applied:
			options.CropX = autoCrop.CropX
			options.CropY = autoCrop.CropY
			options.CropW = autoCrop.CropW
			options.CropH = autoCrop.CropH
			optionsPayload["crop_x"] = options.CropX
			optionsPayload["crop_y"] = options.CropY
			optionsPayload["crop_w"] = options.CropW
			optionsPayload["crop_h"] = options.CropH
			optionsPayload["auto_crop_v1"] = autoCrop
			metrics["auto_crop_v1"] = autoCrop
			p.updateVideoJob(job.ID, map[string]interface{}{
				"options": mustJSON(optionsPayload),
			})
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "auto letterbox crop applied", map[string]interface{}{
				"crop_x":            autoCrop.CropX,
				"crop_y":            autoCrop.CropY,
				"crop_w":            autoCrop.CropW,
				"crop_h":            autoCrop.CropH,
				"confidence":        autoCrop.Confidence,
				"removed_area_rate": roundTo(autoCrop.RemovedAreaRate, 4),
			})
		default:
			metrics["auto_crop_v1"] = map[string]interface{}{
				"enabled": true,
				"applied": false,
			}
		}
	}

	existingPlan := mapFromAny(optionsPayload["ai1_executable_plan_v1"])
	schemaVersion := strings.TrimSpace(stringFromAny(optionsPayload["ai1_plan_schema_version"]))
	if schemaVersion == "" {
		schemaVersion = resolveImageAI1PlanSchemaVersion(primaryFormat)
	}

	planGenerated := false
	planMeta := map[string]interface{}{}
	directorSnapshot := map[string]interface{}{}
	if len(existingPlan) == 0 {
		pipelineStageStatus["ai1"] = "running"
		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", imagePipelineEventMessage(primaryFormat, "ai1 planning started"), map[string]interface{}{
			"flow_mode":        flowMode,
			"requested_format": primaryFormat,
		})
		plan := p.buildImageAI1ExecutablePlan(ctx, job, sourcePath, meta, requestedFormats, options, qualitySettings)
		existingPlan = plan.Executable
		planMeta = plan.EventMeta
		directorSnapshot = plan.DirectorSnapshot
		schemaVersion = plan.SchemaVersion
		planGenerated = true

		optionsPayload["ai1_executable_plan_v1"] = existingPlan
		optionsPayload["ai1_plan_schema_version"] = schemaVersion
		optionsPayload["ai1_plan_generated"] = true
		optionsPayload["ai1_plan_generated_at"] = time.Now().Format(time.RFC3339)
		if len(directorSnapshot) > 0 {
			optionsPayload["ai1_director_snapshot_v1"] = directorSnapshot
		}
		if len(planMeta) > 0 {
			optionsPayload["ai1_event_meta_v1"] = planMeta
		}

		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", imagePipelineEventMessage(primaryFormat, "ai1 preview generated"), planMeta)
		p.persistImageAI1Plan(job, requestedFormats, flowMode, ai1Confirmed, meta, "ai1_preview_generated", planMeta, directorSnapshot, schemaVersion, plan.Schema, existingPlan)
		pipelineStageStatus["ai1"] = "done"
	} else {
		pipelineStageStatus["ai1"] = "reused"
		planMeta = mapFromAny(optionsPayload["ai1_event_meta_v1"])
		directorSnapshot = mapFromAny(optionsPayload["ai1_director_snapshot_v1"])
	}

	appliedPlanPatch := applyImageAI1ExecutablePlan(existingPlan, meta, &options, optionsPayload, schemaVersion)
	if len(appliedPlanPatch) > 0 || planGenerated {
		p.updateVideoJob(job.ID, map[string]interface{}{
			"options": mustJSON(optionsPayload),
		})
	}

	ai1Metric := map[string]interface{}{
		"schema_version":   schemaVersion,
		"generated":        planGenerated,
		"reused":           !planGenerated,
		"flow_mode":        flowMode,
		"requested_format": primaryFormat,
		"applied_patch":    appliedPlanPatch,
		"status":           pipelineStageStatus["ai1"],
	}
	if len(planMeta) > 0 {
		ai1Metric["event_meta"] = planMeta
	}
	if len(directorSnapshot) > 0 {
		ai1Metric["director_snapshot"] = buildAI1PlanDirectorSnapshot(directorSnapshot)
	}
	metrics[ai1MetricKey] = ai1Metric

	if shouldPauseAtAI1(flowMode, ai1Confirmed, ai1PauseConsumed) {
		pipelineStageStatus["extraction"] = "awaiting_user_confirm"
		pipelineStageStatus["delivery"] = "awaiting_user_confirm"
		if p.pauseForAI1Confirmation(job.ID, metrics, optionsPayload, map[string]interface{}{
			"flow_mode": flowMode,
			"stage":     "ai1_preview_generated",
		}) {
			return nil
		}
	}

	pipelineStageStatus["extraction"] = "running"
	extractOptions := applyStillProfileDefaults(options, requestedFormats, qualitySettings)

	frameDir := filepath.Join(tmpDir, "frames")
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return fmt.Errorf("create frame dir: %w", err)
	}

	effectiveDurationSec := effectiveSampleDuration(meta, options)
	candidateBudget := qualitySelectionCandidateBudget(extractOptions.MaxStatic)
	interval := chooseFrameInterval(effectiveDurationSec, extractOptions.FrameIntervalSec, candidateBudget)

	if err := extractFrames(ctx, sourcePath, frameDir, meta, extractOptions, interval, qualitySettings); err != nil {
		return fmt.Errorf("extract frames: %w", err)
	}
	framePaths, err := collectFramePaths(frameDir, candidateBudget)
	if err != nil {
		return fmt.Errorf("collect frames: %w", err)
	}
	if len(framePaths) == 0 {
		return permanentError{err: errors.New("no frames extracted from video")}
	}

	optimizedFramePaths, qualityReport := optimizeFramePathsForQuality(framePaths, extractOptions.MaxStatic, qualitySettings)
	if len(optimizedFramePaths) > 0 {
		framePaths = optimizedFramePaths
	}

	metrics["frame_quality"] = qualityReport
	qualitySettingsMetric := map[string]interface{}{
		"min_brightness":              qualitySettings.MinBrightness,
		"max_brightness":              qualitySettings.MaxBrightness,
		"blur_threshold_factor":       qualitySettings.BlurThresholdFactor,
		"duplicate_hamming_threshold": qualitySettings.DuplicateHammingThreshold,
		"still_min_blur_score":        qualitySettings.StillMinBlurScore,
		"still_min_exposure_score":    qualitySettings.StillMinExposureScore,
		"still_min_width":             qualitySettings.StillMinWidth,
		"still_min_height":            qualitySettings.StillMinHeight,
		"jpg_profile":                 qualitySettings.JPGProfile,
		"png_profile":                 qualitySettings.PNGProfile,
		"webp_profile":                qualitySettings.WebPProfile,
		"live_profile":                qualitySettings.LiveProfile,
		"png_target_size_kb":          qualitySettings.PNGTargetSizeKB,
		"jpg_target_size_kb":          qualitySettings.JPGTargetSizeKB,
		"webp_target_size_kb":         qualitySettings.WebPTargetSizeKB,
		"quality_candidate_budget":    candidateBudget,
		"still_clarity_enhance":       shouldApplyStillClarityEnhancement(meta, extractOptions, qualitySettings),
	}
	metrics[fmt.Sprintf("%s_quality_settings_v1", metricPrefix)] = qualitySettingsMetric
	metrics[extractionMetricKey] = map[string]interface{}{
		"frame_count":        len(framePaths),
		"candidate_budget":   candidateBudget,
		"interval_sec":       roundTo(interval, 3),
		"effective_duration": roundTo(effectiveDurationSec, 3),
	}

	pipelineStageStatus["extraction"] = "done"
	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageRendering,
		"progress": 55,
		"metrics":  mustJSON(metrics),
	})
	p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", "frame extraction completed", map[string]interface{}{
		"frames":                    len(framePaths),
		"quality_blur_reject":       qualityReport.RejectedBlur,
		"quality_bright_reject":     qualityReport.RejectedBrightness,
		"quality_exposure_reject":   qualityReport.RejectedExposure,
		"quality_resolution_reject": qualityReport.RejectedResolution,
		"quality_still_blur_reject": qualityReport.RejectedStillBlurGate,
		"quality_dup_reject":        qualityReport.RejectedNearDuplicate,
		"quality_fallback":          qualityReport.FallbackApplied,
	})

	if p.isJobCancelled(job.ID) {
		p.appendJobEvent(job.ID, models.VideoJobStageCancelled, "info", "job cancelled after rendering", nil)
		p.syncJobCost(job.ID)
		p.syncJobPointSettlement(job.ID, models.VideoJobStatusCancelled)
		p.cleanupSourceVideo(job.ID, "cancelled")
		return nil
	}

	pipelineStageStatus["delivery"] = "running"
	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageRendering,
		"progress": 70,
	})
	p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", imagePipelineEventMessage(primaryFormat, "output render pipeline started"), map[string]interface{}{
		"entry_progress": 70,
	})

	highlightCandidates := make([]highlightCandidate, 0)
	animatedWindows := make([]highlightCandidate, 0)
	resultCollectionID, totalFrames, uploadedKeys, generatedFormats, packageOutcome, err := p.persistJobResults(
		ctx,
		job,
		framePaths,
		sourcePath,
		meta,
		options,
		highlightCandidates,
		animatedWindows,
		qualitySettings,
	)
	if err != nil {
		deleteQiniuKeysByPrefix(p.qiniu, uploadedKeys)
		return err
	}

	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageUploading,
		"progress": 88,
	})

	metrics["static_count"] = totalFrames
	metrics["output_formats_requested"] = requestedFormats
	metrics["output_formats"] = generatedFormats
	metrics["result_collection_id"] = resultCollectionID
	metrics["asset_domain"] = models.VideoJobAssetDomainVideo
	metrics["edit_options"] = jobOptionsMetrics(options, interval)
	metrics["effective_duration_sec"] = roundTo(effectiveDurationSec, 3)
	metrics["package_zip_status"] = packageOutcome.Status
	metrics["package_zip_attempts"] = packageOutcome.Attempts
	metrics["package_zip_retry_count"] = packageOutcome.RetryCount
	if packageOutcome.Key != "" {
		metrics["package_zip_key"] = packageOutcome.Key
	}
	if packageOutcome.Name != "" {
		metrics["package_zip_name"] = packageOutcome.Name
	}
	if packageOutcome.SizeBytes > 0 {
		metrics["package_zip_size_bytes"] = packageOutcome.SizeBytes
	}
	if packageOutcome.Error != "" {
		metrics["package_zip_error"] = packageOutcome.Error
	}
	pipelineStageStatus["delivery"] = "done"
	metrics[stageStatusKey] = pipelineStageStatus

	finishedAt := time.Now()
	p.updateVideoJob(job.ID, map[string]interface{}{
		"status":               models.VideoJobStatusDone,
		"stage":                models.VideoJobStageDone,
		"progress":             100,
		"asset_domain":         models.VideoJobAssetDomainVideo,
		"result_collection_id": resultCollectionID,
		"metrics":              mustJSON(metrics),
		"error_message":        "",
		"finished_at":          finishedAt,
	})
	p.appendJobEvent(job.ID, models.VideoJobStageDone, "info", "video job completed", map[string]interface{}{
		"collection_id":       resultCollectionID,
		"static_count":        totalFrames,
		"package_zip_status":  packageOutcome.Status,
		"package_zip_attempt": packageOutcome.Attempts,
		fmt.Sprintf("%s_pipeline_status_v1", metricPrefix): pipelineStageStatus,
	})
	if packageOutcome.Status == packageZipStatusFailed {
		p.appendJobEvent(job.ID, models.VideoJobStageDone, "warn", "video job completed without zip package", map[string]interface{}{
			"attempts": packageOutcome.Attempts,
			"error":    packageOutcome.Error,
		})
	}
	p.syncJobCost(job.ID)
	p.syncJobPointSettlement(job.ID, models.VideoJobStatusDone)
	p.cleanupSourceVideo(job.ID, "done")
	return nil
}

func resolveImagePipelineMetricPrefix(primaryFormat string) string {
	switch NormalizeRequestedFormat(primaryFormat) {
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

func resolveImageAI1PlanSchemaVersion(primaryFormat string) string {
	switch NormalizeRequestedFormat(primaryFormat) {
	case "png":
		return VideoJobAI1PlanSchemaPNGV1
	default:
		return VideoJobAI1PlanSchemaImageV1
	}
}

func imagePipelineEventMessage(primaryFormat, suffix string) string {
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return ""
	}
	if NormalizeRequestedFormat(primaryFormat) == "png" {
		return "png " + suffix
	}
	return suffix
}

func sanitizeTextList(items []string, maxItems int) []string {
	if len(items) == 0 || maxItems <= 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, minInt(len(items), maxItems))
	for _, item := range items {
		text := strings.TrimSpace(item)
		if text == "" {
			continue
		}
		normalized := strings.ToLower(text)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, text)
		if len(out) >= maxItems {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func clampImageTargetCount(value int) int {
	if value <= 0 {
		return 24
	}
	if value > 80 {
		return 80
	}
	return value
}

func resolveImageFocusWindow(meta videoProbeMeta, localSuggestion highlightSuggestion) (startSec float64, endSec float64, source string) {
	if localSuggestion.Selected == nil {
		return 0, 0, ""
	}
	startSec = localSuggestion.Selected.StartSec
	endSec = localSuggestion.Selected.EndSec
	if meta.DurationSec > 0 {
		if startSec < 0 {
			startSec = 0
		}
		if endSec > meta.DurationSec {
			endSec = meta.DurationSec
		}
	}
	if endSec <= startSec {
		return 0, 0, ""
	}
	return roundTo(startSec, 3), roundTo(endSec, 3), "local_highlight"
}

func resolveImagePlanInterval(preferred float64, targetCount int, meta videoProbeMeta, focusStart, focusEnd float64) float64 {
	if preferred > 0 {
		return roundTo(clampFloat(preferred, 0.2, 8.0), 3)
	}
	duration := meta.DurationSec
	if focusEnd > focusStart {
		duration = focusEnd - focusStart
	}
	if duration <= 0 {
		duration = 8
	}
	if targetCount <= 0 {
		targetCount = 24
	}
	interval := duration / float64(targetCount)
	if interval <= 0 {
		interval = 1
	}
	return roundTo(clampFloat(interval, 0.2, 8.0), 3)
}

func buildImageAI1ExecutableSchema(schemaVersion string, requestedFormat string) map[string]interface{} {
	return map[string]interface{}{
		"$schema":        "https://json-schema.org/draft/2020-12/schema",
		"title":          "AI1 Image Executable Plan",
		"type":           "object",
		"schema_version": schemaVersion,
		"required":       []string{"target_format", "mode", "target_count", "frame_interval_sec"},
		"properties": map[string]interface{}{
			"target_format": map[string]interface{}{
				"type":    "string",
				"default": requestedFormat,
			},
			"mode": map[string]interface{}{
				"type": "string",
				"enum": []string{"uniform_sampling", "focus_window"},
			},
			"target_count": map[string]interface{}{
				"type":    "integer",
				"minimum": 1,
				"maximum": 80,
			},
			"frame_interval_sec": map[string]interface{}{
				"type":    "number",
				"minimum": 0.2,
				"maximum": 8,
			},
			"focus_window": map[string]interface{}{
				"type": []string{"object", "null"},
				"properties": map[string]interface{}{
					"start_sec": map[string]interface{}{"type": "number", "minimum": 0},
					"end_sec":   map[string]interface{}{"type": "number", "minimum": 0},
					"source":    map[string]interface{}{"type": "string"},
				},
				"required": []string{"start_sec", "end_sec"},
			},
			"must_capture": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"avoid":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"style_direction": map[string]interface{}{
				"type": "string",
			},
		},
	}
}

func (p *Processor) buildImageAI1ExecutablePlan(
	ctx context.Context,
	job models.VideoJob,
	sourcePath string,
	meta videoProbeMeta,
	requestedFormats []string,
	options jobOptions,
	qualitySettings QualitySettings,
) imageAI1ExecutablePlan {
	requestedFormat := firstRequestedFormat(requestedFormats)
	if requestedFormat == "" {
		requestedFormat = "png"
	}
	requestedFormat = NormalizeRequestedFormat(requestedFormat)
	schemaVersion := resolveImageAI1PlanSchemaVersion(requestedFormat)

	plan := imageAI1ExecutablePlan{
		SchemaVersion: schemaVersion,
		Schema:        buildImageAI1ExecutableSchema(schemaVersion, requestedFormat),
		Executable:    map[string]interface{}{},
		EventMeta: map[string]interface{}{
			"sub_stage":        "ai1_planning",
			"requested_format": requestedFormat,
			"target_format":    requestedFormat,
			"duration_sec":     roundTo(meta.DurationSec, 3),
			"width":            meta.Width,
			"height":           meta.Height,
			"fps":              roundTo(meta.FPS, 3),
		},
		DirectorSnapshot: map[string]interface{}{},
	}

	localSuggestion, localErr := suggestHighlightWindow(ctx, sourcePath, meta, qualitySettings)
	if localErr != nil {
		plan.EventMeta["local_focus_error"] = localErr.Error()
	} else if localSuggestion.Selected != nil {
		plan.EventMeta["local_focus_start_sec"] = roundTo(localSuggestion.Selected.StartSec, 3)
		plan.EventMeta["local_focus_end_sec"] = roundTo(localSuggestion.Selected.EndSec, 3)
		plan.EventMeta["local_focus_score"] = roundTo(localSuggestion.Selected.Score, 4)
	}

	directive, directorSnapshot, directorErr := p.requestAIGIFPromptDirective(ctx, job, sourcePath, meta, localSuggestion, qualitySettings)
	if len(directorSnapshot) > 0 {
		plan.DirectorSnapshot = directorSnapshot
	}

	aiReply := buildGenericAI1Reply(job.Title, requestedFormats, meta)
	if directorErr != nil {
		plan.EventMeta["error"] = directorErr.Error()
		plan.EventMeta["director_applied"] = false
	} else {
		plan.EventMeta["director_applied"] = true
	}
	if directive != nil {
		if reply := strings.TrimSpace(buildAIDirectorNaturalReply(directive)); reply != "" {
			aiReply = reply
		}
		if text := strings.TrimSpace(directive.BusinessGoal); text != "" {
			plan.EventMeta["business_goal"] = text
		}
		if text := strings.TrimSpace(directive.Audience); text != "" {
			plan.EventMeta["audience"] = text
		}
		if list := sanitizeTextList(directive.MustCapture, 12); len(list) > 0 {
			plan.EventMeta["must_capture"] = list
		}
		if list := sanitizeTextList(directive.Avoid, 12); len(list) > 0 {
			plan.EventMeta["avoid"] = list
		}
		if text := strings.TrimSpace(directive.StyleDirection); text != "" {
			plan.EventMeta["style_direction"] = text
		}
		if text := strings.TrimSpace(directive.DirectiveText); text != "" {
			plan.EventMeta["directive_text"] = text
		}
		if directive.ClipCountMin > 0 {
			plan.EventMeta["clip_count_min"] = directive.ClipCountMin
		}
		if directive.ClipCountMax > 0 {
			plan.EventMeta["clip_count_max"] = directive.ClipCountMax
		}
	}
	plan.EventMeta["ai_reply"] = aiReply

	targetCount := clampImageTargetCount(options.MaxStatic)
	if maxHint := intFromAny(plan.EventMeta["clip_count_max"]); maxHint > 0 {
		hintTarget := clampImageTargetCount(maxHint * 2)
		if hintTarget > 0 && hintTarget < targetCount {
			targetCount = hintTarget
		}
	}

	focusStart, focusEnd, focusSource := resolveImageFocusWindow(meta, localSuggestion)
	mode := "uniform_sampling"
	if focusEnd > focusStart {
		mode = "focus_window"
	}
	intervalSec := resolveImagePlanInterval(options.FrameIntervalSec, targetCount, meta, focusStart, focusEnd)

	plan.Executable = map[string]interface{}{
		"target_format":      requestedFormat,
		"mode":               mode,
		"target_count":       targetCount,
		"frame_interval_sec": intervalSec,
	}
	if mode == "focus_window" {
		plan.Executable["focus_window"] = map[string]interface{}{
			"start_sec": focusStart,
			"end_sec":   focusEnd,
			"source":    focusSource,
		}
		plan.EventMeta["selected_start_sec"] = focusStart
		plan.EventMeta["selected_end_sec"] = focusEnd
		plan.EventMeta["focus_window_source"] = focusSource
	}
	if list := stringSliceFromAny(plan.EventMeta["must_capture"]); len(list) > 0 {
		plan.Executable["must_capture"] = list
	}
	if list := stringSliceFromAny(plan.EventMeta["avoid"]); len(list) > 0 {
		plan.Executable["avoid"] = list
	}
	if style := strings.TrimSpace(stringFromAny(plan.EventMeta["style_direction"])); style != "" {
		plan.Executable["style_direction"] = style
	}
	return plan
}

func applyImageAI1ExecutablePlan(
	executable map[string]interface{},
	meta videoProbeMeta,
	options *jobOptions,
	optionsPayload map[string]interface{},
	schemaVersion string,
) map[string]interface{} {
	if options == nil {
		return map[string]interface{}{}
	}
	if optionsPayload == nil {
		optionsPayload = map[string]interface{}{}
	}
	if len(executable) == 0 {
		return map[string]interface{}{}
	}

	applied := map[string]interface{}{}
	targetCount := clampImageTargetCount(intFromAny(executable["target_count"]))
	if targetCount > 0 {
		options.MaxStatic = targetCount
		optionsPayload["max_static"] = targetCount
		applied["max_static"] = targetCount
	}

	focusStart := 0.0
	focusEnd := 0.0
	if focus := mapFromAny(executable["focus_window"]); len(focus) > 0 {
		focusStart = floatFromAny(focus["start_sec"])
		focusEnd = floatFromAny(focus["end_sec"])
		if meta.DurationSec > 0 {
			focusStart, focusEnd = clampHighlightWindow(focusStart, focusEnd, meta.DurationSec)
		}
		if focusEnd > focusStart {
			options.StartSec = roundTo(focusStart, 3)
			options.EndSec = roundTo(focusEnd, 3)
			optionsPayload["start_sec"] = options.StartSec
			optionsPayload["end_sec"] = options.EndSec
			applied["start_sec"] = options.StartSec
			applied["end_sec"] = options.EndSec
		}
	}

	interval := floatFromAny(executable["frame_interval_sec"])
	if interval <= 0 {
		interval = resolveImagePlanInterval(options.FrameIntervalSec, targetCount, meta, focusStart, focusEnd)
	}
	interval = roundTo(clampFloat(interval, 0.2, 8), 3)
	options.FrameIntervalSec = interval
	optionsPayload["frame_interval_sec"] = interval
	applied["frame_interval_sec"] = interval

	if mode := strings.TrimSpace(stringFromAny(executable["mode"])); mode != "" {
		optionsPayload["ai1_plan_mode"] = mode
		applied["mode"] = mode
	}

	optionsPayload["ai1_plan_applied"] = true
	optionsPayload["ai1_plan_applied_at"] = time.Now().Format(time.RFC3339)
	optionsPayload["ai1_executable_plan_v1"] = executable
	if schemaVersion != "" {
		optionsPayload["ai1_plan_schema_version"] = schemaVersion
	}
	return applied
}

func (p *Processor) persistImageAI1Plan(
	job models.VideoJob,
	requestedFormats []string,
	flowMode string,
	ai1Confirmed bool,
	meta videoProbeMeta,
	eventStage string,
	eventMeta map[string]interface{},
	directorSnapshot map[string]interface{},
	schemaVersion string,
	executableSchema map[string]interface{},
	executablePlan map[string]interface{},
) {
	if p == nil || p.db == nil || job.ID == 0 {
		return
	}
	if schemaVersion == "" {
		schemaVersion = resolveImageAI1PlanSchemaVersion(firstRequestedFormat(requestedFormats))
	}
	requestedFormat := firstRequestedFormat(requestedFormats)
	sourcePrompt := resolveVideoJobSourcePrompt(job)
	aiReply := strings.TrimSpace(stringFromAny(eventMeta["ai_reply"]))

	provider := strings.TrimSpace(stringFromAny(directorSnapshot["provider"]))
	model := strings.TrimSpace(stringFromAny(directorSnapshot["model"]))
	promptVersion := strings.TrimSpace(stringFromAny(directorSnapshot["prompt_version"]))
	fallbackUsed := boolFromAny(directorSnapshot["fallback_used"]) || strings.TrimSpace(stringFromAny(eventMeta["error"])) != ""

	trace := map[string]interface{}{
		"event_stage": strings.TrimSpace(eventStage),
		"flow_mode":   normalizeVideoFlowMode(flowMode),
		"sub_stage":   strings.TrimSpace(stringFromAny(eventMeta["sub_stage"])),
		"error":       strings.TrimSpace(stringFromAny(eventMeta["error"])),
	}
	if inputMode := strings.TrimSpace(stringFromAny(directorSnapshot["director_input_mode_applied"])); inputMode != "" {
		trace["director_input_mode"] = inputMode
	}
	if inputSource := strings.TrimSpace(stringFromAny(directorSnapshot["director_input_source"])); inputSource != "" {
		trace["director_input_source"] = inputSource
	}

	planPayload := map[string]interface{}{
		"schema_version":    schemaVersion,
		"requested_format":  requestedFormat,
		"requested_formats": requestedFormats,
		"flow_mode":         normalizeVideoFlowMode(flowMode),
		"source_prompt":     sourcePrompt,
		"ai_reply":          aiReply,
		"source_meta": map[string]interface{}{
			"duration_sec": roundTo(meta.DurationSec, 3),
			"width":        meta.Width,
			"height":       meta.Height,
			"fps":          roundTo(meta.FPS, 3),
		},
		"directive":         buildAI1PlanDirective(eventMeta),
		"event_meta":        cloneMapStringKey(eventMeta),
		"trace":             trace,
		"director_snapshot": buildAI1PlanDirectorSnapshot(directorSnapshot),
		"executable_schema": cloneMapStringKey(executableSchema),
		"executable_plan":   cloneMapStringKey(executablePlan),
	}

	status := resolveAI1PlanStatus(flowMode, ai1Confirmed)
	row := models.VideoJobAI1Plan{
		JobID:           job.ID,
		UserID:          job.UserID,
		RequestedFormat: requestedFormat,
		SchemaVersion:   schemaVersion,
		Status:          status,
		SourcePrompt:    sourcePrompt,
		PlanJSON:        mustJSON(planPayload),
		ModelProvider:   provider,
		ModelName:       model,
		PromptVersion:   promptVersion,
		FallbackUsed:    fallbackUsed,
		ConfirmedByUser: ai1Confirmed,
	}
	if ai1Confirmed {
		now := time.Now()
		row.ConfirmedAt = &now
	}
	if err := UpsertVideoJobAI1Plan(p.db, row); err != nil {
		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "ai1 plan persistence failed", map[string]interface{}{
			"error": err.Error(),
		})
	}
}
