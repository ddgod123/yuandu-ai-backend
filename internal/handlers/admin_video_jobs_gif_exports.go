package handlers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type adminVideoJobGIFRerankOverview struct {
	Samples         int64   `gorm:"column:samples"`
	PositiveSamples int64   `gorm:"column:positive_samples"`
	NegativeSamples int64   `gorm:"column:negative_samples"`
	AvgScoreDelta   float64 `gorm:"column:avg_score_delta"`
}

func parseCSVLimit(raw string, defaultValue, maxValue int) (int, error) {
	if defaultValue <= 0 {
		defaultValue = 1000
	}
	if maxValue <= 0 || maxValue < defaultValue {
		maxValue = defaultValue
	}
	if strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 || value > maxValue {
		return 0, fmt.Errorf("invalid limit, expected 1..%d", maxValue)
	}
	return value, nil
}

func parseCSVSortDirection(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "desc":
		return "DESC", nil
	case "asc":
		return "ASC", nil
	default:
		return "", fmt.Errorf("invalid order, expected asc or desc")
	}
}

func formatFilenameWindowSuffix(windowLabel string) string {
	label := strings.TrimSpace(strings.ToLower(windowLabel))
	if label == "" {
		return "24h"
	}
	return label
}

func safeUint64Text(raw *uint64) string {
	if raw == nil || *raw == 0 {
		return ""
	}
	return strconv.FormatUint(*raw, 10)
}

func safeTimeText(raw time.Time) string {
	if raw.IsZero() {
		return ""
	}
	return raw.Format(time.RFC3339)
}

func resolveCandidateReason(featureJSON datatypes.JSON) string {
	feature := parseJSONMap(featureJSON)
	if nested, ok := feature["candidate_feature"].(map[string]interface{}); ok {
		if reason := normalizeMaybeNilText(nested["reason"]); reason != "" {
			return reason
		}
	}
	if reason := normalizeMaybeNilText(feature["reason"]); reason != "" {
		return reason
	}
	return ""
}

func normalizeMaybeNilText(raw interface{}) string {
	text := strings.TrimSpace(strings.ToLower(fmt.Sprint(raw)))
	if text == "" || text == "<nil>" || text == "null" {
		return ""
	}
	return text
}

func isMissingGIFRerankTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "video_job_gif_rerank_logs") && strings.Contains(msg, "does not exist")
}

func (h *Handler) loadGIFRerankOverviewSince(since time.Time) (adminVideoJobGIFRerankOverview, error) {
	var row adminVideoJobGIFRerankOverview
	if err := h.db.Raw(`
SELECT
	COUNT(*)::bigint AS samples,
	COUNT(*) FILTER (WHERE score_delta > 0)::bigint AS positive_samples,
	COUNT(*) FILTER (WHERE score_delta < 0)::bigint AS negative_samples,
	COALESCE(AVG(score_delta), 0) AS avg_score_delta
FROM ops.video_job_gif_rerank_logs
WHERE created_at >= ?
`, since).Scan(&row).Error; err != nil {
		if isMissingGIFRerankTableError(err) {
			return adminVideoJobGIFRerankOverview{}, nil
		}
		return adminVideoJobGIFRerankOverview{}, err
	}
	return row, nil
}

// ExportAdminVideoJobsGIFEvaluationsCSV godoc
// @Summary Export gif evaluations CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param limit query int false "rows limit, default 2000, max 20000"
// @Param order query string false "overall score order: desc | asc"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/gif-evaluations.csv [get]
func (h *Handler) ExportAdminVideoJobsGIFEvaluationsCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit, err := parseCSVLimit(c.Query("limit"), 2000, 20000)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sortDirection, err := parseCSVSortDirection(c.Query("order"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	since := time.Now().Add(-windowDuration)
	gifTables := resolveVideoImageReadTables("gif")
	orderClause := fmt.Sprintf("e.overall_score %s, e.id DESC", sortDirection)
	type row struct {
		ID              uint64         `gorm:"column:id"`
		JobID           uint64         `gorm:"column:job_id"`
		UserID          uint64         `gorm:"column:user_id"`
		OutputID        *uint64        `gorm:"column:output_id"`
		CandidateID     *uint64        `gorm:"column:candidate_id"`
		WindowStartMs   int            `gorm:"column:window_start_ms"`
		WindowEndMs     int            `gorm:"column:window_end_ms"`
		EmotionScore    float64        `gorm:"column:emotion_score"`
		ClarityScore    float64        `gorm:"column:clarity_score"`
		MotionScore     float64        `gorm:"column:motion_score"`
		LoopScore       float64        `gorm:"column:loop_score"`
		EfficiencyScore float64        `gorm:"column:efficiency_score"`
		OverallScore    float64        `gorm:"column:overall_score"`
		FeatureJSON     datatypes.JSON `gorm:"column:feature_json"`
		ObjectKey       string         `gorm:"column:object_key"`
		SizeBytes       int64          `gorm:"column:size_bytes"`
		Width           int            `gorm:"column:width"`
		Height          int            `gorm:"column:height"`
		DurationMs      int            `gorm:"column:duration_ms"`
		CreatedAt       time.Time      `gorm:"column:created_at"`
	}
	var rows []row
	query := fmt.Sprintf(`
SELECT
	e.id,
	e.job_id,
	j.user_id,
	e.output_id,
	e.candidate_id,
	e.window_start_ms,
	e.window_end_ms,
	e.emotion_score,
	e.clarity_score,
	e.motion_score,
	e.loop_score,
	e.efficiency_score,
	e.overall_score,
	e.feature_json,
	COALESCE(o.object_key, '') AS object_key,
	COALESCE(o.size_bytes, 0) AS size_bytes,
	COALESCE(o.width, 0) AS width,
	COALESCE(o.height, 0) AS height,
	COALESCE(o.duration_ms, 0) AS duration_ms,
	e.created_at
FROM archive.video_job_gif_evaluations e
JOIN %s j ON j.id = e.job_id
LEFT JOIN %s o ON o.id = e.output_id
WHERE j.requested_format = 'gif'
	AND e.created_at >= ?
ORDER BY %s
LIMIT ?
`, gifTables.Jobs, gifTables.Outputs, orderClause)
	if err := h.db.Raw(query, since, limit).Scan(&rows).Error; err != nil {
		if !isMissingGIFEvaluationTableError(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		rows = []row{}
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"id",
		"window",
		"window_start",
		"window_end",
		"job_id",
		"user_id",
		"output_id",
		"candidate_id",
		"window_start_ms",
		"window_end_ms",
		"emotion_score",
		"clarity_score",
		"motion_score",
		"loop_score",
		"efficiency_score",
		"overall_score",
		"candidate_reason",
		"object_key",
		"size_bytes",
		"width",
		"height",
		"duration_ms",
		"created_at",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	windowStartText := since.Format(time.RFC3339)
	windowEndText := time.Now().Format(time.RFC3339)
	for _, item := range rows {
		if err := writer.Write([]string{
			strconv.FormatUint(item.ID, 10),
			windowLabel,
			windowStartText,
			windowEndText,
			strconv.FormatUint(item.JobID, 10),
			strconv.FormatUint(item.UserID, 10),
			safeUint64Text(item.OutputID),
			safeUint64Text(item.CandidateID),
			strconv.Itoa(item.WindowStartMs),
			strconv.Itoa(item.WindowEndMs),
			fmt.Sprintf("%.6f", item.EmotionScore),
			fmt.Sprintf("%.6f", item.ClarityScore),
			fmt.Sprintf("%.6f", item.MotionScore),
			fmt.Sprintf("%.6f", item.LoopScore),
			fmt.Sprintf("%.6f", item.EfficiencyScore),
			fmt.Sprintf("%.6f", item.OverallScore),
			resolveCandidateReason(item.FeatureJSON),
			strings.TrimSpace(item.ObjectKey),
			strconv.FormatInt(item.SizeBytes, 10),
			strconv.Itoa(item.Width),
			strconv.Itoa(item.Height),
			strconv.Itoa(item.DurationMs),
			safeTimeText(item.CreatedAt),
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("video_jobs_gif_evaluations_%s_%s.csv", formatFilenameWindowSuffix(windowLabel), time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAdminVideoJobsGIFRerankLogsCSV godoc
// @Summary Export gif rerank logs CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param limit query int false "rows limit, default 3000, max 20000"
// @Param min_abs_delta query number false "minimum absolute score_delta filter, default 0"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/gif-rerank-logs.csv [get]
func (h *Handler) ExportAdminVideoJobsGIFRerankLogsCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit, err := parseCSVLimit(c.Query("limit"), 3000, 20000)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	minAbsDelta := 0.0
	if raw := strings.TrimSpace(c.Query("min_abs_delta")); raw != "" {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil || value < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid min_abs_delta"})
			return
		}
		minAbsDelta = value
	}

	since := time.Now().Add(-windowDuration)
	type row struct {
		ID          uint64         `gorm:"column:id"`
		JobID       uint64         `gorm:"column:job_id"`
		UserID      uint64         `gorm:"column:user_id"`
		CandidateID *uint64        `gorm:"column:candidate_id"`
		StartMs     int            `gorm:"column:start_ms"`
		EndMs       int            `gorm:"column:end_ms"`
		BeforeRank  int            `gorm:"column:before_rank"`
		AfterRank   int            `gorm:"column:after_rank"`
		BeforeScore float64        `gorm:"column:before_score"`
		AfterScore  float64        `gorm:"column:after_score"`
		ScoreDelta  float64        `gorm:"column:score_delta"`
		Reason      string         `gorm:"column:reason"`
		Metadata    datatypes.JSON `gorm:"column:metadata"`
		CreatedAt   time.Time      `gorm:"column:created_at"`
	}
	var rows []row
	if err := h.db.Raw(`
SELECT
	id,
	job_id,
	user_id,
	candidate_id,
	start_ms,
	end_ms,
	before_rank,
	after_rank,
	before_score,
	after_score,
	score_delta,
	reason,
	metadata,
	created_at
FROM ops.video_job_gif_rerank_logs
WHERE created_at >= ?
	AND ABS(score_delta) >= ?
ORDER BY created_at DESC, id DESC
LIMIT ?
`, since, minAbsDelta, limit).Scan(&rows).Error; err != nil {
		if !isMissingGIFRerankTableError(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		rows = []row{}
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"id",
		"window",
		"window_start",
		"window_end",
		"job_id",
		"user_id",
		"candidate_id",
		"start_ms",
		"end_ms",
		"before_rank",
		"after_rank",
		"before_score",
		"after_score",
		"score_delta",
		"abs_score_delta",
		"reason",
		"metadata_json",
		"created_at",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	windowStartText := since.Format(time.RFC3339)
	windowEndText := time.Now().Format(time.RFC3339)
	for _, item := range rows {
		if err := writer.Write([]string{
			strconv.FormatUint(item.ID, 10),
			windowLabel,
			windowStartText,
			windowEndText,
			strconv.FormatUint(item.JobID, 10),
			strconv.FormatUint(item.UserID, 10),
			safeUint64Text(item.CandidateID),
			strconv.Itoa(item.StartMs),
			strconv.Itoa(item.EndMs),
			strconv.Itoa(item.BeforeRank),
			strconv.Itoa(item.AfterRank),
			fmt.Sprintf("%.6f", item.BeforeScore),
			fmt.Sprintf("%.6f", item.AfterScore),
			fmt.Sprintf("%.6f", item.ScoreDelta),
			fmt.Sprintf("%.6f", absFloat(item.ScoreDelta)),
			strings.TrimSpace(item.Reason),
			strings.TrimSpace(string(item.Metadata)),
			safeTimeText(item.CreatedAt),
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("video_jobs_gif_rerank_logs_%s_%s.csv", formatFilenameWindowSuffix(windowLabel), time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAdminVideoJobsGIFBaselinesCSV godoc
// @Summary Export gif baselines CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param limit query int false "rows limit, default 90, max 365"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/gif-baselines.csv [get]
func (h *Handler) ExportAdminVideoJobsGIFBaselinesCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit, err := parseCSVLimit(c.Query("limit"), 90, 365)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sinceDate := time.Now().UTC().Add(-windowDuration).Truncate(24 * time.Hour)
	type row struct {
		ID                 uint64    `gorm:"column:id"`
		BaselineDate       time.Time `gorm:"column:baseline_date"`
		WindowLabel        string    `gorm:"column:window_label"`
		Scope              string    `gorm:"column:scope"`
		RequestedFormat    string    `gorm:"column:requested_format"`
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
		AvgOutputScore     float64   `gorm:"column:avg_output_score"`
		AvgLoopClosure     float64   `gorm:"column:avg_loop_closure"`
		AvgSizeBytes       float64   `gorm:"column:avg_size_bytes"`
		CreatedAt          time.Time `gorm:"column:created_at"`
		UpdatedAt          time.Time `gorm:"column:updated_at"`
	}
	var rows []row
	if err := h.db.Raw(`
SELECT
	id,
	baseline_date,
	window_label,
	scope,
	requested_format,
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
	avg_overall_score,
	avg_output_score,
	avg_loop_closure,
	avg_size_bytes,
	created_at,
	updated_at
FROM ops.video_job_gif_baselines
WHERE requested_format = 'gif'
	AND baseline_date >= ?
ORDER BY baseline_date DESC, id DESC
LIMIT ?
`, sinceDate, limit).Scan(&rows).Error; err != nil {
		if !isMissingGIFBaselineTableError(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		rows = []row{}
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"id",
		"window",
		"window_start_date",
		"window_end_date",
		"baseline_date",
		"window_label",
		"scope",
		"requested_format",
		"sample_jobs",
		"done_jobs",
		"failed_jobs",
		"done_rate",
		"failed_rate",
		"sample_outputs",
		"avg_emotion_score",
		"avg_clarity_score",
		"avg_motion_score",
		"avg_loop_score",
		"avg_efficiency_score",
		"avg_overall_score",
		"avg_output_score",
		"avg_loop_closure",
		"avg_size_bytes",
		"created_at",
		"updated_at",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	windowStartDateText := sinceDate.Format("2006-01-02")
	windowEndDateText := time.Now().UTC().Format("2006-01-02")
	for _, item := range rows {
		if err := writer.Write([]string{
			strconv.FormatUint(item.ID, 10),
			windowLabel,
			windowStartDateText,
			windowEndDateText,
			item.BaselineDate.Format("2006-01-02"),
			strings.TrimSpace(item.WindowLabel),
			strings.TrimSpace(item.Scope),
			strings.TrimSpace(item.RequestedFormat),
			strconv.FormatInt(item.SampleJobs, 10),
			strconv.FormatInt(item.DoneJobs, 10),
			strconv.FormatInt(item.FailedJobs, 10),
			fmt.Sprintf("%.6f", item.DoneRate),
			fmt.Sprintf("%.6f", item.FailedRate),
			strconv.FormatInt(item.SampleOutputs, 10),
			fmt.Sprintf("%.6f", item.AvgEmotionScore),
			fmt.Sprintf("%.6f", item.AvgClarityScore),
			fmt.Sprintf("%.6f", item.AvgMotionScore),
			fmt.Sprintf("%.6f", item.AvgLoopScore),
			fmt.Sprintf("%.6f", item.AvgEfficiencyScore),
			fmt.Sprintf("%.6f", item.AvgOverallScore),
			fmt.Sprintf("%.6f", item.AvgOutputScore),
			fmt.Sprintf("%.6f", item.AvgLoopClosure),
			fmt.Sprintf("%.2f", item.AvgSizeBytes),
			safeTimeText(item.CreatedAt),
			safeTimeText(item.UpdatedAt),
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("video_jobs_gif_baselines_%s_%s.csv", formatFilenameWindowSuffix(windowLabel), time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAdminVideoJobsGIFQualityReportCSV godoc
// @Summary Export gif quality report CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/gif-quality-report.csv [get]
func (h *Handler) ExportAdminVideoJobsGIFQualityReportCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	since := now.Add(-windowDuration)

	evaluationOverview, err := h.loadVideoJobGIFEvaluationOverview(since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	loopOverview, err := h.loadVideoJobGIFLoopTuneOverview(since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	baselineSnapshots, err := h.loadLatestVideoJobGIFBaselines(14)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	topSamples, err := h.loadVideoJobGIFEvaluationSamples(since, 5, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	lowSamples, err := h.loadVideoJobGIFEvaluationSamples(since, 5, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rerankOverview, err := h.loadGIFRerankOverviewSince(since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"section", "metric", "value", "extra_1", "extra_2", "extra_3", "extra_4"}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	write := func(row ...string) error {
		return writer.Write(row)
	}
	if err := write("meta", "window", windowLabel, since.Format(time.RFC3339), now.Format(time.RFC3339), "", ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	evalMetrics := []struct {
		metric string
		value  string
	}{
		{"samples", strconv.FormatInt(evaluationOverview.Samples, 10)},
		{"avg_emotion_score", fmt.Sprintf("%.6f", evaluationOverview.AvgEmotionScore)},
		{"avg_clarity_score", fmt.Sprintf("%.6f", evaluationOverview.AvgClarityScore)},
		{"avg_motion_score", fmt.Sprintf("%.6f", evaluationOverview.AvgMotionScore)},
		{"avg_loop_score", fmt.Sprintf("%.6f", evaluationOverview.AvgLoopScore)},
		{"avg_efficiency_score", fmt.Sprintf("%.6f", evaluationOverview.AvgEfficiencyScore)},
		{"avg_overall_score", fmt.Sprintf("%.6f", evaluationOverview.AvgOverallScore)},
	}
	for _, item := range evalMetrics {
		if err := write("evaluation_overview", item.metric, item.value, "", "", "", ""); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	loopMetrics := []struct {
		metric string
		value  string
	}{
		{"samples", strconv.FormatInt(loopOverview.Samples, 10)},
		{"applied_rate", fmt.Sprintf("%.6f", loopOverview.AppliedRate)},
		{"effective_applied_rate", fmt.Sprintf("%.6f", loopOverview.EffectiveAppliedRate)},
		{"fallback_rate", fmt.Sprintf("%.6f", loopOverview.FallbackRate)},
		{"avg_score", fmt.Sprintf("%.6f", loopOverview.AvgScore)},
		{"avg_loop_closure", fmt.Sprintf("%.6f", loopOverview.AvgLoopClosure)},
		{"avg_motion_mean", fmt.Sprintf("%.6f", loopOverview.AvgMotionMean)},
		{"avg_effective_sec", fmt.Sprintf("%.6f", loopOverview.AvgEffectiveSec)},
	}
	for _, item := range loopMetrics {
		if err := write("loop_overview", item.metric, item.value, "", "", "", ""); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	rerankMetrics := []struct {
		metric string
		value  string
	}{
		{"samples", strconv.FormatInt(rerankOverview.Samples, 10)},
		{"positive_samples", strconv.FormatInt(rerankOverview.PositiveSamples, 10)},
		{"negative_samples", strconv.FormatInt(rerankOverview.NegativeSamples, 10)},
		{"avg_score_delta", fmt.Sprintf("%.6f", rerankOverview.AvgScoreDelta)},
	}
	for _, item := range rerankMetrics {
		if err := write("rerank_overview", item.metric, item.value, "", "", "", ""); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	for _, item := range baselineSnapshots {
		if err := write(
			"baseline_snapshot",
			item.BaselineDate,
			fmt.Sprintf("%.6f", item.AvgOverallScore),
			fmt.Sprintf("%.6f", item.AvgLoopScore),
			fmt.Sprintf("%.6f", item.AvgClarityScore),
			strconv.FormatInt(item.SampleJobs, 10),
			strconv.FormatInt(item.SampleOutputs, 10),
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	for _, item := range topSamples {
		if err := write(
			"top_sample",
			strconv.FormatUint(item.JobID, 10),
			fmt.Sprintf("%.6f", item.OverallScore),
			strconv.FormatUint(item.OutputID, 10),
			formatWindowRangeMs(item.WindowStartMs, item.WindowEndMs),
			item.CandidateReason,
			item.CreatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	for _, item := range lowSamples {
		if err := write(
			"low_sample",
			strconv.FormatUint(item.JobID, 10),
			fmt.Sprintf("%.6f", item.OverallScore),
			strconv.FormatUint(item.OutputID, 10),
			formatWindowRangeMs(item.WindowStartMs, item.WindowEndMs),
			item.CandidateReason,
			item.CreatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("video_jobs_gif_quality_report_%s_%s.csv", formatFilenameWindowSuffix(windowLabel), time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAdminVideoJobsGIFManualCompareCSV godoc
// @Summary Export gif manual-vs-auto compare CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param limit query int false "rows limit, default 2000, max 20000"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/gif-manual-compare.csv [get]
func (h *Handler) ExportAdminVideoJobsGIFManualCompareCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit, err := parseCSVLimit(c.Query("limit"), 2000, 20000)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	since := time.Now().Add(-windowDuration)
	overview, err := h.loadVideoJobGIFManualScoreOverview(since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	diffSamples, err := h.loadVideoJobGIFManualScoreDiffSamples(since, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"section",
		"metric",
		"value",
		"sample_id",
		"baseline_version",
		"review_round",
		"reviewer",
		"job_id",
		"output_id",
		"manual_overall_score",
		"auto_overall_score",
		"overall_score_delta",
		"abs_overall_score_diff",
		"manual_loop_score",
		"auto_loop_score",
		"loop_score_delta",
		"manual_clarity_score",
		"auto_clarity_score",
		"clarity_score_delta",
		"is_top_pick",
		"is_pass",
		"reject_reason",
		"reviewed_at",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	write := func(
		section string,
		metric string,
		value string,
		sampleID string,
		baselineVersion string,
		reviewRound string,
		reviewer string,
		jobID string,
		outputID string,
		manualOverall string,
		autoOverall string,
		overallDelta string,
		absOverallDiff string,
		manualLoop string,
		autoLoop string,
		loopDelta string,
		manualClarity string,
		autoClarity string,
		clarityDelta string,
		isTopPick string,
		isPass string,
		rejectReason string,
		reviewedAt string,
	) error {
		return writer.Write([]string{
			section,
			metric,
			value,
			sampleID,
			baselineVersion,
			reviewRound,
			reviewer,
			jobID,
			outputID,
			manualOverall,
			autoOverall,
			overallDelta,
			absOverallDiff,
			manualLoop,
			autoLoop,
			loopDelta,
			manualClarity,
			autoClarity,
			clarityDelta,
			isTopPick,
			isPass,
			rejectReason,
			reviewedAt,
		})
	}

	metrics := []struct {
		metric string
		value  string
	}{
		{"samples", strconv.FormatInt(overview.Samples, 10)},
		{"with_output_id", strconv.FormatInt(overview.WithOutputID, 10)},
		{"matched_evaluations", strconv.FormatInt(overview.MatchedEvaluations, 10)},
		{"matched_rate", fmt.Sprintf("%.6f", overview.MatchedRate)},
		{"top_pick_rate", fmt.Sprintf("%.6f", overview.TopPickRate)},
		{"pass_rate", fmt.Sprintf("%.6f", overview.PassRate)},
		{"avg_manual_overall", fmt.Sprintf("%.6f", overview.AvgManualOverall)},
		{"avg_auto_overall", fmt.Sprintf("%.6f", overview.AvgAutoOverall)},
		{"mae_overall", fmt.Sprintf("%.6f", overview.MAEOverall)},
		{"avg_overall_delta", fmt.Sprintf("%.6f", overview.AvgOverallDelta)},
		{"mae_emotion", fmt.Sprintf("%.6f", overview.MAEEmotion)},
		{"mae_clarity", fmt.Sprintf("%.6f", overview.MAEClarity)},
		{"mae_motion", fmt.Sprintf("%.6f", overview.MAEMotion)},
		{"mae_loop", fmt.Sprintf("%.6f", overview.MAELoop)},
		{"mae_efficiency", fmt.Sprintf("%.6f", overview.MAEEfficiency)},
	}
	for _, item := range metrics {
		if err := write("manual_compare_overview", item.metric, item.value, "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	for _, item := range diffSamples {
		if err := write(
			"manual_compare_sample_diff",
			"sample_diff",
			fmt.Sprintf("%.6f", item.AbsOverallScoreDiff),
			item.SampleID,
			item.BaselineVersion,
			item.ReviewRound,
			item.Reviewer,
			strconv.FormatUint(item.JobID, 10),
			strconv.FormatUint(item.OutputID, 10),
			fmt.Sprintf("%.6f", item.ManualOverallScore),
			fmt.Sprintf("%.6f", item.AutoOverallScore),
			fmt.Sprintf("%.6f", item.OverallScoreDelta),
			fmt.Sprintf("%.6f", item.AbsOverallScoreDiff),
			fmt.Sprintf("%.6f", item.ManualLoopScore),
			fmt.Sprintf("%.6f", item.AutoLoopScore),
			fmt.Sprintf("%.6f", item.LoopScoreDelta),
			fmt.Sprintf("%.6f", item.ManualClarityScore),
			fmt.Sprintf("%.6f", item.AutoClarityScore),
			fmt.Sprintf("%.6f", item.ClarityScoreDelta),
			strconv.FormatBool(item.IsTopPick),
			strconv.FormatBool(item.IsPass),
			item.RejectReason,
			item.ReviewedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("video_jobs_gif_manual_compare_%s_%s.csv", formatFilenameWindowSuffix(windowLabel), time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func formatWindowRangeMs(startMs, endMs int) string {
	if endMs <= startMs || startMs < 0 {
		return "-"
	}
	return fmt.Sprintf("%.2fs~%.2fs", float64(startMs)/1000.0, float64(endMs)/1000.0)
}
