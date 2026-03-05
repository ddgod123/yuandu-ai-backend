package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/storage"

	"github.com/gin-gonic/gin"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

type TagBrief struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type CollectionListItem struct {
	ID               uint64     `json:"id"`
	Title            string     `json:"title"`
	Slug             string     `json:"slug"`
	Description      string     `json:"description,omitempty"`
	CoverURL         string     `json:"cover_url,omitempty"`
	OwnerID          uint64     `json:"owner_id"`
	CreatorProfileID *uint64    `json:"creator_profile_id,omitempty"`
	CreatorName      string     `json:"creator_name,omitempty"`
	CreatorNameZh    string     `json:"creator_name_zh,omitempty"`
	CreatorNameEn    string     `json:"creator_name_en,omitempty"`
	CreatorAvatarURL string     `json:"creator_avatar_url,omitempty"`
	FavoriteCount    int64      `json:"favorite_count"`
	LikeCount        int64      `json:"like_count"`
	DownloadCount    int64      `json:"download_count"`
	Favorited        bool       `json:"favorited"`
	Liked            bool       `json:"liked"`
	CategoryID       *uint64    `json:"category_id,omitempty"`
	IPID             *uint64    `json:"ip_id,omitempty"`
	ThemeID          *uint64    `json:"theme_id,omitempty"`
	Source           string     `json:"source,omitempty"`
	QiniuPrefix      string     `json:"qiniu_prefix,omitempty"`
	FileCount        int        `json:"file_count"`
	IsFeatured       bool       `json:"is_featured"`
	IsPinned         bool       `json:"is_pinned"`
	PinnedAt         *time.Time `json:"pinned_at,omitempty"`
	LatestZipKey     string     `json:"latest_zip_key,omitempty"`
	LatestZipName    string     `json:"latest_zip_name,omitempty"`
	LatestZipSize    int64      `json:"latest_zip_size,omitempty"`
	LatestZipAt      *time.Time `json:"latest_zip_at,omitempty"`
	DownloadCode     string     `json:"download_code,omitempty"`
	Visibility       string     `json:"visibility,omitempty"`
	Status           string     `json:"status,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	Tags             []TagBrief `json:"tags,omitempty"`
	PreviewImages    []string   `json:"preview_images,omitempty"`
}

type CollectionListResponse struct {
	Items []CollectionListItem `json:"items"`
	Total int64                `json:"total"`
}

const maxFeaturedCollections = 4

func isAdminRole(c *gin.Context) bool {
	roleVal, ok := c.Get("role")
	if !ok {
		return false
	}
	role, ok := roleVal.(string)
	if !ok {
		return false
	}
	return strings.EqualFold(role, "super_admin") || strings.EqualFold(role, "admin")
}

func isPublicCollectionVisible(collection models.Collection) bool {
	return strings.EqualFold(strings.TrimSpace(collection.Status), "active") &&
		strings.EqualFold(strings.TrimSpace(collection.Visibility), "public")
}

func normalizeCollectionStatus(raw string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "pending", "active", "disabled":
		return v, true
	default:
		return "", false
	}
}

func normalizeCollectionVisibility(raw string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "public", "private":
		return v, true
	default:
		return "", false
	}
}

func normalizeCollectionMediaType(raw string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return "all", true
	}
	switch v {
	case "all", "animated", "static":
		return v, true
	default:
		return "", false
	}
}

func parseOptionalBoolParam(raw string) (*bool, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" || v == "all" {
		return nil, true
	}
	switch v {
	case "1", "true", "yes", "y", "on":
		value := true
		return &value, true
	case "0", "false", "no", "n", "off":
		value := false
		return &value, true
	default:
		return nil, false
	}
}

func currentUserIDFromContext(c *gin.Context) (uint64, bool) {
	userVal, ok := c.Get("user_id")
	if !ok {
		return 0, false
	}
	userID, ok := userVal.(uint64)
	if !ok || userID == 0 {
		return 0, false
	}
	return userID, true
}

func (h *Handler) ListCollections(c *gin.Context) {
	var (
		page, _  = strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ = strconv.Atoi(c.DefaultQuery("page_size", "20"))
	)
	query := strings.TrimSpace(c.Query("q"))
	categoryID := strings.TrimSpace(c.Query("category_id"))
	categoryIDs := strings.TrimSpace(c.Query("category_ids"))
	ipID := strings.TrimSpace(c.Query("ip_id"))
	featuredRaw := strings.TrimSpace(c.Query("is_featured"))
	if featuredRaw == "" {
		featuredRaw = strings.TrimSpace(c.Query("featured"))
	}
	featured, ok := parseOptionalBoolParam(featuredRaw)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid is_featured"})
		return
	}
	sortField := strings.ToLower(strings.TrimSpace(c.Query("sort")))
	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("order")))
	status := strings.TrimSpace(c.Query("status"))
	visibility := strings.TrimSpace(c.Query("visibility"))
	mediaType, ok := normalizeCollectionMediaType(c.DefaultQuery("media_type", "all"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid media_type"})
		return
	}
	previewCount, _ := strconv.Atoi(c.DefaultQuery("preview_count", "0"))
	adminView := isAdminRole(c)

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

	db := h.db.Model(&models.Collection{})
	if query != "" {
		like := "%" + query + "%"
		tagSub := h.db.Table("taxonomy.collection_tags AS ct").
			Select("ct.collection_id").
			Joins("JOIN taxonomy.tags t ON t.id = ct.tag_id").
			Where("t.name ILIKE ? OR t.slug ILIKE ?", like, like)
		db = db.Where("title ILIKE ? OR slug ILIKE ? OR description ILIKE ? OR id IN (?)", like, like, like, tagSub)
	}

	// 支持多分类ID查询 (category_ids 优先于 category_id)
	if categoryIDs != "" {
		idStrs := strings.Split(categoryIDs, ",")
		var ids []uint64
		for _, idStr := range idStrs {
			id, err := strconv.ParseUint(strings.TrimSpace(idStr), 10, 64)
			if err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			db = db.Where("category_id IN ?", ids)
		}
	} else if categoryID != "" {
		if categoryID == "0" {
			db = db.Where("category_id IS NULL")
		} else {
			cid, err := strconv.ParseUint(categoryID, 10, 64)
			if err != nil || cid == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category_id"})
				return
			}
			db = db.Where("category_id = ?", cid)
		}
	}
	if ipID != "" {
		if ipID == "0" {
			db = db.Where("ip_id IS NULL")
		} else {
			iid, err := strconv.ParseUint(ipID, 10, 64)
			if err != nil || iid == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ip_id"})
				return
			}
			db = db.Where("ip_id = ?", iid)
		}
	}
	if featured != nil {
		db = db.Where("is_featured = ?", *featured)
	}

	animatedExists := `
		EXISTS (
			SELECT 1
			FROM archive.emojis e
			WHERE e.collection_id = archive.collections.id
			  AND e.deleted_at IS NULL
			  AND (%s)
		)
	`
	nonAnimatedExists := `
		EXISTS (
			SELECT 1
			FROM archive.emojis e
			WHERE e.collection_id = archive.collections.id
			  AND e.deleted_at IS NULL
			  AND NOT (%s)
		)
	`
	if !adminView {
		animatedExists = strings.Replace(animatedExists, "AND (%s)", "AND e.status = 'active'\n\t\t\t  AND (%s)", 1)
		nonAnimatedExists = strings.Replace(nonAnimatedExists, "AND NOT (%s)", "AND e.status = 'active'\n\t\t\t  AND NOT (%s)", 1)
	}
	animatedExpr := `LOWER(COALESCE(e.format, '')) LIKE '%gif%' OR LOWER(COALESCE(e.file_url, '')) LIKE '%.gif%'`
	switch mediaType {
	case "animated":
		db = db.Where(fmt.Sprintf(animatedExists, animatedExpr))
	case "static":
		db = db.Where(fmt.Sprintf(nonAnimatedExists, animatedExpr)).
			Where("NOT (" + fmt.Sprintf(animatedExists, animatedExpr) + ")")
	}

	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	orderBy := "archive.collections.id desc"
	switch sortField {
	case "created_at":
		orderBy = "archive.collections.created_at " + sortOrder + ", archive.collections.id desc"
	case "file_count":
		orderBy = "archive.collections.file_count " + sortOrder + ", archive.collections.id desc"
	case "id":
		orderBy = "archive.collections.id " + sortOrder
	case "favorite_count":
		db = db.Joins(`
			LEFT JOIN (
				SELECT collection_id, COUNT(*) AS favorite_count
				FROM action.collection_favorites
				GROUP BY collection_id
			) AS favorite_stats ON favorite_stats.collection_id = archive.collections.id
		`)
		orderBy = fmt.Sprintf(
			"COALESCE(favorite_stats.favorite_count, 0) %s, archive.collections.id desc",
			sortOrder,
		)
	case "like_count":
		db = db.Joins(`
			LEFT JOIN (
				SELECT collection_id, COUNT(*) AS like_count
				FROM action.collection_likes
				GROUP BY collection_id
			) AS like_stats ON like_stats.collection_id = archive.collections.id
		`)
		orderBy = fmt.Sprintf(
			"COALESCE(like_stats.like_count, 0) %s, archive.collections.id desc",
			sortOrder,
		)
	case "download_count":
		db = db.Joins(`
			LEFT JOIN (
				SELECT collection_id, COUNT(*) AS download_count
				FROM action.collection_downloads
				GROUP BY collection_id
			) AS download_stats ON download_stats.collection_id = archive.collections.id
		`)
		orderBy = fmt.Sprintf(
			"COALESCE(download_stats.download_count, 0) %s, archive.collections.id desc",
			sortOrder,
		)
	}
	if !adminView {
		db = db.Where("status = ?", "active").Where("visibility = ?", "public")
	} else {
		if status != "" && strings.ToLower(status) != "all" {
			db = db.Where("status = ?", status)
		}
		if visibility != "" && strings.ToLower(visibility) != "all" {
			db = db.Where("visibility = ?", visibility)
		}
	}

	var total int64
	db.Count(&total)

	var items []models.Collection
	db.Offset((page - 1) * limit).Limit(limit).Order(orderBy).Find(&items)

	tagMap := loadCollectionTags(h.db, items)
	creatorMap := loadCreatorProfiles(h.db, items)
	statMap := loadCollectionStats(h.db, items)
	previewMap := loadCollectionPreviewImages(h.db, h.qiniu, items, previewCount, adminView)
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
		respItems = append(respItems, CollectionListItem{
			ID:               item.ID,
			Title:            item.Title,
			Slug:             item.Slug,
			Description:      item.Description,
			CoverURL:         item.CoverURL,
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
			FileCount:        item.FileCount,
			IsFeatured:       item.IsFeatured,
			IsPinned:         item.IsPinned,
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
			PreviewImages:    previewMap[item.ID],
		})
	}

	c.JSON(http.StatusOK, CollectionListResponse{
		Items: respItems,
		Total: total,
	})
}

// GetCollection returns collection detail by id.
// @Summary Get collection detail
// @Tags collections
// @Produce json
// @Param id path int true "collection id"
// @Success 200 {object} CollectionListItem
// @Router /api/collections/{id} [get]
func (h *Handler) GetCollection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var collection models.Collection
	if err := h.db.First(&collection, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !isAdminRole(c) && !isPublicCollectionVisible(collection) {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}

	tagMap := loadCollectionTags(h.db, []models.Collection{collection})
	creatorMap := loadCreatorProfiles(h.db, []models.Collection{collection})
	statMap := loadCollectionStats(h.db, []models.Collection{collection})
	var creatorName string
	var creatorNameZh string
	var creatorNameEn string
	var creatorAvatar string
	if collection.CreatorProfileID != nil {
		if profile, ok := creatorMap[*collection.CreatorProfileID]; ok {
			creatorNameZh = profile.NameZh
			creatorNameEn = profile.NameEn
			creatorAvatar = profile.AvatarURL
			creatorName = pickCreatorDisplayName(profile)
		}
	}
	stats := statMap[collection.ID]
	liked := false
	favorited := false
	if userID, ok := currentUserIDFromContext(c); ok {
		likedMap, favoritedMap := loadCollectionActionState(h.db, []uint64{collection.ID}, userID)
		liked = likedMap[collection.ID]
		favorited = favoritedMap[collection.ID]
	}
	resp := CollectionListItem{
		ID:               collection.ID,
		Title:            collection.Title,
		Slug:             collection.Slug,
		Description:      collection.Description,
		CoverURL:         collection.CoverURL,
		OwnerID:          collection.OwnerID,
		CreatorProfileID: collection.CreatorProfileID,
		CreatorName:      creatorName,
		CreatorNameZh:    creatorNameZh,
		CreatorNameEn:    creatorNameEn,
		CreatorAvatarURL: creatorAvatar,
		FavoriteCount:    stats.FavoriteCount,
		LikeCount:        stats.LikeCount,
		DownloadCount:    stats.DownloadCount,
		Favorited:        favorited,
		Liked:            liked,
		CategoryID:       collection.CategoryID,
		IPID:             collection.IPID,
		ThemeID:          collection.ThemeID,
		Source:           collection.Source,
		QiniuPrefix:      collection.QiniuPrefix,
		FileCount:        collection.FileCount,
		IsFeatured:       collection.IsFeatured,
		IsPinned:         collection.IsPinned,
		PinnedAt:         collection.PinnedAt,
		LatestZipKey:     collection.LatestZipKey,
		LatestZipName:    collection.LatestZipName,
		LatestZipSize:    collection.LatestZipSize,
		LatestZipAt:      collection.LatestZipAt,
		DownloadCode:     collection.DownloadCode,
		Visibility:       collection.Visibility,
		Status:           collection.Status,
		CreatedAt:        collection.CreatedAt,
		UpdatedAt:        collection.UpdatedAt,
		Tags:             tagMap[collection.ID],
	}

	c.JSON(http.StatusOK, resp)
}

type AdminUpdateCollectionRequest struct {
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	CategoryID  *uint64   `json:"category_id"`
	IPID        *uint64   `json:"ip_id"`
	ThemeID     *uint64   `json:"theme_id"`
	CoverURL    *string   `json:"cover_url"`
	Status      *string   `json:"status"`
	Visibility  *string   `json:"visibility"`
	IsFeatured  *bool     `json:"is_featured"`
	IsPinned    *bool     `json:"is_pinned"`
	TagIDs      *[]uint64 `json:"tag_ids"`
}

// AdminUpdateCollection godoc
// @Summary Update collection (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "collection id"
// @Param body body AdminUpdateCollectionRequest true "collection update"
// @Success 200 {object} CollectionListItem
// @Router /api/admin/collections/{id} [put]
func (h *Handler) AdminUpdateCollection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req AdminUpdateCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	var collection models.Collection
	if err := tx.First(&collection, id).Error; err != nil {
		_ = tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.Title != nil {
		collection.Title = strings.TrimSpace(*req.Title)
	}
	if req.Description != nil {
		collection.Description = strings.TrimSpace(*req.Description)
	}
	if req.CategoryID != nil {
		if *req.CategoryID == 0 {
			collection.CategoryID = nil
		} else {
			if err := h.requireLeafCategory(*req.CategoryID); err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					_ = tx.Rollback()
					c.JSON(http.StatusBadRequest, gin.H{"error": "category not found"})
					return
				}
				if errors.Is(err, errCategoryHasChildren) {
					_ = tx.Rollback()
					c.JSON(http.StatusBadRequest, gin.H{"error": "category has children"})
					return
				}
				if errors.Is(err, errInvalidCategory) {
					_ = tx.Rollback()
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category"})
					return
				}
				_ = tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			collection.CategoryID = req.CategoryID
		}
	}
	if req.ThemeID != nil {
		if *req.ThemeID == 0 {
			collection.ThemeID = nil
		} else {
			collection.ThemeID = req.ThemeID
		}
	}
	if req.IPID != nil {
		if *req.IPID == 0 {
			collection.IPID = nil
		} else {
			var ip models.IP
			if err := tx.First(&ip, *req.IPID).Error; err != nil {
				_ = tx.Rollback()
				if err == gorm.ErrRecordNotFound {
					c.JSON(http.StatusBadRequest, gin.H{"error": "ip not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			collection.IPID = req.IPID
		}
	}
	if req.CoverURL != nil {
		collection.CoverURL = strings.TrimSpace(*req.CoverURL)
	}
	if req.Status != nil {
		status, ok := normalizeCollectionStatus(*req.Status)
		if !ok {
			_ = tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		collection.Status = status
	}
	if req.Visibility != nil {
		visibility, ok := normalizeCollectionVisibility(*req.Visibility)
		if !ok {
			_ = tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid visibility"})
			return
		}
		collection.Visibility = visibility
	}
	if req.IsFeatured != nil {
		// 推荐上限保护：仅在从未推荐改为推荐时校验，避免重复保存被误拦截。
		if *req.IsFeatured && !collection.IsFeatured {
			var featuredCount int64
			if err := tx.Model(&models.Collection{}).
				Where("is_featured = ?", true).
				Where("id <> ?", collection.ID).
				Count(&featuredCount).Error; err != nil {
				_ = tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if featuredCount >= maxFeaturedCollections {
				_ = tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{"error": "已有四个推荐，请先取消一个推荐后再保存"})
				return
			}
		}
		collection.IsFeatured = *req.IsFeatured
	}
	if req.IsPinned != nil {
		collection.IsPinned = *req.IsPinned
		if *req.IsPinned {
			if collection.PinnedAt == nil {
				now := time.Now()
				collection.PinnedAt = &now
			}
		} else {
			collection.PinnedAt = nil
		}
	}

	if err := tx.Save(&collection).Error; err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.TagIDs != nil {
		if err := tx.Where("collection_id = ?", collection.ID).Delete(&models.CollectionTag{}).Error; err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		seen := map[uint64]struct{}{}
		for _, tagID := range *req.TagIDs {
			if tagID == 0 {
				continue
			}
			if _, ok := seen[tagID]; ok {
				continue
			}
			seen[tagID] = struct{}{}
			ct := models.CollectionTag{
				CollectionID: collection.ID,
				TagID:        tagID,
			}
			if err := tx.Create(&ct).Error; err != nil {
				_ = tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tagMap := loadCollectionTags(h.db, []models.Collection{collection})
	creatorMap := loadCreatorProfiles(h.db, []models.Collection{collection})
	var creatorName string
	var creatorNameZh string
	var creatorNameEn string
	var creatorAvatar string
	if collection.CreatorProfileID != nil {
		if profile, ok := creatorMap[*collection.CreatorProfileID]; ok {
			creatorNameZh = profile.NameZh
			creatorNameEn = profile.NameEn
			creatorAvatar = profile.AvatarURL
			creatorName = pickCreatorDisplayName(profile)
		}
	}
	resp := CollectionListItem{
		ID:               collection.ID,
		Title:            collection.Title,
		Slug:             collection.Slug,
		Description:      collection.Description,
		CoverURL:         collection.CoverURL,
		OwnerID:          collection.OwnerID,
		CreatorProfileID: collection.CreatorProfileID,
		CreatorName:      creatorName,
		CreatorNameZh:    creatorNameZh,
		CreatorNameEn:    creatorNameEn,
		CreatorAvatarURL: creatorAvatar,
		CategoryID:       collection.CategoryID,
		IPID:             collection.IPID,
		ThemeID:          collection.ThemeID,
		Source:           collection.Source,
		QiniuPrefix:      collection.QiniuPrefix,
		FileCount:        collection.FileCount,
		IsFeatured:       collection.IsFeatured,
		IsPinned:         collection.IsPinned,
		PinnedAt:         collection.PinnedAt,
		Visibility:       collection.Visibility,
		Status:           collection.Status,
		CreatedAt:        collection.CreatedAt,
		UpdatedAt:        collection.UpdatedAt,
		Tags:             tagMap[collection.ID],
	}
	c.JSON(http.StatusOK, resp)
}

func loadCollectionTags(db *gorm.DB, items []models.Collection) map[uint64][]TagBrief {
	tagMap := map[uint64][]TagBrief{}
	if len(items) == 0 {
		return tagMap
	}
	ids := make([]uint64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	type tagRow struct {
		CollectionID uint64
		TagID        uint64
		Name         string
		Slug         string
	}
	var rows []tagRow
	db.Table("taxonomy.collection_tags AS ct").
		Select("ct.collection_id, t.id as tag_id, t.name, t.slug").
		Joins("JOIN taxonomy.tags t ON t.id = ct.tag_id").
		Where("ct.collection_id IN ?", ids).
		Scan(&rows)
	for _, row := range rows {
		tagMap[row.CollectionID] = append(tagMap[row.CollectionID], TagBrief{
			ID:   row.TagID,
			Name: row.Name,
			Slug: row.Slug,
		})
	}
	return tagMap
}

func loadCreatorProfiles(db *gorm.DB, items []models.Collection) map[uint64]models.CreatorProfile {
	creatorMap := map[uint64]models.CreatorProfile{}
	if len(items) == 0 {
		return creatorMap
	}
	ids := make([]uint64, 0, len(items))
	seen := map[uint64]struct{}{}
	for _, item := range items {
		if item.CreatorProfileID == nil || *item.CreatorProfileID == 0 {
			continue
		}
		if _, ok := seen[*item.CreatorProfileID]; ok {
			continue
		}
		seen[*item.CreatorProfileID] = struct{}{}
		ids = append(ids, *item.CreatorProfileID)
	}
	if len(ids) == 0 {
		return creatorMap
	}
	var profiles []models.CreatorProfile
	db.Where("id IN ?", ids).Find(&profiles)
	for _, profile := range profiles {
		creatorMap[profile.ID] = profile
	}
	return creatorMap
}

type collectionStats struct {
	FavoriteCount int64
	LikeCount     int64
	DownloadCount int64
}

func loadCollectionActionState(db *gorm.DB, collectionIDs []uint64, userID uint64) (map[uint64]bool, map[uint64]bool) {
	likedMap := map[uint64]bool{}
	favoritedMap := map[uint64]bool{}
	if userID == 0 || len(collectionIDs) == 0 {
		return likedMap, favoritedMap
	}

	type row struct {
		CollectionID uint64
	}

	var likeRows []row
	db.Table("action.collection_likes").
		Select("collection_id").
		Where("user_id = ? AND collection_id IN ?", userID, collectionIDs).
		Scan(&likeRows)
	for _, item := range likeRows {
		likedMap[item.CollectionID] = true
	}

	var favoriteRows []row
	db.Table("action.collection_favorites").
		Select("collection_id").
		Where("user_id = ? AND collection_id IN ?", userID, collectionIDs).
		Scan(&favoriteRows)
	for _, item := range favoriteRows {
		favoritedMap[item.CollectionID] = true
	}

	return likedMap, favoritedMap
}

func loadCollectionStats(db *gorm.DB, items []models.Collection) map[uint64]collectionStats {
	statMap := map[uint64]collectionStats{}
	if len(items) == 0 {
		return statMap
	}
	ids := make([]uint64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}

	type countRow struct {
		CollectionID uint64
		Count        int64
	}

	var favoriteRows []countRow
	db.Table("action.collection_favorites").
		Select("collection_id, COUNT(*) AS count").
		Where("collection_id IN ?", ids).
		Group("collection_id").
		Scan(&favoriteRows)
	for _, row := range favoriteRows {
		stats := statMap[row.CollectionID]
		stats.FavoriteCount = row.Count
		statMap[row.CollectionID] = stats
	}

	var likeRows []countRow
	db.Table("action.collection_likes").
		Select("collection_id, COUNT(*) AS count").
		Where("collection_id IN ?", ids).
		Group("collection_id").
		Scan(&likeRows)
	for _, row := range likeRows {
		stats := statMap[row.CollectionID]
		stats.LikeCount = row.Count
		statMap[row.CollectionID] = stats
	}

	var zipDownloadRows []countRow
	db.Table("action.collection_downloads AS cd").
		Select("cd.collection_id AS collection_id, COUNT(*) AS count").
		Where("cd.collection_id IN ?", ids).
		Group("cd.collection_id").
		Scan(&zipDownloadRows)
	for _, row := range zipDownloadRows {
		stats := statMap[row.CollectionID]
		stats.DownloadCount += row.Count
		statMap[row.CollectionID] = stats
	}

	return statMap
}

type collectionPreviewRow struct {
	CollectionID uint64
	FileURL      string
	RankNum      int
}

func loadCollectionPreviewImages(
	db *gorm.DB,
	qiniu *storage.QiniuClient,
	items []models.Collection,
	previewCount int,
	adminView bool,
) map[uint64][]string {
	result := map[uint64][]string{}
	if previewCount <= 0 || len(items) == 0 {
		return result
	}
	ids := make([]uint64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}

	statusFilter := "active"
	if adminView {
		statusFilter = ""
	}
	rows := make([]collectionPreviewRow, 0)
	query := `
SELECT t.collection_id, t.file_url, t.rank_num
FROM (
	SELECT
		e.collection_id,
		e.file_url,
		ROW_NUMBER() OVER (
			PARTITION BY e.collection_id
			ORDER BY COALESCE(e.display_order, 2147483647), e.id
		) AS rank_num
	FROM archive.emojis e
	WHERE e.collection_id IN ?
	  AND e.deleted_at IS NULL
`
	if statusFilter != "" {
		query += " AND e.status = ?\n"
	}
	query += `
) t
WHERE t.rank_num <= ?
ORDER BY t.collection_id, t.rank_num
`

	var err error
	if statusFilter != "" {
		err = db.Raw(query, ids, statusFilter, previewCount).Scan(&rows).Error
	} else {
		err = db.Raw(query, ids, previewCount).Scan(&rows).Error
	}
	if err != nil {
		return result
	}

	for _, row := range rows {
		if len(result[row.CollectionID]) >= previewCount {
			continue
		}
		previewURL := resolvePreviewURL(row.FileURL, qiniu)
		if strings.TrimSpace(previewURL) == "" {
			continue
		}
		result[row.CollectionID] = append(result[row.CollectionID], previewURL)
	}
	return result
}

func pickCreatorDisplayName(profile models.CreatorProfile) string {
	if profile.NameEn != "" && profile.ID%2 == 0 {
		return profile.NameEn
	}
	if profile.NameZh != "" {
		return profile.NameZh
	}
	return profile.NameEn
}

func ensureCreatorProfileID(db *gorm.DB, collection *models.Collection) error {
	if collection.CreatorProfileID != nil && *collection.CreatorProfileID > 0 {
		return nil
	}
	var id uint64
	if err := db.Table("ops.creator_profiles").
		Select("id").
		Where("status = ?", "active").
		Order("random()").
		Limit(1).
		Scan(&id).Error; err != nil {
		return err
	}
	if id == 0 {
		return nil
	}
	collection.CreatorProfileID = &id
	return nil
}

func (h *Handler) CreateCollection(c *gin.Context) {
	var collection models.Collection
	if err := c.ShouldBindJSON(&collection); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if collection.CategoryID != nil {
		if *collection.CategoryID == 0 {
			collection.CategoryID = nil
		} else if err := h.requireLeafCategory(*collection.CategoryID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "category not found"})
				return
			}
			if errors.Is(err, errCategoryHasChildren) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "category has children"})
				return
			}
			if errors.Is(err, errInvalidCategory) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if collection.IPID != nil {
		if *collection.IPID == 0 {
			collection.IPID = nil
		} else {
			var ip models.IP
			if err := h.db.First(&ip, *collection.IPID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "ip not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	if err := ensureCreatorProfileID(h.db, &collection); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign creator profile"})
		return
	}

	collection.Status = "active"
	code, err := ensureCollectionDownloadCode(h.db, collection.DownloadCode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate download code"})
		return
	}
	collection.DownloadCode = code
	if err := h.db.Create(&collection).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create collection"})
		return
	}

	c.JSON(http.StatusCreated, collection)
}

func (h *Handler) UpdateCollection(c *gin.Context) {
	id := c.Param("id")
	var collection models.Collection
	if err := h.db.First(&collection, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}

	if err := c.ShouldBindJSON(&collection); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if collection.CategoryID != nil {
		if *collection.CategoryID == 0 {
			collection.CategoryID = nil
		} else if err := h.requireLeafCategory(*collection.CategoryID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "category not found"})
				return
			}
			if errors.Is(err, errCategoryHasChildren) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "category has children"})
				return
			}
			if errors.Is(err, errInvalidCategory) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if collection.IPID != nil {
		if *collection.IPID == 0 {
			collection.IPID = nil
		} else {
			var ip models.IP
			if err := h.db.First(&ip, *collection.IPID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "ip not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	if err := h.db.Save(&collection).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update collection"})
		return
	}

	c.JSON(http.StatusOK, collection)
}

func (h *Handler) DeleteCollection(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&models.Collection{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete collection"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// AdminDeleteCollection godoc
// @Summary Hard delete collection (admin)
// @Tags admin
// @Produce json
// @Param id path int true "collection id"
// @Success 200 {object} map[string]interface{}
// @Router /api/admin/collections/{id} [delete]
func (h *Handler) AdminDeleteCollection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var collection models.Collection
	if err := h.db.First(&collection, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	prefix := normalizeCollectionPrefix(collection.QiniuPrefix)
	deletedObjects := 0
	if prefix != "" {
		if h.qiniu == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
			return
		}
		if prefix == "emoji/" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsafe qiniu prefix"})
			return
		}
		count, err := h.deleteQiniuPrefixObjects(prefix)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to delete qiniu objects: %v", err)})
			return
		}
		deletedObjects = count
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		return tx.Unscoped().Delete(&models.Collection{}, collection.ID).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hard delete collection"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "hard deleted",
		"collection_id":   collection.ID,
		"deleted_objects": deletedObjects,
		"qiniu_prefix":    prefix,
	})
}

func normalizeCollectionPrefix(prefix string) string {
	clean := strings.TrimLeft(strings.TrimSpace(prefix), "/")
	if clean == "" {
		return ""
	}
	if !strings.HasSuffix(clean, "/") {
		clean += "/"
	}
	return clean
}

func (h *Handler) deleteQiniuPrefixObjects(prefix string) (int, error) {
	bm := h.qiniu.BucketManager()
	marker := ""
	deleted := 0

	for {
		items, _, nextMarker, hasNext, err := bm.ListFiles(h.qiniu.Bucket, prefix, "", marker, 1000)
		if err != nil {
			return deleted, err
		}
		if len(items) == 0 && !hasNext {
			break
		}

		ops := make([]string, 0, len(items))
		for _, item := range items {
			key := strings.TrimSpace(item.Key)
			if key == "" {
				continue
			}
			ops = append(ops, qiniustorage.URIDelete(h.qiniu.Bucket, key))
		}

		if len(ops) > 0 {
			rets, err := bm.Batch(ops)
			if err != nil {
				return deleted, err
			}
			for _, ret := range rets {
				// 612 means the object is already absent; treat as deleted for idempotency.
				if ret.Code == 200 || ret.Code == 612 {
					deleted++
					continue
				}
				return deleted, fmt.Errorf("batch delete failed with code=%d", ret.Code)
			}
		}

		if !hasNext || nextMarker == "" {
			break
		}
		marker = nextMarker
	}

	return deleted, nil
}
