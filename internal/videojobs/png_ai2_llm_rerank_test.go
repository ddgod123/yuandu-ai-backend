package videojobs

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"emoji/internal/config"
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

func TestLoadPNGAI2LLMRerankConfig_FallbackFromPlannerWhenUnset(t *testing.T) {
	t.Setenv(featureFlagPNGAI2LLMRerankMode, "on")
	t.Setenv(featureFlagPNGAI2LLMRerankTimeoutSeconds, "")
	t.Setenv(featureFlagPNGAI2LLMRerankMaxTokens, "")
	t.Setenv(featureFlagPNGAI2LLMRerankPromptVersion, "")
	t.Setenv(featureFlagPNGAI2LLMRerankProvider, "")
	t.Setenv(featureFlagPNGAI2LLMRerankModel, "")
	t.Setenv(featureFlagPNGAI2LLMRerankEndpoint, "")
	t.Setenv(featureFlagPNGAI2LLMRerankAPIKey, "")

	p := &Processor{
		cfg: config.Config{
			AIPlannerEnabled:       true,
			AIPlannerProvider:      "qwen",
			AIPlannerModel:         "qwen-plus",
			AIPlannerEndpoint:      "https://planner.example",
			AIPlannerAPIKey:        "planner-key",
			AIPlannerPromptVersion: "gif_planner_v1",
			AIPlannerTimeoutSec:    19,
			AIPlannerMaxTokens:     1234,
			LLMProvider:            "qwen",
			LLMModel:               "qwen-max",
			LLMEndpoint:            "https://llm.example",
			LLMAPIKey:              "llm-key",
		},
	}
	cfg := p.loadPNGAI2LLMRerankConfig()
	if cfg.Mode != pngAI2LLMRerankModeOn {
		t.Fatalf("expected mode on, got %q", cfg.Mode)
	}
	if cfg.ModelCfg.Timeout != 19*time.Second {
		t.Fatalf("expected timeout from planner 19s, got %s", cfg.ModelCfg.Timeout)
	}
	if cfg.ModelCfg.MaxTokens != 1234 {
		t.Fatalf("expected max tokens from planner 1234, got %d", cfg.ModelCfg.MaxTokens)
	}
	if cfg.ModelCfg.PromptVersion != "gif_planner_v1" {
		t.Fatalf("expected prompt version from planner gif_planner_v1, got %q", cfg.ModelCfg.PromptVersion)
	}
}

func TestLoadPNGAI2LLMRerankConfig_EnvOverrides(t *testing.T) {
	t.Setenv(featureFlagPNGAI2LLMRerankMode, "shadow")
	t.Setenv(featureFlagPNGAI2LLMRerankProvider, "qwen")
	t.Setenv(featureFlagPNGAI2LLMRerankModel, "qwen3.5-omni-flash")
	t.Setenv(featureFlagPNGAI2LLMRerankEndpoint, "https://dashscope.example")
	t.Setenv(featureFlagPNGAI2LLMRerankAPIKey, "rerank-key")
	t.Setenv(featureFlagPNGAI2LLMRerankTimeoutSeconds, "31")
	t.Setenv(featureFlagPNGAI2LLMRerankMaxTokens, "1500")
	t.Setenv(featureFlagPNGAI2LLMRerankPromptVersion, "png_ai2_rerank_v2")
	t.Setenv(featureFlagPNGAI2LLMRerankMaxCandidates, "28")
	t.Setenv(featureFlagPNGAI2LLMRerankMinCandidates, "9")
	t.Setenv(featureFlagPNGAI2LLMRerankImageMaxCandidates, "16")
	t.Setenv(featureFlagPNGAI2LLMRerankImageMaxSide, "900")
	t.Setenv(featureFlagPNGAI2LLMRerankImageJPEGQuality, "80")
	t.Setenv(featureFlagPNGAI2LLMRerankImageMaxBytes, "888888")
	t.Setenv(featureFlagPNGAI2LLMRerankPostEnhance, "1")
	t.Setenv(featureFlagPNGAI2LLMRerankIncludeImages, "0")

	p := &Processor{cfg: config.Config{}}
	cfg := p.loadPNGAI2LLMRerankConfig()
	if cfg.Mode != pngAI2LLMRerankModeShadow {
		t.Fatalf("expected mode shadow, got %q", cfg.Mode)
	}
	if cfg.ModelCfg.Provider != "qwen" || cfg.ModelCfg.Model != "qwen3.5-omni-flash" {
		t.Fatalf("unexpected model routing: provider=%q model=%q", cfg.ModelCfg.Provider, cfg.ModelCfg.Model)
	}
	if cfg.ModelCfg.Timeout != 31*time.Second || cfg.ModelCfg.MaxTokens != 1500 {
		t.Fatalf("unexpected timeout/max_tokens: %s/%d", cfg.ModelCfg.Timeout, cfg.ModelCfg.MaxTokens)
	}
	if cfg.ModelCfg.PromptVersion != "png_ai2_rerank_v2" {
		t.Fatalf("unexpected prompt version: %q", cfg.ModelCfg.PromptVersion)
	}
	if cfg.MaxCandidates != 28 || cfg.MinCandidates != 9 {
		t.Fatalf("unexpected candidates: max=%d min=%d", cfg.MaxCandidates, cfg.MinCandidates)
	}
	if cfg.MaxImageCandidates != 16 || cfg.ImageMaxSide != 900 || cfg.ImageJPEGQuality != 80 || cfg.ImageMaxBytes != 888888 {
		t.Fatalf("unexpected image config: %+v", cfg)
	}
	if !cfg.EnablePostEnhance {
		t.Fatalf("expected post enhance enabled")
	}
	if cfg.IncludeImages {
		t.Fatalf("expected include images disabled")
	}
}
