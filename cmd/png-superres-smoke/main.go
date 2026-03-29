package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/db"
	"emoji/internal/models"
	"emoji/internal/storage"
	"emoji/internal/videojobs"

	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type smokeSummary struct {
	BaseJobID                  uint64                 `json:"base_job_id"`
	NewJobID                   uint64                 `json:"new_job_id"`
	Status                     string                 `json:"status"`
	Stage                      string                 `json:"stage"`
	Progress                   int                    `json:"progress"`
	ProcessingCallSec          float64                `json:"processing_call_sec"`
	PipelineElapsedSec         float64                `json:"pipeline_elapsed_sec"`
	SuperRes                   map[string]interface{} `json:"super_resolution"`
	WorkerSuperResUsageCalls   int64                  `json:"worker_superres_usage_calls"`
	WorkerSuperResUsageCostUSD float64                `json:"worker_superres_usage_cost_usd"`
	SourceVideoKey             string                 `json:"source_video_key"`
}

func main() {
	_ = godotenv.Load()
	_ = godotenv.Load(".env")
	_ = godotenv.Load("backend/.env")

	cfg := config.Load()
	database, err := db.Connect(cfg)
	if err != nil {
		panic(err)
	}
	qiniuClient, err := storage.NewQiniuClient(cfg)
	if err != nil {
		panic(err)
	}

	base, err := pickBasePNGJob(database)
	if err != nil {
		panic(err)
	}

	optionsMap := map[string]interface{}{}
	_ = json.Unmarshal(base.Options, &optionsMap)
	optionsMap["auto_highlight"] = true
	optionsMap["flow_mode"] = "auto"
	optionsMap["ai1_confirmed"] = true
	optionsMap["ai1_pause_consumed"] = true
	optionsJSON, _ := json.Marshal(optionsMap)

	newJob := models.VideoJob{
		UserID:         base.UserID,
		Title:          fmt.Sprintf("PNG超分冒烟-%d", time.Now().Unix()),
		SourceVideoKey: base.SourceVideoKey,
		CategoryID:     base.CategoryID,
		OutputFormats:  "png",
		Status:         models.VideoJobStatusQueued,
		Stage:          models.VideoJobStageQueued,
		Progress:       0,
		Priority:       base.Priority,
		Options:        datatypes.JSON(optionsJSON),
		Metrics:        datatypes.JSON([]byte("{}")),
	}

	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&newJob).Error; err != nil {
			return err
		}
		if err := videojobs.UpsertPublicVideoImageJob(tx, newJob); err != nil {
			return err
		}
		queuedEvent := models.VideoJobEvent{
			JobID:    newJob.ID,
			Stage:    models.VideoJobStageQueued,
			Level:    "info",
			Message:  "png super-resolution smoke job queued",
			Metadata: datatypes.JSON([]byte("{}")),
		}
		if err := tx.Create(&queuedEvent).Error; err != nil {
			return err
		}
		if err := videojobs.CreatePublicVideoImageEvent(tx, queuedEvent); err != nil {
			return err
		}
		return nil
	}); err != nil {
		panic(err)
	}

	processor := videojobs.NewProcessor(database, qiniuClient, cfg)
	task, err := videojobs.NewProcessVideoJobTask(newJob.ID)
	if err != nil {
		panic(err)
	}

	started := time.Now()
	err = processor.HandleProcessVideoJob(context.Background(), asynq.NewTask(videojobs.TaskTypeProcessVideoJob, task.Payload()))
	callElapsed := time.Since(started).Seconds()
	if err != nil {
		panic(err)
	}

	var row struct {
		Status     string
		Stage      string
		Progress   int
		ElapsedSec float64
		Metrics    datatypes.JSON
	}
	if err := database.Raw(`
SELECT status, stage, progress,
  CASE WHEN started_at IS NOT NULL AND finished_at IS NOT NULL
       THEN EXTRACT(EPOCH FROM (finished_at - started_at))
       ELSE 0 END AS elapsed_sec,
  metrics
FROM archive.video_jobs
WHERE id = ?
`, newJob.ID).Scan(&row).Error; err != nil {
		panic(err)
	}

	metricsMap := map[string]interface{}{}
	_ = json.Unmarshal(row.Metrics, &metricsMap)
	superRes := mapFromAny(metricsMap["png_worker_super_resolution_v1"])

	var usage struct {
		Calls   int64
		CostUSD float64
	}
	_ = database.Raw(`
SELECT COUNT(*) AS calls, COALESCE(SUM(cost_usd), 0) AS cost_usd
FROM ops.video_job_ai_usage
WHERE job_id = ? AND stage = 'worker_super_resolution'
`, newJob.ID).Scan(&usage).Error

	summary := smokeSummary{
		BaseJobID:                  base.ID,
		NewJobID:                   newJob.ID,
		Status:                     row.Status,
		Stage:                      row.Stage,
		Progress:                   row.Progress,
		ProcessingCallSec:          round(callElapsed, 3),
		PipelineElapsedSec:         round(row.ElapsedSec, 3),
		SuperRes:                   superRes,
		WorkerSuperResUsageCalls:   usage.Calls,
		WorkerSuperResUsageCostUSD: round(usage.CostUSD, 8),
		SourceVideoKey:             base.SourceVideoKey,
	}
	payload, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(payload))
}

func pickBasePNGJob(db *gorm.DB) (models.VideoJob, error) {
	var fromEnv uint64
	if raw := strings.TrimSpace(os.Getenv("BASE_JOB_ID")); raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &fromEnv)
	}
	if fromEnv > 0 {
		var job models.VideoJob
		if err := db.Where("id = ?", fromEnv).First(&job).Error; err != nil {
			return models.VideoJob{}, err
		}
		if !strings.Contains(strings.ToLower(job.OutputFormats), "png") {
			return models.VideoJob{}, fmt.Errorf("base job %d output_formats=%s is not png", job.ID, job.OutputFormats)
		}
		if strings.TrimSpace(job.SourceVideoKey) == "" {
			return models.VideoJob{}, fmt.Errorf("base job %d source_video_key empty", job.ID)
		}
		return job, nil
	}

	var job models.VideoJob
	err := db.Raw(`
SELECT *
FROM archive.video_jobs
WHERE status = 'done'
  AND LOWER(output_formats) LIKE '%png%'
  AND COALESCE(NULLIF(TRIM(source_video_key), ''), '') <> ''
ORDER BY id DESC
LIMIT 1
`).Scan(&job).Error
	if err != nil {
		return models.VideoJob{}, err
	}
	if job.ID == 0 {
		return models.VideoJob{}, fmt.Errorf("no done png base job found; set BASE_JOB_ID")
	}
	return job, nil
}

func mapFromAny(raw interface{}) map[string]interface{} {
	if raw == nil {
		return map[string]interface{}{}
	}
	if typed, ok := raw.(map[string]interface{}); ok {
		if typed == nil {
			return map[string]interface{}{}
		}
		return typed
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(body, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func round(v float64, n int) float64 {
	if n <= 0 {
		return float64(int(v + 0.5))
	}
	pow := 1.0
	for i := 0; i < n; i++ {
		pow *= 10
	}
	return float64(int(v*pow+0.5)) / pow
}
