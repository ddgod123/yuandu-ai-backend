package handlers

import (
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/gorm"
)

func resolveUserSubscriptionState(user *models.User, now time.Time) (status string, level string, isSubscriber bool) {
	if user == nil {
		return "inactive", "free", false
	}

	status = strings.ToLower(strings.TrimSpace(user.SubscriptionStatus))
	if status == "" {
		status = "inactive"
	}

	expiresAt := user.SubscriptionExpiresAt
	if status == "active" && expiresAt != nil && now.After(*expiresAt) {
		status = "expired"
	}

	isSubscriber = status == "active" && (expiresAt == nil || now.Before(*expiresAt) || now.Equal(*expiresAt))
	if isSubscriber {
		return status, "subscriber", true
	}
	return status, "free", false
}

func syncExpiredSubscription(db *gorm.DB, user *models.User, now time.Time) {
	if db == nil || user == nil {
		return
	}
	if strings.ToLower(strings.TrimSpace(user.SubscriptionStatus)) != "active" {
		return
	}
	if user.SubscriptionExpiresAt == nil || !now.After(*user.SubscriptionExpiresAt) {
		return
	}
	user.SubscriptionStatus = "expired"
	_ = db.Model(&models.User{}).Where("id = ?", user.ID).Update("subscription_status", "expired").Error
}
