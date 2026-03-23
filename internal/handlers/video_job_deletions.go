package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type DeleteVideoJobCollectionRequest struct {
	ConfirmName string `json:"confirm_name"`
	Reason      string `json:"reason"`
}

type DeleteVideoJobOutputRequest struct {
	EmojiID   uint64 `json:"emoji_id"`
	Reason    string `json:"reason"`
	RemoveZip *bool  `json:"remove_zip"`
}

type collectionDeleteResult struct {
	CollectionID         uint64   `json:"collection_id"`
	JobIDs               []uint64 `json:"job_ids"`
	DeletedObjects       int      `json:"deleted_objects"`
	StorageDeleteError   string   `json:"storage_delete_error,omitempty"`
	DeletedGIFCandidates int64    `json:"deleted_gif_candidates"`
	DeletedEmojis        int64    `json:"deleted_emojis"`
	DeletedZips          int64    `json:"deleted_zips"`
	DeletedArtifacts     int64    `json:"deleted_artifacts"`
	DeletedPublicOutputs int64    `json:"deleted_public_outputs"`
	DeletedPackages      int64    `json:"deleted_packages"`
}

type outputDeleteResult struct {
	EmojiID              uint64   `json:"emoji_id"`
	CollectionID         uint64   `json:"collection_id"`
	DeletedObjects       int      `json:"deleted_objects"`
	StorageDeleteError   string   `json:"storage_delete_error,omitempty"`
	DeletedArtifacts     int64    `json:"deleted_artifacts"`
	DeletedPublicOutputs int64    `json:"deleted_public_outputs"`
	ZipRemoved           bool     `json:"zip_removed"`
	RemovedZipKeys       []string `json:"removed_zip_keys,omitempty"`
	RemainingCount       int64    `json:"remaining_count"`
}

func (h *Handler) DeleteVideoJobCollection(c *gin.Context) {
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

	var req DeleteVideoJobCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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

	confirm := strings.TrimSpace(req.ConfirmName)
	if confirm == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirm_name_required"})
		return
	}
	if !matchCollectionConfirm(confirm, collection) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "confirm_name_mismatch",
			"confirm_name": confirm,
		})
		return
	}

	res, err := h.hardDeleteCollectionByJob(job, collection, userID, "user", strings.TrimSpace(req.Reason))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "deleted",
		"result":  res,
	})
}

func (h *Handler) DeleteVideoJobOutput(c *gin.Context) {
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

	var req DeleteVideoJobOutputRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.EmojiID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "emoji_id required"})
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

	emoji, err := h.loadVideoJobEmojiByDomain(job.AssetDomain, req.EmojiID, *job.ResultCollectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	collection, err := h.loadVideoJobResultCollectionByDomain(emoji.CollectionID, job.AssetDomain)
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

	removeZip := true
	if req.RemoveZip != nil {
		removeZip = *req.RemoveZip
	}

	res, err := h.hardDeleteEmojiOutputWithDomain(emoji, collection, normalizeVideoJobAssetDomain(job.AssetDomain), []uint64{job.ID}, userID, "user", strings.TrimSpace(req.Reason), removeZip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "deleted",
		"result":  res,
	})
}

func (h *Handler) AdminDeleteVideoJobCollection(c *gin.Context) {
	adminID, _ := currentUserIDFromContext(c)
	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var job models.VideoJob
	if err := h.db.First(&job, jobID).Error; err != nil {
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

	res, err := h.hardDeleteCollectionByJob(job, collection, adminID, "admin", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "deleted",
		"result":  res,
	})
}

func (h *Handler) AdminDeleteVideoJobOutput(c *gin.Context) {
	adminID, _ := currentUserIDFromContext(c)
	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req DeleteVideoJobOutputRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.EmojiID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "emoji_id required"})
		return
	}

	var job models.VideoJob
	if err := h.db.First(&job, jobID).Error; err != nil {
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

	emoji, err := h.loadVideoJobEmojiByDomain(job.AssetDomain, req.EmojiID, *job.ResultCollectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "emoji not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if emoji.CollectionID != *job.ResultCollectionID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "emoji not in job collection"})
		return
	}

	collection, err := h.loadVideoJobResultCollectionByDomain(emoji.CollectionID, job.AssetDomain)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	removeZip := true
	if req.RemoveZip != nil {
		removeZip = *req.RemoveZip
	}

	res, err := h.hardDeleteEmojiOutputWithDomain(emoji, collection, normalizeVideoJobAssetDomain(job.AssetDomain), []uint64{job.ID}, adminID, "admin", strings.TrimSpace(req.Reason), removeZip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "deleted",
		"result":  res,
	})
}

func (h *Handler) hardDeleteCollectionByJob(job models.VideoJob, collection models.Collection, actorID uint64, actorRole, reason string) (collectionDeleteResult, error) {
	jobIDs := []uint64{job.ID}
	return h.hardDeleteCollectionWithDomain(collection, normalizeVideoJobAssetDomain(job.AssetDomain), jobIDs, actorID, actorRole, reason)
}

func (h *Handler) hardDeleteCollection(collection models.Collection, jobIDs []uint64, actorID uint64, actorRole, reason string) (collectionDeleteResult, error) {
	return h.hardDeleteCollectionWithDomain(collection, models.VideoJobAssetDomainArchive, jobIDs, actorID, actorRole, reason)
}

func (h *Handler) hardDeleteCollectionWithDomain(collection models.Collection, assetDomain string, jobIDs []uint64, actorID uint64, actorRole, reason string) (collectionDeleteResult, error) {
	assetDomain = normalizeVideoJobAssetDomain(assetDomain)
	result := collectionDeleteResult{CollectionID: collection.ID}

	prefix := normalizeCollectionPrefix(collection.QiniuPrefix)
	if prefix == "emoji/" {
		return result, errors.New("unsafe qiniu prefix")
	}

	if len(jobIDs) == 0 {
		var ids []uint64
		if err := h.db.Model(&models.VideoJob{}).
			Where("result_collection_id = ? AND LOWER(COALESCE(NULLIF(TRIM(asset_domain), ''), 'video')) = ?", collection.ID, normalizeVideoJobAssetDomain(assetDomain)).
			Pluck("id", &ids).Error; err != nil {
			return result, err
		}
		jobIDs = ids
	}
	result.JobIDs = jobIDs

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if len(jobIDs) > 0 {
			resCandidates := tx.Unscoped().Where("job_id IN ?", jobIDs).Delete(&models.VideoJobGIFCandidate{})
			if resCandidates.Error != nil {
				return resCandidates.Error
			}
			result.DeletedGIFCandidates = resCandidates.RowsAffected

			resArtifacts := tx.Unscoped().Where("job_id IN ?", jobIDs).Delete(&models.VideoJobArtifact{})
			if resArtifacts.Error != nil {
				return resArtifacts.Error
			}
			result.DeletedArtifacts = resArtifacts.RowsAffected

			resOutputs := tx.Table("public.video_image_outputs").Where("job_id IN ?", jobIDs).Delete(nil)
			if resOutputs.Error != nil {
				return resOutputs.Error
			}
			result.DeletedPublicOutputs = resOutputs.RowsAffected

			resPackages := tx.Table("public.video_image_packages").Where("job_id IN ?", jobIDs).Delete(nil)
			if resPackages.Error != nil {
				return resPackages.Error
			}
			result.DeletedPackages = resPackages.RowsAffected

			if err := tx.Model(&models.VideoJob{}).Where("id IN ?", jobIDs).Updates(map[string]interface{}{
				"result_collection_id": nil,
			}).Error; err != nil {
				return err
			}
		}

		if assetDomain == models.VideoJobAssetDomainVideo {
			resEmoji := tx.Unscoped().Where("collection_id = ?", collection.ID).Delete(&models.VideoAssetEmoji{})
			if resEmoji.Error != nil {
				return resEmoji.Error
			}
			result.DeletedEmojis = resEmoji.RowsAffected

			resZip := tx.Unscoped().Where("collection_id = ?", collection.ID).Delete(&models.VideoAssetCollectionZip{})
			if resZip.Error != nil {
				return resZip.Error
			}
			result.DeletedZips = resZip.RowsAffected

			if err := tx.Unscoped().Delete(&models.VideoAssetCollection{}, collection.ID).Error; err != nil {
				return err
			}
		} else {
			resEmoji := tx.Unscoped().Where("collection_id = ?", collection.ID).Delete(&models.Emoji{})
			if resEmoji.Error != nil {
				return resEmoji.Error
			}
			result.DeletedEmojis = resEmoji.RowsAffected

			resZip := tx.Unscoped().Where("collection_id = ?", collection.ID).Delete(&models.CollectionZip{})
			if resZip.Error != nil {
				return resZip.Error
			}
			result.DeletedZips = resZip.RowsAffected

			if err := tx.Unscoped().Delete(&models.Collection{}, collection.ID).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return result, err
	}
	if prefix != "" {
		if h.qiniu == nil {
			result.StorageDeleteError = "qiniu not configured"
		} else {
			deletedObjects, err := h.deleteQiniuPrefixObjects(prefix)
			result.DeletedObjects = deletedObjects
			if err != nil {
				result.StorageDeleteError = fmt.Sprintf("delete qiniu objects: %v", err)
			}
		}
	}

	h.recordAuditLog(actorID, "collection", collection.ID, fmt.Sprintf("%s_delete_collection", actorRole), map[string]interface{}{
		"actor_id":               actorID,
		"actor_role":             actorRole,
		"reason":                 reason,
		"collection_id":          collection.ID,
		"collection_title":       collection.Title,
		"asset_domain":           assetDomain,
		"qiniu_prefix":           prefix,
		"job_ids":                jobIDs,
		"deleted_objects":        result.DeletedObjects,
		"storage_delete_error":   result.StorageDeleteError,
		"deleted_gif_candidates": result.DeletedGIFCandidates,
		"deleted_emojis":         result.DeletedEmojis,
		"deleted_zips":           result.DeletedZips,
	})

	return result, nil
}

func (h *Handler) hardDeleteEmojiOutput(
	emoji models.Emoji,
	collection models.Collection,
	jobIDs []uint64,
	actorID uint64,
	actorRole, reason string,
	removeZip bool,
) (outputDeleteResult, error) {
	return h.hardDeleteEmojiOutputWithDomain(emoji, collection, models.VideoJobAssetDomainArchive, jobIDs, actorID, actorRole, reason, removeZip)
}

func (h *Handler) hardDeleteEmojiOutputWithDomain(
	emoji models.Emoji,
	collection models.Collection,
	assetDomain string,
	jobIDs []uint64,
	actorID uint64,
	actorRole, reason string,
	removeZip bool,
) (outputDeleteResult, error) {
	assetDomain = normalizeVideoJobAssetDomain(assetDomain)
	result := outputDeleteResult{
		EmojiID:      emoji.ID,
		CollectionID: collection.ID,
	}

	fileKey := strings.TrimLeft(strings.TrimSpace(emoji.FileURL), "/")
	thumbKey := strings.TrimLeft(strings.TrimSpace(emoji.ThumbURL), "/")
	deleteKeys := uniqueKeys([]string{fileKey, thumbKey})

	removedZipKeys := []string{}
	if removeZip {
		zipKeys, err := h.collectCollectionZipKeys(collection.ID, collection, assetDomain)
		if err != nil {
			return result, err
		}
		removedZipKeys = zipKeys
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if assetDomain == models.VideoJobAssetDomainVideo {
			resEmoji := tx.Unscoped().Delete(&models.VideoAssetEmoji{}, emoji.ID)
			if resEmoji.Error != nil {
				return resEmoji.Error
			}
		} else {
			resEmoji := tx.Unscoped().Delete(&models.Emoji{}, emoji.ID)
			if resEmoji.Error != nil {
				return resEmoji.Error
			}
		}

		if len(deleteKeys) > 0 {
			resArtifact := tx.Unscoped().Where("qiniu_key IN ?", deleteKeys).Delete(&models.VideoJobArtifact{})
			if resArtifact.Error != nil {
				return resArtifact.Error
			}
			result.DeletedArtifacts += resArtifact.RowsAffected

			resPublic := tx.Table("public.video_image_outputs").Where("object_key IN ?", deleteKeys).Delete(nil)
			if resPublic.Error != nil {
				return resPublic.Error
			}
			result.DeletedPublicOutputs += resPublic.RowsAffected
		}

		if removeZip {
			if len(jobIDs) == 0 {
				var ids []uint64
				if err := tx.Model(&models.VideoJob{}).
					Where("result_collection_id = ? AND LOWER(COALESCE(NULLIF(TRIM(asset_domain), ''), 'video')) = ?", collection.ID, normalizeVideoJobAssetDomain(assetDomain)).
					Pluck("id", &ids).Error; err != nil {
					return err
				}
				jobIDs = ids
			}
			if len(jobIDs) > 0 {
				if err := tx.Unscoped().Where("job_id IN ? AND (type = ? OR qiniu_key LIKE '%.zip')", jobIDs, "package").Delete(&models.VideoJobArtifact{}).Error; err != nil {
					return err
				}
				if err := tx.Table("public.video_image_outputs").Where("job_id IN ? AND (file_role = ? OR format = 'zip')", jobIDs, "package").Delete(nil).Error; err != nil {
					return err
				}
				if err := tx.Table("public.video_image_packages").Where("job_id IN ?", jobIDs).Delete(nil).Error; err != nil {
					return err
				}
			}

			if assetDomain == models.VideoJobAssetDomainVideo {
				if err := tx.Unscoped().Where("collection_id = ?", collection.ID).Delete(&models.VideoAssetCollectionZip{}).Error; err != nil {
					return err
				}
				if err := tx.Model(&models.VideoAssetCollection{}).Where("id = ?", collection.ID).Updates(map[string]interface{}{
					"latest_zip_key":  "",
					"latest_zip_name": "",
					"latest_zip_size": 0,
					"latest_zip_at":   nil,
				}).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Unscoped().Where("collection_id = ?", collection.ID).Delete(&models.CollectionZip{}).Error; err != nil {
					return err
				}
				if err := tx.Model(&models.Collection{}).Where("id = ?", collection.ID).Updates(map[string]interface{}{
					"latest_zip_key":  "",
					"latest_zip_name": "",
					"latest_zip_size": 0,
					"latest_zip_at":   nil,
				}).Error; err != nil {
					return err
				}
			}
			result.ZipRemoved = true
		}

		var remaining int64
		if assetDomain == models.VideoJobAssetDomainVideo {
			if err := tx.Model(&models.VideoAssetEmoji{}).
				Where("collection_id = ? AND status = ?", collection.ID, "active").
				Count(&remaining).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&models.Emoji{}).
				Where("collection_id = ? AND status = ?", collection.ID, "active").
				Count(&remaining).Error; err != nil {
				return err
			}
		}
		result.RemainingCount = remaining

		updates := map[string]interface{}{
			"file_count": remaining,
		}
		if strings.TrimSpace(collection.CoverURL) == fileKey || strings.TrimSpace(collection.CoverURL) == thumbKey {
			var nextEmoji models.Emoji
			var err error
			if assetDomain == models.VideoJobAssetDomainVideo {
				var nextVideoEmoji models.VideoAssetEmoji
				err = tx.Where("collection_id = ? AND status = ?", collection.ID, "active").Order("display_order ASC, id ASC").First(&nextVideoEmoji).Error
				if err == nil {
					nextEmoji = convertVideoAssetEmoji(nextVideoEmoji)
				}
			} else {
				err = tx.Where("collection_id = ? AND status = ?", collection.ID, "active").Order("display_order ASC, id ASC").First(&nextEmoji).Error
			}
			if err == nil {
				updates["cover_url"] = nextEmoji.FileURL
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				updates["cover_url"] = ""
			} else {
				return err
			}
		}

		if assetDomain == models.VideoJobAssetDomainVideo {
			if err := tx.Model(&models.VideoAssetCollection{}).Where("id = ?", collection.ID).Updates(updates).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&models.Collection{}).Where("id = ?", collection.ID).Updates(updates).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return result, err
	}

	storageKeys := uniqueKeys(append(append([]string{}, deleteKeys...), removedZipKeys...))
	if len(storageKeys) > 0 {
		if h.qiniu == nil {
			result.StorageDeleteError = "qiniu not configured"
		} else {
			count, err := h.deleteQiniuKeysStrict(storageKeys)
			result.DeletedObjects += count
			if err != nil {
				result.StorageDeleteError = err.Error()
			}
		}
	}

	result.RemovedZipKeys = removedZipKeys

	h.recordAuditLog(actorID, "emoji", emoji.ID, fmt.Sprintf("%s_delete_emoji", actorRole), map[string]interface{}{
		"actor_id":             actorID,
		"actor_role":           actorRole,
		"reason":               reason,
		"collection_id":        collection.ID,
		"asset_domain":         assetDomain,
		"emoji_id":             emoji.ID,
		"file_key":             fileKey,
		"thumb_key":            thumbKey,
		"remove_zip":           removeZip,
		"zip_keys":             removedZipKeys,
		"deleted_objects":      result.DeletedObjects,
		"storage_delete_error": result.StorageDeleteError,
		"remaining":            result.RemainingCount,
	})

	return result, nil
}

func (h *Handler) deleteQiniuKeysStrict(keys []string) (int, error) {
	if h.qiniu == nil {
		return 0, errors.New("qiniu not configured")
	}
	cleanKeys := uniqueKeys(keys)
	if len(cleanKeys) == 0 {
		return 0, nil
	}
	bm := h.qiniu.BucketManager()
	ops := make([]string, 0, len(cleanKeys))
	for _, key := range cleanKeys {
		ops = append(ops, qiniustorage.URIDelete(h.qiniu.Bucket, key))
	}
	if len(ops) == 0 {
		return 0, nil
	}
	ret, err := bm.Batch(ops)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, item := range ret {
		if item.Code == 200 || item.Code == 612 {
			deleted++
			continue
		}
		return deleted, fmt.Errorf("batch delete failed with code=%d", item.Code)
	}
	return deleted, nil
}

func (h *Handler) collectCollectionZipKeys(collectionID uint64, collection models.Collection, assetDomain string) ([]string, error) {
	assetDomain = normalizeVideoJobAssetDomain(assetDomain)
	keys := []string{}
	if strings.TrimSpace(collection.LatestZipKey) != "" {
		keys = append(keys, strings.TrimLeft(strings.TrimSpace(collection.LatestZipKey), "/"))
	}
	if assetDomain == models.VideoJobAssetDomainVideo {
		var rows []models.VideoAssetCollectionZip
		if err := h.db.Select("zip_key").Where("collection_id = ?", collectionID).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			key := strings.TrimLeft(strings.TrimSpace(row.ZipKey), "/")
			if key != "" {
				keys = append(keys, key)
			}
		}
	} else {
		var rows []models.CollectionZip
		if err := h.db.Select("zip_key").Where("collection_id = ?", collectionID).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			key := strings.TrimLeft(strings.TrimSpace(row.ZipKey), "/")
			if key != "" {
				keys = append(keys, key)
			}
		}
	}
	return uniqueKeys(keys), nil
}

func matchCollectionConfirm(input string, collection models.Collection) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}
	if strings.TrimSpace(collection.Title) != "" && input == strings.TrimSpace(collection.Title) {
		return true
	}
	if strings.TrimSpace(collection.LatestZipName) != "" && input == strings.TrimSpace(collection.LatestZipName) {
		return true
	}
	return false
}

func uniqueKeys(keys []string) []string {
	set := make(map[string]struct{})
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		clean := strings.TrimLeft(strings.TrimSpace(key), "/")
		if clean == "" {
			continue
		}
		if _, exists := set[clean]; exists {
			continue
		}
		set[clean] = struct{}{}
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}

func (h *Handler) recordAuditLog(adminID uint64, targetType string, targetID uint64, action string, meta map[string]interface{}) {
	if h == nil || h.db == nil {
		return
	}
	payload := datatypes.JSON([]byte("{}"))
	if meta != nil {
		if encoded, err := json.Marshal(meta); err == nil {
			payload = datatypes.JSON(encoded)
		}
	}
	log := models.AuditLog{
		AdminID:    adminID,
		TargetType: strings.TrimSpace(targetType),
		TargetID:   targetID,
		Action:     strings.TrimSpace(action),
		Meta:       payload,
		CreatedAt:  time.Now(),
	}
	_ = h.db.Create(&log).Error
}
