package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EmojiActionSummary struct {
	EmojiID       uint64 `json:"emoji_id"`
	LikeCount     int64  `json:"like_count"`
	FavoriteCount int64  `json:"favorite_count"`
	DownloadCount int64  `json:"download_count"`
	Liked         bool   `json:"liked"`
	Favorited     bool   `json:"favorited"`
}

func (h *Handler) AddEmojiLike(c *gin.Context) {
	h.mutateEmojiLike(c, true)
}

func (h *Handler) RemoveEmojiLike(c *gin.Context) {
	h.mutateEmojiLike(c, false)
}

func (h *Handler) mutateEmojiLike(c *gin.Context, add bool) {
	if _, ok := h.requireActiveUser(c); !ok {
		return
	}
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	emoji, _, ok := h.loadVisibleEmojiByID(c)
	if !ok {
		return
	}

	if add {
		row := models.Like{
			UserID:  userID,
			EmojiID: emoji.ID,
		}
		result := h.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to like emoji"})
			return
		}
		if result.RowsAffected > 0 {
			h.bumpVideoJobFeedbackByEmojiID(emoji.ID, "like", userID)
			h.recordVideoImageFeedbackByEmojiID(emoji.ID, videoImageFeedbackActionLike, userID, nil)
		}
	} else {
		if err := h.db.Where("user_id = ? AND emoji_id = ?", userID, emoji.ID).
			Delete(&models.Like{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel like"})
			return
		}
	}

	h.respondEmojiActionSummary(c, emoji.ID, userID)
}

func (h *Handler) loadVisibleEmojiByID(c *gin.Context) (models.Emoji, models.Collection, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return models.Emoji{}, models.Collection{}, false
	}

	var emoji models.Emoji
	if err := h.db.First(&emoji, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
			return models.Emoji{}, models.Collection{}, false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return models.Emoji{}, models.Collection{}, false
	}
	if !isAdminRole(c) && !strings.EqualFold(strings.TrimSpace(emoji.Status), "active") {
		c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
		return models.Emoji{}, models.Collection{}, false
	}

	var collection models.Collection
	if err := h.db.Select("id", "status", "visibility", "owner_id").First(&collection, emoji.CollectionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return models.Emoji{}, models.Collection{}, false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return models.Emoji{}, models.Collection{}, false
	}
	if isAdminRole(c) {
		return emoji, collection, true
	}
	if isPublicCollectionVisible(collection) {
		return emoji, collection, true
	}
	if userID, ok := currentUserIDFromContext(c); ok && userID == collection.OwnerID {
		return emoji, collection, true
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
	return models.Emoji{}, models.Collection{}, false

}

func (h *Handler) respondEmojiActionSummary(c *gin.Context, emojiID, userID uint64) {
	var likeCount int64
	var favoriteCount int64
	var downloadCount int64
	h.db.Table("action.likes").Where("emoji_id = ?", emojiID).Count(&likeCount)
	h.db.Table("action.favorites").Where("emoji_id = ?", emojiID).Count(&favoriteCount)
	h.db.Table("action.downloads").Where("emoji_id = ?", emojiID).Count(&downloadCount)

	var likedCount int64
	var favoritedCount int64
	h.db.Table("action.likes").
		Where("emoji_id = ? AND user_id = ?", emojiID, userID).
		Count(&likedCount)
	h.db.Table("action.favorites").
		Where("emoji_id = ? AND user_id = ?", emojiID, userID).
		Count(&favoritedCount)

	c.JSON(http.StatusOK, EmojiActionSummary{
		EmojiID:       emojiID,
		LikeCount:     likeCount,
		FavoriteCount: favoriteCount,
		DownloadCount: downloadCount,
		Liked:         likedCount > 0,
		Favorited:     favoritedCount > 0,
	})
}
