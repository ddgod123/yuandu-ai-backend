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

func TestExtractUsageFromOpenAICompat(t *testing.T) {
	raw := map[string]interface{}{
		"usage": map[string]interface{}{
			"prompt_tokens":     1200.0,
			"completion_tokens": 340.0,
			"prompt_tokens_details": map[string]interface{}{
				"cached_tokens": 500.0,
			},
		},
	}
	usage := extractUsageFromOpenAICompat(raw)
	if usage.InputTokens != 1200 {
		t.Fatalf("input tokens mismatch: got=%d", usage.InputTokens)
	}
	if usage.OutputTokens != 340 {
		t.Fatalf("output tokens mismatch: got=%d", usage.OutputTokens)
	}
	if usage.CachedInputTokens != 500 {
		t.Fatalf("cached tokens mismatch: got=%d", usage.CachedInputTokens)
	}
}

func TestExtractOpenAICompatMessageContent_TypedPayload(t *testing.T) {
	raw := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"role": "assistant",
					"content": []interface{}{
						map[string]interface{}{"type": "output_text", "text": "第一段"},
						map[string]interface{}{"type": "output_text", "text": "第二段"},
					},
				},
			},
		},
	}
	content := extractOpenAICompatMessageContent(raw)
	if !strings.Contains(content, "第一段") || !strings.Contains(content, "第二段") {
		t.Fatalf("unexpected content: %s", content)
	}
}

func TestParseOpenAICompatChatResponse_UseNumber(t *testing.T) {
	raw := []byte(`{
		"id":"chatcmpl_xxx",
		"choices":[{"message":{"role":"assistant","content":"ok"}}],
		"usage":{"prompt_tokens":1200,"completion_tokens":340,"audio_seconds":1.25}
	}`)
	parsed, err := parseOpenAICompatChatResponse(raw)
	if err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	if got := extractOpenAICompatMessageContentFromResponse(parsed); got != "ok" {
		t.Fatalf("content mismatch: %s", got)
	}
	usage := extractUsageFromOpenAICompatResponse(parsed)
	if usage.InputTokens != 1200 || usage.OutputTokens != 340 {
		t.Fatalf("usage mismatch: %#v", usage)
	}
	if usage.AudioSeconds <= 1.2 || usage.AudioSeconds >= 1.3 {
		t.Fatalf("audio seconds mismatch: %f", usage.AudioSeconds)
	}
}

func TestNormalizeAIGIFPlannerProposals(t *testing.T) {
	in := []gifAIPlannerProposal{
		{
			ProposalRank:   1,
			StartSec:       -1,
			EndSec:         2.8,
			Score:          1.5,
			ProposalReason: "",
		},
		{
			ProposalRank:   2,
			StartSec:       3.0,
			EndSec:         3.2, // too short, should be filtered out
			Score:          0.5,
			ProposalReason: "too short",
		},
	}
	out, _ := normalizeAIGIFPlannerProposals(in, 10, nil)
	if len(out) != 1 {
		t.Fatalf("expected 1 valid proposal, got %d", len(out))
	}
	if out[0].StartSec < 0 || out[0].EndSec <= out[0].StartSec {
		t.Fatalf("invalid clamped window: %+v", out[0])
	}
	if out[0].Score > 1 {
		t.Fatalf("score should be clamped to <=1, got %f", out[0].Score)
	}
	if out[0].ProposalReason == "" {
		t.Fatalf("proposal reason should be normalized")
	}
}

func TestNormalizeAIGIFPlannerProposals_DedupProposalRank(t *testing.T) {
	in := []gifAIPlannerProposal{
		{ProposalRank: 1, StartSec: 1.0, EndSec: 2.6, Score: 0.9, ProposalReason: "top"},
		{ProposalRank: 1, StartSec: 4.0, EndSec: 5.8, Score: 0.8, ProposalReason: "duplicate rank"},
		{ProposalRank: 2, StartSec: 6.0, EndSec: 8.0, Score: 0.7, ProposalReason: "second"},
	}
	out, _ := normalizeAIGIFPlannerProposals(in, 20, nil)
	if len(out) != 2 {
		t.Fatalf("expected rank-dedup to keep 2 proposals, got %d", len(out))
	}
	if out[0].ProposalRank != 1 || out[1].ProposalRank != 2 {
		t.Fatalf("unexpected proposal ranks after dedup: %+v", out)
	}
}

func TestResolveAIGIFPlannerTargetTopN(t *testing.T) {
	settings := DefaultQualitySettings()
	settings.GIFCandidateMaxOutputs = 3
	settings.GIFCandidateLongVideoMaxOutputs = 3
	settings.GIFCandidateUltraVideoMaxOutputs = 2

	directive := &gifAIDirectiveProfile{
		ClipCountMin: 2,
		ClipCountMax: 5,
	}

	shortMeta := videoProbeMeta{DurationSec: 30, Width: 720, Height: 720}
	if got := resolveAIGIFPlannerTargetTopN(shortMeta, directive, settings); got != 3 {
		t.Fatalf("expected short target_top_n=3 when override disabled, got %d", got)
	}

	longMeta := videoProbeMeta{DurationSec: 180, Width: 720, Height: 720}
	if got := resolveAIGIFPlannerTargetTopN(longMeta, directive, settings); got != 3 {
		t.Fatalf("expected long tier capped target_top_n=3, got %d", got)
	}

	ultraMeta := videoProbeMeta{DurationSec: 420, Width: 720, Height: 720}
	if got := resolveAIGIFPlannerTargetTopN(ultraMeta, directive, settings); got != 2 {
		t.Fatalf("expected ultra tier capped target_top_n=2, got %d", got)
	}

	settings.AIDirectorConstraintOverrideEnabled = true
	settings.AIDirectorCountExpandRatio = 1.0
	settings.AIDirectorCountAbsoluteCap = 10
	if got := resolveAIGIFPlannerTargetTopN(shortMeta, directive, settings); got != 5 {
		t.Fatalf("expected short target_top_n=5 when override enabled, got %d", got)
	}
	if got := resolveAIGIFPlannerTargetTopN(longMeta, directive, settings); got != 5 {
		t.Fatalf("expected long target_top_n=5 when override enabled, got %d", got)
	}
	if got := resolveAIGIFPlannerTargetTopN(ultraMeta, directive, settings); got != 4 {
		t.Fatalf("expected ultra target_top_n=4 when override enabled, got %d", got)
	}
}

func TestNormalizeAIGIFDirective(t *testing.T) {
	in := gifAIDirectiveProfile{
		BusinessGoal:       "",
		MustCapture:        []string{"  表情爆发  ", "表情爆发"},
		Avoid:              []string{"", "转场模糊"},
		RiskFlags:          []string{"", "low_light", "Low_Light"},
		ClipCountMin:       0,
		ClipCountMax:       0,
		DurationPrefMinSec: 0.2,
		DurationPrefMaxSec: 0.1,
		LoopPreference:     1.5,
		QualityWeights: map[string]float64{
			"semantic": 2,
		},
	}
	out := normalizeAIGIFDirective(in, 3)
	if out == nil {
		t.Fatalf("expected directive")
	}
	if out.BusinessGoal == "" {
		t.Fatalf("expected default business goal")
	}
	if len(out.MustCapture) != 1 {
		t.Fatalf("expected dedup must_capture, got %v", out.MustCapture)
	}
	if out.ClipCountMin <= 0 || out.ClipCountMax < out.ClipCountMin {
		t.Fatalf("invalid clip count range: %+v", out)
	}
	if out.DurationPrefMaxSec <= out.DurationPrefMinSec {
		t.Fatalf("invalid duration preference: %+v", out)
	}
	if out.LoopPreference > 1 {
		t.Fatalf("loop preference should be clamped <=1")
	}
	if out.StyleDirection == "" {
		t.Fatalf("style_direction should be filled with default")
	}
	if len(out.RiskFlags) != 1 || out.RiskFlags[0] != "low_light" {
		t.Fatalf("risk_flags should be normalized and deduped, got %+v", out.RiskFlags)
	}
	if out.DirectiveText == "" {
		t.Fatalf("directive text should be filled")
	}
	sum := out.QualityWeights["semantic"] + out.QualityWeights["clarity"] + out.QualityWeights["loop"] + out.QualityWeights["efficiency"]
	if sum < 0.99 || sum > 1.01 {
		t.Fatalf("quality weights should normalize to 1, got %.4f", sum)
	}
}

func TestCallOpenAICompatJSONChat_LargeResponseNotTruncated(t *testing.T) {
	largeReasoning := strings.Repeat("r", 3<<20) // 3 MiB (> old 2 MiB cap)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"role":              "assistant",
						"content":           `{"ok":true}`,
						"reasoning_content": largeReasoning,
					},
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 6,
			},
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	p := &Processor{httpClient: server.Client()}
	cfg := aiModelCallConfig{
		Enabled:       true,
		Provider:      "deepseek",
		Model:         "deepseek-reasoner",
		Endpoint:      server.URL,
		APIKey:        "test-key",
		Timeout:       5 * time.Second,
		MaxTokens:     512,
		PromptVersion: "test",
	}
	content, usage, _, _, err := p.callOpenAICompatJSONChat(context.Background(), cfg, "sys", "user")
	if err != nil {
		t.Fatalf("callOpenAICompatJSONChat returned error: %v", err)
	}
	if content != `{"ok":true}` {
		t.Fatalf("unexpected content: %s", content)
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 6 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestApplyAIGIFTechnicalHardGates_DeliverBlockedToReject(t *testing.T) {
	reviews := []gifAIJudgeReviewRow{
		{
			OutputID:            101,
			FinalRecommendation: "deliver",
			DiagnosticReason:    "语义好",
		},
	}
	samples := []gifJudgeSample{
		{
			OutputID:    101,
			Score:       0.82,
			SizeBytes:   1_500_000,
			Width:       720,
			Height:      720,
			DurationMs:  1800,
			EvalOverall: 0.45,
			EvalClarity: 0.12, // hard gate
			EvalLoop:    0.76,
		},
	}

	updated, verdicts, stats := applyAIGIFTechnicalHardGates(reviews, samples, DefaultQualitySettings())
	if len(updated) != 1 {
		t.Fatalf("expected 1 updated review, got %d", len(updated))
	}
	if updated[0].FinalRecommendation != "reject" {
		t.Fatalf("expected hard gate to force reject, got %s", updated[0].FinalRecommendation)
	}
	v := verdicts[101]
	if !v.Applied || !v.Blocked {
		t.Fatalf("expected hard gate applied+blocked, got %+v", v)
	}
	if !containsString(v.ReasonCodes, "clarity_low") {
		t.Fatalf("expected clarity_low reason, got %+v", v.ReasonCodes)
	}
	if stats.Applied != 1 || stats.RejectCount != 1 || stats.ManualReviewCount != 0 {
		t.Fatalf("unexpected hard gate stats: %+v", stats)
	}
}

func TestApplyAIGIFTechnicalHardGates_EvaluationMissingToManualReview(t *testing.T) {
	reviews := []gifAIJudgeReviewRow{
		{
			OutputID:            202,
			FinalRecommendation: "deliver",
		},
	}
	samples := []gifJudgeSample{
		{
			OutputID:   202,
			Score:      0.77,
			SizeBytes:  1_200_000,
			Width:      640,
			Height:     640,
			DurationMs: 1400,
			// all eval scores are zero => missing evaluation
		},
	}

	updated, verdicts, stats := applyAIGIFTechnicalHardGates(reviews, samples, DefaultQualitySettings())
	if updated[0].FinalRecommendation != "need_manual_review" {
		t.Fatalf("expected manual review fallback, got %s", updated[0].FinalRecommendation)
	}
	v := verdicts[202]
	if !containsString(v.ReasonCodes, "evaluation_missing") {
		t.Fatalf("expected evaluation_missing reason, got %+v", v.ReasonCodes)
	}
	if stats.Applied != 1 || stats.RejectCount != 0 || stats.ManualReviewCount != 1 {
		t.Fatalf("unexpected hard gate stats: %+v", stats)
	}
}

func TestApplyAIGIFTechnicalHardGates_KeepInternalNotForced(t *testing.T) {
	reviews := []gifAIJudgeReviewRow{
		{
			OutputID:            303,
			FinalRecommendation: "keep_internal",
		},
	}
	samples := []gifJudgeSample{
		{
			OutputID:    303,
			Score:       0.15,
			SizeBytes:   2_000_000,
			Width:       640,
			Height:      640,
			DurationMs:  1000,
			EvalOverall: 0.14,
			EvalClarity: 0.10,
			EvalLoop:    0.10,
		},
	}

	updated, _, stats := applyAIGIFTechnicalHardGates(reviews, samples, DefaultQualitySettings())
	if updated[0].FinalRecommendation != "keep_internal" {
		t.Fatalf("non-deliver recommendation should remain unchanged, got %s", updated[0].FinalRecommendation)
	}
	if stats.Applied != 0 {
		t.Fatalf("expected no hard gate changes, got %+v", stats)
	}
}

func TestApplyAIGIFTechnicalHardGates_UsesConfiguredThresholds(t *testing.T) {
	reviews := []gifAIJudgeReviewRow{
		{
			OutputID:            404,
			FinalRecommendation: "deliver",
		},
	}
	samples := []gifJudgeSample{
		{
			OutputID:    404,
			Score:       0.88,
			SizeBytes:   250_000,
			Width:       720,
			Height:      720,
			DurationMs:  1600,
			EvalOverall: 0.80,
			EvalClarity: 0.35, // above default(0.2), below overridden(0.4)
			EvalLoop:    0.70,
		},
	}
	settings := DefaultQualitySettings()
	settings.GIFAIJudgeHardGateMinClarityScore = 0.40

	updated, verdicts, stats := applyAIGIFTechnicalHardGates(reviews, samples, settings)
	if updated[0].FinalRecommendation != "reject" {
		t.Fatalf("expected reject with overridden clarity gate, got %s", updated[0].FinalRecommendation)
	}
	v := verdicts[404]
	if !containsString(v.ReasonCodes, "clarity_low") {
		t.Fatalf("expected clarity_low reason under overridden threshold, got %+v", v.ReasonCodes)
	}
	if stats.Applied != 1 || stats.RejectCount != 1 {
		t.Fatalf("unexpected hard gate stats: %+v", stats)
	}
}

func TestApplyAIGIFTechnicalHardGates_ZeroValueSettingsFallbackToDefaults(t *testing.T) {
	reviews := []gifAIJudgeReviewRow{
		{
			OutputID:            505,
			FinalRecommendation: "deliver",
		},
	}
	samples := []gifJudgeSample{
		{
			OutputID:    505,
			Score:       0.81,
			SizeBytes:   320_000,
			Width:       640,
			Height:      640,
			DurationMs:  1500,
			EvalOverall: 0.70,
			EvalClarity: 0.12, // should still be rejected by default threshold(0.2)
			EvalLoop:    0.66,
		},
	}

	updated, verdicts, stats := applyAIGIFTechnicalHardGates(reviews, samples, QualitySettings{})
	if updated[0].FinalRecommendation != "reject" {
		t.Fatalf("expected reject via default fallback thresholds, got %s", updated[0].FinalRecommendation)
	}
	v := verdicts[505]
	if !containsString(v.ReasonCodes, "clarity_low") {
		t.Fatalf("expected clarity_low reason under default fallback, got %+v", v.ReasonCodes)
	}
	if stats.Applied != 1 || stats.RejectCount != 1 {
		t.Fatalf("unexpected hard gate stats: %+v", stats)
	}
}
