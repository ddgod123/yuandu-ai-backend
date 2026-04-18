package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	adminCollectionGoodStatusOnShelf  = 1
	adminCollectionGoodStatusOffShelf = 2
	adminCollectionGoodsMaxBatch      = 500
)

var (
	collectionGoodsCommandRangeRE = regexp.MustCompile(`(\d+)\s*(?:-|~|—|–|至|到|to)\s*(\d+)`)
	collectionGoodsCommandDigitRE = regexp.MustCompile(`\d+`)
)

type AdminCollectionGoodsListResponse struct {
	Items []AdminCollectionGoodItem `json:"items"`
	Total int64                     `json:"total"`
}

type AdminCollectionGoodItem struct {
	ID                 uint64          `json:"id"`
	CollectionID       uint64          `json:"collection_id"`
	CollectionTitle    string          `json:"collection_title,omitempty"`
	CollectionCoverURL string          `json:"collection_cover_url,omitempty"`
	GoodsNo            string          `json:"goods_no"`
	GoodsType          int             `json:"goods_type"`
	GoodsName          string          `json:"goods_name"`
	Price              int64           `json:"price"`
	Stock              int             `json:"stock"`
	Status             int             `json:"status"`
	ImageCount         int             `json:"image_count"`
	ImageStart         int             `json:"image_start"`
	TemplateJSON       json.RawMessage `json:"template_json"`
	LastSyncAt         *time.Time      `json:"last_sync_at,omitempty"`
	LastSyncError      string          `json:"last_sync_error,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type AdminCollectionGoodUpsertRequest struct {
	CollectionID uint64          `json:"collection_id"`
	GoodsNo      string          `json:"goods_no"`
	GoodsType    int             `json:"goods_type"`
	GoodsName    string          `json:"goods_name"`
	Price        int64           `json:"price"`
	Stock        int             `json:"stock"`
	Status       int             `json:"status"`
	ImageCount   int             `json:"image_count"`
	ImageStart   int             `json:"image_start"`
	TemplateJSON json.RawMessage `json:"template_json"`
}

type AdminCollectionGoodBatchStatusRequest struct {
	CollectionGoodIDs []uint64 `json:"collection_good_ids"`
	Status            int      `json:"status"`
}

type AdminCollectionGoodBatchStatusResponse struct {
	UpdatedCount int64 `json:"updated_count"`
	Status       int   `json:"status"`
}

type AdminCollectionGoodsBootstrapRequest struct {
	CollectionIDs []uint64 `json:"collection_ids"`
	Command       string   `json:"command"`
	DefaultStatus *int     `json:"default_status"`
	DryRun        *bool    `json:"dry_run"`
}

type AdminCollectionGoodsBootstrapCreatedItem struct {
	ID           uint64 `json:"id"`
	CollectionID uint64 `json:"collection_id"`
	GoodsNo      string `json:"goods_no"`
	GoodsName    string `json:"goods_name"`
	Status       int    `json:"status"`
}

type AdminCollectionGoodsBootstrapResponse struct {
	CollectionIDs           []uint64                                   `json:"collection_ids"`
	DefaultStatus           int                                        `json:"default_status"`
	DefaultStatusLabel      string                                     `json:"default_status_label"`
	DryRun                  bool                                       `json:"dry_run"`
	RequestedCount          int                                        `json:"requested_count"`
	FoundCollectionCount    int                                        `json:"found_collection_count"`
	SkippedUGCCount         int                                        `json:"skipped_ugc_count"`
	SkippedUGCCollectionIDs []uint64                                   `json:"skipped_ugc_collection_ids,omitempty"`
	MissingCollectionIDs    []uint64                                   `json:"missing_collection_ids,omitempty"`
	ExistingGoodsCount      int                                        `json:"existing_goods_count"`
	PlannedCreateCount      int                                        `json:"planned_create_count"`
	CreatedCount            int64                                      `json:"created_count"`
	CreatedItems            []AdminCollectionGoodsBootstrapCreatedItem `json:"created_items,omitempty"`
	Message                 string                                     `json:"message"`
}

type AdminCollectionGoodsInitMissingItem struct {
	CollectionID uint64    `json:"collection_id"`
	Title        string    `json:"title"`
	FileCount    int       `json:"file_count"`
	CreatedAt    time.Time `json:"created_at"`
}

type AdminCollectionGoodsInitSummaryResponse struct {
	TotalCollections       int64                                 `json:"total_collections"`
	InitializedCollections int64                                 `json:"initialized_collections"`
	MissingCollections     int64                                 `json:"missing_collections"`
	MissingItems           []AdminCollectionGoodsInitMissingItem `json:"missing_items,omitempty"`
}

type AdminCollectionGoodsSyncMissingRequest struct {
	Limit         int   `json:"limit"`
	DefaultStatus *int  `json:"default_status"`
	DryRun        *bool `json:"dry_run"`
}

type AdminCollectionGoodsSyncMissingResponse struct {
	DryRun             bool                                       `json:"dry_run"`
	Limit              int                                        `json:"limit"`
	DefaultStatus      int                                        `json:"default_status"`
	DefaultStatusLabel string                                     `json:"default_status_label"`
	MissingTotal       int64                                      `json:"missing_total"`
	PlannedCreateCount int                                        `json:"planned_create_count"`
	CreatedCount       int64                                      `json:"created_count"`
	CreatedItems       []AdminCollectionGoodsBootstrapCreatedItem `json:"created_items,omitempty"`
	Message            string                                     `json:"message"`
}

type AdminCollectionGoodsCommandRequest struct {
	Command         string `json:"command"`
	AutoInitMissing *bool  `json:"auto_init_missing"`
	DryRun          *bool  `json:"dry_run"`
}

type AdminCollectionGoodsCommandCreatedItem struct {
	ID           uint64 `json:"id"`
	CollectionID uint64 `json:"collection_id"`
	GoodsNo      string `json:"goods_no"`
	GoodsName    string `json:"goods_name"`
	Status       int    `json:"status"`
}

type AdminCollectionGoodsCommandResponse struct {
	Command                     string                                   `json:"command"`
	Action                      string                                   `json:"action"`
	Status                      int                                      `json:"status"`
	StatusLabel                 string                                   `json:"status_label"`
	CollectionIDs               []uint64                                 `json:"collection_ids"`
	AutoInitMissing             bool                                     `json:"auto_init_missing"`
	DryRun                      bool                                     `json:"dry_run"`
	FoundCollectionCount        int                                      `json:"found_collection_count"`
	SkippedUGCCount             int                                      `json:"skipped_ugc_count"`
	SkippedUGCCollectionIDs     []uint64                                 `json:"skipped_ugc_collection_ids,omitempty"`
	MissingCollectionCount      int                                      `json:"missing_collection_count"`
	MissingCollectionIDs        []uint64                                 `json:"missing_collection_ids,omitempty"`
	ExistingGoodsCount          int                                      `json:"existing_goods_count"`
	MissingGoodsCollectionCount int                                      `json:"missing_goods_collection_count"`
	MissingGoodsCollectionIDs   []uint64                                 `json:"missing_goods_collection_ids,omitempty"`
	UpdatedCount                int64                                    `json:"updated_count"`
	CreatedCount                int64                                    `json:"created_count"`
	CreatedItems                []AdminCollectionGoodsCommandCreatedItem `json:"created_items,omitempty"`
	Message                     string                                   `json:"message"`
}

func normalizeCollectionGoodStatus(raw int) (int, bool) {
	switch raw {
	case adminCollectionGoodStatusOnShelf, adminCollectionGoodStatusOffShelf:
		return raw, true
	default:
		return 0, false
	}
}

func collectionGoodStatusLabel(status int) string {
	if status == adminCollectionGoodStatusOnShelf {
		return "在架"
	}
	return "下架"
}

func normalizeCollectionGoodType(raw int) (int, bool) {
	switch raw {
	case 1, 2, 3:
		return raw, true
	default:
		return 0, false
	}
}

func normalizeTemplateJSON(raw json.RawMessage) (datatypes.JSON, error) {
	if len(raw) == 0 {
		return datatypes.JSON([]byte("[]")), nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return datatypes.JSON([]byte("[]")), nil
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, errors.New("template_json must be valid JSON")
	}
	return datatypes.JSON([]byte(trimmed)), nil
}

func normalizePageValue(raw string, def int) int {
	v, _ := strconv.Atoi(strings.TrimSpace(raw))
	if v <= 0 {
		return def
	}
	return v
}

func normalizeCollectionGoodsCommandAction(raw string) (string, int, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", 0, errors.New("command is required")
	}
	lower := strings.ToLower(text)
	upMatched := strings.Contains(text, "上架") ||
		strings.Contains(text, "在架") ||
		strings.Contains(text, "开售") ||
		strings.Contains(text, "发布") ||
		strings.Contains(lower, "up")
	downMatched := strings.Contains(text, "下架") ||
		strings.Contains(text, "停售") ||
		strings.Contains(text, "停用") ||
		strings.Contains(text, "隐藏") ||
		strings.Contains(lower, "down")
	if upMatched && downMatched {
		return "", 0, errors.New("command action is ambiguous, contains both up/down")
	}
	if upMatched {
		return "up", adminCollectionGoodStatusOnShelf, nil
	}
	if downMatched {
		return "down", adminCollectionGoodStatusOffShelf, nil
	}
	return "", 0, errors.New("command must contain action keyword like 上架/下架")
}

func parseCollectionIDsFromCommand(command string, max int) ([]uint64, error) {
	text := strings.TrimSpace(command)
	if text == "" {
		return nil, errors.New("command is required")
	}
	if max <= 0 {
		max = adminCollectionGoodsMaxBatch
	}
	idSet := map[uint64]struct{}{}
	ids := make([]uint64, 0, 16)
	addID := func(id uint64) error {
		if id == 0 {
			return nil
		}
		if _, exists := idSet[id]; exists {
			return nil
		}
		if len(ids) >= max {
			return fmt.Errorf("too many collection ids in command, max %d", max)
		}
		idSet[id] = struct{}{}
		ids = append(ids, id)
		return nil
	}

	working := text
	rangeMatches := collectionGoodsCommandRangeRE.FindAllStringSubmatch(text, -1)
	for _, match := range rangeMatches {
		if len(match) < 3 {
			continue
		}
		start, err1 := strconv.ParseUint(match[1], 10, 64)
		end, err2 := strconv.ParseUint(match[2], 10, 64)
		if err1 != nil || err2 != nil || start == 0 || end == 0 {
			continue
		}
		if start > end {
			start, end = end, start
		}
		if end-start > uint64(max*4) {
			return nil, fmt.Errorf("collection id range is too large: %d-%d", start, end)
		}
		for i := start; i <= end; i++ {
			if err := addID(i); err != nil {
				return nil, err
			}
			if i == ^uint64(0) {
				break
			}
		}
	}
	working = collectionGoodsCommandRangeRE.ReplaceAllString(working, " ")

	singleMatches := collectionGoodsCommandDigitRE.FindAllString(working, -1)
	for _, token := range singleMatches {
		id, err := strconv.ParseUint(token, 10, 64)
		if err != nil {
			continue
		}
		if err := addID(id); err != nil {
			return nil, err
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("no collection ids found in command")
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func boolValueOrDefault(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func normalizeIDList(rawIDs []uint64, max int) ([]uint64, error) {
	if max <= 0 {
		max = adminCollectionGoodsMaxBatch
	}
	idSet := map[uint64]struct{}{}
	ids := make([]uint64, 0, len(rawIDs))
	for _, id := range rawIDs {
		if id == 0 {
			continue
		}
		if _, exists := idSet[id]; exists {
			continue
		}
		if len(ids) >= max {
			return nil, fmt.Errorf("too many ids, max %d", max)
		}
		idSet[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (h *Handler) getDefaultCollectionGoodGoodsNo(collectionID uint64) string {
	prefix := strings.TrimSpace(h.cfg.GoofishGoodsNoPrefix)
	if prefix == "" {
		prefix = "emoji_col_"
	}
	return fmt.Sprintf("%s%d", prefix, collectionID)
}

func (h *Handler) getUniqueCollectionGoodGoodsNo(collectionID uint64) (string, error) {
	return h.getUniqueCollectionGoodGoodsNoWithDB(h.db, collectionID)
}

func (h *Handler) getUniqueCollectionGoodGoodsNoWithDB(db *gorm.DB, collectionID uint64) (string, error) {
	base := h.getDefaultCollectionGoodGoodsNo(collectionID)
	candidate := base
	for i := 0; i < 50; i++ {
		var count int64
		if err := db.Model(&models.CollectionGood{}).Where("goods_no = ?", candidate).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s_%d", base, i+1)
	}
	return "", errors.New("failed to generate unique goods_no")
}

func getDefaultCollectionGoodImageCount(fileCount int) int {
	if fileCount <= 0 {
		return 6
	}
	if fileCount < 6 {
		return fileCount
	}
	return 6
}

func defaultCollectionGoodStatusByVisibility(visibility string) int {
	if strings.EqualFold(strings.TrimSpace(visibility), "public") {
		return adminCollectionGoodStatusOnShelf
	}
	return adminCollectionGoodStatusOffShelf
}

func shouldInitializeCollectionGood(collection models.Collection) bool {
	return normalizeUGCSource(collection.Source) != ugcCollectionSource
}

func (h *Handler) nonUGCCollectionSubQuery(db *gorm.DB) *gorm.DB {
	base := db
	if base == nil {
		base = h.db
	}
	return base.
		Session(&gorm.Session{NewDB: true}).
		Model(&models.Collection{}).
		Select("id").
		Where("LOWER(COALESCE(source, '')) <> ?", ugcCollectionSource)
}

func (h *Handler) scopeOperationCollectionGoods(db *gorm.DB) *gorm.DB {
	if db == nil {
		db = h.db
	}
	return db.Where("collection_id IN (?)", h.nonUGCCollectionSubQuery(db))
}

func (h *Handler) resolveDefaultCollectionGoodPriceStock() (int64, int) {
	defaultPrice := h.cfg.GoofishDefaultPriceCents
	if defaultPrice <= 0 {
		defaultPrice = 199
	}
	defaultStock := h.cfg.GoofishDefaultInventory
	if defaultStock < 0 {
		defaultStock = 9999
	}
	return defaultPrice, defaultStock
}

func (h *Handler) buildDefaultCollectionGoodRow(
	db *gorm.DB,
	collection models.Collection,
	defaultStatus int,
) (models.CollectionGood, error) {
	goodsNo, err := h.getUniqueCollectionGoodGoodsNoWithDB(db, collection.ID)
	if err != nil {
		return models.CollectionGood{}, err
	}
	goodsName := strings.TrimSpace(collection.Title)
	if goodsName == "" {
		goodsName = fmt.Sprintf("合集#%d", collection.ID)
	}
	defaultPrice, defaultStock := h.resolveDefaultCollectionGoodPriceStock()
	return models.CollectionGood{
		CollectionID: collection.ID,
		GoodsNo:      goodsNo,
		GoodsType:    2,
		GoodsName:    goodsName,
		Price:        defaultPrice,
		Stock:        defaultStock,
		Status:       defaultStatus,
		ImageCount:   getDefaultCollectionGoodImageCount(collection.FileCount),
		ImageStart:   1,
		TemplateJSON: datatypes.JSON([]byte("[]")),
	}, nil
}

func (h *Handler) ensureCollectionGoodInitializedForCollection(
	db *gorm.DB,
	collection models.Collection,
	defaultStatus int,
) (*models.CollectionGood, bool, error) {
	if collection.ID == 0 {
		return nil, false, errors.New("collection id is required")
	}
	if !shouldInitializeCollectionGood(collection) {
		return nil, false, nil
	}
	var existing models.CollectionGood
	if err := db.Select("id").Where("collection_id = ?", collection.ID).Order("id ASC").First(&existing).Error; err == nil {
		return nil, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	row, err := h.buildDefaultCollectionGoodRow(db, collection, defaultStatus)
	if err != nil {
		return nil, false, err
	}
	if err := db.Create(&row).Error; err != nil {
		return nil, false, err
	}
	return &row, true, nil
}

func (h *Handler) queryMissingCollectionGoodsCollections(
	limit int,
) ([]models.Collection, int64, error) {
	missingQuery := h.db.
		Model(&models.Collection{}).
		Where("LOWER(COALESCE(source, '')) <> ?", ugcCollectionSource).
		Where("NOT EXISTS (?)",
			h.db.Model(&models.CollectionGood{}).
				Select("1").
				Where("archive.collection_goods.collection_id = archive.collections.id"),
		)

	var total int64
	if err := missingQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > adminCollectionGoodsMaxBatch {
		limit = adminCollectionGoodsMaxBatch
	}

	var rows []models.Collection
	if err := missingQuery.
		Select("id,title,source,file_count,created_at").
		Order("id DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func normalizeCollectionGoodItem(
	row models.CollectionGood,
	collectionMap map[uint64]models.Collection,
	qiniuClient *storage.QiniuClient,
) AdminCollectionGoodItem {
	templateJSON := row.TemplateJSON
	if len(templateJSON) == 0 {
		templateJSON = datatypes.JSON([]byte("[]"))
	}
	collection := collectionMap[row.CollectionID]
	return AdminCollectionGoodItem{
		ID:                 row.ID,
		CollectionID:       row.CollectionID,
		CollectionTitle:    collection.Title,
		CollectionCoverURL: resolveListPreviewURL(collection.CoverURL, qiniuClient),
		GoodsNo:            row.GoodsNo,
		GoodsType:          row.GoodsType,
		GoodsName:          row.GoodsName,
		Price:              row.Price,
		Stock:              row.Stock,
		Status:             row.Status,
		ImageCount:         row.ImageCount,
		ImageStart:         row.ImageStart,
		TemplateJSON:       json.RawMessage(templateJSON),
		LastSyncAt:         row.LastSyncAt,
		LastSyncError:      strings.TrimSpace(row.LastSyncError),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func (h *Handler) loadCollectionMap(ids []uint64) map[uint64]models.Collection {
	result := map[uint64]models.Collection{}
	if len(ids) == 0 {
		return result
	}
	var collections []models.Collection
	if err := h.db.
		Select("id,title,cover_url").
		Where("id IN ?", ids).
		Where("LOWER(COALESCE(source, '')) <> ?", ugcCollectionSource).
		Find(&collections).Error; err != nil {
		return result
	}
	for _, item := range collections {
		result[item.ID] = item
	}
	return result
}

func (h *Handler) ensureCollectionExists(collectionID uint64) error {
	if collectionID == 0 {
		return errors.New("collection_id is required")
	}
	var collection models.Collection
	if err := h.db.
		Select("id,source").
		Where("id = ?", collectionID).
		First(&collection).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("collection_id not found")
		}
		return err
	}
	if !shouldInitializeCollectionGood(collection) {
		return errors.New("collection_id is ugc collection, not allowed")
	}
	return nil
}

func (h *Handler) ensureGoodsNoUnique(goodsNo string, excludeID uint64) error {
	if strings.TrimSpace(goodsNo) == "" {
		return errors.New("goods_no is required")
	}
	query := h.db.Model(&models.CollectionGood{}).Where("goods_no = ?", goodsNo)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("goods_no already exists")
	}
	return nil
}

func (h *Handler) buildCollectionGoodItemByID(id uint64) (*AdminCollectionGoodItem, error) {
	var row models.CollectionGood
	if err := h.scopeOperationCollectionGoods(h.db.Model(&models.CollectionGood{})).
		Where("id = ?", id).
		First(&row).Error; err != nil {
		return nil, err
	}
	collectionMap := h.loadCollectionMap([]uint64{row.CollectionID})
	item := normalizeCollectionGoodItem(row, collectionMap, h.qiniu)
	return &item, nil
}

// ListAdminCollectionGoods 列出商品库。
// @Summary List collection goods (admin)
// @Tags admin
// @Produce json
// @Success 200 {object} AdminCollectionGoodsListResponse
// @Router /api/admin/collection-goods [get]
func (h *Handler) ListAdminCollectionGoods(c *gin.Context) {
	page := normalizePageValue(c.DefaultQuery("page", "1"), 1)
	pageSize := normalizePageValue(c.DefaultQuery("page_size", "20"), 20)
	if pageSize > 200 {
		pageSize = 200
	}

	q := strings.TrimSpace(c.Query("q"))
	statusRaw := strings.TrimSpace(c.Query("status"))
	goodsTypeRaw := strings.TrimSpace(c.Query("goods_type"))
	collectionIDRaw := strings.TrimSpace(c.Query("collection_id"))

	query := h.scopeOperationCollectionGoods(h.db.Model(&models.CollectionGood{}))
	if q != "" {
		like := "%" + q + "%"
		collectionTitleQuery := h.nonUGCCollectionSubQuery(h.db).Where("title ILIKE ?", like)
		query = query.Where(
			"(goods_no ILIKE ? OR goods_name ILIKE ? OR collection_id IN (?))",
			like,
			like,
			collectionTitleQuery,
		)
	}
	if statusRaw != "" && !strings.EqualFold(statusRaw, "all") {
		status, err := strconv.Atoi(statusRaw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		normalized, ok := normalizeCollectionGoodStatus(status)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status, expected 1 or 2"})
			return
		}
		query = query.Where("status = ?", normalized)
	}
	if goodsTypeRaw != "" && !strings.EqualFold(goodsTypeRaw, "all") {
		goodsType, err := strconv.Atoi(goodsTypeRaw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid goods_type"})
			return
		}
		normalized, ok := normalizeCollectionGoodType(goodsType)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid goods_type, expected 1/2/3"})
			return
		}
		query = query.Where("goods_type = ?", normalized)
	}
	if collectionIDRaw != "" {
		collectionID, err := strconv.ParseUint(collectionIDRaw, 10, 64)
		if err != nil || collectionID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid collection_id"})
			return
		}
		query = query.Where("collection_id = ?", collectionID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count collection goods"})
		return
	}

	var rows []models.CollectionGood
	if err := query.Order("updated_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query collection goods"})
		return
	}

	collectionIDSet := map[uint64]struct{}{}
	for _, row := range rows {
		collectionIDSet[row.CollectionID] = struct{}{}
	}
	collectionIDs := make([]uint64, 0, len(collectionIDSet))
	for id := range collectionIDSet {
		collectionIDs = append(collectionIDs, id)
	}
	collectionMap := h.loadCollectionMap(collectionIDs)

	items := make([]AdminCollectionGoodItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, normalizeCollectionGoodItem(row, collectionMap, h.qiniu))
	}

	c.JSON(http.StatusOK, AdminCollectionGoodsListResponse{
		Items: items,
		Total: total,
	})
}

// CreateAdminCollectionGood 新增商品库记录。
// @Summary Create collection good (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body AdminCollectionGoodUpsertRequest true "create request"
// @Success 200 {object} AdminCollectionGoodItem
// @Router /api/admin/collection-goods [post]
func (h *Handler) CreateAdminCollectionGood(c *gin.Context) {
	var req AdminCollectionGoodUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	goodsNo := strings.TrimSpace(req.GoodsNo)
	goodsName := strings.TrimSpace(req.GoodsName)
	goodsType, ok := normalizeCollectionGoodType(req.GoodsType)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "goods_type must be 1/2/3"})
		return
	}
	status, ok := normalizeCollectionGoodStatus(req.Status)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be 1/2"})
		return
	}
	if goodsNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "goods_no is required"})
		return
	}
	if goodsName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "goods_name is required"})
		return
	}
	if req.Price < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price must be >= 0"})
		return
	}
	if req.Stock < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "stock must be >= 0"})
		return
	}
	if req.ImageCount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image_count must be > 0"})
		return
	}
	if req.ImageStart <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image_start must be > 0"})
		return
	}

	if err := h.ensureCollectionExists(req.CollectionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.ensureGoodsNoUnique(goodsNo, 0); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	templateJSON, err := normalizeTemplateJSON(req.TemplateJSON)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	row := models.CollectionGood{
		CollectionID: req.CollectionID,
		GoodsNo:      goodsNo,
		GoodsType:    goodsType,
		GoodsName:    goodsName,
		Price:        req.Price,
		Stock:        req.Stock,
		Status:       status,
		ImageCount:   req.ImageCount,
		ImageStart:   req.ImageStart,
		TemplateJSON: templateJSON,
	}
	if err := h.db.Create(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create collection good"})
		return
	}
	item, err := h.buildCollectionGoodItemByID(row.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load created collection good"})
		return
	}
	c.JSON(http.StatusOK, item)
}

// UpdateAdminCollectionGood 更新商品库记录。
// @Summary Update collection good (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param id path int true "id"
// @Param body body AdminCollectionGoodUpsertRequest true "update request"
// @Success 200 {object} AdminCollectionGoodItem
// @Router /api/admin/collection-goods/{id} [put]
func (h *Handler) UpdateAdminCollectionGood(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req AdminCollectionGoodUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	goodsNo := strings.TrimSpace(req.GoodsNo)
	goodsName := strings.TrimSpace(req.GoodsName)
	goodsType, ok := normalizeCollectionGoodType(req.GoodsType)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "goods_type must be 1/2/3"})
		return
	}
	status, ok := normalizeCollectionGoodStatus(req.Status)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be 1/2"})
		return
	}
	if goodsNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "goods_no is required"})
		return
	}
	if goodsName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "goods_name is required"})
		return
	}
	if req.Price < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price must be >= 0"})
		return
	}
	if req.Stock < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "stock must be >= 0"})
		return
	}
	if req.ImageCount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image_count must be > 0"})
		return
	}
	if req.ImageStart <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image_start must be > 0"})
		return
	}

	if err := h.ensureCollectionExists(req.CollectionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.ensureGoodsNoUnique(goodsNo, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	templateJSON, err := normalizeTemplateJSON(req.TemplateJSON)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var row models.CollectionGood
	if err := h.scopeOperationCollectionGoods(h.db.Model(&models.CollectionGood{})).
		Where("id = ?", id).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection good not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query collection good"})
		return
	}

	row.CollectionID = req.CollectionID
	row.GoodsNo = goodsNo
	row.GoodsType = goodsType
	row.GoodsName = goodsName
	row.Price = req.Price
	row.Stock = req.Stock
	row.Status = status
	row.ImageCount = req.ImageCount
	row.ImageStart = req.ImageStart
	row.TemplateJSON = templateJSON
	if err := h.db.Save(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update collection good"})
		return
	}
	item, err := h.buildCollectionGoodItemByID(row.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load updated collection good"})
		return
	}
	c.JSON(http.StatusOK, item)
}

// BatchUpdateAdminCollectionGoodStatus 批量更新商品状态（1在架/2下架）。
// @Summary Batch update collection good status (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body AdminCollectionGoodBatchStatusRequest true "batch status request"
// @Success 200 {object} AdminCollectionGoodBatchStatusResponse
// @Router /api/admin/collection-goods/batch-status [post]
func (h *Handler) BatchUpdateAdminCollectionGoodStatus(c *gin.Context) {
	var req AdminCollectionGoodBatchStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status, ok := normalizeCollectionGoodStatus(req.Status)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be 1/2"})
		return
	}
	if len(req.CollectionGoodIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_good_ids is required"})
		return
	}
	if len(req.CollectionGoodIDs) > adminCollectionGoodsMaxBatch {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many collection_good_ids, max 500"})
		return
	}

	idSet := map[uint64]struct{}{}
	ids := make([]uint64, 0, len(req.CollectionGoodIDs))
	for _, id := range req.CollectionGoodIDs {
		if id == 0 {
			continue
		}
		if _, exists := idSet[id]; exists {
			continue
		}
		idSet[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid collection_good_ids"})
		return
	}

	result := h.scopeOperationCollectionGoods(h.db.Model(&models.CollectionGood{})).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}
	c.JSON(http.StatusOK, AdminCollectionGoodBatchStatusResponse{
		UpdatedCount: result.RowsAffected,
		Status:       status,
	})
}

// CommandAdminCollectionGoodsStatus 自然语言指令批量更新商品状态（按合集编号）。
// @Summary Command collection goods status by natural language (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body AdminCollectionGoodsCommandRequest true "command request"
// @Success 200 {object} AdminCollectionGoodsCommandResponse
// @Router /api/admin/collection-goods/command [post]
func (h *Handler) CommandAdminCollectionGoodsStatus(c *gin.Context) {
	var req AdminCollectionGoodsCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	command := strings.TrimSpace(req.Command)
	action, status, err := normalizeCollectionGoodsCommandAction(command)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	collectionIDs, err := parseCollectionIDsFromCommand(command, adminCollectionGoodsMaxBatch)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	autoInitMissing := boolValueOrDefault(req.AutoInitMissing, true)
	dryRun := boolValueOrDefault(req.DryRun, false)

	var collections []models.Collection
	if err := h.db.Select("id,title,file_count,source").Where("id IN ?", collectionIDs).Find(&collections).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query collections"})
		return
	}
	collectionMap := map[uint64]models.Collection{}
	for _, item := range collections {
		collectionMap[item.ID] = item
	}

	foundCollectionIDs := make([]uint64, 0, len(collectionIDs))
	skippedUGCCollectionIDs := make([]uint64, 0)
	missingCollectionIDs := make([]uint64, 0)
	for _, id := range collectionIDs {
		collection, exists := collectionMap[id]
		if exists {
			if !shouldInitializeCollectionGood(collection) {
				skippedUGCCollectionIDs = append(skippedUGCCollectionIDs, id)
				continue
			}
			foundCollectionIDs = append(foundCollectionIDs, id)
		} else {
			missingCollectionIDs = append(missingCollectionIDs, id)
		}
	}

	var existingRows []models.CollectionGood
	if len(foundCollectionIDs) > 0 {
		if err := h.scopeOperationCollectionGoods(h.db.Select("id,collection_id").Model(&models.CollectionGood{})).
			Where("collection_id IN ?", foundCollectionIDs).
			Find(&existingRows).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query collection goods"})
			return
		}
	}
	existingGoodsIDs := make([]uint64, 0, len(existingRows))
	existingCollectionSet := map[uint64]struct{}{}
	for _, row := range existingRows {
		existingGoodsIDs = append(existingGoodsIDs, row.ID)
		existingCollectionSet[row.CollectionID] = struct{}{}
	}
	missingGoodsCollectionIDs := make([]uint64, 0)
	for _, collectionID := range foundCollectionIDs {
		if _, exists := existingCollectionSet[collectionID]; exists {
			continue
		}
		missingGoodsCollectionIDs = append(missingGoodsCollectionIDs, collectionID)
	}

	createdItems := make([]AdminCollectionGoodsCommandCreatedItem, 0)
	updatedCount := int64(0)
	createdCount := int64(0)

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

		if len(existingGoodsIDs) > 0 {
			result := h.scopeOperationCollectionGoods(tx.Model(&models.CollectionGood{})).
				Where("id IN ?", existingGoodsIDs).
				Updates(map[string]interface{}{
					"status":     status,
					"updated_at": time.Now(),
				})
			if result.Error != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update existing collection goods"})
				return
			}
			updatedCount = result.RowsAffected
		}

		if autoInitMissing && len(missingGoodsCollectionIDs) > 0 {
			for _, collectionID := range missingGoodsCollectionIDs {
				collection := collectionMap[collectionID]
				createdRow, created, err := h.ensureCollectionGoodInitializedForCollection(tx, collection, status)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to auto init collection good"})
					return
				}
				if created && createdRow != nil {
					createdCount += 1
					createdItems = append(createdItems, AdminCollectionGoodsCommandCreatedItem{
						ID:           createdRow.ID,
						CollectionID: createdRow.CollectionID,
						GoodsNo:      createdRow.GoodsNo,
						GoodsName:    createdRow.GoodsName,
						Status:       createdRow.Status,
					})
				}
			}
		}
		if err := tx.Commit().Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit transaction"})
			return
		}
		committed = true
	}

	message := fmt.Sprintf(
		"已解析 %d 个合集，状态目标：%s。可操作合集 %d 个，跳过UGC合集 %d 个，存在商品 %d 条，缺失商品合集 %d 个，缺失合集 %d 个。",
		len(collectionIDs),
		collectionGoodStatusLabel(status),
		len(foundCollectionIDs),
		len(skippedUGCCollectionIDs),
		len(existingGoodsIDs),
		len(missingGoodsCollectionIDs),
		len(missingCollectionIDs),
	)
	if dryRun {
		message = "（预览）" + message
	} else {
		if autoInitMissing {
			message = message + fmt.Sprintf(" 已更新 %d 条，自动初始化 %d 条。", updatedCount, createdCount)
		} else {
			message = message + fmt.Sprintf(" 已更新 %d 条。", updatedCount)
		}
	}

	c.JSON(http.StatusOK, AdminCollectionGoodsCommandResponse{
		Command:                     command,
		Action:                      action,
		Status:                      status,
		StatusLabel:                 collectionGoodStatusLabel(status),
		CollectionIDs:               collectionIDs,
		AutoInitMissing:             autoInitMissing,
		DryRun:                      dryRun,
		FoundCollectionCount:        len(foundCollectionIDs),
		SkippedUGCCount:             len(skippedUGCCollectionIDs),
		SkippedUGCCollectionIDs:     skippedUGCCollectionIDs,
		MissingCollectionCount:      len(missingCollectionIDs),
		MissingCollectionIDs:        missingCollectionIDs,
		ExistingGoodsCount:          len(existingGoodsIDs),
		MissingGoodsCollectionCount: len(missingGoodsCollectionIDs),
		MissingGoodsCollectionIDs:   missingGoodsCollectionIDs,
		UpdatedCount:                updatedCount,
		CreatedCount:                createdCount,
		CreatedItems:                createdItems,
		Message:                     message,
	})
}

// BootstrapAdminCollectionGoods 按合集编号初始化商品库（仅创建缺失记录）。
// @Summary Bootstrap collection goods by collection ids (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body AdminCollectionGoodsBootstrapRequest true "bootstrap request"
// @Success 200 {object} AdminCollectionGoodsBootstrapResponse
// @Router /api/admin/collection-goods/bootstrap [post]
func (h *Handler) BootstrapAdminCollectionGoods(c *gin.Context) {
	var req AdminCollectionGoodsBootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ids, err := normalizeIDList(req.CollectionIDs, adminCollectionGoodsMaxBatch)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(ids) == 0 {
		parsed, parseErr := parseCollectionIDsFromCommand(strings.TrimSpace(req.Command), adminCollectionGoodsMaxBatch)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "collection_ids is required"})
			return
		}
		ids = parsed
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_ids is required"})
		return
	}

	defaultStatus := adminCollectionGoodStatusOffShelf
	if req.DefaultStatus != nil {
		normalized, ok := normalizeCollectionGoodStatus(*req.DefaultStatus)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "default_status must be 1/2"})
			return
		}
		defaultStatus = normalized
	}
	dryRun := boolValueOrDefault(req.DryRun, false)

	var collections []models.Collection
	if err := h.db.Select("id,title,source,file_count").Where("id IN ?", ids).Find(&collections).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query collections"})
		return
	}
	collectionMap := map[uint64]models.Collection{}
	for _, item := range collections {
		collectionMap[item.ID] = item
	}

	foundCollectionIDs := make([]uint64, 0, len(ids))
	eligibleCollectionIDs := make([]uint64, 0, len(ids))
	skippedUGCCollectionIDs := make([]uint64, 0)
	missingCollectionIDs := make([]uint64, 0)
	for _, id := range ids {
		collection, exists := collectionMap[id]
		if exists {
			foundCollectionIDs = append(foundCollectionIDs, id)
			if shouldInitializeCollectionGood(collection) {
				eligibleCollectionIDs = append(eligibleCollectionIDs, id)
			} else {
				skippedUGCCollectionIDs = append(skippedUGCCollectionIDs, id)
			}
		} else {
			missingCollectionIDs = append(missingCollectionIDs, id)
		}
	}

	var existingRows []models.CollectionGood
	if len(eligibleCollectionIDs) > 0 {
		if err := h.db.Select("id,collection_id").Where("collection_id IN ?", eligibleCollectionIDs).Find(&existingRows).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query collection goods"})
			return
		}
	}
	existingCollectionSet := map[uint64]struct{}{}
	for _, row := range existingRows {
		existingCollectionSet[row.CollectionID] = struct{}{}
	}
	missingGoodsCollectionIDs := make([]uint64, 0)
	for _, collectionID := range eligibleCollectionIDs {
		if _, exists := existingCollectionSet[collectionID]; exists {
			continue
		}
		missingGoodsCollectionIDs = append(missingGoodsCollectionIDs, collectionID)
	}

	createdItems := make([]AdminCollectionGoodsBootstrapCreatedItem, 0)
	createdCount := int64(0)
	if !dryRun && len(missingGoodsCollectionIDs) > 0 {
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

		for _, collectionID := range missingGoodsCollectionIDs {
			collection := collectionMap[collectionID]
			createdRow, created, err := h.ensureCollectionGoodInitializedForCollection(tx, collection, defaultStatus)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create collection good"})
				return
			}
			if created && createdRow != nil {
				createdCount += 1
				createdItems = append(createdItems, AdminCollectionGoodsBootstrapCreatedItem{
					ID:           createdRow.ID,
					CollectionID: createdRow.CollectionID,
					GoodsNo:      createdRow.GoodsNo,
					GoodsName:    createdRow.GoodsName,
					Status:       createdRow.Status,
				})
			}
		}

		if err := tx.Commit().Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit transaction"})
			return
		}
		committed = true
	}

	message := fmt.Sprintf(
		"共解析 %d 个合集，存在合集 %d 个，缺失合集 %d 个，已存在商品合集 %d 个，待初始化 %d 个，跳过UGC合集 %d 个。",
		len(ids),
		len(foundCollectionIDs),
		len(missingCollectionIDs),
		len(eligibleCollectionIDs)-len(missingGoodsCollectionIDs),
		len(missingGoodsCollectionIDs),
		len(skippedUGCCollectionIDs),
	)
	if dryRun {
		message = "（预览）" + message
	} else {
		message = message + fmt.Sprintf(" 已创建 %d 条商品。", createdCount)
	}

	c.JSON(http.StatusOK, AdminCollectionGoodsBootstrapResponse{
		CollectionIDs:           ids,
		DefaultStatus:           defaultStatus,
		DefaultStatusLabel:      collectionGoodStatusLabel(defaultStatus),
		DryRun:                  dryRun,
		RequestedCount:          len(ids),
		FoundCollectionCount:    len(foundCollectionIDs),
		SkippedUGCCount:         len(skippedUGCCollectionIDs),
		SkippedUGCCollectionIDs: skippedUGCCollectionIDs,
		MissingCollectionIDs:    missingCollectionIDs,
		ExistingGoodsCount:      len(eligibleCollectionIDs) - len(missingGoodsCollectionIDs),
		PlannedCreateCount:      len(missingGoodsCollectionIDs),
		CreatedCount:            createdCount,
		CreatedItems:            createdItems,
		Message:                 message,
	})
}

// GetAdminCollectionGoodsInitSummary 返回商品库初始化覆盖情况。
// @Summary Get collection goods init summary (admin)
// @Tags admin
// @Produce json
// @Success 200 {object} AdminCollectionGoodsInitSummaryResponse
// @Router /api/admin/collection-goods/init-summary [get]
func (h *Handler) GetAdminCollectionGoodsInitSummary(c *gin.Context) {
	previewLimit := normalizePageValue(c.DefaultQuery("preview_limit", "20"), 20)
	if previewLimit > 100 {
		previewLimit = 100
	}

	var totalCollections int64
	if err := h.db.
		Model(&models.Collection{}).
		Where("LOWER(COALESCE(source, '')) <> ?", ugcCollectionSource).
		Count(&totalCollections).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query total collections"})
		return
	}
	missingRows, missingTotal, err := h.queryMissingCollectionGoodsCollections(previewLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query missing collection goods"})
		return
	}
	missingItems := make([]AdminCollectionGoodsInitMissingItem, 0, len(missingRows))
	for _, row := range missingRows {
		missingItems = append(missingItems, AdminCollectionGoodsInitMissingItem{
			CollectionID: row.ID,
			Title:        strings.TrimSpace(row.Title),
			FileCount:    row.FileCount,
			CreatedAt:    row.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, AdminCollectionGoodsInitSummaryResponse{
		TotalCollections:       totalCollections,
		InitializedCollections: totalCollections - missingTotal,
		MissingCollections:     missingTotal,
		MissingItems:           missingItems,
	})
}

// SyncAdminCollectionGoodsMissing 自动同步初始化缺失商品记录。
// @Summary Sync missing collection goods (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body AdminCollectionGoodsSyncMissingRequest true "sync request"
// @Success 200 {object} AdminCollectionGoodsSyncMissingResponse
// @Router /api/admin/collection-goods/sync-missing [post]
func (h *Handler) SyncAdminCollectionGoodsMissing(c *gin.Context) {
	var req AdminCollectionGoodsSyncMissingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defaultStatus := adminCollectionGoodStatusOffShelf
	if req.DefaultStatus != nil {
		normalized, ok := normalizeCollectionGoodStatus(*req.DefaultStatus)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "default_status must be 1/2"})
			return
		}
		defaultStatus = normalized
	}
	dryRun := boolValueOrDefault(req.DryRun, false)
	limit := req.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > adminCollectionGoodsMaxBatch {
		limit = adminCollectionGoodsMaxBatch
	}

	missingRows, missingTotal, err := h.queryMissingCollectionGoodsCollections(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query missing collection goods"})
		return
	}
	plannedCount := len(missingRows)
	createdItems := make([]AdminCollectionGoodsBootstrapCreatedItem, 0, plannedCount)
	createdCount := int64(0)

	if !dryRun && plannedCount > 0 {
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

		for _, collection := range missingRows {
			createdRow, created, err := h.ensureCollectionGoodInitializedForCollection(tx, collection, defaultStatus)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create missing collection good"})
				return
			}
			if created && createdRow != nil {
				createdCount += 1
				createdItems = append(createdItems, AdminCollectionGoodsBootstrapCreatedItem{
					ID:           createdRow.ID,
					CollectionID: createdRow.CollectionID,
					GoodsNo:      createdRow.GoodsNo,
					GoodsName:    createdRow.GoodsName,
					Status:       createdRow.Status,
				})
			}
		}
		if err := tx.Commit().Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit transaction"})
			return
		}
		committed = true
	}

	message := fmt.Sprintf("识别缺失合集 %d 个，本次计划初始化 %d 个。", missingTotal, plannedCount)
	if dryRun {
		message = "（预览）" + message
	} else {
		message = message + fmt.Sprintf(" 已创建 %d 条。", createdCount)
	}

	c.JSON(http.StatusOK, AdminCollectionGoodsSyncMissingResponse{
		DryRun:             dryRun,
		Limit:              limit,
		DefaultStatus:      defaultStatus,
		DefaultStatusLabel: collectionGoodStatusLabel(defaultStatus),
		MissingTotal:       missingTotal,
		PlannedCreateCount: plannedCount,
		CreatedCount:       createdCount,
		CreatedItems:       createdItems,
		Message:            message,
	})
}

// ExportAdminCollectionGoodsTemplateCSV 导出商品库填写模板。
// @Summary Export collection goods template csv (admin)
// @Tags admin
// @Produce text/csv
// @Success 200 {string} string
// @Router /api/admin/collection-goods/template.csv [get]
func (h *Handler) ExportAdminCollectionGoodsTemplateCSV(c *gin.Context) {
	var buf bytes.Buffer
	_, _ = buf.WriteString("\uFEFF")
	writer := csv.NewWriter(&buf)

	headers := []string{
		"goods_no",
		"collection_id",
		"goods_type",
		"goods_name",
		"price",
		"stock",
		"status",
		"image_count",
		"image_start",
		"template_json",
	}
	_ = writer.Write(headers)

	_ = writer.Write([]string{
		"emoji_col_10001",
		"10001",
		"2",
		"测试商品-卡密",
		"199",
		"9999",
		"1",
		"6",
		"1",
		"[]",
	})
	_ = writer.Write([]string{
		"emoji_col_10002",
		"10002",
		"1",
		"测试商品-直充",
		"299",
		"9999",
		"2",
		"8",
		"1",
		"[{\"code\":\"account\",\"name\":\"充值账号\",\"desc\":\"仅支持手机号\",\"check\":1}]",
	})
	writer.Flush()
	if err := writer.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build csv template"})
		return
	}

	filename := fmt.Sprintf("collection_goods_template_%s.csv", time.Now().Format("20060102"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}
