package videojobs

import "testing"

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
