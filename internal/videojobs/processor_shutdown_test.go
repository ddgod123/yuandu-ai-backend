package videojobs

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"emoji/internal/config"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAcquireVideoJobRunRegistersRunningJob(t *testing.T) {
	db := openProcessorShutdownTestDB(t)
	if err := db.Exec(`
		INSERT INTO archive.video_jobs (id, status, stage, progress, output_formats)
		VALUES (?, ?, ?, ?, ?)
	`, 101, "queued", "queued", 0, "png").Error; err != nil {
		t.Fatalf("insert video job: %v", err)
	}

	p := NewProcessor(db, nil, config.Config{})
	startedAt := time.Now().Add(-30 * time.Second).Round(time.Second)
	acquired, err := p.acquireVideoJobRun(101, startedAt)
	if err != nil {
		t.Fatalf("acquireVideoJobRun returned error: %v", err)
	}
	if !acquired {
		t.Fatalf("expected acquireVideoJobRun to acquire queued job")
	}

	p.runMu.Lock()
	run, ok := p.running[101]
	p.runMu.Unlock()
	if !ok {
		t.Fatalf("expected job 101 to be tracked in running registry")
	}
	if run.StartedAt.IsZero() {
		t.Fatalf("expected tracked job to record StartedAt")
	}

	p.unregisterRunningJob(101)
	p.runMu.Lock()
	_, exists := p.running[101]
	p.runMu.Unlock()
	if exists {
		t.Fatalf("expected job 101 to be removed from running registry")
	}
}

func TestRequeueRunningJobsOnShutdown(t *testing.T) {
	db := openProcessorShutdownTestDB(t)
	if err := db.Exec(`
		INSERT INTO archive.video_jobs (id, status, stage, progress, output_formats)
		VALUES (?, ?, ?, ?, ?)
	`, 202, "running", "analyzing", 45, "png").Error; err != nil {
		t.Fatalf("insert video job: %v", err)
	}

	p := NewProcessor(db, nil, config.Config{})
	p.registerRunningJob(202, time.Now().Add(-2*time.Minute))

	reason := "worker received SIGTERM; re-queue unfinished task for retry"
	report := p.RequeueRunningJobsOnShutdown(reason)
	if report.Tracked != 1 || report.Requeued != 1 || report.Skipped != 0 || report.Failed != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}

	var row struct {
		Status       string
		Stage        string
		ErrorMessage string
	}
	if err := db.Table("archive.video_jobs").
		Select("status, stage, error_message").
		Where("id = ?", 202).
		Scan(&row).Error; err != nil {
		t.Fatalf("query updated job: %v", err)
	}
	if row.Status != "queued" {
		t.Fatalf("expected status queued, got %q", row.Status)
	}
	if row.Stage != "retrying" {
		t.Fatalf("expected stage retrying, got %q", row.Stage)
	}
	if row.ErrorMessage != reason {
		t.Fatalf("expected error_message %q, got %q", reason, row.ErrorMessage)
	}

	var eventCount int64
	if err := db.Table("archive.video_job_events").
		Where("job_id = ? AND stage = ? AND message = ?", 202, "retrying", reason).
		Count(&eventCount).Error; err != nil {
		t.Fatalf("count retrying events: %v", err)
	}
	if eventCount == 0 {
		t.Fatalf("expected retrying event to be recorded")
	}

	second := p.RequeueRunningJobsOnShutdown(reason)
	if second.Tracked != 0 {
		t.Fatalf("expected second call to have zero tracked jobs, got %+v", second)
	}
}

func openProcessorShutdownTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsnName := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_")
	dsnName = strings.ReplaceAll(dsnName, " ", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", dsnName)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.Exec(`ATTACH DATABASE ':memory:' AS archive`).Error; err != nil {
		t.Fatalf("attach archive schema: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE archive.video_jobs (
			id INTEGER PRIMARY KEY,
			status TEXT NOT NULL,
			stage TEXT,
			progress INTEGER,
			started_at DATETIME,
			updated_at DATETIME,
			error_message TEXT,
			output_formats TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create archive.video_jobs: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE archive.video_job_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id INTEGER NOT NULL,
			stage TEXT,
			level TEXT,
			message TEXT,
			metadata TEXT,
			created_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create archive.video_job_events: %v", err)
	}
	return db
}
