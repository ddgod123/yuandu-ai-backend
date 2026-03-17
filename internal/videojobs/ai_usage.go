package videojobs

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"emoji/internal/models"

	"gorm.io/gorm"
)

const (
	defaultAIPricingVersion = "v1_20260317"
)

type videoJobAIUsageInput struct {
	JobID             uint64
	UserID            uint64
	Stage             string
	Provider          string
	Model             string
	Endpoint          string
	InputTokens       int64
	OutputTokens      int64
	CachedInputTokens int64
	ImageTokens       int64
	VideoTokens       int64
	AudioSeconds      float64
	RequestDurationMs int64
	RequestStatus     string
	RequestError      string
	Metadata          map[string]interface{}
}

type aiUnitPricing struct {
	InputPer1M       float64
	OutputPer1M      float64
	CachedInputPer1M float64
	AudioPerMin      float64
	Currency         string
	Version          string
	SourceURL        string
}

type aiPricingOverride struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	InputPer1M       float64 `json:"input_per_1m"`
	OutputPer1M      float64 `json:"output_per_1m"`
	CachedInputPer1M float64 `json:"cached_input_per_1m"`
	AudioPerMin      float64 `json:"audio_per_min"`
	Currency         string  `json:"currency"`
	Version          string  `json:"version"`
	PricingSourceURL string  `json:"pricing_source_url"`
}

var (
	aiPricingCatalogCache     map[string]aiUnitPricing
	aiPricingCatalogCacheRaw  string
	aiPricingCatalogCacheLock sync.RWMutex
)

type videoJobAIAggregate struct {
	Calls             int64
	DurationMs        int64
	InputTokens       int64
	OutputTokens      int64
	CachedInputTokens int64
	ImageTokens       int64
	VideoTokens       int64
	AudioSeconds      float64
	CostUSD           float64
	ErrorCalls        int64
}

func RecordVideoJobAIUsage(db *gorm.DB, in videoJobAIUsageInput) error {
	if db == nil || in.JobID == 0 || in.UserID == 0 {
		return nil
	}
	normalized := normalizeVideoJobAIUsageInput(in)
	pricing := lookupAIUnitPricing(normalized.Provider, normalized.Model)
	costUSD := estimateVideoJobAIUsageCostUSD(normalized, pricing)
	if normalized.Metadata == nil {
		normalized.Metadata = map[string]interface{}{}
	}

	row := models.VideoJobAIUsage{
		JobID:             normalized.JobID,
		UserID:            normalized.UserID,
		Stage:             normalized.Stage,
		Provider:          normalized.Provider,
		Model:             normalized.Model,
		Endpoint:          normalized.Endpoint,
		InputTokens:       normalized.InputTokens,
		OutputTokens:      normalized.OutputTokens,
		CachedInputTokens: normalized.CachedInputTokens,
		ImageTokens:       normalized.ImageTokens,
		VideoTokens:       normalized.VideoTokens,
		AudioSeconds:      normalized.AudioSeconds,
		RequestDurationMs: normalized.RequestDurationMs,
		RequestStatus:     normalized.RequestStatus,
		RequestError:      normalized.RequestError,
		UnitPriceInput:    pricing.InputPer1M,
		UnitPriceOutput:   pricing.OutputPer1M,
		UnitPriceCachedIn: pricing.CachedInputPer1M,
		UnitPriceAudioMin: pricing.AudioPerMin,
		CostUSD:           costUSD,
		Currency:          pricing.Currency,
		PricingVersion:    pricing.Version,
		PricingSourceURL:  pricing.SourceURL,
		Metadata:          mustJSON(normalized.Metadata),
	}
	return db.Create(&row).Error
}

func normalizeVideoJobAIUsageInput(in videoJobAIUsageInput) videoJobAIUsageInput {
	out := in
	out.Stage = strings.ToLower(strings.TrimSpace(out.Stage))
	if out.Stage == "" {
		out.Stage = "unknown"
	}
	out.Provider = strings.ToLower(strings.TrimSpace(out.Provider))
	if out.Provider == "" {
		out.Provider = "unknown"
	}
	out.Model = strings.ToLower(strings.TrimSpace(out.Model))
	out.Endpoint = strings.TrimSpace(out.Endpoint)

	if out.InputTokens < 0 {
		out.InputTokens = 0
	}
	if out.OutputTokens < 0 {
		out.OutputTokens = 0
	}
	if out.CachedInputTokens < 0 {
		out.CachedInputTokens = 0
	}
	if out.CachedInputTokens > out.InputTokens {
		out.CachedInputTokens = out.InputTokens
	}
	if out.ImageTokens < 0 {
		out.ImageTokens = 0
	}
	if out.VideoTokens < 0 {
		out.VideoTokens = 0
	}
	if out.AudioSeconds < 0 {
		out.AudioSeconds = 0
	}
	if out.RequestDurationMs < 0 {
		out.RequestDurationMs = 0
	}
	out.RequestStatus = strings.ToLower(strings.TrimSpace(out.RequestStatus))
	if out.RequestStatus == "" {
		out.RequestStatus = "ok"
	}
	out.RequestError = strings.TrimSpace(out.RequestError)
	return out
}

func estimateVideoJobAIUsageCostUSD(in videoJobAIUsageInput, pricing aiUnitPricing) float64 {
	uncachedInput := in.InputTokens - in.CachedInputTokens
	if uncachedInput < 0 {
		uncachedInput = 0
	}
	inputCost := float64(uncachedInput) * pricing.InputPer1M / 1_000_000
	cachedCost := float64(in.CachedInputTokens) * pricing.CachedInputPer1M / 1_000_000
	outputCost := float64(in.OutputTokens) * pricing.OutputPer1M / 1_000_000
	audioCost := (in.AudioSeconds / 60.0) * pricing.AudioPerMin
	return roundTo(inputCost+cachedCost+outputCost+audioCost, 8)
}

func lookupAIUnitPricing(provider, model string) aiUnitPricing {
	key := pricingKey(provider, model)
	catalog := loadAIPricingCatalog()
	if item, ok := catalog[key]; ok {
		return item
	}
	if item, ok := catalog[pricingKey(provider, "*")]; ok {
		return item
	}
	if item, ok := catalog[pricingKey("*", model)]; ok {
		return item
	}
	if item, ok := catalog[pricingKey("*", "*")]; ok {
		return item
	}
	return aiUnitPricing{
		Currency:  "USD",
		Version:   defaultAIPricingVersion,
		SourceURL: "",
	}
}

func loadAIPricingCatalog() map[string]aiUnitPricing {
	raw := strings.TrimSpace(os.Getenv("VIDEO_JOB_AI_PRICING_OVERRIDES_JSON"))

	aiPricingCatalogCacheLock.RLock()
	if aiPricingCatalogCache != nil && raw == aiPricingCatalogCacheRaw {
		defer aiPricingCatalogCacheLock.RUnlock()
		return aiPricingCatalogCache
	}
	aiPricingCatalogCacheLock.RUnlock()

	catalog := defaultAIPricingCatalog()
	if raw != "" {
		var overrides []aiPricingOverride
		if err := json.Unmarshal([]byte(raw), &overrides); err == nil {
			for _, item := range overrides {
				key := pricingKey(item.Provider, item.Model)
				if key == "" {
					continue
				}
				catalog[key] = aiUnitPricing{
					InputPer1M:       maxFloat64(item.InputPer1M, 0),
					OutputPer1M:      maxFloat64(item.OutputPer1M, 0),
					CachedInputPer1M: maxFloat64(item.CachedInputPer1M, 0),
					AudioPerMin:      maxFloat64(item.AudioPerMin, 0),
					Currency:         fallbackString(item.Currency, "USD"),
					Version:          fallbackString(item.Version, defaultAIPricingVersion),
					SourceURL:        strings.TrimSpace(item.PricingSourceURL),
				}
			}
		}
	}

	aiPricingCatalogCacheLock.Lock()
	aiPricingCatalogCache = catalog
	aiPricingCatalogCacheRaw = raw
	aiPricingCatalogCacheLock.Unlock()
	return catalog
}

func defaultAIPricingCatalog() map[string]aiUnitPricing {
	base := map[string]aiUnitPricing{
		pricingKey("*", "*"): {
			InputPer1M:       0,
			OutputPer1M:      0,
			CachedInputPer1M: 0,
			AudioPerMin:      0,
			Currency:         "USD",
			Version:          defaultAIPricingVersion,
			SourceURL:        "",
		},
		// 阿里 qwen3-vl-flash：按 0~32k 档位默认估算（2026-03-17）
		pricingKey("qwen", "qwen3-vl-flash"): {
			InputPer1M:       0.022,
			OutputPer1M:      0.215,
			CachedInputPer1M: 0,
			AudioPerMin:      0,
			Currency:         "USD",
			Version:          defaultAIPricingVersion,
			SourceURL:        "https://www.alibabacloud.com/help/zh/model-studio/model-pricing",
		},
		pricingKey("qwen", "qwen3-vl-plus"): {
			InputPer1M:       0.143,
			OutputPer1M:      1.434,
			CachedInputPer1M: 0,
			AudioPerMin:      0,
			Currency:         "USD",
			Version:          defaultAIPricingVersion,
			SourceURL:        "https://www.alibabacloud.com/help/zh/model-studio/model-pricing",
		},
		// DeepSeek reasoner/chat（2026-03-17）
		pricingKey("deepseek", "deepseek-reasoner"): {
			InputPer1M:       0.55,
			OutputPer1M:      2.19,
			CachedInputPer1M: 0.14,
			AudioPerMin:      0,
			Currency:         "USD",
			Version:          defaultAIPricingVersion,
			SourceURL:        "https://api-docs.deepseek.com/quick_start/pricing-details-usd",
		},
		pricingKey("deepseek", "deepseek-chat"): {
			InputPer1M:       0.27,
			OutputPer1M:      1.10,
			CachedInputPer1M: 0.07,
			AudioPerMin:      0,
			Currency:         "USD",
			Version:          defaultAIPricingVersion,
			SourceURL:        "https://api-docs.deepseek.com/quick_start/pricing-details-usd",
		},
	}
	// provider 兼容别名
	base[pricingKey("dashscope", "qwen3-vl-flash")] = base[pricingKey("qwen", "qwen3-vl-flash")]
	base[pricingKey("dashscope", "qwen3-vl-plus")] = base[pricingKey("qwen", "qwen3-vl-plus")]
	base[pricingKey("tongyi", "qwen3-vl-flash")] = base[pricingKey("qwen", "qwen3-vl-flash")]
	base[pricingKey("tongyi", "qwen3-vl-plus")] = base[pricingKey("qwen", "qwen3-vl-plus")]
	return base
}

func pricingKey(provider, model string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.ToLower(strings.TrimSpace(model))
	if p == "" {
		p = "*"
	}
	if m == "" {
		m = "*"
	}
	return p + ":" + m
}

func fallbackString(value, fallback string) string {
	if v := strings.TrimSpace(value); v != "" {
		return v
	}
	return fallback
}

func maxFloat64(v, minV float64) float64 {
	if v < minV {
		return minV
	}
	return v
}

func loadVideoJobAIAggregate(db *gorm.DB, jobID uint64) (videoJobAIAggregate, error) {
	agg := videoJobAIAggregate{}
	if db == nil || jobID == 0 {
		return agg, nil
	}
	type row struct {
		Calls             int64   `gorm:"column:calls"`
		DurationMs        int64   `gorm:"column:duration_ms"`
		InputTokens       int64   `gorm:"column:input_tokens"`
		OutputTokens      int64   `gorm:"column:output_tokens"`
		CachedInputTokens int64   `gorm:"column:cached_input_tokens"`
		ImageTokens       int64   `gorm:"column:image_tokens"`
		VideoTokens       int64   `gorm:"column:video_tokens"`
		AudioSeconds      float64 `gorm:"column:audio_seconds"`
		CostUSD           float64 `gorm:"column:cost_usd"`
		ErrorCalls        int64   `gorm:"column:error_calls"`
	}
	var out row
	err := db.Model(&models.VideoJobAIUsage{}).
		Select(`
			COALESCE(COUNT(*),0) AS calls,
			COALESCE(SUM(request_duration_ms),0) AS duration_ms,
			COALESCE(SUM(input_tokens),0) AS input_tokens,
			COALESCE(SUM(output_tokens),0) AS output_tokens,
			COALESCE(SUM(cached_input_tokens),0) AS cached_input_tokens,
			COALESCE(SUM(image_tokens),0) AS image_tokens,
			COALESCE(SUM(video_tokens),0) AS video_tokens,
			COALESCE(SUM(audio_seconds),0) AS audio_seconds,
			COALESCE(SUM(cost_usd),0) AS cost_usd,
			COALESCE(SUM(CASE WHEN request_status <> 'ok' THEN 1 ELSE 0 END),0) AS error_calls
		`).
		Where("job_id = ?", jobID).
		Scan(&out).Error
	if err != nil {
		return agg, err
	}
	agg = videoJobAIAggregate{
		Calls:             out.Calls,
		DurationMs:        out.DurationMs,
		InputTokens:       out.InputTokens,
		OutputTokens:      out.OutputTokens,
		CachedInputTokens: out.CachedInputTokens,
		ImageTokens:       out.ImageTokens,
		VideoTokens:       out.VideoTokens,
		AudioSeconds:      roundTo(out.AudioSeconds, 3),
		CostUSD:           roundTo(out.CostUSD, 8),
		ErrorCalls:        out.ErrorCalls,
	}
	return agg, nil
}

func loadVideoJobAIUsageRows(db *gorm.DB, jobID uint64, limit int) ([]models.VideoJobAIUsage, error) {
	if db == nil || jobID == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var rows []models.VideoJobAIUsage
	err := db.Where("job_id = ?", jobID).
		Order("id DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func clampDurationMillis(start time.Time) int64 {
	if start.IsZero() {
		return 0
	}
	d := time.Since(start).Milliseconds()
	if d < 0 {
		return 0
	}
	return d
}
