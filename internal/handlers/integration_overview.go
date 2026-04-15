package handlers

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type IntegrationAccountItem struct {
	ID            uint64    `json:"id"`
	Provider      string    `json:"provider"`
	ProviderLabel string    `json:"provider_label"`
	TenantKey     string    `json:"tenant_key,omitempty"`
	OpenIDMasked  string    `json:"open_id_masked,omitempty"`
	UnionIDMasked string    `json:"union_id_masked,omitempty"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type IntegrationProviderSummaryItem struct {
	Provider      string `json:"provider"`
	ProviderLabel string `json:"provider_label"`
	BoundCount    int64  `json:"bound_count"`
	Active        bool   `json:"active"`
}

type IntegrationCountItem struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type IntegrationRecentIngressItem struct {
	ID              uint64     `json:"id"`
	Provider        string     `json:"provider"`
	ProviderLabel   string     `json:"provider_label"`
	Channel         string     `json:"channel,omitempty"`
	Status          string     `json:"status"`
	VideoJobID      *uint64    `json:"video_job_id,omitempty"`
	SourceFileName  string     `json:"source_file_name,omitempty"`
	SourceVideoKey  string     `json:"source_video_key,omitempty"`
	SourceSizeBytes int64      `json:"source_size_bytes,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type MyIntegrationsOverviewResponse struct {
	Accounts             []IntegrationAccountItem         `json:"accounts"`
	ProviderSummary      []IntegrationProviderSummaryItem `json:"provider_summary"`
	IngressProviderCount []IntegrationCountItem           `json:"ingress_provider_counts"`
	IngressStatusCount   []IntegrationCountItem           `json:"ingress_status_counts"`
	RecentIngress        []IntegrationRecentIngressItem   `json:"recent_ingress"`
}

type AdminVideoIngressCountItem struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type AdminVideoIngressJobItem struct {
	ID              uint64     `json:"id"`
	Provider        string     `json:"provider"`
	ProviderLabel   string     `json:"provider_label"`
	TenantKey       string     `json:"tenant_key,omitempty"`
	Channel         string     `json:"channel,omitempty"`
	Status          string     `json:"status"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	BoundUserID     *uint64    `json:"bound_user_id,omitempty"`
	UserDisplayName string     `json:"user_display_name,omitempty"`
	UserPhone       string     `json:"user_phone,omitempty"`
	ExternalUserID  string     `json:"external_user_id,omitempty"`
	VideoJobID      *uint64    `json:"video_job_id,omitempty"`
	VideoJobStatus  string     `json:"video_job_status,omitempty"`
	SourceFileName  string     `json:"source_file_name,omitempty"`
	SourceVideoKey  string     `json:"source_video_key,omitempty"`
	SourceSizeBytes int64      `json:"source_size_bytes,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type AdminVideoIngressJobListResponse struct {
	Window         string                       `json:"window"`
	WindowStart    *time.Time                   `json:"window_start,omitempty"`
	WindowEnd      time.Time                    `json:"window_end"`
	Total          int64                        `json:"total"`
	Items          []AdminVideoIngressJobItem   `json:"items"`
	ProviderCounts []AdminVideoIngressCountItem `json:"provider_counts"`
	StatusCounts   []AdminVideoIngressCountItem `json:"status_counts"`
}

func integrationProviderLabel(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case models.ExternalAccountProviderFeishu:
		return "飞书"
	case models.ExternalAccountProviderQQ:
		return "QQ"
	case models.ExternalAccountProviderWeCom:
		return "企业微信"
	case models.VideoIngressProviderWeb:
		return "官网Web"
	default:
		return strings.ToUpper(strings.TrimSpace(provider))
	}
}

func maskExternalID(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= 4 {
		return "****"
	}
	prefix := string(runes[:2])
	suffix := string(runes[len(runes)-2:])
	return prefix + "****" + suffix
}

func normalizeIngressProviderFilter(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return ""
	case models.VideoIngressProviderWeb:
		return models.VideoIngressProviderWeb
	case models.VideoIngressProviderFeishu:
		return models.VideoIngressProviderFeishu
	case models.VideoIngressProviderQQ:
		return models.VideoIngressProviderQQ
	case models.VideoIngressProviderWeCom:
		return models.VideoIngressProviderWeCom
	default:
		return ""
	}
}

func normalizeIngressStatusFilter(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return ""
	case models.VideoIngressStatusQueued:
		return models.VideoIngressStatusQueued
	case models.VideoIngressStatusProcessing:
		return models.VideoIngressStatusProcessing
	case models.VideoIngressStatusWaitingBind:
		return models.VideoIngressStatusWaitingBind
	case models.VideoIngressStatusJobQueued:
		return models.VideoIngressStatusJobQueued
	case models.VideoIngressStatusDone:
		return models.VideoIngressStatusDone
	case models.VideoIngressStatusFailed:
		return models.VideoIngressStatusFailed
	default:
		return ""
	}
}

func parseIngressWindow(raw string) (label string, since *time.Time, err error) {
	now := time.Now()
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "7d":
		t := now.Add(-7 * 24 * time.Hour)
		return "7d", &t, nil
	case "24h":
		t := now.Add(-24 * time.Hour)
		return "24h", &t, nil
	case "30d":
		t := now.Add(-30 * 24 * time.Hour)
		return "30d", &t, nil
	case "all":
		return "all", nil, nil
	default:
		return "", nil, errors.New("window must be one of 24h|7d|30d|all")
	}
}

// GetMyIntegrationsOverview godoc
// @Summary Get current user integrations overview
// @Tags user
// @Produce json
// @Success 200 {object} MyIntegrationsOverviewResponse
// @Router /api/me/integrations/overview [get]
func (h *Handler) GetMyIntegrationsOverview(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var accounts []models.ExternalAccount
	if err := h.db.Where("user_id = ?", userID).Order("updated_at DESC").Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	accountItems := make([]IntegrationAccountItem, 0, len(accounts))
	providerMap := map[string]int64{
		models.ExternalAccountProviderFeishu: 0,
		models.ExternalAccountProviderQQ:     0,
		models.ExternalAccountProviderWeCom:  0,
	}
	for _, item := range accounts {
		provider := strings.ToLower(strings.TrimSpace(item.Provider))
		if provider != "" {
			providerMap[provider] += 1
		}
		accountItems = append(accountItems, IntegrationAccountItem{
			ID:            item.ID,
			Provider:      provider,
			ProviderLabel: integrationProviderLabel(provider),
			TenantKey:     strings.TrimSpace(item.TenantKey),
			OpenIDMasked:  maskExternalID(item.OpenID),
			UnionIDMasked: maskExternalID(item.UnionID),
			Status:        strings.TrimSpace(item.Status),
			CreatedAt:     item.CreatedAt,
			UpdatedAt:     item.UpdatedAt,
		})
	}
	providerSummary := make([]IntegrationProviderSummaryItem, 0, 3)
	for _, provider := range []string{models.ExternalAccountProviderFeishu, models.ExternalAccountProviderQQ, models.ExternalAccountProviderWeCom} {
		count := providerMap[provider]
		providerSummary = append(providerSummary, IntegrationProviderSummaryItem{
			Provider:      provider,
			ProviderLabel: integrationProviderLabel(provider),
			BoundCount:    count,
			Active:        count > 0,
		})
	}

	type countRow struct {
		Key   string `gorm:"column:key"`
		Count int64  `gorm:"column:count"`
	}
	var providerRows []countRow
	if err := h.db.Model(&models.VideoIngressJob{}).
		Select("provider AS key, count(*) AS count").
		Where("bound_user_id = ?", userID).
		Group("provider").
		Scan(&providerRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var statusRows []countRow
	if err := h.db.Model(&models.VideoIngressJob{}).
		Select("status AS key, count(*) AS count").
		Where("bound_user_id = ?", userID).
		Group("status").
		Scan(&statusRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var ingressRows []models.VideoIngressJob
	if err := h.db.Where("bound_user_id = ?", userID).Order("id DESC").Limit(20).Find(&ingressRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	recentIngress := make([]IntegrationRecentIngressItem, 0, len(ingressRows))
	for _, item := range ingressRows {
		provider := strings.ToLower(strings.TrimSpace(item.Provider))
		recentIngress = append(recentIngress, IntegrationRecentIngressItem{
			ID:              item.ID,
			Provider:        provider,
			ProviderLabel:   integrationProviderLabel(provider),
			Channel:         strings.TrimSpace(item.Channel),
			Status:          strings.TrimSpace(item.Status),
			VideoJobID:      item.VideoJobID,
			SourceFileName:  strings.TrimSpace(item.SourceFileName),
			SourceVideoKey:  strings.TrimSpace(item.SourceVideoKey),
			SourceSizeBytes: item.SourceSizeBytes,
			ErrorMessage:    strings.TrimSpace(item.ErrorMessage),
			CreatedAt:       item.CreatedAt,
			UpdatedAt:       item.UpdatedAt,
			FinishedAt:      item.FinishedAt,
		})
	}

	providerCounts := make([]IntegrationCountItem, 0, len(providerRows))
	for _, row := range providerRows {
		provider := strings.ToLower(strings.TrimSpace(row.Key))
		providerCounts = append(providerCounts, IntegrationCountItem{
			Key:   provider,
			Count: row.Count,
		})
	}
	sort.Slice(providerCounts, func(i, j int) bool {
		if providerCounts[i].Count == providerCounts[j].Count {
			return providerCounts[i].Key < providerCounts[j].Key
		}
		return providerCounts[i].Count > providerCounts[j].Count
	})

	statusCounts := make([]IntegrationCountItem, 0, len(statusRows))
	for _, row := range statusRows {
		statusCounts = append(statusCounts, IntegrationCountItem{
			Key:   strings.TrimSpace(row.Key),
			Count: row.Count,
		})
	}
	sort.Slice(statusCounts, func(i, j int) bool {
		if statusCounts[i].Count == statusCounts[j].Count {
			return statusCounts[i].Key < statusCounts[j].Key
		}
		return statusCounts[i].Count > statusCounts[j].Count
	})

	c.JSON(http.StatusOK, MyIntegrationsOverviewResponse{
		Accounts:             accountItems,
		ProviderSummary:      providerSummary,
		IngressProviderCount: providerCounts,
		IngressStatusCount:   statusCounts,
		RecentIngress:        recentIngress,
	})
}

// ListAdminVideoIngressJobs godoc
// @Summary List video ingress jobs (admin)
// @Tags admin
// @Produce json
// @Param provider query string false "provider: web|feishu|qq|wecom"
// @Param status query string false "status: queued|processing|waiting_bind|job_queued|done|failed"
// @Param user_id query int false "bound user id"
// @Param window query string false "window: 24h|7d|30d|all"
// @Param limit query int false "limit (default 50, max 200)"
// @Param offset query int false "offset (default 0)"
// @Success 200 {object} AdminVideoIngressJobListResponse
// @Router /api/admin/video-jobs/ingress-jobs [get]
func (h *Handler) ListAdminVideoIngressJobs(c *gin.Context) {
	providerRaw := strings.TrimSpace(c.Query("provider"))
	statusRaw := strings.TrimSpace(c.Query("status"))
	provider := normalizeIngressProviderFilter(providerRaw)
	status := normalizeIngressStatusFilter(statusRaw)
	if providerRaw != "" && strings.ToLower(providerRaw) != "all" && provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider"})
		return
	}
	if statusRaw != "" && strings.ToLower(statusRaw) != "all" && status == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	windowLabel, windowSince, windowErr := parseIngressWindow(c.DefaultQuery("window", "7d"))
	if windowErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid window"})
		return
	}

	userID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("user_id")), 10, 64)
	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "50")))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("offset", "0")))
	if offset < 0 {
		offset = 0
	}

	base := h.db.Table("archive.video_ingress_jobs AS vi")
	if provider != "" {
		base = base.Where("vi.provider = ?", provider)
	}
	if status != "" {
		base = base.Where("vi.status = ?", status)
	}
	if userID > 0 {
		base = base.Where("vi.bound_user_id = ?", userID)
	}
	if windowSince != nil {
		base = base.Where("vi.created_at >= ?", *windowSince)
	}

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type ingressRow struct {
		ID              uint64     `gorm:"column:id"`
		Provider        string     `gorm:"column:provider"`
		TenantKey       string     `gorm:"column:tenant_key"`
		Channel         string     `gorm:"column:channel"`
		Status          string     `gorm:"column:status"`
		ErrorMessage    string     `gorm:"column:error_message"`
		BoundUserID     *uint64    `gorm:"column:bound_user_id"`
		UserDisplayName string     `gorm:"column:user_display_name"`
		UserPhone       string     `gorm:"column:user_phone"`
		ExternalUserID  string     `gorm:"column:external_user_id"`
		VideoJobID      *uint64    `gorm:"column:video_job_id"`
		VideoJobStatus  string     `gorm:"column:video_job_status"`
		SourceFileName  string     `gorm:"column:source_file_name"`
		SourceVideoKey  string     `gorm:"column:source_video_key"`
		SourceSizeBytes int64      `gorm:"column:source_size_bytes"`
		CreatedAt       time.Time  `gorm:"column:created_at"`
		UpdatedAt       time.Time  `gorm:"column:updated_at"`
		FinishedAt      *time.Time `gorm:"column:finished_at"`
	}
	var rows []ingressRow
	if err := base.Session(&gorm.Session{}).
		Select(`
vi.id,
vi.provider,
vi.tenant_key,
vi.channel,
vi.status,
vi.error_message,
vi.bound_user_id,
u.display_name AS user_display_name,
u.phone AS user_phone,
vi.external_user_id,
vi.video_job_id,
vj.status AS video_job_status,
vi.source_file_name,
vi.source_video_key,
vi.source_size_bytes,
vi.created_at,
vi.updated_at,
vi.finished_at
`).
		Joins("LEFT JOIN user.users AS u ON u.id = vi.bound_user_id").
		Joins("LEFT JOIN archive.video_jobs AS vj ON vj.id = vi.video_job_id").
		Order("vi.id DESC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]AdminVideoIngressJobItem, 0, len(rows))
	for _, row := range rows {
		providerKey := strings.ToLower(strings.TrimSpace(row.Provider))
		items = append(items, AdminVideoIngressJobItem{
			ID:              row.ID,
			Provider:        providerKey,
			ProviderLabel:   integrationProviderLabel(providerKey),
			TenantKey:       strings.TrimSpace(row.TenantKey),
			Channel:         strings.TrimSpace(row.Channel),
			Status:          strings.TrimSpace(row.Status),
			ErrorMessage:    strings.TrimSpace(row.ErrorMessage),
			BoundUserID:     row.BoundUserID,
			UserDisplayName: strings.TrimSpace(row.UserDisplayName),
			UserPhone:       strings.TrimSpace(row.UserPhone),
			ExternalUserID:  maskExternalID(row.ExternalUserID),
			VideoJobID:      row.VideoJobID,
			VideoJobStatus:  strings.TrimSpace(row.VideoJobStatus),
			SourceFileName:  strings.TrimSpace(row.SourceFileName),
			SourceVideoKey:  strings.TrimSpace(row.SourceVideoKey),
			SourceSizeBytes: row.SourceSizeBytes,
			CreatedAt:       row.CreatedAt,
			UpdatedAt:       row.UpdatedAt,
			FinishedAt:      row.FinishedAt,
		})
	}

	type countRow struct {
		Key   string `gorm:"column:key"`
		Count int64  `gorm:"column:count"`
	}
	var providerRows []countRow
	if err := base.Session(&gorm.Session{}).
		Select("vi.provider AS key, count(*) AS count").
		Group("vi.provider").
		Scan(&providerRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var statusRows []countRow
	if err := base.Session(&gorm.Session{}).
		Select("vi.status AS key, count(*) AS count").
		Group("vi.status").
		Scan(&statusRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	providerCounts := make([]AdminVideoIngressCountItem, 0, len(providerRows))
	for _, row := range providerRows {
		providerCounts = append(providerCounts, AdminVideoIngressCountItem{
			Key:   strings.TrimSpace(row.Key),
			Count: row.Count,
		})
	}
	sort.Slice(providerCounts, func(i, j int) bool {
		if providerCounts[i].Count == providerCounts[j].Count {
			return providerCounts[i].Key < providerCounts[j].Key
		}
		return providerCounts[i].Count > providerCounts[j].Count
	})

	statusCounts := make([]AdminVideoIngressCountItem, 0, len(statusRows))
	for _, row := range statusRows {
		statusCounts = append(statusCounts, AdminVideoIngressCountItem{
			Key:   strings.TrimSpace(row.Key),
			Count: row.Count,
		})
	}
	sort.Slice(statusCounts, func(i, j int) bool {
		if statusCounts[i].Count == statusCounts[j].Count {
			return statusCounts[i].Key < statusCounts[j].Key
		}
		return statusCounts[i].Count > statusCounts[j].Count
	})

	c.JSON(http.StatusOK, AdminVideoIngressJobListResponse{
		Window:         windowLabel,
		WindowStart:    windowSince,
		WindowEnd:      time.Now(),
		Total:          total,
		Items:          items,
		ProviderCounts: providerCounts,
		StatusCounts:   statusCounts,
	})
}
