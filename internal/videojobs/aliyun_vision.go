package videojobs

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	openapiutil "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	imageenhan "github.com/alibabacloud-go/imageenhan-20190930/v2/client"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/alibabacloud-go/tea/tea"

	"emoji/internal/models"
)

const (
	pngAliyunSuperResModeOff    = "off"
	pngAliyunSuperResModeShadow = "shadow"
	pngAliyunSuperResModeOn     = "on"
)

type pngAliyunSuperResConfig struct {
	Mode            string
	RegionID        string
	Endpoint        string
	MinShortSide    int
	MaxFrames       int
	UpscaleFactor   int
	OutputQuality   int
	CostPerImageCNY float64
	TimeoutSec      int
}

type aliyunVisionClient struct {
	imageClient *imageenhan.Client
	endpoint    string
}

func loadPNGAliyunSuperResConfig() pngAliyunSuperResConfig {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("PNG_ALIYUN_SUPERRES_MODE")))
	switch mode {
	case pngAliyunSuperResModeOff, pngAliyunSuperResModeShadow, pngAliyunSuperResModeOn:
	default:
		// 主线默认开启超分；如需关闭可显式配置 off
		mode = pngAliyunSuperResModeOn
	}

	cfg := pngAliyunSuperResConfig{
		Mode:            mode,
		RegionID:        strings.TrimSpace(os.Getenv("ALIYUN_VISION_REGION_ID")),
		Endpoint:        strings.TrimSpace(os.Getenv("ALIYUN_VISION_IMAGEENHAN_ENDPOINT")),
		MinShortSide:    envIntOrDefault("PNG_ALIYUN_SUPERRES_MIN_SHORT_SIDE", 960),
		MaxFrames:       envIntOrDefault("PNG_ALIYUN_SUPERRES_MAX_FRAMES", 4),
		UpscaleFactor:   envIntOrDefault("PNG_ALIYUN_SUPERRES_UPSCALE_FACTOR", 2),
		OutputQuality:   envIntOrDefault("PNG_ALIYUN_SUPERRES_OUTPUT_QUALITY", 95),
		CostPerImageCNY: envFloatOrDefault("PNG_ALIYUN_SUPERRES_COST_PER_IMAGE_CNY", 0.02),
		TimeoutSec:      envIntOrDefault("PNG_ALIYUN_SUPERRES_TIMEOUT_SECONDS", 25),
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
	if cfg.TimeoutSec < 5 {
		cfg.TimeoutSec = 25
	}
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
	report["endpoint"] = cfg.Endpoint
	report["region_id"] = cfg.RegionID

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
	items := make([]map[string]interface{}, 0, minInt(len(framePaths), cfg.MaxFrames))

	for idx, framePath := range framePaths {
		if attempted >= cfg.MaxFrames {
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
		item["status"] = "ok"
		succeeded++
		totalCostCNY += cfg.CostPerImageCNY
		if cfg.Mode == pngAliyunSuperResModeOn {
			replacedPaths[idx] = enhancedPath
			replaced++
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
		})
	}

	report["status"] = "done"
	report["attempted"] = attempted
	report["succeeded"] = succeeded
	report["replaced"] = replaced
	report["failed"] = failed
	report["skipped"] = skipped
	report["total_cost_cny"] = roundTo(totalCostCNY, 6)
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
