package handlers

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
)

const goofishBatchPublishMax = 500

type AdminGoofishPublishConfigResponse struct {
	Enabled            bool   `json:"enabled"`
	BaseURL            string `json:"base_url,omitempty"`
	Path               string `json:"path,omitempty"`
	TimeoutSeconds     int    `json:"timeout_seconds"`
	AuthHeader         string `json:"auth_header,omitempty"`
	HasAuthToken       bool   `json:"has_auth_token"`
	SignEnabled        bool   `json:"sign_enabled"`
	HasSignCredentials bool   `json:"has_sign_credentials"`
	SignAppID          string `json:"sign_app_id,omitempty"`
	SignMchID          string `json:"sign_mch_id,omitempty"`
	GoodsNoPrefix      string `json:"goods_no_prefix"`
	DefaultPriceCents  int64  `json:"default_price_cents"`
	DefaultInventory   int    `json:"default_inventory"`
	RecommendedPayload string `json:"recommended_payload"`
}

type AdminGoofishBatchPublishRequest struct {
	CollectionIDs  []uint64 `json:"collection_ids"`
	Action         string   `json:"action"` // upsert | remove
	SyncVisibility *bool    `json:"sync_visibility"`
	PriceCents     *int64   `json:"price_cents"`
	Inventory      *int     `json:"inventory"`
}

type GoofishBatchPublishResultItem struct {
	CollectionID uint64 `json:"collection_id"`
	GoodsNo      string `json:"goods_no,omitempty"`
	Success      bool   `json:"success"`
	StatusCode   int    `json:"status_code,omitempty"`
	Message      string `json:"message,omitempty"`
}

type AdminGoofishBatchPublishResponse struct {
	Action              string                          `json:"action"`
	RequestedCount      int                             `json:"requested_count"`
	PushedCount         int                             `json:"pushed_count"`
	SuccessCount        int                             `json:"success_count"`
	FailureCount        int                             `json:"failure_count"`
	PriceCents          int64                           `json:"price_cents"`
	Inventory           int                             `json:"inventory"`
	SyncVisibility      bool                            `json:"sync_visibility"`
	SyncedVisibility    string                          `json:"synced_visibility,omitempty"`
	SyncedVisibilityCnt int                             `json:"synced_visibility_count,omitempty"`
	AdapterEndpoint     string                          `json:"adapter_endpoint"`
	Results             []GoofishBatchPublishResultItem `json:"results"`
}

type goofishAdapterBatchPublishRequest struct {
	Action    string                   `json:"action"`
	Source    string                   `json:"source"`
	Timestamp string                   `json:"timestamp"`
	Items     []goofishAdapterGoodItem `json:"items"`
}

type goofishAdapterGoodItem struct {
	CollectionID uint64 `json:"collection_id"`
	GoodsNo      string `json:"goods_no"`
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	CategoryID   uint64 `json:"category_id,omitempty"`
	CategoryName string `json:"category_name,omitempty"`
	Slug         string `json:"slug,omitempty"`
	CoverURL     string `json:"cover_url,omitempty"`
	ZipKey       string `json:"zip_key,omitempty"`
	ZipURL       string `json:"zip_url,omitempty"`
	FileCount    int    `json:"file_count"`
	Visibility   string `json:"visibility"`
	Status       string `json:"status"`
	PriceCents   int64  `json:"price_cents"`
	Inventory    int    `json:"inventory"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

type goofishAdapterBatchPublishResponse struct {
	Success *bool                             `json:"success,omitempty"`
	Code    interface{}                       `json:"code,omitempty"`
	Message string                            `json:"message,omitempty"`
	Data    *goofishAdapterBatchPublishResult `json:"data,omitempty"`
	Results []goofishAdapterResultItem        `json:"results,omitempty"`
}

type goofishAdapterBatchPublishResult struct {
	Results []goofishAdapterResultItem `json:"results,omitempty"`
}

type goofishAdapterResultItem struct {
	CollectionID uint64 `json:"collection_id,omitempty"`
	GoodsNo      string `json:"goods_no,omitempty"`
	Success      *bool  `json:"success,omitempty"`
	StatusCode   int    `json:"status_code,omitempty"`
	Message      string `json:"message,omitempty"`
}

func normalizeGoofishPublishPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "/goofish/goods/batch-upsert"
	}
	if strings.HasPrefix(trimmed, "/") {
		return trimmed
	}
	return "/" + trimmed
}

func normalizeGoofishPublishAction(raw string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "", "upsert", "publish", "up":
		return "upsert", true
	case "remove", "unpublish", "down":
		return "remove", true
	default:
		return "", false
	}
}

func boolPtrValue(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func summarizeRemoteBody(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > 240 {
		return trimmed[:240] + "..."
	}
	return trimmed
}

func mapGoofishResultByCollectionID(items []goofishAdapterResultItem) map[uint64]goofishAdapterResultItem {
	result := make(map[uint64]goofishAdapterResultItem, len(items))
	for _, item := range items {
		if item.CollectionID == 0 {
			continue
		}
		result[item.CollectionID] = item
	}
	return result
}

func md5Hex(raw string) string {
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (h *Handler) hasGoofishSignCredentials() bool {
	if h == nil {
		return false
	}
	return strings.TrimSpace(h.cfg.GoofishSignAppID) != "" &&
		strings.TrimSpace(h.cfg.GoofishSignAppSecret) != "" &&
		strings.TrimSpace(h.cfg.GoofishSignMchID) != "" &&
		strings.TrimSpace(h.cfg.GoofishSignMchSecret) != ""
}

func (h *Handler) attachGoofishSignature(req *http.Request, body []byte) {
	if h == nil || req == nil || !h.cfg.GoofishSignEnabled || !h.hasGoofishSignCredentials() {
		return
	}
	appID := strings.TrimSpace(h.cfg.GoofishSignAppID)
	appSecret := strings.TrimSpace(h.cfg.GoofishSignAppSecret)
	mchID := strings.TrimSpace(h.cfg.GoofishSignMchID)
	mchSecret := strings.TrimSpace(h.cfg.GoofishSignMchSecret)
	timestamp := time.Now().Unix()
	bodyMD5 := md5Hex(string(body))
	signRaw := fmt.Sprintf("%s,%s,%s,%d,%s,%s", appID, appSecret, bodyMD5, timestamp, mchID, mchSecret)
	sign := md5Hex(signRaw)

	query := req.URL.Query()
	query.Set("app_id", appID)
	query.Set("mch_id", mchID)
	query.Set("timestamp", strconv.FormatInt(timestamp, 10))
	query.Set("sign", sign)
	req.URL.RawQuery = query.Encode()
}

func (h *Handler) getGoofishPublishEndpoint() (string, bool) {
	base := strings.TrimRight(strings.TrimSpace(h.cfg.GoofishPublishBaseURL), "/")
	if base == "" {
		return "", false
	}
	return base + normalizeGoofishPublishPath(h.cfg.GoofishPublishPath), true
}

func (h *Handler) getGoofishGoodsNo(collectionID uint64) string {
	prefix := strings.TrimSpace(h.cfg.GoofishGoodsNoPrefix)
	if prefix == "" {
		prefix = "emoji_col_"
	}
	return fmt.Sprintf("%s%d", prefix, collectionID)
}

func (h *Handler) loadGoofishCategoryNameMap(categoryIDs []uint64) map[uint64]string {
	result := map[uint64]string{}
	if len(categoryIDs) == 0 {
		return result
	}
	var rows []models.Category
	if err := h.db.Where("id IN ?", categoryIDs).Find(&rows).Error; err != nil {
		return result
	}
	for _, row := range rows {
		result[row.ID] = row.Name
	}
	return result
}

func (h *Handler) callGoofishAdapter(
	ctx context.Context,
	endpoint string,
	payload goofishAdapterBatchPublishRequest,
) (int, []byte, *goofishAdapterBatchPublishResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, nil, err
	}

	timeoutSec := h.cfg.GoofishPublishTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	h.attachGoofishSignature(req, body)
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(h.cfg.GoofishPublishAuthToken); token != "" {
		header := strings.TrimSpace(h.cfg.GoofishPublishAuthHeader)
		if header == "" {
			header = "X-Access-Token"
		}
		req.Header.Set(header, token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	parsed := &goofishAdapterBatchPublishResponse{}
	if err := json.Unmarshal(raw, parsed); err != nil {
		parsed = nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, raw, parsed, fmt.Errorf("adapter returned HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, raw, parsed, nil
}

// GetAdminGoofishPublishConfig returns current self-developed Goofish adapter config status.
// @Summary Get goofish publish config (admin)
// @Tags admin
// @Produce json
// @Success 200 {object} AdminGoofishPublishConfigResponse
// @Router /api/admin/goofish/publish-config [get]
func (h *Handler) GetAdminGoofishPublishConfig(c *gin.Context) {
	endpoint, enabled := h.getGoofishPublishEndpoint()
	parts := strings.SplitN(endpoint, "/", 4)
	base := ""
	path := normalizeGoofishPublishPath(h.cfg.GoofishPublishPath)
	if enabled && len(parts) >= 3 {
		base = strings.Join(parts[:3], "/")
		if len(parts) == 4 {
			path = "/" + parts[3]
		}
	}
	timeout := h.cfg.GoofishPublishTimeoutSec
	if timeout <= 0 {
		timeout = 20
	}
	c.JSON(http.StatusOK, AdminGoofishPublishConfigResponse{
		Enabled:            enabled,
		BaseURL:            base,
		Path:               path,
		TimeoutSeconds:     timeout,
		AuthHeader:         strings.TrimSpace(h.cfg.GoofishPublishAuthHeader),
		HasAuthToken:       strings.TrimSpace(h.cfg.GoofishPublishAuthToken) != "",
		SignEnabled:        h.cfg.GoofishSignEnabled,
		HasSignCredentials: h.hasGoofishSignCredentials(),
		SignAppID:          strings.TrimSpace(h.cfg.GoofishSignAppID),
		SignMchID:          strings.TrimSpace(h.cfg.GoofishSignMchID),
		GoodsNoPrefix:      strings.TrimSpace(h.cfg.GoofishGoodsNoPrefix),
		DefaultPriceCents:  h.cfg.GoofishDefaultPriceCents,
		DefaultInventory:   h.cfg.GoofishDefaultInventory,
		RecommendedPayload: "items[] => collection_id/goods_no/title/cover_url/zip_url/price_cents/inventory; sign=md5(app_id,app_secret,bodyMd5,timestamp,mch_id,mch_secret)",
	})
}

// AdminBatchPublishCollectionsToGoofish batches collection goods payload to your self-developed adapter.
// @Summary Batch publish collections to goofish adapter (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Param body body AdminGoofishBatchPublishRequest true "batch publish request"
// @Success 200 {object} AdminGoofishBatchPublishResponse
// @Router /api/admin/goofish/batch-publish [post]
func (h *Handler) AdminBatchPublishCollectionsToGoofish(c *gin.Context) {
	endpoint, enabled := h.getGoofishPublishEndpoint()
	if !enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "goofish adapter is not configured: set GOOFISH_PUBLISH_BASE_URL"})
		return
	}

	var req AdminGoofishBatchPublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.CollectionIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_ids is required"})
		return
	}
	if len(req.CollectionIDs) > goofishBatchPublishMax {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many collection_ids, max 500"})
		return
	}
	action, ok := normalizeGoofishPublishAction(req.Action)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action, supported: upsert/remove"})
		return
	}

	uniqueIDs := make([]uint64, 0, len(req.CollectionIDs))
	idSet := map[uint64]struct{}{}
	for _, id := range req.CollectionIDs {
		if id == 0 {
			continue
		}
		if _, exists := idSet[id]; exists {
			continue
		}
		idSet[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid collection_ids"})
		return
	}
	syncVisibility := true
	if req.SyncVisibility != nil {
		syncVisibility = *req.SyncVisibility
	}

	var collections []models.Collection
	if err := h.db.Where("id IN ?", uniqueIDs).Find(&collections).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query collections"})
		return
	}
	collectionMap := make(map[uint64]models.Collection, len(collections))
	categoryIDSet := map[uint64]struct{}{}
	for _, item := range collections {
		collectionMap[item.ID] = item
		if item.CategoryID != nil && *item.CategoryID > 0 {
			categoryIDSet[*item.CategoryID] = struct{}{}
		}
	}

	categoryIDs := make([]uint64, 0, len(categoryIDSet))
	for id := range categoryIDSet {
		categoryIDs = append(categoryIDs, id)
	}
	categoryNameMap := h.loadGoofishCategoryNameMap(categoryIDs)

	items := make([]goofishAdapterGoodItem, 0, len(collections))
	results := make([]GoofishBatchPublishResultItem, 0, len(uniqueIDs))
	orderingFoundIDs := make([]uint64, 0, len(collections))

	priceCents := h.cfg.GoofishDefaultPriceCents
	if priceCents <= 0 {
		priceCents = 199
	}
	if req.PriceCents != nil {
		if *req.PriceCents <= 0 || *req.PriceCents > 999999999 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "price_cents must be within 1..999999999"})
			return
		}
		priceCents = *req.PriceCents
	}
	inventory := h.cfg.GoofishDefaultInventory
	if inventory <= 0 {
		inventory = 9999
	}
	if req.Inventory != nil {
		if *req.Inventory < 0 || *req.Inventory > 999999999 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "inventory must be within 0..999999999"})
			return
		}
		inventory = *req.Inventory
	}

	for _, id := range uniqueIDs {
		item, exists := collectionMap[id]
		if !exists {
			results = append(results, GoofishBatchPublishResultItem{
				CollectionID: id,
				GoodsNo:      h.getGoofishGoodsNo(id),
				Success:      false,
				Message:      "合集不存在或已删除",
			})
			continue
		}
		orderingFoundIDs = append(orderingFoundIDs, id)
		categoryID := uint64(0)
		categoryName := ""
		if item.CategoryID != nil {
			categoryID = *item.CategoryID
			categoryName = categoryNameMap[categoryID]
		}
		items = append(items, goofishAdapterGoodItem{
			CollectionID: item.ID,
			GoodsNo:      h.getGoofishGoodsNo(item.ID),
			Title:        strings.TrimSpace(item.Title),
			Description:  strings.TrimSpace(item.Description),
			CategoryID:   categoryID,
			CategoryName: strings.TrimSpace(categoryName),
			Slug:         strings.TrimSpace(item.Slug),
			CoverURL:     resolvePreviewURL(item.CoverURL, h.qiniu),
			ZipKey:       strings.TrimSpace(item.LatestZipKey),
			ZipURL:       resolvePreviewURL(item.LatestZipKey, h.qiniu),
			FileCount:    item.FileCount,
			Visibility:   strings.TrimSpace(item.Visibility),
			Status:       strings.TrimSpace(item.Status),
			PriceCents:   priceCents,
			Inventory:    inventory,
			UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
		})
	}

	pushCount := len(items)
	if pushCount > 0 {
		payload := goofishAdapterBatchPublishRequest{
			Action:    action,
			Source:    "emoji_admin",
			Timestamp: time.Now().Format(time.RFC3339),
			Items:     items,
		}
		adapterStatusCode, adapterRaw, adapterResp, err := h.callGoofishAdapter(c.Request.Context(), endpoint, payload)
		if err != nil {
			for _, id := range orderingFoundIDs {
				results = append(results, GoofishBatchPublishResultItem{
					CollectionID: id,
					GoodsNo:      h.getGoofishGoodsNo(id),
					Success:      false,
					StatusCode:   adapterStatusCode,
					Message:      fmt.Sprintf("适配器调用失败：%s %s", err.Error(), summarizeRemoteBody(adapterRaw)),
				})
			}
		} else {
			perItemResults := make([]goofishAdapterResultItem, 0)
			if adapterResp != nil {
				if len(adapterResp.Results) > 0 {
					perItemResults = append(perItemResults, adapterResp.Results...)
				}
				if adapterResp.Data != nil && len(adapterResp.Data.Results) > 0 {
					perItemResults = append(perItemResults, adapterResp.Data.Results...)
				}
			}

			if len(perItemResults) == 0 {
				success := true
				message := ""
				if adapterResp != nil {
					success = boolPtrValue(adapterResp.Success, true)
					message = strings.TrimSpace(adapterResp.Message)
				}
				if !success {
					message = strings.TrimSpace(message)
					if message == "" {
						message = summarizeRemoteBody(adapterRaw)
					}
				}
				for _, id := range orderingFoundIDs {
					itemSuccess := success
					itemMessage := message
					if itemSuccess {
						itemMessage = "ok"
					}
					results = append(results, GoofishBatchPublishResultItem{
						CollectionID: id,
						GoodsNo:      h.getGoofishGoodsNo(id),
						Success:      itemSuccess,
						StatusCode:   adapterStatusCode,
						Message:      itemMessage,
					})
				}
			} else {
				resultByID := mapGoofishResultByCollectionID(perItemResults)
				for _, id := range orderingFoundIDs {
					result, exists := resultByID[id]
					if !exists {
						results = append(results, GoofishBatchPublishResultItem{
							CollectionID: id,
							GoodsNo:      h.getGoofishGoodsNo(id),
							Success:      true,
							StatusCode:   adapterStatusCode,
							Message:      "ok",
						})
						continue
					}
					success := boolPtrValue(result.Success, true)
					message := strings.TrimSpace(result.Message)
					if message == "" {
						if success {
							message = "ok"
						} else {
							message = "adapter result failed"
						}
					}
					statusCode := result.StatusCode
					if statusCode == 0 {
						statusCode = adapterStatusCode
					}
					goodsNo := strings.TrimSpace(result.GoodsNo)
					if goodsNo == "" {
						goodsNo = h.getGoofishGoodsNo(id)
					}
					results = append(results, GoofishBatchPublishResultItem{
						CollectionID: id,
						GoodsNo:      goodsNo,
						Success:      success,
						StatusCode:   statusCode,
						Message:      message,
					})
				}
			}
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		return results[i].CollectionID < results[j].CollectionID
	})
	successIDs := make([]uint64, 0, len(results))
	successCount := 0
	for _, item := range results {
		if item.Success {
			successCount += 1
			if item.CollectionID > 0 {
				successIDs = append(successIDs, item.CollectionID)
			}
		}
	}
	failureCount := len(results) - successCount

	visibilityValue := ""
	syncedVisibilityCount := int64(0)
	if syncVisibility && len(successIDs) > 0 {
		if action == "remove" {
			visibilityValue = "private"
		} else {
			visibilityValue = "public"
		}
		update := h.db.Model(&models.Collection{}).Where("id IN ?", successIDs).Update("visibility", visibilityValue)
		if update.Error == nil {
			syncedVisibilityCount = update.RowsAffected
		}
	}

	c.JSON(http.StatusOK, AdminGoofishBatchPublishResponse{
		Action:              action,
		RequestedCount:      len(uniqueIDs),
		PushedCount:         pushCount,
		SuccessCount:        successCount,
		FailureCount:        failureCount,
		PriceCents:          priceCents,
		Inventory:           inventory,
		SyncVisibility:      syncVisibility,
		SyncedVisibility:    visibilityValue,
		SyncedVisibilityCnt: int(syncedVisibilityCount),
		AdapterEndpoint:     endpoint,
		Results:             results,
	})
}
