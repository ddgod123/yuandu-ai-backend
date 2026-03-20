package videojobs

import "testing"

func TestResolveGIFRenderRetryMaxAttempts(t *testing.T) {
	settings := DefaultQualitySettings()

	short := resolveGIFRenderRetryMaxAttempts(videoProbeMeta{Width: 720, Height: 1280, DurationSec: 20}, settings)
	if short != settings.GIFRenderRetryMaxAttempts {
		t.Fatalf("expected short video retry attempts=%d, got %d", settings.GIFRenderRetryMaxAttempts, short)
	}

	long := resolveGIFRenderRetryMaxAttempts(videoProbeMeta{Width: 720, Height: 1280, DurationSec: 140}, settings)
	if long != 3 {
		t.Fatalf("expected long video retry attempts=3, got %d", long)
	}

	ultra := resolveGIFRenderRetryMaxAttempts(videoProbeMeta{Width: 720, Height: 1280, DurationSec: 280}, settings)
	if ultra != 2 {
		t.Fatalf("expected ultra video retry attempts=2, got %d", ultra)
	}

	highRes := resolveGIFRenderRetryMaxAttempts(videoProbeMeta{Width: 2160, Height: 3840, DurationSec: 40}, settings)
	if highRes != 3 {
		t.Fatalf("expected high-res video retry attempts=3, got %d", highRes)
	}
}
