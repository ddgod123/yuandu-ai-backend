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

type runSummary struct {
	FileName          string  `json:"file_name"`
	SourceVideoKey    string  `json:"source_video_key"`
	PNGMode           string  `json:"png_mode,omitempty"`
	FastExtractFPS    int     `json:"fast_extract_fps,omitempty"`
	RequestedModel    string  `json:"requested_model,omitempty"`
	ResolvedProvider  string  `json:"resolved_provider,omitempty"`
	ResolvedModel     string  `json:"resolved_model,omitempty"`
	AI1Confidence     float64 `json:"ai1_confidence,omitempty"`
	JobID             uint64  `json:"job_id"`
	Status            string  `json:"status"`
	Stage             string  `json:"stage"`
	Progress          int     `json:"progress"`
	PipelineElapsed   float64 `json:"pipeline_elapsed_sec"`
	ProcessingCallSec float64 `json:"processing_call_sec"`
	PNGOutputsTotal   int64   `json:"png_outputs_total"`
	FaceStatus        string  `json:"face_enhance_status"`
	SuperResStatus    string  `json:"superres_status"`
	Error             string  `json:"error,omitempty"`
}

type modelAggregate struct {
	RequestedModel   string  `json:"requested_model"`
	JobsTotal        int     `json:"jobs_total"`
	DoneCount        int     `json:"done_count"`
	PendingCount     int     `json:"pending_count"`
	FailedCount      int     `json:"failed_count"`
	AvgElapsedSec    float64 `json:"avg_elapsed_sec"`
	AvgOutputsTotal  float64 `json:"avg_outputs_total"`
	AvgAI1Confidence float64 `json:"avg_ai1_confidence,omitempty"`
}

type batchReport struct {
	RunTag           string           `json:"run_tag"`
	VideoDir         string           `json:"video_dir"`
	VideoCount       int              `json:"video_count"`
	ModelPreferences []string         `json:"model_preferences"`
	Count            int              `json:"count"`
	BaseJobID        uint64           `json:"base_job_id"`
	Runs             []runSummary     `json:"runs"`
	ModelAggregate   []modelAggregate `json:"model_aggregate"`
	Aggregate        struct {
		DoneCount     int     `json:"done_count"`
		PendingCount  int     `json:"pending_count"`
		FailedCount   int     `json:"failed_count"`
		AvgElapsedSec float64 `json:"avg_elapsed_sec"`
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

	videoDir := strings.TrimSpace(os.Getenv("LOCAL_VIDEO_DIR"))
	if videoDir == "" {
		videoDir = "/Users/mac/go/src/emoji/归档文档/视频测试/第一轮"
	}
	onlyPattern := strings.TrimSpace(os.Getenv("LOCAL_VIDEO_ONLY"))
	count := envInt("LOCAL_VIDEO_COUNT", 3)
	if count <= 0 {
		count = 3
	}
	timeoutMin := envInt("LOCAL_VIDEO_TIMEOUT_MIN", 45)
	if timeoutMin <= 0 {
		timeoutMin = 45
	}
	autoConfirmAI1 := envBool("LOCAL_VIDEO_AUTO_CONFIRM_AI1", true)
	pngMode := videojobs.NormalizePNGMode(strings.TrimSpace(os.Getenv("LOCAL_VIDEO_PNG_MODE")))
	fastExtractFPS := videojobs.NormalizePNGFastExtractFPS(envInt("LOCAL_VIDEO_FAST_EXTRACT_FPS", videojobs.PNGFastExtractFPS1))
	if pngMode == videojobs.PNGModeFastExtract {
		autoConfirmAI1 = false
	}
	modelPreferences := parseModelPreferences(strings.TrimSpace(os.Getenv("LOCAL_VIDEO_AI_MODELS")))
	if len(modelPreferences) == 0 {
		modelPreferences = []string{"auto"}
	}

	files, err := pickVideoFiles(videoDir, onlyPattern, count)
	if err != nil {
		panic(err)
	}
	if len(files) == 0 {
		panic("no video files matched")
	}

	baseJob, err := pickBasePNGJob(database)
	if err != nil {
		panic(err)
	}
	runTag := time.Now().Format("20060102-150405")
	uploader := qiniustorage.NewFormUploader(qiniuClient.Cfg)
	processor := videojobs.NewProcessor(database, qiniuClient, cfg)

	out := batchReport{
		RunTag:           runTag,
		VideoDir:         videoDir,
		VideoCount:       len(files),
		ModelPreferences: modelPreferences,
		Count:            len(files) * len(modelPreferences),
		BaseJobID:        baseJob.ID,
		Runs:             make([]runSummary, 0, len(files)*len(modelPreferences)),
	}

	for idx, filePath := range files {
		fileName := filepath.Base(filePath)

		for _, requestedModel := range modelPreferences {
			modelPreference := normalizeModelPreference(requestedModel)
			sourceKey := buildSourceKey(runTag, idx+1, fileName, modelPreference)
			if err := uploadLocalFileToQiniu(uploader, qiniuClient, sourceKey, filePath); err != nil {
				out.Runs = append(out.Runs, runSummary{
					FileName:       fileName,
					SourceVideoKey: sourceKey,
					RequestedModel: modelPreference,
					Error:          err.Error(),
				})
				fmt.Printf("[ERROR] upload failed: file=%s model=%s err=%v\n", fileName, requestedModel, err)
				continue
			}

			optionsMap := map[string]interface{}{
				"auto_highlight":   true,
				"flow_mode":        "ai1_confirm",
				"benchmark_tag":    "png_local_batch",
				"local_video_file": fileName,
			}
			optionsMap["png_mode"] = pngMode
			if pngMode == videojobs.PNGModeFastExtract {
				optionsMap["flow_mode"] = "direct"
				optionsMap["fast_extract_fps"] = fastExtractFPS
				optionsMap["frame_interval_sec"] = round(1.0/float64(fastExtractFPS), 3)
			}
			if model := modelPreference; model != "" && model != "auto" {
				optionsMap["ai_model_preference"] = model
			}
			optionsJSON, _ := json.Marshal(optionsMap)

			newJob := models.VideoJob{
				UserID:         baseJob.UserID,
				Title:          fmt.Sprintf("PNG-LOCAL-%02d-%s-%s", idx+1, trimTitle(fileName), formatModelLabelForTitle(requestedModel)),
				SourceVideoKey: sourceKey,
				CategoryID:     baseJob.CategoryID,
				OutputFormats:  "png",
				Status:         models.VideoJobStatusQueued,
				Stage:          models.VideoJobStageQueued,
				Progress:       0,
				Priority:       baseJob.Priority,
				Options:        datatypes.JSON(optionsJSON),
				Metrics:        datatypes.JSON([]byte("{}")),
			}

			jobID, err := createQueuedJob(database, newJob)
			if err != nil {
				out.Runs = append(out.Runs, runSummary{
					FileName:       fileName,
					SourceVideoKey: sourceKey,
					RequestedModel: modelPreference,
					Error:          err.Error(),
				})
				fmt.Printf("[ERROR] create job failed: %s model=%s: %v\n", fileName, requestedModel, err)
				continue
			}
			newJob.ID = jobID

			task, err := videojobs.NewProcessVideoJobTask(newJob.ID)
			if err != nil {
				out.Runs = append(out.Runs, runSummary{
					FileName:       fileName,
					SourceVideoKey: sourceKey,
					RequestedModel: modelPreference,
					JobID:          newJob.ID,
					Error:          err.Error(),
				})
				fmt.Printf("[ERROR] create task failed: job=%d file=%s model=%s err=%v\n", newJob.ID, fileName, requestedModel, err)
				continue
			}

			callStart := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMin)*time.Minute)
			processErr := processor.HandleProcessVideoJob(ctx, asynq.NewTask(videojobs.TaskTypeProcessVideoJob, task.Payload()))
			cancel()
			callElapsed := time.Since(callStart).Seconds()

			summary := collectJobSummary(database, newJob.ID, fileName, sourceKey, modelPreference, callElapsed)

			if autoConfirmAI1 && shouldAutoConfirmAI1(summary) {
				resumeElapsed, resumeErr := autoConfirmAI1AndReprocess(database, processor, newJob.ID, baseJob.UserID, timeoutMin)
				callElapsed += resumeElapsed
				summary = collectJobSummary(database, newJob.ID, fileName, sourceKey, modelPreference, callElapsed)
				if resumeErr != nil {
					if summary.Error == "" {
						summary.Error = "auto_confirm_resume_failed: " + resumeErr.Error()
					} else {
						summary.Error = strings.TrimSpace(summary.Error + "; auto_confirm_resume_failed: " + resumeErr.Error())
					}
					fmt.Printf("[WARN] auto-confirm resume failed: job=%d model=%s err=%v\n", newJob.ID, requestedModel, resumeErr)
				} else {
					fmt.Printf("[INFO] auto-confirmed ai1 and resumed: job=%d model=%s\n", newJob.ID, requestedModel)
				}
			}

			if processErr != nil {
				summary.Error = processErr.Error()
				fmt.Printf("[WARN] process job=%d file=%s model=%s err=%v\n", newJob.ID, fileName, requestedModel, processErr)
			}
			out.Runs = append(out.Runs, summary)
			fmt.Printf("[DONE] %s [%s] -> job=%d status=%s stage=%s elapsed=%.3fs outputs=%d mode=%s fps=%d resolved=%s/%s\n",
				fileName,
				summary.RequestedModel,
				summary.JobID,
				summary.Status,
				summary.Stage,
				summary.PipelineElapsed,
				summary.PNGOutputsTotal,
				summary.PNGMode,
				summary.FastExtractFPS,
				summary.ResolvedProvider,
				summary.ResolvedModel,
			)
		}
	}

	finalizeAggregate(&out)
	payload, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(payload))
}

func createQueuedJob(dbConn *gorm.DB, job models.VideoJob) (uint64, error) {
	err := dbConn.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := videojobs.UpsertPublicVideoImageJob(tx, job); err != nil {
			return err
		}
		queuedEvent := models.VideoJobEvent{
			JobID:    job.ID,
			Stage:    models.VideoJobStageQueued,
			Level:    "info",
			Message:  "png local batch job queued",
			Metadata: datatypes.JSON([]byte("{}")),
		}
		if err := tx.Create(&queuedEvent).Error; err != nil {
			return err
		}
		if err := videojobs.CreatePublicVideoImageEvent(tx, queuedEvent); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return job.ID, nil
}

func collectJobSummary(dbConn *gorm.DB, jobID uint64, fileName, sourceKey, requestedModel string, callElapsed float64) runSummary {
	s := runSummary{
		FileName:          fileName,
		SourceVideoKey:    sourceKey,
		RequestedModel:    requestedModel,
		JobID:             jobID,
		ProcessingCallSec: round(callElapsed, 3),
	}

	var row struct {
		Status     string
		Stage      string
		Progress   int
		ElapsedSec float64
		Options    datatypes.JSON
		Metrics    datatypes.JSON
	}
	_ = dbConn.Raw(`
SELECT status, stage, progress,
  CASE WHEN started_at IS NOT NULL AND finished_at IS NOT NULL
       THEN EXTRACT(EPOCH FROM (finished_at - started_at))
       ELSE 0 END AS elapsed_sec,
  options,
  metrics
FROM archive.video_jobs WHERE id = ?
`, jobID).Scan(&row).Error

	var outputs struct {
		Total int64
	}
	_ = dbConn.Raw(`SELECT COUNT(*) AS total FROM public.video_image_outputs_png WHERE job_id = ? AND format = 'png' AND file_role = 'main'`, jobID).Scan(&outputs).Error

	optionsMap := map[string]interface{}{}
	_ = json.Unmarshal(row.Options, &optionsMap)
	metricsMap := map[string]interface{}{}
	_ = json.Unmarshal(row.Metrics, &metricsMap)

	if s.RequestedModel == "" {
		s.RequestedModel = strings.TrimSpace(stringFromAny(optionsMap["ai_model_preference"]))
	}
	if s.RequestedModel == "" {
		s.RequestedModel = "auto"
	}
	s.PNGMode = strings.TrimSpace(stringFromAny(optionsMap["png_mode"]))
	if s.PNGMode == "" {
		s.PNGMode = videojobs.PNGModeSmartLLM
	}
	s.FastExtractFPS = intFromAny(optionsMap["fast_extract_fps"])
	if s.PNGMode == videojobs.PNGModeFastExtract && s.FastExtractFPS <= 0 {
		s.FastExtractFPS = videojobs.ResolvePNGFastExtractFPS(optionsMap)
	}

	ai1Metric := mapFromAny(metricsMap["png_ai1_plan_v1"])
	directorSnapshot := mapFromAny(ai1Metric["director_snapshot"])
	s.ResolvedProvider = strings.TrimSpace(stringFromAny(directorSnapshot["provider"]))
	s.ResolvedModel = strings.TrimSpace(stringFromAny(directorSnapshot["model"]))
	ai1EventMeta := mapFromAny(ai1Metric["event_meta"])
	s.AI1Confidence = round(floatFromAny(ai1EventMeta["ai1_confidence"]), 4)

	face := mapFromAny(metricsMap["png_worker_face_enhancement_v1"])
	super := mapFromAny(metricsMap["png_worker_super_resolution_v1"])

	s.Status = row.Status
	s.Stage = row.Stage
	s.Progress = row.Progress
	s.PipelineElapsed = round(row.ElapsedSec, 3)
	s.PNGOutputsTotal = outputs.Total
	s.FaceStatus = strings.TrimSpace(stringFromAny(face["status"]))
	s.SuperResStatus = strings.TrimSpace(stringFromAny(super["status"]))
	return s
}

func finalizeAggregate(out *batchReport) {
	if out == nil || len(out.Runs) == 0 {
		return
	}
	totalElapsed := 0.0
	doneCount := 0
	pending := 0
	failed := 0
	perModel := map[string]*modelAggregate{}
	for _, row := range out.Runs {
		modelKey := normalizeModelPreference(row.RequestedModel)
		if modelKey == "" {
			modelKey = "auto"
		}
		agg, ok := perModel[modelKey]
		if !ok {
			agg = &modelAggregate{RequestedModel: modelKey}
			perModel[modelKey] = agg
		}
		agg.JobsTotal++

		if strings.EqualFold(strings.TrimSpace(row.Status), models.VideoJobStatusDone) {
			doneCount++
			totalElapsed += row.PipelineElapsed
			agg.DoneCount++
			agg.AvgElapsedSec += row.PipelineElapsed
			agg.AvgOutputsTotal += float64(row.PNGOutputsTotal)
			if row.AI1Confidence > 0 {
				agg.AvgAI1Confidence += row.AI1Confidence
			}
			continue
		}
		if strings.EqualFold(strings.TrimSpace(row.Status), models.VideoJobStatusFailed) ||
			strings.EqualFold(strings.TrimSpace(row.Status), models.VideoJobStatusCancelled) {
			failed++
			agg.FailedCount++
			continue
		}
		pending++
		agg.PendingCount++
	}
	out.Aggregate.DoneCount = doneCount
	out.Aggregate.PendingCount = pending
	out.Aggregate.FailedCount = failed
	if doneCount > 0 {
		out.Aggregate.AvgElapsedSec = round(totalElapsed/float64(doneCount), 3)
	}

	modelKeys := make([]string, 0, len(perModel))
	for key := range perModel {
		modelKeys = append(modelKeys, key)
	}
	sort.Strings(modelKeys)
	out.ModelAggregate = make([]modelAggregate, 0, len(modelKeys))
	for _, key := range modelKeys {
		agg := perModel[key]
		if agg.DoneCount > 0 {
			agg.AvgElapsedSec = round(agg.AvgElapsedSec/float64(agg.DoneCount), 3)
			agg.AvgOutputsTotal = round(agg.AvgOutputsTotal/float64(agg.DoneCount), 3)
			agg.AvgAI1Confidence = round(agg.AvgAI1Confidence/float64(agg.DoneCount), 4)
		}
		out.ModelAggregate = append(out.ModelAggregate, *agg)
	}
}

func shouldAutoConfirmAI1(summary runSummary) bool {
	status := strings.ToLower(strings.TrimSpace(summary.Status))
	stage := strings.ToLower(strings.TrimSpace(summary.Stage))
	return status == models.VideoJobStatusQueued && stage == models.VideoJobStageAwaitingAI1
}

func autoConfirmAI1AndReprocess(
	dbConn *gorm.DB,
	processor *videojobs.Processor,
	jobID uint64,
	userID uint64,
	timeoutMin int,
) (float64, error) {
	if dbConn == nil || processor == nil || jobID == 0 {
		return 0, nil
	}
	var job models.VideoJob
	if err := dbConn.Where("id = ?", jobID).First(&job).Error; err != nil {
		return 0, err
	}
	options := map[string]interface{}{}
	_ = json.Unmarshal(job.Options, &options)
	flowMode := strings.ToLower(strings.TrimSpace(stringFromAny(options["flow_mode"])))
	if flowMode != "ai1_confirm" {
		return 0, nil
	}
	confirmedAt := time.Now()
	options["ai1_pending"] = false
	options["ai1_confirmed"] = true
	options["ai1_confirmed_at"] = confirmedAt.Format(time.RFC3339)
	resumeQueue, resumeTaskType, _ := videojobs.ResolveVideoJobExecutionTarget(job.OutputFormats)
	options["execution_queue"] = resumeQueue
	options["execution_task_type"] = resumeTaskType

	progress := job.Progress
	if progress < 36 {
		progress = 36
	}
	updates := map[string]interface{}{
		"status":        models.VideoJobStatusQueued,
		"stage":         models.VideoJobStageQueued,
		"progress":      progress,
		"options":       mustJSON(options),
		"error_message": "",
	}
	if err := dbConn.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.VideoJob{}).Where("id = ?", job.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := videojobs.SyncPublicVideoImageJobUpdates(tx, job.ID, updates); err != nil {
			return err
		}
		eventMeta := map[string]interface{}{
			"flow_mode": "ai1_confirm",
			"action":    "batch_auto_confirmed",
		}
		confirmEvent := models.VideoJobEvent{
			JobID:    job.ID,
			Stage:    models.VideoJobStageAnalyzing,
			Level:    "info",
			Message:  "batch auto confirmed continue after ai1",
			Metadata: mustJSON(eventMeta),
		}
		if err := tx.Create(&confirmEvent).Error; err != nil {
			return err
		}
		if err := videojobs.CreatePublicVideoImageEvent(tx, confirmEvent); err != nil {
			return err
		}
		if err := videojobs.ConfirmVideoJobAI1Plan(tx, job.ID, userID, confirmedAt); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return 0, err
	}

	task, err := videojobs.NewProcessVideoJobTask(job.ID)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMin)*time.Minute)
	err = processor.HandleProcessVideoJob(ctx, asynq.NewTask(videojobs.TaskTypeProcessVideoJob, task.Payload()))
	cancel()
	return time.Since(start).Seconds(), err
}

func pickVideoFiles(dir string, only string, count int) ([]string, error) {
	patterns := []string{
		filepath.Join(dir, "*.mp4"),
		filepath.Join(dir, "*.MP4"),
		filepath.Join(dir, "*.mov"),
		filepath.Join(dir, "*.MOV"),
	}
	uniq := make(map[string]struct{}, 32)
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			base := strings.ToLower(filepath.Base(m))
			if strings.Contains(base, "report") {
				continue
			}
			if only != "" && !strings.Contains(base, strings.ToLower(only)) {
				continue
			}
			uniq[m] = struct{}{}
		}
	}
	files := make([]string, 0, len(uniq))
	for m := range uniq {
		files = append(files, m)
	}
	sort.Strings(files)
	if count > 0 && len(files) > count {
		files = files[:count]
	}
	return files, nil
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

func prepareBaseOptions(raw datatypes.JSON) map[string]interface{} {
	optionsMap := map[string]interface{}{}
	_ = json.Unmarshal(raw, &optionsMap)
	delete(optionsMap, "start_sec")
	delete(optionsMap, "end_sec")
	delete(optionsMap, "highlight_selected")
	delete(optionsMap, "highlight_source")
	return optionsMap
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func buildSourceKey(runTag string, idx int, fileName string, model string) string {
	base := sanitizeKeyPart(fileName)
	modelPart := sanitizeKeyPart(formatModelLabelForTitle(model))
	if modelPart == "" {
		modelPart = "auto"
	}
	return path.Join("emoji", "video-test", "png-local", runTag, fmt.Sprintf("%02d_%s_%s", idx, modelPart, base))
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

func stringFromAny(v interface{}) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func mustJSON(raw interface{}) datatypes.JSON {
	body, err := json.Marshal(raw)
	if err != nil || len(body) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(body)
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	var out int
	if _, err := fmt.Sscanf(raw, "%d", &out); err != nil || out <= 0 {
		return fallback
	}
	return out
}

func envBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseModelPreferences(raw string) []string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	for _, token := range strings.Split(text, ",") {
		model := normalizeModelPreference(token)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func normalizeModelPreference(raw string) string {
	text := strings.TrimSpace(strings.ToLower(raw))
	switch text {
	case "":
		return ""
	case "auto", "default":
		return "auto"
	case "omni", "omni_flash", "omni-flash", "vision_omni":
		return "qwen3.5-omni-flash"
	case "omni_plus", "omni-plus":
		return "qwen3.5-omni-plus"
	case "qwen3.5-plus", "qwen3_5_plus", "qwen35_plus", "3.5_plus":
		return "qwen3.5-plus"
	default:
		return text
	}
}

func formatModelLabelForTitle(raw string) string {
	model := normalizeModelPreference(raw)
	if model == "" || model == "auto" {
		return "auto"
	}
	replacer := strings.NewReplacer(".", "", "-", "", "_", "", ":", "", "/", "")
	value := replacer.Replace(strings.ToLower(model))
	if len(value) > 20 {
		value = value[:20]
	}
	if value == "" {
		return "model"
	}
	return value
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
	case int32:
		return float64(v)
	case uint:
		return float64(v)
	case uint64:
		return float64(v)
	case uint32:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0
		}
		var out float64
		if _, err := fmt.Sscanf(text, "%f", &out); err == nil {
			return out
		}
	}
	return 0
}

func intFromAny(raw interface{}) int {
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case uint:
		return int(v)
	case uint64:
		return int(v)
	case uint32:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return int(i)
		}
		f, _ := v.Float64()
		return int(f)
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0
		}
		var out int
		if _, err := fmt.Sscanf(text, "%d", &out); err == nil {
			return out
		}
	}
	return 0
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
