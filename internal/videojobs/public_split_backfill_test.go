package videojobs

import (
	"fmt"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBackfillPublicVideoImageSplitTables_JobsOnly(t *testing.T) {
	db := openPublicSplitBackfillTestDB(t)
	if err := db.Exec(`CREATE TABLE public.video_image_jobs (
		id INTEGER PRIMARY KEY,
		user_id INTEGER,
		title TEXT,
		source_video_key TEXT,
		source_video_name TEXT,
		source_video_ext TEXT,
		source_size_bytes INTEGER,
		source_md5 TEXT,
		requested_format TEXT,
		status TEXT,
		stage TEXT,
		progress INTEGER,
		options TEXT,
		metrics TEXT,
		error_code TEXT,
		error_message TEXT,
		idempotency_key TEXT,
		started_at DATETIME,
		finished_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create base jobs table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_jobs_png (
		id INTEGER PRIMARY KEY,
		user_id INTEGER,
		title TEXT,
		source_video_key TEXT,
		source_video_name TEXT,
		source_video_ext TEXT,
		source_size_bytes INTEGER,
		source_md5 TEXT,
		requested_format TEXT,
		status TEXT,
		stage TEXT,
		progress INTEGER,
		options TEXT,
		metrics TEXT,
		error_code TEXT,
		error_message TEXT,
		idempotency_key TEXT,
		started_at DATETIME,
		finished_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create split jobs table: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO public.video_image_jobs (
			id, user_id, title, source_video_key, requested_format,
			status, stage, progress, options, metrics, created_at, updated_at
		) VALUES (
			101, 9001, 'png task', 'videos/in/demo.mp4', 'png',
			'done', 'completed', 100, '{}', '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
	`).Error; err != nil {
		t.Fatalf("insert base row: %v", err)
	}

	report, err := BackfillPublicVideoImageSplitTables(db, PublicVideoImageSplitBackfillOptions{
		Apply:            true,
		BatchSize:        50,
		IncludeJobs:      true,
		IncludeOutputs:   false,
		IncludePackages:  false,
		IncludeEvents:    false,
		IncludeFeedbacks: false,
	})
	if err != nil {
		t.Fatalf("backfill jobs failed: %v", err)
	}
	if report.Jobs.Written != 1 {
		t.Fatalf("expected jobs written=1, got=%d", report.Jobs.Written)
	}
	if report.FailedTotal() != 0 {
		t.Fatalf("expected failed=0, got=%d", report.FailedTotal())
	}

	var count int64
	if err := db.Table("public.video_image_jobs_png").Where("id = 101").Count(&count).Error; err != nil {
		t.Fatalf("count split rows failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected split row count=1, got=%d", count)
	}
}

func openPublicSplitBackfillTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.Exec(`ATTACH DATABASE ':memory:' AS archive`).Error; err != nil {
		t.Fatalf("attach archive schema: %v", err)
	}
	if err := db.Exec(`ATTACH DATABASE ':memory:' AS public`).Error; err != nil {
		t.Fatalf("attach public schema: %v", err)
	}
	return db
}
