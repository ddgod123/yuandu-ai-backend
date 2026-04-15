package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type AdminAnalyticsQueryResponse struct {
	Range     string `json:"range"`
	Dimension string `json:"dimension"`
	Timezone  string `json:"timezone,omitempty"`
	From      string `json:"from"`
	To        string `json:"to"`
}

type AdminAnalyticsOverview struct {
	PV             int64   `json:"pv"`
	UV             int64   `json:"uv"`
	Sessions       int64   `json:"sessions"`
	Downloads      int64   `json:"downloads"`
	DownloadUsers  int64   `json:"downloadUsers"`
	DownloadRate   float64 `json:"downloadRate"`
	NewVisitorRate float64 `json:"newVisitorRate"`
	AvgStaySeconds int64   `json:"avgStaySeconds"`
}

type AdminAnalyticsTrendPoint struct {
	Date         string  `json:"date"`
	PV           int64   `json:"pv"`
	UV           int64   `json:"uv"`
	Downloads    int64   `json:"downloads"`
	DownloadRate float64 `json:"downloadRate"`
}

type AdminAnalyticsTopDownloadItem struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Downloads    int64   `json:"downloads"`
	UV           int64   `json:"uv"`
	DownloadRate float64 `json:"downloadRate"`
	GrowthRate   float64 `json:"growthRate"`
}

type AdminAnalyticsTopPageItem struct {
	Path         string  `json:"path"`
	Title        string  `json:"title"`
	PV           int64   `json:"pv"`
	UV           int64   `json:"uv"`
	DownloadRate float64 `json:"downloadRate"`
	ExitRate     float64 `json:"exitRate"`
}

type AdminAnalyticsGeoItem struct {
	Country   string  `json:"country"`
	Region    string  `json:"region"`
	City      string  `json:"city"`
	Visits    int64   `json:"visits"`
	UV        int64   `json:"uv"`
	UniqueIPs int64   `json:"uniqueIps"`
	Downloads int64   `json:"downloads"`
	Share     float64 `json:"share"`
}

type AdminAnalyticsBreakdownItem struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Value int64   `json:"value"`
	Share float64 `json:"share"`
}

type AdminAnalyticsRealtimePage struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	PV30m int64  `json:"pv30m"`
}

type AdminAnalyticsRealtime struct {
	PV30m        int64                        `json:"pv30m"`
	Downloads30m int64                        `json:"downloads30m"`
	OnlineUsers  int64                        `json:"onlineUsers"`
	ActivePages  []AdminAnalyticsRealtimePage `json:"activePages"`
}

type AdminAnalyticsDashboardResponse struct {
	Source         string                          `json:"source"`
	GeneratedAt    string                          `json:"generatedAt"`
	Query          AdminAnalyticsQueryResponse     `json:"query"`
	Overview       AdminAnalyticsOverview          `json:"overview"`
	Trends         []AdminAnalyticsTrendPoint      `json:"trends"`
	TopCollections []AdminAnalyticsTopDownloadItem `json:"topCollections"`
	TopEmojis      []AdminAnalyticsTopDownloadItem `json:"topEmojis"`
	TopPages       []AdminAnalyticsTopPageItem     `json:"topPages"`
	Geo            []AdminAnalyticsGeoItem         `json:"geo"`
	Channels       []AdminAnalyticsBreakdownItem   `json:"channels"`
	Devices        []AdminAnalyticsBreakdownItem   `json:"devices"`
	Realtime       AdminAnalyticsRealtime          `json:"realtime"`
}

type adminAnalyticsQueryWindow struct {
	Range     string
	Dimension string
	Timezone  string
	From      string
	To        string
	Loc       *time.Location
	Start     time.Time
	End       time.Time
}

// GetAdminAnalyticsDashboard godoc
// @Summary Get admin analytics dashboard (real data)
// @Tags admin
// @Produce json
// @Success 200 {object} AdminAnalyticsDashboardResponse
// @Router /api/admin/analytics/dashboard [get]
func (h *Handler) GetAdminAnalyticsDashboard(c *gin.Context) {
	query, err := parseAdminAnalyticsQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	overview, err := h.loadAdminAnalyticsOverview(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "overview query failed: " + err.Error()})
		return
	}
	trends, err := h.loadAdminAnalyticsTrends(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "trend query failed: " + err.Error()})
		return
	}
	topCollections, err := h.loadAdminAnalyticsTopCollections(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "top collection query failed: " + err.Error()})
		return
	}
	topEmojis, err := h.loadAdminAnalyticsTopEmojis(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "top emoji query failed: " + err.Error()})
		return
	}
	topPages, err := h.loadAdminAnalyticsTopPages(query)
	if err != nil {
		// Degrade gracefully: keep dashboard available even if top-page aggregation fails.
		topPages = []AdminAnalyticsTopPageItem{}
	}
	geo, err := h.loadAdminAnalyticsGeo(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "geo query failed: " + err.Error()})
		return
	}
	channels, err := h.loadAdminAnalyticsChannels(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "channel query failed: " + err.Error()})
		return
	}
	devices, err := h.loadAdminAnalyticsDevices(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "device query failed: " + err.Error()})
		return
	}
	realtime, err := h.loadAdminAnalyticsRealtime()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "realtime query failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, AdminAnalyticsDashboardResponse{
		Source:      "api",
		GeneratedAt: time.Now().In(query.Loc).Format(time.RFC3339),
		Query: AdminAnalyticsQueryResponse{
			Range:     query.Range,
			Dimension: query.Dimension,
			Timezone:  query.Timezone,
			From:      query.From,
			To:        query.To,
		},
		Overview:       overview,
		Trends:         trends,
		TopCollections: topCollections,
		TopEmojis:      topEmojis,
		TopPages:       topPages,
		Geo:            geo,
		Channels:       channels,
		Devices:        devices,
		Realtime:       realtime,
	})
}

func parseAdminAnalyticsQuery(c *gin.Context) (adminAnalyticsQueryWindow, error) {
	rangeRaw := strings.ToLower(strings.TrimSpace(c.Query("range")))
	if rangeRaw == "" {
		rangeRaw = "7d"
	}
	validRange := map[string]bool{
		"today":     true,
		"yesterday": true,
		"7d":        true,
		"30d":       true,
		"custom":    true,
	}
	if !validRange[rangeRaw] {
		return adminAnalyticsQueryWindow{}, fmt.Errorf("invalid range")
	}

	dimension := strings.ToLower(strings.TrimSpace(c.Query("dimension")))
	if dimension == "" {
		dimension = "all"
	}
	validDimension := map[string]bool{
		"all":     true,
		"country": true,
		"region":  true,
		"city":    true,
		"channel": true,
		"device":  true,
	}
	if !validDimension[dimension] {
		return adminAnalyticsQueryWindow{}, fmt.Errorf("invalid dimension")
	}

	timezone := strings.TrimSpace(c.Query("timezone"))
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return adminAnalyticsQueryWindow{}, fmt.Errorf("invalid timezone")
	}

	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	var startLocal time.Time
	var endLocalExclusive time.Time

	if rangeRaw == "custom" {
		fromRaw := strings.TrimSpace(c.Query("from"))
		toRaw := strings.TrimSpace(c.Query("to"))
		if fromRaw == "" || toRaw == "" {
			return adminAnalyticsQueryWindow{}, fmt.Errorf("custom range requires from and to")
		}
		fromDate, err := time.ParseInLocation("2006-01-02", fromRaw, loc)
		if err != nil {
			return adminAnalyticsQueryWindow{}, fmt.Errorf("invalid from date")
		}
		toDate, err := time.ParseInLocation("2006-01-02", toRaw, loc)
		if err != nil {
			return adminAnalyticsQueryWindow{}, fmt.Errorf("invalid to date")
		}
		if toDate.Before(fromDate) {
			return adminAnalyticsQueryWindow{}, fmt.Errorf("to must be greater than or equal to from")
		}
		if toDate.Sub(fromDate) > 89*24*time.Hour {
			return adminAnalyticsQueryWindow{}, fmt.Errorf("date range cannot exceed 90 days")
		}
		startLocal = fromDate
		endLocalExclusive = toDate.Add(24 * time.Hour)
	} else {
		switch rangeRaw {
		case "today":
			startLocal = today
			endLocalExclusive = today.Add(24 * time.Hour)
		case "yesterday":
			startLocal = today.Add(-24 * time.Hour)
			endLocalExclusive = today
		case "30d":
			startLocal = today.AddDate(0, 0, -29)
			endLocalExclusive = today.Add(24 * time.Hour)
		default: // 7d
			startLocal = today.AddDate(0, 0, -6)
			endLocalExclusive = today.Add(24 * time.Hour)
		}
	}

	from := startLocal.Format("2006-01-02")
	to := endLocalExclusive.Add(-time.Nanosecond).In(loc).Format("2006-01-02")

	return adminAnalyticsQueryWindow{
		Range:     rangeRaw,
		Dimension: dimension,
		Timezone:  timezone,
		From:      from,
		To:        to,
		Loc:       loc,
		Start:     startLocal,
		End:       endLocalExclusive,
	}, nil
}

func (h *Handler) loadAdminAnalyticsOverview(query adminAnalyticsQueryWindow) (AdminAnalyticsOverview, error) {
	out := AdminAnalyticsOverview{}

	var traffic struct {
		PV       int64 `gorm:"column:pv"`
		UV       int64 `gorm:"column:uv"`
		Sessions int64 `gorm:"column:sessions"`
	}
	if err := h.db.Raw(`
		SELECT
			COUNT(*) AS pv,
			COUNT(DISTINCT COALESCE(
				CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
				NULLIF(e.device_id, ''),
				NULLIF(e.session_id, ''),
				NULLIF(e.metadata->>'request_ip', ''),
				'event:' || e.id::text
			)) AS uv,
			COUNT(DISTINCT COALESCE(
				NULLIF(e.session_id, ''),
				NULLIF(e.device_id, ''),
				CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
				'event:' || e.id::text
			)) AS sessions
		FROM action.user_behavior_events e
		WHERE e.event_name LIKE 'page_view%'
		  AND e.created_at >= ? AND e.created_at < ?
	`, query.Start, query.End).Scan(&traffic).Error; err != nil {
		return out, err
	}

	var downloads struct {
		Downloads     int64 `gorm:"column:downloads"`
		DownloadUsers int64 `gorm:"column:download_users"`
	}
	if err := h.db.Raw(`
		WITH all_downloads AS (
			SELECT user_id, NULLIF(ip, '') AS ip, id::text AS row_id
			FROM action.downloads
			WHERE created_at >= ? AND created_at < ?
			UNION ALL
			SELECT user_id, NULLIF(ip, '') AS ip, id::text AS row_id
			FROM action.collection_downloads
			WHERE created_at >= ? AND created_at < ?
		)
		SELECT
			COUNT(*) AS downloads,
			COUNT(DISTINCT COALESCE(
				CASE WHEN user_id IS NOT NULL THEN 'u:' || user_id::text END,
				ip,
				'row:' || row_id
			)) AS download_users
		FROM all_downloads
	`, query.Start, query.End, query.Start, query.End).Scan(&downloads).Error; err != nil {
		return out, err
	}

	var avgStay struct {
		AvgStaySeconds float64 `gorm:"column:avg_stay_seconds"`
	}
	if err := h.db.Raw(`
		WITH session_stats AS (
			SELECT
				COALESCE(
					NULLIF(e.session_id, ''),
					NULLIF(e.device_id, ''),
					CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
					'event:' || e.id::text
				) AS session_key,
				MIN(e.created_at) AS first_at,
				MAX(e.created_at) AS last_at
			FROM action.user_behavior_events e
			WHERE e.event_name LIKE 'page_view%'
			  AND e.created_at >= ? AND e.created_at < ?
			GROUP BY 1
		)
		SELECT COALESCE(AVG(LEAST(3600, GREATEST(0, EXTRACT(EPOCH FROM (last_at - first_at))))), 0) AS avg_stay_seconds
		FROM session_stats
	`, query.Start, query.End).Scan(&avgStay).Error; err != nil {
		return out, err
	}

	var newVisitors struct {
		NewRate float64 `gorm:"column:new_rate"`
	}
	if err := h.db.Raw(`
		WITH window_visitors AS (
			SELECT DISTINCT COALESCE(
				CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
				NULLIF(e.device_id, ''),
				NULLIF(e.session_id, ''),
				NULLIF(e.metadata->>'request_ip', ''),
				'event:' || e.id::text
			) AS visitor_key
			FROM action.user_behavior_events e
			WHERE e.event_name LIKE 'page_view%'
			  AND e.created_at >= ? AND e.created_at < ?
		),
		first_seen AS (
			SELECT
				w.visitor_key,
				MIN(e.created_at) AS first_seen_at
			FROM window_visitors w
			JOIN action.user_behavior_events e
			  ON COALESCE(
					CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
					NULLIF(e.device_id, ''),
					NULLIF(e.session_id, ''),
					NULLIF(e.metadata->>'request_ip', ''),
					'event:' || e.id::text
				 ) = w.visitor_key
			WHERE e.event_name LIKE 'page_view%'
			  AND e.created_at < ?
			GROUP BY 1
		)
		SELECT COALESCE(AVG(CASE WHEN first_seen_at >= ? THEN 1.0 ELSE 0.0 END), 0) AS new_rate
		FROM first_seen
	`, query.Start, query.End, query.End, query.Start).Scan(&newVisitors).Error; err != nil {
		return out, err
	}

	out.PV = maxInt64(0, traffic.PV)
	out.UV = maxInt64(0, traffic.UV)
	out.Sessions = maxInt64(0, traffic.Sessions)
	out.Downloads = maxInt64(0, downloads.Downloads)
	out.DownloadUsers = maxInt64(0, downloads.DownloadUsers)
	if out.UV > 0 {
		out.DownloadRate = clampRate(float64(out.DownloadUsers) / float64(out.UV))
	}
	out.NewVisitorRate = clampRate(newVisitors.NewRate)
	out.AvgStaySeconds = int64(maxFloat64(0, avgStay.AvgStaySeconds))

	return out, nil
}

func (h *Handler) loadAdminAnalyticsTrends(query adminAnalyticsQueryWindow) ([]AdminAnalyticsTrendPoint, error) {
	type row struct {
		StatDate      time.Time `gorm:"column:stat_date"`
		PV            int64     `gorm:"column:pv"`
		UV            int64     `gorm:"column:uv"`
		Downloads     int64     `gorm:"column:downloads"`
		DownloadUsers int64     `gorm:"column:download_users"`
	}
	var rows []row

	if err := h.db.Raw(`
		WITH day_series AS (
			SELECT generate_series(?::date, ?::date, interval '1 day')::date AS stat_date
		),
		pv_daily AS (
			SELECT
				DATE(timezone(?, e.created_at)) AS stat_date,
				COUNT(*) AS pv,
				COUNT(DISTINCT COALESCE(
					CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
					NULLIF(e.device_id, ''),
					NULLIF(e.session_id, ''),
					NULLIF(e.metadata->>'request_ip', ''),
					'event:' || e.id::text
				)) AS uv
			FROM action.user_behavior_events e
			WHERE e.event_name LIKE 'page_view%'
			  AND e.created_at >= ? AND e.created_at < ?
			GROUP BY 1
		),
		download_daily AS (
			SELECT
				DATE(timezone(?, d.created_at)) AS stat_date,
				COUNT(*) AS downloads,
				COUNT(DISTINCT COALESCE(
					CASE WHEN d.user_id IS NOT NULL THEN 'u:' || d.user_id::text END,
					d.ip,
					'd:' || d.row_id
				)) AS download_users
			FROM (
				SELECT created_at, user_id, NULLIF(ip, '') AS ip, id::text AS row_id
				FROM action.downloads
				WHERE created_at >= ? AND created_at < ?
				UNION ALL
				SELECT created_at, user_id, NULLIF(ip, '') AS ip, id::text AS row_id
				FROM action.collection_downloads
				WHERE created_at >= ? AND created_at < ?
			) d
			GROUP BY 1
		)
		SELECT
			ds.stat_date,
			COALESCE(pv.pv, 0) AS pv,
			COALESCE(pv.uv, 0) AS uv,
			COALESCE(dd.downloads, 0) AS downloads,
			COALESCE(dd.download_users, 0) AS download_users
		FROM day_series ds
		LEFT JOIN pv_daily pv ON pv.stat_date = ds.stat_date
		LEFT JOIN download_daily dd ON dd.stat_date = ds.stat_date
		ORDER BY ds.stat_date ASC
	`, query.From, query.To,
		query.Timezone,
		query.Start, query.End,
		query.Timezone,
		query.Start, query.End,
		query.Start, query.End,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]AdminAnalyticsTrendPoint, 0, len(rows))
	for _, r := range rows {
		rate := 0.0
		if r.UV > 0 {
			rate = clampRate(float64(r.DownloadUsers) / float64(r.UV))
		}
		items = append(items, AdminAnalyticsTrendPoint{
			Date:         r.StatDate.In(query.Loc).Format("2006-01-02"),
			PV:           maxInt64(0, r.PV),
			UV:           maxInt64(0, r.UV),
			Downloads:    maxInt64(0, r.Downloads),
			DownloadRate: rate,
		})
	}
	return items, nil
}

func (h *Handler) loadAdminAnalyticsTopCollections(query adminAnalyticsQueryWindow) ([]AdminAnalyticsTopDownloadItem, error) {
	type currentRow struct {
		CollectionID int64  `gorm:"column:collection_id"`
		Name         string `gorm:"column:name"`
		Downloads    int64  `gorm:"column:downloads"`
		UV           int64  `gorm:"column:uv"`
	}
	var currentRows []currentRow
	if err := h.db.Raw(`
		SELECT
			cd.collection_id,
			COALESCE(NULLIF(c.title, ''), '合集 #' || cd.collection_id::text) AS name,
			COUNT(*) AS downloads,
			COUNT(DISTINCT COALESCE(
				CASE WHEN cd.user_id IS NOT NULL THEN 'u:' || cd.user_id::text END,
				NULLIF(cd.ip, ''),
				'row:' || cd.id::text
			)) AS uv
		FROM action.collection_downloads cd
		LEFT JOIN archive.collections c ON c.id = cd.collection_id
		WHERE cd.created_at >= ? AND cd.created_at < ?
		GROUP BY cd.collection_id, c.title
		ORDER BY downloads DESC
		LIMIT 20
	`, query.Start, query.End).Scan(&currentRows).Error; err != nil {
		return nil, err
	}

	span := query.End.Sub(query.Start)
	prevStart := query.Start.Add(-span)
	prevEnd := query.Start

	var previousRows []struct {
		CollectionID int64 `gorm:"column:collection_id"`
		Downloads    int64 `gorm:"column:downloads"`
	}
	if err := h.db.Raw(`
		SELECT collection_id, COUNT(*) AS downloads
		FROM action.collection_downloads
		WHERE created_at >= ? AND created_at < ?
		GROUP BY collection_id
	`, prevStart, prevEnd).Scan(&previousRows).Error; err != nil {
		return nil, err
	}
	prevByID := make(map[int64]int64, len(previousRows))
	for _, item := range previousRows {
		prevByID[item.CollectionID] = item.Downloads
	}

	items := make([]AdminAnalyticsTopDownloadItem, 0, len(currentRows))
	for _, item := range currentRows {
		rate := 0.0
		if item.UV > 0 {
			rate = clampRate(float64(item.Downloads) / float64(item.UV))
		}
		prev := prevByID[item.CollectionID]
		growth := 0.0
		if prev > 0 {
			growth = (float64(item.Downloads) - float64(prev)) / float64(prev)
		} else if item.Downloads > 0 {
			growth = 1
		}
		items = append(items, AdminAnalyticsTopDownloadItem{
			ID:           fmt.Sprintf("collection-%d", item.CollectionID),
			Name:         strings.TrimSpace(item.Name),
			Downloads:    maxInt64(0, item.Downloads),
			UV:           maxInt64(0, item.UV),
			DownloadRate: rate,
			GrowthRate:   growth,
		})
	}
	return items, nil
}

func (h *Handler) loadAdminAnalyticsTopEmojis(query adminAnalyticsQueryWindow) ([]AdminAnalyticsTopDownloadItem, error) {
	type currentRow struct {
		EmojiID   int64  `gorm:"column:emoji_id"`
		Name      string `gorm:"column:name"`
		Downloads int64  `gorm:"column:downloads"`
		UV        int64  `gorm:"column:uv"`
	}
	var currentRows []currentRow
	if err := h.db.Raw(`
		SELECT
			d.emoji_id,
			COALESCE(NULLIF(e.title, ''), '表情 #' || d.emoji_id::text) AS name,
			COUNT(*) AS downloads,
			COUNT(DISTINCT COALESCE(
				CASE WHEN d.user_id IS NOT NULL THEN 'u:' || d.user_id::text END,
				NULLIF(d.ip, ''),
				'row:' || d.id::text
			)) AS uv
		FROM action.downloads d
		LEFT JOIN archive.emojis e ON e.id = d.emoji_id
		WHERE d.created_at >= ? AND d.created_at < ?
		GROUP BY d.emoji_id, e.title
		ORDER BY downloads DESC
		LIMIT 20
	`, query.Start, query.End).Scan(&currentRows).Error; err != nil {
		return nil, err
	}

	span := query.End.Sub(query.Start)
	prevStart := query.Start.Add(-span)
	prevEnd := query.Start

	var previousRows []struct {
		EmojiID   int64 `gorm:"column:emoji_id"`
		Downloads int64 `gorm:"column:downloads"`
	}
	if err := h.db.Raw(`
		SELECT emoji_id, COUNT(*) AS downloads
		FROM action.downloads
		WHERE created_at >= ? AND created_at < ?
		GROUP BY emoji_id
	`, prevStart, prevEnd).Scan(&previousRows).Error; err != nil {
		return nil, err
	}
	prevByID := make(map[int64]int64, len(previousRows))
	for _, item := range previousRows {
		prevByID[item.EmojiID] = item.Downloads
	}

	items := make([]AdminAnalyticsTopDownloadItem, 0, len(currentRows))
	for _, item := range currentRows {
		rate := 0.0
		if item.UV > 0 {
			rate = clampRate(float64(item.Downloads) / float64(item.UV))
		}
		prev := prevByID[item.EmojiID]
		growth := 0.0
		if prev > 0 {
			growth = (float64(item.Downloads) - float64(prev)) / float64(prev)
		} else if item.Downloads > 0 {
			growth = 1
		}
		items = append(items, AdminAnalyticsTopDownloadItem{
			ID:           fmt.Sprintf("emoji-%d", item.EmojiID),
			Name:         strings.TrimSpace(item.Name),
			Downloads:    maxInt64(0, item.Downloads),
			UV:           maxInt64(0, item.UV),
			DownloadRate: rate,
			GrowthRate:   growth,
		})
	}
	return items, nil
}

func (h *Handler) loadAdminAnalyticsTopPages(query adminAnalyticsQueryWindow) ([]AdminAnalyticsTopPageItem, error) {
	type row struct {
		Path         string  `gorm:"column:path"`
		PV           int64   `gorm:"column:pv"`
		UV           int64   `gorm:"column:uv"`
		DownloadRate float64 `gorm:"column:download_rate"`
		ExitRate     float64 `gorm:"column:exit_rate"`
	}
	var rows []row

	if err := h.db.Raw(`
		WITH page_events AS (
			SELECT
				COALESCE(NULLIF(split_part(e.route, chr(63), 1), ''), '/') AS path,
				COALESCE(
					CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
					NULLIF(e.device_id, ''),
					NULLIF(e.session_id, ''),
					NULLIF(e.metadata->>'request_ip', ''),
					'event:' || e.id::text
				) AS visitor_key,
				COALESCE(
					NULLIF(e.session_id, ''),
					NULLIF(e.device_id, ''),
					CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
					'event:' || e.id::text
				) AS session_key
			FROM action.user_behavior_events e
			WHERE e.event_name LIKE 'page_view%%'
			  AND e.created_at >= ? AND e.created_at < ?
		),
		page_stats AS (
			SELECT
				path,
				COUNT(*) AS pv,
				COUNT(DISTINCT visitor_key) AS uv,
				COUNT(DISTINCT session_key) AS sessions
			FROM page_events
			GROUP BY path
		),
		session_path_counts AS (
			SELECT path, session_key, COUNT(*) AS cnt
			FROM page_events
			GROUP BY path, session_key
		),
		bounce_stats AS (
			SELECT
				path,
				COUNT(*) FILTER (WHERE cnt = 1) AS bounce_sessions
			FROM session_path_counts
			GROUP BY path
		),
		download_events AS (
			SELECT
				COALESCE(NULLIF(split_part(e.route, chr(63), 1), ''), '/') AS path,
				COUNT(*) AS downloads
			FROM action.user_behavior_events e
			WHERE e.event_name IN (
				'emoji_download',
				'collection_zip_download',
				'download_collection_ticket_consumed',
				'download_single_ticket_consumed'
			)
			  AND COALESCE(e.success, TRUE) = TRUE
			  AND e.created_at >= ? AND e.created_at < ?
			GROUP BY 1
		)
		SELECT
			ps.path,
			ps.pv,
			ps.uv,
			CASE WHEN ps.uv > 0
				THEN COALESCE(de.downloads, 0)::double precision / ps.uv::double precision
				ELSE 0
			END AS download_rate,
			CASE WHEN ps.sessions > 0
				THEN COALESCE(bs.bounce_sessions, 0)::double precision / ps.sessions::double precision
				ELSE 0
			END AS exit_rate
		FROM page_stats ps
		LEFT JOIN download_events de ON de.path = ps.path
		LEFT JOIN bounce_stats bs ON bs.path = ps.path
		ORDER BY ps.pv DESC
		LIMIT 20
	`, query.Start, query.End, query.Start, query.End).Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]AdminAnalyticsTopPageItem, 0, len(rows))
	for _, item := range rows {
		path := normalizeRoutePath(item.Path)
		items = append(items, AdminAnalyticsTopPageItem{
			Path:         path,
			Title:        analyticsRouteTitle(path),
			PV:           maxInt64(0, item.PV),
			UV:           maxInt64(0, item.UV),
			DownloadRate: clampRate(item.DownloadRate),
			ExitRate:     clampRate(item.ExitRate),
		})
	}
	return items, nil
}

func (h *Handler) loadAdminAnalyticsGeo(query adminAnalyticsQueryWindow) ([]AdminAnalyticsGeoItem, error) {
	countryExprE := "COALESCE(NULLIF(e.metadata->>'country', ''), NULLIF(e.metadata->>'country_name', ''), NULLIF(e.metadata->>'cf_ip_country', ''), NULLIF(e.metadata->>'x_country', ''), '未知')"
	regionExprE := "COALESCE(NULLIF(e.metadata->>'region', ''), NULLIF(e.metadata->>'province', ''), NULLIF(e.metadata->>'x_region', ''), '未知')"
	cityExprE := "COALESCE(NULLIF(e.metadata->>'city', ''), NULLIF(e.metadata->>'x_city', ''), '未知')"

	countryExprD := "COALESCE(NULLIF(d.metadata->>'country', ''), NULLIF(d.metadata->>'country_name', ''), NULLIF(d.metadata->>'cf_ip_country', ''), NULLIF(d.metadata->>'x_country', ''), '未知')"
	regionExprD := "COALESCE(NULLIF(d.metadata->>'region', ''), NULLIF(d.metadata->>'province', ''), NULLIF(d.metadata->>'x_region', ''), '未知')"
	cityExprD := "COALESCE(NULLIF(d.metadata->>'city', ''), NULLIF(d.metadata->>'x_city', ''), '未知')"

	visitCountry, visitRegion, visitCity := countryExprE, regionExprE, cityExprE
	downloadCountry, downloadRegion, downloadCity := countryExprD, regionExprD, cityExprD
	groupBy := "country, region, city"

	switch query.Dimension {
	case "country":
		visitRegion = "''"
		visitCity = "''"
		downloadRegion = "''"
		downloadCity = "''"
		groupBy = "country"
	case "region":
		visitCity = "''"
		downloadCity = "''"
		groupBy = "country, region"
	}

	stmt := fmt.Sprintf(`
		WITH page_geo AS (
			SELECT
				%s AS country,
				%s AS region,
				%s AS city,
				COALESCE(
					CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
					NULLIF(e.device_id, ''),
					NULLIF(e.session_id, ''),
					NULLIF(e.metadata->>'request_ip', ''),
					'event:' || e.id::text
				) AS visitor_key,
				NULLIF(e.metadata->>'request_ip', '') AS request_ip
			FROM action.user_behavior_events e
			WHERE e.event_name LIKE 'page_view%%'
			  AND e.created_at >= ? AND e.created_at < ?
		),
		visit_stats AS (
			SELECT
				country,
				region,
				city,
				COUNT(*) AS visits,
				COUNT(DISTINCT visitor_key) AS uv,
				COUNT(DISTINCT request_ip) AS unique_ips
			FROM page_geo
			GROUP BY %s
		),
		download_stats AS (
			SELECT
				%s AS country,
				%s AS region,
				%s AS city,
				COUNT(*) AS downloads
			FROM action.user_behavior_events d
			WHERE d.event_name IN (
				'emoji_download',
				'collection_zip_download',
				'download_collection_ticket_consumed',
				'download_single_ticket_consumed'
			)
			  AND COALESCE(d.success, TRUE) = TRUE
			  AND d.created_at >= ? AND d.created_at < ?
			GROUP BY 1, 2, 3
		),
		total AS (
			SELECT COALESCE(SUM(visits), 0) AS total_visits FROM visit_stats
		)
		SELECT
			v.country,
			v.region,
			v.city,
			v.visits,
			v.uv,
			v.unique_ips,
			COALESCE(d.downloads, 0) AS downloads,
			CASE WHEN t.total_visits > 0
				THEN v.visits::double precision / t.total_visits::double precision
				ELSE 0
			END AS share
		FROM visit_stats v
		LEFT JOIN download_stats d
		  ON d.country = v.country AND d.region = v.region AND d.city = v.city
		CROSS JOIN total t
		ORDER BY v.visits DESC
		LIMIT 50
	`, visitCountry, visitRegion, visitCity, groupBy, downloadCountry, downloadRegion, downloadCity)

	type row struct {
		Country   string  `gorm:"column:country"`
		Region    string  `gorm:"column:region"`
		City      string  `gorm:"column:city"`
		Visits    int64   `gorm:"column:visits"`
		UV        int64   `gorm:"column:uv"`
		UniqueIPs int64   `gorm:"column:unique_ips"`
		Downloads int64   `gorm:"column:downloads"`
		Share     float64 `gorm:"column:share"`
	}
	var rows []row
	if err := h.db.Raw(stmt, query.Start, query.End, query.Start, query.End).Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]AdminAnalyticsGeoItem, 0, len(rows))
	for _, item := range rows {
		items = append(items, AdminAnalyticsGeoItem{
			Country:   strings.TrimSpace(item.Country),
			Region:    strings.TrimSpace(item.Region),
			City:      strings.TrimSpace(item.City),
			Visits:    maxInt64(0, item.Visits),
			UV:        maxInt64(0, item.UV),
			UniqueIPs: maxInt64(0, item.UniqueIPs),
			Downloads: maxInt64(0, item.Downloads),
			Share:     clampRate(item.Share),
		})
	}
	return items, nil
}

func (h *Handler) loadAdminAnalyticsChannels(query adminAnalyticsQueryWindow) ([]AdminAnalyticsBreakdownItem, error) {
	type row struct {
		ChannelKey string  `gorm:"column:channel_key"`
		Value      int64   `gorm:"column:value"`
		Share      float64 `gorm:"column:share"`
	}
	var rows []row

	if err := h.db.Raw(`
		WITH page_events AS (
			SELECT
				CASE
					WHEN e.route ILIKE '%utm_source=%' OR e.route ILIKE '%utm_medium=%' OR e.route ILIKE '%utm_campaign=%' THEN 'campaign'
					WHEN COALESCE(BTRIM(e.referrer), '') = '' THEN 'direct'
					WHEN LOWER(e.referrer) ~ '(baidu|google|bing|sogou|so\\.com|yahoo)' THEN 'search'
					WHEN LOWER(e.referrer) ~ '(douyin|tiktok|xiaohongshu|xhs|weibo|twitter|facebook|instagram|wechat|weixin|qq\\.com)' THEN 'social'
					ELSE 'external'
				END AS channel_key,
				COALESCE(
					CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
					NULLIF(e.device_id, ''),
					NULLIF(e.session_id, ''),
					NULLIF(e.metadata->>'request_ip', ''),
					'event:' || e.id::text
				) AS visitor_key
			FROM action.user_behavior_events e
			WHERE e.event_name LIKE 'page_view%'
			  AND e.created_at >= ? AND e.created_at < ?
		),
		agg AS (
			SELECT channel_key, COUNT(DISTINCT visitor_key) AS value
			FROM page_events
			GROUP BY channel_key
		),
		total AS (
			SELECT COALESCE(SUM(value), 0) AS total_value FROM agg
		)
		SELECT
			a.channel_key,
			a.value,
			CASE WHEN t.total_value > 0
				THEN a.value::double precision / t.total_value::double precision
				ELSE 0
			END AS share
		FROM agg a
		CROSS JOIN total t
		ORDER BY a.value DESC
	`, query.Start, query.End).Scan(&rows).Error; err != nil {
		return nil, err
	}

	labelByKey := map[string]string{
		"direct":   "直接访问",
		"search":   "搜索引擎",
		"social":   "社媒分享",
		"external": "外链推荐",
		"campaign": "活动投放",
	}

	items := make([]AdminAnalyticsBreakdownItem, 0, len(rows))
	for _, item := range rows {
		key := strings.TrimSpace(item.ChannelKey)
		if key == "" {
			continue
		}
		label := labelByKey[key]
		if label == "" {
			label = key
		}
		items = append(items, AdminAnalyticsBreakdownItem{
			Key:   key,
			Label: label,
			Value: maxInt64(0, item.Value),
			Share: clampRate(item.Share),
		})
	}
	return items, nil
}

func (h *Handler) loadAdminAnalyticsDevices(query adminAnalyticsQueryWindow) ([]AdminAnalyticsBreakdownItem, error) {
	type row struct {
		DeviceKey string  `gorm:"column:device_key"`
		Value     int64   `gorm:"column:value"`
		Share     float64 `gorm:"column:share"`
	}
	var rows []row

	if err := h.db.Raw(`
		WITH page_events AS (
			SELECT
				CASE
					WHEN LOWER(COALESCE(NULLIF(e.metadata->>'device_type', ''), '')) IN ('mobile', 'phone') THEN 'mobile'
					WHEN LOWER(COALESCE(NULLIF(e.metadata->>'device_type', ''), '')) IN ('desktop', 'pc') THEN 'desktop'
					WHEN LOWER(COALESCE(NULLIF(e.metadata->>'device_type', ''), '')) IN ('tablet', 'pad') THEN 'tablet'
					WHEN LOWER(COALESCE(NULLIF(e.metadata->>'user_agent', ''), '')) ~ '(ipad|tablet)' THEN 'tablet'
					WHEN LOWER(COALESCE(NULLIF(e.metadata->>'user_agent', ''), '')) ~ '(iphone|android|mobile|harmony|miui|huawei|oppo|vivo)' THEN 'mobile'
					WHEN LOWER(COALESCE(NULLIF(e.metadata->>'user_agent', ''), '')) ~ '(windows|macintosh|linux|x11)' THEN 'desktop'
					ELSE 'other'
				END AS device_key,
				COALESCE(
					CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
					NULLIF(e.device_id, ''),
					NULLIF(e.session_id, ''),
					NULLIF(e.metadata->>'request_ip', ''),
					'event:' || e.id::text
				) AS visitor_key
			FROM action.user_behavior_events e
			WHERE e.event_name LIKE 'page_view%'
			  AND e.created_at >= ? AND e.created_at < ?
		),
		agg AS (
			SELECT device_key, COUNT(DISTINCT visitor_key) AS value
			FROM page_events
			GROUP BY device_key
		),
		total AS (
			SELECT COALESCE(SUM(value), 0) AS total_value FROM agg
		)
		SELECT
			a.device_key,
			a.value,
			CASE WHEN t.total_value > 0
				THEN a.value::double precision / t.total_value::double precision
				ELSE 0
			END AS share
		FROM agg a
		CROSS JOIN total t
		ORDER BY a.value DESC
	`, query.Start, query.End).Scan(&rows).Error; err != nil {
		return nil, err
	}

	labelByKey := map[string]string{
		"mobile":  "Mobile Web",
		"desktop": "Desktop",
		"tablet":  "Tablet",
		"other":   "其他设备",
	}

	items := make([]AdminAnalyticsBreakdownItem, 0, len(rows))
	for _, item := range rows {
		key := strings.TrimSpace(item.DeviceKey)
		if key == "" {
			continue
		}
		label := labelByKey[key]
		if label == "" {
			label = key
		}
		items = append(items, AdminAnalyticsBreakdownItem{
			Key:   key,
			Label: label,
			Value: maxInt64(0, item.Value),
			Share: clampRate(item.Share),
		})
	}
	return items, nil
}

func (h *Handler) loadAdminAnalyticsRealtime() (AdminAnalyticsRealtime, error) {
	out := AdminAnalyticsRealtime{ActivePages: make([]AdminAnalyticsRealtimePage, 0)}
	now := time.Now()
	windowStart := now.Add(-30 * time.Minute)

	var summary struct {
		PV30m       int64 `gorm:"column:pv_30m"`
		OnlineUsers int64 `gorm:"column:online_users"`
	}
	if err := h.db.Raw(`
		SELECT
			COUNT(*) AS pv_30m,
			COUNT(DISTINCT COALESCE(
				CASE WHEN e.user_id IS NOT NULL THEN 'u:' || e.user_id::text END,
				NULLIF(e.device_id, ''),
				NULLIF(e.session_id, ''),
				NULLIF(e.metadata->>'request_ip', ''),
				'event:' || e.id::text
			)) AS online_users
		FROM action.user_behavior_events e
		WHERE e.event_name LIKE 'page_view%'
		  AND e.created_at >= ? AND e.created_at < ?
	`, windowStart, now).Scan(&summary).Error; err != nil {
		return out, err
	}

	var downloadSummary struct {
		Downloads30m int64 `gorm:"column:downloads_30m"`
	}
	if err := h.db.Raw(`
		WITH all_downloads AS (
			SELECT id FROM action.downloads WHERE created_at >= ? AND created_at < ?
			UNION ALL
			SELECT id FROM action.collection_downloads WHERE created_at >= ? AND created_at < ?
		)
		SELECT COUNT(*) AS downloads_30m FROM all_downloads
	`, windowStart, now, windowStart, now).Scan(&downloadSummary).Error; err != nil {
		return out, err
	}

	type pageRow struct {
		Path string `gorm:"column:path"`
		PV30 int64  `gorm:"column:pv_30m"`
	}
	var pageRows []pageRow
	if err := h.db.Raw(`
		SELECT
			COALESCE(NULLIF(split_part(e.route, chr(63), 1), ''), '/') AS path,
			COUNT(*) AS pv_30m
		FROM action.user_behavior_events e
		WHERE e.event_name LIKE 'page_view%'
		  AND e.created_at >= ? AND e.created_at < ?
		GROUP BY 1
		ORDER BY pv_30m DESC
		LIMIT 8
	`, windowStart, now).Scan(&pageRows).Error; err != nil {
		return out, err
	}

	out.PV30m = maxInt64(0, summary.PV30m)
	out.Downloads30m = maxInt64(0, downloadSummary.Downloads30m)
	out.OnlineUsers = maxInt64(0, summary.OnlineUsers)
	out.ActivePages = make([]AdminAnalyticsRealtimePage, 0, len(pageRows))
	for _, item := range pageRows {
		path := normalizeRoutePath(item.Path)
		out.ActivePages = append(out.ActivePages, AdminAnalyticsRealtimePage{
			Path:  path,
			Title: analyticsRouteTitle(path),
			PV30m: maxInt64(0, item.PV30),
		})
	}
	return out, nil
}

func clampRate(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func normalizeRoutePath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "/"
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		if parsed, err := url.Parse(value); err == nil && strings.TrimSpace(parsed.Path) != "" {
			value = parsed.Path
		} else {
			value = "/"
		}
	}
	if idx := strings.Index(value, "?"); idx >= 0 {
		value = value[:idx]
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	if len(value) > 1 {
		value = strings.TrimRight(value, "/")
		if value == "" {
			value = "/"
		}
	}
	return value
}

func analyticsRouteTitle(path string) string {
	normalized := normalizeRoutePath(path)
	switch {
	case normalized == "/":
		return "首页"
	case strings.HasPrefix(normalized, "/collections/"):
		return "合集详情页"
	case normalized == "/collections":
		return "合集列表页"
	case normalized == "/categories":
		return "分类页"
	case strings.HasPrefix(normalized, "/categories/"):
		return "分类详情页"
	case normalized == "/trending":
		return "IP 趋势页"
	case strings.HasPrefix(normalized, "/trending/"):
		return "IP 详情页"
	case normalized == "/showcase":
		return "创作展示页"
	case normalized == "/emoji-recommend":
		return "推荐页"
	default:
		return normalized
	}
}
