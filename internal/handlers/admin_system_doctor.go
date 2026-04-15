package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
)

const (
	adminSystemDoctorHealthGreen  = "green"
	adminSystemDoctorHealthYellow = "yellow"
	adminSystemDoctorHealthRed    = "red"
)

type AdminSystemDoctorCheck struct {
	Key     string      `json:"key"`
	Title   string      `json:"title"`
	Health  string      `json:"health"`
	Summary string      `json:"summary,omitempty"`
	Alerts  []string    `json:"alerts,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

type AdminSystemDoctorResponse struct {
	CheckedAt string                   `json:"checked_at"`
	Health    string                   `json:"health"`
	Alerts    []string                 `json:"alerts"`
	Summary   map[string]int           `json:"summary"`
	Checks    []AdminSystemDoctorCheck `json:"checks"`
}

type adminSystemDoctorTemplateSlotSpec struct {
	Stage    string
	Layer    string
	Required bool
}

type AdminSystemDoctorTemplateSlot struct {
	Format     string `json:"format"`
	Stage      string `json:"stage"`
	Layer      string `json:"layer"`
	Required   bool   `json:"required"`
	Status     string `json:"status"`
	Health     string `json:"health"`
	Source     string `json:"source,omitempty"`
	Enabled    bool   `json:"enabled"`
	TemplateID uint64 `json:"template_id,omitempty"`
	Version    string `json:"version,omitempty"`
}

type AdminSystemDoctorTemplateCoverage struct {
	ActiveTemplateCount int                                        `json:"active_template_count"`
	Stats               map[string]int                             `json:"stats"`
	SlotsByFormat       map[string][]AdminSystemDoctorTemplateSlot `json:"slots_by_format"`
}

type adminSystemDoctorTemplateEvaluation struct {
	Health   string
	Alerts   []string
	Summary  string
	Coverage AdminSystemDoctorTemplateCoverage
}

var adminSystemDoctorTemplateFormats = []string{
	videoAIPromptTemplateFormatGIF,
	videoAIPromptTemplateFormatPNG,
	videoAIPromptTemplateFormatJPG,
	videoAIPromptTemplateFormatWebP,
	videoAIPromptTemplateFormatLive,
}

var adminSystemDoctorTemplateCoreSpecs = []adminSystemDoctorTemplateSlotSpec{
	{Stage: videoAIPromptTemplateStageAI1, Layer: videoAIPromptTemplateLayerFixed, Required: true},
	{Stage: videoAIPromptTemplateStageAI2, Layer: videoAIPromptTemplateLayerFixed, Required: true},
	{Stage: videoAIPromptTemplateStageAI3, Layer: videoAIPromptTemplateLayerFixed, Required: true},
	{Stage: videoAIPromptTemplateStageAI1, Layer: videoAIPromptTemplateLayerEdit, Required: false},
}

var adminSystemDoctorTemplateExtraSpecs = map[string][]adminSystemDoctorTemplateSlotSpec{
	videoAIPromptTemplateFormatPNG: {
		{Stage: videoAIPromptTemplateStageAI2, Layer: "rerank", Required: false},
	},
}

// GetAdminSystemDoctor godoc
// @Summary Run one-shot doctor diagnosis (admin)
// @Tags admin
// @Produce json
// @Success 200 {object} AdminSystemDoctorResponse
// @Router /api/admin/system/doctor [get]
func (h *Handler) GetAdminSystemDoctor(c *gin.Context) {
	c.JSON(http.StatusOK, h.buildAdminSystemDoctorSnapshot(time.Now()))
}

func (h *Handler) buildAdminSystemDoctorSnapshot(now time.Time) AdminSystemDoctorResponse {
	checks := []AdminSystemDoctorCheck{
		h.buildAdminSystemDoctorDependencyCheck(),
		h.buildAdminSystemDoctorConfigCheck(),
		h.buildAdminSystemDoctorQueueCheck(now),
		h.buildAdminSystemDoctorTemplateCheck(),
	}
	out := AdminSystemDoctorResponse{
		CheckedAt: now.Format(time.RFC3339),
		Health:    adminSystemDoctorHealthGreen,
		Alerts:    []string{},
		Summary: map[string]int{
			adminSystemDoctorHealthGreen:  0,
			adminSystemDoctorHealthYellow: 0,
			adminSystemDoctorHealthRed:    0,
		},
		Checks: checks,
	}
	for i := range out.Checks {
		out.Checks[i].Health = normalizeAdminSystemDoctorHealth(out.Checks[i].Health)
		out.Health = elevateAdminWorkerHealth(out.Health, out.Checks[i].Health)
		out.Summary[out.Checks[i].Health]++
		out.Alerts = append(out.Alerts, out.Checks[i].Alerts...)
	}
	out.Alerts = dedupeAdminWorkerAlerts(out.Alerts)
	return out
}

func normalizeAdminSystemDoctorHealth(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case adminSystemDoctorHealthGreen:
		return adminSystemDoctorHealthGreen
	case adminSystemDoctorHealthYellow:
		return adminSystemDoctorHealthYellow
	case adminSystemDoctorHealthRed:
		return adminSystemDoctorHealthRed
	default:
		return adminSystemDoctorHealthGreen
	}
}

func (h *Handler) buildAdminSystemDoctorDependencyCheck() AdminSystemDoctorCheck {
	runtime := videojobs.DetectRuntimeCapabilities()
	health := adminSystemDoctorHealthGreen
	alerts := make([]string, 0, 8)
	requiredFormats := map[string]struct{}{
		videoAIPromptTemplateFormatPNG: {},
		videoAIPromptTemplateFormatGIF: {},
		videoAIPromptTemplateFormatJPG: {},
	}
	requiredUnsupported := make([]string, 0, 3)
	optionalUnsupported := make([]string, 0, 3)
	for _, item := range runtime.Formats {
		if item.Supported {
			continue
		}
		if _, required := requiredFormats[strings.ToLower(strings.TrimSpace(item.Format))]; required {
			requiredUnsupported = append(requiredUnsupported, strings.ToLower(strings.TrimSpace(item.Format)))
			alerts = append(alerts, fmt.Sprintf("格式 %s 不可用：%s", strings.ToUpper(strings.TrimSpace(item.Format)), strings.TrimSpace(item.Reason)))
		} else {
			optionalUnsupported = append(optionalUnsupported, strings.ToLower(strings.TrimSpace(item.Format)))
		}
	}

	if !runtime.FFmpegAvailable {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthRed)
		alerts = append(alerts, "缺少 ffmpeg，视频任务无法执行")
	}
	if !runtime.FFprobeAvailable {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthRed)
		alerts = append(alerts, "缺少 ffprobe，视频探测不可用")
	}
	if len(requiredUnsupported) > 0 {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthRed)
	}
	if !runtime.GifsicleAvailable {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthYellow)
		alerts = append(alerts, "gifsicle 未安装，GIF 产物优化能力降级")
	}
	if len(optionalUnsupported) > 0 {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthYellow)
		alerts = append(alerts, "部分可选格式不可用："+strings.Join(optionalUnsupported, ","))
	}
	if len(alerts) == 0 {
		alerts = []string{}
	}
	return AdminSystemDoctorCheck{
		Key:    "dependencies",
		Title:  "依赖能力",
		Health: health,
		Summary: fmt.Sprintf(
			"ffmpeg=%t ffprobe=%t gifsicle=%t，supported=%s",
			runtime.FFmpegAvailable,
			runtime.FFprobeAvailable,
			runtime.GifsicleAvailable,
			strings.Join(runtime.SupportedFormats, ","),
		),
		Alerts: alerts,
		Details: map[string]interface{}{
			"runtime_capabilities": runtime,
		},
	}
}

func (h *Handler) buildAdminSystemDoctorConfigCheck() AdminSystemDoctorCheck {
	health := adminSystemDoctorHealthGreen
	alerts := make([]string, 0, 12)

	dbReachable := false
	dbError := ""
	if h != nil && h.db != nil {
		var one int
		if err := h.db.Raw("SELECT 1").Scan(&one).Error; err == nil && one == 1 {
			dbReachable = true
		} else if err != nil {
			dbError = err.Error()
		} else {
			dbError = "db ping result invalid"
		}
	} else {
		dbError = "db not initialized"
	}
	if !dbReachable {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthRed)
		alerts = append(alerts, "数据库连接不可用："+strings.TrimSpace(dbError))
	}

	asynqRedisAddrSet := strings.TrimSpace(h.cfg.AsynqRedisAddr) != ""
	if !asynqRedisAddrSet {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthRed)
		alerts = append(alerts, "ASYNQ_REDIS_ADDR 未配置")
	}

	qiniuAKSet := strings.TrimSpace(h.cfg.QiniuAccessKey) != ""
	qiniuSKSet := strings.TrimSpace(h.cfg.QiniuSecretKey) != ""
	qiniuBucketSet := strings.TrimSpace(h.cfg.QiniuBucket) != ""
	qiniuDomainSet := strings.TrimSpace(h.cfg.QiniuDomain) != ""
	if !qiniuAKSet || !qiniuSKSet || !qiniuBucketSet {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthRed)
		alerts = append(alerts, "七牛关键配置缺失（AK/SK/Bucket）")
	}
	if !qiniuDomainSet {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthYellow)
		alerts = append(alerts, "QINIU_DOMAIN 未配置，外链能力可能降级")
	}

	directorBroken := h.cfg.AIDirectorEnabled && !isAIDoctorModelConfigReady(h.cfg.AIDirectorProvider, h.cfg.AIDirectorModel, h.cfg.AIDirectorEndpoint, h.cfg.AIDirectorAPIKey)
	plannerBroken := h.cfg.AIPlannerEnabled && !isAIDoctorModelConfigReady(h.cfg.AIPlannerProvider, h.cfg.AIPlannerModel, h.cfg.AIPlannerEndpoint, h.cfg.AIPlannerAPIKey)
	judgeBroken := h.cfg.AIJudgeEnabled && !isAIDoctorModelConfigReady(h.cfg.AIJudgeProvider, h.cfg.AIJudgeModel, h.cfg.AIJudgeEndpoint, h.cfg.AIJudgeAPIKey)
	if directorBroken {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthYellow)
		alerts = append(alerts, "AI1（Director）已启用但配置不完整")
	}
	if plannerBroken {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthYellow)
		alerts = append(alerts, "AI2（Planner）已启用但配置不完整")
	}
	if judgeBroken {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthYellow)
		alerts = append(alerts, "AI3（Judge）已启用但配置不完整")
	}

	startCommandSet := h.hasAnyAdminWorkerStartCommand()
	stopCommandSet := h.hasAnyAdminWorkerStopCommand()
	if !startCommandSet || !stopCommandSet {
		health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthYellow)
		if !startCommandSet {
			alerts = append(alerts, "未配置 worker 启动命令（仅可软启动恢复队列）")
		}
		if !stopCommandSet {
			alerts = append(alerts, "未配置 worker 停机命令（仅可软停机暂停队列）")
		}
	}

	return AdminSystemDoctorCheck{
		Key:    "configuration",
		Title:  "关键配置",
		Health: health,
		Summary: fmt.Sprintf(
			"db=%t asynq_redis=%t qiniu_core=%t ai_enabled(d/p/j)=%t/%t/%t",
			dbReachable,
			asynqRedisAddrSet,
			qiniuAKSet && qiniuSKSet && qiniuBucketSet,
			h.cfg.AIDirectorEnabled,
			h.cfg.AIPlannerEnabled,
			h.cfg.AIJudgeEnabled,
		),
		Alerts: dedupeAdminWorkerAlerts(alerts),
		Details: map[string]interface{}{
			"db_reachable": dbReachable,
			"db_error":     strings.TrimSpace(dbError),
			"asynq": map[string]interface{}{
				"redis_addr_set":     asynqRedisAddrSet,
				"redis_password_set": strings.TrimSpace(h.cfg.AsynqRedisPassword) != "",
				"redis_db":           h.cfg.AsynqRedisDB,
			},
			"qiniu": map[string]interface{}{
				"access_key_set": qiniuAKSet,
				"secret_key_set": qiniuSKSet,
				"bucket_set":     qiniuBucketSet,
				"domain_set":     qiniuDomainSet,
			},
			"ai_models": map[string]interface{}{
				"director": map[string]interface{}{
					"enabled": h.cfg.AIDirectorEnabled,
					"ready":   !directorBroken,
				},
				"planner": map[string]interface{}{
					"enabled": h.cfg.AIPlannerEnabled,
					"ready":   !plannerBroken,
				},
				"judge": map[string]interface{}{
					"enabled": h.cfg.AIJudgeEnabled,
					"ready":   !judgeBroken,
				},
			},
			"worker_commands": map[string]interface{}{
				"start_command_configured": startCommandSet,
				"stop_command_configured":  stopCommandSet,
			},
		},
	}
}

func isAIDoctorModelConfigReady(provider, model, endpoint, apiKey string) bool {
	return strings.TrimSpace(provider) != "" &&
		strings.TrimSpace(model) != "" &&
		strings.TrimSpace(endpoint) != "" &&
		strings.TrimSpace(apiKey) != ""
}

func (h *Handler) buildAdminSystemDoctorQueueCheck(now time.Time) AdminSystemDoctorCheck {
	workerHealth := h.buildAdminWorkerHealthSnapshot(now)
	health := normalizeAdminSystemDoctorHealth(workerHealth.Health)
	laneOverview := make([]map[string]interface{}, 0, len(workerHealth.Lanes))
	totalPending := 0
	totalRetry := 0
	totalActive := 0
	for _, lane := range workerHealth.Lanes {
		totalPending += lane.Queue.Pending
		totalRetry += lane.Queue.Retry
		totalActive += lane.Queue.Active
		laneOverview = append(laneOverview, map[string]interface{}{
			"role":           lane.Role,
			"queue_name":     lane.QueueName,
			"health":         lane.Health,
			"servers_active": lane.ServersActive,
			"pending":        lane.Queue.Pending,
			"active":         lane.Queue.Active,
			"retry":          lane.Queue.Retry,
			"paused":         lane.Queue.Paused,
			"latency_sec":    lane.Queue.LatencySeconds,
		})
	}
	return AdminSystemDoctorCheck{
		Key:    "queue",
		Title:  "队列与Worker",
		Health: health,
		Summary: fmt.Sprintf(
			"redis=%t worker_active=%d pending=%d retry=%d stale=%d",
			workerHealth.RedisReachable,
			workerHealth.ServersActive,
			totalPending,
			totalRetry,
			workerHealth.StaleQueuedJobs,
		),
		Alerts: dedupeAdminWorkerAlerts(workerHealth.Alerts),
		Details: map[string]interface{}{
			"worker_health": map[string]interface{}{
				"health":                workerHealth.Health,
				"redis_reachable":       workerHealth.RedisReachable,
				"servers_total":         workerHealth.ServersTotal,
				"servers_active":        workerHealth.ServersActive,
				"stale_queued_jobs":     workerHealth.StaleQueuedJobs,
				"oldest_queued_age_sec": workerHealth.OldestQueuedAgeSec,
				"lanes":                 laneOverview,
			},
		},
	}
}

func (h *Handler) buildAdminSystemDoctorTemplateCheck() AdminSystemDoctorCheck {
	if h == nil || h.db == nil {
		return AdminSystemDoctorCheck{
			Key:     "templates",
			Title:   "模板状态",
			Health:  adminSystemDoctorHealthRed,
			Summary: "模板诊断失败：db not initialized",
			Alerts:  []string{"数据库未初始化，无法检查模板状态"},
		}
	}
	var rows []models.VideoAIPromptTemplate
	if err := h.db.Where("is_active = ?", true).
		Order("id DESC").
		Find(&rows).Error; err != nil {
		if isMissingTableError(err, "video_ai_prompt_templates") {
			return AdminSystemDoctorCheck{
				Key:     "templates",
				Title:   "模板状态",
				Health:  adminSystemDoctorHealthRed,
				Summary: "模板表不存在",
				Alerts:  []string{"ops.video_ai_prompt_templates 不存在，模板治理不可用"},
			}
		}
		return AdminSystemDoctorCheck{
			Key:     "templates",
			Title:   "模板状态",
			Health:  adminSystemDoctorHealthRed,
			Summary: "模板查询失败",
			Alerts:  []string{"查询模板失败: " + err.Error()},
		}
	}
	evaluation := evaluateAdminSystemDoctorTemplateCoverage(rows)
	return AdminSystemDoctorCheck{
		Key:     "templates",
		Title:   "模板状态",
		Health:  evaluation.Health,
		Summary: evaluation.Summary,
		Alerts:  dedupeAdminWorkerAlerts(evaluation.Alerts),
		Details: map[string]interface{}{
			"coverage": evaluation.Coverage,
		},
	}
}

func evaluateAdminSystemDoctorTemplateCoverage(rows []models.VideoAIPromptTemplate) adminSystemDoctorTemplateEvaluation {
	index := make(map[string]models.VideoAIPromptTemplate, len(rows))
	for _, row := range rows {
		k := adminSystemDoctorTemplateKey(row.Format, row.Stage, row.Layer)
		if strings.TrimSpace(k) == "" {
			continue
		}
		if _, exists := index[k]; exists {
			continue
		}
		index[k] = row
	}

	health := adminSystemDoctorHealthGreen
	alerts := make([]string, 0, 16)
	slotsByFormat := make(map[string][]AdminSystemDoctorTemplateSlot, len(adminSystemDoctorTemplateFormats))
	stats := map[string]int{
		"missing_required": 0,
		"disabled":         0,
		"fallback_all":     0,
		"missing_optional": 0,
		"ok_exact":         0,
	}

	for _, format := range adminSystemDoctorTemplateFormats {
		specs := make([]adminSystemDoctorTemplateSlotSpec, 0, len(adminSystemDoctorTemplateCoreSpecs)+2)
		specs = append(specs, adminSystemDoctorTemplateCoreSpecs...)
		if extras := adminSystemDoctorTemplateExtraSpecs[format]; len(extras) > 0 {
			specs = append(specs, extras...)
		}
		slots := make([]AdminSystemDoctorTemplateSlot, 0, len(specs))
		for _, spec := range specs {
			slot := AdminSystemDoctorTemplateSlot{
				Format:   format,
				Stage:    spec.Stage,
				Layer:    spec.Layer,
				Required: spec.Required,
				Enabled:  false,
				Health:   adminSystemDoctorHealthGreen,
			}
			exactKey := adminSystemDoctorTemplateKey(format, spec.Stage, spec.Layer)
			allKey := adminSystemDoctorTemplateKey(videoAIPromptTemplateFormatAll, spec.Stage, spec.Layer)
			if row, ok := index[exactKey]; ok {
				slot.Status = "ok_exact"
				slot.Source = format
				slot.TemplateID = row.ID
				slot.Version = strings.TrimSpace(row.Version)
				slot.Enabled = row.Enabled
				if !row.Enabled {
					slot.Status = "disabled"
					slot.Health = adminSystemDoctorHealthYellow
					health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthYellow)
					stats["disabled"]++
					alerts = append(alerts, fmt.Sprintf("模板禁用：%s/%s/%s（id=%d）", format, spec.Stage, spec.Layer, row.ID))
				} else {
					stats["ok_exact"]++
				}
			} else if row, ok := index[allKey]; ok {
				slot.Status = "fallback_all"
				slot.Source = videoAIPromptTemplateFormatAll
				slot.TemplateID = row.ID
				slot.Version = strings.TrimSpace(row.Version)
				slot.Enabled = row.Enabled
				slot.Health = adminSystemDoctorHealthYellow
				health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthYellow)
				stats["fallback_all"]++
				if !row.Enabled {
					slot.Status = "fallback_disabled"
					stats["disabled"]++
					alerts = append(alerts, fmt.Sprintf("模板回退后仍禁用：%s/%s/%s（fallback all, id=%d）", format, spec.Stage, spec.Layer, row.ID))
				} else {
					alerts = append(alerts, fmt.Sprintf("模板使用 all 回退：%s/%s/%s", format, spec.Stage, spec.Layer))
				}
			} else {
				slot.Status = "missing"
				slot.Health = adminSystemDoctorHealthGreen
				if spec.Required {
					slot.Health = adminSystemDoctorHealthRed
					health = elevateAdminWorkerHealth(health, adminSystemDoctorHealthRed)
					stats["missing_required"]++
					alerts = append(alerts, fmt.Sprintf("缺少必需模板：%s/%s/%s", format, spec.Stage, spec.Layer))
				} else {
					slot.Status = "missing_optional"
					stats["missing_optional"]++
				}
			}
			slots = append(slots, slot)
		}
		slotsByFormat[format] = slots
	}

	summary := fmt.Sprintf(
		"active=%d required_missing=%d fallback_all=%d disabled=%d",
		len(rows),
		stats["missing_required"],
		stats["fallback_all"],
		stats["disabled"],
	)
	return adminSystemDoctorTemplateEvaluation{
		Health:  health,
		Alerts:  alerts,
		Summary: summary,
		Coverage: AdminSystemDoctorTemplateCoverage{
			ActiveTemplateCount: len(rows),
			Stats:               stats,
			SlotsByFormat:       slotsByFormat,
		},
	}
}

func adminSystemDoctorTemplateKey(format, stage, layer string) string {
	f := strings.ToLower(strings.TrimSpace(format))
	s := strings.ToLower(strings.TrimSpace(stage))
	l := strings.ToLower(strings.TrimSpace(layer))
	if f == "" || s == "" || l == "" {
		return ""
	}
	return f + "|" + s + "|" + l
}
