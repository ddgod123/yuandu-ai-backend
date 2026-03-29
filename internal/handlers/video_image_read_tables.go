package handlers

import (
	"log"
	"os"
	"strings"
	"sync/atomic"

	"emoji/internal/videojobs"
)

type videoImageReadTables struct {
	Jobs     string
	Outputs  string
	Packages string
	Events   string
	Feedback string
}

var videoImageReadRouteDebugFlag atomic.Bool

func init() {
	videoImageReadRouteDebugFlag.Store(parseVideoImageReadRouteDebugFlag(os.Getenv("VIDEO_IMAGE_READ_ROUTE_DEBUG")))
}

func parseVideoImageReadRouteDebugFlag(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func isVideoImageReadRouteDebugEnabled() bool {
	return videoImageReadRouteDebugFlag.Load()
}

func setVideoImageReadRouteDebugEnabled(enabled bool) {
	videoImageReadRouteDebugFlag.Store(enabled)
}

func resolveVideoImageReadTables(format string) videoImageReadTables {
	normalized := videojobs.NormalizePublicVideoImageSplitFormat(format)
	tables := videoImageReadTables{}
	if normalized == "" {
		tables = videoImageReadTables{
			Jobs:     buildVideoImageReadUnionTableExpr(videojobs.PublicVideoImageJobsSplitTables()),
			Outputs:  buildVideoImageReadUnionTableExpr(videojobs.PublicVideoImageOutputsSplitTables()),
			Packages: buildVideoImageReadUnionTableExpr(videojobs.PublicVideoImagePackagesSplitTables()),
			Events:   buildVideoImageReadUnionTableExpr(videojobs.PublicVideoImageEventsSplitTables()),
			Feedback: buildVideoImageReadUnionTableExpr(videojobs.PublicVideoImageFeedbackSplitTables()),
		}
	} else {
		tables = videoImageReadTables{
			Jobs:     videojobs.ResolvePublicVideoImageJobsTable(normalized),
			Outputs:  videojobs.ResolvePublicVideoImageOutputsTable(normalized),
			Packages: videojobs.ResolvePublicVideoImagePackagesTable(normalized),
			Events:   videojobs.ResolvePublicVideoImageEventsTable(normalized),
			Feedback: videojobs.ResolvePublicVideoImageFeedbackTable(normalized),
		}
	}
	if isVideoImageReadRouteDebugEnabled() {
		log.Printf(
			"[video-image-read-route] format=%q normalized=%q jobs=%s outputs=%s packages=%s events=%s feedback=%s",
			strings.TrimSpace(format),
			normalized,
			tables.Jobs,
			tables.Outputs,
			tables.Packages,
			tables.Events,
			tables.Feedback,
		)
	}
	return tables
}

func resolveVideoImageReadTablesByFilter(filter *videoImageFeedbackFilter) videoImageReadTables {
	if filter == nil {
		return resolveVideoImageReadTables("")
	}
	return resolveVideoImageReadTables(filter.Format)
}

func normalizeVideoImageFormatFilter(raw string) string {
	value := normalizeVideoJobFormat(raw)
	if value == "all" {
		return ""
	}
	return strings.TrimSpace(value)
}

func buildVideoImageReadUnionTableExpr(tables []string) string {
	cleaned := make([]string, 0, len(tables))
	seen := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		table = strings.TrimSpace(table)
		if table == "" {
			continue
		}
		if _, ok := seen[table]; ok {
			continue
		}
		seen[table] = struct{}{}
		cleaned = append(cleaned, table)
	}
	if len(cleaned) == 0 {
		return ""
	}
	if len(cleaned) == 1 {
		return cleaned[0]
	}
	parts := make([]string, 0, len(cleaned))
	for _, table := range cleaned {
		parts = append(parts, "SELECT * FROM "+table)
	}
	return "(" + strings.Join(parts, " UNION ALL ") + ")"
}
