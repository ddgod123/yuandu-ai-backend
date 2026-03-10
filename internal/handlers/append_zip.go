package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/storage"

	"github.com/gin-gonic/gin"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

type AppendZipResponse struct {
	CollectionID uint64 `json:"collection_id"`
	Added        int    `json:"added"`
	FileCount    int    `json:"file_count"`
	Prefix       string `json:"prefix"`
	CoverKey     string `json:"cover_key,omitempty"`
	SourceZipKey string `json:"source_zip_key"`
}

// AppendCollectionZip godoc
// @Summary Append zip to existing collection
// @Tags admin
// @Accept multipart/form-data
// @Produce json
// @Param id path int true "collection id"
// @Param file formData file true "zip file"
// @Param set_cover formData bool false "set first file as cover"
// @Success 200 {object} AppendZipResponse
// @Router /api/admin/collections/{id}/import-zip [post]
func (h *Handler) AppendCollectionZip(c *gin.Context) {
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

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	if !strings.HasSuffix(strings.ToLower(fileHeader.Filename), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only .zip supported"})
		return
	}

	setCover := strings.ToLower(strings.TrimSpace(c.PostForm("set_cover")))
	forceCover := setCover == "1" || setCover == "true" || setCover == "yes"

	userID := collection.OwnerID
	if uidAny, ok := c.Get("user_id"); ok {
		if parsed, ok := uidAny.(uint64); ok && parsed > 0 {
			userID = parsed
		}
	}

	taskCollectionID := collection.ID
	taskID := h.createUploadTask(
		"append",
		userID,
		&taskCollectionID,
		collection.CategoryID,
		fileHeader.Filename,
		fileHeader.Size,
		map[string]interface{}{
			"collection_id": collection.ID,
			"set_cover":     forceCover,
		},
	)
	uploadedKeys := make([]string, 0, 256)
	failed := false
	fail := func(status int, message string) {
		if !failed {
			deleteQiniuKeys(h.qiniu, uploadedKeys)
			h.finishUploadTask(taskID, false, message, nil)
			failed = true
		}
		c.JSON(status, gin.H{"error": message})
	}

	tmpFile, err := os.CreateTemp("", "emoji-append-*.zip")
	if err != nil {
		fail(http.StatusInternalServerError, "temp file failed")
		return
	}
	defer os.Remove(tmpFile.Name())

	src, err := fileHeader.Open()
	if err != nil {
		fail(http.StatusBadRequest, "open file failed")
		return
	}
	defer src.Close()
	if _, err := io.Copy(tmpFile, src); err != nil {
		fail(http.StatusInternalServerError, "save file failed")
		return
	}
	if err := tmpFile.Close(); err != nil {
		fail(http.StatusInternalServerError, "save file failed")
		return
	}
	zipHash, err := hashFileSHA256(tmpFile.Name())
	if err != nil {
		fail(http.StatusInternalServerError, "hash file failed")
		return
	}
	if strings.TrimSpace(zipHash) != "" {
		var dupCount int64
		if err := h.db.Model(&models.CollectionZip{}).
			Where("collection_id = ? AND zip_hash = ?", collection.ID, zipHash).
			Count(&dupCount).Error; err == nil && dupCount > 0 {
			fail(http.StatusConflict, "zip already uploaded")
			return
		}
	}

	reader, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		fail(http.StatusBadRequest, "invalid zip")
		return
	}
	defer reader.Close()

	zipFiles := make([]*zip.File, 0)
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !isAllowedEmojiZipFile(f.Name) {
			continue
		}
		zipFiles = append(zipFiles, f)
	}
	if len(zipFiles) == 0 {
		fail(http.StatusBadRequest, "zip has no supported image files")
		return
	}
	sort.Slice(zipFiles, func(i, j int) bool {
		return zipFiles[i].Name < zipFiles[j].Name
	})

	prefix := strings.TrimSpace(collection.QiniuPrefix)
	if prefix == "" {
		prefix = "emoji/collections/"
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	rawPrefix := prefix + "raw/"

	uploader := qiniustorage.NewFormUploader(h.qiniu.Cfg)

	sourceZipKey := fmt.Sprintf("%ssource/part-%d.zip", prefix, time.Now().Unix())
	if err := uploadToQiniu(uploader, h.qiniu, sourceZipKey, tmpFile.Name()); err != nil {
		fail(http.StatusBadRequest, err.Error())
		return
	}
	uploadedKeys = append(uploadedKeys, sourceZipKey)
	zipUploadedAt := time.Now()
	h.updateUploadTaskStage(taskID, "processing")

	var existingCount int64
	_ = h.db.Model(&models.Emoji{}).Where("collection_id = ?", collection.ID).Count(&existingCount).Error
	maxSeq, seqWidth := maxEmojiSequence(h.db, collection.ID)
	if seqWidth <= 0 {
		seqWidth = 4
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		fail(http.StatusInternalServerError, "db error")
		return
	}
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			h.finishUploadTask(taskID, false, fmt.Sprintf("panic: %v", r), nil)
			panic(r)
		}
	}()

	coverKey := ""
	added := 0
	fileEntries := make([]map[string]interface{}, 0, len(zipFiles))

	for idx, f := range zipFiles {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext == "" {
			ext = ".gif"
		}
		seq := maxSeq + idx + 1
		destName := fmt.Sprintf("%0*d%s", seqWidth, seq, ext)
		destKey := rawPrefix + destName

		rc, err := f.Open()
		if err != nil {
			_ = tx.Rollback()
			fail(http.StatusBadRequest, "zip file read failed")
			return
		}
		buf, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			_ = tx.Rollback()
			fail(http.StatusBadRequest, "zip file read failed")
			return
		}
		if err := uploadReaderToQiniu(uploader, h.qiniu, destKey, bytes.NewReader(buf), int64(len(buf))); err != nil {
			_ = tx.Rollback()
			fail(http.StatusBadRequest, err.Error())
			return
		}
		uploadedKeys = append(uploadedKeys, destKey)
		thumbKey := ""
		if ext == ".gif" {
			thumbKey = tryUploadListPreviewGIF(uploader, h.qiniu, destKey, buf)
			if thumbKey != "" {
				uploadedKeys = append(uploadedKeys, thumbKey)
			}
		}

		if coverKey == "" {
			coverKey = destKey
		}
		title := strings.TrimSuffix(filepath.Base(f.Name), ext)
		if title == "" {
			title = destName
		}
		emoji := models.Emoji{
			CollectionID: collection.ID,
			Title:        title,
			FileURL:      destKey,
			ThumbURL:     thumbKey,
			Format:       strings.TrimPrefix(ext, "."),
			SizeBytes:    int64(len(buf)),
			DisplayOrder: seq,
			Status:       "active",
		}
		if err := tx.Create(&emoji).Error; err != nil {
			_ = tx.Rollback()
			fail(http.StatusBadRequest, err.Error())
			return
		}
		added++
		fileEntries = append(fileEntries, map[string]interface{}{
			"original": f.Name,
			"name":     destName,
			"key":      destKey,
			"thumb":    thumbKey,
			"size":     len(buf),
		})
	}

	collection.FileCount = int(existingCount) + added
	if (forceCover || collection.CoverURL == "") && coverKey != "" {
		collection.CoverURL = coverKey
	}
	collection.LatestZipKey = sourceZipKey
	collection.LatestZipName = fileHeader.Filename
	collection.LatestZipSize = fileHeader.Size
	collection.LatestZipAt = &zipUploadedAt

	if err := updateCollectionMetaJSON(h.qiniu, prefix, collection, fileEntries, sourceZipKey, fileHeader.Filename, zipHash); err != nil {
		_ = tx.Rollback()
		fail(http.StatusBadRequest, err.Error())
		return
	}

	if err := tx.Save(&collection).Error; err != nil {
		_ = tx.Rollback()
		fail(http.StatusBadRequest, err.Error())
		return
	}

	zipRecord := models.CollectionZip{
		CollectionID: collection.ID,
		ZipKey:       sourceZipKey,
		ZipHash:      zipHash,
		ZipName:      fileHeader.Filename,
		SizeBytes:    fileHeader.Size,
		UploadedAt:   &zipUploadedAt,
	}
	if err := tx.Where("collection_id = ? AND zip_key = ?", collection.ID, sourceZipKey).
		FirstOrCreate(&zipRecord).Error; err != nil {
		_ = tx.Rollback()
		fail(http.StatusBadRequest, err.Error())
		return
	}

	if err := validateCollectionZipIntegrity(tx, collection.ID, int(existingCount)+added, sourceZipKey); err != nil {
		_ = tx.Rollback()
		fail(http.StatusBadRequest, err.Error())
		return
	}

	if err := tx.Commit().Error; err != nil {
		fail(http.StatusInternalServerError, err.Error())
		return
	}

	h.finishUploadTask(taskID, true, "", map[string]interface{}{
		"collection_id":  collection.ID,
		"title":          collection.Title,
		"added":          added,
		"file_count":     collection.FileCount,
		"prefix":         prefix,
		"cover_key":      collection.CoverURL,
		"source_zip_key": sourceZipKey,
		"zip_hash":       zipHash,
	})

	c.JSON(http.StatusOK, AppendZipResponse{
		CollectionID: collection.ID,
		Added:        added,
		FileCount:    collection.FileCount,
		Prefix:       prefix,
		CoverKey:     collection.CoverURL,
		SourceZipKey: sourceZipKey,
	})
}

func updateCollectionMetaJSON(q *storage.QiniuClient, prefix string, collection models.Collection, entries []map[string]interface{}, sourceZipKey, sourceName, sourceHash string) error {
	metaKey := prefix + "meta.json"
	meta, _ := fetchMetaJSON(q, metaKey)
	if meta == nil {
		meta = map[string]interface{}{
			"collection": map[string]interface{}{
				"title":     collection.Title,
				"slug":      collection.Slug,
				"source":    collection.Source,
				"zip_name":  sourceName,
				"prefix":    prefix,
				"createdAt": time.Now().Format(time.RFC3339),
			},
			"files": []interface{}{},
		}
	}

	filesVal, _ := meta["files"].([]interface{})
	for _, entry := range entries {
		filesVal = append(filesVal, entry)
	}
	meta["files"] = filesVal
	fileCount := len(filesVal)

	if colVal, ok := meta["collection"].(map[string]interface{}); ok {
		colVal["updatedAt"] = time.Now().Format(time.RFC3339)
		colVal["fileCount"] = fileCount
		colVal["totalFiles"] = fileCount
		meta["collection"] = colVal
	}

	zipParts := []interface{}{}
	if existing, ok := meta["zips"].([]interface{}); ok {
		zipParts = append(zipParts, existing...)
	}
	zipParts = append(zipParts, map[string]interface{}{
		"key":     sourceZipKey,
		"name":    sourceName,
		"hash":    strings.TrimSpace(sourceHash),
		"addedAt": time.Now().Format(time.RFC3339),
	})
	meta["zips"] = zipParts

	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	uploader := qiniustorage.NewFormUploader(q.Cfg)
	return uploadReaderToQiniu(uploader, q, metaKey, bytes.NewReader(metaJSON), int64(len(metaJSON)))
}

func fetchMetaJSON(q *storage.QiniuClient, key string) (map[string]interface{}, error) {
	url := q.PublicURL(key)
	if q.Private {
		signed, err := q.SignedURL(key, 600)
		if err == nil {
			url = signed
		}
	}
	if !strings.HasPrefix(url, "http") {
		return nil, fmt.Errorf("invalid meta url")
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("meta fetch failed: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, err
	}
	return meta, nil
}
