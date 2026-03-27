package videojobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCallOpenAICompatJSONChat_ModelErrorEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"type":    "rate_limit",
				"message": "quota exceeded",
			},
		})
	}))
	defer server.Close()

	p := &Processor{httpClient: server.Client()}
	cfg := aiModelCallConfig{
		Enabled:   true,
		Provider:  "test",
		Model:     "test-model",
		Endpoint:  server.URL,
		APIKey:    "test-key",
		Timeout:   5 * time.Second,
		MaxTokens: 128,
	}

	_, _, _, _, err := p.callOpenAICompatJSONChat(context.Background(), cfg, "sys", "user")
	if err == nil {
		t.Fatalf("expected model error envelope to fail")
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCallOpenAICompatJSONChat_RawPayloadUseNumber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"ok":true}`,
					},
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     123,
				"completion_tokens": 45,
			},
		})
	}))
	defer server.Close()

	p := &Processor{httpClient: server.Client()}
	cfg := aiModelCallConfig{
		Enabled:   true,
		Provider:  "test",
		Model:     "test-model",
		Endpoint:  server.URL,
		APIKey:    "test-key",
		Timeout:   5 * time.Second,
		MaxTokens: 128,
	}

	_, _, raw, _, err := p.callOpenAICompatJSONChat(context.Background(), cfg, "sys", "user")
	if err != nil {
		t.Fatalf("callOpenAICompatJSONChat returned error: %v", err)
	}
	usage, ok := raw["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("usage payload missing: %#v", raw["usage"])
	}
	if _, ok := usage["prompt_tokens"].(json.Number); !ok {
		t.Fatalf("prompt_tokens should be json.Number, got=%T", usage["prompt_tokens"])
	}
}

func TestUnmarshalModelJSONWithRepair_MissingClosers(t *testing.T) {
	raw := `{"directive":{"business_goal":"social_spread","clip_count_min":2,"clip_count_max":4,"duration_pref_min_sec":1.2,"duration_pref_max_sec":2.6}`
	var parsed map[string]interface{}
	if err := unmarshalModelJSONWithRepair(raw, &parsed); err != nil {
		t.Fatalf("unmarshalModelJSONWithRepair failed: %v", err)
	}
	if _, ok := parsed["directive"]; !ok {
		t.Fatalf("directive field missing after repair: %+v", parsed)
	}
}

func TestUnmarshalModelJSONWithRepair_TrailingComma(t *testing.T) {
	raw := `{"a":1,"b":{"c":2,},}`
	var parsed map[string]interface{}
	if err := unmarshalModelJSONWithRepair(raw, &parsed); err != nil {
		t.Fatalf("unmarshalModelJSONWithRepair failed: %v", err)
	}
	if _, ok := parsed["b"]; !ok {
		t.Fatalf("nested field missing after repair: %+v", parsed)
	}
}
