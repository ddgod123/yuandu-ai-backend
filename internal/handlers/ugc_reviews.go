package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	ugcReviewStatusDraft     = "draft"
	ugcReviewStatusReviewing = "reviewing"
	ugcReviewStatusApproved  = "approved"
	ugcReviewStatusRejected  = "rejected"

	ugcPublishStatusOffline = "offline"
	ugcPublishStatusOnline  = "online"

	ugcReviewActionSubmitReview = "submit_review"
	ugcReviewActionApprove      = "approve"
	ugcReviewActionReject       = "reject"
	ugcReviewActionPublish      = "publish"
	ugcReviewActionUnpublish    = "unpublish"
	ugcReviewActionAdminOffline = "admin_offline"
	ugcReviewActionContentEdit  = "content_changed"
)

var errUGCCollectionUnderReview = errors.New("collection_under_review")

type UGCReviewStatePayload struct {
	ReviewStatus         string     `json:"review_status"`
	PublishStatus        string     `json:"publish_status"`
	SubmitCount          int        `json:"submit_count"`
	LastSubmittedAt      *time.Time `json:"last_submitted_at,omitempty"`
	LastReviewedAt       *time.Time `json:"last_reviewed_at,omitempty"`
	LastApprovedAt       *time.Time `json:"last_approved_at,omitempty"`
	LastContentChangedAt time.Time  `json:"last_content_changed_at"`
	RejectReason         string     `json:"reject_reason,omitempty"`
	OfflineReason        string     `json:"offline_reason,omitempty"`
}

type UGCReviewActions struct {
	CanSubmitReview bool `json:"can_submit_review"`
	CanPublish      bool `json:"can_publish"`
	CanUnpublish    bool `json:"can_unpublish"`
}

type UGCReviewCollectionItem struct {
	Collection CollectionListItem    `json:"collection"`
	Review     UGCReviewStatePayload `json:"review"`
	Actions    UGCReviewActions      `json:"actions"`
}

type UGCReviewCollectionListResponse struct {
	Items    []UGCReviewCollectionItem `json:"items"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

type UGCReviewStateUpdateResponse struct {
	CollectionID uint64                `json:"collection_id"`
	Review       UGCReviewStatePayload `json:"review"`
	Actions      UGCReviewActions      `json:"actions"`
}

type UGCReviewReasonRequest struct {
	Reason string `json:"reason"`
}

type AdminUGCReviewBatchRequest struct {
	CollectionIDs []uint64 `json:"collection_ids"`
	Reason        string   `json:"reason"`
}

type AdminUGCReviewBatchErrorItem struct {
	CollectionID uint64 `json:"collection_id"`
	Error        string `json:"error"`
}

type AdminUGCReviewBatchResponse struct {
	Action       string                         `json:"action"`
	Total        int                            `json:"total"`
	SuccessCount int                            `json:"success_count"`
	Failed       []AdminUGCReviewBatchErrorItem `json:"failed"`
}

type UGCReviewLogItem struct {
	ID                uint64    `json:"id"`
	CollectionID      uint64    `json:"collection_id"`
	Action            string    `json:"action"`
	FromReviewStatus  string    `json:"from_review_status,omitempty"`
	ToReviewStatus    string    `json:"to_review_status,omitempty"`
	FromPublishStatus string    `json:"from_publish_status,omitempty"`
	ToPublishStatus   string    `json:"to_publish_status,omitempty"`
	OperatorRole      string    `json:"operator_role"`
	OperatorID        uint64    `json:"operator_id"`
	OperatorName      string    `json:"operator_name,omitempty"`
	Reason            string    `json:"reason,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type UGCReviewLogListResponse struct {
	Items    []UGCReviewLogItem `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

type AdminUGCReviewItem struct {
	CollectionID uint64                `json:"collection_id"`
	OwnerID      uint64                `json:"owner_id"`
	OwnerName    string                `json:"owner_name,omitempty"`
	Collection   CollectionListItem    `json:"collection"`
	Review       UGCReviewStatePayload `json:"review"`
	Actions      UGCReviewActions      `json:"actions"`
}

type AdminUGCReviewListResponse struct {
	Items    []AdminUGCReviewItem `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

func normalizeUGCReviewStatus(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case ugcReviewStatusDraft, ugcReviewStatusReviewing, ugcReviewStatusApproved, ugcReviewStatusRejected:
		return value, true
	default:
		return "", false
	}
}

func normalizeUGCPublishStatus(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case ugcPublishStatusOffline, ugcPublishStatusOnline:
		return value, true
	default:
		return "", false
	}
}

func inferUGCReviewState(collection models.Collection) (string, string) {
	status := strings.ToLower(strings.TrimSpace(collection.Status))
	visibility := strings.ToLower(strings.TrimSpace(collection.Visibility))
	if status == "active" && visibility == "public" {
		return ugcReviewStatusApproved, ugcPublishStatusOnline
	}
	if status == "pending" {
		return ugcReviewStatusReviewing, ugcPublishStatusOffline
	}
	return ugcReviewStatusDraft, ugcPublishStatusOffline
}

func normalizeUGCReviewStateRow(row models.UGCCollectionReviewState, collection models.Collection) models.UGCCollectionReviewState {
	reviewStatus, ok := normalizeUGCReviewStatus(row.ReviewStatus)
	if !ok {
		reviewStatus = ugcReviewStatusDraft
	}
	publishStatus, ok := normalizeUGCPublishStatus(row.PublishStatus)
	if !ok {
		publishStatus = ugcPublishStatusOffline
	}
	row.ReviewStatus = reviewStatus
	row.PublishStatus = publishStatus
	if row.CollectionID == 0 {
		row.CollectionID = collection.ID
	}
	if row.OwnerID == 0 {
		row.OwnerID = collection.OwnerID
	}
	if row.LastContentChangedAt.IsZero() {
		if collection.UpdatedAt.IsZero() {
			row.LastContentChangedAt = time.Now()
		} else {
			row.LastContentChangedAt = collection.UpdatedAt
		}
	}
	return row
}

func (h *Handler) getOrCreateUGCReviewState(db *gorm.DB, collection models.Collection) (models.UGCCollectionReviewState, error) {
	if db == nil {
		db = h.db
	}
	var row models.UGCCollectionReviewState
	if err := db.First(&row, "collection_id = ?", collection.ID).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return models.UGCCollectionReviewState{}, err
		}
		reviewStatus, publishStatus := inferUGCReviewState(collection)
		now := time.Now()
		row = models.UGCCollectionReviewState{
			CollectionID:         collection.ID,
			OwnerID:              collection.OwnerID,
			ReviewStatus:         reviewStatus,
			PublishStatus:        publishStatus,
			SubmitCount:          0,
			LastContentChangedAt: now,
		}
		if reviewStatus == ugcReviewStatusReviewing {
			row.SubmitCount = 1
			row.LastSubmittedAt = &now
		}
		if reviewStatus == ugcReviewStatusApproved {
			row.LastApprovedAt = &now
		}
		if createErr := db.Create(&row).Error; createErr != nil {
			return models.UGCCollectionReviewState{}, createErr
		}
		return normalizeUGCReviewStateRow(row, collection), nil
	}
	return normalizeUGCReviewStateRow(row, collection), nil
}

func (h *Handler) ensureUGCCollectionMutable(collection models.Collection) (models.UGCCollectionReviewState, error) {
	state, err := h.getOrCreateUGCReviewState(h.db, collection)
	if err != nil {
		return models.UGCCollectionReviewState{}, err
	}
	if state.ReviewStatus == ugcReviewStatusReviewing {
		return models.UGCCollectionReviewState{}, errUGCCollectionUnderReview
	}
	return state, nil
}

func isUGCCollectionUnderReviewError(err error) bool {
	return errors.Is(err, errUGCCollectionUnderReview)
}

func buildUGCReviewActions(state models.UGCCollectionReviewState, collection models.Collection) UGCReviewActions {
	collectionDisabled := strings.EqualFold(strings.TrimSpace(collection.Status), "disabled")
	canSubmit := !collectionDisabled && state.ReviewStatus != ugcReviewStatusReviewing && collection.FileCount > 0
	return UGCReviewActions{
		CanSubmitReview: canSubmit,
		CanPublish:      !collectionDisabled && state.ReviewStatus == ugcReviewStatusApproved && state.PublishStatus == ugcPublishStatusOffline,
		CanUnpublish:    !collectionDisabled && state.PublishStatus == ugcPublishStatusOnline,
	}
}

func toUGCReviewStatePayload(state models.UGCCollectionReviewState) UGCReviewStatePayload {
	return UGCReviewStatePayload{
		ReviewStatus:         state.ReviewStatus,
		PublishStatus:        state.PublishStatus,
		SubmitCount:          state.SubmitCount,
		LastSubmittedAt:      state.LastSubmittedAt,
		LastReviewedAt:       state.LastReviewedAt,
		LastApprovedAt:       state.LastApprovedAt,
		LastContentChangedAt: state.LastContentChangedAt,
		RejectReason:         strings.TrimSpace(state.RejectReason),
		OfflineReason:        strings.TrimSpace(state.OfflineReason),
	}
}

func buildUGCCollectionSnapshot(collection models.Collection) datatypes.JSON {
	payload := map[string]interface{}{
		"collection_id": collection.ID,
		"title":         collection.Title,
		"owner_id":      collection.OwnerID,
		"file_count":    collection.FileCount,
		"status":        collection.Status,
		"visibility":    collection.Visibility,
		"source":        collection.Source,
		"updated_at":    collection.UpdatedAt,
	}
	raw, _ := json.Marshal(payload)
	if len(raw) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(raw)
}

func (h *Handler) appendUGCReviewLog(db *gorm.DB, entry models.UGCCollectionReviewLog) error {
	if db == nil {
		db = h.db
	}
	if strings.TrimSpace(entry.OperatorRole) == "" {
		entry.OperatorRole = "system"
	}
	if len(entry.SnapshotJSON) == 0 {
		entry.SnapshotJSON = datatypes.JSON([]byte("{}"))
	}
	return db.Create(&entry).Error
}

func (h *Handler) touchUGCReviewAfterContentChange(collection models.Collection, operatorID uint64, reason string) error {
	state, err := h.getOrCreateUGCReviewState(h.db, collection)
	if err != nil {
		return err
	}
	if state.ReviewStatus == ugcReviewStatusReviewing {
		return errUGCCollectionUnderReview
	}

	now := time.Now()
	updates := map[string]interface{}{
		"last_content_changed_at": now,
	}
	nextReviewStatus := state.ReviewStatus
	nextPublishStatus := state.PublishStatus
	changedState := false

	if state.ReviewStatus == ugcReviewStatusApproved || state.ReviewStatus == ugcReviewStatusRejected {
		nextReviewStatus = ugcReviewStatusDraft
		updates["review_status"] = nextReviewStatus
		updates["reject_reason"] = ""
		updates["last_reviewed_at"] = nil
		updates["last_reviewer_id"] = nil
		changedState = true
	}
	if state.PublishStatus == ugcPublishStatusOnline {
		nextPublishStatus = ugcPublishStatusOffline
		updates["publish_status"] = nextPublishStatus
		if strings.TrimSpace(reason) == "" {
			updates["offline_reason"] = "内容已变更，自动下架，请重新提审"
		} else {
			updates["offline_reason"] = strings.TrimSpace(reason)
		}
		changedState = true
	}
	if err := h.db.Model(&models.UGCCollectionReviewState{}).
		Where("collection_id = ?", collection.ID).
		Updates(updates).Error; err != nil {
		return err
	}
	if changedState {
		_ = h.db.Model(&models.Collection{}).
			Where("id = ?", collection.ID).
			Updates(map[string]interface{}{
				"visibility": "private",
				"status":     "pending",
			}).Error
	}
	_ = h.appendUGCReviewLog(h.db, models.UGCCollectionReviewLog{
		CollectionID:      collection.ID,
		OwnerID:           collection.OwnerID,
		Action:            ugcReviewActionContentEdit,
		FromReviewStatus:  state.ReviewStatus,
		ToReviewStatus:    nextReviewStatus,
		FromPublishStatus: state.PublishStatus,
		ToPublishStatus:   nextPublishStatus,
		OperatorRole:      "user",
		OperatorID:        operatorID,
		Reason:            strings.TrimSpace(reason),
		SnapshotJSON:      buildUGCCollectionSnapshot(collection),
	})
	return nil
}

func (h *Handler) loadUGCReviewStateMap(items []models.Collection) map[uint64]models.UGCCollectionReviewState {
	result := map[uint64]models.UGCCollectionReviewState{}
	if len(items) == 0 {
		return result
	}
	ids := make([]uint64, 0, len(items))
	collectionMap := map[uint64]models.Collection{}
	for _, item := range items {
		ids = append(ids, item.ID)
		collectionMap[item.ID] = item
	}
	var rows []models.UGCCollectionReviewState
	if err := h.db.Where("collection_id IN ?", ids).Find(&rows).Error; err == nil {
		for _, row := range rows {
			collection := collectionMap[row.CollectionID]
			result[row.CollectionID] = normalizeUGCReviewStateRow(row, collection)
		}
	}
	for _, item := range items {
		if _, ok := result[item.ID]; ok {
			continue
		}
		reviewStatus, publishStatus := inferUGCReviewState(item)
		result[item.ID] = models.UGCCollectionReviewState{
			CollectionID:         item.ID,
			OwnerID:              item.OwnerID,
			ReviewStatus:         reviewStatus,
			PublishStatus:        publishStatus,
			LastContentChangedAt: item.UpdatedAt,
		}
	}
	return result
}

func parseReviewStateFilter(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || value == "all" {
		return "", true
	}
	return normalizeUGCReviewStatus(value)
}

func parsePublishStateFilter(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || value == "all" {
		return "", true
	}
	return normalizeUGCPublishStatus(value)
}

// ListMyUploadReviewCollections lists my UGC collections with review/publish state and available actions.
// @Summary List my upload review collections
// @Tags user
// @Produce json
// @Param page query int false "page"
// @Param page_size query int false "page size"
// @Param review_status query string false "draft/reviewing/approved/rejected/all"
// @Param publish_status query string false "offline/online/all"
// @Success 200 {object} UGCReviewCollectionListResponse
// @Router /api/me/uploads/review/collections [get]
func (h *Handler) ListMyUploadReviewCollections(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	page := parseIntQuery(c, "page", 1)
	pageSize := clampPageSize(parseIntQuery(c, "page_size", 12), 12, 60)
	if page <= 0 {
		page = 1
	}
	reviewStatus, ok := parseReviewStateFilter(c.Query("review_status"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review_status"})
		return
	}
	publishStatus, ok := parsePublishStateFilter(c.Query("publish_status"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid publish_status"})
		return
	}

	query := h.db.Model(&models.Collection{}).
		Joins("LEFT JOIN ops.ugc_collection_review_states urs ON urs.collection_id = archive.collections.id").
		Where("archive.collections.owner_id = ?", userID).
		Where("archive.collections.source = ?", ugcCollectionSource).
		Where("archive.collections.owner_deleted_at IS NULL")
	if reviewStatus != "" {
		query = query.Where("COALESCE(urs.review_status, ?) = ?", ugcReviewStatusDraft, reviewStatus)
	}
	if publishStatus != "" {
		query = query.Where("COALESCE(urs.publish_status, ?) = ?", ugcPublishStatusOffline, publishStatus)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list review collections"})
		return
	}

	var rows []models.Collection
	offset := (page - 1) * pageSize
	if err := query.Order("archive.collections.updated_at DESC, archive.collections.id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list review collections"})
		return
	}

	collectionItems := h.mapMyUploadCollections(rows, userID, 12)
	itemMap := map[uint64]CollectionListItem{}
	for _, item := range collectionItems {
		itemMap[item.ID] = item
	}
	stateMap := h.loadUGCReviewStateMap(rows)
	respItems := make([]UGCReviewCollectionItem, 0, len(rows))
	for _, row := range rows {
		state := stateMap[row.ID]
		item := itemMap[row.ID]
		effectiveCollection := row
		effectiveCollection.FileCount = int(item.FileCount)
		respItems = append(respItems, UGCReviewCollectionItem{
			Collection: item,
			Review:     toUGCReviewStatePayload(state),
			Actions:    buildUGCReviewActions(state, effectiveCollection),
		})
	}

	c.JSON(http.StatusOK, UGCReviewCollectionListResponse{
		Items:    respItems,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// GetMyUploadReviewCollectionDetail returns one of my UGC collections with review/publish state and available actions.
// @Summary Get my upload review collection detail
// @Tags user
// @Produce json
// @Param id path int true "collection id"
// @Success 200 {object} UGCReviewCollectionItem
// @Router /api/me/uploads/collections/{id}/review-detail [get]
func (h *Handler) GetMyUploadReviewCollectionDetail(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	collection, err := h.loadMyUGCCollectionByID(userID, collectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}
	state, err := h.getOrCreateUGCReviewState(h.db, collection)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review state"})
		return
	}
	collectionItems := h.mapMyUploadCollections([]models.Collection{collection}, userID, 12)
	collectionItem := CollectionListItem{}
	if len(collectionItems) > 0 {
		collectionItem = collectionItems[0]
	}
	effectiveCollection := collection
	effectiveCollection.FileCount = int(collectionItem.FileCount)
	c.JSON(http.StatusOK, UGCReviewCollectionItem{
		Collection: collectionItem,
		Review:     toUGCReviewStatePayload(state),
		Actions:    buildUGCReviewActions(state, effectiveCollection),
	})
}

// SubmitMyUploadCollectionReview submits one collection for admin review.
// @Summary Submit my upload collection for review
// @Tags user
// @Produce json
// @Param id path int true "collection id"
// @Success 200 {object} UGCReviewStateUpdateResponse
// @Router /api/me/uploads/collections/{id}/submit-review [post]
func (h *Handler) SubmitMyUploadCollectionReview(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	collection, err := h.loadMyUGCCollectionByID(userID, collectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}
	if strings.EqualFold(strings.TrimSpace(collection.Status), "disabled") {
		c.JSON(http.StatusForbidden, gin.H{"error": "collection is disabled"})
		return
	}

	var emojiCount int64
	if err := h.db.Model(&models.Emoji{}).
		Where("collection_id = ?", collection.ID).
		Count(&emojiCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check collection content"})
		return
	}
	if emojiCount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection is empty"})
		return
	}
	if collection.FileCount != int(emojiCount) {
		collection.FileCount = int(emojiCount)
		_ = h.db.Model(&models.Collection{}).Where("id = ?", collection.ID).Update("file_count", collection.FileCount).Error
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	state, err := h.getOrCreateUGCReviewState(tx, collection)
	if err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review state"})
		return
	}
	if state.ReviewStatus == ugcReviewStatusReviewing {
		_ = tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"error": "collection already under review"})
		return
	}

	now := time.Now()
	fromReview := state.ReviewStatus
	fromPublish := state.PublishStatus
	updates := map[string]interface{}{
		"review_status":           ugcReviewStatusReviewing,
		"publish_status":          ugcPublishStatusOffline,
		"submit_count":            state.SubmitCount + 1,
		"last_submitted_at":       now,
		"offline_reason":          "",
		"reject_reason":           "",
		"last_content_changed_at": now,
	}
	if err := tx.Model(&models.UGCCollectionReviewState{}).
		Where("collection_id = ?", collection.ID).
		Updates(updates).Error; err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to submit review"})
		return
	}
	if err := tx.Model(&models.Collection{}).Where("id = ?", collection.ID).Updates(map[string]interface{}{
		"visibility": "private",
		"status":     "pending",
		"file_count": emojiCount,
	}).Error; err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update collection status"})
		return
	}

	if err := h.appendUGCReviewLog(tx, models.UGCCollectionReviewLog{
		CollectionID:      collection.ID,
		OwnerID:           collection.OwnerID,
		Action:            ugcReviewActionSubmitReview,
		FromReviewStatus:  fromReview,
		ToReviewStatus:    ugcReviewStatusReviewing,
		FromPublishStatus: fromPublish,
		ToPublishStatus:   ugcPublishStatusOffline,
		OperatorRole:      "user",
		OperatorID:        userID,
		SnapshotJSON:      buildUGCCollectionSnapshot(collection),
	}); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write review log"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to submit review"})
		return
	}
	h.recordAuditLog(userID, "collection", collection.ID, "user_submit_collection_review", map[string]interface{}{
		"source": ugcCollectionSource,
	})

	state, _ = h.getOrCreateUGCReviewState(h.db, collection)
	collection.Status = "pending"
	collection.Visibility = "private"
	collection.FileCount = int(emojiCount)
	c.JSON(http.StatusOK, UGCReviewStateUpdateResponse{
		CollectionID: collection.ID,
		Review:       toUGCReviewStatePayload(state),
		Actions:      buildUGCReviewActions(state, collection),
	})
}

// PublishMyUploadCollection publishes an approved collection to public website.
// @Summary Publish my approved upload collection
// @Tags user
// @Accept json
// @Produce json
// @Param id path int true "collection id"
// @Success 200 {object} UGCReviewStateUpdateResponse
// @Router /api/me/uploads/collections/{id}/publish [post]
func (h *Handler) PublishMyUploadCollection(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	collection, err := h.loadMyUGCCollectionByID(userID, collectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}
	if strings.EqualFold(strings.TrimSpace(collection.Status), "disabled") {
		c.JSON(http.StatusForbidden, gin.H{"error": "collection is disabled"})
		return
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	state, err := h.getOrCreateUGCReviewState(tx, collection)
	if err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review state"})
		return
	}
	if !(state.ReviewStatus == ugcReviewStatusApproved && state.PublishStatus == ugcPublishStatusOffline) {
		_ = tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection is not publishable"})
		return
	}

	if err := tx.Model(&models.UGCCollectionReviewState{}).
		Where("collection_id = ?", collection.ID).
		Updates(map[string]interface{}{
			"publish_status": ugcPublishStatusOnline,
			"offline_reason": "",
		}).Error; err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish collection"})
		return
	}
	if err := tx.Model(&models.Collection{}).Where("id = ?", collection.ID).Updates(map[string]interface{}{
		"status":     "active",
		"visibility": "public",
	}).Error; err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update collection visibility"})
		return
	}
	if err := h.appendUGCReviewLog(tx, models.UGCCollectionReviewLog{
		CollectionID:      collection.ID,
		OwnerID:           collection.OwnerID,
		Action:            ugcReviewActionPublish,
		FromReviewStatus:  state.ReviewStatus,
		ToReviewStatus:    state.ReviewStatus,
		FromPublishStatus: state.PublishStatus,
		ToPublishStatus:   ugcPublishStatusOnline,
		OperatorRole:      "user",
		OperatorID:        userID,
		SnapshotJSON:      buildUGCCollectionSnapshot(collection),
	}); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write review log"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish collection"})
		return
	}
	h.recordAuditLog(userID, "collection", collection.ID, "user_publish_collection", map[string]interface{}{
		"source": ugcCollectionSource,
	})

	state, _ = h.getOrCreateUGCReviewState(h.db, collection)
	collection.Visibility = "public"
	collection.Status = "active"
	c.JSON(http.StatusOK, UGCReviewStateUpdateResponse{
		CollectionID: collection.ID,
		Review:       toUGCReviewStatePayload(state),
		Actions:      buildUGCReviewActions(state, collection),
	})
}

// UnpublishMyUploadCollection unpublishes one online collection from public website.
// @Summary Unpublish my upload collection
// @Tags user
// @Accept json
// @Produce json
// @Param id path int true "collection id"
// @Param body body UGCReviewReasonRequest false "unpublish reason"
// @Success 200 {object} UGCReviewStateUpdateResponse
// @Router /api/me/uploads/collections/{id}/unpublish [post]
func (h *Handler) UnpublishMyUploadCollection(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	collection, err := h.loadMyUGCCollectionByID(userID, collectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}
	if strings.EqualFold(strings.TrimSpace(collection.Status), "disabled") {
		c.JSON(http.StatusForbidden, gin.H{"error": "collection is disabled"})
		return
	}

	var req UGCReviewReasonRequest
	_ = c.ShouldBindJSON(&req)
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "user_unpublish"
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	state, err := h.getOrCreateUGCReviewState(tx, collection)
	if err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review state"})
		return
	}
	if state.PublishStatus != ugcPublishStatusOnline {
		_ = tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection is not online"})
		return
	}

	if err := tx.Model(&models.UGCCollectionReviewState{}).Where("collection_id = ?", collection.ID).Updates(map[string]interface{}{
		"publish_status": ugcPublishStatusOffline,
		"offline_reason": reason,
	}).Error; err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unpublish collection"})
		return
	}
	if err := tx.Model(&models.Collection{}).Where("id = ?", collection.ID).Updates(map[string]interface{}{
		"visibility": "private",
	}).Error; err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update collection visibility"})
		return
	}
	if err := h.appendUGCReviewLog(tx, models.UGCCollectionReviewLog{
		CollectionID:      collection.ID,
		OwnerID:           collection.OwnerID,
		Action:            ugcReviewActionUnpublish,
		FromReviewStatus:  state.ReviewStatus,
		ToReviewStatus:    state.ReviewStatus,
		FromPublishStatus: state.PublishStatus,
		ToPublishStatus:   ugcPublishStatusOffline,
		OperatorRole:      "user",
		OperatorID:        userID,
		Reason:            reason,
		SnapshotJSON:      buildUGCCollectionSnapshot(collection),
	}); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write review log"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unpublish collection"})
		return
	}
	h.recordAuditLog(userID, "collection", collection.ID, "user_unpublish_collection", map[string]interface{}{
		"source": ugcCollectionSource,
		"reason": reason,
	})

	state, _ = h.getOrCreateUGCReviewState(h.db, collection)
	collection.Visibility = "private"
	c.JSON(http.StatusOK, UGCReviewStateUpdateResponse{
		CollectionID: collection.ID,
		Review:       toUGCReviewStatePayload(state),
		Actions:      buildUGCReviewActions(state, collection),
	})
}

// ListMyUploadCollectionReviewLogs lists review action logs for one collection.
// @Summary List my upload collection review logs
// @Tags user
// @Produce json
// @Param id path int true "collection id"
// @Param page query int false "page"
// @Param page_size query int false "page size"
// @Success 200 {object} UGCReviewLogListResponse
// @Router /api/me/uploads/collections/{id}/review-logs [get]
func (h *Handler) ListMyUploadCollectionReviewLogs(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if _, err := h.loadMyUGCCollectionByID(userID, collectionID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}

	page := parseIntQuery(c, "page", 1)
	pageSize := clampPageSize(parseIntQuery(c, "page_size", 20), 20, 100)
	if page <= 0 {
		page = 1
	}

	query := h.db.Model(&models.UGCCollectionReviewLog{}).Where("collection_id = ?", collectionID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list logs"})
		return
	}

	var logs []models.UGCCollectionReviewLog
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC, id DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list logs"})
		return
	}

	operatorIDs := make([]uint64, 0, len(logs))
	seen := map[uint64]struct{}{}
	for _, item := range logs {
		if item.OperatorID == 0 {
			continue
		}
		if _, ok := seen[item.OperatorID]; ok {
			continue
		}
		seen[item.OperatorID] = struct{}{}
		operatorIDs = append(operatorIDs, item.OperatorID)
	}
	operatorNameMap := map[uint64]string{}
	if len(operatorIDs) > 0 {
		type userRow struct {
			ID          uint64 `gorm:"column:id"`
			DisplayName string `gorm:"column:display_name"`
		}
		var users []userRow
		_ = h.db.Table("user.users").Select("id, display_name").Where("id IN ?", operatorIDs).Scan(&users).Error
		for _, user := range users {
			operatorNameMap[user.ID] = strings.TrimSpace(user.DisplayName)
		}
	}

	items := make([]UGCReviewLogItem, 0, len(logs))
	for _, item := range logs {
		items = append(items, UGCReviewLogItem{
			ID:                item.ID,
			CollectionID:      item.CollectionID,
			Action:            item.Action,
			FromReviewStatus:  item.FromReviewStatus,
			ToReviewStatus:    item.ToReviewStatus,
			FromPublishStatus: item.FromPublishStatus,
			ToPublishStatus:   item.ToPublishStatus,
			OperatorRole:      item.OperatorRole,
			OperatorID:        item.OperatorID,
			OperatorName:      operatorNameMap[item.OperatorID],
			Reason:            strings.TrimSpace(item.Reason),
			CreatedAt:         item.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, UGCReviewLogListResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// ListAdminUGCReviews lists UGC review queue for operations/admin.
// @Summary List UGC reviews (admin)
// @Tags admin
// @Produce json
// @Param page query int false "page"
// @Param page_size query int false "page size"
// @Param review_status query string false "draft/reviewing/approved/rejected/all"
// @Param publish_status query string false "offline/online/all"
// @Param q query string false "search title"
// @Success 200 {object} AdminUGCReviewListResponse
// @Router /api/admin/ugc/reviews [get]
func (h *Handler) ListAdminUGCReviews(c *gin.Context) {
	page := parseIntQuery(c, "page", 1)
	pageSize := clampPageSize(parseIntQuery(c, "page_size", 20), 20, 100)
	if page <= 0 {
		page = 1
	}
	reviewStatus, ok := parseReviewStateFilter(c.Query("review_status"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review_status"})
		return
	}
	publishStatus, ok := parsePublishStateFilter(c.Query("publish_status"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid publish_status"})
		return
	}
	search := strings.TrimSpace(c.Query("q"))

	query := h.db.Model(&models.UGCCollectionReviewState{}).
		Joins("JOIN archive.collections c ON c.id = ops.ugc_collection_review_states.collection_id").
		Where("c.source = ?", ugcCollectionSource)
	if reviewStatus != "" {
		query = query.Where("ops.ugc_collection_review_states.review_status = ?", reviewStatus)
	}
	if publishStatus != "" {
		query = query.Where("ops.ugc_collection_review_states.publish_status = ?", publishStatus)
	}
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("c.title ILIKE ? OR c.description ILIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list reviews"})
		return
	}

	var states []models.UGCCollectionReviewState
	offset := (page - 1) * pageSize
	if err := query.Order("ops.ugc_collection_review_states.updated_at DESC, ops.ugc_collection_review_states.collection_id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&states).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list reviews"})
		return
	}
	if len(states) == 0 {
		c.JSON(http.StatusOK, AdminUGCReviewListResponse{
			Items:    []AdminUGCReviewItem{},
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		})
		return
	}

	collectionIDs := make([]uint64, 0, len(states))
	ownerIDs := make([]uint64, 0, len(states))
	seenOwner := map[uint64]struct{}{}
	for _, item := range states {
		collectionIDs = append(collectionIDs, item.CollectionID)
		if _, ok := seenOwner[item.OwnerID]; ok {
			continue
		}
		seenOwner[item.OwnerID] = struct{}{}
		ownerIDs = append(ownerIDs, item.OwnerID)
	}

	var collections []models.Collection
	if err := h.db.Where("id IN ?", collectionIDs).Find(&collections).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review collections"})
		return
	}
	collectionMap := map[uint64]models.Collection{}
	for _, item := range collections {
		collectionMap[item.ID] = item
	}

	ownerNameMap := map[uint64]string{}
	if len(ownerIDs) > 0 {
		type userRow struct {
			ID          uint64 `gorm:"column:id"`
			DisplayName string `gorm:"column:display_name"`
		}
		var users []userRow
		_ = h.db.Table("user.users").Select("id, display_name").Where("id IN ?", ownerIDs).Scan(&users).Error
		for _, user := range users {
			ownerNameMap[user.ID] = strings.TrimSpace(user.DisplayName)
		}
	}

	sort.Slice(collections, func(i, j int) bool {
		return collections[i].ID < collections[j].ID
	})
	collectionItems := h.mapMyUploadCollections(collections, 0, 12)
	collectionItemMap := map[uint64]CollectionListItem{}
	for _, item := range collectionItems {
		collectionItemMap[item.ID] = item
	}

	items := make([]AdminUGCReviewItem, 0, len(states))
	for _, state := range states {
		collection, ok := collectionMap[state.CollectionID]
		if !ok {
			continue
		}
		collectionItem := collectionItemMap[state.CollectionID]
		effectiveCollection := collection
		effectiveCollection.FileCount = int(collectionItem.FileCount)
		normalizedState := normalizeUGCReviewStateRow(state, collection)
		items = append(items, AdminUGCReviewItem{
			CollectionID: state.CollectionID,
			OwnerID:      state.OwnerID,
			OwnerName:    ownerNameMap[state.OwnerID],
			Collection:   collectionItem,
			Review:       toUGCReviewStatePayload(normalizedState),
			Actions:      buildUGCReviewActions(normalizedState, effectiveCollection),
		})
	}

	c.JSON(http.StatusOK, AdminUGCReviewListResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// GetAdminUGCReviewDetail returns one review state for admin.
// @Summary Get UGC review detail (admin)
// @Tags admin
// @Produce json
// @Param id path int true "collection id"
// @Success 200 {object} AdminUGCReviewItem
// @Router /api/admin/ugc/reviews/{id} [get]
func (h *Handler) GetAdminUGCReviewDetail(c *gin.Context) {
	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var collection models.Collection
	if err := h.db.First(&collection, collectionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}
	if !isUGCCollection(collection) {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	state, err := h.getOrCreateUGCReviewState(h.db, collection)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review state"})
		return
	}

	ownerName := ""
	type userRow struct {
		ID          uint64 `gorm:"column:id"`
		DisplayName string `gorm:"column:display_name"`
	}
	var owner userRow
	_ = h.db.Table("user.users").Select("id, display_name").Where("id = ?", collection.OwnerID).First(&owner).Error
	ownerName = strings.TrimSpace(owner.DisplayName)

	collectionItems := h.mapMyUploadCollections([]models.Collection{collection}, 0, 12)
	collectionItem := CollectionListItem{}
	if len(collectionItems) > 0 {
		collectionItem = collectionItems[0]
	}
	effectiveCollection := collection
	effectiveCollection.FileCount = int(collectionItem.FileCount)
	item := AdminUGCReviewItem{
		CollectionID: collection.ID,
		OwnerID:      collection.OwnerID,
		OwnerName:    ownerName,
		Collection:   collectionItem,
		Review:       toUGCReviewStatePayload(state),
		Actions:      buildUGCReviewActions(state, effectiveCollection),
	}
	c.JSON(http.StatusOK, item)
}

type ugcReviewDecisionError struct {
	StatusCode int
	Message    string
}

func (e *ugcReviewDecisionError) Error() string {
	return e.Message
}

func (h *Handler) applyAdminUGCReviewDecisionByID(collectionID, adminID uint64, action, reason string) (models.Collection, models.UGCCollectionReviewState, error) {
	tx := h.db.Begin()
	if tx.Error != nil {
		return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusInternalServerError, Message: "db error"}
	}

	var collection models.Collection
	if err := tx.First(&collection, collectionID).Error; err != nil {
		_ = tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusNotFound, Message: "collection not found"}
		}
		return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusInternalServerError, Message: "failed to load collection"}
	}
	if !isUGCCollection(collection) {
		_ = tx.Rollback()
		return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusNotFound, Message: "collection not found"}
	}

	state, err := h.getOrCreateUGCReviewState(tx, collection)
	if err != nil {
		_ = tx.Rollback()
		return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusInternalServerError, Message: "failed to load review state"}
	}

	now := time.Now()
	fromReview := state.ReviewStatus
	fromPublish := state.PublishStatus
	nextReview := state.ReviewStatus
	nextPublish := state.PublishStatus
	stateUpdates := map[string]interface{}{}
	collectionUpdates := map[string]interface{}{}

	switch action {
	case ugcReviewActionApprove:
		if state.ReviewStatus != ugcReviewStatusReviewing {
			_ = tx.Rollback()
			return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusBadRequest, Message: "collection is not under review"}
		}
		nextReview = ugcReviewStatusApproved
		nextPublish = ugcPublishStatusOffline
		stateUpdates = map[string]interface{}{
			"review_status":    nextReview,
			"publish_status":   nextPublish,
			"last_reviewed_at": now,
			"last_approved_at": now,
			"last_reviewer_id": adminID,
			"reject_reason":    "",
			"offline_reason":   "",
		}
		collectionUpdates = map[string]interface{}{
			"status":     "active",
			"visibility": "private",
		}
	case ugcReviewActionReject:
		if state.ReviewStatus != ugcReviewStatusReviewing {
			_ = tx.Rollback()
			return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusBadRequest, Message: "collection is not under review"}
		}
		if reason == "" {
			_ = tx.Rollback()
			return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusBadRequest, Message: "reason required"}
		}
		nextReview = ugcReviewStatusRejected
		nextPublish = ugcPublishStatusOffline
		stateUpdates = map[string]interface{}{
			"review_status":    nextReview,
			"publish_status":   nextPublish,
			"last_reviewed_at": now,
			"last_reviewer_id": adminID,
			"reject_reason":    reason,
		}
		collectionUpdates = map[string]interface{}{
			"status":     "pending",
			"visibility": "private",
		}
	case ugcReviewActionAdminOffline:
		if state.PublishStatus != ugcPublishStatusOnline {
			_ = tx.Rollback()
			return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusBadRequest, Message: "collection is not online"}
		}
		if reason == "" {
			reason = "admin_offline"
		}
		nextPublish = ugcPublishStatusOffline
		stateUpdates = map[string]interface{}{
			"publish_status":   nextPublish,
			"offline_reason":   reason,
			"last_reviewed_at": now,
			"last_reviewer_id": adminID,
		}
		collectionUpdates = map[string]interface{}{
			"visibility": "private",
		}
	default:
		_ = tx.Rollback()
		return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusBadRequest, Message: "invalid action"}
	}

	if err := tx.Model(&models.UGCCollectionReviewState{}).
		Where("collection_id = ?", collection.ID).
		Updates(stateUpdates).Error; err != nil {
		_ = tx.Rollback()
		return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusInternalServerError, Message: "failed to update review state"}
	}
	if len(collectionUpdates) > 0 {
		if err := tx.Model(&models.Collection{}).Where("id = ?", collection.ID).Updates(collectionUpdates).Error; err != nil {
			_ = tx.Rollback()
			return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusInternalServerError, Message: "failed to update collection"}
		}
	}
	if err := h.appendUGCReviewLog(tx, models.UGCCollectionReviewLog{
		CollectionID:      collection.ID,
		OwnerID:           collection.OwnerID,
		Action:            action,
		FromReviewStatus:  fromReview,
		ToReviewStatus:    nextReview,
		FromPublishStatus: fromPublish,
		ToPublishStatus:   nextPublish,
		OperatorRole:      "admin",
		OperatorID:        adminID,
		Reason:            reason,
		SnapshotJSON:      buildUGCCollectionSnapshot(collection),
	}); err != nil {
		_ = tx.Rollback()
		return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusInternalServerError, Message: "failed to write review log"}
	}

	if err := tx.Commit().Error; err != nil {
		return models.Collection{}, models.UGCCollectionReviewState{}, &ugcReviewDecisionError{StatusCode: http.StatusInternalServerError, Message: "failed to apply review decision"}
	}

	h.recordAuditLog(adminID, "collection", collection.ID, "admin_ugc_"+action, map[string]interface{}{
		"source": ugcCollectionSource,
		"reason": reason,
	})

	state, _ = h.getOrCreateUGCReviewState(h.db, collection)
	if status, ok := collectionUpdates["status"].(string); ok {
		collection.Status = status
	}
	if visibility, ok := collectionUpdates["visibility"].(string); ok {
		collection.Visibility = visibility
	}
	return collection, state, nil
}

func (h *Handler) applyAdminUGCReviewDecision(c *gin.Context, action string) {
	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	adminID, _ := currentUserIDFromContext(c)

	var req UGCReviewReasonRequest
	_ = c.ShouldBindJSON(&req)
	reason := strings.TrimSpace(req.Reason)

	collection, state, decisionErr := h.applyAdminUGCReviewDecisionByID(collectionID, adminID, action, reason)
	if decisionErr != nil {
		if de, ok := decisionErr.(*ugcReviewDecisionError); ok {
			c.JSON(de.StatusCode, gin.H{"error": de.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to apply review decision"})
		return
	}

	c.JSON(http.StatusOK, UGCReviewStateUpdateResponse{
		CollectionID: collection.ID,
		Review:       toUGCReviewStatePayload(state),
		Actions:      buildUGCReviewActions(state, collection),
	})
}

// AdminApproveUGCReview approves one reviewing collection.
// @Summary Approve UGC review (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "collection id"
// @Param body body UGCReviewReasonRequest false "approve note"
// @Success 200 {object} UGCReviewStateUpdateResponse
// @Router /api/admin/ugc/reviews/{id}/approve [post]
func (h *Handler) AdminApproveUGCReview(c *gin.Context) {
	h.applyAdminUGCReviewDecision(c, ugcReviewActionApprove)
}

// AdminRejectUGCReview rejects one reviewing collection.
// @Summary Reject UGC review (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "collection id"
// @Param body body UGCReviewReasonRequest true "reject reason"
// @Success 200 {object} UGCReviewStateUpdateResponse
// @Router /api/admin/ugc/reviews/{id}/reject [post]
func (h *Handler) AdminRejectUGCReview(c *gin.Context) {
	h.applyAdminUGCReviewDecision(c, ugcReviewActionReject)
}

// AdminOfflineUGCReview force-offlines one online collection.
// @Summary Offline UGC collection (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "collection id"
// @Param body body UGCReviewReasonRequest false "offline reason"
// @Success 200 {object} UGCReviewStateUpdateResponse
// @Router /api/admin/ugc/reviews/{id}/offline [post]
func (h *Handler) AdminOfflineUGCReview(c *gin.Context) {
	h.applyAdminUGCReviewDecision(c, ugcReviewActionAdminOffline)
}

// ListAdminUGCReviewLogs lists review logs for one collection.
// @Summary List UGC review logs (admin)
// @Tags admin
// @Produce json
// @Param id path int true "collection id"
// @Param page query int false "page"
// @Param page_size query int false "page size"
// @Success 200 {object} UGCReviewLogListResponse
// @Router /api/admin/ugc/reviews/{id}/logs [get]
func (h *Handler) ListAdminUGCReviewLogs(c *gin.Context) {
	collectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || collectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var collection models.Collection
	if err := h.db.First(&collection, collectionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}
	if !isUGCCollection(collection) {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}

	page := parseIntQuery(c, "page", 1)
	pageSize := clampPageSize(parseIntQuery(c, "page_size", 20), 20, 100)
	if page <= 0 {
		page = 1
	}

	query := h.db.Model(&models.UGCCollectionReviewLog{}).Where("collection_id = ?", collectionID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list logs"})
		return
	}
	var logs []models.UGCCollectionReviewLog
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC, id DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list logs"})
		return
	}

	operatorIDs := make([]uint64, 0, len(logs))
	seen := map[uint64]struct{}{}
	for _, item := range logs {
		if item.OperatorID == 0 {
			continue
		}
		if _, ok := seen[item.OperatorID]; ok {
			continue
		}
		seen[item.OperatorID] = struct{}{}
		operatorIDs = append(operatorIDs, item.OperatorID)
	}
	operatorNameMap := map[uint64]string{}
	if len(operatorIDs) > 0 {
		type userRow struct {
			ID          uint64 `gorm:"column:id"`
			DisplayName string `gorm:"column:display_name"`
		}
		var users []userRow
		_ = h.db.Table("user.users").Select("id, display_name").Where("id IN ?", operatorIDs).Scan(&users).Error
		for _, user := range users {
			operatorNameMap[user.ID] = strings.TrimSpace(user.DisplayName)
		}
	}

	items := make([]UGCReviewLogItem, 0, len(logs))
	for _, item := range logs {
		items = append(items, UGCReviewLogItem{
			ID:                item.ID,
			CollectionID:      item.CollectionID,
			Action:            item.Action,
			FromReviewStatus:  item.FromReviewStatus,
			ToReviewStatus:    item.ToReviewStatus,
			FromPublishStatus: item.FromPublishStatus,
			ToPublishStatus:   item.ToPublishStatus,
			OperatorRole:      item.OperatorRole,
			OperatorID:        item.OperatorID,
			OperatorName:      operatorNameMap[item.OperatorID],
			Reason:            strings.TrimSpace(item.Reason),
			CreatedAt:         item.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, UGCReviewLogListResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

func (h *Handler) applyAdminUGCReviewBatch(c *gin.Context, action string) {
	adminID, _ := currentUserIDFromContext(c)
	var req AdminUGCReviewBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.CollectionIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_ids required"})
		return
	}
	if len(req.CollectionIDs) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_ids too many"})
		return
	}

	uniq := make([]uint64, 0, len(req.CollectionIDs))
	seen := map[uint64]struct{}{}
	for _, id := range req.CollectionIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_ids required"})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	resp := AdminUGCReviewBatchResponse{
		Action: action,
		Total:  len(uniq),
		Failed: []AdminUGCReviewBatchErrorItem{},
	}
	for _, id := range uniq {
		_, _, err := h.applyAdminUGCReviewDecisionByID(id, adminID, action, reason)
		if err != nil {
			resp.Failed = append(resp.Failed, AdminUGCReviewBatchErrorItem{
				CollectionID: id,
				Error:        err.Error(),
			})
			continue
		}
		resp.SuccessCount++
	}
	c.JSON(http.StatusOK, resp)
}

// AdminBatchApproveUGCReview approves multiple reviewing collections.
// @Summary Batch approve UGC reviews (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body AdminUGCReviewBatchRequest true "batch request"
// @Success 200 {object} AdminUGCReviewBatchResponse
// @Router /api/admin/ugc/reviews/batch/approve [post]
func (h *Handler) AdminBatchApproveUGCReview(c *gin.Context) {
	h.applyAdminUGCReviewBatch(c, ugcReviewActionApprove)
}

// AdminBatchRejectUGCReview rejects multiple reviewing collections.
// @Summary Batch reject UGC reviews (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body AdminUGCReviewBatchRequest true "batch request"
// @Success 200 {object} AdminUGCReviewBatchResponse
// @Router /api/admin/ugc/reviews/batch/reject [post]
func (h *Handler) AdminBatchRejectUGCReview(c *gin.Context) {
	h.applyAdminUGCReviewBatch(c, ugcReviewActionReject)
}

// AdminBatchOfflineUGCReview offlines multiple online collections.
// @Summary Batch offline UGC collections (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body AdminUGCReviewBatchRequest true "batch request"
// @Success 200 {object} AdminUGCReviewBatchResponse
// @Router /api/admin/ugc/reviews/batch/offline [post]
func (h *Handler) AdminBatchOfflineUGCReview(c *gin.Context) {
	h.applyAdminUGCReviewBatch(c, ugcReviewActionAdminOffline)
}
