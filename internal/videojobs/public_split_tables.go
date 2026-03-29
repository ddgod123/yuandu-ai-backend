package videojobs

import (
	"strings"

	"gorm.io/gorm"
)

const (
	publicVideoImageJobsTable     = "public.video_image_jobs"
	publicVideoImageOutputsTable  = "public.video_image_outputs"
	publicVideoImagePackagesTable = "public.video_image_packages"
	publicVideoImageEventsTable   = "public.video_image_events"
	publicVideoImageFeedbackTable = "public.video_image_feedback"
)

var publicVideoImageSplitFormats = []string{
	"gif",
	"png",
	"jpg",
	"webp",
	"live",
	"mp4",
}

func PublicVideoImageBaseJobsTable() string {
	return publicVideoImageJobsTable
}

func PublicVideoImageBaseOutputsTable() string {
	return publicVideoImageOutputsTable
}

func PublicVideoImageBasePackagesTable() string {
	return publicVideoImagePackagesTable
}

func PublicVideoImageBaseEventsTable() string {
	return publicVideoImageEventsTable
}

func PublicVideoImageBaseFeedbackTable() string {
	return publicVideoImageFeedbackTable
}

func NormalizePublicVideoImageSplitFormat(raw string) string {
	return normalizePublicVideoImageSplitFormat(raw)
}

func ResolvePublicVideoImageJobsTable(format string) string {
	return resolvePublicVideoImageJobsTable(format)
}

func ResolvePublicVideoImageOutputsTable(format string) string {
	return resolvePublicVideoImageOutputsTable(format)
}

func ResolvePublicVideoImagePackagesTable(format string) string {
	return resolvePublicVideoImagePackagesTable(format)
}

func ResolvePublicVideoImageEventsTable(format string) string {
	return resolvePublicVideoImageEventsTable(format)
}

func ResolvePublicVideoImageFeedbackTable(format string) string {
	return resolvePublicVideoImageFeedbackTable(format)
}

func normalizePublicVideoImageSplitFormat(raw string) string {
	format := NormalizeRequestedFormat(strings.ToLower(strings.TrimSpace(raw)))
	switch format {
	case "gif", "png", "jpg", "webp", "live", "mp4":
		return format
	default:
		return ""
	}
}

func resolvePublicVideoImageJobsTable(format string) string {
	return resolvePublicVideoImageTableWithBase(publicVideoImageJobsTable, format)
}

func resolvePublicVideoImageOutputsTable(format string) string {
	return resolvePublicVideoImageTableWithBase(publicVideoImageOutputsTable, format)
}

func resolvePublicVideoImagePackagesTable(format string) string {
	return resolvePublicVideoImageTableWithBase(publicVideoImagePackagesTable, format)
}

func resolvePublicVideoImageEventsTable(format string) string {
	return resolvePublicVideoImageTableWithBase(publicVideoImageEventsTable, format)
}

func resolvePublicVideoImageFeedbackTable(format string) string {
	return resolvePublicVideoImageTableWithBase(publicVideoImageFeedbackTable, format)
}

func resolvePublicVideoImageWriteTables(baseTable string, format string) []string {
	baseTable = strings.TrimSpace(baseTable)
	if baseTable == "" {
		return nil
	}
	format = normalizePublicVideoImageSplitFormat(format)
	if format == "" {
		return []string{baseTable}
	}
	return []string{baseTable + "_" + format}
}

func resolvePublicVideoImageTableWithBase(baseTable string, format string) string {
	baseTable = strings.TrimSpace(baseTable)
	if baseTable == "" {
		return ""
	}
	format = normalizePublicVideoImageSplitFormat(format)
	if format == "" {
		return baseTable
	}
	return baseTable + "_" + format
}

func PublicVideoImageJobsMirrorTables() []string {
	return buildPublicVideoImageMirrorTables(publicVideoImageJobsTable)
}

func PublicVideoImageJobsSplitTables() []string {
	return buildPublicVideoImageSplitTables(publicVideoImageJobsTable)
}

func PublicVideoImageOutputsMirrorTables() []string {
	return buildPublicVideoImageMirrorTables(publicVideoImageOutputsTable)
}

func PublicVideoImageOutputsSplitTables() []string {
	return buildPublicVideoImageSplitTables(publicVideoImageOutputsTable)
}

func PublicVideoImagePackagesMirrorTables() []string {
	return buildPublicVideoImageMirrorTables(publicVideoImagePackagesTable)
}

func PublicVideoImagePackagesSplitTables() []string {
	return buildPublicVideoImageSplitTables(publicVideoImagePackagesTable)
}

func PublicVideoImageEventsMirrorTables() []string {
	return buildPublicVideoImageMirrorTables(publicVideoImageEventsTable)
}

func PublicVideoImageEventsSplitTables() []string {
	return buildPublicVideoImageSplitTables(publicVideoImageEventsTable)
}

func PublicVideoImageFeedbackMirrorTables() []string {
	return buildPublicVideoImageMirrorTables(publicVideoImageFeedbackTable)
}

func PublicVideoImageFeedbackSplitTables() []string {
	return buildPublicVideoImageSplitTables(publicVideoImageFeedbackTable)
}

func buildPublicVideoImageMirrorTables(baseTable string) []string {
	out := []string{baseTable}
	for _, format := range publicVideoImageSplitFormats {
		out = append(out, baseTable+"_"+format)
	}
	return out
}

func buildPublicVideoImageSplitTables(baseTable string) []string {
	out := make([]string, 0, len(publicVideoImageSplitFormats))
	for _, format := range publicVideoImageSplitFormats {
		out = append(out, baseTable+"_"+format)
	}
	return out
}

func forEachPublicVideoImageFeedbackTableByJob(db *gorm.DB, jobID uint64, fn func(tableName string) error) error {
	if fn == nil {
		return nil
	}
	requestedFormat := ""
	if db != nil && jobID > 0 {
		requestedFormat = resolvePublicVideoImageRequestedFormat(db, jobID)
	}
	for _, tableName := range resolvePublicVideoImageWriteTables(publicVideoImageFeedbackTable, requestedFormat) {
		tableName = strings.TrimSpace(tableName)
		if tableName == "" {
			continue
		}
		if tableName != publicVideoImageFeedbackTable && !publicVideoImageTableExists(db, tableName) {
			return fn(publicVideoImageFeedbackTable)
		}
		if err := fn(tableName); err != nil {
			if tableName != publicVideoImageFeedbackTable &&
				(isMissingTableError(err, tableName) || isMissingTableError(err, tableOnlyName(tableName))) {
				return fn(publicVideoImageFeedbackTable)
			}
			return err
		}
	}
	return nil
}
