package videojobs

import (
	"fmt"
	"strings"
	"testing"

	"emoji/internal/models"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestExtractGIFLoopTuneSignals(t *testing.T) {
	raw := datatypes.JSON([]byte(`{
	  "format": "gif",
	  "gif_loop_tune": {
	    "applied": true,
	    "effective_applied": true,
	    "fallback_to_base": false,
	    "score": 0.82,
	    "loop_closure": 0.91,
	    "motion_mean": 0.23,
	    "effective_sec": 2.4
	  }
	}`))
	s := extractGIFLoopTuneSignals(raw, "gif", "main")
	if !s.Applied || !s.EffectiveApplied {
		t.Fatalf("expected applied and effective applied true, got %+v", s)
	}
	if s.FallbackToBase {
		t.Fatalf("expected fallback false, got %+v", s)
	}
	if s.LoopClosure != 0.91 {
		t.Fatalf("expected loop closure 0.91, got %.2f", s.LoopClosure)
	}
	if s.EffectiveSec != 2.4 {
		t.Fatalf("expected effective sec 2.4, got %.2f", s.EffectiveSec)
	}
}

func TestExtractGIFLoopTuneSignals_FallbackDurationAndGuard(t *testing.T) {
	raw := datatypes.JSON([]byte(`{
	  "gif_loop_tune": {
	    "applied": "true",
	    "fallback_to_base": "1",
	    "duration_sec": 2.1
	  }
	}`))
	s := extractGIFLoopTuneSignals(raw, "gif", "main")
	if !s.Applied {
		t.Fatalf("expected applied true, got %+v", s)
	}
	if !s.FallbackToBase {
		t.Fatalf("expected fallback true, got %+v", s)
	}
	if s.EffectiveSec != 2.1 {
		t.Fatalf("expected effective sec fallback to duration_sec=2.1, got %.2f", s.EffectiveSec)
	}

	nonGIF := extractGIFLoopTuneSignals(raw, "webp", "main")
	if nonGIF.Applied || nonGIF.EffectiveSec != 0 {
		t.Fatalf("expected non gif zero result, got %+v", nonGIF)
	}

	nonMain := extractGIFLoopTuneSignals(raw, "gif", "cover")
	if nonMain.Applied || nonMain.EffectiveSec != 0 {
		t.Fatalf("expected non-main zero result, got %+v", nonMain)
	}
}

func TestResolveArtifactOutputProposalID_DirectID(t *testing.T) {
	proposalID, err := resolveArtifactOutputProposalID(nil, 101, datatypes.JSON([]byte(`{"proposal_id": 88}`)))
	if err != nil {
		t.Fatalf("resolve direct proposal id returned error: %v", err)
	}
	if proposalID == nil || *proposalID != 88 {
		t.Fatalf("expected proposal_id=88, got %+v", proposalID)
	}
}

func TestResolveArtifactOutputProposalID_ByRank(t *testing.T) {
	db := openPublicSyncTestDB(t)
	if err := db.Exec(`
		CREATE TABLE archive.video_job_gif_ai_proposals (
			id INTEGER PRIMARY KEY,
			job_id INTEGER NOT NULL,
			proposal_rank INTEGER NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("create proposals table: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO archive.video_job_gif_ai_proposals (id, job_id, proposal_rank)
		VALUES (11, 7, 2)
	`).Error; err != nil {
		t.Fatalf("insert proposal row: %v", err)
	}

	proposalID, err := resolveArtifactOutputProposalID(db, 7, datatypes.JSON([]byte(`{"proposal_rank": 2}`)))
	if err != nil {
		t.Fatalf("resolve proposal by rank returned error: %v", err)
	}
	if proposalID == nil || *proposalID != 11 {
		t.Fatalf("expected proposal_id=11, got %+v", proposalID)
	}
}

func TestUpsertPublicVideoImageJob_WritesSplitTableOnly(t *testing.T) {
	db := openPublicSyncTestDB(t)
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

	legacy := models.VideoJob{
		ID:             101,
		UserID:         2001,
		Title:          "png task",
		SourceVideoKey: "videos/in/test.mp4",
		OutputFormats:  "png",
		Status:         "done",
		Stage:          "completed",
		Progress:       100,
	}
	if err := UpsertPublicVideoImageJob(db, legacy); err != nil {
		t.Fatalf("upsert public job: %v", err)
	}

	var splitCount int64
	if err := db.Table("public.video_image_jobs_png").Where("id = ?", legacy.ID).Count(&splitCount).Error; err != nil {
		t.Fatalf("count split rows: %v", err)
	}
	if splitCount != 1 {
		t.Fatalf("expected split rows=1, got %d", splitCount)
	}
	var baseCount int64
	if err := db.Table("public.video_image_jobs").Where("id = ?", legacy.ID).Count(&baseCount).Error; err != nil {
		t.Fatalf("count base rows: %v", err)
	}
	if baseCount != 0 {
		t.Fatalf("expected base rows=0, got %d", baseCount)
	}
}

func TestUpsertPublicVideoImageJob_FallbackToBaseWhenSplitMissing(t *testing.T) {
	db := openPublicSyncTestDB(t)
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

	legacy := models.VideoJob{
		ID:             1001,
		UserID:         2001,
		Title:          "png fallback task",
		SourceVideoKey: "videos/in/fallback.mp4",
		OutputFormats:  "png",
		Status:         "queued",
		Stage:          "queued",
		Progress:       0,
	}
	if err := UpsertPublicVideoImageJob(db, legacy); err != nil {
		t.Fatalf("upsert public job fallback to base failed: %v", err)
	}

	var baseCount int64
	if err := db.Table("public.video_image_jobs").Where("id = ?", legacy.ID).Count(&baseCount).Error; err != nil {
		t.Fatalf("count base rows: %v", err)
	}
	if baseCount != 1 {
		t.Fatalf("expected base rows=1 after fallback, got %d", baseCount)
	}
}

func TestCreatePublicVideoImageEvent_WritesSplitTableOnly(t *testing.T) {
	db := openPublicSyncTestDB(t)
	if err := db.Exec(`CREATE TABLE public.video_image_jobs_png (
		id INTEGER PRIMARY KEY,
		requested_format TEXT
	)`).Error; err != nil {
		t.Fatalf("create split jobs table: %v", err)
	}
	if err := db.Exec(`INSERT INTO public.video_image_jobs_png (id, requested_format) VALUES (212, 'png')`).Error; err != nil {
		t.Fatalf("insert split job row: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER,
		level TEXT,
		stage TEXT,
		message TEXT,
		metadata TEXT,
		created_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create base events table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_events_png (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER,
		level TEXT,
		stage TEXT,
		message TEXT,
		metadata TEXT,
		created_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create split events table: %v", err)
	}

	if err := CreatePublicVideoImageEvent(db, models.VideoJobEvent{
		JobID:   212,
		Level:   "info",
		Stage:   "ai1",
		Message: "png event",
	}); err != nil {
		t.Fatalf("create public event: %v", err)
	}

	var splitCount int64
	if err := db.Table("public.video_image_events_png").Where("job_id = 212").Count(&splitCount).Error; err != nil {
		t.Fatalf("count split events: %v", err)
	}
	if splitCount != 1 {
		t.Fatalf("expected split events=1, got %d", splitCount)
	}
	var baseCount int64
	if err := db.Table("public.video_image_events").Where("job_id = 212").Count(&baseCount).Error; err != nil {
		t.Fatalf("count base events: %v", err)
	}
	if baseCount != 0 {
		t.Fatalf("expected base events=0, got %d", baseCount)
	}
}

func TestCreatePublicVideoImageEvent_FallbackToBaseWhenSplitMissing(t *testing.T) {
	db := openPublicSyncTestDB(t)
	if err := db.Exec(`CREATE TABLE archive.video_jobs (
		id INTEGER PRIMARY KEY,
		user_id INTEGER,
		output_formats TEXT
	)`).Error; err != nil {
		t.Fatalf("create legacy jobs table: %v", err)
	}
	if err := db.Exec(`INSERT INTO archive.video_jobs (id, user_id, output_formats) VALUES (988, 7, 'png')`).Error; err != nil {
		t.Fatalf("insert legacy job: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER,
		level TEXT,
		stage TEXT,
		message TEXT,
		metadata TEXT,
		created_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create base events table: %v", err)
	}

	if err := CreatePublicVideoImageEvent(db, models.VideoJobEvent{
		JobID:   988,
		Level:   "info",
		Stage:   "queued",
		Message: "fallback event",
	}); err != nil {
		t.Fatalf("create public event fallback failed: %v", err)
	}

	var baseCount int64
	if err := db.Table("public.video_image_events").Where("job_id = 988").Count(&baseCount).Error; err != nil {
		t.Fatalf("count base events: %v", err)
	}
	if baseCount != 1 {
		t.Fatalf("expected base events=1 after fallback, got %d", baseCount)
	}
}

func TestSyncPublicVideoImageJobUpdates_FallbackToBaseWhenSplitMissing(t *testing.T) {
	db := openPublicSyncTestDB(t)
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
	if err := db.Exec(`INSERT INTO public.video_image_jobs
		(id, user_id, requested_format, status, stage, progress, options, metrics)
		VALUES (777, 99, 'png', 'queued', 'queued', 0, '{}', '{}')`).Error; err != nil {
		t.Fatalf("insert base job row: %v", err)
	}

	if err := SyncPublicVideoImageJobUpdates(db, 777, map[string]interface{}{
		"status":   "processing",
		"stage":    "analyzing",
		"progress": 35,
	}); err != nil {
		t.Fatalf("sync updates fallback failed: %v", err)
	}

	var row struct {
		Status   string `gorm:"column:status"`
		Stage    string `gorm:"column:stage"`
		Progress int    `gorm:"column:progress"`
	}
	if err := db.Table("public.video_image_jobs").Select("status, stage, progress").Where("id = 777").First(&row).Error; err != nil {
		t.Fatalf("query base job row: %v", err)
	}
	if row.Status != "processing" || row.Stage != "analyzing" || row.Progress != 35 {
		t.Fatalf("unexpected base row after fallback update: %+v", row)
	}
}

func TestCreatePublicVideoImageFeedback_WritesSplitTableOnly(t *testing.T) {
	db := openPublicSyncTestDB(t)
	if err := db.Exec(`CREATE TABLE public.video_image_jobs_png (
		id INTEGER PRIMARY KEY,
		requested_format TEXT
	)`).Error; err != nil {
		t.Fatalf("create split jobs table: %v", err)
	}
	if err := db.Exec(`INSERT INTO public.video_image_jobs_png (id, requested_format) VALUES (313, 'png')`).Error; err != nil {
		t.Fatalf("insert split job row: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_feedback (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER,
		output_id INTEGER,
		user_id INTEGER,
		action TEXT,
		weight REAL,
		scene_tag TEXT,
		metadata TEXT,
		created_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create base feedback table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_feedback_png (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER,
		output_id INTEGER,
		user_id INTEGER,
		action TEXT,
		weight REAL,
		scene_tag TEXT,
		metadata TEXT,
		created_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create split feedback table: %v", err)
	}

	if err := CreatePublicVideoImageFeedback(db, models.VideoImageFeedbackPublic{
		JobID:    313,
		UserID:   9001,
		Action:   "like",
		Weight:   1,
		SceneTag: "png",
	}); err != nil {
		t.Fatalf("create public feedback: %v", err)
	}

	var splitCount int64
	if err := db.Table("public.video_image_feedback_png").Where("job_id = 313").Count(&splitCount).Error; err != nil {
		t.Fatalf("count split feedback: %v", err)
	}
	if splitCount != 1 {
		t.Fatalf("expected split feedback=1, got %d", splitCount)
	}
	var baseCount int64
	if err := db.Table("public.video_image_feedback").Where("job_id = 313").Count(&baseCount).Error; err != nil {
		t.Fatalf("count base feedback: %v", err)
	}
	if baseCount != 0 {
		t.Fatalf("expected base feedback=0, got %d", baseCount)
	}
}

func TestUpsertPublicVideoImageOutputByArtifact_WritesSplitOutputAndPackageOnly(t *testing.T) {
	db := openPublicSyncTestDB(t)
	if err := db.Exec(`CREATE TABLE archive.video_jobs (
		id INTEGER PRIMARY KEY,
		user_id INTEGER,
		output_formats TEXT
	)`).Error; err != nil {
		t.Fatalf("create legacy jobs table: %v", err)
	}
	if err := db.Exec(`INSERT INTO archive.video_jobs (id, user_id, output_formats) VALUES (515, 9001, 'png')`).Error; err != nil {
		t.Fatalf("insert legacy job: %v", err)
	}

	if err := db.Exec(`CREATE TABLE public.video_image_outputs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER,
		user_id INTEGER,
		format TEXT,
		file_role TEXT,
		object_key TEXT NOT NULL UNIQUE,
		bucket TEXT,
		mime_type TEXT,
		size_bytes INTEGER,
		width INTEGER,
		height INTEGER,
		duration_ms INTEGER,
		frame_index INTEGER,
		proposal_id INTEGER,
		score REAL,
		gif_loop_tune_applied BOOLEAN,
		gif_loop_tune_effective_applied BOOLEAN,
		gif_loop_tune_fallback_to_base BOOLEAN,
		gif_loop_tune_score REAL,
		gif_loop_tune_loop_closure REAL,
		gif_loop_tune_motion_mean REAL,
		gif_loop_tune_effective_sec REAL,
		sha256 TEXT,
		is_primary BOOLEAN,
		metadata TEXT,
		created_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create base outputs table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_outputs_png (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER,
		user_id INTEGER,
		format TEXT,
		file_role TEXT,
		object_key TEXT NOT NULL UNIQUE,
		bucket TEXT,
		mime_type TEXT,
		size_bytes INTEGER,
		width INTEGER,
		height INTEGER,
		duration_ms INTEGER,
		frame_index INTEGER,
		proposal_id INTEGER,
		score REAL,
		gif_loop_tune_applied BOOLEAN,
		gif_loop_tune_effective_applied BOOLEAN,
		gif_loop_tune_fallback_to_base BOOLEAN,
		gif_loop_tune_score REAL,
		gif_loop_tune_loop_closure REAL,
		gif_loop_tune_motion_mean REAL,
		gif_loop_tune_effective_sec REAL,
		sha256 TEXT,
		is_primary BOOLEAN,
		metadata TEXT,
		created_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create split outputs table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_packages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER UNIQUE,
		user_id INTEGER,
		zip_object_key TEXT,
		zip_name TEXT,
		zip_size_bytes INTEGER,
		file_count INTEGER,
		manifest TEXT,
		expires_at DATETIME,
		created_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create base packages table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_packages_png (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER UNIQUE,
		user_id INTEGER,
		zip_object_key TEXT,
		zip_name TEXT,
		zip_size_bytes INTEGER,
		file_count INTEGER,
		manifest TEXT,
		expires_at DATETIME,
		created_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create split packages table: %v", err)
	}

	if err := UpsertPublicVideoImageOutputByArtifact(db, models.VideoJobArtifact{
		JobID:     515,
		Type:      "frame",
		QiniuKey:  "emoji/video-image/prod/f/png/u/01/9001/j/515/outputs/png/0001.png",
		MimeType:  "image/png",
		SizeBytes: 1234,
		Width:     720,
		Height:    1280,
		Metadata:  datatypes.JSON([]byte(`{"format":"png","score":0.83}`)),
	}); err != nil {
		t.Fatalf("upsert public output: %v", err)
	}
	if err := UpsertPublicVideoImageOutputByArtifact(db, models.VideoJobArtifact{
		JobID:     515,
		Type:      "package",
		QiniuKey:  "emoji/video-image/prod/f/png/u/01/9001/j/515/package/515_png_v1.zip",
		MimeType:  "application/zip",
		SizeBytes: 9988,
		Metadata:  datatypes.JSON([]byte(`{"format":"zip"}`)),
	}); err != nil {
		t.Fatalf("upsert public package artifact: %v", err)
	}

	var splitOutputCount int64
	if err := db.Table("public.video_image_outputs_png").Where("job_id = 515").Count(&splitOutputCount).Error; err != nil {
		t.Fatalf("count split outputs: %v", err)
	}
	if splitOutputCount == 0 {
		t.Fatalf("expected split outputs > 0, got %d", splitOutputCount)
	}
	var baseOutputCount int64
	if err := db.Table("public.video_image_outputs").Where("job_id = 515").Count(&baseOutputCount).Error; err != nil {
		t.Fatalf("count base outputs: %v", err)
	}
	if baseOutputCount != 0 {
		t.Fatalf("expected base outputs=0, got %d", baseOutputCount)
	}

	var splitPackageCount int64
	if err := db.Table("public.video_image_packages_png").Where("job_id = 515").Count(&splitPackageCount).Error; err != nil {
		t.Fatalf("count split packages: %v", err)
	}
	if splitPackageCount != 1 {
		t.Fatalf("expected split packages=1, got %d", splitPackageCount)
	}
	var basePackageCount int64
	if err := db.Table("public.video_image_packages").Where("job_id = 515").Count(&basePackageCount).Error; err != nil {
		t.Fatalf("count base packages: %v", err)
	}
	if basePackageCount != 0 {
		t.Fatalf("expected base packages=0, got %d", basePackageCount)
	}
}

func TestResolvePublicVideoImageRequestedFormat_PrefersLegacyFormatOverBaseTable(t *testing.T) {
	db := openPublicSyncTestDB(t)
	if err := db.Exec(`CREATE TABLE archive.video_jobs (
		id INTEGER PRIMARY KEY,
		output_formats TEXT
	)`).Error; err != nil {
		t.Fatalf("create legacy jobs table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_jobs (
		id INTEGER PRIMARY KEY,
		requested_format TEXT
	)`).Error; err != nil {
		t.Fatalf("create base public jobs table: %v", err)
	}
	if err := db.Exec(`INSERT INTO archive.video_jobs (id, output_formats) VALUES (616, 'png')`).Error; err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if err := db.Exec(`INSERT INTO public.video_image_jobs (id, requested_format) VALUES (616, 'gif')`).Error; err != nil {
		t.Fatalf("insert base row: %v", err)
	}

	got := resolvePublicVideoImageRequestedFormat(db, 616)
	if got != "png" {
		t.Fatalf("expected format png, got %q", got)
	}
}

func TestResolvePublicVideoImageRequestedFormat_PrefersSplitTableOverLegacyAndBase(t *testing.T) {
	db := openPublicSyncTestDB(t)
	if err := db.Exec(`CREATE TABLE archive.video_jobs (
		id INTEGER PRIMARY KEY,
		output_formats TEXT
	)`).Error; err != nil {
		t.Fatalf("create legacy jobs table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_jobs (
		id INTEGER PRIMARY KEY,
		requested_format TEXT
	)`).Error; err != nil {
		t.Fatalf("create base jobs table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_jobs_png (
		id INTEGER PRIMARY KEY,
		requested_format TEXT
	)`).Error; err != nil {
		t.Fatalf("create png split jobs table: %v", err)
	}
	if err := db.Exec(`INSERT INTO archive.video_jobs (id, output_formats) VALUES (917, 'gif')`).Error; err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if err := db.Exec(`INSERT INTO public.video_image_jobs (id, requested_format) VALUES (917, 'webp')`).Error; err != nil {
		t.Fatalf("insert base row: %v", err)
	}
	if err := db.Exec(`INSERT INTO public.video_image_jobs_png (id, requested_format) VALUES (917, 'png')`).Error; err != nil {
		t.Fatalf("insert split row: %v", err)
	}

	if got := resolvePublicVideoImageRequestedFormat(db, 917); got != "png" {
		t.Fatalf("expected format png from split table, got %q", got)
	}
}

func TestUpsertPublicVideoImageOutputByArtifact_PrefersSplitRequestedFormatWhenLegacyDrifted(t *testing.T) {
	db := openPublicSyncTestDB(t)
	if err := db.Exec(`CREATE TABLE archive.video_jobs (
		id INTEGER PRIMARY KEY,
		user_id INTEGER,
		output_formats TEXT
	)`).Error; err != nil {
		t.Fatalf("create legacy jobs table: %v", err)
	}
	if err := db.Exec(`INSERT INTO archive.video_jobs (id, user_id, output_formats) VALUES (818, 9001, 'gif')`).Error; err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if err := db.Exec(`CREATE TABLE public.video_image_jobs_png (
		id INTEGER PRIMARY KEY,
		requested_format TEXT
	)`).Error; err != nil {
		t.Fatalf("create split jobs table: %v", err)
	}
	if err := db.Exec(`INSERT INTO public.video_image_jobs_png (id, requested_format) VALUES (818, 'png')`).Error; err != nil {
		t.Fatalf("insert split job row: %v", err)
	}

	createOutputTable := func(tableName string) error {
		return db.Exec(`CREATE TABLE ` + tableName + ` (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id INTEGER,
			user_id INTEGER,
			format TEXT,
			file_role TEXT,
			object_key TEXT NOT NULL UNIQUE,
			bucket TEXT,
			mime_type TEXT,
			size_bytes INTEGER,
			width INTEGER,
			height INTEGER,
			duration_ms INTEGER,
			frame_index INTEGER,
			proposal_id INTEGER,
			score REAL,
			gif_loop_tune_applied BOOLEAN,
			gif_loop_tune_effective_applied BOOLEAN,
			gif_loop_tune_fallback_to_base BOOLEAN,
			gif_loop_tune_score REAL,
			gif_loop_tune_loop_closure REAL,
			gif_loop_tune_motion_mean REAL,
			gif_loop_tune_effective_sec REAL,
			sha256 TEXT,
			is_primary BOOLEAN,
			metadata TEXT,
			created_at DATETIME
		)`).Error
	}
	for _, tableName := range []string{
		"public.video_image_outputs",
		"public.video_image_outputs_png",
		"public.video_image_outputs_gif",
	} {
		if err := createOutputTable(tableName); err != nil {
			t.Fatalf("create output table %s: %v", tableName, err)
		}
	}
	createPackageTable := func(tableName string) error {
		return db.Exec(`CREATE TABLE ` + tableName + ` (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id INTEGER UNIQUE,
			user_id INTEGER,
			zip_object_key TEXT,
			zip_name TEXT,
			zip_size_bytes INTEGER,
			file_count INTEGER,
			manifest TEXT,
			expires_at DATETIME,
			created_at DATETIME
		)`).Error
	}
	for _, tableName := range []string{
		"public.video_image_packages",
		"public.video_image_packages_png",
		"public.video_image_packages_gif",
	} {
		if err := createPackageTable(tableName); err != nil {
			t.Fatalf("create package table %s: %v", tableName, err)
		}
	}

	if err := UpsertPublicVideoImageOutputByArtifact(db, models.VideoJobArtifact{
		JobID:     818,
		Type:      "frame",
		QiniuKey:  "emoji/video-image/prod/f/png/u/01/9001/j/818/outputs/png/0001.png",
		MimeType:  "image/png",
		SizeBytes: 4321,
		Width:     720,
		Height:    1280,
		Metadata:  datatypes.JSON([]byte(`{"format":"png","score":0.77}`)),
	}); err != nil {
		t.Fatalf("upsert png frame artifact: %v", err)
	}
	if err := UpsertPublicVideoImageOutputByArtifact(db, models.VideoJobArtifact{
		JobID:     818,
		Type:      "package",
		QiniuKey:  "emoji/video-image/prod/f/png/u/01/9001/j/818/package/818_png_v1.zip",
		MimeType:  "application/zip",
		SizeBytes: 12000,
		Metadata:  datatypes.JSON([]byte(`{"format":"zip"}`)),
	}); err != nil {
		t.Fatalf("upsert png package artifact: %v", err)
	}

	assertCount := func(table string, expected int64) {
		t.Helper()
		var count int64
		if err := db.Table(table).Where("job_id = 818").Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != expected {
			t.Fatalf("expected %s count=%d, got %d", table, expected, count)
		}
	}
	assertCount("public.video_image_outputs_png", 2)
	assertCount("public.video_image_outputs_gif", 0)
	assertCount("public.video_image_outputs", 0)
	assertCount("public.video_image_packages_png", 1)
	assertCount("public.video_image_packages_gif", 0)
	assertCount("public.video_image_packages", 0)
}

func openPublicSyncTestDB(t *testing.T) *gorm.DB {
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
