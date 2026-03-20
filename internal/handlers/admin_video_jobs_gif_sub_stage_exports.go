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

func (h *Handler) ExportAdminVideoJobsGIFSubStageAnomaliesCSV(c *gin.Context) {
	windowLabel, windowDuration, err := parseVideoJobsOverviewWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	subStage, err := parseVideoJobGIFSubStageFilter(c.Query("sub_stage"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	subStatus, err := parseVideoJobGIFSubStageStatusFilter(c.Query("sub_status"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	limit := 500
	if limitRaw := strings.TrimSpace(c.Query("limit")); limitRaw != "" {
		parsed, perr := strconv.Atoi(limitRaw)
		if perr != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		if parsed > 2000 {
			parsed = 2000
		}
		limit = parsed
	}

	now := time.Now()
	since := now.Add(-windowDuration)
	rows, totalRows, totalJobs, err := h.loadVideoJobGIFSubStageAnomalyExportRows(since, subStage, subStatus, limit)
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
		"sub_stage",
		"sub_stage_label",
		"sub_status",
		"total_rows",
		"total_jobs",
		"limit",
		"truncated",
		"job_id",
		"user_id",
		"job_status",
		"job_stage",
		"title",
		"source_video_key",
		"sub_stage_status",
		"sub_stage_reason",
		"sub_stage_duration_ms",
		"sub_stage_started_at",
		"sub_stage_finished_at",
		"job_created_at",
		"job_updated_at",
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

	subStageValue := "all"
	subStageLabel := "全部阶段"
	if subStage != "" {
		subStageValue = subStage
		subStageLabel = videoJobGIFSubStageLabel(subStage)
	}
	subStatusValue := "all"
	if subStatus != "" {
		subStatusValue = subStatus
	}
	truncated := totalRows > int64(len(rows))

	if err := writeRow([]string{
		"summary",
		windowLabel,
		since.Format(time.RFC3339),
		now.Format(time.RFC3339),
		subStageValue,
		subStageLabel,
		subStatusValue,
		strconv.FormatInt(totalRows, 10),
		strconv.FormatInt(totalJobs, 10),
		strconv.Itoa(limit),
		strconv.FormatBool(truncated),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, item := range rows {
		if err := writeRow([]string{
			"row",
			windowLabel,
			since.Format(time.RFC3339),
			now.Format(time.RFC3339),
			item.SubStage,
			videoJobGIFSubStageLabel(item.SubStage),
			subStatusValue,
			"",
			"",
			"",
			"",
			strconv.FormatUint(item.JobID, 10),
			strconv.FormatUint(item.UserID, 10),
			item.JobStatus,
			item.JobStage,
			item.Title,
			item.SourceVideoKey,
			item.SubStageStatus,
			item.SubStageReason,
			strconv.FormatInt(item.SubStageDurationMs, 10),
			item.SubStageStartedAt,
			item.SubStageFinishedAt,
			item.JobCreatedAt.Format(time.RFC3339),
			item.JobUpdatedAt.Format(time.RFC3339),
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

	filename := fmt.Sprintf("video_jobs_gif_sub_stage_anomalies_%s_%s.csv", windowLabel, now.Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}
