package videojobs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceHTTPStatusFromErr(t *testing.T) {
	status, ok := sourceHTTPStatusFromErr(errors.New("unexpected status 404"))
	if !ok {
		t.Fatalf("expected status parse ok")
	}
	if status != 404 {
		t.Fatalf("expected 404, got %d", status)
	}
}

func TestClassifySourceReadabilityFailureNotFound(t *testing.T) {
	reason, hint, permanent := classifySourceReadabilityFailure(
		errors.New("download failed"),
		[]map[string]interface{}{
			{
				"step":        "qiniu_signed_url",
				"http_status": int64(404),
				"error":       "unexpected status 404",
			},
		},
	)
	if reason != "source_video_not_found" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	if !permanent {
		t.Fatalf("expected permanent=true")
	}
	if hint == "" {
		t.Fatalf("expected hint")
	}
}

func TestClassifySourceReadabilityFailureNetwork(t *testing.T) {
	reason, _, permanent := classifySourceReadabilityFailure(
		errors.New("download failed"),
		[]map[string]interface{}{
			{
				"step":  "qiniu_signed_url",
				"error": "dial tcp: no such host",
			},
		},
	)
	if reason != "source_video_network_unstable" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	if permanent {
		t.Fatalf("expected permanent=false")
	}
}

func TestClassifySourceReadabilityFailureIntegrityMismatch(t *testing.T) {
	reason, hint, permanent := classifySourceReadabilityFailure(
		errors.New("download failed"),
		[]map[string]interface{}{
			{
				"step":                  "qiniu_signed_url",
				"downloaded_size_bytes": int64(123),
				"expected_size_bytes":   int64(456),
				"size_match":            false,
				"error":                 "downloaded source size mismatch: expected=456 got=123",
			},
		},
	)
	if reason != "source_video_integrity_mismatch" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	if permanent {
		t.Fatalf("expected permanent=false")
	}
	if hint == "" {
		t.Fatalf("expected hint")
	}
}

func TestValidateDownloadedSourceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.mp4")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	size, sizeMatch, err := validateDownloadedSourceFile(path, 5)
	if err != nil {
		t.Fatalf("validate file: %v", err)
	}
	if size != 5 {
		t.Fatalf("unexpected size: %d", size)
	}
	if !sizeMatch {
		t.Fatalf("expected size match")
	}
}

func TestValidateDownloadedSourceFileMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.mp4")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	size, sizeMatch, err := validateDownloadedSourceFile(path, 7)
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
	if size != 5 {
		t.Fatalf("unexpected size: %d", size)
	}
	if sizeMatch {
		t.Fatalf("expected size mismatch")
	}
}

func TestValidateDownloadedSourceFileNonPositive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.mp4")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	size, sizeMatch, err := validateDownloadedSourceFile(path, 0)
	if err == nil {
		t.Fatalf("expected non_positive error")
	}
	if size != 0 {
		t.Fatalf("unexpected size: %d", size)
	}
	if sizeMatch {
		t.Fatalf("expected size mismatch")
	}
}
