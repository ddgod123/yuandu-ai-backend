package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type RegisterRequest struct {
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type RegisterPhoneRequest struct {
	Phone       string `json:"phone"`
	Code        string `json:"code"`
	DisplayName string `json:"display_name"`
}

type LoginPhoneRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

type SendCodeRequest struct {
	Phone string `json:"phone"`
}

type SendCodeResponse struct {
	Phone     string `json:"phone"`
	Code      string `json:"code"`
	ExpiresIn int64  `json:"expires_in"`
	Mock      bool   `json:"mock"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Bio         *string `json:"bio"`
	WebsiteURL  *string `json:"website_url"`
	Location    *string `json:"location"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type UserResponse struct {
	ID                    uint64     `json:"id"`
	Email                 string     `json:"email,omitempty"`
	Phone                 string     `json:"phone,omitempty"`
	DisplayName           string     `json:"display_name,omitempty"`
	AvatarURL             string     `json:"avatar_url,omitempty"`
	Role                  string     `json:"role"`
	Status                string     `json:"status"`
	SubscriptionStatus    string     `json:"subscription_status,omitempty"`
	SubscriptionPlan      string     `json:"subscription_plan,omitempty"`
	SubscriptionExpiresAt *time.Time `json:"subscription_expires_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
}

type AuthResponse struct {
	User   UserResponse  `json:"user"`
	Tokens TokenResponse `json:"tokens"`
}

type UpdateRoleRequest struct {
	Role string `json:"role"`
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

type AccessClaims struct {
	jwt.RegisteredClaims
	Role string `json:"role"`
}

// SendCode godoc
// @Summary Send phone verification code
// @Tags auth
// @Accept json
// @Produce json
// @Param body body SendCodeRequest true "send code"
// @Success 200 {object} SendCodeResponse
// @Router /api/auth/send-code [post]
func (h *Handler) SendCode(c *gin.Context) {
	var req SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone required"})
		return
	}

	if h.isDevAuthPhone(phone) {
		c.JSON(http.StatusOK, SendCodeResponse{
			Phone:     phone,
			Code:      h.firstDevAuthCode(),
			ExpiresIn: int64(h.cfg.AliyunSmsValidTime),
			Mock:      true,
		})
		return
	}

	if h.smsLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "rate limiter unavailable"})
		return
	}
	ctx := c.Request.Context()
	now := time.Now()
	ttl := ttlUntilEndOfDay(now)
	interval := time.Duration(h.cfg.AliyunSmsInterval) * time.Second
	ip := c.ClientIP()

	if ok, err := h.smsLimiter.AllowInterval(ctx, "sms:interval:phone:"+phone, interval); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "rate limiter unavailable"})
		return
	} else if !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
		return
	}
	if ip != "" {
		if ok, err := h.smsLimiter.AllowInterval(ctx, "sms:interval:ip:"+ip, interval); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "rate limiter unavailable"})
			return
		} else if !ok {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}
	}
	if h.cfg.AliyunSmsDailyMaxPhone > 0 {
		key := dayKey("sms:daily:phone", phone, now)
		if ok, err := h.smsLimiter.AllowDaily(ctx, key, h.cfg.AliyunSmsDailyMaxPhone, ttl); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "rate limiter unavailable"})
			return
		} else if !ok {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "daily limit reached"})
			return
		}
	}
	if ip != "" && h.cfg.AliyunSmsDailyMaxIP > 0 {
		key := dayKey("sms:daily:ip", ip, now)
		if ok, err := h.smsLimiter.AllowDaily(ctx, key, h.cfg.AliyunSmsDailyMaxIP, ttl); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "rate limiter unavailable"})
			return
		} else if !ok {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "daily limit reached"})
			return
		}
	}
	code, expires, mock, err := h.sendAliyunSMSCode(phone)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, SendCodeResponse{
		Phone:     phone,
		Code:      code,
		ExpiresIn: expires,
		Mock:      mock,
	})
}

// RegisterPhone godoc
// @Summary Register by phone
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RegisterPhoneRequest true "register phone"
// @Success 201 {object} AuthResponse
// @Router /api/auth/register-phone [post]
func (h *Handler) RegisterPhone(c *gin.Context) {
	var req RegisterPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Phone = strings.TrimSpace(req.Phone)
	if req.Phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone required"})
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code required"})
		return
	}

	ok, err := h.verifySMSCodeWithLimits(c, req.Phone, req.Code)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid code"})
		return
	}

	var exists int64
	h.db.Model(&models.User{}).Where("phone = ?", req.Phone).Count(&exists)
	if exists > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user already exists"})
		return
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = generateDisplayName()
	}

	now := time.Now()
	user := models.User{
		Phone:       req.Phone,
		DisplayName: displayName,
		AvatarURL:   defaultAvatarURL(req.Phone),
		Role:        "user",
		Status:      "active",
		VerifiedAt:  &now,
		LastLoginAt: &now,
		LastLoginIP: c.ClientIP(),
	}
	if err := h.db.Omit("Email", "Username").Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	tokens, err := h.issueTokens(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	if err := h.storeRefreshToken(user.ID, tokens.RefreshToken, h.cfg.RefreshTokenTTL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store refresh token"})
		return
	}
	h.setAuthCookies(c, tokens)

	c.JSON(http.StatusCreated, AuthResponse{
		User:   mapUser(user),
		Tokens: tokens,
	})
}

// LoginPhone godoc
// @Summary Login by phone
// @Tags auth
// @Accept json
// @Produce json
// @Param body body LoginPhoneRequest true "login phone"
// @Success 200 {object} AuthResponse
// @Router /api/auth/login-phone [post]
func (h *Handler) LoginPhone(c *gin.Context) {
	var req LoginPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Phone = strings.TrimSpace(req.Phone)
	if req.Phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone required"})
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code required"})
		return
	}

	ok, err := h.verifySMSCodeWithLimits(c, req.Phone, req.Code)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid code"})
		return
	}

	var user models.User
	if err := h.db.Where("phone = ?", req.Phone).First(&user).Error; err != nil {
		if h.isDevAuthPhone(req.Phone) && err == gorm.ErrRecordNotFound {
			displayName := generateDisplayName()
			now := time.Now()
			newUser := models.User{
				Phone:       req.Phone,
				DisplayName: displayName,
				AvatarURL:   defaultAvatarURL(req.Phone),
				Role:        "user",
				Status:      "active",
				VerifiedAt:  &now,
				LastLoginAt: &now,
				LastLoginIP: c.ClientIP(),
			}
			if err := h.db.Omit("Email", "Username").Create(&newUser).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
				return
			}
			user = newUser
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}
	}
	if user.Status != "active" {
		c.JSON(http.StatusForbidden, gin.H{"error": "user disabled"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"last_login_at": now,
		"last_login_ip": c.ClientIP(),
	}
	if user.VerifiedAt == nil {
		updates["verified_at"] = now
	}
	_ = h.db.Model(&user).Updates(updates).Error

	tokens, err := h.issueTokens(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	if err := h.storeRefreshToken(user.ID, tokens.RefreshToken, h.cfg.RefreshTokenTTL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store refresh token"})
		return
	}
	h.setAuthCookies(c, tokens)

	c.JSON(http.StatusOK, AuthResponse{
		User:   mapUser(user),
		Tokens: tokens,
	})
}

// Register godoc
// @Summary Register
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RegisterRequest true "register"
// @Success 201 {object} AuthResponse
// @Router /api/auth/register [post]
func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Phone = strings.TrimSpace(req.Phone)
	if req.Email == "" && req.Phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email or phone required"})
		return
	}
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password too short"})
		return
	}

	var exists int64
	q := h.db.Model(&models.User{})
	if req.Email != "" {
		q = q.Where("email = ?", req.Email)
	}
	if req.Phone != "" {
		q = q.Or("phone = ?", req.Phone)
	}
	q.Count(&exists)
	if exists > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user already exists"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user := models.User{
		Email:        req.Email,
		Phone:        req.Phone,
		PasswordHash: string(hash),
		DisplayName:  req.DisplayName,
		Role:         "user",
		Status:       "active",
	}
	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	tokens, err := h.issueTokens(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	if err := h.storeRefreshToken(user.ID, tokens.RefreshToken, h.cfg.RefreshTokenTTL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store refresh token"})
		return
	}
	h.setAuthCookies(c, tokens)

	c.JSON(http.StatusCreated, AuthResponse{
		User:   mapUser(user),
		Tokens: tokens,
	})
}

// Login godoc
// @Summary Login
// @Tags auth
// @Accept json
// @Produce json
// @Param body body LoginRequest true "login"
// @Success 200 {object} AuthResponse
// @Router /api/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Phone = strings.TrimSpace(req.Phone)
	if req.Email == "" && req.Phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email or phone required"})
		return
	}

	var user models.User
	q := h.db.Model(&models.User{})
	if req.Email != "" {
		q = q.Where("email = ?", req.Email)
	}
	if req.Phone != "" {
		q = q.Or("phone = ?", req.Phone)
	}
	if err := q.First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if user.Status != "active" {
		c.JSON(http.StatusForbidden, gin.H{"error": "user disabled"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	tokens, err := h.issueTokens(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	if err := h.storeRefreshToken(user.ID, tokens.RefreshToken, h.cfg.RefreshTokenTTL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store refresh token"})
		return
	}
	h.setAuthCookies(c, tokens)

	c.JSON(http.StatusOK, AuthResponse{
		User:   mapUser(user),
		Tokens: tokens,
	})
}

// Refresh godoc
// @Summary Refresh token
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RefreshRequest true "refresh"
// @Success 200 {object} TokenResponse
// @Router /api/auth/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.RefreshToken = h.refreshTokenFromRequestBodyOrCookie(c, req.RefreshToken)
	if req.RefreshToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh_token required"})
		return
	}

	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(req.RefreshToken, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	if !h.isRefreshTokenValid(req.RefreshToken) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token revoked"})
		return
	}

	userID, err := parseSubjectUint(claims.Subject)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	if err := h.revokeRefreshToken(req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke token"})
		return
	}

	tokens, err := h.issueTokens(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	if err := h.storeRefreshToken(user.ID, tokens.RefreshToken, h.cfg.RefreshTokenTTL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store refresh token"})
		return
	}
	h.setAuthCookies(c, tokens)

	c.JSON(http.StatusOK, tokens)
}

// Logout godoc
// @Summary Logout and revoke refresh token
// @Tags auth
// @Accept json
// @Produce json
// @Param body body LogoutRequest true "logout"
// @Success 200 {object} MessageResponse
// @Router /api/auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	var req LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.RefreshToken = h.refreshTokenFromRequestBodyOrCookie(c, req.RefreshToken)
	if req.RefreshToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token required"})
		return
	}
	if err := h.revokeRefreshToken(req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke token"})
		return
	}
	h.clearAuthCookies(c)
	c.JSON(http.StatusOK, MessageResponse{Message: "logged out"})
}

// Me godoc
// @Summary Current user
// @Tags auth
// @Produce json
// @Success 200 {object} UserResponse
// @Router /api/me [get]
func (h *Handler) Me(c *gin.Context) {
	uidAny, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	uid, ok := uidAny.(uint64)
	if !ok || uid == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, mapUser(user))
}

// UpdateMe godoc
// @Summary Update current user profile
// @Tags auth
// @Accept json
// @Produce json
// @Param body body UpdateProfileRequest true "profile"
// @Success 200 {object} UserResponse
// @Router /api/me [put]
func (h *Handler) UpdateMe(c *gin.Context) {
	uidAny, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	uid, ok := uidAny.(uint64)
	if !ok || uid == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if len(name) > 64 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "display_name too long"})
			return
		}
		updates["display_name"] = name
	}
	if req.AvatarURL != nil {
		avatar := strings.TrimSpace(*req.AvatarURL)
		if len(avatar) > 512 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "avatar_url too long"})
			return
		}
		updates["avatar_url"] = avatar
	}
	if req.Bio != nil {
		updates["bio"] = strings.TrimSpace(*req.Bio)
	}
	if req.WebsiteURL != nil {
		updates["website_url"] = strings.TrimSpace(*req.WebsiteURL)
	}
	if req.Location != nil {
		location := strings.TrimSpace(*req.Location)
		if len(location) > 64 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "location too long"})
			return
		}
		updates["location"] = location
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	if err := h.db.Model(&user).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		return
	}
	if err := h.db.First(&user, uid).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load profile"})
		return
	}

	c.JSON(http.StatusOK, mapUser(user))
}

// UpdateUserRole godoc
// @Summary Update user role
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "user id"
// @Param body body UpdateRoleRequest true "role"
// @Success 200 {object} UserResponse
// @Router /api/admin/users/{id}/role [put]
func (h *Handler) UpdateUserRole(c *gin.Context) {
	id := c.Param("id")
	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role := strings.TrimSpace(strings.ToLower(req.Role))
	if role == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role required"})
		return
	}

	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	user.Role = role
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update role"})
		return
	}

	c.JSON(http.StatusOK, mapUser(user))
}

// UpdateUserStatus godoc
// @Summary Update user status
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "user id"
// @Param body body UpdateStatusRequest true "status"
// @Success 200 {object} UserResponse
// @Router /api/admin/users/{id}/status [put]
func (h *Handler) UpdateUserStatus(c *gin.Context) {
	id := c.Param("id")
	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status := strings.TrimSpace(strings.ToLower(req.Status))
	if status == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status required"})
		return
	}
	if status != "active" && status != "disabled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	user.Status = status
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}

	c.JSON(http.StatusOK, mapUser(user))
}

// ListUsers godoc
// @Summary List users
// @Tags admin
// @Produce json
// @Param q query string false "keyword"
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} UsersListResponse
// @Router /api/admin/users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	q := strings.TrimSpace(c.Query("q"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	query := h.db.Model(&models.User{})
	if q != "" {
		query = query.Where("email ILIKE ? OR phone ILIKE ? OR display_name ILIKE ?", "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}

	var total int64
	query.Count(&total)

	var users []models.User
	query.Order("id DESC").Offset((page - 1) * limit).Limit(limit).Find(&users)

	items := make([]UserResponse, 0, len(users))
	for _, u := range users {
		items = append(items, mapUser(u))
	}

	c.JSON(http.StatusOK, UsersListResponse{Items: items, Total: total})
}

type UsersListResponse struct {
	Items []UserResponse `json:"items"`
	Total int64          `json:"total"`
}

func (h *Handler) issueTokens(user models.User) (TokenResponse, error) {
	now := time.Now()
	accessExp := now.Add(h.cfg.AccessTokenTTL)
	refreshExp := now.Add(h.cfg.RefreshTokenTTL)

	accessClaims := AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   formatSubject(user.ID),
			Issuer:    h.cfg.JWTIssuer,
			ExpiresAt: jwt.NewNumericDate(accessExp),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Role: user.Role,
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		return TokenResponse{}, err
	}

	refreshClaims := jwt.RegisteredClaims{
		Subject:   formatSubject(user.ID),
		Issuer:    h.cfg.JWTIssuer,
		ExpiresAt: jwt.NewNumericDate(refreshExp),
		IssuedAt:  jwt.NewNumericDate(now),
	}
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		return TokenResponse{}, err
	}

	return TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(h.cfg.AccessTokenTTL.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (h *Handler) storeRefreshToken(userID uint64, token string, ttl time.Duration) error {
	expiresAt := time.Now().Add(ttl)
	hash := hashToken(token)
	return h.db.Create(&models.RefreshToken{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	}).Error
}

func (h *Handler) isRefreshTokenValid(token string) bool {
	var rt models.RefreshToken
	hash := hashToken(token)
	if err := h.db.Where("token_hash = ? AND revoked_at IS NULL AND expires_at > NOW()", hash).First(&rt).Error; err != nil {
		return false
	}
	return true
}

func (h *Handler) revokeRefreshToken(token string) error {
	now := time.Now()
	hash := hashToken(token)
	return h.db.Model(&models.RefreshToken{}).Where("token_hash = ? AND revoked_at IS NULL", hash).
		Update("revoked_at", now).Error
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (h *Handler) isDevAuthPhone(phone string) bool {
	if h.cfg.Env == "prod" || !h.cfg.DevAuthEnabled {
		return false
	}
	needle := strings.TrimSpace(phone)
	if needle == "" {
		return false
	}
	raw := strings.TrimSpace(h.cfg.DevAuthPhone)
	if raw == "" {
		return false
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	for _, p := range parts {
		if strings.TrimSpace(p) == needle {
			return true
		}
	}
	return false
}

func (h *Handler) isDevAuthCode(code string) bool {
	if h.cfg.Env == "prod" || !h.cfg.DevAuthEnabled {
		return false
	}
	needle := strings.TrimSpace(code)
	if needle == "" {
		return false
	}
	for _, c := range h.devAuthCodes() {
		if c == needle {
			return true
		}
	}
	return false
}

func (h *Handler) devAuthCodes() []string {
	raw := strings.TrimSpace(h.cfg.DevAuthCode)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	codes := make([]string, 0, len(parts))
	for _, p := range parts {
		val := strings.TrimSpace(p)
		if val != "" {
			codes = append(codes, val)
		}
	}
	return codes
}

func (h *Handler) firstDevAuthCode() string {
	codes := h.devAuthCodes()
	if len(codes) == 0 {
		return ""
	}
	return codes[0]
}

func (h *Handler) verifySMSCodeWithLimits(c *gin.Context, phone, code string) (bool, error) {
	if h.isDevAuthPhone(phone) && h.isDevAuthCode(code) {
		return true, nil
	}
	if h.smsLimiter == nil {
		return false, fmt.Errorf("rate limiter unavailable")
	}
	ctx := c.Request.Context()
	now := time.Now()
	ttl := ttlUntilEndOfDay(now)
	ip := c.ClientIP()
	if h.cfg.LoginDailyMaxPhone > 0 {
		key := dayKey("login:daily:phone", phone, now)
		if ok, err := h.smsLimiter.AllowDaily(ctx, key, h.cfg.LoginDailyMaxPhone, ttl); err != nil {
			return false, fmt.Errorf("rate limiter unavailable")
		} else if !ok {
			return false, nil
		}
	}
	if ip != "" && h.cfg.LoginDailyMaxIP > 0 {
		key := dayKey("login:daily:ip", ip, now)
		if ok, err := h.smsLimiter.AllowDaily(ctx, key, h.cfg.LoginDailyMaxIP, ttl); err != nil {
			return false, fmt.Errorf("rate limiter unavailable")
		} else if !ok {
			return false, nil
		}
	}
	return h.verifyAliyunSMSCode(phone, code)
}

func dayKey(prefix, value string, now time.Time) string {
	return fmt.Sprintf("%s:%s:%s", prefix, value, now.Format("20060102"))
}

func ttlUntilEndOfDay(now time.Time) time.Duration {
	year, month, day := now.Date()
	end := time.Date(year, month, day+1, 0, 0, 0, 0, now.Location())
	return end.Sub(now)
}

func mapUser(user models.User) UserResponse {
	return UserResponse{
		ID:                    user.ID,
		Email:                 user.Email,
		Phone:                 user.Phone,
		DisplayName:           user.DisplayName,
		AvatarURL:             user.AvatarURL,
		Role:                  user.Role,
		Status:                user.Status,
		SubscriptionStatus:    user.SubscriptionStatus,
		SubscriptionPlan:      user.SubscriptionPlan,
		SubscriptionExpiresAt: user.SubscriptionExpiresAt,
		CreatedAt:             user.CreatedAt,
	}
}

func formatSubject(id uint64) string {
	return strconv.FormatUint(id, 10)
}

func parseSubjectUint(sub string) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(sub), 10, 64)
}
