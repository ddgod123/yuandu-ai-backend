package handlers

import (
	"archive/zip"
	"errors"
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
	"gorm.io/gorm"
)

type DownloadURLResponse struct {
	CollectionID uint64 `json:"collection_id,omitempty"`
	EmojiID      uint64 `json:"emoji_id,omitempty"`
	Key          string `json:"key,omitempty"`
	Name         string `json:"name,omitempty"`
	URL          string `json:"url"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
}

type CollectionZipItem struct {
	ID         uint64     `json:"id"`
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	SizeBytes  int64      `json:"size_bytes"`
	UploadedAt *time.Time `json:"uploaded_at,omitempty"`
}

type CollectionZipListResponse struct {
	CollectionID uint64              `json:"collection_id"`
	Items        []CollectionZipItem `json:"items"`
}

type DownloadListItem struct {
	ID           uint64 `json:"id"`
	Title        string `json:"title"`
	Order        int    `json:"order"`
	FileURL      string `json:"file_url"`
	DownloadURL  string `json:"download_url"`
	SizeBytes    int64  `json:"size_bytes"`
	Format       string `json:"format"`
	CollectionID uint64 `json:"collection_id"`
}

type DownloadListResponse struct {
	CollectionID uint64             `json:"collection_id"`
	Items        []DownloadListItem `json:"items"`
	Total        int64              `json:"total"`
}

func ensureCollectionVisibleForRequester(c *gin.Context, collection models.Collection) bool {
	if isAdminRole(c) {
		return true
	}
	if !isPublicCollectionVisible(collection) {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return false
	}
	return true
}

func (h *Handler) requireActiveUser(c *gin.Context) (*models.User, bool) {
	roleVal, _ := c.Get("role")
	if role, ok := roleVal.(string); ok {
		if strings.EqualFold(role, "super_admin") || strings.EqualFold(role, "admin") {
			return nil, true
		}
	}

	userVal, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return nil, false
	}
	userID, ok := userVal.(uint64)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return nil, false
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return nil, false
	}
	if strings.ToLower(user.Status) != "active" {
		c.JSON(http.StatusForbidden, gin.H{"error": "user_disabled"})
		return &user, false
	}

	return &user, true
}

func (h *Handler) requireActiveSubscriber(c *gin.Context) (*models.User, bool) {
	user, ok := h.requireActiveUser(c)
	if !ok {
		return user, false
	}
	if user == nil {
		return nil, true
	}
	now := time.Now()
	syncExpiredSubscription(h.db, user, now)
	status, _, isSubscriber := resolveUserSubscriptionState(user, now)
	if !isSubscriber {
		c.JSON(http.StatusForbidden, gin.H{"error": "subscription_required", "subscription_status": status})
		return user, false
	}
	return user, true
}

func ensureCollectionDownloadAllowed(c *gin.Context, collection models.Collection) bool {
	if !collection.IsShowcase {
		return true
	}
	if isAdminRole(c) {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "collection_showcase_download_disabled"})
	return false
}

// GetCollectionZipDownload returns a download URL for the latest zip of a collection.
// @Summary Get collection zip download URL
// @Description 权限：需登录且账号激活；合集 ZIP 支持「订阅会员」或「合集次卡权益（entitlement）」任一满足即可下载。
// @Tags collections
// @Produce json
// @Param id path int true "collection id"
// @Param ttl query int false "ttl (seconds)"
// @Success 200 {object} DownloadURLResponse
// @Router /api/collections/{id}/download-zip [get]
func (h *Handler) GetCollectionZipDownload(c *gin.Context) {
	user, ok := h.requireActiveUser(c)
	if !ok {
		return
	}
	if user != nil && !h.guardCollectionDownload(c, user.ID) {
		return
	}
	if user != nil && !h.enforceRiskBlock(c, "download", "", "", user.ID) {
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
	if !ensureCollectionVisibleForRequester(c, collection) {
		return
	}
	if !ensureCollectionDownloadAllowed(c, collection) {
		return
	}
	accessDecision := h.resolveCollectionDownloadAccess(user, collection.ID, time.Now())
	if !accessDecision.Allowed {
		writeCollectionDownloadAccessDenied(c, accessDecision)
		return
	}

	zipKeyParam := strings.TrimSpace(c.Query("zip_key"))
	zipIDParam := strings.TrimSpace(c.Query("zip_id"))
	key := ""
	name := ""

	if zipIDParam != "" {
		zipID, err := strconv.ParseUint(zipIDParam, 10, 64)
		if err != nil || zipID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid zip_id"})
			return
		}
		var zip models.CollectionZip
		if err := h.db.Where("id = ? AND collection_id = ?", zipID, collection.ID).First(&zip).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "zip not found"})
			return
		}
		key = zip.ZipKey
		name = zip.ZipName
	}

	if key == "" && zipKeyParam != "" {
		key = zipKeyParam
		var zip models.CollectionZip
		if err := h.db.Where("collection_id = ? AND zip_key = ?", collection.ID, key).First(&zip).Error; err == nil {
			name = zip.ZipName
		}
	}

	if key == "" {
		key = strings.TrimSpace(collection.LatestZipKey)
		name = strings.TrimSpace(collection.LatestZipName)
	}

	if key == "" {
		zips, _ := loadCollectionZips(h.db, h.qiniu, collection)
		if len(zips) > 0 {
			latest := zips[0]
			key = latest.ZipKey
			name = latest.ZipName
			if collection.LatestZipKey == "" || collection.LatestZipKey != key {
				collection.LatestZipKey = key
				collection.LatestZipName = name
				now := time.Now()
				collection.LatestZipAt = &now
				_ = h.db.Model(&models.Collection{}).Where("id = ?", collection.ID).
					Updates(map[string]interface{}{
						"latest_zip_key":  collection.LatestZipKey,
						"latest_zip_name": collection.LatestZipName,
						"latest_zip_at":   collection.LatestZipAt,
					}).Error
			}
		}
	}

	if key == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "zip not found"})
		return
	}

	url, exp := resolveDownloadURL(key, h.qiniu, c.Query("ttl"))
	if url == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "download url unavailable"})
		return
	}
	downloadName := normalizeDownloadFileName(collection.Title, name, ".zip")
	ticketPayload := downloadTicketPayload{
		Kind:         "qiniu_object",
		Key:          key,
		Name:         downloadName,
		CollectionID: collection.ID,
	}
	if userID, hasUserID := currentUserIDFromContext(c); hasUserID && userID > 0 {
		ticketPayload.UserID = userID
	}
	if user != nil && !accessDecision.IsSubscriber {
		ticketPayload.EntitlementMode = "zip"
	}
	ticketURL, ticketExp, err := h.issueDownloadTicketWithPayload(c, ticketPayload)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "download unavailable"})
		return
	}
	url = ticketURL
	exp = ticketExp

	c.JSON(http.StatusOK, DownloadURLResponse{
		CollectionID: collection.ID,
		Key:          key,
		Name:         downloadName,
		URL:          url,
		ExpiresAt:    exp,
	})
}

// GetCollectionZipList returns all uploaded zips for a collection.
// @Summary Get collection zip list
// @Tags collections
// @Produce json
// @Param id path int true "collection id"
// @Success 200 {object} CollectionZipListResponse
// @Router /api/collections/{id}/zips [get]
func (h *Handler) GetCollectionZipList(c *gin.Context) {
	user, ok := h.requireActiveUser(c)
	if !ok {
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
	if !ensureCollectionVisibleForRequester(c, collection) {
		return
	}
	if !ensureCollectionDownloadAllowed(c, collection) {
		return
	}
	accessDecision := h.resolveCollectionDownloadAccess(user, collection.ID, time.Now())
	if !accessDecision.Allowed {
		writeCollectionDownloadAccessDenied(c, accessDecision)
		return
	}

	zips, err := loadCollectionZips(h.db, h.qiniu, collection)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	respItems := make([]CollectionZipItem, 0, len(zips))
	for _, zip := range zips {
		respItems = append(respItems, CollectionZipItem{
			ID:         zip.ID,
			Key:        zip.ZipKey,
			Name:       zip.ZipName,
			SizeBytes:  zip.SizeBytes,
			UploadedAt: zip.UploadedAt,
		})
	}

	c.JSON(http.StatusOK, CollectionZipListResponse{
		CollectionID: collection.ID,
		Items:        respItems,
	})
}

// GetCollectionZipDownloadAll returns a ticket URL for downloading all zip parts as one archive.
// @Summary Get collection aggregated zip download URL
// @Description 权限：需登录且账号激活；合集 ZIP（全分片聚合）支持「订阅会员」或「合集次卡权益（entitlement）」任一满足即可下载。
// @Tags collections
// @Produce json
// @Param id path int true "collection id"
// @Success 200 {object} DownloadURLResponse
// @Router /api/collections/{id}/download-zip-all [get]
func (h *Handler) GetCollectionZipDownloadAll(c *gin.Context) {
	user, ok := h.requireActiveUser(c)
	if !ok {
		return
	}
	if user != nil && !h.guardCollectionDownload(c, user.ID) {
		return
	}
	if user != nil && !h.enforceRiskBlock(c, "download", "", "", user.ID) {
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
	if !ensureCollectionVisibleForRequester(c, collection) {
		return
	}
	if !ensureCollectionDownloadAllowed(c, collection) {
		return
	}
	accessDecision := h.resolveCollectionDownloadAccess(user, collection.ID, time.Now())
	if !accessDecision.Allowed {
		writeCollectionDownloadAccessDenied(c, accessDecision)
		return
	}

	zips, err := loadCollectionZips(h.db, h.qiniu, collection)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(zips) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "zip not found"})
		return
	}
	filename := normalizeDownloadFileName(collection.Title, fmt.Sprintf("collection-%d-all", collection.ID), ".zip")
	zipItems := make([]downloadTicketZipItem, 0, len(zips))
	for _, item := range zips {
		entryName := strings.TrimSpace(item.ZipName)
		if entryName == "" {
			entryName = path.Base(strings.TrimSpace(item.ZipKey))
		}
		zipItems = append(zipItems, downloadTicketZipItem{
			Key:  strings.TrimSpace(item.ZipKey),
			Name: strings.TrimSpace(entryName),
		})
	}
	if len(zipItems) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "zip not found"})
		return
	}

	ticketPayload := downloadTicketPayload{
		Kind:         "collection_zip_aggregate",
		Name:         filename,
		CollectionID: collection.ID,
		ZipItems:     zipItems,
	}
	if userID, hasUserID := currentUserIDFromContext(c); hasUserID && userID > 0 {
		ticketPayload.UserID = userID
	}
	if user != nil && !accessDecision.IsSubscriber {
		ticketPayload.EntitlementMode = "zip_all"
	}
	ticketURL, ticketExp, err := h.issueDownloadTicketWithPayload(c, ticketPayload)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "download unavailable"})
		return
	}

	c.JSON(http.StatusOK, DownloadURLResponse{
		CollectionID: collection.ID,
		Name:         filename,
		URL:          ticketURL,
		ExpiresAt:    ticketExp,
	})
}

// GetCollectionDownloadList returns ordered emoji download URLs.
// @Summary Get collection download list
// @Description 权限：需登录且账号激活；当前接口仅订阅会员可调用（次卡用户请走 ZIP 下载接口）。
// @Tags collections
// @Produce json
// @Param id path int true "collection id"
// @Param ttl query int false "ttl (seconds)"
// @Success 200 {object} DownloadListResponse
// @Router /api/collections/{id}/download-list [get]
func (h *Handler) GetCollectionDownloadList(c *gin.Context) {
	if _, ok := h.requireActiveSubscriber(c); !ok {
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
	if !ensureCollectionVisibleForRequester(c, collection) {
		return
	}
	if !ensureCollectionDownloadAllowed(c, collection) {
		return
	}

	var total int64
	h.db.Model(&models.Emoji{}).Where("collection_id = ? AND status = ?", id, "active").Count(&total)

	var items []models.Emoji
	h.db.Where("collection_id = ? AND status = ?", id, "active").
		Order("case when display_order is null or display_order = 0 then 1 else 0 end").
		Order("display_order asc").
		Order("file_url asc").
		Order("id asc").
		Find(&items)

	resp := make([]DownloadListItem, 0, len(items))
	ttlParam := strings.TrimSpace(c.Query("ttl"))
	for _, item := range items {
		url, _ := resolveDownloadURL(item.FileURL, h.qiniu, ttlParam)
		resp = append(resp, DownloadListItem{
			ID:           item.ID,
			Title:        item.Title,
			Order:        item.DisplayOrder,
			FileURL:      item.FileURL,
			DownloadURL:  url,
			SizeBytes:    item.SizeBytes,
			Format:       item.Format,
			CollectionID: item.CollectionID,
		})
	}

	c.JSON(http.StatusOK, DownloadListResponse{
		CollectionID: collection.ID,
		Items:        resp,
		Total:        total,
	})
}

// GetEmojiDownload returns download URL for a single emoji.
// @Summary Get emoji download URL
// @Description 权限：需登录且账号激活；不要求订阅状态（仍受限频与风控策略约束）。
// @Tags emojis
// @Produce json
// @Param id path int true "emoji id"
// @Param ttl query int false "ttl (seconds)"
// @Success 200 {object} DownloadURLResponse
// @Router /api/emojis/{id}/download [get]
func (h *Handler) GetEmojiDownload(c *gin.Context) {
	user, ok := h.requireActiveUser(c)
	if !ok {
		return
	}
	if user != nil && !h.guardEmojiDownload(c, user.ID) {
		return
	}
	if user != nil && !h.enforceRiskBlock(c, "download", "", "", user.ID) {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var emoji models.Emoji
	if err := h.db.First(&emoji, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var collection models.Collection
	if err := h.db.Select("id", "status", "visibility", "is_showcase").First(&collection, emoji.CollectionID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	if !ensureCollectionVisibleForRequester(c, collection) {
		return
	}
	if !ensureCollectionDownloadAllowed(c, collection) {
		return
	}

	url, exp := resolveDownloadURL(emoji.FileURL, h.qiniu, c.Query("ttl"))
	if url == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "download url unavailable"})
		return
	}
	downloadName := normalizeDownloadFileName(emoji.Title, path.Base(strings.TrimSpace(emoji.FileURL)), inferEmojiDownloadExt(emoji))
	ticketPayload := downloadTicketPayload{
		Kind:    "qiniu_object",
		Key:     emoji.FileURL,
		Name:    downloadName,
		EmojiID: emoji.ID,
	}
	if userID, hasUserID := currentUserIDFromContext(c); hasUserID && userID > 0 {
		ticketPayload.UserID = userID
	}
	ticketURL, ticketExp, err := h.issueDownloadTicketWithPayload(c, ticketPayload)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "download unavailable"})
		return
	}
	url = ticketURL
	exp = ticketExp

	c.JSON(http.StatusOK, DownloadURLResponse{
		EmojiID:   emoji.ID,
		Key:       emoji.FileURL,
		Name:      downloadName,
		URL:       url,
		ExpiresAt: exp,
	})
}

// DownloadEmojiFile streams emoji file directly for local download.
// @Summary Download emoji file directly
// @Description 权限：需登录且账号激活；不要求订阅状态（仍受限频与风控策略约束）。
// @Tags emojis
// @Produce application/octet-stream
// @Param id path int true "emoji id"
// @Param ttl query int false "ttl (seconds)"
// @Router /api/emojis/{id}/download-file [get]
func (h *Handler) DownloadEmojiFile(c *gin.Context) {
	user, ok := h.requireActiveUser(c)
	if !ok {
		return
	}
	if user != nil && !h.guardEmojiDownload(c, user.ID) {
		return
	}
	if user != nil && !h.enforceRiskBlock(c, "download", "", "", user.ID) {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var emoji models.Emoji
	if err := h.db.First(&emoji, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var collection models.Collection
	if err := h.db.Select("id", "status", "visibility", "is_showcase").First(&collection, emoji.CollectionID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	if !ensureCollectionVisibleForRequester(c, collection) {
		return
	}
	if !ensureCollectionDownloadAllowed(c, collection) {
		return
	}

	rawURL, _ := resolveDownloadURL(emoji.FileURL, h.qiniu, c.Query("ttl"))
	if rawURL == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "download url unavailable"})
		return
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := downloadZipPart(client, rawURL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch emoji file"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch emoji file"})
		return
	}

	downloadName := normalizeDownloadFileName(emoji.Title, path.Base(strings.TrimSpace(emoji.FileURL)), inferEmojiDownloadExt(emoji))
	asciiFallback := normalizeDownloadFileName("", fmt.Sprintf("emoji-%d", emoji.ID), inferEmojiDownloadExt(emoji))
	encodedFilename := strings.ReplaceAll(url.QueryEscape(downloadName), "+", "%20")
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", asciiFallback, encodedFilename))
	if resp.ContentLength > 0 {
		c.Header("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	c.Status(http.StatusOK)

	if _, err := io.Copy(c.Writer, resp.Body); err == nil {
		h.recordEmojiDownload(c, emoji.ID, 0)
	}
}

// DownloadVideoJobEmojiFile streams a job output file directly for local download.
// @Summary Download video job output file directly
// @Description 权限：仅任务所属用户可下载自己的创作产物；不要求订阅状态（仍受限频与风控策略约束）。
// @Tags user
// @Produce application/octet-stream
// @Param id path int true "video job id"
// @Param emojiID path int true "emoji/output id in result collection"
// @Param ttl query int false "ttl (seconds)"
// @Router /api/video-jobs/{id}/emojis/{emojiID}/download-file [get]
func (h *Handler) DownloadVideoJobEmojiFile(c *gin.Context) {
	user, ok := h.requireActiveUser(c)
	if !ok {
		return
	}
	if user != nil && !h.guardEmojiDownload(c, user.ID) {
		return
	}
	if user != nil && !h.enforceRiskBlock(c, "download", "", "", user.ID) {
		return
	}

	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}
	emojiID, err := strconv.ParseUint(c.Param("emojiID"), 10, 64)
	if err != nil || emojiID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid emoji id"})
		return
	}

	var job models.VideoJob
	query := h.db.Where("id = ?", jobID)
	if !isAdminRole(c) {
		if user == nil || user.ID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		query = query.Where("user_id = ?", user.ID)
	}
	if err := query.First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job.ResultCollectionID == nil || *job.ResultCollectionID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "result collection not found"})
		return
	}

	collection, err := h.loadVideoJobResultCollection(job)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "result collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	emoji, err := h.loadVideoJobEmojiByDomain(job.AssetDomain, emojiID, collection.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "output not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if status := strings.ToLower(strings.TrimSpace(emoji.Status)); status != "" && status != "active" {
		c.JSON(http.StatusNotFound, gin.H{"error": "output not found"})
		return
	}

	rawURL, _ := resolveDownloadURL(emoji.FileURL, h.qiniu, c.Query("ttl"))
	if rawURL == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "download url unavailable"})
		return
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := downloadZipPart(client, rawURL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch output file"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch output file"})
		return
	}

	downloadName := normalizeDownloadFileName(emoji.Title, path.Base(strings.TrimSpace(emoji.FileURL)), inferEmojiDownloadExt(emoji))
	asciiFallback := normalizeDownloadFileName("", fmt.Sprintf("video-job-emoji-%d", emojiID), inferEmojiDownloadExt(emoji))
	encodedFilename := strings.ReplaceAll(url.QueryEscape(downloadName), "+", "%20")
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", asciiFallback, encodedFilename))
	if resp.ContentLength > 0 {
		c.Header("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	c.Status(http.StatusOK)

	if _, err := io.Copy(c.Writer, resp.Body); err == nil {
		if user != nil && normalizeVideoJobAssetDomain(job.AssetDomain) != models.VideoJobAssetDomainVideo {
			h.recordEmojiDownload(c, emoji.ID, user.ID)
		}
	}
}

func (h *Handler) recordCollectionDownload(c *gin.Context, collectionID, fallbackUserID uint64) {
	download := models.CollectionDownload{
		CollectionID: collectionID,
		IP:           c.ClientIP(),
	}
	userID := fallbackUserID
	if uid, ok := currentUserIDFromContext(c); ok && uid > 0 {
		userID = uid
	}
	if userID > 0 {
		uid := userID
		download.UserID = &uid
	}
	_ = h.db.Create(&download).Error
}

func (h *Handler) recordEmojiDownload(c *gin.Context, emojiID, fallbackUserID uint64) {
	userID := fallbackUserID
	download := models.Download{
		EmojiID: emojiID,
		IP:      c.ClientIP(),
	}
	if uid, ok := currentUserIDFromContext(c); ok && uid > 0 {
		userID = uid
	}
	if userID > 0 {
		uid := userID
		download.UserID = &uid
	}
	_ = h.db.Create(&download).Error
	h.bumpVideoJobFeedbackByEmojiID(emojiID, "download", userID)
	h.recordVideoImageFeedbackByEmojiID(emojiID, videoImageFeedbackActionDownload, userID, nil)
}

func (h *Handler) streamCollectionZipAggregateByTicket(c *gin.Context, payload *downloadTicketPayload) {
	if payload == nil || payload.CollectionID == 0 || len(payload.ZipItems) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ticket"})
		return
	}

	filename := normalizeDownloadFileName("", payload.Name, ".zip")
	asciiFallback := fmt.Sprintf("collection-%d-all.zip", payload.CollectionID)
	encodedFilename := strings.ReplaceAll(url.QueryEscape(filename), "+", "%20")
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", asciiFallback, encodedFilename))
	c.Status(http.StatusOK)

	zw := zip.NewWriter(c.Writer)
	client := &http.Client{Timeout: 10 * time.Minute}
	signTTL := h.cfg.DownloadTicketSignTTL
	if signTTL <= 0 {
		signTTL = 180
	}
	usedNames := make(map[string]int)
	for _, item := range payload.ZipItems {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			continue
		}
		entryName := strings.TrimSpace(item.Name)
		if entryName == "" {
			entryName = path.Base(key)
		}
		if !strings.HasSuffix(strings.ToLower(entryName), ".zip") {
			entryName += ".zip"
		}
		entryName = nextZipEntryName(entryName, usedNames)

		partURL, _ := resolveDownloadURL(key, h.qiniu, strconv.Itoa(signTTL))
		if partURL == "" {
			continue
		}
		resp, err := downloadZipPart(client, partURL)
		if err != nil {
			continue
		}
		if resp.StatusCode >= 300 {
			resp.Body.Close()
			continue
		}
		zipEntry, err := zw.Create(entryName)
		if err != nil {
			resp.Body.Close()
			continue
		}
		_, _ = io.Copy(zipEntry, resp.Body)
		resp.Body.Close()
	}
	_ = zw.Close()
}

func resolveDownloadURL(fileURL string, qiniuClient *storage.QiniuClient, ttlParam string) (string, int64) {
	key := strings.TrimSpace(fileURL)
	if key == "" {
		return "", 0
	}
	if qiniuClient == nil {
		return key, 0
	}
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		if extracted, ok := extractQiniuObjectKey(key, qiniuClient); ok {
			key = extracted
		} else {
			return key, 0
		}
	}
	if qiniuClient.Private {
		ttl, _ := strconv.Atoi(strings.TrimSpace(ttlParam))
		if ttl <= 0 {
			ttl = 3600
		}
		url, err := qiniuClient.SignedURL(key, int64(ttl))
		if err != nil {
			return "", 0
		}
		return url, time.Now().Unix() + int64(ttl)
	}
	return qiniuClient.PublicURL(key), 0
}

func loadCollectionZips(db *gorm.DB, qiniuClient *storage.QiniuClient, collection models.Collection) ([]models.CollectionZip, error) {
	var zips []models.CollectionZip
	if err := db.Where("collection_id = ?", collection.ID).
		Order("uploaded_at desc nulls last, id desc").
		Find(&zips).Error; err != nil {
		return nil, err
	}

	if qiniuClient != nil {
		prefix := strings.TrimSpace(collection.QiniuPrefix)
		if prefix != "" {
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			meta, err := fetchMetaJSON(qiniuClient, prefix+"meta.json")
			if err == nil && meta != nil {
				rawZips, ok := meta["zips"].([]interface{})
				if ok && len(rawZips) > 0 {
					for _, entry := range rawZips {
						row, ok := entry.(map[string]interface{})
						if !ok {
							continue
						}
						key, _ := row["key"].(string)
						name, _ := row["name"].(string)
						if strings.TrimSpace(key) == "" {
							continue
						}
						var uploadedAt *time.Time
						if addedAt, ok := row["addedAt"].(string); ok && addedAt != "" {
							if parsed, err := time.Parse(time.RFC3339, addedAt); err == nil {
								uploadedAt = &parsed
							}
						}
						hash, _ := row["hash"].(string)
						zip := models.CollectionZip{
							CollectionID: collection.ID,
							ZipKey:       key,
							ZipHash:      strings.TrimSpace(hash),
							ZipName:      name,
							UploadedAt:   uploadedAt,
						}
						_ = db.Where("collection_id = ? AND zip_key = ?", collection.ID, key).
							FirstOrCreate(&zip).Error
					}
				}
			}

			// Fallback: list all source zip parts directly from Qiniu.
			sourcePrefix := prefix + "source/"
			bm := qiniuClient.BucketManager()
			marker := ""
			for {
				items, _, nextMarker, hasNext, err := bm.ListFiles(qiniuClient.Bucket, sourcePrefix, "", marker, 1000)
				if err != nil {
					break
				}
				for _, item := range items {
					key := strings.TrimSpace(item.Key)
					if key == "" || !strings.HasSuffix(strings.ToLower(key), ".zip") {
						continue
					}
					name := path.Base(key)
					uploadedAt := qiniuPutTimeToTime(item.PutTime)
					zip := models.CollectionZip{
						CollectionID: collection.ID,
						ZipKey:       key,
					}
					_ = db.Where("collection_id = ? AND zip_key = ?", collection.ID, key).
						Assign(models.CollectionZip{
							ZipName:    name,
							ZipHash:    item.Hash,
							SizeBytes:  item.Fsize,
							UploadedAt: uploadedAt,
						}).
						FirstOrCreate(&zip).Error
				}
				if !hasNext || nextMarker == "" {
					break
				}
				marker = nextMarker
			}
		}
	}

	if collection.LatestZipKey != "" {
		zip := models.CollectionZip{
			CollectionID: collection.ID,
			ZipKey:       collection.LatestZipKey,
			ZipName:      collection.LatestZipName,
			SizeBytes:    collection.LatestZipSize,
			UploadedAt:   collection.LatestZipAt,
		}
		_ = db.Where("collection_id = ? AND zip_key = ?", collection.ID, collection.LatestZipKey).
			FirstOrCreate(&zip).Error
	}

	_ = db.Where("collection_id = ?", collection.ID).
		Order("uploaded_at desc nulls last, id desc").
		Find(&zips).Error

	return zips, nil
}

func qiniuPutTimeToTime(putTime int64) *time.Time {
	if putTime <= 0 {
		return nil
	}
	// Qiniu put_time is in 100ns ticks.
	ns := putTime * 100
	t := time.Unix(0, ns)
	return &t
}

func downloadZipPart(client *http.Client, rawURL string) (*http.Response, error) {
	resp, err := client.Get(rawURL)
	if err == nil {
		return resp, nil
	}
	if strings.HasPrefix(strings.ToLower(rawURL), "https://") {
		fallbackURL := "http://" + strings.TrimPrefix(rawURL, "https://")
		return client.Get(fallbackURL)
	}
	return nil, err
}

func nextZipEntryName(name string, used map[string]int) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "part.zip"
	}
	count := used[base]
	used[base] = count + 1
	if count == 0 {
		return base
	}
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return fmt.Sprintf("%s(%d)%s", stem, count+1, ext)
}

func normalizeDownloadFileName(title, fallback, ext string) string {
	base := strings.TrimSpace(title)
	if base == "" {
		base = strings.TrimSpace(fallback)
	}
	if base == "" {
		base = "download"
	}
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	base = strings.TrimSpace(replacer.Replace(base))
	if base == "" {
		base = "download"
	}
	if ext != "" && !strings.HasSuffix(strings.ToLower(base), strings.ToLower(ext)) {
		base += ext
	}
	return base
}

func appendDownloadAttname(rawURL, filename string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return rawURL
	}
	if strings.TrimSpace(filename) == "" {
		return rawURL
	}
	query := parsed.Query()
	query.Set("attname", filename)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func inferEmojiDownloadExt(emoji models.Emoji) string {
	ext := strings.ToLower(strings.TrimSpace(path.Ext(strings.TrimSpace(emoji.FileURL))))
	if ext != "" && len(ext) <= 8 {
		return ext
	}
	format := strings.ToLower(strings.TrimSpace(emoji.Format))
	switch {
	case strings.Contains(format, "png"):
		return ".png"
	case strings.Contains(format, "gif"):
		return ".gif"
	case strings.Contains(format, "webp"):
		return ".webp"
	case strings.Contains(format, "jpeg"), strings.Contains(format, "jpg"):
		return ".jpg"
	case strings.Contains(format, "svg"):
		return ".svg"
	default:
		return ""
	}
}
