package videojobs

import (
	"math"
	"sort"
)

func optimizeFramePathsForQuality(paths []string, maxStatic int, qualitySettings QualitySettings) ([]string, frameQualityReport) {
	report := frameQualityReport{
		TotalFrames:     len(paths),
		SelectorVersion: "v1_scene_ranker",
	}
	if len(paths) == 0 {
		return nil, report
	}
	if maxStatic <= 0 {
		maxStatic = 24
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	samples := analyzeFrameQualityBatch(paths, qualitySettings.QualityAnalysisWorkers)
	if len(samples) == 0 {
		fallback := paths
		if len(fallback) > maxStatic {
			fallback = fallback[:maxStatic]
		}
		report.KeptFrames = len(fallback)
		report.FallbackApplied = true
		report.KeptSample = pickFramePathSample(fallback, 6)
		return fallback, report
	}

	blurScores := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if sample.BlurScore > 0 {
			blurScores = append(blurScores, sample.BlurScore)
		}
	}

	blurThreshold := chooseBlurThreshold(blurScores, qualitySettings)
	report.BlurThreshold = roundTo(blurThreshold, 2)

	sceneCutThreshold := chooseSceneCutThreshold(samples)
	report.SceneCutThreshold = roundTo(sceneCutThreshold, 2)
	assignSceneAndMotionScores(samples, sceneCutThreshold)

	sceneIDs := map[int]struct{}{}
	for idx := range samples {
		samples[idx].Exposure = roundTo(computeExposureScore(samples[idx].Brightness, qualitySettings), 3)
		samples[idx].QualityScore = roundTo(computeFrameQualityScore(samples[idx], blurThreshold), 3)
		sceneIDs[samples[idx].SceneID] = struct{}{}
	}
	report.SceneCount = len(sceneIDs)

	selected := make([]frameQualitySample, 0, len(samples))
	rejected := make([]frameQualitySample, 0, len(samples))
	for _, sample := range samples {
		if qualitySettings.StillMinWidth > 0 && sample.Width > 0 && sample.Width < qualitySettings.StillMinWidth {
			report.RejectedResolution++
			rejected = append(rejected, sample)
			continue
		}
		if qualitySettings.StillMinHeight > 0 && sample.Height > 0 && sample.Height < qualitySettings.StillMinHeight {
			report.RejectedResolution++
			rejected = append(rejected, sample)
			continue
		}
		if sample.Brightness < qualitySettings.MinBrightness || sample.Brightness > qualitySettings.MaxBrightness {
			report.RejectedBrightness++
			rejected = append(rejected, sample)
			continue
		}
		if sample.Exposure < qualitySettings.StillMinExposureScore {
			report.RejectedExposure++
			rejected = append(rejected, sample)
			continue
		}
		if sample.BlurScore < qualitySettings.StillMinBlurScore {
			report.RejectedStillBlurGate++
			rejected = append(rejected, sample)
			continue
		}
		if sample.BlurScore < blurThreshold {
			report.RejectedBlur++
			rejected = append(rejected, sample)
			continue
		}
		if hasNearDuplicate(selected, sample.Hash, qualitySettings.DuplicateHammingThreshold, qualitySettings.DuplicateBacktrackFrames) {
			report.RejectedNearDuplicate++
			rejected = append(rejected, sample)
			continue
		}
		selected = append(selected, sample)
	}

	if len(selected) > 0 {
		selected = rankFrameCandidatesByScene(selected, maxStatic, qualitySettings)
	}

	minKeep := qualitySettings.MinKeepBase
	if limit := int(math.Round(float64(len(samples)) * qualitySettings.MinKeepRatio)); limit > minKeep {
		minKeep = limit
	}
	if minKeep > maxStatic {
		minKeep = maxStatic
	}
	if minKeep <= 0 {
		minKeep = 1
	}

	if len(selected) < minKeep {
		report.FallbackApplied = true
		sort.SliceStable(rejected, func(i, j int) bool {
			if rejected[i].QualityScore == rejected[j].QualityScore {
				return rejected[i].Index < rejected[j].Index
			}
			return rejected[i].QualityScore > rejected[j].QualityScore
		})
		for _, sample := range rejected {
			if !passesStillHardQualityGate(sample, qualitySettings) {
				continue
			}
			if sample.Brightness < qualitySettings.MinBrightness || sample.Brightness > qualitySettings.MaxBrightness {
				continue
			}
			if sample.BlurScore < blurThreshold*qualitySettings.FallbackBlurRelaxFactor {
				continue
			}
			if hasNearDuplicate(selected, sample.Hash, qualitySettings.FallbackHammingThreshold, qualitySettings.DuplicateBacktrackFrames) {
				continue
			}
			selected = append(selected, sample)
			if len(selected) >= minKeep || len(selected) >= maxStatic {
				break
			}
		}
		selected = rankFrameCandidatesByScene(selected, maxStatic, qualitySettings)
	}

	if len(selected) == 0 {
		report.FallbackApplied = true
		fallbackCandidates := append([]frameQualitySample{}, samples...)
		sort.SliceStable(fallbackCandidates, func(i, j int) bool {
			if fallbackCandidates[i].QualityScore == fallbackCandidates[j].QualityScore {
				return fallbackCandidates[i].Index < fallbackCandidates[j].Index
			}
			return fallbackCandidates[i].QualityScore > fallbackCandidates[j].QualityScore
		})
		hardFiltered := make([]string, 0, maxStatic)
		for _, sample := range fallbackCandidates {
			if sample.Brightness < qualitySettings.MinBrightness || sample.Brightness > qualitySettings.MaxBrightness {
				continue
			}
			if !passesStillHardQualityGate(sample, qualitySettings) {
				continue
			}
			hardFiltered = append(hardFiltered, sample.Path)
			if len(hardFiltered) >= maxStatic {
				break
			}
		}
		if len(hardFiltered) > 0 {
			report.KeptFrames = len(hardFiltered)
			report.KeptSample = pickFramePathSample(hardFiltered, 6)
			return hardFiltered, report
		}
		fallback := paths
		if len(fallback) > maxStatic {
			fallback = fallback[:maxStatic]
		}
		report.KeptFrames = len(fallback)
		report.KeptSample = pickFramePathSample(fallback, 6)
		return fallback, report
	}

	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].QualityScore == selected[j].QualityScore {
			return selected[i].Index < selected[j].Index
		}
		return selected[i].QualityScore > selected[j].QualityScore
	})

	out := make([]string, 0, len(selected))
	totalScore := 0.0
	for _, sample := range selected {
		out = append(out, sample.Path)
		totalScore += sample.QualityScore
		if len(out) >= maxStatic {
			break
		}
	}

	report.KeptFrames = len(out)
	if report.KeptFrames > 0 {
		report.AvgKeptScore = roundTo(totalScore/float64(report.KeptFrames), 3)
	}
	report.KeptSample = pickFramePathSample(out, 6)
	return out, report
}
