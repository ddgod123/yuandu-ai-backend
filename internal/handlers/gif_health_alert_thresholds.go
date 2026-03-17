package handlers

import "emoji/internal/models"

type GIFHealthAlertThresholdSettings struct {
	GIFHealthDoneRateWarn             float64 `json:"gif_health_done_rate_warn"`
	GIFHealthDoneRateCritical         float64 `json:"gif_health_done_rate_critical"`
	GIFHealthFailedRateWarn           float64 `json:"gif_health_failed_rate_warn"`
	GIFHealthFailedRateCritical       float64 `json:"gif_health_failed_rate_critical"`
	GIFHealthPathStrictRateWarn       float64 `json:"gif_health_path_strict_rate_warn"`
	GIFHealthPathStrictRateCritical   float64 `json:"gif_health_path_strict_rate_critical"`
	GIFHealthLoopFallbackRateWarn     float64 `json:"gif_health_loop_fallback_rate_warn"`
	GIFHealthLoopFallbackRateCritical float64 `json:"gif_health_loop_fallback_rate_critical"`
}

func defaultGIFHealthAlertThresholdSettings() GIFHealthAlertThresholdSettings {
	return GIFHealthAlertThresholdSettings{
		GIFHealthDoneRateWarn:             0.85,
		GIFHealthDoneRateCritical:         0.60,
		GIFHealthFailedRateWarn:           0.15,
		GIFHealthFailedRateCritical:       0.30,
		GIFHealthPathStrictRateWarn:       0.90,
		GIFHealthPathStrictRateCritical:   0.50,
		GIFHealthLoopFallbackRateWarn:     0.40,
		GIFHealthLoopFallbackRateCritical: 0.70,
	}
}

func gifHealthAlertThresholdSettingsFromModel(setting models.VideoQualitySetting) GIFHealthAlertThresholdSettings {
	return normalizeGIFHealthAlertThresholdSettings(GIFHealthAlertThresholdSettings{
		GIFHealthDoneRateWarn:             setting.GIFHealthDoneRateWarn,
		GIFHealthDoneRateCritical:         setting.GIFHealthDoneRateCritical,
		GIFHealthFailedRateWarn:           setting.GIFHealthFailedRateWarn,
		GIFHealthFailedRateCritical:       setting.GIFHealthFailedRateCritical,
		GIFHealthPathStrictRateWarn:       setting.GIFHealthPathStrictRateWarn,
		GIFHealthPathStrictRateCritical:   setting.GIFHealthPathStrictRateCritical,
		GIFHealthLoopFallbackRateWarn:     setting.GIFHealthLoopFallbackRateWarn,
		GIFHealthLoopFallbackRateCritical: setting.GIFHealthLoopFallbackRateCritical,
	})
}

func applyGIFHealthAlertThresholdSettingsToModel(dst *models.VideoQualitySetting, input GIFHealthAlertThresholdSettings) {
	if dst == nil {
		return
	}
	normalized := normalizeGIFHealthAlertThresholdSettings(input)
	dst.GIFHealthDoneRateWarn = normalized.GIFHealthDoneRateWarn
	dst.GIFHealthDoneRateCritical = normalized.GIFHealthDoneRateCritical
	dst.GIFHealthFailedRateWarn = normalized.GIFHealthFailedRateWarn
	dst.GIFHealthFailedRateCritical = normalized.GIFHealthFailedRateCritical
	dst.GIFHealthPathStrictRateWarn = normalized.GIFHealthPathStrictRateWarn
	dst.GIFHealthPathStrictRateCritical = normalized.GIFHealthPathStrictRateCritical
	dst.GIFHealthLoopFallbackRateWarn = normalized.GIFHealthLoopFallbackRateWarn
	dst.GIFHealthLoopFallbackRateCritical = normalized.GIFHealthLoopFallbackRateCritical
}

func normalizeGIFHealthAlertThresholdSettings(in GIFHealthAlertThresholdSettings) GIFHealthAlertThresholdSettings {
	def := defaultGIFHealthAlertThresholdSettings()
	out := in
	if out.GIFHealthDoneRateWarn == 0 &&
		out.GIFHealthDoneRateCritical == 0 &&
		out.GIFHealthFailedRateWarn == 0 &&
		out.GIFHealthFailedRateCritical == 0 &&
		out.GIFHealthPathStrictRateWarn == 0 &&
		out.GIFHealthPathStrictRateCritical == 0 &&
		out.GIFHealthLoopFallbackRateWarn == 0 &&
		out.GIFHealthLoopFallbackRateCritical == 0 {
		return def
	}

	out.GIFHealthDoneRateWarn = clampOrDefault(out.GIFHealthDoneRateWarn, 0.50, 0.99, def.GIFHealthDoneRateWarn)
	out.GIFHealthDoneRateCritical = clampOrDefault(out.GIFHealthDoneRateCritical, 0.01, 0.98, def.GIFHealthDoneRateCritical)
	if out.GIFHealthDoneRateCritical >= out.GIFHealthDoneRateWarn {
		out.GIFHealthDoneRateCritical = clampFloatLocal(out.GIFHealthDoneRateWarn-0.05, 0.01, out.GIFHealthDoneRateWarn-0.01)
	}

	out.GIFHealthFailedRateWarn = clampOrDefault(out.GIFHealthFailedRateWarn, 0.01, 0.95, def.GIFHealthFailedRateWarn)
	out.GIFHealthFailedRateCritical = clampOrDefault(out.GIFHealthFailedRateCritical, 0.02, 0.99, def.GIFHealthFailedRateCritical)
	if out.GIFHealthFailedRateCritical <= out.GIFHealthFailedRateWarn {
		out.GIFHealthFailedRateCritical = clampFloatLocal(out.GIFHealthFailedRateWarn+0.05, out.GIFHealthFailedRateWarn+0.01, 0.99)
	}

	out.GIFHealthPathStrictRateWarn = clampOrDefault(out.GIFHealthPathStrictRateWarn, 0.50, 0.99, def.GIFHealthPathStrictRateWarn)
	out.GIFHealthPathStrictRateCritical = clampOrDefault(out.GIFHealthPathStrictRateCritical, 0.01, 0.98, def.GIFHealthPathStrictRateCritical)
	if out.GIFHealthPathStrictRateCritical >= out.GIFHealthPathStrictRateWarn {
		out.GIFHealthPathStrictRateCritical = clampFloatLocal(out.GIFHealthPathStrictRateWarn-0.05, 0.01, out.GIFHealthPathStrictRateWarn-0.01)
	}

	out.GIFHealthLoopFallbackRateWarn = clampOrDefault(out.GIFHealthLoopFallbackRateWarn, 0.01, 0.95, def.GIFHealthLoopFallbackRateWarn)
	out.GIFHealthLoopFallbackRateCritical = clampOrDefault(out.GIFHealthLoopFallbackRateCritical, 0.02, 0.99, def.GIFHealthLoopFallbackRateCritical)
	if out.GIFHealthLoopFallbackRateCritical <= out.GIFHealthLoopFallbackRateWarn {
		out.GIFHealthLoopFallbackRateCritical = clampFloatLocal(out.GIFHealthLoopFallbackRateWarn+0.05, out.GIFHealthLoopFallbackRateWarn+0.01, 0.99)
	}

	return out
}

func clampOrDefault(value, min, max, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return clampFloatLocal(value, min, max)
}

func clampFloatLocal(value, min, max float64) float64 {
	if min > max {
		min, max = max, min
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
