package handlers

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const captchaAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

var deviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

type CaptchaResponse struct {
	CaptchaToken  string `json:"captcha_token"`
	CaptchaSVG    string `json:"captcha_svg"`
	CaptchaLength int    `json:"captcha_length"`
	ExpiresIn     int64  `json:"expires_in"`
}

func sanitizeDeviceID(raw, ip, userAgent string) string {
	trimmed := strings.TrimSpace(raw)
	if deviceIDPattern.MatchString(trimmed) {
		return trimmed
	}
	fallback := strings.TrimSpace(ip) + "|" + strings.TrimSpace(userAgent)
	if fallback == "|" || fallback == "" {
		fallback = "unknown-device"
	}
	sum := sha256.Sum256([]byte(fallback))
	return "fp_" + hex.EncodeToString(sum[:])[:24]
}

func (h *Handler) GetCaptcha(c *gin.Context) {
	if h.smsLimiter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "captcha unavailable"})
		return
	}
	ip := strings.TrimSpace(c.ClientIP())
	deviceID := sanitizeDeviceID(c.Query("device_id"), ip, c.GetHeader("User-Agent"))
	ctx := c.Request.Context()

	if ip != "" {
		if ok, err := h.smsLimiter.AllowInterval(ctx, "captcha:interval:ip:"+ip, 2*time.Second); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "captcha unavailable"})
			return
		} else if !ok {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}
	}
	if deviceID != "" {
		if ok, err := h.smsLimiter.AllowInterval(ctx, "captcha:interval:device:"+deviceID, 2*time.Second); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "captcha unavailable"})
			return
		} else if !ok {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}
	}

	resp, err := h.issueCaptcha(ctx, deviceID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "captcha unavailable"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) issueCaptcha(ctx context.Context, deviceID string) (CaptchaResponse, error) {
	length := h.cfg.CaptchaLength
	if length < 4 {
		length = 4
	}
	if length > 6 {
		length = 6
	}
	ttlSec := h.cfg.CaptchaTTLSeconds
	if ttlSec < 60 {
		ttlSec = 60
	}
	ttl := time.Duration(ttlSec) * time.Second

	code, err := randomCodeChars(length)
	if err != nil {
		return CaptchaResponse{}, err
	}
	token, err := randomHex(16)
	if err != nil {
		return CaptchaResponse{}, err
	}
	payload := strings.ToUpper(strings.TrimSpace(deviceID)) + "|" + strings.ToUpper(code)
	if err := h.smsLimiter.SetValue(ctx, "captcha:challenge:"+token, payload, ttl); err != nil {
		return CaptchaResponse{}, err
	}
	return CaptchaResponse{
		CaptchaToken:  token,
		CaptchaSVG:    buildCaptchaSVG(code),
		CaptchaLength: length,
		ExpiresIn:     int64(ttl.Seconds()),
	}, nil
}

func (h *Handler) verifyCaptchaChallenge(ctx context.Context, deviceID, token, answer string) (bool, error) {
	token = strings.TrimSpace(token)
	answer = strings.ToUpper(strings.TrimSpace(answer))
	deviceID = strings.ToUpper(strings.TrimSpace(deviceID))
	if token == "" || answer == "" || deviceID == "" {
		return false, nil
	}
	key := "captcha:challenge:" + token
	raw, err := h.smsLimiter.GetValue(ctx, key)
	if err != nil {
		return false, err
	}
	if raw == "" {
		return false, nil
	}
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 {
		_ = h.smsLimiter.Delete(ctx, key)
		return false, nil
	}
	storedDevice := strings.TrimSpace(parts[0])
	storedAnswer := strings.TrimSpace(parts[1])
	if storedDevice != deviceID || storedAnswer != answer {
		failKey := "captcha:fail:" + token
		if count, e := h.smsLimiter.IncrWithTTL(ctx, failKey, 10*time.Minute); e == nil && count >= 5 {
			_ = h.smsLimiter.Delete(ctx, key)
			_ = h.smsLimiter.Delete(ctx, failKey)
		}
		return false, nil
	}
	_ = h.smsLimiter.Delete(ctx, key)
	_ = h.smsLimiter.Delete(ctx, "captcha:fail:"+token)
	return true, nil
}

func (h *Handler) allowUniqueDailyCount(ctx context.Context, seenKey, countKey string, limit int, ttl time.Duration) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	firstSeen, err := h.smsLimiter.AllowInterval(ctx, seenKey, ttl)
	if err != nil {
		return false, err
	}
	if !firstSeen {
		return true, nil
	}
	return h.smsLimiter.AllowDaily(ctx, countKey, limit, ttl)
}

func (h *Handler) guardRegisterCreate(c *gin.Context, deviceID string) (bool, error) {
	if h.smsLimiter == nil {
		return false, fmt.Errorf("rate limiter unavailable")
	}
	now := time.Now()
	ttl := ttlUntilEndOfDay(now)
	ctx := c.Request.Context()
	ip := strings.TrimSpace(c.ClientIP())

	if h.cfg.RegisterDailyMaxIP > 0 && ip != "" {
		key := dayKey("register:daily:ip", ip, now)
		ok, err := h.smsLimiter.AllowDaily(ctx, key, h.cfg.RegisterDailyMaxIP, ttl)
		if err != nil || !ok {
			return ok, err
		}
	}
	if h.cfg.RegisterDailyMaxDevice > 0 && deviceID != "" {
		key := dayKey("register:daily:device", deviceID, now)
		ok, err := h.smsLimiter.AllowDaily(ctx, key, h.cfg.RegisterDailyMaxDevice, ttl)
		if err != nil || !ok {
			return ok, err
		}
	}
	return true, nil
}

func (h *Handler) isAuthVerifyLocked(ctx context.Context, phone, ip, deviceID string) (bool, error) {
	keys := []string{
		authLockKey("phone", phone),
		authLockKey("ip", ip),
		authLockKey("device", deviceID),
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		val, err := h.smsLimiter.GetValue(ctx, key)
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(val) != "" {
			return true, nil
		}
	}
	return false, nil
}

func (h *Handler) markAuthVerifyFailure(ctx context.Context, phone, ip, deviceID string) error {
	if h.smsLimiter == nil {
		return fmt.Errorf("rate limiter unavailable")
	}
	window := time.Duration(maxInt(h.cfg.AuthFailWindowSeconds, 60)) * time.Second
	level1 := maxInt(h.cfg.AuthFailLockLevel1, 5)
	level2 := maxInt(h.cfg.AuthFailLockLevel2, level1+1)
	lockTTL1 := time.Duration(maxInt(h.cfg.AuthFailLockTTL1, 600)) * time.Second
	lockTTL2 := time.Duration(maxInt(h.cfg.AuthFailLockTTL2, 3600)) * time.Second

	items := []struct {
		scope string
		value string
	}{
		{scope: "phone", value: phone},
		{scope: "ip", value: ip},
		{scope: "device", value: deviceID},
	}

	for _, item := range items {
		value := strings.TrimSpace(item.value)
		if value == "" {
			continue
		}
		failKey := authFailKey(item.scope, value)
		count, err := h.smsLimiter.IncrWithTTL(ctx, failKey, window)
		if err != nil {
			return err
		}
		if count >= int64(level2) {
			if err := h.smsLimiter.SetValue(ctx, authLockKey(item.scope, value), strconv.Itoa(level2), lockTTL2); err != nil {
				return err
			}
		} else if count >= int64(level1) {
			if err := h.smsLimiter.SetValue(ctx, authLockKey(item.scope, value), strconv.Itoa(level1), lockTTL1); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) clearAuthVerifyFailures(ctx context.Context, phone, ip, deviceID string) error {
	if h.smsLimiter == nil {
		return fmt.Errorf("rate limiter unavailable")
	}
	items := []struct {
		scope string
		value string
	}{
		{scope: "phone", value: phone},
		{scope: "ip", value: ip},
		{scope: "device", value: deviceID},
	}
	for _, item := range items {
		value := strings.TrimSpace(item.value)
		if value == "" {
			continue
		}
		_ = h.smsLimiter.Delete(ctx, authFailKey(item.scope, value))
		_ = h.smsLimiter.Delete(ctx, authLockKey(item.scope, value))
	}
	return nil
}

func authFailKey(scope, value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	return "risk:auth:fail:" + scope + ":" + v
}

func authLockKey(scope, value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	return "risk:auth:lock:" + scope + ":" + v
}

func randomCodeChars(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}
	var sb strings.Builder
	sb.Grow(length)
	max := big.NewInt(int64(len(captchaAlphabet)))
	for i := 0; i < length; i++ {
		n, err := crand.Int(crand.Reader, max)
		if err != nil {
			return "", err
		}
		sb.WriteByte(captchaAlphabet[n.Int64()])
	}
	return sb.String(), nil
}

func randomHex(size int) (string, error) {
	if size <= 0 {
		size = 16
	}
	buf := make([]byte, size)
	if _, err := crand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func cryptoRandInt(max int) int {
	if max <= 1 {
		return 0
	}
	n, err := crand.Int(crand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return int(time.Now().UnixNano() % int64(max))
	}
	return int(n.Int64())
}

func buildCaptchaSVG(code string) string {
	width := 132
	height := 44
	bgNoise := 6
	var sb strings.Builder
	sb.Grow(512)
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height))
	sb.WriteString(`<rect width="100%" height="100%" rx="8" fill="#F8FAFC"/>`)
	for i := 0; i < bgNoise; i++ {
		x1 := cryptoRandInt(width)
		y1 := cryptoRandInt(height)
		x2 := cryptoRandInt(width)
		y2 := cryptoRandInt(height)
		opacity := 20 + cryptoRandInt(25)
		sb.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#94A3B8" stroke-opacity="0.%d" stroke-width="1"/>`, x1, y1, x2, y2, opacity))
	}
	for i, r := range code {
		x := 16 + i*26 + cryptoRandInt(4)
		y := 30 + cryptoRandInt(4)
		rotate := cryptoRandInt(20) - 10
		fill := "#0F172A"
		if i%2 == 1 {
			fill = "#1D4ED8"
		}
		sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" transform="rotate(%d %d %d)" font-size="24" font-family="Arial, sans-serif" font-weight="700" fill="%s">%s</text>`,
			x, y, rotate, x, y, fill, html.EscapeString(string(r)),
		))
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

func maxInt(a, b int) int {
	if a >= b {
		return a
	}
	return b
}
