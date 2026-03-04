package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type OpsMetricsSummaryResponse struct {
	Days          int     `json:"days"`
	DownloadCount int64   `json:"download_count"`
	SearchCount   int64   `json:"search_count"`
	ViewCount     int64   `json:"view_count"`
	AvgHeatScore  float64 `json:"avg_heat_score"`
}

type OpsTopCategoryItem struct {
	CategoryID    uint64  `json:"category_id"`
	Name          string  `json:"name,omitempty"`
	ParentID      *uint64 `json:"parent_id,omitempty"`
	DownloadCount int64   `json:"download_count"`
	SearchCount   int64   `json:"search_count"`
	ViewCount     int64   `json:"view_count"`
	HeatScore     float64 `json:"heat_score"`
}

type OpsSearchTermItem struct {
	Term        string `json:"term"`
	SearchCount int64  `json:"search_count"`
}

func (h *Handler) GetOpsMetricsSummary(c *gin.Context) {
	days := parseDays(c.Query("days"), 7)
	startDate := time.Now().AddDate(0, 0, -(days - 1))

	var row struct {
		DownloadCount int64
		SearchCount   int64
		ViewCount     int64
		AvgHeatScore  float64
	}
	if err := h.db.Table("audit.category_daily_stats").
		Select("COALESCE(SUM(download_count),0) as download_count, COALESCE(SUM(search_count),0) as search_count, COALESCE(SUM(view_count),0) as view_count, COALESCE(AVG(heat_score),0) as avg_heat_score").
		Where("stat_date >= ?", startDate).
		Scan(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, OpsMetricsSummaryResponse{
		Days:          days,
		DownloadCount: row.DownloadCount,
		SearchCount:   row.SearchCount,
		ViewCount:     row.ViewCount,
		AvgHeatScore:  row.AvgHeatScore,
	})
}

func (h *Handler) ListOpsTopCategories(c *gin.Context) {
	days := parseDays(c.Query("days"), 7)
	limit := parseLimit(c.Query("limit"), 10, 50)
	startDate := time.Now().AddDate(0, 0, -(days - 1))

	var rows []OpsTopCategoryItem
	if err := h.db.Table("audit.category_daily_stats AS s").
		Select("s.category_id, c.name, c.parent_id, COALESCE(SUM(s.download_count),0) as download_count, COALESCE(SUM(s.search_count),0) as search_count, COALESCE(SUM(s.view_count),0) as view_count, COALESCE(SUM(s.heat_score),0) as heat_score").
		Joins("LEFT JOIN taxonomy.categories c ON c.id = s.category_id").
		Where("s.stat_date >= ?", startDate).
		Group("s.category_id, c.name, c.parent_id").
		Order("heat_score DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rows)
}

func (h *Handler) ListOpsSearchTerms(c *gin.Context) {
	days := parseDays(c.Query("days"), 7)
	limit := parseLimit(c.Query("limit"), 10, 50)
	startDate := time.Now().AddDate(0, 0, -(days - 1))

	var rows []OpsSearchTermItem
	if err := h.db.Table("audit.search_term_daily_stats").
		Select("normalized_term as term, COALESCE(SUM(search_count),0) as search_count").
		Where("stat_date >= ?", startDate).
		Group("normalized_term").
		Order("search_count DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rows)
}

func parseDays(val string, def int) int {
	days, err := strconv.Atoi(val)
	if err != nil || days <= 0 {
		return def
	}
	if days > 365 {
		return 365
	}
	return days
}

func parseLimit(val string, def int, max int) int {
	limit, err := strconv.Atoi(val)
	if err != nil || limit <= 0 {
		return def
	}
	if limit > max {
		return max
	}
	return limit
}
