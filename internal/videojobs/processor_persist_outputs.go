package videojobs

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"emoji/internal/models"

	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

func (p *Processor) persistJobResults(
	ctx context.Context,
	job models.VideoJob,
	framePaths []string,
	sourcePath string,
	meta videoProbeMeta,
	options jobOptions,
	highlightCandidates []highlightCandidate,
	animatedWindows []highlightCandidate,
	qualitySettings QualitySettings,
) (uint64, int, []string, []string, packageBundleOutcome, error) {
	formats := normalizeOutputFormats(job.OutputFormats)
	stillFormats, animatedFormats := splitVideoOutputFormats(formats)
	if len(stillFormats) == 0 && len(animatedFormats) == 0 {
		stillFormats = []string{"jpg"}
	}
	if len(stillFormats) > 0 && len(framePaths) == 0 {
		return 0, 0, nil, nil, packageBundleOutcome{}, permanentError{err: errors.New("frame paths is empty")}
	}

	prefix, err := p.resolveCollectionPrefix(job)
	if err != nil {
		return 0, 0, nil, nil, packageBundleOutcome{}, err
	}

	uploader := qiniustorage.NewFormUploader(p.qiniu.Cfg)
	uploadedKeys := make([]string, 0, len(framePaths)*(len(stillFormats)+1)+16)
	generatedFormatSet := map[string]struct{}{}

	tx := p.db.Begin()
	if tx.Error != nil {
		return 0, 0, uploadedKeys, nil, packageBundleOutcome{}, tx.Error
	}

	collection := models.Collection{
		Title:       fallbackTitle(job.Title),
		Slug:        ensureUniqueSlug(tx, slugify(fallbackTitle(job.Title))),
		Description: fmt.Sprintf("由视频任务 #%d 自动生成", job.ID),
		OwnerID:     job.UserID,
		CategoryID:  job.CategoryID,
		Source:      "user_video_mvp",
		QiniuPrefix: prefix,
		FileCount:   0,
		Visibility:  "private",
		Status:      "active",
	}
	code, err := ensureUniqueDownloadCode(tx)
	if err != nil {
		_ = tx.Rollback()
		return 0, 0, uploadedKeys, nil, packageBundleOutcome{}, err
	}
	collection.DownloadCode = code

	if err := tx.Create(&collection).Error; err != nil {
		_ = tx.Rollback()
		return 0, 0, uploadedKeys, nil, packageBundleOutcome{}, err
	}

	displayOrder := 1
	staticCount := 0
	coverKey := ""

	if len(stillFormats) > 0 {
		stillCreated, stillCover, err := p.persistStillFrameOutputs(
			tx,
			job.ID,
			collection,
			prefix,
			framePaths,
			stillFormats,
			qualitySettings,
			displayOrder,
			uploader,
			&uploadedKeys,
			generatedFormatSet,
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, 0, uploadedKeys, nil, packageBundleOutcome{}, err
		}
		staticCount += stillCreated
		displayOrder += stillCreated
		if coverKey == "" {
			coverKey = stillCover
		}
	}

	if len(animatedFormats) > 0 {
		windows := animatedWindows
		if len(windows) == 0 {
			windows, _ = resolveOutputClipWindows(meta, options, highlightCandidates, qualitySettings, len(highlightCandidates))
		}
		animatedCreated, animatedCover, err := p.persistAnimatedOutputs(
			ctx,
			tx,
			job.ID,
			collection,
			prefix,
			sourcePath,
			meta,
			options,
			windows,
			animatedFormats,
			qualitySettings,
			displayOrder,
			uploader,
			&uploadedKeys,
			coverKey,
			generatedFormatSet,
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, 0, uploadedKeys, nil, packageBundleOutcome{}, err
		}
		displayOrder += animatedCreated
		if coverKey == "" {
			coverKey = animatedCover
		}
	}

	// 动图渲染已完成，切到上传/打包阶段。
	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageUploading,
		"progress": 86,
	})

	packageOutcome := p.persistCollectionOutputZipWithRetry(
		ctx,
		tx,
		job,
		collection,
		prefix,
		uploader,
		&uploadedKeys,
		generatedFormatSet,
	)
	if packageOutcome.Key != "" {
		now := time.Now()
		collection.LatestZipKey = packageOutcome.Key
		collection.LatestZipName = packageOutcome.Name
		collection.LatestZipSize = packageOutcome.SizeBytes
		collection.LatestZipAt = &now
	}

	if displayOrder <= 1 {
		_ = tx.Rollback()
		return 0, 0, uploadedKeys, nil, packageOutcome, permanentError{err: errors.New("no output generated; please select at least one supported format")}
	}

	if coverKey != "" {
		collection.CoverURL = coverKey
	}
	collection.FileCount = displayOrder - 1
	if err := tx.Save(&collection).Error; err != nil {
		_ = tx.Rollback()
		return 0, 0, uploadedKeys, nil, packageOutcome, err
	}

	if err := tx.Model(&models.VideoJob{}).
		Where("id = ?", job.ID).
		Update("result_collection_id", collection.ID).Error; err != nil {
		_ = tx.Rollback()
		return 0, 0, uploadedKeys, nil, packageOutcome, err
	}

	if err := tx.Commit().Error; err != nil {
		return 0, 0, uploadedKeys, nil, packageOutcome, err
	}

	generatedFormats := make([]string, 0, len(generatedFormatSet))
	for format := range generatedFormatSet {
		generatedFormats = append(generatedFormats, format)
	}
	sort.Strings(generatedFormats)

	return collection.ID, staticCount, uploadedKeys, generatedFormats, packageOutcome, nil
}

func (p *Processor) persistStillFrameOutputs(
	tx *gorm.DB,
	jobID uint64,
	collection models.Collection,
	prefix string,
	framePaths []string,
	formats []string,
	qualitySettings QualitySettings,
	startOrder int,
	uploader *qiniustorage.FormUploader,
	uploadedKeys *[]string,
	generatedFormatSet map[string]struct{},
) (int, string, error) {
	if len(formats) == 0 {
		return 0, "", nil
	}
	tasks := make([]stillFrameTask, 0, len(formats)*len(framePaths))
	order := startOrder
	for _, format := range formats {
		for index, framePath := range framePaths {
			tasks = append(tasks, stillFrameTask{
				Format:    format,
				FramePath: framePath,
				FrameIdx:  index + 1,
				Order:     order,
				Key:       buildVideoImageOutputObjectKey(prefix, format, fmt.Sprintf("%04d.%s", order, format)),
			})
			order++
		}
	}

	results := p.processStillFrameTasks(tasks, qualitySettings, uploader)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Task.Order < results[j].Task.Order
	})

	var firstErr error
	for _, result := range results {
		if result.Err != nil {
			if firstErr == nil {
				firstErr = result.Err
			}
			continue
		}
		*uploadedKeys = append(*uploadedKeys, result.Task.Key)
	}
	if firstErr != nil {
		return 0, "", firstErr
	}

	created := 0
	coverKey := ""
	for _, result := range results {
		generatedFormatSet[result.Task.Format] = struct{}{}
		title := fmt.Sprintf("%s-%02d", collection.Title, result.Task.FrameIdx)
		if len(formats) > 1 {
			title = fmt.Sprintf("%s-%s-%02d", collection.Title, strings.ToUpper(result.Task.Format), result.Task.FrameIdx)
		}

		emoji := models.Emoji{
			CollectionID: collection.ID,
			Title:        title,
			FileURL:      result.Task.Key,
			ThumbURL:     result.Task.Key,
			Format:       result.Task.Format,
			Width:        result.Width,
			Height:       result.Height,
			SizeBytes:    result.SizeBytes,
			DisplayOrder: result.Task.Order,
			Status:       "active",
		}
		if err := upsertEmojiByCollectionFile(tx, &emoji); err != nil {
			return created, coverKey, err
		}

		artifact := models.VideoJobArtifact{
			JobID:     jobID,
			Type:      "frame",
			QiniuKey:  result.Task.Key,
			MimeType:  mimeTypeByFormat(result.Task.Format),
			SizeBytes: result.SizeBytes,
			Width:     result.Width,
			Height:    result.Height,
			Metadata: mustJSON(map[string]interface{}{
				"index":  result.Task.FrameIdx,
				"format": result.Task.Format,
			}),
		}
		if err := upsertVideoJobArtifactByJobKey(tx, &artifact); err != nil {
			return created, coverKey, err
		}

		if coverKey == "" {
			coverKey = result.Task.Key
		}
		created++
	}
	return created, coverKey, nil
}

type stillFrameTask struct {
	Format    string
	FramePath string
	FrameIdx  int
	Order     int
	Key       string
}

type stillFrameTaskResult struct {
	Task      stillFrameTask
	SizeBytes int64
	Width     int
	Height    int
	Err       error
}

func (p *Processor) processStillFrameTasks(tasks []stillFrameTask, qualitySettings QualitySettings, uploader *qiniustorage.FormUploader) []stillFrameTaskResult {
	results := make([]stillFrameTaskResult, len(tasks))
	if len(tasks) == 0 {
		return results
	}

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	workers := qualitySettings.UploadConcurrency
	if workers < 1 {
		workers = 1
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}

	jobs := make(chan int)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				task := tasks[idx]
				targetPath, err := prepareStillFrameTarget(task.FramePath, task.Format, qualitySettings)
				if err != nil {
					results[idx] = stillFrameTaskResult{
						Task: task,
						Err:  fmt.Errorf("prepare %s frame %d: %w", task.Format, task.FrameIdx, err),
					}
					continue
				}

				if err := uploadFileToQiniu(uploader, p.qiniu, task.Key, targetPath); err != nil {
					results[idx] = stillFrameTaskResult{
						Task: task,
						Err:  fmt.Errorf("upload %s frame %d: %w", task.Format, task.FrameIdx, err),
					}
					continue
				}

				sizeBytes, width, height := readImageInfo(targetPath)
				results[idx] = stillFrameTaskResult{
					Task:      task,
					SizeBytes: sizeBytes,
					Width:     width,
					Height:    height,
				}
			}
		}()
	}

	for idx := range tasks {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()
	return results
}

func prepareStillFrameTarget(framePath, format string, qualitySettings QualitySettings) (string, error) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	switch format {
	case "jpg":
		if qualitySettings.JPGProfile == QualityProfileClarity {
			return framePath, nil
		}
		convertedPath := filepath.Join(
			filepath.Dir(framePath),
			fmt.Sprintf("%s_size.jpg", strings.TrimSuffix(filepath.Base(framePath), filepath.Ext(framePath))),
		)
		if err := convertImageToJPG(framePath, convertedPath, qualitySettings.JPGProfile, qualitySettings.JPGTargetSizeKB); err != nil {
			return "", err
		}
		return convertedPath, nil
	case "png":
		convertedPath := filepath.Join(
			filepath.Dir(framePath),
			fmt.Sprintf("%s.png", strings.TrimSuffix(filepath.Base(framePath), filepath.Ext(framePath))),
		)
		if err := convertImageToPNG(framePath, convertedPath, qualitySettings.PNGProfile, qualitySettings.PNGTargetSizeKB); err != nil {
			return "", err
		}
		return convertedPath, nil
	default:
		return "", fmt.Errorf("unsupported still format: %s", format)
	}
}

func (p *Processor) persistAnimatedOutputs(
	ctx context.Context,
	tx *gorm.DB,
	jobID uint64,
	collection models.Collection,
	prefix string,
	sourcePath string,
	meta videoProbeMeta,
	options jobOptions,
	windows []highlightCandidate,
	formats []string,
	qualitySettings QualitySettings,
	startOrder int,
	uploader *qiniustorage.FormUploader,
	uploadedKeys *[]string,
	fallbackCover string,
	generatedFormatSet map[string]struct{},
) (int, string, error) {
	if len(windows) == 0 || len(formats) == 0 {
		return 0, fallbackCover, nil
	}
	outputDir := filepath.Join(filepath.Dir(sourcePath), "animated")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return 0, fallbackCover, fmt.Errorf("create animated output dir: %w", err)
	}

	order := startOrder
	tasks := make([]animatedTask, 0, len(windows)*len(formats))
	for windowIndex, window := range windows {
		for _, format := range formats {
			tasks = append(tasks, animatedTask{
				WindowIndex: windowIndex + 1,
				Window:      window,
				Format:      format,
				Order:       order,
			})
			order++
		}
	}

	results := p.processAnimatedTasks(ctx, jobID, sourcePath, outputDir, prefix, meta, options, qualitySettings, uploader, tasks)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Task.Order < results[j].Task.Order
	})

	unsupportedFormatReasons := map[string]string{}
	var firstErr error
	for _, result := range results {
		if len(result.UploadedKeys) > 0 {
			*uploadedKeys = append(*uploadedKeys, result.UploadedKeys...)
		}
		if result.Err != nil && firstErr == nil {
			firstErr = result.Err
		}
	}
	if firstErr != nil {
		return 0, fallbackCover, firstErr
	}

	coverKey := fallbackCover
	created := 0
	for _, result := range results {
		if result.UnsupportedReason != "" {
			if _, exists := unsupportedFormatReasons[result.Task.Format]; !exists {
				unsupportedFormatReasons[result.Task.Format] = result.UnsupportedReason
			}
			continue
		}
		if result.Err != nil {
			continue
		}
		generatedFormatSet[result.Task.Format] = struct{}{}
		if coverKey == "" {
			coverKey = result.ThumbKey
		}

		emoji := models.Emoji{
			CollectionID: collection.ID,
			Title:        buildAnimatedEmojiTitle(collection.Title, result.Task.WindowIndex, result.Task.Format),
			FileURL:      result.FileKey,
			ThumbURL:     result.ThumbKey,
			Format:       result.Task.Format,
			Width:        result.Width,
			Height:       result.Height,
			SizeBytes:    result.SizeBytes,
			DisplayOrder: result.Task.Order,
			Status:       "active",
		}
		if err := upsertEmojiByCollectionFile(tx, &emoji); err != nil {
			return created, coverKey, err
		}

		for _, payload := range result.Artifacts {
			artifact := models.VideoJobArtifact{
				JobID:      jobID,
				Type:       payload.Type,
				QiniuKey:   payload.Key,
				MimeType:   payload.MimeType,
				SizeBytes:  payload.SizeBytes,
				Width:      payload.Width,
				Height:     payload.Height,
				DurationMs: payload.DurationMs,
				Metadata:   mustJSON(payload.Metadata),
			}
			if err := upsertVideoJobArtifactByJobKey(tx, &artifact); err != nil {
				return created, coverKey, err
			}
		}
		created++
	}

	for format, reason := range unsupportedFormatReasons {
		p.appendJobEvent(jobID, models.VideoJobStageRendering, "warn", "skip unsupported output format", map[string]interface{}{
			"format": format,
			"reason": reason,
		})
	}
	return created, coverKey, nil
}

const (
	packageZipStatusSkipped = "skipped"
	packageZipStatusPending = "pending"
	packageZipStatusReady   = "ready"
	packageZipStatusFailed  = "failed"

	defaultPackageZipMaxAttempts = 3
)

type packageBundleOutcome struct {
	Status     string
	Key        string
	Name       string
	SizeBytes  int64
	Attempts   int
	RetryCount int
	Error      string
}

func (p *Processor) persistCollectionOutputZipWithRetry(
	ctx context.Context,
	tx *gorm.DB,
	job models.VideoJob,
	collection models.Collection,
	prefix string,
	uploader *qiniustorage.FormUploader,
	uploadedKeys *[]string,
	generatedFormatSet map[string]struct{},
) packageBundleOutcome {
	outcome := packageBundleOutcome{
		Status: packageZipStatusSkipped,
	}
	if p == nil || tx == nil || p.qiniu == nil || uploader == nil || job.ID == 0 || collection.ID == 0 {
		return outcome
	}

	outcome.Status = packageZipStatusPending
	maxAttempts := defaultPackageZipMaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		outcome.Attempts = attempt
		packageKey, packageName, packageSize, packageErr := p.persistCollectionOutputZip(
			ctx,
			tx,
			job,
			collection,
			prefix,
			uploader,
			uploadedKeys,
			generatedFormatSet,
		)
		if packageErr == nil {
			if packageKey == "" {
				outcome.Status = packageZipStatusSkipped
				outcome.RetryCount = max(0, attempt-1)
				return outcome
			}
			outcome.Status = packageZipStatusReady
			outcome.Key = packageKey
			outcome.Name = packageName
			outcome.SizeBytes = packageSize
			outcome.RetryCount = max(0, attempt-1)
			return outcome
		}

		lastErr = packageErr
		outcome.RetryCount = max(0, attempt-1)
		p.appendJobEvent(job.ID, models.VideoJobStageUploading, "warn", "zip package attempt failed", map[string]interface{}{
			"attempt":      attempt,
			"max_attempts": maxAttempts,
			"error":        packageErr.Error(),
		})
		if attempt >= maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			lastErr = ctx.Err()
			attempt = maxAttempts
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}

	outcome.Status = packageZipStatusFailed
	if lastErr != nil {
		outcome.Error = lastErr.Error()
	}
	outcome.RetryCount = max(0, outcome.Attempts-1)
	p.appendJobEvent(job.ID, models.VideoJobStageUploading, "warn", "zip package generation exhausted retries", map[string]interface{}{
		"attempts": outcome.Attempts,
		"error":    outcome.Error,
	})
	return outcome
}

func (p *Processor) persistCollectionOutputZip(
	ctx context.Context,
	tx *gorm.DB,
	job models.VideoJob,
	collection models.Collection,
	prefix string,
	uploader *qiniustorage.FormUploader,
	uploadedKeys *[]string,
	generatedFormatSet map[string]struct{},
) (string, string, int64, error) {
	if p == nil || tx == nil || p.qiniu == nil || uploader == nil || job.ID == 0 || collection.ID == 0 {
		return "", "", 0, nil
	}

	var emojis []models.Emoji
	if err := tx.Where("collection_id = ? AND status = ?", collection.ID, "active").
		Order("display_order ASC, id ASC").
		Find(&emojis).Error; err != nil {
		return "", "", 0, err
	}
	if len(emojis) == 0 {
		return "", "", 0, nil
	}

	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("video-job-%d-zip-*", job.ID))
	if err != nil {
		return "", "", 0, err
	}
	defer os.RemoveAll(tmpDir)

	entries := make([]zipEntrySource, 0, len(emojis))
	for idx, item := range emojis {
		key := strings.TrimLeft(strings.TrimSpace(item.FileURL), "/")
		if key == "" {
			continue
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(key)), ".")
		if ext == "" {
			ext = strings.TrimSpace(strings.ToLower(item.Format))
		}
		if ext == "" {
			ext = "bin"
		}

		entryBase := sanitizeZipEntryComponent(item.Title)
		if entryBase == "" {
			entryBase = fmt.Sprintf("item_%03d", idx+1)
		}
		entryName := fmt.Sprintf("%03d_%s.%s", idx+1, entryBase, ext)
		localFile := filepath.Join(tmpDir, fmt.Sprintf("%03d.%s", idx+1, ext))

		if err := p.downloadObjectByKey(ctx, key, localFile); err != nil {
			return "", "", 0, err
		}
		entries = append(entries, zipEntrySource{
			Name: entryName,
			Path: localFile,
		})
	}
	if len(entries) == 0 {
		return "", "", 0, nil
	}

	zipPath := filepath.Join(tmpDir, fmt.Sprintf("%d_outputs.zip", job.ID))
	if err := createZipArchive(zipPath, entries); err != nil {
		return "", "", 0, err
	}
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return "", "", 0, err
	}

	packageFormat := resolvePackageFormatFromGeneratedSet(generatedFormatSet)
	packageName := fmt.Sprintf("%d_%s_v1.zip", job.ID, packageFormat)
	packageKey := buildVideoImagePackageObjectKey(prefix, packageName)
	if err := uploadFileToQiniu(uploader, p.qiniu, packageKey, zipPath); err != nil {
		return "", "", 0, err
	}
	if uploadedKeys != nil {
		*uploadedKeys = append(*uploadedKeys, packageKey)
	}

	artifact := models.VideoJobArtifact{
		JobID:     job.ID,
		Type:      "package",
		QiniuKey:  packageKey,
		MimeType:  "application/zip",
		SizeBytes: zipInfo.Size(),
		Metadata: mustJSON(map[string]interface{}{
			"format":      "zip",
			"source":      "auto_bundle",
			"file_count":  len(entries),
			"bundle_type": packageFormat,
		}),
	}
	if err := upsertVideoJobArtifactByJobKey(tx, &artifact); err != nil {
		return "", "", 0, err
	}

	uploadedAt := time.Now()
	zipRecord := models.CollectionZip{
		CollectionID: collection.ID,
		ZipKey:       packageKey,
		ZipName:      packageName,
		SizeBytes:    zipInfo.Size(),
		UploadedAt:   &uploadedAt,
	}
	if err := tx.Where("collection_id = ? AND zip_key = ?", collection.ID, packageKey).
		Assign(models.CollectionZip{
			ZipName:    packageName,
			SizeBytes:  zipInfo.Size(),
			UploadedAt: &uploadedAt,
		}).
		FirstOrCreate(&zipRecord).Error; err != nil {
		return "", "", 0, err
	}

	return packageKey, packageName, zipInfo.Size(), nil
}

func resolvePackageFormatFromGeneratedSet(generatedFormatSet map[string]struct{}) string {
	if len(generatedFormatSet) == 1 {
		for format := range generatedFormatSet {
			clean := strings.TrimSpace(strings.ToLower(format))
			if clean != "" {
				return clean
			}
		}
	}
	return "mixed"
}

func sanitizeZipEntryComponent(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "\\", "-")
	value = strings.ReplaceAll(value, ":", "-")
	value = strings.ReplaceAll(value, "*", "-")
	value = strings.ReplaceAll(value, "?", "-")
	value = strings.ReplaceAll(value, "\"", "-")
	value = strings.ReplaceAll(value, "<", "-")
	value = strings.ReplaceAll(value, ">", "-")
	value = strings.ReplaceAll(value, "|", "-")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 64 {
		value = strings.TrimSpace(value[:64])
	}
	value = strings.Trim(value, ". ")
	if value == "" {
		return ""
	}
	return value
}

type animatedTask struct {
	WindowIndex int
	Window      highlightCandidate
	Format      string
	Order       int
}

type animatedArtifactPayload struct {
	Type       string
	Key        string
	MimeType   string
	SizeBytes  int64
	Width      int
	Height     int
	DurationMs int
	Metadata   map[string]interface{}
}

type animatedTaskResult struct {
	Task              animatedTask
	FileKey           string
	ThumbKey          string
	Width             int
	Height            int
	SizeBytes         int64
	UploadedKeys      []string
	Artifacts         []animatedArtifactPayload
	UnsupportedReason string
	Err               error
}

type qiniuUploadTask struct {
	Key   string
	Path  string
	Label string
}

type animatedAdaptiveProfile struct {
	MotionScore        float64
	Level              string
	DurationSec        float64
	FPS                int
	Width              int
	MaxColors          int
	StabilityTier      string
	LongVideoDownshift bool
}

type gifLoopSampleFrame struct {
	TimestampSec float64
	Hash         uint64
	QualityScore float64
}

type gifLoopTuningResult struct {
	Applied        bool
	BaseStartSec   float64
	BaseEndSec     float64
	TunedStartSec  float64
	TunedEndSec    float64
	EffectiveSec   float64
	DurationSec    float64
	Score          float64
	BaseScore      float64
	BestScore      float64
	Improvement    float64
	MinImprovement float64
	LoopClosure    float64
	BaseLoop       float64
	BestLoop       float64
	MotionMean     float64
	BaseMotion     float64
	BestMotion     float64
	QualityMean    float64
	SampleFrames   int
	Candidates     int
	FallbackToBase bool
	FallbackReason string
	DecisionReason string
}

func (p *Processor) processAnimatedTasks(
	ctx context.Context,
	jobID uint64,
	sourcePath string,
	outputDir string,
	prefix string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	uploader *qiniustorage.FormUploader,
	tasks []animatedTask,
) []animatedTaskResult {
	results := make([]animatedTaskResult, len(tasks))
	if len(tasks) == 0 {
		return results
	}

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	workers := qualitySettings.UploadConcurrency
	if workers < 1 {
		workers = 1
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}
	workers = resolveGIFRenderWorkerCap(meta, tasks, qualitySettings, workers)

	totalTasks := len(tasks)
	var completedTasks atomic.Int32

	sem := make(chan struct{}, workers)
	var group errgroup.Group
	for idx := range tasks {
		idx := idx
		group.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = p.processAnimatedTask(ctx, sourcePath, outputDir, prefix, meta, options, qualitySettings, uploader, tasks[idx])
			if jobID > 0 && totalTasks > 0 {
				done := int(completedTasks.Add(1))
				progress := 70 + int(math.Round(float64(done)/float64(totalTasks)*15.0))
				if progress < 70 {
					progress = 70
				}
				if progress > 85 {
					progress = 85
				}
				p.updateVideoJob(jobID, map[string]interface{}{
					"stage":    models.VideoJobStageRendering,
					"progress": progress,
				})
			}
			return nil
		})
	}
	_ = group.Wait()
	return results
}
