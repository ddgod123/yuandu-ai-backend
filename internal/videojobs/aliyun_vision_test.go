package videojobs

import (
	"image/color"
	"path/filepath"
	"testing"
)

func TestLoadPNGAliyunSuperResConfig_ModeDefaultOn(t *testing.T) {
	t.Setenv("PNG_ALIYUN_SUPERRES_MODE", "")
	cfg := loadPNGAliyunSuperResConfig()
	if cfg.Mode != pngAliyunSuperResModeOn {
		t.Fatalf("expected default mode=%s, got=%s", pngAliyunSuperResModeOn, cfg.Mode)
	}
}

func TestLoadPNGAliyunSuperResConfig_ModeInvalidFallbackOn(t *testing.T) {
	t.Setenv("PNG_ALIYUN_SUPERRES_MODE", "invalid_mode")
	cfg := loadPNGAliyunSuperResConfig()
	if cfg.Mode != pngAliyunSuperResModeOn {
		t.Fatalf("expected invalid mode fallback=%s, got=%s", pngAliyunSuperResModeOn, cfg.Mode)
	}
}

func TestLoadPNGAliyunSuperResConfig_ModeExplicit(t *testing.T) {
	t.Setenv("PNG_ALIYUN_SUPERRES_MODE", "off")
	cfg := loadPNGAliyunSuperResConfig()
	if cfg.Mode != pngAliyunSuperResModeOff {
		t.Fatalf("expected mode off, got=%s", cfg.Mode)
	}

	t.Setenv("PNG_ALIYUN_SUPERRES_MODE", "shadow")
	cfg = loadPNGAliyunSuperResConfig()
	if cfg.Mode != pngAliyunSuperResModeShadow {
		t.Fatalf("expected mode shadow, got=%s", cfg.Mode)
	}

	t.Setenv("PNG_ALIYUN_SUPERRES_MODE", "on")
	cfg = loadPNGAliyunSuperResConfig()
	if cfg.Mode != pngAliyunSuperResModeOn {
		t.Fatalf("expected mode on, got=%s", cfg.Mode)
	}
}

func TestLoadPNGAliyunSuperResConfig_BoundsAndDefaults(t *testing.T) {
	t.Setenv("PNG_ALIYUN_SUPERRES_MODE", "on")
	t.Setenv("ALIYUN_VISION_REGION_ID", "")
	t.Setenv("ALIYUN_VISION_IMAGEENHAN_ENDPOINT", "")
	t.Setenv("PNG_ALIYUN_SUPERRES_MIN_SHORT_SIDE", "-1")
	t.Setenv("PNG_ALIYUN_SUPERRES_MAX_FRAMES", "999")
	t.Setenv("PNG_ALIYUN_SUPERRES_UPSCALE_FACTOR", "9")
	t.Setenv("PNG_ALIYUN_SUPERRES_OUTPUT_QUALITY", "200")
	t.Setenv("PNG_ALIYUN_SUPERRES_TIMEOUT_SECONDS", "1")
	t.Setenv("PNG_ALIYUN_SUPERRES_COST_PER_IMAGE_CNY", "-0.5")
	t.Setenv("PNG_ALIYUN_SUPERRES_MAX_COST_PER_JOB_CNY", "-1")

	cfg := loadPNGAliyunSuperResConfig()

	if cfg.RegionID != "cn-shanghai" {
		t.Fatalf("expected default region cn-shanghai, got=%s", cfg.RegionID)
	}
	if cfg.Endpoint != "imageenhan.cn-shanghai.aliyuncs.com" {
		t.Fatalf("expected default endpoint, got=%s", cfg.Endpoint)
	}
	if cfg.MinShortSide != 960 {
		t.Fatalf("expected min short side clamp=960, got=%d", cfg.MinShortSide)
	}
	if cfg.MaxFrames != 16 {
		t.Fatalf("expected max frames clamp=16, got=%d", cfg.MaxFrames)
	}
	if cfg.UpscaleFactor != 4 {
		t.Fatalf("expected upscale factor clamp=4, got=%d", cfg.UpscaleFactor)
	}
	if cfg.OutputQuality != 100 {
		t.Fatalf("expected output quality clamp=100, got=%d", cfg.OutputQuality)
	}
	if cfg.TimeoutSec != 25 {
		t.Fatalf("expected timeout lower-bound fallback=25, got=%d", cfg.TimeoutSec)
	}
	if cfg.CostPerImageCNY != 0 {
		t.Fatalf("expected non-negative cost clamp=0, got=%f", cfg.CostPerImageCNY)
	}
	if cfg.MaxCostPerJobCNY != 0 {
		t.Fatalf("expected non-negative max cost clamp=0, got=%f", cfg.MaxCostPerJobCNY)
	}
}

func TestLoadPNGAliyunSuperResConfig_MaxCostDefaultAndParse(t *testing.T) {
	t.Setenv("PNG_ALIYUN_SUPERRES_MODE", "on")
	t.Setenv("PNG_ALIYUN_SUPERRES_MAX_COST_PER_JOB_CNY", "")
	cfg := loadPNGAliyunSuperResConfig()
	if cfg.MaxCostPerJobCNY != 0.08 {
		t.Fatalf("expected default max cost per job=0.08, got=%f", cfg.MaxCostPerJobCNY)
	}

	t.Setenv("PNG_ALIYUN_SUPERRES_MAX_COST_PER_JOB_CNY", "0.125")
	cfg = loadPNGAliyunSuperResConfig()
	if cfg.MaxCostPerJobCNY != 0.125 {
		t.Fatalf("expected parsed max cost per job=0.125, got=%f", cfg.MaxCostPerJobCNY)
	}
}

func TestLoadPNGAliyunFaceEnhanceConfig_ModeDefaultAuto(t *testing.T) {
	t.Setenv("PNG_ALIYUN_FACE_ENHANCE_MODE", "")
	cfg := loadPNGAliyunFaceEnhanceConfig()
	if cfg.Mode != pngAliyunFaceEnhanceModeAuto {
		t.Fatalf("expected default mode=%s, got=%s", pngAliyunFaceEnhanceModeAuto, cfg.Mode)
	}
}

func TestLoadPNGAliyunFaceEnhanceConfig_BoundsAndDefaults(t *testing.T) {
	t.Setenv("PNG_ALIYUN_FACE_ENHANCE_MODE", "on")
	t.Setenv("ALIYUN_VISION_REGION_ID", "")
	t.Setenv("ALIYUN_VISION_FACEBODY_ENDPOINT", "")
	t.Setenv("PNG_ALIYUN_FACE_ENHANCE_MIN_SHORT_SIDE", "-1")
	t.Setenv("PNG_ALIYUN_FACE_ENHANCE_MAX_FRAMES", "999")
	t.Setenv("PNG_ALIYUN_FACE_ENHANCE_TIMEOUT_SECONDS", "1")
	t.Setenv("PNG_ALIYUN_FACE_ENHANCE_COST_PER_IMAGE_CNY", "-1")
	t.Setenv("PNG_ALIYUN_FACE_ENHANCE_MAX_COST_PER_JOB_CNY", "-2")

	cfg := loadPNGAliyunFaceEnhanceConfig()
	if cfg.RegionID != "cn-shanghai" {
		t.Fatalf("expected default region cn-shanghai, got=%s", cfg.RegionID)
	}
	if cfg.Endpoint != "facebody.cn-shanghai.aliyuncs.com" {
		t.Fatalf("expected default endpoint, got=%s", cfg.Endpoint)
	}
	if cfg.MinShortSide != 360 {
		t.Fatalf("expected min short side clamp=360, got=%d", cfg.MinShortSide)
	}
	if cfg.MaxFrames != 12 {
		t.Fatalf("expected max frames clamp=12, got=%d", cfg.MaxFrames)
	}
	if cfg.TimeoutSec != 25 {
		t.Fatalf("expected timeout lower-bound fallback=25, got=%d", cfg.TimeoutSec)
	}
	if cfg.CostPerImageCNY != 0 {
		t.Fatalf("expected non-negative cost clamp=0, got=%f", cfg.CostPerImageCNY)
	}
	if cfg.MaxCostPerJobCNY != 0 {
		t.Fatalf("expected non-negative max cost clamp=0, got=%f", cfg.MaxCostPerJobCNY)
	}
	if !cfg.DetectFaceGate {
		t.Fatalf("expected detect face gate default enabled")
	}
	if cfg.MinFaceAreaRatio <= 0 {
		t.Fatalf("expected min face area ratio > 0")
	}
	if cfg.ReplaceMinGain < -0.05 || cfg.ReplaceMinGain > 0.2 {
		t.Fatalf("unexpected replace min gain clamp: %f", cfg.ReplaceMinGain)
	}
}

func TestShouldAutoApplyPNGAliyunFaceEnhancement(t *testing.T) {
	if !shouldAutoApplyPNGAliyunFaceEnhancement(imageAI2Guidance{VisualFocus: []string{"portrait"}}) {
		t.Fatal("expected portrait focus to enable auto face enhancement")
	}
	if !shouldAutoApplyPNGAliyunFaceEnhancement(imageAI2Guidance{EnableMatting: true}) {
		t.Fatal("expected matting to enable auto face enhancement")
	}
	if !shouldAutoApplyPNGAliyunFaceEnhancement(imageAI2Guidance{Scene: AdvancedScenarioXiaohongshu}) {
		t.Fatal("expected xiaohongshu scene to enable auto face enhancement")
	}
	if !shouldAutoApplyPNGAliyunFaceEnhancement(imageAI2Guidance{MustCapture: []string{"人脸特写"}}) {
		t.Fatal("expected face must_capture to enable auto face enhancement")
	}
	if shouldAutoApplyPNGAliyunFaceEnhancement(imageAI2Guidance{
		Scene:       AdvancedScenarioDefault,
		VisualFocus: []string{"vibe"},
		MustCapture: []string{"全景氛围"},
	}) {
		t.Fatal("expected non-portrait vibe scene to skip auto face enhancement")
	}
}

func TestBuildPNGAliyunSuperResCandidateOrder_FaceFirst(t *testing.T) {
	in := []string{
		"/tmp/frame_0001.png",
		"/tmp/frame_0002.png.face.png",
		"/tmp/frame_0003.png",
		"/tmp/frame_0004.png.face.png",
	}
	order := buildPNGAliyunSuperResCandidateOrder(in)
	if len(order) != len(in) {
		t.Fatalf("unexpected order length %d", len(order))
	}
	if order[0] != 1 || order[1] != 3 {
		t.Fatalf("expected face frames first, got %+v", order)
	}
}

func TestDecideEnhancedFrameReplacement(t *testing.T) {
	tmpDir := t.TempDir()
	origin := filepath.Join(tmpDir, "origin.jpg")
	enhanced := filepath.Join(tmpDir, "enhanced.jpg")
	if err := writeJPEG(origin, buildSolidImage(160, 100, color.Gray{Y: 128})); err != nil {
		t.Fatalf("write origin: %v", err)
	}
	if err := writeJPEG(enhanced, buildCheckerImage(160, 100)); err != nil {
		t.Fatalf("write enhanced: %v", err)
	}
	replace, beforeScore, afterScore, reason := decideEnhancedFrameReplacement(origin, enhanced, 0.001)
	if !replace {
		t.Fatalf("expected replace=true, reason=%s", reason)
	}
	if afterScore <= beforeScore {
		t.Fatalf("expected after score > before score, before=%.4f after=%.4f", beforeScore, afterScore)
	}
}

func TestComputeEnhancementResolutionBonus(t *testing.T) {
	before := frameQualitySample{Width: 720, Height: 1280}
	after := frameQualitySample{Width: 1440, Height: 2560}
	bonus := computeEnhancementResolutionBonus(before, after)
	if bonus <= 0 {
		t.Fatalf("expected positive bonus for upscaled frame, got=%.6f", bonus)
	}

	flat := computeEnhancementResolutionBonus(before, before)
	if flat != 0 {
		t.Fatalf("expected zero bonus for same resolution, got=%.6f", flat)
	}
}
