package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const redeemAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

type GenerateRedeemCodesRequest struct {
	Count        int        `json:"count"`
	BatchNo      string     `json:"batch_no"`
	Plan         string     `json:"plan"`
	DurationDays int        `json:"duration_days"`
	MaxUses      int        `json:"max_uses"`
	Prefix       string     `json:"prefix"`
	StartsAt     *time.Time `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	Note         string     `json:"note"`
}

type GenerateRedeemCodesResponse struct {
	BatchNo      string     `json:"batch_no"`
	Count        int        `json:"count"`
	Plan         string     `json:"plan"`
	DurationDays int        `json:"duration_days"`
	MaxUses      int        `json:"max_uses"`
	StartsAt     *time.Time `json:"starts_at,omitempty"`
	EndsAt       *time.Time `json:"ends_at,omitempty"`
	Codes        []string   `json:"codes"`
}

type RedeemCodeAdminItem struct {
	ID            uint64     `json:"id"`
	CodeMask      string     `json:"code_mask"`
	BatchNo       string     `json:"batch_no"`
	Plan          string     `json:"plan"`
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

type RedeemCodeListResponse struct {
	Items []RedeemCodeAdminItem `json:"items"`
	Total int64                 `json:"total"`
}

type UpdateRedeemCodeStatusRequest struct {
	Status string `json:"status"`
}

type RedeemCodeSubmitRequest struct {
	Code string `json:"code"`
}

type RedeemCodeSubmitResponse struct {
	Message      string       `json:"message"`
	User         UserResponse `json:"user"`
	Plan         string       `json:"plan"`
	ExpiresAt    *time.Time   `json:"expires_at,omitempty"`
	UsedCount    int          `json:"used_count"`
	MaxUses      int          `json:"max_uses"`
	CodeMask     string       `json:"code_mask"`
	DurationDays int          `json:"duration_days"`
}

type RedemptionRecordResponse struct {
	ID               uint64    `json:"id"`
	CodeID           uint64    `json:"code_id"`
	CodeMask         string    `json:"code_mask"`
	UserID           uint64    `json:"user_id"`
	UserDisplayName  string    `json:"user_display_name,omitempty"`
	UserPhone        string    `json:"user_phone,omitempty"`
	GrantedPlan      string    `json:"granted_plan"`
	GrantedStatus    string    `json:"granted_status"`
	GrantedStartsAt  time.Time `json:"granted_starts_at"`
	GrantedExpiresAt time.Time `json:"granted_expires_at"`
	IP               string    `json:"ip,omitempty"`
	UserAgent        string    `json:"user_agent,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type RedemptionRecordListResponse struct {
	Items []RedemptionRecordResponse `json:"items"`
	Total int64                      `json:"total"`
}

type AdminUserDetailResponse struct {
	User              UserResponse               `json:"user"`
	RedemptionRecords []RedemptionRecordResponse `json:"redemption_records"`
}

type redemptionRecordRow struct {
	ID               uint64
	CodeID           uint64
	CodeMask         string
	UserID           uint64
	UserDisplayName  string
	UserPhone        string
	GrantedPlan      string
	GrantedStatus    string
	GrantedStartsAt  time.Time
	GrantedExpiresAt time.Time
	IP               string
	UserAgent        string
	CreatedAt        time.Time
}

func normalizeRedeemCode(input string) string {
	input = strings.TrimSpace(strings.ToUpper(input))
	if input == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range input {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func formatRedeemCode(canonical string) string {
	raw := normalizeRedeemCode(canonical)
	if raw == "" {
		return ""
	}
	parts := make([]string, 0, (len(raw)+3)/4)
	for i := 0; i < len(raw); i += 4 {
		end := i + 4
		if end > len(raw) {
			end = len(raw)
		}
		parts = append(parts, raw[i:end])
	}
	return strings.Join(parts, "-")
}

func maskRedeemCode(displayCode string) string {
	raw := normalizeRedeemCode(displayCode)
	if len(raw) <= 8 {
		return "****"
	}
	return fmt.Sprintf("%s****%s", raw[:4], raw[len(raw)-4:])
}

func hashRedeemCode(input string) string {
	sum := sha256.Sum256([]byte(normalizeRedeemCode(input)))
	return hex.EncodeToString(sum[:])
}

func sanitizeRedeemCodePrefix(prefix string) string {
	raw := normalizeRedeemCode(prefix)
	if raw == "" {
		return ""
	}
	if len(raw) > 8 {
		return raw[:8]
	}
	return raw
}

func randomCodePart(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	var b strings.Builder
	b.Grow(length)
	for _, v := range buf {
		b.WriteByte(redeemAlphabet[int(v)%len(redeemAlphabet)])
	}
	return b.String(), nil
}

func buildRedeemCode(prefix string) (display string, canonical string, err error) {
	safePrefix := sanitizeRedeemCodePrefix(prefix)
	randomLength := 16 - len(safePrefix)
	if randomLength < 8 {
		randomLength = 8
	}
	part, err := randomCodePart(randomLength)
	if err != nil {
		return "", "", err
	}
	canonical = safePrefix + part
	return formatRedeemCode(canonical), canonical, nil
}

func (h *Handler) buildUniqueRedeemCode(tx *gorm.DB, prefix string, localSet map[string]struct{}) (display string, hash string, err error) {
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
		if err := tx.Model(&models.RedeemCode{}).Where("code_hash = ?", sum).Count(&exists).Error; err != nil {
			return "", "", err
		}
		if exists > 0 {
			continue
		}
		localSet[sum] = struct{}{}
		return codeDisplay, sum, nil
	}
	return "", "", errors.New("failed to generate unique redeem code")
}

func (h *Handler) extractUserID(c *gin.Context) uint64 {
	if userAny, ok := c.Get("user_id"); ok {
		if uid, ok := userAny.(uint64); ok {
			return uid
		}
	}
	return 0
}

// GenerateRedeemCodes godoc
// @Summary Generate redeem codes
// @Tags admin
// @Accept json
// @Produce json
// @Param body body GenerateRedeemCodesRequest true "generate request"
// @Success 200 {object} GenerateRedeemCodesResponse
// @Router /api/admin/redeem-codes/generate [post]
func (h *Handler) GenerateRedeemCodes(c *gin.Context) {
	var req GenerateRedeemCodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Count <= 0 || req.Count > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "count must be between 1 and 500"})
		return
	}
	if req.DurationDays <= 0 || req.DurationDays > 3650 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "duration_days must be between 1 and 3650"})
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

	plan := strings.ToLower(strings.TrimSpace(req.Plan))
	if plan == "" {
		plan = "subscriber"
	}
	batchNo := strings.TrimSpace(req.BatchNo)
	if batchNo == "" {
		batchNo = "BATCH-" + time.Now().Format("20060102150405")
	}
	prefix := sanitizeRedeemCodePrefix(req.Prefix)
	adminUID := h.extractUserID(c)
	var createdBy *uint64
	if adminUID > 0 {
		createdBy = &adminUID
	}

	codesPlain := make([]string, 0, req.Count)
	rows := make([]models.RedeemCode, 0, req.Count)
	usedHashes := make(map[string]struct{}, req.Count)

	err := h.db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < req.Count; i++ {
			display, sum, err := h.buildUniqueRedeemCode(tx, prefix, usedHashes)
			if err != nil {
				return err
			}
			codesPlain = append(codesPlain, display)
			rows = append(rows, models.RedeemCode{
				CodeHash:     sum,
				CodeMask:     maskRedeemCode(display),
				BatchNo:      batchNo,
				Plan:         plan,
				DurationDays: req.DurationDays,
				MaxUses:      req.MaxUses,
				UsedCount:    0,
				Status:       "active",
				StartsAt:     req.StartsAt,
				EndsAt:       req.EndsAt,
				CreatedBy:    createdBy,
				Note:         strings.TrimSpace(req.Note),
			})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, GenerateRedeemCodesResponse{
		BatchNo:      batchNo,
		Count:        req.Count,
		Plan:         plan,
		DurationDays: req.DurationDays,
		MaxUses:      req.MaxUses,
		StartsAt:     req.StartsAt,
		EndsAt:       req.EndsAt,
		Codes:        codesPlain,
	})
}

// ListRedeemCodes godoc
// @Summary List redeem codes
// @Tags admin
// @Produce json
// @Param q query string false "keyword"
// @Param status query string false "status"
// @Param plan query string false "plan"
// @Param batch_no query string false "batch"
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} RedeemCodeListResponse
// @Router /api/admin/redeem-codes [get]
func (h *Handler) ListRedeemCodes(c *gin.Context) {
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
	plan := strings.ToLower(strings.TrimSpace(c.Query("plan")))
	batchNo := strings.TrimSpace(c.Query("batch_no"))

	query := h.db.Model(&models.RedeemCode{})
	if q != "" {
		query = query.Where("code_mask ILIKE ? OR note ILIKE ? OR batch_no ILIKE ?", "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if plan != "" {
		query = query.Where("plan = ?", plan)
	}
	if batchNo != "" {
		query = query.Where("batch_no = ?", batchNo)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []models.RedeemCode
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]RedeemCodeAdminItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RedeemCodeAdminItem{
			ID:            row.ID,
			CodeMask:      row.CodeMask,
			BatchNo:       row.BatchNo,
			Plan:          row.Plan,
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
	c.JSON(http.StatusOK, RedeemCodeListResponse{Items: items, Total: total})
}

// UpdateRedeemCodeStatus godoc
// @Summary Update redeem code status
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "code id"
// @Param body body UpdateRedeemCodeStatusRequest true "status"
// @Success 200 {object} RedeemCodeAdminItem
// @Router /api/admin/redeem-codes/{id}/status [put]
func (h *Handler) UpdateRedeemCodeStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req UpdateRedeemCodeStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != "active" && status != "disabled" && status != "expired" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	var row models.RedeemCode
	if err := h.db.First(&row, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "redeem code not found"})
		return
	}
	row.Status = status
	if err := h.db.Save(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, RedeemCodeAdminItem{
		ID:            row.ID,
		CodeMask:      row.CodeMask,
		BatchNo:       row.BatchNo,
		Plan:          row.Plan,
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

func (h *Handler) queryRedemptionRecords(base *gorm.DB, page, pageSize int) ([]RedemptionRecordResponse, int64, error) {
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := base
	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}

	var rows []redemptionRecordRow
	err := query.
		Order("r.id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]RedemptionRecordResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, RedemptionRecordResponse{
			ID:               row.ID,
			CodeID:           row.CodeID,
			CodeMask:         row.CodeMask,
			UserID:           row.UserID,
			UserDisplayName:  row.UserDisplayName,
			UserPhone:        row.UserPhone,
			GrantedPlan:      row.GrantedPlan,
			GrantedStatus:    row.GrantedStatus,
			GrantedStartsAt:  row.GrantedStartsAt,
			GrantedExpiresAt: row.GrantedExpiresAt,
			IP:               row.IP,
			UserAgent:        row.UserAgent,
			CreatedAt:        row.CreatedAt,
		})
	}

	return items, total, nil
}

func redemptionRecordBaseQuery(db *gorm.DB) *gorm.DB {
	return db.Table("ops.redeem_code_redemptions AS r").
		Select(`
r.id,
r.code_id,
COALESCE(c.code_mask, '') AS code_mask,
r.user_id,
COALESCE(u.display_name, '') AS user_display_name,
COALESCE(u.phone, '') AS user_phone,
r.granted_plan,
r.granted_status,
r.granted_starts_at,
r.granted_expires_at,
r.ip,
r.user_agent,
r.created_at
		`).
		Joins("LEFT JOIN ops.redeem_codes c ON c.id = r.code_id").
		Joins(`LEFT JOIN "user".users u ON u.id = r.user_id`)
}

// ListRedeemCodeRedemptions godoc
// @Summary List redemption records by code
// @Tags admin
// @Produce json
// @Param id path int true "code id"
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} RedemptionRecordListResponse
// @Router /api/admin/redeem-codes/{id}/redemptions [get]
func (h *Handler) ListRedeemCodeRedemptions(c *gin.Context) {
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

	base := redemptionRecordBaseQuery(h.db).Where("r.code_id = ?", codeID)
	items, total, err := h.queryRedemptionRecords(base, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RedemptionRecordListResponse{Items: items, Total: total})
}

// RedeemCodeForMe godoc
// @Summary Redeem subscription code
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RedeemCodeSubmitRequest true "redeem code"
// @Success 200 {object} RedeemCodeSubmitResponse
// @Router /api/me/redeem-code [post]
func (h *Handler) RedeemCodeForMe(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req RedeemCodeSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	canonical := normalizeRedeemCode(req.Code)
	if canonical == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code required"})
		return
	}
	codeHash := hashRedeemCode(canonical)
	now := time.Now()

	var finalUser models.User
	var finalCode models.RedeemCode
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var code models.RedeemCode
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
			_ = tx.Model(&models.RedeemCode{}).Where("id = ?", code.ID).Update("status", "expired").Error
			return fmt.Errorf("兑换码已过期")
		}
		if code.MaxUses > 0 && code.UsedCount >= code.MaxUses {
			return fmt.Errorf("兑换码已被使用完")
		}

		var already int64
		if err := tx.Model(&models.RedeemCodeRedemption{}).
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
		if user.SubscriptionExpiresAt != nil && user.SubscriptionExpiresAt.After(now) {
			grantStartsAt = *user.SubscriptionExpiresAt
		}
		durationDays := code.DurationDays
		if durationDays <= 0 {
			durationDays = 30
		}
		grantExpiresAt := grantStartsAt.AddDate(0, 0, durationDays)
		previousSubscriber := false
		_, _, previousSubscriber = resolveUserSubscriptionState(&user, now)

		updateFields := map[string]interface{}{
			"subscription_status":     "active",
			"subscription_plan":       strings.TrimSpace(code.Plan),
			"subscription_expires_at": grantExpiresAt,
		}
		if !previousSubscriber || user.SubscriptionStartedAt == nil {
			updateFields["subscription_started_at"] = now
		}
		if err := tx.Model(&models.User{}).Where("id = ?", user.ID).Updates(updateFields).Error; err != nil {
			return err
		}

		redemption := models.RedeemCodeRedemption{
			CodeID:           code.ID,
			UserID:           user.ID,
			GrantedPlan:      strings.TrimSpace(code.Plan),
			GrantedStatus:    "active",
			GrantedStartsAt:  grantStartsAt,
			GrantedExpiresAt: grantExpiresAt,
			IP:               strings.TrimSpace(c.ClientIP()),
			UserAgent:        strings.TrimSpace(c.GetHeader("User-Agent")),
		}
		if err := tx.Create(&redemption).Error; err != nil {
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
		if err := tx.Model(&models.RedeemCode{}).Where("id = ?", code.ID).Updates(codeUpdates).Error; err != nil {
			return err
		}

		if err := tx.First(&finalUser, user.ID).Error; err != nil {
			return err
		}
		if err := tx.First(&finalCode, code.ID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, RedeemCodeSubmitResponse{
		Message:      "兑换成功，合集下载权限已开通",
		User:         mapUser(finalUser),
		Plan:         finalCode.Plan,
		ExpiresAt:    finalUser.SubscriptionExpiresAt,
		UsedCount:    finalCode.UsedCount,
		MaxUses:      finalCode.MaxUses,
		CodeMask:     finalCode.CodeMask,
		DurationDays: finalCode.DurationDays,
	})
}

// ListMyRedeemRecords godoc
// @Summary List my redemption records
// @Tags auth
// @Produce json
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} RedemptionRecordListResponse
// @Router /api/me/redeem-records [get]
func (h *Handler) ListMyRedeemRecords(c *gin.Context) {
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

	base := redemptionRecordBaseQuery(h.db).Where("r.user_id = ?", userID)
	items, total, err := h.queryRedemptionRecords(base, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RedemptionRecordListResponse{Items: items, Total: total})
}

// GetAdminUserDetail godoc
// @Summary Get user detail with redemption records
// @Tags admin
// @Produce json
// @Param id path int true "user id"
// @Success 200 {object} AdminUserDetailResponse
// @Router /api/admin/users/{id}/detail [get]
func (h *Handler) GetAdminUserDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	syncExpiredSubscription(h.db, &user, time.Now())

	base := redemptionRecordBaseQuery(h.db).Where("r.user_id = ?", id)
	items, _, err := h.queryRedemptionRecords(base, 1, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, AdminUserDetailResponse{
		User:              mapUser(user),
		RedemptionRecords: items,
	})
}
