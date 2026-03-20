package videojobs

import (
	"strings"

	"emoji/internal/models"
)

const (
	gifPipelineModeLight    = "light"
	gifPipelineModeStandard = "standard"
	gifPipelineModeHQ       = "hq"
)

type gifPipelineModeDecision struct {
	Mode          string
	RequestedMode string
	Reason        string
	EnableAI1     bool
	EnableAI2     bool
	EnableAI3     bool
}

func resolveGIFPipelineMode(
	job models.VideoJob,
	meta videoProbeMeta,
	options map[string]interface{},
	qualitySettings QualitySettings,
) gifPipelineModeDecision {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	requestedMode := normalizeRequestedGIFPipelineMode(stringFromAny(options["gif_pipeline_mode"]))
	reason := ""
	mode := requestedMode
	if mode == "" {
		priority := strings.ToLower(strings.TrimSpace(job.Priority))
		switch {
		case qualitySettings.GIFPipelineHighPriorityEnabled && (priority == "high" || priority == "urgent"):
			mode = normalizeRequestedGIFPipelineMode(qualitySettings.GIFPipelineHighPriorityMode)
			if mode == "" {
				mode = gifPipelineModeHQ
			}
			reason = "auto_priority_high"
		case meta.DurationSec > 0 && meta.DurationSec <= qualitySettings.GIFPipelineShortVideoMaxSec:
			mode = normalizeRequestedGIFPipelineMode(qualitySettings.GIFPipelineShortVideoMode)
			if mode == "" {
				mode = gifPipelineModeLight
			}
			reason = "auto_short_video"
		case meta.DurationSec >= qualitySettings.GIFPipelineLongVideoMinSec:
			mode = normalizeRequestedGIFPipelineMode(qualitySettings.GIFPipelineLongVideoMode)
			if mode == "" {
				mode = gifPipelineModeLight
			}
			reason = "auto_long_video_cost_control"
		default:
			mode = normalizeRequestedGIFPipelineMode(qualitySettings.GIFPipelineDefaultMode)
			if mode == "" {
				mode = gifPipelineModeStandard
			}
			reason = "auto_default"
		}
	} else {
		reason = "requested"
	}
	if mode == "" {
		mode = gifPipelineModeStandard
	}
	return gifPipelineModeDecision{
		Mode:          mode,
		RequestedMode: requestedMode,
		Reason:        reason,
		EnableAI1:     mode == gifPipelineModeHQ,
		EnableAI2:     mode == gifPipelineModeHQ || mode == gifPipelineModeStandard,
		EnableAI3:     mode == gifPipelineModeHQ || mode == gifPipelineModeStandard,
	}
}

func normalizeRequestedGIFPipelineMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case gifPipelineModeLight:
		return gifPipelineModeLight
	case gifPipelineModeStandard:
		return gifPipelineModeStandard
	case gifPipelineModeHQ:
		return gifPipelineModeHQ
	default:
		return ""
	}
}
