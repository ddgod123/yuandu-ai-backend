package models

import (
	"time"

	"gorm.io/gorm"
)

type MemePhrase struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Phrase     string         `gorm:"type:text" json:"phrase"`
	Category   string         `gorm:"size:64;index" json:"category"`
	Emotion    string         `gorm:"size:64;index" json:"emotion"`
	Context    string         `gorm:"size:255" json:"context"`
	HotScore   int            `gorm:"index;default:0" json:"hot_score"`
	TemplateID *uint64        `gorm:"index" json:"template_id"`
	Status     string         `gorm:"size:32;index;default:active" json:"status"`
	CreatedAt  time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (MemePhrase) TableName() string {
	return "meme.phrases"
}
