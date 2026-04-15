package videojobs

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"emoji/internal/models"

	"gorm.io/gorm"
)

const (
	videoQualityScopeAll  = "all"
	videoQualityScopeGIF  = "gif"
	videoQualityScopePNG  = "png"
	videoQualityScopeJPG  = "jpg"
	videoQualityScopeWebP = "webp"
	videoQualityScopeLive = "live"
	videoQualityScopeMP4  = "mp4"
)

func normalizeRuntimeVideoQualityScope(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return videoQualityScopeAll
	}
	if value == "jpeg" {
		value = "jpg"
	}
	switch value {
	case videoQualityScopeAll,
		videoQualityScopeGIF,
		videoQualityScopePNG,
		videoQualityScopeJPG,
		videoQualityScopeWebP,
		videoQualityScopeLive,
		videoQualityScopeMP4:
		return value
	default:
		return videoQualityScopeAll
	}
}

func resolveRuntimeQualitySettingsFormatScope(requestedFormats []string) string {
	if containsString(requestedFormats, videoQualityScopeGIF) {
		return videoQualityScopeGIF
	}
	return firstRequestedFormat(requestedFormats)
}

func qualitySettingsToMap(in QualitySettings) map[string]interface{} {
	raw, err := json.Marshal(in)
	if err != nil {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func qualitySettingsFromMap(in map[string]interface{}) (QualitySettings, error) {
	if in == nil {
		in = map[string]interface{}{}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return QualitySettings{}, err
	}
	var out QualitySettings
	if err := json.Unmarshal(raw, &out); err != nil {
		return QualitySettings{}, err
	}
	return NormalizeQualitySettings(out), nil
}

func mergeQualitySettingsMap(base, override map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(base)+len(override))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range override {
		key := strings.TrimSpace(k)
		if key == "" || key == "created_at" || key == "updated_at" {
			continue
		}
		merged[key] = v
	}
	return merged
}

func sortedMapKeys(in map[string]interface{}) []string {
	if len(in) == 0 {
		return []string{}
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		k = strings.TrimSpace(k)
		if k == "" || k == "created_at" || k == "updated_at" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (p *Processor) loadActiveVideoQualityScope(format string) (*models.VideoQualitySettingScoped, error) {
	if p == nil || p.db == nil {
		return nil, nil
	}
	scope := normalizeRuntimeVideoQualityScope(format)
	if scope == videoQualityScopeAll {
		return nil, nil
	}

	var row models.VideoQualitySettingScoped
	err := p.db.Where("format = ? AND is_active = ?", scope, true).
		Order("id DESC").
		Limit(1).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableError(err, "video_quality_settings_scoped") {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (p *Processor) loadQualitySettingsByFormat(format string) (QualitySettings, map[string]interface{}) {
	base := p.loadQualitySettings()
	scope := normalizeRuntimeVideoQualityScope(format)
	meta := map[string]interface{}{
		"requested_format": scope,
		"resolved_format":  videoQualityScopeAll,
		"resolved_from":    []string{videoQualityScopeAll},
		"source_chain":     []string{"ops.video_quality_settings:all"},
		"override_keys":    []string{},
		"override_count":   0,
		"fallback_used":    scope != videoQualityScopeAll,
	}
	if scope == videoQualityScopeAll || p == nil || p.db == nil {
		return base, meta
	}

	row, err := p.loadActiveVideoQualityScope(scope)
	if err != nil {
		meta["resolution_error"] = err.Error()
		return base, meta
	}
	if row == nil {
		return base, meta
	}

	override := parseJSONMap(row.SettingsJSON)
	if len(override) == 0 {
		meta["resolved_format"] = scope
		meta["resolved_from"] = []string{videoQualityScopeAll, scope}
		meta["source_chain"] = []string{
			"ops.video_quality_settings:all",
			"ops.video_quality_settings_scoped:" + scope,
		}
		meta["fallback_used"] = false
		meta["scope_id"] = row.ID
		meta["scope_version"] = strings.TrimSpace(row.Version)
		return base, meta
	}

	merged, mergeErr := qualitySettingsFromMap(
		mergeQualitySettingsMap(qualitySettingsToMap(base), override),
	)
	if mergeErr != nil {
		meta["resolution_error"] = mergeErr.Error()
		return base, meta
	}

	overrideKeys := sortedMapKeys(override)
	meta["resolved_format"] = scope
	meta["resolved_from"] = []string{videoQualityScopeAll, scope}
	meta["source_chain"] = []string{
		"ops.video_quality_settings:all",
		"ops.video_quality_settings_scoped:" + scope,
	}
	meta["override_keys"] = overrideKeys
	meta["override_count"] = len(overrideKeys)
	meta["fallback_used"] = false
	meta["scope_id"] = row.ID
	meta["scope_version"] = strings.TrimSpace(row.Version)
	return merged, meta
}
