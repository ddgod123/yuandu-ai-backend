package handlers

import (
	"sort"
	"time"

	"emoji/internal/models"
)

func (h *Handler) loadVideoJobFormatStats24h(since time.Time) ([]AdminVideoJobFormatStat, error) {
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
	type feedbackRow struct {
		Format             string   `gorm:"column:format"`
		EngagedJobs        int64    `gorm:"column:engaged_jobs"`
		FeedbackSignals    int64    `gorm:"column:feedback_signals"`
		AvgEngagementScore *float64 `gorm:"column:avg_engagement_score"`
	}
	type sizeProfileRow struct {
		Format          string `gorm:"column:format"`
		SizeProfileJobs int64  `gorm:"column:size_profile_jobs"`
	}
	type sizeProfileArtifactRow struct {
		Format               string   `gorm:"column:format"`
		AvgArtifactSizeBytes *float64 `gorm:"column:avg_artifact_size_bytes"`
	}
	type sizeBudgetHitRow struct {
		Format            string `gorm:"column:format"`
		SizeBudgetSamples int64  `gorm:"column:size_budget_samples"`
		SizeBudgetHits    int64  `gorm:"column:size_budget_hits"`
	}

	var requestedRows []requestedRow
	if err := h.db.Raw(`
WITH requested AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE fmt.value
		END AS format
	FROM archive.video_jobs j
	CROSS JOIN LATERAL unnest(
		string_to_array(replace(lower(COALESCE(j.output_formats, '')), ' ', ''), ',')
	) AS fmt(value)
	WHERE j.created_at >= ?
		AND trim(fmt.value) <> ''
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
WITH generated AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE lower(fmt.value)
		END AS format
	FROM archive.video_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'output_formats') = 'array' THEN j.metrics->'output_formats'
			ELSE '[]'::jsonb
		END
	) AS fmt(value)
	WHERE j.created_at >= ?
		AND trim(fmt.value) <> ''
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
WITH output_artifacts AS (
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
		JOIN archive.video_jobs j ON j.id = a.job_id
		WHERE j.created_at >= ?
			AND a.type IN ('frame', 'clip', 'live_package')
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

	var feedbackRows []feedbackRow
	if err := h.db.Raw(`
WITH generated AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE lower(fmt.value)
		END AS format,
		COALESCE((j.metrics->'feedback_v1'->>'total_signals')::bigint, 0) AS total_signals,
		COALESCE((j.metrics->'feedback_v1'->>'engagement_score')::double precision, 0) AS engagement_score
	FROM archive.video_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'output_formats') = 'array' THEN j.metrics->'output_formats'
			ELSE '[]'::jsonb
		END
	) AS fmt(value)
	WHERE j.created_at >= ?
		AND j.status = ?
		AND trim(fmt.value) <> ''
)
SELECT
	format,
	COUNT(*) FILTER (WHERE total_signals > 0)::bigint AS engaged_jobs,
	COALESCE(SUM(total_signals), 0)::bigint AS feedback_signals,
	AVG(engagement_score) FILTER (WHERE total_signals > 0) AS avg_engagement_score
FROM generated
GROUP BY format
`, since, models.VideoJobStatusDone).Scan(&feedbackRows).Error; err != nil {
		return nil, err
	}

	var sizeProfileRows []sizeProfileRow
	if err := h.db.Raw(`
WITH generated AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE lower(fmt.value)
		END AS format
	FROM archive.video_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'output_formats') = 'array' THEN j.metrics->'output_formats'
			ELSE '[]'::jsonb
		END
	) AS fmt(value)
	WHERE j.created_at >= ?
		AND j.status = ?
		AND trim(fmt.value) <> ''
)
SELECT
	g.format AS format,
	COUNT(*)::bigint AS size_profile_jobs
FROM generated g
JOIN archive.video_jobs j ON j.id = g.job_id
WHERE
	(g.format = 'jpg' AND lower(COALESCE(j.metrics->'quality_settings'->>'jpg_profile', '')) = 'size')
	OR
	(g.format = 'png' AND lower(COALESCE(j.metrics->'quality_settings'->>'png_profile', '')) = 'size')
GROUP BY g.format
`, since, models.VideoJobStatusDone).Scan(&sizeProfileRows).Error; err != nil {
		return nil, err
	}

	var sizeProfileArtifactRows []sizeProfileArtifactRow
	if err := h.db.Raw(`
WITH generated AS (
	SELECT DISTINCT
		j.id AS job_id,
		CASE
			WHEN fmt.value = 'jpeg' THEN 'jpg'
			ELSE lower(fmt.value)
		END AS format
	FROM archive.video_jobs j
	CROSS JOIN LATERAL jsonb_array_elements_text(
		CASE
			WHEN jsonb_typeof(j.metrics->'output_formats') = 'array' THEN j.metrics->'output_formats'
			ELSE '[]'::jsonb
		END
	) AS fmt(value)
	WHERE j.created_at >= ?
		AND j.status = ?
		AND trim(fmt.value) <> ''
),
size_profile_jobs AS (
	SELECT
		g.job_id,
		g.format
	FROM generated g
	JOIN archive.video_jobs j ON j.id = g.job_id
	WHERE
		(g.format = 'jpg' AND lower(COALESCE(j.metrics->'quality_settings'->>'jpg_profile', '')) = 'size')
		OR
		(g.format = 'png' AND lower(COALESCE(j.metrics->'quality_settings'->>'png_profile', '')) = 'size')
),
still_artifacts AS (
	SELECT
		t.job_id,
		CASE
			WHEN raw_format = 'jpeg' THEN 'jpg'
			ELSE raw_format
		END AS format,
		t.size_bytes
	FROM (
		SELECT
			a.job_id,
			lower(COALESCE(NULLIF(a.metadata->>'format', ''), '')) AS raw_format,
			a.size_bytes
		FROM archive.video_job_artifacts a
		JOIN archive.video_jobs j ON j.id = a.job_id
		WHERE j.created_at >= ?
			AND j.status = ?
			AND a.type = 'frame'
	) t
	WHERE raw_format <> ''
)
SELECT
	a.format,
	AVG(a.size_bytes)::double precision AS avg_artifact_size_bytes
FROM still_artifacts a
JOIN size_profile_jobs s ON s.job_id = a.job_id AND s.format = a.format
GROUP BY a.format
	`, since, models.VideoJobStatusDone, since, models.VideoJobStatusDone).Scan(&sizeProfileArtifactRows).Error; err != nil {
		return nil, err
	}

	var sizeBudgetHitRows []sizeBudgetHitRow
	if err := h.db.Raw(`
WITH still_artifacts AS (
	SELECT
		a.job_id,
		CASE
			WHEN raw_format = 'jpeg' THEN 'jpg'
			ELSE raw_format
		END AS format,
		a.size_bytes,
		lower(COALESCE(j.metrics->'quality_settings'->>'jpg_profile', '')) AS jpg_profile,
		lower(COALESCE(j.metrics->'quality_settings'->>'png_profile', '')) AS png_profile,
		COALESCE((j.metrics->'quality_settings'->>'jpg_target_size_kb')::bigint, 0) AS jpg_target_size_kb,
		COALESCE((j.metrics->'quality_settings'->>'png_target_size_kb')::bigint, 0) AS png_target_size_kb
	FROM (
		SELECT
			a.job_id,
			lower(COALESCE(NULLIF(a.metadata->>'format', ''), '')) AS raw_format,
			a.size_bytes
		FROM archive.video_job_artifacts a
		JOIN archive.video_jobs j ON j.id = a.job_id
		WHERE j.created_at >= ?
			AND j.status = ?
			AND a.type = 'frame'
	) a
	JOIN archive.video_jobs j ON j.id = a.job_id
	WHERE raw_format IN ('jpg', 'jpeg', 'png')
),
budget_targets AS (
	SELECT
		format,
		size_bytes,
		CASE
			WHEN format = 'jpg' AND jpg_profile = 'size' AND jpg_target_size_kb > 0
				THEN jpg_target_size_kb * 1024
			WHEN format = 'png' AND png_profile = 'size' AND png_target_size_kb > 0
				THEN png_target_size_kb * 1024
			ELSE 0
		END AS target_bytes
	FROM still_artifacts
)
SELECT
	format,
	COUNT(*) FILTER (WHERE target_bytes > 0)::bigint AS size_budget_samples,
	COUNT(*) FILTER (WHERE target_bytes > 0 AND size_bytes <= target_bytes)::bigint AS size_budget_hits
FROM budget_targets
GROUP BY format
`, since, models.VideoJobStatusDone).Scan(&sizeBudgetHitRows).Error; err != nil {
		return nil, err
	}

	statsMap := make(map[string]*AdminVideoJobFormatStat)
	for _, item := range requestedRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.RequestedJobs += item.RequestedJobs
	}
	for _, item := range generatedRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.GeneratedJobs += item.GeneratedJobs
	}
	for _, item := range artifactRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.ArtifactCount += item.ArtifactCount
		if item.AvgArtifactSizeBytes != nil {
			stat.AvgArtifactSizeBytes = *item.AvgArtifactSizeBytes
		}
	}
	for _, item := range feedbackRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.EngagedJobs += item.EngagedJobs
		stat.FeedbackSignals += item.FeedbackSignals
		if item.AvgEngagementScore != nil {
			stat.AvgEngagementScore = *item.AvgEngagementScore
		}
	}
	for _, item := range sizeProfileRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.SizeProfileJobs += item.SizeProfileJobs
	}
	for _, item := range sizeProfileArtifactRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		if item.AvgArtifactSizeBytes != nil {
			stat.SizeProfileAvgBytes = *item.AvgArtifactSizeBytes
		}
	}
	for _, item := range sizeBudgetHitRows {
		format := normalizeVideoJobFormat(item.Format)
		if format == "" {
			continue
		}
		stat, ok := statsMap[format]
		if !ok {
			stat = &AdminVideoJobFormatStat{Format: format}
			statsMap[format] = stat
		}
		stat.SizeBudgetSamples += item.SizeBudgetSamples
		stat.SizeBudgetHits += item.SizeBudgetHits
	}

	stats := make([]AdminVideoJobFormatStat, 0, len(statsMap))
	for _, item := range statsMap {
		if item.RequestedJobs > 0 {
			item.SuccessRate = float64(item.GeneratedJobs) / float64(item.RequestedJobs)
		}
		if item.GeneratedJobs > 0 {
			item.SizeProfileRate = float64(item.SizeProfileJobs) / float64(item.GeneratedJobs)
		}
		if item.SizeBudgetSamples > 0 {
			item.SizeBudgetHitRate = float64(item.SizeBudgetHits) / float64(item.SizeBudgetSamples)
		}
		stats = append(stats, *item)
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].RequestedJobs != stats[j].RequestedJobs {
			return stats[i].RequestedJobs > stats[j].RequestedJobs
		}
		if stats[i].GeneratedJobs != stats[j].GeneratedJobs {
			return stats[i].GeneratedJobs > stats[j].GeneratedJobs
		}
		return stats[i].Format < stats[j].Format
	})
	return stats, nil
}
