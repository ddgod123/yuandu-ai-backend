package handlers

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/storage"

	"github.com/gin-gonic/gin"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
)

type EmojiListItem struct {
	ID           uint64    `json:"id"`
	CollectionID uint64    `json:"collection_id"`
	Title        string    `json:"title"`
	FileURL      string    `json:"file_url"`
	PreviewURL   string    `json:"preview_url,omitempty"`
	ThumbURL     string    `json:"thumb_url,omitempty"`
	Format       string    `json:"format"`
	Favorited    bool      `json:"favorited"`
	Width        int       `json:"width,omitempty"`
	Height       int       `json:"height,omitempty"`
	SizeBytes    int64     `json:"size_bytes"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type EmojiListResponse struct {
	Items []EmojiListItem `json:"items"`
	Total int64           `json:"total"`
}

func (h *Handler) ListEmojis(c *gin.Context) {
	var (
		q        = c.Query("q")
		tag      = c.Query("tag")
		colIDStr = c.Query("collection_id")
		page, _  = strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ = strconv.Atoi(c.DefaultQuery("page_size", "30"))
	)

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 30
	}

	adminView := isAdminRole(c)
	db := h.db.Model(&models.Emoji{})
	if !adminView {
		db = db.Where("status = ?", "active")
	}

	if q != "" {
		db = db.Where("title ILIKE ?", "%"+q+"%")
	}

	if colIDStr != "" {
		if colID, err := strconv.ParseUint(colIDStr, 10, 64); err == nil {
			db = db.Where("collection_id = ?", colID)
			if !adminView {
				var collection models.Collection
				if err := h.db.Select("id", "status", "visibility").First(&collection, colID).Error; err != nil || !isPublicCollectionVisible(collection) {
					c.JSON(http.StatusOK, EmojiListResponse{
						Items: []EmojiListItem{},
						Total: 0,
					})
					return
				}
			}
		}
	} else if !adminView {
		publicCollectionIDs := h.db.Model(&models.Collection{}).
			Select("id").
			Where("status = ? AND visibility = ?", "active", "public")
		db = db.Where("collection_id IN (?)", publicCollectionIDs)
	}

	if tag != "" {
		db = db.Joins("JOIN emoji_tags ON emoji_tags.emoji_id = emojis.id").
			Joins("JOIN tags ON tags.id = emoji_tags.tag_id").
			Where("tags.name = ?", tag)
	}

	var total int64
	db.Count(&total)

	var items []models.Emoji
	db.Offset((page - 1) * limit).Limit(limit).Order("id DESC").Find(&items)

	respItems := mapEmojiItems(items, h.qiniu)
	if userID, ok := currentUserIDFromContext(c); ok && len(respItems) > 0 {
		emojiIDs := make([]uint64, 0, len(respItems))
		for _, item := range respItems {
			emojiIDs = append(emojiIDs, item.ID)
		}
		type favoriteRow struct {
			EmojiID uint64
		}
		var favoriteRows []favoriteRow
		h.db.Table("action.favorites").
			Select("emoji_id").
			Where("user_id = ? AND emoji_id IN ?", userID, emojiIDs).
			Scan(&favoriteRows)
		favoriteMap := make(map[uint64]bool, len(favoriteRows))
		for _, row := range favoriteRows {
			favoriteMap[row.EmojiID] = true
		}
		for i := range respItems {
			respItems[i].Favorited = favoriteMap[respItems[i].ID]
		}
	}

	c.JSON(http.StatusOK, EmojiListResponse{
		Items: respItems,
		Total: total,
	})
}

func (h *Handler) BatchUploadEmoji(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse form"})
		return
	}

	files := form.File["files"]
	collectionIDStr := c.PostForm("collection_id")
	var collectionID uint64
	if collectionIDStr != "" {
		collectionID, _ = strconv.ParseUint(collectionIDStr, 10, 64)
	}

	var uploaded []models.Emoji
	uploader := qiniustorage.NewFormUploader(h.qiniu.Cfg)
	for _, file := range files {
		src, err := file.Open()
		if err != nil {
			continue
		}

		objectName := path.Join("emoji", file.Filename)
		if collectionID > 0 {
			objectName = path.Join("emoji", fmt.Sprintf("%d", collectionID), file.Filename)
		}
		putPolicy := qiniustorage.PutPolicy{
			Scope:   fmt.Sprintf("%s:%s", h.qiniu.Bucket, objectName),
			Expires: 3600,
		}
		upToken := putPolicy.UploadToken(h.qiniu.Mac)
		var putRet qiniustorage.PutRet
		err = uploader.Put(c.Request.Context(), &putRet, upToken, objectName, src, file.Size, &qiniustorage.PutExtra{
			MimeType: file.Header.Get("Content-Type"),
		})
		src.Close()

		if err != nil {
			continue
		}

		fileURL := h.qiniu.PublicURL(objectName)
		if h.qiniu.Private {
			fileURL = objectName
		}

		emoji := models.Emoji{
			Title:        file.Filename,
			CollectionID: collectionID,
			FileURL:      fileURL,
			Format:       file.Header.Get("Content-Type"),
			SizeBytes:    file.Size,
			Status:       "active",
		}

		if err := h.db.Create(&emoji).Error; err == nil {
			uploaded = append(uploaded, emoji)
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"uploaded": uploaded,
		"total":    len(uploaded),
	})
}

func (h *Handler) UpdateEmoji(c *gin.Context) {
	id := c.Param("id")
	var emoji models.Emoji
	if err := h.db.First(&emoji, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
		return
	}

	var input struct {
		Title        string `json:"title"`
		CollectionID uint64 `json:"collection_id"`
		Status       string `json:"status"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	emoji.Title = input.Title
	emoji.CollectionID = input.CollectionID
	emoji.Status = input.Status

	if err := h.db.Save(&emoji).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update emoji"})
		return
	}

	c.JSON(http.StatusOK, emoji)
}

func mapEmojiItems(items []models.Emoji, qiniuClient *storage.QiniuClient) []EmojiListItem {
	out := make([]EmojiListItem, 0, len(items))
	for _, item := range items {
		previewURL := resolvePreviewURL(item.FileURL, qiniuClient)
		out = append(out, EmojiListItem{
			ID:           item.ID,
			CollectionID: item.CollectionID,
			Title:        item.Title,
			FileURL:      item.FileURL,
			PreviewURL:   previewURL,
			ThumbURL:     item.ThumbURL,
			Format:       item.Format,
			Width:        item.Width,
			Height:       item.Height,
			SizeBytes:    item.SizeBytes,
			Status:       item.Status,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
		})
	}
	return out
}

func resolvePreviewURL(fileURL string, qiniuClient *storage.QiniuClient) string {
	fileURL = strings.TrimSpace(fileURL)
	if fileURL == "" {
		return ""
	}
	if strings.HasPrefix(fileURL, "http://") || strings.HasPrefix(fileURL, "https://") {
		return fileURL
	}
	if qiniuClient == nil {
		return fileURL
	}
	if qiniuClient.Private {
		if signed, err := qiniuClient.SignedURL(fileURL, 0); err == nil && signed != "" {
			return signed
		}
	}
	return qiniuClient.PublicURL(fileURL)
}

func (h *Handler) DeleteEmoji(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&models.Emoji{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete emoji"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
