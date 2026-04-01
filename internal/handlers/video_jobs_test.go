package handlers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"emoji/internal/videojobs"
)

func TestNormalizeSourceVideoKey_AllowsCommonVideoExtensions(t *testing.T) {
	h := &Handler{}
	tests := []string{
		"emoji/sample.mp4",
		"emoji/sample.mov",
		"emoji/sample.mkv",
		"emoji/sample.webm",
		"emoji/sample.avi",
		"emoji/sample.m4v",
		"emoji/sample.mpeg",
		"emoji/sample.mpg",
		"emoji/sample.wmv",
		"emoji/sample.flv",
		"emoji/sample.3gp",
		"emoji/sample.ts",
		"emoji/sample.mts",
		"emoji/sample.m2ts",
	}

	for _, raw := range tests {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			got, err := h.normalizeSourceVideoKey(raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != raw {
				t.Fatalf("unexpected normalized key, want=%s got=%s", raw, got)
			}
		})
	}
}

func TestNormalizeSourceVideoKey_RejectsUnsupportedExtension(t *testing.T) {
	h := &Handler{}
	if _, err := h.normalizeSourceVideoKey("emoji/sample.heic"); err == nil {
		t.Fatalf("expected unsupported video format error")
	}
}

func TestNormalizeSourceVideoKey_AllowsConfiguredRootPrefix(t *testing.T) {
	h := &Handler{}
	h.cfg.QiniuRootPrefix = "emoji-prod"

	if got, err := h.normalizeSourceVideoKey("emoji-prod/sample.mp4"); err != nil || got != "emoji-prod/sample.mp4" {
		t.Fatalf("expected configured root key to pass, got=%s err=%v", got, err)
	}
	// Legacy prefix still accepted during migration.
	if got, err := h.normalizeSourceVideoKey("emoji/sample.mp4"); err != nil || got != "emoji/sample.mp4" {
		t.Fatalf("expected legacy root key to pass during migration, got=%s err=%v", got, err)
	}
	if _, err := h.normalizeSourceVideoKey("other/sample.mp4"); err == nil {
		t.Fatalf("expected root prefix validation error")
	}
}

func TestDescribeSourceVideoProbeFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{
			name: "missing stream",
			err:  videojobs.ErrVideoStreamNotFound,
			code: "video_stream_missing",
		},
		{
			name: "duration missing",
			err:  videojobs.ErrVideoDurationMissing,
			code: "video_duration_invalid",
		},
		{
			name: "duration too long",
			err:  fmt.Errorf("wrap: %w", videojobs.ErrVideoDurationTooLong),
			code: "video_duration_too_long",
		},
		{
			name: "timeout",
			err:  context.DeadlineExceeded,
			code: "video_probe_timeout",
		},
		{
			name: "corrupted",
			err:  errors.New("ffprobe failed: Invalid data found when processing input"),
			code: "video_corrupted",
		},
		{
			name: "default",
			err:  errors.New("something else"),
			code: "video_probe_failed",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := describeSourceVideoProbeFailure(tc.err)
			if got.Code != tc.code {
				t.Fatalf("unexpected code, want=%s got=%s", tc.code, got.Code)
			}
			if got.Message == "" {
				t.Fatalf("message should not be empty")
			}
		})
	}
}

func TestClassifyVideoBuckets(t *testing.T) {
	if got := classifyVideoDurationBucket(8); got != "5-10s" {
		t.Fatalf("unexpected duration bucket: %s", got)
	}
	if got := classifyVideoResolutionBucket(1920, 1080); got != "1080p" {
		t.Fatalf("unexpected resolution bucket: %s", got)
	}
	if got := classifyVideoFPSBucket(61); got != "60fps+" {
		t.Fatalf("unexpected fps bucket: %s", got)
	}
}

func TestRecommendFormatProfilesForVideo_PrefersSizeOnRiskyLongInput(t *testing.T) {
	durationStat := &AdminVideoJobSourceProbeQualityStat{
		Bucket:       "10m+",
		TerminalJobs: 20,
		FailureRate:  0.24,
	}
	profiles, reasons := recommendFormatProfilesForVideo(
		[]string{"gif", "webp", "jpg", "png", "live"},
		"10m+",
		"1080p",
		"30-60fps",
		durationStat,
		nil,
		nil,
	)
	if profiles["gif"] != videojobs.QualityProfileSize {
		t.Fatalf("expected gif=size, got %s", profiles["gif"])
	}
	if profiles["webp"] != videojobs.QualityProfileSize {
		t.Fatalf("expected webp=size, got %s", profiles["webp"])
	}
	if profiles["live"] != videojobs.QualityProfileSize {
		t.Fatalf("expected live=size, got %s", profiles["live"])
	}
	if profiles["jpg"] != videojobs.QualityProfileClarity {
		t.Fatalf("expected jpg=clarity, got %s", profiles["jpg"])
	}
	if len(reasons) == 0 {
		t.Fatalf("expected non-empty recommendation reasons")
	}
}

func TestExtractRecommendedProfilesFromRecommendation(t *testing.T) {
	rec := map[string]interface{}{
		"recommended_profiles": map[string]interface{}{
			"gif":  "size",
			"jpg":  "clarity",
			"live": "size",
			"svg":  "clarity",
		},
	}
	out := extractRecommendedProfilesFromRecommendation(rec)
	if out["gif"] != "size" {
		t.Fatalf("expected gif size, got %s", out["gif"])
	}
	if out["jpg"] != "clarity" {
		t.Fatalf("expected jpg clarity, got %s", out["jpg"])
	}
	if out["live"] != "size" {
		t.Fatalf("expected live size, got %s", out["live"])
	}
	if _, ok := out["svg"]; ok {
		t.Fatalf("unsupported format should be ignored")
	}
}
