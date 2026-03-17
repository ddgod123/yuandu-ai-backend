package videojobs

import "testing"

func TestResolveGIFRegressionBaseWindow(t *testing.T) {
	meta := videoProbeMeta{DurationSec: 12, Width: 1920, Height: 1080, FPS: 30}
	window := resolveGIFRegressionBaseWindow(meta, 3)
	if window.EndSec-window.StartSec < 2.9 || window.EndSec-window.StartSec > 3.1 {
		t.Fatalf("expected ~3s window, got %.3f", window.EndSec-window.StartSec)
	}
	if window.StartSec < 4.4 || window.StartSec > 4.6 {
		t.Fatalf("expected centered start around 4.5, got %.3f", window.StartSec)
	}

	shortMeta := videoProbeMeta{DurationSec: 1.5, Width: 640, Height: 360}
	shortWindow := resolveGIFRegressionBaseWindow(shortMeta, 3)
	if shortWindow.StartSec != 0 {
		t.Fatalf("expected short window starts at 0, got %.3f", shortWindow.StartSec)
	}
	if shortWindow.EndSec != 1.5 {
		t.Fatalf("expected short window ends at duration, got %.3f", shortWindow.EndSec)
	}
}

func TestNormalizeGIFLoopRegressionOptions(t *testing.T) {
	out := normalizeGIFLoopRegressionOptions(GIFLoopRegressionOptions{})
	if out.PreferWindowSec <= 0 {
		t.Fatalf("expected default prefer window >0, got %.2f", out.PreferWindowSec)
	}
	if out.UseHighlight {
		t.Fatalf("expected default UseHighlight false when unset")
	}

	custom := normalizeGIFLoopRegressionOptions(GIFLoopRegressionOptions{
		PreferWindowSec: 9,
		UseHighlight:    true,
		RenderOutputs:   true,
	})
	if custom.PreferWindowSec != 3.4 {
		t.Fatalf("expected prefer window clamped to 3.4, got %.2f", custom.PreferWindowSec)
	}
	if !custom.UseHighlight || !custom.RenderOutputs {
		t.Fatalf("expected true flags kept, got %+v", custom)
	}
}
