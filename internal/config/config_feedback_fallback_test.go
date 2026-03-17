package config

import "testing"

func TestLoadEnableLegacyFeedbackFallbackDefaultFalse(t *testing.T) {
	t.Setenv("ENABLE_LEGACY_FEEDBACK_FALLBACK", "")
	cfg := Load()
	if cfg.EnableLegacyFeedbackFallback {
		t.Fatalf("expected EnableLegacyFeedbackFallback default false")
	}
}

func TestLoadEnableLegacyFeedbackFallbackFromEnv(t *testing.T) {
	t.Setenv("ENABLE_LEGACY_FEEDBACK_FALLBACK", "true")
	cfg := Load()
	if !cfg.EnableLegacyFeedbackFallback {
		t.Fatalf("expected EnableLegacyFeedbackFallback=true when env is true")
	}
}
