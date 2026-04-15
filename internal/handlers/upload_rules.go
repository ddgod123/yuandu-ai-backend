package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UploadRulePublicResponse struct {
	Enabled               bool     `json:"enabled"`
	AllowedExtensions     []string `json:"allowed_extensions"`
	MaxFileSizeBytes      int64    `json:"max_file_size_bytes"`
	MaxFilesPerCollection int      `json:"max_files_per_collection"`
	MaxFilesPerRequest    int      `json:"max_files_per_request"`
	ContentRules          []string `json:"content_rules"`
	ReferenceURL          string   `json:"reference_url,omitempty"`
}

type UploadRuleAdminResponse struct {
	UploadRulePublicResponse
	AutoAuditEnabled   bool     `json:"auto_audit_enabled"`
	AutoActivateOnPass bool     `json:"auto_activate_on_pass"`
	BlockedKeywords    []string `json:"blocked_keywords"`
	ContentRulesText   string   `json:"content_rules_text"`
	UpdatedBy          *uint64  `json:"updated_by,omitempty"`
	UpdatedAt          string   `json:"updated_at,omitempty"`
}

type UpdateUploadRuleAdminRequest struct {
	Enabled               *bool    `json:"enabled"`
	AutoAuditEnabled      *bool    `json:"auto_audit_enabled"`
	AutoActivateOnPass    *bool    `json:"auto_activate_on_pass"`
	AllowedExtensions     []string `json:"allowed_extensions"`
	MaxFileSizeBytes      *int64   `json:"max_file_size_bytes"`
	MaxFilesPerCollection *int     `json:"max_files_per_collection"`
	MaxFilesPerRequest    *int     `json:"max_files_per_request"`
	BlockedKeywords       []string `json:"blocked_keywords"`
	ContentRulesText      *string  `json:"content_rules_text"`
	ReferenceURL          *string  `json:"reference_url"`
}

type uploadRuleRuntime struct {
	Enabled               bool
	AutoAuditEnabled      bool
	AutoActivateOnPass    bool
	AllowedExtensions     []string
	AllowedExtSet         map[string]struct{}
	MaxFileSizeBytes      int64
	MaxFilesPerCollection int
	MaxFilesPerRequest    int
	BlockedKeywords       []string
	ContentRules          []string
	ContentRulesText      string
	ReferenceURL          string
}

func defaultUploadRuleSetting() models.UploadRuleSetting {
	return models.UploadRuleSetting{
		ID:                    1,
		Enabled:               true,
		AutoAuditEnabled:      true,
		AutoActivateOnPass:    false,
		AllowedExtensions:     "jpg,jpeg,png,gif,webp",
		MaxFileSizeBytes:      10 * 1024 * 1024,
		MaxFilesPerCollection: 50,
		MaxFilesPerRequest:    20,
		BlockedKeywords:       "涉政,色情,赌博,暴力,恐怖,诈骗,侵权,违法",
		ContentRules: strings.Join([]string{
			"不得上传违法违规、涉政极端、色情暴力、恐怖、诈骗等内容。",
			"不得侵犯他人著作权、商标权、肖像权、隐私权等合法权益。",
			"上传即默认你对素材拥有合法使用与传播授权。",
		}, "\n"),
		ReferenceURL: "https://mos.m.taobao.com/iconfont/upload_rule?spm=a313x.icons_upload.i1.5.176b3a813scH6m",
	}
}

func splitCSVOrLines(raw string) []string {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, "，", ",")
	normalized = strings.ReplaceAll(normalized, "；", ",")
	normalized = strings.ReplaceAll(normalized, ";", ",")
	parts := strings.FieldsFunc(normalized, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		val := strings.ToLower(strings.TrimSpace(part))
		val = strings.TrimPrefix(val, ".")
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out
}

func splitRuleLines(raw string) []string {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func normalizeUploadRuleRow(row models.UploadRuleSetting) models.UploadRuleSetting {
	def := defaultUploadRuleSetting()
	if strings.TrimSpace(row.AllowedExtensions) == "" {
		row.AllowedExtensions = def.AllowedExtensions
	}
	if row.MaxFileSizeBytes <= 0 {
		row.MaxFileSizeBytes = def.MaxFileSizeBytes
	}
	if row.MaxFilesPerCollection <= 0 {
		row.MaxFilesPerCollection = def.MaxFilesPerCollection
	}
	if row.MaxFilesPerRequest <= 0 {
		row.MaxFilesPerRequest = def.MaxFilesPerRequest
	}
	if strings.TrimSpace(row.ContentRules) == "" {
		row.ContentRules = def.ContentRules
	}
	if strings.TrimSpace(row.ReferenceURL) == "" {
		row.ReferenceURL = def.ReferenceURL
	}
	return row
}

func buildUploadRuleRuntime(row models.UploadRuleSetting) uploadRuleRuntime {
	row = normalizeUploadRuleRow(row)
	allowed := splitCSVOrLines(row.AllowedExtensions)
	if len(allowed) == 0 {
		allowed = splitCSVOrLines(defaultUploadRuleSetting().AllowedExtensions)
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, ext := range allowed {
		allowedSet[ext] = struct{}{}
	}
	blocked := splitCSVOrLines(row.BlockedKeywords)
	contentLines := splitRuleLines(row.ContentRules)
	return uploadRuleRuntime{
		Enabled:               row.Enabled,
		AutoAuditEnabled:      row.AutoAuditEnabled,
		AutoActivateOnPass:    row.AutoActivateOnPass,
		AllowedExtensions:     allowed,
		AllowedExtSet:         allowedSet,
		MaxFileSizeBytes:      row.MaxFileSizeBytes,
		MaxFilesPerCollection: row.MaxFilesPerCollection,
		MaxFilesPerRequest:    row.MaxFilesPerRequest,
		BlockedKeywords:       blocked,
		ContentRules:          contentLines,
		ContentRulesText:      strings.TrimSpace(row.ContentRules),
		ReferenceURL:          strings.TrimSpace(row.ReferenceURL),
	}
}

func (h *Handler) loadUploadRuleSetting() (models.UploadRuleSetting, error) {
	def := defaultUploadRuleSetting()
	if h == nil || h.db == nil {
		return def, nil
	}

	var row models.UploadRuleSetting
	if err := h.db.First(&row, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if createErr := h.db.Create(&def).Error; createErr != nil {
				if !isMissingTableError(createErr, "ops.upload_rule_settings") {
					return def, createErr
				}
				return def, nil
			}
			return def, nil
		}
		if isMissingTableError(err, "ops.upload_rule_settings") {
			return def, nil
		}
		return def, err
	}
	return normalizeUploadRuleRow(row), nil
}

func (h *Handler) loadUploadRuleRuntime() uploadRuleRuntime {
	row, err := h.loadUploadRuleSetting()
	if err != nil {
		return buildUploadRuleRuntime(defaultUploadRuleSetting())
	}
	return buildUploadRuleRuntime(row)
}

func toUploadRulePublicResponse(runtime uploadRuleRuntime) UploadRulePublicResponse {
	return UploadRulePublicResponse{
		Enabled:               runtime.Enabled,
		AllowedExtensions:     runtime.AllowedExtensions,
		MaxFileSizeBytes:      runtime.MaxFileSizeBytes,
		MaxFilesPerCollection: runtime.MaxFilesPerCollection,
		MaxFilesPerRequest:    runtime.MaxFilesPerRequest,
		ContentRules:          runtime.ContentRules,
		ReferenceURL:          runtime.ReferenceURL,
	}
}

func toUploadRuleAdminResponse(row models.UploadRuleSetting) UploadRuleAdminResponse {
	runtime := buildUploadRuleRuntime(row)
	resp := UploadRuleAdminResponse{
		UploadRulePublicResponse: toUploadRulePublicResponse(runtime),
		AutoAuditEnabled:         runtime.AutoAuditEnabled,
		AutoActivateOnPass:       runtime.AutoActivateOnPass,
		BlockedKeywords:          runtime.BlockedKeywords,
		ContentRulesText:         runtime.ContentRulesText,
		UpdatedBy:                row.UpdatedBy,
	}
	if !row.UpdatedAt.IsZero() {
		resp.UpdatedAt = row.UpdatedAt.Format(timeLayoutRFC3339())
	}
	return resp
}

func timeLayoutRFC3339() string {
	return "2006-01-02T15:04:05Z07:00"
}

// GetUploadRules returns public upload rules for user-side UI.
// @Summary Get upload rules
// @Tags public
// @Produce json
// @Success 200 {object} UploadRulePublicResponse
// @Router /api/upload-rules [get]
func (h *Handler) GetUploadRules(c *gin.Context) {
	runtime := h.loadUploadRuleRuntime()
	c.JSON(http.StatusOK, toUploadRulePublicResponse(runtime))
}

// GetAdminUploadRules returns editable upload rule config.
// @Summary Get upload rules (admin)
// @Tags admin
// @Produce json
// @Success 200 {object} UploadRuleAdminResponse
// @Router /api/admin/upload-rules [get]
func (h *Handler) GetAdminUploadRules(c *gin.Context) {
	row, err := h.loadUploadRuleSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load upload rules"})
		return
	}
	c.JSON(http.StatusOK, toUploadRuleAdminResponse(row))
}

// UpdateAdminUploadRules updates upload rules config.
// @Summary Update upload rules (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body UpdateUploadRuleAdminRequest true "update request"
// @Success 200 {object} UploadRuleAdminResponse
// @Router /api/admin/upload-rules [put]
func (h *Handler) UpdateAdminUploadRules(c *gin.Context) {
	var req UpdateUploadRuleAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	row, err := h.loadUploadRuleSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load upload rules"})
		return
	}

	updates := map[string]interface{}{}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.AutoAuditEnabled != nil {
		updates["auto_audit_enabled"] = *req.AutoAuditEnabled
	}
	if req.AutoActivateOnPass != nil {
		updates["auto_activate_on_pass"] = *req.AutoActivateOnPass
	}
	if req.MaxFileSizeBytes != nil {
		value := *req.MaxFileSizeBytes
		if value < 1024 || value > 100*1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "max_file_size_bytes must be within 1024..104857600"})
			return
		}
		updates["max_file_size_bytes"] = value
	}
	if req.MaxFilesPerCollection != nil {
		value := *req.MaxFilesPerCollection
		if value < 1 || value > 500 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "max_files_per_collection must be within 1..500"})
			return
		}
		updates["max_files_per_collection"] = value
	}
	if req.MaxFilesPerRequest != nil {
		value := *req.MaxFilesPerRequest
		if value < 1 || value > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "max_files_per_request must be within 1..100"})
			return
		}
		updates["max_files_per_request"] = value
	}
	if req.AllowedExtensions != nil {
		normalized := make([]string, 0, len(req.AllowedExtensions))
		seen := map[string]struct{}{}
		for _, item := range req.AllowedExtensions {
			ext := strings.ToLower(strings.TrimSpace(item))
			ext = strings.TrimPrefix(ext, ".")
			if ext == "" {
				continue
			}
			if _, ok := seen[ext]; ok {
				continue
			}
			seen[ext] = struct{}{}
			normalized = append(normalized, ext)
		}
		if len(normalized) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "allowed_extensions cannot be empty"})
			return
		}
		updates["allowed_extensions"] = strings.Join(normalized, ",")
	}
	if req.BlockedKeywords != nil {
		normalized := make([]string, 0, len(req.BlockedKeywords))
		seen := map[string]struct{}{}
		for _, item := range req.BlockedKeywords {
			keyword := strings.ToLower(strings.TrimSpace(item))
			if keyword == "" {
				continue
			}
			if _, ok := seen[keyword]; ok {
				continue
			}
			seen[keyword] = struct{}{}
			normalized = append(normalized, keyword)
		}
		updates["blocked_keywords"] = strings.Join(normalized, ",")
	}
	if req.ContentRulesText != nil {
		updates["content_rules"] = strings.TrimSpace(*req.ContentRulesText)
	}
	if req.ReferenceURL != nil {
		updates["reference_url"] = strings.TrimSpace(*req.ReferenceURL)
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, toUploadRuleAdminResponse(row))
		return
	}
	if adminID, ok := currentUserIDFromContext(c); ok && adminID > 0 {
		updates["updated_by"] = adminID
	}

	if err := h.db.Model(&models.UploadRuleSetting{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update upload rules"})
		return
	}

	updated, err := h.loadUploadRuleSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load upload rules"})
		return
	}
	c.JSON(http.StatusOK, toUploadRuleAdminResponse(updated))
}

func keywordHit(content string, keywords []string) (string, bool) {
	low := strings.ToLower(strings.TrimSpace(content))
	if low == "" || len(keywords) == 0 {
		return "", false
	}
	for _, keyword := range keywords {
		needle := strings.ToLower(strings.TrimSpace(keyword))
		if needle == "" {
			continue
		}
		if strings.Contains(low, needle) {
			return needle, true
		}
	}
	return "", false
}

func pickAuditStatus(runtime uploadRuleRuntime, title, fileName string) (status string, reason string) {
	if !runtime.AutoAuditEnabled {
		return "pending", "auto_audit_disabled"
	}
	if hit, ok := keywordHit(title+" "+fileName, runtime.BlockedKeywords); ok {
		return "pending", "keyword_hit:" + hit
	}
	if runtime.AutoActivateOnPass {
		return "active", "auto_pass"
	}
	return "pending", "needs_manual_review"
}

func (r uploadRuleRuntime) joinedAllowedExtensions() string {
	return strings.Join(r.AllowedExtensions, ",")
}

func (r uploadRuleRuntime) joinedBlockedKeywords() string {
	return strings.Join(r.BlockedKeywords, ",")
}

func (r uploadRuleRuntime) asDebugMap() map[string]string {
	return map[string]string{
		"enabled":                  strconv.FormatBool(r.Enabled),
		"auto_audit_enabled":       strconv.FormatBool(r.AutoAuditEnabled),
		"auto_activate_on_pass":    strconv.FormatBool(r.AutoActivateOnPass),
		"allowed_extensions":       r.joinedAllowedExtensions(),
		"blocked_keywords":         r.joinedBlockedKeywords(),
		"max_file_size_bytes":      strconv.FormatInt(r.MaxFileSizeBytes, 10),
		"max_files_per_collection": strconv.Itoa(r.MaxFilesPerCollection),
		"max_files_per_request":    strconv.Itoa(r.MaxFilesPerRequest),
	}
}
