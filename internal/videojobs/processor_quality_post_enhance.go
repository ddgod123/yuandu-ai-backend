package videojobs

import (
	"math"
	"os"
	"sort"
	"strings"
)

func rerankEnhancedFramePaths(paths []string, qualitySettings QualitySettings, guidance imageAI2Guidance) ([]string, frameQualityReport) {
	report := frameQualityReport{
		TotalFrames:     len(paths),
		SelectorVersion: "v3_post_enhance_ranker",
		ScoringFormula:  resolveFrameQualityScoringFormula(guidance),
		ScoringMode:     "post_enhance_weighted_v1",
	}
	if len(guidance.QualityWeights) > 0 {
		report.ScoringWeights = guidance.QualityWeights
		report.RiskFlags = append([]string{}, guidance.RiskFlags...)
		report.MaxBlurTolerance = guidance.MaxBlurTolerance
	}
	report.SelectionPolicy = resolveFrameSelectionPolicy(guidance)
	if len(paths) == 0 {
		return nil, report
	}

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	samples := analyzeFrameQualityBatch(paths, qualitySettings.QualityAnalysisWorkers)
	if len(samples) == 0 {
		fallback := append([]string{}, paths...)
		report.KeptFrames = len(fallback)
		report.KeptSample = pickFramePathSample(fallback, 6)
		report.FallbackApplied = true
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
	breakdownByPath := map[string]frameQualityScoreBreakdown{}
	for idx := range samples {
		samples[idx].Exposure = roundTo(computeExposureScore(samples[idx].Brightness, qualitySettings), 3)
		breakdown := computeFrameQualityScoreBreakdownWithAI2Weights(samples[idx], blurThreshold, guidance)
		samples[idx].QualityScore = roundTo(breakdown.FinalScore, 3)
		key := strings.TrimSpace(samples[idx].Path)
		if key != "" {
			breakdownByPath[key] = breakdown
		}
		sceneIDs[samples[idx].SceneID] = struct{}{}
	}
	report.SceneCount = len(sceneIDs)

	orderedSamples := reorderSamplesBySelectionPolicyWithoutDrop(samples, report.SelectionPolicy)
	out := make([]string, 0, len(orderedSamples))
	totalScore := 0.0
	decisionByPath := map[string]string{}
	rejectReasonByPath := map[string]string{}
	for _, sample := range orderedSamples {
		key := strings.TrimSpace(sample.Path)
		if key == "" {
			continue
		}
		out = append(out, key)
		decisionByPath[key] = "kept"
		totalScore += sample.QualityScore
	}

	report.KeptFrames = len(out)
	if report.KeptFrames > 0 {
		report.AvgKeptScore = roundTo(totalScore/float64(report.KeptFrames), 3)
	}
	report.KeptSample = pickFramePathSample(out, 6)
	report.CandidateScores = buildFrameQualityCandidateScores(
		orderedSamples,
		breakdownByPath,
		decisionByPath,
		rejectReasonByPath,
		guidance,
		blurThreshold,
		28,
	)
	return out, report
}

func reorderSamplesBySelectionPolicyWithoutDrop(samples []frameQualitySample, selectionPolicy string) []frameQualitySample {
	if len(samples) == 0 {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(selectionPolicy))
	switch normalized {
	case "ai2_global_quality_first", "global_quality_first":
		out := append([]frameQualitySample{}, samples...)
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].QualityScore == out[j].QualityScore {
				return out[i].Index < out[j].Index
			}
			return out[i].QualityScore > out[j].QualityScore
		})
		return out
	default:
		return sceneDiversityOrderKeepAll(samples)
	}
}

func sceneDiversityOrderKeepAll(samples []frameQualitySample) []frameQualitySample {
	if len(samples) == 0 {
		return nil
	}
	byScene := map[int][]frameQualitySample{}
	sceneOrder := make([]int, 0)
	seenScene := map[int]struct{}{}
	for _, item := range samples {
		sceneID := item.SceneID
		byScene[sceneID] = append(byScene[sceneID], item)
		if _, ok := seenScene[sceneID]; !ok {
			seenScene[sceneID] = struct{}{}
			sceneOrder = append(sceneOrder, sceneID)
		}
	}
	for sceneID := range byScene {
		rows := byScene[sceneID]
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].QualityScore == rows[j].QualityScore {
				return rows[i].Index < rows[j].Index
			}
			return rows[i].QualityScore > rows[j].QualityScore
		})
		byScene[sceneID] = rows
	}

	out := make([]frameQualitySample, 0, len(samples))
	cursor := map[int]int{}
	remaining := len(samples)
	for remaining > 0 {
		progress := false
		for _, sceneID := range sceneOrder {
			rows := byScene[sceneID]
			idx := cursor[sceneID]
			if idx >= len(rows) {
				continue
			}
			out = append(out, rows[idx])
			cursor[sceneID] = idx + 1
			remaining--
			progress = true
		}
		if !progress {
			break
		}
	}
	return out
}

func applyPNGFinalQualityGuards(
	paths []string,
	qualitySettings QualitySettings,
	guidance imageAI2Guidance,
) ([]string, map[string]interface{}) {
	report := map[string]interface{}{
		"schema_version": "png_final_quality_guard_v1",
	}
	if len(paths) == 0 {
		report["status"] = "no_frames"
		return paths, report
	}

	enabled := parseEnvBool("PNG_FINAL_QUALITY_GUARD_ENABLED", true)
	report["enabled"] = enabled
	if !enabled {
		report["status"] = "disabled"
		return paths, report
	}

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	samples := analyzeFrameQualityBatch(paths, qualitySettings.QualityAnalysisWorkers)
	if len(samples) == 0 {
		report["status"] = "analysis_failed"
		return paths, report
	}

	sampleByPath := make(map[string]frameQualitySample, len(samples))
	blurScores := make([]float64, 0, len(samples))
	for _, sample := range samples {
		sample.Exposure = roundTo(computeExposureScore(sample.Brightness, qualitySettings), 3)
		key := strings.TrimSpace(sample.Path)
		if key == "" {
			continue
		}
		sampleByPath[key] = sample
		if sample.BlurScore > 0 {
			blurScores = append(blurScores, sample.BlurScore)
		}
	}
	blurThreshold := chooseBlurThreshold(blurScores, qualitySettings)
	hardBlurFloor := maxFloat(qualitySettings.StillMinBlurScore, blurThreshold*0.85)
	report["blur_threshold"] = roundTo(blurThreshold, 3)
	report["hard_blur_floor"] = roundTo(hardBlurFloor, 3)

	requiredMin := resolvePNGFinalGuardRequiredMin(len(paths))
	report["required_min_keep"] = requiredMin
	report["input_count"] = len(paths)

	kept := make([]string, 0, len(paths))
	rejectBrightness := 0
	rejectExposure := 0
	rejectBlur := 0
	rejectWatermark := 0
	rejectMissingSample := 0

	for _, framePath := range paths {
		key := strings.TrimSpace(framePath)
		sample, ok := sampleByPath[key]
		if !ok {
			rejectMissingSample++
			continue
		}
		if sample.Brightness < qualitySettings.MinBrightness || sample.Brightness > qualitySettings.MaxBrightness {
			rejectBrightness++
			continue
		}
		if sample.Exposure < qualitySettings.StillMinExposureScore {
			rejectExposure++
			continue
		}
		if sample.BlurScore < hardBlurFloor {
			rejectBlur++
			continue
		}
		if shouldRejectForWatermarkRisk(sample, blurThreshold, guidance) {
			rejectWatermark++
			continue
		}
		kept = append(kept, key)
	}

	report["hard_reject_brightness"] = rejectBrightness
	report["hard_reject_exposure"] = rejectExposure
	report["hard_reject_blur"] = rejectBlur
	report["hard_reject_watermark"] = rejectWatermark
	report["hard_reject_missing_sample"] = rejectMissingSample
	report["hard_kept_count"] = len(kept)

	if len(kept) < requiredMin {
		report["status"] = "relaxed_keep_original_due_to_low_count"
		report["applied"] = false
		report["output_count"] = len(paths)
		return paths, report
	}

	diversityEnabled := parseEnvBool("PNG_FINAL_DIVERSITY_GUARD_ENABLED", true)
	report["diversity_enabled"] = diversityEnabled
	if !diversityEnabled {
		report["status"] = "applied_hard_gate_only"
		report["applied"] = len(kept) != len(paths)
		report["output_count"] = len(kept)
		return kept, report
	}

	diversityThreshold := clampInt(envIntOrDefault("PNG_FINAL_DIVERSITY_HAMMING_THRESHOLD", qualitySettings.DuplicateHammingThreshold), 2, 16)
	diversityBacktrack := clampInt(envIntOrDefault("PNG_FINAL_DIVERSITY_BACKTRACK", qualitySettings.DuplicateBacktrackFrames), 2, 24)
	report["diversity_hamming_threshold"] = diversityThreshold
	report["diversity_backtrack"] = diversityBacktrack

	diversified := make([]string, 0, len(kept))
	diversifiedSamples := make([]frameQualitySample, 0, len(kept))
	diversityRejected := 0
	for _, framePath := range kept {
		sample, ok := sampleByPath[strings.TrimSpace(framePath)]
		if !ok {
			continue
		}
		if hasNearDuplicate(diversifiedSamples, sample.Hash, diversityThreshold, diversityBacktrack) {
			diversityRejected++
			continue
		}
		diversified = append(diversified, framePath)
		diversifiedSamples = append(diversifiedSamples, sample)
	}
	report["diversity_rejected"] = diversityRejected
	report["diversity_kept_count"] = len(diversified)

	diversityMinKeep := resolvePNGFinalDiversityMinKeep(len(kept))
	report["diversity_min_keep"] = diversityMinKeep
	if len(diversified) < diversityMinKeep {
		report["status"] = "applied_hard_gate_diversity_relaxed"
		report["applied"] = len(kept) != len(paths)
		report["output_count"] = len(kept)
		return kept, report
	}

	report["status"] = "applied_hard_gate_and_diversity"
	report["applied"] = !isSameStringOrder(paths, diversified)
	report["output_count"] = len(diversified)
	return diversified, report
}

func resolvePNGFinalGuardRequiredMin(total int) int {
	if total <= 1 {
		return total
	}
	minKeep := envIntOrDefault("PNG_FINAL_QUALITY_GUARD_MIN_KEEP", 4)
	minKeep = clampInt(minKeep, 1, total)
	ratio := clampFloat(parseFloat(firstNonEmptyString(strings.TrimSpace(os.Getenv("PNG_FINAL_QUALITY_GUARD_MIN_KEEP_RATIO")), "0.55")), 0.2, 0.95)
	ratioKeep := int(math.Ceil(float64(total) * ratio))
	if ratioKeep > minKeep {
		minKeep = ratioKeep
	}
	if minKeep > total {
		minKeep = total
	}
	return minKeep
}

func resolvePNGFinalDiversityMinKeep(total int) int {
	if total <= 1 {
		return total
	}
	minKeep := envIntOrDefault("PNG_FINAL_DIVERSITY_MIN_KEEP", 4)
	minKeep = clampInt(minKeep, 1, total)
	ratio := clampFloat(parseFloat(firstNonEmptyString(strings.TrimSpace(os.Getenv("PNG_FINAL_DIVERSITY_MIN_KEEP_RATIO")), "0.6")), 0.2, 0.98)
	ratioKeep := int(math.Ceil(float64(total) * ratio))
	if ratioKeep > minKeep {
		minKeep = ratioKeep
	}
	if minKeep > total {
		minKeep = total
	}
	return minKeep
}
