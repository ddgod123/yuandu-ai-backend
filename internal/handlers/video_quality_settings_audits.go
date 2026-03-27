package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
)

const (
	videoQualitySettingAuditTargetType = "video_quality_setting"
	videoQualitySettingAuditAction     = "admin_update_video_quality_setting"
)

type VideoQualitySettingChangedField struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value,omitempty"`
	NewValue interface{} `json:"new_value,omitempty"`
}

type VideoQualitySettingAuditItem struct {
	ID            uint64                            `json:"id"`
	AdminID       uint64                            `json:"admin_id"`
	Action        string                            `json:"action"`
	ChangeKind    string                            `json:"change_kind,omitempty"`
	FormatScope   string                            `json:"format_scope"`
	ResolvedFrom  []string                          `json:"resolved_from,omitempty"`
	ChangedCount  int                               `json:"changed_count"`
	ChangedFields []VideoQualitySettingChangedField `json:"changed_fields,omitempty"`
	CreatedAt     string                            `json:"created_at"`
}

type ListVideoQualitySettingAuditsResponse struct {
	Items []VideoQualitySettingAuditItem `json:"items"`
}

func buildVideoQualitySettingChangedFields(beforeReq, afterReq VideoQualitySettingRequest) ([]VideoQualitySettingChangedField, error) {
	beforeMap, err := videoQualitySettingRequestToMap(beforeReq)
	if err != nil {
		return nil, err
	}
	afterMap, err := videoQualitySettingRequestToMap(afterReq)
	if err != nil {
		return nil, err
	}
	diff := diffVideoQualitySettingMaps(beforeMap, afterMap)
	if len(diff) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(diff))
	for key := range diff {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	changes := make([]VideoQualitySettingChangedField, 0, len(keys))
	for _, key := range keys {
		changes = append(changes, VideoQualitySettingChangedField{
			Field:    key,
			OldValue: beforeMap[key],
			NewValue: afterMap[key],
		})
	}
	return changes, nil
}

func parseVideoQualitySettingChangedFields(raw interface{}) []VideoQualitySettingChangedField {
	rows, ok := raw.([]interface{})
	if !ok || len(rows) == 0 {
		return nil
	}
	changes := make([]VideoQualitySettingChangedField, 0, len(rows))
	for _, row := range rows {
		item := mapFromAnyValue(row)
		field := strings.TrimSpace(stringFromAny(item["field"]))
		if field == "" {
			continue
		}
		changes = append(changes, VideoQualitySettingChangedField{
			Field:    field,
			OldValue: item["old_value"],
			NewValue: item["new_value"],
		})
	}
	if len(changes) == 0 {
		return nil
	}
	sort.SliceStable(changes, func(i, j int) bool {
		return changes[i].Field < changes[j].Field
	})
	return changes
}

func intFromAnyAudit(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case int32:
		return int(value)
	case uint:
		return int(value)
	case uint64:
		return int(value)
	case uint32:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	case string:
		num, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return num
		}
	}
	return 0
}

func stringSliceFromAnyValue(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(stringFromAny(item))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (h *Handler) recordVideoQualitySettingAudit(c *gin.Context, formatScope string, resolvedFrom []string, changeKind string, beforeReq, afterReq VideoQualitySettingRequest) {
	if h == nil || c == nil {
		return
	}
	changes, err := buildVideoQualitySettingChangedFields(beforeReq, afterReq)
	if err != nil || len(changes) == 0 {
		return
	}
	changeRows := make([]map[string]interface{}, 0, len(changes))
	for _, change := range changes {
		changeRows = append(changeRows, map[string]interface{}{
			"field":     change.Field,
			"old_value": change.OldValue,
			"new_value": change.NewValue,
		})
	}
	if formatScope == "" {
		formatScope = videoQualitySettingFormatAll
	}
	adminID, _ := currentUserIDFromContext(c)
	h.recordAuditLog(adminID, videoQualitySettingAuditTargetType, 1, videoQualitySettingAuditAction, map[string]interface{}{
		"format_scope":   formatScope,
		"resolved_from":  resolvedFrom,
		"change_kind":    strings.ToLower(strings.TrimSpace(changeKind)),
		"changed_count":  len(changeRows),
		"changed_fields": changeRows,
	})
}

// ListAdminVideoQualitySettingAudits godoc
// @Summary List video quality setting field-level audits (admin)
// @Tags admin
// @Produce json
// @Param format query string false "format scope: all|gif|png|jpg|webp|live|mp4"
// @Param limit query int false "max rows, default 20, max 100"
// @Success 200 {object} ListVideoQualitySettingAuditsResponse
// @Router /api/admin/video-jobs/quality-settings/audits [get]
func (h *Handler) ListAdminVideoQualitySettingAudits(c *gin.Context) {
	formatScope, err := normalizeVideoQualitySettingFormatScope(c.Query("format"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 1 || value > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit, expected 1..100"})
			return
		}
		limit = value
	}

	query := h.db.Model(&models.AuditLog{}).
		Where("target_type = ? AND action = ?", videoQualitySettingAuditTargetType, videoQualitySettingAuditAction)
	if formatScope != videoQualitySettingFormatAll {
		query = query.Where("(meta ->> 'format_scope') = ?", formatScope)
	}

	var rows []models.AuditLog
	if err := query.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]VideoQualitySettingAuditItem, 0, len(rows))
	for _, row := range rows {
		meta := parseJSONMap(row.Meta)
		changedFields := parseVideoQualitySettingChangedFields(meta["changed_fields"])
		changedCount := intFromAnyAudit(meta["changed_count"])
		if changedCount <= 0 {
			changedCount = len(changedFields)
		}
		scope := strings.ToLower(strings.TrimSpace(stringFromAny(meta["format_scope"])))
		if scope == "" {
			scope = videoQualitySettingFormatAll
		}
		items = append(items, VideoQualitySettingAuditItem{
			ID:            row.ID,
			AdminID:       row.AdminID,
			Action:        row.Action,
			ChangeKind:    strings.ToLower(strings.TrimSpace(stringFromAny(meta["change_kind"]))),
			FormatScope:   scope,
			ResolvedFrom:  stringSliceFromAnyValue(meta["resolved_from"]),
			ChangedCount:  changedCount,
			ChangedFields: changedFields,
			CreatedAt:     row.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, ListVideoQualitySettingAuditsResponse{Items: items})
}
