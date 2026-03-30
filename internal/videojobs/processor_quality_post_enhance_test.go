package videojobs

import (
	"image/color"
	"path/filepath"
	"testing"
)

func TestSceneDiversityOrderKeepAll_KeepCount(t *testing.T) {
	in := []frameQualitySample{
		{Path: "a1", SceneID: 1, QualityScore: 0.90, Index: 1},
		{Path: "a2", SceneID: 1, QualityScore: 0.80, Index: 2},
		{Path: "b1", SceneID: 2, QualityScore: 0.85, Index: 3},
		{Path: "b2", SceneID: 2, QualityScore: 0.70, Index: 4},
		{Path: "c1", SceneID: 3, QualityScore: 0.88, Index: 5},
	}
	got := sceneDiversityOrderKeepAll(in)
	if len(got) != len(in) {
		t.Fatalf("expected keep-all order length %d, got %d", len(in), len(got))
	}
	seen := map[string]struct{}{}
	for _, item := range got {
		if _, ok := seen[item.Path]; ok {
			t.Fatalf("duplicate path in output: %s", item.Path)
		}
		seen[item.Path] = struct{}{}
	}
	for _, item := range in {
		if _, ok := seen[item.Path]; !ok {
			t.Fatalf("missing path in output: %s", item.Path)
		}
	}
}

func TestApplyPNGFinalQualityGuards_DiversityApplied(t *testing.T) {
	tmpDir := t.TempDir()
	paths := []string{
		filepath.Join(tmpDir, "f1.jpg"),
		filepath.Join(tmpDir, "f2.jpg"),
		filepath.Join(tmpDir, "f3.jpg"),
		filepath.Join(tmpDir, "f4.jpg"),
		filepath.Join(tmpDir, "f5.jpg"),
		filepath.Join(tmpDir, "f6.jpg"),
	}
	if err := writeJPEG(paths[0], buildCheckerImage(128, 96)); err != nil {
		t.Fatalf("write f1: %v", err)
	}
	if err := writeJPEG(paths[1], buildCheckerImage(128, 96)); err != nil {
		t.Fatalf("write f2: %v", err)
	}
	if err := writeJPEG(paths[2], buildCheckerImage(128, 96)); err != nil {
		t.Fatalf("write f3: %v", err)
	}
	if err := writeJPEG(paths[3], buildSolidImage(128, 96, color.Gray{Y: 140})); err != nil {
		t.Fatalf("write f4: %v", err)
	}
	if err := writeJPEG(paths[4], buildCheckerImage(128, 96)); err != nil {
		t.Fatalf("write f5: %v", err)
	}
	if err := writeJPEG(paths[5], buildSolidImage(128, 96, color.Gray{Y: 156})); err != nil {
		t.Fatalf("write f6: %v", err)
	}

	t.Setenv("PNG_FINAL_QUALITY_GUARD_ENABLED", "1")
	t.Setenv("PNG_FINAL_QUALITY_GUARD_MIN_KEEP", "3")
	t.Setenv("PNG_FINAL_QUALITY_GUARD_MIN_KEEP_RATIO", "0.5")
	t.Setenv("PNG_FINAL_DIVERSITY_GUARD_ENABLED", "1")
	t.Setenv("PNG_FINAL_DIVERSITY_HAMMING_THRESHOLD", "5")
	t.Setenv("PNG_FINAL_DIVERSITY_MIN_KEEP", "3")
	t.Setenv("PNG_FINAL_DIVERSITY_MIN_KEEP_RATIO", "0.5")

	settings := DefaultQualitySettings()
	settings.MinBrightness = 0
	settings.MaxBrightness = 255
	settings.StillMinExposureScore = 0
	settings.StillMinBlurScore = 1
	out, report := applyPNGFinalQualityGuards(paths, settings, imageAI2Guidance{})
	if len(out) == 0 {
		t.Fatalf("expected non-empty output")
	}
	if !boolFromAny(report["applied"]) {
		t.Fatalf("expected guard applied, report=%+v", report)
	}
	if len(out) >= len(paths) {
		t.Fatalf("expected diversity to reduce count, in=%d out=%d", len(paths), len(out))
	}
}
