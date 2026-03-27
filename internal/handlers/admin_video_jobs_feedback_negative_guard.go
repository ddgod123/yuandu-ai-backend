package handlers

import (
	"fmt"
	"strings"
	"time"

	"emoji/internal/models"
)

func (h *Handler) loadVideoJobFeedbackNegativeGuardOverview(
	since time.Time,
	filter *videoImageFeedbackFilter,
) (AdminVideoJobFeedbackNegativeGuardOverview, error) {
	type row struct {
		Samples            int64    `gorm:"column:samples"`
		TreatmentJobs      int64    `gorm:"column:treatment_jobs"`
		GuardEnabledJobs   int64    `gorm:"column:guard_enabled_jobs"`
		GuardReasonHitJobs int64    `gorm:"column:guard_reason_hit_jobs"`
		SelectionShiftJobs int64    `gorm:"column:selection_shift_jobs"`
		BlockedReasonJobs  int64    `gorm:"column:blocked_reason_jobs"`
		AvgNegativeSignals *float64 `gorm:"column:avg_negative_signals"`
		AvgPositiveSignals *float64 `gorm:"column:avg_positive_signals"`
	}

	var dbRow row
	filterSQL, filterArgs := buildVideoJobFilterClause(filter, "j")
	query := `
WITH base AS (
	SELECT
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->>'group'), ''), 'unknown')) AS group_name,
		CASE
			WHEN LOWER(COALESCE(j.metrics->'highlight_feedback_v1'->>'negative_guard_enabled', 'false')) IN ('true', '1', 't', 'yes', 'y') THEN 1
			ELSE 0
		END AS guard_enabled,
		CASE
			WHEN jsonb_typeof(j.metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
				AND (j.metrics->'highlight_feedback_v1'->'reason_negative_guard') <> '{}'::jsonb
			THEN 1
			ELSE 0
		END AS guard_reason_hit,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->>'public_negative_signals', '')::double precision, 0) AS public_negative_signals,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->>'public_positive_signals', '')::double precision, 0) AS public_positive_signals,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), '')) AS before_reason,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_after'->>'reason'), ''), '')) AS after_reason,
		COALESCE(j.metrics->'highlight_feedback_v1'->'reason_negative_guard', '{}'::jsonb) AS reason_negative_guard,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->'selected_before'->>'start_sec', '')::double precision, -1) AS before_start_sec,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->'selected_before'->>'end_sec', '')::double precision, -1) AS before_end_sec,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->'selected_after'->>'start_sec', '')::double precision, -1) AS after_start_sec,
		COALESCE(NULLIF(j.metrics->'highlight_feedback_v1'->'selected_after'->>'end_sec', '')::double precision, -1) AS after_end_sec
	FROM archive.video_jobs j
	WHERE j.status = ?
	  AND j.finished_at >= ?
` + filterSQL + `
)
SELECT
	COUNT(*) FILTER (WHERE group_name IN ('treatment', 'control'))::bigint AS samples,
	COUNT(*) FILTER (WHERE group_name = 'treatment')::bigint AS treatment_jobs,
	COUNT(*) FILTER (WHERE group_name = 'treatment' AND guard_enabled = 1)::bigint AS guard_enabled_jobs,
	COUNT(*) FILTER (WHERE group_name = 'treatment' AND guard_reason_hit = 1)::bigint AS guard_reason_hit_jobs,
	COUNT(*) FILTER (
		WHERE group_name = 'treatment'
			AND guard_reason_hit = 1
			AND (ABS(before_start_sec - after_start_sec) > 0.0001 OR ABS(before_end_sec - after_end_sec) > 0.0001)
	)::bigint AS selection_shift_jobs,
	COUNT(*) FILTER (
		WHERE group_name = 'treatment'
			AND guard_reason_hit = 1
			AND before_reason <> ''
			AND jsonb_exists(reason_negative_guard, before_reason)
			AND after_reason <> ''
			AND after_reason <> before_reason
	)::bigint AS blocked_reason_jobs,
	AVG(public_negative_signals) FILTER (WHERE group_name = 'treatment' AND guard_reason_hit = 1) AS avg_negative_signals,
	AVG(public_positive_signals) FILTER (WHERE group_name = 'treatment' AND guard_reason_hit = 1) AS avg_positive_signals
FROM base
`
	args := []interface{}{models.VideoJobStatusDone, since}
	args = append(args, filterArgs...)
	if err := h.db.Raw(query, args...).Scan(&dbRow).Error; err != nil {
		return AdminVideoJobFeedbackNegativeGuardOverview{}, err
	}

	out := AdminVideoJobFeedbackNegativeGuardOverview{
		Samples:            dbRow.Samples,
		TreatmentJobs:      dbRow.TreatmentJobs,
		GuardEnabledJobs:   dbRow.GuardEnabledJobs,
		GuardReasonHitJobs: dbRow.GuardReasonHitJobs,
		SelectionShiftJobs: dbRow.SelectionShiftJobs,
		BlockedReasonJobs:  dbRow.BlockedReasonJobs,
	}
	if dbRow.AvgNegativeSignals != nil {
		out.AvgNegativeSignals = *dbRow.AvgNegativeSignals
	}
	if dbRow.AvgPositiveSignals != nil {
		out.AvgPositiveSignals = *dbRow.AvgPositiveSignals
	}

	if out.GuardEnabledJobs > 0 {
		out.GuardHitRate = float64(out.GuardReasonHitJobs) / float64(out.GuardEnabledJobs)
	}
	if out.GuardReasonHitJobs > 0 {
		out.SelectionShiftRate = float64(out.SelectionShiftJobs) / float64(out.GuardReasonHitJobs)
		out.BlockedReasonRate = float64(out.BlockedReasonJobs) / float64(out.GuardReasonHitJobs)
	}
	return out, nil
}

func (h *Handler) loadVideoJobFeedbackNegativeGuardReasonStats(
	since time.Time,
	filter *videoImageFeedbackFilter,
	limit int,
) ([]AdminVideoJobFeedbackNegativeGuardReasonStat, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	type row struct {
		Reason      string   `gorm:"column:reason"`
		Jobs        int64    `gorm:"column:jobs"`
		BlockedJobs int64    `gorm:"column:blocked_jobs"`
		AvgWeight   *float64 `gorm:"column:avg_weight"`
	}
	filterSQL, filterArgs := buildVideoJobFilterClause(filter, "j")
	tables := resolveVideoImageReadTablesByFilter(filter)
	query := fmt.Sprintf(`
WITH base AS (
	SELECT
		j.id AS job_id,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->>'group'), ''), 'unknown')) AS group_name,
		LOWER(TRIM(reason_entry.key)) AS reason,
		CASE
			WHEN reason_entry.value ~ '^([0-9]+|-[0-9]+)(\.[0-9]+){0,1}$' THEN reason_entry.value::double precision
			ELSE NULL
		END AS reason_weight,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), '')) AS before_reason,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_after'->>'reason'), ''), '')) AS after_reason
	FROM %s j
	CROSS JOIN LATERAL jsonb_each_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
			THEN j.metrics->'highlight_feedback_v1'->'reason_negative_guard'
			ELSE '{}'::jsonb
		END
	) AS reason_entry(key, value)
	WHERE j.status = ?
		AND j.finished_at >= ?
`+filterSQL+`
)
SELECT
	reason,
	COUNT(DISTINCT job_id)::bigint AS jobs,
	COUNT(DISTINCT job_id) FILTER (
		WHERE before_reason = reason
			AND after_reason <> ''
			AND after_reason <> before_reason
	)::bigint AS blocked_jobs,
	AVG(reason_weight) AS avg_weight
FROM base
WHERE group_name = 'treatment'
	AND reason <> ''
GROUP BY reason
ORDER BY blocked_jobs DESC, jobs DESC, reason ASC
LIMIT ?
`, tables.Jobs)

	args := []interface{}{models.VideoJobStatusDone, since}
	args = append(args, filterArgs...)
	args = append(args, limit)

	var dbRows []row
	if err := h.db.Raw(query, args...).Scan(&dbRows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobFeedbackNegativeGuardReasonStat, 0, len(dbRows))
	for _, item := range dbRows {
		reason := strings.TrimSpace(item.Reason)
		if reason == "" {
			continue
		}
		avgWeight := 0.0
		if item.AvgWeight != nil {
			avgWeight = *item.AvgWeight
		}
		out = append(out, AdminVideoJobFeedbackNegativeGuardReasonStat{
			Reason:      reason,
			Jobs:        item.Jobs,
			BlockedJobs: item.BlockedJobs,
			AvgWeight:   avgWeight,
		})
	}
	return out, nil
}

func (h *Handler) loadVideoJobFeedbackNegativeGuardJobRows(
	since time.Time,
	filter *videoImageFeedbackFilter,
	limit int,
	blockedOnly bool,
) ([]AdminVideoJobFeedbackNegativeGuardJobRow, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}

	type row struct {
		JobID           uint64     `gorm:"column:job_id"`
		UserID          uint64     `gorm:"column:user_id"`
		Title           string     `gorm:"column:title"`
		GroupName       string     `gorm:"column:group_name"`
		GuardHit        bool       `gorm:"column:guard_hit"`
		BlockedReason   bool       `gorm:"column:blocked_reason"`
		BeforeReason    string     `gorm:"column:before_reason"`
		AfterReason     string     `gorm:"column:after_reason"`
		BeforeStartSec  *float64   `gorm:"column:before_start_sec"`
		BeforeEndSec    *float64   `gorm:"column:before_end_sec"`
		AfterStartSec   *float64   `gorm:"column:after_start_sec"`
		AfterEndSec     *float64   `gorm:"column:after_end_sec"`
		GuardReasonList string     `gorm:"column:guard_reason_list"`
		FinishedAt      *time.Time `gorm:"column:finished_at"`
	}

	filterSQL, filterArgs := buildVideoJobFilterClause(filter, "j")
	tables := resolveVideoImageReadTablesByFilter(filter)
	query := fmt.Sprintf(`
WITH base AS (
	SELECT
		j.id AS job_id,
		j.user_id AS user_id,
		COALESCE(j.title, '') AS title,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->>'group'), ''), 'unknown')) AS group_name,
		COALESCE(j.metrics->'highlight_feedback_v1'->'reason_negative_guard', '{}'::jsonb) AS reason_negative_guard,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), '')) AS before_reason,
		LOWER(COALESCE(NULLIF(TRIM(j.metrics->'highlight_feedback_v1'->'selected_after'->>'reason'), ''), '')) AS after_reason,
		NULLIF(j.metrics->'highlight_feedback_v1'->'selected_before'->>'start_sec', '')::double precision AS before_start_sec,
		NULLIF(j.metrics->'highlight_feedback_v1'->'selected_before'->>'end_sec', '')::double precision AS before_end_sec,
		NULLIF(j.metrics->'highlight_feedback_v1'->'selected_after'->>'start_sec', '')::double precision AS after_start_sec,
		NULLIF(j.metrics->'highlight_feedback_v1'->'selected_after'->>'end_sec', '')::double precision AS after_end_sec,
		j.finished_at AS finished_at
	FROM %s j
	WHERE j.status = ?
		AND j.finished_at >= ?
`+filterSQL+`
)
SELECT
	job_id,
	user_id,
	title,
	group_name,
	(jsonb_typeof(reason_negative_guard) = 'object' AND reason_negative_guard <> '{}'::jsonb) AS guard_hit,
	(
		(jsonb_typeof(reason_negative_guard) = 'object' AND reason_negative_guard <> '{}'::jsonb)
		AND before_reason <> ''
		AND jsonb_exists(reason_negative_guard, before_reason)
		AND after_reason <> ''
		AND after_reason <> before_reason
	) AS blocked_reason,
	before_reason,
	after_reason,
	before_start_sec,
	before_end_sec,
	after_start_sec,
	after_end_sec,
	COALESCE((
		SELECT string_agg(r.key || ':' || r.value, ' | ' ORDER BY r.key ASC)
		FROM jsonb_each_text(
			CASE
				WHEN jsonb_typeof(reason_negative_guard) = 'object'
				THEN reason_negative_guard
				ELSE '{}'::jsonb
			END
		) AS r(key, value)
	), '') AS guard_reason_list,
	finished_at
FROM base
WHERE group_name = 'treatment'
	AND jsonb_typeof(reason_negative_guard) = 'object'
	AND reason_negative_guard <> '{}'::jsonb
	AND (
		? = FALSE
		OR (
			before_reason <> ''
			AND after_reason <> ''
			AND after_reason <> before_reason
			AND jsonb_exists(reason_negative_guard, before_reason)
		)
	)
ORDER BY blocked_reason DESC, finished_at DESC NULLS LAST, job_id DESC
LIMIT ?
`, tables.Jobs)

	args := []interface{}{models.VideoJobStatusDone, since}
	args = append(args, filterArgs...)
	args = append(args, blockedOnly)
	args = append(args, limit)

	var dbRows []row
	if err := h.db.Raw(query, args...).Scan(&dbRows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobFeedbackNegativeGuardJobRow, 0, len(dbRows))
	for _, item := range dbRows {
		out = append(out, AdminVideoJobFeedbackNegativeGuardJobRow{
			JobID:           item.JobID,
			UserID:          item.UserID,
			Title:           strings.TrimSpace(item.Title),
			Group:           strings.TrimSpace(item.GroupName),
			GuardHit:        item.GuardHit,
			BlockedReason:   item.BlockedReason,
			BeforeReason:    strings.TrimSpace(item.BeforeReason),
			AfterReason:     strings.TrimSpace(item.AfterReason),
			BeforeStartSec:  item.BeforeStartSec,
			BeforeEndSec:    item.BeforeEndSec,
			AfterStartSec:   item.AfterStartSec,
			AfterEndSec:     item.AfterEndSec,
			GuardReasonList: strings.TrimSpace(item.GuardReasonList),
			FinishedAt:      item.FinishedAt,
		})
	}
	return out, nil
}
