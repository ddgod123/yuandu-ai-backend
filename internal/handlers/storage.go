package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
)

type UploadTokenRequest struct {
	Key        string `json:"key"`
	Prefix     string `json:"prefix"`
	Collection uint64 `json:"collection_id"`
	Expires    int64  `json:"expires"`
	InsertOnly bool   `json:"insert_only"`
}

type UploadTokenResponse struct {
	Token     string `json:"token"`
	Bucket    string `json:"bucket"`
	Key       string `json:"key,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	ExpiresAt int64  `json:"expires_at"`
	UpHost    string `json:"up_host,omitempty"`
}

func normalizeUploadTokenExpire(raw int64, fallback int) int64 {
	expires := raw
	if expires <= 0 {
		expires = int64(fallback)
	}
	if expires < 60 {
		expires = 60
	}
	if expires > 86400 {
		expires = 86400
	}
	return expires
}

func issueQiniuUploadToken(h *Handler, key, prefix string, expires int64, insertOnly bool) UploadTokenResponse {
	policy := qiniustorage.PutPolicy{}
	if key != "" {
		policy.Scope = h.qiniu.Bucket + ":" + key
	} else {
		policy.Scope = h.qiniu.Bucket + ":" + prefix
		policy.IsPrefixalScope = 1
	}
	policy.Expires = uint64(expires)
	if insertOnly {
		policy.InsertOnly = 1
	}

	token := policy.UploadToken(h.qiniu.Mac)
	resp := UploadTokenResponse{
		Token:     token,
		Bucket:    h.qiniu.Bucket,
		Key:       key,
		Prefix:    prefix,
		ExpiresAt: time.Now().Unix() + expires,
	}

	upHost, err := qiniustorage.NewFormUploader(h.qiniu.Cfg).UpHost(h.qiniu.Mac.AccessKey, h.qiniu.Bucket)
	if err == nil {
		resp.UpHost = normalizeUploadHost(upHost)
	}
	return resp
}

func normalizeUploadHost(raw string) string {
	host := strings.TrimSpace(raw)
	if host == "" {
		return ""
	}
	if strings.HasPrefix(host, "//") {
		return "https:" + host
	}
	if strings.HasPrefix(host, "http://") {
		return "https://" + strings.TrimPrefix(host, "http://")
	}
	if strings.HasPrefix(host, "https://") {
		return host
	}
	return "https://" + host
}

// GetUploadToken godoc
// @Summary Get upload token
// @Description Generate Qiniu upload token for key or prefix (default emoji/)
// @Tags storage
// @Accept json
// @Produce json
// @Param body body UploadTokenRequest true "upload token request"
// @Success 200 {object} UploadTokenResponse
// @Router /api/storage/upload-token [post]
func (h *Handler) GetUploadToken(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}

	var req UploadTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	key := strings.TrimSpace(req.Key)
	prefix := strings.TrimSpace(req.Prefix)
	if req.Collection > 0 && prefix == "" && key == "" {
		prefix = path.Join("emoji", strconv.FormatUint(req.Collection, 10)) + "/"
	}
	if prefix == "" && key == "" {
		prefix = "emoji/"
	}
	if key != "" {
		key = strings.TrimLeft(key, "/")
		if !strings.HasPrefix(key, "emoji/") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "key must start with emoji/"})
			return
		}
	}
	if prefix != "" {
		prefix = strings.TrimLeft(prefix, "/")
		if !strings.HasPrefix(prefix, "emoji/") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prefix must start with emoji/"})
			return
		}
	}

	expires := normalizeUploadTokenExpire(req.Expires, h.qiniu.SignTTL)
	resp := issueQiniuUploadToken(h, key, prefix, expires, req.InsertOnly)
	c.JSON(http.StatusOK, resp)
}

// GetVideoJobUploadToken godoc
// @Summary Get upload token for video-job source upload
// @Description Authenticated user upload token, scoped to emoji/user-video/...
// @Tags storage
// @Accept json
// @Produce json
// @Param body body UploadTokenRequest true "upload token request"
// @Success 200 {object} UploadTokenResponse
// @Router /api/video-jobs/upload-token [post]
func (h *Handler) GetVideoJobUploadToken(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req UploadTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userPrefix := path.Join("emoji", "user-video", strconv.FormatUint(userID, 10)) + "/"
	legacyPrefix := "emoji/user-video/"

	key := strings.TrimLeft(strings.TrimSpace(req.Key), "/")
	prefix := strings.TrimLeft(strings.TrimSpace(req.Prefix), "/")

	if key == "" && prefix == "" {
		prefix = userPrefix
	}
	if key != "" {
		if !strings.HasPrefix(key, userPrefix) && !strings.HasPrefix(key, legacyPrefix) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden key"})
			return
		}
	}
	if prefix != "" {
		if !strings.HasPrefix(prefix, userPrefix) && !strings.HasPrefix(prefix, legacyPrefix) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden key"})
			return
		}
	}

	insertOnly := true
	expires := normalizeUploadTokenExpire(req.Expires, h.qiniu.SignTTL)
	resp := issueQiniuUploadToken(h, key, prefix, expires, insertOnly)
	c.JSON(http.StatusOK, resp)
}

type QiniuFileInfo struct {
	Fsize     int64  `json:"fsize"`
	Hash      string `json:"hash"`
	MimeType  string `json:"mime_type"`
	Type      int    `json:"type"`
	PutTime   int64  `json:"put_time"`
	Status    int    `json:"status"`
	Md5       string `json:"md5"`
	EndUser   string `json:"end_user"`
	ExpiresAt int64  `json:"expiration"`
}

type StorageObjectResponse struct {
	Key  string        `json:"key"`
	Info QiniuFileInfo `json:"info"`
}

// GetObject godoc
// @Summary Get object info
// @Tags storage
// @Produce json
// @Param key query string true "object key"
// @Success 200 {object} StorageObjectResponse
// @Router /api/storage/object [get]
func (h *Handler) GetObject(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	key := strings.TrimLeft(c.Query("key"), "/")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing key"})
		return
	}

	bm := h.qiniu.BucketManager()
	info, err := bm.Stat(h.qiniu.Bucket, key)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, StorageObjectResponse{
		Key:  key,
		Info: mapFileInfo(info),
	})
}

// ProxyObject godoc
// @Summary Proxy object content (temporary fallback)
// @Tags storage
// @Produce application/octet-stream
// @Param key query string false "object key (emoji/...)"
// @Param url query string false "raw object url"
// @Success 200 {string} string
// @Router /api/storage/proxy [get]
func (h *Handler) ProxyObject(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}

	key := strings.TrimSpace(c.Query("key"))
	rawURL := strings.TrimSpace(c.Query("url"))
	if key == "" && rawURL != "" {
		if extracted, ok := extractQiniuObjectKey(rawURL, h.qiniu); ok {
			key = extracted
		} else {
			key = rawURL
		}
	}
	key = normalizeStorageObjectKey(key)
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing key"})
		return
	}
	if isAdminRole(c) {
		// Admin callers can proxy arbitrary keys for operational workflows.
	} else if !canProxyStorageKeyPublic(key) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden key"})
		return
	}

	input := &qiniustorage.GetObjectInput{}
	if rangeHeader := strings.TrimSpace(c.GetHeader("Range")); rangeHeader != "" {
		input.Range = rangeHeader
	}
	output, err := h.qiniu.BucketManager().Get(h.qiniu.Bucket, key, input)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch object"})
		return
	}
	defer output.Close()

	contentType := strings.TrimSpace(output.ContentType)
	if contentType == "" {
		contentType = strings.TrimSpace(mime.TypeByExtension(strings.ToLower(path.Ext(key))))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=120")
	c.Header("X-Emoji-Storage-Proxy", "1")
	if output.ETag != "" {
		c.Header("ETag", output.ETag)
	}
	if output.ContentLength > 0 {
		c.Header("Content-Length", strconv.FormatInt(output.ContentLength, 10))
	}
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, output.Body)
}

type QiniuListItem struct {
	Key      string `json:"key"`
	PutTime  int64  `json:"put_time"`
	Hash     string `json:"hash"`
	Fsize    int64  `json:"fsize"`
	MimeType string `json:"mime_type"`
	EndUser  string `json:"end_user"`
	Type     int    `json:"type"`
	Status   int    `json:"status"`
	Md5      string `json:"md5"`
}

type ListObjectsResponse struct {
	Items      []QiniuListItem `json:"items"`
	Prefixes   []string        `json:"prefixes"`
	NextMarker string          `json:"next_marker"`
	HasNext    bool            `json:"has_next"`
}

// ListObjects godoc
// @Summary List objects
// @Tags storage
// @Produce json
// @Param prefix query string false "prefix" default(emoji/)
// @Param marker query string false "marker"
// @Param limit query int false "limit" default(50)
// @Success 200 {object} ListObjectsResponse
// @Router /api/storage/objects [get]
func (h *Handler) ListObjects(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}

	prefix := strings.TrimLeft(strings.TrimSpace(c.DefaultQuery("prefix", "emoji/")), "/")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	marker := c.Query("marker")

	bm := h.qiniu.BucketManager()
	items, prefixes, nextMarker, hasNext, err := bm.ListFiles(h.qiniu.Bucket, prefix, "", marker, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	listItems := make([]QiniuListItem, 0, len(items))
	for _, item := range items {
		listItems = append(listItems, mapListItem(item))
	}

	c.JSON(http.StatusOK, ListObjectsResponse{
		Items:      listItems,
		Prefixes:   prefixes,
		NextMarker: nextMarker,
		HasNext:    hasNext,
	})
}

type RenameObjectRequest struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Force bool   `json:"force"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

// RenameObject godoc
// @Summary Rename object
// @Tags storage
// @Accept json
// @Produce json
// @Param body body RenameObjectRequest true "rename request"
// @Success 200 {object} MessageResponse
// @Router /api/storage/rename [post]
func (h *Handler) RenameObject(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}

	var req RenameObjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	from := strings.TrimLeft(strings.TrimSpace(req.From), "/")
	to := strings.TrimLeft(strings.TrimSpace(req.To), "/")
	if from == "" || to == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from/to required"})
		return
	}

	bm := h.qiniu.BucketManager()
	if err := bm.Move(h.qiniu.Bucket, from, h.qiniu.Bucket, to, req.Force); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "renamed"})
}

// DeleteObject godoc
// @Summary Delete object
// @Tags storage
// @Produce json
// @Param key query string true "object key"
// @Success 200 {object} MessageResponse
// @Router /api/storage/object [delete]
func (h *Handler) DeleteObject(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	key := strings.TrimLeft(c.Query("key"), "/")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing key"})
		return
	}

	bm := h.qiniu.BucketManager()
	if err := bm.Delete(h.qiniu.Bucket, key); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "deleted"})
}

type ObjectURLResponse struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
}

type BatchObjectURLRequest struct {
	Keys    []string `json:"keys"`
	TTL     int      `json:"ttl"`
	Private bool     `json:"private"`
	Style   string   `json:"style"`
}

type BatchObjectURLItem struct {
	Key       string `json:"key"`
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
}

type BatchObjectURLResponse struct {
	Items []BatchObjectURLItem `json:"items"`
}

type storageURLCacheValue struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
}

var publicProxyAllowedImageExt = map[string]struct{}{
	".gif":  {},
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".webp": {},
	".avif": {},
	".bmp":  {},
}

func normalizeStorageObjectKey(raw string) string {
	key := strings.TrimSpace(raw)
	key = strings.TrimLeft(strings.SplitN(strings.SplitN(key, "?", 2)[0], "#", 2)[0], "/")
	return key
}

func canProxyStorageKeyPublic(key string) bool {
	normalized := normalizeStorageObjectKey(key)
	if normalized == "" {
		return false
	}
	if !strings.HasPrefix(normalized, "emoji/") {
		return false
	}
	lower := strings.ToLower(normalized)
	if strings.Contains(lower, "/zip/") || strings.Contains(lower, "/zips/") || strings.HasSuffix(lower, ".zip") {
		return false
	}
	ext := strings.ToLower(path.Ext(lower))
	_, ok := publicProxyAllowedImageExt[ext]
	return ok
}

func canAccessStorageKey(c *gin.Context, key string) bool {
	trimmed := normalizeStorageObjectKey(key)
	if trimmed == "" {
		return false
	}
	// Signed URL issuing is admin-only to avoid bypassing business download gates.
	if !isAdminRole(c) {
		return false
	}
	return true
}

const (
	signedURLCacheSeconds = int64(300) // 5 minutes
	signedURLCacheSkewSec = int64(20)  // avoid serving near-expiry cache
)

func storageURLCacheKey(key string, ttl int64, forcePrivate bool, style string) string {
	raw := strings.TrimSpace(key) + "|" + strconv.FormatInt(ttl, 10) + "|" + strconv.FormatBool(forcePrivate) + "|" + strings.TrimSpace(style)
	sum := sha256.Sum256([]byte(raw))
	return "cache:storage:url:v1:" + hex.EncodeToString(sum[:])
}

func normalizeStorageURLStyle(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "none":
		return ""
	case "cover_static":
		// 管理后台列表封面：静态首帧 + 小图尺寸 + webp，减少 GIF 动图解码和带宽占用
		return "imageMogr2/thumbnail/!160x160r/gravity/Center/crop/160x160/format/webp"
	case "ip_cover_card":
		// IP 卡片封面：统一 1.875:1（1200x640）居中裁剪，前后台展示一致
		return "imageMogr2/thumbnail/!1200x640r/gravity/Center/crop/1200x640/format/webp"
	default:
		return ""
	}
}

func (h *Handler) readStorageURLCache(ctx context.Context, cacheKey string) (ObjectURLResponse, bool) {
	if h.smsLimiter == nil || strings.TrimSpace(cacheKey) == "" {
		return ObjectURLResponse{}, false
	}
	raw, err := h.smsLimiter.GetValue(ctx, cacheKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return ObjectURLResponse{}, false
	}
	var cached storageURLCacheValue
	if err := json.Unmarshal([]byte(raw), &cached); err != nil {
		return ObjectURLResponse{}, false
	}
	now := time.Now().Unix()
	if strings.TrimSpace(cached.URL) == "" || cached.ExpiresAt <= now+signedURLCacheSkewSec {
		_ = h.smsLimiter.Delete(ctx, cacheKey)
		return ObjectURLResponse{}, false
	}
	return ObjectURLResponse{URL: cached.URL, ExpiresAt: cached.ExpiresAt}, true
}

func (h *Handler) writeStorageURLCache(ctx context.Context, cacheKey string, value ObjectURLResponse) {
	if h.smsLimiter == nil || strings.TrimSpace(cacheKey) == "" {
		return
	}
	if strings.TrimSpace(value.URL) == "" || value.ExpiresAt <= 0 {
		return
	}
	now := time.Now().Unix()
	maxCacheTTL := value.ExpiresAt - now - signedURLCacheSkewSec
	if maxCacheTTL <= 1 {
		return
	}
	cacheTTL := signedURLCacheSeconds
	if maxCacheTTL < cacheTTL {
		cacheTTL = maxCacheTTL
	}
	payload, err := json.Marshal(storageURLCacheValue{
		URL:       value.URL,
		ExpiresAt: value.ExpiresAt,
	})
	if err != nil {
		return
	}
	_ = h.smsLimiter.SetValue(ctx, cacheKey, string(payload), time.Duration(cacheTTL)*time.Second)
}

func normalizeStorageURLTTL(raw int, fallback int) int {
	ttl := raw
	if ttl <= 0 {
		ttl = fallback
	}
	if ttl <= 0 {
		ttl = 3600
	}
	if ttl < 60 {
		ttl = 60
	}
	if ttl > 86400 {
		ttl = 86400
	}
	return ttl
}

func (h *Handler) resolveObjectURLWithCache(ctx context.Context, key string, ttl int, forcePrivate bool, style string) (ObjectURLResponse, error) {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if key == "" {
		return ObjectURLResponse{}, nil
	}
	if h.qiniu == nil {
		return ObjectURLResponse{}, nil
	}
	style = strings.TrimPrefix(strings.TrimSpace(style), "?")

	if h.qiniu.Private || forcePrivate {
		signedTTL := normalizeStorageURLTTL(ttl, h.qiniu.SignTTL)
		cacheKey := storageURLCacheKey(key, int64(signedTTL), forcePrivate, style)
		if cached, ok := h.readStorageURLCache(ctx, cacheKey); ok {
			return cached, nil
		}
		var (
			url string
			err error
		)
		if style != "" {
			url, err = h.qiniu.SignedURLWithQuery(key, style, int64(signedTTL))
		} else {
			url, err = h.qiniu.SignedURL(key, int64(signedTTL))
		}
		if err != nil {
			return ObjectURLResponse{}, err
		}
		resp := ObjectURLResponse{
			URL:       url,
			ExpiresAt: time.Now().Unix() + int64(signedTTL),
		}
		h.writeStorageURLCache(ctx, cacheKey, resp)
		return resp, nil
	}

	if style != "" {
		return ObjectURLResponse{URL: h.qiniu.PublicURLWithQuery(key, style), ExpiresAt: 0}, nil
	}
	return ObjectURLResponse{URL: h.qiniu.PublicURL(key), ExpiresAt: 0}, nil
}

// GetObjectURL godoc
// @Summary Get object URL
// @Tags storage
// @Produce json
// @Param key query string true "object key"
// @Param ttl query int false "ttl (seconds)" default(3600)
// @Param private query bool false "force private url"
// @Param style query string false "url style preset, e.g. cover_static"
// @Success 200 {object} ObjectURLResponse
// @Router /api/storage/url [get]
func (h *Handler) GetObjectURL(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	if !isAdminRole(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}
	key := strings.TrimLeft(c.Query("key"), "/")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing key"})
		return
	}
	if !canAccessStorageKey(c, key) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden key"})
		return
	}
	privateParam := strings.ToLower(c.DefaultQuery("private", ""))
	forcePrivate := privateParam == "1" || privateParam == "true" || privateParam == "yes"
	if forcePrivate && !isAdminRole(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "private url requires admin"})
		return
	}
	style := normalizeStorageURLStyle(c.Query("style"))
	ttl, _ := strconv.Atoi(c.DefaultQuery("ttl", "0"))
	resp, err := h.resolveObjectURLWithCache(c.Request.Context(), key, ttl, forcePrivate, style)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// GetObjectURLs godoc
// @Summary Batch get object URLs
// @Tags storage
// @Accept json
// @Produce json
// @Param body body BatchObjectURLRequest true "batch object urls request"
// @Success 200 {object} BatchObjectURLResponse
// @Router /api/storage/urls [post]
func (h *Handler) GetObjectURLs(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	if !isAdminRole(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var req BatchObjectURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keys required"})
		return
	}
	if req.Private && !isAdminRole(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "private url requires admin"})
		return
	}
	style := normalizeStorageURLStyle(req.Style)

	seen := make(map[string]struct{}, len(req.Keys))
	keys := make([]string, 0, len(req.Keys))
	for _, raw := range req.Keys {
		key := strings.TrimLeft(strings.TrimSpace(raw), "/")
		if key == "" {
			continue
		}
		if !canAccessStorageKey(c, key) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden key"})
			return
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
		if len(keys) >= 300 {
			break
		}
	}
	if len(keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keys required"})
		return
	}

	items := make([]BatchObjectURLItem, 0, len(keys))
	for _, key := range keys {
		resp, err := h.resolveObjectURLWithCache(c.Request.Context(), key, req.TTL, req.Private, style)
		if err != nil || strings.TrimSpace(resp.URL) == "" {
			continue
		}
		items = append(items, BatchObjectURLItem{
			Key:       key,
			URL:       resp.URL,
			ExpiresAt: resp.ExpiresAt,
		})
	}

	c.JSON(http.StatusOK, BatchObjectURLResponse{Items: items})
}

func mapFileInfo(info qiniustorage.FileInfo) QiniuFileInfo {
	return QiniuFileInfo{
		Fsize:     info.Fsize,
		Hash:      info.Hash,
		MimeType:  info.MimeType,
		Type:      info.Type,
		PutTime:   info.PutTime,
		Status:    info.Status,
		Md5:       info.Md5,
		EndUser:   info.EndUser,
		ExpiresAt: info.Expiration,
	}
}

func mapListItem(item qiniustorage.ListItem) QiniuListItem {
	return QiniuListItem{
		Key:      item.Key,
		PutTime:  item.PutTime,
		Hash:     item.Hash,
		Fsize:    item.Fsize,
		MimeType: item.MimeType,
		EndUser:  item.EndUser,
		Type:     item.Type,
		Status:   item.Status,
		Md5:      item.Md5,
	}
}
