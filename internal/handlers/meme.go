package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type GenerateMemeRequest struct {
	Text string `json:"text" binding:"required"`
}

type MemeResponse struct {
	ID           uint64 `json:"id"`
	CreatorID    uint64 `json:"creator_id"`
	InputText    string `json:"input_text"`
	MemeText     string `json:"meme_text"`
	ImageURL     string `json:"image_url"`
	LikeCount    int    `json:"like_count"`
	CollectCount int    `json:"collect_count"`
	IsPublic     bool   `json:"is_public"`
	Liked        bool   `json:"liked"`
	Collected    bool   `json:"collected"`
	CreatedAt    string `json:"created_at"`
}

// GenerateMeme creates a meme from user text input.
// POST /api/v1/memes/generate
func (h *Handler) GenerateMeme(c *gin.Context) {
	uid, ok := h.requireAuth(c)
	if !ok {
		return
	}

	var req GenerateMemeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text is required"})
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text is required"})
		return
	}

	if h.ai == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI service not available"})
		return
	}

	// Step 1: AI matches a phrase
	match, err := h.ai.MatchPhrase(c.Request.Context(), text)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI matching failed: " + err.Error()})
		return
	}

	// Step 2: Find a template
	var tmpl models.MemeTemplate
	// Try phrase's recommended template first
	if match.PhraseID > 0 {
		var phrase models.MemePhrase
		if err := h.db.First(&phrase, match.PhraseID).Error; err == nil && phrase.TemplateID != nil {
			h.db.First(&tmpl, *phrase.TemplateID)
		}
	}
	// Fallback: find by category
	if tmpl.ID == 0 && match.TemplateCategory != "" {
		h.db.Where("category = ? AND status = ?", match.TemplateCategory, "active").Order("RANDOM()").First(&tmpl)
	}
	// Fallback: any active template
	if tmpl.ID == 0 {
		h.db.Where("status = ?", "active").Order("RANDOM()").First(&tmpl)
	}
	if tmpl.ID == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "no template available"})
		return
	}

	// Step 3: Compose image
	if h.compose == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "compose service not available"})
		return
	}
	imageURL, err := h.compose.Compose(tmpl, match.Phrase, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "image compose failed: " + err.Error()})
		return
	}

	// Step 4: Save to DB
	meme := models.Meme{
		CreatorID:  uid,
		InputText:  text,
		MemeText:   match.Phrase,
		TemplateID: &tmpl.ID,
		ImageURL:   imageURL,
		IsPublic:   true,
	}
	if err := h.db.Create(&meme).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"meme":      mapMeme(meme, false, false),
		"meme_text": match.Phrase,
	})
}

// PLACEHOLDER_MEME_1

// FeedMemes returns paginated public memes.
// GET /api/v1/memes/feed?page=1&size=20&sort=latest|hot
func (h *Handler) FeedMemes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	sort := c.DefaultQuery("sort", "latest")
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}

	query := h.db.Model(&models.Meme{}).Where("is_public = ?", true)
	var total int64
	query.Count(&total)

	orderBy := "created_at DESC"
	if sort == "hot" {
		orderBy = "like_count DESC, created_at DESC"
	}

	var memes []models.Meme
	query.Order(orderBy).Offset((page - 1) * size).Limit(size).Find(&memes)

	// Check liked/collected status for current user
	uid, _ := c.Get("user_id")
	userID, _ := uid.(uint64)

	items := make([]MemeResponse, 0, len(memes))
	for _, m := range memes {
		liked, collected := false, false
		if userID > 0 {
			liked = h.isMemeLiked(userID, m.ID)
			collected = h.isMemeCollected(userID, m.ID)
		}
		items = append(items, mapMeme(m, liked, collected))
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// ToggleMemeLike toggles like on a meme.
// POST /api/v1/memes/:id/like
func (h *Handler) ToggleMemeLike(c *gin.Context) {
	uid, ok := h.requireAuth(c)
	if !ok {
		return
	}
	memeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var meme models.Meme
	if err := h.db.First(&meme, memeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "meme not found"})
		return
	}

	var existing models.UserLike
	err = h.db.Where("user_id = ? AND meme_id = ?", uid, memeID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		// Add like
		h.db.Create(&models.UserLike{UserID: uid, MemeID: memeID})
		h.db.Model(&meme).Update("like_count", gorm.Expr("like_count + 1"))
		c.JSON(http.StatusOK, gin.H{"liked": true, "like_count": meme.LikeCount + 1})
	} else if err == nil {
		// Remove like
		h.db.Where("user_id = ? AND meme_id = ?", uid, memeID).Delete(&models.UserLike{})
		h.db.Model(&meme).Update("like_count", gorm.Expr("GREATEST(like_count - 1, 0)"))
		count := meme.LikeCount - 1
		if count < 0 {
			count = 0
		}
		c.JSON(http.StatusOK, gin.H{"liked": false, "like_count": count})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
	}
}

// PLACEHOLDER_MEME_2

// ToggleMemeCollect toggles collect on a meme.
// POST /api/v1/memes/:id/collect
func (h *Handler) ToggleMemeCollect(c *gin.Context) {
	uid, ok := h.requireAuth(c)
	if !ok {
		return
	}
	memeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var meme models.Meme
	if err := h.db.First(&meme, memeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "meme not found"})
		return
	}

	var existing models.UserCollect
	err = h.db.Where("user_id = ? AND meme_id = ?", uid, memeID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		h.db.Create(&models.UserCollect{UserID: uid, MemeID: memeID})
		h.db.Model(&meme).Update("collect_count", gorm.Expr("collect_count + 1"))
		c.JSON(http.StatusOK, gin.H{"collected": true, "collect_count": meme.CollectCount + 1})
	} else if err == nil {
		h.db.Where("user_id = ? AND meme_id = ?", uid, memeID).Delete(&models.UserCollect{})
		h.db.Model(&meme).Update("collect_count", gorm.Expr("GREATEST(collect_count - 1, 0)"))
		count := meme.CollectCount - 1
		if count < 0 {
			count = 0
		}
		c.JSON(http.StatusOK, gin.H{"collected": false, "collect_count": count})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
	}
}

// MyMemes returns memes created by the current user.
// GET /api/v1/users/me/memes?page=1&size=20
func (h *Handler) MyMemes(c *gin.Context) {
	uid, ok := h.requireAuth(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}

	query := h.db.Model(&models.Meme{}).Where("creator_id = ?", uid)
	var total int64
	query.Count(&total)

	var memes []models.Meme
	query.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&memes)

	items := make([]MemeResponse, 0, len(memes))
	for _, m := range memes {
		items = append(items, mapMeme(m, h.isMemeLiked(uid, m.ID), h.isMemeCollected(uid, m.ID)))
	}

	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// MyCollections returns memes collected by the current user.
// GET /api/v1/users/me/collections?page=1&size=20
func (h *Handler) MyMemeCollections(c *gin.Context) {
	uid, ok := h.requireAuth(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}

	var total int64
	h.db.Model(&models.UserCollect{}).Where("user_id = ?", uid).Count(&total)

	var collects []models.UserCollect
	h.db.Where("user_id = ?", uid).Order("created_at DESC").Offset((page-1)*size).Limit(size).Find(&collects)

	memeIDs := make([]uint64, 0, len(collects))
	for _, col := range collects {
		memeIDs = append(memeIDs, col.MemeID)
	}

	var memes []models.Meme
	if len(memeIDs) > 0 {
		h.db.Where("id IN ?", memeIDs).Find(&memes)
	}

	items := make([]MemeResponse, 0, len(memes))
	for _, m := range memes {
		items = append(items, mapMeme(m, h.isMemeLiked(uid, m.ID), true))
	}

	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// Helper functions

func (h *Handler) requireAuth(c *gin.Context) (uint64, bool) {
	uidAny, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return 0, false
	}
	uid, ok := uidAny.(uint64)
	if !ok || uid == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return 0, false
	}
	return uid, true
}

func (h *Handler) isMemeLiked(userID, memeID uint64) bool {
	var count int64
	h.db.Model(&models.UserLike{}).Where("user_id = ? AND meme_id = ?", userID, memeID).Count(&count)
	return count > 0
}

func (h *Handler) isMemeCollected(userID, memeID uint64) bool {
	var count int64
	h.db.Model(&models.UserCollect{}).Where("user_id = ? AND meme_id = ?", userID, memeID).Count(&count)
	return count > 0
}

func mapMeme(m models.Meme, liked, collected bool) MemeResponse {
	return MemeResponse{
		ID:           m.ID,
		CreatorID:    m.CreatorID,
		InputText:    m.InputText,
		MemeText:     m.MemeText,
		ImageURL:     m.ImageURL,
		LikeCount:    m.LikeCount,
		CollectCount: m.CollectCount,
		IsPublic:     m.IsPublic,
		Liked:        liked,
		Collected:    collected,
		CreatedAt:    m.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
