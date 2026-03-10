package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TodayStatsResponse struct {
	Date           string `json:"date"`
	TodayNewEmojis int64  `json:"today_new_emojis"`
}

type HomeStatsResponse struct {
	StatDate         string `json:"stat_date"`
	TotalCollections int64  `json:"total_collections"`
	TotalEmojis      int64  `json:"total_emojis"`
	TodayNewEmojis   int64  `json:"today_new_emojis"`
	UpdatedAt        string `json:"updated_at"`
	Source           string `json:"source"`
}

type homeStatsSnapshot struct {
	StatDate         time.Time
	TotalCollections int64
	TotalEmojis      int64
	TodayNewEmojis   int64
	UpdatedAt        time.Time
}

// GetTodayStats godoc
// @Summary Get today stats
// @Tags public
// @Produce json
// @Success 200 {object} TodayStatsResponse
// @Router /api/stats/today [get]
func (h *Handler) GetTodayStats(c *gin.Context) {
	loc := loadStatsLocation()
	statDate := normalizeStatDate(time.Now().In(loc), loc)
	count, err := h.countTodayNewPublicEmojis(statDate, loc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, TodayStatsResponse{
		Date:           statDate.Format("2006-01-02"),
		TodayNewEmojis: count,
	})
}

// GetHomeStats godoc
// @Summary Get home page stats snapshot
// @Tags public
// @Produce json
// @Success 200 {object} HomeStatsResponse
// @Router /api/stats/home [get]
func (h *Handler) GetHomeStats(c *gin.Context) {
	loc := loadStatsLocation()
	snapshot, source, err := h.loadLatestHomeStatsSnapshot()
	if err != nil && !isMissingHomeStatsTableErr(err) && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if errors.Is(err, gorm.ErrRecordNotFound) || isMissingHomeStatsTableErr(err) {
		snapshot, err = h.computePublicHomeStats(time.Now().In(loc), loc)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		source = "live"
	} else if !isSameStatDate(snapshot.StatDate, time.Now().In(loc), loc) {
		// Snapshot table exists but latest row is stale (e.g. scheduler not run today).
		// Fallback to live aggregation to keep homepage counters valid.
		snapshot, err = h.computePublicHomeStats(time.Now().In(loc), loc)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		source = "live"
	}

	c.JSON(http.StatusOK, HomeStatsResponse{
		StatDate:         snapshot.StatDate.Format("2006-01-02"),
		TotalCollections: snapshot.TotalCollections,
		TotalEmojis:      snapshot.TotalEmojis,
		TodayNewEmojis:   snapshot.TodayNewEmojis,
		UpdatedAt:        snapshot.UpdatedAt.Format(time.RFC3339),
		Source:           source,
	})
}

func loadStatsLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Local
	}
	return loc
}

func normalizeStatDate(base time.Time, loc *time.Location) time.Time {
	v := base.In(loc)
	return time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, loc)
}

func isSameStatDate(a time.Time, b time.Time, loc *time.Location) bool {
	da := normalizeStatDate(a, loc)
	db := normalizeStatDate(b, loc)
	return da.Equal(db)
}

func (h *Handler) computePublicHomeStats(base time.Time, loc *time.Location) (homeStatsSnapshot, error) {
	statDate := normalizeStatDate(base, loc)
	stats := homeStatsSnapshot{
		StatDate:  statDate,
		UpdatedAt: time.Now().In(loc),
	}

	if err := h.db.Model(&models.Collection{}).
		Where("deleted_at IS NULL").
		Where("status = ?", "active").
		Where("visibility = ?", "public").
		Count(&stats.TotalCollections).Error; err != nil {
		return homeStatsSnapshot{}, err
	}

	baseEmojiQuery := h.db.Table("archive.emojis AS e").
		Joins("JOIN archive.collections c ON c.id = e.collection_id").
		Where("e.deleted_at IS NULL").
		Where("e.status = ?", "active").
		Where("c.deleted_at IS NULL").
		Where("c.status = ?", "active").
		Where("c.visibility = ?", "public")

	if err := baseEmojiQuery.Count(&stats.TotalEmojis).Error; err != nil {
		return homeStatsSnapshot{}, err
	}
	count, err := h.countTodayNewPublicEmojis(statDate, loc)
	if err != nil {
		return homeStatsSnapshot{}, err
	}
	stats.TodayNewEmojis = count

	return stats, nil
}

func (h *Handler) countTodayNewPublicEmojis(statDate time.Time, loc *time.Location) (int64, error) {
	start := normalizeStatDate(statDate, loc)
	end := start.Add(24 * time.Hour)

	var count int64
	err := h.db.Table("archive.emojis AS e").
		Joins("JOIN archive.collections c ON c.id = e.collection_id").
		Where("e.deleted_at IS NULL").
		Where("e.status = ?", "active").
		Where("c.deleted_at IS NULL").
		Where("c.status = ?", "active").
		Where("c.visibility = ?", "public").
		Where("e.created_at >= ? AND e.created_at < ?", start, end).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (h *Handler) loadLatestHomeStatsSnapshot() (homeStatsSnapshot, string, error) {
	var row models.HomeDailyStats
	if err := h.db.
		Table(row.TableName()).
		Order("stat_date DESC").
		Limit(1).
		Take(&row).Error; err != nil {
		return homeStatsSnapshot{}, "", err
	}
	return homeStatsSnapshot{
		StatDate:         row.StatDate,
		TotalCollections: row.TotalCollections,
		TotalEmojis:      row.TotalEmojis,
		TodayNewEmojis:   row.TodayNewEmojis,
		UpdatedAt:        row.UpdatedAt,
	}, "snapshot", nil
}

func isMissingHomeStatsTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "home_daily_stats") && strings.Contains(msg, "does not exist")
}
