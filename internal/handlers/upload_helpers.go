package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"emoji/internal/models"
	"emoji/internal/storage"

	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

var allowedEmojiExt = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".gif":  {},
	".webp": {},
	".avif": {},
	".bmp":  {},
}

func isAllowedEmojiZipFile(name string) bool {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(name)))
	if base == "" {
		return false
	}
	if base == ".ds_store" {
		return false
	}
	if strings.Contains(strings.ToLower(name), "__macosx/") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(base))
	_, ok := allowedEmojiExt[ext]
	return ok
}

func hashFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func deleteQiniuKeys(q *storage.QiniuClient, keys []string) {
	if q == nil || len(keys) == 0 {
		return
	}
	bm := q.BucketManager()
	ops := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, key := range keys {
		clean := strings.TrimSpace(key)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		ops = append(ops, qiniustorage.URIDelete(q.Bucket, clean))
	}
	if len(ops) == 0 {
		return
	}
	_, _ = bm.Batch(ops)
}

func validateCollectionZipIntegrity(tx *gorm.DB, collectionID uint64, expectedFileCount int, sourceZipKey string) error {
	var actualCount int64
	if err := tx.Model(&models.Emoji{}).
		Where("collection_id = ? AND status = ?", collectionID, "active").
		Count(&actualCount).Error; err != nil {
		return err
	}
	if int(actualCount) != expectedFileCount {
		return fmt.Errorf("emoji count mismatch: expected=%d actual=%d", expectedFileCount, actualCount)
	}

	if strings.TrimSpace(sourceZipKey) != "" {
		var zipCount int64
		if err := tx.Model(&models.CollectionZip{}).
			Where("collection_id = ? AND zip_key = ?", collectionID, sourceZipKey).
			Count(&zipCount).Error; err != nil {
			return err
		}
		if zipCount == 0 {
			return fmt.Errorf("zip record missing: %s", sourceZipKey)
		}
	}

	return nil
}
