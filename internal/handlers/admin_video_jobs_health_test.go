package handlers

import (
	"testing"

	"emoji/internal/models"
)

func TestParseRequestedFormatFromVideoJob(t *testing.T) {
	job := models.VideoJob{OutputFormats: " gif ,png"}
	if got := parseRequestedFormatFromVideoJob(job); got != "gif" {
		t.Fatalf("unexpected format: %s", got)
	}

	job = models.VideoJob{OutputFormats: "jpeg"}
	if got := parseRequestedFormatFromVideoJob(job); got != "jpg" {
		t.Fatalf("expected jpg, got %s", got)
	}

	job = models.VideoJob{OutputFormats: ""}
	if got := parseRequestedFormatFromVideoJob(job); got != "" {
		t.Fatalf("expected empty format, got %s", got)
	}
}

func TestCheckSourceVideoKeyHealth(t *testing.T) {
	status, _ := checkSourceVideoKeyHealth("emoji/source/demo.mp4")
	if status != "pass" {
		t.Fatalf("expected pass, got %s", status)
	}

	status, _ = checkSourceVideoKeyHealth("other/demo.mp4")
	if status != "warn" {
		t.Fatalf("expected warn, got %s", status)
	}

	status, _ = checkSourceVideoKeyHealth("emoji/source/demo.heic")
	if status != "fail" {
		t.Fatalf("expected fail, got %s", status)
	}
}

func TestAdminHealthCheckCollectorHealth(t *testing.T) {
	collector := &adminHealthCheckCollector{}
	collector.add("pass", "a", "ok", nil)
	if got := collector.health(); got != "green" {
		t.Fatalf("expected green, got %s", got)
	}

	collector.add("warn", "b", "warn", nil)
	if got := collector.health(); got != "yellow" {
		t.Fatalf("expected yellow, got %s", got)
	}

	collector.add("fail", "c", "fail", nil)
	if got := collector.health(); got != "red" {
		t.Fatalf("expected red, got %s", got)
	}

	summary := collector.summary()
	if summary.Total != 3 || summary.Passed != 1 || summary.Warned != 1 || summary.Failed != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestShouldExpectPackageForHealthJob(t *testing.T) {
	if shouldExpectPackageForHealthJob("gif", nil) {
		t.Fatalf("gif should not require package")
	}
	if !shouldExpectPackageForHealthJob("live", nil) {
		t.Fatalf("live should require package")
	}
	if !shouldExpectPackageForHealthJob("gif", []adminVideoJobHealthOutput{{Format: "zip", Role: "package"}}) {
		t.Fatalf("zip/package output should require package")
	}
}
