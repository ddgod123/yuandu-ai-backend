package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"emoji/internal/config"
	"emoji/internal/db"
	"emoji/internal/models"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

type homeDailyStatsOutput struct {
	StatDate         string `json:"stat_date"`
	TotalCollections int64  `json:"total_collections"`
	TotalEmojis      int64  `json:"total_emojis"`
	TodayNewEmojis   int64  `json:"today_new_emojis"`
	UpdatedAt        string `json:"updated_at"`
	Written          bool   `json:"written"`
}

func main() {
	dateArg := flag.String("date", "", "stat date in YYYY-MM-DD (Asia/Shanghai), default today")
	dryRun := flag.Bool("dry-run", false, "compute only, do not write snapshot")
	flag.Parse()

	loadEnv()
	cfg := config.Load()

	dbConn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	if err := ensureHomeStatsTable(dbConn); err != nil {
		log.Fatalf("ensure home stats table failed: %v", err)
	}

	loc := loadStatsLocation()
	statDate, err := parseStatDate(*dateArg, loc)
	if err != nil {
		log.Fatalf("invalid -date value: %v", err)
	}

	stats, err := computeHomeStats(dbConn, statDate, loc)
	if err != nil {
		log.Fatalf("compute home stats failed: %v", err)
	}

	written := false
	if !*dryRun {
		if err := dbConn.Exec(`
			INSERT INTO audit.home_daily_stats (
				stat_date, total_collections, total_emojis, today_new_emojis, updated_at
			) VALUES (?, ?, ?, ?, NOW())
			ON CONFLICT (stat_date) DO UPDATE SET
				total_collections = EXCLUDED.total_collections,
				total_emojis = EXCLUDED.total_emojis,
				today_new_emojis = EXCLUDED.today_new_emojis,
				updated_at = NOW()
		`,
			stats.StatDate, stats.TotalCollections, stats.TotalEmojis, stats.TodayNewEmojis,
		).Error; err != nil {
			log.Fatalf("write home stats snapshot failed: %v", err)
		}
		written = true
	}

	out := homeDailyStatsOutput{
		StatDate:         stats.StatDate.Format("2006-01-02"),
		TotalCollections: stats.TotalCollections,
		TotalEmojis:      stats.TotalEmojis,
		TodayNewEmojis:   stats.TodayNewEmojis,
		UpdatedAt:        time.Now().In(loc).Format(time.RFC3339),
		Written:          written,
	}
	raw, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(raw))
}

func computeHomeStats(dbConn *gorm.DB, statDate time.Time, loc *time.Location) (models.HomeDailyStats, error) {
	start := normalizeStatDate(statDate, loc)
	end := start.Add(24 * time.Hour)

	stats := models.HomeDailyStats{
		StatDate:  start,
		UpdatedAt: time.Now().In(loc),
	}

	if err := dbConn.Model(&models.Collection{}).
		Where("deleted_at IS NULL").
		Where("status = ?", "active").
		Where("visibility = ?", "public").
		Count(&stats.TotalCollections).Error; err != nil {
		return models.HomeDailyStats{}, err
	}

	baseEmojiQuery := dbConn.Table("archive.emojis AS e").
		Joins("JOIN archive.collections c ON c.id = e.collection_id").
		Where("e.deleted_at IS NULL").
		Where("e.status = ?", "active").
		Where("c.deleted_at IS NULL").
		Where("c.status = ?", "active").
		Where("c.visibility = ?", "public")

	if err := baseEmojiQuery.Count(&stats.TotalEmojis).Error; err != nil {
		return models.HomeDailyStats{}, err
	}

	if err := baseEmojiQuery.
		Where("e.created_at >= ? AND e.created_at < ?", start, end).
		Count(&stats.TodayNewEmojis).Error; err != nil {
		return models.HomeDailyStats{}, err
	}

	return stats, nil
}

func ensureHomeStatsTable(dbConn *gorm.DB) error {
	if err := dbConn.Exec(`
		CREATE TABLE IF NOT EXISTS audit.home_daily_stats (
			stat_date DATE PRIMARY KEY,
			total_collections BIGINT NOT NULL DEFAULT 0,
			total_emojis BIGINT NOT NULL DEFAULT 0,
			today_new_emojis BIGINT NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`).Error; err != nil {
		return err
	}
	return dbConn.Exec(`
		CREATE INDEX IF NOT EXISTS idx_home_daily_stats_updated_at
		ON audit.home_daily_stats (updated_at DESC)
	`).Error
}

func loadStatsLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Local
	}
	return loc
}

func parseStatDate(raw string, loc *time.Location) (time.Time, error) {
	if raw == "" {
		return normalizeStatDate(time.Now().In(loc), loc), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, loc)
	if err != nil {
		return time.Time{}, err
	}
	return normalizeStatDate(parsed, loc), nil
}

func normalizeStatDate(base time.Time, loc *time.Location) time.Time {
	v := base.In(loc)
	return time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, loc)
}

func loadEnv() {
	seen := map[string]struct{}{}
	candidates := []string{".env", "backend/.env"}

	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for i := 0; i < 5; i++ {
			candidates = append(candidates,
				filepath.Join(dir, ".env"),
				filepath.Join(dir, "backend", ".env"),
			)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	for _, path := range candidates {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		if _, err := os.Stat(path); err == nil {
			_ = godotenv.Overload(path)
		}
	}
}
