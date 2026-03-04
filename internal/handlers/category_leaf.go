package handlers

import (
	"errors"

	"emoji/internal/models"
)

var (
	errCategoryHasChildren = errors.New("category has children")
	errInvalidCategory     = errors.New("invalid category")
)

func (h *Handler) requireLeafCategory(categoryID uint64) error {
	if categoryID == 0 {
		return errInvalidCategory
	}
	var cat models.Category
	if err := h.db.Where("deleted_at IS NULL").First(&cat, categoryID).Error; err != nil {
		return err
	}
	var count int64
	if err := h.db.Model(&models.Category{}).
		Where("parent_id = ? AND deleted_at IS NULL", categoryID).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errCategoryHasChildren
	}
	return nil
}
