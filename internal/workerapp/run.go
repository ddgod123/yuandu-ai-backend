package workerapp

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/copyrightjobs"
	"emoji/internal/db"
	"emoji/internal/models"
	"emoji/internal/queue"
	"emoji/internal/storage"
	"emoji/internal/videojobs"

	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
)

func Run(videoWorkerRole string) {
	loadEnv()
	if role := normalizeWorkerRole(videoWorkerRole); role != "" {
		_ = os.Setenv("VIDEO_WORKER_ROLE", role)
	}

	cfg := config.Load()

	dbConn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	qiniuClient, err := storage.NewQiniuClient(cfg)
	if err != nil {
		log.Fatalf("qiniu connect failed: %v", err)
	}

	if os.Getenv("DB_AUTO_MIGRATE") == "true" {
		if err := models.AutoMigrate(dbConn); err != nil {
			log.Fatalf("auto migrate failed: %v", err)
		}
	}

	server := queue.NewServer(cfg)
	mux := asynq.NewServeMux()
	processor := videojobs.NewProcessor(dbConn, qiniuClient, cfg)
	processor.Register(mux)
	copyrightProcessor := copyrightjobs.NewProcessor(dbConn, cfg)
	copyrightProcessor.Register(mux)

	cleanupHours := parseCleanupHours("VIDEO_JOB_TMP_CLEANUP_HOURS", 12)
	if cleanupHours > 0 {
		report := videojobs.CleanupStaleTempDirs("video-job-", time.Duration(cleanupHours)*time.Hour)
		log.Printf(
			"video temp cleanup done (scanned=%d removed=%d failed=%d older_than=%dh)",
			report.Scanned,
			report.Removed,
			report.Failed,
			cleanupHours,
		)
	}

	log.Printf(
		"video worker started (redis=%s db=%d role=%s queues=%s)",
		cfg.AsynqRedisAddr,
		cfg.AsynqRedisDB,
		normalizeWorkerRole(os.Getenv("VIDEO_WORKER_ROLE")),
		formatQueueWeightsForLog(queue.ResolveAsynqQueueWeightsFromEnv()),
	)
	if err := server.Run(mux); err != nil {
		log.Fatalf("worker failed: %v", err)
	}
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

	for _, p := range candidates {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Overload(p)
		}
	}
}

func parseCleanupHours(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

func normalizeWorkerRole(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "all", "default":
		return "all"
	case "gif":
		return "gif"
	case "png", "image":
		return "png"
	case "media":
		return "media"
	default:
		return "all"
	}
}

func formatQueueWeightsForLog(weights map[string]int) string {
	if len(weights) == 0 {
		return "-"
	}
	names := make([]string, 0, len(weights))
	for queueName := range weights {
		names = append(names, queueName)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, queueName := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", queueName, weights[queueName]))
	}
	return strings.Join(parts, ",")
}
