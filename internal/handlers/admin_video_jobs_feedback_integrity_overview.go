package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) GetAdminVideoJobsFeedbackIntegrityOverview(c *gin.Context) {
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
	sinceWindow := now.Add(-windowDuration)
	out := AdminVideoJobFeedbackIntegrityOverviewResponse{
		Window:                           windowLabel,
		WindowStart:                      sinceWindow.Format(time.RFC3339),
		WindowEnd:                        now.Format(time.RFC3339),
		FeedbackLearningChain:            buildFeedbackLearningChainStatus(h.cfg.EnableLegacyFeedbackFallback),
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
	}
	if filter != nil {
		out.FilterUserID = filter.UserID
		out.FilterFormat = filter.Format
		out.FilterGuardReason = filter.GuardReason
	}
	if count, countErr := h.loadLegacyEvalBackfillCandidateCount(); countErr == nil {
		out.FeedbackLearningChain.LegacyEvalBackfillCandidates = count
	}
	if setting, loadErr := h.loadVideoQualitySetting(); loadErr == nil {
		out.FeedbackIntegrityAlertThresholds = feedbackIntegrityAlertThresholdSettingsFromModel(setting)
	}

	feedbackIntegrity, err := h.loadVideoImageFeedbackIntegrityOverview(sinceWindow, filter)
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
	previousFeedbackIntegrity, err := h.loadVideoImageFeedbackIntegrityOverviewRange(previousWindowStart, previousWindowEnd, filter)
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
		filter,
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
	feedbackAnomalyJobs, feedbackTopPickConflictJobs, err := h.loadVideoImageFeedbackIntegrityRiskJobCounts(sinceWindow, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackIntegrityAnomalyJobs = feedbackAnomalyJobs
	out.FeedbackIntegrityTopPickJobs = feedbackTopPickConflictJobs

	feedbackActionStats, err := h.loadVideoImageFeedbackActionStats(sinceWindow, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackActionStats = feedbackActionStats

	feedbackTopSceneTags, err := h.loadVideoImageFeedbackTopSceneStats(sinceWindow, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackTopSceneTags = feedbackTopSceneTags

	feedbackTrend, err := h.loadVideoImageFeedbackTrend(sinceWindow, now, windowDuration, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackTrend = feedbackTrend

	feedbackNegativeGuardOverview, err := h.loadVideoJobFeedbackNegativeGuardOverview(sinceWindow, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackNegativeGuardOverview = feedbackNegativeGuardOverview
	feedbackNegativeGuardReasons, err := h.loadVideoJobFeedbackNegativeGuardReasonStats(sinceWindow, filter, 12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out.FeedbackNegativeGuardReasons = feedbackNegativeGuardReasons

	c.JSON(http.StatusOK, out)
}

// ExportAdminSampleVideoJobsBaselineCSV godoc
// @Summary Export sample video-jobs baseline CSV (admin)
// @Tags admin
// @Produce text/csv
// @Param window query string false "window: 24h | 7d | 30d"
// @Success 200 {string} string
// @Router /api/admin/video-jobs/samples/baseline.csv [get]
