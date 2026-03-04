package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type IPRequest struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	CoverURL    string  `json:"cover_url"`
	CategoryID  *uint64 `json:"category_id"`
	Description string  `json:"description"`
	Sort        int     `json:"sort"`
	Status      string  `json:"status"`
}

type IPResponse struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	CoverURL    string    `json:"cover_url"`
	CategoryID  *uint64   `json:"category_id,omitempty"`
	Description string    `json:"description"`
	Sort        int       `json:"sort"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListIPs godoc
// @Summary List IPs
// @Tags public
// @Produce json
// @Param keyword query string false "keyword"
// @Param category_id query int false "category id"
// @Success 200 {array} IPResponse
// @Router /api/ips [get]
func (h *Handler) ListIPs(c *gin.Context) {
	var ips []models.IP
	query := h.db.Model(&models.IP{})
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		keyword = strings.ToLower(keyword)
		query = query.Where("LOWER(name) LIKE ? OR LOWER(slug) LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if categoryID := strings.TrimSpace(c.Query("category_id")); categoryID != "" {
		if cid, err := strconv.ParseUint(categoryID, 10, 64); err == nil && cid > 0 {
			query = query.Where("category_id = ?", cid)
		}
	}
	if err := query.Order("sort ASC, id ASC").Find(&ips).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := make([]IPResponse, 0, len(ips))
	for _, ip := range ips {
		resp = append(resp, IPResponse{
			ID:          ip.ID,
			Name:        ip.Name,
			Slug:        ip.Slug,
			CoverURL:    ip.CoverURL,
			CategoryID:  ip.CategoryID,
			Description: ip.Description,
			Sort:        ip.Sort,
			Status:      ip.Status,
			CreatedAt:   ip.CreatedAt,
			UpdatedAt:   ip.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, resp)
}

// GetIP godoc
// @Summary Get IP
// @Tags public
// @Produce json
// @Param id path int true "ip id"
// @Success 200 {object} IPResponse
// @Router /api/ips/{id} [get]
func (h *Handler) GetIP(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var ip models.IP
	if err := h.db.First(&ip, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, IPResponse{
		ID:          ip.ID,
		Name:        ip.Name,
		Slug:        ip.Slug,
		CoverURL:    ip.CoverURL,
		CategoryID:  ip.CategoryID,
		Description: ip.Description,
		Sort:        ip.Sort,
		Status:      ip.Status,
		CreatedAt:   ip.CreatedAt,
		UpdatedAt:   ip.UpdatedAt,
	})
}

// ListAdminIPs godoc
// @Summary List IPs (admin)
// @Tags admin
// @Produce json
// @Param keyword query string false "keyword"
// @Param category_id query int false "category id"
// @Success 200 {array} IPResponse
// @Router /api/admin/ips [get]
func (h *Handler) ListAdminIPs(c *gin.Context) {
	h.ListIPs(c)
}

// CreateIP godoc
// @Summary Create IP
// @Tags admin
// @Accept json
// @Produce json
// @Param body body IPRequest true "ip request"
// @Success 200 {object} IPResponse
// @Router /api/admin/ips [post]
func (h *Handler) CreateIP(c *gin.Context) {
	var req IPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugifyTag(name)
	}
	if slug == "" {
		slug = name
	}
	slug = ensureUniqueIPSlug(h.db, slug, 0)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug required"})
		return
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "active"
	}

	ip := models.IP{
		Name:        name,
		Slug:        truncateString(slug, 128),
		CoverURL:    strings.TrimSpace(req.CoverURL),
		CategoryID:  req.CategoryID,
		Description: strings.TrimSpace(req.Description),
		Sort:        req.Sort,
		Status:      status,
	}
	if err := h.db.Create(&ip).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, IPResponse{
		ID:          ip.ID,
		Name:        ip.Name,
		Slug:        ip.Slug,
		CoverURL:    ip.CoverURL,
		CategoryID:  ip.CategoryID,
		Description: ip.Description,
		Sort:        ip.Sort,
		Status:      ip.Status,
		CreatedAt:   ip.CreatedAt,
		UpdatedAt:   ip.UpdatedAt,
	})
}

// UpdateIP godoc
// @Summary Update IP
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "ip id"
// @Param body body IPRequest true "ip request"
// @Success 200 {object} IPResponse
// @Router /api/admin/ips/{id} [put]
func (h *Handler) UpdateIP(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var ip models.IP
	if err := h.db.First(&ip, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req IPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		ip.Name = name
	}
	if slug := strings.TrimSpace(req.Slug); slug != "" && slug != ip.Slug {
		ip.Slug = truncateString(ensureUniqueIPSlug(h.db, slug, ip.ID), 128)
	}
	if cover := strings.TrimSpace(req.CoverURL); cover != "" || req.CoverURL == "" {
		ip.CoverURL = strings.TrimSpace(req.CoverURL)
	}
	ip.CategoryID = req.CategoryID
	ip.Description = strings.TrimSpace(req.Description)
	ip.Sort = req.Sort
	if status := strings.TrimSpace(req.Status); status != "" {
		ip.Status = status
	}
	if err := h.db.Save(&ip).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, IPResponse{
		ID:          ip.ID,
		Name:        ip.Name,
		Slug:        ip.Slug,
		CoverURL:    ip.CoverURL,
		CategoryID:  ip.CategoryID,
		Description: ip.Description,
		Sort:        ip.Sort,
		Status:      ip.Status,
		CreatedAt:   ip.CreatedAt,
		UpdatedAt:   ip.UpdatedAt,
	})
}

// DeleteIP godoc
// @Summary Delete IP
// @Tags admin
// @Produce json
// @Param id path int true "ip id"
// @Success 200 {object} MessageResponse
// @Router /api/admin/ips/{id} [delete]
func (h *Handler) DeleteIP(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var count int64
	_ = h.db.Model(&models.Collection{}).Where("ip_id = ?", id).Count(&count).Error
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip in use"})
		return
	}
	if err := h.db.Delete(&models.IP{}, id).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "deleted"})
}

func ensureUniqueIPSlug(db *gorm.DB, slug string, excludeID uint64) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	base := truncateString(slug, 128)
	candidate := base
	counter := 1
	for {
		var count int64
		query := db.Model(&models.IP{}).Where("slug = ?", candidate)
		if excludeID > 0 {
			query = query.Where("id <> ?", excludeID)
		}
		if err := query.Count(&count).Error; err == nil && count == 0 {
			return candidate
		}
		candidate = truncateString(base, 120) + "-" + strconv.Itoa(counter)
		counter++
		if counter > 1000 {
			return ""
		}
	}
}
