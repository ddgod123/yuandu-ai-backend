package videojobs

import (
	"fmt"
	"strings"
	"testing"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

func openPublicSyncTestDB(t *testing.T) *gorm.DB {
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
