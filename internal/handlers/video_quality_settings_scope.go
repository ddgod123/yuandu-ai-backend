package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	videoQualitySettingFormatAll  = "all"
	videoQualitySettingFormatGIF  = "gif"
	videoQualitySettingFormatPNG  = "png"
	videoQualitySettingFormatJPG  = "jpg"
	videoQualitySettingFormatWebP = "webp"
	videoQualitySettingFormatLive = "live"
	videoQualitySettingFormatMP4  = "mp4"
)

func normalizeVideoQualitySettingFormatScope(raw string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(raw))
	if format == "" {
		return videoQualitySettingFormatAll, nil
	}
	if format == "jpeg" {
		format = "jpg"
	}
	switch format {
	case videoQualitySettingFormatAll,
		videoQualitySettingFormatGIF,
		videoQualitySettingFormatPNG,
		videoQualitySettingFormatJPG,
		videoQualitySettingFormatWebP,
		videoQualitySettingFormatLive,
		videoQualitySettingFormatMP4:
		return format, nil
	default:
		return "", errors.New("invalid format, expected one of all/gif/png/jpg/webp/live/mp4")
	}
}

func videoQualitySettingRequestToMap(req VideoQualitySettingRequest) (map[string]interface{}, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	delete(out, "created_at")
	delete(out, "updated_at")
	return out, nil
}

func videoQualitySettingRequestFromMap(payload map[string]interface{}) (VideoQualitySettingRequest, error) {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return VideoQualitySettingRequest{}, err
	}
	var req VideoQualitySettingRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return VideoQualitySettingRequest{}, err
	}
	return req, nil
}

func mergeVideoQualitySettingMaps(base, override map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range override {
		if key == "created_at" || key == "updated_at" {
			continue
		}
		out[key] = value
	}
	return out
}

func diffVideoQualitySettingMaps(base, target map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for key, value := range target {
		if key == "created_at" || key == "updated_at" {
			continue
		}
		if baseValue, ok := base[key]; ok && reflect.DeepEqual(baseValue, value) {
			continue
		}
		out[key] = value
	}
	return out
}

func buildVideoQualitySettingModelFromRequest(req VideoQualitySettingRequest, base models.VideoQualitySetting) (models.VideoQualitySetting, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return models.VideoQualitySetting{}, err
	}

	var quality videojobs.QualitySettings
	if err := json.Unmarshal(raw, &quality); err != nil {
		return models.VideoQualitySetting{}, err
	}
	quality = videojobs.NormalizeQualitySettings(quality)

	var gifThresholds GIFHealthAlertThresholdSettings
	if err := json.Unmarshal(raw, &gifThresholds); err != nil {
		return models.VideoQualitySetting{}, err
	}
	gifThresholds = normalizeGIFHealthAlertThresholdSettings(gifThresholds)

	var feedbackThresholds FeedbackIntegrityAlertThresholdSettings
	if err := json.Unmarshal(raw, &feedbackThresholds); err != nil {
		return models.VideoQualitySetting{}, err
	}
	feedbackThresholds = normalizeFeedbackIntegrityAlertThresholdSettings(feedbackThresholds)

	out := base
	applyQualitySettingsToModel(&out, quality)
	applyGIFHealthAlertThresholdSettingsToModel(&out, gifThresholds)
	applyFeedbackIntegrityAlertThresholdSettingsToModel(&out, feedbackThresholds)
	return out, nil
}

func (h *Handler) loadActiveVideoQualityScope(format string) (*models.VideoQualitySettingScoped, error) {
	if h == nil || h.db == nil {
		return nil, nil
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" || format == videoQualitySettingFormatAll {
		return nil, nil
	}

	var row models.VideoQualitySettingScoped
	err := h.db.Where("format = ? AND is_active = ?", format, true).Order("id DESC").First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableError(err, "video_quality_settings_scoped") {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (h *Handler) resolveVideoQualitySettingByFormat(format string) (models.VideoQualitySetting, []string, *models.VideoQualitySettingScoped, error) {
	base, err := h.loadVideoQualitySetting()
	if err != nil {
		return models.VideoQualitySetting{}, nil, nil, err
	}
	if format == "" || format == videoQualitySettingFormatAll {
		return base, []string{videoQualitySettingFormatAll}, nil, nil
	}

	override, err := h.loadActiveVideoQualityScope(format)
	if err != nil {
		return models.VideoQualitySetting{}, nil, nil, err
	}
	if override == nil {
		return base, []string{videoQualitySettingFormatAll}, nil, nil
	}

	baseReq := videoQualitySettingRequestFromModel(base)
	baseMap, err := videoQualitySettingRequestToMap(baseReq)
	if err != nil {
		return models.VideoQualitySetting{}, nil, nil, err
	}
	overrideMap := parseJSONMap(override.SettingsJSON)
	mergedMap := mergeVideoQualitySettingMaps(baseMap, overrideMap)
	mergedReq, err := videoQualitySettingRequestFromMap(mergedMap)
	if err != nil {
		return models.VideoQualitySetting{}, nil, nil, err
	}
	effective, err := buildVideoQualitySettingModelFromRequest(mergedReq, base)
	if err != nil {
		return models.VideoQualitySetting{}, nil, nil, err
	}
	if override.UpdatedAt.After(effective.UpdatedAt) {
		effective.UpdatedAt = override.UpdatedAt
	}
	return effective, []string{videoQualitySettingFormatAll, format}, override, nil
}

func (h *Handler) saveVideoQualitySettingScope(format string, override map[string]interface{}, adminID uint64) (*models.VideoQualitySettingScoped, error) {
	if h == nil || h.db == nil {
		return nil, errors.New("database not configured")
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" || format == videoQualitySettingFormatAll {
		return nil, errors.New("scoped format is required")
	}
	if override == nil {
		override = map[string]interface{}{}
	}
	delete(override, "created_at")
	delete(override, "updated_at")

	overrideJSON, err := json.Marshal(override)
	if err != nil {
		return nil, err
	}

	var saved *models.VideoQualitySettingScoped
	err = h.db.Transaction(func(tx *gorm.DB) error {
		deactivate := map[string]interface{}{"is_active": false}
		if adminID > 0 {
			deactivate["updated_by"] = adminID
		}
		if err := tx.Model(&models.VideoQualitySettingScoped{}).
			Where("format = ? AND is_active = ?", format, true).
			Updates(deactivate).Error; err != nil {
			return err
		}

		if len(override) == 0 {
			saved = nil
			return nil
		}

		version := fmt.Sprintf("%s_%s", format, time.Now().UTC().Format("20060102T150405Z"))
		row := models.VideoQualitySettingScoped{
			Format:       format,
			Version:      version,
			IsActive:     true,
			SettingsJSON: datatypes.JSON(overrideJSON),
			CreatedBy:    adminID,
			UpdatedBy:    adminID,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		saved = &row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}
