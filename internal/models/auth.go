package models

import "time"

type AdminRole struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	UserID    uint64    `gorm:"index"`
	Role      string    `gorm:"size:32;index"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (AdminRole) TableName() string {
	return "user.admin_roles"
}

type RefreshToken struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement"`
	UserID    uint64     `gorm:"index"`
	TokenHash string     `gorm:"size:64;uniqueIndex"`
	ExpiresAt time.Time  `gorm:"index"`
	RevokedAt *time.Time `gorm:"index"`
	CreatedAt time.Time  `gorm:"autoCreateTime"`
}

func (RefreshToken) TableName() string {
	return "user.refresh_tokens"
}
