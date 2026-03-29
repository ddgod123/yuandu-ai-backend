package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GenerateComputeRedeemCodesRequest struct {
	Count         int        `json:"count"`
	BatchNo       string     `json:"batch_no"`
	GrantedPoints int64      `json:"granted_points"`
	DurationDays  int        `json:"duration_days"`
	MaxUses       int        `json:"max_uses"`
	Prefix        string     `json:"prefix"`
	StartsAt      *time.Time `json:"starts_at"`
	EndsAt        *time.Time `json:"ends_at"`
	Note          string     `json:"note"`
}

type GenerateComputeRedeemCodesResponse struct {
	BatchNo       string     `json:"batch_no"`
	Count         int        `json:"count"`
	GrantedPoints int64      `json:"granted_points"`
	DurationDays  int        `json:"duration_days"`
	MaxUses       int        `json:"max_uses"`
	StartsAt      *time.Time `json:"starts_at,omitempty"`
	EndsAt        *time.Time `json:"ends_at,omitempty"`
	Codes         []string   `json:"codes"`
}

type ComputeRedeemCodeAdminItem struct {
	ID            uint64     `json:"id"`
	CodeMask      string     `json:"code_mask"`
	CodePlain     string     `json:"code_plain,omitempty"`
	BatchNo       string     `json:"batch_no"`
	GrantedPoints int64      `json:"granted_points"`
	DurationDays  int        `json:"duration_days"`
	MaxUses       int        `json:"max_uses"`
	UsedCount     int        `json:"used_count"`
	Status        string     `json:"status"`
	StartsAt      *time.Time `json:"starts_at,omitempty"`
	EndsAt        *time.Time `json:"ends_at,omitempty"`
	Note          string     `json:"note"`
	LastIssuedAt  *time.Time `json:"last_issued_at,omitempty"`
	LastIssuedUID *uint64    `json:"last_issued_uid,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type ComputeRedeemCodeListResponse struct {
	Items []ComputeRedeemCodeAdminItem `json:"items"`
	Total int64                        `json:"total"`
}

type UpdateComputeRedeemCodeStatusRequest struct {
	Status string `json:"status"`
}

type ComputeRedeemCodeSubmitRequest struct {
	Code string `json:"code"`
}

type ComputeRedeemCodeValidateResponse struct {
	Valid         bool       `json:"valid"`
	Message       string     `json:"message"`
	CodeMask      string     `json:"code_mask,omitempty"`
	GrantedPoints int64      `json:"granted_points,omitempty"`
	DurationDays  int        `json:"duration_days,omitempty"`
	StartsAt      *time.Time `json:"starts_at,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	Status        string     `json:"status,omitempty"`
}

type ComputeRedeemCodeSubmitResponse struct {
	Message       string                 `json:"message"`
	CodeMask      string                 `json:"code_mask"`
	GrantedPoints int64                  `json:"granted_points"`
	DurationDays  int                    `json:"duration_days"`
	StartsAt      *time.Time             `json:"starts_at,omitempty"`
	ExpiresAt     *time.Time             `json:"expires_at,omitempty"`
	UsedCount     int                    `json:"used_count"`
	MaxUses       int                    `json:"max_uses"`
	Account       ComputeAccountResponse `json:"account"`
}

type ComputeRedeemRedemptionRecordResponse struct {
	ID               uint64     `json:"id"`
	CodeID           uint64     `json:"code_id"`
	CodeMask         string     `json:"code_mask"`
	UserID           uint64     `json:"user_id"`
	UserDisplayName  string     `json:"user_display_name,omitempty"`
	UserPhone        string     `json:"user_phone,omitempty"`
	GrantedPoints    int64      `json:"granted_points"`
	GrantedStartsAt  time.Time  `json:"granted_starts_at"`
	GrantedExpiresAt *time.Time `json:"granted_expires_at,omitempty"`
	IP               string     `json:"ip,omitempty"`
	UserAgent        string     `json:"user_agent,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type ComputeRedeemRedemptionRecordListResponse struct {
	Items []ComputeRedeemRedemptionRecordResponse `json:"items"`
	Total int64                                   `json:"total"`
}

type computeRedeemRedemptionRecordRow struct {
	ID               uint64
	CodeID           uint64
	CodeMask         string
	UserID           uint64
	UserDisplayName  string
	UserPhone        string
	GrantedPoints    int64
	GrantedStartsAt  time.Time
	GrantedExpiresAt *time.Time
	IP               string
	UserAgent        string
	CreatedAt        time.Time
}

func (h *Handler) buildUniqueComputeRedeemCode(tx *gorm.DB, prefix string, localSet map[string]struct{}) (display string, hash string, err error) {
	for i := 0; i < 20; i++ {
		codeDisplay, canonical, genErr := buildRedeemCode(prefix)
		if genErr != nil {
			return "", "", genErr
		}
		sum := hashRedeemCode(canonical)
		if _, exists := localSet[sum]; exists {
			continue
		}
		var exists int64
		if err := tx.Model(&models.ComputeRedeemCode{}).Where("code_hash = ?", sum).Count(&exists).Error; err != nil {
			return "", "", err
		}
		if exists > 0 {
			continue
		}
		localSet[sum] = struct{}{}
		return codeDisplay, sum, nil
	}
	return "", "", errors.New("failed to generate unique compute redeem code")
}

func evaluateComputeRedeemCodeForUser(db *gorm.DB, userID uint64, codeHash string, now time.Time) ComputeRedeemCodeValidateResponse {
	resp := ComputeRedeemCodeValidateResponse{
		Valid:   false,
		Message: "兑换码无效",
	}

	var code models.ComputeRedeemCode
	if err := db.Where("code_hash = ?", codeHash).First(&code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return resp
		}
		resp.Message = "暂时无法验证，请稍后重试"
		return resp
	}

	resp.CodeMask = code.CodeMask
	resp.GrantedPoints = code.GrantedPoints
	resp.DurationDays = code.DurationDays
	resp.Status = strings.ToLower(strings.TrimSpace(code.Status))

	if resp.Status != "active" {
		resp.Message = "兑换码不可用"
		return resp
	}
	if code.StartsAt != nil && now.Before(*code.StartsAt) {
		resp.Message = "兑换码尚未到生效时间"
		return resp
	}
	if code.EndsAt != nil && now.After(*code.EndsAt) {
		resp.Message = "兑换码已过期"
		resp.Status = "expired"
		return resp
	}
	if code.MaxUses > 0 && code.UsedCount >= code.MaxUses {
		resp.Message = "兑换码已被使用完"
		resp.Status = "expired"
		return resp
	}
	if code.GrantedPoints <= 0 {
		resp.Message = "兑换码额度异常"
		return resp
	}

	var already int64
	if err := db.Model(&models.ComputeRedeemRedemption{}).
		Where("code_id = ? AND user_id = ?", code.ID, userID).
		Count(&already).Error; err != nil {
		resp.Message = "暂时无法验证，请稍后重试"
		return resp
	}
	if already > 0 {
		resp.Message = "你已使用过该兑换码"
		return resp
	}

	var user models.User
	if err := db.Select("id", "status").First(&user, userID).Error; err != nil {
		resp.Message = "用户不存在"
		return resp
	}
	if strings.ToLower(strings.TrimSpace(user.Status)) != "active" {
		resp.Message = "账号状态异常，无法兑换"
		return resp
	}

	grantStartsAt := now
	resp.StartsAt = &grantStartsAt
	if code.DurationDays > 0 {
		grantExpiresAt := grantStartsAt.AddDate(0, 0, code.DurationDays)
		resp.ExpiresAt = &grantExpiresAt
	}
	resp.Valid = true
	resp.Message = "兑换码可用，可立即兑换"
	return resp
}

func (h *Handler) queryComputeRedeemRecords(base *gorm.DB, page, pageSize int) ([]ComputeRedeemRedemptionRecordResponse, int64, error) {
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := base
	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}

	var rows []computeRedeemRedemptionRecordRow
	err := query.
		Order("r.id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]ComputeRedeemRedemptionRecordResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, ComputeRedeemRedemptionRecordResponse{
			ID:               row.ID,
			CodeID:           row.CodeID,
			CodeMask:         row.CodeMask,
			UserID:           row.UserID,
			UserDisplayName:  row.UserDisplayName,
			UserPhone:        row.UserPhone,
			GrantedPoints:    row.GrantedPoints,
			GrantedStartsAt:  row.GrantedStartsAt,
			GrantedExpiresAt: row.GrantedExpiresAt,
			IP:               row.IP,
			UserAgent:        row.UserAgent,
			CreatedAt:        row.CreatedAt,
		})
	}

	return items, total, nil
}

func computeRedeemRecordBaseQuery(db *gorm.DB) *gorm.DB {
	return db.Table("ops.compute_redeem_redemptions AS r").
		Select(`
r.id,
r.code_id,
COALESCE(c.code_mask, '') AS code_mask,
r.user_id,
COALESCE(u.display_name, '') AS user_display_name,
COALESCE(u.phone, '') AS user_phone,
r.granted_points,
r.granted_starts_at,
r.granted_expires_at,
r.ip,
r.user_agent,
r.created_at
		`).
		Joins("LEFT JOIN ops.compute_redeem_codes c ON c.id = r.code_id").
		Joins(`LEFT JOIN "user".users u ON u.id = r.user_id`)
}

// GenerateComputeRedeemCodes godoc
// @Summary Generate compute redeem codes
// @Tags admin
// @Accept json
// @Produce json
// @Param body body GenerateComputeRedeemCodesRequest true "generate request"
// @Success 200 {object} GenerateComputeRedeemCodesResponse
// @Router /api/admin/compute-redeem-codes/generate [post]
func (h *Handler) GenerateComputeRedeemCodes(c *gin.Context) {
	var req GenerateComputeRedeemCodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Count <= 0 || req.Count > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "count must be between 1 and 500"})
		return
	}
	if req.GrantedPoints <= 0 || req.GrantedPoints > 1_000_000_000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "granted_points must be between 1 and 1000000000"})
		return
	}
	if req.DurationDays < 0 || req.DurationDays > 3650 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "duration_days must be between 0 and 3650"})
		return
	}
	if req.MaxUses <= 0 || req.MaxUses > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_uses must be between 1 and 10000"})
		return
	}
	if req.StartsAt != nil && req.EndsAt != nil && req.EndsAt.Before(*req.StartsAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ends_at must be after starts_at"})
		return
	}

	batchNo := strings.TrimSpace(req.BatchNo)
	if batchNo == "" {
		batchNo = "COMPUTE-" + time.Now().Format("20060102150405")
	}
	prefix := sanitizeRedeemCodePrefix(req.Prefix)
	adminUID := h.extractUserID(c)
	var createdBy *uint64
	if adminUID > 0 {
		createdBy = &adminUID
	}

	codesPlain := make([]string, 0, req.Count)
	rows := make([]models.ComputeRedeemCode, 0, req.Count)
	usedHashes := make(map[string]struct{}, req.Count)

	err := h.db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < req.Count; i++ {
			display, sum, err := h.buildUniqueComputeRedeemCode(tx, prefix, usedHashes)
			if err != nil {
				return err
			}
			codesPlain = append(codesPlain, display)
			rows = append(rows, models.ComputeRedeemCode{
				CodeHash:      sum,
				CodePlain:     display,
				CodeMask:      maskRedeemCode(display),
				BatchNo:       batchNo,
				GrantedPoints: req.GrantedPoints,
				DurationDays:  req.DurationDays,
				MaxUses:       req.MaxUses,
				UsedCount:     0,
				Status:        "active",
				StartsAt:      req.StartsAt,
				EndsAt:        req.EndsAt,
				CreatedBy:     createdBy,
				Note:          strings.TrimSpace(req.Note),
			})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, GenerateComputeRedeemCodesResponse{
		BatchNo:       batchNo,
		Count:         req.Count,
		GrantedPoints: req.GrantedPoints,
		DurationDays:  req.DurationDays,
		MaxUses:       req.MaxUses,
		StartsAt:      req.StartsAt,
		EndsAt:        req.EndsAt,
		Codes:         codesPlain,
	})
}

// ListComputeRedeemCodes godoc
// @Summary List compute redeem codes
// @Tags admin
// @Produce json
// @Param q query string false "keyword"
// @Param status query string false "status"
// @Param batch_no query string false "batch"
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} ComputeRedeemCodeListResponse
// @Router /api/admin/compute-redeem-codes [get]
func (h *Handler) ListComputeRedeemCodes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}

	q := strings.TrimSpace(c.Query("q"))
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	batchNo := strings.TrimSpace(c.Query("batch_no"))

	query := h.db.Model(&models.ComputeRedeemCode{})
	if q != "" {
		query = query.Where("code_mask ILIKE ? OR code_plain ILIKE ? OR note ILIKE ? OR batch_no ILIKE ?", "%"+q+"%", "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if batchNo != "" {
		query = query.Where("batch_no = ?", batchNo)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []models.ComputeRedeemCode
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]ComputeRedeemCodeAdminItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, ComputeRedeemCodeAdminItem{
			ID:            row.ID,
			CodeMask:      row.CodeMask,
			CodePlain:     strings.TrimSpace(row.CodePlain),
			BatchNo:       row.BatchNo,
			GrantedPoints: row.GrantedPoints,
			DurationDays:  row.DurationDays,
			MaxUses:       row.MaxUses,
			UsedCount:     row.UsedCount,
			Status:        row.Status,
			StartsAt:      row.StartsAt,
			EndsAt:        row.EndsAt,
			Note:          row.Note,
			LastIssuedAt:  row.LastIssuedAt,
			LastIssuedUID: row.LastIssuedUID,
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, ComputeRedeemCodeListResponse{Items: items, Total: total})
}

// UpdateComputeRedeemCodeStatus godoc
// @Summary Update compute redeem code status
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "code id"
// @Param body body UpdateComputeRedeemCodeStatusRequest true "status"
// @Success 200 {object} ComputeRedeemCodeAdminItem
// @Router /api/admin/compute-redeem-codes/{id}/status [put]
func (h *Handler) UpdateComputeRedeemCodeStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req UpdateComputeRedeemCodeStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != "active" && status != "disabled" && status != "expired" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	var row models.ComputeRedeemCode
	if err := h.db.First(&row, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "compute redeem code not found"})
		return
	}
	row.Status = status
	if err := h.db.Save(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ComputeRedeemCodeAdminItem{
		ID:            row.ID,
		CodeMask:      row.CodeMask,
		CodePlain:     strings.TrimSpace(row.CodePlain),
		BatchNo:       row.BatchNo,
		GrantedPoints: row.GrantedPoints,
		DurationDays:  row.DurationDays,
		MaxUses:       row.MaxUses,
		UsedCount:     row.UsedCount,
		Status:        row.Status,
		StartsAt:      row.StartsAt,
		EndsAt:        row.EndsAt,
		Note:          row.Note,
		LastIssuedAt:  row.LastIssuedAt,
		LastIssuedUID: row.LastIssuedUID,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	})
}

// ListComputeRedeemCodeRedemptions godoc
// @Summary List redemption records by compute redeem code
// @Tags admin
// @Produce json
// @Param id path int true "code id"
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} ComputeRedeemRedemptionRecordListResponse
// @Router /api/admin/compute-redeem-codes/{id}/redemptions [get]
func (h *Handler) ListComputeRedeemCodeRedemptions(c *gin.Context) {
	codeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || codeID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}

	base := computeRedeemRecordBaseQuery(h.db).Where("r.code_id = ?", codeID)
	items, total, err := h.queryComputeRedeemRecords(base, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ComputeRedeemRedemptionRecordListResponse{Items: items, Total: total})
}

// ValidateComputeRedeemCodeForMe godoc
// @Summary Validate compute redeem code availability
// @Tags auth
// @Accept json
// @Produce json
// @Param body body ComputeRedeemCodeSubmitRequest true "redeem code"
// @Success 200 {object} ComputeRedeemCodeValidateResponse
// @Router /api/me/compute-redeem-code/validate [post]
func (h *Handler) ValidateComputeRedeemCodeForMe(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	deviceID := sanitizeDeviceID(c.GetHeader("X-Device-ID"), c.ClientIP(), c.GetHeader("User-Agent"))
	if !h.enforceRiskBlock(c, "redeem", "", deviceID, userID) {
		return
	}
	if !h.guardRedeemValidate(c, userID) {
		return
	}

	var req ComputeRedeemCodeSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	canonical := normalizeRedeemCode(req.Code)
	if canonical == "" {
		h.recordRiskEvent(c, "compute_redeem_validate_invalid_payload", "redeem", "user", strconv.FormatUint(userID, 10), "low", "compute redeem validate missing code", nil)
		c.JSON(http.StatusBadRequest, gin.H{"error": "code required"})
		return
	}
	codeHash := hashRedeemCode(canonical)
	resp := evaluateComputeRedeemCodeForUser(h.db, userID, codeHash, time.Now())
	if !resp.Valid {
		h.recordRiskEvent(c, "compute_redeem_validate_rejected", "redeem", "user", strconv.FormatUint(userID, 10), "low", resp.Message, map[string]interface{}{
			"code_mask": resp.CodeMask,
			"status":    resp.Status,
		})
	}
	c.JSON(http.StatusOK, resp)
}

// RedeemComputeCodeForMe godoc
// @Summary Redeem compute points code
// @Tags auth
// @Accept json
// @Produce json
// @Param body body ComputeRedeemCodeSubmitRequest true "redeem code"
// @Success 200 {object} ComputeRedeemCodeSubmitResponse
// @Router /api/me/compute-redeem-code/redeem [post]
func (h *Handler) RedeemComputeCodeForMe(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	deviceID := sanitizeDeviceID(c.GetHeader("X-Device-ID"), c.ClientIP(), c.GetHeader("User-Agent"))
	if !h.enforceRiskBlock(c, "redeem", "", deviceID, userID) {
		return
	}
	if !h.guardRedeemSubmit(c, userID) {
		return
	}

	var req ComputeRedeemCodeSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	canonical := normalizeRedeemCode(req.Code)
	if canonical == "" {
		h.recordRiskEvent(c, "compute_redeem_submit_invalid_payload", "redeem", "user", strconv.FormatUint(userID, 10), "medium", "compute redeem submit missing code", nil)
		c.JSON(http.StatusBadRequest, gin.H{"error": "code required"})
		return
	}
	codeHash := hashRedeemCode(canonical)
	now := time.Now()

	var finalCode models.ComputeRedeemCode
	var finalAccount models.ComputeAccount
	var finalGrantStartsAt *time.Time
	var finalGrantExpiresAt *time.Time

	err := h.db.Transaction(func(tx *gorm.DB) error {
		var code models.ComputeRedeemCode
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("code_hash = ?", codeHash).First(&code).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("兑换码无效")
			}
			return err
		}
		status := strings.ToLower(strings.TrimSpace(code.Status))
		if status != "active" {
			return fmt.Errorf("兑换码不可用")
		}
		if code.StartsAt != nil && now.Before(*code.StartsAt) {
			return fmt.Errorf("兑换码尚未到生效时间")
		}
		if code.EndsAt != nil && now.After(*code.EndsAt) {
			_ = tx.Model(&models.ComputeRedeemCode{}).Where("id = ?", code.ID).Update("status", "expired").Error
			return fmt.Errorf("兑换码已过期")
		}
		if code.MaxUses > 0 && code.UsedCount >= code.MaxUses {
			return fmt.Errorf("兑换码已被使用完")
		}
		if code.GrantedPoints <= 0 {
			return fmt.Errorf("兑换码额度异常")
		}

		var already int64
		if err := tx.Model(&models.ComputeRedeemRedemption{}).
			Where("code_id = ? AND user_id = ?", code.ID, userID).
			Count(&already).Error; err != nil {
			return err
		}
		if already > 0 {
			return fmt.Errorf("你已使用过该兑换码")
		}

		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("用户不存在")
			}
			return err
		}
		if strings.ToLower(strings.TrimSpace(user.Status)) != "active" {
			return fmt.Errorf("账号状态异常，无法兑换")
		}

		grantStartsAt := now
		var grantExpiresAt *time.Time
		if code.DurationDays > 0 {
			expires := grantStartsAt.AddDate(0, 0, code.DurationDays)
			grantExpiresAt = &expires
		}
		finalGrantStartsAt = &grantStartsAt
		finalGrantExpiresAt = grantExpiresAt
		clearStatus := videojobs.NormalizeComputeRedeemClearStatus("", grantExpiresAt)

		redemption := models.ComputeRedeemRedemption{
			CodeID:           code.ID,
			UserID:           user.ID,
			GrantedPoints:    code.GrantedPoints,
			GrantedStartsAt:  grantStartsAt,
			GrantedExpiresAt: grantExpiresAt,
			ClearStatus:      clearStatus,
			IP:               strings.TrimSpace(c.ClientIP()),
			UserAgent:        strings.TrimSpace(c.GetHeader("User-Agent")),
		}
		if err := tx.Create(&redemption).Error; err != nil {
			return err
		}

		if err := videojobs.AdjustComputePointsTx(tx, user.ID, code.GrantedPoints, "compute_redeem_code", map[string]interface{}{
			"source":        "compute_redeem_code",
			"code_id":       code.ID,
			"code_mask":     code.CodeMask,
			"batch_no":      strings.TrimSpace(code.BatchNo),
			"duration_days": code.DurationDays,
		}); err != nil {
			return err
		}

		codeUpdates := map[string]interface{}{
			"used_count":      gorm.Expr("used_count + 1"),
			"last_issued_at":  now,
			"last_issued_uid": user.ID,
		}
		if code.MaxUses > 0 && code.UsedCount+1 >= code.MaxUses {
			codeUpdates["status"] = "expired"
		}
		if err := tx.Model(&models.ComputeRedeemCode{}).Where("id = ?", code.ID).Updates(codeUpdates).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", user.ID).First(&finalAccount).Error; err != nil {
			return err
		}
		if err := tx.First(&finalCode, code.ID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		h.recordRiskEvent(c, "compute_redeem_submit_rejected", "redeem", "user", strconv.FormatUint(userID, 10), "medium", err.Error(), nil)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.recordRiskEvent(c, "compute_redeem_submit_success", "redeem", "user", strconv.FormatUint(userID, 10), "info", "compute redeem code accepted", map[string]interface{}{
		"granted_points": finalCode.GrantedPoints,
		"used_count":     finalCode.UsedCount,
		"max_uses":       finalCode.MaxUses,
		"code_mask":      finalCode.CodeMask,
	})

	c.JSON(http.StatusOK, ComputeRedeemCodeSubmitResponse{
		Message:       "兑换成功，算力点已到账",
		CodeMask:      finalCode.CodeMask,
		GrantedPoints: finalCode.GrantedPoints,
		DurationDays:  finalCode.DurationDays,
		StartsAt:      finalGrantStartsAt,
		ExpiresAt:     finalGrantExpiresAt,
		UsedCount:     finalCode.UsedCount,
		MaxUses:       finalCode.MaxUses,
		Account:       buildComputeAccountResponse(finalAccount),
	})
}

// ListMyComputeRedeemRecords godoc
// @Summary List my compute redeem records
// @Tags auth
// @Produce json
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} ComputeRedeemRedemptionRecordListResponse
// @Router /api/me/compute-redeem-records [get]
func (h *Handler) ListMyComputeRedeemRecords(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}

	base := computeRedeemRecordBaseQuery(h.db).Where("r.user_id = ?", userID)
	items, total, err := h.queryComputeRedeemRecords(base, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ComputeRedeemRedemptionRecordListResponse{Items: items, Total: total})
}
