package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type AdminAnalyticsGeoBackfillRequest struct {
	Days  int `json:"days"`
	Limit int `json:"limit"`
}

type AdminAnalyticsGeoBackfillResponse struct {
	Days      int    `json:"days"`
	Limit     int    `json:"limit"`
	Scanned   int    `json:"scanned"`
	Updated   int    `json:"updated"`
	Skipped   int    `json:"skipped"`
	Failed    int    `json:"failed"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at"`
}

// BackfillAdminAnalyticsGeo godoc
// @Summary Backfill country/region/city for behavior events by request_ip
// @Tags admin
// @Accept json
// @Produce json
// @Success 200 {object} AdminAnalyticsGeoBackfillResponse
// @Router /api/admin/analytics/geo-backfill [post]
func (h *Handler) BackfillAdminAnalyticsGeo(c *gin.Context) {
	if !h.cfg.GeoIPEnabled || strings.TrimSpace(h.cfg.GeoIPMMDBPath) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "geoip not configured (set GEOIP_ENABLED and GEOIP_MMDB_PATH)"})
		return
	}

	var req AdminAnalyticsGeoBackfillRequest
	_ = c.ShouldBindJSON(&req)

	days := req.Days
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 2000
	}
	if limit > 20000 {
		limit = 20000
	}

	startedAt := time.Now()
	startTime := startedAt.AddDate(0, 0, -(days - 1))

	type row struct {
		ID       uint64         `gorm:"column:id"`
		Metadata datatypes.JSON `gorm:"column:metadata"`
	}
	var rows []row
	if err := h.db.Raw(`
		SELECT id, metadata
		FROM action.user_behavior_events
		WHERE created_at >= ?
		  AND COALESCE(metadata->>'request_ip', '') <> ''
		  AND (
			COALESCE(metadata->>'country', '') = ''
			OR COALESCE(metadata->>'region', '') = ''
			OR COALESCE(metadata->>'city', '') = ''
		  )
		ORDER BY id ASC
		LIMIT ?
	`, startTime, limit).Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated := 0
	skipped := 0
	failed := 0

	for _, item := range rows {
		meta := map[string]interface{}{}
		if len(item.Metadata) > 0 {
			if err := json.Unmarshal(item.Metadata, &meta); err != nil {
				failed += 1
				continue
			}
		}
		requestIP := strings.TrimSpace(metaStringInterface(meta["request_ip"]))
		if requestIP == "" {
			skipped += 1
			continue
		}

		country := strings.TrimSpace(metaStringInterface(meta["country"]))
		region := strings.TrimSpace(metaStringInterface(meta["region"]))
		city := strings.TrimSpace(metaStringInterface(meta["city"]))
		if country != "" && region != "" && city != "" {
			skipped += 1
			continue
		}

		geo := lookupGeoByIP(h.cfg, requestIP)
		if geo.Country == "" && geo.Region == "" && geo.City == "" {
			skipped += 1
			continue
		}

		changed := false
		if country == "" && geo.Country != "" {
			meta["country"] = geo.Country
			changed = true
		}
		if region == "" && geo.Region != "" {
			meta["region"] = geo.Region
			changed = true
		}
		if city == "" && geo.City != "" {
			meta["city"] = geo.City
			changed = true
		}
		if changed {
			if _, exists := meta["geo_source"]; !exists && geo.Source != "" {
				meta["geo_source"] = geo.Source
			}
		}
		if !changed {
			skipped += 1
			continue
		}

		raw, err := json.Marshal(meta)
		if err != nil {
			failed += 1
			continue
		}
		if err := h.db.Table("action.user_behavior_events").
			Where("id = ?", item.ID).
			Update("metadata", datatypes.JSON(raw)).Error; err != nil {
			failed += 1
			continue
		}
		updated += 1
	}

	endedAt := time.Now()
	c.JSON(http.StatusOK, AdminAnalyticsGeoBackfillResponse{
		Days:      days,
		Limit:     limit,
		Scanned:   len(rows),
		Updated:   updated,
		Skipped:   skipped,
		Failed:    failed,
		StartedAt: startedAt.Format(time.RFC3339),
		EndedAt:   endedAt.Format(time.RFC3339),
	})
}

func metaStringInterface(raw interface{}) string {
	if raw == nil {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
