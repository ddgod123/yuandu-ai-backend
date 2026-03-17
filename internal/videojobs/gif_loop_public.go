package videojobs

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// GIFLoopRegressionOptions defines options used by AnalyzeGIFLoopRegression.
type GIFLoopRegressionOptions struct {
	PreferWindowSec float64 `json:"prefer_window_sec"`
	UseHighlight    bool    `json:"use_highlight"`
	RenderOutputs   bool    `json:"render_outputs"`
}

// GIFLoopWindow is a normalized time window in seconds.
type GIFLoopWindow struct {
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
}

// GIFLoopWindowScore is a sampled quality score for a GIF loop window.
type GIFLoopWindowScore struct {
	SampleFrames int     `json:"sample_frames"`
	Score        float64 `json:"score"`
	LoopClosure  float64 `json:"loop_closure"`
	MotionMean   float64 `json:"motion_mean"`
	QualityMean  float64 `json:"quality_mean"`
	DurationSec  float64 `json:"duration_sec"`
}

// GIFLoopRenderMetrics is render timing/output size for a given window.
type GIFLoopRenderMetrics struct {
	Attempted  bool    `json:"attempted"`
	Success    bool    `json:"success"`
	ElapsedSec float64 `json:"elapsed_sec"`
	SizeBytes  int64   `json:"size_bytes"`
	Error      string  `json:"error,omitempty"`
}

// GIFLoopRegressionResult contains per-video regression analysis for GIF loop tuning.
type GIFLoopRegressionResult struct {
	SourcePath          string               `json:"source_path"`
	Probe               ProbeMeta            `json:"probe"`
	BaseWindow          GIFLoopWindow        `json:"base_window"`
	FinalWindow         GIFLoopWindow        `json:"final_window"`
	BaseScore           GIFLoopWindowScore   `json:"base_score"`
	FinalScore          GIFLoopWindowScore   `json:"final_score"`
	TuneApplied         bool                 `json:"tune_applied"`
	EffectiveApplied    bool                 `json:"effective_applied"`
	FallbackToBase      bool                 `json:"fallback_to_base"`
	CandidateWindows    int                  `json:"candidate_windows"`
	TuneSampleFrames    int                  `json:"tune_sample_frames"`
	TuneScore           float64              `json:"tune_score"`
	TuneLoopClosure     float64              `json:"tune_loop_closure"`
	TuneMotionMean      float64              `json:"tune_motion_mean"`
	TuneDurationSec     float64              `json:"tune_duration_sec"`
	ScoreDelta          float64              `json:"score_delta"`
	LoopClosureDelta    float64              `json:"loop_closure_delta"`
	BaseRender          GIFLoopRenderMetrics `json:"base_render"`
	FinalRender         GIFLoopRenderMetrics `json:"final_render"`
	HighlightSuggestion string               `json:"highlight_suggestion,omitempty"`
}

func normalizeGIFLoopRegressionOptions(in GIFLoopRegressionOptions) GIFLoopRegressionOptions {
	out := in
	if out.PreferWindowSec <= 0 {
		out.PreferWindowSec = 3.0
	}
	out.PreferWindowSec = clampFloat(out.PreferWindowSec, 1.2, 3.4)
	if !out.UseHighlight {
		out.UseHighlight = false
	}
	if !out.RenderOutputs {
		out.RenderOutputs = false
	}
	return out
}

// AnalyzeGIFLoopRegression analyzes one local video file and returns loop-tuning quality metrics.
func AnalyzeGIFLoopRegression(ctx context.Context, sourcePath string, qualitySettings QualitySettings, options GIFLoopRegressionOptions) (GIFLoopRegressionResult, error) {
	result := GIFLoopRegressionResult{SourcePath: strings.TrimSpace(sourcePath)}
	if result.SourcePath == "" {
		return result, fmt.Errorf("empty source path")
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	options = normalizeGIFLoopRegressionOptions(options)

	meta, err := probeVideo(ctx, result.SourcePath)
	if err != nil {
		return result, err
	}
	if err := validateProbeMeta(meta); err != nil {
		return result, err
	}
	result.Probe = ProbeMeta{DurationSec: meta.DurationSec, Width: meta.Width, Height: meta.Height, FPS: meta.FPS}

	baseWindow := resolveGIFRegressionBaseWindow(meta, options.PreferWindowSec)
	if options.UseHighlight {
		if suggestion, suggestErr := suggestHighlightWindow(ctx, result.SourcePath, meta, qualitySettings); suggestErr == nil {
			if suggestion.Selected != nil && suggestion.Selected.EndSec > suggestion.Selected.StartSec {
				baseWindow = clampWindowDuration(*suggestion.Selected, options.PreferWindowSec, meta.DurationSec)
				result.HighlightSuggestion = strings.TrimSpace(suggestion.Selected.Reason)
			}
		}
	}

	jobOpts := jobOptions{}
	adaptiveOptions, adaptiveProfile := tuneAnimatedOptionsForWindow(meta, jobOpts, qualitySettings, "gif", baseWindow)
	if adaptiveProfile.DurationSec > 0 {
		baseWindow = clampWindowDuration(baseWindow, adaptiveProfile.DurationSec, meta.DurationSec)
	}
	result.BaseWindow = GIFLoopWindow{StartSec: roundTo(baseWindow.StartSec, 3), EndSec: roundTo(baseWindow.EndSec, 3)}

	baseScore, err := analyzeGIFLoopWindowScore(ctx, result.SourcePath, baseWindow, adaptiveOptions, qualitySettings)
	if err != nil {
		return result, err
	}
	result.BaseScore = baseScore

	tunedWindow, tune, tuneErr := optimizeGIFLoopWindow(ctx, result.SourcePath, meta, adaptiveOptions, qualitySettings, baseWindow)
	if tuneErr == nil {
		result.CandidateWindows = tune.Candidates
		result.TuneSampleFrames = tune.SampleFrames
		result.TuneScore = roundTo(tune.Score, 3)
		result.TuneLoopClosure = roundTo(tune.LoopClosure, 3)
		result.TuneMotionMean = roundTo(tune.MotionMean, 3)
		result.TuneDurationSec = roundTo(tune.DurationSec, 3)
		if tune.Applied {
			result.TuneApplied = true
		}
	}

	finalWindow := baseWindow
	if result.TuneApplied {
		finalWindow = tunedWindow
	}
	result.FinalWindow = GIFLoopWindow{StartSec: roundTo(finalWindow.StartSec, 3), EndSec: roundTo(finalWindow.EndSec, 3)}

	finalScore := baseScore
	if finalWindow.StartSec != baseWindow.StartSec || finalWindow.EndSec != baseWindow.EndSec {
		if scored, scoreErr := analyzeGIFLoopWindowScore(ctx, result.SourcePath, finalWindow, adaptiveOptions, qualitySettings); scoreErr == nil {
			finalScore = scored
		}
	}
	result.FinalScore = finalScore
	result.ScoreDelta = roundTo(result.FinalScore.Score-result.BaseScore.Score, 4)
	result.LoopClosureDelta = roundTo(result.FinalScore.LoopClosure-result.BaseScore.LoopClosure, 4)

	if options.RenderOutputs {
		renderDir, err := os.MkdirTemp("", "gif-loop-regression-*")
		if err != nil {
			return result, err
		}
		defer os.RemoveAll(renderDir)

		baseOutput := filepath.Join(renderDir, "base.gif")
		result.BaseRender = renderGIFRegressionOutput(ctx, result.SourcePath, baseOutput, meta, adaptiveOptions, qualitySettings, baseWindow)

		if !result.TuneApplied {
			result.FinalRender = result.BaseRender
			result.EffectiveApplied = false
		} else {
			finalOutput := filepath.Join(renderDir, "final.gif")
			result.FinalRender = renderGIFRegressionOutput(ctx, result.SourcePath, finalOutput, meta, adaptiveOptions, qualitySettings, finalWindow)
			if result.FinalRender.Success {
				result.EffectiveApplied = true
			} else if result.BaseRender.Success {
				result.FallbackToBase = true
				result.FinalRender = result.BaseRender
				result.EffectiveApplied = false
			}
		}
	} else {
		if result.TuneApplied {
			result.EffectiveApplied = true
		}
	}

	return result, nil
}

func resolveGIFRegressionBaseWindow(meta videoProbeMeta, preferWindowSec float64) highlightCandidate {
	preferWindowSec = clampFloat(preferWindowSec, 1.2, 3.4)
	if meta.DurationSec <= 0 {
		return highlightCandidate{StartSec: 0, EndSec: preferWindowSec, Score: 0.6, Reason: "regression_default"}
	}
	duration := preferWindowSec
	if duration > meta.DurationSec {
		duration = meta.DurationSec
	}
	if duration < 0.8 {
		duration = meta.DurationSec
	}
	start := (meta.DurationSec - duration) / 2
	if start < 0 {
		start = 0
	}
	end := start + duration
	if end > meta.DurationSec {
		end = meta.DurationSec
		start = math.Max(0, end-duration)
	}
	return highlightCandidate{StartSec: roundTo(start, 3), EndSec: roundTo(end, 3), Score: 0.7, Reason: "regression_center_window"}
}

func analyzeGIFLoopWindowScore(
	ctx context.Context,
	sourcePath string,
	window highlightCandidate,
	options jobOptions,
	qualitySettings QualitySettings,
) (GIFLoopWindowScore, error) {
	durationSec := window.EndSec - window.StartSec
	if durationSec <= 0 {
		return GIFLoopWindowScore{}, fmt.Errorf("invalid gif loop window")
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

	sampleDir, err := os.MkdirTemp("", "gif-loop-score-*")
	if err != nil {
		return GIFLoopWindowScore{}, err
	}
	defer os.RemoveAll(sampleDir)

	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
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
		return GIFLoopWindowScore{}, fmt.Errorf("ffmpeg gif loop score sample failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	paths, err := collectFramePaths(sampleDir, maxFrames)
	if err != nil {
		return GIFLoopWindowScore{}, err
	}
	if len(paths) < 6 {
		return GIFLoopWindowScore{SampleFrames: len(paths), DurationSec: roundTo(durationSec, 3)}, nil
	}

	samples := analyzeFrameQualityBatch(paths, minInt(qualitySettings.QualityAnalysisWorkers, 6))
	if len(samples) < 6 {
		return GIFLoopWindowScore{SampleFrames: len(samples), DurationSec: roundTo(durationSec, 3)}, nil
	}

	blurScores := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if sample.BlurScore > 0 {
			blurScores = append(blurScores, sample.BlurScore)
		}
	}
	blurThreshold := chooseBlurThreshold(blurScores, qualitySettings)
	loopSamples := make([]gifLoopSampleFrame, 0, len(samples))
	step := durationSec / float64(maxIntValue(1, len(samples)-1))
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

	score, loopClosure, motionMean, duration, qualityMean := evaluateGIFLoopScoreRange(loopSamples, 0, len(loopSamples)-1, qualitySettings)
	if score < 0 {
		score = 0
	}
	return GIFLoopWindowScore{
		SampleFrames: len(loopSamples),
		Score:        roundTo(score, 3),
		LoopClosure:  roundTo(loopClosure, 3),
		MotionMean:   roundTo(motionMean, 3),
		QualityMean:  roundTo(qualityMean, 3),
		DurationSec:  roundTo(duration, 3),
	}, nil
}

func evaluateGIFLoopScoreRange(samples []gifLoopSampleFrame, startIdx, endIdx int, qualitySettings QualitySettings) (float64, float64, float64, float64, float64) {
	if startIdx < 0 || endIdx >= len(samples) || endIdx <= startIdx {
		return -1, 0, 0, 0, 0
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	duration := samples[endIdx].TimestampSec - samples[startIdx].TimestampSec
	if duration <= 0 {
		return -1, 0, 0, duration, 0
	}
	loopClosure := 1 - float64(hammingDistance64(samples[startIdx].Hash, samples[endIdx].Hash))/64.0
	loopClosure = clampZeroOne(loopClosure)
	motionMean := meanHashMotion(samples[startIdx : endIdx+1])
	motionTarget := qualitySettings.GIFLoopTuneMotionTarget
	motionScore := 1 - math.Abs(motionMean-motionTarget)/motionTarget
	motionScore = clampZeroOne(motionScore)
	qualityMean := clampZeroOne((samples[startIdx].QualityScore + samples[endIdx].QualityScore) / 2)
	preferDuration := qualitySettings.GIFLoopTunePreferDuration
	durationSpan := clampFloat(preferDuration*0.67, 0.6, 2.4)
	durationScore := 1 - math.Abs(duration-preferDuration)/durationSpan
	durationScore = clampZeroOne(durationScore)
	totalScore := loopClosure*0.52 + motionScore*0.22 + qualityMean*0.16 + durationScore*0.10
	return totalScore, loopClosure, motionMean, duration, qualityMean
}

func renderGIFRegressionOutput(
	ctx context.Context,
	sourcePath string,
	outputPath string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	window highlightCandidate,
) GIFLoopRenderMetrics {
	start := time.Now()
	metrics := GIFLoopRenderMetrics{Attempted: true}
	if err := renderGIFOutput(ctx, sourcePath, outputPath, meta, options, qualitySettings, window); err != nil {
		metrics.Error = err.Error()
		metrics.ElapsedSec = roundTo(time.Since(start).Seconds(), 3)
		return metrics
	}
	info, statErr := os.Stat(outputPath)
	if statErr == nil {
		metrics.SizeBytes = info.Size()
	}
	metrics.Success = true
	metrics.ElapsedSec = roundTo(time.Since(start).Seconds(), 3)
	return metrics
}
