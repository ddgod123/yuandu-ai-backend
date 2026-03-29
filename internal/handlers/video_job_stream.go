package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type videoJobStreamEnvelope struct {
	SchemaVersion string                     `json:"schema_version"`
	Type          string                     `json:"type"`
	JobID         uint64                     `json:"job_id"`
	Event         *VideoJobEventItemResponse `json:"event,omitempty"`
	NextSinceID   uint64                     `json:"next_since_id,omitempty"`
	TS            string                     `json:"ts"`
	Message       string                     `json:"message,omitempty"`
}

// StreamVideoJobEvents godoc
// @Summary Stream current user video job events (SSE)
// @Tags user
// @Produce text/event-stream
// @Param id path int true "job id"
// @Param since_id query int false "event cursor"
// @Router /api/video-jobs/{id}/stream [get]
func (h *Handler) StreamVideoJobEvents(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var job models.VideoJob
	if err := h.db.Select("id, status, stage, output_formats").Where("id = ? AND user_id = ?", jobID, userID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sinceID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("since_id")), 10, 64)
	heartbeatSec, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("heartbeat_sec", "15")))
	if heartbeatSec < 5 {
		heartbeatSec = 5
	}
	if heartbeatSec > 60 {
		heartbeatSec = 60
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "stream not supported"})
		return
	}

	header := c.Writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache, no-transform")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	header.Set("Transfer-Encoding", "chunked")
	c.Status(http.StatusOK)

	writeSSE := func(event string, payload videoJobStreamEnvelope) error {
		payload.SchemaVersion = "video_job_stream_v1"
		payload.JobID = jobID
		if strings.TrimSpace(payload.TS) == "" {
			payload.TS = time.Now().UTC().Format(time.RFC3339Nano)
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if event != "" {
			if _, err := c.Writer.WriteString("event: " + event + "\n"); err != nil {
				return err
			}
		}
		if _, err := c.Writer.WriteString("data: " + string(raw) + "\n\n"); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := writeSSE("hello", videoJobStreamEnvelope{
		Type:        "hello",
		NextSinceID: sinceID,
		Message:     fmt.Sprintf("job #%d stream connected", jobID),
	}); err != nil {
		return
	}

	ctx := c.Request.Context()
	pollTicker := time.NewTicker(1200 * time.Millisecond)
	defer pollTicker.Stop()
	heartbeatTicker := time.NewTicker(time.Duration(heartbeatSec) * time.Second)
	defer heartbeatTicker.Stop()

	requestedFormat := normalizeVideoImageFormatFilter(strings.Split(strings.ToLower(strings.TrimSpace(job.OutputFormats)), ",")[0])
	routedTables := resolveVideoImageReadTables(requestedFormat)
	baseEventsTable := models.VideoImageEventPublic{}.TableName()
	activeEventsTable := strings.TrimSpace(routedTables.Events)
	if activeEventsTable == "" {
		activeEventsTable = baseEventsTable
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			var rows []models.VideoImageEventPublic
			query := h.db.Table(activeEventsTable).
				Where("job_id = ? AND id > ?", jobID, sinceID).
				Order("id ASC").
				Limit(120)
			if err := query.Find(&rows).Error; err != nil {
				if activeEventsTable != baseEventsTable && isMissingTableError(err, activeEventsTable) {
					activeEventsTable = baseEventsTable
					continue
				}
				_ = writeSSE("stream_error", videoJobStreamEnvelope{
					Type:        "error",
					Message:     err.Error(),
					NextSinceID: sinceID,
				})
				return
			}

			for _, row := range rows {
				item := VideoJobEventItemResponse{
					ID:        row.ID,
					Stage:     strings.TrimSpace(row.Stage),
					Level:     strings.TrimSpace(row.Level),
					Message:   strings.TrimSpace(row.Message),
					Metadata:  parseJSONMap(row.Metadata),
					CreatedAt: row.CreatedAt,
				}
				if row.ID > sinceID {
					sinceID = row.ID
				}
				if err := writeSSE("video_job_event", videoJobStreamEnvelope{
					Type:        "video_job_event",
					Event:       &item,
					NextSinceID: sinceID,
				}); err != nil {
					return
				}
			}
		case <-heartbeatTicker.C:
			if err := writeSSE("heartbeat", videoJobStreamEnvelope{
				Type:        "heartbeat",
				NextSinceID: sinceID,
				Message:     "keepalive",
			}); err != nil {
				return
			}
		}
	}
}
