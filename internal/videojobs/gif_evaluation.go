package videojobs

import (
	"fmt"
	"math"
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func UpsertGIFEvaluationByPublicOutput(tx *gorm.DB, output models.VideoImageOutputPublic) error {
	if tx == nil || output.JobID == 0 || output.ID == 0 {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(output.Format)) != "gif" || strings.ToLower(strings.TrimSpace(output.FileRole)) != "main" {
		return nil
	}

	outputMeta := parseJSONMap(output.Metadata)
	startSec := floatFromAny(outputMeta["start_sec"])
	if startSec < 0 {
		startSec = 0
	}
	endSec := floatFromAny(outputMeta["end_sec"])
	if endSec <= startSec {
		if output.DurationMs > 0 {
			endSec = startSec + float64(output.DurationMs)/1000.0
		}
	}
	windowStartMs := int(math.Round(startSec * 1000))
	if windowStartMs < 0 {
		windowStartMs = 0
	}
	windowEndMs := int(math.Round(endSec * 1000))
	if windowEndMs < windowStartMs {
		windowEndMs = windowStartMs
	}

	candidateID, candidateBaseScore, candidateConfidence, candidateFeature, err := resolveGIFCandidateForEvaluation(tx, output.JobID, outputMeta, windowStartMs, windowEndMs)
	if err != nil {
		return err
	}

	emotion := clampZeroOne(output.Score)
	if emotion <= 0 {
		emotion = clampZeroOne(candidateBaseScore)
	}

	motion := clampZeroOne(output.GIFLoopTuneMotionMean)
	if motion <= 0 {
		motion = clampZeroOne(floatFromAny(outputMeta["motion_score"]))
	}
	if motion <= 0 {
		motion = clampZeroOne(floatFromAny(candidateFeature["motion_mean"]))
	}

	loop := clampZeroOne(output.GIFLoopTuneLoopClosure)
	if loop <= 0 {
		if tune, ok := outputMeta["gif_loop_tune"].(map[string]interface{}); ok {
			loop = clampZeroOne(floatFromAny(tune["loop_closure"]))
		}
	}

	clarity := estimateGIFClarityScore(candidateFeature, outputMeta, emotion)
	efficiency := estimateGIFFiciencyScore(output.SizeBytes, output.Width, output.Height, output.DurationMs)
	overall := clampZeroOne(
		emotion*0.24 +
			clarity*0.24 +
			motion*0.16 +
			loop*0.20 +
			efficiency*0.16,
	)

	featureJSON := map[string]interface{}{
		"output_score":         roundTo(output.Score, 4),
		"candidate_base_score": roundTo(candidateBaseScore, 4),
		"candidate_confidence": roundTo(candidateConfidence, 4),
		"window_start_ms":      windowStartMs,
		"window_end_ms":        windowEndMs,
		"size_bytes":           output.SizeBytes,
		"width":                output.Width,
		"height":               output.Height,
		"duration_ms":          output.DurationMs,
		"loop_closure":         roundTo(loop, 4),
		"motion_mean":          roundTo(motion, 4),
		"candidate_feature":    candidateFeature,
	}
	if optimization := mapFromAny(outputMeta["gif_optimization_v1"]); len(optimization) > 0 {
		featureJSON["gif_optimization_v1"] = optimization
	}

	outputID := output.ID
	row := models.VideoJobGIFEvaluation{
		JobID:           output.JobID,
		OutputID:        &outputID,
		CandidateID:     candidateID,
		WindowStartMs:   windowStartMs,
		WindowEndMs:     windowEndMs,
		EmotionScore:    roundTo(emotion, 4),
		ClarityScore:    roundTo(clarity, 4),
		MotionScore:     roundTo(motion, 4),
		LoopScore:       roundTo(loop, 4),
		EfficiencyScore: roundTo(efficiency, 4),
		OverallScore:    roundTo(overall, 4),
		FeatureJSON:     mustJSON(featureJSON),
	}
	if row.FeatureJSON == nil {
		row.FeatureJSON = datatypes.JSON([]byte("{}"))
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "job_id"}, {Name: "output_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"candidate_id":     row.CandidateID,
			"window_start_ms":  row.WindowStartMs,
			"window_end_ms":    row.WindowEndMs,
			"emotion_score":    row.EmotionScore,
			"clarity_score":    row.ClarityScore,
			"motion_score":     row.MotionScore,
			"loop_score":       row.LoopScore,
			"efficiency_score": row.EfficiencyScore,
			"overall_score":    row.OverallScore,
			"feature_json":     row.FeatureJSON,
			"updated_at":       time.Now(),
		}),
	}).Create(&row).Error
}

func matchGIFCandidateForEvaluation(
	tx *gorm.DB,
	jobID uint64,
	windowStartMs int,
	windowEndMs int,
) (candidateID *uint64, baseScore float64, confidence float64, feature map[string]interface{}, err error) {
	feature = map[string]interface{}{}
	if tx == nil || jobID == 0 {
		return nil, 0, 0, feature, nil
	}
	type row struct {
		ID              uint64         `gorm:"column:id"`
		StartMs         int            `gorm:"column:start_ms"`
		EndMs           int            `gorm:"column:end_ms"`
		BaseScore       float64        `gorm:"column:base_score"`
		ConfidenceScore float64        `gorm:"column:confidence_score"`
		FeatureJSON     datatypes.JSON `gorm:"column:feature_json"`
		IsSelected      bool           `gorm:"column:is_selected"`
		FinalRank       int            `gorm:"column:final_rank"`
	}
	var rows []row
	if err := tx.Model(&models.VideoJobGIFCandidate{}).
		Select("id", "start_ms", "end_ms", "base_score", "confidence_score", "feature_json", "is_selected", "final_rank").
		Where("job_id = ?", jobID).
		Order("is_selected DESC, final_rank ASC, id ASC").
		Limit(24).
		Scan(&rows).Error; err != nil {
		return nil, 0, 0, feature, err
	}
	if len(rows) == 0 {
		return nil, 0, 0, feature, nil
	}

	bestIdx := -1
	bestIOU := 0.0
	if windowEndMs > windowStartMs {
		for idx := range rows {
			if rows[idx].EndMs <= rows[idx].StartMs {
				continue
			}
			iou := windowIOUMs(windowStartMs, windowEndMs, rows[idx].StartMs, rows[idx].EndMs)
			if iou > bestIOU {
				bestIOU = iou
				bestIdx = idx
			}
		}
	}
	if bestIdx < 0 || bestIOU < 0.2 {
		bestIdx = 0
	}
	selected := rows[bestIdx]
	id := selected.ID
	candidateID = &id
	baseScore = selected.BaseScore
	confidence = selected.ConfidenceScore
	feature = parseJSONMap(selected.FeatureJSON)
	return candidateID, baseScore, confidence, feature, nil
}

func resolveGIFCandidateForEvaluation(
	tx *gorm.DB,
	jobID uint64,
	outputMeta map[string]interface{},
	windowStartMs int,
	windowEndMs int,
) (candidateID *uint64, baseScore float64, confidence float64, feature map[string]interface{}, err error) {
	feature = map[string]interface{}{}
	if tx == nil || jobID == 0 {
		return nil, 0, 0, feature, nil
	}
	if explicitID := uint64(floatFromAny(outputMeta["candidate_id"])); explicitID > 0 {
		type candidateRow struct {
			ID              uint64         `gorm:"column:id"`
			BaseScore       float64        `gorm:"column:base_score"`
			ConfidenceScore float64        `gorm:"column:confidence_score"`
			FeatureJSON     datatypes.JSON `gorm:"column:feature_json"`
		}
		var row candidateRow
		queryErr := tx.Model(&models.VideoJobGIFCandidate{}).
			Select("id", "base_score", "confidence_score", "feature_json").
			Where("job_id = ? AND id = ?", jobID, explicitID).
			Limit(1).
			Find(&row).Error
		if queryErr != nil {
			return nil, 0, 0, feature, queryErr
		}
		if row.ID > 0 {
			id := row.ID
			return &id, row.BaseScore, row.ConfidenceScore, parseJSONMap(row.FeatureJSON), nil
		}
	}
	return matchGIFCandidateForEvaluation(tx, jobID, windowStartMs, windowEndMs)
}

func windowIOUMs(aStart, aEnd, bStart, bEnd int) float64 {
	if aEnd <= aStart || bEnd <= bStart {
		return 0
	}
	interStart := maxIntValue(aStart, bStart)
	interEnd := minInt(aEnd, bEnd)
	if interEnd <= interStart {
		return 0
	}
	inter := float64(interEnd - interStart)
	union := float64((aEnd - aStart) + (bEnd - bStart) - (interEnd - interStart))
	if union <= 0 {
		return 0
	}
	return inter / union
}

func estimateGIFClarityScore(candidateFeature map[string]interface{}, outputMeta map[string]interface{}, fallback float64) float64 {
	parts := make([]float64, 0, 4)
	if value := clampZeroOne(floatFromAny(candidateFeature["quality_mean"])); value > 0 {
		parts = append(parts, value)
	}
	if value := clampZeroOne(floatFromAny(candidateFeature["subject_mean"])); value > 0 {
		parts = append(parts, value)
	}
	if value := clampZeroOne(floatFromAny(candidateFeature["exposure_mean"])); value > 0 {
		parts = append(parts, value)
	}
	if value := clampZeroOne(floatFromAny(candidateFeature["blur_norm"])); value > 0 {
		parts = append(parts, value)
	}
	if len(parts) == 0 {
		if value := clampZeroOne(floatFromAny(outputMeta["quality_score"])); value > 0 {
			return value
		}
		return clampZeroOne(fallback)
	}
	sum := 0.0
	for _, item := range parts {
		sum += item
	}
	return clampZeroOne(sum / float64(len(parts)))
}

func estimateGIFFiciencyScore(sizeBytes int64, width, height, durationMs int) float64 {
	if sizeBytes <= 0 || width <= 0 || height <= 0 {
		return 0.5
	}
	durationSec := float64(durationMs) / 1000.0
	if durationSec <= 0 {
		durationSec = 1.0
	}
	pixels := float64(width * height)
	normalized := float64(sizeBytes) / (pixels * durationSec)
	// 经验区间：越小越高，映射到 (0,1]
	score := 1.0 / (1.0 + normalized*180.0)
	return clampZeroOne(score)
}

func SyncGIFBaselineByJobID(db *gorm.DB, jobID uint64) error {
	if db == nil || jobID == 0 {
		return nil
	}
	var row struct {
		CreatedAt       time.Time `gorm:"column:created_at"`
		RequestedFormat string    `gorm:"column:requested_format"`
	}
	if err := db.Model(&models.VideoImageJobPublic{}).
		Select("created_at", "requested_format").
		Where("id = ?", jobID).
		Scan(&row).Error; err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(row.RequestedFormat)) != "gif" || row.CreatedAt.IsZero() {
		return nil
	}
	return SyncGIFBaselineDaily(db, row.CreatedAt)
}

func SyncGIFBaselineDaily(db *gorm.DB, date time.Time) error {
	if db == nil || date.IsZero() {
		return nil
	}
	dayStart := time.Date(date.UTC().Year(), date.UTC().Month(), date.UTC().Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)

	type jobsAgg struct {
		SampleJobs int64 `gorm:"column:sample_jobs"`
		DoneJobs   int64 `gorm:"column:done_jobs"`
		FailedJobs int64 `gorm:"column:failed_jobs"`
	}
	type outputsAgg struct {
		SampleOutputs  int64   `gorm:"column:sample_outputs"`
		AvgOutputScore float64 `gorm:"column:avg_output_score"`
		AvgLoopClosure float64 `gorm:"column:avg_loop_closure"`
		AvgSizeBytes   float64 `gorm:"column:avg_size_bytes"`
	}
	type evalAgg struct {
		AvgEmotionScore    float64 `gorm:"column:avg_emotion_score"`
		AvgClarityScore    float64 `gorm:"column:avg_clarity_score"`
		AvgMotionScore     float64 `gorm:"column:avg_motion_score"`
		AvgLoopScore       float64 `gorm:"column:avg_loop_score"`
		AvgEfficiencyScore float64 `gorm:"column:avg_efficiency_score"`
		AvgOverallScore    float64 `gorm:"column:avg_overall_score"`
	}

	var jobs jobsAgg
	if err := db.Raw(`
SELECT
	COUNT(*) AS sample_jobs,
	COUNT(*) FILTER (WHERE status = 'done') AS done_jobs,
	COUNT(*) FILTER (WHERE status = 'failed') AS failed_jobs
FROM public.video_image_jobs
WHERE requested_format = 'gif'
	AND created_at >= ?
	AND created_at < ?
`, dayStart, dayEnd).Scan(&jobs).Error; err != nil {
		return err
	}

	var outputs outputsAgg
	if err := db.Raw(`
SELECT
	COUNT(*) AS sample_outputs,
	COALESCE(AVG(score), 0) AS avg_output_score,
	COALESCE(AVG(gif_loop_tune_loop_closure), 0) AS avg_loop_closure,
	COALESCE(AVG(size_bytes), 0) AS avg_size_bytes
FROM public.video_image_outputs
WHERE format = 'gif'
	AND file_role = 'main'
	AND created_at >= ?
	AND created_at < ?
`, dayStart, dayEnd).Scan(&outputs).Error; err != nil {
		return err
	}

	var eval evalAgg
	if err := db.Raw(`
SELECT
	COALESCE(AVG(e.emotion_score), 0) AS avg_emotion_score,
	COALESCE(AVG(e.clarity_score), 0) AS avg_clarity_score,
	COALESCE(AVG(e.motion_score), 0) AS avg_motion_score,
	COALESCE(AVG(e.loop_score), 0) AS avg_loop_score,
	COALESCE(AVG(e.efficiency_score), 0) AS avg_efficiency_score,
	COALESCE(AVG(e.overall_score), 0) AS avg_overall_score
FROM archive.video_job_gif_evaluations e
LEFT JOIN public.video_image_outputs o ON o.id = e.output_id
LEFT JOIN public.video_image_jobs j ON j.id = e.job_id
WHERE COALESCE(o.created_at, j.created_at, e.created_at) >= ?
	AND COALESCE(o.created_at, j.created_at, e.created_at) < ?
`, dayStart, dayEnd).Scan(&eval).Error; err != nil {
		return err
	}

	doneRate := 0.0
	failedRate := 0.0
	if jobs.SampleJobs > 0 {
		doneRate = float64(jobs.DoneJobs) / float64(jobs.SampleJobs)
		failedRate = float64(jobs.FailedJobs) / float64(jobs.SampleJobs)
	}

	row := models.VideoJobGIFBaseline{
		BaselineDate:       dayStart,
		WindowLabel:        "1d",
		Scope:              "all",
		RequestedFormat:    "gif",
		SampleJobs:         jobs.SampleJobs,
		DoneJobs:           jobs.DoneJobs,
		FailedJobs:         jobs.FailedJobs,
		DoneRate:           roundTo(doneRate, 6),
		FailedRate:         roundTo(failedRate, 6),
		SampleOutputs:      outputs.SampleOutputs,
		AvgEmotionScore:    roundTo(eval.AvgEmotionScore, 6),
		AvgClarityScore:    roundTo(eval.AvgClarityScore, 6),
		AvgMotionScore:     roundTo(eval.AvgMotionScore, 6),
		AvgLoopScore:       roundTo(eval.AvgLoopScore, 6),
		AvgEfficiencyScore: roundTo(eval.AvgEfficiencyScore, 6),
		AvgOverallScore:    roundTo(eval.AvgOverallScore, 6),
		AvgOutputScore:     roundTo(outputs.AvgOutputScore, 6),
		AvgLoopClosure:     roundTo(outputs.AvgLoopClosure, 6),
		AvgSizeBytes:       roundTo(outputs.AvgSizeBytes, 2),
	}
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "baseline_date"},
			{Name: "window_label"},
			{Name: "scope"},
			{Name: "requested_format"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"sample_jobs":          row.SampleJobs,
			"done_jobs":            row.DoneJobs,
			"failed_jobs":          row.FailedJobs,
			"done_rate":            row.DoneRate,
			"failed_rate":          row.FailedRate,
			"sample_outputs":       row.SampleOutputs,
			"avg_emotion_score":    row.AvgEmotionScore,
			"avg_clarity_score":    row.AvgClarityScore,
			"avg_motion_score":     row.AvgMotionScore,
			"avg_loop_score":       row.AvgLoopScore,
			"avg_efficiency_score": row.AvgEfficiencyScore,
			"avg_overall_score":    row.AvgOverallScore,
			"avg_output_score":     row.AvgOutputScore,
			"avg_loop_closure":     row.AvgLoopClosure,
			"avg_size_bytes":       row.AvgSizeBytes,
			"updated_at":           time.Now(),
		}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("upsert gif baseline failed: %w", err)
	}
	return nil
}
