package videojobs

import (
	"strings"
	"testing"
)

func buildAI1ContractTestFixture() (videoProbeMeta, map[string]interface{}, map[string]interface{}, map[string]interface{}, []string) {
	meta := videoProbeMeta{
		DurationSec: 18.6,
		Width:       1280,
		Height:      720,
		FPS:         30,
	}
	eventMeta := map[string]interface{}{
		"ai_reply":        "我会优先抓取进球瞬间，并避开黑屏和抖动画面。",
		"business_goal":   "提取精彩进球画面",
		"must_capture":    []interface{}{"进球瞬间", "庆祝动作"},
		"avoid":           []interface{}{"黑屏", "强烈抖动"},
		"style_direction": "高清静态帧",
	}
	executablePlan := map[string]interface{}{
		"target_format": "png",
		"mode":          "focus_window",
		"target_count":  12,
		"focus_window": map[string]interface{}{
			"start_sec": 4.2,
			"end_sec":   9.1,
		},
	}
	trace := map[string]interface{}{
		"event_stage": "ai1_preview_generated",
		"flow_mode":   "ai1_confirm",
		"sub_stage":   "ai1_briefing",
	}
	return meta, eventMeta, executablePlan, trace, []string{"png"}
}

func TestValidateAndRepairAI1OutputV2_ValidPayload(t *testing.T) {
	meta, eventMeta, executablePlan, trace, requestedFormats := buildAI1ContractTestFixture()
	raw := buildAI1OutputV2(
		VideoJobAI1PlanSchemaPNGV1,
		requestedFormats,
		meta,
		eventMeta,
		executablePlan,
		trace,
	)
	got := validateAndRepairAI1OutputV2(
		raw,
		VideoJobAI1PlanSchemaPNGV1,
		requestedFormats,
		meta,
		eventMeta,
		executablePlan,
		trace,
	)
	if !hasRequiredAI1OutputV2(got) {
		t.Fatalf("expected valid ai1_output_v2")
	}
	if boolFromAny(mapFromAny(got["trace"])["contract_repaired"]) {
		t.Fatalf("valid payload should not trigger contract_repaired")
	}
}

func TestValidateAndRepairAI1OutputV2_RepairsMissingRequired(t *testing.T) {
	meta, eventMeta, executablePlan, trace, requestedFormats := buildAI1ContractTestFixture()
	raw := map[string]interface{}{
		"schema_version": "broken",
		"user_feedback": map[string]interface{}{
			"schema_version":     "broken",
			"interactive_action": "proceed",
			"risk_warning":       map[string]interface{}{},
		},
		"ai2_directive": map[string]interface{}{
			"schema_version": "broken",
		},
		"trace": map[string]interface{}{},
	}
	got := validateAndRepairAI1OutputV2(
		raw,
		VideoJobAI1PlanSchemaPNGV1,
		requestedFormats,
		meta,
		eventMeta,
		executablePlan,
		trace,
	)
	if !hasRequiredAI1OutputV2(got) {
		t.Fatalf("expected repaired ai1_output_v2 to be valid")
	}
	traceOut := mapFromAny(got["trace"])
	if !boolFromAny(traceOut["contract_repaired"]) {
		t.Fatalf("expected contract_repaired=true")
	}
	repairItems := stringSliceFromAny(traceOut["repair_items"])
	if len(repairItems) == 0 {
		t.Fatalf("expected repair_items to be populated")
	}
}

func TestValidateAndRepairAI1OutputV2_InvalidEnumsRepaired(t *testing.T) {
	meta, eventMeta, executablePlan, trace, requestedFormats := buildAI1ContractTestFixture()
	raw := buildAI1OutputV2(
		VideoJobAI1PlanSchemaPNGV1,
		requestedFormats,
		meta,
		eventMeta,
		executablePlan,
		trace,
	)
	userFeedback := mapFromAny(raw["user_feedback"])
	userFeedback["interactive_action"] = "hold"
	raw["user_feedback"] = userFeedback

	ai2Directive := mapFromAny(raw["ai2_directive"])
	ai2Directive["visual_focus_area"] = "left"
	ai2Directive["rhythm_trajectory"] = "random"
	technicalReject := mapFromAny(ai2Directive["technical_reject"])
	technicalReject["max_blur_tolerance"] = "extreme"
	ai2Directive["technical_reject"] = technicalReject
	raw["ai2_directive"] = ai2Directive

	got := validateAndRepairAI1OutputV2(
		raw,
		VideoJobAI1PlanSchemaPNGV1,
		requestedFormats,
		meta,
		eventMeta,
		executablePlan,
		trace,
	)

	userFeedbackOut := mapFromAny(got["user_feedback"])
	if action := strings.TrimSpace(stringFromAny(userFeedbackOut["interactive_action"])); action != "proceed" {
		t.Fatalf("unexpected interactive_action after repair: %s", action)
	}

	ai2DirectiveOut := mapFromAny(got["ai2_directive"])
	if focus := strings.TrimSpace(stringFromAny(ai2DirectiveOut["visual_focus_area"])); focus != "auto" {
		t.Fatalf("expected visual_focus_area=auto, got %s", focus)
	}
	technicalRejectOut := mapFromAny(ai2DirectiveOut["technical_reject"])
	if blur := strings.TrimSpace(stringFromAny(technicalRejectOut["max_blur_tolerance"])); blur != "low" {
		t.Fatalf("expected max_blur_tolerance=low, got %s", blur)
	}
	if rhythm := strings.TrimSpace(stringFromAny(ai2DirectiveOut["rhythm_trajectory"])); rhythm != "start_peak_fade" {
		t.Fatalf("unexpected rhythm_trajectory after repair: %s", rhythm)
	}
	if !boolFromAny(mapFromAny(got["trace"])["contract_repaired"]) {
		t.Fatalf("expected contract_repaired=true for enum fixes")
	}
}

func TestValidateAndRepairAI1OutputV2_EmptyInputFallback(t *testing.T) {
	meta, eventMeta, executablePlan, trace, requestedFormats := buildAI1ContractTestFixture()
	got := validateAndRepairAI1OutputV2(
		nil,
		VideoJobAI1PlanSchemaPNGV1,
		requestedFormats,
		meta,
		eventMeta,
		executablePlan,
		trace,
	)
	if !hasRequiredAI1OutputV2(got) {
		t.Fatalf("expected fallback safe envelope to be valid")
	}
	traceOut := mapFromAny(got["trace"])
	if !boolFromAny(traceOut["contract_repaired"]) {
		t.Fatalf("expected fallback to mark contract_repaired")
	}
	if reason := strings.TrimSpace(stringFromAny(traceOut["repair_reason"])); reason != "empty_output" {
		t.Fatalf("unexpected repair_reason: %s", reason)
	}
	repairItems := stringSliceFromAny(traceOut["repair_items"])
	if !containsString(repairItems, "fallback.safe_envelope") {
		t.Fatalf("expected fallback.safe_envelope in repair_items, got %#v", repairItems)
	}
}

func TestValidateAndRepairAI1OutputV2_LowConfidenceForcesClarify(t *testing.T) {
	meta, eventMeta, executablePlan, trace, requestedFormats := buildAI1ContractTestFixture()
	eventMeta["error"] = "director timeout"
	raw := buildAI1OutputV2(
		VideoJobAI1PlanSchemaPNGV1,
		requestedFormats,
		meta,
		eventMeta,
		executablePlan,
		trace,
	)
	userFeedback := mapFromAny(raw["user_feedback"])
	userFeedback["interactive_action"] = "proceed"
	userFeedback["confidence"] = 0.2
	userFeedback["clarify_questions"] = []interface{}{}
	raw["user_feedback"] = userFeedback

	got := validateAndRepairAI1OutputV2(
		raw,
		VideoJobAI1PlanSchemaPNGV1,
		requestedFormats,
		meta,
		eventMeta,
		executablePlan,
		trace,
	)
	userFeedbackOut := mapFromAny(got["user_feedback"])
	if action := strings.TrimSpace(stringFromAny(userFeedbackOut["interactive_action"])); action != "need_clarify" {
		t.Fatalf("expected interactive_action=need_clarify, got %s", action)
	}
	if len(stringSliceFromAny(userFeedbackOut["clarify_questions"])) == 0 {
		t.Fatalf("expected clarify_questions to be auto generated when need_clarify")
	}
}
