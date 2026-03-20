package handlers

import (
	"net/http"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
)

// GetAdminVideoJobsOverview godoc
// @Summary Get video jobs overview (admin)
// @Tags admin
// @Produce json
// @Param window query string false "window: 24h | 7d | 30d"
// @Success 200 {object} AdminVideoJobOverviewResponse
// @Router /api/admin/video-jobs/overview [get]
func (h *Handler) GetAdminVideoJobsOverview(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	sinceWindow := now.Add(-windowDuration)
	since24h := now.Add(-24 * time.Hour)
	out := AdminVideoJobOverviewResponse{
		Window:                           windowLabel,
		WindowStart:                      sinceWindow.Format(time.RFC3339),
		WindowEnd:                        now.Format(time.RFC3339),
		FeedbackIntegrityAlertThresholds: defaultFeedbackIntegrityAlertThresholdSettings(),
		FeedbackIntegrityHealth:          "green",
		FeedbackIntegrityAlerts:          make([]AdminVideoJobFeedbackIntegrityAlert, 0, 8),
		FeedbackIntegrityHealthTrend:     make([]AdminVideoJobFeedbackIntegrityHealthTrendPoint, 0, 32),
		FeedbackIntegrityAlertCodeStats:  make([]AdminVideoJobFeedbackIntegrityAlertCodeStat, 0, 16),
		FeedbackIntegrityRecoveryStatus:  "no_data",
		FeedbackIntegrityRecommendations: make([]AdminVideoJobFeedbackIntegrityRecommendation, 0, 4),
		FeedbackIntegrityEscalation: AdminVideoJobFeedbackIntegrityEscalation{
			Required: false,
			Level:    "none",
		},
		GIFSubStageAnomalyOverview: make([]AdminVideoJobGIFSubStageAnomalyStat, 0, len(videoJobGIFSubStageOrder)),
		GIFSubStageAnomalyReasons:  make([]AdminVideoJobGIFSubStageAnomalyReasonStat, 0, 12),
	}
	if setting, loadErr := h.loadVideoQualitySetting(); loadErr == nil {
		out.FeedbackIntegrityAlertThresholds = feedbackIntegrityAlertThresholdSettingsFromModel(setting)
	}

	if err := h.db.Model(&models.VideoJob{}).Count(&out.Total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type statusRow struct {
		Status string
		Count  int64
	}
	var statusRows []statusRow
	if err := h.db.Model(&models.VideoJob{}).
		Select("status, count(*) AS count").
		Group("status").
		Scan(&statusRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, row := range statusRows {
		switch strings.ToLower(strings.TrimSpace(row.Status)) {
		case models.VideoJobStatusQueued:
			out.Queued = row.Count
		case models.VideoJobStatusRunning:
			out.Running = row.Count
		case models.VideoJobStatusDone:
			out.Done = row.Count
		case models.VideoJobStatusFailed:
			out.Failed = row.Count
		case models.VideoJobStatusCancelled:
			out.Cancelled = row.Count
		}
	}

	if err := h.db.Model(&models.VideoJob{}).
		Where("stage = ?", models.VideoJobStageRetrying).
		Count(&out.Retrying).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Model(&models.VideoJob{}).
		Where("created_at >= ?", since24h).
		Count(&out.Created24h).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Model(&models.VideoJob{}).
		Where("status = ? AND finished_at >= ?", models.VideoJobStatusDone, since24h).
		Count(&out.Done24h).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Model(&models.VideoJob{}).
		Where("status = ? AND updated_at >= ?", models.VideoJobStatusFailed, since24h).
		Count(&out.Failed24h).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Model(&models.VideoJob{}).
		Where("created_at >= ?", sinceWindow).
		Count(&out.CreatedWindow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Model(&models.VideoJob{}).
		Where("status = ? AND finished_at >= ?", models.VideoJobStatusDone, sinceWindow).
		Count(&out.DoneWindow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Model(&models.VideoJob{}).
		Where("status = ? AND updated_at >= ?", models.VideoJobStatusFailed, sinceWindow).
		Count(&out.FailedWindow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sourceProbeJobsWindow, durationBuckets, resolutionBuckets, fpsBuckets, err := h.loadVideoJobSourceProbeBuckets(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.SourceProbeJobsWindow = sourceProbeJobsWindow
	out.SourceProbeDurationBuckets = durationBuckets
	out.SourceProbeResolutionBuckets = resolutionBuckets
	out.SourceProbeFpsBuckets = fpsBuckets
	durationQuality, resolutionQuality, fpsQuality, err := h.loadVideoJobSourceProbeQualityStats(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.SourceProbeDurationQuality = durationQuality
	out.SourceProbeResolutionQuality = resolutionQuality
	out.SourceProbeFpsQuality = fpsQuality

	type sampleWindowRow struct {
		SampleJobsWindow   int64 `gorm:"column:sample_jobs_window"`
		SampleDoneWindow   int64 `gorm:"column:sample_done_window"`
		SampleFailedWindow int64 `gorm:"column:sample_failed_window"`
	}
	var sampleWindow sampleWindowRow
	if err := h.db.Raw(`
SELECT
	COUNT(*) FILTER (WHERE v.created_at >= ?) AS sample_jobs_window,
	COUNT(*) FILTER (WHERE v.status = ? AND v.finished_at >= ?) AS sample_done_window,
	COUNT(*) FILTER (WHERE v.status = ? AND v.updated_at >= ?) AS sample_failed_window
FROM archive.video_jobs v
JOIN archive.collections c ON c.id = v.result_collection_id
WHERE c.is_sample = TRUE
`,
		sinceWindow,
		models.VideoJobStatusDone,
		sinceWindow,
		models.VideoJobStatusFailed,
		sinceWindow,
	).Scan(&sampleWindow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.SampleJobsWindow = sampleWindow.SampleJobsWindow
	out.SampleDoneWindow = sampleWindow.SampleDoneWindow
	out.SampleFailedWindow = sampleWindow.SampleFailedWindow
	if out.SampleJobsWindow > 0 {
		out.SampleSuccessRateWindow = float64(out.SampleDoneWindow) / float64(out.SampleJobsWindow)
	}

	type stageRow struct {
		Key   string
		Count int64
	}
	var stageRows []stageRow
	if err := h.db.Model(&models.VideoJob{}).
		Select("stage AS key, count(*) AS count").
		Group("stage").
		Order("count DESC").
		Scan(&stageRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.StageCounts = make([]AdminVideoJobSimpleCount, 0, len(stageRows))
	for _, row := range stageRows {
		out.StageCounts = append(out.StageCounts, AdminVideoJobSimpleCount{
			Key:   strings.TrimSpace(row.Key),
			Count: row.Count,
		})
	}

	type failureRow struct {
		Reason string
		Count  int64
	}
	var failureRows []failureRow
	if err := h.db.Model(&models.VideoJob{}).
		Select("error_message AS reason, count(*) AS count").
		Where("status = ? AND error_message <> ''", models.VideoJobStatusFailed).
		Group("error_message").
		Order("count DESC").
		Limit(6).
		Scan(&failureRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.TopFailures = make([]AdminVideoJobFailureReason, 0, len(failureRows))
	for _, row := range failureRows {
		out.TopFailures = append(out.TopFailures, AdminVideoJobFailureReason{
			Reason: strings.TrimSpace(row.Reason),
			Count:  row.Count,
		})
	}

	stageDurations, err := h.loadVideoJobStageDurations(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.StageDurations = stageDurations
	gifSubStageAnomalyOverview, gifSubStageAnomalyJobsWindow, err := h.loadVideoJobGIFSubStageAnomalyOverview(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFSubStageAnomalyOverview = gifSubStageAnomalyOverview
	out.GIFSubStageAnomalyJobsWindow = gifSubStageAnomalyJobsWindow
	gifSubStageAnomalyReasons, err := h.loadVideoJobGIFSubStageAnomalyReasons(sinceWindow, 18)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFSubStageAnomalyReasons = gifSubStageAnomalyReasons

	formatStats24h, err := h.loadVideoJobFormatStats24h(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FormatStats24h = formatStats24h

	feedbackSignals, feedbackDownloads, feedbackFavorites, feedbackEngagedJobs, feedbackAvgScore, err := h.loadVideoJobFeedbackOverview(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackSignalsWindow = feedbackSignals
	out.FeedbackDownloadsWindow = feedbackDownloads
	out.FeedbackFavoritesWindow = feedbackFavorites
	out.FeedbackEngagedJobsWindow = feedbackEngagedJobs
	out.FeedbackAvgScoreWindow = feedbackAvgScore
	feedbackIntegrity, err := h.loadVideoImageFeedbackIntegrityOverview(sinceWindow, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackIntegrityOverview = feedbackIntegrity
	out.FeedbackIntegrityAlerts, out.FeedbackIntegrityHealth = buildVideoJobFeedbackIntegrityAlerts(
		feedbackIntegrity,
		out.FeedbackIntegrityAlertThresholds,
	)
	out.FeedbackIntegrityRecommendations = buildFeedbackIntegrityRecommendations(
		out.FeedbackIntegrityAlerts,
		out.FeedbackIntegrityHealth,
	)
	previousWindowStart := sinceWindow.Add(-windowDuration)
	previousWindowEnd := sinceWindow
	previousFeedbackIntegrity, err := h.loadVideoImageFeedbackIntegrityOverviewRange(previousWindowStart, previousWindowEnd, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	previousAlerts, previousHealth := buildVideoJobFeedbackIntegrityAlerts(
		previousFeedbackIntegrity,
		out.FeedbackIntegrityAlertThresholds,
	)
	out.FeedbackIntegrityDelta = buildFeedbackIntegrityDelta(
		feedbackIntegrity,
		out.FeedbackIntegrityHealth,
		countEffectiveFeedbackIntegrityAlerts(out.FeedbackIntegrityAlerts),
		previousFeedbackIntegrity,
		previousHealth,
		countEffectiveFeedbackIntegrityAlerts(previousAlerts),
		previousWindowStart,
		previousWindowEnd,
	)
	feedbackHealthTrend, err := h.loadVideoImageFeedbackIntegrityHealthTrend(
		sinceWindow,
		now,
		out.FeedbackIntegrityAlertThresholds,
		nil,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackIntegrityHealthTrend = feedbackHealthTrend
	out.FeedbackIntegrityAlertCodeStats = buildFeedbackIntegrityAlertCodeStats(feedbackHealthTrend)
	out.FeedbackIntegrityStreaks = buildFeedbackIntegrityStreaks(feedbackHealthTrend)
	out.FeedbackIntegrityRecoveryStatus, out.FeedbackIntegrityRecovered, out.FeedbackIntegrityPreviousHealth = resolveFeedbackIntegrityRecovery(
		feedbackHealthTrend,
	)
	out.FeedbackIntegrityEscalation = buildFeedbackIntegrityEscalation(
		out.FeedbackIntegrityHealth,
		out.FeedbackIntegrityStreaks,
		out.FeedbackIntegrityDelta,
		out.FeedbackIntegrityRecoveryStatus,
	)
	out.FeedbackIntegrityEscalationTrend = buildFeedbackIntegrityEscalationTrend(feedbackHealthTrend)
	out.FeedbackIntegrityEscalationStats = buildFeedbackIntegrityEscalationStats(out.FeedbackIntegrityEscalationTrend)
	out.FeedbackIntegrityEscalationIncidents = buildFeedbackIntegrityEscalationIncidents(out.FeedbackIntegrityEscalationTrend, 7)
	feedbackAnomalyJobs, feedbackTopPickConflictJobs, err := h.loadVideoImageFeedbackIntegrityRiskJobCounts(sinceWindow, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackIntegrityAnomalyJobs = feedbackAnomalyJobs
	out.FeedbackIntegrityTopPickJobs = feedbackTopPickConflictJobs

	feedbackSceneStats, err := h.loadVideoJobFeedbackSceneStats(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackSceneStats = feedbackSceneStats

	feedbackActionStats, err := h.loadVideoImageFeedbackActionStats(sinceWindow, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackActionStats = feedbackActionStats

	feedbackTopSceneTags, err := h.loadVideoImageFeedbackTopSceneStats(sinceWindow, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackTopSceneTags = feedbackTopSceneTags

	feedbackTrend, err := h.loadVideoImageFeedbackTrend(sinceWindow, now, windowDuration, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackTrend = feedbackTrend

	feedbackNegativeGuardOverview, err := h.loadVideoJobFeedbackNegativeGuardOverview(sinceWindow, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackNegativeGuardOverview = feedbackNegativeGuardOverview
	feedbackNegativeGuardReasons, err := h.loadVideoJobFeedbackNegativeGuardReasonStats(sinceWindow, nil, 12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackNegativeGuardReasons = feedbackNegativeGuardReasons

	feedbackEnabled, currentRollout, liveGuardConfig, err := h.loadCurrentFeedbackRolloutConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.LiveCoverSceneMinSamples = liveGuardConfig.MinSamples
	out.LiveCoverSceneGuardMinTotal = liveGuardConfig.MinTotal
	out.LiveCoverSceneGuardScoreFloor = liveGuardConfig.ScoreFloor

	liveCoverSceneStats, err := h.loadVideoJobLiveCoverSceneStats(sinceWindow, liveGuardConfig.MinSamples)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.LiveCoverSceneStats = liveCoverSceneStats

	gifLoopOverview, err := h.loadVideoJobGIFLoopTuneOverview(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFLoopTuneOverview = gifLoopOverview
	gifEvaluationOverview, err := h.loadVideoJobGIFEvaluationOverview(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFEvaluationOverview = gifEvaluationOverview
	gifBaselineSnapshots, err := h.loadLatestVideoJobGIFBaselines(7)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFBaselineSnapshots = gifBaselineSnapshots
	gifTopSamples, err := h.loadVideoJobGIFEvaluationSamples(sinceWindow, 5, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFEvaluationTopSamples = gifTopSamples
	gifLowSamples, err := h.loadVideoJobGIFEvaluationSamples(sinceWindow, 5, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFEvaluationLowSamples = gifLowSamples
	gifManualScoreOverview, err := h.loadVideoJobGIFManualScoreOverview(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFManualScoreOverview = gifManualScoreOverview
	gifManualScoreDiffSamples, err := h.loadVideoJobGIFManualScoreDiffSamples(sinceWindow, 8)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.GIFManualScoreDiffSamples = gifManualScoreDiffSamples

	feedbackGroupStats, err := h.loadVideoJobFeedbackGroupStats(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackGroupStats = feedbackGroupStats

	feedbackGroupFormatStats, err := h.loadVideoJobFeedbackGroupFormatStats(sinceWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackGroupFormatStats = feedbackGroupFormatStats

	feedbackHistory, err := h.loadVideoJobFeedbackGroupStatsHistory(windowDuration, 3, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	recommendation := buildFeedbackRolloutRecommendationWithHistory(
		feedbackEnabled,
		currentRollout,
		feedbackHistory,
		3,
		liveCoverSceneStats,
		liveGuardConfig,
	)
	out.FeedbackRolloutRecommendation = &recommendation
	rolloutAudits, err := h.loadVideoJobFeedbackRolloutAudits(12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackRolloutAuditLogs = rolloutAudits

	type durationRow struct {
		P50 *float64 `gorm:"column:p50"`
		P95 *float64 `gorm:"column:p95"`
	}
	var duration durationRow
	if err := h.db.Raw(`
SELECT
	percentile_cont(0.50) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at))) AS p50,
	percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at))) AS p95
FROM archive.video_jobs
WHERE status = ?
	AND started_at IS NOT NULL
	AND finished_at IS NOT NULL
	AND finished_at >= ?
`, models.VideoJobStatusDone, sinceWindow).Scan(&duration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if duration.P50 != nil {
		out.DurationP50Sec = *duration.P50
	}
	if duration.P95 != nil {
		out.DurationP95Sec = *duration.P95
	}

	var sampleDuration durationRow
	if err := h.db.Raw(`
SELECT
	percentile_cont(0.50) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (v.finished_at - v.started_at))) AS p50,
	percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (v.finished_at - v.started_at))) AS p95
FROM archive.video_jobs v
JOIN archive.collections c ON c.id = v.result_collection_id
WHERE c.is_sample = TRUE
	AND v.status = ?
	AND v.started_at IS NOT NULL
	AND v.finished_at IS NOT NULL
	AND v.finished_at >= ?
`, models.VideoJobStatusDone, sinceWindow).Scan(&sampleDuration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sampleDuration.P50 != nil {
		out.SampleDurationP50Sec = *sampleDuration.P50
	}
	if sampleDuration.P95 != nil {
		out.SampleDurationP95Sec = *sampleDuration.P95
	}
	_ = h.db.Model(&models.VideoJobCost{}).
		Select("COALESCE(sum(estimated_cost), 0)").
		Scan(&out.CostTotal).Error
	_ = h.db.Model(&models.VideoJobCost{}).
		Select("COALESCE(sum(estimated_cost), 0)").
		Where("created_at >= ?", since24h).
		Scan(&out.Cost24h).Error
	_ = h.db.Model(&models.VideoJobCost{}).
		Select("COALESCE(avg(estimated_cost), 0)").
		Where("created_at >= ?", since24h).
		Scan(&out.CostAvg24h).Error
	_ = h.db.Model(&models.VideoJobCost{}).
		Select("COALESCE(sum(estimated_cost), 0)").
		Where("created_at >= ?", sinceWindow).
		Scan(&out.CostWindow).Error
	_ = h.db.Model(&models.VideoJobCost{}).
		Select("COALESCE(avg(estimated_cost), 0)").
		Where("created_at >= ?", sinceWindow).
		Scan(&out.CostAvgWindow).Error

	c.JSON(http.StatusOK, out)
}
