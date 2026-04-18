package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	goofishCodeSuccess          = 0
	goofishCodeInvalidParams    = 400
	goofishCodeInvalidSign      = 401
	goofishCodeForbidden        = 403
	goofishCodeNotFound         = 404
	goofishCodeTimestampExpired = 408
	goofishCodeInternal         = 500
	goofishSignWindowSeconds    = 300
	goofishListDefaultPageSize  = 20
	goofishListMaxPageSize      = 100
)

type goofishGoodsListRequest struct {
	Keyword   string `json:"keyword"`
	GoodsType *int   `json:"goods_type"`
	PageNo    int    `json:"page_no"`
	PageSize  int    `json:"page_size"`
}

type goofishGoodsDetailRequest struct {
	GoodsType int    `json:"goods_type"`
	GoodsNo   string `json:"goods_no"`
}

type goofishOpenInfoData struct {
	AppID interface{} `json:"app_id"`
}

type goofishUserInfoData struct {
	Balance int64 `json:"balance"`
}

type goofishGoodsListData struct {
	List  []goofishGoodsListItem `json:"list"`
	Count int64                  `json:"count"`
}

type goofishGoodsListItem struct {
	GoodsNo    string `json:"goods_no"`
	GoodsType  int    `json:"goods_type"`
	GoodsName  string `json:"goods_name"`
	Price      int64  `json:"price"`
	Stock      int    `json:"stock"`
	Status     int    `json:"status"`
	UpdateTime int64  `json:"update_time"`
}

type goofishGoodsDetailData struct {
	GoodsNo    string          `json:"goods_no"`
	GoodsType  int             `json:"goods_type"`
	GoodsName  string          `json:"goods_name"`
	Price      int64           `json:"price"`
	Stock      int             `json:"stock"`
	Status     int             `json:"status"`
	UpdateTime int64           `json:"update_time"`
	Template   json.RawMessage `json:"template"`
}

func writeGoofishResponse(c *gin.Context, code int, msg string, data interface{}) {
	payload := gin.H{
		"code": code,
		"msg":  strings.TrimSpace(msg),
	}
	if payload["msg"] == "" {
		payload["msg"] = "ok"
	}
	if data != nil {
		payload["data"] = data
	}
	c.JSON(http.StatusOK, payload)
}

func normalizeGoofishStatus(raw int) int {
	if raw == adminCollectionGoodStatusOnShelf {
		return adminCollectionGoodStatusOnShelf
	}
	return adminCollectionGoodStatusOffShelf
}

func buildGoofishListItem(row models.CollectionGood) goofishGoodsListItem {
	return goofishGoodsListItem{
		GoodsNo:    strings.TrimSpace(row.GoodsNo),
		GoodsType:  row.GoodsType,
		GoodsName:  strings.TrimSpace(row.GoodsName),
		Price:      row.Price,
		Stock:      row.Stock,
		Status:     normalizeGoofishStatus(row.Status),
		UpdateTime: row.UpdatedAt.Unix(),
	}
}

func buildGoofishDetailData(row models.CollectionGood) goofishGoodsDetailData {
	template := row.TemplateJSON
	if len(template) == 0 {
		template = datatypes.JSON([]byte("[]"))
	}
	return goofishGoodsDetailData{
		GoodsNo:    strings.TrimSpace(row.GoodsNo),
		GoodsType:  row.GoodsType,
		GoodsName:  strings.TrimSpace(row.GoodsName),
		Price:      row.Price,
		Stock:      row.Stock,
		Status:     normalizeGoofishStatus(row.Status),
		UpdateTime: row.UpdatedAt.Unix(),
		Template:   json.RawMessage(template),
	}
}

func parseGoofishJSONBody(c *gin.Context, dst interface{}) ([]byte, error) {
	raw, err := c.GetRawData()
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		text = "{}"
	}
	if err := json.Unmarshal([]byte(text), dst); err != nil {
		return raw, err
	}
	return raw, nil
}

func (h *Handler) resolveGoofishOpenInfoAppID(c *gin.Context) interface{} {
	candidates := []string{
		strings.TrimSpace(c.Query("app_id")),
		strings.TrimSpace(h.cfg.GoofishSignAppID),
		strings.TrimSpace(c.Query("mch_id")),
		strings.TrimSpace(h.cfg.GoofishSignMchID),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if num, err := strconv.ParseInt(candidate, 10, 64); err == nil && num > 0 {
			return num
		}
		return candidate
	}
	return int64(0)
}

func (h *Handler) verifyGoofishRequestSignature(c *gin.Context, rawBody []byte) (int, string, bool) {
	if !h.cfg.GoofishSignEnabled {
		return 0, "", true
	}
	if !h.hasGoofishSignCredentials() {
		return goofishCodeInternal, "goofish sign credentials are not configured", false
	}

	mchID := strings.TrimSpace(c.Query("mch_id"))
	timestampRaw := strings.TrimSpace(c.Query("timestamp"))
	sign := strings.TrimSpace(c.Query("sign"))
	queryAppID := strings.TrimSpace(c.Query("app_id"))
	if mchID == "" || timestampRaw == "" || sign == "" {
		return goofishCodeInvalidSign, "missing sign params: mch_id/timestamp/sign", false
	}

	expectedMchID := strings.TrimSpace(h.cfg.GoofishSignMchID)
	if expectedMchID != "" && mchID != expectedMchID {
		return goofishCodeForbidden, "invalid mch_id", false
	}
	expectedAppID := strings.TrimSpace(h.cfg.GoofishSignAppID)
	if queryAppID != "" && expectedAppID != "" && queryAppID != expectedAppID {
		return goofishCodeForbidden, "invalid app_id", false
	}

	timestamp, err := strconv.ParseInt(timestampRaw, 10, 64)
	if err != nil {
		return goofishCodeInvalidSign, "invalid timestamp", false
	}
	now := time.Now().Unix()
	if timestamp < now-goofishSignWindowSeconds || timestamp > now+goofishSignWindowSeconds {
		return goofishCodeTimestampExpired, "timestamp expired", false
	}

	bodyMD5 := md5Hex(string(rawBody))
	signRaw := fmt.Sprintf(
		"%s,%s,%s,%d,%s,%s",
		expectedAppID,
		strings.TrimSpace(h.cfg.GoofishSignAppSecret),
		bodyMD5,
		timestamp,
		mchID,
		strings.TrimSpace(h.cfg.GoofishSignMchSecret),
	)
	expectedSign := md5Hex(signRaw)
	if !strings.EqualFold(sign, expectedSign) {
		return goofishCodeInvalidSign, "invalid sign", false
	}
	return 0, "", true
}

// GoofishOpenInfo 查询虚拟货源应用信息（供闲管家授权校验）。
// @Summary Goofish open info
// @Tags goofish
// @Accept json
// @Produce json
// @Param mch_id query string false "merchant id"
// @Param timestamp query string false "unix timestamp"
// @Param sign query string false "md5 sign"
// @Success 200 {object} map[string]interface{}
// @Router /goofish/open/info [post]
func (h *Handler) GoofishOpenInfo(c *gin.Context) {
	var req map[string]interface{}
	rawBody, err := parseGoofishJSONBody(c, &req)
	if err != nil {
		writeGoofishResponse(c, goofishCodeInvalidParams, "invalid json body", nil)
		return
	}
	if code, msg, ok := h.verifyGoofishRequestSignature(c, rawBody); !ok {
		writeGoofishResponse(c, code, msg, nil)
		return
	}
	writeGoofishResponse(c, goofishCodeSuccess, "ok", goofishOpenInfoData{
		AppID: h.resolveGoofishOpenInfoAppID(c),
	})
}

// GoofishUserInfo 查询虚拟货源账户余额（供闲管家授权校验）。
// @Summary Goofish user info
// @Tags goofish
// @Accept json
// @Produce json
// @Param mch_id query string false "merchant id"
// @Param timestamp query string false "unix timestamp"
// @Param sign query string false "md5 sign"
// @Success 200 {object} map[string]interface{}
// @Router /goofish/user/info [post]
func (h *Handler) GoofishUserInfo(c *gin.Context) {
	var req map[string]interface{}
	rawBody, err := parseGoofishJSONBody(c, &req)
	if err != nil {
		writeGoofishResponse(c, goofishCodeInvalidParams, "invalid json body", nil)
		return
	}
	if code, msg, ok := h.verifyGoofishRequestSignature(c, rawBody); !ok {
		writeGoofishResponse(c, code, msg, nil)
		return
	}
	writeGoofishResponse(c, goofishCodeSuccess, "ok", goofishUserInfoData{
		Balance: 0,
	})
}

// GoofishGoodsList 查询商品列表（供闲管家调用）。
// @Summary Goofish goods list
// @Tags goofish
// @Accept json
// @Produce json
// @Param mch_id query string false "merchant id"
// @Param timestamp query string false "unix timestamp"
// @Param sign query string false "md5 sign"
// @Param body body goofishGoodsListRequest false "list request"
// @Success 200 {object} map[string]interface{}
// @Router /goofish/goods/list [post]
func (h *Handler) GoofishGoodsList(c *gin.Context) {
	var req goofishGoodsListRequest
	rawBody, err := parseGoofishJSONBody(c, &req)
	if err != nil {
		writeGoofishResponse(c, goofishCodeInvalidParams, "invalid json body", nil)
		return
	}

	if code, msg, ok := h.verifyGoofishRequestSignature(c, rawBody); !ok {
		writeGoofishResponse(c, code, msg, nil)
		return
	}

	pageNo := req.PageNo
	if pageNo <= 0 {
		pageNo = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = goofishListDefaultPageSize
	}
	if pageSize > goofishListMaxPageSize {
		pageSize = goofishListMaxPageSize
	}

	query := h.scopeOperationCollectionGoods(h.db.Model(&models.CollectionGood{}))
	if keyword := strings.TrimSpace(req.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("goods_no ILIKE ? OR goods_name ILIKE ?", like, like)
	}
	if req.GoodsType != nil {
		goodsType, ok := normalizeCollectionGoodType(*req.GoodsType)
		if !ok {
			writeGoofishResponse(c, goofishCodeInvalidParams, "goods_type must be 1/2/3", nil)
			return
		}
		query = query.Where("goods_type = ?", goodsType)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeGoofishResponse(c, goofishCodeInternal, "failed to query goods count", nil)
		return
	}

	var rows []models.CollectionGood
	if err := query.
		Order("updated_at DESC, id DESC").
		Offset((pageNo - 1) * pageSize).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		writeGoofishResponse(c, goofishCodeInternal, "failed to query goods list", nil)
		return
	}

	list := make([]goofishGoodsListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, buildGoofishListItem(row))
	}
	writeGoofishResponse(c, goofishCodeSuccess, "ok", goofishGoodsListData{
		List:  list,
		Count: total,
	})
}

// GoofishGoodsDetail 查询商品详情（供闲管家调用）。
// @Summary Goofish goods detail
// @Tags goofish
// @Accept json
// @Produce json
// @Param mch_id query string false "merchant id"
// @Param timestamp query string false "unix timestamp"
// @Param sign query string false "md5 sign"
// @Param body body goofishGoodsDetailRequest true "detail request"
// @Success 200 {object} map[string]interface{}
// @Router /goofish/goods/detail [post]
func (h *Handler) GoofishGoodsDetail(c *gin.Context) {
	var req goofishGoodsDetailRequest
	rawBody, err := parseGoofishJSONBody(c, &req)
	if err != nil {
		writeGoofishResponse(c, goofishCodeInvalidParams, "invalid json body", nil)
		return
	}

	if code, msg, ok := h.verifyGoofishRequestSignature(c, rawBody); !ok {
		writeGoofishResponse(c, code, msg, nil)
		return
	}

	goodsNo := strings.TrimSpace(req.GoodsNo)
	if goodsNo == "" {
		writeGoofishResponse(c, goofishCodeInvalidParams, "goods_no is required", nil)
		return
	}
	goodsType, ok := normalizeCollectionGoodType(req.GoodsType)
	if !ok {
		writeGoofishResponse(c, goofishCodeInvalidParams, "goods_type must be 1/2/3", nil)
		return
	}

	var row models.CollectionGood
	if err := h.scopeOperationCollectionGoods(h.db.Model(&models.CollectionGood{})).
		Where("goods_no = ? AND goods_type = ?", goodsNo, goodsType).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeGoofishResponse(c, goofishCodeNotFound, "goods not found", nil)
			return
		}
		writeGoofishResponse(c, goofishCodeInternal, "failed to query goods detail", nil)
		return
	}

	writeGoofishResponse(c, goofishCodeSuccess, "ok", buildGoofishDetailData(row))
}
