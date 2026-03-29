package videojobs

import (
	"errors"
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PublicVideoImageSplitBackfillOptions struct {
	Apply          bool
	BatchSize      int
	FormatFilter   string
	FallbackFormat string

	StartJobID      uint64
	StartOutputID   uint64
	StartPackageID  uint64
	StartEventID    uint64
	StartFeedbackID uint64

	LimitJobs      int
	LimitOutputs   int
	LimitPackages  int
	LimitEvents    int
	LimitFeedbacks int

	IncludeJobs      bool
	IncludeOutputs   bool
	IncludePackages  bool
	IncludeEvents    bool
	IncludeFeedbacks bool

	OnProgress func(table string, stats PublicVideoImageSplitBackfillTableReport)
	StopSignal <-chan struct{}
	ShouldStop func() bool
}

type PublicVideoImageSplitBackfillTableReport struct {
	Scanned         int    `json:"scanned"`
	WouldWrite      int    `json:"would_write"`
	Written         int    `json:"written"`
	SkippedByFormat int    `json:"skipped_by_format"`
	FallbackUsed    int    `json:"fallback_used"`
	Failed          int    `json:"failed"`
	LastID          uint64 `json:"last_id"`
}

type PublicVideoImageSplitBackfillReport struct {
	Apply          bool      `json:"apply"`
	BatchSize      int       `json:"batch_size"`
	FormatFilter   string    `json:"format_filter,omitempty"`
	FallbackFormat string    `json:"fallback_format"`
	Stopped        bool      `json:"stopped"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at"`

	Jobs      PublicVideoImageSplitBackfillTableReport `json:"jobs"`
	Outputs   PublicVideoImageSplitBackfillTableReport `json:"outputs"`
	Packages  PublicVideoImageSplitBackfillTableReport `json:"packages"`
	Events    PublicVideoImageSplitBackfillTableReport `json:"events"`
	Feedbacks PublicVideoImageSplitBackfillTableReport `json:"feedbacks"`
}

func (r PublicVideoImageSplitBackfillReport) FailedTotal() int {
	return r.Jobs.Failed + r.Outputs.Failed + r.Packages.Failed + r.Events.Failed + r.Feedbacks.Failed
}

var errPublicVideoImageBackfillStopped = errors.New("public video image split backfill stopped")

func BackfillPublicVideoImageSplitTables(db *gorm.DB, opt PublicVideoImageSplitBackfillOptions) (PublicVideoImageSplitBackfillReport, error) {
	if db == nil {
		return PublicVideoImageSplitBackfillReport{}, nil
	}
	opt = normalizePublicVideoImageSplitBackfillOptions(opt)
	report := PublicVideoImageSplitBackfillReport{
		Apply:          opt.Apply,
		BatchSize:      opt.BatchSize,
		FormatFilter:   opt.FormatFilter,
		FallbackFormat: opt.FallbackFormat,
		StartedAt:      time.Now(),
	}
	jobFormatCache := map[uint64]string{}

	if opt.IncludeJobs {
		stats, err := backfillPublicVideoImageJobsSplitTable(db, opt, jobFormatCache)
		report.Jobs = stats
		emitPublicVideoImageSplitBackfillProgress(opt, "jobs", stats)
		if err != nil {
			report.FinishedAt = time.Now()
			if errors.Is(err, errPublicVideoImageBackfillStopped) {
				report.Stopped = true
				return report, nil
			}
			return report, err
		}
	}
	if opt.IncludeOutputs {
		stats, err := backfillPublicVideoImageOutputsSplitTable(db, opt, jobFormatCache)
		report.Outputs = stats
		emitPublicVideoImageSplitBackfillProgress(opt, "outputs", stats)
		if err != nil {
			report.FinishedAt = time.Now()
			if errors.Is(err, errPublicVideoImageBackfillStopped) {
				report.Stopped = true
				return report, nil
			}
			return report, err
		}
	}
	if opt.IncludePackages {
		stats, err := backfillPublicVideoImagePackagesSplitTable(db, opt, jobFormatCache)
		report.Packages = stats
		emitPublicVideoImageSplitBackfillProgress(opt, "packages", stats)
		if err != nil {
			report.FinishedAt = time.Now()
			if errors.Is(err, errPublicVideoImageBackfillStopped) {
				report.Stopped = true
				return report, nil
			}
			return report, err
		}
	}
	if opt.IncludeEvents {
		stats, err := backfillPublicVideoImageEventsSplitTable(db, opt, jobFormatCache)
		report.Events = stats
		emitPublicVideoImageSplitBackfillProgress(opt, "events", stats)
		if err != nil {
			report.FinishedAt = time.Now()
			if errors.Is(err, errPublicVideoImageBackfillStopped) {
				report.Stopped = true
				return report, nil
			}
			return report, err
		}
	}
	if opt.IncludeFeedbacks {
		stats, err := backfillPublicVideoImageFeedbackSplitTable(db, opt, jobFormatCache)
		report.Feedbacks = stats
		emitPublicVideoImageSplitBackfillProgress(opt, "feedbacks", stats)
		if err != nil {
			report.FinishedAt = time.Now()
			if errors.Is(err, errPublicVideoImageBackfillStopped) {
				report.Stopped = true
				return report, nil
			}
			return report, err
		}
	}

	report.FinishedAt = time.Now()
	return report, nil
}

func normalizePublicVideoImageSplitBackfillOptions(opt PublicVideoImageSplitBackfillOptions) PublicVideoImageSplitBackfillOptions {
	if opt.BatchSize <= 0 || opt.BatchSize > 5000 {
		opt.BatchSize = 500
	}
	opt.FormatFilter = normalizePublicVideoImageSplitFormat(opt.FormatFilter)
	opt.FallbackFormat = normalizePublicVideoImageSplitFormat(opt.FallbackFormat)
	if opt.FallbackFormat == "" {
		opt.FallbackFormat = "gif"
	}
	if !opt.IncludeJobs && !opt.IncludeOutputs && !opt.IncludePackages && !opt.IncludeEvents && !opt.IncludeFeedbacks {
		opt.IncludeJobs = true
		opt.IncludeOutputs = true
		opt.IncludePackages = true
		opt.IncludeEvents = true
		opt.IncludeFeedbacks = true
	}
	return opt
}

func backfillPublicVideoImageJobsSplitTable(
	db *gorm.DB,
	opt PublicVideoImageSplitBackfillOptions,
	jobFormatCache map[uint64]string,
) (PublicVideoImageSplitBackfillTableReport, error) {
	stats := PublicVideoImageSplitBackfillTableReport{LastID: opt.StartJobID}
	for {
		if publicVideoImageSplitBackfillShouldStop(opt) {
			emitPublicVideoImageSplitBackfillProgress(opt, "jobs", stats)
			return stats, errPublicVideoImageBackfillStopped
		}
		if opt.LimitJobs > 0 && stats.Scanned >= opt.LimitJobs {
			break
		}
		var rows []models.VideoImageJobPublic
		if err := db.Table(publicVideoImageJobsTable).
			Where("id > ?", stats.LastID).
			Order("id ASC").
			Limit(opt.BatchSize).
			Find(&rows).Error; err != nil {
			return stats, err
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			if publicVideoImageSplitBackfillShouldStop(opt) {
				emitPublicVideoImageSplitBackfillProgress(opt, "jobs", stats)
				return stats, errPublicVideoImageBackfillStopped
			}
			stats.LastID = row.ID
			if opt.LimitJobs > 0 && stats.Scanned >= opt.LimitJobs {
				break
			}
			stats.Scanned++

			targetFormat, fallbackUsed := resolvePublicVideoImageBackfillRouteFormat(
				db,
				row.ID,
				[]string{row.RequestedFormat},
				opt.FallbackFormat,
				jobFormatCache,
			)
			if targetFormat == "" {
				stats.Failed++
				continue
			}
			if opt.FormatFilter != "" && targetFormat != opt.FormatFilter {
				stats.SkippedByFormat++
				continue
			}
			if fallbackUsed {
				stats.FallbackUsed++
			}
			if !opt.Apply {
				stats.WouldWrite++
				continue
			}

			copyRow := row
			copyRow.RequestedFormat = targetFormat
			if err := upsertPublicVideoImageJobRow(db, resolvePublicVideoImageJobsTable(targetFormat), copyRow); err != nil {
				stats.Failed++
				continue
			}
			jobFormatCache[row.ID] = targetFormat
			stats.Written++
		}
		emitPublicVideoImageSplitBackfillProgress(opt, "jobs", stats)
	}
	return stats, nil
}

func backfillPublicVideoImageOutputsSplitTable(
	db *gorm.DB,
	opt PublicVideoImageSplitBackfillOptions,
	jobFormatCache map[uint64]string,
) (PublicVideoImageSplitBackfillTableReport, error) {
	stats := PublicVideoImageSplitBackfillTableReport{LastID: opt.StartOutputID}
	for {
		if publicVideoImageSplitBackfillShouldStop(opt) {
			emitPublicVideoImageSplitBackfillProgress(opt, "outputs", stats)
			return stats, errPublicVideoImageBackfillStopped
		}
		if opt.LimitOutputs > 0 && stats.Scanned >= opt.LimitOutputs {
			break
		}
		var rows []models.VideoImageOutputPublic
		if err := db.Table(publicVideoImageOutputsTable).
			Where("id > ?", stats.LastID).
			Order("id ASC").
			Limit(opt.BatchSize).
			Find(&rows).Error; err != nil {
			return stats, err
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			if publicVideoImageSplitBackfillShouldStop(opt) {
				emitPublicVideoImageSplitBackfillProgress(opt, "outputs", stats)
				return stats, errPublicVideoImageBackfillStopped
			}
			stats.LastID = row.ID
			if opt.LimitOutputs > 0 && stats.Scanned >= opt.LimitOutputs {
				break
			}
			stats.Scanned++

			targetFormat, fallbackUsed := resolvePublicVideoImageBackfillRouteFormat(
				db,
				row.JobID,
				[]string{row.Format},
				opt.FallbackFormat,
				jobFormatCache,
			)
			if targetFormat == "" {
				stats.Failed++
				continue
			}
			if opt.FormatFilter != "" && targetFormat != opt.FormatFilter {
				stats.SkippedByFormat++
				continue
			}
			if fallbackUsed {
				stats.FallbackUsed++
			}
			if !opt.Apply {
				stats.WouldWrite++
				continue
			}
			if err := upsertPublicVideoImageOutputRow(db, resolvePublicVideoImageOutputsTable(targetFormat), row); err != nil {
				stats.Failed++
				continue
			}
			stats.Written++
		}
		emitPublicVideoImageSplitBackfillProgress(opt, "outputs", stats)
	}
	return stats, nil
}

func backfillPublicVideoImagePackagesSplitTable(
	db *gorm.DB,
	opt PublicVideoImageSplitBackfillOptions,
	jobFormatCache map[uint64]string,
) (PublicVideoImageSplitBackfillTableReport, error) {
	stats := PublicVideoImageSplitBackfillTableReport{LastID: opt.StartPackageID}
	for {
		if publicVideoImageSplitBackfillShouldStop(opt) {
			emitPublicVideoImageSplitBackfillProgress(opt, "packages", stats)
			return stats, errPublicVideoImageBackfillStopped
		}
		if opt.LimitPackages > 0 && stats.Scanned >= opt.LimitPackages {
			break
		}
		var rows []models.VideoImagePackagePublic
		if err := db.Table(publicVideoImagePackagesTable).
			Where("id > ?", stats.LastID).
			Order("id ASC").
			Limit(opt.BatchSize).
			Find(&rows).Error; err != nil {
			return stats, err
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			if publicVideoImageSplitBackfillShouldStop(opt) {
				emitPublicVideoImageSplitBackfillProgress(opt, "packages", stats)
				return stats, errPublicVideoImageBackfillStopped
			}
			stats.LastID = row.ID
			if opt.LimitPackages > 0 && stats.Scanned >= opt.LimitPackages {
				break
			}
			stats.Scanned++

			targetFormat, fallbackUsed := resolvePublicVideoImageBackfillRouteFormat(
				db,
				row.JobID,
				nil,
				opt.FallbackFormat,
				jobFormatCache,
			)
			if targetFormat == "" {
				stats.Failed++
				continue
			}
			if opt.FormatFilter != "" && targetFormat != opt.FormatFilter {
				stats.SkippedByFormat++
				continue
			}
			if fallbackUsed {
				stats.FallbackUsed++
			}
			if !opt.Apply {
				stats.WouldWrite++
				continue
			}
			if err := upsertPublicVideoImagePackageBackfillRow(db, resolvePublicVideoImagePackagesTable(targetFormat), row); err != nil {
				stats.Failed++
				continue
			}
			stats.Written++
		}
		emitPublicVideoImageSplitBackfillProgress(opt, "packages", stats)
	}
	return stats, nil
}

func backfillPublicVideoImageEventsSplitTable(
	db *gorm.DB,
	opt PublicVideoImageSplitBackfillOptions,
	jobFormatCache map[uint64]string,
) (PublicVideoImageSplitBackfillTableReport, error) {
	stats := PublicVideoImageSplitBackfillTableReport{LastID: opt.StartEventID}
	for {
		if publicVideoImageSplitBackfillShouldStop(opt) {
			emitPublicVideoImageSplitBackfillProgress(opt, "events", stats)
			return stats, errPublicVideoImageBackfillStopped
		}
		if opt.LimitEvents > 0 && stats.Scanned >= opt.LimitEvents {
			break
		}
		var rows []models.VideoImageEventPublic
		if err := db.Table(publicVideoImageEventsTable).
			Where("id > ?", stats.LastID).
			Order("id ASC").
			Limit(opt.BatchSize).
			Find(&rows).Error; err != nil {
			return stats, err
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			if publicVideoImageSplitBackfillShouldStop(opt) {
				emitPublicVideoImageSplitBackfillProgress(opt, "events", stats)
				return stats, errPublicVideoImageBackfillStopped
			}
			stats.LastID = row.ID
			if opt.LimitEvents > 0 && stats.Scanned >= opt.LimitEvents {
				break
			}
			stats.Scanned++

			targetFormat, fallbackUsed := resolvePublicVideoImageBackfillRouteFormat(
				db,
				row.JobID,
				nil,
				opt.FallbackFormat,
				jobFormatCache,
			)
			if targetFormat == "" {
				stats.Failed++
				continue
			}
			if opt.FormatFilter != "" && targetFormat != opt.FormatFilter {
				stats.SkippedByFormat++
				continue
			}
			if fallbackUsed {
				stats.FallbackUsed++
			}
			if !opt.Apply {
				stats.WouldWrite++
				continue
			}
			if err := upsertPublicVideoImageEventBackfillRow(db, resolvePublicVideoImageEventsTable(targetFormat), row); err != nil {
				stats.Failed++
				continue
			}
			stats.Written++
		}
		emitPublicVideoImageSplitBackfillProgress(opt, "events", stats)
	}
	return stats, nil
}

func backfillPublicVideoImageFeedbackSplitTable(
	db *gorm.DB,
	opt PublicVideoImageSplitBackfillOptions,
	jobFormatCache map[uint64]string,
) (PublicVideoImageSplitBackfillTableReport, error) {
	stats := PublicVideoImageSplitBackfillTableReport{LastID: opt.StartFeedbackID}
	for {
		if publicVideoImageSplitBackfillShouldStop(opt) {
			emitPublicVideoImageSplitBackfillProgress(opt, "feedbacks", stats)
			return stats, errPublicVideoImageBackfillStopped
		}
		if opt.LimitFeedbacks > 0 && stats.Scanned >= opt.LimitFeedbacks {
			break
		}
		var rows []models.VideoImageFeedbackPublic
		if err := db.Table(publicVideoImageFeedbackTable).
			Where("id > ?", stats.LastID).
			Order("id ASC").
			Limit(opt.BatchSize).
			Find(&rows).Error; err != nil {
			return stats, err
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			if publicVideoImageSplitBackfillShouldStop(opt) {
				emitPublicVideoImageSplitBackfillProgress(opt, "feedbacks", stats)
				return stats, errPublicVideoImageBackfillStopped
			}
			stats.LastID = row.ID
			if opt.LimitFeedbacks > 0 && stats.Scanned >= opt.LimitFeedbacks {
				break
			}
			stats.Scanned++

			targetFormat, fallbackUsed := resolvePublicVideoImageBackfillRouteFormat(
				db,
				row.JobID,
				nil,
				opt.FallbackFormat,
				jobFormatCache,
			)
			if targetFormat == "" {
				stats.Failed++
				continue
			}
			if opt.FormatFilter != "" && targetFormat != opt.FormatFilter {
				stats.SkippedByFormat++
				continue
			}
			if fallbackUsed {
				stats.FallbackUsed++
			}
			if !opt.Apply {
				stats.WouldWrite++
				continue
			}
			if err := upsertPublicVideoImageFeedbackBackfillRow(db, resolvePublicVideoImageFeedbackTable(targetFormat), row); err != nil {
				stats.Failed++
				continue
			}
			stats.Written++
		}
		emitPublicVideoImageSplitBackfillProgress(opt, "feedbacks", stats)
	}
	return stats, nil
}

func publicVideoImageSplitBackfillShouldStop(opt PublicVideoImageSplitBackfillOptions) bool {
	if opt.ShouldStop != nil && opt.ShouldStop() {
		return true
	}
	if opt.StopSignal == nil {
		return false
	}
	select {
	case <-opt.StopSignal:
		return true
	default:
		return false
	}
}

func emitPublicVideoImageSplitBackfillProgress(
	opt PublicVideoImageSplitBackfillOptions,
	table string,
	stats PublicVideoImageSplitBackfillTableReport,
) {
	if opt.OnProgress == nil {
		return
	}
	opt.OnProgress(strings.TrimSpace(strings.ToLower(table)), stats)
}

func resolvePublicVideoImageBackfillRouteFormat(
	db *gorm.DB,
	jobID uint64,
	hints []string,
	fallbackFormat string,
	cache map[uint64]string,
) (string, bool) {
	if jobID > 0 && cache != nil {
		if format := normalizePublicVideoImageSplitFormat(cache[jobID]); format != "" {
			return format, false
		}
	}
	if jobID > 0 {
		if format := normalizePublicVideoImageSplitFormat(resolvePublicVideoImageRequestedFormat(db, jobID)); format != "" {
			if cache != nil {
				cache[jobID] = format
			}
			return format, false
		}
	}
	for _, hint := range hints {
		if format := normalizePublicVideoImageSplitFormat(hint); format != "" {
			if jobID > 0 && cache != nil {
				cache[jobID] = format
			}
			return format, false
		}
	}
	if fallback := normalizePublicVideoImageSplitFormat(strings.TrimSpace(fallbackFormat)); fallback != "" {
		if jobID > 0 && cache != nil {
			cache[jobID] = fallback
		}
		return fallback, true
	}
	return "", false
}

func upsertPublicVideoImagePackageBackfillRow(tx *gorm.DB, tableName string, row models.VideoImagePackagePublic) error {
	if tx == nil {
		return nil
	}
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return nil
	}
	return tx.Table(tableName).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "job_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"user_id":        row.UserID,
			"zip_object_key": row.ZipObjectKey,
			"zip_name":       row.ZipName,
			"zip_size_bytes": row.ZipSizeBytes,
			"file_count":     row.FileCount,
			"manifest":       row.Manifest,
			"expires_at":     row.ExpiresAt,
			"created_at":     row.CreatedAt,
		}),
	}).Create(&row).Error
}

func upsertPublicVideoImageEventBackfillRow(tx *gorm.DB, tableName string, row models.VideoImageEventPublic) error {
	if tx == nil {
		return nil
	}
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return nil
	}
	return tx.Table(tableName).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"job_id":     row.JobID,
			"level":      row.Level,
			"stage":      row.Stage,
			"message":    row.Message,
			"metadata":   row.Metadata,
			"created_at": row.CreatedAt,
		}),
	}).Create(&row).Error
}

func upsertPublicVideoImageFeedbackBackfillRow(tx *gorm.DB, tableName string, row models.VideoImageFeedbackPublic) error {
	if tx == nil {
		return nil
	}
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return nil
	}
	return tx.Table(tableName).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"job_id":     row.JobID,
			"output_id":  row.OutputID,
			"user_id":    row.UserID,
			"action":     row.Action,
			"weight":     row.Weight,
			"scene_tag":  row.SceneTag,
			"metadata":   row.Metadata,
			"created_at": row.CreatedAt,
		}),
	}).Create(&row).Error
}
