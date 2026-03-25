package copyrightjobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/models"
	"emoji/internal/service"

	"github.com/hibiken/asynq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Processor struct {
	db        *gorm.DB
	cfg       config.Config
	provider  service.LLMProvider
	enableLLM bool
}

func NewProcessor(db *gorm.DB, cfg config.Config) *Processor {
	providerName := strings.TrimSpace(cfg.AIPlannerProvider)
	apiKey := strings.TrimSpace(cfg.AIPlannerAPIKey)
	model := strings.TrimSpace(cfg.AIPlannerModel)
	endpoint := strings.TrimSpace(cfg.AIPlannerEndpoint)
	if providerName == "" {
		providerName = strings.TrimSpace(cfg.LLMProvider)
		apiKey = strings.TrimSpace(cfg.LLMAPIKey)
		model = strings.TrimSpace(cfg.LLMModel)
		endpoint = strings.TrimSpace(cfg.LLMEndpoint)
	}
	provider := service.NewLLMProvider(providerName, apiKey, model, endpoint)
	enable := cfg.AIPlannerEnabled && apiKey != ""
	return &Processor{db: db, cfg: cfg, provider: provider, enableLLM: enable}
}

func (p *Processor) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskTypeProcessCollectionCopyright, p.HandleProcessCollectionCopyright)
}

func (p *Processor) HandleProcessCollectionCopyright(ctx context.Context, task *asynq.Task) error {
	var payload ProcessCollectionCopyrightPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if payload.TaskID == 0 {
		return fmt.Errorf("invalid task id")
	}

	var taskRow models.CollectionCopyrightTask
	if err := p.db.First(&taskRow, payload.TaskID).Error; err != nil {
		return err
	}

	now := time.Now()
	_ = p.db.Model(&models.CollectionCopyrightTask{}).
		Where("id = ?", payload.TaskID).
		Updates(map[string]interface{}{
			"status":     "running",
			"started_at": now,
			"updated_at": now,
		}).Error

	var samples []models.CollectionCopyrightTaskImage
	if err := p.db.Where("task_id = ?", payload.TaskID).Order("sample_order ASC, id ASC").Find(&samples).Error; err != nil {
		_ = p.failTask(payload.TaskID, err)
		return err
	}

	if len(samples) == 0 {
		_ = p.completeTask(payload.TaskID, "success", 100, 0, 0, 0, "")
		return nil
	}

	highRiskCount := 0
	unknownSourceCount := 0
	ipHitCount := 0
	processed := 0

	for i := range samples {
		sample := samples[i]
		_ = p.db.Model(&models.CollectionCopyrightTaskImage{}).
			Where("id = ?", sample.ID).
			Updates(map[string]interface{}{"status": "processing", "updated_at": time.Now()}).Error

		var emoji models.Emoji
		if err := p.db.First(&emoji, sample.EmojiID).Error; err != nil {
			_ = p.markSampleFailed(sample.ID, err)
			_ = p.insertLog(payload.TaskID, &sample.EmojiID, "process", "failed", err.Error(), nil)
			continue
		}

		result := p.classifyEmoji(ctx, taskRow, emoji)
		if result.RiskLevel == "L3" {
			highRiskCount++
		}
		if result.IsSourceUnknown {
			unknownSourceCount++
		}
		if result.IsCommercialIP {
			ipHitCount++
		}

		now = time.Now()
		row := models.ImageCopyrightResult{
			TaskID:              payload.TaskID,
			CollectionID:        taskRow.CollectionID,
			EmojiID:             emoji.ID,
			OCRText:             result.OCRText,
			ContentType:         result.ContentType,
			CopyrightOwnerGuess: result.CopyrightOwnerGuess,
			OwnerType:           result.OwnerType,
			IsCommercialIP:      result.IsCommercialIP,
			IPName:              result.IPName,
			IsBrandRelated:      result.IsBrandRelated,
			BrandName:           result.BrandName,
			IsCelebrityRelated:  result.IsCelebrityRelated,
			CelebrityName:       result.CelebrityName,
			IsScreenshot:        result.IsScreenshot,
			IsSourceUnknown:     result.IsSourceUnknown,
			RightsStatus:        result.RightsStatus,
			CommercialUseAdvice: result.CommercialUseAdvice,
			RiskLevel:           result.RiskLevel,
			RiskScore:           result.RiskScore,
			ModelConfidence:     result.ModelConfidence,
			EvidenceJSON:        datatypes.JSON(result.EvidenceJSON),
			MachineSummary:      result.MachineSummary,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := p.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "task_id"}, {Name: "emoji_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"ocr_text":              row.OCRText,
				"content_type":          row.ContentType,
				"copyright_owner_guess": row.CopyrightOwnerGuess,
				"owner_type":            row.OwnerType,
				"is_commercial_ip":      row.IsCommercialIP,
				"ip_name":               row.IPName,
				"is_brand_related":      row.IsBrandRelated,
				"brand_name":            row.BrandName,
				"is_celebrity_related":  row.IsCelebrityRelated,
				"celebrity_name":        row.CelebrityName,
				"is_screenshot":         row.IsScreenshot,
				"is_source_unknown":     row.IsSourceUnknown,
				"rights_status":         row.RightsStatus,
				"commercial_use_advice": row.CommercialUseAdvice,
				"risk_level":            row.RiskLevel,
				"risk_score":            row.RiskScore,
				"model_confidence":      row.ModelConfidence,
				"evidence_json":         row.EvidenceJSON,
				"machine_summary":       row.MachineSummary,
				"updated_at":            now,
			}),
		}).Create(&row).Error; err != nil {
			_ = p.markSampleFailed(sample.ID, err)
			_ = p.insertLog(payload.TaskID, &sample.EmojiID, "persist", "failed", err.Error(), nil)
			continue
		}

		processed++
		progress := int(float64(processed) * 100 / float64(len(samples)))
		_ = p.db.Model(&models.CollectionCopyrightTask{}).
			Where("id = ?", payload.TaskID).
			Updates(map[string]interface{}{
				"progress":             progress,
				"high_risk_count":      highRiskCount,
				"unknown_source_count": unknownSourceCount,
				"ip_hit_count":         ipHitCount,
				"updated_at":           time.Now(),
			}).Error
		_ = p.db.Model(&models.CollectionCopyrightTaskImage{}).
			Where("id = ?", sample.ID).
			Updates(map[string]interface{}{"status": "success", "error_msg": "", "updated_at": time.Now()}).Error
		_ = p.insertLog(payload.TaskID, &sample.EmojiID, "process", "success", "ok", map[string]interface{}{"risk_level": result.RiskLevel})
	}

	status := "success"
	if processed == 0 {
		status = "failed"
	} else if processed < len(samples) {
		status = "partial"
	}

	machineConclusion := p.buildMachineConclusion(status, highRiskCount, ipHitCount, unknownSourceCount)
	riskLevel := "L1"
	if highRiskCount > 0 {
		riskLevel = "L3"
	} else if ipHitCount > 0 || unknownSourceCount > 0 {
		riskLevel = "L2"
	}

	now = time.Now()
	if err := p.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.CollectionCopyrightTask{}).
			Where("id = ?", payload.TaskID).
			Updates(map[string]interface{}{
				"status":               status,
				"progress":             100,
				"high_risk_count":      highRiskCount,
				"unknown_source_count": unknownSourceCount,
				"ip_hit_count":         ipHitCount,
				"machine_conclusion":   machineConclusion,
				"finished_at":          now,
				"updated_at":           now,
			}).Error; err != nil {
			return err
		}

		coverage := 0.0
		if taskRow.SampleCount > 0 {
			coverage = float64(taskRow.ActualSampleCount) * 100 / float64(taskRow.SampleCount)
		}

		if err := tx.Model(&models.CollectionCopyrightResult{}).
			Where("collection_id = ?", taskRow.CollectionID).
			Updates(map[string]interface{}{
				"latest_task_id":       payload.TaskID,
				"run_mode":             taskRow.RunMode,
				"sample_coverage":      coverage,
				"machine_conclusion":   machineConclusion,
				"risk_level":           riskLevel,
				"sampled_image_count":  taskRow.ActualSampleCount,
				"high_risk_count":      highRiskCount,
				"unknown_source_count": unknownSourceCount,
				"ip_hit_count":         ipHitCount,
				"brand_hit_count":      0,
				"recommended_action":   p.recommendedAction(riskLevel, taskRow.RunMode),
				"summary":              machineConclusion,
				"updated_at":           now,
			}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (p *Processor) failTask(taskID uint64, runErr error) error {
	now := time.Now()
	_ = p.db.Model(&models.CollectionCopyrightTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":      "failed",
		"finished_at": now,
		"updated_at":  now,
	}).Error
	_ = p.insertLog(taskID, nil, "task", "failed", runErr.Error(), nil)
	return runErr
}

func (p *Processor) completeTask(taskID uint64, status string, progress, highRisk, unknown, ipHit int, conclusion string) error {
	now := time.Now()
	return p.db.Model(&models.CollectionCopyrightTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":               status,
		"progress":             progress,
		"high_risk_count":      highRisk,
		"unknown_source_count": unknown,
		"ip_hit_count":         ipHit,
		"machine_conclusion":   conclusion,
		"finished_at":          now,
		"updated_at":           now,
	}).Error
}

func (p *Processor) markSampleFailed(sampleID uint64, err error) error {
	return p.db.Model(&models.CollectionCopyrightTaskImage{}).Where("id = ?", sampleID).Updates(map[string]interface{}{
		"status":     "failed",
		"error_msg":  truncate(err.Error(), 1000),
		"updated_at": time.Now(),
	}).Error
}

func (p *Processor) insertLog(taskID uint64, emojiID *uint64, stage, status, message string, detail map[string]interface{}) error {
	detailRaw := datatypes.JSON([]byte("{}"))
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailRaw = datatypes.JSON(b)
		}
	}
	row := models.CopyrightTaskLog{
		TaskID:     taskID,
		EmojiID:    emojiID,
		Stage:      stage,
		Status:     status,
		Message:    truncate(message, 1024),
		DetailJSON: detailRaw,
		CreatedAt:  time.Now(),
	}
	return p.db.Create(&row).Error
}

type classifyResult struct {
	OCRText             string
	ContentType         string
	CopyrightOwnerGuess string
	OwnerType           string
	IsCommercialIP      bool
	IPName              string
	IsBrandRelated      bool
	BrandName           string
	IsCelebrityRelated  bool
	CelebrityName       string
	IsScreenshot        bool
	IsSourceUnknown     bool
	RightsStatus        string
	CommercialUseAdvice string
	RiskLevel           string
	RiskScore           float64
	ModelConfidence     float64
	MachineSummary      string
	EvidenceJSON        []byte
}

func (p *Processor) classifyEmoji(ctx context.Context, task models.CollectionCopyrightTask, emoji models.Emoji) classifyResult {
	base := classifyResult{
		OCRText:             strings.TrimSpace(emoji.Title),
		ContentType:         "emoji",
		CopyrightOwnerGuess: "公众网络来源不明",
		OwnerType:           "unknown",
		IsCommercialIP:      false,
		IPName:              "",
		IsBrandRelated:      false,
		BrandName:           "",
		IsCelebrityRelated:  false,
		CelebrityName:       "",
		IsScreenshot:        false,
		IsSourceUnknown:     true,
		RightsStatus:        "unknown_source",
		CommercialUseAdvice: "not_recommended",
		RiskLevel:           "L2",
		RiskScore:           60,
		ModelConfidence:     0.6,
		MachineSummary:      "来源不明，建议人工复核",
	}

	titleLower := strings.ToLower(strings.TrimSpace(emoji.Title))
	if strings.Contains(titleLower, "原创") || strings.Contains(titleLower, "自制") {
		base.CopyrightOwnerGuess = "自有原创"
		base.OwnerType = "self"
		base.IsSourceUnknown = false
		base.RightsStatus = "clear"
		base.CommercialUseAdvice = "allowed"
		base.RiskLevel = "L0"
		base.RiskScore = 10
		base.ModelConfidence = 0.75
		base.MachineSummary = "命中原创关键词，低风险"
	}
	if strings.Contains(titleLower, "迪士尼") || strings.Contains(titleLower, "漫威") || strings.Contains(titleLower, "海绵宝宝") {
		base.IsCommercialIP = true
		base.IPName = emoji.Title
		base.RightsStatus = "high_risk"
		base.CommercialUseAdvice = "forbidden"
		base.RiskLevel = "L3"
		base.RiskScore = 95
		base.ModelConfidence = 0.9
		base.MachineSummary = "疑似命中商业IP，建议禁止商用"
	}
	if strings.Contains(titleLower, "logo") || strings.Contains(titleLower, "品牌") {
		base.IsBrandRelated = true
		if base.RiskScore < 75 {
			base.RiskScore = 75
		}
		if base.RiskLevel == "L0" || base.RiskLevel == "L1" {
			base.RiskLevel = "L2"
		}
		base.CommercialUseAdvice = "need_authorization"
	}

	if p.enableLLM {
		if enriched, ok := p.visionRefine(ctx, emoji, base); ok {
			base = enriched
		} else if enriched, ok := p.llmRefine(ctx, emoji, base); ok {
			base = enriched
		}
	}

	evidence := map[string]interface{}{
		"emoji_id":        emoji.ID,
		"emoji_title":     emoji.Title,
		"file_url":        emoji.FileURL,
		"thumb_url":       emoji.ThumbURL,
		"rule_version":    "copyright_v1",
		"run_mode":        task.RunMode,
		"sample_strategy": task.SampleStrategy,
	}
	b, _ := json.Marshal(evidence)
	base.EvidenceJSON = b
	return base
}

func (p *Processor) llmRefine(ctx context.Context, emoji models.Emoji, base classifyResult) (classifyResult, bool) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	systemPrompt := "你是版权风控助手。根据素材标题和路径信息，输出JSON，不要markdown。"
	userPrompt := fmt.Sprintf("请判断风险并返回JSON: {\"is_commercial_ip\":bool,\"is_brand_related\":bool,\"is_source_unknown\":bool,\"rights_status\":\"clear|third_party_suspected|unknown_source|high_risk|need_authorization|unknown\",\"commercial_use_advice\":\"allowed|need_authorization|not_recommended|forbidden\",\"risk_level\":\"L0|L1|L2|L3\",\"risk_score\":number,\"confidence\":number,\"summary\":\"text\"}. 输入: title=%q, file_url=%q, thumb_url=%q", emoji.Title, emoji.FileURL, emoji.ThumbURL)
	resp, err := p.provider.Chat(timeoutCtx, systemPrompt, userPrompt, 300)
	if err != nil {
		return base, false
	}
	resp = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(resp, "```json"), "```"))
	resp = strings.TrimSpace(strings.TrimSuffix(resp, "```"))
	var parsed struct {
		IsCommercialIP      bool    `json:"is_commercial_ip"`
		IsBrandRelated      bool    `json:"is_brand_related"`
		IsSourceUnknown     bool    `json:"is_source_unknown"`
		RightsStatus        string  `json:"rights_status"`
		CommercialUseAdvice string  `json:"commercial_use_advice"`
		RiskLevel           string  `json:"risk_level"`
		RiskScore           float64 `json:"risk_score"`
		Confidence          float64 `json:"confidence"`
		Summary             string  `json:"summary"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return base, false
	}
	if parsed.RiskLevel != "L0" && parsed.RiskLevel != "L1" && parsed.RiskLevel != "L2" && parsed.RiskLevel != "L3" {
		return base, false
	}
	base.IsCommercialIP = parsed.IsCommercialIP
	base.IsBrandRelated = parsed.IsBrandRelated
	base.IsSourceUnknown = parsed.IsSourceUnknown
	if parsed.RightsStatus != "" {
		base.RightsStatus = parsed.RightsStatus
	}
	if parsed.CommercialUseAdvice != "" {
		base.CommercialUseAdvice = parsed.CommercialUseAdvice
	}
	base.RiskLevel = parsed.RiskLevel
	if parsed.RiskScore >= 0 {
		base.RiskScore = parsed.RiskScore
	}
	if parsed.Confidence > 0 {
		base.ModelConfidence = parsed.Confidence
	}
	if strings.TrimSpace(parsed.Summary) != "" {
		base.MachineSummary = strings.TrimSpace(parsed.Summary)
	}
	return base, true
}

func (p *Processor) visionRefine(ctx context.Context, emoji models.Emoji, base classifyResult) (classifyResult, bool) {
	imageURL := strings.TrimSpace(emoji.FileURL)
	if imageURL == "" {
		imageURL = strings.TrimSpace(emoji.ThumbURL)
	}
	if imageURL == "" || !(strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://")) {
		return base, false
	}
	endpoint := strings.TrimRight(strings.TrimSpace(p.cfg.AIPlannerEndpoint), "/")
	apiKey := strings.TrimSpace(p.cfg.AIPlannerAPIKey)
	model := strings.TrimSpace(p.cfg.AIPlannerModel)
	if endpoint == "" || apiKey == "" || model == "" {
		return base, false
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{
				"role": "system",
				"content": []map[string]interface{}{
					{"type": "text", "text": "你是版权风控助手。请输出严格JSON。"},
				},
			},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "text", "text": "根据图片和标题判断版权风险，返回JSON字段：is_commercial_ip,is_brand_related,is_source_unknown,rights_status,commercial_use_advice,risk_level,risk_score,confidence,summary"},
					{"type": "text", "text": fmt.Sprintf("title=%s", emoji.Title)},
					{"type": "image_url", "image_url": map[string]interface{}{"url": imageURL}},
				},
			},
		},
		"max_tokens": 400,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return base, false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/chat/completions", strings.NewReader(string(b)))
	if err != nil {
		return base, false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return base, false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return base, false
	}
	var cr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &cr); err != nil || len(cr.Choices) == 0 {
		return base, false
	}
	raw := strings.TrimSpace(cr.Choices[0].Message.Content)
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(raw, "```json"), "```"))
	raw = strings.TrimSpace(strings.TrimSuffix(raw, "```"))
	var parsed struct {
		IsCommercialIP      bool    `json:"is_commercial_ip"`
		IsBrandRelated      bool    `json:"is_brand_related"`
		IsSourceUnknown     bool    `json:"is_source_unknown"`
		RightsStatus        string  `json:"rights_status"`
		CommercialUseAdvice string  `json:"commercial_use_advice"`
		RiskLevel           string  `json:"risk_level"`
		RiskScore           float64 `json:"risk_score"`
		Confidence          float64 `json:"confidence"`
		Summary             string  `json:"summary"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return base, false
	}
	if parsed.RiskLevel != "L0" && parsed.RiskLevel != "L1" && parsed.RiskLevel != "L2" && parsed.RiskLevel != "L3" {
		return base, false
	}
	base.IsCommercialIP = parsed.IsCommercialIP
	base.IsBrandRelated = parsed.IsBrandRelated
	base.IsSourceUnknown = parsed.IsSourceUnknown
	if parsed.RightsStatus != "" {
		base.RightsStatus = parsed.RightsStatus
	}
	if parsed.CommercialUseAdvice != "" {
		base.CommercialUseAdvice = parsed.CommercialUseAdvice
	}
	base.RiskLevel = parsed.RiskLevel
	base.RiskScore = parsed.RiskScore
	if parsed.Confidence > 0 {
		base.ModelConfidence = parsed.Confidence
	}
	if strings.TrimSpace(parsed.Summary) != "" {
		base.MachineSummary = strings.TrimSpace(parsed.Summary)
	}
	return base, true
}

func (p *Processor) buildMachineConclusion(status string, highRiskCount, ipHitCount, unknownSourceCount int) string {
	if status == "failed" {
		return "识别失败"
	}
	if highRiskCount > 0 {
		return "命中高风险内容，建议法务复核"
	}
	if ipHitCount > 0 {
		return "命中疑似商业IP，建议补充授权"
	}
	if unknownSourceCount > 0 {
		return "存在来源不明内容，建议复核"
	}
	return "未发现明显高风险"
}

func (p *Processor) recommendedAction(riskLevel, runMode string) string {
	if riskLevel == "L3" {
		return "legal_review_required"
	}
	if riskLevel == "L2" {
		if runMode != "all" {
			return "suggest_full_run"
		}
		return "need_authorization"
	}
	return "preliminarily_pass"
}

func truncate(v string, n int) string {
	v = strings.TrimSpace(v)
	if len(v) <= n {
		return v
	}
	return v[:n]
}
