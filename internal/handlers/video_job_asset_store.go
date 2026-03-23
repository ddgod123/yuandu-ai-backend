package handlers

import (
	"errors"
	"strconv"
	"strings"

	"emoji/internal/models"

	"gorm.io/gorm"
)

func normalizeVideoJobAssetDomain(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case models.VideoJobAssetDomainVideo:
		return models.VideoJobAssetDomainVideo
	case models.VideoJobAssetDomainUGC:
		return models.VideoJobAssetDomainUGC
	case models.VideoJobAssetDomainAdmin:
		return models.VideoJobAssetDomainAdmin
	case models.VideoJobAssetDomainArchive:
		return models.VideoJobAssetDomainArchive
	default:
		// 新链路默认归到 video 域；旧链路兼容可显式写 archive。
		return models.VideoJobAssetDomainVideo
	}
}

func videoJobCollectionMapKey(domain string, id uint64) string {
	return normalizeVideoJobAssetDomain(domain) + ":" + strconv.FormatUint(id, 10)
}

func convertVideoAssetCollection(row models.VideoAssetCollection) models.Collection {
	return models.Collection{
		ID:            row.ID,
		Title:         row.Title,
		Slug:          row.Slug,
		Description:   row.Description,
		CoverURL:      row.CoverURL,
		OwnerID:       row.OwnerID,
		Source:        row.Source,
		QiniuPrefix:   row.QiniuPrefix,
		FileCount:     row.FileCount,
		LatestZipKey:  row.LatestZipKey,
		LatestZipName: row.LatestZipName,
		LatestZipSize: row.LatestZipSize,
		LatestZipAt:   row.LatestZipAt,
		Visibility:    row.Visibility,
		Status:        row.Status,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		DeletedAt:     row.DeletedAt,
	}
}

func convertVideoAssetEmoji(row models.VideoAssetEmoji) models.Emoji {
	return models.Emoji{
		ID:           row.ID,
		CollectionID: row.CollectionID,
		Title:        row.Title,
		FileURL:      row.FileURL,
		ThumbURL:     row.ThumbURL,
		Format:       row.Format,
		Width:        row.Width,
		Height:       row.Height,
		SizeBytes:    row.SizeBytes,
		DisplayOrder: row.DisplayOrder,
		Status:       row.Status,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
		DeletedAt:    row.DeletedAt,
	}
}

func convertVideoAssetCollectionZip(row models.VideoAssetCollectionZip) models.CollectionZip {
	return models.CollectionZip{
		ID:           row.ID,
		CollectionID: row.CollectionID,
		ZipKey:       row.ZipKey,
		ZipHash:      row.ZipHash,
		ZipName:      row.ZipName,
		SizeBytes:    row.SizeBytes,
		UploadedAt:   row.UploadedAt,
		CreatedAt:    row.CreatedAt,
	}
}

func (h *Handler) loadVideoJobResultCollectionByDomain(collectionID uint64, assetDomain string) (models.Collection, error) {
	domain := normalizeVideoJobAssetDomain(assetDomain)
	if domain == models.VideoJobAssetDomainVideo {
		var row models.VideoAssetCollection
		if err := h.db.Where("id = ?", collectionID).First(&row).Error; err == nil {
			return convertVideoAssetCollection(row), nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Collection{}, err
		}
		// 兼容旧数据：domain 为空或标记异常时，回退 archive 查询。
		var fallback models.Collection
		if err := h.db.Where("id = ?", collectionID).First(&fallback).Error; err != nil {
			return models.Collection{}, err
		}
		return fallback, nil
	}

	var row models.Collection
	if err := h.db.Where("id = ?", collectionID).First(&row).Error; err == nil {
		return row, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.Collection{}, err
	}
	// archive miss 时兜底 video 域，降低迁移阶段读失败风险。
	var fallback models.VideoAssetCollection
	if err := h.db.Where("id = ?", collectionID).First(&fallback).Error; err != nil {
		return models.Collection{}, err
	}
	return convertVideoAssetCollection(fallback), nil
}

func (h *Handler) loadVideoJobResultCollection(job models.VideoJob) (models.Collection, error) {
	if job.ResultCollectionID == nil || *job.ResultCollectionID == 0 {
		return models.Collection{}, gorm.ErrRecordNotFound
	}
	return h.loadVideoJobResultCollectionByDomain(*job.ResultCollectionID, job.AssetDomain)
}

func (h *Handler) listVideoJobResultEmojisByDomain(collectionID uint64, assetDomain string) ([]models.Emoji, error) {
	domain := normalizeVideoJobAssetDomain(assetDomain)
	if domain == models.VideoJobAssetDomainVideo {
		var rows []models.VideoAssetEmoji
		if err := h.db.Where("collection_id = ? AND status = ?", collectionID, "active").
			Order("display_order ASC, id ASC").
			Find(&rows).Error; err != nil {
			return nil, err
		}
		out := make([]models.Emoji, 0, len(rows))
		for _, row := range rows {
			out = append(out, convertVideoAssetEmoji(row))
		}
		return out, nil
	}

	var rows []models.Emoji
	if err := h.db.Where("collection_id = ? AND status = ?", collectionID, "active").
		Order("display_order ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (h *Handler) loadVideoJobEmojiByDomain(assetDomain string, emojiID uint64, collectionID uint64) (models.Emoji, error) {
	domain := normalizeVideoJobAssetDomain(assetDomain)
	if domain == models.VideoJobAssetDomainVideo {
		var row models.VideoAssetEmoji
		if err := h.db.Where("id = ? AND collection_id = ?", emojiID, collectionID).First(&row).Error; err != nil {
			return models.Emoji{}, err
		}
		return convertVideoAssetEmoji(row), nil
	}

	var row models.Emoji
	if err := h.db.Where("id = ? AND collection_id = ?", emojiID, collectionID).First(&row).Error; err != nil {
		return models.Emoji{}, err
	}
	return row, nil
}

func (h *Handler) loadLatestVideoJobResultZipByDomain(collectionID uint64, assetDomain string) (models.CollectionZip, error) {
	domain := normalizeVideoJobAssetDomain(assetDomain)
	if domain == models.VideoJobAssetDomainVideo {
		var row models.VideoAssetCollectionZip
		if err := h.db.Where("collection_id = ?", collectionID).
			Order("uploaded_at desc nulls last, id desc").
			First(&row).Error; err != nil {
			return models.CollectionZip{}, err
		}
		return convertVideoAssetCollectionZip(row), nil
	}

	var row models.CollectionZip
	if err := h.db.Where("collection_id = ?", collectionID).
		Order("uploaded_at desc nulls last, id desc").
		First(&row).Error; err != nil {
		return models.CollectionZip{}, err
	}
	return row, nil
}
