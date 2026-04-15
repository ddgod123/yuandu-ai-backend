package videojobs

import (
	"errors"
	"testing"
)

func TestIsOpenAICompatAccessDeniedError(t *testing.T) {
	t.Run("access_denied_code", func(t *testing.T) {
		err := errors.New(`http 403: {"error":{"code":"access_denied","message":"Access denied"}}`)
		if !isOpenAICompatAccessDeniedError(err) {
			t.Fatalf("expected access_denied error to be recognized")
		}
	})

	t.Run("access denied text", func(t *testing.T) {
		err := errors.New("http 403: Access denied. For details, see ...")
		if !isOpenAICompatAccessDeniedError(err) {
			t.Fatalf("expected access denied text to be recognized")
		}
	})

	t.Run("non access denied", func(t *testing.T) {
		err := errors.New("http 429: rate limit")
		if isOpenAICompatAccessDeniedError(err) {
			t.Fatalf("expected non-access-denied error to be ignored")
		}
	})
}

func TestSuggestQwenFallbackModels(t *testing.T) {
	got := suggestQwenFallbackModels("qwen3.5-omni-plus")
	if len(got) < 3 || got[0] != "qwen3-vl-flash" {
		t.Fatalf("unexpected fallback chain for omni-plus: %+v", got)
	}

	got = suggestQwenFallbackModels("qwen-plus")
	if len(got) != 1 || got[0] != "qwen-turbo" {
		t.Fatalf("unexpected fallback chain for qwen-plus: %+v", got)
	}

	got = suggestQwenFallbackModels("unknown-model")
	if len(got) != 0 {
		t.Fatalf("expected empty fallback chain for unknown model, got %+v", got)
	}
}
