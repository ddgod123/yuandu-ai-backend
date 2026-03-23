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

type summary struct {
	BaseJobID           uint64                 `json:"base_job_id"`
	NewJobID            uint64                 `json:"new_job_id"`
	Status              string                 `json:"status"`
	Stage               string                 `json:"stage"`
	Progress            int                    `json:"progress"`
	TotalElapsedSec     float64                `json:"total_elapsed_sec"`
	ProcessingCallSec   float64                `json:"processing_call_sec"`
	GIFOutputsTotal     int64                  `json:"gif_outputs_total"`
	ReviewDeliver       int64                  `json:"review_deliver"`
	ReviewKeepInternal  int64                  `json:"review_keep_internal"`
	ReviewReject        int64                  `json:"review_reject"`
	ReviewNeedManual    int64                  `json:"review_need_manual_review"`
	AvgRenderMs         float64                `json:"avg_render_ms"`
	P90RenderMs         float64                `json:"p90_render_ms"`
	AvgPredRenderSec    float64                `json:"avg_predicted_render_sec"`
	AvgActualRenderSec  float64                `json:"avg_actual_render_sec"`
	AvgRenderCostUnits  float64                `json:"avg_render_cost_units"`
	BundleCount         int64                  `json:"bundle_count"`
	BundleHitOutputs    int64                  `json:"bundle_hit_outputs"`
	MezzanineHitOutputs int64                  `json:"mezzanine_hit_outputs"`
	Extra               map[string]interface{} `json:"extra,omitempty"`
}

func main() {
	_ = godotenv.Overload()
	_ = godotenv.Overload(".env")
	_ = godotenv.Overload("backend/.env")

	cfg := config.Load()
	database, err := db.Connect(cfg)
	if err != nil {
		panic(err)
	}
	qiniuClient, err := storage.NewQiniuClient(cfg)
	if err != nil {
		panic(err)
	}

	baseID := uint64(182)
	if raw := strings.TrimSpace(os.Getenv("BASE_JOB_ID")); raw != "" {
		var parsed uint64
		_, _ = fmt.Sscanf(raw, "%d", &parsed)
		if parsed > 0 {
			baseID = parsed
		}
	}

	var base models.VideoJob
	if err := database.Where("id = ?", baseID).First(&base).Error; err != nil {
		panic(err)
	}

	optionsMap := map[string]interface{}{}
	_ = json.Unmarshal(base.Options, &optionsMap)
	delete(optionsMap, "start_sec")
	delete(optionsMap, "end_sec")
	delete(optionsMap, "highlight_selected")
	delete(optionsMap, "highlight_source")
	optionsMap["auto_highlight"] = true
	optionsMap["benchmark_source_job_id"] = baseID
	optionsMap["benchmark_tag"] = "p1_bundle_mezzanine_compare"
	optionsJSON, _ := json.Marshal(optionsMap)

	newJob := models.VideoJob{
		UserID:         base.UserID,
		Title:          fmt.Sprintf("P1对比测试-%d", time.Now().Unix()),
		SourceVideoKey: base.SourceVideoKey,
		CategoryID:     base.CategoryID,
		OutputFormats:  "gif",
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
			Message:  "benchmark job queued",
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
		fmt.Printf("PROCESS_ERROR: %v\n", err)
	}

	var row struct {
		Status     string
		Stage      string
		Progress   int
		ElapsedSec float64
	}
	_ = database.Raw(`
SELECT status, stage, progress,
  CASE WHEN started_at IS NOT NULL AND finished_at IS NOT NULL
       THEN EXTRACT(EPOCH FROM (finished_at - started_at))
       ELSE 0 END AS elapsed_sec
FROM archive.video_jobs WHERE id = ?`, newJob.ID).Scan(&row).Error

	var stats struct {
		GIFOutputsTotal     int64
		ReviewDeliver       int64
		ReviewKeepInternal  int64
		ReviewReject        int64
		ReviewNeedManual    int64
		AvgRenderMs         float64
		P90RenderMs         float64
		AvgPredRenderSec    float64
		AvgActualRenderSec  float64
		AvgRenderCostUnits  float64
		BundleCount         int64
		BundleHitOutputs    int64
		MezzanineHitOutputs int64
	}
	_ = database.Raw(`
WITH outputs AS (
  SELECT id
  FROM public.video_image_outputs
  WHERE job_id = ? AND format = 'gif' AND file_role = 'main'
), reviews AS (
  SELECT final_recommendation
  FROM archive.video_job_gif_ai_reviews
  WHERE job_id = ?
), artifacts AS (
  SELECT
    a.id,
    NULLIF(a.metadata->>'render_elapsed_ms','')::numeric AS render_elapsed_ms,
    NULLIF(a.metadata->>'predicted_render_sec','')::numeric AS predicted_render_sec,
    NULLIF(a.metadata->>'render_actual_sec','')::numeric AS actual_render_sec,
    NULLIF(a.metadata->>'render_cost_units','')::numeric AS render_cost_units,
    NULLIF(a.metadata->>'bundle_id','') AS bundle_id,
    CASE WHEN COALESCE(a.metadata->>'cache_hit','false') IN ('true','1') THEN 1 ELSE 0 END AS cache_hit
  FROM archive.video_job_artifacts a
  WHERE a.job_id = ? AND a.type='clip' AND a.metadata->>'format'='gif'
)
SELECT
  (SELECT COUNT(*) FROM outputs) AS gif_outputs_total,
  (SELECT COUNT(*) FROM reviews WHERE final_recommendation='deliver') AS review_deliver,
  (SELECT COUNT(*) FROM reviews WHERE final_recommendation='keep_internal') AS review_keep_internal,
  (SELECT COUNT(*) FROM reviews WHERE final_recommendation='reject') AS review_reject,
  (SELECT COUNT(*) FROM reviews WHERE final_recommendation='need_manual_review') AS review_need_manual,
  COALESCE((SELECT AVG(render_elapsed_ms) FROM artifacts),0) AS avg_render_ms,
  COALESCE((SELECT PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY render_elapsed_ms) FROM artifacts),0) AS p90_render_ms,
  COALESCE((SELECT AVG(predicted_render_sec) FROM artifacts),0) AS avg_pred_render_sec,
  COALESCE((SELECT AVG(actual_render_sec) FROM artifacts),0) AS avg_actual_render_sec,
  COALESCE((SELECT AVG(render_cost_units) FROM artifacts),0) AS avg_render_cost_units,
  COALESCE((SELECT COUNT(DISTINCT bundle_id) FROM artifacts WHERE bundle_id IS NOT NULL AND bundle_id <> ''),0) AS bundle_count,
  COALESCE((SELECT COUNT(*) FROM artifacts WHERE bundle_id IS NOT NULL AND bundle_id <> ''),0) AS bundle_hit_outputs,
  COALESCE((SELECT SUM(cache_hit) FROM artifacts),0) AS mezzanine_hit_outputs
`, newJob.ID, newJob.ID, newJob.ID).Scan(&stats).Error

	out := summary{
		BaseJobID:           baseID,
		NewJobID:            newJob.ID,
		Status:              row.Status,
		Stage:               row.Stage,
		Progress:            row.Progress,
		TotalElapsedSec:     round(row.ElapsedSec, 3),
		ProcessingCallSec:   round(callElapsed, 3),
		GIFOutputsTotal:     stats.GIFOutputsTotal,
		ReviewDeliver:       stats.ReviewDeliver,
		ReviewKeepInternal:  stats.ReviewKeepInternal,
		ReviewReject:        stats.ReviewReject,
		ReviewNeedManual:    stats.ReviewNeedManual,
		AvgRenderMs:         round(stats.AvgRenderMs, 2),
		P90RenderMs:         round(stats.P90RenderMs, 2),
		AvgPredRenderSec:    round(stats.AvgPredRenderSec, 3),
		AvgActualRenderSec:  round(stats.AvgActualRenderSec, 3),
		AvgRenderCostUnits:  round(stats.AvgRenderCostUnits, 3),
		BundleCount:         stats.BundleCount,
		BundleHitOutputs:    stats.BundleHitOutputs,
		MezzanineHitOutputs: stats.MezzanineHitOutputs,
		Extra: map[string]interface{}{
			"gif_bundle_enabled":    cfg.GIFBundleEnabled,
			"gif_mezzanine_enabled": cfg.GIFMezzanineEnabled,
			"source_video_key":      base.SourceVideoKey,
		},
	}
	payload, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(payload))
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
