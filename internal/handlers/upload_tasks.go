package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type UploadTaskItem struct {
	ID           uint64         `json:"id"`
	Kind         string         `json:"kind"`
	Status       string         `json:"status"`
	Stage        string         `json:"stage"`
	UserID       uint64         `json:"user_id"`
	CollectionID *uint64        `json:"collection_id,omitempty"`
	CategoryID   *uint64        `json:"category_id,omitempty"`
	FileName     string         `json:"file_name"`
	FileSize     int64          `json:"file_size"`
	Input        datatypes.JSON `json:"input"`
	Result       datatypes.JSON `json:"result"`
	ErrorMessage string         `json:"error_message,omitempty"`
	StartedAt    time.Time      `json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type UploadTaskListResponse struct {
	Items []UploadTaskItem `json:"items"`
	Total int64            `json:"total"`
}

func jsonOrEmpty(payload any) datatypes.JSON {
	if payload == nil {
		return datatypes.JSON([]byte("{}"))
	}
	bs, err := json.Marshal(payload)
	if err != nil || len(bs) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(bs)
}

func (h *Handler) createUploadTask(
	kind string,
	userID uint64,
	collectionID *uint64,
	categoryID *uint64,
	fileName string,
	fileSize int64,
	input map[string]interface{},
) uint64 {
	task := models.UploadTask{
		Kind:         kind,
		Status:       "running",
		Stage:        "uploading",
		UserID:       userID,
		CollectionID: collectionID,
		CategoryID:   categoryID,
		FileName:     fileName,
		FileSize:     fileSize,
		Input:        jsonOrEmpty(input),
		Result:       datatypes.JSON([]byte("{}")),
		ErrorMessage: "",
		StartedAt:    time.Now(),
	}
	if err := h.db.Create(&task).Error; err != nil {
		return 0
	}
	return task.ID
}

func (h *Handler) updateUploadTaskStage(taskID uint64, stage string) {
	if taskID == 0 {
		return
	}
	_ = h.db.Model(&models.UploadTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"stage":      stage,
			"updated_at": time.Now(),
		}).Error
}

func (h *Handler) finishUploadTask(taskID uint64, success bool, errMsg string, result map[string]interface{}) {
	if taskID == 0 {
		return
	}
	now := time.Now()
	status := "success"
	if !success {
		status = "failed"
	}
	updates := map[string]interface{}{
		"status":        status,
		"stage":         "done",
		"error_message": strings.TrimSpace(errMsg),
		"result":        jsonOrEmpty(result),
		"finished_at":   &now,
		"updated_at":    now,
	}
	_ = h.db.Model(&models.UploadTask{}).
		Where("id = ?", taskID).
		Updates(updates).Error
}

// ListUploadTasks godoc
// @Summary List upload tasks (admin)
// @Tags admin
// @Produce json
// @Param page query int false "page"
// @Param page_size query int false "page size (max 200)"
// @Param kind query string false "task kind: import/append"
// @Param status query string false "task status: running/success/failed"
// @Success 200 {object} UploadTaskListResponse
// @Router /api/admin/upload-tasks [get]
func (h *Handler) ListUploadTasks(c *gin.Context) {
	page := 1
	pageSize := 30
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			if parsed > 200 {
				parsed = 200
			}
			pageSize = parsed
		}
	}
	offset := (page - 1) * pageSize

	query := h.db.Model(&models.UploadTask{})
	if kind := strings.TrimSpace(c.Query("kind")); kind != "" {
		query = query.Where("kind = ?", kind)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []models.UploadTask
	if err := query.Order("id desc").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]UploadTaskItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, UploadTaskItem{
			ID:           row.ID,
			Kind:         row.Kind,
			Status:       row.Status,
			Stage:        row.Stage,
			UserID:       row.UserID,
			CollectionID: row.CollectionID,
			CategoryID:   row.CategoryID,
			FileName:     row.FileName,
			FileSize:     row.FileSize,
			Input:        row.Input,
			Result:       row.Result,
			ErrorMessage: row.ErrorMessage,
			StartedAt:    row.StartedAt,
			FinishedAt:   row.FinishedAt,
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, UploadTaskListResponse{
		Items: items,
		Total: total,
	})
}
