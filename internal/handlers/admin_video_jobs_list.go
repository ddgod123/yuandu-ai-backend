package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
)

// ListAdminVideoJobs godoc
// @Summary List video jobs (admin)
// @Tags admin
// @Produce json
// @Param page query int false "page"
// @Param page_size query int false "page_size"
// @Param user_id query int false "user id"
// @Param status query string false "job status"
// @Param format query string false "requested format"
// @Param asset_domain query string false "asset domain: all|video|archive|admin|ugc"
// @Param guard_reason query string false "negative guard reason filter"
// @Param source_read_reason query string false "source readability reason_code filter"
// @Param reason_code query string false "alias of source_read_reason"
// @Param stage query string false "job stage"
// @Param audit_signal query string false "audit signal filter: proposal | deliver | feedback | rerender"
// @Param quick query string false "quick filter: retrying | failed_24h | guard_hit | guard_blocked | feedback_anomaly | top_pick_conflict | sub_stage_anomaly | sub_stage_briefing_anomaly | sub_stage_planning_anomaly | sub_stage_scoring_anomaly | sub_stage_reviewing_anomaly"
// @Param is_sample query string false "sample filter: all | 1 | 0"
// @Param q query string false "title/source search"
// @Success 200 {object} AdminVideoJobListResponse
// @Router /api/admin/video-jobs [get]
func (h *Handler) ListAdminVideoJobs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := h.db.Model(&models.VideoJob{})
	if quick := strings.ToLower(strings.TrimSpace(c.Query("quick"))); quick != "" {
		quickSince := time.Now().Add(-24 * time.Hour)
		switch quick {
		case "retrying":
			query = query.Where("stage = ?", models.VideoJobStageRetrying)
		case "failed_24h":
			query = query.Where("status = ? AND updated_at >= ?", models.VideoJobStatusFailed, time.Now().Add(-24*time.Hour))
		case "guard_hit":
			query = query.Where(`
status = ?
AND LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->>'group'), ''), '')) = 'treatment'
AND jsonb_typeof(metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
AND (metrics->'highlight_feedback_v1'->'reason_negative_guard') <> '{}'::jsonb
`, models.VideoJobStatusDone)
		case "guard_blocked":
			query = query.Where(`
status = ?
AND LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->>'group'), ''), '')) = 'treatment'
AND jsonb_typeof(metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
AND (metrics->'highlight_feedback_v1'->'reason_negative_guard') <> '{}'::jsonb
AND LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), '')) <> ''
AND LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->'selected_after'->>'reason'), ''), '')) <> ''
AND LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->'selected_after'->>'reason'), ''), '')) <>
	LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), ''))
AND (metrics->'highlight_feedback_v1'->'reason_negative_guard' ? LOWER(COALESCE(NULLIF(TRIM(metrics->'highlight_feedback_v1'->'selected_before'->>'reason'), ''), '')))
`, models.VideoJobStatusDone)
		case "feedback_anomaly":
			query = query.Where(`
EXISTS (
	SELECT 1
	FROM public.video_image_feedback f
	LEFT JOIN public.video_image_outputs o ON o.id = f.output_id
	WHERE f.job_id = video_jobs.id
		AND f.created_at >= ?
		AND (
			f.output_id IS NULL
			OR (f.output_id IS NOT NULL AND o.id IS NULL)
			OR (f.output_id IS NOT NULL AND o.id IS NOT NULL AND o.job_id <> f.job_id)
		)
)
`, quickSince)
		case "top_pick_conflict":
			query = query.Where(`
EXISTS (
	SELECT 1
	FROM public.video_image_feedback f
	WHERE f.job_id = video_jobs.id
		AND f.created_at >= ?
		AND LOWER(COALESCE(NULLIF(TRIM(f.action), ''), 'unknown')) = 'top_pick'
	GROUP BY f.job_id, f.user_id
	HAVING COUNT(*) > 1
)
`, quickSince)
		case "sub_stage_anomaly":
			query = query.Where("created_at >= ?", quickSince).
				Where(buildVideoJobAnyGIFSubStageAnomalyPredicate())
		case "sub_stage_briefing_anomaly":
			query = query.Where("created_at >= ?", quickSince).
				Where(buildVideoJobGIFSubStageAnomalyPredicate(), videoJobGIFSubStageBriefing)
		case "sub_stage_planning_anomaly":
			query = query.Where("created_at >= ?", quickSince).
				Where(buildVideoJobGIFSubStageAnomalyPredicate(), videoJobGIFSubStagePlanning)
		case "sub_stage_scoring_anomaly":
			query = query.Where("created_at >= ?", quickSince).
				Where(buildVideoJobGIFSubStageAnomalyPredicate(), videoJobGIFSubStageScoring)
		case "sub_stage_reviewing_anomaly":
			query = query.Where("created_at >= ?", quickSince).
				Where(buildVideoJobGIFSubStageAnomalyPredicate(), videoJobGIFSubStageReviewing)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid quick"})
			return
		}
	}
	if userIDRaw := strings.TrimSpace(c.Query("user_id")); userIDRaw != "" {
		userID, err := strconv.ParseUint(userIDRaw, 10, 64)
		if err != nil || userID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
			return
		}
		query = query.Where("user_id = ?", userID)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if format := strings.ToLower(strings.TrimSpace(c.Query("format"))); format != "" {
		query = query.Where(buildVideoJobFormatFilterPredicate("video_jobs"), format, format)
	}
	if assetDomain := strings.ToLower(strings.TrimSpace(c.Query("asset_domain"))); assetDomain != "" && assetDomain != "all" {
		switch assetDomain {
		case models.VideoJobAssetDomainVideo, models.VideoJobAssetDomainArchive, models.VideoJobAssetDomainAdmin, models.VideoJobAssetDomainUGC:
			query = query.Where("LOWER(COALESCE(NULLIF(TRIM(asset_domain), ''), 'video')) = ?", assetDomain)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid asset_domain"})
			return
		}
	}
	if guardReason := strings.ToLower(strings.TrimSpace(c.Query("guard_reason"))); guardReason != "" {
		query = query.Where(`
EXISTS (
	SELECT 1
	FROM jsonb_each_text(
		CASE
			WHEN jsonb_typeof(metrics->'highlight_feedback_v1'->'reason_negative_guard') = 'object'
			THEN metrics->'highlight_feedback_v1'->'reason_negative_guard'
			ELSE '{}'::jsonb
		END
	) AS guard_reason(key, value)
	WHERE LOWER(TRIM(guard_reason.key)) = ?
	)`, guardReason)
	}
	readabilityReason := strings.ToLower(strings.TrimSpace(c.Query("source_read_reason")))
	if readabilityReason == "" {
		readabilityReason = strings.ToLower(strings.TrimSpace(c.Query("reason_code")))
	}
	if readabilityReason != "" {
		query = query.Where(`
LOWER(COALESCE(NULLIF(TRIM(metrics->'source_video_readability_v1'->>'reason_code'), ''), '')) = ?
OR LOWER(COALESCE(NULLIF(TRIM(options->'source_video_probe_degraded_v1'->>'reason_code'), ''), '')) = ?
OR LOWER(COALESCE(NULLIF(TRIM(options->'source_video_probe'->>'reason_code'), ''), '')) = ?
`, readabilityReason, readabilityReason, readabilityReason)
	}
	if stage := strings.TrimSpace(c.Query("stage")); stage != "" {
		query = query.Where("stage = ?", stage)
	}
	if auditSignal := strings.ToLower(strings.TrimSpace(c.Query("audit_signal"))); auditSignal != "" && auditSignal != "all" {
		switch auditSignal {
		case "proposal":
			query = query.Where(`
EXISTS (
	SELECT 1
	FROM archive.video_job_gif_ai_proposals p
	WHERE p.job_id = video_jobs.id
)`)
		case "deliver":
			query = query.Where(`
EXISTS (
	SELECT 1
	FROM archive.video_job_gif_ai_reviews r
	WHERE r.job_id = video_jobs.id
	  AND LOWER(COALESCE(NULLIF(TRIM(r.final_recommendation), ''), '')) = 'deliver'
)`)
		case "feedback":
			query = query.Where(`
EXISTS (
	SELECT 1
	FROM public.video_image_feedback f
	WHERE f.job_id = video_jobs.id
)`)
		case "rerender":
			query = query.Where(`
EXISTS (
	SELECT 1
	FROM archive.video_job_gif_ai_reviews r
	WHERE r.job_id = video_jobs.id
	  AND (
		LOWER(COALESCE(NULLIF(TRIM(r.model), ''), '')) = 'admin_rerender_v1'
		OR LOWER(COALESCE(NULLIF(TRIM(r.metadata->>'rerender'), ''), 'false')) IN ('1', 'true', 'yes', 'y')
	  )
)`)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid audit_signal"})
			return
		}
	}
	sampleRaw := strings.TrimSpace(c.Query("is_sample"))
	if sampleRaw == "" {
		sampleRaw = strings.TrimSpace(c.Query("sample"))
	}
	sampleFilter, ok := parseOptionalBoolParam(sampleRaw)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid is_sample"})
		return
	}
	if sampleFilter != nil {
		if *sampleFilter {
			query = query.Where(`
LOWER(COALESCE(NULLIF(TRIM(asset_domain), ''), 'video')) <> 'video'
AND EXISTS (
	SELECT 1
	FROM archive.collections c
	WHERE c.id = video_jobs.result_collection_id
	  AND c.is_sample = TRUE
)`)
		} else {
			query = query.Where(`
NOT (
	LOWER(COALESCE(NULLIF(TRIM(asset_domain), ''), 'video')) <> 'video'
	AND EXISTS (
	SELECT 1
	FROM archive.collections c
	WHERE c.id = video_jobs.result_collection_id
	  AND c.is_sample = TRUE
))`)
		}
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		like := "%" + q + "%"
		query = query.Where("title ILIKE ? OR source_video_key ILIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []models.VideoJob
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	jobs := make([]models.VideoJob, 0, len(rows))
	jobs = append(jobs, rows...)

	userMap := h.loadVideoJobUserMap(jobs)
	collectionMap := h.loadVideoJobCollectionMap(jobs)
	costMap := h.loadVideoJobCostMap(jobs)
	pointHoldMap := h.loadVideoJobPointHoldMap(jobs)
	auditSummaryMap := h.loadVideoJobAuditSummaryMap(jobs)

	items := make([]AdminVideoJobListItem, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, h.buildAdminVideoJobListItem(job, userMap, collectionMap, costMap, pointHoldMap, auditSummaryMap))
	}

	c.JSON(http.StatusOK, AdminVideoJobListResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}
