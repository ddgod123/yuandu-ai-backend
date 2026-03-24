package handlers

import (
	"emoji/internal/models"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type FavoriteEmojiListItem struct {
	EmojiID   uint64        `json:"emoji_id"`
	CreatedAt time.Time     `json:"created_at"`
	Emoji     EmojiListItem `json:"emoji"`
}

type FavoriteEmojiListResponse struct {
	Items    []FavoriteEmojiListItem `json:"items"`
	Total    int64                   `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}

type FavoriteCollectionListItem struct {
	CollectionID uint64             `json:"collection_id"`
	CreatedAt    time.Time          `json:"created_at"`
	Collection   CollectionListItem `json:"collection"`
}

type FavoriteCollectionListResponse struct {
	Items    []FavoriteCollectionListItem `json:"items"`
	Total    int64                        `json:"total"`
	Page     int                          `json:"page"`
	PageSize int                          `json:"page_size"`
}

// AddFavorite 添加收藏
// @Summary 添加收藏
// @Tags favorites
// @Accept json
// @Produce json
// @Param body body object{emoji_id=uint64} true "Emoji ID"
// @Success 200 {object} models.Favorite
// @Router /api/favorites [post]
func (h *Handler) AddFavorite(c *gin.Context) {
	if _, ok := h.requireActiveUser(c); !ok {
		return
	}
	userID := c.GetUint64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		EmojiID uint64 `json:"emoji_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	// 检查表情是否存在
	var emoji models.Emoji
	if err := h.db.First(&emoji, req.EmojiID).Error; err != nil {
		c.JSON(404, gin.H{"error": "emoji not found"})
		return
	}

	// 创建收藏记录（如果已存在会被忽略）
	favorite := models.Favorite{
		UserID:    userID,
		EmojiID:   req.EmojiID,
		CreatedAt: time.Now(),
	}

	result := h.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&favorite)
	if result.Error != nil {
		c.JSON(500, gin.H{"error": "failed to add favorite"})
		return
	}
	if result.RowsAffected > 0 {
		h.bumpVideoJobFeedbackByCollectionID(emoji.CollectionID, "favorite", userID)
		h.recordVideoImageFeedbackByEmojiID(req.EmojiID, videoImageFeedbackActionFavorite, userID, nil)
	}

	c.JSON(200, favorite)
}

// RemoveFavorite 取消收藏
// @Summary 取消收藏
// @Tags favorites
// @Produce json
// @Param emoji_id path string true "Emoji ID"
// @Success 200 {object} object{message=string}
// @Router /api/favorites/{emoji_id} [delete]
func (h *Handler) RemoveFavorite(c *gin.Context) {
	if _, ok := h.requireActiveUser(c); !ok {
		return
	}
	userID := c.GetUint64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	emojiIDStr := c.Param("emoji_id")

	emojiID, err := strconv.ParseUint(emojiIDStr, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid emoji_id"})
		return
	}

	result := h.db.Where("user_id = ? AND emoji_id = ?", userID, emojiID).
		Delete(&models.Favorite{})

	if result.Error != nil {
		c.JSON(500, gin.H{"error": "failed to remove favorite"})
		return
	}

	c.JSON(200, gin.H{"message": "removed"})
}

// ListFavorites 获取收藏列表
// @Summary 获取收藏列表
// @Tags favorites
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Success 200 {object} FavoriteEmojiListResponse
// @Router /api/favorites [get]
func (h *Handler) ListFavorites(c *gin.Context) {
	if _, ok := h.requireActiveUser(c); !ok {
		return
	}
	userID := c.GetUint64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	page := parseIntQuery(c, "page", 1)
	pageSize := parseIntQuery(c, "page_size", 20)
	if pageSize > 100 {
		pageSize = 100
	}

	type favoriteEmojiRow struct {
		EmojiID   uint64
		CreatedAt time.Time
	}
	var total int64

	query := h.db.Table("action.favorites AS f").
		Joins("JOIN archive.emojis e ON e.id = f.emoji_id AND e.deleted_at IS NULL").
		Joins("JOIN archive.collections c ON c.id = e.collection_id AND c.deleted_at IS NULL").
		Where("f.user_id = ?", userID)
	if !isAdminRole(c) {
		query = query.Where("e.status = ? AND c.status = ? AND c.visibility = ?", "active", "active", "public")
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	var favoriteRows []favoriteEmojiRow
	if err := query.Select("f.emoji_id, f.created_at").
		Order("f.created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Scan(&favoriteRows).Error; err != nil {
		c.JSON(500, gin.H{"error": "failed to list favorites"})
		return
	}

	if len(favoriteRows) == 0 {
		c.JSON(200, FavoriteEmojiListResponse{
			Items:    []FavoriteEmojiListItem{},
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		})
		return
	}

	emojiIDs := make([]uint64, 0, len(favoriteRows))
	for _, row := range favoriteRows {
		emojiIDs = append(emojiIDs, row.EmojiID)
	}

	var emojis []models.Emoji
	if err := h.db.Where("id IN ?", emojiIDs).Find(&emojis).Error; err != nil {
		c.JSON(500, gin.H{"error": "failed to load favorite emojis"})
		return
	}

	emojiMap := make(map[uint64]EmojiListItem, len(emojis))
	for _, item := range mapEmojiItems(emojis, h.qiniu, false) {
		item.Favorited = true
		emojiMap[item.ID] = item
	}

	respItems := make([]FavoriteEmojiListItem, 0, len(favoriteRows))
	for _, row := range favoriteRows {
		emoji, ok := emojiMap[row.EmojiID]
		if !ok {
			continue
		}
		respItems = append(respItems, FavoriteEmojiListItem{
			EmojiID:   row.EmojiID,
			CreatedAt: row.CreatedAt,
			Emoji:     emoji,
		})
	}

	c.JSON(200, FavoriteEmojiListResponse{
		Items:    respItems,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// ListCollectionFavorites 获取收藏合集列表
// @Summary 获取收藏合集列表
// @Tags favorites
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Success 200 {object} FavoriteCollectionListResponse
// @Router /api/favorites/collections [get]
func (h *Handler) ListCollectionFavorites(c *gin.Context) {
	if _, ok := h.requireActiveUser(c); !ok {
		return
	}
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	page := parseIntQuery(c, "page", 1)
	pageSize := parseIntQuery(c, "page_size", 20)
	if pageSize > 100 {
		pageSize = 100
	}

	type favoriteCollectionRow struct {
		CollectionID uint64
		CreatedAt    time.Time
	}

	query := h.db.Table("action.collection_favorites AS cf").
		Joins("JOIN archive.collections c ON c.id = cf.collection_id AND c.deleted_at IS NULL").
		Where("cf.user_id = ?", userID)
	if !isAdminRole(c) {
		query = query.Where("c.status = ? AND c.visibility = ?", "active", "public")
	}

	var total int64
	query.Count(&total)

	offset := (page - 1) * pageSize
	var rows []favoriteCollectionRow
	if err := query.Select("cf.collection_id, cf.created_at").
		Order("cf.created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list favorite collections"})
		return
	}

	if len(rows) == 0 {
		c.JSON(http.StatusOK, FavoriteCollectionListResponse{
			Items:    []FavoriteCollectionListItem{},
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		})
		return
	}

	collectionIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		collectionIDs = append(collectionIDs, row.CollectionID)
	}

	var collections []models.Collection
	if err := h.db.Where("id IN ?", collectionIDs).Find(&collections).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collections"})
		return
	}
	if len(collections) == 0 {
		c.JSON(http.StatusOK, FavoriteCollectionListResponse{
			Items:    []FavoriteCollectionListItem{},
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		})
		return
	}

	tagMap := loadCollectionTags(h.db, collections)
	creatorMap := loadCreatorProfiles(h.db, collections)
	statMap := loadCollectionStats(h.db, collections)
	likedMap, _ := loadCollectionActionState(h.db, collectionIDs, userID)

	collectionMap := make(map[uint64]CollectionListItem, len(collections))
	for _, item := range collections {
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
		collectionMap[item.ID] = CollectionListItem{
			ID:               item.ID,
			Title:            item.Title,
			Slug:             item.Slug,
			Description:      item.Description,
			CoverURL:         resolvePreviewURL(item.CoverURL, h.qiniu),
			OwnerID:          item.OwnerID,
			CreatorProfileID: item.CreatorProfileID,
			CreatorName:      creatorName,
			CreatorNameZh:    creatorNameZh,
			CreatorNameEn:    creatorNameEn,
			CreatorAvatarURL: creatorAvatar,
			FavoriteCount:    stats.FavoriteCount,
			LikeCount:        stats.LikeCount,
			DownloadCount:    stats.DownloadCount,
			Favorited:        true,
			Liked:            likedMap[item.ID],
			CategoryID:       item.CategoryID,
			IPID:             item.IPID,
			ThemeID:          item.ThemeID,
			Source:           item.Source,
			QiniuPrefix:      item.QiniuPrefix,
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
		}
	}

	respItems := make([]FavoriteCollectionListItem, 0, len(rows))
	for _, row := range rows {
		collection, ok := collectionMap[row.CollectionID]
		if !ok {
			continue
		}
		respItems = append(respItems, FavoriteCollectionListItem{
			CollectionID: row.CollectionID,
			CreatedAt:    row.CreatedAt,
			Collection:   collection,
		})
	}

	c.JSON(http.StatusOK, FavoriteCollectionListResponse{
		Items:    respItems,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}
