package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
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

type ImportZipResponse struct {
	CollectionID uint64 `json:"collection_id"`
	Title        string `json:"title"`
	Slug         string `json:"slug"`
	Prefix       string `json:"prefix"`
	FileCount    int    `json:"file_count"`
	CoverKey     string `json:"cover_key"`
	SourceZipKey string `json:"source_zip_key"`
}

// ImportCollectionZip godoc
// @Summary Import collection zip
// @Tags admin
// @Accept multipart/form-data
// @Produce json
// @Param title formData string true "collection title"
// @Param description formData string false "collection description"
// @Param category_id formData int true "category id"
// @Param tag_ids formData string false "comma-separated tag ids"
// @Param file formData file true "zip file"
// @Success 200 {object} ImportZipResponse
// @Router /api/admin/collections/import-zip [post]
func (h *Handler) ImportCollectionZip(c *gin.Context) {
	if h.qiniu == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "qiniu not configured"})
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title required"})
		return
	}
	description := strings.TrimSpace(c.PostForm("description"))
	categoryIDStr := strings.TrimSpace(c.PostForm("category_id"))
	if categoryIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category_id required"})
		return
	}
	categoryID, err := strconv.ParseUint(categoryIDStr, 10, 64)
	if err != nil || categoryID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category_id"})
		return
	}

	var category models.Category
	if err := h.db.First(&category, categoryID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusBadRequest, gin.H{"error": "category not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.requireLeafCategory(categoryID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "category not found"})
			return
		}
		if errors.Is(err, errCategoryHasChildren) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "category has children"})
			return
		}
		if errors.Is(err, errInvalidCategory) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	uidVal, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing user"})
		return
	}
	ownerID, _ := uidVal.(uint64)
	if ownerID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user"})
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

	tagIDs := parseIDList(c.PostForm("tag_ids"))

	taskCategoryID := categoryID
	taskID := h.createUploadTask(
		"import",
		ownerID,
		nil,
		&taskCategoryID,
		fileHeader.Filename,
		fileHeader.Size,
		map[string]interface{}{
			"title":       title,
			"description": description,
			"category_id": categoryID,
			"tag_ids":     tagIDs,
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

	collectionSlug := slugifyCollection(title)
	if collectionSlug == "" {
		collectionSlug = fmt.Sprintf("collection-%d", time.Now().Unix())
	}
	collectionSlug = ensureUniqueCollectionSlug(h.db, collectionSlug)

	categoryPrefix := strings.TrimSpace(category.Prefix)
	if categoryPrefix == "" {
		categoryPrefix = h.qiniuCollectionsPrefix()
	}
	if !strings.HasSuffix(categoryPrefix, "/") {
		categoryPrefix += "/"
	}
	prefix := path.Join(strings.TrimSuffix(categoryPrefix, "/"), collectionSlug) + "/"

	tmpFile, err := os.CreateTemp("", "emoji-import-*.zip")
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

	uploader := qiniustorage.NewFormUploader(h.qiniu.Cfg)

	// upload source zip
	sourceZipKey := fmt.Sprintf("%ssource/part-%d.zip", prefix, time.Now().Unix())
	if err := uploadToQiniu(uploader, h.qiniu, sourceZipKey, tmpFile.Name()); err != nil {
		fail(http.StatusBadRequest, err.Error())
		return
	}
	uploadedKeys = append(uploadedKeys, sourceZipKey)
	zipUploadedAt := time.Now()
	h.updateUploadTaskStage(taskID, "processing")

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

	collection := models.Collection{
		Title:         title,
		Slug:          collectionSlug,
		Description:   description,
		OwnerID:       ownerID,
		CategoryID:    &categoryID,
		Source:        "manual_zip",
		QiniuPrefix:   prefix,
		Status:        "active",
		Visibility:    "public",
		LatestZipKey:  sourceZipKey,
		LatestZipName: fileHeader.Filename,
		LatestZipSize: fileHeader.Size,
		LatestZipAt:   &zipUploadedAt,
	}
	if err := ensureCreatorProfileID(tx, &collection); err != nil {
		_ = tx.Rollback()
		fail(http.StatusInternalServerError, "failed to assign creator profile")
		return
	}
	code, err := ensureCollectionDownloadCode(tx, collection.DownloadCode)
	if err != nil {
		_ = tx.Rollback()
		fail(http.StatusInternalServerError, "failed to generate download code")
		return
	}
	collection.DownloadCode = code
	if err := tx.Create(&collection).Error; err != nil {
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

	meta := map[string]interface{}{
		"collection": map[string]interface{}{
			"title":     title,
			"slug":      collectionSlug,
			"category":  category.Slug,
			"source":    "manual_zip",
			"zip_name":  fileHeader.Filename,
			"prefix":    prefix,
			"createdAt": time.Now().Format(time.RFC3339),
		},
		"files": []map[string]interface{}{},
		"zips": []map[string]interface{}{
			{
				"key":     sourceZipKey,
				"name":    fileHeader.Filename,
				"hash":    zipHash,
				"addedAt": time.Now().Format(time.RFC3339),
			},
		},
	}
	fileEntries := make([]map[string]interface{}, 0, len(zipFiles))
	coverKey := ""
	emojiCount := 0

	for idx, f := range zipFiles {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext == "" {
			ext = ".gif"
		}
		destName := fmt.Sprintf("%04d%s", idx+1, ext)
		destKey := prefix + "raw/" + destName

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
		emoji := models.Emoji{
			CollectionID: collection.ID,
			Title:        strings.TrimSuffix(filepath.Base(f.Name), ext),
			FileURL:      destKey,
			ThumbURL:     thumbKey,
			Format:       strings.TrimPrefix(ext, "."),
			SizeBytes:    int64(len(buf)),
			DisplayOrder: idx + 1,
			Status:       "active",
		}
		if err := tx.Create(&emoji).Error; err != nil {
			_ = tx.Rollback()
			fail(http.StatusBadRequest, err.Error())
			return
		}
		emojiCount++

		fileEntries = append(fileEntries, map[string]interface{}{
			"original": f.Name,
			"name":     destName,
			"key":      destKey,
			"thumb":    thumbKey,
			"size":     len(buf),
		})
	}

	meta["files"] = fileEntries
	if colVal, ok := meta["collection"].(map[string]interface{}); ok {
		colVal["fileCount"] = len(fileEntries)
		colVal["totalFiles"] = len(fileEntries)
		meta["collection"] = colVal
	}
	metaJSON, _ := json.MarshalIndent(meta, "", "  ")
	if err := uploadReaderToQiniu(uploader, h.qiniu, prefix+"meta.json", bytes.NewReader(metaJSON), int64(len(metaJSON))); err != nil {
		_ = tx.Rollback()
		fail(http.StatusBadRequest, err.Error())
		return
	}
	uploadedKeys = append(uploadedKeys, prefix+"meta.json")

	collection.CoverURL = coverKey
	collection.FileCount = emojiCount
	collection.LatestZipKey = sourceZipKey
	collection.LatestZipName = fileHeader.Filename
	collection.LatestZipSize = fileHeader.Size
	collection.LatestZipAt = &zipUploadedAt
	if err := tx.Save(&collection).Error; err != nil {
		_ = tx.Rollback()
		fail(http.StatusBadRequest, err.Error())
		return
	}

	if len(tagIDs) > 0 {
		for _, tagID := range tagIDs {
			ct := models.CollectionTag{
				CollectionID: collection.ID,
				TagID:        tagID,
			}
			if err := tx.Create(&ct).Error; err != nil {
				_ = tx.Rollback()
				fail(http.StatusBadRequest, err.Error())
				return
			}
		}
	}

	if err := validateCollectionZipIntegrity(tx, collection.ID, emojiCount, sourceZipKey); err != nil {
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
		"prefix":         prefix,
		"file_count":     emojiCount,
		"cover_key":      coverKey,
		"source_zip_key": sourceZipKey,
		"zip_hash":       zipHash,
	})

	c.JSON(http.StatusOK, ImportZipResponse{
		CollectionID: collection.ID,
		Title:        collection.Title,
		Slug:         collection.Slug,
		Prefix:       prefix,
		FileCount:    emojiCount,
		CoverKey:     coverKey,
		SourceZipKey: sourceZipKey,
	})
}

func parseIDList(raw string) []uint64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]uint64, 0, len(parts))
	seen := map[uint64]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseUint(p, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func slugifyCollection(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == '_' || r == ' ' {
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func ensureUniqueCollectionSlug(db *gorm.DB, slug string) string {
	base := slug
	for i := 1; i < 100; i++ {
		var count int64
		_ = db.Model(&models.Collection{}).Where("slug = ?", slug).Count(&count).Error
		if count == 0 {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

func uploadToQiniu(uploader *qiniustorage.FormUploader, q *storage.QiniuClient, key, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file failed: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat file failed: %w", err)
	}
	return uploadReaderToQiniu(uploader, q, key, file, info.Size())
}

func uploadReaderToQiniu(uploader *qiniustorage.FormUploader, q *storage.QiniuClient, key string, reader io.Reader, size int64) error {
	putPolicy := qiniustorage.PutPolicy{
		Scope: q.Bucket + ":" + key,
	}
	upToken := putPolicy.UploadToken(q.Mac)
	var ret qiniustorage.PutRet
	return uploader.Put(context.Background(), &ret, upToken, key, reader, size, &qiniustorage.PutExtra{})
}
