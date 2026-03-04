package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/storage"

	"github.com/joho/godotenv"
	"github.com/qiniu/go-sdk/v7/client"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
)

func main() {
	_ = godotenv.Overload()
	_ = godotenv.Overload("backend/.env")

	cfg := config.Load()
	q, err := storage.NewQiniuClient(cfg)
	if err != nil {
		log.Fatalf("qiniu config error: %v", err)
	}

	bm := q.BucketManager()
	prefix := "emoji/"
	marker := ""
	migrated := 0
	failed := 0

	for {
		entries, _, nextMarker, hasNext, err := bm.ListFiles(q.Bucket, prefix, marker, "", 1000)
		if err != nil {
			log.Fatalf("list files failed: %v", err)
		}

		for _, entry := range entries {
			key := entry.Key
			if !strings.HasSuffix(key, "/source.zip") {
				continue
			}
			if err := migrateSourceZip(bm, q, key); err != nil {
				failed++
				log.Printf("migrate failed: %s (%v)", key, err)
				continue
			}
			migrated++
		}

		if !hasNext {
			break
		}
		marker = nextMarker
	}

	log.Printf("done: migrated=%d failed=%d", migrated, failed)
}

func migrateSourceZip(bm *qiniustorage.BucketManager, q *storage.QiniuClient, key string) error {
	bucket := q.Bucket
	prefix := strings.TrimSuffix(key, "source.zip")
	destKey := fmt.Sprintf("%ssource/part-%d.zip", prefix, time.Now().UnixNano())

	if err := bm.Copy(bucket, key, bucket, destKey, true); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	if err := updateMetaJSON(bm, q, prefix, destKey, key); err != nil {
		return fmt.Errorf("meta update failed: %w", err)
	}

	if err := bm.Delete(bucket, key); err != nil {
		return fmt.Errorf("delete old key failed: %w", err)
	}

	log.Printf("migrated %s -> %s", key, destKey)
	return nil
}

func updateMetaJSON(bm *qiniustorage.BucketManager, q *storage.QiniuClient, prefix, destKey, oldKey string) error {
	metaKey := prefix + "meta.json"
	meta, err := fetchMetaJSON(bm, q.Bucket, metaKey)
	if err != nil {
		return err
	}
	if meta == nil {
		meta = map[string]interface{}{}
	}

	colVal, _ := meta["collection"].(map[string]interface{})
	if colVal == nil {
		colVal = map[string]interface{}{}
	}

	sourceName := "source.zip"
	if name, ok := colVal["zip_name"].(string); ok && strings.TrimSpace(name) != "" {
		sourceName = name
	}
	if sourceName == "source.zip" {
		if base := path.Base(oldKey); base != "" {
			sourceName = base
		}
	}

	colVal["updatedAt"] = time.Now().Format(time.RFC3339)
	if _, ok := colVal["createdAt"]; !ok {
		colVal["createdAt"] = time.Now().Format(time.RFC3339)
	}
	if _, ok := colVal["prefix"]; !ok {
		colVal["prefix"] = prefix
	}
	meta["collection"] = colVal

	zips, _ := meta["zips"].([]interface{})
	if zips == nil {
		zips = []interface{}{}
	}
	if !zipEntryExists(zips, destKey) {
		zips = append(zips, map[string]interface{}{
			"key":     destKey,
			"name":    sourceName,
			"addedAt": time.Now().Format(time.RFC3339),
		})
	}
	meta["zips"] = zips

	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	uploader := qiniustorage.NewFormUploader(q.Cfg)
	putPolicy := qiniustorage.PutPolicy{Scope: fmt.Sprintf("%s:%s", q.Bucket, metaKey), Expires: 3600}
	upToken := putPolicy.UploadToken(q.Mac)
	var putRet qiniustorage.PutRet
	return uploader.Put(context.Background(), &putRet, upToken, metaKey, bytes.NewReader(metaJSON), int64(len(metaJSON)), &qiniustorage.PutExtra{MimeType: "application/json"})
}

func fetchMetaJSON(bm *qiniustorage.BucketManager, bucket, key string) (map[string]interface{}, error) {
	out, err := bm.Get(bucket, key, nil)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	defer out.Close()
	body, err := io.ReadAll(out)
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

func zipEntryExists(entries []interface{}, key string) bool {
	for _, item := range entries {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if v, ok := row["key"].(string); ok && v == key {
			return true
		}
	}
	return false
}

func isNotFound(err error) bool {
	info, ok := err.(*client.ErrorInfo)
	if !ok {
		return false
	}
	return info.Code == 404 || info.Code == 612
}
