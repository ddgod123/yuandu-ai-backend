package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type adminVideoImageSplitBackfillStartRequest struct {
	Apply          *bool  `json:"apply"`
	BatchSize      int    `json:"batch_size"`
	Format         string `json:"format"`
	FallbackFormat string `json:"fallback_format"`
	Tables         string `json:"tables"`

	StartJobID      uint64 `json:"start_job_id"`
	StartOutputID   uint64 `json:"start_output_id"`
	StartPackageID  uint64 `json:"start_package_id"`
	StartEventID    uint64 `json:"start_event_id"`
	StartFeedbackID uint64 `json:"start_feedback_id"`

	LimitJobs      int `json:"limit_jobs"`
	LimitOutputs   int `json:"limit_outputs"`
	LimitPackages  int `json:"limit_packages"`
	LimitEvents    int `json:"limit_events"`
	LimitFeedbacks int `json:"limit_feedbacks"`
}

type adminVideoImageSplitBackfillOptionsPayload struct {
	Apply          bool   `json:"apply"`
	BatchSize      int    `json:"batch_size"`
	Format         string `json:"format,omitempty"`
	FallbackFormat string `json:"fallback_format"`
	Tables         string `json:"tables"`

	StartJobID      uint64 `json:"start_job_id"`
	StartOutputID   uint64 `json:"start_output_id"`
	StartPackageID  uint64 `json:"start_package_id"`
	StartEventID    uint64 `json:"start_event_id"`
	StartFeedbackID uint64 `json:"start_feedback_id"`

	LimitJobs      int `json:"limit_jobs"`
	LimitOutputs   int `json:"limit_outputs"`
	LimitPackages  int `json:"limit_packages"`
	LimitEvents    int `json:"limit_events"`
	LimitFeedbacks int `json:"limit_feedbacks"`
}

type adminVideoImageSplitBackfillStatusResponse struct {
	Running       bool                                           `json:"running"`
	StopRequested bool                                           `json:"stop_requested"`
	RunID         string                                         `json:"run_id,omitempty"`
	RequestedBy   uint64                                         `json:"requested_by,omitempty"`
	StartedAt     *time.Time                                     `json:"started_at,omitempty"`
	FinishedAt    *time.Time                                     `json:"finished_at,omitempty"`
	HeartbeatAt   *time.Time                                     `json:"heartbeat_at,omitempty"`
	LastError     string                                         `json:"last_error,omitempty"`
	Options       adminVideoImageSplitBackfillOptionsPayload     `json:"options"`
	Lease         adminVideoImageSplitBackfillLeaseStatus        `json:"lease"`
	Report        *videojobs.PublicVideoImageSplitBackfillReport `json:"report,omitempty"`
	History       []adminVideoImageSplitBackfillHistoryItem      `json:"history,omitempty"`
}

type adminVideoImageSplitBackfillLeaseStatus struct {
	OwnerInstance    string     `json:"owner_instance,omitempty"`
	IsLocalOwner     bool       `json:"is_local_owner"`
	TimeoutSeconds   int64      `json:"timeout_seconds"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	RemainingSeconds int64      `json:"remaining_seconds"`
	CanTakeover      bool       `json:"can_takeover"`
}

type adminVideoImageSplitBackfillHistoryItem struct {
	RunID       string                                         `json:"run_id"`
	Status      string                                         `json:"status"`
	RequestedBy uint64                                         `json:"requested_by,omitempty"`
	StartedAt   *time.Time                                     `json:"started_at,omitempty"`
	FinishedAt  *time.Time                                     `json:"finished_at,omitempty"`
	Stopped     bool                                           `json:"stopped"`
	LastError   string                                         `json:"last_error,omitempty"`
	Options     adminVideoImageSplitBackfillOptionsPayload     `json:"options"`
	Report      *videojobs.PublicVideoImageSplitBackfillReport `json:"report,omitempty"`
}

type adminVideoImageSplitBackfillRuntimeState struct {
	mu            sync.RWMutex
	running       bool
	stopRequested bool
	runID         string
	requestedBy   uint64
	startedAt     *time.Time
	finishedAt    *time.Time
	heartbeatAt   *time.Time
	lastError     string
	options       adminVideoImageSplitBackfillOptionsPayload
	report        *videojobs.PublicVideoImageSplitBackfillReport
	stopCh        chan struct{}
}

const (
	adminVideoImageSplitBackfillCoordinatorRunID = "__split_backfill_coordinator__"
	adminVideoImageSplitBackfillRunningStatus    = "running"
	adminVideoImageSplitBackfillLeaseTimeout     = 5 * time.Minute
	adminVideoImageSplitBackfillDBPollInterval   = 800 * time.Millisecond
)

var errAdminVideoImageSplitBackfillActiveRunExists = errors.New("active backfill run exists")

var adminVideoImageSplitBackfillRuntime = &adminVideoImageSplitBackfillRuntimeState{
	options: adminVideoImageSplitBackfillOptionsPayload{
		Apply:          false,
		BatchSize:      500,
		FallbackFormat: "gif",
		Tables:         "jobs,outputs,packages,events,feedbacks",
	},
}

// GetAdminVideoImageSplitBackfillStatus godoc
// @Summary 查看视频任务分表回填执行状态
// @Tags admin
// @Produce json
// @Success 200 {object} adminVideoImageSplitBackfillStatusResponse
// @Router /api/admin/video-jobs/split-backfill [get]
func (h *Handler) GetAdminVideoImageSplitBackfillStatus(c *gin.Context) {
	status := snapshotAdminVideoImageSplitBackfillStatus()
	if h != nil {
		_, _ = h.markAdminVideoImageSplitBackfillStaleRuns(time.Now())
		h.mergeAdminVideoImageSplitBackfillDBStatus(&status)
		h.attachAdminVideoImageSplitBackfillHistory(&status, 20)
	}
	c.JSON(http.StatusOK, status)
}

// StartAdminVideoImageSplitBackfill godoc
// @Summary 启动视频任务分表回填
// @Tags admin
// @Accept json
// @Produce json
// @Param body body adminVideoImageSplitBackfillStartRequest false "start request"
// @Success 200 {object} adminVideoImageSplitBackfillStatusResponse
// @Router /api/admin/video-jobs/split-backfill/start [post]
func (h *Handler) StartAdminVideoImageSplitBackfill(c *gin.Context) {
	var req adminVideoImageSplitBackfillStartRequest
	_ = c.ShouldBindJSON(&req)

	adminID, _ := currentUserIDFromContext(c)
	apply := req.Apply != nil && *req.Apply

	optionsPayload := adminVideoImageSplitBackfillOptionsPayload{
		Apply:           apply,
		BatchSize:       req.BatchSize,
		Format:          strings.TrimSpace(req.Format),
		FallbackFormat:  strings.TrimSpace(req.FallbackFormat),
		Tables:          strings.TrimSpace(req.Tables),
		StartJobID:      req.StartJobID,
		StartOutputID:   req.StartOutputID,
		StartPackageID:  req.StartPackageID,
		StartEventID:    req.StartEventID,
		StartFeedbackID: req.StartFeedbackID,
		LimitJobs:       req.LimitJobs,
		LimitOutputs:    req.LimitOutputs,
		LimitPackages:   req.LimitPackages,
		LimitEvents:     req.LimitEvents,
		LimitFeedbacks:  req.LimitFeedbacks,
	}
	if optionsPayload.BatchSize <= 0 || optionsPayload.BatchSize > 5000 {
		optionsPayload.BatchSize = 500
	}
	if optionsPayload.FallbackFormat == "" {
		optionsPayload.FallbackFormat = "gif"
	}
	if optionsPayload.Tables == "" {
		optionsPayload.Tables = "jobs,outputs,packages,events,feedbacks"
	}
	includeJobs, includeOutputs, includePackages, includeEvents, includeFeedbacks := parseAdminVideoImageSplitBackfillTables(optionsPayload.Tables)

	if h == nil || h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "split backfill db is unavailable"})
		return
	}

	runID := fmt.Sprintf("split-backfill-%d-%s", time.Now().UnixNano(), adminVideoImageSplitBackfillInstanceID())
	now := time.Now()

	activeRun, err := h.persistAdminVideoImageSplitBackfillRunStartWithLease(runID, adminID, optionsPayload, now)
	if err != nil {
		if errors.Is(err, errAdminVideoImageSplitBackfillActiveRunExists) {
			status := snapshotAdminVideoImageSplitBackfillStatus()
			if activeRun != nil {
				dbStatus := buildAdminVideoImageSplitBackfillStatusFromRun(*activeRun, true)
				mergeAdminVideoImageSplitBackfillStatusByRecency(&status, dbStatus)
			} else {
				h.mergeAdminVideoImageSplitBackfillDBStatus(&status)
			}
			h.attachAdminVideoImageSplitBackfillHistory(&status, 20)
			c.JSON(http.StatusConflict, gin.H{
				"error":  "backfill is already running",
				"status": status,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist split backfill start failed"})
		return
	}

	stopCh := make(chan struct{})
	adminVideoImageSplitBackfillRuntime.mu.Lock()
	adminVideoImageSplitBackfillRuntime.running = true
	adminVideoImageSplitBackfillRuntime.stopRequested = false
	adminVideoImageSplitBackfillRuntime.runID = runID
	adminVideoImageSplitBackfillRuntime.requestedBy = adminID
	adminVideoImageSplitBackfillRuntime.startedAt = &now
	adminVideoImageSplitBackfillRuntime.finishedAt = nil
	adminVideoImageSplitBackfillRuntime.heartbeatAt = &now
	adminVideoImageSplitBackfillRuntime.lastError = ""
	adminVideoImageSplitBackfillRuntime.options = optionsPayload
	adminVideoImageSplitBackfillRuntime.report = &videojobs.PublicVideoImageSplitBackfillReport{
		Apply:          optionsPayload.Apply,
		BatchSize:      optionsPayload.BatchSize,
		FormatFilter:   optionsPayload.Format,
		FallbackFormat: optionsPayload.FallbackFormat,
		StartedAt:      now,
	}
	adminVideoImageSplitBackfillRuntime.stopCh = stopCh
	adminVideoImageSplitBackfillRuntime.mu.Unlock()

	go func(localRunID string, localOptions adminVideoImageSplitBackfillOptionsPayload, localStopCh <-chan struct{}) {
		lastPersistAt := time.Now()
		dbStopChecker := newAdminVideoImageSplitBackfillDBStopChecker(h, localRunID, adminVideoImageSplitBackfillDBPollInterval)
		opt := videojobs.PublicVideoImageSplitBackfillOptions{
			Apply:            localOptions.Apply,
			BatchSize:        localOptions.BatchSize,
			FormatFilter:     localOptions.Format,
			FallbackFormat:   localOptions.FallbackFormat,
			StartJobID:       localOptions.StartJobID,
			StartOutputID:    localOptions.StartOutputID,
			StartPackageID:   localOptions.StartPackageID,
			StartEventID:     localOptions.StartEventID,
			StartFeedbackID:  localOptions.StartFeedbackID,
			LimitJobs:        localOptions.LimitJobs,
			LimitOutputs:     localOptions.LimitOutputs,
			LimitPackages:    localOptions.LimitPackages,
			LimitEvents:      localOptions.LimitEvents,
			LimitFeedbacks:   localOptions.LimitFeedbacks,
			IncludeJobs:      includeJobs,
			IncludeOutputs:   includeOutputs,
			IncludePackages:  includePackages,
			IncludeEvents:    includeEvents,
			IncludeFeedbacks: includeFeedbacks,
			StopSignal:       localStopCh,
			ShouldStop:       dbStopChecker.ShouldStop,
			OnProgress: func(table string, stats videojobs.PublicVideoImageSplitBackfillTableReport) {
				adminVideoImageSplitBackfillRuntime.mu.Lock()
				defer adminVideoImageSplitBackfillRuntime.mu.Unlock()
				if adminVideoImageSplitBackfillRuntime.runID != localRunID {
					return
				}
				if adminVideoImageSplitBackfillRuntime.report == nil {
					now := time.Now()
					adminVideoImageSplitBackfillRuntime.report = &videojobs.PublicVideoImageSplitBackfillReport{
						Apply:          localOptions.Apply,
						BatchSize:      localOptions.BatchSize,
						FormatFilter:   localOptions.Format,
						FallbackFormat: localOptions.FallbackFormat,
						StartedAt:      now,
					}
				}
				switch strings.TrimSpace(strings.ToLower(table)) {
				case "jobs":
					adminVideoImageSplitBackfillRuntime.report.Jobs = stats
				case "outputs":
					adminVideoImageSplitBackfillRuntime.report.Outputs = stats
				case "packages":
					adminVideoImageSplitBackfillRuntime.report.Packages = stats
				case "events":
					adminVideoImageSplitBackfillRuntime.report.Events = stats
				case "feedbacks":
					adminVideoImageSplitBackfillRuntime.report.Feedbacks = stats
				}
				now := time.Now()
				adminVideoImageSplitBackfillRuntime.heartbeatAt = &now
				if dbStopChecker.LastKnownStopRequested() {
					adminVideoImageSplitBackfillRuntime.stopRequested = true
				}
				if now.Sub(lastPersistAt) >= 2*time.Second {
					_ = h.persistAdminVideoImageSplitBackfillRunProgress(localRunID, now, adminVideoImageSplitBackfillRuntime.report)
					lastPersistAt = now
				}
			},
		}
		report, err := videojobs.BackfillPublicVideoImageSplitTables(h.db, opt)

		now := time.Now()
		lastError := ""
		if err != nil {
			lastError = err.Error()
		}
		if dbStopChecker.LastKnownStopRequested() && !report.Stopped {
			report.Stopped = true
		}
		_ = h.persistAdminVideoImageSplitBackfillRunFinish(
			localRunID,
			resolveAdminVideoImageSplitBackfillHistoryStatus(err, report),
			now,
			lastError,
			clonePublicVideoImageSplitBackfillReportPtr(&report),
		)

		adminVideoImageSplitBackfillRuntime.mu.Lock()
		defer adminVideoImageSplitBackfillRuntime.mu.Unlock()
		if adminVideoImageSplitBackfillRuntime.runID != localRunID {
			return
		}
		adminVideoImageSplitBackfillRuntime.running = false
		adminVideoImageSplitBackfillRuntime.finishedAt = &now
		adminVideoImageSplitBackfillRuntime.heartbeatAt = &now
		adminVideoImageSplitBackfillRuntime.stopCh = nil
		adminVideoImageSplitBackfillRuntime.stopRequested = adminVideoImageSplitBackfillRuntime.stopRequested || report.Stopped
		adminVideoImageSplitBackfillRuntime.report = &report
		if err != nil {
			adminVideoImageSplitBackfillRuntime.lastError = err.Error()
		} else {
			adminVideoImageSplitBackfillRuntime.lastError = ""
		}
	}(runID, optionsPayload, stopCh)

	status := snapshotAdminVideoImageSplitBackfillStatus()
	h.mergeAdminVideoImageSplitBackfillDBStatus(&status)
	h.attachAdminVideoImageSplitBackfillHistory(&status, 20)
	c.JSON(http.StatusOK, status)
}

// StopAdminVideoImageSplitBackfill godoc
// @Summary 停止正在执行的视频任务分表回填
// @Tags admin
// @Produce json
// @Success 200 {object} adminVideoImageSplitBackfillStatusResponse
// @Router /api/admin/video-jobs/split-backfill/stop [post]
func (h *Handler) StopAdminVideoImageSplitBackfill(c *gin.Context) {
	if h != nil {
		_, _ = h.markAdminVideoImageSplitBackfillStaleRuns(time.Now())
	}
	requestRunID := strings.TrimSpace(c.Query("run_id"))
	targetRunID := requestRunID

	adminVideoImageSplitBackfillRuntime.mu.RLock()
	localRunID := strings.TrimSpace(adminVideoImageSplitBackfillRuntime.runID)
	localRunning := adminVideoImageSplitBackfillRuntime.running
	adminVideoImageSplitBackfillRuntime.mu.RUnlock()

	if targetRunID == "" && localRunning {
		targetRunID = localRunID
	}
	if targetRunID == "" && h != nil {
		if active, err := h.loadActiveAdminVideoImageSplitBackfillRun(time.Now()); err == nil && active != nil {
			targetRunID = strings.TrimSpace(active.RunID)
		}
	}
	if targetRunID != "" && h != nil {
		_ = h.persistAdminVideoImageSplitBackfillRequestStop(targetRunID, time.Now())
	}

	adminVideoImageSplitBackfillRuntime.mu.Lock()
	if adminVideoImageSplitBackfillRuntime.running &&
		(targetRunID == "" || strings.EqualFold(strings.TrimSpace(adminVideoImageSplitBackfillRuntime.runID), targetRunID)) &&
		!adminVideoImageSplitBackfillRuntime.stopRequested &&
		adminVideoImageSplitBackfillRuntime.stopCh != nil {
		close(adminVideoImageSplitBackfillRuntime.stopCh)
		adminVideoImageSplitBackfillRuntime.stopRequested = true
		now := time.Now()
		adminVideoImageSplitBackfillRuntime.heartbeatAt = &now
	} else if adminVideoImageSplitBackfillRuntime.running &&
		targetRunID != "" &&
		strings.EqualFold(strings.TrimSpace(adminVideoImageSplitBackfillRuntime.runID), targetRunID) {
		adminVideoImageSplitBackfillRuntime.stopRequested = true
		now := time.Now()
		adminVideoImageSplitBackfillRuntime.heartbeatAt = &now
	}
	status := buildAdminVideoImageSplitBackfillStatusLocked()
	adminVideoImageSplitBackfillRuntime.mu.Unlock()
	if h != nil {
		h.mergeAdminVideoImageSplitBackfillDBStatus(&status)
		h.attachAdminVideoImageSplitBackfillHistory(&status, 20)
	}
	c.JSON(http.StatusOK, status)
}

func snapshotAdminVideoImageSplitBackfillStatus() adminVideoImageSplitBackfillStatusResponse {
	adminVideoImageSplitBackfillRuntime.mu.RLock()
	defer adminVideoImageSplitBackfillRuntime.mu.RUnlock()
	return buildAdminVideoImageSplitBackfillStatusLocked()
}

func buildAdminVideoImageSplitBackfillStatusLocked() adminVideoImageSplitBackfillStatusResponse {
	resp := adminVideoImageSplitBackfillStatusResponse{
		Running:       adminVideoImageSplitBackfillRuntime.running,
		StopRequested: adminVideoImageSplitBackfillRuntime.stopRequested,
		RunID:         adminVideoImageSplitBackfillRuntime.runID,
		RequestedBy:   adminVideoImageSplitBackfillRuntime.requestedBy,
		StartedAt:     adminVideoImageSplitBackfillRuntime.startedAt,
		FinishedAt:    adminVideoImageSplitBackfillRuntime.finishedAt,
		HeartbeatAt:   adminVideoImageSplitBackfillRuntime.heartbeatAt,
		LastError:     strings.TrimSpace(adminVideoImageSplitBackfillRuntime.lastError),
		Options:       adminVideoImageSplitBackfillRuntime.options,
	}
	if adminVideoImageSplitBackfillRuntime.report != nil {
		resp.Report = clonePublicVideoImageSplitBackfillReportPtr(adminVideoImageSplitBackfillRuntime.report)
	}
	resp.Lease = buildAdminVideoImageSplitBackfillLeaseStatus(resp, time.Now())
	return resp
}

func resolveAdminVideoImageSplitBackfillHistoryStatus(err error, report videojobs.PublicVideoImageSplitBackfillReport) string {
	if err != nil {
		return "failed"
	}
	if report.Stopped {
		return "stopped"
	}
	if report.FailedTotal() > 0 {
		return "done_with_errors"
	}
	return "done"
}

func clonePublicVideoImageSplitBackfillReportPtr(
	in *videojobs.PublicVideoImageSplitBackfillReport,
) *videojobs.PublicVideoImageSplitBackfillReport {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func adminVideoImageSplitBackfillInstanceID() string {
	host := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if host == "" {
		if value, err := os.Hostname(); err == nil {
			host = strings.TrimSpace(value)
		}
	}
	if host == "" {
		host = "local"
	}
	pid := os.Getpid()
	return strings.ToLower(strings.TrimSpace(host)) + "-" + strconv.Itoa(pid)
}

func mergeAdminVideoImageSplitBackfillStatusByRecency(
	base *adminVideoImageSplitBackfillStatusResponse,
	incoming adminVideoImageSplitBackfillStatusResponse,
) {
	if base == nil {
		return
	}
	if !incoming.Running {
		base.Lease = buildAdminVideoImageSplitBackfillLeaseStatus(*base, time.Now())
		return
	}
	if !base.Running {
		*base = incoming
		base.Lease = buildAdminVideoImageSplitBackfillLeaseStatus(*base, time.Now())
		return
	}
	if strings.EqualFold(strings.TrimSpace(base.RunID), strings.TrimSpace(incoming.RunID)) {
		if incoming.StopRequested {
			base.StopRequested = true
		}
		if base.Report == nil && incoming.Report != nil {
			base.Report = clonePublicVideoImageSplitBackfillReportPtr(incoming.Report)
		}
		if adminVideoImageSplitBackfillTimeAfter(incoming.HeartbeatAt, base.HeartbeatAt) {
			base.HeartbeatAt = incoming.HeartbeatAt
			base.StartedAt = incoming.StartedAt
			base.FinishedAt = incoming.FinishedAt
			base.LastError = incoming.LastError
			base.Options = incoming.Options
			base.Report = clonePublicVideoImageSplitBackfillReportPtr(incoming.Report)
		}
		base.Lease = buildAdminVideoImageSplitBackfillLeaseStatus(*base, time.Now())
		return
	}
	if adminVideoImageSplitBackfillTimeAfter(incoming.HeartbeatAt, base.HeartbeatAt) {
		*base = incoming
	}
	base.Lease = buildAdminVideoImageSplitBackfillLeaseStatus(*base, time.Now())
}

func adminVideoImageSplitBackfillTimeAfter(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}

func buildAdminVideoImageSplitBackfillLeaseStatus(
	status adminVideoImageSplitBackfillStatusResponse,
	now time.Time,
) adminVideoImageSplitBackfillLeaseStatus {
	lease := adminVideoImageSplitBackfillLeaseStatus{
		TimeoutSeconds: int64(adminVideoImageSplitBackfillLeaseTimeout / time.Second),
		CanTakeover:    true,
	}
	runID := strings.TrimSpace(status.RunID)
	if runID == "" || !status.Running {
		return lease
	}
	lease.OwnerInstance = extractAdminVideoImageSplitBackfillInstanceFromRunID(runID)
	lease.IsLocalOwner = strings.EqualFold(
		strings.TrimSpace(lease.OwnerInstance),
		strings.TrimSpace(adminVideoImageSplitBackfillInstanceID()),
	)
	base := status.HeartbeatAt
	if base == nil {
		base = status.StartedAt
	}
	if base == nil {
		lease.CanTakeover = false
		return lease
	}
	expiresAt := base.Add(adminVideoImageSplitBackfillLeaseTimeout)
	lease.ExpiresAt = &expiresAt
	remaining := int64(expiresAt.Sub(now).Seconds())
	lease.RemainingSeconds = remaining
	lease.CanTakeover = remaining <= 0
	return lease
}

func extractAdminVideoImageSplitBackfillInstanceFromRunID(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ""
	}
	parts := strings.SplitN(runID, "-", 4)
	if len(parts) < 4 {
		return ""
	}
	if strings.TrimSpace(parts[0]) != "split" || strings.TrimSpace(parts[1]) != "backfill" {
		return ""
	}
	return strings.TrimSpace(parts[3])
}

func buildAdminVideoImageSplitBackfillStatusFromRun(
	row models.VideoImageSplitBackfillRun,
	running bool,
) adminVideoImageSplitBackfillStatusResponse {
	options := decodeAdminVideoImageSplitBackfillOptions(row.OptionsJSON)
	if strings.TrimSpace(options.Tables) == "" {
		options.Tables = strings.TrimSpace(row.Tables)
	}
	if options.BatchSize <= 0 {
		options.BatchSize = row.BatchSize
	}
	options.Apply = row.Apply
	if strings.TrimSpace(options.Format) == "" {
		options.Format = strings.TrimSpace(row.FormatFilter)
	}
	if strings.TrimSpace(options.FallbackFormat) == "" {
		options.FallbackFormat = strings.TrimSpace(row.FallbackFormat)
	}
	resp := adminVideoImageSplitBackfillStatusResponse{
		Running:       running,
		StopRequested: running && row.Stopped,
		RunID:         strings.TrimSpace(row.RunID),
		RequestedBy:   row.RequestedBy,
		StartedAt:     row.StartedAt,
		FinishedAt:    row.FinishedAt,
		HeartbeatAt:   row.HeartbeatAt,
		LastError:     strings.TrimSpace(row.LastError),
		Options:       options,
	}
	resp.Report = decodeAdminVideoImageSplitBackfillReport(row.ReportJSON)
	resp.Lease = buildAdminVideoImageSplitBackfillLeaseStatus(resp, time.Now())
	return resp
}

func (h *Handler) mergeAdminVideoImageSplitBackfillDBStatus(status *adminVideoImageSplitBackfillStatusResponse) {
	if h == nil || h.db == nil || status == nil {
		return
	}
	active, err := h.loadActiveAdminVideoImageSplitBackfillRun(time.Now())
	if err != nil || active == nil {
		return
	}
	mergeAdminVideoImageSplitBackfillStatusByRecency(status, buildAdminVideoImageSplitBackfillStatusFromRun(*active, true))
}

func (h *Handler) loadActiveAdminVideoImageSplitBackfillRun(now time.Time) (*models.VideoImageSplitBackfillRun, error) {
	if h == nil || h.db == nil {
		return nil, nil
	}
	return loadActiveAdminVideoImageSplitBackfillRunTx(h.db, now)
}

func loadActiveAdminVideoImageSplitBackfillRunTx(tx *gorm.DB, now time.Time) (*models.VideoImageSplitBackfillRun, error) {
	if tx == nil {
		return nil, nil
	}
	cutoff := now.Add(-adminVideoImageSplitBackfillLeaseTimeout)
	var row models.VideoImageSplitBackfillRun
	err := tx.Model(&models.VideoImageSplitBackfillRun{}).
		Where("run_id <> ?", adminVideoImageSplitBackfillCoordinatorRunID).
		Where("status = ?", adminVideoImageSplitBackfillRunningStatus).
		Where("heartbeat_at IS NULL OR heartbeat_at >= ?", cutoff).
		Order("heartbeat_at DESC, id DESC").
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (h *Handler) markAdminVideoImageSplitBackfillStaleRuns(now time.Time) (int64, error) {
	if h == nil || h.db == nil {
		return 0, nil
	}
	return markAdminVideoImageSplitBackfillStaleRunsTx(h.db, now)
}

func markAdminVideoImageSplitBackfillStaleRunsTx(tx *gorm.DB, now time.Time) (int64, error) {
	if tx == nil {
		return 0, nil
	}
	cutoff := now.Add(-adminVideoImageSplitBackfillLeaseTimeout)
	updates := map[string]interface{}{
		"status":       "timeout",
		"finished_at":  now,
		"heartbeat_at": now,
		"last_error":   "backfill lease expired",
	}
	result := tx.Model(&models.VideoImageSplitBackfillRun{}).
		Where("run_id <> ?", adminVideoImageSplitBackfillCoordinatorRunID).
		Where("status = ?", adminVideoImageSplitBackfillRunningStatus).
		Where("(heartbeat_at IS NOT NULL AND heartbeat_at < ?) OR (heartbeat_at IS NULL AND started_at < ?)", cutoff, cutoff).
		Updates(updates)
	return result.RowsAffected, result.Error
}

func ensureAdminVideoImageSplitBackfillCoordinatorRunTx(tx *gorm.DB, now time.Time) error {
	if tx == nil {
		return nil
	}
	emptyJSON := datatypes.JSON([]byte("{}"))
	coordinator := models.VideoImageSplitBackfillRun{
		RunID:          adminVideoImageSplitBackfillCoordinatorRunID,
		Status:         "system",
		RequestedBy:    0,
		Apply:          false,
		BatchSize:      0,
		FormatFilter:   "",
		FallbackFormat: "",
		Tables:         "",
		Stopped:        false,
		LastError:      "",
		OptionsJSON:    emptyJSON,
		ReportJSON:     emptyJSON,
		StartedAt:      &now,
		FinishedAt:     &now,
		HeartbeatAt:    &now,
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "run_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"status":       "system",
			"heartbeat_at": now,
		}),
	}).Create(&coordinator).Error
}

type adminVideoImageSplitBackfillDBStopChecker struct {
	handler       *Handler
	runID         string
	minInterval   time.Duration
	mu            sync.Mutex
	lastCheckAt   time.Time
	stopRequested bool
}

func newAdminVideoImageSplitBackfillDBStopChecker(
	handler *Handler,
	runID string,
	minInterval time.Duration,
) *adminVideoImageSplitBackfillDBStopChecker {
	if minInterval <= 0 {
		minInterval = adminVideoImageSplitBackfillDBPollInterval
	}
	return &adminVideoImageSplitBackfillDBStopChecker{
		handler:     handler,
		runID:       strings.TrimSpace(runID),
		minInterval: minInterval,
	}
}

func (s *adminVideoImageSplitBackfillDBStopChecker) ShouldStop() bool {
	if s == nil || s.handler == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if !s.lastCheckAt.IsZero() && now.Sub(s.lastCheckAt) < s.minInterval {
		return s.stopRequested
	}
	s.lastCheckAt = now
	stopRequested, err := s.handler.shouldStopAdminVideoImageSplitBackfillRunFromDB(s.runID)
	if err != nil {
		return s.stopRequested
	}
	s.stopRequested = stopRequested
	return s.stopRequested
}

func (s *adminVideoImageSplitBackfillDBStopChecker) LastKnownStopRequested() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopRequested
}

func (h *Handler) shouldStopAdminVideoImageSplitBackfillRunFromDB(runID string) (bool, error) {
	if h == nil || h.db == nil {
		return false, nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return true, nil
	}
	var row models.VideoImageSplitBackfillRun
	err := h.db.Model(&models.VideoImageSplitBackfillRun{}).
		Select("run_id", "status", "stopped", "heartbeat_at").
		Where("run_id = ?", runID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	status := strings.TrimSpace(strings.ToLower(row.Status))
	if status != adminVideoImageSplitBackfillRunningStatus {
		return true, nil
	}
	if row.Stopped {
		return true, nil
	}
	return false, nil
}

func (h *Handler) attachAdminVideoImageSplitBackfillHistory(status *adminVideoImageSplitBackfillStatusResponse, limit int) {
	if h == nil || status == nil {
		return
	}
	items, err := h.listAdminVideoImageSplitBackfillHistory(limit)
	if err != nil {
		return
	}
	status.History = items
}

func (h *Handler) listAdminVideoImageSplitBackfillHistory(limit int) ([]adminVideoImageSplitBackfillHistoryItem, error) {
	if h == nil || h.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []models.VideoImageSplitBackfillRun
	if err := h.db.Model(&models.VideoImageSplitBackfillRun{}).
		Where("run_id <> ?", adminVideoImageSplitBackfillCoordinatorRunID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]adminVideoImageSplitBackfillHistoryItem, 0, len(rows))
	for _, row := range rows {
		options := decodeAdminVideoImageSplitBackfillOptions(row.OptionsJSON)
		if strings.TrimSpace(options.Tables) == "" {
			options.Tables = strings.TrimSpace(row.Tables)
		}
		if options.BatchSize <= 0 {
			options.BatchSize = row.BatchSize
		}
		options.Apply = row.Apply
		if strings.TrimSpace(options.Format) == "" {
			options.Format = strings.TrimSpace(row.FormatFilter)
		}
		if strings.TrimSpace(options.FallbackFormat) == "" {
			options.FallbackFormat = strings.TrimSpace(row.FallbackFormat)
		}
		items = append(items, adminVideoImageSplitBackfillHistoryItem{
			RunID:       strings.TrimSpace(row.RunID),
			Status:      strings.TrimSpace(row.Status),
			RequestedBy: row.RequestedBy,
			StartedAt:   row.StartedAt,
			FinishedAt:  row.FinishedAt,
			Stopped:     row.Stopped,
			LastError:   strings.TrimSpace(row.LastError),
			Options:     options,
			Report:      decodeAdminVideoImageSplitBackfillReport(row.ReportJSON),
		})
	}
	return items, nil
}

func (h *Handler) persistAdminVideoImageSplitBackfillRunStartWithLease(
	runID string,
	requestedBy uint64,
	options adminVideoImageSplitBackfillOptionsPayload,
	startedAt time.Time,
) (*models.VideoImageSplitBackfillRun, error) {
	if h == nil || h.db == nil {
		return nil, nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, nil
	}
	optionsJSON, _ := json.Marshal(options)
	reportJSON, _ := json.Marshal(videojobs.PublicVideoImageSplitBackfillReport{
		Apply:          options.Apply,
		BatchSize:      options.BatchSize,
		FormatFilter:   options.Format,
		FallbackFormat: options.FallbackFormat,
		StartedAt:      startedAt,
	})
	startedAtCopy := startedAt

	var active *models.VideoImageSplitBackfillRun
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureAdminVideoImageSplitBackfillCoordinatorRunTx(tx, startedAt); err != nil {
			return err
		}
		var coordinator models.VideoImageSplitBackfillRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("run_id = ?", adminVideoImageSplitBackfillCoordinatorRunID).
			First(&coordinator).Error; err != nil {
			return err
		}
		if _, err := markAdminVideoImageSplitBackfillStaleRunsTx(tx, startedAt); err != nil {
			return err
		}
		found, err := loadActiveAdminVideoImageSplitBackfillRunTx(tx, startedAt)
		if err != nil {
			return err
		}
		if found != nil {
			active = found
			return errAdminVideoImageSplitBackfillActiveRunExists
		}
		row := models.VideoImageSplitBackfillRun{
			RunID:          runID,
			Status:         adminVideoImageSplitBackfillRunningStatus,
			RequestedBy:    requestedBy,
			Apply:          options.Apply,
			BatchSize:      options.BatchSize,
			FormatFilter:   strings.TrimSpace(options.Format),
			FallbackFormat: strings.TrimSpace(options.FallbackFormat),
			Tables:         strings.TrimSpace(options.Tables),
			Stopped:        false,
			LastError:      "",
			OptionsJSON:    datatypes.JSON(optionsJSON),
			ReportJSON:     datatypes.JSON(reportJSON),
			StartedAt:      &startedAtCopy,
			HeartbeatAt:    &startedAtCopy,
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return active, err
	}
	return nil, nil
}

func (h *Handler) persistAdminVideoImageSplitBackfillRequestStop(runID string, now time.Time) error {
	if h == nil || h.db == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	return h.db.Model(&models.VideoImageSplitBackfillRun{}).
		Where("run_id = ?", runID).
		Where("status = ?", adminVideoImageSplitBackfillRunningStatus).
		Updates(map[string]interface{}{
			"stopped":      true,
			"heartbeat_at": now,
		}).Error
}

func (h *Handler) persistAdminVideoImageSplitBackfillRunProgress(
	runID string,
	heartbeatAt time.Time,
	report *videojobs.PublicVideoImageSplitBackfillReport,
) error {
	if h == nil || h.db == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	updates := map[string]interface{}{
		"heartbeat_at": heartbeatAt,
	}
	if report != nil {
		reportJSON, _ := json.Marshal(report)
		updates["report_json"] = datatypes.JSON(reportJSON)
		updates["stopped"] = report.Stopped
	}
	return h.db.Model(&models.VideoImageSplitBackfillRun{}).
		Where("run_id = ?", runID).
		Where("status = ?", adminVideoImageSplitBackfillRunningStatus).
		Updates(updates).Error
}

func (h *Handler) persistAdminVideoImageSplitBackfillRunFinish(
	runID string,
	status string,
	finishedAt time.Time,
	lastError string,
	report *videojobs.PublicVideoImageSplitBackfillReport,
) error {
	if h == nil || h.db == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	updates := map[string]interface{}{
		"status":       strings.TrimSpace(strings.ToLower(status)),
		"finished_at":  finishedAt,
		"heartbeat_at": finishedAt,
		"last_error":   strings.TrimSpace(lastError),
	}
	if report != nil {
		reportJSON, _ := json.Marshal(report)
		updates["report_json"] = datatypes.JSON(reportJSON)
		updates["stopped"] = report.Stopped
	}
	return h.db.Model(&models.VideoImageSplitBackfillRun{}).
		Where("run_id = ?", runID).
		Where("status = ?", adminVideoImageSplitBackfillRunningStatus).
		Updates(updates).Error
}

func decodeAdminVideoImageSplitBackfillOptions(raw datatypes.JSON) adminVideoImageSplitBackfillOptionsPayload {
	if len(raw) == 0 {
		return adminVideoImageSplitBackfillOptionsPayload{}
	}
	var out adminVideoImageSplitBackfillOptionsPayload
	_ = json.Unmarshal(raw, &out)
	return out
}

func decodeAdminVideoImageSplitBackfillReport(raw datatypes.JSON) *videojobs.PublicVideoImageSplitBackfillReport {
	if len(raw) == 0 {
		return nil
	}
	var out videojobs.PublicVideoImageSplitBackfillReport
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return &out
}

func parseAdminVideoImageSplitBackfillTables(raw string) (jobs, outputs, packages, events, feedbacks bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || raw == "all" || raw == "*" {
		return true, true, true, true, true
	}
	for _, item := range strings.Split(raw, ",") {
		switch strings.TrimSpace(strings.ToLower(item)) {
		case "job", "jobs":
			jobs = true
		case "output", "outputs":
			outputs = true
		case "package", "packages":
			packages = true
		case "event", "events":
			events = true
		case "feedback", "feedbacks":
			feedbacks = true
		}
	}
	return
}
