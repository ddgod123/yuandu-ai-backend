package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ComputeAccountResponse struct {
	UserID               uint64  `json:"user_id"`
	AvailablePoints      int64   `json:"available_points"`
	FrozenPoints         int64   `json:"frozen_points"`
	DebtPoints           int64   `json:"debt_points"`
	TotalConsumedPoints  int64   `json:"total_consumed_points"`
	TotalRechargedPoints int64   `json:"total_recharged_points"`
	Status               string  `json:"status"`
	PointPerCNY          float64 `json:"point_per_cny"`
}

type ComputeLedgerItem struct {
	ID              uint64                 `json:"id"`
	JobID           *uint64                `json:"job_id,omitempty"`
	Type            string                 `json:"type"`
	Points          int64                  `json:"points"`
	AvailableBefore int64                  `json:"available_before"`
	AvailableAfter  int64                  `json:"available_after"`
	FrozenBefore    int64                  `json:"frozen_before"`
	FrozenAfter     int64                  `json:"frozen_after"`
	DebtBefore      int64                  `json:"debt_before"`
	DebtAfter       int64                  `json:"debt_after"`
	Remark          string                 `json:"remark,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt       string                 `json:"created_at"`
}

type ComputeAccountSummaryResponse struct {
	Account  ComputeAccountResponse `json:"account"`
	Ledgers  []ComputeLedgerItem    `json:"ledgers"`
	HeldJobs int64                  `json:"held_jobs"`
}

type AdminAdjustComputeAccountRequest struct {
	DeltaPoints int64  `json:"delta_points"`
	Reason      string `json:"reason"`
}

type AdminComputeAccountUser struct {
	ID                 uint64 `json:"id"`
	DisplayName        string `json:"display_name,omitempty"`
	Phone              string `json:"phone,omitempty"`
	Role               string `json:"role,omitempty"`
	Status             string `json:"status,omitempty"`
	SubscriptionPlan   string `json:"subscription_plan,omitempty"`
	SubscriptionStatus string `json:"subscription_status,omitempty"`
}

type AdminComputeAccountItem struct {
	User         AdminComputeAccountUser `json:"user"`
	Account      ComputeAccountResponse  `json:"account"`
	HeldJobs     int64                   `json:"held_jobs"`
	LastLedgerAt string                  `json:"last_ledger_at,omitempty"`
}

type AdminComputeAccountListSummary struct {
	AvailablePoints int64 `json:"available_points"`
	FrozenPoints    int64 `json:"frozen_points"`
	DebtPoints      int64 `json:"debt_points"`
}

type AdminComputeAccountListResponse struct {
	Items    []AdminComputeAccountItem      `json:"items"`
	Total    int64                          `json:"total"`
	Page     int                            `json:"page"`
	PageSize int                            `json:"page_size"`
	Summary  AdminComputeAccountListSummary `json:"summary"`
}

type AdminComputePointHoldItem struct {
	ID             uint64 `json:"id"`
	JobID          uint64 `json:"job_id"`
	ReservedPoints int64  `json:"reserved_points"`
	SettledPoints  int64  `json:"settled_points"`
	Status         string `json:"status"`
	Remark         string `json:"remark,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	SettledAt      string `json:"settled_at,omitempty"`
}

type AdminComputeAccountDetailResponse struct {
	User        AdminComputeAccountUser     `json:"user"`
	Account     ComputeAccountResponse      `json:"account"`
	HeldJobs    int64                       `json:"held_jobs"`
	Holds       []AdminComputePointHoldItem `json:"holds"`
	Ledgers     []ComputeLedgerItem         `json:"ledgers"`
	PointPerCNY float64                     `json:"point_per_cny"`
}

func buildComputeAccountResponse(account models.ComputeAccount) ComputeAccountResponse {
	return ComputeAccountResponse{
		UserID:               account.UserID,
		AvailablePoints:      account.AvailablePoints,
		FrozenPoints:         account.FrozenPoints,
		DebtPoints:           account.DebtPoints,
		TotalConsumedPoints:  account.TotalConsumedPoints,
		TotalRechargedPoints: account.TotalRechargedPoints,
		Status:               strings.TrimSpace(account.Status),
		PointPerCNY:          videojobs.PointPerCNY(),
	}
}

type userIDOnlyRow struct {
	ID uint64 `gorm:"column:id"`
}

type holdCountRow struct {
	UserID uint64 `gorm:"column:user_id"`
	Count  int64  `gorm:"column:count"`
}

type ledgerLastRow struct {
	UserID    uint64    `gorm:"column:user_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func loadComputeUserFilters(h *Handler, userIDParam, q string) ([]uint64, error) {
	userIDParam = strings.TrimSpace(userIDParam)
	q = strings.TrimSpace(q)
	if userIDParam == "" && q == "" {
		return nil, nil
	}

	userSet := map[uint64]struct{}{}
	if userIDParam != "" {
		id, err := strconv.ParseUint(userIDParam, 10, 64)
		if err != nil || id == 0 {
			return nil, errors.New("invalid user id")
		}
		userSet[id] = struct{}{}
	}

	if q != "" {
		like := "%" + q + "%"
		query := h.db.Model(&models.User{}).Select("id")
		if _, err := strconv.ParseUint(q, 10, 64); err == nil {
			query = query.Where("CAST(id AS TEXT) = ? OR phone ILIKE ? OR display_name ILIKE ?", q, like, like)
		} else {
			query = query.Where("phone ILIKE ? OR display_name ILIKE ?", like, like)
		}
		var rows []userIDOnlyRow
		if err := query.Limit(500).Find(&rows).Error; err != nil {
			return nil, err
		}
		if userIDParam != "" {
			allowed := map[uint64]struct{}{}
			for _, row := range rows {
				allowed[row.ID] = struct{}{}
			}
			filtered := map[uint64]struct{}{}
			for id := range userSet {
				if _, ok := allowed[id]; ok {
					filtered[id] = struct{}{}
				}
			}
			userSet = filtered
		} else {
			for _, row := range rows {
				userSet[row.ID] = struct{}{}
			}
		}
	}

	if len(userSet) == 0 {
		return []uint64{}, nil
	}
	out := make([]uint64, 0, len(userSet))
	for id := range userSet {
		out = append(out, id)
	}
	return out, nil
}

// ListAdminComputeAccounts godoc
// @Summary List compute accounts (admin)
// @Tags admin
// @Produce json
// @Param page query int false "page"
// @Param page_size query int false "page size (max 100)"
// @Param status query string false "account status: all|active|disabled"
// @Param q query string false "user fuzzy query: uid/phone/display_name"
// @Param user_id query int false "exact user id"
// @Param with_debt query bool false "filter by debt users"
// @Param min_available query int false "available_points >= value"
// @Param max_available query int false "available_points <= value"
// @Param min_frozen query int false "frozen_points >= value"
// @Param min_debt query int false "debt_points >= value"
// @Success 200 {object} AdminComputeAccountListResponse
// @Router /api/admin/compute/accounts [get]
func (h *Handler) ListAdminComputeAccounts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	q := strings.TrimSpace(c.Query("q"))
	filterUserIDs, err := loadComputeUserFilters(h, c.Query("user_id"), q)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user filter"})
		return
	}
	withDebtFilter, err := parseOptionalBoolQuery(c.Query("with_debt"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid with_debt filter"})
		return
	}
	minAvailable, err := parseOptionalInt64Query(c.Query("min_available"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid min_available filter"})
		return
	}
	maxAvailable, err := parseOptionalInt64Query(c.Query("max_available"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid max_available filter"})
		return
	}
	minFrozen, err := parseOptionalInt64Query(c.Query("min_frozen"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid min_frozen filter"})
		return
	}
	minDebt, err := parseOptionalInt64Query(c.Query("min_debt"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid min_debt filter"})
		return
	}
	if minAvailable != nil && maxAvailable != nil && *minAvailable > *maxAvailable {
		c.JSON(http.StatusBadRequest, gin.H{"error": "min_available cannot be greater than max_available"})
		return
	}
	if filterUserIDs != nil && len(filterUserIDs) == 0 {
		c.JSON(http.StatusOK, AdminComputeAccountListResponse{
			Items:    []AdminComputeAccountItem{},
			Total:    0,
			Page:     page,
			PageSize: pageSize,
			Summary:  AdminComputeAccountListSummary{},
		})
		return
	}

	var total int64
	newAccountQuery := func() *gorm.DB {
		query := h.db.Session(&gorm.Session{NewDB: true}).Model(&models.ComputeAccount{})
		if status != "" && status != "all" {
			query = query.Where("status = ?", status)
		}
		if filterUserIDs != nil {
			query = query.Where("user_id IN ?", filterUserIDs)
		}
		if withDebtFilter != nil {
			if *withDebtFilter {
				query = query.Where("debt_points > 0")
			} else {
				query = query.Where("debt_points <= 0")
			}
		}
		if minAvailable != nil {
			query = query.Where("available_points >= ?", *minAvailable)
		}
		if maxAvailable != nil {
			query = query.Where("available_points <= ?", *maxAvailable)
		}
		if minFrozen != nil {
			query = query.Where("frozen_points >= ?", *minFrozen)
		}
		if minDebt != nil {
			query = query.Where("debt_points >= ?", *minDebt)
		}
		return query
	}

	if err := newAccountQuery().Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var summary AdminComputeAccountListSummary
	if err := newAccountQuery().Select(
		"COALESCE(SUM(available_points),0) AS available_points, " +
			"COALESCE(SUM(frozen_points),0) AS frozen_points, " +
			"COALESCE(SUM(debt_points),0) AS debt_points",
	).Scan(&summary).Error; err != nil {
		summary = AdminComputeAccountListSummary{}
	}

	offset := (page - 1) * pageSize
	var accounts []models.ComputeAccount
	if err := newAccountQuery().
		Order("updated_at DESC, id DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	userIDs := make([]uint64, 0, len(accounts))
	for _, item := range accounts {
		if item.UserID > 0 {
			userIDs = append(userIDs, item.UserID)
		}
	}

	userMap := map[uint64]models.User{}
	holdMap := map[uint64]int64{}
	lastLedgerMap := map[uint64]string{}

	if len(userIDs) > 0 {
		var users []models.User
		_ = h.db.Select("id", "phone", "display_name", "role", "status", "subscription_plan", "subscription_status").
			Where("id IN ?", userIDs).
			Find(&users).Error
		for _, item := range users {
			userMap[item.ID] = item
		}

		var holds []holdCountRow
		_ = h.db.Model(&models.ComputePointHold{}).
			Select("user_id, COUNT(*) AS count").
			Where("status = ? AND user_id IN ?", "held", userIDs).
			Group("user_id").
			Scan(&holds).Error
		for _, row := range holds {
			holdMap[row.UserID] = row.Count
		}

		var lastRows []ledgerLastRow
		_ = h.db.Model(&models.ComputeLedger{}).
			Select("user_id, MAX(created_at) AS created_at").
			Where("user_id IN ?", userIDs).
			Group("user_id").
			Scan(&lastRows).Error
		for _, row := range lastRows {
			if row.CreatedAt.IsZero() {
				continue
			}
			lastLedgerMap[row.UserID] = row.CreatedAt.Format("2006-01-02 15:04:05")
		}
	}

	items := make([]AdminComputeAccountItem, 0, len(accounts))
	for _, account := range accounts {
		user := userMap[account.UserID]
		items = append(items, AdminComputeAccountItem{
			User: AdminComputeAccountUser{
				ID:                 account.UserID,
				DisplayName:        strings.TrimSpace(user.DisplayName),
				Phone:              strings.TrimSpace(user.Phone),
				Role:               strings.TrimSpace(user.Role),
				Status:             strings.TrimSpace(user.Status),
				SubscriptionPlan:   strings.TrimSpace(user.SubscriptionPlan),
				SubscriptionStatus: strings.TrimSpace(user.SubscriptionStatus),
			},
			Account:      buildComputeAccountResponse(account),
			HeldJobs:     holdMap[account.UserID],
			LastLedgerAt: lastLedgerMap[account.UserID],
		})
	}

	c.JSON(http.StatusOK, AdminComputeAccountListResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Summary:  summary,
	})
}

// GetAdminComputeAccount godoc
// @Summary Get compute account detail by user id (admin)
// @Tags admin
// @Produce json
// @Param id path int true "user id"
// @Param limit query int false "ledger limit (max 300)"
// @Success 200 {object} AdminComputeAccountDetailResponse
// @Router /api/admin/compute/accounts/{id} [get]
func (h *Handler) GetAdminComputeAccount(c *gin.Context) {
	rawUserID := strings.TrimSpace(c.Param("id"))
	userID, err := strconv.ParseUint(rawUserID, 10, 64)
	if err != nil || userID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 300 {
		limit = 300
	}

	var account models.ComputeAccount
	if err := h.db.Where("user_id = ?", userID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "compute account not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	_ = h.db.Select("id", "phone", "display_name", "role", "status", "subscription_plan", "subscription_status").
		Where("id = ?", userID).
		First(&user).Error

	var ledgers []models.ComputeLedger
	if err := h.db.Where("user_id = ?", userID).Order("id DESC").Limit(limit).Find(&ledgers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	respLedgers := make([]ComputeLedgerItem, 0, len(ledgers))
	for _, row := range ledgers {
		respLedgers = append(respLedgers, ComputeLedgerItem{
			ID:              row.ID,
			JobID:           row.JobID,
			Type:            strings.TrimSpace(row.Type),
			Points:          row.Points,
			AvailableBefore: row.AvailableBefore,
			AvailableAfter:  row.AvailableAfter,
			FrozenBefore:    row.FrozenBefore,
			FrozenAfter:     row.FrozenAfter,
			DebtBefore:      row.DebtBefore,
			DebtAfter:       row.DebtAfter,
			Remark:          strings.TrimSpace(row.Remark),
			Metadata:        parseJSONMap(row.Metadata),
			CreatedAt:       row.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var holds []models.ComputePointHold
	if err := h.db.Where("user_id = ?", userID).Order("id DESC").Limit(50).Find(&holds).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	respHolds := make([]AdminComputePointHoldItem, 0, len(holds))
	var heldJobs int64
	for _, item := range holds {
		if strings.EqualFold(strings.TrimSpace(item.Status), "held") {
			heldJobs++
		}
		resp := AdminComputePointHoldItem{
			ID:             item.ID,
			JobID:          item.JobID,
			ReservedPoints: item.ReservedPoints,
			SettledPoints:  item.SettledPoints,
			Status:         strings.TrimSpace(item.Status),
			Remark:         strings.TrimSpace(item.Remark),
			CreatedAt:      item.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:      item.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
		if item.SettledAt != nil && !item.SettledAt.IsZero() {
			resp.SettledAt = item.SettledAt.Format("2006-01-02 15:04:05")
		}
		respHolds = append(respHolds, resp)
	}

	c.JSON(http.StatusOK, AdminComputeAccountDetailResponse{
		User: AdminComputeAccountUser{
			ID:                 userID,
			DisplayName:        strings.TrimSpace(user.DisplayName),
			Phone:              strings.TrimSpace(user.Phone),
			Role:               strings.TrimSpace(user.Role),
			Status:             strings.TrimSpace(user.Status),
			SubscriptionPlan:   strings.TrimSpace(user.SubscriptionPlan),
			SubscriptionStatus: strings.TrimSpace(user.SubscriptionStatus),
		},
		Account:     buildComputeAccountResponse(account),
		HeldJobs:    heldJobs,
		Holds:       respHolds,
		Ledgers:     respLedgers,
		PointPerCNY: videojobs.PointPerCNY(),
	})
}

// GetMyComputeAccount godoc
// @Summary Get current user compute account
// @Tags user
// @Produce json
// @Param limit query int false "ledger limit (max 100)"
// @Success 200 {object} ComputeAccountSummaryResponse
// @Router /api/me/compute-account [get]
func (h *Handler) GetMyComputeAccount(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	account, err := videojobs.EnsureComputeAccount(h.db, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var rows []models.ComputeLedger
	if err := h.db.Where("user_id = ?", userID).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ledgers := make([]ComputeLedgerItem, 0, len(rows))
	for _, row := range rows {
		ledgers = append(ledgers, ComputeLedgerItem{
			ID:              row.ID,
			JobID:           row.JobID,
			Type:            strings.TrimSpace(row.Type),
			Points:          row.Points,
			AvailableBefore: row.AvailableBefore,
			AvailableAfter:  row.AvailableAfter,
			FrozenBefore:    row.FrozenBefore,
			FrozenAfter:     row.FrozenAfter,
			DebtBefore:      row.DebtBefore,
			DebtAfter:       row.DebtAfter,
			Remark:          strings.TrimSpace(row.Remark),
			Metadata:        parseJSONMap(row.Metadata),
			CreatedAt:       row.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	var heldCount int64
	_ = h.db.Model(&models.ComputePointHold{}).
		Where("user_id = ? AND status = ?", userID, "held").
		Count(&heldCount).Error

	c.JSON(http.StatusOK, ComputeAccountSummaryResponse{
		Account: ComputeAccountResponse{
			UserID:               account.UserID,
			AvailablePoints:      account.AvailablePoints,
			FrozenPoints:         account.FrozenPoints,
			DebtPoints:           account.DebtPoints,
			TotalConsumedPoints:  account.TotalConsumedPoints,
			TotalRechargedPoints: account.TotalRechargedPoints,
			Status:               strings.TrimSpace(account.Status),
			PointPerCNY:          videojobs.PointPerCNY(),
		},
		Ledgers:  ledgers,
		HeldJobs: heldCount,
	})
}

// AdminAdjustComputeAccount godoc
// @Summary Adjust compute points for user (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "user id"
// @Param body body AdminAdjustComputeAccountRequest true "delta points and reason"
// @Success 200 {object} ComputeAccountResponse
// @Router /api/admin/compute/accounts/{id}/adjust [post]
func (h *Handler) AdminAdjustComputeAccount(c *gin.Context) {
	rawUserID := strings.TrimSpace(c.Param("id"))
	userID, err := strconv.ParseUint(rawUserID, 10, 64)
	if err != nil || userID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req AdminAdjustComputeAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.DeltaPoints == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "delta_points cannot be 0"})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "admin_adjust"
	}

	adminID, _ := currentUserIDFromContext(c)
	if err := videojobs.AdjustComputePoints(h.db, userID, req.DeltaPoints, reason, map[string]interface{}{
		"admin_id": adminID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	account, err := videojobs.EnsureComputeAccount(h.db, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ComputeAccountResponse{
		UserID:               account.UserID,
		AvailablePoints:      account.AvailablePoints,
		FrozenPoints:         account.FrozenPoints,
		DebtPoints:           account.DebtPoints,
		TotalConsumedPoints:  account.TotalConsumedPoints,
		TotalRechargedPoints: account.TotalRechargedPoints,
		Status:               strings.TrimSpace(account.Status),
		PointPerCNY:          videojobs.PointPerCNY(),
	})
}

func parseOptionalInt64Query(raw string) (*int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func parseOptionalBoolQuery(raw string) (*bool, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return nil, nil
	}
	switch value {
	case "1", "true", "t", "yes", "y", "on":
		v := true
		return &v, nil
	case "0", "false", "f", "no", "n", "off":
		v := false
		return &v, nil
	default:
		return nil, errors.New("invalid bool value")
	}
}
