package videojobs

import "strings"

func resolveOutputClipWindows(
	meta videoProbeMeta,
	options jobOptions,
	candidates []highlightCandidate,
	qualitySettings QualitySettings,
	preferredMaxOutputs int,
) ([]highlightCandidate, map[string]interface{}) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	_, longVideoThresholdSec, ultraVideoThresholdSec := resolveGIFDurationTierThresholds(qualitySettings)
	baseMaxOutputs := qualitySettings.GIFCandidateMaxOutputs
	if baseMaxOutputs <= 0 {
		baseMaxOutputs = defaultHighlightTopN
	}
	if baseMaxOutputs > 6 {
		baseMaxOutputs = 6
	}
	if baseMaxOutputs < 1 {
		baseMaxOutputs = 1
	}
	preferredMaxOutputs = normalizePreferredGIFMaxOutputs(preferredMaxOutputs)
	if preferredMaxOutputs > baseMaxOutputs {
		baseMaxOutputs = preferredMaxOutputs
	}
	longTierMaxOutputs := qualitySettings.GIFCandidateLongVideoMaxOutputs
	if longTierMaxOutputs <= 0 {
		longTierMaxOutputs = baseMaxOutputs
	}
	if longTierMaxOutputs > 6 {
		longTierMaxOutputs = 6
	}
	if longTierMaxOutputs > baseMaxOutputs {
		longTierMaxOutputs = baseMaxOutputs
	}
	if longTierMaxOutputs < 1 {
		longTierMaxOutputs = 1
	}
	ultraTierMaxOutputs := qualitySettings.GIFCandidateUltraVideoMaxOutputs
	if ultraTierMaxOutputs <= 0 {
		ultraTierMaxOutputs = min(longTierMaxOutputs, DefaultQualitySettings().GIFCandidateUltraVideoMaxOutputs)
	}
	if ultraTierMaxOutputs > 6 {
		ultraTierMaxOutputs = 6
	}
	if ultraTierMaxOutputs > longTierMaxOutputs {
		ultraTierMaxOutputs = longTierMaxOutputs
	}
	if ultraTierMaxOutputs < 1 {
		ultraTierMaxOutputs = 1
	}

	durationTier := "normal"
	tierMaxOutputs := baseMaxOutputs
	budgetMultiplier := qualitySettings.GIFRenderBudgetNormalMultiplier
	switch {
	case meta.DurationSec >= ultraVideoThresholdSec:
		durationTier = "ultra"
		tierMaxOutputs = min(tierMaxOutputs, ultraTierMaxOutputs)
		budgetMultiplier = qualitySettings.GIFRenderBudgetUltraMultiplier
	case meta.DurationSec >= longVideoThresholdSec:
		durationTier = "long"
		tierMaxOutputs = min(tierMaxOutputs, longTierMaxOutputs)
		budgetMultiplier = qualitySettings.GIFRenderBudgetLongMultiplier
	}

	confidenceThreshold := qualitySettings.GIFCandidateConfidenceThreshold
	if confidenceThreshold <= 0 {
		confidenceThreshold = DefaultQualitySettings().GIFCandidateConfidenceThreshold
	}
	targetSizeKB := float64(qualitySettings.GIFTargetSizeKB)
	if targetSizeKB <= 0 {
		targetSizeKB = float64(DefaultQualitySettings().GIFTargetSizeKB)
	}
	budgetLimitKB := maxFloat(targetSizeKB*float64(tierMaxOutputs)*budgetMultiplier, targetSizeKB)

	pool := make([]highlightCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		start, end := clampHighlightWindow(candidate.StartSec, candidate.EndSec, meta.DurationSec)
		if end <= start {
			continue
		}
		pool = append(pool, highlightCandidate{
			StartSec:     start,
			EndSec:       end,
			Score:        candidate.Score,
			Reason:       candidate.Reason,
			ProposalRank: candidate.ProposalRank,
			ProposalID:   candidate.ProposalID,
			CandidateID:  candidate.CandidateID,
		})
	}

	fallbackApplied := false
	fallbackReason := ""
	if len(pool) == 0 {
		windows := resolveOutputClipWindowsFallbackSingle(meta, options)
		if len(windows) > 0 {
			fallbackApplied = true
			fallbackReason = "no_highlight_candidates"
		}
		return windows, map[string]interface{}{
			"version":                   gifRenderSelectionVersion,
			"enabled":                   true,
			"candidate_pool_count":      0,
			"selected_window_count":     len(windows),
			"base_max_outputs":          baseMaxOutputs,
			"preferred_max_outputs":     preferredMaxOutputs,
			"long_tier_max_outputs":     longTierMaxOutputs,
			"ultra_tier_max_outputs":    ultraTierMaxOutputs,
			"tier_max_outputs":          tierMaxOutputs,
			"duration_tier":             durationTier,
			"duration_sec":              roundTo(meta.DurationSec, 3),
			"confidence_threshold":      roundTo(confidenceThreshold, 4),
			"estimated_budget_limit_kb": roundTo(budgetLimitKB, 2),
			"estimated_selected_kb":     0,
			"dropped_low_confidence":    0,
			"dropped_size_budget":       0,
			"dropped_output_limit":      0,
			"fallback_applied":          fallbackApplied,
			"fallback_reason":           fallbackReason,
		}
	}

	topScore := pool[0].Score
	minScore := pool[0].Score
	for _, item := range pool {
		if item.Score > topScore {
			topScore = item.Score
		}
		if item.Score < minScore {
			minScore = item.Score
		}
	}
	scoreSpread := topScore - minScore
	if scoreSpread < 0 {
		scoreSpread = 0
	}

	droppedLowConfidence := 0
	eligible := make([]highlightCandidate, 0, len(pool))
	for idx, candidate := range pool {
		confidence := estimateGIFCandidateConfidence(candidate.Score, topScore, scoreSpread, idx == 0)
		if confidence < confidenceThreshold && len(eligible) > 0 {
			droppedLowConfidence++
			continue
		}
		eligible = append(eligible, candidate)
	}
	if len(eligible) == 0 {
		eligible = append(eligible, pool[0])
		fallbackApplied = true
		fallbackReason = "all_candidates_below_confidence"
	}

	selected := make([]highlightCandidate, 0, tierMaxOutputs)
	selectedEstimatedKB := 0.0
	droppedByBudget := 0
	droppedByOutputLimit := 0
	selectedWindowInfo := make([]map[string]interface{}, 0, tierMaxOutputs)
	for _, candidate := range eligible {
		estimatedKB := estimateGIFCandidateSizeKB(meta, candidate, qualitySettings)
		if len(selected) > 0 && selectedEstimatedKB+estimatedKB > budgetLimitKB {
			droppedByBudget++
			continue
		}
		if len(selected) >= tierMaxOutputs {
			droppedByOutputLimit++
			continue
		}
		selected = append(selected, candidate)
		selectedEstimatedKB += estimatedKB
		selectedWindowInfo = append(selectedWindowInfo, map[string]interface{}{
			"start_sec":     roundTo(candidate.StartSec, 3),
			"end_sec":       roundTo(candidate.EndSec, 3),
			"duration_sec":  roundTo(candidate.EndSec-candidate.StartSec, 3),
			"score":         roundTo(candidate.Score, 4),
			"reason":        strings.TrimSpace(candidate.Reason),
			"proposal_rank": candidate.ProposalRank,
			"proposal_id":   valueOrNilUint64(candidate.ProposalID),
			"candidate_id":  valueOrNilUint64(candidate.CandidateID),
			"estimated_kb":  roundTo(estimatedKB, 2),
		})
	}
	if len(selected) == 0 {
		selected = append(selected, eligible[0])
		estimatedKB := estimateGIFCandidateSizeKB(meta, eligible[0], qualitySettings)
		selectedEstimatedKB = estimatedKB
		fallbackApplied = true
		if fallbackReason == "" {
			fallbackReason = "budget_filtered_all"
		}
		selectedWindowInfo = append(selectedWindowInfo, map[string]interface{}{
			"start_sec":     roundTo(eligible[0].StartSec, 3),
			"end_sec":       roundTo(eligible[0].EndSec, 3),
			"duration_sec":  roundTo(eligible[0].EndSec-eligible[0].StartSec, 3),
			"score":         roundTo(eligible[0].Score, 4),
			"reason":        strings.TrimSpace(eligible[0].Reason),
			"proposal_rank": eligible[0].ProposalRank,
			"proposal_id":   valueOrNilUint64(eligible[0].ProposalID),
			"candidate_id":  valueOrNilUint64(eligible[0].CandidateID),
			"estimated_kb":  roundTo(estimatedKB, 2),
		})
	}

	return selected, map[string]interface{}{
		"version":                   gifRenderSelectionVersion,
		"enabled":                   true,
		"candidate_pool_count":      len(pool),
		"eligible_candidate_count":  len(eligible),
		"selected_window_count":     len(selected),
		"base_max_outputs":          baseMaxOutputs,
		"preferred_max_outputs":     preferredMaxOutputs,
		"long_tier_max_outputs":     longTierMaxOutputs,
		"ultra_tier_max_outputs":    ultraTierMaxOutputs,
		"tier_max_outputs":          tierMaxOutputs,
		"duration_tier":             durationTier,
		"duration_sec":              roundTo(meta.DurationSec, 3),
		"confidence_threshold":      roundTo(confidenceThreshold, 4),
		"estimated_budget_limit_kb": roundTo(budgetLimitKB, 2),
		"estimated_selected_kb":     roundTo(selectedEstimatedKB, 2),
		"dropped_low_confidence":    droppedLowConfidence,
		"dropped_size_budget":       droppedByBudget,
		"dropped_output_limit":      droppedByOutputLimit,
		"fallback_applied":          fallbackApplied,
		"fallback_reason":           fallbackReason,
		"selected_windows":          selectedWindowInfo,
	}
}

func normalizePreferredGIFMaxOutputs(value int) int {
	if value <= 0 {
		return 0
	}
	if value > 6 {
		return 6
	}
	return value
}

func resolveOutputClipWindowsFallbackSingle(meta videoProbeMeta, options jobOptions) []highlightCandidate {
	startSec, durationSec := resolveClipWindow(meta, options)
	if durationSec > 0 {
		return []highlightCandidate{{
			StartSec: startSec,
			EndSec:   startSec + durationSec,
			Score:    1,
			Reason:   "single_window",
		}}
	}
	if meta.DurationSec > 0 {
		if startSec > 0 && startSec < meta.DurationSec {
			return []highlightCandidate{{
				StartSec: startSec,
				EndSec:   meta.DurationSec,
				Score:    1,
				Reason:   "single_window",
			}}
		}
		defaultWindow := chooseHighlightDuration(meta.DurationSec)
		end := defaultWindow
		if end > meta.DurationSec {
			end = meta.DurationSec
		}
		return []highlightCandidate{{
			StartSec: 0,
			EndSec:   end,
			Score:    1,
			Reason:   "single_window",
		}}
	}
	return nil
}

func appendWindowBindingMetadata(meta map[string]interface{}, window highlightCandidate) {
	if len(meta) == 0 {
		return
	}
	if window.ProposalRank > 0 {
		meta["proposal_rank"] = window.ProposalRank
	}
	if window.ProposalID != nil && *window.ProposalID > 0 {
		meta["proposal_id"] = *window.ProposalID
	}
	if window.CandidateID != nil && *window.CandidateID > 0 {
		meta["candidate_id"] = *window.CandidateID
	}
}
