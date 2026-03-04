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

type TagRequest struct {
	Name    string  `json:"name"`
	Slug    string  `json:"slug"`
	GroupID *uint64 `json:"group_id"`
	Sort    int     `json:"sort"`
	Status  string  `json:"status"`
}

type TagResponse struct {
	ID              uint64    `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	GroupID         *uint64   `json:"group_id,omitempty"`
	GroupName       string    `json:"group_name,omitempty"`
	GroupSlug       string    `json:"group_slug,omitempty"`
	Sort            int       `json:"sort"`
	Status          string    `json:"status"`
	CollectionCount int64     `json:"collection_count,omitempty"`
	EmojiCount      int64     `json:"emoji_count,omitempty"`
	UsageCount      int64     `json:"usage_count,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type TagListResponse struct {
	Items []TagResponse `json:"items"`
	Total int64         `json:"total"`
}

// ListTags godoc
// @Summary List tags
// @Tags admin
// @Produce json
// @Param keyword query string false "keyword"
// @Param page query int false "page"
// @Param page_size query int false "page size"
// @Success 200 {array} TagResponse
// @Router /api/admin/tags [get]
func (h *Handler) ListTags(c *gin.Context) {
	var tags []models.Tag
	query := h.db.Model(&models.Tag{})
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		keyword = strings.ToLower(keyword)
		query = query.Where("LOWER(name) LIKE ? OR LOWER(slug) LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if groupID := strings.TrimSpace(c.Query("group_id")); groupID != "" {
		if gid, err := strconv.ParseUint(groupID, 10, 64); err == nil {
			query = query.Where("tag_group_id = ?", gid)
		}
	}
	pageStr := strings.TrimSpace(c.Query("page"))
	pageSizeStr := strings.TrimSpace(c.Query("page_size"))
	usePagination := pageStr != "" || pageSizeStr != ""
	page := 1
	pageSize := 20
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}
	if pageSize > 200 {
		pageSize = 200
	}

	var total int64
	if usePagination {
		if err := query.Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		offset := (page - 1) * pageSize
		if err := query.Order("sort ASC, id DESC").Limit(pageSize).Offset(offset).Find(&tags).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		if err := query.Order("sort ASC, id DESC").Find(&tags).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	groupMap := map[uint64]models.TagGroup{}
	collectionCountMap := map[uint64]int64{}
	emojiCountMap := map[uint64]int64{}
	if len(tags) > 0 {
		groupIDs := make([]uint64, 0)
		seen := map[uint64]struct{}{}
		tagIDs := make([]uint64, 0, len(tags))
		for _, t := range tags {
			tagIDs = append(tagIDs, t.ID)
			if t.GroupID == nil || *t.GroupID == 0 {
				continue
			}
			if _, ok := seen[*t.GroupID]; ok {
				continue
			}
			seen[*t.GroupID] = struct{}{}
			groupIDs = append(groupIDs, *t.GroupID)
		}
		if len(groupIDs) > 0 {
			var groups []models.TagGroup
			if err := h.db.Where("id IN ?", groupIDs).Find(&groups).Error; err == nil {
				for _, g := range groups {
					groupMap[g.ID] = g
				}
			}
		}

		type countRow struct {
			TagID uint64 `gorm:"column:tag_id"`
			Count int64  `gorm:"column:count"`
		}
		if len(tagIDs) > 0 {
			var collectionRows []countRow
			if err := h.db.Model(&models.CollectionTag{}).
				Select("tag_id, COUNT(*) as count").
				Where("tag_id IN ?", tagIDs).
				Group("tag_id").
				Scan(&collectionRows).Error; err == nil {
				for _, row := range collectionRows {
					collectionCountMap[row.TagID] = row.Count
				}
			}

			var emojiRows []countRow
			if err := h.db.Model(&models.EmojiTag{}).
				Select("tag_id, COUNT(*) as count").
				Where("tag_id IN ?", tagIDs).
				Group("tag_id").
				Scan(&emojiRows).Error; err == nil {
				for _, row := range emojiRows {
					emojiCountMap[row.TagID] = row.Count
				}
			}
		}
	}

	resp := make([]TagResponse, 0, len(tags))
	for _, t := range tags {
		var groupName string
		var groupSlug string
		if t.GroupID != nil {
			if g, ok := groupMap[*t.GroupID]; ok {
				groupName = g.Name
				groupSlug = g.Slug
			}
		}
		collectionCount := collectionCountMap[t.ID]
		emojiCount := emojiCountMap[t.ID]
		resp = append(resp, TagResponse{
			ID:              t.ID,
			Name:            t.Name,
			Slug:            t.Slug,
			GroupID:         t.GroupID,
			GroupName:       groupName,
			GroupSlug:       groupSlug,
			Sort:            t.Sort,
			Status:          t.Status,
			CollectionCount: collectionCount,
			EmojiCount:      emojiCount,
			UsageCount:      collectionCount + emojiCount,
			CreatedAt:       t.CreatedAt,
			UpdatedAt:       t.UpdatedAt,
		})
	}
	if usePagination {
		c.JSON(http.StatusOK, TagListResponse{Items: resp, Total: total})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// CreateTag godoc
// @Summary Create tag
// @Tags admin
// @Accept json
// @Produce json
// @Param body body TagRequest true "tag request"
// @Success 200 {object} TagResponse
// @Router /api/admin/tags [post]
func (h *Handler) CreateTag(c *gin.Context) {
	var req TagRequest
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
	slug = ensureUniqueTagSlug(h.db, slug, 0)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug required"})
		return
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "active"
	}

	tag := models.Tag{
		Name:    name,
		Slug:    truncateString(slug, 64),
		GroupID: req.GroupID,
		Sort:    req.Sort,
		Status:  status,
	}
	if err := h.db.Create(&tag).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var groupName string
	var groupSlug string
	if tag.GroupID != nil {
		var group models.TagGroup
		if err := h.db.First(&group, *tag.GroupID).Error; err == nil {
			groupName = group.Name
			groupSlug = group.Slug
		}
	}

	c.JSON(http.StatusOK, TagResponse{
		ID:        tag.ID,
		Name:      tag.Name,
		Slug:      tag.Slug,
		GroupID:   tag.GroupID,
		GroupName: groupName,
		GroupSlug: groupSlug,
		Sort:      tag.Sort,
		Status:    tag.Status,
		CreatedAt: tag.CreatedAt,
		UpdatedAt: tag.UpdatedAt,
	})
}

// UpdateTag godoc
// @Summary Update tag
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "tag id"
// @Param body body TagRequest true "tag request"
// @Success 200 {object} TagResponse
// @Router /api/admin/tags/{id} [put]
func (h *Handler) UpdateTag(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var tag models.Tag
	if err := h.db.First(&tag, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req TagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if name := strings.TrimSpace(req.Name); name != "" {
		tag.Name = name
	}
	if slug := strings.TrimSpace(req.Slug); slug != "" {
		tag.Slug = truncateString(ensureUniqueTagSlug(h.db, slug, tag.ID), 64)
	}
	tag.Sort = req.Sort
	if status := strings.TrimSpace(req.Status); status != "" {
		tag.Status = status
	}
	if req.GroupID != nil {
		if *req.GroupID == 0 {
			tag.GroupID = nil
		} else {
			tag.GroupID = req.GroupID
		}
	}

	if err := h.db.Save(&tag).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var groupName string
	var groupSlug string
	if tag.GroupID != nil {
		var group models.TagGroup
		if err := h.db.First(&group, *tag.GroupID).Error; err == nil {
			groupName = group.Name
			groupSlug = group.Slug
		}
	}

	c.JSON(http.StatusOK, TagResponse{
		ID:        tag.ID,
		Name:      tag.Name,
		Slug:      tag.Slug,
		GroupID:   tag.GroupID,
		GroupName: groupName,
		GroupSlug: groupSlug,
		Sort:      tag.Sort,
		Status:    tag.Status,
		CreatedAt: tag.CreatedAt,
		UpdatedAt: tag.UpdatedAt,
	})
}

// DeleteTag godoc
// @Summary Delete tag
// @Tags admin
// @Produce json
// @Param id path int true "tag id"
// @Success 200 {object} MessageResponse
// @Router /api/admin/tags/{id} [delete]
func (h *Handler) DeleteTag(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var collectionCount int64
	var emojiCount int64
	_ = h.db.Model(&models.CollectionTag{}).Where("tag_id = ?", id).Count(&collectionCount).Error
	_ = h.db.Model(&models.EmojiTag{}).Where("tag_id = ?", id).Count(&emojiCount).Error
	if collectionCount+emojiCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag in use"})
		return
	}
	if err := h.db.Delete(&models.Tag{}, id).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "deleted"})
}

func slugifyTag(input string) string {
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

func ensureUniqueTagSlug(db *gorm.DB, slug string, excludeID uint64) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	base := truncateString(slug, 64)
	candidate := base
	counter := 1
	for {
		var count int64
		query := db.Model(&models.Tag{}).Where("slug = ?", candidate)
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

func truncateString(input string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	return string(runes[:max])
}
