package videojobs

import (
	"encoding/json"
	"testing"

	"emoji/internal/models"
)

func TestBuildAIGIFPlannerInputPayload_Contract(t *testing.T) {
	job := models.VideoJob{ID: 99, Title: "  test clip  "}
	meta := videoProbeMeta{DurationSec: 12.34, Width: 720, Height: 1280, FPS: 29.97}
	decision := aiGIFPlannerTopNDecision{
		BaseTopN:        3,
		AISuggestedTopN: 4,
		AllowedTopN:     4,
		AppliedTopN:     4,
		OverrideEnabled: true,
		ExpandRatio:     0.25,
		AbsoluteCap:     6,
		ClampReason:     "",
		DurationTier:    "normal",
	}
	directive := &gifAIDirectiveProfile{
		BusinessGoal: "social_spread",
		ClipCountMin: 2,
		ClipCountMax: 4,
	}
	frameManifest := []aiGIFFrameManifestEntry{
		{Index: 1, TimestampSec: 1.25, Bytes: 12345},
	}

	payload := buildAIGIFPlannerInputPayload(job, meta, decision, frameManifest, directive)
	if payload.JobID != 99 || payload.Title != "test clip" {
		t.Fatalf("unexpected payload identity: %+v", payload)
	}
	if payload.TargetTopN != 4 || payload.FrameCount != 1 {
		t.Fatalf("unexpected topn/frame_count: %+v", payload)
	}
	if payload.Director == nil || payload.Director.BusinessGoal != "social_spread" {
		t.Fatalf("director contract missing: %+v", payload.Director)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if _, ok := decoded["hard_constraint_policy"]; !ok {
		t.Fatalf("hard_constraint_policy missing in payload json")
	}
	if _, ok := decoded["frame_manifest"]; !ok {
		t.Fatalf("frame_manifest missing in payload json")
	}
}

func TestBuildAIGIFDirectorPayloads_Contract(t *testing.T) {
	rt := &aiGIFDirectorRuntime{
		job: models.VideoJob{
			ID:             101,
			Title:          "  demo title  ",
			SourceVideoKey: "video/source.mp4",
		},
		meta: videoProbeMeta{
			DurationSec: 30.5,
			Width:       1280,
			Height:      720,
			FPS:         25,
		},
		qualitySettings:             DefaultQualitySettings(),
		targetFormat:                "gif",
		directorInputModeApplied:    "full_video",
		directorInputModeRequested:  "hybrid",
		directorInputSource:         "full_video_url",
		sourceVideoURL:              "https://example.com/video.mp4",
		operatorEnabled:             true,
		operatorVersion:             "op_v1",
		operatorInstructionRaw:      "raw",
		operatorInstructionRendered: "rendered",
		frameManifest: []aiGIFFrameManifestEntry{
			{Index: 1, TimestampSec: 1.2, Bytes: 4321},
		},
	}

	p := &Processor{}
	payload, debug := p.buildAIGIFDirectorPayloads(rt)
	if payload.SchemaVersion != "ai1_input_v2" {
		t.Fatalf("unexpected schema version: %s", payload.SchemaVersion)
	}
	if payload.Task.AssetGoal == "" || payload.Task.DeliveryGoal == "" {
		t.Fatalf("task fields missing: %+v", payload.Task)
	}
	if payload.Source.VideoSourceKind != "full_video_url_attached_in_content_part" {
		t.Fatalf("video source kind mismatch: %+v", payload.Source)
	}
	if payload.Source.Title != "demo title" {
		t.Fatalf("source title should be trimmed, got=%q", payload.Source.Title)
	}
	if len(payload.Source.FrameRefs) != 1 {
		t.Fatalf("frame refs missing: %+v", payload.Source.FrameRefs)
	}
	if len(debug) == 0 {
		t.Fatalf("debug payload should not be empty")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal director payload failed: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode director payload failed: %v", err)
	}
	if decoded["schema_version"] != "ai1_input_v2" {
		t.Fatalf("schema_version missing in json: %+v", decoded)
	}
	if _, ok := decoded["task"]; !ok {
		t.Fatalf("task missing in director payload json")
	}
	if _, ok := decoded["source"]; !ok {
		t.Fatalf("source missing in director payload json")
	}
}

func TestAIGIFUsageMetadata_Contract(t *testing.T) {
	plannerMeta := aiGIFPlannerUsageMetadata{
		PromptVersion:          "v1",
		PromptTemplateVersion:  "tpl_v1",
		PromptTemplateSource:   "built_in",
		TargetTopN:             3,
		TargetTopNBase:         2,
		TargetTopNAISuggested:  3,
		TargetTopNAllowed:      3,
		TargetTopNApplied:      3,
		TargetTopNDurationTier: "normal",
		CandidateSource:        "frame_manifest",
		FrameCount:             6,
		DirectorApplied:        true,
		PlannerInputPayloadV1: aiGIFPlannerInputPayload{
			JobID: 1,
		},
		PlannerInputPayloadBytes: 128,
	}
	body, err := json.Marshal(plannerMeta)
	if err != nil {
		t.Fatalf("marshal planner metadata failed: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode planner metadata failed: %v", err)
	}
	required := []string{
		"prompt_version",
		"prompt_template_version",
		"target_top_n",
		"planner_input_payload_v1",
		"planner_input_payload_bytes",
	}
	for _, key := range required {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("metadata key missing: %s", key)
		}
	}
}

func TestAIGIFJudgeReviewRowMetadata_Contract(t *testing.T) {
	meta := aiGIFJudgeReviewRowMetadata{
		ProposalRank: 2,
		OutputScore:  0.88,
		EvalOverall:  0.8,
		EvalLoop:     0.7,
		EvalClarity:  0.9,
		WindowStart:  12.3,
		WindowEnd:    14.8,
		HardGate: aiGIFJudgeHardGateMetadata{
			Applied:       true,
			Blocked:       true,
			From:          "deliver",
			To:            "reject",
			ReasonCodes:   []string{"clarity_low"},
			ReasonSummary: "clarity_low",
		},
	}
	body, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal judge review metadata failed: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode judge review metadata failed: %v", err)
	}
	if _, ok := decoded["hard_gate"]; !ok {
		t.Fatalf("hard_gate missing in metadata json")
	}
}

func TestAIGIFJudgeRunSnapshot_Contract(t *testing.T) {
	snapshot := aiGIFJudgeRunSnapshot{
		Enabled:                      true,
		Provider:                     "qwen",
		Model:                        "qwen3-vl-plus",
		PromptVersion:                "v1",
		Mode:                         "",
		Reason:                       "",
		Applied:                      true,
		ReviewedOutputs:              6,
		DeliverCount:                 2,
		KeepInternalCount:            1,
		RejectCount:                  2,
		ManualReviewCount:            1,
		HardGateApplied:              3,
		HardGateRejectCount:          2,
		HardGateManualReviewCount:    1,
		DeliverFallbackApplied:       false,
		DeliverFallbackReason:        "disabled_ai3",
		DeliverFallbackTriggerReason: "not_applicable",
		Summary: map[string]interface{}{
			"note": "ok",
		},
	}
	m := normalizeVideoJobAIUsageMetadata(snapshot)
	keys := []string{
		"enabled",
		"provider",
		"model",
		"prompt_version",
		"applied",
		"reviewed_outputs",
		"deliver_count",
		"hard_gate_applied",
		"deliver_fallback_applied",
	}
	for _, key := range keys {
		if _, ok := m[key]; !ok {
			t.Fatalf("judge snapshot key missing: %s", key)
		}
	}
}

func TestAIGIFDeliverFallbackResult_Contract(t *testing.T) {
	result := aiGIFDeliverFallbackResult{
		Attempted:              true,
		Applied:                false,
		Reason:                 "disabled_ai3",
		TriggerReason:          "not_applicable",
		Policy:                 "ai3_final_review_authoritative",
		SampleCount:            8,
		ReviewCount:            1,
		DeliverCountBefore:     0,
		DeliverCountAfter:      0,
		SelectedOutputID:       0,
		PreviousRecommendation: "",
	}
	m := normalizeVideoJobAIUsageMetadata(result)
	keys := []string{
		"attempted",
		"applied",
		"reason",
		"trigger_reason",
		"policy",
		"sample_count",
		"review_count",
	}
	for _, key := range keys {
		if _, ok := m[key]; !ok {
			t.Fatalf("deliver fallback result key missing: %s", key)
		}
	}
}

func TestDecodeAIGIFJudgeRunSnapshot_FromMap(t *testing.T) {
	raw := map[string]interface{}{
		"enabled":                         true,
		"provider":                        "qwen",
		"model":                           "qwen3-vl-plus",
		"prompt_version":                  "v1",
		"applied":                         false,
		"error":                           "x",
		"deliver_fallback_applied":        false,
		"deliver_fallback_reason":         "disabled_ai3",
		"deliver_fallback_trigger_reason": "not_applicable",
	}
	got := decodeAIGIFJudgeRunSnapshot(raw)
	if !got.Enabled || got.Provider != "qwen" || got.PromptVersion != "v1" {
		t.Fatalf("decode snapshot basic fields mismatch: %+v", got)
	}
	if got.DeliverFallbackReason != "disabled_ai3" {
		t.Fatalf("decode snapshot fallback fields mismatch: %+v", got)
	}
}
