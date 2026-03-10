package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// LLMProvider defines the interface for any LLM API backend.
type LLMProvider interface {
	// Chat sends a system prompt + user message and returns the assistant's text reply.
	Chat(ctx context.Context, systemPrompt, userMessage string, maxTokens int) (string, error)
	Name() string
}

// ---------------------------------------------------------------------------
// Claude (Anthropic Messages API)
// ---------------------------------------------------------------------------

type ClaudeProvider struct {
	APIKey   string
	Model    string
	Endpoint string // e.g. "https://api.anthropic.com"
}

func (p *ClaudeProvider) Name() string { return "claude" }

func (p *ClaudeProvider) Chat(ctx context.Context, systemPrompt, userMessage string, maxTokens int) (string, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	payload := struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		System    string `json:"system"`
		Messages  []msg  `json:"messages"`
	}{
		Model:     p.Model,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages:  []msg{{Role: "user", Content: userMessage}},
	}
	body, _ := json.Marshal(payload)

	endpoint := strings.TrimRight(p.Endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("claude request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("claude %d: %s", resp.StatusCode, string(respBody))
	}

	var cr struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return "", fmt.Errorf("claude decode: %w", err)
	}
	if len(cr.Content) == 0 {
		return "", fmt.Errorf("claude empty response")
	}
	return strings.TrimSpace(cr.Content[0].Text), nil
}

// ---------------------------------------------------------------------------
// Volcengine Doubao — Responses API (/api/v3/responses)
// 格式与 OpenAI Chat Completions 不同，input 字段为数组
// ---------------------------------------------------------------------------

type VolcengineProvider struct {
	APIKey   string
	Model    string
	Endpoint string
}

func (p *VolcengineProvider) Name() string { return "volcengine" }

func (p *VolcengineProvider) Chat(ctx context.Context, systemPrompt, userMessage string, maxTokens int) (string, error) {
	type textContent struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type inputMsg struct {
		Role    string        `json:"role"`
		Content []textContent `json:"content"`
	}

	msgs := []inputMsg{}
	if systemPrompt != "" {
		msgs = append(msgs, inputMsg{
			Role:    "system",
			Content: []textContent{{Type: "input_text", Text: systemPrompt}},
		})
	}
	msgs = append(msgs, inputMsg{
		Role:    "user",
		Content: []textContent{{Type: "input_text", Text: userMessage}},
	})

	payload := struct {
		Model string     `json:"model"`
		Input []inputMsg `json:"input"`
	}{
		Model: p.Model,
		Input: msgs,
	}
	body, _ := json.Marshal(payload)

	endpoint := strings.TrimRight(p.Endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint+"/responses", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("volcengine request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("volcengine %d: %s", resp.StatusCode, string(respBody))
	}

	// Response format: {"output": [{"type":"text","text":"..."}], ...}
	var cr struct {
		Output []struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return "", fmt.Errorf("volcengine decode: %w (raw: %s)", err, string(respBody))
	}
	if cr.Error != nil {
		return "", fmt.Errorf("volcengine error: %s", cr.Error.Message)
	}
	for _, out := range cr.Output {
		if out.Type == "text" && out.Text != "" {
			return strings.TrimSpace(out.Text), nil
		}
		// Some responses nest content inside output items
		for _, c := range out.Content {
			if c.Type == "output_text" && c.Text != "" {
				return strings.TrimSpace(c.Text), nil
			}
		}
	}
	return "", fmt.Errorf("volcengine: no text in response: %s", string(respBody))
}

// ---------------------------------------------------------------------------
// OpenAI-compatible (works for DeepSeek, Qwen/DashScope, Moonshot, etc.)
// ---------------------------------------------------------------------------

type OpenAICompatProvider struct {
	APIKey   string
	Model    string
	Endpoint string // e.g. "https://api.deepseek.com" or "https://dashscope.aliyuncs.com/compatible-mode"
	ProviderName string
}

func (p *OpenAICompatProvider) Name() string {
	if p.ProviderName != "" {
		return p.ProviderName
	}
	return "openai-compat"
}

// PLACEHOLDER_OPENAI_1

func (p *OpenAICompatProvider) Chat(ctx context.Context, systemPrompt, userMessage string, maxTokens int) (string, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	payload := struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []msg  `json:"messages"`
	}{
		Model:     p.Model,
		MaxTokens: maxTokens,
		Messages: []msg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
	}
	body, _ := json.Marshal(payload)

	endpoint := strings.TrimRight(p.Endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s request: %w", p.Name(), err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s %d: %s", p.Name(), resp.StatusCode, string(respBody))
	}

	var cr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return "", fmt.Errorf("%s decode: %w", p.Name(), err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("%s empty response", p.Name())
	}
	return strings.TrimSpace(cr.Choices[0].Message.Content), nil
}

// ---------------------------------------------------------------------------
// Factory: create provider from config
// ---------------------------------------------------------------------------

func NewLLMProvider(provider, apiKey, model, endpoint string) LLMProvider {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "claude", "anthropic":
		if endpoint == "" {
			endpoint = "https://api.anthropic.com"
		}
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
		return &ClaudeProvider{APIKey: apiKey, Model: model, Endpoint: endpoint}

	case "deepseek":
		if endpoint == "" {
			endpoint = "https://api.deepseek.com"
		}
		if model == "" {
			model = "deepseek-chat"
		}
		return &OpenAICompatProvider{APIKey: apiKey, Model: model, Endpoint: endpoint, ProviderName: "deepseek"}

	case "qwen", "dashscope", "tongyi":
		if endpoint == "" {
			endpoint = "https://dashscope.aliyuncs.com/compatible-mode"
		}
		if model == "" {
			model = "qwen-plus"
		}
		return &OpenAICompatProvider{APIKey: apiKey, Model: model, Endpoint: endpoint, ProviderName: "qwen"}

	case "moonshot", "kimi":
		if endpoint == "" {
			endpoint = "https://api.moonshot.cn"
		}
		if model == "" {
			model = "moonshot-v1-8k"
		}
		return &OpenAICompatProvider{APIKey: apiKey, Model: model, Endpoint: endpoint, ProviderName: "moonshot"}

	case "volcengine", "volc", "doubao", "火山":
		if endpoint == "" {
			endpoint = "https://ark.cn-beijing.volces.com/api/v3"
		}
		if model == "" {
			model = "doubao-seed-1-8-251228"
		}
		return &VolcengineProvider{APIKey: apiKey, Model: model, Endpoint: endpoint}

	default:
		// Default: treat as OpenAI-compatible with custom endpoint
		if endpoint == "" {
			endpoint = "https://api.openai.com"
		}
		if model == "" {
			model = "gpt-4o-mini"
		}
		return &OpenAICompatProvider{APIKey: apiKey, Model: model, Endpoint: endpoint, ProviderName: provider}
	}
}
