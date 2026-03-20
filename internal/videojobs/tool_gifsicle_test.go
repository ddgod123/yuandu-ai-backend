package videojobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptimizeGIFWithGifsicle_Disabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.gif")
	if err := os.WriteFile(path, []byte("GIF89a"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	settings := DefaultQualitySettings()
	settings.GIFGifsicleEnabled = false

	got := optimizeGIFWithGifsicle(context.Background(), path, settings, "")
	if got.Applied {
		t.Fatalf("expected not applied when disabled")
	}
	if got.Attempted {
		t.Fatalf("expected not attempted when disabled")
	}
	if got.Reason != "disabled" {
		t.Fatalf("expected reason disabled, got %q", got.Reason)
	}
}

func TestOptimizeGIFWithGifsicle_SkipSmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.gif")
	if err := os.WriteFile(path, []byte("GIF89a"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	settings := DefaultQualitySettings()
	settings.GIFGifsicleEnabled = true
	settings.GIFGifsicleSkipBelowKB = 1

	got := optimizeGIFWithGifsicle(context.Background(), path, settings, "")
	if got.Applied {
		t.Fatalf("expected small file skip")
	}
	if got.Attempted {
		t.Fatalf("expected not attempted for skip_below_threshold")
	}
	if got.Reason != "skip_below_threshold" {
		t.Fatalf("expected reason skip_below_threshold, got %q", got.Reason)
	}
}

func TestOptimizeGIFWithGifsicle_ToolUnavailable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.gif")
	if err := os.WriteFile(path, []byte("GIF89a"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	settings := DefaultQualitySettings()
	settings.GIFGifsicleEnabled = true
	settings.GIFGifsicleSkipBelowKB = 0

	got := optimizeGIFWithGifsicle(context.Background(), path, settings, filepath.Join(dir, "missing-gifsicle"))
	if got.Applied {
		t.Fatalf("expected not applied when tool unavailable")
	}
	if got.Attempted {
		t.Fatalf("expected not attempted when tool unavailable")
	}
	if got.Reason != "tool_unavailable" {
		t.Fatalf("expected reason tool_unavailable, got %q", got.Reason)
	}
	if !strings.Contains(got.Error, "not found") && got.Error == "" {
		t.Fatalf("expected tool unavailable error, got %q", got.Error)
	}
}
