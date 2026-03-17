package videojobs

import (
	"os"
	"testing"
)

func TestEstimateVideoJobAIUsageCostUSD(t *testing.T) {
	in := videoJobAIUsageInput{
		InputTokens:       1_000_000,
		OutputTokens:      100_000,
		CachedInputTokens: 200_000,
		AudioSeconds:      60,
	}
	pricing := aiUnitPricing{
		InputPer1M:       1.0,
		OutputPer1M:      2.0,
		CachedInputPer1M: 0.5,
		AudioPerMin:      0.1,
	}
	got := estimateVideoJobAIUsageCostUSD(in, pricing)
	const want = 1.2
	if got != want {
		t.Fatalf("unexpected cost: got %.8f want %.8f", got, want)
	}
}

func TestLookupAIUnitPricing_DefaultQwenFlash(t *testing.T) {
	prev := os.Getenv("VIDEO_JOB_AI_PRICING_OVERRIDES_JSON")
	defer os.Setenv("VIDEO_JOB_AI_PRICING_OVERRIDES_JSON", prev)
	_ = os.Unsetenv("VIDEO_JOB_AI_PRICING_OVERRIDES_JSON")

	item := lookupAIUnitPricing("qwen", "qwen3-vl-flash")
	if item.InputPer1M <= 0 || item.OutputPer1M <= 0 {
		t.Fatalf("expected positive default pricing, got input=%.6f output=%.6f", item.InputPer1M, item.OutputPer1M)
	}
	if item.Version == "" {
		t.Fatalf("expected pricing version")
	}
}

func TestLookupAIUnitPricing_Override(t *testing.T) {
	prev := os.Getenv("VIDEO_JOB_AI_PRICING_OVERRIDES_JSON")
	defer os.Setenv("VIDEO_JOB_AI_PRICING_OVERRIDES_JSON", prev)
	if err := os.Setenv("VIDEO_JOB_AI_PRICING_OVERRIDES_JSON", `[{"provider":"deepseek","model":"deepseek-reasoner","input_per_1m":1.23,"output_per_1m":4.56,"cached_input_per_1m":0.12,"audio_per_min":0.34,"currency":"USD","version":"test_v1"}]`); err != nil {
		t.Fatalf("set env failed: %v", err)
	}

	item := lookupAIUnitPricing("deepseek", "deepseek-reasoner")
	if item.InputPer1M != 1.23 || item.OutputPer1M != 4.56 {
		t.Fatalf("override not applied: %+v", item)
	}
	if item.Version != "test_v1" {
		t.Fatalf("unexpected version: %s", item.Version)
	}
}
