package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"emoji/internal/models"
	"emoji/internal/videojobs"
)

const (
	videoAIPromptTemplateSchemaMaxBytes      = 96 * 1024
	videoAIPromptTemplateSchemaMaxDepth      = 10
	videoAIPromptTemplateSchemaMaxNodes      = 4096
	videoAIPromptTemplateSchemaMaxObjectKeys = 256
	videoAIPromptTemplateSchemaMaxArrayItems = 256
	videoAIPromptTemplateSchemaMaxStringLen  = 4096
	videoAIPromptTemplateMaxVersionLen       = 64
	videoAIPromptTemplateMinCountBias        = 1
	videoAIPromptTemplateMaxCountBias        = 80
	videoAIPromptTemplateMaxSceneEntries     = 64
	videoAIPromptTemplateMaxTagItems         = 16
	videoAIPromptTemplateMaxTagLen           = 120
	videoAIPromptTemplateWeightsTolerance    = 0.08
)

type promptTemplateValidationIssue struct {
	FieldPath string `json:"field_path"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type promptTemplateValidationError struct {
	Issues []promptTemplateValidationIssue `json:"issues"`
}

func (e *promptTemplateValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "template validation failed"
	}
	return strings.TrimSpace(e.Issues[0].Message)
}

func newPromptTemplateValidationError(fieldPath, code, message string) error {
	fieldPath = strings.TrimSpace(fieldPath)
	if fieldPath == "" {
		fieldPath = "template"
	}
	code = strings.TrimSpace(strings.ToLower(code))
	if code == "" {
		code = "invalid_template"
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "template validation failed"
	}
	return &promptTemplateValidationError{
		Issues: []promptTemplateValidationIssue{
			{
				FieldPath: fieldPath,
				Code:      code,
				Message:   message,
			},
		},
	}
}

func wrapPromptTemplateValidationError(err error, defaultPath, defaultCode string) error {
	if err == nil {
		return nil
	}
	var validationErr *promptTemplateValidationError
	if errors.As(err, &validationErr) && validationErr != nil {
		return validationErr
	}
	fieldPath := extractValidationFieldPath(err.Error(), defaultPath)
	return newPromptTemplateValidationError(fieldPath, defaultCode, err.Error())
}

var validationFieldPathPattern = regexp.MustCompile(`^([a-zA-Z0-9_.$\[\]-]+)\s`)

func extractValidationFieldPath(message string, defaultPath string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return strings.TrimSpace(defaultPath)
	}
	if matches := validationFieldPathPattern.FindStringSubmatch(message); len(matches) > 1 {
		candidate := strings.TrimSpace(matches[1])
		if candidate != "" {
			return candidate
		}
	}
	return strings.TrimSpace(defaultPath)
}

func validateVideoAIPromptTemplateBeforeSave(
	format string,
	stage string,
	layer string,
	templateText string,
	templateSchema map[string]interface{},
) error {
	if _, err := normalizeVideoAIPromptTemplateFormat(format); err != nil {
		return newPromptTemplateValidationError("format", "invalid_format", err.Error())
	}
	normalizedStage, err := normalizeVideoAIPromptTemplateStage(stage)
	if err != nil {
		return newPromptTemplateValidationError("stage", "invalid_stage", err.Error())
	}
	normalizedLayer, err := normalizeVideoAIPromptTemplateLayer(layer, normalizedStage)
	if err != nil {
		return newPromptTemplateValidationError("layer", "invalid_layer", err.Error())
	}
	if _, err := normalizeVideoAIPromptTemplateText(templateText); err != nil {
		return newPromptTemplateValidationError("template_text", "invalid_template_text", err.Error())
	}
	if err := validateVideoAIPromptTemplateSchema(
		strings.ToLower(strings.TrimSpace(format)),
		normalizedStage,
		normalizedLayer,
		templateSchema,
	); err != nil {
		return wrapPromptTemplateValidationError(err, "template_json_schema", "invalid_template_json_schema")
	}
	return nil
}

func validateVideoAIPromptTemplateBeforeActivate(row models.VideoAIPromptTemplate) error {
	if strings.TrimSpace(row.Version) == "" {
		return newPromptTemplateValidationError("version", "invalid_version", "version cannot be empty")
	}
	if len(strings.TrimSpace(row.Version)) > videoAIPromptTemplateMaxVersionLen {
		return newPromptTemplateValidationError("version", "invalid_version", fmt.Sprintf("version length cannot exceed %d", videoAIPromptTemplateMaxVersionLen))
	}
	return validateVideoAIPromptTemplateBeforeSave(
		strings.ToLower(strings.TrimSpace(row.Format)),
		strings.ToLower(strings.TrimSpace(row.Stage)),
		strings.ToLower(strings.TrimSpace(row.Layer)),
		row.TemplateText,
		toJSONMap(row.TemplateJSONSchema),
	)
}

func validateVideoAIPromptTemplateSchema(format, stage, layer string, templateSchema map[string]interface{}) error {
	if err := validateVideoAIPromptTemplateSchemaBoundaries(templateSchema); err != nil {
		return err
	}
	if len(templateSchema) == 0 {
		return nil
	}

	switch {
	case stage == videoAIPromptTemplateStageAI1 && layer == videoAIPromptTemplateLayerEdit:
		if err := validateAI1EditableTemplateSchema(templateSchema); err != nil {
			return err
		}
	case stage == videoAIPromptTemplateStageAI1 && layer == videoAIPromptTemplateLayerFixed:
		// fixed 层允许不同模板策略，仅做通用边界校验
		return nil
	case stage == videoAIPromptTemplateStageAI2 && layer == videoAIPromptTemplateLayerFixed:
		return nil
	case stage == videoAIPromptTemplateStageScore && layer == videoAIPromptTemplateLayerFixed:
		return nil
	case stage == videoAIPromptTemplateStageAI3 && layer == videoAIPromptTemplateLayerFixed:
		return nil
	default:
		// 理论上不会进入，兜底保护
		return fmt.Errorf("unsupported format/stage/layer combination: %s/%s/%s", format, stage, layer)
	}
	return nil
}

func validateVideoAIPromptTemplateSchemaBoundaries(templateSchema map[string]interface{}) error {
	if templateSchema == nil {
		return nil
	}
	raw, err := json.Marshal(templateSchema)
	if err != nil {
		return fmt.Errorf("template_json_schema marshal failed: %w", err)
	}
	if len(raw) > videoAIPromptTemplateSchemaMaxBytes {
		return fmt.Errorf("template_json_schema exceeds %d bytes", videoAIPromptTemplateSchemaMaxBytes)
	}
	visited := 0
	return validateSchemaNode(templateSchema, "$", 1, &visited)
}

func validateSchemaNode(node interface{}, path string, depth int, visited *int) error {
	if depth > videoAIPromptTemplateSchemaMaxDepth {
		return fmt.Errorf("template_json_schema exceeds max depth %d at %s", videoAIPromptTemplateSchemaMaxDepth, path)
	}
	if visited != nil {
		*visited = *visited + 1
		if *visited > videoAIPromptTemplateSchemaMaxNodes {
			return fmt.Errorf("template_json_schema exceeds max nodes %d", videoAIPromptTemplateSchemaMaxNodes)
		}
	}

	switch value := node.(type) {
	case nil, bool:
		return nil
	case string:
		if len(value) > videoAIPromptTemplateSchemaMaxStringLen {
			return fmt.Errorf("string too long at %s", path)
		}
		return nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("invalid number at %s", path)
		}
		return nil
	case float32:
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("invalid number at %s", path)
		}
		return nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return nil
	case []interface{}:
		if len(value) > videoAIPromptTemplateSchemaMaxArrayItems {
			return fmt.Errorf("array too large at %s", path)
		}
		for idx, item := range value {
			if err := validateSchemaNode(item, fmt.Sprintf("%s[%d]", path, idx), depth+1, visited); err != nil {
				return err
			}
		}
		return nil
	case map[string]interface{}:
		if len(value) > videoAIPromptTemplateSchemaMaxObjectKeys {
			return fmt.Errorf("object too large at %s", path)
		}
		for key, item := range value {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				return fmt.Errorf("empty key at %s", path)
			}
			if len(trimmed) > 128 {
				return fmt.Errorf("key too long at %s.%s", path, trimmed)
			}
			if err := validateSchemaNode(item, path+"."+trimmed, depth+1, visited); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported value type %T at %s", node, path)
	}
}

func validateAI1EditableTemplateSchema(templateSchema map[string]interface{}) error {
	root := mapFromAny(templateSchema["scene_strategies_v1"])
	if len(root) == 0 {
		root = mapFromAny(templateSchema["scene_strategies"])
	}
	if len(root) == 0 {
		return errors.New("ai1 editable template_json_schema must contain scene_strategies_v1")
	}

	scenes := mapFromAny(root["scenes"])
	if len(scenes) == 0 {
		scenes = map[string]interface{}{}
		for key, value := range root {
			switch strings.TrimSpace(key) {
			case "version", "schema_version":
				continue
			default:
				scenes[key] = value
			}
		}
	}
	if len(scenes) == 0 {
		return errors.New("scene_strategies_v1.scenes is required")
	}
	if len(scenes) > videoAIPromptTemplateMaxSceneEntries {
		return fmt.Errorf("scene_strategies_v1.scenes exceeds %d entries", videoAIPromptTemplateMaxSceneEntries)
	}

	enabledProfiles := 0
	for sceneKey, rawEntry := range scenes {
		entry := mapFromAny(rawEntry)
		if len(entry) == 0 {
			return fmt.Errorf("scene_strategies_v1.scenes.%s must be an object", strings.TrimSpace(sceneKey))
		}
		scenePath := "scene_strategies_v1.scenes." + strings.TrimSpace(sceneKey)

		if enabledValue, exists := entry["enabled"]; exists {
			enabled, ok := enabledValue.(bool)
			if !ok {
				return fmt.Errorf("%s.enabled must be boolean", scenePath)
			}
			if !enabled {
				continue
			}
		}
		enabledProfiles++

		if err := validateAI1CandidateCountBias(scenePath, mapFromAny(entry["candidate_count_bias"])); err != nil {
			return err
		}
		if err := validateAI1QualityWeights(scenePath, mapFromAny(entry["quality_weights"])); err != nil {
			return err
		}
		if err := validateOptionalStringArray(entry["must_capture_bias"], scenePath+".must_capture_bias"); err != nil {
			return err
		}
		if err := validateOptionalStringArray(entry["avoid_bias"], scenePath+".avoid_bias"); err != nil {
			return err
		}
		if err := validateOptionalStringArray(entry["risk_flags"], scenePath+".risk_flags"); err != nil {
			return err
		}
		if err := validateAI1TechnicalReject(scenePath, mapFromAny(entry["technical_reject"])); err != nil {
			return err
		}
	}

	if enabledProfiles == 0 {
		return errors.New("scene_strategies_v1 must contain at least one enabled scene profile")
	}

	// 复用流水线侧解析器做二次约束，确保结构可被运行时消费。
	raw, err := json.Marshal(templateSchema)
	if err != nil {
		return fmt.Errorf("template_json_schema marshal failed: %w", err)
	}
	profiles, _ := videojobs.DecodeAI1SceneStrategyProfilesFromTemplateSchema(raw)
	if len(profiles) == 0 {
		return errors.New("scene_strategies_v1 is invalid or has no usable scene profiles")
	}

	return nil
}

func validateAI1CandidateCountBias(scenePath string, bias map[string]interface{}) error {
	if len(bias) == 0 {
		return nil
	}
	minValue, hasMin, minErr := parseOptionalInteger(bias["min"])
	if minErr != nil {
		return fmt.Errorf("%s.candidate_count_bias.min must be integer", scenePath)
	}
	maxValue, hasMax, maxErr := parseOptionalInteger(bias["max"])
	if maxErr != nil {
		return fmt.Errorf("%s.candidate_count_bias.max must be integer", scenePath)
	}
	if hasMin {
		if minValue < videoAIPromptTemplateMinCountBias || minValue > videoAIPromptTemplateMaxCountBias {
			return fmt.Errorf("%s.candidate_count_bias.min out of range [%d,%d]", scenePath, videoAIPromptTemplateMinCountBias, videoAIPromptTemplateMaxCountBias)
		}
	}
	if hasMax {
		if maxValue < videoAIPromptTemplateMinCountBias || maxValue > videoAIPromptTemplateMaxCountBias {
			return fmt.Errorf("%s.candidate_count_bias.max out of range [%d,%d]", scenePath, videoAIPromptTemplateMinCountBias, videoAIPromptTemplateMaxCountBias)
		}
	}
	if hasMin && hasMax && maxValue < minValue {
		return fmt.Errorf("%s.candidate_count_bias.max must be >= min", scenePath)
	}
	return nil
}

func validateAI1QualityWeights(scenePath string, weights map[string]interface{}) error {
	if len(weights) == 0 {
		return nil
	}
	keys := []string{"semantic", "clarity", "loop", "efficiency"}
	sum := 0.0
	for _, key := range keys {
		value, exists := weights[key]
		if !exists {
			return fmt.Errorf("%s.quality_weights.%s is required", scenePath, key)
		}
		num, err := parseNumeric(value)
		if err != nil {
			return fmt.Errorf("%s.quality_weights.%s must be numeric", scenePath, key)
		}
		if num < 0 || num > 1 {
			return fmt.Errorf("%s.quality_weights.%s must be within [0,1]", scenePath, key)
		}
		sum += num
	}
	if math.Abs(sum-1.0) > videoAIPromptTemplateWeightsTolerance {
		return fmt.Errorf("%s.quality_weights sum must be close to 1", scenePath)
	}
	return nil
}

func validateOptionalStringArray(raw interface{}, path string) error {
	if raw == nil {
		return nil
	}
	values, err := parseStringArray(raw)
	if err != nil {
		return fmt.Errorf("%s must be string array", path)
	}
	if len(values) > videoAIPromptTemplateMaxTagItems {
		return fmt.Errorf("%s exceeds %d items", path, videoAIPromptTemplateMaxTagItems)
	}
	for _, item := range values {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if len(strings.TrimSpace(item)) > videoAIPromptTemplateMaxTagLen {
			return fmt.Errorf("%s contains too long item", path)
		}
	}
	return nil
}

func validateAI1TechnicalReject(scenePath string, reject map[string]interface{}) error {
	if len(reject) == 0 {
		return nil
	}
	if value, exists := reject["max_blur_tolerance"]; exists {
		normalized := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", value)))
		switch normalized {
		case "", "low", "medium", "high":
		default:
			return fmt.Errorf("%s.technical_reject.max_blur_tolerance must be one of low/medium/high", scenePath)
		}
	}
	for _, key := range []string{"avoid_watermarks", "avoid_extreme_dark"} {
		if value, exists := reject[key]; exists {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s.technical_reject.%s must be boolean", scenePath, key)
			}
		}
	}
	return nil
}

func parseStringArray(raw interface{}) ([]string, error) {
	switch value := raw.(type) {
	case []string:
		return value, nil
	case []interface{}:
		out := make([]string, 0, len(value))
		for _, item := range value {
			str, ok := item.(string)
			if !ok {
				return nil, errors.New("non-string item")
			}
			out = append(out, str)
		}
		return out, nil
	default:
		return nil, errors.New("not array")
	}
}

func parseNumeric(raw interface{}) (float64, error) {
	switch value := raw.(type) {
	case float64:
		return value, nil
	case float32:
		return float64(value), nil
	case int:
		return float64(value), nil
	case int8:
		return float64(value), nil
	case int16:
		return float64(value), nil
	case int32:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case uint:
		return float64(value), nil
	case uint8:
		return float64(value), nil
	case uint16:
		return float64(value), nil
	case uint32:
		return float64(value), nil
	case uint64:
		return float64(value), nil
	case json.Number:
		return value.Float64()
	default:
		return 0, errors.New("not numeric")
	}
}

func parseOptionalInteger(raw interface{}) (int, bool, error) {
	if raw == nil {
		return 0, false, nil
	}
	num, err := parseNumeric(raw)
	if err != nil {
		return 0, false, err
	}
	if math.Trunc(num) != num {
		return 0, false, errors.New("not integer")
	}
	return int(num), true, nil
}
