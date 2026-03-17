package videojobs

import (
	"fmt"
	"strings"
	"testing"

	"emoji/internal/models"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUpsertGIFEvaluationByPublicOutput_UsesMetadataCandidateID(t *testing.T) {
	db := openGIFEvaluationTestDB(t)
	if err := db.Exec(`
		CREATE TABLE archive.video_job_gif_candidates (
			id INTEGER PRIMARY KEY,
			job_id INTEGER NOT NULL,
			start_ms INTEGER NOT NULL DEFAULT 0,
			end_ms INTEGER NOT NULL DEFAULT 0,
			base_score REAL NOT NULL DEFAULT 0,
			confidence_score REAL NOT NULL DEFAULT 0,
			feature_json TEXT,
			is_selected BOOLEAN NOT NULL DEFAULT 0,
			final_rank INTEGER NOT NULL DEFAULT 0
		)
	`).Error; err != nil {
		t.Fatalf("create candidates table: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE archive.video_job_gif_evaluations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id INTEGER NOT NULL,
			output_id INTEGER,
			candidate_id INTEGER,
			window_start_ms INTEGER NOT NULL DEFAULT 0,
			window_end_ms INTEGER NOT NULL DEFAULT 0,
			emotion_score REAL NOT NULL DEFAULT 0,
			clarity_score REAL NOT NULL DEFAULT 0,
			motion_score REAL NOT NULL DEFAULT 0,
			loop_score REAL NOT NULL DEFAULT 0,
			efficiency_score REAL NOT NULL DEFAULT 0,
			overall_score REAL NOT NULL DEFAULT 0,
			feature_json TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE (job_id, output_id)
		)
	`).Error; err != nil {
		t.Fatalf("create evaluations table: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO archive.video_job_gif_candidates (
			id, job_id, start_ms, end_ms, base_score, confidence_score, feature_json, is_selected, final_rank
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 9, 7, 1200, 3200, 0.81, 0.92, `{"quality_mean":0.8}`, 1, 1).Error; err != nil {
		t.Fatalf("insert candidate: %v", err)
	}

	output := models.VideoImageOutputPublic{
		ID:         101,
		JobID:      7,
		Format:     "gif",
		FileRole:   "main",
		SizeBytes:  240000,
		Width:      640,
		Height:     640,
		DurationMs: 2000,
		Score:      0.83,
		Metadata: datatypes.JSON([]byte(`{
			"start_sec": 1.2,
			"end_sec": 3.2,
			"candidate_id": 9
		}`)),
	}
	if err := UpsertGIFEvaluationByPublicOutput(db, output); err != nil {
		t.Fatalf("upsert gif evaluation failed: %v", err)
	}

	var row struct {
		CandidateID *uint64 `gorm:"column:candidate_id"`
	}
	if err := db.Table("archive.video_job_gif_evaluations").
		Select("candidate_id").
		Where("job_id = ? AND output_id = ?", 7, 101).
		Scan(&row).Error; err != nil {
		t.Fatalf("query evaluation row: %v", err)
	}
	if row.CandidateID == nil || *row.CandidateID != 9 {
		t.Fatalf("expected candidate_id=9, got %+v", row.CandidateID)
	}
}

func openGIFEvaluationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.Exec(`ATTACH DATABASE ':memory:' AS archive`).Error; err != nil {
		t.Fatalf("attach archive schema: %v", err)
	}
	return db
}
