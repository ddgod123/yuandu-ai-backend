package main

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/storage"

	"github.com/joho/godotenv"
	"github.com/qiniu/go-sdk/v7/client"
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

	srcBucket := firstNonEmpty(strings.TrimSpace(os.Getenv("QINIU_SRC_BUCKET")), q.Bucket)
	dstBucket := strings.TrimSpace(os.Getenv("QINIU_DST_BUCKET"))
	if dstBucket == "" {
		log.Fatalf("missing QINIU_DST_BUCKET")
	}

	srcPrefix := normalizePrefix(firstNonEmpty(strings.TrimSpace(os.Getenv("QINIU_SRC_PREFIX")), "emoji/"), true)
	dstPrefix := normalizePrefix(strings.TrimSpace(os.Getenv("QINIU_DST_PREFIX")), false)

	force := parseBoolEnv("QINIU_COPY_FORCE", false)
	dryRun := parseBoolEnv("QINIU_COPY_DRY_RUN", true)
	limit := parseIntEnv("QINIU_COPY_LIMIT", 0)
	sleepMs := parseIntEnv("QINIU_COPY_SLEEP_MS", 0)

	log.Printf(
		"start copy: src=%s prefix=%s -> dst=%s prefix=%s dry_run=%v force=%v limit=%d",
		srcBucket,
		firstNonEmpty(srcPrefix, "(all)"),
		dstBucket,
		firstNonEmpty(dstPrefix, "(same-key)"),
		dryRun,
		force,
		limit,
	)

	var (
		marker   string
		scanned  int64
		willCopy int64
		copied   int64
		skipped  int64
		failed   int64
	)

	for {
		entries, _, nextMarker, hasNext, err := bm.ListFiles(srcBucket, srcPrefix, "", marker, 1000)
		if err != nil {
			log.Fatalf("list files failed: %v", err)
		}

		for _, entry := range entries {
			if limit > 0 && scanned >= int64(limit) {
				break
			}
			srcKey := strings.TrimLeft(strings.TrimSpace(entry.Key), "/")
			if srcKey == "" {
				continue
			}
			scanned++

			dstKey, ok := mapDestKey(srcKey, srcPrefix, dstPrefix)
			if !ok || dstKey == "" {
				skipped++
				continue
			}

			if dryRun {
				willCopy++
				if willCopy <= 20 {
					log.Printf("[dry-run] %s -> %s", srcKey, dstKey)
				}
				continue
			}

			if err := bm.Copy(srcBucket, srcKey, dstBucket, dstKey, force); err != nil {
				if info, ok := err.(*client.ErrorInfo); ok && info.Code == 614 {
					skipped++
					continue
				}
				failed++
				log.Printf("[copy-failed] %s -> %s err=%v", srcKey, dstKey, err)
				continue
			}

			copied++
			if copied <= 20 {
				log.Printf("[copied] %s -> %s", srcKey, dstKey)
			}
			if sleepMs > 0 {
				time.Sleep(time.Duration(sleepMs) * time.Millisecond)
			}
		}

		if limit > 0 && scanned >= int64(limit) {
			break
		}
		if !hasNext {
			break
		}
		marker = nextMarker
	}

	if dryRun {
		log.Printf("done dry-run: scanned=%d will_copy=%d skipped=%d failed=%d", scanned, willCopy, skipped, failed)
		return
	}
	log.Printf("done: scanned=%d copied=%d skipped=%d failed=%d", scanned, copied, skipped, failed)
}

func mapDestKey(srcKey, srcPrefix, dstPrefix string) (string, bool) {
	srcKey = strings.TrimLeft(strings.TrimSpace(srcKey), "/")
	if srcKey == "" {
		return "", false
	}
	if srcPrefix == "" {
		return strings.TrimLeft(dstPrefix+srcKey, "/"), true
	}
	if !strings.HasPrefix(srcKey, srcPrefix) {
		return "", false
	}
	rel := strings.TrimPrefix(srcKey, srcPrefix)
	if dstPrefix == "" {
		return srcKey, true
	}
	return strings.TrimLeft(dstPrefix+rel, "/"), true
}

func normalizePrefix(raw string, keepDefault bool) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimLeft(raw, "/")
	if raw == "" {
		if keepDefault {
			return "emoji/"
		}
		return ""
	}
	if !strings.HasSuffix(raw, "/") {
		raw += "/"
	}
	return raw
}

func parseBoolEnv(key string, def bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func parseIntEnv(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
