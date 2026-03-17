package handlers

import (
	"testing"
	"time"
)

func hasFeedbackIntegrityAlertCode(alerts []AdminVideoJobFeedbackIntegrityAlert, code string) bool {
	for _, item := range alerts {
		if item.Code == code {
			return true
		}
	}
	return false
}

func findFeedbackIntegrityRecommendation(
	recs []AdminVideoJobFeedbackIntegrityRecommendation,
	category string,
) *AdminVideoJobFeedbackIntegrityRecommendation {
	for idx := range recs {
		if recs[idx].Category == category {
			return &recs[idx]
		}
	}
	return nil
}

func TestBuildVideoJobFeedbackIntegrityAlerts_NoSamples(t *testing.T) {
	alerts, health := buildVideoJobFeedbackIntegrityAlerts(
		AdminVideoJobFeedbackIntegrityOverview{},
		defaultFeedbackIntegrityAlertThresholdSettings(),
	)
	if health != "green" {
		t.Fatalf("expected green health, got %s", health)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected one alert, got %d", len(alerts))
	}
	if alerts[0].Level != "info" || alerts[0].Code != "feedback_integrity_no_data" {
		t.Fatalf("unexpected no-data alert: %#v", alerts[0])
	}
}

func TestBuildVideoJobFeedbackIntegrityAlerts_Warn(t *testing.T) {
	overview := AdminVideoJobFeedbackIntegrityOverview{
		Samples:                  100,
		WithOutputID:             97,
		ResolvedOutput:           95,
		OutputCoverageRate:       0.97,
		OutputResolvedRate:       0.98,
		OutputJobConsistencyRate: 0.998,
		TopPickMultiHitUsers:     1,
	}
	alerts, health := buildVideoJobFeedbackIntegrityAlerts(overview, defaultFeedbackIntegrityAlertThresholdSettings())
	if health != "yellow" {
		t.Fatalf("expected yellow health, got %s", health)
	}
	if !hasFeedbackIntegrityAlertCode(alerts, "feedback_integrity_output_coverage_warn") {
		t.Fatalf("expected output coverage warn alert, got %#v", alerts)
	}
	if !hasFeedbackIntegrityAlertCode(alerts, "feedback_integrity_output_resolved_warn") {
		t.Fatalf("expected output resolved warn alert, got %#v", alerts)
	}
	if !hasFeedbackIntegrityAlertCode(alerts, "feedback_integrity_job_consistency_warn") {
		t.Fatalf("expected job consistency warn alert, got %#v", alerts)
	}
	if !hasFeedbackIntegrityAlertCode(alerts, "feedback_integrity_top_pick_conflict_warn") {
		t.Fatalf("expected top pick conflict warn alert, got %#v", alerts)
	}
}

func TestBuildVideoJobFeedbackIntegrityAlerts_Critical(t *testing.T) {
	overview := AdminVideoJobFeedbackIntegrityOverview{
		Samples:                  80,
		OutputCoverageRate:       0.90,
		OutputResolvedRate:       0.95,
		OutputJobConsistencyRate: 0.990,
		TopPickMultiHitUsers:     4,
	}
	alerts, health := buildVideoJobFeedbackIntegrityAlerts(overview, defaultFeedbackIntegrityAlertThresholdSettings())
	if health != "red" {
		t.Fatalf("expected red health, got %s", health)
	}
	if !hasFeedbackIntegrityAlertCode(alerts, "feedback_integrity_output_coverage_critical") {
		t.Fatalf("expected output coverage critical alert, got %#v", alerts)
	}
	if !hasFeedbackIntegrityAlertCode(alerts, "feedback_integrity_output_resolved_critical") {
		t.Fatalf("expected output resolved critical alert, got %#v", alerts)
	}
	if !hasFeedbackIntegrityAlertCode(alerts, "feedback_integrity_job_consistency_critical") {
		t.Fatalf("expected job consistency critical alert, got %#v", alerts)
	}
	if !hasFeedbackIntegrityAlertCode(alerts, "feedback_integrity_top_pick_conflict_critical") {
		t.Fatalf("expected top pick conflict critical alert, got %#v", alerts)
	}
}

func TestResolveFeedbackIntegrityRecovery_Recovered(t *testing.T) {
	trend := []AdminVideoJobFeedbackIntegrityHealthTrendPoint{
		{Bucket: "2026-03-13", Health: "yellow"},
		{Bucket: "2026-03-14", Health: "green"},
	}
	status, recovered, previous := resolveFeedbackIntegrityRecovery(trend)
	if status != "recovered" {
		t.Fatalf("expected recovered status, got %s", status)
	}
	if !recovered {
		t.Fatalf("expected recovered=true")
	}
	if previous != "yellow" {
		t.Fatalf("expected previous health yellow, got %s", previous)
	}
}

func TestResolveFeedbackIntegrityRecovery_Regressed(t *testing.T) {
	trend := []AdminVideoJobFeedbackIntegrityHealthTrendPoint{
		{Bucket: "2026-03-13", Health: "green"},
		{Bucket: "2026-03-14", Health: "red"},
	}
	status, recovered, previous := resolveFeedbackIntegrityRecovery(trend)
	if status != "regressed" {
		t.Fatalf("expected regressed status, got %s", status)
	}
	if recovered {
		t.Fatalf("expected recovered=false")
	}
	if previous != "green" {
		t.Fatalf("expected previous health green, got %s", previous)
	}
}

func TestResolveFeedbackIntegrityRecovery_NoData(t *testing.T) {
	trend := []AdminVideoJobFeedbackIntegrityHealthTrendPoint{
		{Bucket: "2026-03-13", Health: "no_data"},
		{Bucket: "2026-03-14", Health: "no_data"},
	}
	status, recovered, previous := resolveFeedbackIntegrityRecovery(trend)
	if status != "no_data" {
		t.Fatalf("expected no_data status, got %s", status)
	}
	if recovered {
		t.Fatalf("expected recovered=false")
	}
	if previous != "" {
		t.Fatalf("expected empty previous health, got %s", previous)
	}
}

func TestBuildFeedbackIntegrityRecommendations_WithAlerts(t *testing.T) {
	alerts := []AdminVideoJobFeedbackIntegrityAlert{
		{Level: "critical", Code: "feedback_integrity_output_coverage_critical", Message: "coverage low"},
		{Level: "warn", Code: "feedback_integrity_top_pick_conflict_warn", Message: "top pick conflict"},
	}
	recs := buildFeedbackIntegrityRecommendations(alerts, "red")
	if len(recs) < 2 {
		t.Fatalf("expected >=2 recommendations, got %d", len(recs))
	}
	mapping := findFeedbackIntegrityRecommendation(recs, "mapping_chain")
	if mapping == nil {
		t.Fatalf("missing mapping_chain recommendation: %#v", recs)
	}
	if mapping.SuggestedQuick != "feedback_anomaly" {
		t.Fatalf("unexpected mapping quick action: %s", mapping.SuggestedQuick)
	}
	if mapping.Severity != "critical" {
		t.Fatalf("unexpected mapping severity: %s", mapping.Severity)
	}
	topPick := findFeedbackIntegrityRecommendation(recs, "top_pick_conflict")
	if topPick == nil {
		t.Fatalf("missing top_pick_conflict recommendation: %#v", recs)
	}
	if topPick.SuggestedQuick != "top_pick_conflict" {
		t.Fatalf("unexpected top_pick quick action: %s", topPick.SuggestedQuick)
	}
}

func TestBuildFeedbackIntegrityRecommendations_HealthyFallback(t *testing.T) {
	recs := buildFeedbackIntegrityRecommendations(nil, "green")
	if len(recs) != 1 {
		t.Fatalf("expected exactly one recommendation, got %d", len(recs))
	}
	if recs[0].Category != "healthy" {
		t.Fatalf("expected healthy recommendation, got %#v", recs[0])
	}
}

func TestBuildFeedbackLearningChainStatus_Strict(t *testing.T) {
	status := buildFeedbackLearningChainStatus(false)
	if status.LearningMode != "strict_output_candidate" {
		t.Fatalf("unexpected strict learning mode: %s", status.LearningMode)
	}
	if status.LegacyFeedbackFallbackEnabled {
		t.Fatalf("expected legacy fallback disabled")
	}
}

func TestBuildFeedbackLearningChainStatus_LegacyFallback(t *testing.T) {
	status := buildFeedbackLearningChainStatus(true)
	if status.LearningMode != "legacy_fallback_enabled" {
		t.Fatalf("unexpected legacy learning mode: %s", status.LearningMode)
	}
	if !status.LegacyFeedbackFallbackEnabled {
		t.Fatalf("expected legacy fallback enabled")
	}
}

func TestBuildFeedbackIntegrityRecommendations_NoData(t *testing.T) {
	alerts := []AdminVideoJobFeedbackIntegrityAlert{
		{Level: "info", Code: "feedback_integrity_no_data", Message: "no samples"},
	}
	recs := buildFeedbackIntegrityRecommendations(alerts, "green")
	noData := findFeedbackIntegrityRecommendation(recs, "no_data")
	if noData == nil {
		t.Fatalf("expected no_data recommendation, got %#v", recs)
	}
	if noData.SuggestedAction != "collect_more_samples" {
		t.Fatalf("unexpected no_data suggested action: %s", noData.SuggestedAction)
	}
}

func TestBuildFeedbackIntegrityAlertCodeStats(t *testing.T) {
	trend := []AdminVideoJobFeedbackIntegrityHealthTrendPoint{
		{
			Bucket:     "2026-03-13",
			Health:     "yellow",
			AlertCodes: []string{"feedback_integrity_output_coverage_warn", "feedback_integrity_output_coverage_warn"},
		},
		{
			Bucket:     "2026-03-14",
			Health:     "red",
			AlertCodes: []string{"feedback_integrity_output_coverage_warn", "feedback_integrity_top_pick_conflict_critical"},
		},
	}
	stats := buildFeedbackIntegrityAlertCodeStats(trend)
	if len(stats) != 2 {
		t.Fatalf("expected 2 alert code stats, got %d (%#v)", len(stats), stats)
	}
	if stats[0].Code != "feedback_integrity_output_coverage_warn" {
		t.Fatalf("expected coverage warn to rank first, got %#v", stats[0])
	}
	if stats[0].DaysHit != 2 {
		t.Fatalf("expected coverage warn days_hit=2, got %d", stats[0].DaysHit)
	}
	if stats[0].LatestBucket != "2026-03-14" {
		t.Fatalf("expected latest_bucket=2026-03-14, got %s", stats[0].LatestBucket)
	}
}

func TestBuildFeedbackIntegrityDelta(t *testing.T) {
	current := AdminVideoJobFeedbackIntegrityOverview{
		Samples:                  120,
		OutputCoverageRate:       0.98,
		OutputResolvedRate:       0.99,
		OutputJobConsistencyRate: 0.997,
		TopPickMultiHitUsers:     1,
	}
	previous := AdminVideoJobFeedbackIntegrityOverview{
		Samples:                  100,
		OutputCoverageRate:       0.96,
		OutputResolvedRate:       0.98,
		OutputJobConsistencyRate: 0.995,
		TopPickMultiHitUsers:     3,
	}
	start := time.Date(2026, 3, 13, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)
	delta := buildFeedbackIntegrityDelta(
		current,
		"yellow",
		4,
		previous,
		"red",
		6,
		start,
		end,
	)
	if delta == nil {
		t.Fatalf("expected non-nil delta")
	}
	if delta.SamplesDelta != 20 {
		t.Fatalf("expected samples_delta=20, got %d", delta.SamplesDelta)
	}
	if delta.AlertCountDelta != -2 {
		t.Fatalf("expected alert_count_delta=-2, got %d", delta.AlertCountDelta)
	}
	if delta.TopPickMultiHitUsersDelta != -2 {
		t.Fatalf("expected top_pick delta=-2, got %d", delta.TopPickMultiHitUsersDelta)
	}
	if !delta.HasPreviousData {
		t.Fatalf("expected has_previous_data=true")
	}
}

func TestBuildFeedbackIntegrityStreaks(t *testing.T) {
	trend := []AdminVideoJobFeedbackIntegrityHealthTrendPoint{
		{Bucket: "2026-03-10", Health: "green"},
		{Bucket: "2026-03-11", Health: "yellow"},
		{Bucket: "2026-03-12", Health: "red"},
		{Bucket: "2026-03-13", Health: "red"},
	}
	streaks := buildFeedbackIntegrityStreaks(trend)
	if streaks.ConsecutiveRedDays != 2 {
		t.Fatalf("expected consecutive red days=2, got %d", streaks.ConsecutiveRedDays)
	}
	if streaks.ConsecutiveNonGreenDays != 3 {
		t.Fatalf("expected consecutive non-green days=3, got %d", streaks.ConsecutiveNonGreenDays)
	}
	if streaks.ConsecutiveGreenDays != 0 {
		t.Fatalf("expected consecutive green days=0, got %d", streaks.ConsecutiveGreenDays)
	}
	if streaks.LastNonGreenBucket != "2026-03-13" {
		t.Fatalf("expected last non-green bucket=2026-03-13, got %s", streaks.LastNonGreenBucket)
	}
	if streaks.LastRedBucket != "2026-03-13" {
		t.Fatalf("expected last red bucket=2026-03-13, got %s", streaks.LastRedBucket)
	}
}

func TestBuildFeedbackIntegrityEscalation_Oncall(t *testing.T) {
	streaks := AdminVideoJobFeedbackIntegrityStreaks{
		ConsecutiveRedDays:      2,
		Recent7dRedDays:         3,
		ConsecutiveNonGreenDays: 3,
	}
	delta := &AdminVideoJobFeedbackIntegrityDelta{
		AlertCountDelta:           3,
		TopPickMultiHitUsersDelta: 1,
	}
	escalation := buildFeedbackIntegrityEscalation("red", streaks, delta, "stable")
	if escalation.Level != "oncall" {
		t.Fatalf("expected escalation level oncall, got %s", escalation.Level)
	}
	if !escalation.Required {
		t.Fatalf("expected escalation required")
	}
	if len(escalation.TriggeredRules) == 0 {
		t.Fatalf("expected triggered rules")
	}
}

func TestBuildFeedbackIntegrityEscalation_RecoveredDowngrade(t *testing.T) {
	streaks := AdminVideoJobFeedbackIntegrityStreaks{
		ConsecutiveRedDays:      2,
		ConsecutiveNonGreenDays: 4,
	}
	escalation := buildFeedbackIntegrityEscalation("red", streaks, nil, "recovered")
	if escalation.Level != "notice" {
		t.Fatalf("expected recovered downgrade to notice, got %s", escalation.Level)
	}
	if escalation.Required {
		t.Fatalf("expected escalation not required after downgrade")
	}
}

func TestBuildFeedbackIntegrityEscalationTrend(t *testing.T) {
	trend := []AdminVideoJobFeedbackIntegrityHealthTrendPoint{
		{Bucket: "2026-03-10", Health: "green", AlertCount: 0, TopPickMultiHitUsers: 0},
		{Bucket: "2026-03-11", Health: "yellow", AlertCount: 1, TopPickMultiHitUsers: 0},
		{Bucket: "2026-03-12", Health: "red", AlertCount: 3, TopPickMultiHitUsers: 1},
		{Bucket: "2026-03-13", Health: "red", AlertCount: 4, TopPickMultiHitUsers: 2},
		{Bucket: "2026-03-14", Health: "green", AlertCount: 0, TopPickMultiHitUsers: 0},
	}
	points := buildFeedbackIntegrityEscalationTrend(trend)
	if len(points) != len(trend) {
		t.Fatalf("expected %d trend points, got %d", len(trend), len(points))
	}
	if points[2].EscalationLevel != "watch" || !points[2].EscalationRequired {
		t.Fatalf("expected day3 escalation watch/required, got level=%s required=%v", points[2].EscalationLevel, points[2].EscalationRequired)
	}
	if points[3].EscalationLevel != "oncall" || !points[3].EscalationRequired {
		t.Fatalf("expected day4 escalation oncall/required, got level=%s required=%v", points[3].EscalationLevel, points[3].EscalationRequired)
	}
	if points[4].RecoveryStatus != "recovered" {
		t.Fatalf("expected day5 recovery status recovered, got %s", points[4].RecoveryStatus)
	}
	if points[4].EscalationRequired {
		t.Fatalf("expected day5 escalation not required")
	}
}

func TestBuildFeedbackIntegrityEscalationStats(t *testing.T) {
	points := []AdminVideoJobFeedbackIntegrityEscalationTrendPoint{
		{Bucket: "2026-03-10", EscalationLevel: "none", EscalationRequired: false},
		{Bucket: "2026-03-11", EscalationLevel: "watch", EscalationRequired: true},
		{Bucket: "2026-03-12", EscalationLevel: "oncall", EscalationRequired: true},
		{Bucket: "2026-03-13", EscalationLevel: "notice", EscalationRequired: false, EscalationReason: "recovered"},
	}
	stats := buildFeedbackIntegrityEscalationStats(points)
	if stats.TotalDays != 4 {
		t.Fatalf("expected total_days=4, got %d", stats.TotalDays)
	}
	if stats.RequiredDays != 2 {
		t.Fatalf("expected required_days=2, got %d", stats.RequiredDays)
	}
	if stats.OncallDays != 1 || stats.WatchDays != 1 || stats.NoticeDays != 1 || stats.NoneDays != 1 {
		t.Fatalf("unexpected level days: oncall=%d watch=%d notice=%d none=%d", stats.OncallDays, stats.WatchDays, stats.NoticeDays, stats.NoneDays)
	}
	if stats.LatestBucket != "2026-03-13" || stats.LatestLevel != "notice" || stats.LatestRequired {
		t.Fatalf("unexpected latest summary: bucket=%s level=%s required=%v", stats.LatestBucket, stats.LatestLevel, stats.LatestRequired)
	}
	if stats.LatestReason != "recovered" {
		t.Fatalf("expected latest reason recovered, got %s", stats.LatestReason)
	}
}

func TestBuildFeedbackIntegrityEscalationIncidents(t *testing.T) {
	points := []AdminVideoJobFeedbackIntegrityEscalationTrendPoint{
		{Bucket: "2026-03-10", EscalationLevel: "none", EscalationRequired: false},
		{Bucket: "2026-03-11", EscalationLevel: "watch", EscalationRequired: true, EscalationReason: "alert growth", AlertCount: 3},
		{Bucket: "2026-03-12", EscalationLevel: "oncall", EscalationRequired: true, EscalationReason: "consecutive red", AlertCount: 5},
	}
	incidents := buildFeedbackIntegrityEscalationIncidents(points, 2)
	if len(incidents) != 2 {
		t.Fatalf("expected 2 incidents, got %d", len(incidents))
	}
	if incidents[0].Bucket != "2026-03-12" || incidents[0].EscalationLevel != "oncall" {
		t.Fatalf("expected latest incident 2026-03-12/oncall, got %s/%s", incidents[0].Bucket, incidents[0].EscalationLevel)
	}
	if incidents[1].Bucket != "2026-03-11" || incidents[1].EscalationLevel != "watch" {
		t.Fatalf("expected second incident 2026-03-11/watch, got %s/%s", incidents[1].Bucket, incidents[1].EscalationLevel)
	}
}
