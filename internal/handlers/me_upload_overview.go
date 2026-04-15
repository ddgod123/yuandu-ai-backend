package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
)

const (
	myUploadOverviewDefaultRange = "7d"
	myUploadOverviewTopLimit     = 10
)

type MyUploadOverviewSummary struct {
	CollectionCount    int64 `json:"collection_count"`
	EmojiCount         int64 `json:"emoji_count"`
	LikeCount          int64 `json:"like_count"`
	FavoriteCount      int64 `json:"favorite_count"`
	DownloadCount      int64 `json:"download_count"`
	RangeLikeCount     int64 `json:"range_like_count"`
	RangeFavoriteCount int64 `json:"range_favorite_count"`
	RangeDownloadCount int64 `json:"range_download_count"`
}

type MyUploadOverviewTrendPoint struct {
	Date          string `json:"date"`
	LikeCount     int64  `json:"like_count"`
	FavoriteCount int64  `json:"favorite_count"`
	DownloadCount int64  `json:"download_count"`
}

type MyUploadOverviewCollectionItem struct {
	ID                uint64 `json:"id"`
	Title             string `json:"title"`
	CoverURL          string `json:"cover_url,omitempty"`
	FileCount         int    `json:"file_count"`
	Status            string `json:"status,omitempty"`
	Visibility        string `json:"visibility,omitempty"`
	LikeCount         int64  `json:"like_count"`
	FavoriteCount     int64  `json:"favorite_count"`
	DownloadCount     int64  `json:"download_count"`
	TotalInteractions int64  `json:"total_interactions"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

type MyUploadOverviewEmojiItem struct {
	ID                uint64 `json:"id"`
	CollectionID      uint64 `json:"collection_id"`
	CollectionTitle   string `json:"collection_title,omitempty"`
	Title             string `json:"title"`
	PreviewURL        string `json:"preview_url,omitempty"`
	Format            string `json:"format,omitempty"`
	LikeCount         int64  `json:"like_count"`
	FavoriteCount     int64  `json:"favorite_count"`
	DownloadCount     int64  `json:"download_count"`
	TotalInteractions int64  `json:"total_interactions"`
}

type MyUploadOverviewResponse struct {
	Range          string                           `json:"range"`
	StartDate      string                           `json:"start_date"`
	EndDate        string                           `json:"end_date"`
	Summary        MyUploadOverviewSummary          `json:"summary"`
	Trend          []MyUploadOverviewTrendPoint     `json:"trend"`
	TopCollections []MyUploadOverviewCollectionItem `json:"top_collections"`
	TopEmojis      []MyUploadOverviewEmojiItem      `json:"top_emojis"`
}

type myUploadOverviewDayRow struct {
	Day   time.Time `gorm:"column:day"`
	Count int64     `gorm:"column:count"`
}

func parseMyUploadOverviewRange(raw string) (string, int) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "30d", "30", "month", "monthly":
		return "30d", 30
	case "7d", "7", "week", "weekly", "":
		return myUploadOverviewDefaultRange, 7
	default:
		return myUploadOverviewDefaultRange, 7
	}
}

func buildMyUploadOverviewTrendSkeleton(start time.Time, days int) []MyUploadOverviewTrendPoint {
	if days <= 0 {
		return []MyUploadOverviewTrendPoint{}
	}
	points := make([]MyUploadOverviewTrendPoint, 0, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		points = append(points, MyUploadOverviewTrendPoint{
			Date: day.Format("2006-01-02"),
		})
	}
	return points
}

func applyMyUploadOverviewTrendRows(points []MyUploadOverviewTrendPoint, rows []myUploadOverviewDayRow, field string) int64 {
	if len(points) == 0 || len(rows) == 0 {
		return 0
	}
	indexByDate := make(map[string]int, len(points))
	for idx, point := range points {
		indexByDate[point.Date] = idx
	}
	var total int64
	for _, row := range rows {
		dateKey := row.Day.In(time.Local).Format("2006-01-02")
		index, ok := indexByDate[dateKey]
		if !ok {
			continue
		}
		switch field {
		case "like":
			points[index].LikeCount += row.Count
		case "favorite":
			points[index].FavoriteCount += row.Count
		case "download":
			points[index].DownloadCount += row.Count
		}
		total += row.Count
	}
	return total
}

func (h *Handler) loadMyUploadOverviewTrendRows(table string, userID uint64, start, end time.Time) []myUploadOverviewDayRow {
	if userID == 0 || table == "" {
		return []myUploadOverviewDayRow{}
	}
	rows := make([]myUploadOverviewDayRow, 0)
	query := fmt.Sprintf(`
SELECT DATE(t.created_at) AS day, COUNT(*) AS count
FROM %s AS t
JOIN archive.collections c ON c.id = t.collection_id
WHERE c.owner_id = ?
  AND c.source = ?
  AND c.owner_deleted_at IS NULL
  AND c.deleted_at IS NULL
  AND t.created_at >= ?
  AND t.created_at < ?
GROUP BY DATE(t.created_at)
ORDER BY DATE(t.created_at)
`, table)
	_ = h.db.Raw(query, userID, ugcCollectionSource, start, end).Scan(&rows).Error
	return rows
}

func (h *Handler) loadMyUploadOverviewTotalCount(table string, userID uint64) int64 {
	if userID == 0 || table == "" {
		return 0
	}
	var total int64
	query := fmt.Sprintf(`
SELECT COUNT(*) AS total
FROM %s AS t
JOIN archive.collections c ON c.id = t.collection_id
WHERE c.owner_id = ?
  AND c.source = ?
  AND c.owner_deleted_at IS NULL
  AND c.deleted_at IS NULL
`, table)
	_ = h.db.Raw(query, userID, ugcCollectionSource).Scan(&total).Error
	return total
}

// GetMyUploadOverview returns interaction overview for current user's upload collections.
// @Summary Get my upload interaction overview
// @Tags user
// @Produce json
// @Param range query string false "range(7d/30d)"
// @Success 200 {object} MyUploadOverviewResponse
// @Router /api/me/uploads/overview [get]
func (h *Handler) GetMyUploadOverview(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	rangeLabel, rangeDays := parseMyUploadOverviewRange(c.Query("range"))
	now := time.Now().In(time.Local)
	rangeEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).Add(24 * time.Hour)
	rangeStart := rangeEnd.AddDate(0, 0, -rangeDays)

	trend := buildMyUploadOverviewTrendSkeleton(rangeStart, rangeDays)
	likeRows := h.loadMyUploadOverviewTrendRows("action.collection_likes", userID, rangeStart, rangeEnd)
	favoriteRows := h.loadMyUploadOverviewTrendRows("action.collection_favorites", userID, rangeStart, rangeEnd)
	downloadRows := h.loadMyUploadOverviewTrendRows("action.collection_downloads", userID, rangeStart, rangeEnd)
	rangeLikeCount := applyMyUploadOverviewTrendRows(trend, likeRows, "like")
	rangeFavoriteCount := applyMyUploadOverviewTrendRows(trend, favoriteRows, "favorite")
	rangeDownloadCount := applyMyUploadOverviewTrendRows(trend, downloadRows, "download")

	var collections []models.Collection
	_ = h.db.Model(&models.Collection{}).
		Where("owner_id = ? AND source = ? AND owner_deleted_at IS NULL", userID, ugcCollectionSource).
		Order("updated_at DESC, id DESC").
		Find(&collections).Error

	summary := MyUploadOverviewSummary{
		CollectionCount:    int64(len(collections)),
		RangeLikeCount:     rangeLikeCount,
		RangeFavoriteCount: rangeFavoriteCount,
		RangeDownloadCount: rangeDownloadCount,
	}
	summary.LikeCount = h.loadMyUploadOverviewTotalCount("action.collection_likes", userID)
	summary.FavoriteCount = h.loadMyUploadOverviewTotalCount("action.collection_favorites", userID)
	summary.DownloadCount = h.loadMyUploadOverviewTotalCount("action.collection_downloads", userID)

	collectionIDs := make([]uint64, 0, len(collections))
	for _, item := range collections {
		if item.ID == 0 {
			continue
		}
		collectionIDs = append(collectionIDs, item.ID)
	}
	if len(collectionIDs) > 0 {
		_ = h.db.Model(&models.Emoji{}).
			Where("collection_id IN ?", collectionIDs).
			Count(&summary.EmojiCount).Error
	}

	topCollections := make([]MyUploadOverviewCollectionItem, 0, len(collections))
	if len(collections) > 0 {
		statMap := loadCollectionStats(h.db, collections)
		for _, item := range collections {
			stats := statMap[item.ID]
			topCollections = append(topCollections, MyUploadOverviewCollectionItem{
				ID:                item.ID,
				Title:             item.Title,
				CoverURL:          resolveListStaticPreviewURL(item.CoverURL, h.qiniu),
				FileCount:         item.FileCount,
				Status:            item.Status,
				Visibility:        item.Visibility,
				LikeCount:         stats.LikeCount,
				FavoriteCount:     stats.FavoriteCount,
				DownloadCount:     stats.DownloadCount,
				TotalInteractions: stats.LikeCount + stats.FavoriteCount + stats.DownloadCount,
				UpdatedAt:         item.UpdatedAt.Format(time.RFC3339),
			})
		}
		sort.Slice(topCollections, func(i, j int) bool {
			if topCollections[i].TotalInteractions != topCollections[j].TotalInteractions {
				return topCollections[i].TotalInteractions > topCollections[j].TotalInteractions
			}
			if topCollections[i].DownloadCount != topCollections[j].DownloadCount {
				return topCollections[i].DownloadCount > topCollections[j].DownloadCount
			}
			if topCollections[i].FavoriteCount != topCollections[j].FavoriteCount {
				return topCollections[i].FavoriteCount > topCollections[j].FavoriteCount
			}
			if topCollections[i].LikeCount != topCollections[j].LikeCount {
				return topCollections[i].LikeCount > topCollections[j].LikeCount
			}
			return topCollections[i].ID > topCollections[j].ID
		})
		if len(topCollections) > myUploadOverviewTopLimit {
			topCollections = topCollections[:myUploadOverviewTopLimit]
		}
	}

	type emojiRow struct {
		ID              uint64 `gorm:"column:id"`
		CollectionID    uint64 `gorm:"column:collection_id"`
		CollectionTitle string `gorm:"column:collection_title"`
		Title           string `gorm:"column:title"`
		StaticSource    string `gorm:"column:static_source"`
		Format          string `gorm:"column:format"`
		LikeCount       int64  `gorm:"column:like_count"`
		FavoriteCount   int64  `gorm:"column:favorite_count"`
		DownloadCount   int64  `gorm:"column:download_count"`
	}

	emojiRows := make([]emojiRow, 0)
	_ = h.db.Raw(`
SELECT
	e.id,
	e.collection_id,
	c.title AS collection_title,
	e.title,
	COALESCE(NULLIF(TRIM(e.thumb_url), ''), e.file_url) AS static_source,
	COALESCE(NULLIF(TRIM(e.format), ''), '') AS format,
	COALESCE(l.like_count, 0) AS like_count,
	COALESCE(f.favorite_count, 0) AS favorite_count,
	COALESCE(d.download_count, 0) AS download_count
FROM archive.emojis e
JOIN archive.collections c ON c.id = e.collection_id
LEFT JOIN (
	SELECT emoji_id, COUNT(*) AS like_count
	FROM action.likes
	GROUP BY emoji_id
) l ON l.emoji_id = e.id
LEFT JOIN (
	SELECT emoji_id, COUNT(*) AS favorite_count
	FROM action.favorites
	GROUP BY emoji_id
) f ON f.emoji_id = e.id
LEFT JOIN (
	SELECT emoji_id, COUNT(*) AS download_count
	FROM action.downloads
	GROUP BY emoji_id
) d ON d.emoji_id = e.id
WHERE c.owner_id = ?
  AND c.source = ?
  AND c.owner_deleted_at IS NULL
  AND c.deleted_at IS NULL
  AND e.deleted_at IS NULL
ORDER BY (COALESCE(l.like_count, 0) + COALESCE(f.favorite_count, 0) + COALESCE(d.download_count, 0)) DESC, e.id DESC
LIMIT ?
`, userID, ugcCollectionSource, myUploadOverviewTopLimit).Scan(&emojiRows).Error

	topEmojis := make([]MyUploadOverviewEmojiItem, 0, len(emojiRows))
	for _, row := range emojiRows {
		total := row.LikeCount + row.FavoriteCount + row.DownloadCount
		topEmojis = append(topEmojis, MyUploadOverviewEmojiItem{
			ID:                row.ID,
			CollectionID:      row.CollectionID,
			CollectionTitle:   row.CollectionTitle,
			Title:             row.Title,
			PreviewURL:        resolveListStaticPreviewURL(row.StaticSource, h.qiniu),
			Format:            row.Format,
			LikeCount:         row.LikeCount,
			FavoriteCount:     row.FavoriteCount,
			DownloadCount:     row.DownloadCount,
			TotalInteractions: total,
		})
	}

	c.JSON(http.StatusOK, MyUploadOverviewResponse{
		Range:          rangeLabel,
		StartDate:      rangeStart.Format("2006-01-02"),
		EndDate:        rangeEnd.Add(-24 * time.Hour).Format("2006-01-02"),
		Summary:        summary,
		Trend:          trend,
		TopCollections: topCollections,
		TopEmojis:      topEmojis,
	})
}
