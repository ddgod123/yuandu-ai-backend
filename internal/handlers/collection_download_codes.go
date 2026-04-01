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

const collectionDownloadCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

var (
	errCollectionEntitlementRequired  = errors.New("collection_download_entitlement_required")
	errCollectionEntitlementExhausted = errors.New("collection_download_entitlement_exhausted")
	errCollectionEntitlementExpired   = errors.New("collection_download_entitlement_expired")
	errCollectionEntitlementDisabled  = errors.New("collection_download_entitlement_disabled")
)

type GenerateCollectionDownloadCodesRequest struct {
	Count          int        `json:"count"`
	BatchNo        string     `json:"batch_no"`
	CollectionID   uint64     `json:"collection_id"`
	DownloadTimes  int        `json:"download_times"`
	MaxRedeemUsers int        `json:"max_redeem_users"`
	Prefix         string     `json:"prefix"`
	StartsAt       *time.Time `json:"starts_at"`
	EndsAt         *time.Time `json:"ends_at"`
	Note           string     `json:"note"`
}

type GenerateCollectionDownloadCodesResponse struct {
	BatchNo         string   `json:"batch_no"`
	CollectionID    uint64   `json:"collection_id"`
	CollectionTitle string   `json:"collection_title"`
	Count           int      `json:"count"`
	DownloadTimes   int      `json:"download_times"`
	MaxRedeemUsers  int      `json:"max_redeem_users"`
	Codes           []string `json:"codes"`
}

type CollectionDownloadCodeAdminItem struct {
	ID                   uint64     `json:"id"`
	CodeMask             string     `json:"code_mask"`
	CodePlain            string     `json:"code_plain,omitempty"`
	BatchNo              string     `json:"batch_no"`
	CollectionID         uint64     `json:"collection_id"`
	CollectionTitle      string     `json:"collection_title,omitempty"`
	GrantedDownloadTimes int        `json:"granted_download_times"`
	MaxRedeemUsers       int        `json:"max_redeem_users"`
	UsedRedeemUsers      int        `json:"used_redeem_users"`
	Status               string     `json:"status"`
	StartsAt             *time.Time `json:"starts_at,omitempty"`
	EndsAt               *time.Time `json:"ends_at,omitempty"`
	Note                 string     `json:"note,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type CollectionDownloadCodeListResponse struct {
	Items []CollectionDownloadCodeAdminItem `json:"items"`
	Total int64                             `json:"total"`
}

type UpdateCollectionDownloadCodeStatusRequest struct {
	Status string `json:"status"`
}

type CollectionDownloadCodeSubmitRequest struct {
	Code string `json:"code"`
}

type CollectionDownloadCodeValidateResponse struct {
	Valid                bool       `json:"valid"`
	Message              string     `json:"message"`
	CodeMask             string     `json:"code_mask,omitempty"`
	Status               string     `json:"status,omitempty"`
	CollectionID         uint64     `json:"collection_id,omitempty"`
	CollectionTitle      string     `json:"collection_title,omitempty"`
	GrantedDownloadTimes int        `json:"granted_download_times,omitempty"`
	MaxRedeemUsers       int        `json:"max_redeem_users,omitempty"`
	UsedRedeemUsers      int        `json:"used_redeem_users,omitempty"`
	StartsAt             *time.Time `json:"starts_at,omitempty"`
	EndsAt               *time.Time `json:"ends_at,omitempty"`
}

type CollectionDownloadCodeRedeemResponse struct {
	Message                string     `json:"message"`
	CodeMask               string     `json:"code_mask,omitempty"`
	CollectionID           uint64     `json:"collection_id"`
	CollectionTitle        string     `json:"collection_title,omitempty"`
	GrantedDownloadTimes   int        `json:"granted_download_times"`
	RemainingDownloadTimes int        `json:"remaining_download_times"`
	ExpiresAt              *time.Time `json:"expires_at,omitempty"`
}

type CollectionDownloadEntitlementItem struct {
	ID                     uint64     `json:"id"`
	UserID                 uint64     `json:"user_id,omitempty"`
	UserDisplayName        string     `json:"user_display_name,omitempty"`
	UserPhone              string     `json:"user_phone,omitempty"`
	CollectionID           uint64     `json:"collection_id"`
	CollectionTitle        string     `json:"collection_title,omitempty"`
	CodeID                 *uint64    `json:"code_id,omitempty"`
	GrantedDownloadTimes   int        `json:"granted_download_times"`
	UsedDownloadTimes      int        `json:"used_download_times"`
	RemainingDownloadTimes int        `json:"remaining_download_times"`
	Status                 string     `json:"status"`
	ExpiresAt              *time.Time `json:"expires_at,omitempty"`
	LastConsumedAt         *time.Time `json:"last_consumed_at,omitempty"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type CollectionDownloadEntitlementListResponse struct {
	Items []CollectionDownloadEntitlementItem `json:"items"`
	Total int64                               `json:"total"`
}

type CollectionDownloadRedemptionRecord struct {
	ID                   uint64     `json:"id"`
	CodeID               uint64     `json:"code_id"`
	CodeMask             string     `json:"code_mask,omitempty"`
	UserID               uint64     `json:"user_id"`
	UserDisplayName      string     `json:"user_display_name,omitempty"`
	UserPhone            string     `json:"user_phone,omitempty"`
	CollectionID         uint64     `json:"collection_id"`
	CollectionTitle      string     `json:"collection_title,omitempty"`
	GrantedDownloadTimes int        `json:"granted_download_times"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	IP                   string     `json:"ip,omitempty"`
	UserAgent            string     `json:"user_agent,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

type CollectionDownloadRedemptionListResponse struct {
	Items []CollectionDownloadRedemptionRecord `json:"items"`
	Total int64                                `json:"total"`
}

type UpdateCollectionDownloadEntitlementRequest struct {
	SetRemainingDownloadTimes   *int   `json:"set_remaining_download_times"`
	DeltaRemainingDownloadTimes *int   `json:"delta_remaining_download_times"`
	Status                      string `json:"status"`
}

type collectionDownloadEntitlementQueryRow struct {
	ID                     uint64
	UserID                 uint64
	UserDisplayName        string
	UserPhone              string
	CollectionID           uint64
	CollectionTitle        string
	CodeID                 *uint64
	GrantedDownloadTimes   int
	UsedDownloadTimes      int
	RemainingDownloadTimes int
	Status                 string
	ExpiresAt              *time.Time
	LastConsumedAt         *time.Time
	UpdatedAt              time.Time
}

type collectionDownloadRedemptionQueryRow struct {
	ID                   uint64
	CodeID               uint64
	CodeMask             string
	UserID               uint64
	UserDisplayName      string
	UserPhone            string
	CollectionID         uint64
	CollectionTitle      string
	GrantedDownloadTimes int
	ExpiresAt            *time.Time
	IP                   string
	UserAgent            string
	CreatedAt            time.Time
}

type collectionDownloadAccessDecision struct {
	Allowed            bool
	IsSubscriber       bool
	SubscriptionStatus string
	DenyError          error
	Entitlement        *models.CollectionDownloadEntitlement
}

func normalizeCollectionDownloadCode(input string) string {
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

func sanitizeCollectionDownloadCodePrefix(prefix string) string {
	raw := normalizeCollectionDownloadCode(prefix)
	if raw == "" {
		return ""
	}
	if len(raw) > 8 {
		return raw[:8]
	}
	return raw
}

func formatCollectionDownloadCode(canonical string) string {
	raw := normalizeCollectionDownloadCode(canonical)
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

func maskCollectionDownloadCode(display string) string {
	raw := normalizeCollectionDownloadCode(display)
	if len(raw) <= 8 {
		return "****"
	}
	return fmt.Sprintf("%s****%s", raw[:4], raw[len(raw)-4:])
}

func hashCollectionDownloadCode(input string) string {
	sum := sha256.Sum256([]byte(normalizeCollectionDownloadCode(input)))
	return hex.EncodeToString(sum[:])
}

func randomCollectionDownloadCodePart(length int) (string, error) {
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
		b.WriteByte(collectionDownloadCodeAlphabet[int(v)%len(collectionDownloadCodeAlphabet)])
	}
	return b.String(), nil
}

func buildCollectionDownloadCode(prefix string) (display string, canonical string, err error) {
	safePrefix := sanitizeCollectionDownloadCodePrefix(prefix)
	randomLength := 16 - len(safePrefix)
	if randomLength < 8 {
		randomLength = 8
	}
	part, err := randomCollectionDownloadCodePart(randomLength)
	if err != nil {
		return "", "", err
	}
	canonical = safePrefix + part
	return formatCollectionDownloadCode(canonical), canonical, nil
}

func (h *Handler) buildUniqueCollectionDownloadCode(tx *gorm.DB, prefix string, localSet map[string]struct{}) (display string, hash string, err error) {
	for i := 0; i < 20; i++ {
		codeDisplay, canonical, genErr := buildCollectionDownloadCode(prefix)
		if genErr != nil {
			return "", "", genErr
		}
		sum := hashCollectionDownloadCode(canonical)
		if _, exists := localSet[sum]; exists {
			continue
		}
		var exists int64
		if err := tx.Model(&models.CollectionDownloadCode{}).Where("code_hash = ?", sum).Count(&exists).Error; err != nil {
			return "", "", err
		}
		if exists > 0 {
			continue
		}
		localSet[sum] = struct{}{}
		return codeDisplay, sum, nil
	}
	return "", "", errors.New("failed to generate unique collection download code")
}

func (h *Handler) ensureCollectionDownloadCodeVisible(tx *gorm.DB, collectionID uint64) (models.Collection, error) {
	var collection models.Collection
	if err := tx.Select("id", "title", "status", "visibility").First(&collection, collectionID).Error; err != nil {
		return collection, err
	}
	if !isPublicCollectionVisible(collection) {
		return collection, errors.New("collection unavailable")
	}
	return collection, nil
}

func evaluateCollectionDownloadCodeForUser(db *gorm.DB, userID uint64, codeHash string, now time.Time) CollectionDownloadCodeValidateResponse {
	resp := CollectionDownloadCodeValidateResponse{
		Valid:   false,
		Message: "兑换码无效",
	}

	var code models.CollectionDownloadCode
	if err := db.Where("code_hash = ?", codeHash).First(&code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return resp
		}
		resp.Message = "暂时无法验证，请稍后重试"
		return resp
	}
	resp.CodeMask = code.CodeMask
	resp.Status = strings.ToLower(strings.TrimSpace(code.Status))
	resp.CollectionID = code.CollectionID
	resp.GrantedDownloadTimes = code.GrantedDownloadTimes
	resp.MaxRedeemUsers = code.MaxRedeemUsers
	resp.UsedRedeemUsers = code.UsedRedeemUsers
	resp.StartsAt = code.StartsAt
	resp.EndsAt = code.EndsAt

	var collection models.Collection
	if err := db.Select("id", "title", "status", "visibility").First(&collection, code.CollectionID).Error; err == nil {
		resp.CollectionTitle = strings.TrimSpace(collection.Title)
		if !isPublicCollectionVisible(collection) {
			resp.Message = "该合集暂不可用"
			return resp
		}
	}

	if resp.Status != "active" {
		resp.Message = "兑换码不可用"
		return resp
	}
	if code.StartsAt != nil && now.Before(*code.StartsAt) {
		resp.Message = "兑换码尚未到生效时间"
		return resp
	}
	if code.EndsAt != nil && !code.EndsAt.After(now) {
		resp.Status = "expired"
		resp.Message = "兑换码已过期"
		return resp
	}
	if code.MaxRedeemUsers > 0 && code.UsedRedeemUsers >= code.MaxRedeemUsers {
		resp.Status = "expired"
		resp.Message = "兑换码已被使用完"
		return resp
	}

	var already int64
	if err := db.Model(&models.CollectionDownloadRedemption{}).
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

	resp.Valid = true
	resp.Message = "兑换码可用，可立即兑换"
	return resp
}

func mapCollectionDownloadEntitlementRow(row collectionDownloadEntitlementQueryRow) CollectionDownloadEntitlementItem {
	return CollectionDownloadEntitlementItem{
		ID:                     row.ID,
		UserID:                 row.UserID,
		UserDisplayName:        row.UserDisplayName,
		UserPhone:              row.UserPhone,
		CollectionID:           row.CollectionID,
		CollectionTitle:        row.CollectionTitle,
		CodeID:                 row.CodeID,
		GrantedDownloadTimes:   row.GrantedDownloadTimes,
		UsedDownloadTimes:      row.UsedDownloadTimes,
		RemainingDownloadTimes: row.RemainingDownloadTimes,
		Status:                 row.Status,
		ExpiresAt:              row.ExpiresAt,
		LastConsumedAt:         row.LastConsumedAt,
		UpdatedAt:              row.UpdatedAt,
	}
}

func normalizeCollectionEntitlementStatus(raw string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "active", "disabled", "exhausted", "expired":
		return v, true
	default:
		return "", false
	}
}

func isTruthyQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func shouldExposeCollectionCodePlain(c *gin.Context) bool {
	if c == nil || !isTruthyQuery(c.Query("include_plain")) {
		return false
	}
	roleAny, ok := c.Get("role")
	if !ok {
		return false
	}
	role, ok := roleAny.(string)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(role), "super_admin")
}

func normalizeCollectionDownloadEntitlementRuntimeStatus(status string, remainingDownloadTimes int, expiresAt *time.Time, now time.Time) string {
	current := strings.ToLower(strings.TrimSpace(status))
	if current == "disabled" {
		return "disabled"
	}
	if expiresAt != nil && !expiresAt.After(now) {
		return "expired"
	}
	if remainingDownloadTimes <= 0 {
		return "exhausted"
	}
	return "active"
}

func (h *Handler) normalizeCollectionDownloadEntitlementStatuses(now time.Time, userID, collectionID uint64) error {
	if h == nil || h.db == nil {
		return nil
	}
	scope := func() *gorm.DB {
		query := h.db.Model(&models.CollectionDownloadEntitlement{})
		if userID > 0 {
			query = query.Where("user_id = ?", userID)
		}
		if collectionID > 0 {
			query = query.Where("collection_id = ?", collectionID)
		}
		return query
	}

	if err := scope().
		Where("status <> ? AND expires_at IS NOT NULL AND expires_at <= ?", "expired", now).
		Update("status", "expired").Error; err != nil {
		return err
	}
	if err := scope().
		Where("status NOT IN ? AND remaining_download_times <= 0", []string{"disabled", "expired", "exhausted"}).
		Update("status", "exhausted").Error; err != nil {
		return err
	}
	if err := scope().
		Where("status = ? AND remaining_download_times > 0 AND (expires_at IS NULL OR expires_at > ?)", "exhausted", now).
		Update("status", "active").Error; err != nil {
		return err
	}
	return nil
}

func collectionDownloadCodeSnapshot(row models.CollectionDownloadCode) map[string]interface{} {
	return map[string]interface{}{
		"id":                     row.ID,
		"collection_id":          row.CollectionID,
		"batch_no":               strings.TrimSpace(row.BatchNo),
		"code_mask":              strings.TrimSpace(row.CodeMask),
		"granted_download_times": row.GrantedDownloadTimes,
		"max_redeem_users":       row.MaxRedeemUsers,
		"used_redeem_users":      row.UsedRedeemUsers,
		"status":                 strings.TrimSpace(row.Status),
		"starts_at":              row.StartsAt,
		"ends_at":                row.EndsAt,
		"note":                   strings.TrimSpace(row.Note),
		"updated_at":             row.UpdatedAt,
	}
}

func collectionDownloadEntitlementSnapshot(row models.CollectionDownloadEntitlement) map[string]interface{} {
	return map[string]interface{}{
		"id":                       row.ID,
		"user_id":                  row.UserID,
		"collection_id":            row.CollectionID,
		"code_id":                  row.CodeID,
		"granted_download_times":   row.GrantedDownloadTimes,
		"used_download_times":      row.UsedDownloadTimes,
		"remaining_download_times": row.RemainingDownloadTimes,
		"status":                   strings.TrimSpace(row.Status),
		"expires_at":               row.ExpiresAt,
		"last_consumed_at":         row.LastConsumedAt,
		"updated_at":               row.UpdatedAt,
	}
}

func sampleUint64(values []uint64, max int) []uint64 {
	if max <= 0 || len(values) <= max {
		return values
	}
	out := make([]uint64, max)
	copy(out, values[:max])
	return out
}

func (h *Handler) resolveCollectionDownloadAccess(user *models.User, collectionID uint64, now time.Time) collectionDownloadAccessDecision {
	if user == nil {
		return collectionDownloadAccessDecision{Allowed: true, IsSubscriber: true}
	}

	syncExpiredSubscription(h.db, user, now)
	subscriptionStatus, _, isSubscriber := resolveUserSubscriptionState(user, now)
	if isSubscriber {
		return collectionDownloadAccessDecision{
			Allowed:            true,
			IsSubscriber:       true,
			SubscriptionStatus: subscriptionStatus,
		}
	}

	var entitlement models.CollectionDownloadEntitlement
	err := h.db.Where("user_id = ? AND collection_id = ?", user.ID, collectionID).
		Order("id ASC").
		First(&entitlement).Error
	if err != nil {
		return collectionDownloadAccessDecision{
			Allowed:            false,
			IsSubscriber:       false,
			SubscriptionStatus: subscriptionStatus,
			DenyError:          errCollectionEntitlementRequired,
		}
	}

	status := strings.ToLower(strings.TrimSpace(entitlement.Status))
	if status == "disabled" {
		return collectionDownloadAccessDecision{
			Allowed:            false,
			IsSubscriber:       false,
			SubscriptionStatus: subscriptionStatus,
			DenyError:          errCollectionEntitlementDisabled,
		}
	}
	if entitlement.ExpiresAt != nil && !entitlement.ExpiresAt.After(now) {
		_ = h.db.Model(&models.CollectionDownloadEntitlement{}).
			Where("id = ? AND status <> ?", entitlement.ID, "expired").
			Update("status", "expired").Error
		return collectionDownloadAccessDecision{
			Allowed:            false,
			IsSubscriber:       false,
			SubscriptionStatus: subscriptionStatus,
			DenyError:          errCollectionEntitlementExpired,
		}
	}
	if entitlement.RemainingDownloadTimes <= 0 || status == "exhausted" {
		_ = h.db.Model(&models.CollectionDownloadEntitlement{}).
			Where("id = ? AND status <> ?", entitlement.ID, "exhausted").
			Update("status", "exhausted").Error
		return collectionDownloadAccessDecision{
			Allowed:            false,
			IsSubscriber:       false,
			SubscriptionStatus: subscriptionStatus,
			DenyError:          errCollectionEntitlementExhausted,
		}
	}
	if status != "active" {
		return collectionDownloadAccessDecision{
			Allowed:            false,
			IsSubscriber:       false,
			SubscriptionStatus: subscriptionStatus,
			DenyError:          errCollectionEntitlementRequired,
		}
	}

	return collectionDownloadAccessDecision{
		Allowed:            true,
		IsSubscriber:       false,
		SubscriptionStatus: subscriptionStatus,
		Entitlement:        &entitlement,
	}
}

func writeCollectionDownloadAccessDenied(c *gin.Context, decision collectionDownloadAccessDecision) {
	errCode := "subscription_or_entitlement_required"
	message := "当前账号暂无合集下载权限"
	switch decision.DenyError {
	case errCollectionEntitlementExhausted:
		errCode = "collection_download_entitlement_exhausted"
		message = "该合集下载次数已用尽"
	case errCollectionEntitlementExpired:
		errCode = "collection_download_entitlement_expired"
		message = "该合集下载权益已过期"
	case errCollectionEntitlementDisabled:
		errCode = "collection_download_entitlement_disabled"
		message = "该合集下载权益不可用"
	case errCollectionEntitlementRequired:
		errCode = "collection_download_entitlement_required"
		message = "该合集下载需要订阅或次卡权益"
	}
	c.JSON(http.StatusForbidden, gin.H{
		"error":               errCode,
		"message":             message,
		"subscription_status": decision.SubscriptionStatus,
	})
}

func (h *Handler) consumeCollectionDownloadEntitlement(c *gin.Context, userID, collectionID uint64, mode string) error {
	if userID == 0 || collectionID == 0 {
		return errCollectionEntitlementRequired
	}
	now := time.Now()
	return h.db.Transaction(func(tx *gorm.DB) error {
		var entitlement models.CollectionDownloadEntitlement
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND collection_id = ?", userID, collectionID).
			First(&entitlement).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errCollectionEntitlementRequired
			}
			return err
		}

		status := strings.ToLower(strings.TrimSpace(entitlement.Status))
		if status == "disabled" {
			return errCollectionEntitlementDisabled
		}
		if entitlement.ExpiresAt != nil && !entitlement.ExpiresAt.After(now) {
			_ = tx.Model(&models.CollectionDownloadEntitlement{}).
				Where("id = ?", entitlement.ID).
				Update("status", "expired").Error
			return errCollectionEntitlementExpired
		}
		if entitlement.RemainingDownloadTimes <= 0 || status == "exhausted" {
			_ = tx.Model(&models.CollectionDownloadEntitlement{}).
				Where("id = ?", entitlement.ID).
				Update("status", "exhausted").Error
			return errCollectionEntitlementExhausted
		}
		if status != "active" {
			return errCollectionEntitlementRequired
		}

		entitlement.RemainingDownloadTimes--
		entitlement.UsedDownloadTimes++
		lastConsumedAt := now
		entitlement.LastConsumedAt = &lastConsumedAt
		if entitlement.RemainingDownloadTimes <= 0 {
			entitlement.Status = "exhausted"
		} else {
			entitlement.Status = "active"
		}
		if entitlement.GrantedDownloadTimes < entitlement.UsedDownloadTimes+entitlement.RemainingDownloadTimes {
			entitlement.GrantedDownloadTimes = entitlement.UsedDownloadTimes + entitlement.RemainingDownloadTimes
		}
		if err := tx.Save(&entitlement).Error; err != nil {
			return err
		}

		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		consumption := models.CollectionDownloadConsumption{
			EntitlementID: entitlement.ID,
			UserID:        userID,
			CollectionID:  collectionID,
			CodeID:        entitlement.CodeID,
			DownloadMode:  strings.TrimSpace(mode),
			ConsumedTimes: 1,
			IP:            strings.TrimSpace(c.ClientIP()),
			UserAgent:     strings.TrimSpace(c.GetHeader("User-Agent")),
			RequestID:     requestID,
		}
		return tx.Create(&consumption).Error
	})
}

// GenerateCollectionDownloadCodes godoc
// @Summary Generate collection download code cards
// @Tags admin
// @Accept json
// @Produce json
// @Param body body GenerateCollectionDownloadCodesRequest true "generate request"
// @Success 200 {object} GenerateCollectionDownloadCodesResponse
// @Router /api/admin/collection-download-codes/generate [post]
func (h *Handler) GenerateCollectionDownloadCodes(c *gin.Context) {
	var req GenerateCollectionDownloadCodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Count <= 0 || req.Count > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "count must be between 1 and 500"})
		return
	}
	if req.CollectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_id required"})
		return
	}
	if req.DownloadTimes <= 0 || req.DownloadTimes > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "download_times must be between 1 and 10000"})
		return
	}
	if req.MaxRedeemUsers <= 0 || req.MaxRedeemUsers > 10000 {
		req.MaxRedeemUsers = 1
	}
	if req.StartsAt != nil && req.EndsAt != nil && !req.EndsAt.After(*req.StartsAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ends_at must be after starts_at"})
		return
	}

	adminUID := h.extractUserID(c)
	var createdBy *uint64
	if adminUID > 0 {
		createdBy = &adminUID
	}
	prefix := sanitizeCollectionDownloadCodePrefix(req.Prefix)
	batchNo := strings.TrimSpace(req.BatchNo)
	if batchNo == "" {
		batchNo = "CARD-" + time.Now().Format("20060102150405")
	}

	codesPlain := make([]string, 0, req.Count)
	rows := make([]models.CollectionDownloadCode, 0, req.Count)
	usedHashes := make(map[string]struct{}, req.Count)
	collectionTitle := ""

	err := h.db.Transaction(func(tx *gorm.DB) error {
		collection, err := h.ensureCollectionDownloadCodeVisible(tx, req.CollectionID)
		if err != nil {
			return err
		}
		collectionTitle = strings.TrimSpace(collection.Title)
		for i := 0; i < req.Count; i++ {
			display, sum, err := h.buildUniqueCollectionDownloadCode(tx, prefix, usedHashes)
			if err != nil {
				return err
			}
			codesPlain = append(codesPlain, display)
			rows = append(rows, models.CollectionDownloadCode{
				CodeHash:             sum,
				CodePlain:            display,
				CodeMask:             maskCollectionDownloadCode(display),
				BatchNo:              batchNo,
				CollectionID:         req.CollectionID,
				GrantedDownloadTimes: req.DownloadTimes,
				MaxRedeemUsers:       req.MaxRedeemUsers,
				UsedRedeemUsers:      0,
				Status:               "active",
				StartsAt:             req.StartsAt,
				EndsAt:               req.EndsAt,
				CreatedBy:            createdBy,
				Note:                 strings.TrimSpace(req.Note),
			})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "collection unavailable") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "collection unavailable"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	generatedIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		if row.ID > 0 {
			generatedIDs = append(generatedIDs, row.ID)
		}
	}
	h.recordAuditLog(adminUID, "collection", req.CollectionID, "admin_generate_collection_download_codes", map[string]interface{}{
		"batch_no":                batchNo,
		"collection_id":           req.CollectionID,
		"collection_title":        collectionTitle,
		"count":                   req.Count,
		"download_times":          req.DownloadTimes,
		"max_redeem_users":        req.MaxRedeemUsers,
		"prefix":                  prefix,
		"starts_at":               req.StartsAt,
		"ends_at":                 req.EndsAt,
		"note":                    strings.TrimSpace(req.Note),
		"generated_code_ids":      sampleUint64(generatedIDs, 30),
		"generated_code_id_count": len(generatedIDs),
	})

	c.JSON(http.StatusOK, GenerateCollectionDownloadCodesResponse{
		BatchNo:         batchNo,
		CollectionID:    req.CollectionID,
		CollectionTitle: collectionTitle,
		Count:           req.Count,
		DownloadTimes:   req.DownloadTimes,
		MaxRedeemUsers:  req.MaxRedeemUsers,
		Codes:           codesPlain,
	})
}

// ListCollectionDownloadCodes godoc
// @Summary List collection download code cards
// @Tags admin
// @Produce json
// @Router /api/admin/collection-download-codes [get]
func (h *Handler) ListCollectionDownloadCodes(c *gin.Context) {
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
	collectionID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("collection_id")), 10, 64)
	includePlain := shouldExposeCollectionCodePlain(c)

	query := h.db.Model(&models.CollectionDownloadCode{})
	if q != "" {
		query = query.Where("code_mask ILIKE ? OR code_plain ILIKE ? OR note ILIKE ? OR batch_no ILIKE ?", "%"+q+"%", "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if batchNo != "" {
		query = query.Where("batch_no = ?", batchNo)
	}
	if collectionID > 0 {
		query = query.Where("collection_id = ?", collectionID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []models.CollectionDownloadCode
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	collectionIDs := make([]uint64, 0, len(rows))
	seen := map[uint64]struct{}{}
	for _, row := range rows {
		if _, ok := seen[row.CollectionID]; ok {
			continue
		}
		seen[row.CollectionID] = struct{}{}
		collectionIDs = append(collectionIDs, row.CollectionID)
	}
	titleMap := make(map[uint64]string, len(collectionIDs))
	if len(collectionIDs) > 0 {
		var collections []models.Collection
		_ = h.db.Select("id", "title").Where("id IN ?", collectionIDs).Find(&collections).Error
		for _, item := range collections {
			titleMap[item.ID] = strings.TrimSpace(item.Title)
		}
	}

	items := make([]CollectionDownloadCodeAdminItem, 0, len(rows))
	for _, row := range rows {
		codePlain := ""
		if includePlain {
			codePlain = strings.TrimSpace(row.CodePlain)
		}
		items = append(items, CollectionDownloadCodeAdminItem{
			ID:                   row.ID,
			CodeMask:             row.CodeMask,
			CodePlain:            codePlain,
			BatchNo:              row.BatchNo,
			CollectionID:         row.CollectionID,
			CollectionTitle:      titleMap[row.CollectionID],
			GrantedDownloadTimes: row.GrantedDownloadTimes,
			MaxRedeemUsers:       row.MaxRedeemUsers,
			UsedRedeemUsers:      row.UsedRedeemUsers,
			Status:               row.Status,
			StartsAt:             row.StartsAt,
			EndsAt:               row.EndsAt,
			Note:                 row.Note,
			CreatedAt:            row.CreatedAt,
			UpdatedAt:            row.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, CollectionDownloadCodeListResponse{Items: items, Total: total})
}

// UpdateCollectionDownloadCodeStatus godoc
// @Summary Update collection download code card status
// @Tags admin
// @Accept json
// @Produce json
// @Router /api/admin/collection-download-codes/{id}/status [put]
func (h *Handler) UpdateCollectionDownloadCodeStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req UpdateCollectionDownloadCodeStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status, ok := normalizeCollectionEntitlementStatus(req.Status)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}
	if status == "exhausted" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	var row models.CollectionDownloadCode
	if err := h.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection download code not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	before := collectionDownloadCodeSnapshot(row)
	row.Status = status
	if err := h.db.Save(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	adminUID := h.extractUserID(c)
	h.recordAuditLog(adminUID, "collection_download_code", row.ID, "admin_update_collection_download_code_status", map[string]interface{}{
		"before": before,
		"after":  collectionDownloadCodeSnapshot(row),
	})
	c.JSON(http.StatusOK, gin.H{
		"id":      row.ID,
		"status":  row.Status,
		"updated": true,
	})
}

// ListCollectionDownloadCodeRedemptions godoc
// @Summary List collection download code redemptions by code
// @Tags admin
// @Produce json
// @Router /api/admin/collection-download-codes/{id}/redemptions [get]
func (h *Handler) ListCollectionDownloadCodeRedemptions(c *gin.Context) {
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

	base := h.db.Table("ops.collection_download_redemptions AS r").
		Select(`
r.id,
r.code_id,
COALESCE(c.code_mask, '') AS code_mask,
r.user_id,
COALESCE(u.display_name, '') AS user_display_name,
COALESCE(u.phone, '') AS user_phone,
r.collection_id,
COALESCE(col.title, '') AS collection_title,
r.granted_download_times,
r.expires_at,
r.ip,
r.user_agent,
r.created_at
		`).
		Joins("LEFT JOIN ops.collection_download_codes c ON c.id = r.code_id").
		Joins(`LEFT JOIN "user".users u ON u.id = r.user_id`).
		Joins("LEFT JOIN archive.collections col ON col.id = r.collection_id").
		Where("r.code_id = ?", codeID)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []collectionDownloadRedemptionQueryRow
	if err := base.Order("r.id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]CollectionDownloadRedemptionRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, CollectionDownloadRedemptionRecord{
			ID:                   row.ID,
			CodeID:               row.CodeID,
			CodeMask:             row.CodeMask,
			UserID:               row.UserID,
			UserDisplayName:      row.UserDisplayName,
			UserPhone:            row.UserPhone,
			CollectionID:         row.CollectionID,
			CollectionTitle:      row.CollectionTitle,
			GrantedDownloadTimes: row.GrantedDownloadTimes,
			ExpiresAt:            row.ExpiresAt,
			IP:                   row.IP,
			UserAgent:            row.UserAgent,
			CreatedAt:            row.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, CollectionDownloadRedemptionListResponse{Items: items, Total: total})
}

// ValidateCollectionDownloadCodeForMe godoc
// @Summary Validate collection download code card
// @Tags auth
// @Accept json
// @Produce json
// @Router /api/me/collection-download-code/validate [post]
func (h *Handler) ValidateCollectionDownloadCodeForMe(c *gin.Context) {
	user, ok := h.requireActiveUser(c)
	if !ok {
		return
	}
	if user == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin_not_supported"})
		return
	}
	if !h.guardRedeemValidate(c, user.ID) {
		return
	}
	deviceID := sanitizeDeviceID(c.GetHeader("X-Device-ID"), c.ClientIP(), c.GetHeader("User-Agent"))
	if !h.enforceRiskBlock(c, "redeem", "", deviceID, user.ID) {
		return
	}

	var req CollectionDownloadCodeSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	canonical := normalizeCollectionDownloadCode(req.Code)
	if canonical == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code required"})
		return
	}

	resp := evaluateCollectionDownloadCodeForUser(h.db, user.ID, hashCollectionDownloadCode(canonical), time.Now())
	c.JSON(http.StatusOK, resp)
}

// RedeemCollectionDownloadCodeForMe godoc
// @Summary Redeem collection download code card
// @Tags auth
// @Accept json
// @Produce json
// @Router /api/me/collection-download-code/redeem [post]
func (h *Handler) RedeemCollectionDownloadCodeForMe(c *gin.Context) {
	user, ok := h.requireActiveUser(c)
	if !ok {
		return
	}
	if user == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin_not_supported"})
		return
	}
	if !h.guardRedeemSubmit(c, user.ID) {
		return
	}
	deviceID := sanitizeDeviceID(c.GetHeader("X-Device-ID"), c.ClientIP(), c.GetHeader("User-Agent"))
	if !h.enforceRiskBlock(c, "redeem", "", deviceID, user.ID) {
		return
	}

	var req CollectionDownloadCodeSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	canonical := normalizeCollectionDownloadCode(req.Code)
	if canonical == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code required"})
		return
	}
	codeHash := hashCollectionDownloadCode(canonical)
	now := time.Now()

	var finalCode models.CollectionDownloadCode
	var finalEntitlement models.CollectionDownloadEntitlement
	collectionTitle := ""

	err := h.db.Transaction(func(tx *gorm.DB) error {
		var code models.CollectionDownloadCode
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("code_hash = ?", codeHash).
			First(&code).Error; err != nil {
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
		if code.EndsAt != nil && !code.EndsAt.After(now) {
			_ = tx.Model(&models.CollectionDownloadCode{}).Where("id = ?", code.ID).Update("status", "expired").Error
			return fmt.Errorf("兑换码已过期")
		}
		if code.MaxRedeemUsers > 0 && code.UsedRedeemUsers >= code.MaxRedeemUsers {
			return fmt.Errorf("兑换码已被使用完")
		}

		collection, err := h.ensureCollectionDownloadCodeVisible(tx, code.CollectionID)
		if err != nil {
			return fmt.Errorf("该合集暂不可用")
		}
		collectionTitle = strings.TrimSpace(collection.Title)

		var already int64
		if err := tx.Model(&models.CollectionDownloadRedemption{}).
			Where("code_id = ? AND user_id = ?", code.ID, user.ID).
			Count(&already).Error; err != nil {
			return err
		}
		if already > 0 {
			return fmt.Errorf("你已使用过该兑换码")
		}

		var entitlement models.CollectionDownloadEntitlement
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND collection_id = ?", user.ID, code.CollectionID).
			First(&entitlement).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			entitlement = models.CollectionDownloadEntitlement{
				UserID:                 user.ID,
				CollectionID:           code.CollectionID,
				CodeID:                 &code.ID,
				GrantedDownloadTimes:   code.GrantedDownloadTimes,
				UsedDownloadTimes:      0,
				RemainingDownloadTimes: code.GrantedDownloadTimes,
				Status:                 "active",
				ExpiresAt:              code.EndsAt,
			}
			if err := tx.Create(&entitlement).Error; err != nil {
				return err
			}
		} else {
			entitlement.CodeID = &code.ID
			entitlement.GrantedDownloadTimes += code.GrantedDownloadTimes
			entitlement.RemainingDownloadTimes += code.GrantedDownloadTimes
			if code.EndsAt != nil {
				if entitlement.ExpiresAt == nil || entitlement.ExpiresAt.Before(*code.EndsAt) {
					exp := *code.EndsAt
					entitlement.ExpiresAt = &exp
				}
			}
			if entitlement.RemainingDownloadTimes > 0 && strings.ToLower(strings.TrimSpace(entitlement.Status)) != "disabled" {
				entitlement.Status = "active"
			}
			if err := tx.Save(&entitlement).Error; err != nil {
				return err
			}
		}

		redemption := models.CollectionDownloadRedemption{
			CodeID:               code.ID,
			UserID:               user.ID,
			CollectionID:         code.CollectionID,
			GrantedDownloadTimes: code.GrantedDownloadTimes,
			ExpiresAt:            code.EndsAt,
			IP:                   strings.TrimSpace(c.ClientIP()),
			UserAgent:            strings.TrimSpace(c.GetHeader("User-Agent")),
		}
		if err := tx.Create(&redemption).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{
			"used_redeem_users": gorm.Expr("used_redeem_users + 1"),
			"updated_at":        now,
		}
		if code.MaxRedeemUsers > 0 && code.UsedRedeemUsers+1 >= code.MaxRedeemUsers {
			updates["status"] = "expired"
		}
		if err := tx.Model(&models.CollectionDownloadCode{}).Where("id = ?", code.ID).Updates(updates).Error; err != nil {
			return err
		}

		if err := tx.First(&finalCode, code.ID).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND collection_id = ?", user.ID, code.CollectionID).First(&finalEntitlement).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, CollectionDownloadCodeRedeemResponse{
		Message:                "兑换成功，合集下载次数已到账",
		CodeMask:               finalCode.CodeMask,
		CollectionID:           finalCode.CollectionID,
		CollectionTitle:        collectionTitle,
		GrantedDownloadTimes:   finalCode.GrantedDownloadTimes,
		RemainingDownloadTimes: finalEntitlement.RemainingDownloadTimes,
		ExpiresAt:              finalEntitlement.ExpiresAt,
	})
}

// ListMyCollectionDownloadEntitlements godoc
// @Summary List my collection download entitlements
// @Tags auth
// @Produce json
// @Router /api/me/collection-download-entitlements [get]
func (h *Handler) ListMyCollectionDownloadEntitlements(c *gin.Context) {
	user, ok := h.requireActiveUser(c)
	if !ok {
		return
	}
	if user == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin_not_supported"})
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
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	now := time.Now()
	if err := h.normalizeCollectionDownloadEntitlementStatuses(now, user.ID, 0); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	base := h.db.Table("ops.collection_download_entitlements AS e").
		Select(`
e.id,
e.user_id,
COALESCE(u.display_name, '') AS user_display_name,
COALESCE(u.phone, '') AS user_phone,
e.collection_id,
COALESCE(c.title, '') AS collection_title,
e.code_id,
e.granted_download_times,
e.used_download_times,
e.remaining_download_times,
e.status,
e.expires_at,
e.last_consumed_at,
e.updated_at
		`).
		Joins(`LEFT JOIN "user".users u ON u.id = e.user_id`).
		Joins("LEFT JOIN archive.collections c ON c.id = e.collection_id").
		Where("e.user_id = ?", user.ID)
	if status != "" {
		base = base.Where("e.status = ?", status)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []collectionDownloadEntitlementQueryRow
	if err := base.Order("e.id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]CollectionDownloadEntitlementItem, 0, len(rows))
	for _, row := range rows {
		row.Status = normalizeCollectionDownloadEntitlementRuntimeStatus(row.Status, row.RemainingDownloadTimes, row.ExpiresAt, now)
		if status != "" && row.Status != status {
			continue
		}
		items = append(items, mapCollectionDownloadEntitlementRow(row))
	}
	c.JSON(http.StatusOK, CollectionDownloadEntitlementListResponse{Items: items, Total: total})
}

// ListMyCollectionDownloadRedeemRecords godoc
// @Summary List my collection card redeem records
// @Tags auth
// @Produce json
// @Router /api/me/collection-download-redeem-records [get]
func (h *Handler) ListMyCollectionDownloadRedeemRecords(c *gin.Context) {
	user, ok := h.requireActiveUser(c)
	if !ok {
		return
	}
	if user == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin_not_supported"})
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
	collectionID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("collection_id")), 10, 64)

	base := h.db.Table("ops.collection_download_redemptions AS r").
		Select(`
r.id,
r.code_id,
COALESCE(c.code_mask, '') AS code_mask,
r.user_id,
COALESCE(u.display_name, '') AS user_display_name,
COALESCE(u.phone, '') AS user_phone,
r.collection_id,
COALESCE(col.title, '') AS collection_title,
r.granted_download_times,
r.expires_at,
r.ip,
r.user_agent,
r.created_at
		`).
		Joins("LEFT JOIN ops.collection_download_codes c ON c.id = r.code_id").
		Joins(`LEFT JOIN "user".users u ON u.id = r.user_id`).
		Joins("LEFT JOIN archive.collections col ON col.id = r.collection_id").
		Where("r.user_id = ?", user.ID)
	if collectionID > 0 {
		base = base.Where("r.collection_id = ?", collectionID)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []collectionDownloadRedemptionQueryRow
	if err := base.Order("r.id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]CollectionDownloadRedemptionRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, CollectionDownloadRedemptionRecord{
			ID:                   row.ID,
			CodeID:               row.CodeID,
			CodeMask:             row.CodeMask,
			UserID:               row.UserID,
			UserDisplayName:      row.UserDisplayName,
			UserPhone:            row.UserPhone,
			CollectionID:         row.CollectionID,
			CollectionTitle:      row.CollectionTitle,
			GrantedDownloadTimes: row.GrantedDownloadTimes,
			ExpiresAt:            row.ExpiresAt,
			IP:                   row.IP,
			UserAgent:            row.UserAgent,
			CreatedAt:            row.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, CollectionDownloadRedemptionListResponse{Items: items, Total: total})
}

// ListAdminCollectionDownloadEntitlements godoc
// @Summary List collection download entitlements for admin
// @Tags admin
// @Produce json
// @Router /api/admin/collection-download-entitlements [get]
func (h *Handler) ListAdminCollectionDownloadEntitlements(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	userID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("user_id")), 10, 64)
	collectionID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("collection_id")), 10, 64)
	q := strings.TrimSpace(c.Query("q"))
	now := time.Now()
	if err := h.normalizeCollectionDownloadEntitlementStatuses(now, userID, collectionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	base := h.db.Table("ops.collection_download_entitlements AS e").
		Select(`
e.id,
e.user_id,
COALESCE(u.display_name, '') AS user_display_name,
COALESCE(u.phone, '') AS user_phone,
e.collection_id,
COALESCE(c.title, '') AS collection_title,
e.code_id,
e.granted_download_times,
e.used_download_times,
e.remaining_download_times,
e.status,
e.expires_at,
e.last_consumed_at,
e.updated_at
		`).
		Joins(`LEFT JOIN "user".users u ON u.id = e.user_id`).
		Joins("LEFT JOIN archive.collections c ON c.id = e.collection_id")
	if status != "" {
		base = base.Where("e.status = ?", status)
	}
	if userID > 0 {
		base = base.Where("e.user_id = ?", userID)
	}
	if collectionID > 0 {
		base = base.Where("e.collection_id = ?", collectionID)
	}
	if q != "" {
		base = base.Where("u.display_name ILIKE ? OR u.phone ILIKE ? OR c.title ILIKE ?", "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rows []collectionDownloadEntitlementQueryRow
	if err := base.Order("e.id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]CollectionDownloadEntitlementItem, 0, len(rows))
	for _, row := range rows {
		row.Status = normalizeCollectionDownloadEntitlementRuntimeStatus(row.Status, row.RemainingDownloadTimes, row.ExpiresAt, now)
		if status != "" && row.Status != status {
			continue
		}
		items = append(items, mapCollectionDownloadEntitlementRow(row))
	}
	c.JSON(http.StatusOK, CollectionDownloadEntitlementListResponse{Items: items, Total: total})
}

// UpdateAdminCollectionDownloadEntitlement godoc
// @Summary Adjust collection download entitlement
// @Tags admin
// @Accept json
// @Produce json
// @Router /api/admin/collection-download-entitlements/{id} [put]
func (h *Handler) UpdateAdminCollectionDownloadEntitlement(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req UpdateCollectionDownloadEntitlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SetRemainingDownloadTimes == nil && req.DeltaRemainingDownloadTimes == nil && strings.TrimSpace(req.Status) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no update fields"})
		return
	}

	var result models.CollectionDownloadEntitlement
	var before models.CollectionDownloadEntitlement
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var row models.CollectionDownloadEntitlement
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		before = row

		remaining := row.RemainingDownloadTimes
		if req.SetRemainingDownloadTimes != nil {
			remaining = *req.SetRemainingDownloadTimes
		}
		if req.DeltaRemainingDownloadTimes != nil {
			remaining += *req.DeltaRemainingDownloadTimes
		}
		if remaining < 0 {
			return fmt.Errorf("remaining_download_times must be >= 0")
		}
		row.RemainingDownloadTimes = remaining

		if row.GrantedDownloadTimes < row.UsedDownloadTimes+row.RemainingDownloadTimes {
			row.GrantedDownloadTimes = row.UsedDownloadTimes + row.RemainingDownloadTimes
		}

		if statusRaw := strings.TrimSpace(req.Status); statusRaw != "" {
			status, ok := normalizeCollectionEntitlementStatus(statusRaw)
			if !ok {
				return fmt.Errorf("invalid status")
			}
			row.Status = status
		}

		if row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now()) {
			row.Status = "expired"
		} else if row.RemainingDownloadTimes <= 0 && strings.ToLower(strings.TrimSpace(row.Status)) != "disabled" {
			row.Status = "exhausted"
		} else if row.RemainingDownloadTimes > 0 && strings.ToLower(strings.TrimSpace(row.Status)) == "exhausted" {
			row.Status = "active"
		}

		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		result = row
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "entitlement not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	adminUID := h.extractUserID(c)
	h.recordAuditLog(adminUID, "collection_download_entitlement", result.ID, "admin_adjust_collection_download_entitlement", map[string]interface{}{
		"request": map[string]interface{}{
			"set_remaining_download_times":   req.SetRemainingDownloadTimes,
			"delta_remaining_download_times": req.DeltaRemainingDownloadTimes,
			"status":                         strings.TrimSpace(req.Status),
		},
		"before": collectionDownloadEntitlementSnapshot(before),
		"after":  collectionDownloadEntitlementSnapshot(result),
	})

	c.JSON(http.StatusOK, gin.H{
		"id":                       result.ID,
		"status":                   result.Status,
		"granted_download_times":   result.GrantedDownloadTimes,
		"used_download_times":      result.UsedDownloadTimes,
		"remaining_download_times": result.RemainingDownloadTimes,
		"expires_at":               result.ExpiresAt,
		"updated_at":               result.UpdatedAt,
	})
}
