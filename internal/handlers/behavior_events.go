package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type TrackUserBehaviorEventRequest struct {
	EventName          string                 `json:"event_name"`
	Route              string                 `json:"route"`
	Referrer           string                 `json:"referrer"`
	CollectionID       *uint64                `json:"collection_id"`
	EmojiID            *uint64                `json:"emoji_id"`
	IPID               *uint64                `json:"ip_id"`
	SubscriptionStatus string                 `json:"subscription_status"`
	Success            *bool                  `json:"success"`
	ErrorCode          string                 `json:"error_code"`
	RequestID          string                 `json:"request_id"`
	SessionID          string                 `json:"session_id"`
	DeviceID           string                 `json:"device_id"`
	Metadata           map[string]interface{} `json:"metadata"`
}

type userBehaviorEventOptions struct {
	UserID             uint64
	DeviceID           string
	SessionID          string
	Route              string
	Referrer           string
	CollectionID       *uint64
	EmojiID            *uint64
	IPID               *uint64
	SubscriptionStatus string
	Success            *bool
	ErrorCode          string
	RequestID          string
	Metadata           map[string]interface{}
}

func normalizeUserBehaviorEventName(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || len(value) > 64 {
		return ""
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-' {
			continue
		}
		return ""
	}
	return value
}

func trimToMax(raw string, max int) string {
	value := strings.TrimSpace(raw)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func firstHeaderValue(c *gin.Context, keys ...string) string {
	if c == nil {
		return ""
	}
	for _, key := range keys {
		value := trimToMax(c.GetHeader(key), 128)
		if value != "" {
			return value
		}
	}
	return ""
}

func metaStringValue(meta map[string]interface{}, key string) string {
	if meta == nil {
		return ""
	}
	raw, ok := meta[key]
	if !ok || raw == nil {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func (h *Handler) recordUserBehaviorEvent(c *gin.Context, eventName string, options userBehaviorEventOptions) {
	if h == nil || h.db == nil {
		return
	}
	normalizedEventName := normalizeUserBehaviorEventName(eventName)
	if normalizedEventName == "" {
		return
	}

	route := trimToMax(options.Route, 512)
	if route == "" && c != nil && c.Request != nil && c.Request.URL != nil {
		route = trimToMax(c.Request.URL.Path, 512)
	}
	referrer := trimToMax(options.Referrer, 512)
	if referrer == "" && c != nil {
		referrer = trimToMax(c.GetHeader("Referer"), 512)
	}
	deviceID := trimToMax(options.DeviceID, 128)
	if deviceID == "" && c != nil {
		deviceID = trimToMax(c.GetHeader("X-Device-ID"), 128)
	}
	sessionID := trimToMax(options.SessionID, 128)
	if sessionID == "" && c != nil {
		sessionID = trimToMax(c.GetHeader("X-Session-ID"), 128)
	}
	requestID := trimToMax(options.RequestID, 128)
	if requestID == "" && c != nil {
		requestID = trimToMax(c.GetHeader("X-Request-ID"), 128)
	}

	var userID *uint64
	if options.UserID > 0 {
		uid := options.UserID
		userID = &uid
	} else if c != nil {
		if uid, ok := currentUserIDFromContext(c); ok && uid > 0 {
			userID = &uid
		}
	}

	metaMap := make(map[string]interface{}, 8)
	for k, v := range options.Metadata {
		metaMap[k] = v
	}
	if c != nil {
		ipValue := ""
		if _, exists := metaMap["request_ip"]; !exists {
			if ip := trimToMax(c.ClientIP(), 64); ip != "" {
				metaMap["request_ip"] = ip
				ipValue = ip
			}
		}
		if ipValue == "" {
			ipValue = metaStringValue(metaMap, "request_ip")
		}
		if _, exists := metaMap["user_agent"]; !exists {
			if ua := trimToMax(c.GetHeader("User-Agent"), 512); ua != "" {
				metaMap["user_agent"] = ua
			}
		}
		if _, exists := metaMap["country"]; !exists {
			if country := firstHeaderValue(
				c,
				"X-Geo-Country",
				"X-Country",
				"CF-IPCountry",
				"X-AppEngine-Country",
				"CloudFront-Viewer-Country",
			); country != "" {
				metaMap["country"] = country
			}
		}
		if _, exists := metaMap["region"]; !exists {
			if region := firstHeaderValue(
				c,
				"X-Geo-Region",
				"X-Region",
				"X-AppEngine-Region",
				"CloudFront-Viewer-Country-Region",
			); region != "" {
				metaMap["region"] = region
			}
		}
		if _, exists := metaMap["city"]; !exists {
			if city := firstHeaderValue(
				c,
				"X-Geo-City",
				"X-City",
				"X-AppEngine-City",
				"CloudFront-Viewer-City",
			); city != "" {
				metaMap["city"] = city
			}
		}

		missingCountry := metaStringValue(metaMap, "country") == ""
		missingRegion := metaStringValue(metaMap, "region") == ""
		missingCity := metaStringValue(metaMap, "city") == ""
		if (missingCountry || missingRegion || missingCity) && ipValue != "" {
			geo := lookupGeoByIP(h.cfg, ipValue)
			if missingCountry && geo.Country != "" {
				metaMap["country"] = geo.Country
			}
			if missingRegion && geo.Region != "" {
				metaMap["region"] = geo.Region
			}
			if missingCity && geo.City != "" {
				metaMap["city"] = geo.City
			}
			if geo.Source != "" {
				if _, exists := metaMap["geo_source"]; !exists {
					metaMap["geo_source"] = geo.Source
				}
			}
		}
	}

	meta := datatypes.JSON([]byte("{}"))
	if len(metaMap) > 0 {
		if raw, err := json.Marshal(metaMap); err == nil {
			meta = datatypes.JSON(raw)
		}
	}

	event := models.UserBehaviorEvent{
		UserID:             userID,
		DeviceID:           deviceID,
		SessionID:          sessionID,
		EventName:          normalizedEventName,
		Route:              route,
		Referrer:           referrer,
		CollectionID:       options.CollectionID,
		EmojiID:            options.EmojiID,
		IPID:               options.IPID,
		SubscriptionStatus: trimToMax(options.SubscriptionStatus, 32),
		Success:            options.Success,
		ErrorCode:          trimToMax(options.ErrorCode, 64),
		RequestID:          requestID,
		Metadata:           meta,
	}
	_ = h.db.Create(&event).Error
}

func (h *Handler) TrackUserBehaviorEvent(c *gin.Context) {
	var req TrackUserBehaviorEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	eventName := normalizeUserBehaviorEventName(req.EventName)
	if eventName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event_name"})
		return
	}

	h.recordUserBehaviorEvent(c, eventName, userBehaviorEventOptions{
		DeviceID:           req.DeviceID,
		SessionID:          req.SessionID,
		Route:              req.Route,
		Referrer:           req.Referrer,
		CollectionID:       req.CollectionID,
		EmojiID:            req.EmojiID,
		IPID:               req.IPID,
		SubscriptionStatus: req.SubscriptionStatus,
		Success:            req.Success,
		ErrorCode:          req.ErrorCode,
		RequestID:          req.RequestID,
		Metadata:           req.Metadata,
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
