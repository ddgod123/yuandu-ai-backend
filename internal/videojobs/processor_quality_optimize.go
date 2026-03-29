package videojobs

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
)

func optimizeFramePathsForQuality(paths []string, maxStatic int, qualitySettings QualitySettings) ([]string, frameQualityReport) {
	return optimizeFramePathsForQualityWithGuidance(paths, maxStatic, qualitySettings, imageAI2Guidance{})
}

func optimizeFramePathsForQualityWithGuidance(
	paths []string,
	maxStatic int,
	qualitySettings QualitySettings,
	guidance imageAI2Guidance,
) ([]string, frameQualityReport) {
	report := frameQualityReport{
		TotalFrames:     len(paths),
		SelectorVersion: "v1_scene_ranker",
		ScoringFormula:  resolveFrameQualityScoringFormula(guidance),
	}
	if len(guidance.QualityWeights) > 0 {
		report.SelectorVersion = "v2_ai2_guided_ranker"
		report.ScoringWeights = guidance.QualityWeights
		report.RiskFlags = append([]string{}, guidance.RiskFlags...)
		report.MaxBlurTolerance = guidance.MaxBlurTolerance
		report.ScoringMode = strings.TrimSpace(guidance.ScoringMode)
		if report.ScoringMode == "" {
			report.ScoringMode = "weighted_quality_v1"
		}
	}
	report.SelectionPolicy = resolveFrameSelectionPolicy(guidance)
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
	scoreBreakdownByPath := map[string]frameQualityScoreBreakdown{}
	decisionByPath := map[string]string{}
	rejectReasonByPath := map[string]string{}
	markRejected := func(path string, reason string) {
		key := strings.TrimSpace(path)
		if key == "" {
			return
		}
		decisionByPath[key] = "rejected"
		rejectReasonByPath[key] = strings.TrimSpace(reason)
	}
	markSelected := func(path string) {
		key := strings.TrimSpace(path)
		if key == "" {
			return
		}
		decisionByPath[key] = "selected"
		delete(rejectReasonByPath, key)
	}
	markFinalDecision := func(selected []frameQualitySample, keptPaths []string) {
		keptSet := map[string]struct{}{}
		for _, path := range keptPaths {
			key := strings.TrimSpace(path)
			if key == "" {
				continue
			}
			keptSet[key] = struct{}{}
			decisionByPath[key] = "kept"
			delete(rejectReasonByPath, key)
		}
		for _, sample := range selected {
			key := strings.TrimSpace(sample.Path)
			if key == "" {
				continue
			}
			if _, ok := keptSet[key]; ok {
				continue
			}
			if strings.TrimSpace(decisionByPath[key]) == "" || decisionByPath[key] == "selected" {
				decisionByPath[key] = "dropped_by_budget"
			}
		}
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
		breakdown := computeFrameQualityScoreBreakdownWithAI2Weights(samples[idx], blurThreshold, guidance)
		samples[idx].QualityScore = roundTo(breakdown.FinalScore, 3)
		key := strings.TrimSpace(samples[idx].Path)
		if key != "" {
			scoreBreakdownByPath[key] = breakdown
		}
		sceneIDs[samples[idx].SceneID] = struct{}{}
	}
	report.SceneCount = len(sceneIDs)

	selected := make([]frameQualitySample, 0, len(samples))
	rejected := make([]frameQualitySample, 0, len(samples))
	for _, sample := range samples {
		if qualitySettings.StillMinWidth > 0 && sample.Width > 0 && sample.Width < qualitySettings.StillMinWidth {
			report.RejectedResolution++
			rejected = append(rejected, sample)
			markRejected(sample.Path, "resolution_gate")
			continue
		}
		if qualitySettings.StillMinHeight > 0 && sample.Height > 0 && sample.Height < qualitySettings.StillMinHeight {
			report.RejectedResolution++
			rejected = append(rejected, sample)
			markRejected(sample.Path, "resolution_gate")
			continue
		}
		if sample.Brightness < qualitySettings.MinBrightness || sample.Brightness > qualitySettings.MaxBrightness {
			report.RejectedBrightness++
			rejected = append(rejected, sample)
			markRejected(sample.Path, "brightness_gate")
			continue
		}
		if sample.Exposure < qualitySettings.StillMinExposureScore {
			report.RejectedExposure++
			rejected = append(rejected, sample)
			markRejected(sample.Path, "exposure_gate")
			continue
		}
		if sample.BlurScore < qualitySettings.StillMinBlurScore {
			report.RejectedStillBlurGate++
			rejected = append(rejected, sample)
			markRejected(sample.Path, "still_blur_gate")
			continue
		}
		if sample.BlurScore < blurThreshold {
			report.RejectedBlur++
			rejected = append(rejected, sample)
			markRejected(sample.Path, "blur_threshold_gate")
			continue
		}
		if shouldRejectForWatermarkRisk(sample, blurThreshold, guidance) {
			report.RejectedWatermark++
			rejected = append(rejected, sample)
			markRejected(sample.Path, "watermark_gate")
			continue
		}
		if hasNearDuplicate(selected, sample.Hash, qualitySettings.DuplicateHammingThreshold, qualitySettings.DuplicateBacktrackFrames) {
			report.RejectedNearDuplicate++
			rejected = append(rejected, sample)
			markRejected(sample.Path, "near_duplicate")
			continue
		}
		selected = append(selected, sample)
		markSelected(sample.Path)
	}

	if len(selected) > 0 {
		selected = rankFrameCandidatesBySelectionPolicy(selected, maxStatic, qualitySettings, report.SelectionPolicy)
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
			markSelected(sample.Path)
			if len(selected) >= minKeep || len(selected) >= maxStatic {
				break
			}
		}
		selected = rankFrameCandidatesBySelectionPolicy(selected, maxStatic, qualitySettings, report.SelectionPolicy)
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
			markFinalDecision(selected, hardFiltered)
			report.CandidateScores = buildFrameQualityCandidateScores(
				samples,
				scoreBreakdownByPath,
				decisionByPath,
				rejectReasonByPath,
				guidance,
				blurThreshold,
				28,
			)
			return hardFiltered, report
		}
		fallback := paths
		if len(fallback) > maxStatic {
			fallback = fallback[:maxStatic]
		}
		report.KeptFrames = len(fallback)
		report.KeptSample = pickFramePathSample(fallback, 6)
		markFinalDecision(selected, fallback)
		report.CandidateScores = buildFrameQualityCandidateScores(
			samples,
			scoreBreakdownByPath,
			decisionByPath,
			rejectReasonByPath,
			guidance,
			blurThreshold,
			28,
		)
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
	markFinalDecision(selected, out)
	report.CandidateScores = buildFrameQualityCandidateScores(
		samples,
		scoreBreakdownByPath,
		decisionByPath,
		rejectReasonByPath,
		guidance,
		blurThreshold,
		28,
	)
	return out, report
}

func computeFrameQualityScoreWithAI2Weights(sample frameQualitySample, blurThreshold float64, guidance imageAI2Guidance) float64 {
	breakdown := computeFrameQualityScoreBreakdownWithAI2Weights(sample, blurThreshold, guidance)
	return breakdown.FinalScore
}

func computeFrameQualityScoreBreakdownWithAI2Weights(
	sample frameQualitySample,
	blurThreshold float64,
	guidance imageAI2Guidance,
) frameQualityScoreBreakdown {
	if blurThreshold <= 0 {
		blurThreshold = 1
	}
	blurScore := sample.BlurScore / (blurThreshold * 2.4)
	if blurScore > 1 {
		blurScore = 1
	}
	if blurScore < 0 {
		blurScore = 0
	}
	subject := clampZeroOne(sample.SubjectScore)
	motion := clampZeroOne(sample.MotionScore)
	exposure := clampZeroOne(sample.Exposure)

	// 默认（无 AI2 指导）仍然输出分项，方便管理后台展示候选打分拆解。
	if len(guidance.QualityWeights) == 0 {
		semanticWeight := 0.20
		clarityWeight := 0.44
		loopWeight := 0.12
		efficiencyWeight := 0.24
		semanticScore := subject
		clarityScore := blurScore
		loopScore := motion
		efficiencyScore := exposure
		semanticContribution := semanticScore * semanticWeight
		clarityContribution := clarityScore * clarityWeight
		loopContribution := loopScore * loopWeight
		efficiencyContribution := efficiencyScore * efficiencyWeight
		return frameQualityScoreBreakdown{
			SemanticScore:          semanticScore,
			ClarityScore:           clarityScore,
			LoopScore:              loopScore,
			EfficiencyScore:        efficiencyScore,
			SemanticWeight:         semanticWeight,
			ClarityWeight:          clarityWeight,
			LoopWeight:             loopWeight,
			EfficiencyWeight:       efficiencyWeight,
			SemanticContribution:   semanticContribution,
			ClarityContribution:    clarityContribution,
			LoopContribution:       loopContribution,
			EfficiencyContribution: efficiencyContribution,
			FinalScore:             semanticContribution + clarityContribution + loopContribution + efficiencyContribution,
		}
	}

	weights := guidance.QualityWeights
	semanticW := clampZeroOne(weights["semantic"])
	clarityW := clampZeroOne(weights["clarity"])
	loopW := clampZeroOne(weights["loop"])
	efficiencyW := clampZeroOne(weights["efficiency"])
	if semanticW+clarityW+loopW+efficiencyW <= 0 {
		return computeFrameQualityScoreBreakdownWithAI2Weights(sample, blurThreshold, imageAI2Guidance{})
	}
	normalized := normalizeDirectiveQualityWeights(map[string]float64{
		"semantic":   semanticW,
		"clarity":    clarityW,
		"loop":       loopW,
		"efficiency": efficiencyW,
	})

	semanticScore := clampZeroOne(subject*0.72 + motion*0.28)
	clarityScore := clampZeroOne(blurScore*0.74 + exposure*0.26)
	loopScore := clampZeroOne(1 - math.Abs(motion-0.32)/0.32)
	efficiencyScore := clampZeroOne(exposure*0.62 + (1-motion)*0.38)

	if hasRiskFlag(guidance.RiskFlags, "low_light") {
		clarityScore = clampZeroOne(blurScore*0.82 + exposure*0.18)
		efficiencyScore = clampZeroOne(exposure*0.55 + (1-motion)*0.45)
	}
	if hasRiskFlag(guidance.RiskFlags, "fast_motion") {
		semanticScore = clampZeroOne(subject*0.68 + motion*0.32)
		efficiencyScore = clampZeroOne(exposure*0.52 + (1-motion)*0.48)
	}
	if hasVisualFocus(guidance.VisualFocus, "portrait") {
		semanticScore = clampZeroOne(subject*0.82 + motion*0.18)
	}
	if hasVisualFocus(guidance.VisualFocus, "action") {
		semanticScore = clampZeroOne(subject*0.62 + motion*0.38)
		loopScore = clampZeroOne(1 - math.Abs(motion-0.42)/0.42)
	}
	if hasVisualFocus(guidance.VisualFocus, "text") {
		clarityScore = clampZeroOne(blurScore*0.78 + exposure*0.22)
	}
	if guidance.EnableMatting {
		clarityScore = clampZeroOne(clarityScore*0.85 + subject*0.15)
	}
	switch NormalizeAdvancedScenario(guidance.Scene) {
	case AdvancedScenarioXiaohongshu:
		semanticScore = clampZeroOne(semanticScore*0.88 + subject*0.12)
		clarityScore = clampZeroOne(clarityScore*0.9 + exposure*0.1)
	case AdvancedScenarioWallpaper:
		clarityScore = clampZeroOne(clarityScore*0.92 + exposure*0.08)
		efficiencyScore = clampZeroOne(efficiencyScore*0.78 + (1-motion)*0.22)
	case AdvancedScenarioNews:
		semanticScore = clampZeroOne(semanticScore*0.93 + motion*0.07)
	}
	directiveMustCapture := normalizeStringSlice(guidance.MustCapture, 16)
	directiveAvoid := normalizeStringSlice(guidance.Avoid, 16)
	if len(directiveMustCapture) > 0 || len(directiveAvoid) > 0 {
		mustCaptureHits, avoidHits, _, _ := buildFrameCandidateDirectiveSignalHits(sample, blurThreshold, guidance)
		if expected := len(directiveMustCapture); expected > 0 {
			coverage := clampZeroOne(float64(len(mustCaptureHits)) / float64(expected))
			if coverage > 0 {
				semanticScore = clampZeroOne(semanticScore + coverage*0.22)
				clarityScore = clampZeroOne(clarityScore + coverage*0.08)
				efficiencyScore = clampZeroOne(efficiencyScore + coverage*0.06)
			} else {
				semanticScore = clampZeroOne(semanticScore - 0.06)
			}
		}
		if len(avoidHits) > 0 {
			penalty := clampFloat(float64(len(avoidHits))*0.14, 0.12, 0.42)
			semanticScore = clampZeroOne(semanticScore - penalty)
			clarityScore = clampZeroOne(clarityScore - penalty*0.55)
			efficiencyScore = clampZeroOne(efficiencyScore - penalty*0.35)
		}
	}

	semanticContribution := semanticScore * normalized["semantic"]
	clarityContribution := clarityScore * normalized["clarity"]
	loopContribution := loopScore * normalized["loop"]
	efficiencyContribution := efficiencyScore * normalized["efficiency"]
	return frameQualityScoreBreakdown{
		SemanticScore:          semanticScore,
		ClarityScore:           clarityScore,
		LoopScore:              loopScore,
		EfficiencyScore:        efficiencyScore,
		SemanticWeight:         normalized["semantic"],
		ClarityWeight:          normalized["clarity"],
		LoopWeight:             normalized["loop"],
		EfficiencyWeight:       normalized["efficiency"],
		SemanticContribution:   semanticContribution,
		ClarityContribution:    clarityContribution,
		LoopContribution:       loopContribution,
		EfficiencyContribution: efficiencyContribution,
		FinalScore:             semanticContribution + clarityContribution + loopContribution + efficiencyContribution,
	}
}

func resolveFrameQualityScoringFormula(guidance imageAI2Guidance) string {
	if len(guidance.QualityWeights) > 0 {
		if len(guidance.MustCapture) > 0 || len(guidance.Avoid) > 0 {
			return "final = Σ(base_scores×weights) + directive_hit_adjust(must_capture,avoid)"
		}
		return "final = semantic_score×w_semantic + clarity_score×w_clarity + loop_score×w_loop + efficiency_score×w_efficiency"
	}
	return "final = blur_score×0.44 + exposure_score×0.24 + subject_score×0.20 + motion_score×0.12"
}

func resolveFrameSelectionPolicy(guidance imageAI2Guidance) string {
	policy := strings.ToLower(strings.TrimSpace(guidance.SelectionPolicy))
	if policy != "" {
		return policy
	}
	if len(guidance.QualityWeights) == 0 {
		return "scene_diversity_first"
	}
	return resolveAI2FrameSelectionPolicy(guidance.QualityWeights, guidance.VisualFocus, guidance.EnableMatting)
}

func rankFrameCandidatesBySelectionPolicy(
	in []frameQualitySample,
	maxStatic int,
	qualitySettings QualitySettings,
	selectionPolicy string,
) []frameQualitySample {
	switch strings.ToLower(strings.TrimSpace(selectionPolicy)) {
	case "ai2_global_quality_first":
		return rankFrameCandidatesGlobal(in, maxStatic, qualitySettings)
	default:
		return rankFrameCandidatesByScene(in, maxStatic, qualitySettings)
	}
}

func rankFrameCandidatesGlobal(in []frameQualitySample, maxStatic int, qualitySettings QualitySettings) []frameQualitySample {
	if len(in) == 0 {
		return nil
	}
	if maxStatic <= 0 || maxStatic > len(in) {
		maxStatic = len(in)
	}
	global := append([]frameQualitySample{}, in...)
	sort.SliceStable(global, func(i, j int) bool {
		if global[i].QualityScore == global[j].QualityScore {
			return global[i].Index < global[j].Index
		}
		return global[i].QualityScore > global[j].QualityScore
	})
	out := make([]frameQualitySample, 0, maxStatic)
	seenPath := map[string]struct{}{}
	for _, item := range global {
		if len(out) >= maxStatic {
			break
		}
		pathKey := strings.TrimSpace(item.Path)
		if pathKey == "" {
			continue
		}
		if _, exists := seenPath[pathKey]; exists {
			continue
		}
		if hasNearDuplicate(out, item.Hash, qualitySettings.DuplicateHammingThreshold, qualitySettings.DuplicateBacktrackFrames) {
			continue
		}
		seenPath[pathKey] = struct{}{}
		out = append(out, item)
	}
	return out
}

func buildFrameQualityCandidateScores(
	samples []frameQualitySample,
	breakdownByPath map[string]frameQualityScoreBreakdown,
	decisionByPath map[string]string,
	rejectReasonByPath map[string]string,
	guidance imageAI2Guidance,
	blurThreshold float64,
	maxRows int,
) []frameQualityCandidateScore {
	if len(samples) == 0 || maxRows <= 0 {
		return nil
	}
	sorted := append([]frameQualitySample{}, samples...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].QualityScore == sorted[j].QualityScore {
			return sorted[i].Index < sorted[j].Index
		}
		return sorted[i].QualityScore > sorted[j].QualityScore
	})
	candidates := make([]frameQualityCandidateScore, 0, minInt(maxRows, len(sorted)))
	added := map[string]struct{}{}
	appendSample := func(sample frameQualitySample, rank int) {
		if len(candidates) >= maxRows {
			return
		}
		pathKey := strings.TrimSpace(sample.Path)
		if pathKey != "" {
			if _, ok := added[pathKey]; ok {
				return
			}
			added[pathKey] = struct{}{}
		}
		breakdown, ok := breakdownByPath[pathKey]
		if !ok {
			breakdown = frameQualityScoreBreakdown{
				FinalScore: sample.QualityScore,
			}
		}
		decision := strings.TrimSpace(decisionByPath[pathKey])
		rejectReason := strings.TrimSpace(rejectReasonByPath[pathKey])
		mustCaptureHits, avoidHits, positiveSignals, negativeSignals := buildFrameCandidateDirectiveSignalHits(sample, blurThreshold, guidance)
		candidates = append(candidates, frameQualityCandidateScore{
			Rank:                   rank,
			Index:                  sample.Index,
			FramePath:              sample.Path,
			FrameName:              filepath.Base(sample.Path),
			SceneID:                sample.SceneID,
			Decision:               decision,
			RejectReason:           rejectReason,
			MustCaptureHits:        mustCaptureHits,
			AvoidHits:              avoidHits,
			PositiveSignals:        positiveSignals,
			NegativeSignals:        negativeSignals,
			ExplainSummary:         buildFrameCandidateExplainSummary(decision, rejectReason, mustCaptureHits, avoidHits, breakdown.FinalScore),
			FinalScore:             roundTo(breakdown.FinalScore, 4),
			SemanticScore:          roundTo(breakdown.SemanticScore, 4),
			ClarityScore:           roundTo(breakdown.ClarityScore, 4),
			LoopScore:              roundTo(breakdown.LoopScore, 4),
			EfficiencyScore:        roundTo(breakdown.EfficiencyScore, 4),
			SemanticWeight:         roundTo(breakdown.SemanticWeight, 4),
			ClarityWeight:          roundTo(breakdown.ClarityWeight, 4),
			LoopWeight:             roundTo(breakdown.LoopWeight, 4),
			EfficiencyWeight:       roundTo(breakdown.EfficiencyWeight, 4),
			SemanticContribution:   roundTo(breakdown.SemanticContribution, 4),
			ClarityContribution:    roundTo(breakdown.ClarityContribution, 4),
			LoopContribution:       roundTo(breakdown.LoopContribution, 4),
			EfficiencyContribution: roundTo(breakdown.EfficiencyContribution, 4),
			BlurScore:              roundTo(sample.BlurScore, 4),
			SubjectScore:           roundTo(sample.SubjectScore, 4),
			MotionScore:            roundTo(sample.MotionScore, 4),
			ExposureScore:          roundTo(sample.Exposure, 4),
			Brightness:             roundTo(sample.Brightness, 2),
		})
	}
	for idx, sample := range sorted {
		appendSample(sample, idx+1)
		if len(candidates) >= maxRows {
			break
		}
	}
	return candidates
}

func buildFrameCandidateDirectiveSignalHits(
	sample frameQualitySample,
	blurThreshold float64,
	guidance imageAI2Guidance,
) (mustCaptureHits []string, avoidHits []string, positiveSignals []string, negativeSignals []string) {
	subjectStrong := sample.SubjectScore >= 0.68
	subjectPresent := sample.SubjectScore >= 0.52
	actionStrong := sample.MotionScore >= 0.62
	actionPresent := sample.MotionScore >= 0.45
	exposureStrong := sample.Exposure >= 0.62
	exposureWeak := sample.Exposure > 0 && sample.Exposure < 0.35
	darkFrame := sample.Brightness > 0 && sample.Brightness < 30
	veryDarkFrame := sample.Brightness > 0 && sample.Brightness < 22
	if blurThreshold <= 0 {
		blurThreshold = 1
	}
	blurRisk := sample.BlurScore > 0 && sample.BlurScore < blurThreshold*0.96
	sharpFrame := sample.BlurScore >= blurThreshold*1.12

	termHit := func(term string, isAvoid bool) bool {
		normalized := strings.ToLower(strings.TrimSpace(term))
		if normalized == "" {
			return false
		}
		if isAvoid {
			switch {
			case containsAnyKeyword(normalized, []string{"水印", "logo", "台标", "watermark"}):
				return estimateWatermarkRisk(sample, blurThreshold)
			case containsAnyKeyword(normalized, []string{"背影", "侧脸", "遮挡", "残缺", "挡住", "occlusion", "backview"}):
				return sample.SubjectScore < 0.50 || (sample.SubjectScore < 0.58 && sample.MotionScore > 0.52)
			case containsAnyKeyword(normalized, []string{"低饱和", "灰雾", "发灰", "灰蒙", "雾感", "washout", "haze"}):
				return sample.Exposure < 0.50 || blurRisk
			case containsAnyKeyword(normalized, []string{"杂乱", "凌乱", "乱入", "路人", "clutter"}):
				return sample.SubjectScore < 0.48 && (sample.MotionScore >= 0.45 || sample.Exposure >= 0.55)
			case containsAnyKeyword(normalized, []string{"拖影", "抖动", "虚焦", "模糊", "blur", "shake"}):
				return blurRisk || (sample.MotionScore >= 0.72 && sample.BlurScore < blurThreshold*1.1)
			case containsAnyKeyword(normalized, []string{"暗", "黑", "夜", "low_light", "dark", "night"}):
				return darkFrame || exposureWeak
			default:
				return sample.QualityScore > 0 && sample.QualityScore < 0.50
			}
		}
		switch {
		case containsAnyKeyword(normalized, []string{"人物", "人像", "人脸", "面部", "特写", "portrait", "face", "person", "subject", "主角", "主体", "表情"}):
			return subjectPresent
		case containsAnyKeyword(normalized, []string{"关键内容", "内容完整", "信息完整", "完整画面", "完整"}):
			return sample.SubjectScore >= 0.45 || sample.Exposure >= 0.42
		case containsAnyKeyword(normalized, []string{"构图", "稳定", "平衡", "居中", "composition"}):
			return sample.MotionScore <= 0.62 && !blurRisk
		case containsAnyKeyword(normalized, []string{"动作", "瞬间", "高光", "精彩", "运动", "庆祝", "进球", "action", "moment", "peak", "highlight"}):
			return actionPresent
		case containsAnyKeyword(normalized, []string{"情绪", "张力", "爆发", "崩溃", "emotion", "expressive"}):
			return actionPresent || subjectStrong || (subjectPresent && sample.Exposure >= 0.50)
		case containsAnyKeyword(normalized, []string{"姿态", "定格", "pose", "造型"}):
			return subjectPresent && sample.MotionScore >= 0.22 && sample.MotionScore <= 0.72
		case containsAnyKeyword(normalized, []string{"色彩", "明快", "鲜艳", "饱和", "网感", "vibrant", "colorful"}):
			return sample.Exposure >= 0.50 && sample.Brightness >= 35 && !blurRisk
		case containsAnyKeyword(normalized, []string{"字幕", "文字", "文案", "text", "subtitle", "caption"}):
			return !blurRisk && sample.Exposure >= 0.45
		case containsAnyKeyword(normalized, []string{"水印", "logo", "台标", "watermark"}):
			return estimateWatermarkRisk(sample, blurThreshold)
		case containsAnyKeyword(normalized, []string{"场景", "氛围", "风景", "背景", "全景", "vibe", "scene", "landscape"}):
			return sample.Exposure >= 0.42 && !veryDarkFrame
		case containsAnyKeyword(normalized, []string{"暗", "黑", "夜", "low_light", "dark", "night"}):
			return darkFrame
		case containsAnyKeyword(normalized, []string{"模糊", "糊", "抖动", "虚焦", "blur", "shake"}):
			return blurRisk || (sample.MotionScore >= 0.72 && sample.BlurScore < blurThreshold*1.05)
		default:
			return sample.QualityScore >= 0.56
		}
	}

	for _, term := range normalizeStringSlice(guidance.MustCapture, 16) {
		if termHit(term, false) {
			mustCaptureHits = append(mustCaptureHits, term)
		}
	}
	for _, term := range normalizeStringSlice(guidance.Avoid, 16) {
		if termHit(term, true) {
			avoidHits = append(avoidHits, term)
		}
	}
	mustCaptureHits = normalizeStringSlice(mustCaptureHits, 16)
	avoidHits = normalizeStringSlice(avoidHits, 16)

	if subjectStrong {
		positiveSignals = append(positiveSignals, "主体清晰")
	}
	if sharpFrame {
		positiveSignals = append(positiveSignals, "清晰度高")
	}
	if actionStrong {
		positiveSignals = append(positiveSignals, "动作峰值明显")
	}
	if exposureStrong {
		positiveSignals = append(positiveSignals, "曝光良好")
	}
	if len(mustCaptureHits) > 0 {
		positiveSignals = append(positiveSignals, fmt.Sprintf("命中 must_capture %d 项", len(mustCaptureHits)))
	}

	if blurRisk {
		negativeSignals = append(negativeSignals, "存在模糊风险")
	}
	if darkFrame {
		negativeSignals = append(negativeSignals, "画面偏暗")
	}
	if veryDarkFrame && hasRiskFlag(guidance.RiskFlags, "low_light") {
		negativeSignals = append(negativeSignals, "低光场景风险高")
	}
	if exposureWeak {
		negativeSignals = append(negativeSignals, "曝光不足")
	}
	if len(avoidHits) > 0 {
		negativeSignals = append(negativeSignals, fmt.Sprintf("触发 avoid %d 项", len(avoidHits)))
	}
	if hasRiskFlag(guidance.RiskFlags, "fast_motion") && sample.MotionScore >= 0.75 {
		negativeSignals = append(negativeSignals, "高速运动导致稳定性下降")
	}

	positiveSignals = normalizeStringSlice(positiveSignals, 8)
	negativeSignals = normalizeStringSlice(negativeSignals, 8)
	return mustCaptureHits, avoidHits, positiveSignals, negativeSignals
}

func buildFrameCandidateExplainSummary(
	decision string,
	rejectReason string,
	mustCaptureHits []string,
	avoidHits []string,
	finalScore float64,
) string {
	decision = strings.ToLower(strings.TrimSpace(decision))
	rejectReason = strings.ToLower(strings.TrimSpace(rejectReason))
	switch decision {
	case "kept":
		return fmt.Sprintf(
			"保留：命中 must_capture %d 项，触发 avoid %d 项，综合分 %.3f。",
			len(mustCaptureHits),
			len(avoidHits),
			roundTo(finalScore, 3),
		)
	case "dropped_by_budget":
		return fmt.Sprintf(
			"预算淘汰：候选质量可用，但在数量裁剪中被移除（综合分 %.3f）。",
			roundTo(finalScore, 3),
		)
	case "rejected":
		return fmt.Sprintf(
			"拒绝：%s（综合分 %.3f）。",
			frameRejectReasonLabel(rejectReason),
			roundTo(finalScore, 3),
		)
	default:
		return fmt.Sprintf(
			"候选：命中 must_capture %d 项，触发 avoid %d 项，综合分 %.3f。",
			len(mustCaptureHits),
			len(avoidHits),
			roundTo(finalScore, 3),
		)
	}
}

func frameRejectReasonLabel(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "resolution_gate":
		return "分辨率未达标"
	case "brightness_gate":
		return "亮度范围不达标"
	case "exposure_gate":
		return "曝光评分不达标"
	case "still_blur_gate":
		return "静态清晰度门槛未达标"
	case "blur_threshold_gate":
		return "动态模糊阈值未达标"
	case "watermark_gate":
		return "水印风险命中策略门槛"
	case "near_duplicate":
		return "与已选帧近重复"
	case "dropped_by_budget":
		return "候选预算裁剪"
	default:
		if reason == "" {
			return "未命中保留条件"
		}
		return reason
	}
}

func shouldRejectForWatermarkRisk(sample frameQualitySample, blurThreshold float64, guidance imageAI2Guidance) bool {
	if !guidance.AvoidWatermarks {
		return false
	}
	watermarkSensitive := hasRiskFlag(guidance.RiskFlags, "watermark_risk")
	if !watermarkSensitive {
		for _, term := range normalizeStringSlice(guidance.Avoid, 16) {
			normalized := strings.ToLower(strings.TrimSpace(term))
			if containsAnyKeyword(normalized, []string{"水印", "logo", "台标", "watermark"}) {
				watermarkSensitive = true
				break
			}
		}
	}
	if !watermarkSensitive {
		return false
	}
	return estimateWatermarkRisk(sample, blurThreshold)
}

func estimateWatermarkRisk(sample frameQualitySample, blurThreshold float64) bool {
	if blurThreshold <= 0 {
		blurThreshold = 1
	}
	highExposure := sample.Exposure >= 0.68 || sample.Brightness >= 170
	lowSubject := sample.SubjectScore <= 0.42
	lowMotion := sample.MotionScore <= 0.36
	clarityEnough := sample.BlurScore >= blurThreshold*0.88
	return highExposure && lowSubject && lowMotion && clarityEnough
}

func containsAnyKeyword(text string, keywords []string) bool {
	if text == "" || len(keywords) == 0 {
		return false
	}
	for _, keyword := range keywords {
		if keyword == "" {
			continue
		}
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}
