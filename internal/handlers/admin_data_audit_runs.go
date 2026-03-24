package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
)

type AdminDataAuditRunItem struct {
	ID                      uint64    `json:"id"`
	RunAt                   time.Time `json:"run_at"`
	Status                  string    `json:"status"`
	Apply                   bool      `json:"apply"`
	FixOrphans              bool      `json:"fix_orphans"`
	DurationMs              int64     `json:"duration_ms"`
	ReportPath              string    `json:"report_path"`
	ErrorMessage            string    `json:"error_message"`
	DBEmojiTotal            int       `json:"db_emoji_total"`
	DBZipTotal              int       `json:"db_zip_total"`
	QiniuObjectTotal        int       `json:"qiniu_object_total"`
	MissingEmojiObjectCount int       `json:"missing_emoji_object_count"`
	MissingZipObjectCount   int       `json:"missing_zip_object_count"`
	QiniuOrphanRawCount     int       `json:"qiniu_orphan_raw_count"`
	QiniuOrphanZipCount     int       `json:"qiniu_orphan_zip_count"`
	FileCountMismatchCount  int       `json:"file_count_mismatch_count"`
	CreatedAt               time.Time `json:"created_at"`
}

type AdminDataAuditTrendPoint struct {
	Date                   string `json:"date"`
	Runs                   int    `json:"runs"`
	HealthyRuns            int    `json:"healthy_runs"`
	WarnRuns               int    `json:"warn_runs"`
	FailedRuns             int    `json:"failed_runs"`
	MaxMissingEmojiObjects int    `json:"max_missing_emoji_object_count"`
	MaxMissingZipObjects   int    `json:"max_missing_zip_object_count"`
	MaxQiniuOrphanRawCount int    `json:"max_qiniu_orphan_raw_count"`
	MaxFileCountMismatch   int    `json:"max_file_count_mismatch_count"`
}

type AdminDataAuditOverviewResponse struct {
	CheckedAt  time.Time                  `json:"checked_at"`
	WindowDays int                        `json:"window_days"`
	Total      int64                      `json:"total"`
	Latest     *AdminDataAuditRunItem     `json:"latest,omitempty"`
	Items      []AdminDataAuditRunItem    `json:"items"`
	Trend      []AdminDataAuditTrendPoint `json:"trend"`
}

func mapAdminDataAuditRunItem(row models.DataAuditRun) AdminDataAuditRunItem {
	return AdminDataAuditRunItem{
		ID:                      row.ID,
		RunAt:                   row.RunAt,
		Status:                  strings.TrimSpace(strings.ToLower(row.Status)),
		Apply:                   row.Apply,
		FixOrphans:              row.FixOrphans,
		DurationMs:              row.DurationMs,
		ReportPath:              strings.TrimSpace(row.ReportPath),
		ErrorMessage:            strings.TrimSpace(row.ErrorMessage),
		DBEmojiTotal:            row.DBEmojiTotal,
		DBZipTotal:              row.DBZipTotal,
		QiniuObjectTotal:        row.QiniuObjectTotal,
		MissingEmojiObjectCount: row.MissingEmojiObjectCount,
		MissingZipObjectCount:   row.MissingZipObjectCount,
		QiniuOrphanRawCount:     row.QiniuOrphanRawCount,
		QiniuOrphanZipCount:     row.QiniuOrphanZipCount,
		FileCountMismatchCount:  row.FileCountMismatchCount,
		CreatedAt:               row.CreatedAt,
	}
}

func normalizeDataAuditStatus(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "healthy":
		return "healthy"
	case "warn":
		return "warn"
	case "failed":
		return "failed"
	default:
		return ""
	}
}

// GetAdminDataAuditOverview godoc
// @Summary Get data-audit runs overview (admin)
// @Tags admin
// @Produce json
// @Param limit query int false "limit, default 30, max 200"
// @Param window_days query int false "trend window days, default 7, max 30"
// @Param status query string false "status filter: all|healthy|warn|failed"
// @Success 200 {object} AdminDataAuditOverviewResponse
// @Router /api/admin/system/data-audit/overview [get]
func (h *Handler) GetAdminDataAuditOverview(c *gin.Context) {
	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "30")))
	if limit <= 0 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}

	windowDays, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("window_days", "7")))
	if windowDays <= 0 {
		windowDays = 7
	}
	if windowDays > 30 {
		windowDays = 30
	}

	statusFilter := normalizeDataAuditStatus(c.Query("status"))

	query := h.db.Model(&models.DataAuditRun{})
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []models.DataAuditRun
	if err := query.Order("run_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]AdminDataAuditRunItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapAdminDataAuditRunItem(row))
	}

	var latest *AdminDataAuditRunItem
	if len(items) > 0 {
		latest = &items[0]
	}

	// Trend always uses real status distribution in the requested window.
	trendSince := time.Now().AddDate(0, 0, -windowDays+1)
	var trendRows []models.DataAuditRun
	trendQuery := h.db.Model(&models.DataAuditRun{}).Where("run_at >= ?", trendSince)
	if err := trendQuery.Order("run_at ASC, id ASC").Find(&trendRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type agg struct {
		AdminDataAuditTrendPoint
	}
	byDate := make(map[string]*agg, windowDays)
	for _, row := range trendRows {
		day := row.RunAt.In(time.Local).Format("2006-01-02")
		node, ok := byDate[day]
		if !ok {
			node = &agg{AdminDataAuditTrendPoint: AdminDataAuditTrendPoint{Date: day}}
			byDate[day] = node
		}
		node.Runs++
		switch normalizeDataAuditStatus(row.Status) {
		case "healthy":
			node.HealthyRuns++
		case "warn":
			node.WarnRuns++
		case "failed":
			node.FailedRuns++
		}
		if row.MissingEmojiObjectCount > node.MaxMissingEmojiObjects {
			node.MaxMissingEmojiObjects = row.MissingEmojiObjectCount
		}
		if row.MissingZipObjectCount > node.MaxMissingZipObjects {
			node.MaxMissingZipObjects = row.MissingZipObjectCount
		}
		if row.QiniuOrphanRawCount > node.MaxQiniuOrphanRawCount {
			node.MaxQiniuOrphanRawCount = row.QiniuOrphanRawCount
		}
		if row.FileCountMismatchCount > node.MaxFileCountMismatch {
			node.MaxFileCountMismatch = row.FileCountMismatchCount
		}
	}

	trend := make([]AdminDataAuditTrendPoint, 0, len(byDate))
	for _, node := range byDate {
		trend = append(trend, node.AdminDataAuditTrendPoint)
	}
	sort.Slice(trend, func(i, j int) bool { return trend[i].Date < trend[j].Date })

	c.JSON(http.StatusOK, AdminDataAuditOverviewResponse{
		CheckedAt:  time.Now(),
		WindowDays: windowDays,
		Total:      total,
		Latest:     latest,
		Items:      items,
		Trend:      trend,
	})
}
