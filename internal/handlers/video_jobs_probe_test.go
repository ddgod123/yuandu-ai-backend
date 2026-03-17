package handlers

import "testing"

func TestBuildAspectRatio(t *testing.T) {
	if got := buildAspectRatio(1920, 1080); got != "16:9" {
		t.Fatalf("expected 16:9, got %s", got)
	}
	if got := buildAspectRatio(720, 1280); got != "9:16" {
		t.Fatalf("expected 9:16, got %s", got)
	}
	if got := buildAspectRatio(0, 1080); got != "" {
		t.Fatalf("expected empty ratio, got %s", got)
	}
}

func TestMimeTypeByVideoExt(t *testing.T) {
	if got := mimeTypeByVideoExt("mp4"); got != "video/mp4" {
		t.Fatalf("unexpected mime for mp4: %s", got)
	}
	if got := mimeTypeByVideoExt(".mov"); got != "video/quicktime" {
		t.Fatalf("unexpected mime for mov: %s", got)
	}
	if got := mimeTypeByVideoExt("unknown"); got != "video/*" {
		t.Fatalf("unexpected mime for unknown: %s", got)
	}
}
