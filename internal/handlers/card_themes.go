package handlers

import (
	"net/http"
	"emoji/internal/models"
	"github.com/gin-gonic/gin"
)

type CardThemeResponse struct {
	ID       uint64      `json:"id"`
	Name     string      `json:"name"`
	Slug     string      `json:"slug"`
	Config   interface{} `json:"config"`
	IsSystem bool        `json:"is_system"`
}

func (h *Handler) ListCardThemes(c *gin.Context) {
	var themes []models.CardTheme
	if err := h.db.Order("id ASC").Find(&themes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := make([]CardThemeResponse, 0, len(themes))
	for _, t := range themes {
		resp = append(resp, CardThemeResponse{
			ID:       t.ID,
			Name:     t.Name,
			Slug:     t.Slug,
			Config:   t.Config,
			IsSystem: t.IsSystem,
		})
	}

	c.JSON(http.StatusOK, resp)
}
