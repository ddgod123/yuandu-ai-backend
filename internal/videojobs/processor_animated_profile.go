package videojobs

import (
	"math"
	"strings"
)

func tuneAnimatedOptionsForWindow(
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	format string,
	window highlightCandidate,
) (jobOptions, animatedAdaptiveProfile) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	profile := animatedAdaptiveProfile{
		MotionScore: estimateWindowMotionScore(window),
		Level:       "medium",
		DurationSec: window.EndSec - window.StartSec,
	}
	lowMotionThreshold := 0.30
	highMotionThreshold := 0.64
	if strings.EqualFold(format, "gif") {
		lowMotionThreshold = qualitySettings.GIFMotionLowScoreThreshold
		highMotionThreshold = qualitySettings.GIFMotionHighScoreThreshold
	}
	if profile.MotionScore < lowMotionThreshold {
		profile.Level = "low"
	} else if profile.MotionScore > highMotionThreshold {
		profile.Level = "high"
	}

	tuned := options
	formatProfile := QualityProfileClarity
	switch format {
	case "gif":
		formatProfile = qualitySettings.GIFProfile
	case "webp":
		formatProfile = qualitySettings.WebPProfile
	case "live":
		formatProfile = qualitySettings.LiveProfile
	case "mp4":
		formatProfile = qualitySettings.LiveProfile
	}

	targetFPS := tuned.FPS
	if targetFPS <= 0 {
		switch format {
		case "gif":
			targetFPS = qualitySettings.GIFDefaultFPS
		case "webp", "mp4", "live":
			targetFPS = 12
		}
	}
	if strings.EqualFold(format, "gif") {
		switch profile.Level {
		case "low":
			targetFPS += qualitySettings.GIFMotionLowFPSDelta
		case "high":
			targetFPS += qualitySettings.GIFMotionHighFPSDelta
		}
	} else {
		switch profile.Level {
		case "low":
			targetFPS -= 2
		case "high":
			targetFPS += 2
		}
	}

	sourceFPSCap := 0
	if meta.FPS > 0 {
		sourceFPSCap = int(math.Round(meta.FPS))
		if sourceFPSCap < 1 {
			sourceFPSCap = 1
		}
	}
	switch format {
	case "gif":
		minFPS := qualitySettings.GIFAdaptiveFPSMin
		maxFPS := qualitySettings.GIFAdaptiveFPSMax
		if sourceFPSCap > 0 && sourceFPSCap < minFPS {
			minFPS = sourceFPSCap
		}
		if minFPS < 2 {
			minFPS = 2
		}
		if maxFPS < minFPS {
			maxFPS = minFPS
		}
		if targetFPS < minFPS {
			targetFPS = minFPS
		}
		if targetFPS > maxFPS {
			targetFPS = maxFPS
		}
	case "webp":
		minFPS := 6
		if sourceFPSCap > 0 && sourceFPSCap < minFPS {
			minFPS = sourceFPSCap
		}
		if minFPS < 2 {
			minFPS = 2
		}
		if targetFPS < 6 {
			targetFPS = minFPS
		}
		if targetFPS > 18 {
			targetFPS = 18
		}
	case "mp4", "live":
		minFPS := 8
		if sourceFPSCap > 0 && sourceFPSCap < minFPS {
			minFPS = sourceFPSCap
		}
		if minFPS < 4 {
			minFPS = 4
		}
		if targetFPS < 8 {
			targetFPS = minFPS
		}
		if targetFPS > 24 {
			targetFPS = 24
		}
	}
	if sourceFPSCap > 0 && targetFPS > sourceFPSCap {
		targetFPS = sourceFPSCap
	}
	if targetFPS > 0 {
		tuned.FPS = targetFPS
		profile.FPS = targetFPS
	}

	if tuned.Width <= 0 {
		switch format {
		case "gif":
			if formatProfile == QualityProfileSize {
				switch profile.Level {
				case "low":
					tuned.Width = qualitySettings.GIFWidthSizeLow
				case "high":
					tuned.Width = qualitySettings.GIFWidthSizeHigh
				default:
					tuned.Width = qualitySettings.GIFWidthSizeMedium
				}
			} else {
				switch profile.Level {
				case "low":
					tuned.Width = qualitySettings.GIFWidthClarityLow
				case "high":
					tuned.Width = qualitySettings.GIFWidthClarityHigh
				default:
					tuned.Width = qualitySettings.GIFWidthClarityMedium
				}
			}
		case "webp":
			if formatProfile == QualityProfileSize {
				switch profile.Level {
				case "low":
					tuned.Width = 640
				case "high":
					tuned.Width = 768
				default:
					tuned.Width = 720
				}
			} else {
				switch profile.Level {
				case "low":
					tuned.Width = 720
				case "high":
					tuned.Width = 1080
				default:
					tuned.Width = 960
				}
			}
		case "mp4":
			if formatProfile == QualityProfileSize {
				tuned.Width = 960
			}
		}
	}
	if tuned.Width > 0 && meta.Width > 0 && tuned.Width > meta.Width {
		tuned.Width = meta.Width
	}
	if tuned.Width > 0 && (strings.EqualFold(format, "mp4") || strings.EqualFold(format, "live")) {
		if tuned.Width%2 != 0 {
			tuned.Width--
		}
		if tuned.Width < 2 {
			tuned.Width = 2
		}
	}
	profile.Width = tuned.Width

	if format == "gif" {
		targetColors := tuned.MaxColors
		if targetColors <= 0 {
			if formatProfile == QualityProfileSize {
				switch profile.Level {
				case "low":
					targetColors = qualitySettings.GIFColorsSizeLow
				case "high":
					targetColors = qualitySettings.GIFColorsSizeHigh
				default:
					targetColors = qualitySettings.GIFColorsSizeMedium
				}
			} else {
				switch profile.Level {
				case "low":
					targetColors = qualitySettings.GIFColorsClarityLow
				case "high":
					targetColors = qualitySettings.GIFColorsClarityHigh
				default:
					targetColors = qualitySettings.GIFColorsClarityMedium
				}
			}
		}
		if targetColors < 16 {
			targetColors = 16
		}
		if targetColors > 256 {
			targetColors = 256
		}
		tuned.MaxColors = targetColors
		profile.MaxColors = targetColors
	}

	if format == "gif" || format == "webp" || format == "mp4" {
		if strings.EqualFold(format, "gif") {
			switch profile.Level {
			case "low":
				profile.DurationSec = qualitySettings.GIFDurationLowSec
			case "high":
				profile.DurationSec = qualitySettings.GIFDurationHighSec
			default:
				profile.DurationSec = qualitySettings.GIFDurationMediumSec
			}
		} else {
			switch profile.Level {
			case "low":
				profile.DurationSec = 2.0
			case "high":
				profile.DurationSec = 2.8
			default:
				profile.DurationSec = 2.4
			}
		}
		sizeProfileDurationMax := 2.4
		if strings.EqualFold(format, "gif") {
			sizeProfileDurationMax = qualitySettings.GIFDurationSizeProfileMaxSec
		}
		if formatProfile == QualityProfileSize && profile.DurationSec > sizeProfileDurationMax {
			profile.DurationSec = sizeProfileDurationMax
		}
		if windowDuration := window.EndSec - window.StartSec; windowDuration > 0 && profile.DurationSec > windowDuration {
			profile.DurationSec = windowDuration
		}
	}
	if format == "live" {
		profile.DurationSec = 3.0
	}

	applyLongVideoStabilityCaps(meta, format, qualitySettings, &tuned, &profile)
	if strings.EqualFold(format, "gif") {
		if tuned.FPS > 0 {
			profile.FPS = tuned.FPS
		}
		if tuned.Width > 0 {
			profile.Width = tuned.Width
		}
		if tuned.MaxColors > 0 {
			profile.MaxColors = tuned.MaxColors
		}
	}

	return tuned, profile
}

func applyLongVideoStabilityCaps(
	meta videoProbeMeta,
	format string,
	qualitySettings QualitySettings,
	tuned *jobOptions,
	profile *animatedAdaptiveProfile,
) {
	if tuned == nil || profile == nil || !strings.EqualFold(format, "gif") {
		return
	}

	sourceDuration := meta.DurationSec
	longSide := meta.Width
	if meta.Height > longSide {
		longSide = meta.Height
	}

	mediumVideoThresholdSec, longVideoThresholdSec, ultraVideoThresholdSec := resolveGIFDurationTierThresholds(qualitySettings)

	// 触发条件：
	// 1) 中长视频（>= 60s）自动降档；
	// 2) 高分辨率源（长边 >= 1600）自动降档；
	// 3) 短视频中若长边极大（>=1800）且时长>=45s，也提前降档。
	highResLongSideThreshold := qualitySettings.GIFDownshiftHighResLongSideThreshold
	earlyDurationThreshold := qualitySettings.GIFDownshiftEarlyDurationSec
	earlyLongSideThreshold := qualitySettings.GIFDownshiftEarlyLongSideThreshold

	if sourceDuration < mediumVideoThresholdSec && longSide < highResLongSideThreshold {
		if !(sourceDuration >= earlyDurationThreshold && longSide >= earlyLongSideThreshold) {
			return
		}
	}

	fpsCap := qualitySettings.GIFDownshiftMediumFPSCap
	widthCap := qualitySettings.GIFDownshiftMediumWidthCap
	colorCap := qualitySettings.GIFDownshiftMediumColorsCap
	durationCap := qualitySettings.GIFDownshiftMediumDurationCapSec
	tier := "medium_long"

	switch {
	case sourceDuration >= ultraVideoThresholdSec:
		fpsCap = qualitySettings.GIFDownshiftUltraFPSCap
		widthCap = qualitySettings.GIFDownshiftUltraWidthCap
		colorCap = qualitySettings.GIFDownshiftUltraColorsCap
		durationCap = qualitySettings.GIFDownshiftUltraDurationCapSec
		tier = "ultra_long"
	case sourceDuration >= longVideoThresholdSec:
		fpsCap = qualitySettings.GIFDownshiftLongFPSCap
		widthCap = qualitySettings.GIFDownshiftLongWidthCap
		colorCap = qualitySettings.GIFDownshiftLongColorsCap
		durationCap = qualitySettings.GIFDownshiftLongDurationCapSec
		tier = "long"
	case sourceDuration >= mediumVideoThresholdSec || longSide >= highResLongSideThreshold:
		fpsCap = qualitySettings.GIFDownshiftMediumFPSCap
		widthCap = qualitySettings.GIFDownshiftMediumWidthCap
		colorCap = qualitySettings.GIFDownshiftMediumColorsCap
		durationCap = qualitySettings.GIFDownshiftMediumDurationCapSec
		tier = "medium_long"
	case sourceDuration >= earlyDurationThreshold && longSide >= earlyLongSideThreshold:
		fpsCap = qualitySettings.GIFDownshiftHighResFPSCap
		widthCap = qualitySettings.GIFDownshiftHighResWidthCap
		colorCap = qualitySettings.GIFDownshiftHighResColorsCap
		durationCap = qualitySettings.GIFDownshiftHighResDurationCapSec
		tier = "high_res_stability"
	default:
		return
	}

	changed := false
	if tuned.FPS <= 0 || tuned.FPS > fpsCap {
		tuned.FPS = fpsCap
		changed = true
	}
	if tuned.Width <= 0 || tuned.Width > widthCap {
		tuned.Width = widthCap
		changed = true
	}
	if tuned.MaxColors <= 0 || tuned.MaxColors > colorCap {
		tuned.MaxColors = colorCap
		changed = true
	}
	if profile.DurationSec > 0 && profile.DurationSec > durationCap {
		profile.DurationSec = durationCap
		changed = true
	}

	profile.StabilityTier = tier
	profile.LongVideoDownshift = changed
}

func estimateWindowMotionScore(window highlightCandidate) float64 {
	score := math.Max(window.SceneScore, window.Score)
	reason := strings.ToLower(strings.TrimSpace(window.Reason))
	if (reason == "single_window" || reason == "fallback_uniform") && window.SceneScore <= 0 {
		score = 0.45
	}
	if score <= 0 {
		score = 0.45
	}
	if score > 1 {
		score = 1
	}
	return roundTo(score, 3)
}
