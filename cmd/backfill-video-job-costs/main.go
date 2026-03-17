package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/db"
	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

type report struct {
	Apply        bool
	TotalScanned int
	Updated      int
	Failed       int
	LastID       uint64
	StartedAt    time.Time
	FinishedAt   time.Time
}

func main() {
	apply := flag.Bool("apply", false, "apply upsert to ops.video_job_costs")
	startID := flag.Uint64("start-id", 0, "start from job id (exclusive)")
	limit := flag.Int("limit", 0, "max jobs to scan (0 = no limit)")
	batchSize := flag.Int("batch-size", 200, "batch size")
	statusesRaw := flag.String("statuses", "done,failed,cancelled", "comma separated statuses to include")
	flag.Parse()

	if *batchSize <= 0 || *batchSize > 2000 {
		*batchSize = 200
	}

	_ = godotenv.Load()
	cfg := config.Load()
	dbConn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	result := run(dbConn, *apply, *startID, *limit, *batchSize, parseStatuses(*statusesRaw))
	fmt.Printf(
		"apply=%v total=%d updated=%d failed=%d last_id=%d elapsed=%s\n",
		result.Apply,
		result.TotalScanned,
		result.Updated,
		result.Failed,
		result.LastID,
		result.FinishedAt.Sub(result.StartedAt).Round(time.Millisecond).String(),
	)
}

func run(dbConn *gorm.DB, apply bool, startID uint64, limit, batchSize int, statuses []string) report {
	out := report{
		Apply:     apply,
		LastID:    startID,
		StartedAt: time.Now(),
	}

	for {
		if limit > 0 && out.TotalScanned >= limit {
			break
		}

		var rows []models.VideoJob
		query := dbConn.Model(&models.VideoJob{}).
			Select("id").
			Where("id > ?", out.LastID).
			Order("id ASC").
			Limit(batchSize)
		if len(statuses) > 0 {
			query = query.Where("status IN ?", statuses)
		}
		if err := query.Find(&rows).Error; err != nil {
			log.Printf("scan failed: %v", err)
			out.Failed++
			break
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			out.LastID = row.ID
			if limit > 0 && out.TotalScanned >= limit {
				break
			}
			out.TotalScanned++
			if !apply {
				continue
			}
			if err := videojobs.UpsertJobCost(dbConn, row.ID); err != nil {
				out.Failed++
				log.Printf("job_id=%d cost upsert failed: %v", row.ID, err)
				continue
			}
			out.Updated++
		}
	}

	out.FinishedAt = time.Now()
	return out
}

func parseStatuses(raw string) []string {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	seen := map[string]struct{}{}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
