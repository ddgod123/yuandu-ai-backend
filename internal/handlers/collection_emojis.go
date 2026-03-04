package handlers

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

type EmojiUploadResponse struct {
	CollectionID uint64          `json:"collection_id"`
	Added        int             `json:"added"`
	FileCount    int64           `json:"file_count"`
	CoverURL     string          `json:"cover_url,omitempty"`
	Items        []EmojiListItem `json:"items"`
}

// UploadCollectionEmojis godoc
// @Summary Upload emojis to existing collection
// @Tags admin
// @Accept multipart/form-data
// @Produce json
// @Param id path int true "collection id"
// @Param files formData file true "emoji files" collectionFormat multi
// @Param set_cover formData bool false "set first file as cover"
// @Success 200 {object} EmojiUploadResponse
// @Router /api/admin/collections/{id}/emojis/upload [post]
func (h *Handler) UploadCollectionEmojis(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var collection models.Collection
	if err := h.db.First(&collection, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	setCover := strings.ToLower(strings.TrimSpace(c.PostForm("set_cover")))
	forceCover := setCover == "1" || setCover == "true" || setCover == "yes"

	prefix := strings.TrimSpace(collection.QiniuPrefix)
	if prefix == "" {
		prefix = "emoji/collections/"
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	rawPrefix := prefix + "raw/"
	uploader := qiniustorage.NewFormUploader(h.qiniu.Cfg)

	var existingCount int64
	_ = h.db.Model(&models.Emoji{}).Where("collection_id = ?", collection.ID).Count(&existingCount).Error
	maxSeq, seqWidth := maxEmojiSequence(h.db, collection.ID)
	if seqWidth <= 0 {
		seqWidth = 4
	}

	uploaded := make([]EmojiListItem, 0, len(files))
	var coverKey string

	for idx, file := range files {
		src, err := file.Open()
		if err != nil {
			continue
		}

		ext := strings.ToLower(path.Ext(file.Filename))
		if ext == "" {
			ext = ".gif"
		}
		seq := maxSeq + idx + 1
		destName := fmt.Sprintf("%0*d%s", seqWidth, seq, ext)
		destKey := rawPrefix + destName

		if err := uploadReaderToQiniu(uploader, h.qiniu, destKey, src, file.Size); err != nil {
			_ = src.Close()
			continue
		}
		_ = src.Close()

		if coverKey == "" {
			coverKey = destKey
		}

		title := strings.TrimSpace(strings.TrimSuffix(path.Base(file.Filename), ext))
		if title == "" {
			title = destName
		}
		emoji := models.Emoji{
			CollectionID: collection.ID,
			Title:        title,
			FileURL:      destKey,
			Format:       strings.TrimPrefix(ext, "."),
			SizeBytes:    file.Size,
			DisplayOrder: seq,
			Status:       "active",
		}
		if err := h.db.Create(&emoji).Error; err != nil {
			continue
		}
		uploaded = append(uploaded, EmojiListItem{
			ID:           emoji.ID,
			CollectionID: emoji.CollectionID,
			Title:        emoji.Title,
			FileURL:      emoji.FileURL,
			ThumbURL:     emoji.ThumbURL,
			Format:       emoji.Format,
			Width:        emoji.Width,
			Height:       emoji.Height,
			SizeBytes:    emoji.SizeBytes,
			Status:       emoji.Status,
			CreatedAt:    emoji.CreatedAt,
			UpdatedAt:    emoji.UpdatedAt,
		})
	}

	newCount := existingCount + int64(len(uploaded))
	if newCount < 0 {
		newCount = 0
	}
	collection.FileCount = int(newCount)
	if (forceCover || collection.CoverURL == "") && coverKey != "" {
		collection.CoverURL = coverKey
	}
	_ = h.db.Save(&collection).Error

	c.JSON(http.StatusOK, EmojiUploadResponse{
		CollectionID: collection.ID,
		Added:        len(uploaded),
		FileCount:    newCount,
		CoverURL:     collection.CoverURL,
		Items:        uploaded,
	})
}

func maxEmojiSequence(db *gorm.DB, collectionID uint64) (int, int) {
	type row struct {
		FileURL string
	}
	var rows []row
	if err := db.Model(&models.Emoji{}).
		Select("file_url").
		Where("collection_id = ?", collectionID).
		Find(&rows).Error; err != nil {
		return 0, 0
	}

	maxSeq := 0
	width := 0
	for _, item := range rows {
		base := path.Base(strings.TrimSpace(item.FileURL))
		if base == "" {
			continue
		}
		ext := path.Ext(base)
		name := strings.TrimSuffix(base, ext)
		if name == "" || !isDigits(name) {
			continue
		}
		seq, err := strconv.Atoi(name)
		if err != nil {
			continue
		}
		if seq > maxSeq {
			maxSeq = seq
		}
		if len(name) > width {
			width = len(name)
		}
	}
	return maxSeq, width
}

func isDigits(val string) bool {
	for _, r := range val {
		if r < '0' || r > '9' {
			return false
		}
	}
	return val != ""
}
