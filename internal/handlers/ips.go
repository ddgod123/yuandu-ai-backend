package handlers

import (
	"context"
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
	CategoryID  *uint64 `json:"category_id,omitempty"` // 兼容保留，不再作为主关联
	Description string  `json:"description"`
	Sort        int     `json:"sort"`
	Status      string  `json:"status"`
}

type IPResponse struct {
	ID              uint64    `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	CoverURL        string    `json:"cover_url"`
	CoverThumbURL   string    `json:"cover_thumb_url,omitempty"`
	CategoryID      *uint64   `json:"category_id,omitempty"`
	Description     string    `json:"description"`
	Sort            int       `json:"sort"`
	Status          string    `json:"status"`
	CollectionCount int64     `json:"collection_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type IPCollectionsResponse struct {
	Items []CollectionListItem `json:"items"`
	Total int64                `json:"total"`
}

func extractIPIDs(ips []models.IP) []uint64 {
	ids := make([]uint64, 0, len(ips))
	for _, ip := range ips {
		ids = append(ids, ip.ID)
	}
	return ids
}

func (h *Handler) loadIPCollectionCountMap(tx *gorm.DB, ipIDs []uint64, publicOnly bool) (map[uint64]int64, error) {
	result := make(map[uint64]int64, len(ipIDs))
	if len(ipIDs) == 0 {
		return result, nil
	}

	type row struct {
		IPID  uint64
		Count int64
	}

	// 新链路：优先读绑定表（仅 active 绑定），且当合集存在任意绑定记录时不再回退 collections.ip_id。
	// 旧链路：无绑定记录时，继续回退 collections.ip_id，兼容历史数据。
	if h.ipBindingTableReady(tx) {
		var bindingRows []row
		bindingQuery := tx.Table("taxonomy.ip_collection_bindings AS b").
			Select("b.ip_id AS ip_id, COUNT(DISTINCT b.collection_id) AS count").
			Joins("JOIN archive.collections c ON c.id = b.collection_id").
			Where("b.ip_id IN ?", ipIDs).
			Where("b.status = ?", ipBindingStatusActive).
			Where("c.deleted_at IS NULL")
		if publicOnly {
			bindingQuery = bindingQuery.Where("c.status = ?", "active").Where("c.visibility = ?", "public")
		}
		if err := bindingQuery.Group("b.ip_id").Scan(&bindingRows).Error; err != nil {
			return nil, err
		}
		for _, item := range bindingRows {
			result[item.IPID] += item.Count
		}

		var fallbackRows []row
		fallbackQuery := tx.Table("archive.collections AS c").
			Select("c.ip_id AS ip_id, COUNT(*) AS count").
			Where("c.ip_id IN ?", ipIDs).
			Where("c.deleted_at IS NULL").
			Where("NOT EXISTS (SELECT 1 FROM taxonomy.ip_collection_bindings bx WHERE bx.collection_id = c.id)")
		if publicOnly {
			fallbackQuery = fallbackQuery.Where("c.status = ?", "active").Where("c.visibility = ?", "public")
		}
		if err := fallbackQuery.Group("c.ip_id").Scan(&fallbackRows).Error; err != nil {
			return nil, err
		}
		for _, item := range fallbackRows {
			result[item.IPID] += item.Count
		}
		return result, nil
	}

	var rows []row
	query := tx.Model(&models.Collection{}).
		Select("ip_id AS ip_id, COUNT(*) AS count").
		Where("ip_id IN ?", ipIDs).
		Where("deleted_at IS NULL")
	if publicOnly {
		query = query.Where("status = ?", "active").Where("visibility = ?", "public")
	}
	if err := query.Group("ip_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, item := range rows {
		result[item.IPID] += item.Count
	}
	return result, nil
}

func buildIPResponse(ip models.IP, collectionCount int64) IPResponse {
	return IPResponse{
		ID:              ip.ID,
		Name:            ip.Name,
		Slug:            ip.Slug,
		CoverURL:        ip.CoverURL,
		CategoryID:      ip.CategoryID,
		Description:     ip.Description,
		Sort:            ip.Sort,
		Status:          ip.Status,
		CollectionCount: collectionCount,
		CreatedAt:       ip.CreatedAt,
		UpdatedAt:       ip.UpdatedAt,
	}
}

func hasIPImageExt(value string) bool {
	clean := strings.ToLower(strings.SplitN(strings.SplitN(strings.TrimSpace(value), "?", 2)[0], "#", 2)[0])
	return strings.HasSuffix(clean, ".jpg") ||
		strings.HasSuffix(clean, ".jpeg") ||
		strings.HasSuffix(clean, ".png") ||
		strings.HasSuffix(clean, ".gif") ||
		strings.HasSuffix(clean, ".webp")
}

func buildIPStaticPreview(raw string) string {
	val := strings.TrimSpace(raw)
	if val == "" || !hasIPImageExt(val) {
		return ""
	}
	if !strings.HasPrefix(val, "http://") && !strings.HasPrefix(val, "https://") && !strings.HasPrefix(val, "//") {
		return ""
	}
	if strings.Contains(val, "token=") || strings.Contains(val, "e=") {
		return ""
	}
	separator := "?"
	if strings.Contains(val, "?") {
		separator = "&"
	}
	// 贴合前台 IP 卡片展示比例，优先返回静态缩略图，降低前台解码和带宽开销。
	return val + separator + "imageMogr2/thumbnail/!640x340r/gravity/Center/crop/640x340/format/webp"
}

func (h *Handler) resolveIPCoverURLs(ctx context.Context, raw string, adminView bool) (coverURL string, coverThumbURL string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	key := normalizeStorageObjectKey(raw)
	if h.qiniu != nil && h.hasQiniuAllowedRootPrefix(key) {
		coverURL = raw
		if original, err := h.resolveObjectURLWithCache(ctx, key, 0, false, ""); err == nil && strings.TrimSpace(original.URL) != "" {
			coverURL = original.URL
		}

		if thumb, err := h.resolveObjectURLWithCache(ctx, key, 0, false, normalizeStorageURLStyle("ip_cover_card")); err == nil && strings.TrimSpace(thumb.URL) != "" {
			coverThumbURL = thumb.URL
		}
		if coverThumbURL == "" {
			coverThumbURL = coverURL
		}
		return coverURL, coverThumbURL
	}

	coverURL = raw
	if thumb := buildIPStaticPreview(raw); thumb != "" {
		return coverURL, thumb
	}
	return coverURL, coverURL
}

func (h *Handler) listIPs(c *gin.Context, adminView bool) {
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
	if !adminView {
		query = query.Where("status = ?", "active")
	}

	if err := query.Order("sort ASC, id ASC").Find(&ips).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ipIDs := extractIPIDs(ips)
	countMap, err := h.loadIPCollectionCountMap(h.db, ipIDs, !adminView)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := make([]IPResponse, 0, len(ips))
	ctx := c.Request.Context()
	for _, ip := range ips {
		item := buildIPResponse(ip, countMap[ip.ID])
		coverURL, coverThumbURL := h.resolveIPCoverURLs(ctx, ip.CoverURL, adminView)
		if strings.TrimSpace(coverURL) != "" {
			item.CoverURL = coverURL
		}
		if strings.TrimSpace(coverThumbURL) != "" {
			item.CoverThumbURL = coverThumbURL
		}
		resp = append(resp, item)
	}
	c.JSON(http.StatusOK, resp)
}

// ListIPs godoc
// @Summary List IPs
// @Tags public
// @Produce json
// @Param keyword query string false "keyword"
// @Success 200 {array} IPResponse
// @Router /api/ips [get]
func (h *Handler) ListIPs(c *gin.Context) {
	h.listIPs(c, false)
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
	if err := h.db.Where("id = ? AND status = ?", id, "active").First(&ip).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	countMap, err := h.loadIPCollectionCountMap(h.db, []uint64{id}, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := buildIPResponse(ip, countMap[id])
	coverURL, coverThumbURL := h.resolveIPCoverURLs(c.Request.Context(), ip.CoverURL, false)
	if strings.TrimSpace(coverURL) != "" {
		resp.CoverURL = coverURL
	}
	if strings.TrimSpace(coverThumbURL) != "" {
		resp.CoverThumbURL = coverThumbURL
	}
	c.JSON(http.StatusOK, resp)
}

// GetIPCollections godoc
// @Summary Get IP collections
// @Tags public
// @Produce json
// @Param id path int true "ip id"
// @Param page query int false "page"
// @Param page_size query int false "page size"
// @Param sort query string false "sort field: created_at|file_count|id"
// @Param order query string false "sort order: asc|desc"
// @Param preview_count query int false "preview images count"
// @Success 200 {object} IPCollectionsResponse
// @Router /api/ips/{id}/collections [get]
func (h *Handler) GetIPCollections(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var ip models.IP
	if err := h.db.Where("id = ? AND status = ?", id, "active").First(&ip).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	previewCount, _ := strconv.Atoi(c.DefaultQuery("preview_count", "15"))
	sortField := strings.ToLower(strings.TrimSpace(c.Query("sort")))
	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("order")))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if previewCount < 0 {
		previewCount = 0
	}
	if previewCount > 30 {
		previewCount = 30
	}
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	db := h.db.Model(&models.Collection{}).
		Where("archive.collections.deleted_at IS NULL").
		Where("archive.collections.status = ?", "active").
		Where("archive.collections.visibility = ?", "public")

	withBindingSort := false
	if h.ipBindingTableReady(h.db) {
		withBindingSort = true
		db = db.Joins(
			"LEFT JOIN taxonomy.ip_collection_bindings ipb ON ipb.collection_id = archive.collections.id AND ipb.ip_id = ? AND ipb.status = ?",
			id,
			ipBindingStatusActive,
		).Where(
			`ipb.id IS NOT NULL OR (archive.collections.ip_id = ? AND NOT EXISTS (
				SELECT 1 FROM taxonomy.ip_collection_bindings bx WHERE bx.collection_id = archive.collections.id
			))`,
			id,
		)
	} else {
		db = db.Where("archive.collections.ip_id = ?", id)
	}

	orderBy := "archive.collections.id desc"
	switch sortField {
	case "created_at":
		orderBy = "archive.collections.created_at " + sortOrder + ", archive.collections.id desc"
	case "file_count":
		orderBy = "archive.collections.file_count " + sortOrder + ", archive.collections.id desc"
	case "id":
		orderBy = "archive.collections.id " + sortOrder
	}
	if withBindingSort {
		orderBy = "COALESCE(ipb.sort, 2147483647) ASC, " + orderBy
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var items []models.Collection
	if err := db.Order(orderBy).Offset((page - 1) * limit).Limit(limit).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tagMap := loadCollectionTags(h.db, items)
	categoryPrefixMap := loadCollectionCategoryPrefixMap(h.db, items)
	creatorMap := loadCreatorProfiles(h.db, items)
	statMap := loadCollectionStats(h.db, items)
	previewAssetMap := loadCollectionPreviewAssets(h.db, h.qiniu, items, previewCount, false)
	collectionIDs := make([]uint64, 0, len(items))
	for _, item := range items {
		collectionIDs = append(collectionIDs, item.ID)
	}
	likedMap := map[uint64]bool{}
	favoritedMap := map[uint64]bool{}
	if userID, ok := currentUserIDFromContext(c); ok {
		likedMap, favoritedMap = loadCollectionActionState(h.db, collectionIDs, userID)
	}

	respItems := make([]CollectionListItem, 0, len(items))
	for _, item := range items {
		var creatorName string
		var creatorNameZh string
		var creatorNameEn string
		var creatorAvatar string
		if item.CreatorProfileID != nil {
			if profile, ok := creatorMap[*item.CreatorProfileID]; ok {
				creatorNameZh = profile.NameZh
				creatorNameEn = profile.NameEn
				creatorAvatar = profile.AvatarURL
				creatorName = pickCreatorDisplayName(profile)
			}
		}
		stats := statMap[item.ID]
		coverKey := resolveCollectionCoverKey(item.CoverURL, h.qiniu)
		respItems = append(respItems, CollectionListItem{
			ID:               item.ID,
			Title:            item.Title,
			Slug:             item.Slug,
			Description:      item.Description,
			CoverKey:         coverKey,
			CoverURL:         resolveListPreviewURL(item.CoverURL, h.qiniu),
			OwnerID:          item.OwnerID,
			CreatorProfileID: item.CreatorProfileID,
			CreatorName:      creatorName,
			CreatorNameZh:    creatorNameZh,
			CreatorNameEn:    creatorNameEn,
			CreatorAvatarURL: creatorAvatar,
			FavoriteCount:    stats.FavoriteCount,
			LikeCount:        stats.LikeCount,
			DownloadCount:    stats.DownloadCount,
			Favorited:        favoritedMap[item.ID],
			Liked:            likedMap[item.ID],
			CategoryID:       item.CategoryID,
			IPID:             item.IPID,
			ThemeID:          item.ThemeID,
			Source:           item.Source,
			QiniuPrefix:      item.QiniuPrefix,
			PathMismatch:     resolveCollectionPathMismatch(item, categoryPrefixMap),
			FileCount:        item.FileCount,
			IsFeatured:       item.IsFeatured,
			IsPinned:         item.IsPinned,
			IsSample:         item.IsSample,
			PinnedAt:         item.PinnedAt,
			LatestZipKey:     item.LatestZipKey,
			LatestZipName:    item.LatestZipName,
			LatestZipSize:    item.LatestZipSize,
			LatestZipAt:      item.LatestZipAt,
			DownloadCode:     item.DownloadCode,
			Visibility:       item.Visibility,
			Status:           item.Status,
			CreatedAt:        item.CreatedAt,
			UpdatedAt:        item.UpdatedAt,
			Tags:             tagMap[item.ID],
			PreviewImages:    flattenPreviewAssetsToImages(previewAssetMap[item.ID]),
			PreviewAssets:    previewAssetMap[item.ID],
		})
	}

	c.JSON(http.StatusOK, IPCollectionsResponse{Items: respItems, Total: total})
}

// ListAdminIPs godoc
// @Summary List IPs (admin)
// @Tags admin
// @Produce json
// @Param keyword query string false "keyword"
// @Success 200 {array} IPResponse
// @Router /api/admin/ips [get]
func (h *Handler) ListAdminIPs(c *gin.Context) {
	h.listIPs(c, true)
}

// GetAdminIP godoc
// @Summary Get IP (admin)
// @Tags admin
// @Produce json
// @Param id path int true "ip id"
// @Success 200 {object} IPResponse
// @Router /api/admin/ips/{id} [get]
func (h *Handler) GetAdminIP(c *gin.Context) {
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

	countMap, err := h.loadIPCollectionCountMap(h.db, []uint64{id}, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := buildIPResponse(ip, countMap[id])
	_, coverThumbURL := h.resolveIPCoverURLs(c.Request.Context(), ip.CoverURL, true)
	if strings.TrimSpace(coverThumbURL) != "" {
		resp.CoverThumbURL = coverThumbURL
	}
	c.JSON(http.StatusOK, resp)
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
	c.JSON(http.StatusOK, buildIPResponse(ip, 0))
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
	ip.CoverURL = strings.TrimSpace(req.CoverURL)
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
	countMap, loadErr := h.loadIPCollectionCountMap(h.db, []uint64{ip.ID}, false)
	if loadErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": loadErr.Error()})
		return
	}
	c.JSON(http.StatusOK, buildIPResponse(ip, countMap[ip.ID]))
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
	_ = h.db.Model(&models.Collection{}).Where("ip_id = ?", id).Where("deleted_at IS NULL").Count(&count).Error
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
