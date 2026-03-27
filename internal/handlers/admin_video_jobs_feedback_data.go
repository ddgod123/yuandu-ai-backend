package handlers

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"emoji/internal/models"
)

func countEffectiveFeedbackIntegrityAlerts(alerts []AdminVideoJobFeedbackIntegrityAlert) int {
	if len(alerts) == 0 {
		return 0
	}
	count := 0
	for _, item := range alerts {
		code := strings.TrimSpace(item.Code)
		if code == "" || code == "feedback_integrity_no_data" {
			continue
		}
		count++
	}
	return count
}

func buildFeedbackIntegrityDelta(
	current AdminVideoJobFeedbackIntegrityOverview,
	currentHealth string,
	currentAlertCount int,
	previous AdminVideoJobFeedbackIntegrityOverview,
	previousHealth string,
	previousAlertCount int,
	previousWindowStart time.Time,
	previousWindowEnd time.Time,
) *AdminVideoJobFeedbackIntegrityDelta {
	delta := &AdminVideoJobFeedbackIntegrityDelta{
		HasPreviousData:               previous.Samples > 0,
		PreviousWindowStart:           previousWindowStart.Format(time.RFC3339),
		PreviousWindowEnd:             previousWindowEnd.Format(time.RFC3339),
		PreviousHealth:                strings.ToLower(strings.TrimSpace(previousHealth)),
		CurrentHealth:                 strings.ToLower(strings.TrimSpace(currentHealth)),
		PreviousSamples:               previous.Samples,
		CurrentSamples:                current.Samples,
		SamplesDelta:                  current.Samples - previous.Samples,
		PreviousAlertCount:            previousAlertCount,
		CurrentAlertCount:             currentAlertCount,
		AlertCountDelta:               currentAlertCount - previousAlertCount,
		OutputCoverageRateDelta:       current.OutputCoverageRate - previous.OutputCoverageRate,
		OutputResolvedRateDelta:       current.OutputResolvedRate - previous.OutputResolvedRate,
		OutputJobConsistencyRateDelta: current.OutputJobConsistencyRate - previous.OutputJobConsistencyRate,
		TopPickMultiHitUsersDelta:     current.TopPickMultiHitUsers - previous.TopPickMultiHitUsers,
	}
	return delta
}

func (h *Handler) loadVideoImageFeedbackIntegrityOverview(
	since time.Time,
	filter *videoImageFeedbackFilter,
) (AdminVideoJobFeedbackIntegrityOverview, error) {
	return h.loadVideoImageFeedbackIntegrityOverviewRange(since, time.Time{}, filter)
}

func (h *Handler) loadLegacyEvalBackfillCandidateCount() (int64, error) {
	if h == nil || h.db == nil {
		return 0, nil
	}
	var count int64
	if err := h.db.Model(&models.VideoJobGIFCandidate{}).
		Where("COALESCE(feature_json->>'version', '') = ?", "legacy_eval_backfill_v1").
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (h *Handler) loadVideoImageFeedbackIntegrityOverviewRange(
	since time.Time,
	until time.Time,
	filter *videoImageFeedbackFilter,
) (AdminVideoJobFeedbackIntegrityOverview, error) {
	type row struct {
		Samples              int64 `gorm:"column:samples"`
		WithOutputID         int64 `gorm:"column:with_output_id"`
		MissingOutputID      int64 `gorm:"column:missing_output_id"`
		ResolvedOutput       int64 `gorm:"column:resolved_output"`
		OrphanOutput         int64 `gorm:"column:orphan_output"`
		JobMismatch          int64 `gorm:"column:job_mismatch"`
		TopPickMultiHitUsers int64 `gorm:"column:top_pick_multi_hit_users"`
	}

	tables := resolveVideoImageReadTablesByFilter(filter)
	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQLWithTables(filter, "f", tables.Jobs)
	untilSQL := ""
	args := make([]interface{}, 0, 2+len(filterArgs))
	args = append(args, since)
	if !until.IsZero() {
		untilSQL = " AND f.created_at < ?"
		args = append(args, until)
	}
	args = append(args, filterArgs...)
	query := fmt.Sprintf(`
WITH base AS (
	SELECT
		f.id,
		f.job_id,
		f.user_id,
		f.output_id,
		LOWER(COALESCE(f.action, '')) AS action
	FROM %s f
	WHERE f.created_at >= ?
`+untilSQL+`
`+filterSQL+`
),
joined AS (
	SELECT
		b.*,
		o.id AS output_exists_id,
		o.job_id AS output_job_id
	FROM base b
	LEFT JOIN %s o ON o.id = b.output_id
),
top_pick_conflict AS (
	SELECT COUNT(*)::bigint AS users
	FROM (
		SELECT job_id, user_id
		FROM base
		WHERE action = 'top_pick'
		GROUP BY job_id, user_id
		HAVING COUNT(*) > 1
	) t
)
SELECT
	COUNT(*)::bigint AS samples,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL)::bigint AS with_output_id,
	COUNT(*) FILTER (WHERE output_id IS NULL)::bigint AS missing_output_id,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::bigint AS resolved_output,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NULL)::bigint AS orphan_output,
	COUNT(*) FILTER (
		WHERE output_id IS NOT NULL
			AND output_exists_id IS NOT NULL
			AND output_job_id <> job_id
	)::bigint AS job_mismatch,
	COALESCE((SELECT users FROM top_pick_conflict), 0)::bigint AS top_pick_multi_hit_users
FROM joined
`, tables.Feedback, tables.Outputs)

	var dbRow row
	if err := h.db.Raw(query, args...).Scan(&dbRow).Error; err != nil {
		return AdminVideoJobFeedbackIntegrityOverview{}, err
	}

	out := AdminVideoJobFeedbackIntegrityOverview{
		Samples:              dbRow.Samples,
		WithOutputID:         dbRow.WithOutputID,
		MissingOutputID:      dbRow.MissingOutputID,
		ResolvedOutput:       dbRow.ResolvedOutput,
		OrphanOutput:         dbRow.OrphanOutput,
		JobMismatch:          dbRow.JobMismatch,
		TopPickMultiHitUsers: dbRow.TopPickMultiHitUsers,
	}
	if out.Samples > 0 {
		out.OutputCoverageRate = float64(out.WithOutputID) / float64(out.Samples)
	}
	if out.WithOutputID > 0 {
		out.OutputResolvedRate = float64(out.ResolvedOutput) / float64(out.WithOutputID)
		consistent := out.ResolvedOutput - out.JobMismatch
		if consistent < 0 {
			consistent = 0
		}
		out.OutputJobConsistencyRate = float64(consistent) / float64(out.WithOutputID)
	}
	return out, nil
}

func (h *Handler) loadVideoImageFeedbackIntegrityHealthTrend(
	since time.Time,
	until time.Time,
	thresholds FeedbackIntegrityAlertThresholdSettings,
	filter *videoImageFeedbackFilter,
) ([]AdminVideoJobFeedbackIntegrityHealthTrendPoint, error) {
	type row struct {
		BucketDay            time.Time `gorm:"column:bucket_day"`
		Samples              int64     `gorm:"column:samples"`
		WithOutputID         int64     `gorm:"column:with_output_id"`
		ResolvedOutput       int64     `gorm:"column:resolved_output"`
		JobMismatch          int64     `gorm:"column:job_mismatch"`
		TopPickMultiHitUsers int64     `gorm:"column:top_pick_multi_hit_users"`
	}

	tables := resolveVideoImageReadTablesByFilter(filter)
	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQLWithTables(filter, "f", tables.Jobs)
	query := fmt.Sprintf(`
WITH base AS (
	SELECT
		date_trunc('day', f.created_at) AS bucket_day,
		f.job_id,
		f.user_id,
		f.output_id,
		LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action
	FROM %s f
	WHERE f.created_at >= ?
		AND f.created_at <= ?
`+filterSQL+`
),
joined AS (
	SELECT
		b.*,
		o.id AS output_exists_id,
		o.job_id AS output_job_id
	FROM base b
	LEFT JOIN %s o ON o.id = b.output_id
),
daily_agg AS (
	SELECT
		bucket_day,
		COUNT(*)::bigint AS samples,
		COUNT(*) FILTER (WHERE output_id IS NOT NULL)::bigint AS with_output_id,
		COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::bigint AS resolved_output,
		COUNT(*) FILTER (
			WHERE output_id IS NOT NULL
				AND output_exists_id IS NOT NULL
				AND output_job_id <> job_id
		)::bigint AS job_mismatch
	FROM joined
	GROUP BY bucket_day
),
daily_top_pick_conflict AS (
	SELECT
		bucket_day,
		COUNT(*)::bigint AS users
	FROM (
		SELECT
			bucket_day,
			job_id,
			user_id
		FROM base
		WHERE action = 'top_pick'
		GROUP BY bucket_day, job_id, user_id
		HAVING COUNT(*) > 1
	) t
	GROUP BY bucket_day
),
days AS (
	SELECT generate_series(
		date_trunc('day', ?::timestamp),
		date_trunc('day', ?::timestamp),
		interval '1 day'
	) AS bucket_day
)
SELECT
	days.bucket_day,
	COALESCE(daily_agg.samples, 0)::bigint AS samples,
	COALESCE(daily_agg.with_output_id, 0)::bigint AS with_output_id,
	COALESCE(daily_agg.resolved_output, 0)::bigint AS resolved_output,
	COALESCE(daily_agg.job_mismatch, 0)::bigint AS job_mismatch,
	COALESCE(daily_top_pick_conflict.users, 0)::bigint AS top_pick_multi_hit_users
FROM days
LEFT JOIN daily_agg ON daily_agg.bucket_day = days.bucket_day
LEFT JOIN daily_top_pick_conflict ON daily_top_pick_conflict.bucket_day = days.bucket_day
ORDER BY days.bucket_day ASC
`, tables.Feedback, tables.Outputs)
	args := []interface{}{since, until}
	args = append(args, filterArgs...)
	args = append(args, since, until)

	var rows []row
	if err := h.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobFeedbackIntegrityHealthTrendPoint, 0, len(rows))
	for _, item := range rows {
		overview := AdminVideoJobFeedbackIntegrityOverview{
			Samples:              item.Samples,
			WithOutputID:         item.WithOutputID,
			ResolvedOutput:       item.ResolvedOutput,
			JobMismatch:          item.JobMismatch,
			TopPickMultiHitUsers: item.TopPickMultiHitUsers,
		}
		if overview.Samples > 0 {
			overview.OutputCoverageRate = float64(overview.WithOutputID) / float64(overview.Samples)
		}
		if overview.WithOutputID > 0 {
			overview.OutputResolvedRate = float64(overview.ResolvedOutput) / float64(overview.WithOutputID)
			consistent := overview.ResolvedOutput - overview.JobMismatch
			if consistent < 0 {
				consistent = 0
			}
			overview.OutputJobConsistencyRate = float64(consistent) / float64(overview.WithOutputID)
		}
		alerts, health := buildVideoJobFeedbackIntegrityAlerts(overview, thresholds)
		alertCount := len(alerts)
		alertCodes := make([]string, 0, len(alerts))
		for _, alert := range alerts {
			code := strings.TrimSpace(alert.Code)
			if code == "" || code == "feedback_integrity_no_data" {
				continue
			}
			alertCodes = append(alertCodes, code)
		}
		if len(alertCodes) > 1 {
			sort.Strings(alertCodes)
			unique := alertCodes[:0]
			for _, code := range alertCodes {
				if len(unique) == 0 || unique[len(unique)-1] != code {
					unique = append(unique, code)
				}
			}
			alertCodes = unique
		}
		if overview.Samples <= 0 {
			health = "no_data"
			alertCount = 0
			alertCodes = nil
		}
		out = append(out, AdminVideoJobFeedbackIntegrityHealthTrendPoint{
			Bucket:                   item.BucketDay.Format("2006-01-02"),
			Samples:                  overview.Samples,
			Health:                   health,
			AlertCount:               alertCount,
			OutputCoverageRate:       overview.OutputCoverageRate,
			OutputResolvedRate:       overview.OutputResolvedRate,
			OutputJobConsistencyRate: overview.OutputJobConsistencyRate,
			TopPickMultiHitUsers:     overview.TopPickMultiHitUsers,
			AlertCodes:               alertCodes,
		})
	}
	return out, nil
}

func resolveFeedbackIntegrityRecovery(
	trend []AdminVideoJobFeedbackIntegrityHealthTrendPoint,
) (status string, recovered bool, previousHealth string) {
	if len(trend) == 0 {
		return "no_data", false, ""
	}

	lastIndex := -1
	for i := len(trend) - 1; i >= 0; i-- {
		health := strings.ToLower(strings.TrimSpace(trend[i].Health))
		if health == "" || health == "no_data" {
			continue
		}
		lastIndex = i
		break
	}
	if lastIndex < 0 {
		return "no_data", false, ""
	}

	currentHealth := strings.ToLower(strings.TrimSpace(trend[lastIndex].Health))
	previousHealth = ""
	for i := lastIndex - 1; i >= 0; i-- {
		health := strings.ToLower(strings.TrimSpace(trend[i].Health))
		if health == "" || health == "no_data" {
			continue
		}
		previousHealth = health
		break
	}
	if previousHealth == "" {
		return "insufficient_history", false, ""
	}

	if currentHealth == "green" && (previousHealth == "yellow" || previousHealth == "red") {
		return "recovered", true, previousHealth
	}
	if (currentHealth == "yellow" || currentHealth == "red") && previousHealth == "green" {
		return "regressed", false, previousHealth
	}
	return "stable", false, previousHealth
}

func buildFeedbackIntegrityAlertCodeStats(
	trend []AdminVideoJobFeedbackIntegrityHealthTrendPoint,
) []AdminVideoJobFeedbackIntegrityAlertCodeStat {
	type agg struct {
		DaysHit      int
		LatestLevel  string
		LatestBucket string
	}
	acc := make(map[string]agg, 16)

	for _, point := range trend {
		bucket := strings.TrimSpace(point.Bucket)
		level := strings.ToLower(strings.TrimSpace(point.Health))
		codes := point.AlertCodes
		if len(codes) == 0 {
			continue
		}
		seen := make(map[string]struct{}, len(codes))
		for _, raw := range codes {
			code := strings.TrimSpace(raw)
			if code == "" {
				continue
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			current := acc[code]
			current.DaysHit++
			if current.LatestBucket == "" || bucket > current.LatestBucket {
				current.LatestBucket = bucket
				current.LatestLevel = level
			}
			acc[code] = current
		}
	}

	out := make([]AdminVideoJobFeedbackIntegrityAlertCodeStat, 0, len(acc))
	for code, item := range acc {
		out = append(out, AdminVideoJobFeedbackIntegrityAlertCodeStat{
			Code:         code,
			DaysHit:      item.DaysHit,
			LatestLevel:  item.LatestLevel,
			LatestBucket: item.LatestBucket,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DaysHit != out[j].DaysHit {
			return out[i].DaysHit > out[j].DaysHit
		}
		if out[i].LatestBucket != out[j].LatestBucket {
			return out[i].LatestBucket > out[j].LatestBucket
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func buildFeedbackIntegrityStreaks(
	trend []AdminVideoJobFeedbackIntegrityHealthTrendPoint,
) AdminVideoJobFeedbackIntegrityStreaks {
	out := AdminVideoJobFeedbackIntegrityStreaks{}
	if len(trend) == 0 {
		return out
	}

	normalize := func(value string) string {
		return strings.ToLower(strings.TrimSpace(value))
	}

	for _, point := range trend {
		health := normalize(point.Health)
		if health == "red" || health == "yellow" {
			out.LastNonGreenBucket = strings.TrimSpace(point.Bucket)
		}
		if health == "red" {
			out.LastRedBucket = strings.TrimSpace(point.Bucket)
		}
	}

	recent := trend
	if len(recent) > 7 {
		recent = recent[len(recent)-7:]
	}
	for _, point := range recent {
		health := normalize(point.Health)
		if health == "red" {
			out.Recent7dRedDays++
			out.Recent7dNonGreenDays++
		} else if health == "yellow" {
			out.Recent7dNonGreenDays++
		}
	}

	for i := len(trend) - 1; i >= 0; i-- {
		health := normalize(trend[i].Health)
		if health == "red" {
			out.ConsecutiveRedDays++
		} else {
			break
		}
	}
	for i := len(trend) - 1; i >= 0; i-- {
		health := normalize(trend[i].Health)
		if health == "red" || health == "yellow" {
			out.ConsecutiveNonGreenDays++
		} else {
			break
		}
	}
	for i := len(trend) - 1; i >= 0; i-- {
		health := normalize(trend[i].Health)
		if health == "green" {
			out.ConsecutiveGreenDays++
		} else {
			break
		}
	}
	return out
}

func buildFeedbackIntegrityEscalation(
	health string,
	streaks AdminVideoJobFeedbackIntegrityStreaks,
	delta *AdminVideoJobFeedbackIntegrityDelta,
	recoveryStatus string,
) AdminVideoJobFeedbackIntegrityEscalation {
	normalizedHealth := strings.ToLower(strings.TrimSpace(health))
	normalizedRecovery := strings.ToLower(strings.TrimSpace(recoveryStatus))

	triggered := make([]string, 0, 6)
	reasonParts := make([]string, 0, 4)
	levelRank := func(value string) int {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "oncall":
			return 3
		case "watch":
			return 2
		case "notice":
			return 1
		default:
			return 0
		}
	}
	level := "none"
	setLevel := func(target string) {
		if levelRank(target) > levelRank(level) {
			level = target
		}
	}
	addRule := func(rule string, reason string) {
		rule = strings.TrimSpace(rule)
		if rule != "" {
			triggered = append(triggered, rule)
		}
		reason = strings.TrimSpace(reason)
		if reason != "" {
			reasonParts = append(reasonParts, reason)
		}
	}

	if normalizedHealth == "red" && streaks.ConsecutiveRedDays >= 2 {
		setLevel("oncall")
		addRule("consecutive_red_2d", "连续红色 >=2 天")
	}
	if normalizedHealth == "red" && streaks.Recent7dRedDays >= 3 {
		setLevel("oncall")
		addRule("recent_7d_red_3d", "近7天红色 >=3 天")
	}
	if delta != nil && delta.AlertCountDelta >= 2 && normalizedHealth != "green" {
		setLevel("watch")
		addRule("alert_count_growth", "告警数量较上一窗口明显上升")
	}
	if delta != nil && delta.TopPickMultiHitUsersDelta > 0 {
		setLevel("watch")
		addRule("top_pick_conflict_growth", "top_pick 冲突用户上升")
	}
	if streaks.ConsecutiveNonGreenDays >= 3 && normalizedHealth != "green" {
		setLevel("watch")
		addRule("consecutive_non_green_3d", "连续非绿 >=3 天")
	}
	if normalizedRecovery == "recovered" && levelRank(level) > levelRank("notice") {
		// 已恢复时降级一级，避免误报持续升级
		level = "notice"
		addRule("recovered_downgrade", "当前已恢复，降级为观察通知")
	}

	required := level == "watch" || level == "oncall"
	if len(triggered) == 0 {
		if normalizedHealth == "green" {
			return AdminVideoJobFeedbackIntegrityEscalation{
				Required: false,
				Level:    "none",
				Reason:   "当前完整性健康，无需升级处理",
			}
		}
		if normalizedHealth == "yellow" {
			return AdminVideoJobFeedbackIntegrityEscalation{
				Required: false,
				Level:    "notice",
				Reason:   "存在轻度告警，建议持续观察",
			}
		}
		if normalizedHealth == "red" {
			return AdminVideoJobFeedbackIntegrityEscalation{
				Required: true,
				Level:    "watch",
				Reason:   "当前为红色健康状态，建议立即排障",
			}
		}
		return AdminVideoJobFeedbackIntegrityEscalation{
			Required: false,
			Level:    "none",
			Reason:   "当前窗口反馈样本不足",
		}
	}

	// 去重 triggered rules，保留原始顺序
	dedup := make([]string, 0, len(triggered))
	seen := make(map[string]struct{}, len(triggered))
	for _, rule := range triggered {
		if _, ok := seen[rule]; ok {
			continue
		}
		seen[rule] = struct{}{}
		dedup = append(dedup, rule)
	}
	reasonText := strings.Join(reasonParts, "；")
	if strings.TrimSpace(reasonText) == "" {
		reasonText = "满足升级规则"
	}

	return AdminVideoJobFeedbackIntegrityEscalation{
		Required:       required,
		Level:          level,
		Reason:         reasonText,
		TriggeredRules: dedup,
	}
}

func deriveFeedbackIntegrityRecoveryStatusForPoint(currentHealth, previousHealth string) string {
	current := strings.ToLower(strings.TrimSpace(currentHealth))
	previous := strings.ToLower(strings.TrimSpace(previousHealth))
	if current == "" || current == "no_data" {
		return "no_data"
	}
	if previous == "" || previous == "no_data" {
		return "stable"
	}
	if (previous == "red" || previous == "yellow") && current == "green" {
		return "recovered"
	}
	if previous == "green" && (current == "red" || current == "yellow") {
		return "regressed"
	}
	return "stable"
}

func buildFeedbackIntegrityEscalationTrend(
	trend []AdminVideoJobFeedbackIntegrityHealthTrendPoint,
) []AdminVideoJobFeedbackIntegrityEscalationTrendPoint {
	if len(trend) == 0 {
		return nil
	}
	out := make([]AdminVideoJobFeedbackIntegrityEscalationTrendPoint, 0, len(trend))
	for idx, point := range trend {
		previousHealth := ""
		if idx > 0 {
			previousHealth = trend[idx-1].Health
		}
		recoveryStatus := deriveFeedbackIntegrityRecoveryStatusForPoint(point.Health, previousHealth)
		streaks := buildFeedbackIntegrityStreaks(trend[:idx+1])
		var delta *AdminVideoJobFeedbackIntegrityDelta
		alertCountDelta := 0
		topPickConflictDelta := int64(0)
		if idx > 0 {
			alertCountDelta = point.AlertCount - trend[idx-1].AlertCount
			topPickConflictDelta = point.TopPickMultiHitUsers - trend[idx-1].TopPickMultiHitUsers
			delta = &AdminVideoJobFeedbackIntegrityDelta{
				HasPreviousData:           true,
				AlertCountDelta:           alertCountDelta,
				TopPickMultiHitUsersDelta: topPickConflictDelta,
			}
		}
		escalation := buildFeedbackIntegrityEscalation(point.Health, streaks, delta, recoveryStatus)
		out = append(out, AdminVideoJobFeedbackIntegrityEscalationTrendPoint{
			Bucket:                    strings.TrimSpace(point.Bucket),
			Health:                    strings.ToLower(strings.TrimSpace(point.Health)),
			RecoveryStatus:            recoveryStatus,
			AlertCount:                point.AlertCount,
			AlertCountDelta:           alertCountDelta,
			TopPickMultiHitUsers:      point.TopPickMultiHitUsers,
			TopPickMultiHitUsersDelta: topPickConflictDelta,
			EscalationLevel:           escalation.Level,
			EscalationRequired:        escalation.Required,
			EscalationReason:          escalation.Reason,
			TriggeredRules:            escalation.TriggeredRules,
		})
	}
	return out
}

func buildFeedbackIntegrityEscalationStats(
	points []AdminVideoJobFeedbackIntegrityEscalationTrendPoint,
) AdminVideoJobFeedbackIntegrityEscalationStats {
	stats := AdminVideoJobFeedbackIntegrityEscalationStats{}
	if len(points) == 0 {
		return stats
	}
	stats.TotalDays = len(points)
	for _, item := range points {
		if item.EscalationRequired {
			stats.RequiredDays++
		}
		switch strings.ToLower(strings.TrimSpace(item.EscalationLevel)) {
		case "oncall":
			stats.OncallDays++
		case "watch":
			stats.WatchDays++
		case "notice":
			stats.NoticeDays++
		default:
			stats.NoneDays++
		}
	}
	last := points[len(points)-1]
	stats.LatestBucket = strings.TrimSpace(last.Bucket)
	stats.LatestLevel = strings.ToLower(strings.TrimSpace(last.EscalationLevel))
	stats.LatestRequired = last.EscalationRequired
	stats.LatestReason = strings.TrimSpace(last.EscalationReason)
	return stats
}

func buildFeedbackIntegrityEscalationIncidents(
	points []AdminVideoJobFeedbackIntegrityEscalationTrendPoint,
	limit int,
) []AdminVideoJobFeedbackIntegrityEscalationIncident {
	if len(points) == 0 || limit == 0 {
		return nil
	}
	if limit < 0 {
		limit = 0
	}
	capHint := len(points)
	if limit > 0 && limit < capHint {
		capHint = limit
	}
	out := make([]AdminVideoJobFeedbackIntegrityEscalationIncident, 0, capHint)
	for idx := len(points) - 1; idx >= 0; idx-- {
		item := points[idx]
		if !item.EscalationRequired {
			continue
		}
		out = append(out, AdminVideoJobFeedbackIntegrityEscalationIncident{
			Bucket:                    strings.TrimSpace(item.Bucket),
			EscalationLevel:           strings.ToLower(strings.TrimSpace(item.EscalationLevel)),
			EscalationRequired:        item.EscalationRequired,
			EscalationReason:          strings.TrimSpace(item.EscalationReason),
			TriggeredRules:            item.TriggeredRules,
			AlertCount:                item.AlertCount,
			AlertCountDelta:           item.AlertCountDelta,
			TopPickMultiHitUsers:      item.TopPickMultiHitUsers,
			TopPickMultiHitUsersDelta: item.TopPickMultiHitUsersDelta,
			RecoveryStatus:            strings.ToLower(strings.TrimSpace(item.RecoveryStatus)),
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (h *Handler) loadVideoImageFeedbackIntegrityRiskJobCounts(
	since time.Time,
	filter *videoImageFeedbackFilter,
) (anomalyJobs int64, topPickConflictJobs int64, err error) {
	type row struct {
		AnomalyJobs         int64 `gorm:"column:anomaly_jobs"`
		TopPickConflictJobs int64 `gorm:"column:top_pick_conflict_jobs"`
	}

	tables := resolveVideoImageReadTablesByFilter(filter)
	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQLWithTables(filter, "f", tables.Jobs)
	query := fmt.Sprintf(`
WITH base AS (
	SELECT
		f.id,
		f.job_id,
		f.user_id,
		f.output_id,
		LOWER(COALESCE(f.action, '')) AS action
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
),
anomaly_jobs AS (
	SELECT COUNT(DISTINCT job_id)::bigint AS jobs
	FROM joined
	WHERE output_id IS NULL
		OR (output_id IS NOT NULL AND output_exists_id IS NULL)
		OR (output_id IS NOT NULL AND output_exists_id IS NOT NULL AND output_job_id <> job_id)
),
top_pick_conflict_jobs AS (
	SELECT COUNT(DISTINCT job_id)::bigint AS jobs
	FROM (
		SELECT job_id, user_id
		FROM base
		WHERE action = 'top_pick'
		GROUP BY job_id, user_id
		HAVING COUNT(*) > 1
	) t
)
SELECT
	COALESCE((SELECT jobs FROM anomaly_jobs), 0)::bigint AS anomaly_jobs,
	COALESCE((SELECT jobs FROM top_pick_conflict_jobs), 0)::bigint AS top_pick_conflict_jobs
`, tables.Feedback, tables.Outputs)
	args := []interface{}{since}
	args = append(args, filterArgs...)

	var dbRow row
	if err = h.db.Raw(query, args...).Scan(&dbRow).Error; err != nil {
		return 0, 0, err
	}
	return dbRow.AnomalyJobs, dbRow.TopPickConflictJobs, nil
}

type adminVideoImageFeedbackIntegrityActionRow struct {
	Action                   string
	Samples                  int64
	WithOutputID             int64
	MissingOutputID          int64
	ResolvedOutput           int64
	OrphanOutput             int64
	JobMismatch              int64
	OutputCoverageRate       float64
	OutputResolvedRate       float64
	OutputJobConsistencyRate float64
}

func (h *Handler) loadVideoImageFeedbackIntegrityActionRows(
	since time.Time,
	filter *videoImageFeedbackFilter,
	limit int,
) ([]adminVideoImageFeedbackIntegrityActionRow, error) {
	type row struct {
		Action          string `gorm:"column:action"`
		Samples         int64  `gorm:"column:samples"`
		WithOutputID    int64  `gorm:"column:with_output_id"`
		MissingOutputID int64  `gorm:"column:missing_output_id"`
		ResolvedOutput  int64  `gorm:"column:resolved_output"`
		OrphanOutput    int64  `gorm:"column:orphan_output"`
		JobMismatch     int64  `gorm:"column:job_mismatch"`
	}

	tables := resolveVideoImageReadTablesByFilter(filter)
	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQLWithTables(filter, "f", tables.Jobs)
	query := fmt.Sprintf(`
WITH base AS (
	SELECT
		f.id,
		f.job_id,
		f.output_id,
		LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) AS action
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
	action,
	COUNT(*)::bigint AS samples,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL)::bigint AS with_output_id,
	COUNT(*) FILTER (WHERE output_id IS NULL)::bigint AS missing_output_id,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NOT NULL)::bigint AS resolved_output,
	COUNT(*) FILTER (WHERE output_id IS NOT NULL AND output_exists_id IS NULL)::bigint AS orphan_output,
	COUNT(*) FILTER (
		WHERE output_id IS NOT NULL
			AND output_exists_id IS NOT NULL
			AND output_job_id <> job_id
	)::bigint AS job_mismatch
FROM joined
GROUP BY action
ORDER BY samples DESC, action ASC
`, tables.Feedback, tables.Outputs)
	args := []interface{}{since}
	args = append(args, filterArgs...)
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	var rows []row
	if err := h.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]adminVideoImageFeedbackIntegrityActionRow, 0, len(rows))
	for _, item := range rows {
		rowOut := adminVideoImageFeedbackIntegrityActionRow{
			Action:          strings.TrimSpace(item.Action),
			Samples:         item.Samples,
			WithOutputID:    item.WithOutputID,
			MissingOutputID: item.MissingOutputID,
			ResolvedOutput:  item.ResolvedOutput,
			OrphanOutput:    item.OrphanOutput,
			JobMismatch:     item.JobMismatch,
		}
		if rowOut.Samples > 0 {
			rowOut.OutputCoverageRate = float64(rowOut.WithOutputID) / float64(rowOut.Samples)
		}
		if rowOut.WithOutputID > 0 {
			rowOut.OutputResolvedRate = float64(rowOut.ResolvedOutput) / float64(rowOut.WithOutputID)
			consistent := rowOut.ResolvedOutput - rowOut.JobMismatch
			if consistent < 0 {
				consistent = 0
			}
			rowOut.OutputJobConsistencyRate = float64(consistent) / float64(rowOut.WithOutputID)
		}
		out = append(out, rowOut)
	}
	return out, nil
}

func (h *Handler) loadVideoImageFeedbackActionStats(
	since time.Time,
	filter *videoImageFeedbackFilter,
) ([]AdminVideoJobFeedbackActionStat, error) {
	type row struct {
		Action    string   `gorm:"column:action"`
		Count     int64    `gorm:"column:count"`
		WeightSum *float64 `gorm:"column:weight_sum"`
	}

	tables := resolveVideoImageReadTablesByFilter(filter)
	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQLWithTables(filter, "f", tables.Jobs)
	query := fmt.Sprintf(`
SELECT
	LOWER(COALESCE(NULLIF(TRIM(action), ''), 'unknown')) AS action,
	COUNT(*)::bigint AS count,
	COALESCE(SUM(weight), 0)::double precision AS weight_sum
FROM %s f
WHERE f.created_at >= ?
`+filterSQL+`
GROUP BY 1
ORDER BY count DESC, action ASC
LIMIT 16
`, tables.Feedback)

	args := []interface{}{since}
	args = append(args, filterArgs...)

	var rows []row
	if err := h.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	total := int64(0)
	for _, item := range rows {
		total += item.Count
	}

	out := make([]AdminVideoJobFeedbackActionStat, 0, len(rows))
	for _, item := range rows {
		ratio := 0.0
		if total > 0 {
			ratio = float64(item.Count) / float64(total)
		}
		weightSum := 0.0
		if item.WeightSum != nil {
			weightSum = *item.WeightSum
		}
		out = append(out, AdminVideoJobFeedbackActionStat{
			Action:    strings.TrimSpace(item.Action),
			Count:     item.Count,
			Ratio:     ratio,
			WeightSum: weightSum,
		})
	}
	return out, nil
}

func (h *Handler) loadVideoImageFeedbackTopSceneStats(
	since time.Time,
	filter *videoImageFeedbackFilter,
) ([]AdminVideoJobFeedbackSceneStat, error) {
	type row struct {
		SceneTag string `gorm:"column:scene_tag"`
		Signals  int64  `gorm:"column:signals"`
	}

	tables := resolveVideoImageReadTablesByFilter(filter)
	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQLWithTables(filter, "f", tables.Jobs)
	query := fmt.Sprintf(`
SELECT
	COALESCE(
		NULLIF(LOWER(TRIM(scene_tag)), ''),
		NULLIF(LOWER(TRIM(metadata->>'scene_tag')), ''),
		'uncategorized'
	) AS scene_tag,
	COUNT(*)::bigint AS signals
FROM %s f
WHERE f.created_at >= ?
`+filterSQL+`
GROUP BY 1
ORDER BY signals DESC, scene_tag ASC
LIMIT 16
`, tables.Feedback)
	args := []interface{}{since}
	args = append(args, filterArgs...)

	var rows []row
	if err := h.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobFeedbackSceneStat, 0, len(rows))
	for _, item := range rows {
		out = append(out, AdminVideoJobFeedbackSceneStat{
			SceneTag: strings.TrimSpace(item.SceneTag),
			Signals:  item.Signals,
		})
	}
	return out, nil
}

func (h *Handler) loadVideoImageFeedbackTrend(
	since,
	until time.Time,
	windowDuration time.Duration,
	filter *videoImageFeedbackFilter,
) ([]AdminVideoJobFeedbackTrendPoint, error) {
	precision := "day"
	step := 24 * time.Hour
	if windowDuration <= 24*time.Hour {
		precision = "hour"
		step = time.Hour
	}

	type row struct {
		BucketTS time.Time `gorm:"column:bucket_ts"`
		Total    int64     `gorm:"column:total"`
		Positive int64     `gorm:"column:positive"`
		Neutral  int64     `gorm:"column:neutral"`
		Negative int64     `gorm:"column:negative"`
		TopPick  int64     `gorm:"column:top_pick"`
	}

	tables := resolveVideoImageReadTablesByFilter(filter)
	filterSQL, filterArgs := buildVideoImageFeedbackFilterSQLWithTables(filter, "f", tables.Jobs)
	query := fmt.Sprintf(`
SELECT
	date_trunc(?, f.created_at) AS bucket_ts,
	COUNT(*)::bigint AS total,
	COUNT(*) FILTER (
		WHERE LOWER(COALESCE(action, '')) IN ('download', 'favorite', 'share', 'use', 'like', 'top_pick')
	)::bigint AS positive,
	COUNT(*) FILTER (WHERE LOWER(COALESCE(action, '')) = 'neutral')::bigint AS neutral,
	COUNT(*) FILTER (WHERE LOWER(COALESCE(action, '')) = 'dislike')::bigint AS negative,
	COUNT(*) FILTER (WHERE LOWER(COALESCE(action, '')) = 'top_pick')::bigint AS top_pick
FROM %s f
WHERE f.created_at >= ?
	AND f.created_at <= ?
`+filterSQL+`
GROUP BY 1
ORDER BY 1 ASC
`, tables.Feedback)
	args := []interface{}{precision, since, until}
	args = append(args, filterArgs...)

	var rows []row
	if err := h.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	align := func(t time.Time) time.Time {
		if precision == "hour" {
			return t.Truncate(time.Hour)
		}
		year, month, day := t.Date()
		return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
	}

	rowByBucket := make(map[int64]row, len(rows))
	for _, item := range rows {
		bucket := align(item.BucketTS).Unix()
		rowByBucket[bucket] = item
	}

	start := align(since)
	end := align(until)
	if end.Before(start) {
		end = start
	}

	out := make([]AdminVideoJobFeedbackTrendPoint, 0, len(rows))
	for cursor := start; !cursor.After(end); cursor = cursor.Add(step) {
		bucket := align(cursor)
		key := bucket.Unix()
		item, ok := rowByBucket[key]
		if !ok {
			out = append(out, AdminVideoJobFeedbackTrendPoint{
				Bucket: bucket.Format(time.RFC3339),
			})
			continue
		}
		out = append(out, AdminVideoJobFeedbackTrendPoint{
			Bucket:   bucket.Format(time.RFC3339),
			Total:    item.Total,
			Positive: item.Positive,
			Neutral:  item.Neutral,
			Negative: item.Negative,
			TopPick:  item.TopPick,
		})
	}
	return out, nil
}
