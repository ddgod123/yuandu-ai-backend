package handlers

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/videojobs"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	videoJobFeedbackSignalDownload = "download"
	videoJobFeedbackSignalFavorite = "favorite"

	videoImageFeedbackActionDownload = "download"
	videoImageFeedbackActionFavorite = "favorite"
	videoImageFeedbackActionShare    = "share"
	videoImageFeedbackActionUse      = "use"
	videoImageFeedbackActionLike     = "like"
	videoImageFeedbackActionNeutral  = "neutral"
	videoImageFeedbackActionDislike  = "dislike"
	videoImageFeedbackActionTopPick  = "top_pick"
)

func normalizeVideoJobFeedbackSignal(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case videoJobFeedbackSignalFavorite:
		return videoJobFeedbackSignalFavorite
	default:
		return videoJobFeedbackSignalDownload
	}
}

func normalizeVideoImageFeedbackAction(raw string) (string, float64) {
	if action, weight, ok := parseVideoImageFeedbackAction(raw); ok {
		return action, weight
	}
	return videoImageFeedbackActionDownload, 1.0
}

func parseVideoImageFeedbackAction(raw string) (string, float64, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case videoImageFeedbackActionDownload:
		return videoImageFeedbackActionDownload, 1.0, true
	case videoImageFeedbackActionFavorite:
		return videoImageFeedbackActionFavorite, 2.5, true
	case videoImageFeedbackActionShare:
		return videoImageFeedbackActionShare, 2.0, true
	case videoImageFeedbackActionUse:
		return videoImageFeedbackActionUse, 1.0, true
	case videoImageFeedbackActionLike, "thumb_up", "good":
		return videoImageFeedbackActionLike, 1.8, true
	case videoImageFeedbackActionNeutral, "ok", "normal":
		return videoImageFeedbackActionNeutral, 0.4, true
	case videoImageFeedbackActionDislike, "bad":
		return videoImageFeedbackActionDislike, -0.8, true
	case videoImageFeedbackActionTopPick, "best", "top":
		return videoImageFeedbackActionTopPick, 3.5, true
	default:
		return "", 0, false
	}
}

func mapVideoImageActionToLegacySignal(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case videoImageFeedbackActionDownload:
		return videoJobFeedbackSignalDownload
	case videoImageFeedbackActionFavorite, videoImageFeedbackActionLike, videoImageFeedbackActionTopPick:
		return videoJobFeedbackSignalFavorite
	default:
		return ""
	}
}

func (h *Handler) legacyFeedbackFallbackEnabled() bool {
	if h == nil {
		return false
	}
	return h.cfg.EnableLegacyFeedbackFallback
}

func (h *Handler) bumpVideoJobFeedbackByEmojiID(emojiID uint64, signal string, userID uint64) {
	if h == nil || h.db == nil || emojiID == 0 || !h.legacyFeedbackFallbackEnabled() {
		return
	}
	var emoji models.Emoji
	if err := h.db.Select("id", "collection_id").Where("id = ?", emojiID).First(&emoji).Error; err != nil {
		return
	}
	h.bumpVideoJobFeedbackByCollectionID(emoji.CollectionID, signal, userID)
}

func (h *Handler) bumpVideoJobFeedbackByCollectionID(collectionID uint64, signal string, userID uint64) {
	if h == nil || h.db == nil || collectionID == 0 || !h.legacyFeedbackFallbackEnabled() {
		return
	}
	signal = normalizeVideoJobFeedbackSignal(signal)

	tx := h.db.Begin()
	if tx == nil {
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var job models.VideoJob
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "metrics").
		Where("result_collection_id = ? AND status = ?", collectionID, models.VideoJobStatusDone).
		Order("id DESC").
		First(&job).Error
	if err != nil {
		return
	}

	metrics := parseJSONMap(job.Metrics)
	feedback := asMap(metrics["feedback_v1"])
	if len(feedback) == 0 {
		feedback = map[string]interface{}{}
	}
	feedback["version"] = "v1"
	feedback["total_signals"] = asInt64(feedback["total_signals"]) + 1
	feedback["download_count"] = asInt64(feedback["download_count"])
	feedback["favorite_count"] = asInt64(feedback["favorite_count"])
	switch signal {
	case videoJobFeedbackSignalFavorite:
		feedback["favorite_count"] = asInt64(feedback["favorite_count"]) + 1
	default:
		feedback["download_count"] = asInt64(feedback["download_count"]) + 1
	}
	feedback["engagement_score"] = asInt64(feedback["download_count"]) + asInt64(feedback["favorite_count"])*3
	feedback["last_signal"] = signal
	feedback["last_signal_at"] = time.Now().Format(time.RFC3339)
	if userID > 0 {
		feedback["last_user_id"] = userID
	}

	sceneTags := asStringSlice(metrics["scene_tags_v1"])
	if len(sceneTags) > 0 {
		sceneSignalCounts := asMap(feedback["scene_signal_counts"])
		for _, tag := range sceneTags {
			tag = strings.TrimSpace(strings.ToLower(tag))
			if tag == "" {
				continue
			}
			sceneSignalCounts[tag] = asInt64(sceneSignalCounts[tag]) + 1
		}
		feedback["scene_signal_counts"] = sceneSignalCounts
		feedback["scene_tags"] = sceneTags
	}
	metrics["feedback_v1"] = feedback

	if err := tx.Model(&models.VideoJob{}).
		Where("id = ?", job.ID).
		Updates(map[string]interface{}{
			"metrics":    toJSON(metrics),
			"updated_at": time.Now(),
		}).Error; err != nil {
		return
	}
	if err := tx.Create(&models.VideoJobEvent{
		JobID:   job.ID,
		Stage:   models.VideoJobStageIndexing,
		Level:   "info",
		Message: "feedback signal recorded",
		Metadata: toJSON(map[string]interface{}{
			"signal":      signal,
			"collection":  collectionID,
			"user_id":     userID,
			"total_count": feedback["total_signals"],
		}),
	}).Error; err != nil {
		return
	}
	_ = tx.Commit().Error
}

func asMap(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case map[string]interface{}:
		return value
	default:
		return map[string]interface{}{}
	}
}

func asInt64(raw interface{}) int64 {
	switch value := raw.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func asStringSlice(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if ok {
		out := make([]string, 0, len(items))
		for _, item := range items {
			if value, ok := item.(string); ok {
				value = strings.TrimSpace(strings.ToLower(value))
				if value == "" {
					continue
				}
				out = append(out, value)
			}
		}
		return out
	}

	values, ok := raw.([]string)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func (h *Handler) recordVideoImageFeedbackByEmojiID(
	emojiID uint64,
	action string,
	userID uint64,
	metadata map[string]interface{},
) {
	if h == nil || h.db == nil || emojiID == 0 || userID == 0 {
		return
	}
	action, weight := normalizeVideoImageFeedbackAction(action)

	var emoji models.Emoji
	if err := h.db.
		Select("id", "collection_id", "file_url", "format").
		Where("id = ?", emojiID).
		First(&emoji).Error; err != nil {
		return
	}

	var job models.VideoJob
	if err := h.db.
		Select("id", "metrics").
		Where("result_collection_id = ? AND status = ?", emoji.CollectionID, models.VideoJobStatusDone).
		Order("id DESC").
		First(&job).Error; err != nil {
		return
	}

	var output models.VideoImageOutputPublic
	outputID := uint64(0)
	objectKey := strings.TrimSpace(emoji.FileURL)
	if objectKey != "" {
		if err := h.db.
			Select("id", "metadata", "object_key").
			Where("job_id = ? AND object_key = ?", job.ID, objectKey).
			First(&output).Error; err == nil {
			outputID = output.ID
		}
	}
	if outputID == 0 {
		return
	}

	sceneTag := resolveVideoImageFeedbackSceneTag(job.Metrics, output.Metadata)

	meta := map[string]interface{}{
		"source":        "emoji_action",
		"emoji_id":      emoji.ID,
		"emoji_format":  strings.TrimSpace(strings.ToLower(emoji.Format)),
		"collection_id": emoji.CollectionID,
		"object_key":    objectKey,
	}
	for key, value := range metadata {
		meta[key] = value
	}

	meta["output_id"] = outputID

	entry := models.VideoImageFeedbackPublic{
		JobID:     job.ID,
		UserID:    userID,
		Action:    action,
		Weight:    weight,
		SceneTag:  sceneTag,
		Metadata:  toJSON(meta),
		CreatedAt: time.Now(),
	}
	id := outputID
	entry.OutputID = &id
	_ = videojobs.CreatePublicVideoImageFeedback(h.db, entry)
}

func resolveVideoImageFeedbackSceneTag(jobMetrics, outputMetadata datatypes.JSON) string {
	outputMap := parseJSONMap(outputMetadata)
	if direct := strings.TrimSpace(strings.ToLower(feedbackStringFromAny(outputMap["scene_tag"]))); direct != "" {
		return direct
	}

	jobMap := parseJSONMap(jobMetrics)
	sceneTags := asStringSlice(jobMap["scene_tags_v1"])
	if len(sceneTags) == 0 {
		return ""
	}
	return sceneTags[0]
}

func feedbackStringFromAny(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func feedbackFloatFromAny(raw interface{}) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int32:
		return float64(value)
	case int64:
		return float64(value)
	case uint:
		return float64(value)
	case uint32:
		return float64(value)
	case uint64:
		return float64(value)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func (h *Handler) resolveFeedbackOutputByJob(jobID uint64, outputID uint64) (*models.VideoImageOutputPublic, error) {
	if h == nil || h.db == nil || jobID == 0 || outputID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var output models.VideoImageOutputPublic
	if err := h.db.
		Select("id", "job_id", "object_key", "metadata", "file_role").
		Where("id = ? AND job_id = ?", outputID, jobID).
		First(&output).Error; err != nil {
		return nil, err
	}
	return &output, nil
}

func (h *Handler) resolveFeedbackEmojiInCollection(
	emojiID uint64,
	collectionID uint64,
) (*models.Emoji, error) {
	if h == nil || h.db == nil || emojiID == 0 || collectionID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var emoji models.Emoji
	if err := h.db.
		Select("id", "collection_id", "file_url", "thumb_url", "format", "title").
		Where("id = ? AND collection_id = ?", emojiID, collectionID).
		First(&emoji).Error; err == nil {
		return &emoji, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var videoEmoji models.VideoAssetEmoji
	if err := h.db.
		Select("id", "collection_id", "file_url", "thumb_url", "format", "title").
		Where("id = ? AND collection_id = ?", emojiID, collectionID).
		First(&videoEmoji).Error; err != nil {
		return nil, err
	}
	converted := convertVideoAssetEmoji(videoEmoji)
	return &converted, nil
}

func (h *Handler) resolveFeedbackOutputByJobAndObjectKey(
	jobID uint64,
	objectKey string,
) (*models.VideoImageOutputPublic, error) {
	if h == nil || h.db == nil || jobID == 0 || strings.TrimSpace(objectKey) == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var output models.VideoImageOutputPublic
	if err := h.db.
		Select("id", "job_id", "object_key", "metadata", "file_role").
		Where("job_id = ? AND object_key = ?", jobID, strings.TrimSpace(objectKey)).
		First(&output).Error; err != nil {
		return nil, err
	}
	return &output, nil
}

func isNotFoundError(err error) bool {
	return err != nil && errors.Is(err, gorm.ErrRecordNotFound)
}
