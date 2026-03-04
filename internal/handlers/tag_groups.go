package handlers

import (
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

type TagGroupRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Sort        int    `json:"sort"`
	Status      string `json:"status"`
}

type TagGroupResponse struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Sort        int       `json:"sort"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListTagGroups godoc
// @Summary List tag groups
// @Tags admin
// @Produce json
// @Success 200 {array} TagGroupResponse
// @Router /api/admin/tag-groups [get]
func (h *Handler) ListTagGroups(c *gin.Context) {
	var groups []models.TagGroup
	if err := h.db.Order("sort ASC, id ASC").Find(&groups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := make([]TagGroupResponse, 0, len(groups))
	for _, g := range groups {
		resp = append(resp, TagGroupResponse{
			ID:          g.ID,
			Name:        g.Name,
			Slug:        g.Slug,
			Description: g.Description,
			Sort:        g.Sort,
			Status:      g.Status,
			CreatedAt:   g.CreatedAt,
			UpdatedAt:   g.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, resp)
}

// CreateTagGroup godoc
// @Summary Create tag group
// @Tags admin
// @Accept json
// @Produce json
// @Param body body TagGroupRequest true "tag group request"
// @Success 200 {object} TagGroupResponse
// @Router /api/admin/tag-groups [post]
func (h *Handler) CreateTagGroup(c *gin.Context) {
	var req TagGroupRequest
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
	slug = ensureUniqueTagGroupSlug(h.db, slug, 0)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug required"})
		return
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "active"
	}

	group := models.TagGroup{
		Name:        name,
		Slug:        truncateString(slug, 64),
		Description: strings.TrimSpace(req.Description),
		Sort:        req.Sort,
		Status:      status,
	}
	if err := h.db.Create(&group).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, TagGroupResponse{
		ID:          group.ID,
		Name:        group.Name,
		Slug:        group.Slug,
		Description: group.Description,
		Sort:        group.Sort,
		Status:      group.Status,
		CreatedAt:   group.CreatedAt,
		UpdatedAt:   group.UpdatedAt,
	})
}

// UpdateTagGroup godoc
// @Summary Update tag group
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "group id"
// @Param body body TagGroupRequest true "tag group request"
// @Success 200 {object} TagGroupResponse
// @Router /api/admin/tag-groups/{id} [put]
func (h *Handler) UpdateTagGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var group models.TagGroup
	if err := h.db.First(&group, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req TagGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if name := strings.TrimSpace(req.Name); name != "" {
		group.Name = name
	}
	if slug := strings.TrimSpace(req.Slug); slug != "" && slug != group.Slug {
		group.Slug = truncateString(ensureUniqueTagGroupSlug(h.db, slug, group.ID), 64)
	}
	group.Description = strings.TrimSpace(req.Description)
	group.Sort = req.Sort
	if status := strings.TrimSpace(req.Status); status != "" {
		group.Status = status
	}

	if err := h.db.Save(&group).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, TagGroupResponse{
		ID:          group.ID,
		Name:        group.Name,
		Slug:        group.Slug,
		Description: group.Description,
		Sort:        group.Sort,
		Status:      group.Status,
		CreatedAt:   group.CreatedAt,
		UpdatedAt:   group.UpdatedAt,
	})
}

// DeleteTagGroup godoc
// @Summary Delete tag group
// @Tags admin
// @Produce json
// @Param id path int true "group id"
// @Success 200 {object} MessageResponse
// @Router /api/admin/tag-groups/{id} [delete]
func (h *Handler) DeleteTagGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var count int64
	_ = h.db.Model(&models.Tag{}).Where("tag_group_id = ?", id).Count(&count).Error
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group not empty"})
		return
	}
	if err := h.db.Delete(&models.TagGroup{}, id).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "deleted"})
}

func ensureUniqueTagGroupSlug(db *gorm.DB, slug string, excludeID uint64) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	base := truncateString(slug, 64)
	candidate := base
	counter := 1
	for {
		var count int64
		query := db.Model(&models.TagGroup{}).Where("slug = ?", candidate)
		if excludeID > 0 {
			query = query.Where("id <> ?", excludeID)
		}
		if err := query.Count(&count).Error; err != nil {
			return candidate
		}
		if count == 0 {
			return candidate
		}
		counter++
		suffix := fmt.Sprintf("-%d", counter)
		candidate = truncateString(base, 64-len(suffix)) + suffix
		if candidate == base {
			return candidate
		}
	}
}
