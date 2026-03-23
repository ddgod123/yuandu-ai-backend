package videojobs

import "math"

const gifCandidateCostModelVersion = "gif_cost_v2"

type gifCandidateCostEstimate struct {
	PredictedSizeKB    float64
	PredictedRenderSec float64
	CostUnits          float64
	ModelVersion       string
}

func estimateGIFCandidateCost(
	meta videoProbeMeta,
	candidate highlightCandidate,
	options jobOptions,
	qualitySettings QualitySettings,
) gifCandidateCostEstimate {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	tunedOptions, adaptiveProfile := tuneAnimatedOptionsForWindow(meta, options, qualitySettings, "gif", candidate)

	width := tunedOptions.Width
	if width <= 0 {
		width = meta.Width
	}
	if width <= 0 {
		width = qualitySettings.GIFWidthClarityMedium
	}
	if width <= 0 {
		width = 720
	}

	height := meta.Height
	if height <= 0 {
		height = int(math.Round(float64(width) * 9.0 / 16.0))
	}
	if meta.Width > 0 && meta.Height > 0 && width > 0 {
		height = int(math.Round(float64(width) * (float64(meta.Height) / float64(meta.Width))))
	}
	if height <= 0 {
		height = int(math.Round(float64(width) * 9.0 / 16.0))
	}

	durationSec := candidate.EndSec - candidate.StartSec
	if durationSec <= 0 {
		durationSec = 2.4
	}
	if adaptiveProfile.DurationSec > 0 && adaptiveProfile.DurationSec < durationSec {
		durationSec = adaptiveProfile.DurationSec
	}
	if durationSec < 0.8 {
		durationSec = 0.8
	}
	if durationSec > 8.0 {
		durationSec = 8.0
	}

	fps := tunedOptions.FPS
	if fps <= 0 {
		fps = qualitySettings.GIFDefaultFPS
	}
	if fps <= 0 {
		fps = 12
	}
	if fps < 4 {
		fps = 4
	}
	if fps > 30 {
		fps = 30
	}

	maxColors := tunedOptions.MaxColors
	if maxColors <= 0 {
		maxColors = qualitySettings.GIFDefaultMaxColors
	}
	if maxColors <= 0 {
		maxColors = 128
	}
	if maxColors < 16 {
		maxColors = 16
	}
	if maxColors > 256 {
		maxColors = 256
	}

	pixels := float64(width * height)
	frameCount := float64(fps) * durationSec
	colorFactor := 0.65 + clampZeroOne(float64(maxColors)/256.0)*0.9
	motionFactor := 0.75 + clampZeroOne(adaptiveProfile.MotionScore)*0.7
	profileFactor := 1.0
	if qualitySettings.GIFProfile == QualityProfileClarity {
		profileFactor = 1.08
	} else if qualitySettings.GIFProfile == QualityProfileSize {
		profileFactor = 0.92
	}

	rawBytes := pixels * frameCount * 0.040 * colorFactor * motionFactor * profileFactor
	predictedSizeKB := maxFloat(48, rawBytes/1024.0)

	pixelMegaFrames := (pixels / 1_000_000.0) * frameCount
	predictedRenderSec := 1.2 + pixelMegaFrames*0.42*motionFactor*profileFactor
	if predictedRenderSec < 0.8 {
		predictedRenderSec = 0.8
	}

	costUnits := predictedRenderSec / 4.0
	targetSizeKB := float64(qualitySettings.GIFTargetSizeKB)
	if targetSizeKB > 0 && predictedSizeKB > targetSizeKB {
		costUnits *= 1.1
	}
	if costUnits < 0.5 {
		costUnits = 0.5
	}

	return gifCandidateCostEstimate{
		PredictedSizeKB:    predictedSizeKB,
		PredictedRenderSec: predictedRenderSec,
		CostUnits:          costUnits,
		ModelVersion:       gifCandidateCostModelVersion,
	}
}

func estimateGIFCandidateSizeKB(meta videoProbeMeta, candidate highlightCandidate, qualitySettings QualitySettings) float64 {
	estimate := estimateGIFCandidateCost(meta, candidate, jobOptions{}, qualitySettings)
	return estimate.PredictedSizeKB
}
