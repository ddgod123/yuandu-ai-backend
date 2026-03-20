package handlers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
)

type AdminGIFHealthSchemaCheck struct {
	TableName  string `json:"table_name"`
	ColumnName string `json:"column_name"`
	Status     string `json:"status"`
}

type AdminGIFHealthJobsSummary struct {
	JobsTotal   int64   `json:"jobs_total"`
	JobsDone    int64   `json:"jobs_done"`
	JobsFailed  int64   `json:"jobs_failed"`
	JobsRunning int64   `json:"jobs_running"`
	JobsQueued  int64   `json:"jobs_queued"`
	DoneRate    float64 `json:"done_rate"`
	FailedRate  float64 `json:"failed_rate"`
}

type AdminGIFHealthOutputsSummary struct {
	OutputsTotal int64   `json:"outputs_total"`
	AvgSizeBytes float64 `json:"avg_size_bytes"`
	P50SizeBytes float64 `json:"p50_size_bytes"`
	P95SizeBytes float64 `json:"p95_size_bytes"`
	AvgWidth     float64 `json:"avg_width"`
	AvgHeight    float64 `json:"avg_height"`
}

type AdminGIFHealthLoopTuneSummary struct {
	Samples              int64   `json:"samples"`
	Applied              int64   `json:"applied"`
	EffectiveApplied     int64   `json:"effective_applied"`
	FallbackToBase       int64   `json:"fallback_to_base"`
	AppliedRate          float64 `json:"applied_rate"`
	EffectiveAppliedRate float64 `json:"effective_applied_rate"`
	FallbackRate         float64 `json:"fallback_rate"`
	AvgScore             float64 `json:"avg_score"`
	AvgLoopClosure       float64 `json:"avg_loop_closure"`
	AvgMotionMean        float64 `json:"avg_motion_mean"`
	AvgEffectiveSec      float64 `json:"avg_effective_sec"`
}

type AdminGIFHealthOptimizerSummary struct {
	Samples       int64   `json:"samples"`
	Attempted     int64   `json:"attempted"`
	Applied       int64   `json:"applied"`
	AppliedRate   float64 `json:"applied_rate"`
	AvgSavedRatio float64 `json:"avg_saved_ratio"`
	AvgSavedBytes float64 `json:"avg_saved_bytes"`
}

type AdminGIFHealthPathSummary struct {
	Total              int64   `json:"total"`
	NewPathPrefixCount int64   `json:"new_path_prefix_count"`
	NewPathStrictCount int64   `json:"new_path_strict_count"`
	NewPathPrefixRate  float64 `json:"new_path_prefix_rate"`
	NewPathStrictRate  float64 `json:"new_path_strict_rate"`
}

type AdminGIFHealthFailureReason struct {
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Count        int64  `json:"count"`
}

type AdminGIFHealthConsistencySummary struct {
	DoneWithoutMainOutput   int64 `json:"done_without_main_output"`
	FailedButHasMainOutput  int64 `json:"failed_but_has_main_output"`
	RunningButHasMainOutput int64 `json:"running_but_has_main_output"`
}

type AdminGIFHealthIntegritySummary struct {
	Samples                 int64 `json:"samples"`
	MissingObjectKey        int64 `json:"missing_object_key"`
	NonPositiveSize         int64 `json:"non_positive_size"`
	InvalidDimension        int64 `json:"invalid_dimension"`
	TuneAppliedButZeroScore int64 `json:"tune_applied_but_zero_score"`
}

type AdminGIFHealthCandidateRejectSummary struct {
	Samples            int64   `json:"samples"`
	Rejected           int64   `json:"rejected"`
	RejectRate         float64 `json:"reject_rate"`
	LowEmotion         int64   `json:"low_emotion"`
	LowConfidence      int64   `json:"low_confidence"`
	Duplicate          int64   `json:"duplicate_candidate"`
	BlurLow            int64   `json:"blur_low"`
	SizeBudgetExceeded int64   `json:"size_budget_exceeded"`
	LoopPoor           int64   `json:"loop_poor"`
	UnknownReason      int64   `json:"unknown_reason"`
}

type AdminGIFHealthCandidateRejectItem struct {
	Reason string  `json:"reason"`
	Count  int64   `json:"count"`
	Rate   float64 `json:"rate"`
}

type AdminGIFHealthAlert struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type AdminGIFHealthReportResponse struct {
	WindowHours     int                                  `json:"window_hours"`
	WindowStart     string                               `json:"window_start"`
	WindowEnd       string                               `json:"window_end"`
	CheckedAt       string                               `json:"checked_at"`
	Health          string                               `json:"health"`
	AlertThresholds GIFHealthAlertThresholdSettings      `json:"alert_thresholds"`
	SchemaOK        bool                                 `json:"schema_ok"`
	MissingColumns  int                                  `json:"missing_columns"`
	SchemaChecks    []AdminGIFHealthSchemaCheck          `json:"schema_checks"`
	Jobs            AdminGIFHealthJobsSummary            `json:"jobs"`
	Outputs         AdminGIFHealthOutputsSummary         `json:"outputs"`
	LoopTune        AdminGIFHealthLoopTuneSummary        `json:"loop_tune"`
	Optimizer       AdminGIFHealthOptimizerSummary       `json:"optimizer"`
	Path            AdminGIFHealthPathSummary            `json:"path"`
	TopFailures     []AdminGIFHealthFailureReason        `json:"top_failures"`
	Consistency     AdminGIFHealthConsistencySummary     `json:"consistency"`
	Integrity       AdminGIFHealthIntegritySummary       `json:"integrity"`
	CandidateReject AdminGIFHealthCandidateRejectSummary `json:"candidate_reject"`
	CandidateTop    []AdminGIFHealthCandidateRejectItem  `json:"candidate_top"`
	Alerts          []AdminGIFHealthAlert                `json:"alerts"`
}

type AdminGIFHealthTrendPoint struct {
	BucketStart       string  `json:"bucket_start"`
	BucketEnd         string  `json:"bucket_end"`
	JobsTotal         int64   `json:"jobs_total"`
	JobsDone          int64   `json:"jobs_done"`
	JobsFailed        int64   `json:"jobs_failed"`
	JobsRunning       int64   `json:"jobs_running"`
	JobsQueued        int64   `json:"jobs_queued"`
	DoneRate          float64 `json:"done_rate"`
	FailedRate        float64 `json:"failed_rate"`
	OutputsTotal      int64   `json:"outputs_total"`
	LoopAppliedRate   float64 `json:"loop_applied_rate"`
	LoopFallbackRate  float64 `json:"loop_fallback_rate"`
	NewPathStrictRate float64 `json:"new_path_strict_rate"`
	AvgSizeBytes      float64 `json:"avg_size_bytes"`
	AvgLoopScore      float64 `json:"avg_loop_score"`
}

type AdminGIFHealthTrendResponse struct {
	WindowHours int                        `json:"window_hours"`
	WindowStart string                     `json:"window_start"`
	WindowEnd   string                     `json:"window_end"`
	CheckedAt   string                     `json:"checked_at"`
	Points      []AdminGIFHealthTrendPoint `json:"points"`
}

// GetAdminVideoJobsGIFHealth godoc
// @Summary Get GIF SQL health report (admin)
// @Tags admin
// @Produce json
// @Param window_hours query int false "lookback hours, default 24, range 1..168"
// @Success 200 {object} AdminGIFHealthReportResponse
// @Router /api/admin/video-jobs/gif-health [get]
func (h *Handler) GetAdminVideoJobsGIFHealth(c *gin.Context) {
	windowHours, err := parseGIFHealthWindowHours(c.Query("window_hours"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	since := now.Add(-time.Duration(windowHours) * time.Hour)
	out := AdminGIFHealthReportResponse{
		WindowHours:     windowHours,
		WindowStart:     since.Format(time.RFC3339),
		WindowEnd:       now.Format(time.RFC3339),
		CheckedAt:       now.Format(time.RFC3339),
		Health:          "green",
		AlertThresholds: defaultGIFHealthAlertThresholdSettings(),
		SchemaOK:        true,
		SchemaChecks:    make([]AdminGIFHealthSchemaCheck, 0, 24),
		TopFailures:     make([]AdminGIFHealthFailureReason, 0, 10),
		CandidateTop:    make([]AdminGIFHealthCandidateRejectItem, 0, 10),
		Alerts:          make([]AdminGIFHealthAlert, 0, 8),
	}
	if setting, loadErr := h.loadVideoQualitySetting(); loadErr == nil {
		out.AlertThresholds = gifHealthAlertThresholdSettingsFromModel(setting)
	}

	schemaChecks, err := h.loadAdminGIFHealthSchemaChecks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.SchemaChecks = schemaChecks
	for _, item := range schemaChecks {
		if strings.EqualFold(item.Status, "missing") {
			out.SchemaOK = false
			out.MissingColumns++
		}
	}

	if !out.SchemaOK {
		out.Alerts = append(out.Alerts, AdminGIFHealthAlert{
			Level:   "critical",
			Code:    "schema_missing",
			Message: fmt.Sprintf("public.video_image_* 缺少 %d 个关键字段，请先补齐迁移", out.MissingColumns),
		})
		out.Health = "red"
		c.JSON(http.StatusOK, out)
		return
	}

	if err := h.db.Raw(`
SELECT
  COUNT(*)::bigint AS jobs_total,
  COUNT(*) FILTER (WHERE status = 'done')::bigint AS jobs_done,
  COUNT(*) FILTER (WHERE status = 'failed')::bigint AS jobs_failed,
  COUNT(*) FILTER (WHERE status = 'running')::bigint AS jobs_running,
  COUNT(*) FILTER (WHERE status = 'queued')::bigint AS jobs_queued,
  COALESCE(
    COUNT(*) FILTER (WHERE status = 'done')::double precision / NULLIF(COUNT(*), 0)::double precision,
    0
  ) AS done_rate,
  COALESCE(
    COUNT(*) FILTER (WHERE status = 'failed')::double precision / NULLIF(COUNT(*), 0)::double precision,
    0
  ) AS failed_rate
FROM public.video_image_jobs
WHERE requested_format = 'gif'
  AND created_at >= ?
`, since).Scan(&out.Jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Raw(`
SELECT
  COUNT(*)::bigint AS outputs_total,
  COALESCE(AVG(size_bytes), 0)::double precision AS avg_size_bytes,
  COALESCE(PERCENTILE_DISC(0.5) WITHIN GROUP (ORDER BY size_bytes), 0)::double precision AS p50_size_bytes,
  COALESCE(PERCENTILE_DISC(0.95) WITHIN GROUP (ORDER BY size_bytes), 0)::double precision AS p95_size_bytes,
  COALESCE(AVG(width), 0)::double precision AS avg_width,
  COALESCE(AVG(height), 0)::double precision AS avg_height
FROM public.video_image_outputs
WHERE format = 'gif'
  AND file_role = 'main'
  AND created_at >= ?
`, since).Scan(&out.Outputs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Raw(`
SELECT
  COUNT(*)::bigint AS samples,
  COUNT(*) FILTER (WHERE gif_loop_tune_applied = TRUE)::bigint AS applied,
  COUNT(*) FILTER (WHERE gif_loop_tune_effective_applied = TRUE)::bigint AS effective_applied,
  COUNT(*) FILTER (WHERE gif_loop_tune_fallback_to_base = TRUE)::bigint AS fallback_to_base,
  COALESCE(COUNT(*) FILTER (WHERE gif_loop_tune_applied = TRUE)::double precision / NULLIF(COUNT(*), 0)::double precision, 0) AS applied_rate,
  COALESCE(COUNT(*) FILTER (WHERE gif_loop_tune_effective_applied = TRUE)::double precision / NULLIF(COUNT(*), 0)::double precision, 0) AS effective_applied_rate,
  COALESCE(COUNT(*) FILTER (WHERE gif_loop_tune_fallback_to_base = TRUE)::double precision / NULLIF(COUNT(*), 0)::double precision, 0) AS fallback_rate,
  COALESCE(AVG(gif_loop_tune_score) FILTER (WHERE gif_loop_tune_applied = TRUE), 0)::double precision AS avg_score,
  COALESCE(AVG(gif_loop_tune_loop_closure) FILTER (WHERE gif_loop_tune_applied = TRUE), 0)::double precision AS avg_loop_closure,
  COALESCE(AVG(gif_loop_tune_motion_mean) FILTER (WHERE gif_loop_tune_applied = TRUE), 0)::double precision AS avg_motion_mean,
  COALESCE(AVG(gif_loop_tune_effective_sec) FILTER (WHERE gif_loop_tune_applied = TRUE), 0)::double precision AS avg_effective_sec
FROM public.video_image_outputs
WHERE format = 'gif'
  AND file_role = 'main'
  AND created_at >= ?
`, since).Scan(&out.LoopTune).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Raw(`
SELECT
  COUNT(*)::bigint AS samples,
  COUNT(*) FILTER (
    WHERE COALESCE((metadata->'gif_optimization_v1'->>'attempted')::boolean, FALSE) = TRUE
  )::bigint AS attempted,
  COUNT(*) FILTER (
    WHERE COALESCE((metadata->'gif_optimization_v1'->>'applied')::boolean, FALSE) = TRUE
  )::bigint AS applied,
  COALESCE(
    COUNT(*) FILTER (
      WHERE COALESCE((metadata->'gif_optimization_v1'->>'applied')::boolean, FALSE) = TRUE
    )::double precision / NULLIF(COUNT(*), 0)::double precision,
    0
  ) AS applied_rate,
  COALESCE(
    AVG(NULLIF(metadata->'gif_optimization_v1'->>'saved_ratio', '')::double precision)
      FILTER (WHERE COALESCE((metadata->'gif_optimization_v1'->>'applied')::boolean, FALSE) = TRUE),
    0
  ) AS avg_saved_ratio,
  COALESCE(
    AVG(NULLIF(metadata->'gif_optimization_v1'->>'saved_bytes', '')::double precision)
      FILTER (WHERE COALESCE((metadata->'gif_optimization_v1'->>'applied')::boolean, FALSE) = TRUE),
    0
  ) AS avg_saved_bytes
FROM public.video_image_outputs
WHERE format = 'gif'
  AND file_role = 'main'
  AND created_at >= ?
`, since).Scan(&out.Optimizer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Raw(`
SELECT
  COUNT(*)::bigint AS total,
  COUNT(*) FILTER (WHERE object_key LIKE 'emoji/video-image/%')::bigint AS new_path_prefix_count,
  COUNT(*) FILTER (
    WHERE object_key ~ '^emoji/video-image/[^/]+/u/[0-9]{1,2}/[0-9]+/j/[0-9]+/outputs/gif/.+'
  )::bigint AS new_path_strict_count,
  COALESCE(
    COUNT(*) FILTER (WHERE object_key LIKE 'emoji/video-image/%')::double precision / NULLIF(COUNT(*), 0)::double precision,
    0
  ) AS new_path_prefix_rate,
  COALESCE(
    COUNT(*) FILTER (
      WHERE object_key ~ '^emoji/video-image/[^/]+/u/[0-9]{1,2}/[0-9]+/j/[0-9]+/outputs/gif/.+'
    )::double precision / NULLIF(COUNT(*), 0)::double precision,
    0
  ) AS new_path_strict_rate
FROM public.video_image_outputs
WHERE format = 'gif'
  AND file_role = 'main'
  AND created_at >= ?
`, since).Scan(&out.Path).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Raw(`
SELECT
  COALESCE(NULLIF(error_code, ''), '[empty]') AS error_code,
  COALESCE(NULLIF(error_message, ''), '[empty]') AS error_message,
  COUNT(*)::bigint AS count
FROM public.video_image_jobs
WHERE requested_format = 'gif'
  AND status = 'failed'
  AND created_at >= ?
GROUP BY 1, 2
ORDER BY count DESC
LIMIT 10
`, since).Scan(&out.TopFailures).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Raw(`
WITH gif_jobs AS (
  SELECT id, status
  FROM public.video_image_jobs
  WHERE requested_format = 'gif'
    AND created_at >= ?
), gif_main_outputs AS (
  SELECT DISTINCT job_id
  FROM public.video_image_outputs
  WHERE format = 'gif'
    AND file_role = 'main'
    AND created_at >= ?
)
SELECT
  COUNT(*) FILTER (WHERE j.status = 'done' AND o.job_id IS NULL)::bigint AS done_without_main_output,
  COUNT(*) FILTER (WHERE j.status = 'failed' AND o.job_id IS NOT NULL)::bigint AS failed_but_has_main_output,
  COUNT(*) FILTER (WHERE j.status = 'running' AND o.job_id IS NOT NULL)::bigint AS running_but_has_main_output
FROM gif_jobs j
LEFT JOIN gif_main_outputs o ON o.job_id = j.id
`, since, since).Scan(&out.Consistency).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Raw(`
SELECT
  COUNT(*)::bigint AS samples,
  COUNT(*) FILTER (WHERE COALESCE(object_key, '') = '')::bigint AS missing_object_key,
  COUNT(*) FILTER (WHERE size_bytes <= 0)::bigint AS non_positive_size,
  COUNT(*) FILTER (WHERE width <= 0 OR height <= 0)::bigint AS invalid_dimension,
  COUNT(*) FILTER (WHERE gif_loop_tune_applied = TRUE AND gif_loop_tune_score = 0)::bigint AS tune_applied_but_zero_score
FROM public.video_image_outputs
WHERE format = 'gif'
  AND file_role = 'main'
  AND created_at >= ?
`, since).Scan(&out.Integrity).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Raw(`
WITH base AS (
  SELECT
    LOWER(TRIM(COALESCE(c.reject_reason, ''))) AS reject_reason
  FROM archive.video_job_gif_candidates c
  JOIN public.video_image_jobs j ON j.id = c.job_id
  WHERE j.requested_format = 'gif'
    AND c.created_at >= ?
),
total AS (
  SELECT COUNT(*)::bigint AS samples FROM base
),
rejected AS (
  SELECT
    COUNT(*) FILTER (WHERE reject_reason <> '')::bigint AS rejected,
    COUNT(*) FILTER (WHERE reject_reason = ?)::bigint AS low_emotion,
    COUNT(*) FILTER (WHERE reject_reason = ?)::bigint AS low_confidence,
    COUNT(*) FILTER (WHERE reject_reason = ?)::bigint AS duplicate_candidate,
    COUNT(*) FILTER (WHERE reject_reason = ?)::bigint AS blur_low,
    COUNT(*) FILTER (WHERE reject_reason = ?)::bigint AS size_budget_exceeded,
    COUNT(*) FILTER (WHERE reject_reason = ?)::bigint AS loop_poor,
    COUNT(*) FILTER (
      WHERE reject_reason <> ''
        AND reject_reason NOT IN (?, ?, ?, ?, ?, ?)
    )::bigint AS unknown_reason
  FROM base
)
SELECT
  COALESCE(t.samples, 0)::bigint AS samples,
  COALESCE(r.rejected, 0)::bigint AS rejected,
  COALESCE(
    COALESCE(r.rejected, 0)::double precision / NULLIF(COALESCE(t.samples, 0)::double precision, 0),
    0
  ) AS reject_rate,
  COALESCE(r.low_emotion, 0)::bigint AS low_emotion,
  COALESCE(r.low_confidence, 0)::bigint AS low_confidence,
  COALESCE(r.duplicate_candidate, 0)::bigint AS duplicate_candidate,
  COALESCE(r.blur_low, 0)::bigint AS blur_low,
  COALESCE(r.size_budget_exceeded, 0)::bigint AS size_budget_exceeded,
  COALESCE(r.loop_poor, 0)::bigint AS loop_poor,
  COALESCE(r.unknown_reason, 0)::bigint AS unknown_reason
FROM total t
LEFT JOIN rejected r ON TRUE
`,
		since,
		videojobs.GIFCandidateRejectReasonLowEmotion,
		videojobs.GIFCandidateRejectReasonLowConfidence,
		videojobs.GIFCandidateRejectReasonDuplicate,
		videojobs.GIFCandidateRejectReasonBlurLow,
		videojobs.GIFCandidateRejectReasonSizeBudgetExceeded,
		videojobs.GIFCandidateRejectReasonLoopPoor,
		videojobs.GIFCandidateRejectReasonLowEmotion,
		videojobs.GIFCandidateRejectReasonLowConfidence,
		videojobs.GIFCandidateRejectReasonDuplicate,
		videojobs.GIFCandidateRejectReasonBlurLow,
		videojobs.GIFCandidateRejectReasonSizeBudgetExceeded,
		videojobs.GIFCandidateRejectReasonLoopPoor,
	).Scan(&out.CandidateReject).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_gif_candidates") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if err := h.db.Raw(`
WITH base AS (
  SELECT
    LOWER(TRIM(COALESCE(c.reject_reason, ''))) AS reject_reason
  FROM archive.video_job_gif_candidates c
  JOIN public.video_image_jobs j ON j.id = c.job_id
  WHERE j.requested_format = 'gif'
    AND c.created_at >= ?
    AND COALESCE(c.reject_reason, '') <> ''
),
tot AS (
  SELECT COUNT(*)::double precision AS rejected_total FROM base
)
SELECT
  b.reject_reason AS reason,
  COUNT(*)::bigint AS count,
  COALESCE(COUNT(*)::double precision / NULLIF(t.rejected_total, 0), 0) AS rate
FROM base b
CROSS JOIN tot t
GROUP BY b.reject_reason, t.rejected_total
ORDER BY count DESC, reason ASC
LIMIT 10
`, since).Scan(&out.CandidateTop).Error; err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if !(strings.Contains(msg, "archive.video_job_gif_candidates") && strings.Contains(msg, "does not exist")) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	out.Alerts = buildAdminGIFHealthAlerts(out, out.AlertThresholds)
	out.Health = resolveAdminGIFHealth(out.Alerts)

	c.JSON(http.StatusOK, out)
}

// GetAdminVideoJobsGIFHealthTrend godoc
// @Summary Get GIF SQL health trend (admin)
// @Tags admin
// @Produce json
// @Param window_hours query int false "lookback hours, default 24, range 1..168"
// @Success 200 {object} AdminGIFHealthTrendResponse
// @Router /api/admin/video-jobs/gif-health/trend [get]
func (h *Handler) GetAdminVideoJobsGIFHealthTrend(c *gin.Context) {
	windowHours, err := parseGIFHealthWindowHours(c.Query("window_hours"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	since := now.Add(-time.Duration(windowHours) * time.Hour)
	points, err := h.loadAdminGIFHealthTrendPoints(since, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	out := AdminGIFHealthTrendResponse{
		WindowHours: windowHours,
		WindowStart: since.Format(time.RFC3339),
		WindowEnd:   now.Format(time.RFC3339),
		CheckedAt:   now.Format(time.RFC3339),
		Points:      points,
	}
	c.JSON(http.StatusOK, out)
}

// ExportAdminVideoJobsGIFHealthTrendCSV godoc
// @Summary Export GIF SQL health trend CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window_hours query int false "lookback hours, default 24, range 1..168"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/gif-health/trend.csv [get]
func (h *Handler) ExportAdminVideoJobsGIFHealthTrendCSV(c *gin.Context) {
	windowHours, err := parseGIFHealthWindowHours(c.Query("window_hours"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	since := now.Add(-time.Duration(windowHours) * time.Hour)
	points, err := h.loadAdminGIFHealthTrendPoints(since, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"bucket_start",
		"bucket_end",
		"jobs_total",
		"jobs_done",
		"jobs_failed",
		"jobs_running",
		"jobs_queued",
		"done_rate",
		"failed_rate",
		"outputs_total",
		"loop_applied_rate",
		"loop_fallback_rate",
		"new_path_strict_rate",
		"avg_size_bytes",
		"avg_loop_score",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, point := range points {
		if err := writer.Write([]string{
			point.BucketStart,
			point.BucketEnd,
			strconv.FormatInt(point.JobsTotal, 10),
			strconv.FormatInt(point.JobsDone, 10),
			strconv.FormatInt(point.JobsFailed, 10),
			strconv.FormatInt(point.JobsRunning, 10),
			strconv.FormatInt(point.JobsQueued, 10),
			fmt.Sprintf("%.6f", point.DoneRate),
			fmt.Sprintf("%.6f", point.FailedRate),
			strconv.FormatInt(point.OutputsTotal, 10),
			fmt.Sprintf("%.6f", point.LoopAppliedRate),
			fmt.Sprintf("%.6f", point.LoopFallbackRate),
			fmt.Sprintf("%.6f", point.NewPathStrictRate),
			fmt.Sprintf("%.2f", point.AvgSizeBytes),
			fmt.Sprintf("%.6f", point.AvgLoopScore),
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

	filename := fmt.Sprintf("video_jobs_gif_health_trend_%dh_%s.csv", windowHours, time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

func (h *Handler) loadAdminGIFHealthSchemaChecks() ([]AdminGIFHealthSchemaCheck, error) {
	rows := make([]AdminGIFHealthSchemaCheck, 0, 24)
	err := h.db.Raw(`
WITH required_columns AS (
  SELECT *
  FROM (VALUES
    ('video_image_jobs', 'id'),
    ('video_image_jobs', 'requested_format'),
    ('video_image_jobs', 'status'),
    ('video_image_jobs', 'stage'),
    ('video_image_jobs', 'error_message'),
    ('video_image_jobs', 'created_at'),
    ('video_image_outputs', 'job_id'),
    ('video_image_outputs', 'format'),
    ('video_image_outputs', 'file_role'),
    ('video_image_outputs', 'object_key'),
    ('video_image_outputs', 'size_bytes'),
    ('video_image_outputs', 'width'),
    ('video_image_outputs', 'height'),
    ('video_image_outputs', 'gif_loop_tune_applied'),
    ('video_image_outputs', 'gif_loop_tune_effective_applied'),
    ('video_image_outputs', 'gif_loop_tune_fallback_to_base'),
    ('video_image_outputs', 'gif_loop_tune_score'),
    ('video_image_outputs', 'gif_loop_tune_loop_closure'),
    ('video_image_outputs', 'gif_loop_tune_motion_mean'),
    ('video_image_outputs', 'gif_loop_tune_effective_sec'),
    ('video_image_outputs', 'created_at')
  ) AS t(table_name, column_name)
), existing_columns AS (
  SELECT table_name, column_name
  FROM information_schema.columns
  WHERE table_schema = 'public'
    AND table_name IN ('video_image_jobs', 'video_image_outputs')
)
SELECT
  rc.table_name,
  rc.column_name,
  CASE WHEN ec.column_name IS NULL THEN 'missing' ELSE 'ok' END AS status
FROM required_columns rc
LEFT JOIN existing_columns ec
  ON ec.table_name = rc.table_name
 AND ec.column_name = rc.column_name
ORDER BY rc.table_name, rc.column_name
`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (h *Handler) loadAdminGIFHealthTrendPoints(since time.Time, until time.Time) ([]AdminGIFHealthTrendPoint, error) {
	type row struct {
		BucketStart       time.Time `gorm:"column:bucket_start"`
		JobsTotal         int64     `gorm:"column:jobs_total"`
		JobsDone          int64     `gorm:"column:jobs_done"`
		JobsFailed        int64     `gorm:"column:jobs_failed"`
		JobsRunning       int64     `gorm:"column:jobs_running"`
		JobsQueued        int64     `gorm:"column:jobs_queued"`
		DoneRate          float64   `gorm:"column:done_rate"`
		FailedRate        float64   `gorm:"column:failed_rate"`
		OutputsTotal      int64     `gorm:"column:outputs_total"`
		LoopAppliedRate   float64   `gorm:"column:loop_applied_rate"`
		LoopFallbackRate  float64   `gorm:"column:loop_fallback_rate"`
		NewPathStrictRate float64   `gorm:"column:new_path_strict_rate"`
		AvgSizeBytes      float64   `gorm:"column:avg_size_bytes"`
		AvgLoopScore      float64   `gorm:"column:avg_loop_score"`
	}

	rows := make([]row, 0, 200)
	err := h.db.Raw(`
WITH buckets AS (
  SELECT generate_series(
    date_trunc('hour', ?::timestamptz),
    date_trunc('hour', ?::timestamptz),
    interval '1 hour'
  ) AS bucket_start
),
job_agg AS (
  SELECT
    date_trunc('hour', created_at) AS bucket_start,
    COUNT(*)::bigint AS jobs_total,
    COUNT(*) FILTER (WHERE status = 'done')::bigint AS jobs_done,
    COUNT(*) FILTER (WHERE status = 'failed')::bigint AS jobs_failed,
    COUNT(*) FILTER (WHERE status = 'running')::bigint AS jobs_running,
    COUNT(*) FILTER (WHERE status = 'queued')::bigint AS jobs_queued
  FROM public.video_image_jobs
  WHERE requested_format = 'gif'
    AND created_at >= ?
    AND created_at <= ?
  GROUP BY 1
),
output_agg AS (
  SELECT
    date_trunc('hour', created_at) AS bucket_start,
    COUNT(*)::bigint AS outputs_total,
    COUNT(*) FILTER (WHERE gif_loop_tune_applied = TRUE)::bigint AS loop_applied,
    COUNT(*) FILTER (WHERE gif_loop_tune_fallback_to_base = TRUE)::bigint AS loop_fallback,
    COUNT(*) FILTER (
      WHERE object_key ~ '^emoji/video-image/[^/]+/u/[0-9]{1,2}/[0-9]+/j/[0-9]+/outputs/gif/.+'
    )::bigint AS new_path_strict,
    COALESCE(AVG(size_bytes), 0)::double precision AS avg_size_bytes,
    COALESCE(AVG(gif_loop_tune_score) FILTER (WHERE gif_loop_tune_applied = TRUE), 0)::double precision AS avg_loop_score
  FROM public.video_image_outputs
  WHERE format = 'gif'
    AND file_role = 'main'
    AND created_at >= ?
    AND created_at <= ?
  GROUP BY 1
)
SELECT
  b.bucket_start,
  COALESCE(j.jobs_total, 0)::bigint AS jobs_total,
  COALESCE(j.jobs_done, 0)::bigint AS jobs_done,
  COALESCE(j.jobs_failed, 0)::bigint AS jobs_failed,
  COALESCE(j.jobs_running, 0)::bigint AS jobs_running,
  COALESCE(j.jobs_queued, 0)::bigint AS jobs_queued,
  COALESCE(
    COALESCE(j.jobs_done, 0)::double precision / NULLIF(COALESCE(j.jobs_total, 0)::double precision, 0),
    0
  ) AS done_rate,
  COALESCE(
    COALESCE(j.jobs_failed, 0)::double precision / NULLIF(COALESCE(j.jobs_total, 0)::double precision, 0),
    0
  ) AS failed_rate,
  COALESCE(o.outputs_total, 0)::bigint AS outputs_total,
  COALESCE(
    COALESCE(o.loop_applied, 0)::double precision / NULLIF(COALESCE(o.outputs_total, 0)::double precision, 0),
    0
  ) AS loop_applied_rate,
  COALESCE(
    COALESCE(o.loop_fallback, 0)::double precision / NULLIF(COALESCE(o.outputs_total, 0)::double precision, 0),
    0
  ) AS loop_fallback_rate,
  COALESCE(
    COALESCE(o.new_path_strict, 0)::double precision / NULLIF(COALESCE(o.outputs_total, 0)::double precision, 0),
    0
  ) AS new_path_strict_rate,
  COALESCE(o.avg_size_bytes, 0)::double precision AS avg_size_bytes,
  COALESCE(o.avg_loop_score, 0)::double precision AS avg_loop_score
FROM buckets b
LEFT JOIN job_agg j ON j.bucket_start = b.bucket_start
LEFT JOIN output_agg o ON o.bucket_start = b.bucket_start
ORDER BY b.bucket_start ASC
`, since, until, since, until, since, until).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]AdminGIFHealthTrendPoint, 0, len(rows))
	for _, item := range rows {
		end := item.BucketStart.Add(time.Hour)
		out = append(out, AdminGIFHealthTrendPoint{
			BucketStart:       item.BucketStart.Format(time.RFC3339),
			BucketEnd:         end.Format(time.RFC3339),
			JobsTotal:         item.JobsTotal,
			JobsDone:          item.JobsDone,
			JobsFailed:        item.JobsFailed,
			JobsRunning:       item.JobsRunning,
			JobsQueued:        item.JobsQueued,
			DoneRate:          item.DoneRate,
			FailedRate:        item.FailedRate,
			OutputsTotal:      item.OutputsTotal,
			LoopAppliedRate:   item.LoopAppliedRate,
			LoopFallbackRate:  item.LoopFallbackRate,
			NewPathStrictRate: item.NewPathStrictRate,
			AvgSizeBytes:      item.AvgSizeBytes,
			AvgLoopScore:      item.AvgLoopScore,
		})
	}
	return out, nil
}

func parseGIFHealthWindowHours(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 24, nil
	}
	hours, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid window_hours, expected 1..168")
	}
	if hours < 1 || hours > 168 {
		return 0, fmt.Errorf("invalid window_hours, expected 1..168")
	}
	return hours, nil
}

func buildAdminGIFHealthAlerts(out AdminGIFHealthReportResponse, thresholds GIFHealthAlertThresholdSettings) []AdminGIFHealthAlert {
	alerts := make([]AdminGIFHealthAlert, 0, 8)
	add := func(level, code, message string) {
		alerts = append(alerts, AdminGIFHealthAlert{
			Level:   level,
			Code:    code,
			Message: message,
		})
	}

	if out.Jobs.JobsTotal == 0 {
		add("warn", "jobs_empty", "窗口内没有 GIF 任务，建议确认流量或筛选窗口")
	} else {
		if out.Jobs.DoneRate < thresholds.GIFHealthDoneRateCritical {
			add("critical", "done_rate_low", fmt.Sprintf("GIF 完成率偏低（%.2f%%）", out.Jobs.DoneRate*100))
		} else if out.Jobs.DoneRate < thresholds.GIFHealthDoneRateWarn {
			add("warn", "done_rate_warn", fmt.Sprintf("GIF 完成率需要关注（%.2f%%）", out.Jobs.DoneRate*100))
		}
		if out.Jobs.FailedRate > thresholds.GIFHealthFailedRateCritical {
			add("critical", "failed_rate_high", fmt.Sprintf("GIF 失败率过高（%.2f%%）", out.Jobs.FailedRate*100))
		} else if out.Jobs.FailedRate > thresholds.GIFHealthFailedRateWarn {
			add("warn", "failed_rate_warn", fmt.Sprintf("GIF 失败率偏高（%.2f%%）", out.Jobs.FailedRate*100))
		}
	}

	if out.Path.Total > 0 {
		if out.Path.NewPathStrictRate < thresholds.GIFHealthPathStrictRateCritical {
			add("critical", "path_strict_low", fmt.Sprintf("七牛新路径严格命中率过低（%.2f%%）", out.Path.NewPathStrictRate*100))
		} else if out.Path.NewPathStrictRate < thresholds.GIFHealthPathStrictRateWarn {
			add("warn", "path_strict_warn", fmt.Sprintf("七牛新路径严格命中率偏低（%.2f%%）", out.Path.NewPathStrictRate*100))
		}
	}

	if out.Consistency.DoneWithoutMainOutput > 0 {
		add("critical", "done_missing_output", fmt.Sprintf("存在 %d 个 done 任务缺少 GIF 主产物", out.Consistency.DoneWithoutMainOutput))
	}
	if out.Consistency.FailedButHasMainOutput > 0 {
		add("warn", "failed_with_output", fmt.Sprintf("存在 %d 个 failed 任务却有 GIF 主产物", out.Consistency.FailedButHasMainOutput))
	}
	if out.Consistency.RunningButHasMainOutput > 0 {
		add("warn", "running_with_output", fmt.Sprintf("存在 %d 个 running 任务已出现 GIF 主产物", out.Consistency.RunningButHasMainOutput))
	}

	if out.Integrity.MissingObjectKey > 0 {
		add("critical", "missing_object_key", fmt.Sprintf("发现 %d 条 GIF 主产物 object_key 为空", out.Integrity.MissingObjectKey))
	}
	if out.Integrity.InvalidDimension > 0 {
		add("critical", "invalid_dimension", fmt.Sprintf("发现 %d 条 GIF 主产物宽高异常", out.Integrity.InvalidDimension))
	}
	if out.Integrity.NonPositiveSize > 0 {
		add("warn", "non_positive_size", fmt.Sprintf("发现 %d 条 GIF 主产物 size_bytes<=0", out.Integrity.NonPositiveSize))
	}
	if out.Integrity.TuneAppliedButZeroScore > 0 {
		add("warn", "tune_zero_score", fmt.Sprintf("发现 %d 条 GIF 调优触发但 score=0", out.Integrity.TuneAppliedButZeroScore))
	}

	if out.LoopTune.Samples > 0 {
		if out.LoopTune.FallbackRate > thresholds.GIFHealthLoopFallbackRateCritical {
			add("critical", "loop_fallback_high", fmt.Sprintf("GIF loop 回退率过高（%.2f%%）", out.LoopTune.FallbackRate*100))
		} else if out.LoopTune.FallbackRate > thresholds.GIFHealthLoopFallbackRateWarn {
			add("warn", "loop_fallback_warn", fmt.Sprintf("GIF loop 回退率偏高（%.2f%%）", out.LoopTune.FallbackRate*100))
		}
	}

	return alerts
}

func resolveAdminGIFHealth(alerts []AdminGIFHealthAlert) string {
	hasWarn := false
	for _, alert := range alerts {
		switch strings.ToLower(strings.TrimSpace(alert.Level)) {
		case "critical", "error", "fatal":
			return "red"
		case "warn", "warning":
			hasWarn = true
		}
	}
	if hasWarn {
		return "yellow"
	}
	return "green"
}
