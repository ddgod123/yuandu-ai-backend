package handlers

import (
	"fmt"
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/datatypes"
)

func (h *Handler) loadVideoJobLiveCoverSceneStats(since time.Time, minSamples int64) ([]AdminVideoJobLiveCoverSceneStat, error) {
	if minSamples <= 0 {
		minSamples = 5
	}
	type row struct {
		SceneTag         string   `gorm:"column:scene_tag"`
		Samples          int64    `gorm:"column:samples"`
		AvgCoverScore    *float64 `gorm:"column:avg_cover_score"`
		AvgCoverPortrait *float64 `gorm:"column:avg_cover_portrait"`
		AvgCoverExposure *float64 `gorm:"column:avg_cover_exposure"`
		AvgCoverFace     *float64 `gorm:"column:avg_cover_face"`
	}

	var rows []row
	if err := h.db.Raw(`
WITH cover AS (
	SELECT
		a.job_id,
		COALESCE((a.metadata->>'cover_score')::double precision, 0) AS cover_score,
		COALESCE((a.metadata->>'cover_portrait')::double precision, 0) AS cover_portrait,
		COALESCE((a.metadata->>'cover_exposure')::double precision, 0) AS cover_exposure,
		COALESCE((a.metadata->>'cover_face')::double precision, 0) AS cover_face
	FROM archive.video_job_artifacts a
	JOIN archive.video_jobs j ON j.id = a.job_id
	WHERE a.type = 'live_cover'
		AND j.status = ?
		AND j.finished_at >= ?
),
scene AS (
	SELECT
		j.id AS job_id,
		LOWER(TRIM(tag.value)) AS scene_tag
	FROM archive.video_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'scene_tags_v1') = 'array'
			THEN j.metrics->'scene_tags_v1'
			ELSE '[]'::jsonb
		END
	) AS tag(value)
	WHERE j.status = ?
		AND j.finished_at >= ?
),
joined AS (
	SELECT
		COALESCE(NULLIF(scene.scene_tag, ''), 'uncategorized') AS scene_tag,
		cover.cover_score,
		cover.cover_portrait,
		cover.cover_exposure,
		cover.cover_face
	FROM cover
	LEFT JOIN scene ON scene.job_id = cover.job_id
)
SELECT
	scene_tag,
	COUNT(*)::bigint AS samples,
	AVG(cover_score) AS avg_cover_score,
	AVG(cover_portrait) AS avg_cover_portrait,
	AVG(cover_exposure) AS avg_cover_exposure,
	AVG(cover_face) AS avg_cover_face
FROM joined
GROUP BY scene_tag
ORDER BY samples DESC, scene_tag ASC
LIMIT 16
`, models.VideoJobStatusDone, since, models.VideoJobStatusDone, since).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobLiveCoverSceneStat, 0, len(rows))
	for _, item := range rows {
		tag := strings.TrimSpace(item.SceneTag)
		if tag == "" {
			tag = "uncategorized"
		}
		stat := AdminVideoJobLiveCoverSceneStat{
			SceneTag:  tag,
			Samples:   item.Samples,
			LowSample: item.Samples < minSamples,
		}
		if item.AvgCoverScore != nil {
			stat.AvgCoverScore = *item.AvgCoverScore
		}
		if item.AvgCoverPortrait != nil {
			stat.AvgCoverPortrait = *item.AvgCoverPortrait
		}
		if item.AvgCoverExposure != nil {
			stat.AvgCoverExposure = *item.AvgCoverExposure
		}
		if item.AvgCoverFace != nil {
			stat.AvgCoverFace = *item.AvgCoverFace
		}
		out = append(out, stat)
	}
	return out, nil
}

func (h *Handler) loadVideoJobGIFLoopTuneOverview(since time.Time) (AdminVideoJobGIFLoopTuneOverview, error) {
	type row struct {
		Samples          int64    `gorm:"column:samples"`
		Applied          int64    `gorm:"column:applied"`
		EffectiveApplied int64    `gorm:"column:effective_applied"`
		FallbackToBase   int64    `gorm:"column:fallback_to_base"`
		AvgScore         *float64 `gorm:"column:avg_score"`
		AvgLoopClosure   *float64 `gorm:"column:avg_loop_closure"`
		AvgMotionMean    *float64 `gorm:"column:avg_motion_mean"`
		AvgEffectiveSec  *float64 `gorm:"column:avg_effective_sec"`
	}

	var result row
	gifTables := resolveVideoImageReadTables("gif")
	err := h.db.Raw(fmt.Sprintf(`
SELECT
	COUNT(*)::bigint AS samples,
	COUNT(*) FILTER (WHERE o.gif_loop_tune_applied = TRUE)::bigint AS applied,
	COUNT(*) FILTER (WHERE o.gif_loop_tune_effective_applied = TRUE)::bigint AS effective_applied,
	COUNT(*) FILTER (WHERE o.gif_loop_tune_fallback_to_base = TRUE)::bigint AS fallback_to_base,
	AVG(o.gif_loop_tune_score) FILTER (WHERE o.gif_loop_tune_applied = TRUE) AS avg_score,
	AVG(o.gif_loop_tune_loop_closure) FILTER (WHERE o.gif_loop_tune_applied = TRUE) AS avg_loop_closure,
	AVG(o.gif_loop_tune_motion_mean) FILTER (WHERE o.gif_loop_tune_applied = TRUE) AS avg_motion_mean,
	AVG(o.gif_loop_tune_effective_sec) FILTER (WHERE o.gif_loop_tune_applied = TRUE) AS avg_effective_sec
FROM %s o
JOIN %s j ON j.id = o.job_id
WHERE o.format = 'gif'
	AND o.file_role = 'main'
	AND j.status = ?
	AND j.finished_at >= ?
`, gifTables.Outputs, gifTables.Jobs), models.VideoJobStatusDone, since).Scan(&result).Error
	if err != nil {
		if isMissingPublicGIFLoopColumnsError(err) {
			return AdminVideoJobGIFLoopTuneOverview{}, nil
		}
		return AdminVideoJobGIFLoopTuneOverview{}, err
	}

	out := AdminVideoJobGIFLoopTuneOverview{
		Samples:          result.Samples,
		Applied:          result.Applied,
		EffectiveApplied: result.EffectiveApplied,
		FallbackToBase:   result.FallbackToBase,
	}
	if out.Samples > 0 {
		out.AppliedRate = float64(out.Applied) / float64(out.Samples)
		out.EffectiveAppliedRate = float64(out.EffectiveApplied) / float64(out.Samples)
		out.FallbackRate = float64(out.FallbackToBase) / float64(out.Samples)
	}
	if result.AvgScore != nil {
		out.AvgScore = *result.AvgScore
	}
	if result.AvgLoopClosure != nil {
		out.AvgLoopClosure = *result.AvgLoopClosure
	}
	if result.AvgMotionMean != nil {
		out.AvgMotionMean = *result.AvgMotionMean
	}
	if result.AvgEffectiveSec != nil {
		out.AvgEffectiveSec = *result.AvgEffectiveSec
	}
	return out, nil
}

func (h *Handler) loadVideoJobGIFEvaluationOverview(since time.Time) (AdminVideoJobGIFEvaluationOverview, error) {
	type row struct {
		Samples            int64    `gorm:"column:samples"`
		AvgEmotionScore    *float64 `gorm:"column:avg_emotion_score"`
		AvgClarityScore    *float64 `gorm:"column:avg_clarity_score"`
		AvgMotionScore     *float64 `gorm:"column:avg_motion_score"`
		AvgLoopScore       *float64 `gorm:"column:avg_loop_score"`
		AvgEfficiencyScore *float64 `gorm:"column:avg_efficiency_score"`
		AvgOverallScore    *float64 `gorm:"column:avg_overall_score"`
	}
	var result row
	gifTables := resolveVideoImageReadTables("gif")
	if err := h.db.Raw(fmt.Sprintf(`
SELECT
	COUNT(*)::bigint AS samples,
	AVG(e.emotion_score) AS avg_emotion_score,
	AVG(e.clarity_score) AS avg_clarity_score,
	AVG(e.motion_score) AS avg_motion_score,
	AVG(e.loop_score) AS avg_loop_score,
	AVG(e.efficiency_score) AS avg_efficiency_score,
	AVG(e.overall_score) AS avg_overall_score
FROM archive.video_job_gif_evaluations e
JOIN %s j ON j.id = e.job_id
WHERE j.requested_format = 'gif'
	AND e.created_at >= ?
`, gifTables.Jobs), since).Scan(&result).Error; err != nil {
		if isMissingGIFEvaluationTableError(err) {
			return AdminVideoJobGIFEvaluationOverview{}, nil
		}
		return AdminVideoJobGIFEvaluationOverview{}, err
	}
	out := AdminVideoJobGIFEvaluationOverview{
		Samples: result.Samples,
	}
	if result.AvgEmotionScore != nil {
		out.AvgEmotionScore = *result.AvgEmotionScore
	}
	if result.AvgClarityScore != nil {
		out.AvgClarityScore = *result.AvgClarityScore
	}
	if result.AvgMotionScore != nil {
		out.AvgMotionScore = *result.AvgMotionScore
	}
	if result.AvgLoopScore != nil {
		out.AvgLoopScore = *result.AvgLoopScore
	}
	if result.AvgEfficiencyScore != nil {
		out.AvgEfficiencyScore = *result.AvgEfficiencyScore
	}
	if result.AvgOverallScore != nil {
		out.AvgOverallScore = *result.AvgOverallScore
	}
	return out, nil
}

func (h *Handler) loadLatestVideoJobGIFBaselines(limit int) ([]AdminVideoJobGIFBaselineSnapshot, error) {
	if limit <= 0 {
		limit = 7
	}
	type row struct {
		BaselineDate       time.Time `gorm:"column:baseline_date"`
		WindowLabel        string    `gorm:"column:window_label"`
		Scope              string    `gorm:"column:scope"`
		SampleJobs         int64     `gorm:"column:sample_jobs"`
		DoneJobs           int64     `gorm:"column:done_jobs"`
		FailedJobs         int64     `gorm:"column:failed_jobs"`
		DoneRate           float64   `gorm:"column:done_rate"`
		FailedRate         float64   `gorm:"column:failed_rate"`
		SampleOutputs      int64     `gorm:"column:sample_outputs"`
		AvgEmotionScore    float64   `gorm:"column:avg_emotion_score"`
		AvgClarityScore    float64   `gorm:"column:avg_clarity_score"`
		AvgMotionScore     float64   `gorm:"column:avg_motion_score"`
		AvgLoopScore       float64   `gorm:"column:avg_loop_score"`
		AvgEfficiencyScore float64   `gorm:"column:avg_efficiency_score"`
		AvgOverallScore    float64   `gorm:"column:avg_overall_score"`
	}
	var rows []row
	if err := h.db.Raw(`
SELECT
	baseline_date,
	window_label,
	scope,
	sample_jobs,
	done_jobs,
	failed_jobs,
	done_rate,
	failed_rate,
	sample_outputs,
	avg_emotion_score,
	avg_clarity_score,
	avg_motion_score,
	avg_loop_score,
	avg_efficiency_score,
	avg_overall_score
FROM ops.video_job_gif_baselines
WHERE requested_format = 'gif'
ORDER BY baseline_date DESC, id DESC
LIMIT ?
`, limit).Scan(&rows).Error; err != nil {
		if isMissingGIFBaselineTableError(err) {
			return []AdminVideoJobGIFBaselineSnapshot{}, nil
		}
		return nil, err
	}
	out := make([]AdminVideoJobGIFBaselineSnapshot, 0, len(rows))
	for _, item := range rows {
		out = append(out, AdminVideoJobGIFBaselineSnapshot{
			BaselineDate:       item.BaselineDate.Format("2006-01-02"),
			WindowLabel:        strings.TrimSpace(item.WindowLabel),
			Scope:              strings.TrimSpace(item.Scope),
			SampleJobs:         item.SampleJobs,
			DoneJobs:           item.DoneJobs,
			FailedJobs:         item.FailedJobs,
			DoneRate:           item.DoneRate,
			FailedRate:         item.FailedRate,
			SampleOutputs:      item.SampleOutputs,
			AvgEmotionScore:    item.AvgEmotionScore,
			AvgClarityScore:    item.AvgClarityScore,
			AvgMotionScore:     item.AvgMotionScore,
			AvgLoopScore:       item.AvgLoopScore,
			AvgEfficiencyScore: item.AvgEfficiencyScore,
			AvgOverallScore:    item.AvgOverallScore,
		})
	}
	return out, nil
}

func (h *Handler) loadVideoJobGIFEvaluationSamples(
	since time.Time,
	limit int,
	ascending bool,
) ([]AdminVideoJobGIFEvaluationSample, error) {
	if limit <= 0 {
		limit = 5
	}
	type row struct {
		JobID           uint64         `gorm:"column:job_id"`
		OutputID        *uint64        `gorm:"column:output_id"`
		ObjectKey       string         `gorm:"column:object_key"`
		WindowStartMs   int            `gorm:"column:window_start_ms"`
		WindowEndMs     int            `gorm:"column:window_end_ms"`
		OverallScore    float64        `gorm:"column:overall_score"`
		EmotionScore    float64        `gorm:"column:emotion_score"`
		ClarityScore    float64        `gorm:"column:clarity_score"`
		MotionScore     float64        `gorm:"column:motion_score"`
		LoopScore       float64        `gorm:"column:loop_score"`
		EfficiencyScore float64        `gorm:"column:efficiency_score"`
		FeatureJSON     datatypes.JSON `gorm:"column:feature_json"`
		SizeBytes       int64          `gorm:"column:size_bytes"`
		Width           int            `gorm:"column:width"`
		Height          int            `gorm:"column:height"`
		DurationMs      int            `gorm:"column:duration_ms"`
		CreatedAt       time.Time      `gorm:"column:created_at"`
	}
	orderExpr := "e.overall_score DESC, e.id DESC"
	if ascending {
		orderExpr = "e.overall_score ASC, e.id ASC"
	}
	gifTables := resolveVideoImageReadTables("gif")

	var rows []row
	if err := h.db.Raw(fmt.Sprintf(`
SELECT
	e.job_id,
	e.output_id,
	o.object_key,
	e.window_start_ms,
	e.window_end_ms,
	e.overall_score,
	e.emotion_score,
	e.clarity_score,
	e.motion_score,
	e.loop_score,
	e.efficiency_score,
	e.feature_json,
	o.size_bytes,
	o.width,
	o.height,
	o.duration_ms,
	e.created_at
FROM archive.video_job_gif_evaluations e
JOIN %s o ON o.id = e.output_id
JOIN %s j ON j.id = e.job_id
WHERE j.requested_format = 'gif'
	AND e.created_at >= ?
ORDER BY %s
LIMIT ?
`, gifTables.Outputs, gifTables.Jobs, orderExpr), since, limit).Scan(&rows).Error; err != nil {
		if isMissingGIFEvaluationTableError(err) {
			return []AdminVideoJobGIFEvaluationSample{}, nil
		}
		return nil, err
	}

	out := make([]AdminVideoJobGIFEvaluationSample, 0, len(rows))
	for _, item := range rows {
		outputID := uint64(0)
		if item.OutputID != nil {
			outputID = *item.OutputID
		}
		feature := parseJSONMap(item.FeatureJSON)
		reason := ""
		if raw, ok := feature["candidate_feature"].(map[string]interface{}); ok {
			reason = strings.TrimSpace(strings.ToLower(fmt.Sprint(raw["reason"])))
		}
		if reason == "" {
			reason = strings.TrimSpace(strings.ToLower(fmt.Sprint(feature["reason"])))
		}
		out = append(out, AdminVideoJobGIFEvaluationSample{
			JobID:           item.JobID,
			OutputID:        outputID,
			PreviewURL:      resolvePreviewURL(strings.TrimSpace(item.ObjectKey), h.qiniu),
			ObjectKey:       strings.TrimSpace(item.ObjectKey),
			WindowStartMs:   item.WindowStartMs,
			WindowEndMs:     item.WindowEndMs,
			OverallScore:    item.OverallScore,
			EmotionScore:    item.EmotionScore,
			ClarityScore:    item.ClarityScore,
			MotionScore:     item.MotionScore,
			LoopScore:       item.LoopScore,
			EfficiencyScore: item.EfficiencyScore,
			CandidateReason: reason,
			SizeBytes:       item.SizeBytes,
			Width:           item.Width,
			Height:          item.Height,
			DurationMs:      item.DurationMs,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func (h *Handler) loadVideoJobGIFManualScoreOverview(since time.Time) (AdminVideoJobGIFManualScoreOverview, error) {
	type row struct {
		Samples             int64    `gorm:"column:samples"`
		WithOutputID        int64    `gorm:"column:with_output_id"`
		MatchedEvaluations  int64    `gorm:"column:matched_evaluations"`
		TopPickRate         *float64 `gorm:"column:top_pick_rate"`
		PassRate            *float64 `gorm:"column:pass_rate"`
		AvgManualEmotion    *float64 `gorm:"column:avg_manual_emotion"`
		AvgManualClarity    *float64 `gorm:"column:avg_manual_clarity"`
		AvgManualMotion     *float64 `gorm:"column:avg_manual_motion"`
		AvgManualLoop       *float64 `gorm:"column:avg_manual_loop"`
		AvgManualEfficiency *float64 `gorm:"column:avg_manual_efficiency"`
		AvgManualOverall    *float64 `gorm:"column:avg_manual_overall"`
		AvgAutoEmotion      *float64 `gorm:"column:avg_auto_emotion"`
		AvgAutoClarity      *float64 `gorm:"column:avg_auto_clarity"`
		AvgAutoMotion       *float64 `gorm:"column:avg_auto_motion"`
		AvgAutoLoop         *float64 `gorm:"column:avg_auto_loop"`
		AvgAutoEfficiency   *float64 `gorm:"column:avg_auto_efficiency"`
		AvgAutoOverall      *float64 `gorm:"column:avg_auto_overall"`
		MAEEmotion          *float64 `gorm:"column:mae_emotion"`
		MAEClarity          *float64 `gorm:"column:mae_clarity"`
		MAEMotion           *float64 `gorm:"column:mae_motion"`
		MAELoop             *float64 `gorm:"column:mae_loop"`
		MAEEfficiency       *float64 `gorm:"column:mae_efficiency"`
		MAEOverall          *float64 `gorm:"column:mae_overall"`
		AvgOverallDelta     *float64 `gorm:"column:avg_overall_delta"`
	}
	var result row
	if err := h.db.Raw(`
SELECT
	COUNT(*)::bigint AS samples,
	COUNT(*) FILTER (WHERE m.output_id IS NOT NULL)::bigint AS with_output_id,
	COUNT(*) FILTER (WHERE e.id IS NOT NULL)::bigint AS matched_evaluations,
	AVG(CASE WHEN m.is_top_pick THEN 1.0 ELSE 0.0 END) AS top_pick_rate,
	AVG(CASE WHEN m.is_pass THEN 1.0 ELSE 0.0 END) AS pass_rate,
	AVG(m.emotion_score) AS avg_manual_emotion,
	AVG(m.clarity_score) AS avg_manual_clarity,
	AVG(m.motion_score) AS avg_manual_motion,
	AVG(m.loop_score) AS avg_manual_loop,
	AVG(m.efficiency_score) AS avg_manual_efficiency,
	AVG(m.overall_score) AS avg_manual_overall,
	AVG(e.emotion_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_emotion,
	AVG(e.clarity_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_clarity,
	AVG(e.motion_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_motion,
	AVG(e.loop_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_loop,
	AVG(e.efficiency_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_efficiency,
	AVG(e.overall_score) FILTER (WHERE e.id IS NOT NULL) AS avg_auto_overall,
	AVG(ABS(m.emotion_score - e.emotion_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_emotion,
	AVG(ABS(m.clarity_score - e.clarity_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_clarity,
	AVG(ABS(m.motion_score - e.motion_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_motion,
	AVG(ABS(m.loop_score - e.loop_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_loop,
	AVG(ABS(m.efficiency_score - e.efficiency_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_efficiency,
	AVG(ABS(m.overall_score - e.overall_score)) FILTER (WHERE e.id IS NOT NULL) AS mae_overall,
	AVG(m.overall_score - e.overall_score) FILTER (WHERE e.id IS NOT NULL) AS avg_overall_delta
FROM ops.video_job_gif_manual_scores m
LEFT JOIN archive.video_job_gif_evaluations e ON e.output_id = m.output_id
WHERE m.reviewed_at >= ?
`, since).Scan(&result).Error; err != nil {
		if isMissingGIFManualScoreTableError(err) {
			return AdminVideoJobGIFManualScoreOverview{}, nil
		}
		return AdminVideoJobGIFManualScoreOverview{}, err
	}

	out := AdminVideoJobGIFManualScoreOverview{
		Samples:            result.Samples,
		WithOutputID:       result.WithOutputID,
		MatchedEvaluations: result.MatchedEvaluations,
	}
	if out.Samples > 0 {
		out.MatchedRate = float64(out.MatchedEvaluations) / float64(out.Samples)
	}
	if result.TopPickRate != nil {
		out.TopPickRate = *result.TopPickRate
	}
	if result.PassRate != nil {
		out.PassRate = *result.PassRate
	}
	if result.AvgManualEmotion != nil {
		out.AvgManualEmotion = *result.AvgManualEmotion
	}
	if result.AvgManualClarity != nil {
		out.AvgManualClarity = *result.AvgManualClarity
	}
	if result.AvgManualMotion != nil {
		out.AvgManualMotion = *result.AvgManualMotion
	}
	if result.AvgManualLoop != nil {
		out.AvgManualLoop = *result.AvgManualLoop
	}
	if result.AvgManualEfficiency != nil {
		out.AvgManualEfficiency = *result.AvgManualEfficiency
	}
	if result.AvgManualOverall != nil {
		out.AvgManualOverall = *result.AvgManualOverall
	}
	if result.AvgAutoEmotion != nil {
		out.AvgAutoEmotion = *result.AvgAutoEmotion
	}
	if result.AvgAutoClarity != nil {
		out.AvgAutoClarity = *result.AvgAutoClarity
	}
	if result.AvgAutoMotion != nil {
		out.AvgAutoMotion = *result.AvgAutoMotion
	}
	if result.AvgAutoLoop != nil {
		out.AvgAutoLoop = *result.AvgAutoLoop
	}
	if result.AvgAutoEfficiency != nil {
		out.AvgAutoEfficiency = *result.AvgAutoEfficiency
	}
	if result.AvgAutoOverall != nil {
		out.AvgAutoOverall = *result.AvgAutoOverall
	}
	if result.MAEEmotion != nil {
		out.MAEEmotion = *result.MAEEmotion
	}
	if result.MAEClarity != nil {
		out.MAEClarity = *result.MAEClarity
	}
	if result.MAEMotion != nil {
		out.MAEMotion = *result.MAEMotion
	}
	if result.MAELoop != nil {
		out.MAELoop = *result.MAELoop
	}
	if result.MAEEfficiency != nil {
		out.MAEEfficiency = *result.MAEEfficiency
	}
	if result.MAEOverall != nil {
		out.MAEOverall = *result.MAEOverall
	}
	if result.AvgOverallDelta != nil {
		out.AvgOverallDelta = *result.AvgOverallDelta
	}
	return out, nil
}

func (h *Handler) loadVideoJobGIFManualScoreDiffSamples(
	since time.Time,
	limit int,
) ([]AdminVideoJobGIFManualScoreDiffSample, error) {
	if limit <= 0 {
		limit = 8
	}
	type row struct {
		SampleID            string    `gorm:"column:sample_id"`
		BaselineVersion     string    `gorm:"column:baseline_version"`
		ReviewRound         string    `gorm:"column:review_round"`
		Reviewer            string    `gorm:"column:reviewer"`
		JobID               uint64    `gorm:"column:job_id"`
		OutputID            uint64    `gorm:"column:output_id"`
		ObjectKey           string    `gorm:"column:object_key"`
		ManualOverallScore  float64   `gorm:"column:manual_overall_score"`
		AutoOverallScore    float64   `gorm:"column:auto_overall_score"`
		OverallScoreDelta   float64   `gorm:"column:overall_score_delta"`
		AbsOverallScoreDiff float64   `gorm:"column:abs_overall_score_diff"`
		ManualLoopScore     float64   `gorm:"column:manual_loop_score"`
		AutoLoopScore       float64   `gorm:"column:auto_loop_score"`
		LoopScoreDelta      float64   `gorm:"column:loop_score_delta"`
		ManualClarityScore  float64   `gorm:"column:manual_clarity_score"`
		AutoClarityScore    float64   `gorm:"column:auto_clarity_score"`
		ClarityScoreDelta   float64   `gorm:"column:clarity_score_delta"`
		IsTopPick           bool      `gorm:"column:is_top_pick"`
		IsPass              bool      `gorm:"column:is_pass"`
		RejectReason        string    `gorm:"column:reject_reason"`
		ReviewedAt          time.Time `gorm:"column:reviewed_at"`
	}
	rows := make([]row, 0, limit)
	gifTables := resolveVideoImageReadTables("gif")
	if err := h.db.Raw(fmt.Sprintf(`
SELECT
	m.sample_id,
	m.baseline_version,
	m.review_round,
	m.reviewer,
	COALESCE(m.job_id, e.job_id, 0)::bigint AS job_id,
	COALESCE(m.output_id, 0)::bigint AS output_id,
	COALESCE(o.object_key, '') AS object_key,
	m.overall_score AS manual_overall_score,
	e.overall_score AS auto_overall_score,
	(m.overall_score - e.overall_score) AS overall_score_delta,
	ABS(m.overall_score - e.overall_score) AS abs_overall_score_diff,
	m.loop_score AS manual_loop_score,
	e.loop_score AS auto_loop_score,
	(m.loop_score - e.loop_score) AS loop_score_delta,
	m.clarity_score AS manual_clarity_score,
	e.clarity_score AS auto_clarity_score,
	(m.clarity_score - e.clarity_score) AS clarity_score_delta,
	m.is_top_pick,
	m.is_pass,
	m.reject_reason,
	m.reviewed_at
FROM ops.video_job_gif_manual_scores m
JOIN archive.video_job_gif_evaluations e ON e.output_id = m.output_id
LEFT JOIN %s o ON o.id = m.output_id
WHERE m.reviewed_at >= ?
ORDER BY ABS(m.overall_score - e.overall_score) DESC, m.reviewed_at DESC
LIMIT ?
`, gifTables.Outputs), since, limit).Scan(&rows).Error; err != nil {
		if isMissingGIFManualScoreTableError(err) || isMissingGIFEvaluationTableError(err) {
			return []AdminVideoJobGIFManualScoreDiffSample{}, nil
		}
		return nil, err
	}

	out := make([]AdminVideoJobGIFManualScoreDiffSample, 0, len(rows))
	for _, item := range rows {
		out = append(out, AdminVideoJobGIFManualScoreDiffSample{
			SampleID:            strings.TrimSpace(item.SampleID),
			BaselineVersion:     strings.TrimSpace(item.BaselineVersion),
			ReviewRound:         strings.TrimSpace(item.ReviewRound),
			Reviewer:            strings.TrimSpace(item.Reviewer),
			JobID:               item.JobID,
			OutputID:            item.OutputID,
			PreviewURL:          resolvePreviewURL(strings.TrimSpace(item.ObjectKey), h.qiniu),
			ObjectKey:           strings.TrimSpace(item.ObjectKey),
			ManualOverallScore:  item.ManualOverallScore,
			AutoOverallScore:    item.AutoOverallScore,
			OverallScoreDelta:   item.OverallScoreDelta,
			AbsOverallScoreDiff: item.AbsOverallScoreDiff,
			ManualLoopScore:     item.ManualLoopScore,
			AutoLoopScore:       item.AutoLoopScore,
			LoopScoreDelta:      item.LoopScoreDelta,
			ManualClarityScore:  item.ManualClarityScore,
			AutoClarityScore:    item.AutoClarityScore,
			ClarityScoreDelta:   item.ClarityScoreDelta,
			IsTopPick:           item.IsTopPick,
			IsPass:              item.IsPass,
			RejectReason:        strings.TrimSpace(strings.ToLower(item.RejectReason)),
			ReviewedAt:          item.ReviewedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func isMissingGIFEvaluationTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "video_job_gif_evaluations") && strings.Contains(msg, "does not exist")
}

func isMissingGIFBaselineTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "video_job_gif_baselines") && strings.Contains(msg, "does not exist")
}

func isMissingGIFManualScoreTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "video_job_gif_manual_scores") && strings.Contains(msg, "does not exist")
}

func isMissingPublicGIFLoopColumnsError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "public.video_image_outputs") && strings.Contains(msg, "does not exist") {
		return true
	}
	if strings.Contains(msg, "gif_loop_tune_") && strings.Contains(msg, "does not exist") {
		return true
	}
	return false
}
