package handlers

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type actionRateWindow struct {
	Key       string
	UserLimit int
	IPLimit   int
	TTL       time.Duration
}

func (h *Handler) enforceActionRateLimits(c *gin.Context, action string, userID uint64, windows []actionRateWindow) bool {
	if c == nil || len(windows) == 0 || h.smsLimiter == nil {
		return true
	}

	ctx := c.Request.Context()
	ipKeyPart := sanitizeRateKeyPart(c.ClientIP())

	for _, window := range windows {
		if (window.UserLimit <= 0 && window.IPLimit <= 0) || window.TTL <= 0 {
			continue
		}

		if userID > 0 && window.UserLimit > 0 {
			userKey := fmt.Sprintf("abuse:%s:user:%d:%s", action, userID, window.Key)
			ok, err := h.smsLimiter.AllowDaily(ctx, userKey, window.UserLimit, window.TTL)
			if err != nil {
				// Fail open to avoid taking core APIs down when Redis is unstable.
				log.Printf("action rate limit skipped (user): action=%s user_id=%d key=%s err=%v", action, userID, window.Key, err)
			} else if !ok {
				h.writeRateLimitError(c, action, window.TTL)
				return false
			}
		}

		if ipKeyPart != "" && window.IPLimit > 0 {
			ipKey := fmt.Sprintf("abuse:%s:ip:%s:%s", action, ipKeyPart, window.Key)
			ok, err := h.smsLimiter.AllowDaily(ctx, ipKey, window.IPLimit, window.TTL)
			if err != nil {
				log.Printf("action rate limit skipped (ip): action=%s ip=%s key=%s err=%v", action, ipKeyPart, window.Key, err)
			} else if !ok {
				h.writeRateLimitError(c, action, window.TTL)
				return false
			}
		}
	}

	return true
}

func (h *Handler) guardEmojiDownload(c *gin.Context, userID uint64) bool {
	return h.enforceActionRateLimits(c, "emoji_download", userID, []actionRateWindow{
		{Key: "per_min", UserLimit: h.cfg.DownloadEmojiPerMinuteUser, IPLimit: h.cfg.DownloadEmojiPerMinuteIP, TTL: time.Minute},
		{Key: "per_hour", UserLimit: h.cfg.DownloadEmojiPerHourUser, IPLimit: h.cfg.DownloadEmojiPerHourIP, TTL: time.Hour},
	})
}

func (h *Handler) guardCollectionDownload(c *gin.Context, userID uint64) bool {
	return h.enforceActionRateLimits(c, "collection_download", userID, []actionRateWindow{
		{Key: "per_hour", UserLimit: h.cfg.DownloadCollectionPerHourUser, IPLimit: h.cfg.DownloadCollectionPerHourIP, TTL: time.Hour},
		{Key: "per_day", UserLimit: h.cfg.DownloadCollectionPerDayUser, IPLimit: h.cfg.DownloadCollectionPerDayIP, TTL: 24 * time.Hour},
	})
}

func (h *Handler) guardRedeemValidate(c *gin.Context, userID uint64) bool {
	return h.enforceActionRateLimits(c, "redeem_validate", userID, []actionRateWindow{
		{Key: "per_10m", UserLimit: h.cfg.RedeemValidatePer10MinUser, IPLimit: h.cfg.RedeemValidatePer10MinIP, TTL: 10 * time.Minute},
	})
}

func (h *Handler) guardRedeemSubmit(c *gin.Context, userID uint64) bool {
	return h.enforceActionRateLimits(c, "redeem_submit", userID, []actionRateWindow{
		{Key: "per_10m", UserLimit: h.cfg.RedeemSubmitPer10MinUser, IPLimit: h.cfg.RedeemSubmitPer10MinIP, TTL: 10 * time.Minute},
	})
}

func sanitizeRateKeyPart(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return ""
	}
	replacer := strings.NewReplacer(":", "_", ".", "_", "/", "_", "\\", "_", " ", "_")
	return replacer.Replace(trimmed)
}

func (h *Handler) writeRateLimitError(c *gin.Context, action string, ttl time.Duration) {
	if c == nil {
		return
	}
	seconds := int(math.Ceil(ttl.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", strconv.Itoa(seconds))
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":   "too_many_requests",
		"action":  action,
		"message": "操作过于频繁，请稍后再试",
	})
	h.recordRiskEvent(c, "rate_limited", action, "ip", strings.TrimSpace(c.ClientIP()), "medium", "action rate limited", map[string]interface{}{
		"retry_after_sec": seconds,
	})
}
