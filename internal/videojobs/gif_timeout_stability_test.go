package videojobs

import "testing"

func TestApplyGIFEmergencyFallbackProfile(t *testing.T) {
	options := jobOptions{
		FPS:   14,
		Width: 960,
	}
	next, colors, dither, duration, changed := applyGIFEmergencyFallbackProfile(options, 192, "sierra2_4a", 3.2)
	if !changed {
		t.Fatalf("expected emergency fallback changed=true")
	}
	if next.FPS > 8 {
		t.Fatalf("expected fps <= 8, got %d", next.FPS)
	}
	if next.Width > 540 {
		t.Fatalf("expected width <= 540, got %d", next.Width)
	}
	if colors > 64 {
		t.Fatalf("expected colors <= 64, got %d", colors)
	}
	if dither != "none" {
		t.Fatalf("expected dither none, got %s", dither)
	}
	if duration > 3.2 || duration < 1.4 {
		t.Fatalf("unexpected duration %.3f", duration)
	}
}

func TestShouldLimitGIFRenderConcurrency(t *testing.T) {
	tasks := []animatedTask{
		{Format: "gif"},
		{Format: "gif"},
		{Format: "gif"},
	}
	if !shouldLimitGIFRenderConcurrency(videoProbeMeta{Width: 1140, Height: 2026, DurationSec: 75}, tasks) {
		t.Fatalf("expected high-res medium-long video to limit gif concurrency")
	}
	if shouldLimitGIFRenderConcurrency(videoProbeMeta{Width: 640, Height: 360, DurationSec: 20}, tasks) {
		t.Fatalf("expected low-res short video to keep gif concurrency")
	}
}
