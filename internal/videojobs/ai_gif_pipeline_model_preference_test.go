package videojobs

import "testing"

func TestParseVideoJobAIModelPreferenceOmniAliases(t *testing.T) {
	provider, model := parseVideoJobAIModelPreference("omni")
	if provider != "qwen" || model != "qwen3.5-omni-flash" {
		t.Fatalf("unexpected omni parse result: provider=%s model=%s", provider, model)
	}

	provider, model = parseVideoJobAIModelPreference("omni-plus")
	if provider != "qwen" || model != "qwen3.5-omni-plus" {
		t.Fatalf("unexpected omni-plus parse result: provider=%s model=%s", provider, model)
	}
}

func TestParseVideoJobAIModelPreferenceQwen35PlusAliases(t *testing.T) {
	cases := []string{
		"qwen3.5-plus",
		"qwen3_5_plus",
		"qwen35_plus",
		"3.5_plus",
	}
	for _, raw := range cases {
		provider, model := parseVideoJobAIModelPreference(raw)
		if provider != "qwen" || model != "qwen3.5-plus" {
			t.Fatalf("unexpected parse result for %q: provider=%s model=%s", raw, provider, model)
		}
	}
}
