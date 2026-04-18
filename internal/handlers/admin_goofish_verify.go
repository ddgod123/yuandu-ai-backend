package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type AdminGoofishVerifyStatusRequest struct {
	CollectionIDs []uint64 `json:"collection_ids"`
	Command       string   `json:"command"`
}

type AdminGoofishVerifyGoodsPreview struct {
	GoodsNo    string          `json:"goods_no"`
	GoodsType  int             `json:"goods_type"`
	GoodsName  string          `json:"goods_name"`
	Price      int64           `json:"price"`
	Stock      int             `json:"stock"`
	Status     int             `json:"status"`
	UpdateTime int64           `json:"update_time"`
	Template   json.RawMessage `json:"template,omitempty"`
}

type AdminGoofishVerifyStatusItem struct {
	CollectionID          uint64                          `json:"collection_id"`
	CollectionExists      bool                            `json:"collection_exists"`
	CollectionTitle       string                          `json:"collection_title,omitempty"`
	CollectionCoverURL    string                          `json:"collection_cover_url,omitempty"`
	CollectionVisibility  string                          `json:"collection_visibility,omitempty"`
	HasGoods              bool                            `json:"has_goods"`
	GoodsCount            int                             `json:"goods_count"`
	OnShelfGoodsCount     int                             `json:"on_shelf_goods_count"`
	OffShelfGoodsCount    int                             `json:"off_shelf_goods_count"`
	MixedGoodsStatus      bool                            `json:"mixed_goods_status"`
	GoodsStatus           int                             `json:"goods_status,omitempty"`
	GoodsStatusLabel      string                          `json:"goods_status_label,omitempty"`
	GoodsNo               string                          `json:"goods_no,omitempty"`
	GoodsType             int                             `json:"goods_type,omitempty"`
	GoodsName             string                          `json:"goods_name,omitempty"`
	Price                 int64                           `json:"price,omitempty"`
	Stock                 int                             `json:"stock,omitempty"`
	UpdateTime            int64                           `json:"update_time,omitempty"`
	ExpectedVisibility    string                          `json:"expected_visibility,omitempty"`
	VisibilityConsistent  bool                            `json:"visibility_consistent"`
	StatusInternallyValid bool                            `json:"status_internally_valid"`
	ListPreview           *AdminGoofishVerifyGoodsPreview `json:"list_preview,omitempty"`
	DetailPreview         *AdminGoofishVerifyGoodsPreview `json:"detail_preview,omitempty"`
	Note                  string                          `json:"note,omitempty"`
}

type AdminGoofishVerifyStatusSummary struct {
	TotalCollections        int `json:"total_collections"`
	FoundCollections        int `json:"found_collections"`
	MissingCollections      int `json:"missing_collections"`
	HasGoodsCollections     int `json:"has_goods_collections"`
	MissingGoodsCollections int `json:"missing_goods_collections"`
	VisibilityMismatchCount int `json:"visibility_mismatch_count"`
	MixedStatusCount        int `json:"mixed_status_count"`
	OnShelfGoodsCount       int `json:"on_shelf_goods_count"`
	OffShelfGoodsCount      int `json:"off_shelf_goods_count"`
}

type AdminGoofishVerifyStatusResponse struct {
	CollectionIDs []uint64                        `json:"collection_ids"`
	Summary       AdminGoofishVerifyStatusSummary `json:"summary"`
	Items         []AdminGoofishVerifyStatusItem  `json:"items"`
}

type AdminGoofishVerifyFixRequest struct {
	CollectionIDs       []uint64 `json:"collection_ids"`
	Command             string   `json:"command"`
	DryRun              *bool    `json:"dry_run"`
	FixVisibility       *bool    `json:"fix_visibility"`
	FixMixedGoodsStatus *bool    `json:"fix_mixed_goods_status"`
	MixedStatusStrategy string   `json:"mixed_status_strategy"`
}

type AdminGoofishVerifyFixItem struct {
	CollectionID            uint64 `json:"collection_id"`
	CollectionExists        bool   `json:"collection_exists"`
	HasGoods                bool   `json:"has_goods"`
	MixedGoodsStatus        bool   `json:"mixed_goods_status"`
	VisibilityMismatch      bool   `json:"visibility_mismatch"`
	CurrentVisibility       string `json:"current_visibility,omitempty"`
	ExpectedVisibility      string `json:"expected_visibility,omitempty"`
	TargetGoodsStatus       int    `json:"target_goods_status,omitempty"`
	PlannedGoodsUpdateRows  int    `json:"planned_goods_update_rows"`
	AppliedGoodsUpdateRows  int64  `json:"applied_goods_update_rows"`
	PlannedVisibilityUpdate bool   `json:"planned_visibility_update"`
	AppliedVisibilityUpdate bool   `json:"applied_visibility_update"`
	Note                    string `json:"note,omitempty"`
}

type AdminGoofishVerifyFixResponse struct {
	CollectionIDs             []uint64                    `json:"collection_ids"`
	DryRun                    bool                        `json:"dry_run"`
	FixVisibility             bool                        `json:"fix_visibility"`
	FixMixedGoodsStatus       bool                        `json:"fix_mixed_goods_status"`
	MixedStatusStrategy       string                      `json:"mixed_status_strategy"`
	ScannedCollectionCount    int                         `json:"scanned_collection_count"`
	FoundCollectionCount      int                         `json:"found_collection_count"`
	MissingCollectionCount    int                         `json:"missing_collection_count"`
	MismatchCollectionCount   int                         `json:"mismatch_collection_count"`
	MixedCollectionCount      int                         `json:"mixed_collection_count"`
	PlannedVisibilityUpdates  int                         `json:"planned_visibility_updates"`
	AppliedVisibilityUpdates  int64                       `json:"applied_visibility_updates"`
	PlannedGoodsStatusUpdates int                         `json:"planned_goods_status_updates"`
	AppliedGoodsStatusUpdates int64                       `json:"applied_goods_status_updates"`
	ChangedCollectionIDs      []uint64                    `json:"changed_collection_ids,omitempty"`
	Items                     []AdminGoofishVerifyFixItem `json:"items"`
	Message                   string                      `json:"message"`
}

func normalizeCollectionIDsInput(rawIDs []uint64) []uint64 {
	idSet := map[uint64]struct{}{}
	ids := make([]uint64, 0, len(rawIDs))
	for _, id := range rawIDs {
		if id == 0 {
			continue
		}
		if _, exists := idSet[id]; exists {
			continue
		}
		idSet[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func toGoofishGoodsPreview(row models.CollectionGood, includeTemplate bool) *AdminGoofishVerifyGoodsPreview {
	template := row.TemplateJSON
	if len(template) == 0 {
		template = datatypes.JSON([]byte("[]"))
	}
	item := &AdminGoofishVerifyGoodsPreview{
		GoodsNo:    strings.TrimSpace(row.GoodsNo),
		GoodsType:  row.GoodsType,
		GoodsName:  strings.TrimSpace(row.GoodsName),
		Price:      row.Price,
		Stock:      row.Stock,
		Status:     row.Status,
		UpdateTime: row.UpdatedAt.Unix(),
	}
	if includeTemplate {
		item.Template = json.RawMessage(template)
	}
	return item
}

func normalizeGoofishMixedStatusStrategy(raw string) (string, bool) {
	v := strings.TrimSpace(strings.ToLower(raw))
	switch v {
	case "", "prefer_on_shelf", "prefer_on", "on", "up", "在架优先":
		return "prefer_on_shelf", true
	case "prefer_off_shelf", "prefer_off", "off", "down", "下架优先":
		return "prefer_off_shelf", true
	case "follow_visibility", "visibility", "按可见性":
		return "follow_visibility", true
	default:
		return "", false
	}
}

func resolveGoofishMixedTargetStatus(strategy string, visibility string) int {
	switch strategy {
	case "prefer_off_shelf":
		return adminCollectionGoodStatusOffShelf
	case "follow_visibility":
		if strings.EqualFold(strings.TrimSpace(visibility), "public") {
			return adminCollectionGoodStatusOnShelf
		}
		return adminCollectionGoodStatusOffShelf
	default:
		return adminCollectionGoodStatusOnShelf
	}
}

// VerifyAdminGoofishStatusByCollections 联动校验商品库状态与合集可见性。
// @Summary Verify goofish goods status and collection visibility consistency (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body AdminGoofishVerifyStatusRequest true "verify request"
// @Success 200 {object} AdminGoofishVerifyStatusResponse
// @Router /api/admin/goofish/verify-status [post]
func (h *Handler) VerifyAdminGoofishStatusByCollections(c *gin.Context) {
	var req AdminGoofishVerifyStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ids := normalizeCollectionIDsInput(req.CollectionIDs)
	if len(ids) == 0 {
		parsed, err := parseCollectionIDsFromCommand(strings.TrimSpace(req.Command), adminCollectionGoodsMaxBatch)
		if err == nil {
			ids = parsed
		}
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_ids or command with ids is required"})
		return
	}
	if len(ids) > adminCollectionGoodsMaxBatch {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many collection_ids, max 500"})
		return
	}

	var collections []models.Collection
	if err := h.db.
		Select("id,title,cover_url,visibility,updated_at").
		Where("id IN ?", ids).
		Find(&collections).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query collections"})
		return
	}
	collectionMap := map[uint64]models.Collection{}
	for _, row := range collections {
		collectionMap[row.ID] = row
	}

	var goodsRows []models.CollectionGood
	if err := h.db.
		Select("id,collection_id,goods_no,goods_type,goods_name,price,stock,status,template_json,updated_at").
		Where("collection_id IN ?", ids).
		Order("updated_at DESC, id DESC").
		Find(&goodsRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query collection goods"})
		return
	}
	goodsMap := map[uint64][]models.CollectionGood{}
	for _, row := range goodsRows {
		goodsMap[row.CollectionID] = append(goodsMap[row.CollectionID], row)
	}

	items := make([]AdminGoofishVerifyStatusItem, 0, len(ids))
	summary := AdminGoofishVerifyStatusSummary{
		TotalCollections: len(ids),
	}

	for _, collectionID := range ids {
		collection, exists := collectionMap[collectionID]
		if !exists {
			summary.MissingCollections += 1
			items = append(items, AdminGoofishVerifyStatusItem{
				CollectionID:          collectionID,
				CollectionExists:      false,
				HasGoods:              false,
				StatusInternallyValid: false,
				Note:                  "合集不存在或已删除",
			})
			continue
		}
		summary.FoundCollections += 1

		item := AdminGoofishVerifyStatusItem{
			CollectionID:          collectionID,
			CollectionExists:      true,
			CollectionTitle:       strings.TrimSpace(collection.Title),
			CollectionCoverURL:    resolveListPreviewURL(collection.CoverURL, h.qiniu),
			CollectionVisibility:  strings.TrimSpace(collection.Visibility),
			StatusInternallyValid: true,
		}
		rows := goodsMap[collectionID]
		item.GoodsCount = len(rows)
		if len(rows) == 0 {
			item.HasGoods = false
			item.ExpectedVisibility = "private"
			item.VisibilityConsistent = strings.EqualFold(item.CollectionVisibility, "private")
			item.Note = "该合集尚无商品库记录"
			summary.MissingGoodsCollections += 1
			if !item.VisibilityConsistent {
				summary.VisibilityMismatchCount += 1
			}
			items = append(items, item)
			continue
		}

		item.HasGoods = true
		summary.HasGoodsCollections += 1
		for _, row := range rows {
			if row.Status == adminCollectionGoodStatusOnShelf {
				item.OnShelfGoodsCount += 1
				summary.OnShelfGoodsCount += 1
			} else if row.Status == adminCollectionGoodStatusOffShelf {
				item.OffShelfGoodsCount += 1
				summary.OffShelfGoodsCount += 1
			} else {
				item.StatusInternallyValid = false
			}
		}
		item.MixedGoodsStatus = item.OnShelfGoodsCount > 0 && item.OffShelfGoodsCount > 0
		if item.MixedGoodsStatus {
			summary.MixedStatusCount += 1
		}

		primary := rows[0]
		item.GoodsStatus = primary.Status
		item.GoodsStatusLabel = collectionGoodStatusLabel(primary.Status)
		item.GoodsNo = strings.TrimSpace(primary.GoodsNo)
		item.GoodsType = primary.GoodsType
		item.GoodsName = strings.TrimSpace(primary.GoodsName)
		item.Price = primary.Price
		item.Stock = primary.Stock
		item.UpdateTime = primary.UpdatedAt.Unix()
		item.ListPreview = toGoofishGoodsPreview(primary, false)
		item.DetailPreview = toGoofishGoodsPreview(primary, true)

		if item.OnShelfGoodsCount > 0 {
			item.ExpectedVisibility = "public"
		} else {
			item.ExpectedVisibility = "private"
		}
		item.VisibilityConsistent = strings.EqualFold(item.CollectionVisibility, item.ExpectedVisibility)
		if !item.VisibilityConsistent {
			summary.VisibilityMismatchCount += 1
		}
		if item.MixedGoodsStatus {
			item.Note = "同一合集存在在架/下架混合商品，建议统一状态"
		}
		items = append(items, item)
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].CollectionID < items[j].CollectionID })
	c.JSON(http.StatusOK, AdminGoofishVerifyStatusResponse{
		CollectionIDs: ids,
		Summary:       summary,
		Items:         items,
	})
}

// FixAdminGoofishStatusByCollections 一键修复校验发现的问题（可见性不一致/混合状态）。
// @Summary Fix goofish verify issues by collections (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body AdminGoofishVerifyFixRequest true "fix request"
// @Success 200 {object} AdminGoofishVerifyFixResponse
// @Router /api/admin/goofish/verify-fix [post]
func (h *Handler) FixAdminGoofishStatusByCollections(c *gin.Context) {
	var req AdminGoofishVerifyFixRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ids := normalizeCollectionIDsInput(req.CollectionIDs)
	if len(ids) == 0 {
		parsed, err := parseCollectionIDsFromCommand(strings.TrimSpace(req.Command), adminCollectionGoodsMaxBatch)
		if err == nil {
			ids = parsed
		}
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_ids or command with ids is required"})
		return
	}
	if len(ids) > adminCollectionGoodsMaxBatch {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many collection_ids, max 500"})
		return
	}

	strategy, ok := normalizeGoofishMixedStatusStrategy(req.MixedStatusStrategy)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mixed_status_strategy, expected prefer_on_shelf/prefer_off_shelf/follow_visibility"})
		return
	}
	dryRun := boolValueOrDefault(req.DryRun, false)
	fixVisibility := boolValueOrDefault(req.FixVisibility, true)
	fixMixedGoodsStatus := boolValueOrDefault(req.FixMixedGoodsStatus, true)

	var collections []models.Collection
	if err := h.db.
		Select("id,visibility").
		Where("id IN ?", ids).
		Find(&collections).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query collections"})
		return
	}
	collectionMap := map[uint64]models.Collection{}
	for _, row := range collections {
		collectionMap[row.ID] = row
	}

	var goodsRows []models.CollectionGood
	if err := h.db.
		Select("id,collection_id,status").
		Where("collection_id IN ?", ids).
		Find(&goodsRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query collection goods"})
		return
	}
	goodsMap := map[uint64][]models.CollectionGood{}
	for _, row := range goodsRows {
		goodsMap[row.CollectionID] = append(goodsMap[row.CollectionID], row)
	}

	items := make([]AdminGoofishVerifyFixItem, 0, len(ids))
	plannedVisibilityUpdates := 0
	plannedGoodsStatusUpdates := 0
	missingCollectionCount := 0
	foundCollectionCount := 0
	mismatchCollectionCount := 0
	mixedCollectionCount := 0

	for _, collectionID := range ids {
		collection, exists := collectionMap[collectionID]
		if !exists {
			missingCollectionCount += 1
			items = append(items, AdminGoofishVerifyFixItem{
				CollectionID:     collectionID,
				CollectionExists: false,
				HasGoods:         false,
				Note:             "合集不存在或已删除",
			})
			continue
		}
		foundCollectionCount += 1

		rows := goodsMap[collectionID]
		item := AdminGoofishVerifyFixItem{
			CollectionID:      collectionID,
			CollectionExists:  true,
			HasGoods:          len(rows) > 0,
			CurrentVisibility: strings.TrimSpace(collection.Visibility),
		}
		if len(rows) == 0 {
			item.ExpectedVisibility = "private"
			item.VisibilityMismatch = !strings.EqualFold(item.CurrentVisibility, "private")
			item.MixedGoodsStatus = false
			if item.VisibilityMismatch {
				mismatchCollectionCount += 1
			}
			if fixVisibility && item.VisibilityMismatch {
				item.PlannedVisibilityUpdate = true
				plannedVisibilityUpdates += 1
			}
			item.Note = "无商品记录，按规则 visibility 应为 private"
			items = append(items, item)
			continue
		}

		onShelf := 0
		offShelf := 0
		for _, row := range rows {
			if row.Status == adminCollectionGoodStatusOnShelf {
				onShelf += 1
			} else if row.Status == adminCollectionGoodStatusOffShelf {
				offShelf += 1
			}
		}
		item.MixedGoodsStatus = onShelf > 0 && offShelf > 0
		if item.MixedGoodsStatus {
			mixedCollectionCount += 1
			targetStatus := resolveGoofishMixedTargetStatus(strategy, item.CurrentVisibility)
			item.TargetGoodsStatus = targetStatus
			if fixMixedGoodsStatus {
				for _, row := range rows {
					if row.Status != targetStatus {
						item.PlannedGoodsUpdateRows += 1
					}
				}
				plannedGoodsStatusUpdates += item.PlannedGoodsUpdateRows
			}
		}

		finalHasOnShelf := onShelf > 0
		if item.MixedGoodsStatus && fixMixedGoodsStatus {
			finalHasOnShelf = item.TargetGoodsStatus == adminCollectionGoodStatusOnShelf
		}
		if finalHasOnShelf {
			item.ExpectedVisibility = "public"
		} else {
			item.ExpectedVisibility = "private"
		}
		item.VisibilityMismatch = !strings.EqualFold(item.CurrentVisibility, item.ExpectedVisibility)
		if item.VisibilityMismatch {
			mismatchCollectionCount += 1
		}
		if fixVisibility && item.VisibilityMismatch {
			item.PlannedVisibilityUpdate = true
			plannedVisibilityUpdates += 1
		}
		if item.MixedGoodsStatus && !fixMixedGoodsStatus {
			item.Note = "存在混合状态，但当前未启用 mixed 状态修复"
		}
		items = append(items, item)
	}

	changedSet := map[uint64]struct{}{}
	appliedVisibilityUpdates := int64(0)
	appliedGoodsStatusUpdates := int64(0)

	if !dryRun {
		tx := h.db.Begin()
		if tx.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start transaction"})
			return
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback().Error
			}
		}()

		now := time.Now()
		for idx := range items {
			item := &items[idx]
			if !item.CollectionExists {
				continue
			}
			if fixMixedGoodsStatus && item.MixedGoodsStatus && item.PlannedGoodsUpdateRows > 0 {
				result := tx.Model(&models.CollectionGood{}).
					Where("collection_id = ? AND status <> ?", item.CollectionID, item.TargetGoodsStatus).
					Updates(map[string]interface{}{
						"status":     item.TargetGoodsStatus,
						"updated_at": now,
					})
				if result.Error != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update mixed goods status"})
					return
				}
				item.AppliedGoodsUpdateRows = result.RowsAffected
				appliedGoodsStatusUpdates += result.RowsAffected
				if result.RowsAffected > 0 {
					changedSet[item.CollectionID] = struct{}{}
				}
			}
			if fixVisibility && item.PlannedVisibilityUpdate {
				result := tx.Model(&models.Collection{}).
					Where("id = ? AND visibility <> ?", item.CollectionID, item.ExpectedVisibility).
					Updates(map[string]interface{}{
						"visibility": item.ExpectedVisibility,
						"updated_at": now,
					})
				if result.Error != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update collection visibility"})
					return
				}
				item.AppliedVisibilityUpdate = result.RowsAffected > 0
				appliedVisibilityUpdates += result.RowsAffected
				if result.RowsAffected > 0 {
					changedSet[item.CollectionID] = struct{}{}
				}
			}
		}

		if err := tx.Commit().Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit transaction"})
			return
		}
		committed = true
	} else {
		for _, item := range items {
			if item.PlannedVisibilityUpdate || item.PlannedGoodsUpdateRows > 0 {
				changedSet[item.CollectionID] = struct{}{}
			}
		}
	}

	changedIDs := make([]uint64, 0, len(changedSet))
	for id := range changedSet {
		changedIDs = append(changedIDs, id)
	}
	sort.Slice(changedIDs, func(i, j int) bool { return changedIDs[i] < changedIDs[j] })

	message := "修复预览完成。"
	if !dryRun {
		message = "修复执行完成。"
	}
	message += " 可见性计划更新 " + strconv.Itoa(plannedVisibilityUpdates) + " 项，商品状态计划更新 " + strconv.Itoa(plannedGoodsStatusUpdates) + " 条。"

	c.JSON(http.StatusOK, AdminGoofishVerifyFixResponse{
		CollectionIDs:             ids,
		DryRun:                    dryRun,
		FixVisibility:             fixVisibility,
		FixMixedGoodsStatus:       fixMixedGoodsStatus,
		MixedStatusStrategy:       strategy,
		ScannedCollectionCount:    len(ids),
		FoundCollectionCount:      foundCollectionCount,
		MissingCollectionCount:    missingCollectionCount,
		MismatchCollectionCount:   mismatchCollectionCount,
		MixedCollectionCount:      mixedCollectionCount,
		PlannedVisibilityUpdates:  plannedVisibilityUpdates,
		AppliedVisibilityUpdates:  appliedVisibilityUpdates,
		PlannedGoodsStatusUpdates: plannedGoodsStatusUpdates,
		AppliedGoodsStatusUpdates: appliedGoodsStatusUpdates,
		ChangedCollectionIDs:      changedIDs,
		Items:                     items,
		Message:                   message,
	})
}
