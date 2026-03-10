package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"emoji/internal/models"
	"emoji/pkg/oss"

	"github.com/gin-gonic/gin"
)

// UploadTemplate handles multipart form upload of a meme template.
// POST /api/v1/admin/templates
func (h *Handler) UploadTemplate(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	category := strings.TrimSpace(c.PostForm("category"))
	tags := strings.TrimSpace(c.PostForm("tags"))
	textX, _ := strconv.Atoi(c.DefaultPostForm("text_x", "0"))
	textY, _ := strconv.Atoi(c.DefaultPostForm("text_y", "0"))
	textW, _ := strconv.Atoi(c.DefaultPostForm("text_width", "0"))
	textH, _ := strconv.Atoi(c.DefaultPostForm("text_height", "0"))
	textColor := c.DefaultPostForm("text_color", "#FFFFFF")
	fontSize, _ := strconv.Atoi(c.DefaultPostForm("font_size", "32"))

	file, _, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image file required"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "read file failed"})
		return
	}

	// Save to DB first to get ID
	tmpl := models.MemeTemplate{
		Name:       name,
		Category:   category,
		Tags:       tags,
		TextX:      textX,
		TextY:      textY,
		TextWidth:  textW,
		TextHeight: textH,
		TextColor:  textColor,
		FontSize:   fontSize,
		Status:     "active",
	}
	if err := h.db.Create(&tmpl).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}

	// Upload to OSS
	if h.ossClient != nil {
		objectKey := oss.TemplateKey(tmpl.ID)
		url, err := h.ossClient.Upload(objectKey, data)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed: " + err.Error()})
			return
		}
		tmpl.ImageURL = url
		h.db.Model(&tmpl).Update("image_url", url)
	} else {
		// Fallback: store via qiniu if OSS not configured
		tmpl.ImageURL = fmt.Sprintf("templates/%d.png", tmpl.ID)
		h.db.Model(&tmpl).Update("image_url", tmpl.ImageURL)
	}

	c.JSON(http.StatusCreated, tmpl)
}

// PLACEHOLDER_ADMIN_1

// ListTemplates returns all meme templates.
// GET /api/v1/admin/templates
func (h *Handler) ListMemeTemplates(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	var total int64
	h.db.Model(&models.MemeTemplate{}).Count(&total)

	var templates []models.MemeTemplate
	h.db.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&templates)

	c.JSON(http.StatusOK, gin.H{"items": templates, "total": total})
}

type AddPhraseRequest struct {
	Phrase     string  `json:"phrase" binding:"required"`
	Category   string  `json:"category"`
	Emotion    string  `json:"emotion"`
	Context    string  `json:"context"`
	HotScore   int     `json:"hot_score"`
	TemplateID *uint64 `json:"template_id"`
}

// AddPhrase adds a meme phrase to the library.
// POST /api/v1/admin/phrases
func (h *Handler) AddPhrase(c *gin.Context) {
	var req AddPhraseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	phrase := models.MemePhrase{
		Phrase:     strings.TrimSpace(req.Phrase),
		Category:   strings.TrimSpace(req.Category),
		Emotion:    strings.TrimSpace(req.Emotion),
		Context:    strings.TrimSpace(req.Context),
		HotScore:   req.HotScore,
		TemplateID: req.TemplateID,
		Status:     "active",
	}
	if err := h.db.Create(&phrase).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}

	c.JSON(http.StatusCreated, phrase)
}

// ListPhrases returns all meme phrases.
// GET /api/v1/admin/phrases
func (h *Handler) ListPhrases(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	var total int64
	h.db.Model(&models.MemePhrase{}).Count(&total)

	var phrases []models.MemePhrase
	h.db.Order("hot_score DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&phrases)

	c.JSON(http.StatusOK, gin.H{"items": phrases, "total": total})
}
