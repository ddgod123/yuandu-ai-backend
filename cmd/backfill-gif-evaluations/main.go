package main

import (
	"flag"
	"fmt"
	"log"
	"sort"
	"time"

	"emoji/internal/config"
	"emoji/internal/db"
	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

type report struct {
	Apply           bool
	OnlyMissing     bool
	SyncBaseline    bool
	TotalScanned    int
	Upserted        int
	BaselineSynced  int
	Failed          int
	LastOutputID    uint64
	StartedAt       time.Time
	FinishedAt      time.Time
	ProcessedJobIDs map[uint64]struct{}
}

func main() {
	apply := flag.Bool("apply", false, "apply backfill writes to archive.video_job_gif_evaluations")
	startID := flag.Uint64("start-id", 0, "start from output id (exclusive)")
	limit := flag.Int("limit", 0, "max outputs to scan (0 = no limit)")
	batchSize := flag.Int("batch-size", 200, "batch size")
	onlyMissing := flag.Bool("only-missing", true, "only scan outputs without evaluation rows")
	syncBaseline := flag.Bool("sync-baseline", true, "sync daily gif baseline for processed jobs")
	flag.Parse()

	if *batchSize <= 0 || *batchSize > 5000 {
		*batchSize = 200
	}

	_ = godotenv.Load()
	cfg := config.Load()
	dbConn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	result := run(dbConn, *apply, *startID, *limit, *batchSize, *onlyMissing, *syncBaseline)
	fmt.Printf(
		"apply=%v only_missing=%v sync_baseline=%v total=%d upserted=%d baseline_synced=%d failed=%d last_output_id=%d elapsed=%s\n",
		result.Apply,
		result.OnlyMissing,
		result.SyncBaseline,
		result.TotalScanned,
		result.Upserted,
		result.BaselineSynced,
		result.Failed,
		result.LastOutputID,
		result.FinishedAt.Sub(result.StartedAt).Round(time.Millisecond).String(),
	)
}

func run(dbConn *gorm.DB, apply bool, startID uint64, limit, batchSize int, onlyMissing, syncBaseline bool) report {
	out := report{
		Apply:           apply,
		OnlyMissing:     onlyMissing,
		SyncBaseline:    syncBaseline,
		LastOutputID:    startID,
		StartedAt:       time.Now(),
		ProcessedJobIDs: map[uint64]struct{}{},
	}

	for {
		if limit > 0 && out.TotalScanned >= limit {
			break
		}

		var rows []models.VideoImageOutputPublic
		query := dbConn.Model(&models.VideoImageOutputPublic{}).
			Where("id > ?", out.LastOutputID).
			Where("format = ? AND file_role = ?", "gif", "main")
		if onlyMissing {
			query = query.Where("NOT EXISTS (SELECT 1 FROM archive.video_job_gif_evaluations e WHERE e.output_id = public.video_image_outputs.id)")
		}
		if err := query.Order("id ASC").Limit(batchSize).Find(&rows).Error; err != nil {
			out.Failed++
			log.Printf("scan outputs failed: %v", err)
			break
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			out.LastOutputID = row.ID
			if limit > 0 && out.TotalScanned >= limit {
				break
			}
			out.TotalScanned++
			if !apply {
				continue
			}
			if err := dbConn.Transaction(func(tx *gorm.DB) error {
				return videojobs.UpsertGIFEvaluationByPublicOutput(tx, row)
			}); err != nil {
				out.Failed++
				log.Printf("output_id=%d job_id=%d upsert gif evaluation failed: %v", row.ID, row.JobID, err)
				continue
			}
			out.Upserted++
			if row.JobID > 0 {
				out.ProcessedJobIDs[row.JobID] = struct{}{}
			}
		}
	}

	if apply && syncBaseline && len(out.ProcessedJobIDs) > 0 {
		jobIDs := make([]uint64, 0, len(out.ProcessedJobIDs))
		for jobID := range out.ProcessedJobIDs {
			jobIDs = append(jobIDs, jobID)
		}
		sort.Slice(jobIDs, func(i, j int) bool { return jobIDs[i] < jobIDs[j] })
		for _, jobID := range jobIDs {
			if err := videojobs.SyncGIFBaselineByJobID(dbConn, jobID); err != nil {
				out.Failed++
				log.Printf("job_id=%d sync gif baseline failed: %v", jobID, err)
				continue
			}
			out.BaselineSynced++
		}
	}

	out.FinishedAt = time.Now()
	return out
}
