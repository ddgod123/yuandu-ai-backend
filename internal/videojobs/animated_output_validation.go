package videojobs

import "strings"

func invalidAnimatedOutputReason(format string, sizeBytes int64) string {
	if sizeBytes <= 0 {
		return "non_positive_size"
	}
	cleanFormat := strings.ToLower(strings.TrimSpace(format))
	if cleanFormat == "" {
		return "unknown_format"
	}
	return ""
}
