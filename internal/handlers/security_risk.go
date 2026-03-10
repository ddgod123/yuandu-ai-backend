package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type RiskBlacklistItem struct {
	ID        uint64     `json:"id"`
	Scope     string     `json:"scope"`
	Target    string     `json:"target"`
	Action    string     `json:"action"`
	Status    string     `json:"status"`
	Reason    string     `json:"reason"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedBy *uint64    `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type RiskBlacklistListResponse struct {
	Items []RiskBlacklistItem `json:"items"`
	Total int64               `json:"total"`
}

type RiskBlacklistCreateRequest struct {
	Scope     string     `json:"scope"`
	Target    string     `json:"target"`
	Action    string     `json:"action"`
	Status    string     `json:"status"`
	Reason    string     `json:"reason"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type RiskBlacklistStatusRequest struct {
	Status string `json:"status"`
}

type RiskEventItem struct {
	ID        uint64         `json:"id"`
	EventType string         `json:"event_type"`
	Action    string         `json:"action"`
	Scope     string         `json:"scope"`
	Target    string         `json:"target"`
	Severity  string         `json:"severity"`
	Message   string         `json:"message"`
	Metadata  datatypes.JSON `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

type RiskEventListResponse struct {
	Items []RiskEventItem `json:"items"`
	Total int64           `json:"total"`
}

type SecurityTopIPItem struct {
	IP            string `json:"ip"`
	DownloadCount int64  `json:"download_count"`
}

type SecurityTopBlockedTarget struct {
	Scope      string `json:"scope"`
	Target     string `json:"target"`
	BlockCount int64  `json:"block_count"`
}

type SecurityOverviewResponse struct {
	WindowStartHour      time.Time                  `json:"window_start_hour"`
	WindowStartDay       time.Time                  `json:"window_start_day"`
	EmojiDownloadLast1h  int64                      `json:"emoji_download_last_1h"`
	CollectionDownload1h int64                      `json:"collection_download_last_1h"`
	BlockedEventsLast24h int64                      `json:"blocked_events_last_24h"`
	RateLimitedLast24h   int64                      `json:"rate_limited_last_24h"`
	ActiveBlacklistCount int64                      `json:"active_blacklist_count"`
	TopDownloadIPs       []SecurityTopIPItem        `json:"top_download_ips"`
	TopBlockedTargets    []SecurityTopBlockedTarget `json:"top_blocked_targets"`
}

var allowedRiskScopes = map[string]bool{
	"ip":     true,
	"device": true,
	"user":   true,
	"phone":  true,
}

var allowedRiskActions = map[string]bool{
	"all":      true,
	"sms":      true,
	"auth":     true,
	"download": true,
	"redeem":   true,
}

var allowedRiskStatuses = map[string]bool{
	"active":   true,
	"disabled": true,
}

var autoBlockCandidateEvents = map[string]bool{
	"captcha_invalid":                true,
	"auth_verify_invalid_code":       true,
	"download_ticket_ip_mismatch":    true,
	"download_ticket_ua_mismatch":    true,
	"download_ticket_owner_mismatch": true,
}

func normalizeRiskScope(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if allowedRiskScopes[v] {
		return v
	}
	return ""
}

func normalizeRiskAction(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return "all"
	}
	if allowedRiskActions[v] {
		return v
	}
	return ""
}

func normalizeRiskStatus(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return "active"
	}
	if allowedRiskStatuses[v] {
		return v
	}
	return ""
}

func (h *Handler) mapRiskBlacklistItem(row models.RiskBlacklist) RiskBlacklistItem {
	return RiskBlacklistItem{
		ID:        row.ID,
		Scope:     row.Scope,
		Target:    row.Target,
		Action:    row.Action,
		Status:    row.Status,
		Reason:    row.Reason,
		ExpiresAt: row.ExpiresAt,
		CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func (h *Handler) recordRiskEvent(c *gin.Context, eventType, action, scope, target, severity, message string, metadata map[string]interface{}) {
	if h == nil || h.db == nil {
		return
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if c != nil {
		metadata["path"] = strings.TrimSpace(c.FullPath())
		metadata["method"] = strings.TrimSpace(c.Request.Method)
		if _, ok := metadata["ip"]; !ok {
			metadata["ip"] = strings.TrimSpace(c.ClientIP())
		}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		raw = []byte("{}")
	}
	event := models.RiskEvent{
		EventType: eventType,
		Action:    strings.TrimSpace(action),
		Scope:     strings.TrimSpace(scope),
		Target:    strings.TrimSpace(target),
		Severity:  strings.TrimSpace(severity),
		Message:   strings.TrimSpace(message),
		Metadata:  datatypes.JSON(raw),
	}
	if event.Severity == "" {
		event.Severity = "info"
	}
	if err := h.db.Create(&event).Error; err != nil {
		return
	}
	h.maybeAutoBlacklistFromRiskEvent(c, event)
}

func (h *Handler) maybeAutoBlacklistFromRiskEvent(c *gin.Context, event models.RiskEvent) {
	if h == nil || h.db == nil || c == nil || h.smsLimiter == nil || !h.cfg.RiskAutoBlockEnabled {
		return
	}
	eventType := strings.TrimSpace(event.EventType)
	if !autoBlockCandidateEvents[eventType] {
		return
	}
	scope := normalizeRiskScope(event.Scope)
	target := strings.TrimSpace(event.Target)
	if scope == "" || target == "" {
		return
	}
	action := normalizeRiskAction(event.Action)
	if action == "" {
		action = "all"
	}

	threshold := h.cfg.RiskAutoBlockThreshold
	if threshold < 2 {
		threshold = 2
	}
	windowSec := h.cfg.RiskAutoBlockWindowSeconds
	if windowSec < 60 {
		windowSec = 60
	}
	counterKey := fmt.Sprintf("risk:auto:block:%s:%s:%s:%s", eventType, action, scope, shortHash(target))
	count, err := h.smsLimiter.IncrWithTTL(c.Request.Context(), counterKey, time.Duration(windowSec)*time.Second)
	if err != nil || count < int64(threshold) {
		return
	}

	blk, err := h.findActiveRiskBlacklist(scope, target, action)
	if err != nil || blk != nil {
		return
	}
	var expiresAt *time.Time
	if h.cfg.RiskAutoBlockDurationSec > 0 {
		v := time.Now().Add(time.Duration(h.cfg.RiskAutoBlockDurationSec) * time.Second)
		expiresAt = &v
	}
	row := models.RiskBlacklist{
		Scope:     scope,
		Target:    target,
		Action:    action,
		Status:    "active",
		Reason:    fmt.Sprintf("auto blocked by risk rule: %s", eventType),
		ExpiresAt: expiresAt,
	}
	if err := h.db.Create(&row).Error; err != nil {
		return
	}
	h.recordRiskEvent(c, "blacklist_auto_create", action, scope, target, "high", "auto blacklist created by risk engine", map[string]interface{}{
		"source_event": eventType,
		"blacklist_id": row.ID,
		"threshold":    threshold,
		"count":        count,
	})
}

func (h *Handler) maybeSweepExpiredRiskBlacklists(c *gin.Context) {
	if h == nil || h.db == nil {
		return
	}

	if h.smsLimiter != nil && c != nil {
		ok, err := h.smsLimiter.AllowInterval(c.Request.Context(), "risk:blacklist:sweep", 60*time.Second)
		if err == nil && !ok {
			return
		}
	}

	now := time.Now()
	var rows []models.RiskBlacklist
	if err := h.db.Select("id", "scope", "target", "action").
		Where("status = 'active' AND expires_at IS NOT NULL AND expires_at <= ? AND deleted_at IS NULL", now).
		Limit(200).
		Find(&rows).Error; err != nil || len(rows) == 0 {
		return
	}

	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	if err := h.db.Model(&models.RiskBlacklist{}).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{
			"status":     "disabled",
			"updated_at": now,
		}).Error; err != nil {
		return
	}

	h.recordRiskEvent(c, "blacklist_auto_expire", "all", "system", "", "low", "expired blacklists auto disabled", map[string]interface{}{
		"count": len(ids),
		"ids":   ids,
	})
}

func (h *Handler) findActiveRiskBlacklist(scope, target, action string) (*models.RiskBlacklist, error) {
	scope = normalizeRiskScope(scope)
	target = strings.TrimSpace(target)
	action = normalizeRiskAction(action)
	if scope == "" || target == "" || action == "" {
		return nil, nil
	}
	var row models.RiskBlacklist
	err := h.db.Where(
		"scope = ? AND target = ? AND status = 'active' AND (action = 'all' OR action = ?) AND (expires_at IS NULL OR expires_at > NOW())",
		scope,
		target,
		action,
	).Order("id DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (h *Handler) enforceRiskBlock(c *gin.Context, action, phone, deviceID string, userID uint64) bool {
	h.maybeSweepExpiredRiskBlacklists(c)

	action = normalizeRiskAction(action)
	if action == "" {
		action = "all"
	}
	candidates := []struct {
		scope  string
		target string
	}{
		{scope: "ip", target: strings.TrimSpace(c.ClientIP())},
		{scope: "phone", target: strings.TrimSpace(phone)},
		{scope: "device", target: strings.TrimSpace(deviceID)},
	}
	if userID > 0 {
		candidates = append(candidates, struct {
			scope  string
			target string
		}{scope: "user", target: strconv.FormatUint(userID, 10)})
	}

	for _, candidate := range candidates {
		blk, err := h.findActiveRiskBlacklist(candidate.scope, candidate.target, action)
		if err != nil || blk == nil {
			continue
		}
		h.recordRiskEvent(c, "blacklist_block", action, candidate.scope, candidate.target, "high", "request blocked by blacklist", map[string]interface{}{
			"blacklist_id": blk.ID,
			"reason":       blk.Reason,
		})
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "access_blocked",
			"message": "请求受限，请稍后再试或联系管理员",
		})
		return false
	}

	return true
}

func (h *Handler) GetSecurityOverview(c *gin.Context) {
	h.maybeSweepExpiredRiskBlacklists(c)

	hourStart := time.Now().Add(-1 * time.Hour)
	dayStart := time.Now().Add(-24 * time.Hour)

	var emojiDownloads int64
	_ = h.db.Table("action.downloads").
		Where("created_at >= ?", hourStart).
		Count(&emojiDownloads).Error

	var collectionDownloads int64
	_ = h.db.Table("action.collection_downloads").
		Where("created_at >= ?", hourStart).
		Count(&collectionDownloads).Error

	var activeBlacklistCount int64
	_ = h.db.Model(&models.RiskBlacklist{}).
		Where("status = 'active' AND (expires_at IS NULL OR expires_at > NOW())").
		Count(&activeBlacklistCount).Error

	var blockedEvents int64
	_ = h.db.Model(&models.RiskEvent{}).
		Where("created_at >= ? AND event_type = ?", dayStart, "blacklist_block").
		Count(&blockedEvents).Error

	var rateLimitedEvents int64
	_ = h.db.Model(&models.RiskEvent{}).
		Where("created_at >= ? AND event_type = ?", dayStart, "rate_limited").
		Count(&rateLimitedEvents).Error

	var topIPs []SecurityTopIPItem
	_ = h.db.Raw(`
		SELECT ip, COUNT(*) AS download_count
		FROM (
			SELECT ip FROM action.downloads WHERE created_at >= ?
			UNION ALL
			SELECT ip FROM action.collection_downloads WHERE created_at >= ?
		) t
		WHERE COALESCE(ip, '') <> ''
		GROUP BY ip
		ORDER BY download_count DESC
		LIMIT 10
	`, hourStart, hourStart).Scan(&topIPs).Error

	var topBlocked []SecurityTopBlockedTarget
	_ = h.db.Raw(`
		SELECT scope, target, COUNT(*) AS block_count
		FROM ops.risk_events
		WHERE created_at >= ? AND event_type = 'blacklist_block'
		GROUP BY scope, target
		ORDER BY block_count DESC
		LIMIT 10
	`, dayStart).Scan(&topBlocked).Error

	c.JSON(http.StatusOK, SecurityOverviewResponse{
		WindowStartHour:      hourStart,
		WindowStartDay:       dayStart,
		EmojiDownloadLast1h:  emojiDownloads,
		CollectionDownload1h: collectionDownloads,
		BlockedEventsLast24h: blockedEvents,
		RateLimitedLast24h:   rateLimitedEvents,
		ActiveBlacklistCount: activeBlacklistCount,
		TopDownloadIPs:       topIPs,
		TopBlockedTargets:    topBlocked,
	})
}

func (h *Handler) ListRiskBlacklists(c *gin.Context) {
	h.maybeSweepExpiredRiskBlacklists(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}

	q := strings.TrimSpace(c.Query("q"))
	scope := normalizeRiskScope(c.Query("scope"))
	action := normalizeRiskAction(c.Query("action"))
	status := normalizeRiskStatus(c.Query("status"))
	if strings.TrimSpace(c.Query("scope")) != "" && scope == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	if strings.TrimSpace(c.Query("action")) != "" && action == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action"})
		return
	}
	if strings.TrimSpace(c.Query("status")) != "" && status == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	query := h.db.Model(&models.RiskBlacklist{})
	if q != "" {
		query = query.Where("target ILIKE ? OR reason ILIKE ?", "%"+q+"%", "%"+q+"%")
	}
	if scope != "" {
		query = query.Where("scope = ?", scope)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []models.RiskBlacklist
	if err := query.Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]RiskBlacklistItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, h.mapRiskBlacklistItem(row))
	}
	c.JSON(http.StatusOK, RiskBlacklistListResponse{Items: items, Total: total})
}

func (h *Handler) CreateRiskBlacklist(c *gin.Context) {
	var req RiskBlacklistCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	scope := normalizeRiskScope(req.Scope)
	if scope == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	action := normalizeRiskAction(req.Action)
	if action == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action"})
		return
	}
	status := normalizeRiskStatus(req.Status)
	if status == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}
	target := strings.TrimSpace(req.Target)
	if target == "" || len(target) > 191 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target"})
		return
	}
	if scope == "user" {
		if _, err := strconv.ParseUint(target, 10, 64); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user target must be numeric"})
			return
		}
	}
	if scope == "device" {
		if !deviceIDPattern.MatchString(target) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device id"})
			return
		}
	}
	if req.ExpiresAt != nil && req.ExpiresAt.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expires_at must be in future"})
		return
	}

	adminUID := h.extractUserID(c)
	var createdBy *uint64
	if adminUID > 0 {
		createdBy = &adminUID
	}

	row := models.RiskBlacklist{
		Scope:     scope,
		Target:    target,
		Action:    action,
		Status:    status,
		Reason:    strings.TrimSpace(req.Reason),
		ExpiresAt: req.ExpiresAt,
		CreatedBy: createdBy,
	}
	if err := h.db.Create(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.recordRiskEvent(c, "blacklist_create", action, scope, target, "medium", "risk blacklist created", map[string]interface{}{
		"blacklist_id": row.ID,
	})
	c.JSON(http.StatusOK, h.mapRiskBlacklistItem(row))
}

func (h *Handler) UpdateRiskBlacklistStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req RiskBlacklistStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status := normalizeRiskStatus(req.Status)
	if status == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	var row models.RiskBlacklist
	if err := h.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Model(&row).Updates(map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.db.First(&row, row.ID).Error

	h.recordRiskEvent(c, "blacklist_update_status", row.Action, row.Scope, row.Target, "medium", "risk blacklist status updated", map[string]interface{}{
		"blacklist_id": row.ID,
		"status":       status,
	})
	c.JSON(http.StatusOK, h.mapRiskBlacklistItem(row))
}

func (h *Handler) DeleteRiskBlacklist(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var row models.RiskBlacklist
	if err := h.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Delete(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.recordRiskEvent(c, "blacklist_delete", row.Action, row.Scope, row.Target, "medium", "risk blacklist deleted", map[string]interface{}{
		"blacklist_id": row.ID,
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ListRiskEvents(c *gin.Context) {
	h.maybeSweepExpiredRiskBlacklists(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 30
	}
	days := parseDays(c.Query("days"), 7)
	eventType := strings.TrimSpace(c.Query("event_type"))
	action := strings.ToLower(strings.TrimSpace(c.Query("action")))
	severity := strings.ToLower(strings.TrimSpace(c.Query("severity")))
	q := strings.TrimSpace(c.Query("q"))

	query := h.db.Model(&models.RiskEvent{}).
		Where("created_at >= ?", time.Now().AddDate(0, 0, -(days-1)))
	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	if action != "" && action != "all" {
		query = query.Where("action = ?", action)
	}
	if severity != "" {
		query = query.Where("severity = ?", severity)
	}
	if q != "" {
		query = query.Where("target ILIKE ? OR message ILIKE ?", "%"+q+"%", "%"+q+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []models.RiskEvent
	if err := query.Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]RiskEventItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RiskEventItem{
			ID:        row.ID,
			EventType: row.EventType,
			Action:    row.Action,
			Scope:     row.Scope,
			Target:    row.Target,
			Severity:  row.Severity,
			Message:   row.Message,
			Metadata:  row.Metadata,
			CreatedAt: row.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, RiskEventListResponse{Items: items, Total: total})
}
