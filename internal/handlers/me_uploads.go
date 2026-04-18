package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

const (
	ugcCollectionSource = "ugc_upload"
)

type MyUploadCollectionsResponse struct {
	Items    []CollectionListItem `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

type MyUploadEmojisResponse struct {
	Items    []EmojiListItem `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

type CreateMyUploadCollectionRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type UpdateMyUploadCollectionRequest struct {
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	CoverEmojiID *uint64 `json:"cover_emoji_id"`
}

type UpdateMyUploadEmojiRequest struct {
	Title        *string `json:"title"`
	DisplayOrder *int    `json:"display_order"`
}

type UploadMyCollectionEmojisResponse struct {
	CollectionID   uint64          `json:"collection_id"`
	Added          int             `json:"added"`
	FileCount      int64           `json:"file_count"`
	MaxAllowed     int             `json:"max_allowed"`
	RemainingQuota int64           `json:"remaining_quota"`
	CoverURL       string          `json:"cover_url,omitempty"`
	Items          []EmojiListItem `json:"items"`
}

func normalizeUGCSource(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func isUGCCollection(collection models.Collection) bool {
	return normalizeUGCSource(collection.Source) == ugcCollectionSource
}

func clampPageSize(value, def, max int) int {
	if value <= 0 {
		return def
	}
	if value > max {
		return max
	}
	return value
}

func trimToRuneLimit(raw string, max int) string {
	trimmed := strings.TrimSpace(raw)
	if max <= 0 || utf8.RuneCountInString(trimmed) <= max {
		return trimmed
	}
	rs := []rune(trimmed)
	return strings.TrimSpace(string(rs[:max]))
}

func buildUGCCollectionSlug(title string, userID uint64) string {
	base := slugify(title)
	if base == "" {
		base = "ugc"
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	slug := fmt.Sprintf("%s-u%d-%s", base, userID, suffix)
	if len(slug) <= 120 {
		return slug
	}
	head := trimToRuneLimit(base, 48)
	if head == "" {
		head = "ugc"
	}
	return fmt.Sprintf("%s-u%d-%s", head, userID, suffix)
}

func (h *Handler) buildUGCCollectionPrefix(userID, collectionID uint64) string {
	base := strings.TrimSuffix(h.qiniuUserUGCPrefix(userID), "/")
	return path.Join(base, strconv.FormatUint(collectionID, 10)) + "/"
}

func (h *Handler) loadMyUGCCollectionByID(userID, collectionID uint64) (models.Collection, error) {
	var collection models.Collection
	if err := h.db.
		Where("id = ?", collectionID).
		Where("owner_id = ?", userID).
		Where("source = ?", ugcCollectionSource).
		Where("owner_deleted_at IS NULL").
		First(&collection).Error; err != nil {
		return models.Collection{}, err
	}
	return collection, nil
}

func validateUGCUploadFile(fileName string, size int64, runtime uploadRuleRuntime) (string, error) {
	base := strings.TrimSpace(path.Base(fileName))
	if base == "" {
		return "", errors.New("invalid file name")
	}
	ext := strings.ToLower(strings.TrimSpace(path.Ext(base)))
	normalizedExt := strings.TrimPrefix(ext, ".")
	if _, ok := runtime.AllowedExtSet[normalizedExt]; !ok {
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}
	if size <= 0 {
		return "", errors.New("empty file")
	}
	if size > runtime.MaxFileSizeBytes {
		return "", fmt.Errorf("file too large: %s (max %d bytes)", base, runtime.MaxFileSizeBytes)
	}
	return ext, nil
}

func loadCollectionEmojiCountMap(db *gorm.DB, collectionIDs []uint64) (map[uint64]int64, error) {
	result := map[uint64]int64{}
	if db == nil || len(collectionIDs) == 0 {
		return result, nil
	}
	uniq := make([]uint64, 0, len(collectionIDs))
	seen := map[uint64]struct{}{}
	for _, id := range collectionIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return result, nil
	}

	type countRow struct {
		CollectionID uint64 `gorm:"column:collection_id"`
		Count        int64  `gorm:"column:count"`
	}
	var rows []countRow
	if err := db.Model(&models.Emoji{}).
		Select("collection_id, COUNT(*) AS count").
		Where("collection_id IN ?", uniq).
		Where("deleted_at IS NULL").
		Group("collection_id").
		Scan(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		if row.CollectionID == 0 {
			continue
		}
		if row.Count < 0 {
			row.Count = 0
		}
		result[row.CollectionID] = row.Count
	}
	return result, nil
}

func (h *Handler) mapMyUploadCollections(items []models.Collection, userID uint64, previewCount int) []CollectionListItem {
	if len(items) == 0 {
		return []CollectionListItem{}
	}
	if previewCount <= 0 {
		previewCount = 15
	}
	if previewCount > 30 {
		previewCount = 30
	}
	statMap := loadCollectionStats(h.db, items)
	previewAssetMap := loadCollectionPreviewAssets(h.db, h.qiniu, items, previewCount, true)
	reviewStateMap := h.loadUGCReviewStateMap(items)
	collectionIDs := make([]uint64, 0, len(items))
	for _, item := range items {
		collectionIDs = append(collectionIDs, item.ID)
	}
	emojiCountMap, emojiCountErr := loadCollectionEmojiCountMap(h.db, collectionIDs)
	likedMap, favoritedMap := loadCollectionActionState(h.db, collectionIDs, userID)
	resp := make([]CollectionListItem, 0, len(items))
	for _, item := range items {
		stats := statMap[item.ID]
		reviewState := reviewStateMap[item.ID]
		fileCount := item.FileCount
		if emojiCountErr == nil {
			if realCount, ok := emojiCountMap[item.ID]; ok {
				fileCount = int(realCount)
			} else {
				fileCount = 0
			}
		}
		if fileCount < 0 {
			fileCount = 0
		}
		resp = append(resp, CollectionListItem{
			ID:            item.ID,
			Title:         item.Title,
			Slug:          item.Slug,
			Description:   item.Description,
			CoverKey:      resolveCollectionCoverKey(item.CoverURL, h.qiniu),
			CoverURL:      resolveListStaticPreviewURL(item.CoverURL, h.qiniu),
			OwnerID:       item.OwnerID,
			Source:        item.Source,
			FileCount:     fileCount,
			PreviewImages: flattenPreviewAssetsToImages(previewAssetMap[item.ID]),
			PreviewAssets: previewAssetMap[item.ID],
			FavoriteCount: stats.FavoriteCount,
			LikeCount:     stats.LikeCount,
			DownloadCount: stats.DownloadCount,
			Liked:         likedMap[item.ID],
			Favorited:     favoritedMap[item.ID],
			CreatedAt:     item.CreatedAt,
			UpdatedAt:     item.UpdatedAt,
			Visibility:    item.Visibility,
			Status:        item.Status,
			ReviewStatus:  reviewState.ReviewStatus,
			PublishStatus: reviewState.PublishStatus,
		})
	}
	return resp
}

// ListMyUploadCollections lists current user's uploaded collections.
// @Summary List my uploaded collections
// @Tags user
// @Produce json
// @Param page query int false "page"
// @Param page_size query int false "page size"
// @Param preview_count query int false "preview count"
// @Success 200 {object} MyUploadCollectionsResponse
// @Router /api/me/uploads/collections [get]
func (h *Handler) ListMyUploadCollections(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	page := parseIntQuery(c, "page", 1)
	pageSize := clampPageSize(parseIntQuery(c, "page_size", 12), 12, 60)
	previewCount := clampPageSize(parseIntQuery(c, "preview_count", 15), 15, 30)
	keyword := strings.TrimSpace(c.Query("q"))
	status := strings.TrimSpace(c.Query("status"))
	visibility := strings.TrimSpace(c.Query("visibility"))
	sortField := strings.ToLower(strings.TrimSpace(c.DefaultQuery("sort", "updated_at")))
	sortOrder := strings.ToLower(strings.TrimSpace(c.DefaultQuery("order", "desc")))
	if page <= 0 {
		page = 1
	}
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	query := h.db.Model(&models.Collection{}).
		Where("owner_id = ?", userID).
		Where("source = ?", ugcCollectionSource).
		Where("owner_deleted_at IS NULL")

	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("(title ILIKE ? OR description ILIKE ?)", like, like)
	}
	if status != "" && strings.ToLower(status) != "all" {
		if normalized, ok := normalizeCollectionStatus(status); ok {
			query = query.Where("status = ?", normalized)
		}
	}
	if visibility != "" && strings.ToLower(visibility) != "all" {
		if normalized, ok := normalizeCollectionVisibility(visibility); ok {
			query = query.Where("visibility = ?", normalized)
		}
	}

	orderBy := "updated_at DESC, id DESC"
	switch sortField {
	case "created_at":
		orderBy = "created_at " + sortOrder + ", id DESC"
	case "file_count":
		orderBy = "file_count " + sortOrder + ", id DESC"
	case "title":
		orderBy = "title " + sortOrder + ", id DESC"
	case "like_count":
		query = query.Joins(`
			LEFT JOIN (
				SELECT collection_id, COUNT(*) AS like_count
				FROM action.collection_likes
				GROUP BY collection_id
			) AS like_stats ON like_stats.collection_id = id
		`)
		orderBy = fmt.Sprintf("COALESCE(like_stats.like_count, 0) %s, updated_at DESC, id DESC", sortOrder)
	case "favorite_count":
		query = query.Joins(`
			LEFT JOIN (
				SELECT collection_id, COUNT(*) AS favorite_count
				FROM action.collection_favorites
				GROUP BY collection_id
			) AS favorite_stats ON favorite_stats.collection_id = id
		`)
		orderBy = fmt.Sprintf("COALESCE(favorite_stats.favorite_count, 0) %s, updated_at DESC, id DESC", sortOrder)
	case "download_count":
		query = query.Joins(`
			LEFT JOIN (
				SELECT collection_id, COUNT(*) AS download_count
				FROM action.collection_downloads
				GROUP BY collection_id
			) AS download_stats ON download_stats.collection_id = id
		`)
		orderBy = fmt.Sprintf("COALESCE(download_stats.download_count, 0) %s, updated_at DESC, id DESC", sortOrder)
	case "interaction_count", "total_interactions":
		query = query.Joins(`
			LEFT JOIN (
				SELECT collection_id, COUNT(*) AS like_count
				FROM action.collection_likes
				GROUP BY collection_id
			) AS like_stats ON like_stats.collection_id = id
		`).Joins(`
			LEFT JOIN (
				SELECT collection_id, COUNT(*) AS favorite_count
				FROM action.collection_favorites
				GROUP BY collection_id
			) AS favorite_stats ON favorite_stats.collection_id = id
		`).Joins(`
			LEFT JOIN (
				SELECT collection_id, COUNT(*) AS download_count
				FROM action.collection_downloads
				GROUP BY collection_id
			) AS download_stats ON download_stats.collection_id = id
		`)
		orderBy = fmt.Sprintf(
			"(COALESCE(like_stats.like_count, 0) + COALESCE(favorite_stats.favorite_count, 0) + COALESCE(download_stats.download_count, 0)) %s, updated_at DESC, id DESC",
			sortOrder,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list collections"})
		return
	}

	var items []models.Collection
	offset := (page - 1) * pageSize
	if err := query.Order(orderBy).Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list collections"})
		return
	}

	c.JSON(http.StatusOK, MyUploadCollectionsResponse{
		Items:    h.mapMyUploadCollections(items, userID, previewCount),
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// CreateMyUploadCollection creates a new user-owned upload collection.
// @Summary Create my uploaded collection
// @Tags user
// @Accept json
// @Produce json
// @Param body body CreateMyUploadCollectionRequest true "create request"
// @Success 201 {object} CollectionListItem
// @Router /api/me/uploads/collections [post]
func (h *Handler) CreateMyUploadCollection(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreateMyUploadCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	title := trimToRuneLimit(req.Title, 80)
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title required"})
		return
	}
	description := trimToRuneLimit(req.Description, 500)
	runtime := h.loadUploadRuleRuntime()
	if !runtime.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "upload_disabled"})
		return
	}

	collectionStatus := "active"
	if runtime.AutoAuditEnabled && runtime.AutoActivateOnPass {
		collectionStatus = "active"
	}

	collection := models.Collection{
		Title:       title,
		Slug:        buildUGCCollectionSlug(title, userID),
		Description: description,
		OwnerID:     userID,
		Source:      ugcCollectionSource,
		FileCount:   0,
		Status:      collectionStatus,
		Visibility:  "private",
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start transaction"})
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback().Error
		}
	}()

	if err := ensureCreatorProfileID(tx, &collection); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign creator profile"})
		return
	}
	code, err := ensureCollectionDownloadCode(tx, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate download code"})
		return
	}
	collection.DownloadCode = code
	autoCode, err := ensureCollectionAutoTagCode(tx, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate auto tag code"})
		return
	}
	collection.AutoTagCode = autoCode

	if err := tx.Create(&collection).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create collection"})
		return
	}
	collection.QiniuPrefix = h.buildUGCCollectionPrefix(userID, collection.ID)
	if err := tx.Model(&models.Collection{}).Where("id = ?", collection.ID).Update("qiniu_prefix", collection.QiniuPrefix).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set qiniu prefix"})
		return
	}
	reviewState := models.UGCCollectionReviewState{
		CollectionID:         collection.ID,
		OwnerID:              userID,
		ReviewStatus:         ugcReviewStatusDraft,
		PublishStatus:        ugcPublishStatusOffline,
		SubmitCount:          0,
		LastContentChangedAt: time.Now(),
	}
	if err := tx.Create(&reviewState).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to init review state"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit transaction"})
		return
	}
	committed = true

	respItems := h.mapMyUploadCollections([]models.Collection{collection}, userID, 12)
	if len(respItems) == 0 {
		c.JSON(http.StatusCreated, CollectionListItem{
			ID:          collection.ID,
			Title:       collection.Title,
			Slug:        collection.Slug,
			Description: collection.Description,
			OwnerID:     collection.OwnerID,
			Source:      collection.Source,
			FileCount:   collection.FileCount,
			Visibility:  collection.Visibility,
			Status:      collection.Status,
			CreatedAt:   collection.CreatedAt,
			UpdatedAt:   collection.UpdatedAt,
		})
		return
	}
	c.JSON(http.StatusCreated, respItems[0])
}

// GetMyUploadCollection returns current user's upload collection detail.
// @Summary Get my uploaded collection
// @Tags user
// @Produce json
// @Param id path int true "collection id"
// @Success 200 {object} CollectionListItem
// @Router /api/me/uploads/collections/{id} [get]
func (h *Handler) GetMyUploadCollection(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	collection, err := h.loadMyUGCCollectionByID(userID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}
	respItems := h.mapMyUploadCollections([]models.Collection{collection}, userID, 15)
	if len(respItems) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	c.JSON(http.StatusOK, respItems[0])
}

// UpdateMyUploadCollection updates current user's upload collection metadata.
// @Summary Update my uploaded collection
// @Tags user
// @Accept json
// @Produce json
// @Param id path int true "collection id"
// @Param body body UpdateMyUploadCollectionRequest true "update request"
// @Success 200 {object} CollectionListItem
// @Router /api/me/uploads/collections/{id} [put]
func (h *Handler) UpdateMyUploadCollection(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req UpdateMyUploadCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	collection, err := h.loadMyUGCCollectionByID(userID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}
	if strings.EqualFold(strings.TrimSpace(collection.Status), "disabled") {
		c.JSON(http.StatusForbidden, gin.H{"error": "collection is disabled"})
		return
	}

	updates := map[string]interface{}{}
	if req.Title != nil {
		title := trimToRuneLimit(*req.Title, 80)
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "title cannot be empty"})
			return
		}
		updates["title"] = title
	}
	if req.Description != nil {
		updates["description"] = trimToRuneLimit(*req.Description, 500)
	}
	if req.CoverEmojiID != nil {
		if *req.CoverEmojiID == 0 {
			updates["cover_url"] = ""
		} else {
			var emoji models.Emoji
			if err := h.db.Where("id = ? AND collection_id = ? AND deleted_at IS NULL", *req.CoverEmojiID, collection.ID).First(&emoji).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "cover emoji not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update collection"})
				return
			}
			updates["cover_url"] = strings.TrimSpace(emoji.FileURL)
		}
	}

	if len(updates) > 0 {
		if _, err := h.ensureUGCCollectionMutable(collection); err != nil {
			if isUGCCollectionUnderReviewError(err) {
				c.JSON(http.StatusConflict, gin.H{"error": "collection is under review"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review state"})
			return
		}
		if err := h.db.Model(&models.Collection{}).Where("id = ?", collection.ID).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update collection"})
			return
		}
		if err := h.touchUGCReviewAfterContentChange(collection, userID, "update_collection_metadata"); err != nil {
			if isUGCCollectionUnderReviewError(err) {
				c.JSON(http.StatusConflict, gin.H{"error": "collection is under review"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update review state"})
			return
		}
	}

	updated, err := h.loadMyUGCCollectionByID(userID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update collection"})
		return
	}
	respItems := h.mapMyUploadCollections([]models.Collection{updated}, userID, 15)
	if len(respItems) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	c.JSON(http.StatusOK, respItems[0])
}

// DeleteMyUploadCollection soft-deletes current user's upload collection (hide in user side, keep for admin).
// @Summary Delete my uploaded collection
// @Tags user
// @Produce json
// @Param id path int true "collection id"
// @Success 200 {object} map[string]interface{}
// @Router /api/me/uploads/collections/{id} [delete]
func (h *Handler) DeleteMyUploadCollection(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	collection, err := h.loadMyUGCCollectionByID(userID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	now := time.Now()
	if err := tx.Model(&models.Collection{}).
		Where("id = ?", collection.ID).
		Where("owner_id = ?", userID).
		Where("source = ?", ugcCollectionSource).
		Where("owner_deleted_at IS NULL").
		Updates(map[string]interface{}{
			"owner_deleted_at": now,
			"visibility":       "private",
			"updated_at":       now,
		}).Error; err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete collection"})
		return
	}

	if state, err := h.getOrCreateUGCReviewState(tx, collection); err == nil {
		_ = tx.Model(&models.UGCCollectionReviewState{}).
			Where("collection_id = ?", collection.ID).
			Updates(map[string]interface{}{
				"publish_status": "offline",
				"offline_reason": "用户删除",
				"updated_at":     now,
			}).Error

		snapshot := map[string]interface{}{
			"review_status":  state.ReviewStatus,
			"publish_status": "offline",
			"reason":         "owner_soft_deleted",
		}
		_ = tx.Create(&models.UGCCollectionReviewLog{
			CollectionID:      collection.ID,
			OwnerID:           collection.OwnerID,
			Action:            ugcReviewActionContentEdit,
			FromReviewStatus:  state.ReviewStatus,
			ToReviewStatus:    state.ReviewStatus,
			FromPublishStatus: state.PublishStatus,
			ToPublishStatus:   ugcPublishStatusOffline,
			OperatorRole:      "user",
			OperatorID:        userID,
			Reason:            "owner_soft_deleted",
			SnapshotJSON:      toJSONBOrDefault(snapshot, nil),
		}).Error
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete collection"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "deleted",
		"soft_deleted":  true,
		"collection_id": collection.ID,
	})
}

// ListMyUploadCollectionEmojis lists emojis in a current user's upload collection.
// @Summary List my uploaded collection emojis
// @Tags user
// @Produce json
// @Param id path int true "collection id"
// @Param page query int false "page"
// @Param page_size query int false "page size"
// @Success 200 {object} MyUploadEmojisResponse
// @Router /api/me/uploads/collections/{id}/emojis [get]
func (h *Handler) ListMyUploadCollectionEmojis(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if _, err := h.loadMyUGCCollectionByID(userID, collectionID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}

	page := parseIntQuery(c, "page", 1)
	pageSize := clampPageSize(parseIntQuery(c, "page_size", 50), 50, 100)
	if page <= 0 {
		page = 1
	}

	query := h.db.Model(&models.Emoji{}).Where("collection_id = ?", collectionID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list emojis"})
		return
	}

	var items []models.Emoji
	offset := (page - 1) * pageSize
	if err := query.Order("display_order ASC, id ASC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list emojis"})
		return
	}

	c.JSON(http.StatusOK, MyUploadEmojisResponse{
		Items:    enrichEmojiListItemsWithEngagement(h.db, mapEmojiItems(items, h.qiniu, false), userID),
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// UploadMyCollectionEmojis uploads emojis into current user's upload collection.
// @Summary Upload emojis to my collection
// @Tags user
// @Accept multipart/form-data
// @Produce json
// @Param id path int true "collection id"
// @Param files formData file true "emoji files" collectionFormat multi
// @Success 200 {object} UploadMyCollectionEmojisResponse
// @Router /api/me/uploads/collections/{id}/emojis/upload [post]
func (h *Handler) UploadMyCollectionEmojis(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	runtime := h.loadUploadRuleRuntime()
	if !runtime.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "upload_disabled"})
		return
	}

	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	collection, err := h.loadMyUGCCollectionByID(userID, collectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}
	if strings.EqualFold(strings.TrimSpace(collection.Status), "disabled") {
		c.JSON(http.StatusForbidden, gin.H{"error": "collection is disabled"})
		return
	}
	if _, err := h.ensureUGCCollectionMutable(collection); err != nil {
		if isUGCCollectionUnderReviewError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "collection is under review"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review state"})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse form"})
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "files required"})
		return
	}
	if len(files) > runtime.MaxFilesPerRequest {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("too many files per request, max %d", runtime.MaxFilesPerRequest)})
		return
	}

	var existingCount int64
	if err := h.db.Model(&models.Emoji{}).Where("collection_id = ?", collection.ID).Count(&existingCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check collection size"})
		return
	}
	if existingCount+int64(len(files)) > int64(runtime.MaxFilesPerCollection) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":         "collection_emoji_limit_exceeded",
			"message":       fmt.Sprintf("单个合集最多上传 %d 张表情", runtime.MaxFilesPerCollection),
			"max_allowed":   runtime.MaxFilesPerCollection,
			"current_count": existingCount,
		})
		return
	}

	for _, f := range files {
		if _, err := validateUGCUploadFile(f.Filename, f.Size, runtime); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	prefix := strings.TrimSpace(collection.QiniuPrefix)
	if prefix == "" {
		prefix = h.buildUGCCollectionPrefix(userID, collection.ID)
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	rawPrefix := prefix + "raw/"

	uploader := qiniustorage.NewFormUploader(h.qiniu.Cfg)
	var maxDisplayOrder int
	_ = h.db.Model(&models.Emoji{}).
		Select("COALESCE(MAX(display_order), 0)").
		Where("collection_id = ?", collection.ID).
		Scan(&maxDisplayOrder).Error

	uploadedRows := make([]models.Emoji, 0, len(files))
	for idx, file := range files {
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read upload file"})
			return
		}
		buf, err := io.ReadAll(src)
		_ = src.Close()
		if err != nil || len(buf) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read upload file"})
			return
		}
		ext, err := validateUGCUploadFile(file.Filename, int64(len(buf)), runtime)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		objectName := fmt.Sprintf("%03d-%d%s", maxDisplayOrder+idx+1, time.Now().UnixNano(), ext)
		destKey := rawPrefix + objectName
		if err := uploadReaderToQiniu(uploader, h.qiniu, destKey, bytes.NewReader(buf), int64(len(buf))); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload file"})
			return
		}
		thumbKey := ""
		if ext == ".gif" {
			thumbKey = tryUploadListPreviewGIF(uploader, h.qiniu, destKey, buf)
		}

		title := trimToRuneLimit(strings.TrimSuffix(strings.TrimSpace(path.Base(file.Filename)), ext), 80)
		if title == "" {
			title = trimToRuneLimit(strings.TrimSuffix(objectName, ext), 80)
		}
		emojiStatus, auditReason := pickAuditStatus(runtime, title, file.Filename)
		emoji := models.Emoji{
			CollectionID: collection.ID,
			Title:        title,
			FileURL:      destKey,
			ThumbURL:     thumbKey,
			Format:       strings.TrimPrefix(ext, "."),
			SizeBytes:    int64(len(buf)),
			DisplayOrder: maxDisplayOrder + idx + 1,
			Status:       emojiStatus,
		}
		if err := h.db.Create(&emoji).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save emoji"})
			return
		}
		h.recordAuditLog(userID, "emoji", emoji.ID, "user_upload_auto_review", map[string]interface{}{
			"collection_id": collection.ID,
			"status":        emojiStatus,
			"reason":        auditReason,
			"source":        ugcCollectionSource,
		})
		uploadedRows = append(uploadedRows, emoji)
	}

	newCount := existingCount + int64(len(uploadedRows))
	var pendingCount int64
	_ = h.db.Model(&models.Emoji{}).
		Where("collection_id = ? AND status = ?", collection.ID, "pending").
		Count(&pendingCount).Error
	nextCollectionStatus := collection.Status
	if strings.ToLower(strings.TrimSpace(nextCollectionStatus)) != "disabled" {
		if pendingCount > 0 {
			nextCollectionStatus = "pending"
		} else if runtime.AutoAuditEnabled && runtime.AutoActivateOnPass {
			nextCollectionStatus = "active"
		} else {
			nextCollectionStatus = "pending"
		}
	}
	updateValues := map[string]interface{}{
		"file_count":   newCount,
		"qiniu_prefix": prefix,
		"status":       nextCollectionStatus,
	}
	if strings.TrimSpace(collection.CoverURL) == "" && len(uploadedRows) > 0 {
		updateValues["cover_url"] = uploadedRows[0].FileURL
		collection.CoverURL = uploadedRows[0].FileURL
	}
	_ = h.db.Model(&models.Collection{}).Where("id = ?", collection.ID).Updates(updateValues).Error
	if len(uploadedRows) > 0 {
		collection.FileCount = int(newCount)
		if err := h.touchUGCReviewAfterContentChange(collection, userID, "upload_emojis"); err != nil {
			if isUGCCollectionUnderReviewError(err) {
				c.JSON(http.StatusConflict, gin.H{"error": "collection is under review"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update review state"})
			return
		}
	}

	c.JSON(http.StatusOK, UploadMyCollectionEmojisResponse{
		CollectionID:   collection.ID,
		Added:          len(uploadedRows),
		FileCount:      newCount,
		MaxAllowed:     runtime.MaxFilesPerCollection,
		RemainingQuota: int64(runtime.MaxFilesPerCollection) - newCount,
		CoverURL:       resolvePreviewURL(collection.CoverURL, h.qiniu),
		Items:          enrichEmojiListItemsWithEngagement(h.db, mapEmojiItems(uploadedRows, h.qiniu, false), userID),
	})
}

// UpdateMyUploadEmoji updates metadata for an emoji in current user's upload collection.
// @Summary Update my uploaded emoji
// @Tags user
// @Accept json
// @Produce json
// @Param id path int true "emoji id"
// @Param body body UpdateMyUploadEmojiRequest true "update request"
// @Success 200 {object} EmojiListItem
// @Router /api/me/uploads/emojis/{id} [put]
func (h *Handler) UpdateMyUploadEmoji(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	emojiID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || emojiID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var emoji models.Emoji
	if err := h.db.First(&emoji, emojiID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load emoji"})
		return
	}
	collection, err := h.loadMyUGCCollectionByID(userID, emoji.CollectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load emoji"})
		return
	}
	if strings.EqualFold(strings.TrimSpace(collection.Status), "disabled") {
		c.JSON(http.StatusForbidden, gin.H{"error": "collection is disabled"})
		return
	}
	_ = collection

	var req UpdateMyUploadEmojiRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.Title != nil {
		title := trimToRuneLimit(*req.Title, 80)
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "title cannot be empty"})
			return
		}
		updates["title"] = title
	}
	if req.DisplayOrder != nil {
		if *req.DisplayOrder < 0 || *req.DisplayOrder > 999999 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid display_order"})
			return
		}
		updates["display_order"] = *req.DisplayOrder
	}
	if len(updates) > 0 {
		if _, err := h.ensureUGCCollectionMutable(collection); err != nil {
			if isUGCCollectionUnderReviewError(err) {
				c.JSON(http.StatusConflict, gin.H{"error": "collection is under review"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review state"})
			return
		}
		if err := h.db.Model(&models.Emoji{}).Where("id = ?", emoji.ID).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update emoji"})
			return
		}
		if err := h.touchUGCReviewAfterContentChange(collection, userID, "update_emoji_metadata"); err != nil {
			if isUGCCollectionUnderReviewError(err) {
				c.JSON(http.StatusConflict, gin.H{"error": "collection is under review"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update review state"})
			return
		}
	}

	var updated models.Emoji
	if err := h.db.First(&updated, emoji.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update emoji"})
		return
	}
	items := enrichEmojiListItemsWithEngagement(h.db, mapEmojiItems([]models.Emoji{updated}, h.qiniu, false), userID)
	if len(items) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
		return
	}
	c.JSON(http.StatusOK, items[0])
}

// DeleteMyUploadEmoji deletes one emoji in current user's upload collection.
// @Summary Delete my uploaded emoji
// @Tags user
// @Produce json
// @Param id path int true "emoji id"
// @Success 200 {object} map[string]interface{}
// @Router /api/me/uploads/emojis/{id} [delete]
func (h *Handler) DeleteMyUploadEmoji(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	emojiID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || emojiID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var emoji models.Emoji
	if err := h.db.First(&emoji, emojiID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load emoji"})
		return
	}
	collection, err := h.loadMyUGCCollectionByID(userID, emoji.CollectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load emoji"})
		return
	}
	if strings.EqualFold(strings.TrimSpace(collection.Status), "disabled") {
		c.JSON(http.StatusForbidden, gin.H{"error": "collection is disabled"})
		return
	}
	if _, err := h.ensureUGCCollectionMutable(collection); err != nil {
		if isUGCCollectionUnderReviewError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "collection is under review"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review state"})
		return
	}

	res, err := h.hardDeleteEmojiOutputWithDomain(
		emoji,
		collection,
		models.VideoJobAssetDomainArchive,
		nil,
		userID,
		"user",
		"ugc_delete_emoji",
		false,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.touchUGCReviewAfterContentChange(collection, userID, "delete_emoji")
	c.JSON(http.StatusOK, gin.H{
		"message": "deleted",
		"result":  res,
	})
}
