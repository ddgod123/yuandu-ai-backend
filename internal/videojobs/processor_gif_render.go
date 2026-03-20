package videojobs

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func renderGIFOutput(
	ctx context.Context,
	sourcePath string,
	outputPath string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	window highlightCandidate,
) error {
	startSec := window.StartSec
	durationSec := window.EndSec - window.StartSec
	if durationSec <= 0 {
		return errors.New("invalid gif clip window")
	}

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	segmentTimeoutFallbackCap := time.Duration(qualitySettings.GIFSegmentTimeoutFallbackCapSec) * time.Second
	segmentTimeoutEmergencyCap := time.Duration(qualitySettings.GIFSegmentTimeoutEmergencyCapSec) * time.Second
	segmentTimeoutLastResortCap := time.Duration(qualitySettings.GIFSegmentTimeoutLastResortCapSec) * time.Second
	maxColors := options.MaxColors
	if maxColors <= 0 {
		maxColors = qualitySettings.GIFDefaultMaxColors
		if qualitySettings.GIFProfile == QualityProfileSize && maxColors > qualitySettings.GIFRenderInitialSizeColorsCap {
			maxColors = qualitySettings.GIFRenderInitialSizeColorsCap
		}
		if qualitySettings.GIFProfile == QualityProfileClarity && maxColors < qualitySettings.GIFRenderInitialClarityColorsFloor {
			maxColors = qualitySettings.GIFRenderInitialClarityColorsFloor
		}
	}
	if maxColors < 16 {
		maxColors = 16
	}
	if maxColors > 256 {
		maxColors = 256
	}

	gifOptions := options
	if gifOptions.FPS <= 0 {
		gifOptions.FPS = qualitySettings.GIFDefaultFPS
		if qualitySettings.GIFProfile == QualityProfileSize && gifOptions.FPS > qualitySettings.GIFRenderInitialSizeFPSCap {
			gifOptions.FPS = qualitySettings.GIFRenderInitialSizeFPSCap
		}
		if qualitySettings.GIFProfile == QualityProfileClarity && gifOptions.FPS < qualitySettings.GIFRenderInitialClarityFPSFloor {
			gifOptions.FPS = qualitySettings.GIFRenderInitialClarityFPSFloor
		}
	}
	ditherMode := qualitySettings.GIFDitherMode
	targetBytes := int64(qualitySettings.GIFTargetSizeKB) * 1024
	if targetBytes < 0 {
		targetBytes = 0
	}
	segmentTimeout := chooseGIFSegmentRenderTimeout(meta, gifOptions, window, maxColors, qualitySettings)
	retryMaxAttempts := resolveGIFRenderRetryMaxAttempts(meta, qualitySettings)
	timeoutFallbackApplied := false
	emergencyFallbackApplied := false
	lastResortFallbackApplied := false

	for attempt := 0; attempt < retryMaxAttempts; attempt++ {
		args := []string{
			"-hide_banner",
			"-loglevel", "error",
			"-y",
		}
		if startSec > 0 {
			args = append(args, "-ss", formatFFmpegNumber(startSec))
		}
		args = append(args, "-i", sourcePath, "-t", formatFFmpegNumber(durationSec))
		baseFilters := buildAnimatedFilters(meta, gifOptions, "gif")
		var complex string
		if len(baseFilters) > 0 {
			complex = fmt.Sprintf(
				"[0:v]%s,split[v0][v1];[v0]palettegen=stats_mode=diff:max_colors=%d[p];[v1][p]paletteuse=dither=%s:diff_mode=rectangle[v]",
				strings.Join(baseFilters, ","),
				maxColors,
				ditherMode,
			)
		} else {
			complex = fmt.Sprintf(
				"[0:v]split[v0][v1];[v0]palettegen=stats_mode=diff:max_colors=%d[p];[v1][p]paletteuse=dither=%s:diff_mode=rectangle[v]",
				maxColors,
				ditherMode,
			)
		}
		args = append(args,
			"-filter_complex", complex,
			"-map", "[v]",
			"-an",
			"-loop", "0",
			outputPath,
		)
		out, err, timedOut := runFFmpegWithTimeout(ctx, segmentTimeout, args)
		if err != nil {
			if timedOut {
				if !timeoutFallbackApplied {
					nextOptions, nextColors, nextDither, changed := applyGIFTimeoutFallbackProfile(gifOptions, maxColors, ditherMode, meta.DurationSec, qualitySettings)
					if changed {
						gifOptions = nextOptions
						maxColors = nextColors
						ditherMode = nextDither
						timeoutFallbackApplied = true
						segmentTimeout = chooseGIFSegmentRenderTimeout(meta, gifOptions, highlightCandidate{
							StartSec: startSec,
							EndSec:   startSec + durationSec,
						}, maxColors, qualitySettings)
						if segmentTimeout > segmentTimeoutFallbackCap {
							segmentTimeout = segmentTimeoutFallbackCap
						}
						continue
					}
				}
				if !emergencyFallbackApplied {
					nextOptions, nextColors, nextDither, nextDuration, changed := applyGIFEmergencyFallbackProfile(gifOptions, maxColors, ditherMode, durationSec, qualitySettings)
					if changed {
						gifOptions = nextOptions
						maxColors = nextColors
						ditherMode = nextDither
						durationSec = nextDuration
						emergencyFallbackApplied = true
						segmentTimeout = chooseGIFSegmentRenderTimeout(meta, gifOptions, highlightCandidate{
							StartSec: startSec,
							EndSec:   startSec + durationSec,
						}, maxColors, qualitySettings)
						if segmentTimeout > segmentTimeoutEmergencyCap {
							segmentTimeout = segmentTimeoutEmergencyCap
						}
						continue
					}
				}
				if !lastResortFallbackApplied {
					nextOptions, nextColors, nextDither, nextDuration, changed := applyGIFLastResortFallbackProfile(gifOptions, maxColors, ditherMode, durationSec, qualitySettings)
					if changed {
						gifOptions = nextOptions
						maxColors = nextColors
						ditherMode = nextDither
						durationSec = nextDuration
						lastResortFallbackApplied = true
						segmentTimeout = chooseGIFSegmentRenderTimeout(meta, gifOptions, highlightCandidate{
							StartSec: startSec,
							EndSec:   startSec + durationSec,
						}, maxColors, qualitySettings)
						if segmentTimeout > segmentTimeoutLastResortCap {
							segmentTimeout = segmentTimeoutLastResortCap
						}
						continue
					}
				}
				return permanentError{err: fmt.Errorf(
					"ffmpeg gif render timeout after %s (attempt=%d): %s",
					segmentTimeout.String(),
					attempt+1,
					strings.TrimSpace(string(out)),
				)}
			}
			return fmt.Errorf("ffmpeg gif render failed: %w: %s", err, strings.TrimSpace(string(out)))
		}
		if targetBytes <= 0 {
			break
		}
		info, statErr := os.Stat(outputPath)
		if statErr == nil && info.Size() <= targetBytes {
			break
		}
		changed := false
		if maxColors > qualitySettings.GIFRenderRetryPrimaryColorsFloor {
			maxColors -= qualitySettings.GIFRenderRetryPrimaryColorsStep
			if maxColors < qualitySettings.GIFRenderRetryPrimaryColorsFloor {
				maxColors = qualitySettings.GIFRenderRetryPrimaryColorsFloor
			}
			changed = true
		} else if gifOptions.FPS > qualitySettings.GIFRenderRetryFPSFloor {
			gifOptions.FPS -= qualitySettings.GIFRenderRetryFPSStep
			if gifOptions.FPS < qualitySettings.GIFRenderRetryFPSFloor {
				gifOptions.FPS = qualitySettings.GIFRenderRetryFPSFloor
			}
			changed = true
		} else if gifOptions.Width > 0 && gifOptions.Width > qualitySettings.GIFRenderRetryWidthTrigger {
			nextWidth := int(math.Round(float64(gifOptions.Width) * qualitySettings.GIFRenderRetryWidthScale))
			if nextWidth%2 == 1 {
				nextWidth--
			}
			if nextWidth < qualitySettings.GIFRenderRetryWidthFloor {
				nextWidth = qualitySettings.GIFRenderRetryWidthFloor
			}
			if nextWidth < gifOptions.Width {
				gifOptions.Width = nextWidth
				changed = true
			}
		} else if maxColors > qualitySettings.GIFRenderRetrySecondaryColorsFloor {
			maxColors -= qualitySettings.GIFRenderRetrySecondaryColorsStep
			if maxColors < qualitySettings.GIFRenderRetrySecondaryColorsFloor {
				maxColors = qualitySettings.GIFRenderRetrySecondaryColorsFloor
			}
			changed = true
		} else if ditherMode != "none" {
			ditherMode = "none"
			changed = true
		}
		if !changed {
			break
		}
	}
	return nil
}

func resolveGIFRenderRetryMaxAttempts(meta videoProbeMeta, qualitySettings QualitySettings) int {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	maxAttempts := qualitySettings.GIFRenderRetryMaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	longSide := meta.Width
	if meta.Height > longSide {
		longSide = meta.Height
	}
	highResThreshold := qualitySettings.GIFDownshiftHighResLongSideThreshold
	if highResThreshold <= 0 {
		highResThreshold = 1600
	}

	// 长视频/高分辨率下优先缩短重试尾延迟：失败时更快进入降档参数，而不是在高参数上多次重试。
	if meta.DurationSec >= qualitySettings.GIFDurationTierLongSec || longSide >= highResThreshold {
		if maxAttempts > 3 {
			maxAttempts = 3
		}
	}
	if meta.DurationSec >= qualitySettings.GIFDurationTierUltraSec {
		if maxAttempts > 2 {
			maxAttempts = 2
		}
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	return maxAttempts
}

func chooseGIFSegmentRenderTimeout(
	meta videoProbeMeta,
	options jobOptions,
	window highlightCandidate,
	maxColors int,
	qualitySettings QualitySettings,
) time.Duration {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	_, longVideoThresholdSec, ultraVideoThresholdSec := resolveGIFDurationTierThresholds(qualitySettings)
	timeoutMin := time.Duration(qualitySettings.GIFSegmentTimeoutMinSec) * time.Second
	timeoutMax := time.Duration(qualitySettings.GIFSegmentTimeoutMaxSec) * time.Second
	durationSec := window.EndSec - window.StartSec
	if durationSec <= 0 {
		durationSec = 2.4
	}
	timeoutSec := 30.0 + durationSec*8.0

	if meta.DurationSec >= longVideoThresholdSec {
		timeoutSec += 12
	}
	if meta.DurationSec >= ultraVideoThresholdSec {
		timeoutSec += 8
	}

	targetWidth := options.Width
	if targetWidth <= 0 {
		targetWidth = meta.Width
	}
	switch {
	case targetWidth >= 1080:
		timeoutSec += 22
	case targetWidth >= 960:
		timeoutSec += 14
	case targetWidth >= 720:
		timeoutSec += 8
	}

	targetFPS := options.FPS
	if targetFPS <= 0 {
		targetFPS = 12
	}
	if targetFPS >= 16 {
		timeoutSec += 7
	}
	if targetFPS >= 20 {
		timeoutSec += 4
	}

	switch {
	case maxColors >= 192:
		timeoutSec += 6
	case maxColors >= 128:
		timeoutSec += 3
	}

	timeout := time.Duration(math.Round(timeoutSec)) * time.Second
	if timeout < timeoutMin {
		return timeoutMin
	}
	if timeout > timeoutMax {
		return timeoutMax
	}
	return timeout
}

func applyGIFTimeoutFallbackProfile(
	options jobOptions,
	maxColors int,
	ditherMode string,
	sourceDurationSec float64,
	qualitySettings QualitySettings,
) (jobOptions, int, string, bool) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	next := options
	changed := false

	fpsCap := qualitySettings.GIFTimeoutFallbackFPSCap
	widthCap := qualitySettings.GIFTimeoutFallbackWidthCap
	colorsCap := qualitySettings.GIFTimeoutFallbackColorsCap
	minWidth := qualitySettings.GIFTimeoutFallbackMinWidth
	_, _, ultraVideoThresholdSec := resolveGIFDurationTierThresholds(qualitySettings)
	if sourceDurationSec >= ultraVideoThresholdSec {
		fpsCap = qualitySettings.GIFTimeoutFallbackUltraFPSCap
		widthCap = qualitySettings.GIFTimeoutFallbackUltraWidthCap
		colorsCap = qualitySettings.GIFTimeoutFallbackUltraColorsCap
	}

	if next.FPS <= 0 || next.FPS > fpsCap {
		next.FPS = fpsCap
		changed = true
	}
	if next.Width <= 0 || next.Width > widthCap {
		next.Width = widthCap
		changed = true
	}
	if next.Width > 0 && next.Width%2 != 0 {
		next.Width--
		changed = true
	}
	if next.Width > 0 && next.Width < minWidth {
		next.Width = minWidth
		changed = true
	}

	if maxColors <= 0 || maxColors > colorsCap {
		maxColors = colorsCap
		changed = true
	}

	mode := strings.ToLower(strings.TrimSpace(ditherMode))
	if mode != "none" {
		ditherMode = "none"
		changed = true
	}

	return next, maxColors, ditherMode, changed
}

func applyGIFEmergencyFallbackProfile(
	options jobOptions,
	maxColors int,
	ditherMode string,
	durationSec float64,
	qualitySettings QualitySettings,
) (jobOptions, int, string, float64, bool) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	next := options
	changed := false

	if next.FPS <= 0 || next.FPS > qualitySettings.GIFTimeoutEmergencyFPSCap {
		next.FPS = qualitySettings.GIFTimeoutEmergencyFPSCap
		changed = true
	}
	if next.Width <= 0 || next.Width > qualitySettings.GIFTimeoutEmergencyWidthCap {
		next.Width = qualitySettings.GIFTimeoutEmergencyWidthCap
		changed = true
	}
	if next.Width > 0 && next.Width%2 != 0 {
		next.Width--
		changed = true
	}
	if next.Width > 0 && next.Width < qualitySettings.GIFTimeoutEmergencyMinWidth {
		next.Width = qualitySettings.GIFTimeoutEmergencyMinWidth
		changed = true
	}
	if maxColors <= 0 || maxColors > qualitySettings.GIFTimeoutEmergencyColorsCap {
		maxColors = qualitySettings.GIFTimeoutEmergencyColorsCap
		changed = true
	}
	mode := strings.ToLower(strings.TrimSpace(ditherMode))
	if mode != "none" {
		ditherMode = "none"
		changed = true
	}

	nextDuration := durationSec
	if nextDuration > qualitySettings.GIFTimeoutEmergencyDurationTrigger {
		nextDuration = math.Max(
			qualitySettings.GIFTimeoutEmergencyDurationMinSec,
			nextDuration*qualitySettings.GIFTimeoutEmergencyDurationScale,
		)
		changed = true
	}
	return next, maxColors, ditherMode, nextDuration, changed
}

func applyGIFLastResortFallbackProfile(
	options jobOptions,
	maxColors int,
	ditherMode string,
	durationSec float64,
	qualitySettings QualitySettings,
) (jobOptions, int, string, float64, bool) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	next := options
	changed := false

	if next.FPS <= 0 || next.FPS > qualitySettings.GIFTimeoutLastResortFPSCap {
		next.FPS = qualitySettings.GIFTimeoutLastResortFPSCap
		changed = true
	}
	if next.Width <= 0 || next.Width > qualitySettings.GIFTimeoutLastResortWidthCap {
		next.Width = qualitySettings.GIFTimeoutLastResortWidthCap
		changed = true
	}
	if next.Width > 0 && next.Width%2 != 0 {
		next.Width--
		changed = true
	}
	if next.Width > 0 && next.Width < qualitySettings.GIFTimeoutLastResortMinWidth {
		next.Width = qualitySettings.GIFTimeoutLastResortMinWidth
		changed = true
	}
	if maxColors <= 0 || maxColors > qualitySettings.GIFTimeoutLastResortColorsCap {
		maxColors = qualitySettings.GIFTimeoutLastResortColorsCap
		changed = true
	}
	if mode := strings.ToLower(strings.TrimSpace(ditherMode)); mode != "none" {
		ditherMode = "none"
		changed = true
	}

	nextDuration := durationSec
	if nextDuration > qualitySettings.GIFTimeoutLastResortDurationMaxSec {
		nextDuration = qualitySettings.GIFTimeoutLastResortDurationMaxSec
		changed = true
	}
	if nextDuration < qualitySettings.GIFTimeoutLastResortDurationMinSec {
		nextDuration = qualitySettings.GIFTimeoutLastResortDurationMinSec
		changed = true
	}

	return next, maxColors, ditherMode, nextDuration, changed
}

func runFFmpegWithTimeout(ctx context.Context, timeout time.Duration, args []string) ([]byte, error, bool) {
	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return out, nil, false
	}

	timedOut := false
	if timeout > 0 && runCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
		timedOut = true
	}
	return out, err, timedOut
}

func optimizeGIFLoopWindow(
	ctx context.Context,
	sourcePath string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	window highlightCandidate,
) (highlightCandidate, gifLoopTuningResult, error) {
	result := gifLoopTuningResult{
		BaseStartSec:  window.StartSec,
		BaseEndSec:    window.EndSec,
		TunedStartSec: window.StartSec,
		TunedEndSec:   window.EndSec,
		DurationSec:   window.EndSec - window.StartSec,
		EffectiveSec:  window.EndSec - window.StartSec,
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	if !qualitySettings.GIFLoopTuneEnabled {
		result.DecisionReason = "feature_disabled"
		return window, result, nil
	}
	durationSec := window.EndSec - window.StartSec
	if durationSec < qualitySettings.GIFLoopTuneMinEnableSec {
		result.DecisionReason = "duration_below_min_enable"
		result.MinImprovement = roundTo(qualitySettings.GIFLoopTuneMinImprovement, 4)
		return window, result, nil
	}

	sampleFPS := 6.0
	if options.FPS > 0 {
		sampleFPS = clampFloat(float64(options.FPS)*0.6, 4, 10)
	}
	maxFrames := int(math.Round(durationSec * sampleFPS))
	if maxFrames < 16 {
		maxFrames = 16
	}
	if maxFrames > 72 {
		maxFrames = 72
	}

	sampleDir, err := os.MkdirTemp("", "gif-loop-tune-*")
	if err != nil {
		return window, result, err
	}
	defer os.RemoveAll(sampleDir)

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
	}
	if window.StartSec > 0 {
		args = append(args, "-ss", formatFFmpegNumber(window.StartSec))
	}
	args = append(args,
		"-i", sourcePath,
		"-t", formatFFmpegNumber(durationSec),
		"-vf", fmt.Sprintf("fps=%s,scale=160:-1:flags=lanczos", formatFFmpegNumber(sampleFPS)),
		"-frames:v", strconv.Itoa(maxFrames),
		filepath.Join(sampleDir, "frame_%03d.jpg"),
	)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return window, result, fmt.Errorf("ffmpeg gif loop sample failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	paths, err := collectFramePaths(sampleDir, maxFrames)
	if err != nil {
		return window, result, err
	}
	if len(paths) < 6 {
		result.SampleFrames = len(paths)
		result.DecisionReason = "insufficient_sample_frames"
		return window, result, nil
	}

	samples := analyzeFrameQualityBatch(paths, minInt(qualitySettings.QualityAnalysisWorkers, 6))
	if len(samples) < 6 {
		result.SampleFrames = len(samples)
		result.DecisionReason = "insufficient_quality_samples"
		return window, result, nil
	}
	if len(samples) != len(paths) {
		// analyzeFrameQualityBatch may drop unreadable frames; keep timeline deterministic by truncating.
		if len(samples) < 6 {
			result.SampleFrames = len(samples)
			result.DecisionReason = "insufficient_quality_samples"
			return window, result, nil
		}
	}

	blurScores := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if sample.BlurScore > 0 {
			blurScores = append(blurScores, sample.BlurScore)
		}
	}
	blurThreshold := chooseBlurThreshold(blurScores, qualitySettings)
	step := durationSec / float64(maxIntValue(1, len(samples)-1))
	loopSamples := make([]gifLoopSampleFrame, 0, len(samples))
	for idx := range samples {
		samples[idx].Exposure = roundTo(computeExposureScore(samples[idx].Brightness, qualitySettings), 3)
		samples[idx].QualityScore = roundTo(computeFrameQualityScore(samples[idx], blurThreshold), 3)
		ts := window.StartSec + float64(idx)*step
		if ts > window.EndSec {
			ts = window.EndSec
		}
		loopSamples = append(loopSamples, gifLoopSampleFrame{
			TimestampSec: ts,
			Hash:         samples[idx].Hash,
			QualityScore: clampZeroOne(samples[idx].QualityScore),
		})
	}

	tunedWindow, tunedResult := selectBestGIFLoopWindowFromSamples(window, loopSamples, qualitySettings)
	tunedResult.SampleFrames = len(loopSamples)
	if tunedResult.Candidates == 0 {
		if strings.TrimSpace(tunedResult.DecisionReason) == "" {
			tunedResult.DecisionReason = "no_candidate_window"
		}
		return window, tunedResult, nil
	}
	if tunedResult.Applied {
		tunedWindow = clampWindowDuration(tunedWindow, tunedWindow.EndSec-tunedWindow.StartSec, meta.DurationSec)
		tunedResult.EffectiveSec = roundTo(tunedWindow.EndSec-tunedWindow.StartSec, 3)
		if strings.TrimSpace(tunedResult.DecisionReason) == "" {
			tunedResult.DecisionReason = "applied"
		}
	} else if strings.TrimSpace(tunedResult.DecisionReason) == "" {
		tunedResult.DecisionReason = "not_applied"
	}
	return tunedWindow, tunedResult, nil
}

func selectBestGIFLoopWindowFromSamples(window highlightCandidate, samples []gifLoopSampleFrame, qualitySettings QualitySettings) (highlightCandidate, gifLoopTuningResult) {
	result := gifLoopTuningResult{
		BaseStartSec:  window.StartSec,
		BaseEndSec:    window.EndSec,
		TunedStartSec: window.StartSec,
		TunedEndSec:   window.EndSec,
		DurationSec:   window.EndSec - window.StartSec,
		EffectiveSec:  window.EndSec - window.StartSec,
		SampleFrames:  len(samples),
	}
	if len(samples) < 6 {
		result.DecisionReason = "insufficient_samples"
		return window, result
	}
	baseDuration := window.EndSec - window.StartSec
	if baseDuration <= 0 {
		result.DecisionReason = "invalid_base_duration"
		return window, result
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	minDuration := math.Min(1.6, baseDuration*0.75)
	if minDuration < qualitySettings.GIFLoopTuneMinEnableSec {
		minDuration = qualitySettings.GIFLoopTuneMinEnableSec
	}
	maxDuration := baseDuration
	if maxDuration > 3.2 {
		maxDuration = 3.2
	}
	motionTarget := qualitySettings.GIFLoopTuneMotionTarget
	preferDuration := qualitySettings.GIFLoopTunePreferDuration
	durationSpan := clampFloat(preferDuration*0.67, 0.6, 2.4)
	minImprovement := qualitySettings.GIFLoopTuneMinImprovement

	evaluateRange := func(startIdx, endIdx int) (float64, float64, float64, float64) {
		if startIdx < 0 || endIdx >= len(samples) || endIdx <= startIdx {
			return -1, 0, 0, 0
		}
		duration := samples[endIdx].TimestampSec - samples[startIdx].TimestampSec
		if duration < minDuration || duration > maxDuration {
			return -1, 0, 0, duration
		}
		loopClosure := 1 - float64(hammingDistance64(samples[startIdx].Hash, samples[endIdx].Hash))/64.0
		loopClosure = clampZeroOne(loopClosure)
		motionMean := meanHashMotion(samples[startIdx : endIdx+1])
		motionScore := 1 - math.Abs(motionMean-motionTarget)/motionTarget
		motionScore = clampZeroOne(motionScore)
		qualityMean := clampZeroOne((samples[startIdx].QualityScore + samples[endIdx].QualityScore) / 2)
		durationScore := 1 - math.Abs(duration-preferDuration)/durationSpan
		durationScore = clampZeroOne(durationScore)
		totalScore := loopClosure*0.52 + motionScore*0.22 + qualityMean*0.16 + durationScore*0.10
		return totalScore, loopClosure, motionMean, duration
	}

	baseScore, baseLoop, baseMotion, baseDurationEval := evaluateRange(0, len(samples)-1)
	if baseScore < 0 {
		baseScore = 0
		baseLoop = 0
		baseMotion = 0
		baseDurationEval = baseDuration
	}
	result.BaseScore = roundTo(baseScore, 3)
	result.BaseLoop = roundTo(baseLoop, 3)
	result.BaseMotion = roundTo(baseMotion, 3)
	adaptiveMinImprovement := minImprovement
	if minImprovement <= 0.05 {
		if baseLoop < 0.6 {
			adaptiveMinImprovement = math.Min(adaptiveMinImprovement, 0.025)
		}
		if baseLoop < 0.5 {
			adaptiveMinImprovement = math.Min(adaptiveMinImprovement, 0.015)
		}
	}
	result.MinImprovement = roundTo(adaptiveMinImprovement, 4)
	result.Score = roundTo(baseScore, 3)
	result.LoopClosure = roundTo(baseLoop, 3)
	result.MotionMean = roundTo(baseMotion, 3)
	result.QualityMean = roundTo((samples[0].QualityScore+samples[len(samples)-1].QualityScore)/2, 3)
	result.DurationSec = roundTo(baseDurationEval, 3)

	bestScore := baseScore
	bestStart := 0
	bestEnd := len(samples) - 1
	bestLoop := baseLoop
	bestMotion := baseMotion
	bestDuration := baseDurationEval
	candidates := 0

	for i := 0; i < len(samples)-2; i++ {
		for j := i + 2; j < len(samples); j++ {
			score, loopClosure, motionMean, duration := evaluateRange(i, j)
			if score < 0 {
				continue
			}
			candidates++
			if score > bestScore {
				bestScore = score
				bestStart = i
				bestEnd = j
				bestLoop = loopClosure
				bestMotion = motionMean
				bestDuration = duration
			}
		}
	}
	result.Candidates = candidates
	if candidates == 0 {
		result.BestScore = result.BaseScore
		result.BestLoop = result.BaseLoop
		result.BestMotion = result.BaseMotion
		result.Improvement = 0
		result.DecisionReason = "no_candidate_window"
		return window, result
	}

	improvement := bestScore - baseScore
	loopGain := bestLoop - baseLoop
	result.BestScore = roundTo(bestScore, 3)
	result.BestLoop = roundTo(bestLoop, 3)
	result.BestMotion = roundTo(bestMotion, 3)
	result.Improvement = roundTo(improvement, 4)

	softApply := shouldApplyGIFLoopTuneBySoftGate(
		improvement,
		adaptiveMinImprovement,
		baseScore,
		bestScore,
		baseLoop,
		bestLoop,
		baseMotion,
		bestMotion,
		motionTarget,
	)
	if improvement < adaptiveMinImprovement && !softApply {
		switch {
		case baseLoop >= 0.82 && loopGain <= 0.05:
			result.DecisionReason = "already_loop_stable"
		case loopGain >= 0.08 && improvement > 0:
			result.DecisionReason = "loop_gain_but_score_small"
		default:
			result.DecisionReason = "improvement_below_threshold"
		}
		return window, result
	}
	tunedStart := samples[bestStart].TimestampSec
	tunedEnd := samples[bestEnd].TimestampSec
	if tunedEnd <= tunedStart {
		result.DecisionReason = "invalid_tuned_window"
		return window, result
	}
	tunedWindow := window
	tunedWindow.StartSec = tunedStart
	tunedWindow.EndSec = tunedEnd
	tunedWindow.Score = window.Score + improvement*0.08

	result.Applied = true
	result.TunedStartSec = tunedStart
	result.TunedEndSec = tunedEnd
	result.DurationSec = roundTo(bestDuration, 3)
	result.EffectiveSec = result.DurationSec
	result.Score = roundTo(bestScore, 3)
	result.LoopClosure = roundTo(bestLoop, 3)
	result.MotionMean = roundTo(bestMotion, 3)
	result.QualityMean = roundTo((samples[bestStart].QualityScore+samples[bestEnd].QualityScore)/2, 3)
	if softApply {
		result.DecisionReason = "soft_loop_closure_gain"
	} else {
		result.DecisionReason = "score_improvement_gate_pass"
	}
	return tunedWindow, result
}

func shouldApplyGIFLoopTuneBySoftGate(
	improvement float64,
	minImprovement float64,
	baseScore float64,
	bestScore float64,
	baseLoop float64,
	bestLoop float64,
	baseMotion float64,
	bestMotion float64,
	motionTarget float64,
) bool {
	// Respect explicitly strict thresholds from ops (e.g. manual roll-back / conservative mode).
	if minImprovement > 0.05 {
		return false
	}
	// Soft gate is only for near-threshold improvements and should not accept score regressions.
	if improvement < 0 {
		return false
	}
	if bestScore+1e-6 < baseScore-0.003 {
		return false
	}

	loopGain := bestLoop - baseLoop
	requiredLoopGain := 0.06
	if baseLoop < 0.6 {
		requiredLoopGain = 0.045
	}
	if baseLoop < 0.5 {
		requiredLoopGain = 0.03
	}
	if loopGain < requiredLoopGain {
		return false
	}

	softMinImprovement := clampFloat(minImprovement*0.35, 0.004, 0.02)
	if improvement < softMinImprovement {
		// Permit very small score gains only when loop closure gain is clearly significant.
		if !(loopGain >= 0.1 && improvement >= -0.001) {
			return false
		}
	}

	baseMotionDelta := math.Abs(baseMotion - motionTarget)
	bestMotionDelta := math.Abs(bestMotion - motionTarget)
	if bestMotionDelta-baseMotionDelta > 0.1 {
		return false
	}
	return true
}

func meanHashMotion(samples []gifLoopSampleFrame) float64 {
	if len(samples) <= 1 {
		return 0
	}
	total := 0.0
	count := 0
	for idx := 1; idx < len(samples); idx++ {
		total += float64(hammingDistance64(samples[idx-1].Hash, samples[idx].Hash)) / 64.0
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}
