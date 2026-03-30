package videojobs

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizePNGAI2LLMOrderedPaths_PrioritizeIDsThenFallback(t *testing.T) {
	original := []string{
		"/tmp/frame_0001.png",
		"/tmp/frame_0002.png",
		"/tmp/frame_0003.png",
	}
	resp := pngAI2LLMRerankResponse{
		OrderedCandidateIDs: []string{"c02", "c01"},
		OrderedFramePaths:   []string{"frame_0003.png"},
	}
	idToPath := map[string]string{
		"c01": original[0],
		"c02": original[1],
		"c03": original[2],
	}
	frameNameToPath := map[string]string{
		"frame_0001.png": original[0],
		"frame_0002.png": original[1],
		"frame_0003.png": original[2],
	}

	got := normalizePNGAI2LLMOrderedPaths(resp, original, idToPath, frameNameToPath)
	want := []string{
		"/tmp/frame_0002.png",
		"/tmp/frame_0001.png",
		"/tmp/frame_0003.png",
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got=%d want=%d, got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order at %d: got=%v want=%v", i, got, want)
		}
	}
}

func TestExtractPNGWorkerEnhancementCostCNY(t *testing.T) {
	metrics := map[string]interface{}{
		"png_worker_face_enhancement_v1": map[string]interface{}{
			"total_cost_cny": 0.0123,
		},
		"png_worker_super_resolution_v1": map[string]interface{}{
			"total_cost_cny": 0.08,
		},
	}
	total, face, super := extractPNGWorkerEnhancementCostCNY(metrics)
	if face != 0.0123 {
		t.Fatalf("unexpected face cost: %v", face)
	}
	if super != 0.08 {
		t.Fatalf("unexpected super cost: %v", super)
	}
	if total != 0.0923 {
		t.Fatalf("unexpected total cost: %v", total)
	}
}

func TestBuildPNGAI2LLMRerankVisualParts_AttachImages(t *testing.T) {
	tmpDir := t.TempDir()
	framePath := filepath.Join(tmpDir, "frame_0001.png")
	img := image.NewRGBA(image.Rect(0, 0, 160, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 180, A: 255})
		}
	}
	f, err := os.Create(framePath)
	if err != nil {
		t.Fatalf("create frame: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("encode png: %v", err)
	}
	_ = f.Close()

	cfg := pngAI2LLMRerankConfig{
		IncludeImages:      true,
		MaxImageCandidates: 4,
		ImageMaxSide:       512,
		ImageJPEGQuality:   72,
		ImageMaxBytes:      512 * 1024,
	}
	candidates := []pngAI2LLMRerankCandidate{
		{
			CandidateID: "c01",
			FramePath:   framePath,
			LocalRank:   1,
		},
	}
	parts, report := buildPNGAI2LLMRerankVisualParts(candidates, cfg)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (text+image), got %d", len(parts))
	}
	if count := countPNGAI2LLMRerankVisionAttachments(parts); count != 1 {
		t.Fatalf("expected 1 image attachment, got %d", count)
	}
	if got := stringFromAny(report["status"]); got != "attached" {
		t.Fatalf("expected status attached, got %q", got)
	}
	if got := intFromAny(report["succeeded"]); got != 1 {
		t.Fatalf("expected succeeded=1, got %d", got)
	}
}
