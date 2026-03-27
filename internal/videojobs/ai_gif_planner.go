package videojobs

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"emoji/internal/models"
)

func (p *Processor) requestAIGIFPlannerSuggestion(
	ctx context.Context,
	job models.VideoJob,
	sourcePath string,
	meta videoProbeMeta,
	local highlightSuggestion,
	directive *gifAIDirectiveProfile,
	qualitySettings QualitySettings,
) (highlightSuggestion, map[string]interface{}, error) {
	cfg := p.loadGIFAIPlannerConfig()
	info := map[string]interface{}{
		"enabled":        cfg.Enabled,
		"provider":       cfg.Provider,
		"model":          cfg.Model,
		"prompt_version": cfg.PromptVersion,
	}
	if !cfg.Enabled {
		info["applied"] = false
		return local, info, fmt.Errorf("planner disabled")
	}
	if !containsString(normalizeOutputFormats(job.OutputFormats), "gif") {
		info["applied"] = false
		return local, info, fmt.Errorf("non-gif job")
	}

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	topNDecision := resolveAIGIFPlannerTargetTopNDecision(meta, directive, qualitySettings)
	targetTopN := topNDecision.AppliedTopN
	frameSamples, frameErr := sampleAIDirectorFrames(ctx, sourcePath, meta, 8)
	frameManifest := buildAIGIFFrameManifest(frameSamples)
	payload := buildAIGIFPlannerInputPayload(job, meta, topNDecision, frameManifest, directive)
	userBytes, _ := json.Marshal(payload)
	userParts := make([]openAICompatContentPart, 0, len(frameSamples)+1)
	userParts = append(userParts, openAICompatContentPart{
		Type: "text",
		Text: string(userBytes),
	})
	for _, item := range frameSamples {
		if strings.TrimSpace(item.DataURL) == "" {
			continue
		}
		userParts = append(userParts, openAICompatContentPart{
			Type: "image_url",
			ImageURL: &openAICompatImageURL{
				URL: item.DataURL,
			},
		})
	}

	systemPrompt := defaultAIGIFPlannerSystemPrompt
	promptVersion := "built_in_v1"
	promptSource := "built_in_default"
	if template, templateErr := p.loadAIPromptTemplateWithFallback("gif", "ai2", "fixed"); templateErr == nil {
		if template.Found {
			if strings.TrimSpace(template.Text) != "" && template.Enabled {
				systemPrompt = strings.TrimSpace(template.Text)
			}
			if strings.TrimSpace(template.Version) != "" {
				promptVersion = strings.TrimSpace(template.Version)
			}
			if strings.TrimSpace(template.Source) != "" {
				promptSource = strings.TrimSpace(template.Source)
			}
		}
	}

	modelText, usage, rawResp, durationMs, err := p.callOpenAICompatJSONChatWithUserParts(ctx, cfg, systemPrompt, userParts)
	usageMetadata := aiGIFPlannerUsageMetadata{
		PromptVersion:             cfg.PromptVersion,
		PromptTemplateVersion:     promptVersion,
		PromptTemplateSource:      promptSource,
		TargetTopN:                targetTopN,
		TargetTopNBase:            topNDecision.BaseTopN,
		TargetTopNAISuggested:     topNDecision.AISuggestedTopN,
		TargetTopNAllowed:         topNDecision.AllowedTopN,
		TargetTopNApplied:         topNDecision.AppliedTopN,
		TargetTopNOverrideEnabled: topNDecision.OverrideEnabled,
		TargetTopNExpandRatio:     roundTo(topNDecision.ExpandRatio, 4),
		TargetTopNAbsoluteCap:     topNDecision.AbsoluteCap,
		TargetTopNClampReason:     topNDecision.ClampReason,
		TargetTopNDurationTier:    topNDecision.DurationTier,
		CandidateSource:           "frame_manifest",
		FrameCount:                len(frameSamples),
		DirectorApplied:           directive != nil,
		PlannerInputPayloadV1:     payload,
		PlannerInputPayloadBytes:  len(userBytes),
	}
	if frameErr != nil {
		usageMetadata.FrameSamplingError = frameErr.Error()
	}
	status := "ok"
	errText := ""
	if err != nil {
		status = "error"
		errText = err.Error()
	}
	p.recordVideoJobAIUsage(videoJobAIUsageInput{
		JobID:             job.ID,
		UserID:            job.UserID,
		Stage:             gifAIPlannerStage,
		Provider:          cfg.Provider,
		Model:             cfg.Model,
		Endpoint:          cfg.Endpoint,
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CachedInputTokens: usage.CachedInputTokens,
		ImageTokens:       usage.ImageTokens,
		VideoTokens:       usage.VideoTokens,
		AudioSeconds:      usage.AudioSeconds,
		RequestDurationMs: durationMs,
		RequestStatus:     status,
		RequestError:      errText,
		Metadata:          usageMetadata,
	})
	if err != nil {
		info["applied"] = false
		info["error"] = err.Error()
		return local, info, err
	}

	var parsed gifAIPlannerResponse
	if err := unmarshalModelJSONWithRepair(modelText, &parsed); err != nil {
		info["applied"] = false
		info["error"] = "parse planner response: " + err.Error()
		return local, info, err
	}

	proposals := normalizeAIGIFPlannerProposals(parsed.Proposals, meta.DurationSec)
	if len(proposals) == 0 {
		info["applied"] = false
		info["error"] = "planner produced empty proposals"
		return local, info, fmt.Errorf("planner produced empty proposals")
	}
	candidates := make([]highlightCandidate, 0, len(proposals))
	for _, item := range proposals {
		candidates = append(candidates, highlightCandidate{
			StartSec:     roundTo(item.StartSec, 3),
			EndSec:       roundTo(item.EndSec, 3),
			Score:        roundTo(item.Score, 4),
			SceneScore:   roundTo(item.StandaloneConfidence, 4),
			Reason:       strings.TrimSpace(item.ProposalReason),
			ProposalRank: item.ProposalRank,
		})
	}
	selected := selectAIGIFPlannerExecutionCandidates(candidates, targetTopN)
	if len(selected) == 0 {
		selected = candidates
	}
	suggestion := highlightSuggestion{
		Version:    "ai_planner_v1",
		Strategy:   "ai_semantic_planner",
		Selected:   &selected[0],
		Candidates: selected,
		All:        candidates,
	}
	proposalIDByRank, _ := p.persistAIGIFProposals(job.ID, job.UserID, cfg, proposals, suggestion, rawResp)
	if len(proposalIDByRank) > 0 {
		for idx := range suggestion.Candidates {
			rank := suggestion.Candidates[idx].ProposalRank
			if rank <= 0 {
				continue
			}
			if proposalID, ok := proposalIDByRank[rank]; ok && proposalID > 0 {
				proposalID := proposalID
				suggestion.Candidates[idx].ProposalID = &proposalID
			}
		}
		for idx := range suggestion.All {
			rank := suggestion.All[idx].ProposalRank
			if rank <= 0 {
				continue
			}
			if proposalID, ok := proposalIDByRank[rank]; ok && proposalID > 0 {
				proposalID := proposalID
				suggestion.All[idx].ProposalID = &proposalID
			}
		}
		if len(suggestion.Candidates) > 0 {
			suggestion.Selected = &suggestion.Candidates[0]
		}
	}

	info["applied"] = true
	info["candidate_count"] = len(proposals)
	info["selected_count"] = len(selected)
	info["selected_start_sec"] = suggestion.Selected.StartSec
	info["selected_end_sec"] = suggestion.Selected.EndSec
	info["selected_score"] = suggestion.Selected.Score
	info["target_top_n_base"] = topNDecision.BaseTopN
	info["target_top_n_ai_suggested"] = topNDecision.AISuggestedTopN
	info["target_top_n_allowed"] = topNDecision.AllowedTopN
	info["target_top_n_applied"] = topNDecision.AppliedTopN
	info["selection_policy"] = "ai2_rank_order_primary"
	info["target_top_n_override_enabled"] = topNDecision.OverrideEnabled
	info["target_top_n_expand_ratio"] = roundTo(topNDecision.ExpandRatio, 4)
	info["target_top_n_absolute_cap"] = topNDecision.AbsoluteCap
	info["target_top_n_clamp_reason"] = topNDecision.ClampReason
	info["target_top_n_duration_tier"] = topNDecision.DurationTier
	info["frame_count"] = len(frameSamples)
	if frameErr != nil {
		info["frame_sampling_error"] = frameErr.Error()
	}
	info["director_applied"] = directive != nil
	return suggestion, info, nil
}

func resolveAIGIFPlannerTargetTopN(
	meta videoProbeMeta,
	directive *gifAIDirectiveProfile,
	qualitySettings QualitySettings,
) int {
	return resolveAIGIFPlannerTargetTopNDecision(meta, directive, qualitySettings).AppliedTopN
}

type aiGIFPlannerTopNDecision struct {
	BaseTopN        int
	AISuggestedTopN int
	AllowedTopN     int
	AppliedTopN     int
	OverrideEnabled bool
	ExpandRatio     float64
	AbsoluteCap     int
	ClampReason     string
	DurationTier    string
}

func resolveAIGIFPlannerTargetTopNDecision(
	meta videoProbeMeta,
	directive *gifAIDirectiveProfile,
	qualitySettings QualitySettings,
) aiGIFPlannerTopNDecision {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	_, longVideoThresholdSec, ultraVideoThresholdSec := resolveGIFDurationTierThresholds(qualitySettings)
	baseTarget := qualitySettings.GIFCandidateMaxOutputs
	if baseTarget <= 0 {
		baseTarget = defaultHighlightTopN
	}
	if baseTarget < 1 {
		baseTarget = 1
	}
	if baseTarget > maxGIFCandidateOutputs {
		baseTarget = maxGIFCandidateOutputs
	}
	durationTier := "normal"
	if meta.DurationSec >= ultraVideoThresholdSec {
		durationTier = "ultra"
		ultraCap := qualitySettings.GIFCandidateUltraVideoMaxOutputs
		if ultraCap <= 0 {
			ultraCap = qualitySettings.GIFCandidateLongVideoMaxOutputs
		}
		if ultraCap > 0 && baseTarget > ultraCap {
			baseTarget = ultraCap
		}
	} else if meta.DurationSec >= longVideoThresholdSec {
		durationTier = "long"
		longCap := qualitySettings.GIFCandidateLongVideoMaxOutputs
		if longCap > 0 && baseTarget > longCap {
			baseTarget = longCap
		}
	}
	if baseTarget < 1 {
		baseTarget = 1
	}
	if baseTarget > maxGIFCandidateOutputs {
		baseTarget = maxGIFCandidateOutputs
	}

	aiSuggested := baseTarget
	if directive != nil {
		if directive.ClipCountMax > 0 {
			aiSuggested = directive.ClipCountMax
		}
		if directive.ClipCountMin > 0 && aiSuggested < directive.ClipCountMin {
			aiSuggested = directive.ClipCountMin
		}
	}
	if aiSuggested < 1 {
		aiSuggested = 1
	}
	if aiSuggested > maxGIFCandidateOutputs {
		aiSuggested = maxGIFCandidateOutputs
	}

	decision := aiGIFPlannerTopNDecision{
		BaseTopN:        baseTarget,
		AISuggestedTopN: aiSuggested,
		AllowedTopN:     baseTarget,
		AppliedTopN:     baseTarget,
		OverrideEnabled: qualitySettings.AIDirectorConstraintOverrideEnabled,
		ExpandRatio:     qualitySettings.AIDirectorCountExpandRatio,
		AbsoluteCap:     qualitySettings.AIDirectorCountAbsoluteCap,
		DurationTier:    durationTier,
	}
	if decision.AbsoluteCap <= 0 {
		decision.AbsoluteCap = baseTarget
	}
	if decision.AbsoluteCap > maxGIFCandidateOutputs {
		decision.AbsoluteCap = maxGIFCandidateOutputs
	}
	if decision.AbsoluteCap < baseTarget {
		decision.AbsoluteCap = baseTarget
	}

	if decision.OverrideEnabled {
		expanded := int(float64(baseTarget)*(1.0+decision.ExpandRatio) + 0.5)
		if expanded < baseTarget {
			expanded = baseTarget
		}
		if expanded > decision.AbsoluteCap {
			expanded = decision.AbsoluteCap
		}
		if expanded > maxGIFCandidateOutputs {
			expanded = maxGIFCandidateOutputs
		}
		if expanded < 1 {
			expanded = 1
		}
		decision.AllowedTopN = expanded
	} else {
		decision.AllowedTopN = baseTarget
	}

	decision.AppliedTopN = aiSuggested
	if decision.AppliedTopN > decision.AllowedTopN {
		decision.AppliedTopN = decision.AllowedTopN
		if decision.OverrideEnabled {
			decision.ClampReason = "exceeds_policy_allowed_top_n"
		} else {
			decision.ClampReason = "override_disabled"
		}
	}
	if decision.AppliedTopN < 1 {
		decision.AppliedTopN = 1
	}
	if decision.AppliedTopN > maxGIFCandidateOutputs {
		decision.AppliedTopN = maxGIFCandidateOutputs
	}

	return decision
}

func normalizeAIGIFPlannerProposals(in []gifAIPlannerProposal, durationSec float64) []gifAIPlannerProposal {
	if len(in) == 0 {
		return nil
	}
	out := make([]gifAIPlannerProposal, 0, len(in))
	for idx, item := range in {
		start, end := clampHighlightWindow(item.StartSec, item.EndSec, durationSec)
		if end-start < 0.8 {
			continue
		}
		row := item
		row.StartSec = start
		row.EndSec = end
		if row.ProposalRank <= 0 {
			row.ProposalRank = idx + 1
		}
		row.Score = clampZeroOne(row.Score)
		row.StandaloneConfidence = clampZeroOne(row.StandaloneConfidence)
		row.LoopFriendlinessHint = clampZeroOne(row.LoopFriendlinessHint)
		if strings.TrimSpace(row.ProposalReason) == "" {
			row.ProposalReason = "ai_proposal"
		}
		row.ExpectedValueLevel = strings.ToLower(strings.TrimSpace(row.ExpectedValueLevel))
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProposalRank == out[j].ProposalRank {
			if out[i].Score == out[j].Score {
				return out[i].StartSec < out[j].StartSec
			}
			return out[i].Score > out[j].Score
		}
		return out[i].ProposalRank < out[j].ProposalRank
	})
	dedup := make([]gifAIPlannerProposal, 0, len(out))
	seenRank := map[int]struct{}{}
	for _, item := range out {
		if item.ProposalRank <= 0 {
			continue
		}
		if _, exists := seenRank[item.ProposalRank]; exists {
			continue
		}
		seenRank[item.ProposalRank] = struct{}{}
		dedup = append(dedup, item)
	}
	return dedup
}

func selectAIGIFPlannerExecutionCandidates(candidates []highlightCandidate, targetTopN int) []highlightCandidate {
	if len(candidates) == 0 {
		return nil
	}
	if targetTopN <= 0 || targetTopN >= len(candidates) {
		return append([]highlightCandidate{}, candidates...)
	}
	selected := make([]highlightCandidate, 0, targetTopN)
	for idx := 0; idx < len(candidates) && len(selected) < targetTopN; idx++ {
		selected = append(selected, candidates[idx])
	}
	return selected
}
