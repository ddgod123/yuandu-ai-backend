package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/queue"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
)

const (
	adminWorkerRoleAll   = "all"
	adminWorkerRoleGIF   = "gif"
	adminWorkerRolePNG   = "png"
	adminWorkerRoleMedia = "media"

	adminWorkerStaleQueueDuration = 2 * time.Minute
)

type adminWorkerLaneDefinition struct {
	Role      string
	Label     string
	QueueName string
}

var adminWorkerLaneDefinitions = []adminWorkerLaneDefinition{
	{Role: adminWorkerRoleGIF, Label: "GIF", QueueName: videojobs.QueueVideoJobGIF},
	{Role: adminWorkerRolePNG, Label: "PNG", QueueName: videojobs.QueueVideoJobPNG},
	{Role: adminWorkerRoleMedia, Label: "通用", QueueName: videojobs.QueueVideoJobMedia},
}

type AdminWorkerServerStatus struct {
	ID            string         `json:"id"`
	Host          string         `json:"host"`
	PID           int            `json:"pid"`
	Status        string         `json:"status"`
	StartedAt     string         `json:"started_at"`
	Concurrency   int            `json:"concurrency"`
	Queues        map[string]int `json:"queues"`
	Roles         []string       `json:"roles,omitempty"`
	ActiveWorkers int            `json:"active_workers"`
}

type AdminWorkerQueueStatus struct {
	Name           string  `json:"name"`
	Size           int     `json:"size"`
	Pending        int     `json:"pending"`
	Active         int     `json:"active"`
	Scheduled      int     `json:"scheduled"`
	Retry          int     `json:"retry"`
	Archived       int     `json:"archived"`
	Completed      int     `json:"completed"`
	Paused         bool    `json:"paused"`
	LatencySeconds float64 `json:"latency_seconds"`
	ProcessedToday int     `json:"processed_today"`
	FailedToday    int     `json:"failed_today"`
}

type AdminWorkerLaneStatus struct {
	Role          string                 `json:"role"`
	Label         string                 `json:"label"`
	QueueName     string                 `json:"queue_name"`
	Health        string                 `json:"health"`
	ServersTotal  int                    `json:"servers_total"`
	ServersActive int                    `json:"servers_active"`
	Queue         AdminWorkerQueueStatus `json:"queue"`
	Alerts        []string               `json:"alerts"`
	StartEnabled  bool                   `json:"start_enabled"`
	StartHint     string                 `json:"start_hint,omitempty"`
	StopEnabled   bool                   `json:"stop_enabled"`
	StopHint      string                 `json:"stop_hint,omitempty"`
}

type AdminWorkerHealthResponse struct {
	CheckedAt          string                    `json:"checked_at"`
	Health             string                    `json:"health"`
	RedisReachable     bool                      `json:"redis_reachable"`
	RedisAddr          string                    `json:"redis_addr"`
	RedisDB            int                       `json:"redis_db"`
	QueueName          string                    `json:"queue_name"` // legacy
	ServersTotal       int                       `json:"servers_total"`
	ServersActive      int                       `json:"servers_active"`
	Servers            []AdminWorkerServerStatus `json:"servers"`
	Queue              AdminWorkerQueueStatus    `json:"queue"` // legacy
	Lanes              []AdminWorkerLaneStatus   `json:"lanes"`
	StaleQueuedJobs    int64                     `json:"stale_queued_jobs"`
	OldestQueuedAgeSec float64                   `json:"oldest_queued_age_sec"`
	Alerts             []string                  `json:"alerts"`
	StartEnabled       bool                      `json:"start_enabled"`
	StartHint          string                    `json:"start_hint,omitempty"`
	StopEnabled        bool                      `json:"stop_enabled"`
	StopHint           string                    `json:"stop_hint,omitempty"`
}

// GetAdminWorkerHealth godoc
// @Summary Get worker health overview (admin)
// @Tags admin
// @Produce json
// @Success 200 {object} AdminWorkerHealthResponse
// @Router /api/admin/system/worker-health [get]
func (h *Handler) GetAdminWorkerHealth(c *gin.Context) {
	now := time.Now()
	out := AdminWorkerHealthResponse{
		CheckedAt:    now.Format(time.RFC3339),
		Health:       "green",
		RedisAddr:    h.cfg.AsynqRedisAddr,
		RedisDB:      h.cfg.AsynqRedisDB,
		QueueName:    videojobs.QueueVideoJobMedia,
		Servers:      make([]AdminWorkerServerStatus, 0, 4),
		Alerts:       make([]string, 0, 8),
		Lanes:        h.initAdminWorkerLanes(),
		Queue:        AdminWorkerQueueStatus{Name: videojobs.QueueVideoJobMedia},
		StartEnabled: true,
		StopEnabled:  true,
	}
	if !h.hasAnyAdminWorkerStartCommand() {
		out.StartHint = "未配置启动命令：将只执行恢复队列消费（软启动）"
	}
	if !h.hasAnyAdminWorkerStopCommand() {
		out.StopHint = "未配置停机命令：将只执行暂停队列消费（软停机）"
	}

	inspector := queue.NewInspector(h.cfg)
	defer inspector.Close()

	servers, err := inspector.Servers()
	if err != nil {
		out.Health = "red"
		out.Alerts = append(out.Alerts, "无法连接 Asynq/Redis，请检查 worker 与 Redis 配置")
		c.JSON(http.StatusOK, out)
		return
	}
	out.RedisReachable = true

	sort.SliceStable(servers, func(i, j int) bool {
		return servers[i].Started.After(servers[j].Started)
	})
	for _, item := range servers {
		status := strings.ToLower(strings.TrimSpace(item.Status))
		if status == "active" {
			out.ServersActive++
		}
		out.Servers = append(out.Servers, AdminWorkerServerStatus{
			ID:            item.ID,
			Host:          item.Host,
			PID:           item.PID,
			Status:        item.Status,
			StartedAt:     item.Started.Format(time.RFC3339),
			Concurrency:   item.Concurrency,
			Queues:        item.Queues,
			Roles:         inferAdminWorkerRolesFromQueues(item.Queues),
			ActiveWorkers: len(item.ActiveWorkers),
		})
	}
	out.ServersTotal = len(out.Servers)

	for idx := range out.Lanes {
		lane := out.Lanes[idx]

		queueInfo, qErr := inspector.GetQueueInfo(lane.QueueName)
		if qErr != nil {
			if !isAsynqQueueNotFoundErr(qErr) {
				lane.Alerts = append(lane.Alerts, fmt.Sprintf("读取队列失败: %v", qErr))
			}
		} else if queueInfo != nil {
			lane.Queue = AdminWorkerQueueStatus{
				Name:           queueInfo.Queue,
				Size:           queueInfo.Size,
				Pending:        queueInfo.Pending,
				Active:         queueInfo.Active,
				Scheduled:      queueInfo.Scheduled,
				Retry:          queueInfo.Retry,
				Archived:       queueInfo.Archived,
				Completed:      queueInfo.Completed,
				Paused:         queueInfo.Paused,
				LatencySeconds: queueInfo.Latency.Seconds(),
				ProcessedToday: queueInfo.Processed,
				FailedToday:    queueInfo.Failed,
			}
		}

		lane.ServersTotal = countAdminServersByQueue(servers, lane.QueueName, false)
		lane.ServersActive = countAdminServersByQueue(servers, lane.QueueName, true)
		lane.Health, lane.Alerts = finalizeAdminWorkerLaneHealth(lane)
		out.Lanes[idx] = lane
	}

	for _, lane := range out.Lanes {
		if lane.Role == adminWorkerRoleMedia {
			out.QueueName = lane.QueueName
			out.Queue = lane.Queue
			break
		}
	}

	staleCount, oldestAgeSec, staleErr := h.loadAdminStaleQueuedVideoJobs(now)
	if staleErr != nil {
		out.Health = elevateAdminWorkerHealth(out.Health, "yellow")
		out.Alerts = append(out.Alerts, "读取视频任务排队情况失败")
	} else {
		out.StaleQueuedJobs = staleCount
		out.OldestQueuedAgeSec = oldestAgeSec
	}

	out.Health, out.Alerts = finalizeAdminWorkerHealth(out)
	c.JSON(http.StatusOK, out)
}

// StartAdminWorker godoc
// @Summary Start worker process via configured command (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param role query string false "all|gif|png|media"
// @Param force query boolean false "force execute command even when worker is already online"
// @Success 200 {object} map[string]interface{}
// @Router /api/admin/system/worker-start [post]
func (h *Handler) StartAdminWorker(c *gin.Context) {
	role, force, err := parseAdminWorkerActionParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resumedQueues, resumeWarnings, resumeErr := h.toggleAdminWorkerQueues(role, false)
	if resumeErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":          "failed to resume worker queues",
			"role":           role,
			"resumed_queues": resumedQueues,
			"warnings":       resumeWarnings,
			"detail":         resumeErr.Error(),
		})
		return
	}

	startCommand := h.resolveAdminWorkerStartCommand(role)
	activeBefore, _ := h.countAdminActiveWorkersByRole(role)

	if startCommand == "" {
		message := "未配置启动命令，已执行恢复队列消费（软启动）"
		if activeBefore > 0 {
			message = "检测到 worker 已在线，已恢复队列消费"
		}
		c.JSON(http.StatusOK, gin.H{
			"ok":              true,
			"mode":            "resume_only",
			"role":            role,
			"already_running": activeBefore > 0,
			"active_servers":  activeBefore,
			"resumed_queues":  resumedQueues,
			"warnings":        resumeWarnings,
			"message":         message,
		})
		return
	}

	if activeBefore > 0 && !force {
		c.JSON(http.StatusOK, gin.H{
			"ok":              true,
			"role":            role,
			"already_running": true,
			"active_servers":  activeBefore,
			"resumed_queues":  resumedQueues,
			"warnings":        resumeWarnings,
			"message":         "检测到 worker 已在线，若需强制执行请传 force=1",
		})
		return
	}

	output, timedOut, execErr := h.executeAdminWorkerCommand(c.Request.Context(), startCommand)
	if timedOut {
		c.JSON(http.StatusGatewayTimeout, gin.H{
			"error":           "start command timeout",
			"role":            role,
			"timeout_seconds": h.resolveAdminWorkerActionTimeoutSeconds(),
			"output":          output,
			"resumed_queues":  resumedQueues,
			"warnings":        resumeWarnings,
		})
		return
	}
	if execErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":          "start command failed",
			"role":           role,
			"detail":         execErr.Error(),
			"output":         output,
			"resumed_queues": resumedQueues,
			"warnings":       resumeWarnings,
		})
		return
	}

	started := h.waitAdminWorkerOnline(role, 8*time.Second)
	activeAfter, _ := h.countAdminActiveWorkersByRole(role)
	message := "启动命令已执行，等待 worker 心跳"
	if started {
		message = "worker 已在线"
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":             true,
		"role":           role,
		"started":        started,
		"active_before":  activeBefore,
		"active_after":   activeAfter,
		"resumed_queues": resumedQueues,
		"warnings":       resumeWarnings,
		"message":        message,
		"output":         output,
	})
}

// StopAdminWorker godoc
// @Summary Stop or pause worker by role (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param role query string false "all|gif|png|media"
// @Param force query boolean false "force execute stop command even when worker is already offline"
// @Success 200 {object} map[string]interface{}
// @Router /api/admin/system/worker-stop [post]
func (h *Handler) StopAdminWorker(c *gin.Context) {
	role, force, err := parseAdminWorkerActionParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pausedQueues, pauseWarnings, pauseErr := h.toggleAdminWorkerQueues(role, true)
	if pauseErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":         "failed to pause worker queues",
			"role":          role,
			"paused_queues": pausedQueues,
			"warnings":      pauseWarnings,
			"detail":        pauseErr.Error(),
		})
		return
	}

	stopCommand := h.resolveAdminWorkerStopCommand(role)
	activeBefore, _ := h.countAdminActiveWorkersByRole(role)

	if stopCommand == "" {
		message := "未配置停机命令，已执行暂停队列消费（软停机）"
		if activeBefore <= 0 {
			message = "未发现在线 worker，队列已暂停"
		}
		c.JSON(http.StatusOK, gin.H{
			"ok":              true,
			"mode":            "pause_only",
			"role":            role,
			"active_before":   activeBefore,
			"paused_queues":   pausedQueues,
			"warnings":        pauseWarnings,
			"already_stopped": activeBefore <= 0,
			"message":         message,
		})
		return
	}

	if activeBefore <= 0 && !force {
		c.JSON(http.StatusOK, gin.H{
			"ok":              true,
			"role":            role,
			"already_stopped": true,
			"active_before":   activeBefore,
			"paused_queues":   pausedQueues,
			"warnings":        pauseWarnings,
			"message":         "未发现在线 worker，已保持队列暂停；若需强制执行停机命令请传 force=1",
		})
		return
	}

	output, timedOut, execErr := h.executeAdminWorkerCommand(c.Request.Context(), stopCommand)
	if timedOut {
		c.JSON(http.StatusGatewayTimeout, gin.H{
			"error":           "stop command timeout",
			"role":            role,
			"timeout_seconds": h.resolveAdminWorkerActionTimeoutSeconds(),
			"output":          output,
			"paused_queues":   pausedQueues,
			"warnings":        pauseWarnings,
		})
		return
	}
	if execErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":         "stop command failed",
			"role":          role,
			"detail":        execErr.Error(),
			"output":        output,
			"paused_queues": pausedQueues,
			"warnings":      pauseWarnings,
		})
		return
	}

	stopped := h.waitAdminWorkerOffline(role, 8*time.Second)
	activeAfter, _ := h.countAdminActiveWorkersByRole(role)
	message := "停机命令已执行，等待 worker 下线"
	if stopped {
		message = "worker 已下线"
	} else if activeAfter > 0 {
		message = "停机命令已执行，但仍检测到在线 worker（请检查进程管理器）"
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":            true,
		"role":          role,
		"stopped":       stopped,
		"active_before": activeBefore,
		"active_after":  activeAfter,
		"paused_queues": pausedQueues,
		"warnings":      pauseWarnings,
		"message":       message,
		"output":        output,
	})
}

func (h *Handler) initAdminWorkerLanes() []AdminWorkerLaneStatus {
	lanes := make([]AdminWorkerLaneStatus, 0, len(adminWorkerLaneDefinitions))
	for _, def := range adminWorkerLaneDefinitions {
		startEnabled, startHint := h.resolveAdminWorkerStartCapability(def.Role)
		stopEnabled, stopHint := h.resolveAdminWorkerStopCapability(def.Role)
		lanes = append(lanes, AdminWorkerLaneStatus{
			Role:      def.Role,
			Label:     def.Label,
			QueueName: def.QueueName,
			Health:    "unknown",
			Queue: AdminWorkerQueueStatus{
				Name: def.QueueName,
			},
			Alerts:       []string{},
			StartEnabled: startEnabled,
			StartHint:    startHint,
			StopEnabled:  stopEnabled,
			StopHint:     stopHint,
		})
	}
	return lanes
}

func (h *Handler) resolveAdminWorkerStartCapability(role string) (bool, string) {
	if strings.TrimSpace(h.resolveAdminWorkerStartCommand(role)) != "" {
		return true, ""
	}
	return true, "未配置启动命令，可恢复队列消费"
}

func (h *Handler) resolveAdminWorkerStopCapability(role string) (bool, string) {
	if strings.TrimSpace(h.resolveAdminWorkerStopCommand(role)) != "" {
		return true, ""
	}
	return true, "未配置停机命令，可暂停队列消费"
}

func (h *Handler) hasAnyAdminWorkerStartCommand() bool {
	for _, item := range []string{
		h.cfg.WorkerStartCommand,
		h.cfg.WorkerStartCommandGIF,
		h.cfg.WorkerStartCommandPNG,
		h.cfg.WorkerStartCommandMedia,
	} {
		if strings.TrimSpace(item) != "" {
			return true
		}
	}
	return false
}

func (h *Handler) hasAnyAdminWorkerStopCommand() bool {
	for _, item := range []string{
		h.cfg.WorkerStopCommand,
		h.cfg.WorkerStopCommandGIF,
		h.cfg.WorkerStopCommandPNG,
		h.cfg.WorkerStopCommandMedia,
	} {
		if strings.TrimSpace(item) != "" {
			return true
		}
	}
	return false
}

func (h *Handler) resolveAdminWorkerStartCommand(role string) string {
	switch normalizeAdminWorkerRole(role) {
	case adminWorkerRoleGIF:
		if value := strings.TrimSpace(h.cfg.WorkerStartCommandGIF); value != "" {
			return value
		}
	case adminWorkerRolePNG:
		if value := strings.TrimSpace(h.cfg.WorkerStartCommandPNG); value != "" {
			return value
		}
	case adminWorkerRoleMedia:
		if value := strings.TrimSpace(h.cfg.WorkerStartCommandMedia); value != "" {
			return value
		}
	}
	return strings.TrimSpace(h.cfg.WorkerStartCommand)
}

func (h *Handler) resolveAdminWorkerStopCommand(role string) string {
	switch normalizeAdminWorkerRole(role) {
	case adminWorkerRoleGIF:
		if value := strings.TrimSpace(h.cfg.WorkerStopCommandGIF); value != "" {
			return value
		}
	case adminWorkerRolePNG:
		if value := strings.TrimSpace(h.cfg.WorkerStopCommandPNG); value != "" {
			return value
		}
	case adminWorkerRoleMedia:
		if value := strings.TrimSpace(h.cfg.WorkerStopCommandMedia); value != "" {
			return value
		}
	}
	return strings.TrimSpace(h.cfg.WorkerStopCommand)
}

func (h *Handler) resolveAdminWorkerActionTimeoutSeconds() int {
	timeoutSec := h.cfg.WorkerStartTimeout
	if timeoutSec < 3 {
		timeoutSec = 3
	}
	if timeoutSec > 120 {
		timeoutSec = 120
	}
	return timeoutSec
}

func (h *Handler) executeAdminWorkerCommand(parent context.Context, command string) (output string, timedOut bool, err error) {
	timeoutSec := h.resolveAdminWorkerActionTimeoutSeconds()
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	rawOut, runErr := cmd.CombinedOutput()
	output = strings.TrimSpace(string(rawOut))
	if len(output) > 3000 {
		output = output[:3000] + "...<truncated>"
	}

	if ctx.Err() == context.DeadlineExceeded {
		return output, true, ctx.Err()
	}
	if runErr != nil {
		return output, false, runErr
	}
	return output, false, nil
}

func (h *Handler) toggleAdminWorkerQueues(role string, pause bool) ([]string, []string, error) {
	queueNames := resolveAdminWorkerQueueNamesByRole(role)
	actionLabel := "恢复"
	if pause {
		actionLabel = "暂停"
	}
	if len(queueNames) == 0 {
		return []string{}, []string{fmt.Sprintf("角色 %s 无匹配队列", role)}, nil
	}

	inspector := queue.NewInspector(h.cfg)
	defer inspector.Close()

	affected := make([]string, 0, len(queueNames))
	warnings := make([]string, 0, len(queueNames))
	fatalErrCount := 0
	for _, queueName := range queueNames {
		var opErr error
		if pause {
			opErr = inspector.PauseQueue(queueName)
		} else {
			opErr = inspector.UnpauseQueue(queueName)
		}
		if opErr != nil {
			if isAsynqQueueNotFoundErr(opErr) {
				warnings = append(warnings, fmt.Sprintf("队列 %s 不存在（可能尚未产生任务）", queueName))
				continue
			}
			fatalErrCount++
			warnings = append(warnings, fmt.Sprintf("%s队列 %s 失败: %v", actionLabel, queueName, opErr))
			continue
		}
		affected = append(affected, queueName)
	}

	if fatalErrCount >= len(queueNames) && len(affected) == 0 {
		return affected, warnings, fmt.Errorf("%s队列失败", actionLabel)
	}
	return affected, warnings, nil
}

func (h *Handler) countAdminActiveWorkersByRole(role string) (int, error) {
	inspector := queue.NewInspector(h.cfg)
	defer inspector.Close()
	servers, err := inspector.Servers()
	if err != nil {
		return 0, err
	}
	return countAdminActiveServersByRole(servers, role), nil
}

func (h *Handler) waitAdminWorkerOnline(role string, maxWait time.Duration) bool {
	if maxWait <= 0 {
		maxWait = 5 * time.Second
	}
	deadline := time.Now().Add(maxWait)
	for {
		count, err := h.countAdminActiveWorkersByRole(role)
		if err == nil && count > 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(400 * time.Millisecond)
	}
}

func (h *Handler) waitAdminWorkerOffline(role string, maxWait time.Duration) bool {
	if maxWait <= 0 {
		maxWait = 5 * time.Second
	}
	deadline := time.Now().Add(maxWait)
	for {
		count, err := h.countAdminActiveWorkersByRole(role)
		if err == nil && count <= 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(400 * time.Millisecond)
	}
}

func parseAdminWorkerActionParams(c *gin.Context) (role string, force bool, err error) {
	rawRole := strings.TrimSpace(c.Query("role"))
	if rawRole == "" {
		role = adminWorkerRoleAll
	} else {
		role = normalizeAdminWorkerRole(rawRole)
		if role == "" {
			return "", false, fmt.Errorf("invalid role, expect one of: all, gif, png, media")
		}
	}
	force = strings.EqualFold(strings.TrimSpace(c.Query("force")), "1") ||
		strings.EqualFold(strings.TrimSpace(c.Query("force")), "true")

	if c.Request.ContentLength > 0 {
		var req struct {
			Force bool   `json:"force"`
			Role  string `json:"role"`
		}
		if bindErr := c.ShouldBindJSON(&req); bindErr != nil {
			return "", false, fmt.Errorf("invalid request body")
		}
		force = force || req.Force
		if strings.TrimSpace(req.Role) != "" {
			normalized := normalizeAdminWorkerRole(req.Role)
			if normalized == "" {
				return "", false, fmt.Errorf("invalid role, expect one of: all, gif, png, media")
			}
			role = normalized
		}
	}
	return role, force, nil
}

func normalizeAdminWorkerRole(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", adminWorkerRoleAll:
		return adminWorkerRoleAll
	case adminWorkerRoleGIF:
		return adminWorkerRoleGIF
	case adminWorkerRolePNG, "image":
		return adminWorkerRolePNG
	case adminWorkerRoleMedia:
		return adminWorkerRoleMedia
	default:
		return ""
	}
}

func resolveAdminWorkerQueueNamesByRole(role string) []string {
	switch normalizeAdminWorkerRole(role) {
	case adminWorkerRoleGIF:
		return []string{videojobs.QueueVideoJobGIF}
	case adminWorkerRolePNG:
		return []string{videojobs.QueueVideoJobPNG}
	case adminWorkerRoleMedia:
		return []string{videojobs.QueueVideoJobMedia}
	default:
		return []string{
			videojobs.QueueVideoJobGIF,
			videojobs.QueueVideoJobPNG,
			videojobs.QueueVideoJobMedia,
		}
	}
}

func inferAdminWorkerRolesFromQueues(weights map[string]int) []string {
	if len(weights) == 0 {
		return []string{}
	}
	out := make([]string, 0, 3)
	for _, def := range adminWorkerLaneDefinitions {
		weight, ok := weights[def.QueueName]
		if !ok || weight <= 0 {
			continue
		}
		out = append(out, def.Role)
	}
	sort.Strings(out)
	return out
}

func countAdminServersByQueue(servers []*asynq.ServerInfo, queueName string, activeOnly bool) int {
	count := 0
	for _, item := range servers {
		if item == nil {
			continue
		}
		if activeOnly && !strings.EqualFold(strings.TrimSpace(item.Status), "active") {
			continue
		}
		weight, ok := item.Queues[queueName]
		if !ok || weight <= 0 {
			continue
		}
		count++
	}
	return count
}

func countAdminActiveServersByRole(servers []*asynq.ServerInfo, role string) int {
	normalizedRole := normalizeAdminWorkerRole(role)
	count := 0
	for _, item := range servers {
		if item == nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(item.Status), "active") {
			continue
		}
		if normalizedRole == adminWorkerRoleAll {
			count++
			continue
		}
		for _, queueName := range resolveAdminWorkerQueueNamesByRole(normalizedRole) {
			weight, ok := item.Queues[queueName]
			if !ok || weight <= 0 {
				continue
			}
			count++
			break
		}
	}
	return count
}

func (h *Handler) loadAdminStaleQueuedVideoJobs(now time.Time) (int64, float64, error) {
	var row struct {
		Count  int64      `gorm:"column:count"`
		Oldest *time.Time `gorm:"column:oldest"`
	}
	if err := h.db.Raw(`
SELECT
  COUNT(*)::bigint AS count,
  MIN(created_at) AS oldest
FROM public.video_image_jobs
WHERE status = ?
  AND created_at <= ?
`, models.VideoJobStatusQueued, now.Add(-adminWorkerStaleQueueDuration)).Scan(&row).Error; err != nil {
		return 0, 0, err
	}
	oldestAgeSec := 0.0
	if row.Oldest != nil {
		oldestAgeSec = now.Sub(*row.Oldest).Seconds()
		if oldestAgeSec < 0 {
			oldestAgeSec = 0
		}
	}
	return row.Count, oldestAgeSec, nil
}

func isAsynqQueueNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no such")
}

func finalizeAdminWorkerLaneHealth(lane AdminWorkerLaneStatus) (string, []string) {
	health := strings.ToLower(strings.TrimSpace(lane.Health))
	if health == "" || health == "unknown" {
		health = "green"
	}
	alerts := append([]string{}, lane.Alerts...)

	queueBacklog := lane.Queue.Pending + lane.Queue.Active + lane.Queue.Scheduled + lane.Queue.Retry
	if lane.ServersActive <= 0 {
		if queueBacklog > 0 {
			health = elevateAdminWorkerHealth(health, "red")
			alerts = append(alerts, "未发现在线 worker，但队列存在待处理任务")
		}
	}

	if lane.Queue.Paused {
		health = elevateAdminWorkerHealth(health, "yellow")
		alerts = append(alerts, "队列处于暂停状态")
	}

	if lane.Queue.LatencySeconds >= 120 {
		health = elevateAdminWorkerHealth(health, "yellow")
		alerts = append(alerts, fmt.Sprintf("队列延迟较高（%.0f 秒）", lane.Queue.LatencySeconds))
	}

	if lane.Queue.Pending >= 100 || lane.Queue.LatencySeconds >= 600 {
		health = elevateAdminWorkerHealth(health, "red")
	}

	return health, dedupeAdminWorkerAlerts(alerts)
}

func finalizeAdminWorkerHealth(in AdminWorkerHealthResponse) (string, []string) {
	health := strings.ToLower(strings.TrimSpace(in.Health))
	if health == "" {
		health = "green"
	}
	alerts := append([]string{}, in.Alerts...)

	if !in.RedisReachable {
		health = "red"
		if len(alerts) == 0 {
			alerts = append(alerts, "无法连接 Asynq/Redis")
		}
		return health, dedupeAdminWorkerAlerts(alerts)
	}

	if len(in.Lanes) > 0 {
		for _, lane := range in.Lanes {
			health = elevateAdminWorkerHealth(health, lane.Health)
			for _, laneAlert := range lane.Alerts {
				msg := strings.TrimSpace(laneAlert)
				if msg == "" {
					continue
				}
				label := strings.TrimSpace(lane.Label)
				if label == "" {
					label = strings.ToUpper(strings.TrimSpace(lane.Role))
				}
				alerts = append(alerts, fmt.Sprintf("[%s] %s", label, msg))
			}
		}
	} else {
		queueBacklog := in.Queue.Pending + in.Queue.Active + in.Queue.Scheduled + in.Queue.Retry
		if in.ServersActive <= 0 {
			if queueBacklog > 0 {
				health = elevateAdminWorkerHealth(health, "red")
				alerts = append(alerts, "未发现活跃 worker，但队列存在待处理任务")
			} else {
				health = elevateAdminWorkerHealth(health, "yellow")
				alerts = append(alerts, "未发现活跃 worker（当前队列空闲）")
			}
		}
		if in.Queue.Paused {
			health = elevateAdminWorkerHealth(health, "yellow")
			alerts = append(alerts, fmt.Sprintf("队列 %s 处于暂停状态", in.Queue.Name))
		}
		if in.Queue.LatencySeconds >= 120 {
			health = elevateAdminWorkerHealth(health, "yellow")
			alerts = append(alerts, fmt.Sprintf("队列延迟较高（%.0f 秒）", in.Queue.LatencySeconds))
		}
		if in.Queue.Pending >= 100 || in.Queue.LatencySeconds >= 600 {
			health = elevateAdminWorkerHealth(health, "red")
		}
	}

	if in.StaleQueuedJobs > 0 {
		health = elevateAdminWorkerHealth(health, "yellow")
		alerts = append(alerts, fmt.Sprintf("有 %d 个任务排队超过 2 分钟", in.StaleQueuedJobs))
	}
	if in.StaleQueuedJobs >= 20 {
		health = elevateAdminWorkerHealth(health, "red")
	}

	return health, dedupeAdminWorkerAlerts(alerts)
}

func elevateAdminWorkerHealth(current, target string) string {
	rank := map[string]int{"green": 1, "yellow": 2, "red": 3}
	current = strings.ToLower(strings.TrimSpace(current))
	target = strings.ToLower(strings.TrimSpace(target))
	if rank[target] > rank[current] {
		return target
	}
	if _, ok := rank[current]; ok {
		return current
	}
	if _, ok := rank[target]; ok {
		return target
	}
	return "green"
}

func dedupeAdminWorkerAlerts(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		msg := strings.TrimSpace(item)
		if msg == "" {
			continue
		}
		if _, ok := seen[msg]; ok {
			continue
		}
		seen[msg] = struct{}{}
		out = append(out, msg)
	}
	return out
}
