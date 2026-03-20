package handlers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ExportAdminSampleVideoJobsBaselineCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	since := time.Now().Add(-windowDuration)

	summary, err := h.loadSampleVideoJobsWindowSummary(since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	formatRows, err := h.loadSampleVideoJobFormatBaselineRows(since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"section",
		"window",
		"metric",
		"value",
		"format",
		"requested_jobs",
		"generated_jobs",
		"success_rate",
		"artifact_count",
		"avg_artifact_size_bytes",
		"duration_p50_sec",
		"duration_p95_sec",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	summaryRows := []struct {
		metric string
		value  string
	}{
		{metric: "sample_jobs_window", value: strconv.FormatInt(summary.JobsWindow, 10)},
		{metric: "sample_done_window", value: strconv.FormatInt(summary.DoneWindow, 10)},
		{metric: "sample_failed_window", value: strconv.FormatInt(summary.FailedWindow, 10)},
		{metric: "sample_success_rate_window", value: fmt.Sprintf("%.6f", summary.SuccessRate)},
		{metric: "sample_duration_p50_sec", value: fmt.Sprintf("%.6f", summary.DurationP50)},
		{metric: "sample_duration_p95_sec", value: fmt.Sprintf("%.6f", summary.DurationP95)},
	}
	for _, row := range summaryRows {
		if err := writer.Write([]string{
			"summary",
			windowLabel,
			row.metric,
			row.value,
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	for _, row := range formatRows {
		if err := writer.Write([]string{
			"format",
			windowLabel,
			"",
			"",
			row.Format,
			strconv.FormatInt(row.RequestedJobs, 10),
			strconv.FormatInt(row.GeneratedJobs, 10),
			fmt.Sprintf("%.6f", row.SuccessRate),
			strconv.FormatInt(row.ArtifactCount, 10),
			fmt.Sprintf("%.6f", row.AvgArtifactSizeBytes),
			fmt.Sprintf("%.6f", row.DurationP50Sec),
			fmt.Sprintf("%.6f", row.DurationP95Sec),
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

	filename := fmt.Sprintf("video_jobs_sample_baseline_%s_%s.csv", windowLabel, time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAdminSampleVideoJobsBaselineDiffCSV godoc
// @Summary Export sample video-jobs baseline diff CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param base_window query string false "reference window: 24h | 7d | 30d (default: 7d)"
// @Param target_window query string false "target window: 24h | 7d | 30d (default: 24h)"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/samples/baseline-diff.csv [get]
func (h *Handler) ExportAdminSampleVideoJobsBaselineDiffCSV(c *gin.Context) {
	baseLabel, baseDuration, err := parseVideoJobsOverviewWindow(c.DefaultQuery("base_window", "7d"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base_window"})
		return
	}
	targetLabel, targetDuration, err := parseVideoJobsOverviewWindow(c.DefaultQuery("target_window", "24h"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_window"})
		return
	}

	now := time.Now()
	baseSince := now.Add(-baseDuration)
	targetSince := now.Add(-targetDuration)

	baseSummary, err := h.loadSampleVideoJobsWindowSummary(baseSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetSummary, err := h.loadSampleVideoJobsWindowSummary(targetSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	baseRows, err := h.loadSampleVideoJobFormatBaselineRows(baseSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetRows, err := h.loadSampleVideoJobFormatBaselineRows(targetSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	diffRows := buildSampleVideoJobFormatDiffRows(baseRows, targetRows)

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"section",
		"base_window",
		"target_window",
		"metric",
		"base_value",
		"target_value",
		"delta",
		"uplift",
		"format",
		"base_requested_jobs",
		"target_requested_jobs",
		"base_generated_jobs",
		"target_generated_jobs",
		"base_success_rate",
		"target_success_rate",
		"success_rate_delta",
		"success_rate_uplift",
		"base_avg_artifact_size_bytes",
		"target_avg_artifact_size_bytes",
		"avg_artifact_size_delta",
		"avg_artifact_size_uplift",
		"base_duration_p50_sec",
		"target_duration_p50_sec",
		"duration_p50_delta",
		"duration_p50_uplift",
		"base_duration_p95_sec",
		"target_duration_p95_sec",
		"duration_p95_delta",
		"duration_p95_uplift",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	writeSummaryMetric := func(metric string, baseValue, targetValue float64) error {
		delta, uplift := computeDeltaAndUplift(baseValue, targetValue)
		return writer.Write([]string{
			"summary",
			baseLabel,
			targetLabel,
			metric,
			fmt.Sprintf("%.6f", baseValue),
			fmt.Sprintf("%.6f", targetValue),
			fmt.Sprintf("%.6f", delta),
			fmt.Sprintf("%.6f", uplift),
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
		})
	}

	if err := writeSummaryMetric("sample_jobs_window", float64(baseSummary.JobsWindow), float64(targetSummary.JobsWindow)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := writeSummaryMetric("sample_done_window", float64(baseSummary.DoneWindow), float64(targetSummary.DoneWindow)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := writeSummaryMetric("sample_failed_window", float64(baseSummary.FailedWindow), float64(targetSummary.FailedWindow)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := writeSummaryMetric("sample_success_rate_window", baseSummary.SuccessRate, targetSummary.SuccessRate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := writeSummaryMetric("sample_duration_p50_sec", baseSummary.DurationP50, targetSummary.DurationP50); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := writeSummaryMetric("sample_duration_p95_sec", baseSummary.DurationP95, targetSummary.DurationP95); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, row := range diffRows {
		if err := writer.Write([]string{
			"format",
			baseLabel,
			targetLabel,
			"",
			"",
			"",
			"",
			"",
			row.Format,
			strconv.FormatInt(row.BaseRequestedJobs, 10),
			strconv.FormatInt(row.TargetRequestedJobs, 10),
			strconv.FormatInt(row.BaseGeneratedJobs, 10),
			strconv.FormatInt(row.TargetGeneratedJobs, 10),
			fmt.Sprintf("%.6f", row.BaseSuccessRate),
			fmt.Sprintf("%.6f", row.TargetSuccessRate),
			fmt.Sprintf("%.6f", row.SuccessRateDelta),
			fmt.Sprintf("%.6f", row.SuccessRateUplift),
			fmt.Sprintf("%.6f", row.BaseAvgArtifactSizeBytes),
			fmt.Sprintf("%.6f", row.TargetAvgArtifactSizeBytes),
			fmt.Sprintf("%.6f", row.AvgArtifactSizeDelta),
			fmt.Sprintf("%.6f", row.AvgArtifactSizeUplift),
			fmt.Sprintf("%.6f", row.BaseDurationP50Sec),
			fmt.Sprintf("%.6f", row.TargetDurationP50Sec),
			fmt.Sprintf("%.6f", row.DurationP50Delta),
			fmt.Sprintf("%.6f", row.DurationP50Uplift),
			fmt.Sprintf("%.6f", row.BaseDurationP95Sec),
			fmt.Sprintf("%.6f", row.TargetDurationP95Sec),
			fmt.Sprintf("%.6f", row.DurationP95Delta),
			fmt.Sprintf("%.6f", row.DurationP95Uplift),
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

	filename := fmt.Sprintf(
		"video_jobs_sample_baseline_diff_%s_vs_%s_%s.csv",
		targetLabel,
		baseLabel,
		time.Now().Format("20060102_150405"),
	)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// GetAdminSampleVideoJobsBaselineDiff godoc
// @Summary Get sample video-jobs baseline diff (admin)
// @Tags admin
// @Produce json
// @Param base_window query string false "reference window: 24h | 7d | 30d (default: 7d)"
// @Param target_window query string false "target window: 24h | 7d | 30d (default: 24h)"
// @Success 200 {object} AdminSampleVideoJobsBaselineDiffResponse
// @Router /api/admin/video-jobs/samples/baseline-diff [get]
func (h *Handler) GetAdminSampleVideoJobsBaselineDiff(c *gin.Context) {
	baseLabel, baseDuration, err := parseVideoJobsOverviewWindow(c.DefaultQuery("base_window", "7d"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base_window"})
		return
	}
	targetLabel, targetDuration, err := parseVideoJobsOverviewWindow(c.DefaultQuery("target_window", "24h"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_window"})
		return
	}

	now := time.Now()
	baseSince := now.Add(-baseDuration)
	targetSince := now.Add(-targetDuration)

	baseSummary, err := h.loadSampleVideoJobsWindowSummary(baseSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetSummary, err := h.loadSampleVideoJobsWindowSummary(targetSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	baseRows, err := h.loadSampleVideoJobFormatBaselineRows(baseSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetRows, err := h.loadSampleVideoJobFormatBaselineRows(targetSince)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	summary := buildSampleVideoJobsDiffSummary(baseSummary, targetSummary)
	formatDiffRows := buildSampleVideoJobFormatDiffRows(baseRows, targetRows)
	formats := make([]AdminSampleVideoJobsBaselineDiffFormatStat, 0, len(formatDiffRows))
	for _, row := range formatDiffRows {
		formats = append(formats, AdminSampleVideoJobsBaselineDiffFormatStat{
			Format:                     row.Format,
			BaseRequestedJobs:          row.BaseRequestedJobs,
			TargetRequestedJobs:        row.TargetRequestedJobs,
			BaseGeneratedJobs:          row.BaseGeneratedJobs,
			TargetGeneratedJobs:        row.TargetGeneratedJobs,
			BaseSuccessRate:            row.BaseSuccessRate,
			TargetSuccessRate:          row.TargetSuccessRate,
			SuccessRateDelta:           row.SuccessRateDelta,
			SuccessRateUplift:          row.SuccessRateUplift,
			BaseAvgArtifactSizeBytes:   row.BaseAvgArtifactSizeBytes,
			TargetAvgArtifactSizeBytes: row.TargetAvgArtifactSizeBytes,
			AvgArtifactSizeDelta:       row.AvgArtifactSizeDelta,
			AvgArtifactSizeUplift:      row.AvgArtifactSizeUplift,
			BaseDurationP50Sec:         row.BaseDurationP50Sec,
			TargetDurationP50Sec:       row.TargetDurationP50Sec,
			DurationP50Delta:           row.DurationP50Delta,
			DurationP50Uplift:          row.DurationP50Uplift,
			BaseDurationP95Sec:         row.BaseDurationP95Sec,
			TargetDurationP95Sec:       row.TargetDurationP95Sec,
			DurationP95Delta:           row.DurationP95Delta,
			DurationP95Uplift:          row.DurationP95Uplift,
		})
	}

	c.JSON(http.StatusOK, AdminSampleVideoJobsBaselineDiffResponse{
		BaseWindow:   baseLabel,
		TargetWindow: targetLabel,
		GeneratedAt:  now.Format(time.RFC3339),
		Summary:      summary,
		Formats:      formats,
	})
}

// ExportAdminVideoJobsGIFSubStageAnomaliesCSV godoc
// @Summary Export GIF sub-stage anomalies CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param sub_stage query string false "all | briefing | planning | scoring | reviewing"
// @Param sub_status query string false "all | degraded | failed"
// @Param limit query int false "row limit (default 500, max 2000)"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/gif-sub-stage-anomalies.csv [get]
