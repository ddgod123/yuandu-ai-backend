package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/db"
	"emoji/internal/models"
	"emoji/internal/storage"

	"github.com/joho/godotenv"
	"github.com/qiniu/go-sdk/v7/client"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

type zipInfo struct {
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	Hash       string     `json:"hash,omitempty"`
	Size       int64      `json:"size"`
	UploadedAt *time.Time `json:"uploaded_at,omitempty"`
	FromDB     bool       `json:"from_db"`
	FromMeta   bool       `json:"from_meta"`
	FromQiniu  bool       `json:"from_qiniu"`
}

type collectionRepairReport struct {
	CollectionID        uint64     `json:"collection_id"`
	Title               string     `json:"title"`
	Prefix              string     `json:"prefix"`
	ScannedAt           time.Time  `json:"scanned_at"`
	DBZipCount          int        `json:"db_zip_count"`
	DetectedZipCount    int        `json:"detected_zip_count"`
	MissingInDBCount    int        `json:"missing_in_db_count"`
	MissingOnQiniuCount int        `json:"missing_on_qiniu_count"`
	InsertedCount       int        `json:"inserted_count"`
	UpdatedCount        int        `json:"updated_count"`
	LatestFixed         bool       `json:"latest_fixed"`
	MissingInDB         []string   `json:"missing_in_db,omitempty"`
	MissingOnQiniu      []string   `json:"missing_on_qiniu,omitempty"`
	LatestZipKeyBefore  string     `json:"latest_zip_key_before,omitempty"`
	LatestZipKeyAfter   string     `json:"latest_zip_key_after,omitempty"`
	LatestZipNameAfter  string     `json:"latest_zip_name_after,omitempty"`
	LatestZipSizeAfter  int64      `json:"latest_zip_size_after,omitempty"`
	LatestZipUploadedAt *time.Time `json:"latest_zip_uploaded_at,omitempty"`
}

type summaryReport struct {
	Apply               bool                     `json:"apply"`
	ScannedCollections  int                      `json:"scanned_collections"`
	InsertedRows        int                      `json:"inserted_rows"`
	UpdatedRows         int                      `json:"updated_rows"`
	LatestFixedCount    int                      `json:"latest_fixed_count"`
	MissingInDBTotal    int                      `json:"missing_in_db_total"`
	MissingOnQiniuTotal int                      `json:"missing_on_qiniu_total"`
	GeneratedAt         time.Time                `json:"generated_at"`
	Collections         []collectionRepairReport `json:"collections"`
}

func main() {
	apply := flag.Bool("apply", false, "apply repair changes to database")
	collectionID := flag.Uint64("collection-id", 0, "repair only one collection id")
	limit := flag.Int("limit", 0, "limit number of collections to scan")
	reportFile := flag.String("report", "", "write json report to file path")
	flag.Parse()

	loadEnv()
	cfg := config.Load()

	dbConn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	qiniuClient, err := storage.NewQiniuClient(cfg)
	if err != nil {
		log.Fatalf("qiniu connect failed: %v", err)
	}
	bm := qiniuClient.BucketManager()

	query := dbConn.Model(&models.Collection{}).Where("COALESCE(qiniu_prefix, '') <> ''")
	if *collectionID > 0 {
		query = query.Where("id = ?", *collectionID)
	}
	if *limit > 0 {
		query = query.Limit(*limit)
	}
	var collections []models.Collection
	if err := query.Order("id asc").Find(&collections).Error; err != nil {
		log.Fatalf("load collections failed: %v", err)
	}

	summary := summaryReport{
		Apply:       *apply,
		GeneratedAt: time.Now(),
		Collections: make([]collectionRepairReport, 0, len(collections)),
	}

	for _, collection := range collections {
		rep, err := repairOneCollection(dbConn, bm, qiniuClient.Bucket, collection, *apply)
		if err != nil {
			log.Printf("repair collection %d failed: %v", collection.ID, err)
			continue
		}
		summary.ScannedCollections++
		summary.InsertedRows += rep.InsertedCount
		summary.UpdatedRows += rep.UpdatedCount
		summary.MissingInDBTotal += rep.MissingInDBCount
		summary.MissingOnQiniuTotal += rep.MissingOnQiniuCount
		if rep.LatestFixed {
			summary.LatestFixedCount++
		}
		summary.Collections = append(summary.Collections, rep)
	}

	out, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(out))
	if strings.TrimSpace(*reportFile) != "" {
		if err := os.WriteFile(*reportFile, out, 0644); err != nil {
			log.Fatalf("write report failed: %v", err)
		}
	}
}

func repairOneCollection(dbConn *gorm.DB, bm *qiniustorage.BucketManager, bucket string, collection models.Collection, apply bool) (collectionRepairReport, error) {
	prefix := normalizePrefix(collection.QiniuPrefix)
	report := collectionRepairReport{
		CollectionID:       collection.ID,
		Title:              collection.Title,
		Prefix:             prefix,
		ScannedAt:          time.Now(),
		LatestZipKeyBefore: strings.TrimSpace(collection.LatestZipKey),
	}

	if prefix == "" {
		return report, nil
	}

	infos := make(map[string]*zipInfo)

	var dbRows []models.CollectionZip
	if err := dbConn.Where("collection_id = ?", collection.ID).Find(&dbRows).Error; err != nil {
		return report, err
	}
	report.DBZipCount = len(dbRows)
	for _, row := range dbRows {
		key := strings.TrimSpace(row.ZipKey)
		if key == "" {
			continue
		}
		item := ensureZipInfo(infos, key)
		item.FromDB = true
		if item.Name == "" {
			item.Name = strings.TrimSpace(row.ZipName)
		}
		if item.Size == 0 {
			item.Size = row.SizeBytes
		}
		if item.Hash == "" {
			item.Hash = strings.TrimSpace(row.ZipHash)
		}
		if item.UploadedAt == nil && row.UploadedAt != nil {
			ts := *row.UploadedAt
			item.UploadedAt = &ts
		}
	}

	metaMap, _ := fetchMetaJSONFromQiniu(bm, bucket, prefix+"meta.json")
	if metaMap != nil {
		if rawList, ok := metaMap["zips"].([]interface{}); ok {
			for _, raw := range rawList {
				row, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				key, _ := row["key"].(string)
				key = strings.TrimSpace(key)
				if key == "" || !strings.HasSuffix(strings.ToLower(key), ".zip") {
					continue
				}
				item := ensureZipInfo(infos, key)
				item.FromMeta = true
				if item.Name == "" {
					item.Name, _ = row["name"].(string)
				}
				if item.Hash == "" {
					hash, _ := row["hash"].(string)
					item.Hash = strings.TrimSpace(hash)
				}
				if item.UploadedAt == nil {
					if addedAt, _ := row["addedAt"].(string); strings.TrimSpace(addedAt) != "" {
						if parsed, err := time.Parse(time.RFC3339, addedAt); err == nil {
							item.UploadedAt = &parsed
						}
					}
				}
			}
		}
	}

	marker := ""
	sourcePrefix := prefix + "source/"
	for {
		items, _, nextMarker, hasNext, err := bm.ListFiles(bucket, sourcePrefix, "", marker, 1000)
		if err != nil {
			break
		}
		for _, row := range items {
			key := strings.TrimSpace(row.Key)
			if key == "" || !strings.HasSuffix(strings.ToLower(key), ".zip") {
				continue
			}
			item := ensureZipInfo(infos, key)
			item.FromQiniu = true
			if item.Name == "" {
				item.Name = path.Base(key)
			}
			if item.Size == 0 {
				item.Size = row.Fsize
			}
			if item.Hash == "" {
				item.Hash = strings.TrimSpace(row.Hash)
			}
			if item.UploadedAt == nil {
				item.UploadedAt = qiniuPutTimeToTime(row.PutTime)
			}
		}
		if !hasNext || nextMarker == "" {
			break
		}
		marker = nextMarker
	}

	keys := make([]string, 0, len(infos))
	for key := range infos {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	report.DetectedZipCount = len(keys)
	for _, key := range keys {
		item := infos[key]
		if !item.FromDB && (item.FromMeta || item.FromQiniu) {
			report.MissingInDB = append(report.MissingInDB, key)
		}
		if item.FromDB && !item.FromQiniu {
			report.MissingOnQiniu = append(report.MissingOnQiniu, key)
		}
	}
	report.MissingInDBCount = len(report.MissingInDB)
	report.MissingOnQiniuCount = len(report.MissingOnQiniu)

	if apply {
		for _, key := range keys {
			item := infos[key]
			if item.Name == "" {
				item.Name = path.Base(key)
			}
			var row models.CollectionZip
			err := dbConn.Where("collection_id = ? AND zip_key = ?", collection.ID, key).First(&row).Error
			if err != nil {
				if err == gorm.ErrRecordNotFound {
					newRow := models.CollectionZip{
						CollectionID: collection.ID,
						ZipKey:       key,
						ZipHash:      item.Hash,
						ZipName:      item.Name,
						SizeBytes:    item.Size,
						UploadedAt:   item.UploadedAt,
					}
					if err := dbConn.Create(&newRow).Error; err == nil {
						report.InsertedCount++
					}
				}
				continue
			}

			updates := map[string]interface{}{}
			if strings.TrimSpace(row.ZipName) == "" && strings.TrimSpace(item.Name) != "" {
				updates["zip_name"] = item.Name
			}
			if row.SizeBytes <= 0 && item.Size > 0 {
				updates["size_bytes"] = item.Size
			}
			if strings.TrimSpace(row.ZipHash) == "" && strings.TrimSpace(item.Hash) != "" {
				updates["zip_hash"] = item.Hash
			}
			if row.UploadedAt == nil && item.UploadedAt != nil {
				updates["uploaded_at"] = item.UploadedAt
			}
			if len(updates) > 0 {
				if err := dbConn.Model(&models.CollectionZip{}).Where("id = ?", row.ID).Updates(updates).Error; err == nil {
					report.UpdatedCount++
				}
			}
		}
	}

	latest := pickLatestZip(keys, infos)
	if latest != nil {
		report.LatestZipKeyAfter = latest.Key
		report.LatestZipNameAfter = latest.Name
		report.LatestZipSizeAfter = latest.Size
		report.LatestZipUploadedAt = latest.UploadedAt
		if apply && strings.TrimSpace(collection.LatestZipKey) != strings.TrimSpace(latest.Key) {
			updates := map[string]interface{}{
				"latest_zip_key":  latest.Key,
				"latest_zip_name": latest.Name,
				"latest_zip_size": latest.Size,
				"latest_zip_at":   latest.UploadedAt,
			}
			if err := dbConn.Model(&models.Collection{}).Where("id = ?", collection.ID).Updates(updates).Error; err == nil {
				report.LatestFixed = true
			}
		}
	}

	return report, nil
}

func pickLatestZip(keys []string, infos map[string]*zipInfo) *zipInfo {
	if len(keys) == 0 {
		return nil
	}
	best := infos[keys[0]]
	for _, key := range keys[1:] {
		current := infos[key]
		if isAfter(current.UploadedAt, best.UploadedAt) {
			best = current
			continue
		}
		if equalTime(current.UploadedAt, best.UploadedAt) && current.Key > best.Key {
			best = current
		}
	}
	return best
}

func isAfter(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}

func equalTime(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func ensureZipInfo(infos map[string]*zipInfo, key string) *zipInfo {
	if row, ok := infos[key]; ok {
		return row
	}
	row := &zipInfo{Key: key}
	infos[key] = row
	return row
}

func fetchMetaJSONFromQiniu(bm *qiniustorage.BucketManager, bucket, key string) (map[string]interface{}, error) {
	rc, err := bm.Get(bucket, key, nil)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
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

func qiniuPutTimeToTime(putTime int64) *time.Time {
	if putTime <= 0 {
		return nil
	}
	ns := putTime * 100
	ts := time.Unix(0, ns)
	return &ts
}

func normalizePrefix(prefix string) string {
	clean := strings.TrimLeft(strings.TrimSpace(prefix), "/")
	if clean == "" {
		return ""
	}
	if !strings.HasSuffix(clean, "/") {
		clean += "/"
	}
	return clean
}

func loadEnv() {
	seen := map[string]struct{}{}
	candidates := []string{".env", "backend/.env"}

	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for i := 0; i < 5; i++ {
			candidates = append(candidates,
				filepath.Join(dir, ".env"),
				filepath.Join(dir, "backend", ".env"),
			)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	for _, file := range candidates {
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		if _, err := os.Stat(file); err == nil {
			_ = godotenv.Overload(file)
		}
	}
}

func isNotFound(err error) bool {
	info, ok := err.(*client.ErrorInfo)
	if !ok {
		return false
	}
	return info.Code == 404 || info.Code == 612
}
