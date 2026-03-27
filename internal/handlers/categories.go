package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/storage"

	"github.com/gin-gonic/gin"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

const (
	qiniuRootPrefix  = "emoji/"
	qiniuTrashPrefix = "emoji/_trash/"
)

type CategoryRequest struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	ParentID    *uint64 `json:"parent_id"`
	Prefix      string  `json:"prefix"`
	Description string  `json:"description"`
	CoverURL    string  `json:"cover_url"`
	Icon        string  `json:"icon"`
	Sort        int     `json:"sort"`
	Status      string  `json:"status"`
}

type CategoryResponse struct {
	ID          uint64     `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	ParentID    *uint64    `json:"parent_id"`
	Prefix      string     `json:"prefix"`
	Description string     `json:"description"`
	CoverURL    string     `json:"cover_url"`
	Icon        string     `json:"icon"`
	Sort        int        `json:"sort"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

type PublicCategoryResponse struct {
	ID                    uint64  `json:"id"`
	Name                  string  `json:"name"`
	Slug                  string  `json:"slug"`
	ParentID              *uint64 `json:"parent_id,omitempty"`
	Description           string  `json:"description,omitempty"`
	CoverURL              string  `json:"cover_url,omitempty"`
	Icon                  string  `json:"icon,omitempty"`
	Sort                  int     `json:"sort"`
	PublicCollectionCount int64   `json:"public_collection_count"`
}

type ListCategoryObjectsResponse struct {
	Items      []AdminStorageItem `json:"items"`
	NextMarker string             `json:"next_marker"`
	HasNext    bool               `json:"has_next"`
	Prefix     string             `json:"prefix"`
}

type AdminStorageItem struct {
	Key      string `json:"key"`
	PutTime  int64  `json:"put_time"`
	Hash     string `json:"hash"`
	Fsize    int64  `json:"fsize"`
	MimeType string `json:"mime_type"`
	Status   int    `json:"status"`
	Md5      string `json:"md5"`
	URL      string `json:"url,omitempty"`
}

type BatchDeleteRequest struct {
	Keys []string `json:"keys"`
	Mode string   `json:"mode"`
}

type BatchDeleteResponse struct {
	Mode      string `json:"mode"`
	Processed int    `json:"processed"`
	Success   int    `json:"success"`
	Failed    int    `json:"failed"`
	TrashPath string `json:"trash_path,omitempty"`
}

type DeleteCategoryResponse struct {
	Message   string `json:"message"`
	Mode      string `json:"mode"`
	Moved     int    `json:"moved"`
	Prefix    string `json:"prefix"`
	TrashPath string `json:"trash_path,omitempty"`
}

type TrashListResponse struct {
	Items      []AdminStorageItem `json:"items"`
	NextMarker string             `json:"next_marker"`
	HasNext    bool               `json:"has_next"`
	Prefix     string             `json:"prefix"`
}

type SearchObjectsResponse struct {
	Items      []AdminStorageItem `json:"items"`
	NextMarker string             `json:"next_marker"`
	HasNext    bool               `json:"has_next"`
	Prefix     string             `json:"prefix"`
}

type TrashRestoreRequest struct {
	Key string `json:"key"`
}

type TrashRestoreResponse struct {
	Message string `json:"message"`
	From    string `json:"from"`
	To      string `json:"to"`
}

type BatchRestoreRequest struct {
	Keys []string `json:"keys"`
}

type BatchRestoreResponse struct {
	Processed int `json:"processed"`
	Success   int `json:"success"`
	Failed    int `json:"failed"`
}

type TrashEmptyResponse struct {
	Message string `json:"message"`
	Prefix  string `json:"prefix"`
	Deleted int    `json:"deleted"`
	Failed  int    `json:"failed"`
}

// ListCategories godoc
// @Summary List categories
// @Tags admin
// @Produce json
// @Success 200 {array} CategoryResponse
// @Router /api/admin/categories [get]
func (h *Handler) ListCategories(c *gin.Context) {
	var categories []models.Category
	query := h.db
	if strings.ToLower(c.Query("include_deleted")) != "true" {
		query = query.Where("deleted_at IS NULL")
	}
	if err := query.Order("sort ASC, id ASC").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := make([]CategoryResponse, 0, len(categories))
	for _, cat := range categories {
		resp = append(resp, mapCategory(cat))
	}
	c.JSON(http.StatusOK, resp)
}

// ListPublicCategories godoc
// @Summary List categories (public)
// @Tags public
// @Produce json
// @Success 200 {array} PublicCategoryResponse
// @Router /api/categories [get]
func (h *Handler) ListPublicCategories(c *gin.Context) {
	var categories []models.Category
	query := h.db.Where("deleted_at IS NULL")
	status := strings.TrimSpace(c.Query("status"))
	if status == "" {
		query = query.Where("status = ? OR status = '' OR status IS NULL", "active")
	} else if strings.ToLower(status) != "all" {
		query = query.Where("status = ?", status)
	}
	if err := query.Order("sort ASC, id ASC").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type publicCollectionCountRow struct {
		CategoryID      uint64 `gorm:"column:category_id"`
		CollectionCount int64  `gorm:"column:collection_count"`
	}
	var countRows []publicCollectionCountRow
	if err := h.db.Table("archive.collections").
		Select("category_id, COUNT(*) AS collection_count").
		Where("category_id IS NOT NULL").
		Where("deleted_at IS NULL").
		Where("status = ?", "active").
		Where("visibility = ?", "public").
		Group("category_id").
		Scan(&countRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	publicCountMap := make(map[uint64]int64, len(countRows))
	for _, row := range countRows {
		publicCountMap[row.CategoryID] = row.CollectionCount
	}

	resp := make([]PublicCategoryResponse, 0, len(categories))
	for _, cat := range categories {
		resp = append(resp, PublicCategoryResponse{
			ID:                    cat.ID,
			Name:                  cat.Name,
			Slug:                  cat.Slug,
			ParentID:              cat.ParentID,
			Description:           cat.Description,
			CoverURL:              cat.CoverURL,
			Icon:                  cat.Icon,
			Sort:                  cat.Sort,
			PublicCollectionCount: publicCountMap[cat.ID],
		})
	}
	c.JSON(http.StatusOK, resp)
}

// CreateCategory godoc
// @Summary Create category
// @Tags admin
// @Accept json
// @Produce json
// @Param body body CategoryRequest true "category request"
// @Success 200 {object} CategoryResponse
// @Router /api/admin/categories [post]
func (h *Handler) CreateCategory(c *gin.Context) {
	var req CategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	var parent *models.Category
	if req.ParentID != nil && *req.ParentID != 0 {
		var parentCat models.Category
		if err := h.db.First(&parentCat, *req.ParentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "parent category not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if parentCat.ParentID != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "parent must be top-level"})
			return
		}
		parent = &parentCat
	} else if req.ParentID != nil && *req.ParentID == 0 {
		req.ParentID = nil
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(name)
	}
	if slug == "" && parent == nil {
		prefixSeed := strings.TrimSpace(req.Prefix)
		if prefixSeed != "" {
			slug = deriveSlug(name, prefixSeed)
		}
	}
	if slug == "" {
		slug = name
	}
	slug = strings.ToLower(strings.TrimSpace(slug))

	prefixCandidate := strings.TrimSpace(req.Prefix)
	if parent != nil && prefixCandidate == "" {
		base := slug
		if base == "" {
			base = name
		}
		parentPrefix := strings.TrimSuffix(strings.TrimSpace(parent.Prefix), "/")
		if parentPrefix == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "parent prefix required"})
			return
		}
		prefixCandidate = path.Join(parentPrefix, base)
	}

	prefix, err := normalizePrefix(prefixCandidate, slug, name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if prefix == qiniuRootPrefix || strings.HasPrefix(prefix, qiniuTrashPrefix) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid prefix"})
		return
	}
	if parent != nil {
		parentPrefix := strings.TrimSpace(parent.Prefix)
		if parentPrefix == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "parent prefix required"})
			return
		}
		if !strings.HasSuffix(parentPrefix, "/") {
			parentPrefix += "/"
		}
		if !strings.HasPrefix(prefix, parentPrefix) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prefix must be under parent prefix"})
			return
		}
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "active"
	}

	cat := models.Category{
		Name:        name,
		Slug:        slug,
		ParentID:    req.ParentID,
		Prefix:      prefix,
		Description: strings.TrimSpace(req.Description),
		CoverURL:    strings.TrimSpace(req.CoverURL),
		Icon:        strings.TrimSpace(req.Icon),
		Sort:        req.Sort,
		Status:      status,
	}

	if err := h.db.Create(&cat).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, mapCategory(cat))
}

// UpdateCategory godoc
// @Summary Update category
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "category id"
// @Param body body CategoryRequest true "category request"
// @Success 200 {object} CategoryResponse
// @Router /api/admin/categories/{id} [put]
func (h *Handler) UpdateCategory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var cat models.Category
	if err := h.db.First(&cat, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req CategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if strings.TrimSpace(req.Prefix) != "" && strings.TrimSpace(req.Prefix) != cat.Prefix {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prefix immutable"})
		return
	}
	if strings.TrimSpace(req.Slug) != "" && strings.TrimSpace(req.Slug) != cat.Slug {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug immutable"})
		return
	}

	if name := strings.TrimSpace(req.Name); name != "" {
		cat.Name = name
	}
	if req.ParentID != nil {
		var newParentID *uint64
		if *req.ParentID == 0 {
			newParentID = nil
		} else {
			if *req.ParentID == cat.ID {
				c.JSON(http.StatusBadRequest, gin.H{"error": "parent_id cannot be self"})
				return
			}
			var parentCat models.Category
			if err := h.db.First(&parentCat, *req.ParentID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "parent category not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if parentCat.ParentID != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "parent must be top-level"})
				return
			}
			newParentID = req.ParentID
		}

		if (cat.ParentID == nil) != (newParentID == nil) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "parent immutable"})
			return
		}
		if cat.ParentID != nil && newParentID != nil && *cat.ParentID != *newParentID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "parent immutable"})
			return
		}
	}
	cat.Description = strings.TrimSpace(req.Description)
	cat.CoverURL = strings.TrimSpace(req.CoverURL)
	cat.Icon = strings.TrimSpace(req.Icon)
	cat.Sort = req.Sort
	if status := strings.TrimSpace(req.Status); status != "" {
		cat.Status = status
	}

	if err := h.db.Save(&cat).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, mapCategory(cat))
}

// DeleteCategory godoc
// @Summary Delete category (empty or move to trash)
// @Tags admin
// @Produce json
// @Param id path int true "category id"
// @Param mode query string false "empty|trash"
// @Success 200 {object} DeleteCategoryResponse
// @Router /api/admin/categories/{id} [delete]
func (h *Handler) DeleteCategory(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var cat models.Category
	if err := h.db.First(&cat, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if cat.Prefix == qiniuRootPrefix || strings.HasPrefix(cat.Prefix, qiniuTrashPrefix) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete reserved prefix"})
		return
	}

	mode := strings.ToLower(strings.TrimSpace(c.DefaultQuery("mode", "empty")))
	if mode != "empty" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trash mode disabled"})
		return
	}

	var childCount int64
	if err := h.db.Model(&models.Category{}).
		Where("parent_id = ? AND deleted_at IS NULL", cat.ID).
		Count(&childCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if childCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category has children"})
		return
	}

	var collectionCount int64
	if err := h.db.Model(&models.Collection{}).
		Where("category_id = ? AND deleted_at IS NULL", cat.ID).
		Count(&collectionCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if collectionCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category has collections"})
		return
	}

	hasObjects, err := h.prefixHasObjects(cat.Prefix)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if hasObjects {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category not empty"})
		return
	}

	moved := 0
	trashPath := ""

	if err := h.db.Delete(&cat).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, DeleteCategoryResponse{
		Message:   "deleted",
		Mode:      mode,
		Moved:     moved,
		Prefix:    cat.Prefix,
		TrashPath: trashPath,
	})
}

// ListCategoryObjects godoc
// @Summary List objects under category
// @Tags admin
// @Produce json
// @Param id path int true "category id"
// @Param keyword query string false "keyword"
// @Param type query string false "type (image|gif|video|other)"
// @Param sort query string false "sort (put_time|size|key)"
// @Param order query string false "order (asc|desc)"
// @Param marker query string false "marker"
// @Param limit query int false "limit" default(50)
// @Success 200 {object} ListCategoryObjectsResponse
// @Router /api/admin/categories/{id}/objects [get]
func (h *Handler) ListCategoryObjects(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var cat models.Category
	if err := h.db.First(&cat, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	marker := c.Query("marker")
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
	typeFilter := strings.ToLower(strings.TrimSpace(c.Query("type")))
	sortField := strings.ToLower(strings.TrimSpace(c.Query("sort")))
	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("order")))
	if sortField == "time" {
		sortField = "put_time"
	}
	if typeFilter == "all" {
		typeFilter = ""
	}

	items, nextMarker, hasNext, err := h.listFilteredObjects(cat.Prefix, marker, limit, keyword, typeFilter, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if sortField != "" {
		sortItems(items, sortField, sortOrder)
	}

	c.JSON(http.StatusOK, ListCategoryObjectsResponse{
		Items:      mapAdminItems(items, h.qiniu),
		NextMarker: nextMarker,
		HasNext:    hasNext,
		Prefix:     cat.Prefix,
	})
}

// AdminDeleteObject godoc
// @Summary Delete or trash an object
// @Tags admin
// @Produce json
// @Param key query string true "object key"
// @Param mode query string false "delete|trash" default(trash)
// @Success 200 {object} MessageResponse
// @Router /api/admin/storage/object [delete]
func (h *Handler) AdminDeleteObject(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	key := strings.TrimLeft(strings.TrimSpace(c.Query("key")), "/")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing key"})
		return
	}
	if !strings.HasPrefix(key, qiniuRootPrefix) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key must start with emoji/"})
		return
	}

	mode := strings.ToLower(strings.TrimSpace(c.DefaultQuery("mode", "trash")))
	bm := h.qiniu.BucketManager()
	if mode == "trash" {
		destKey := buildTrashKey(key, time.Now())
		if err := bm.Move(h.qiniu.Bucket, key, h.qiniu.Bucket, destKey, true); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, MessageResponse{Message: "moved to trash"})
		return
	}
	if mode != "delete" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be delete or trash"})
		return
	}
	if err := bm.Delete(h.qiniu.Bucket, key); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "deleted"})
}

// BatchDeleteObjects godoc
// @Summary Batch delete or trash objects
// @Tags admin
// @Accept json
// @Produce json
// @Param body body BatchDeleteRequest true "batch delete request"
// @Success 200 {object} BatchDeleteResponse
// @Router /api/admin/storage/batch-delete [post]
func (h *Handler) BatchDeleteObjects(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keys required"})
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "trash"
	}
	if mode != "trash" && mode != "delete" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be trash or delete"})
		return
	}

	uniqueKeys := make([]string, 0, len(req.Keys))
	seen := map[string]struct{}{}
	for _, raw := range req.Keys {
		key := strings.TrimLeft(strings.TrimSpace(raw), "/")
		if key == "" {
			continue
		}
		if !strings.HasPrefix(key, qiniuRootPrefix) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "key must start with emoji/"})
			return
		}
		if mode == "trash" && strings.HasPrefix(key, qiniuTrashPrefix) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "trash objects cannot be trashed again"})
			return
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		uniqueKeys = append(uniqueKeys, key)
	}
	if len(uniqueKeys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid keys"})
		return
	}

	bm := h.qiniu.BucketManager()
	trashBase := ""
	if mode == "trash" {
		trashBase = path.Join(qiniuTrashPrefix, time.Now().Format("20060102-150405"))
	}

	success := 0
	failed := 0
	for start := 0; start < len(uniqueKeys); start += 1000 {
		end := start + 1000
		if end > len(uniqueKeys) {
			end = len(uniqueKeys)
		}
		keys := uniqueKeys[start:end]
		ops := make([]string, 0, len(keys))
		for _, key := range keys {
			if mode == "delete" {
				ops = append(ops, qiniustorage.URIDelete(h.qiniu.Bucket, key))
			} else {
				destKey := buildTrashKeyWithBase(key, trashBase)
				ops = append(ops, qiniustorage.URIMove(h.qiniu.Bucket, key, h.qiniu.Bucket, destKey, true))
			}
		}
		rets, err := bm.Batch(ops)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		for _, ret := range rets {
			if ret.Code == 200 {
				success++
			} else {
				failed++
			}
		}
	}

	c.JSON(http.StatusOK, BatchDeleteResponse{
		Mode:      mode,
		Processed: len(uniqueKeys),
		Success:   success,
		Failed:    failed,
		TrashPath: trashBase,
	})
}

// ListTrashObjects godoc
// @Summary List trash objects
// @Tags admin
// @Produce json
// @Param prefix query string false "prefix" default(emoji/_trash/)
// @Param marker query string false "marker"
// @Param limit query int false "limit" default(50)
// @Success 200 {object} TrashListResponse
// @Router /api/admin/storage/trash [get]
func (h *Handler) ListTrashObjects(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	prefix := strings.TrimLeft(strings.TrimSpace(c.DefaultQuery("prefix", qiniuTrashPrefix)), "/")
	if !strings.HasPrefix(prefix, qiniuTrashPrefix) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prefix must start with emoji/_trash/"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	marker := c.Query("marker")
	items, nextMarker, hasNext, err := h.listFilteredObjects(prefix, marker, limit, "", "", nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, TrashListResponse{
		Items:      mapAdminItems(items, h.qiniu),
		NextMarker: nextMarker,
		HasNext:    hasNext,
		Prefix:     prefix,
	})
}

// AdminSearchObjects godoc
// @Summary Search objects in storage
// @Tags admin
// @Produce json
// @Param prefix query string false "prefix" default(emoji/)
// @Param keyword query string false "keyword"
// @Param type query string false "type (image|gif|video|other)"
// @Param sort query string false "sort (put_time|size|key)"
// @Param order query string false "order (asc|desc)"
// @Param marker query string false "marker"
// @Param limit query int false "limit" default(50)
// @Success 200 {object} SearchObjectsResponse
// @Router /api/admin/storage/search [get]
func (h *Handler) AdminSearchObjects(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	prefix := strings.TrimLeft(strings.TrimSpace(c.DefaultQuery("prefix", qiniuRootPrefix)), "/")
	if !strings.HasPrefix(prefix, qiniuRootPrefix) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prefix must start with emoji/"})
		return
	}
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
	typeFilter := strings.ToLower(strings.TrimSpace(c.Query("type")))
	if typeFilter == "all" {
		typeFilter = ""
	}
	sortField := strings.ToLower(strings.TrimSpace(c.Query("sort")))
	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("order")))
	if sortField == "time" {
		sortField = "put_time"
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	marker := c.Query("marker")

	items, nextMarker, hasNext, err := h.listFilteredObjects(prefix, marker, limit, keyword, typeFilter, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if sortField != "" {
		sortItems(items, sortField, sortOrder)
	}

	c.JSON(http.StatusOK, SearchObjectsResponse{
		Items:      mapAdminItems(items, h.qiniu),
		NextMarker: nextMarker,
		HasNext:    hasNext,
		Prefix:     prefix,
	})
}

// RestoreTrashObject godoc
// @Summary Restore trash object
// @Tags admin
// @Accept json
// @Produce json
// @Param body body TrashRestoreRequest true "restore request"
// @Success 200 {object} TrashRestoreResponse
// @Router /api/admin/storage/trash/restore [post]
func (h *Handler) RestoreTrashObject(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	var req TrashRestoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	key := strings.TrimLeft(strings.TrimSpace(req.Key), "/")
	if key == "" || !strings.HasPrefix(key, qiniuTrashPrefix) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid trash key"})
		return
	}
	destKey, err := restoreKeyFromTrash(key)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	bm := h.qiniu.BucketManager()
	if err := bm.Move(h.qiniu.Bucket, key, h.qiniu.Bucket, destKey, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, TrashRestoreResponse{
		Message: "restored",
		From:    key,
		To:      destKey,
	})
}

// BatchRestoreTrashObjects godoc
// @Summary Batch restore trash objects
// @Tags admin
// @Accept json
// @Produce json
// @Param body body BatchRestoreRequest true "batch restore request"
// @Success 200 {object} BatchRestoreResponse
// @Router /api/admin/storage/trash/batch-restore [post]
func (h *Handler) BatchRestoreTrashObjects(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	var req BatchRestoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keys required"})
		return
	}

	unique := make([]string, 0, len(req.Keys))
	seen := map[string]struct{}{}
	for _, raw := range req.Keys {
		key := strings.TrimLeft(strings.TrimSpace(raw), "/")
		if key == "" {
			continue
		}
		if !strings.HasPrefix(key, qiniuTrashPrefix) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "trash key must start with emoji/_trash/"})
			return
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}
	if len(unique) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid keys"})
		return
	}

	bm := h.qiniu.BucketManager()
	success := 0
	failed := 0
	for start := 0; start < len(unique); start += 1000 {
		end := start + 1000
		if end > len(unique) {
			end = len(unique)
		}
		keys := unique[start:end]
		ops := make([]string, 0, len(keys))
		for _, key := range keys {
			destKey, err := restoreKeyFromTrash(key)
			if err != nil {
				failed++
				continue
			}
			ops = append(ops, qiniustorage.URIMove(h.qiniu.Bucket, key, h.qiniu.Bucket, destKey, true))
		}
		if len(ops) == 0 {
			continue
		}
		rets, err := bm.Batch(ops)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		for _, ret := range rets {
			if ret.Code == 200 {
				success++
			} else {
				failed++
			}
		}
	}

	c.JSON(http.StatusOK, BatchRestoreResponse{
		Processed: len(unique),
		Success:   success,
		Failed:    failed,
	})
}

// EmptyTrash godoc
// @Summary Empty trash prefix
// @Tags admin
// @Produce json
// @Param prefix query string false "prefix" default(emoji/_trash/)
// @Success 200 {object} TrashEmptyResponse
// @Router /api/admin/storage/trash [delete]
func (h *Handler) EmptyTrash(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	prefix := strings.TrimLeft(strings.TrimSpace(c.DefaultQuery("prefix", qiniuTrashPrefix)), "/")
	if !strings.HasPrefix(prefix, qiniuTrashPrefix) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prefix must start with emoji/_trash/"})
		return
	}

	bm := h.qiniu.BucketManager()
	deleted := 0
	failed := 0
	marker := ""
	for {
		items, _, nextMarker, hasNext, err := bm.ListFiles(h.qiniu.Bucket, prefix, "", marker, 1000)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if len(items) == 0 && !hasNext {
			break
		}
		if len(items) > 0 {
			ops := make([]string, 0, len(items))
			for _, item := range items {
				ops = append(ops, qiniustorage.URIDelete(h.qiniu.Bucket, item.Key))
			}
			rets, err := bm.Batch(ops)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			for _, ret := range rets {
				if ret.Code == 200 {
					deleted++
				} else {
					failed++
				}
			}
		}
		if !hasNext {
			break
		}
		marker = nextMarker
	}

	c.JSON(http.StatusOK, TrashEmptyResponse{
		Message: "emptied",
		Prefix:  prefix,
		Deleted: deleted,
		Failed:  failed,
	})
}

func mapCategory(cat models.Category) CategoryResponse {
	var deletedAt *time.Time
	if cat.DeletedAt.Valid {
		t := cat.DeletedAt.Time
		deletedAt = &t
	}
	return CategoryResponse{
		ID:          cat.ID,
		Name:        cat.Name,
		Slug:        cat.Slug,
		ParentID:    cat.ParentID,
		Prefix:      cat.Prefix,
		Description: cat.Description,
		CoverURL:    cat.CoverURL,
		Icon:        cat.Icon,
		Sort:        cat.Sort,
		Status:      cat.Status,
		CreatedAt:   cat.CreatedAt,
		UpdatedAt:   cat.UpdatedAt,
		DeletedAt:   deletedAt,
	}
}

func normalizePrefix(prefix, slug, name string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		base := ""
		if slug != "" {
			base = slug
		} else if name != "" {
			base = name
		}
		if base != "" {
			prefix = path.Join("collections", base)
		}
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", errors.New("prefix required")
	}
	prefix = strings.TrimLeft(prefix, "/")
	if !strings.HasPrefix(prefix, qiniuRootPrefix) {
		prefix = path.Join(qiniuRootPrefix, prefix)
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if !strings.HasPrefix(prefix, qiniuRootPrefix) {
		return "", errors.New("prefix must start with emoji/")
	}
	return prefix, nil
}

func deriveSlug(name, prefix string) string {
	candidate := ""
	if prefix != "" {
		candidate = strings.TrimPrefix(prefix, qiniuRootPrefix)
		candidate = strings.Trim(candidate, "/")
		if strings.HasPrefix(candidate, "collections/") {
			candidate = strings.TrimPrefix(candidate, "collections/")
		}
	}
	if candidate == "" {
		candidate = slugify(name)
	}
	if candidate == "" {
		candidate = fmt.Sprintf("cat-%d", time.Now().Unix())
	}
	return candidate
}

func slugify(input string) string {
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
	out := strings.Trim(b.String(), "-")
	return out
}

func (h *Handler) prefixHasObjects(prefix string) (bool, error) {
	bm := h.qiniu.BucketManager()
	items, _, _, _, err := bm.ListFiles(h.qiniu.Bucket, prefix, "", "", 1)
	if err != nil {
		return false, err
	}
	return len(items) > 0, nil
}

func (h *Handler) movePrefixToTrash(prefix string) (int, string, error) {
	bm := h.qiniu.BucketManager()
	marker := ""
	moved := 0
	segment := time.Now().Format("20060102-150405")
	trashBase := path.Join(qiniuTrashPrefix, segment)
	for {
		items, _, nextMarker, hasNext, err := bm.ListFiles(h.qiniu.Bucket, prefix, "", marker, 1000)
		if err != nil {
			return moved, trashBase, err
		}
		if len(items) == 0 && !hasNext {
			break
		}
		ops := make([]string, 0, len(items))
		for _, item := range items {
			destKey := buildTrashKeyWithBase(item.Key, trashBase)
			ops = append(ops, qiniustorage.URIMove(h.qiniu.Bucket, item.Key, h.qiniu.Bucket, destKey, true))
		}
		if len(ops) > 0 {
			rets, err := bm.Batch(ops)
			if err != nil {
				return moved, trashBase, err
			}
			if len(rets) != len(ops) {
				return moved, trashBase, errors.New("batch move incomplete")
			}
			for _, ret := range rets {
				if ret.Code == 200 {
					moved++
					continue
				}
				meta := map[string]interface{}{
					"code": ret.Code,
				}
				raw, _ := json.Marshal(meta)
				return moved, trashBase, fmt.Errorf("batch move failed: %s", string(raw))
			}
		}
		if !hasNext {
			break
		}
		marker = nextMarker
	}
	return moved, trashBase, nil
}

func buildTrashKey(key string, now time.Time) string {
	segment := now.Format("20060102-150405")
	trashBase := path.Join(qiniuTrashPrefix, segment)
	return buildTrashKeyWithBase(key, trashBase)
}

func buildTrashKeyWithBase(key, trashBase string) string {
	rel := strings.TrimPrefix(key, qiniuRootPrefix)
	rel = strings.TrimLeft(rel, "/")
	return path.Join(trashBase, rel)
}

func restoreKeyFromTrash(key string) (string, error) {
	if !strings.HasPrefix(key, qiniuTrashPrefix) {
		return "", errors.New("invalid trash key")
	}
	trimmed := strings.TrimPrefix(key, qiniuTrashPrefix)
	trimmed = strings.TrimLeft(trimmed, "/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) < 2 {
		return "", errors.New("invalid trash key")
	}
	rel := strings.TrimLeft(parts[1], "/")
	if rel == "" {
		return "", errors.New("invalid trash key")
	}
	return path.Join(qiniuRootPrefix, rel), nil
}

func mapAdminItems(items []qiniustorage.ListItem, q *storage.QiniuClient) []AdminStorageItem {
	respItems := make([]AdminStorageItem, 0, len(items))
	for _, item := range items {
		obj := AdminStorageItem{
			Key:      item.Key,
			PutTime:  item.PutTime,
			Hash:     item.Hash,
			Fsize:    item.Fsize,
			MimeType: item.MimeType,
			Status:   item.Status,
			Md5:      item.Md5,
		}
		if q != nil && !q.Private && q.Domain != "" {
			obj.URL = q.PublicURL(item.Key)
		}
		respItems = append(respItems, obj)
	}
	return respItems
}

type itemFilter func(item qiniustorage.ListItem) bool

func (h *Handler) listFilteredObjects(prefix, marker string, limit int, keyword, typeFilter string, extraFilter itemFilter) ([]qiniustorage.ListItem, string, bool, error) {
	if h.qiniu == nil {
		return nil, "", false, errors.New("qiniu not configured")
	}
	if limit <= 0 {
		limit = 50
	}
	bm := h.qiniu.BucketManager()
	collected := make([]qiniustorage.ListItem, 0, limit)
	currentMarker := strings.TrimSpace(marker)
	for {
		items, _, nextMarker, hasNext, err := bm.ListFiles(h.qiniu.Bucket, prefix, "", currentMarker, 1000)
		if err != nil {
			return nil, "", false, err
		}
		if len(items) == 0 && !hasNext {
			return collected, "", false, nil
		}
		for _, item := range items {
			if keyword != "" && !strings.Contains(strings.ToLower(item.Key), keyword) {
				continue
			}
			if typeFilter != "" && !matchTypeFilter(item, typeFilter) {
				continue
			}
			if extraFilter != nil && !extraFilter(item) {
				continue
			}
			collected = append(collected, item)
			if len(collected) >= limit {
				return collected, item.Key, true, nil
			}
		}
		if !hasNext {
			return collected, "", false, nil
		}
		currentMarker = nextMarker
	}
}

func matchTypeFilter(item qiniustorage.ListItem, filter string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" || filter == "all" {
		return true
	}
	keyExt := strings.ToLower(path.Ext(item.Key))
	mime := strings.ToLower(item.MimeType)
	if filter == "gif" {
		return keyExt == ".gif" || strings.Contains(mime, "gif")
	}
	if filter == "video" {
		if strings.HasPrefix(mime, "video/") {
			return true
		}
		switch keyExt {
		case ".webm", ".mp4", ".mov":
			return true
		default:
			return false
		}
	}
	if filter == "image" {
		if strings.HasPrefix(mime, "image/") {
			return !strings.Contains(mime, "gif")
		}
		switch keyExt {
		case ".png", ".jpg", ".jpeg", ".webp":
			return true
		default:
			return false
		}
	}
	if filter == "other" {
		return !matchTypeFilter(item, "image") && !matchTypeFilter(item, "gif") && !matchTypeFilter(item, "video")
	}
	return true
}

func sortItems(items []qiniustorage.ListItem, field, order string) {
	field = strings.ToLower(strings.TrimSpace(field))
	order = strings.ToLower(strings.TrimSpace(order))
	desc := order == "desc"
	switch field {
	case "put_time", "time":
		sort.Slice(items, func(i, j int) bool {
			if desc {
				return items[i].PutTime > items[j].PutTime
			}
			return items[i].PutTime < items[j].PutTime
		})
	case "size", "fsize":
		sort.Slice(items, func(i, j int) bool {
			if desc {
				return items[i].Fsize > items[j].Fsize
			}
			return items[i].Fsize < items[j].Fsize
		})
	case "key":
		sort.Slice(items, func(i, j int) bool {
			if desc {
				return items[i].Key > items[j].Key
			}
			return items[i].Key < items[j].Key
		})
	}
}
