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

func TestVideoAIPromptTemplateCache_DetectChangeInvalidatesEntries(t *testing.T) {
	db := openAIPromptTemplateCacheTestDB(t)
	if err := db.Exec(`
		INSERT INTO ops.video_ai_prompt_templates
		(id, format, stage, layer, template_text, enabled, version, is_active, created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 1, "gif", "ai1", "fixed", "v1", true, "v1", true, 1, 1, time.Now(), time.Now()).Error; err != nil {
		t.Fatalf("insert template: %v", err)
	}

	InvalidateVideoAIPromptTemplateCache()
	putCachedVideoAIPromptTemplateSnapshot("gif", "ai1", "fixed", aiPromptTemplateSnapshot{
		Found:   true,
		Format:  "gif",
		Stage:   "ai1",
		Layer:   "fixed",
		Text:    "cached-v1",
		Version: "v1",
		Enabled: true,
		Source:  "ops.video_ai_prompt_templates:gif",
	})
	if _, ok := getCachedVideoAIPromptTemplateSnapshot("gif", "ai1", "fixed"); !ok {
		t.Fatalf("expected cache hit before detect")
	}

	detectVideoAIPromptTemplateChange(db)
	videoAIPromptTemplateRuntimeCache.mu.Lock()
	videoAIPromptTemplateRuntimeCache.lastProbeAt = time.Time{}
	videoAIPromptTemplateRuntimeCache.mu.Unlock()

	if err := db.Exec(`
		UPDATE ops.video_ai_prompt_templates
		SET template_text = ?, updated_at = ?
		WHERE id = ?
	`, "v2", time.Now().Add(2*time.Second), 1).Error; err != nil {
		t.Fatalf("update template: %v", err)
	}
	detectVideoAIPromptTemplateChange(db)

	if _, ok := getCachedVideoAIPromptTemplateSnapshot("gif", "ai1", "fixed"); ok {
		t.Fatalf("expected cache invalidated after table change")
	}
}

func TestLoadAIPromptTemplateWithFallback_ChangeDetectionRefreshesCache(t *testing.T) {
	db := openAIPromptTemplateCacheTestDB(t)
	if err := db.Exec(`
		INSERT INTO ops.video_ai_prompt_templates
		(id, format, stage, layer, template_text, enabled, version, is_active, created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 11, "gif", "ai1", "fixed", "first", true, "v1", true, 1, 1, time.Now(), time.Now()).Error; err != nil {
		t.Fatalf("insert template: %v", err)
	}

	InvalidateVideoAIPromptTemplateCache()
	p := NewProcessor(db, nil, config.Config{})
	first, err := p.loadAIPromptTemplateWithFallback("gif", "ai1", "fixed")
	if err != nil {
		t.Fatalf("loadAIPromptTemplateWithFallback first failed: %v", err)
	}
	if got := strings.TrimSpace(first.Text); got != "first" {
		t.Fatalf("expected first text, got %q", got)
	}

	videoAIPromptTemplateRuntimeCache.mu.Lock()
	videoAIPromptTemplateRuntimeCache.lastProbeAt = time.Time{}
	videoAIPromptTemplateRuntimeCache.mu.Unlock()
	if err := db.Exec(`
		UPDATE ops.video_ai_prompt_templates
		SET template_text = ?, updated_at = ?
		WHERE id = ?
	`, "second", time.Now().Add(3*time.Second), 11).Error; err != nil {
		t.Fatalf("update template: %v", err)
	}

	second, err := p.loadAIPromptTemplateWithFallback("gif", "ai1", "fixed")
	if err != nil {
		t.Fatalf("loadAIPromptTemplateWithFallback second failed: %v", err)
	}
	if got := strings.TrimSpace(second.Text); got != "second" {
		t.Fatalf("expected updated text second, got %q", got)
	}
}

func TestLoadAIPromptTemplateWithFallback_StageDefaultPrecedence(t *testing.T) {
	db := openAIPromptTemplateCacheTestDB(t)
	now := time.Now()
	rows := []struct {
		id      int
		format  string
		stage   string
		layer   string
		text    string
		version string
	}{
		{id: 21, format: "all", stage: "ai2", layer: "fixed", text: "all-ai2", version: "all-ai2-v1"},
		{id: 22, format: "all", stage: "default", layer: "fixed", text: "all-default", version: "all-default-v1"},
		{id: 23, format: "png", stage: "default", layer: "fixed", text: "png-default", version: "png-default-v1"},
	}
	for _, row := range rows {
		if err := db.Exec(`
			INSERT INTO ops.video_ai_prompt_templates
			(id, format, stage, layer, template_text, enabled, version, is_active, created_by, updated_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, row.id, row.format, row.stage, row.layer, row.text, true, row.version, true, 1, 1, now, now).Error; err != nil {
			t.Fatalf("insert template %d failed: %v", row.id, err)
		}
	}

	InvalidateVideoAIPromptTemplateCache()
	p := NewProcessor(db, nil, config.Config{})

	pngAI2, err := p.loadAIPromptTemplateWithFallback("png", "ai2", "fixed")
	if err != nil {
		t.Fatalf("load png/ai2 failed: %v", err)
	}
	if !pngAI2.Found {
		t.Fatalf("expected png/ai2 fallback hit")
	}
	if got := strings.TrimSpace(pngAI2.Text); got != "png-default" {
		t.Fatalf("png/ai2 should resolve png/default first, got %q", got)
	}
	if got := strings.TrimSpace(pngAI2.Source); got != "ops.video_ai_prompt_templates:png/default" {
		t.Fatalf("unexpected source: %q", got)
	}

	gifAI2, err := p.loadAIPromptTemplateWithFallback("gif", "ai2", "fixed")
	if err != nil {
		t.Fatalf("load gif/ai2 failed: %v", err)
	}
	if !gifAI2.Found {
		t.Fatalf("expected gif/ai2 fallback hit")
	}
	if got := strings.TrimSpace(gifAI2.Text); got != "all-ai2" {
		t.Fatalf("gif/ai2 should resolve all/ai2, got %q", got)
	}
	if got := strings.TrimSpace(gifAI2.Source); got != "ops.video_ai_prompt_templates:all/ai2" {
		t.Fatalf("unexpected source: %q", got)
	}

	gifAI3, err := p.loadAIPromptTemplateWithFallback("gif", "ai3", "fixed")
	if err != nil {
		t.Fatalf("load gif/ai3 failed: %v", err)
	}
	if !gifAI3.Found {
		t.Fatalf("expected gif/ai3 fallback hit")
	}
	if got := strings.TrimSpace(gifAI3.Text); got != "all-default" {
		t.Fatalf("gif/ai3 should resolve all/default, got %q", got)
	}
	if got := strings.TrimSpace(gifAI3.Source); got != "ops.video_ai_prompt_templates:all/default" {
		t.Fatalf("unexpected source: %q", got)
	}
}

func openAIPromptTemplateCacheTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsnName := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_")
	dsnName = strings.ReplaceAll(dsnName, " ", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", dsnName)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.Exec(`ATTACH DATABASE ':memory:' AS ops`).Error; err != nil {
		t.Fatalf("attach ops schema: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE ops.video_ai_prompt_templates (
			id INTEGER PRIMARY KEY,
			format TEXT,
			stage TEXT,
			layer TEXT,
			template_text TEXT,
			template_json_schema TEXT,
			enabled BOOLEAN,
			version TEXT,
			is_active BOOLEAN,
			created_by INTEGER,
			updated_by INTEGER,
			metadata TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create ops.video_ai_prompt_templates: %v", err)
	}
	return db
}
