package handlers

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/storage"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const minValidCollectionZipBytes int64 = 128

// GetVideoJobZipDownload returns a download URL for a job's collection zip.
// If the zip is missing (e.g., after deleting a single output), it will be regenerated.
func (h *Handler) GetVideoJobZipDownload(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var job models.VideoJob
	if err := h.db.Where("id = ? AND user_id = ?", jobID, userID).First(&job).Error; err != nil {
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
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if collection.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	key := strings.TrimSpace(collection.LatestZipKey)
	name := strings.TrimSpace(collection.LatestZipName)
	if shouldRegenerateCollectionZip(collection) {
		generatedKey, generatedName, err := h.regenerateCollectionZip(c, job, collection, normalizeVideoJobAssetDomain(job.AssetDomain))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		key = generatedKey
		name = generatedName
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

	c.JSON(http.StatusOK, DownloadURLResponse{
		CollectionID: collection.ID,
		Key:          key,
		Name:         name,
		URL:          url,
		ExpiresAt:    exp,
	})
}

// GetAdminVideoJobZipDownload returns a download URL for a job's collection zip for admins.
// If the zip is missing (e.g., after deleting a single output), it will be regenerated.
func (h *Handler) GetAdminVideoJobZipDownload(c *gin.Context) {
	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var job models.VideoJob
	if err := h.db.Where("id = ?", jobID).First(&job).Error; err != nil {
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
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	key := strings.TrimSpace(collection.LatestZipKey)
	name := strings.TrimSpace(collection.LatestZipName)
	if shouldRegenerateCollectionZip(collection) {
		generatedKey, generatedName, err := h.regenerateCollectionZip(c, job, collection, normalizeVideoJobAssetDomain(job.AssetDomain))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		key = generatedKey
		name = generatedName
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

	c.JSON(http.StatusOK, DownloadURLResponse{
		CollectionID: collection.ID,
		Key:          key,
		Name:         name,
		URL:          url,
		ExpiresAt:    exp,
	})
}

func (h *Handler) regenerateCollectionZip(ctx context.Context, job models.VideoJob, collection models.Collection, assetDomain string) (string, string, error) {
	if h.qiniu == nil {
		return "", "", errors.New("qiniu not configured")
	}

	emojis, err := h.listVideoJobResultEmojisByDomain(collection.ID, assetDomain)
	if err != nil {
		return "", "", err
	}
	if len(emojis) == 0 {
		return "", "", errors.New("no outputs to zip")
	}

	formatSet := map[string]struct{}{}
	for _, item := range emojis {
		format := strings.TrimSpace(strings.ToLower(item.Format))
		if format != "" {
			if format == "jpeg" {
				format = "jpg"
			}
			formatSet[format] = struct{}{}
		}
	}
	formatLabel := "mixed"
	if len(formatSet) == 1 {
		for key := range formatSet {
			if key != "" {
				formatLabel = key
				break
			}
		}
	}

	prefix := normalizeCollectionPrefix(collection.QiniuPrefix)
	if prefix == "" {
		layout := videojobs.NewVideoImageStorageLayout(h.cfg.Env)
		prefix = layout.JobPrefix(job.UserID, job.ID)
	}
	packageName := fmt.Sprintf("%d_%s_v1.zip", job.ID, formatLabel)
	packageKey := path.Join(strings.TrimSuffix(strings.TrimPrefix(prefix, "/"), "/"), "package", packageName)

	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("job-%d-zip-*", job.ID))
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, fmt.Sprintf("%d_outputs.zip", job.ID))
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", "", err
	}
	zw := zip.NewWriter(zipFile)

	client := &http.Client{Timeout: 10 * time.Minute}
	bm := h.qiniu.BucketManager()
	addedEntries := 0
	lastDownloadErr := ""
	for idx, item := range emojis {
		if reason := videojobs.PackageZipEmojiSkipReason(item); reason != "" {
			lastDownloadErr = reason
			continue
		}

		rawFile := strings.TrimSpace(item.FileURL)
		if rawFile == "" {
			continue
		}
		fileKey := rawFile
		if key, ok := extractQiniuObjectKey(rawFile, h.qiniu); ok {
			fileKey = key
		} else if !(strings.HasPrefix(rawFile, "http://") || strings.HasPrefix(rawFile, "https://")) {
			fileKey = strings.TrimLeft(rawFile, "/")
		}
		if fileKey == "" {
			continue
		}

		cleanForExt := strings.SplitN(fileKey, "?", 2)[0]
		cleanForExt = strings.SplitN(cleanForExt, "#", 2)[0]
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(cleanForExt)), ".")
		if ext == "" {
			ext = strings.TrimSpace(strings.ToLower(item.Format))
		}
		if ext == "jpeg" {
			ext = "jpg"
		}
		if ext == "" {
			ext = "bin"
		}
		base := sanitizeZipEntryComponent(item.Title)
		if base == "" {
			base = fmt.Sprintf("item_%03d", idx+1)
		}
		entryName := fmt.Sprintf("%03d_%s.%s", idx+1, base, ext)

		var (
			content io.ReadCloser
			err     error
		)
		if bm != nil {
			obj, getErr := bm.Get(h.qiniu.Bucket, fileKey, nil)
			if getErr == nil && obj != nil {
				content = obj.Body
			} else if getErr != nil {
				lastDownloadErr = getErr.Error()
			}
		}
		if content == nil {
			url, _ := resolveDownloadURL(fileKey, h.qiniu, "")
			if url == "" {
				lastDownloadErr = fmt.Sprintf("empty download url for key=%s", fileKey)
				continue
			}

			resp, getErr := client.Get(url)
			if getErr != nil {
				lastDownloadErr = getErr.Error()
				continue
			}
			if resp.StatusCode >= 300 {
				lastDownloadErr = fmt.Sprintf("download status=%d key=%s", resp.StatusCode, fileKey)
				resp.Body.Close()
				continue
			}
			content = resp.Body
		}

		entry, err := zw.Create(entryName)
		if err != nil {
			lastDownloadErr = err.Error()
			content.Close()
			continue
		}
		if _, err = io.Copy(entry, content); err != nil {
			lastDownloadErr = err.Error()
			content.Close()
			continue
		}
		content.Close()
		addedEntries++
	}

	if err := zw.Close(); err != nil {
		_ = zipFile.Close()
		return "", "", err
	}
	if err := zipFile.Close(); err != nil {
		return "", "", err
	}
	if addedEntries == 0 {
		if lastDownloadErr == "" {
			lastDownloadErr = "all outputs unavailable"
		}
		return "", "", fmt.Errorf("zip regeneration failed: %s", lastDownloadErr)
	}

	info, err := os.Stat(zipPath)
	if err != nil {
		return "", "", err
	}

	uploader := qiniustorage.NewFormUploader(h.qiniu.Cfg)
	if err := uploadFileToQiniu(uploader, h.qiniu, packageKey, zipPath); err != nil {
		return "", "", err
	}

	now := time.Now()
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		artifact := models.VideoJobArtifact{
			JobID:     job.ID,
			Type:      "package",
			QiniuKey:  packageKey,
			MimeType:  "application/zip",
			SizeBytes: info.Size(),
			Metadata: mustJSONMap(map[string]interface{}{
				"format":      "zip",
				"source":      "on_demand_regen",
				"file_count":  addedEntries,
				"bundle_type": formatLabel,
			}),
		}
		if err := upsertVideoJobArtifact(tx, &artifact); err != nil {
			return err
		}
		if err := videojobs.UpsertPublicVideoImageOutputByArtifact(tx, artifact); err != nil {
			return err
		}

		if normalizeVideoJobAssetDomain(assetDomain) == models.VideoJobAssetDomainVideo {
			zipRecord := models.VideoAssetCollectionZip{
				CollectionID: collection.ID,
				ZipKey:       packageKey,
				ZipName:      packageName,
				SizeBytes:    info.Size(),
				UploadedAt:   &now,
			}
			if err := tx.Where("collection_id = ? AND zip_key = ?", collection.ID, packageKey).
				Assign(models.VideoAssetCollectionZip{
					ZipName:    packageName,
					SizeBytes:  info.Size(),
					UploadedAt: &now,
				}).FirstOrCreate(&zipRecord).Error; err != nil {
				return err
			}

			if err := tx.Model(&models.VideoAssetCollection{}).Where("id = ?", collection.ID).
				Updates(map[string]interface{}{
					"latest_zip_key":  packageKey,
					"latest_zip_name": packageName,
					"latest_zip_size": info.Size(),
					"latest_zip_at":   &now,
				}).Error; err != nil {
				return err
			}
		} else {
			zipRecord := models.CollectionZip{
				CollectionID: collection.ID,
				ZipKey:       packageKey,
				ZipName:      packageName,
				SizeBytes:    info.Size(),
				UploadedAt:   &now,
			}
			if err := tx.Where("collection_id = ? AND zip_key = ?", collection.ID, packageKey).
				Assign(models.CollectionZip{
					ZipName:    packageName,
					SizeBytes:  info.Size(),
					UploadedAt: &now,
				}).FirstOrCreate(&zipRecord).Error; err != nil {
				return err
			}

			if err := tx.Model(&models.Collection{}).Where("id = ?", collection.ID).
				Updates(map[string]interface{}{
					"latest_zip_key":  packageKey,
					"latest_zip_name": packageName,
					"latest_zip_size": info.Size(),
					"latest_zip_at":   &now,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return "", "", err
	}

	return packageKey, packageName, nil
}

func sanitizeZipEntryComponent(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "\\", "-")
	value = strings.ReplaceAll(value, ":", "-")
	value = strings.ReplaceAll(value, "*", "-")
	value = strings.ReplaceAll(value, "?", "-")
	value = strings.ReplaceAll(value, "\"", "-")
	value = strings.ReplaceAll(value, "<", "-")
	value = strings.ReplaceAll(value, ">", "-")
	value = strings.ReplaceAll(value, "|", "-")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 64 {
		value = strings.TrimSpace(value[:64])
	}
	value = strings.Trim(value, ". ")
	if value == "" {
		return ""
	}
	return value
}

func uploadFileToQiniu(uploader *qiniustorage.FormUploader, q *storage.QiniuClient, key, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	putPolicy := qiniustorage.PutPolicy{Scope: q.Bucket + ":" + key}
	upToken := putPolicy.UploadToken(q.Mac)
	var ret qiniustorage.PutRet
	return uploader.Put(context.Background(), &ret, upToken, key, f, info.Size(), &qiniustorage.PutExtra{})
}

func mustJSONMap(payload map[string]interface{}) datatypes.JSON {
	if payload == nil {
		return datatypes.JSON([]byte("{}"))
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(raw)
}

func upsertVideoJobArtifact(tx *gorm.DB, artifact *models.VideoJobArtifact) error {
	if tx == nil || artifact == nil {
		return errors.New("invalid artifact upsert input")
	}
	artifact.QiniuKey = strings.TrimSpace(artifact.QiniuKey)
	if artifact.JobID == 0 || artifact.QiniuKey == "" {
		return tx.Create(artifact).Error
	}

	var existing models.VideoJobArtifact
	err := tx.Where("job_id = ? AND qiniu_key = ?", artifact.JobID, artifact.QiniuKey).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(artifact).Error
		}
		return err
	}

	updates := map[string]interface{}{
		"type":        artifact.Type,
		"mime_type":   artifact.MimeType,
		"size_bytes":  artifact.SizeBytes,
		"width":       artifact.Width,
		"height":      artifact.Height,
		"duration_ms": artifact.DurationMs,
		"metadata":    artifact.Metadata,
	}
	if err := tx.Model(&models.VideoJobArtifact{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
		return err
	}
	artifact.ID = existing.ID
	return nil
}

func shouldRegenerateCollectionZip(collection models.Collection) bool {
	if strings.TrimSpace(collection.LatestZipKey) == "" {
		return true
	}
	if collection.LatestZipSize > 0 && collection.LatestZipSize < minValidCollectionZipBytes {
		return true
	}
	return false
}
