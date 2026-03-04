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

type ThemeRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Sort        int    `json:"sort"`
	Status      string `json:"status"`
}

type ThemeResponse struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Sort        int       `json:"sort"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListThemes godoc
// @Summary List themes
// @Tags admin
// @Produce json
// @Param keyword query string false "keyword"
// @Success 200 {array} ThemeResponse
// @Router /api/admin/themes [get]
func (h *Handler) ListThemes(c *gin.Context) {
	var themes []models.Theme
	query := h.db
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		keyword = strings.ToLower(keyword)
		query = query.Where("LOWER(name) LIKE ? OR LOWER(slug) LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if err := query.Order("sort ASC, id ASC").Find(&themes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := make([]ThemeResponse, 0, len(themes))
	for _, theme := range themes {
		resp = append(resp, ThemeResponse{
			ID:          theme.ID,
			Name:        theme.Name,
			Slug:        theme.Slug,
			Description: theme.Description,
			Sort:        theme.Sort,
			Status:      theme.Status,
			CreatedAt:   theme.CreatedAt,
			UpdatedAt:   theme.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, resp)
}

// CreateTheme godoc
// @Summary Create theme
// @Tags admin
// @Accept json
// @Produce json
// @Param body body ThemeRequest true "theme request"
// @Success 200 {object} ThemeResponse
// @Router /api/admin/themes [post]
func (h *Handler) CreateTheme(c *gin.Context) {
	var req ThemeRequest
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
		slug = slugifyTheme(name)
	}
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug required"})
		return
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "active"
	}

	theme := models.Theme{
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(req.Description),
		Sort:        req.Sort,
		Status:      status,
	}

	if err := h.db.Create(&theme).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ThemeResponse{
		ID:          theme.ID,
		Name:        theme.Name,
		Slug:        theme.Slug,
		Description: theme.Description,
		Sort:        theme.Sort,
		Status:      theme.Status,
		CreatedAt:   theme.CreatedAt,
		UpdatedAt:   theme.UpdatedAt,
	})
}

// UpdateTheme godoc
// @Summary Update theme
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "theme id"
// @Param body body ThemeRequest true "theme request"
// @Success 200 {object} ThemeResponse
// @Router /api/admin/themes/{id} [put]
func (h *Handler) UpdateTheme(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var theme models.Theme
	if err := h.db.First(&theme, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req ThemeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if slug := strings.TrimSpace(req.Slug); slug != "" && slug != theme.Slug {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug immutable"})
		return
	}

	if name := strings.TrimSpace(req.Name); name != "" {
		theme.Name = name
	}
	theme.Description = strings.TrimSpace(req.Description)
	theme.Sort = req.Sort
	if status := strings.TrimSpace(req.Status); status != "" {
		theme.Status = status
	}

	if err := h.db.Save(&theme).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ThemeResponse{
		ID:          theme.ID,
		Name:        theme.Name,
		Slug:        theme.Slug,
		Description: theme.Description,
		Sort:        theme.Sort,
		Status:      theme.Status,
		CreatedAt:   theme.CreatedAt,
		UpdatedAt:   theme.UpdatedAt,
	})
}

// DeleteTheme godoc
// @Summary Delete theme
// @Tags admin
// @Produce json
// @Param id path int true "theme id"
// @Success 200 {object} MessageResponse
// @Router /api/admin/themes/{id} [delete]
func (h *Handler) DeleteTheme(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.db.Delete(&models.Theme{}, id).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "deleted"})
}

func slugifyTheme(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == '_' || r == ' ' {
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
