package videojobs

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestNormalizeOutputFormatsIncludesLive(t *testing.T) {
	got := normalizeOutputFormats("gif,live,webp,jpeg,live,mp4,bad")
	want := []string{"gif", "live", "webp", "jpg", "mp4"}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected item at %d: got=%v want=%v", i, got, want)
		}
	}
}

func TestSplitVideoOutputFormatsWithLive(t *testing.T) {
	staticFormats, animatedFormats := splitVideoOutputFormats([]string{"jpg", "gif", "live", "png"})
	if len(staticFormats) != 2 || staticFormats[0] != "jpg" || staticFormats[1] != "png" {
		t.Fatalf("unexpected static formats: %v", staticFormats)
	}
	if len(animatedFormats) != 2 || animatedFormats[0] != "gif" || animatedFormats[1] != "live" {
		t.Fatalf("unexpected animated formats: %v", animatedFormats)
	}
}

func TestCreateZipArchive(t *testing.T) {
	tmpDir := t.TempDir()
	photoPath := filepath.Join(tmpDir, "photo.jpg")
	videoPath := filepath.Join(tmpDir, "video.mov")
	zipPath := filepath.Join(tmpDir, "bundle.zip")

	if err := os.WriteFile(photoPath, []byte("photo"), 0o644); err != nil {
		t.Fatalf("write photo file: %v", err)
	}
	if err := os.WriteFile(videoPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("write video file: %v", err)
	}

	err := createZipArchive(zipPath, []zipEntrySource{
		{Name: "photo.jpg", Path: photoPath},
		{Name: "video.mov", Path: videoPath},
	})
	if err != nil {
		t.Fatalf("create zip archive: %v", err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip archive: %v", err)
	}
	defer reader.Close()

	if len(reader.File) != 2 {
		t.Fatalf("unexpected zip entries: %d", len(reader.File))
	}

	names := []string{reader.File[0].Name, reader.File[1].Name}
	sort.Strings(names)
	if names[0] != "photo.jpg" || names[1] != "video.mov" {
		t.Fatalf("unexpected zip names: %v", names)
	}
}
