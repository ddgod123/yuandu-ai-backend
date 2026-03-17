package handlers

import (
	"strconv"
	"strings"
)

var videoJobReviewStatusOrder = []string{
	"deliver",
	"keep_internal",
	"reject",
	"need_manual_review",
}

func normalizeVideoJobReviewStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "deliver":
		return "deliver"
	case "keep_internal":
		return "keep_internal"
	case "reject":
		return "reject"
	case "need_manual_review":
		return "need_manual_review"
	default:
		return ""
	}
}

func parseVideoJobReviewStatusFilter(raw string) []string {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		status := normalizeVideoJobReviewStatus(part)
		if status == "" {
			continue
		}
		if _, exists := seen[status]; exists {
			continue
		}
		seen[status] = struct{}{}
		out = append(out, status)
	}
	return out
}

func buildVideoJobReviewStatusSet(list []string) map[string]struct{} {
	if len(list) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(list))
	for _, item := range list {
		status := normalizeVideoJobReviewStatus(item)
		if status == "" {
			continue
		}
		out[status] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseBoolQueryWithDefault(raw string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		return parsed
	}
	return fallback
}
