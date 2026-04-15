package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"emoji/internal/config"
)

type Client struct {
	appID     string
	appSecret string
	baseURL   string
	http      *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

type apiEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type tenantTokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int64  `json:"expire"`
}

func NewClient(cfg config.Config) *Client {
	base := strings.TrimSpace(cfg.FeishuOpenBaseURL)
	if base == "" {
		base = "https://open.feishu.cn"
	}
	return &Client{
		appID:     strings.TrimSpace(cfg.FeishuAppID),
		appSecret: strings.TrimSpace(cfg.FeishuAppSecret),
		baseURL:   strings.TrimRight(base, "/"),
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Enabled() bool {
	return strings.TrimSpace(c.appID) != "" && strings.TrimSpace(c.appSecret) != ""
}

func (c *Client) TenantAccessToken(ctx context.Context) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("feishu app credentials not configured")
	}

	c.mu.Lock()
	if strings.TrimSpace(c.token) != "" && time.Now().Before(c.expiresAt.Add(-30*time.Second)) {
		token := c.token
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	requestBody := map[string]string{
		"app_id":     c.appID,
		"app_secret": c.appSecret,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}
	urlStr := c.baseURL + "/open-apis/auth/v3/tenant_access_token/internal"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "emoji-feishu-bot/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("tenant_access_token request failed: status=%d body=%s", resp.StatusCode, truncateForLog(string(respBody), 600))
	}

	var decoded tenantTokenResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return "", err
	}
	if decoded.Code != 0 {
		return "", fmt.Errorf("tenant_access_token request failed: code=%d msg=%s", decoded.Code, strings.TrimSpace(decoded.Msg))
	}
	if strings.TrimSpace(decoded.TenantAccessToken) == "" {
		return "", fmt.Errorf("tenant_access_token missing in response")
	}

	expireSec := decoded.Expire
	if expireSec <= 0 {
		expireSec = 7200
	}

	c.mu.Lock()
	c.token = strings.TrimSpace(decoded.TenantAccessToken)
	c.expiresAt = time.Now().Add(time.Duration(expireSec) * time.Second)
	token := c.token
	c.mu.Unlock()
	return token, nil
}

func (c *Client) SendTextMessageToChat(ctx context.Context, chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return fmt.Errorf("chat_id is required")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("text is required")
	}

	token, err := c.TenantAccessToken(ctx)
	if err != nil {
		return err
	}

	contentBytes, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}

	requestBody := map[string]interface{}{
		"receive_id": chatID,
		"msg_type":   "text",
		"content":    string(contentBytes),
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}
	urlStr := c.baseURL + "/open-apis/im/v1/messages?receive_id_type=chat_id"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "emoji-feishu-bot/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("send message failed: status=%d body=%s", resp.StatusCode, truncateForLog(string(respBody), 600))
	}

	var decoded apiEnvelope
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return err
	}
	if decoded.Code != 0 {
		return fmt.Errorf("send message failed: code=%d msg=%s", decoded.Code, strings.TrimSpace(decoded.Msg))
	}
	return nil
}

func (c *Client) DownloadMessageResource(
	ctx context.Context,
	messageID string,
	fileKey string,
	resourceTypes []string,
) (*http.Response, string, error) {
	messageID = strings.TrimSpace(messageID)
	fileKey = strings.TrimSpace(fileKey)
	if messageID == "" || fileKey == "" {
		return nil, "", fmt.Errorf("message_id and file_key are required")
	}

	token, err := c.TenantAccessToken(ctx)
	if err != nil {
		return nil, "", err
	}

	tryTypes := normalizeResourceTypes(resourceTypes)
	var lastErr error
	for _, resourceType := range tryTypes {
		u := c.baseURL + path.Join("/open-apis/im/v1/messages", url.PathEscape(messageID), "resources", url.PathEscape(fileKey)) + "?type=" + url.QueryEscape(resourceType)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "emoji-feishu-bot/1.0")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, resourceType, nil
		}

		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		lastErr = fmt.Errorf("download resource failed: type=%s status=%d body=%s", resourceType, resp.StatusCode, truncateForLog(string(body), 600))
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("download resource failed")
	}
	return nil, "", lastErr
}

func normalizeResourceTypes(in []string) []string {
	out := make([]string, 0, len(in)+2)
	seen := map[string]struct{}{}
	appendOne := func(raw string) {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, item := range in {
		appendOne(item)
	}
	appendOne("file")
	appendOne("media")
	if len(out) == 0 {
		return []string{"file", "media"}
	}
	return out
}

func truncateForLog(raw string, limit int) string {
	raw = strings.TrimSpace(raw)
	if limit <= 0 || len(raw) <= limit {
		return raw
	}
	return raw[:limit] + "..."
}
