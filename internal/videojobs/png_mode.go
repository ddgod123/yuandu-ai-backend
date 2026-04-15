package videojobs

import (
	"math"
	"strings"
)

const (
	PNGModeSmartLLM    = "smart_llm"
	PNGModeFastExtract = "fast_extract"

	PNGFastExtractFPS1 = 1
	PNGFastExtractFPS2 = 2
)

func NormalizePNGMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "smart", "smart_llm", "llm_smart", "large_video", "large-video":
		return PNGModeSmartLLM
	case "fast", "fast_extract", "normal", "normal_extract", "by_second":
		return PNGModeFastExtract
	default:
		return PNGModeSmartLLM
	}
}

func NormalizePNGFastExtractFPS(raw int) int {
	switch raw {
	case PNGFastExtractFPS2:
		return PNGFastExtractFPS2
	default:
		return PNGFastExtractFPS1
	}
}

func ResolvePNGPipelineMode(primaryFormat string, options map[string]interface{}) string {
	if NormalizeRequestedFormat(primaryFormat) != "png" {
		return PNGModeSmartLLM
	}
	return NormalizePNGMode(stringFromAny(options["png_mode"]))
}

func ResolvePNGFastExtractFPS(options map[string]interface{}) int {
	rawFPS := intFromAny(options["fast_extract_fps"])
	if rawFPS > 0 {
		return NormalizePNGFastExtractFPS(rawFPS)
	}
	interval := floatFromAny(options["frame_interval_sec"])
	if interval <= 0 {
		return PNGFastExtractFPS1
	}
	derived := int(math.Round(1.0 / interval))
	return NormalizePNGFastExtractFPS(derived)
}

func ResolvePNGFastExtractIntervalSec(options map[string]interface{}) float64 {
	fps := ResolvePNGFastExtractFPS(options)
	if fps <= 0 {
		fps = PNGFastExtractFPS1
	}
	return roundTo(1.0/float64(fps), 3)
}

func IsPNGFastExtractByOptions(options map[string]interface{}) bool {
	return NormalizePNGMode(stringFromAny(options["png_mode"])) == PNGModeFastExtract
}
