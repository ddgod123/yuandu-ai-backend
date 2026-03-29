package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
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

type runSummary struct {
	FileName                  string  `json:"file_name"`
	SourceVideoKey            string  `json:"source_video_key"`
	JobID                     uint64  `json:"job_id"`
	Status                    string  `json:"status"`
	Stage                     string  `json:"stage"`
	Progress                  int     `json:"progress"`
	ProcessingCallSec         float64 `json:"processing_call_sec"`
	PipelineElapsedSec        float64 `json:"pipeline_elapsed_sec"`
	OutputFrameCount          int64   `json:"output_frame_count"`
	OutputAvgWidth            int64   `json:"output_avg_width"`
	OutputAvgHeight           int64   `json:"output_avg_height"`
	PackageZipSizeBytes       int64   `json:"package_zip_size_bytes"`
	SuperResMode              string  `json:"superres_mode"`
	SuperResAttempted         int64   `json:"superres_attempted"`
	SuperResSucceeded         int64   `json:"superres_succeeded"`
	SuperResReplaced          int64   `json:"superres_replaced"`
	SuperResFailed            int64   `json:"superres_failed"`
	SuperResTotalCostCNY      float64 `json:"superres_total_cost_cny"`
	WorkerSuperResUsageCalls  int64   `json:"worker_superres_usage_calls"`
	WorkerSuperResUsageCostUS float64 `json:"worker_superres_usage_cost_usd"`
	EstimatedCostCNY          float64 `json:"estimated_cost_cny"`
	AICostCNY                 float64 `json:"ai_cost_cny"`
}

type batchReport struct {
	RunTag         string       `json:"run_tag"`
	VideoDir       string       `json:"video_dir"`
	Mode           string       `json:"superres_mode"`
	Scene          string       `json:"scene"`
	FileCount      int          `json:"file_count"`
	Runs           []runSummary `json:"runs"`
	TotalEstimated float64      `json:"total_estimated_cost_cny"`
	TotalAICost    float64      `json:"total_ai_cost_cny"`
	TotalSuperCost float64      `json:"total_superres_cost_cny"`
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

	videoDir := strings.TrimSpace(os.Getenv("PNG_BATCH_VIDEO_DIR"))
	if videoDir == "" {
		videoDir = "/Users/mac/go/src/emoji/视频测试/第一轮"
	}
	limit := envInt("PNG_BATCH_LIMIT", 2)
	if limit < 1 {
		limit = 1
	}
	if limit > 10 {
		limit = 10
	}
	only := strings.TrimSpace(os.Getenv("PNG_BATCH_ONLY"))
	scene := strings.TrimSpace(os.Getenv("PNG_BATCH_SCENE"))
	if scene == "" {
		scene = "xiaohongshu"
	}
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("PNG_ALIYUN_SUPERRES_MODE")))
	if mode == "" {
		mode = "off"
	}

	files, err := pickVideoFiles(videoDir, only)
	if err != nil {
		panic(err)
	}
	if len(files) == 0 {
		panic("no video files found")
	}
	if len(files) > limit {
		files = files[:limit]
	}

	runTag := time.Now().Format("20060130-150405")
	uploader := qiniustorage.NewFormUploader(qiniuClient.Cfg)
	processor := videojobs.NewProcessor(database, qiniuClient, cfg)

	report := batchReport{
		RunTag:    runTag,
		VideoDir:  videoDir,
		Mode:      mode,
		Scene:     scene,
		FileCount: len(files),
		Runs:      make([]runSummary, 0, len(files)),
	}

	for idx, filePath := range files {
		fileName := filepath.Base(filePath)
		sourceKey := buildSourceKey(runTag, idx+1, fileName)
		if err := uploadLocalFileToQiniu(uploader, qiniuClient, sourceKey, filePath); err != nil {
			panic(fmt.Errorf("upload failed for %s: %w", fileName, err))
		}

		job, err := createPNGJob(database, sourceKey, fileName, scene)
		if err != nil {
			panic(fmt.Errorf("create job failed for %s: %w", fileName, err))
		}

		task, err := videojobs.NewProcessVideoJobTask(job.ID)
		if err != nil {
			panic(err)
		}
		callStarted := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		err = processor.HandleProcessVideoJob(ctx, asynq.NewTask(videojobs.TaskTypeProcessVideoJob, task.Payload()))
		cancel()
		if err != nil {
			panic(fmt.Errorf("process job failed file=%s job=%d: %w", fileName, job.ID, err))
		}
		s := collectPNGRunSummary(database, fileName, sourceKey, job.ID, time.Since(callStarted).Seconds())
		report.Runs = append(report.Runs, s)
		report.TotalEstimated += s.EstimatedCostCNY
		report.TotalAICost += s.AICostCNY
		report.TotalSuperCost += s.SuperResTotalCostCNY
	}

	report.TotalEstimated = round(report.TotalEstimated, 6)
	report.TotalAICost = round(report.TotalAICost, 6)
	report.TotalSuperCost = round(report.TotalSuperCost, 6)

	body, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(body))
}

func pickVideoFiles(dir, only string) ([]string, error) {
	patterns := []string{
		filepath.Join(dir, "*.MP4"),
		filepath.Join(dir, "*.mp4"),
		filepath.Join(dir, "*.Mov"),
		filepath.Join(dir, "*.MOV"),
	}
	uniq := make(map[string]struct{}, 16)
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			name := strings.ToLower(filepath.Base(m))
			if only != "" && !strings.Contains(name, strings.ToLower(only)) {
				continue
			}
			uniq[m] = struct{}{}
		}
	}
	out := make([]string, 0, len(uniq))
	for file := range uniq {
		out = append(out, file)
	}
	sort.Strings(out)
	return out, nil
}

func createPNGJob(dbConn *gorm.DB, sourceKey, fileName, scene string) (models.VideoJob, error) {
	title := fmt.Sprintf("PNG批量实测-%s", trimTitle(fileName))
	optionsMap := map[string]interface{}{
		"auto_highlight":     true,
		"flow_mode":          "direct",
		"requested_format":   "png",
		"frame_interval_sec": 0.8,
		"max_static":         8,
		"estimate_points":    24,
		"ai1_advanced_options_v1": map[string]interface{}{
			"scene":          scene,
			"visual_focus":   []string{},
			"enable_matting": false,
		},
	}
	optionsJSON, _ := json.Marshal(optionsMap)
	job := models.VideoJob{
		UserID:         1,
		Title:          title,
		SourceVideoKey: sourceKey,
		OutputFormats:  "png",
		Status:         models.VideoJobStatusQueued,
		Stage:          models.VideoJobStageQueued,
		Progress:       0,
		Priority:       "normal",
		Options:        datatypes.JSON(optionsJSON),
		Metrics:        datatypes.JSON([]byte("{}")),
	}
	err := dbConn.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := videojobs.UpsertPublicVideoImageJob(tx, job); err != nil {
			return err
		}
		event := models.VideoJobEvent{
			JobID:    job.ID,
			Stage:    models.VideoJobStageQueued,
			Level:    "info",
			Message:  "png superres batch job queued",
			Metadata: datatypes.JSON([]byte("{}")),
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		if err := videojobs.CreatePublicVideoImageEvent(tx, event); err != nil {
			return err
		}
		return nil
	})
	return job, err
}

func collectPNGRunSummary(dbConn *gorm.DB, fileName, sourceKey string, jobID uint64, callElapsed float64) runSummary {
	s := runSummary{FileName: fileName, SourceVideoKey: sourceKey, JobID: jobID, ProcessingCallSec: round(callElapsed, 3)}

	var row struct {
		Status     string
		Stage      string
		Progress   int
		ElapsedSec float64
		Metrics    datatypes.JSON
	}
	_ = dbConn.Raw(`
SELECT status, stage, progress,
  CASE WHEN started_at IS NOT NULL AND finished_at IS NOT NULL
       THEN EXTRACT(EPOCH FROM (finished_at - started_at))
       ELSE 0 END AS elapsed_sec,
  metrics
FROM archive.video_jobs WHERE id = ?`, jobID).Scan(&row).Error
	s.Status = row.Status
	s.Stage = row.Stage
	s.Progress = row.Progress
	s.PipelineElapsedSec = round(row.ElapsedSec, 3)

	metrics := map[string]interface{}{}
	_ = json.Unmarshal(row.Metrics, &metrics)
	s.PackageZipSizeBytes = int64FromAny(mapFromAny(metrics)["package_zip_size_bytes"])
	super := mapFromAny(metrics["png_worker_super_resolution_v1"])
	s.SuperResMode = strings.TrimSpace(stringFromAny(super["mode"]))
	s.SuperResAttempted = int64FromAny(super["attempted"])
	s.SuperResSucceeded = int64FromAny(super["succeeded"])
	s.SuperResReplaced = int64FromAny(super["replaced"])
	s.SuperResFailed = int64FromAny(super["failed"])
	s.SuperResTotalCostCNY = round(floatFromAny(super["total_cost_cny"]), 6)

	var outAgg struct {
		Cnt int64
		W   float64
		H   float64
	}
	_ = dbConn.Raw(`
SELECT COUNT(*) AS cnt,
       COALESCE(AVG(width),0) AS w,
       COALESCE(AVG(height),0) AS h
FROM archive.video_job_artifacts
WHERE job_id = ? AND type='frame'`, jobID).Scan(&outAgg).Error
	s.OutputFrameCount = outAgg.Cnt
	s.OutputAvgWidth = int64(outAgg.W + 0.5)
	s.OutputAvgHeight = int64(outAgg.H + 0.5)

	var usage struct {
		Calls   int64
		CostUSD float64
	}
	_ = dbConn.Raw(`
SELECT COUNT(*) AS calls, COALESCE(SUM(cost_usd),0) AS cost_usd
FROM ops.video_job_ai_usage
WHERE job_id = ? AND stage='worker_super_resolution'`, jobID).Scan(&usage).Error
	s.WorkerSuperResUsageCalls = usage.Calls
	s.WorkerSuperResUsageCostUS = round(usage.CostUSD, 8)

	var cost struct {
		Estimated float64
		AICostCNY float64
	}
	_ = dbConn.Raw(`
SELECT estimated_cost AS estimated,
       COALESCE((details->>'ai_cost_cny')::numeric,0) AS ai_cost_cny
FROM ops.video_job_costs
WHERE job_id = ?`, jobID).Scan(&cost).Error
	s.EstimatedCostCNY = round(cost.Estimated, 6)
	s.AICostCNY = round(cost.AICostCNY, 6)
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

func buildSourceKey(runTag string, idx int, fileName string) string {
	base := sanitizeKeyPart(fileName)
	return path.Join("emoji", "video-test", "png-superres", runTag, fmt.Sprintf("%02d_%s", idx, base))
}

func sanitizeKeyPart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "video.mp4"
	}
	r := strings.NewReplacer(" ", "_", "/", "_", "\\", "_", ":", "_", "?", "_", "&", "_")
	return r.Replace(s)
}

func trimTitle(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

func envInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
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

func stringFromAny(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func int64FromAny(raw interface{}) int64 {
	switch v := raw.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		iv, _ := v.Int64()
		return iv
	case string:
		iv, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return iv
	default:
		return 0
	}
}

func floatFromAny(raw interface{}) float64 {
	switch v := raw.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		fv, _ := v.Float64()
		return fv
	case string:
		fv, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return fv
	default:
		return 0
	}
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
