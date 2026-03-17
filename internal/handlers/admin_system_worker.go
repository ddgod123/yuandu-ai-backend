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

	"github.com/gin-gonic/gin"
)

const (
	adminWorkerQueueName          = "media"
	adminWorkerStaleQueueDuration = 2 * time.Minute
)

type AdminWorkerServerStatus struct {
	ID            string         `json:"id"`
	Host          string         `json:"host"`
	PID           int            `json:"pid"`
	Status        string         `json:"status"`
	StartedAt     string         `json:"started_at"`
	Concurrency   int            `json:"concurrency"`
	Queues        map[string]int `json:"queues"`
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

type AdminWorkerHealthResponse struct {
	CheckedAt          string                    `json:"checked_at"`
	Health             string                    `json:"health"`
	RedisReachable     bool                      `json:"redis_reachable"`
	RedisAddr          string                    `json:"redis_addr"`
	RedisDB            int                       `json:"redis_db"`
	QueueName          string                    `json:"queue_name"`
	ServersTotal       int                       `json:"servers_total"`
	ServersActive      int                       `json:"servers_active"`
	Servers            []AdminWorkerServerStatus `json:"servers"`
	Queue              AdminWorkerQueueStatus    `json:"queue"`
	StaleQueuedJobs    int64                     `json:"stale_queued_jobs"`
	OldestQueuedAgeSec float64                   `json:"oldest_queued_age_sec"`
	Alerts             []string                  `json:"alerts"`
	StartEnabled       bool                      `json:"start_enabled"`
	StartHint          string                    `json:"start_hint,omitempty"`
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
		QueueName:    adminWorkerQueueName,
		Servers:      make([]AdminWorkerServerStatus, 0, 2),
		Alerts:       make([]string, 0, 6),
		Queue:        AdminWorkerQueueStatus{Name: adminWorkerQueueName},
		StartEnabled: strings.TrimSpace(h.cfg.WorkerStartCommand) != "",
	}
	if !out.StartEnabled {
		out.StartHint = "未配置 WORKER_START_COMMAND，暂不支持一键启动"
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
			ActiveWorkers: len(item.ActiveWorkers),
		})
	}
	out.ServersTotal = len(out.Servers)

	queueInfo, qErr := inspector.GetQueueInfo(adminWorkerQueueName)
	if qErr != nil {
		if !isAsynqQueueNotFoundErr(qErr) {
			out.Health = elevateAdminWorkerHealth(out.Health, "yellow")
			out.Alerts = append(out.Alerts, fmt.Sprintf("读取 %s 队列失败: %v", adminWorkerQueueName, qErr))
		}
	} else if queueInfo != nil {
		out.Queue = AdminWorkerQueueStatus{
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
// @Param force query boolean false "force execute command even when worker is already online"
// @Success 200 {object} map[string]interface{}
// @Router /api/admin/system/worker-start [post]
func (h *Handler) StartAdminWorker(c *gin.Context) {
	startCommand := strings.TrimSpace(h.cfg.WorkerStartCommand)
	if startCommand == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "worker start is disabled",
			"hint":  "set WORKER_START_COMMAND to enable one-click start",
		})
		return
	}

	force := strings.EqualFold(strings.TrimSpace(c.Query("force")), "1") ||
		strings.EqualFold(strings.TrimSpace(c.Query("force")), "true")
	if c.Request.ContentLength > 0 {
		var req struct {
			Force bool `json:"force"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		force = force || req.Force
	}

	if activeServers, err := h.countAdminActiveWorkers(); err == nil && activeServers > 0 && !force {
		c.JSON(http.StatusOK, gin.H{
			"ok":              true,
			"already_running": true,
			"active_servers":  activeServers,
			"message":         "检测到 worker 已在线，若需强制执行请传 force=1",
		})
		return
	}

	timeoutSec := h.cfg.WorkerStartTimeout
	if timeoutSec < 3 {
		timeoutSec = 3
	}
	if timeoutSec > 120 {
		timeoutSec = 120
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", startCommand)
	rawOut, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(rawOut))
	if len(output) > 3000 {
		output = output[:3000] + "...<truncated>"
	}

	if ctx.Err() == context.DeadlineExceeded {
		c.JSON(http.StatusGatewayTimeout, gin.H{
			"error":           "start command timeout",
			"timeout_seconds": timeoutSec,
			"output":          output,
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "start command failed",
			"detail": err.Error(),
			"output": output,
		})
		return
	}

	started := h.waitAdminWorkerOnline(8 * time.Second)
	message := "启动命令已执行，等待 worker 心跳"
	if started {
		message = "worker 已在线"
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"started": started,
		"message": message,
		"output":  output,
	})
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

func (h *Handler) countAdminActiveWorkers() (int, error) {
	inspector := queue.NewInspector(h.cfg)
	defer inspector.Close()
	servers, err := inspector.Servers()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range servers {
		if strings.EqualFold(strings.TrimSpace(item.Status), "active") {
			count++
		}
	}
	return count, nil
}

func (h *Handler) waitAdminWorkerOnline(maxWait time.Duration) bool {
	if maxWait <= 0 {
		maxWait = 5 * time.Second
	}
	deadline := time.Now().Add(maxWait)
	for {
		count, err := h.countAdminActiveWorkers()
		if err == nil && count > 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(400 * time.Millisecond)
	}
}

func isAsynqQueueNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no such")
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

	queueBacklog := in.Queue.Pending + in.Queue.Active + in.Queue.Scheduled + in.Queue.Retry
	if in.ServersActive <= 0 {
		if queueBacklog > 0 || in.StaleQueuedJobs > 0 {
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

	if in.StaleQueuedJobs > 0 {
		health = elevateAdminWorkerHealth(health, "yellow")
		alerts = append(alerts, fmt.Sprintf("有 %d 个任务排队超过 2 分钟", in.StaleQueuedJobs))
	}

	if in.Queue.Pending >= 100 || in.Queue.LatencySeconds >= 600 || in.StaleQueuedJobs >= 20 {
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
