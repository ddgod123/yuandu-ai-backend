package handlers

import (
	"strings"
	"testing"

	"emoji/internal/models"
)

func TestValidateVideoAIPromptTemplateBeforeSave_AI1EditableSchemaValid(t *testing.T) {
	schema := map[string]interface{}{
		"scene_strategies_v1": map[string]interface{}{
			"version": "ai1_strategy_profile_v1",
			"scenes": map[string]interface{}{
				"default": map[string]interface{}{
					"scene": "default",
					"candidate_count_bias": map[string]interface{}{
						"min": 4,
						"max": 8,
					},
					"quality_weights": map[string]interface{}{
						"semantic":   0.40,
						"clarity":    0.30,
						"loop":       0.10,
						"efficiency": 0.20,
					},
					"must_capture_bias": []interface{}{"主体清晰"},
					"avoid_bias":        []interface{}{"严重模糊"},
					"risk_flags":        []interface{}{"low_light"},
					"technical_reject": map[string]interface{}{
						"max_blur_tolerance": "low",
						"avoid_watermarks":   true,
						"avoid_extreme_dark": true,
					},
				},
			},
		},
	}
	if err := validateVideoAIPromptTemplateBeforeSave("all", "ai1", "editable", "prompt", schema); err != nil {
		t.Fatalf("expected valid schema, got err=%v", err)
	}
}

func TestValidateVideoAIPromptTemplateBeforeSave_AI1EditableMissingSceneStrategies(t *testing.T) {
	schema := map[string]interface{}{
		"foo": "bar",
	}
	err := validateVideoAIPromptTemplateBeforeSave("all", "ai1", "editable", "prompt", schema)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "scene_strategies_v1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateVideoAIPromptTemplateBeforeSave_AI1EditableCandidateCountBiasInvalid(t *testing.T) {
	schema := map[string]interface{}{
		"scene_strategies_v1": map[string]interface{}{
			"scenes": map[string]interface{}{
				"default": map[string]interface{}{
					"candidate_count_bias": map[string]interface{}{
						"min": 7,
						"max": 4,
					},
				},
			},
		},
	}
	err := validateVideoAIPromptTemplateBeforeSave("all", "ai1", "editable", "prompt", schema)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "candidate_count_bias.max must be >= min") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateVideoAIPromptTemplateBeforeSave_BoundaryDepthExceeded(t *testing.T) {
	current := map[string]interface{}{"leaf": "v"}
	for i := 0; i < 12; i++ {
		current = map[string]interface{}{
			"level": current,
		}
	}
	deep := current
	err := validateVideoAIPromptTemplateBeforeSave("all", "ai2", "fixed", "prompt", deep)
	if err == nil {
		t.Fatalf("expected depth validation error")
	}
	if !strings.Contains(err.Error(), "max depth") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateVideoAIPromptTemplateBeforeActivate_BlocksInvalidSchema(t *testing.T) {
	row := models.VideoAIPromptTemplate{
		Format:             "all",
		Stage:              "ai1",
		Layer:              "editable",
		TemplateText:       "prompt",
		Version:            "v1",
		TemplateJSONSchema: toJSONBOrDefault(map[string]interface{}{"foo": "bar"}, nil),
	}
	err := validateVideoAIPromptTemplateBeforeActivate(row)
	if err == nil {
		t.Fatalf("expected activation validation error")
	}
}

func TestValidateVideoAIPromptTemplateBeforeSave_FixedLayerAllowsSchemaWhenWithinBoundary(t *testing.T) {
	schema := map[string]interface{}{
		"schema_version": "v2",
		"fields": map[string]interface{}{
			"directive": map[string]interface{}{
				"type":     "object",
				"required": []interface{}{"business_goal"},
			},
		},
	}
	if err := validateVideoAIPromptTemplateBeforeSave("png", "ai1", "fixed", "prompt", schema); err != nil {
		t.Fatalf("expected fixed schema valid, got err=%v", err)
	}
}
