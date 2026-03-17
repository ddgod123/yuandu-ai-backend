package handlers

import "testing"

func TestNormalizeGIFHealthAlertThresholdSettings_DefaultFallback(t *testing.T) {
	got := normalizeGIFHealthAlertThresholdSettings(GIFHealthAlertThresholdSettings{})
	def := defaultGIFHealthAlertThresholdSettings()
	if got != def {
		t.Fatalf("expected defaults, got %+v", got)
	}
}

func TestNormalizeGIFHealthAlertThresholdSettings_OrderGuard(t *testing.T) {
	got := normalizeGIFHealthAlertThresholdSettings(GIFHealthAlertThresholdSettings{
		GIFHealthDoneRateWarn:             0.70,
		GIFHealthDoneRateCritical:         0.90,
		GIFHealthFailedRateWarn:           0.40,
		GIFHealthFailedRateCritical:       0.20,
		GIFHealthPathStrictRateWarn:       0.80,
		GIFHealthPathStrictRateCritical:   0.95,
		GIFHealthLoopFallbackRateWarn:     0.60,
		GIFHealthLoopFallbackRateCritical: 0.20,
	})

	if !(got.GIFHealthDoneRateCritical < got.GIFHealthDoneRateWarn) {
		t.Fatalf("expected done critical < warn, got %+v", got)
	}
	if !(got.GIFHealthFailedRateCritical > got.GIFHealthFailedRateWarn) {
		t.Fatalf("expected failed critical > warn, got %+v", got)
	}
	if !(got.GIFHealthPathStrictRateCritical < got.GIFHealthPathStrictRateWarn) {
		t.Fatalf("expected path critical < warn, got %+v", got)
	}
	if !(got.GIFHealthLoopFallbackRateCritical > got.GIFHealthLoopFallbackRateWarn) {
		t.Fatalf("expected fallback critical > warn, got %+v", got)
	}
}
