package handlers

import (
	"testing"

	"emoji/internal/models"
)

func TestNormalizeAdminSystemDoctorHealth(t *testing.T) {
	cases := map[string]string{
		"green":  "green",
		"yellow": "yellow",
		"red":    "red",
		"RED":    "red",
		"bad":    "green",
		"":       "green",
	}
	for input, want := range cases {
		if got := normalizeAdminSystemDoctorHealth(input); got != want {
			t.Fatalf("normalize health %q => %q, want %q", input, got, want)
		}
	}
}

func TestEvaluateAdminSystemDoctorTemplateCoverage_AllRequiredPresent(t *testing.T) {
	rows := []models.VideoAIPromptTemplate{
		{ID: 101, Format: "gif", Stage: "ai1", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 102, Format: "gif", Stage: "ai2", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 103, Format: "gif", Stage: "ai3", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 111, Format: "png", Stage: "ai1", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 112, Format: "png", Stage: "ai2", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 113, Format: "png", Stage: "ai3", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 121, Format: "jpg", Stage: "ai1", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 122, Format: "jpg", Stage: "ai2", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 123, Format: "jpg", Stage: "ai3", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 131, Format: "webp", Stage: "ai1", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 132, Format: "webp", Stage: "ai2", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 133, Format: "webp", Stage: "ai3", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 141, Format: "live", Stage: "ai1", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 142, Format: "live", Stage: "ai2", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
		{ID: 143, Format: "live", Stage: "ai3", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
	}
	got := evaluateAdminSystemDoctorTemplateCoverage(rows)
	if got.Health != adminSystemDoctorHealthGreen {
		t.Fatalf("expected health green, got %s alerts=%v", got.Health, got.Alerts)
	}
	if got.Coverage.Stats["missing_required"] != 0 {
		t.Fatalf("expected missing_required=0, got %+v", got.Coverage.Stats)
	}
	if got.Coverage.Stats["disabled"] != 0 {
		t.Fatalf("expected disabled=0, got %+v", got.Coverage.Stats)
	}
}

func TestEvaluateAdminSystemDoctorTemplateCoverage_FallbackAndMissing(t *testing.T) {
	rows := []models.VideoAIPromptTemplate{
		{ID: 1, Format: "all", Stage: "ai1", Layer: "fixed", Enabled: true, IsActive: true, Version: "all-v1"},
		{ID: 2, Format: "all", Stage: "ai2", Layer: "fixed", Enabled: false, IsActive: true, Version: "all-v1"},
		{ID: 3, Format: "all", Stage: "ai3", Layer: "fixed", Enabled: true, IsActive: true, Version: "all-v1"},
		{ID: 4, Format: "png", Stage: "ai2", Layer: "rerank", Enabled: true, IsActive: true, Version: "r1"},
	}
	got := evaluateAdminSystemDoctorTemplateCoverage(rows)
	if got.Health != adminSystemDoctorHealthYellow {
		t.Fatalf("expected yellow (fallback+disabled), got %s alerts=%v", got.Health, got.Alerts)
	}
	if got.Coverage.Stats["fallback_all"] == 0 {
		t.Fatalf("expected fallback_all > 0, got %+v", got.Coverage.Stats)
	}
	if got.Coverage.Stats["disabled"] == 0 {
		t.Fatalf("expected disabled > 0, got %+v", got.Coverage.Stats)
	}
}

func TestEvaluateAdminSystemDoctorTemplateCoverage_MissingRequiredRed(t *testing.T) {
	rows := []models.VideoAIPromptTemplate{
		{ID: 11, Format: "gif", Stage: "ai1", Layer: "fixed", Enabled: true, IsActive: true, Version: "v1"},
	}
	got := evaluateAdminSystemDoctorTemplateCoverage(rows)
	if got.Health != adminSystemDoctorHealthRed {
		t.Fatalf("expected red for missing required slots, got %s alerts=%v", got.Health, got.Alerts)
	}
	if got.Coverage.Stats["missing_required"] == 0 {
		t.Fatalf("expected missing_required > 0, got %+v", got.Coverage.Stats)
	}
}

func TestIsAIDoctorModelConfigReady(t *testing.T) {
	if !isAIDoctorModelConfigReady("qwen", "qwen3.5-omni-flash", "https://example.com/v1", "key") {
		t.Fatalf("expected ready config")
	}
	if isAIDoctorModelConfigReady("qwen", "", "https://example.com/v1", "key") {
		t.Fatalf("expected missing model to be not ready")
	}
}
