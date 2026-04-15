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

func TestLoadQualitySettingsByFormat_MergesScopedOverride(t *testing.T) {
	db := openQualitySettingsScopeTestDB(t)
	now := time.Now()
	if err := db.Exec(`
		INSERT INTO ops.video_quality_settings
		(id, png_profile, still_min_width, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, 1, "clarity", 100, now, now).Error; err != nil {
		t.Fatalf("insert base quality setting failed: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO ops.video_quality_settings_scoped
		(id, format, version, is_active, settings_json, created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 1, "png", "png_v2", true, `{"png_profile":"size","still_min_width":180}`, 1, 1, now, now).Error; err != nil {
		t.Fatalf("insert scoped quality setting failed: %v", err)
	}

	p := NewProcessor(db, nil, config.Config{})
	settings, meta := p.loadQualitySettingsByFormat("png")

	if got := settings.PNGProfile; got != QualityProfileSize {
		t.Fatalf("png profile should resolve from scope, got=%q", got)
	}
	if got := settings.StillMinWidth; got != 180 {
		t.Fatalf("still_min_width should resolve from scope, got=%d", got)
	}

	if got := stringFromAny(meta["resolved_format"]); got != "png" {
		t.Fatalf("resolved_format mismatch, got=%q", got)
	}
	resolvedFrom := stringSliceFromAny(meta["resolved_from"])
	if len(resolvedFrom) != 2 || resolvedFrom[0] != "all" || resolvedFrom[1] != "png" {
		t.Fatalf("resolved_from mismatch, got=%v", resolvedFrom)
	}
	overrideKeys := stringSliceFromAny(meta["override_keys"])
	if len(overrideKeys) != 2 || overrideKeys[0] != "png_profile" || overrideKeys[1] != "still_min_width" {
		t.Fatalf("override_keys mismatch, got=%v", overrideKeys)
	}
	if boolFromAny(meta["fallback_used"]) {
		t.Fatalf("fallback_used should be false when scoped setting exists")
	}
}

func TestLoadQualitySettingsByFormat_FallbackToAll(t *testing.T) {
	db := openQualitySettingsScopeTestDB(t)
	now := time.Now()
	if err := db.Exec(`
		INSERT INTO ops.video_quality_settings
		(id, gif_profile, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, 1, "size", now, now).Error; err != nil {
		t.Fatalf("insert base quality setting failed: %v", err)
	}

	p := NewProcessor(db, nil, config.Config{})
	settings, meta := p.loadQualitySettingsByFormat("gif")

	if got := settings.GIFProfile; got != QualityProfileSize {
		t.Fatalf("gif profile should come from base setting, got=%q", got)
	}
	if got := stringFromAny(meta["resolved_format"]); got != "all" {
		t.Fatalf("resolved_format mismatch, got=%q", got)
	}
	resolvedFrom := stringSliceFromAny(meta["resolved_from"])
	if len(resolvedFrom) != 1 || resolvedFrom[0] != "all" {
		t.Fatalf("resolved_from mismatch, got=%v", resolvedFrom)
	}
	if !boolFromAny(meta["fallback_used"]) {
		t.Fatalf("fallback_used should be true when scoped setting missing")
	}
}

func openQualitySettingsScopeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsnName := fmt.Sprintf("%s_quality_scope", stringsReplaceAllSafe(t.Name()))
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", dsnName)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.Exec(`ATTACH DATABASE ':memory:' AS ops`).Error; err != nil {
		t.Fatalf("attach ops schema: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE ops.video_quality_settings (
			id INTEGER PRIMARY KEY,
			gif_profile TEXT,
			png_profile TEXT,
			still_min_width INTEGER,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create ops.video_quality_settings failed: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE ops.video_quality_settings_scoped (
			id INTEGER PRIMARY KEY,
			format TEXT,
			version TEXT,
			is_active BOOLEAN,
			settings_json TEXT,
			created_by INTEGER,
			updated_by INTEGER,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create ops.video_quality_settings_scoped failed: %v", err)
	}
	return db
}

func stringsReplaceAllSafe(input string) string {
	replaced := input
	replaced = strings.ReplaceAll(replaced, "/", "_")
	replaced = strings.ReplaceAll(replaced, " ", "_")
	return strings.ToLower(replaced)
}
