package handlers

import (
	"net/http"
	"strings"

	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
)

type adminVideoImageReadTablesPayload struct {
	Jobs     string `json:"jobs"`
	Outputs  string `json:"outputs"`
	Packages string `json:"packages"`
	Events   string `json:"events"`
	Feedback string `json:"feedback"`
}

type adminVideoImageReadRouteResponse struct {
	SplitOnly        bool                             `json:"split_only"`
	DebugEnabled     bool                             `json:"debug_enabled"`
	RequestedFormat  string                           `json:"requested_format"`
	NormalizedFormat string                           `json:"normalized_format"`
	Tables           adminVideoImageReadTablesPayload `json:"tables"`
	BaseTables       adminVideoImageReadTablesPayload `json:"base_tables"`
}

type setVideoImageReadRouteDebugRequest struct {
	Enabled *bool `json:"enabled"`
}

// GetAdminVideoImageReadRoute godoc
// @Summary 查看视频图片读侧路由（按格式命中分表）
// @Tags admin
// @Produce json
// @Param format query string false "format: gif|png|jpg|webp|live|mp4|all"
// @Success 200 {object} adminVideoImageReadRouteResponse
// @Router /api/admin/video-jobs/read-route [get]
func (h *Handler) GetAdminVideoImageReadRoute(c *gin.Context) {
	rawFormat := strings.TrimSpace(c.Query("format"))
	normalized := normalizeVideoImageFormatFilter(rawFormat)
	tables := resolveVideoImageReadTables(normalized)
	c.JSON(http.StatusOK, adminVideoImageReadRouteResponse{
		SplitOnly:        true,
		DebugEnabled:     isVideoImageReadRouteDebugEnabled(),
		RequestedFormat:  rawFormat,
		NormalizedFormat: normalized,
		Tables: adminVideoImageReadTablesPayload{
			Jobs:     tables.Jobs,
			Outputs:  tables.Outputs,
			Packages: tables.Packages,
			Events:   tables.Events,
			Feedback: tables.Feedback,
		},
		BaseTables: adminVideoImageReadTablesPayload{
			Jobs:     videojobs.PublicVideoImageBaseJobsTable(),
			Outputs:  videojobs.PublicVideoImageBaseOutputsTable(),
			Packages: videojobs.PublicVideoImageBasePackagesTable(),
			Events:   videojobs.PublicVideoImageBaseEventsTable(),
			Feedback: videojobs.PublicVideoImageBaseFeedbackTable(),
		},
	})
}

// SetAdminVideoImageReadRouteDebug godoc
// @Summary 设置视频图片读侧路由日志开关
// @Tags admin
// @Accept json
// @Produce json
// @Param enabled query string false "1|0|true|false"
// @Param body body setVideoImageReadRouteDebugRequest false "debug switch"
// @Success 200 {object} map[string]interface{}
// @Router /api/admin/video-jobs/read-route/debug [post]
func (h *Handler) SetAdminVideoImageReadRouteDebug(c *gin.Context) {
	var req setVideoImageReadRouteDebugRequest
	_ = c.ShouldBindJSON(&req)

	enabled := req.Enabled
	if enabled == nil {
		if parsed, ok := parseOptionalBoolParam(c.Query("enabled")); ok {
			enabled = parsed
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid enabled"})
			return
		}
	}
	if enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled is required"})
		return
	}

	setVideoImageReadRouteDebugEnabled(*enabled)
	c.JSON(http.StatusOK, gin.H{
		"ok":            true,
		"debug_enabled": isVideoImageReadRouteDebugEnabled(),
	})
}
