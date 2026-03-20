package handlers

import (
	"sort"
	"time"

	"emoji/internal/models"
)

func (h *Handler) loadSampleVideoJobFormatBaselineRows(since time.Time) ([]adminSampleVideoJobFormatBaselineRow, error) {
	type requestedRow struct {
		Format        string `gorm:"column:format"`
		RequestedJobs int64  `gorm:"column:requested_jobs"`
	}
	type generatedRow struct {
		Format        string `gorm:"column:format"`
		GeneratedJobs int64  `gorm:"column:generated_jobs"`
	}
	type artifactRow struct {
		Format               string   `gorm:"column:format"`
		ArtifactCount        int64    `gorm:"column:artifact_count"`
		AvgArtifactSizeBytes *float64 `gorm:"column:avg_artifact_size_bytes"`
	}
	type durationRow struct {
		Format string   `gorm:"column:format"`
		P50Sec *float64 `gorm:"column:p50_sec"`
		P95Sec *float64 `gorm:"column:p95_sec"`
	}

	var requestedRows []requestedRow
	if err := h.db.Raw(`
WITH sample_jobs AS (
	SELECT v.id, v.output_formats
	FROM archive.video_jobs v
	JOIN archive.collections c ON c.id = v.result_collection_id
	WHERE c.is_sample = TRUE
		AND v.created_at >= ?
),
requested AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE fmt.value
		END AS format
	FROM sample_jobs j
	CROSS JOIN LATERAL unnest(
		string_to_array(replace(lower(COALESCE(j.output_formats, '')), ' ', ''), ',')
	) AS fmt(value)
	WHERE trim(fmt.value) <> ''
)
SELECT
	format,
	COUNT(*)::bigint AS requested_jobs
FROM requested
GROUP BY format
`, since).Scan(&requestedRows).Error; err != nil {
		return nil, err
	}

	var generatedRows []generatedRow
	if err := h.db.Raw(`
WITH sample_jobs AS (
	SELECT v.id, v.metrics
	FROM archive.video_jobs v
	JOIN archive.collections c ON c.id = v.result_collection_id
	WHERE c.is_sample = TRUE
		AND v.created_at >= ?
),
generated AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE lower(fmt.value)
		END AS format
	FROM sample_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'output_formats') = 'array' THEN j.metrics->'output_formats'
			ELSE '[]'::jsonb
		END
	) AS fmt(value)
	WHERE trim(fmt.value) <> ''
)
SELECT
	format,
	COUNT(*)::bigint AS generated_jobs
FROM generated
GROUP BY format
`, since).Scan(&generatedRows).Error; err != nil {
		return nil, err
	}

	var artifactRows []artifactRow
	if err := h.db.Raw(`
WITH sample_jobs AS (
	SELECT v.id
	FROM archive.video_jobs v
	JOIN archive.collections c ON c.id = v.result_collection_id
	WHERE c.is_sample = TRUE
		AND v.created_at >= ?
),
output_artifacts AS (
	SELECT
		CASE
			WHEN raw_format = 'jpeg' THEN 'jpg'
			ELSE raw_format
		END AS format,
		size_bytes
	FROM (
		SELECT
			lower(COALESCE(NULLIF(a.metadata->>'format', ''), '')) AS raw_format,
			a.size_bytes
		FROM archive.video_job_artifacts a
		JOIN sample_jobs s ON s.id = a.job_id
		WHERE a.type IN ('frame', 'clip', 'live_package')
	) t
	WHERE raw_format <> ''
)
SELECT
	format,
	COUNT(*)::bigint AS artifact_count,
	AVG(size_bytes)::double precision AS avg_artifact_size_bytes
FROM output_artifacts
GROUP BY format
`, since).Scan(&artifactRows).Error; err != nil {
		return nil, err
	}

	var durationRows []durationRow
	if err := h.db.Raw(`
WITH sample_jobs AS (
	SELECT v.id, v.output_formats, v.status, v.started_at, v.finished_at
	FROM archive.video_jobs v
	JOIN archive.collections c ON c.id = v.result_collection_id
	WHERE c.is_sample = TRUE
		AND v.created_at >= ?
),
requested AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE fmt.value
		END AS format
	FROM sample_jobs j
	CROSS JOIN LATERAL unnest(
		string_to_array(replace(lower(COALESCE(j.output_formats, '')), ' ', ''), ',')
	) AS fmt(value)
	WHERE trim(fmt.value) <> ''
),
durations AS (
	SELECT
		r.format,
		EXTRACT(EPOCH FROM (j.finished_at - j.started_at)) AS duration_sec
	FROM requested r
	JOIN sample_jobs j ON j.id = r.job_id
	WHERE j.status = ?
		AND j.started_at IS NOT NULL
		AND j.finished_at IS NOT NULL
)
SELECT
	format,
	percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_sec) AS p50_sec,
	percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_sec) AS p95_sec
FROM durations
GROUP BY format
`, since, models.VideoJobStatusDone).Scan(&durationRows).Error; err != nil {
		return nil, err
	}

	statsMap := make(map[string]*adminSampleVideoJobFormatBaselineRow)
	ensure := func(raw string) *adminSampleVideoJobFormatBaselineRow {
		format := normalizeVideoJobFormat(raw)
		if format == "" {
			return nil
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &adminSampleVideoJobFormatBaselineRow{Format: format}
			statsMap[format] = stat
		}
		return stat
	}
	for _, item := range requestedRows {
		stat := ensure(item.Format)
		if stat == nil {
			continue
		}
		stat.RequestedJobs += item.RequestedJobs
	}
	for _, item := range generatedRows {
		stat := ensure(item.Format)
		if stat == nil {
			continue
		}
		stat.GeneratedJobs += item.GeneratedJobs
	}
	for _, item := range artifactRows {
		stat := ensure(item.Format)
		if stat == nil {
			continue
		}
		stat.ArtifactCount += item.ArtifactCount
		if item.AvgArtifactSizeBytes != nil {
			stat.AvgArtifactSizeBytes = *item.AvgArtifactSizeBytes
		}
	}
	for _, item := range durationRows {
		stat := ensure(item.Format)
		if stat == nil {
			continue
		}
		if item.P50Sec != nil {
			stat.DurationP50Sec = *item.P50Sec
		}
		if item.P95Sec != nil {
			stat.DurationP95Sec = *item.P95Sec
		}
	}

	rows := make([]adminSampleVideoJobFormatBaselineRow, 0, len(statsMap))
	for _, item := range statsMap {
		if item.RequestedJobs > 0 {
			item.SuccessRate = float64(item.GeneratedJobs) / float64(item.RequestedJobs)
		}
		rows = append(rows, *item)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].RequestedJobs != rows[j].RequestedJobs {
			return rows[i].RequestedJobs > rows[j].RequestedJobs
		}
		if rows[i].GeneratedJobs != rows[j].GeneratedJobs {
			return rows[i].GeneratedJobs > rows[j].GeneratedJobs
		}
		return rows[i].Format < rows[j].Format
	})
	return rows, nil
}
