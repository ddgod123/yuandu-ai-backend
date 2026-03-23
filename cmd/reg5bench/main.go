package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/db"
	"emoji/internal/models"
	"emoji/internal/storage"
	"emoji/internal/videojobs"

	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type jobRunSummary struct {
	FileName           string  `json:"file_name"`
	SourceVideoKey     string  `json:"source_video_key"`
	JobID              uint64  `json:"job_id"`
	Status             string  `json:"status"`
	Stage              string  `json:"stage"`
	Progress           int     `json:"progress"`
	TotalElapsedSec    float64 `json:"total_elapsed_sec"`
	ProcessingCallSec  float64 `json:"processing_call_sec"`
	GIFOutputsTotal    int64   `json:"gif_outputs_total"`
	ReviewDeliver      int64   `json:"review_deliver"`
	ReviewKeepInternal int64   `json:"review_keep_internal"`
	ReviewReject       int64   `json:"review_reject"`
	AvgRenderMs        float64 `json:"avg_render_ms"`
	P90RenderMs        float64 `json:"p90_render_ms"`
	TotalCostCNY       float64 `json:"total_cost_cny"`
	AICostCNY          float64 `json:"ai_cost_cny"`
	CPUMs              int64   `json:"cpu_ms"`
}

type report struct {
	BaseJobID uint64          `json:"base_job_id"`
	VideoDir  string          `json:"video_dir"`
	Count     int             `json:"count"`
	Runs      []jobRunSummary `json:"runs"`
	Aggregate struct {
		DoneCount         int     `json:"done_count"`
		AvgElapsedSec     float64 `json:"avg_elapsed_sec"`
		P90ElapsedSec     float64 `json:"p90_elapsed_sec"`
		AvgRenderMs       float64 `json:"avg_render_ms"`
		AvgCostCNY        float64 `json:"avg_cost_cny"`
		TotalCostCNY      float64 `json:"total_cost_cny"`
		TotalGIFOutputs   int64   `json:"total_gif_outputs"`
		TotalDeliver      int64   `json:"total_deliver"`
		TotalKeepInternal int64   `json:"total_keep_internal"`
		TotalReject       int64   `json:"total_reject"`
	} `json:"aggregate"`
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

	videoDir := strings.TrimSpace(os.Getenv("REG5_VIDEO_DIR"))
	if videoDir == "" {
		videoDir = "/Users/mac/go/src/emoji/视频测试/第一轮"
	}
	onlyPattern := strings.TrimSpace(os.Getenv("REG5_ONLY"))
	files, err := pickREG5Videos(videoDir, onlyPattern)
	if err != nil {
		panic(err)
	}
	if len(files) == 0 {
		panic("no REG5 files matched")
	}
	if onlyPattern == "" && len(files) != 5 {
		panic(fmt.Errorf("expect 5 REG5 files, got %d", len(files)))
	}

	var base models.VideoJob
	if err := database.Where("id = ?", baseID).First(&base).Error; err != nil {
		panic(err)
	}
	baseOptions := prepareBaseOptions(base.Options, baseID)

	processor := videojobs.NewProcessor(database, qiniuClient, cfg)
	uploader := qiniustorage.NewFormUploader(qiniuClient.Cfg)
	runTag := time.Now().Format("20060121-150405")

	out := report{
		BaseJobID: baseID,
		VideoDir:  videoDir,
		Count:     len(files),
		Runs:      make([]jobRunSummary, 0, len(files)),
	}

	for idx, filePath := range files {
		fileName := filepath.Base(filePath)
		sourceKey := buildSourceKey(runTag, idx+1, fileName)
		if err := uploadLocalFileToQiniu(uploader, qiniuClient, sourceKey, filePath); err != nil {
			panic(fmt.Errorf("upload failed: %s: %w", fileName, err))
		}

		optionsMap := cloneMap(baseOptions)
		optionsMap["auto_highlight"] = true
		optionsMap["benchmark_tag"] = "reg5_gate"
		optionsMap["benchmark_source_job_id"] = baseID
		optionsMap["reg5_file_name"] = fileName
		optionsMap["reg5_run_tag"] = runTag
		optionsJSON, _ := json.Marshal(optionsMap)

		newJob := models.VideoJob{
			UserID:         base.UserID,
			Title:          fmt.Sprintf("REG5-%02d-%s", idx+1, trimTitle(fileName)),
			SourceVideoKey: sourceKey,
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
				Message:  "reg5 benchmark job queued",
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

		task, err := videojobs.NewProcessVideoJobTask(newJob.ID)
		if err != nil {
			panic(err)
		}
		callStart := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
		err = processor.HandleProcessVideoJob(ctx, asynq.NewTask(videojobs.TaskTypeProcessVideoJob, task.Payload()))
		cancel()
		if err != nil {
			fmt.Printf("[WARN] process job=%d file=%s err=%v\n", newJob.ID, fileName, err)
		}
		callElapsed := time.Since(callStart).Seconds()

		s := collectJobSummary(database, newJob.ID, fileName, sourceKey, callElapsed)
		out.Runs = append(out.Runs, s)
		fmt.Printf("[DONE] %s -> job=%d status=%s elapsed=%.3fs outputs=%d\n", fileName, s.JobID, s.Status, s.TotalElapsedSec, s.GIFOutputsTotal)
	}

	finalizeAggregate(&out)
	payload, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(payload))
}

func pickREG5Videos(dir string, only string) ([]string, error) {
	patterns := []string{
		filepath.Join(dir, "REG5_*.MP4"),
		filepath.Join(dir, "REG5_*.mp4"),
		filepath.Join(dir, "REG5_*.Mov"),
		filepath.Join(dir, "REG5_*.MOV"),
	}
	uniq := make(map[string]struct{}, 8)
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			uniq[m] = struct{}{}
		}
	}
	files := make([]string, 0, len(uniq))
	for m := range uniq {
		if only != "" && !strings.Contains(strings.ToLower(filepath.Base(m)), strings.ToLower(only)) {
			continue
		}
		files = append(files, m)
	}
	sort.Strings(files)
	return files, nil
}

func prepareBaseOptions(raw datatypes.JSON, baseID uint64) map[string]interface{} {
	optionsMap := map[string]interface{}{}
	_ = json.Unmarshal(raw, &optionsMap)
	delete(optionsMap, "start_sec")
	delete(optionsMap, "end_sec")
	delete(optionsMap, "highlight_selected")
	delete(optionsMap, "highlight_source")
	optionsMap["auto_highlight"] = true
	optionsMap["benchmark_source_job_id"] = baseID
	return optionsMap
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func buildSourceKey(runTag string, idx int, fileName string) string {
	base := sanitizeKeyPart(fileName)
	return path.Join("emoji", "video-test", "reg5", runTag, fmt.Sprintf("%02d_%s", idx, base))
}

func sanitizeKeyPart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "video.mp4"
	}
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, "?", "_")
	s = strings.ReplaceAll(s, "&", "_")
	return s
}

func trimTitle(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func uploadLocalFileToQiniu(
	uploader *qiniustorage.FormUploader,
	q *storage.QiniuClient,
	key string,
	filePath string,
) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	putPolicy := qiniustorage.PutPolicy{Scope: q.Bucket + ":" + key}
	upToken := putPolicy.UploadToken(q.Mac)
	var ret qiniustorage.PutRet
	return uploader.Put(context.Background(), &ret, upToken, key, f, info.Size(), &qiniustorage.PutExtra{})
}

func collectJobSummary(dbConn *gorm.DB, jobID uint64, fileName, sourceKey string, callElapsed float64) jobRunSummary {
	s := jobRunSummary{
		FileName:          fileName,
		SourceVideoKey:    sourceKey,
		JobID:             jobID,
		ProcessingCallSec: round(callElapsed, 3),
	}
	var row struct {
		Status     string
		Stage      string
		Progress   int
		ElapsedSec float64
	}
	_ = dbConn.Raw(`
SELECT status, stage, progress,
  CASE WHEN started_at IS NOT NULL AND finished_at IS NOT NULL
       THEN EXTRACT(EPOCH FROM (finished_at - started_at))
       ELSE 0 END AS elapsed_sec
FROM archive.video_jobs WHERE id = ?`, jobID).Scan(&row).Error

	var stats struct {
		GIFOutputsTotal int64
		ReviewDeliver   int64
		ReviewKeep      int64
		ReviewReject    int64
		AvgRenderMs     float64
		P90RenderMs     float64
	}
	_ = dbConn.Raw(`
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
    NULLIF(a.metadata->>'render_elapsed_ms','')::numeric AS render_elapsed_ms
  FROM archive.video_job_artifacts a
  WHERE a.job_id = ? AND a.type='clip' AND a.metadata->>'format'='gif'
)
SELECT
  (SELECT COUNT(*) FROM outputs) AS gif_outputs_total,
  (SELECT COUNT(*) FROM reviews WHERE final_recommendation='deliver') AS review_deliver,
  (SELECT COUNT(*) FROM reviews WHERE final_recommendation='keep_internal') AS review_keep,
  (SELECT COUNT(*) FROM reviews WHERE final_recommendation='reject') AS review_reject,
  COALESCE((SELECT AVG(render_elapsed_ms) FROM artifacts),0) AS avg_render_ms,
  COALESCE((SELECT PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY render_elapsed_ms) FROM artifacts),0) AS p90_render_ms
`, jobID, jobID, jobID).Scan(&stats).Error

	var cost struct {
		EstimatedCost float64
		AICostCNY     float64
		CPUMs         int64
	}
	_ = dbConn.Raw(`
SELECT estimated_cost,
       COALESCE((details->>'ai_cost_cny')::numeric, 0) AS ai_cost_cny,
       COALESCE(cpu_ms,0) AS cpu_ms
FROM ops.video_job_costs
WHERE job_id = ?`, jobID).Scan(&cost).Error

	s.Status = row.Status
	s.Stage = row.Stage
	s.Progress = row.Progress
	s.TotalElapsedSec = round(row.ElapsedSec, 3)
	s.GIFOutputsTotal = stats.GIFOutputsTotal
	s.ReviewDeliver = stats.ReviewDeliver
	s.ReviewKeepInternal = stats.ReviewKeep
	s.ReviewReject = stats.ReviewReject
	s.AvgRenderMs = round(stats.AvgRenderMs, 1)
	s.P90RenderMs = round(stats.P90RenderMs, 1)
	s.TotalCostCNY = round(cost.EstimatedCost, 6)
	s.AICostCNY = round(cost.AICostCNY, 6)
	s.CPUMs = cost.CPUMs
	return s
}

func finalizeAggregate(out *report) {
	if out == nil || len(out.Runs) == 0 {
		return
	}
	elapsed := make([]float64, 0, len(out.Runs))
	var sumElapsed float64
	var sumRender float64
	var sumCost float64
	for _, r := range out.Runs {
		if strings.EqualFold(r.Status, "done") {
			out.Aggregate.DoneCount++
		}
		elapsed = append(elapsed, r.TotalElapsedSec)
		sumElapsed += r.TotalElapsedSec
		sumRender += r.AvgRenderMs
		sumCost += r.TotalCostCNY
		out.Aggregate.TotalGIFOutputs += r.GIFOutputsTotal
		out.Aggregate.TotalDeliver += r.ReviewDeliver
		out.Aggregate.TotalKeepInternal += r.ReviewKeepInternal
		out.Aggregate.TotalReject += r.ReviewReject
	}
	sort.Float64s(elapsed)
	out.Aggregate.AvgElapsedSec = round(sumElapsed/float64(len(out.Runs)), 3)
	out.Aggregate.AvgRenderMs = round(sumRender/float64(len(out.Runs)), 1)
	out.Aggregate.TotalCostCNY = round(sumCost, 6)
	out.Aggregate.AvgCostCNY = round(sumCost/float64(len(out.Runs)), 6)
	out.Aggregate.P90ElapsedSec = percentile(elapsed, 0.9)
}

func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[n-1]
	}
	pos := p * float64(n-1)
	lo := int(pos)
	hi := lo + 1
	if hi >= n {
		return round(sorted[n-1], 3)
	}
	frac := pos - float64(lo)
	return round(sorted[lo]+(sorted[hi]-sorted[lo])*frac, 3)
}

func round(v float64, n int) float64 {
	if n <= 0 {
		return float64(int64(v + 0.5))
	}
	pow := 1.0
	for i := 0; i < n; i++ {
		pow *= 10
	}
	if v >= 0 {
		return float64(int64(v*pow+0.5)) / pow
	}
	return float64(int64(v*pow-0.5)) / pow
}
