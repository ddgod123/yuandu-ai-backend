package handlers

import (
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type downloadTicketPayload struct {
	Kind            string                  `json:"kind"`
	Key             string                  `json:"key,omitempty"`
	Name            string                  `json:"name,omitempty"`
	UserID          uint64                  `json:"user_id,omitempty"`
	IP              string                  `json:"ip,omitempty"`
	UAHash          string                  `json:"ua_hash,omitempty"`
	CollectionID    uint64                  `json:"collection_id,omitempty"`
	EmojiID         uint64                  `json:"emoji_id,omitempty"`
	EntitlementMode string                  `json:"entitlement_mode,omitempty"`
	ZipItems        []downloadTicketZipItem `json:"zip_items,omitempty"`
}

type downloadTicketZipItem struct {
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
}

func (h *Handler) issueDownloadTicket(c *gin.Context, key, name string, userID uint64) (string, int64, error) {
	return h.issueDownloadTicketWithPayload(c, downloadTicketPayload{
		Kind:   "qiniu_object",
		Key:    strings.TrimSpace(key),
		Name:   strings.TrimSpace(name),
		UserID: userID,
	})
}

func (h *Handler) issueDownloadTicketWithPayload(c *gin.Context, payload downloadTicketPayload) (string, int64, error) {
	if h.smsLimiter == nil {
		return "", 0, fmt.Errorf("rate limiter unavailable")
	}
	payload.Kind = strings.TrimSpace(payload.Kind)
	if payload.Kind == "" {
		payload.Kind = "qiniu_object"
	}
	payload.Key = strings.TrimSpace(payload.Key)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.EntitlementMode = strings.TrimSpace(payload.EntitlementMode)
	if payload.Kind == "qiniu_object" && payload.Key == "" {
		return "", 0, fmt.Errorf("empty key")
	}
	if payload.Kind == "collection_zip_aggregate" && len(payload.ZipItems) == 0 {
		return "", 0, fmt.Errorf("empty key")
	}

	ttlSec := h.cfg.DownloadTicketTTL
	if ttlSec <= 0 {
		ttlSec = 120
	}
	ttl := time.Duration(ttlSec) * time.Second
	token, err := randomTicketToken(20)
	if err != nil {
		return "", 0, err
	}

	if h.cfg.DownloadTicketBindIP {
		payload.IP = strings.TrimSpace(c.ClientIP())
	}
	if h.cfg.DownloadTicketBindUA {
		payload.UAHash = shortHash(strings.TrimSpace(c.GetHeader("User-Agent")))
	}
	raw, _ := json.Marshal(payload)

	if err := h.smsLimiter.SetValue(c.Request.Context(), "download:ticket:"+token, string(raw), ttl); err != nil {
		return "", 0, err
	}

	downloadURL := buildAbsoluteURL(c, "/api/download/ticket/"+token)
	return downloadURL, time.Now().Unix() + int64(ttlSec), nil
}

func (h *Handler) consumeDownloadTicket(c *gin.Context, token string) (*downloadTicketPayload, error) {
	if h.smsLimiter == nil {
		return nil, fmt.Errorf("rate limiter unavailable")
	}
	raw, err := h.smsLimiter.ConsumeValue(c.Request.Context(), "download:ticket:"+token)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var payload downloadTicketPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func (h *Handler) DownloadByTicket(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token"})
		return
	}
	if uid, ok := currentUserIDFromContext(c); ok && uid > 0 {
		if !h.enforceRiskBlock(c, "download", "", strings.TrimSpace(c.GetHeader("X-Device-ID")), uid) {
			return
		}
	} else if !h.enforceRiskBlock(c, "download", "", strings.TrimSpace(c.GetHeader("X-Device-ID")), 0) {
		return
	}
	payload, err := h.consumeDownloadTicket(c, token)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "download unavailable"})
		return
	}
	if payload == nil || strings.TrimSpace(payload.Key) == "" {
		h.recordRiskEvent(c, "download_ticket_expired", "download", "ip", strings.TrimSpace(c.ClientIP()), "low", "download ticket expired", map[string]interface{}{
			"token": token,
		})
		c.JSON(http.StatusGone, gin.H{"error": "ticket expired"})
		return
	}
	kind := strings.TrimSpace(payload.Kind)
	if kind == "" {
		kind = "qiniu_object"
	}
	if kind != "qiniu_object" && kind != "collection_zip_aggregate" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ticket"})
		return
	}

	if payload.UserID > 0 {
		if uid, ok := currentUserIDFromContext(c); ok && uid > 0 && uid != payload.UserID {
			h.recordRiskEvent(c, "download_ticket_owner_mismatch", "download", "user", strconv.FormatUint(uid, 10), "high", "download ticket owner mismatch", map[string]interface{}{
				"ticket_user_id": payload.UserID,
			})
			c.JSON(http.StatusForbidden, gin.H{"error": "ticket owner mismatch"})
			return
		}
	}
	if h.cfg.DownloadTicketBindIP {
		expected := strings.TrimSpace(payload.IP)
		actual := strings.TrimSpace(c.ClientIP())
		if expected != "" && actual != "" && expected != actual {
			h.recordRiskEvent(c, "download_ticket_ip_mismatch", "download", "ip", actual, "high", "download ticket ip mismatch", map[string]interface{}{
				"expected_ip": expected,
			})
			c.JSON(http.StatusForbidden, gin.H{"error": "ticket ip mismatch"})
			return
		}
	}
	if h.cfg.DownloadTicketBindUA {
		expected := strings.TrimSpace(payload.UAHash)
		actual := shortHash(strings.TrimSpace(c.GetHeader("User-Agent")))
		if expected != "" && actual != "" && expected != actual {
			h.recordRiskEvent(c, "download_ticket_ua_mismatch", "download", "ip", strings.TrimSpace(c.ClientIP()), "high", "download ticket ua mismatch", map[string]interface{}{
				"expected_ua_hash": expected,
			})
			c.JSON(http.StatusForbidden, gin.H{"error": "ticket ua mismatch"})
			return
		}
	}

	if !h.applyDownloadTicketAccounting(c, payload) {
		return
	}

	if kind == "collection_zip_aggregate" {
		h.streamCollectionZipAggregateByTicket(c, payload)
		return
	}

	signTTL := h.cfg.DownloadTicketSignTTL
	if signTTL <= 0 {
		signTTL = 180
	}
	rawURL, _ := resolveDownloadURL(payload.Key, h.qiniu, strconv.Itoa(signTTL))
	if rawURL == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "download url unavailable"})
		return
	}
	if h.qiniu != nil && !h.qiniu.Private {
		h.proxyPublicDownloadByTicket(c, payload.Key, rawURL, payload.Name)
		return
	}
	c.Redirect(http.StatusFound, rawURL)
}

func (h *Handler) resolveDownloadTicketUserID(c *gin.Context, payload *downloadTicketPayload) uint64 {
	if payload != nil && payload.UserID > 0 {
		return payload.UserID
	}
	if uid, ok := currentUserIDFromContext(c); ok && uid > 0 {
		return uid
	}
	return 0
}

func (h *Handler) applyDownloadTicketAccounting(c *gin.Context, payload *downloadTicketPayload) bool {
	if payload == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ticket"})
		return false
	}
	userID := h.resolveDownloadTicketUserID(c, payload)
	mode := strings.TrimSpace(payload.EntitlementMode)
	if mode != "" {
		if payload.CollectionID == 0 || userID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ticket"})
			return false
		}
		if err := h.consumeCollectionDownloadEntitlement(c, userID, payload.CollectionID, mode); err != nil {
			writeCollectionDownloadAccessDenied(c, collectionDownloadAccessDecision{
				Allowed:      false,
				IsSubscriber: false,
				DenyError:    err,
			})
			return false
		}
	}

	if payload.CollectionID > 0 {
		collectionID := payload.CollectionID
		h.recordCollectionDownload(c, payload.CollectionID, userID)
		success := true
		h.recordUserBehaviorEvent(c, "download_collection_ticket_consumed", userBehaviorEventOptions{
			UserID:       userID,
			CollectionID: &collectionID,
			Success:      &success,
			RequestID:    strings.TrimSpace(c.GetHeader("X-Request-ID")),
			Metadata: map[string]interface{}{
				"entitlement_mode": mode,
			},
		})
	}
	if payload.EmojiID > 0 {
		emojiID := payload.EmojiID
		h.recordEmojiDownload(c, payload.EmojiID, userID)
		success := true
		h.recordUserBehaviorEvent(c, "download_single_ticket_consumed", userBehaviorEventOptions{
			UserID:    userID,
			EmojiID:   &emojiID,
			Success:   &success,
			RequestID: strings.TrimSpace(c.GetHeader("X-Request-ID")),
		})
	}
	return true
}

func (h *Handler) proxyPublicDownloadByTicket(c *gin.Context, key, rawURL, name string) {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := downloadZipPart(client, rawURL)
	if err != nil {
		h.recordRiskEvent(c, "download_proxy_fetch_failed", "download", "ip", strings.TrimSpace(c.ClientIP()), "low", "download proxy fetch failed", map[string]interface{}{
			"key": key,
		})
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch file"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		h.recordRiskEvent(c, "download_proxy_upstream_failed", "download", "ip", strings.TrimSpace(c.ClientIP()), "low", "download proxy upstream failed", map[string]interface{}{
			"key":    key,
			"status": resp.StatusCode,
		})
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch file"})
		return
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	if resp.ContentLength > 0 {
		c.Header("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}

	fileName := strings.TrimSpace(name)
	if fileName == "" {
		fileName = path.Base(strings.TrimSpace(rawURL))
	}
	fileName = normalizeDownloadFileName(fileName, "download", path.Ext(fileName))
	if fileName != "" {
		asciiFallback := normalizeDownloadFileName("", "download", path.Ext(fileName))
		encodedFilename := strings.ReplaceAll(url.QueryEscape(fileName), "+", "%20")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", asciiFallback, encodedFilename))
	}

	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		h.recordRiskEvent(c, "download_proxy_copy_failed", "download", "ip", strings.TrimSpace(c.ClientIP()), "low", "download proxy copy failed", map[string]interface{}{
			"key": key,
		})
	}
}

func randomTicketToken(size int) (string, error) {
	if size <= 0 {
		size = 16
	}
	buf := make([]byte, size)
	if _, err := crand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func shortHash(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func buildAbsoluteURL(c *gin.Context, path string) string {
	if c == nil || c.Request == nil {
		return path
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if fwd := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); fwd != "" {
		parts := strings.Split(fwd, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			scheme = strings.TrimSpace(parts[0])
		}
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}
