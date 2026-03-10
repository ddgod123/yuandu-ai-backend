package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CollectionActionSummary struct {
	CollectionID  uint64 `json:"collection_id"`
	LikeCount     int64  `json:"like_count"`
	FavoriteCount int64  `json:"favorite_count"`
	DownloadCount int64  `json:"download_count"`
	Liked         bool   `json:"liked"`
	Favorited     bool   `json:"favorited"`
}

func (h *Handler) AddCollectionLike(c *gin.Context) {
	h.mutateCollectionLike(c, true)
}

func (h *Handler) RemoveCollectionLike(c *gin.Context) {
	h.mutateCollectionLike(c, false)
}

func (h *Handler) AddCollectionFavorite(c *gin.Context) {
	h.mutateCollectionFavorite(c, true)
}

func (h *Handler) RemoveCollectionFavorite(c *gin.Context) {
	h.mutateCollectionFavorite(c, false)
}

func (h *Handler) mutateCollectionLike(c *gin.Context, add bool) {
	if _, ok := h.requireActiveUser(c); !ok {
		return
	}
	userID, ok := currentUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	collection, ok := h.loadVisibleCollectionByID(c)
	if !ok {
		return
	}

	if add {
		like := models.CollectionLike{
			UserID:       userID,
			CollectionID: collection.ID,
		}
		if err := h.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&like).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to like collection"})
			return
		}
	} else {
		if err := h.db.Where("user_id = ? AND collection_id = ?", userID, collection.ID).
			Delete(&models.CollectionLike{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel like"})
			return
		}
	}

	h.respondCollectionActionSummary(c, collection.ID, userID)
}

func (h *Handler) mutateCollectionFavorite(c *gin.Context, add bool) {
	if _, ok := h.requireActiveUser(c); !ok {
		return
	}
	userID, ok := currentUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	collection, ok := h.loadVisibleCollectionByID(c)
	if !ok {
		return
	}

	if add {
		favorite := models.CollectionFavorite{
			UserID:       userID,
			CollectionID: collection.ID,
		}
		if err := h.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&favorite).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to favorite collection"})
			return
		}
	} else {
		if err := h.db.Where("user_id = ? AND collection_id = ?", userID, collection.ID).
			Delete(&models.CollectionFavorite{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel favorite"})
			return
		}
	}

	h.respondCollectionActionSummary(c, collection.ID, userID)
}

func (h *Handler) loadVisibleCollectionByID(c *gin.Context) (models.Collection, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return models.Collection{}, false
	}

	var collection models.Collection
	if err := h.db.First(&collection, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return models.Collection{}, false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return models.Collection{}, false
	}
	if !ensureCollectionVisibleForRequester(c, collection) {
		return models.Collection{}, false
	}
	return collection, true
}

func (h *Handler) respondCollectionActionSummary(c *gin.Context, collectionID, userID uint64) {
	var likeCount int64
	var favoriteCount int64
	var downloadCount int64
	h.db.Table("action.collection_likes").Where("collection_id = ?", collectionID).Count(&likeCount)
	h.db.Table("action.collection_favorites").Where("collection_id = ?", collectionID).Count(&favoriteCount)
	h.db.Table("action.collection_downloads").Where("collection_id = ?", collectionID).Count(&downloadCount)

	var likedCount int64
	var favoritedCount int64
	h.db.Table("action.collection_likes").
		Where("collection_id = ? AND user_id = ?", collectionID, userID).
		Count(&likedCount)
	h.db.Table("action.collection_favorites").
		Where("collection_id = ? AND user_id = ?", collectionID, userID).
		Count(&favoritedCount)

	c.JSON(http.StatusOK, CollectionActionSummary{
		CollectionID:  collectionID,
		LikeCount:     likeCount,
		FavoriteCount: favoriteCount,
		DownloadCount: downloadCount,
		Liked:         likedCount > 0,
		Favorited:     favoritedCount > 0,
	})
}
