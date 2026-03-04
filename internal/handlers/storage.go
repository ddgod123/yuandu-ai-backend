package handlers

import (
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

	expires := req.Expires
	if expires <= 0 {
		expires = int64(h.qiniu.SignTTL)
	}
	if expires < 60 {
		expires = 60
	}
	if expires > 86400 {
		expires = 86400
	}

	policy := qiniustorage.PutPolicy{}
	if key != "" {
		policy.Scope = h.qiniu.Bucket + ":" + key
	} else {
		policy.Scope = h.qiniu.Bucket + ":" + prefix
		policy.IsPrefixalScope = 1
	}
	policy.Expires = uint64(expires)
	if req.InsertOnly {
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
		resp.UpHost = upHost
	}

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

// GetObjectURL godoc
// @Summary Get object URL
// @Tags storage
// @Produce json
// @Param key query string true "object key"
// @Param ttl query int false "ttl (seconds)" default(3600)
// @Param private query bool false "force private url"
// @Success 200 {object} ObjectURLResponse
// @Router /api/storage/url [get]
func (h *Handler) GetObjectURL(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}
	key := strings.TrimLeft(c.Query("key"), "/")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing key"})
		return
	}
	privateParam := strings.ToLower(c.DefaultQuery("private", ""))
	forcePrivate := privateParam == "1" || privateParam == "true" || privateParam == "yes"
	if h.qiniu.Private || forcePrivate {
		ttl, _ := strconv.Atoi(c.DefaultQuery("ttl", "0"))
		url, err := h.qiniu.SignedURL(key, int64(ttl))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		expires := time.Now().Unix() + int64(ttl)
		if ttl <= 0 {
			expires = time.Now().Unix() + int64(h.qiniu.SignTTL)
		}
		c.JSON(http.StatusOK, ObjectURLResponse{URL: url, ExpiresAt: expires})
		return
	}

	c.JSON(http.StatusOK, ObjectURLResponse{URL: h.qiniu.PublicURL(key), ExpiresAt: 0})
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
