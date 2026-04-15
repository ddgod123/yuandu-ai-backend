package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/storage"

	"github.com/gin-gonic/gin"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

type EmojiListItem struct {
	ID            uint64    `json:"id"`
	CollectionID  uint64    `json:"collection_id"`
	Title         string    `json:"title"`
	FileURL       string    `json:"file_url,omitempty"`
	PreviewURL    string    `json:"preview_url,omitempty"`
	ThumbURL      string    `json:"thumb_url,omitempty"`
	Format        string    `json:"format"`
	LikeCount     int64     `json:"like_count"`
	FavoriteCount int64     `json:"favorite_count"`
	DownloadCount int64     `json:"download_count"`
	Liked         bool      `json:"liked"`
	Favorited     bool      `json:"favorited"`
	Width         int       `json:"width,omitempty"`
	Height        int       `json:"height,omitempty"`
	SizeBytes     int64     `json:"size_bytes"`
	Status        string    `json:"status,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type EmojiListResponse struct {
	Items []EmojiListItem `json:"items"`
	Total int64           `json:"total"`
}

const (
	// List pages use a lighter animated preview to reduce GIF payload while keeping motion.
	qiniuListGIFWidth     = 320
	qiniuListGIFFPS       = 12
	qiniuListGIFTransform = "avthumb/gif/s/%dx/r/%d"
	// Static list preview must be single-frame. Use PNG instead of WEBP to avoid animated-webp playback.
	qiniuListStaticTransform = "imageMogr2/thumbnail/!200x200r/gravity/Center/crop/200x200/format/png"
)

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

	respItems := mapEmojiItems(items, h.qiniu, adminView)
	userID := uint64(0)
	if uid, ok := currentUserIDFromContext(c); ok {
		userID = uid
	}
	respItems = enrichEmojiListItemsWithEngagement(h.db, respItems, userID)

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
	rootPrefix := strings.TrimSuffix(h.qiniuRootPrefix(), "/")
	for _, file := range files {
		src, err := file.Open()
		if err != nil {
			continue
		}
		buf, err := io.ReadAll(src)
		_ = src.Close()
		if err != nil || len(buf) == 0 {
			continue
		}

		objectName := path.Join(rootPrefix, file.Filename)
		if collectionID > 0 {
			objectName = path.Join(rootPrefix, fmt.Sprintf("%d", collectionID), file.Filename)
		}
		if err := uploadReaderToQiniu(uploader, h.qiniu, objectName, bytes.NewReader(buf), int64(len(buf))); err != nil {
			continue
		}
		thumbKey := ""
		ext := strings.ToLower(path.Ext(file.Filename))
		if ext == "" && strings.Contains(strings.ToLower(strings.TrimSpace(file.Header.Get("Content-Type"))), "gif") {
			ext = ".gif"
		}
		if ext == ".gif" {
			thumbKey = tryUploadListPreviewGIF(uploader, h.qiniu, objectName, buf)
		}

		fileURL := h.qiniu.PublicURL(objectName)
		if h.qiniu.Private {
			fileURL = objectName
		}
		thumbURL := thumbKey
		if thumbURL != "" && !h.qiniu.Private {
			thumbURL = h.qiniu.PublicURL(thumbKey)
		}
		format := strings.TrimPrefix(ext, ".")
		if format == "" {
			format = file.Header.Get("Content-Type")
		}

		emoji := models.Emoji{
			Title:        file.Filename,
			CollectionID: collectionID,
			FileURL:      fileURL,
			ThumbURL:     thumbURL,
			Format:       format,
			SizeBytes:    int64(len(buf)),
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

func mapEmojiItems(items []models.Emoji, qiniuClient *storage.QiniuClient, adminView bool) []EmojiListItem {
	out := make([]EmojiListItem, 0, len(items))
	for _, item := range items {
		previewSource := strings.TrimSpace(item.FileURL)
		if thumb := strings.TrimSpace(item.ThumbURL); thumb != "" {
			previewSource = thumb
		}
		previewURL := resolvePreviewURL(previewSource, qiniuClient)
		fileURL := ""
		thumbURL := ""
		status := ""
		if adminView {
			fileURL = item.FileURL
			thumbURL = item.ThumbURL
			status = item.Status
		}
		out = append(out, EmojiListItem{
			ID:           item.ID,
			CollectionID: item.CollectionID,
			Title:        item.Title,
			FileURL:      fileURL,
			PreviewURL:   previewURL,
			ThumbURL:     thumbURL,
			Format:       item.Format,
			Width:        item.Width,
			Height:       item.Height,
			SizeBytes:    item.SizeBytes,
			Status:       status,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
		})
	}
	return out
}

type emojiStats struct {
	LikeCount     int64
	FavoriteCount int64
	DownloadCount int64
}

func uniqueUint64(values []uint64) []uint64 {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[uint64]struct{}, len(values))
	out := make([]uint64, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func loadEmojiStats(db *gorm.DB, emojiIDs []uint64) map[uint64]emojiStats {
	stats := map[uint64]emojiStats{}
	ids := uniqueUint64(emojiIDs)
	if db == nil || len(ids) == 0 {
		return stats
	}

	type countRow struct {
		EmojiID uint64
		Count   int64
	}

	var likeRows []countRow
	db.Table("action.likes").
		Select("emoji_id, COUNT(*) AS count").
		Where("emoji_id IN ?", ids).
		Group("emoji_id").
		Scan(&likeRows)
	for _, row := range likeRows {
		next := stats[row.EmojiID]
		next.LikeCount = row.Count
		stats[row.EmojiID] = next
	}

	var favoriteRows []countRow
	db.Table("action.favorites").
		Select("emoji_id, COUNT(*) AS count").
		Where("emoji_id IN ?", ids).
		Group("emoji_id").
		Scan(&favoriteRows)
	for _, row := range favoriteRows {
		next := stats[row.EmojiID]
		next.FavoriteCount = row.Count
		stats[row.EmojiID] = next
	}

	var downloadRows []countRow
	db.Table("action.downloads").
		Select("emoji_id, COUNT(*) AS count").
		Where("emoji_id IN ?", ids).
		Group("emoji_id").
		Scan(&downloadRows)
	for _, row := range downloadRows {
		next := stats[row.EmojiID]
		next.DownloadCount = row.Count
		stats[row.EmojiID] = next
	}

	return stats
}

func loadEmojiActionState(db *gorm.DB, emojiIDs []uint64, userID uint64) (map[uint64]bool, map[uint64]bool) {
	likedMap := map[uint64]bool{}
	favoritedMap := map[uint64]bool{}
	ids := uniqueUint64(emojiIDs)
	if db == nil || userID == 0 || len(ids) == 0 {
		return likedMap, favoritedMap
	}

	type actionRow struct {
		EmojiID uint64
	}

	var likeRows []actionRow
	db.Table("action.likes").
		Select("emoji_id").
		Where("user_id = ? AND emoji_id IN ?", userID, ids).
		Scan(&likeRows)
	for _, row := range likeRows {
		likedMap[row.EmojiID] = true
	}

	var favoriteRows []actionRow
	db.Table("action.favorites").
		Select("emoji_id").
		Where("user_id = ? AND emoji_id IN ?", userID, ids).
		Scan(&favoriteRows)
	for _, row := range favoriteRows {
		favoritedMap[row.EmojiID] = true
	}

	return likedMap, favoritedMap
}

func enrichEmojiListItemsWithEngagement(db *gorm.DB, items []EmojiListItem, userID uint64) []EmojiListItem {
	if len(items) == 0 {
		return items
	}
	ids := make([]uint64, 0, len(items))
	for _, item := range items {
		if item.ID == 0 {
			continue
		}
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return items
	}

	statsMap := loadEmojiStats(db, ids)
	likedMap, favoritedMap := loadEmojiActionState(db, ids, userID)

	for i := range items {
		stats := statsMap[items[i].ID]
		items[i].LikeCount = stats.LikeCount
		items[i].FavoriteCount = stats.FavoriteCount
		items[i].DownloadCount = stats.DownloadCount
		items[i].Liked = likedMap[items[i].ID]
		items[i].Favorited = favoritedMap[items[i].ID]
	}
	return items
}

func resolvePreviewURL(fileURL string, qiniuClient *storage.QiniuClient) string {
	fileURL = strings.TrimSpace(fileURL)
	if fileURL == "" {
		return ""
	}
	if qiniuClient == nil {
		return fileURL
	}
	key, ok := extractQiniuObjectKey(fileURL, qiniuClient)
	if ok {
		if qiniuClient.Private {
			if signed, err := qiniuClient.SignedURL(key, 0); err == nil && signed != "" {
				return signed
			}
		}
		return qiniuClient.PublicURL(key)
	}
	if strings.HasPrefix(fileURL, "http://") || strings.HasPrefix(fileURL, "https://") {
		return fileURL
	}
	if qiniuClient.Private {
		if signed, err := qiniuClient.SignedURL(fileURL, 0); err == nil && signed != "" {
			return signed
		}
	}
	return qiniuClient.PublicURL(fileURL)
}

func resolveListPreviewURL(fileURL string, qiniuClient *storage.QiniuClient) string {
	fileURL = strings.TrimSpace(fileURL)
	if fileURL == "" {
		return ""
	}
	if qiniuClient == nil {
		return fileURL
	}

	key, ok := extractQiniuObjectKey(fileURL, qiniuClient)
	if !ok {
		if strings.HasPrefix(fileURL, "http://") || strings.HasPrefix(fileURL, "https://") {
			return fileURL
		}
		return resolvePreviewURL(fileURL, qiniuClient)
	}

	if !isGIFObjectKey(key) {
		return resolvePreviewURL(key, qiniuClient)
	}
	// Pre-generated list GIF keys should be served directly.
	if isListPreviewGIFKey(key) {
		return resolvePreviewURL(key, qiniuClient)
	}
	// For private buckets, transformed query-sign URLs can be rejected by CDN.
	// Keep list previews as normal signed object URLs to ensure reliability.
	if qiniuClient.Private {
		return resolvePreviewURL(key, qiniuClient)
	}

	listKey, listQuery := buildListGIFPreviewSpec(key)
	if listKey == "" || listQuery == "" {
		return resolvePreviewURL(key, qiniuClient)
	}
	return qiniuClient.PublicURLWithQuery(listKey, listQuery)
}

func resolveListStaticPreviewURL(fileURL string, qiniuClient *storage.QiniuClient) string {
	fileURL = strings.TrimSpace(fileURL)
	if fileURL == "" {
		return ""
	}
	if qiniuClient == nil {
		return fileURL
	}

	key, ok := extractQiniuObjectKey(fileURL, qiniuClient)
	if !ok {
		if strings.HasPrefix(fileURL, "http://") || strings.HasPrefix(fileURL, "https://") {
			return fileURL
		}
		return resolvePreviewURL(fileURL, qiniuClient)
	}

	if !isGIFObjectKey(key) {
		return resolvePreviewURL(key, qiniuClient)
	}
	if qiniuClient.Private {
		if signed, err := qiniuClient.SignedURLWithQuery(key, qiniuListStaticTransform, 0); err == nil && signed != "" {
			return signed
		}
		return resolvePreviewURL(key, qiniuClient)
	}
	return qiniuClient.PublicURLWithQuery(key, qiniuListStaticTransform)
}

func isGIFObjectKey(raw string) bool {
	clean := strings.SplitN(raw, "?", 2)[0]
	clean = strings.SplitN(clean, "#", 2)[0]
	return strings.HasSuffix(strings.ToLower(clean), ".gif")
}

func buildListGIFPreviewSpec(key string) (string, string) {
	baseKey := strings.TrimSpace(strings.SplitN(key, "?", 2)[0])
	baseKey = strings.TrimLeft(baseKey, "/")
	if baseKey == "" {
		return "", ""
	}
	transform := fmt.Sprintf(qiniuListGIFTransform, qiniuListGIFWidth, qiniuListGIFFPS)
	return baseKey, transform
}

func isListPreviewGIFKey(key string) bool {
	clean := strings.TrimSpace(strings.SplitN(key, "?", 2)[0])
	clean = strings.TrimLeft(clean, "/")
	clean = strings.ToLower(clean)
	return strings.Contains(clean, "/list/") || strings.HasSuffix(clean, "_list.gif")
}

func extractQiniuObjectKey(raw string, qiniuClient *storage.QiniuClient) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		key := strings.TrimLeft(raw, "/")
		return key, key != ""
	}

	parsedURL, err := url.Parse(raw)
	if err != nil || parsedURL.Host == "" {
		return "", false
	}
	domainHost, domainPath, ok := qiniuDomainInfo(qiniuClient)
	if !ok || !strings.EqualFold(parsedURL.Hostname(), domainHost) {
		// Legacy compatibility: if an old absolute URL still embeds a storage key
		// under storage root prefix, treat it as a Qiniu object key so responses can be rewritten
		// to the current configured domain.
		fallback := strings.TrimLeft(parsedURL.EscapedPath(), "/")
		if decoded, err := url.PathUnescape(fallback); err == nil {
			fallback = decoded
		}
		fallback = strings.TrimSpace(fallback)
		if storage.HasRootPrefix(fallback, configuredQiniuRootPrefix(qiniuClient)) || storage.HasRootPrefix(fallback, qiniuLegacyRootPrefix) {
			return fallback, true
		}
		return "", false
	}

	pathKey := strings.TrimLeft(parsedURL.EscapedPath(), "/")
	if domainPath != "" {
		if pathKey == domainPath {
			pathKey = ""
		} else if strings.HasPrefix(pathKey, domainPath+"/") {
			pathKey = strings.TrimPrefix(pathKey, domainPath+"/")
		} else {
			return "", false
		}
	}
	if pathKey == "" {
		return "", false
	}
	if decoded, err := url.PathUnescape(pathKey); err == nil {
		pathKey = decoded
	}
	return pathKey, true
}

func configuredQiniuRootPrefix(qiniuClient *storage.QiniuClient) string {
	if qiniuClient == nil {
		return ""
	}
	return qiniuClient.RootPrefix
}

func qiniuDomainInfo(qiniuClient *storage.QiniuClient) (host string, pathPrefix string, ok bool) {
	if qiniuClient == nil {
		return "", "", false
	}
	domain := strings.TrimSpace(qiniuClient.Domain)
	if domain == "" {
		return "", "", false
	}
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		if qiniuClient.UseHTTPS {
			domain = "https://" + domain
		} else {
			domain = "http://" + domain
		}
	}
	parsedDomain, err := url.Parse(domain)
	if err != nil || parsedDomain.Host == "" {
		return "", "", false
	}
	return strings.ToLower(parsedDomain.Hostname()), strings.Trim(parsedDomain.EscapedPath(), "/"), true
}

func (h *Handler) DeleteEmoji(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&models.Emoji{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete emoji"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
