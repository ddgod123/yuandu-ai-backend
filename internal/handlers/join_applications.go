package handlers

import (
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/gin-gonic/gin"
)

type JoinApplicationRequest struct {
	Name       string `json:"name"`
	Phone      string `json:"phone"`
	Gender     string `json:"gender"`
	Age        int    `json:"age"`
	Email      string `json:"email"`
	Occupation string `json:"occupation"`
}

type JoinApplicationResponse struct {
	ID         uint64    `json:"id"`
	Name       string    `json:"name"`
	Phone      string    `json:"phone"`
	Gender     string    `json:"gender"`
	Age        int       `json:"age"`
	Email      string    `json:"email"`
	Occupation string    `json:"occupation"`
	CreatedAt  time.Time `json:"created_at"`
}

type JoinApplicationListResponse struct {
	Items []JoinApplicationResponse `json:"items"`
	Total int64                     `json:"total"`
}

var phonePattern = regexp.MustCompile(`^[0-9+\-\s]{6,32}$`)

func normalizeJoinGender(raw string) (string, bool) {
	v := strings.TrimSpace(raw)
	switch strings.ToLower(v) {
	case "男", "male", "m":
		return "男", true
	case "女", "female", "f":
		return "女", true
	case "其他", "other", "o":
		return "其他", true
	case "保密", "unknown", "u":
		return "保密", true
	default:
		return "", false
	}
}

func validateJoinApplicationRequest(req JoinApplicationRequest) (JoinApplicationRequest, string) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return req, "name required"
	}
	if len(name) > 64 {
		return req, "name too long"
	}

	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		return req, "phone required"
	}
	if !phonePattern.MatchString(phone) {
		return req, "invalid phone"
	}

	gender, ok := normalizeJoinGender(req.Gender)
	if !ok {
		return req, "invalid gender"
	}

	if req.Age < 1 || req.Age > 120 {
		return req, "invalid age"
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		return req, "email required"
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return req, "invalid email"
	}

	occupation := strings.TrimSpace(req.Occupation)
	if occupation == "" {
		return req, "occupation required"
	}
	if len(occupation) > 64 {
		return req, "occupation too long"
	}

	req.Name = name
	req.Phone = phone
	req.Gender = gender
	req.Email = email
	req.Occupation = occupation
	return req, ""
}

// CreateJoinApplication godoc
// @Summary Submit join application
// @Tags public
// @Accept json
// @Produce json
// @Param body body JoinApplicationRequest true "join application request"
// @Success 200 {object} JoinApplicationResponse
// @Router /api/join-applications [post]
func (h *Handler) CreateJoinApplication(c *gin.Context) {
	var req JoinApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validReq, validationErr := validateJoinApplicationRequest(req)
	if validationErr != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErr})
		return
	}

	application := models.JoinApplication{
		Name:       validReq.Name,
		Phone:      validReq.Phone,
		Gender:     validReq.Gender,
		Age:        validReq.Age,
		Email:      validReq.Email,
		Occupation: validReq.Occupation,
	}
	if err := h.db.Create(&application).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, JoinApplicationResponse{
		ID:         application.ID,
		Name:       application.Name,
		Phone:      application.Phone,
		Gender:     application.Gender,
		Age:        application.Age,
		Email:      application.Email,
		Occupation: application.Occupation,
		CreatedAt:  application.CreatedAt,
	})
}

// ListJoinApplications godoc
// @Summary List join applications (admin)
// @Tags admin
// @Produce json
// @Param page query int false "page"
// @Param page_size query int false "page size"
// @Param keyword query string false "keyword"
// @Success 200 {object} JoinApplicationListResponse
// @Router /api/admin/join-applications [get]
func (h *Handler) ListJoinApplications(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := h.db.Model(&models.JoinApplication{})
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where(
			"name ILIKE ? OR phone ILIKE ? OR email ILIKE ? OR occupation ILIKE ?",
			like, like, like, like,
		)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var items []models.JoinApplication
	if err := db.Order("created_at DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	respItems := make([]JoinApplicationResponse, 0, len(items))
	for _, item := range items {
		respItems = append(respItems, JoinApplicationResponse{
			ID:         item.ID,
			Name:       item.Name,
			Phone:      item.Phone,
			Gender:     item.Gender,
			Age:        item.Age,
			Email:      item.Email,
			Occupation: item.Occupation,
			CreatedAt:  item.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, JoinApplicationListResponse{
		Items: respItems,
		Total: total,
	})
}
