package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ipBindingStatusActive   = "active"
	ipBindingStatusInactive = "inactive"

	ipBindingAuditActionUpsert      = "admin_ip_binding_upsert"
	ipBindingAuditActionUpdate      = "admin_ip_binding_update"
	ipBindingAuditActionDelete      = "admin_ip_binding_delete"
	ipBindingAuditActionReorder     = "admin_ip_binding_reorder"
	ipBindingAuditActionBatchImport = "admin_ip_binding_batch_import"
)

type AdminIPBindingCollectionItem struct {
	ID         uint64     `json:"id"`
	Title      string     `json:"title"`
	CoverURL   string     `json:"cover_url,omitempty"`
	FileCount  int        `json:"file_count"`
	Source     string     `json:"source,omitempty"`
	Status     string     `json:"status,omitempty"`
	Visibility string     `json:"visibility,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

type AdminIPBindingItem struct {
	ID           uint64                       `json:"id"`
	IPID         uint64                       `json:"ip_id"`
	CollectionID uint64                       `json:"collection_id"`
	Sort         int                          `json:"sort"`
	Status       string                       `json:"status"`
	Note         string                       `json:"note,omitempty"`
	CreatedAt    time.Time                    `json:"created_at"`
	UpdatedAt    time.Time                    `json:"updated_at"`
	Collection   AdminIPBindingCollectionItem `json:"collection"`
}

type AdminIPBindingListResponse struct {
	Items    []AdminIPBindingItem `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

type AdminIPBindingUpsertRequest struct {
	CollectionID uint64 `json:"collection_id"`
	Sort         *int   `json:"sort"`
	Status       string `json:"status"`
	Note         string `json:"note"`
}

type AdminIPBindingUpdateRequest struct {
	Sort   *int    `json:"sort"`
	Status *string `json:"status"`
	Note   *string `json:"note"`
}

type AdminIPBindingReorderRequest struct {
	BindingIDs []uint64 `json:"binding_ids"`
}

type AdminIPBindingBatchImportRequest struct {
	CollectionIDs []uint64 `json:"collection_ids"`
	StartSort     *int     `json:"start_sort"`
	SortStep      *int     `json:"sort_step"`
	Note          string   `json:"note"`
	Replace       bool     `json:"replace"`
}

type AdminIPBindingBatchImportErrorItem struct {
	CollectionID uint64 `json:"collection_id"`
	Error        string `json:"error"`
}

type AdminIPBindingBatchImportResponse struct {
	Total        int                                  `json:"total"`
	SuccessCount int                                  `json:"success_count"`
	Failed       []AdminIPBindingBatchImportErrorItem `json:"failed"`
}

type AdminIPBindingMetricsResponse struct {
	TotalCollections   int64   `json:"total_collections"`
	BoundCollections   int64   `json:"bound_collections"`
	UnboundCollections int64   `json:"unbound_collections"`
	Coverage           float64 `json:"coverage"`
}

type AdminIPBindingAuditLogItem struct {
	ID              uint64    `json:"id"`
	AdminID         uint64    `json:"admin_id"`
	AdminName       string    `json:"admin_name"`
	Action          string    `json:"action"`
	IPID            uint64    `json:"ip_id"`
	BindingID       uint64    `json:"binding_id"`
	CollectionID    uint64    `json:"collection_id"`
	CollectionTitle string    `json:"collection_title,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type AdminIPBindingAuditLogListResponse struct {
	Total int64                        `json:"total"`
	Items []AdminIPBindingAuditLogItem `json:"items"`
}

func normalizeIPBindingStatus(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case ipBindingStatusActive, ipBindingStatusInactive:
		return value, true
	default:
		return "", false
	}
}

func (h *Handler) ipBindingTableReady(tx *gorm.DB) bool {
	if tx == nil {
		tx = h.db
	}
	if tx == nil {
		return false
	}
	return tx.Migrator().HasTable(&models.IPCollectionBinding{})
}

func (h *Handler) reconcileCollectionIPIDFromBindings(tx *gorm.DB, collectionID uint64) error {
	if tx == nil || collectionID == 0 || !h.ipBindingTableReady(tx) {
		return nil
	}

	var active models.IPCollectionBinding
	err := tx.Model(&models.IPCollectionBinding{}).
		Where("collection_id = ? AND status = ?", collectionID, ipBindingStatusActive).
		Order("sort ASC, id ASC").
		First(&active).Error

	updates := map[string]interface{}{}
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		updates["ip_id"] = nil
	} else {
		updates["ip_id"] = active.IPID
	}
	return tx.Model(&models.Collection{}).
		Where("id = ?", collectionID).
		Updates(updates).Error
}

func (h *Handler) syncCollectionBindingsToSingleIP(tx *gorm.DB, collectionIDs []uint64, ipID *uint64) error {
	if tx == nil || !h.ipBindingTableReady(tx) || len(collectionIDs) == 0 {
		return nil
	}

	uniq := make([]uint64, 0, len(collectionIDs))
	seen := map[uint64]struct{}{}
	for _, id := range collectionIDs {
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
		return nil
	}

	if ipID == nil || *ipID == 0 {
		if err := tx.Where("collection_id IN ?", uniq).Delete(&models.IPCollectionBinding{}).Error; err != nil {
			return err
		}
		return tx.Model(&models.Collection{}).Where("id IN ?", uniq).Update("ip_id", nil).Error
	}

	targetIP := *ipID
	if err := tx.Where("collection_id IN ? AND ip_id <> ?", uniq, targetIP).Delete(&models.IPCollectionBinding{}).Error; err != nil {
		return err
	}

	var currentMax int64
	if err := tx.Model(&models.IPCollectionBinding{}).
		Where("ip_id = ?", targetIP).
		Select("COALESCE(MAX(sort), 0)").
		Scan(&currentMax).Error; err != nil {
		return err
	}

	for idx, collectionID := range uniq {
		row := models.IPCollectionBinding{
			IPID:         targetIP,
			CollectionID: collectionID,
			Sort:         int(currentMax) + (idx+1)*10,
			Status:       ipBindingStatusActive,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "ip_id"}, {Name: "collection_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"status": ipBindingStatusActive, "updated_at": gorm.Expr("NOW()")}),
		}).Create(&row).Error; err != nil {
			return err
		}
	}

	for _, collectionID := range uniq {
		if err := h.reconcileCollectionIPIDFromBindings(tx, collectionID); err != nil {
			return err
		}
	}
	return nil
}

func buildAdminIPBindingItem(row models.IPCollectionBinding, collectionMap map[uint64]models.Collection, qiniuURLResolver func(string) string) AdminIPBindingItem {
	item := AdminIPBindingItem{
		ID:           row.ID,
		IPID:         row.IPID,
		CollectionID: row.CollectionID,
		Sort:         row.Sort,
		Status:       row.Status,
		Note:         strings.TrimSpace(row.Note),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
		Collection: AdminIPBindingCollectionItem{
			ID: row.CollectionID,
		},
	}
	if collection, ok := collectionMap[row.CollectionID]; ok {
		cover := strings.TrimSpace(collection.CoverURL)
		if qiniuURLResolver != nil {
			cover = qiniuURLResolver(cover)
		}
		updatedAt := collection.UpdatedAt
		item.Collection = AdminIPBindingCollectionItem{
			ID:         collection.ID,
			Title:      strings.TrimSpace(collection.Title),
			CoverURL:   cover,
			FileCount:  collection.FileCount,
			Source:     strings.TrimSpace(collection.Source),
			Status:     strings.TrimSpace(collection.Status),
			Visibility: strings.TrimSpace(collection.Visibility),
			UpdatedAt:  &updatedAt,
		}
	}
	return item
}

func (h *Handler) ensureIPExistsForBinding(tx *gorm.DB, ipID uint64) error {
	if tx == nil {
		tx = h.db
	}
	if ipID == 0 {
		return gorm.ErrRecordNotFound
	}
	var ip models.IP
	return tx.Select("id").First(&ip, ipID).Error
}

func buildIPBindingAuditSummary(action string, meta map[string]interface{}) string {
	switch strings.TrimSpace(action) {
	case ipBindingAuditActionBatchImport:
		success := parseUint64FromAny(meta["success_count"])
		total := parseUint64FromAny(meta["total"])
		if total == 0 {
			total = success
		}
		return "批量导入：" + strconv.FormatUint(success, 10) + "/" + strconv.FormatUint(total, 10)
	case ipBindingAuditActionReorder:
		updated := parseUint64FromAny(meta["updated_count"])
		return "重排条数：" + strconv.FormatUint(updated, 10)
	case ipBindingAuditActionDelete:
		return "移除绑定"
	case ipBindingAuditActionUpdate:
		return "更新绑定"
	case ipBindingAuditActionUpsert:
		return "新增/覆盖绑定"
	default:
		return ""
	}
}

// ListAdminIPBindings lists bindings for one IP.
// @Summary List IP bindings (admin)
// @Tags admin
// @Produce json
// @Param id path int true "ip id"
// @Param page query int false "page"
// @Param page_size query int false "page size"
// @Param status query string false "active|inactive|all"
// @Param q query string false "search collection title / id"
// @Success 200 {object} AdminIPBindingListResponse
// @Router /api/admin/ips/{id}/bindings [get]
func (h *Handler) ListAdminIPBindings(c *gin.Context) {
	ipID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || ipID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.ensureIPExistsForBinding(h.db, ipID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ip not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load ip"})
		return
	}

	page := parseIntQuery(c, "page", 1)
	pageSize := clampPageSize(parseIntQuery(c, "page_size", 20), 20, 100)
	if page <= 0 {
		page = 1
	}
	status := strings.TrimSpace(c.Query("status"))
	search := strings.TrimSpace(c.Query("q"))
	statusFilter := ""
	if status != "" && strings.ToLower(status) != "all" {
		normalized, ok := normalizeIPBindingStatus(status)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		statusFilter = normalized
	}

	if !h.ipBindingTableReady(h.db) {
		c.JSON(http.StatusOK, AdminIPBindingListResponse{
			Items:    []AdminIPBindingItem{},
			Total:    0,
			Page:     page,
			PageSize: pageSize,
		})
		return
	}

	query := h.db.Model(&models.IPCollectionBinding{}).Where("ip_id = ?", ipID)
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}
	if search != "" {
		like := "%" + search + "%"
		query = query.Joins("JOIN archive.collections c ON c.id = taxonomy.ip_collection_bindings.collection_id")
		if idValue, parseErr := strconv.ParseUint(search, 10, 64); parseErr == nil && idValue > 0 {
			query = query.Where("c.title ILIKE ? OR c.id = ?", like, idValue)
		} else {
			query = query.Where("c.title ILIKE ?", like)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list bindings"})
		return
	}

	var rows []models.IPCollectionBinding
	offset := (page - 1) * pageSize
	if err := query.Order("sort ASC, id ASC").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list bindings"})
		return
	}

	collectionIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		collectionIDs = append(collectionIDs, row.CollectionID)
	}
	collectionMap := map[uint64]models.Collection{}
	if len(collectionIDs) > 0 {
		var collections []models.Collection
		if err := h.db.Where("id IN ?", collectionIDs).Find(&collections).Error; err == nil {
			for _, collection := range collections {
				collectionMap[collection.ID] = collection
			}
		}
	}

	items := make([]AdminIPBindingItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, buildAdminIPBindingItem(row, collectionMap, func(raw string) string {
			return resolveListPreviewURL(raw, h.qiniu)
		}))
	}
	c.JSON(http.StatusOK, AdminIPBindingListResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// UpsertAdminIPBinding creates or updates one binding for an IP.
// @Summary Upsert IP binding (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "ip id"
// @Param body body AdminIPBindingUpsertRequest true "binding request"
// @Success 200 {object} AdminIPBindingItem
// @Router /api/admin/ips/{id}/bindings [post]
func (h *Handler) UpsertAdminIPBinding(c *gin.Context) {
	ipID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || ipID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if !h.ipBindingTableReady(h.db) {
		c.JSON(http.StatusConflict, gin.H{"error": "ip bindings table not ready"})
		return
	}

	var req AdminIPBindingUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.CollectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_id required"})
		return
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = ipBindingStatusActive
	}
	normalizedStatus, ok := normalizeIPBindingStatus(status)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	if err := h.ensureIPExistsForBinding(tx, ipID); err != nil {
		_ = tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ip not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load ip"})
		return
	}

	var collection models.Collection
	if err := tx.Select("id", "title").First(&collection, req.CollectionID).Error; err != nil {
		_ = tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "collection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collection"})
		return
	}

	sortValue := 0
	if req.Sort != nil {
		sortValue = *req.Sort
	}

	var row models.IPCollectionBinding
	var beforeRow models.IPCollectionBinding
	findErr := tx.Where("ip_id = ? AND collection_id = ?", ipID, req.CollectionID).First(&beforeRow).Error
	if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load binding"})
		return
	}
	existed := !errors.Is(findErr, gorm.ErrRecordNotFound)

	if normalizedStatus == ipBindingStatusActive {
		targetIP := ipID
		if err := h.syncCollectionBindingsToSingleIP(tx, []uint64{req.CollectionID}, &targetIP); err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync ip binding"})
			return
		}
		if err := tx.Where("ip_id = ? AND collection_id = ?", ipID, req.CollectionID).First(&row).Error; err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh binding"})
			return
		}
		updates := map[string]interface{}{
			"status": ipBindingStatusActive,
			"note":   strings.TrimSpace(req.Note),
		}
		if req.Sort != nil {
			updates["sort"] = sortValue
		}
		if err := tx.Model(&models.IPCollectionBinding{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update binding"})
			return
		}
		if err := tx.First(&row, row.ID).Error; err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh binding"})
			return
		}
	} else {
		if existed {
			row = beforeRow
			updates := map[string]interface{}{
				"status": ipBindingStatusInactive,
				"note":   strings.TrimSpace(req.Note),
			}
			if req.Sort != nil {
				updates["sort"] = sortValue
			}
			if err := tx.Model(&models.IPCollectionBinding{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				_ = tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update binding"})
				return
			}
			if err := tx.First(&row, row.ID).Error; err != nil {
				_ = tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh binding"})
				return
			}
		} else {
			if req.Sort == nil {
				var maxSort int64
				if err := tx.Model(&models.IPCollectionBinding{}).
					Where("ip_id = ?", ipID).
					Select("COALESCE(MAX(sort), 0)").
					Scan(&maxSort).Error; err == nil {
					sortValue = int(maxSort) + 10
				}
			}
			row = models.IPCollectionBinding{
				IPID:         ipID,
				CollectionID: req.CollectionID,
				Sort:         sortValue,
				Status:       ipBindingStatusInactive,
				Note:         strings.TrimSpace(req.Note),
			}
			if err := tx.Create(&row).Error; err != nil {
				_ = tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		if err := h.reconcileCollectionIPIDFromBindings(tx, row.CollectionID); err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync collection ip"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save binding"})
		return
	}

	adminID, _ := currentUserIDFromContext(c)
	h.recordAuditLog(adminID, "ip", ipID, ipBindingAuditActionUpsert, map[string]interface{}{
		"ip_id":              ipID,
		"binding_id":         row.ID,
		"collection_id":      row.CollectionID,
		"collection_title":   strings.TrimSpace(collection.Title),
		"before_binding_id":  beforeRow.ID,
		"before_sort":        beforeRow.Sort,
		"before_status":      strings.TrimSpace(beforeRow.Status),
		"before_note":        strings.TrimSpace(beforeRow.Note),
		"after_sort":         row.Sort,
		"after_status":       strings.TrimSpace(row.Status),
		"after_note":         strings.TrimSpace(row.Note),
		"before_exist":       existed,
		"requested_status":   normalizedStatus,
		"requested_sort_nil": req.Sort == nil,
	})

	var fullCollection models.Collection
	_ = h.db.First(&fullCollection, row.CollectionID).Error
	item := buildAdminIPBindingItem(row, map[uint64]models.Collection{fullCollection.ID: fullCollection}, func(raw string) string {
		return resolveListPreviewURL(raw, h.qiniu)
	})
	c.JSON(http.StatusOK, item)
}

// UpdateAdminIPBinding updates one binding row for an IP.
// @Summary Update IP binding (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "ip id"
// @Param binding_id path int true "binding id"
// @Param body body AdminIPBindingUpdateRequest true "binding update"
// @Success 200 {object} AdminIPBindingItem
// @Router /api/admin/ips/{id}/bindings/{binding_id} [put]
func (h *Handler) UpdateAdminIPBinding(c *gin.Context) {
	ipID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || ipID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	bindingID, err := strconv.ParseUint(c.Param("binding_id"), 10, 64)
	if err != nil || bindingID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid binding_id"})
		return
	}
	if !h.ipBindingTableReady(h.db) {
		c.JSON(http.StatusConflict, gin.H{"error": "ip bindings table not ready"})
		return
	}

	var req AdminIPBindingUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	var row models.IPCollectionBinding
	if err := tx.Where("id = ? AND ip_id = ?", bindingID, ipID).First(&row).Error; err != nil {
		_ = tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "binding not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load binding"})
		return
	}
	beforeRow := row

	updates := map[string]interface{}{}
	targetStatus := strings.TrimSpace(row.Status)
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if req.Status != nil {
		status, ok := normalizeIPBindingStatus(*req.Status)
		if !ok {
			_ = tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		targetStatus = status
		updates["status"] = status
	}
	if req.Note != nil {
		updates["note"] = strings.TrimSpace(*req.Note)
	}
	if targetStatus == ipBindingStatusActive {
		targetIP := ipID
		if err := h.syncCollectionBindingsToSingleIP(tx, []uint64{row.CollectionID}, &targetIP); err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync ip binding"})
			return
		}
	}
	if len(updates) > 0 {
		if err := tx.Model(&models.IPCollectionBinding{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update binding"})
			return
		}
	}

	if err := h.reconcileCollectionIPIDFromBindings(tx, row.CollectionID); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync collection ip"})
		return
	}

	if err := tx.First(&row, row.ID).Error; err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh binding"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save binding"})
		return
	}

	adminID, _ := currentUserIDFromContext(c)
	h.recordAuditLog(adminID, "ip", ipID, ipBindingAuditActionUpdate, map[string]interface{}{
		"ip_id":            ipID,
		"binding_id":       row.ID,
		"collection_id":    row.CollectionID,
		"before_sort":      beforeRow.Sort,
		"before_status":    strings.TrimSpace(beforeRow.Status),
		"before_note":      strings.TrimSpace(beforeRow.Note),
		"after_sort":       row.Sort,
		"after_status":     strings.TrimSpace(row.Status),
		"after_note":       strings.TrimSpace(row.Note),
		"requested_status": targetStatus,
	})

	var collection models.Collection
	_ = h.db.First(&collection, row.CollectionID).Error
	item := buildAdminIPBindingItem(row, map[uint64]models.Collection{collection.ID: collection}, func(raw string) string {
		return resolveListPreviewURL(raw, h.qiniu)
	})
	c.JSON(http.StatusOK, item)
}

// DeleteAdminIPBinding deletes one binding row for an IP.
// @Summary Delete IP binding (admin)
// @Tags admin
// @Produce json
// @Param id path int true "ip id"
// @Param binding_id path int true "binding id"
// @Success 200 {object} MessageResponse
// @Router /api/admin/ips/{id}/bindings/{binding_id} [delete]
func (h *Handler) DeleteAdminIPBinding(c *gin.Context) {
	ipID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || ipID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	bindingID, err := strconv.ParseUint(c.Param("binding_id"), 10, 64)
	if err != nil || bindingID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid binding_id"})
		return
	}
	if !h.ipBindingTableReady(h.db) {
		c.JSON(http.StatusConflict, gin.H{"error": "ip bindings table not ready"})
		return
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	var row models.IPCollectionBinding
	if err := tx.Where("id = ? AND ip_id = ?", bindingID, ipID).First(&row).Error; err != nil {
		_ = tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "binding not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load binding"})
		return
	}
	collectionTitle := ""
	_ = tx.Model(&models.Collection{}).Select("title").Where("id = ?", row.CollectionID).Scan(&collectionTitle).Error
	if err := tx.Delete(&models.IPCollectionBinding{}, row.ID).Error; err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete binding"})
		return
	}

	if err := h.reconcileCollectionIPIDFromBindings(tx, row.CollectionID); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync collection ip"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete binding"})
		return
	}

	adminID, _ := currentUserIDFromContext(c)
	h.recordAuditLog(adminID, "ip", ipID, ipBindingAuditActionDelete, map[string]interface{}{
		"ip_id":            ipID,
		"binding_id":       row.ID,
		"collection_id":    row.CollectionID,
		"collection_title": strings.TrimSpace(collectionTitle),
		"before_sort":      row.Sort,
		"before_status":    strings.TrimSpace(row.Status),
		"before_note":      strings.TrimSpace(row.Note),
	})
	c.JSON(http.StatusOK, MessageResponse{Message: "deleted"})
}

// ReorderAdminIPBindings updates sort order for bindings of one IP.
// @Summary Reorder IP bindings (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "ip id"
// @Param body body AdminIPBindingReorderRequest true "reorder request"
// @Success 200 {object} map[string]interface{}
// @Router /api/admin/ips/{id}/bindings/reorder [post]
func (h *Handler) ReorderAdminIPBindings(c *gin.Context) {
	ipID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || ipID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if !h.ipBindingTableReady(h.db) {
		c.JSON(http.StatusConflict, gin.H{"error": "ip bindings table not ready"})
		return
	}

	var req AdminIPBindingReorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.BindingIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "binding_ids required"})
		return
	}
	if len(req.BindingIDs) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "binding_ids too many"})
		return
	}

	uniqIDs := make([]uint64, 0, len(req.BindingIDs))
	seen := map[uint64]struct{}{}
	for _, id := range req.BindingIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqIDs = append(uniqIDs, id)
	}
	if len(uniqIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "binding_ids required"})
		return
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	var rows []models.IPCollectionBinding
	if err := tx.Where("ip_id = ? AND id IN ?", ipID, uniqIDs).Find(&rows).Error; err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load bindings"})
		return
	}
	if len(rows) != len(uniqIDs) {
		_ = tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "some bindings not found"})
		return
	}

	affectedCollectionSet := map[uint64]struct{}{}
	for idx, bindingID := range uniqIDs {
		sortValue := (idx + 1) * 10
		if err := tx.Model(&models.IPCollectionBinding{}).
			Where("id = ? AND ip_id = ?", bindingID, ipID).
			Update("sort", sortValue).Error; err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reorder bindings"})
			return
		}
	}
	for _, row := range rows {
		affectedCollectionSet[row.CollectionID] = struct{}{}
	}
	for collectionID := range affectedCollectionSet {
		if err := h.reconcileCollectionIPIDFromBindings(tx, collectionID); err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync collection ip"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reorder bindings"})
		return
	}
	adminID, _ := currentUserIDFromContext(c)
	h.recordAuditLog(adminID, "ip", ipID, ipBindingAuditActionReorder, map[string]interface{}{
		"ip_id":         ipID,
		"updated_count": len(uniqIDs),
		"binding_ids":   uniqIDs,
	})
	c.JSON(http.StatusOK, gin.H{
		"updated_count": len(uniqIDs),
	})
}

// BatchImportAdminIPBindings imports collection IDs to one IP in batch.
// @Summary Batch import IP bindings (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "ip id"
// @Param body body AdminIPBindingBatchImportRequest true "batch import request"
// @Success 200 {object} AdminIPBindingBatchImportResponse
// @Router /api/admin/ips/{id}/bindings/batch-import [post]
func (h *Handler) BatchImportAdminIPBindings(c *gin.Context) {
	ipID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || ipID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if !h.ipBindingTableReady(h.db) {
		c.JSON(http.StatusConflict, gin.H{"error": "ip bindings table not ready"})
		return
	}

	var req AdminIPBindingBatchImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.CollectionIDs) == 0 && !req.Replace {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_ids required"})
		return
	}
	if len(req.CollectionIDs) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_ids too many"})
		return
	}

	uniqIDs := make([]uint64, 0, len(req.CollectionIDs))
	seen := map[uint64]struct{}{}
	for _, id := range req.CollectionIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqIDs = append(uniqIDs, id)
	}

	resp := AdminIPBindingBatchImportResponse{
		Total:  len(uniqIDs),
		Failed: []AdminIPBindingBatchImportErrorItem{},
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	if err := h.ensureIPExistsForBinding(tx, ipID); err != nil {
		_ = tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ip not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load ip"})
		return
	}

	if req.Replace {
		var oldRows []models.IPCollectionBinding
		if err := tx.Where("ip_id = ?", ipID).Find(&oldRows).Error; err == nil && len(oldRows) > 0 {
			if err := tx.Where("ip_id = ?", ipID).Delete(&models.IPCollectionBinding{}).Error; err != nil {
				_ = tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear previous bindings"})
				return
			}
			reconciled := map[uint64]struct{}{}
			for _, row := range oldRows {
				if _, ok := reconciled[row.CollectionID]; ok {
					continue
				}
				reconciled[row.CollectionID] = struct{}{}
				if err := h.reconcileCollectionIPIDFromBindings(tx, row.CollectionID); err != nil {
					_ = tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reconcile previous bindings"})
					return
				}
			}
		}
	}

	if len(uniqIDs) > 0 {
		type collectionRow struct {
			ID    uint64 `gorm:"column:id"`
			Title string `gorm:"column:title"`
		}
		var collections []collectionRow
		if err := tx.Model(&models.Collection{}).Select("id, title").Where("id IN ?", uniqIDs).Find(&collections).Error; err != nil {
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load collections"})
			return
		}
		existMap := map[uint64]collectionRow{}
		for _, row := range collections {
			existMap[row.ID] = row
		}

		validIDs := make([]uint64, 0, len(uniqIDs))
		for _, id := range uniqIDs {
			if _, ok := existMap[id]; !ok {
				resp.Failed = append(resp.Failed, AdminIPBindingBatchImportErrorItem{
					CollectionID: id,
					Error:        "collection not found",
				})
				continue
			}
			validIDs = append(validIDs, id)
		}
		resp.SuccessCount = len(validIDs)

		if len(validIDs) > 0 {
			targetIP := ipID
			if err := h.syncCollectionBindingsToSingleIP(tx, validIDs, &targetIP); err != nil {
				_ = tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to import bindings"})
				return
			}

			startSort := 10
			if req.StartSort != nil {
				startSort = *req.StartSort
			}
			sortStep := 10
			if req.SortStep != nil && *req.SortStep > 0 {
				sortStep = *req.SortStep
			}
			note := strings.TrimSpace(req.Note)

			for idx, collectionID := range validIDs {
				updates := map[string]interface{}{
					"status": ipBindingStatusActive,
					"sort":   startSort + idx*sortStep,
				}
				if note != "" {
					updates["note"] = note
				}
				if err := tx.Model(&models.IPCollectionBinding{}).
					Where("ip_id = ? AND collection_id = ?", ipID, collectionID).
					Updates(updates).Error; err != nil {
					_ = tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update imported bindings"})
					return
				}
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to import bindings"})
		return
	}

	adminID, _ := currentUserIDFromContext(c)
	h.recordAuditLog(adminID, "ip", ipID, ipBindingAuditActionBatchImport, map[string]interface{}{
		"ip_id":         ipID,
		"total":         resp.Total,
		"success_count": resp.SuccessCount,
		"failed_count":  len(resp.Failed),
		"replace":       req.Replace,
		"start_sort":    req.StartSort,
		"sort_step":     req.SortStep,
	})
	c.JSON(http.StatusOK, resp)
}

// GetAdminIPBindingMetrics returns binding health metrics.
// @Summary Get IP binding metrics (admin)
// @Tags admin
// @Produce json
// @Success 200 {object} AdminIPBindingMetricsResponse
// @Router /api/admin/ips/binding-metrics [get]
func (h *Handler) GetAdminIPBindingMetrics(c *gin.Context) {
	var totalCollections int64
	if err := h.db.Model(&models.Collection{}).
		Where("archive.collections.deleted_at IS NULL").
		Count(&totalCollections).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load metrics"})
		return
	}

	var boundCollections int64
	if h.ipBindingTableReady(h.db) {
		if err := h.db.Table("taxonomy.ip_collection_bindings AS b").
			Select("COUNT(DISTINCT b.collection_id)").
			Joins("JOIN archive.collections c ON c.id = b.collection_id").
			Where("b.status = ?", ipBindingStatusActive).
			Where("c.deleted_at IS NULL").
			Scan(&boundCollections).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load metrics"})
			return
		}
		var fallbackBound int64
		if err := h.db.Table("archive.collections AS c").
			Select("COUNT(*)").
			Where("c.deleted_at IS NULL").
			Where("c.ip_id IS NOT NULL").
			Where("NOT EXISTS (SELECT 1 FROM taxonomy.ip_collection_bindings bx WHERE bx.collection_id = c.id)").
			Scan(&fallbackBound).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load metrics"})
			return
		}
		boundCollections += fallbackBound
	} else {
		if err := h.db.Model(&models.Collection{}).
			Where("archive.collections.deleted_at IS NULL").
			Where("archive.collections.ip_id IS NOT NULL").
			Count(&boundCollections).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load metrics"})
			return
		}
	}

	if boundCollections > totalCollections {
		boundCollections = totalCollections
	}
	unboundCollections := totalCollections - boundCollections
	coverage := 0.0
	if totalCollections > 0 {
		coverage = float64(boundCollections) / float64(totalCollections)
	}
	c.JSON(http.StatusOK, AdminIPBindingMetricsResponse{
		TotalCollections:   totalCollections,
		BoundCollections:   boundCollections,
		UnboundCollections: unboundCollections,
		Coverage:           coverage,
	})
}

// ListAdminIPBindingAuditLogs lists audit logs of IP binding operations.
// @Summary List IP binding audit logs (admin)
// @Tags admin
// @Produce json
// @Param id path int true "ip id"
// @Param limit query int false "limit, default 50, max 200"
// @Success 200 {object} AdminIPBindingAuditLogListResponse
// @Router /api/admin/ips/{id}/bindings/logs [get]
func (h *Handler) ListAdminIPBindingAuditLogs(c *gin.Context) {
	ipID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || ipID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.ensureIPExistsForBinding(h.db, ipID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ip not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load ip"})
		return
	}

	limit := parseIntQuery(c, "limit", 50)
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	actions := []string{
		ipBindingAuditActionUpsert,
		ipBindingAuditActionUpdate,
		ipBindingAuditActionDelete,
		ipBindingAuditActionReorder,
		ipBindingAuditActionBatchImport,
	}
	query := h.db.Model(&models.AuditLog{}).
		Where("target_type = ? AND target_id = ?", "ip", ipID).
		Where("action IN ?", actions)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load logs"})
		return
	}

	var logs []models.AuditLog
	if err := query.Order("id DESC").Limit(limit).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load logs"})
		return
	}

	adminIDs := make([]uint64, 0, len(logs))
	adminIDSet := map[uint64]struct{}{}
	for _, row := range logs {
		if row.AdminID == 0 {
			continue
		}
		if _, ok := adminIDSet[row.AdminID]; ok {
			continue
		}
		adminIDSet[row.AdminID] = struct{}{}
		adminIDs = append(adminIDs, row.AdminID)
	}
	adminNameMap := map[uint64]string{}
	if len(adminIDs) > 0 {
		type userRow struct {
			ID          uint64 `gorm:"column:id"`
			DisplayName string `gorm:"column:display_name"`
		}
		var users []userRow
		_ = h.db.Table("user.users").Select("id, display_name").Where("id IN ?", adminIDs).Scan(&users).Error
		for _, user := range users {
			adminNameMap[user.ID] = strings.TrimSpace(user.DisplayName)
		}
	}

	items := make([]AdminIPBindingAuditLogItem, 0, len(logs))
	for _, row := range logs {
		meta := map[string]interface{}{}
		if len(row.Meta) > 0 {
			_ = json.Unmarshal(row.Meta, &meta)
		}
		collectionID := parseUint64FromAny(meta["collection_id"])
		bindingID := parseUint64FromAny(meta["binding_id"])
		collectionTitle := ""
		if raw, ok := meta["collection_title"].(string); ok {
			collectionTitle = strings.TrimSpace(raw)
		}

		item := AdminIPBindingAuditLogItem{
			ID:              row.ID,
			AdminID:         row.AdminID,
			AdminName:       adminNameMap[row.AdminID],
			Action:          strings.TrimSpace(row.Action),
			IPID:            ipID,
			BindingID:       bindingID,
			CollectionID:    collectionID,
			CollectionTitle: collectionTitle,
			Summary:         buildIPBindingAuditSummary(row.Action, meta),
			CreatedAt:       row.CreatedAt,
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, AdminIPBindingAuditLogListResponse{
		Total: total,
		Items: items,
	})
}
