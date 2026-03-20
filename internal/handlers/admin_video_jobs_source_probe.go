package handlers

import (
	"fmt"
	"strings"
	"time"

	"emoji/internal/models"
)

func (h *Handler) loadVideoJobSourceProbeBuckets(since time.Time) (int64, []AdminVideoJobSimpleCount, []AdminVideoJobSimpleCount, []AdminVideoJobSimpleCount, error) {
	type countRow struct {
		Count int64 `gorm:"column:count"`
	}
	type bucketRow struct {
		Key       string `gorm:"column:key"`
		Count     int64  `gorm:"column:count"`
		SortOrder int    `gorm:"column:sort_order"`
	}

	baseCTE := `
WITH probe AS (
	SELECT
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'duration_sec') = 'number'
			THEN (v.options->'source_video_probe'->>'duration_sec')::double precision
			ELSE NULL
		END AS duration_sec,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'width') = 'number'
			THEN (v.options->'source_video_probe'->>'width')::double precision
			ELSE NULL
		END AS width,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'height') = 'number'
			THEN (v.options->'source_video_probe'->>'height')::double precision
			ELSE NULL
		END AS height,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'fps') = 'number'
			THEN (v.options->'source_video_probe'->>'fps')::double precision
			ELSE NULL
		END AS fps
	FROM archive.video_jobs v
	WHERE v.created_at >= ?
		AND jsonb_typeof(v.options->'source_video_probe') = 'object'
)
`

	var jobsWindowRow countRow
	if err := h.db.Raw(baseCTE+`
SELECT COUNT(*)::bigint AS count
FROM probe
`, since).Scan(&jobsWindowRow).Error; err != nil {
		return 0, nil, nil, nil, err
	}

	toCounts := func(rows []bucketRow) []AdminVideoJobSimpleCount {
		out := make([]AdminVideoJobSimpleCount, 0, len(rows))
		for _, row := range rows {
			key := strings.TrimSpace(row.Key)
			if key == "" {
				continue
			}
			out = append(out, AdminVideoJobSimpleCount{
				Key:   key,
				Count: row.Count,
			})
		}
		return out
	}

	var durationRows []bucketRow
	if err := h.db.Raw(baseCTE+`
SELECT
	bucket AS key,
	COUNT(*)::bigint AS count,
	MIN(sort_order)::int AS sort_order
FROM (
	SELECT
		CASE
			WHEN duration_sec < 5 THEN '<5s'
			WHEN duration_sec < 10 THEN '5-10s'
			WHEN duration_sec < 30 THEN '10-30s'
			WHEN duration_sec < 60 THEN '30-60s'
			WHEN duration_sec < 180 THEN '1-3m'
			WHEN duration_sec < 600 THEN '3-10m'
			ELSE '10m+'
		END AS bucket,
		CASE
			WHEN duration_sec < 5 THEN 1
			WHEN duration_sec < 10 THEN 2
			WHEN duration_sec < 30 THEN 3
			WHEN duration_sec < 60 THEN 4
			WHEN duration_sec < 180 THEN 5
			WHEN duration_sec < 600 THEN 6
			ELSE 7
		END AS sort_order
	FROM probe
	WHERE duration_sec IS NOT NULL
) t
GROUP BY bucket
ORDER BY sort_order ASC, bucket ASC
`, since).Scan(&durationRows).Error; err != nil {
		return 0, nil, nil, nil, err
	}

	var resolutionRows []bucketRow
	if err := h.db.Raw(baseCTE+`
SELECT
	bucket AS key,
	COUNT(*)::bigint AS count,
	MIN(sort_order)::int AS sort_order
FROM (
	SELECT
		CASE
			WHEN width >= 3840 OR height >= 2160 THEN '4k+'
			WHEN width >= 2560 OR height >= 1440 THEN '2k'
			WHEN width >= 1920 OR height >= 1080 THEN '1080p'
			WHEN width >= 1280 OR height >= 720 THEN '720p'
			WHEN width >= 854 OR height >= 480 THEN '480p'
			ELSE '<480p'
		END AS bucket,
		CASE
			WHEN width >= 3840 OR height >= 2160 THEN 1
			WHEN width >= 2560 OR height >= 1440 THEN 2
			WHEN width >= 1920 OR height >= 1080 THEN 3
			WHEN width >= 1280 OR height >= 720 THEN 4
			WHEN width >= 854 OR height >= 480 THEN 5
			ELSE 6
		END AS sort_order
	FROM probe
	WHERE width IS NOT NULL
		AND height IS NOT NULL
		AND width > 0
		AND height > 0
) t
GROUP BY bucket
ORDER BY sort_order ASC, bucket ASC
`, since).Scan(&resolutionRows).Error; err != nil {
		return 0, nil, nil, nil, err
	}

	var fpsRows []bucketRow
	if err := h.db.Raw(baseCTE+`
SELECT
	bucket AS key,
	COUNT(*)::bigint AS count,
	MIN(sort_order)::int AS sort_order
FROM (
	SELECT
		CASE
			WHEN fps < 15 THEN '<15fps'
			WHEN fps < 24 THEN '15-24fps'
			WHEN fps < 30 THEN '24-30fps'
			WHEN fps < 60 THEN '30-60fps'
			ELSE '60fps+'
		END AS bucket,
		CASE
			WHEN fps < 15 THEN 1
			WHEN fps < 24 THEN 2
			WHEN fps < 30 THEN 3
			WHEN fps < 60 THEN 4
			ELSE 5
		END AS sort_order
	FROM probe
	WHERE fps IS NOT NULL
) t
GROUP BY bucket
ORDER BY sort_order ASC, bucket ASC
`, since).Scan(&fpsRows).Error; err != nil {
		return 0, nil, nil, nil, err
	}

	return jobsWindowRow.Count, toCounts(durationRows), toCounts(resolutionRows), toCounts(fpsRows), nil
}

func sourceProbeBucketSQL(dimension string) (bucketExpr string, sortExpr string, filterExpr string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(dimension)) {
	case "duration":
		return `
CASE
	WHEN duration_sec < 5 THEN '<5s'
	WHEN duration_sec < 10 THEN '5-10s'
	WHEN duration_sec < 30 THEN '10-30s'
	WHEN duration_sec < 60 THEN '30-60s'
	WHEN duration_sec < 180 THEN '1-3m'
	WHEN duration_sec < 600 THEN '3-10m'
	ELSE '10m+'
END
`, `
CASE
	WHEN duration_sec < 5 THEN 1
	WHEN duration_sec < 10 THEN 2
	WHEN duration_sec < 30 THEN 3
	WHEN duration_sec < 60 THEN 4
	WHEN duration_sec < 180 THEN 5
	WHEN duration_sec < 600 THEN 6
	ELSE 7
END
`, "duration_sec IS NOT NULL", true
	case "resolution":
		return `
CASE
	WHEN width >= 3840 OR height >= 2160 THEN '4k+'
	WHEN width >= 2560 OR height >= 1440 THEN '2k'
	WHEN width >= 1920 OR height >= 1080 THEN '1080p'
	WHEN width >= 1280 OR height >= 720 THEN '720p'
	WHEN width >= 854 OR height >= 480 THEN '480p'
	ELSE '<480p'
END
`, `
CASE
	WHEN width >= 3840 OR height >= 2160 THEN 1
	WHEN width >= 2560 OR height >= 1440 THEN 2
	WHEN width >= 1920 OR height >= 1080 THEN 3
	WHEN width >= 1280 OR height >= 720 THEN 4
	WHEN width >= 854 OR height >= 480 THEN 5
	ELSE 6
END
`, "width IS NOT NULL AND height IS NOT NULL AND width > 0 AND height > 0", true
	case "fps":
		return `
CASE
	WHEN fps < 15 THEN '<15fps'
	WHEN fps < 24 THEN '15-24fps'
	WHEN fps < 30 THEN '24-30fps'
	WHEN fps < 60 THEN '30-60fps'
	ELSE '60fps+'
END
`, `
CASE
	WHEN fps < 15 THEN 1
	WHEN fps < 24 THEN 2
	WHEN fps < 30 THEN 3
	WHEN fps < 60 THEN 4
	ELSE 5
END
`, "fps IS NOT NULL", true
	default:
		return "", "", "", false
	}
}

func (h *Handler) loadVideoJobSourceProbeQualityStats(since time.Time) ([]AdminVideoJobSourceProbeQualityStat, []AdminVideoJobSourceProbeQualityStat, []AdminVideoJobSourceProbeQualityStat, error) {
	durationStats, err := h.loadVideoJobSourceProbeQualityStatsByDimension(since, "duration")
	if err != nil {
		return nil, nil, nil, err
	}
	resolutionStats, err := h.loadVideoJobSourceProbeQualityStatsByDimension(since, "resolution")
	if err != nil {
		return nil, nil, nil, err
	}
	fpsStats, err := h.loadVideoJobSourceProbeQualityStatsByDimension(since, "fps")
	if err != nil {
		return nil, nil, nil, err
	}
	return durationStats, resolutionStats, fpsStats, nil
}

func (h *Handler) loadVideoJobSourceProbeQualityStatsByDimension(since time.Time, dimension string) ([]AdminVideoJobSourceProbeQualityStat, error) {
	bucketExpr, sortExpr, filterExpr, ok := sourceProbeBucketSQL(dimension)
	if !ok {
		return nil, fmt.Errorf("unsupported source probe dimension: %s", dimension)
	}

	type row struct {
		Bucket         string   `gorm:"column:bucket"`
		Jobs           int64    `gorm:"column:jobs"`
		DoneJobs       int64    `gorm:"column:done_jobs"`
		FailedJobs     int64    `gorm:"column:failed_jobs"`
		CancelledJobs  int64    `gorm:"column:cancelled_jobs"`
		PendingJobs    int64    `gorm:"column:pending_jobs"`
		DurationP50Sec *float64 `gorm:"column:duration_p50_sec"`
		DurationP95Sec *float64 `gorm:"column:duration_p95_sec"`
	}

	query := fmt.Sprintf(`
WITH probe AS (
	SELECT
		COALESCE(LOWER(TRIM(v.status)), '') AS status,
		v.started_at,
		v.finished_at,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'duration_sec') = 'number'
			THEN (v.options->'source_video_probe'->>'duration_sec')::double precision
			ELSE NULL
		END AS duration_sec,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'width') = 'number'
			THEN (v.options->'source_video_probe'->>'width')::double precision
			ELSE NULL
		END AS width,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'height') = 'number'
			THEN (v.options->'source_video_probe'->>'height')::double precision
			ELSE NULL
		END AS height,
		CASE
			WHEN jsonb_typeof(v.options->'source_video_probe') = 'object'
				AND jsonb_typeof(v.options->'source_video_probe'->'fps') = 'number'
			THEN (v.options->'source_video_probe'->>'fps')::double precision
			ELSE NULL
		END AS fps
	FROM archive.video_jobs v
	WHERE v.created_at >= ?
		AND jsonb_typeof(v.options->'source_video_probe') = 'object'
)
SELECT
	bucket,
	COUNT(*)::bigint AS jobs,
	COUNT(*) FILTER (WHERE status = ?)::bigint AS done_jobs,
	COUNT(*) FILTER (WHERE status = ?)::bigint AS failed_jobs,
	COUNT(*) FILTER (WHERE status = ?)::bigint AS cancelled_jobs,
	COUNT(*) FILTER (WHERE status NOT IN (?, ?, ?))::bigint AS pending_jobs,
	percentile_cont(0.50) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at)))
		FILTER (WHERE status = ? AND started_at IS NOT NULL AND finished_at IS NOT NULL AND finished_at > started_at) AS duration_p50_sec,
	percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at)))
		FILTER (WHERE status = ? AND started_at IS NOT NULL AND finished_at IS NOT NULL AND finished_at > started_at) AS duration_p95_sec,
	MIN(sort_order)::int AS sort_order
FROM (
	SELECT
		%s AS bucket,
		%s AS sort_order,
		status,
		started_at,
		finished_at
	FROM probe
	WHERE %s
) b
GROUP BY bucket
ORDER BY sort_order ASC, bucket ASC
`, bucketExpr, sortExpr, filterExpr)

	var rows []row
	if err := h.db.Raw(
		query,
		since,
		models.VideoJobStatusDone,
		models.VideoJobStatusFailed,
		models.VideoJobStatusCancelled,
		models.VideoJobStatusDone,
		models.VideoJobStatusFailed,
		models.VideoJobStatusCancelled,
		models.VideoJobStatusDone,
		models.VideoJobStatusDone,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]AdminVideoJobSourceProbeQualityStat, 0, len(rows))
	for _, item := range rows {
		bucket := strings.TrimSpace(item.Bucket)
		if bucket == "" {
			continue
		}
		stat := AdminVideoJobSourceProbeQualityStat{
			Bucket:        bucket,
			Jobs:          item.Jobs,
			DoneJobs:      item.DoneJobs,
			FailedJobs:    item.FailedJobs,
			PendingJobs:   item.PendingJobs,
			CancelledJobs: item.CancelledJobs,
		}
		terminal := stat.DoneJobs + stat.FailedJobs
		stat.TerminalJobs = terminal
		if terminal > 0 {
			stat.SuccessRate = float64(stat.DoneJobs) / float64(terminal)
			stat.FailureRate = float64(stat.FailedJobs) / float64(terminal)
		}
		if item.DurationP50Sec != nil {
			stat.DurationP50Sec = *item.DurationP50Sec
		}
		if item.DurationP95Sec != nil {
			stat.DurationP95Sec = *item.DurationP95Sec
		}
		out = append(out, stat)
	}

	return out, nil
}
