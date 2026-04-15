package videojobs

import "testing"

func TestLoadVideoJobFeatureFlags_Defaults(t *testing.T) {
	t.Setenv(featureFlagPNGEnhancementStageEnabled, "")
	t.Setenv(featureFlagPNGMainlineOutputCountGuardEnabled, "")
	t.Setenv(featureFlagPNGMainlineOutputCountMin, "")
	t.Setenv(featureFlagPNGMainlineOutputCountMax, "")
	t.Setenv(featureFlagPNGMainlineCoverageWindowGuard, "")
	t.Setenv(featureFlagPNGMainlineCoverageWindowTargetMin, "")
	t.Setenv(featureFlagPNGMainlineCoverageWindowReqRatio, "")
	t.Setenv(featureFlagPNGMainlineCoverageWindowMinRatio, "")
	t.Setenv(featureFlagPNGFinalQualityGuardEnabled, "")
	t.Setenv(featureFlagPNGFinalQualityGuardMinKeep, "")
	t.Setenv(featureFlagPNGFinalQualityGuardMinKeepRatio, "")
	t.Setenv(featureFlagPNGFinalDiversityGuardEnabled, "")
	t.Setenv(featureFlagPNGFinalDiversityMinKeep, "")
	t.Setenv(featureFlagPNGFinalDiversityMinKeepRatio, "")
	t.Setenv(featureFlagPNGAliyunSuperResQualityGuard, "")
	t.Setenv(featureFlagPNGAliyunFaceDetectGate, "")
	t.Setenv(featureFlagPNGAliyunFaceQualityGuard, "")
	t.Setenv(featureFlagPNGAI2LLMRerankPostEnhance, "")
	t.Setenv(featureFlagPNGAI2LLMRerankIncludeImages, "")

	flags := loadVideoJobFeatureFlags()
	if flags.PNGEnhancementStageEnabled {
		t.Fatalf("expected PNG enhancement stage default disabled")
	}
	if !flags.PNGMainlineOutputCountGuardEnabled {
		t.Fatalf("expected PNG output count guard default enabled")
	}
	if !flags.PNGMainlineCoverageWindowGuardEnabled {
		t.Fatalf("expected PNG coverage window guard default enabled")
	}
	if flags.PNGMainlineOutputCountMin != 12 || flags.PNGMainlineOutputCountMax != 36 {
		t.Fatalf("expected output count defaults 12/36, got %d/%d", flags.PNGMainlineOutputCountMin, flags.PNGMainlineOutputCountMax)
	}
	if flags.PNGMainlineCoverageWindowTargetMin != 12 {
		t.Fatalf("expected coverage target min default 12, got %d", flags.PNGMainlineCoverageWindowTargetMin)
	}
	if flags.PNGMainlineCoverageWindowReqRatio != 0.75 {
		t.Fatalf("expected coverage required ratio default 0.75, got %.3f", flags.PNGMainlineCoverageWindowReqRatio)
	}
	if flags.PNGMainlineCoverageWindowMinRatio != 0.45 {
		t.Fatalf("expected coverage min ratio default 0.45, got %.3f", flags.PNGMainlineCoverageWindowMinRatio)
	}
	if !flags.PNGFinalQualityGuardEnabled {
		t.Fatalf("expected PNG final quality guard default enabled")
	}
	if !flags.PNGFinalDiversityGuardEnabled {
		t.Fatalf("expected PNG final diversity guard default enabled")
	}
	if flags.PNGFinalQualityGuardMinKeep != 4 || flags.PNGFinalQualityGuardMinKeepRatio != 0.55 {
		t.Fatalf("expected final quality min keep defaults 4/0.55, got %d/%.3f", flags.PNGFinalQualityGuardMinKeep, flags.PNGFinalQualityGuardMinKeepRatio)
	}
	if flags.PNGFinalDiversityMinKeep != 4 || flags.PNGFinalDiversityMinKeepRatio != 0.6 {
		t.Fatalf("expected final diversity min keep defaults 4/0.60, got %d/%.3f", flags.PNGFinalDiversityMinKeep, flags.PNGFinalDiversityMinKeepRatio)
	}
	if !flags.PNGAliyunSuperResQualityGuard {
		t.Fatalf("expected aliyun super-res quality guard default enabled")
	}
	if !flags.PNGAliyunFaceEnhanceDetectFaceGate {
		t.Fatalf("expected face detect gate default enabled")
	}
	if !flags.PNGAliyunFaceEnhanceQualityGuard {
		t.Fatalf("expected face quality guard default enabled")
	}
	if flags.PNGAI2LLMRerankPostEnhance {
		t.Fatalf("expected ai2 llm rerank post-enhance default disabled")
	}
	if !flags.PNGAI2LLMRerankIncludeImages {
		t.Fatalf("expected ai2 llm rerank include-images default enabled")
	}
}

func TestLoadVideoJobFeatureFlags_Overrides(t *testing.T) {
	t.Setenv(featureFlagPNGEnhancementStageEnabled, "1")
	t.Setenv(featureFlagPNGMainlineOutputCountGuardEnabled, "0")
	t.Setenv(featureFlagPNGMainlineOutputCountMin, "16")
	t.Setenv(featureFlagPNGMainlineOutputCountMax, "44")
	t.Setenv(featureFlagPNGMainlineCoverageWindowGuard, "false")
	t.Setenv(featureFlagPNGMainlineCoverageWindowTargetMin, "15")
	t.Setenv(featureFlagPNGMainlineCoverageWindowReqRatio, "0.82")
	t.Setenv(featureFlagPNGMainlineCoverageWindowMinRatio, "0.55")
	t.Setenv(featureFlagPNGFinalQualityGuardEnabled, "off")
	t.Setenv(featureFlagPNGFinalQualityGuardMinKeep, "5")
	t.Setenv(featureFlagPNGFinalQualityGuardMinKeepRatio, "0.66")
	t.Setenv(featureFlagPNGFinalDiversityGuardEnabled, "NO")
	t.Setenv(featureFlagPNGFinalDiversityMinKeep, "6")
	t.Setenv(featureFlagPNGFinalDiversityMinKeepRatio, "0.73")
	t.Setenv(featureFlagPNGAliyunSuperResQualityGuard, "true")
	t.Setenv(featureFlagPNGAliyunFaceDetectGate, "on")
	t.Setenv(featureFlagPNGAliyunFaceQualityGuard, "y")
	t.Setenv(featureFlagPNGAI2LLMRerankPostEnhance, "yes")
	t.Setenv(featureFlagPNGAI2LLMRerankIncludeImages, "n")

	flags := loadVideoJobFeatureFlags()
	if !flags.PNGEnhancementStageEnabled {
		t.Fatalf("expected PNG enhancement stage enabled")
	}
	if flags.PNGMainlineOutputCountGuardEnabled {
		t.Fatalf("expected PNG output count guard disabled")
	}
	if flags.PNGMainlineCoverageWindowGuardEnabled {
		t.Fatalf("expected PNG coverage window guard disabled")
	}
	if flags.PNGMainlineOutputCountMin != 16 || flags.PNGMainlineOutputCountMax != 44 {
		t.Fatalf("expected output count override 16/44, got %d/%d", flags.PNGMainlineOutputCountMin, flags.PNGMainlineOutputCountMax)
	}
	if flags.PNGMainlineCoverageWindowTargetMin != 15 {
		t.Fatalf("expected coverage target min 15, got %d", flags.PNGMainlineCoverageWindowTargetMin)
	}
	if flags.PNGMainlineCoverageWindowReqRatio != 0.82 {
		t.Fatalf("expected coverage required ratio 0.82, got %.3f", flags.PNGMainlineCoverageWindowReqRatio)
	}
	if flags.PNGMainlineCoverageWindowMinRatio != 0.55 {
		t.Fatalf("expected coverage min ratio 0.55, got %.3f", flags.PNGMainlineCoverageWindowMinRatio)
	}
	if flags.PNGFinalQualityGuardEnabled {
		t.Fatalf("expected PNG final quality guard disabled")
	}
	if flags.PNGFinalDiversityGuardEnabled {
		t.Fatalf("expected PNG final diversity guard disabled")
	}
	if flags.PNGFinalQualityGuardMinKeep != 5 || flags.PNGFinalQualityGuardMinKeepRatio != 0.66 {
		t.Fatalf("expected final quality min keep overrides 5/0.66, got %d/%.3f", flags.PNGFinalQualityGuardMinKeep, flags.PNGFinalQualityGuardMinKeepRatio)
	}
	if flags.PNGFinalDiversityMinKeep != 6 || flags.PNGFinalDiversityMinKeepRatio != 0.73 {
		t.Fatalf("expected final diversity min keep overrides 6/0.73, got %d/%.3f", flags.PNGFinalDiversityMinKeep, flags.PNGFinalDiversityMinKeepRatio)
	}
	if !flags.PNGAliyunSuperResQualityGuard {
		t.Fatalf("expected aliyun super-res quality guard enabled")
	}
	if !flags.PNGAliyunFaceEnhanceDetectFaceGate {
		t.Fatalf("expected face detect gate enabled")
	}
	if !flags.PNGAliyunFaceEnhanceQualityGuard {
		t.Fatalf("expected face quality guard enabled")
	}
	if !flags.PNGAI2LLMRerankPostEnhance {
		t.Fatalf("expected ai2 llm rerank post-enhance enabled")
	}
	if flags.PNGAI2LLMRerankIncludeImages {
		t.Fatalf("expected ai2 llm rerank include-images disabled")
	}
}

func TestReadFeatureGateBool_InvalidFallsBackToDefault(t *testing.T) {
	t.Setenv("TEST_INVALID_FEATURE_GATE_BOOL", "abc")
	if got := readFeatureGateBool("TEST_INVALID_FEATURE_GATE_BOOL", true); !got {
		t.Fatalf("expected invalid bool fallback true")
	}
	if got := readFeatureGateBool("TEST_INVALID_FEATURE_GATE_BOOL", false); got {
		t.Fatalf("expected invalid bool fallback false")
	}
}

func TestReadFeatureGateIntAndFloat_InvalidFallsBackToDefault(t *testing.T) {
	t.Setenv("TEST_INVALID_FEATURE_GATE_INT", "abc")
	if got := readFeatureGateInt("TEST_INVALID_FEATURE_GATE_INT", 42); got != 42 {
		t.Fatalf("expected invalid int fallback 42, got %d", got)
	}
	t.Setenv("TEST_INVALID_FEATURE_GATE_FLOAT", "abc")
	if got := readFeatureGateFloat("TEST_INVALID_FEATURE_GATE_FLOAT", 0.37); got != 0.37 {
		t.Fatalf("expected invalid float fallback 0.37, got %.4f", got)
	}
}

func TestVideoJobFeatureFlags_DiversityThresholdOverrides(t *testing.T) {
	t.Setenv(featureFlagPNGFinalDiversityHammingThreshold, "7")
	t.Setenv(featureFlagPNGFinalDiversityBacktrack, "11")
	flags := loadVideoJobFeatureFlags()
	if got := flags.pngFinalDiversityHammingThreshold(5); got != 7 {
		t.Fatalf("expected diversity hamming threshold override 7, got %d", got)
	}
	if got := flags.pngFinalDiversityBacktrack(9); got != 11 {
		t.Fatalf("expected diversity backtrack override 11, got %d", got)
	}

	t.Setenv(featureFlagPNGFinalDiversityHammingThreshold, "")
	t.Setenv(featureFlagPNGFinalDiversityBacktrack, "")
	if got := flags.pngFinalDiversityHammingThreshold(5); got != 5 {
		t.Fatalf("expected diversity hamming threshold fallback 5, got %d", got)
	}
	if got := flags.pngFinalDiversityBacktrack(9); got != 9 {
		t.Fatalf("expected diversity backtrack fallback 9, got %d", got)
	}
}

func TestLoadVideoJobFeatureFlags_AliyunAndRerankNonBoolean(t *testing.T) {
	t.Setenv(featureFlagPNGAliyunSuperResMode, "")
	t.Setenv(featureFlagPNGAliyunSuperResMinShortSide, "")
	t.Setenv(featureFlagPNGAliyunSuperResMaxFrames, "")
	t.Setenv(featureFlagPNGAliyunSuperResUpscaleFactor, "")
	t.Setenv(featureFlagPNGAliyunSuperResOutputQuality, "")
	t.Setenv(featureFlagPNGAliyunSuperResCostPerImageCNY, "")
	t.Setenv(featureFlagPNGAliyunSuperResMaxCostPerJobCNY, "")
	t.Setenv(featureFlagPNGAliyunSuperResTimeoutSeconds, "")
	t.Setenv(featureFlagPNGAliyunSuperResReplaceMinGain, "")
	t.Setenv(featureFlagPNGAliyunFaceMode, "")
	t.Setenv(featureFlagPNGAliyunFaceMinShortSide, "")
	t.Setenv(featureFlagPNGAliyunFaceMaxFrames, "")
	t.Setenv(featureFlagPNGAliyunFaceTimeoutSeconds, "")
	t.Setenv(featureFlagPNGAliyunFaceCostPerImageCNY, "")
	t.Setenv(featureFlagPNGAliyunFaceMaxCostPerJobCNY, "")
	t.Setenv(featureFlagPNGAliyunFaceMinFaceAreaRatio, "")
	t.Setenv(featureFlagPNGAliyunFaceMinFaceConfidence, "")
	t.Setenv(featureFlagPNGAliyunFaceSkipHighBlurScore, "")
	t.Setenv(featureFlagPNGAliyunFaceReplaceMinGain, "")
	t.Setenv(featureFlagPNGAliyunFaceDetectFaceMaxCount, "")
	t.Setenv(featureFlagPNGAI2LLMRerankMode, "")
	t.Setenv(featureFlagPNGAI2LLMRerankProvider, "")
	t.Setenv(featureFlagPNGAI2LLMRerankModel, "")
	t.Setenv(featureFlagPNGAI2LLMRerankEndpoint, "")
	t.Setenv(featureFlagPNGAI2LLMRerankAPIKey, "")
	t.Setenv(featureFlagPNGAI2LLMRerankTimeoutSeconds, "")
	t.Setenv(featureFlagPNGAI2LLMRerankMaxTokens, "")
	t.Setenv(featureFlagPNGAI2LLMRerankPromptVersion, "")
	t.Setenv(featureFlagPNGAI2LLMRerankMaxCandidates, "")
	t.Setenv(featureFlagPNGAI2LLMRerankMinCandidates, "")
	t.Setenv(featureFlagPNGAI2LLMRerankImageMaxCandidates, "")
	t.Setenv(featureFlagPNGAI2LLMRerankImageMaxSide, "")
	t.Setenv(featureFlagPNGAI2LLMRerankImageJPEGQuality, "")
	t.Setenv(featureFlagPNGAI2LLMRerankImageMaxBytes, "")

	flags := loadVideoJobFeatureFlags()
	if flags.PNGAliyunSuperResMode != pngAliyunSuperResModeOn {
		t.Fatalf("expected super-res mode default on, got %q", flags.PNGAliyunSuperResMode)
	}
	if flags.PNGAliyunFaceEnhanceMode != pngAliyunFaceEnhanceModeAuto {
		t.Fatalf("expected face mode default auto, got %q", flags.PNGAliyunFaceEnhanceMode)
	}
	if flags.PNGAI2LLMRerankMode != pngAI2LLMRerankModeOff {
		t.Fatalf("expected rerank mode default off, got %q", flags.PNGAI2LLMRerankMode)
	}
	if flags.PNGAI2LLMRerankTimeoutSeconds != 0 || flags.PNGAI2LLMRerankMaxTokens != 0 {
		t.Fatalf("expected rerank timeout/max_tokens default 0/0, got %d/%d", flags.PNGAI2LLMRerankTimeoutSeconds, flags.PNGAI2LLMRerankMaxTokens)
	}

	t.Setenv(featureFlagPNGAliyunSuperResMode, "shadow")
	t.Setenv(featureFlagPNGAliyunSuperResMinShortSide, "1024")
	t.Setenv(featureFlagPNGAliyunSuperResMaxFrames, "8")
	t.Setenv(featureFlagPNGAliyunSuperResUpscaleFactor, "3")
	t.Setenv(featureFlagPNGAliyunSuperResOutputQuality, "88")
	t.Setenv(featureFlagPNGAliyunSuperResCostPerImageCNY, "0.03")
	t.Setenv(featureFlagPNGAliyunSuperResMaxCostPerJobCNY, "0.12")
	t.Setenv(featureFlagPNGAliyunSuperResTimeoutSeconds, "31")
	t.Setenv(featureFlagPNGAliyunSuperResReplaceMinGain, "0.012")
	t.Setenv(featureFlagPNGAliyunFaceMode, "on")
	t.Setenv(featureFlagPNGAliyunFaceMinShortSide, "420")
	t.Setenv(featureFlagPNGAliyunFaceMaxFrames, "5")
	t.Setenv(featureFlagPNGAliyunFaceTimeoutSeconds, "29")
	t.Setenv(featureFlagPNGAliyunFaceCostPerImageCNY, "0.02")
	t.Setenv(featureFlagPNGAliyunFaceMaxCostPerJobCNY, "0.07")
	t.Setenv(featureFlagPNGAliyunFaceMinFaceAreaRatio, "0.08")
	t.Setenv(featureFlagPNGAliyunFaceMinFaceConfidence, "0.77")
	t.Setenv(featureFlagPNGAliyunFaceSkipHighBlurScore, "1300")
	t.Setenv(featureFlagPNGAliyunFaceReplaceMinGain, "0.02")
	t.Setenv(featureFlagPNGAliyunFaceDetectFaceMaxCount, "6")
	t.Setenv(featureFlagPNGAI2LLMRerankMode, "on")
	t.Setenv(featureFlagPNGAI2LLMRerankProvider, "qwen")
	t.Setenv(featureFlagPNGAI2LLMRerankModel, "qwen3.5-omni-flash")
	t.Setenv(featureFlagPNGAI2LLMRerankEndpoint, "https://example.com/v1")
	t.Setenv(featureFlagPNGAI2LLMRerankAPIKey, "test-key")
	t.Setenv(featureFlagPNGAI2LLMRerankTimeoutSeconds, "32")
	t.Setenv(featureFlagPNGAI2LLMRerankMaxTokens, "1400")
	t.Setenv(featureFlagPNGAI2LLMRerankPromptVersion, "png_ai2_rerank_v2")
	t.Setenv(featureFlagPNGAI2LLMRerankMaxCandidates, "26")
	t.Setenv(featureFlagPNGAI2LLMRerankMinCandidates, "7")
	t.Setenv(featureFlagPNGAI2LLMRerankImageMaxCandidates, "14")
	t.Setenv(featureFlagPNGAI2LLMRerankImageMaxSide, "960")
	t.Setenv(featureFlagPNGAI2LLMRerankImageJPEGQuality, "84")
	t.Setenv(featureFlagPNGAI2LLMRerankImageMaxBytes, "777777")

	flags = loadVideoJobFeatureFlags()
	if flags.PNGAliyunSuperResMode != "shadow" || flags.PNGAliyunSuperResMinShortSide != 1024 || flags.PNGAliyunSuperResMaxFrames != 8 {
		t.Fatalf("unexpected super-res override: %+v", flags)
	}
	if flags.PNGAliyunFaceEnhanceMode != "on" || flags.PNGAliyunFaceEnhanceDetectFaceMaxCount != 6 {
		t.Fatalf("unexpected face override: %+v", flags)
	}
	if flags.PNGAI2LLMRerankMode != "on" || flags.PNGAI2LLMRerankModel != "qwen3.5-omni-flash" {
		t.Fatalf("unexpected rerank override: %+v", flags)
	}
	if flags.PNGAI2LLMRerankTimeoutSeconds != 32 || flags.PNGAI2LLMRerankMaxTokens != 1400 {
		t.Fatalf("unexpected rerank timeout/max_tokens override: %d/%d", flags.PNGAI2LLMRerankTimeoutSeconds, flags.PNGAI2LLMRerankMaxTokens)
	}
}
