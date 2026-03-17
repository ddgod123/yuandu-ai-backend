package videojobs

import (
	"fmt"
	"strings"
	"testing"

	"emoji/internal/config"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLoadUserHighlightFeedbackProfileStrictModeSkipsLegacyOnPublicQueryError(t *testing.T) {
	db := openFeedbackProfileTestDB(t, false)
	insertFeedbackProfileJob(t, db, 1, 7, `{
		"duration_sec": 10,
		"highlight_v1": {
			"selected": {"start_sec": 2, "end_sec": 5, "reason": "scene_change_peak"}
		},
		"feedback_v1": {"total_signals": 3, "favorite_count": 1},
		"scene_tags_v1": ["reaction"]
	}`)

	p := NewProcessor(db, nil, config.Config{EnableLegacyFeedbackFallback: false})
	profile, err := p.loadUserHighlightFeedbackProfile(7, 80, DefaultQualitySettings())
	if err != nil {
		t.Fatalf("load profile failed: %v", err)
	}
	if profile.EngagedJobs != 0 {
		t.Fatalf("expected strict mode to skip legacy fallback, got engaged_jobs=%d", profile.EngagedJobs)
	}
	if profile.WeightedSignals != 0 {
		t.Fatalf("expected strict mode weighted_signals=0, got %.4f", profile.WeightedSignals)
	}
}

func TestLoadUserHighlightFeedbackProfileLegacyFallbackEnabledUsesLegacyMetrics(t *testing.T) {
	db := openFeedbackProfileTestDB(t, false)
	insertFeedbackProfileJob(t, db, 1, 7, `{
		"duration_sec": 10,
		"highlight_v1": {
			"selected": {"start_sec": 2, "end_sec": 5, "reason": "scene_change_peak"}
		},
		"feedback_v1": {"total_signals": 3, "favorite_count": 1},
		"scene_tags_v1": ["reaction"]
	}`)

	p := NewProcessor(db, nil, config.Config{EnableLegacyFeedbackFallback: true})
	profile, err := p.loadUserHighlightFeedbackProfile(7, 80, DefaultQualitySettings())
	if err != nil {
		t.Fatalf("load profile failed: %v", err)
	}
	if profile.EngagedJobs == 0 {
		t.Fatalf("expected legacy fallback to engage at least one job")
	}
	if profile.WeightedSignals <= 0 {
		t.Fatalf("expected weighted_signals > 0, got %.4f", profile.WeightedSignals)
	}
	if profile.ReasonPreference["scene_change_peak"] <= 0 {
		t.Fatalf("expected reason preference from legacy feedback, got %+v", profile.ReasonPreference)
	}
}

func TestLoadUserHighlightFeedbackProfileStrictModeUsesPublicOutputFeedback(t *testing.T) {
	db := openFeedbackProfileTestDB(t, true)
	insertFeedbackProfileJob(t, db, 1, 7, `{
		"duration_sec": 10,
		"highlight_v1": {
			"selected": {"start_sec": 2, "end_sec": 5, "reason": "scene_change_peak"}
		}
	}`)

	if err := db.Exec(`
		INSERT INTO public.video_image_outputs (id, job_id, metadata)
		VALUES (?, ?, ?)
	`, 101, 1, `{"start_sec":2,"end_sec":5,"reason":"scene_change_peak"}`).Error; err != nil {
		t.Fatalf("insert output: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO public.video_image_feedback (job_id, output_id, user_id, action, weight, scene_tag, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, 1, 101, 7, "top_pick", 3.5, "reaction", `{"source":"unit_test"}`).Error; err != nil {
		t.Fatalf("insert feedback: %v", err)
	}

	p := NewProcessor(db, nil, config.Config{EnableLegacyFeedbackFallback: false})
	profile, err := p.loadUserHighlightFeedbackProfile(7, 80, DefaultQualitySettings())
	if err != nil {
		t.Fatalf("load profile failed: %v", err)
	}
	if profile.EngagedJobs == 0 {
		t.Fatalf("expected strict mode to consume public output feedback")
	}
	if profile.PublicPositiveSignals <= 0 {
		t.Fatalf("expected positive public signals, got %.4f", profile.PublicPositiveSignals)
	}
	if profile.WeightedSignals <= 0 {
		t.Fatalf("expected weighted_signals > 0, got %.4f", profile.WeightedSignals)
	}
	if profile.ReasonPreference["scene_change_peak"] <= 0 {
		t.Fatalf("expected reason preference from output-level feedback, got %+v", profile.ReasonPreference)
	}
}

func openFeedbackProfileTestDB(t *testing.T, withPublicSchema bool) *gorm.DB {
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
	if withPublicSchema {
		if err := db.Exec(`ATTACH DATABASE ':memory:' AS public`).Error; err != nil {
			t.Fatalf("attach public schema: %v", err)
		}
	}
	if err := db.Exec(`
		CREATE TABLE archive.video_jobs (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			metrics TEXT,
			finished_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create archive.video_jobs: %v", err)
	}
	if withPublicSchema {
		if err := db.Exec(`
			CREATE TABLE public.video_image_outputs (
				id INTEGER PRIMARY KEY,
				job_id INTEGER NOT NULL,
				metadata TEXT
			)
		`).Error; err != nil {
			t.Fatalf("create public.video_image_outputs: %v", err)
		}
		if err := db.Exec(`
			CREATE TABLE public.video_image_feedback (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				job_id INTEGER NOT NULL,
				output_id INTEGER,
				user_id INTEGER NOT NULL,
				action TEXT,
				weight REAL,
				scene_tag TEXT,
				metadata TEXT,
				created_at DATETIME
			)
		`).Error; err != nil {
			t.Fatalf("create public.video_image_feedback: %v", err)
		}
	}
	return db
}

func insertFeedbackProfileJob(t *testing.T, db *gorm.DB, jobID uint64, userID uint64, metricsJSON string) {
	t.Helper()
	if err := db.Exec(`
		INSERT INTO archive.video_jobs (id, user_id, status, metrics, finished_at)
		VALUES (?, ?, 'done', ?, CURRENT_TIMESTAMP)
	`, jobID, userID, metricsJSON).Error; err != nil {
		t.Fatalf("insert archive.video_jobs: %v", err)
	}
}
