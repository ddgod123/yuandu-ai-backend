package handlers

import "emoji/internal/models"

type FeedbackIntegrityAlertThresholdSettings struct {
	FeedbackIntegrityOutputCoverageRateWarn           float64 `json:"feedback_integrity_output_coverage_rate_warn"`
	FeedbackIntegrityOutputCoverageRateCritical       float64 `json:"feedback_integrity_output_coverage_rate_critical"`
	FeedbackIntegrityOutputResolvedRateWarn           float64 `json:"feedback_integrity_output_resolved_rate_warn"`
	FeedbackIntegrityOutputResolvedRateCritical       float64 `json:"feedback_integrity_output_resolved_rate_critical"`
	FeedbackIntegrityOutputJobConsistencyRateWarn     float64 `json:"feedback_integrity_output_job_consistency_rate_warn"`
	FeedbackIntegrityOutputJobConsistencyRateCritical float64 `json:"feedback_integrity_output_job_consistency_rate_critical"`
	FeedbackIntegrityTopPickConflictUsersWarn         int     `json:"feedback_integrity_top_pick_conflict_users_warn"`
	FeedbackIntegrityTopPickConflictUsersCritical     int     `json:"feedback_integrity_top_pick_conflict_users_critical"`
}

func defaultFeedbackIntegrityAlertThresholdSettings() FeedbackIntegrityAlertThresholdSettings {
	return FeedbackIntegrityAlertThresholdSettings{
		FeedbackIntegrityOutputCoverageRateWarn:           0.98,
		FeedbackIntegrityOutputCoverageRateCritical:       0.95,
		FeedbackIntegrityOutputResolvedRateWarn:           0.99,
		FeedbackIntegrityOutputResolvedRateCritical:       0.97,
		FeedbackIntegrityOutputJobConsistencyRateWarn:     0.999,
		FeedbackIntegrityOutputJobConsistencyRateCritical: 0.995,
		FeedbackIntegrityTopPickConflictUsersWarn:         1,
		FeedbackIntegrityTopPickConflictUsersCritical:     3,
	}
}

func feedbackIntegrityAlertThresholdSettingsFromModel(setting models.VideoQualitySetting) FeedbackIntegrityAlertThresholdSettings {
	return normalizeFeedbackIntegrityAlertThresholdSettings(FeedbackIntegrityAlertThresholdSettings{
		FeedbackIntegrityOutputCoverageRateWarn:           setting.FeedbackIntegrityOutputCoverageRateWarn,
		FeedbackIntegrityOutputCoverageRateCritical:       setting.FeedbackIntegrityOutputCoverageRateCritical,
		FeedbackIntegrityOutputResolvedRateWarn:           setting.FeedbackIntegrityOutputResolvedRateWarn,
		FeedbackIntegrityOutputResolvedRateCritical:       setting.FeedbackIntegrityOutputResolvedRateCritical,
		FeedbackIntegrityOutputJobConsistencyRateWarn:     setting.FeedbackIntegrityOutputJobConsistencyRateWarn,
		FeedbackIntegrityOutputJobConsistencyRateCritical: setting.FeedbackIntegrityOutputJobConsistencyRateCritical,
		FeedbackIntegrityTopPickConflictUsersWarn:         setting.FeedbackIntegrityTopPickConflictUsersWarn,
		FeedbackIntegrityTopPickConflictUsersCritical:     setting.FeedbackIntegrityTopPickConflictUsersCritical,
	})
}

func applyFeedbackIntegrityAlertThresholdSettingsToModel(dst *models.VideoQualitySetting, input FeedbackIntegrityAlertThresholdSettings) {
	if dst == nil {
		return
	}
	normalized := normalizeFeedbackIntegrityAlertThresholdSettings(input)
	dst.FeedbackIntegrityOutputCoverageRateWarn = normalized.FeedbackIntegrityOutputCoverageRateWarn
	dst.FeedbackIntegrityOutputCoverageRateCritical = normalized.FeedbackIntegrityOutputCoverageRateCritical
	dst.FeedbackIntegrityOutputResolvedRateWarn = normalized.FeedbackIntegrityOutputResolvedRateWarn
	dst.FeedbackIntegrityOutputResolvedRateCritical = normalized.FeedbackIntegrityOutputResolvedRateCritical
	dst.FeedbackIntegrityOutputJobConsistencyRateWarn = normalized.FeedbackIntegrityOutputJobConsistencyRateWarn
	dst.FeedbackIntegrityOutputJobConsistencyRateCritical = normalized.FeedbackIntegrityOutputJobConsistencyRateCritical
	dst.FeedbackIntegrityTopPickConflictUsersWarn = normalized.FeedbackIntegrityTopPickConflictUsersWarn
	dst.FeedbackIntegrityTopPickConflictUsersCritical = normalized.FeedbackIntegrityTopPickConflictUsersCritical
}

func normalizeFeedbackIntegrityAlertThresholdSettings(in FeedbackIntegrityAlertThresholdSettings) FeedbackIntegrityAlertThresholdSettings {
	def := defaultFeedbackIntegrityAlertThresholdSettings()
	out := in
	if out.FeedbackIntegrityOutputCoverageRateWarn == 0 &&
		out.FeedbackIntegrityOutputCoverageRateCritical == 0 &&
		out.FeedbackIntegrityOutputResolvedRateWarn == 0 &&
		out.FeedbackIntegrityOutputResolvedRateCritical == 0 &&
		out.FeedbackIntegrityOutputJobConsistencyRateWarn == 0 &&
		out.FeedbackIntegrityOutputJobConsistencyRateCritical == 0 &&
		out.FeedbackIntegrityTopPickConflictUsersWarn == 0 &&
		out.FeedbackIntegrityTopPickConflictUsersCritical == 0 {
		return def
	}

	out.FeedbackIntegrityOutputCoverageRateWarn = clampOrDefault(out.FeedbackIntegrityOutputCoverageRateWarn, 0.50, 1.00, def.FeedbackIntegrityOutputCoverageRateWarn)
	out.FeedbackIntegrityOutputCoverageRateCritical = clampOrDefault(out.FeedbackIntegrityOutputCoverageRateCritical, 0.01, 0.999, def.FeedbackIntegrityOutputCoverageRateCritical)
	if out.FeedbackIntegrityOutputCoverageRateCritical >= out.FeedbackIntegrityOutputCoverageRateWarn {
		out.FeedbackIntegrityOutputCoverageRateCritical = clampFloatLocal(out.FeedbackIntegrityOutputCoverageRateWarn-0.01, 0.01, out.FeedbackIntegrityOutputCoverageRateWarn-0.001)
	}

	out.FeedbackIntegrityOutputResolvedRateWarn = clampOrDefault(out.FeedbackIntegrityOutputResolvedRateWarn, 0.50, 1.00, def.FeedbackIntegrityOutputResolvedRateWarn)
	out.FeedbackIntegrityOutputResolvedRateCritical = clampOrDefault(out.FeedbackIntegrityOutputResolvedRateCritical, 0.01, 0.999, def.FeedbackIntegrityOutputResolvedRateCritical)
	if out.FeedbackIntegrityOutputResolvedRateCritical >= out.FeedbackIntegrityOutputResolvedRateWarn {
		out.FeedbackIntegrityOutputResolvedRateCritical = clampFloatLocal(out.FeedbackIntegrityOutputResolvedRateWarn-0.01, 0.01, out.FeedbackIntegrityOutputResolvedRateWarn-0.001)
	}

	out.FeedbackIntegrityOutputJobConsistencyRateWarn = clampOrDefault(out.FeedbackIntegrityOutputJobConsistencyRateWarn, 0.50, 1.00, def.FeedbackIntegrityOutputJobConsistencyRateWarn)
	out.FeedbackIntegrityOutputJobConsistencyRateCritical = clampOrDefault(out.FeedbackIntegrityOutputJobConsistencyRateCritical, 0.01, 0.999, def.FeedbackIntegrityOutputJobConsistencyRateCritical)
	if out.FeedbackIntegrityOutputJobConsistencyRateCritical >= out.FeedbackIntegrityOutputJobConsistencyRateWarn {
		out.FeedbackIntegrityOutputJobConsistencyRateCritical = clampFloatLocal(out.FeedbackIntegrityOutputJobConsistencyRateWarn-0.001, 0.01, out.FeedbackIntegrityOutputJobConsistencyRateWarn-0.0001)
	}

	if out.FeedbackIntegrityTopPickConflictUsersWarn <= 0 {
		out.FeedbackIntegrityTopPickConflictUsersWarn = def.FeedbackIntegrityTopPickConflictUsersWarn
	}
	if out.FeedbackIntegrityTopPickConflictUsersCritical <= 0 {
		out.FeedbackIntegrityTopPickConflictUsersCritical = def.FeedbackIntegrityTopPickConflictUsersCritical
	}
	if out.FeedbackIntegrityTopPickConflictUsersCritical <= out.FeedbackIntegrityTopPickConflictUsersWarn {
		out.FeedbackIntegrityTopPickConflictUsersCritical = out.FeedbackIntegrityTopPickConflictUsersWarn + 1
	}

	return out
}
