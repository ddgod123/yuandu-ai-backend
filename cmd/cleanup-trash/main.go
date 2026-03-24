package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/storage"

	"github.com/joho/godotenv"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
)

var (
	trashTimePattern       = regexp.MustCompile(`^\d{8}-\d{6}$`)
	trashSuffixTimePattern = regexp.MustCompile(`(\d{8}-\d{6})$`)
)

type cleanupReport struct {
	GeneratedAt      time.Time `json:"generated_at"`
	Prefix           string    `json:"prefix"`
	RetentionDays    int       `json:"retention_days"`
	CutoffTime       time.Time `json:"cutoff_time"`
	Apply            bool      `json:"apply"`
	TotalScanned     int       `json:"total_scanned"`
	CandidateCount   int       `json:"candidate_count"`
	DeletedCount     int       `json:"deleted_count"`
	FailedCount      int       `json:"failed_count"`
	FailedCodes      []int     `json:"failed_codes"`
	CandidateSamples []string  `json:"candidate_samples"`
}

func main() {
	prefix := flag.String("prefix", "emoji/_trash/", "trash prefix")
	retentionDays := flag.Int("retention-days", 14, "retention in days before hard-delete")
	apply := flag.Bool("apply", false, "execute deletion (default dry-run)")
	reportFile := flag.String("report", "", "write cleanup report JSON to file")
	flag.Parse()

	if *retentionDays < 1 {
		log.Fatal("retention-days must be >= 1")
	}

	loadEnv()
	cfg := config.Load()
	q, err := storage.NewQiniuClient(cfg)
	if err != nil {
		log.Fatalf("qiniu connect failed: %v", err)
	}

	cleanPrefix := strings.TrimLeft(strings.TrimSpace(*prefix), "/")
	if cleanPrefix == "" {
		cleanPrefix = "emoji/_trash/"
	}
	if !strings.HasSuffix(cleanPrefix, "/") {
		cleanPrefix += "/"
	}
	if !strings.HasPrefix(cleanPrefix, "emoji/_trash/") {
		log.Fatal("prefix must start with emoji/_trash/")
	}

	cutoff := time.Now().AddDate(0, 0, -*retentionDays)
	report, err := runCleanup(q, cleanPrefix, cutoff, *retentionDays, *apply)
	if err != nil {
		log.Fatalf("cleanup failed: %v", err)
	}

	out, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(out))
	if strings.TrimSpace(*reportFile) != "" {
		if err := os.WriteFile(*reportFile, out, 0644); err != nil {
			log.Fatalf("write report failed: %v", err)
		}
	}
}

func runCleanup(q *storage.QiniuClient, prefix string, cutoff time.Time, retentionDays int, apply bool) (*cleanupReport, error) {
	bm := q.BucketManager()
	marker := ""
	candidates := make([]string, 0, 1024)
	totalScanned := 0

	for {
		items, _, nextMarker, hasNext, err := bm.ListFiles(q.Bucket, prefix, "", marker, 1000)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			key := strings.TrimSpace(item.Key)
			if key == "" {
				continue
			}
			totalScanned++
			if createdAt, ok := parseTrashCreationTime(key, prefix); ok && createdAt.Before(cutoff) {
				candidates = append(candidates, key)
			}
		}
		if !hasNext || nextMarker == "" {
			break
		}
		marker = nextMarker
	}

	sort.Strings(candidates)
	report := &cleanupReport{
		GeneratedAt:      time.Now(),
		Prefix:           prefix,
		RetentionDays:    retentionDays,
		CutoffTime:       cutoff,
		Apply:            apply,
		TotalScanned:     totalScanned,
		CandidateCount:   len(candidates),
		CandidateSamples: sampleStrings(candidates, 30),
	}

	if !apply || len(candidates) == 0 {
		return report, nil
	}

	failCodes := map[int]struct{}{}
	for start := 0; start < len(candidates); start += 1000 {
		end := start + 1000
		if end > len(candidates) {
			end = len(candidates)
		}
		chunk := candidates[start:end]
		ops := make([]string, 0, len(chunk))
		for _, key := range chunk {
			ops = append(ops, qiniustorage.URIDelete(q.Bucket, key))
		}
		rets, err := bm.Batch(ops)
		if err != nil {
			return report, err
		}
		for _, ret := range rets {
			if ret.Code == 200 || ret.Code == 612 {
				report.DeletedCount++
			} else {
				report.FailedCount++
				failCodes[ret.Code] = struct{}{}
			}
		}
	}

	for code := range failCodes {
		report.FailedCodes = append(report.FailedCodes, code)
	}
	sort.Ints(report.FailedCodes)
	return report, nil
}

func parseTrashCreationTime(key string, prefix string) (time.Time, bool) {
	trimmed := strings.TrimPrefix(strings.TrimLeft(strings.TrimSpace(key), "/"), strings.TrimLeft(prefix, "/"))
	trimmed = strings.TrimLeft(trimmed, "/")
	if trimmed == "" {
		return time.Time{}, false
	}
	segment := strings.SplitN(trimmed, "/", 2)[0]
	if segment == "" {
		return time.Time{}, false
	}
	if trashTimePattern.MatchString(segment) {
		t, err := time.ParseInLocation("20060102-150405", segment, time.Local)
		return t, err == nil
	}
	match := trashSuffixTimePattern.FindStringSubmatch(segment)
	if len(match) == 2 {
		t, err := time.ParseInLocation("20060102-150405", match[1], time.Local)
		return t, err == nil
	}
	return time.Time{}, false
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
