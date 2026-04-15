package videojobs

import (
	"context"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	openapiutil "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	facebody "github.com/alibabacloud-go/facebody-20191230/v6/client"
	imageenhan "github.com/alibabacloud-go/imageenhan-20190930/v2/client"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/alibabacloud-go/tea/tea"

	"emoji/internal/models"
)

const (
	pngAliyunSuperResModeOff    = "off"
	pngAliyunSuperResModeShadow = "shadow"
	pngAliyunSuperResModeOn     = "on"

	pngAliyunFaceEnhanceModeOff  = "off"
	pngAliyunFaceEnhanceModeAuto = "auto"
	pngAliyunFaceEnhanceModeOn   = "on"
)

type pngAliyunSuperResConfig struct {
	Mode             string
	RegionID         string
	Endpoint         string
	MinShortSide     int
	MaxFrames        int
	UpscaleFactor    int
	OutputQuality    int
	CostPerImageCNY  float64
	MaxCostPerJobCNY float64
	TimeoutSec       int
	QualityGuard     bool
	ReplaceMinGain   float64
}

type pngAliyunFaceEnhanceConfig struct {
	Mode                   string
	RegionID               string
	Endpoint               string
	MinShortSide           int
	MaxFrames              int
	TimeoutSec             int
	CostPerImageCNY        float64
	MaxCostPerJobCNY       float64
	DetectFaceGate         bool
	MinFaceAreaRatio       float64
	MinFaceConfidence      float64
	SkipHighBlurScore      float64
	QualityGuard           bool
	ReplaceMinGain         float64
	DetectFaceMaxFaceCount int
}

type aliyunVisionClient struct {
	imageClient *imageenhan.Client
	endpoint    string
}

type aliyunFaceVisionClient struct {
	faceClient *facebody.Client
	endpoint   string
}

type faceDetectionSummary struct {
	FaceCount       int
	MaxAreaRatio    float64
	MaxProbability  float64
	QualityScoreMax float64
}

func loadPNGAliyunSuperResConfig() pngAliyunSuperResConfig {
	featureFlags := loadVideoJobFeatureFlags()
	mode := strings.ToLower(strings.TrimSpace(featureFlags.PNGAliyunSuperResMode))
	switch mode {
	case pngAliyunSuperResModeOff, pngAliyunSuperResModeShadow, pngAliyunSuperResModeOn:
	default:
		// 主线默认开启超分；如需关闭可显式配置 off
		mode = pngAliyunSuperResModeOn
	}

	cfg := pngAliyunSuperResConfig{
		Mode:             mode,
		RegionID:         strings.TrimSpace(os.Getenv("ALIYUN_VISION_REGION_ID")),
		Endpoint:         strings.TrimSpace(os.Getenv("ALIYUN_VISION_IMAGEENHAN_ENDPOINT")),
		MinShortSide:     featureFlags.PNGAliyunSuperResMinShortSide,
		MaxFrames:        featureFlags.PNGAliyunSuperResMaxFrames,
		UpscaleFactor:    featureFlags.PNGAliyunSuperResUpscaleFactor,
		OutputQuality:    featureFlags.PNGAliyunSuperResOutputQuality,
		CostPerImageCNY:  featureFlags.PNGAliyunSuperResCostPerImageCNY,
		MaxCostPerJobCNY: featureFlags.PNGAliyunSuperResMaxCostPerJobCNY,
		TimeoutSec:       featureFlags.PNGAliyunSuperResTimeoutSeconds,
		QualityGuard:     featureFlags.PNGAliyunSuperResQualityGuard,
		ReplaceMinGain:   featureFlags.PNGAliyunSuperResReplaceMinGain,
	}
	if cfg.RegionID == "" {
		cfg.RegionID = "cn-shanghai"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "imageenhan.cn-shanghai.aliyuncs.com"
	}
	if cfg.MinShortSide < 1 {
		cfg.MinShortSide = 960
	}
	if cfg.MaxFrames < 1 {
		cfg.MaxFrames = 4
	}
	if cfg.MaxFrames > 16 {
		cfg.MaxFrames = 16
	}
	if cfg.UpscaleFactor < 2 {
		cfg.UpscaleFactor = 2
	}
	if cfg.UpscaleFactor > 4 {
		cfg.UpscaleFactor = 4
	}
	if cfg.OutputQuality < 1 {
		cfg.OutputQuality = 95
	}
	if cfg.OutputQuality > 100 {
		cfg.OutputQuality = 100
	}
	if cfg.CostPerImageCNY < 0 {
		cfg.CostPerImageCNY = 0
	}
	if cfg.MaxCostPerJobCNY < 0 {
		cfg.MaxCostPerJobCNY = 0
	}
	if cfg.TimeoutSec < 5 {
		cfg.TimeoutSec = 25
	}
	cfg.ReplaceMinGain = clampFloat(cfg.ReplaceMinGain, -0.05, 0.2)
	return cfg
}

func loadPNGAliyunFaceEnhanceConfig() pngAliyunFaceEnhanceConfig {
	featureFlags := loadVideoJobFeatureFlags()
	mode := strings.ToLower(strings.TrimSpace(featureFlags.PNGAliyunFaceEnhanceMode))
	switch mode {
	case pngAliyunFaceEnhanceModeOff, pngAliyunFaceEnhanceModeAuto, pngAliyunFaceEnhanceModeOn:
	default:
		// 人脸增强默认自动：仅 portrait 场景尝试
		mode = pngAliyunFaceEnhanceModeAuto
	}

	cfg := pngAliyunFaceEnhanceConfig{
		Mode:                   mode,
		RegionID:               strings.TrimSpace(os.Getenv("ALIYUN_VISION_REGION_ID")),
		Endpoint:               strings.TrimSpace(os.Getenv("ALIYUN_VISION_FACEBODY_ENDPOINT")),
		MinShortSide:           featureFlags.PNGAliyunFaceEnhanceMinShortSide,
		MaxFrames:              featureFlags.PNGAliyunFaceEnhanceMaxFrames,
		TimeoutSec:             featureFlags.PNGAliyunFaceEnhanceTimeoutSeconds,
		CostPerImageCNY:        featureFlags.PNGAliyunFaceEnhanceCostPerImageCNY,
		MaxCostPerJobCNY:       featureFlags.PNGAliyunFaceEnhanceMaxCostPerJobCNY,
		DetectFaceGate:         featureFlags.PNGAliyunFaceEnhanceDetectFaceGate,
		MinFaceAreaRatio:       featureFlags.PNGAliyunFaceEnhanceMinFaceAreaRatio,
		MinFaceConfidence:      featureFlags.PNGAliyunFaceEnhanceMinFaceConfidence,
		SkipHighBlurScore:      featureFlags.PNGAliyunFaceEnhanceSkipHighBlurScore,
		QualityGuard:           featureFlags.PNGAliyunFaceEnhanceQualityGuard,
		ReplaceMinGain:         featureFlags.PNGAliyunFaceEnhanceReplaceMinGain,
		DetectFaceMaxFaceCount: featureFlags.PNGAliyunFaceEnhanceDetectFaceMaxCount,
	}
	if cfg.RegionID == "" {
		cfg.RegionID = "cn-shanghai"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "facebody.cn-shanghai.aliyuncs.com"
	}
	if cfg.MinShortSide < 1 {
		cfg.MinShortSide = 360
	}
	if cfg.MaxFrames < 1 {
		cfg.MaxFrames = 2
	}
	if cfg.MaxFrames > 12 {
		cfg.MaxFrames = 12
	}
	if cfg.TimeoutSec < 5 {
		cfg.TimeoutSec = 25
	}
	if cfg.CostPerImageCNY < 0 {
		cfg.CostPerImageCNY = 0
	}
	if cfg.MaxCostPerJobCNY < 0 {
		cfg.MaxCostPerJobCNY = 0
	}
	cfg.MinFaceAreaRatio = clampFloat(cfg.MinFaceAreaRatio, 0.005, 0.6)
	cfg.MinFaceConfidence = clampFloat(cfg.MinFaceConfidence, 0, 1)
	cfg.SkipHighBlurScore = clampFloat(cfg.SkipHighBlurScore, 0, 100000)
	cfg.ReplaceMinGain = clampFloat(cfg.ReplaceMinGain, -0.05, 0.2)
	cfg.DetectFaceMaxFaceCount = clampInt(cfg.DetectFaceMaxFaceCount, 1, 8)
	return cfg
}

func newAliyunVisionClient(cfg pngAliyunSuperResConfig) (*aliyunVisionClient, error) {
	ak := strings.TrimSpace(os.Getenv("ALIYUN_ACCESS_KEY_ID"))
	sk := strings.TrimSpace(os.Getenv("ALIYUN_ACCESS_KEY_SECRET"))
	if ak == "" || sk == "" {
		return nil, errors.New("missing ALIYUN_ACCESS_KEY_ID / ALIYUN_ACCESS_KEY_SECRET")
	}

	openapiCfg := &openapiutil.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String(cfg.Endpoint),
		RegionId:        tea.String(cfg.RegionID),
	}
	imageClient, err := imageenhan.NewClient(openapiCfg)
	if err != nil {
		return nil, err
	}
	return &aliyunVisionClient{
		imageClient: imageClient,
		endpoint:    cfg.Endpoint,
	}, nil
}

func newAliyunFaceVisionClient(cfg pngAliyunFaceEnhanceConfig) (*aliyunFaceVisionClient, error) {
	ak := strings.TrimSpace(os.Getenv("ALIYUN_ACCESS_KEY_ID"))
	sk := strings.TrimSpace(os.Getenv("ALIYUN_ACCESS_KEY_SECRET"))
	if ak == "" || sk == "" {
		return nil, errors.New("missing ALIYUN_ACCESS_KEY_ID / ALIYUN_ACCESS_KEY_SECRET")
	}

	openapiCfg := &openapiutil.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String(cfg.Endpoint),
		RegionId:        tea.String(cfg.RegionID),
	}
	faceClient, err := facebody.NewClient(openapiCfg)
	if err != nil {
		return nil, err
	}
	return &aliyunFaceVisionClient{
		faceClient: faceClient,
		endpoint:   cfg.Endpoint,
	}, nil
}

func (c *aliyunVisionClient) makeSuperResolutionFromFile(
	srcPath string,
	upscaleFactor int,
	outputQuality int,
	timeoutSec int,
) (url string, requestID string, durationMs int64, err error) {
	if c == nil || c.imageClient == nil {
		return "", "", 0, errors.New("aliyun image client is nil")
	}
	file, err := os.Open(strings.TrimSpace(srcPath))
	if err != nil {
		return "", "", 0, err
	}
	defer file.Close()

	req := &imageenhan.MakeSuperResolutionImageAdvanceRequest{
		UrlObject: file,
	}
	req.SetOutputFormat("png")
	req.SetUpscaleFactor(int64(upscaleFactor))
	req.SetOutputQuality(int64(outputQuality))

	runtime := &dara.RuntimeOptions{}
	runtime.SetReadTimeout(timeoutSec * 1000)
	runtime.SetConnectTimeout(timeoutSec * 1000)

	started := time.Now()
	resp, err := c.imageClient.MakeSuperResolutionImageAdvance(req, runtime)
	durationMs = clampDurationMillis(started)
	if err != nil {
		return "", "", durationMs, err
	}
	if resp == nil || resp.Body == nil || resp.Body.Data == nil || resp.Body.Data.Url == nil {
		return "", "", durationMs, errors.New("aliyun MakeSuperResolutionImage response url is empty")
	}
	if resp.Body.RequestId != nil {
		requestID = strings.TrimSpace(*resp.Body.RequestId)
	}
	return strings.TrimSpace(*resp.Body.Data.Url), requestID, durationMs, nil
}

func (c *aliyunFaceVisionClient) enhanceFaceFromFile(
	srcPath string,
	timeoutSec int,
) (url string, requestID string, durationMs int64, err error) {
	if c == nil || c.faceClient == nil {
		return "", "", 0, errors.New("aliyun face client is nil")
	}
	file, err := os.Open(strings.TrimSpace(srcPath))
	if err != nil {
		return "", "", 0, err
	}
	defer file.Close()

	req := &facebody.EnhanceFaceAdvanceRequest{
		ImageURLObject: file,
	}
	runtime := &dara.RuntimeOptions{}
	runtime.SetReadTimeout(timeoutSec * 1000)
	runtime.SetConnectTimeout(timeoutSec * 1000)

	started := time.Now()
	resp, err := c.faceClient.EnhanceFaceAdvance(req, runtime)
	durationMs = clampDurationMillis(started)
	if err != nil {
		return "", "", durationMs, err
	}
	if resp == nil || resp.Body == nil || resp.Body.Data == nil || resp.Body.Data.ImageURL == nil {
		return "", "", durationMs, errors.New("aliyun EnhanceFace response image url is empty")
	}
	if resp.Body.RequestId != nil {
		requestID = strings.TrimSpace(*resp.Body.RequestId)
	}
	return strings.TrimSpace(*resp.Body.Data.ImageURL), requestID, durationMs, nil
}

func (c *aliyunFaceVisionClient) detectFaceFromFile(
	srcPath string,
	timeoutSec int,
	maxFaceCount int,
	imageWidth int,
	imageHeight int,
) (summary faceDetectionSummary, requestID string, durationMs int64, err error) {
	if c == nil || c.faceClient == nil {
		return summary, "", 0, errors.New("aliyun face client is nil")
	}
	file, err := os.Open(strings.TrimSpace(srcPath))
	if err != nil {
		return summary, "", 0, err
	}
	defer file.Close()

	req := &facebody.DetectFaceAdvanceRequest{
		ImageURLObject: file,
	}
	if maxFaceCount <= 0 {
		maxFaceCount = 3
	}
	req.SetMaxFaceNumber(int64(maxFaceCount))
	req.SetQuality(true)

	runtime := &dara.RuntimeOptions{}
	runtime.SetReadTimeout(timeoutSec * 1000)
	runtime.SetConnectTimeout(timeoutSec * 1000)

	started := time.Now()
	resp, err := c.faceClient.DetectFaceAdvance(req, runtime)
	durationMs = clampDurationMillis(started)
	if err != nil {
		return summary, "", durationMs, err
	}
	if resp != nil && resp.Body != nil && resp.Body.RequestId != nil {
		requestID = strings.TrimSpace(*resp.Body.RequestId)
	}
	if resp == nil || resp.Body == nil || resp.Body.Data == nil {
		return summary, requestID, durationMs, nil
	}
	data := resp.Body.Data
	if data.FaceCount != nil {
		summary.FaceCount = int(*data.FaceCount)
	}
	summary.MaxProbability = maxFloat32Slice(data.FaceProbabilityList)
	summary.MaxAreaRatio = computeFaceAreaRatioFromRectangles(data.FaceRectangles, imageWidth, imageHeight)
	if data.Qualities != nil {
		summary.QualityScoreMax = maxFloat32Slice(data.Qualities.ScoreList)
	}
	return summary, requestID, durationMs, nil
}

func computeFaceAreaRatioFromRectangles(rects []*int32, width int, height int) float64 {
	if width <= 0 || height <= 0 || len(rects) < 4 {
		return 0
	}
	frameArea := float64(width * height)
	if frameArea <= 0 {
		return 0
	}
	maxRatio := 0.0
	for idx := 0; idx+3 < len(rects); idx += 4 {
		a := float64(int32Value(rects[idx]))
		b := float64(int32Value(rects[idx+1]))
		c := float64(int32Value(rects[idx+2]))
		d := float64(int32Value(rects[idx+3]))

		faceW := math.Abs(c)
		faceH := math.Abs(d)
		if faceW <= 0 || faceH <= 0 {
			faceW = math.Abs(c - a)
			faceH = math.Abs(d - b)
		}
		if faceW <= 0 || faceH <= 0 {
			continue
		}
		ratio := (faceW * faceH) / frameArea
		if ratio > maxRatio {
			maxRatio = ratio
		}
	}
	return clampFloat(maxRatio, 0, 1)
}

func int32Value(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

func maxFloat32Slice(values []*float32) float64 {
	maxValue := 0.0
	for _, item := range values {
		if item == nil {
			continue
		}
		v := float64(*item)
		if v > maxValue {
			maxValue = v
		}
	}
	return maxValue
}

func shouldAutoApplyPNGAliyunFaceEnhancement(guidance imageAI2Guidance) bool {
	if hasVisualFocus(guidance.VisualFocus, "portrait") || guidance.EnableMatting {
		return true
	}
	if guidance.Scene == AdvancedScenarioXiaohongshu {
		return true
	}
	if containsFaceIntentKeyword(guidance.Objective) || containsFaceIntentKeyword(guidance.StyleDirection) {
		return true
	}
	for _, item := range guidance.MustCapture {
		if containsFaceIntentKeyword(item) {
			return true
		}
	}
	return false
}

func containsFaceIntentKeyword(input string) bool {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "" {
		return false
	}
	keywords := []string{
		"人脸", "面部", "脸部", "人像", "肖像", "特写",
		"face", "portrait", "headshot", "close-up", "closeup",
	}
	for _, key := range keywords {
		if strings.Contains(value, key) {
			return true
		}
	}
	return false
}

func computeEnhancementDecisionScore(sample frameQualitySample, pairMaxBlur float64) float64 {
	if pairMaxBlur <= 0 {
		pairMaxBlur = 1
	}
	blurNorm := clampZeroOne(sample.BlurScore / pairMaxBlur)
	subject := clampZeroOne(sample.SubjectScore)
	exposure := clampZeroOne(sample.Exposure)
	return roundTo((blurNorm*0.58)+(subject*0.24)+(exposure*0.18), 6)
}

func computeEnhancementResolutionBonus(before, after frameQualitySample) float64 {
	beforePixels := float64(before.Width * before.Height)
	if beforePixels <= 0 {
		beforePixels = 1
	}
	afterPixels := float64(after.Width * after.Height)
	if afterPixels <= 0 {
		afterPixels = 1
	}
	if afterPixels <= beforePixels {
		return 0
	}
	linearGain := math.Sqrt(afterPixels / beforePixels)
	bonus := (linearGain - 1.0) * 0.05
	return roundTo(clampFloat(bonus, 0, 0.08), 6)
}

func decideEnhancedFrameReplacement(originalPath, enhancedPath string, minGain float64) (replace bool, beforeScore float64, afterScore float64, reason string) {
	beforeSample, okBefore := analyzeFrameQuality(strings.TrimSpace(originalPath))
	afterSample, okAfter := analyzeFrameQuality(strings.TrimSpace(enhancedPath))
	if !okBefore || !okAfter {
		return true, 0, 0, "fallback_replace_without_quality_compare"
	}
	pairMaxBlur := maxFloat(beforeSample.BlurScore, afterSample.BlurScore)
	if pairMaxBlur <= 0 {
		pairMaxBlur = 1
	}
	beforeScore = computeEnhancementDecisionScore(beforeSample, pairMaxBlur)
	afterScoreRaw := computeEnhancementDecisionScore(afterSample, pairMaxBlur)
	resolutionBonus := computeEnhancementResolutionBonus(beforeSample, afterSample)
	afterScore = roundTo(afterScoreRaw+resolutionBonus, 6)
	if afterScore >= beforeScore+minGain {
		if resolutionBonus > 0 && afterScoreRaw < beforeScore+minGain {
			return true, beforeScore, afterScore, "enhanced_quality_improved_with_resolution_bonus"
		}
		return true, beforeScore, afterScore, "enhanced_quality_improved"
	}
	return false, beforeScore, afterScore, "kept_original_better_or_equal"
}

func (p *Processor) maybeApplyPNGAliyunFaceEnhancement(
	ctx context.Context,
	job models.VideoJob,
	primaryFormat string,
	framePaths []string,
	guidance imageAI2Guidance,
) ([]string, map[string]interface{}) {
	report := map[string]interface{}{
		"schema_version": "png_worker_face_enhancement_v1",
	}
	if NormalizeRequestedFormat(primaryFormat) != "png" {
		report["status"] = "skipped_not_png"
		return framePaths, report
	}
	if len(framePaths) == 0 {
		report["status"] = "no_frames"
		return framePaths, report
	}

	cfg := loadPNGAliyunFaceEnhanceConfig()
	report["mode"] = cfg.Mode
	report["min_short_side"] = cfg.MinShortSide
	report["max_frames"] = cfg.MaxFrames
	report["timeout_sec"] = cfg.TimeoutSec
	report["cost_per_image_cny"] = roundTo(cfg.CostPerImageCNY, 6)
	report["max_cost_per_job_cny"] = roundTo(cfg.MaxCostPerJobCNY, 6)
	report["endpoint"] = cfg.Endpoint
	report["region_id"] = cfg.RegionID
	report["scene"] = guidance.Scene
	report["visual_focus"] = guidance.VisualFocus
	report["enable_matting"] = guidance.EnableMatting
	report["detect_face_gate"] = cfg.DetectFaceGate
	report["min_face_area_ratio"] = roundTo(cfg.MinFaceAreaRatio, 4)
	report["min_face_confidence"] = roundTo(cfg.MinFaceConfidence, 4)
	report["skip_high_blur_score"] = roundTo(cfg.SkipHighBlurScore, 2)
	report["quality_guard"] = cfg.QualityGuard
	report["replace_min_gain"] = roundTo(cfg.ReplaceMinGain, 4)

	if cfg.Mode == pngAliyunFaceEnhanceModeOff {
		report["status"] = "disabled"
		return framePaths, report
	}
	autoShouldApply := shouldAutoApplyPNGAliyunFaceEnhancement(guidance)
	report["auto_should_apply"] = autoShouldApply
	if cfg.Mode == pngAliyunFaceEnhanceModeAuto && !autoShouldApply {
		report["status"] = "auto_skipped_non_portrait"
		return framePaths, report
	}

	client, err := newAliyunFaceVisionClient(cfg)
	if err != nil {
		report["status"] = "client_init_failed"
		report["error"] = err.Error()
		return framePaths, report
	}

	replacedPaths := make([]string, len(framePaths))
	copy(replacedPaths, framePaths)

	attempted := 0
	succeeded := 0
	replaced := 0
	skipped := 0
	failed := 0
	costCapped := false
	totalCostCNY := 0.0
	items := make([]map[string]interface{}, 0, minInt(len(framePaths), cfg.MaxFrames))

	skippedNoFace := 0
	skippedFaceTooSmall := 0
	skippedFaceConfidence := 0
	skippedAlreadyClear := 0
	for idx, framePath := range framePaths {
		if attempted >= cfg.MaxFrames {
			break
		}
		if cfg.MaxCostPerJobCNY > 0 && (totalCostCNY+cfg.CostPerImageCNY) > (cfg.MaxCostPerJobCNY+1e-9) {
			costCapped = true
			break
		}
		_, width, height := readImageInfo(framePath)
		shortSide := minInt(width, height)
		if width <= 0 || height <= 0 || shortSide < cfg.MinShortSide {
			skipped++
			continue
		}

		attempted++
		item := map[string]interface{}{
			"index":      idx,
			"frame_path": framePath,
			"width":      width,
			"height":     height,
		}
		if cfg.SkipHighBlurScore > 0 {
			if sample, ok := analyzeFrameQuality(framePath); ok {
				item["local_blur_score"] = roundTo(sample.BlurScore, 2)
				if sample.BlurScore >= cfg.SkipHighBlurScore {
					skipped++
					skippedAlreadyClear++
					item["status"] = "skipped_already_clear"
					items = append(items, item)
					continue
				}
			}
		}
		if cfg.DetectFaceGate {
			faceSummary, faceRequestID, faceDurationMs, faceErr := client.detectFaceFromFile(framePath, cfg.TimeoutSec, cfg.DetectFaceMaxFaceCount, width, height)
			item["face_probe_duration_ms"] = faceDurationMs
			item["face_probe_request_id"] = faceRequestID
			item["face_count"] = faceSummary.FaceCount
			item["face_max_area_ratio"] = roundTo(faceSummary.MaxAreaRatio, 4)
			item["face_max_confidence"] = roundTo(faceSummary.MaxProbability, 4)
			item["face_max_quality_score"] = roundTo(faceSummary.QualityScoreMax, 4)
			if faceErr != nil {
				item["face_probe_error"] = faceErr.Error()
			}
			if faceErr == nil {
				if faceSummary.FaceCount <= 0 {
					skipped++
					skippedNoFace++
					item["status"] = "skipped_no_face"
					items = append(items, item)
					continue
				}
				if faceSummary.MaxAreaRatio < cfg.MinFaceAreaRatio {
					skipped++
					skippedFaceTooSmall++
					item["status"] = "skipped_face_too_small"
					items = append(items, item)
					continue
				}
				if faceSummary.MaxProbability > 0 && faceSummary.MaxProbability < cfg.MinFaceConfidence {
					skipped++
					skippedFaceConfidence++
					item["status"] = "skipped_low_face_confidence"
					items = append(items, item)
					continue
				}
			}
		}
		url, requestID, durationMs, callErr := client.enhanceFaceFromFile(framePath, cfg.TimeoutSec)
		item["duration_ms"] = durationMs
		item["request_id"] = requestID
		if callErr != nil {
			failed++
			item["status"] = "api_error"
			item["error"] = callErr.Error()
			items = append(items, item)
			p.recordAliyunFaceEnhancementUsage(job, client.endpoint, "error", durationMs, 0, map[string]interface{}{
				"reason":      "api_error",
				"frame_index": idx,
				"frame_path":  framePath,
				"width":       width,
				"height":      height,
				"request_id":  requestID,
			})
			continue
		}
		item["response_url"] = url

		enhancedPath := framePath + ".face.png"
		if err := p.downloadObject(ctx, url, enhancedPath); err != nil {
			failed++
			item["status"] = "download_error"
			item["error"] = err.Error()
			items = append(items, item)
			p.recordAliyunFaceEnhancementUsage(job, client.endpoint, "error", durationMs, 0, map[string]interface{}{
				"reason":       "download_error",
				"frame_index":  idx,
				"frame_path":   framePath,
				"enhanced_url": url,
				"request_id":   requestID,
			})
			continue
		}

		_, ew, eh := readImageInfo(enhancedPath)
		if ew <= 0 || eh <= 0 {
			failed++
			item["status"] = "enhanced_invalid"
			item["error"] = "enhanced image info invalid"
			items = append(items, item)
			p.recordAliyunFaceEnhancementUsage(job, client.endpoint, "error", durationMs, 0, map[string]interface{}{
				"reason":        "enhanced_invalid",
				"frame_index":   idx,
				"frame_path":    framePath,
				"enhanced_path": enhancedPath,
				"request_id":    requestID,
			})
			continue
		}

		item["enhanced_path"] = enhancedPath
		item["enhanced_width"] = ew
		item["enhanced_height"] = eh
		succeeded++
		totalCostCNY += cfg.CostPerImageCNY
		replace := true
		beforeScore := 0.0
		afterScore := 0.0
		decisionReason := "quality_guard_disabled"
		if cfg.QualityGuard {
			replace, beforeScore, afterScore, decisionReason = decideEnhancedFrameReplacement(framePath, enhancedPath, cfg.ReplaceMinGain)
			item["quality_before_score"] = roundTo(beforeScore, 4)
			item["quality_after_score"] = roundTo(afterScore, 4)
			item["quality_delta"] = roundTo(afterScore-beforeScore, 4)
			item["quality_decision_reason"] = decisionReason
		}
		if replace {
			replacedPaths[idx] = enhancedPath
			replaced++
			item["status"] = "ok_replaced"
		} else {
			item["status"] = "ok_kept_original"
		}
		items = append(items, item)
		p.recordAliyunFaceEnhancementUsage(job, client.endpoint, "ok", durationMs, cfg.CostPerImageCNY, map[string]interface{}{
			"frame_index":     idx,
			"frame_path":      framePath,
			"enhanced_path":   enhancedPath,
			"enhanced_width":  ew,
			"enhanced_height": eh,
			"request_id":      requestID,
			"mode":            cfg.Mode,
			"replaced":        replace,
			"decision_reason": decisionReason,
		})
	}

	report["status"] = "done"
	report["attempted"] = attempted
	report["succeeded"] = succeeded
	report["replaced"] = replaced
	report["failed"] = failed
	report["skipped"] = skipped
	report["skipped_no_face"] = skippedNoFace
	report["skipped_face_too_small"] = skippedFaceTooSmall
	report["skipped_low_face_confidence"] = skippedFaceConfidence
	report["skipped_already_clear"] = skippedAlreadyClear
	report["cost_capped"] = costCapped
	report["total_cost_cny"] = roundTo(totalCostCNY, 6)
	report["remaining_budget_cny"] = roundTo(maxFloat(0, cfg.MaxCostPerJobCNY-totalCostCNY), 6)
	if costCapped {
		report["stop_reason"] = "cost_cap_reached"
	}
	report["items"] = items
	return replacedPaths, report
}

func buildPNGAliyunSuperResCandidateOrder(framePaths []string) []int {
	if len(framePaths) == 0 {
		return nil
	}
	faceFirst := make([]int, 0, len(framePaths))
	others := make([]int, 0, len(framePaths))
	for idx, framePath := range framePaths {
		name := strings.ToLower(strings.TrimSpace(framePath))
		if strings.Contains(name, ".face.") {
			faceFirst = append(faceFirst, idx)
			continue
		}
		others = append(others, idx)
	}
	return append(faceFirst, others...)
}

func (p *Processor) maybeApplyPNGAliyunSuperResolution(
	ctx context.Context,
	job models.VideoJob,
	primaryFormat string,
	framePaths []string,
) ([]string, map[string]interface{}) {
	report := map[string]interface{}{
		"schema_version": "png_worker_super_resolution_v1",
	}
	if NormalizeRequestedFormat(primaryFormat) != "png" {
		report["status"] = "skipped_not_png"
		return framePaths, report
	}

	cfg := loadPNGAliyunSuperResConfig()
	report["mode"] = cfg.Mode
	report["min_short_side"] = cfg.MinShortSide
	report["max_frames"] = cfg.MaxFrames
	report["upscale_factor"] = cfg.UpscaleFactor
	report["output_quality"] = cfg.OutputQuality
	report["cost_per_image_cny"] = roundTo(cfg.CostPerImageCNY, 6)
	report["max_cost_per_job_cny"] = roundTo(cfg.MaxCostPerJobCNY, 6)
	report["endpoint"] = cfg.Endpoint
	report["region_id"] = cfg.RegionID
	report["quality_guard"] = cfg.QualityGuard
	report["replace_min_gain"] = roundTo(cfg.ReplaceMinGain, 4)

	if cfg.Mode == pngAliyunSuperResModeOff {
		report["status"] = "disabled"
		return framePaths, report
	}
	if len(framePaths) == 0 {
		report["status"] = "no_frames"
		return framePaths, report
	}

	client, err := newAliyunVisionClient(cfg)
	if err != nil {
		report["status"] = "client_init_failed"
		report["error"] = err.Error()
		return framePaths, report
	}

	replacedPaths := make([]string, len(framePaths))
	copy(replacedPaths, framePaths)

	attempted := 0
	succeeded := 0
	replaced := 0
	skipped := 0
	failed := 0
	totalCostCNY := 0.0
	costCapped := false
	items := make([]map[string]interface{}, 0, minInt(len(framePaths), cfg.MaxFrames))

	candidateOrder := buildPNGAliyunSuperResCandidateOrder(framePaths)
	report["candidate_order_mode"] = "face_first_then_default"
	for _, idx := range candidateOrder {
		framePath := framePaths[idx]
		if attempted >= cfg.MaxFrames {
			break
		}
		if cfg.MaxCostPerJobCNY > 0 && (totalCostCNY+cfg.CostPerImageCNY) > (cfg.MaxCostPerJobCNY+1e-9) {
			costCapped = true
			break
		}
		_, width, height := readImageInfo(framePath)
		shortSide := minInt(width, height)
		if width <= 0 || height <= 0 || shortSide >= cfg.MinShortSide {
			skipped++
			continue
		}

		attempted++
		item := map[string]interface{}{
			"index":      idx,
			"frame_path": framePath,
			"width":      width,
			"height":     height,
		}
		url, requestID, durationMs, callErr := client.makeSuperResolutionFromFile(framePath, cfg.UpscaleFactor, cfg.OutputQuality, cfg.TimeoutSec)
		item["duration_ms"] = durationMs
		item["request_id"] = requestID
		if callErr != nil {
			failed++
			item["status"] = "api_error"
			item["error"] = callErr.Error()
			items = append(items, item)
			p.recordAliyunSuperResolutionUsage(job, client.endpoint, "error", durationMs, 0, map[string]interface{}{
				"reason":      "api_error",
				"frame_index": idx,
				"frame_path":  framePath,
				"width":       width,
				"height":      height,
				"request_id":  requestID,
			})
			continue
		}
		item["response_url"] = url

		enhancedPath := framePath + ".superres.png"
		if err := p.downloadObject(ctx, url, enhancedPath); err != nil {
			failed++
			item["status"] = "download_error"
			item["error"] = err.Error()
			items = append(items, item)
			p.recordAliyunSuperResolutionUsage(job, client.endpoint, "error", durationMs, 0, map[string]interface{}{
				"reason":       "download_error",
				"frame_index":  idx,
				"frame_path":   framePath,
				"enhanced_url": url,
				"request_id":   requestID,
			})
			continue
		}

		_, ew, eh := readImageInfo(enhancedPath)
		if ew <= 0 || eh <= 0 {
			failed++
			item["status"] = "enhanced_invalid"
			item["error"] = "enhanced image info invalid"
			items = append(items, item)
			p.recordAliyunSuperResolutionUsage(job, client.endpoint, "error", durationMs, 0, map[string]interface{}{
				"reason":        "enhanced_invalid",
				"frame_index":   idx,
				"frame_path":    framePath,
				"enhanced_path": enhancedPath,
				"request_id":    requestID,
			})
			continue
		}

		item["enhanced_path"] = enhancedPath
		item["enhanced_width"] = ew
		item["enhanced_height"] = eh
		succeeded++
		totalCostCNY += cfg.CostPerImageCNY
		replace := cfg.Mode == pngAliyunSuperResModeOn
		beforeScore := 0.0
		afterScore := 0.0
		decisionReason := "shadow_mode_no_replace"
		if cfg.Mode == pngAliyunSuperResModeOn {
			decisionReason = "quality_guard_disabled"
			if cfg.QualityGuard {
				replace, beforeScore, afterScore, decisionReason = decideEnhancedFrameReplacement(framePath, enhancedPath, cfg.ReplaceMinGain)
				item["quality_before_score"] = roundTo(beforeScore, 4)
				item["quality_after_score"] = roundTo(afterScore, 4)
				item["quality_delta"] = roundTo(afterScore-beforeScore, 4)
				item["quality_decision_reason"] = decisionReason
			}
		}
		if replace {
			replacedPaths[idx] = enhancedPath
			replaced++
			item["status"] = "ok_replaced"
		} else {
			item["status"] = "ok_kept_original"
		}
		items = append(items, item)

		p.recordAliyunSuperResolutionUsage(job, client.endpoint, "ok", durationMs, cfg.CostPerImageCNY, map[string]interface{}{
			"frame_index":     idx,
			"frame_path":      framePath,
			"enhanced_path":   enhancedPath,
			"enhanced_width":  ew,
			"enhanced_height": eh,
			"request_id":      requestID,
			"mode":            cfg.Mode,
			"upscale_factor":  cfg.UpscaleFactor,
			"output_quality":  cfg.OutputQuality,
			"replaced":        replace,
			"decision_reason": decisionReason,
		})
	}

	report["status"] = "done"
	report["attempted"] = attempted
	report["succeeded"] = succeeded
	report["replaced"] = replaced
	report["failed"] = failed
	report["skipped"] = skipped
	report["total_cost_cny"] = roundTo(totalCostCNY, 6)
	report["cost_capped"] = costCapped
	report["remaining_budget_cny"] = roundTo(maxFloat(0, cfg.MaxCostPerJobCNY-totalCostCNY), 6)
	if costCapped {
		report["stop_reason"] = "cost_cap_reached"
	}
	report["items"] = items
	return replacedPaths, report
}

func (p *Processor) recordAliyunSuperResolutionUsage(
	job models.VideoJob,
	endpoint string,
	status string,
	durationMs int64,
	costCNY float64,
	metadata map[string]interface{},
) {
	if p == nil || p.db == nil || job.ID == 0 || job.UserID == 0 {
		return
	}
	usdToCNY := loadUSDtoCNYRate()
	costUSD := 0.0
	if costCNY > 0 && usdToCNY > 0 {
		costUSD = roundTo(costCNY/usdToCNY, 8)
	}
	_ = RecordVideoJobAIUsage(p.db, videoJobAIUsageInput{
		JobID:             job.ID,
		UserID:            job.UserID,
		Stage:             "worker_super_resolution",
		Provider:          "aliyun_viapi",
		Model:             "MakeSuperResolutionImage",
		Endpoint:          strings.TrimSpace(endpoint),
		RequestDurationMs: durationMs,
		RequestStatus:     strings.ToLower(strings.TrimSpace(status)),
		Metadata:          metadata,
		CostUSDOverride:   costUSD,
	})
}

func (p *Processor) recordAliyunFaceEnhancementUsage(
	job models.VideoJob,
	endpoint string,
	status string,
	durationMs int64,
	costCNY float64,
	metadata map[string]interface{},
) {
	if p == nil || p.db == nil || job.ID == 0 || job.UserID == 0 {
		return
	}
	usdToCNY := loadUSDtoCNYRate()
	costUSD := 0.0
	if costCNY > 0 && usdToCNY > 0 {
		costUSD = roundTo(costCNY/usdToCNY, 8)
	}
	_ = RecordVideoJobAIUsage(p.db, videoJobAIUsageInput{
		JobID:             job.ID,
		UserID:            job.UserID,
		Stage:             "worker_face_enhancement",
		Provider:          "aliyun_viapi",
		Model:             "EnhanceFace",
		Endpoint:          strings.TrimSpace(endpoint),
		RequestDurationMs: durationMs,
		RequestStatus:     strings.ToLower(strings.TrimSpace(status)),
		Metadata:          metadata,
		CostUSDOverride:   costUSD,
	})
}

func envIntOrDefault(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return value
}

func envFloatOrDefault(key string, def float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return value
}
