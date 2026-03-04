package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type CategoryStatsResponse struct {
	CategoryID      uint64 `json:"category_id"`
	CollectionCount int64  `json:"collection_count"`
}

// ListCategoryStats godoc
// @Summary List category stats (collection counts)
// @Tags admin
// @Produce json
// @Success 200 {array} CategoryStatsResponse
// @Router /api/admin/categories/stats [get]
func (h *Handler) ListCategoryStats(c *gin.Context) {
	var rows []CategoryStatsResponse
	if err := h.db.Table("archive.collections").
		Select("category_id, COUNT(*) as collection_count").
		Where("category_id IS NOT NULL AND deleted_at IS NULL").
		Group("category_id").
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}
