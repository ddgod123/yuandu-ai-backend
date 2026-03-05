package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SiteFooterSettingRequest struct {
	SiteName             string `json:"site_name"`
	SiteDescription      string `json:"site_description"`
	ContactEmail         string `json:"contact_email"`
	ComplaintEmail       string `json:"complaint_email"`
	ICPNumber            string `json:"icp_number"`
	ICPLink              string `json:"icp_link"`
	PublicSecurityNumber string `json:"public_security_number"`
	PublicSecurityLink   string `json:"public_security_link"`
	CopyrightText        string `json:"copyright_text"`
}

type SiteFooterSettingResponse struct {
	SiteName             string `json:"site_name"`
	SiteDescription      string `json:"site_description"`
	ContactEmail         string `json:"contact_email"`
	ComplaintEmail       string `json:"complaint_email"`
	ICPNumber            string `json:"icp_number"`
	ICPLink              string `json:"icp_link"`
	PublicSecurityNumber string `json:"public_security_number"`
	PublicSecurityLink   string `json:"public_security_link"`
	CopyrightText        string `json:"copyright_text"`
	CreatedAt            string `json:"created_at,omitempty"`
	UpdatedAt            string `json:"updated_at,omitempty"`
}

func defaultSiteFooterSetting() models.SiteFooterSetting {
	return models.SiteFooterSetting{
		ID:                   1,
		SiteName:             "表情包档案馆",
		SiteDescription:      "致力于收集、整理和分享互联网表情包资源。本站提供合集浏览、下载与收藏功能，服务于个人非商业交流场景。",
		ContactEmail:         "contact@emoji-archive.com",
		ComplaintEmail:       "contact@emoji-archive.com",
		ICPNumber:            "ICP备案号：待补充",
		PublicSecurityNumber: "公安备案号：待补充",
		CopyrightText:        "表情包档案馆. All rights reserved.",
	}
}

func normalizeSiteFooterSetting(setting models.SiteFooterSetting) models.SiteFooterSetting {
	defaultValue := defaultSiteFooterSetting()
	if strings.TrimSpace(setting.SiteName) == "" {
		setting.SiteName = defaultValue.SiteName
	}
	if strings.TrimSpace(setting.SiteDescription) == "" {
		setting.SiteDescription = defaultValue.SiteDescription
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

func toSiteFooterSettingResponse(setting models.SiteFooterSetting, withMeta bool) SiteFooterSettingResponse {
	resp := SiteFooterSettingResponse{
		SiteName:             setting.SiteName,
		SiteDescription:      setting.SiteDescription,
		ContactEmail:         setting.ContactEmail,
		ComplaintEmail:       setting.ComplaintEmail,
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
	c.JSON(http.StatusOK, toSiteFooterSettingResponse(setting, false))
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
	c.JSON(http.StatusOK, toSiteFooterSettingResponse(setting, true))
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
		ICPNumber:            strings.TrimSpace(req.ICPNumber),
		ICPLink:              icpLink,
		PublicSecurityNumber: strings.TrimSpace(req.PublicSecurityNumber),
		PublicSecurityLink:   publicSecurityLink,
		CopyrightText:        strings.TrimSpace(req.CopyrightText),
	}

	var saved models.SiteFooterSetting
	err := h.db.Transaction(func(tx *gorm.DB) error {
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

	c.JSON(http.StatusOK, toSiteFooterSettingResponse(normalizeSiteFooterSetting(saved), true))
}
