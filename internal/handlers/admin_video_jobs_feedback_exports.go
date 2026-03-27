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
)

// ExportAdminVideoJobsFeedbackCSV godoc
// @Summary Export video-jobs feedback report CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param user_id query int false "user id filter"
// @Param format query string false "requested format filter"
// @Param guard_reason query string false "negative guard reason filter"
// @Param blocked_only query bool false "only export blocked_reason rows"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/feedback-report.csv [get]
func (h *Handler) ExportAdminVideoJobsFeedbackCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter, err := parseVideoImageFeedbackFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	blockedOnlyParam, ok := parseOptionalBoolParam(c.Query("blocked_only"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid blocked_only"})
		return
	}
	blockedOnly := blockedOnlyParam != nil && *blockedOnlyParam

	now := time.Now()
	since := now.Add(-windowDuration)

	actionRows := make([]AdminVideoJobFeedbackActionStat, 0)
	sceneRows := make([]AdminVideoJobFeedbackSceneStat, 0)
	trendRows := make([]AdminVideoJobFeedbackTrendPoint, 0)
	negativeGuardOverview := AdminVideoJobFeedbackNegativeGuardOverview{}
	if !blockedOnly {
		actionRows, err = h.loadVideoImageFeedbackActionStats(since, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		sceneRows, err = h.loadVideoImageFeedbackTopSceneStats(since, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		trendRows, err = h.loadVideoImageFeedbackTrend(since, now, windowDuration, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		negativeGuardOverview, err = h.loadVideoJobFeedbackNegativeGuardOverview(since, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	negativeGuardJobRows, err := h.loadVideoJobFeedbackNegativeGuardJobRows(since, filter, 300, blockedOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{
		"section",
		"window",
		"window_start",
		"window_end",
		"bucket",
		"action",
		"scene_tag",
		"count",
		"ratio",
		"weight_sum",
		"metric",
		"value",
		"total",
		"positive",
		"neutral",
		"negative",
		"top_pick",
		"filter_user_id",
		"filter_format",
		"filter_guard_reason",
		"job_id",
		"job_user_id",
		"job_title",
		"job_group",
		"guard_hit",
		"blocked_reason",
		"before_reason",
		"after_reason",
		"before_start_sec",
		"before_end_sec",
		"after_start_sec",
		"after_end_sec",
		"guard_reason_list",
		"job_finished_at",
	}
	writeRow := func(row []string) error {
		if len(row) < len(header) {
			row = append(row, make([]string, len(header)-len(row))...)
		}
		return writer.Write(row)
	}
	if err := writer.Write(header); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	windowStartText := since.Format(time.RFC3339)
	windowEndText := now.Format(time.RFC3339)
	filterUserID := ""
	filterFormat := ""
	filterGuardReason := ""
	if filter != nil {
		if filter.UserID > 0 {
			filterUserID = strconv.FormatUint(filter.UserID, 10)
		}
		filterFormat = filter.Format
		filterGuardReason = filter.GuardReason
	}
	formatOptionalFloat := func(value *float64) string {
		if value == nil {
			return ""
		}
		return fmt.Sprintf("%.6f", *value)
	}
	formatOptionalTime := func(value *time.Time) string {
		if value == nil || value.IsZero() {
			return ""
		}
		return value.Format(time.RFC3339)
	}

	if err := writeRow([]string{
		"meta",
		windowLabel,
		windowStartText,
		windowEndText,
		"blocked_only",
		strconv.FormatBool(blockedOnly),
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
		filterUserID,
		filterFormat,
		filterGuardReason,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !blockedOnly {
		for _, item := range actionRows {
			if err := writeRow([]string{
				"action",
				windowLabel,
				windowStartText,
				windowEndText,
				"",
				item.Action,
				"",
				strconv.FormatInt(item.Count, 10),
				fmt.Sprintf("%.6f", item.Ratio),
				fmt.Sprintf("%.6f", item.WeightSum),
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				filterUserID,
				filterFormat,
				filterGuardReason,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		for _, item := range sceneRows {
			if err := writeRow([]string{
				"scene",
				windowLabel,
				windowStartText,
				windowEndText,
				"",
				"",
				item.SceneTag,
				strconv.FormatInt(item.Signals, 10),
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				filterUserID,
				filterFormat,
				filterGuardReason,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		for _, item := range trendRows {
			if err := writeRow([]string{
				"trend",
				windowLabel,
				windowStartText,
				windowEndText,
				item.Bucket,
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				strconv.FormatInt(item.Total, 10),
				strconv.FormatInt(item.Positive, 10),
				strconv.FormatInt(item.Neutral, 10),
				strconv.FormatInt(item.Negative, 10),
				strconv.FormatInt(item.TopPick, 10),
				filterUserID,
				filterFormat,
				filterGuardReason,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		negativeGuardMetrics := []struct {
			metric string
			value  string
		}{
			{metric: "samples", value: strconv.FormatInt(negativeGuardOverview.Samples, 10)},
			{metric: "treatment_jobs", value: strconv.FormatInt(negativeGuardOverview.TreatmentJobs, 10)},
			{metric: "guard_enabled_jobs", value: strconv.FormatInt(negativeGuardOverview.GuardEnabledJobs, 10)},
			{metric: "guard_reason_hit_jobs", value: strconv.FormatInt(negativeGuardOverview.GuardReasonHitJobs, 10)},
			{metric: "selection_shift_jobs", value: strconv.FormatInt(negativeGuardOverview.SelectionShiftJobs, 10)},
			{metric: "blocked_reason_jobs", value: strconv.FormatInt(negativeGuardOverview.BlockedReasonJobs, 10)},
			{metric: "guard_hit_rate", value: fmt.Sprintf("%.6f", negativeGuardOverview.GuardHitRate)},
			{metric: "selection_shift_rate", value: fmt.Sprintf("%.6f", negativeGuardOverview.SelectionShiftRate)},
			{metric: "blocked_reason_rate", value: fmt.Sprintf("%.6f", negativeGuardOverview.BlockedReasonRate)},
			{metric: "avg_negative_signals", value: fmt.Sprintf("%.6f", negativeGuardOverview.AvgNegativeSignals)},
			{metric: "avg_positive_signals", value: fmt.Sprintf("%.6f", negativeGuardOverview.AvgPositiveSignals)},
		}
		for _, item := range negativeGuardMetrics {
			if err := writeRow([]string{
				"negative_guard_overview",
				windowLabel,
				windowStartText,
				windowEndText,
				"",
				"",
				"",
				"",
				"",
				"",
				item.metric,
				item.value,
				"",
				"",
				"",
				"",
				"",
				filterUserID,
				filterFormat,
				filterGuardReason,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	for _, item := range negativeGuardJobRows {
		if err := writeRow([]string{
			"negative_guard_job_detail",
			windowLabel,
			windowStartText,
			windowEndText,
			"",
			"",
			"",
			"",
			"",
			"",
			"blocked_reason",
			strconv.FormatBool(item.BlockedReason),
			"",
			"",
			"",
			"",
			"",
			filterUserID,
			filterFormat,
			filterGuardReason,
			strconv.FormatUint(item.JobID, 10),
			strconv.FormatUint(item.UserID, 10),
			item.Title,
			item.Group,
			strconv.FormatBool(item.GuardHit),
			strconv.FormatBool(item.BlockedReason),
			item.BeforeReason,
			item.AfterReason,
			formatOptionalFloat(item.BeforeStartSec),
			formatOptionalFloat(item.BeforeEndSec),
			formatOptionalFloat(item.AfterStartSec),
			formatOptionalFloat(item.AfterEndSec),
			item.GuardReasonList,
			formatOptionalTime(item.FinishedAt),
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

	filenamePrefix := "video_jobs_feedback_report"
	if blockedOnly {
		filenamePrefix = "video_jobs_feedback_blocked_report"
	}
	filename := fmt.Sprintf("%s_%s_%s.csv", filenamePrefix, windowLabel, now.Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAdminVideoJobsFeedbackIntegrityCSV godoc
// @Summary Export video-jobs feedback integrity CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param user_id query int false "user id filter"
// @Param format query string false "requested format filter"
// @Param guard_reason query string false "negative guard reason filter"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/feedback-integrity.csv [get]
func (h *Handler) ExportAdminVideoJobsFeedbackIntegrityCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter, err := parseVideoImageFeedbackFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	since := now.Add(-windowDuration)
	overview, err := h.loadVideoImageFeedbackIntegrityOverview(since, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	thresholds := defaultFeedbackIntegrityAlertThresholdSettings()
	if setting, loadErr := h.loadVideoQualitySetting(); loadErr == nil {
		thresholds = feedbackIntegrityAlertThresholdSettingsFromModel(setting)
	}
	alertRows, health := buildVideoJobFeedbackIntegrityAlerts(overview, thresholds)
	actionRows, err := h.loadVideoImageFeedbackIntegrityActionRows(since, filter, 24)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filterUserID := ""
	filterFormat := ""
	filterGuardReason := ""
	if filter != nil {
		if filter.UserID > 0 {
			filterUserID = strconv.FormatUint(filter.UserID, 10)
		}
		filterFormat = filter.Format
		filterGuardReason = filter.GuardReason
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{
		"section",
		"window",
		"window_start",
		"window_end",
		"action",
		"samples",
		"with_output_id",
		"missing_output_id",
		"resolved_output",
		"orphan_output",
		"job_mismatch",
		"top_pick_multi_hit_users",
		"output_coverage_rate",
		"output_resolved_rate",
		"output_job_consistency_rate",
		"health",
		"alert_code",
		"alert_message",
		"metric_value",
		"warn_threshold",
		"critical_threshold",
		"filter_user_id",
		"filter_format",
		"filter_guard_reason",
	}
	if err := writer.Write(header); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	writeRow := func(row []string) error {
		if len(row) < len(header) {
			row = append(row, make([]string, len(header)-len(row))...)
		}
		return writer.Write(row)
	}
	windowStartText := since.Format(time.RFC3339)
	windowEndText := now.Format(time.RFC3339)
	if err := writeRow([]string{
		"summary",
		windowLabel,
		windowStartText,
		windowEndText,
		"all",
		strconv.FormatInt(overview.Samples, 10),
		strconv.FormatInt(overview.WithOutputID, 10),
		strconv.FormatInt(overview.MissingOutputID, 10),
		strconv.FormatInt(overview.ResolvedOutput, 10),
		strconv.FormatInt(overview.OrphanOutput, 10),
		strconv.FormatInt(overview.JobMismatch, 10),
		strconv.FormatInt(overview.TopPickMultiHitUsers, 10),
		fmt.Sprintf("%.6f", overview.OutputCoverageRate),
		fmt.Sprintf("%.6f", overview.OutputResolvedRate),
		fmt.Sprintf("%.6f", overview.OutputJobConsistencyRate),
		health,
		"",
		"",
		"",
		"",
		"",
		filterUserID,
		filterFormat,
		filterGuardReason,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, item := range actionRows {
		if err := writeRow([]string{
			"action",
			windowLabel,
			windowStartText,
			windowEndText,
			item.Action,
			strconv.FormatInt(item.Samples, 10),
			strconv.FormatInt(item.WithOutputID, 10),
			strconv.FormatInt(item.MissingOutputID, 10),
			strconv.FormatInt(item.ResolvedOutput, 10),
			strconv.FormatInt(item.OrphanOutput, 10),
			strconv.FormatInt(item.JobMismatch, 10),
			"",
			fmt.Sprintf("%.6f", item.OutputCoverageRate),
			fmt.Sprintf("%.6f", item.OutputResolvedRate),
			fmt.Sprintf("%.6f", item.OutputJobConsistencyRate),
			"",
			"",
			"",
			"",
			"",
			"",
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	thresholdRows := []struct {
		key      string
		value    string
		warn     string
		critical string
	}{
		{
			key:      "output_coverage_rate",
			value:    fmt.Sprintf("%.6f", overview.OutputCoverageRate),
			warn:     fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputCoverageRateWarn),
			critical: fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputCoverageRateCritical),
		},
		{
			key:      "output_resolved_rate",
			value:    fmt.Sprintf("%.6f", overview.OutputResolvedRate),
			warn:     fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputResolvedRateWarn),
			critical: fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputResolvedRateCritical),
		},
		{
			key:      "output_job_consistency_rate",
			value:    fmt.Sprintf("%.6f", overview.OutputJobConsistencyRate),
			warn:     fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputJobConsistencyRateWarn),
			critical: fmt.Sprintf("%.6f", thresholds.FeedbackIntegrityOutputJobConsistencyRateCritical),
		},
		{
			key:      "top_pick_multi_hit_users",
			value:    strconv.FormatInt(overview.TopPickMultiHitUsers, 10),
			warn:     strconv.Itoa(thresholds.FeedbackIntegrityTopPickConflictUsersWarn),
			critical: strconv.Itoa(thresholds.FeedbackIntegrityTopPickConflictUsersCritical),
		},
	}
	for _, item := range thresholdRows {
		if err := writeRow([]string{
			"threshold",
			windowLabel,
			windowStartText,
			windowEndText,
			item.key,
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
			item.value,
			item.warn,
			item.critical,
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	for _, item := range alertRows {
		if err := writeRow([]string{
			"alert",
			windowLabel,
			windowStartText,
			windowEndText,
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
			item.Level,
			item.Code,
			item.Message,
			"",
			"",
			"",
			filterUserID,
			filterFormat,
			filterGuardReason,
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

	filename := fmt.Sprintf("video_jobs_feedback_integrity_%s_%s.csv", windowLabel, now.Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAdminVideoJobsFeedbackIntegrityTrendCSV godoc
// @Summary Export video-jobs feedback integrity trend CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param user_id query int false "user id filter"
// @Param format query string false "requested format filter"
// @Param guard_reason query string false "negative guard reason filter"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/feedback-integrity-trend.csv [get]
func (h *Handler) ExportAdminVideoJobsFeedbackIntegrityTrendCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter, err := parseVideoImageFeedbackFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	since := now.Add(-windowDuration)
	thresholds := defaultFeedbackIntegrityAlertThresholdSettings()
	if setting, loadErr := h.loadVideoQualitySetting(); loadErr == nil {
		thresholds = feedbackIntegrityAlertThresholdSettingsFromModel(setting)
	}
	trend, err := h.loadVideoImageFeedbackIntegrityHealthTrend(since, now, thresholds, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	alertCodeStats := buildFeedbackIntegrityAlertCodeStats(trend)
	escalationTrend := buildFeedbackIntegrityEscalationTrend(trend)
	escalationIncidents := buildFeedbackIntegrityEscalationIncidents(escalationTrend, len(escalationTrend))
	streaks := buildFeedbackIntegrityStreaks(trend)
	recoveryStatus, recovered, previousHealth := resolveFeedbackIntegrityRecovery(trend)
	previousWindowStart := since.Add(-windowDuration)
	previousWindowEnd := since
	currentOverview, currentOverviewErr := h.loadVideoImageFeedbackIntegrityOverviewRange(since, now, filter)
	if currentOverviewErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": currentOverviewErr.Error()})
		return
	}
	previousOverview, previousOverviewErr := h.loadVideoImageFeedbackIntegrityOverviewRange(previousWindowStart, previousWindowEnd, filter)
	if previousOverviewErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": previousOverviewErr.Error()})
		return
	}
	currentAlerts, currentHealth := buildVideoJobFeedbackIntegrityAlerts(currentOverview, thresholds)
	previousAlerts, previousHealthForDelta := buildVideoJobFeedbackIntegrityAlerts(previousOverview, thresholds)
	delta := buildFeedbackIntegrityDelta(
		currentOverview,
		currentHealth,
		countEffectiveFeedbackIntegrityAlerts(currentAlerts),
		previousOverview,
		previousHealthForDelta,
		countEffectiveFeedbackIntegrityAlerts(previousAlerts),
		previousWindowStart,
		previousWindowEnd,
	)
	escalation := buildFeedbackIntegrityEscalation(currentHealth, streaks, delta, recoveryStatus)

	filterUserID := ""
	filterFormat := ""
	filterGuardReason := ""
	if filter != nil {
		if filter.UserID > 0 {
			filterUserID = strconv.FormatUint(filter.UserID, 10)
		}
		filterFormat = filter.Format
		filterGuardReason = filter.GuardReason
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{
		"section",
		"window",
		"window_start",
		"window_end",
		"bucket",
		"samples",
		"health",
		"alert_count",
		"output_coverage_rate",
		"output_resolved_rate",
		"output_job_consistency_rate",
		"top_pick_multi_hit_users",
		"recovery_status",
		"recovered",
		"previous_health",
		"alert_code",
		"days_hit",
		"latest_bucket",
		"latest_level",
		"escalation_level",
		"escalation_required",
		"escalation_reason",
		"escalation_triggered_rules",
		"filter_user_id",
		"filter_format",
		"filter_guard_reason",
	}
	if err := writer.Write(header); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	writeRow := func(row []string) error {
		if len(row) < len(header) {
			row = append(row, make([]string, len(header)-len(row))...)
		}
		return writer.Write(row)
	}

	windowStartText := since.Format(time.RFC3339)
	windowEndText := now.Format(time.RFC3339)
	if err := writeRow([]string{
		"summary",
		windowLabel,
		windowStartText,
		windowEndText,
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		recoveryStatus,
		strconv.FormatBool(recovered),
		previousHealth,
		"",
		"",
		"",
		"",
		escalation.Level,
		strconv.FormatBool(escalation.Required),
		escalation.Reason,
		strings.Join(escalation.TriggeredRules, "|"),
		filterUserID,
		filterFormat,
		filterGuardReason,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	escalationTrendByBucket := make(map[string]AdminVideoJobFeedbackIntegrityEscalationTrendPoint, len(escalationTrend))
	for _, point := range escalationTrend {
		bucket := strings.TrimSpace(point.Bucket)
		if bucket == "" {
			continue
		}
		escalationTrendByBucket[bucket] = point
	}
	for _, item := range trend {
		escalationPoint := escalationTrendByBucket[strings.TrimSpace(item.Bucket)]
		if err := writeRow([]string{
			"trend_day",
			windowLabel,
			windowStartText,
			windowEndText,
			item.Bucket,
			strconv.FormatInt(item.Samples, 10),
			item.Health,
			strconv.Itoa(item.AlertCount),
			fmt.Sprintf("%.6f", item.OutputCoverageRate),
			fmt.Sprintf("%.6f", item.OutputResolvedRate),
			fmt.Sprintf("%.6f", item.OutputJobConsistencyRate),
			strconv.FormatInt(item.TopPickMultiHitUsers, 10),
			escalationPoint.RecoveryStatus,
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			escalationPoint.EscalationLevel,
			strconv.FormatBool(escalationPoint.EscalationRequired),
			escalationPoint.EscalationReason,
			strings.Join(escalationPoint.TriggeredRules, "|"),
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	for _, item := range escalationIncidents {
		if err := writeRow([]string{
			"escalation_incident",
			windowLabel,
			windowStartText,
			windowEndText,
			item.Bucket,
			"",
			"",
			strconv.Itoa(item.AlertCount),
			"",
			"",
			"",
			strconv.FormatInt(item.TopPickMultiHitUsers, 10),
			item.RecoveryStatus,
			"",
			"",
			"",
			"",
			"",
			"",
			item.EscalationLevel,
			strconv.FormatBool(item.EscalationRequired),
			item.EscalationReason,
			strings.Join(item.TriggeredRules, "|"),
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	for _, item := range alertCodeStats {
		if err := writeRow([]string{
			"alert_code_stat",
			windowLabel,
			windowStartText,
			windowEndText,
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
			item.Code,
			strconv.Itoa(item.DaysHit),
			item.LatestBucket,
			item.LatestLevel,
			"",
			"",
			"",
			"",
			filterUserID,
			filterFormat,
			filterGuardReason,
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

	filename := fmt.Sprintf("video_jobs_feedback_integrity_trend_%s_%s.csv", windowLabel, now.Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// GetAdminVideoJobsFeedbackIntegrityDrilldown godoc
// @Summary Get feedback-integrity drilldown rows (admin)
// @Tags admin
// @Produce json
// @Param window query string false "window: 24h | 7d | 30d"
// @Param user_id query int false "user id filter"
// @Param format query string false "requested format filter"
// @Param guard_reason query string false "negative guard reason filter"
// @Param limit query int false "rows per section, default 20, max 100"
// @Success 200 {object} AdminVideoJobFeedbackIntegrityDrilldownResponse
// @Router /api/admin/video-jobs/feedback-integrity/drilldown [get]
func (h *Handler) GetAdminVideoJobsFeedbackIntegrityDrilldown(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter, err := parseVideoImageFeedbackFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		v, parseErr := strconv.Atoi(raw)
		if parseErr != nil || v <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		if v > 100 {
			v = 100
		}
		limit = v
	}

	now := time.Now()
	since := now.Add(-windowDuration)
	tables := resolveVideoImageReadTablesByFilter(filter)
	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQLWithTables(filter, "f", tables.Jobs)

	type anomalyRow struct {
		JobID            uint64     `gorm:"column:job_id"`
		UserID           uint64     `gorm:"column:user_id"`
		Title            string     `gorm:"column:title"`
		Status           string     `gorm:"column:status"`
		Stage            string     `gorm:"column:stage"`
		AnomalyCount     int64      `gorm:"column:anomaly_count"`
		LatestFeedbackAt *time.Time `gorm:"column:latest_feedback_at"`
	}
	anomalyQuery := fmt.Sprintf(`
WITH base AS (
	SELECT
		f.id,
		f.job_id,
		f.user_id,
		f.output_id,
		f.created_at
	FROM %s f
	WHERE f.created_at >= ?
`+filterSQL+`
),
joined AS (
	SELECT
		b.*,
		o.id AS output_exists_id,
		o.job_id AS output_job_id
	FROM base b
	LEFT JOIN %s o ON o.id = b.output_id
)
SELECT
	j.id::bigint AS job_id,
	j.user_id::bigint AS user_id,
	COALESCE(j.title, '') AS title,
	COALESCE(j.status, '') AS status,
	COALESCE(j.stage, '') AS stage,
	COUNT(*)::bigint AS anomaly_count,
	MAX(joined.created_at) AS latest_feedback_at
FROM joined
JOIN %s j ON j.id = joined.job_id
WHERE joined.output_id IS NULL
	OR (joined.output_id IS NOT NULL AND joined.output_exists_id IS NULL)
	OR (joined.output_id IS NOT NULL AND joined.output_exists_id IS NOT NULL AND joined.output_job_id <> joined.job_id)
GROUP BY j.id, j.user_id, j.title, j.status, j.stage
ORDER BY anomaly_count DESC, latest_feedback_at DESC, j.id DESC
LIMIT ?
`, tables.Feedback, tables.Outputs, tables.Jobs)
	anomalyArgs := []interface{}{since}
	anomalyArgs = append(anomalyArgs, filterArgs...)
	anomalyArgs = append(anomalyArgs, limit)
	var anomalyRows []anomalyRow
	if err := h.db.Raw(anomalyQuery, anomalyArgs...).Scan(&anomalyRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type topPickConflictRow struct {
		JobID                  uint64     `gorm:"column:job_id"`
		UserID                 uint64     `gorm:"column:user_id"`
		Title                  string     `gorm:"column:title"`
		Status                 string     `gorm:"column:status"`
		Stage                  string     `gorm:"column:stage"`
		TopPickConflictUsers   int64      `gorm:"column:top_pick_conflict_users"`
		TopPickConflictActions int64      `gorm:"column:top_pick_conflict_actions"`
		LatestFeedbackAt       *time.Time `gorm:"column:latest_feedback_at"`
	}
	topPickQuery := fmt.Sprintf(`
WITH base AS (
	SELECT
		f.job_id,
		f.user_id,
		LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action,
		f.created_at
	FROM %s f
	WHERE f.created_at >= ?
`+filterSQL+`
),
conflict AS (
	SELECT
		job_id,
		user_id,
		COUNT(*)::bigint AS conflict_count,
		MAX(created_at) AS latest_feedback_at
	FROM base
	WHERE action = 'top_pick'
	GROUP BY job_id, user_id
	HAVING COUNT(*) > 1
),
agg AS (
	SELECT
		job_id,
		COUNT(*)::bigint AS top_pick_conflict_users,
		COALESCE(SUM(conflict_count), 0)::bigint AS top_pick_conflict_actions,
		MAX(latest_feedback_at) AS latest_feedback_at
	FROM conflict
	GROUP BY job_id
)
SELECT
	j.id::bigint AS job_id,
	j.user_id::bigint AS user_id,
	COALESCE(j.title, '') AS title,
	COALESCE(j.status, '') AS status,
	COALESCE(j.stage, '') AS stage,
	agg.top_pick_conflict_users,
	agg.top_pick_conflict_actions,
	agg.latest_feedback_at
FROM agg
JOIN %s j ON j.id = agg.job_id
ORDER BY agg.top_pick_conflict_users DESC, agg.top_pick_conflict_actions DESC, agg.latest_feedback_at DESC, agg.job_id DESC
LIMIT ?
`, tables.Feedback, tables.Jobs)
	topPickArgs := []interface{}{since}
	topPickArgs = append(topPickArgs, filterArgs...)
	topPickArgs = append(topPickArgs, limit)
	var topPickRows []topPickConflictRow
	if err := h.db.Raw(topPickQuery, topPickArgs...).Scan(&topPickRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	formatOptionalTime := func(value *time.Time) string {
		if value == nil || value.IsZero() {
			return ""
		}
		return value.Format(time.RFC3339)
	}
	anomalyJobs := make([]AdminVideoJobFeedbackIntegrityDrilldownRow, 0, len(anomalyRows))
	for _, item := range anomalyRows {
		anomalyJobs = append(anomalyJobs, AdminVideoJobFeedbackIntegrityDrilldownRow{
			JobID:            item.JobID,
			UserID:           item.UserID,
			Title:            strings.TrimSpace(item.Title),
			Status:           strings.TrimSpace(item.Status),
			Stage:            strings.TrimSpace(item.Stage),
			AnomalyCount:     item.AnomalyCount,
			LatestFeedbackAt: formatOptionalTime(item.LatestFeedbackAt),
		})
	}
	topPickJobs := make([]AdminVideoJobFeedbackIntegrityDrilldownRow, 0, len(topPickRows))
	for _, item := range topPickRows {
		topPickJobs = append(topPickJobs, AdminVideoJobFeedbackIntegrityDrilldownRow{
			JobID:                  item.JobID,
			UserID:                 item.UserID,
			Title:                  strings.TrimSpace(item.Title),
			Status:                 strings.TrimSpace(item.Status),
			Stage:                  strings.TrimSpace(item.Stage),
			TopPickConflictUsers:   item.TopPickConflictUsers,
			TopPickConflictActions: item.TopPickConflictActions,
			LatestFeedbackAt:       formatOptionalTime(item.LatestFeedbackAt),
		})
	}

	response := AdminVideoJobFeedbackIntegrityDrilldownResponse{
		Window:              windowLabel,
		WindowStart:         since.Format(time.RFC3339),
		WindowEnd:           now.Format(time.RFC3339),
		AnomalyJobs:         anomalyJobs,
		TopPickConflictJobs: topPickJobs,
	}
	if filter != nil {
		response.FilterUserID = filter.UserID
		response.FilterFormat = filter.Format
		response.FilterGuardReason = filter.GuardReason
	}

	c.JSON(http.StatusOK, response)
}

// ExportAdminVideoJobsFeedbackIntegrityAnomaliesCSV godoc
// @Summary Export video-jobs feedback integrity anomalies CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Param user_id query int false "user id filter"
// @Param format query string false "requested format filter"
// @Param guard_reason query string false "negative guard reason filter"
// @Param limit query int false "max rows per section, default 300, max 2000"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/feedback-integrity-anomalies.csv [get]
func (h *Handler) ExportAdminVideoJobsFeedbackIntegrityAnomaliesCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter, err := parseVideoImageFeedbackFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	limit := 300
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		v, parseErr := strconv.Atoi(raw)
		if parseErr != nil || v <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		if v > 2000 {
			v = 2000
		}
		limit = v
	}

	now := time.Now()
	since := now.Add(-windowDuration)
	tables := resolveVideoImageReadTablesByFilter(filter)
	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQLWithTables(filter, "f", tables.Jobs)

	type anomalyRow struct {
		FeedbackID  uint64    `gorm:"column:feedback_id"`
		JobID       uint64    `gorm:"column:job_id"`
		UserID      uint64    `gorm:"column:user_id"`
		OutputID    *uint64   `gorm:"column:output_id"`
		Action      string    `gorm:"column:action"`
		Anomaly     string    `gorm:"column:anomaly"`
		OutputJobID *uint64   `gorm:"column:output_job_id"`
		ObjectKey   string    `gorm:"column:object_key"`
		CreatedAt   time.Time `gorm:"column:created_at"`
	}
	anomalyQuery := fmt.Sprintf(`
WITH base AS (
	SELECT
		f.id,
		f.job_id,
		f.user_id,
		f.output_id,
		LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action,
		f.created_at
	FROM %s f
	WHERE f.created_at >= ?
`+filterSQL+`
),
joined AS (
	SELECT
		b.*,
		o.id AS output_exists_id,
		o.job_id AS output_job_id,
		o.object_key
	FROM base b
	LEFT JOIN %s o ON o.id = b.output_id
)
SELECT
	id::bigint AS feedback_id,
	job_id::bigint AS job_id,
	user_id::bigint AS user_id,
	output_id::bigint AS output_id,
	action,
	CASE
		WHEN output_id IS NULL THEN 'missing_output_id'
		WHEN output_id IS NOT NULL AND output_exists_id IS NULL THEN 'orphan_output'
		WHEN output_id IS NOT NULL AND output_exists_id IS NOT NULL AND output_job_id <> job_id THEN 'job_mismatch'
		ELSE 'ok'
	END AS anomaly,
	output_job_id::bigint AS output_job_id,
	COALESCE(object_key, '') AS object_key,
	created_at
FROM joined
WHERE output_id IS NULL
	OR (output_id IS NOT NULL AND output_exists_id IS NULL)
	OR (output_id IS NOT NULL AND output_exists_id IS NOT NULL AND output_job_id <> job_id)
ORDER BY created_at DESC
LIMIT ?
`, tables.Feedback, tables.Outputs)
	anomalyArgs := []interface{}{since}
	anomalyArgs = append(anomalyArgs, filterArgs...)
	anomalyArgs = append(anomalyArgs, limit)

	var anomalyRows []anomalyRow
	if err := h.db.Raw(anomalyQuery, anomalyArgs...).Scan(&anomalyRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type topPickConflictRow struct {
		JobID         uint64    `gorm:"column:job_id"`
		UserID        uint64    `gorm:"column:user_id"`
		ConflictCount int64     `gorm:"column:conflict_count"`
		CreatedAt     time.Time `gorm:"column:created_at"`
	}
	topPickQuery := fmt.Sprintf(`
WITH base AS (
	SELECT
		f.job_id,
		f.user_id,
		LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action,
		f.created_at
	FROM %s f
	WHERE f.created_at >= ?
`+filterSQL+`
)
SELECT
	job_id::bigint AS job_id,
	user_id::bigint AS user_id,
	COUNT(*)::bigint AS conflict_count,
	MAX(created_at) AS created_at
FROM base
WHERE action = 'top_pick'
GROUP BY job_id, user_id
HAVING COUNT(*) > 1
ORDER BY conflict_count DESC, created_at DESC
LIMIT ?
`, tables.Feedback)
	topPickArgs := []interface{}{since}
	topPickArgs = append(topPickArgs, filterArgs...)
	topPickArgs = append(topPickArgs, limit)

	var topPickRows []topPickConflictRow
	if err := h.db.Raw(topPickQuery, topPickArgs...).Scan(&topPickRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filterUserID := ""
	filterFormat := ""
	filterGuardReason := ""
	if filter != nil {
		if filter.UserID > 0 {
			filterUserID = strconv.FormatUint(filter.UserID, 10)
		}
		filterFormat = filter.Format
		filterGuardReason = filter.GuardReason
	}
	formatOptionalUint := func(value *uint64) string {
		if value == nil || *value == 0 {
			return ""
		}
		return strconv.FormatUint(*value, 10)
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{
		"section",
		"window",
		"window_start",
		"window_end",
		"feedback_id",
		"job_id",
		"user_id",
		"output_id",
		"action",
		"anomaly",
		"output_job_id",
		"object_key",
		"conflict_count",
		"created_at",
		"filter_user_id",
		"filter_format",
		"filter_guard_reason",
	}
	if err := writer.Write(header); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	writeRow := func(row []string) error {
		if len(row) < len(header) {
			row = append(row, make([]string, len(header)-len(row))...)
		}
		return writer.Write(row)
	}

	windowStartText := since.Format(time.RFC3339)
	windowEndText := now.Format(time.RFC3339)
	for _, item := range anomalyRows {
		if err := writeRow([]string{
			"feedback_anomaly",
			windowLabel,
			windowStartText,
			windowEndText,
			strconv.FormatUint(item.FeedbackID, 10),
			strconv.FormatUint(item.JobID, 10),
			strconv.FormatUint(item.UserID, 10),
			formatOptionalUint(item.OutputID),
			item.Action,
			item.Anomaly,
			formatOptionalUint(item.OutputJobID),
			item.ObjectKey,
			"",
			item.CreatedAt.Format(time.RFC3339),
			filterUserID,
			filterFormat,
			filterGuardReason,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	for _, item := range topPickRows {
		if err := writeRow([]string{
			"top_pick_conflict",
			windowLabel,
			windowStartText,
			windowEndText,
			"",
			strconv.FormatUint(item.JobID, 10),
			strconv.FormatUint(item.UserID, 10),
			"",
			"top_pick",
			"top_pick_conflict",
			"",
			"",
			strconv.FormatInt(item.ConflictCount, 10),
			item.CreatedAt.Format(time.RFC3339),
			filterUserID,
			filterFormat,
			filterGuardReason,
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

	filename := fmt.Sprintf("video_jobs_feedback_integrity_anomalies_%s_%s.csv", windowLabel, now.Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}
