package models

import "time"

type UserCollect struct {
	UserID    uint64    `gorm:"primaryKey" json:"user_id"`
	MemeID    uint64    `gorm:"primaryKey" json:"meme_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (UserCollect) TableName() string {
	return "action.user_collects"
}
