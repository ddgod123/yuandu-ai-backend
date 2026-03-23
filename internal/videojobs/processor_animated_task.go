package videojobs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"emoji/internal/storage"

	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"golang.org/x/sync/errgroup"
)

func shouldLimitGIFRenderConcurrency(meta videoProbeMeta, tasks []animatedTask) bool {
	return resolveGIFRenderWorkerCap(meta, tasks, NormalizeQualitySettings(DefaultQualitySettings()), 2) <= 1
}

func resolveGIFRenderWorkerCap(
	meta videoProbeMeta,
	tasks []animatedTask,
	qualitySettings QualitySettings,
	currentWorkers int,
) int {
	if currentWorkers <= 1 {
		return 1
	}
	if len(tasks) <= 1 {
		return currentWorkers
	}
	gifTaskCount := 0
	for _, task := range tasks {
		if strings.EqualFold(strings.TrimSpace(task.Format), "gif") {
			gifTaskCount++
		}
	}
	if gifTaskCount <= 1 {
		return currentWorkers
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	longSide := meta.Width
	if meta.Height > longSide {
		longSide = meta.Height
	}
	highResThreshold := qualitySettings.GIFDownshiftHighResLongSideThreshold
	if highResThreshold <= 0 {
		highResThreshold = 1400
	}
	earlyDurationThreshold := qualitySettings.GIFDownshiftEarlyDurationSec
	if earlyDurationThreshold <= 0 {
		earlyDurationThreshold = 45
	}
	longDurationThreshold := qualitySettings.GIFDurationTierLongSec
	if longDurationThreshold <= 0 {
		longDurationThreshold = 120
	}

	// 高分辨率或真正长视频仍维持串行，优先稳定性。
	if longSide >= highResThreshold || meta.DurationSec >= longDurationThreshold {
		return 1
	}
	// 中长视频限制到 2 并发：在可控风险下显著缩短总渲染耗时。
	if meta.DurationSec >= earlyDurationThreshold && currentWorkers > 2 {
		return 2
	}
	return currentWorkers
}

func (p *Processor) processAnimatedTask(
	ctx context.Context,
	sourcePath string,
	outputDir string,
	prefix string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	uploader *qiniustorage.FormUploader,
	task animatedTask,
) animatedTaskResult {
	result := animatedTaskResult{Task: task}
	window := task.Window
	adaptiveOptions, adaptiveProfile := tuneAnimatedOptionsForWindow(meta, options, qualitySettings, task.Format, window)
	if adaptiveProfile.DurationSec > 0 && (task.Format == "gif" || task.Format == "webp" || task.Format == "mp4") {
		window = clampWindowDuration(window, adaptiveProfile.DurationSec, meta.DurationSec)
	}
	finalWindow := window
	renderWindow := window
	renderSourcePath := sourcePath
	bundleOffsetSec := 0.0
	if task.Format == "gif" && strings.TrimSpace(task.MezzaninePath) != "" {
		renderSourcePath = strings.TrimSpace(task.MezzaninePath)
		bundleOffsetSec = task.BundleStartSec
		renderWindow = toBundleRelativeWindow(window, bundleOffsetSec)
	}
	baseAnimatedWindow := renderWindow
	var gifLoopTune *gifLoopTuningResult
	var gifOptimization *GIFOptimizationResult
	var gifLoopTuneElapsedMs int64
	var gifsicleStageMs int64
	if task.Format == "gif" {
		loopTuneStartedAt := time.Now()
		tunedWindow, tune, tuneErr := optimizeGIFLoopWindow(ctx, renderSourcePath, meta, adaptiveOptions, qualitySettings, renderWindow)
		gifLoopTuneElapsedMs = time.Since(loopTuneStartedAt).Milliseconds()
		if tuneErr == nil && tune.SampleFrames > 0 {
			if bundleOffsetSec > 0 {
				tune.BaseStartSec = roundTo(tune.BaseStartSec+bundleOffsetSec, 3)
				tune.BaseEndSec = roundTo(tune.BaseEndSec+bundleOffsetSec, 3)
				tune.TunedStartSec = roundTo(tune.TunedStartSec+bundleOffsetSec, 3)
				tune.TunedEndSec = roundTo(tune.TunedEndSec+bundleOffsetSec, 3)
			}
			gifLoopTune = &tune
			if tune.Applied {
				renderWindow = tunedWindow
			}
		}
	}
	if task.Format == "live" {
		window = clampWindowDuration(window, 3.0, meta.DurationSec)
		uploaded := make([]string, 0, 3)
		liveOut, err := renderLiveOutputPackage(ctx, sourcePath, outputDir, meta, adaptiveOptions, qualitySettings, window, task.WindowIndex)
		if err != nil {
			var unsupported unsupportedOutputFormatError
			if errors.As(err, &unsupported) {
				reason := strings.TrimSpace(unsupported.Reason)
				if reason == "" {
					reason = "format is not supported by ffmpeg runtime"
				}
				result.UnsupportedReason = reason
				return result
			}
			result.Err = fmt.Errorf("render live clip %d failed: %w", task.WindowIndex, err)
			return result
		}

		liveVideoKey := buildVideoImageOutputObjectKey(prefix, "live", fmt.Sprintf("clip_%02d_video.mov", task.WindowIndex))
		coverFileKey := buildVideoImageOutputObjectKey(prefix, "live", fmt.Sprintf("clip_%02d_cover.jpg", task.WindowIndex))
		packageKey := buildVideoImagePackageObjectKey(prefix, fmt.Sprintf("clip_%02d_live.zip", task.WindowIndex))

		uploadTasks := []qiniuUploadTask{
			{
				Key:   liveVideoKey,
				Path:  liveOut.VideoPath,
				Label: fmt.Sprintf("live video clip %d", task.WindowIndex),
			},
			{
				Key:   coverFileKey,
				Path:  liveOut.CoverPath,
				Label: fmt.Sprintf("live cover clip %d", task.WindowIndex),
			},
			{
				Key:   packageKey,
				Path:  liveOut.PackagePath,
				Label: fmt.Sprintf("live package clip %d", task.WindowIndex),
			},
		}
		maxUploadWorkers := qualitySettings.UploadConcurrency
		if maxUploadWorkers > 3 {
			maxUploadWorkers = 3
		}
		uploaded, err = uploadQiniuTasksConcurrently(ctx, uploader, p.qiniu, uploadTasks, maxUploadWorkers)
		if err != nil {
			result.Err = err
			result.UploadedKeys = uploaded
			return result
		}

		result.FileKey = packageKey
		result.ThumbKey = coverFileKey
		result.Width = liveOut.Width
		result.Height = liveOut.Height
		result.SizeBytes = liveOut.PackageSizeBytes
		result.UploadedKeys = uploaded
		result.Artifacts = []animatedArtifactPayload{
			{
				Type:       "live_video",
				Key:        liveVideoKey,
				MimeType:   "video/quicktime",
				SizeBytes:  liveOut.VideoSizeBytes,
				Width:      liveOut.Width,
				Height:     liveOut.Height,
				DurationMs: liveOut.DurationMs,
				Metadata: map[string]interface{}{
					"window_index": task.WindowIndex,
					"start_sec":    window.StartSec,
					"end_sec":      window.EndSec,
					"score":        window.Score,
					"reason":       strings.TrimSpace(window.Reason),
					"format":       "live",
					"motion_score": adaptiveProfile.MotionScore,
					"motion_level": adaptiveProfile.Level,
				},
			},
			{
				Type:      "live_cover",
				Key:       coverFileKey,
				MimeType:  "image/jpeg",
				SizeBytes: liveOut.CoverSizeBytes,
				Width:     liveOut.Width,
				Height:    liveOut.Height,
				Metadata: map[string]interface{}{
					"window_index":          task.WindowIndex,
					"format":                "live",
					"cover_score":           roundTo(liveOut.CoverScore, 3),
					"cover_ts_sec":          roundTo(liveOut.CoverTimestamp, 3),
					"cover_quality":         roundTo(liveOut.CoverQuality, 3),
					"cover_stability":       roundTo(liveOut.CoverStability, 3),
					"cover_temporal":        roundTo(liveOut.CoverTemporal, 3),
					"cover_portrait":        roundTo(liveOut.CoverPortrait, 3),
					"cover_exposure":        roundTo(liveOut.CoverExposure, 3),
					"cover_face":            roundTo(liveOut.CoverFace, 3),
					"cover_portrait_weight": roundTo(qualitySettings.LiveCoverPortraitWeight, 3),
				},
			},
			{
				Type:       "live_package",
				Key:        packageKey,
				MimeType:   mimeTypeByFormat("live"),
				SizeBytes:  liveOut.PackageSizeBytes,
				Width:      liveOut.Width,
				Height:     liveOut.Height,
				DurationMs: liveOut.DurationMs,
				Metadata: map[string]interface{}{
					"window_index":          task.WindowIndex,
					"start_sec":             window.StartSec,
					"end_sec":               window.EndSec,
					"score":                 window.Score,
					"reason":                strings.TrimSpace(window.Reason),
					"format":                "live",
					"entries":               []string{"photo.jpg", "video.mov"},
					"cover_score":           roundTo(liveOut.CoverScore, 3),
					"cover_ts_sec":          roundTo(liveOut.CoverTimestamp, 3),
					"cover_quality":         roundTo(liveOut.CoverQuality, 3),
					"cover_stability":       roundTo(liveOut.CoverStability, 3),
					"cover_temporal":        roundTo(liveOut.CoverTemporal, 3),
					"cover_portrait":        roundTo(liveOut.CoverPortrait, 3),
					"cover_exposure":        roundTo(liveOut.CoverExposure, 3),
					"cover_face":            roundTo(liveOut.CoverFace, 3),
					"cover_portrait_weight": roundTo(qualitySettings.LiveCoverPortraitWeight, 3),
					"motion_score":          adaptiveProfile.MotionScore,
					"motion_level":          adaptiveProfile.Level,
				},
			},
		}
		for idx := range result.Artifacts {
			appendWindowBindingMetadata(result.Artifacts[idx].Metadata, window)
		}
		return result
	}

	filePath := filepath.Join(outputDir, fmt.Sprintf("clip_%02d_%s.%s", task.WindowIndex, task.Format, task.Format))
	renderStartedAt := time.Now()
	renderAttemptCount := 1
	renderRetriedWithBaseWindow := false
	renderRetriedWithExpandedWindow := false
	expandedRetryAttempted := false
	expandedRetryAttemptCount := 0
	expandedRetryError := ""
	expandedRetryWindow := highlightCandidate{}
	renderErr := renderClipOutput(ctx, renderSourcePath, filePath, meta, adaptiveOptions, qualitySettings, renderWindow, task.Format)
	if renderErr != nil && task.Format == "gif" && gifLoopTune != nil && gifLoopTune.Applied {
		renderAttemptCount = 2
		retryWindow := baseAnimatedWindow
		retryErr := renderClipOutput(ctx, renderSourcePath, filePath, meta, adaptiveOptions, qualitySettings, retryWindow, task.Format)
		if retryErr == nil {
			renderWindow = retryWindow
			renderRetriedWithBaseWindow = true
			gifLoopTune.FallbackToBase = true
			gifLoopTune.FallbackReason = "tuned_window_render_failed"
			gifLoopTune.EffectiveSec = roundTo(retryWindow.EndSec-retryWindow.StartSec, 3)
			renderErr = nil
		} else {
			renderErr = fmt.Errorf("render failed with tuned window (%v), fallback also failed: %w", renderErr, retryErr)
		}
	}
	if renderErr != nil {
		var unsupported unsupportedOutputFormatError
		if errors.As(renderErr, &unsupported) {
			reason := strings.TrimSpace(unsupported.Reason)
			if reason == "" {
				reason = "format is not supported by ffmpeg runtime"
			}
			result.UnsupportedReason = reason
			return result
		}
		result.Err = fmt.Errorf("render %s clip %d failed: %w", task.Format, task.WindowIndex, renderErr)
		return result
	}
	syncFinalWindow := func() {
		if task.Format == "gif" && bundleOffsetSec > 0 {
			finalWindow = fromBundleRelativeWindow(renderWindow, bundleOffsetSec)
		} else {
			finalWindow = renderWindow
		}
	}
	syncFinalWindow()
	renderElapsedMs := time.Since(renderStartedAt).Milliseconds()
	if task.Format == "gif" {
		optimized := optimizeGIFWithGifsicle(ctx, filePath, qualitySettings, p.cfg.GIFSICLEBin)
		gifOptimization = &optimized
		gifsicleStageMs = optimized.DurationMs
	}

	sizeBytes, width, height, durationMs := readMediaOutputInfo(filePath)
	invalidReason := invalidAnimatedOutputReason(task.Format, sizeBytes)
	buildInvalidMetadata := func(reason string) map[string]interface{} {
		meta := map[string]interface{}{
			"reason":                              strings.TrimSpace(reason),
			"format":                              task.Format,
			"render_window_start_sec":             roundTo(renderWindow.StartSec, 3),
			"render_window_end_sec":               roundTo(renderWindow.EndSec, 3),
			"render_window_duration_sec":          roundTo(renderWindow.EndSec-renderWindow.StartSec, 3),
			"base_window_start_sec":               roundTo(baseAnimatedWindow.StartSec, 3),
			"base_window_end_sec":                 roundTo(baseAnimatedWindow.EndSec, 3),
			"base_window_duration_sec":            roundTo(baseAnimatedWindow.EndSec-baseAnimatedWindow.StartSec, 3),
			"render_elapsed_ms":                   renderElapsedMs,
			"render_attempt_count":                renderAttemptCount,
			"render_retried_with_base_window":     renderRetriedWithBaseWindow,
			"render_retried_with_expanded_window": renderRetriedWithExpandedWindow,
			"expanded_retry_attempted":            expandedRetryAttempted,
			"expanded_retry_attempt_count":        expandedRetryAttemptCount,
			"expanded_retry_error":                strings.TrimSpace(expandedRetryError),
			"expanded_retry_window_start_sec":     roundTo(expandedRetryWindow.StartSec, 3),
			"expanded_retry_window_end_sec":       roundTo(expandedRetryWindow.EndSec, 3),
			"expanded_retry_window_duration_sec":  roundTo(expandedRetryWindow.EndSec-expandedRetryWindow.StartSec, 3),
			"gif_loop_tune_stage_ms":              gifLoopTuneElapsedMs,
			"gifsicle_stage_ms":                   gifsicleStageMs,
			"bundle_offset_sec":                   roundTo(bundleOffsetSec, 3),
			"mezzanine_enabled":                   strings.TrimSpace(task.MezzaninePath) != "",
			"adaptive_fps":                        adaptiveProfile.FPS,
			"adaptive_width":                      adaptiveProfile.Width,
			"adaptive_duration_sec":               roundTo(adaptiveProfile.DurationSec, 3),
			"adaptive_motion_level":               adaptiveProfile.Level,
			"adaptive_long_video_downshift":       adaptiveProfile.LongVideoDownshift,
			"adaptive_stability_tier":             adaptiveProfile.StabilityTier,
			"gif_loop_tune_applied":               false,
			"gif_loop_tune_fallback_to_base":      false,
			"gif_loop_tune_fallback_reason":       "",
			"gif_loop_tune_decision_reason":       "",
			"gif_loop_tune_effective_applied":     false,
			"gif_loop_tune_effective_duration":    roundTo(renderWindow.EndSec-renderWindow.StartSec, 3),
		}
		if gifLoopTune != nil {
			effectiveDuration := renderWindow.EndSec - renderWindow.StartSec
			if gifLoopTune.EffectiveSec > 0 {
				effectiveDuration = gifLoopTune.EffectiveSec
			}
			effectiveApplied := gifLoopTune.Applied && !gifLoopTune.FallbackToBase
			meta["gif_loop_tune_applied"] = gifLoopTune.Applied
			meta["gif_loop_tune_fallback_to_base"] = gifLoopTune.FallbackToBase
			meta["gif_loop_tune_fallback_reason"] = gifLoopTune.FallbackReason
			meta["gif_loop_tune_decision_reason"] = gifLoopTune.DecisionReason
			meta["gif_loop_tune_effective_applied"] = effectiveApplied
			meta["gif_loop_tune_effective_duration"] = roundTo(effectiveDuration, 3)
		}
		return meta
	}
	if invalidReason != "" && task.Format == "gif" && gifLoopTune != nil && gifLoopTune.Applied && !renderRetriedWithBaseWindow {
		renderAttemptCount++
		retryWindow := baseAnimatedWindow
		retryStartedAt := time.Now()
		retryErr := renderClipOutput(ctx, renderSourcePath, filePath, meta, adaptiveOptions, qualitySettings, retryWindow, task.Format)
		if retryErr != nil {
			var unsupported unsupportedOutputFormatError
			if errors.As(retryErr, &unsupported) {
				reason := strings.TrimSpace(unsupported.Reason)
				if reason == "" {
					reason = "format is not supported by ffmpeg runtime"
				}
				result.UnsupportedReason = reason
				return result
			}
			result.Err = fmt.Errorf("render %s clip %d fallback after invalid output failed: %w", task.Format, task.WindowIndex, retryErr)
			return result
		}
		renderWindow = retryWindow
		renderRetriedWithBaseWindow = true
		gifLoopTune.FallbackToBase = true
		gifLoopTune.FallbackReason = "tuned_window_invalid_output"
		gifLoopTune.EffectiveSec = roundTo(retryWindow.EndSec-retryWindow.StartSec, 3)
		syncFinalWindow()
		renderElapsedMs = time.Since(retryStartedAt).Milliseconds()
		if task.Format == "gif" {
			optimized := optimizeGIFWithGifsicle(ctx, filePath, qualitySettings, p.cfg.GIFSICLEBin)
			gifOptimization = &optimized
			gifsicleStageMs = optimized.DurationMs
		}
		sizeBytes, width, height, durationMs = readMediaOutputInfo(filePath)
		invalidReason = invalidAnimatedOutputReason(task.Format, sizeBytes)
	}
	if invalidReason != "" && task.Format == "gif" {
		for expandedTry := 0; expandedTry < 2 && invalidReason != ""; expandedTry++ {
			expandedWindow, ok := buildExpandedInvalidGIFWindow(renderWindow, meta.DurationSec)
			if !ok {
				break
			}
			expandedRetryAttempted = true
			expandedRetryAttemptCount++
			expandedRetryWindow = expandedWindow
			renderAttemptCount++
			retryStartedAt := time.Now()
			retryErr := renderClipOutput(ctx, renderSourcePath, filePath, meta, adaptiveOptions, qualitySettings, expandedWindow, task.Format)
			if retryErr != nil {
				reason := strings.TrimSpace(retryErr.Error())
				if expandedRetryError == "" {
					expandedRetryError = reason
				} else {
					expandedRetryError = expandedRetryError + "; " + reason
				}
				break
			}
			renderWindow = expandedWindow
			renderRetriedWithExpandedWindow = true
			syncFinalWindow()
			renderElapsedMs = time.Since(retryStartedAt).Milliseconds()
			optimized := optimizeGIFWithGifsicle(ctx, filePath, qualitySettings, p.cfg.GIFSICLEBin)
			gifOptimization = &optimized
			gifsicleStageMs = optimized.DurationMs
			sizeBytes, width, height, durationMs = readMediaOutputInfo(filePath)
			invalidReason = invalidAnimatedOutputReason(task.Format, sizeBytes)
		}
	}
	if invalidReason != "" {
		result.InvalidReason = invalidReason
		result.InvalidMetadata = buildInvalidMetadata(invalidReason)
		return result
	}

	fileKey := buildVideoImageOutputObjectKey(prefix, task.Format, fmt.Sprintf("clip_%02d.%s", task.WindowIndex, task.Format))
	uploadStartedAt := time.Now()
	if err := uploadFileToQiniu(uploader, p.qiniu, fileKey, filePath); err != nil {
		result.Err = fmt.Errorf("upload %s clip %d failed: %w", task.Format, task.WindowIndex, err)
		return result
	}
	uploadElapsedMs := time.Since(uploadStartedAt).Milliseconds()
	thumbKey := fileKey
	uploads := []string{fileKey}
	clipMetadata := map[string]interface{}{
		"window_index":                        task.WindowIndex,
		"start_sec":                           finalWindow.StartSec,
		"end_sec":                             finalWindow.EndSec,
		"score":                               finalWindow.Score,
		"reason":                              strings.TrimSpace(finalWindow.Reason),
		"format":                              task.Format,
		"motion_score":                        adaptiveProfile.MotionScore,
		"motion_level":                        adaptiveProfile.Level,
		"adaptive_fps":                        adaptiveProfile.FPS,
		"adaptive_w":                          adaptiveProfile.Width,
		"adaptive_sec":                        roundTo(adaptiveProfile.DurationSec, 3),
		"render_elapsed_ms":                   renderElapsedMs,
		"render_actual_sec":                   roundTo(float64(renderElapsedMs)/1000.0, 3),
		"upload_elapsed_ms":                   uploadElapsedMs,
		"render_attempt_count":                renderAttemptCount,
		"render_retried_with_base_window":     renderRetriedWithBaseWindow,
		"render_retried_with_expanded_window": renderRetriedWithExpandedWindow,
		"expanded_retry_attempted":            expandedRetryAttempted,
		"expanded_retry_attempt_count":        expandedRetryAttemptCount,
		"expanded_retry_error":                strings.TrimSpace(expandedRetryError),
	}
	if strings.EqualFold(task.Format, "gif") {
		clipMetadata["predicted_size_kb"] = roundTo(task.PredictedSizeKB, 2)
		clipMetadata["predicted_render_sec"] = roundTo(task.PredictedRenderSec, 3)
		clipMetadata["render_cost_units"] = roundTo(task.RenderCostUnits, 3)
		if strings.TrimSpace(task.CostModelVersion) != "" {
			clipMetadata["cost_model_version"] = strings.TrimSpace(task.CostModelVersion)
		}
		clipMetadata["decode_stage_ms"] = nil
		clipMetadata["normalize_stage_ms"] = nil
		clipMetadata["palette_stage_ms"] = nil
		clipMetadata["gifsicle_stage_ms"] = gifsicleStageMs
		clipMetadata["loop_tune_stage_ms"] = gifLoopTuneElapsedMs
		clipMetadata["bundle_id"] = strings.TrimSpace(task.BundleID)
		clipMetadata["cache_hit"] = strings.TrimSpace(task.MezzaninePath) != ""
		clipMetadata["mezzanine_enabled"] = strings.TrimSpace(task.MezzaninePath) != ""
		clipMetadata["mezzanine_build_ms"] = task.MezzanineBuildMs
		if expandedRetryAttempted {
			clipMetadata["expanded_retry_window_start_sec"] = roundTo(expandedRetryWindow.StartSec, 3)
			clipMetadata["expanded_retry_window_end_sec"] = roundTo(expandedRetryWindow.EndSec, 3)
			clipMetadata["expanded_retry_window_duration_sec"] = roundTo(expandedRetryWindow.EndSec-expandedRetryWindow.StartSec, 3)
		}
	}
	appendWindowBindingMetadata(clipMetadata, finalWindow)
	if strings.TrimSpace(adaptiveProfile.StabilityTier) != "" {
		clipMetadata["adaptive_stability_tier"] = adaptiveProfile.StabilityTier
	}
	if adaptiveProfile.LongVideoDownshift {
		clipMetadata["adaptive_long_video_downshift"] = true
	}
	if gifLoopTune != nil {
		effectiveApplied := gifLoopTune.Applied && !gifLoopTune.FallbackToBase
		effectiveDuration := finalWindow.EndSec - finalWindow.StartSec
		if effectiveDuration < 0 {
			effectiveDuration = 0
		}
		if gifLoopTune.EffectiveSec > 0 {
			effectiveDuration = gifLoopTune.EffectiveSec
		}
		clipMetadata["gif_loop_tune"] = map[string]interface{}{
			"applied":           gifLoopTune.Applied,
			"effective_applied": effectiveApplied,
			"fallback_to_base":  gifLoopTune.FallbackToBase,
			"fallback_reason":   gifLoopTune.FallbackReason,
			"decision_reason":   gifLoopTune.DecisionReason,
			"base_start_sec":    roundTo(gifLoopTune.BaseStartSec, 3),
			"base_end_sec":      roundTo(gifLoopTune.BaseEndSec, 3),
			"tuned_start_sec":   roundTo(gifLoopTune.TunedStartSec, 3),
			"tuned_end_sec":     roundTo(gifLoopTune.TunedEndSec, 3),
			"base_score":        roundTo(gifLoopTune.BaseScore, 3),
			"best_score":        roundTo(gifLoopTune.BestScore, 3),
			"score_improvement": roundTo(gifLoopTune.Improvement, 4),
			"min_improvement":   roundTo(gifLoopTune.MinImprovement, 4),
			"duration_sec":      roundTo(gifLoopTune.DurationSec, 3),
			"effective_sec":     roundTo(effectiveDuration, 3),
			"score":             roundTo(gifLoopTune.Score, 3),
			"loop_closure":      roundTo(gifLoopTune.LoopClosure, 3),
			"base_loop":         roundTo(gifLoopTune.BaseLoop, 3),
			"best_loop":         roundTo(gifLoopTune.BestLoop, 3),
			"motion_mean":       roundTo(gifLoopTune.MotionMean, 3),
			"base_motion":       roundTo(gifLoopTune.BaseMotion, 3),
			"best_motion":       roundTo(gifLoopTune.BestMotion, 3),
			"quality_mean":      roundTo(gifLoopTune.QualityMean, 3),
			"sample_frames":     gifLoopTune.SampleFrames,
			"candidate_windows": gifLoopTune.Candidates,
		}
	}
	if gifOptimization != nil {
		clipMetadata["gif_optimization_v1"] = map[string]interface{}{
			"enabled":           gifOptimization.Enabled,
			"attempted":         gifOptimization.Attempted,
			"applied":           gifOptimization.Applied,
			"tool":              gifOptimization.Tool,
			"tool_path":         gifOptimization.ToolPath,
			"level":             gifOptimization.Level,
			"skip_below_kb":     gifOptimization.SkipBelowKB,
			"min_gain_ratio":    roundTo(gifOptimization.MinGainRatio, 4),
			"before_size_bytes": gifOptimization.BeforeSizeBytes,
			"after_size_bytes":  gifOptimization.AfterSizeBytes,
			"saved_bytes":       gifOptimization.SavedBytes,
			"saved_ratio":       roundTo(gifOptimization.SavedRatio, 6),
			"duration_ms":       gifOptimization.DurationMs,
			"reason":            gifOptimization.Reason,
			"error":             gifOptimization.Error,
		}
	}
	artifacts := []animatedArtifactPayload{
		{
			Type:       "clip",
			Key:        fileKey,
			MimeType:   mimeTypeByFormat(task.Format),
			SizeBytes:  sizeBytes,
			Width:      width,
			Height:     height,
			DurationMs: durationMs,
			Metadata:   clipMetadata,
		},
	}

	if task.Format == "mp4" {
		posterPath := filepath.Join(outputDir, fmt.Sprintf("clip_%02d_poster.jpg", task.WindowIndex))
		if err := extractPosterFrame(ctx, filePath, posterPath); err == nil {
			posterKey := buildVideoImageOutputObjectKey(prefix, task.Format, fmt.Sprintf("clip_%02d_poster.jpg", task.WindowIndex))
			if err := uploadFileToQiniu(uploader, p.qiniu, posterKey, posterPath); err == nil {
				thumbKey = posterKey
				uploads = append(uploads, posterKey)
				posterSize, posterW, posterH := readImageInfo(posterPath)
				artifacts = append(artifacts, animatedArtifactPayload{
					Type:      "poster",
					Key:       posterKey,
					MimeType:  "image/jpeg",
					SizeBytes: posterSize,
					Width:     posterW,
					Height:    posterH,
					Metadata: map[string]interface{}{
						"window_index": task.WindowIndex,
						"format":       task.Format,
						"motion_level": adaptiveProfile.Level,
					},
				})
			}
		}
	}
	for idx := range artifacts {
		appendWindowBindingMetadata(artifacts[idx].Metadata, finalWindow)
	}

	result.FileKey = fileKey
	result.ThumbKey = thumbKey
	result.Width = width
	result.Height = height
	result.SizeBytes = sizeBytes
	result.UploadedKeys = uploads
	result.Artifacts = artifacts
	return result
}

func toBundleRelativeWindow(window highlightCandidate, bundleStartSec float64) highlightCandidate {
	relative := window
	relative.StartSec = roundTo(window.StartSec-bundleStartSec, 3)
	relative.EndSec = roundTo(window.EndSec-bundleStartSec, 3)
	if relative.StartSec < 0 {
		relative.StartSec = 0
	}
	if relative.EndSec <= relative.StartSec {
		relative.EndSec = roundTo(relative.StartSec+0.2, 3)
	}
	return relative
}

func fromBundleRelativeWindow(window highlightCandidate, bundleStartSec float64) highlightCandidate {
	absolute := window
	absolute.StartSec = roundTo(window.StartSec+bundleStartSec, 3)
	absolute.EndSec = roundTo(window.EndSec+bundleStartSec, 3)
	return absolute
}

func buildExpandedInvalidGIFWindow(window highlightCandidate, sourceDurationSec float64) (highlightCandidate, bool) {
	currentDuration := window.EndSec - window.StartSec
	if currentDuration <= 0 {
		return highlightCandidate{}, false
	}
	targetDuration := currentDuration
	if targetDuration < 2.8 {
		targetDuration = 2.8
	}
	scaled := currentDuration * 1.4
	if targetDuration < scaled {
		targetDuration = scaled
	}
	if targetDuration > 3.2 {
		targetDuration = 3.2
	}
	if sourceDurationSec > 0 && targetDuration > sourceDurationSec {
		targetDuration = sourceDurationSec
	}
	if targetDuration <= currentDuration+0.05 {
		return highlightCandidate{}, false
	}

	center := (window.StartSec + window.EndSec) / 2.0
	start := center - targetDuration/2.0
	end := center + targetDuration/2.0
	if sourceDurationSec > 0 {
		if start < 0 {
			end -= start
			start = 0
		}
		if end > sourceDurationSec {
			shift := end - sourceDurationSec
			start -= shift
			end = sourceDurationSec
			if start < 0 {
				start = 0
			}
		}
	}
	if end <= start {
		return highlightCandidate{}, false
	}

	expanded := window
	expanded.StartSec = roundTo(start, 3)
	expanded.EndSec = roundTo(end, 3)
	if expanded.EndSec-expanded.StartSec <= currentDuration+0.03 {
		return highlightCandidate{}, false
	}
	return expanded, true
}

func uploadQiniuTasksConcurrently(
	ctx context.Context,
	uploader *qiniustorage.FormUploader,
	q *storage.QiniuClient,
	tasks []qiniuUploadTask,
	maxConcurrency int,
) ([]string, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}
	if maxConcurrency > len(tasks) {
		maxConcurrency = len(tasks)
	}

	g, gctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, maxConcurrency)
	successMap := make(map[string]struct{}, len(tasks))
	var mu sync.Mutex
	for _, task := range tasks {
		task := task
		g.Go(func() error {
			select {
			case sem <- struct{}{}:
			case <-gctx.Done():
				return gctx.Err()
			}
			defer func() { <-sem }()

			if err := uploadFileToQiniu(uploader, q, task.Key, task.Path); err != nil {
				return fmt.Errorf("upload %s failed: %w", task.Label, err)
			}
			mu.Lock()
			successMap[task.Key] = struct{}{}
			mu.Unlock()
			return nil
		})
	}
	err := g.Wait()
	uploaded := make([]string, 0, len(successMap))
	for _, task := range tasks {
		if _, ok := successMap[task.Key]; ok {
			uploaded = append(uploaded, task.Key)
		}
	}
	if err != nil {
		return uploaded, err
	}
	return uploaded, nil
}

func buildAnimatedEmojiTitle(collectionTitle string, windowIndex int, format string) string {
	if strings.EqualFold(format, "live") {
		return fmt.Sprintf("%s-Clip%02d-LIVE", collectionTitle, windowIndex)
	}
	return fmt.Sprintf("%s-Clip%02d-%s", collectionTitle, windowIndex, strings.ToUpper(format))
}
