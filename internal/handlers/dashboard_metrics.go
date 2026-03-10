package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type DashboardTrendPoint struct {
	Date          string `json:"date"`
	NewEmojis     int64  `json:"new_emojis"`
	Downloads     int64  `json:"downloads"`
	BlockedEvents int64  `json:"blocked_events"`
}

type DashboardTrendResponse struct {
	Days  int                   `json:"days"`
	Items []DashboardTrendPoint `json:"items"`
}

func (h *Handler) GetAdminDashboardTrends(c *gin.Context) {
	days := parseDays(c.Query("days"), 7)
	if days > 60 {
		days = 60
	}

	loc := loadStatsLocation()
	today := normalizeStatDate(time.Now().In(loc), loc)
	start := today.AddDate(0, 0, -(days - 1))
	end := today.Add(24 * time.Hour)

	type row struct {
		StatDate      time.Time `gorm:"column:stat_date"`
		NewEmojis     int64     `gorm:"column:new_emojis"`
		Downloads     int64     `gorm:"column:downloads"`
		BlockedEvents int64     `gorm:"column:blocked_events"`
	}
	var rows []row
	if err := h.db.Raw(`
		WITH day_series AS (
			SELECT generate_series(?::date, ?::date, interval '1 day')::date AS stat_date
		),
		emoji_daily AS (
			SELECT
				DATE(timezone('Asia/Shanghai', e.created_at)) AS stat_date,
				COUNT(*) AS cnt
			FROM archive.emojis e
			JOIN archive.collections c ON c.id = e.collection_id
			WHERE e.deleted_at IS NULL
			  AND e.status = 'active'
			  AND c.deleted_at IS NULL
			  AND c.status = 'active'
			  AND c.visibility = 'public'
			  AND e.created_at >= ? AND e.created_at < ?
			GROUP BY 1
		),
		download_daily AS (
			SELECT
				DATE(timezone('Asia/Shanghai', created_at)) AS stat_date,
				COUNT(*) AS cnt
			FROM (
				SELECT created_at FROM action.downloads WHERE created_at >= ? AND created_at < ?
				UNION ALL
				SELECT created_at FROM action.collection_downloads WHERE created_at >= ? AND created_at < ?
			) t
			GROUP BY 1
		),
		blocked_daily AS (
			SELECT
				DATE(timezone('Asia/Shanghai', created_at)) AS stat_date,
				COUNT(*) AS cnt
			FROM ops.risk_events
			WHERE event_type = 'blacklist_block'
			  AND created_at >= ? AND created_at < ?
			GROUP BY 1
		)
		SELECT
			ds.stat_date,
			COALESCE(e.cnt, 0) AS new_emojis,
			COALESCE(d.cnt, 0) AS downloads,
			COALESCE(b.cnt, 0) AS blocked_events
		FROM day_series ds
		LEFT JOIN emoji_daily e ON e.stat_date = ds.stat_date
		LEFT JOIN download_daily d ON d.stat_date = ds.stat_date
		LEFT JOIN blocked_daily b ON b.stat_date = ds.stat_date
		ORDER BY ds.stat_date ASC
	`, start.Format("2006-01-02"), today.Format("2006-01-02"),
		start, end,
		start, end,
		start, end,
		start, end,
	).Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]DashboardTrendPoint, 0, len(rows))
	for _, r := range rows {
		items = append(items, DashboardTrendPoint{
			Date:          r.StatDate.Format("2006-01-02"),
			NewEmojis:     r.NewEmojis,
			Downloads:     r.Downloads,
			BlockedEvents: r.BlockedEvents,
		})
	}

	c.JSON(http.StatusOK, DashboardTrendResponse{
		Days:  days,
		Items: items,
	})
}
