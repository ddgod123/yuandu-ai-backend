package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm/clause"
)

type myWorkJobRow struct {
	ID              uint64         `gorm:"column:id"`
	Title           string         `gorm:"column:title"`
	RequestedFormat string         `gorm:"column:requested_format"`
	Status          string         `gorm:"column:status"`
	Stage           string         `gorm:"column:stage"`
	Progress        int            `gorm:"column:progress"`
	Options         datatypes.JSON `gorm:"column:options"`
	Metrics         datatypes.JSON `gorm:"column:metrics"`
	StartedAt       *time.Time     `gorm:"column:started_at"`
	FinishedAt      *time.Time     `gorm:"column:finished_at"`
	CreatedAt       time.Time      `gorm:"column:created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at"`
}

type myWorkOutputRow struct {
	ID                     uint64    `gorm:"column:id"`
	JobID                  uint64    `gorm:"column:job_id"`
	Format                 string    `gorm:"column:format"`
	FileRole               string    `gorm:"column:file_role"`
	ObjectKey              string    `gorm:"column:object_key"`
	Score                  float64   `gorm:"column:score"`
	GIFLoopTuneLoopClosure float64   `gorm:"column:gif_loop_tune_loop_closure"`
	CreatedAt              time.Time `gorm:"column:created_at"`
}

type myWorkPackageRow struct {
	JobID        uint64 `gorm:"column:job_id"`
	ZipObjectKey string `gorm:"column:zip_object_key"`
	ZipName      string `gorm:"column:zip_name"`
	ZipSizeBytes int64  `gorm:"column:zip_size_bytes"`
}

type myWorkSummaryAccumulator struct {
	Summary               VideoJobResultSummary
	PreviewSet            map[string]struct{}
	FormatCount           map[string]int
	QualityScoreSum       float64
	QualityLoopClosureSum float64
	QualityCount          int
}

// ListMyWorks godoc
// @Summary List current user works cards
// @Tags user
// @Produce json
// @Param format query string false "format: all|gif|png|jpg|webp|live|mp4"
// @Param status query string false "status filter, default done"
// @Param limit query int false "limit (default 80, max 200)"
// @Success 200 {object} map[string]interface{}
// @Router /api/my/works [get]
func (h *Handler) ListMyWorks(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "80")))
	if limit <= 0 {
		limit = 80
	}
	if limit > 200 {
		limit = 200
	}

	statusFilter := strings.ToLower(strings.TrimSpace(c.DefaultQuery("status", models.VideoJobStatusDone)))
	if statusFilter == "all" {
		statusFilter = ""
	}

	formatFilter := normalizeVideoImageFormatFilter(c.Query("format"))
	tables := resolveVideoImageReadTables(formatFilter)

	jobs, resolvedTables, err := h.queryMyWorkJobsRows(userID, statusFilter, formatFilter, limit, tables)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(jobs) == 0 {
		c.JSON(http.StatusOK, gin.H{"items": []VideoJobResponse{}})
		return
	}

	jobIDs := make([]uint64, 0, len(jobs))
	for _, job := range jobs {
		jobIDs = append(jobIDs, job.ID)
	}

	summaryByJobID, err := h.buildMyWorkResultSummary(jobIDs, resolvedTables)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]VideoJobResponse, 0, len(jobs))
	for _, job := range jobs {
		summary := summaryByJobID[job.ID]
		if summary.CollectionID == 0 {
			summary.CollectionID = job.ID
		}
		if summary.CollectionTitle == "" {
			summary.CollectionTitle = strings.TrimSpace(job.Title)
		}

		options := parseJSONMap(job.Options)
		metrics := parseJSONMap(job.Metrics)
		summary.PackageStatus = resolveMyWorkPackageStatus(job.Status, metrics, summary.PackageStatus)

		resultCollectionID := summary.CollectionID
		requestedFormat := normalizeMyWorkFormat(job.RequestedFormat)
		outputFormats := []string{}
		if requestedFormat != "" {
			outputFormats = append(outputFormats, requestedFormat)
		}

		filled := summary
		items = append(items, VideoJobResponse{
			ID:                 job.ID,
			Title:              strings.TrimSpace(job.Title),
			OutputFormats:      outputFormats,
			Status:             strings.TrimSpace(job.Status),
			Stage:              strings.TrimSpace(job.Stage),
			Progress:           job.Progress,
			ResultCollectionID: &resultCollectionID,
			Options:            options,
			Metrics:            metrics,
			QueuedAt:           job.CreatedAt,
			StartedAt:          job.StartedAt,
			FinishedAt:         job.FinishedAt,
			CreatedAt:          job.CreatedAt,
			UpdatedAt:          job.UpdatedAt,
			ResultSummary:      &filled,
		})
	}

	h.upsertVideoWorkCardsProjection(userID, items)
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) queryMyWorkJobsRows(
	userID uint64,
	statusFilter string,
	formatFilter string,
	limit int,
	tables videoImageReadTables,
) ([]myWorkJobRow, videoImageReadTables, error) {
	activeTables := tables
	baseTables := resolveVideoImageReadTables("")

	for i := 0; i < 2; i++ {
		var rows []myWorkJobRow
		query := h.db.Table(activeTables.Jobs).
			Select("id", "title", "requested_format", "status", "stage", "progress", "options", "metrics", "started_at", "finished_at", "created_at", "updated_at").
			Where("user_id = ?", userID)

		if statusFilter != "" {
			query = query.Where("LOWER(COALESCE(status, '')) = ?", statusFilter)
		}
		if formatFilter != "" && activeTables.Jobs == baseTables.Jobs {
			query = query.Where("LOWER(COALESCE(requested_format, '')) = ?", formatFilter)
		}

		err := query.Order("updated_at DESC, id DESC").Limit(limit).Find(&rows).Error
		if err == nil {
			return rows, activeTables, nil
		}
		if activeTables.Jobs != baseTables.Jobs && isMissingTableError(err, activeTables.Jobs) {
			activeTables = baseTables
			continue
		}
		return nil, activeTables, err
	}
	return []myWorkJobRow{}, activeTables, nil
}

func (h *Handler) buildMyWorkResultSummary(
	jobIDs []uint64,
	tables videoImageReadTables,
) (map[uint64]VideoJobResultSummary, error) {
	acc := make(map[uint64]*myWorkSummaryAccumulator, len(jobIDs))
	for _, jobID := range jobIDs {
		acc[jobID] = &myWorkSummaryAccumulator{
			Summary: VideoJobResultSummary{
				CollectionID:  jobID,
				PreviewImages: make([]string, 0, 15),
				PackageStatus: "processing",
			},
			PreviewSet:  map[string]struct{}{},
			FormatCount: map[string]int{},
		}
	}

	baseTables := resolveVideoImageReadTables("")
	outputTable := tables.Outputs
	var outputs []myWorkOutputRow
	for i := 0; i < 2; i++ {
		err := h.db.Table(outputTable).
			Select("id", "job_id", "format", "file_role", "object_key", "score", "gif_loop_tune_loop_closure", "created_at").
			Where("job_id IN ?", jobIDs).
			Where("file_role IN ?", []string{"main", "cover"}).
			Order("job_id ASC, created_at ASC, id ASC").
			Find(&outputs).Error
		if err == nil {
			break
		}
		if outputTable != baseTables.Outputs && isMissingTableError(err, outputTable) {
			outputTable = baseTables.Outputs
			continue
		}
		return nil, err
	}

	for _, row := range outputs {
		entry, ok := acc[row.JobID]
		if !ok || entry == nil {
			continue
		}
		fileRole := strings.ToLower(strings.TrimSpace(row.FileRole))
		if fileRole == "" {
			fileRole = "main"
		}

		format := normalizeMyWorkFormat(row.Format)
		if format == "" {
			format = normalizeMyWorkFormat(inferMyWorkFormatFromObjectKey(row.ObjectKey))
		}

		if fileRole == "main" {
			entry.Summary.FileCount++
			if format != "" {
				entry.FormatCount[format]++
			}
		}

		if len(entry.Summary.PreviewImages) < 15 && (fileRole == "main" || fileRole == "cover") {
			if isMyWorkPreviewFormat(format, row.ObjectKey) {
				previewURL := strings.TrimSpace(resolvePreviewURL(strings.TrimSpace(row.ObjectKey), h.qiniu))
				if previewURL != "" {
					if _, exists := entry.PreviewSet[previewURL]; !exists {
						entry.PreviewSet[previewURL] = struct{}{}
						entry.Summary.PreviewImages = append(entry.Summary.PreviewImages, previewURL)
					}
				}
			}
		}

		if fileRole == "main" && format == "gif" {
			entry.QualityCount++
			entry.QualityScoreSum += row.Score
			entry.QualityLoopClosureSum += row.GIFLoopTuneLoopClosure
			if entry.QualityCount == 1 || row.Score > entry.Summary.QualityTopScore {
				entry.Summary.QualityTopScore = row.Score
			}
		}
	}

	packageTable := tables.Packages
	var packages []myWorkPackageRow
	for i := 0; i < 2; i++ {
		err := h.db.Table(packageTable).
			Select("job_id", "zip_object_key", "zip_name", "zip_size_bytes").
			Where("job_id IN ?", jobIDs).
			Find(&packages).Error
		if err == nil {
			break
		}
		if packageTable != baseTables.Packages && isMissingTableError(err, packageTable) {
			packageTable = baseTables.Packages
			continue
		}
		return nil, err
	}
	for _, row := range packages {
		entry, ok := acc[row.JobID]
		if !ok || entry == nil {
			continue
		}
		if strings.TrimSpace(row.ZipObjectKey) == "" {
			continue
		}
		entry.Summary.PackageStatus = "ready"
	}

	out := make(map[uint64]VideoJobResultSummary, len(acc))
	for jobID, item := range acc {
		formats := make([]string, 0, len(item.FormatCount))
		for format := range item.FormatCount {
			formats = append(formats, format)
		}
		sort.Strings(formats)
		if len(formats) > 0 {
			item.Summary.FormatSummary = make([]string, 0, len(formats))
			for _, format := range formats {
				item.Summary.FormatSummary = append(
					item.Summary.FormatSummary,
					strings.ToUpper(format)+" × "+strconv.Itoa(item.FormatCount[format]),
				)
			}
		}
		if item.QualityCount > 0 {
			item.Summary.QualitySampleCount = item.QualityCount
			item.Summary.QualityAvgScore = item.QualityScoreSum / float64(item.QualityCount)
			item.Summary.QualityAvgLoopClosure = item.QualityLoopClosureSum / float64(item.QualityCount)
		}
		out[jobID] = item.Summary
	}
	return out, nil
}

func resolveMyWorkPackageStatus(jobStatus string, metrics map[string]interface{}, fallback string) string {
	current := strings.ToLower(strings.TrimSpace(fallback))
	rawStatus := strings.ToLower(strings.TrimSpace(stringFromAny(metrics["package_zip_status"])))
	switch rawStatus {
	case "ready":
		return "ready"
	case "pending", "processing":
		return "processing"
	case "failed":
		return "failed"
	}
	if current == "ready" || current == "failed" || current == "processing" {
		return current
	}
	if strings.EqualFold(strings.TrimSpace(jobStatus), models.VideoJobStatusDone) {
		return "failed"
	}
	return "processing"
}

func normalizeMyWorkFormat(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "jpeg" {
		return "jpg"
	}
	return value
}

func inferMyWorkFormatFromObjectKey(raw string) string {
	key := strings.TrimSpace(raw)
	if key == "" {
		return ""
	}
	clean := strings.Split(key, "?")[0]
	clean = strings.Split(clean, "#")[0]
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(clean), "."))
	return normalizeMyWorkFormat(ext)
}

func isMyWorkPreviewFormat(format string, objectKey string) bool {
	switch normalizeMyWorkFormat(format) {
	case "gif", "png", "jpg", "webp":
		return true
	default:
		ext := inferMyWorkFormatFromObjectKey(objectKey)
		return ext == "gif" || ext == "png" || ext == "jpg" || ext == "webp"
	}
}

func upsertWorkCardJSONOrDefault(value interface{}, fallback string) datatypes.JSON {
	if fallback == "" {
		fallback = "{}"
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) == 0 {
		return datatypes.JSON([]byte(fallback))
	}
	return datatypes.JSON(raw)
}

func (h *Handler) upsertVideoWorkCardsProjection(userID uint64, items []VideoJobResponse) {
	if h == nil || h.db == nil || userID == 0 || len(items) == 0 {
		return
	}

	rows := make([]models.VideoWorkCardPublic, 0, len(items))
	for _, item := range items {
		if item.ID == 0 || item.ResultSummary == nil {
			continue
		}
		format := ""
		if len(item.OutputFormats) > 0 {
			format = normalizeMyWorkFormat(item.OutputFormats[0])
		}
		if format == "" {
			format = normalizeMyWorkFormat(stringFromAny(item.Options["requested_format"]))
		}
		summary := item.ResultSummary
		row := models.VideoWorkCardPublic{
			JobID:                 item.ID,
			UserID:                userID,
			RequestedFormat:       format,
			Title:                 strings.TrimSpace(item.Title),
			Status:                strings.TrimSpace(item.Status),
			Stage:                 strings.TrimSpace(item.Stage),
			Progress:              item.Progress,
			ResultCollectionID:    item.ResultCollectionID,
			FileCount:             summary.FileCount,
			PreviewImages:         upsertWorkCardJSONOrDefault(summary.PreviewImages, "[]"),
			FormatSummary:         upsertWorkCardJSONOrDefault(summary.FormatSummary, "[]"),
			PackageStatus:         strings.TrimSpace(summary.PackageStatus),
			QualitySampleCount:    summary.QualitySampleCount,
			QualityTopScore:       summary.QualityTopScore,
			QualityAvgScore:       summary.QualityAvgScore,
			QualityAvgLoopClosure: summary.QualityAvgLoopClosure,
			Options:               upsertWorkCardJSONOrDefault(item.Options, "{}"),
			Metrics:               upsertWorkCardJSONOrDefault(item.Metrics, "{}"),
			SourceUpdatedAt:       &item.UpdatedAt,
			CreatedAt:             item.CreatedAt,
			UpdatedAt:             item.UpdatedAt,
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return
	}

	err := h.db.Table("public.video_work_cards").
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "job_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"user_id":                  gormExprExcluded("user_id"),
				"requested_format":         gormExprExcluded("requested_format"),
				"title":                    gormExprExcluded("title"),
				"status":                   gormExprExcluded("status"),
				"stage":                    gormExprExcluded("stage"),
				"progress":                 gormExprExcluded("progress"),
				"result_collection_id":     gormExprExcluded("result_collection_id"),
				"file_count":               gormExprExcluded("file_count"),
				"preview_images":           gormExprExcluded("preview_images"),
				"format_summary":           gormExprExcluded("format_summary"),
				"package_status":           gormExprExcluded("package_status"),
				"quality_sample_count":     gormExprExcluded("quality_sample_count"),
				"quality_top_score":        gormExprExcluded("quality_top_score"),
				"quality_avg_score":        gormExprExcluded("quality_avg_score"),
				"quality_avg_loop_closure": gormExprExcluded("quality_avg_loop_closure"),
				"options":                  gormExprExcluded("options"),
				"metrics":                  gormExprExcluded("metrics"),
				"source_updated_at":        gormExprExcluded("source_updated_at"),
				"updated_at":               gormExprExcluded("updated_at"),
			}),
		}).
		Create(&rows).Error
	if err != nil && !isMissingTableError(err, "video_work_cards") {
		return
	}
}

func gormExprExcluded(column string) clause.Expr {
	return clause.Expr{SQL: "EXCLUDED." + column}
}
