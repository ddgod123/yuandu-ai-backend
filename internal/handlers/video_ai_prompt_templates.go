package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	videoAIPromptTemplateFormatAll    = "all"
	videoAIPromptTemplateFormatGIF    = "gif"
	videoAIPromptTemplateFormatWebP   = "webp"
	videoAIPromptTemplateFormatJPG    = "jpg"
	videoAIPromptTemplateFormatPNG    = "png"
	videoAIPromptTemplateFormatLive   = "live"
	videoAIPromptTemplateStageAI1     = "ai1"
	videoAIPromptTemplateStageAI2     = "ai2"
	videoAIPromptTemplateStageScore   = "scoring"
	videoAIPromptTemplateStageAI3     = "ai3"
	videoAIPromptTemplateLayerFixed   = "fixed"
	videoAIPromptTemplateLayerEdit    = "editable"
	videoAIPromptTemplateActionCreate = "create"
	videoAIPromptTemplateActionUpdate = "update"
	videoAIPromptTemplateActionActive = "activate"
)

type AdminVideoAIPromptTemplateItem struct {
	ID                 uint64                 `json:"id"`
	Format             string                 `json:"format"`
	Stage              string                 `json:"stage"`
	Layer              string                 `json:"layer"`
	TemplateText       string                 `json:"template_text"`
	TemplateJSONSchema map[string]interface{} `json:"template_json_schema,omitempty"`
	Enabled            bool                   `json:"enabled"`
	Version            string                 `json:"version"`
	IsActive           bool                   `json:"is_active"`
	CreatedBy          uint64                 `json:"created_by"`
	UpdatedBy          uint64                 `json:"updated_by"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt          string                 `json:"created_at,omitempty"`
	UpdatedAt          string                 `json:"updated_at,omitempty"`
	ResolvedFrom       string                 `json:"resolved_from,omitempty"`
}

type GetAdminVideoAIPromptTemplatesResponse struct {
	Format string                           `json:"format"`
	Items  []AdminVideoAIPromptTemplateItem `json:"items"`
}

type AdminVideoAIPromptTemplateAuditItem struct {
	ID              uint64                 `json:"id"`
	TemplateID      uint64                 `json:"template_id,omitempty"`
	Format          string                 `json:"format"`
	Stage           string                 `json:"stage"`
	Layer           string                 `json:"layer"`
	Action          string                 `json:"action"`
	Reason          string                 `json:"reason,omitempty"`
	OperatorAdminID uint64                 `json:"operator_admin_id"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt       string                 `json:"created_at,omitempty"`
}

type ListAdminVideoAIPromptTemplateAuditsResponse struct {
	Items []AdminVideoAIPromptTemplateAuditItem `json:"items"`
}

type AdminVideoAIPromptTemplateVersionItem struct {
	ID        uint64 `json:"id"`
	Format    string `json:"format"`
	Stage     string `json:"stage"`
	Layer     string `json:"layer"`
	Version   string `json:"version"`
	Enabled   bool   `json:"enabled"`
	IsActive  bool   `json:"is_active"`
	CreatedBy uint64 `json:"created_by"`
	UpdatedBy uint64 `json:"updated_by"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type ListAdminVideoAIPromptTemplateVersionsResponse struct {
	Format       string                                  `json:"format"`
	Stage        string                                  `json:"stage"`
	Layer        string                                  `json:"layer"`
	ResolvedFrom string                                  `json:"resolved_from"`
	Items        []AdminVideoAIPromptTemplateVersionItem `json:"items"`
}

type ActivateAdminVideoAIPromptTemplateVersionRequest struct {
	Format     string `json:"format"`
	Stage      string `json:"stage"`
	Layer      string `json:"layer"`
	TemplateID uint64 `json:"template_id"`
	Reason     string `json:"reason"`
}

type PatchAdminVideoAIPromptTemplateRequest struct {
	Format string                                 `json:"format"`
	Items  []PatchAdminVideoAIPromptTemplateEntry `json:"items"`
}

type PatchAdminVideoAIPromptTemplateEntry struct {
	Stage              string                 `json:"stage"`
	Layer              string                 `json:"layer"`
	TemplateText       string                 `json:"template_text"`
	TemplateJSONSchema map[string]interface{} `json:"template_json_schema"`
	Enabled            *bool                  `json:"enabled"`
	Version            string                 `json:"version"`
	Metadata           map[string]interface{} `json:"metadata"`
	Reason             string                 `json:"reason"`
}

type resolvedVideoAIPromptTemplate struct {
	Row  models.VideoAIPromptTemplate
	From string
}

func normalizeVideoAIPromptTemplateFormat(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return videoAIPromptTemplateFormatAll, nil
	}
	switch value {
	case videoAIPromptTemplateFormatAll,
		videoAIPromptTemplateFormatGIF,
		videoAIPromptTemplateFormatWebP,
		videoAIPromptTemplateFormatJPG,
		videoAIPromptTemplateFormatPNG,
		videoAIPromptTemplateFormatLive:
		return value, nil
	default:
		return "", errors.New("invalid format, expected one of all/gif/webp/jpg/png/live")
	}
}

func normalizeVideoAIPromptTemplateStage(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case videoAIPromptTemplateStageAI1, videoAIPromptTemplateStageAI2, videoAIPromptTemplateStageScore, videoAIPromptTemplateStageAI3:
		return value, nil
	default:
		return "", errors.New("invalid stage, expected one of ai1/ai2/scoring/ai3")
	}
}

func normalizeVideoAIPromptTemplateStageOptional(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return normalizeVideoAIPromptTemplateStage(raw)
}

func normalizeVideoAIPromptTemplateLayer(raw string, stage string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if stage == videoAIPromptTemplateStageAI1 {
		switch value {
		case videoAIPromptTemplateLayerEdit, videoAIPromptTemplateLayerFixed:
			return value, nil
		default:
			return "", errors.New("invalid layer for ai1, expected editable/fixed")
		}
	}
	if value != videoAIPromptTemplateLayerFixed {
		return "", errors.New("invalid layer, non-ai1 stage only supports fixed")
	}
	return value, nil
}

func normalizeVideoAIPromptTemplateLayerOptional(raw string, stage string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if strings.TrimSpace(stage) == "" {
		return "", errors.New("stage is required when layer is provided")
	}
	return normalizeVideoAIPromptTemplateLayer(raw, stage)
}

func toJSONMap(raw datatypes.JSON) map[string]interface{} {
	if len(raw) == 0 {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]interface{}{}
	}
	return out
}

func toJSONBOrDefault(input map[string]interface{}, fallback map[string]interface{}) datatypes.JSON {
	value := input
	if value == nil {
		value = fallback
	}
	if value == nil {
		value = map[string]interface{}{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(raw)
}

func isMissingTableError(err error, table string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	if strings.Contains(message, "sqlstate 42p01") || strings.Contains(message, "undefined_table") {
		return true
	}
	if strings.Contains(message, "does not exist") && strings.Contains(message, strings.ToLower(strings.TrimSpace(table))) {
		return true
	}
	return false
}

func toAdminVideoAIPromptTemplateItem(row models.VideoAIPromptTemplate, resolvedFrom string) AdminVideoAIPromptTemplateItem {
	return AdminVideoAIPromptTemplateItem{
		ID:                 row.ID,
		Format:             strings.ToLower(strings.TrimSpace(row.Format)),
		Stage:              strings.ToLower(strings.TrimSpace(row.Stage)),
		Layer:              strings.ToLower(strings.TrimSpace(row.Layer)),
		TemplateText:       row.TemplateText,
		TemplateJSONSchema: toJSONMap(row.TemplateJSONSchema),
		Enabled:            row.Enabled,
		Version:            strings.TrimSpace(row.Version),
		IsActive:           row.IsActive,
		CreatedBy:          row.CreatedBy,
		UpdatedBy:          row.UpdatedBy,
		Metadata:           toJSONMap(row.Metadata),
		CreatedAt:          row.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          row.UpdatedAt.Format(time.RFC3339),
		ResolvedFrom:       strings.TrimSpace(resolvedFrom),
	}
}

func toAdminVideoAIPromptTemplateVersionItem(row models.VideoAIPromptTemplate) AdminVideoAIPromptTemplateVersionItem {
	return AdminVideoAIPromptTemplateVersionItem{
		ID:        row.ID,
		Format:    strings.ToLower(strings.TrimSpace(row.Format)),
		Stage:     strings.ToLower(strings.TrimSpace(row.Stage)),
		Layer:     strings.ToLower(strings.TrimSpace(row.Layer)),
		Version:   strings.TrimSpace(row.Version),
		Enabled:   row.Enabled,
		IsActive:  row.IsActive,
		CreatedBy: row.CreatedBy,
		UpdatedBy: row.UpdatedBy,
		CreatedAt: row.CreatedAt.Format(time.RFC3339),
		UpdatedAt: row.UpdatedAt.Format(time.RFC3339),
	}
}

func (h *Handler) loadVideoAIPromptTemplateVersions(format, stage, layer string) ([]models.VideoAIPromptTemplate, string, error) {
	if h == nil || h.db == nil {
		return nil, "", errors.New("db not initialized")
	}
	list := make([]models.VideoAIPromptTemplate, 0)
	if err := h.db.Where("format = ? AND stage = ? AND layer = ?", format, stage, layer).
		Order("is_active DESC, id DESC").
		Find(&list).Error; err != nil {
		if isMissingTableError(err, "video_ai_prompt_templates") {
			return nil, "", nil
		}
		return nil, "", err
	}
	if len(list) > 0 {
		return list, format, nil
	}
	if format != videoAIPromptTemplateFormatAll {
		if err := h.db.Where("format = ? AND stage = ? AND layer = ?", videoAIPromptTemplateFormatAll, stage, layer).
			Order("is_active DESC, id DESC").
			Find(&list).Error; err != nil {
			if isMissingTableError(err, "video_ai_prompt_templates") {
				return nil, "", nil
			}
			return nil, "", err
		}
		if len(list) > 0 {
			return list, videoAIPromptTemplateFormatAll, nil
		}
	}
	return nil, "", nil
}

func (h *Handler) loadVideoAIPromptTemplateActiveExact(tx *gorm.DB, format, stage, layer string) (models.VideoAIPromptTemplate, bool, error) {
	var row models.VideoAIPromptTemplate
	err := tx.Where("format = ? AND stage = ? AND layer = ? AND is_active = ?", format, stage, layer, true).
		Order("id DESC").
		Limit(1).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.VideoAIPromptTemplate{}, false, nil
		}
		return models.VideoAIPromptTemplate{}, false, err
	}
	return row, true, nil
}

func (h *Handler) loadVideoAIPromptTemplateResolved(tx *gorm.DB, format, stage, layer string) (resolvedVideoAIPromptTemplate, bool, error) {
	exact, found, err := h.loadVideoAIPromptTemplateActiveExact(tx, format, stage, layer)
	if err != nil {
		return resolvedVideoAIPromptTemplate{}, false, err
	}
	if found {
		return resolvedVideoAIPromptTemplate{Row: exact, From: format}, true, nil
	}
	if format != videoAIPromptTemplateFormatAll {
		fallback, fallbackFound, fallbackErr := h.loadVideoAIPromptTemplateActiveExact(tx, videoAIPromptTemplateFormatAll, stage, layer)
		if fallbackErr != nil {
			return resolvedVideoAIPromptTemplate{}, false, fallbackErr
		}
		if fallbackFound {
			return resolvedVideoAIPromptTemplate{Row: fallback, From: videoAIPromptTemplateFormatAll}, true, nil
		}
	}
	return resolvedVideoAIPromptTemplate{}, false, nil
}

func (h *Handler) loadAdminVideoAIPromptTemplateItems(format string) ([]AdminVideoAIPromptTemplateItem, error) {
	if h == nil || h.db == nil {
		return nil, errors.New("db not initialized")
	}
	pairs := [][2]string{
		{videoAIPromptTemplateStageAI1, videoAIPromptTemplateLayerEdit},
		{videoAIPromptTemplateStageAI1, videoAIPromptTemplateLayerFixed},
		{videoAIPromptTemplateStageAI2, videoAIPromptTemplateLayerFixed},
		{videoAIPromptTemplateStageScore, videoAIPromptTemplateLayerFixed},
		{videoAIPromptTemplateStageAI3, videoAIPromptTemplateLayerFixed},
	}
	items := make([]AdminVideoAIPromptTemplateItem, 0, len(pairs))
	for _, pair := range pairs {
		row, found, err := h.loadVideoAIPromptTemplateResolved(h.db, format, pair[0], pair[1])
		if err != nil {
			if isMissingTableError(err, "video_ai_prompt_templates") {
				return items, nil
			}
			return nil, err
		}
		if !found {
			continue
		}
		items = append(items, toAdminVideoAIPromptTemplateItem(row.Row, row.From))
	}
	return items, nil
}

func buildVideoAIPromptTemplateSnapshot(row models.VideoAIPromptTemplate) map[string]interface{} {
	return map[string]interface{}{
		"id":                   row.ID,
		"format":               strings.ToLower(strings.TrimSpace(row.Format)),
		"stage":                strings.ToLower(strings.TrimSpace(row.Stage)),
		"layer":                strings.ToLower(strings.TrimSpace(row.Layer)),
		"template_text":        row.TemplateText,
		"template_json_schema": toJSONMap(row.TemplateJSONSchema),
		"enabled":              row.Enabled,
		"version":              strings.TrimSpace(row.Version),
		"is_active":            row.IsActive,
		"created_by":           row.CreatedBy,
		"updated_by":           row.UpdatedBy,
		"metadata":             toJSONMap(row.Metadata),
		"created_at":           row.CreatedAt.Format(time.RFC3339),
		"updated_at":           row.UpdatedAt.Format(time.RFC3339),
	}
}

// GetAdminVideoAIPromptTemplates godoc
// @Summary Get AI prompt templates by format (admin)
// @Tags admin
// @Produce json
// @Param format query string false "all|gif|webp|jpg|png|live"
// @Success 200 {object} GetAdminVideoAIPromptTemplatesResponse
// @Router /api/admin/video-jobs/ai-prompt-templates [get]
func (h *Handler) GetAdminVideoAIPromptTemplates(c *gin.Context) {
	format, err := normalizeVideoAIPromptTemplateFormat(c.Query("format"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	items, loadErr := h.loadAdminVideoAIPromptTemplateItems(format)
	if loadErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": loadErr.Error()})
		return
	}

	c.JSON(http.StatusOK, GetAdminVideoAIPromptTemplatesResponse{
		Format: format,
		Items:  items,
	})
}

// ListAdminVideoAIPromptTemplateAudits godoc
// @Summary List AI prompt template audits (admin)
// @Tags admin
// @Produce json
// @Param format query string false "all|gif|webp|jpg|png|live"
// @Param stage query string false "ai1|ai2|scoring|ai3"
// @Param layer query string false "editable|fixed"
// @Param limit query int false "1..200"
// @Success 200 {object} ListAdminVideoAIPromptTemplateAuditsResponse
// @Router /api/admin/video-jobs/ai-prompt-templates/audits [get]
func (h *Handler) ListAdminVideoAIPromptTemplateAudits(c *gin.Context) {
	if h == nil || h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db not initialized"})
		return
	}
	format := ""
	if strings.TrimSpace(c.Query("format")) != "" {
		var err error
		format, err = normalizeVideoAIPromptTemplateFormat(c.Query("format"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	stage, err := normalizeVideoAIPromptTemplateStageOptional(c.Query("stage"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	layer, err := normalizeVideoAIPromptTemplateLayerOptional(c.Query("layer"), stage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		var parsed int
		if _, scanErr := fmt.Sscanf(raw, "%d", &parsed); scanErr != nil || parsed < 1 || parsed > 200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit, expected 1..200"})
			return
		}
		limit = parsed
	}

	query := h.db.Model(&models.VideoAIPromptTemplateAudit{}).Order("id DESC").Limit(limit)
	if format != "" {
		query = query.Where("format = ?", format)
	}
	if stage != "" {
		query = query.Where("stage = ?", stage)
	}
	if layer != "" {
		query = query.Where("layer = ?", layer)
	}
	var rows []models.VideoAIPromptTemplateAudit
	if err := query.Find(&rows).Error; err != nil {
		if isMissingTableError(err, "video_ai_prompt_template_audits") {
			c.JSON(http.StatusOK, ListAdminVideoAIPromptTemplateAuditsResponse{Items: []AdminVideoAIPromptTemplateAuditItem{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]AdminVideoAIPromptTemplateAuditItem, 0, len(rows))
	for _, row := range rows {
		item := AdminVideoAIPromptTemplateAuditItem{
			ID:              row.ID,
			Format:          strings.ToLower(strings.TrimSpace(row.Format)),
			Stage:           strings.ToLower(strings.TrimSpace(row.Stage)),
			Layer:           strings.ToLower(strings.TrimSpace(row.Layer)),
			Action:          strings.ToLower(strings.TrimSpace(row.Action)),
			Reason:          strings.TrimSpace(row.Reason),
			OperatorAdminID: row.OperatorAdminID,
			Metadata:        toJSONMap(row.Metadata),
			CreatedAt:       row.CreatedAt.Format(time.RFC3339),
		}
		if row.TemplateID != nil {
			item.TemplateID = *row.TemplateID
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, ListAdminVideoAIPromptTemplateAuditsResponse{Items: items})
}

// ListAdminVideoAIPromptTemplateVersions godoc
// @Summary List AI prompt template versions (admin)
// @Tags admin
// @Produce json
// @Param format query string false "all|gif|webp|jpg|png|live"
// @Param stage query string true "ai1|ai2|scoring|ai3"
// @Param layer query string true "editable|fixed"
// @Success 200 {object} ListAdminVideoAIPromptTemplateVersionsResponse
// @Router /api/admin/video-jobs/ai-prompt-templates/versions [get]
func (h *Handler) ListAdminVideoAIPromptTemplateVersions(c *gin.Context) {
	format, err := normalizeVideoAIPromptTemplateFormat(c.Query("format"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	stage, err := normalizeVideoAIPromptTemplateStage(c.Query("stage"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	layer, err := normalizeVideoAIPromptTemplateLayer(c.Query("layer"), stage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rows, resolvedFrom, loadErr := h.loadVideoAIPromptTemplateVersions(format, stage, layer)
	if loadErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": loadErr.Error()})
		return
	}
	items := make([]AdminVideoAIPromptTemplateVersionItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAdminVideoAIPromptTemplateVersionItem(row))
	}
	c.JSON(http.StatusOK, ListAdminVideoAIPromptTemplateVersionsResponse{
		Format:       format,
		Stage:        stage,
		Layer:        layer,
		ResolvedFrom: resolvedFrom,
		Items:        items,
	})
}

// ActivateAdminVideoAIPromptTemplateVersion godoc
// @Summary Activate AI prompt template version (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body ActivateAdminVideoAIPromptTemplateVersionRequest true "activate template version"
// @Success 200 {object} ListAdminVideoAIPromptTemplateVersionsResponse
// @Router /api/admin/video-jobs/ai-prompt-templates/activate [post]
func (h *Handler) ActivateAdminVideoAIPromptTemplateVersion(c *gin.Context) {
	if h == nil || h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db not initialized"})
		return
	}
	var req ActivateAdminVideoAIPromptTemplateVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	format, err := normalizeVideoAIPromptTemplateFormat(req.Format)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	stage, err := normalizeVideoAIPromptTemplateStage(req.Stage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	layer, err := normalizeVideoAIPromptTemplateLayer(req.Layer, stage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.TemplateID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template_id is required"})
		return
	}
	adminID, _ := currentUserIDFromContext(c)
	reason := strings.TrimSpace(req.Reason)

	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		var target models.VideoAIPromptTemplate
		if err := tx.Where("id = ? AND format = ? AND stage = ? AND layer = ?", req.TemplateID, format, stage, layer).
			Take(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("template version not found under selected scope")
			}
			return err
		}
		now := time.Now()
		if err := tx.Model(&models.VideoAIPromptTemplate{}).
			Where("format = ? AND stage = ? AND layer = ? AND is_active = ?", format, stage, layer, true).
			Updates(map[string]interface{}{
				"is_active":  false,
				"updated_by": adminID,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.VideoAIPromptTemplate{}).
			Where("id = ?", target.ID).
			Updates(map[string]interface{}{
				"is_active":  true,
				"updated_by": adminID,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}

		target.IsActive = true
		target.UpdatedBy = adminID
		target.UpdatedAt = now
		templateID := target.ID
		audit := models.VideoAIPromptTemplateAudit{
			TemplateID:      &templateID,
			Format:          format,
			Stage:           stage,
			Layer:           layer,
			Action:          videoAIPromptTemplateActionActive,
			OldValue:        datatypes.JSON([]byte("{}")),
			NewValue:        toJSONBOrDefault(buildVideoAIPromptTemplateSnapshot(target), nil),
			Reason:          reason,
			OperatorAdminID: adminID,
			Metadata:        toJSONBOrDefault(map[string]interface{}{"source": "admin_activate"}, nil),
		}
		if err := tx.Create(&audit).Error; err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": txErr.Error()})
		return
	}

	rows, resolvedFrom, loadErr := h.loadVideoAIPromptTemplateVersions(format, stage, layer)
	if loadErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": loadErr.Error()})
		return
	}
	items := make([]AdminVideoAIPromptTemplateVersionItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAdminVideoAIPromptTemplateVersionItem(row))
	}
	c.JSON(http.StatusOK, ListAdminVideoAIPromptTemplateVersionsResponse{
		Format:       format,
		Stage:        stage,
		Layer:        layer,
		ResolvedFrom: resolvedFrom,
		Items:        items,
	})
}

// PatchAdminVideoAIPromptTemplates godoc
// @Summary Patch AI prompt templates by format (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body PatchAdminVideoAIPromptTemplateRequest true "patch ai prompt templates"
// @Success 200 {object} GetAdminVideoAIPromptTemplatesResponse
// @Router /api/admin/video-jobs/ai-prompt-templates [patch]
func (h *Handler) PatchAdminVideoAIPromptTemplates(c *gin.Context) {
	if h == nil || h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db not initialized"})
		return
	}

	var req PatchAdminVideoAIPromptTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "items is required"})
		return
	}

	format, err := normalizeVideoAIPromptTemplateFormat(req.Format)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	adminID, _ := currentUserIDFromContext(c)

	if txErr := h.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range req.Items {
			stage, stageErr := normalizeVideoAIPromptTemplateStage(item.Stage)
			if stageErr != nil {
				return stageErr
			}
			layer, layerErr := normalizeVideoAIPromptTemplateLayer(item.Layer, stage)
			if layerErr != nil {
				return layerErr
			}
			if !(stage == videoAIPromptTemplateStageAI1 && layer == videoAIPromptTemplateLayerEdit) {
				return errors.New("only ai1 editable layer can be patched; fixed layers are read-only")
			}

			current, found, currentErr := h.loadVideoAIPromptTemplateActiveExact(tx, format, stage, layer)
			if currentErr != nil {
				return currentErr
			}

			enabled := true
			if found {
				enabled = current.Enabled
			}
			if item.Enabled != nil {
				enabled = *item.Enabled
			}
			requestedVersion := strings.TrimSpace(item.Version)
			version, versionErr := resolveVideoAIPromptTemplatePatchVersion(tx, format, stage, layer, requestedVersion, current, found)
			if versionErr != nil {
				return versionErr
			}

			templateText := item.TemplateText
			if len(strings.TrimSpace(templateText)) > 0 {
				templateText = strings.TrimSpace(templateText)
			}
			if len(templateText) > 16000 {
				return errors.New("template_text length cannot exceed 16000")
			}

			schemaJSON := toJSONBOrDefault(item.TemplateJSONSchema, nil)
			metadataJSON := toJSONBOrDefault(item.Metadata, nil)
			now := time.Now()
			reason := strings.TrimSpace(item.Reason)

			if !found {
				row := models.VideoAIPromptTemplate{
					Format:             format,
					Stage:              stage,
					Layer:              layer,
					TemplateText:       templateText,
					TemplateJSONSchema: schemaJSON,
					Enabled:            enabled,
					Version:            version,
					IsActive:           true,
					CreatedBy:          adminID,
					UpdatedBy:          adminID,
					Metadata:           metadataJSON,
				}
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
				templateID := row.ID
				audit := models.VideoAIPromptTemplateAudit{
					TemplateID:      &templateID,
					Format:          format,
					Stage:           stage,
					Layer:           layer,
					Action:          videoAIPromptTemplateActionCreate,
					OldValue:        datatypes.JSON([]byte("{}")),
					NewValue:        toJSONBOrDefault(buildVideoAIPromptTemplateSnapshot(row), nil),
					Reason:          reason,
					OperatorAdminID: adminID,
					Metadata:        toJSONBOrDefault(map[string]interface{}{"source": "admin_patch"}, nil),
				}
				if err := tx.Create(&audit).Error; err != nil {
					return err
				}
				if format == videoAIPromptTemplateFormatAll && stage == videoAIPromptTemplateStageAI1 && layer == videoAIPromptTemplateLayerEdit {
					if err := tx.Model(&models.VideoQualitySetting{}).Where("id = 1").Updates(map[string]interface{}{
						"ai_director_operator_instruction":         templateText,
						"ai_director_operator_instruction_version": version,
						"ai_director_operator_enabled":             enabled,
						"updated_at":                               now,
					}).Error; err != nil && !isMissingTableError(err, "video_quality_settings") {
						return err
					}
				}
				continue
			}

			previous := current
			if err := tx.Model(&models.VideoAIPromptTemplate{}).
				Where("id = ? AND is_active = ?", current.ID, true).
				Updates(map[string]interface{}{
					"is_active":  false,
					"updated_by": adminID,
					"updated_at": now,
				}).Error; err != nil {
				return err
			}
			current.IsActive = false
			current.UpdatedBy = adminID
			current.UpdatedAt = now

			row := models.VideoAIPromptTemplate{
				Format:             format,
				Stage:              stage,
				Layer:              layer,
				TemplateText:       templateText,
				TemplateJSONSchema: schemaJSON,
				Enabled:            enabled,
				Version:            version,
				IsActive:           true,
				CreatedBy:          adminID,
				UpdatedBy:          adminID,
				Metadata:           metadataJSON,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}

			templateID := row.ID
			audit := models.VideoAIPromptTemplateAudit{
				TemplateID:      &templateID,
				Format:          format,
				Stage:           stage,
				Layer:           layer,
				Action:          videoAIPromptTemplateActionUpdate,
				OldValue:        toJSONBOrDefault(buildVideoAIPromptTemplateSnapshot(previous), nil),
				NewValue:        toJSONBOrDefault(buildVideoAIPromptTemplateSnapshot(row), nil),
				Reason:          reason,
				OperatorAdminID: adminID,
				Metadata:        toJSONBOrDefault(map[string]interface{}{"source": "admin_patch"}, nil),
			}
			if err := tx.Create(&audit).Error; err != nil {
				return err
			}

			if format == videoAIPromptTemplateFormatAll && stage == videoAIPromptTemplateStageAI1 && layer == videoAIPromptTemplateLayerEdit {
				if err := tx.Model(&models.VideoQualitySetting{}).Where("id = 1").Updates(map[string]interface{}{
					"ai_director_operator_instruction":         templateText,
					"ai_director_operator_instruction_version": version,
					"ai_director_operator_enabled":             enabled,
					"updated_at":                               now,
				}).Error; err != nil && !isMissingTableError(err, "video_quality_settings") {
					return err
				}
			}
		}
		return nil
	}); txErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": txErr.Error()})
		return
	}

	items, loadErr := h.loadAdminVideoAIPromptTemplateItems(format)
	if loadErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": loadErr.Error()})
		return
	}

	c.JSON(http.StatusOK, GetAdminVideoAIPromptTemplatesResponse{
		Format: format,
		Items:  items,
	})
}

func resolveVideoAIPromptTemplatePatchVersion(
	tx *gorm.DB,
	format string,
	stage string,
	layer string,
	requestedVersion string,
	current models.VideoAIPromptTemplate,
	found bool,
) (string, error) {
	baseVersion := strings.TrimSpace(requestedVersion)
	if baseVersion == "" {
		if found {
			baseVersion = strings.TrimSpace(current.Version)
		}
		if baseVersion == "" {
			baseVersion = "v1"
		}
		return ensureUniqueVideoAIPromptTemplateVersion(tx, format, stage, layer, baseVersion, true)
	}
	if len(baseVersion) > 64 {
		return "", errors.New("version length cannot exceed 64")
	}

	var exists int64
	if err := tx.Model(&models.VideoAIPromptTemplate{}).
		Where("format = ? AND stage = ? AND layer = ? AND version = ?", format, stage, layer, baseVersion).
		Count(&exists).Error; err != nil {
		return "", err
	}
	if exists > 0 {
		return "", errors.New("version already exists under current scope; please use a new version")
	}
	return baseVersion, nil
}

func ensureUniqueVideoAIPromptTemplateVersion(
	tx *gorm.DB,
	format string,
	stage string,
	layer string,
	base string,
	autoBump bool,
) (string, error) {
	version := strings.TrimSpace(base)
	if version == "" {
		version = "v1"
	}
	if len(version) > 64 {
		return "", errors.New("version length cannot exceed 64")
	}

	exists, err := checkVideoAIPromptTemplateVersionExists(tx, format, stage, layer, version)
	if err != nil {
		return "", err
	}
	if !exists {
		return version, nil
	}
	if !autoBump {
		return "", errors.New("version already exists under current scope; please use a new version")
	}

	basePrefix := version
	for i := 2; i < 10000; i++ {
		suffix := fmt.Sprintf("-rev%d", i)
		maxBaseLen := 64 - len(suffix)
		if maxBaseLen <= 0 {
			return "", errors.New("unable to allocate unique version")
		}
		candidatePrefix := basePrefix
		if len(candidatePrefix) > maxBaseLen {
			candidatePrefix = candidatePrefix[:maxBaseLen]
		}
		candidate := candidatePrefix + suffix
		hit, hitErr := checkVideoAIPromptTemplateVersionExists(tx, format, stage, layer, candidate)
		if hitErr != nil {
			return "", hitErr
		}
		if !hit {
			return candidate, nil
		}
	}
	return "", errors.New("unable to allocate unique version")
}

func checkVideoAIPromptTemplateVersionExists(
	tx *gorm.DB,
	format string,
	stage string,
	layer string,
	version string,
) (bool, error) {
	var cnt int64
	if err := tx.Model(&models.VideoAIPromptTemplate{}).
		Where("format = ? AND stage = ? AND layer = ? AND version = ?", format, stage, layer, version).
		Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}
