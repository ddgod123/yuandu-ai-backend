package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"sort"
	"strings"

	"emoji/internal/models"
	"emoji/internal/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SiteFooterSelfMediaItemRequest struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Logo        string `json:"logo"`
	QRCode      string `json:"qr_code"`
	ProfileLink string `json:"profile_link"`
	Enabled     bool   `json:"enabled"`
	Sort        int    `json:"sort"`
}

type SiteFooterSelfMediaItemResponse struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Logo        string `json:"logo"`
	LogoURL     string `json:"logo_url"`
	QRCode      string `json:"qr_code"`
	QRCodeURL   string `json:"qr_code_url"`
	ProfileLink string `json:"profile_link"`
	Enabled     bool   `json:"enabled"`
	Sort        int    `json:"sort"`
}

type siteFooterSelfMediaItemStored struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Logo        string `json:"logo"`
	QRCode      string `json:"qr_code"`
	ProfileLink string `json:"profile_link"`
	Enabled     bool   `json:"enabled"`
	Sort        int    `json:"sort"`
}

type SiteFooterSettingRequest struct {
	SiteName             string                           `json:"site_name"`
	SiteDescription      string                           `json:"site_description"`
	ContactEmail         string                           `json:"contact_email"`
	ComplaintEmail       string                           `json:"complaint_email"`
	SelfMediaLogo        string                           `json:"self_media_logo"`
	SelfMediaQRCode      string                           `json:"self_media_qr_code"`
	SelfMediaItems       []SiteFooterSelfMediaItemRequest `json:"self_media_items"`
	ICPNumber            string                           `json:"icp_number"`
	ICPLink              string                           `json:"icp_link"`
	PublicSecurityNumber string                           `json:"public_security_number"`
	PublicSecurityLink   string                           `json:"public_security_link"`
	CopyrightText        string                           `json:"copyright_text"`
}

type SiteFooterSettingResponse struct {
	SiteName             string                            `json:"site_name"`
	SiteDescription      string                            `json:"site_description"`
	ContactEmail         string                            `json:"contact_email"`
	ComplaintEmail       string                            `json:"complaint_email"`
	SelfMediaLogo        string                            `json:"self_media_logo"`
	SelfMediaLogoURL     string                            `json:"self_media_logo_url"`
	SelfMediaQRCode      string                            `json:"self_media_qr_code"`
	SelfMediaQRCodeURL   string                            `json:"self_media_qr_code_url"`
	SelfMediaItems       []SiteFooterSelfMediaItemResponse `json:"self_media_items"`
	ICPNumber            string                            `json:"icp_number"`
	ICPLink              string                            `json:"icp_link"`
	PublicSecurityNumber string                            `json:"public_security_number"`
	PublicSecurityLink   string                            `json:"public_security_link"`
	CopyrightText        string                            `json:"copyright_text"`
	CreatedAt            string                            `json:"created_at,omitempty"`
	UpdatedAt            string                            `json:"updated_at,omitempty"`
}

func defaultSiteFooterSetting() models.SiteFooterSetting {
	return models.SiteFooterSetting{
		ID:                   1,
		SiteName:             "元都AI",
		SiteDescription:      "面向创作者与团队的 AI 视觉资产生产平台，提供视频转图、视觉内容生成与资产管理能力，让每次创作更快、更稳、更可控。",
		ContactEmail:         "3909356254@qq.com",
		ComplaintEmail:       "3909356254@qq.com",
		SelfMediaItems:       "[]",
		ICPNumber:            "ICP备案号：待补充",
		PublicSecurityNumber: "公安备案号：待补充",
		CopyrightText:        "元都AI · AI视觉资产生产平台. All rights reserved.",
	}
}

func defaultSelfMediaNameByKey(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "qq":
		return "QQ"
	case "wechat":
		return "微信"
	case "xiaohongshu":
		return "小红书"
	default:
		return "自媒体"
	}
}

func sanitizeSelfMediaKey(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, ch := range raw {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func defaultSelfMediaItems() []siteFooterSelfMediaItemStored {
	return []siteFooterSelfMediaItemStored{
		{Key: "qq", Name: "QQ", Enabled: false, Sort: 1},
		{Key: "wechat", Name: "微信", Enabled: false, Sort: 2},
		{Key: "xiaohongshu", Name: "小红书", Enabled: false, Sort: 3},
	}
}

func parseStoredSelfMediaItems(raw string) []siteFooterSelfMediaItemStored {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var items []siteFooterSelfMediaItemStored
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil
	}
	result := make([]siteFooterSelfMediaItemStored, 0, len(items))
	for idx, item := range items {
		key := sanitizeSelfMediaKey(item.Key)
		if key == "" {
			key = fmt.Sprintf("item_%d", idx+1)
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = defaultSelfMediaNameByKey(key)
		}
		sortValue := item.Sort
		if sortValue <= 0 {
			sortValue = idx + 1
		}
		result = append(result, siteFooterSelfMediaItemStored{
			Key:         key,
			Name:        name,
			Logo:        strings.TrimSpace(item.Logo),
			QRCode:      strings.TrimSpace(item.QRCode),
			ProfileLink: strings.TrimSpace(item.ProfileLink),
			Enabled:     item.Enabled,
			Sort:        sortValue,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Sort == result[j].Sort {
			return result[i].Key < result[j].Key
		}
		return result[i].Sort < result[j].Sort
	})
	return result
}

func legacySelfMediaItems(logo, qrCode string) []siteFooterSelfMediaItemStored {
	logo = strings.TrimSpace(logo)
	qrCode = strings.TrimSpace(qrCode)
	if logo == "" && qrCode == "" {
		return nil
	}
	return []siteFooterSelfMediaItemStored{
		{
			Key:     "qq",
			Name:    "QQ",
			Logo:    logo,
			QRCode:  qrCode,
			Enabled: true,
			Sort:    1,
		},
	}
}

func serializeSelfMediaItems(items []siteFooterSelfMediaItemStored) string {
	if len(items) == 0 {
		return "[]"
	}
	bytes, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

func normalizeSiteFooterSetting(setting models.SiteFooterSetting) models.SiteFooterSetting {
	defaultValue := defaultSiteFooterSetting()
	if strings.TrimSpace(setting.SiteName) == "" {
		setting.SiteName = defaultValue.SiteName
	}
	if strings.TrimSpace(setting.SiteDescription) == "" {
		setting.SiteDescription = defaultValue.SiteDescription
	}
	if strings.TrimSpace(setting.SelfMediaItems) == "" {
		setting.SelfMediaItems = defaultValue.SelfMediaItems
	}
	if strings.TrimSpace(setting.ICPNumber) == "" {
		setting.ICPNumber = defaultValue.ICPNumber
	}
	if strings.TrimSpace(setting.PublicSecurityNumber) == "" {
		setting.PublicSecurityNumber = defaultValue.PublicSecurityNumber
	}
	if strings.TrimSpace(setting.CopyrightText) == "" {
		setting.CopyrightText = defaultValue.CopyrightText
	}
	return setting
}

func pickPrimarySelfMedia(items []siteFooterSelfMediaItemStored) *siteFooterSelfMediaItemStored {
	for i := range items {
		if !items[i].Enabled {
			continue
		}
		if strings.TrimSpace(items[i].Logo) != "" || strings.TrimSpace(items[i].QRCode) != "" {
			return &items[i]
		}
	}
	for i := range items {
		if strings.TrimSpace(items[i].Logo) != "" || strings.TrimSpace(items[i].QRCode) != "" {
			return &items[i]
		}
	}
	return nil
}

func buildSelfMediaResponseItems(items []siteFooterSelfMediaItemStored, qiniuClient *storage.QiniuClient) []SiteFooterSelfMediaItemResponse {
	result := make([]SiteFooterSelfMediaItemResponse, 0, len(items))
	for _, item := range items {
		logo := strings.TrimSpace(item.Logo)
		qrCode := strings.TrimSpace(item.QRCode)
		result = append(result, SiteFooterSelfMediaItemResponse{
			Key:         item.Key,
			Name:        item.Name,
			Logo:        logo,
			LogoURL:     resolvePreviewURL(logo, qiniuClient),
			QRCode:      qrCode,
			QRCodeURL:   resolvePreviewURL(qrCode, qiniuClient),
			ProfileLink: strings.TrimSpace(item.ProfileLink),
			Enabled:     item.Enabled,
			Sort:        item.Sort,
		})
	}
	return result
}

func toSiteFooterSettingResponse(setting models.SiteFooterSetting, qiniuClient *storage.QiniuClient, withMeta bool) SiteFooterSettingResponse {
	items := parseStoredSelfMediaItems(setting.SelfMediaItems)
	if len(items) == 0 {
		items = legacySelfMediaItems(setting.SelfMediaLogo, setting.SelfMediaQRCode)
	}
	if len(items) == 0 {
		items = defaultSelfMediaItems()
	}

	selfMediaLogo := strings.TrimSpace(setting.SelfMediaLogo)
	selfMediaQRCode := strings.TrimSpace(setting.SelfMediaQRCode)
	if primary := pickPrimarySelfMedia(items); primary != nil {
		if selfMediaLogo == "" {
			selfMediaLogo = strings.TrimSpace(primary.Logo)
		}
		if selfMediaQRCode == "" {
			selfMediaQRCode = strings.TrimSpace(primary.QRCode)
		}
	}

	resp := SiteFooterSettingResponse{
		SiteName:             setting.SiteName,
		SiteDescription:      setting.SiteDescription,
		ContactEmail:         setting.ContactEmail,
		ComplaintEmail:       setting.ComplaintEmail,
		SelfMediaLogo:        selfMediaLogo,
		SelfMediaLogoURL:     resolvePreviewURL(selfMediaLogo, qiniuClient),
		SelfMediaQRCode:      selfMediaQRCode,
		SelfMediaQRCodeURL:   resolvePreviewURL(selfMediaQRCode, qiniuClient),
		SelfMediaItems:       buildSelfMediaResponseItems(items, qiniuClient),
		ICPNumber:            setting.ICPNumber,
		ICPLink:              setting.ICPLink,
		PublicSecurityNumber: setting.PublicSecurityNumber,
		PublicSecurityLink:   setting.PublicSecurityLink,
		CopyrightText:        setting.CopyrightText,
	}
	if withMeta {
		if !setting.CreatedAt.IsZero() {
			resp.CreatedAt = setting.CreatedAt.Format("2006-01-02 15:04:05")
		}
		if !setting.UpdatedAt.IsZero() {
			resp.UpdatedAt = setting.UpdatedAt.Format("2006-01-02 15:04:05")
		}
	}
	return resp
}

func (h *Handler) loadSiteFooterSetting() (models.SiteFooterSetting, error) {
	var setting models.SiteFooterSetting
	if err := h.db.First(&setting, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return defaultSiteFooterSetting(), nil
		}
		return models.SiteFooterSetting{}, err
	}
	return normalizeSiteFooterSetting(setting), nil
}

func validateEmailIfPresent(value string) bool {
	if value == "" {
		return true
	}
	_, err := mail.ParseAddress(value)
	return err == nil
}

func normalizeOptionalLink(value string) (string, bool) {
	if value == "" {
		return "", true
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return value, true
	}
	return "", false
}

func normalizeOptionalAsset(value string, qiniuClient *storage.QiniuClient) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}

	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		if key, ok := extractQiniuObjectKey(value, qiniuClient); ok {
			return key, true
		}
		return value, true
	}
	if strings.Contains(lower, "://") {
		return "", false
	}

	key := strings.TrimLeft(value, "/")
	return key, key != ""
}

func normalizeSelfMediaItemsForRequest(items []SiteFooterSelfMediaItemRequest, qiniuClient *storage.QiniuClient) ([]siteFooterSelfMediaItemStored, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if len(items) > 20 {
		return nil, errors.New("self_media_items exceeds limit")
	}

	usedKeys := make(map[string]int)
	result := make([]siteFooterSelfMediaItemStored, 0, len(items))
	for idx, item := range items {
		key := sanitizeSelfMediaKey(item.Key)
		if key == "" {
			key = sanitizeSelfMediaKey(item.Name)
		}
		if key == "" {
			key = fmt.Sprintf("item_%d", idx+1)
		}
		usedKeys[key]++
		if usedKeys[key] > 1 {
			key = fmt.Sprintf("%s_%d", key, usedKeys[key])
		}

		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = defaultSelfMediaNameByKey(key)
		}

		logo, ok := normalizeOptionalAsset(item.Logo, qiniuClient)
		if !ok {
			return nil, fmt.Errorf("self_media_items[%d].logo must be http(s) url or object key", idx)
		}
		qrCode, ok := normalizeOptionalAsset(item.QRCode, qiniuClient)
		if !ok {
			return nil, fmt.Errorf("self_media_items[%d].qr_code must be http(s) url or object key", idx)
		}
		profileLink, ok := normalizeOptionalLink(strings.TrimSpace(item.ProfileLink))
		if !ok {
			return nil, fmt.Errorf("self_media_items[%d].profile_link must start with http:// or https://", idx)
		}

		sortValue := item.Sort
		if sortValue <= 0 {
			sortValue = idx + 1
		}

		result = append(result, siteFooterSelfMediaItemStored{
			Key:         key,
			Name:        name,
			Logo:        logo,
			QRCode:      qrCode,
			ProfileLink: profileLink,
			Enabled:     item.Enabled,
			Sort:        sortValue,
		})
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Sort == result[j].Sort {
			return result[i].Key < result[j].Key
		}
		return result[i].Sort < result[j].Sort
	})
	return result, nil
}

func mergeLegacySelfMediaWithItems(items []siteFooterSelfMediaItemStored, logo, qrCode string) []siteFooterSelfMediaItemStored {
	if len(items) > 0 {
		return items
	}
	legacy := legacySelfMediaItems(logo, qrCode)
	if len(legacy) > 0 {
		return legacy
	}
	return defaultSelfMediaItems()
}

// GetSiteFooterSetting godoc
// @Summary Get public site footer setting
// @Tags public
// @Produce json
// @Success 200 {object} SiteFooterSettingResponse
// @Router /api/site-settings/footer [get]
func (h *Handler) GetSiteFooterSetting(c *gin.Context) {
	setting, err := h.loadSiteFooterSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toSiteFooterSettingResponse(setting, h.qiniu, false))
}

// GetAdminSiteFooterSetting godoc
// @Summary Get footer setting for admin edit
// @Tags admin
// @Produce json
// @Success 200 {object} SiteFooterSettingResponse
// @Router /api/admin/site-settings/footer [get]
func (h *Handler) GetAdminSiteFooterSetting(c *gin.Context) {
	setting, err := h.loadSiteFooterSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toSiteFooterSettingResponse(setting, h.qiniu, true))
}

// UpdateAdminSiteFooterSetting godoc
// @Summary Update footer setting
// @Tags admin
// @Accept json
// @Produce json
// @Param body body SiteFooterSettingRequest true "footer setting"
// @Success 200 {object} SiteFooterSettingResponse
// @Router /api/admin/site-settings/footer [put]
func (h *Handler) UpdateAdminSiteFooterSetting(c *gin.Context) {
	var req SiteFooterSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	siteName := strings.TrimSpace(req.SiteName)
	if siteName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "site_name required"})
		return
	}

	contactEmail := strings.TrimSpace(req.ContactEmail)
	if !validateEmailIfPresent(contactEmail) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contact_email"})
		return
	}

	complaintEmail := strings.TrimSpace(req.ComplaintEmail)
	if !validateEmailIfPresent(complaintEmail) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid complaint_email"})
		return
	}

	selfMediaLogo, ok := normalizeOptionalAsset(req.SelfMediaLogo, h.qiniu)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "self_media_logo must be http(s) url or object key"})
		return
	}
	selfMediaQRCode, ok := normalizeOptionalAsset(req.SelfMediaQRCode, h.qiniu)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "self_media_qr_code must be http(s) url or object key"})
		return
	}
	selfMediaItems, err := normalizeSelfMediaItemsForRequest(req.SelfMediaItems, h.qiniu)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	selfMediaItems = mergeLegacySelfMediaWithItems(selfMediaItems, selfMediaLogo, selfMediaQRCode)

	if primary := pickPrimarySelfMedia(selfMediaItems); primary != nil {
		if strings.TrimSpace(selfMediaLogo) == "" {
			selfMediaLogo = strings.TrimSpace(primary.Logo)
		}
		if strings.TrimSpace(selfMediaQRCode) == "" {
			selfMediaQRCode = strings.TrimSpace(primary.QRCode)
		}
	}

	icpLink, ok := normalizeOptionalLink(strings.TrimSpace(req.ICPLink))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "icp_link must start with http:// or https://"})
		return
	}
	publicSecurityLink, ok := normalizeOptionalLink(strings.TrimSpace(req.PublicSecurityLink))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "public_security_link must start with http:// or https://"})
		return
	}

	payload := models.SiteFooterSetting{
		ID:                   1,
		SiteName:             siteName,
		SiteDescription:      strings.TrimSpace(req.SiteDescription),
		ContactEmail:         contactEmail,
		ComplaintEmail:       complaintEmail,
		SelfMediaLogo:        selfMediaLogo,
		SelfMediaQRCode:      selfMediaQRCode,
		SelfMediaItems:       serializeSelfMediaItems(selfMediaItems),
		ICPNumber:            strings.TrimSpace(req.ICPNumber),
		ICPLink:              icpLink,
		PublicSecurityNumber: strings.TrimSpace(req.PublicSecurityNumber),
		PublicSecurityLink:   publicSecurityLink,
		CopyrightText:        strings.TrimSpace(req.CopyrightText),
	}

	var saved models.SiteFooterSetting
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var current models.SiteFooterSetting
		err := tx.First(&current, 1).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&payload).Error; err != nil {
				return err
			}
			saved = payload
			return nil
		}

		current.SiteName = payload.SiteName
		current.SiteDescription = payload.SiteDescription
		current.ContactEmail = payload.ContactEmail
		current.ComplaintEmail = payload.ComplaintEmail
		current.SelfMediaLogo = payload.SelfMediaLogo
		current.SelfMediaQRCode = payload.SelfMediaQRCode
		current.SelfMediaItems = payload.SelfMediaItems
		current.ICPNumber = payload.ICPNumber
		current.ICPLink = payload.ICPLink
		current.PublicSecurityNumber = payload.PublicSecurityNumber
		current.PublicSecurityLink = payload.PublicSecurityLink
		current.CopyrightText = payload.CopyrightText
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		saved = current
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, toSiteFooterSettingResponse(normalizeSiteFooterSetting(saved), h.qiniu, true))
}
