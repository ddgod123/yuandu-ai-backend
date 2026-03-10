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
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	Name   string `json:"name"`
	UserID uint64 `json:"user_id,omitempty"`
	IP     string `json:"ip,omitempty"`
	UAHash string `json:"ua_hash,omitempty"`
}

func (h *Handler) issueDownloadTicket(c *gin.Context, key, name string, userID uint64) (string, int64, error) {
	if h.smsLimiter == nil {
		return "", 0, fmt.Errorf("rate limiter unavailable")
	}
	key = strings.TrimSpace(key)
	if key == "" {
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

	payload := downloadTicketPayload{
		Kind:   "qiniu_object",
		Key:    key,
		Name:   strings.TrimSpace(name),
		UserID: userID,
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
	if strings.TrimSpace(payload.Kind) != "qiniu_object" {
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
		h.proxyPublicDownloadByTicket(c, rawURL, payload.Name)
		return
	}
	c.Redirect(http.StatusFound, rawURL)
}

func (h *Handler) proxyPublicDownloadByTicket(c *gin.Context, rawURL, name string) {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := downloadZipPart(client, rawURL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch file"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
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
	_, _ = io.Copy(c.Writer, resp.Body)
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
