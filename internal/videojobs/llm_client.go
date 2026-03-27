package videojobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type openAICompatChatResponse struct {
	ID      string                     `json:"id,omitempty"`
	Model   string                     `json:"model,omitempty"`
	Choices []openAICompatChatChoice   `json:"choices"`
	Usage   openAICompatUsageEnvelope  `json:"usage"`
	Error   *openAICompatErrorEnvelope `json:"error,omitempty"`
}

type openAICompatChatChoice struct {
	Index        int                             `json:"index,omitempty"`
	Message      openAICompatChatResponseMessage `json:"message"`
	FinishReason string                          `json:"finish_reason,omitempty"`
}

type openAICompatChatResponseMessage struct {
	Role    string          `json:"role,omitempty"`
	Content json.RawMessage `json:"content"`
}

type openAICompatChatResponseContentPart struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

type openAICompatUsageEnvelope struct {
	PromptTokens       json.Number                      `json:"prompt_tokens,omitempty"`
	CompletionTokens   json.Number                      `json:"completion_tokens,omitempty"`
	InputTokens        json.Number                      `json:"input_tokens,omitempty"`
	OutputTokens       json.Number                      `json:"output_tokens,omitempty"`
	CachedTokens       json.Number                      `json:"cached_tokens,omitempty"`
	PromptCacheHit     json.Number                      `json:"prompt_cache_hit_tokens,omitempty"`
	ImageTokens        json.Number                      `json:"image_tokens,omitempty"`
	VideoTokens        json.Number                      `json:"video_tokens,omitempty"`
	AudioSeconds       json.Number                      `json:"audio_seconds,omitempty"`
	PromptTokenDetails *openAICompatPromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

type openAICompatPromptTokensDetails struct {
	CachedTokens json.Number `json:"cached_tokens,omitempty"`
}

type openAICompatErrorEnvelope struct {
	Message string      `json:"message,omitempty"`
	Type    string      `json:"type,omitempty"`
	Code    interface{} `json:"code,omitempty"`
}

func (p *Processor) callOpenAICompatJSONChat(
	ctx context.Context,
	cfg aiModelCallConfig,
	systemPrompt string,
	userPrompt string,
) (string, cloudHighlightUsage, map[string]interface{}, int64, error) {
	userParts := []openAICompatContentPart{
		{Type: "text", Text: userPrompt},
	}
	return p.callOpenAICompatJSONChatWithUserParts(ctx, cfg, systemPrompt, userParts)
}

func (p *Processor) callOpenAICompatJSONChatWithUserParts(
	ctx context.Context,
	cfg aiModelCallConfig,
	systemPrompt string,
	userParts []openAICompatContentPart,
) (string, cloudHighlightUsage, map[string]interface{}, int64, error) {
	userContent := interface{}("")
	switch len(userParts) {
	case 0:
		userContent = ""
	case 1:
		if strings.EqualFold(strings.TrimSpace(userParts[0].Type), "text") {
			userContent = userParts[0].Text
		} else {
			userContent = userParts
		}
	default:
		userContent = userParts
	}
	return p.callOpenAICompatJSONChatWithMessages(ctx, cfg, []openAICompatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	})
}

func (p *Processor) callOpenAICompatJSONChatWithMessages(
	ctx context.Context,
	cfg aiModelCallConfig,
	messages []openAICompatMessage,
) (string, cloudHighlightUsage, map[string]interface{}, int64, error) {
	if !cfg.Enabled {
		return "", cloudHighlightUsage{}, nil, 0, fmt.Errorf("ai call disabled")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", cloudHighlightUsage{}, nil, 0, fmt.Errorf("missing api key")
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return "", cloudHighlightUsage{}, nil, 0, fmt.Errorf("missing endpoint")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return "", cloudHighlightUsage{}, nil, 0, fmt.Errorf("missing model")
	}

	reqPayload := openAICompatChatRequest{
		Model:       cfg.Model,
		MaxTokens:   cfg.MaxTokens,
		Temperature: 0.1,
		Messages:    messages,
	}
	body, _ := json.Marshal(reqPayload)

	endpoint := strings.TrimRight(cfg.Endpoint, "/") + "/v1/chat/completions"
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", cloudHighlightUsage{}, nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := p.httpClient
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	} else {
		client = &http.Client{
			Transport: client.Transport,
			Timeout:   cfg.Timeout,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", cloudHighlightUsage{}, nil, clampDurationMillis(started), err
	}
	defer resp.Body.Close()
	rawResp, readErr := io.ReadAll(io.LimitReader(resp.Body, openAICompatMaxRespBytes))
	durationMs := clampDurationMillis(started)
	if readErr != nil {
		return "", cloudHighlightUsage{}, nil, durationMs, fmt.Errorf("read response: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", cloudHighlightUsage{}, nil, durationMs, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(rawResp)))
	}

	typedPayload, err := parseOpenAICompatChatResponse(rawResp)
	if err != nil {
		return "", cloudHighlightUsage{}, nil, durationMs, fmt.Errorf("decode response: %w", err)
	}

	payload, err := parseOpenAICompatRawMap(rawResp)
	if err != nil {
		return "", cloudHighlightUsage{}, nil, durationMs, fmt.Errorf("decode response: %w", err)
	}
	if modelErr := resolveOpenAICompatModelError(typedPayload); modelErr != nil {
		return "", cloudHighlightUsage{}, payload, durationMs, modelErr
	}
	content := extractOpenAICompatMessageContentFromResponse(typedPayload)
	if strings.TrimSpace(content) == "" {
		return "", cloudHighlightUsage{}, payload, durationMs, fmt.Errorf("empty content in model response")
	}

	usage := extractUsageFromOpenAICompatResponse(typedPayload)
	return content, usage, payload, durationMs, nil
}

func parseOpenAICompatChatResponse(raw []byte) (openAICompatChatResponse, error) {
	out := openAICompatChatResponse{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return openAICompatChatResponse{}, err
	}
	return out, nil
}

func parseOpenAICompatRawMap(raw []byte) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func resolveOpenAICompatModelError(payload openAICompatChatResponse) error {
	if payload.Error == nil {
		return nil
	}
	message := strings.TrimSpace(payload.Error.Message)
	if message == "" {
		message = "model returned error envelope"
	}
	errType := strings.TrimSpace(payload.Error.Type)
	if errType == "" {
		return fmt.Errorf("model error: %s", message)
	}
	return fmt.Errorf("model error(%s): %s", errType, message)
}

func parseOpenAICompatResponseFromAnyMap(raw map[string]interface{}) (openAICompatChatResponse, bool) {
	if len(raw) == 0 {
		return openAICompatChatResponse{}, false
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return openAICompatChatResponse{}, false
	}
	parsed, err := parseOpenAICompatChatResponse(body)
	if err != nil {
		return openAICompatChatResponse{}, false
	}
	return parsed, true
}

func extractOpenAICompatMessageContent(raw map[string]interface{}) string {
	typedPayload, ok := parseOpenAICompatResponseFromAnyMap(raw)
	if !ok {
		return ""
	}
	return extractOpenAICompatMessageContentFromResponse(typedPayload)
}

func extractOpenAICompatMessageContentFromResponse(raw openAICompatChatResponse) string {
	if len(raw.Choices) == 0 {
		return ""
	}
	contentRaw := bytes.TrimSpace(raw.Choices[0].Message.Content)
	if len(contentRaw) == 0 || bytes.Equal(contentRaw, []byte("null")) {
		return ""
	}

	var text string
	if err := json.Unmarshal(contentRaw, &text); err == nil {
		return strings.TrimSpace(text)
	}

	var contentParts []openAICompatChatResponseContentPart
	if err := json.Unmarshal(contentRaw, &contentParts); err == nil {
		parts := make([]string, 0, len(contentParts))
		for _, item := range contentParts {
			value := strings.TrimSpace(item.Text)
			if value == "" {
				continue
			}
			parts = append(parts, value)
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}

	var contentPart openAICompatChatResponseContentPart
	if err := json.Unmarshal(contentRaw, &contentPart); err == nil {
		return strings.TrimSpace(contentPart.Text)
	}
	return strings.TrimSpace(string(contentRaw))
}

func summarizeOpenAICompatContentParts(parts []openAICompatContentPart) []map[string]interface{} {
	if len(parts) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(parts))
	for idx, part := range parts {
		item := map[string]interface{}{
			"index": idx + 1,
			"type":  strings.ToLower(strings.TrimSpace(part.Type)),
		}
		switch strings.ToLower(strings.TrimSpace(part.Type)) {
		case "text":
			text := strings.TrimSpace(part.Text)
			item["text_len"] = len(text)
			if text != "" {
				item["text_preview"] = truncateTextForDebug(text, 260)
			}
		case "image_url":
			if part.ImageURL != nil {
				urlText := strings.TrimSpace(part.ImageURL.URL)
				item["url_prefix"] = truncateTextForDebug(urlText, 80)
				item["url_len"] = len(urlText)
				item["is_data_url"] = strings.HasPrefix(strings.ToLower(urlText), "data:")
			}
		case "video_url":
			if part.VideoURL != nil {
				urlText := strings.TrimSpace(part.VideoURL.URL)
				item["url"] = urlText
				item["url_len"] = len(urlText)
			}
		default:
			item["raw"] = part
		}
		out = append(out, item)
	}
	return out
}

func truncateTextForDebug(text string, maxLen int) string {
	text = strings.TrimSpace(text)
	if maxLen <= 0 || len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "...(truncated)"
}

func jsonNumberToFloat64(value json.Number) float64 {
	text := strings.TrimSpace(value.String())
	if text == "" {
		return 0
	}
	if parsed, err := value.Float64(); err == nil {
		return parsed
	}
	return parseFloat(text)
}

func jsonNumberToInt64(value json.Number) int64 {
	return int64(jsonNumberToFloat64(value))
}

func extractUsageFromOpenAICompat(raw map[string]interface{}) cloudHighlightUsage {
	typedPayload, ok := parseOpenAICompatResponseFromAnyMap(raw)
	if !ok {
		return cloudHighlightUsage{}
	}
	return extractUsageFromOpenAICompatResponse(typedPayload)
}

func extractUsageFromOpenAICompatResponse(raw openAICompatChatResponse) cloudHighlightUsage {
	usage := cloudHighlightUsage{
		InputTokens:       jsonNumberToInt64(raw.Usage.PromptTokens),
		OutputTokens:      jsonNumberToInt64(raw.Usage.CompletionTokens),
		CachedInputTokens: jsonNumberToInt64(raw.Usage.CachedTokens),
		ImageTokens:       jsonNumberToInt64(raw.Usage.ImageTokens),
		VideoTokens:       jsonNumberToInt64(raw.Usage.VideoTokens),
		AudioSeconds:      jsonNumberToFloat64(raw.Usage.AudioSeconds),
	}
	if usage.InputTokens <= 0 {
		usage.InputTokens = jsonNumberToInt64(raw.Usage.InputTokens)
	}
	if usage.OutputTokens <= 0 {
		usage.OutputTokens = jsonNumberToInt64(raw.Usage.OutputTokens)
	}
	if usage.CachedInputTokens <= 0 {
		usage.CachedInputTokens = jsonNumberToInt64(raw.Usage.PromptCacheHit)
	}
	if usage.CachedInputTokens <= 0 && raw.Usage.PromptTokenDetails != nil {
		usage.CachedInputTokens = jsonNumberToInt64(raw.Usage.PromptTokenDetails.CachedTokens)
	}
	return normalizeCloudHighlightUsage(usage)
}

func sanitizeModelJSON(raw string) string {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSpace(text)
	}
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSpace(text)
	}
	if strings.HasSuffix(text, "```") {
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	return text
}

func unmarshalModelJSONWithRepair(raw string, out interface{}) error {
	text := sanitizeModelJSON(raw)
	if err := json.Unmarshal([]byte(text), out); err == nil {
		return nil
	}
	repaired := repairModelJSONText(text)
	return json.Unmarshal([]byte(repaired), out)
}

func repairModelJSONText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	text = trimLooseTrailingCommas(text)
	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		return text
	}
	stack := make([]rune, 0, 16)
	inString := false
	escaped := false
	for _, ch := range text {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 {
				continue
			}
			expected := stack[len(stack)-1]
			if ch == expected {
				stack = stack[:len(stack)-1]
			}
		}
	}
	var builder strings.Builder
	builder.Grow(len(text) + len(stack) + 2)
	builder.WriteString(text)
	if inString {
		builder.WriteRune('"')
	}
	for i := len(stack) - 1; i >= 0; i-- {
		builder.WriteRune(stack[i])
	}
	return trimLooseTrailingCommas(strings.TrimSpace(builder.String()))
}

func trimLooseTrailingCommas(text string) string {
	for {
		trimmed := strings.ReplaceAll(strings.ReplaceAll(text, ",}", "}"), ",]", "]")
		if trimmed == text {
			return text
		}
		text = trimmed
	}
}
