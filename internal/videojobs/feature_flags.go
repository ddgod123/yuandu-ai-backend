package videojobs

import (
	"os"
	"strconv"
	"strings"
)

const (
	featureFlagPNGEnhancementStageEnabled         = "PNG_ENHANCEMENT_STAGE_ENABLED"
	featureFlagPNGMainlineOutputCountGuardEnabled = "PNG_MAINLINE_OUTPUT_COUNT_GUARD_ENABLED"
	featureFlagPNGMainlineOutputCountMin          = "PNG_MAINLINE_OUTPUT_COUNT_MIN"
	featureFlagPNGMainlineOutputCountMax          = "PNG_MAINLINE_OUTPUT_COUNT_MAX"
	featureFlagPNGMainlineCoverageWindowGuard     = "PNG_MAINLINE_COVERAGE_WINDOW_GUARD_ENABLED"
	featureFlagPNGMainlineCoverageWindowTargetMin = "PNG_MAINLINE_COVERAGE_WINDOW_TARGET_MIN"
	featureFlagPNGMainlineCoverageWindowReqRatio  = "PNG_MAINLINE_COVERAGE_WINDOW_REQUIRED_RATIO"
	featureFlagPNGMainlineCoverageWindowMinRatio  = "PNG_MAINLINE_COVERAGE_WINDOW_MIN_RATIO"
	featureFlagPNGFinalQualityGuardEnabled        = "PNG_FINAL_QUALITY_GUARD_ENABLED"
	featureFlagPNGFinalQualityGuardMinKeep        = "PNG_FINAL_QUALITY_GUARD_MIN_KEEP"
	featureFlagPNGFinalQualityGuardMinKeepRatio   = "PNG_FINAL_QUALITY_GUARD_MIN_KEEP_RATIO"
	featureFlagPNGFinalDiversityGuardEnabled      = "PNG_FINAL_DIVERSITY_GUARD_ENABLED"
	featureFlagPNGFinalDiversityHammingThreshold  = "PNG_FINAL_DIVERSITY_HAMMING_THRESHOLD"
	featureFlagPNGFinalDiversityBacktrack         = "PNG_FINAL_DIVERSITY_BACKTRACK"
	featureFlagPNGFinalDiversityMinKeep           = "PNG_FINAL_DIVERSITY_MIN_KEEP"
	featureFlagPNGFinalDiversityMinKeepRatio      = "PNG_FINAL_DIVERSITY_MIN_KEEP_RATIO"
	featureFlagPNGAliyunSuperResQualityGuard      = "PNG_ALIYUN_SUPERRES_QUALITY_GUARD_ENABLED"
	featureFlagPNGAliyunSuperResMode              = "PNG_ALIYUN_SUPERRES_MODE"
	featureFlagPNGAliyunSuperResMinShortSide      = "PNG_ALIYUN_SUPERRES_MIN_SHORT_SIDE"
	featureFlagPNGAliyunSuperResMaxFrames         = "PNG_ALIYUN_SUPERRES_MAX_FRAMES"
	featureFlagPNGAliyunSuperResUpscaleFactor     = "PNG_ALIYUN_SUPERRES_UPSCALE_FACTOR"
	featureFlagPNGAliyunSuperResOutputQuality     = "PNG_ALIYUN_SUPERRES_OUTPUT_QUALITY"
	featureFlagPNGAliyunSuperResCostPerImageCNY   = "PNG_ALIYUN_SUPERRES_COST_PER_IMAGE_CNY"
	featureFlagPNGAliyunSuperResMaxCostPerJobCNY  = "PNG_ALIYUN_SUPERRES_MAX_COST_PER_JOB_CNY"
	featureFlagPNGAliyunSuperResTimeoutSeconds    = "PNG_ALIYUN_SUPERRES_TIMEOUT_SECONDS"
	featureFlagPNGAliyunSuperResReplaceMinGain    = "PNG_ALIYUN_SUPERRES_REPLACE_MIN_GAIN"
	featureFlagPNGAliyunFaceDetectGate            = "PNG_ALIYUN_FACE_ENHANCE_DETECT_FACE_GATE"
	featureFlagPNGAliyunFaceQualityGuard          = "PNG_ALIYUN_FACE_ENHANCE_QUALITY_GUARD_ENABLED"
	featureFlagPNGAliyunFaceMode                  = "PNG_ALIYUN_FACE_ENHANCE_MODE"
	featureFlagPNGAliyunFaceMinShortSide          = "PNG_ALIYUN_FACE_ENHANCE_MIN_SHORT_SIDE"
	featureFlagPNGAliyunFaceMaxFrames             = "PNG_ALIYUN_FACE_ENHANCE_MAX_FRAMES"
	featureFlagPNGAliyunFaceTimeoutSeconds        = "PNG_ALIYUN_FACE_ENHANCE_TIMEOUT_SECONDS"
	featureFlagPNGAliyunFaceCostPerImageCNY       = "PNG_ALIYUN_FACE_ENHANCE_COST_PER_IMAGE_CNY"
	featureFlagPNGAliyunFaceMaxCostPerJobCNY      = "PNG_ALIYUN_FACE_ENHANCE_MAX_COST_PER_JOB_CNY"
	featureFlagPNGAliyunFaceMinFaceAreaRatio      = "PNG_ALIYUN_FACE_ENHANCE_MIN_FACE_AREA_RATIO"
	featureFlagPNGAliyunFaceMinFaceConfidence     = "PNG_ALIYUN_FACE_ENHANCE_MIN_FACE_CONFIDENCE"
	featureFlagPNGAliyunFaceSkipHighBlurScore     = "PNG_ALIYUN_FACE_ENHANCE_SKIP_HIGH_BLUR_SCORE"
	featureFlagPNGAliyunFaceReplaceMinGain        = "PNG_ALIYUN_FACE_ENHANCE_REPLACE_MIN_GAIN"
	featureFlagPNGAliyunFaceDetectFaceMaxCount    = "PNG_ALIYUN_FACE_ENHANCE_DETECT_FACE_MAX_FACE_COUNT"
	featureFlagPNGAI2LLMRerankMode                = "PNG_AI2_LLM_RERANK_MODE"
	featureFlagPNGAI2LLMRerankProvider            = "PNG_AI2_LLM_RERANK_PROVIDER"
	featureFlagPNGAI2LLMRerankModel               = "PNG_AI2_LLM_RERANK_MODEL"
	featureFlagPNGAI2LLMRerankEndpoint            = "PNG_AI2_LLM_RERANK_ENDPOINT"
	featureFlagPNGAI2LLMRerankAPIKey              = "PNG_AI2_LLM_RERANK_API_KEY"
	featureFlagPNGAI2LLMRerankTimeoutSeconds      = "PNG_AI2_LLM_RERANK_TIMEOUT_SECONDS"
	featureFlagPNGAI2LLMRerankMaxTokens           = "PNG_AI2_LLM_RERANK_MAX_TOKENS"
	featureFlagPNGAI2LLMRerankPromptVersion       = "PNG_AI2_LLM_RERANK_PROMPT_VERSION"
	featureFlagPNGAI2LLMRerankMaxCandidates       = "PNG_AI2_LLM_RERANK_MAX_CANDIDATES"
	featureFlagPNGAI2LLMRerankMinCandidates       = "PNG_AI2_LLM_RERANK_MIN_CANDIDATES"
	featureFlagPNGAI2LLMRerankImageMaxCandidates  = "PNG_AI2_LLM_RERANK_IMAGE_MAX_CANDIDATES"
	featureFlagPNGAI2LLMRerankImageMaxSide        = "PNG_AI2_LLM_RERANK_IMAGE_MAX_SIDE"
	featureFlagPNGAI2LLMRerankImageJPEGQuality    = "PNG_AI2_LLM_RERANK_IMAGE_JPEG_QUALITY"
	featureFlagPNGAI2LLMRerankImageMaxBytes       = "PNG_AI2_LLM_RERANK_IMAGE_MAX_BYTES"
	featureFlagPNGAI2LLMRerankPostEnhance         = "PNG_AI2_LLM_RERANK_POST_ENHANCE"
	featureFlagPNGAI2LLMRerankIncludeImages       = "PNG_AI2_LLM_RERANK_INCLUDE_IMAGES"
)

type videoJobFeatureFlags struct {
	PNGEnhancementStageEnabled             bool
	PNGMainlineOutputCountGuardEnabled     bool
	PNGMainlineCoverageWindowGuardEnabled  bool
	PNGMainlineOutputCountMin              int
	PNGMainlineOutputCountMax              int
	PNGMainlineCoverageWindowTargetMin     int
	PNGMainlineCoverageWindowReqRatio      float64
	PNGMainlineCoverageWindowMinRatio      float64
	PNGFinalQualityGuardEnabled            bool
	PNGFinalQualityGuardMinKeep            int
	PNGFinalQualityGuardMinKeepRatio       float64
	PNGFinalDiversityGuardEnabled          bool
	PNGFinalDiversityMinKeep               int
	PNGFinalDiversityMinKeepRatio          float64
	PNGAliyunSuperResQualityGuard          bool
	PNGAliyunSuperResMode                  string
	PNGAliyunSuperResMinShortSide          int
	PNGAliyunSuperResMaxFrames             int
	PNGAliyunSuperResUpscaleFactor         int
	PNGAliyunSuperResOutputQuality         int
	PNGAliyunSuperResCostPerImageCNY       float64
	PNGAliyunSuperResMaxCostPerJobCNY      float64
	PNGAliyunSuperResTimeoutSeconds        int
	PNGAliyunSuperResReplaceMinGain        float64
	PNGAliyunFaceEnhanceDetectFaceGate     bool
	PNGAliyunFaceEnhanceQualityGuard       bool
	PNGAliyunFaceEnhanceMode               string
	PNGAliyunFaceEnhanceMinShortSide       int
	PNGAliyunFaceEnhanceMaxFrames          int
	PNGAliyunFaceEnhanceTimeoutSeconds     int
	PNGAliyunFaceEnhanceCostPerImageCNY    float64
	PNGAliyunFaceEnhanceMaxCostPerJobCNY   float64
	PNGAliyunFaceEnhanceMinFaceAreaRatio   float64
	PNGAliyunFaceEnhanceMinFaceConfidence  float64
	PNGAliyunFaceEnhanceSkipHighBlurScore  float64
	PNGAliyunFaceEnhanceReplaceMinGain     float64
	PNGAliyunFaceEnhanceDetectFaceMaxCount int
	PNGAI2LLMRerankMode                    string
	PNGAI2LLMRerankProvider                string
	PNGAI2LLMRerankModel                   string
	PNGAI2LLMRerankEndpoint                string
	PNGAI2LLMRerankAPIKey                  string
	PNGAI2LLMRerankTimeoutSeconds          int
	PNGAI2LLMRerankMaxTokens               int
	PNGAI2LLMRerankPromptVersion           string
	PNGAI2LLMRerankMaxCandidates           int
	PNGAI2LLMRerankMinCandidates           int
	PNGAI2LLMRerankImageMaxCandidates      int
	PNGAI2LLMRerankImageMaxSide            int
	PNGAI2LLMRerankImageJPEGQuality        int
	PNGAI2LLMRerankImageMaxBytes           int
	PNGAI2LLMRerankPostEnhance             bool
	PNGAI2LLMRerankIncludeImages           bool
}

func loadVideoJobFeatureFlags() videoJobFeatureFlags {
	return videoJobFeatureFlags{
		PNGEnhancementStageEnabled:             readFeatureGateBool(featureFlagPNGEnhancementStageEnabled, false),
		PNGMainlineOutputCountGuardEnabled:     readFeatureGateBool(featureFlagPNGMainlineOutputCountGuardEnabled, true),
		PNGMainlineCoverageWindowGuardEnabled:  readFeatureGateBool(featureFlagPNGMainlineCoverageWindowGuard, true),
		PNGMainlineOutputCountMin:              readFeatureGateInt(featureFlagPNGMainlineOutputCountMin, 12),
		PNGMainlineOutputCountMax:              readFeatureGateInt(featureFlagPNGMainlineOutputCountMax, 36),
		PNGMainlineCoverageWindowTargetMin:     readFeatureGateInt(featureFlagPNGMainlineCoverageWindowTargetMin, 12),
		PNGMainlineCoverageWindowReqRatio:      readFeatureGateFloat(featureFlagPNGMainlineCoverageWindowReqRatio, 0.75),
		PNGMainlineCoverageWindowMinRatio:      readFeatureGateFloat(featureFlagPNGMainlineCoverageWindowMinRatio, 0.45),
		PNGFinalQualityGuardEnabled:            readFeatureGateBool(featureFlagPNGFinalQualityGuardEnabled, true),
		PNGFinalQualityGuardMinKeep:            readFeatureGateInt(featureFlagPNGFinalQualityGuardMinKeep, 4),
		PNGFinalQualityGuardMinKeepRatio:       readFeatureGateFloat(featureFlagPNGFinalQualityGuardMinKeepRatio, 0.55),
		PNGFinalDiversityGuardEnabled:          readFeatureGateBool(featureFlagPNGFinalDiversityGuardEnabled, true),
		PNGFinalDiversityMinKeep:               readFeatureGateInt(featureFlagPNGFinalDiversityMinKeep, 4),
		PNGFinalDiversityMinKeepRatio:          readFeatureGateFloat(featureFlagPNGFinalDiversityMinKeepRatio, 0.6),
		PNGAliyunSuperResQualityGuard:          readFeatureGateBool(featureFlagPNGAliyunSuperResQualityGuard, true),
		PNGAliyunSuperResMode:                  readFeatureGateString(featureFlagPNGAliyunSuperResMode, pngAliyunSuperResModeOn),
		PNGAliyunSuperResMinShortSide:          readFeatureGateInt(featureFlagPNGAliyunSuperResMinShortSide, 960),
		PNGAliyunSuperResMaxFrames:             readFeatureGateInt(featureFlagPNGAliyunSuperResMaxFrames, 4),
		PNGAliyunSuperResUpscaleFactor:         readFeatureGateInt(featureFlagPNGAliyunSuperResUpscaleFactor, 2),
		PNGAliyunSuperResOutputQuality:         readFeatureGateInt(featureFlagPNGAliyunSuperResOutputQuality, 95),
		PNGAliyunSuperResCostPerImageCNY:       readFeatureGateFloat(featureFlagPNGAliyunSuperResCostPerImageCNY, 0.02),
		PNGAliyunSuperResMaxCostPerJobCNY:      readFeatureGateFloat(featureFlagPNGAliyunSuperResMaxCostPerJobCNY, 0.08),
		PNGAliyunSuperResTimeoutSeconds:        readFeatureGateInt(featureFlagPNGAliyunSuperResTimeoutSeconds, 25),
		PNGAliyunSuperResReplaceMinGain:        readFeatureGateFloat(featureFlagPNGAliyunSuperResReplaceMinGain, 0.005),
		PNGAliyunFaceEnhanceDetectFaceGate:     readFeatureGateBool(featureFlagPNGAliyunFaceDetectGate, true),
		PNGAliyunFaceEnhanceQualityGuard:       readFeatureGateBool(featureFlagPNGAliyunFaceQualityGuard, true),
		PNGAliyunFaceEnhanceMode:               readFeatureGateString(featureFlagPNGAliyunFaceMode, pngAliyunFaceEnhanceModeAuto),
		PNGAliyunFaceEnhanceMinShortSide:       readFeatureGateInt(featureFlagPNGAliyunFaceMinShortSide, 360),
		PNGAliyunFaceEnhanceMaxFrames:          readFeatureGateInt(featureFlagPNGAliyunFaceMaxFrames, 2),
		PNGAliyunFaceEnhanceTimeoutSeconds:     readFeatureGateInt(featureFlagPNGAliyunFaceTimeoutSeconds, 25),
		PNGAliyunFaceEnhanceCostPerImageCNY:    readFeatureGateFloat(featureFlagPNGAliyunFaceCostPerImageCNY, 0.01),
		PNGAliyunFaceEnhanceMaxCostPerJobCNY:   readFeatureGateFloat(featureFlagPNGAliyunFaceMaxCostPerJobCNY, 0.02),
		PNGAliyunFaceEnhanceMinFaceAreaRatio:   readFeatureGateFloat(featureFlagPNGAliyunFaceMinFaceAreaRatio, 0.03),
		PNGAliyunFaceEnhanceMinFaceConfidence:  readFeatureGateFloat(featureFlagPNGAliyunFaceMinFaceConfidence, 0.62),
		PNGAliyunFaceEnhanceSkipHighBlurScore:  readFeatureGateFloat(featureFlagPNGAliyunFaceSkipHighBlurScore, 1200),
		PNGAliyunFaceEnhanceReplaceMinGain:     readFeatureGateFloat(featureFlagPNGAliyunFaceReplaceMinGain, 0.005),
		PNGAliyunFaceEnhanceDetectFaceMaxCount: readFeatureGateInt(featureFlagPNGAliyunFaceDetectFaceMaxCount, 3),
		PNGAI2LLMRerankMode:                    readFeatureGateString(featureFlagPNGAI2LLMRerankMode, pngAI2LLMRerankModeOff),
		PNGAI2LLMRerankProvider:                readFeatureGateString(featureFlagPNGAI2LLMRerankProvider, ""),
		PNGAI2LLMRerankModel:                   readFeatureGateString(featureFlagPNGAI2LLMRerankModel, ""),
		PNGAI2LLMRerankEndpoint:                readFeatureGateString(featureFlagPNGAI2LLMRerankEndpoint, ""),
		PNGAI2LLMRerankAPIKey:                  readFeatureGateString(featureFlagPNGAI2LLMRerankAPIKey, ""),
		PNGAI2LLMRerankTimeoutSeconds:          readFeatureGateInt(featureFlagPNGAI2LLMRerankTimeoutSeconds, 0),
		PNGAI2LLMRerankMaxTokens:               readFeatureGateInt(featureFlagPNGAI2LLMRerankMaxTokens, 0),
		PNGAI2LLMRerankPromptVersion:           readFeatureGateString(featureFlagPNGAI2LLMRerankPromptVersion, ""),
		PNGAI2LLMRerankMaxCandidates:           readFeatureGateInt(featureFlagPNGAI2LLMRerankMaxCandidates, 18),
		PNGAI2LLMRerankMinCandidates:           readFeatureGateInt(featureFlagPNGAI2LLMRerankMinCandidates, 4),
		PNGAI2LLMRerankImageMaxCandidates:      readFeatureGateInt(featureFlagPNGAI2LLMRerankImageMaxCandidates, 10),
		PNGAI2LLMRerankImageMaxSide:            readFeatureGateInt(featureFlagPNGAI2LLMRerankImageMaxSide, 640),
		PNGAI2LLMRerankImageJPEGQuality:        readFeatureGateInt(featureFlagPNGAI2LLMRerankImageJPEGQuality, 72),
		PNGAI2LLMRerankImageMaxBytes:           readFeatureGateInt(featureFlagPNGAI2LLMRerankImageMaxBytes, 900*1024),
		PNGAI2LLMRerankPostEnhance:             readFeatureGateBool(featureFlagPNGAI2LLMRerankPostEnhance, false),
		PNGAI2LLMRerankIncludeImages:           readFeatureGateBool(featureFlagPNGAI2LLMRerankIncludeImages, true),
	}
}

func readFeatureGateBool(key string, def bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(strings.TrimSpace(key))))
	if raw == "" {
		return def
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func readFeatureGateInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(strings.TrimSpace(key)))
	if raw == "" {
		return def
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return value
}

func readFeatureGateFloat(key string, def float64) float64 {
	raw := strings.TrimSpace(os.Getenv(strings.TrimSpace(key)))
	if raw == "" {
		return def
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return value
}

func readFeatureGateString(key, def string) string {
	raw := strings.TrimSpace(os.Getenv(strings.TrimSpace(key)))
	if raw == "" {
		return strings.TrimSpace(def)
	}
	return raw
}

func (f videoJobFeatureFlags) pngFinalDiversityHammingThreshold(def int) int {
	return readFeatureGateInt(featureFlagPNGFinalDiversityHammingThreshold, def)
}

func (f videoJobFeatureFlags) pngFinalDiversityBacktrack(def int) int {
	return readFeatureGateInt(featureFlagPNGFinalDiversityBacktrack, def)
}
