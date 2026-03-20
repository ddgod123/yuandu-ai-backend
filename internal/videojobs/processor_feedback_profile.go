package videojobs

import (
	"math"
	"strings"

	"emoji/internal/models"
)

func (p *Processor) loadUserHighlightFeedbackProfile(
	userID uint64,
	limit int,
	qualitySettings QualitySettings,
) (highlightFeedbackProfile, error) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	profile := highlightFeedbackProfile{
		ReasonPreference:    map[string]float64{},
		ReasonNegativeGuard: map[string]float64{},
		ScenePreference:     map[string]float64{},
	}
	if p == nil || p.db == nil || userID == 0 {
		return profile, nil
	}
	if limit <= 0 {
		limit = 80
	}
	if limit > 200 {
		limit = 200
	}

	var jobs []models.VideoJob
	if err := p.db.
		Select("id", "metrics").
		Where("user_id = ? AND status = ?", userID, models.VideoJobStatusDone).
		Order("finished_at DESC NULLS LAST, id DESC").
		Limit(limit).
		Find(&jobs).Error; err != nil {
		return profile, err
	}
	if len(jobs) == 0 {
		return profile, nil
	}

	jobIDs := make([]uint64, 0, len(jobs))
	for _, job := range jobs {
		jobIDs = append(jobIDs, job.ID)
	}

	allowLegacyFallback := p.cfg.EnableLegacyFeedbackFallback
	publicSignalSummaryByJob, err := p.loadUserPublicFeedbackSignalSummary(userID, jobIDs)
	if err != nil {
		if !allowLegacyFallback {
			// 严格链路下，公共反馈读取失败时直接降级为“无画像”，不消费 legacy metrics。
			return profile, nil
		}
		publicSignalSummaryByJob = nil
	}
	if !allowLegacyFallback && len(publicSignalSummaryByJob) == 0 {
		return profile, nil
	}

	totalWeight := 0.0
	centerWeighted := 0.0
	durationWeighted := 0.0
	reasonWeights := map[string]float64{}
	reasonPositiveWeights := map[string]float64{}
	reasonNegativeWeights := map[string]float64{}
	sceneWeights := map[string]float64{}

	for _, job := range jobs {
		metrics := parseJSONMap(job.Metrics)
		highlight := mapFromAny(metrics["highlight_v1"])
		selected := mapFromAny(highlight["selected"])
		if len(selected) == 0 {
			continue
		}
		selectedReason := normalizeReason(selected["reason"])

		durationSec := floatFromAny(metrics["duration_sec"])
		startSec := floatFromAny(selected["start_sec"])
		endSec := floatFromAny(selected["end_sec"])
		if durationSec <= 0 && endSec > 0 {
			durationSec = endSec
		}

		publicSummary, hasPublicFeedback := publicSignalSummaryByJob[job.ID]
		if hasPublicFeedback {
			profile.PublicPositiveSignals += publicSummary.PositiveWeight
			profile.PublicNegativeSignals += publicSummary.NegativeWeight

			usedOutputDetails := false
			jobPositiveWeight := 0.0
			for _, detail := range publicSummary.Details {
				usedOutputDetails = true
				prefWeight := detail.Weight
				if prefWeight < 0 {
					prefWeight = 0
				}

				signalStartSec := detail.StartSec
				signalEndSec := detail.EndSec
				if signalEndSec <= signalStartSec {
					signalStartSec = startSec
					signalEndSec = endSec
				}
				if prefWeight > 0 && durationSec > 0 && signalEndSec > signalStartSec {
					mid := clampZeroOne(((signalStartSec + signalEndSec) / 2) / durationSec)
					windowDuration := clampZeroOne((signalEndSec - signalStartSec) / durationSec)
					centerWeighted += mid * prefWeight
					durationWeighted += windowDuration * prefWeight
					totalWeight += prefWeight
					profile.WeightedSignals += prefWeight
					jobPositiveWeight += prefWeight
				}

				reason := normalizeReason(detail.Reason)
				if reason == "" {
					reason = selectedReason
				}
				if reason != "" {
					if prefWeight > 0 && durationSec > 0 && signalEndSec > signalStartSec {
						reasonWeights[reason] += prefWeight
						reasonPositiveWeights[reason] += prefWeight
					}
					if detail.Weight < 0 {
						reasonNegativeWeights[reason] += -detail.Weight
					}
				}

				sceneTag := strings.TrimSpace(strings.ToLower(detail.SceneTag))
				if sceneTag != "" && prefWeight > 0 && durationSec > 0 && signalEndSec > signalStartSec {
					sceneWeights[sceneTag] += prefWeight
				}
			}
			if selectedReason != "" {
				if publicSummary.PositiveWeight > 0 && !usedOutputDetails {
					reasonPositiveWeights[selectedReason] += publicSummary.PositiveWeight
				}
				if publicSummary.NegativeWeight > 0 {
					reasonNegativeWeights[selectedReason] += publicSummary.NegativeWeight
				}
			}
			if !usedOutputDetails {
				signalWeight := publicSummary.PositiveWeight
				if signalWeight > 0 && durationSec > 0 && endSec > startSec {
					mid := clampZeroOne(((startSec + endSec) / 2) / durationSec)
					windowDuration := clampZeroOne((endSec - startSec) / durationSec)
					centerWeighted += mid * signalWeight
					durationWeighted += windowDuration * signalWeight
					totalWeight += signalWeight
					profile.WeightedSignals += signalWeight

					if selectedReason != "" {
						reasonWeights[selectedReason] += signalWeight
						reasonPositiveWeights[selectedReason] += signalWeight
					}

					tags := stringSliceFromAny(metrics["scene_tags_v1"])
					for _, tag := range tags {
						sceneWeights[tag] += signalWeight
					}
					jobPositiveWeight += signalWeight
				}
			}
			if jobPositiveWeight > 0 || publicSummary.NegativeWeight > 0 {
				profile.EngagedJobs++
			}
			continue
		}

		if !allowLegacyFallback {
			continue
		}

		feedback := mapFromAny(metrics["feedback_v1"])
		if len(feedback) == 0 {
			feedback = map[string]interface{}{}
		}
		totalSignals := floatFromAny(feedback["total_signals"])
		if totalSignals <= 0 {
			totalSignals = 0
		}
		if durationSec <= 0 || endSec <= startSec {
			continue
		}

		mid := clampZeroOne(((startSec + endSec) / 2) / durationSec)
		windowDuration := clampZeroOne((endSec - startSec) / durationSec)

		favoriteCount := floatFromAny(feedback["favorite_count"])
		signalWeight := totalSignals + favoriteCount*0.8
		if signalWeight <= 0 {
			continue
		}

		centerWeighted += mid * signalWeight
		durationWeighted += windowDuration * signalWeight
		totalWeight += signalWeight
		profile.WeightedSignals += signalWeight
		profile.EngagedJobs++

		if selectedReason != "" {
			reasonWeights[selectedReason] += signalWeight
			reasonPositiveWeights[selectedReason] += signalWeight
		}

		tags := stringSliceFromAny(metrics["scene_tags_v1"])
		for _, tag := range tags {
			sceneWeights[tag] += signalWeight
		}
	}

	if profile.EngagedJobs == 0 || totalWeight <= 0 {
		return profile, nil
	}

	profile.PreferredCenter = centerWeighted / totalWeight
	profile.PreferredDuration = durationWeighted / totalWeight
	profile.AverageSignalWeight = profile.WeightedSignals / float64(profile.EngagedJobs)

	for reason, value := range reasonWeights {
		if value <= 0 {
			continue
		}
		profile.ReasonPreference[reason] = value / totalWeight
	}
	if qualitySettings.HighlightNegativeGuardEnabled {
		for reason, negWeight := range reasonNegativeWeights {
			if negWeight <= 0 {
				continue
			}
			posWeight := reasonPositiveWeights[reason]
			total := posWeight + negWeight
			if total <= 0 {
				continue
			}
			dominance := negWeight / total
			threshold := qualitySettings.HighlightNegativeGuardThreshold
			if dominance <= threshold {
				continue
			}
			confidence := math.Min(1, negWeight/qualitySettings.HighlightNegativeGuardMinWeight)
			rangeWidth := 1 - threshold
			if rangeWidth <= 0 {
				rangeWidth = 0.01
			}
			guard := clampZeroOne((dominance-threshold)/rangeWidth) * confidence
			if guard <= 0 {
				continue
			}
			profile.ReasonNegativeGuard[reason] = roundTo(guard, 4)
		}
	}
	for tag, value := range sceneWeights {
		if value <= 0 {
			continue
		}
		profile.ScenePreference[tag] = value / totalWeight
	}
	return profile, nil
}
