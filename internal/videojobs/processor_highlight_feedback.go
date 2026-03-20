package videojobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strings"

	"emoji/internal/models"

	"gorm.io/datatypes"
)

func probeVideo(ctx context.Context, sourcePath string) (videoProbeMeta, error) {
	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate,duration",
		"-show_entries", "format=duration",
		"-of", "json",
		sourcePath,
	}
	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return videoProbeMeta{}, fmt.Errorf("ffprobe failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	var result ffprobeJSON
	if err := json.Unmarshal(out, &result); err != nil {
		return videoProbeMeta{}, err
	}
	meta := videoProbeMeta{}
	if len(result.Streams) > 0 {
		meta.Width = result.Streams[0].Width
		meta.Height = result.Streams[0].Height
		meta.FPS = parseFPS(result.Streams[0].RFrameRate)
		meta.DurationSec = parseFloat(result.Streams[0].Duration)
	}
	if meta.DurationSec <= 0 {
		meta.DurationSec = parseFloat(result.Format.Duration)
	}
	return meta, nil
}

func suggestHighlightWindow(ctx context.Context, sourcePath string, meta videoProbeMeta, qualitySettings QualitySettings) (highlightSuggestion, error) {
	suggestion := highlightSuggestion{
		Version:  "v1",
		Strategy: "scene_score",
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	if meta.DurationSec <= 0 {
		return suggestion, errors.New("video duration unavailable for highlight scoring")
	}

	targetDuration := chooseHighlightDuration(meta.DurationSec)
	scenePoints, err := detectScenePoints(ctx, sourcePath, 0.10)
	if err != nil {
		return suggestion, err
	}

	candidates := make([]highlightCandidate, 0, len(scenePoints))
	for _, point := range scenePoints {
		start := point.PtsSec - targetDuration*0.35
		end := start + targetDuration
		start, end = clampHighlightWindow(start, end, meta.DurationSec)
		if end-start < 0.8 {
			continue
		}

		mid := (start + end) / 2
		centerBias := 1 - math.Min(1, math.Abs(mid-meta.DurationSec/2)/(meta.DurationSec/2))
		score := point.Score*0.85 + centerBias*0.15
		candidates = append(candidates, highlightCandidate{
			StartSec:   start,
			EndSec:     end,
			Score:      roundTo(score, 4),
			SceneScore: roundTo(point.Score, 4),
			Reason:     "scene_change_peak",
		})
	}

	if len(candidates) == 0 {
		suggestion.Strategy = "fallback_uniform"
		candidates = buildFallbackHighlightCandidates(meta.DurationSec, targetDuration)
	}
	if len(candidates) == 0 {
		return suggestion, errors.New("no highlight candidates generated")
	}

	selected := pickNonOverlapCandidates(candidates, qualitySettings.GIFCandidateMaxOutputs, qualitySettings.GIFCandidateDedupIOUThreshold)
	selected = applyGIFCandidateConfidenceThreshold(selected, candidates, qualitySettings.GIFCandidateConfidenceThreshold)
	if len(selected) == 0 {
		selected = candidates
	}
	if len(selected) > qualitySettings.GIFCandidateMaxOutputs {
		selected = selected[:qualitySettings.GIFCandidateMaxOutputs]
	}
	suggestion.Candidates = selected
	suggestion.All = candidates
	suggestion.Selected = &selected[0]
	return suggestion, nil
}

func legacySignalWeightByFeedbackAction(action string) float64 {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "download":
		return 1.0
	case "favorite", "like", "top_pick":
		// legacy feedback_v1 通过 total_signals + favorite_count*0.8 计分，favorite 类动作约等价 1.8
		return 1.8
	default:
		return 0
	}
}

func (p *Processor) loadUserPublicFeedbackSignalSummary(
	userID uint64,
	jobIDs []uint64,
) (map[uint64]publicFeedbackSignalSummary, error) {
	out := map[uint64]publicFeedbackSignalSummary{}
	if p == nil || p.db == nil || userID == 0 || len(jobIDs) == 0 {
		return out, nil
	}

	type row struct {
		JobID            uint64         `gorm:"column:job_id"`
		OutputID         *uint64        `gorm:"column:output_id"`
		Action           string         `gorm:"column:action"`
		Weight           float64        `gorm:"column:weight"`
		SceneTag         string         `gorm:"column:scene_tag"`
		FeedbackMetadata datatypes.JSON `gorm:"column:feedback_metadata"`
		OutputMetadata   datatypes.JSON `gorm:"column:output_metadata"`
	}

	candidateWindowsByJob, err := p.loadGIFCandidateFeedbackWindows(jobIDs)
	if err != nil {
		// 候选窗口映射异常不阻塞主链路，后续回退到 output metadata / legacy 逻辑。
		candidateWindowsByJob = map[uint64][]gifCandidateFeedbackWindow{}
	}

	var rows []row
	if err := p.db.Table("public.video_image_feedback AS f").
		Select(
			"f.job_id",
			"f.output_id",
			"LOWER(COALESCE(f.action, '')) AS action",
			"COALESCE(f.weight, 0) AS weight",
			"LOWER(COALESCE(NULLIF(TRIM(f.scene_tag), ''), '')) AS scene_tag",
			"f.metadata AS feedback_metadata",
			"o.metadata AS output_metadata",
		).
		Joins("LEFT JOIN public.video_image_outputs o ON o.id = f.output_id AND o.job_id = f.job_id").
		Where("f.user_id = ? AND f.job_id IN ?", userID, jobIDs).
		Order("f.created_at DESC, f.id DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, item := range rows {
		if item.OutputID == nil || *item.OutputID == 0 {
			// 反馈画像严格收敛到 output_id 级，不再消费 job 级历史回退信号。
			continue
		}
		action := strings.ToLower(strings.TrimSpace(item.Action))
		entry := out[item.JobID]
		entry.TotalWeight += item.Weight
		entry.TotalCount++
		entry.DeltaWeight += item.Weight - legacySignalWeightByFeedbackAction(action)
		if item.Weight > 0 {
			entry.PositiveWeight += item.Weight
		} else if item.Weight < 0 {
			entry.NegativeWeight += -item.Weight
		}

		startSec, endSec, reason := parseFeedbackOutputWindowContext(item.OutputMetadata)
		if reason == "" {
			startMeta, endMeta, reasonMeta := parseFeedbackOutputWindowContext(item.FeedbackMetadata)
			if startSec <= 0 && endSec <= 0 {
				startSec = startMeta
				endSec = endMeta
			}
			if reasonMeta != "" {
				reason = reasonMeta
			}
		}
		if reason == "" {
			reason = matchGIFCandidateReasonByWindow(startSec, endSec, candidateWindowsByJob[item.JobID])
		}
		sceneTag := strings.TrimSpace(strings.ToLower(item.SceneTag))
		if sceneTag == "" {
			sceneTag = strings.TrimSpace(strings.ToLower(parseFeedbackSceneTagFromMetadata(item.FeedbackMetadata)))
		}
		entry.Details = append(entry.Details, publicFeedbackSignalDetail{
			OutputID: *item.OutputID,
			Action:   action,
			Weight:   item.Weight,
			SceneTag: sceneTag,
			StartSec: startSec,
			EndSec:   endSec,
			Reason:   reason,
		})
		out[item.JobID] = entry
	}
	return out, nil
}

type gifCandidateFeedbackWindow struct {
	StartSec float64
	EndSec   float64
	Reason   string
	Rank     int
	Selected bool
}

func (p *Processor) loadGIFCandidateFeedbackWindows(jobIDs []uint64) (map[uint64][]gifCandidateFeedbackWindow, error) {
	out := map[uint64][]gifCandidateFeedbackWindow{}
	if p == nil || p.db == nil || len(jobIDs) == 0 {
		return out, nil
	}

	type row struct {
		JobID       uint64         `gorm:"column:job_id"`
		StartMs     int            `gorm:"column:start_ms"`
		EndMs       int            `gorm:"column:end_ms"`
		FinalRank   int            `gorm:"column:final_rank"`
		IsSelected  bool           `gorm:"column:is_selected"`
		FeatureJSON datatypes.JSON `gorm:"column:feature_json"`
	}

	var rows []row
	if err := p.db.Model(&models.VideoJobGIFCandidate{}).
		Select("job_id", "start_ms", "end_ms", "final_rank", "is_selected", "feature_json").
		Where("job_id IN ?", jobIDs).
		Order("job_id ASC, is_selected DESC, final_rank ASC, id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, item := range rows {
		if item.EndMs <= item.StartMs {
			continue
		}
		reason := parseGIFCandidateReason(item.FeatureJSON)
		if reason == "" {
			if item.IsSelected {
				reason = "selected_candidate"
			} else {
				reason = "candidate"
			}
		}
		out[item.JobID] = append(out[item.JobID], gifCandidateFeedbackWindow{
			StartSec: float64(item.StartMs) / 1000.0,
			EndSec:   float64(item.EndMs) / 1000.0,
			Reason:   reason,
			Rank:     item.FinalRank,
			Selected: item.IsSelected,
		})
	}
	return out, nil
}

func parseGIFCandidateReason(feature datatypes.JSON) string {
	if len(feature) == 0 || string(feature) == "null" {
		return ""
	}
	payload := parseJSONMap(feature)
	return normalizeReason(payload["reason"])
}

func parseFeedbackOutputWindowContext(raw datatypes.JSON) (float64, float64, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, 0, ""
	}
	payload := parseJSONMap(raw)

	startSec := floatFromAny(payload["start_sec"])
	if startSec <= 0 {
		startSec = floatFromAny(payload["output_start_sec"])
	}
	endSec := floatFromAny(payload["end_sec"])
	if endSec <= 0 {
		endSec = floatFromAny(payload["output_end_sec"])
	}

	reason := normalizeReason(payload["reason"])
	if reason == "" {
		reason = normalizeReason(payload["output_reason"])
	}
	return startSec, endSec, reason
}

func parseFeedbackSceneTagFromMetadata(raw datatypes.JSON) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	payload := parseJSONMap(raw)
	return stringFromAny(payload["scene_tag"])
}

func matchGIFCandidateReasonByWindow(startSec, endSec float64, windows []gifCandidateFeedbackWindow) string {
	if len(windows) == 0 || endSec <= startSec {
		return ""
	}
	bestReason := ""
	bestIOU := 0.0
	for _, candidate := range windows {
		if candidate.EndSec <= candidate.StartSec {
			continue
		}
		if math.Abs(candidate.StartSec-startSec) <= 0.08 && math.Abs(candidate.EndSec-endSec) <= 0.08 {
			return candidate.Reason
		}
		iou := windowIoU(startSec, endSec, candidate.StartSec, candidate.EndSec)
		if iou <= bestIOU {
			continue
		}
		bestIOU = iou
		bestReason = candidate.Reason
	}
	if bestIOU >= 0.35 {
		return bestReason
	}
	return ""
}

func shouldApplyFeedbackRerank(jobID uint64, settings QualitySettings) bool {
	if !settings.HighlightFeedbackEnabled {
		return false
	}
	rollout := settings.HighlightFeedbackRollout
	if rollout <= 0 {
		return false
	}
	if rollout >= 100 {
		return true
	}
	bucket := int(jobID % 100)
	return bucket < rollout
}

func applyHighlightFeedbackProfile(suggestion highlightSuggestion, durationSec float64, profile highlightFeedbackProfile, qualitySettings QualitySettings) (highlightSuggestion, bool) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	if len(suggestion.Candidates) == 0 || durationSec <= 0 {
		return suggestion, false
	}
	if profile.EngagedJobs < qualitySettings.HighlightFeedbackMinJobs || profile.WeightedSignals < qualitySettings.HighlightFeedbackMinScore {
		return suggestion, false
	}

	baseWeight := qualitySettings.HighlightWeightPosition + qualitySettings.HighlightWeightDuration + qualitySettings.HighlightWeightReason
	if baseWeight <= 0 {
		return suggestion, false
	}

	strength := math.Min(1, profile.WeightedSignals/36) * qualitySettings.HighlightFeedbackBoost
	if strength < 0.15 {
		return suggestion, false
	}
	if profile.PreferredDuration <= 0 {
		profile.PreferredDuration = 0.16
	}

	ranked := append([]highlightCandidate{}, suggestion.Candidates...)
	for idx := range ranked {
		candidate := ranked[idx]
		position := clampZeroOne(((candidate.StartSec + candidate.EndSec) / 2) / durationSec)
		durationRatio := clampZeroOne((candidate.EndSec - candidate.StartSec) / durationSec)
		positionMatch := 1 - math.Abs(position-profile.PreferredCenter)
		if positionMatch < 0 {
			positionMatch = 0
		}

		durationMatch := 1 - math.Abs(durationRatio-profile.PreferredDuration)/0.35
		if durationMatch < 0 {
			durationMatch = 0
		}
		if durationMatch > 1 {
			durationMatch = 1
		}

		reasonWeight := profile.ReasonPreference[normalizeReason(candidate.Reason)]
		if reasonWeight > 1 {
			reasonWeight = 1
		}
		negativeGuard := 0.0
		if qualitySettings.HighlightNegativeGuardEnabled {
			negativeGuard = profile.ReasonNegativeGuard[normalizeReason(candidate.Reason)]
			if negativeGuard > 1 {
				negativeGuard = 1
			}
		}

		boost := (positionMatch*qualitySettings.HighlightWeightPosition +
			durationMatch*qualitySettings.HighlightWeightDuration +
			reasonWeight*qualitySettings.HighlightWeightReason) * strength
		guardPenalty := negativeGuard * qualitySettings.HighlightWeightReason * strength * qualitySettings.HighlightNegativePenaltyWeight
		nextScore := candidate.Score + boost - guardPenalty
		if negativeGuard > 0 {
			guardScale := 1 - negativeGuard*qualitySettings.HighlightNegativePenaltyScale
			if guardScale < 0.2 {
				guardScale = 0.2
			}
			nextScore *= guardScale
		}
		if nextScore < 0 {
			nextScore = 0
		}
		candidate.Score = roundTo(math.Min(1.5, nextScore), 4)
		ranked[idx] = candidate
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].StartSec < ranked[j].StartSec
		}
		return ranked[i].Score > ranked[j].Score
	})
	selected := pickNonOverlapCandidates(ranked, qualitySettings.GIFCandidateMaxOutputs, qualitySettings.GIFCandidateDedupIOUThreshold)
	selected = applyGIFCandidateConfidenceThreshold(selected, ranked, qualitySettings.GIFCandidateConfidenceThreshold)
	if len(selected) == 0 {
		selected = ranked
	}
	if len(selected) > qualitySettings.GIFCandidateMaxOutputs {
		selected = selected[:qualitySettings.GIFCandidateMaxOutputs]
	}
	if len(selected) == 0 {
		return suggestion, false
	}

	suggestion.Candidates = selected
	if len(suggestion.All) == 0 {
		suggestion.All = ranked
	}
	suggestion.Selected = &selected[0]
	suggestion.Strategy = strings.TrimSpace(suggestion.Strategy + "+feedback_rerank")
	return suggestion, true
}
