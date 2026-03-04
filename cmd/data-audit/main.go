package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/db"
	"emoji/internal/models"
	"emoji/internal/storage"

	"github.com/joho/godotenv"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

var allowedImageExt = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".gif":  {},
	".webp": {},
	".avif": {},
	".bmp":  {},
}

type emojiRow struct {
	ID           uint64 `gorm:"column:id"`
	CollectionID uint64 `gorm:"column:collection_id"`
	FileURL      string `gorm:"column:file_url"`
	Status       string `gorm:"column:status"`
}

type collectionZipRow struct {
	ID           uint64 `gorm:"column:id"`
	CollectionID uint64 `gorm:"column:collection_id"`
	ZipKey       string `gorm:"column:zip_key"`
}

type auditReport struct {
	Apply                   bool                `json:"apply"`
	FixOrphans              bool                `json:"fix_orphans"`
	GeneratedAt             time.Time           `json:"generated_at"`
	DBEmojiTotal            int                 `json:"db_emoji_total"`
	DBZipTotal              int                 `json:"db_zip_total"`
	QiniuObjectTotal        int                 `json:"qiniu_object_total"`
	DSEmojiCount            int                 `json:"ds_emoji_count"`
	NonImageEmojiCount      int                 `json:"non_image_emoji_count"`
	MissingEmojiObjectCount int                 `json:"missing_emoji_object_count"`
	MissingZipObjectCount   int                 `json:"missing_zip_object_count"`
	QiniuDSStoreCount       int                 `json:"qiniu_ds_store_count"`
	QiniuNonImageRawCount   int                 `json:"qiniu_non_image_raw_count"`
	QiniuOrphanRawCount     int                 `json:"qiniu_orphan_raw_count"`
	QiniuOrphanZipCount     int                 `json:"qiniu_orphan_zip_count"`
	FileCountMismatchCount  int                 `json:"file_count_mismatch_count"`
	Applied                 map[string]int      `json:"applied,omitempty"`
	Suggestions             []string            `json:"suggestions"`
	Samples                 map[string][]string `json:"samples"`
}

func main() {
	apply := flag.Bool("apply", false, "apply fixes")
	fixOrphans := flag.Bool("fix-orphans", false, "also fix orphan db/qiniu records (requires --apply)")
	reportFile := flag.String("report", "", "write audit report JSON to file")
	flag.Parse()

	if *fixOrphans && !*apply {
		log.Fatal("--fix-orphans requires --apply")
	}

	loadEnv()
	cfg := config.Load()
	dbConn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	q, err := storage.NewQiniuClient(cfg)
	if err != nil {
		log.Fatalf("qiniu connect failed: %v", err)
	}

	report, err := runAudit(dbConn, q, *apply, *fixOrphans)
	if err != nil {
		log.Fatalf("audit failed: %v", err)
	}

	out, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(out))
	if strings.TrimSpace(*reportFile) != "" {
		if err := os.WriteFile(*reportFile, out, 0644); err != nil {
			log.Fatalf("write report failed: %v", err)
		}
	}
}

func runAudit(dbConn *gorm.DB, q *storage.QiniuClient, apply, fixOrphans bool) (*auditReport, error) {
	report := &auditReport{
		Apply:       apply,
		FixOrphans:  fixOrphans,
		GeneratedAt: time.Now(),
		Applied:     map[string]int{},
		Samples:     map[string][]string{},
	}

	var emojis []emojiRow
	if err := dbConn.Model(&models.Emoji{}).Select("id", "collection_id", "file_url", "status").Find(&emojis).Error; err != nil {
		return nil, err
	}
	report.DBEmojiTotal = len(emojis)

	var zipRows []collectionZipRow
	if err := dbConn.Model(&models.CollectionZip{}).Select("id", "collection_id", "zip_key").Find(&zipRows).Error; err != nil {
		return nil, err
	}
	report.DBZipTotal = len(zipRows)

	qiniuKeys, qiniuDSStore, qiniuNonImageRaw, qiniuRawImage, qiniuSourceZip, err := listQiniuObjects(q)
	if err != nil {
		return nil, err
	}
	report.QiniuObjectTotal = len(qiniuKeys)

	dbEmojiKeys := map[string]emojiRow{}
	dbZipKeys := map[string]collectionZipRow{}
	dsEmojiIDs := make([]uint64, 0)
	nonImageEmojiIDs := make([]uint64, 0)
	missingEmojiIDs := make([]uint64, 0)
	missingZipIDs := make([]uint64, 0)
	affectedCollectionIDs := map[uint64]struct{}{}
	missingEmojiKeys := make([]string, 0)
	missingZipKeys := make([]string, 0)

	for _, row := range emojis {
		key := strings.TrimSpace(row.FileURL)
		if key == "" || strings.HasPrefix(strings.ToLower(key), "http://") || strings.HasPrefix(strings.ToLower(key), "https://") {
			continue
		}
		dbEmojiKeys[key] = row
		base := strings.ToLower(filepath.Base(key))
		if base == ".ds_store" {
			dsEmojiIDs = append(dsEmojiIDs, row.ID)
			affectedCollectionIDs[row.CollectionID] = struct{}{}
		}
		if !isAllowedImageKey(key) {
			nonImageEmojiIDs = append(nonImageEmojiIDs, row.ID)
			affectedCollectionIDs[row.CollectionID] = struct{}{}
		}
		if _, ok := qiniuKeys[key]; !ok {
			missingEmojiIDs = append(missingEmojiIDs, row.ID)
			missingEmojiKeys = append(missingEmojiKeys, key)
			affectedCollectionIDs[row.CollectionID] = struct{}{}
		}
	}

	for _, row := range zipRows {
		key := strings.TrimSpace(row.ZipKey)
		if key == "" {
			continue
		}
		dbZipKeys[key] = row
		if _, ok := qiniuKeys[key]; !ok {
			missingZipIDs = append(missingZipIDs, row.ID)
			missingZipKeys = append(missingZipKeys, key)
		}
	}

	orphanRawQiniu := make([]string, 0)
	for key := range qiniuRawImage {
		if _, ok := dbEmojiKeys[key]; !ok {
			orphanRawQiniu = append(orphanRawQiniu, key)
		}
	}

	orphanSourceZipQiniu := make([]string, 0)
	for key := range qiniuSourceZip {
		if _, ok := dbZipKeys[key]; !ok {
			orphanSourceZipQiniu = append(orphanSourceZipQiniu, key)
		}
	}

	sort.Strings(missingEmojiKeys)
	sort.Strings(missingZipKeys)
	sort.Strings(orphanRawQiniu)
	sort.Strings(orphanSourceZipQiniu)

	report.DSEmojiCount = len(dsEmojiIDs)
	report.NonImageEmojiCount = len(nonImageEmojiIDs)
	report.MissingEmojiObjectCount = len(missingEmojiIDs)
	report.MissingZipObjectCount = len(missingZipIDs)
	report.QiniuDSStoreCount = len(qiniuDSStore)
	report.QiniuNonImageRawCount = len(qiniuNonImageRaw)
	report.QiniuOrphanRawCount = len(orphanRawQiniu)
	report.QiniuOrphanZipCount = len(orphanSourceZipQiniu)
	report.Samples["missing_emoji_object_keys"] = sampleStrings(missingEmojiKeys, 30)
	report.Samples["missing_zip_object_keys"] = sampleStrings(missingZipKeys, 30)
	report.Samples["qiniu_orphan_raw_keys"] = sampleStrings(orphanRawQiniu, 30)
	report.Samples["qiniu_orphan_zip_keys"] = sampleStrings(orphanSourceZipQiniu, 30)

	if apply {
		if len(dsEmojiIDs) > 0 {
			if err := dbConn.Where("id IN ?", dsEmojiIDs).Delete(&models.Emoji{}).Error; err == nil {
				report.Applied["delete_ds_emoji_rows"] = len(dsEmojiIDs)
			}
		}
		if len(nonImageEmojiIDs) > 0 {
			if err := dbConn.Where("id IN ?", nonImageEmojiIDs).Delete(&models.Emoji{}).Error; err == nil {
				report.Applied["delete_non_image_emoji_rows"] = len(nonImageEmojiIDs)
			}
		}
		if fixOrphans && len(missingEmojiIDs) > 0 {
			if err := dbConn.Where("id IN ?", missingEmojiIDs).Delete(&models.Emoji{}).Error; err == nil {
				report.Applied["delete_missing_object_emoji_rows"] = len(missingEmojiIDs)
			}
		}
		if fixOrphans && len(missingZipIDs) > 0 {
			if err := dbConn.Where("id IN ?", missingZipIDs).Delete(&models.CollectionZip{}).Error; err == nil {
				report.Applied["delete_missing_object_zip_rows"] = len(missingZipIDs)
			}
		}

		deleteQiniu := append([]string{}, qiniuDSStore...)
		deleteQiniu = append(deleteQiniu, qiniuNonImageRaw...)
		if fixOrphans {
			deleteQiniu = append(deleteQiniu, orphanRawQiniu...)
			deleteQiniu = append(deleteQiniu, orphanSourceZipQiniu...)
		}
		if len(deleteQiniu) > 0 {
			if deleted := batchDeleteQiniuKeys(q, deleteQiniu); deleted > 0 {
				report.Applied["delete_qiniu_objects"] = deleted
			}
		}

		if fixOrphans {
			if updated := recalcAllCollectionFileCount(dbConn); updated > 0 {
				report.Applied["recalc_collection_file_count"] = updated
			}
		} else {
			if updated := recalcCollectionFileCount(dbConn, affectedCollectionIDs); updated > 0 {
				report.Applied["recalc_collection_file_count"] = updated
			}
		}
	}

	report.FileCountMismatchCount = countFileCountMismatch(dbConn)
	report.Suggestions = buildSuggestions(report)
	return report, nil
}

func listQiniuObjects(q *storage.QiniuClient) (map[string]struct{}, []string, []string, map[string]struct{}, map[string]struct{}, error) {
	keys := map[string]struct{}{}
	dsStore := make([]string, 0)
	nonImageRaw := make([]string, 0)
	rawImage := map[string]struct{}{}
	sourceZip := map[string]struct{}{}

	bm := q.BucketManager()
	marker := ""
	for {
		items, _, nextMarker, hasNext, err := bm.ListFiles(q.Bucket, "emoji/collections/", "", marker, 1000)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		for _, item := range items {
			key := strings.TrimSpace(item.Key)
			if key == "" {
				continue
			}
			keys[key] = struct{}{}
			base := strings.ToLower(filepath.Base(key))
			if base == ".ds_store" {
				dsStore = append(dsStore, key)
			}
			lower := strings.ToLower(key)
			if strings.Contains(lower, "/raw/") {
				if isAllowedImageKey(key) {
					rawImage[key] = struct{}{}
				} else {
					nonImageRaw = append(nonImageRaw, key)
				}
			}
			if strings.Contains(lower, "/source/") && strings.HasSuffix(lower, ".zip") {
				sourceZip[key] = struct{}{}
			}
		}
		if !hasNext || nextMarker == "" {
			break
		}
		marker = nextMarker
	}

	sort.Strings(dsStore)
	sort.Strings(nonImageRaw)
	return keys, dsStore, nonImageRaw, rawImage, sourceZip, nil
}

func isAllowedImageKey(key string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(key)))
	_, ok := allowedImageExt[ext]
	return ok
}

func batchDeleteQiniuKeys(q *storage.QiniuClient, keys []string) int {
	if q == nil || len(keys) == 0 {
		return 0
	}
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
		return 0
	}
	rets, err := q.BucketManager().Batch(ops)
	if err != nil {
		return 0
	}
	deleted := 0
	for _, ret := range rets {
		if ret.Code == 200 || ret.Code == 612 {
			deleted++
		}
	}
	return deleted
}

func recalcCollectionFileCount(dbConn *gorm.DB, ids map[uint64]struct{}) int {
	if len(ids) == 0 {
		return 0
	}
	collectionIDs := make([]uint64, 0, len(ids))
	for id := range ids {
		if id > 0 {
			collectionIDs = append(collectionIDs, id)
		}
	}
	if len(collectionIDs) == 0 {
		return 0
	}
	updated := 0
	for _, id := range collectionIDs {
		var cnt int64
		_ = dbConn.Model(&models.Emoji{}).
			Where("collection_id = ? AND status = ?", id, "active").
			Count(&cnt).Error
		if err := dbConn.Model(&models.Collection{}).Where("id = ?", id).Update("file_count", int(cnt)).Error; err == nil {
			updated++
		}
	}
	return updated
}

func recalcAllCollectionFileCount(dbConn *gorm.DB) int {
	affected := 0
	res1 := dbConn.Exec(`
UPDATE archive.collections c
SET file_count = e.cnt
FROM (
  SELECT collection_id, COUNT(*)::int AS cnt
  FROM archive.emojis
  WHERE status = 'active' AND deleted_at IS NULL
  GROUP BY collection_id
) e
WHERE c.id = e.collection_id
  AND c.file_count <> e.cnt
`)
	if res1.Error == nil {
		affected += int(res1.RowsAffected)
	}

	res2 := dbConn.Exec(`
UPDATE archive.collections c
SET file_count = 0
WHERE c.file_count <> 0
  AND NOT EXISTS (
    SELECT 1
    FROM archive.emojis e
    WHERE e.collection_id = c.id
      AND e.status = 'active'
      AND e.deleted_at IS NULL
  )
`)
	if res2.Error == nil {
		affected += int(res2.RowsAffected)
	}
	return affected
}

func countFileCountMismatch(dbConn *gorm.DB) int {
	type row struct {
		CollectionID uint64 `gorm:"column:collection_id"`
	}
	var rows []row
	query := `
SELECT c.id AS collection_id
FROM archive.collections c
LEFT JOIN (
  SELECT collection_id, COUNT(*)::int AS cnt
  FROM archive.emojis
  WHERE status = 'active' AND deleted_at IS NULL
  GROUP BY collection_id
) e ON e.collection_id = c.id
WHERE c.file_count <> COALESCE(e.cnt, 0)
`
	if err := dbConn.Raw(query).Scan(&rows).Error; err != nil {
		return 0
	}
	return len(rows)
}

func buildSuggestions(report *auditReport) []string {
	items := make([]string, 0, 8)
	if report.MissingZipObjectCount > 0 {
		items = append(items, "运行 go run ./cmd/repair-collection-zips --apply 补录历史 ZIP 记录并修复最新 ZIP 指针")
	}
	if report.MissingEmojiObjectCount > 0 {
		items = append(items, "检查七牛是否误删素材；若确认缺失可用 --apply --fix-orphans 清理失效 emoji 记录")
	}
	if report.QiniuOrphanRawCount > 0 || report.QiniuOrphanZipCount > 0 {
		items = append(items, "先 dry-run 审核样本后再执行 --apply --fix-orphans 清理孤儿对象")
	}
	if report.FileCountMismatchCount > 0 {
		items = append(items, "优先修复 file_count 不一致，避免前台数量展示异常")
	}
	if len(items) == 0 {
		items = append(items, "当前未发现高风险数据问题，可将该审计命令加入每日 cron")
	}
	return items
}

func sampleStrings(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func loadEnv() {
	seen := map[string]struct{}{}
	candidates := []string{".env", "backend/.env"}
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for i := 0; i < 5; i++ {
			candidates = append(candidates, filepath.Join(dir, ".env"), filepath.Join(dir, "backend", ".env"))
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
