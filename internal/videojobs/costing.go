package videojobs

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	costCurrencyCNY           = "CNY"
	costPricingVersion        = "v1_lite"
	defaultBaseCostCNY        = 0.003
	defaultCPUCostPerHourCNY  = 0.68
	defaultDurationCostPerMin = 0.012
	defaultSourceGBStorageCNY = 0.02
	defaultOutputGBStorageCNY = 0.06
	defaultOutputItemCostCNY  = 0.0002
	defaultUSDtoCNYRate       = 7.2
)

// UpsertJobCost computes and stores cost snapshot for a video job.
// It is safe to call multiple times and will update the existing row by job_id.
func UpsertJobCost(db *gorm.DB, jobID uint64) error {
	if db == nil || jobID == 0 {
		return errors.New("invalid db or job id")
	}

	snapshot, err := buildVideoJobCostSnapshot(db, jobID)
	if err != nil {
		return err
	}

	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "job_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"user_id":              snapshot.UserID,
			"status":               snapshot.Status,
			"cpu_ms":               snapshot.CPUms,
			"gpu_ms":               snapshot.GPUms,
			"asr_seconds":          snapshot.ASRSeconds,
			"ocr_frames":           snapshot.OCRFrames,
			"storage_bytes_raw":    snapshot.StorageBytesRaw,
			"storage_bytes_output": snapshot.StorageBytesOutput,
			"output_count":         snapshot.OutputCount,
			"estimated_cost":       snapshot.EstimatedCost,
			"currency":             snapshot.Currency,
			"pricing_version":      snapshot.PricingVersion,
			"details":              snapshot.Details,
			"updated_at":           time.Now(),
		}),
	}).Create(snapshot).Error
}

func buildVideoJobCostSnapshot(db *gorm.DB, jobID uint64) (*models.VideoJobCost, error) {
	var job models.VideoJob
	if err := db.Select("id", "user_id", "status", "started_at", "finished_at", "metrics").Where("id = ?", jobID).First(&job).Error; err != nil {
		return nil, err
	}

	var agg struct {
		OutputCount int64 `gorm:"column:output_count"`
		OutputBytes int64 `gorm:"column:output_bytes"`
	}
	if err := db.Model(&models.VideoJobArtifact{}).
		Select("COALESCE(count(*), 0) AS output_count, COALESCE(sum(size_bytes), 0) AS output_bytes").
		Where("job_id = ? AND type IN ?", jobID, []string{"frame", "clip", "poster"}).
		Scan(&agg).Error; err != nil {
		return nil, err
	}

	metrics := parseJSONMap(job.Metrics)
	sourceDurationSec := metricFloat(metrics, "duration_sec")
	sourceBytes := metricInt64(metrics, "source_video_size_bytes")
	if sourceBytes < 0 {
		sourceBytes = 0
	}

	gpuMs := metricInt64(metrics, "gpu_ms")
	asrSeconds := metricFloat(metrics, "asr_seconds")
	ocrFrames := int(metricInt64(metrics, "ocr_frames"))

	cpuMs := computeCPUms(job.StartedAt, job.FinishedAt)
	computeCostCNY, breakdown := estimateVideoJobCostCNY(cpuMs, sourceDurationSec, sourceBytes, agg.OutputBytes, agg.OutputCount)
	aiAgg, aiAggErr := loadVideoJobAIAggregate(db, jobID)
	usdToCNYRate := loadUSDtoCNYRate()
	aiLLMCostCNY := roundTo(aiAgg.CostUSD*usdToCNYRate, 6)
	aiImageEnhanceCostCNY, aiFaceEnhanceCostCNY, aiSuperResCostCNY := extractPNGWorkerEnhancementCostCNY(metrics)
	aiCostCNY := roundTo(aiLLMCostCNY+aiImageEnhanceCostCNY, 6)
	estimatedCost := roundTo(computeCostCNY+aiCostCNY, 6)

	status := strings.TrimSpace(job.Status)
	if status == "" {
		status = models.VideoJobStatusQueued
	}

	details := map[string]interface{}{
		"breakdown":                 breakdown,
		"source_duration_sec":       roundTo(sourceDurationSec, 3),
		"source_bytes":              sourceBytes,
		"output_count":              agg.OutputCount,
		"output_bytes":              agg.OutputBytes,
		"ai_usage_calls":            aiAgg.Calls,
		"ai_usage_error_calls":      aiAgg.ErrorCalls,
		"ai_usage_duration_ms":      aiAgg.DurationMs,
		"ai_input_tokens":           aiAgg.InputTokens,
		"ai_output_tokens":          aiAgg.OutputTokens,
		"ai_cached_input_tokens":    aiAgg.CachedInputTokens,
		"ai_image_tokens":           aiAgg.ImageTokens,
		"ai_video_tokens":           aiAgg.VideoTokens,
		"ai_audio_seconds":          aiAgg.AudioSeconds,
		"ai_cost_usd":               aiAgg.CostUSD,
		"ai_llm_cost_cny":           aiLLMCostCNY,
		"ai_image_enhance_cost_cny": aiImageEnhanceCostCNY,
		"ai_face_enhance_cost_cny":  aiFaceEnhanceCostCNY,
		"ai_superres_cost_cny":      aiSuperResCostCNY,
		"ai_cost_cny":               aiCostCNY,
		"usd_to_cny_rate":           usdToCNYRate,
		"ai_aggregate_loaded":       aiAggErr == nil,
	}
	if aiAggErr != nil {
		details["ai_aggregate_error"] = aiAggErr.Error()
	}
	breakdown["compute_total"] = roundTo(computeCostCNY, 6)
	breakdown["ai_token_cny"] = aiLLMCostCNY
	breakdown["ai_image_enhance_cny"] = aiImageEnhanceCostCNY
	breakdown["ai_total_cny"] = aiCostCNY
	breakdown["total"] = estimatedCost

	snapshot := &models.VideoJobCost{
		JobID:              job.ID,
		UserID:             job.UserID,
		Status:             status,
		CPUms:              cpuMs,
		GPUms:              gpuMs,
		ASRSeconds:         asrSeconds,
		OCRFrames:          ocrFrames,
		StorageBytesRaw:    sourceBytes,
		StorageBytesOutput: agg.OutputBytes,
		OutputCount:        int(agg.OutputCount),
		EstimatedCost:      estimatedCost,
		Currency:           costCurrencyCNY,
		PricingVersion:     costPricingVersion,
		Details:            mustJSON(details),
	}
	return snapshot, nil
}

func extractPNGWorkerEnhancementCostCNY(metrics map[string]interface{}) (total float64, face float64, super float64) {
	if len(metrics) == 0 {
		return 0, 0, 0
	}
	face = roundTo(maxFloat(0, parseOptionFloat(mapFromAny(metrics["png_worker_face_enhancement_v1"]), "total_cost_cny")), 6)
	super = roundTo(maxFloat(0, parseOptionFloat(mapFromAny(metrics["png_worker_super_resolution_v1"]), "total_cost_cny")), 6)
	total = roundTo(face+super, 6)
	return total, face, super
}

func computeCPUms(startedAt, finishedAt *time.Time) int64 {
	if startedAt == nil {
		return 0
	}
	end := time.Now()
	if finishedAt != nil {
		end = *finishedAt
	}
	if end.Before(*startedAt) {
		return 0
	}
	return end.Sub(*startedAt).Milliseconds()
}

func estimateVideoJobCostCNY(cpuMs int64, sourceDurationSec float64, sourceBytes, outputBytes int64, outputCount int64) (float64, map[string]float64) {
	if cpuMs < 0 {
		cpuMs = 0
	}
	if sourceDurationSec < 0 {
		sourceDurationSec = 0
	}
	if sourceBytes < 0 {
		sourceBytes = 0
	}
	if outputBytes < 0 {
		outputBytes = 0
	}
	if outputCount < 0 {
		outputCount = 0
	}

	cpuHours := float64(cpuMs) / 3600000
	durationMin := sourceDurationSec / 60
	sourceGB := float64(sourceBytes) / (1024 * 1024 * 1024)
	outputGB := float64(outputBytes) / (1024 * 1024 * 1024)

	cpuCost := cpuHours * defaultCPUCostPerHourCNY
	durationCost := durationMin * defaultDurationCostPerMin
	sourceStorageCost := sourceGB * defaultSourceGBStorageCNY
	outputStorageCost := outputGB * defaultOutputGBStorageCNY
	outputCountCost := float64(outputCount) * defaultOutputItemCostCNY
	total := roundTo(defaultBaseCostCNY+cpuCost+durationCost+sourceStorageCost+outputStorageCost+outputCountCost, 6)

	breakdown := map[string]float64{
		"base":           roundTo(defaultBaseCostCNY, 6),
		"cpu":            roundTo(cpuCost, 6),
		"duration":       roundTo(durationCost, 6),
		"source_storage": roundTo(sourceStorageCost, 6),
		"output_storage": roundTo(outputStorageCost, 6),
		"output_count":   roundTo(outputCountCost, 6),
		"total":          total,
	}
	return total, breakdown
}

func metricFloat(metrics map[string]interface{}, key string) float64 {
	if len(metrics) == 0 || strings.TrimSpace(key) == "" {
		return 0
	}
	raw, ok := metrics[key]
	if !ok {
		return 0
	}
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
	case uint64:
		return float64(v)
	case uint32:
		return float64(v)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func metricInt64(metrics map[string]interface{}, key string) int64 {
	return int64(metricFloat(metrics, key))
}

func loadUSDtoCNYRate() float64 {
	raw := strings.TrimSpace(os.Getenv("VIDEO_JOB_USD_TO_CNY_RATE"))
	if raw == "" {
		return defaultUSDtoCNYRate
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return defaultUSDtoCNYRate
	}
	return v
}
