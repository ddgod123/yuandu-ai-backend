package videojobs

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"emoji/internal/models"
	"gorm.io/gorm"
)

func (p *Processor) persistGIFHighlightCandidates(
	ctx context.Context,
	sourcePath string,
	meta videoProbeMeta,
	jobID uint64,
	suggestion highlightSuggestion,
	qualitySettings QualitySettings,
) error {
	if p == nil || p.db == nil || jobID == 0 {
		return nil
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	pool := append([]highlightCandidate{}, suggestion.All...)
	if len(pool) == 0 {
		pool = append(pool, suggestion.Candidates...)
	}
	if len(pool) == 0 {
		return nil
	}

	selectedRanks := make(map[string]int, len(suggestion.Candidates))
	for idx, candidate := range suggestion.Candidates {
		key := highlightCandidateWindowKey(candidate)
		if key == "" {
			continue
		}
		selectedRanks[key] = idx + 1
	}

	topScore := 0.0
	minScore := 0.0
	if len(suggestion.Candidates) > 0 {
		topScore = suggestion.Candidates[0].Score
		minScore = suggestion.Candidates[0].Score
		for _, candidate := range suggestion.Candidates {
			if candidate.Score > topScore {
				topScore = candidate.Score
			}
			if candidate.Score < minScore {
				minScore = candidate.Score
			}
		}
	}
	scoreSpread := topScore - minScore
	if scoreSpread < 0 {
		scoreSpread = 0
	}

	merged := make(map[string]models.VideoJobGIFCandidate, len(pool))
	for _, candidate := range pool {
		startMs, endMs, ok := normalizeHighlightCandidateWindowMs(candidate)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%d-%d", startMs, endMs)
		rank, selected := selectedRanks[key]
		featureSnapshot := p.buildGIFCandidateFeatureSnapshot(
			ctx,
			sourcePath,
			meta,
			candidate,
			qualitySettings,
			topScore,
			scoreSpread,
			selected,
			suggestion.Strategy,
			suggestion.Version,
		)
		rejectReason := ""
		if !selected {
			rejectReason = inferGIFCandidateRejectReason(
				candidate,
				suggestion.Candidates,
				qualitySettings.GIFCandidateDedupIOUThreshold,
				qualitySettings.GIFCandidateConfidenceThreshold,
				topScore,
				scoreSpread,
				featureSnapshot,
				qualitySettings,
			)
		}

		row := models.VideoJobGIFCandidate{
			JobID:           jobID,
			StartMs:         startMs,
			EndMs:           endMs,
			DurationMs:      endMs - startMs,
			BaseScore:       roundTo(candidate.Score, 4),
			ConfidenceScore: roundTo(estimateGIFCandidateConfidence(candidate.Score, topScore, scoreSpread, selected), 4),
			FinalRank:       rank,
			IsSelected:      selected,
			RejectReason:    rejectReason,
			FeatureJSON:     mustJSON(featureSnapshot),
		}

		if existing, exists := merged[key]; exists {
			replace := false
			if row.IsSelected && !existing.IsSelected {
				replace = true
			} else if row.IsSelected == existing.IsSelected {
				if row.FinalRank > 0 && (existing.FinalRank == 0 || row.FinalRank < existing.FinalRank) {
					replace = true
				} else if row.BaseScore > existing.BaseScore {
					replace = true
				}
			}
			if replace {
				merged[key] = row
			}
			continue
		}
		merged[key] = row
	}

	if len(merged) == 0 {
		return nil
	}

	rows := make([]models.VideoJobGIFCandidate, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].IsSelected != rows[j].IsSelected {
			return rows[i].IsSelected
		}
		if rows[i].FinalRank != rows[j].FinalRank {
			if rows[i].FinalRank == 0 {
				return false
			}
			if rows[j].FinalRank == 0 {
				return true
			}
			return rows[i].FinalRank < rows[j].FinalRank
		}
		if rows[i].BaseScore != rows[j].BaseScore {
			return rows[i].BaseScore > rows[j].BaseScore
		}
		return rows[i].StartMs < rows[j].StartMs
	})

	return p.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("job_id = ?", jobID).Delete(&models.VideoJobGIFCandidate{}).Error; err != nil {
			return err
		}
		return tx.CreateInBatches(rows, 100).Error
	})
}

func (p *Processor) attachGIFCandidateBindings(jobID uint64, in []highlightCandidate) []highlightCandidate {
	if p == nil || p.db == nil || jobID == 0 || len(in) == 0 {
		return in
	}
	type candidateRow struct {
		ID      uint64 `gorm:"column:id"`
		StartMs int    `gorm:"column:start_ms"`
		EndMs   int    `gorm:"column:end_ms"`
	}
	var rows []candidateRow
	if err := p.db.Model(&models.VideoJobGIFCandidate{}).
		Select("id", "start_ms", "end_ms").
		Where("job_id = ?", jobID).
		Order("is_selected DESC, final_rank ASC, id ASC").
		Limit(200).
		Find(&rows).Error; err != nil {
		return in
	}
	if len(rows) == 0 {
		return in
	}
	exactByWindow := map[string]uint64{}
	for _, row := range rows {
		if row.ID == 0 || row.EndMs <= row.StartMs {
			continue
		}
		key := fmt.Sprintf("%d-%d", row.StartMs, row.EndMs)
		if _, exists := exactByWindow[key]; exists {
			continue
		}
		exactByWindow[key] = row.ID
	}

	out := make([]highlightCandidate, 0, len(in))
	for _, item := range in {
		startMs, endMs, ok := normalizeHighlightCandidateWindowMs(item)
		if !ok {
			out = append(out, item)
			continue
		}
		key := fmt.Sprintf("%d-%d", startMs, endMs)
		if matchedID, exists := exactByWindow[key]; exists && matchedID > 0 {
			id := matchedID
			item.CandidateID = &id
			out = append(out, item)
			continue
		}
		bestID := uint64(0)
		bestIOU := 0.0
		for _, row := range rows {
			if row.ID == 0 || row.EndMs <= row.StartMs {
				continue
			}
			iou := windowIOUMs(startMs, endMs, row.StartMs, row.EndMs)
			if iou > bestIOU {
				bestIOU = iou
				bestID = row.ID
			}
		}
		if bestID > 0 && bestIOU >= 0.9 {
			bestID := bestID
			item.CandidateID = &bestID
		}
		out = append(out, item)
	}
	return out
}

func (p *Processor) persistGIFRerankLogs(
	jobID uint64,
	userID uint64,
	before []highlightCandidate,
	after []highlightCandidate,
	profile highlightFeedbackProfile,
) {
	if p == nil || p.db == nil || jobID == 0 || userID == 0 || len(after) == 0 {
		return
	}
	type rankMeta struct {
		Rank  int
		Score float64
	}
	beforeMap := make(map[string]rankMeta, len(before))
	for idx, candidate := range before {
		key := highlightCandidateWindowKey(candidate)
		if key == "" {
			continue
		}
		beforeMap[key] = rankMeta{
			Rank:  idx + 1,
			Score: candidate.Score,
		}
	}
	rows := make([]models.VideoJobGIFRerankLog, 0, len(after))
	for idx, candidate := range after {
		startMs, endMs, ok := normalizeHighlightCandidateWindowMs(candidate)
		if !ok {
			continue
		}
		key := highlightCandidateWindowKey(candidate)
		beforeRank := 0
		beforeScore := candidate.Score
		if prev, exists := beforeMap[key]; exists {
			beforeRank = prev.Rank
			beforeScore = prev.Score
		}
		afterRank := idx + 1
		if beforeRank == afterRank && math.Abs(beforeScore-candidate.Score) < 1e-6 {
			continue
		}
		rows = append(rows, models.VideoJobGIFRerankLog{
			JobID:       jobID,
			UserID:      userID,
			StartMs:     startMs,
			EndMs:       endMs,
			BeforeRank:  beforeRank,
			AfterRank:   afterRank,
			BeforeScore: roundTo(beforeScore, 4),
			AfterScore:  roundTo(candidate.Score, 4),
			ScoreDelta:  roundTo(candidate.Score-beforeScore, 4),
			Reason:      strings.ToLower(strings.TrimSpace(candidate.Reason)),
			Metadata: mustJSON(map[string]interface{}{
				"profile_engaged_jobs":       profile.EngagedJobs,
				"profile_weighted_signals":   roundTo(profile.WeightedSignals, 3),
				"profile_avg_signal_weight":  roundTo(profile.AverageSignalWeight, 3),
				"profile_public_positive":    roundTo(profile.PublicPositiveSignals, 3),
				"profile_public_negative":    roundTo(profile.PublicNegativeSignals, 3),
				"profile_preferred_center":   roundTo(profile.PreferredCenter, 4),
				"profile_preferred_duration": roundTo(profile.PreferredDuration, 4),
				"profile_reason_preference":  profile.ReasonPreference,
				"profile_reason_negative":    profile.ReasonNegativeGuard,
			}),
		})
	}
	if len(rows) == 0 {
		return
	}
	_ = p.db.CreateInBatches(rows, 100).Error
}

func (p *Processor) buildGIFCandidateFeatureSnapshot(
	ctx context.Context,
	sourcePath string,
	meta videoProbeMeta,
	candidate highlightCandidate,
	qualitySettings QualitySettings,
	topScore float64,
	scoreSpread float64,
	selected bool,
	strategy string,
	version string,
) map[string]interface{} {
	durationSec := candidate.EndSec - candidate.StartSec
	if durationSec < 0 {
		durationSec = 0
	}
	confidence := estimateGIFCandidateConfidence(candidate.Score, topScore, scoreSpread, selected)
	feature := map[string]interface{}{
		"scene_score":            roundTo(candidate.SceneScore, 4),
		"reason":                 strings.TrimSpace(candidate.Reason),
		"proposal_rank":          candidate.ProposalRank,
		"window_sec":             roundTo(durationSec, 3),
		"window_center_sec":      roundTo((candidate.StartSec+candidate.EndSec)/2, 3),
		"window_position_ratio":  roundTo(candidateWindowPositionRatio(candidate, meta.DurationSec), 4),
		"strategy":               strings.TrimSpace(suggestionStrategyOrDefault(strategy)),
		"version":                strings.TrimSpace(suggestionVersionOrDefault(version)),
		"base_score":             roundTo(candidate.Score, 4),
		"confidence_score":       roundTo(confidence, 4),
		"confidence_threshold":   roundTo(qualitySettings.GIFCandidateConfidenceThreshold, 4),
		"dedup_iou_threshold":    roundTo(qualitySettings.GIFCandidateDedupIOUThreshold, 4),
		"estimated_size_kb":      roundTo(estimateGIFCandidateSizeKB(meta, candidate, qualitySettings), 2),
		"gif_profile":            strings.TrimSpace(strings.ToLower(qualitySettings.GIFProfile)),
		"gif_default_fps":        qualitySettings.GIFDefaultFPS,
		"gif_default_max_colors": qualitySettings.GIFDefaultMaxColors,
	}
	if candidate.ProposalID != nil && *candidate.ProposalID > 0 {
		feature["proposal_id"] = *candidate.ProposalID
	}
	if sampled := p.sampleGIFCandidateFrameQuality(ctx, sourcePath, candidate, qualitySettings); len(sampled) > 0 {
		for key, value := range sampled {
			feature[key] = value
		}
	}
	return feature
}

func (p *Processor) sampleGIFCandidateFrameQuality(
	ctx context.Context,
	sourcePath string,
	candidate highlightCandidate,
	qualitySettings QualitySettings,
) map[string]interface{} {
	if p == nil || strings.TrimSpace(sourcePath) == "" {
		return nil
	}
	timestamps := buildCandidateSampleTimestamps(candidate.StartSec, candidate.EndSec)
	if len(timestamps) == 0 {
		return nil
	}
	tmpDir, err := os.MkdirTemp("", "gif-candidate-sample-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(tmpDir)

	samples := make([]frameQualitySample, 0, len(timestamps))
	for idx, ts := range timestamps {
		target := filepath.Join(tmpDir, fmt.Sprintf("sample_%02d.jpg", idx))
		if err := extractFrameAtTimestamp(ctx, sourcePath, ts, target); err != nil {
			continue
		}
		sample, ok := analyzeFrameQuality(target)
		if !ok {
			continue
		}
		sample.Index = len(samples)
		samples = append(samples, sample)
	}
	if len(samples) == 0 {
		return map[string]interface{}{
			"sample_count": 0,
		}
	}

	sceneCutThreshold := chooseSceneCutThreshold(samples)
	assignSceneAndMotionScores(samples, sceneCutThreshold)
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	blurThreshold := chooseBlurThreshold(extractSampleBlurScores(samples), qualitySettings)

	brightnessSum := 0.0
	blurSum := 0.0
	subjectSum := 0.0
	exposureSum := 0.0
	motionSum := 0.0
	qualitySum := 0.0
	sceneMax := 1
	for idx := range samples {
		samples[idx].Exposure = roundTo(computeExposureScore(samples[idx].Brightness, qualitySettings), 4)
		samples[idx].QualityScore = roundTo(computeFrameQualityScore(samples[idx], blurThreshold), 4)
		brightnessSum += samples[idx].Brightness
		blurSum += samples[idx].BlurScore
		subjectSum += samples[idx].SubjectScore
		exposureSum += samples[idx].Exposure
		motionSum += samples[idx].MotionScore
		qualitySum += samples[idx].QualityScore
		if samples[idx].SceneID > sceneMax {
			sceneMax = samples[idx].SceneID
		}
	}
	count := float64(len(samples))
	blurMean := blurSum / count
	blurNorm := clampZeroOne(blurMean / maxFloat(qualitySettings.BlurThresholdMin, 12))

	return map[string]interface{}{
		"sample_count":        len(samples),
		"brightness_mean":     roundTo(brightnessSum/count, 3),
		"blur_mean":           roundTo(blurMean, 3),
		"blur_norm":           roundTo(blurNorm, 4),
		"subject_mean":        roundTo(subjectSum/count, 4),
		"exposure_mean":       roundTo(exposureSum/count, 4),
		"motion_mean":         roundTo(motionSum/count, 4),
		"quality_mean":        roundTo(qualitySum/count, 4),
		"scene_count":         sceneMax,
		"scene_cut_threshold": roundTo(sceneCutThreshold, 3),
		"blur_threshold":      roundTo(blurThreshold, 3),
	}
}

func buildCandidateSampleTimestamps(startSec, endSec float64) []float64 {
	if endSec <= startSec {
		return nil
	}
	duration := endSec - startSec
	if duration <= 0 {
		return nil
	}
	lead := startSec + duration*0.12
	mid := startSec + duration*0.5
	tail := endSec - duration*0.12
	seq := []float64{lead, mid, tail}
	out := make([]float64, 0, len(seq))
	for _, ts := range seq {
		if ts <= startSec {
			ts = startSec + duration*0.05
		}
		if ts >= endSec {
			ts = endSec - duration*0.05
		}
		if ts <= startSec || ts >= endSec {
			continue
		}
		out = append(out, ts)
	}
	return out
}

func extractSampleBlurScores(samples []frameQualitySample) []float64 {
	out := make([]float64, 0, len(samples))
	for _, item := range samples {
		if item.BlurScore > 0 {
			out = append(out, item.BlurScore)
		}
	}
	return out
}

func estimateGIFCandidateSizeKB(meta videoProbeMeta, candidate highlightCandidate, qualitySettings QualitySettings) float64 {
	width := meta.Width
	height := meta.Height
	if width <= 0 || height <= 0 {
		width = 480
		height = 270
	}
	durationSec := candidate.EndSec - candidate.StartSec
	if durationSec <= 0 {
		durationSec = 2.4
	}
	fps := qualitySettings.GIFDefaultFPS
	if fps <= 0 {
		fps = 12
	}
	maxColors := qualitySettings.GIFDefaultMaxColors
	if maxColors <= 0 {
		maxColors = 128
	}
	pixels := float64(width * height)
	colorFactor := 0.6 + clampZeroOne(float64(maxColors)/256.0)*0.8
	frameFactor := float64(fps) / 12.0
	rawBytes := pixels * durationSec * frameFactor * colorFactor * 0.065
	return math.Max(32, rawBytes/1024.0)
}

func candidateWindowPositionRatio(candidate highlightCandidate, totalDurationSec float64) float64 {
	if totalDurationSec <= 0 {
		return 0
	}
	center := (candidate.StartSec + candidate.EndSec) / 2
	if center < 0 {
		center = 0
	}
	return clampZeroOne(center / totalDurationSec)
}

func suggestionStrategyOrDefault(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "scene_score"
	}
	return value
}

func suggestionVersionOrDefault(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "v1"
	}
	return value
}

func highlightCandidateWindowKey(candidate highlightCandidate) string {
	startMs, endMs, ok := normalizeHighlightCandidateWindowMs(candidate)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d-%d", startMs, endMs)
}

func normalizeHighlightCandidateWindowMs(candidate highlightCandidate) (int, int, bool) {
	if candidate.EndSec <= candidate.StartSec {
		return 0, 0, false
	}
	startMs := int(math.Round(candidate.StartSec * 1000))
	endMs := int(math.Round(candidate.EndSec * 1000))
	if startMs < 0 {
		startMs = 0
	}
	if endMs <= startMs {
		return 0, 0, false
	}
	return startMs, endMs, true
}

func inferGIFCandidateRejectReason(
	candidate highlightCandidate,
	selected []highlightCandidate,
	dedupIOUThreshold float64,
	confidenceThreshold float64,
	topScore float64,
	scoreSpread float64,
	feature map[string]interface{},
	qualitySettings QualitySettings,
) string {
	if dedupIOUThreshold <= 0 {
		dedupIOUThreshold = 0.45
	}
	for _, picked := range selected {
		if windowIoU(candidate.StartSec, candidate.EndSec, picked.StartSec, picked.EndSec) > dedupIOUThreshold {
			return GIFCandidateRejectReasonDuplicate
		}
	}
	if confidenceThreshold > 0 {
		confidence := estimateGIFCandidateConfidence(candidate.Score, topScore, scoreSpread, false)
		if confidence < confidenceThreshold {
			return GIFCandidateRejectReasonLowConfidence
		}
	}
	if isGIFCandidateBlurLow(feature, qualitySettings) {
		return GIFCandidateRejectReasonBlurLow
	}
	if isGIFCandidateSizeBudgetExceeded(feature, qualitySettings) {
		return GIFCandidateRejectReasonSizeBudgetExceeded
	}
	if isGIFCandidateLoopPoor(feature, qualitySettings) {
		return GIFCandidateRejectReasonLoopPoor
	}
	return GIFCandidateRejectReasonLowEmotion
}

func isGIFCandidateBlurLow(feature map[string]interface{}, qualitySettings QualitySettings) bool {
	if len(feature) == 0 {
		return false
	}
	blurMean := floatFromAny(feature["blur_mean"])
	if blurMean <= 0 {
		return false
	}
	blurFloor := qualitySettings.StillMinBlurScore
	if blurFloor <= 0 {
		blurFloor = DefaultQualitySettings().StillMinBlurScore
	}
	blurThreshold := floatFromAny(feature["blur_threshold"])
	if blurThreshold > 0 {
		blurFloor = maxFloat(blurFloor, blurThreshold*0.82)
	}
	return blurMean < blurFloor
}

func isGIFCandidateSizeBudgetExceeded(feature map[string]interface{}, qualitySettings QualitySettings) bool {
	if len(feature) == 0 {
		return false
	}
	estimatedSizeKB := floatFromAny(feature["estimated_size_kb"])
	if estimatedSizeKB <= 0 {
		return false
	}
	targetSizeKB := float64(qualitySettings.GIFTargetSizeKB)
	if targetSizeKB <= 0 {
		targetSizeKB = float64(DefaultQualitySettings().GIFTargetSizeKB)
	}
	return estimatedSizeKB > targetSizeKB*1.18
}

func isGIFCandidateLoopPoor(feature map[string]interface{}, qualitySettings QualitySettings) bool {
	if len(feature) == 0 {
		return false
	}
	motionMean := floatFromAny(feature["motion_mean"])
	sceneCount := floatFromAny(feature["scene_count"])
	qualityMean := floatFromAny(feature["quality_mean"])
	loopMotionTarget := qualitySettings.GIFLoopTuneMotionTarget
	if loopMotionTarget <= 0 {
		loopMotionTarget = DefaultQualitySettings().GIFLoopTuneMotionTarget
	}
	if motionMean > maxFloat(loopMotionTarget*2.2, 0.48) && sceneCount >= 2 {
		return true
	}
	if qualityMean > 0 && qualityMean < 0.34 && sceneCount >= 2 {
		return true
	}
	return false
}

func applyGIFCandidateConfidenceThreshold(
	selected []highlightCandidate,
	reference []highlightCandidate,
	confidenceThreshold float64,
) []highlightCandidate {
	if len(selected) == 0 || confidenceThreshold <= 0 {
		return selected
	}
	pool := reference
	if len(pool) == 0 {
		pool = selected
	}
	topScore := pool[0].Score
	minScore := pool[0].Score
	for _, candidate := range pool {
		if candidate.Score > topScore {
			topScore = candidate.Score
		}
		if candidate.Score < minScore {
			minScore = candidate.Score
		}
	}
	scoreSpread := topScore - minScore
	if scoreSpread < 0 {
		scoreSpread = 0
	}

	filtered := make([]highlightCandidate, 0, len(selected))
	for idx, candidate := range selected {
		confidence := estimateGIFCandidateConfidence(candidate.Score, topScore, scoreSpread, idx == 0)
		if confidence >= confidenceThreshold || len(filtered) == 0 {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func estimateGIFCandidateConfidence(score, topScore, spread float64, selected bool) float64 {
	base := clampZeroOne(score)
	if topScore > 0 {
		base = clampZeroOne(base - math.Max(0, topScore-score)*0.35)
	}
	if spread > 0 && spread < 0.06 {
		base *= 0.85
	}
	if selected {
		base = clampZeroOne(base + 0.05)
	}
	return base
}

type videoProbeMeta struct {
	DurationSec float64
	Width       int
	Height      int
	FPS         float64
}

type ffprobeJSON struct {
	Streams []struct {
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		RFrameRate string `json:"r_frame_rate"`
		Duration   string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}
