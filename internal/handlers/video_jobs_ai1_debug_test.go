package handlers

import (
	"testing"

	"emoji/internal/videojobs"
)

func TestBuildAI2ExecutionObservability_CollectsGuidedSignals(t *testing.T) {
	options := map[string]interface{}{
		"ai2_quality_weights_v1": map[string]interface{}{
			"semantic":   0.45,
			"clarity":    0.25,
			"loop":       0.10,
			"efficiency": 0.20,
		},
		"ai2_risk_flags_v1": []interface{}{"low_light", "fast_motion"},
		"ai2_technical_reject_v1": map[string]interface{}{
			"max_blur_tolerance": "low",
		},
	}
	metrics := map[string]interface{}{
		"frame_quality": map[string]interface{}{
			"selector_version": "v2_ai2_guided_ranker",
			"scoring_mode":     "weighted_quality_v1",
			"scoring_formula":  "final = Σ(base_scores×weights) + directive_hit_adjust(must_capture,avoid)",
			"selection_policy": "ai2_global_quality_first",
			"candidate_scores": []interface{}{
				map[string]interface{}{
					"rank":              1,
					"frame_name":        "frame_0001.jpg",
					"decision":          "kept",
					"must_capture_hits": []interface{}{"人物特写"},
					"avoid_hits":        []interface{}{},
					"explain_summary":   "保留：命中 must_capture 1 项，触发 avoid 0 项，综合分 0.81。",
				},
			},
			"scoring_weights": map[string]interface{}{
				"semantic":   0.50,
				"clarity":    0.20,
				"loop":       0.10,
				"efficiency": 0.20,
			},
			"risk_flags":         []interface{}{"low_light", "fast_motion"},
			"max_blur_tolerance": "low",
			"kept_frames":        12,
			"total_frames":       24,
			"rejected_blur":      4,
			"rejected_watermark": 2,
		},
		"png_worker_strategy_v1": map[string]interface{}{
			"risk_flags": []interface{}{"low_light", "fast_motion"},
		},
	}
	ai2Instruction := map[string]interface{}{
		"quality_weights": map[string]interface{}{
			"semantic":   0.35,
			"clarity":    0.30,
			"loop":       0.15,
			"efficiency": 0.20,
		},
		"risk_flags": []interface{}{"low_light"},
		"technical_reject": map[string]interface{}{
			"max_blur_tolerance": "low",
		},
	}

	out := buildAI2ExecutionObservability("png", options, metrics, ai2Instruction)
	if stringFromAny(out["schema_version"]) != "ai2_execution_observability_v1" {
		t.Fatalf("unexpected schema version: %+v", out)
	}
	if stringFromAny(out["selector_version"]) != "v2_ai2_guided_ranker" {
		t.Fatalf("expected selector version from frame_quality, got %+v", out["selector_version"])
	}
	if stringFromAny(out["scoring_mode"]) != "weighted_quality_v1" {
		t.Fatalf("expected scoring mode weighted_quality_v1, got %+v", out["scoring_mode"])
	}
	if stringFromAny(out["scoring_formula"]) == "" {
		t.Fatalf("expected non-empty scoring_formula, got %+v", out["scoring_formula"])
	}
	if stringFromAny(out["selection_policy"]) != "ai2_global_quality_first" {
		t.Fatalf("expected selection_policy ai2_global_quality_first, got %+v", out["selection_policy"])
	}
	candidateExplainability := mapFromAnyValue(out["candidate_explainability"])
	if intFromAnyValue(candidateExplainability["total_candidates"]) != 1 {
		t.Fatalf("expected candidate_explainability.total_candidates=1, got %+v", candidateExplainability)
	}
	topRows := parseMapSliceFromAny(candidateExplainability["top_rows"])
	if len(topRows) == 0 {
		t.Fatalf("expected candidate_explainability.top_rows, got %+v", candidateExplainability)
	}
	if stringFromAny(topRows[0]["decision"]) != "kept" {
		t.Fatalf("expected first top row decision kept, got %+v", topRows[0]["decision"])
	}
	if stringFromAny(out["max_blur_tolerance"]) != "low" {
		t.Fatalf("expected max_blur_tolerance low, got %+v", out["max_blur_tolerance"])
	}
	weights, ok := out["effective_quality_weights"].(map[string]float64)
	if !ok || len(weights) != 4 {
		t.Fatalf("expected 4 effective quality weights, got %+v", out["effective_quality_weights"])
	}
	flags := normalizeAI2RiskFlagsForDebug(out["risk_flags_applied"])
	if len(flags) == 0 {
		t.Fatalf("expected risk_flags_applied, got %+v", out)
	}
	frameSummary := mapFromAnyValue(out["frame_quality_summary"])
	rejectCounts := mapFromAnyValue(frameSummary["reject_counts"])
	if intFromAnyValue(rejectCounts["watermark"]) != 2 {
		t.Fatalf("expected reject_counts.watermark=2, got %+v", rejectCounts)
	}
}

func TestBuildAI1DebugTimelineV1_IncludesAI2ExecutionStep(t *testing.T) {
	execution := map[string]interface{}{
		"schema_version":   "ai2_execution_observability_v1",
		"selector_version": "v2_ai2_guided_ranker",
		"scoring_mode":     "weighted_quality_v1",
		"effective_quality_weights": map[string]float64{
			"semantic":   0.4,
			"clarity":    0.3,
			"loop":       0.1,
			"efficiency": 0.2,
		},
	}
	timeline := buildAI1DebugTimelineV1(
		"test prompt",
		"png",
		map[string]interface{}{"user": map[string]interface{}{}},
		map[string]interface{}{"request_status": "ok"},
		map[string]interface{}{"response_summary_v2": map[string]interface{}{"request_status": "ok"}},
		map[string]interface{}{"user_reply": map[string]interface{}{"summary": "ok"}, "ai2_instruction": map[string]interface{}{"objective": "extract"}},
		map[string]interface{}{"valid": true},
		execution,
	)

	found := false
	for _, step := range timeline {
		if stringFromAny(step["key"]) != "ai2_execution" {
			continue
		}
		found = true
		if stringFromAny(step["status"]) != "done" {
			t.Fatalf("expected ai2_execution step status done, got %+v", step["status"])
		}
	}
	if !found {
		t.Fatalf("expected ai2_execution step in timeline, got %+v", timeline)
	}
}

func TestBuildAI1OutputContractReport_RequiresAI2QualityWeights(t *testing.T) {
	ai1Output := map[string]interface{}{
		"schema_version": videojobs.AI1OutputSchemaV2,
		"user_feedback": map[string]interface{}{
			"schema_version":       videojobs.AI1UserFeedbackSchemaV2,
			"summary":              "ok",
			"intent_understanding": "ok",
			"strategy_summary":     "ok",
			"interactive_action":   "proceed",
			"risk_warning": map[string]interface{}{
				"has_risk": false,
			},
			"confidence":        0.8,
			"clarify_questions": []interface{}{},
		},
		"ai2_directive": map[string]interface{}{
			"schema_version": videojobs.AI2DirectiveSchemaV2,
			"target_format":  "png",
			"objective":      "extract_high_quality_frames",
			"sampling_plan":  map[string]interface{}{"mode": "uniform"},
			"technical_reject": map[string]interface{}{
				"max_blur_tolerance": "low",
			},
			"visual_focus_area": "auto",
			"rhythm_trajectory": "start_peak_fade",
		},
		"trace": map[string]interface{}{},
	}

	report := buildAI1OutputContractReport(ai1Output)
	missing := parseStringSliceFromAny(report["missing_fields"])
	hasQualityWeightsMissing := false
	for _, item := range missing {
		if item == "ai2_directive.quality_weights" {
			hasQualityWeightsMissing = true
			break
		}
	}
	if !hasQualityWeightsMissing {
		t.Fatalf("expected ai2_directive.quality_weights in missing_fields, got %+v", missing)
	}
}

func TestBuildPipelineAlignmentReportV1_ProducesConsistencyChecks(t *testing.T) {
	ai2Execution := map[string]interface{}{
		"effective_quality_weights": map[string]float64{
			"semantic":   0.4,
			"clarity":    0.4,
			"loop":       0.1,
			"efficiency": 0.1,
		},
		"technical_reject": map[string]interface{}{
			"avoid_watermarks": true,
		},
		"frame_quality_summary": map[string]interface{}{
			"reject_counts": map[string]interface{}{
				"watermark": 2,
			},
		},
	}
	ai2Instruction := map[string]interface{}{
		"style_direction": "封面级清晰特写",
		"quality_weights": map[string]interface{}{
			"semantic":   0.4,
			"clarity":    0.4,
			"loop":       0.1,
			"efficiency": 0.1,
		},
		"technical_reject": map[string]interface{}{
			"avoid_watermarks": true,
		},
	}
	strategyProfile := map[string]interface{}{
		"scene":           "xiaohongshu",
		"scene_label":     "小红书网感",
		"style_direction": "封面级清晰特写",
	}
	metrics := map[string]interface{}{
		"png_pipeline_stage_status_v1": map[string]interface{}{
			"ai1":    "done",
			"ai2":    "done",
			"worker": "done",
			"ai3":    "done",
		},
		"png_ai3_review_v1": map[string]interface{}{
			"recommendation": "deliver",
		},
	}

	report := buildPipelineAlignmentReportV1(
		"png",
		"帮我挑几张封面图",
		map[string]interface{}{
			"scene":        "xiaohongshu",
			"visual_focus": []interface{}{"portrait"},
		},
		strategyProfile,
		map[string]interface{}{"override_count": 2},
		map[string]interface{}{},
		map[string]interface{}{"summary": "已理解你的需求"},
		ai2Instruction,
		ai2Execution,
		map[string]interface{}{},
		metrics,
	)
	if stringFromAny(report["schema_version"]) != "pipeline_alignment_report_v1" {
		t.Fatalf("unexpected report schema: %+v", report)
	}
	sceneBaselineDiff := mapFromAnyValue(report["scene_baseline_diff_v1"])
	if stringFromAny(sceneBaselineDiff["schema_version"]) != "scene_baseline_diff_v1" {
		t.Fatalf("expected scene_baseline_diff_v1 in report, got %+v", sceneBaselineDiff)
	}
	if !boolFromAny(sceneBaselineDiff["scene_changed"]) {
		t.Fatalf("expected scene_changed=true for xiaohongshu, got %+v", sceneBaselineDiff["scene_changed"])
	}
	if len(parseStringSliceFromAny(sceneBaselineDiff["weight_diff_keys"])) == 0 {
		t.Fatalf("expected non-empty weight_diff_keys for xiaohongshu baseline diff, got %+v", sceneBaselineDiff)
	}

	scene := mapFromAnyValue(report["scenario"])
	if stringFromAny(scene["scene"]) != "xiaohongshu" {
		t.Fatalf("expected scene xiaohongshu, got %+v", scene["scene"])
	}
	summary := mapFromAnyValue(report["summary"])
	if stringFromAny(summary["status"]) == "fail" {
		t.Fatalf("expected non-fail summary, got %+v", summary)
	}

	checks := parseMapSliceFromAny(report["consistency_checks"])
	if len(checks) == 0 {
		t.Fatalf("expected consistency checks, got %+v", report)
	}
	if got := findCheckStatus(checks, "quality_weights_applied"); got != "pass" {
		t.Fatalf("expected quality_weights_applied=pass, got %s", got)
	}
	if got := findCheckStatus(checks, "watermark_gate_active"); got != "pass" {
		t.Fatalf("expected watermark_gate_active=pass, got %s", got)
	}
	if got := findCheckStatus(checks, "scene_baseline_diff"); got != "pass" {
		t.Fatalf("expected scene_baseline_diff=pass, got %s", got)
	}
	if got := findCheckStatus(checks, "ai3_review_available"); got != "pass" {
		t.Fatalf("expected ai3_review_available=pass, got %s", got)
	}
}

func parseMapSliceFromAny(raw interface{}) []map[string]interface{} {
	if rows, ok := raw.([]map[string]interface{}); ok {
		return rows
	}
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		value, ok := item.(map[string]interface{})
		if !ok || len(value) == 0 {
			continue
		}
		out = append(out, value)
	}
	return out
}

func findCheckStatus(checks []map[string]interface{}, key string) string {
	normalized := key
	for _, item := range checks {
		if stringFromAny(item["key"]) != normalized {
			continue
		}
		return stringFromAny(item["status"])
	}
	return ""
}
