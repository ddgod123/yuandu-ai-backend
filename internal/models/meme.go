package models

import (
	"time"

	"gorm.io/gorm"
)

// Meme represents a user-generated meme (text + template composited image).
type Meme struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatorID    uint64         `gorm:"index" json:"creator_id"`
	InputText    string         `gorm:"type:text" json:"input_text"`
	MemeText     string         `gorm:"type:text" json:"meme_text"`
	TemplateID   *uint64        `gorm:"index" json:"template_id"`
	ImageURL     string         `gorm:"size:512" json:"image_url"`
	LikeCount    int            `gorm:"default:0" json:"like_count"`
	CollectCount int            `gorm:"default:0" json:"collect_count"`
	IsPublic     bool           `gorm:"default:true;index" json:"is_public"`
	CreatedAt    time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Meme) TableName() string {
	return "meme.memes"
}
