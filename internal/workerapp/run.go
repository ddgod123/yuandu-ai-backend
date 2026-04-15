package workerapp

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"emoji/internal/config"
	"emoji/internal/copyrightjobs"
	"emoji/internal/db"
	"emoji/internal/feishujobs"
	"emoji/internal/models"
	"emoji/internal/queue"
	"emoji/internal/storage"
	"emoji/internal/videojobs"

	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
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
	feishuProcessor := feishujobs.NewProcessor(dbConn, qiniuClient, cfg)
	feishuProcessor.Register(mux)

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

	if isComputeRedeemExpireSweepEnabled() {
		interval := parsePositiveDuration("COMPUTE_REDEEM_EXPIRE_SWEEP_INTERVAL", 10*time.Minute)
		batchSize := parsePositiveInt("COMPUTE_REDEEM_EXPIRE_SWEEP_BATCH", 100)
		go startComputeRedeemExpireSweepLoop(dbConn, interval, batchSize)
	}

	log.Printf(
		"video worker started (redis=%s db=%d role=%s queues=%s)",
		cfg.AsynqRedisAddr,
		cfg.AsynqRedisDB,
		normalizeWorkerRole(os.Getenv("VIDEO_WORKER_ROLE")),
		formatQueueWeightsForLog(queue.ResolveAsynqQueueWeightsFromEnv()),
	)
	if err := server.Start(mux); err != nil {
		log.Fatalf("worker failed to start: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	signal.Stop(sigCh)

	reason := fmt.Sprintf("worker received %s; re-queue unfinished task for retry", sig.String())
	log.Printf("video worker shutdown signal received: %s", sig.String())

	server.Stop()
	preReport := processor.RequeueRunningJobsOnShutdown(reason)
	logShutdownRequeueReport("pre-shutdown", preReport)

	server.Shutdown()
	postReport := processor.RequeueRunningJobsOnShutdown("worker shutdown cleanup: dangling running task re-queued")
	logShutdownRequeueReport("post-shutdown", postReport)
	log.Printf("video worker shutdown complete")
}

func logShutdownRequeueReport(phase string, report videojobs.ShutdownRequeueReport) {
	phase = strings.TrimSpace(phase)
	if phase == "" {
		phase = "shutdown"
	}
	log.Printf(
		"video worker %s requeue report (tracked=%d requeued=%d skipped=%d failed=%d)",
		phase,
		report.Tracked,
		report.Requeued,
		report.Skipped,
		report.Failed,
	)
}

func isComputeRedeemExpireSweepEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("COMPUTE_REDEEM_EXPIRE_SWEEP_ENABLED")))
	if raw == "" {
		return true
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func parsePositiveInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func parsePositiveDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func startComputeRedeemExpireSweepLoop(dbConn *gorm.DB, interval time.Duration, batchSize int) {
	if dbConn == nil {
		return
	}
	log.Printf("compute redeem expire sweep enabled (interval=%s batch=%d)", interval, batchSize)
	sweep := func() {
		result, err := videojobs.SweepExpiredComputeRedeemPoints(dbConn, time.Now(), batchSize)
		if err != nil {
			log.Printf("compute redeem expire sweep failed: %v", err)
			return
		}
		if result.Scanned > 0 || result.Cleared > 0 || result.Errors > 0 {
			log.Printf(
				"compute redeem expire sweep done (scanned=%d cleared=%d skipped=%d errors=%d cleared_points=%d last_error=%s)",
				result.Scanned,
				result.Cleared,
				result.Skipped,
				result.Errors,
				result.TotalClearedPoint,
				result.LastError,
			)
		}
	}

	sweep()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		sweep()
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
	case "png":
		return "png"
	case "jpg":
		return "jpg"
	case "webp":
		return "webp"
	case "live":
		return "live"
	case "mp4":
		return "mp4"
	case "ai":
		return "ai"
	case "image":
		return "image"
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
